package websocket

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrNilDialer              = errors.New("websocket: nil dialer")
	ErrNilDecoder             = errors.New("websocket: nil topic decoder")
	ErrNoEncoder              = errors.New("websocket: nil control encoder")
	ErrBadConfig              = errors.New("websocket: invalid config")
	ErrNoPool                 = errors.New("websocket: nil buffer pool")
	ErrFrameTooLarge          = errors.New("websocket: frame exceeds buffer")
	ErrTopicAlreadySubscribed = errors.New("websocket: topic already subscribed")
	ErrUnknownTopic           = errors.New("websocket: unknown topic")
	ErrAlreadyRunning         = errors.New("websocket: manager already running")
	ErrNotRunning             = errors.New("websocket: manager not running")
)

const (
	DefaultWriteQueueSize   = 1024
	defaultMaxTopicsPerConn = 10
)

/*
func (m *Manager) AcquireOutbound(msgType MessageType, size int) *outboundFrame
func (m *Manager) AddConsumer(topic TopicID, consumer *Consumer) error
func (m *Manager) EnqueueOutbound(topic TopicID, frame *outboundFrame) error
func (m *Manager) RemoveConsumer(topic TopicID, consumer *Consumer) error
func (m *Manager) Run(ctx context.Context) error
func (m *Manager) Send(msgType MessageType, payload []byte) error
func (m *Manager) Subscribe(topic TopicID) error
func (m *Manager) Unsubscribe(topic TopicID) error
func (m *Manager) pickSessionLocked() *session
func (m *Manager) removeSessionLocked(target *session)
func (m *Manager) stopAll()
*/

// Option defines the manager runtime configuration.
type Option struct {
	// MaxTopicsPerConn caps topics per session connection. Optional; default 10.
	MaxTopicsPerConn int

	// FanOut controls how frames are fanned out to consumers. Optional; default FanOutCopy.
	FanOut FanOutMode
	// WriteQueueSize is the outbound write queue capacity. Optional; default DefaultWriteQueueSize (1024).
	WriteQueueSize int
	// WriteOverflow sets the policy when the write queue is full. Optional; default OverflowBlock.
	WriteOverflow OverflowPolicy
	// PingInterval enables periodic ping frames when >0. Optional; default 0 (disabled).
	PingInterval time.Duration
	// Backoff defines reconnect backoff. Optional; default DefaultBackoff when all fields are zero.
	Backoff Backoff
	// OnConnect runs after a connection is established, before resubscribe; return error to abort. Optional; default nil.
	OnConnect func(ctx context.Context, w Writer) error
	// OnDisconnect runs after a session ends with the terminal error. Optional; default nil.
	OnDisconnect func(err error)

	// dialer establishes new WebSocket connections. Required (nil returns ErrNilDialer).
	dialer Dialer
	// decoder extracts topic IDs from incoming frames. Required (nil returns ErrNilDecoder).
	decoder TopicDecoder
	// encoder builds subscribe/unsubscribe control frames. Required for Subscribe/Unsubscribe (nil returns ErrNoEncoder).
	encoder ControlEncoder
	// bufferPool is internal and wired by New; callers should leave nil.
	bufferPool *bufferPool
	// framePool is internal and wired by New; callers should leave nil.
	framePool *framePool
	// outboundPool is internal and wired by New; callers should leave nil.
	outboundPool *outboundPool
	// router is internal and wired by New; callers should leave nil.
	router *router
}

func (opt *Option) init(dialer Dialer, decoder TopicDecoder, encoder ControlEncoder) {
	opt.dialer = dialer
	opt.decoder = decoder
	opt.encoder = encoder

	if opt.MaxTopicsPerConn <= 0 {
		opt.MaxTopicsPerConn = defaultMaxTopicsPerConn
	}

	bp := defaultBufferPool()
	fp := newFramePool(bp)

	opt.bufferPool = bp
	opt.framePool = fp
	opt.outboundPool = newOutboundPool(bp)
	opt.router = newRouter(opt.FanOut, fp)

	if opt.WriteQueueSize <= 0 {
		opt.WriteQueueSize = DefaultWriteQueueSize
	}

	if opt.Backoff.Min == 0 && opt.Backoff.Max == 0 && opt.Backoff.Factor == 0 && opt.Backoff.Jitter == 0 {
		opt.Backoff = DefaultBackoff()
	}
}

type subscription struct {
	topic   TopicID
	session *session
}

// Manager owns the WebSocket lifecycle and routing.
type Manager struct {
	opt          Option
	router       *router
	bufferPool   *bufferPool
	framePool    *framePool
	outboundPool *outboundPool

	mu            sync.Mutex
	sessions      []*session
	subscriptions map[TopicID]*subscription
	nextSessionID uint64
	ctx           context.Context
	running       atomic.Bool
	wg            sync.WaitGroup
}

// New validates config and builds a manager.
func New(dialer Dialer, decoder TopicDecoder, encoder ControlEncoder, option ...Option) (*Manager, error) {
	if dialer == nil {
		return nil, ErrNilDialer
	}

	if decoder == nil {
		return nil, ErrNilDecoder
	}

	if encoder == nil {
		return nil, ErrNoEncoder
	}

	var opt Option
	if len(option) != 0 {
		opt = option[0]
	}

	opt.init(dialer, decoder, encoder)

	manager := &Manager{
		opt:           opt,
		router:        opt.router,
		bufferPool:    opt.bufferPool,
		framePool:     opt.framePool,
		outboundPool:  opt.outboundPool,
		subscriptions: make(map[TopicID]*subscription),
	}
	return manager, nil
}

// Run starts the connection lifecycle and blocks until ctx is done.
func (m *Manager) Run(ctx context.Context) error {
	if m == nil {
		return ErrBadConfig
	}
	m.mu.Lock()
	if m.running.Load() {
		m.mu.Unlock()
		return ErrAlreadyRunning
	}
	m.running.Store(true)
	m.ctx = ctx
	sessions := append([]*session(nil), m.sessions...)
	m.mu.Unlock()

	for _, s := range sessions {
		s.start(ctx, &m.wg)
	}

	<-ctx.Done()

	m.stopAll()
	m.wg.Wait()
	m.mu.Lock()
	m.ctx = nil
	m.running.Store(false)
	m.mu.Unlock()
	return ctx.Err()
}

// Subscribe registers a topic subscription.
func (m *Manager) Subscribe(topic TopicID) error {
	if m == nil {
		return ErrBadConfig
	}

	if m.opt.encoder == nil {
		return ErrNoEncoder
	}

	m.mu.Lock()

	if m.ctx != nil && m.ctx.Err() != nil {
		m.mu.Unlock()
		return ErrNotRunning
	}

	if _, ok := m.subscriptions[topic]; ok {
		m.mu.Unlock()
		return ErrTopicAlreadySubscribed
	}

	s := m.pickSessionLocked()
	s.subscriptions.Add(topic)
	sub := &subscription{topic: topic, session: s}
	m.subscriptions[topic] = sub
	ctx := m.ctx
	startNow := ctx != nil
	m.mu.Unlock()

	if startNow {
		s.start(ctx, &m.wg)
	}
	return s.subscribe(topic)
}

// Unsubscribe removes a subscription by topic id.
func (m *Manager) Unsubscribe(topic TopicID) error {
	if m == nil {
		return ErrBadConfig
	}
	m.mu.Lock()
	sub, ok := m.subscriptions[topic]
	if !ok {
		m.mu.Unlock()
		return ErrUnknownTopic
	}
	delete(m.subscriptions, topic)
	_ = sub.session.subscriptions.Remove(topic)
	empty := sub.session.subscriptions.Count() == 0
	if empty {
		m.removeSessionLocked(sub.session)
	}
	m.mu.Unlock()

	m.router.RemoveTopic(topic)

	if err := sub.session.unsubscribe(topic); err != nil {
		return err
	}
	if empty {
		sub.session.stop()
	}
	return nil
}

// AddConsumer registers a consumer for a subscription.
func (m *Manager) AddConsumer(topic TopicID, consumer *Consumer) error {
	if m == nil {
		return ErrBadConfig
	}
	m.mu.Lock()
	sub, ok := m.subscriptions[topic]
	m.mu.Unlock()
	if !ok {
		return ErrUnknownTopic
	}
	m.router.AddConsumer(sub.topic, consumer)
	return nil
}

// RemoveConsumer unregisters a consumer for a subscription.
func (m *Manager) RemoveConsumer(topic TopicID, consumer *Consumer) error {
	if m == nil {
		return ErrBadConfig
	}
	m.mu.Lock()
	sub, ok := m.subscriptions[topic]
	m.mu.Unlock()
	if !ok {
		return ErrUnknownTopic
	}
	m.router.RemoveConsumer(sub.topic, consumer)
	return nil
}

// Send enqueues an outbound message by copying payload.
func (m *Manager) Send(msgType MessageType, payload []byte) error {
	if m == nil {
		return ErrBadConfig
	}
	sessions := m.collectSessions()
	var firstErr error
	for _, s := range sessions {
		if !s.connected.Load() {
			if firstErr == nil {
				firstErr = ErrNotConnected
			}
			continue
		}
		if !s.writer.Send(msgType, payload) {
			if firstErr == nil {
				firstErr = ErrQueueFull
			}
		}
	}
	return firstErr
}

// AcquireOutbound returns a pooled frame for zero-copy sending.
func (m *Manager) AcquireOutbound(msgType MessageType, size int) *outboundFrame {
	if m == nil {
		return nil
	}
	if m.outboundPool == nil || m.outboundPool.buffers == nil {
		return &outboundFrame{MsgType: msgType, Buf: make([]byte, size)}
	}
	buf := m.outboundPool.buffers.Get(size)
	return m.outboundPool.New(msgType, buf[:size])
}

// EnqueueOutbound enqueues a pooled outbound frame.
func (m *Manager) EnqueueOutbound(topic TopicID, frame *outboundFrame) error {
	if m == nil {
		return ErrBadConfig
	}
	m.mu.Lock()
	sub, ok := m.subscriptions[topic]
	m.mu.Unlock()
	if !ok {
		return ErrUnknownTopic
	}
	if !sub.session.connected.Load() {
		return ErrNotConnected
	}
	if !sub.session.writer.Enqueue(frame) {
		if frame != nil {
			frame.Release()
		}
		return ErrQueueFull
	}
	return nil
}

func (m *Manager) collectSessions() []*session {
	m.mu.Lock()
	sessions := append([]*session(nil), m.sessions...)
	m.mu.Unlock()
	return sessions
}

func (m *Manager) pickSessionLocked() *session {
	for _, s := range m.sessions {
		if s.subscriptions.Count() < m.opt.MaxTopicsPerConn {
			return s
		}
	}
	m.nextSessionID++
	s := newSession(m.nextSessionID, &m.opt, m.router, m.bufferPool, m.framePool, m.outboundPool)
	m.sessions = append(m.sessions, s)
	return s
}

func (m *Manager) removeSessionLocked(target *session) {
	for i, s := range m.sessions {
		if s == target {
			m.sessions[i] = m.sessions[len(m.sessions)-1]
			m.sessions = m.sessions[:len(m.sessions)-1]
			return
		}
	}
}

func (m *Manager) stopAll() {
	m.mu.Lock()
	sessions := append([]*session(nil), m.sessions...)
	m.mu.Unlock()
	for _, s := range sessions {
		s.stop()
	}
}

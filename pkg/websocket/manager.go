package websocket

import (
	"context"
	"errors"
	"sync/atomic"
	"time"
)

var (
	ErrNilDialer     = errors.New("websocket: nil dialer")
	ErrNilParser     = errors.New("websocket: nil topic parser")
	ErrNoEncoder     = errors.New("websocket: nil control encoder")
	ErrBadConfig     = errors.New("websocket: invalid config")
	ErrNoPool        = errors.New("websocket: nil buffer pool")
	ErrFrameTooLarge = errors.New("websocket: frame exceeds max size")
)

// Config defines the manager runtime configuration.
type Config struct {
	Dialer            Dialer
	Parser            TopicParser
	Encoder           ControlEncoder
	BufferPool        *BufferPool
	FramePool         *FramePool
	OutboundPool      *OutboundPool
	Router            *Router
	Subscriptions     *Subscriptions
	Fanout            FanoutMode
	MaxFrameSize      int
	ControlBufferSize int
	WriteQueueSize    int
	WriteOverflow     OverflowPolicy
	PingInterval      time.Duration
	Backoff           Backoff
	OnConnect         func(ctx context.Context, w *Writer) error
	OnDisconnect      func(err error)
}

// Manager owns the WebSocket lifecycle and routing.
type Manager struct {
	cfg            Config
	writer         *Writer
	router         *Router
	subscriptions  *Subscriptions
	bufferPool     *BufferPool
	framePool      *FramePool
	outboundPool   *OutboundPool
	controlBufSize int
	maxFrameSize   int
	connected      atomic.Bool
}

// NewManager validates config and builds a manager.
func NewManager(cfg Config) (*Manager, error) {
	if cfg.Dialer == nil {
		return nil, ErrNilDialer
	}
	if cfg.Parser == nil {
		return nil, ErrNilParser
	}
	if cfg.MaxFrameSize <= 0 {
		return nil, ErrBadConfig
	}
	if cfg.BufferPool == nil {
		cfg.BufferPool = DefaultBufferPool()
	}
	if cfg.FramePool == nil {
		cfg.FramePool = NewFramePool(cfg.BufferPool)
	}
	if cfg.OutboundPool == nil {
		cfg.OutboundPool = NewOutboundPool(cfg.BufferPool)
	}
	if cfg.Router == nil {
		cfg.Router = NewRouter(cfg.Fanout, cfg.FramePool)
	}
	if cfg.Subscriptions == nil {
		cfg.Subscriptions = NewSubscriptions()
	}
	if cfg.ControlBufferSize <= 0 {
		cfg.ControlBufferSize = cfg.MaxFrameSize
	}
	if cfg.WriteQueueSize <= 0 {
		cfg.WriteQueueSize = 1024
	}
	if cfg.Backoff.Min == 0 && cfg.Backoff.Max == 0 && cfg.Backoff.Factor == 0 && cfg.Backoff.Jitter == 0 {
		cfg.Backoff = DefaultBackoff()
	}

	manager := &Manager{
		cfg:            cfg,
		writer:         NewWriter(cfg.OutboundPool, cfg.WriteQueueSize, cfg.WriteOverflow),
		router:         cfg.Router,
		subscriptions:  cfg.Subscriptions,
		bufferPool:     cfg.BufferPool,
		framePool:      cfg.FramePool,
		outboundPool:   cfg.OutboundPool,
		controlBufSize: cfg.ControlBufferSize,
		maxFrameSize:   cfg.MaxFrameSize,
	}
	return manager, nil
}

// Run starts the connection lifecycle and blocks until ctx is done.
func (m *Manager) Run(ctx context.Context) error {
	if m == nil {
		return ErrBadConfig
	}
	attempt := 0
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		conn, err := m.cfg.Dialer.Dial(ctx)
		if err != nil {
			attempt++
			m.sleepBackoff(ctx, attempt)
			continue
		}

		attempt = 0
		m.connected.Store(true)
		m.writer.SetConnected(true)

		if m.cfg.OnConnect != nil {
			if err := m.cfg.OnConnect(ctx, m.writer); err != nil {
				_ = conn.Close(CloseNormal, "on_connect_failed")
				m.connected.Store(false)
				m.writer.SetConnected(false)
				m.writer.Drain()
				attempt++
				m.sleepBackoff(ctx, attempt)
				continue
			}
		}

		m.subscriptions.ClearActive()
		if err := m.resubscribe(); err != nil {
			_ = conn.Close(CloseNormal, "resubscribe_failed")
			m.connected.Store(false)
			m.writer.SetConnected(false)
			m.writer.Drain()
			attempt++
			m.sleepBackoff(ctx, attempt)
			continue
		}

		err = m.runSession(ctx, conn)
		if m.cfg.OnDisconnect != nil {
			m.cfg.OnDisconnect(err)
		}
		m.connected.Store(false)
		m.writer.SetConnected(false)
		m.writer.Drain()
		_ = conn.Close(CloseNormal, "session_end")

		if ctx.Err() != nil {
			return ctx.Err()
		}
		attempt++
		m.sleepBackoff(ctx, attempt)
	}
}

// Subscribe increments refcount and optionally sends a subscribe message.
func (m *Manager) Subscribe(topic TopicID) error {
	if m == nil {
		return ErrBadConfig
	}
	if m.cfg.Encoder == nil {
		return ErrNoEncoder
	}
	first := m.subscriptions.Inc(topic)
	if !first {
		return nil
	}
	if !m.connected.Load() {
		return nil
	}
	if err := m.sendSubscribe(topic); err != nil {
		return err
	}
	m.subscriptions.MarkActive(topic)
	return nil
}

// Unsubscribe decrements refcount and optionally sends an unsubscribe message.
func (m *Manager) Unsubscribe(topic TopicID) error {
	if m == nil {
		return ErrBadConfig
	}
	if m.cfg.Encoder == nil {
		return ErrNoEncoder
	}
	last := m.subscriptions.Dec(topic)
	if !last {
		return nil
	}
	if !m.connected.Load() {
		return nil
	}
	return m.sendUnsubscribe(topic)
}

// AddConsumer registers a consumer and updates subscription state.
func (m *Manager) AddConsumer(topic TopicID, consumer *Consumer) error {
	if m == nil {
		return ErrBadConfig
	}
	m.router.AddConsumer(topic, consumer)
	if err := m.Subscribe(topic); err != nil {
		m.router.RemoveConsumer(topic, consumer)
		return err
	}
	return nil
}

// RemoveConsumer unregisters a consumer and updates subscription state.
func (m *Manager) RemoveConsumer(topic TopicID, consumer *Consumer) error {
	if m == nil {
		return ErrBadConfig
	}
	m.router.RemoveConsumer(topic, consumer)
	return m.Unsubscribe(topic)
}

// Send enqueues an outbound message by copying payload.
func (m *Manager) Send(msgType MessageType, payload []byte) error {
	if m == nil {
		return ErrBadConfig
	}
	if !m.connected.Load() {
		return ErrNotConnected
	}
	if !m.writer.Send(msgType, payload) {
		return ErrQueueFull
	}
	return nil
}

// AcquireOutbound returns a pooled frame for zero-copy sending.
func (m *Manager) AcquireOutbound(msgType MessageType, size int) *OutboundFrame {
	if m == nil {
		return nil
	}
	return m.writer.Acquire(msgType, size)
}

// EnqueueOutbound enqueues a pooled outbound frame.
func (m *Manager) EnqueueOutbound(frame *OutboundFrame) error {
	if m == nil {
		return ErrBadConfig
	}
	if !m.connected.Load() {
		return ErrNotConnected
	}
	if !m.writer.Enqueue(frame) {
		if frame != nil {
			frame.Release()
		}
		return ErrQueueFull
	}
	return nil
}

func (m *Manager) resubscribe() error {
	topics := m.subscriptions.Desired(nil)
	if len(topics) == 0 {
		return nil
	}
	if m.cfg.Encoder == nil {
		return ErrNoEncoder
	}
	for _, topic := range topics {
		if err := m.sendSubscribe(topic); err != nil {
			return err
		}
		m.subscriptions.MarkActive(topic)
	}
	return nil
}

func (m *Manager) sendSubscribe(topic TopicID) error {
	payload, msgType, err := m.encodeControl(func(dst []byte) (MessageType, []byte, error) {
		return m.cfg.Encoder.EncodeSubscribe(dst, topic)
	})
	if err != nil {
		return err
	}
	frame := m.outboundPool.New(msgType, payload)
	if !m.writer.Enqueue(frame) {
		frame.Release()
		return ErrQueueFull
	}
	return nil
}

func (m *Manager) sendUnsubscribe(topic TopicID) error {
	payload, msgType, err := m.encodeControl(func(dst []byte) (MessageType, []byte, error) {
		return m.cfg.Encoder.EncodeUnsubscribe(dst, topic)
	})
	if err != nil {
		return err
	}
	frame := m.outboundPool.New(msgType, payload)
	if !m.writer.Enqueue(frame) {
		frame.Release()
		return ErrQueueFull
	}
	return nil
}

func (m *Manager) encodeControl(encode func(dst []byte) (MessageType, []byte, error)) ([]byte, MessageType, error) {
	if m.bufferPool == nil {
		return nil, 0, ErrNoPool
	}
	buf := m.bufferPool.Get(m.controlBufSize)
	msgType, payload, err := encode(buf[:0])
	if err != nil {
		m.bufferPool.Put(buf)
		return nil, 0, err
	}
	if len(payload) == 0 {
		payload = buf[:0]
	}
	if len(payload) > cap(buf) {
		m.bufferPool.Put(buf)
		return nil, 0, ErrFrameTooLarge
	}
	return payload, msgType, nil
}

func (m *Manager) runSession(ctx context.Context, conn Conn) error {
	sessionCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 1)
	go m.readLoop(sessionCtx, conn, errCh)

	var pingTicker *time.Ticker
	if m.cfg.PingInterval > 0 {
		pingTicker = time.NewTicker(m.cfg.PingInterval)
		defer pingTicker.Stop()
	}
	var ping <-chan time.Time
	if pingTicker != nil {
		ping = pingTicker.C
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errCh:
			return err
		case frame := <-m.writer.queue:
			if frame == nil {
				continue
			}
			if err := conn.Write(sessionCtx, frame.MsgType, frame.Buf); err != nil {
				frame.Release()
				return err
			}
			frame.Release()
		case <-ping:
			m.writer.Send(MessagePing, nil)
		}
	}
}

func (m *Manager) readLoop(ctx context.Context, conn Conn, errCh chan<- error) {
	base := time.Now()
	for {
		buf := m.bufferPool.Get(m.maxFrameSize)
		n, msgType, err := conn.Read(ctx, buf)
		if err != nil {
			m.bufferPool.Put(buf)
			errCh <- err
			return
		}
		if n <= 0 {
			m.bufferPool.Put(buf)
			continue
		}
		if msgType != MessageText && msgType != MessageBinary {
			m.bufferPool.Put(buf)
			continue
		}
		frame := m.framePool.New(buf[:n])
		frame.Meta = uint64(time.Since(base))
		topic, ok := m.cfg.Parser.ParseTopic(frame.Buf)
		if !ok {
			frame.Release()
			continue
		}
		frame.Topic = topic
		m.router.Route(frame)
	}
}

func (m *Manager) sleepBackoff(ctx context.Context, attempt int) {
	wait := m.cfg.Backoff.Next(attempt)
	if wait <= 0 {
		return
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

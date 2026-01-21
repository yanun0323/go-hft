package websocket

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

type session struct {
	id             uint64
	opt            *Option
	writer         *writer
	router         *router
	subscriptions  *subscriptions
	bufferPool     *bufferPool
	framePool      *framePool
	outboundPool   *outboundPool
	controlBufSize int
	frameSize      int
	connected      atomic.Bool
	writeMu        sync.Mutex
	cancel         context.CancelFunc
	done           chan struct{}
	running        atomic.Bool
}

func newSession(id uint64, opt *Option, router *router, bufferPool *bufferPool, framePool *framePool, outboundPool *outboundPool) *session {
	return &session{
		id:             id,
		opt:            opt,
		writer:         newWriter(outboundPool, opt.WriteQueueSize, opt.WriteOverflow),
		router:         router,
		subscriptions:  newSubscriptions(),
		bufferPool:     bufferPool,
		framePool:      framePool,
		outboundPool:   outboundPool,
		controlBufSize: opt.bufferPool.InitSize(),
		frameSize:      opt.bufferPool.InitSize(),
		done:           make(chan struct{}),
	}
}

func (s *session) start(parent context.Context, wg *sync.WaitGroup) {
	if parent == nil {
		return
	}
	if !s.running.CompareAndSwap(false, true) {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.run(ctx)
		close(s.done)
		s.running.Store(false)
	}()
}

func (s *session) stop() {
	if s == nil {
		return
	}
	if s.cancel != nil {
		s.cancel()
	}
}

func (s *session) subscribe(topic TopicID, id ConnectionID) error {
	if s == nil {
		return ErrBadConfig
	}
	if s.opt.encoder == nil {
		return ErrNoEncoder
	}
	if !s.connected.Load() {
		return nil
	}
	if err := s.sendSubscribe(id, topic); err != nil {
		return err
	}
	s.subscriptions.MarkActive(topic)
	return nil
}

func (s *session) unsubscribe(topic TopicID, id ConnectionID) error {
	if s == nil {
		return ErrBadConfig
	}
	if s.opt.encoder == nil {
		return ErrNoEncoder
	}
	if !s.connected.Load() {
		return nil
	}
	return s.sendUnsubscribe(id, topic)
}

func (s *session) run(ctx context.Context) {
	attempt := 0
	for {
		if ctx.Err() != nil {
			return
		}
		if s.subscriptions.Count() == 0 {
			return
		}
		conn, err := s.opt.dialer.Dial(ctx)
		if err != nil {
			attempt++
			s.sleepBackoff(ctx, attempt)
			continue
		}

		attempt = 0
		s.connected.Store(true)
		s.writer.SetConnected(true)

		if s.opt.OnConnect != nil {
			if err := s.opt.OnConnect(ctx, s.writer); err != nil {
				_ = conn.Close(CloseNormal, "on_connect_failed")
				s.connected.Store(false)
				s.writer.SetConnected(false)
				s.writer.Drain()
				attempt++
				s.sleepBackoff(ctx, attempt)
				continue
			}
		}

		s.subscriptions.ClearActive()
		if err := s.resubscribe(); err != nil {
			_ = conn.Close(CloseNormal, "resubscribe_failed")
			s.connected.Store(false)
			s.writer.SetConnected(false)
			s.writer.Drain()
			attempt++
			s.sleepBackoff(ctx, attempt)
			continue
		}

		err = s.runSession(ctx, conn)
		if s.opt.OnDisconnect != nil {
			s.opt.OnDisconnect(err)
		}
		s.connected.Store(false)
		s.writer.SetConnected(false)
		s.writer.Drain()
		_ = conn.Close(CloseNormal, "session_end")

		if ctx.Err() != nil {
			return
		}
		attempt++
		s.sleepBackoff(ctx, attempt)
	}
}

func (s *session) resubscribe() error {
	desired := s.subscriptions.Desired(nil)
	if len(desired) == 0 {
		return nil
	}
	if s.opt.encoder == nil {
		return ErrNoEncoder
	}
	for _, sub := range desired {
		if err := s.sendSubscribe(sub.ID, sub.Topic); err != nil {
			return err
		}
		s.subscriptions.MarkActive(sub.Topic)
	}
	return nil
}

func (s *session) sendSubscribe(id ConnectionID, topic TopicID) error {
	payload, msgType, err := s.encodeControl(func(dst []byte) (MessageType, []byte, error) {
		return s.opt.encoder.EncodeSubscribe(dst, id, topic)
	})
	if err != nil {
		return err
	}
	frame := s.outboundPool.New(msgType, payload)
	if !s.writer.Enqueue(frame) {
		frame.Release()
		return ErrQueueFull
	}
	return nil
}

func (s *session) sendUnsubscribe(id ConnectionID, topic TopicID) error {
	payload, msgType, err := s.encodeControl(func(dst []byte) (MessageType, []byte, error) {
		return s.opt.encoder.EncodeUnsubscribe(dst, id, topic)
	})
	if err != nil {
		return err
	}
	frame := s.outboundPool.New(msgType, payload)
	if !s.writer.Enqueue(frame) {
		frame.Release()
		return ErrQueueFull
	}
	return nil
}

func (s *session) encodeControl(encode func(dst []byte) (MessageType, []byte, error)) ([]byte, MessageType, error) {
	if s.bufferPool == nil {
		return nil, 0, ErrNoPool
	}
	buf := s.bufferPool.Get(s.controlBufSize)
	msgType, payload, err := encode(buf[:0])
	if err != nil {
		s.bufferPool.Put(buf)
		return nil, 0, err
	}
	if len(payload) == 0 {
		payload = buf[:0]
	}
	if len(payload) > cap(buf) {
		s.bufferPool.Put(buf)
		bufSize := len(payload)
		if bufSize <= 0 {
			return nil, 0, ErrFrameTooLarge
		}
		buf = s.bufferPool.Get(bufSize)
		msgType, payload, err = encode(buf[:0])
		if err != nil {
			s.bufferPool.Put(buf)
			return nil, 0, err
		}
		if len(payload) == 0 {
			payload = buf[:0]
		}
		if len(payload) > cap(buf) {
			s.bufferPool.Put(buf)
			return nil, 0, ErrFrameTooLarge
		}
	}
	return payload, msgType, nil
}

func (s *session) runSession(ctx context.Context, conn Conn) error {
	sessionCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 1)
	go s.readLoop(sessionCtx, conn, errCh)

	var pingTicker *time.Ticker
	if s.opt.PingInterval > 0 {
		pingTicker = time.NewTicker(s.opt.PingInterval)
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
		case frame := <-s.writer.queue:
			if frame == nil {
				continue
			}
			s.writeMu.Lock()
			err := conn.Write(sessionCtx, frame.MsgType, frame.Buf)
			s.writeMu.Unlock()
			if err != nil {
				frame.Release()
				return err
			}
			frame.Release()
		case <-ping:
			s.writer.Send(MessagePing, nil)
		}
	}
}

func (s *session) readLoop(ctx context.Context, conn Conn, errCh chan<- error) {
	base := time.Now()
	for {
		buf := s.bufferPool.Get(s.frameSize)
		n, msgType, err := conn.Read(ctx, buf)
		if err != nil {
			s.bufferPool.Put(buf)
			if errors.Is(err, errFrameTooLarge) {
				if s.tryGrowReadBuffer(len(buf), cap(buf)) {
					continue
				}
				errCh <- ErrFrameTooLarge
				return
			}
			errCh <- err
			return
		}
		if n <= 0 {
			s.bufferPool.Put(buf)
			continue
		}
		if msgType != MessageText && msgType != MessageBinary {
			s.bufferPool.Put(buf)
			continue
		}
		frame := s.framePool.New(buf[:n])
		frame.Meta = uint64(time.Since(base))
		topic, ok := s.opt.decoder.DecodeTopic(frame.Buf)
		if !ok {
			frame.Release()
			continue
		}
		frame.Topic = topic
		s.router.Route(frame)
	}
}

func (s *session) tryGrowReadBuffer(currentLen int, bucketCap int) bool {
	if s == nil || s.bufferPool == nil || currentLen <= 0 {
		return false
	}

	next := 0
	if bucketCap > currentLen {
		next = bucketCap
	} else {
		next = s.bufferPool.NextBucketSize(bucketCap)
		if next == 0 {
			return false
		}
	}

	if next > s.frameSize {
		s.frameSize = next
	}

	return true
}

func (s *session) sleepBackoff(ctx context.Context, attempt int) {
	wait := s.opt.Backoff.Next(attempt)
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

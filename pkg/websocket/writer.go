package websocket

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

var (
	// ErrNotConnected is returned when sending while the writer is disconnected.
	ErrNotConnected = errors.New("websocket: not connected")
	// ErrQueueFull is returned when the outbound queue cannot accept more frames.
	ErrQueueFull = errors.New("websocket: outbound queue full")
)

// outboundFrame represents a queued write payload.
type outboundFrame struct {
	// MsgType is the WebSocket message type for the payload.
	MsgType MessageType
	// Buf is the payload buffer to send.
	Buf  []byte
	pool *outboundPool
}

// Release returns the payload buffer and the frame to the pool.
func (f *outboundFrame) Release() {
	if f == nil || f.pool == nil {
		return
	}
	if f.Buf != nil && f.pool.buffers != nil {
		f.pool.buffers.Put(f.Buf)
	}
	f.MsgType = 0
	f.Buf = nil
	f.pool.pool.Put(f)
}

// outboundPool recycles outbound frames and buffers.
type outboundPool struct {
	buffers *bufferPool
	pool    sync.Pool
}

// newOutboundPool creates an OutboundPool.
func newOutboundPool(buffers *bufferPool) *outboundPool {
	op := &outboundPool{buffers: buffers}
	op.pool.New = func() any {
		return &outboundFrame{}
	}
	return op
}

// New creates an outbound frame backed by a pooled buffer.
func (p *outboundPool) New(msgType MessageType, buf []byte) *outboundFrame {
	frame := p.pool.Get().(*outboundFrame)
	frame.MsgType = msgType
	frame.Buf = buf
	frame.pool = p
	return frame
}

// writer provides a bounded outbound queue with pooling support.
type writer struct {
	pool      *outboundPool
	queue     chan *outboundFrame
	policy    OverflowPolicy
	connected atomic.Bool
}

// newWriter creates a Writer with a bounded queue.
func newWriter(pool *outboundPool, capacity int, policy OverflowPolicy) *writer {
	if capacity <= 0 {
		capacity = 1
	}
	return &writer{
		pool:   pool,
		queue:  make(chan *outboundFrame, capacity),
		policy: policy,
	}
}

// SetConnected toggles the writer connection state.
func (w *writer) SetConnected(connected bool) {
	w.connected.Store(connected)
}

// Acquire returns an outbound frame with a buffer of the requested size.
func (w *writer) Acquire(msgType MessageType, size int) *outboundFrame {
	if w.pool == nil || w.pool.buffers == nil {
		return &outboundFrame{MsgType: msgType, Buf: make([]byte, size)}
	}
	buf := w.pool.buffers.Get(size)
	return w.pool.New(msgType, buf[:size])
}

// Enqueue queues a frame for writing according to the overflow policy.
func (w *writer) Enqueue(frame *outboundFrame) bool {
	if frame == nil {
		return false
	}
	if !w.connected.Load() {
		return false
	}
	switch w.policy {
	case OverflowBlock:
		w.queue <- frame
		return true
	case OverflowDropOldest:
		for {
			select {
			case w.queue <- frame:
				return true
			default:
				select {
				case old := <-w.queue:
					if old != nil {
						old.Release()
					}
				default:
					return false
				}
			}
		}
	default:
		select {
		case w.queue <- frame:
			return true
		default:
			return false
		}
	}
}

// Send copies payload into a pooled buffer and enqueues it.
func (w *writer) Send(msgType MessageType, payload []byte) bool {
	if !w.connected.Load() {
		return false
	}
	frame := w.Acquire(msgType, len(payload))
	copy(frame.Buf, payload)
	if !w.Enqueue(frame) {
		frame.Release()
		return false
	}
	return true
}

// Next waits for the next outbound frame or context cancellation.
func (w *writer) Next(ctx context.Context) (*outboundFrame, bool) {
	select {
	case <-ctx.Done():
		return nil, false
	case frame := <-w.queue:
		return frame, frame != nil
	}
}

// Drain clears the queue and releases all frames.
func (w *writer) Drain() {
	for {
		select {
		case frame := <-w.queue:
			if frame != nil {
				frame.Release()
			}
		default:
			return
		}
	}
}

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
	ErrQueueFull    = errors.New("websocket: outbound queue full")
)

// OutboundFrame represents a queued write payload.
type OutboundFrame struct {
	// MsgType is the WebSocket message type for the payload.
	MsgType MessageType
	// Buf is the payload buffer to send.
	Buf     []byte
	pool    *OutboundPool
}

// Release returns the payload buffer and the frame to the pool.
func (f *OutboundFrame) Release() {
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

// OutboundPool recycles outbound frames and buffers.
type OutboundPool struct {
	buffers *BufferPool
	pool    sync.Pool
}

// NewOutboundPool creates an OutboundPool.
func NewOutboundPool(buffers *BufferPool) *OutboundPool {
	op := &OutboundPool{buffers: buffers}
	op.pool.New = func() any {
		return &OutboundFrame{}
	}
	return op
}

// New creates an outbound frame backed by a pooled buffer.
func (p *OutboundPool) New(msgType MessageType, buf []byte) *OutboundFrame {
	frame := p.pool.Get().(*OutboundFrame)
	frame.MsgType = msgType
	frame.Buf = buf
	frame.pool = p
	return frame
}

// Writer provides a bounded outbound queue with pooling support.
type Writer struct {
	pool      *OutboundPool
	queue     chan *OutboundFrame
	policy    OverflowPolicy
	connected atomic.Bool
}

// NewWriter creates a Writer with a bounded queue.
func NewWriter(pool *OutboundPool, capacity int, policy OverflowPolicy) *Writer {
	if capacity <= 0 {
		capacity = 1
	}
	return &Writer{
		pool:   pool,
		queue: make(chan *OutboundFrame, capacity),
		policy: policy,
	}
}

// SetConnected toggles the writer connection state.
func (w *Writer) SetConnected(connected bool) {
	w.connected.Store(connected)
}

// Acquire returns an outbound frame with a buffer of the requested size.
func (w *Writer) Acquire(msgType MessageType, size int) *OutboundFrame {
	if w.pool == nil || w.pool.buffers == nil {
		return &OutboundFrame{MsgType: msgType, Buf: make([]byte, size)}
	}
	buf := w.pool.buffers.Get(size)
	return w.pool.New(msgType, buf[:size])
}

// Enqueue queues a frame for writing according to the overflow policy.
func (w *Writer) Enqueue(frame *OutboundFrame) bool {
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
func (w *Writer) Send(msgType MessageType, payload []byte) bool {
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
func (w *Writer) Next(ctx context.Context) (*OutboundFrame, bool) {
	select {
	case <-ctx.Done():
		return nil, false
	case frame := <-w.queue:
		return frame, frame != nil
	}
}

// Drain clears the queue and releases all frames.
func (w *Writer) Drain() {
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

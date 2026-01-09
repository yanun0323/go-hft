package websocket

import (
	"sync"
	"sync/atomic"
)

// Frame represents a routed payload with ownership tracking.
type Frame struct {
	// Buf is the payload buffer for the frame.
	Buf []byte
	// Topic is the routed topic identifier.
	Topic TopicID
	// Meta carries optional metadata derived from the payload.
	Meta uint64

	ref  int32
	pool *framePool
}

// Retain increments the ref count for shared fanout.
func (f *Frame) Retain() {
	atomic.AddInt32(&f.ref, 1)
}

// Release decrements the ref count and recycles the frame when it reaches zero.
func (f *Frame) Release() {
	if f == nil {
		return
	}
	if atomic.AddInt32(&f.ref, -1) != 0 {
		return
	}
	if f.pool != nil {
		f.pool.recycle(f)
	}
}

// framePool recycles frames and their backing buffers.
type framePool struct {
	buffers *bufferPool
	pool    sync.Pool
}

// newFramePool creates a pool that recycles frames and buffers.
func newFramePool(buffers *bufferPool) *framePool {
	fp := &framePool{buffers: buffers}
	fp.pool.New = func() any {
		return &Frame{}
	}
	return fp
}

// New creates a Frame from a payload buffer.
func (p *framePool) New(buf []byte) *Frame {
	frame := p.pool.Get().(*Frame)
	frame.Buf = buf
	frame.Topic = 0
	frame.Meta = 0
	frame.pool = p
	atomic.StoreInt32(&frame.ref, 1)
	return frame
}

func (p *framePool) recycle(frame *Frame) {
	if frame.Buf != nil && p.buffers != nil {
		p.buffers.Put(frame.Buf)
	}
	frame.Buf = nil
	frame.Topic = 0
	frame.Meta = 0
	frame.pool = nil
	p.pool.Put(frame)
}

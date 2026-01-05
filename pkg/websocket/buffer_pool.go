package websocket

import (
	"sync"
)

// BufferPool provides bucketed []byte pooling by size class.
type BufferPool struct {
	pool *sync.Pool
}

// DefaultBufferPool returns a pool with common size buckets.
func DefaultBufferPool() *BufferPool {
	return NewBufferPool(64 << 10)
}

// NewBufferPool creates a bucketed pool using the provided sizes.
// Sizes are rounded to ascending unique values.
func NewBufferPool(size int64) *BufferPool {
	return &BufferPool{
		pool: &sync.Pool{
			New: func() any {
				return make([]byte, size)
			},
		},
	}
}

// Get returns a buffer with length size and capacity from the nearest bucket.
func (p *BufferPool) Get(size int) []byte {
	if size <= 0 {
		return nil
	}

	buf := p.pool.Get().([]byte)
	return buf[:size]
}

// Put returns a buffer to the pool when its capacity matches a bucket.
func (p *BufferPool) Put(buf []byte) {
	if buf == nil {
		return
	}

	buf = buf[:0]
	p.pool.Put(buf)
}

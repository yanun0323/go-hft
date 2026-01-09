package websocket

import (
	"sort"
	"sync"
)

var defaultBufferPoolBuckets = []int64{
	1 << 4,
	1 << 8,
	1 << 12,
	1 << 16,
	1 << 32,
	1<<63 - 1,
}

type bufferPool struct {
	sizes []int
	pools []*sync.Pool
}

func defaultBufferPool() *bufferPool {
	return newBufferPool(defaultBufferPoolBuckets...)
}

func newBufferPool(sizes ...int64) *bufferPool {
	cleaned := normalizeBucketSizes(sizes)
	if len(cleaned) == 0 {
		cleaned = normalizeBucketSizes(defaultBufferPoolBuckets)
	}
	pools := make([]*sync.Pool, len(cleaned))
	for i, size := range cleaned {
		bucketSize := size
		pools[i] = &sync.Pool{
			New: func() any {
				return make([]byte, bucketSize)
			},
		}
	}
	return &bufferPool{
		sizes: cleaned,
		pools: pools,
	}
}

func (p *bufferPool) InitSize() int {
	if p == nil || len(p.sizes) == 0 {
		return 0
	}

	return p.sizeAt(0)
}

func (p *bufferPool) Get(size int) []byte {
	if p == nil || size <= 0 || len(p.pools) == 0 {
		return nil
	}

	idx := p.bucketIndex(size)
	if idx < 0 {
		return make([]byte, size)
	}

	buf := p.poolAt(idx).Get().([]byte)
	if cap(buf) < size {
		return make([]byte, size)
	}
	return buf[:size]
}

func (p *bufferPool) Put(buf []byte) {
	if p == nil || buf == nil {
		return
	}
	size := cap(buf)
	idx := p.bucketIndex(size)
	if idx < 0 || p.sizeAt(idx) != size {
		return
	}
	buf = buf[:0]
	p.poolAt(idx).Put(buf)
}

func (p *bufferPool) EnsureSize(size int) int {
	if p == nil || size <= 0 {
		return 0
	}
	idx := p.bucketIndex(size)
	if idx < 0 {
		return 0
	}
	return p.sizeAt(idx)
}

func (p *bufferPool) bucketIndex(size int) int {
	idx := sort.SearchInts(p.sizes, size)
	if idx >= len(p.sizes) {
		return -1
	}
	return idx
}

func (p *bufferPool) NextBucketSize(size int) int {
	if p == nil || size <= 0 {
		return 0
	}
	idx := sort.SearchInts(p.sizes, size)
	if idx < len(p.sizes) && p.sizes[idx] == size {
		idx++
	}
	if idx >= len(p.sizes) {
		return 0
	}
	return p.sizes[idx]
}

func (p *bufferPool) poolAt(idx int) *sync.Pool {
	pool := p.pools[idx]
	return pool
}

func (p *bufferPool) sizeAt(idx int) int {
	size := p.sizes[idx]
	return size
}

func normalizeBucketSizes(sizes []int64) []int {
	if len(sizes) == 0 {
		return nil
	}
	maxInt := int(^uint(0) >> 1)
	filtered := make([]int, 0, len(sizes))
	for _, size := range sizes {
		if size <= 0 || size > int64(maxInt) {
			continue
		}
		filtered = append(filtered, int(size))
	}
	if len(filtered) == 0 {
		return nil
	}
	sort.Ints(filtered)
	out := filtered[:0]
	last := 0
	for i, size := range filtered {
		if i == 0 || size != last {
			out = append(out, size)
			last = size
		}
	}
	return out
}

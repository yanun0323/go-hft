package obs

import (
	"sync/atomic"
	"time"
)

// TraceGenerator creates monotonically increasing trace IDs.
type TraceGenerator struct {
	next uint64
}

// NewTraceGenerator returns a generator seeded with the given value.
func NewTraceGenerator(seed uint64) *TraceGenerator {
	if seed == 0 {
		seed = uint64(time.Now().UTC().UnixNano())
	}
	return &TraceGenerator{next: seed}
}

// Next returns the next trace ID.
func (g *TraceGenerator) Next() uint64 {
	if g == nil {
		return 0
	}
	return atomic.AddUint64(&g.next, 1)
}

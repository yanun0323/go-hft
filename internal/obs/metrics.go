package obs

import (
	"sync/atomic"
	"time"

	"main/internal/schema"
)

const (
	maxEventType  = int(schema.EventStrategyDecision)
	maxRiskReason = int(schema.RiskReasonPositionLimit)
)

// Metrics collects lightweight counters and latency stats.
type Metrics struct {
	eventCounts      [maxEventType + 1]uint64
	riskReasonCounts [maxRiskReason + 1]uint64
	queueDrops       uint64
	queueClosed      uint64

	eventLatency     LatencyStats
	orderFlowLatency LatencyStats
	riskEvalLatency  LatencyStats
}

// LatencyStats aggregates duration samples in nanoseconds.
type LatencyStats struct {
	count uint64
	sum   uint64
	min   uint64
	max   uint64
}

// LatencySnapshot is a point-in-time view of latency stats.
type LatencySnapshot struct {
	Count uint64
	Min   time.Duration
	Max   time.Duration
	Avg   time.Duration
}

// Snapshot captures the current metrics values.
type Snapshot struct {
	EventCounts      map[schema.EventType]uint64
	RiskReasonCounts map[schema.RiskReason]uint64
	QueueDrops       uint64
	QueueClosed      uint64
	EventLatency     LatencySnapshot
	OrderFlowLatency LatencySnapshot
	RiskEvalLatency  LatencySnapshot
}

// NewMetrics allocates a metrics container.
func NewMetrics() *Metrics {
	return &Metrics{}
}

// ObserveEvent increments counters and tracks event latency when timestamps are present.
func (m *Metrics) ObserveEvent(header schema.EventHeader) {
	if m == nil {
		return
	}
	idx := int(header.Type)
	if idx >= 0 && idx < len(m.eventCounts) {
		atomic.AddUint64(&m.eventCounts[idx], 1)
	}
	if header.TsEvent > 0 && header.TsRecv > 0 {
		delta := header.TsRecv - header.TsEvent
		if delta >= 0 {
			m.eventLatency.Observe(time.Duration(delta))
		}
	}
}

// IncRiskReason increments the risk reason counter.
func (m *Metrics) IncRiskReason(reason schema.RiskReason) {
	if m == nil {
		return
	}
	idx := int(reason)
	if idx >= 0 && idx < len(m.riskReasonCounts) {
		atomic.AddUint64(&m.riskReasonCounts[idx], 1)
	}
}

// IncQueueDrop records a queue drop.
func (m *Metrics) IncQueueDrop() {
	if m == nil {
		return
	}
	atomic.AddUint64(&m.queueDrops, 1)
}

// IncQueueClosed records a closed-queue publish attempt.
func (m *Metrics) IncQueueClosed() {
	if m == nil {
		return
	}
	atomic.AddUint64(&m.queueClosed, 1)
}

// ObserveOrderFlow measures end-to-end order flow latency.
func (m *Metrics) ObserveOrderFlow(d time.Duration) {
	if m == nil {
		return
	}
	m.orderFlowLatency.Observe(d)
}

// ObserveRiskEval measures risk evaluation latency.
func (m *Metrics) ObserveRiskEval(d time.Duration) {
	if m == nil {
		return
	}
	m.riskEvalLatency.Observe(d)
}

// Snapshot returns a copy of the current metrics values.
func (m *Metrics) Snapshot() Snapshot {
	if m == nil {
		return Snapshot{}
	}
	eventCounts := make(map[schema.EventType]uint64)
	for i := range m.eventCounts {
		if v := atomic.LoadUint64(&m.eventCounts[i]); v > 0 {
			eventCounts[schema.EventType(i)] = v
		}
	}
	riskCounts := make(map[schema.RiskReason]uint64)
	for i := range m.riskReasonCounts {
		if v := atomic.LoadUint64(&m.riskReasonCounts[i]); v > 0 {
			riskCounts[schema.RiskReason(i)] = v
		}
	}
	return Snapshot{
		EventCounts:      eventCounts,
		RiskReasonCounts: riskCounts,
		QueueDrops:       atomic.LoadUint64(&m.queueDrops),
		QueueClosed:      atomic.LoadUint64(&m.queueClosed),
		EventLatency:     m.eventLatency.Snapshot(),
		OrderFlowLatency: m.orderFlowLatency.Snapshot(),
		RiskEvalLatency:  m.riskEvalLatency.Snapshot(),
	}
}

// Observe records a duration sample.
func (l *LatencyStats) Observe(d time.Duration) {
	if d < 0 {
		return
	}
	nanos := uint64(d)
	atomic.AddUint64(&l.count, 1)
	atomic.AddUint64(&l.sum, nanos)

	for {
		min := atomic.LoadUint64(&l.min)
		if min != 0 && nanos >= min {
			break
		}
		if atomic.CompareAndSwapUint64(&l.min, min, nanos) {
			break
		}
	}

	for {
		max := atomic.LoadUint64(&l.max)
		if nanos <= max {
			break
		}
		if atomic.CompareAndSwapUint64(&l.max, max, nanos) {
			break
		}
	}
}

// Snapshot returns the aggregated latency stats.
func (l *LatencyStats) Snapshot() LatencySnapshot {
	count := atomic.LoadUint64(&l.count)
	if count == 0 {
		return LatencySnapshot{}
	}
	sum := atomic.LoadUint64(&l.sum)
	min := atomic.LoadUint64(&l.min)
	max := atomic.LoadUint64(&l.max)
	return LatencySnapshot{
		Count: count,
		Min:   time.Duration(min),
		Max:   time.Duration(max),
		Avg:   time.Duration(sum / count),
	}
}

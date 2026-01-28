package metric

import (
	"context"
	"log"
	"runtime"
	"strconv"
	"time"
)

type RuntimeMemoryMetric struct {
	buf        [2048]byte
	prev, curr runtime.MemStats
	prevAt     time.Time
	currAt     time.Time
}

func (m *RuntimeMemoryMetric) RunReportSchedule(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.Snapshot()
			m.Print()
		}
	}
}

func (m *RuntimeMemoryMetric) Snapshot() {
	m.prev, m.curr = m.curr, m.prev
	m.prevAt = m.currAt
	m.currAt = time.Now()

	runtime.ReadMemStats(&m.curr)

	if m.prevAt.IsZero() {
		m.prevAt = m.currAt
	}
}

func (m RuntimeMemoryMetric) Get() (prev, curr runtime.MemStats) {
	return m.prev, m.curr
}

func (m *RuntimeMemoryMetric) Print() {
	line := m.buf[:0]

	// --- TIME ---
	{
		line = append(line, "[TIME] "...)
		line = strconv.AppendInt(line, m.currAt.Unix(), 10)
		line = append(line, "  "...)
	}

	dt := m.currAt.Sub(m.prevAt).Seconds()
	if dt <= 0 {
		dt = 1
	}

	// --- HEAP ---
	{
		line = append(line, "[HEAP] "...)

		line = append(line, "alc_grow="...)
		b, unit := bytesCarry(m.curr.TotalAlloc - m.prev.TotalAlloc)
		line = strconv.AppendUint(line, b, 10)
		line = append(line, unit...)

		line = append(line, "\t"...)
		line = append(line, "alc="...)
		b, unit = bytesCarry(m.curr.HeapAlloc)
		line = strconv.AppendUint(line, b, 10)
		line = append(line, unit...)

		line = append(line, "\t"...)
		line = append(line, "inuse="...)
		b, unit = bytesCarry(m.curr.HeapInuse)
		line = strconv.AppendUint(line, b, 10)
		line = append(line, unit...)

		line = append(line, "\t"...)
		line = append(line, "object="...)
		line = strconv.AppendUint(line, m.curr.HeapObjects, 10)

		line = append(line, "\t"...)
		line = append(line, "alc_rate="...)
		rate := float64(m.curr.TotalAlloc-m.prev.TotalAlloc) / dt
		rb, runit := bytesCarryFloat(rate)
		line = strconv.AppendFloat(line, rb, 'f', 2, 64)
		line = append(line, runit...)
		line = append(line, "/s"...)
	}

	// --- GC ---
	{
		gcTimes := uint64(m.curr.NumGC - m.prev.NumGC)
		stwMs := float64(m.curr.PauseTotalNs-m.prev.PauseTotalNs) / 1_000_000.0

		line = append(line, "\t"...)
		line = append(line, "[GC] "...)

		line = append(line, "times="...)
		line = strconv.AppendUint(line, gcTimes, 10)

		line = append(line, "\t"...)
		line = append(line, "stw="...)
		line = strconv.AppendFloat(line, stwMs, 'f', 4, 64)
		line = append(line, "ms"...)

		line = append(line, "\t"...)
		line = append(line, "last_gc_unix="...)
		lastUnix := uint64(0)
		if m.curr.LastGC != 0 {
			lastUnix = uint64(m.curr.LastGC / 1_000_000_000)
		}
		line = strconv.AppendUint(line, lastUnix, 10)

		line = append(line, "\t"...)
		line = append(line, "next_gc="...)
		b, unit := bytesCarry(m.curr.NextGC)
		line = strconv.AppendUint(line, b, 10)
		line = append(line, unit...)

		line = append(line, "\t"...)
		line = append(line, "gc_cpu="...)
		line = strconv.AppendFloat(line, m.curr.GCCPUFraction, 'f', 6, 64)

		line = append(line, "\t"...)
		line = append(line, "mallocs="...)
		line = strconv.AppendUint(line, m.curr.Mallocs-m.prev.Mallocs, 10)

		line = append(line, "\t"...)
		line = append(line, "frees="...)
		line = strconv.AppendUint(line, m.curr.Frees-m.prev.Frees, 10)

		line = append(line, "\t"...)
		line = append(line, "live="...)
		live := int64(m.curr.Mallocs) - int64(m.curr.Frees)
		line = strconv.AppendInt(line, live, 10)
	}

	line = append(line, '\n')
	_, _ = log.Writer().Write(line)
}

const carryThreshold = 1 << 15

func bytesCarry(value uint64) (uint64, string) {
	if value < carryThreshold {
		return value, " B"
	}
	value >>= 10
	if value < carryThreshold {
		return value, " KB"
	}
	value >>= 10
	if value < carryThreshold {
		return value, " MB"
	}
	return value >> 10, " GB"
}

func bytesCarryFloat(value float64) (float64, string) {
	if value < float64(carryThreshold) {
		return value, " B"
	}
	value /= 1024
	if value < float64(carryThreshold) {
		return value, " KB"
	}
	value /= 1024
	if value < float64(carryThreshold) {
		return value, " MB"
	}
	return value / 1024, " GB"
}

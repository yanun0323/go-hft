package websocket

import (
	"math/rand"
	"time"
)

// DefaultBackoff provides conservative reconnect defaults.
func DefaultBackoff() Backoff {
	return Backoff{
		Min:    250 * time.Millisecond,
		Max:    5 * time.Second,
		Factor: 2.0,
		Jitter: 0.2,
	}
}

// Next returns the next backoff duration for the given attempt (1-based).
func (b Backoff) Next(attempt int) time.Duration {
	if attempt <= 0 {
		attempt = 1
	}
	min := b.Min
	if min <= 0 {
		min = 100 * time.Millisecond
	}
	max := b.Max
	if max <= 0 {
		max = 5 * time.Second
	}
	factor := b.Factor
	if factor <= 1 {
		factor = 2.0
	}

	wait := min
	for i := 1; i < attempt; i++ {
		next := time.Duration(float64(wait) * factor)
		if next > max {
			wait = max
			break
		}
		wait = next
	}

	if b.Jitter <= 0 {
		return wait
	}
	jitter := b.Jitter
	if jitter > 1 {
		jitter = 1
	}
	delta := float64(wait) * jitter
	return wait - time.Duration(delta) + time.Duration(rand.Float64()*2*delta)
}

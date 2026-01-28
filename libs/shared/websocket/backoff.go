package websocket

import (
	"math/rand"
	"time"
)

const (
	defaultBackoffMin    = 250 * time.Millisecond
	defaultBackoffMax    = 5 * time.Second
	defaultBackoffFactor = 2.0
	defaultBackoffJitter = 0.2

	fallbackBackoffMin    = 100 * time.Millisecond
	fallbackBackoffMax    = 5 * time.Second
	fallbackBackoffFactor = 2.0

	minAttempt            = 1
	minBackoffFactor      = 1.0
	maxBackoffJitter      = 1.0
	jitterRangeMultiplier = 2.0
)

// DefaultBackoff provides conservative reconnect defaults.
func DefaultBackoff() Backoff {
	return Backoff{
		Min:    defaultBackoffMin,
		Max:    defaultBackoffMax,
		Factor: defaultBackoffFactor,
		Jitter: defaultBackoffJitter,
	}
}

// Next returns the next backoff duration for the given attempt (1-based).
func (b Backoff) Next(attempt int) time.Duration {
	if attempt <= 0 {
		attempt = minAttempt
	}
	min := b.Min
	if min <= 0 {
		min = fallbackBackoffMin
	}
	max := b.Max
	if max <= 0 {
		max = fallbackBackoffMax
	}
	factor := b.Factor
	if factor <= minBackoffFactor {
		factor = fallbackBackoffFactor
	}

	wait := min
	for i := minAttempt; i < attempt; i++ {
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
	if jitter > maxBackoffJitter {
		jitter = maxBackoffJitter
	}
	delta := float64(wait) * jitter
	return wait - time.Duration(delta) + time.Duration(rand.Float64()*jitterRangeMultiplier*delta)
}

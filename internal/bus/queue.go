package bus

import (
	"context"
	"errors"
	"sync/atomic"

	"main/internal/schema"
)

var (
	ErrQueueFull   = errors.New("event queue full")
	ErrQueueClosed = errors.New("event queue closed")
)

// Event is the unit passed through the in-memory bus.
type Event struct {
	Header  schema.EventHeader
	Payload []byte
}

// Queue is a bounded, non-blocking event queue.
type Queue struct {
	ch     chan Event
	closed uint32
}

// NewQueue allocates a queue with the given capacity.
func NewQueue(capacity int) *Queue {
	if capacity <= 0 {
		capacity = 1
	}
	return &Queue{ch: make(chan Event, capacity)}
}

// TryPublish enqueues an event without blocking.
func (q *Queue) TryPublish(e Event) error {
	if atomic.LoadUint32(&q.closed) != 0 {
		return ErrQueueClosed
	}
	select {
	case q.ch <- e:
		return nil
	default:
		return ErrQueueFull
	}
}

// Close stops the queue from accepting new events.
func (q *Queue) Close() {
	if atomic.CompareAndSwapUint32(&q.closed, 0, 1) {
		close(q.ch)
	}
}

// Run consumes events until the context is done or the queue is closed.
func (q *Queue) Run(ctx context.Context, handler func(Event)) {
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-q.ch:
			if !ok {
				return
			}
			handler(e)
		}
	}
}

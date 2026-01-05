package websocket

import "sync"

// Consumer receives routed frames from a topic queue.
type Consumer struct {
	queue *FrameQueue
}

// NewConsumer creates a consumer with a bounded queue.
func NewConsumer(capacity int, policy OverflowPolicy) *Consumer {
	return &Consumer{
		queue: NewFrameQueue(capacity, policy),
	}
}

// Next blocks until a frame is available or the queue is closed.
func (c *Consumer) Next() (*Frame, bool) {
	if c == nil || c.queue == nil {
		return nil, false
	}
	return c.queue.Pop()
}

// Close closes the consumer queue and releases pending frames.
func (c *Consumer) Close() {
	if c == nil || c.queue == nil {
		return
	}
	c.queue.Close()
}

// Len returns the number of queued frames.
func (c *Consumer) Len() int {
	if c == nil || c.queue == nil {
		return 0
	}
	return c.queue.Len()
}

func (c *Consumer) enqueue(frame *Frame) bool {
	if c == nil || c.queue == nil {
		return false
	}
	return c.queue.Push(frame)
}

// FrameQueue is a bounded ring buffer for frames.
type FrameQueue struct {
	mu       sync.Mutex
	notEmpty *sync.Cond
	notFull  *sync.Cond
	buf      []*Frame
	head     int
	tail     int
	size     int
	closed   bool
	policy   OverflowPolicy
}

// NewFrameQueue creates a bounded ring buffer.
func NewFrameQueue(capacity int, policy OverflowPolicy) *FrameQueue {
	if capacity <= 0 {
		capacity = 1
	}
	q := &FrameQueue{
		buf:    make([]*Frame, capacity),
		policy: policy,
	}
	q.notEmpty = sync.NewCond(&q.mu)
	q.notFull = sync.NewCond(&q.mu)
	return q
}

// Push enqueues a frame according to the overflow policy.
func (q *FrameQueue) Push(frame *Frame) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	for {
		if q.closed {
			return false
		}
		if q.size < len(q.buf) {
			q.buf[q.tail] = frame
			q.tail = (q.tail + 1) % len(q.buf)
			q.size++
			q.notEmpty.Signal()
			return true
		}
		switch q.policy {
		case OverflowBlock:
			q.notFull.Wait()
		case OverflowDropOldest:
			old := q.buf[q.head]
			q.buf[q.head] = nil
			q.head = (q.head + 1) % len(q.buf)
			q.size--
			if old != nil {
				old.Release()
			}
		default:
			return false
		}
	}
}

// Pop dequeues the next frame, blocking until available or closed.
func (q *FrameQueue) Pop() (*Frame, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for {
		if q.size > 0 {
			frame := q.buf[q.head]
			q.buf[q.head] = nil
			q.head = (q.head + 1) % len(q.buf)
			q.size--
			q.notFull.Signal()
			return frame, true
		}
		if q.closed {
			return nil, false
		}
		q.notEmpty.Wait()
	}
}

// Close closes the queue and releases pending frames.
func (q *FrameQueue) Close() {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return
	}
	q.closed = true
	for i := 0; i < q.size; i++ {
		idx := (q.head + i) % len(q.buf)
		if q.buf[idx] != nil {
			q.buf[idx].Release()
			q.buf[idx] = nil
		}
	}
	q.size = 0
	q.head = 0
	q.tail = 0
	q.notEmpty.Broadcast()
	q.notFull.Broadcast()
	q.mu.Unlock()
}

// Len returns the number of queued frames.
func (q *FrameQueue) Len() int {
	q.mu.Lock()
	size := q.size
	q.mu.Unlock()
	return size
}

package websocket

import "sync"

// Subscriptions tracks desired and active topic states.
type Subscriptions struct {
	mu      sync.Mutex
	desired map[TopicID]int
	active  map[TopicID]struct{}
}

// NewSubscriptions creates a subscription tracker.
func NewSubscriptions() *Subscriptions {
	return &Subscriptions{
		desired: make(map[TopicID]int),
		active:  make(map[TopicID]struct{}),
	}
}

// Inc increments the desired refcount for a topic.
// Returns true if this is the first reference.
func (s *Subscriptions) Inc(topic TopicID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := s.desired[topic]
	count++
	s.desired[topic] = count
	return count == 1
}

// Dec decrements the desired refcount for a topic.
// Returns true if the topic is removed.
func (s *Subscriptions) Dec(topic TopicID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := s.desired[topic]
	if count <= 1 {
		delete(s.desired, topic)
		delete(s.active, topic)
		return count > 0
	}
	s.desired[topic] = count - 1
	return false
}

// MarkActive marks a topic as active.
func (s *Subscriptions) MarkActive(topic TopicID) {
	s.mu.Lock()
	s.active[topic] = struct{}{}
	s.mu.Unlock()
}

// ClearActive clears all active topics.
func (s *Subscriptions) ClearActive() {
	s.mu.Lock()
	for topic := range s.active {
		delete(s.active, topic)
	}
	s.mu.Unlock()
}

// Desired fills dst with desired topics and returns it.
func (s *Subscriptions) Desired(dst []TopicID) []TopicID {
	s.mu.Lock()
	if dst == nil {
		dst = make([]TopicID, 0, len(s.desired))
	} else {
		dst = dst[:0]
	}
	for topic := range s.desired {
		dst = append(dst, topic)
	}
	s.mu.Unlock()
	return dst
}

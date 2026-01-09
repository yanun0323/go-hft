package websocket

import "sync"

// DesiredSubscription represents a desired topic subscription.
type DesiredSubscription struct {
	Topic TopicID
	ID    SubscribeID
}

// subscriptions tracks desired and active topic states per session.
type subscriptions struct {
	mu      sync.Mutex
	desired map[TopicID]SubscribeID
	active  map[TopicID]struct{}
}

// newSubscriptions creates a subscription tracker.
func newSubscriptions() *subscriptions {
	return &subscriptions{
		desired: make(map[TopicID]SubscribeID),
		active:  make(map[TopicID]struct{}),
	}
}

// Add registers a desired topic subscription.
// Returns true if the topic was newly added.
func (s *subscriptions) Add(topic TopicID, id SubscribeID) bool {
	s.mu.Lock()
	_, exists := s.desired[topic]
	if !exists {
		s.desired[topic] = id
	}
	s.mu.Unlock()
	return !exists
}

// Remove deletes a desired topic subscription.
// Returns the subscribe id and true if removed.
func (s *subscriptions) Remove(topic TopicID) (SubscribeID, bool) {
	s.mu.Lock()
	id, ok := s.desired[topic]
	if ok {
		delete(s.desired, topic)
		delete(s.active, topic)
	}
	s.mu.Unlock()
	return id, ok
}

// Get returns the subscribe id for a topic.
func (s *subscriptions) Get(topic TopicID) (SubscribeID, bool) {
	s.mu.Lock()
	id, ok := s.desired[topic]
	s.mu.Unlock()
	return id, ok
}

// MarkActive marks a topic as active.
func (s *subscriptions) MarkActive(topic TopicID) {
	s.mu.Lock()
	s.active[topic] = struct{}{}
	s.mu.Unlock()
}

// ClearActive clears all active topics.
func (s *subscriptions) ClearActive() {
	s.mu.Lock()
	for topic := range s.active {
		delete(s.active, topic)
	}
	s.mu.Unlock()
}

// Desired fills dst with desired subscriptions and returns it.
func (s *subscriptions) Desired(dst []DesiredSubscription) []DesiredSubscription {
	s.mu.Lock()
	if dst == nil {
		dst = make([]DesiredSubscription, 0, len(s.desired))
	} else {
		dst = dst[:0]
	}
	for topic, id := range s.desired {
		dst = append(dst, DesiredSubscription{Topic: topic, ID: id})
	}
	s.mu.Unlock()
	return dst
}

// Count returns the number of desired topics.
func (s *subscriptions) Count() int {
	s.mu.Lock()
	count := len(s.desired)
	s.mu.Unlock()
	return count
}

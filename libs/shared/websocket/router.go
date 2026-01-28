package websocket

import (
	"sync"
	"sync/atomic"
)

// router delivers frames to consumers based on topic.
type router struct {
	mu        sync.RWMutex
	topics    map[TopicID][]*Consumer
	fanOut    FanOutMode
	framePool *framePool
}

// newRouter creates a router with the specified fan out mode.
func newRouter(fanOut FanOutMode, framePool *framePool) *router {
	return &router{
		topics:    make(map[TopicID][]*Consumer),
		fanOut:    fanOut,
		framePool: framePool,
	}
}

// AddConsumer registers a consumer for a topic.
func (r *router) AddConsumer(topic TopicID, consumer *Consumer) {
	if r == nil || consumer == nil {
		return
	}
	r.addConsumer(topic, consumer)
}

// RemoveConsumer unregisters a consumer from a topic.
func (r *router) RemoveConsumer(topic TopicID, consumer *Consumer) {
	if r == nil || consumer == nil {
		return
	}
	r.removeConsumer(topic, consumer)
}

// RemoveTopic removes all consumers for a topic.
func (r *router) RemoveTopic(topic TopicID) {
	if r == nil {
		return
	}
	r.mu.Lock()
	delete(r.topics, topic)
	r.mu.Unlock()
}

// Route dispatches a frame to all consumers of its topic.
func (r *router) Route(frame *Frame) {
	if r == nil || frame == nil {
		return
	}
	r.mu.RLock()
	consumers := r.topics[frame.Topic]
	if len(consumers) == 0 {
		r.mu.RUnlock()
		frame.Release()
		return
	}
	if r.fanOut == FanOutCopy {
		r.routeCopy(frame, consumers)
		r.mu.RUnlock()
		frame.Release()
		return
	}
	r.routeShared(frame, consumers)
	r.mu.RUnlock()
}

func (r *router) routeShared(frame *Frame, consumers []*Consumer) {
	atomic.StoreInt32(&frame.ref, int32(len(consumers)))
	for _, consumer := range consumers {
		if consumer == nil || !consumer.enqueue(frame) {
			frame.Release()
		}
	}
}

func (r *router) routeCopy(frame *Frame, consumers []*Consumer) {
	for _, consumer := range consumers {
		if consumer == nil {
			continue
		}
		copied := r.copyFrame(frame)
		if copied == nil {
			continue
		}
		if !consumer.enqueue(copied) {
			copied.Release()
		}
	}
}

func (r *router) copyFrame(frame *Frame) *Frame {
	if r.framePool == nil || r.framePool.buffers == nil {
		return nil
	}
	buf := r.framePool.buffers.Get(len(frame.Buf))
	copy(buf, frame.Buf)
	copyFrame := r.framePool.New(buf[:len(frame.Buf)])
	copyFrame.Topic = frame.Topic
	copyFrame.Meta = frame.Meta
	return copyFrame
}

func (r *router) addConsumer(topic TopicID, consumer *Consumer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := r.topics[topic]
	for _, existing := range list {
		if existing == consumer {
			return
		}
	}
	r.topics[topic] = append(list, consumer)
}

func (r *router) removeConsumer(topic TopicID, consumer *Consumer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := r.topics[topic]
	for i, existing := range list {
		if existing == consumer {
			list[i] = list[len(list)-1]
			list = list[:len(list)-1]
			if len(list) == 0 {
				delete(r.topics, topic)
			} else {
				r.topics[topic] = list
			}
			return
		}
	}
}

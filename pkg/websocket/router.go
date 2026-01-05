package websocket

import (
	"sync"
	"sync/atomic"
)

// Router delivers frames to consumers based on topic.
type Router struct {
	mu        sync.RWMutex
	topics    map[TopicID][]*Consumer
	fanOut    FanoutMode
	framePool *FramePool
}

// NewRouter creates a router with the specified fan out mode.
func NewRouter(fanOut FanoutMode, framePool *FramePool) *Router {
	return &Router{
		topics:    make(map[TopicID][]*Consumer),
		fanOut:    fanOut,
		framePool: framePool,
	}
}

// AddConsumer registers a consumer for a topic.
func (r *Router) AddConsumer(topic TopicID, consumer *Consumer) {
	if r == nil || consumer == nil {
		return
	}
	r.addConsumer(topic, consumer)
}

// RemoveConsumer unregisters a consumer from a topic.
func (r *Router) RemoveConsumer(topic TopicID, consumer *Consumer) {
	if r == nil || consumer == nil {
		return
	}
	r.removeConsumer(topic, consumer)
}

// Route dispatches a frame to all consumers of its topic.
func (r *Router) Route(frame *Frame) {
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
	if r.fanOut == FanoutCopy {
		r.routeCopy(frame, consumers)
		r.mu.RUnlock()
		frame.Release()
		return
	}
	r.routeShared(frame, consumers)
	r.mu.RUnlock()
}

func (r *Router) routeShared(frame *Frame, consumers []*Consumer) {
	atomic.StoreInt32(&frame.ref, int32(len(consumers)))
	for _, consumer := range consumers {
		if consumer == nil || !consumer.enqueue(frame) {
			frame.Release()
		}
	}
}

func (r *Router) routeCopy(frame *Frame, consumers []*Consumer) {
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

func (r *Router) copyFrame(frame *Frame) *Frame {
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

func (r *Router) addConsumer(topic TopicID, consumer *Consumer) {
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

func (r *Router) removeConsumer(topic TopicID, consumer *Consumer) {
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

package websocket

import "time"

// TopicID is the numeric identifier for a topic.
type TopicID uint32

// ConnectionID is the numeric identifier for the websocket connection of the topic.
type ConnectionID uint64

// MessageType represents a WebSocket message type.
// Values match RFC 6455 opcodes where applicable.
type MessageType uint8

const (
	// MessageText is a text data frame.
	MessageText MessageType = 1
	// MessageBinary is a binary data frame.
	MessageBinary MessageType = 2
	// MessageClose is a close control frame.
	MessageClose MessageType = 8
	// MessagePing is a ping control frame.
	MessagePing MessageType = 9
	// MessagePong is a pong control frame.
	MessagePong MessageType = 10
)

// CloseCode is a WebSocket close code.
type CloseCode uint16

const (
	// CloseNormal indicates a normal closure.
	CloseNormal CloseCode = 1000
)

// FanOutMode controls how frames are delivered to multiple consumers.
type FanOutMode uint8

const (
	// FanOutCopy copies the payload for each consumer.
	FanOutCopy FanOutMode = iota
	// FanOutShared shares the same frame across all consumers using ref counting.
	FanOutShared
)

// OverflowPolicy defines queue behavior when full.
type OverflowPolicy uint8

const (
	// OverflowBlock blocks until space is available.
	OverflowBlock OverflowPolicy = iota
	// OverflowDropNewest drops the incoming item if the queue is full.
	OverflowDropNewest
	// OverflowDropOldest drops the oldest item to make room.
	OverflowDropOldest
)

// Backoff defines reconnect backoff behavior.
type Backoff struct {
	// Min is the minimum backoff duration.
	Min time.Duration
	// Max is the maximum backoff duration.
	Max time.Duration
	// Factor multiplies the delay for each retry attempt.
	Factor float64
	// Jitter adds randomization as a fraction of the delay (0-1).
	Jitter float64
}

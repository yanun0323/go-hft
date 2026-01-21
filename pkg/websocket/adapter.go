package websocket

import "context"

// Conn is a minimal interface for a WebSocket connection.
// Implementations should read into the provided dst buffer.
type Conn interface {
	Read(ctx context.Context, dst []byte) (n int, msgType MessageType, err error)
	Write(ctx context.Context, msgType MessageType, payload []byte) error
	Close(code CloseCode, reason string) error
}

// Dialer creates new connections.
type Dialer interface {
	Dial(ctx context.Context) (Conn, error)
}

// TopicDecoder extracts the topic ID from a payload.
type TopicDecoder interface {
	DecodeTopic(payload []byte) (TopicID, bool)
}

// ControlEncoder builds subscribe and unsubscribe payloads.
// Implementations should write into dst and return a slice backed by dst.
type ControlEncoder interface {
	EncodeSubscribe(dst []byte, subscribeID ConnectionID, topic TopicID) (MessageType, []byte, error)
	EncodeUnsubscribe(dst []byte, subscribeID ConnectionID, topic TopicID) (MessageType, []byte, error)
}

// MetaFunc derives a frame metadata value from the payload.
type MetaFunc func(payload []byte) uint64

// Writer enqueues outbound frames and returns false when the payload is not accepted.
type Writer interface {
	Send(msgType MessageType, payload []byte) bool
}

package client

import (
	"main/pkg/exception"
	"main/pkg/scanner"
	"main/pkg/websocket"
)

type TopicSpec struct {
	ID          websocket.TopicID
	SymbolUpper []byte
	StreamName  []byte
	Label       []byte
}

type decoder struct {
	streamLookup map[uint64]websocket.TopicID
	symbolLookup map[uint64]websocket.TopicID
}

func NewDecoder(topics []TopicSpec) *decoder {
	streamLookup := make(map[uint64]websocket.TopicID, len(topics))
	symbolLookup := make(map[uint64]websocket.TopicID, len(topics))

	for _, topic := range topics {
		streamLookup[hashBytes(topic.StreamName)] = topic.ID
		symbolLookup[hashBytes(topic.SymbolUpper)] = topic.ID
	}

	return &decoder{
		streamLookup: streamLookup,
		symbolLookup: symbolLookup,
	}
}

func (p *decoder) DecodeTopic(payload []byte) (websocket.TopicID, bool) {
	if stream, ok := scanner.ScanStringField(payload, keyEvent); ok {
		if id, exists := p.streamLookup[hashBytes(stream)]; exists {
			return id, true
		}
	}
	if symbol, ok := scanner.ScanStringField(payload, keySymbol); ok {
		if id, exists := p.symbolLookup[hashBytes(symbol)]; exists {
			return id, true
		}
	}
	return 0, false
}

type encoder struct {
	streamByID map[websocket.TopicID][]byte
}

func NewEncoder(topics []TopicSpec) *encoder {
	streamByID := make(map[websocket.TopicID][]byte, len(topics))
	for _, topic := range topics {
		streamByID[topic.ID] = topic.StreamName
	}
	return &encoder{streamByID: streamByID}
}

func (e *encoder) EncodeSubscribe(dst []byte, subscribeID websocket.SubscribeID, topic websocket.TopicID) (websocket.MessageType, []byte, error) {
	stream, ok := e.streamByID[topic]
	if !ok {
		return 0, nil, exception.ErrWebSocketProtocol
	}
	dst = append(dst, `{"method":"SUBSCRIBE","params":["`...)
	dst = append(dst, stream...)
	dst = append(dst, `"],"id":`...)
	dst = appendUint(dst, uint64(subscribeID))
	dst = append(dst, '}')
	return websocket.MessageText, dst, nil
}

func (e *encoder) EncodeUnsubscribe(dst []byte, subscribeID websocket.SubscribeID, topic websocket.TopicID) (websocket.MessageType, []byte, error) {
	stream, ok := e.streamByID[topic]
	if !ok {
		return 0, nil, exception.ErrWebSocketProtocol
	}
	dst = append(dst, `{"method":"UNSUBSCRIBE","params":["`...)
	dst = append(dst, stream...)
	dst = append(dst, `"],"id":`...)
	dst = appendUint(dst, uint64(subscribeID))
	dst = append(dst, '}')
	return websocket.MessageText, dst, nil
}

var (
	keyEvent  = []byte(`"e"`)
	keySymbol = []byte(`"s"`)
)

func hashBytes(data []byte) uint64 {
	const offset64 = 14695981039346656037
	const prime64 = 1099511628211
	var hash uint64 = offset64
	for i := range data {
		hash ^= uint64(data[i])
		hash *= prime64
	}
	return hash
}

func appendUint(dst []byte, v uint64) []byte {
	if v == 0 {
		return append(dst, '0')
	}

	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}

	return append(dst, buf[i:]...)
}

package client

import (
	"errors"
	"main/pkg/exception"
	"main/pkg/websocket"
	"sync"
)

var (
	errEmptyTopic = errors.New("binance: empty topic")
)

var (
	keyID          = []byte(`"id"`)
	keyResult      = []byte(`"result"`)
	keyStatus      = []byte(`"status"`)
	keySuccessTrue = []byte(`"success":true`)
	successValue   = []byte("success")
)

// DynamicCodec supports runtime topic registration for Binance streams.
type DynamicCodec struct {
	mu           sync.RWMutex
	streamByID   map[websocket.TopicID][]byte
	streamLookup map[uint64]websocket.TopicID
	symbolLookup map[uint64]websocket.TopicID
	authTopicID  websocket.TopicID
	authReqID    uint64
	authAPIKey   []byte
	authEnabled  bool
}

// NewDynamicCodec creates a codec that can register topics on demand.
func NewDynamicCodec() *DynamicCodec {
	return &DynamicCodec{
		streamByID:   make(map[websocket.TopicID][]byte),
		streamLookup: make(map[uint64]websocket.TopicID),
		symbolLookup: make(map[uint64]websocket.TopicID),
	}
}

// RegisterAuth configures auth payload metadata for this connection.
func (c *DynamicCodec) RegisterAuth(topicID websocket.TopicID, apiKey string, reqID uint64) error {
	if c == nil {
		return errEmptyTopic
	}
	if apiKey == "" {
		return errEmptyTopic
	}
	c.mu.Lock()
	c.authTopicID = topicID
	c.authReqID = reqID
	c.authAPIKey = []byte(apiKey)
	c.authEnabled = true
	c.mu.Unlock()
	return nil
}

// ClearAuth removes auth payload metadata.
func (c *DynamicCodec) ClearAuth() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.authTopicID = 0
	c.authReqID = 0
	c.authAPIKey = nil
	c.authEnabled = false
	c.mu.Unlock()
}

// Register adds a topic mapping for a stream name.
func (c *DynamicCodec) Register(topicID websocket.TopicID, topic string) error {
	if c == nil {
		return errEmptyTopic
	}
	if topic == "" {
		return errEmptyTopic
	}
	stream := []byte(topic)
	symbol := parseSymbolUpper(stream)

	c.mu.Lock()
	c.streamByID[topicID] = stream
	c.streamLookup[hashBytes(stream)] = topicID
	if len(symbol) > 0 {
		c.symbolLookup[hashBytes(symbol)] = topicID
	}
	c.mu.Unlock()
	return nil
}

// Unregister removes a topic mapping.
func (c *DynamicCodec) Unregister(topicID websocket.TopicID) {
	if c == nil {
		return
	}
	c.mu.Lock()
	stream, ok := c.streamByID[topicID]
	if ok {
		delete(c.streamByID, topicID)
		delete(c.streamLookup, hashBytes(stream))
		symbol := parseSymbolUpper(stream)
		if len(symbol) > 0 {
			delete(c.symbolLookup, hashBytes(symbol))
		}
	}
	c.mu.Unlock()
}

// DecodeTopic extracts the topic id from a payload.
func (c *DynamicCodec) DecodeTopic(payload []byte) (websocket.TopicID, bool) {
	if c == nil {
		return 0, false
	}
	if c.isAuthAck(payload) {
		return c.authTopicID, true
	}
	if stream, ok := scanStringField(payload, keyStream); ok {
		if id, exists := c.lookupStream(stream); exists {
			return id, true
		}
	}
	if symbol, ok := scanStringField(payload, keySymbol); ok {
		if id, exists := c.lookupSymbol(symbol); exists {
			return id, true
		}
	}
	return 0, false
}

// EncodeSubscribe builds a Binance subscribe payload.
func (c *DynamicCodec) EncodeSubscribe(dst []byte, subscribeID websocket.SubscribeID, topic websocket.TopicID) (websocket.MessageType, []byte, error) {
	if c.isAuthTopic(topic) {
		return c.encodeAuth(dst)
	}
	stream, ok := c.lookupStreamByID(topic)
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

// EncodeUnsubscribe builds a Binance unsubscribe payload.
func (c *DynamicCodec) EncodeUnsubscribe(dst []byte, subscribeID websocket.SubscribeID, topic websocket.TopicID) (websocket.MessageType, []byte, error) {
	if c.isAuthTopic(topic) {
		return websocket.MessageText, dst[:0], nil
	}
	stream, ok := c.lookupStreamByID(topic)
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

func (c *DynamicCodec) lookupStreamByID(topic websocket.TopicID) ([]byte, bool) {
	c.mu.RLock()
	stream, ok := c.streamByID[topic]
	c.mu.RUnlock()
	return stream, ok
}

func (c *DynamicCodec) lookupStream(stream []byte) (websocket.TopicID, bool) {
	c.mu.RLock()
	id, ok := c.streamLookup[hashBytes(stream)]
	c.mu.RUnlock()
	return id, ok
}

func (c *DynamicCodec) lookupSymbol(symbol []byte) (websocket.TopicID, bool) {
	c.mu.RLock()
	id, ok := c.symbolLookup[hashBytes(symbol)]
	c.mu.RUnlock()
	return id, ok
}

func (c *DynamicCodec) isAuthTopic(topic websocket.TopicID) bool {
	c.mu.RLock()
	enabled := c.authEnabled && c.authTopicID == topic
	c.mu.RUnlock()
	return enabled
}

func (c *DynamicCodec) encodeAuth(dst []byte) (websocket.MessageType, []byte, error) {
	c.mu.RLock()
	apiKey := c.authAPIKey
	reqID := c.authReqID
	c.mu.RUnlock()
	if len(apiKey) == 0 {
		return 0, nil, exception.ErrWebSocketProtocol
	}
	dst = append(dst, `{"method":"auth","params":["`...)
	dst = append(dst, apiKey...)
	dst = append(dst, `"],"id":`...)
	dst = appendUint(dst, reqID)
	dst = append(dst, '}')
	return websocket.MessageText, dst, nil
}

func (c *DynamicCodec) isAuthAck(payload []byte) bool {
	c.mu.RLock()
	enabled := c.authEnabled
	reqID := c.authReqID
	c.mu.RUnlock()
	if !enabled {
		return false
	}
	if id, ok := scanUintField(payload, keyID); !ok || id != reqID {
		return false
	}
	if result, ok := scanStringField(payload, keyResult); ok {
		return isSuccessValue(result)
	}
	if status, ok := scanStringField(payload, keyStatus); ok {
		return isSuccessValue(status)
	}
	return bytesContains(payload, keySuccessTrue)
}

func scanUintField(payload []byte, key []byte) (uint64, bool) {
	idx := indexOf(payload, key)
	if idx < 0 {
		return 0, false
	}
	i := idx + len(key)
	for i < len(payload) && payload[i] != ':' {
		i++
	}
	if i >= len(payload) {
		return 0, false
	}
	i++
	for i < len(payload) && isSpace(payload[i]) {
		i++
	}
	if i >= len(payload) || payload[i] < '0' || payload[i] > '9' {
		return 0, false
	}
	var v uint64
	for i < len(payload) && payload[i] >= '0' && payload[i] <= '9' {
		v = v*10 + uint64(payload[i]-'0')
		i++
	}
	return v, true
}

func isSuccessValue(value []byte) bool {
	if len(value) != len(successValue) {
		return false
	}
	for i := range value {
		if value[i] != successValue[i] {
			return false
		}
	}
	return true
}

func bytesContains(haystack []byte, needle []byte) bool {
	if len(needle) == 0 {
		return true
	}
	if len(haystack) < len(needle) {
		return false
	}
outer:
	for i := 0; i <= len(haystack)-len(needle); i++ {
		for j := 0; j < len(needle); j++ {
			if haystack[i+j] != needle[j] {
				continue outer
			}
		}
		return true
	}
	return false
}

func parseSymbolUpper(stream []byte) []byte {
	if len(stream) == 0 {
		return nil
	}
	end := len(stream)
	for i := 0; i < len(stream); i++ {
		if stream[i] == '@' {
			end = i
			break
		}
	}
	if end == 0 {
		return nil
	}
	out := make([]byte, end)
	for i := 0; i < end; i++ {
		b := stream[i]
		if b >= 'a' && b <= 'z' {
			b -= 'a' - 'A'
		}
		out[i] = b
	}
	return out
}

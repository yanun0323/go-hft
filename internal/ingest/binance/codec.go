package binance

import (
	"errors"
	"main/internal/adapter"
	"main/internal/adapter/enum"
	"main/pkg/exception"
	"main/pkg/scanner"
	"main/pkg/websocket"
	"strings"
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

// Codec supports runtime topic registration for Binance streams.
type Codec struct {
	mu           sync.RWMutex
	streamByID   map[websocket.TopicID][]byte
	streamLookup map[uint64]websocket.TopicID
	symbolLookup map[uint64]websocket.TopicID
	authTopicID  websocket.TopicID
	authReqID    uint64
	authAPIKey   []byte
	authEnabled  bool
}

// NewCodec creates a codec that can register topics on demand.
func NewCodec() *Codec {
	return &Codec{
		streamByID:   make(map[websocket.TopicID][]byte),
		streamLookup: make(map[uint64]websocket.TopicID),
		symbolLookup: make(map[uint64]websocket.TopicID),
	}
}

// RegisterAuth configures auth payload metadata for this connection.
func (c *Codec) RegisterAuth(topicID websocket.TopicID, apiKey string, reqID uint64) error {
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

// EncodeAuth builds the auth payload.
func (c *Codec) EncodeAuth(dst []byte, apiKey string, reqID uint64) (websocket.MessageType, []byte, error) {
	if c == nil {
		return 0, nil, exception.ErrWebSocketProtocol
	}
	if apiKey == "" {
		return 0, nil, exception.ErrWebSocketProtocol
	}
	dst = append(dst, `{"method":"auth","params":["`...)
	dst = append(dst, apiKey...)
	dst = append(dst, `"],"id":`...)
	dst = appendUint(dst, reqID)
	dst = append(dst, '}')
	return websocket.MessageText, dst, nil
}

// Register adds a topic mapping for a stream name.
func (c *Codec) Register(topicID websocket.TopicID, req adapter.IngestRequest) error {
	if c == nil {
		return errEmptyTopic
	}
	if !req.Topic.IsAvailable() {
		return errEmptyTopic
	}
	stream, err := streamForRequest(req)
	if err != nil {
		return err
	}
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

func streamForRequest(req adapter.IngestRequest) ([]byte, error) {
	switch req.Topic {
	case enum.TopicDepth:
		market := symbolToMarket(req.Symbol)
		stream := strings.ToLower(market) + "@depth@100ms"
		return []byte(stream), nil
	case enum.TopicOrder:
		market := symbolToMarket(req.Symbol)
		stream := strings.ToLower(market)
		return []byte(stream), nil
	default:
		return nil, errEmptyTopic
	}
}

func symbolToMarket(symbol adapter.Symbol) string {
	// TODO: map symbol to market string (e.g. BTCUSDT).
	return symbol.String()
}

// Unregister removes a topic mapping.
func (c *Codec) Unregister(topicID websocket.TopicID) {
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
func (c *Codec) DecodeTopic(payload []byte) (websocket.TopicID, bool) {
	if c == nil {
		return 0, false
	}
	if c.isAuthAck(payload) {
		return c.authTopicID, true
	}
	if stream, ok := scanner.ScanStringField(payload, keyEvent); ok {
		if id, exists := c.lookupStream(stream); exists {
			return id, true
		}
	}
	if symbol, ok := scanner.ScanStringField(payload, keySymbol); ok {
		if id, exists := c.lookupSymbol(symbol); exists {
			return id, true
		}
	}
	return 0, false
}

// EncodeSubscribe builds a Binance subscribe payload.
func (c *Codec) EncodeSubscribe(dst []byte, topic websocket.TopicID) (websocket.MessageType, []byte, error) {
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
	dst = appendUint(dst, uint64(topic))
	dst = append(dst, '}')
	return websocket.MessageText, dst, nil
}

// EncodeUnsubscribe builds a Binance unsubscribe payload.
func (c *Codec) EncodeUnsubscribe(dst []byte, topic websocket.TopicID) (websocket.MessageType, []byte, error) {
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
	dst = appendUint(dst, uint64(topic))
	dst = append(dst, '}')
	return websocket.MessageText, dst, nil
}

func (c *Codec) lookupStreamByID(topic websocket.TopicID) ([]byte, bool) {
	c.mu.RLock()
	stream, ok := c.streamByID[topic]
	c.mu.RUnlock()
	return stream, ok
}

func (c *Codec) lookupStream(stream []byte) (websocket.TopicID, bool) {
	c.mu.RLock()
	id, ok := c.streamLookup[hashBytes(stream)]
	c.mu.RUnlock()
	return id, ok
}

func (c *Codec) lookupSymbol(symbol []byte) (websocket.TopicID, bool) {
	c.mu.RLock()
	id, ok := c.symbolLookup[hashBytes(symbol)]
	c.mu.RUnlock()
	return id, ok
}

func (c *Codec) isAuthTopic(topic websocket.TopicID) bool {
	c.mu.RLock()
	enabled := c.authEnabled && c.authTopicID == topic
	c.mu.RUnlock()
	return enabled
}

func (c *Codec) encodeAuth(dst []byte) (websocket.MessageType, []byte, error) {
	c.mu.RLock()
	apiKey := c.authAPIKey
	reqID := c.authReqID
	c.mu.RUnlock()
	return c.EncodeAuth(dst, string(apiKey), reqID)
}

func (c *Codec) isAuthAck(payload []byte) bool {
	c.mu.RLock()
	enabled := c.authEnabled
	reqID := c.authReqID
	c.mu.RUnlock()
	if !enabled {
		return false
	}
	if id, ok := scanner.ScanUintField(payload, keyID); !ok || id != reqID {
		return false
	}
	if result, ok := scanner.ScanStringField(payload, keyResult); ok {
		return isSuccessValue(result)
	}
	if status, ok := scanner.ScanStringField(payload, keyStatus); ok {
		return isSuccessValue(status)
	}
	return scanner.BytesContains(payload, keySuccessTrue)
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

func parseSymbolUpper(stream []byte) []byte {
	if len(stream) == 0 {
		return nil
	}
	end := len(stream)
	for i := range stream {
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

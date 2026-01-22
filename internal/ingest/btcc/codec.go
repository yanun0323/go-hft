package btcc

import (
	"crypto/sha256"
	"encoding/hex"
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
	errEmptyTopic = errors.New("btcc: empty topic")
)

var (
	keyID     = []byte(`"id"`)
	keyMethod = []byte(`"method"`)
	keyResult = []byte(`"result"`)
	keyStatus = []byte(`"status"`)
	keyMarket = []byte(`"market"`)
)

var (
	methodDepthUpdate = []byte("depth.update")
	methodOrderUpdate = []byte("order.update")
	successValue      = []byte("success")
)

type topicKind uint8

const (
	topicKindUnknown topicKind = iota
	topicKindDepth
	topicKindOrder
)

type topicMeta struct {
	kind   topicKind
	market []byte
}

// Codec supports runtime topic registration for BTCC streams.
type Codec struct {
	mu          sync.RWMutex
	topicsByID  map[websocket.TopicID]topicMeta
	depthLookup map[uint64]websocket.TopicID
	orderLookup map[uint64]websocket.TopicID

	authTopicID websocket.TopicID
	authReqID   uint64
	authAccess  string
	authSecret  string
	authEnabled bool
}

// NewCodec creates a codec that can register topics on demand.
func NewCodec() *Codec {
	return &Codec{
		topicsByID:  make(map[websocket.TopicID]topicMeta),
		depthLookup: make(map[uint64]websocket.TopicID),
		orderLookup: make(map[uint64]websocket.TopicID),
	}
}

// RegisterAuth configures auth payload metadata for this connection.
// apiKey is expected as "access_id:secret".
func (c *Codec) RegisterAuth(topicID websocket.TopicID, apiKey string, reqID uint64) error {
	if c == nil {
		return errEmptyTopic
	}
	accessID, secret, ok := splitAPIKey(apiKey)
	if !ok {
		return errEmptyTopic
	}
	c.mu.Lock()
	c.authTopicID = topicID
	c.authReqID = reqID
	c.authAccess = accessID
	c.authSecret = secret
	c.authEnabled = true
	c.mu.Unlock()
	return nil
}

// EncodeAuth builds the auth payload.
func (c *Codec) EncodeAuth(dst []byte, apiKey string, reqID uint64) (websocket.MessageType, []byte, error) {
	if c == nil {
		return 0, nil, exception.ErrWebSocketProtocol
	}
	accessID, secret, ok := splitAPIKey(apiKey)
	if !ok {
		return 0, nil, exception.ErrWebSocketProtocol
	}
	sum := sha256.Sum256([]byte(secret))
	signData := hex.EncodeToString(sum[:])
	dst = append(dst, `{"id":`...)
	dst = appendUint(dst, reqID)
	dst = append(dst, `,"method":"server.accessid_auth","params":["`...)
	dst = append(dst, accessID...)
	dst = append(dst, `","`...)
	dst = append(dst, signData...)
	dst = append(dst, `"]}`...)
	return websocket.MessageText, dst, nil
}

// Register adds a topic mapping for a market name.
func (c *Codec) Register(topicID websocket.TopicID, req adapter.MarketDataRequest) error {
	if c == nil {
		return errEmptyTopic
	}
	kind, market, err := marketFromRequest(req)
	if err != nil {
		return err
	}
	if len(market) == 0 {
		return errEmptyTopic
	}

	c.mu.Lock()
	c.topicsByID[topicID] = topicMeta{kind: kind, market: market}
	switch kind {
	case topicKindOrder:
		c.orderLookup[hashBytes(market)] = topicID
	default:
		c.depthLookup[hashBytes(market)] = topicID
	}
	c.mu.Unlock()
	return nil
}

// Unregister removes a topic mapping.
func (c *Codec) Unregister(topicID websocket.TopicID) {
	if c == nil {
		return
	}
	c.mu.Lock()
	meta, ok := c.topicsByID[topicID]
	if ok {
		delete(c.topicsByID, topicID)
		switch meta.kind {
		case topicKindOrder:
			delete(c.orderLookup, hashBytes(meta.market))
		default:
			delete(c.depthLookup, hashBytes(meta.market))
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
	method, ok := scanner.ScanStringField(payload, keyMethod)
	if !ok {
		return 0, false
	}
	switch {
	case bytesEqual(method, methodDepthUpdate):
		market, ok := scanParamsString(payload, 2)
		if !ok {
			return 0, false
		}
		return c.lookupDepth(market)
	case bytesEqual(method, methodOrderUpdate):
		orderValue, ok := scanParamsValue(payload, 1)
		if !ok {
			return 0, false
		}
		market, ok := scanStringField(orderValue, keyMarket)
		if !ok {
			return 0, false
		}
		return c.lookupOrder(market)
	default:
		return 0, false
	}
}

// EncodeSubscribe builds a BTCC subscribe payload.
func (c *Codec) EncodeSubscribe(dst []byte, topic websocket.TopicID) (websocket.MessageType, []byte, error) {
	if c.isAuthTopic(topic) {
		return c.encodeAuth(dst)
	}
	meta, ok := c.lookupTopicMeta(topic)
	if !ok {
		return 0, nil, exception.ErrWebSocketProtocol
	}
	switch meta.kind {
	case topicKindOrder:
		dst = append(dst, `{"id":`...)
		dst = appendUint(dst, uint64(topic))
		dst = append(dst, `,"method":"order.subscribe","params":["`...)
		dst = append(dst, meta.market...)
		dst = append(dst, `"]}`...)
	default:
		dst = append(dst, `{"id":`...)
		dst = appendUint(dst, uint64(topic))
		dst = append(dst, `,"method":"depth.subscribe","params":["`...)
		dst = append(dst, meta.market...)
		dst = append(dst, `",20,"0.00000001"]}`...)
	}
	return websocket.MessageText, dst, nil
}

// EncodeUnsubscribe builds a BTCC unsubscribe payload.
func (c *Codec) EncodeUnsubscribe(dst []byte, topic websocket.TopicID) (websocket.MessageType, []byte, error) {
	if c.isAuthTopic(topic) {
		return websocket.MessageText, dst[:0], nil
	}
	meta, ok := c.lookupTopicMeta(topic)
	if !ok {
		return 0, nil, exception.ErrWebSocketProtocol
	}
	switch meta.kind {
	case topicKindOrder:
		dst = append(dst, `{"id":`...)
		dst = appendUint(dst, uint64(topic))
		dst = append(dst, `,"method":"order.unsubscribe","params":["`...)
		dst = append(dst, meta.market...)
		dst = append(dst, `"]}`...)
	default:
		dst = append(dst, `{"id":`...)
		dst = appendUint(dst, uint64(topic))
		dst = append(dst, `,"method":"depth.unsubscribe","params":["`...)
		dst = append(dst, meta.market...)
		dst = append(dst, `"]}`...)
	}
	return websocket.MessageText, dst, nil
}

func (c *Codec) lookupTopicMeta(topic websocket.TopicID) (topicMeta, bool) {
	c.mu.RLock()
	meta, ok := c.topicsByID[topic]
	c.mu.RUnlock()
	return meta, ok
}

func (c *Codec) lookupDepth(market []byte) (websocket.TopicID, bool) {
	c.mu.RLock()
	id, ok := c.depthLookup[hashBytes(market)]
	c.mu.RUnlock()
	return id, ok
}

func (c *Codec) lookupOrder(market []byte) (websocket.TopicID, bool) {
	c.mu.RLock()
	id, ok := c.orderLookup[hashBytes(market)]
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
	accessID := c.authAccess
	secret := c.authSecret
	reqID := c.authReqID
	c.mu.RUnlock()
	if accessID == "" || secret == "" {
		return 0, nil, exception.ErrWebSocketProtocol
	}
	sum := sha256.Sum256([]byte(secret))
	signData := hex.EncodeToString(sum[:])
	dst = append(dst, `{"id":`...)
	dst = appendUint(dst, reqID)
	dst = append(dst, `,"method":"server.accessid_auth","params":["`...)
	dst = append(dst, accessID...)
	dst = append(dst, `","`...)
	dst = append(dst, signData...)
	dst = append(dst, `"]}`...)
	return websocket.MessageText, dst, nil
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
	if status, ok := scanner.ScanStringField(payload, keyStatus); ok {
		return bytesEqual(status, successValue)
	}
	if result, ok := scanner.ScanStringField(payload, keyResult); ok {
		return bytesEqual(result, successValue)
	}
	return false
}

func splitAPIKey(apiKey string) (string, string, bool) {
	if apiKey == "" {
		return "", "", false
	}
	idx := strings.IndexByte(apiKey, ':')
	if idx <= 0 || idx >= len(apiKey)-1 {
		return "", "", false
	}
	return apiKey[:idx], apiKey[idx+1:], true
}

func bytesEqual(a []byte, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func marketFromRequest(req adapter.MarketDataRequest) (topicKind, []byte, error) {
	switch req.Topic {
	case enum.TopicDepth:
		market := symbolToMarket(req.Symbol)
		return topicKindDepth, []byte(market), nil
	case enum.TopicOrder:
		market := symbolToMarket(req.Symbol)
		return topicKindOrder, []byte(market), nil
	default:
		return topicKindUnknown, nil, errEmptyTopic
	}
}

func symbolToMarket(symbol adapter.Symbol) string {
	// TODO: map symbol to market string (e.g. BTCUSDT).
	return symbol.String()
}

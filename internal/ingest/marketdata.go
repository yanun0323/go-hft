package ingest

import (
	"context"
	"encoding/binary"
	"errors"
	binance "main/internal/ingest/binance"
	"main/internal/model"
	"main/internal/model/enum"
	"main/pkg/websocket"
	"sync"
	"sync/atomic"
)

var (
	ErrInvalidMarketDataRequest = errors.New("marketdata: invalid request")
	ErrUnsupportedPlatform      = errors.New("marketdata: unsupported platform")
	ErrNilConsumer              = errors.New("marketdata: nil consumer")
	ErrTopicKindMismatch        = errors.New("marketdata: topic kind mismatch")
	ErrUnknownTopic             = errors.New("marketdata: unknown topic")
)

const (
	marketDataReqHeaderSize = 1 + 1 + 2
	maxUint16               = int(^uint16(0))
)

// MarketData wires UDS requests to platform websocket managers.
// This is the minimal skeleton; request/response framing can evolve later.
type MarketData struct {
	mu        sync.Mutex
	groups    map[groupKey]*wsGroup
	nextSubID atomic.Uint64
}

// NewMarketData initializes a MarketData registry.
func NewMarketData() *MarketData {
	return &MarketData{
		groups: make(map[groupKey]*wsGroup),
	}
}

func (m *MarketData) nextSubscribeID() websocket.SubscribeID {
	return websocket.SubscribeID(m.nextSubID.Add(1))
}

type groupKey struct {
	platform enum.Platform
	apiKey   string
}

type wsGroup struct {
	mu           sync.RWMutex
	manager      *websocket.Manager
	codec        platformCodec
	topics       map[topicKey]*topicState
	topicsByID   map[websocket.TopicID]*topicState
	nextTopicID  atomic.Uint32
	running      atomic.Bool
	authReady    atomic.Bool
	authInit     atomic.Bool
	authTopicID  websocket.TopicID
	authSubID    websocket.SubscribeID
	authRequired bool
}

type topicState struct {
	topic    enum.Topic
	arg      string
	argBytes []byte
	topicID  websocket.TopicID
	subID    websocket.SubscribeID
	refCount int
	kind     enum.MarketDataKind
}

type topicKey struct {
	topic enum.Topic
	arg   string
}

type platformCodec interface {
	websocket.TopicDecoder
	websocket.ControlEncoder
	Register(topicID websocket.TopicID, topic string) error
	Unregister(topicID websocket.TopicID)
	RegisterAuth(topicID websocket.TopicID, apiKey string, reqID uint64) error
	ClearAuth()
}

const (
	binanceHost = "stream.binance.com"
	binancePort = "9443"
	binancePath = "/ws"
)

// KindFromTopic maps a topic enum to the market data kind.
func KindFromTopic(topic enum.Topic) (enum.MarketDataKind, bool) {
	switch topic {
	case enum.TopicDepth:
		return enum.MarketDataDepth, true
	case enum.TopicOrder:
		return enum.MarketDataOrder, true
	default:
		return 0, false
	}
}

// Subscribe registers a topic and attaches the consumer to receive frames.
func (m *MarketData) Subscribe(ctx context.Context, platform enum.Platform, apiKey string, topic enum.Topic, arg []byte, consumer *websocket.Consumer) error {
	if m == nil {
		return ErrInvalidMarketDataRequest
	}
	if !platform.IsAvailable() || !topic.IsAvailable() || len(arg) == 0 {
		return ErrInvalidMarketDataRequest
	}
	if consumer == nil {
		return ErrNilConsumer
	}

	group, err := m.getOrCreateGroup(ctx, platform, apiKey)
	if err != nil {
		return err
	}
	if apiKey != "" {
		if err := group.ensureAuth(ctx, m, apiKey); err != nil {
			return err
		}
	}

	state, err := group.ensureTopic(m, topic, arg)
	if err != nil {
		return err
	}
	if err := group.manager.AddConsumer(state.subID, consumer); err != nil {
		return err
	}

	group.mu.Lock()
	state.refCount++
	group.mu.Unlock()
	return nil
}

// Unsubscribe detaches the consumer and removes topic registration when no longer used.
func (m *MarketData) Unsubscribe(platform enum.Platform, apiKey string, topic enum.Topic, arg []byte, consumer *websocket.Consumer) error {
	if m == nil {
		return ErrInvalidMarketDataRequest
	}
	if !platform.IsAvailable() || !topic.IsAvailable() || len(arg) == 0 {
		return ErrInvalidMarketDataRequest
	}
	if consumer == nil {
		return ErrNilConsumer
	}

	group := m.getGroup(platform, apiKey)
	if group == nil {
		return ErrUnknownTopic
	}
	key := topicKey{topic: topic, arg: string(arg)}

	group.mu.RLock()
	state := group.topics[key]
	group.mu.RUnlock()
	if state == nil {
		return ErrUnknownTopic
	}

	if err := group.manager.RemoveConsumer(state.subID, consumer); err != nil {
		return err
	}

	remove := false
	group.mu.Lock()
	state.refCount--
	if state.refCount <= 0 {
		delete(group.topics, key)
		delete(group.topicsByID, state.topicID)
		remove = true
	}
	group.mu.Unlock()

	if remove {
		group.codec.Unregister(state.topicID)
		_ = group.manager.Unsubscribe(state.subID)
	}
	return nil
}

// Resolve maps a topic id to its topic metadata.
func (m *MarketData) Resolve(platform enum.Platform, apiKey string, topicID websocket.TopicID) (enum.Topic, []byte, enum.MarketDataKind, bool) {
	if m == nil || !platform.IsAvailable() {
		return 0, nil, 0, false
	}
	group := m.getGroup(platform, apiKey)
	if group == nil {
		return 0, nil, 0, false
	}
	group.mu.RLock()
	state := group.topicsByID[topicID]
	group.mu.RUnlock()
	if state == nil {
		return 0, nil, 0, false
	}
	return state.topic, state.argBytes, state.kind, true
}

// AuthReady reports whether auth has completed for the group.
func (m *MarketData) AuthReady(platform enum.Platform, apiKey string) bool {
	if m == nil || !platform.IsAvailable() {
		return false
	}
	group := m.getGroup(platform, apiKey)
	if group == nil {
		return false
	}
	if !group.authRequired {
		return true
	}
	return group.authReady.Load()
}

func (m *MarketData) getGroup(platform enum.Platform, apiKey string) *wsGroup {
	if m == nil {
		return nil
	}
	key := groupKey{platform: platform, apiKey: apiKey}
	m.mu.Lock()
	group := m.groups[key]
	m.mu.Unlock()
	return group
}

func (m *MarketData) getOrCreateGroup(ctx context.Context, platform enum.Platform, apiKey string) (*wsGroup, error) {
	if !platform.IsAvailable() {
		return nil, ErrInvalidMarketDataRequest
	}
	key := groupKey{platform: platform, apiKey: apiKey}

	m.mu.Lock()
	group := m.groups[key]
	if group != nil {
		m.mu.Unlock()
		group.start(ctx)
		return group, nil
	}

	group, err := newGroup(platform, apiKey)
	if err != nil {
		m.mu.Unlock()
		return nil, err
	}
	m.groups[key] = group
	m.mu.Unlock()
	group.start(ctx)
	return group, nil
}

func newGroup(platform enum.Platform, apiKey string) (*wsGroup, error) {
	switch platform {
	case enum.PlatformBinance:
		codec := binance.NewDynamicCodec()
		group := &wsGroup{
			codec:      codec,
			topics:     make(map[topicKey]*topicState),
			topicsByID: make(map[websocket.TopicID]*topicState),
		}
		dialer := websocket.NewDialer(context.Background(), binanceHost, binancePort, binancePath)
		manager, err := websocket.New(dialer, codec, codec, websocket.Option{
			FanOut: websocket.FanOutShared,
			OnConnect: func(ctx context.Context, w websocket.Writer) error {
				group.authReady.Store(false)
				return nil
			},
			OnDisconnect: func(err error) {
				group.authReady.Store(false)
			},
		})
		if err != nil {
			return nil, err
		}
		group.manager = manager
		return group, nil
	default:
		return nil, ErrUnsupportedPlatform
	}
}

func (g *wsGroup) start(ctx context.Context) {
	if g == nil || g.manager == nil {
		return
	}
	if !g.running.CompareAndSwap(false, true) {
		return
	}
	go func() {
		_ = g.manager.Run(ctx)
		g.running.Store(false)
	}()
}

func (g *wsGroup) ensureTopic(m *MarketData, topic enum.Topic, arg []byte) (*topicState, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	kind, ok := KindFromTopic(topic)
	if !ok {
		return nil, ErrInvalidMarketDataRequest
	}
	argStr := string(arg)
	key := topicKey{topic: topic, arg: argStr}

	if existing := g.topics[key]; existing != nil {
		return existing, nil
	}

	topicID := websocket.TopicID(g.nextTopicID.Add(1))
	subID := m.nextSubscribeID()
	if err := g.codec.Register(topicID, argStr); err != nil {
		return nil, err
	}
	if err := g.manager.Subscribe(subID, topicID); err != nil {
		g.codec.Unregister(topicID)
		return nil, err
	}

	state := &topicState{
		topic:    topic,
		arg:      argStr,
		argBytes: []byte(argStr),
		topicID:  topicID,
		subID:    subID,
		kind:     kind,
	}
	g.topics[key] = state
	g.topicsByID[topicID] = state
	return state, nil
}

func (g *wsGroup) ensureAuth(ctx context.Context, m *MarketData, apiKey string) error {
	if g == nil || g.manager == nil || g.codec == nil {
		return ErrInvalidMarketDataRequest
	}
	if apiKey == "" {
		return nil
	}
	if !g.authInit.CompareAndSwap(false, true) {
		return nil
	}
	g.authRequired = true
	g.authReady.Store(false)

	g.authTopicID = websocket.TopicID(g.nextTopicID.Add(1))
	g.authSubID = m.nextSubscribeID()

	if err := g.codec.RegisterAuth(g.authTopicID, apiKey, uint64(g.authSubID)); err != nil {
		g.authInit.Store(false)
		g.authRequired = false
		return err
	}
	if err := g.manager.Subscribe(g.authSubID, g.authTopicID); err != nil {
		g.codec.ClearAuth()
		g.authInit.Store(false)
		g.authRequired = false
		return err
	}

	authConsumer := websocket.NewConsumer(8, websocket.OverflowDropOldest)
	if err := g.manager.AddConsumer(g.authSubID, authConsumer); err != nil {
		g.codec.ClearAuth()
		_ = g.manager.Unsubscribe(g.authSubID)
		g.authInit.Store(false)
		g.authRequired = false
		return err
	}

	go g.watchAuth(ctx, authConsumer)
	return nil
}

func (g *wsGroup) watchAuth(ctx context.Context, consumer *websocket.Consumer) {
	if consumer == nil {
		return
	}
	for {
		frame, ok := consumer.Next()
		if !ok {
			return
		}
		g.authReady.Store(true)
		frame.Release()
		if ctx.Err() != nil {
			return
		}
	}
}

// MarketDataRequest is the minimal UDS request format.
// Arg slices alias the input buffer; copy if you need to retain them.
type MarketDataRequest struct {
	Platform enum.Platform
	Topic    enum.Topic
	Arg      []byte
}

type MarketDataArgDepth struct {
	Symbol   model.Symbol
	Interval []byte
}

type MarketDataArgOrder struct {
	Symbol model.Symbol
	APIKey []byte
}

// DecodeMarketDataRequest parses a request from a byte buffer.
// Format:
// [0] platform (uint8)
// [1] topic (uint8)
// [2:4] argLen (uint16, big endian)
// [4:] arg
func DecodeMarketDataRequest(src []byte) (MarketDataRequest, int, error) {
	var req MarketDataRequest
	if len(src) < marketDataReqHeaderSize {
		return req, 0, ErrInvalidMarketDataRequest
	}
	req.Platform = enum.Platform(src[0])
	req.Topic = enum.Topic(src[1])
	argLen := int(binary.BigEndian.Uint16(src[2:4]))
	total := marketDataReqHeaderSize + argLen
	if argLen < 0 || total > len(src) {
		return req, 0, ErrInvalidMarketDataRequest
	}
	if !req.Platform.IsAvailable() || !req.Topic.IsAvailable() {
		return req, 0, ErrInvalidMarketDataRequest
	}
	if argLen > 0 {
		req.Arg = src[marketDataReqHeaderSize : marketDataReqHeaderSize+argLen]
	}
	return req, total, nil
}

// EncodeMarketDataRequest serializes a request into dst.
func EncodeMarketDataRequest(dst []byte, req MarketDataRequest) ([]byte, error) {
	if !req.Platform.IsAvailable() || !req.Topic.IsAvailable() {
		return nil, ErrInvalidMarketDataRequest
	}

	argLen := len(req.Arg)
	if argLen > maxUint16 {
		return nil, ErrInvalidMarketDataRequest
	}
	total := marketDataReqHeaderSize + argLen
	if cap(dst) < total {
		dst = make([]byte, total)
	} else {
		dst = dst[:total]
	}
	dst[0] = byte(req.Platform)
	dst[1] = byte(req.Topic)
	binary.BigEndian.PutUint16(dst[2:4], uint16(argLen))
	copy(dst[marketDataReqHeaderSize:], req.Arg)
	return dst, nil
}

// DecodeMarketDataArgDepth parses a depth arg payload.
// Format: [0:2] symbol (uint16, big endian) + interval bytes.
func DecodeMarketDataArgDepth(src []byte) (MarketDataArgDepth, error) {
	var arg MarketDataArgDepth
	if len(src) < 2 {
		return arg, ErrInvalidMarketDataRequest
	}
	arg.Symbol = model.Symbol(binary.BigEndian.Uint16(src[0:2]))
	if len(src) > 2 {
		arg.Interval = src[2:]
	}
	return arg, nil
}

// EncodeMarketDataArgDepth serializes a depth arg payload.
func EncodeMarketDataArgDepth(dst []byte, arg MarketDataArgDepth) ([]byte, error) {
	total := 2 + len(arg.Interval)
	if total > maxUint16 {
		return nil, ErrInvalidMarketDataRequest
	}
	if cap(dst) < total {
		dst = make([]byte, total)
	} else {
		dst = dst[:total]
	}
	binary.BigEndian.PutUint16(dst[0:2], uint16(arg.Symbol))
	copy(dst[2:], arg.Interval)
	return dst, nil
}

// DecodeMarketDataArgOrder parses an order arg payload.
// Format: [0:2] symbol (uint16, big endian) + apiKey bytes.
func DecodeMarketDataArgOrder(src []byte) (MarketDataArgOrder, error) {
	var arg MarketDataArgOrder
	if len(src) < 2 {
		return arg, ErrInvalidMarketDataRequest
	}
	arg.Symbol = model.Symbol(binary.BigEndian.Uint16(src[0:2]))
	if len(src) > 2 {
		arg.APIKey = src[2:]
	}
	return arg, nil
}

// EncodeMarketDataArgOrder serializes an order arg payload.
func EncodeMarketDataArgOrder(dst []byte, arg MarketDataArgOrder) ([]byte, error) {
	total := 2 + len(arg.APIKey)
	if total > maxUint16 {
		return nil, ErrInvalidMarketDataRequest
	}
	if cap(dst) < total {
		dst = make([]byte, total)
	} else {
		dst = dst[:total]
	}
	binary.BigEndian.PutUint16(dst[0:2], uint16(arg.Symbol))
	copy(dst[2:], arg.APIKey)
	return dst, nil
}

package ingest

import (
	"context"
	"main/internal/adapter/enum"
	"main/internal/ingest/binance"
	"main/pkg/exception"
	"main/pkg/websocket"
	"sync"
	"sync/atomic"
)

var ()

// MarketData wires UDS requests to platform websocket managers.
// This is the minimal skeleton; request/response framing can evolve later.
type MarketData struct {
	mu          sync.Mutex
	groups      map[groupKey]*wsGroup
	currTopicID atomic.Uint32
}

// NewMarketData initializes a MarketData registry.
func NewMarketData() *MarketData {
	return &MarketData{
		groups: make(map[groupKey]*wsGroup),
	}
}

func (m *MarketData) nextTopicID() websocket.TopicID {
	return websocket.TopicID(m.currTopicID.Add(1))
}

type groupKey struct {
	platform enum.Platform
	apiKey   string
}

type wsGroup struct {
	mu         sync.RWMutex
	manager    *websocket.Manager
	codec      platformCodec
	topics     map[topicKey]*topicState
	topicsByID map[websocket.TopicID]*topicState
	running    atomic.Bool
	apiKey     string
	authReqID  uint64
}

type topicState struct {
	topic    enum.Topic
	arg      string
	argBytes []byte
	topicID  websocket.TopicID
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
	EncodeAuth(dst []byte, apiKey string, reqID uint64) (websocket.MessageType, []byte, error)
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
		return exception.ErrInvalidMarketDataRequest
	}
	if !platform.IsAvailable() || !topic.IsAvailable() || len(arg) == 0 {
		return exception.ErrInvalidMarketDataRequest
	}
	if consumer == nil {
		return exception.ErrNilConsumer
	}

	group, err := m.getOrCreateGroup(ctx, platform, apiKey)
	if err != nil {
		return err
	}

	state, err := group.ensureTopic(m, topic, arg)
	if err != nil {
		return err
	}
	if err := group.manager.AddConsumer(state.topicID, consumer); err != nil {
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
		return exception.ErrInvalidMarketDataRequest
	}
	if !platform.IsAvailable() || !topic.IsAvailable() || len(arg) == 0 {
		return exception.ErrInvalidMarketDataRequest
	}
	if consumer == nil {
		return exception.ErrNilConsumer
	}

	group := m.getGroup(platform, apiKey)
	if group == nil {
		return exception.ErrUnknownTopic
	}
	key := topicKey{topic: topic, arg: string(arg)}

	group.mu.RLock()
	state := group.topics[key]
	group.mu.RUnlock()
	if state == nil {
		return exception.ErrUnknownTopic
	}

	if err := group.manager.RemoveConsumer(state.topicID, consumer); err != nil {
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
		_ = group.manager.Unsubscribe(state.topicID)
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
		return nil, exception.ErrInvalidMarketDataRequest
	}
	key := groupKey{platform: platform, apiKey: apiKey}
	var authReqID uint64
	if apiKey != "" {
		authReqID = uint64(m.nextTopicID())
	}

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
	if apiKey != "" {
		group.authReqID = authReqID
		if err := group.codec.RegisterAuth(websocket.TopicID(authReqID), apiKey, authReqID); err != nil {
			m.mu.Unlock()
			return nil, err
		}
	}
	m.groups[key] = group
	m.mu.Unlock()
	group.start(ctx)
	return group, nil
}

func newGroup(platform enum.Platform, apiKey string) (*wsGroup, error) {
	switch platform {
	case enum.PlatformBinance:
		codec := binance.NewCodec()
		group := &wsGroup{
			codec:      codec,
			topics:     make(map[topicKey]*topicState),
			topicsByID: make(map[websocket.TopicID]*topicState),
			apiKey:     apiKey,
		}
		dialer := websocket.NewDialer(context.Background(), binanceHost, binancePort, binancePath)
		manager, err := websocket.New(dialer, codec, codec, websocket.Option{
			FanOut: websocket.FanOutShared,
			OnConnect: func(ctx context.Context, w websocket.Writer) error {
				group.mu.RLock()
				groupAPIKey := group.apiKey
				reqID := group.authReqID
				group.mu.RUnlock()
				if groupAPIKey == "" {
					return nil
				}
				msgType, payload, err := codec.EncodeAuth(nil, groupAPIKey, reqID)
				if err != nil {
					return err
				}
				if !w.Send(msgType, payload) {
					return websocket.ErrQueueFull
				}
				return nil
			},
		})
		if err != nil {
			return nil, err
		}
		group.manager = manager
		return group, nil
	default:
		return nil, exception.ErrUnsupportedPlatform
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
		return nil, exception.ErrInvalidMarketDataRequest
	}
	argStr := string(arg)
	key := topicKey{topic: topic, arg: argStr}

	if existing := g.topics[key]; existing != nil {
		return existing, nil
	}

	topicID := m.nextTopicID()
	if err := g.codec.Register(topicID, argStr); err != nil {
		return nil, err
	}
	if err := g.manager.Subscribe(topicID); err != nil {
		g.codec.Unregister(topicID)
		return nil, err
	}

	state := &topicState{
		topic:    topic,
		arg:      argStr,
		argBytes: []byte(argStr),
		topicID:  topicID,
		kind:     kind,
	}
	g.topics[key] = state
	g.topicsByID[topicID] = state
	return state, nil
}

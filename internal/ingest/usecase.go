package ingest

import (
	"context"
	"main/internal/adapter"
	"main/internal/adapter/enum"
	"main/internal/ingest/binance"
	"main/internal/ingest/btcc"
	"main/pkg/exception"
	"main/pkg/websocket"
	"sync"
	"sync/atomic"
)

// Usecase wires UDS requests to platform websocket managers.
// This is the minimal skeleton; request/response framing can evolve later.
type Usecase struct {
	mu          sync.Mutex
	groups      map[groupKey]*wsGroup
	currTopicID atomic.Uint32
}

// NewUsecase initializes a Ingest registry.
func NewUsecase() *Usecase {
	return &Usecase{
		groups: make(map[groupKey]*wsGroup),
	}
}

func (use *Usecase) nextTopicID() websocket.TopicID {
	return websocket.TopicID(use.currTopicID.Add(1))
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
	apiKey     adapter.APIKey
	authReqID  uint64
	platform   enum.Platform
}

type topicState struct {
	topic    enum.Topic
	arg      string
	symbol   adapter.Symbol
	topicID  websocket.TopicID
	refCount int
}

type topicKey struct {
	topic enum.Topic
	arg   string
}

type platformCodec interface {
	websocket.TopicDecoder
	websocket.ControlEncoder

	Register(topicID websocket.TopicID, req adapter.IngestRequest) error
	Unregister(topicID websocket.TopicID)
	RegisterAuth(topicID websocket.TopicID, apiKey string, reqID uint64) error
	EncodeAuth(dst []byte, apiKey string, reqID uint64) (websocket.MessageType, []byte, error)
}

const (
	binanceHost = "stream.binance.com"
	binancePort = "9443"
	binancePath = "/ws"

	btccHost = "spotprice2.btcccdn.com"
	btccPort = "443"
	btccPath = "/ws"
)

// Subscribe registers a topic and attaches the consumer to receive frames.
func (use *Usecase) Subscribe(ctx context.Context, platform enum.Platform, apiKey adapter.APIKey, topic enum.Topic, symbol adapter.Symbol, consumer *websocket.Consumer) error {
	if use == nil {
		return exception.ErrIngestInvalidRequest
	}
	if !platform.IsAvailable() || !topic.IsAvailable() {
		return exception.ErrIngestInvalidRequest
	}
	if consumer == nil {
		return exception.ErrIngestNilConsumer
	}

	group, err := use.getOrCreateGroup(ctx, platform, apiKey)
	if err != nil {
		return err
	}

	state, err := group.ensureTopic(use, topic, symbol)
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
func (use *Usecase) Unsubscribe(platform enum.Platform, apiKey adapter.APIKey, topic enum.Topic, symbol adapter.Symbol, consumer *websocket.Consumer) error {
	if use == nil {
		return exception.ErrIngestInvalidRequest
	}
	if !platform.IsAvailable() || !topic.IsAvailable() {
		return exception.ErrIngestInvalidRequest
	}
	if consumer == nil {
		return exception.ErrIngestNilConsumer
	}

	group := use.getGroup(platform, apiKey)
	if group == nil {
		return exception.ErrIngestUnknownTopic
	}
	key := topicKey{topic: topic, arg: symbol.String()}

	group.mu.RLock()
	state := group.topics[key]
	group.mu.RUnlock()
	if state == nil {
		return exception.ErrIngestUnknownTopic
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

func (use *Usecase) Resolve(platform enum.Platform, apiKey adapter.APIKey, topicID websocket.TopicID) (enum.Topic, adapter.Symbol, bool) {
	if use == nil || !platform.IsAvailable() {
		return 0, adapter.Symbol{}, false
	}
	group := use.getGroup(platform, apiKey)
	if group == nil {
		return 0, adapter.Symbol{}, false
	}
	group.mu.RLock()
	state := group.topicsByID[topicID]
	group.mu.RUnlock()
	if state == nil {
		return 0, adapter.Symbol{}, false
	}
	return state.topic, state.symbol, true
}

func (use *Usecase) getGroup(platform enum.Platform, apiKey adapter.APIKey) *wsGroup {
	if use == nil {
		return nil
	}
	key := groupKey{platform: platform, apiKey: apiKey.String()}
	use.mu.Lock()
	group := use.groups[key]
	use.mu.Unlock()
	return group
}

func (use *Usecase) getOrCreateGroup(ctx context.Context, platform enum.Platform, apiKey adapter.APIKey) (*wsGroup, error) {
	if !platform.IsAvailable() {
		return nil, exception.ErrIngestInvalidRequest
	}
	key := groupKey{platform: platform, apiKey: apiKey.String()}
	var authReqID uint64
	if len(apiKey) > 0 {
		authReqID = uint64(use.nextTopicID())
	}

	use.mu.Lock()
	group := use.groups[key]
	if group != nil {
		use.mu.Unlock()
		group.start(ctx)
		return group, nil
	}

	group, err := newGroup(platform, apiKey)
	if err != nil {
		use.mu.Unlock()
		return nil, err
	}
	if len(apiKey) > 0 {
		group.authReqID = authReqID
		if err := group.codec.RegisterAuth(websocket.TopicID(authReqID), key.apiKey, authReqID); err != nil {
			use.mu.Unlock()
			return nil, err
		}
	}
	use.groups[key] = group
	use.mu.Unlock()
	group.start(ctx)
	return group, nil
}

func newGroup(platform enum.Platform, apiKey adapter.APIKey) (*wsGroup, error) {
	switch platform {
	case enum.PlatformBinance:
		codec := binance.NewCodec()
		group := &wsGroup{
			codec:      codec,
			topics:     make(map[topicKey]*topicState),
			topicsByID: make(map[websocket.TopicID]*topicState),
			apiKey:     apiKey,
			platform:   platform,
		}
		dialer := websocket.NewDialer(context.Background(), binanceHost, binancePort, binancePath)
		manager, err := websocket.New(dialer, codec, codec, websocket.Option{
			FanOut: websocket.FanOutShared,
			OnConnect: func(ctx context.Context, w websocket.Writer) error {
				group.mu.RLock()
				groupAPIKey := group.apiKey
				reqID := group.authReqID
				group.mu.RUnlock()
				if len(groupAPIKey) == 0 {
					return nil
				}
				msgType, payload, err := codec.EncodeAuth(nil, groupAPIKey.String(), reqID)
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
	case enum.PlatformBTCC:
		codec := btcc.NewCodec()
		group := &wsGroup{
			codec:      codec,
			topics:     make(map[topicKey]*topicState),
			topicsByID: make(map[websocket.TopicID]*topicState),
			apiKey:     apiKey,
			platform:   platform,
		}
		dialer := websocket.NewDialer(context.Background(), btccHost, btccPort, btccPath)
		manager, err := websocket.New(dialer, codec, codec, websocket.Option{
			FanOut: websocket.FanOutShared,
			OnConnect: func(ctx context.Context, w websocket.Writer) error {
				group.mu.RLock()
				groupAPIKey := group.apiKey
				reqID := group.authReqID
				group.mu.RUnlock()
				if len(groupAPIKey) == 0 {
					return nil
				}
				msgType, payload, err := codec.EncodeAuth(nil, groupAPIKey.String(), reqID)
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
		return nil, exception.ErrIngestUnsupportedPlatform
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

func (g *wsGroup) ensureTopic(m *Usecase, topic enum.Topic, symbol adapter.Symbol) (*topicState, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	symbolStr := symbol.String()
	key := topicKey{topic: topic, arg: symbolStr}

	if existing := g.topics[key]; existing != nil {
		return existing, nil
	}

	topicID := m.nextTopicID()
	if err := g.codec.Register(topicID, adapter.IngestRequest{
		Platform: g.platform,
		Topic:    topic,
		Symbol:   symbol,
		APIKey:   g.apiKey,
	}); err != nil {
		return nil, err
	}

	if err := g.manager.Subscribe(topicID); err != nil {
		g.codec.Unregister(topicID)
		return nil, err
	}

	state := &topicState{
		topic:   topic,
		arg:     symbolStr,
		symbol:  symbol,
		topicID: topicID,
	}

	g.topics[key] = state
	g.topicsByID[topicID] = state

	return state, nil
}

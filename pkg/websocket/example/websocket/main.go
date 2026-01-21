package main

import (
	"context"
	"errors"
	"log"
	"main/pkg/metric"
	"main/pkg/websocket"
	"net"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/grafana/pyroscope-go"
)

const (
	binanceHost = "stream.binance.com"
	binancePort = "9443"
	binancePath = "/ws"

	_sockPath = "/tmp/unix.sock"
)

var (
	errHandshakeFailed = errors.New("websocket: handshake failed")
	errProtocol        = errors.New("websocket: protocol error")
	errFrameTooLarge   = errors.New("frame exceeds buffer")
)

const maxPayloadLen = int(^uint32(0) >> 1)

type topicSpec struct {
	ID          websocket.TopicID
	SubscribeID websocket.ConnectionID
	SymbolUpper []byte
	StreamName  []byte
	Label       []byte
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	statsInterval := 15 * time.Second

	conn, err := net.Dial("unix", _sockPath)
	if err != nil {
		panic(err)
	}

	if false {
		profiler, err := pyroscope.Start(pyroscope.Config{
			ApplicationName: "net/ws",
			ServerAddress:   "http://localhost:4040",
			Tags: map[string]string{
				"env": "local",
			},
			Logger: emptyLogger{},
			ProfileTypes: []pyroscope.ProfileType{
				pyroscope.ProfileCPU,
				pyroscope.ProfileAllocObjects,
				pyroscope.ProfileAllocSpace,
				pyroscope.ProfileInuseObjects,
				pyroscope.ProfileInuseSpace,
			},
		})
		if err != nil {
			log.Fatalf("pyroscope start failed: %v", err)
		}
		defer func() {
			_ = profiler.Stop()
		}()
	}

	topics := []topicSpec{
		{
			ID:          1,
			SubscribeID: 1,
			SymbolUpper: []byte("BTCUSDT"),
			StreamName:  []byte("btcusdt@depth@100ms"),
			Label:       []byte("BTCUSDT"),
		},
	}

	decoder := newBinanceTopicDecoder(topics)
	encoder := newBinanceControlEncoder(topics)

	manager, err := websocket.New(
		websocket.NewDialer(ctx, binanceHost, binancePort, binancePath),
		decoder,
		encoder,
		websocket.Option{
			OnConnect: func(ctx context.Context, w websocket.Writer) error {
				log.Println("connected")
				return nil
			},
			OnDisconnect: func(err error) {
				if err != nil && !errors.Is(err, context.Canceled) {
					log.Printf("disconnected: %v", err)
				}
			},
		})
	if err != nil {
		log.Fatalf("manager init failed: %v", err)
	}

	consumer := websocket.NewConsumer(4096, websocket.OverflowDropOldest)
	for _, topic := range topics {
		if err := manager.Subscribe(topic.SubscribeID, topic.ID); err != nil {
			log.Fatalf("register subscription failed: %v", err)
		}
		if err := manager.AddConsumer(topic.SubscribeID, consumer); err != nil {
			log.Fatalf("add consumer failed: %v", err)
		}
	}

	counts := make([]uint64, maxTopicID(topics)+1)
	go consumeFrames(ctx, consumer, counts, conn)
	go memMetric.RunReportSchedule(ctx, statsInterval)

	if err := manager.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("manager stopped: %v", err)
	}

	<-ctx.Done()
	consumer.Close()
}

type emptyLogger struct{}

func (emptyLogger) Infof(_ string, _ ...interface{})  {}
func (emptyLogger) Debugf(_ string, _ ...interface{}) {}
func (emptyLogger) Errorf(_ string, _ ...interface{}) {}

func maxTopicID(topics []topicSpec) int {
	var max int
	for _, t := range topics {
		if id := int(t.ID); id > max {
			max = id
		}
	}
	return max
}

func consumeFrames(ctx context.Context, consumer *websocket.Consumer, counts []uint64, conn net.Conn) {
	arr := make([]byte, 64<<10)
	b := arr[:0]
	for {
		frame, ok := consumer.Next()
		if !ok {
			return
		}
		if int(frame.Topic) < len(counts) {
			atomic.AddUint64(&counts[int(frame.Topic)], 1)
		}

		b = append(b, frame.Buf...)
		if _, err := conn.Write(b); err != nil {
			log.Println("write conn err:", err)
		}
		b = b[:0]

		frame.Release()
		if ctx.Err() != nil {
			return
		}
	}
}

var (
	memMetric = metric.RuntimeMemoryMetric{}
)

type binanceTopicDecoder struct {
	streamLookup map[uint64]websocket.TopicID
	symbolLookup map[uint64]websocket.TopicID
}

func newBinanceTopicDecoder(topics []topicSpec) *binanceTopicDecoder {
	streamLookup := make(map[uint64]websocket.TopicID, len(topics))
	symbolLookup := make(map[uint64]websocket.TopicID, len(topics))
	for _, topic := range topics {
		streamLookup[hashBytes(topic.StreamName)] = topic.ID
		symbolLookup[hashBytes(topic.SymbolUpper)] = topic.ID
	}
	return &binanceTopicDecoder{
		streamLookup: streamLookup,
		symbolLookup: symbolLookup,
	}
}

func (p *binanceTopicDecoder) DecodeTopic(payload []byte) (websocket.TopicID, bool) {
	if stream, ok := scanStringField(payload, keyStream); ok {
		if id, exists := p.streamLookup[hashBytes(stream)]; exists {
			return id, true
		}
	}
	if symbol, ok := scanStringField(payload, keySymbol); ok {
		if id, exists := p.symbolLookup[hashBytes(symbol)]; exists {
			return id, true
		}
	}
	return 0, false
}

type binanceControlEncoder struct {
	streamByID map[websocket.TopicID][]byte
}

func newBinanceControlEncoder(topics []topicSpec) *binanceControlEncoder {
	streamByID := make(map[websocket.TopicID][]byte, len(topics))
	for _, topic := range topics {
		streamByID[topic.ID] = topic.StreamName
	}
	return &binanceControlEncoder{streamByID: streamByID}
}

func (e *binanceControlEncoder) EncodeSubscribe(dst []byte, subscribeID websocket.ConnectionID, topic websocket.TopicID) (websocket.MessageType, []byte, error) {
	stream, ok := e.streamByID[topic]
	if !ok {
		return 0, nil, errProtocol
	}
	dst = append(dst, `{"method":"SUBSCRIBE","params":["`...)
	dst = append(dst, stream...)
	dst = append(dst, `"],"id":`...)
	dst = appendUint(dst, uint64(subscribeID))
	dst = append(dst, '}')
	return websocket.MessageText, dst, nil
}

func (e *binanceControlEncoder) EncodeUnsubscribe(dst []byte, subscribeID websocket.ConnectionID, topic websocket.TopicID) (websocket.MessageType, []byte, error) {
	stream, ok := e.streamByID[topic]
	if !ok {
		return 0, nil, errProtocol
	}
	dst = append(dst, `{"method":"UNSUBSCRIBE","params":["`...)
	dst = append(dst, stream...)
	dst = append(dst, `"],"id":`...)
	dst = appendUint(dst, uint64(subscribeID))
	dst = append(dst, '}')
	return websocket.MessageText, dst, nil
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

var (
	keyStream = []byte(`"stream"`)
	keySymbol = []byte(`"s"`)
)

func scanStringField(payload []byte, key []byte) ([]byte, bool) {
	idx := indexOf(payload, key)
	if idx < 0 {
		return nil, false
	}
	i := idx + len(key)
	for i < len(payload) && payload[i] != ':' {
		i++
	}
	if i >= len(payload) {
		return nil, false
	}
	i++
	for i < len(payload) && isSpace(payload[i]) {
		i++
	}
	if i >= len(payload) || payload[i] != '"' {
		return nil, false
	}
	i++
	start := i
	for i < len(payload) && payload[i] != '"' {
		i++
	}
	if i >= len(payload) {
		return nil, false
	}
	return payload[start:i], true
}

func indexOf(payload []byte, key []byte) int {
	if len(key) == 0 || len(payload) < len(key) {
		return -1
	}
outer:
	for i := 0; i <= len(payload)-len(key); i++ {
		for j := 0; j < len(key); j++ {
			if payload[i+j] != key[j] {
				continue outer
			}
		}
		return i
	}
	return -1
}

func hashBytes(data []byte) uint64 {
	const offset64 = 14695981039346656037
	const prime64 = 1099511628211
	var hash uint64 = offset64
	for i := 0; i < len(data); i++ {
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

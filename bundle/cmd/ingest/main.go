package main

import (
	"context"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"main/bundle/internal/ingest"
	binance "main/bundle/internal/ingest/binance"
	"main/libs/adapter"
	"main/libs/adapter/enum"
	"main/libs/shared/exception"
	"main/libs/shared/uds"
	"main/libs/shared/websocket"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

const (
	defaultUDSDir  = "/tmp/go-hft"
	reqHeaderSize  = 4
	respHeaderSize = 4
	maxUint16      = int(^uint16(0))
	maxTopicName   = 64
)

var (
	dummyDepthPayload = adapter.Depth{}.Encode(nil)
	dummyOrderPayload = adapter.Order{}.Encode(nil)
)

func main() {
	if err := run(); err != nil {
		log.Printf("ingest: %v", err)
		os.Exit(1)
	}
}

func run() error {
	platformFlag := flag.String("platform", "", "platform name for shard socket")
	udsDirFlag := flag.String("uds-dir", defaultUDSDir, "UDS socket directory")
	udsPathFlag := flag.String("uds-path", "", "UDS socket path (optional)")
	flag.Parse()

	udsDir := strings.TrimSpace(*udsDirFlag)
	if udsDir == "" {
		udsDir = defaultUDSDir
	}
	platformName := strings.TrimSpace(*platformFlag)
	if platformName == "" {
		return errors.New("missing platform; use -platform")
	}
	platform, err := parsePlatform(platformName)
	if err != nil {
		return err
	}
	socketPath := strings.TrimSpace(*udsPathFlag)
	if socketPath == "" {
		if err := os.MkdirAll(udsDir, 0o755); err != nil {
			return err
		}
		socketPath = filepath.Join(udsDir, buildSocketFilename(platformName))
	} else {
		socketDir := filepath.Dir(socketPath)
		if err := os.MkdirAll(socketDir, 0o755); err != nil {
			return err
		}
	}

	server, err := uds.NewServer(socketPath)
	if err != nil {
		return err
	}
	if err := server.Listen(); err != nil {
		return err
	}
	defer server.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("marketdata uds listening: %s", socketPath)

	md := ingest.NewUsecase()

	var wg sync.WaitGroup
	go func() {
		<-ctx.Done()
		_ = server.Close()
	}()

	for {
		conn, err := server.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				break
			}
			log.Printf("accept error: %v", err)
			continue
		}
		wg.Add(1)
		go func(c *net.UnixConn) {
			defer wg.Done()
			handleConn(ctx, c, md, platform)
		}(conn)
	}

	wg.Wait()
	return nil
}

func handleConn(ctx context.Context, conn *net.UnixConn, md *ingest.Usecase, serverPlatform enum.Platform) {
	if conn == nil {
		return
	}
	defer conn.Close()

	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()
	defer close(done)

	var (
		buf           []byte
		subscriptions = make(map[string]connSubscription)
		groups        = make(map[adapter.Str64]*connGroup)
		writeMu       sync.Mutex
	)
	defer func() {
		for _, sub := range subscriptions {
			group := groups[sub.apiKey]
			if group == nil {
				continue
			}
			_ = md.Unsubscribe(sub.platform, sub.apiKey, sub.topic, sub.symbol, group.consumer)
		}
		for _, group := range groups {
			if group != nil && group.consumer != nil {
				group.consumer.Close()
			}
		}
	}()

	for {
		req, nextBuf, err := readRequest(conn, buf)
		buf = nextBuf
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
				return
			}
			log.Printf("read request error: %v", err)
			return
		}
		if req.Platform != serverPlatform {
			log.Printf("platform mismatch: got %v want %v", req.Platform, serverPlatform)
			return
		}
		apiKey := req.APIKey
		symbolStr := req.Symbol.String()
		subKey := subscriptionKey(apiKey, req.Topic, symbolStr)
		if existing, exists := subscriptions[subKey]; exists {
			if existing.topic != req.Topic {
				log.Printf("topic mismatch: %v", req.Topic)
				return
			}
			continue
		}

		group := groups[apiKey]
		if group == nil {
			consumer := websocket.NewConsumer(4096, websocket.OverflowDropOldest) // TODO: 根據 topic 來決定 policy
			group = &connGroup{
				platform: serverPlatform,
				apiKey:   apiKey,
				consumer: consumer,
			}
			groups[apiKey] = group
			go runConsumer(ctx, conn, md, group, &writeMu)
		}

		if err := md.Subscribe(ctx, req.Platform, apiKey, req.Topic, req.Symbol, group.consumer); err != nil {
			log.Printf("subscribe error: %v", err)
			return
		}

		subscriptions[subKey] = connSubscription{
			platform:  req.Platform,
			apiKey:    apiKey,
			topic:     req.Topic,
			symbol:    req.Symbol,
			symbolStr: symbolStr,
		}
	}
}

func readRequest(conn *net.UnixConn, buf []byte) (adapter.IngestRequest, []byte, error) {
	var req adapter.IngestRequest
	if conn == nil {
		return req, buf, exception.ErrIngestInvalidRequest
	}

	return req.Decode(buf), buf, nil
}

func writeFull(conn *net.UnixConn, buf []byte) error {
	for len(buf) > 0 {
		n, err := conn.Write(buf)
		if err != nil {
			return err
		}
		buf = buf[n:]
	}
	return nil
}

type connGroup struct {
	platform enum.Platform
	apiKey   adapter.Str64
	consumer *websocket.Consumer
}

type connSubscription struct {
	platform  enum.Platform
	apiKey    adapter.Str64
	topic     enum.Topic
	symbol    adapter.Symbol
	symbolStr string
}

func subscriptionKey(apiKey adapter.Str64, topic enum.Topic, symbol string) string {
	return apiKey.String() + "|" + strconv.Itoa(int(topic)) + "|" + symbol
}

func runConsumer(ctx context.Context, conn *net.UnixConn, md *ingest.Usecase, group *connGroup, writeMu *sync.Mutex) {
	if conn == nil || md == nil || group == nil || group.consumer == nil {
		return
	}

	for {
		frame, ok := group.consumer.Next()
		if !ok {
			return
		}
		topic, symbol, ok := md.Resolve(group.platform, group.apiKey, frame.Topic)
		if !ok {
			frame.Release()
			continue
		}
		var payload []byte
		var err error
		switch group.platform {
		case enum.PlatformBinance:
			payload, err = binance.DecodeMarketDataPayload(topic, symbol, frame.Buf)
		default:
			frame.Release()
			continue
		}
		if err != nil {
			frame.Release()
			continue
		}

		if writeMu != nil {
			writeMu.Lock()
		}
		err = writeResponse(conn, group.platform, topic, symbol[:], payload)
		if writeMu != nil {
			writeMu.Unlock()
		}
		frame.Release()
		if err != nil {
			return
		}
		if ctx.Err() != nil {
			return
		}
	}
}

func writeResponse(conn *net.UnixConn, platform enum.Platform, topic enum.Topic, arg []byte, payload []byte) error {
	if conn == nil {
		return exception.ErrIngestInvalidRequest
	}
	argLen := len(arg)
	if argLen > maxUint16 {
		return exception.ErrIngestInvalidRequest
	}
	total := respHeaderSize + argLen + len(payload)
	buf := make([]byte, total)
	buf[0] = byte(platform)
	buf[1] = byte(topic)
	binary.BigEndian.PutUint16(buf[2:4], uint16(argLen))
	copy(buf[respHeaderSize:respHeaderSize+argLen], arg)
	copy(buf[respHeaderSize+argLen:], payload)
	return writeFull(conn, buf)
}

func buildSocketFilename(platformName string) string {
	platformSegment := sanitizeSegment(platformName, "")
	return "marketdata-" + platformSegment
}

func sanitizeSegment(value string, fallback string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return fallback
	}
	value = strings.ReplaceAll(value, "/", "_")
	value = strings.ReplaceAll(value, "\\", "_")
	value = strings.ReplaceAll(value, " ", "_")
	if len(value) > maxTopicName {
		return value[:maxTopicName] + "-" + strconv.FormatUint(hashBytes([]byte(value)), 16)
	}
	return value
}

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

func parsePlatform(value string) (enum.Platform, error) {
	val := strings.TrimSpace(strings.ToLower(value))
	if val == "" {
		return 0, errors.New("missing platform")
	}
	switch val {
	case "btcc":
		return enum.PlatformBTCC, nil
	case "binance":
		return enum.PlatformBinance, nil
	default:
	}

	if num, err := strconv.Atoi(val); err == nil {
		p := enum.Platform(num)
		if !p.IsAvailable() {
			return 0, fmt.Errorf("unknown platform: %s", value)
		}
		return p, nil
	}
	return 0, fmt.Errorf("unknown platform: %s", value)
}

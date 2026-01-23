package main

import (
	"context"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"main/internal/adapter"
	"main/internal/adapter/enum"
	"main/internal/ingest"
	"main/pkg/uds"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const (
	defaultUDSDir  = "/tmp/go-hft"
	respHeaderSize = 4
	maxUint16      = int(^uint16(0))
	maxTopicName   = 64
)

func main() {
	if err := run(); err != nil {
		log.Printf("cmd_test: %v", err)
		os.Exit(1)
	}
}

func run() error {
	platformFlag := flag.String("platform", "", "platform name for shard socket and request")
	socketTopicFlag := flag.String("socket-topic", "", "socket shard id used in filename")
	reqTopicFlag := flag.String("req-topic", "depth", "request topic: depth|order|<number>")
	baseFlag := flag.String("base", "BTC", "symbol base for arg encoding (required when -arg is empty)")
	quoteFlag := flag.String("quote", "USDT", "symbol quote for arg encoding (required when -arg is empty)")
	apiKeyFlag := flag.String("api-key", "", "api key (optional)")
	udsDirFlag := flag.String("uds-dir", defaultUDSDir, "UDS socket directory")
	udsPathFlag := flag.String("uds-path", "", "UDS socket path (optional)")
	flag.Parse()

	platformName := strings.TrimSpace(*platformFlag)
	if platformName == "" {
		return errors.New("missing platform; use -platform")
	}

	socketTopic := strings.TrimSpace(*socketTopicFlag)
	if socketTopic == "" {
		return errors.New("missing socket topic; use -socket-topic")
	}

	platform, err := parsePlatform(platformName)
	if err != nil {
		return err
	}

	topic, err := parseTopic(*reqTopicFlag)
	if err != nil {
		return err
	}

	symbol := adapter.NewSymbol(*baseFlag, *quoteFlag)
	apiKey := adapter.NewAPIKey(*apiKeyFlag)

	socketPath := strings.TrimSpace(*udsPathFlag)
	if socketPath == "" {
		udsDir := strings.TrimSpace(*udsDirFlag)
		if udsDir == "" {
			udsDir = defaultUDSDir
		}
		socketPath = filepath.Join(udsDir, buildSocketFilename(platformName, socketTopic))
	}

	client, err := uds.NewClient(socketPath)
	if err != nil {
		return err
	}
	conn, err := client.Dial()
	if err != nil {
		return err
	}
	defer conn.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()

	req := adapter.IngestRequest{
		Platform: platform,
		Topic:    topic,
		Symbol:   symbol,
		APIKey:   apiKey,
	}
	payload := req.Encode(nil)
	if err := writeFull(conn, payload); err != nil {
		return err
	}
	resp, err := readResponse(conn)
	if err != nil {
		return err
	}
	log.Printf("subscribe ack platform=%d topic=%d arg=%s payload=%dB", resp.Platform, resp.Topic, resp.Arg, len(resp.Payload))

	for {
		resp, err = readResponse(conn)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		log.Printf("resp platform=%d topic=%d arg=%s payload=%dB", resp.Platform, resp.Topic, resp.Arg, len(resp.Payload))
		if resp.Kind == enum.MarketDataDepth && len(resp.Payload) > 0 {
			depth := adapter.Depth{}.Decode(resp.Payload)
			log.Printf("depth:\n%s", depth.Debug())
		}
	}
}

type response struct {
	Platform enum.Platform
	Topic    enum.Topic
	Arg      string
	Kind     enum.MarketDataKind
	Payload  []byte
}

func readResponse(conn *net.UnixConn) (response, error) {
	var resp response
	if conn == nil {
		return resp, errors.New("nil conn")
	}
	var header [respHeaderSize]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		return resp, err
	}
	resp.Platform = enum.Platform(header[0])
	resp.Topic = enum.Topic(header[1])
	argLen := int(binary.BigEndian.Uint16(header[2:4]))

	var arg []byte
	if argLen > 0 {
		arg = make([]byte, argLen)
		if _, err := io.ReadFull(conn, arg); err != nil {
			return resp, err
		}
	}
	resp.Arg = string(arg)

	kind, ok := ingest.KindFromTopic(resp.Topic)
	if !ok {
		return resp, fmt.Errorf("unknown topic: %d", resp.Topic)
	}
	resp.Kind = kind
	payloadSize, err := payloadSize(kind)
	if err != nil {
		return resp, err
	}
	if payloadSize > 0 {
		resp.Payload = make([]byte, payloadSize)
		if _, err := io.ReadFull(conn, resp.Payload); err != nil {
			return resp, err
		}
	}
	return resp, nil
}

func payloadSize(kind enum.MarketDataKind) (int, error) {
	switch kind {
	case enum.MarketDataDepth:
		return adapter.Depth{}.SizeInByte(), nil
	case enum.MarketDataOrder:
		return adapter.Order{}.SizeInByte(), nil
	default:
		return 0, fmt.Errorf("unknown kind: %d", kind)
	}
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

func parseTopic(value string) (enum.Topic, error) {
	val := strings.TrimSpace(strings.ToLower(value))
	switch val {
	case "depth":
		return enum.TopicDepth, nil
	case "order":
		return enum.TopicOrder, nil
	default:
	}
	if val == "" {
		return 0, errors.New("missing topic")
	}
	if num, err := strconv.Atoi(val); err == nil {
		t := enum.Topic(num)
		if !t.IsAvailable() {
			return 0, fmt.Errorf("unknown topic: %s", value)
		}
		return t, nil
	}
	return 0, fmt.Errorf("unknown topic: %s", value)
}

func buildSocketFilename(platformName string, topic string) string {
	platformSegment := sanitizeSegment(platformName, "platform")
	topicSegment := sanitizeSegment(topic, "topic")
	return "marketdata-" + platformSegment + "-" + topicSegment
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

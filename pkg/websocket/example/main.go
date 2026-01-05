package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"log"
	"main/pkg/metric"
	"main/pkg/websocket"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	pyroscope "github.com/grafana/pyroscope-go"
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
	errFrameTooLarge   = errors.New("websocket: frame exceeds buffer")
)

const maxPayloadLen = int(^uint32(0) >> 1)

type topicSpec struct {
	ID          websocket.TopicID
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
			SymbolUpper: []byte("BTCUSDT"),
			StreamName:  []byte("btcusdt@depth@100ms"),
			Label:       []byte("BTCUSDT"),
		},
	}

	parser := newBinanceTopicParser(topics)
	encoder := newBinanceControlEncoder(topics)

	bufferPool := websocket.DefaultBufferPool()
	framePool := websocket.NewFramePool(bufferPool)
	outboundPool := websocket.NewOutboundPool(bufferPool)
	router := websocket.NewRouter(websocket.FanoutShared, framePool)
	subscriptions := websocket.NewSubscriptions()

	manager, err := websocket.NewManager(websocket.Config{
		Dialer: &binanceDialer{
			Addr: binanceHost + ":" + binancePort,
			Host: binanceHost,
			Path: binancePath,
			TLSConfig: &tls.Config{
				ServerName: binanceHost,
				MinVersion: tls.VersionTLS12,
			},
			DialTimeout: 10 * time.Second,
		},
		Parser:            parser,
		Encoder:           encoder,
		BufferPool:        bufferPool,
		FramePool:         framePool,
		OutboundPool:      outboundPool,
		Router:            router,
		Subscriptions:     subscriptions,
		Fanout:            websocket.FanoutCopy,
		MaxFrameSize:      64 << 10,
		ControlBufferSize: 256,
		WriteQueueSize:    256,
		WriteOverflow:     websocket.OverflowDropOldest,
		PingInterval:      0,
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
		if err := manager.AddConsumer(topic.ID, consumer); err != nil {
			log.Fatalf("subscribe failed: %v", err)
		}
	}

	counts := make([]uint64, maxTopicID(topics)+1)
	go consumeFrames(ctx, consumer, counts, conn)
	go memoryMetric.RunReportSchedule(ctx, statsInterval)

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
	memoryMetric = metric.RuntimeMemoryMetric{}
)

type binanceTopicParser struct {
	streamLookup map[uint64]websocket.TopicID
	symbolLookup map[uint64]websocket.TopicID
}

func newBinanceTopicParser(topics []topicSpec) *binanceTopicParser {
	streamLookup := make(map[uint64]websocket.TopicID, len(topics))
	symbolLookup := make(map[uint64]websocket.TopicID, len(topics))
	for _, topic := range topics {
		streamLookup[hashBytes(topic.StreamName)] = topic.ID
		symbolLookup[hashBytes(topic.SymbolUpper)] = topic.ID
	}
	return &binanceTopicParser{
		streamLookup: streamLookup,
		symbolLookup: symbolLookup,
	}
}

func (p *binanceTopicParser) ParseTopic(payload []byte) (websocket.TopicID, bool) {
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
	nextID     atomic.Uint64
}

func newBinanceControlEncoder(topics []topicSpec) *binanceControlEncoder {
	streamByID := make(map[websocket.TopicID][]byte, len(topics))
	for _, topic := range topics {
		streamByID[topic.ID] = topic.StreamName
	}
	return &binanceControlEncoder{streamByID: streamByID}
}

func (e *binanceControlEncoder) EncodeSubscribe(dst []byte, topic websocket.TopicID) (websocket.MessageType, []byte, error) {
	stream, ok := e.streamByID[topic]
	if !ok {
		return 0, nil, errProtocol
	}
	id := e.nextID.Add(1)
	dst = append(dst, `{"method":"SUBSCRIBE","params":["`...)
	dst = append(dst, stream...)
	dst = append(dst, `"],"id":`...)
	dst = appendUint(dst, id)
	dst = append(dst, '}')
	return websocket.MessageText, dst, nil
}

func (e *binanceControlEncoder) EncodeUnsubscribe(dst []byte, topic websocket.TopicID) (websocket.MessageType, []byte, error) {
	stream, ok := e.streamByID[topic]
	if !ok {
		return 0, nil, errProtocol
	}
	id := e.nextID.Add(1)
	dst = append(dst, `{"method":"UNSUBSCRIBE","params":["`...)
	dst = append(dst, stream...)
	dst = append(dst, `"],"id":`...)
	dst = appendUint(dst, id)
	dst = append(dst, '}')
	return websocket.MessageText, dst, nil
}

type binanceDialer struct {
	Addr        string
	Host        string
	Path        string
	TLSConfig   *tls.Config
	DialTimeout time.Duration
	KeepAlive   time.Duration
}

func (d *binanceDialer) Dial(ctx context.Context) (websocket.Conn, error) {
	if d.KeepAlive == 0 {
		d.KeepAlive = 30 * time.Second
	}
	dialer := net.Dialer{
		Timeout:   d.DialTimeout,
		KeepAlive: d.KeepAlive,
	}
	rawConn, err := dialer.DialContext(ctx, "tcp", d.Addr)
	if err != nil {
		return nil, err
	}
	if tcpConn, ok := rawConn.(*net.TCPConn); ok {
		_ = tcpConn.SetNoDelay(true)
		_ = tcpConn.SetKeepAlive(true)
		_ = tcpConn.SetKeepAlivePeriod(d.KeepAlive)
	}

	tlsConn := tls.Client(rawConn, d.TLSConfig)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		_ = rawConn.Close()
		return nil, err
	}

	wsConn, err := dialWebSocket(ctx, tlsConn, d.Host, d.Path)
	if err != nil {
		_ = tlsConn.Close()
		return nil, err
	}
	return wsConn, nil
}

type wsConn struct {
	conn   net.Conn
	reader *bufio.Reader
	mask   uint32
	mu     sync.Mutex
}

func dialWebSocket(ctx context.Context, conn net.Conn, host, path string) (*wsConn, error) {
	key, err := newWebSocketKey()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+host+path, nil)
	if err != nil {
		return nil, err
	}
	req.Host = host
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", key)
	req.Header.Set("Sec-WebSocket-Version", "13")

	if err := req.Write(conn); err != nil {
		return nil, err
	}

	reader := bufio.NewReaderSize(conn, 32<<10)
	resp, err := http.ReadResponse(reader, req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		return nil, errHandshakeFailed
	}
	if !headerContainsToken(resp.Header.Get("Upgrade"), "websocket") {
		return nil, errHandshakeFailed
	}
	if !headerContainsToken(resp.Header.Get("Connection"), "upgrade") {
		return nil, errHandshakeFailed
	}
	if !validateAcceptKey(key, resp.Header.Get("Sec-WebSocket-Accept")) {
		return nil, errHandshakeFailed
	}

	return &wsConn{
		conn:   conn,
		reader: reader,
		mask:   seedMask(),
	}, nil
}

func (c *wsConn) Read(ctx context.Context, dst []byte) (int, websocket.MessageType, error) {
	var (
		total   int
		msgType websocket.MessageType
	)
	for {
		fin, opcode, masked, maskKey, payloadLen, err := c.readHeader(ctx)
		if err != nil {
			return 0, 0, err
		}

		if opcode == opPing || opcode == opPong || opcode == opClose {
			var ctrl [125]byte
			if payloadLen > len(ctrl) {
				return 0, 0, errProtocol
			}
			if err := c.readPayload(ctx, ctrl[:payloadLen], masked, maskKey); err != nil {
				return 0, 0, err
			}
			if opcode == opPing {
				_ = c.writeFrame(context.Background(), opPong, ctrl[:payloadLen])
			}
			if opcode == opClose {
				return 0, 0, io.EOF
			}
			continue
		}

		if require := total + payloadLen; require > len(dst) {
			return 0, 0, errFrameTooLarge
		}

		if err := c.readPayload(ctx, dst[total:total+payloadLen], masked, maskKey); err != nil {
			return 0, 0, err
		}

		if opcode != opContinuation {
			msgType = opcodeToMessageType(opcode)
			if msgType == 0 {
				return 0, 0, errProtocol
			}
		} else if msgType == 0 {
			return 0, 0, errProtocol
		}

		total += payloadLen
		if fin {
			return total, msgType, nil
		}
	}
}

func (c *wsConn) Write(ctx context.Context, msgType websocket.MessageType, payload []byte) error {
	opcode := messageTypeToOpcode(msgType)
	if opcode == 0 {
		return errProtocol
	}
	return c.writeFrame(ctx, opcode, payload)
}

func (c *wsConn) Close(code websocket.CloseCode, reason string) error {
	payload := makeClosePayload(code, reason)
	_ = c.writeFrame(context.Background(), opClose, payload)
	return c.conn.Close()
}

func (c *wsConn) readHeader(ctx context.Context) (fin bool, opcode byte, masked bool, maskKey [4]byte, payloadLen int, err error) {
	if err = c.setReadDeadline(ctx); err != nil {
		return
	}
	var header [2]byte
	if _, err = io.ReadFull(c.reader, header[:]); err != nil {
		return
	}
	fin = header[0]&0x80 != 0
	if header[0]&0x70 != 0 {
		err = errProtocol
		return
	}
	opcode = header[0] & 0x0f
	masked = header[1]&0x80 != 0
	payloadLen = int(header[1] & 0x7f)
	if payloadLen == 126 {
		var ext [2]byte
		if _, err = io.ReadFull(c.reader, ext[:]); err != nil {
			return
		}
		payloadLen = int(binary.BigEndian.Uint16(ext[:]))
	} else if payloadLen == 127 {
		var ext [8]byte
		if _, err = io.ReadFull(c.reader, ext[:]); err != nil {
			return
		}
		if ext[0]&0x80 != 0 {
			err = errProtocol
			return
		}
		length64 := binary.BigEndian.Uint64(ext[:])
		if length64 > uint64(maxPayloadLen) {
			err = errFrameTooLarge
			return
		}
		payloadLen = int(length64)
	}

	if masked {
		if _, err = io.ReadFull(c.reader, maskKey[:]); err != nil {
			return
		}
	}
	if opcode == opPing || opcode == opPong || opcode == opClose {
		if !fin || payloadLen > 125 {
			err = errProtocol
			return
		}
	}
	return
}

func (c *wsConn) readPayload(ctx context.Context, dst []byte, masked bool, maskKey [4]byte) error {
	if err := c.setReadDeadline(ctx); err != nil {
		return err
	}
	if _, err := io.ReadFull(c.reader, dst); err != nil {
		return err
	}
	if masked {
		for i := 0; i < len(dst); i++ {
			dst[i] ^= maskKey[i&3]
		}
	}
	return nil
}

func (c *wsConn) writeFrame(ctx context.Context, opcode byte, payload []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.setWriteDeadline(ctx); err != nil {
		return err
	}
	var header [14]byte
	header[0] = 0x80 | opcode
	mask := c.nextMask()
	maskKey := [4]byte{byte(mask), byte(mask >> 8), byte(mask >> 16), byte(mask >> 24)}

	n := buildLengthHeader(header[:], len(payload), true, maskKey)
	if _, err := c.conn.Write(header[:n]); err != nil {
		return err
	}
	if len(payload) == 0 {
		return nil
	}
	for i := 0; i < len(payload); i++ {
		payload[i] ^= maskKey[i&3]
	}
	if _, err := c.conn.Write(payload); err != nil {
		return err
	}
	return nil
}

func (c *wsConn) setReadDeadline(ctx context.Context) error {
	return setDeadline(ctx, c.conn.SetReadDeadline)
}

func (c *wsConn) setWriteDeadline(ctx context.Context) error {
	return setDeadline(ctx, c.conn.SetWriteDeadline)
}

func (c *wsConn) nextMask() uint32 {
	c.mask ^= c.mask << 13
	c.mask ^= c.mask >> 17
	c.mask ^= c.mask << 5
	if c.mask == 0 {
		c.mask = 0x9e3779b9
	}
	return c.mask
}

func seedMask() uint32 {
	n := uint32(time.Now().UnixNano())
	if n == 0 {
		return 0x9e3779b9
	}
	return n
}

const (
	opContinuation = 0x0
	opText         = 0x1
	opBinary       = 0x2
	opClose        = 0x8
	opPing         = 0x9
	opPong         = 0xA
)

func messageTypeToOpcode(msgType websocket.MessageType) byte {
	switch msgType {
	case websocket.MessageText:
		return opText
	case websocket.MessageBinary:
		return opBinary
	case websocket.MessagePing:
		return opPing
	case websocket.MessagePong:
		return opPong
	case websocket.MessageClose:
		return opClose
	default:
		return 0
	}
}

func opcodeToMessageType(opcode byte) websocket.MessageType {
	switch opcode {
	case opText:
		return websocket.MessageText
	case opBinary:
		return websocket.MessageBinary
	default:
		return 0
	}
}

func buildLengthHeader(dst []byte, payloadLen int, masked bool, maskKey [4]byte) int {
	n := 2
	if payloadLen <= 125 {
		dst[1] = byte(payloadLen)
	} else if payloadLen <= 0xffff {
		dst[1] = 126
		binary.BigEndian.PutUint16(dst[2:4], uint16(payloadLen))
		n += 2
	} else {
		dst[1] = 127
		binary.BigEndian.PutUint64(dst[2:10], uint64(payloadLen))
		n += 8
	}
	if masked {
		dst[1] |= 0x80
		copy(dst[n:n+4], maskKey[:])
		n += 4
	}
	return n
}

func setDeadline(ctx context.Context, set func(time.Time) error) error {
	if ctx == nil {
		return set(time.Time{})
	}
	if deadline, ok := ctx.Deadline(); ok {
		return set(deadline)
	}
	if ctx.Err() != nil {
		return set(time.Now())
	}
	return set(time.Time{})
}

func newWebSocketKey() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf[:]), nil
}

func validateAcceptKey(key, accept string) bool {
	h := sha1.New()
	io.WriteString(h, key)
	io.WriteString(h, "258EAFA5-E914-47DA-95CA-C5AB0DC85B11")
	sum := h.Sum(nil)
	expected := base64.StdEncoding.EncodeToString(sum[:])
	return accept == expected
}

// headerContainsToken
//
// token must be lower case
func headerContainsToken(headerValue, token string) bool {
	if headerValue == "" {
		return false
	}
	start := 0
	for i := 0; i <= len(headerValue); i++ {
		if i == len(headerValue) || headerValue[i] == ',' {
			part := headerValue[start:i]
			if trimLowerEqual(part, token) {
				return true
			}
			start = i + 1
		}
	}
	return false
}

func trimLowerEqual(value string, tokenLower string) bool {
	i := 0
	j := len(value) - 1
	for i <= j && isSpace(value[i]) {
		i++
	}
	for j >= i && isSpace(value[j]) {
		j--
	}
	if j < i {
		return false
	}
	if j-i+1 != len(tokenLower) {
		return false
	}
	for k := 0; k < len(tokenLower); k++ {
		if toLowerByte(value[i+k]) != tokenLower[k] {
			return false
		}
	}
	return true
}

func toLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		b[i] = toLowerByte(s[i])
	}
	return string(b)
}

func toLowerByte(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

var (
	keyStream   = []byte(`"stream"`)
	keySymbol   = []byte(`"s"`)
	keyUpdateID = []byte(`"u"`)
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

func parseUintField(payload []byte, key []byte) (uint64, bool) {
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
	var buf [20]byte
	i := len(buf)
	if v == 0 {
		i--
		buf[i] = '0'
		return append(dst, buf[i:]...)
	}
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return append(dst, buf[i:]...)
}

func makeClosePayload(code websocket.CloseCode, reason string) []byte {
	if code == 0 {
		return nil
	}
	var buf [2]byte
	binary.BigEndian.PutUint16(buf[:], uint16(code))
	if reason == "" {
		return buf[:]
	}
	payload := make([]byte, 0, 2+len(reason))
	payload = append(payload, buf[:]...)
	payload = append(payload, reason...)
	return payload
}

type latencyHistogram struct {
	bounds  []time.Duration
	counts  []uint64
	scratch []uint64
}

func newLatencyHistogram() *latencyHistogram {
	bounds := []time.Duration{
		50 * time.Microsecond,
		100 * time.Microsecond,
		250 * time.Microsecond,
		500 * time.Microsecond,
		1 * time.Millisecond,
		2 * time.Millisecond,
		5 * time.Millisecond,
		10 * time.Millisecond,
		20 * time.Millisecond,
		50 * time.Millisecond,
		100 * time.Millisecond,
		200 * time.Millisecond,
		500 * time.Millisecond,
		1 * time.Second,
		2 * time.Second,
		5 * time.Second,
	}
	counts := make([]uint64, len(bounds)+1)
	return &latencyHistogram{
		bounds:  bounds,
		counts:  counts,
		scratch: make([]uint64, len(counts)),
	}
}

func (h *latencyHistogram) Add(value time.Duration) {
	if h == nil {
		return
	}
	idx := len(h.bounds)
	for i, bound := range h.bounds {
		if value <= bound {
			idx = i
			break
		}
	}
	atomic.AddUint64(&h.counts[idx], 1)
}

func (h *latencyHistogram) SnapshotP99() (total uint64, p99 time.Duration, overflow uint64) {
	if h == nil {
		return 0, 0, 0
	}
	for i := range h.counts {
		h.scratch[i] = atomic.LoadUint64(&h.counts[i])
		total += h.scratch[i]
	}
	if total == 0 {
		return 0, 0, 0
	}
	target := (total*99 + 99) / 100
	var cum uint64
	for i, count := range h.scratch {
		cum += count
		if cum >= target {
			if i < len(h.bounds) {
				p99 = h.bounds[i]
			} else {
				p99 = h.bounds[len(h.bounds)-1]
			}
			break
		}
	}
	overflow = h.scratch[len(h.scratch)-1]
	return total, p99, overflow
}

func (h *latencyHistogram) Reset() {
	if h == nil {
		return
	}
	for i := range h.counts {
		atomic.StoreUint64(&h.counts[i], 0)
	}
}

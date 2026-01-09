package websocket

import (
	"bufio"
	"context"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"math/rand"
	"net"
	"net/http"
	"sync"
	"time"
)

var (
	errFrameTooLarge   = errors.New("frame exceeds buffer")
	errHandshakeFailed = errors.New("websocket: handshake failed")
	errProtocol        = errors.New("websocket: protocol error")
)

const (
	DefaultDialerTimeout   = 10 * time.Second
	DefaultDialerKeepAlive = 30 * time.Second
)

const maxPayloadLen = int(^uint32(0) >> 1)

type dialer struct {
	Addr        string
	Host        string
	Path        string
	TLSConfig   *tls.Config
	DialTimeout time.Duration
	KeepAlive   time.Duration
}

func NewDialer(ctx context.Context, host string, port string, path string) Dialer {
	return &dialer{
		Addr: host + ":" + port,
		Host: host,
		Path: path,
		TLSConfig: &tls.Config{
			ServerName: host,
			MinVersion: tls.VersionTLS12,
		},
		DialTimeout: 10 * time.Second,
	}
}

func (d *dialer) Dial(ctx context.Context) (Conn, error) {
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
	conn    net.Conn
	reader  *bufio.Reader
	mask    uint32
	mu      sync.Mutex
	pending *pendingRead
}

type pendingRead struct {
	data       []byte
	msgType    MessageType
	fin        bool
	masked     bool
	maskKey    [4]byte
	payloadLen int
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

func (c *wsConn) Read(ctx context.Context, dst []byte) (int, MessageType, error) {
	var (
		total   int
		msgType MessageType
	)
	if c.pending != nil {
		pending := c.pending
		need := len(pending.data) + pending.payloadLen
		if len(dst) < need {
			return 0, 0, errFrameTooLarge
		}
		if len(pending.data) > 0 {
			copy(dst, pending.data)
		}
		total = len(pending.data)
		msgType = pending.msgType
		c.pending = nil
		if err := c.readPayload(ctx, dst[total:total+pending.payloadLen], pending.masked, pending.maskKey); err != nil {
			return 0, 0, err
		}
		total += pending.payloadLen
		if pending.fin {
			return total, msgType, nil
		}
	}
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

		if opcode != opContinuation {
			msgType = opcodeToMessageType(opcode)
			if msgType == 0 {
				return 0, 0, errProtocol
			}
		} else if msgType == 0 {
			return 0, 0, errProtocol
		}

		if require := total + payloadLen; require > len(dst) {
			c.stashPending(dst[:total], msgType, fin, masked, maskKey, payloadLen)
			return 0, 0, errFrameTooLarge
		}

		if err := c.readPayload(ctx, dst[total:total+payloadLen], masked, maskKey); err != nil {
			return 0, 0, err
		}

		total += payloadLen
		if fin {
			return total, msgType, nil
		}
	}
}

func (c *wsConn) stashPending(data []byte, msgType MessageType, fin bool, masked bool, maskKey [4]byte, payloadLen int) {
	if c == nil {
		return
	}
	var copied []byte
	if len(data) > 0 {
		copied = make([]byte, len(data))
		copy(copied, data)
	}
	c.pending = &pendingRead{
		data:       copied,
		msgType:    msgType,
		fin:        fin,
		masked:     masked,
		maskKey:    maskKey,
		payloadLen: payloadLen,
	}
}

func (c *wsConn) Write(ctx context.Context, msgType MessageType, payload []byte) error {
	opcode := messageTypeToOpcode(msgType)
	if opcode == 0 {
		return errProtocol
	}
	return c.writeFrame(ctx, opcode, payload)
}

func (c *wsConn) Close(code CloseCode, reason string) error {
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
	switch payloadLen {
	case 126:
		var ext [2]byte
		if _, err = io.ReadFull(c.reader, ext[:]); err != nil {
			return
		}
		payloadLen = int(binary.BigEndian.Uint16(ext[:]))
	case 127:
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

func messageTypeToOpcode(msgType MessageType) byte {
	switch msgType {
	case MessageText:
		return opText
	case MessageBinary:
		return opBinary
	case MessagePing:
		return opPing
	case MessagePong:
		return opPong
	case MessageClose:
		return opClose
	default:
		return 0
	}
}

func opcodeToMessageType(opcode byte) MessageType {
	switch opcode {
	case opText:
		return MessageText
	case opBinary:
		return MessageBinary
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

func toLowerByte(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

func makeClosePayload(code CloseCode, reason string) []byte {
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

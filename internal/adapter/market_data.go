package adapter

import (
	"encoding/binary"
	"main/internal/adapter/enum"
	"main/pkg/exception"
)

const (
	marketDataReqHeaderSize = 1 + 1 + 2
	maxUint16               = int(^uint16(0))
)

// MarketDataRequest is the minimal UDS request format.
// Arg slices alias the input buffer; copy if you need to retain them.
type MarketDataRequest struct {
	Platform enum.Platform
	Topic    enum.Topic
	Arg      []byte
}

type MarketDataArgDepth struct {
	Symbol Symbol
}

type MarketDataArgOrder struct {
	Symbol Symbol
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
		return req, 0, exception.ErrInvalidMarketDataRequest
	}
	req.Platform = enum.Platform(src[0])
	req.Topic = enum.Topic(src[1])
	argLen := int(binary.BigEndian.Uint16(src[2:4]))
	total := marketDataReqHeaderSize + argLen
	if argLen < 0 || total > len(src) {
		return req, 0, exception.ErrInvalidMarketDataRequest
	}
	if !req.Platform.IsAvailable() || !req.Topic.IsAvailable() {
		return req, 0, exception.ErrInvalidMarketDataRequest
	}
	if argLen > 0 {
		req.Arg = src[marketDataReqHeaderSize : marketDataReqHeaderSize+argLen]
	}
	return req, total, nil
}

// EncodeMarketDataRequest serializes a request into dst.
func EncodeMarketDataRequest(dst []byte, req MarketDataRequest) ([]byte, error) {
	if !req.Platform.IsAvailable() || !req.Topic.IsAvailable() {
		return nil, exception.ErrInvalidMarketDataRequest
	}

	argLen := len(req.Arg)
	if argLen > maxUint16 {
		return nil, exception.ErrInvalidMarketDataRequest
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
		return arg, exception.ErrInvalidMarketDataRequest
	}
	arg.Symbol = Symbol(binary.BigEndian.Uint16(src[0:2]))
	return arg, nil
}

// EncodeMarketDataArgDepth serializes a depth arg payload.
func EncodeMarketDataArgDepth(dst []byte, arg MarketDataArgDepth) ([]byte, error) {
	total := 2
	if total > maxUint16 {
		return nil, exception.ErrInvalidMarketDataRequest
	}
	if cap(dst) < total {
		dst = make([]byte, total)
	} else {
		dst = dst[:total]
	}
	binary.BigEndian.PutUint16(dst[0:2], uint16(arg.Symbol))
	return dst, nil
}

// DecodeMarketDataArgOrder parses an order arg payload.
// Format: [0:2] symbol (uint16, big endian) + apiKey bytes.
func DecodeMarketDataArgOrder(src []byte) (MarketDataArgOrder, error) {
	var arg MarketDataArgOrder
	if len(src) < 2 {
		return arg, exception.ErrInvalidMarketDataRequest
	}
	arg.Symbol = Symbol(binary.BigEndian.Uint16(src[0:2]))
	if len(src) > 2 {
		arg.APIKey = src[2:]
	}
	return arg, nil
}

// EncodeMarketDataArgOrder serializes an order arg payload.
func EncodeMarketDataArgOrder(dst []byte, arg MarketDataArgOrder) ([]byte, error) {
	total := 2 + len(arg.APIKey)
	if total > maxUint16 {
		return nil, exception.ErrInvalidMarketDataRequest
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

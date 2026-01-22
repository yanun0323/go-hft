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
type MarketDataRequest struct {
	Platform enum.Platform
	Topic    enum.Topic
	Symbol   Symbol
	APIKey   []byte
}

// DecodeMarketDataRequest parses a request from a byte buffer.
// Format:
// [0] platform (uint8)
// [1] topic (uint8)
// [2:4] apiKeyLen (uint16, big endian)
// [4:4+SymbolCap] symbol
// [4+SymbolCap:] apiKey
func DecodeMarketDataRequest(src []byte) (MarketDataRequest, int, error) {
	var req MarketDataRequest
	if len(src) < marketDataReqHeaderSize+SymbolCap {
		return req, 0, exception.ErrInvalidMarketDataRequest
	}
	req.Platform = enum.Platform(src[0])
	req.Topic = enum.Topic(src[1])
	apiKeyLen := int(binary.BigEndian.Uint16(src[2:4]))
	total := marketDataReqHeaderSize + SymbolCap + apiKeyLen
	if apiKeyLen < 0 || total > len(src) {
		return req, 0, exception.ErrInvalidMarketDataRequest
	}
	if !req.Platform.IsAvailable() || !req.Topic.IsAvailable() {
		return req, 0, exception.ErrInvalidMarketDataRequest
	}
	copy(req.Symbol[:], src[marketDataReqHeaderSize:marketDataReqHeaderSize+SymbolCap])
	if apiKeyLen > 0 {
		req.APIKey = src[marketDataReqHeaderSize+SymbolCap : total]
	}
	return req, total, nil
}

// EncodeMarketDataRequest serializes a request into dst.
func EncodeMarketDataRequest(dst []byte, req MarketDataRequest) ([]byte, error) {
	if !req.Platform.IsAvailable() || !req.Topic.IsAvailable() {
		return nil, exception.ErrInvalidMarketDataRequest
	}

	apiKeyLen := len(req.APIKey)
	if apiKeyLen > maxUint16 {
		return nil, exception.ErrInvalidMarketDataRequest
	}
	total := marketDataReqHeaderSize + SymbolCap + apiKeyLen
	if cap(dst) < total {
		dst = make([]byte, total)
	} else {
		dst = dst[:total]
	}
	dst[0] = byte(req.Platform)
	dst[1] = byte(req.Topic)
	binary.BigEndian.PutUint16(dst[2:4], uint16(apiKeyLen))
	copy(dst[marketDataReqHeaderSize:marketDataReqHeaderSize+SymbolCap], req.Symbol[:])
	copy(dst[marketDataReqHeaderSize+SymbolCap:], req.APIKey)
	return dst, nil
}

package ingest

import (
	"bytes"
	"testing"

	"main/internal/adapter"
	"main/internal/adapter/enum"
)

func TestMarketDataRequestEncodeDecodeRoundTrip(t *testing.T) {
	orig := MarketDataRequest{
		Platform: enum.PlatformBinance,
		Topic:    enum.TopicDepth,
		Arg:      []byte{0x01, 0x02, 0x03},
	}

	encoded, err := EncodeMarketDataRequest(nil, orig)
	if err != nil {
		t.Fatalf("encode request: %v", err)
	}
	decoded, n, err := DecodeMarketDataRequest(encoded)
	if err != nil {
		t.Fatalf("decode request: %v", err)
	}
	if n != len(encoded) {
		t.Fatalf("decoded size mismatch: got %d want %d", n, len(encoded))
	}
	if decoded.Platform != orig.Platform || decoded.Topic != orig.Topic {
		t.Fatalf("decoded header mismatch: got %+v want %+v", decoded, orig)
	}
	if !bytes.Equal(decoded.Arg, orig.Arg) {
		t.Fatalf("decoded arg mismatch: got %x want %x", decoded.Arg, orig.Arg)
	}
}

func TestMarketDataArgDepthEncodeDecodeRoundTrip(t *testing.T) {
	orig := MarketDataArgDepth{
		Symbol:   adapter.Symbol(42),
		Interval: []byte("100ms"),
	}

	encoded, err := EncodeMarketDataArgDepth(nil, orig)
	if err != nil {
		t.Fatalf("encode depth arg: %v", err)
	}
	decoded, err := DecodeMarketDataArgDepth(encoded)
	if err != nil {
		t.Fatalf("decode depth arg: %v", err)
	}
	if decoded.Symbol != orig.Symbol {
		t.Fatalf("depth symbol mismatch: got %d want %d", decoded.Symbol, orig.Symbol)
	}
	if !bytes.Equal(decoded.Interval, orig.Interval) {
		t.Fatalf("depth interval mismatch: got %s want %s", decoded.Interval, orig.Interval)
	}
}

func TestMarketDataArgOrderEncodeDecodeRoundTrip(t *testing.T) {
	orig := MarketDataArgOrder{
		Symbol: adapter.Symbol(7),
		APIKey: []byte("key-123"),
	}

	encoded, err := EncodeMarketDataArgOrder(nil, orig)
	if err != nil {
		t.Fatalf("encode order arg: %v", err)
	}
	decoded, err := DecodeMarketDataArgOrder(encoded)
	if err != nil {
		t.Fatalf("decode order arg: %v", err)
	}
	if decoded.Symbol != orig.Symbol {
		t.Fatalf("order symbol mismatch: got %d want %d", decoded.Symbol, orig.Symbol)
	}
	if !bytes.Equal(decoded.APIKey, orig.APIKey) {
		t.Fatalf("order api key mismatch: got %s want %s", decoded.APIKey, orig.APIKey)
	}
}

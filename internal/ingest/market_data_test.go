package ingest

import (
	"bytes"
	"testing"

	"main/internal/adapter"
	"main/internal/adapter/enum"
)

func TestMarketDataRequestEncodeDecodeRoundTrip(t *testing.T) {
	orig := adapter.MarketDataRequest{
		Platform: enum.PlatformBinance,
		Topic:    enum.TopicDepth,
		Symbol:   adapter.NewSymbol("BTC", "USDT"),
		APIKey:   []byte("key-123"),
	}

	encoded, err := adapter.EncodeMarketDataRequest(nil, orig)
	if err != nil {
		t.Fatalf("encode request: %v", err)
	}
	decoded, n, err := adapter.DecodeMarketDataRequest(encoded)
	if err != nil {
		t.Fatalf("decode request: %v", err)
	}
	if n != len(encoded) {
		t.Fatalf("decoded size mismatch: got %d want %d", n, len(encoded))
	}
	if decoded.Platform != orig.Platform || decoded.Topic != orig.Topic {
		t.Fatalf("decoded header mismatch: got %+v want %+v", decoded, orig)
	}
	if decoded.Symbol != orig.Symbol {
		t.Fatalf("decoded symbol mismatch: got %v want %v", decoded.Symbol, orig.Symbol)
	}
	if !bytes.Equal(decoded.APIKey, orig.APIKey) {
		t.Fatalf("decoded api key mismatch: got %x want %x", decoded.APIKey, orig.APIKey)
	}
}

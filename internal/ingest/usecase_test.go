package ingest

import (
	"testing"

	"main/libs/adapter"
	"main/libs/adapter/enum"
)

func TestMarketDataRequestEncodeDecodeRoundTrip(t *testing.T) {
	orig := adapter.IngestRequest{
		Platform: enum.PlatformBinance,
		Topic:    enum.TopicDepth,
		Symbol:   adapter.NewSymbol("BTC", "USDT"),
		APIKey:   adapter.NewStr64("key-123"),
	}

	encoded := orig.Encode(nil)
	decoded := orig.Decode(encoded)
	if decoded.Platform != orig.Platform || decoded.Topic != orig.Topic {
		t.Fatalf("decoded header mismatch: got %+v want %+v", decoded, orig)
	}
	if decoded.Symbol != orig.Symbol {
		t.Fatalf("decoded symbol mismatch: got %v want %v", decoded.Symbol, orig.Symbol)
	}
	if decoded.APIKey != orig.APIKey {
		t.Fatalf("decoded api key mismatch: got \n%b want \n%b", decoded.APIKey, orig.APIKey)
	}
}

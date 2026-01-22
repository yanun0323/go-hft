package binance

import (
	"testing"

	"main/internal/adapter"
	"main/internal/adapter/enum"
)

func TestDecodeMarketDataPayloadDepthBinance(t *testing.T) {
	symbol := adapter.NewSymbol("BTC", "USDT")
	payload := []byte(`{"e":"depthUpdate","E":1700000000123,"s":"BTCUSDT","U":1,"u":2,"b":[["100.10","1.5"],["99","2"]],"a":[["100.20","3"]]}`)
	encoded, err := DecodeMarketDataPayload(enum.TopicDepth, symbol, payload)
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	decoded := (adapter.Depth{}).Decode(encoded)
	if decoded.Symbol != adapter.NewSymbol("BTC", "USDT") {
		t.Fatalf("symbol mismatch: got %d want %d", decoded.Symbol, 42)
	}
	if decoded.Platform != enum.PlatformBinance {
		t.Fatalf("platform mismatch: got %d", decoded.Platform)
	}
	if decoded.EventTsNano != 1700000000123*1_000_000 {
		t.Fatalf("event time mismatch: got %d", decoded.EventTsNano)
	}
	if decoded.BidsLength != 2 || decoded.AsksLength != 1 {
		t.Fatalf("depth lengths mismatch: bids=%d asks=%d", decoded.BidsLength, decoded.AsksLength)
	}
	bid0 := decoded.Bids[0]
	if bid0.Price != (adapter.Price{Integer: 10010, Scale: 2}) || bid0.Quantity != (adapter.Quantity{Integer: 15, Scale: 1}) {
		t.Fatalf("bid0 mismatch: %+v", bid0)
	}
	bid1 := decoded.Bids[1]
	if bid1.Price != (adapter.Price{Integer: 99, Scale: 0}) || bid1.Quantity != (adapter.Quantity{Integer: 2, Scale: 0}) {
		t.Fatalf("bid1 mismatch: %+v", bid1)
	}
	ask0 := decoded.Asks[0]
	if ask0.Price != (adapter.Price{Integer: 10020, Scale: 2}) || ask0.Quantity != (adapter.Quantity{Integer: 3, Scale: 0}) {
		t.Fatalf("ask0 mismatch: %+v", ask0)
	}
	if decoded.RecvTsNano == 0 {
		t.Fatalf("recv timestamp missing")
	}
}

func TestDecodeMarketDataPayloadOrderBinance(t *testing.T) {
	symbol := adapter.NewSymbol("BTC", "USDT")
	payload := []byte(`{"e":"executionReport","E":1700000000123,"s":"BTCUSDT","i":12345,"S":"BUY","o":"LIMIT","f":"GTC","X":"NEW","q":"2.5","p":"30000.01","z":"0.5","T":1700000000456}`)
	encoded, err := DecodeMarketDataPayload(enum.TopicOrder, symbol, payload)
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	decoded := (adapter.Order{}).Decode(encoded)
	if decoded.ID != 12345 {
		t.Fatalf("order id mismatch: got %d", decoded.ID)
	}
	if decoded.Symbol != adapter.NewSymbol("BTC", "USDT") {
		t.Fatalf("symbol mismatch: got %d", decoded.Symbol)
	}
	if decoded.Platform != enum.PlatformBinance {
		t.Fatalf("platform mismatch: got %d", decoded.Platform)
	}
	if decoded.EventTsNano != 1700000000123*1_000_000 {
		t.Fatalf("event time mismatch: got %d", decoded.EventTsNano)
	}
	if decoded.UpdatedTime != 1700000000456*1_000_000 {
		t.Fatalf("update time mismatch: got %d", decoded.UpdatedTime)
	}
	if decoded.Side != enum.OrderSideBuy {
		t.Fatalf("side mismatch: got %d", decoded.Side)
	}
	if decoded.Type != enum.OrderTypeLimit {
		t.Fatalf("type mismatch: got %d", decoded.Type)
	}
	if decoded.Status != enum.OrderStatusPlaced {
		t.Fatalf("status mismatch: got %d", decoded.Status)
	}
	if decoded.TimeInForce != enum.OrderTimeInForceGTC {
		t.Fatalf("time in force mismatch: got %d", decoded.TimeInForce)
	}
	if decoded.Price != (adapter.Price{Integer: 3000001, Scale: 2}) {
		t.Fatalf("price mismatch: %+v", decoded.Price)
	}
	if decoded.Quantity != (adapter.Quantity{Integer: 25, Scale: 1}) {
		t.Fatalf("quantity mismatch: %+v", decoded.Quantity)
	}
	if decoded.LeftQuantity != (adapter.Quantity{Integer: 20, Scale: 1}) {
		t.Fatalf("left quantity mismatch: %+v", decoded.LeftQuantity)
	}
	if decoded.RecvTsNano == 0 {
		t.Fatalf("recv timestamp missing")
	}
}

package adapter

import (
	"testing"

	"main/libs/adapter/enum"
)

func TestDepthEncodeDecodeRoundTrip(t *testing.T) {
	orig := Depth{
		Symbol:      NewSymbol("BTC", "USDT"),
		EventTsNano: 1700000000123,
		RecvTsNano:  1700000000456,
		Platform:    enum.PlatformBinance,
		BidsLength:  2,
		AsksLength:  3,
	}
	orig.Bids[0] = DepthRow{Price: Decimal{Integer: 101}, Quantity: Decimal{Integer: 5}}
	orig.Bids[1] = DepthRow{Price: Decimal{Integer: 100}, Quantity: Decimal{Integer: 9}}
	orig.Bids[5] = DepthRow{Price: Decimal{Integer: 95}, Quantity: Decimal{Integer: 1}}
	orig.Asks[0] = DepthRow{Price: Decimal{Integer: 102}, Quantity: Decimal{Integer: 4}}
	orig.Asks[2] = DepthRow{Price: Decimal{Integer: 105}, Quantity: Decimal{Integer: 7}}

	encoded := orig.Encode(nil)
	decoded := (Depth{}).Decode(encoded)

	if decoded != orig {
		t.Fatalf("depth round-trip mismatch: got %+v want %+v", decoded, orig)
	}
}

package adapter

import (
	"testing"

	"main/internal/adapter/enum"
)

func TestDepthEncodeDecodeRoundTrip(t *testing.T) {
	orig := Depth{
		SymbolID:    42,
		EventTsNano: 1700000000123,
		RecvTsNano:  1700000000456,
		Platform:    enum.PlatformBinance,
		BidsLength:  2,
		AsksLength:  3,
	}
	orig.Bids[0] = DepthRow{Price: Price{Integer: 101}, Quantity: Quantity{Integer: 5}}
	orig.Bids[1] = DepthRow{Price: Price{Integer: 100}, Quantity: Quantity{Integer: 9}}
	orig.Bids[5] = DepthRow{Price: Price{Integer: 95}, Quantity: Quantity{Integer: 1}}
	orig.Asks[0] = DepthRow{Price: Price{Integer: 102}, Quantity: Quantity{Integer: 4}}
	orig.Asks[2] = DepthRow{Price: Price{Integer: 105}, Quantity: Quantity{Integer: 7}}

	encoded := orig.Encode(nil)
	decoded := (Depth{}).Decode(encoded)

	if decoded != orig {
		t.Fatalf("depth round-trip mismatch: got %+v want %+v", decoded, orig)
	}
}

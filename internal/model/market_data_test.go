package model

import (
	"testing"

	"main/internal/model/enum"
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
	orig.Bids[0] = DepthRow{Price: 101, Quantity: 5}
	orig.Bids[1] = DepthRow{Price: 100, Quantity: 9}
	orig.Bids[5] = DepthRow{Price: 95, Quantity: 1}
	orig.Asks[0] = DepthRow{Price: 102, Quantity: 4}
	orig.Asks[2] = DepthRow{Price: 105, Quantity: 7}

	encoded := orig.Encode(nil)
	decoded := (Depth{}).Decode(encoded)

	if decoded != orig {
		t.Fatalf("depth round-trip mismatch: got %+v want %+v", decoded, orig)
	}
}

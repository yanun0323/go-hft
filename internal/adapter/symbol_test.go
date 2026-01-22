package adapter

import (
	"strings"
	"testing"
)

func TestSymbol(t *testing.T) {
	type SymbolContainer struct {
		Base, Quote, Memo string
	}

	testCases := []struct {
		desc     string
		input    SymbolContainer
		expected SymbolContainer
	}{
		{
			"BTC USDT",
			SymbolContainer{"BTC", "USDT", ""},
			SymbolContainer{"BTC", "USDT", ""},
		},
		{
			"ETH USDT -Hello",
			SymbolContainer{"ETH", "USDT", "-Hello"},
			SymbolContainer{"ETH", "USDT", ""},
		},
		{
			"max cap",
			SymbolContainer{strings.Repeat("A", baseCap), strings.Repeat("B", quoteCap), strings.Repeat("C", memoCap)},
			SymbolContainer{strings.Repeat("A", baseCap), strings.Repeat("B", quoteCap), strings.Repeat("C", memoCap)},
		},
		{
			"overflow",
			SymbolContainer{strings.Repeat("A", baseCap) + "123", strings.Repeat("B", quoteCap) + "456", strings.Repeat("C", memoCap) + "789"},
			SymbolContainer{strings.Repeat("A", baseCap), strings.Repeat("B", quoteCap), strings.Repeat("C", memoCap)},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			s := NewSymbol(tc.input.Base, tc.input.Quote, tc.input.Memo)
			b, q, m := s.Decode()

			if b != tc.expected.Base {
				t.Fatalf("base mismatch! should be %s but got %s", tc.expected.Base, b)
			}

			if q != tc.expected.Quote {
				t.Fatalf("quote mismatch! should be %s but got %s", tc.expected.Quote, q)
			}

			if m != tc.expected.Memo {
				t.Fatalf("memo mismatch! should be %s but got %s", tc.expected.Memo, m)
			}
		})
	}
}

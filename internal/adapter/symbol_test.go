package adapter

import "testing"

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
			SymbolContainer{"ETH", "USDT", "-Hello"},
		},
		{
			"max cap",
			SymbolContainer{"ABCDEF", "UVWXYZ", "GHIJKLMNOPQR"},
			SymbolContainer{"ABCDEF", "UVWXYZ", "GHIJKLMNOPQR"},
		},
		{
			"overflow",
			SymbolContainer{"ABCDEF123", "UVWXYZ456", "GHIJKLMNOPQR789"},
			SymbolContainer{"ABCDEF", "UVWXYZ", "GHIJKLMNOPQR"},
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

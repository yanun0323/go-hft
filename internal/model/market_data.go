package model

import "github.com/govalues/decimal"




type Depth struct {
	SymbolID    uint16
	EventTsNano int64
	RecvTsNano  int64
	Bids        [][2]decimal.Decimal
	Asks        [][2]decimal.Decimal
}

func (Depth) Encode() []byte {
	return nil
}

func (Depth) Decode() Depth {
	return Depth{}
}

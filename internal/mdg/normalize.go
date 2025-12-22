package mdg

import (
	"fmt"
	"time"

	"main/internal/schema"
)

// RawTick is a normalized raw market data input.
type RawTick struct {
	Symbol   string
	Kind     schema.MarketDataKind
	Flags    uint16
	Price    int64
	Size     int64
	BidPrice int64
	BidSize  int64
	AskPrice int64
	AskSize  int64
	Source   uint16
	TsEvent  int64
	TsRecv   int64
}

// Normalizer maps raw ticks to schema.MarketData.
type Normalizer struct {
	reg *schema.Registry
}

// NewNormalizer creates a normalizer for a registry.
func NewNormalizer(reg *schema.Registry) *Normalizer {
	return &Normalizer{reg: reg}
}

// Normalize converts a raw tick into a schema event and header.
func (n *Normalizer) Normalize(seq uint64, tick RawTick) (schema.EventHeader, schema.MarketData, error) {
	if n.reg == nil {
		return schema.EventHeader{}, schema.MarketData{}, fmt.Errorf("registry is nil")
	}
	symbolID, ok := n.reg.SymbolIDByName(tick.Symbol)
	if !ok {
		return schema.EventHeader{}, schema.MarketData{}, fmt.Errorf("symbol not found: %s", tick.Symbol)
	}
	if tick.TsRecv == 0 {
		tick.TsRecv = time.Now().UTC().UnixNano()
	}
	if tick.TsEvent == 0 {
		tick.TsEvent = tick.TsRecv
	}
	header := schema.NewHeader(schema.EventMarketData, tick.Source, seq, tick.TsEvent, tick.TsRecv)
	md := schema.MarketData{
		SymbolID: uint32(symbolID),
		Kind:     tick.Kind,
		Flags:    tick.Flags,
		Price:    schema.Price(tick.Price),
		Size:     schema.Quantity(tick.Size),
		BidPrice: schema.Price(tick.BidPrice),
		BidSize:  schema.Quantity(tick.BidSize),
		AskPrice: schema.Price(tick.AskPrice),
		AskSize:  schema.Quantity(tick.AskSize),
	}
	return header, md, nil
}

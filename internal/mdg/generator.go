package mdg

import (
	"fmt"
	"time"

	"main/internal/schema"
)

// Generator creates synthetic market data ticks.
type Generator struct {
	symbols   []schema.Symbol
	kind      schema.MarketDataKind
	source    uint16
	basePrice int64
	baseSize  int64
	spread    int64
	index     int
}

// NewGenerator creates a generator for all symbols in the registry.
func NewGenerator(reg *schema.Registry, kind schema.MarketDataKind, source uint16, basePrice, baseSize, spread int64) (*Generator, error) {
	if reg == nil || reg.SymbolCount() == 0 {
		return nil, fmt.Errorf("registry has no symbols")
	}
	symbols := make([]schema.Symbol, 0, reg.SymbolCount())
	for i := 0; i < reg.SymbolCount(); i++ {
		symbol, ok := reg.SymbolAt(i)
		if !ok {
			continue
		}
		symbols = append(symbols, symbol)
	}
	if len(symbols) == 0 {
		return nil, fmt.Errorf("registry has no symbols")
	}
	if baseSize <= 0 {
		baseSize = 1
	}
	if spread < 0 {
		spread = 0
	}
	return &Generator{
		symbols:   symbols,
		kind:      kind,
		source:    source,
		basePrice: basePrice,
		baseSize:  baseSize,
		spread:    spread,
	}, nil
}

// Next creates the next raw tick in sequence.
func (g *Generator) Next(now time.Time) RawTick {
	symbol := g.symbols[g.index]
	g.index = (g.index + 1) % len(g.symbols)
	price := g.basePrice + int64(g.index)
	return RawTick{
		Symbol:   symbol.Name,
		Kind:     g.kind,
		Price:    price,
		Size:     g.baseSize,
		BidPrice: price - g.spread,
		BidSize:  g.baseSize,
		AskPrice: price + g.spread,
		AskSize:  g.baseSize,
		Source:   g.source,
		TsEvent:  now.UnixNano(),
		TsRecv:   now.UnixNano(),
	}
}

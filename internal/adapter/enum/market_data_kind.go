package enum

// MarketDataKind describes the meaning of the market data payload.
type MarketDataKind uint8

const (
	_market_data_beg MarketDataKind = iota
	MarketDataDepth
	MarketDataOrder
	// MarketDataQuote
	_market_data_end
)

func (m MarketDataKind) IsAvailable() bool {
	return m > _market_data_beg && m < _market_data_end
}

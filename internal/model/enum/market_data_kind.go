package enum

// MarketDataKind describes the meaning of the market data payload.
type MarketDataKind uint8

const (
	MarketDataUnknown MarketDataKind = iota
	MarketDataDepth
	MarketDataTrade
	MarketDataQuote
)

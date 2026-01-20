package adapter

import "main/internal/adapter/enum"

// use `make codable-gen` to generate code
//
//go:generate codable
type Depth struct {
	SymbolID    uint16
	EventTsNano int64
	RecvTsNano  int64
	Platform    enum.Platform
	Bids        [128]DepthRow
	BidsLength  int
	Asks        [128]DepthRow
	AsksLength  int
}

type DepthRow struct {
	Price    Price
	Quantity Quantity
}

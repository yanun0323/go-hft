package model

import "main/internal/model/enum"

type DepthRow struct {
	Price    Price
	Quantity Quantity
}

// use make codable-gen
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

// use make codable-gen
//
//go:generate codable
type Order struct {
	ID           int64
	SymbolID     uint16
	EventTsNano  int64
	RecvTsNano   int64
	Platform     enum.Platform
	Type         enum.OrderType
	Side         enum.OrderSide
	Status       enum.OrderStatus
	Price        Price
	Quantity     Quantity
	LeftQuantity Quantity
	TimeInForce  enum.OrderTimeInForce
	UpdatedTime  int64
	Source       [64]byte
}

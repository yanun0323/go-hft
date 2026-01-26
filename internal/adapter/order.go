package adapter

import "main/internal/adapter/enum"

// use `make codable-gen` to generate code
//
//go:generate codable
type Order struct {
	ID           int64
	Symbol       Symbol
	EventTsNano  int64
	RecvTsNano   int64
	Platform     enum.Platform
	Type         enum.OrderType
	Side         enum.OrderSide
	Status       enum.OrderStatus
	Price        Decimal
	Quantity     Decimal
	LeftQuantity Decimal
	TimeInForce  enum.OrderTimeInForce
	UpdatedTime  int64
	Source       [64]byte
}

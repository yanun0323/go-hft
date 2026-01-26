package adapter

import "main/internal/adapter/enum"

// use `make codable-gen` to generate code
//
//go:generate codable
type OrderIntent struct {
	SymbolID      Symbol
	Platform      enum.Platform
	Action        enum.OrderAction
	Type          enum.OrderType
	Side          enum.OrderSide
	Price         Price
	Quantity      Quantity
	TimeInForce   enum.OrderTimeInForce
	CreatedTime   int64
	ID            Str64
	ClientOrderID Str64
	Source        Str64
}

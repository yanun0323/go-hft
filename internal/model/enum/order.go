package enum

// OrderSide buy, sell
type OrderSide uint8

const (
	_order_side_beg OrderSide = iota
	OrderSideBuy
	OrderSideSell
	_order_side_end
)

func (s OrderSide) IsAvailable() bool {
	return s > _order_side_beg && s < _order_side_end
}

// OrderSide limit, market
type OrderType uint8

const (
	_order_type_beg OrderType = iota
	OrderTypeLimit
	OrderTypeMarket
	_order_type_end
)

func (t OrderType) IsAvailable() bool {
	return t > _order_type_beg && t < _order_type_end
}

// OrderStatus placed, partial filled, filled, cancel, expired
type OrderStatus uint8

const (
	_order_status_beg OrderStatus = iota
	OrderStatusPlaced
	OrderStatusPartialFilled
	OrderStatusFilled
	OrderStatusCanceled
	OrderStatusExpired
	_order_status_end
)

func (s OrderStatus) IsAvailable() bool {
	return s > _order_status_beg && s < _order_status_end
}

// OrderTimeInForce GTC, IOC, FOK
type OrderTimeInForce uint8

const (
	_order_time_in_force_beg OrderTimeInForce = iota
	OrderTimeInForceGTC
	OrderTimeInForceIOC
	OrderTimeInForceFOK
	_order_time_in_force_end
)

func (s OrderTimeInForce) IsAvailable() bool {
	return s > _order_time_in_force_beg && s < _order_time_in_force_end
}

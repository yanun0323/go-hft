package schema

// Price is a scaled integer. The scale is defined by configuration.
type Price int64

// Quantity is a scaled integer. The scale is defined by configuration.
type Quantity int64

// Notional is a scaled integer. The scale is defined by configuration.
type Notional int64

// Fee is a scaled integer. The scale is defined by configuration.
type Fee int64

// MarketDataKind describes the meaning of the market data payload.
type MarketDataKind uint16

const (
	MarketDataUnknown MarketDataKind = iota
	MarketDataTrade
	MarketDataQuote
)

// MarketData is the payload for EventMarketData.
type MarketData struct {
	SymbolID uint32
	Kind     MarketDataKind
	Flags    uint16
	Price    Price
	Size     Quantity
	BidPrice Price
	BidSize  Quantity
	AskPrice Price
	AskSize  Quantity
}

// OrderSide describes order direction.
type OrderSide uint16

const (
	OrderSideUnknown OrderSide = iota
	OrderSideBuy
	OrderSideSell
)

// OrderType describes order type.
type OrderType uint16

const (
	OrderTypeUnknown OrderType = iota
	OrderTypeLimit
	OrderTypeMarket
)

// TimeInForce describes order time-in-force.
type TimeInForce uint16

const (
	TimeInForceUnknown TimeInForce = iota
	TimeInForceGTC
	TimeInForceIOC
	TimeInForceFOK
)

// OrderIntent is the payload for EventOrderIntent.
type OrderIntent struct {
	OrderID     uint64
	StrategyID  uint32
	SymbolID    uint32
	Side        OrderSide
	Type        OrderType
	TimeInForce TimeInForce
	Flags       uint16
	Price       Price
	Qty         Quantity
}

// OrderAckStatus describes the outcome of an order acknowledgment.
type OrderAckStatus uint16

const (
	OrderAckStatusUnknown OrderAckStatus = iota
	OrderAckStatusAcked
	OrderAckStatusRejected
	OrderAckStatusCanceled
	OrderAckStatusExpired
	OrderAckStatusPartFilled
	OrderAckStatusFilled
)

// OrderAckReason describes the reason for an order acknowledgment.
type OrderAckReason uint16

const (
	OrderAckReasonNone OrderAckReason = iota
	OrderAckReasonExchangeReject
	OrderAckReasonRiskReject
	OrderAckReasonRateLimit
	OrderAckReasonInvalidPrice
	OrderAckReasonInvalidQty
	OrderAckReasonNotAllowed
)

// OrderAck is the payload for EventOrderAck.
type OrderAck struct {
	OrderID   uint64
	SymbolID  uint32
	Status    OrderAckStatus
	Reason    OrderAckReason
	Flags     uint16
	Reserved  uint16
	Price     Price
	Qty       Quantity
	LeavesQty Quantity
	Reserved2 uint32
}

// RiskAction is the outcome of a risk decision.
type RiskAction uint16

const (
	RiskActionUnknown RiskAction = iota
	RiskActionAllow
	RiskActionDeny
)

// RiskReason is a coarse reason code for risk decisions.
type RiskReason uint16

const (
	RiskReasonNone RiskReason = iota
	RiskReasonKillSwitch
	RiskReasonMaxQty
	RiskReasonMaxNotional
	RiskReasonRateLimit
	RiskReasonPriceBand
	RiskReasonPositionLimit
)

// RiskDecision is the payload for EventRiskDecision.
type RiskDecision struct {
	OrderID       uint64
	StrategyID    uint32
	SymbolID      uint32
	Action        RiskAction
	Reason        RiskReason
	Flags         uint16
	Reserved      uint16
	ProposedQty   Quantity
	ProposedPrice Price
	CurrentPos    Quantity
	MaxPos        Quantity
	MaxNotional   Notional
}

// Fill is the payload for EventFill.
type Fill struct {
	OrderID  uint64
	SymbolID uint32
	Side     OrderSide
	Flags    uint16
	Price    Price
	Qty      Quantity
	Fee      Fee
}

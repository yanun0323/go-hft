package og

import (
	"errors"

	"main/internal/schema"
)

var (
	ErrDuplicateOrder    = errors.New("order already exists")
	ErrUnknownOrder      = errors.New("order not found")
	ErrInvalidTransition = errors.New("invalid order state transition")
	ErrInvalidFill       = errors.New("invalid fill quantity")
)

// OrderState tracks the lifecycle of an order.
type OrderState uint16

const (
	OrderStateUnknown OrderState = iota
	OrderStateNew
	OrderStateSent
	OrderStateAcked
	OrderStatePartFilled
	OrderStateFilled
	OrderStateCanceled
	OrderStateRejected
	OrderStateExpired
)

// Order holds the gateway's view of an order.
type Order struct {
	ID        uint64
	SymbolID  uint32
	Side      schema.OrderSide
	Price     schema.Price
	Qty       schema.Quantity
	LeavesQty schema.Quantity
	State     OrderState
}

// StateMachine updates orders from intent/ack/fill events.
type StateMachine struct {
	orders map[uint64]*Order
}

// NewStateMachine creates an empty state machine.
func NewStateMachine() *StateMachine {
	return &StateMachine{orders: make(map[uint64]*Order)}
}

// Order returns the current order state.
func (m *StateMachine) Order(id uint64) (*Order, bool) {
	o, ok := m.orders[id]
	return o, ok
}

// ApplyIntent creates a new order in Sent state.
func (m *StateMachine) ApplyIntent(intent schema.OrderIntent) (*Order, error) {
	if intent.OrderID == 0 {
		return nil, ErrUnknownOrder
	}
	if _, ok := m.orders[intent.OrderID]; ok {
		return nil, ErrDuplicateOrder
	}
	o := &Order{
		ID:        intent.OrderID,
		SymbolID:  intent.SymbolID,
		Side:      intent.Side,
		Price:     intent.Price,
		Qty:       intent.Qty,
		LeavesQty: intent.Qty,
		State:     OrderStateSent,
	}
	m.orders[o.ID] = o
	return o, nil
}

// ApplyAck updates an order from an acknowledgment event.
func (m *StateMachine) ApplyAck(ack schema.OrderAck) (*Order, error) {
	o, ok := m.orders[ack.OrderID]
	if !ok {
		return nil, ErrUnknownOrder
	}
	if isTerminal(o.State) {
		return o, ErrInvalidTransition
	}
	if ack.SymbolID != 0 {
		o.SymbolID = ack.SymbolID
	}
	if ack.Qty != 0 {
		o.Qty = ack.Qty
	}
	if ack.LeavesQty != 0 {
		o.LeavesQty = ack.LeavesQty
	}

	switch ack.Status {
	case schema.OrderAckStatusAcked:
		o.State = OrderStateAcked
	case schema.OrderAckStatusRejected:
		o.State = OrderStateRejected
	case schema.OrderAckStatusCanceled:
		o.State = OrderStateCanceled
	case schema.OrderAckStatusExpired:
		o.State = OrderStateExpired
	case schema.OrderAckStatusPartFilled:
		o.State = OrderStatePartFilled
	case schema.OrderAckStatusFilled:
		o.State = OrderStateFilled
	default:
		o.State = OrderStateUnknown
	}
	return o, nil
}

// ApplyFill updates an order from a fill event.
func (m *StateMachine) ApplyFill(fill schema.Fill) (*Order, error) {
	o, ok := m.orders[fill.OrderID]
	if !ok {
		return nil, ErrUnknownOrder
	}
	if isTerminal(o.State) {
		return o, ErrInvalidTransition
	}
	qty := int64(fill.Qty)
	if qty <= 0 {
		return o, ErrInvalidFill
	}
	if o.LeavesQty == 0 && o.Qty > 0 {
		o.LeavesQty = o.Qty
	}
	leaves := int64(o.LeavesQty) - qty
	if leaves <= 0 {
		o.LeavesQty = 0
		o.State = OrderStateFilled
	} else {
		o.LeavesQty = schema.Quantity(leaves)
		o.State = OrderStatePartFilled
	}
	return o, nil
}

func isTerminal(state OrderState) bool {
	switch state {
	case OrderStateFilled, OrderStateCanceled, OrderStateRejected, OrderStateExpired:
		return true
	default:
		return false
	}
}

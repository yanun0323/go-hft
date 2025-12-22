package state

import "main/internal/schema"

// PositionReducer updates positions based on fill events.
type PositionReducer struct {
	positions map[uint32]schema.Quantity
}

// NewPositionReducer creates an empty reducer.
func NewPositionReducer() *PositionReducer {
	return &PositionReducer{positions: make(map[uint32]schema.Quantity)}
}

// ApplyFill updates the position and returns the new quantity.
func (r *PositionReducer) ApplyFill(fill schema.Fill) schema.Quantity {
	current := r.positions[fill.SymbolID]
	var next schema.Quantity
	switch fill.Side {
	case schema.OrderSideBuy:
		next = schema.Quantity(int64(current) + int64(fill.Qty))
	case schema.OrderSideSell:
		next = schema.Quantity(int64(current) - int64(fill.Qty))
	default:
		next = current
	}
	r.positions[fill.SymbolID] = next
	return next
}

// ApplySnapshot replaces positions with a snapshot.
func (r *PositionReducer) ApplySnapshot(snapshot Snapshot) {
	if r.positions == nil {
		r.positions = make(map[uint32]schema.Quantity, len(snapshot.Positions))
	} else {
		for key := range r.positions {
			delete(r.positions, key)
		}
	}
	for _, entry := range snapshot.Positions {
		r.positions[entry.SymbolID] = entry.Qty
	}
}

// Position returns the current position quantity for a symbol.
func (r *PositionReducer) Position(symbolID uint32) schema.Quantity {
	return r.positions[symbolID]
}

// Count returns the number of tracked symbols.
func (r *PositionReducer) Count() int {
	return len(r.positions)
}

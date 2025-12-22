package og

import (
	"errors"

	"main/internal/schema"
)

var ErrGatewayDisconnected = errors.New("order gateway disconnected")

// GatewayConfig controls the stub gateway behavior.
type GatewayConfig struct {
	Session           string
	ResendOnReconnect bool
}

// Gateway is a minimal order gateway stub with reconnect/resend support.
type Gateway struct {
	cfg       GatewayConfig
	state     *StateMachine
	pending   map[uint64]schema.OrderIntent
	connected bool
}

// NewGateway creates a new gateway stub.
func NewGateway(cfg GatewayConfig) *Gateway {
	if cfg.Session == "" {
		cfg.Session = "default"
	}
	return &Gateway{
		cfg:       cfg,
		state:     NewStateMachine(),
		pending:   make(map[uint64]schema.OrderIntent),
		connected: true,
	}
}

// State returns the underlying order state machine.
func (g *Gateway) State() *StateMachine {
	return g.state
}

// Send registers a new intent and stores it for potential resend.
func (g *Gateway) Send(intent schema.OrderIntent) error {
	if _, err := g.state.ApplyIntent(intent); err != nil {
		return err
	}
	g.pending[intent.OrderID] = intent
	if !g.connected {
		return ErrGatewayDisconnected
	}
	return nil
}

// OnAck updates order state from an acknowledgment.
func (g *Gateway) OnAck(ack schema.OrderAck) error {
	order, err := g.state.ApplyAck(ack)
	if err != nil {
		return err
	}
	if isTerminalState(order.State) {
		delete(g.pending, ack.OrderID)
	}
	return nil
}

// OnFill updates order state from a fill.
func (g *Gateway) OnFill(fill schema.Fill) error {
	order, err := g.state.ApplyFill(fill)
	if err != nil {
		return err
	}
	if isTerminalState(order.State) {
		delete(g.pending, fill.OrderID)
	}
	return nil
}

// Disconnect marks the gateway as disconnected.
func (g *Gateway) Disconnect() {
	g.connected = false
}

// Reconnect marks the gateway as connected and returns pending intents to resend.
func (g *Gateway) Reconnect() []schema.OrderIntent {
	g.connected = true
	if !g.cfg.ResendOnReconnect {
		return nil
	}
	out := make([]schema.OrderIntent, 0, len(g.pending))
	for _, intent := range g.pending {
		out = append(out, intent)
	}
	return out
}

func isTerminalState(state OrderState) bool {
	switch state {
	case OrderStateFilled, OrderStateCanceled, OrderStateRejected, OrderStateExpired:
		return true
	default:
		return false
	}
}

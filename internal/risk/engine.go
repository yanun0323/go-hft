package risk

import (
	"time"

	"main/internal/schema"
)

const maxInt64 = int64(^uint64(0) >> 1)

// Config defines simple risk limits.
type Config struct {
	Version             uint16          `json:"version"`
	KillSwitch          bool            `json:"killSwitch"`
	MaxOrderQty         schema.Quantity `json:"maxOrderQty"`
	MaxOrderNotional    schema.Notional `json:"maxOrderNotional"`
	MaxPosition         schema.Quantity `json:"maxPosition"`
	OrderRateLimit      int             `json:"orderRateLimit"`
	OrderRateWindow     time.Duration   `json:"orderRateWindow"`
	MaxPriceDeviationBps int64          `json:"maxPriceDeviationBps"`
}

// StateView provides the current position snapshot.
type StateView struct {
	Position       schema.Quantity
	ReferencePrice schema.Price
	Now            int64
}

// Engine evaluates risk decisions.
type Engine struct {
	cfg            Config
	rateWindowStart int64
	rateCount       int
}

// NewEngine creates a risk engine with static limits.
func NewEngine(cfg Config) *Engine {
	return &Engine{cfg: cfg}
}

// Evaluate applies simple checks to an order intent.
func (e *Engine) Evaluate(intent schema.OrderIntent, state StateView) schema.RiskDecision {
	decision := schema.RiskDecision{
		OrderID:       intent.OrderID,
		StrategyID:    intent.StrategyID,
		SymbolID:      intent.SymbolID,
		Action:        schema.RiskActionAllow,
		Reason:        schema.RiskReasonNone,
		Reserved:      e.cfg.Version,
		ProposedQty:   intent.Qty,
		ProposedPrice: intent.Price,
		CurrentPos:    state.Position,
		MaxPos:        e.cfg.MaxPosition,
		MaxNotional:   e.cfg.MaxOrderNotional,
	}

	now := state.Now
	if now == 0 {
		now = time.Now().UTC().UnixNano()
	}

	if e.cfg.KillSwitch {
		decision.Action = schema.RiskActionDeny
		decision.Reason = schema.RiskReasonKillSwitch
		return decision
	}

	if e.cfg.OrderRateLimit > 0 && e.cfg.OrderRateWindow > 0 {
		window := int64(e.cfg.OrderRateWindow)
		if e.rateWindowStart == 0 || now-e.rateWindowStart >= window {
			e.rateWindowStart = now
			e.rateCount = 0
		}
		e.rateCount++
		if e.rateCount > e.cfg.OrderRateLimit {
			decision.Action = schema.RiskActionDeny
			decision.Reason = schema.RiskReasonRateLimit
			return decision
		}
	}

	if e.cfg.MaxOrderQty > 0 && intent.Qty > e.cfg.MaxOrderQty {
		decision.Action = schema.RiskActionDeny
		decision.Reason = schema.RiskReasonMaxQty
		return decision
	}

	if e.cfg.MaxPriceDeviationBps > 0 && intent.Type == schema.OrderTypeLimit && intent.Price > 0 {
		ref := int64(state.ReferencePrice)
		if ref > 0 {
			diff := absInt64(int64(intent.Price) - ref)
			if exceedsDeviation(diff, ref, e.cfg.MaxPriceDeviationBps) {
				decision.Action = schema.RiskActionDeny
				decision.Reason = schema.RiskReasonPriceBand
				return decision
			}
		}
	}

	notional, overflow := mulNotional(intent.Price, intent.Qty)
	if overflow {
		decision.Action = schema.RiskActionDeny
		decision.Reason = schema.RiskReasonMaxNotional
		return decision
	}
	if e.cfg.MaxOrderNotional > 0 && notional > e.cfg.MaxOrderNotional {
		decision.Action = schema.RiskActionDeny
		decision.Reason = schema.RiskReasonMaxNotional
		return decision
	}

	nextPos := applySide(state.Position, intent.Side, intent.Qty)
	if e.cfg.MaxPosition > 0 && absQuantity(nextPos) > e.cfg.MaxPosition {
		decision.Action = schema.RiskActionDeny
		decision.Reason = schema.RiskReasonPositionLimit
		return decision
	}

	return decision
}

func mulNotional(price schema.Price, qty schema.Quantity) (schema.Notional, bool) {
	p := int64(price)
	q := int64(qty)
	if p == 0 || q == 0 {
		return 0, false
	}
	if p < 0 {
		p = -p
	}
	if q < 0 {
		q = -q
	}
	if p > maxInt64/q {
		return 0, true
	}
	return schema.Notional(int64(price) * int64(qty)), false
}

func applySide(pos schema.Quantity, side schema.OrderSide, qty schema.Quantity) schema.Quantity {
	switch side {
	case schema.OrderSideBuy:
		return schema.Quantity(int64(pos) + int64(qty))
	case schema.OrderSideSell:
		return schema.Quantity(int64(pos) - int64(qty))
	default:
		return pos
	}
}

func absQuantity(q schema.Quantity) schema.Quantity {
	if q < 0 {
		return -q
	}
	return q
}

func absInt64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func exceedsDeviation(diff int64, ref int64, bps int64) bool {
	if diff <= 0 || ref <= 0 || bps <= 0 {
		return false
	}
	if diff > maxInt64/10000 {
		return true
	}
	lhs := diff * 10000
	if ref > maxInt64/bps {
		return true
	}
	rhs := ref * bps
	return lhs > rhs
}

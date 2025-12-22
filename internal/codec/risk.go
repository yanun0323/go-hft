package codec

import (
	"encoding/binary"

	"main/internal/schema"
)

const RiskDecisionPayloadSize = 64

// EncodeRiskDecision serializes a risk decision into a fixed-size payload.
func EncodeRiskDecision(dst []byte, decision schema.RiskDecision) []byte {
	if cap(dst) < RiskDecisionPayloadSize {
		dst = make([]byte, RiskDecisionPayloadSize)
	} else {
		dst = dst[:RiskDecisionPayloadSize]
	}

	binary.LittleEndian.PutUint64(dst[0:8], decision.OrderID)
	binary.LittleEndian.PutUint32(dst[8:12], decision.StrategyID)
	binary.LittleEndian.PutUint32(dst[12:16], decision.SymbolID)
	binary.LittleEndian.PutUint16(dst[16:18], uint16(decision.Action))
	binary.LittleEndian.PutUint16(dst[18:20], uint16(decision.Reason))
	binary.LittleEndian.PutUint16(dst[20:22], decision.Flags)
	binary.LittleEndian.PutUint16(dst[22:24], decision.Reserved)
	binary.LittleEndian.PutUint64(dst[24:32], uint64(decision.ProposedQty))
	binary.LittleEndian.PutUint64(dst[32:40], uint64(decision.ProposedPrice))
	binary.LittleEndian.PutUint64(dst[40:48], uint64(decision.CurrentPos))
	binary.LittleEndian.PutUint64(dst[48:56], uint64(decision.MaxPos))
	binary.LittleEndian.PutUint64(dst[56:64], uint64(decision.MaxNotional))

	return dst
}

// DecodeRiskDecision parses a fixed-size risk decision payload.
func DecodeRiskDecision(src []byte) (schema.RiskDecision, bool) {
	if len(src) < RiskDecisionPayloadSize {
		return schema.RiskDecision{}, false
	}
	return schema.RiskDecision{
		OrderID:       binary.LittleEndian.Uint64(src[0:8]),
		StrategyID:    binary.LittleEndian.Uint32(src[8:12]),
		SymbolID:      binary.LittleEndian.Uint32(src[12:16]),
		Action:        schema.RiskAction(binary.LittleEndian.Uint16(src[16:18])),
		Reason:        schema.RiskReason(binary.LittleEndian.Uint16(src[18:20])),
		Flags:         binary.LittleEndian.Uint16(src[20:22]),
		Reserved:      binary.LittleEndian.Uint16(src[22:24]),
		ProposedQty:   schema.Quantity(int64(binary.LittleEndian.Uint64(src[24:32]))),
		ProposedPrice: schema.Price(int64(binary.LittleEndian.Uint64(src[32:40]))),
		CurrentPos:    schema.Quantity(int64(binary.LittleEndian.Uint64(src[40:48]))),
		MaxPos:        schema.Quantity(int64(binary.LittleEndian.Uint64(src[48:56]))),
		MaxNotional:   schema.Notional(int64(binary.LittleEndian.Uint64(src[56:64]))),
	}, true
}

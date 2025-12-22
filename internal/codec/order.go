package codec

import (
	"encoding/binary"

	"main/internal/schema"
)

const OrderIntentPayloadSize = 40

// EncodeOrderIntent serializes an order intent into a fixed-size payload.
func EncodeOrderIntent(dst []byte, order schema.OrderIntent) []byte {
	if cap(dst) < OrderIntentPayloadSize {
		dst = make([]byte, OrderIntentPayloadSize)
	} else {
		dst = dst[:OrderIntentPayloadSize]
	}

	binary.LittleEndian.PutUint64(dst[0:8], order.OrderID)
	binary.LittleEndian.PutUint32(dst[8:12], order.StrategyID)
	binary.LittleEndian.PutUint32(dst[12:16], order.SymbolID)
	binary.LittleEndian.PutUint16(dst[16:18], uint16(order.Side))
	binary.LittleEndian.PutUint16(dst[18:20], uint16(order.Type))
	binary.LittleEndian.PutUint16(dst[20:22], uint16(order.TimeInForce))
	binary.LittleEndian.PutUint16(dst[22:24], order.Flags)
	binary.LittleEndian.PutUint64(dst[24:32], uint64(order.Price))
	binary.LittleEndian.PutUint64(dst[32:40], uint64(order.Qty))

	return dst
}

// DecodeOrderIntent parses a fixed-size order intent payload.
func DecodeOrderIntent(src []byte) (schema.OrderIntent, bool) {
	if len(src) < OrderIntentPayloadSize {
		return schema.OrderIntent{}, false
	}
	return schema.OrderIntent{
		OrderID:     binary.LittleEndian.Uint64(src[0:8]),
		StrategyID:  binary.LittleEndian.Uint32(src[8:12]),
		SymbolID:    binary.LittleEndian.Uint32(src[12:16]),
		Side:        schema.OrderSide(binary.LittleEndian.Uint16(src[16:18])),
		Type:        schema.OrderType(binary.LittleEndian.Uint16(src[18:20])),
		TimeInForce: schema.TimeInForce(binary.LittleEndian.Uint16(src[20:22])),
		Flags:       binary.LittleEndian.Uint16(src[22:24]),
		Price:       schema.Price(int64(binary.LittleEndian.Uint64(src[24:32]))),
		Qty:         schema.Quantity(int64(binary.LittleEndian.Uint64(src[32:40]))),
	}, true
}

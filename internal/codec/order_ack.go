package codec

import (
	"encoding/binary"

	"main/internal/schema"
)

const OrderAckPayloadSize = 48

// EncodeOrderAck serializes an order acknowledgment into a fixed-size payload.
func EncodeOrderAck(dst []byte, ack schema.OrderAck) []byte {
	if cap(dst) < OrderAckPayloadSize {
		dst = make([]byte, OrderAckPayloadSize)
	} else {
		dst = dst[:OrderAckPayloadSize]
	}

	binary.LittleEndian.PutUint64(dst[0:8], ack.OrderID)
	binary.LittleEndian.PutUint32(dst[8:12], ack.SymbolID)
	binary.LittleEndian.PutUint16(dst[12:14], uint16(ack.Status))
	binary.LittleEndian.PutUint16(dst[14:16], uint16(ack.Reason))
	binary.LittleEndian.PutUint16(dst[16:18], ack.Flags)
	binary.LittleEndian.PutUint16(dst[18:20], ack.Reserved)
	binary.LittleEndian.PutUint64(dst[20:28], uint64(ack.Price))
	binary.LittleEndian.PutUint64(dst[28:36], uint64(ack.Qty))
	binary.LittleEndian.PutUint64(dst[36:44], uint64(ack.LeavesQty))
	binary.LittleEndian.PutUint32(dst[44:48], ack.Reserved2)

	return dst
}

// DecodeOrderAck parses a fixed-size order acknowledgment payload.
func DecodeOrderAck(src []byte) (schema.OrderAck, bool) {
	if len(src) < OrderAckPayloadSize {
		return schema.OrderAck{}, false
	}
	return schema.OrderAck{
		OrderID:   binary.LittleEndian.Uint64(src[0:8]),
		SymbolID:  binary.LittleEndian.Uint32(src[8:12]),
		Status:    schema.OrderAckStatus(binary.LittleEndian.Uint16(src[12:14])),
		Reason:    schema.OrderAckReason(binary.LittleEndian.Uint16(src[14:16])),
		Flags:     binary.LittleEndian.Uint16(src[16:18]),
		Reserved:  binary.LittleEndian.Uint16(src[18:20]),
		Price:     schema.Price(int64(binary.LittleEndian.Uint64(src[20:28]))),
		Qty:       schema.Quantity(int64(binary.LittleEndian.Uint64(src[28:36]))),
		LeavesQty: schema.Quantity(int64(binary.LittleEndian.Uint64(src[36:44]))),
		Reserved2: binary.LittleEndian.Uint32(src[44:48]),
	}, true
}

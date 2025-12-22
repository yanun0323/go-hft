package codec

import (
	"encoding/binary"

	"main/internal/schema"
)

const FillPayloadSize = 40

// EncodeFill serializes a fill into a fixed-size payload.
func EncodeFill(dst []byte, fill schema.Fill) []byte {
	if cap(dst) < FillPayloadSize {
		dst = make([]byte, FillPayloadSize)
	} else {
		dst = dst[:FillPayloadSize]
	}

	binary.LittleEndian.PutUint64(dst[0:8], fill.OrderID)
	binary.LittleEndian.PutUint32(dst[8:12], fill.SymbolID)
	binary.LittleEndian.PutUint16(dst[12:14], uint16(fill.Side))
	binary.LittleEndian.PutUint16(dst[14:16], fill.Flags)
	binary.LittleEndian.PutUint64(dst[16:24], uint64(fill.Price))
	binary.LittleEndian.PutUint64(dst[24:32], uint64(fill.Qty))
	binary.LittleEndian.PutUint64(dst[32:40], uint64(fill.Fee))

	return dst
}

// DecodeFill parses a fixed-size fill payload.
func DecodeFill(src []byte) (schema.Fill, bool) {
	if len(src) < FillPayloadSize {
		return schema.Fill{}, false
	}
	return schema.Fill{
		OrderID:  binary.LittleEndian.Uint64(src[0:8]),
		SymbolID: binary.LittleEndian.Uint32(src[8:12]),
		Side:     schema.OrderSide(binary.LittleEndian.Uint16(src[12:14])),
		Flags:    binary.LittleEndian.Uint16(src[14:16]),
		Price:    schema.Price(int64(binary.LittleEndian.Uint64(src[16:24]))),
		Qty:      schema.Quantity(int64(binary.LittleEndian.Uint64(src[24:32]))),
		Fee:      schema.Fee(int64(binary.LittleEndian.Uint64(src[32:40]))),
	}, true
}

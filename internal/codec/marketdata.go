package codec

import (
	"encoding/binary"

	"main/internal/schema"
)

const MarketDataPayloadSize = 56

// EncodeMarketData serializes market data into a fixed-size payload.
func EncodeMarketData(dst []byte, md schema.MarketData) []byte {
	if cap(dst) < MarketDataPayloadSize {
		dst = make([]byte, MarketDataPayloadSize)
	} else {
		dst = dst[:MarketDataPayloadSize]
	}

	binary.LittleEndian.PutUint32(dst[0:4], md.SymbolID)
	binary.LittleEndian.PutUint16(dst[4:6], uint16(md.Kind))
	binary.LittleEndian.PutUint16(dst[6:8], md.Flags)
	binary.LittleEndian.PutUint64(dst[8:16], uint64(md.Price))
	binary.LittleEndian.PutUint64(dst[16:24], uint64(md.Size))
	binary.LittleEndian.PutUint64(dst[24:32], uint64(md.BidPrice))
	binary.LittleEndian.PutUint64(dst[32:40], uint64(md.BidSize))
	binary.LittleEndian.PutUint64(dst[40:48], uint64(md.AskPrice))
	binary.LittleEndian.PutUint64(dst[48:56], uint64(md.AskSize))

	return dst
}

// DecodeMarketData parses a fixed-size market data payload.
func DecodeMarketData(src []byte) (schema.MarketData, bool) {
	if len(src) < MarketDataPayloadSize {
		return schema.MarketData{}, false
	}
	return schema.MarketData{
		SymbolID: binary.LittleEndian.Uint32(src[0:4]),
		Kind:     schema.MarketDataKind(binary.LittleEndian.Uint16(src[4:6])),
		Flags:    binary.LittleEndian.Uint16(src[6:8]),
		Price:    schema.Price(int64(binary.LittleEndian.Uint64(src[8:16]))),
		Size:     schema.Quantity(int64(binary.LittleEndian.Uint64(src[16:24]))),
		BidPrice: schema.Price(int64(binary.LittleEndian.Uint64(src[24:32]))),
		BidSize:  schema.Quantity(int64(binary.LittleEndian.Uint64(src[32:40]))),
		AskPrice: schema.Price(int64(binary.LittleEndian.Uint64(src[40:48]))),
		AskSize:  schema.Quantity(int64(binary.LittleEndian.Uint64(src[48:56]))),
	}, true
}

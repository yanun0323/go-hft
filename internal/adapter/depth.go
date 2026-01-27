package adapter

import (
	"strconv"

	"main/internal/adapter/enum"
)

// use `make codable-gen` to generate code
//
//go:generate codable
type Depth struct {
	Symbol      Symbol
	EventTsNano int64
	RecvTsNano  int64
	Platform    enum.Platform
	Bids        [128]DepthRow
	BidsLength  int
	Asks        [128]DepthRow
	AsksLength  int
}

type DepthRow struct {
	Price    Decimal
	Quantity Decimal
}

// Debug returns a human readable format string
func (d Depth) Debug() string {
	appendPlatform := func(buf []byte, p enum.Platform) []byte {
		switch p {
		case enum.PlatformBTCC:
			return append(buf, "btcc"...)
		case enum.PlatformBinance:
			return append(buf, "binance"...)
		default:
			return strconv.AppendInt(buf, int64(p), 10)
		}
	}

	appendDepthSide := func(buf []byte, rows []DepthRow, length int) []byte {
		buf = append(buf, '[')
		if length < 0 {
			length = 0
		}
		if length > len(rows) {
			length = len(rows)
		}
		for i := 0; i < length; i++ {
			if i > 0 {
				buf = append(buf, ',')
			}
			buf = append(buf, '(')
			buf = rows[i].Price.AppendBytes(buf)
			buf = append(buf, ',')
			buf = rows[i].Quantity.AppendBytes(buf)
			buf = append(buf, ')')
		}
		buf = append(buf, ']')
		return buf
	}

	buf := make([]byte, 0, 256)
	buf = append(buf, "Depth{symbol="...)
	buf = append(buf, d.Symbol.String()...)
	buf = append(buf, " platform="...)
	buf = appendPlatform(buf, d.Platform)
	buf = append(buf, " event_ts="...)
	buf = strconv.AppendInt(buf, d.EventTsNano, 10)
	buf = append(buf, " recv_ts="...)
	buf = strconv.AppendInt(buf, d.RecvTsNano, 10)
	buf = append(buf, " bids="...)
	buf = appendDepthSide(buf, d.Bids[:], d.BidsLength)
	buf = append(buf, " asks="...)
	buf = appendDepthSide(buf, d.Asks[:], d.AsksLength)
	buf = append(buf, '}')
	return string(buf)
}

package model

import "strconv"

// Price is a scaled integer. The scale is defined by configuration.
type Price int64

func (p Price) AppendString(priceScale int, buf []byte) []byte {
	return appendScaledInt(buf, int64(p), priceScale)
}

// Quantity is a scaled integer. The scale is defined by configuration.
type Quantity int64

func (q Quantity) AppendString(quantityScale int, buf []byte) []byte {
	return appendScaledInt(buf, int64(q), quantityScale)
}

// Notional is a scaled integer. The scale is defined by configuration.
type Notional int64

func (n Notional) AppendString(nationalScale int, buf []byte) []byte {
	return appendScaledInt(buf, int64(n), nationalScale)
}

// Fee is a scaled integer. The scale is defined by configuration.
type Fee int64

func (f Fee) AppendString(feeScale int, buf []byte) []byte {
	return appendScaledInt(buf, int64(f), feeScale)
}

func appendScaledInt(buf []byte, value int64, scale int) []byte {
	if scale <= 0 {
		return strconv.AppendInt(buf, value, 10)
	}

	neg := value < 0
	u := uint64(value)
	if neg {
		u = uint64(^value) + 1
	}

	var tmp [32]byte
	digits := strconv.AppendUint(tmp[:0], u, 10)

	if neg {
		buf = append(buf, '-')
	}

	if len(digits) <= scale {
		buf = append(buf, '0', '.')
		for i := 0; i < scale-len(digits); i++ {
			buf = append(buf, '0')
		}
		buf = append(buf, digits...)
		return buf
	}

	idx := len(digits) - scale
	buf = append(buf, digits[:idx]...)
	buf = append(buf, '.')
	buf = append(buf, digits[idx:]...)
	return buf
}

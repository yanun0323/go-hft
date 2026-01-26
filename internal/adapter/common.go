package adapter

import "strconv"

// Decimal
type Decimal struct {
	Integer int64
	Scale   int
}

func (d Decimal) AppendBytes(buf []byte) []byte {
	return appendScaledInt(buf, d.Integer, d.Scale)
}

func appendScaledInt(buf []byte, value int64, scale int) []byte {
	buf = buf[:0]

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

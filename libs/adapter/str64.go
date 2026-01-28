package adapter

type Str64 [64]byte

func NewStr64(s string) Str64 {
	var k Str64
	copy(k[:], s[:])
	return k
}

func (bs Str64) Slice() []byte {
	return bs.AppendBytes(make([]byte, 0, len(bs)))
}

func (bs Str64) String() string {
	return string(bs.Slice())
}

func (bs Str64) Len() int {
	var l int
	for _, char := range bs {
		if char != 0 {
			l++
		}
	}
	return l
}

func (bs Str64) AppendBytes(buf []byte) []byte {
	buf = buf[:0]
	for i := range bs {
		if bs[i] != 0 {
			buf = append(buf, bs[i])
		}
	}
	return buf
}

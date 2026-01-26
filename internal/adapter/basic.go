package adapter

type Str64 [64]byte

func NewBufferedString(s string) Str64 {
	var k Str64
	copy(k[:], s[:])
	return k
}

func (bs Str64) Slice() []byte {
	buf := make([]byte, 0, len(bs))
	for i := range bs {
		if bs[i] != 0 {
			buf = append(buf, bs[i])
		}
	}
	return buf
}

func (bs Str64) String() string {
	return string(bs.Slice())
}

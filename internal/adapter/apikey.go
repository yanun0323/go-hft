package adapter

type APIKey [64]byte

func NewAPIKey(s string) APIKey {
	var k APIKey
	copy(k[:], s[:])
	return k
}

func (k APIKey) Slice() []byte {
	buf := make([]byte, 0, len(k))
	for i := range k {
		if k[i] != 0 {
			buf = append(buf, k[i])
		}
	}
	return buf
}

func (k APIKey) String() string {
	return string(k.Slice())
}

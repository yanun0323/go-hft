package scanner

func ScanUintField(payload []byte, key []byte) (uint64, bool) {
	idx := IndexOf(payload, key)
	if idx < 0 {
		return 0, false
	}
	i := idx + len(key)
	for i < len(payload) && payload[i] != ':' {
		i++
	}
	if i >= len(payload) {
		return 0, false
	}
	i++
	for i < len(payload) && IsSpace(payload[i]) {
		i++
	}
	if i >= len(payload) || payload[i] < '0' || payload[i] > '9' {
		return 0, false
	}
	var v uint64
	for i < len(payload) && payload[i] >= '0' && payload[i] <= '9' {
		v = v*10 + uint64(payload[i]-'0')
		i++
	}
	return v, true
}

func ScanStringField(payload []byte, key []byte) ([]byte, bool) {
	idx := IndexOf(payload, key)
	if idx < 0 {
		return nil, false
	}
	i := idx + len(key)
	for i < len(payload) && payload[i] != ':' {
		i++
	}
	if i >= len(payload) {
		return nil, false
	}
	i++
	for i < len(payload) && IsSpace(payload[i]) {
		i++
	}
	if i >= len(payload) || payload[i] != '"' {
		return nil, false
	}
	i++
	start := i
	for i < len(payload) && payload[i] != '"' {
		i++
	}
	if i >= len(payload) {
		return nil, false
	}
	return payload[start:i], true
}

func IndexOf(payload []byte, key []byte) int {
	if len(key) == 0 || len(payload) < len(key) {
		return -1
	}
outer:
	for i := 0; i <= len(payload)-len(key); i++ {
		for j := 0; j < len(key); j++ {
			if payload[i+j] != key[j] {
				continue outer
			}
		}
		return i
	}
	return -1
}

func IsSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

func BytesContains(haystack []byte, needle []byte) bool {
	if len(needle) == 0 {
		return true
	}
	if len(haystack) < len(needle) {
		return false
	}
outer:
	for i := 0; i <= len(haystack)-len(needle); i++ {
		for j := range needle {
			if haystack[i+j] != needle[j] {
				continue outer
			}
		}
		return true
	}
	return false
}

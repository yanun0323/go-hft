package adapter

const (
	baseCap   = 6
	quoteCap  = 6
	memoCap   = 12
	SymbolCap = baseCap + quoteCap + memoCap
)

var (
	symbolCharset = [...]rune{
		'\x00', ' ',
		'0', '1', '2', '3', '4', '5', '6', '7', '8', '9',
		'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'l', 'm', 'n', 'o', 'p', 'q', 'r', 's', 't', 'u', 'v', 'w', 'x', 'y', 'z',
		'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J', 'K', 'L', 'M', 'N', 'O', 'P', 'Q', 'R', 'S', 'T', 'U', 'V', 'W', 'X', 'Y', 'Z',
		'-', '_', '.', '*', '=', '\'', '"', '/', '\\', '|',
	}
	symbolCharsetMap = initSymbolCharsetMap()
)

func NewSymbol(base, quote string, memo ...string) Symbol {
	var s Symbol
	for i, char := range base {
		if i >= baseCap {
			break
		}
		s[i] = symbolCharsetMap[char]
	}

	for i, char := range quote {
		if i >= quoteCap {
			break
		}
		s[i+baseCap] = symbolCharsetMap[char]
	}

	if len(memo) != 0 {
		for i, char := range memo[0] {
			if i >= memoCap {
				break
			}
			s[i+baseCap+quoteCap] = symbolCharsetMap[char]
		}
	}

	return s
}

type Symbol [SymbolCap]uint8

func (Symbol) Encode(base, quote, memo string) Symbol {
	return NewSymbol(base, quote, memo)
}

func (symbol Symbol) Decode() (base, quote, memo string) {
	baseBuf := make([]rune, 0, baseCap)
	for _, n := range symbol[:baseCap] {
		if n == 0 {
			break
		}

		baseBuf = append(baseBuf, symbolCharset[n])
	}

	quoteBuf := make([]rune, 0, quoteCap)
	for _, n := range symbol[baseCap : baseCap+quoteCap] {
		if n == 0 {
			break
		}

		quoteBuf = append(quoteBuf, symbolCharset[n])
	}

	memoBuf := make([]rune, 0, memoCap)
	for _, n := range symbol[baseCap+quoteCap:] {
		if n == 0 {
			break
		}

		memoBuf = append(memoBuf, symbolCharset[n])
	}

	return string(baseBuf), string(quoteBuf), string(memoBuf)
}

func (symbol Symbol) String() string {
	symbolBuf := make([]rune, 0, SymbolCap)
	for _, n := range symbol[:baseCap] {
		if n == 0 {
			break
		}

		symbolBuf = append(symbolBuf, symbolCharset[n])
	}

	for _, n := range symbol[baseCap : baseCap+quoteCap] {
		if n == 0 {
			break
		}

		symbolBuf = append(symbolBuf, symbolCharset[n])
	}

	for _, n := range symbol[baseCap+quoteCap:] {
		if n == 0 {
			break
		}

		symbolBuf = append(symbolBuf, symbolCharset[n])
	}

	return string(symbolBuf)
}

func initSymbolCharsetMap() map[rune]uint8 {
	m := make(map[rune]uint8, len(symbolCharset))
	for i, char := range symbolCharset {
		m[char] = uint8(i)
	}

	return m
}

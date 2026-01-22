package adapter

const (
	baseCap   = 16
	quoteCap  = 16
	memoCap   = 0
	SymbolCap = baseCap + quoteCap + memoCap
)

type Symbol [SymbolCap]byte

func NewSymbol(base, quote string, memo ...string) Symbol {
	var s Symbol
	for i := range base {
		if i >= baseCap {
			break
		}
		s[i] = base[i]
	}

	for i := range quote {
		if i >= quoteCap {
			break
		}
		s[i+baseCap] = quote[i]
	}

	if len(memo) != 0 {
		for i := range memo[0] {
			if i >= memoCap {
				break
			}
			s[i+baseCap+quoteCap] = memo[0][i]
		}
	}

	return s
}

func (Symbol) Encode(base, quote, memo string) Symbol {
	return NewSymbol(base, quote, memo)
}

func (symbol Symbol) Decode() (base, quote, memo string) {
	baseBuf := make([]byte, 0, baseCap)
	for _, char := range symbol[:baseCap] {
		if char == 0 {
			break
		}

		baseBuf = append(baseBuf, char)
	}

	quoteBuf := make([]byte, 0, quoteCap)
	for _, char := range symbol[baseCap : baseCap+quoteCap] {
		if char == 0 {
			break
		}

		quoteBuf = append(quoteBuf, char)
	}

	memoBuf := make([]byte, 0, memoCap)
	for _, char := range symbol[baseCap+quoteCap:] {
		if char == 0 {
			break
		}

		memoBuf = append(memoBuf, char)
	}

	return string(baseBuf), string(quoteBuf), string(memoBuf)
}

func (symbol Symbol) String() string {
	symbolBuf := make([]byte, 0, SymbolCap)
	for _, char := range symbol[:baseCap] {
		if char == 0 {
			break
		}

		symbolBuf = append(symbolBuf, char)
	}

	for _, n := range symbol[baseCap : baseCap+quoteCap] {
		if n == 0 {
			break
		}

		symbolBuf = append(symbolBuf, n)
	}

	for _, n := range symbol[baseCap+quoteCap:] {
		if n == 0 {
			break
		}

		symbolBuf = append(symbolBuf, n)
	}

	return string(symbolBuf)
}

package adapter

// Token represents a API token
type Token struct {
	Key    Str64
	Secret Str64
}

// NewToken creates a API token
func NewToken(key, secret string) Token {
	var token Token
	copy(token.Key[:], key[:])
	copy(token.Secret[:], secret[:])
	return token
}


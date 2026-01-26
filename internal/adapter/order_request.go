package adapter

//go:generate codable
type OrderRequest struct {
	Intent OrderIntent
	Token  Token
}

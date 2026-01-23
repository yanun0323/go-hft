package adapter

//go:generate codable
type OrderRequest struct {
	Intent OrderIntent
	APIKey APIKey
}

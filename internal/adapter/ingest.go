package adapter

import (
	"main/internal/adapter/enum"
)

// IngestRequest is the minimal UDS request format.
//
//go:generate codable
type IngestRequest struct {
	Platform enum.Platform
	Topic    enum.Topic
	Symbol   Symbol
	APIKey   Str64
}

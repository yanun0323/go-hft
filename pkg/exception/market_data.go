package exception

import "errors"

var (
	ErrInvalidMarketDataRequest = errors.New("market data: invalid request")
	ErrUnsupportedPlatform      = errors.New("market data: unsupported platform")
	ErrNilConsumer              = errors.New("market data: nil consumer")
	ErrTopicKindMismatch        = errors.New("market data: topic kind mismatch")
	ErrUnknownTopic             = errors.New("market data: unknown topic")
)

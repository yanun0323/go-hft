package exception

import "errors"

var (
	ErrIngestInvalidRequest      = errors.New("ingest: invalid request")
	ErrIngestUnsupportedPlatform = errors.New("ingest: unsupported platform")
	ErrIngestNilConsumer         = errors.New("ingest: nil consumer")
	ErrIngestTopicKindMismatch   = errors.New("ingest: topic kind mismatch")
	ErrIngestUnknownTopic        = errors.New("ingest: unknown topic")
)

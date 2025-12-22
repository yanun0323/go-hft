package recorder

import (
	"fmt"
	"time"
)

const (
	defaultSegmentMaxBytes int64 = 1 << 30
	defaultQueueSize             = 4096
	defaultBufferSize            = 256 * 1024
	defaultFilePrefix            = "wal"
)

var defaultSegmentMaxDuration = 5 * time.Minute

// Config controls WAL writer behavior.
type Config struct {
	Dir                string
	SegmentMaxBytes    int64
	SegmentMaxDuration time.Duration
	QueueSize          int
	BufferSize         int
	FilePrefix         string
	FlushInterval      time.Duration
	SyncInterval       time.Duration
	CopyPayload        bool
}

// DefaultConfig returns a baseline configuration for the WAL writer.
func DefaultConfig(dir string) Config {
	return Config{
		Dir:                dir,
		SegmentMaxBytes:    defaultSegmentMaxBytes,
		SegmentMaxDuration: defaultSegmentMaxDuration,
		QueueSize:          defaultQueueSize,
		BufferSize:         defaultBufferSize,
		FilePrefix:         defaultFilePrefix,
	}
}

func (c Config) withDefaults() Config {
	if c.SegmentMaxBytes == 0 {
		c.SegmentMaxBytes = defaultSegmentMaxBytes
	}
	if c.QueueSize == 0 {
		c.QueueSize = defaultQueueSize
	}
	if c.BufferSize == 0 {
		c.BufferSize = defaultBufferSize
	}
	if c.FilePrefix == "" {
		c.FilePrefix = defaultFilePrefix
	}
	return c
}

// Validate checks if the configuration is usable.
func (c Config) Validate() error {
	if c.Dir == "" {
		return fmt.Errorf("invalid recorder config: Dir is empty")
	}
	if c.SegmentMaxBytes <= 0 {
		return fmt.Errorf("invalid recorder config: SegmentMaxBytes must be > 0")
	}
	if c.QueueSize <= 0 {
		return fmt.Errorf("invalid recorder config: QueueSize must be > 0")
	}
	if c.BufferSize <= 0 {
		return fmt.Errorf("invalid recorder config: BufferSize must be > 0")
	}
	if c.FilePrefix == "" {
		return fmt.Errorf("invalid recorder config: FilePrefix is empty")
	}
	if c.FlushInterval < 0 {
		return fmt.Errorf("invalid recorder config: FlushInterval must be >= 0")
	}
	if c.SyncInterval < 0 {
		return fmt.Errorf("invalid recorder config: SyncInterval must be >= 0")
	}
	return nil
}

package recorder

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"main/internal/schema"
)

// PlaybackConfig controls WAL playback behavior.
type PlaybackConfig struct {
	Dir             string
	FilePrefix      string
	Speed           float64
	UseRecvTime     bool
	DisableChecksum bool
	MaxPayloadSize  int
}

// Clock allows deterministic playback control.
type Clock interface {
	Sleep(ctx context.Context, d time.Duration) error
}

type realClock struct{}

func (realClock) Sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Playback replays WAL records in file order.
type Playback struct {
	cfg   PlaybackConfig
	clock Clock
}

// NewPlayback validates the config and creates a playback engine.
func NewPlayback(cfg PlaybackConfig) (*Playback, error) {
	cfg = cfg.withDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Playback{cfg: cfg, clock: realClock{}}, nil
}

// WithClock swaps the clock implementation.
func (p *Playback) WithClock(clock Clock) *Playback {
	if clock != nil {
		p.clock = clock
	}
	return p
}

// Run replays WAL records and calls the handler for each event.
func (p *Playback) Run(ctx context.Context, handler func(schema.EventHeader, []byte) error) error {
	if handler == nil {
		return errors.New("playback handler is nil")
	}
	files, err := p.collectFiles()
	if err != nil {
		return err
	}

	var prevTS int64
	for _, path := range files {
		if err := p.playFile(ctx, path, handler, &prevTS); err != nil {
			return err
		}
	}
	return nil
}

func (c PlaybackConfig) withDefaults() PlaybackConfig {
	if c.FilePrefix == "" {
		c.FilePrefix = defaultFilePrefix
	}
	return c
}

// Validate checks if the config is usable.
func (c PlaybackConfig) Validate() error {
	if c.Dir == "" {
		return fmt.Errorf("invalid playback config: Dir is empty")
	}
	if c.Speed < 0 {
		return fmt.Errorf("invalid playback config: Speed must be >= 0")
	}
	if c.MaxPayloadSize < 0 {
		return fmt.Errorf("invalid playback config: MaxPayloadSize must be >= 0")
	}
	return nil
}

func (p *Playback) collectFiles() ([]string, error) {
	entries, err := os.ReadDir(p.cfg.Dir)
	if err != nil {
		return nil, err
	}
	prefix := p.cfg.FilePrefix + "-"
	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".wal") {
			continue
		}
		files = append(files, filepath.Join(p.cfg.Dir, name))
	}
	sort.Strings(files)
	return files, nil
}

func (p *Playback) playFile(ctx context.Context, path string, handler func(schema.EventHeader, []byte) error, prevTS *int64) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	reader := NewReader(file, ReaderOptions{
		DisableChecksum: p.cfg.DisableChecksum,
		MaxPayloadSize:  p.cfg.MaxPayloadSize,
	})

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		header, payload, err := reader.Next()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("read %s: %w", path, err)
		}

		if err := p.pace(ctx, header, prevTS); err != nil {
			return err
		}
		if err := handler(header, payload); err != nil {
			return err
		}
	}
}

func (p *Playback) pace(ctx context.Context, header schema.EventHeader, prevTS *int64) error {
	if p.cfg.Speed <= 0 {
		return nil
	}
	current := header.TsEvent
	if p.cfg.UseRecvTime {
		current = header.TsRecv
	}
	if current <= 0 {
		return nil
	}
	if *prevTS > 0 {
		delta := current - *prevTS
		if delta > 0 {
			sleep := time.Duration(float64(delta) / p.cfg.Speed)
			if err := p.clock.Sleep(ctx, sleep); err != nil {
				return err
			}
		}
	}
	*prevTS = current
	return nil
}

package chaos

import (
	"fmt"
	"math/rand"
	"time"

	"main/internal/schema"
)

// Event wraps a WAL event for chaos processing.
type Event struct {
	Header  schema.EventHeader
	Payload []byte
}

// Config controls chaos injection behavior.
type Config struct {
	Seed          int64
	DropRate      float64
	DuplicateRate float64
	ReorderWindow int
	MaxDelay      time.Duration
}

// Engine applies chaos rules to events.
type Engine struct {
	cfg     Config
	rng     *rand.Rand
	pending []Event
}

// NewEngine creates a chaos engine with validation.
func NewEngine(cfg Config) (*Engine, error) {
	if cfg.ReorderWindow <= 0 {
		cfg.ReorderWindow = 1
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if cfg.Seed == 0 {
		cfg.Seed = time.Now().UTC().UnixNano()
	}
	return &Engine{
		cfg: cfg,
		rng: rand.New(rand.NewSource(cfg.Seed)),
	}, nil
}

// Validate ensures the config is within supported ranges.
func (c Config) Validate() error {
	if c.DropRate < 0 || c.DropRate > 1 {
		return fmt.Errorf("dropRate must be between 0 and 1")
	}
	if c.DuplicateRate < 0 || c.DuplicateRate > 1 {
		return fmt.Errorf("duplicateRate must be between 0 and 1")
	}
	if c.ReorderWindow <= 0 {
		return fmt.Errorf("reorderWindow must be >= 1")
	}
	if c.MaxDelay < 0 {
		return fmt.Errorf("maxDelay must be >= 0")
	}
	return nil
}

// Process applies chaos to a single event and returns any output events.
func (e *Engine) Process(ev Event) []Event {
	if e == nil {
		return []Event{ev}
	}
	if e.shouldDrop() {
		return nil
	}
	ev = e.applyDelay(ev)
	if e.cfg.ReorderWindow <= 1 {
		return e.applyDuplicate(ev)
	}
	e.pending = append(e.pending, ev)
	if len(e.pending) < e.cfg.ReorderWindow {
		return nil
	}
	idx := e.rng.Intn(len(e.pending))
	out := e.pending[idx]
	e.pending = append(e.pending[:idx], e.pending[idx+1:]...)
	return e.applyDuplicate(out)
}

// Flush returns any buffered events after processing completes.
func (e *Engine) Flush() []Event {
	if e == nil || len(e.pending) == 0 {
		return nil
	}
	out := make([]Event, 0, len(e.pending))
	for len(e.pending) > 0 {
		idx := e.rng.Intn(len(e.pending))
		ev := e.pending[idx]
		e.pending = append(e.pending[:idx], e.pending[idx+1:]...)
		out = append(out, e.applyDuplicate(ev)...)
	}
	return out
}

func (e *Engine) shouldDrop() bool {
	return e.cfg.DropRate > 0 && e.rng.Float64() < e.cfg.DropRate
}

func (e *Engine) applyDuplicate(ev Event) []Event {
	out := []Event{ev}
	if e.cfg.DuplicateRate > 0 && e.rng.Float64() < e.cfg.DuplicateRate {
		out = append(out, ev)
	}
	return out
}

func (e *Engine) applyDelay(ev Event) Event {
	if e.cfg.MaxDelay <= 0 {
		return ev
	}
	maxDelay := e.cfg.MaxDelay.Nanoseconds()
	if maxDelay <= 0 {
		return ev
	}
	delay := time.Duration(e.rng.Int63n(maxDelay + 1))
	if delay == 0 {
		return ev
	}
	if ev.Header.TsRecv > 0 {
		ev.Header.TsRecv += int64(delay)
		return ev
	}
	if ev.Header.TsEvent > 0 {
		ev.Header.TsRecv = ev.Header.TsEvent + int64(delay)
	}
	return ev
}

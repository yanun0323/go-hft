package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"main/internal/schema"
)

// Snapshot captures position quantities at a point in time.
type Snapshot struct {
	Timestamp   int64           `json:"timestamp"`
	LastSeq     uint64          `json:"lastSeq"`
	LastEventTs int64           `json:"lastEventTs"`
	Positions   []PositionEntry `json:"positions"`
}

// PositionEntry is a single symbol position entry.
type PositionEntry struct {
	SymbolID uint32         `json:"symbolId"`
	Qty      schema.Quantity `json:"qty"`
}

// Snapshot builds a snapshot from current positions.
func (r *PositionReducer) Snapshot() Snapshot {
	return r.SnapshotWithMeta(0, 0)
}

// SnapshotWithMeta builds a snapshot with event metadata.
func (r *PositionReducer) SnapshotWithMeta(lastSeq uint64, lastEventTs int64) Snapshot {
	entries := make([]PositionEntry, 0, len(r.positions))
	for symbolID, qty := range r.positions {
		entries = append(entries, PositionEntry{
			SymbolID: symbolID,
			Qty:      qty,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].SymbolID < entries[j].SymbolID
	})
	return Snapshot{
		Timestamp:   time.Now().UTC().UnixNano(),
		LastSeq:     lastSeq,
		LastEventTs: lastEventTs,
		Positions:   entries,
	}
}

// WriteSnapshot writes a snapshot to disk as JSON.
func WriteSnapshot(path string, snapshot Snapshot) error {
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0o644)
}

// ReadSnapshot loads a snapshot from disk.
func ReadSnapshot(path string) (Snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Snapshot{}, err
	}
	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return Snapshot{}, err
	}
	return snap, nil
}

// CompareSnapshots checks if two snapshots match.
func CompareSnapshots(expected, actual Snapshot) error {
	if len(expected.Positions) != len(actual.Positions) {
		return fmt.Errorf("snapshot length mismatch: expected=%d actual=%d", len(expected.Positions), len(actual.Positions))
	}
	expectedMap := make(map[uint32]schema.Quantity, len(expected.Positions))
	for _, entry := range expected.Positions {
		expectedMap[entry.SymbolID] = entry.Qty
	}
	for _, entry := range actual.Positions {
		want, ok := expectedMap[entry.SymbolID]
		if !ok {
			return fmt.Errorf("snapshot missing symbol: %d", entry.SymbolID)
		}
		if want != entry.Qty {
			return fmt.Errorf("snapshot qty mismatch: symbol=%d expected=%d actual=%d", entry.SymbolID, want, entry.Qty)
		}
	}
	return nil
}

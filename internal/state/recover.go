package state

import (
	"context"
	"fmt"

	"main/internal/codec"
	"main/internal/recorder"
	"main/internal/schema"
)

// RecoverConfig controls snapshot + WAL recovery.
type RecoverConfig struct {
	WALDir          string
	SnapshotPath    string
	FilePrefix      string
	DisableChecksum bool
	MaxPayloadSize  int
	UseRecvTime     bool
}

// RecoverResult contains recovered state and metadata.
type RecoverResult struct {
	Positions   *PositionReducer
	LastSeq     uint64
	LastEventTs int64
}

// RecoverPositions loads a snapshot and replays WAL tail to rebuild positions.
func RecoverPositions(ctx context.Context, cfg RecoverConfig) (RecoverResult, error) {
	if cfg.WALDir == "" {
		return RecoverResult{}, fmt.Errorf("wal dir is empty")
	}
	positions := NewPositionReducer()
	var lastSeq uint64
	var lastEventTs int64

	if cfg.SnapshotPath != "" {
		snapshot, err := ReadSnapshot(cfg.SnapshotPath)
		if err != nil {
			return RecoverResult{}, err
		}
		positions.ApplySnapshot(snapshot)
		lastSeq = snapshot.LastSeq
		lastEventTs = snapshot.LastEventTs
	}

	playbackCfg := recorder.PlaybackConfig{
		Dir:             cfg.WALDir,
		FilePrefix:      cfg.FilePrefix,
		Speed:           0,
		UseRecvTime:     cfg.UseRecvTime,
		DisableChecksum: cfg.DisableChecksum,
		MaxPayloadSize:  cfg.MaxPayloadSize,
	}
	pb, err := recorder.NewPlayback(playbackCfg)
	if err != nil {
		return RecoverResult{}, err
	}

	err = pb.Run(ctx, func(header schema.EventHeader, payload []byte) error {
		if lastSeq > 0 && header.Seq <= lastSeq {
			return nil
		}
		if lastSeq == 0 && lastEventTs > 0 {
			ts := header.TsEvent
			if cfg.UseRecvTime {
				ts = header.TsRecv
			}
			if ts <= lastEventTs {
				return nil
			}
		}
		if header.Seq > lastSeq {
			lastSeq = header.Seq
		}
		if header.TsEvent > lastEventTs {
			lastEventTs = header.TsEvent
		}

		if header.Type != schema.EventFill {
			return nil
		}
		fill, ok := codec.DecodeFill(payload)
		if !ok {
			return fmt.Errorf("decode fill failed")
		}
		positions.ApplyFill(fill)
		return nil
	})
	if err != nil {
		return RecoverResult{}, err
	}

	return RecoverResult{
		Positions:   positions,
		LastSeq:     lastSeq,
		LastEventTs: lastEventTs,
	}, nil
}

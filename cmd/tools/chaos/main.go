package main

import (
	"context"
	"flag"
	"log"

	"main/internal/chaos"
	"main/internal/recorder"
	"main/internal/schema"
)

func main() {
	inputDir := flag.String("input-dir", "testdata/wal", "Input WAL directory")
	inputPrefix := flag.String("input-prefix", "", "Input WAL file prefix (default: wal)")
	outputDir := flag.String("output-dir", "testdata/wal_chaos", "Output WAL directory")
	outputPrefix := flag.String("output-prefix", "chaos", "Output WAL file prefix")
	seed := flag.Int64("seed", 0, "RNG seed (0=now)")
	dropRate := flag.Float64("drop-rate", 0, "Drop probability [0-1]")
	dupRate := flag.Float64("dup-rate", 0, "Duplicate probability [0-1]")
	reorderWindow := flag.Int("reorder-window", 1, "Reorder window (>=1)")
	maxDelay := flag.Duration("max-delay", 0, "Max receive delay")
	noChecksum := flag.Bool("no-checksum", false, "Disable checksum validation")
	maxPayload := flag.Int("max-payload", 0, "Max payload size in bytes (0=unlimited)")
	flag.Parse()

	pb, err := recorder.NewPlayback(recorder.PlaybackConfig{
		Dir:             *inputDir,
		FilePrefix:      *inputPrefix,
		DisableChecksum: *noChecksum,
		MaxPayloadSize:  *maxPayload,
	})
	if err != nil {
		log.Fatalf("playback init failed: %v", err)
	}

	engine, err := chaos.NewEngine(chaos.Config{
		Seed:          *seed,
		DropRate:      *dropRate,
		DuplicateRate: *dupRate,
		ReorderWindow: *reorderWindow,
		MaxDelay:      *maxDelay,
	})
	if err != nil {
		log.Fatalf("chaos config invalid: %v", err)
	}

	outCfg := recorder.DefaultConfig(*outputDir)
	outCfg.FilePrefix = *outputPrefix
	outCfg.CopyPayload = true
	writer, err := recorder.NewWriter(outCfg)
	if err != nil {
		log.Fatalf("writer init failed: %v", err)
	}
	ctx := context.Background()
	if err := writer.Start(ctx); err != nil {
		log.Fatalf("writer start failed: %v", err)
	}

	var seq uint64
	err = pb.Run(ctx, func(header schema.EventHeader, payload []byte) error {
		ev := chaos.Event{
			Header:  header,
			Payload: copyPayload(payload),
		}
		for _, out := range engine.Process(ev) {
			if err := appendEvent(writer, &seq, out); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		for _, out := range engine.Flush() {
			if err := appendEvent(writer, &seq, out); err != nil {
				log.Fatalf("append failed: %v", err)
			}
		}
	}

	if err != nil {
		log.Fatalf("playback failed: %v", err)
	}
	if err := writer.Close(); err != nil {
		log.Fatalf("writer close failed: %v", err)
	}
}

func appendEvent(writer *recorder.Writer, seq *uint64, ev chaos.Event) error {
	ev.Header.Seq = nextSeq(seq)
	if ev.Header.Version == 0 {
		ev.Header.Version = schema.SchemaVersion
	}
	return writer.TryAppend(ev.Header, ev.Payload)
}

func nextSeq(seq *uint64) uint64 {
	*seq += 1
	return *seq
}

func copyPayload(payload []byte) []byte {
	if len(payload) == 0 {
		return nil
	}
	cp := make([]byte, len(payload))
	copy(cp, payload)
	return cp
}

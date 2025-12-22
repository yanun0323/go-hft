package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"sync"
	"time"

	"main/internal/bus"
	"main/internal/codec"
	"main/internal/mdg"
	"main/internal/obs"
	"main/internal/ops"
	"main/internal/recorder"
	"main/internal/schema"
)

func main() {
	walDir := flag.String("wal-dir", "testdata/wal", "WAL directory for market data")
	configPath := flag.String("config", "", "Path to JSON config")
	ticks := flag.Int("ticks", 10, "Number of ticks to generate")
	interval := flag.Duration("interval", 0, "Delay between ticks")
	basePrice := flag.Int64("base-price", 100, "Base price (scaled)")
	baseSize := flag.Int64("base-size", 1, "Base size (scaled)")
	spread := flag.Int64("spread", 1, "Bid/ask spread (scaled)")
	source := flag.Uint("source", 1, "Source ID")
	kind := flag.String("kind", "quote", "Market data kind: quote|trade")
	flag.Parse()

	if *ticks <= 0 {
		log.Fatalf("ticks must be > 0")
	}

	registry, err := loadRegistry(*configPath)
	if err != nil {
		log.Fatalf("registry load failed: %v", err)
	}
	mdKind, err := parseKind(*kind)
	if err != nil {
		log.Fatalf("invalid kind: %v", err)
	}

	generator, err := mdg.NewGenerator(registry, mdKind, uint16(*source), *basePrice, *baseSize, *spread)
	if err != nil {
		log.Fatalf("generator init failed: %v", err)
	}
	normalizer := mdg.NewNormalizer(registry)

	ctx := context.Background()
	cfg := recorder.DefaultConfig(*walDir)
	writer, err := recorder.NewWriter(cfg)
	if err != nil {
		log.Fatalf("wal init failed: %v", err)
	}
	if err := writer.Start(ctx); err != nil {
		log.Fatalf("wal start failed: %v", err)
	}

	queue := bus.NewQueue(1024)
	errCh := make(chan error, 1)
	var wg sync.WaitGroup
	metrics := obs.NewMetrics()
	traceGen := obs.NewTraceGenerator(0)

	wg.Add(1)
	go func() {
		defer wg.Done()
		queue.Run(ctx, func(e bus.Event) {
			if err := writer.TryAppend(e.Header, e.Payload); err != nil {
				select {
				case errCh <- err:
				default:
				}
			}
		})
	}()

	seq := uint64(0)
	for i := 0; i < *ticks; i++ {
		seq++
		now := time.Now().UTC()
		raw := generator.Next(now)
		header, md, err := normalizer.Normalize(seq, raw)
		if err != nil {
			log.Fatalf("normalize failed: %v", err)
		}
		header.TraceID = traceGen.Next()
		payload := codec.EncodeMarketData(nil, md)
		if err := queue.TryPublish(bus.Event{Header: header, Payload: payload}); err != nil {
			if err == bus.ErrQueueFull {
				metrics.IncQueueDrop()
			} else if err == bus.ErrQueueClosed {
				metrics.IncQueueClosed()
			}
			log.Fatalf("publish failed: %v", err)
		}
		metrics.ObserveEvent(header)
		if *interval > 0 && i < *ticks-1 {
			time.Sleep(*interval)
		}
	}

	queue.Close()
	wg.Wait()

	var appendErr error
	select {
	case appendErr = <-errCh:
	default:
	}

	if err := writer.Close(); err != nil {
		log.Fatalf("wal close failed: %v", err)
	}
	if appendErr != nil {
		log.Fatalf("wal append failed: %v", appendErr)
	}
	snapshot := metrics.Snapshot()
	log.Printf("metrics: events=%v drops=%d closed=%d event_latency=%+v",
		snapshot.EventCounts, snapshot.QueueDrops, snapshot.QueueClosed, snapshot.EventLatency)
}

func loadRegistry(path string) (*schema.Registry, error) {
	if path == "" {
		return defaultRegistry()
	}
	return ops.LoadRegistry(path)
}

func defaultRegistry() (*schema.Registry, error) {
	reg := schema.NewRegistry()
	venueID, err := reg.AddVenue("SIM")
	if err != nil {
		return nil, err
	}
	scale := schema.ScaleSpec{
		PriceScale:    8,
		QuantityScale: 8,
		NotionalScale: 8,
		FeeScale:      8,
	}
	if _, err := reg.AddSymbol("TEST-USD", venueID, scale); err != nil {
		return nil, err
	}
	return reg, nil
}

func parseKind(kind string) (schema.MarketDataKind, error) {
	switch kind {
	case "quote":
		return schema.MarketDataQuote, nil
	case "trade":
		return schema.MarketDataTrade, nil
	default:
		return schema.MarketDataUnknown, fmt.Errorf("unsupported kind: %s", kind)
	}
}

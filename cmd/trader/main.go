package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"main/internal/bus"
	"main/internal/codec"
	"main/internal/obs"
	"main/internal/og"
	"main/internal/ops"
	"main/internal/recorder"
	"main/internal/risk"
	"main/internal/schema"
	"main/internal/state"
)

type runtimeConfig struct {
	v atomic.Value
}

func newRuntimeConfig(loaded ops.Loaded) *runtimeConfig {
	var rc runtimeConfig
	rc.v.Store(loaded)
	return &rc
}

func (r *runtimeConfig) Load() ops.Loaded {
	return r.v.Load().(ops.Loaded)
}

func (r *runtimeConfig) Update(loaded ops.Loaded) {
	r.v.Store(loaded)
}

func main() {
	walDir := flag.String("wal-dir", "testdata/wal", "WAL directory for recording")
	configPath := flag.String("config", "", "Path to JSON config")
	configReload := flag.Duration("config-reload-interval", 2*time.Second, "Config reload interval (0=disable)")
	orderCount := flag.Int("order-count", 1, "Number of orders to publish in record mode")
	orderInterval := flag.Duration("order-interval", 0, "Delay between orders in record mode")
	snapshotPath := flag.String("snapshot-path", "", "Position snapshot output (default: <wal-dir>/positions.json)")
	recoverEnabled := flag.Bool("recover", false, "Recover positions from snapshot + WAL")
	recoverSnapshot := flag.String("recover-snapshot", "", "Snapshot path for recovery (default: <wal-dir>/positions.json)")
	recoverPrefix := flag.String("recover-prefix", "", "WAL file prefix for recovery (default: wal)")
	recoverNoChecksum := flag.Bool("recover-no-checksum", false, "Disable checksum validation for recovery")
	recoverMaxPayload := flag.Int("recover-max-payload", 0, "Max payload size in bytes for recovery (0=unlimited)")

	replayDir := flag.String("replay-dir", "", "WAL directory for replay mode")
	replayPrefix := flag.String("replay-prefix", "", "WAL file prefix (default: wal)")
	replaySpeed := flag.Float64("replay-speed", 0, "Playback speed (1=real-time, 0=no pacing)")
	replayUseRecv := flag.Bool("replay-use-recv-time", false, "Use receive timestamp for pacing")
	replayNoChecksum := flag.Bool("replay-no-checksum", false, "Disable checksum validation")
	replayMaxPayload := flag.Int("replay-max-payload", 0, "Max payload size in bytes (0=unlimited)")
	replaySnapshot := flag.String("replay-snapshot", "", "Snapshot path for replay verification (default: <replay-dir>/positions.json)")
	replayVerifySnapshot := flag.Bool("replay-verify-snapshot", true, "Verify positions against snapshot after replay")
	flag.Parse()

	ctx := context.Background()
	if *replayDir != "" {
		cfg := recorder.PlaybackConfig{
			Dir:             *replayDir,
			FilePrefix:      *replayPrefix,
			Speed:           *replaySpeed,
			UseRecvTime:     *replayUseRecv,
			DisableChecksum: *replayNoChecksum,
			MaxPayloadSize:  *replayMaxPayload,
		}
		snapshotIn := resolveSnapshotPath(*replayDir, *replaySnapshot)
		if err := runReplay(ctx, cfg, snapshotIn, *replayVerifySnapshot); err != nil {
			log.Fatalf("replay failed: %v", err)
		}
		return
	}

	loaded, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("config load failed: %v", err)
	}
	runtime := newRuntimeConfig(loaded)
	if *configPath != "" && *configReload > 0 {
		go watchConfig(ctx, *configPath, *configReload, runtime.Update)
	}

	snapshotOut := resolveSnapshotPath(*walDir, *snapshotPath)
	var recoverCfg *state.RecoverConfig
	if *recoverEnabled {
		recoverPath := resolveSnapshotPath(*walDir, *recoverSnapshot)
		recoverCfg = &state.RecoverConfig{
			WALDir:          *walDir,
			SnapshotPath:    recoverPath,
			FilePrefix:      *recoverPrefix,
			DisableChecksum: *recoverNoChecksum,
			MaxPayloadSize:  *recoverMaxPayload,
		}
	}
	if err := runRecord(ctx, *walDir, runtime, *orderCount, *orderInterval, snapshotOut, recoverCfg); err != nil {
		log.Fatalf("record failed: %v", err)
	}
}

func runRecord(ctx context.Context, dir string, runtime *runtimeConfig, orderCount int, orderInterval time.Duration, snapshotPath string, recoverCfg *state.RecoverConfig) error {
	if orderCount <= 0 {
		return fmt.Errorf("order-count must be > 0")
	}
	cfg := recorder.DefaultConfig(dir)
	w, err := recorder.NewWriter(cfg)
	if err != nil {
		return err
	}
	if err := w.Start(ctx); err != nil {
		return err
	}

	queue := bus.NewQueue(1024)
	errCh := make(chan error, 1)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		queue.Run(ctx, func(e bus.Event) {
			if err := w.TryAppend(e.Header, e.Payload); err != nil {
				select {
				case errCh <- err:
				default:
				}
			}
		})
	}()

	seq := uint64(0)
	var lastEventTs int64
	positions := state.NewPositionReducer()
	metrics := obs.NewMetrics()
	traceGen := obs.NewTraceGenerator(0)
	if recoverCfg != nil {
		recovered, err := state.RecoverPositions(ctx, *recoverCfg)
		if err != nil {
			return err
		}
		positions = recovered.Positions
		seq = recovered.LastSeq
		lastEventTs = recovered.LastEventTs
		log.Printf("recovered positions=%d last_seq=%d", positions.Count(), seq)
	}

	now := time.Now().UTC().UnixNano()
	traceID := traceGen.Next()
	if err := publishEvent(queue, schema.EventStrategyDecision, &seq, now, []byte("dummy event"), traceID, &lastEventTs, metrics); err != nil {
		return err
	}

	gateway := og.NewGateway(og.GatewayConfig{Session: "SIM", ResendOnReconnect: true})
	var orderCounter uint64
	loaded := runtime.Load()
	engine := risk.NewEngine(loaded.Risk)
	riskVersion := loaded.Risk.Version

	for i := 0; i < orderCount; i++ {
		loaded = runtime.Load()
		if loaded.Risk.Version != riskVersion {
			engine = risk.NewEngine(loaded.Risk)
			riskVersion = loaded.Risk.Version
		}
		if loaded.Features.EnableOrderFlow {
			orderID := loaded.Order.OrderID + orderCounter
			orderCounter++
			if err := publishOrderFlow(queue, gateway, engine, positions, loaded, orderID, &seq, &lastEventTs, traceGen, metrics); err != nil {
				return err
			}
		}
		if orderInterval > 0 && i < orderCount-1 {
			time.Sleep(orderInterval)
		}
	}

	queue.Close()
	wg.Wait()

	var appendErr error
	select {
	case appendErr = <-errCh:
	default:
	}

	if err := w.Close(); err != nil {
		return err
	}
	if appendErr != nil {
		return appendErr
	}
	if snapshotPath != "" {
		snapshot := positions.SnapshotWithMeta(seq, lastEventTs)
		if err := state.WriteSnapshot(snapshotPath, snapshot); err != nil {
			return err
		}
	}
	snapshot := metrics.Snapshot()
	log.Printf("metrics: events=%v risk_reasons=%v drops=%d closed=%d order_flow=%+v risk_eval=%+v event_latency=%+v",
		snapshot.EventCounts, snapshot.RiskReasonCounts, snapshot.QueueDrops, snapshot.QueueClosed,
		snapshot.OrderFlowLatency, snapshot.RiskEvalLatency, snapshot.EventLatency)
	return nil
}

func runReplay(ctx context.Context, cfg recorder.PlaybackConfig, snapshotPath string, verifySnapshot bool) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	queue := bus.NewQueue(1024)
	errCh := make(chan error, 1)
	var wg sync.WaitGroup
	counts := make(map[schema.EventType]int)
	total := 0
	orders := og.NewStateMachine()
	positions := state.NewPositionReducer()

	wg.Add(1)
	go func() {
		defer wg.Done()
		queue.Run(ctx, func(e bus.Event) {
			total++
			counts[e.Header.Type]++
			if err := applyReplayEvent(orders, positions, e); err != nil {
				select {
				case errCh <- err:
				default:
				}
				cancel()
			}
		})
	}()

	pb, err := recorder.NewPlayback(cfg)
	if err != nil {
		return err
	}
	err = pb.Run(ctx, func(header schema.EventHeader, payload []byte) error {
		var copied []byte
		if len(payload) > 0 {
			copied = make([]byte, len(payload))
			copy(copied, payload)
		}
		return queue.TryPublish(bus.Event{Header: header, Payload: copied})
	})

	queue.Close()
	wg.Wait()

	if err != nil {
		return err
	}
	var applyErr error
	select {
	case applyErr = <-errCh:
	default:
	}
	if applyErr != nil {
		return applyErr
	}
	if verifySnapshot {
		if snapshotPath == "" {
			return fmt.Errorf("snapshot path is empty")
		}
		expected, err := state.ReadSnapshot(snapshotPath)
		if err != nil {
			return err
		}
		actual := positions.Snapshot()
		if err := state.CompareSnapshots(expected, actual); err != nil {
			return err
		}
		log.Printf("snapshot verified: positions=%d", len(actual.Positions))
	}
	log.Printf("replay completed: total=%d counts=%v positions=%d", total, counts, positions.Count())
	return nil
}

func loadConfig(path string) (ops.Loaded, error) {
	if path == "" {
		return defaultLoaded()
	}
	return ops.Load(path)
}

func defaultLoaded() (ops.Loaded, error) {
	reg := schema.NewRegistry()
	venueID, err := reg.AddVenue("SIM")
	if err != nil {
		return ops.Loaded{}, err
	}
	scale := schema.ScaleSpec{
		PriceScale:    8,
		QuantityScale: 8,
		NotionalScale: 8,
		FeeScale:      8,
	}
	symbolID, err := reg.AddSymbol("TEST-USD", venueID, scale)
	if err != nil {
		return ops.Loaded{}, err
	}
	return ops.Loaded{
		Registry: reg,
		Risk: risk.Config{
			MaxOrderQty:      schema.Quantity(1000),
			MaxOrderNotional: schema.Notional(1_000_000),
			MaxPosition:      schema.Quantity(5_000),
		},
		Order: ops.OrderSpec{
			OrderID:     1001,
			StrategyID:  1,
			SymbolID:    symbolID,
			Side:        schema.OrderSideBuy,
			Type:        schema.OrderTypeLimit,
			TimeInForce: schema.TimeInForceGTC,
			Price:       schema.Price(100),
			Qty:         schema.Quantity(10),
		},
		Features: ops.FeatureFlags{
			EnableOrderFlow: true,
			EnableFills:     true,
		},
	}, nil
}

func resolveSnapshotPath(dir string, path string) string {
	if path != "" {
		return path
	}
	return filepath.Join(dir, "positions.json")
}

func watchConfig(ctx context.Context, path string, interval time.Duration, update func(ops.Loaded)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var lastMod time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			info, err := os.Stat(path)
			if err != nil {
				log.Printf("config stat failed: %v", err)
				continue
			}
			if !info.ModTime().After(lastMod) {
				continue
			}
			loaded, err := ops.Load(path)
			if err != nil {
				log.Printf("config reload failed: %v", err)
				continue
			}
			update(loaded)
			lastMod = info.ModTime()
			log.Printf("config reloaded: %s", path)
		}
	}
}

func publishOrderFlow(queue *bus.Queue, gateway *og.Gateway, engine *risk.Engine, positions *state.PositionReducer, loaded ops.Loaded, orderID uint64, seq *uint64, lastEventTs *int64, traceGen *obs.TraceGenerator, metrics *obs.Metrics) error {
	flowStart := time.Now()
	spec := loaded.Order
	traceID := orderID
	if traceGen != nil {
		traceID = traceGen.Next()
	}
	intent := schema.OrderIntent{
		OrderID:     orderID,
		StrategyID:  spec.StrategyID,
		SymbolID:    uint32(spec.SymbolID),
		Side:        spec.Side,
		Type:        spec.Type,
		TimeInForce: spec.TimeInForce,
		Price:       spec.Price,
		Qty:         spec.Qty,
	}
	if err := gateway.Send(intent); err != nil {
		if !errors.Is(err, og.ErrGatewayDisconnected) {
			return err
		}
	}

	intentTs := time.Now().UTC().UnixNano()
	intentPayload := codec.EncodeOrderIntent(nil, intent)
	if err := publishEvent(queue, schema.EventOrderIntent, seq, intentTs, intentPayload, traceID, lastEventTs, metrics); err != nil {
		return err
	}

	evalStart := time.Now()
	refPrice := spec.Price
	if refPrice == 0 {
		refPrice = intent.Price
	}
	decision := engine.Evaluate(intent, risk.StateView{
		Position:       positions.Position(intent.SymbolID),
		ReferencePrice: refPrice,
		Now:            intentTs,
	})
	if metrics != nil {
		metrics.ObserveRiskEval(time.Since(evalStart))
		metrics.IncRiskReason(decision.Reason)
	}
	decisionTs := time.Now().UTC().UnixNano()
	decisionPayload := codec.EncodeRiskDecision(nil, decision)
	if err := publishEvent(queue, schema.EventRiskDecision, seq, decisionTs, decisionPayload, traceID, lastEventTs, metrics); err != nil {
		return err
	}

	ack := schema.OrderAck{
		OrderID:   intent.OrderID,
		SymbolID:  intent.SymbolID,
		Status:    schema.OrderAckStatusAcked,
		Reason:    schema.OrderAckReasonNone,
		Price:     intent.Price,
		Qty:       intent.Qty,
		LeavesQty: intent.Qty,
	}
	if decision.Action != schema.RiskActionAllow {
		ack.Status = schema.OrderAckStatusRejected
		ack.Reason = schema.OrderAckReasonRiskReject
		ack.LeavesQty = 0
	}
	if err := gateway.OnAck(ack); err != nil {
		return err
	}
	ackTs := time.Now().UTC().UnixNano()
	ackPayload := codec.EncodeOrderAck(nil, ack)
	if err := publishEvent(queue, schema.EventOrderAck, seq, ackTs, ackPayload, traceID, lastEventTs, metrics); err != nil {
		return err
	}

	if decision.Action == schema.RiskActionAllow && loaded.Features.EnableFills {
		fill := schema.Fill{
			OrderID:  intent.OrderID,
			SymbolID: intent.SymbolID,
			Side:     intent.Side,
			Price:    intent.Price,
			Qty:      intent.Qty,
			Fee:      0,
		}
		if err := gateway.OnFill(fill); err != nil {
			return err
		}
		positions.ApplyFill(fill)
		fillTs := time.Now().UTC().UnixNano()
		fillPayload := codec.EncodeFill(nil, fill)
		if err := publishEvent(queue, schema.EventFill, seq, fillTs, fillPayload, traceID, lastEventTs, metrics); err != nil {
			return err
		}
	}

	if metrics != nil {
		metrics.ObserveOrderFlow(time.Since(flowStart))
	}
	return nil
}

func publishEvent(queue *bus.Queue, eventType schema.EventType, seq *uint64, ts int64, payload []byte, traceID uint64, lastEventTs *int64, metrics *obs.Metrics) error {
	next := nextSeq(seq)
	if lastEventTs != nil {
		*lastEventTs = ts
	}
	header := schema.NewHeader(eventType, 1, next, ts, ts)
	if traceID == 0 {
		traceID = next
	}
	header.TraceID = traceID
	err := queue.TryPublish(bus.Event{Header: header, Payload: payload})
	if metrics != nil {
		if err != nil {
			if errors.Is(err, bus.ErrQueueFull) {
				metrics.IncQueueDrop()
			} else if errors.Is(err, bus.ErrQueueClosed) {
				metrics.IncQueueClosed()
			}
		} else {
			metrics.ObserveEvent(header)
		}
	}
	return err
}

func nextSeq(seq *uint64) uint64 {
	*seq += 1
	return *seq
}

func applyReplayEvent(orders *og.StateMachine, positions *state.PositionReducer, e bus.Event) error {
	switch e.Header.Type {
	case schema.EventOrderIntent:
		intent, ok := codec.DecodeOrderIntent(e.Payload)
		if !ok {
			return fmt.Errorf("decode order intent failed")
		}
		_, err := orders.ApplyIntent(intent)
		return err
	case schema.EventOrderAck:
		ack, ok := codec.DecodeOrderAck(e.Payload)
		if !ok {
			return fmt.Errorf("decode order ack failed")
		}
		_, err := orders.ApplyAck(ack)
		return err
	case schema.EventFill:
		fill, ok := codec.DecodeFill(e.Payload)
		if !ok {
			return fmt.Errorf("decode fill failed")
		}
		if _, err := orders.ApplyFill(fill); err != nil {
			return err
		}
		positions.ApplyFill(fill)
		return nil
	default:
		return nil
	}
}

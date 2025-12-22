package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"time"

	"main/internal/codec"
	"main/internal/obs"
	"main/internal/og"
	"main/internal/ops"
	"main/internal/recorder"
	"main/internal/risk"
	"main/internal/schema"
	"main/internal/state"
)

func main() {
	inputDir := flag.String("input-dir", "testdata/wal", "Input WAL directory")
	inputPrefix := flag.String("input-prefix", "", "Input WAL file prefix (default: wal)")
	outputDir := flag.String("output-dir", "testdata/wal_paper", "Output WAL directory")
	outputPrefix := flag.String("output-prefix", "paper", "Output WAL file prefix")
	configPath := flag.String("config", "", "Path to JSON config")
	orderEvery := flag.Int("order-every", 10, "Generate one order every N market data events (0=disable)")
	maxOrders := flag.Int("max-orders", 0, "Maximum orders to generate (0=unlimited)")
	includeMD := flag.Bool("include-md", true, "Pass through market data events to output WAL")
	includeNonMD := flag.Bool("include-non-md", false, "Pass through non-market data events")
	noChecksum := flag.Bool("no-checksum", false, "Disable checksum validation")
	maxPayload := flag.Int("max-payload", 0, "Max payload size in bytes (0=unlimited)")
	flag.Parse()

	if *orderEvery < 0 {
		log.Fatalf("order-every must be >= 0")
	}
	if *maxOrders < 0 {
		log.Fatalf("max-orders must be >= 0")
	}

	loaded, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("config load failed: %v", err)
	}

	playback, err := recorder.NewPlayback(recorder.PlaybackConfig{
		Dir:             *inputDir,
		FilePrefix:      *inputPrefix,
		DisableChecksum: *noChecksum,
		MaxPayloadSize:  *maxPayload,
	})
	if err != nil {
		log.Fatalf("playback init failed: %v", err)
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

	engine := risk.NewEngine(loaded.Risk)
	positions := state.NewPositionReducer()
	gateway := og.NewGateway(og.GatewayConfig{Session: "PAPER", ResendOnReconnect: true})
	traceGen := obs.NewTraceGenerator(0)

	var seq uint64
	var mdCount int
	var orderCount int
	var refPrice schema.Price

	err = playback.Run(ctx, func(header schema.EventHeader, payload []byte) error {
		if header.Type != schema.EventMarketData {
			if *includeNonMD {
				return appendPassthrough(writer, &seq, header, payload)
			}
			return nil
		}
		mdCount++
		md, ok := codec.DecodeMarketData(payload)
		if !ok {
			return fmt.Errorf("decode market data failed")
		}
		if price := referencePrice(md); price > 0 {
			refPrice = price
		}
		if *includeMD {
			if err := appendPassthrough(writer, &seq, header, payload); err != nil {
				return err
			}
		}
		if !loaded.Features.EnableOrderFlow || *orderEvery == 0 {
			return nil
		}
		if *orderEvery > 0 && mdCount%*orderEvery != 0 {
			return nil
		}
		if *maxOrders > 0 && orderCount >= *maxOrders {
			return nil
		}
		orderID := loaded.Order.OrderID + uint64(orderCount)
		orderCount++
		now := header.TsEvent
		if now == 0 {
			now = header.TsRecv
		}
		if now == 0 {
			now = time.Now().UTC().UnixNano()
		}
		traceID := traceGen.Next()
		if err := publishPaperOrder(writer, engine, gateway, positions, loaded, orderID, refPrice, now, traceID, &seq); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		log.Fatalf("playback failed: %v", err)
	}
	if err := writer.Close(); err != nil {
		log.Fatalf("writer close failed: %v", err)
	}

	log.Printf("paper completed: md=%d orders=%d positions=%d", mdCount, orderCount, positions.Count())
}

func publishPaperOrder(writer *recorder.Writer, engine *risk.Engine, gateway *og.Gateway, positions *state.PositionReducer, loaded ops.Loaded, orderID uint64, refPrice schema.Price, now int64, traceID uint64, seq *uint64) error {
	spec := loaded.Order
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

	intentPayload := codec.EncodeOrderIntent(nil, intent)
	if err := appendEvent(writer, seq, schema.EventOrderIntent, now, traceID, intentPayload); err != nil {
		return err
	}

	ref := refPrice
	if ref == 0 {
		ref = intent.Price
	}
	decision := engine.Evaluate(intent, risk.StateView{
		Position:       positions.Position(intent.SymbolID),
		ReferencePrice: ref,
		Now:            now,
	})
	decisionPayload := codec.EncodeRiskDecision(nil, decision)
	if err := appendEvent(writer, seq, schema.EventRiskDecision, now, traceID, decisionPayload); err != nil {
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
	ackPayload := codec.EncodeOrderAck(nil, ack)
	if err := appendEvent(writer, seq, schema.EventOrderAck, now, traceID, ackPayload); err != nil {
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
		fillPayload := codec.EncodeFill(nil, fill)
		if err := appendEvent(writer, seq, schema.EventFill, now, traceID, fillPayload); err != nil {
			return err
		}
	}

	return nil
}

func appendEvent(writer *recorder.Writer, seq *uint64, eventType schema.EventType, ts int64, traceID uint64, payload []byte) error {
	header := schema.NewHeader(eventType, 1, nextSeq(seq), ts, ts)
	if traceID == 0 {
		traceID = header.Seq
	}
	header.TraceID = traceID
	return writer.TryAppend(header, payload)
}

func appendPassthrough(writer *recorder.Writer, seq *uint64, header schema.EventHeader, payload []byte) error {
	header.Seq = nextSeq(seq)
	if header.Version == 0 {
		header.Version = schema.SchemaVersion
	}
	if header.TraceID == 0 {
		header.TraceID = header.Seq
	}
	return writer.TryAppend(header, payload)
}

func nextSeq(seq *uint64) uint64 {
	*seq += 1
	return *seq
}

func referencePrice(md schema.MarketData) schema.Price {
	if md.Kind == schema.MarketDataTrade && md.Price > 0 {
		return md.Price
	}
	if md.BidPrice > 0 && md.AskPrice > 0 {
		return schema.Price((int64(md.BidPrice) + int64(md.AskPrice)) / 2)
	}
	if md.BidPrice > 0 {
		return md.BidPrice
	}
	if md.AskPrice > 0 {
		return md.AskPrice
	}
	return md.Price
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

package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"main/internal/codec"
	"main/internal/recorder"
	"main/internal/schema"
)

func main() {
	dir := flag.String("dir", "testdata/wal", "WAL directory")
	prefix := flag.String("prefix", "", "WAL file prefix (default: wal)")
	speed := flag.Float64("speed", 0, "Playback speed (1=real-time, 0=no pacing)")
	useRecv := flag.Bool("use-recv-time", false, "Use receive timestamp for pacing")
	noChecksum := flag.Bool("no-checksum", false, "Disable checksum validation")
	maxPayload := flag.Int("max-payload", 0, "Max payload size in bytes (0=unlimited)")
	decode := flag.Bool("decode", false, "Decode known payload types")
	flag.Parse()

	cfg := recorder.PlaybackConfig{
		Dir:             *dir,
		FilePrefix:      *prefix,
		Speed:           *speed,
		UseRecvTime:     *useRecv,
		DisableChecksum: *noChecksum,
		MaxPayloadSize:  *maxPayload,
	}
	pb, err := recorder.NewPlayback(cfg)
	if err != nil {
		log.Fatalf("playback init failed: %v", err)
	}

	ctx := context.Background()
	var index int
	err = pb.Run(ctx, func(header schema.EventHeader, payload []byte) error {
		index++
		fmt.Printf("%06d seq=%d type=%s ts_event=%d ts_recv=%d len=%d\n", index, header.Seq, eventTypeName(header.Type), header.TsEvent, header.TsRecv, len(payload))
		if *decode {
			printDecoded(header.Type, payload)
		}
		return nil
	})
	if err != nil {
		log.Fatalf("playback run failed: %v", err)
	}
}

func eventTypeName(t schema.EventType) string {
	switch t {
	case schema.EventMarketData:
		return "MarketData"
	case schema.EventOrderIntent:
		return "OrderIntent"
	case schema.EventOrderAck:
		return "OrderAck"
	case schema.EventFill:
		return "Fill"
	case schema.EventRiskDecision:
		return "RiskDecision"
	case schema.EventStrategyDecision:
		return "StrategyDecision"
	default:
		return fmt.Sprintf("Unknown(%d)", t)
	}
}

func printDecoded(t schema.EventType, payload []byte) {
	switch t {
	case schema.EventMarketData:
		md, ok := codec.DecodeMarketData(payload)
		if !ok {
			fmt.Println("  decode MarketData failed")
			return
		}
		fmt.Printf("  md symbol=%d kind=%d price=%d size=%d bid=%d/%d ask=%d/%d\n",
			md.SymbolID, md.Kind, md.Price, md.Size, md.BidPrice, md.BidSize, md.AskPrice, md.AskSize)
	case schema.EventOrderIntent:
		order, ok := codec.DecodeOrderIntent(payload)
		if !ok {
			fmt.Println("  decode OrderIntent failed")
			return
		}
		fmt.Printf("  order id=%d symbol=%d side=%d type=%d tif=%d price=%d qty=%d\n",
			order.OrderID, order.SymbolID, order.Side, order.Type, order.TimeInForce, order.Price, order.Qty)
	case schema.EventOrderAck:
		ack, ok := codec.DecodeOrderAck(payload)
		if !ok {
			fmt.Println("  decode OrderAck failed")
			return
		}
		fmt.Printf("  ack id=%d symbol=%d status=%d reason=%d price=%d qty=%d leaves=%d\n",
			ack.OrderID, ack.SymbolID, ack.Status, ack.Reason, ack.Price, ack.Qty, ack.LeavesQty)
	case schema.EventRiskDecision:
		decision, ok := codec.DecodeRiskDecision(payload)
		if !ok {
			fmt.Println("  decode RiskDecision failed")
			return
		}
		fmt.Printf("  risk id=%d action=%d reason=%d price=%d qty=%d pos=%d max_pos=%d max_notional=%d\n",
			decision.OrderID, decision.Action, decision.Reason, decision.ProposedPrice, decision.ProposedQty,
			decision.CurrentPos, decision.MaxPos, decision.MaxNotional)
	case schema.EventFill:
		fill, ok := codec.DecodeFill(payload)
		if !ok {
			fmt.Println("  decode Fill failed")
			return
		}
		fmt.Printf("  fill id=%d symbol=%d side=%d price=%d qty=%d fee=%d\n",
			fill.OrderID, fill.SymbolID, fill.Side, fill.Price, fill.Qty, fill.Fee)
	default:
		return
	}
}

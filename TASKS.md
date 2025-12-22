# TASKS

## Architecture Summary

- Event schema + payloads: `internal/schema/*.go`
- Binary codecs: `internal/codec/*.go`
- In-memory bus queue: `internal/bus/queue.go`
- WAL writer/reader/playback: `internal/recorder/*.go`
- Trader app (record/replay/recover): `cmd/trader/main.go`
- Market data generator + normalizer: `cmd/mdg/main.go`, `internal/mdg/*.go`
- Order gateway stub + order state machine: `internal/og/*.go`
- Risk engine: `internal/risk/engine.go`
- Position reducer + snapshot/recover: `internal/state/*.go`
- Observability (metrics + trace IDs): `internal/obs/*.go`
- Tools: replay/chaos/paper: `cmd/tools/*`

## Progress (Done)

- [x] Define event schema and payload types for MarketData/OrderIntent/OrderAck/Fill/RiskDecision/StrategyDecision.
- [x] Implement fixed-size codecs for all payload types.
- [x] Implement WAL writer/reader/playback with CRC32C and segment rotation.
- [x] Add recorder integration with non-blocking bus queue.
- [x] Implement trader record mode with dummy strategy event and order flow, plus config reload.
- [x] Implement trader replay mode with order state machine and optional snapshot verification.
- [x] Add position reducer, snapshot IO, and recovery from snapshot + WAL tail.
- [x] Implement risk engine (kill switch, max qty/notional/position, rate limit, price band).
- [x] Add MDG generator + normalizer to write MarketData events.
- [x] Add order gateway stub with reconnect/resend and order state machine.
- [x] Add observability scaffolding (metrics counters + trace IDs per event).
- [x] Add tools: WAL replay, chaos WAL mutator, paper trading replay.

## Planned / Next Tasks

- [ ] Export metrics to Prometheus/OTel and add queue depth + WAL lag gauges.
- [ ] Add market data ingress adapter(s) with parser/validator and data quality metrics (gap/replay/latency).
- [ ] Add strategy stub + event loop that consumes MarketData and produces OrderIntents.
- [ ] Extend risk config with per-strategy/symbol/venue kill switch and record config changes to WAL.
- [ ] Track open orders/exposure, fees, and PnL; add periodic snapshots/scheduling.
- [ ] Improve paper trading to simulate partial fills/slippage and a simple matching engine.
- [ ] Extend chaos harness with disconnect/reconnect and rate limit simulation.
- [ ] Add runbook/scripts for local workflows and deployment checklist.
- [ ] Update documentation/config examples with new risk fields and tools.

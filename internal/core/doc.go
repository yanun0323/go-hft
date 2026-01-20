/*
Core implements the main strategy executor.

# Module
  - in-memory bus: receives market data & order data then push them to strategy runtime
  - strategy runtime: single thread strategy invoker
  - position reducer: stores assets status in memory
  - risk engine: valid the order intent which created by strategy runtime with assets from position reducer

# Source
 1. market data & order data from ingest and order
 2. mock market data from paper trading service
 3. WAL replay from replay service

# Produce
  - order intents to order module

# Sharded
  - apiKey + tradingPair + strategy
*/
package core

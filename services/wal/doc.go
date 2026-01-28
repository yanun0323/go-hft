/*
WAL records trading log in Write Append Log way.

# Module
  - writer: ensure idempotence write in
  - reader

# Source
  - market data from ingest
  - order result from order
  - order decision from core
  - order intent from core
  - strategy risk from core
  - asset risk from risk

# Produce
  - none

# Sharded
  - none
*/
package wal

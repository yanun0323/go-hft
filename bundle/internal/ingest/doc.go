/*
Ingest handles data from market.

# Module
  - market data:
  - normalizer:

# Source
  - market data from websocket

# Produce
  - normalized bytes structure of market data

# Sharded
  - platform

It normalizes data from external APIs then sends them to both core module and WAL.
*/
package ingest

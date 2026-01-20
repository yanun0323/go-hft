-include Makefile.env
export

GOBIN := $(shell go env GOBIN)
GOPATH := $(shell go env GOPATH)
BIN_DIR := $(if $(GOBIN),$(GOBIN),$(GOPATH)/bin)
GO ?= go
PLATFORM ?= binance
SOCKET_TOPIC ?= depth
REQ_TOPIC ?= depth
ARG ?= btcusdt@depth
SYMBOL_ID ?= -1
INTERVAL ?=
UDS_DIR ?= /tmp/go-hft
UDS_PATH ?=
API_KEY ?=

.PHONY: $(wildcard *)

## help: show help
help:
	@echo ""
	@echo "Usage:"
	@echo ""
	@sed -n 's/^## //p' Makefile | column -t -s ':' | sed -e 's/^/\t/'
	@echo ""

## codable-gen: run go generate for codable (auto-install if missing)
codable-gen:
	@$(GO) install ./tool/codable
	@PATH="$(BIN_DIR):$$PATH" $(GO) generate ./...

## ingest-cli: run ingest client (cmd_test)
ingest-cli:
	@$(GO) run ./cmd_test \
		-platform "$(PLATFORM)" \
		-socket-topic "$(SOCKET_TOPIC)" \
		-req-topic "$(REQ_TOPIC)" \
		-arg "$(ARG)" \
		-symbol-id "$(SYMBOL_ID)" \
		-interval "$(INTERVAL)" \
		-uds-dir "$(UDS_DIR)" \
		-api-key "$(API_KEY)"

## ingest: run ingest server (cmd/ingest)
ingest:
	@set -e; \
	if [ -n "$(UDS_PATH)" ]; then \
		$(GO) run ./cmd/ingest -platform "$(PLATFORM)" -topic "$(SOCKET_TOPIC)" -uds-path "$(UDS_PATH)"; \
	else \
		$(GO) run ./cmd/ingest -platform "$(PLATFORM)" -topic "$(SOCKET_TOPIC)" -uds-dir "$(UDS_DIR)"; \
	fi

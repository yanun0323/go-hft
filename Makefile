-include Makefile.env
export

GOBIN := $(shell go env GOBIN)
GOPATH := $(shell go env GOPATH)
BIN_DIR := $(if $(GOBIN),$(GOBIN),$(GOPATH)/bin)
GO ?= go

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
	@$(GO) install ./cmd/codable
	@PATH="$(BIN_DIR):$$PATH" $(GO) generate ./...

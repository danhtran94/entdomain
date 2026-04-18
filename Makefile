.DEFAULT_GOAL := help

.PHONY: help build test gen gen-basic gen-custom fuzz-fiql

help:
	@echo "entdomain — Makefile targets"
	@echo ""
	@echo "  build       go build ./..."
	@echo "  test        go test ./... (fast; matches CI)"
	@echo "  gen         regenerate every example (entc.go under examples/*/ent)"
	@echo "  gen-basic   regenerate examples/basic/ent only"
	@echo "  gen-custom  regenerate examples/custom/repo/ent only"
	@echo "  fuzz-fiql   ad-hoc FIQL parser fuzz (not in CI)"

build:
	@go build ./...

test:
	@go test ./...

# `gen` regenerates every example. Required after edits to template/*.tmpl
# or any root-package generator. CI catches divergence; running locally is faster.
gen: gen-basic gen-custom

gen-basic:
	@cd examples/basic/ent && go run entc.go

gen-custom:
	@cd examples/custom/repo/ent && go run entc.go

fuzz-fiql:
	@go test -fuzz=FuzzParseFIQL -fuzztime=30s

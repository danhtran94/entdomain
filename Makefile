.DEFAULT_GOAL := help

.PHONY: all clean help build test gen gen-basic gen-custom gen-sliced gen-sliceddomain fuzz-fiql

define HELP_TEXT
entdomain — Makefile targets

  all         alias for build (checkmake convention)
  build       go build ./...
  clean       go clean -testcache
  test        go test ./... (fast; matches CI)
  gen         regenerate every example (entc.go under examples/*/ent)
  gen-basic   regenerate examples/basic/ent only
  gen-custom  regenerate examples/custom/repo/ent only
  gen-sliced  regenerate examples/sliced/ent only (vertical slices; .pb.go needs `buf generate`)
  gen-sliceddomain  regenerate examples/sliceddomain/ent only (slices + WithProtoHelpersInDomain)
  fuzz-fiql   ad-hoc FIQL parser fuzz (not in CI)
endef
export HELP_TEXT

all: build

clean:
	@go clean -testcache

help:
	@echo "$$HELP_TEXT"

build:
	@go build ./...

test:
	@go test ./...

# `gen` regenerates every example. Required after edits to template/*.tmpl
# or any root-package generator. CI catches divergence; running locally is faster.
gen: gen-basic gen-custom gen-sliced gen-sliceddomain

gen-basic:
	@cd examples/basic/ent && go run entc.go

gen-custom:
	@cd examples/custom/repo/ent && go run entc.go

gen-sliced:
	@cd examples/sliced/ent && go run entc.go

gen-sliceddomain:
	@cd examples/sliceddomain/ent && go run entc.go

fuzz-fiql:
	@go test -fuzz=FuzzParseFIQL -fuzztime=30s

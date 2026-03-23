//go:build tools

// Package entdomain (tools) tracks build-time dependencies that are required
// for testdata generation but not imported by the library itself.
// This keeps them visible to go mod tidy.
//
// To regenerate testdata:
//
//	cd examples/basic/ent && go run entc.go
//	cd examples/basic/proto && buf generate
package entdomain

import (
	// Required by examples/basic/proto/entpb (buf generate output).
	_ "google.golang.org/protobuf/proto"
)

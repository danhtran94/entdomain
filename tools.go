//go:build tools

// Package entdomain (tools) tracks build-time dependencies that are required
// for testdata generation but not imported by the library itself.
// This keeps them visible to go mod tidy.
//
// To regenerate testdata:
//
//	cd internal/testdata/ent && go run entc.go
//	cd internal/testdata/proto && buf generate
package entdomain

import (
	// Required by internal/testdata/proto/entpb (buf generate output).
	_ "google.golang.org/protobuf/proto"
)

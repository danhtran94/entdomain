//go:build ignore

// Custom layout example: demonstrates a non-standard project layout where the
// ent directory is nested inside a subdirectory (repo/ent) rather than directly
// at the module root, and the domain package lives outside (internal/domain).
//
// Key: WithPackagePath is relative to the module root (go.mod), not to the ent dir.
//
// To regenerate:
//
//	cd examples/custom/repo/ent && go run entc.go
package main

import (
	"log"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
	"github.com/danhtran94/entdomain"
)

func main() {
	ex, err := entdomain.NewExtension(
		entdomain.WithPackagePath("examples/custom/internal/domain"),
		entdomain.WithPackageName("domain"),
	)
	if err != nil {
		log.Fatalf("creating entdomain extension: %v", err)
	}
	if err := entc.Generate("../schema",
		&gen.Config{
			Target:   ".",
			Package:  "github.com/danhtran94/entdomain/examples/custom/repo/ent",
			Features: []gen.Feature{gen.FeatureUpsert},
		},
		entc.Extensions(ex),
	); err != nil {
		log.Fatalf("running ent codegen: %v", err)
	}
}

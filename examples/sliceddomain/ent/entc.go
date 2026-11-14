//go:build ignore

package main

import (
	"log"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
	"github.com/danhtran94/entdomain"
)

func main() {
	ex, err := entdomain.NewExtension(
		entdomain.WithPackagePath("examples/sliceddomain/internal/domain"),
		entdomain.WithPackageName("domain"),
		entdomain.WithProto(
			entdomain.WithProtoDir("examples/sliceddomain/proto"),
			entdomain.WithProtoPackageName("entpb"),
			entdomain.WithProtoHelpersInDomain(),
			entdomain.WithProtoGoPackage("github.com/danhtran94/entdomain/examples/sliceddomain/proto/entpb;entpb"),
		),
	)
	if err != nil {
		log.Fatalf("creating entdomain extension: %v", err)
	}
	if err := entc.Generate("./schema",
		&gen.Config{
			Features: []gen.Feature{gen.FeatureUpsert},
		},
		entc.Extensions(ex),
	); err != nil {
		log.Fatalf("running ent codegen: %v", err)
	}
}

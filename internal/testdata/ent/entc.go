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
		entdomain.WithPackagePath("domain"),
		entdomain.WithPackageName("domain"),
		entdomain.WithNoBulk("Post"),
		entdomain.WithProto(
			entdomain.WithProtoDir("proto"),
			entdomain.WithProtoPackageName("entpb"),
			entdomain.WithProtoGoPackage("github.com/danhtran94/entdomain/internal/testdata/proto/entpb;entpb"),
		),
	)
	if err != nil {
		log.Fatalf("creating entdomain extension: %v", err)
	}
	if err := entc.Generate("./schema",
		&gen.Config{},
		entc.Extensions(ex),
	); err != nil {
		log.Fatalf("running ent codegen: %v", err)
	}
}

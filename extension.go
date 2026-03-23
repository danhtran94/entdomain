// Copyright 2019-present Facebook
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package entdomain

import (
	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
)

// ProtoConfig holds configuration for proto file generation.
type ProtoConfig struct {
	dir             string // relative to module root, e.g. "proto"
	pkgName         string // directory suffix AND default proto package name, e.g. "entpb" or "v1"
	fullPkgName     string // optional override for the proto `package` declaration, e.g. "entminimal.v1"
	goPackage       string // go_package option, e.g. "github.com/example/proto/entpb;entpb"
	helpersInDomain bool   // when true, mappers+helpers are generated flat in the domain package; default: domain/pbmap/ subpackage
}

// ProtoOption is a functional option for ProtoConfig.
type ProtoOption func(*ProtoConfig)

// WithProtoDir sets the directory for generated .proto files, relative to the module root.
func WithProtoDir(dir string) ProtoOption {
	return func(c *ProtoConfig) { c.dir = dir }
}

// WithProtoPackageName sets the proto package name.
func WithProtoPackageName(name string) ProtoOption {
	return func(c *ProtoConfig) { c.pkgName = name }
}

// WithProtoGoPackage sets the go_package proto option value.
func WithProtoGoPackage(goPackage string) ProtoOption {
	return func(c *ProtoConfig) { c.goPackage = goPackage }
}

// WithProtoFullPackageName overrides the proto `package` declaration written into the
// generated .proto file. By default the declaration uses pkgName (the directory suffix),
// e.g. "entpb". Use this when the proto package name differs from the directory name,
// e.g. WithProtoPackageName("v1") + WithProtoFullPackageName("entminimal.v1").
func WithProtoFullPackageName(name string) ProtoOption {
	return func(c *ProtoConfig) { c.fullPkgName = name }
}

// WithProtoHelpersInDomain places the generated proto helper functions (toInt64Slice, etc.)
// in the domain package instead of the proto package (the default).
func WithProtoHelpersInDomain() ProtoOption {
	return func(c *ProtoConfig) { c.helpersInDomain = true }
}

// Extension implements entc.Extension for generating a pure Go domain layer.
type Extension struct {
	entc.DefaultExtension
	pkgPath        string          // e.g. "internal/domain"
	pkgName        string          // e.g. "domain"
	noBulkAll      bool            // disables bulk generation for all entities
	noBulkEntities map[string]bool // entity names that opt out of bulk generation
	proto          *ProtoConfig    // nil if proto generation is disabled
}

// ExtensionOption is a functional option for configuring Extension.
type ExtensionOption func(*Extension) error

// WithPackagePath sets the output path for the generated domain package,
// relative to the module root (the parent of the ent directory).
// Example: "internal/domain"
func WithPackagePath(path string) ExtensionOption {
	return func(e *Extension) error {
		e.pkgPath = path
		return nil
	}
}

// WithPackageName sets the Go package name for the generated domain package.
// Defaults to "domain" if not specified.
func WithPackageName(name string) ExtensionOption {
	return func(e *Extension) error {
		e.pkgName = name
		return nil
	}
}

// WithNoBulk disables bulk generation (XxxList type, ToDomain slice method,
// CreateBulkDomain, UpdateBulkDomain, XxxUpdateOneBulk).
// Called with no arguments, it disables bulk for all entities.
// Called with entity names, it disables bulk only for those entities.
func WithNoBulk(entityNames ...string) ExtensionOption {
	return func(e *Extension) error {
		if len(entityNames) == 0 {
			e.noBulkAll = true
			return nil
		}
		if e.noBulkEntities == nil {
			e.noBulkEntities = make(map[string]bool)
		}
		for _, name := range entityNames {
			e.noBulkEntities[name] = true
		}
		return nil
	}
}

// WithProto enables proto file generation with the given options.
// When enabled, the extension generates .proto message files and domain ↔ proto mapper files.
func WithProto(opts ...ProtoOption) ExtensionOption {
	return func(e *Extension) error {
		cfg := &ProtoConfig{}
		for _, opt := range opts {
			opt(cfg)
		}
		e.proto = cfg
		return nil
	}
}

// NewExtension creates a new Extension with the given options.
func NewExtension(opts ...ExtensionOption) (*Extension, error) {
	e := &Extension{
		pkgName: "domain",
		pkgPath: "internal/domain",
	}
	for _, opt := range opts {
		if err := opt(e); err != nil {
			return nil, err
		}
	}
	return e, nil
}

// Hooks returns the gen.Hook implementations for this extension.
func (e *Extension) Hooks() []gen.Hook {
	return []gen.Hook{e.generateDomainHook()}
}

// Templates returns the gen.Template implementations for this extension.
func (e *Extension) Templates() []*gen.Template {
	return []*gen.Template{DomainTemplate, FIQLTemplate}
}

// generateDomainHook returns a gen.Hook that:
// 1. Generates pure domain struct files first (so they exist when ent runs goimports)
// 2. Stores extension config in graph annotations so domain.tmpl can read it
// 3. Runs next generator (writes ent files including domain.tmpl output via goimports)
func (e *Extension) generateDomainHook() gen.Hook {
	return func(next gen.Generator) gen.Generator {
		return gen.GenerateFunc(func(g *gen.Graph) error {
			// Propagate extension-level noBulk config into each entity's annotation
			// so that both the domain file generator and the ent template can read it.
			for _, n := range g.Nodes {
				if !e.noBulkAll && !e.noBulkEntities[n.Name] {
					continue
				}
				ant, err := extractEntityAnnotation(n.Annotations)
				if err != nil || ant == nil {
					continue
				}
				ant.NoBulk = true
				n.Annotations[ant.Name()] = ant
			}
			// Generate domain struct files first, so goimports can resolve the domain
			// package when it processes the ent/domain.go mapper file.
			if err := generateDomainFiles(g, e.pkgPath, e.pkgName); err != nil {
				return err
			}
			// Generate proto files and domain ↔ proto mapper files if enabled.
			if e.proto != nil {
				outDir := moduleRoot(g)
				if err := generateProtoFiles(g, e.proto, outDir); err != nil {
					return err
				}
				if err := generateProtoMapperFiles(g, e.proto, outDir, e.pkgPath, e.pkgName); err != nil {
					return err
				}
			}
			if g.Annotations == nil {
				g.Annotations = make(gen.Annotations)
			}
			g.Annotations["EntDomainConfig"] = map[string]string{
				"PkgPath": e.pkgPath,
				"PkgName": e.pkgName,
			}
			return next.Generate(g)
		})
	}
}

var _ entc.Extension = (*Extension)(nil)

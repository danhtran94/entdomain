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

// Extension implements entc.Extension for generating a pure Go domain layer.
type Extension struct {
	entc.DefaultExtension
	pkgPath string // e.g. "internal/domain"
	pkgName string // e.g. "domain"
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
	return []*gen.Template{DomainTemplate}
}

// generateDomainHook returns a gen.Hook that:
// 1. Generates pure domain struct files first (so they exist when ent runs goimports)
// 2. Stores extension config in graph annotations so domain.tmpl can read it
// 3. Runs next generator (writes ent files including domain.tmpl output via goimports)
func (e *Extension) generateDomainHook() gen.Hook {
	return func(next gen.Generator) gen.Generator {
		return gen.GenerateFunc(func(g *gen.Graph) error {
			// Generate domain struct files first, so goimports can resolve the domain
			// package when it processes the ent/domain.go mapper file.
			if err := generateDomainFiles(g, e.pkgPath, e.pkgName); err != nil {
				return err
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

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

// Code generation templates live exclusively in the template/ directory and
// are loaded via //go:embed. This applies to every template in the project,
// regardless of which pipeline consumes it:
//
//   - ent's gen.Template plugin (domain.tmpl, fiql.tmpl) — loaded here via
//     parseT into *gen.Template values registered by extension.go.
//   - entdomain's own generators (domain_struct.tmpl, proto_messages.tmpl,
//     proto_mapper_subpkg.tmpl, proto_mapper_domain.tmpl, proto_helpers.tmpl)
//     — loaded with //go:embed string in the driver Go file (generate.go,
//     proto_generate.go, proto_mapper_generate.go) and parsed via stdlib
//     text/template.
//
// When adding a new template:
//
//  1. Create template/<name>.tmpl with the template body.
//  2. Embed it in the driver Go file with //go:embed template/<name>.tmpl
//     into a string variable, then parse via text/template (or, for ent
//     plugin templates, via parseT here).
//  3. Never inline template bodies as Go string literals — that pattern is
//     deprecated; the template/ directory is the single source of truth.
//
// Custom template.FuncMap definitions stay in the driver Go file because they
// reference Go functions; only the template body itself moves to disk.

package entdomain

import (
	"embed"
	"strings"
	"text/template"

	"entgo.io/ent/entc/gen"
	"entgo.io/ent/schema/field"
)

var (
	// DomainTemplate generates mapper methods (ToDomain, ApplyDomain) in the ent/ dir.
	DomainTemplate = parseT("template/domain.tmpl")

	// FIQLTemplate generates FIQL field registries and entry-point functions in the ent/ dir.
	FIQLTemplate = parseT("template/fiql.tmpl")

	// TemplateFuncs contains the extra template functions used by entdomain.
	TemplateFuncs = template.FuncMap{
		"domainNodes":      domainNodesFn,
		"entityAnnotation": entityAnnotationFn,
		"edgeAnnotation":   edgeAnnotationFn,
		"domainImportPath": domainImportPathFn,
		"domainPkgName":    domainPkgNameFn,
		"virtualFieldType": virtualFieldTypeFn,
		"isEnum":           func(f *gen.Field) bool { return f.Type.Type == field.TypeEnum },
		"isNillable":       func(f *gen.Field) bool { return f.Optional || f.Nillable },
		"isJSONField":      func(f *gen.Field) bool { return f.Type.Type == field.TypeJSON || f.Type.Type == field.TypeBytes },
		"hasEnumFields":    hasEnumFieldsFn,
		"hasUpsert":           hasUpsertFn,
		"hasFIQLNodes":        hasFIQLNodesFn,
		"hasFIQLFields":       hasFIQLFieldsFn,
		"fieldFIQLAnnotation": fieldFIQLAnnotationFn,
		"fieldFIQLKind":       fieldFIQLKindFn,
		"lower":               strings.ToLower,
		"singular":            func(s string) string { return gen.Funcs["singular"].(func(string) string)(s) },
		"pascal":              func(s string) string { return gen.Funcs["pascal"].(func(string) string)(s) },
		"snake":               func(s string) string { return gen.Funcs["snake"].(func(string) string)(s) },
	}

	//go:embed template/*
	_templates embed.FS
)

func parseT(path string) *gen.Template {
	return gen.MustParse(gen.NewTemplate(path).
		Funcs(TemplateFuncs).
		ParseFS(_templates, path))
}

// domainNodesFn filters gen.Type nodes to those with EntityAnnotation.
func domainNodesFn(nodes []*gen.Type) ([]*gen.Type, error) {
	var out []*gen.Type
	for _, n := range nodes {
		ant, err := extractEntityAnnotation(n.Annotations)
		if err != nil {
			return nil, err
		}
		if ant != nil {
			out = append(out, n)
		}
	}
	return out, nil
}

// entityAnnotationFn returns the EntityAnnotation for a node, or nil if not present.
func entityAnnotationFn(n *gen.Type) (*EntityAnnotation, error) {
	return extractEntityAnnotation(n.Annotations)
}

// edgeAnnotationFn returns the EdgeAnnotation for an edge, or nil if not present.
func edgeAnnotationFn(e *gen.Edge) (*EdgeAnnotation, error) {
	return extractEdgeAnnotation(e.Annotations)
}

// domainImportPathFn computes the full Go import path for the domain package.
func domainImportPathFn(g *gen.Graph, pkgPath string) (string, error) {
	return domainImportPath(g, pkgPath)
}

// domainPkgNameFn extracts the pkgName from the EntDomainConfig annotation map.
func domainPkgNameFn(cfg map[string]string) string {
	return cfg["PkgName"]
}

// virtualFieldTypeFn resolves a FieldType to its Go type string.
func virtualFieldTypeFn(ft FieldType) string {
	typeStr, _ := resolveGoTypeDomainStr(ft)
	return typeStr
}

// hasEnumFieldsFn reports whether a node has any enum fields.
func hasEnumFieldsFn(n *gen.Type) bool {
	for _, f := range n.Fields {
		if f.Type.Type == field.TypeEnum && !f.IsEdgeField() {
			return true
		}
	}
	return false
}

// hasUpsertFn reports whether the graph has gen.FeatureUpsert enabled.
func hasUpsertFn(g *gen.Graph) bool {
	for _, f := range g.Features {
		if f.Name == gen.FeatureUpsert.Name {
			return true
		}
	}
	return false
}

// hasFIQLFieldsFn reports whether a single entity node has at least one FIQL-annotated field.
func hasFIQLFieldsFn(n *gen.Type) (bool, error) {
	ant, err := extractEntityAnnotation(n.Annotations)
	if err != nil {
		return false, err
	}
	if ant == nil {
		return false, nil
	}
	for _, f := range n.Fields {
		if f.IsEdgeField() {
			continue
		}
		fa, err := extractFieldAnnotation(f.Annotations)
		if err != nil {
			return false, err
		}
		if fa != nil && len(fa.FIQLOps) > 0 {
			return true, nil
		}
	}
	return false, nil
}

// hasFIQLNodesFn reports whether any entity node in the graph has at least one FIQL-annotated field.
func hasFIQLNodesFn(nodes []*gen.Type) (bool, error) {
	for _, n := range nodes {
		ant, err := extractEntityAnnotation(n.Annotations)
		if err != nil {
			return false, err
		}
		if ant == nil {
			continue
		}
		for _, f := range n.Fields {
			if f.IsEdgeField() {
				continue
			}
			fa, err := extractFieldAnnotation(f.Annotations)
			if err != nil {
				return false, err
			}
			if fa != nil && len(fa.FIQLOps) > 0 {
				return true, nil
			}
		}
	}
	return false, nil
}

// fieldFIQLAnnotationFn returns the FIQL ops for a field as strings (e.g. "==", "=gt="),
// or nil if the field is not FIQL-annotated. Returning strings lets templates use eq directly.
func fieldFIQLAnnotationFn(f *gen.Field) ([]string, error) {
	fa, err := extractFieldAnnotation(f.Annotations)
	if err != nil {
		return nil, err
	}
	if fa == nil || len(fa.FIQLOps) == 0 {
		return nil, nil
	}
	ops := make([]string, len(fa.FIQLOps))
	for i, op := range fa.FIQLOps {
		ops[i] = string(op)
	}
	return ops, nil
}

// fieldFIQLKindFn returns the FIQL type kind string for a field:
// "String", "Int", "Float", "Bool", "Time", "Enum", or "" (unsupported).
func fieldFIQLKindFn(f *gen.Field) string {
	switch f.Type.Type {
	case field.TypeString:
		return "String"
	case field.TypeInt, field.TypeInt8, field.TypeInt16, field.TypeInt32, field.TypeInt64,
		field.TypeUint, field.TypeUint8, field.TypeUint16, field.TypeUint32, field.TypeUint64:
		return "Int"
	case field.TypeFloat32, field.TypeFloat64:
		return "Float"
	case field.TypeBool:
		return "Bool"
	case field.TypeTime:
		return "Time"
	case field.TypeEnum:
		return "Enum"
	case field.TypeUUID:
		return "UUID"
	default:
		return "" // JSON and others: not supported
	}
}

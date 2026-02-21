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
	"embed"
	"text/template"

	"entgo.io/ent/entc/gen"
	"entgo.io/ent/schema/field"
)

var (
	// DomainTemplate generates mapper methods (ToDomain, ApplyDomain) in the ent/ dir.
	DomainTemplate = parseT("template/domain.tmpl")

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
		"hasEnumFields":    hasEnumFieldsFn,
		"singular":         func(s string) string { return gen.Funcs["singular"].(func(string) string)(s) },
		"pascal":           func(s string) string { return gen.Funcs["pascal"].(func(string) string)(s) },
		"snake":            func(s string) string { return gen.Funcs["snake"].(func(string) string)(s) },
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
func domainImportPathFn(g *gen.Graph, pkgPath string) string {
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

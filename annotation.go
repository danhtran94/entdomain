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
	"encoding/json"

	"entgo.io/ent/schema"
)

// EdgeMode is a bitmask of edge inclusion modes.
type EdgeMode int

const (
	// EdgeModeIDs includes the edge IDs in the domain struct.
	EdgeModeIDs EdgeMode = 1 << iota
	// EdgeModeNest includes the full nested structs in the domain struct.
	EdgeModeNest
)

// FieldType describes the Go type of a virtual field.
type FieldType struct {
	// PkgPath is the import path; empty string means local/stdlib (no import added).
	PkgPath string
	// TypeName is the Go type name as-is; prefix with "*" for pointer types.
	TypeName string
}

var (
	// String is a FieldType for string.
	String = FieldType{TypeName: "string"}
	// Bool is a FieldType for bool.
	Bool = FieldType{TypeName: "bool"}
	// Int is a FieldType for int.
	Int = FieldType{TypeName: "int"}
	// Float64 is a FieldType for float64.
	Float64 = FieldType{TypeName: "float64"}
)

// GoType constructs a FieldType for an arbitrary Go type.
// pkgPath is the full import path (empty for local/stdlib).
// typeName is the Go type name (prefix with "*" for pointer).
func GoType(pkgPath, typeName string) FieldType {
	return FieldType{PkgPath: pkgPath, TypeName: typeName}
}

// ProtoTypeConfig specifies an explicit proto type for a field or virtual field.
type ProtoTypeConfig struct {
	TypeName      string // e.g. "google.protobuf.Timestamp"
	ImportPath    string // e.g. "google/protobuf/timestamp.proto"
	ToProtoExpr   string // optional Go fmt string, e.g. "UserMetadataToProto(%s)"
	FromProtoExpr string // optional Go fmt string, e.g. "UserMetadataFromProto(%s)"
}

// VirtualFieldConfig represents a single virtual field entry on EntityAnnotation.
type VirtualFieldConfig struct {
	Name      string
	FieldType FieldType
	ProtoType *ProtoTypeConfig
}

// virtualFieldOption configures a VirtualFieldConfig.
type virtualFieldOption interface {
	applyVirtualField(*VirtualFieldConfig)
}

// fieldOption configures a FieldAnnotation.
type fieldOption interface {
	applyField(*FieldAnnotation)
}

// ProtoTypeOpt is the value returned by ProtoType(). It implements both
// virtualFieldOption (for use with VirtualField()) and fieldOption (for use
// with Field()), so the same call works in both contexts.
type ProtoTypeOpt struct {
	TypeName      string
	ImportPath    string
	ToProtoExpr   string
	FromProtoExpr string
}

func (p ProtoTypeOpt) applyVirtualField(vf *VirtualFieldConfig) {
	vf.ProtoType = &ProtoTypeConfig{
		TypeName:      p.TypeName,
		ImportPath:    p.ImportPath,
		ToProtoExpr:   p.ToProtoExpr,
		FromProtoExpr: p.FromProtoExpr,
	}
}

func (p ProtoTypeOpt) applyField(a *FieldAnnotation) {
	a.ProtoType = &ProtoTypeConfig{
		TypeName:      p.TypeName,
		ImportPath:    p.ImportPath,
		ToProtoExpr:   p.ToProtoExpr,
		FromProtoExpr: p.FromProtoExpr,
	}
}

// ProtoType sets an explicit proto type. Usable with both VirtualField() and Field().
// importPath is optional; if omitted, no import is added.
func ProtoType(typeName string, importPath ...string) ProtoTypeOpt {
	opt := ProtoTypeOpt{TypeName: typeName}
	if len(importPath) > 0 {
		opt.ImportPath = importPath[0]
	}
	return opt
}

// WithConversion returns a copy of ProtoTypeOpt with explicit Go conversion
// expressions. toExpr and fromExpr are Go fmt strings where %s is the source
// expression (e.g. "UserMetadataToProto(%s)" / "UserMetadataFromProto(%s)").
func (p ProtoTypeOpt) WithConversion(toExpr, fromExpr string) ProtoTypeOpt {
	p.ToProtoExpr = toExpr
	p.FromProtoExpr = fromExpr
	return p
}

// EntityAnnotation is placed on schema Annotations() to opt an entity into domain generation.
type EntityAnnotation struct {
	VirtualFields []VirtualFieldConfig
	NoBulk        bool
}

// Name implements schema.Annotation.
func (EntityAnnotation) Name() string { return "EntDomain" }

// Merge implements schema.Merger.
func (a EntityAnnotation) Merge(other schema.Annotation) schema.Annotation {
	var ant EntityAnnotation
	switch other := other.(type) {
	case EntityAnnotation:
		ant = other
	case *EntityAnnotation:
		if other != nil {
			ant = *other
		}
	default:
		return a
	}
	a.VirtualFields = append(a.VirtualFields, ant.VirtualFields...)
	return a
}

// Decode unmarshals the annotation.
func (a *EntityAnnotation) Decode(v interface{}) error {
	buf, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return json.Unmarshal(buf, a)
}

// entityOption is a functional option for EntityAnnotation.
type entityOption func(*EntityAnnotation)

// Entity builds an EntityAnnotation with the given options.
func Entity(opts ...entityOption) EntityAnnotation {
	a := EntityAnnotation{}
	for _, opt := range opts {
		opt(&a)
	}
	return a
}

// VirtualField returns an entityOption that adds a virtual field.
// Optional virtualFieldOptions (e.g., ProtoType) can be passed.
func VirtualField(name string, ft FieldType, opts ...virtualFieldOption) entityOption {
	return func(a *EntityAnnotation) {
		vf := VirtualFieldConfig{
			Name:      name,
			FieldType: ft,
		}
		for _, opt := range opts {
			opt.applyVirtualField(&vf)
		}
		a.VirtualFields = append(a.VirtualFields, vf)
	}
}

// EdgeAnnotation is placed on individual edge Annotations() to configure domain inclusion.
type EdgeAnnotation struct {
	Mode EdgeMode
}

// Name implements schema.Annotation.
func (EdgeAnnotation) Name() string { return "EntDomainEdge" }

// Merge implements schema.Merger.
func (a EdgeAnnotation) Merge(other schema.Annotation) schema.Annotation {
	var ant EdgeAnnotation
	switch other := other.(type) {
	case EdgeAnnotation:
		ant = other
	case *EdgeAnnotation:
		if other != nil {
			ant = *other
		}
	default:
		return a
	}
	a.Mode |= ant.Mode
	return a
}

// Decode unmarshals the annotation.
func (a *EdgeAnnotation) Decode(v interface{}) error {
	buf, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return json.Unmarshal(buf, a)
}

// HasIDs reports whether the IDs mode is set.
func (a EdgeAnnotation) HasIDs() bool { return a.Mode&EdgeModeIDs != 0 }

// HasNest reports whether the Nest mode is set.
func (a EdgeAnnotation) HasNest() bool { return a.Mode&EdgeModeNest != 0 }

// EdgeOption is a functional option for EdgeAnnotation.
type EdgeOption func(*EdgeAnnotation)

// Edge builds an EdgeAnnotation with the given options.
func Edge(opts ...EdgeOption) EdgeAnnotation {
	a := EdgeAnnotation{}
	for _, opt := range opts {
		opt(&a)
	}
	return a
}

// IDs returns an EdgeOption that includes edge IDs in the domain struct.
func IDs() EdgeOption {
	return func(a *EdgeAnnotation) {
		a.Mode |= EdgeModeIDs
	}
}

// Nest returns an EdgeOption that includes nested structs in the domain struct.
func Nest() EdgeOption {
	return func(a *EdgeAnnotation) {
		a.Mode |= EdgeModeNest
	}
}

// FieldAnnotation is placed on individual field Annotations() to configure domain field behaviour.
type FieldAnnotation struct {
	SkipProto bool
	ProtoType *ProtoTypeConfig
}

// Name implements schema.Annotation.
func (FieldAnnotation) Name() string { return "EntDomainField" }

// Merge implements schema.Merger.
func (a FieldAnnotation) Merge(other schema.Annotation) schema.Annotation {
	var ant FieldAnnotation
	switch other := other.(type) {
	case FieldAnnotation:
		ant = other
	case *FieldAnnotation:
		if other != nil {
			ant = *other
		}
	default:
		return a
	}
	a.SkipProto = a.SkipProto || ant.SkipProto
	if a.ProtoType == nil && ant.ProtoType != nil {
		a.ProtoType = ant.ProtoType
	}
	return a
}

// Decode unmarshals the annotation.
func (a *FieldAnnotation) Decode(v interface{}) error {
	buf, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return json.Unmarshal(buf, a)
}

// skipProtoOpt implements fieldOption.
type skipProtoOpt struct{}

func (skipProtoOpt) applyField(a *FieldAnnotation) { a.SkipProto = true }

// Field builds a FieldAnnotation with the given options.
func Field(opts ...fieldOption) FieldAnnotation {
	a := FieldAnnotation{}
	for _, opt := range opts {
		opt.applyField(&a)
	}
	return a
}

// SkipProto returns a fieldOption that excludes the field from proto generation.
func SkipProto() fieldOption { return skipProtoOpt{} }

// extractEntityAnnotation extracts EntityAnnotation from a map of annotations.
func extractEntityAnnotation(annotations map[string]interface{}) (*EntityAnnotation, error) {
	a := &EntityAnnotation{}
	v, ok := annotations[a.Name()]
	if !ok || v == nil {
		return nil, nil
	}
	if err := a.Decode(v); err != nil {
		return nil, err
	}
	return a, nil
}

// extractFieldAnnotation extracts FieldAnnotation from a map of annotations.
func extractFieldAnnotation(annotations map[string]interface{}) (*FieldAnnotation, error) {
	a := &FieldAnnotation{}
	v, ok := annotations[a.Name()]
	if !ok || v == nil {
		return nil, nil
	}
	if err := a.Decode(v); err != nil {
		return nil, err
	}
	return a, nil
}

// extractEdgeAnnotation extracts EdgeAnnotation from a map of annotations.
func extractEdgeAnnotation(annotations map[string]interface{}) (*EdgeAnnotation, error) {
	a := &EdgeAnnotation{}
	v, ok := annotations[a.Name()]
	if !ok || v == nil {
		return nil, nil
	}
	if err := a.Decode(v); err != nil {
		return nil, err
	}
	return a, nil
}

var (
	_ schema.Annotation = (*EntityAnnotation)(nil)
	_ schema.Merger     = (*EntityAnnotation)(nil)
	_ schema.Annotation = (*EdgeAnnotation)(nil)
	_ schema.Merger     = (*EdgeAnnotation)(nil)
	_ schema.Annotation = (*FieldAnnotation)(nil)
	_ schema.Merger     = (*FieldAnnotation)(nil)
)

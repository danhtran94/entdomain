// Copyright 2026-present Danh Tran Thanh
// SPDX-License-Identifier: Apache-2.0
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
	"testing"

	"entgo.io/ent/entc/gen"
	"entgo.io/ent/schema/field"
	"github.com/stretchr/testify/assert"
)

func TestResolveVirtualFieldProtoSpec_Primitives(t *testing.T) {
	tests := []struct {
		name       string
		ft         FieldType
		wantType   string
		wantExcl   bool
		wantOpt    bool
	}{
		{"string", String, "string", false, false},
		{"bool", Bool, "bool", false, false},
		{"int", Int, "int64", false, false},
		{"float64", Float64, "double", false, false},
		{"*string", GoType("*string"), "string", false, true},
		{"*int", GoType("*int"), "int64", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vf := VirtualFieldConfig{Name: "f", FieldType: tt.ft}
			spec := resolveVirtualFieldProtoSpec(vf)
			assert.Equal(t, tt.wantExcl, spec.IsExcluded, "IsExcluded")
			if !tt.wantExcl {
				assert.Equal(t, tt.wantType, spec.ProtoType, "ProtoType")
				assert.Equal(t, tt.wantOpt, spec.IsOptional, "IsOptional")
			}
		})
	}
}

func TestResolveVirtualFieldProtoSpec_WellKnownGoTypes(t *testing.T) {
	tests := []struct {
		name       string
		pkgPath    string
		typeName   string
		wantType   string
		wantImport string
		wantExcl   bool
	}{
		{
			name:       "time.Time → Timestamp",
			pkgPath:    "time",
			typeName:   "Time",
			wantType:   "google.protobuf.Timestamp",
			wantImport: "google/protobuf/timestamp.proto",
		},
		{
			name:       "time.Duration → Duration",
			pkgPath:    "time",
			typeName:   "Duration",
			wantType:   "google.protobuf.Duration",
			wantImport: "google/protobuf/duration.proto",
		},
		{
			name:     "uuid.UUID → string",
			pkgPath:  "github.com/google/uuid",
			typeName: "UUID",
			wantType: "string",
		},
		{
			name:     "unknown GoType → excluded",
			pkgPath:  "github.com/shopspring/decimal",
			typeName: "Decimal",
			wantExcl: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vf := VirtualFieldConfig{
				Name:      "f",
				FieldType: GoType(tt.typeName, tt.pkgPath),
			}
			spec := resolveVirtualFieldProtoSpec(vf)
			assert.Equal(t, tt.wantExcl, spec.IsExcluded, "IsExcluded")
			if !tt.wantExcl {
				assert.Equal(t, tt.wantType, spec.ProtoType, "ProtoType")
				assert.Equal(t, tt.wantImport, spec.ImportPath, "ImportPath")
			}
		})
	}
}

func TestResolveVirtualFieldProtoSpec_ExplicitProtoType(t *testing.T) {
	vf := VirtualFieldConfig{
		Name:      "expires_at",
		FieldType: GoType("Time", "time"),
		ProtoType: &ProtoTypeConfig{
			TypeName:   "google.protobuf.Timestamp",
			ImportPath: "google/protobuf/timestamp.proto",
		},
	}
	spec := resolveVirtualFieldProtoSpec(vf)
	assert.False(t, spec.IsExcluded)
	assert.Equal(t, "google.protobuf.Timestamp", spec.ProtoType)
	assert.Equal(t, "google/protobuf/timestamp.proto", spec.ImportPath)
}

func TestResolveVirtualFieldProtoSpec_MapTypeExcluded(t *testing.T) {
	// map[string]any has empty PkgPath and non-primitive TypeName → excluded.
	vf := VirtualFieldConfig{
		Name:      "metadata",
		FieldType: GoType("map[string]any"),
	}
	spec := resolveVirtualFieldProtoSpec(vf)
	assert.True(t, spec.IsExcluded, "map type should be excluded from proto")
}

func TestResolveVirtualFieldProtoSpec_ProtoTypeOverridePriority(t *testing.T) {
	// Even if the GoType is well-known, an explicit ProtoType override takes priority.
	vf := VirtualFieldConfig{
		Name:      "ts",
		FieldType: GoType("Time", "time"),
		ProtoType: &ProtoTypeConfig{
			TypeName:   "CustomTimestamp",
			ImportPath: "custom/timestamp.proto",
		},
	}
	spec := resolveVirtualFieldProtoSpec(vf)
	assert.Equal(t, "CustomTimestamp", spec.ProtoType)
	assert.Equal(t, "custom/timestamp.proto", spec.ImportPath)
}

func TestResolveVirtualFieldProtoSpec_ConversionExprs(t *testing.T) {
	t.Run("time.Time non-optional", func(t *testing.T) {
		vf := VirtualFieldConfig{Name: "created_at", FieldType: GoType("Time", "time")}
		spec := resolveVirtualFieldProtoSpec(vf)
		assert.Equal(t, "timestamppb.New(%s)", spec.ToProtoExpr)
		assert.Equal(t, "%s.AsTime()", spec.FromProtoExpr)
	})
	t.Run("int non-optional", func(t *testing.T) {
		vf := VirtualFieldConfig{Name: "count", FieldType: Int}
		spec := resolveVirtualFieldProtoSpec(vf)
		assert.Equal(t, "int64(%s)", spec.ToProtoExpr)
		assert.Equal(t, "int(%s)", spec.FromProtoExpr)
	})
	t.Run("string direct copy", func(t *testing.T) {
		vf := VirtualFieldConfig{Name: "name", FieldType: String}
		spec := resolveVirtualFieldProtoSpec(vf)
		assert.Equal(t, "", spec.ToProtoExpr, "string should be direct copy (empty expr)")
		assert.Equal(t, "", spec.FromProtoExpr)
	})
}

func TestResolveVirtualFieldProtoSpec_DurationConversionExprs(t *testing.T) {
	t.Run("time.Duration non-optional", func(t *testing.T) {
		vf := VirtualFieldConfig{Name: "ttl", FieldType: GoType("Duration", "time")}
		spec := resolveVirtualFieldProtoSpec(vf)
		assert.False(t, spec.IsExcluded)
		assert.Equal(t, "google.protobuf.Duration", spec.ProtoType)
		assert.Equal(t, "google/protobuf/duration.proto", spec.ImportPath)
		assert.Equal(t, "durationpb.New(%s)", spec.ToProtoExpr)
		assert.Equal(t, "%s.AsDuration()", spec.FromProtoExpr)
		assert.False(t, spec.IsOptional)
	})
	t.Run("*time.Duration optional", func(t *testing.T) {
		vf := VirtualFieldConfig{Name: "ttl", FieldType: GoType("*Duration", "time")}
		spec := resolveVirtualFieldProtoSpec(vf)
		assert.False(t, spec.IsExcluded)
		assert.Equal(t, "google.protobuf.Duration", spec.ProtoType)
		assert.Equal(t, "durationToDurationProto(%s)", spec.ToProtoExpr)
		assert.Equal(t, "durationProtoToDuration(%s)", spec.FromProtoExpr)
		assert.True(t, spec.IsOptional)
	})
}

func TestResolveVirtualFieldProtoSpec_OptionalTimeConversionExprs(t *testing.T) {
	// *time.Time should use the nullable helper functions.
	vf := VirtualFieldConfig{Name: "deleted_at", FieldType: GoType("*Time", "time")}
	spec := resolveVirtualFieldProtoSpec(vf)
	assert.False(t, spec.IsExcluded)
	assert.Equal(t, "google.protobuf.Timestamp", spec.ProtoType)
	assert.True(t, spec.IsOptional)
	assert.Equal(t, "timeToTimestampProto(%s)", spec.ToProtoExpr)
	assert.Equal(t, "timestampProtoToTime(%s)", spec.FromProtoExpr)
}

func TestTrackExprImports(t *testing.T) {
	t.Run("timestamppb expr", func(t *testing.T) {
		imports := map[string]bool{}
		trackExprImports("timestamppb.New(d.CreatedAt)", "p.CreatedAt.AsTime()", imports)
		assert.True(t, imports["google.golang.org/protobuf/types/known/timestamppb"])
		assert.False(t, imports["google.golang.org/protobuf/types/known/durationpb"])
		assert.False(t, imports["github.com/google/uuid"])
	})
	t.Run("durationpb expr", func(t *testing.T) {
		imports := map[string]bool{}
		trackExprImports("durationpb.New(d.TTL)", "p.TTL.AsDuration()", imports)
		assert.True(t, imports["google.golang.org/protobuf/types/known/durationpb"])
		assert.False(t, imports["google.golang.org/protobuf/types/known/timestamppb"])
	})
	t.Run("uuid.MustParse in FromProto", func(t *testing.T) {
		imports := map[string]bool{}
		trackExprImports("d.ExternalID.String()", "uuid.MustParse(p.ExternalId)", imports)
		assert.True(t, imports["github.com/google/uuid"])
		assert.False(t, imports["google.golang.org/protobuf/types/known/timestamppb"])
	})
	t.Run("no special imports for plain string", func(t *testing.T) {
		imports := map[string]bool{}
		trackExprImports("d.Name", "p.Name", imports)
		assert.Empty(t, imports)
	})
}

func TestApplyExprTemplate(t *testing.T) {
	assert.Equal(t, "d.Name", applyExprTemplate("", "d.Name"))
	assert.Equal(t, "int64(d.Score)", applyExprTemplate("int64(%s)", "d.Score"))
	assert.Equal(t, "timestamppb.New(d.CreatedAt)", applyExprTemplate("timestamppb.New(%s)", "d.CreatedAt"))
	assert.Equal(t, "p.CreatedAt.AsTime()", applyExprTemplate("%s.AsTime()", "p.CreatedAt"))
}

func TestIsMapStringAny(t *testing.T) {
	tests := []struct {
		name  string
		ident string
		want  bool
	}{
		{"map[string]interface {} (reflect form)", "map[string]interface {}", true},
		{"map[string]any (alias form)", "map[string]any", true},
		{"map[string]string", "map[string]string", false},
		{"[]string", "[]string", false},
		{"json.RawMessage", "json.RawMessage", false},
		{"struct", "SomeStruct", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &gen.Field{}
			f.Type = &field.TypeInfo{
				Type:  field.TypeJSON,
				RType: &field.RType{Ident: tt.ident},
			}
			assert.Equal(t, tt.want, isMapStringAny(f))
		})
	}
}

func TestResolveEntFieldProtoSpec_JSONField(t *testing.T) {
	t.Run("map[string]any → google.protobuf.Struct", func(t *testing.T) {
		f := &gen.Field{}
		f.Type = &field.TypeInfo{
			Type:  field.TypeJSON,
			RType: &field.RType{Ident: "map[string]interface {}"},
		}
		spec := resolveEntFieldProtoSpec("User", f, nil)
		assert.False(t, spec.IsExcluded)
		assert.Equal(t, "google.protobuf.Struct", spec.ProtoType)
		assert.Equal(t, "google/protobuf/struct.proto", spec.ImportPath)
		assert.Equal(t, "mapToProtoStruct(%s)", spec.ToProtoExpr)
		assert.Equal(t, "protoStructToMap(%s)", spec.FromProtoExpr)
		assert.False(t, spec.IsOptional, "Struct is a message type; no optional keyword")
	})

	t.Run("typed JSON ([]string) → excluded without annotation", func(t *testing.T) {
		f := &gen.Field{}
		f.Type = &field.TypeInfo{
			Type:  field.TypeJSON,
			RType: &field.RType{Ident: "[]string"},
		}
		spec := resolveEntFieldProtoSpec("User", f, nil)
		assert.True(t, spec.IsExcluded)
		assert.Contains(t, spec.ExcludedReason, "JSON", "ExcludedReason should explain the skip")
	})

	t.Run("custom UUID GoType → excluded with reason naming the gate", func(t *testing.T) {
		f := &gen.Field{}
		f.Type = &field.TypeInfo{
			Type:  field.TypeUUID,
			RType: &field.RType{PkgPath: "example.com/custom", Ident: "custom.MyUUID"},
		}
		spec := resolveEntFieldProtoSpec("User", f, nil)
		assert.True(t, spec.IsExcluded)
		assert.Contains(t, spec.ExcludedReason, "UUID GoType", "reason should mention UUID GoType so it's actionable")
	})

	t.Run("bytes → excluded with non-empty reason", func(t *testing.T) {
		f := &gen.Field{}
		f.Type = &field.TypeInfo{Type: field.TypeBytes}
		spec := resolveEntFieldProtoSpec("User", f, nil)
		assert.True(t, spec.IsExcluded)
		assert.NotEmpty(t, spec.ExcludedReason)
	})

	t.Run("typed JSON with explicit ProtoType annotation → not excluded", func(t *testing.T) {
		f := &gen.Field{}
		f.Type = &field.TypeInfo{
			Type:  field.TypeJSON,
			RType: &field.RType{Ident: "map[string]string"},
		}
		fa := &FieldAnnotation{
			ProtoType: &ProtoTypeConfig{
				TypeName:   "google.protobuf.Struct",
				ImportPath: "google/protobuf/struct.proto",
			},
		}
		spec := resolveEntFieldProtoSpec("User", f, fa)
		assert.False(t, spec.IsExcluded)
		assert.Equal(t, "google.protobuf.Struct", spec.ProtoType)
		assert.Equal(t, "google/protobuf/struct.proto", spec.ImportPath)
		assert.False(t, spec.IsRepeated)
	})

	t.Run("[]string JSON with explicit ProtoType('string') → repeated, not excluded", func(t *testing.T) {
		f := &gen.Field{}
		f.Type = &field.TypeInfo{
			Type:  field.TypeJSON,
			RType: &field.RType{Ident: "[]string"},
		}
		fa := &FieldAnnotation{
			ProtoType: &ProtoTypeConfig{TypeName: "string"},
		}
		spec := resolveEntFieldProtoSpec("User", f, fa)
		assert.False(t, spec.IsExcluded)
		assert.Equal(t, "string", spec.ProtoType)
		assert.True(t, spec.IsRepeated, "slice type should infer IsRepeated")
		assert.False(t, spec.IsOptional, "repeated fields are never optional")
		assert.Empty(t, spec.ToProtoExpr, "[]string → []string needs no conversion")
		assert.Empty(t, spec.FromProtoExpr)
	})

	t.Run("json.RawMessage → excluded", func(t *testing.T) {
		f := &gen.Field{}
		f.Type = &field.TypeInfo{
			Type:  field.TypeJSON,
			RType: &field.RType{Ident: "json.RawMessage", PkgPath: "encoding/json"},
		}
		spec := resolveEntFieldProtoSpec("User", f, nil)
		assert.True(t, spec.IsExcluded)
	})

	t.Run("nil RType → excluded", func(t *testing.T) {
		f := &gen.Field{}
		f.Type = &field.TypeInfo{Type: field.TypeJSON}
		spec := resolveEntFieldProtoSpec("User", f, nil)
		assert.True(t, spec.IsExcluded)
	})
}

func TestFieldAnnotation_ProtoType(t *testing.T) {
	// Verify Field(ProtoType(...)) compiles and populates FieldAnnotation correctly.
	fa := Field(ProtoType("google.protobuf.Struct", "google/protobuf/struct.proto"))
	assert.NotNil(t, fa.ProtoType)
	assert.Equal(t, "google.protobuf.Struct", fa.ProtoType.TypeName)
	assert.Equal(t, "google/protobuf/struct.proto", fa.ProtoType.ImportPath)
	assert.False(t, fa.SkipProto)
}

func TestParseGoPackage(t *testing.T) {
	path, alias := parseGoPackage("github.com/example/proto/entpb;entpb")
	assert.Equal(t, "github.com/example/proto/entpb", path)
	assert.Equal(t, "entpb", alias)

	path2, alias2 := parseGoPackage("github.com/example/proto/entpb")
	assert.Equal(t, "github.com/example/proto/entpb", path2)
	assert.Equal(t, "", alias2)
}

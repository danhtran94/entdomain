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
	"testing"

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
		{"*string", GoType("", "*string"), "string", false, true},
		{"*int", GoType("", "*int"), "int64", false, true},
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
				FieldType: GoType(tt.pkgPath, tt.typeName),
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
		FieldType: GoType("time", "Time"),
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
		FieldType: GoType("", "map[string]any"),
	}
	spec := resolveVirtualFieldProtoSpec(vf)
	assert.True(t, spec.IsExcluded, "map type should be excluded from proto")
}

func TestResolveVirtualFieldProtoSpec_ProtoTypeOverridePriority(t *testing.T) {
	// Even if the GoType is well-known, an explicit ProtoType override takes priority.
	vf := VirtualFieldConfig{
		Name:      "ts",
		FieldType: GoType("time", "Time"),
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
		vf := VirtualFieldConfig{Name: "created_at", FieldType: GoType("time", "Time")}
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

func TestApplyExprTemplate(t *testing.T) {
	assert.Equal(t, "d.Name", applyExprTemplate("", "d.Name"))
	assert.Equal(t, "int64(d.Score)", applyExprTemplate("int64(%s)", "d.Score"))
	assert.Equal(t, "timestamppb.New(d.CreatedAt)", applyExprTemplate("timestamppb.New(%s)", "d.CreatedAt"))
	assert.Equal(t, "p.CreatedAt.AsTime()", applyExprTemplate("%s.AsTime()", "p.CreatedAt"))
}

func TestParseGoPackage(t *testing.T) {
	path, alias := parseGoPackage("github.com/example/proto/entpb;entpb")
	assert.Equal(t, "github.com/example/proto/entpb", path)
	assert.Equal(t, "entpb", alias)

	path2, alias2 := parseGoPackage("github.com/example/proto/entpb")
	assert.Equal(t, "github.com/example/proto/entpb", path2)
	assert.Equal(t, "", alias2)
}

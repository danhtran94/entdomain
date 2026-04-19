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

	"entgo.io/ent/entc/gen"
	"entgo.io/ent/schema/field"
	"github.com/stretchr/testify/assert"
)

func mkField(t field.Type, rt *field.RType) *gen.Field {
	f := &gen.Field{}
	f.Type = &field.TypeInfo{Type: t, RType: rt}
	return f
}

func TestResolveFieldKind_SupportedTypes(t *testing.T) {
	cases := []struct {
		name string
		t    field.Type
		want FieldKind
		tag  string
	}{
		{"string", field.TypeString, KindString, "String"},
		{"int", field.TypeInt, KindInt, "Int"},
		{"int8", field.TypeInt8, KindInt, "Int"},
		{"int32", field.TypeInt32, KindInt, "Int"},
		{"int64", field.TypeInt64, KindInt, "Int"},
		{"uint", field.TypeUint, KindInt, "Int"},
		{"uint64", field.TypeUint64, KindInt, "Int"},
		{"float32", field.TypeFloat32, KindFloat, "Float"},
		{"float64", field.TypeFloat64, KindFloat, "Float"},
		{"bool", field.TypeBool, KindBool, "Bool"},
		{"time", field.TypeTime, KindTime, "Time"},
		{"enum", field.TypeEnum, KindEnum, "Enum"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, reason := resolveFieldKind(mkField(tc.t, nil))
			assert.Equal(t, tc.want, got)
			assert.Empty(t, reason)
			assert.Equal(t, tc.tag, got.String())
		})
	}
}

func TestResolveFieldKind_UUID(t *testing.T) {
	t.Run("standard uuid.UUID is KindUUID", func(t *testing.T) {
		rt := &field.RType{PkgPath: "github.com/google/uuid", Ident: "uuid.UUID"}
		got, reason := resolveFieldKind(mkField(field.TypeUUID, rt))
		assert.Equal(t, KindUUID, got)
		assert.Empty(t, reason)
		assert.Equal(t, "UUID", got.String())
	})

	t.Run("custom GoType is KindUnsupported with reason", func(t *testing.T) {
		rt := &field.RType{PkgPath: "example.com/custom", Ident: "custom.MyUUID"}
		got, reason := resolveFieldKind(mkField(field.TypeUUID, rt))
		assert.Equal(t, KindUnsupported, got)
		assert.Contains(t, reason, "custom UUID GoType")
		assert.Equal(t, "", got.String())
	})

	t.Run("nil RType is KindUnsupported", func(t *testing.T) {
		got, reason := resolveFieldKind(mkField(field.TypeUUID, nil))
		assert.Equal(t, KindUnsupported, got)
		assert.NotEmpty(t, reason)
	})
}

func TestResolveFieldKind_UnsupportedTypes(t *testing.T) {
	cases := []struct {
		name string
		t    field.Type
		hint string
	}{
		{"json", field.TypeJSON, "JSON"},
		{"bytes", field.TypeBytes, "bytes"},
		{"other", field.TypeOther, "TypeOther"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, reason := resolveFieldKind(mkField(tc.t, nil))
			assert.Equal(t, KindUnsupported, got)
			assert.Contains(t, reason, tc.hint)
		})
	}
}

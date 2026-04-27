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
	"github.com/stretchr/testify/require"
)

func TestFieldFIQLKind_UUID(t *testing.T) {
	t.Run("standard uuid.UUID emits UUID kind", func(t *testing.T) {
		f := &gen.Field{}
		f.Type = &field.TypeInfo{
			Type: field.TypeUUID,
			RType: &field.RType{
				PkgPath: "github.com/google/uuid",
				Ident:   "uuid.UUID",
			},
		}
		assert.Equal(t, "UUID", fieldFIQLKindFn(f))
	})

	t.Run("custom GoType is skipped to avoid signature mismatch", func(t *testing.T) {
		f := &gen.Field{}
		f.Type = &field.TypeInfo{
			Type: field.TypeUUID,
			RType: &field.RType{
				PkgPath: "example.com/custom",
				Ident:   "custom.MyUUID",
			},
		}
		assert.Equal(t, "", fieldFIQLKindFn(f))
	})

	t.Run("nil RType is skipped", func(t *testing.T) {
		f := &gen.Field{}
		f.Type = &field.TypeInfo{Type: field.TypeUUID}
		assert.Equal(t, "", fieldFIQLKindFn(f))
	})
}

// fieldWithFIQLOps constructs a gen.Field with a FieldAnnotation containing
// the given FIQL ops. Used to exercise the codegen-time optionality gate.
func fieldWithFIQLOps(name string, optional, nillable bool, ops ...FIQLOp) *gen.Field {
	f := &gen.Field{Name: name, Optional: optional, Nillable: nillable}
	f.Type = &field.TypeInfo{Type: field.TypeString}
	ann := FieldAnnotation{FIQLOps: ops}
	f.Annotations = map[string]interface{}{ann.Name(): ann}
	return f
}

func TestFieldFIQLAnnotation_NullOnNonOptional(t *testing.T) {
	t.Run("IsNull on non-optional non-nillable field is a codegen error", func(t *testing.T) {
		f := fieldWithFIQLOps("name", false, false, EQ, IsNull)
		ops, err := fieldFIQLAnnotationFn(f)
		require.Error(t, err)
		assert.Nil(t, ops)
		assert.Contains(t, err.Error(), "requires Optional() or Nillable()")
		assert.Contains(t, err.Error(), "name")
	})

	t.Run("NotNull on non-optional non-nillable field is a codegen error", func(t *testing.T) {
		f := fieldWithFIQLOps("name", false, false, NotNull)
		_, err := fieldFIQLAnnotationFn(f)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires Optional() or Nillable()")
	})

	t.Run("IsNull on Optional() field is allowed", func(t *testing.T) {
		f := fieldWithFIQLOps("bio", true, false, IsNull, NotNull)
		ops, err := fieldFIQLAnnotationFn(f)
		require.NoError(t, err)
		assert.Equal(t, []string{string(IsNull), string(NotNull)}, ops)
	})

	t.Run("IsNull on Nillable() field is allowed", func(t *testing.T) {
		f := fieldWithFIQLOps("bio", false, true, IsNull)
		_, err := fieldFIQLAnnotationFn(f)
		require.NoError(t, err)
	})

	t.Run("non-null ops on non-optional field stay allowed", func(t *testing.T) {
		f := fieldWithFIQLOps("name", false, false, EQ, NEQ, Contains)
		ops, err := fieldFIQLAnnotationFn(f)
		require.NoError(t, err)
		assert.Len(t, ops, 3)
	})
}

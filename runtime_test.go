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

package entdomain_test

import (
	"testing"

	"github.com/danhtran94/entdomain"
	"github.com/stretchr/testify/assert"
)

func TestApplyConfig_ShouldApply(t *testing.T) {
	t.Run("no options — always apply", func(t *testing.T) {
		cfg := entdomain.NewApplyConfig()
		assert.True(t, cfg.ShouldApply("name", "alice"))
		assert.True(t, cfg.ShouldApply("score", 0))
		assert.True(t, cfg.ShouldApply("active", false))
	})

	t.Run("OmitZeroVal skips zero values", func(t *testing.T) {
		cfg := entdomain.NewApplyConfig(entdomain.OmitZeroVal())
		assert.True(t, cfg.ShouldApply("name", "alice"))
		assert.False(t, cfg.ShouldApply("name", ""))
		assert.False(t, cfg.ShouldApply("score", 0))
		assert.True(t, cfg.ShouldApply("score", 42))
		assert.False(t, cfg.ShouldApply("active", false))
		assert.True(t, cfg.ShouldApply("active", true))
	})

	t.Run("OmitFields skips specified fields", func(t *testing.T) {
		cfg := entdomain.NewApplyConfig(entdomain.OmitFields("bio", "status"))
		assert.False(t, cfg.ShouldApply("bio", "hello"))
		assert.False(t, cfg.ShouldApply("status", "active"))
		assert.True(t, cfg.ShouldApply("name", "alice"))
	})

	t.Run("OnlyFields restricts to allowlisted fields", func(t *testing.T) {
		cfg := entdomain.NewApplyConfig(entdomain.OnlyFields("name", "status"))
		assert.True(t, cfg.ShouldApply("name", "alice"))
		assert.True(t, cfg.ShouldApply("status", "active"))
		assert.False(t, cfg.ShouldApply("bio", "hello"))
		assert.False(t, cfg.ShouldApply("score", 10))
	})

	t.Run("OmitFields + OmitZeroVal combination", func(t *testing.T) {
		cfg := entdomain.NewApplyConfig(
			entdomain.OmitZeroVal(),
			entdomain.OmitFields("bio"),
		)
		assert.False(t, cfg.ShouldApply("bio", "value")) // explicitly omitted
		assert.False(t, cfg.ShouldApply("name", ""))     // zero value
		assert.True(t, cfg.ShouldApply("name", "alice")) // non-zero, not omitted
	})
}

func TestApplyConfig_ShouldApplyPtr(t *testing.T) {
	bio := "hello"

	t.Run("no options — apply even if nil", func(t *testing.T) {
		cfg := entdomain.NewApplyConfig()
		assert.True(t, cfg.ShouldApplyPtr("bio", (*string)(nil)))
		assert.True(t, cfg.ShouldApplyPtr("bio", &bio))
	})

	t.Run("OmitNil skips nil pointers", func(t *testing.T) {
		cfg := entdomain.NewApplyConfig(entdomain.OmitNil())
		assert.False(t, cfg.ShouldApplyPtr("bio", (*string)(nil)))
		assert.True(t, cfg.ShouldApplyPtr("bio", &bio))
	})

	t.Run("OmitFields skips specified fields regardless of nil", func(t *testing.T) {
		cfg := entdomain.NewApplyConfig(entdomain.OmitFields("bio"))
		assert.False(t, cfg.ShouldApplyPtr("bio", &bio))
		assert.False(t, cfg.ShouldApplyPtr("bio", (*string)(nil)))
		assert.True(t, cfg.ShouldApplyPtr("name", &bio))
	})

	t.Run("OnlyFields restricts pointer fields", func(t *testing.T) {
		cfg := entdomain.NewApplyConfig(entdomain.OnlyFields("bio"))
		assert.True(t, cfg.ShouldApplyPtr("bio", &bio))
		assert.False(t, cfg.ShouldApplyPtr("score", (*int)(nil)))
	})
}

func TestApplyConfig_IsAppendEdge(t *testing.T) {
	t.Run("no AppendEdge option — default replace semantics", func(t *testing.T) {
		cfg := entdomain.NewApplyConfig()
		assert.False(t, cfg.IsAppendEdge("post_ids"))
		assert.False(t, cfg.IsAppendEdge("tag_ids"))
	})

	t.Run("AppendEdge enables append for specified edge", func(t *testing.T) {
		cfg := entdomain.NewApplyConfig(entdomain.AppendEdge("post_ids"))
		assert.True(t, cfg.IsAppendEdge("post_ids"))
		assert.False(t, cfg.IsAppendEdge("tag_ids"))
	})

	t.Run("multiple AppendEdge options", func(t *testing.T) {
		cfg := entdomain.NewApplyConfig(
			entdomain.AppendEdge("post_ids"),
			entdomain.AppendEdge("tag_ids"),
		)
		assert.True(t, cfg.IsAppendEdge("post_ids"))
		assert.True(t, cfg.IsAppendEdge("tag_ids"))
		assert.False(t, cfg.IsAppendEdge("comment_ids"))
	})
}

func TestAnnotation_Name(t *testing.T) {
	assert.Equal(t, "EntDomain", entdomain.EntityAnnotation{}.Name())
	assert.Equal(t, "EntDomainEdge", entdomain.EdgeAnnotation{}.Name())
}

func TestEdgeAnnotation_HasIDsHasNest(t *testing.T) {
	idsOnly := entdomain.Edge(entdomain.IDs())
	nestOnly := entdomain.Edge(entdomain.Nest())
	both := entdomain.Edge(entdomain.IDs(), entdomain.Nest())
	none := entdomain.Edge()

	assert.True(t, idsOnly.HasIDs())
	assert.False(t, idsOnly.HasNest())

	assert.False(t, nestOnly.HasIDs())
	assert.True(t, nestOnly.HasNest())

	assert.True(t, both.HasIDs())
	assert.True(t, both.HasNest())

	assert.False(t, none.HasIDs())
	assert.False(t, none.HasNest())
}

func TestEntityAnnotation_VirtualFields(t *testing.T) {
	ant := entdomain.Entity(
		entdomain.VirtualField("full_name", entdomain.String),
		entdomain.VirtualField("is_premium", entdomain.Bool),
		entdomain.VirtualField("price", entdomain.GoType("Decimal", "github.com/shopspring/decimal")),
	)
	assert.Equal(t, "EntDomain", ant.Name())
	assert.Len(t, ant.VirtualFields, 3)
	assert.Equal(t, "full_name", ant.VirtualFields[0].Name)
	assert.Equal(t, entdomain.String, ant.VirtualFields[0].FieldType)
	assert.Equal(t, "is_premium", ant.VirtualFields[1].Name)
	assert.Equal(t, entdomain.Bool, ant.VirtualFields[1].FieldType)
	assert.Equal(t, "price", ant.VirtualFields[2].Name)
	assert.Equal(t, "github.com/shopspring/decimal", ant.VirtualFields[2].FieldType.PkgPath)
	assert.Equal(t, "Decimal", ant.VirtualFields[2].FieldType.TypeName)
}

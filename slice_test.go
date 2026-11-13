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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sliceType builds a domain entity in the given slice ("" = root).
func sliceType(name, slice string) *gen.Type {
	var opts []entityOption
	if slice != "" {
		opts = append(opts, Slice(slice))
	}
	return &gen.Type{
		Name:        name,
		Annotations: gen.Annotations{Entity(opts...).Name(): Entity(opts...)},
	}
}

// addEdge appends an edge with the given modes from -> to.
func addEdge(from, to *gen.Type, name string, opts ...EdgeOption) {
	from.Edges = append(from.Edges, &gen.Edge{
		Name:        name,
		Type:        to,
		Annotations: gen.Annotations{Edge(opts...).Name(): Edge(opts...)},
	})
}

func TestValidateSliceName(t *testing.T) {
	for _, ok := range []string{"billing", "identity", "a", "v2", "user"} {
		assert.NoError(t, validateSliceName(ok), ok)
	}
	for _, bad := range []string{"", "Billing", "a/b", "a-b", "2fa", "pbmap", "func", "type", "bill ing"} {
		assert.Error(t, validateSliceName(bad), "%q must be rejected", bad)
	}
}

func TestValidateSliceNames_NamesEntity(t *testing.T) {
	err := validateSliceNames([]*gen.Type{sliceType("Invoice", "Billing"), sliceType("Account", "identity")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "entity Invoice")
	assert.Contains(t, err.Error(), `"Billing"`)
	assert.NotContains(t, err.Error(), "Account")
}

func TestValidateSliceEdges(t *testing.T) {
	t.Run("same slice nest allowed", func(t *testing.T) {
		inv, line := sliceType("Invoice", "billing"), sliceType("LineItem", "billing")
		addEdge(inv, line, "line_items", Nest())
		addEdge(line, inv, "invoice", Nest())
		assert.NoError(t, validateSliceEdges([]*gen.Type{inv, line}))
	})
	t.Run("cross slice IDs allowed", func(t *testing.T) {
		inv, acc := sliceType("Invoice", "billing"), sliceType("Account", "identity")
		addEdge(inv, acc, "account", IDs())
		addEdge(acc, inv, "invoices", IDs())
		assert.NoError(t, validateSliceEdges([]*gen.Type{inv, acc}))
	})
	t.Run("cross slice nest rejected with entity and edge", func(t *testing.T) {
		inv, acc := sliceType("Invoice", "billing"), sliceType("Account", "identity")
		addEdge(inv, acc, "account", IDs(), Nest())
		err := validateSliceEdges([]*gen.Type{inv, acc})
		require.Error(t, err)
		msg := err.Error()
		assert.Contains(t, msg, "cross-slice Nest")
		assert.Contains(t, msg, "Invoice.account")
		assert.Contains(t, msg, `"billing"`)
		assert.Contains(t, msg, `"identity"`)
		assert.Contains(t, msg, "use entdomain.IDs() for this edge or move both entities into one slice")
	})
	t.Run("root to slice is cross slice and prints <root>", func(t *testing.T) {
		inv, acc := sliceType("Invoice", "billing"), sliceType("Account", "")
		addEdge(acc, inv, "invoices", Nest())
		err := validateSliceEdges([]*gen.Type{inv, acc})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "<root>")
	})
	t.Run("every offending edge is reported", func(t *testing.T) {
		a, b := sliceType("A", "one"), sliceType("B", "two")
		addEdge(a, b, "to_b", Nest())
		addEdge(b, a, "to_a", Nest())
		err := validateSliceEdges([]*gen.Type{a, b})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "A.to_b")
		assert.Contains(t, err.Error(), "B.to_a")
	})
	t.Run("unannotated target is not a domain entity", func(t *testing.T) {
		inv := sliceType("Invoice", "billing")
		addEdge(inv, &gen.Type{Name: "Audit"}, "audit", Nest())
		assert.NoError(t, validateSliceEdges([]*gen.Type{inv}))
	})
}

func TestGroupBySlice(t *testing.T) {
	inv, line := sliceType("Invoice", "billing"), sliceType("LineItem", "billing")
	acc, tag := sliceType("Account", "identity"), sliceType("Tag", "")
	skip := &gen.Type{Name: "Internal"} // no entdomain annotation

	groups, err := groupBySlice([]*gen.Type{inv, acc, tag, line, skip}, "internal/domain", "domain")
	require.NoError(t, err)
	require.Len(t, groups, 3)

	assert.Equal(t, sliceGroup{Slice: "", Dir: "internal/domain", PkgName: "domain", Alias: "domain", Nodes: []*gen.Type{tag}}, groups[0])
	assert.Equal(t, "billing", groups[1].Slice)
	assert.Equal(t, "internal/domain/billing", groups[1].Dir)
	assert.Equal(t, "billing", groups[1].PkgName)
	assert.Equal(t, "billingdomain", groups[1].Alias)
	assert.Equal(t, []*gen.Type{inv, line}, groups[1].Nodes, "graph order is kept")
	assert.Equal(t, "identitydomain", groups[2].Alias)
}

func TestGroupBySlice_NoSlicesIsOneRootGroup(t *testing.T) {
	groups, err := groupBySlice([]*gen.Type{sliceType("User", ""), sliceType("Post", "")}, "internal/domain", "domain")
	require.NoError(t, err)
	require.Len(t, groups, 1)
	assert.Equal(t, "internal/domain", groups[0].Dir)
	assert.Len(t, groups[0].Nodes, 2)

	empty, err := groupBySlice(nil, "internal/domain", "domain")
	require.NoError(t, err)
	assert.Len(t, empty, 1, "an empty graph keeps the root group, as before slices existed")
}

func TestGroupBySlice_AllSlicedOmitsRoot(t *testing.T) {
	groups, err := groupBySlice([]*gen.Type{sliceType("Invoice", "billing")}, "internal/domain", "domain")
	require.NoError(t, err)
	require.Len(t, groups, 1)
	assert.Equal(t, "billing", groups[0].Slice)
}

func TestEntityAnnotation_MergeKeepsSlice(t *testing.T) {
	merged := Entity().Merge(Entity(Slice("billing"))).(EntityAnnotation)
	assert.Equal(t, "billing", merged.Slice)
	kept := Entity(Slice("billing")).Merge(Entity()).(EntityAnnotation)
	assert.Equal(t, "billing", kept.Slice, "an empty Slice must not erase a set one")
}

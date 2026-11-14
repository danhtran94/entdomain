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
	"path/filepath"
	"regexp"
	"testing"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const boundFKDomainDir = "examples/boundfk/internal/domain"

// loadBoundFKGraph loads examples/boundfk/ent/schema: Ring plus RingVersion
// (required, immutable, IDs()), RingNote (optional) and RingTag (required,
// mutable, Nest, FIQL).
func loadBoundFKGraph(t *testing.T) *gen.Graph {
	t.Helper()
	target, err := filepath.Abs("examples/boundfk/ent")
	require.NoError(t, err)
	g, err := entc.LoadGraph(filepath.Join(target, "schema"), &gen.Config{Target: target})
	require.NoError(t, err)
	return g
}

func nodeByName(t *testing.T, g *gen.Graph, name string) *gen.Type {
	t.Helper()
	for _, n := range g.Nodes {
		if n.Name == name {
			return n
		}
	}
	require.FailNow(t, "entity not found", name)
	return nil
}

func edgeByName(t *testing.T, n *gen.Type, name string) *gen.Edge {
	t.Helper()
	for _, e := range n.Edges {
		if e.Name == name {
			return e
		}
	}
	require.FailNow(t, "edge not found", "%s.%s", n.Name, name)
	return nil
}

func count(src, re string) int { return len(regexp.MustCompile(re).FindAllString(src, -1)) }

// edgeIDsDeduped is true for the edge that owns a bound field and false for
// the inverse side, which only references it.
func TestEdgeIDsDeduped_RealGraph(t *testing.T) {
	g := loadBoundFKGraph(t)
	assert.True(t, edgeIDsDeduped(edgeByName(t, nodeByName(t, g, "RingVersion"), "ring")))
	assert.True(t, edgeIDsDeduped(edgeByName(t, nodeByName(t, g, "RingNote"), "ring")))
	assert.False(t, edgeIDsDeduped(edgeByName(t, nodeByName(t, g, "Ring"), "versions")),
		"the To side has no bound field and is not unique")
}

// Success Criteria 1, 2: the bound field is one regular domain field; an
// optional key is a pointer.
func TestBoundFK_DomainStructs(t *testing.T) {
	version := readText(t, filepath.Join(boundFKDomainDir, "ringversion_gen.go"))
	assert.Equal(t, 1, count(version, `RingID\s+uuid\.UUID`), "edge with IDs() and a bound field emits RingID once")
	assert.Equal(t, 1, count(version, `\bRingID\b`))

	note := readText(t, filepath.Join(boundFKDomainDir, "ringnote_gen.go"))
	assert.Regexp(t, `RingID\s+\*uuid\.UUID`, note)

	tag := readText(t, filepath.Join(boundFKDomainDir, "ringtag_gen.go"))
	assert.Regexp(t, `RingID\s+uuid\.UUID`, tag)
	assert.Regexp(t, `Ring\s+\*Ring`, tag, "Nest still adds the nested struct beside the key")
}

// Success Criteria 8, 11: no duplicate symbols and the FIQL registry entry.
func TestBoundFK_EntMapperAndFIQL(t *testing.T) {
	mapper := readText(t, "examples/boundfk/ent/domain.go")
	assert.Equal(t, 1, count(mapper, `RingVersionDomainFieldRingID\b`), "one constant even with IDs() on the bound edge")
	assert.Contains(t, mapper, "entdomain.PtrNonZero(e.RingID)", "optional bound key reads NULL as nil")

	fiql := readText(t, "examples/boundfk/ent/fiql.go")
	assert.Contains(t, fiql, `"ring_id": entdomain.FIQLUUID[predicate.RingTag]`)
}

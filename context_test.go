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

package entdomain_test

import (
	"context"
	"testing"

	"github.com/danhtran94/entdomain"
	"github.com/stretchr/testify/assert"
)

// Two distinct struct types used to verify that different T parameters
// produce distinct context keys under WithDomain.
type testUser struct {
	ID   int
	Name string
}

type testPost struct {
	ID    int
	Title string
}

func TestWithDomain_RoundTrip(t *testing.T) {
	u := &testUser{ID: 7, Name: "alice"}
	ctx := entdomain.WithDomain(context.Background(), u)

	got, ok := entdomain.DomainFrom[*testUser](ctx)
	assert.True(t, ok, "DomainFrom must find the value stashed by WithDomain")
	assert.Same(t, u, got, "retrieved pointer must be identical to the stashed one")
}

func TestDomainFrom_Absent(t *testing.T) {
	got, ok := entdomain.DomainFrom[*testUser](context.Background())
	assert.False(t, ok, "bare context must not yield a domain value")
	assert.Nil(t, got, "absent retrieval must return the zero value of T")
}

func TestWithDomain_DistinctTypesCoexist(t *testing.T) {
	u := &testUser{ID: 1}
	p := &testPost{ID: 2}

	ctx := context.Background()
	ctx = entdomain.WithDomain(ctx, u)
	ctx = entdomain.WithDomain(ctx, p)

	gotU, okU := entdomain.DomainFrom[*testUser](ctx)
	gotP, okP := entdomain.DomainFrom[*testPost](ctx)

	assert.True(t, okU)
	assert.True(t, okP)
	assert.Same(t, u, gotU, "User retrieval must not collide with Post stash")
	assert.Same(t, p, gotP, "Post retrieval must not collide with User stash")
}

func TestWithDomain_TypeAsymmetry(t *testing.T) {
	u := &testUser{ID: 1}
	ctx := entdomain.WithDomain(context.Background(), u)

	// Retrieving under a different T returns absent — the key is type-scoped.
	gotP, ok := entdomain.DomainFrom[*testPost](ctx)
	assert.False(t, ok, "key is type-scoped: asking for *testPost must not find a *testUser")
	assert.Nil(t, gotP)
}

func TestWithDomain_LastWriteWins(t *testing.T) {
	first := &testUser{ID: 1, Name: "first"}
	second := &testUser{ID: 2, Name: "second"}

	ctx := context.Background()
	ctx = entdomain.WithDomain(ctx, first)
	ctx = entdomain.WithDomain(ctx, second)

	got, ok := entdomain.DomainFrom[*testUser](ctx)
	assert.True(t, ok)
	assert.Same(t, second, got, "innermost WithDomain must shadow earlier one of the same type")
}

func TestDomainFrom_NilValueStash(t *testing.T) {
	// Stashing a nil pointer under the type key is legal and retrievable.
	// Hook authors must rely on the returned pointer value, not the ok flag,
	// to distinguish "absent" from "present but nil".
	var nilUser *testUser
	ctx := entdomain.WithDomain(context.Background(), nilUser)

	got, ok := entdomain.DomainFrom[*testUser](ctx)
	assert.True(t, ok, "ok signals presence of the key, regardless of value")
	assert.Nil(t, got, "nil pointer round-trips as nil")
}

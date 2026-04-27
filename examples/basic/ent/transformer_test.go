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

// Behavioral tests for ADR-005: verify that generated ApplyDomain* methods
// actually invoke transformers with the expected ctx + domain struct, that
// errors propagate, that X-variants panic, and that bulk paths short-circuit.
//
// These tests deliberately avoid Save() — transformer invocation happens at
// ApplyDomain time, before persistence. We only need a valid ent.Client to
// construct builders; no DB rows are written.

package ent_test

import (
	"context"
	"errors"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/danhtran94/entdomain/examples/basic/domain"
	"github.com/danhtran94/entdomain/examples/basic/ent"
	"github.com/danhtran94/entdomain/examples/basic/ent/enttest"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestClient builds an in-memory sqlite ent.Client. The schema is created
// so client.User.Create() returns a valid builder; nothing is persisted.
func newTestClient(t *testing.T) *ent.Client {
	t.Helper()
	c := enttest.Open(t, "sqlite3", "file:ent-tx?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// withUserTransformer swaps the package-level ent.UserTransformer for the
// duration of t and restores it on cleanup. Tests using this MUST NOT call
// t.Parallel() — the transformer var is package-global.
func withUserTransformer(t *testing.T, tr *ent.UserDomainTransformer) {
	t.Helper()
	prev := ent.UserTransformer
	ent.UserTransformer = tr
	t.Cleanup(func() { ent.UserTransformer = prev })
}

// sampleUser produces a fully-populated domain.User so transformers can
// observe sibling fields. All required ent columns have non-zero values.
func sampleUser() *domain.User {
	bio := "engineer"
	score := 100
	return &domain.User{
		Name:                 "alice",
		Bio:                  &bio,
		Status:               domain.UserStatusActive,
		Username:             "alice",
		Score:                &score,
		ExternalID:           uuid.New(),
		Labels:               map[string]any{"team": "platform"},
		TagNames:             []string{"gopher"},
		Metadata:             domain.UserMetadata{},
		FullName:             "Alice Example",
		IsPremium:            true,
		ExpiresAt:            time.Now().Add(24 * time.Hour),
		SubscriptionDuration: 30 * 24 * time.Hour,
	}
}

// ─── Create path ──────────────────────────────────────────────────────────

func TestApplyDomainCreate_TransformerReceivesCtxAndDomain(t *testing.T) {
	client := newTestClient(t)

	type capture struct {
		ctx context.Context
		d   *domain.User
		val string
	}
	var got capture
	withUserTransformer(t, &ent.UserDomainTransformer{
		SetFullNameOnCreate: func(ctx context.Context, c *ent.UserCreate, d *domain.User, val string) error {
			got = capture{ctx: ctx, d: d, val: val}
			return nil
		},
	})

	type ctxKey struct{}
	inCtx := context.WithValue(context.Background(), ctxKey{}, "marker")
	d := sampleUser()

	_, err := client.User.Create().ApplyDomain(inCtx, d)
	require.NoError(t, err)

	assert.Equal(t, inCtx, got.ctx, "transformer must receive the ctx passed to ApplyDomain")
	assert.Equal(t, "marker", got.ctx.Value(ctxKey{}), "ctx values must be preserved end-to-end")
	assert.Same(t, d, got.d, "transformer must receive the same *domain.User pointer (sibling access)")
	assert.Equal(t, "Alice Example", got.val, "transformer must receive the virtual field value")
	assert.Equal(t, "alice", got.d.Username, "sibling fields (Username) must be populated on d")
	assert.True(t, got.d.IsPremium, "sibling virtual fields (IsPremium) must be visible on d")
}

func TestApplyDomainCreate_ErrorPropagates(t *testing.T) {
	client := newTestClient(t)

	sentinel := errors.New("encrypt failed")
	withUserTransformer(t, &ent.UserDomainTransformer{
		SetFullNameOnCreate: func(ctx context.Context, c *ent.UserCreate, d *domain.User, val string) error {
			return sentinel
		},
	})

	b, err := client.User.Create().ApplyDomain(context.Background(), sampleUser())
	assert.ErrorIs(t, err, sentinel, "transformer error must propagate unchanged")
	assert.Nil(t, b, "on transformer error, builder must be nil")
}

func TestApplyDomainCreate_NilTransformer_NoOp(t *testing.T) {
	client := newTestClient(t)
	withUserTransformer(t, nil)

	b, err := client.User.Create().ApplyDomain(context.Background(), sampleUser())
	require.NoError(t, err, "nil transformer must not be an error condition")
	assert.NotNil(t, b)
}

func TestApplyDomainCreate_NilHookField_IsSkipped(t *testing.T) {
	client := newTestClient(t)

	// Transformer present, but the specific hook we need is nil — the codegen
	// must gate each hook on a nil check and skip silently.
	withUserTransformer(t, &ent.UserDomainTransformer{
		SetFullNameOnCreate: nil, // explicitly nil
		// SetIsPremiumOnCreate also absent.
	})

	b, err := client.User.Create().ApplyDomain(context.Background(), sampleUser())
	require.NoError(t, err, "nil hook field must not panic or error")
	assert.NotNil(t, b)
}

func TestApplyDomainCreate_MultipleHooksAllFire(t *testing.T) {
	client := newTestClient(t)

	var fullNameCalled, isPremiumCalled bool
	withUserTransformer(t, &ent.UserDomainTransformer{
		SetFullNameOnCreate: func(ctx context.Context, c *ent.UserCreate, d *domain.User, val string) error {
			fullNameCalled = true
			return nil
		},
		SetIsPremiumOnCreate: func(ctx context.Context, c *ent.UserCreate, d *domain.User, val bool) error {
			isPremiumCalled = true
			return nil
		},
	})

	_, err := client.User.Create().ApplyDomain(context.Background(), sampleUser())
	require.NoError(t, err)
	assert.True(t, fullNameCalled, "FullName hook must fire")
	assert.True(t, isPremiumCalled, "IsPremium hook must fire")
}

func TestApplyDomainCreate_FirstErrorShortCircuits(t *testing.T) {
	client := newTestClient(t)

	var secondCalled bool
	sentinel := errors.New("stop")
	withUserTransformer(t, &ent.UserDomainTransformer{
		SetFullNameOnCreate: func(ctx context.Context, c *ent.UserCreate, d *domain.User, val string) error {
			return sentinel
		},
		// The second hook must NOT be invoked once the first returns err.
		SetIsPremiumOnCreate: func(ctx context.Context, c *ent.UserCreate, d *domain.User, val bool) error {
			secondCalled = true
			return nil
		},
	})

	_, err := client.User.Create().ApplyDomain(context.Background(), sampleUser())
	require.ErrorIs(t, err, sentinel)
	assert.False(t, secondCalled, "subsequent hooks must not run after a hook returns error")
}

// ─── UpdateOne path ───────────────────────────────────────────────────────

func TestApplyDomainUpdateOne_TransformerReceivesCtxAndDomain(t *testing.T) {
	client := newTestClient(t)

	var gotCtx context.Context
	var gotD *domain.User
	withUserTransformer(t, &ent.UserDomainTransformer{
		SetFullNameOnUpdate: func(ctx context.Context, u *ent.UserUpdateOne, d *domain.User, val string) error {
			gotCtx = ctx
			gotD = d
			return nil
		},
	})

	inCtx := context.Background()
	d := sampleUser()
	d.ID = 42

	_, err := client.User.UpdateOneID(d.ID).ApplyDomain(inCtx, d)
	require.NoError(t, err)
	assert.Equal(t, inCtx, gotCtx)
	assert.Same(t, d, gotD)
}

func TestApplyDomainUpdateOne_ErrorPropagates(t *testing.T) {
	client := newTestClient(t)

	sentinel := errors.New("update encrypt failed")
	withUserTransformer(t, &ent.UserDomainTransformer{
		SetFullNameOnUpdate: func(ctx context.Context, u *ent.UserUpdateOne, d *domain.User, val string) error {
			return sentinel
		},
	})

	d := sampleUser()
	d.ID = 1
	b, err := client.User.UpdateOneID(d.ID).ApplyDomain(context.Background(), d)
	assert.ErrorIs(t, err, sentinel)
	assert.Nil(t, b)
}

// ─── Update / Upsert paths (no transformer fire — always nil error) ──────

func TestApplyDomainUpdate_AlwaysNilErr(t *testing.T) {
	client := newTestClient(t)
	// Transformer errors on Update path would be ignored because Update
	// does not invoke transformer hooks — verify the path returns nil.
	withUserTransformer(t, &ent.UserDomainTransformer{
		SetFullNameOnCreate: func(ctx context.Context, c *ent.UserCreate, d *domain.User, val string) error {
			t.Fatal("Create hook must not fire on Update path")
			return nil
		},
		SetFullNameOnUpdate: func(ctx context.Context, u *ent.UserUpdateOne, d *domain.User, val string) error {
			t.Fatal("UpdateOne hook must not fire on Update (bulk where) path")
			return nil
		},
	})

	b, err := client.User.Update().ApplyDomain(context.Background(), sampleUser())
	require.NoError(t, err, "Update path claims always-nil error in ADR-005; contract must hold")
	assert.NotNil(t, b)
}

func TestApplyDomainUpsertOne_AlwaysNilErr(t *testing.T) {
	client := newTestClient(t)
	withUserTransformer(t, &ent.UserDomainTransformer{
		SetFullNameOnCreate: func(ctx context.Context, c *ent.UserCreate, d *domain.User, val string) error {
			// Create hook may fire on the initial Create() before OnConflict.
			// That's intended. The subsequent UpsertOne.ApplyDomain path
			// must not itself invoke any transformer.
			return nil
		},
	})

	initial, err := client.User.Create().ApplyDomain(context.Background(), sampleUser())
	require.NoError(t, err)

	b, err := initial.OnConflictColumns("username").ApplyDomain(context.Background(), sampleUser())
	require.NoError(t, err, "UpsertOne path must return nil error — no transformer fire")
	assert.NotNil(t, b)
}

// ─── X-variants: opt-in panic ────────────────────────────────────────────

func TestApplyDomainCreateX_PanicsOnTransformerError(t *testing.T) {
	client := newTestClient(t)

	sentinel := errors.New("kaboom")
	withUserTransformer(t, &ent.UserDomainTransformer{
		SetFullNameOnCreate: func(ctx context.Context, c *ent.UserCreate, d *domain.User, val string) error {
			return sentinel
		},
	})

	assert.PanicsWithValue(t, sentinel, func() {
		_ = client.User.Create().ApplyDomainX(context.Background(), sampleUser())
	}, "ApplyDomainX must panic with the exact error returned by the transformer")
}

func TestApplyDomainCreateX_ReturnsBuilderOnSuccess(t *testing.T) {
	client := newTestClient(t)
	withUserTransformer(t, nil) // pure path, no hook

	b := client.User.Create().ApplyDomainX(context.Background(), sampleUser())
	assert.NotNil(t, b, "ApplyDomainX must return the underlying builder on success")
}

func TestApplyDomainUpdateOneX_PanicsOnTransformerError(t *testing.T) {
	client := newTestClient(t)

	sentinel := errors.New("update kaboom")
	withUserTransformer(t, &ent.UserDomainTransformer{
		SetFullNameOnUpdate: func(ctx context.Context, u *ent.UserUpdateOne, d *domain.User, val string) error {
			return sentinel
		},
	})

	d := sampleUser()
	d.ID = 7

	assert.PanicsWithValue(t, sentinel, func() {
		_ = client.User.UpdateOneID(d.ID).ApplyDomainX(context.Background(), d)
	})
}

// ─── Bulk paths: short-circuit on first error ────────────────────────────

func TestCreateBulkDomain_ShortCircuitsOnFirstError(t *testing.T) {
	client := newTestClient(t)

	var callCount int
	sentinel := errors.New("row 2 failed")
	withUserTransformer(t, &ent.UserDomainTransformer{
		SetFullNameOnCreate: func(ctx context.Context, c *ent.UserCreate, d *domain.User, val string) error {
			callCount++
			if callCount == 2 {
				return sentinel
			}
			return nil
		},
	})

	ds := domain.UserList{sampleUser(), sampleUser(), sampleUser()}
	b, err := client.User.CreateBulkDomain(context.Background(), ds)
	assert.ErrorIs(t, err, sentinel, "bulk must surface the first transformer error")
	assert.Nil(t, b, "bulk must not return a partial builder")
	assert.Equal(t, 2, callCount, "bulk must abort at the failing row — third row must not be processed")
}

func TestCreateBulkDomainX_PanicsOnFirstError(t *testing.T) {
	client := newTestClient(t)

	sentinel := errors.New("bulk kaboom")
	withUserTransformer(t, &ent.UserDomainTransformer{
		SetFullNameOnCreate: func(ctx context.Context, c *ent.UserCreate, d *domain.User, val string) error {
			return sentinel
		},
	})

	ds := domain.UserList{sampleUser()}
	assert.PanicsWithValue(t, sentinel, func() {
		_ = client.User.CreateBulkDomainX(context.Background(), ds)
	})
}

func TestUpdateBulkDomain_ShortCircuitsOnFirstError(t *testing.T) {
	client := newTestClient(t)

	var callCount int
	sentinel := errors.New("update row failed")
	withUserTransformer(t, &ent.UserDomainTransformer{
		SetFullNameOnUpdate: func(ctx context.Context, u *ent.UserUpdateOne, d *domain.User, val string) error {
			callCount++
			if callCount == 2 {
				return sentinel
			}
			return nil
		},
	})

	d1, d2, d3 := sampleUser(), sampleUser(), sampleUser()
	d1.ID, d2.ID, d3.ID = 1, 2, 3

	b, err := client.User.UpdateBulkDomain(context.Background(), domain.UserList{d1, d2, d3})
	assert.ErrorIs(t, err, sentinel)
	assert.Nil(t, b)
	assert.Equal(t, 2, callCount, "bulk update must abort at the failing row")
}

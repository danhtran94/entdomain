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

package ent_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danhtran94/entdomain/examples/boundfk/ent"
	"github.com/danhtran94/entdomain/examples/boundfk/ent/enttest"
	"github.com/danhtran94/entdomain/examples/boundfk/ent/ringtag"
	"github.com/danhtran94/entdomain/examples/boundfk/internal/domain"
)

// newClient opens an isolated in-memory database with foreign keys enforced,
// so a missing or wrong ring_id fails like it would in production.
func newClient(t *testing.T) *ent.Client {
	t.Helper()
	c := enttest.Open(t, "sqlite3", "file:"+uuid.NewString()+"?mode=memory&_fk=1")
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestToDomainReadsFKWithoutEagerLoad(t *testing.T) {
	ctx := context.Background()
	c := newClient(t)
	ring := c.Ring.Create().SaveX(ctx)
	v := c.RingVersion.Create().SetRingID(ring.ID).SetVersion(1).SaveX(ctx)

	// No WithRing(): the FK comes from the entity field, not the loaded edge.
	got := c.RingVersion.GetX(ctx, v.ID).ToDomain()
	assert.Equal(t, ring.ID, got.RingID)
}

func TestCreateApplyDomainWritesFK(t *testing.T) {
	ctx := context.Background()
	c := newClient(t)
	ring := c.Ring.Create().SaveX(ctx)

	cr, err := c.RingVersion.Create().ApplyDomain(ctx, &domain.RingVersion{RingID: ring.ID, Version: 1})
	require.NoError(t, err)
	saved := cr.SaveX(ctx)
	assert.Equal(t, ring.ID, c.RingVersion.GetX(ctx, saved.ID).RingID)
}

func TestUpdateFKMutableAndImmutable(t *testing.T) {
	ctx := context.Background()
	c := newClient(t)
	r1 := c.Ring.Create().SaveX(ctx)
	r2 := c.Ring.Create().SaveX(ctx)

	t.Run("mutable FK changes", func(t *testing.T) {
		tag := c.RingTag.Create().SetRingID(r1.ID).SetName("mutable").SaveX(ctx)
		u, err := c.RingTag.UpdateOneID(tag.ID).ApplyDomain(ctx, &domain.RingTag{RingID: r2.ID, Name: "mutable"})
		require.NoError(t, err)
		u.SaveX(ctx)
		assert.Equal(t, r2.ID, c.RingTag.GetX(ctx, tag.ID).RingID)
	})

	t.Run("immutable FK is untouched", func(t *testing.T) {
		v := c.RingVersion.Create().SetRingID(r1.ID).SetVersion(1).SaveX(ctx)
		u, err := c.RingVersion.UpdateOneID(v.ID).ApplyDomain(ctx, &domain.RingVersion{RingID: r2.ID, Version: 2})
		require.NoError(t, err)
		u.SaveX(ctx)
		got := c.RingVersion.GetX(ctx, v.ID)
		assert.Equal(t, r1.ID, got.RingID, "immutable ring_id must not move")
		assert.Equal(t, 2, got.Version, "other fields still update")
	})
}

func TestOptionalFKNullRoundTrip(t *testing.T) {
	ctx := context.Background()
	c := newClient(t)
	ring := c.Ring.Create().SaveX(ctx)

	set := c.RingNote.Create().SetRingID(ring.ID).SetText("set").SaveX(ctx)
	d := c.RingNote.GetX(ctx, set.ID).ToDomain()
	require.NotNil(t, d.RingID)
	assert.Equal(t, ring.ID, *d.RingID)

	null := c.RingNote.Create().SetText("orphan").SaveX(ctx)
	nd := c.RingNote.GetX(ctx, null.ID).ToDomain()
	assert.Nil(t, nd.RingID, "NULL FK must read as nil, not a pointer to uuid.Nil")

	// Writing the NULL row's domain value back must not violate the FK.
	nd.Text = "orphan copy"
	cr, err := c.RingNote.Create().ApplyDomain(ctx, nd)
	require.NoError(t, err)
	cr.SaveX(ctx)
}

func TestOptionalFKNilDoesNotClear(t *testing.T) {
	ctx := context.Background()
	c := newClient(t)
	ring := c.Ring.Create().SaveX(ctx)
	note := c.RingNote.Create().SetRingID(ring.ID).SetText("keep").SaveX(ctx)

	u, err := c.RingNote.UpdateOneID(note.ID).ApplyDomain(ctx, &domain.RingNote{Text: "changed"})
	require.NoError(t, err)
	u.SaveX(ctx)

	got := c.RingNote.GetX(ctx, note.ID)
	assert.Equal(t, ring.ID, got.RingID, "nil means unchanged, never cleared")
	assert.Equal(t, "changed", got.Text)
}

func TestNestAndFKCoexist(t *testing.T) {
	ctx := context.Background()
	c := newClient(t)
	ring := c.Ring.Create().SaveX(ctx)
	c.RingTag.Create().SetRingID(ring.ID).SetName("nested").SaveX(ctx)

	t.Run("with WithRing both are set", func(t *testing.T) {
		d := c.RingTag.Query().Where(ringtag.Name("nested")).WithRing().OnlyX(ctx).ToDomain()
		assert.Equal(t, ring.ID, d.RingID)
		require.NotNil(t, d.Ring)
		assert.Equal(t, ring.ID, d.Ring.ID)
	})

	t.Run("without WithRing the FK is set and the nest is nil", func(t *testing.T) {
		d := c.RingTag.Query().Where(ringtag.Name("nested")).OnlyX(ctx).ToDomain()
		assert.Equal(t, ring.ID, d.RingID)
		assert.Nil(t, d.Ring)
	})
}

func TestUpsertApplyDomainWritesFK(t *testing.T) {
	ctx := context.Background()
	c := newClient(t)
	r1 := c.Ring.Create().SaveX(ctx)
	r2 := c.Ring.Create().SaveX(ctx)

	t.Run("required FK is replaced on conflict", func(t *testing.T) {
		tag := c.RingTag.Create().SetRingID(r1.ID).SetName("upsert").SaveX(ctx)
		up, err := c.RingTag.Create().SetRingID(r1.ID).SetName("upsert").
			OnConflictColumns(ringtag.FieldName).
			ApplyDomain(ctx, &domain.RingTag{RingID: r2.ID, Name: "upsert"})
		require.NoError(t, err)
		up.ExecX(ctx)
		assert.Equal(t, r2.ID, c.RingTag.GetX(ctx, tag.ID).RingID)
	})

	t.Run("optional FK honors nil on conflict", func(t *testing.T) {
		note := c.RingNote.Create().SetRingID(r1.ID).SetText("upsert").SaveX(ctx)
		up, err := c.RingNote.Create().SetText("upsert").
			OnConflictColumns("text").
			ApplyDomain(ctx, &domain.RingNote{Text: "upsert"})
		require.NoError(t, err)
		up.ExecX(ctx)
		assert.Equal(t, r1.ID, c.RingNote.GetX(ctx, note.ID).RingID, "nil leaves the FK unchanged")

		up, err = c.RingNote.Create().SetText("upsert").
			OnConflictColumns("text").
			ApplyDomain(ctx, &domain.RingNote{RingID: &r2.ID, Text: "upsert"})
		require.NoError(t, err)
		up.ExecX(ctx)
		assert.Equal(t, r2.ID, c.RingNote.GetX(ctx, note.ID).RingID)
	})
}

func TestFIQLFilterByFK(t *testing.T) {
	ctx := context.Background()
	c := newClient(t)
	r1 := c.Ring.Create().SaveX(ctx)
	r2 := c.Ring.Create().SaveX(ctx)
	c.RingTag.Create().SetRingID(r1.ID).SetName("a").SaveX(ctx)
	c.RingTag.Create().SetRingID(r2.ID).SetName("b").SaveX(ctx)

	pred, err := ent.RingTagFIQL("ring_id==" + r1.ID.String())
	require.NoError(t, err)
	got := c.RingTag.Query().Where(pred).AllX(ctx)
	require.Len(t, got, 1)
	assert.Equal(t, "a", got[0].Name)
}

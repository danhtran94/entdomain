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

// E2E tests for ENTD-009: ApplyDomain must honor OnlyFields / OmitFields /
// OmitZeroVal for IDs edges, so a field-restricted update never unlinks
// relations the caller excluded.

package ent_test

import (
	"context"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/danhtran94/entdomain"
	"github.com/danhtran94/entdomain/examples/basic/domain"
	"github.com/danhtran94/entdomain/examples/basic/ent"
	"github.com/danhtran94/entdomain/examples/basic/ent/enttest"
	"github.com/danhtran94/entdomain/examples/basic/ent/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newEdgeTestClient opens a private in-memory database per test so seeded
// rows never leak between tests.
func newEdgeTestClient(t *testing.T) *ent.Client {
	t.Helper()
	withUserTransformer(t, nil)
	c := enttest.Open(t, "sqlite3", "file:"+t.Name()+"?mode=memory&_fk=1")
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// seededUser is a user linked to two posts and one tag.
type seededUser struct {
	user  *ent.User
	posts []*ent.Post
	tag   *ent.Tag
}

func seedUserWithEdges(t *testing.T, c *ent.Client) seededUser {
	t.Helper()
	ctx := context.Background()
	u := c.User.Create().ApplyDomainX(ctx, sampleUser()).SaveX(ctx)
	p1 := c.Post.Create().SetTitle("p1").SetOwner(u).SaveX(ctx)
	p2 := c.Post.Create().SetTitle("p2").SetOwner(u).SaveX(ctx)
	tag := c.Tag.Create().SetName("t").AddUsers(u).SaveX(ctx)
	return seededUser{user: u, posts: []*ent.Post{p1, p2}, tag: tag}
}

// readWithoutEdges is the read half of a read-modify-write: edges are not
// eager-loaded, so the domain value carries nil edge IDs.
func readWithoutEdges(t *testing.T, c *ent.Client, id int) *domain.User {
	t.Helper()
	d := c.User.GetX(context.Background(), id).ToDomain()
	require.Nil(t, d.PostIDs, "precondition: posts not eager-loaded")
	return d
}

func postCount(c *ent.Client, u *ent.User) int {
	return c.User.QueryPosts(u).CountX(context.Background())
}

func TestUpdateOneOnlyFieldsKeepsEdges(t *testing.T) {
	ctx := context.Background()
	c := newEdgeTestClient(t)
	s := seedUserWithEdges(t, c)

	d := readWithoutEdges(t, c, s.user.ID)
	d.Name = "renamed"
	c.User.UpdateOneID(s.user.ID).
		ApplyDomainX(ctx, d, entdomain.OnlyFields(ent.UserDomainFieldName)).
		ExecX(ctx)

	assert.Equal(t, "renamed", c.User.GetX(ctx, s.user.ID).Name)
	assert.Equal(t, 2, postCount(c, s.user), "OnlyFields(name) must not touch posts")
	assert.Equal(t, 1, c.User.QueryTags(s.user).CountX(ctx), "OnlyFields(name) must not touch tags")
}

func TestUpdateWhereOnlyFieldsKeepsEdges(t *testing.T) {
	ctx := context.Background()
	c := newEdgeTestClient(t)
	a, b := seedUserWithEdges(t, c), seedUserWithEdges(t, c)

	c.User.Update().
		ApplyDomainX(ctx,
			&domain.User{Status: domain.UserStatusInactive},
			entdomain.OnlyFields(ent.UserDomainFieldStatus),
		).
		Where(user.StatusEQ(user.StatusActive)).
		ExecX(ctx)

	for _, s := range []seededUser{a, b} {
		assert.Equal(t, user.StatusInactive, c.User.GetX(ctx, s.user.ID).Status)
		assert.Equal(t, 2, postCount(c, s.user), "user %d posts", s.user.ID)
		assert.Equal(t, 1, c.User.QueryTags(s.user).CountX(ctx), "user %d tags", s.user.ID)
	}
}

func TestUpdateOneOmitFieldsKeepsEdges(t *testing.T) {
	ctx := context.Background()
	c := newEdgeTestClient(t)
	s := seedUserWithEdges(t, c)

	d := readWithoutEdges(t, c, s.user.ID)
	c.User.UpdateOneID(s.user.ID).
		ApplyDomainX(ctx, d, entdomain.OmitFields(ent.UserDomainFieldPostIDs)).
		ExecX(ctx)

	assert.Equal(t, 2, postCount(c, s.user))
}

func TestUpdateOneOnlyFieldsIncludingEdgeReplaces(t *testing.T) {
	ctx := context.Background()
	c := newEdgeTestClient(t)
	s := seedUserWithEdges(t, c)

	d := readWithoutEdges(t, c, s.user.ID)
	d.PostIDs = []int{s.posts[0].ID}
	c.User.UpdateOneID(s.user.ID).
		ApplyDomainX(ctx, d, entdomain.OnlyFields(ent.UserDomainFieldName, ent.UserDomainFieldPostIDs)).
		ExecX(ctx)

	ids := c.User.QueryPosts(s.user).IDsX(ctx)
	assert.Equal(t, []int{s.posts[0].ID}, ids, "an allowlisted edge is replaced")
}

// Pins replace-by-default (ADR-001:234). Changing it is scope U1, not ENTD-009.
func TestUpdateOneDefaultStillReplaces(t *testing.T) {
	ctx := context.Background()
	c := newEdgeTestClient(t)
	s := seedUserWithEdges(t, c)

	d := readWithoutEdges(t, c, s.user.ID)
	c.User.UpdateOneID(s.user.ID).ApplyDomainX(ctx, d).ExecX(ctx)

	assert.Equal(t, 0, postCount(c, s.user), "no options: nil PostIDs replaces with the empty set")
}

func TestUpdateOneAppendEdgeOutsideOnlyFields(t *testing.T) {
	ctx := context.Background()
	c := newEdgeTestClient(t)
	s := seedUserWithEdges(t, c)
	p3 := c.Post.Create().SetTitle("p3").SaveX(ctx)

	d := readWithoutEdges(t, c, s.user.ID)
	d.PostIDs = []int{p3.ID}
	c.User.UpdateOneID(s.user.ID).
		ApplyDomainX(ctx, d,
			entdomain.OnlyFields(ent.UserDomainFieldName),
			entdomain.AppendEdge(ent.UserDomainFieldPostIDs),
		).
		ExecX(ctx)

	assert.Equal(t, 3, postCount(c, s.user), "AppendEdge applies even outside OnlyFields")
}

func TestUpdateOneOmitZeroValNilKeepsEmptyClears(t *testing.T) {
	ctx := context.Background()
	c := newEdgeTestClient(t)
	s := seedUserWithEdges(t, c)

	d := readWithoutEdges(t, c, s.user.ID)
	c.User.UpdateOneID(s.user.ID).ApplyDomainX(ctx, d, entdomain.OmitZeroVal()).ExecX(ctx)
	assert.Equal(t, 2, postCount(c, s.user), "OmitZeroVal: nil PostIDs leaves the edge")

	d.PostIDs = []int{}
	c.User.UpdateOneID(s.user.ID).ApplyDomainX(ctx, d, entdomain.OmitZeroVal()).ExecX(ctx)
	assert.Equal(t, 0, postCount(c, s.user), "OmitZeroVal: empty PostIDs clears the edge")
}

func TestCreateOmitFieldsSkipsEdge(t *testing.T) {
	ctx := context.Background()
	c := newEdgeTestClient(t)
	p := c.Post.Create().SetTitle("p").SaveX(ctx)

	d := sampleUser()
	d.PostIDs = []int{p.ID}
	u := c.User.Create().
		ApplyDomainX(ctx, d, entdomain.OmitFields(ent.UserDomainFieldPostIDs)).
		SaveX(ctx)

	assert.Equal(t, 0, postCount(c, u))
}

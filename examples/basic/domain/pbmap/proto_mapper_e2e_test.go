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

// E2E tests for ENTD-010: generated FromProto mappers reject untrusted input
// with an error instead of panicking or inventing values.

package pbmap_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danhtran94/entdomain/examples/basic/domain"
	"github.com/danhtran94/entdomain/examples/basic/domain/pbmap"
	"github.com/danhtran94/entdomain/examples/basic/proto/entpb"
)

// validProtoUser is a proto message every field of which maps without error.
func validProtoUser() *entpb.User {
	return &entpb.User{
		Name:       "alice",
		Status:     entpb.UserStatus_USER_STATUS_ACTIVE,
		ExternalId: uuid.NewString(),
	}
}

func TestUserFromProtoRejectsMalformedUUID(t *testing.T) {
	for _, bad := range []string{"", "not-a-uuid"} {
		p := validProtoUser()
		p.ExternalId = bad

		var (
			d   *domain.User
			err error
		)
		require.NotPanics(t, func() { d, err = pbmap.UserFromProto(p) }, "ExternalId %q", bad)
		require.Error(t, err, "ExternalId %q", bad)
		assert.Nil(t, d)
		assert.Contains(t, err.Error(), "User.external_id")
	}
}

func TestUserFromProtoRejectsUnspecifiedEnum(t *testing.T) {
	for _, s := range []entpb.UserStatus{entpb.UserStatus_USER_STATUS_UNSPECIFIED, entpb.UserStatus(99)} {
		p := validProtoUser()
		p.Status = s

		d, err := pbmap.UserFromProto(p)
		require.Error(t, err, "Status %v", s)
		assert.Nil(t, d)
		assert.Contains(t, err.Error(), "User.status")
	}
}

func TestUserFromProtoNilIsNil(t *testing.T) {
	d, err := pbmap.UserFromProto(nil)
	require.NoError(t, err)
	assert.Nil(t, d)
}

func TestUserListFromProtoReportsIndex(t *testing.T) {
	bad := validProtoUser()
	bad.ExternalId = "not-a-uuid"

	ds, err := pbmap.UserListFromProto([]*entpb.User{validProtoUser(), bad})
	require.Error(t, err)
	assert.Nil(t, ds)
	assert.Contains(t, err.Error(), "index 1")
	assert.Contains(t, err.Error(), "User.external_id")
}

func TestUserFromProtoNestedErrorPropagates(t *testing.T) {
	badUser := validProtoUser()
	badUser.ExternalId = "not-a-uuid"
	p := validProtoUser()
	p.Tags = []*entpb.Tag{{Name: "ok"}, {Name: "t", Users: []*entpb.User{badUser}}}

	d, err := pbmap.UserFromProto(p)
	require.Error(t, err)
	assert.Nil(t, d)
	assert.Contains(t, err.Error(), "User.tags: index 1: Tag.users: index 0: User.external_id")
}

func TestUserProtoRoundTrip(t *testing.T) {
	bio := "engineer"
	score := 100
	updated := time.Date(2026, 11, 17, 8, 30, 0, 0, time.UTC)
	d := &domain.User{
		ID:                   7,
		Name:                 "alice",
		Bio:                  &bio,
		Status:               domain.UserStatusInactive,
		CreatedAt:            time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Score:                &score,
		ExternalID:           uuid.New(),
		UpdatedAt:            &updated,
		Settings:             map[string]any{"theme": "dark"},
		Labels:               map[string]any{"team": "platform"},
		TagNames:             []string{"gopher"},
		Metadata:             domain.UserMetadata{Links: []string{"https://example.com"}},
		PostIDs:              []int{1, 2},
		Posts:                domain.PostList{{ID: 1, Title: "p1", Published: true, OwnerID: 7, PinnerIDs: []int{7}}},
		PinnedPostID:         1,
		PinnedPost:           &domain.Post{ID: 1, Title: "p1", Published: true, OwnerID: 7, PinnerIDs: []int{7}},
		TagIDs:               []int{3},
		// ListFromProto maps an absent repeated field to an empty, non-nil list.
		Tags:                 domain.TagList{{ID: 3, Name: "go", UserIDs: []int{7}, Users: domain.UserList{}}},
		FullName:             "Alice Example",
		IsPremium:            true,
		ExpiresAt:            time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		SubscriptionDuration: 30 * 24 * time.Hour,
	}

	got, err := pbmap.UserFromProto(pbmap.UserToProto(d))
	require.NoError(t, err)
	assert.Equal(t, d, got)
}

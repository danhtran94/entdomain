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

package ent_test

import (
	"testing"

	"entgo.io/ent/dialect/sql"
	"github.com/danhtran94/entdomain/examples/basic/ent"
	"github.com/danhtran94/entdomain/examples/basic/ent/predicate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// applySQL applies a predicate to a fresh selector and returns the resulting SQL query.
func applySQL(pred predicate.User) string {
	s := sql.Dialect("sqlite3").Select("*").From(sql.Table("users"))
	pred(s)
	q, _ := s.Query()
	return q
}

func TestUserFIQL_SimpleEQ(t *testing.T) {
	pred, err := ent.UserFIQL("name==john")
	require.NoError(t, err)
	assert.Contains(t, applySQL(pred), "`name` = ?")
}

func TestUserFIQL_StringContains(t *testing.T) {
	pred, err := ent.UserFIQL("name=like=jo")
	require.NoError(t, err)
	assert.Contains(t, applySQL(pred), "`name` LIKE ?")
}

func TestUserFIQL_IntGT(t *testing.T) {
	pred, err := ent.UserFIQL("score=gt=100")
	require.NoError(t, err)
	assert.Contains(t, applySQL(pred), "`score` > ?")
}

func TestUserFIQL_IntRange(t *testing.T) {
	// score >= 10 AND score <= 100
	pred, err := ent.UserFIQL("score=ge=10;score=le=100")
	require.NoError(t, err)
	q := applySQL(pred)
	assert.Contains(t, q, "`score` >= ?")
	assert.Contains(t, q, "`score` <= ?")
	assert.Contains(t, q, "AND")
}

func TestUserFIQL_EnumEQ(t *testing.T) {
	pred, err := ent.UserFIQL("status==active")
	require.NoError(t, err)
	assert.Contains(t, applySQL(pred), "`status` = ?")
}

func TestUserFIQL_EnumOR(t *testing.T) {
	// status==active OR status==inactive
	pred, err := ent.UserFIQL("status==active,status==inactive")
	require.NoError(t, err)
	q := applySQL(pred)
	assert.Contains(t, q, "OR")
	assert.Contains(t, q, "`status` = ?")
}

func TestUserFIQL_TimeGTE(t *testing.T) {
	pred, err := ent.UserFIQL("created_at=ge=2024-01-01T00:00:00Z")
	require.NoError(t, err)
	assert.Contains(t, applySQL(pred), "`created_at` >= ?")
}

func TestUserFIQL_TimeRange(t *testing.T) {
	pred, err := ent.UserFIQL("created_at=ge=2024-01-01T00:00:00Z;created_at=le=2024-12-31T23:59:59Z")
	require.NoError(t, err)
	q := applySQL(pred)
	assert.Contains(t, q, "`created_at` >= ?")
	assert.Contains(t, q, "`created_at` <= ?")
	assert.Contains(t, q, "AND")
}

func TestUserFIQL_ComplexPrecedence(t *testing.T) {
	// name==john;score=gt=25,status==active
	// → (name AND score) OR status
	pred, err := ent.UserFIQL("name==john;score=gt=25,status==active")
	require.NoError(t, err)
	q := applySQL(pred)
	assert.Contains(t, q, "OR")
	assert.Contains(t, q, "AND")
	// AND clause comes before OR at the top level
	andIdx := len(q) - len(q[indexOf(q, "AND"):])
	orIdx := len(q) - len(q[indexOf(q, "OR"):])
	assert.Less(t, andIdx, orIdx, "AND group must appear before OR in: %s", q)
}

func TestUserFIQL_GroupingOverridesPrecedence(t *testing.T) {
	// (name==john,status==active);score=gt=0
	// → (name OR status) AND score
	pred, err := ent.UserFIQL("(name==john,status==active);score=gt=0")
	require.NoError(t, err)
	q := applySQL(pred)
	assert.Contains(t, q, "AND")
	assert.Contains(t, q, "OR")
	// The OR group is wrapped in parens before the AND (ent qualifies columns with table name)
	assert.Contains(t, q, "(`users`.`name` = ? OR `users`.`status` = ?) AND `users`.`score` > ?")
}

func TestUserFIQL_UUIDEQ(t *testing.T) {
	pred, err := ent.UserFIQL("external_id==550e8400-e29b-41d4-a716-446655440000")
	require.NoError(t, err)
	assert.Contains(t, applySQL(pred), "`external_id` = ?")
}

func TestUserFIQL_UUIDNEQ(t *testing.T) {
	pred, err := ent.UserFIQL("external_id!=550e8400-e29b-41d4-a716-446655440000")
	require.NoError(t, err)
	assert.Contains(t, applySQL(pred), "`external_id` <> ?")
}

func TestUserFIQL_UUIDInvalid(t *testing.T) {
	_, err := ent.UserFIQL("external_id==not-a-uuid")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid UUID value")
}

func TestUserFIQL_IntIn(t *testing.T) {
	pred, err := ent.UserFIQL("score=in=(10,20,30)")
	require.NoError(t, err)
	q := applySQL(pred)
	assert.Contains(t, q, "`score` IN (?")
}

func TestUserFIQL_IntNotIn(t *testing.T) {
	pred, err := ent.UserFIQL("score=out=(10,20)")
	require.NoError(t, err)
	q := applySQL(pred)
	assert.Contains(t, q, "`score` NOT IN (?")
}

func TestUserFIQL_EnumIn(t *testing.T) {
	// Enum =in= composes via OR-of-EQ at apply time (no FieldIn predicate generated).
	pred, err := ent.UserFIQL("status=in=(active,inactive)")
	require.NoError(t, err)
	q := applySQL(pred)
	assert.Contains(t, q, "OR")
	assert.Contains(t, q, "`status` = ?")
}

func TestUserFIQL_EnumNotIn(t *testing.T) {
	pred, err := ent.UserFIQL("status=out=(active)")
	require.NoError(t, err)
	q := applySQL(pred)
	assert.Contains(t, q, "`status` <> ?")
}

func TestUserFIQL_InEmptyList(t *testing.T) {
	_, err := ent.UserFIQL("score=in=()")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty value list")
}

func TestUserFIQL_InBadElement(t *testing.T) {
	_, err := ent.UserFIQL("score=in=(10,abc)")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `invalid integer value "abc" in list`)
}

// Error cases on the real generated registry

func TestUserFIQL_UnknownField(t *testing.T) {
	_, err := ent.UserFIQL("email==foo")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown field")
	assert.Contains(t, err.Error(), "email")
}

func TestUserFIQL_UnannotatedField(t *testing.T) {
	// bio has no FIQL annotation
	_, err := ent.UserFIQL("bio==hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown field")
}

func TestUserFIQL_InvalidEnumValue(t *testing.T) {
	_, err := ent.UserFIQL("status==pending")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown enum value")
	assert.Contains(t, err.Error(), "pending")
}

func TestUserFIQL_InvalidIntValue(t *testing.T) {
	_, err := ent.UserFIQL("score=gt=notanumber")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid integer value")
}

func TestUserFIQL_InvalidTimeValue(t *testing.T) {
	_, err := ent.UserFIQL("created_at=ge=not-a-time")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid time value")
}

func TestUserFIQL_DisallowedOperator(t *testing.T) {
	// created_at only has GTE and LTE — EQ is not annotated
	_, err := ent.UserFIQL("created_at==2024-01-01T00:00:00Z")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")
}

// indexOf returns the byte index of substr in s, or len(s) if not found.
func indexOf(s, substr string) int {
	for i := range s {
		if i+len(substr) <= len(s) && s[i:i+len(substr)] == substr {
			return i
		}
	}
	return len(s)
}

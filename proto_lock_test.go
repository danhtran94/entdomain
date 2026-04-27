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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func emptyLock() *ProtoLockFile {
	return &ProtoLockFile{
		Version:  1,
		Entities: make(map[string]ProtoEntityLock),
	}
}

func TestAllocateFieldNumbers_IDAlwaysOne(t *testing.T) {
	lock := emptyLock()
	nums := allocateFieldNumbers(lock, "User", []string{"id", "name", "bio"})

	assert.Equal(t, 1, nums["id"], "id must always be field number 1")
	assert.NotEqual(t, 1, nums["name"], "name must not be 1")
	assert.NotEqual(t, 1, nums["bio"], "bio must not be 1")
}

func TestAllocateFieldNumbers_Sequential(t *testing.T) {
	lock := emptyLock()
	fields := []string{"id", "name", "bio", "status"}
	nums := allocateFieldNumbers(lock, "User", fields)

	assert.Equal(t, 1, nums["id"])
	// Other fields get sequential numbers > 1.
	for _, f := range fields {
		if f == "id" {
			continue
		}
		assert.Greater(t, nums[f], 1, "field %s should have number > 1", f)
	}
	// All numbers are unique.
	seen := map[int]string{}
	for name, n := range nums {
		prev, dup := seen[n]
		assert.False(t, dup, "field number %d used by both %s and %s", n, prev, name)
		seen[n] = name
	}
}

func TestAllocateFieldNumbers_Stable(t *testing.T) {
	// First generation.
	lock := emptyLock()
	fields := []string{"id", "name", "bio", "score"}
	first := allocateFieldNumbers(lock, "User", fields)

	// Second generation with the same fields: numbers must not change.
	second := allocateFieldNumbers(lock, "User", fields)

	for _, f := range fields {
		assert.Equal(t, first[f], second[f], "field %s number changed between generations", f)
	}
}

func TestAllocateFieldNumbers_RemovedFieldsReserved(t *testing.T) {
	lock := emptyLock()
	// Initial fields.
	initial := []string{"id", "name", "old_field"}
	nums := allocateFieldNumbers(lock, "User", initial)
	oldNum := nums["old_field"]

	// Remove "old_field", add "new_field".
	updated := []string{"id", "name", "new_field"}
	nums2 := allocateFieldNumbers(lock, "User", updated)

	// "old_field" number must now be reserved.
	entity := lock.Entities["User"]
	reserved := make(map[int]bool)
	for _, n := range entity.Reserved {
		reserved[n] = true
	}
	assert.True(t, reserved[oldNum], "old_field number %d should be reserved", oldNum)

	// "new_field" must not reuse the reserved number.
	assert.NotEqual(t, oldNum, nums2["new_field"], "new_field must not reuse reserved number %d", oldNum)
}

func TestAllocateFieldNumbers_ReservedNeverReused(t *testing.T) {
	lock := emptyLock()

	// Add and remove several fields, building up reserved list.
	gen1 := []string{"id", "a", "b", "c"}
	allocateFieldNumbers(lock, "E", gen1)

	gen2 := []string{"id", "d", "e", "f"} // remove a,b,c; add d,e,f
	nums2 := allocateFieldNumbers(lock, "E", gen2)

	entity := lock.Entities["E"]
	reserved := make(map[int]bool)
	for _, n := range entity.Reserved {
		reserved[n] = true
	}

	// Verify new fields don't collide with reserved.
	for name, n := range nums2 {
		assert.False(t, reserved[n], "field %s reuses reserved number %d", name, n)
	}
}

func TestAllocateFieldNumbers_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, ".entdomain.lock.json")

	// Create lock and allocate.
	lock1, err := loadLockFile(lockPath)
	require.NoError(t, err)

	fields := []string{"id", "name", "email", "status"}
	nums1 := allocateFieldNumbers(lock1, "User", fields)
	require.NoError(t, saveLockFile(lockPath, lock1))

	// Load and re-allocate with same fields.
	lock2, err := loadLockFile(lockPath)
	require.NoError(t, err)

	nums2 := allocateFieldNumbers(lock2, "User", fields)

	for _, f := range fields {
		assert.Equal(t, nums1[f], nums2[f], "field %s number changed after round-trip", f)
	}
}

func TestLoadLockFile_NonExistent(t *testing.T) {
	lf, err := loadLockFile(filepath.Join(t.TempDir(), "nonexistent.json"))
	require.NoError(t, err)
	assert.NotNil(t, lf)
	assert.Equal(t, 1, lf.Version)
	assert.NotNil(t, lf.Entities)
}

func TestSaveLockFile(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "lock.json")

	lf := &ProtoLockFile{
		Version: 1,
		Entities: map[string]ProtoEntityLock{
			"User": {
				Fields:   map[string]int{"id": 1, "name": 2},
				Reserved: []int{3, 4},
			},
		},
	}

	require.NoError(t, saveLockFile(lockPath, lf))

	// Verify file exists and is valid JSON.
	data, err := os.ReadFile(lockPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"id": 1`)
}

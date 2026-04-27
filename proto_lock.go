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
	"encoding/json"
	"errors"
	"os"
	"sort"
)

// ProtoLockFile is the in-memory representation of the .entdomain.lock.json file.
// It records the field number assignment for each proto entity to ensure stability.
type ProtoLockFile struct {
	Version  int                        `json:"version"`
	Entities map[string]ProtoEntityLock `json:"entities"`
}

// ProtoEntityLock records the field numbers assigned to an entity's fields,
// plus any field numbers that have been permanently retired (reserved).
type ProtoEntityLock struct {
	Fields   map[string]int `json:"fields"`   // snake_name → field number
	Reserved []int          `json:"reserved"` // permanently retired numbers
}

// loadLockFile reads the lock file at path.
// If the file does not exist, an empty lock is returned (no error).
func loadLockFile(path string) (*ProtoLockFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &ProtoLockFile{
				Version:  1,
				Entities: make(map[string]ProtoEntityLock),
			}, nil
		}
		return nil, err
	}
	lf := &ProtoLockFile{}
	if err := json.Unmarshal(data, lf); err != nil {
		return nil, err
	}
	if lf.Entities == nil {
		lf.Entities = make(map[string]ProtoEntityLock)
	}
	return lf, nil
}

// saveLockFile writes the lock file to path in pretty-printed JSON.
func saveLockFile(path string, lf *ProtoLockFile) error {
	data, err := json.MarshalIndent(lf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// allocateFieldNumbers reconciles the current list of fields against the lock file
// for the named entity and returns a stable field-number map.
//
// Rules:
//   - "id" is always assigned field number 1.
//   - Fields present in the lock keep their existing numbers.
//   - Fields that were removed have their numbers added to Reserved (never reused).
//   - New fields receive the next available number: max(allocated ∪ reserved) + 1.
func allocateFieldNumbers(lock *ProtoLockFile, entity string, currentFields []string) map[string]int {
	el, ok := lock.Entities[entity]
	if !ok {
		el = ProtoEntityLock{
			Fields:   make(map[string]int),
			Reserved: nil,
		}
	}
	if el.Fields == nil {
		el.Fields = make(map[string]int)
	}

	// Build a set of currently requested fields for quick lookup.
	currentSet := make(map[string]bool, len(currentFields))
	for _, f := range currentFields {
		currentSet[f] = true
	}

	// Retire field numbers that are no longer in the current schema.
	reservedSet := make(map[int]bool, len(el.Reserved))
	for _, n := range el.Reserved {
		reservedSet[n] = true
	}
	for name, num := range el.Fields {
		if !currentSet[name] {
			if !reservedSet[num] {
				el.Reserved = append(el.Reserved, num)
				reservedSet[num] = true
			}
			delete(el.Fields, name)
		}
	}

	// Ensure "id" = 1.
	if currentSet["id"] {
		el.Fields["id"] = 1
		reservedSet[1] = true
	}

	// Compute next available field number.
	maxNum := 1
	for _, n := range el.Fields {
		if n > maxNum {
			maxNum = n
		}
	}
	for n := range reservedSet {
		if n > maxNum {
			maxNum = n
		}
	}

	// Assign numbers to new fields in stable (sorted) order.
	var newFields []string
	for _, f := range currentFields {
		if f == "id" {
			continue
		}
		if _, exists := el.Fields[f]; !exists {
			newFields = append(newFields, f)
		}
	}
	sort.Strings(newFields)
	for _, f := range newFields {
		maxNum++
		el.Fields[f] = maxNum
	}

	// Sort reserved list for deterministic output.
	sort.Ints(el.Reserved)

	lock.Entities[entity] = el
	return el.Fields
}

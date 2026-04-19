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

package entdomain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestOpRegistryCovered guards against drift between the FIQLOp constant
// block and the opByName registry. Every constant must have an entry so
// templates can address it by Go-identifier name via the isOp helper.
//
// Adding a new FIQLOp constant without an opByName entry fails this test.
func TestOpRegistryCovered(t *testing.T) {
	wantOps := []FIQLOp{EQ, NEQ, GT, LT, GTE, LTE, Contains, HasPrefix, In, NotIn}
	for _, op := range wantOps {
		t.Run(string(op), func(t *testing.T) {
			found := false
			for _, registered := range opByName {
				if registered == op {
					found = true
					break
				}
			}
			assert.True(t, found, "FIQLOp %q is not in opByName — add it to the registry so templates can address it via isOp", op)
		})
	}

	// Inverse: every registry entry must map to a known constant. Catches
	// stale entries left behind after a constant rename or removal.
	wantSet := make(map[FIQLOp]bool)
	for _, op := range wantOps {
		wantSet[op] = true
	}
	for name, op := range opByName {
		assert.True(t, wantSet[op], "opByName[%q] = %q has no matching FIQLOp constant — remove the stale entry or add the constant to wantOps", name, op)
	}
}

func TestIsOpFn(t *testing.T) {
	t.Run("matches by Go-identifier name", func(t *testing.T) {
		assert.True(t, isOpFn("In", "=in="))
		assert.True(t, isOpFn("EQ", "=="))
		assert.True(t, isOpFn("NotIn", "=out="))
	})
	t.Run("rejects non-matching pair", func(t *testing.T) {
		assert.False(t, isOpFn("In", "=out="))
		assert.False(t, isOpFn("EQ", "!="))
	})
	t.Run("unknown name returns false (no panic)", func(t *testing.T) {
		assert.False(t, isOpFn("DoesNotExist", "=in="))
	})
}

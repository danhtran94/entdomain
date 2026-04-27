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
	"go/parser"
	"go/token"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCustomLayout validates that WithPackagePath works correctly when the ent
// directory is nested several levels below the module root (examples/custom/repo/ent),
// rather than directly at the module root. This exercises the moduleRoot fix that
// walks up to go.mod instead of assuming ent is one level below the module root.
func TestCustomLayout_DomainFileLandsAtCorrectPath(t *testing.T) {
	_, err := os.Stat("examples/custom/internal/domain/user_gen.go")
	assert.NoError(t, err, "domain file must exist at examples/custom/internal/domain/ — not next to ent dir")
}

func TestCustomLayout_EntDomainImportsCorrectDomainPackage(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "examples/custom/repo/ent/domain.go", nil, parser.ParseComments)
	require.NoError(t, err)
	assert.True(t,
		hasImport(f, "github.com/danhtran94/entdomain/examples/custom/internal/domain"),
		"generated ent/domain.go must import examples/custom/internal/domain — not a path relative to repo/ent",
	)
}

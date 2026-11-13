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
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const slicedDomainDir = "examples/sliced/internal/domain"

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err, "reading %s", path)
	return string(data)
}

// packageClause returns the identifier of the first `package` line.
func packageClause(t *testing.T, src string) string {
	t.Helper()
	m := regexp.MustCompile(`(?m)^package (\w+)`).FindStringSubmatch(src)
	require.NotNil(t, m, "no package clause")
	return m[1]
}

// Success Criterion 3: each slice is its own directory and package; nothing
// lands in the root domain directory.
func TestSliced_OutputLayout(t *testing.T) {
	invoice := readFile(t, filepath.Join(slicedDomainDir, "billing/invoice_gen.go"))
	assert.Equal(t, "billing", packageClause(t, invoice))
	lineItem := readFile(t, filepath.Join(slicedDomainDir, "billing/lineitem_gen.go"))
	assert.Equal(t, "billing", packageClause(t, lineItem))
	account := readFile(t, filepath.Join(slicedDomainDir, "identity/account_gen.go"))
	assert.Equal(t, "identity", packageClause(t, account))

	rootGen, err := filepath.Glob(filepath.Join(slicedDomainDir, "*_gen.go"))
	require.NoError(t, err)
	assert.Empty(t, rootGen, "no domain file may land in the root domain directory")
}

// Success Criterion 4: ent/domain.go imports one package per slice under the
// <slice>domain alias.
func TestSliced_EntMapperAliases(t *testing.T) {
	src := readFile(t, "examples/sliced/ent/domain.go")
	assert.Contains(t, src, "func (e *Invoice) ToDomain() *billingdomain.Invoice")
	assert.Contains(t, src, "func (e *LineItem) ToDomain() *billingdomain.LineItem")
	assert.Contains(t, src, "func (e *Account) ToDomain() *identitydomain.Account")
	assert.Contains(t, src, `billingdomain "github.com/danhtran94/entdomain/examples/sliced/internal/domain/billing"`)
	assert.Contains(t, src, `identitydomain "github.com/danhtran94/entdomain/examples/sliced/internal/domain/identity"`)
}

// Success Criterion 5: edges inside a slice still nest, in both directions.
func TestSliced_SameSliceNest(t *testing.T) {
	invoice := readFile(t, filepath.Join(slicedDomainDir, "billing/invoice_gen.go"))
	assert.Regexp(t, `LineItems\s+LineItemList`, invoice)
	lineItem := readFile(t, filepath.Join(slicedDomainDir, "billing/lineitem_gen.go"))
	assert.Regexp(t, `Invoice\s+\*Invoice`, lineItem)
}

// Success Criterion 6: an edge across slices carries IDs and imports nothing.
func TestSliced_CrossSliceEdgesCarryIDsOnly(t *testing.T) {
	invoice := readFile(t, filepath.Join(slicedDomainDir, "billing/invoice_gen.go"))
	assert.Regexp(t, `AccountID\s+int`, invoice)
	assert.Zero(t, strings.Count(invoice, "identity"), "billing must not mention identity")

	account := readFile(t, filepath.Join(slicedDomainDir, "identity/account_gen.go"))
	assert.Regexp(t, `InvoiceIDs\s+\[\]int`, account)
	assert.Zero(t, strings.Count(account, "billing"), "identity must not mention billing")
}

// Success Criterion 9: proto messages stay in one proto package.
func TestSliced_OneProtoPackage(t *testing.T) {
	proto := readFile(t, "examples/sliced/proto/entpb/ent_messages.proto")
	assert.Contains(t, proto, "package entpb;")
	for _, msg := range []string{"message Invoice", "message LineItem", "message Account"} {
		assert.Contains(t, proto, msg)
	}
	// Cross-slice references are plain IDs, never message types.
	assert.Regexp(t, `int64\s+account_id`, proto)
	assert.Regexp(t, `repeated\s+int64\s+invoice_ids`, proto)
}

// Success Criterion 10: each slice owns its mapper package.
func TestSliced_MapperPackagesPerSlice(t *testing.T) {
	invoice := readFile(t, filepath.Join(slicedDomainDir, "billing/pbmap/invoice_proto_gen.go"))
	assert.Equal(t, "pbmap", packageClause(t, invoice))
	assert.Contains(t, invoice, "func InvoiceToProto(d *billingdomain.Invoice)")

	account := readFile(t, filepath.Join(slicedDomainDir, "identity/pbmap/account_proto_gen.go"))
	assert.Equal(t, "pbmap", packageClause(t, account))
	assert.Contains(t, account, "func AccountToProto(d *identitydomain.Account)")

	_, err := os.Stat(filepath.Join(slicedDomainDir, "pbmap"))
	assert.True(t, os.IsNotExist(err), "no root pbmap directory: every entity is sliced")
}

// Success Criterion 11: a mapper package never imports a sibling slice.
func TestSliced_MappersDoNotImportSiblingSlices(t *testing.T) {
	billing, err := filepath.Glob(filepath.Join(slicedDomainDir, "billing/pbmap/*.go"))
	require.NoError(t, err)
	require.NotEmpty(t, billing)
	for _, f := range billing {
		assert.NotContains(t, readFile(t, f), "identity", f)
	}
	identity, err := filepath.Glob(filepath.Join(slicedDomainDir, "identity/pbmap/*.go"))
	require.NoError(t, err)
	require.NotEmpty(t, identity)
	for _, f := range identity {
		assert.NotContains(t, readFile(t, f), "billing", f)
	}
}

// Success Criterion 13: the compiled proto package is checked in, so
// `make build` type-checks every slice mapper against it.
func TestSliced_CompiledProtoIsCheckedIn(t *testing.T) {
	assert.FileExists(t, "examples/sliced/proto/entpb/ent_messages.pb.go")
}

// WithConversion expressions are emitted unqualified, so the hand-written
// functions must be defined in the entity's own slice mapper package.
func TestSliced_WithConversionResolvesInSliceMapperPackage(t *testing.T) {
	const dir = "examples/sliced/internal/domain/billing/pbmap"

	mapper := readFile(t, filepath.Join(dir, "invoice_proto_gen.go"))
	assert.Contains(t, mapper, "InvoiceNumberToProto(d.Number)")
	assert.Contains(t, mapper, "InvoiceNumberFromProto(p.Number)")

	conv := readFile(t, filepath.Join(dir, "number_conv.go"))
	assert.Equal(t, "pbmap", packageClause(t, conv))
	assert.Contains(t, conv, "func InvoiceNumberToProto(")
	assert.Contains(t, conv, "func InvoiceNumberFromProto(")

	// The identity mapper package has no such function: conversions are per slice.
	identity, err := filepath.Glob(filepath.Join(slicedDomainDir, "identity/pbmap/*.go"))
	require.NoError(t, err)
	for _, f := range identity {
		assert.NotContains(t, readFile(t, f), "InvoiceNumber", f)
	}
}

// WithProtoHelpersInDomain with slices: each mapper joins its own slice package.
func TestSlicedDomainMode_MappersLiveInSlicePackages(t *testing.T) {
	const dir = "examples/sliceddomain/internal/domain"

	invoice := readFile(t, filepath.Join(dir, "billing/invoice_proto_gen.go"))
	assert.Equal(t, "billing", packageClause(t, invoice))
	assert.Contains(t, invoice, "func (d *Invoice) ToProto() *entpb.Invoice")
	assert.Equal(t, "identity", packageClause(t, readFile(t, filepath.Join(dir, "identity/account_proto_gen.go"))))

	for _, sub := range []string{"billing/pbmap", "identity/pbmap", "pbmap"} {
		_, err := os.Stat(filepath.Join(dir, sub))
		assert.True(t, os.IsNotExist(err), "domain mode has no %s directory", sub)
	}
	rootGen, err := filepath.Glob(filepath.Join(dir, "*.go"))
	require.NoError(t, err)
	assert.Empty(t, rootGen, "no mapper may land in the root domain directory")

	// Slices stay independent: the billing package never mentions identity.
	billing, err := filepath.Glob(filepath.Join(dir, "billing/*.go"))
	require.NoError(t, err)
	for _, f := range billing {
		assert.NotContains(t, readFile(t, f), "identity", f)
	}
}

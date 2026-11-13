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
	"regexp"
	"strings"
	"testing"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// loadSlicedGraph loads examples/sliced/ent/schema, the two-slice fixture
// (billing: Invoice, LineItem; identity: Account).
func loadSlicedGraph(t *testing.T) *gen.Graph {
	t.Helper()
	target, err := filepath.Abs("examples/sliced/ent")
	require.NoError(t, err)
	g, err := entc.LoadGraph(filepath.Join(target, "schema"), &gen.Config{Target: target})
	require.NoError(t, err)
	return g
}

func protoCfg(opts ...ProtoOption) *ProtoConfig {
	cfg := &ProtoConfig{}
	all := append([]ProtoOption{
		WithProtoDir("examples/sliced/proto"),
		WithProtoPackageName("entpb"),
		WithProtoGoPackage("github.com/danhtran94/entdomain/examples/sliced/proto/entpb;entpb"),
	}, opts...)
	for _, o := range all {
		o(cfg)
	}
	return cfg
}

func readText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err, "reading %s", path)
	return string(data)
}

func mapperPackage(t *testing.T, path string) string {
	t.Helper()
	m := regexp.MustCompile(`(?m)^package (\w+)`).FindStringSubmatch(readText(t, path))
	require.NotNil(t, m, "no package clause in %s", path)
	return m[1]
}

// Subpackage mode: every slice owns {slice}/pbmap and imports only itself.
func TestProtoMappers_SubpackagePerSlice(t *testing.T) {
	g := loadSlicedGraph(t)
	out := t.TempDir()
	const domainPath = "examples/sliced/internal/domain"
	require.NoError(t, generateProtoMapperFiles(g, protoCfg(), out, domainPath, "domain"))

	invoice := filepath.Join(out, domainPath, "billing/pbmap/invoice_proto_gen.go")
	assert.Equal(t, "pbmap", mapperPackage(t, invoice))
	src := readText(t, invoice)
	assert.Contains(t, src, "func InvoiceToProto(d *billingdomain.Invoice)")
	assert.Contains(t, src, `billingdomain "github.com/danhtran94/entdomain/examples/sliced/internal/domain/billing"`)
	assert.NotContains(t, src, "identitydomain", "a mapper must not import a sibling slice")

	assert.FileExists(t, filepath.Join(out, domainPath, "billing/pbmap/lineitem_proto_gen.go"))
	account := filepath.Join(out, domainPath, "identity/pbmap/account_proto_gen.go")
	assert.Equal(t, "pbmap", mapperPackage(t, account))
	assert.Contains(t, readText(t, account), "func AccountToProto(d *identitydomain.Account)")
	assert.NotContains(t, readText(t, account), "billingdomain")

	assert.NoDirExists(t, filepath.Join(out, domainPath, "pbmap"), "no entity is in the root package")
}

// Domain mode: the mapper joins its slice's own domain package.
func TestProtoMappers_DomainModePerSlice(t *testing.T) {
	g := loadSlicedGraph(t)
	out := t.TempDir()
	const domainPath = "examples/sliced/internal/domain"
	require.NoError(t, generateProtoMapperFiles(g, protoCfg(WithProtoHelpersInDomain()), out, domainPath, "domain"))

	assert.Equal(t, "billing", mapperPackage(t, filepath.Join(out, domainPath, "billing/invoice_proto_gen.go")))
	assert.Equal(t, "billing", mapperPackage(t, filepath.Join(out, domainPath, "billing/lineitem_proto_gen.go")))
	assert.Equal(t, "identity", mapperPackage(t, filepath.Join(out, domainPath, "identity/account_proto_gen.go")))

	rootFiles, err := filepath.Glob(filepath.Join(out, domainPath, "*.go"))
	require.NoError(t, err)
	assert.Empty(t, rootFiles, "no mapper may land in the root directory: %s", strings.Join(rootFiles, ", "))
	assert.NoDirExists(t, filepath.Join(out, domainPath, "billing/pbmap"), "domain mode has no pbmap subpackage")
}

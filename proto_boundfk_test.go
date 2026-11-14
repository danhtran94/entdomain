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
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Success Criterion 12: proto messages and mappers carry the key exactly once.
func TestBoundFK_Proto(t *testing.T) {
	g := loadBoundFKGraph(t)
	cfg := &ProtoConfig{}
	for _, o := range []ProtoOption{
		WithProtoDir("examples/boundfk/proto"),
		WithProtoPackageName("entpb"),
		WithProtoGoPackage("github.com/danhtran94/entdomain/examples/boundfk/proto/entpb;entpb"),
	} {
		o(cfg)
	}

	out := t.TempDir()
	require.NoError(t, generateProtoFiles(g, cfg, out))
	proto := readText(t, filepath.Join(out, "examples/boundfk/proto", "entpb", "ent_messages.proto"))
	for _, msg := range []string{"RingVersion", "RingNote", "RingTag"} {
		body := regexp.MustCompile(`(?s)message ` + msg + ` \{(.*?)\n\}`).FindStringSubmatch(proto)
		require.NotNil(t, body, "message %s", msg)
		assert.Equal(t, 1, count(body[1], `\bring_id\b`), "%s: ring_id once", msg)
	}
	assert.Regexp(t, `optional string ring_id`, proto)

	mapperOut := t.TempDir()
	require.NoError(t, generateProtoMapperFiles(g, cfg, mapperOut, boundFKDomainDir, "domain"))
	for _, name := range []string{"ringversion", "ringnote", "ringtag"} {
		src := readText(t, filepath.Join(mapperOut, boundFKDomainDir, "pbmap", name+"_proto_gen.go"))
		assert.Equal(t, 1, count(src, `\bRingId:`), "%s mapper: RingId assigned once to proto", name)
		assert.Equal(t, 1, count(src, `\bRingID:`), "%s mapper: RingID assigned once to domain", name)
	}
}

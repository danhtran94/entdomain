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

package entdomain_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const protoFile = "internal/testdata/proto/entpb/ent_messages.proto"
const lockFile = "internal/testdata/proto/entpb/.entdomain.lock.json"

// protoFileContent reads the generated proto file as a string.
func protoFileContent(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(protoFile)
	require.NoError(t, err, "reading %s", protoFile)
	return string(data)
}

// TestProtoGenerate_FileExists verifies the generated files exist.
func TestProtoGenerate_FileExists(t *testing.T) {
	for _, path := range []string{
		protoFile,
		lockFile,
		// Mappers and helpers are generated in domain/pbmap/ subpackage by default.
		"internal/testdata/domain/pbmap/user_proto_gen.go",
		"internal/testdata/domain/pbmap/post_proto_gen.go",
		"internal/testdata/domain/pbmap/proto_helpers_gen.go",
	} {
		_, err := os.Stat(path)
		assert.NoError(t, err, "expected file to exist: %s", path)
	}
}

// TestProtoGenerate_Proto3Syntax checks the proto file uses proto3.
func TestProtoGenerate_Proto3Syntax(t *testing.T) {
	content := protoFileContent(t)
	assert.Contains(t, content, `syntax = "proto3"`, "proto file must use proto3 syntax")
}

// TestProtoGenerate_PackageAndGoPackage checks the package declarations.
func TestProtoGenerate_PackageAndGoPackage(t *testing.T) {
	content := protoFileContent(t)
	assert.Contains(t, content, "package entpb;")
	assert.Contains(t, content, `option go_package = "github.com/danhtran94/entdomain/internal/testdata/proto/entpb;entpb"`)
}

// TestProtoGenerate_UserMessage verifies the User proto message.
func TestProtoGenerate_UserMessage(t *testing.T) {
	content := protoFileContent(t)

	// Must contain User message.
	assert.Contains(t, content, "message User {")

	// id = 1.
	assert.Contains(t, content, "id = 1;", "id field must have number 1")

	// Required fields.
	assert.Contains(t, content, "string name", "name field")
	assert.Contains(t, content, "optional string bio", "bio must be optional (pointer type)")
	assert.Contains(t, content, "UserStatus status", "status enum field")
	assert.Contains(t, content, "google.protobuf.Timestamp created_at", "created_at Timestamp")
	assert.Contains(t, content, "optional int64 score", "score must be optional (pointer type)")
	assert.Contains(t, content, "repeated int64 post_ids", "post_ids repeated")
	assert.Contains(t, content, "string full_name", "full_name virtual field")
	assert.Contains(t, content, "bool is_premium", "is_premium virtual field")
	assert.Contains(t, content, "google.protobuf.Timestamp expires_at", "expires_at virtual field with explicit ProtoType")

	// username must NOT appear (SkipProto).
	assert.NotContains(t, content, "username", "username should be excluded (SkipProto)")

	// metadata must NOT appear (map type → excluded).
	assert.NotContains(t, content, "metadata", "metadata should be excluded (map type)")
}

// TestProtoGenerate_PostMessage verifies the Post proto message.
func TestProtoGenerate_PostMessage(t *testing.T) {
	content := protoFileContent(t)

	assert.Contains(t, content, "message Post {")
	assert.Contains(t, content, "id = 1;")
	assert.Contains(t, content, "string title")
	assert.Contains(t, content, "bool published")
	assert.Contains(t, content, "int64 owner_id", "singular IDs edge")
	assert.Contains(t, content, "repeated int64 pinner_ids", "plural IDs edge")
}

// TestProtoGenerate_EnumDefinition verifies the UserStatus enum is generated correctly.
func TestProtoGenerate_EnumDefinition(t *testing.T) {
	content := protoFileContent(t)

	assert.Contains(t, content, "enum UserStatus {")
	assert.Contains(t, content, "USER_STATUS_UNSPECIFIED = 0;", "unspecified sentinel must be 0")
	assert.Contains(t, content, "USER_STATUS_ACTIVE = 1;")
	assert.Contains(t, content, "USER_STATUS_INACTIVE = 2;")
}

// TestProtoGenerate_TimestampImport verifies the timestamp.proto import is included.
func TestProtoGenerate_TimestampImport(t *testing.T) {
	content := protoFileContent(t)
	assert.Contains(t, content, `import "google/protobuf/timestamp.proto"`)
}

// TestProtoGenerate_LockFile_IDIsOne verifies the lock file assigns id=1.
func TestProtoGenerate_LockFile_IDIsOne(t *testing.T) {
	data, err := os.ReadFile(lockFile)
	require.NoError(t, err)
	content := string(data)
	// Both entities should have "id": 1.
	assert.Contains(t, content, `"id": 1`)
}

// TestProtoGenerate_MapperUserMethods verifies domain/pbmap/user_proto_gen.go structure.
// Mappers live in the pbmap subpackage as standalone functions.
// Both domain and proto types are qualified.
func TestProtoGenerate_MapperUserMethods(t *testing.T) {
	data, err := os.ReadFile("internal/testdata/domain/pbmap/user_proto_gen.go")
	require.NoError(t, err)
	content := string(data)

	assert.Contains(t, content, "package pbmap", "mapper must be in the pbmap package")
	assert.Contains(t, content, "func UserToProto(d *domain.User)")
	assert.Contains(t, content, "func UserFromProto(p *entpb.User)")
	assert.Contains(t, content, "func UserListToProto(ds domain.UserList)")
	assert.Contains(t, content, "func UserListFromProto(ps []*entpb.User)")
}

// TestProtoGenerate_MapperUserFieldMappings verifies the field expressions in the mapper.
func TestProtoGenerate_MapperUserFieldMappings(t *testing.T) {
	data, err := os.ReadFile("internal/testdata/domain/pbmap/user_proto_gen.go")
	require.NoError(t, err)
	content := string(data)

	// ID conversion.
	assert.Contains(t, content, "int64(d.ID)", "ID should be converted to int64")
	assert.Contains(t, content, "int(p.Id)", "proto Id should be converted back to int")

	// Optional string bio: direct copy.
	assert.Contains(t, content, "Bio:")
	assert.True(t, strings.Contains(content, "d.Bio"), "Bio ToProto should use d.Bio")
	assert.True(t, strings.Contains(content, "p.Bio"), "Bio FromProto should use p.Bio")

	// Enum conversion via local (unexported) helper.
	assert.Contains(t, content, "userStatusToProto(d.Status)")
	assert.Contains(t, content, "userStatusFromProto(p.Status)")

	// Timestamp.
	assert.Contains(t, content, "timestamppb.New(d.CreatedAt)")
	assert.Contains(t, content, "p.CreatedAt.AsTime()")

	// Optional int score → exported helper (same pbmap package).
	assert.Contains(t, content, "ToInt64Ptr(d.Score)")
	assert.Contains(t, content, "FromInt64Ptr(p.Score)")

	// Edge IDs slice → exported helper.
	assert.Contains(t, content, "ToInt64Slice(d.PostIDs)")
	assert.Contains(t, content, "FromInt64Slice(p.PostIds)")

	// Virtual fields.
	assert.Contains(t, content, "d.FullName")
	assert.Contains(t, content, "d.IsPremium")
	assert.Contains(t, content, "timestamppb.New(d.ExpiresAt)", "expires_at with explicit ProtoType")

	// username must NOT be mapped (SkipProto).
	assert.False(t, strings.Contains(content, "Username"), "username should not appear in proto mapper")
}

// TestProtoGenerate_MapperPostMethods verifies domain/pbmap/post_proto_gen.go structure.
func TestProtoGenerate_MapperPostMethods(t *testing.T) {
	data, err := os.ReadFile("internal/testdata/domain/pbmap/post_proto_gen.go")
	require.NoError(t, err)
	content := string(data)

	assert.Contains(t, content, "package pbmap", "mapper must be in the pbmap package")
	assert.Contains(t, content, "func PostToProto(d *domain.Post)")
	assert.Contains(t, content, "func PostFromProto(p *entpb.Post)")
}

// TestProtoGenerate_HelpersFile verifies proto_helpers_gen.go is in the pbmap package
// with exported (PascalCase) function names.
func TestProtoGenerate_HelpersFile(t *testing.T) {
	data, err := os.ReadFile("internal/testdata/domain/pbmap/proto_helpers_gen.go")
	require.NoError(t, err)
	content := string(data)

	assert.Contains(t, content, "package pbmap", "helpers must be in the pbmap package")
	assert.Contains(t, content, "func ToInt64Slice(")
	assert.Contains(t, content, "func FromInt64Slice(")
	assert.Contains(t, content, "func ToInt64Ptr(")
	assert.Contains(t, content, "func FromInt64Ptr(")
	assert.Contains(t, content, "func ToUint64Ptr(")
	assert.Contains(t, content, "func FromUint64Ptr(")
	assert.Contains(t, content, "func TimeToTimestampProto(")
	assert.Contains(t, content, "func TimestampProtoToTime(")
	assert.Contains(t, content, "func DurationToDurationProto(")
	assert.Contains(t, content, "func DurationProtoToDuration(")
	assert.Contains(t, content, "func UuidPtrToProtoString(")
	assert.Contains(t, content, "func ProtoStringToUUIDPtr(")
}

// TestProtoGenerate_PbmapCompiles verifies the generated pbmap package compiles cleanly
// against the real protobuf-generated entpb types (from buf generate).
func TestProtoGenerate_PbmapCompiles(t *testing.T) {
	cmd := exec.Command("go", "build", "./internal/testdata/domain/pbmap/")
	out, err := cmd.CombinedOutput()
	assert.NoError(t, err, "pbmap package must compile:\n%s", out)
}

// TestProtoGenerate_EnumHelpers verifies the enum helper functions in user_proto_gen.go.
func TestProtoGenerate_EnumHelpers(t *testing.T) {
	data, err := os.ReadFile("internal/testdata/domain/pbmap/user_proto_gen.go")
	require.NoError(t, err)
	content := string(data)

	// Enum helpers use domain. prefix for domain types and entpb. prefix for proto types.
	assert.Contains(t, content, "func userStatusToProto(s domain.UserStatus) entpb.UserStatus {")
	assert.Contains(t, content, "func userStatusFromProto(s entpb.UserStatus) domain.UserStatus {")
	assert.Contains(t, content, "case domain.UserStatusActive:")
	assert.Contains(t, content, "return entpb.UserStatus_USER_STATUS_ACTIVE")
	assert.Contains(t, content, "case domain.UserStatusInactive:")
	assert.Contains(t, content, "return entpb.UserStatus_USER_STATUS_INACTIVE")
}

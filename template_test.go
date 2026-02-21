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
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// entDomainFile parses internal/testdata/ent/domain.go and returns the AST.
func entDomainFile(t *testing.T) *ast.File {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "internal/testdata/ent/domain.go", nil, parser.ParseComments)
	require.NoError(t, err)
	return f
}

// findFuncDecl returns the *ast.FuncDecl for the given receiver type and method name, or nil.
func findFuncDecl(f *ast.File, recv, method string) *ast.FuncDecl {
	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Name.Name != method {
			continue
		}
		if fd.Recv == nil || len(fd.Recv.List) == 0 {
			continue
		}
		if fieldTypeStr(fd.Recv.List[0].Type) == recv {
			return fd
		}
	}
	return nil
}

func TestTemplate_DomainFile_Imports(t *testing.T) {
	f := entDomainFile(t)

	assert.True(t, hasImport(f, "github.com/danhtran94/entdomain"), "must import entdomain for ApplyOption")
	assert.True(t, hasImport(f, "github.com/danhtran94/entdomain/internal/testdata/domain"), "must import domain package")
}

func TestTemplate_DomainField_Constants(t *testing.T) {
	f := entDomainFile(t)

	// User domain field constants
	assert.Equal(t, `"name"`, findConst(f, "UserDomainFieldName"))
	assert.Equal(t, `"bio"`, findConst(f, "UserDomainFieldBio"))
	assert.Equal(t, `"status"`, findConst(f, "UserDomainFieldStatus"))
	assert.Equal(t, `"created_at"`, findConst(f, "UserDomainFieldCreatedAt"))
	assert.Equal(t, `"username"`, findConst(f, "UserDomainFieldUsername"))
	assert.Equal(t, `"score"`, findConst(f, "UserDomainFieldScore"))
	assert.Equal(t, `"post_ids"`, findConst(f, "UserDomainFieldPostIDs"))

	// Post domain field constants
	assert.Equal(t, `"title"`, findConst(f, "PostDomainFieldTitle"))
	assert.Equal(t, `"published"`, findConst(f, "PostDomainFieldPublished"))
}

func TestTemplate_DomainTransformer_Struct(t *testing.T) {
	f := entDomainFile(t)

	t.Run("UserDomainTransformer has virtual field hooks", func(t *testing.T) {
		st := findStructType(f, "UserDomainTransformer")
		require.NotNil(t, st, "UserDomainTransformer struct must exist")

		fields := structFieldNames(st)
		assert.Contains(t, fields, "GetFullName")
		assert.Contains(t, fields, "SetFullNameOnCreate")
		assert.Contains(t, fields, "SetFullNameOnUpdate")
		assert.Contains(t, fields, "GetIsPremium")
		assert.Contains(t, fields, "SetIsPremiumOnCreate")
		assert.Contains(t, fields, "SetIsPremiumOnUpdate")
		assert.Contains(t, fields, "GetMetadata")
		assert.Contains(t, fields, "SetMetadataOnCreate")
		assert.Contains(t, fields, "SetMetadataOnUpdate")
	})

	t.Run("PostDomainTransformer is empty (no virtual fields)", func(t *testing.T) {
		st := findStructType(f, "PostDomainTransformer")
		require.NotNil(t, st, "PostDomainTransformer struct must exist")
		assert.Empty(t, structFieldNames(st))
	})
}

func TestTemplate_ToDomain_Methods(t *testing.T) {
	f := entDomainFile(t)

	t.Run("User.ToDomain exists", func(t *testing.T) {
		fd := findFuncDecl(f, "*User", "ToDomain")
		require.NotNil(t, fd, "(*User).ToDomain() must be declared")
	})

	t.Run("Post.ToDomain exists", func(t *testing.T) {
		fd := findFuncDecl(f, "*Post", "ToDomain")
		require.NotNil(t, fd, "(*Post).ToDomain() must be declared")
	})
}

func TestTemplate_ApplyDomain_Create(t *testing.T) {
	f := entDomainFile(t)

	t.Run("UserCreate.ApplyDomain exists", func(t *testing.T) {
		fd := findFuncDecl(f, "*UserCreate", "ApplyDomain")
		require.NotNil(t, fd, "(*UserCreate).ApplyDomain() must be declared")
	})

	t.Run("PostCreate.ApplyDomain exists", func(t *testing.T) {
		fd := findFuncDecl(f, "*PostCreate", "ApplyDomain")
		require.NotNil(t, fd, "(*PostCreate).ApplyDomain() must be declared")
	})
}

func TestTemplate_ApplyDomain_UpdateOne(t *testing.T) {
	f := entDomainFile(t)

	t.Run("UserUpdateOne.ApplyDomain exists", func(t *testing.T) {
		fd := findFuncDecl(f, "*UserUpdateOne", "ApplyDomain")
		require.NotNil(t, fd, "(*UserUpdateOne).ApplyDomain() must be declared")
	})

	t.Run("PostUpdateOne.ApplyDomain exists", func(t *testing.T) {
		fd := findFuncDecl(f, "*PostUpdateOne", "ApplyDomain")
		require.NotNil(t, fd, "(*PostUpdateOne).ApplyDomain() must be declared")
	})
}

func TestTemplate_ApplyDomain_Update(t *testing.T) {
	f := entDomainFile(t)

	t.Run("UserUpdate.ApplyDomain exists", func(t *testing.T) {
		fd := findFuncDecl(f, "*UserUpdate", "ApplyDomain")
		require.NotNil(t, fd, "(*UserUpdate).ApplyDomain() must be declared")
	})

	t.Run("PostUpdate.ApplyDomain exists", func(t *testing.T) {
		fd := findFuncDecl(f, "*PostUpdate", "ApplyDomain")
		require.NotNil(t, fd, "(*PostUpdate).ApplyDomain() must be declared")
	})

}

func TestTemplate_SliceToDomain(t *testing.T) {
	f := entDomainFile(t)

	t.Run("Users.ToDomain exists", func(t *testing.T) {
		fd := findFuncDecl(f, "Users", "ToDomain")
		require.NotNil(t, fd, "(Users).ToDomain() must be declared")
	})

	t.Run("Posts.ToDomain absent (NoBulk)", func(t *testing.T) {
		fd := findFuncDecl(f, "Posts", "ToDomain")
		assert.Nil(t, fd, "(Posts).ToDomain() must not be generated when NoBulk() is set")
	})
}

func TestTemplate_CreateBulkDomain(t *testing.T) {
	f := entDomainFile(t)

	t.Run("UserClient.CreateBulkDomain exists", func(t *testing.T) {
		fd := findFuncDecl(f, "*UserClient", "CreateBulkDomain")
		require.NotNil(t, fd, "(*UserClient).CreateBulkDomain() must be declared")
	})

	t.Run("PostClient.CreateBulkDomain absent (NoBulk)", func(t *testing.T) {
		fd := findFuncDecl(f, "*PostClient", "CreateBulkDomain")
		assert.Nil(t, fd, "(*PostClient).CreateBulkDomain() must not be generated when NoBulk() is set")
	})
}

func TestTemplate_UpdateBulkDomain(t *testing.T) {
	f := entDomainFile(t)

	t.Run("UserClient.UpdateBulkDomain exists", func(t *testing.T) {
		fd := findFuncDecl(f, "*UserClient", "UpdateBulkDomain")
		require.NotNil(t, fd, "(*UserClient).UpdateBulkDomain() must be declared")
	})

	t.Run("PostClient.UpdateBulkDomain absent (NoBulk)", func(t *testing.T) {
		fd := findFuncDecl(f, "*PostClient", "UpdateBulkDomain")
		assert.Nil(t, fd, "(*PostClient).UpdateBulkDomain() must not be generated when NoBulk() is set")
	})

	t.Run("UserUpdateOneBulk.Save exists", func(t *testing.T) {
		fd := findFuncDecl(f, "*UserUpdateOneBulk", "Save")
		require.NotNil(t, fd, "(*UserUpdateOneBulk).Save() must be declared")
	})

	t.Run("UserUpdateOneBulk.SaveX exists", func(t *testing.T) {
		fd := findFuncDecl(f, "*UserUpdateOneBulk", "SaveX")
		require.NotNil(t, fd, "(*UserUpdateOneBulk).SaveX() must be declared")
	})

	t.Run("UserUpdateOneBulk.Exec exists", func(t *testing.T) {
		fd := findFuncDecl(f, "*UserUpdateOneBulk", "Exec")
		require.NotNil(t, fd, "(*UserUpdateOneBulk).Exec() must be declared")
	})

	t.Run("UserUpdateOneBulk.ExecX exists", func(t *testing.T) {
		fd := findFuncDecl(f, "*UserUpdateOneBulk", "ExecX")
		require.NotNil(t, fd, "(*UserUpdateOneBulk).ExecX() must be declared")
	})
}

func TestTemplate_TransformerVar(t *testing.T) {
	f := entDomainFile(t)

	// Verify package-level transformer vars exist.
	findVar := func(name string) bool {
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				continue
			}
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for _, n := range vs.Names {
					if n.Name == name {
						return true
					}
				}
			}
		}
		return false
	}

	assert.True(t, findVar("UserTransformer"), "UserTransformer var must be declared")
	assert.True(t, findVar("PostTransformer"), "PostTransformer var must be declared")
}

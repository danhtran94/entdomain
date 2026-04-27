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
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// entDomainFile parses examples/basic/ent/domain.go and returns the AST.
func entDomainFile(t *testing.T) *ast.File {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "examples/basic/ent/domain.go", nil, parser.ParseComments)
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
	assert.True(t, hasImport(f, "github.com/danhtran94/entdomain/examples/basic/domain"), "must import domain package")
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
		// metadata is now a regular ent field (not virtual) → no transformer hooks.
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

	t.Run("Posts.ToDomain exists (always generated)", func(t *testing.T) {
		fd := findFuncDecl(f, "Posts", "ToDomain")
		require.NotNil(t, fd, "(Posts).ToDomain() must always be generated")
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

func TestTemplate_ApplyDomain_Upsert(t *testing.T) {
	f := entDomainFile(t)

	t.Run("UserUpsertOne.ApplyDomain exists", func(t *testing.T) {
		fd := findFuncDecl(f, "*UserUpsertOne", "ApplyDomain")
		require.NotNil(t, fd, "(*UserUpsertOne).ApplyDomain() must be declared")
	})

	t.Run("UserUpsertBulk.ApplyDomain exists (User is not NoBulk)", func(t *testing.T) {
		fd := findFuncDecl(f, "*UserUpsertBulk", "ApplyDomain")
		require.NotNil(t, fd, "(*UserUpsertBulk).ApplyDomain() must be declared")
	})

	t.Run("PostUpsertBulk.ApplyDomain absent (Post has NoBulk)", func(t *testing.T) {
		fd := findFuncDecl(f, "*PostUpsertBulk", "ApplyDomain")
		assert.Nil(t, fd, "(*PostUpsertBulk).ApplyDomain() must not be generated when NoBulk() is set")
	})

	t.Run("nillable fields use && nil guard, not SetNillable", func(t *testing.T) {
		body := methodBodySource(t, "*UserUpsertOne", "ApplyDomain")
		require.NotEmpty(t, body)
		assert.Contains(t, body, "d.Bio != nil", "bio must use nil guard in upsert")
		assert.Contains(t, body, "d.Score != nil", "score must use nil guard in upsert")
		assert.NotContains(t, body, "SetNillable", "upsert must not use SetNillable*")
	})

	t.Run("immutable field CreatedAt absent from upsert body", func(t *testing.T) {
		body := methodBodySource(t, "*UserUpsertOne", "ApplyDomain")
		require.NotEmpty(t, body)
		assert.NotContains(t, body, "SetCreatedAt", "immutable field must be absent from upsert body")
	})

	t.Run("edge IDs absent from upsert body", func(t *testing.T) {
		body := methodBodySource(t, "*UserUpsertOne", "ApplyDomain")
		require.NotEmpty(t, body)
		assert.NotContains(t, body, "PostIDs", "edge IDs must be absent from upsert body")
		assert.NotContains(t, body, "TagIDs", "edge IDs must be absent from upsert body")
	})

	t.Run("virtual field transformer absent from upsert body", func(t *testing.T) {
		body := methodBodySource(t, "*UserUpsertOne", "ApplyDomain")
		require.NotEmpty(t, body)
		assert.NotContains(t, body, "Transformer", "virtual field transformer must be absent from upsert body")
	})
}

// entFIQLFile parses examples/basic/ent/fiql.go and returns the AST.
func entFIQLFile(t *testing.T) *ast.File {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "examples/basic/ent/fiql.go", nil, parser.ParseComments)
	require.NoError(t, err)
	return f
}

// findVar reports whether a package-level var with the given name exists in the file.
func findVarDecl(f *ast.File, name string) bool {
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

// findFuncDeclTop finds a top-level (non-method) func by name.
func findFuncDeclTop(f *ast.File, name string) *ast.FuncDecl {
	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Recv != nil {
			continue
		}
		if fd.Name.Name == name {
			return fd
		}
	}
	return nil
}

func TestTemplate_FIQL_Generated(t *testing.T) {
	f := entFIQLFile(t)

	t.Run("imports predicate and user sub-packages", func(t *testing.T) {
		assert.True(t, hasImport(f, "github.com/danhtran94/entdomain"), "must import entdomain")
		assert.True(t, hasImport(f, "github.com/danhtran94/entdomain/examples/basic/ent/predicate"), "must import predicate sub-package")
		assert.True(t, hasImport(f, "github.com/danhtran94/entdomain/examples/basic/ent/user"), "must import user sub-package for field predicates")
	})

	t.Run("UserFIQLFields var declared", func(t *testing.T) {
		assert.True(t, findVarDecl(f, "UserFIQLFields"), "UserFIQLFields var must be declared")
	})

	t.Run("UserFIQL function declared", func(t *testing.T) {
		fd := findFuncDeclTop(f, "UserFIQL")
		require.NotNil(t, fd, "UserFIQL function must be declared")
	})

	t.Run("name field uses FIQLString with EQ, NEQ, Contains", func(t *testing.T) {
		src := fileSource(t, "examples/basic/ent/fiql.go")
		assert.Contains(t, src, `"name": entdomain.FIQLString[predicate.User]`)
		assert.Contains(t, src, "user.NameEQ")
		assert.Contains(t, src, "user.NameNEQ")
		assert.Contains(t, src, "user.NameContains")
		assert.NotContains(t, src, "user.NameGT", "GT not annotated on name field")
	})

	t.Run("score field uses FIQLInt with all 5 ops", func(t *testing.T) {
		src := fileSource(t, "examples/basic/ent/fiql.go")
		assert.Contains(t, src, `"score": entdomain.FIQLInt[predicate.User]`)
		assert.Contains(t, src, "user.ScoreEQ")
		assert.Contains(t, src, "user.ScoreGT")
		assert.Contains(t, src, "user.ScoreLT")
		assert.Contains(t, src, "user.ScoreGTE")
		assert.Contains(t, src, "user.ScoreLTE")
	})

	t.Run("status field uses FIQLEnum with pre-built maps", func(t *testing.T) {
		src := fileSource(t, "examples/basic/ent/fiql.go")
		assert.Contains(t, src, `"status": entdomain.FIQLEnum[predicate.User]`)
		assert.Contains(t, src, `"active":`)
		assert.Contains(t, src, `"inactive":`)
		assert.Contains(t, src, "user.StatusEQ(user.StatusActive)")
		assert.Contains(t, src, "user.StatusNEQ(user.StatusActive)")
	})

	t.Run("created_at field uses FIQLTime with GTE and LTE only", func(t *testing.T) {
		src := fileSource(t, "examples/basic/ent/fiql.go")
		assert.Contains(t, src, `"created_at": entdomain.FIQLTime[predicate.User]`)
		assert.Contains(t, src, "user.CreatedAtGTE")
		assert.Contains(t, src, "user.CreatedAtLTE")
		assert.NotContains(t, src, "user.CreatedAtEQ", "EQ not annotated on created_at")
	})

	t.Run("password_hash not in registry (unannotated)", func(t *testing.T) {
		src := fileSource(t, "examples/basic/ent/fiql.go")
		assert.NotContains(t, src, `"password_hash"`, "unannotated fields must not appear in FIQL registry")
	})

	t.Run("bio in registry with IsNil/NotNil only (null-handling annotation)", func(t *testing.T) {
		// Updated for ENTD-004: bio gained IsNull/NotNull annotation. The
		// generated entry should contain only those slots, not EQ/NEQ/etc.
		// Token-level checks rather than full lines — gofmt may realign the
		// colon spacing as siblings change length.
		src := fileSource(t, "examples/basic/ent/fiql.go")
		assert.Contains(t, src, `"bio": entdomain.FIQLString[predicate.User]{`)
		assert.Contains(t, src, "IsNil:")
		assert.Contains(t, src, "user.BioIsNil")
		assert.Contains(t, src, "NotNil:")
		assert.Contains(t, src, "user.BioNotNil")
	})
}

// fileSource reads the raw source text of a file.
func fileSource(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
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

// X-variants (panicking wrappers that mirror ent's SaveX/FirstX convention)
// must be generated alongside every error-returning ApplyDomain* / BulkDomain.
// They exist to restore fluent chaining for tests and scripts where callers
// opt into panic semantics explicitly via the X suffix.

func TestTemplate_ApplyDomainX_AllVariants(t *testing.T) {
	f := entDomainFile(t)

	cases := []struct {
		recv   string
		method string
	}{
		{"*UserCreate", "ApplyDomainX"},
		{"*UserUpdateOne", "ApplyDomainX"},
		{"*UserUpdate", "ApplyDomainX"},
		{"*UserUpsertOne", "ApplyDomainX"},
		{"*UserUpsertBulk", "ApplyDomainX"},
		{"*PostCreate", "ApplyDomainX"},
		{"*PostUpdateOne", "ApplyDomainX"},
		{"*PostUpdate", "ApplyDomainX"},
		{"*PostUpsertOne", "ApplyDomainX"},
		// Post is NoBulk — no UpsertBulk variant.
	}
	for _, c := range cases {
		t.Run(c.recv+"."+c.method, func(t *testing.T) {
			fd := findFuncDecl(f, c.recv, c.method)
			require.NotNil(t, fd, "(%s).%s must be declared as the panicking variant", c.recv, c.method)
		})
	}

	// Negative assertion: Post is configured with NoBulk, so the base
	// *PostUpsertBulk.ApplyDomain is suppressed — and the X-variant must be
	// symmetrically suppressed. If this ever fails, the codegen has leaked
	// an X-variant past the NoBulk gate.
	t.Run("*PostUpsertBulk.ApplyDomainX absent (NoBulk)", func(t *testing.T) {
		fd := findFuncDecl(f, "*PostUpsertBulk", "ApplyDomainX")
		require.Nil(t, fd, "(*PostUpsertBulk).ApplyDomainX must not be generated when NoBulk is set")
	})
}

func TestTemplate_BulkDomainX(t *testing.T) {
	f := entDomainFile(t)

	t.Run("UserClient.CreateBulkDomainX exists", func(t *testing.T) {
		fd := findFuncDecl(f, "*UserClient", "CreateBulkDomainX")
		require.NotNil(t, fd, "panicking bulk create must be generated when NoBulk is unset")
	})

	t.Run("UserClient.UpdateBulkDomainX exists", func(t *testing.T) {
		fd := findFuncDecl(f, "*UserClient", "UpdateBulkDomainX")
		require.NotNil(t, fd, "panicking bulk update must be generated when NoBulk is unset")
	})

	t.Run("PostClient.CreateBulkDomainX absent (NoBulk)", func(t *testing.T) {
		fd := findFuncDecl(f, "*PostClient", "CreateBulkDomainX")
		assert.Nil(t, fd, "X-variant must not be generated when the base method is suppressed by NoBulk")
	})

	t.Run("PostClient.UpdateBulkDomainX absent (NoBulk)", func(t *testing.T) {
		fd := findFuncDecl(f, "*PostClient", "UpdateBulkDomainX")
		assert.Nil(t, fd, "X-variant must not be generated when the base method is suppressed by NoBulk")
	})
}

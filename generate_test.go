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

// domainFile parses the given testdata domain file and returns the AST file.
func domainFile(t *testing.T, filename string) *ast.File {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	require.NoError(t, err, "parsing %s", filename)
	return f
}

// findStructType returns the *ast.StructType for the named type in the file, or nil.
func findStructType(f *ast.File, name string) *ast.StructType {
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok || ts.Name.Name != name {
				continue
			}
			if st, ok := ts.Type.(*ast.StructType); ok {
				return st
			}
		}
	}
	return nil
}

// structFieldNames returns the field names in the struct.
func structFieldNames(st *ast.StructType) []string {
	var names []string
	for _, f := range st.Fields.List {
		for _, n := range f.Names {
			names = append(names, n.Name)
		}
	}
	return names
}

// fieldTypeStr returns a string representation of the field type expr.
func fieldTypeStr(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + fieldTypeStr(t.X)
	case *ast.SelectorExpr:
		return fieldTypeStr(t.X) + "." + t.Sel.Name
	case *ast.ArrayType:
		return "[]" + fieldTypeStr(t.Elt)
	case *ast.MapType:
		return "map[" + fieldTypeStr(t.Key) + "]" + fieldTypeStr(t.Value)
	case *ast.InterfaceType:
		return "interface{}"
	default:
		return "?"
	}
}

// findField returns the type string of the named field in the struct, or "".
func findField(st *ast.StructType, name string) string {
	for _, f := range st.Fields.List {
		for _, n := range f.Names {
			if n.Name == name {
				return fieldTypeStr(f.Type)
			}
		}
	}
	return ""
}

// findConst returns the value of the named const in the file, or "".
func findConst(f *ast.File, name string) string {
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, n := range vs.Names {
				if n.Name == name && i < len(vs.Values) {
					if bl, ok := vs.Values[i].(*ast.BasicLit); ok {
						return bl.Value
					}
				}
			}
		}
	}
	return ""
}

// hasImport reports whether the file imports the given path.
func hasImport(f *ast.File, path string) bool {
	for _, imp := range f.Imports {
		if imp.Path.Value == `"`+path+`"` {
			return true
		}
	}
	return false
}

func TestGenerate_UserDomainStruct(t *testing.T) {
	f := domainFile(t, "internal/testdata/domain/user_gen.go")

	t.Run("no ent import", func(t *testing.T) {
		assert.False(t, hasImport(f, "entgo.io/ent"), "domain file must not import entgo.io/ent")
	})

	t.Run("time import present", func(t *testing.T) {
		assert.True(t, hasImport(f, "time"), "domain file must import time for time.Time fields")
	})

	t.Run("User struct fields", func(t *testing.T) {
		st := findStructType(f, "User")
		require.NotNil(t, st, "User struct must be declared")

		assert.Equal(t, "int", findField(st, "ID"))
		assert.Equal(t, "string", findField(st, "Name"))
		assert.Equal(t, "*string", findField(st, "Bio"))
		assert.Equal(t, "UserStatus", findField(st, "Status"))
		assert.Equal(t, "time.Time", findField(st, "CreatedAt"))
		assert.Equal(t, "string", findField(st, "Username"))
		assert.Equal(t, "*int", findField(st, "Score"))
		// plural To: IDs + Nest
		assert.Equal(t, "[]int", findField(st, "PostIDs"))
		assert.Equal(t, "PostList", findField(st, "Posts"))
		// singular To: IDs + Nest → pointer type (may not be loaded)
		assert.Equal(t, "*Post", findField(st, "PinnedPost"))
		// Virtual fields
		assert.Equal(t, "string", findField(st, "FullName"))
		assert.Equal(t, "bool", findField(st, "IsPremium"))
		assert.Equal(t, "map[string]any", findField(st, "Metadata"))
	})

	t.Run("UserStatus enum type", func(t *testing.T) {
		// Should have a UserStatus type alias for string
		found := false
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				if ts.Name.Name == "UserStatus" {
					found = true
				}
			}
		}
		assert.True(t, found, "UserStatus type must be declared")
	})

	t.Run("UserStatus enum constants", func(t *testing.T) {
		assert.Equal(t, `"active"`, findConst(f, "UserStatusActive"))
		assert.Equal(t, `"inactive"`, findConst(f, "UserStatusInactive"))
	})
}

func TestGenerate_ListType(t *testing.T) {
	t.Run("UserList declared in user.go", func(t *testing.T) {
		f := domainFile(t, "internal/testdata/domain/user_gen.go")
		found := false
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if ok && ts.Name.Name == "UserList" {
					found = true
				}
			}
		}
		assert.True(t, found, "UserList type must be declared")
	})

	t.Run("User.ToList declared in user.go", func(t *testing.T) {
		f := domainFile(t, "internal/testdata/domain/user_gen.go")
		fd := findFuncDecl(f, "*User", "ToList")
		require.NotNil(t, fd, "(*User).ToList() must be declared")
	})

	t.Run("UserList.GetIDs declared in user.go", func(t *testing.T) {
		f := domainFile(t, "internal/testdata/domain/user_gen.go")
		fd := findFuncDecl(f, "UserList", "GetIDs")
		require.NotNil(t, fd, "(UserList).GetIDs() must be declared")
	})

	t.Run("PostList declared in post.go (always, NoBulk only gates operators)", func(t *testing.T) {
		f := domainFile(t, "internal/testdata/domain/post_gen.go")
		found := false
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if ok && ts.Name.Name == "PostList" {
					found = true
				}
			}
		}
		assert.True(t, found, "PostList type must always be declared regardless of NoBulk")
	})

	t.Run("PostList.GetIDs declared in post.go", func(t *testing.T) {
		f := domainFile(t, "internal/testdata/domain/post_gen.go")
		fd := findFuncDecl(f, "PostList", "GetIDs")
		require.NotNil(t, fd, "(PostList).GetIDs() must be declared")
	})
}

func TestGenerate_PostDomainStruct(t *testing.T) {
	f := domainFile(t, "internal/testdata/domain/post_gen.go")

	t.Run("no ent import", func(t *testing.T) {
		assert.False(t, hasImport(f, "entgo.io/ent"))
	})

	t.Run("Post struct fields", func(t *testing.T) {
		st := findStructType(f, "Post")
		require.NotNil(t, st, "Post struct must be declared")

		assert.Equal(t, "int", findField(st, "ID"))
		assert.Equal(t, "string", findField(st, "Title"))
		assert.Equal(t, "bool", findField(st, "Published"))
		// singular From: IDs
		assert.Equal(t, "int", findField(st, "OwnerID"))
		// plural From: IDs
		assert.Equal(t, "[]int", findField(st, "PinnerIDs"))
	})
}

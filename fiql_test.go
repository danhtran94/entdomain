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
	"fmt"
	"strings"
	"testing"

	"entgo.io/ent/dialect/sql"
	"github.com/danhtran94/entdomain"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testPred is a predicate type for testing — satisfies the Predicate constraint.
type testPred func(*sql.Selector)

// sqlPred produces a testPred that writes a real SQL WHERE condition, letting us
// inspect the generated query to verify AND/OR structure.
func sqlPred(field, value string) testPred {
	return testPred(func(s *sql.Selector) { s.Where(sql.EQ(field, value)) })
}

// buildSQL returns the WHERE fragment produced by applying pred to a fresh selector.
func buildSQL(pred testPred) string {
	s := sql.Dialect("sqlite3").Select("*").From(sql.Table("t"))
	pred(s)
	q, _ := s.Query()
	return q
}

// newEQPred returns a testPred that records its call in the given string pointer.
func newEQPred(tag string, record *[]string) func(string) testPred {
	return func(v string) testPred {
		return testPred(func(s *sql.Selector) {
			*record = append(*record, fmt.Sprintf("%s==%s", tag, v))
		})
	}
}

func newIntPred(tag string, op string, record *[]string) func(int) testPred {
	return func(v int) testPred {
		return testPred(func(s *sql.Selector) {
			*record = append(*record, fmt.Sprintf("%s%s%d", tag, op, v))
		})
	}
}

func TestParseFIQL_StringField_EQ(t *testing.T) {
	var calls []string
	fields := entdomain.FIQLFields[testPred]{
		"name": entdomain.FIQLString[testPred]{
			EQ:  newEQPred("name", &calls),
			NEQ: func(v string) testPred { return func(s *sql.Selector) { calls = append(calls, "name!="+v) } },
		},
	}

	pred, err := entdomain.ParseFIQL("name==alice", fields)
	require.NoError(t, err)

	pred(nil) // call to record
	assert.Equal(t, []string{"name==alice"}, calls)
}

func TestParseFIQL_ANDExpression(t *testing.T) {
	var calls []string
	record := func(tag, val string) testPred {
		return testPred(func(s *sql.Selector) { calls = append(calls, tag+"=="+val) })
	}
	fields := entdomain.FIQLFields[testPred]{
		"name": entdomain.FIQLString[testPred]{EQ: func(v string) testPred { return record("name", v) }},
		"city": entdomain.FIQLString[testPred]{EQ: func(v string) testPred { return record("city", v) }},
	}

	// ';' = AND
	pred, err := entdomain.ParseFIQL("name==alice;city==london", fields)
	require.NoError(t, err)

	// Run the combined predicate on a real selector to trigger both leaves.
	db, _ := sql.Open("sqlite3", ":memory:")
	s := sql.Dialect("sqlite3").Select("*").From(sql.Table("users"))
	pred(s)
	_ = db

	// Both leaf predicates must have been recorded.
	assert.Contains(t, calls, "name==alice")
	assert.Contains(t, calls, "city==london")
}

func TestParseFIQL_ORExpression(t *testing.T) {
	var calls []string
	record := func(tag, val string) testPred {
		return testPred(func(s *sql.Selector) { calls = append(calls, tag+"=="+val) })
	}
	fields := entdomain.FIQLFields[testPred]{
		"status": entdomain.FIQLString[testPred]{EQ: func(v string) testPred { return record("status", v) }},
	}

	// ',' = OR: both branches must be wired
	pred, err := entdomain.ParseFIQL("status==active,status==pending", fields)
	require.NoError(t, err)

	s := sql.Dialect("sqlite3").Select("*").From(sql.Table("t"))
	pred(s)

	assert.Contains(t, calls, "status==active")
	assert.Contains(t, calls, "status==pending")
}

func TestParseFIQL_Grouping(t *testing.T) {
	var calls []string
	record := func(tag, val string) testPred {
		return testPred(func(s *sql.Selector) { calls = append(calls, tag+"=="+val) })
	}
	fields := entdomain.FIQLFields[testPred]{
		"a": entdomain.FIQLString[testPred]{EQ: func(v string) testPred { return record("a", v) }},
		"b": entdomain.FIQLString[testPred]{EQ: func(v string) testPred { return record("b", v) }},
		"c": entdomain.FIQLString[testPred]{EQ: func(v string) testPred { return record("c", v) }},
	}

	// (a==1;b==2),c==3 — AND is higher precedence inside group, OR at top level
	pred, err := entdomain.ParseFIQL("(a==1;b==2),c==3", fields)
	require.NoError(t, err)

	s := sql.Dialect("sqlite3").Select("*").From(sql.Table("t"))
	pred(s)

	assert.Contains(t, calls, "a==1")
	assert.Contains(t, calls, "b==2")
	assert.Contains(t, calls, "c==3")
}

func TestParseFIQL_IntField_AllOps(t *testing.T) {
	var calls []string
	rec := func(op string) func(int) testPred {
		return newIntPred("age", op, &calls)
	}
	fields := entdomain.FIQLFields[testPred]{
		"age": entdomain.FIQLInt[testPred]{
			EQ: rec("=="), NEQ: rec("!="),
			GT: rec(">"), LT: rec("<"),
			GTE: rec(">="), LTE: rec("<="),
		},
	}

	cases := []struct {
		expr   string
		expect string
	}{
		{"age==25", "age==25"},
		{"age!=25", "age!=25"},
		{"age=gt=25", "age>25"},
		{"age=lt=25", "age<25"},
		{"age=ge=25", "age>=25"},
		{"age=le=25", "age<=25"},
	}

	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			calls = nil
			pred, err := entdomain.ParseFIQL(tc.expr, fields)
			require.NoError(t, err)
			pred(nil)
			assert.Equal(t, []string{tc.expect}, calls)
		})
	}
}

func TestParseFIQL_EnumField(t *testing.T) {
	active := testPred(func(s *sql.Selector) {})
	inactive := testPred(func(s *sql.Selector) {})

	fields := entdomain.FIQLFields[testPred]{
		"status": entdomain.FIQLEnum[testPred]{
			EQ:  map[string]testPred{"active": active, "inactive": inactive},
			NEQ: map[string]testPred{"active": active, "inactive": inactive},
		},
	}

	t.Run("valid enum EQ", func(t *testing.T) {
		_, err := entdomain.ParseFIQL("status==active", fields)
		assert.NoError(t, err)
	})

	t.Run("valid enum NEQ", func(t *testing.T) {
		_, err := entdomain.ParseFIQL("status!=inactive", fields)
		assert.NoError(t, err)
	})

	t.Run("unknown enum value returns error", func(t *testing.T) {
		_, err := entdomain.ParseFIQL("status==pending", fields)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown enum value")
		assert.Contains(t, err.Error(), "pending")
	})

	t.Run("disallowed operator on enum returns error", func(t *testing.T) {
		_, err := entdomain.ParseFIQL("status=gt=active", fields)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not allowed on enum field")
	})
}

func TestParseFIQL_FloatField(t *testing.T) {
	var calls []string
	rec := func(op string) func(float64) testPred {
		return func(v float64) testPred {
			return testPred(func(s *sql.Selector) {
				calls = append(calls, fmt.Sprintf("price%s%v", op, v))
			})
		}
	}
	fields := entdomain.FIQLFields[testPred]{
		"price": entdomain.FIQLFloat[testPred]{
			EQ: rec("=="), GT: rec(">"), LTE: rec("<="),
		},
	}

	cases := []struct{ expr, want string }{
		{"price==9.99", "price==9.99"},
		{"price=gt=5.0", "price>5"},
		{"price=le=100.5", "price<=100.5"},
	}
	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			calls = nil
			pred, err := entdomain.ParseFIQL(tc.expr, fields)
			require.NoError(t, err)
			pred(nil)
			assert.Equal(t, []string{tc.want}, calls)
		})
	}
}

func TestParseFIQL_BoolField(t *testing.T) {
	var called bool
	fields := entdomain.FIQLFields[testPred]{
		"active": entdomain.FIQLBool[testPred]{
			EQ: func(v bool) testPred {
				return testPred(func(s *sql.Selector) { called = v })
			},
		},
	}

	t.Run("true", func(t *testing.T) {
		called = false
		pred, err := entdomain.ParseFIQL("active==true", fields)
		require.NoError(t, err)
		pred(nil)
		assert.True(t, called)
	})

	t.Run("false", func(t *testing.T) {
		called = true
		pred, err := entdomain.ParseFIQL("active==false", fields)
		require.NoError(t, err)
		pred(nil)
		assert.False(t, called)
	})

	t.Run("invalid bool value", func(t *testing.T) {
		_, err := entdomain.ParseFIQL("active==yes", fields)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid bool value")
	})

	t.Run("non-EQ operator rejected", func(t *testing.T) {
		_, err := entdomain.ParseFIQL("active=gt=true", fields)
		require.Error(t, err)
	})
}

func TestParseFIQL_UUIDField(t *testing.T) {
	canonical := "550e8400-e29b-41d4-a716-446655440000"
	expected := uuid.MustParse(canonical)

	var gotEQ, gotNEQ uuid.UUID
	fields := entdomain.FIQLFields[testPred]{
		"external_id": entdomain.FIQLUUID[testPred]{
			EQ:  func(v uuid.UUID) testPred { return testPred(func(s *sql.Selector) { gotEQ = v }) },
			NEQ: func(v uuid.UUID) testPred { return testPred(func(s *sql.Selector) { gotNEQ = v }) },
		},
	}

	t.Run("EQ canonical", func(t *testing.T) {
		gotEQ = uuid.Nil
		pred, err := entdomain.ParseFIQL("external_id=="+canonical, fields)
		require.NoError(t, err)
		pred(nil)
		assert.Equal(t, expected, gotEQ)
	})

	t.Run("NEQ canonical", func(t *testing.T) {
		gotNEQ = uuid.Nil
		pred, err := entdomain.ParseFIQL("external_id!="+canonical, fields)
		require.NoError(t, err)
		pred(nil)
		assert.Equal(t, expected, gotNEQ)
	})

	t.Run("invalid UUID value", func(t *testing.T) {
		_, err := entdomain.ParseFIQL("external_id==not-a-uuid", fields)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid UUID value")
	})

	t.Run("ordering operator rejected", func(t *testing.T) {
		_, err := entdomain.ParseFIQL("external_id=gt="+canonical, fields)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "only == and != are supported")
	})

	t.Run("substring operator rejected", func(t *testing.T) {
		_, err := entdomain.ParseFIQL("external_id=like="+canonical, fields)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "only == and != are supported")
	})
}

func TestParseFIQL_NullHandling(t *testing.T) {
	var calls []string
	mkPred := func(tag string) testPred {
		return testPred(func(s *sql.Selector) { calls = append(calls, tag) })
	}
	fields := entdomain.FIQLFields[testPred]{
		"bio": entdomain.FIQLString[testPred]{
			IsNil:  func() testPred { return mkPred("bio:isnil") },
			NotNil: func() testPred { return mkPred("bio:notnil") },
		},
	}

	t.Run("=is=null routes to IsNil", func(t *testing.T) {
		calls = nil
		pred, err := entdomain.ParseFIQL("bio=is=null", fields)
		require.NoError(t, err)
		pred(nil)
		assert.Equal(t, []string{"bio:isnil"}, calls)
	})

	t.Run("=is=notnull routes to NotNil", func(t *testing.T) {
		calls = nil
		pred, err := entdomain.ParseFIQL("bio=is=notnull", fields)
		require.NoError(t, err)
		pred(nil)
		assert.Equal(t, []string{"bio:notnil"}, calls)
	})

	t.Run("=is=maybe rejected with valid-values hint", func(t *testing.T) {
		_, err := entdomain.ParseFIQL("bio=is=maybe", fields)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `unknown =is= value "maybe"`)
		assert.Contains(t, err.Error(), "valid: null, notnull")
	})

	t.Run("=is= alias rejected (no nil/empty/present)", func(t *testing.T) {
		for _, alias := range []string{"nil", "empty", "present"} {
			_, err := entdomain.ParseFIQL("bio=is="+alias, fields)
			require.Error(t, err, alias)
			assert.Contains(t, err.Error(), "valid: null, notnull")
		}
	})

	t.Run("nil IsNil function returns not-allowed error", func(t *testing.T) {
		bareFields := entdomain.FIQLFields[testPred]{
			"bio": entdomain.FIQLString[testPred]{
				NotNil: func() testPred { return mkPred("bio:notnil") },
			},
		}
		_, err := entdomain.ParseFIQL("bio=is=null", bareFields)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "operator =is=null not allowed on this string field")
	})

	t.Run("composes with AND/OR", func(t *testing.T) {
		calls = nil
		pred, err := entdomain.ParseFIQL("bio=is=null,bio=is=notnull", fields)
		require.NoError(t, err)
		pred(sql.Dialect("sqlite3").Select("*").From(sql.Table("t")))
		assert.ElementsMatch(t, []string{"bio:isnil", "bio:notnil"}, calls)
	})
}

func TestParseFIQL_StringIn(t *testing.T) {
	var got []string
	fields := entdomain.FIQLFields[testPred]{
		"name": entdomain.FIQLString[testPred]{
			In: func(vs ...string) testPred {
				return testPred(func(s *sql.Selector) { got = append([]string(nil), vs...) })
			},
			NotIn: func(vs ...string) testPred {
				return testPred(func(s *sql.Selector) { got = append([]string{"not"}, vs...) })
			},
		},
	}

	t.Run("=in= happy path", func(t *testing.T) {
		got = nil
		pred, err := entdomain.ParseFIQL("name=in=(alice,bob,carol)", fields)
		require.NoError(t, err)
		pred(nil)
		assert.Equal(t, []string{"alice", "bob", "carol"}, got)
	})

	t.Run("=out= happy path", func(t *testing.T) {
		got = nil
		pred, err := entdomain.ParseFIQL("name=out=(alice,bob)", fields)
		require.NoError(t, err)
		pred(nil)
		assert.Equal(t, []string{"not", "alice", "bob"}, got)
	})

	t.Run("missing opening paren", func(t *testing.T) {
		_, err := entdomain.ParseFIQL("name=in=alice,bob", fields)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "expected '('")
	})

	t.Run("unterminated list", func(t *testing.T) {
		_, err := entdomain.ParseFIQL("name=in=(alice,bob", fields)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unterminated list")
	})

	t.Run("empty list rejected", func(t *testing.T) {
		_, err := entdomain.ParseFIQL("name=in=()", fields)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty value list")
	})

	t.Run("cap exceeded", func(t *testing.T) {
		vs := make([]string, 0, 101)
		for i := 0; i <= 100; i++ {
			vs = append(vs, fmt.Sprintf("v%d", i))
		}
		expr := "name=in=(" + strings.Join(vs, ",") + ")"
		_, err := entdomain.ParseFIQL(expr, fields)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds maximum of 100")
	})
}

func TestParseFIQL_IntIn(t *testing.T) {
	var got []int
	fields := entdomain.FIQLFields[testPred]{
		"score": entdomain.FIQLInt[testPred]{
			In: func(vs ...int) testPred { return testPred(func(s *sql.Selector) { got = append([]int(nil), vs...) }) },
		},
	}

	t.Run("happy path", func(t *testing.T) {
		got = nil
		pred, err := entdomain.ParseFIQL("score=in=(10,20,30)", fields)
		require.NoError(t, err)
		pred(nil)
		assert.Equal(t, []int{10, 20, 30}, got)
	})

	t.Run("invalid element", func(t *testing.T) {
		_, err := entdomain.ParseFIQL("score=in=(10,abc,30)", fields)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `invalid integer value "abc" in list`)
	})
}

func TestParseFIQL_FloatIn(t *testing.T) {
	var got []float64
	fields := entdomain.FIQLFields[testPred]{
		"ratio": entdomain.FIQLFloat[testPred]{
			In: func(vs ...float64) testPred {
				return testPred(func(s *sql.Selector) { got = append([]float64(nil), vs...) })
			},
		},
	}
	pred, err := entdomain.ParseFIQL("ratio=in=(1.5,2.5,3.5)", fields)
	require.NoError(t, err)
	pred(nil)
	assert.Equal(t, []float64{1.5, 2.5, 3.5}, got)
}

func TestParseFIQL_UUIDIn(t *testing.T) {
	a := "550e8400-e29b-41d4-a716-446655440000"
	b := "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
	var got []uuid.UUID
	fields := entdomain.FIQLFields[testPred]{
		"id": entdomain.FIQLUUID[testPred]{
			In: func(vs ...uuid.UUID) testPred {
				return testPred(func(s *sql.Selector) { got = append([]uuid.UUID(nil), vs...) })
			},
		},
	}

	t.Run("happy path", func(t *testing.T) {
		got = nil
		pred, err := entdomain.ParseFIQL("id=in=("+a+","+b+")", fields)
		require.NoError(t, err)
		pred(nil)
		assert.Equal(t, []uuid.UUID{uuid.MustParse(a), uuid.MustParse(b)}, got)
	})

	t.Run("invalid uuid in list", func(t *testing.T) {
		_, err := entdomain.ParseFIQL("id=in=("+a+",not-a-uuid)", fields)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `invalid UUID value "not-a-uuid" in list`)
	})
}

func TestParseFIQL_EnumIn(t *testing.T) {
	var calls []string
	mkPred := func(tag string) testPred {
		return testPred(func(s *sql.Selector) { calls = append(calls, tag) })
	}
	fields := entdomain.FIQLFields[testPred]{
		"status": entdomain.FIQLEnum[testPred]{
			EQ:  map[string]testPred{"active": mkPred("eq:active"), "inactive": mkPred("eq:inactive")},
			NEQ: map[string]testPred{"active": mkPred("neq:active"), "inactive": mkPred("neq:inactive")},
		},
	}

	t.Run("=in= composes OR of EQ entries", func(t *testing.T) {
		calls = nil
		pred, err := entdomain.ParseFIQL("status=in=(active,inactive)", fields)
		require.NoError(t, err)
		pred(sql.Dialect("sqlite3").Select("*").From(sql.Table("t")))
		assert.ElementsMatch(t, []string{"eq:active", "eq:inactive"}, calls)
	})

	t.Run("=out= composes AND of NEQ entries", func(t *testing.T) {
		calls = nil
		pred, err := entdomain.ParseFIQL("status=out=(active,inactive)", fields)
		require.NoError(t, err)
		pred(sql.Dialect("sqlite3").Select("*").From(sql.Table("t")))
		assert.ElementsMatch(t, []string{"neq:active", "neq:inactive"}, calls)
	})

	t.Run("unknown enum value in list", func(t *testing.T) {
		_, err := entdomain.ParseFIQL("status=in=(active,pending)", fields)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `unknown enum value "pending" in list`)
	})
}

func TestParseFIQL_BoolRejectsIn(t *testing.T) {
	fields := entdomain.FIQLFields[testPred]{
		"active": entdomain.FIQLBool[testPred]{
			EQ: func(v bool) testPred { return testPred(func(s *sql.Selector) {}) },
		},
	}
	_, err := entdomain.ParseFIQL("active=in=(true,false)", fields)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only == is supported")
}

func TestParseFIQL_GroupingPathUntouched(t *testing.T) {
	// Regression: confirm the (expr) grouping path still works after
	// adding =in= / =out= and the readListValue dispatch.
	var calls []string
	record := func(tag string) testPred {
		return testPred(func(s *sql.Selector) { calls = append(calls, tag) })
	}
	fields := entdomain.FIQLFields[testPred]{
		"name": entdomain.FIQLString[testPred]{
			EQ: func(v string) testPred { return record("name=" + v) },
		},
		"score": entdomain.FIQLInt[testPred]{
			GT: func(v int) testPred { return record(fmt.Sprintf("score>%d", v)) },
		},
	}

	pred, err := entdomain.ParseFIQL("(name==a,name==b);score=gt=10", fields)
	require.NoError(t, err)
	pred(sql.Dialect("sqlite3").Select("*").From(sql.Table("t")))
	assert.Contains(t, calls, "name=a")
	assert.Contains(t, calls, "name=b")
	assert.Contains(t, calls, "score>10")
}

func TestParseFIQL_StringHasPrefix(t *testing.T) {
	var got string
	fields := entdomain.FIQLFields[testPred]{
		"name": entdomain.FIQLString[testPred]{
			HasPrefix: func(v string) testPred {
				return testPred(func(s *sql.Selector) { got = v })
			},
		},
	}

	pred, err := entdomain.ParseFIQL("name=prefix=jo", fields)
	require.NoError(t, err)
	pred(nil)
	assert.Equal(t, "jo", got)
}

func TestParseFIQL_Errors(t *testing.T) {
	fields := entdomain.FIQLFields[testPred]{
		"name": entdomain.FIQLString[testPred]{EQ: func(v string) testPred { return nil }},
		"age":  entdomain.FIQLInt[testPred]{GT: func(v int) testPred { return nil }},
	}

	t.Run("empty expression", func(t *testing.T) {
		_, err := entdomain.ParseFIQL("", fields)
		require.Error(t, err)
	})

	t.Run("unknown field", func(t *testing.T) {
		_, err := entdomain.ParseFIQL("email==foo", fields)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown field")
		assert.Contains(t, err.Error(), "email")
	})

	t.Run("disallowed operator on string field", func(t *testing.T) {
		// EQ is set but GT is nil on FIQLString
		_, err := entdomain.ParseFIQL("name=gt=foo", fields)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not allowed on string field")
	})

	t.Run("invalid int value", func(t *testing.T) {
		_, err := entdomain.ParseFIQL("age=gt=notanumber", fields)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid integer value")
	})

	t.Run("malformed operator", func(t *testing.T) {
		_, err := entdomain.ParseFIQL("name=badop=foo", fields)
		require.Error(t, err)
	})

	t.Run("missing closing paren", func(t *testing.T) {
		_, err := entdomain.ParseFIQL("(name==foo", fields)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "expected ')'")
	})

	t.Run("empty value rejected", func(t *testing.T) {
		_, err := entdomain.ParseFIQL("name==", fields)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty value")
	})

	t.Run("excessive nesting depth rejected", func(t *testing.T) {
		expr := strings.Repeat("(", 51) + "name==x" + strings.Repeat(")", 51)
		_, err := entdomain.ParseFIQL(expr, fields)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "maximum nesting depth")
	})
}

// sqlFields builds a FIQLFields registry where every op on every field produces a
// real SQL EQ condition — letting us inspect the generated WHERE clause.
func sqlFields(names ...string) entdomain.FIQLFields[testPred] {
	fields := make(entdomain.FIQLFields[testPred], len(names))
	for _, n := range names {
		name := n // capture
		fields[name] = entdomain.FIQLString[testPred]{
			EQ: func(v string) testPred { return sqlPred(name, v) },
		}
	}
	return fields
}

// TestParseFIQL_ComplexConditions verifies operator precedence and multi-term chains
// by inspecting the actual SQL WHERE clause produced by ent's selector.
func TestParseFIQL_ComplexConditions(t *testing.T) {
	fields := sqlFields("a", "b", "c", "d")

	cases := []struct {
		expr    string
		wantSQL string // expected WHERE fragment in the generated SELECT
	}{
		{
			// ';' (AND) binds tighter than ',' (OR) — standard FIQL precedence.
			// a==1,b==2;c==3  →  a=1 OR (b=2 AND c=3)
			expr:    "a==1,b==2;c==3",
			wantSQL: "`a` = ? OR (`b` = ? AND `c` = ?)",
		},
		{
			// Explicit grouping overrides precedence: (a OR b) AND c
			expr:    "(a==1,b==2);c==3",
			wantSQL: "(`a` = ? OR `b` = ?) AND `c` = ?",
		},
		{
			// Three-term AND chain — must produce flat AND, not nested pairs.
			expr:    "a==1;b==2;c==3",
			wantSQL: "`a` = ? AND `b` = ? AND `c` = ?",
		},
		{
			// Three-term OR chain — must produce flat OR.
			expr:    "a==1,b==2,c==3",
			wantSQL: "`a` = ? OR `b` = ? OR `c` = ?",
		},
		{
			// ADR-003 example: name==john;age=gt=25,status==active
			// Rewritten with only string fields: a==john;b==25,c==active
			// → (a AND b) OR c
			expr:    "a==john;b==25,c==active",
			wantSQL: "(`a` = ? AND `b` = ?) OR `c` = ?",
		},
		{
			// Nested groups: ((a AND b) OR c) AND d
			expr:    "((a==1;b==2),c==3);d==4",
			wantSQL: "((`a` = ? AND `b` = ?) OR `c` = ?) AND `d` = ?",
		},
	}

	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			pred, err := entdomain.ParseFIQL(tc.expr, fields)
			require.NoError(t, err, "expression must parse without error")

			got := buildSQL(pred)
			assert.Contains(t, got, tc.wantSQL,
				"SQL WHERE clause must reflect correct AND/OR structure")
		})
	}
}

// FuzzParseFIQL ensures the FIQL parser never panics on arbitrary input.
// Run with: go test -fuzz=FuzzParseFIQL -fuzztime=30s
func FuzzParseFIQL(f *testing.F) {
	// Seed corpus: valid expressions, edge cases, and known tricky inputs.
	seeds := []string{
		`name==alice`,
		`name==alice;age==30`,
		`name==alice,age==30`,
		`(name==alice;age==30),status==active`,
		``,
		`==`,
		`name==`,
		`;;`,
		`((((name==x))))`,
		`name==(`,
		strings.Repeat("(", 60) + `name==x` + strings.Repeat(")", 60),
	}
	for _, s := range seeds {
		f.Add(s)
	}

	fields := entdomain.FIQLFields[testPred]{
		"name":   entdomain.FIQLString[testPred]{EQ: func(v string) testPred { return sqlPred("name", v) }},
		"age":    entdomain.FIQLInt[testPred]{EQ: func(v int) testPred { return sqlPred("age", fmt.Sprintf("%d", v)) }},
		"status": entdomain.FIQLString[testPred]{EQ: func(v string) testPred { return sqlPred("status", v) }},
	}

	f.Fuzz(func(t *testing.T, input string) {
		// Must not panic regardless of input.
		_, _ = entdomain.ParseFIQL(input, fields)
	})
}

// --- ENTD-005: parse/compile split -----------------------------------------

// astTestFields is the registry shared by the parse/compile split tests. It
// covers the three shapes the split has to keep working: a string field with
// set membership, an int field with a range operator, and a second string
// field standing in for the tenant column an authorization rewrite injects.
func astTestFields() entdomain.FIQLFields[testPred] {
	inFn := func(col string) func(...string) testPred {
		return func(vs ...string) testPred {
			args := make([]any, len(vs))
			for i, v := range vs {
				args[i] = v
			}
			return testPred(func(s *sql.Selector) { s.Where(sql.In(col, args...)) })
		}
	}
	return entdomain.FIQLFields[testPred]{
		"ids": entdomain.FIQLString[testPred]{
			EQ: func(v string) testPred { return sqlPred("ids", v) },
			In: inFn("ids"),
		},
		"name": entdomain.FIQLString[testPred]{
			EQ: func(v string) testPred { return sqlPred("name", v) },
		},
		"org_id": entdomain.FIQLString[testPred]{
			EQ: func(v string) testPred { return sqlPred("org_id", v) },
		},
		"age": entdomain.FIQLInt[testPred]{
			GT: func(v int) testPred {
				return testPred(func(s *sql.Selector) { s.Where(sql.GT("age", v)) })
			},
		},
	}
}

// buildSQLArgs is buildSQL's sibling for assertions that care about the bound
// values rather than the WHERE shape.
func buildSQLArgs(pred testPred) []any {
	s := sql.Dialect("sqlite3").Select("*").From(sql.Table("t"))
	pred(s)
	_, args := s.Query()
	return args
}

// TestFIQLErrorMessagesUnchanged pins the exact text of one error per fault
// class. Splitting parse from compile moved field resolution to a later pass,
// so ordering between classes changed; the wording of each individual failure
// is the contract callers actually depend on and must not drift.
func TestFIQLErrorMessagesUnchanged(t *testing.T) {
	fields := astTestFields()
	cases := []struct{ expr, want string }{
		{"", "empty FIQL expression"},
		{"name", "expected operator at position 4"},
		{"name==a;b", "expected operator at position 9"},
		{"unknown==x", `unknown field "unknown" — annotate with entdomain.FIQL(...) to enable`},
		{"age=gt=abc", `field "age": invalid integer value "abc": strconv.Atoi: parsing "abc": invalid syntax`},
		{"name=is=maybe", `unknown =is= value "maybe" — valid: null, notnull`},
	}
	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			_, err := entdomain.ParseFIQL(tc.expr, fields)
			require.Error(t, err)
			assert.Equal(t, tc.want, err.Error())
		})
	}
}

// TestFIQLNodeSealed proves the node set is closed. A type declared outside
// package entdomain cannot satisfy FIQLNode, so adding a node type later stays
// non-breaking — nobody outside can be switching exhaustively over it.
func TestFIQLNodeSealed(t *testing.T) {
	type notANode struct{ Field string }
	var v any = notANode{Field: "ids"}
	_, ok := v.(entdomain.FIQLNode)
	assert.False(t, ok, "a type outside package entdomain must not satisfy FIQLNode")
}

// TestFindFIQLInValues covers the read-back path: parse an =in= term and
// recover its operands as a slice without touching the registry.
func TestFindFIQLInValues(t *testing.T) {
	node, err := entdomain.ParseFIQLExpr("ids=in=(abc,xyz,mnz)")
	require.NoError(t, err)

	found := entdomain.FindFIQL(node, "ids")
	require.Len(t, found, 1)
	assert.Equal(t, entdomain.In, found[0].Op)
	assert.Equal(t, []string{"abc", "xyz", "mnz"}, found[0].Values)

	assert.Empty(t, entdomain.FindFIQL(node, "name"))
}

// TestWalkFIQLDoesNotMutateSource guards the copy contract. The ergonomic
// transform edits Values in place; without the defensive copy that edit would
// reach the tree ParseFIQLExpr returned and destroy the caller's ability to
// audit-log the original expression while querying with the rewritten one.
func TestWalkFIQLDoesNotMutateSource(t *testing.T) {
	node, err := entdomain.ParseFIQLExpr("ids=in=(abc,xyz)")
	require.NoError(t, err)

	_, err = entdomain.WalkFIQL(node, func(c *entdomain.FIQLCmp) (entdomain.FIQLNode, error) {
		c.Values[0] = "CLOBBERED"
		c.Value = "CLOBBERED"
		return c, nil
	})
	require.NoError(t, err)

	src := entdomain.FindFIQL(node, "ids")
	require.Len(t, src, 1)
	assert.Equal(t, []string{"abc", "xyz"}, src[0].Values, "source tree must survive an in-place edit")
}

// TestFIQLInPrefixRoundTrip is the end-to-end shape the split exists for:
// parse without a registry, rewrite the operands, then compile to a predicate.
func TestFIQLInPrefixRoundTrip(t *testing.T) {
	node, err := entdomain.ParseFIQLExpr("ids=in=(abc,xyz,mnz)")
	require.NoError(t, err)

	node, err = entdomain.WalkFIQL(node, func(c *entdomain.FIQLCmp) (entdomain.FIQLNode, error) {
		if c.Field != "ids" || c.Op != entdomain.In {
			return c, nil
		}
		for i, v := range c.Values {
			c.Values[i] = "id-" + v
		}
		return c, nil
	})
	require.NoError(t, err)

	pred, err := entdomain.CompileFIQL(node, astTestFields())
	require.NoError(t, err)
	assert.Equal(t, []any{"id-abc", "id-xyz", "id-mnz"}, buildSQLArgs(pred))
}

// TestWalkFIQLAuthzInjection covers the tenant-scoping shape: the caller's
// expression is kept intact and ANDed with a predicate it cannot influence.
func TestWalkFIQLAuthzInjection(t *testing.T) {
	user, err := entdomain.ParseFIQLExpr("name==john,name==jane")
	require.NoError(t, err)

	scoped := &entdomain.FIQLAnd{Nodes: []entdomain.FIQLNode{
		user,
		&entdomain.FIQLCmp{Field: "org_id", Op: entdomain.EQ, Value: "org-42"},
	}}

	pred, err := entdomain.CompileFIQL(scoped, astTestFields())
	require.NoError(t, err)

	sqlText := buildSQL(pred)
	assert.Contains(t, sqlText, "org_id")
	assert.Contains(t, buildSQLArgs(pred), any("org-42"))
	assert.Contains(t, sqlText, "OR", "the caller's own OR must survive inside the AND")
}

// TestWalkFIQLValueTransform covers rewriting a scalar operand rather than a
// list, and confirms untouched terms pass through unchanged.
func TestWalkFIQLValueTransform(t *testing.T) {
	node, err := entdomain.ParseFIQLExpr("name==John;age=gt=25")
	require.NoError(t, err)

	node, err = entdomain.WalkFIQL(node, func(c *entdomain.FIQLCmp) (entdomain.FIQLNode, error) {
		if c.Field == "name" {
			c.Value = strings.ToLower(c.Value)
		}
		return c, nil
	})
	require.NoError(t, err)

	pred, err := entdomain.CompileFIQL(node, astTestFields())
	require.NoError(t, err)
	assert.Contains(t, buildSQLArgs(pred), any("john"))
	assert.Contains(t, buildSQLArgs(pred), any(25))
}

// TestWalkFIQLTermFilter covers pruning: dropping one term keeps the rest, and
// dropping every term yields nil rather than a match-everything predicate.
func TestWalkFIQLTermFilter(t *testing.T) {
	dropName := func(c *entdomain.FIQLCmp) (entdomain.FIQLNode, error) {
		if c.Field == "name" {
			return nil, nil
		}
		return c, nil
	}

	t.Run("drops one term, keeps the rest", func(t *testing.T) {
		node, err := entdomain.ParseFIQLExpr("name==john;age=gt=25")
		require.NoError(t, err)

		node, err = entdomain.WalkFIQL(node, dropName)
		require.NoError(t, err)

		pred, err := entdomain.CompileFIQL(node, astTestFields())
		require.NoError(t, err)
		args := buildSQLArgs(pred)
		assert.Contains(t, args, any(25))
		assert.NotContains(t, args, any("john"))
	})

	t.Run("dropping every term yields nil, not match-all", func(t *testing.T) {
		node, err := entdomain.ParseFIQLExpr("name==john,name==jane")
		require.NoError(t, err)

		out, err := entdomain.WalkFIQL(node, dropName)
		require.NoError(t, err)
		assert.Nil(t, out)

		_, err = entdomain.CompileFIQL(out, astTestFields())
		require.Error(t, err, "a fully pruned tree must not compile to match-everything")
		assert.Equal(t, "empty FIQL expression", err.Error())
	})

	t.Run("callback error rejects the whole expression", func(t *testing.T) {
		node, err := entdomain.ParseFIQLExpr("name==john;age=gt=25")
		require.NoError(t, err)

		_, err = entdomain.WalkFIQL(node, func(c *entdomain.FIQLCmp) (entdomain.FIQLNode, error) {
			if c.Field == "age" {
				return nil, fmt.Errorf("field %q is not filterable by this caller", c.Field)
			}
			return c, nil
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not filterable")
	})
}

// TestParseFIQLExprLimitsWithoutRegistry confirms both hostile-input bounds are
// enforced by the syntax pass. Before the split, the list bound lived in apply
// and so only fired after a registry lookup had already succeeded.
func TestParseFIQLExprLimitsWithoutRegistry(t *testing.T) {
	t.Run("nesting depth", func(t *testing.T) {
		expr := strings.Repeat("(", 51) + "a==1" + strings.Repeat(")", 51)
		_, err := entdomain.ParseFIQLExpr(expr)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "maximum nesting depth of 50")
	})

	t.Run("value list length", func(t *testing.T) {
		expr := "a=in=(" + strings.TrimSuffix(strings.Repeat("v,", 101), ",") + ")"
		_, err := entdomain.ParseFIQLExpr(expr)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "maximum of 100 entries")
	})
}

// TestCompileFIQLNilNode pins the fail-closed contract in isolation.
func TestCompileFIQLNilNode(t *testing.T) {
	_, err := entdomain.CompileFIQL(nil, astTestFields())
	require.Error(t, err)
	assert.Equal(t, "empty FIQL expression", err.Error())
}

// --- ENTD-006: AST serialization -------------------------------------------

// TestToFIQLRoundTrip pins that any expression the parser accepts renders back
// to the identical string. The parenthesised cases matter most: the parser
// drops redundant parens, so the serializer has to re-derive exactly the ones
// precedence requires.
func TestToFIQLRoundTrip(t *testing.T) {
	for _, expr := range []string{
		"ids=in=(abc,xyz,mnz)",
		"name==john;age=gt=25",
		"a==1,b==2;c==3",
		"(a==1,b==2);c==3",
		"bio=is=null",
		"bio=is=notnull",
		"x==1;(y==2,z==3);w==4",
		"name=like=jo",
	} {
		t.Run(expr, func(t *testing.T) {
			node, err := entdomain.ParseFIQLExpr(expr)
			require.NoError(t, err)

			out, err := entdomain.ToFIQL(node)
			require.NoError(t, err)
			assert.Equal(t, expr, out)

			// Rendering must also be stable: parsing the output and rendering
			// again yields the same text.
			again, err := entdomain.ParseFIQLExpr(out)
			require.NoError(t, err)
			out2, err := entdomain.ToFIQL(again)
			require.NoError(t, err)
			assert.Equal(t, out, out2)
		})
	}
}

// TestToFIQLRejectsUnrepresentable covers operands the grammar cannot carry.
// The "a,b==c" row is the reason ToFIQL returns an error at all: rendered
// naively it parses back cleanly into a different query, with nothing to
// signal that the filter changed meaning.
func TestToFIQLRejectsUnrepresentable(t *testing.T) {
	for _, tc := range []struct{ name, value, wantSubstr string }{
		{"semicolon", "a;b", `reserved character ";"`},
		{"comma", "a,b", `reserved character ","`},
		{"close paren", "a)b", `reserved character ")"`},
		{"silent corruption", "a,b==c", `reserved character ","`},
		{"empty", "", "empty value cannot be rendered"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			node := &entdomain.FIQLCmp{Field: "name", Op: entdomain.EQ, Value: tc.value}
			_, err := entdomain.ToFIQL(node)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantSubstr)
		})
	}

	t.Run("reserved character inside an =in= list", func(t *testing.T) {
		node := &entdomain.FIQLCmp{Field: "ids", Op: entdomain.In, Values: []string{"ok", "a,b"}}
		_, err := entdomain.ToFIQL(node)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `reserved character ","`)
	})

	t.Run("open paren is allowed", func(t *testing.T) {
		node := &entdomain.FIQLCmp{Field: "name", Op: entdomain.EQ, Value: "foo(bar"}
		out, err := entdomain.ToFIQL(node)
		require.NoError(t, err)
		assert.Equal(t, "name==foo(bar", out)

		back, err := entdomain.ParseFIQLExpr(out)
		require.NoError(t, err)
		assert.Equal(t, "foo(bar", entdomain.FindFIQL(back, "name")[0].Value)
	})
}

// TestToFIQLRejectsMalformedNode covers hand-built trees that cannot render.
// These shapes never come out of ParseFIQLExpr; they come from callers
// assembling nodes directly, which the authorization pattern encourages.
func TestToFIQLRejectsMalformedNode(t *testing.T) {
	cases := []struct {
		name       string
		node       entdomain.FIQLNode
		wantSubstr string
	}{
		{"nil", nil, "empty FIQL expression"},
		{"empty And", &entdomain.FIQLAnd{}, "FIQLAnd with no children"},
		{"empty Or", &entdomain.FIQLOr{}, "FIQLOr with no children"},
		{
			"empty field",
			&entdomain.FIQLCmp{Op: entdomain.EQ, Value: "x"},
			"empty field name",
		},
		{
			"field outside the selector grammar",
			&entdomain.FIQLCmp{Field: "user.name", Op: entdomain.EQ, Value: "x"},
			`contains character "."`,
		},
		{
			"parser-internal Is op",
			&entdomain.FIQLCmp{Field: "bio", Op: entdomain.Is, Value: "null"},
			"cannot be rendered",
		},
		{
			"=in= with no values",
			&entdomain.FIQLCmp{Field: "ids", Op: entdomain.In},
			"no values to render",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := entdomain.ToFIQL(tc.node)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantSubstr)
		})
	}
}

// TestToFIQLAfterWalk is the motivating flow: rewrite a filter, render the
// rewritten form for the audit log, and confirm it still compiles to the
// predicate the query runs on.
func TestToFIQLAfterWalk(t *testing.T) {
	node, err := entdomain.ParseFIQLExpr("ids=in=(abc,xyz,mnz)")
	require.NoError(t, err)

	rewritten, err := entdomain.WalkFIQL(node, func(c *entdomain.FIQLCmp) (entdomain.FIQLNode, error) {
		for i, v := range c.Values {
			c.Values[i] = "id-" + v
		}
		return c, nil
	})
	require.NoError(t, err)

	out, err := entdomain.ToFIQL(rewritten)
	require.NoError(t, err)
	assert.Equal(t, "ids=in=(id-abc,id-xyz,id-mnz)", out)

	// The original stays renderable and unchanged — that is the whole point of
	// WalkFIQL copying: both forms can be logged side by side.
	original, err := entdomain.ToFIQL(node)
	require.NoError(t, err)
	assert.Equal(t, "ids=in=(abc,xyz,mnz)", original)

	pred, err := entdomain.CompileFIQL(rewritten, astTestFields())
	require.NoError(t, err)
	assert.Equal(t, []any{"id-abc", "id-xyz", "id-mnz"}, buildSQLArgs(pred))
}

// TestToFIQLAuthzInjectionRenders covers rendering a hand-assembled tree — the
// tenant-scoping shape from the README — including the parens the injected AND
// forces around the caller's OR.
func TestToFIQLAuthzInjectionRenders(t *testing.T) {
	user, err := entdomain.ParseFIQLExpr("name==john,name==jane")
	require.NoError(t, err)

	scoped := &entdomain.FIQLAnd{Nodes: []entdomain.FIQLNode{
		user,
		&entdomain.FIQLCmp{Field: "org_id", Op: entdomain.EQ, Value: "org-42"},
	}}

	out, err := entdomain.ToFIQL(scoped)
	require.NoError(t, err)
	assert.Equal(t, "(name==john,name==jane);org_id==org-42", out)

	back, err := entdomain.ParseFIQLExpr(out)
	require.NoError(t, err)
	again, err := entdomain.ToFIQL(back)
	require.NoError(t, err)
	assert.Equal(t, out, again, "the injected scope must survive a parse/render cycle")
}

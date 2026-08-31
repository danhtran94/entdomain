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
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
)

// FIQLOp is a FIQL comparison operator.
type FIQLOp string

const (
	// EQ matches field == value (all types).
	EQ FIQLOp = "=="
	// NEQ matches field != value (all types).
	NEQ FIQLOp = "!="
	// GT matches field > value (int, float, time).
	GT FIQLOp = "=gt="
	// LT matches field < value (int, float, time).
	LT FIQLOp = "=lt="
	// GTE matches field >= value (int, float, time).
	GTE FIQLOp = "=ge="
	// LTE matches field <= value (int, float, time).
	LTE FIQLOp = "=le="
	// Contains matches field LIKE '%value%' (string).
	Contains FIQLOp = "=like="
	// HasPrefix matches field LIKE 'value%' (string).
	HasPrefix FIQLOp = "=prefix="
	// In matches field IN (v1, v2, ...) — value syntax: =in=(a,b,c).
	// Supported on string, int, float, enum, uuid fields.
	In FIQLOp = "=in="
	// NotIn matches field NOT IN (v1, v2, ...) — value syntax: =out=(a,b,c).
	// Supported on string, int, float, enum, uuid fields.
	NotIn FIQLOp = "=out="
	// Is is the parser-internal wire form ("=is=") that gets normalized into
	// IsNull or NotNull based on its value during parseComparison. Never
	// reaches apply — the normalization is the contract. Kept as a constant
	// so readOp can recognise the wire form symbolically.
	Is FIQLOp = "=is="
	// IsNull matches field IS NULL — value syntax: field=is=null.
	// Synthetic op produced by parser normalization (never appears on the wire
	// literally). Supported on Optional() / Nillable() fields only — codegen
	// errors otherwise.
	IsNull FIQLOp = "=is=null"
	// NotNull matches field IS NOT NULL — value syntax: field=is=notnull.
	// Synthetic op produced by parser normalization. Same optionality
	// requirement as IsNull.
	NotNull FIQLOp = "=is=notnull"
)

// validIsValues is the source of truth for the strict "=is=" value vocabulary.
// Used by both the parser normalization (parseComparison) and the error message
// when an unknown value is supplied.
var validIsValues = []string{"null", "notnull"}

// opByName lets templates address operators by their Go identifier (e.g.
// "In") instead of hard-coding the FIQL symbol (e.g. "=in="). Adding a new
// operator means one entry here + one FIQLOp constant; templates branch via
// the `isOp` FuncMap helper registered in template.go.
//
// TestOpRegistryCovered (fiql_test.go) guards against drift — adding a new
// FIQLOp constant without a registry entry fails the test.
var opByName = map[string]FIQLOp{
	"EQ":        EQ,
	"NEQ":       NEQ,
	"GT":        GT,
	"LT":        LT,
	"GTE":       GTE,
	"LTE":       LTE,
	"Contains":  Contains,
	"HasPrefix": HasPrefix,
	"In":        In,
	"NotIn":     NotIn,
	"IsNull":    IsNull,
	"NotNull":   NotNull,
}

// isOpFn powers the `isOp` template helper: `{{ if isOp "In" $op }}` instead
// of `{{ if eq $op "=in=" }}`. Keeping the symbol → constant mapping in Go
// (opByName) means adding a new operator is one registry entry, not a hunt
// through every template branch that hard-codes the literal.
func isOpFn(name string, op string) bool {
	return opByName[name] == FIQLOp(op)
}

// Predicate is a constraint for ent predicate types. All ent predicate types have
// the underlying type func(*sql.Selector), enabling generic AND/OR combination.
type Predicate interface {
	~func(*sql.Selector)
}

// FIQLFields maps field names to their FIQL field descriptors.
type FIQLFields[P Predicate] map[string]FIQLField[P]

// FIQLField is implemented by all typed FIQL field helpers.
type FIQLField[P Predicate] interface {
	// vals carries the operands for In/NotIn, already split and validated by
	// the parser; val carries the operand for every other operator. Widening
	// this signature is safe precisely because apply is unexported — no type
	// outside this package can implement FIQLField.
	apply(op FIQLOp, val string, vals []string) (P, error)
}

// FIQLString handles FIQL filtering for string fields.
type FIQLString[P Predicate] struct {
	EQ        func(string) P
	NEQ       func(string) P
	Contains  func(string) P
	HasPrefix func(string) P
	In        func(...string) P
	NotIn     func(...string) P
	IsNil     func() P
	NotNil    func() P
}

func (f FIQLString[P]) apply(op FIQLOp, val string, vals []string) (P, error) {
	var zero P
	switch op {
	case EQ:
		if f.EQ == nil {
			return zero, fmt.Errorf("operator == not allowed on this string field")
		}
		return f.EQ(val), nil
	case NEQ:
		if f.NEQ == nil {
			return zero, fmt.Errorf("operator != not allowed on this string field")
		}
		return f.NEQ(val), nil
	case Contains:
		if f.Contains == nil {
			return zero, fmt.Errorf("operator =like= not allowed on this string field")
		}
		return f.Contains(val), nil
	case HasPrefix:
		if f.HasPrefix == nil {
			return zero, fmt.Errorf("operator =prefix= not allowed on this string field")
		}
		return f.HasPrefix(val), nil
	case In:
		if f.In == nil {
			return zero, fmt.Errorf("operator =in= not allowed on this string field")
		}
		parts := vals
		return f.In(parts...), nil
	case NotIn:
		if f.NotIn == nil {
			return zero, fmt.Errorf("operator =out= not allowed on this string field")
		}
		parts := vals
		return f.NotIn(parts...), nil
	case IsNull, NotNull:
		return applyNullTyped(op, f.IsNil, f.NotNil, "string")
	default:
		return zero, fmt.Errorf("operator %q not allowed on string field", op)
	}
}

// FIQLInt handles FIQL filtering for integer fields.
type FIQLInt[P Predicate] struct {
	EQ     func(int) P
	NEQ    func(int) P
	GT     func(int) P
	LT     func(int) P
	GTE    func(int) P
	LTE    func(int) P
	In     func(...int) P
	NotIn  func(...int) P
	IsNil  func() P
	NotNil func() P
}

func (f FIQLInt[P]) apply(op FIQLOp, val string, vals []string) (P, error) {
	var zero P
	if op == In || op == NotIn {
		return applyListTyped(op, vals, f.In, f.NotIn, strconv.Atoi, "integer", "int")
	}
	if op == IsNull || op == NotNull {
		return applyNullTyped(op, f.IsNil, f.NotNil, "int")
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return zero, fmt.Errorf("invalid integer value %q: %w", val, err)
	}
	switch op {
	case EQ:
		if f.EQ == nil {
			return zero, fmt.Errorf("operator == not allowed on this int field")
		}
		return f.EQ(n), nil
	case NEQ:
		if f.NEQ == nil {
			return zero, fmt.Errorf("operator != not allowed on this int field")
		}
		return f.NEQ(n), nil
	case GT:
		if f.GT == nil {
			return zero, fmt.Errorf("operator =gt= not allowed on this int field")
		}
		return f.GT(n), nil
	case LT:
		if f.LT == nil {
			return zero, fmt.Errorf("operator =lt= not allowed on this int field")
		}
		return f.LT(n), nil
	case GTE:
		if f.GTE == nil {
			return zero, fmt.Errorf("operator =ge= not allowed on this int field")
		}
		return f.GTE(n), nil
	case LTE:
		if f.LTE == nil {
			return zero, fmt.Errorf("operator =le= not allowed on this int field")
		}
		return f.LTE(n), nil
	default:
		return zero, fmt.Errorf("operator %q not allowed on int field", op)
	}
}

// FIQLFloat handles FIQL filtering for float fields.
type FIQLFloat[P Predicate] struct {
	EQ     func(float64) P
	NEQ    func(float64) P
	GT     func(float64) P
	LT     func(float64) P
	GTE    func(float64) P
	LTE    func(float64) P
	In     func(...float64) P
	NotIn  func(...float64) P
	IsNil  func() P
	NotNil func() P
}

func (f FIQLFloat[P]) apply(op FIQLOp, val string, vals []string) (P, error) {
	var zero P
	if op == In || op == NotIn {
		return applyListTyped(op, vals, f.In, f.NotIn,
			func(s string) (float64, error) { return strconv.ParseFloat(s, 64) },
			"float", "float")
	}
	if op == IsNull || op == NotNull {
		return applyNullTyped(op, f.IsNil, f.NotNil, "float")
	}
	n, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return zero, fmt.Errorf("invalid float value %q: %w", val, err)
	}
	switch op {
	case EQ:
		if f.EQ == nil {
			return zero, fmt.Errorf("operator == not allowed on this float field")
		}
		return f.EQ(n), nil
	case NEQ:
		if f.NEQ == nil {
			return zero, fmt.Errorf("operator != not allowed on this float field")
		}
		return f.NEQ(n), nil
	case GT:
		if f.GT == nil {
			return zero, fmt.Errorf("operator =gt= not allowed on this float field")
		}
		return f.GT(n), nil
	case LT:
		if f.LT == nil {
			return zero, fmt.Errorf("operator =lt= not allowed on this float field")
		}
		return f.LT(n), nil
	case GTE:
		if f.GTE == nil {
			return zero, fmt.Errorf("operator =ge= not allowed on this float field")
		}
		return f.GTE(n), nil
	case LTE:
		if f.LTE == nil {
			return zero, fmt.Errorf("operator =le= not allowed on this float field")
		}
		return f.LTE(n), nil
	default:
		return zero, fmt.Errorf("operator %q not allowed on float field", op)
	}
}

// FIQLTime handles FIQL filtering for time.Time fields.
// Values are parsed as RFC3339 (e.g. "2024-01-15T10:30:00Z").
type FIQLTime[P Predicate] struct {
	EQ     func(time.Time) P
	NEQ    func(time.Time) P
	GT     func(time.Time) P
	LT     func(time.Time) P
	GTE    func(time.Time) P
	LTE    func(time.Time) P
	IsNil  func() P
	NotNil func() P
}

func (f FIQLTime[P]) apply(op FIQLOp, val string, vals []string) (P, error) {
	var zero P
	if op == IsNull || op == NotNull {
		return applyNullTyped(op, f.IsNil, f.NotNil, "time")
	}
	t, err := time.Parse(time.RFC3339, val)
	if err != nil {
		return zero, fmt.Errorf("invalid time value %q (expected RFC3339, e.g. 2024-01-15T10:30:00Z): %w", val, err)
	}
	switch op {
	case EQ:
		if f.EQ == nil {
			return zero, fmt.Errorf("operator == not allowed on this time field")
		}
		return f.EQ(t), nil
	case NEQ:
		if f.NEQ == nil {
			return zero, fmt.Errorf("operator != not allowed on this time field")
		}
		return f.NEQ(t), nil
	case GT:
		if f.GT == nil {
			return zero, fmt.Errorf("operator =gt= not allowed on this time field")
		}
		return f.GT(t), nil
	case LT:
		if f.LT == nil {
			return zero, fmt.Errorf("operator =lt= not allowed on this time field")
		}
		return f.LT(t), nil
	case GTE:
		if f.GTE == nil {
			return zero, fmt.Errorf("operator =ge= not allowed on this time field")
		}
		return f.GTE(t), nil
	case LTE:
		if f.LTE == nil {
			return zero, fmt.Errorf("operator =le= not allowed on this time field")
		}
		return f.LTE(t), nil
	default:
		return zero, fmt.Errorf("operator %q not allowed on time field", op)
	}
}

// FIQLBool handles FIQL filtering for boolean fields.
// Values must be "true" or "false".
type FIQLBool[P Predicate] struct {
	EQ     func(bool) P
	IsNil  func() P
	NotNil func() P
}

func (f FIQLBool[P]) apply(op FIQLOp, val string, vals []string) (P, error) {
	var zero P
	if op == IsNull || op == NotNull {
		return applyNullTyped(op, f.IsNil, f.NotNil, "bool")
	}
	if op != EQ {
		return zero, fmt.Errorf("operator %q not allowed on bool field — only == is supported", op)
	}
	b, err := strconv.ParseBool(val)
	if err != nil {
		return zero, fmt.Errorf("invalid bool value %q (expected true or false): %w", val, err)
	}
	if f.EQ == nil {
		return zero, fmt.Errorf("operator == not configured on this bool field")
	}
	return f.EQ(b), nil
}

// FIQLUUID handles FIQL filtering for UUID fields (github.com/google/uuid).
// Values are parsed via uuid.Parse, which accepts canonical 36-char hyphenated
// form, braced ({...}), urn:uuid:..., and 32-char hex without hyphens.
// Only == and != are supported — UUIDs are opaque identifiers, so ordering and
// substring operators are intentionally rejected.
type FIQLUUID[P Predicate] struct {
	EQ     func(uuid.UUID) P
	NEQ    func(uuid.UUID) P
	In     func(...uuid.UUID) P
	NotIn  func(...uuid.UUID) P
	IsNil  func() P
	NotNil func() P
}

func (f FIQLUUID[P]) apply(op FIQLOp, val string, vals []string) (P, error) {
	var zero P
	if op == In || op == NotIn {
		return applyListTyped(op, vals, f.In, f.NotIn, uuid.Parse, "UUID", "UUID")
	}
	if op == IsNull || op == NotNull {
		return applyNullTyped(op, f.IsNil, f.NotNil, "UUID")
	}
	u, err := uuid.Parse(val)
	if err != nil {
		return zero, fmt.Errorf("invalid UUID value %q: %w", val, err)
	}
	switch op {
	case EQ:
		if f.EQ == nil {
			return zero, fmt.Errorf("operator == not allowed on this UUID field")
		}
		return f.EQ(u), nil
	case NEQ:
		if f.NEQ == nil {
			return zero, fmt.Errorf("operator != not allowed on this UUID field")
		}
		return f.NEQ(u), nil
	default:
		return zero, fmt.Errorf("operator %q not allowed on UUID field — only == and != are supported", op)
	}
}

// FIQLEnum handles FIQL filtering for enum fields.
// Predicates are pre-built at generation time; lookups are O(1) map access.
type FIQLEnum[P Predicate] struct {
	EQ     map[string]P
	NEQ    map[string]P
	IsNil  func() P
	NotNil func() P
}

func (f FIQLEnum[P]) apply(op FIQLOp, val string, vals []string) (P, error) {
	var zero P
	switch op {
	case EQ:
		if f.EQ == nil {
			return zero, fmt.Errorf("operator == not allowed on this enum field")
		}
		p, ok := f.EQ[val]
		if !ok {
			return zero, fmt.Errorf("unknown enum value %q — valid: %s", val, sortedMapKeys(f.EQ))
		}
		return p, nil
	case NEQ:
		if f.NEQ == nil {
			return zero, fmt.Errorf("operator != not allowed on this enum field")
		}
		p, ok := f.NEQ[val]
		if !ok {
			return zero, fmt.Errorf("unknown enum value %q — valid: %s", val, sortedMapKeys(f.NEQ))
		}
		return p, nil
	case In:
		if f.EQ == nil {
			return zero, fmt.Errorf("operator =in= not allowed on this enum field (requires == annotation)")
		}
		parts := vals
		preds := make([]P, len(parts))
		for i, v := range parts {
			p, ok := f.EQ[v]
			if !ok {
				return zero, fmt.Errorf("unknown enum value %q in list — valid: %s", v, sortedMapKeys(f.EQ))
			}
			preds[i] = p
		}
		if len(preds) == 1 {
			return preds[0], nil
		}
		return orPreds(preds...), nil
	case NotIn:
		if f.NEQ == nil {
			return zero, fmt.Errorf("operator =out= not allowed on this enum field (requires != annotation)")
		}
		parts := vals
		preds := make([]P, len(parts))
		for i, v := range parts {
			p, ok := f.NEQ[v]
			if !ok {
				return zero, fmt.Errorf("unknown enum value %q in list — valid: %s", v, sortedMapKeys(f.NEQ))
			}
			preds[i] = p
		}
		if len(preds) == 1 {
			return preds[0], nil
		}
		return andPreds(preds...), nil
	case IsNull, NotNull:
		return applyNullTyped(op, f.IsNil, f.NotNil, "enum")
	default:
		return zero, fmt.Errorf("operator %q not allowed on enum field — only ==, !=, =in=, =out=, =is= are supported", op)
	}
}

// sortedMapKeys returns the keys of a map as a sorted comma-separated string.
func sortedMapKeys[P Predicate](m map[string]P) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}

// ParseFIQL parses a FIQL expression and returns an ent predicate using the provided
// field registry. Returns an error for unknown fields, disallowed operators, or malformed
// expressions.
//
// FIQL syntax:
//   - field==value        equality
//   - field!=value        inequality
//   - field=gt=value      greater than (numeric/time fields)
//   - field=lt=value      less than
//   - field=ge=value      greater than or equal
//   - field=le=value      less than or equal
//   - field=like=value    contains substring (string fields)
//   - field=prefix=value  has prefix (string fields)
//   - expr;expr           AND (higher precedence)
//   - expr,expr           OR (lower precedence)
//   - (expr)              grouping
func ParseFIQL[P Predicate](expr string, fields FIQLFields[P]) (P, error) {
	var zero P
	node, err := ParseFIQLExpr(expr)
	if err != nil {
		return zero, err
	}
	return CompileFIQL(node, fields)
}

// CompileFIQL turns a parsed FIQL AST into an ent predicate using the
// provided field registry. It resolves each comparison's field, checks the
// operator is allowed on it, and coerces the raw value to the field's Go
// type — the three things ParseFIQLExpr deliberately does not do.
//
// A nil node is an error, not a match-everything predicate. A WalkFIQL
// rewrite that prunes every term yields nil, and silently compiling that
// into "match all" would turn a failed authorization filter into an
// unfiltered query. Callers that want an absent filter to mean "no
// restriction" must handle the empty case explicitly.
func CompileFIQL[P Predicate](n FIQLNode, fields FIQLFields[P]) (P, error) {
	var zero P
	switch v := n.(type) {
	case nil:
		return zero, fmt.Errorf("empty FIQL expression")
	case *FIQLAnd:
		preds, err := compileChildren(v.Nodes, fields)
		if err != nil {
			return zero, err
		}
		return andPreds(preds...), nil
	case *FIQLOr:
		preds, err := compileChildren(v.Nodes, fields)
		if err != nil {
			return zero, err
		}
		return orPreds(preds...), nil
	case *FIQLCmp:
		fieldDesc, ok := fields[v.Field]
		if !ok {
			return zero, fmt.Errorf("unknown field %q — annotate with entdomain.FIQL(...) to enable", v.Field)
		}
		pred, err := fieldDesc.apply(v.Op, v.Value, v.Values)
		if err != nil {
			return zero, fmt.Errorf("field %q: %w", v.Field, err)
		}
		return pred, nil
	default:
		return zero, fmt.Errorf("unknown FIQL node type %T", n)
	}
}

// compileChildren compiles every child of an And/Or node. A nil child is
// rejected rather than skipped: WalkFIQL prunes empty branches on the way
// out, so a nil surviving into compile means a hand-built tree is malformed.
func compileChildren[P Predicate](nodes []FIQLNode, fields FIQLFields[P]) ([]P, error) {
	preds := make([]P, 0, len(nodes))
	for _, child := range nodes {
		pred, err := CompileFIQL(child, fields)
		if err != nil {
			return nil, err
		}
		preds = append(preds, pred)
	}
	return preds, nil
}

// maxFIQLDepth is the maximum nesting depth for parenthesised groups.
// Prevents stack overflow on maliciously crafted expressions.
const maxFIQLDepth = 50

// maxFIQLListValues is the maximum number of values in a single =in= or =out=
// list. Bounds parse cost and downstream SQL planner cost on hostile input.
const maxFIQLListValues = 100

// parseInListValue strips the enclosing parens from a =in= / =out= value,
// splits on ',', and enforces maxFIQLListValues. Returns the per-element
// raw strings; per-type callers parse each element with their own parser.
func parseInListValue(val string) ([]string, error) {
	if len(val) < 2 || val[0] != '(' || val[len(val)-1] != ')' {
		return nil, fmt.Errorf("malformed list value %q (expected (a,b,c))", val)
	}
	inner := val[1 : len(val)-1]
	if inner == "" {
		return nil, fmt.Errorf("empty value list for =in=/=out= operator")
	}
	parts := strings.Split(inner, ",")
	if len(parts) > maxFIQLListValues {
		return nil, fmt.Errorf("list value exceeds maximum of %d entries", maxFIQLListValues)
	}
	return parts, nil
}

// applyListTyped centralises the =in= / =out= dispatch shared by FIQLInt,
// FIQLFloat, and FIQLUUID. Each typed apply was 25 lines of nearly-identical
// boilerplate (nil-check op, parse list, parse each element, call inFn /
// notInFn) — hidden duplication that drifted across CodeRabbit cycles.
//
// parseLabel is the noun used in parse errors ("integer", "float", "UUID");
// fieldLabel is the noun used in op-not-allowed errors ("int", "float",
// "UUID"). They differ for int (parse "integer" vs field "int") — preserved
// to match existing test wording.
//
// FIQLString uses parseInListValue directly (no element parser needed) and
// is small enough to skip the helper. FIQLEnum and FIQLBool have different
// shapes (map composition, single-EQ) and don't fit.
func applyListTyped[T any, P Predicate](
	op FIQLOp,
	parts []string,
	inFn func(...T) P,
	notInFn func(...T) P,
	parseOne func(string) (T, error),
	parseLabel string,
	fieldLabel string,
) (P, error) {
	var zero P
	// Guard: this helper only handles set-membership ops. Without an explicit
	// switch, the tail `if op == In ... else NotIn` would silently treat any
	// future caller bug as NotIn instead of failing clearly.
	switch op {
	case In, NotIn:
	default:
		return zero, fmt.Errorf("operator %q not supported by applyListTyped (only =in= and =out=)", op)
	}
	if op == In && inFn == nil {
		return zero, fmt.Errorf("operator =in= not allowed on this %s field", fieldLabel)
	}
	if op == NotIn && notInFn == nil {
		return zero, fmt.Errorf("operator =out= not allowed on this %s field", fieldLabel)
	}
	ts := make([]T, len(parts))
	for i, s := range parts {
		t, err := parseOne(s)
		if err != nil {
			return zero, fmt.Errorf("invalid %s value %q in list: %w", parseLabel, s, err)
		}
		ts[i] = t
	}
	if op == In {
		return inFn(ts...), nil
	}
	return notInFn(ts...), nil
}

// applyNullTyped centralises the IsNull / NotNull dispatch shared by every
// typed FIQL field. Each per-type apply method's null branch is identical
// modulo the field-kind label — six lines × seven types would duplicate the
// boilerplate; one helper keeps the per-type apply methods uniform and gives
// the dispatch one place to grow if the contract evolves.
//
// fieldLabel is the noun used in op-not-allowed errors ("string", "int",
// "UUID", etc.) — preserved verbatim across types so existing assertion
// patterns continue to match.
func applyNullTyped[P Predicate](op FIQLOp, isNilFn, notNilFn func() P, fieldLabel string) (P, error) {
	var zero P
	switch op {
	case IsNull:
		if isNilFn == nil {
			return zero, fmt.Errorf("operator =is=null not allowed on this %s field", fieldLabel)
		}
		return isNilFn(), nil
	case NotNull:
		if notNilFn == nil {
			return zero, fmt.Errorf("operator =is=notnull not allowed on this %s field", fieldLabel)
		}
		return notNilFn(), nil
	default:
		return zero, fmt.Errorf("operator %q not supported by applyNullTyped", op)
	}
}

// andPreds combines multiple predicates with AND.
func andPreds[P Predicate](ps ...P) P {
	return P(sql.AndPredicates(ps...))
}

// orPreds combines multiple predicates with OR.
func orPreds[P Predicate](ps ...P) P {
	return P(sql.OrPredicates(ps...))
}

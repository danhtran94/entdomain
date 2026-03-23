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

package entdomain

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
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
)

// Predicate is a constraint for ent predicate types. All ent predicate types have
// the underlying type func(*sql.Selector), enabling generic AND/OR combination.
type Predicate interface {
	~func(*sql.Selector)
}

// FIQLFields maps field names to their FIQL field descriptors.
type FIQLFields[P Predicate] map[string]FIQLField[P]

// FIQLField is implemented by all typed FIQL field helpers.
type FIQLField[P Predicate] interface {
	apply(op FIQLOp, value string) (P, error)
}

// FIQLString handles FIQL filtering for string fields.
type FIQLString[P Predicate] struct {
	EQ        func(string) P
	NEQ       func(string) P
	Contains  func(string) P
	HasPrefix func(string) P
}

func (f FIQLString[P]) apply(op FIQLOp, val string) (P, error) {
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
	default:
		return zero, fmt.Errorf("operator %q not allowed on string field", op)
	}
}

// FIQLInt handles FIQL filtering for integer fields.
type FIQLInt[P Predicate] struct {
	EQ  func(int) P
	NEQ func(int) P
	GT  func(int) P
	LT  func(int) P
	GTE func(int) P
	LTE func(int) P
}

func (f FIQLInt[P]) apply(op FIQLOp, val string) (P, error) {
	var zero P
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
	EQ  func(float64) P
	NEQ func(float64) P
	GT  func(float64) P
	LT  func(float64) P
	GTE func(float64) P
	LTE func(float64) P
}

func (f FIQLFloat[P]) apply(op FIQLOp, val string) (P, error) {
	var zero P
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
	EQ  func(time.Time) P
	NEQ func(time.Time) P
	GT  func(time.Time) P
	LT  func(time.Time) P
	GTE func(time.Time) P
	LTE func(time.Time) P
}

func (f FIQLTime[P]) apply(op FIQLOp, val string) (P, error) {
	var zero P
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
	EQ func(bool) P
}

func (f FIQLBool[P]) apply(op FIQLOp, val string) (P, error) {
	var zero P
	b, err := strconv.ParseBool(val)
	if err != nil {
		return zero, fmt.Errorf("invalid bool value %q (expected true or false): %w", val, err)
	}
	if op != EQ {
		return zero, fmt.Errorf("operator %q not allowed on bool field — only == is supported", op)
	}
	if f.EQ == nil {
		return zero, fmt.Errorf("operator == not configured on this bool field")
	}
	return f.EQ(b), nil
}

// FIQLEnum handles FIQL filtering for enum fields.
// Predicates are pre-built at generation time; lookups are O(1) map access.
type FIQLEnum[P Predicate] struct {
	EQ  map[string]P
	NEQ map[string]P
}

func (f FIQLEnum[P]) apply(op FIQLOp, val string) (P, error) {
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
	default:
		return zero, fmt.Errorf("operator %q not allowed on enum field — only == and != are supported", op)
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
	p := &fiqlParser[P]{expr: expr, fields: fields}
	return p.parse()
}

// maxFIQLDepth is the maximum nesting depth for parenthesised groups.
// Prevents stack overflow on maliciously crafted expressions.
const maxFIQLDepth = 50

// fiqlParser is a recursive descent FIQL parser.
type fiqlParser[P Predicate] struct {
	expr   string
	pos    int
	depth  int
	fields FIQLFields[P]
}

func (p *fiqlParser[P]) parse() (P, error) {
	var zero P
	if p.expr == "" {
		return zero, fmt.Errorf("empty FIQL expression")
	}
	pred, err := p.parseOrExpr()
	if err != nil {
		return zero, err
	}
	if p.pos != len(p.expr) {
		return zero, fmt.Errorf("unexpected character at position %d: %q", p.pos, string(p.expr[p.pos]))
	}
	return pred, nil
}

func (p *fiqlParser[P]) parseOrExpr() (P, error) {
	left, err := p.parseAndExpr()
	if err != nil {
		return left, err
	}

	preds := []P{left}
	for p.pos < len(p.expr) && p.expr[p.pos] == ',' {
		p.pos++ // consume ','
		right, err := p.parseAndExpr()
		if err != nil {
			return right, err
		}
		preds = append(preds, right)
	}

	if len(preds) == 1 {
		return preds[0], nil
	}
	return orPreds(preds...), nil
}

func (p *fiqlParser[P]) parseAndExpr() (P, error) {
	left, err := p.parseAtom()
	if err != nil {
		return left, err
	}

	preds := []P{left}
	for p.pos < len(p.expr) && p.expr[p.pos] == ';' {
		p.pos++ // consume ';'
		right, err := p.parseAtom()
		if err != nil {
			return right, err
		}
		preds = append(preds, right)
	}

	if len(preds) == 1 {
		return preds[0], nil
	}
	return andPreds(preds...), nil
}

func (p *fiqlParser[P]) parseAtom() (P, error) {
	var zero P
	if p.pos < len(p.expr) && p.expr[p.pos] == '(' {
		p.depth++
		if p.depth > maxFIQLDepth {
			return zero, fmt.Errorf("expression exceeds maximum nesting depth of %d", maxFIQLDepth)
		}
		p.pos++ // consume '('
		pred, err := p.parseOrExpr()
		if err != nil {
			return pred, err
		}
		if p.pos >= len(p.expr) || p.expr[p.pos] != ')' {
			return zero, fmt.Errorf("expected ')' at position %d", p.pos)
		}
		p.pos++ // consume ')'
		p.depth--
		return pred, nil
	}
	return p.parseComparison()
}

func (p *fiqlParser[P]) parseComparison() (P, error) {
	var zero P

	selector := p.readSelector()
	if selector == "" {
		return zero, fmt.Errorf("expected field name at position %d", p.pos)
	}

	op, err := p.readOp()
	if err != nil {
		return zero, err
	}

	value := p.readValue()
	if value == "" {
		return zero, fmt.Errorf("empty value for field %q at position %d", selector, p.pos)
	}

	fieldDesc, ok := p.fields[selector]
	if !ok {
		return zero, fmt.Errorf("unknown field %q — annotate with entdomain.FIQL(...) to enable", selector)
	}

	pred, err := fieldDesc.apply(op, value)
	if err != nil {
		return zero, fmt.Errorf("field %q: %w", selector, err)
	}
	return pred, nil
}

func (p *fiqlParser[P]) readSelector() string {
	start := p.pos
	for p.pos < len(p.expr) {
		c := p.expr[p.pos]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
			p.pos++
		} else {
			break
		}
	}
	return p.expr[start:p.pos]
}

func (p *fiqlParser[P]) readOp() (FIQLOp, error) {
	if p.pos+2 <= len(p.expr) {
		if p.expr[p.pos:p.pos+2] == "==" {
			p.pos += 2
			return EQ, nil
		}
		if p.expr[p.pos:p.pos+2] == "!=" {
			p.pos += 2
			return NEQ, nil
		}
	}
	if p.pos < len(p.expr) && p.expr[p.pos] == '=' {
		// Extended operator of the form =xxx= — find the closing '='
		rest := p.expr[p.pos+1:]
		end := strings.Index(rest, "=")
		if end < 0 {
			return "", fmt.Errorf("malformed operator at position %d", p.pos)
		}
		opStr := FIQLOp(p.expr[p.pos : p.pos+1+end+1])
		p.pos += len(opStr)
		switch opStr {
		case GT, LT, GTE, LTE, Contains, HasPrefix:
			return opStr, nil
		default:
			return "", fmt.Errorf("unknown operator %q at position %d", opStr, p.pos-len(opStr))
		}
	}
	return "", fmt.Errorf("expected operator at position %d", p.pos)
}

func (p *fiqlParser[P]) readValue() string {
	start := p.pos
	for p.pos < len(p.expr) {
		c := p.expr[p.pos]
		if c == ';' || c == ',' || c == ')' {
			break
		}
		p.pos++
	}
	return p.expr[start:p.pos]
}

// andPreds combines multiple predicates with AND.
func andPreds[P Predicate](ps ...P) P {
	return P(sql.AndPredicates(ps...))
}

// orPreds combines multiple predicates with OR.
func orPreds[P Predicate](ps ...P) P {
	return P(sql.OrPredicates(ps...))
}

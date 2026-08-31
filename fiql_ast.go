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
	"errors"
	"fmt"
	"strings"
)

// FIQLNode is a node in a parsed FIQL expression tree.
//
// The interface is sealed by the unexported isFIQLNode method: only
// *FIQLAnd, *FIQLOr, and *FIQLCmp implement it. Sealing keeps the node set
// closed so adding a node type later stays non-breaking — no code outside
// this package can be switching exhaustively over an interface it cannot
// implement.
type FIQLNode interface {
	isFIQLNode()
}

// FIQLAnd is a conjunction of two or more child nodes (FIQL ';').
type FIQLAnd struct {
	Nodes []FIQLNode
}

// FIQLOr is a disjunction of two or more child nodes (FIQL ',').
type FIQLOr struct {
	Nodes []FIQLNode
}

// FIQLCmp is a single comparison term such as name==john or age=gt=25.
//
// Values carry the raw, uncoerced text exactly as it appeared in the
// expression. Coercion to int, float, time.Time, or uuid.UUID happens in
// CompileFIQL against the field registry — never here. Keeping the AST
// untyped is what lets rewriters edit values without the node needing an
// any-typed field.
//
// Value holds the operand for scalar operators. Values holds the operands
// for In and NotIn, already split on ',' and stripped of the enclosing
// parens; Value is empty for those two operators and Values is nil for
// every other operator. Both are empty for IsNull and NotNull.
type FIQLCmp struct {
	Field  string
	Op     FIQLOp
	Value  string
	Values []string
}

func (*FIQLAnd) isFIQLNode() {}
func (*FIQLOr) isFIQLNode()  {}
func (*FIQLCmp) isFIQLNode() {}

// ParseFIQLExpr parses a FIQL expression into an AST without consulting a
// field registry. It validates syntax, operator spelling, nesting depth, and
// value-list length; it does not validate that a field exists or that an
// operator is allowed on it. Those checks belong to CompileFIQL, which owns
// the registry.
//
// The returned tree can be inspected with FindFIQL, rewritten with
// WalkFIQL, and turned into an ent predicate with CompileFIQL. ParseFIQL is
// the composition of this function and CompileFIQL.
//
// Error ordering across fault classes is not a contract. An expression with
// both a syntax fault and an unknown field now reports the syntax fault
// first, because field resolution has moved to a later pass. The message
// text for each individual fault is unchanged.
func ParseFIQLExpr(expr string) (FIQLNode, error) {
	p := &fiqlExprParser{expr: expr}
	return p.parse()
}

// fiqlExprParser is a recursive descent FIQL parser producing an AST. It
// carries no type parameter and no registry — the syntax of FIQL does not
// depend on either.
type fiqlExprParser struct {
	expr  string
	pos   int
	depth int
}

func (p *fiqlExprParser) parse() (FIQLNode, error) {
	if p.expr == "" {
		return nil, fmt.Errorf("empty FIQL expression")
	}
	node, err := p.parseOrExpr()
	if err != nil {
		return nil, err
	}
	if p.pos != len(p.expr) {
		return nil, fmt.Errorf("unexpected character at position %d: %q", p.pos, string(p.expr[p.pos]))
	}
	return node, nil
}

func (p *fiqlExprParser) parseOrExpr() (FIQLNode, error) {
	left, err := p.parseAndExpr()
	if err != nil {
		return nil, err
	}

	nodes := []FIQLNode{left}
	for p.pos < len(p.expr) && p.expr[p.pos] == ',' {
		p.pos++ // consume ','
		right, err := p.parseAndExpr()
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, right)
	}

	if len(nodes) == 1 {
		return nodes[0], nil
	}
	return &FIQLOr{Nodes: nodes}, nil
}

func (p *fiqlExprParser) parseAndExpr() (FIQLNode, error) {
	left, err := p.parseAtom()
	if err != nil {
		return nil, err
	}

	nodes := []FIQLNode{left}
	for p.pos < len(p.expr) && p.expr[p.pos] == ';' {
		p.pos++ // consume ';'
		right, err := p.parseAtom()
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, right)
	}

	if len(nodes) == 1 {
		return nodes[0], nil
	}
	return &FIQLAnd{Nodes: nodes}, nil
}

func (p *fiqlExprParser) parseAtom() (FIQLNode, error) {
	if p.pos < len(p.expr) && p.expr[p.pos] == '(' {
		p.depth++
		if p.depth > maxFIQLDepth {
			return nil, fmt.Errorf("expression exceeds maximum nesting depth of %d", maxFIQLDepth)
		}
		p.pos++ // consume '('
		node, err := p.parseOrExpr()
		if err != nil {
			return nil, err
		}
		if p.pos >= len(p.expr) || p.expr[p.pos] != ')' {
			return nil, fmt.Errorf("expected ')' at position %d", p.pos)
		}
		p.pos++ // consume ')'
		p.depth--
		return node, nil
	}
	return p.parseComparison()
}

func (p *fiqlExprParser) parseComparison() (FIQLNode, error) {
	selector := p.readSelector()
	if selector == "" {
		return nil, fmt.Errorf("expected field name at position %d", p.pos)
	}

	op, err := p.readOp()
	if err != nil {
		return nil, err
	}

	// In/NotIn take a parenthesised list; every other operator takes a bare
	// scalar. Splitting the list here rather than in apply moves the
	// maxFIQLListValues bound to parse time, so a hostile list is rejected
	// before any registry lookup happens.
	if op == In || op == NotIn {
		raw, err := p.readListValue()
		if err != nil {
			return nil, err
		}
		values, err := parseInListValue(raw)
		if err != nil {
			return nil, err
		}
		return &FIQLCmp{Field: selector, Op: op, Values: values}, nil
	}

	value := p.readValue()
	if value == "" {
		return nil, fmt.Errorf("empty value for field %q at position %d", selector, p.pos)
	}

	// Normalize the wire-form Is op into IsNull/NotNull based on its value.
	// Downstream consumers — CompileFIQL, WalkFIQL callbacks — only ever see
	// IsNull/NotNull, never Is.
	if op == Is {
		switch value {
		case "null":
			op = IsNull
			value = ""
		case "notnull":
			op = NotNull
			value = ""
		default:
			return nil, fmt.Errorf("unknown =is= value %q — valid: %s", value, strings.Join(validIsValues, ", "))
		}
	}

	return &FIQLCmp{Field: selector, Op: op, Value: value}, nil
}

func (p *fiqlExprParser) readSelector() string {
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

func (p *fiqlExprParser) readOp() (FIQLOp, error) {
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
		case GT, LT, GTE, LTE, Contains, HasPrefix, In, NotIn, Is:
			return opStr, nil
		default:
			return "", fmt.Errorf("unknown operator %q at position %d", opStr, p.pos-len(opStr))
		}
	}
	return "", fmt.Errorf("expected operator at position %d", p.pos)
}

func (p *fiqlExprParser) readValue() string {
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

// readListValue reads a parenthesised value list for =in= / =out= operators.
// Returns the substring including the enclosing parens (e.g. "(a,b,c)").
func (p *fiqlExprParser) readListValue() (string, error) {
	if p.pos >= len(p.expr) || p.expr[p.pos] != '(' {
		return "", fmt.Errorf("expected '(' after =in=/=out= operator at position %d", p.pos)
	}
	start := p.pos
	p.pos++ // consume '('
	for p.pos < len(p.expr) && p.expr[p.pos] != ')' {
		p.pos++
	}
	if p.pos >= len(p.expr) {
		return "", fmt.Errorf("unterminated list value starting at position %d", start)
	}
	p.pos++ // consume ')'
	return p.expr[start:p.pos], nil
}

// WalkFIQL rebuilds a FIQL tree, handing every comparison node to fn and
// using whatever fn returns in its place. It is the single mechanism behind
// authorization, value transformation, and term pruning:
//
//   - return the node (edited or not) to keep it
//   - return a different node — including an *FIQLAnd or *FIQLOr — to expand it
//   - return nil to prune the term
//   - return an error to reject the whole expression
//
// fn receives a copy of each *FIQLCmp with its Values slice cloned, so
// editing a value in place cannot reach the tree the caller parsed. That
// matters when the original expression still has to be audit-logged while
// the query runs on the rewritten one.
//
// Pruning propagates: an And/Or whose children all prune returns nil to its
// own parent, and a fully pruned tree returns nil. CompileFIQL rejects a nil
// node rather than treating it as match-everything.
//
// That guarantee is narrow, and authorization callers should not mistake it
// for more. It only says a *fully* pruned tree cannot become match-all.
// Pruning a single conjunct still widens the result — dropping org_id==x from
// "org_id==x;status==active" leaves "status==active", which matches more rows,
// with no error anywhere. To restrict what a caller may filter on, reject the
// term by returning an error, or add an independent scope by wrapping the tree
// in an FIQLAnd. Do not prune conjuncts.
//
// An And/Or left with exactly one surviving child collapses to that child,
// matching the shape ParseFIQLExpr produces for a single-term expression.
func WalkFIQL(n FIQLNode, fn func(*FIQLCmp) (FIQLNode, error)) (FIQLNode, error) {
	return walkNode(n, fn, 0)
}

func walkNode(n FIQLNode, fn func(*FIQLCmp) (FIQLNode, error), depth int) (FIQLNode, error) {
	if depth > maxFIQLDepth {
		return nil, errFIQLDepth
	}
	switch v := n.(type) {
	case nil:
		return nil, nil
	case *FIQLCmp:
		if v == nil {
			return nil, errNilFIQLNode
		}
		return fn(copyCmp(v))
	case *FIQLAnd:
		if v == nil {
			return nil, errNilFIQLNode
		}
		// Zero children on input is malformed, and CompileFIQL and ToFIQL both
		// reject it. Without this guard a no-op walk would fold the empty
		// compound into nil, drop it from its parent, and hand back a tree that
		// compiles — laundering a malformed AST into an accepted one. Note this
		// is the input count; zero *survivors* after pruning is the intended
		// prune-to-nil path handled below.
		if len(v.Nodes) == 0 {
			return nil, fmt.Errorf("cannot walk an FIQLAnd with no children")
		}
		kids, err := walkChildren(v.Nodes, fn, depth+1)
		if err != nil {
			return nil, err
		}
		switch len(kids) {
		case 0:
			return nil, nil
		case 1:
			return kids[0], nil
		default:
			return &FIQLAnd{Nodes: kids}, nil
		}
	case *FIQLOr:
		if v == nil {
			return nil, errNilFIQLNode
		}
		if len(v.Nodes) == 0 {
			return nil, fmt.Errorf("cannot walk an FIQLOr with no children")
		}
		kids, err := walkChildren(v.Nodes, fn, depth+1)
		if err != nil {
			return nil, err
		}
		switch len(kids) {
		case 0:
			return nil, nil
		case 1:
			return kids[0], nil
		default:
			return &FIQLOr{Nodes: kids}, nil
		}
	default:
		return nil, fmt.Errorf("unknown FIQL node type %T", n)
	}
}

// walkChildren walks each child and drops the ones that pruned to nil.
func walkChildren(nodes []FIQLNode, fn func(*FIQLCmp) (FIQLNode, error), depth int) ([]FIQLNode, error) {
	kids := make([]FIQLNode, 0, len(nodes))
	for _, child := range nodes {
		out, err := walkNode(child, fn, depth)
		if err != nil {
			return nil, err
		}
		if out != nil {
			kids = append(kids, out)
		}
	}
	return kids, nil
}

// FindFIQL returns every comparison naming field, in left-to-right source
// order. Use it to inspect what a caller filtered on — reading the values of
// an =in= term, metering by field, or building a cache key.
//
// Each returned node is a copy, so the result is safe to read and safe to
// scribble on; neither reaches the source tree. Rewriting goes through
// WalkFIQL, which is also why inspection does not: a read-only caller using
// WalkFIQL has to remember to return the node, and an accidental nil silently
// drops the term.
func FindFIQL(n FIQLNode, field string) []*FIQLCmp {
	var out []*FIQLCmp
	collectCmp(n, field, &out, 0)
	return out
}

func collectCmp(n FIQLNode, field string, out *[]*FIQLCmp, depth int) {
	// FindFIQL has no error channel, so an over-deep or cyclic branch is
	// simply not descended into rather than reported.
	if depth > maxFIQLDepth {
		return
	}
	switch v := n.(type) {
	case *FIQLCmp:
		if v == nil {
			return
		}
		if v.Field == field {
			*out = append(*out, copyCmp(v))
		}
	case *FIQLAnd:
		if v == nil {
			return
		}
		for _, child := range v.Nodes {
			collectCmp(child, field, out, depth+1)
		}
	case *FIQLOr:
		if v == nil {
			return
		}
		for _, child := range v.Nodes {
			collectCmp(child, field, out, depth+1)
		}
	}
}

// copyCmp returns a shallow copy with Values cloned. Field, Op, and Value are
// strings and need no defensive copy; Values is the only shared mutable state.
func copyCmp(c *FIQLCmp) *FIQLCmp {
	out := *c
	if c.Values != nil {
		out.Values = append([]string(nil), c.Values...)
	}
	return &out
}

// fiqlReservedInValue lists the characters a scalar or list operand cannot
// contain. They terminate a value in the grammar and FIQL has no escape
// syntax, so a value holding one cannot be rendered and read back.
//
// '(' is deliberately absent: readValue and readListValue only break on these
// three, so a value such as "foo(bar" round-trips unharmed.
const fiqlReservedInValue = ";,)"

// fiqlReservedInListValue is the reserved set for an operand inside an
// =in= / =out= list. It is narrower than fiqlReservedInValue: readListValue
// scans to the closing paren without treating ';' as a terminator, so
// ids=in=(a;b,c) parses to the operands ["a;b", "c"] and must render back.
// Only the element separator and the list terminator are special here.
const fiqlReservedInListValue = ",)"

// errNilFIQLNode is returned when a typed-nil node pointer reaches a
// traversal. A type switch matches *FIQLCmp(nil) on its concrete case rather
// than on `case nil`, so without this guard the next field access panics.
var errNilFIQLNode = errors.New("nil FIQL node")

// errFIQLDepth bounds every public traversal. The parser enforces
// maxFIQLDepth while building a tree, but a hand-assembled or rewritten AST
// never went through the parser — and the exported Nodes slice makes a
// self-referential graph expressible (a.Nodes = []FIQLNode{a}). Unbounded
// recursion on one is a fatal stack overflow, which no caller can recover
// from, so every entry point counts depth of its own.
var errFIQLDepth = fmt.Errorf("FIQL node tree exceeds maximum nesting depth of %d", maxFIQLDepth)

// ToFIQL renders a FIQL AST back to its wire form. Use it to audit-log the
// expression a query actually ran on after WalkFIQL rewrote it, to build a
// cache key, or to forward a scoped filter to another service.
//
// Rendering is canonical, not byte-exact. The AST does not record redundant
// grouping, so "((a==1))" parses to a bare comparison and renders as "a==1";
// WalkFIQL likewise collapses a compound left with a single surviving child.
// What ToFIQL guarantees is semantic: the output parses back to a tree that
// compiles to the same predicate, and rendering is idempotent from the first
// pass onward. Input already in canonical form renders byte-identical.
//
// Parentheses are emitted only where an FIQLOr sits inside an FIQLAnd, the
// single place precedence would otherwise change the meaning.
//
// ToFIQL returns an error instead of emitting text that would parse back into
// a different tree. A rewriter can place a value the grammar cannot express —
// setting a value to "a,b==c" would render as name==a,b==c and read back as
// two OR'd comparisons, a different query with no error anywhere. Refusing at
// render time keeps that failure loud and local. The restriction disappears on
// its own if value quoting is ever added to the grammar.
func ToFIQL(n FIQLNode) (string, error) {
	var sb strings.Builder
	if err := writeFIQL(&sb, n, 0); err != nil {
		return "", err
	}
	return sb.String(), nil
}

func writeFIQL(sb *strings.Builder, n FIQLNode, depth int) error {
	if depth > maxFIQLDepth {
		return errFIQLDepth
	}
	switch v := n.(type) {
	case nil:
		return fmt.Errorf("empty FIQL expression")
	case *FIQLAnd:
		if v == nil {
			return errNilFIQLNode
		}
		if len(v.Nodes) == 0 {
			return fmt.Errorf("cannot render an FIQLAnd with no children")
		}
		// A one-child compound carries no grouping information: rendering it
		// as a group would emit parens that the parser then discards, so the
		// second render would differ from the first and idempotence would
		// break. Collapse to the child, matching what WalkFIQL already does.
		if len(v.Nodes) == 1 {
			return writeFIQL(sb, v.Nodes[0], depth+1)
		}
		for i, child := range v.Nodes {
			if i > 0 {
				sb.WriteByte(';')
			}
			// An Or inside an And is the only shape needing parens: ';' binds
			// tighter than ',', so a bare Or child would rebind on reparse.
			// The test follows single-child collapse, because a compound that
			// collapses to a disjunction still renders as one.
			needsParens := rendersAsDisjunction(child)
			if needsParens {
				sb.WriteByte('(')
			}
			if err := writeFIQL(sb, child, depth+1); err != nil {
				return err
			}
			if needsParens {
				sb.WriteByte(')')
			}
		}
		return nil
	case *FIQLOr:
		if v == nil {
			return errNilFIQLNode
		}
		if len(v.Nodes) == 0 {
			return fmt.Errorf("cannot render an FIQLOr with no children")
		}
		if len(v.Nodes) == 1 {
			return writeFIQL(sb, v.Nodes[0], depth+1)
		}
		for i, child := range v.Nodes {
			if i > 0 {
				sb.WriteByte(',')
			}
			if err := writeFIQL(sb, child, depth+1); err != nil {
				return err
			}
		}
		return nil
	case *FIQLCmp:
		if v == nil {
			return errNilFIQLNode
		}
		return writeFIQLCmp(sb, v)
	default:
		return fmt.Errorf("unknown FIQL node type %T", n)
	}
}

// effectiveNode follows single-child compounds down to the node that actually
// gets rendered. A one-child FIQLAnd or FIQLOr emits nothing of its own, so
// precedence has to be judged against what survives the collapse.
//
// The walk is bounded because it is iterative, not recursive: writeFIQL's own
// depth guard never fires here, and a one-child compound pointing at itself
// would otherwise spin a CPU core forever. On hitting the bound it returns the
// node it reached — the caller's writeFIQL recursion then trips its depth
// guard and reports the malformed tree.
func effectiveNode(n FIQLNode) FIQLNode {
	for i := 0; i <= maxFIQLDepth; i++ {
		switch v := n.(type) {
		case *FIQLAnd:
			if v == nil || len(v.Nodes) != 1 {
				return n
			}
			n = v.Nodes[0]
		case *FIQLOr:
			if v == nil || len(v.Nodes) != 1 {
				return n
			}
			n = v.Nodes[0]
		default:
			return n
		}
	}
	return n
}

// rendersAsDisjunction reports whether n ultimately emits a ',' at its top
// level, which is the only case an enclosing And has to parenthesise.
func rendersAsDisjunction(n FIQLNode) bool {
	v, ok := effectiveNode(n).(*FIQLOr)
	return ok && v != nil && len(v.Nodes) > 1
}

func writeFIQLCmp(sb *strings.Builder, c *FIQLCmp) error {
	if err := checkFIQLSelector(c.Field); err != nil {
		return err
	}
	if !knownFIQLOp(c.Op) {
		return fmt.Errorf("field %q: operator %q cannot be rendered — not one of the FIQL operators", c.Field, c.Op)
	}

	switch c.Op {
	case IsNull, NotNull:
		// The constants already hold the complete wire form ("=is=null"), so
		// the operand is part of the operator and nothing else is emitted.
		sb.WriteString(c.Field)
		sb.WriteString(string(c.Op))
		return nil

	case In, NotIn:
		if len(c.Values) == 0 {
			return fmt.Errorf("field %q: operator %q has no values to render", c.Field, c.Op)
		}
		// A rewriter can grow Values past the parser's bound. Emitting them
		// would produce text ParseFIQLExpr rejects, so the round-trip
		// guarantee has to be enforced on the way out as well as in.
		if len(c.Values) > maxFIQLListValues {
			return fmt.Errorf("field %q: value list of %d exceeds maximum of %d entries", c.Field, len(c.Values), maxFIQLListValues)
		}
		for _, val := range c.Values {
			if err := checkFIQLListValue(c.Field, val); err != nil {
				return err
			}
		}
		sb.WriteString(c.Field)
		sb.WriteString(string(c.Op))
		sb.WriteByte('(')
		for i, val := range c.Values {
			if i > 0 {
				sb.WriteByte(',')
			}
			sb.WriteString(val)
		}
		sb.WriteByte(')')
		return nil

	default:
		if err := checkFIQLValue(c.Field, c.Value); err != nil {
			return err
		}
		sb.WriteString(c.Field)
		sb.WriteString(string(c.Op))
		sb.WriteString(c.Value)
		return nil
	}
}

// checkFIQLValue rejects operands the grammar cannot carry back.
func checkFIQLValue(field, val string) error {
	if val == "" {
		return fmt.Errorf("field %q: empty value cannot be rendered as FIQL", field)
	}
	if i := strings.IndexAny(val, fiqlReservedInValue); i >= 0 {
		return fmt.Errorf("field %q: value %q contains reserved character %q, which FIQL cannot escape", field, val, string(val[i]))
	}
	return nil
}

// checkFIQLListValue rejects operands an =in= / =out= list cannot carry back.
// ';' is deliberately absent from the rejected set — see fiqlReservedInListValue.
// An empty element is deliberately allowed: the parser splits
// ids=in=(a,,b) into ["a", "", "b"], and rendering that text reparses
// identically. Rejecting it here would refuse a tree ParseFIQLExpr produced.
// The whole list still may not be empty — writeFIQLCmp checks len(Values).
func checkFIQLListValue(field, val string) error {
	if i := strings.IndexAny(val, fiqlReservedInListValue); i >= 0 {
		return fmt.Errorf("field %q: value %q contains reserved character %q, which FIQL cannot escape", field, val, string(val[i]))
	}
	return nil
}

// checkFIQLSelector mirrors readSelector's accepted character set. A field
// name outside it would render into text that reads back as a different
// selector, or fails to parse at all.
func checkFIQLSelector(field string) error {
	if field == "" {
		return fmt.Errorf("comparison has an empty field name")
	}
	for i := 0; i < len(field); i++ {
		c := field[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
			continue
		}
		return fmt.Errorf("field %q contains character %q, which the FIQL selector grammar does not accept", field, string(c))
	}
	return nil
}

// knownFIQLOp reports whether op is one of the renderable operators. opByName
// is the registry TestOpRegistryCovered already pins, and it deliberately
// excludes the parser-internal Is — which normalizes to IsNull/NotNull during
// parsing and must never appear in a tree.
func knownFIQLOp(op FIQLOp) bool {
	for _, registered := range opByName {
		if registered == op {
			return true
		}
	}
	return false
}

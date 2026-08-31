# Scope: FIQL parse/compile split

| Field    | Value                |
|----------|----------------------|
| Status   | Accepted             |
| Created  | 2026-08-31           |
| Author   | danhtran94           |

## Problem

`ParseFIQL` (`fiql.go:535`) folds a FIQL expression directly into an ent
predicate. Nothing observable survives the call.

`parseComparison` (`fiql.go:637`) computes `selector`, `op`, and `value` as
local variables, passes them to `fieldDesc.apply`, and returns only `P`.
`andPreds` / `orPreds` (`fiql.go:867`, `fiql.go:872`) wrap children in
`sql.AndPredicates` / `sql.OrPredicates`, which are opaque closures — the
AND/OR tree shape is gone the moment it is built.

Coerced values are lost one level deeper. In `FIQLTime.apply`
(`fiql.go:319`) the `time.Time` from `time.Parse` exists only inside the
closure `f.GTE(t)`. Same for `strconv.Atoi` in `FIQLInt.apply` and
`uuid.Parse` in `FIQLUUID.apply`.

There is no user-land workaround, by construction rather than omission:
`FIQLField.apply` is an **unexported method** (per A1), so no package
outside `entdomain` can implement or wrap `FIQLField` to observe values in
flight.

⚠ The only readback available today is executing the predicate against a
throwaway `sql.Selector` and reading `q, args := s.Query()` — the technique
`buildSQL` uses in `fiql_test.go:39`. It yields SQL text and flat args, not
`field/op/value`; enum terms have already been mapped to ent constants; and
AND cannot be distinguished from OR without parsing SQL. Adequate for tests,
unusable as a mechanism.

This blocks four concrete needs:

| Need | Blocked because |
|---|---|
| **Authorization** | Cannot reject or rewrite a term naming a field the caller may not filter on |
| **Value transformation** | Cannot normalise a value (case-folding, trimming, unit conversion) before it reaches the registry |
| **Term filtering** | Cannot drop a term and keep the rest of the expression |
| **Inspection** | Cannot log, meter, or cache-key what was filtered on |

The parser needs the registry at exactly one line — `fieldDesc.apply` at
`fiql.go:684` (per A2). Everything above it is pure syntax. Two
responsibilities are fused into one pass with no technical reason.

## Assumptions

- **A1 [VERIFIED]:** `FIQLField.apply` is an unexported method, sealing the
  interface against external implementations. Evidence: `fiql.go:116-118`
  declares `apply(op FIQLOp, value string) (P, error)` with a lowercase
  name; all six implementations (`FIQLString`, `FIQLInt`, `FIQLFloat`,
  `FIQLTime`, `FIQLBool`, `FIQLUUID`, `FIQLEnum`) are in-package.
- **A2 [VERIFIED]:** The parser consults `FIQLFields` at exactly one call
  site. Evidence: `grep -n "p.fields" fiql.go` returns only
  `fiql.go:681` (map lookup) and `fiql.go:684` (`fieldDesc.apply`), both
  inside `parseComparison`.
- **A3 [VERIFIED]:** `maxFIQLListValues` is enforced at apply time, not
  parse time. Evidence: `fiql.go:778` sits in `parseInListValue`, which is
  reached only from `applyListTyped` (`fiql.go:797`) and the per-type
  `apply` methods.
- **A4 [VERIFIED]:** The only non-test caller of `ParseFIQL` is generated
  code. Evidence: `template/fiql.tmpl:115` emits
  `return entdomain.ParseFIQL(expr, {{ $n.Name }}FIQLFields)`; the sole
  materialised instance is `examples/basic/ent/fiql.go:60`.
- **A5 [EXTERNAL FACT]:** Go does not permit type parameters on methods.
  Evidence: [Go spec, Method declarations](https://go.dev/ref/spec#Method_declarations).
  This forces the compile step to be a free function taking `FIQLNode`,
  not a method on `FIQLNode`.

## Success Criteria

1. `entdomain.ParseFIQLExpr(expr string) (FIQLNode, error)` is declared in
   `fiql_ast.go` and its signature takes no `FIQLFields` argument.
2. `entdomain.CompileFIQL[P Predicate](n FIQLNode, fields FIQLFields[P]) (P, error)`
   is declared in `fiql.go`.
3. `ParseFIQL` in `fiql.go` retains the exact signature
   `func ParseFIQL[P Predicate](expr string, fields FIQLFields[P]) (P, error)`
   and its body calls `ParseFIQLExpr` followed by `CompileFIQL`.
4. `git diff --exit-code template/fiql.tmpl examples/basic/ent/fiql.go`
   exits 0 after `make gen`.
5. All 23 existing test functions in `fiql_test.go` (20) and
   `fiql_internal_test.go` (3) pass with no edits to their bodies.
6. `go test -run TestFIQLErrorMessagesUnchanged ./...` passes, where that
   test in `fiql_test.go` asserts byte-identical error strings for these
   six inputs: `""`, `"name"`, `"name==a;b"`, `"unknown==x"`,
   `"age=gt=abc"`, `"status=is=maybe"`.
7. `entdomain.WalkFIQL(n FIQLNode, fn func(*FIQLCmp) (FIQLNode, error)) (FIQLNode, error)`
   is declared in `fiql_ast.go`.
8. When `fn` returns `nil` for every `*FIQLCmp` in the tree, `WalkFIQL`
   returns `nil, nil`.
9. When `fn` returns `nil` for a strict subset of an `FIQLAnd`'s children,
   `WalkFIQL` returns an `FIQLAnd` holding exactly the surviving children.
10. `CompileFIQL(nil, fields)` returns a non-nil error whose message is
    `empty FIQL expression`.
11. `ParseFIQLExpr(strings.Repeat("(", 51) + "a==1" + strings.Repeat(")", 51))`
    returns a non-nil error — depth is enforced without a registry.
12. `ParseFIQLExpr("a=in=(" + 101 comma-separated values + ")")` returns a
    non-nil error — list length is enforced without a registry, closing A3.
13. `FIQLNode` declares an unexported method so that a type declared
    outside package `entdomain` cannot satisfy it; verified by
    `TestFIQLNodeSealed` in `fiql_test.go` (package `entdomain_test`).
    An internal test cannot verify this — inside package `entdomain` a
    type can implement the unexported method.
14. `fiql_test.go` contains `TestWalkFIQLAuthzInjection`,
    `TestWalkFIQLValueTransform`, and `TestWalkFIQLTermFilter`, each
    asserting the resulting SQL via the existing `buildSQL` helper.
15. `entdomain.FindFIQL(n FIQLNode, field string) []*FIQLCmp` is declared
    in `fiql_ast.go` and returns the comparison nodes naming `field`, in
    left-to-right source order.
16. `TestFindFIQLInValues` in `fiql_test.go` asserts that
    `FindFIQL(ParseFIQLExpr("ids=in=(abc,xyz,mnz)"), "ids")[0].Values`
    equals `[]string{"abc", "xyz", "mnz"}`.
17. `TestWalkFIQLDoesNotMutateSource` in `fiql_test.go` asserts that after
    a `WalkFIQL` callback assigns to `c.Values[0]`, the node returned by
    the original `ParseFIQLExpr` call still holds the pre-walk value.
18. `TestFIQLInPrefixRoundTrip` in `fiql_test.go` parses
    `ids=in=(abc,xyz,mnz)`, prefixes each value with `id-` via `WalkFIQL`,
    compiles with `CompileFIQL`, and asserts the `buildSQL` args equal
    `[id-abc id-xyz id-mnz]`.
19. `make build`, `make test`, and `make gen` all exit 0 with a clean
    `git status --porcelain`.

## Out of Scope

- **Edge / relation filtering** (`owner.name==john`) — `template/fiql.tmpl:31`
  skips `IsEdgeField` deliberately; unbounded join depth is a separate
  DoS surface needing its own ADR.
- **Value quoting and percent-escape handling** — the `name==a%3Bb` silent
  mismatch is a parser grammar change; separate scope note.
- **Case-insensitive operators** (`ContainsFold`, `EqualFold`) — additive
  operator work, independent of this split.
- **Date-only time values** (`created_at=ge=2024-01-15`) — needs a
  boundary-semantics decision, unrelated to AST shape.
- **Exposing coerced Go values on the AST** — `FIQLCmp` carries raw strings
  only; typing stays in compile, which avoids `any` in the public surface.
- **Sort and pagination generation** — not FIQL; separate feature.

## Risks

- **Pruning to an empty tree fails open.** A `WalkFIQL` that drops every
  term yields `nil`, and a caller who then skips `.Where()` runs an
  unfiltered query — the opposite of the intended authorization outcome.
  Mitigation: `CompileFIQL(nil, fields)` returns the error
  `empty FIQL expression` (criterion 10) so the empty tree cannot silently
  become match-all; the caller must handle it explicitly.
- **A rewriter injects a field absent from the registry.** `WalkFIQL` does
  not consult `FIQLFields`, so a synthetic `FIQLCmp` naming an
  unannotated field is only caught later. Mitigation: `CompileFIQL` keeps
  the existing unknown-field check at the current call site
  (`fiql.go:681`) and its current message, so the failure is a compile
  error with the same wording callers already handle.
- **Error ordering changes when an expression has both a syntax fault and
  an unknown field.** Today one pass reports whichever comes first
  positionally; after the split every syntax error precedes every
  unknown-field error. Mitigation: criterion 6 pins the exact message for
  each fault class in isolation; ordering across classes is declared a
  non-contract in the `ParseFIQLExpr` doc comment.
- **Publishing the AST freezes it as API.** External code depending on
  `FIQLAnd` / `FIQLOr` / `FIQLCmp` field layout makes later node types a
  breaking change. Mitigation: `FIQLNode` carries an unexported method
  (criterion 13), so no external package can implement it and adding node
  types stays non-breaking for implementers.
- **A rewriter mutating a node in place corrupts the source tree.** The
  ergonomic transform is `c.Values[i] = "id-" + v`, which would edit the
  tree returned by `ParseFIQLExpr` and destroy the caller's ability to
  audit-log the original expression while querying with the rewritten one.
  Mitigation: `WalkFIQL` passes each callback a shallow copy of the
  `FIQLCmp` with its `Values` slice cloned, so in-place edits cannot reach
  the source tree; criterion 17 verifies this.
- **Read-only inspection through `WalkFIQL` risks silent term loss.** A
  caller reading values must remember to `return c, nil`; returning `nil`
  instead drops the term with no error. Mitigation: `FindFIQL`
  (criterion 15) provides read access with no return-value contract, so
  inspection never travels through the pruning path.
- **`In` / `NotIn` list values need a representation decision.** Keeping
  the raw `(a,b,c)` string on the AST forces every rewriter to re-parse it.
  Mitigation: `FIQLCmp` carries `Values []string` for `In` / `NotIn` and
  `Value string` otherwise; `parseInListValue` moves to parse time, which
  criterion 12 verifies.

## History

| Ticket    | Title                    | Created    | Status   |
|-----------|--------------------------|------------|----------|
| (initial) | FIQL parse/compile split | 2026-08-31 | Accepted |

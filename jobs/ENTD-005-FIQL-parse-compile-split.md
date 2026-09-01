# ENTD-005-FIQL: Split FIQL parse from compile

| Field      | Value                                     |
|------------|-------------------------------------------|
| Status     | Done                                      |
| Created    | 2026-08-31                                |
| Assignee   | danhtran94                                |
| Source     | docs/scope-fiql-parse-compile-split.md     |
| Blocked by | —                                         |

<!-- approved -->
<!-- approval: user instruction "tạo job và làm đi" (2026-08-31) -->

## Goal

Split `ParseFIQL` into a registry-free syntax pass (`ParseFIQLExpr`) that
returns a public AST and a typed pass (`CompileFIQL`) that turns that AST
into an ent predicate. Add `WalkFIQL` and `FindFIQL` so callers can inspect,
authorize, transform, and prune terms between the two. `ParseFIQL` keeps its
exact signature and `template/fiql.tmpl` is untouched, so generated code
stays byte-identical.

## Problem

`ParseFIQL` folds an expression straight into a predicate. Nothing survives.

    Current:
      "ids=in=(abc,xyz,mnz)"
        → parseComparison  (selector/op/value are locals)
        → fieldDesc.apply  → predicate closure
        → andPreds/orPreds → opaque sql.AndPredicates ✗ tree shape gone

`FIQLField.apply` is unexported, so no external package can wrap it to
observe values in flight. There is no user-land workaround.

## Solution: two passes over a public AST

    After:
      "ids=in=(abc,xyz,mnz)"
        → ParseFIQLExpr → &FIQLCmp{Field:"ids", Op:In, Values:[abc xyz mnz]} ✓
        → FindFIQL / WalkFIQL  (inspect, authz, transform, prune)
        → CompileFIQL          → predicate ✓

### Components

**`FIQLNode`** — sealed interface, implemented by `FIQLAnd`, `FIQLOr`, `FIQLCmp`
- Carries raw strings only; coercion stays in compile
- Unexported method blocks external implementations

**`ParseFIQLExpr`** — syntax pass
- No `FIQLFields` argument, no type parameter
- Enforces `maxFIQLDepth` and `maxFIQLListValues`
- Normalizes `=is=` into `IsNull` / `NotNull`, splits `=in=` lists

**`CompileFIQL`** — typed pass
- Resolves fields against the registry, coerces values, builds the predicate
- Free function, not a method — Go forbids type parameters on methods

**`WalkFIQL` / `FindFIQL`** — AST → AST rewrite, and read-only lookup

### Why not alternatives

| Approach | Verdict |
|---|---|
| **Parse/compile split (chosen)** | Removes a conflation; unblocks all four needs with one AST |
| Observer callback on the single pass | Read-only; cannot rewrite or prune; no tree shape |
| `ParseFIQLWithBindings` returning `[]FIQLBinding` | Flat list loses AND/OR structure; forces `any` for coerced values |

## Workstreams

### 1. AST and syntax pass

Introduces the node types and moves the parser off the registry, unblocking
every other workstream.

| # | Task | File | Status |
|---|------|------|--------|
| 1.1 | Add `FIQLNode` sealed interface plus `FIQLAnd`, `FIQLOr`, `FIQLCmp` | `fiql_ast.go` | [x] |
| 1.2 | Port recursive-descent parser to emit AST as `ParseFIQLExpr`; drop the `P` type parameter | same | [x] |
| 1.3 | Enforce `maxFIQLDepth` and `maxFIQLListValues` at parse time | same | [x] |
| 1.4 | Split `=in=` / `=out=` payload into `FIQLCmp.Values` at parse time | same | [x] |

**Key details:** `=is=` normalization stays in the parser — the AST only
ever carries `IsNull` / `NotNull`, never the wire-form `Is`. Node structs
expose fields but the interface stays sealed via `isFIQLNode()`.

### 2. Typed compile pass

Turns the AST into a predicate and re-expresses `ParseFIQL` as a composition.

| # | Task | File | Status |
|---|------|------|--------|
| 2.1 | Change `FIQLField.apply` to `apply(op FIQLOp, val string, vals []string) (P, error)` and update all 7 implementations | `fiql.go` | [x] |
| 2.2 | Add `CompileFIQL[P Predicate](n FIQLNode, fields FIQLFields[P]) (P, error)` | same | [x] |
| 2.3 | Return error `empty FIQL expression` from `CompileFIQL(nil, ...)` | same | [x] |
| 2.4 | Rewrite `ParseFIQL` body as `ParseFIQLExpr` + `CompileFIQL`, signature unchanged | same | [x] |
| 2.5 | Delete the now-dead `fiqlParser` type and `parseInListValue` call sites | same | [x] |

**Key details:** `apply` is unexported, so widening its signature breaks no
external caller. Unknown-field detection and its message stay at the
existing call site.

### 3. Rewrite and inspection helpers

Gives callers the authz / transform / prune / inspect surface the split exists for.

| # | Task | File | Status |
|---|------|------|--------|
| 3.1 | Add `WalkFIQL(n FIQLNode, fn func(*FIQLCmp) (FIQLNode, error)) (FIQLNode, error)` with prune-on-nil | `fiql_ast.go` | [x] |
| 3.2 | Pass each callback a shallow copy of `FIQLCmp` with `Values` cloned | same | [x] |
| 3.3 | Add `FindFIQL(n FIQLNode, field string) []*FIQLCmp` in source order | same | [x] |

**Key details:** An `FIQLAnd` / `FIQLOr` whose children all prune returns
`nil` up to its parent. A fully pruned tree yields `nil, nil`, which
`CompileFIQL` then rejects — pruning must never degrade to match-all.

### 4. Testing

| # | Task | Status |
|---|------|--------|
| 4.1 | `TestFIQLErrorMessagesUnchanged` — 6 inputs, byte-identical messages | [x] |
| 4.2 | `TestFIQLNodeSealed` — external type cannot satisfy `FIQLNode` | [x] |
| 4.3 | `TestFindFIQLInValues` — `ids=in=(abc,xyz,mnz)` reads back 3 values | [x] |
| 4.4 | `TestWalkFIQLDoesNotMutateSource` — source tree intact after in-place edit | [x] |
| 4.5 | `TestFIQLInPrefixRoundTrip` — parse → prefix `id-` → compile → SQL args | [x] |
| 4.6 | `TestWalkFIQLAuthzInjection`, `TestWalkFIQLValueTransform`, `TestWalkFIQLTermFilter` | [x] |
| 4.7 | `TestParseFIQLExprLimitsWithoutRegistry` — depth 51 and list 101 both error | [x] |
| 4.8 | `TestCompileFIQLNilNode` — nil AST returns `empty FIQL expression` | [x] |

### 5. Documentation and build verification

| # | Task | Status |
|---|------|--------|
| 5.1 | Add a parse/compile subsection to README `## FIQL Filtering` | [x] |
| 5.2 | `make gen` leaves `template/fiql.tmpl` and `examples/basic/ent/fiql.go` unchanged | [x] |
| 5.3 | `make build` + `make test` + `harness validate-pr-checklist` all exit 0 | [x] |

## Design Decisions

### AST carries raw strings, not coerced values
`FIQLCmp.Value` and `.Values` stay `string` / `[]string`. Coercion to
`time.Time`, `int`, and `uuid.UUID` remains inside compile. Exposing coerced
values would force `any` into the public surface for no gain — a transform
that produces an invalid value should fail at `CompileFIQL` with the
existing type error, not silently reach SQL.

### An empty tree is an error, not match-all
`CompileFIQL(nil, fields)` returns `empty FIQL expression`. If a fully
pruned authorization rewrite compiled to match-all, a caller skipping
`.Where()` would run an unfiltered query — the exact opposite of intent.
Fail closed and make the caller decide.

### WalkFIQL copies before handing a node to the callback
The ergonomic transform is `c.Values[i] = "id-" + v`. Without copying, that
edits the tree `ParseFIQLExpr` returned, destroying the caller's ability to
audit-log the original expression while querying with the rewritten one.
AST nodes are tiny; the copy is not worth optimising away.

### Reading goes through FindFIQL, not WalkFIQL
A read-only caller using `WalkFIQL` must remember `return c, nil`; an
accidental `return nil` drops the term with no error. `FindFIQL` has no
return-value contract, so inspection never travels the pruning path.

## What Stays Unchanged

- `template/fiql.tmpl` — no template edits; generated output byte-identical
- `examples/basic/ent/fiql.go` and every other generated file
- `ParseFIQL` signature and all existing error message strings
- The `FIQLFields` map type and all 7 typed field structs' exported fields
- `annotation.go`, `kinds.go`, `generate.go` — codegen side untouched
- Operator set — no new operators; `opByName` unchanged

## Implementation Order

    1. WS1 AST + syntax pass    ← unblocks WS2 and WS3
    2. WS2 compile pass          ← depends on WS1
    3. WS3 walk/find helpers     ← depends on WS1, parallel with WS2
    4. WS4 testing               ← depends on WS2 and WS3
    5. WS5 docs + verification   ← last

## Notes

- `maxFIQLDepth = 50` and `maxFIQLListValues = 100` keep their current values; only their enforcement point moves.
- New file `fiql_ast.go` triggers `gen_triggers` (root `*.go`) in `.claude/harness.json`, so `hook-postbuild` runs `make gen` automatically.
- Error-ordering across fault classes is explicitly a non-contract; document it in the `ParseFIQLExpr` doc comment.

## Discoveries & Decisions During Implementation

### [Implementer] Widening `apply` broke an internal test's call arity
`fiql_internal_test.go:70` calls the unexported `f.apply(Is, "")` directly to
prove `FIQLString.apply` has no `Is` case. Adding the `vals []string`
parameter broke that call. The fix was mechanical — `f.apply(Is, "", nil)` —
and no assertion changed. Success criterion 5 ("no edits to their bodies")
was written assuming tests only reach the package through its exported
surface; it did not account for internal tests exercising unexported methods.

### [Implementer] The sealed-interface test cannot live in the internal test file
Criterion 13 originally named `fiql_internal_test.go`. That file is in package
`entdomain`, where a local type *can* implement `isFIQLNode()` — the test
would have proved nothing. `TestFIQLNodeSealed` went into `fiql_test.go`
(package `entdomain_test`) instead, where the seal is real. The scope note
criterion was corrected to match.

### [Implementer] List-length rejection now precedes the operator check
`applyListTyped` used to check `inFn == nil` before calling
`parseInListValue`, so `=in=` on a field without `In` reported
"operator =in= not allowed" and never validated the list. With splitting moved
to parse time, an over-long list on such a field now reports the list error
first. Both messages are unchanged; only which one wins changed. This is the
same non-contract as the syntax-vs-unknown-field ordering already documented
on `ParseFIQLExpr`, and it fails earlier on hostile input, which is the point.

### [Implementer] Pre-existing gofmt drift in three untouched files
`gofmt -l` flags `generate.go`, `template.go`, and `proto_types_test.go`, none
of which this job modified. CI's golangci-lint runs gosec, staticcheck, and
gocritic — none of which include gofmt — so the drift goes unreported. All
four files this job touched are gofmt-clean. Cleanup is out of scope here and
worth a separate one-line job.


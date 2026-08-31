# ENTD-006-FIQL: Serialize a FIQL AST back to its wire form

| Field      | Value                                   |
|------------|-----------------------------------------|
| Status     | Done                                    |
| Created    | 2026-09-01                              |
| Assignee   | danhtran94                              |
| Source     | jobs/ENTD-005-FIQL-parse-compile-split.md |
| Blocked by | —                                       |

<!-- approved -->
<!-- approval: user instruction "KISS đi" (2026-09-01) -->

## Goal

Add `ToFIQL(n FIQLNode) (string, error)` so a tree rewritten by `WalkFIQL`
can be rendered back to FIQL text — for audit-logging the expression the
query actually ran on, building cache keys, and forwarding a scoped filter to
another service. Follow-up to ENTD-005, which produced the AST but left it
one-way.

## Problem

ENTD-005 made the AST readable and rewritable but not renderable. After a
transform there is no way to recover the wire form:

    ParseFIQLExpr → AST → WalkFIQL → AST → ??? ✗

Rendering naively is not safe. A rewriter can put a value into a node that
the grammar cannot express, and one shape corrupts silently rather than
failing:

    value "a;b"     → name==a;b      reparse errors        (safe)
    value "a,b"     → name==a,b      reparse errors        (safe)
    value ""        → name==         reparse errors        (safe)
    value "a)b"     → name==a)b      reparse errors        (safe)
    value "a,b==c"  → name==a,b==c   → OR(name=="a" b=="c")  ⚠ silent

## Solution: render, but refuse what cannot round-trip

`ToFIQL` returns an error rather than emitting text that would reparse into a
different tree. That keeps the failure loud at the point of rendering instead
of surfacing as a wrong query somewhere downstream.

    ParseFIQLExpr → AST → WalkFIQL → AST → ToFIQL → "ids=in=(id-abc,id-xyz)" ✓
                                              └─→ error if unrepresentable ✓

### Components

**`ToFIQL`** — free function, matching `CompileFIQL` / `WalkFIQL` / `FindFIQL`
- Not a `String()` method: rendering can fail, and silence is the dangerous outcome
- Parenthesises exactly one case — an `FIQLOr` nested inside an `FIQLAnd`
- `IsNull` / `NotNull` need no operand: the constants already hold the full
  wire form (`=is=null`)

### Why not alternatives

| Approach | Verdict |
|---|---|
| **Render, error on unrepresentable (chosen)** | Usable now; the error case disappears for free once value quoting lands |
| Quoted values first, then render | Correct end state but changes parser grammar — separate scope |
| Percent-encode reserved chars | `r.URL.Query().Get` already decodes once, so `%` cannot be disambiguated |
| `String()` method | Cannot report failure; silent corruption is the exact risk here |

## Workstreams

### 1. Serializer

| # | Task | File | Status |
|---|------|------|--------|
| 1.1 | Add `ToFIQL(n FIQLNode) (string, error)` with the AND-wraps-OR paren rule | `fiql_ast.go` | [x] |
| 1.2 | Reject unrepresentable comparisons: empty value, value containing `;` `,` `)` | same | [x] |
| 1.3 | Reject malformed nodes: empty field, field outside `[A-Za-z0-9_]`, unknown op, `Is` op, empty And/Or, nil | same | [x] |

**Key details:** `(` is permitted in a scalar value — verified to round-trip,
since `readValue` only breaks on `;`, `,`, and `)`. `In` / `NotIn` render
their operands from `Values`, never from `Value`.

### 2. Testing

| # | Task | Status |
|---|------|--------|
| 2.1 | `TestToFIQLRoundTrip` — 6 expressions render byte-identical to their source | [x] |
| 2.2 | `TestToFIQLRejectsUnrepresentable` — the 4 unsafe values plus the silent-corruption case | [x] |
| 2.3 | `TestToFIQLRejectsMalformedNode` — nil, empty And, bad field, `Is` op | [x] |
| 2.4 | `TestToFIQLAfterWalk` — rewrite then render, asserting the emitted text | [x] |

### 3. Documentation

| # | Task | Status |
|---|------|--------|
| 3.1 | Extend README `### Inspecting and rewriting a filter` with `ToFIQL` and its error contract | [x] |
| 3.2 | `make build` + `make test` + `make gen` + `harness validate-pr-checklist` exit 0 | [x] |

## Design Decisions

### Error, not best-effort text
A renderer that emits `name==a,b==c` produces a string that parses cleanly
into a different query. Returning an error makes the one genuinely dangerous
case loud, and costs nothing for every expression that came from a parse.

### Free function, not `String()`
`String()` cannot signal failure, and the failure is precisely what must not
be swallowed. Keeping `ToFIQL` a free function also matches the shape of the
rest of the AST API.

## What Stays Unchanged

- `ParseFIQLExpr`, `CompileFIQL`, `WalkFIQL`, `FindFIQL` — no signature changes
- The AST node types and the sealed `FIQLNode` interface
- `ParseFIQL`, `template/fiql.tmpl`, and all generated output
- Parser grammar — value quoting stays out of scope

## Implementation Order

    1. WS1 serializer   ← unblocks WS2
    2. WS2 testing
    3. WS3 docs + verification

## Notes

- Rejected value characters are exactly `;`, `,`, `)` plus the empty string. `(` is allowed.
- Valid operators are the values of `opByName`; the parser-internal `Is` is not among them and must be rejected.

## Discoveries & Decisions During Implementation

### [Implementer] `opByName` doubled as the render allowlist
`knownFIQLOp` checks membership in `opByName` rather than repeating the
operator list. That registry already excludes the parser-internal `Is` and is
pinned in both directions by `TestOpRegistryCovered`, so the serializer
inherits the existing drift guard instead of adding a second list that could
fall out of sync.

### [Implementer] `(` is safe in a value and stays allowed
The reserved set is `;`, `,`, `)` — not `(`. `readValue` and `readListValue`
only terminate on those three, so `name==foo(bar` round-trips unharmed;
rejecting `(` would have refused legitimate values for no gain. A value
containing `)` is rejected, which is what actually breaks list parsing.

### [Implementer] One paren rule covers every nesting case
Only an `FIQLOr` nested inside an `FIQLAnd` needs parentheses. And-in-Or,
And-in-And, and Or-in-Or are all either associative or already bound correctly
because `;` binds tighter than `,`. Verified against `(a==1,b==2);c==3` and
`x==1;(y==2,z==3);w==4`, both of which render byte-identical to their source
despite the parser having dropped and re-derived the grouping.

### [Reviewer] Typed-nil node pointers panicked every traversal
CodeRabbit flagged it on PR #55 and it reproduced: a type switch matches
`(*FIQLCmp)(nil)` on its concrete case, not on `case nil`, so `ToFIQL`,
`CompileFIQL`, and `WalkFIQL` all dereferenced and panicked. The `case nil`
arm only catches a genuinely nil interface. Fixed with an explicit `v == nil`
guard per pointer case returning `errNilFIQLNode`; `FindFIQL` skips instead,
having no error channel. Covered by `TestFIQLTypedNilNodes`, including a
typed-nil nested inside a compound.

### [Reviewer] "Byte-identical" was an overclaim
The doc comment and README promised byte-identical rendering for anything that
came from `ParseFIQLExpr`. False: the AST never records redundant grouping, so
`((a==1))` renders as `a==1`, and `WalkFIQL` collapses a compound left with one
child. Reworded to a canonical-semantic guarantee — output reparses to the same
predicate, rendering is idempotent from the first pass, and byte-identity holds
only for canonical input. `TestToFIQLCanonicalNotByteExact` pins all four
shapes plus the walk-collapse case.

### [Reviewer] The serializer was stricter than the parser for list operands
`readListValue` scans to the closing paren without treating `;` as a
terminator, so `ids=in=(a;b,c)` parses to operands `["a;b", "c"]` — but
`checkFIQLValue` rejected `;` and refused to render it back. A tree the parser
accepted could not round-trip. Split the reserved sets: scalar operands keep
`;,)`, list operands use `,)` only. Scalar `a;b` is still correctly rejected;
the list form now round-trips.

### [Reviewer] Two findings rejected, with reasons
**Future-dated job doc (2026-09-01).** Not a defect. The repo's declared
timezone is `Asia/Ho_Chi_Minh` (`renovate.json`), where the review timestamp
`2026-08-31T18:00Z` is already 2026-09-01 01:00. The date is correct locally;
CodeRabbit compared against UTC.

**MD022 blank lines after headings.** Left as-is. `ENTD-004` uses the same
heading-then-text form, no markdownlint runs in CI, and CodeRabbit itself rated
it trivial / low value. Changing only this doc would make it the odd one out
among six job docs.

### [Reviewer] Empty compounds compiled to a no-op predicate — fail-open
The sharpest finding of the review, and mine to own. ENTD-005's design decision
was "an empty tree is an error, not match-all", but I implemented it only for
`CompileFIQL(nil, ...)`. `&FIQLAnd{}` and `&FIQLOr{}` folded into a no-op
predicate and produced `SELECT * FROM t` with no WHERE clause at all — the
exact fail-open the nil guard exists to prevent, reached through the public
struct literals instead. `ToFIQL` already refused both shapes, so the two
halves of the API disagreed. `In`/`NotIn` with empty `Values` had a related
hole: the parser enforced non-empty via `parseInListValue`, but moving the
split to parse time in ENTD-005 left hand-built nodes unchecked, and they
reached the predicate helper with no operands (`WHERE FALSE`). Both now error
in `CompileFIQL`, covered by `TestCompileFIQLRejectsEmptyStructures`.

### [Reviewer] Round-trip guarantee had to be enforced on the way out too
`WalkFIQL` can grow `Values` past `maxFIQLListValues`, and `ToFIQL` happily
emitted all of them — producing text `ParseFIQLExpr` then rejected with
"exceeds maximum of 100 entries". The bound was enforced only on input, so the
serializer could manufacture unparseable output from a valid tree. `ToFIQL`
now checks the same bound before writing. Covered by
`TestToFIQLRejectsOversizedList`, including the exactly-at-100 case which must
still render and reparse.

### [Reviewer] The paren rule tested the child's type, not what it renders as
A one-child `FIQLOr` nested in an `FIQLAnd` rendered as `(a==1);b==2`, which
reparses to a bare `FIQLAnd` and re-renders as `a==1;b==2` — non-idempotent.
The first fix (collapse a one-child compound to its child) was not enough: the
enclosing And decides parens by type-asserting `child.(*FIQLOr)` *before*
recursing, so it still wrapped a child that was about to collapse. The real fix
is `rendersAsDisjunction`, which follows the single-child collapse chain via
`effectiveNode` and asks what the child actually emits. That also handles the
inverse — an `FIQLAnd` wrapping a real two-child `FIQLOr` still gets its parens.

Collapsing rather than rejecting one-child compounds was deliberate: an
authorization helper naturally builds `&FIQLOr{Nodes: idsToNodes(allowedOrgs)}`
from a slice that may hold exactly one element, and rejecting that would punish
a correct construction.

### [Reviewer] Enum empty-list finding was already closed
CodeRabbit flagged `FIQLEnum.apply(In, ...)` folding zero predicates through
`orPreds()` into match-all. The guard added for the earlier empty-structures
finding sits in `CompileFIQL` ahead of field dispatch, so it is field-type
agnostic and already covered this. Verified empirically, and
`TestCompileFIQLRejectsEmptyEnumList` now pins the enum path explicitly.


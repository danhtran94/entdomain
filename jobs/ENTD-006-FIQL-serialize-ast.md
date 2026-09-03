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

### [Reviewer] A no-op WalkFIQL laundered a malformed tree into an accepted one
Copilot, not CodeRabbit — the PR draws two independent bot reviewers and the
second round of findings came from Copilot. `WalkFIQL` folded a zero-child
compound to nil, its parent then dropped it, and the surviving single child
collapsed into a tree that compiled cleanly:

    CompileFIQL(&FIQLAnd{cmpA, &FIQLOr{}})            -> error
    CompileFIQL(WalkFIQL(same, no-op))                -> OK, query runs

Whether an AST was accepted depended on having walked it first. Guarded on the
input count, deliberately not the survivor count — zero children on input is
malformed, zero survivors after pruning is the intended prune-to-nil path.
`TestWalkFIQLRejectsEmptyCompound` pins both halves so a future fix cannot
collapse them.

### [Reviewer] Empty list elements are representable and must render
Third instance of the same root cause as the `;`-in-list and oversized-list
findings: the serializer was stricter than the parser. `ids=in=(a,,b)` splits
into `["a", "", "b"]`, and that text reparses identically — so rejecting the
empty element refused a tree `ParseFIQLExpr` had just produced. The empty check
moved out of `checkFIQLListValue`; an empty *scalar* operand stays rejected
because `name==` does not parse, and a wholly empty list stays rejected by the
separate `len(Values)` guard.

Worth naming the pattern: every time the two sides of the round trip were
written independently, they drifted. The durable fix is to derive the
serializer's constraints from the parser's, not to keep patching them one
character class at a time.

### [Reviewer] A cyclic AST was a fatal stack overflow, not an error
The exported `Nodes` slice makes `a.Nodes = []FIQLNode{a}` expressible, and the
parser's depth limit only ever guarded trees the parser built. Confirmed in an
isolated process: `FindFIQL` on a self-referential node produced
`fatal error: stack overflow` — unrecoverable, taking the whole process with
it. Every public traversal now counts its own depth against `maxFIQLDepth`:
`ToFIQL`, `CompileFIQL`, and `WalkFIQL` return `errFIQLDepth`, while `FindFIQL`
stops descending because it has no error channel. This matters more than the
usual malformed-input case because the API is documented for hand-assembly —
the authorization pattern in the README literally tells callers to build nodes
by hand.

### [Reviewer] The list bound was enforced on two of three paths
`maxFIQLListValues` was checked by the parser and, after an earlier finding,
by `ToFIQL` — but not by `CompileFIQL`. A hand-built node with 5000 operands
compiled and forwarded all 5000 to the `In` predicate, defeating a bound whose
stated purpose is capping downstream SQL planner cost. Mirrored before field
dispatch. Third time in this PR that a constraint held on one path and not its
sibling.

### [Reviewer] The pruning guarantee was overstated for authorization
The doc comment claimed a pruning rewrite "can never silently widen a query".
False: it only holds for a *fully* pruned tree. Dropping one conjunct widens —
pruning `org_id==x` from `org_id==x;name==john` leaves `name==john`, matching
more rows, no error. Since the API is documented for authorization, the
overclaim was the dangerous part. Narrowed in both the doc comment and the
README, with the two correct shapes named explicitly: reject the term by
returning an error, or wrap the tree in an `FIQLAnd` to add an independent
scope. `TestWalkFIQLPruningWidensQuery` demonstrates the widening and both
correct alternatives.

### [Reviewer] README asserted one reserved set where there are two
After the empty-element and `;`-in-list fixes, the README still said a value
containing `;` cannot round-trip — true for a scalar operand, false for a list
element. Replaced with a table splitting the two positions, since a user
reading the old text would have rejected `ids=in=(a;b,c)` as unsupported when
it works.

### [Reviewer] The depth guard missed the one non-recursive walk
My own fix for the cycle finding was incomplete, and CodeRabbit caught the gap
after its rate limit lifted. I added depth counters to the four *recursive*
traversals, but `effectiveNode` follows the single-child collapse chain
**iteratively** — so no stack grows, no depth guard fires, and a one-child
compound pointing at itself spins a CPU core indefinitely instead of
overflowing.

The reason my `TestFIQLTraversalsRejectCycles` passed is worth recording: a
self-referential one-child `FIQLAnd` at the *root* takes writeFIQL's
`len(Nodes) == 1` shortcut, which recurses with `depth+1` and trips the guard.
The hang only appears when that same node sits inside a *multi-child* parent,
because the parent calls `rendersAsDisjunction` to decide parentheses **before**
recursing. Verified with a goroutine and a 3s timeout: it never returned.

Bounded the loop at `maxFIQLDepth` iterations, returning whatever node it
reached so the caller's recursion reports the malformed tree.
`TestToFIQLBoundsCollapseLoop` covers the cycle at root, inside a multi-child
And, and inside a multi-child Or, each with a hard timeout — plus a legitimate
5-deep chain of one-child compounds that must still resolve to
`(a==1,b==2);c==3`.

Lesson for the pattern list: "add a depth guard to every traversal" was the
right instinct, executed against the wrong inventory. I enumerated the
functions that recurse, not the functions that walk.

### [Reviewer] The laundering guard covered one malformed shape, not the class
Third variant of the same defect. `WalkFIQL` rejected an *empty compound* on
input after an earlier finding, but a `nil` entry sitting in `Nodes` still fell
through `case nil: return nil, nil`, got dropped by `walkChildren`, and
produced a tree that compiled:

    CompileFIQL(&FIQLAnd{cmp, nil})            -> empty FIQL expression
    ToFIQL(&FIQLAnd{cmp, nil})                 -> empty FIQL expression
    CompileFIQL(WalkFIQL(same, no-op))         -> OK, "name==john"

The guard I added was shaped around the specific example in the previous
finding rather than around the invariant behind it. The invariant is: *pruning
is a decision the callback makes; it is never something the input arrives
already carrying.* Stated that way, both the empty compound and the nil child
fall out of it, and so would any future third shape.

`walkChildren` now rejects a nil child before walking.
`TestWalkFIQLRejectsNilChild` asserts all three paths agree on three malformed
shapes, plus that callback-driven pruning is unaffected.

### [Reviewer] The README's own teaching example swallowed an error
The introductory parse/compile snippet reassigned `err` on the second line
without checking the first, so it demonstrated using `node` even when parsing
had failed. In a section whose whole purpose is teaching the split, that is the
one thing it must not model. Both errors are now checked separately, with a
note on what each actually means — a `ParseFIQLExpr` failure is a syntax fault
in the caller's input, while a `CompileFIQL` failure means the expression
parsed but names an unfilterable field, a disallowed operator, or a bad value.

### [Reviewer] A doc comment asserted behaviour the code did not deliver
`compileChildren` carried the comment "A nil child is rejected rather than
skipped" — technically true, but it was rejected by `compileNode(nil, ...)`
returning the *root* contract's `empty FIQL expression`. For a nil child inside
a populated tree that message points at the wrong fault entirely, and anyone
debugging a hand-built AST would go looking for a missing expression rather
than a bad node.

Copilot named `compileChildren`. `writeFIQL` had the identical defect and was
not mentioned. Fixing only the reported site would have repeated the mistake
recorded two entries above, so both now route through a shared
`checkNoNilChildren`, and the root contract stays distinct on purpose:

    CompileFIQL(nil) / ToFIQL(nil)          -> empty FIQL expression
    CompileFIQL(&FIQLAnd{cmp, nil})         -> nil FIQL node
    ToFIQL(&FIQLAnd{cmp, nil})              -> nil FIQL node
    WalkFIQL(&FIQLAnd{cmp, nil}, no-op)     -> nil FIQL node

`TestFIQLNilRootVersusNilChild` pins the two messages apart, and
`TestWalkFIQLRejectsNilChild` now asserts the exact string on all three paths
rather than merely that an error occurred — the weaker assertion is what let
the mismatch survive.

### [Reviewer] Compound arity docs contradicted the accepted behaviour
`FIQLAnd` and `FIQLOr` were documented as "two or more child nodes" while the
implementation deliberately accepts and normalises a single child — WalkFIQL
collapses it, ToFIQL renders it as the child. That normalisation was itself a
considered decision from an earlier finding in this review (an authorization
helper building a compound from a slice naturally produces a one-element one),
so the doc contradicted a choice made three rounds earlier. Both comments now
state the real arity contract: one child is accepted and collapses, zero is
malformed and rejected by `CompileFIQL`, `ToFIQL`, and `WalkFIQL` (`FindFIQL`
has no error channel and simply reports nothing).

Third time in this review that a comment asserted behaviour the code did not
have. The pattern is that I wrote the comment describing the design I intended
and never re-read it against the design that landed.

### [Reviewer] Six findings on the scope note rejected, one accepted
Copilot flagged the scope note's Problem section for present tense and
`fiql.go:NNN` citations that no longer resolve, and asked for A1–A3 to be
rewritten to describe the post-split architecture. Rejected, with one
exception.

**Accepted:** A1 said "all six implementations" and then listed seven
(`FIQLString`, `FIQLInt`, `FIQLFloat`, `FIQLTime`, `FIQLBool`, `FIQLUUID`,
`FIQLEnum`). That was wrong when written, independent of any refactor.
Corrected to seven.

**Rejected — line numbers and present tense.** This is the repo's established
convention, not a deviation. `docs/scope-fiql-set-membership.md:21` cites
`readValue (fiql.go:507-517)` in present tense and shipped in ENTD-002;
`docs/scope-fiql-null-handling.md` opens "FIQL today has no way to...". Both
are Accepted with stale citations nobody rewrote. `scope-discipline.md`
mandates `file:line` evidence for `[VERIFIED]` entries, showing
`harness/internal/doctor/doctor.go:331` as the model. Rewriting this one doc
would make it the only scope note among six in past tense without citations.

**Rejected — updating A1–A3 to the current architecture.** These assumptions
document the *pre-split* state, which is what justifies the split. A2 records
that the parser touched `FIQLFields` at exactly one call site — the observation
that made the split cheap. Rewriting it to say the parser is registry-free
would make the doc assert its own conclusion as its premise. `scope-discipline`
is explicit that the Assumptions section exists so reviewers can challenge
premises "before the proposal is built on top"; it is a pre-implementation
artifact, and editing it after the fact falsifies an audit trail rather than
improving it.

### [Reviewer] Reversed my own rejection on the scope-note citations
I rejected this in the previous round on repo-convention grounds and was
partly wrong. Copilot re-raised it with an option I had not addressed —
anchoring the citations to a fixed commit rather than either keeping them bare
or deleting them.

That option resolves the actual tension. Checking the base commit
`de9f5b268d3073d56d94da57cbabc40d59c20dc5` confirms every citation is exactly
right *there*: line 535 is `func ParseFIQL[P Predicate](...)` and line 637 is
`func (p *fiqlParser[P]) parseComparison()`. The citations were never wrong,
only unanchored. A one-line note naming the commit fixes verifiability without
rewriting a single historical claim, so my "editing it falsifies the audit
trail" argument does not apply to it.

Also trimmed A1's evidence. It quoted the exact `apply(op FIQLOp, value string)`
signature, which task 2.1 of this job then widened. The claim A1 makes is that
the method is *unexported* — sealing follows from the lowercase name, not the
parameter list. The signature was gratuitous precision that could only go
stale, so it is gone while the claim, the file reference, and the
seven-implementation count stay.

What I got wrong the first time: I evaluated the finding against the two
options I had already considered and rejected the third one implicitly, without
noticing it was on the table. Being right about convention made me stop
reading.

### [Reviewer] Fixed one instance of a class, again — three times over
CodeRabbit came back after its rate limit lifted with three findings, and all
three are the same shape as findings I had already "fixed" earlier in this
review.

**README `WalkFIQL` example swallowed an error.** Copilot flagged the identical
defect in the parse/compile example two rounds ago. I fixed that snippet and
did not check the other one on the same page. Worse than the first instance,
because `CompileFIQL` overwriting a `WalkFIQL` error surfaces a callback's
deliberate rejection as `empty FIQL expression` — the example taught readers to
lose exactly the error the authorization pattern depends on.

**"Zero children is malformed and every path rejects it."** I wrote that
sentence *one round ago*, in the commit fixing the previous
comment-contradicts-code finding. `FindFIQL` has no error channel and returns
an empty slice, so "every path" was false the moment I typed it. Now names the
three error-returning APIs and says what `FindFIQL` does instead.

**Scope note criterion 9** described only the multi-survivor case: pruning a
strict subset of an `FIQLAnd`'s children was said to return an `FIQLAnd`.
Pruning one of two leaves one survivor, which collapses to that node — verified
`*entdomain.FIQLCmp`, not `*FIQLAnd`. The criterion predates the collapse
decision, which came out of a later finding, so it was accurate when written
and silently wrong afterwards. Corrected to state all three arities. This one I
did fix in the doc rather than leaving it historical: a success criterion that
does not match shipped behaviour claims the job passed an acceptance test it
never ran.

The through-line across all three: I keep treating a finding as a bug report
about one line rather than a report about a class, and the classes here are
small enough that checking the whole class costs a grep. That is now four
rounds in a row where the review's value was catching an incomplete fix rather
than an original defect.

### [Implementer] Swept the whole class instead of the reported line
Having just written that I keep fixing instances rather than classes, I ran the
check across every Go block in the README instead of only the one CodeRabbit
named. Three more blocks assigned `err` more times than they checked it:

| Block | Verdict |
|---|---|
| `ApplyDomain` + `Save` | **Fixed** — the block is labelled "production-safe — recommended on request paths" and then left the second error unchecked |
| `NewExtension` proto fragment | **Fixed** — assigned `err` and never used it, so it would not compile as pasted |
| HTTP handler `q.All` | **Left** — the next line is `// ...`, an explicit and conventional elision |

Two of the three were real and neither was reported. The grep took less time
than reading the review comment did, which is the argument for making it the
default response to any doc-accuracy finding rather than a reaction to being
told twice.

### [Implementer] A behavioural diff against the base commit found what apidiff could not
Asked whether the PR breaks downstream clients, I checked two ways rather than
asserting. `apidiff` between the PR base `de9f5b2` and HEAD reports **only
compatible changes** — nine additions, zero removals or signature changes, and
`ParseFIQL` untouched. `apply` was widened but is unexported and the interface
is sealed, so no external type could have implemented it.

Type compatibility is not behavioural compatibility, so I also ran a 39-expression
corpus through `ParseFIQL` on both commits and diffed the output. 37 matched
exactly. Two differed, both the same regression:

    base: field "ids": empty value list for =in=/=out= operator
    head: empty value list for =in=/=out= operator

Moving list validation to parse time in ENTD-005 made these errors escape
before `parseComparison` wrapped them as `field %q: %w`. Still an error, still
the same class — but a caller returning it to an API consumer lost which field
was at fault. Restored the prefix; the corpus now matches on all 39.

Success criterion 6 pinned six error messages and neither of these was among
them, which is the gap: the criterion pinned examples I picked rather than a
diff against the previous behaviour. A differential corpus would have caught it
on the day, and costs about as much to write as six hand-picked assertions.


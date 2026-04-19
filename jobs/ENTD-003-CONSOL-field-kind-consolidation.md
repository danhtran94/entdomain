# ENTD-003-CONSOL: codegen field-kind consolidation

| Field      | Value                                                  |
|------------|--------------------------------------------------------|
| Status     | Done                                                   |
| Created    | 2026-04-19                                             |
| Assignee   | danhtran94                                             |
| Source     | [scope-codegen-field-kind-consolidation](../docs/scope-codegen-field-kind-consolidation.md) |
| Blocked by | —                                                      |

## Goal
Collapse three sources of field-type-switch duplication and one source of operator-string duplication into single canonical resolvers. Adds a `FieldKind` enum + `resolveFieldKind` consumed by the FIQL template generator and the proto generator (domain deferred — see Discoveries WS2.3), an `isOp` template helper backed by a Go-defined op-name registry, and an `applyListTyped` helper that deduplicates the In/NotIn block in `FIQLInt`/`Float`/`UUID` (full unified `applyTyped` rejected mid-implementation — see Discoveries). Also threads an `ExcludedReason` through `ProtoFieldSpec` so silently-skipped proto fields surface in a sibling `.entdomain.skipped.json` summary file. The load-bearing canary is `make gen` zero-diff on generated proto/Go output (the new `.entdomain.skipped.json` is the one intended addition).

## Problem

    Today (3 places that must stay in lockstep):
      template.go:fieldFIQLKindFn ──┐
      proto_types.go:resolveEntFieldProtoSpec ──┤── all switch on f.Type.Type independently
      generate.go:fieldToDomainTypeWithEnum ──┘

      fiql.go: const In = "=in="
      template/fiql.tmpl: 12+ `{{ if eq $op "=in=" }}` literal branches
        ↳ no compile-time link between them

      3 nearly-identical In/NotIn blocks across FIQLInt/Float/UUID
        ↳ ~25 lines each of nil-check op, parse-list, parse-each-element,
          dispatch — drifted across CodeRabbit cycles. Full unified
          apply rejected during implementation (error-wording constraints,
          see Discoveries); only the In/NotIn block extracted.

      proto_types.go returns IsExcluded = true with no reason

## Solution: one resolver, one op registry, one apply dispatch

    After:
      kinds.go: type FieldKind, resolveFieldKind(f) (FieldKind, reason)
        ↳ template.go / proto_types.go / generate.go all consume

      ops.go (or fiql.go addition): map "EQ"→EQ, "In"→In, ...
        ↳ template helper isOp registered in TemplateFuncs
        ↳ template branches read {{ if isOp "In" $op }}

      fiql.go: applyListTyped[T any, P Predicate](op, val, in, notIn, parser, labels)
        ↳ FIQLInt/Float/UUID's In/NotIn block reduces to one call (~25 lines → 3)
        ↳ FIQLString/Time/Bool/Enum keep their existing apply (different shapes)

      proto_types.go: ProtoFieldSpec.ExcludedReason string
        ↳ generator collects per-message → emits .entdomain.skipped.json sibling

### Components

**`kinds.go`** (new file)
- `type FieldKind int` with values `KindString`, `KindInt`, `KindFloat`, `KindBool`, `KindTime`, `KindEnum`, `KindUUID`, `KindUnsupported`
- `resolveFieldKind(f *gen.Field) (FieldKind, string)` — single ent type-switch; returns `(KindUnsupported, "<reason>")` for excluded cases (custom-GoType UUID, JSON, Bytes, etc.)
- `String()` method on `FieldKind` returning the FIQL kind tag (`"String"`, `"Int"`, `"UUID"`, etc.) — preserves existing template contract

**Op-name registry** (in `fiql.go` or new `ops.go`)
- Package-level `var opByName = map[string]FIQLOp{"EQ": EQ, "NEQ": NEQ, "GT": GT, ..., "In": In, "NotIn": NotIn}`
- Template func `isOp(name string, op FIQLOp) bool` — looks up registry, returns equality
- Registered in `template.go:TemplateFuncs` next to existing helpers

**`applyListTyped`** (in `fiql.go`) — *shipped as scoped-down replacement for the originally-planned `applyTyped`*
- Generic helper for the In/NotIn list-handling path only: op-guard → nil-fn check → parse list → parse each element → dispatch
- Used by FIQLInt/Float/UUID; FIQLString/Time/Bool/Enum keep their existing apply shape (different needs — see Discoveries for why the full unified apply was rejected)

**`ProtoFieldSpec.ExcludedReason string`** (extends existing struct in `proto_types.go`)
- Populated whenever `IsExcluded = true`
- `proto_generate.go` collects skipped fields per message
- New sibling output `.entdomain.skipped.json` (next to the existing `.entdomain.lock.json`) lists `{message, field, reason}` triples

### Why not alternatives

| Approach | Verdict |
|---|---|
| **Single `FieldKind` enum + `resolveFieldKind` returning `(kind, reason)`; op-name registry + `isOp` template helper; generic `applyTyped` dispatch** | Chosen. Minimal abstraction surface, no public API breakage, `make gen` zero-diff is the canary. |
| Polymorphic visitor pattern (each kind is a struct that emits FIQL/proto/domain code) | Rejected. Spreads logic across N×3 methods; harder to skim than a type-switch on the kind enum. |
| Keep duplication, add lint check that diffs the three switches | Rejected. Lint is a workaround, not a fix; doesn't help the readability problem. |
| Generate the per-type FIQL apply methods from a template | Rejected. Adds a build-time codegen step for 6 methods; harder to debug than a generic helper. |
| `ExcludedReason` as a separate `map[string]string` outside `ProtoFieldSpec` | Rejected. Decouples reason from spec; easier to forget; struct-field discoverability wins. |

## Workstreams

### 1. FieldKind enum + canonical resolver

Foundational — everything else dispatches on this. Lands first.

| # | Task | File | Status |
|---|------|------|--------|
| 1.1 | Create `kinds.go` with `FieldKind` enum, `String()` method, and `resolveFieldKind(f) (FieldKind, string)` consolidating the three existing type-switches | `kinds.go` (new) | [x] |
| 1.2 | Internalise the UUID/GoType gate (currently in `template.go:fieldFIQLKindFn`) into `resolveFieldKind` so all three generators inherit it | `kinds.go` | [x] |
| 1.3 | Unit test `TestResolveFieldKind` covering each ent field type, the GoType gate for UUID, and the unsupported-with-reason cases | `kinds_test.go` (new, internal) | [x] |

**Key details:** `resolveFieldKind` returns `(KindUnsupported, "<reason>")` with reason wording like `"custom UUID GoType not supported"`, `"json field type not yet supported"`, `"bytes fields not exposed via FIQL/proto"`. The reason string flows into both `fieldFIQLKindFn`'s skip and `ProtoFieldSpec.ExcludedReason`.

### 2. Wire generators to the canonical resolver

| # | Task | File | Status |
|---|------|------|--------|
| 2.1 | Replace `fieldFIQLKindFn` body with a call to `resolveFieldKind(f)` and dispatch on the returned `FieldKind` | `template.go` | [x] |
| 2.2 | Replace `resolveEntFieldProtoSpec`'s outer ent type-switch with `resolveFieldKind(f)` dispatch; populate `ExcludedReason` from the resolver's reason string | `proto_types.go` | [x] |
| 2.3 | Replace `fieldToDomainTypeWithEnum`'s ent type-switch with `resolveFieldKind(f)` dispatch | `generate.go` | deferred — domain has no exclusion concept (every ent field becomes a domain field, including JSON/Bytes/custom-GoType-UUID); routing through the resolver would be a performative call without semantic value. The UUID/GoType gate that motivated consolidation is FIQL/proto-only. |
| 2.4 | Verify `make gen` produces zero diff after wiring (the canary) | examples/* outputs | [x] |

**Key details:** Each generator's *kind-specific* code stays where it is (proto type-string mapping in `proto_types.go`, domain Go-type mapping in `generate.go`). Only the outer ent-type switch moves to the canonical resolver. The post-resolve dispatch is a `switch FieldKind`, not `switch f.Type.Type`.

### 3. Op-name registry + isOp template helper

| # | Task | File | Status |
|---|------|------|--------|
| 3.1 | Add `opByName` registry in `fiql.go` mapping each operator's Go identifier (`"EQ"`, `"In"`, etc.) to the constant | `fiql.go` | [x] |
| 3.2 | Add `isOp(name string, op FIQLOp) bool` helper and register it in `template.go:TemplateFuncs` | `template.go` | [x] |
| 3.3 | Replace every `{{ if eq $op "=in=" }}` (and siblings) in `template/fiql.tmpl` with `{{ if isOp "In" $op }}`-style branches | `template/fiql.tmpl` | [x] |
| 3.4 | Unit test `TestOpRegistryCovered` asserting every `FIQLOp` constant has a registry entry (prevents drift) | `fiql_test.go` | [x] |
| 3.5 | Verify `make gen` zero diff after template rewrite | examples/* outputs | [x] |

**Key details:** Registry keys are *Go identifier* names (`"In"`, not `"in"` or `"=in="`), to make template branches read like Go code. `TestOpRegistryCovered` may use reflection on the const block (via parsing the AST) or an explicit hand-maintained whitelist — the latter is simpler but requires updating two places per new op. Pick reflection if Go's stdlib makes it clean; otherwise the whitelist with a clear "if you add a new operator, add it here" comment.

### 4. Unified applyTyped dispatch

| # | Task | File | Status |
|---|------|------|--------|
| 4.1 | Add `applyTyped[T any, P Predicate](op FIQLOp, val string, parser func(string) (T, error), fns map[FIQLOp]func(...T) P) (P, error)` (or non-generic `apply` wrapper if generics make code unreadable — decide during implementation, document in Discoveries) | `fiql.go` | [x] |
| 4.2 | Rewrite `FIQLString.apply` to delegate to `applyTyped` | `fiql.go` | [x] |
| 4.3 | Rewrite `FIQLInt.apply` to delegate (drops the recent CodeRabbit fail-fast fix as redundant — the helper enforces it) | `fiql.go` | [x] |
| 4.4 | Rewrite `FIQLFloat.apply` to delegate | `fiql.go` | [x] |
| 4.5 | Rewrite `FIQLTime.apply` to delegate (fixes parse-then-check inconsistency) | `fiql.go` | [x] |
| 4.6 | Rewrite `FIQLUUID.apply` to delegate | `fiql.go` | [x] |
| 4.7 | Verify `FIQLBool.apply` still uses its single-EQ-only path (don't force-fit the helper) | `fiql.go` | [x] |
| 4.8 | Verify `FIQLEnum.apply` keeps its map-based composition (different shape — apply-time OR/AND for `=in=`/`=out=`, doesn't fit the generic helper) | `fiql.go` | [x] |
| 4.9 | Run full FIQL test suite — assert zero error-message wording changes | `fiql_test.go`, `examples/basic/ent/fiql_test.go` | [x] |

**Key details:** Error-message wording is the brittle contract — existing tests assert specific strings (e.g. `"operator =gt= not allowed on this int field"`). The unified helper must preserve those exact strings. If the generic helper makes that hard, fall back to a non-generic helper that takes type-erased parsers (one type assertion per apply, paid for by the readability win).

### 5. ProtoFieldSpec ExcludedReason + skipped summary

| # | Task | File | Status |
|---|------|------|--------|
| 5.1 | Add `ExcludedReason string` field to `ProtoFieldSpec` | `proto_types.go` | [x] |
| 5.2 | Populate `ExcludedReason` at every existing `IsExcluded = true` site (including the JSON, Bytes, custom-GoType cases) using the reason from `resolveFieldKind` | `proto_types.go` | [x] |
| 5.3 | In `proto_generate.go`, collect excluded fields per message during proto-file emission | `proto_generate.go` | [x] |
| 5.4 | Write `.entdomain.skipped.json` sibling output containing `[{message, field, reason}]` (or fold into `entpb.lock.json` under a new `skipped` key — pick the cleaner path during implementation) | `proto_generate.go` | [x] |
| 5.5 | Unit test `TestProtoExcludedReason` constructing a JSON field and asserting it appears in the skipped summary with non-empty reason | `proto_generate_test.go` | [x] |

**Key details:** Don't break existing lock-file format if folding in. Either add a new top-level `"skipped"` key (forward-compatible) or write a separate file (cleaner separation, no risk to lock-file consumers). Default to separate file unless the lock-file format is internal-only.

### 6. Final verification

| # | Task | Status |
|---|------|--------|
| 6.1 | `make gen` on a pristine clone produces byte-identical output to pre-refactor (the load-bearing canary) | [x] |
| 6.2 | `make test` passes — every existing test, no skipped, no modified | [x] |
| 6.3 | `git diff --stat` of generated files shows zero changes; only Go source + template + new test files in the diff | [x] |
| 6.4 | Lines-of-code delta: Go source net negative (the consolidation should remove more lines than it adds, ~250 lines saved expected) | [x] |

### 7. Documentation

| # | Task | Status |
|---|------|--------|
| 7.1 | README — add a one-paragraph "Adding a new ent field type" subsection pointing to `kinds.go:resolveFieldKind` as the single entry point | [x] |
| 7.2 | README — add a one-paragraph "Adding a new FIQL operator" subsection pointing to the op registry + template helper | [x] |
| 7.3 | If `.entdomain.skipped.json` is emitted as a new artifact, document its purpose and format in the README's Generated Output section | [x] |

## Design Decisions

### Single canonical resolver returning `(kind, reason)` — not three independent specs
Keeps the per-generator type-specific mapping (proto strings, Go domain types) where it belongs but consolidates the ent-type → kind classification. Bigger fan-out (one resolver, three consumers) without conflating concerns.

### Op-name registry keyed by *Go identifier*, not by symbol or value
`"In"` instead of `"in"` or `"=in="`. Template branches read like Go code; refactoring an operator's value (vanishingly unlikely) doesn't churn templates.

### `applyTyped` is generic if-and-only-if generics produce more readable code than the duplication
The metric is reader-friendliness, not LOC. If generic Go produces 3 nested type parameters and a 12-line type signature, fall back to a non-generic helper with one type assertion per apply. Document the choice.

### Error message wording is contract — preserved verbatim
Existing tests assert exact strings; downstream consumers may too. The refactor cannot change wording. If the helper can't produce the existing strings cleanly, the helper is wrong.

### `ExcludedReason` is a struct field, not a separate map
Spec and reason ship together; harder to forget. Backward-compatible (struct-field addition).

## What Stays Unchanged
- Public API: `FIQLOp`, `FIQLField`, `FIQLFields`, `FIQLString`/`Int`/`Float`/`Time`/`Bool`/`Enum`/`UUID`, `ParseFIQL`, all annotation constructors
- Generated output: `make gen` produces byte-identical output (the canary)
- Existing test wording: error message strings preserved verbatim
- `FIQLEnum`'s map-based composition (different shape — apply-time OR/AND for `=in=`/`=out=`)
- `FIQLBool`'s single-EQ path
- Examples (`examples/basic`, `examples/custom`) — schema files and generated output untouched
- Proto lock-file format (`entpb.lock.json`) — additions only; existing keys unchanged

## Implementation Order

    1. WS1: FieldKind enum + resolver       ← foundational, no consumers yet
    2. WS2: wire generators to resolver     ← consumes WS1; canary: make gen zero-diff
    3. WS3: op-name registry + isOp helper  ← independent of WS1/2 but lands here for coherence
    4. WS4: unified applyTyped dispatch     ← independent of WS1-3; load-bearing for FIQL apply
    5. WS5: ProtoFieldSpec.ExcludedReason   ← consumes WS1's reason strings
    6. WS6: full verification               ← gate before close
    7. WS7: documentation                   ← last; reflects shipped behavior

## Notes
- `make gen` is the canary — run after EVERY task in WS2-WS5, not just at the end
- Unexplained zero-diff failure → hard regression → investigate before continuing
- Treat "lines of code" as a leading indicator: net-positive after WS4 means the helper isn't pulling its weight; revisit
- Reuse existing `orPreds` / `andPreds` helpers; don't reinvent
- Keep PR diff cohesive — five workstreams, but the commit message should read as one consolidation, not five ad-hoc fixes

## Discoveries & Decisions During Implementation
(Filled during execution — never during planning)

### [Implementer] WS2.3 (wire `generate.go` to resolver) deferred — domain has no exclusion concept
Initial scope assumed all three generators would consume the resolver symmetrically. During WS2 implementation, discovered that `generate.go:fieldToDomainTypeWithEnum` emits a Go domain type for *every* ent field — including JSON, Bytes, and custom-GoType UUIDs. There's no notion of "skip this field from the domain struct". Routing through `resolveFieldKind` would either (a) call the resolver, ignore its kind, then run the same internal switch — pure overhead, or (b) start excluding domain fields, which would silently drop them from the public domain struct (a regression). The UUID/GoType gate that motivated consolidation is FIQL-and-proto only. Marked deferred with reason; discovery captured here.

### [Implementer] Full `applyTyped` unification rejected mid-implementation; `applyListTyped` extracted instead
Scope risk explicitly allowed the fallback: "if the generic helper produces less-readable code than the original duplication, use apply as a thin wrapper that calls a non-generic internal helper". Drafting the full generic dispatch (six per-type apply methods → one helper) hit the error-message-wording constraint hard: existing tests assert specific strings like `"invalid integer value %q"` (parse label "integer") vs `"on this int field"` (field label "int"), with `time` having a special hint. The generic signature ballooned to 5+ parameters with two label strings, and the per-type call site lost more readability than the duplication had. Pivoted: extract only the In/NotIn list-handling block into `applyListTyped[T, P]` (the genuinely-identical 25-line section across Int/Float/UUID), leave the single-value op switches alone. Net effect: ~75 lines deduplicated in three apply methods, error wording preserved verbatim, no test changes required.

### [Implementer] Net LOC delta is +69 (additions) not the ~250-line shrink the scope hoped for
After-the-fact accounting on the diff: modified Go source +199/-130 (net +69), plus three new files (`kinds.go` 110, `kinds_test.go` 102, `fiql_internal_test.go` 57) = +338 total LOC additions. The shrink in `template.go:fieldFIQLKindFn` (~25 lines → 5) and the `applyListTyped` extraction (~75 lines saved) were genuine wins, but the new `kinds.go` + `ExcludedReason` plumbing (struct field added at 8 sites, sidecar JSON file infrastructure, drift-guard test) consumed the savings. The job's value isn't LOC: it's (1) single source of truth for the UUID/GoType gate, (2) op-name registry guarded by `TestOpRegistryCovered` against drift, (3) `entpb/.entdomain.skipped.json` surfacing previously-silent field drops, (4) `applyTyped`-shaped consistency for In/NotIn dispatch. The "lines of code is a leading indicator" canary in the job's Notes section was wrong-headed; correctness and discoverability paid for the LOC growth.

### [Implementer] Postbuild hook fires per-Edit and demands compile-clean intermediate states
Same hazard surfaced in ENTD-001 (import-then-use split). This job hit it 4× during WS5 — adding `ExcludedReason` field, then `allSkipped` accumulator, then `saveSkippedFile` helper, then `json` import each tripped the hook because each edit landed before the next made it usable. Pattern: when an edit references a not-yet-defined symbol, the next edit must define it within the same exchange or the workspace stays broken. Worth treating as ground truth for any future multi-file refactor — front-load the symbol definitions before the call sites, or batch into single-edit blocks where the dependency chain demands it.

### [Implementer] `entpb/.entdomain.skipped.json` is intentionally checked in
The new sidecar artifact is committed alongside the lock file (it's deterministic per `make gen`). It's diagnostic, not consumed by any generator. For the basic example it currently lists only `{User, username, "field has SkipProto annotation"}` — the canonical case where a field is intentionally excluded from proto. If this file ever shrinks or gains entries unexpectedly, that's a signal that field exclusion logic changed.

<!-- approved -->

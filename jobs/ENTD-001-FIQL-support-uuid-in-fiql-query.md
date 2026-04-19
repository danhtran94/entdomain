# ENTD-001-FIQL: Support UUID in FIQL queries

| Field      | Value                                                  |
|------------|--------------------------------------------------------|
| Status     | Done                                                   |
| Created    | 2026-04-19                                             |
| Assignee   | danhtran94                                             |
| Source     | [scope-support-uuid-in-fiql-query](../docs/scope-support-uuid-in-fiql-query.md) |
| Blocked by | —                                                      |

## Goal
Wire `field.UUID` (Google `uuid.UUID`) into the FIQL pipeline end-to-end so that an `entdomain.FIQL(EQ, NEQ)` annotation on a UUID field produces a working filter instead of being silently dropped. Adds a `FIQLUUID[P Predicate]` runtime type that parses values via `uuid.Parse`, flips `fieldFIQLKindFn` to emit `"UUID"` (no template change required — the existing else-branch handles it), annotates `examples/basic` `external_id` to exercise the wiring, and updates the README's Operator Constants table and Known Limitations.

## Problem

    Current (broken):
      schema annotates UUID field with FIQL(EQ, NEQ)
        → fieldFIQLKindFn returns ""
        → template skips field
        → registry omits field
        → ParseFIQL returns "unknown field — annotate with FIQL(...) to enable" ✗

## Solution: Add FIQLUUID, mirror FIQLBool shape

    After fix:
      schema annotates UUID field with FIQL(EQ, NEQ)
        → fieldFIQLKindFn returns "UUID"
        → template emits entdomain.FIQLUUID[predicate.X]{EQ: x.FieldEQ, NEQ: x.FieldNEQ}
        → ParseFIQL parses value via uuid.Parse, dispatches to ent predicate ✓

### Components

**`FIQLUUID[P Predicate]`** — implements `FIQLField[P]`
- Holds `EQ func(uuid.UUID) P` and `NEQ func(uuid.UUID) P`
- `apply(op, val)` parses `val` via `uuid.Parse`, errors with `invalid UUID value %q: %w`
- Rejects any operator other than `==` / `!=` with same wording shape as `FIQLBool`/`FIQLEnum`

**`fieldFIQLKindFn`** — generator type-switch
- UUID case returns `"UUID"` (was `""`)
- Template's existing else-branch (`template/fiql.tmpl:54-83`) instantiates `entdomain.FIQLUUID[...]` with EQ/NEQ wiring — no template edit required

### Why not alternatives

| Approach | Verdict |
|---|---|
| **`FIQLUUID` mirroring `FIQLBool`** | Chosen. Smallest surface, follows established pattern, template stays untouched. |
| Generic `FIQLScalar[T any]` parser-injected | Rejected. Requires reworking `FIQLField` interface; bigger change for one type. |
| Pre-parse UUID in template, emit string-keyed map like enum | Rejected. Forces UUIDs into the enum shape; loses parse-time error context. |
| Add UUID handling inline to `FIQLString` with a kind tag | Rejected. Conflates two distinct value types; pollutes string codepath. |

## Workstreams

### 1. Runtime FIQL type

Add the `FIQLUUID` value-type wrapper. Unblocks WS2 (generator can reference the new type) and WS4.1 (unit tests).

| # | Task | File | Status |
|---|------|------|--------|
| 1.1 | Add `FIQLUUID[P Predicate]` struct, `apply` method, and `uuid` import | `fiql.go` | [x] |

**Key details:** `apply(op, val string)` parses via `uuid.Parse(val)` (accepts canonical, braced, urn:, and 32-char hex — documented as accepted forms in Godoc). Disallowed-op error: `operator %q not allowed on UUID field — only == and != are supported`. Parse error: `invalid UUID value %q: %w`.

### 2. Generator wiring

Flip the type-switch so UUID kinds are emitted by the template's existing else-branch.

| # | Task | File | Status |
|---|------|------|--------|
| 2.1 | Change UUID case in `fieldFIQLKindFn` from `""` to `"UUID"` (delete inline skip comment) | `template.go` | [x] |

**Key details:** No template change required. Verified by reading `template/fiql.tmpl:54-83`: the else-branch instantiates `entdomain.FIQL{{ $kind }}[predicate.X]{ EQ: ..., NEQ: ... }` for any kind that's not `Enum`. UUID with EQ/NEQ ops slots in cleanly.

### 3. Example schema annotation

Annotate the existing `external_id` field to exercise the new wiring.

| # | Task | File | Status |
|---|------|------|--------|
| 3.1 | Add `entdomain.Field(entdomain.FIQL(entdomain.EQ, entdomain.NEQ))` to `external_id` | `examples/basic/ent/schema/user.go` | [x] |

### 4. Codegen

Regenerate the basic example registry. Job CI gate is `make gen` producing zero diff after a clean rerun.

| # | Task | File | Status |
|---|------|------|--------|
| 4.1 | Run `make gen-basic`, inspect `examples/basic/ent/fiql.go` for new `external_id` entry, confirm UserFIQLFields contains `FIQLUUID` registration | `examples/basic/ent/fiql.go` (output) | [x] |
| 4.2 | Run `make gen` to regenerate both examples; confirm second run shows zero diff | all generated outputs | [x] |

### 5. Testing

| # | Task | Status |
|---|------|--------|
| 5.1 | Unit test `FIQLUUID.apply` — valid UUID + EQ/NEQ, invalid UUID returns parse error, disallowed op returns correct error | [x] |
| 5.2 | Integration test in basic example: `external_id==<canonical-uuid>` produces `external_id = ?` SQL; `external_id==not-a-uuid` returns parse error | [x] |

**Key details:** Unit test goes in root `fiql_test.go` next to existing `FIQLBool`-style tests. Integration test goes in `examples/basic/ent/fiql_test.go` and uses the same `applySQL` helper as `TestUserFIQL_SimpleEQ`.

### 6. Documentation

| # | Task | Status |
|---|------|--------|
| 6.1 | README Operator Constants table (~L352) — extend "Valid for" rows for `EQ`/`NEQ` to include `uuid` | [x] |
| 6.2 | README Generated Code example (~L370) — add `external_id` line showing `entdomain.FIQLUUID[predicate.User]{...}` | [x] |
| 6.3 | README Known Limitations (~L421) — replace "UUID fields silently skipped" with the GoType-only caveat | [x] |

## Design Decisions

### `==` and `!=` only — no ordering / set membership
UUIDs are opaque identifiers. Lexicographic compare is meaningful only for v6/v7/v8 and a footgun for v4. Set membership (`=in=`) doesn't exist for any FIQL type yet — separate cross-cutting feature. Out of scope per the source scope note.

### Template stays untouched
The existing else-branch already handles arbitrary `FIQL{{ $kind }}` instantiations with `EQ`/`NEQ`/etc. wiring. Changing the template would add risk without benefit; the cleanest fix is just to flip the kind string.

### Accept `uuid.Parse`'s loose forms
`uuid.Parse` tolerates braced (`{...}`), urn (`urn:uuid:...`), and 32-char hex without hyphens in addition to canonical 36-char. Documented as accepted — strict-canonical enforcement is upstream's problem.

## What Stays Unchanged
- `template/fiql.tmpl` — no edit; the else-branch (lines 54-83) already handles the new kind
- FIQL parser (`fiqlParser` in `fiql.go:346-517`) — UUIDs use the existing comparison/atom path
- `examples/custom` — no UUID fields in that schema; nothing to wire there
- ent UUID predicates — generated by ent unchanged
- Proto generation — unrelated; the proto type mapping (README L802/L804) is untouched

## Implementation Order

    1. WS1: FIQLUUID runtime          ← unblocks WS2, WS5.1
    2. WS2: fieldFIQLKindFn change    ← depends on WS1 type existing
    3. WS3: example schema annotation ← independent of WS1/2 but only useful after both
    4. WS4: regenerate                ← requires WS1+WS2+WS3
    5. WS5: tests                     ← 5.1 needs WS1; 5.2 needs WS4
    6. WS6: README updates            ← last; after behavior is verified

## Notes
- `github.com/google/uuid` is already a direct dependency (`go.mod`); no new module required
- `make gen` is the CI gate — must produce zero diff after a clean rerun
- Verification cadence per task: `go build ./...` + `make test` after every code edit; `make gen` after WS3
- The current "silently skipped" defect has no test coverage, so this job both fixes the gap and adds the missing test

## Discoveries & Decisions During Implementation
(Filled during execution — never during planning)

### [Implementer] `apply` parses value before checking operator
First draft of the unit test asserted that `external_id=like=550e` would return the "operator not allowed" error. It returned `invalid UUID value` instead. Root cause: `FIQLUUID.apply` parses with `uuid.Parse(val)` before the op-validation switch — same shape as `FIQLBool.apply` (`fiql.go:262`), which parses the bool first then rejects non-EQ ops. This is consistent precedent, not a defect: the op-rejection error is reachable only when the value happens to parse. Fix: rewrote the disallowed-op tests to use a valid canonical UUID with the disallowed operator, isolating the op-validation path. Worth knowing for any future "unsupported operator on parsed type" test — exercise it with a value that parses cleanly.

### [Implementer] Postbuild hook fires per-Edit, not per-batch
Adding the `uuid` import to `fiql.go` in one edit triggered `harness hook-postbuild`, which ran `make gen-basic`, which compiled `fiql.go` with an unused import — hard fail. The `FIQLUUID` type that consumed the import was a separate edit. Lesson: edits that cross a compile boundary (introducing an import + the symbol that uses it) must be batched into a single Edit call, OR the new symbol must be added before the import. Handled here by following up immediately with the type-adding edit; if it had been a longer pause, the working tree would be broken. Same hazard applies to any future split between "infrastructure" and "use site" edits.

### [Implementer] No template change required — confirmed by zero-diff regen
The plan predicted that `template/fiql.tmpl:54-83`'s else-branch (the catchall for non-Enum kinds) would handle UUID without edits. Confirmed in practice: after flipping `fieldFIQLKindFn` to return `"UUID"`, `make gen` produced exactly the expected `entdomain.FIQLUUID[predicate.User]{EQ: user.ExternalIDEQ, NEQ: user.ExternalIDNEQ}` registry entry, and a second `make gen` showed zero diff. The template's else-branch is a true generic over `(kind, ops)` — any future scalar type with EQ/NEQ/etc. wiring can be added the same way (runtime type + kind string flip, no template touch).

<!-- approved -->

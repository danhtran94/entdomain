# ENTD-004-FIQL: null handling (`=is=null` / `=is=notnull`)

| Field      | Value                                                  |
|------------|--------------------------------------------------------|
| Status     | Done                                                   |
| Created    | 2026-04-19                                             |
| Assignee   | danhtran94                                             |
| Source     | [scope-fiql-null-handling](../docs/scope-fiql-null-handling.md) |
| Blocked by | —                                                      |

## Goal
Wire FIQL `=is=null` and `=is=notnull` end-to-end so callers can express IS NULL / IS NOT NULL filters that today aren't representable (chained NEQ doesn't substitute, per SQL three-valued logic). Adds two FIQLOp constants (`IsNull`, `NotNull` with synthetic string values), a parser normalization step that rewrites `=is=`+value into the right internal op, two new zero-arg slot fields per FIQL field type, generator emission gated on `Optional() || Nillable()` (codegen error otherwise), an `applyNullTyped` helper for the dispatch boilerplate, and a round-trip example wiring `bio` (optional string) plus integration tests asserting the SQL contains `IS NULL` / `IS NOT NULL`.

## Problem

    Today (no way to express IS NULL):
      bio!=foo;bio!=bar              -- SQL evaluates NEQ on NULL bio as NULL
                                     -- → not TRUE → row excluded
                                     -- so this filters OUT the rows the user
                                     -- actually wants to see

    Want (FIQL standard, what every other RSQL/HATEOAS impl supports):
      bio=is=null                    -- WHERE bio IS NULL
      bio=is=notnull                 -- WHERE bio IS NOT NULL

ent already generates the predicates (`user.BioIsNil`, `user.BioNotNil`) for every optional / nillable field — they're sitting unwired. The only blockers are wire-format support in the parser and a slot shape in the runtime field types.

## Solution: synthetic FIQLOp constants + parser normalization + optionality gate

    After:
      parseComparison: read op (=is=), read value, then normalize:
        op == Is && value == "null"    → op = IsNull,    value = ""
        op == Is && value == "notnull" → op = NotNull,   value = ""
        op == Is && value == anything  → "unknown =is= value %q — valid: null, notnull"
        ↳ downstream apply only ever sees IsNull / NotNull, never Is

      Each FIQL field type gains:
        IsNil  func() P
        NotNil func() P
      ↳ apply: op IsNull  → f.IsNil()  (or "operator =is=null not allowed on this <kind> field")
                NotNull → f.NotNil() (same shape)

      Generator: emit IsNil/NotNil slot only when:
        annotation includes the matching FIQLOp AND
        f.Optional || f.Nillable
      ↳ if annotated on non-optional → codegen error naming the field

### Components

**Parser changes** in `fiql.go`:
- Two FIQLOp constants with synthetic string values:
  ```go
  IsNull  FIQLOp = "=is=null"     // never appears on the wire literally
  NotNull FIQLOp = "=is=notnull"  // produced by parser normalization
  ```
- Internal `Is FIQLOp = "=is="` constant (unexported or commented as parser-internal) — the wire-form identifier; never reaches `apply`.
- `readOp` recognises `=is=` alongside the existing extended ops.
- `parseComparison` adds a normalization block after `readValue`: if `op == Is`, switch on value to derive `IsNull` / `NotNull`, or error on unknown value.

**Runtime field-type slots** in `fiql.go`:
- New zero-arg slot fields on every typed FIQL field struct (`FIQLString`, `FIQLInt`, `FIQLFloat`, `FIQLTime`, `FIQLBool`, `FIQLEnum`, `FIQLUUID`):
  ```go
  IsNil  func() P
  NotNil func() P
  ```
- Each `apply` method handles the `IsNull` / `NotNull` op via a shared `applyNullTyped` helper (factor early — small DRY win, ready to absorb growth).

**Generator changes**:
- `template/fiql.tmpl` else-branch gains two new `{{ if isOp ... }}` branches emitting `IsNil` / `NotNil` slots.
- `kinds.go` (or template-level helper) gains an optionality check: if the field is annotated with `IsNull` / `NotNull` but is non-optional/non-nillable, the codegen returns an error with the field name and the requirement (`Optional() or Nillable()`).
- Op-name registry (`opByName`) gains two entries: `"IsNull": IsNull`, `"NotNull": NotNull`.

**Annotation API**:
- No new function signature. `entdomain.FIQL(...)` already takes variadic `FIQLOp` — schema authors write `entdomain.FIQL(entdomain.IsNull, entdomain.NotNull)`.

### Why not alternatives

| Approach | Verdict |
|---|---|
| **Synthetic FIQLOp constants + parser normalization (Option C from discussion)** | Chosen. One wire op, two internal ops, parser does the rewriting once. Apply paths stay simple. |
| `=isnull=` / `=notnull=` as no-value wire ops (Option A) | Rejected. Would force a parser branch to skip `readValue` for some ops — a new "no-value op" shape. Equal complexity; the wire form is uglier in URLs. |
| Single `=is=` op + value-based dispatch in apply (no normalization) | Rejected. Would push value-vocabulary error wording into every per-type apply, no place to cache the validation, no clean way to gate annotations per direction. |
| Auto-enable on all optional fields (no annotation needed) | Rejected per scope decision — explicit allow keeps schemas self-documenting and avoids silently exposing nullness to API callers who might not want it. |
| Single `entdomain.Is` annotation enabling both directions | Rejected per scope decision — split annotation lets schema authors express "allow IsNull but not NotNull" for fields where that distinction matters. |

## Workstreams

### 1. Parser + op constants

Foundational. Adds the two synthetic constants, the wire-form recogniser, the normalization step.

| # | Task | File | Status |
|---|------|------|--------|
| 1.1 | Add `Is` (parser-internal), `IsNull = "=is=null"`, `NotNull = "=is=notnull"` to the `FIQLOp` constant block; add `validIsValues = []string{"null", "notnull"}` near them | `fiql.go` | [x] |
| 1.2 | Update `readOp` switch to recognise `=is=` alongside the existing extended ops | `fiql.go` | [x] |
| 1.3 | Add normalization block in `parseComparison` after `readValue`: switch on value for `op == Is`, rewrite to `IsNull` / `NotNull` or return `unknown =is= value %q — valid: null, notnull` | `fiql.go` | [x] |
| 1.4 | Add `IsNull` and `NotNull` entries to `opByName` registry | `fiql.go` | [x] |
| 1.5 | Extend `TestOpRegistryCovered` `wantOps` slice to include `IsNull`, `NotNull` (inverse drift check catches stale entries automatically) | `fiql_internal_test.go` | [x] |

**Key details:** `Is` is documented as parser-internal — comment near the constant declaration explains it never reaches `apply` and exists only for `readOp` to surface from the wire. The error message wording matches the existing `unknown enum value` shape so the test assertions look familiar.

### 2. Runtime field-type slots + apply dispatch

| # | Task | File | Status |
|---|------|------|--------|
| 2.1 | Add `applyNullTyped[P Predicate](op FIQLOp, isNilFn, notNilFn func() P, fieldLabel string) (P, error)` helper covering the IsNull/NotNull dispatch + nil-fn errors | `fiql.go` | [x] |
| 2.2 | Add `IsNil func() P` / `NotNil func() P` to `FIQLString[P]`; extend `apply` to dispatch via `applyNullTyped` for `IsNull` / `NotNull` | `fiql.go` | [x] |
| 2.3 | Same additions to `FIQLInt[P]` | `fiql.go` | [x] |
| 2.4 | Same additions to `FIQLFloat[P]` | `fiql.go` | [x] |
| 2.5 | Same additions to `FIQLTime[P]` | `fiql.go` | [x] |
| 2.6 | Same additions to `FIQLBool[P]` (note: bool can be Optional too — `*bool` field) | `fiql.go` | [x] |
| 2.7 | Same additions to `FIQLEnum[P]` (slot fields are zero-arg `func() P`, no map needed) | `fiql.go` | [x] |
| 2.8 | Same additions to `FIQLUUID[P]` | `fiql.go` | [x] |

**Key details:** `applyNullTyped` is short — early factor so all 7 per-type apply methods stay uniform. Sketch:

```go
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
```

Each per-type `apply` adds one branch: `case IsNull, NotNull: return applyNullTyped(op, f.IsNil, f.NotNil, "<kind>")`.

### 3. Generator template + optionality gate

| # | Task | File | Status |
|---|------|------|--------|
| 3.1 | Add `{{ if isOp "IsNull" $op }}` / `{{ if isOp "NotNull" $op }}` branches to `template/fiql.tmpl` else-branch, emitting `IsNil: <pred>.<Field>IsNil` / `NotNil: <pred>.<Field>NotNil` | `template/fiql.tmpl` | [x] |
| 3.2 | Add same two branches to the Enum sub-template (same pattern, slots are still zero-arg) | `template/fiql.tmpl` | [x] |
| 3.3 | Add codegen-time gate in `template.go:fieldFIQLAnnotationFn` (or a sibling helper) that errors when annotation includes `IsNull` / `NotNull` but `!f.Optional && !f.Nillable`. Error: `field <Type>.<Field>: =is= operator requires Optional() or Nillable() — ent generates IsNil/NotNil predicates only for those` | `template.go` | [x] |
| 3.4 | Unit test `TestFieldFIQLAnnotation_NullOnNonOptional` constructing a non-optional field with the IsNull annotation; asserts the codegen error fires | `template_internal_test.go` | [x] |

**Key details:** The codegen gate fires inside the template func chain; ent's codegen surfaces the error at `make gen` time as a build failure. Wording matches the scope's specification verbatim. The Enum branch needs the new ops because enum fields can be `Optional()` too — same nullness semantics.

### 4. Example schema annotation

| # | Task | File | Status |
|---|------|------|--------|
| 4.1 | Add `entdomain.IsNull, entdomain.NotNull` to the existing FIQL annotation on the `bio` field (currently has no FIQL annotation — needs an annotation block added) | `examples/basic/ent/schema/user.go` | [x] |

**Key details:** `bio` is `field.String("bio").Optional()` today with no annotations — the natural test target since it's the only existing optional field with a clean shape. If the user prefers another field, swap. Annotation: `entdomain.Field(entdomain.FIQL(entdomain.IsNull, entdomain.NotNull))`.

### 5. Codegen

| # | Task | File | Status |
|---|------|------|--------|
| 5.1 | `make gen-basic` — confirm `examples/basic/ent/fiql.go` shows a new `bio` entry with `IsNil: user.BioIsNil, NotNil: user.BioNotNil` | `examples/basic/ent/fiql.go` (output) | [x] |
| 5.2 | `make gen` — second run produces zero diff | all generated outputs | [x] |

### 6. Tests

| # | Task | Status |
|---|------|--------|
| 6.1 | Parser unit tests in `fiql_test.go`: `bio=is=null` parses to op=IsNull/value=""; `bio=is=notnull` parses to op=NotNull; `bio=is=maybe` errors with "unknown =is= value 'maybe'"; `bio=is=` (empty value) errors with the existing empty-value-for-field error | [x] |
| 6.2 | Internal regression test in `fiql_internal_test.go`: assert `apply(Is, "null")` returns `operator "=is=" not supported by applyNullTyped` (i.e., the parser-internal `Is` op is unreachable from `apply` — guards the contract that normalization always runs before dispatch) | [x] |
| 6.3 | Runtime unit tests in `fiql_test.go`: `FIQLString.apply(IsNull, "")` calls `IsNil`; same for `NotNull`; `IsNull` with nil `f.IsNil` returns the not-allowed error. Repeat once for one numeric type (Int) and one for UUID to cover the helper shape across slot types | [x] |
| 6.4 | Integration tests in `examples/basic/ent/fiql_test.go`: `bio=is=null` produces SQL containing `IS NULL`; `bio=is=notnull` produces `IS NOT NULL`; composition `name==john,bio=is=null` produces OR-joined output | [x] |
| 6.5 | Op registry coverage test (existing `TestOpRegistryCovered`) extended via WS1.5 — verifies `IsNull`/`NotNull` are registered and inverse-checked | [x] |
| 6.6 | Grouping regression test: confirm `(name==john,bio=is=null);score=gt=0` still parses correctly with the new parser normalization in place | [x] |
| 6.7 | Update stale assertions in `template_test.go` (`bio_not_in_registry`) and `examples/basic/ent/fiql_test.go` (`TestUserFIQL_UnannotatedField`) — both assumed bio was unannotated pre-WS4. See Discoveries. | [x] |

### 7. Documentation

| # | Task | Status |
|---|------|--------|
| 7.1 | `README.md` Operator Constants table — add `IsNull` (`=is=null`) and `NotNull` (`=is=notnull`) rows with "Valid for" listing all field kinds | [x] |
| 7.2 | `README.md` — add a "Filtering by nullness" subsection under FIQL Filtering. Show schema annotation requirement (`Optional()` + `entdomain.FIQL(entdomain.IsNull, entdomain.NotNull)`) and an example query | [x] |
| 7.3 | `README.md` Generated Code example — show the `bio` line in `UserFIQLFields` with `IsNil: user.BioIsNil, NotNil: user.BioNotNil` | [x] |
| 7.4 | `README.md` Known Limitations — note (a) non-optional fields cannot be annotated (codegen error), (b) strict `null`/`notnull` value vocabulary doesn't accept aliases like `nil` or `empty`, (c) implicit `""` → null conversion is NOT supported (use `bio==,bio=is=null` to express "empty or null"), (d) SQL three-valued logic still applies — `bio!=foo` excludes NULL bio rows by SQL definition | [x] |

## Design Decisions

### Synthetic FIQLOp constants over a value-vocabulary in `apply`
Two distinct internal ops keep the apply-time logic per-op (matches every other FIQL operator pattern). The parser normalizes once; downstream code is uniform with the rest of the codebase. The "one wire op → two internal ops" mapping is novel but contained.

### `Is` constant exists for `readOp` but is parser-internal
Exposing `Is` as a public FIQLOp would invite annotations like `entdomain.FIQL(entdomain.Is)` that the generator would have to special-case. Cleaner to leave it parser-internal — schema authors only see `IsNull` / `NotNull`.

### Codegen error on non-optional + IsNull annotation
Loud failure at `make gen` time. The alternative (silent skip) would mean a schema author who annotates a required field gets zero error, zero generated wiring, and zero indication anything went wrong — same defect class ENTD-001 fixed for UUID GoType. The error message names the ent requirement (`Optional() or Nillable()`) so the fix is self-evident.

### `applyNullTyped` helper despite small size
Six lines of dispatch boilerplate × seven types = forty-two lines of duplication if not factored. The helper is small enough to skip but the precedent (`applyListTyped` from ENTD-003) says the cost of factoring is low and the readability win compounds. Land the helper from day one rather than refactoring later.

### Both `Optional()` AND `Nillable()` allowed
ent treats both flags as "this field can be NULL in the database" — `IsNil` / `NotNil` predicates exist for either. Gating on both is correct; gating on only one would surprise schema authors who use `Nillable()` without `Optional()` (rare but valid).

## What Stays Unchanged
- Existing FIQL grammar (`==`, `!=`, `=gt=`, etc.) — no behavior change
- `parseAtom` grouping path — `(expr)` for sub-expressions stays exactly as is
- `readListValue` / `applyListTyped` — `=in=` / `=out=` paths unchanged
- `FIQLEnum`'s map-based composition for `=in=` / `=out=` — orthogonal to nullness
- `examples/custom` — no schema changes (no optional fields with FIQL there today)
- Public API of every existing FIQL field type (additive only — new fields with zero-value `nil`)
- Proto generation pipeline — orthogonal to FIQL nullness
- `entdomain.FIQL(...)` annotation function signature — variadic FIQLOp, no change

## Implementation Order

    1. WS1: parser + op constants     ← unblocks WS2, WS3, WS6
    2. WS2: runtime slots + helper    ← depends on WS1 (op constants)
    3. WS3: template + codegen gate   ← depends on WS2 (slot fields exist)
    4. WS4: schema annotation         ← depends on WS1 (op constants)
    5. WS5: regenerate                ← depends on WS1+WS2+WS3+WS4
    6. WS6: tests                     ← parser tests after WS1, runtime after WS2, integration after WS5
    7. WS7: README                    ← last; reflects shipped behavior

## Notes
- `make gen` is the canary — run after every workstream task that touches Go source or the template
- Reuses existing `applyListTyped` precedent for the helper-extraction pattern; if `applyNullTyped` shape diverges much from it during implementation, capture the why in Discoveries
- Watch for parser regressions on the existing extended ops — the new `Is` recognition shouldn't change `=gt=` / `=in=` behavior, but `TestParseFIQL_*` is the canary
- The codegen-error path (WS3.3) is the load-bearing safety net — if a maintainer adds a new FIQL slot type later and forgets the optionality gate, the test in WS3.4 catches it

## Discoveries & Decisions During Implementation
(Filled during execution — never during planning)

### [Implementer] Postbuild hook fires per-Edit (recurring lesson)
Hit it twice in this job — once when adding the `=is=` op handling required `fmt` import in `template.go` (same hazard as ENTD-001/002/003), once when adding `require` import to `fiql_internal_test.go` for the new gate test. Pattern remains: any edit that introduces a new package symbol or import must batch with its first use, or add the import in a separate edit BEFORE the use site. Worth treating as ground truth for this codebase — it costs nothing to land the import first.

### [Implementer] hook-plan blocks edits to files not in active job tasks
When updating stale `template_test.go` and `README.md` assertions, the hook-plan validator rejected the edits because those file paths weren't named in any active job task. For `README.md`, the prior job (ENTD-003) had used `README` (no extension) in task descriptions and worked — the validator may have changed since, or the matching is stricter when the file has no FIQL-specific keyword to attach to. Resolution: explicitly include the literal `README.md` filename in WS7 task descriptions so the matcher finds it. Pattern: when a job touches a non-source file (test files in different packages, top-level docs, config), name the literal path in the task description.

### [Implementer] Two pre-existing tests asserted "bio is unannotated" — need updating after WS4
After WS4 added the IsNull/NotNull annotation to `bio`, `template_test.go:371` (`bio_not_in_registry`) and `examples/basic/ent/fiql_test.go:226` (`TestUserFIQL_UnannotatedField` using `bio==hello`) failed because both assumed bio had no FIQL annotation. The job's WS6 task list didn't anticipate touching either file — these are *stale assertions*, not new tests. Rewriting `template_test.go` to assert bio appears WITH the IsNil/NotNil slots (locks in the new shape) and switching `TestUserFIQL_UnannotatedField` to use a different existing-but-unannotated field (e.g. `username`, which has `entdomain.SkipProto()` but no FIQL annotation). Anytime an example schema annotation changes, scan the existing test files for assertions tied to the prior state.

<!-- approved -->

# ENTD-002-FIQL: Set membership operators (`=in=` / `=out=`)

| Field      | Value                                                  |
|------------|--------------------------------------------------------|
| Status     | Done                                                   |
| Created    | 2026-04-19                                             |
| Assignee   | danhtran94                                             |
| Source     | [scope-fiql-set-membership](../docs/scope-fiql-set-membership.md) |
| Blocked by | —                                                      |

## Goal
Wire FIQL `=in=` (set membership) and `=out=` (set exclusion) end-to-end. Adds two operator constants, a parenthesised-list value reader (`readListValue`) gated to those operators only, `In`/`NotIn` slots on every set-supporting runtime field type (`FIQLString`/`Int`/`Float`/`UUID`), enum membership built from the existing EQ/NEQ maps at apply time, a 100-element list cap, and template emission via two new operator branches in the existing `FIQL{{kind}}` else-branch. Bool and time stay rejected; the parser's existing grouping path is untouched.

## Problem

    Current (verbose):
      status==open,status==pending,status==closed                ← chained OR-EQ
      status!=open;status!=pending;status!=closed                ← chained AND-NEQ

    Want (FIQL standard, what every REST client expects):
      status=in=(open,pending,closed)
      status=out=(open,closed)

The blocker is the parser, not ent — `user.NameIn(vs ...string)` and `user.NameNotIn(...)` already exist for every typed field. `readValue` (`fiql.go:507-517`) stops on the first `,` or `)`, so the parenthesised list form is unparseable today.

## Solution: dedicated list reader + variadic field slots

    After fix:
      parseComparison reads op
        → if op is In/NotIn → readListValue (consumes (...) verbatim)
        → otherwise         → readValue (unchanged)

      apply(In, "(a,b,c)") on FIQLString:
        → strip parens, split on ',', enforce cap, call f.In("a", "b", "c")
        → ent emits SQL IN (?, ?, ?) ✓

      apply(In, "(active,closed)") on FIQLEnum:
        → look up each value in f.EQ map → orPreds(p1, p2, ...) ✓
        → no new template wiring; apply-time composition

### Components

**Parser changes** in `fiql.go`:
- Two new op constants: `In = "=in="`, `NotIn = "=out="`
- `maxFIQLListValues = 100` constant (precedent: `maxFIQLDepth = 50`)
- `readListValue() (string, error)` — consumes `(` to matching `)`, returns the literal substring including parens
- `parseComparison` dispatches to `readListValue` when the op is In/NotIn

**Runtime field types** in `fiql.go`:
- `FIQLString[P]`, `FIQLInt[P]`, `FIQLFloat[P]`, `FIQLUUID[P]` each gain `In func(...T) P` and `NotIn func(...T) P`
- Each `apply` method handles `In`/`NotIn` by stripping parens, splitting on `,`, parsing each element via the same per-type parser as `==`, enforcing `maxFIQLListValues`, then calling `f.In(parsed...)` / `f.NotIn(parsed...)`
- `FIQLEnum.apply` handles `In`/`NotIn` by looking up each value in the existing `EQ` (or `NEQ`) map and combining via `orPreds` (or `andPreds` for `=out=`); no new struct fields required
- `FIQLBool.apply` explicitly rejects `In`/`NotIn` with the existing "operator not allowed on bool field" error shape

**Template changes** in `template/fiql.tmpl`:
- Add two new `{{- if eq $op }}` branches to the else-branch (`fiql.tmpl:54-83`):
  - `=in=` → `In: {{ lower $n.Name }}.{{ $f.StructField }}In,`
  - `=out=` → `NotIn: {{ lower $n.Name }}.{{ $f.StructField }}NotIn,`
- Enum branch (`fiql.tmpl:36-53`) stays unchanged — set membership is built at apply time from the existing maps

### Why not alternatives

| Approach | Verdict |
|---|---|
| **`=in=(a,b,c)` standard syntax + parser dispatch on op** | Chosen. FIQL-standard, clearly bounded parser change, every type uses ent's native `FieldIn`. |
| Pipe-separated values (`=in=a\|b\|c`) | Rejected. Non-standard; breaks any value containing `\|`; gains nothing — parser change is needed either way for the cap enforcement. |
| Reuse OR-of-EQ at template level (no new operators) | Rejected. Already works today via chained `==`; fails to match the FIQL spec; doesn't produce SQL `IN (...)` for typed fields. |
| Per-type variadic struct fields with type-parameterised `FIQLEnum[P, E]` | Rejected. Adding a second type parameter to FIQLEnum breaks the public API; OR-of-EQ at apply time is functionally identical and SQL-equivalent on every supported engine. |

## Workstreams

### 1. Operator constants & parser

Foundational — adds the two op constants, the cap constant, the list-value reader, and the dispatch in `parseComparison`. Unblocks every other workstream.

| # | Task | File | Status |
|---|------|------|--------|
| 1.1 | Add `In = "=in="` and `NotIn = "=out="` to the `FIQLOp` constant block | `fiql.go` | [x] |
| 1.2 | Add `maxFIQLListValues = 100` constant near `maxFIQLDepth` | `fiql.go` | [x] |
| 1.3 | Add `readListValue() (string, error)` — consumes `(...)` from current pos, returns substring including parens; errors on missing `(`, missing `)`, or empty list `()` | `fiql.go` | [x] |
| 1.4 | Update `parseComparison` to call `readListValue` when op is `In`/`NotIn`, otherwise `readValue` (unchanged path for all other ops) | `fiql.go` | [x] |
| 1.5 | Update `readOp` switch to recognise `=in=` and `=out=` alongside the existing extended ops | `fiql.go` | [x] |

**Key details:** `readListValue` returns the value WITH parens (e.g. `"(a,b,c)"`). The per-type `apply` strips them. Empty list error wording: `empty value list for =in= operator at position N`. Cap exceeded error wording: `=in= list exceeds maximum of 100 values at position N`. Both errors wrap the failing position so callers can locate the bad term.

### 2. Runtime field-type slots and apply dispatch

| # | Task | File | Status |
|---|------|------|--------|
| 2.1 | Add `In func(...string) P` and `NotIn func(...string) P` to `FIQLString[P]`; extend `apply` to strip parens, split, enforce cap, call `f.In(parts...)` / `f.NotIn(parts...)` | `fiql.go` | [x] |
| 2.2 | Add analogous `In`/`NotIn` to `FIQLInt[P]`; extend `apply` (parse each via `strconv.Atoi`, error on first bad element with element value named) | `fiql.go` | [x] |
| 2.3 | Add analogous `In`/`NotIn` to `FIQLFloat[P]`; extend `apply` (parse via `strconv.ParseFloat`) | `fiql.go` | [x] |
| 2.4 | Add analogous `In`/`NotIn` to `FIQLUUID[P]`; extend `apply` (parse via `uuid.Parse`) | `fiql.go` | [x] |
| 2.5 | Extend `FIQLEnum.apply` to handle `In`/`NotIn` by looking up each value in the existing `EQ`/`NEQ` map and combining via `orPreds`/`andPreds`; no new struct fields | `fiql.go` | [x] |
| 2.6 | Confirm `FIQLBool.apply` rejects `In`/`NotIn` with the existing default-case error (no code change expected; verify) | `fiql.go` | [x] |

**Key details:** Factor the parse-list-value helper once. Sketch:

```go
func parseInListValue(val string, max int) ([]string, error) {
    if len(val) < 2 || val[0] != '(' || val[len(val)-1] != ')' {
        return nil, fmt.Errorf("malformed list value %q (expected (a,b,c))", val)
    }
    inner := val[1 : len(val)-1]
    if inner == "" {
        return nil, fmt.Errorf("empty value list")
    }
    parts := strings.Split(inner, ",")
    if len(parts) > max {
        return nil, fmt.Errorf("list exceeds maximum of %d values", max)
    }
    return parts, nil
}
```

Each typed `apply` calls this then maps via the per-type parser. Enum reuses it but maps via the EQ/NEQ map lookup with the same "unknown enum value" error wording the existing code produces.

### 3. Generator template

Wire emission so annotated `In`/`NotIn` ops produce `In: x.FieldIn` / `NotIn: x.FieldNotIn` lines.

| # | Task | File | Status |
|---|------|------|--------|
| 3.1 | Add two new `{{- if eq $op "=in=" }}` / `{{- if eq $op "=out=" }}` blocks to `template/fiql.tmpl` else-branch (lines 54-83) | `template/fiql.tmpl` | [x] |
| 3.2 | Verify enum branch (lines 36-53) needs no template edit — set membership is apply-time | `template/fiql.tmpl` | [x] |

**Key details:** The else-branch emits `entdomain.FIQL{{ $kind }}[predicate.X]{ ...op fields... }`. Two new lines to add, mirroring the existing `EQ`/`NEQ` shape. Enum stays as-is because `FIQLEnum` keeps the same `EQ`/`NEQ` map fields and dispatches `In`/`NotIn` at apply time.

### 4. Example schema annotations

| # | Task | File | Status |
|---|------|------|--------|
| 4.1 | Extend `score` field annotation to include `entdomain.In, entdomain.NotIn` | `examples/basic/ent/schema/user.go` | [x] |
| 4.2 | Extend `status` field annotation to include `entdomain.In, entdomain.NotIn` | `examples/basic/ent/schema/user.go` | [x] |

### 5. Codegen

| # | Task | File | Status |
|---|------|------|--------|
| 5.1 | `make gen-basic` — confirm `examples/basic/ent/fiql.go` shows `In: user.ScoreIn, NotIn: user.ScoreNotIn` for score; status entry unchanged structurally (still `EQ`/`NEQ` maps) | `examples/basic/ent/fiql.go` (output) | [x] |
| 5.2 | `make gen` — second run shows zero diff | all generated outputs | [x] |

### 6. Testing

| # | Task | Status |
|---|------|--------|
| 6.1 | Parser unit tests in `fiql_test.go`: `readListValue` happy path, missing `(`, missing `)`, empty `()`, cap exceeded (101 values) | [x] |
| 6.2 | Runtime unit tests in `fiql_test.go`: `FIQLString.apply(In, ...)` with valid list, invalid element, exceeds cap; same for `FIQLInt`, `FIQLFloat`, `FIQLUUID`, `FIQLEnum` (including unknown-enum-value error) | [x] |
| 6.3 | Runtime unit test: `FIQLBool.apply(In, ...)` returns the operator-not-allowed error | [x] |
| 6.4 | Regression test in `fiql_test.go`: `(name==a,name==b)` still parses as grouped-OR (proves parser grouping path is untouched) | [x] |
| 6.5 | Integration tests in `examples/basic/ent/fiql_test.go`: `score=in=(10,20,30)` produces SQL containing `IN (?`; `score=out=(10,20)` produces `NOT IN (?`; `status=in=(active,inactive)` produces `OR`-joined EQ; `status=out=(active)` produces NEQ; empty `()` and bad-value error paths | [x] |

### 7. Documentation

| # | Task | Status |
|---|------|--------|
| 7.1 | README Operator Constants table — add `In` (`=in=`) and `NotIn` (`=out=`) rows; "Valid for" lists string, int, float, enum, uuid (excludes bool, time) | [x] |
| 7.2 | README FIQL syntax block (~L324) — add the `field=in=(a,b,c)` and `field=out=(a,b,c)` syntax lines | [x] |
| 7.3 | README Generated Code example (~L370) — show `In: user.ScoreIn, NotIn: user.ScoreNotIn` on the `score` line | [x] |
| 7.4 | README Known Limitations — add: (a) bool and time fields don't support set membership; (b) values containing `,` or `)` cannot be used in lists (use chained `==` for those); (c) max 100 elements per list | [x] |

## Design Decisions

### `=in=(a,b,c)` standard syntax over a non-standard separator
FIQL spec compliance is the goal. The parser change is contained (one new helper, one dispatch site) and reusing the standard form means existing FIQL clients work without translation.

### Hard 100-element cap, not configurable
Default that bounds parse cost and SQL planner cost without user friction for typical filters. Configurability is a clean follow-up if a real user need surfaces; preemptively exposing knobs invites footguns.

### Empty `()` is a parse error, not "always-false"
Silent always-false hides upstream bugs (CSV import with zero rows shouldn't silently match nothing — it should fail loudly). Loud parse error is the safer default.

### Enum set membership composed from existing EQ/NEQ maps
Avoids adding a second type parameter to `FIQLEnum` or duplicating the enum→typed-value mapping. SQL plan difference (OR-of-EQ vs. `IN`) is collapsed by every supported DB engine.

## What Stays Unchanged
- `parseAtom`'s grouping path (`fiql.go:412-432`) — `(expr)` for sub-expression grouping is reachable only outside a comparison context; the new `readListValue` is reachable only after an `In`/`NotIn` operator
- `readValue` (`fiql.go:507-517`) — used for every other op, no behavior change
- `FIQLEnum` struct shape — no new fields; new behavior in apply only
- `FIQLBool` — no new fields, no apply change beyond verifying the existing default-case rejection works for `In`/`NotIn`
- `examples/custom` — no annotation changes; will pick up the runtime types when imported
- Proto generation pipeline — unrelated; untouched

## Implementation Order

    1. WS1: parser & op constants    ← unblocks WS2, WS3, WS6
    2. WS2: runtime types            ← depends on WS1 (op constants)
    3. WS3: template                 ← depends on WS2 (slot fields exist)
    4. WS4: schema annotations       ← depends on WS1 (op constants)
    5. WS5: regenerate               ← depends on WS1+WS2+WS3+WS4
    6. WS6: tests                    ← parser unit (after WS1), runtime unit (after WS2), integration (after WS5)
    7. WS7: README                   ← last; after behavior is verified

## Notes
- Reuses existing `orPreds` / `andPreds` helpers (`fiql.go:519-527`)
- `github.com/google/uuid` already imported (from ENTD-001)
- `make gen` is the CI gate — must produce zero diff after a clean rerun
- Verification cadence: `go build ./...` + `make test` after every code edit; `make gen` after WS4
- Watch for parser regressions on grouped expressions — the dedicated regression test (WS6.4) is the canary

## Discoveries & Decisions During Implementation
(Filled during execution — never during planning)

### [Implementer] FIQLBool.apply parsed value before checking op — needed reordering
The original `FIQLBool.apply` (`fiql.go:341-354`) called `strconv.ParseBool(val)` first, then validated the operator. With `=in=(true,false)` as input, ParseBool would fail with "invalid bool value", masking the real error (`=in=` is unsupported on bool). Fixed by moving the op check ahead of the parse so the rejection error is reachable. Same hazard exists in `FIQLInt`/`Float`/`UUID`/`Time` apply methods — they parse first too — but for those, the In/NotIn handling is added as a top-level branch BEFORE the parse, so the issue doesn't surface. Worth knowing for any future "this op is unsupported on this type" diagnostic: it has to fire before any value-parsing code.

### [Implementer] `pred(nil)` panics through `andPreds`/`orPreds`
The grouping regression test (WS6.4) initially called `pred(nil)` like the simple-EQ tests do. That works for a single predicate function but panics inside `sql.AndPredicates`/`OrPredicates` because they call methods on the selector. Fixed by passing a real `sql.Dialect("sqlite3").Select(...)` selector. Lesson: any test that builds a composite predicate (AND/OR) cannot use `nil` as the selector — give it a real one. The simple-record-via-side-effect pattern only works with single-predicate paths.

### [Implementer] Enum struct shape unchanged — apply-time composition validates the design
The scope decision to skip new `FIQLEnum` struct fields and compose set membership from existing `EQ`/`NEQ` maps proved correct in practice: the template needed zero changes for enums, the regenerated `examples/basic/ent/fiql.go` `status` entry stayed structurally identical, and the SQL output is the standard OR-of-EQ pattern that the planner collapses to an IN scan. No type-parameter expansion of `FIQLEnum` was needed. Confirmed by direct integration test (`TestUserFIQL_EnumIn`).

<!-- approved -->

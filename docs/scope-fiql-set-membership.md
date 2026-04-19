# Scope: FIQL set membership (`=in=` / `=out=`)

| Field    | Value                |
|----------|----------------------|
| Status   | Accepted             |
| Created  | 2026-04-19           |
| Author   | danhtran94           |

## Problem

FIQL today has no set-membership operator. Users wanting "field is one of N values" must chain equality terms:

    status==open,status==pending,status==closed

This is verbose, scales linearly with the value count, and produces an OR-of-EQ SQL plan instead of the SQL `IN (...)` plan ent's `FieldIn` predicates already generate. The exclusion variant (`field is none of N values`) is even uglier:

    status!=open;status!=pending;status!=closed

Both patterns also miss the standard FIQL `=in=` / `=out=` operators that REST clients commonly expect (every other typed FIQL field already follows the FIQL spec — set-membership is the conspicuous gap).

The blocker isn't ent — `user.NameIn(vs ...string)` and `user.NameNotIn(...)` already exist for every typed field. The blocker is the parser: `readValue` (`fiql.go:507-517`) stops on the first `,` or `)`, so `field=in=(a,b,c)` is unparseable today. The runtime field types (`FIQLString`, `FIQLInt`, `FIQLFloat`, `FIQLEnum`, `FIQLUUID`) also have no slot to receive a list.

## Success Criteria

1. **Two new operator constants** in `fiql.go`: `In FIQLOp = "=in="` and `NotIn FIQLOp = "=out="`. Both accepted by the FIQL annotation system (`entdomain.FIQL(entdomain.In, entdomain.NotIn)`).

2. **Parser handles `(a,b,c)` value form.** When the comparison operator is `In` or `NotIn`, the parser reads a parenthesised value via a new `readListValue` helper that consumes from `(` to the matching `)`. Inner commas are part of the value list, not OR separators. Nested parens are not supported (lists are flat). Standard `==`, `!=`, `=gt=` etc. continue using the existing `readValue` unchanged.

3. **Field-type slots added** to runtime types in `fiql.go`:
   - `FIQLString[P]` gains `In func(...string) P` and `NotIn func(...string) P`
   - `FIQLInt[P]`, `FIQLFloat[P]`, `FIQLUUID[P]` gain analogous `In`/`NotIn` with their respective scalar types
   - `FIQLEnum[P]` does NOT gain new fields. Set-membership for enums is built by OR-ing entries from the existing `EQ` map (and AND-ing entries from `NEQ` for `=out=`). Same SQL semantics as the typed `FieldIn`, no new wiring required.

4. **`apply` methods parse the list value** by stripping outer parens and splitting on `,`. Empty lists (`()`) return a clear parse error: `empty value list for =in= operator`. Each element is parsed with the existing per-type parser (e.g. `strconv.Atoi` for int, `uuid.Parse` for UUID); a single bad element fails the whole expression with the offending value named.

5. **List size cap** of 100 elements per `=in=`/`=out=` term — matching the `maxFIQLDepth = 50` precedent. Exceeding the cap returns `=in= list exceeds maximum of 100 values`. Defined as a top-level `maxFIQLListValues` constant for symmetry with `maxFIQLDepth`.

6. **Generator emits the new wirings** in `template/fiql.tmpl`. The else-branch (the `FIQL{{kind}}` instantiation that already handles String/Int/Float/Bool/Time/UUID) gains two new `if eq $op` branches:

       {{- if eq $op "=in=" }}
       In: {{ lower $n.Name }}.{{ $f.StructField }}In,
       {{- end }}
       {{- if eq $op "=out=" }}
       NotIn: {{ lower $n.Name }}.{{ $f.StructField }}NotIn,
       {{- end }}

   The Enum branch is unchanged — set-membership for enums uses the existing EQ/NEQ maps at apply time.

7. **Round-trip example** in `examples/basic`: `status` (enum) and `score` (int) get `entdomain.In` / `entdomain.NotIn` added to their FIQL annotations. New tests in `fiql_test.go` (root) cover the runtime types; new integration tests in `examples/basic/ent/fiql_test.go` assert `score=in=(10,20,30)` produces SQL containing `IN (?` and `status=out=(active,inactive)` produces an AND-of-NEQ.

8. **`make gen` zero-diff after regen.** `make test` passes the new `=in=` / `=out=` tests plus all existing FIQL tests unchanged.

## Out of Scope

- **Bool fields.** `=in=(true,false)` is always-true and `=in=(true)` is identical to `==true`. Adding the slot to `FIQLBool` is dead code. `entdomain.FIQL(entdomain.In, ...)` on a bool field returns the existing "operator not allowed" error.

- **Time fields.** Range queries (`=gt=` / `=le=`) are the right idiom for time. Set-membership over discrete timestamps is a corner case and the value parsing (RFC3339 with embedded `:` and `T`) interacts awkwardly with the comma list separator. Deferred until a concrete user need surfaces.

- **Escape syntax for values containing `,` or `)`.** FIQL standards don't define one. Values with commas or close-parens cannot be used in `=in=`/`=out=` lists; use chained `==` / `!=` for those. Documented in the README.

- **Nested lists or set algebra.** `=in=(a,(b,c))`, `=in=(set1) AND =out=(set2)` collapsed expressions, etc. Not addressed — the existing `;` (AND) and `,` (OR) grammar at the term level still composes membership expressions.

- **Custom GoType UUID set membership.** Inherits the same gate from ENTD-001 — custom-GoType UUID fields are skipped at codegen, including for `=in=`. No new gate logic required.

- **Server-side cap configurability.** The 100-element cap is a hard constant. Making it tunable via annotation or environment is deferred — pick a sensible default first, expose configurability if a user actually needs it.

- **`=in=` value type that differs from the field's scalar type.** Each list element is parsed with the same per-type parser as `==`. No coercion (e.g. `score=in=("10","20")` with stringified ints) — this would diverge from the rest of FIQL's contract.

## Risks

- **Parser regression on existing expressions containing `(...)` for grouping.** The current parser reads `(` only inside `parseAtom` for sub-expressions. The new `readListValue` is reachable only after a `=in=` / `=out=` operator is matched, so the grouping path is untouched. *Mitigation:* keep all existing parser tests green (they already cover precedence and grouping); add a regression test specifically asserting `(name==a,name==b)` still parses as grouped-OR even with the new operator constants present.

- **List-cap defense vs. usability.** A 100-element cap protects parse time and downstream SQL planner cost. But genuine bulk-filter use cases (e.g. "users in this 200-row CSV") will hit it. *Mitigation:* document the cap in the README with the chained-equality fallback for larger sets; revisit if a real user needs >100 (configurable cap is a clean follow-up scope).

- **Empty-list error vs. always-false convention.** Choosing parse-error means `=in=()` from an upstream caller (perhaps an unfiltered CSV import path) hard-fails the request rather than returning zero rows. The opposite choice (silent always-false) hides bugs in caller code. *Mitigation:* parse-error is the louder, less-buggy default; callers handling user input should validate non-empty before constructing the FIQL expression.

- **Different SQL plans for enum `=in=` (OR-of-EQ) vs. typed `=in=` (`IN (...)`)** could surprise reviewers expecting symmetry. *Mitigation:* document the difference in the README's FIQL section; performance is equivalent on every supported DB engine (the planner collapses OR-of-EQ-on-same-column to an IN scan), so this is purely cosmetic in the generated SQL.

- **Living list** — update during implementation as new failure modes surface.

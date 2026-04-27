# Scope: FIQL null handling (`=is=null` / `=is=notnull`)

| Field    | Value                |
|----------|----------------------|
| Status   | Accepted             |
| Created  | 2026-04-19           |
| Author   | danhtran94           |

## Problem

FIQL today has no way to filter on column nullness. ent generates `XxxIsNil()` and `XxxNotNil()` predicate methods for every optional or nillable field — they exist, they're free for the asking, but the FIQL pipeline has no slot to wire them into.

The user-visible consequence is worse than "missing feature." SQL's three-valued logic means a NEQ chain doesn't substitute for an IS NULL check:

    bio!=foo;bio!=bar;bio!=baz   -- excludes rows where bio is NULL

`WHERE bio != 'foo'` evaluates to NULL on a NULL bio, which is not TRUE, which the engine treats as "exclude". A schema author who wants "find users without a bio" cannot express it in FIQL today; the closest workaround silently filters out the rows they actually want.

A second-order consequence: HTTP API consumers expect FIQL to cover this. It's the most common "why doesn't this work" question for any FIQL-shaped query language. Every standards-tracking FIQL implementation (Spring HATEOAS, RSQL-Parser, RSQL-jpa) supports `=is=null` or an equivalent — entdomain is the outlier.

The blocker isn't ent. ent already emits:

    func BioIsNil() predicate.User
    func BioNotNil() predicate.User

for the basic example's `bio` field (optional). The blocker is two missing pieces: a wire format the parser recognises, and runtime field-type slots to receive `func() P` predicates (every existing slot takes at least one value argument).

## Success Criteria

1. **Two new `FIQLOp` constants** in `fiql.go` with synthetic-but-distinct string values:

       IsNull  FIQLOp = "=is=null"
       NotNull FIQLOp = "=is=notnull"

   These never appear on the wire literally — they're internal identifiers produced by the parser after value normalization. The wire form is `field=is=null` / `field=is=notnull` only.

2. **Parser handles the `=is=` wire op + value normalization.** `readOp` recognises `=is=` alongside the existing extended ops. `parseComparison` reads the value via the existing `readValue` (no new reader needed), then if `op == Is`, switches on the value:

   - `"null"` → normalize to `op = IsNull`, `value = ""`
   - `"notnull"` → normalize to `op = NotNull`, `value = ""`
   - anything else → `unknown =is= value %q — valid: null, notnull`

   Downstream `apply` only ever sees the normalized `IsNull` / `NotNull`. Drop the `Is` constant after normalization is wired — it's an internal-only identifier and exposing it would invite confused annotations.

3. **Strict value vocabulary.** Only `null` and `notnull`. Aliases like `nil`, `empty`, `present` are rejected with the same "valid: null, notnull" error. Empty string and whitespace also rejected. Codifies the value list in one constant in `fiql.go` so the error message and the parser switch share a source of truth.

4. **Split annotation API.** Two `FIQLOp` constants `IsNull` and `NotNull` exposed via the existing `entdomain.FIQL(...)` annotation function. Schema author writes:

       field.String("bio").Optional().
           Annotations(entdomain.Field(entdomain.FIQL(
               entdomain.IsNull,
               entdomain.NotNull,
           )))

   Annotating only `IsNull` allows `=is=null` and rejects `=is=notnull` with the existing "operator not allowed" error. Each direction gates independently.

5. **Generator gate on optional/nillable.** `template/fiql.tmpl` emits the `IsNil` / `NotNil` slot only when both conditions hold:
   - The annotation includes the corresponding `FIQLOp`
   - `f.Optional || f.Nillable` is true at codegen

   If the annotation is present but the field is non-optional, the generator emits a `// codegen error` comment AND the `entcgen` run fails with `field <X>.<Y>: =is= operator requires Optional() or Nillable() — ent generates IsNil/NotNil predicates only for those`. Loud failure, not silent skip.

6. **New runtime slot shape — zero-arg predicate functions** — added to every typed FIQL field struct (`FIQLString`, `FIQLInt`, `FIQLFloat`, `FIQLTime`, `FIQLBool`, `FIQLEnum`, `FIQLUUID`):

       IsNil  func() P
       NotNil func() P

   First FIQL slot of this shape; existing slots all take ≥1 value argument. The shape is consistent across every type — no per-type variation.

7. **`apply` dispatch** routes `IsNull` → `f.IsNil()` (or `operator =is=null not allowed on this <kind> field` if nil) and `NotNull` → `f.NotNil()` (same error shape if nil). The dispatch lives in each per-type `apply` since the slot fields differ per struct, but the logic is identical and could be factored into a helper if it grows.

8. **Op-name registry entries** for `IsNull` and `NotNull` — adds two entries to `opByName`. Templates address them via `{{ if isOp "IsNull" $op }}` / `{{ if isOp "NotNull" $op }}`. `TestOpRegistryCovered` extends to include the new constants in its `wantOps` slice (the inverse drift check catches stale entries automatically).

9. **Round-trip example** in `examples/basic`: `bio` (optional string) gains `entdomain.IsNull, entdomain.NotNull` in its annotation. Generated `UserFIQLFields` registry includes `IsNil: user.BioIsNil, NotNil: user.BioNotNil` on the `FIQLString` entry for `bio`. New integration tests assert `bio=is=null` produces SQL containing `IS NULL` and `bio=is=notnull` produces `IS NOT NULL`.

10. **`make gen` zero-diff** for fields without `IsNull` / `NotNull` annotations. The only generated changes are on `bio` (the example field gaining the annotation) and the new bio entry in `UserFIQLFields`. Other examples and other fields produce identical output.

11. **README updates** — add `IsNull` (`=is=null`) and `NotNull` (`=is=notnull`) rows to the Operator Constants table; add a "Filtering by nullness" subsection showing the `Optional()` requirement and the schema annotation; add a Known Limitations bullet noting that non-optional fields cannot be annotated (codegen error) and that the strict `null`/`notnull` value vocabulary doesn't accept aliases.

## Out of Scope

- **SQL three-valued logic semantics.** This scope adds a way to express IS NULL / IS NOT NULL — it does not change the existing NEQ-on-NULL behavior (`name!=foo` still excludes NULL rows by SQL definition). Users who want "exclude foo OR include null" must compose: `name!=foo,name=is=null`. Documented in the README.

- **`empty` / `present` semantics for empty strings vs null.** `bio=is=empty` would mean `bio == "" OR bio IS NULL`, which conflates two distinct concepts. Out of scope; users compose `bio==,bio=is=null` if they want this.

- **Implicit `""` → `null` conversion in input.** `bio==` is currently rejected as "empty value for field 'bio'" (per existing parseComparison error) and stays that way. No new sentinel handling.

- **Null filtering on JSON / virtual / edge fields.** JSON and virtual fields aren't FIQL-addressable today; edge filtering (`owner=is=null`) is a separate cross-entity concern. Both inherit their existing exclusions.

- **Custom UUID GoType IS NULL.** A `field.UUID(..., uuid.UUID{}).GoType(CustomUUID{})` field is already excluded from the FIQL registry by ENTD-001's gate. The same gate suppresses `IsNull` / `NotNull` slots for it — no new logic needed.

- **The standalone `Is` operator constant.** The wire form is `=is=` but the runtime never sees it as a final op (parser normalizes to `IsNull` / `NotNull`). Exposing `Is` as a constant in the public API would invite annotations like `entdomain.FIQL(entdomain.Is)` which the generator would have to special-case. Cleaner: leave `Is` as parser-internal.

- **Multi-value `=is=in=(null,notnull)` or other compositions.** The grammar stays one-value-per-`=is=` term. AND/OR composition uses the existing `;` / `,` operators at the term level.

- **Renaming optional to nillable or vice versa.** Both `f.Optional` and `f.Nillable` flip the gate to "allowed" — they have semantically equivalent NULL behavior at the SQL level.

## Risks

- **Parser normalization step in `parseComparison` is novel.** Previous parser changes added new readers (`readListValue`); this one adds post-read normalization (op rewriting based on value). A maintainer reading the code could be confused by a `Is` op constant existing in the recognised-ops list but never appearing downstream. *Mitigation:* the normalization block is short (one switch statement) and gets a comment naming the contract: "wire `=is=` is normalized to internal `IsNull` / `NotNull` here; downstream apply paths never see `Is`." Add an internal test asserting no `IsNull` / `NotNull` reaches `apply` without going through the normalizer (i.e., test that `apply(Is, "null")` is unreachable from the parser).

- **Two FIQLOp constants mapping to the wire form `=is=`** breaks the assumption that one constant ↔ one wire op. The op-name registry handles it (two entries, both pointing at synthetic strings), and `TestOpRegistryCovered`'s inverse check still passes since both `IsNull` and `NotNull` are in `wantOps`. *Mitigation:* document the synthetic-string convention in a comment on the constants. If a future contributor needs to add another value-distinguished op, the pattern is now established.

- **Codegen-time error on non-optional field with `IsNull` annotation could surprise existing users.** Today nobody has the annotation, so the surprise risk is zero on landing. After landing, a schema author who annotates a required field will see the build fail. *Mitigation:* the error wording explicitly names the ent requirement (`requires Optional() or Nillable()`) and points to the field, so the fix is obvious. Document in the README's Known Limitations.

- **The new zero-arg slot shape (`func() P`) is the third FIQL slot pattern in the codebase** (after single-value `func(T) P` and variadic `func(...T) P`). Each additional shape grows the cognitive footprint of the per-type FIQL structs. *Mitigation:* the shape is genuinely needed (ent's predicates take no args); there's no abstraction that would reduce it. Document in the runtime type Godoc that the three shapes correspond to single-value ops, list ops, and nullness ops.

- **`apply` dispatch sprawl.** Each per-type `apply` already handles single-value + list-value paths. Adding null-check dispatch grows them further. The existing `applyListTyped` precedent suggests an `applyNullTyped` helper, but it would be a 6-line function called identically from every type. *Mitigation:* add the helper anyway to keep the per-type `apply` methods uniform — small DRY win; if the helper grows, it's already in place to absorb the growth.

- **Generator must distinguish "annotation present + field optional" from "annotation present + field non-optional"** at codegen time. The current generator structure (per-field annotation lookup → kind dispatch → template emission) needs a new gate at the annotation-lookup site. *Mitigation:* add the optionality check next to the existing UUID/GoType gate in the resolver layer; if it grows, factor into `kinds.go`. Keep `make gen` zero-diff for unaffected fields as the canary.

- **README's Operator Constants table is getting busy.** Eleven rows after this addition (EQ, NEQ, GT, LT, GTE, LTE, Contains, HasPrefix, In, NotIn, IsNull, NotNull = twelve). *Mitigation:* split the table into "comparison ops" / "set ops" / "nullness ops" subsections if the user finds it noisy on review; otherwise the single sortable table is fine.

- **Living list** — update during implementation as new failure modes surface.

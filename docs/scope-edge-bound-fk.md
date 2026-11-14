# Scope: Edge-bound foreign keys in the domain layer (treat the FK as a regular field)

| Field   | Value      |
|---------|------------|
| Status  | Accepted   |
| Created | 2026-11-14 |
| Author  | danhtran94 |

---

## Problem

When an ent schema declares a foreign key as a field and binds an edge to it (`edge.From("ring", Ring.Type).Ref("versions").Field("ring_id")`), entdomain loses the key. Per A1, a reporter reproduced this on v1.6.0 and v1.5.0 with `RingVersion.ring_id`.

Two code paths each assume the other one handles the key:

- **Field path skips it.** `buildDomainFileData` drops every field where `f.IsEdgeField()` is true (`generate.go:192`). `domain.RingVersion` has no `RingID`. The same skip appears in `template/domain.tmpl` (lines 41, 81, 142, 220, 303, 379, 439), `proto_generate.go:188`, `proto_mapper_generate.go:276`, `template.go:206`, `template.go:231`, and `template/fiql.tmpl:31`.
- **Edge path reads it conditionally and never writes it.** With `Edge(IDs())` the struct gains `RingID` (`generate.go:220`). `ToDomain` fills it only from `e.Edges.Ring` (`template/domain.tmpl:107-110`), which is `nil` without `WithRing()`. `ApplyDomain` writes edge IDs only for non-unique edges (`template/domain.tmpl:184`, `264`, `347`), so `Create().ApplyDomain(ctx, d).Save(ctx)` fails with `missing required field "RingVersion.ring_id"`.

Net effect: `ToDomain` returns `uuid.Nil` for a value ent already holds in `e.RingID`, and a service must either drop the edge (losing the database FK constraint) or bypass entdomain for that column.

A spike (patch kept outside the repo, discarded) shows that treating the field as a regular field fixes the defect with 12 edits across 7 files (A2). It also surfaced three behaviors this scope fixes in advance:

- **Name collision.** An edge with `IDs()` and a bound field both emit `RingID`. The struct field, the `RingVersionDomainFieldRingID` constant (`template/domain.tmpl:48`), the proto spec (`proto_generate.go:252`) and the mapper (`proto_mapper_generate.go:362`) must emit it once (A2).
- **Optional keys.** ent types an optional bound field as a value, `uuid.UUID`, and scans NULL as the zero value (A3). entdomain maps an `Optional` field to `*T` (`generate.go:278`, `generate.go:381`), so the domain field is `*uuid.UUID`. The mapping `&e.RingID` would turn a NULL row into a pointer to `uuid.Nil`, which fails the FK when written back.
- **No clear semantics.** No generated code calls `ClearX` for any optional field; `nil` means "leave unchanged" everywhere (A4). This scope does not add clearing for FKs only.

## Assumptions

- **A1 [VERIFIED]:** The defect reproduces on the current tree. Evidence: `/Users/danhtran94/Downloads/Telegram Desktop/entdomain-issue-edge-bound-fk.md` and a local run in this session: `ToDomain` returned `uuid.Nil` without `WithRing()`, and `Save` failed with `missing required field "RingVersion.ring_id"`.
- **A2 [VERIFIED]:** Treating the field as regular plus one dedupe condition is sufficient. Evidence: a throwaway prototype in `examples/spikefk` passed an sqlite e2e (read without `WithRing()`, create, mutable update, immutable update, optional NULL round trip, `Nest()` beside the FK, upsert on conflict, FIQL filter). `make gen` produced no change in `examples/basic`, `examples/custom`, `examples/sliced`, `examples/sliceddomain`, and `go test ./...` passed. The dedupe condition is `e.Unique && e.Field() != nil`.
- **A3 [VERIFIED]:** ent generates `RingID uuid.UUID` (not a pointer) on the entity for an `Optional()` edge-bound field, scanning NULL as zero, plus `SetRingID`, `SetNillableRingID`, and `ClearRingID`. Evidence: spike output `ent/ringnote.go` lines 21-22 and 78-86, `ent/ringnote_create.go:25`, `ent/ringnote_update.go:171-185`.
- **A4 [VERIFIED]:** entdomain never clears an optional field. Evidence: `grep -n Clear template/*.tmpl` matches only `$e.MutationClear` for non-unique `IDs()` edges; `ApplyDomain` on `Optional` non-nillable fields uses `ShouldApplyPtr(...) && d.X != nil` then `Set` (`template/domain.tmpl:159-162`, `239-241`).
- **A5 [VERIFIED]:** ent exposes the FK predicates FIQL needs. Evidence: spike `fiql.go` rendered `EQ: ringtag.RingIDEQ, NEQ: ringtag.RingIDNEQ`, and the filter `ring_id==<uuid>` returned one of two rows. FIQL stays opt-in per field, so it only activates when the author annotates the field.
- **A6 [HYPOTHESIS]:** No consumer relies on `XxxID` being `uuid.UUID` for an `Optional` bound field with `Edge(IDs())`. Verification deferred: no consumer schema is available. The current value is only populated under `WithRing()` and is never written, so reliance is unlikely.
- **A7 [VERIFIED]:** An enum-typed foreign key cannot exist. Evidence: a spike schema with `field.Enum("kind_id")` bound to an edge made `entc.Generate` fail with `mismatch field type between edge field "kind_id" and id of type "Kind" (string != int)` (ent `entc/gen/type.go:836`). The `IsEdgeField()` checks at `template/domain.tmpl:97` and `template.go:179` therefore stay as dead-code guards.
- **A8 [VERIFIED]:** Bound fields compose with vertical slices in both mapper modes. Evidence: a spike with `Ring` in slice `rings` and `RingVersion`/`RingNote` in slice `versions` (required immutable FK with `Edge(IDs())`, optional FK) generated, compiled against a real `.pb.go` from `buf generate`, and passed an sqlite round trip (`ToDomain`, `ApplyDomain`, proto `ToProto`/`FromProto`, NULL ↔ unset optional) under `WithProtoHelpersInDomain()` and under the default `pbmap` subpackage. The FK stays an ID-only reference across slices.
- **A9 [VERIFIED]:** Upsert on an optional bound field honors `nil`. Evidence: spike test with `OnConflictColumns("text").ApplyDomain`: `RingID == nil` left the stored FK unchanged, a set `RingID` replaced it.

## Success Criteria

1. **Required FK, struct.** In `examples/boundfk/internal/domain/ringversion_gen.go`, the literal `RingID uuid.UUID` appears exactly once. The entity also carries `Edge(IDs())` on the bound edge.
2. **Optional FK, struct.** `examples/boundfk/internal/domain/ringnote_gen.go` contains the literal `RingID *uuid.UUID`.
3. **Read without eager load.** In `examples/boundfk/ent/boundfk_e2e_test.go`, `TestToDomainReadsFKWithoutEagerLoad` passes. It queries without `WithRing()` and asserts `ToDomain().RingID` equals the ring's ID.
4. **Create writes the FK.** `TestCreateApplyDomainWritesFK` passes. `Create().ApplyDomain(ctx, d)` followed by `Save(ctx)` inserts with the FK set, with no manual `SetRingID`.
5. **Mutable FK updates, immutable FK does not.** `TestUpdateFKMutableAndImmutable` passes. `UpdateOne().ApplyDomain` changes `RingTag.ring_id` (mutable) and leaves `RingVersion.ring_id` (immutable) unchanged.
6. **Optional NULL maps to nil.** `TestOptionalFKNullRoundTrip` passes. A NULL row yields `ToDomain().RingID == nil`, and passing that domain value to `Create().ApplyDomain` then `Save` succeeds.
7. **`nil` does not clear.** In the same test file, `TestOptionalFKNilDoesNotClear` passes. `UpdateOne().ApplyDomain` with `RingID == nil` leaves the stored FK unchanged.
8. **No duplicate symbols.** `examples/boundfk/ent/domain.go` contains exactly one occurrence of `RingVersionDomainFieldRingID`, and `go build ./...` exits 0.
9. **`Nest()` beside the FK.** `TestNestAndFKCoexist` passes. With `WithRing()`, `ToDomain()` returns both `RingID` and `Ring`; without it, `RingID` is set and `Ring` is `nil`.
10. **Upsert applies the FK.** `TestUpsertApplyDomainWritesFK` passes. `OnConflictColumns("name").ApplyDomain` changes the stored `ring_id` on conflict.
11. **FIQL.** `examples/boundfk/ent/fiql.go` contains the literal `"ring_id": entdomain.FIQLUUID[predicate.RingTag]`, and `TestFIQLFilterByFK` passes with the expression `ring_id==<uuid>`.
12. **Proto.** `proto_boundfk_test.go` loads the `examples/boundfk/ent/schema` graph and asserts: each generated message contains exactly one `ring_id` field, the `RingNote` message declares it `optional string ring_id`, and each generated mapper contains exactly one `RingId:` assignment per direction.
13. **Helper.** `runtime_test.go` contains `TestPtrNonZero`: `PtrNonZero(uuid.Nil)` returns `nil`, `PtrNonZero(0)` returns `nil`, and `PtrNonZero(7)` returns a pointer to `7`.
14. **No regression.** After `make gen`, `git diff --exit-code HEAD -- examples/basic examples/custom examples/sliced examples/sliceddomain` exits 0.
15. **Gate stays green.** `make build`, `make test`, and `make gen` followed by `git diff --exit-code HEAD` all exit 0, with `examples/boundfk` regenerated by a new `gen-boundfk` target.

## Out of Scope

- **Clearing an optional FK through `ApplyDomain` (`nil` clears).**
  - Reason: no optional field has clear semantics today (A4); a FK-only rule creates a second convention.
  - A repo-wide "clear" design (e.g. an `ApplyOption`) is a separate scope.
- **Changing the domain type of an optional FK to a value type.**
  - Reason: entdomain maps `Optional` to `*T`; deviating for FKs breaks that convention.
- **FKs on edges without a bound field (`e.Field() == nil`).**
  - Reason: no entity field exists to read or write; `IDs()` behavior for these edges is unchanged.
- **Writing a unique `IDs()` edge through `ApplyDomain` when no bound field exists.**
  - Reason: a different defect (unique edges are read-only, `template/domain.tmpl:184`); not required by the reporter.
- **Compiled proto in `examples/boundfk`.**
  - Reason: `.pb.go` needs a manual `buf generate` (same limit as `examples/basic`); proto output is covered by `proto_boundfk_test.go` text assertions instead.
- **A checked-in example combining bound FKs with slices.**
  - Reason: spike verified both mapper modes (A8) and the change touches no slice-specific code; a new example adds a manual `buf generate` for no new generator path.
- **Composite or multi-column foreign keys.**
  - Reason: ent edge fields bind one column.

## Risks

- **Type change for one consumer shape (A6).** A schema with an `Optional` bound field and `Edge(IDs())` on the same edge sees `XxxID` change from `uuid.UUID` to `*uuid.UUID`.
  - Mitigation: README gets a note naming the shape and the one-line fix; the release note states it explicitly. Required FKs and entities without bound fields keep their types (Success Criterion 14).
- **Dedupe condition drifts across five sites.** The struct, the domain-field constant, the `ToDomain` edge read, the proto spec and the mapper each need `e.Unique && e.Field() != nil`.
  - Mitigation: one Go helper in `generate.go`, exposed to templates through the `funcMap` in `template.go`; Success Criteria 1, 8 and 12 each fail when any site is missed.
- **Blanket skip removal changes unrelated entities.** Removing `IsEdgeField()` skips affects every entity.
  - Mitigation: Success Criterion 14 diffs all four existing examples after `make gen`.
- **FIQL surface grows.** Bound fields become filterable once annotated.
  - Mitigation: FIQL remains opt-in per field annotation (A5); no field gains FIQL without an explicit `entdomain.FIQL(...)`.
- **A zero key reads as NULL.** `PtrNonZero` treats `uuid.Nil` and `0` as NULL, so an `Optional()` bound key whose target legitimately has the zero ID reads back as `nil`, and a create from that domain value stores NULL (raised in PR review).
  - Mitigation: `PtrNonZero` applies only to `Optional`, non-`Nillable` edge-bound fields. A schema whose zero key is valid declares `.Optional().Nillable()`, which makes the entity field `*T` and bypasses the helper (verified on a spike). The README "Foreign key fields" section and the helper's doc comment state the rule.

## History

| Ticket    | Title                                                                         | Created    | Status   |
|-----------|-------------------------------------------------------------------------------|------------|----------|
| (initial) | Edge-bound foreign keys in the domain layer (treat the FK as a regular field) | 2026-11-14 | Accepted |

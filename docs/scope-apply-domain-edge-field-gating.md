# Scope: ApplyDomain honors OnlyFields / OmitFields / OmitZeroVal for IDs edges

| Field   | Value      |
|---------|------------|
| Status  | Accepted   |
| Created | 2026-11-16 |
| Author  | danhtran94 |

---

## Problem

Generated `ApplyDomain` writes every non-unique `IDs()` edge without asking `ApplyConfig` whether the edge should be applied. Scalar fields go through `cfg.ShouldApply` / `cfg.ShouldApplyPtr`. Edges do not:

- **UpdateOne** (`template/domain.tmpl:258-263`) and **Update** (`template/domain.tmpl:339-344`) emit `if cfg.IsAppendEdge("<edge>_ids") { Add } else { Clear().Add }` with no `ShouldApply` gate. In `examples/basic/ent/domain.go` this renders as `u = u.ClearPosts().AddPostIDs(d.PostIDs...)`.
- **Create** (`template/domain.tmpl:180-184`) emits `if len(d.XIDs) > 0 { Add }`, also without a gate, so `OmitFields("post_ids")` does not stop the edge from being attached.

`ToDomain()` sets `PostIDs` only from `e.Edges.Posts`, so a domain value read without `WithPosts()` carries `PostIDs == nil`. Combined with the ungated clear, a read-modify-write restricted by `OnlyFields` deletes every relation (A1):

    Current (broken):
      GetX(id).ToDomain()  ──► d.PostIDs == nil (edge not loaded)
      UpdateOneID(id).ApplyDomain(d, OnlyFields("name"))
                           ──► SetName ✓   ClearPosts().AddPostIDs() ✗  → all posts unlinked

The two usage examples in ADR-001 (`UserRepo.Update` with `OnlyFields(name, status)`, lines 539-550; `DeactivateAll` with `OnlyFields(status)`, lines 564-573) and the same examples in README "Repository Adapter Example" trigger it. `DeactivateAll` unlinks the posts and tags of every matched row in one statement (A1).

Replace-by-default itself is a recorded decision (ADR-001:234, "desired final state semantics") and is not the defect. The defect is that the field-selection options, which the generator already exposes edge keys for (`UserDomainFieldPostIDs = "post_ids"`), have no effect on edges.

## Assumptions

- **A1 [VERIFIED]:** The defect reproduces on HEAD (`ff4aab2`). Evidence: a throwaway test against the committed `examples/basic` generated code (copy under `/tmp/entd-repro`, not in the repo): `UpdateOneID(...).ApplyDomain(d, OnlyFields(name))` after `GetX(...).ToDomain()` took posts from 2 to 0; `Update().ApplyDomain(&User{Status: inactive}, OnlyFields(status)).Where(status==active)` took two users from 2 posts each to 0; the same update after `.WithPosts()` kept 2.
- **A2 [VERIFIED]:** `ApplyConfig.ShouldApply` already gives the needed semantics for a slice value. Evidence: a run in this session against `runtime.go`: `OnlyFields("name")` → `ShouldApply("post_ids", []int{1}) == false`; `OmitZeroVal()` → `ShouldApply("post_ids", []int(nil)) == false` and `ShouldApply("post_ids", []int{}) == true` (`reflect.Value.IsZero` is true only for a nil slice). No runtime change is needed.
- **A3 [VERIFIED]:** The option key for a non-unique edge equals the generated constant value `{{ singular $e.Name }}_ids` (`template/domain.tmpl:50`), the same key `IsAppendEdge` already uses (`template/domain.tmpl:259`, `340`).
- **A4 [VERIFIED]:** Upsert paths never write edges (`template/domain.tmpl:360-475` contain no `Mutation*` call), and `CreateBulkDomain` / `UpdateBulkDomain` delegate to `Create().ApplyDomain` / `UpdateOneID().ApplyDomain` (`template/domain.tmpl:498`, `573`). Gating the three single-builder sites covers every write path.
- **A5 [HYPOTHESIS]:** No consumer relies on `OnlyFields(<scalars>)` also replacing edges. Verification deferred: no consumer code is available. The only documented combination is README "Apply Options → Example" (`OnlyFields(Name, Status), AppendEdge(PostIDs)`), handled by Decision D1.

## Decisions

- **D1: `AppendEdge(key)` applies the edge even when `key` is outside `OnlyFields`.** The README example `OnlyFields(Name, Status), AppendEdge(PostIDs)` is documented as "update only name and status, append new posts"; keeping it working means an explicit `AppendEdge` counts as opting the edge in. Append is additive (`AddXIDs`), never destructive, so bypassing the gate cannot delete data. Generated condition: `if cfg.IsAppendEdge(k) { Add } else if cfg.ShouldApply(k, d.XIDs) { Clear().Add }`.
- **D2: the gate uses `ShouldApply`, not a new edge-specific method.** It reuses the allowlist, denylist and `OmitZeroVal` checks already tested in `runtime_test.go`. Side effect, accepted: under `OmitZeroVal()` a nil `XIDs` leaves the edge untouched while `[]T{}` still clears (A2), which gives callers an opt-in "nil means untouched" today.

## Success Criteria

1. In `examples/basic/ent/apply_edges_e2e_test.go`, `TestUpdateOneOnlyFieldsKeepsEdges` passes: after `GetX(id).ToDomain()` without `WithPosts()`, `UpdateOneID(id).ApplyDomain(d, OnlyFields(UserDomainFieldName))` leaves `QueryPosts(u).CountX` at 2 and `Name` updated.
2. `TestUpdateWhereOnlyFieldsKeepsEdges` passes: `Update().ApplyDomain(&domain.User{Status: inactive}, OnlyFields(UserDomainFieldStatus)).Where(status==active)` leaves posts and tags of both seeded users unchanged.
3. `TestUpdateOneOmitFieldsKeepsEdges` passes: `OmitFields(UserDomainFieldPostIDs)` with `d.PostIDs == nil` leaves 2 posts.
4. `TestUpdateOneOnlyFieldsIncludingEdgeReplaces` passes: `OnlyFields(UserDomainFieldName, UserDomainFieldPostIDs)` with `d.PostIDs = []int{p1}` leaves exactly post `p1` linked.
5. `TestUpdateOneDefaultStillReplaces` passes: `ApplyDomain(d)` with no options and `d.PostIDs == nil` leaves 0 posts (replace-by-default is unchanged by this scope).
6. `TestUpdateOneAppendEdgeOutsideOnlyFields` passes: `OnlyFields(UserDomainFieldName), AppendEdge(UserDomainFieldPostIDs)` with `d.PostIDs = []int{p3}` leaves 3 posts (D1).
7. `TestUpdateOneOmitZeroValNilKeepsEmptyClears` passes: under `OmitZeroVal()`, `d.PostIDs == nil` leaves 2 posts and `d.PostIDs = []int{}` leaves 0 (D2).
8. `TestCreateOmitFieldsSkipsEdge` passes: `Create().ApplyDomain(d, OmitFields(UserDomainFieldPostIDs))` with `d.PostIDs = []int{p1}` creates a user with 0 posts.
9. After `make gen`, `git diff --stat HEAD -- examples` lists only files named `domain.go` under `examples/*/ent/` (or `examples/custom/repo/ent/`).
10. `make build`, `make test`, and `make gen` followed by `git add -N . && git diff --exit-code HEAD` all exit 0 after the commits.

## Out of Scope

- **Changing replace-by-default, or making a nil slice mean "untouched" without `OmitZeroVal`.**
  - Reason: behavior change for every caller that passes no options; needs its own decision. Tracked as Unresolved Question U1.
- **Writing unique `IDs()` edges through `ApplyDomain`.**
  - Reason: unique edges are read-only today (`template/domain.tmpl:180`, only `not $e.Unique` is written); separate defect, already listed out of scope in `docs/scope-edge-bound-fk.md`.
- **Typed field keys (`UserDomainField` as a defined type instead of an alias).**
  - Reason: API change to `OnlyFields` / `OmitFields` signatures; unrelated to the data-loss path.
- **Upsert edge writes.**
  - Reason: ent upsert builders have no edge mutators (A4).
- **Other findings from the 2026-11-16 analysis (NULL→zero on Optional fields, Create overriding defaults, FIQL, proto).**
  - Reason: independent defects; each gets its own scope.

## Unresolved Questions

- **U1: Should a nil `XIDs` mean "do not touch the edge" by default, and `[]T{}` mean "clear"?** This removes the read-modify-write trap for callers who pass no options (Success Criterion 5 pins today's behavior). It is a breaking change for callers who rely on nil clearing. Decide before a follow-up job; not blocking this one.

## Risks

- **A caller relied on `OnlyFields(<scalars>)` also replacing edges (A5).**
  - Mitigation: the release note states that `OnlyFields` / `OmitFields` / `OmitZeroVal` now apply to `<edge>_ids` keys and names the one-line fix (`OnlyFields(..., UserDomainFieldPostIDs)`). README "Apply Options" documents it.
- **`OmitZeroVal()` callers now keep edges when `XIDs` is nil (D2).**
  - Mitigation: this matches the documented meaning of `OmitZeroVal` ("skip fields with zero values"); Success Criterion 7 pins it and the release note names it.
- **One of the three template sites is missed.**
  - Mitigation: Success Criteria 1-4 and 6-7 cover UpdateOne, 2 covers Update, 8 covers Create; each fails if its site is not gated.

## History

| Ticket    | Title                                                                  | Created    | Status   |
|-----------|------------------------------------------------------------------------|------------|----------|
| (initial) | ApplyDomain honors OnlyFields / OmitFields / OmitZeroVal for IDs edges | 2026-11-16 | Accepted |

# ENTD-009-EDGE: ApplyDomain honors OnlyFields / OmitFields / OmitZeroVal for IDs edges

| Field      | Value                                          |
|------------|------------------------------------------------|
| Status     | Done                                           |
| Created    | 2026-11-16                                     |
| Assignee   | danhtran94                                     |
| Source     | docs/scope-apply-domain-edge-field-gating.md   |
| Blocked by | —                                              |

<!-- approved -->
<!-- approval: user instruction "approved" (2026-11-16) -->

## Goal

Stop `ApplyDomain` from unlinking relations the caller excluded. After the job, the generated edge blocks for non-unique `IDs()` edges in `Create`, `UpdateOne` and `Update` (`template/domain.tmpl:180-184`, `258-263`, `339-344`) check `cfg.ShouldApply("<edge>_ids", d.XIDs)` before writing, and `AppendEdge` keeps working outside `OnlyFields` (scope D1). The job ships when the 10 Success Criteria in the source scope pass, verified by a new `examples/basic/ent/apply_edges_e2e_test.go` and by `make build`, `make test`, `make gen` + `git diff --exit-code HEAD` all exiting 0.

## Problem

    Current (broken):
      GetX(id).ToDomain()                       → d.PostIDs == nil (not eager-loaded)
      UpdateOneID(id).ApplyDomain(d, OnlyFields("name"))
        SetName                                 ✓ gated by ShouldApply
        ClearPosts().AddPostIDs()               ✗ ungated → every post unlinked

Reproduced on HEAD `ff4aab2` (scope A1): 2 posts → 0 for UpdateOne; 2+2 → 0+0 for the README `DeactivateAll` pattern.

## Solution: gate edges with the existing ShouldApply

    After fix (UpdateOne / Update):
      if cfg.IsAppendEdge(k)            { AddXIDs(d.XIDs...) }              ← explicit opt-in, additive (D1)
      else if cfg.ShouldApply(k, d.XIDs) { ClearX().AddXIDs(d.XIDs...) }    ← honors OnlyFields / OmitFields / OmitZeroVal (D2)

    After fix (Create):
      if len(d.XIDs) > 0 && cfg.ShouldApply(k, d.XIDs) { AddXIDs(d.XIDs...) }

    k = "{{ singular $e.Name }}_ids"   (same key as the generated UserDomainFieldPostIDs constant and IsAppendEdge)

No runtime change: `ShouldApply` already returns false for a key outside `OnlyFields`, a key in `OmitFields`, and a nil slice under `OmitZeroVal` (scope A2).

### Why not alternatives

| Approach | Verdict |
|---|---|
| **Gate with `ShouldApply`; `AppendEdge` bypasses the gate** | Chosen. Template-only, three sites, reuses tested runtime logic; keeps the README `OnlyFields + AppendEdge` example working. |
| Gate with `ShouldApply` for both branches (AppendEdge also gated) | Rejected: breaks README "Apply Options → Example" (`OnlyFields(Name, Status), AppendEdge(PostIDs)` would stop appending). |
| New `ApplyConfig.ShouldApplyEdge` method | Rejected: second code path with the same allowlist/denylist semantics; no behavior it would add. |
| nil slice means "untouched" by default | Deferred: breaking for callers that pass no options; scope U1. |

## Workstreams

### 1. Template

Gates the three edge-write sites. One atomic edit: the three blocks share the key expression and must stay consistent.

| # | Task | File | Status |
|---|------|------|--------|
| 1.1 | Create (`:180-184`): wrap `{{ $e.MutationAdd }}` in `len(...) > 0 && cfg.ShouldApply("{{ singular $e.Name }}_ids", d.XIDs)` | `template/domain.tmpl` | [x] |
| 1.2 | UpdateOne (`:258-263`): change `} else {` to `} else if cfg.ShouldApply("{{ singular $e.Name }}_ids", d.XIDs) {` | same | [x] |
| 1.3 | Update (`:339-344`): same change as 1.2 | same | [x] |
| 1.4 | Run `make gen`; confirm `git diff --stat HEAD -- examples` lists only `examples/*/ent/domain.go` and `examples/custom/repo/ent/domain.go` (SC 9) | `examples/basic/ent/domain.go` | [x] |

**Key details:** the key literal must stay `{{ singular $e.Name }}_ids` to match the constant at `:50` and `IsAppendEdge` at `:259` / `:340`. Editing the template triggers `harness hook-postbuild` → `make gen`; inspect the diff before committing.

### 2. Tests

One e2e test per Success Criterion, against the regenerated `examples/basic`. Shape copied from `examples/basic/ent/transformer_test.go` (`enttest.Open` on sqlite, `sampleUser()`).

| # | Task | File | Status |
|---|------|------|--------|
| 2.1 | Seed helper: one user + 2 posts (+ 1 tag) via ent builders; reset `ent.UserTransformer = nil` | `examples/basic/ent/apply_edges_e2e_test.go` | [x] |
| 2.2 | `TestUpdateOneOnlyFieldsKeepsEdges` (SC 1) | same | [x] |
| 2.3 | `TestUpdateWhereOnlyFieldsKeepsEdges` — two users, posts and tags checked (SC 2) | same | [x] |
| 2.4 | `TestUpdateOneOmitFieldsKeepsEdges` (SC 3) | same | [x] |
| 2.5 | `TestUpdateOneOnlyFieldsIncludingEdgeReplaces` (SC 4) | same | [x] |
| 2.6 | `TestUpdateOneDefaultStillReplaces` — pins replace-by-default (SC 5) | same | [x] |
| 2.7 | `TestUpdateOneAppendEdgeOutsideOnlyFields` (SC 6, D1) | same | [x] |
| 2.8 | `TestUpdateOneOmitZeroValNilKeepsEmptyClears` (SC 7, D2) | same | [x] |
| 2.9 | `TestCreateOmitFieldsSkipsEdge` (SC 8) | same | [x] |
| 2.10 | Write 2.2 and 2.3 first and confirm they FAIL on the unpatched template before WS1 lands | same | [x] |

### 3. Documentation

| # | Task | File | Status |
|---|------|------|--------|
| 3.1 | "Apply Options": state that `OnlyFields` / `OmitFields` / `OmitZeroVal` apply to edge keys (`UserDomainFieldPostIDs`), that `AppendEdge` applies even outside `OnlyFields`, and that `OmitZeroVal` keeps an edge when `XIDs` is nil but clears on `[]T{}` | `README.md` | [x] |
| 3.2 | "Edges" table: `ApplyDomain` column notes the `ShouldApply` gate and the eager-load requirement for replace-by-default | same | [x] |
| 3.3 | Add a one-line amendment under sections 3 and 4 pointing to ENTD-009 (the code sketches there show the ungated block) | `docs/ADR-001-entdomain-extension.md` | [x] |
| 3.4 | Release-note draft text (behavior change + one-line migration `OnlyFields(..., UserDomainFieldPostIDs)`) recorded in this job's Notes for the GitHub release | `jobs/ENTD-009-EDGE-apply-domain-edge-field-gating.md` | [x] |

### 4. Follow-up (scope U1)

| # | Task | File | Status |
|---|------|------|--------|
| 4.1 | nil `XIDs` means "untouched" by default, `[]T{}` clears | `template/domain.tmpl` | deferred — breaking change awaiting user decision (scope U1) |

### Verification gate

| # | Task | Status |
|---|------|--------|
| V.1 | `make build` exits 0 | [x] |
| V.2 | `make test` exits 0 | [x] |
| V.3 | `make gen` then `git add -N . && git diff --exit-code HEAD` exits 0 after the commits (SC 10) | [x] |
| V.4 | `harness validate-docs` exits 0; `harness validate-pr-checklist` exits 0 after the commits | [x] |

## Design Decisions

### AppendEdge bypasses the gate (scope D1)
`AppendEdge(k)` is an explicit per-edge request, and append only adds links. Letting it bypass `OnlyFields` keeps the documented README example working and cannot delete data. Contradictory input (`OmitFields(k), AppendEdge(k)`) appends; the explicit edge-level option wins.

### Reuse ShouldApply (scope D2)
The edge key already lives in the same `DomainField` namespace as scalar keys. Reusing `ShouldApply` means one rule for every field and no runtime change; the `OmitZeroVal` nil/empty distinction comes for free.

## What Stays Unchanged

- `runtime.go` and `runtime_test.go`: `ShouldApply` semantics are reused as is (scope A2).
- Replace-by-default when no option excludes the edge (SC 5; scope U1 deferred).
- Upsert `ApplyDomain` (no edge writes, scope A4); `CreateBulkDomain` / `UpdateBulkDomain` (delegate to the gated methods).
- Unique `IDs()` edges (read-only), `Nest()` edges (never written), edge-bound FK fields (regular fields since ENTD-008).
- `generate.go`, `template.go`, proto and FIQL generators and templates.
- Domain struct output under `examples/*/domain/` and `examples/*/internal/domain/` (SC 9).

## Implementation Order

    1. WS2 tasks 2.1-2.3, 2.10  ← failing tests first, proving the defect on the unpatched template
    2. WS1 Template             ← makes 2.2/2.3 pass; regenerates examples
    3. WS2 remaining tests      ← pin D1, D2, Create gating and unchanged default
    4. WS3 Docs                 ← documents verified behavior
    5. Verification gate        ← V.3/V.4 need the committed tree

## Notes

- `harness hook-postbuild` runs `make gen` after template edits; CI fails on generated drift.
- Local `golangci-lint` is v1 while `.golangci.yml` is v2; run lint as `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.11.4 run ./...` (ENTD-008 Discoveries).
- Throwaway repro used for scope A1: `/tmp/entd-repro/examples/basic/ent/zz_repro_test.go` (outside the repo; delete after WS2).
- Follow-up (not ENTD-009): `OnlyFields()` with no keys sets no allowlist (`runtime.go`, `len(c.onlyFields) > 0`), so a caller that builds an empty field mask applies every field and replaces unloaded edges. Making it fail closed changes scalar behavior too; needs its own scope. Raised in PR #69 review.
- The next release currently depends on ent master `v0.14.7-0.20261014065818-8037bc89bcab` and Go 1.26.0; revert to a released ent before tagging (separate job, not ENTD-009).

### Release note draft (task 3.4)

> **Fix: `ApplyDomain` no longer unlinks relations excluded by `OnlyFields` / `OmitFields` (ENTD-009).**
> Previously, `UpdateOne`/`Update` `ApplyDomain` replaced every non-unique `IDs()` edge regardless of apply options, so `OnlyFields(UserDomainFieldName)` on a value read without `WithPosts()` cleared all posts, and `Update().ApplyDomain(..., OnlyFields(status)).Where(...)` cleared them for every matched row. Edge keys (`post_ids`) now follow `OnlyFields`, `OmitFields` and `OmitZeroVal` on create and update.
>
> **Behavior changes:**
> - If you relied on `OnlyFields(<scalars>)` also replacing an edge, add the edge key: `OnlyFields(..., ent.UserDomainFieldPostIDs)`.
> - Under `OmitZeroVal()`, a `nil` `PostIDs` now leaves the edge unchanged; pass `[]int{}` to clear it.
> - `AppendEdge(key)` still appends when `key` is not in `OnlyFields`.
> - With no options, replace-by-default is unchanged: load the edge (`WithPosts()`) before writing a value back.

## Discoveries & Decisions During Implementation

### [Implementer] Tests written all at once, run before the fix
The plan ordered 2.2/2.3 before WS1 and the other tests after. All eight were written first and run against the unpatched template instead, which gives a stronger before/after record: five failed exactly as the scope predicts (2.2, 2.3, 2.4, 2.8 posts 2→0 or tags 1→0; 2.9 attached 1 post despite `OmitFields`), and the three that pin unchanged behavior (2.5, 2.6, 2.7) passed. After WS1 all eight pass.

### [Implementer] Only three examples change; `custom` and `boundfk` have no non-unique `IDs()` edge
`make gen` changed `examples/basic/ent/domain.go`, `examples/sliced/ent/domain.go` and `examples/sliceddomain/ent/domain.go` (42+/18−). `examples/custom` and `examples/boundfk` regenerate byte-identical because none of their edges is both non-unique and `IDs()`-annotated. A second `make gen` produced the same diff (idempotent).

### [Implementer] README "Apply Options → Example" used the pre-ADR-005 signature
The example called `ApplyDomain(d, ...)` without `ctx` and chained `.Save(ctx)` on a two-value return, which no longer compiles. Since task 3.1 rewrote that section, the example now passes `ctx` and handles the error. Other README snippets were not audited here.

### [Implementer] Shell gotcha: a `grep -c` guard short-circuited `make gen`
A `grep -c '…{{ singular $e.Name }}…' && make gen` sanity check returned 0 matches (exit 1) on the correctly edited template, so `make gen` never ran and the examples looked unchanged. Running `make gen` directly regenerated them. Run generation unconditionally and verify with `git diff` instead.

### [Implementer] GitButler placed the new branch on a stale base
`but commit -b entd-009-edge-field-gating` created the branch on `d0799c4` (PR #64), the workspace's last-pulled target, while `origin/main` was three merges ahead (#65 ENTD-008, #67, #68). The workspace tree still showed those changes because their branches were applied, so nothing looked wrong locally. `git merge-base` exposed it; `but pull --check` reported a clean rebase and `but pull` moved the branch onto `d065853`. V.3 was then verified on a `git archive` export of the branch alone (build, tests, `make gen` with no diff), not on the workspace, which also carries `chore-remove-changelog`.

### [Reviewer] README overstated what the options protect (PR #69, CodeRabbit)
Two README rules held only for replace. `OnlyFields()` with no keys sets no allowlist (`ShouldApply` checks it only when `len(c.onlyFields) > 0`), so "OnlyFields without the edge key leaves the edge unchanged" was false for an empty call. `AppendEdge` runs before the `ShouldApply` gate, so it also overrides `OmitFields`, and an empty `[]T{}` under `AppendEdge` appends nothing instead of clearing. Both behaviors are intended (Design Decision "AppendEdge bypasses the gate"); the README now scopes the unchanged/clear rules to replace and states the `AppendEdge` and empty-`OnlyFields()` cases. No runtime change; the empty-`OnlyFields()` fail-open is recorded as a follow-up in Notes.

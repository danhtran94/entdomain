# ENTD-008-FK: Edge-bound foreign keys in the domain layer (treat the FK as a regular field)

| Field      | Value                      |
|------------|----------------------------|
| Status     | In Progress                |
| Created    | 2026-11-14                 |
| Assignee   | danhtran94                 |
| Source     | docs/scope-edge-bound-fk.md |
| Blocked by | —                          |

<!-- approved -->
<!-- approval: user instruction "approved" (2026-11-14) -->

## Goal

Make an ent field bound to an edge (`edge.From(...).Field("ring_id")`) a first-class domain field. After the job, `domain.RingVersion` carries `RingID uuid.UUID` exactly once, `ToDomain()` fills it from `e.RingID` without `WithRing()`, and `Create().ApplyDomain(ctx, d).Save(ctx)` inserts with the FK set. An `Optional()` bound field becomes `RingID *uuid.UUID`; a NULL row maps to `nil` and `nil` leaves the stored FK unchanged. An edge that carries both a bound field and `Edge(IDs())` emits one field, not two. Proto messages, proto mappers, and opt-in FIQL treat the field like any other.

The job ships when the 15 Success Criteria in the source scope note pass, verified by a new `examples/boundfk` example with sqlite e2e tests, and by `make build`, `make test`, and `make gen` followed by `git diff --exit-code HEAD`, all exiting 0. The four existing examples regenerate with zero diff.

## Problem

Two code paths each assume the other handles the FK (source scope note, Problem; reproduced per A1):

    Current (broken):
      field ring_id ──► IsEdgeField() ──► skipped in struct, ToDomain, Create/Update/Upsert, bulk, proto, mapper, FIQL
      edge  ring    ──► Edge(IDs()) ──► struct gets RingID
                                      ──► ToDomain reads e.Edges.Ring only   (nil without WithRing())
                                      ──► ApplyDomain never writes unique IDs edges
      Result: ToDomain → RingID == uuid.Nil; Save → missing required field "RingVersion.ring_id" ✗

## Solution: one field, one dedupe rule

Stop skipping edge-bound fields and let them flow through the existing regular-field code. The only new logic is the dedupe rule when an `IDs()` edge also has a bound field, and a NULL-to-`nil` read for optional keys.

    After:
      field ring_id ──► regular field ──► struct, ToDomain (e.RingID), Create/Update/Upsert (SetRingID), proto, FIQL ✓
      edge  ring    ──► IDs() suppressed when e.Unique && e.Field() != nil   (one RingID, from the field)
      Optional field ──► *T in domain; ToDomain uses PtrNonZero; nil in ApplyDomain = unchanged

### Components

**`edgeIDsDeduped(e *gen.Edge) bool`** (new, `generate.go`) — returns `e.Unique && e.Field() != nil`.
- Single owner of the dedupe rule; exposed to templates through the `funcMap` in `template.go`.
- Called from the struct builder, the domain-field constants, the `ToDomain` edge read, the proto spec, and the proto mapper.

**`PtrNonZero[T comparable](v T) *T`** (new, `runtime.go`) — returns `nil` for the zero value, else `&v`.
- Used only for `Optional()`, non-`Nillable` edge-bound fields in `ToDomain`, so a NULL row never becomes a pointer to `uuid.Nil`.

### Why not alternatives

| Approach | Verdict |
|---|---|
| **Treat bound field as regular + dedupe helper** | Chosen. Reuses write paths that already honor `Immutable`, `Optional`, bulk and upsert; spike passed every case. |
| Annotation-only: read the field for `IDs()` edges, write via the setter | Rejected: users must annotate every bound edge, and optional keys need a second nil/clear implementation beside the regular one. |
| Pointer domain type with clear-on-nil for FKs only | Rejected: no optional field clears today (scope A4); a FK-only rule makes a second convention. |
| Keep skipping and document "drop the edge" | Rejected: loses the database FK constraint, the reporter's actual problem. |

## Workstreams

### 1. Generator core

Adds the dedupe rule and the runtime helper, and stops the struct builder from skipping bound fields. Everything else calls the helper, so it lands first.

| # | Task | File | Status |
|---|------|------|--------|
| 1.1 | Add `PtrNonZero[T comparable](v T) *T` with a doc comment stating it applies only to `Optional()`, non-`Nillable` edge-bound fields | `runtime.go` | [x] |
| 1.2 | Add `TestPtrNonZero`: `uuid.Nil` → `nil`, `0` → `nil`, `7` → pointer to `7` (SC 13) | `runtime_test.go` | [x] |
| 1.3 | Add `edgeIDsDeduped(e *gen.Edge) bool` | `generate.go` | [x] |
| 1.4 | Register `edgeIDsDeduped` in the template `funcMap` | `template.go` | [x] |
| 1.5 | `buildDomainFileData`: delete the `IsEdgeField()` skip (`generate.go:192`); guard the `HasIDs()` branch (`generate.go:220`) with `!edgeIDsDeduped(e)` | `generate.go` | [x] |
| 1.6 | Unit test `edgeIDsDeduped` with `gen.Edge` literals: unique unbound → false; non-unique → false (the positive case needs a real FK and moves to 6.5) | `template_internal_test.go` | [x] |

**Key details:** `gen.Edge.Field()` returns `*gen.Field`; only the edge that declares `.Field(...)` returns non-nil, so the `To` side of an O2M edge is never deduped. Tasks 1.3-1.5 are one atomic edit: the helper must exist in the same change that calls it.

### 2. Ent mapper template

Removes the skips in the template and applies the dedupe rule. Depends on workstream 1 for `edgeIDsDeduped`.

| # | Task | File | Status |
|---|------|------|--------|
| 2.1 | Replace `{{- if not $f.IsEdgeField }}` with the unconditional body at lines 41 (domain-field constants), 81 (`ToDomain`), 142 (create), 220 (update), 303 (upsert), 379 (create-bulk), 439 (update-bulk) | `template/domain.tmpl` | [x] |
| 2.2 | Constants block (line 48): emit `{{ $n.Name }}DomainField{{ pascal $e.Name }}ID` only when the edge is not deduped | `template/domain.tmpl` | [x] |
| 2.3 | `ToDomain` unique-edge read (line 109): skip `d.XID = _e.ID` when the edge is deduped | `template/domain.tmpl` | [x] |
| 2.4 | `ToDomain` field init (line 87): for `Optional`, non-`Nillable`, `IsEdgeField` use `entdomain.PtrNonZero(e.X)` instead of `&e.X` | `template/domain.tmpl` | [x] |

**Key details:** line 97 (`and (not $f.IsEdgeField) (isNillable $f) (isEnum $f)`) stays; an enum-typed FK cannot exist because ent rejects it (scope A7). `make gen` after each edit must leave `examples/basic`, `examples/custom`, `examples/sliced`, `examples/sliceddomain` unchanged. Tasks 2.1-2.4 land as one change because the struct (workstream 1) gains `RingID` and the template must set it.

### 3. Proto messages and mappers

Removes the skips and applies the dedupe rule in both proto generators. Independent of workstream 2; depends on workstream 1.

| # | Task | File | Status |
|---|------|------|--------|
| 3.1 | Delete the `IsEdgeField()` skip (`proto_generate.go:188`); guard the `HasIDs()` spec (`proto_generate.go:252`) with `!edgeIDsDeduped(e)` | `proto_generate.go` | [x] |
| 3.2 | Delete the `IsEdgeField()` skip (`proto_mapper_generate.go:276`); guard the `HasIDs()` mapper entry (`proto_mapper_generate.go:362`) with `!edgeIDsDeduped(e)` | `proto_mapper_generate.go` | [x] |

**Key details:** `edgeIDsDeduped` is a Go function here, not a template func, so call it directly. Proto field numbers: `proto_lock.go` assigns numbers by field name, so the new `ring_id` field gets a fresh number and existing messages are unaffected.

### 4. FIQL

Lets an annotated bound field participate in FIQL. FIQL stays opt-in per field annotation, so no existing entity changes.

| # | Task | File | Status |
|---|------|------|--------|
| 4.1 | Delete the `IsEdgeField()` skip in `hasFIQLFieldsFn` (`template.go:206`) | `template.go` | [x] |
| 4.2 | Delete the `IsEdgeField()` skip in `hasFIQLNodesFn` (`template.go:231`) | `template.go` | [x] |
| 4.3 | Replace `{{- if not $f.IsEdgeField }}` (line 31) with the unconditional body | `template/fiql.tmpl` | [x] |

**Key details:** `template.go:179` (`hasEnumFieldsFn`) keeps its `IsEdgeField()` check; enum FKs cannot exist (scope A7).

### 5. `examples/boundfk`

Builds the fixture the success criteria assert against. Depends on workstreams 1-4 so the generated output exercises every path.

| # | Task | File | Status |
|---|------|------|--------|
| 5.1 | `Ring` with UUID `id` (`Default(uuid.New)`) and `To` edges `versions`, `notes`, `tags` | `examples/boundfk/ent/schema/ring.go` | [x] |
| 5.2 | `RingVersion`: `ring_id` UUID `Immutable`, `version` int; edge `ring` From `Ring`, `Ref("versions")`, `.Field("ring_id")`, `Unique().Required().Immutable()`, annotated `Edge(IDs())` | `examples/boundfk/ent/schema/ringversion.go` | [x] |
| 5.3 | `RingNote`: `ring_id` UUID `Optional`, `text` string; edge `ring` From `Ring`, `Ref("notes")`, `.Field("ring_id")`, `Unique()` | `examples/boundfk/ent/schema/ringnote.go` | [x] |
| 5.4 | `RingTag`: `ring_id` UUID with `Field(FIQL(EQ, NEQ))`, `name` string `Unique()`; edge `ring` From `Ring`, `Ref("tags")`, `.Field("ring_id")`, `Unique().Required()`, annotated `Edge(Nest())` | `examples/boundfk/ent/schema/ringtag.go` | [x] |
| 5.5 | `entc.go` (`//go:build ignore`) with `WithPackagePath("examples/boundfk/internal/domain")`, `WithPackageName("domain")`, `gen.FeatureUpsert`, no `WithProto` | `examples/boundfk/ent/entc.go` | [x] |
| 5.6 | Add `gen-boundfk`, append it to `gen`, `.PHONY`, and the help text | `Makefile` | [x] |
| 5.7 | Run `make gen-boundfk`; generated output stays uncommitted until the author reviews | `examples/boundfk/internal/domain/ringversion_gen.go` | [x] |

**Key details:** all four entities carry `entdomain.Entity()`. `RingTag.name` is unique so the upsert test can conflict on it (ID is an auto-increment int, `SetID` is unavailable). No `WithProto` here because `.pb.go` needs a manual `buf generate` (scope Out of Scope); proto output is asserted in workstream 6.

### 6. Tests

Each test maps to one Success Criterion. Written alongside the workstream that enables it where possible; consolidated here for tracking.

| # | Task | File | Status |
|---|------|------|--------|
| 6.1 | `TestToDomainReadsFKWithoutEagerLoad`, `TestCreateApplyDomainWritesFK` (SC 3, 4) | `examples/boundfk/ent/boundfk_e2e_test.go` | [x] |
| 6.2 | `TestUpdateFKMutableAndImmutable` (SC 5) | same | [x] |
| 6.3 | `TestOptionalFKNullRoundTrip`, `TestOptionalFKNilDoesNotClear` (SC 6, 7) | same | [x] |
| 6.4 | `TestNestAndFKCoexist`, `TestUpsertApplyDomainWritesFK`, `TestFIQLFilterByFK` (SC 9, 10, 11) | same | [x] |
| 6.5 | `edgeIDsDeduped` positive case through `entc.LoadGraph` (`RingVersion.ring` true, `Ring.versions` false); generated-text assertions: single `RingID uuid.UUID` in `ringversion_gen.go`, `RingID *uuid.UUID` in `ringnote_gen.go`, one `RingVersionDomainFieldRingID` in `ent/domain.go`, the FIQL literal in `ent/fiql.go` (SC 1, 2, 8, 11) | `boundfk_example_test.go` | [x] |
| 6.6 | Proto: `entc.LoadGraph` on `examples/boundfk/ent/schema`; each message has one `ring_id`, `RingNote` has `optional string ring_id`, each mapper has one `RingId:` per direction (SC 12) | `proto_boundfk_test.go` | [x] |

**Key details:** copy the e2e shape from `examples/sliced/ent/slice_e2e_test.go` (sqlite via `enttest.Open`). `proto_boundfk_test.go` follows `proto_slice_test.go`: real schema through `entc.LoadGraph`, output into `t.TempDir()`. A spike with the same scenarios passed on the prototype; its schema and test live only in the session scratchpad, so rewrite them from this table.

### 7. Documentation and close

| # | Task | File | Status |
|---|------|------|--------|
| 7.1 | README section "Foreign key fields": bound fields appear as regular domain fields; optional keys are `*T`; `nil` leaves the FK unchanged; the one-line note for `Optional()` bound field plus `Edge(IDs())` (type becomes `*T`) | `README.md` | [x] |
| 7.2 | Add `examples/sliced`, `examples/sliceddomain`, and `examples/boundfk` to the examples tree | `CONTRIBUTING.md` | [x] |

### Verification gate

| # | Task | Status |
|---|------|--------|
| V.1 | `make build` exits 0 | [x] |
| V.2 | `make test` exits 0 | [x] |
| V.3 | `make gen` then `git diff --exit-code HEAD -- examples/basic examples/custom examples/sliced examples/sliceddomain` exits 0 (SC 14) | [x] |
| V.4 | `make gen` then `git diff --exit-code HEAD` exits 0 after the commits (SC 15) | [ ] |
| V.5 | `harness validate-docs` exits 0; `harness validate-pr-checklist` exits 0 after the commits | [ ] |

## Design Decisions

### Dedupe by `e.Unique && e.Field() != nil`, not by name
An edge name and its bound field name can differ (`edge "parent"` with `field "parent_ref"`). Comparing names would miss that case and emit two fields. The edge-level predicate covers every shape and lives in one function.

### Optional keys are `*T`, NULL reads as `nil`
entdomain maps `Optional` to `*T` everywhere; bound fields follow it. ent scans NULL into the zero value, so `ToDomain` must convert zero to `nil`, otherwise a read-then-write round trip would insert `uuid.Nil` and violate the FK.

### `nil` never clears
No optional field in entdomain clears on `nil` (scope A4). Adding it for FKs alone would split the convention, so it is out of scope. Callers who must clear a key use the ent builder (`ClearRingID`).

### One example, no compiled proto
`examples/boundfk` skips `WithProto` to avoid a manual `buf generate`. Proto behavior is pinned by text assertions against a generated temp directory.

## What Stays Unchanged

- Entities with no edge-bound fields: `examples/basic`, `examples/custom`, `examples/sliced`, `examples/sliceddomain` regenerate byte-identical (SC 14).
- `IDs()` behavior for edges without a bound field, including the read-only unique-edge rule at `template/domain.tmpl:184` (scope Out of Scope).
- `template.go:179` (`hasEnumFieldsFn`) and `template/domain.tmpl:97`: enum-typed FKs cannot exist because ent rejects them at generation (scope A7), so these guards stay as they are.
- `slice.go`, `annotation.go`, `extension.go`: slices are unrelated to this change.
- `proto_lock.go`, `proto_types.go`: field numbering is by name; no change needed.
- Clear semantics for any optional field (scope Out of Scope).

## Implementation Order

    1. WS1 Generator core      ← every other workstream calls edgeIDsDeduped / PtrNonZero; the struct change must precede the template that sets the new field
    2. WS2 Ent mapper template ← needs WS1; the struct gains RingID, so the template must set it in the same change
    3. WS3 Proto               ← needs edgeIDsDeduped (WS1); independent of WS2, can parallel it
    4. WS4 FIQL                ← needs only the template.go skips removed; independent of WS2 and WS3
    5. WS5 examples/boundfk    ← needs WS1-WS4 so generated output covers struct, mapper, proto and FIQL
    6. WS6 Tests               ← 6.1-6.5 read the WS5 output; 6.6 reads the WS5 schema
    7. WS7 Docs                ← documents verified behavior from WS5/WS6
    8. Verification gate       ← runs last; V.3 and V.4 need the committed tree

## Notes

- Editing `template/*.tmpl` or root `*.go` triggers `harness hook-postbuild`, which runs `make gen`. Inspect the diff before every commit; CI fails on divergence.
- Branch strategy: new GitButler branch off `main`. The local branch `chore-remove-changelog` (commit "docs: remove CHANGELOG.md…") is unrelated and unpushed; keep this work on its own stack.
- No CHANGELOG file exists; user-visible notes go in README and the GitHub release notes (see `CONTRIBUTING.md` "Release notes").
- ent `v0.14.7-0.20261014065818-8037bc89bcab` generates `RingID uuid.UUID` for optional bound fields (scope A3); a future ent release that changes this needs `PtrNonZero` removed for that case.
- Reference report: `entdomain-issue-edge-bound-fk.md` (external, not in the repo). Its acceptance bullet "nil pointer clears the FK" is intentionally not implemented (scope Out of Scope); tell the reporter.

## Discoveries & Decisions During Implementation

### [Implementer] The first prototype missed three FIQL skip sites
The spike removed the skips in the struct builder, template and proto generators, then reported FIQL "passing" while `fiql.go` held no `ring_id` entry. `hasFIQLFieldsFn` and `hasFIQLNodesFn` (`template.go`) and `template/fiql.tmpl` each had their own `IsEdgeField()` skip, so a field the author annotated was silently dropped. The post-spike check (generate, then grep the output) found it; the job lists all 12 sites explicitly (WS2-WS4) because a build never fails when one is missed.

### [Implementer] A scripted `if`/`end` removal needs `else if` counted as continuation
Removing `{{- if not $f.IsEdgeField }}` and its matching `{{- end }}` was scripted by brace counting. The first version subtracted a count for `{{- else if` lines, which never opened a block, so the match ended early and `domain.tmpl` failed to parse (`expected end; found {{else}}`). Only the template-parse panic in `make gen-basic` exposed it. The fixed script matched each removal at a different line than the broken one (for example create at 142→179, not 142→167), and the diff was checked by `make gen` leaving all four existing examples unchanged.

### [Implementer] Task 1.6 positive case moved to a real graph
`gen.Edge.Field()` reads the unexported foreign-key relation, so a `gen.Edge` literal can only test the false cases. The positive case runs through `entc.LoadGraph` on `examples/boundfk/ent/schema` in `TestEdgeIDsDeduped_RealGraph` (task 6.5), which also asserts the `To` side stays false. Job rows 1.6 and 6.5 were edited to match.

### [Implementer] Proto test lives in its own file, helpers in the example test
Scope Success Criterion 12 names `proto_boundfk_test.go`, so `TestBoundFK_Proto` is there, while `loadBoundFKGraph` and the generated-text assertions are in `boundfk_example_test.go` (same package). The proto assertion reads `outDir/examples/boundfk/proto/entpb/ent_messages.proto`; a glob under `outDir` matched nothing because `generateProtoFiles` nests by `cfg.dir/cfg.pkgName`.

### [Implementer] Spike coverage beyond the plan
Before the job started, three questions were probed on a throwaway prototype and recorded as scope A7-A9: enum FKs cannot exist (ent rejects them), bound fields compose with slices in both mapper modes against a real `.pb.go`, and upsert honors `nil`. `examples/boundfk` regenerates under the real generator with the same output as the spike, and `make gen` left `examples/basic`, `examples/custom`, `examples/sliced` and `examples/sliceddomain` unchanged.

### [Implementer] Local lint
The installed `golangci-lint` is v1.64.8 while CI pins v2.11.4, so lint ran as `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.11.4 run ./...` and reported 0 issues. `gofmt -l` still lists `generate.go:301` and two other files; those are pre-existing and out of scope.

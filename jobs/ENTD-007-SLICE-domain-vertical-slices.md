# ENTD-007-SLICE: Domain vertical slices (slice by bounded context, cross-slice by ID, proto included)

| Field      | Value                                  |
|------------|----------------------------------------|
| Status     | Done                                   |
| Created    | 2026-11-13                             |
| Assignee   | danhtran94                             |
| Source     | docs/scope-domain-vertical-slices.md   |
| Blocked by | —                                      |

<!-- approved -->
<!-- approval: user instruction "làm hết job, đừng commit để tôi review" (2026-11-13) -->

## Goal

Let a schema opt entities into named slices with `entdomain.Entity(entdomain.Slice("billing"))`. The generator then writes each slice's domain structs to `{pkgPath}/{slice}/` as `package {slice}`, and its proto mappers to `{pkgPath}/{slice}/pbmap/` (or into the slice package under `WithProtoHelpersInDomain()`). `ent/domain.go` imports one domain package per slice under the alias `<slice>domain`. Edges inside a slice behave as today. A `Nest()` edge across slices fails generation with `cross-slice Nest`; an `IDs()` edge across slices emits a bare ID field and no import. Entities without `Slice` produce byte-identical output to today.

The job ships when the 14 Success Criteria in the source scope note pass, verified by the new `examples/sliced` example and by `make build`, `make test`, and `make gen` followed by `git diff --exit-code HEAD`, all exiting 0.

## Problem

The domain layer is single-package end to end (source scope note, Problem and A1):

    Current:
      Entity (any) ──► generateDomainFiles ──► {pkgPath}/{entity}_gen.go   (one dir, one package)
                  └──► domain.tmpl         ──► ent/domain.go imports ONE domain pkg ($pkgName, 37 uses)
                  └──► generateProtoMapperFiles ──► {pkgPath}/pbmap/*.go  (one dir, one domainGoImport)

    Naive per-entity packages ✗  User.Posts ↔ Post.Owner  = Go import cycle

## Solution: slice groups, qualified by the referenced type

One helper partitions the graph into slice groups. Every generator iterates the groups instead of assuming one package. Cross-slice Nest is rejected up front, so no group ever needs another group's types.

    After:
      validateSliceName / validateSliceEdges   (fail before any file is written)
              │
      groupBySlice(g) ──► [root | billing | identity ...]
              │
              ├─► generateDomainFiles   ──► {pkgPath}/{slice}/{entity}_gen.go      package {slice}
              ├─► domain.tmpl           ──► ent/domain.go, alias = domainAlias(type)
              └─► generateProtoMapperFiles ──► {pkgPath}/{slice}/pbmap/*.go        one dir per group

### Components

**`slice.go` (new)** — owns slice rules and the shared grouping.
- `validateSliceName(name string) error`: `^[a-z][a-z0-9]*$`, not a Go keyword, not `pbmap`.
- `validateSliceEdges(nodes []*gen.Type) error`: rejects Nest across two different slices. Single place Option 2 will later replace.
- `groupBySlice(nodes []*gen.Type, pkgPath, pkgName string) ([]sliceGroup, error)`: root group first (`Dir=pkgPath`, `PkgName=pkgName`, `Alias=pkgName`), then named groups sorted by name (`Dir=pkgPath/slice`, `PkgName=slice`, `Alias=slice+"domain"`).

**`domainAlias` / `domainImports` (template funcs in `template.go`)** — `domainAlias(cfg, t)` returns the import alias for the type's slice (root: the configured package name). `domainImports(g, cfg)` returns the `{Alias, Path}` pairs `domain.tmpl` must import.

### Why not alternatives

| Approach | Verdict |
|---|---|
| **Per-slice `pbmap`, grouped by `groupBySlice`** | Chosen. Slice owns its mapper, imports only its own domain package, helpers file generated per dir (already parameterised by `outDir`). |
| One shared `pbmap` importing every slice | Rejected: re-couples all slices in one adapter package, defeating the slice boundary. |
| Slice set through an extension option (`WithSlice("billing", "Invoice", ...)`) | Rejected: separates the slice from the schema it describes; annotation matches how `NoBulk` and `VirtualField` are already declared. |
| Auto-downgrade cross-slice Nest to IDs | Rejected: later relaxing an error is non-breaking, later changing a silent downgrade is breaking. |

## Workstreams

### 1. Slice model and validation

Adds the annotation, the validation, and the grouping helper. Everything else reads `Slice` and `groupBySlice`, and validation must run before any generator writes a file.

| # | Task | File | Status |
|---|------|------|--------|
| 1.1 | Add `Slice string` to `EntityAnnotation`, add `Slice(name string) entityOption`, make `Merge` keep a non-empty `Slice` from `other` | `annotation.go` | [x] |
| 1.2 | Add `validateSliceName`, `validateSliceEdges`, `sliceGroup`, `groupBySlice` | `slice.go` | [x] |
| 1.3 | Call `validateSliceName` per entity and `validateSliceEdges` once at the top of `generateDomainHook`, before `generateDomainFiles` | `extension.go` | [x] |
| 1.4 | Unit tests: `validateSliceName` table (Success Criterion 8), `validateSliceEdges` error text with entity and edge name and `<root>` (SC 7), `groupBySlice` dirs/pkg names/aliases | `slice_test.go` | [x] |

**Key details:** `validateSliceEdges` takes `[]*gen.Type`; tests build `gen.Type` / `gen.Edge` literals directly, no graph load. The edge's Nest-ness comes from `extractEdgeAnnotation(e.Annotations).HasNest()`. Error text ends with `use entdomain.IDs() for this edge or move both entities into one slice`.

### 2. Domain structs and ent mapper per slice

Moves struct generation and the ent mapper imports onto slice groups. Tasks 2.1-2.4 land as one atomic change: structs in slice directories without the matching mapper imports leave `ent/domain.go` uncompilable, and `hook-postbuild` runs `make gen` after each matching edit.

| # | Task | File | Status |
|---|------|------|--------|
| 2.1 | `generateDomainFiles`: iterate `groupBySlice`, `MkdirAll` each `Dir`, resolve `ownImportPath` per group, render with the group's `PkgName` | `generate.go` | [x] |
| 2.2 | Register `domainAlias` and `domainImports` in `TemplateFuncs` | `template.go` | [x] |
| 2.3 | Replace the single `$pkgName` import with `range domainImports $` and every `$pkgName.` qualifier with `domainAlias` of the referenced type (own type: `$n`; edge target: `$e.Type`) | `template/domain.tmpl` | [x] |
| 2.4 | Unit tests: `domainAlias` returns `domain` for root and `billingdomain` for slice `billing`; `domainImports` lists one pair per group | `template_internal_test.go` | [x] |

**Key details:** qualifying by the referenced type, not by the current entity, is what lets Option 2 land later without touching this template. `make gen` on `examples/basic` and `examples/custom` must stay byte-identical (SC 2).

### 3. `examples/sliced` (domain only)

Builds the fixture the success criteria assert against. Proto is added in workstream 4, so this workstream verifies Success Criteria 2-6 first.

| # | Task | File | Status |
|---|------|------|--------|
| 3.1 | `Invoice` in slice `billing`: `number`, `total`; edge `line_items` (To `LineItem`, `Nest`); edge `account` (From `Account`, `Ref("invoices")`, unique, `IDs`) | `examples/sliced/ent/schema/invoice.go` | [x] |
| 3.2 | `LineItem` in slice `billing`: `description`, `qty`; edge `invoice` (From `Invoice`, `Ref("line_items")`, unique, `Nest`) for the mutual Nest | `examples/sliced/ent/schema/lineitem.go` | [x] |
| 3.3 | `Account` in slice `identity`: `email`; edge `invoices` (To `Invoice`, `IDs`) | `examples/sliced/ent/schema/account.go` | [x] |
| 3.4 | `entc.go` modelled on `examples/custom/repo/ent/entc.go`, `WithPackagePath("examples/sliced/internal/domain")`, `WithPackageName("domain")` | `examples/sliced/ent/entc.go` | [x] |
| 3.5 | Add `gen-sliced`, append it to `gen`, `.PHONY`, and the help text | `Makefile` | [x] |
| 3.6 | Run `make gen-sliced`; generated output is left uncommitted for author review | `examples/sliced/internal/domain/billing/invoice_gen.go` | [x] |
| 3.7 | Assert SC 3 (package names, no root `*_gen.go`), SC 4 (`ToDomain` signature with `billingdomain`), SC 5 (`LineItems LineItemList`), SC 6 (`AccountID int`, zero `identity`) | `slice_example_test.go` | [x] |

**Key details:** `examples/sliced/ent/entc.go` carries `//go:build ignore` like the other examples. Edge `account` is unique, so its IDs field is `AccountID int`; `Account.invoices` is non-unique, so it yields `InvoiceIDs []int` in `identity/account_gen.go` with no `billing` import.

### 4. Proto mappers per slice

Makes the mapper generator iterate slice groups, then turns proto on in the example and commits the compiled `.pb.go`. Depends on workstream 1 (`groupBySlice`) and on workstream 3 (the example to generate).

| # | Task | File | Status |
|---|------|------|--------|
| 4.1 | `generateProtoMapperFiles`: loop `groupBySlice`; per group set `mapperOutDir` (`Dir/pbmap` or `Dir`), `mapperPkgName` (`pbmap` or `PkgName`), `domainGoImport`/alias (`resolveGoImportPath(g, Dir)`, `Alias`); write helpers once per dir that needs them | `proto_mapper_generate.go` | [x] |
| 4.2 | Tests: domain mode on a two-slice graph into `t.TempDir()` asserts `billing/invoice_proto_gen.go` starts `package billing` and the root dir stays empty (SC 12); subpackage mode asserts `billing/pbmap` and no root `pbmap` | `proto_slice_test.go` | [x] |
| 4.3 | Add `WithProto(WithProtoDir("examples/sliced/proto"), WithProtoPackageName("entpb"), WithProtoGoPackage(...))` | `examples/sliced/ent/entc.go` | [x] |
| 4.4 | Copy `buf.yaml` and `buf.gen.yaml` from `examples/basic/proto/` | `examples/sliced/proto/buf.gen.yaml` | [x] |
| 4.5 | Run `make gen-sliced`, then `buf generate` in `examples/sliced/proto/`; `ent_messages.proto` and `ent_messages.pb.go` are left uncommitted for author review | `examples/sliced/proto/entpb/ent_messages.pb.go` | [x] |
| 4.6 | Document the exact `buf generate` command and why it is manual | `examples/sliced/proto/README.md` | [x] |
| 4.7 | Assert SC 9 (three messages), SC 10 (`package pbmap`, `InvoiceToProto(d *billingdomain.Invoice)`, no root `pbmap`), SC 11 (zero `identitydomain` in the billing mapper) | `slice_example_test.go` | [x] |

**Key details:** the mapper templates need no change because `DomainGoImport` and `DomainPkgAlias` are already per-file (scope A7). Root entities keep alias `domain` and dir `{pkgPath}/pbmap`, so `examples/basic` output stays byte-identical. For 4.2, build the graph with `entc.LoadGraph` on `examples/sliced/ent/schema`; if the load is slower than 10 seconds under `go test`, fall back to `gen.Type` literals and record the fallback in Discoveries.

### 5. Documentation and close

| # | Task | File | Status |
|---|------|------|--------|
| 5.1 | README section "Vertical slices": annotation, same-slice vs cross-slice rule, alias naming, per-slice mappers, and the migration note naming the two orphan files (`{entity}_gen.go`, `{entity}_proto_gen.go`) to delete | `README.md` | [x] |
| 5.2 | Changelog entry under `[Unreleased]` / `### Added` | `CHANGELOG.md` | [x] |

### 6. `WithConversion` per slice (added after review question)

Pins how hand-written proto conversion functions behave with slices. No generator change: the expression is emitted unqualified, so it resolves in the slice's mapper package.

| # | Task | File | Status |
|---|------|------|--------|
| 6.1 | `Invoice.number` gets `ProtoType("string").WithConversion("InvoiceNumberToProto(%s)", "InvoiceNumberFromProto(%s)")` | `examples/sliced/ent/schema/invoice.go` | [x] |
| 6.2 | Hand-written conversion functions in the billing mapper package | `examples/sliced/internal/domain/billing/pbmap/number_conv.go` | [x] |
| 6.3 | Test: mapper calls both functions unqualified, they are defined in `billing/pbmap`, and `identity/pbmap` has none | `slice_example_test.go` | [x] |
| 6.4 | README paragraph "Custom proto conversions" under Vertical Slices | `README.md` | [x] |
| 6.5 | Scope Risks entry for per-slice conversions | `docs/scope-domain-vertical-slices.md` | [x] |

### 7. Runtime end-to-end tests (added after review question)

The generation tests assert file contents and compilation only. These run the generated slice code against sqlite and through the per-slice `pbmap` packages.

| # | Task | File | Status |
|---|------|------|--------|
| 7.1 | `ToDomain` across two slices: same-slice Nest loads `LineItems`, cross-slice edge arrives as `AccountID` / `InvoiceIDs` | `examples/sliced/ent/slice_e2e_test.go` | [x] |
| 7.2 | Bulk create, `ent.Invoices(...).ToDomain()` returns `billing.InvoiceList`, nil element returns an error | same | [x] |
| 7.3 | `ApplyDomain` writes the non-unique cross-slice IDs edge (`Account.invoices`); ent's own ownership constraint still applies | same | [x] |
| 7.4 | `ApplyDomain` leaves the unique cross-slice IDs edge (`Invoice.account`) untouched | same | [x] |
| 7.5 | Proto round trip through `billing/pbmap` and `identity/pbmap`, including the `WithConversion` functions | same | [x] |

### 8. Domain mode (`WithProtoHelpersInDomain`) end to end (added after review question)

No example used domain mode, with or without slices. This adds one and runs it.

| # | Task | File | Status |
|---|------|------|--------|
| 8.1 | Schemas copied from `examples/sliced` without `WithConversion` | `examples/sliceddomain/ent/schema/invoice.go` | [x] |
| 8.2 | `entc.go` with `WithProtoHelpersInDomain()` | `examples/sliceddomain/ent/entc.go` | [x] |
| 8.3 | `gen-sliceddomain` target, added to `gen`, `.PHONY`, help | `Makefile` | [x] |
| 8.4 | `buf generate` in the example (`.pb.go` left uncommitted for review) | `examples/sliceddomain/proto/entpb/ent_messages.pb.go` | [x] |
| 8.5 | Runtime e2e: `d.ToProto()`, `billing.InvoiceFromProto`, list `ToProto`, helper-using `identity` mapper | `examples/sliceddomain/ent/slice_domain_e2e_test.go` | [x] |
| 8.6 | Layout assertions: package names, no `pbmap` dirs, no root files, billing never mentions identity | `slice_example_test.go` | [x] |
| 8.7 | Point to the example from README | `README.md` | [x] |

### Verification gate

| # | Task | Status |
|---|------|--------|
| V.1 | `make build` exits 0 (type-checks every slice mapper against the `.pb.go` produced by `buf generate`, SC 13) | [x] |
| V.2 | `make test` exits 0 | [x] |
| V.3 | `make gen` leaves the working tree unchanged (hash before == after) and `git diff HEAD -- examples/basic examples/custom` is empty (SC 2, SC 14) | [x] |
| V.4 | `harness validate-docs` exits 0; `harness validate-pr-checklist` exits 0 (run after the commits, passed) | [x] |

## Design Decisions

### Alias per entity, resolved in the template
`domain.tmpl` shadows `$pkgName` inside the per-entity loop with `domainAlias $cfg $n`. Every qualifier in that template names the entity's own type, so no further change was needed (see Discoveries). Cross-slice references do not exist in the template; they would appear in `generate.go` when Option 2 relaxes `validateSliceEdges`.

### Alias `<slice>domain`, package name `<slice>`
The alias cannot equal an ent per-entity package name such as `user` (scope A5). The package name stays the bare slice name so consumers read `billing.Invoice`.

### Validation before any write
`generateDomainHook` validates names and edges first. A rejected schema leaves the tree untouched, so a failed `go run entc.go` cannot half-migrate generated files.

### Slice is an annotation, not an extension option
It sits beside the entity it describes, matching `VirtualField` and `NoBulk`.

## What Stays Unchanged

- `template/fiql.tmpl`, `fiql.go`, `fiql_ast.go`, `kinds.go`: FIQL imports no domain package (scope A2).
- `proto_generate.go`, `proto_types.go`, `proto_lock.go`: proto message generation and the lock file stay single-package; messages are unique across slices (scope A8).
- `template/proto_mapper_subpkg.tmpl`, `template/proto_mapper_domain.tmpl`, `template/proto_helpers.tmpl`: template data is already per-file (scope A7).
- `runtime.go`, `context.go`: runtime helpers are package-agnostic.
- Generated output under `examples/basic/` and `examples/custom/`: byte-identical after `make gen` (SC 2).
- Behaviour of any entity without `Slice`: same dir, same package name, same alias `domain`.

## Implementation Order

    1. WS1 Slice model & validation   ← every generator reads Slice/groupBySlice; validation must exist before any write path uses it
    2. WS2 Structs + ent mapper       ← needs groupBySlice (WS1); atomic because slice dirs without matching ent imports do not compile
    3. WS3 examples/sliced (domain)   ← needs WS2 output to generate and to assert on; proves SC 2-6 before proto adds a second moving part
    4. WS4 Proto mappers + proto      ← needs groupBySlice (WS1) and the example (WS3); its .pb.go must be built from WS3's schema
    5. WS5 Docs                       ← documents verified behaviour from WS3 and WS4
    6. Verification gate              ← runs last; V.1 needs the checked-in .pb.go from WS4

## Notes

- Editing `template/*.tmpl` or root `*.go` triggers `harness hook-postbuild`, which runs `make gen` (`.claude/harness.json` `gen_triggers`). Inspect the `make gen` diff before every commit; CI fails on divergence.
- Branch strategy: new GitButler branch off `main`. PRs #61 (plural fix) and #63 (ent pre-release bump) are merged; the branch starts from `1c6379a` with ent `v0.14.7-0.20261014065818-8037bc89bcab` and `go 1.26.0`.
- No commits during execution (author reviews the working tree first). Commit message, when the author asks, leads with `(ENTD-007)`.
- `examples/sliced/proto/entpb/ent_messages.pb.go` is not regenerated by CI (scope A9). Any schema edit in `examples/sliced` needs `buf generate` by hand; a stale file fails `make build`.

## Discoveries & Decisions During Implementation

### [Implementer] `domain.tmpl` needed one shadowed variable, not 37 rewrites
The plan was to qualify every domain reference by the referenced type's slice. Reading the 37 `$pkgName` uses showed each one names the loop entity's own type; nested-edge types (`LineItemList`) are written by `generate.go`, never by this template. Defining `$pkgName := domainAlias $cfg $n` once per entity gave the same result. Consequence for Option 2: the cross-slice type reference lives in `generate.go` (`e.Type.Name + "List"`), so that is where it will need an import, not the template.

### [Implementer] Helper signatures differ from the plan
`groupBySlice` takes `[]*gen.Type` and returns `([]sliceGroup, error)` instead of `*gen.Graph`; this made `slice_test.go` pure literals with no graph. `domainAlias` takes the config map because the root alias is the user's `WithPackageName`. `validateSliceNames` was added next to `validateSliceEdges` so each entity's name error is reported with the entity. A `groupOf` helper written for the plan was only used by its own test and was removed.

### [Implementer] Empty root group rule keeps old behaviour
`groupBySlice` returns the root group when it has entities, or when no named slice exists. An empty graph therefore still creates the domain directory as before, and an all-sliced graph creates no root `pbmap` (Success Criterion 10). `make gen` on `examples/basic` and `examples/custom` produced zero diff, which is the byte-identical check.

### [Implementer] Proto helpers appear only in slices that need them
`proto_helpers_gen.go` exists in `identity/pbmap` (the `[]int64` IDs field) and not in `billing/pbmap`. The per-directory `hasHelpers` flag in `generateProtoMapperGroup` handles this with no template change, confirming scope assumption A7.

### [Implementer] `entc.LoadGraph` is cheap enough for tests
Loading `examples/sliced/ent/schema` takes about 1 second per test, well under the 10 second fallback threshold, so `proto_slice_test.go` uses the real schema and the `gen.Type` literal fallback was not needed.

### [Implementer] Local lint and `validate-pr-checklist` limits
`golangci-lint` on this machine is v1 while the repo config is v2, so lint was not run locally; `go vet`, `gofmt -l`, and `staticcheck`-style unused-code review were done by hand and CI runs the real lint. `harness validate-pr-checklist` compares regenerated files with HEAD, so it cannot pass before the work is committed (task V.4); it passed once the four commits were in. `buf generate` uses a remote plugin and needs network access.

### [Implementer] `WithConversion` resolves in the slice's mapper package
The generated mapper emits conversion expressions verbatim and `trackExprImports` auto-imports only `timestamppb`, `durationpb`, `uuid`, and `structpb`. A hand-written `UserMetadataToProto`-style function therefore has to sit in the entity's own `{slice}/pbmap` (or slice package in domain mode); a root `pbmap` file is invisible to sliced entities. Raised in review after the main job; handled by a fixture, a test, and a README note, not by generator changes. Qualified conversion expressions with generated imports are a possible follow-up.

### [Implementer] Unique IDs edges are read-only in `ApplyDomain` (pre-existing, not slice-related)
The first e2e draft assumed `ApplyDomain` writes `Invoice.AccountID`. It does not: `template/domain.tmpl:184` emits `AddXIDs` only for `IDs()` edges that are not unique, so `examples/basic`'s `Post.owner` behaves the same. The test now seeds the link with the ent builder and pins the behaviour (task 7.4). Non-unique cross-slice edges are written (7.3); ent additionally refuses to move an invoice that already belongs to another account.

### [Implementer] Generation tests alone missed this class of mistake
Before workstream 7 every test checked file text or compilation. The wrong assumption above compiled and generated fine; only running the mapper against sqlite exposed it. Slice changes to mapper output deserve a runtime test, not only a string match.

### [Implementer] Domain mode had no example, with or without slices
`WithProtoHelpersInDomain()` was exercised by no checked-in example before this job, so its generated code was never compiled or run in CI. `examples/sliceddomain` now builds it (method-receiver `ToProto`, `XxxFromProto`, per-slice helpers file only in `identity`) and runs a sqlite round trip. Because the example is regenerated by `make gen`, domain-mode output is also covered by the generation drift check.

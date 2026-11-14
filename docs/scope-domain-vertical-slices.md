# Scope: Domain vertical slices (KISS: slice by bounded context, cross-slice by ID, proto included)

| Field   | Value      |
|---------|------------|
| Status  | Accepted   |
| Created | 2026-11-13 |
| Author  | danhtran94 |

---

## Problem

The extension emits every domain struct into **one** Go package. `WithPackagePath` / `WithPackageName` (`extension.go:80`, `extension.go:89`) set a single `pkgPath` and `pkgName`. `generateDomainFiles` (`generate.go:67`) writes every `{entity}_gen.go` into `filepath.Join(moduleRoot(g), pkgPath)`. A project that organises code as vertical slices (`domain/billing`, `domain/identity`) cannot express that layout. Per A1, the single path is hard-wired through generation, not only through one config key.

Four places assume one package:

- **Struct generation.** `buildDomainFileData` (`generate.go:146`) types a nested edge as `e.Type.Name + "List"` (`generate.go:241`). The bare name only resolves inside one package.
- **Ent mapper template.** `template/domain.tmpl` reads one `$pkgName` / `$domainImport` pair (lines 16-22) and qualifies every domain type with it (37 occurrences, e.g. `*{{ $pkgName }}.{{ $n.Name }}` at line 74). `ent/domain.go` therefore imports exactly one domain package (`examples/basic/ent/domain.go:12`).
- **Proto mappers, location.** `generateProtoMapperFiles` (`proto_mapper_generate.go:145`) writes every `{entity}_proto_gen.go` into one directory: `{pkgPath}/pbmap` by default, or `{pkgPath}` itself under `WithProtoHelpersInDomain()` (`proto_mapper_generate.go:157-166`).
- **Proto mappers, imports.** The same function resolves one `domainGoImport` for the whole run (`proto_mapper_generate.go:168`) and passes it to every entity (`proto_mapper_generate.go:191-192`). The template data already carries `DomainGoImport` and `DomainPkgAlias` per file (`proto_mapper_generate.go:49-50`, per A7), so only the caller is single-valued.

A naive fix (point each entity at its own package) breaks on Go import cycles. `User.Posts PostList` in one package and `Post.Owner *User` in another cannot both compile. Mutual Nest, which the shipped example uses (`User` ↔ `Tag`, per A4), is exactly this shape. The single-package layout hides the problem today.

This scope covers the smallest design that avoids the cycle by construction. Slices are opt-in. Edges inside a slice behave exactly as today. Edges across slices carry IDs only. Proto mappers follow the slice: each slice owns its mapper package and imports only its own domain package.

## Assumptions

- **A1 [VERIFIED]:** The domain package path is a single value end to end. Evidence: `extension.go:191-194` stores one `PkgPath` / `PkgName` pair in `g.Annotations["EntDomainConfig"]`; `generate.go:61` (`domainImportPath`) and `template/domain.tmpl:16-22` consume only that pair.
- **A2 [VERIFIED]:** FIQL generation does not import the domain package. Evidence: `template/fiql.tmpl` references only `predicate.{{ $n.Name }}` and `entdomain.*`; `domainNodes` at line 11 selects annotated entities and adds no domain import.
- **A3 [VERIFIED]:** The generator never deletes files. `generateDomainFiles` and `generateProtoMapperFiles` only call `os.WriteFile` (`generate.go:93`, `proto_mapper_generate.go:208`); no `os.Remove` exists in either path. Moving an entity into a slice leaves the old `{entity}_gen.go` and `{entity}_proto_gen.go` where they were.
- **A4 [VERIFIED]:** Mutual Nest exists in the shipped example. Evidence: `examples/basic/domain/user_gen.go` has `Posts PostList` and `Tags TagList`; `examples/basic/ent/schema/tag.go:42-44` declares `users` with `Nest()` back to `User`.
- **A5 [VERIFIED]:** `ent/domain.go` already imports ent's per-entity packages under their bare names. Evidence: `examples/basic/ent/domain.go:13` imports `.../ent/user` as `user`. A slice named `user` would collide with it unless aliased.
- **A6 [HYPOTHESIS]:** Real consumers need slice-per-bounded-context and tolerate ID-only references across slices. Verification deferred: no consumer schema has been shared yet. Cross-slice Nest is wanted but not critical (author statement, 2026-11-13); it is a follow-up, not a defect here.
- **A7 [VERIFIED]:** Proto mapper template data is already per-entity for the domain import. Evidence: `protoMapperFileData` has `DomainGoImport` and `DomainPkgAlias` (`proto_mapper_generate.go:49-50`); `template/proto_mapper_subpkg.tmpl:8` renders them per file. The domain-mode template (`template/proto_mapper_domain.tmpl`) uses bare domain types and needs no import.
- **A8 [VERIFIED]:** Entity names are unique across all slices. Evidence: every entity compiles into one Go package (`gen.Config.Package` in `examples/custom/repo/ent/entc.go`), where two `type User struct` declarations do not compile. One proto package therefore keeps unique message names across slices.
- **A9 [VERIFIED]:** The compiled `entpb` package is not produced by `make gen`. Evidence: `examples/basic/proto/buf.gen.yaml` drives `buf generate`; `examples/basic/proto/entpb/ent_messages.pb.go` is checked in; `CONTRIBUTING.md:9` states `protoc` is not required at development time. A new example with proto needs its `.pb.go` generated by hand and committed.

## Success Criteria

1. **Opt-in annotation.** `entdomain.Entity(entdomain.Slice("billing"))` is accepted. `Slice` is declared in `annotation.go` and stored in a new `Slice string` field of `EntityAnnotation`. An entity without it stays in the root domain package.
2. **Backward compatibility.** On a tree with no `Slice` annotation, `make gen` followed by `git diff --exit-code HEAD` exits 0 for `examples/basic` and `examples/custom`.
3. **Output layout.** In the new `examples/sliced`, `examples/sliced/internal/domain/billing/invoice_gen.go` starts with `package billing`. `examples/sliced/internal/domain/identity/account_gen.go` starts with `package identity`. No `*_gen.go` file exists directly in `examples/sliced/internal/domain/`.
4. **Ent mapper imports.** `examples/sliced/ent/domain.go` contains the literal string `func (e *Invoice) ToDomain() *billingdomain.Invoice`. The import alias is always `<slice>domain` for a named slice. The alias for the root package stays `$pkgName`, unchanged.
5. **Same-slice edges unchanged.** In `examples/sliced`, `Invoice` and `LineItem` share slice `billing` with a mutual Nest. `invoice_gen.go` contains the literal field `LineItems LineItemList`.
6. **Cross-slice edge carries IDs only.** In `examples/sliced`, `Invoice` has an `IDs()` edge to `Account` (slice `identity`). `invoice_gen.go` contains the literal field `AccountID int`. The file contains zero occurrences of the string `identity`.
7. **Cross-slice Nest is rejected.** `slice_test.go` calls `validateSliceEdges` with a `Nest()` edge between two different slices. The returned error message contains the literal `cross-slice Nest`, the entity name, and the edge name.
8. **Slice name validation.** `slice_test.go` asserts that `validateSliceName` rejects `""`, `"Billing"`, `"a/b"`, `"pbmap"`, and `"func"`, and accepts `"billing"`. The rule is `^[a-z][a-z0-9]*$`, not a Go keyword, not `pbmap`.
9. **One proto package.** `examples/sliced/proto/entpb/ent_messages.proto` contains the literals `message Invoice`, `message LineItem`, and `message Account`. Proto message generation is unchanged for sliced entities.
10. **Per-slice mapper packages (default subpackage mode).** `examples/sliced/internal/domain/billing/pbmap/invoice_proto_gen.go` starts with `package pbmap` and contains `func InvoiceToProto(d *billingdomain.Invoice)`. `examples/sliced/internal/domain/identity/pbmap/account_proto_gen.go` exists. The directory `examples/sliced/internal/domain/pbmap` does not exist.
11. **Mapper packages do not import sibling slices.** `examples/sliced/internal/domain/billing/pbmap/invoice_proto_gen.go` contains zero occurrences of the string `identitydomain`.
12. **Per-slice mapper packages (domain mode).** `proto_slice_test.go` runs the mapper generator with `WithProtoHelpersInDomain()` into a temporary directory on a two-slice input. It asserts that `billing/invoice_proto_gen.go` starts with `package billing` and that no mapper file lands in the root directory.
13. **Compiled proto in the example.** `examples/sliced/proto/entpb/ent_messages.pb.go` is checked in, produced by `buf generate` in `examples/sliced/proto/`. `make build` exits 0, which type-checks every slice mapper against it.
14. **Gate stays green.** `make build`, `make test`, and `make gen` followed by `git diff --exit-code HEAD` all exit 0. `examples/sliced` is regenerated by `make gen` through a new `gen-sliced` target.

## Out of Scope

- **Cross-slice Nest (the "option 2" design).**
  - Reason: needs cycle detection across the slice graph and dynamic per-file imports. The author rates it wanted but not critical (A6).
  - Relaxing the Success Criterion 7 error later is non-breaking. Silently downgrading Nest to IDs now would not be.
- **One `.proto` file or proto package per slice.**
  - Reason: cross-slice edges are IDs only, so a single proto package has no cycle and unique message names (A8).
  - Splitting adds `buf` package churn without removing any coupling.
- **Running `buf generate` inside `make gen`.**
  - Reason: CI has no `buf` and the existing `examples/basic` has the same manual step (A9). Drift in the sliced `.pb.go` surfaces as a `make build` failure instead.
- **Entity-per-package layout ("option 3").**
  - Reason: mutual Nest between any two entities forms a cycle, so the layout collapses to ID-only everywhere.
- **Splitting the ent package per slice.**
  - Reason: ent codegen stays a single package; only the domain layer and its mappers are sliced.
- **De-prefixing enum type names (`UserStatus` to `Status`).**
  - Reason: breaking change for every consumer, and not required to slice.
- **Deleting orphaned files when an entity moves slice.**
  - Reason: the generator has never deleted files (A3); adding deletion is a separate safety decision.
- **FIQL changes.**
  - Reason: FIQL does not import the domain package (A2).

## Risks

- **Alias collision in `ent/domain.go`.** A slice named `user` collides with ent's own `user` package import (A5).
  - Mitigation: Success Criterion 4 fixes the alias to `<slice>domain` (`userdomain`), which cannot equal an ent per-entity package name. Mapper files in Success Criterion 10 use the same alias.
- **Root versus named slice confusion.** An entity without `Slice` sits in the root package. An edge between a root entity and a sliced entity is cross-slice, so a Nest there is rejected.
  - Mitigation: the Success Criterion 7 error names both slices and prints the root slice as `<root>`, so the cause is readable.
- **Users hit the Nest error and do not know the fix.**
  - Mitigation: the error text ends with `use entdomain.IDs() for this edge or move both entities into one slice`.
- **Option 2 later rewrites validation logic.**
  - Mitigation: put the edge check in one function `validateSliceEdges` in a new file `slice.go`. Option 2 replaces its body with cycle detection and touches no other file.
- **Orphaned files after moving an entity into a slice (A3).** The old `{entity}_gen.go` and `{entity}_proto_gen.go` remain in the root package. They still compile but are dead and stale. The `make gen` diff gate does not flag them because the files are unchanged.
  - Mitigation: add a README migration note naming both files to delete. Generator deletion is deferred (Out of Scope).
- **Stale checked-in `.pb.go` in `examples/sliced` (A9).** CI does not regenerate it, so a schema edit can leave it behind.
  - Mitigation: Success Criterion 13 makes `make build` the guard, because a mapper that references a proto field absent from the stale file fails to compile. `examples/sliced/proto/README.md` gives the exact `buf generate` command.
- **Domain-mode mapper placement is easy to get wrong.** Under `WithProtoHelpersInDomain()` the mapper joins the slice's domain package, so its package name must be the slice name, not the root `pkgName`.
  - Mitigation: Success Criterion 12 asserts `package billing` on the generated file, so a wrong name fails a test.
- **Hand-written `WithConversion` functions resolve per slice.** The generated mapper calls the expression unqualified (`trackExprImports` only auto-imports `timestamppb`, `durationpb`, `uuid`, `structpb`), so the function must live in the entity's slice mapper package, not in a root `pbmap`.
  - Mitigation: README "Vertical Slices" documents the rule, and `slice_example_test.go` (`TestSliced_WithConversionResolvesInSliceMapperPackage`) pins it against `examples/sliced/internal/domain/billing/pbmap/number_conv.go`. Qualified conversion expressions with generated imports are deferred.
- **Third example inflates CI time.** `examples/sliced` adds a `go run` generation to `make gen`.
  - Mitigation: reuse the existing `gen-custom` pattern in the `Makefile`; measure CI wall time on the first PR and revisit only if `test` exceeds the current runtime by more than 30 seconds.

## History

| Ticket    | Title                                                                                  | Created    | Status |
|-----------|----------------------------------------------------------------------------------------|------------|--------|
| (initial) | Domain vertical slices (KISS: slice by bounded context, cross-slice by ID, proto included) | 2026-11-13 | Accepted |

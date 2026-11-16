# ENTD-010-INPUT: Proto mappers reject invalid input; FIQL filter survives `;` and `+`

| Field      | Value                                     |
|------------|-------------------------------------------|
| Status     | Done                                      |
| Created    | 2026-11-17                                |
| Assignee   | danhtran94                                |
| Source     | docs/scope-untrusted-input-proto-fiql.md  |
| Blocked by | —                                         |

<!-- approved -->
<!-- approval: user reply "b" (2026-11-17) to the prompt "say approved with (a) or (b)": approved, regenerate entpb with local protoc-gen-go -->

## Goal

Stop untrusted input from crashing the process or being silently rewritten. After the job, every generated `XFromProto` returns `(*domain.X, error)` and `XListFromProto` returns `(domain.XList, error)`. A malformed UUID, an `UNSPECIFIED` or unknown non-optional enum, or a failing nested edge yields an error naming `Entity.field`, never a panic or an invented value (scope U1 = B). A new runtime helper `entdomain.FIQLQueryParam` lets the README handler read `;` and `+` intact and reject a present-but-empty filter (scope U2 = B). The job ships when the 10 Success Criteria in the source scope pass.

## Problem

    Current:
      UserFromProto(&entpb.User{ExternalId: ""})      → uuid.MustParse panics            ✗ crash
      UserFromProto(&entpb.User{Status: UNSPECIFIED}) → domain Status "active"           ✗ invented value
      GET /users?filter=name==alice;score=gt=10       → r.URL.Query().Get = "" → no WHERE ✗ unfiltered

All three reproduced (scope A1-A3).

## Solution: fallible fields after the struct literal

    After (generated, both mapper modes):
      func UserFromProto(p *entpb.User) (*domain.User, error) {
          if p == nil { return nil, nil }
          d := &domain.User{ /* infallible fields, unchanged */ }
          var err error
          if d.ExternalID, err = protoStringToUUID(p.ExternalId); err != nil {
              return nil, fmt.Errorf("User.external_id: %w", err)
          }
          if d.Status, err = userStatusFromProto(p.Status); err != nil {
              return nil, fmt.Errorf("User.status: %w", err)
          }
          // Nest edges: d.Posts, err = PostListFromProto(p.Posts) …
          return d, nil
      }

    FIQL:
      expr, ok, err := entdomain.FIQLQueryParam(r.URL.RawQuery, "filter")
        splits on '&' only → url.PathUnescape (keeps '+', ';') → first occurrence wins

### Components

**`protoMapperField.Fallible bool`** (`proto_mapper_generate.go`): set for UUID values and IDs, non-optional enums, and Nest edges. Templates emit fallible fields as checked assignments after the literal, and infallible ones inside it.

**UUID helpers** (`template/proto_helpers.tmpl`): new `protoStringToUUID(string) (uuid.UUID, error)`. `protoStringToUUIDPtr` and `stringSliceToUUIDSlice` return an error instead of `nil` / `uuid.Nil`.

**Enum `FromProto` helper** (`template/proto_mapper_subpkg.tmpl`, `template/proto_mapper_domain.tmpl`): returns `(Domain, error)`. `default:` returns `fmt.Errorf("unknown or unspecified value %v", s)` instead of `DefaultDomainConst`.

**`FIQLQueryParam(rawQuery, key string) (string, bool, error)`** (`fiql.go`): runtime helper.

### Why not alternatives

Recorded in the scope (U1, U2). In short: returning `uuid.Nil` (U1-A) swaps a crash for silent corruption; opt-in `ValidateXProto` (U1-C) is how P1 shipped. Docs-only encoding (U2-A) leaves the handler failing open; an inline parser in the README (U2-C) gets copied and drifts.

## Workstreams

### 1. Prerequisite: a working `entpb` for `examples/basic`

The committed `.pb.go` files panic on import, so no mapper test in `examples/basic` can run until they are regenerated.

| # | Task | File | Status |
|---|------|------|--------|
| 1.1 | Regenerate with `buf generate` in `examples/basic/proto` using the local `protoc-gen-go` (see Notes); confirm the diff is limited to the descriptor bytes; importing `entpb` no longer panics (SC 6) | `examples/basic/proto/entpb/ent_messages.pb.go` | [x] |
| 1.2 | Same check for `user_metadata.pb.go` | `examples/basic/proto/entpb/user_metadata.pb.go` | [x] |

### 2. Failing tests first

Prove P1-P3 on the current generator before changing it.

| # | Task | File | Status |
|---|------|------|--------|
| 2.1 | `TestUserFromProtoRejectsMalformedUUID`, `TestUserFromProtoRejectsUnspecifiedEnum`, `TestUserListFromProtoReportsIndex`, `TestUserProtoRoundTrip` (SC 2-5); 2.1-2.3 must fail on the current generator (compile error on the new signature counts) | `examples/basic/domain/pbmap/proto_mapper_e2e_test.go` | [x] |
| 2.2 | `TestReadmeFIQLHandler` with the handler as currently documented; must return 2 rows for `name==alice;score=gt=10` before the fix (SC 8) | `examples/basic/ent/fiql_http_e2e_test.go` | [x] |

### 3. Proto generator

Makes the mappers fallible. One atomic change: the helpers, field specs and both templates must agree on signatures or the examples stop compiling.

| # | Task | File | Status |
|---|------|------|--------|
| 3.1 | Replace `uuid.MustParse(%s)` with `protoStringToUUID(%s)` at `:113` and `:449`; mark UUID specs fallible | `proto_types.go` | [x] |
| 3.2 | Add `protoStringToUUID`; make `protoStringToUUIDPtr` and `stringSliceToUUIDSlice` return an error | `template/proto_helpers.tmpl` | [x] |
| 3.3 | Add `Fallible` to `protoMapperField`; set it for UUID, non-optional enum and Nest fields; update `trackExprImports` (`:556`, currently keyed on `uuid.MustParse`) and add the `fmt` import when any field is fallible | `proto_mapper_generate.go` | [x] |
| 3.4 | Subpackage mode: enum `FromProto` returns `(T, error)`; `XFromProto` / `XListFromProto` return errors; fallible fields emitted after the literal with `Entity.field` context; list errors carry `index %d` | `template/proto_mapper_subpkg.tmpl` | [x] |
| 3.5 | Same as 3.4 for `WithProtoHelpersInDomain` mode | `template/proto_mapper_domain.tmpl` | [x] |
| 3.6 | `make gen`; inspect the diff: only `*_proto_gen.go` and `proto_helpers_gen.go` under `examples/{basic,sliced,sliceddomain}` change | `examples/basic/domain/pbmap/user_proto_gen.go` | [x] |

**Key details:** `WithConversion` expressions stay infallible and unchanged (scope Out of Scope). Optional enums are not touched (their mapper does not compile today; Out of Scope). A nil `p` still returns `nil, nil`, so nested optional messages keep working.

### 4. Generator tests and existing callers

| # | Task | File | Status |
|---|------|------|--------|
| 4.1 | Replace the `uuid.MustParse(p.ExternalId)` assertion (`:259-261`) and the `FromProto` signature assertions (`:146`, `:148`, `:168`, `:199`, `:315`, `:317`, `:364`) with the new forms | `proto_generate_test.go` | [x] |
| 4.2 | Update the `uuid.MustParse in FromProto` subtest (`:214`) | `proto_types_test.go` | [x] |
| 4.3 | Update the Nest read-back regexp (`:62`) to the checked-assignment form | `proto_boundfk_test.go` | [x] |
| 4.4 | Adapt `FromProto` call sites to the `(v, err)` return (none found: the file only asserts `WithConversion` text; see Discoveries) | `slice_example_test.go` | [x] |
| 4.5 | Same, `:173` | `examples/sliced/ent/slice_e2e_test.go` | [x] |
| 4.6 | Same, `:63`, `:73` | `examples/sliceddomain/ent/slice_domain_e2e_test.go` | [x] |

### 5. FIQL query helper

| # | Task | File | Status |
|---|------|------|--------|
| 5.1 | `FIQLQueryParam(rawQuery, key string) (string, bool, error)`: split on `&`, match `key=` (a bare `key` counts as present and empty), `url.PathUnescape` the value, first occurrence wins | `fiql.go` | [x] |
| 5.2 | `TestFIQLQueryParam`: `;` kept, `+` kept, absent → `ok == false`, `filter=` → `("", true, nil)`, `%zz` → error, repeated key → first (SC 7) | `fiql_test.go` | [x] |
| 5.3 | Switch `TestReadmeFIQLHandler` (2.2) to the new README handler: 1 row for `name==alice;score=gt=10`, HTTP 400 for `filter=` (SC 8) | `examples/basic/ent/fiql_http_e2e_test.go` | [x] |

### 6. Documentation

| # | Task | File | Status |
|---|------|------|--------|
| 6.1 | "HTTP Handler Usage": the handler from 5.3, character for character; "no percent-encoding required" qualified to name `FIQLQueryParam` (SC 9) | `README.md` | [x] |
| 6.2 | "Mapper Usage": `d, err := pbmap.UserFromProto(req.User)`; state that malformed UUIDs and unset enums return errors | same | [x] |
| 6.3 | Amendment note: `r.URL.Query()` drops `;` since Go 1.17; use `FIQLQueryParam` | `docs/ADR-003-fiql-filter-generation.md` | [x] |
| 6.4 | Amendment note: `FromProto` is fallible (ENTD-010) | `docs/ADR-002-proto-generation.md` | [x] |
| 6.5 | Release-note draft in Notes: breaking mapper signature with one-line migration; new `FIQLQueryParam` | `jobs/ENTD-010-INPUT-untrusted-proto-fiql-input.md` | [x] |
| 6.6 | Mark the `uuid.MustParse` finding resolved (added during implementation, see Discoveries) | `docs/ADR-004-security-pipeline.md` | [x] |

### Verification gate

| # | Task | Status |
|---|------|--------|
| V.1 | `make build` exits 0 | [x] |
| V.2 | `make test` exits 0 | [x] |
| V.3 | `make gen` then `git add -N . && git diff --exit-code HEAD` exits 0 after the commits, checked on a `git archive` export of the branch (SC 10) | [x] |
| V.4 | `harness validate-docs` and `harness validate-pr-checklist` exit 0 after the commits | [x] |

## Design Decisions

### Fallible fields go after the literal
Keeping infallible fields in the struct literal leaves most of each generated mapper unchanged and the diff reviewable. Checked assignments in field order give the first error a stable `Entity.field` prefix.

### Errors, not an X-variant
ADR-005's `ApplyDomainX` pattern is not repeated here: no panicking `FromProtoX`. Mapping untrusted input is the case where a panic does the most damage, and a caller who wants one can wrap the call.

### `FIQLQueryParam` uses `PathUnescape`
`PathUnescape` decodes `%XX` like `Query()` does today but leaves `+` alone, so offsets such as `+07:00` survive. `%3B` still becomes `;`, the same as today; fixing that needs the parser grammar change the scope excludes.

## What Stays Unchanged

- `ToProto` / `XListToProto` signatures and output.
- `.proto` messages, field numbers, `.entdomain.lock.json`, `.entdomain.skipped.json`.
- `WithConversion` expressions and `examples/basic/domain/pbmap/user_metadata_conv.go`.
- Optional enum proto mapping (out of scope, known compile failure).
- `ParseFIQL` / `ParseFIQLExpr` / `CompileFIQL` behavior; FIQL limits.
- `template/domain.tmpl`, `generate.go`, `ent/domain.go` output.

## Implementation Order

    1. WS1 entpb regen        ← nothing in examples/basic/domain/pbmap can be tested until import stops panicking
    2. WS2 failing tests       ← proves P1-P3 on the current generator
    3. WS3 proto generator     ← one atomic change; makes 2.1 pass
    4. WS4 existing callers    ← compile fixes forced by WS3; same commit range as WS3
    5. WS5 FIQL helper         ← independent of WS3/WS4; makes 2.2 pass after 5.3
    6. WS6 docs                ← documents verified behavior
    7. Verification gate

## Notes

- Run `but pull --check` before creating the branch; ENTD-009 found GitButler basing a new branch on a stale target.
- 1.1 decision (user, 2026-11-17): generate with the local `protoc-gen-go` (v1.36.11; module requires `google.golang.org/protobuf` v1.36.12), not the repository's remote `buf.build/protocolbuffers/go` plugin, so no `.proto` file leaves the machine. Pass the local plugin as an inline `--template`; `buf.gen.yaml` stays unchanged.
- Repros for P1-P3: `/tmp/entd-repros` (ephemeral; the failing-first tests in WS2 replace them).
- ENTD-009 (PR #69) and ADR-006 (PR #70) are open; this job does not depend on either.

### Release note draft (task 6.5)

> **⚠ Breaking (generated code): proto `FromProto` mappers return errors (ENTD-010).**
> `XFromProto(p)` now returns `(*domain.X, error)` and `XListFromProto(ps)` returns `(domain.XList, error)`, in both the `pbmap` subpackage and `WithProtoHelpersInDomain` modes. Previously a malformed UUID panicked (`uuid.MustParse`), and an `UNSPECIFIED` or unknown enum value silently became the first enum value. Now both return an error naming the field (`User.external_id: …`, `User.status: …`); list errors are prefixed with `index N`, and nested-message errors propagate. Migration after `go generate`: `d, err := pbmap.UserFromProto(p); if err != nil { … }`. The exported UUID helpers `ProtoStringToUUIDPtr` and `StringSliceToUUIDSlice` also return an error, and `ProtoStringToUUID` is new.
>
> **New: `entdomain.FIQLQueryParam(rawQuery, key)`.** Read FIQL filters with it instead of `r.URL.Query().Get`: since Go 1.17 `net/url` drops any query pair containing `;` (FIQL AND), so the previously documented handler ran unfiltered for `?filter=a==1;b==2`. It keeps `;` and `+`, and reports whether the parameter was present so `?filter=` can be rejected. The README handler is updated.

## Discoveries & Decisions During Implementation

### [Implementer] All tests written before the generator change; WS1 was the real prerequisite
The committed `examples/basic/proto/entpb/*.pb.go` panicked on import, so no mapper test could run. Regenerating with the local `protoc-gen-go` v1.36.11 changed exactly one byte pair per file (`BEZC` → `BBZ@`, the length prefix of `go_package`) and kept the `protoc-gen-go v1.36.11` header, which shows the files were generated correctly and the `go_package` path was edited by hand afterwards. The WS2 tests then failed as intended: the mapper tests did not compile against the old one-value signatures, and `TestReadmeFIQLHandler` returned 2 rows (unfiltered) for `name==alice;score=gt=10` and 200 for `filter=`.

### [Implementer] ADR-004 already listed the panic as a Low finding
`docs/ADR-004-security-pipeline.md` recorded "`uuid.MustParse` panic on invalid proto input in generated helpers" in its self-audit, with no accepted-risk note. The optional-UUID helper was hardened afterwards (its "must not crash" comment), but the non-optional field path (`proto_types.go:113`) and the unique-edge path (`:449`) kept `MustParse`. Task 6.6 was added to mark the finding resolved. This also settles the question raised before the job of whether the panic was intended: it was a known, unfinished fix.

### [Implementer] Nest edges had to become fallible, and the old helpers were silent, not safe
`Ring: RingFromProto(p.Ring)` and `Tags: TagListFromProto(p.Tags)` call the mappers whose signatures change, so Nest fields join UUID and enum fields as checked assignments. `TestUserFromProtoNestedErrorPropagates` pins the resulting chain (`User.tags: index 1: Tag.users: index 0: User.external_id`). The first draft of that test only fed a valid nested message and could not fail; it was rewritten before WS3 landed. Separately, `protoStringToUUIDPtr` and `stringSliceToUUIDSlice` avoided panics by returning `nil` / `uuid.Nil`, which stores a value the caller never sent; they now return errors too (scope U1 = B).

### [Implementer] Smaller deviations
- Task 4.4: `slice_example_test.go` has no `FromProto` call, only text assertions on the `WithConversion` function name, so it needed no change; the real callers were in the two example e2e tests (4.5, 4.6).
- `trackExprImports` now keys the uuid import on any `uuid.` reference. The mapper no longer references the uuid package at all (the helpers file does), so a stricter test asserts the import is absent; an unused import would not compile.
- The `harness hook-postbuild` `make gen` fired between adding `FIQLQueryParam` and adding its `net/url` import, so one hook run failed (`undefined: url`). Harmless (gen-basic fails before writing), but edits to root `*.go` should land with their imports in one edit.
- golangci-lint v2.11.4 flagged `defer resp.Body.Close()` in the new HTTP test (errcheck); fixed with an explicit discard.


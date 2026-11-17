# Scope: Untrusted input in proto mappers and the FIQL HTTP pattern

| Field   | Value      |
|---------|------------|
| Status  | Accepted   |
| Created | 2026-11-17 |
| Author  | danhtran94 |

---

## Problem

Two paths take caller-controlled input and either crash or silently drop it.

**P1. Proto `FromProto` panics on a malformed UUID.** For a non-optional UUID field the generator sets `FromProtoExpr = "uuid.MustParse(%s)"` (`proto_types.go:113`), and for a unique edge with a UUID target `fromExpr = "uuid.MustParse(%s)"` (`proto_types.go:449`). The generated `UserFromProto` contains `ExternalID: uuid.MustParse(p.ExternalId)` (`examples/basic/domain/pbmap/user_proto_gen.go:78`). An empty or malformed string panics (A1). This contradicts the rule the repository already states for the optional path: *"protobuf payloads can carry untrusted strings and must not crash the process"* (`template/proto_helpers.tmpl:111-113`).

**P2. An unset proto enum becomes the first enum value.** The generated `<entity><Field>FromProto` switch returns `DefaultDomainConst`, the first schema value, for `UNSPECIFIED` and unknown numbers (`proto_mapper_generate.go:322-324`, `template/proto_mapper_subpkg.tmpl:25-34`). A client that omits `status` gets `status = "active"` with no error (A2).

**P3. The documented FIQL handler runs unfiltered on any expression containing `;`.** README "HTTP Handler Usage" and ADR-003 read the filter with `r.URL.Query().Get("filter")`. Since Go 1.17, `net/url` rejects `;` as a query separator and drops the pair, so `?filter=name==alice;score=gt=10` reads as `""`, the `expr != ""` guard skips filtering, and the query returns every row (A3). `+` in a time offset decodes to a space (A4). ADR-003's core constraint, *"URI-safe without percent-encoding"* (`docs/ADR-003-fiql-filter-generation.md:11`), is false for `;` with the standard library.

## Assumptions

- **A1 [VERIFIED]:** `pbmap.UserFromProto(&entpb.User{ExternalId: ""})` panics with `uuid: Parse(): invalid UUID length: 0`. Evidence: `/tmp/entd-repros/zzproto/repro_test.go` (2026-11-17), run against a locally regenerated `entpb` because the committed one panics on import.
- **A2 [VERIFIED]:** `UserFromProto` with `Status: USER_STATUS_UNSPECIFIED` returns domain `Status == "active"`. Evidence: same repro file.
- **A3 [VERIFIED]:** The README handler behind `httptest` returns 1 row for `name==alice`, 2 rows for `name==alice;score=gt=10`, and 1 row for the `%3B`-encoded form. Evidence: `/tmp/entd-repros/examples/basic/ent/zz_repro_must_test.go`; `url.ParseQuery("filter=a==1;b==2")` returns `invalid semicolon separator in query`.
- **A4 [VERIFIED]:** `url.ParseQuery` decodes `t=ge=2024-01-15T10:30:00+07:00` to `…00 07:00`; `url.PathUnescape` keeps `+` and `;`. Evidence: a Go run in this session.
- **A5 [VERIFIED]:** The optional UUID helper and the UUID slice helper already avoid panics by returning `nil` / `uuid.Nil` for malformed input (`template/proto_helpers.tmpl:111-147`). Only the two `MustParse` sites panic.
- **A6 [HYPOTHESIS]:** A grpc-go server without a recovery interceptor terminates on a handler panic. Not run here (grpc is not a dependency); the panic itself is A1.

## Decisions

### U1: what `FromProto` does with invalid input (P1, P2)

| Option | Behavior | Trade-off |
|---|---|---|
| **A. KISS** | Replace `MustParse` with a helper that returns `uuid.Nil`; map `UNSPECIFIED`/unknown enum to the zero value `""` instead of the first value | No API change; matches A5. Silent: a malformed `ExternalId` is stored as `uuid.Nil`. An enum `""` fails ent's validator at `Save`, which is loud |
| **B. Recommended** | `XFromProto(p) (*domain.X, error)` and `XListFromProto(ps) (domain.XList, error)`; malformed UUIDs (all three helper paths) and `UNSPECIFIED`/unknown values of a non-optional enum return an error naming `Entity.field` | Untrusted input is rejected instead of stored. Breaking for generated call sites: they fail to compile after `make gen`, and the fix is mechanical. Generator must split fallible fields out of the struct literal |
| **C. Alternative** | Option A plus a generated `ValidateXProto(p) error` that callers run before mapping | No API change and an explicit check, but validation is opt-in and easy to forget, which is how P1 shipped |

**Decided: B** (2026-11-17, delegated to the implementer by the user). Replacing a crash with silent `uuid.Nil` (A) trades one must-fix for another under this repository's own criteria.

### U2: how a handler reads the FIQL expression (P3)

| Option | Behavior | Trade-off |
|---|---|---|
| **A. KISS** | Docs only: clients must percent-encode `;` (`%3B`) and `+` (`%2B`); README and ADR-003 say so | No code. Drops ADR-003's "no percent-encoding" premise, and the handler still fails open when a client forgets |
| **B. Recommended** | Runtime helper `entdomain.FIQLQueryParam(rawQuery, key string) (expr string, ok bool, err error)`: splits `rawQuery` on `&` only, applies `url.PathUnescape` (keeps `+`, decodes `%XX`), and reports a present-but-empty key with `ok == true`. README handler uses it and rejects a present empty filter | Restores the premise for `;` and `+` with about 20 lines of runtime code. `%3B` still decodes to `;` (the parser grammar issue, see Out of Scope), the same as today |
| **C. Alternative** | README handler parses `r.URL.RawQuery` inline; no library API | No new API, but every user copies the parsing, and the copies drift |

**Decided: B** (2026-11-17, same).

B covers every fallible field: UUID values and IDs, non-optional enums, and Nest edges, because `Ring: RingFromProto(p.Ring)` and `Tags: TagListFromProto(p.Tags)` call the now-fallible mappers.

## Success Criteria

Written for the decided options U1 = B and U2 = B.

1. `examples/basic/domain/pbmap/user_proto_gen.go` contains no `MustParse`, and `grep -rn MustParse proto_types.go template/` returns no match.
2. In `examples/basic/domain/pbmap/proto_mapper_e2e_test.go`, `TestUserFromProtoRejectsMalformedUUID` passes: `UserFromProto(&entpb.User{ExternalId: ""})` returns a nil user and an error containing `User.external_id`.
3. `TestUserFromProtoRejectsUnspecifiedEnum` passes: `Status: USER_STATUS_UNSPECIFIED` returns an error containing `User.status`.
4. `TestUserListFromProtoReportsIndex` passes: a malformed element at index 1 returns an error containing `index 1`.
5. `TestUserProtoRoundTrip` passes: `UserFromProto(UserToProto(d))` equals `d` for a fully populated `sampleUser`.
6. `examples/basic/proto/entpb/*.pb.go` are regenerated with `buf generate`; importing `entpb` no longer panics (prerequisite for 2-5).
7. `fiql_test.go` `TestFIQLQueryParam` passes: `"filter=name==a;b==c"` → `("name==a;b==c", true, nil)`; `"filter=t=ge=2024-01-15T10:30:00+07:00"` keeps `+`; `"x=1"` → `ok == false`; `"filter="` → `("", true, nil)`; `"filter=%zz"` → error.
8. In `examples/basic/ent/fiql_http_e2e_test.go`, `TestReadmeFIQLHandler` runs the README handler as written behind `httptest` and gets 1 row for `name==alice;score=gt=10` and HTTP 400 for `filter=` (present but empty).
9. The README "HTTP Handler Usage" handler is identical to the one in criterion 8, and ADR-003 carries an amendment note naming `FIQLQueryParam`.
10. `make build`, `make test`, and `make gen` followed by `git add -N . && git diff --exit-code HEAD` exit 0.

## Out of Scope

- **Percent-decoding inside the FIQL parser** (`name==a%3Bb` meaning a literal `;`).
  - Reason: a parser grammar change; already deferred by `docs/scope-fiql-parse-compile-split.md:140`.
- **`WithConversion` expressions returning errors.**
  - Reason: user-supplied expressions are called unqualified; changing their contract is a separate API decision.
- **FIQL total size or term limits.**
  - Reason: DoS hardening, separate scope.
- **Proto field-number lock duplicates and enum wire-number stability.**
  - Reason: wire-format stability, separate scope.
- **Proto mapping of optional enums.**
  - Reason: it does not compile today (`proto_mapper_generate.go:298-302` passes `*Enum` to a value helper); a separate defect, found in the 2026-11-16 analysis.
- **Request validation frameworks (protovalidate).**
  - Reason: callers' choice; the mapper only has to stop crashing and stop inventing values.

## Risks

- **U1 = B breaks generated call sites.**
  - Mitigation: the break is a compile error at the consumer's next `make gen`, never a silent behavior change. The release note gives the one-line migration (`d, err := pbmap.UserFromProto(p)`).
- **Splitting fallible fields changes mapper structure in both modes** (`pbmap` subpackage and `WithProtoHelpersInDomain`).
  - Mitigation: `examples/sliced` and `examples/sliceddomain` cover the two modes; criterion 10 regenerates both, and their e2e tests must compile against the new signatures.
- **`FIQLQueryParam` differs from `url.Values.Get` for repeated keys.**
  - Mitigation: return the first occurrence, as `Get` does, and test it under criterion 7.

## History

| Ticket    | Title                                                      | Created    | Status   |
|-----------|------------------------------------------------------------|------------|----------|
| (initial) | Untrusted input in proto mappers and the FIQL HTTP pattern | 2026-11-17 | Accepted |

# Scope: Support UUID in FIQL queries

| Field    | Value                |
|----------|----------------------|
| Status   | Accepted             |
| Created  | 2026-04-19           |
| Author   | danhtran94           |

## Problem

UUID fields are silently dropped from generated FIQL registries.

`fieldFIQLKindFn` in `template.go:220-240` returns empty string for the UUID case with the inline comment *"UUID predicates take uuid.UUID not string — not supported via FIQL"*. The template (`template/fiql.tmpl:34`) treats empty-kind fields as "skip", so a schema author who annotates a UUID field with `entdomain.FIQL(...)` gets:

- No build error
- No warning at codegen
- The field is absent from the generated `FIQLFields` registry
- At request time, `ParseFIQL` returns `unknown field "external_id" — annotate with entdomain.FIQL(...) to enable` — the most misleading error possible, because the field *is* annotated

Concrete example: `examples/basic/ent/schema/user.go:48` defines `field.UUID("external_id", uuid.UUID{})`. There is no FIQL annotation today, but the moment one is added, the field stays invisible to `ParseFIQL`. No FIQL test exercises a UUID field — the gap is invisible.

The underlying technical reason is real: ent's UUID predicates take `uuid.UUID`, not `string`, so the existing `apply(op, val string)` interface can't dispatch directly. But UUIDs are first-class identifiers in REST/gRPC APIs (lookup by external ID is the canonical filter), so silently skipping is a usability cliff.

## Success Criteria

1. **New `FIQLUUID[P Predicate]` type** in `fiql.go`, mirroring the structural pattern of `FIQLBool` (single-type wrapper, parses string → typed value, dispatches to ent predicate functions). Supports `==` and `!=` only.

2. **Generator emits `FIQLUUID` descriptors** for ent UUID fields. `fieldFIQLKindFn` in `template.go` returns `"UUID"` for the UUID case (replacing the current `""` skip). Template `template/fiql.tmpl` handles the new kind alongside the existing String/Int/Float/Bool/Time/Enum branches.

3. **Value parsing uses `uuid.Parse`** (the `github.com/google/uuid` package — already a direct dependency per `go.mod`). Invalid UUID strings return `invalid UUID value %q: %w` wrapping the parse error — same shape as the existing int/float/time parse errors.

4. **Disallowed operators return a clear error**: `operator %q not allowed on UUID field — only == and != are supported`. Mirrors the `FIQLBool`/`FIQLEnum` shape.

5. **Round-trip example**: `examples/basic/ent/schema/user.go` `external_id` field gets an `entdomain.FIQL(...)` annotation. Generated `FIQLFields` registry includes it. A new FIQL test in `examples/basic/ent/fiql_test.go` asserts `external_id==<canonical-uuid>` returns the expected user, and `external_id==not-a-uuid` returns the parse error.

6. **`make gen` produces zero diff** after regeneration (CI gate). **`make test` passes** the new UUID test plus all existing FIQL tests unchanged.

## Out of Scope

- **Other ID types beyond `uuid.UUID`.** `xid`, `ulid`, custom string-aliased ID types, or ent's `field.Other(...)` are not addressed. Only `field.UUID` (Google's `github.com/google/uuid` type) is wired. Users on alternative ID libraries will continue to hit the silent-skip behavior; addressing that requires a separate generalization pass.

- **Range / ordering operators (`=gt=`, `=lt=`, `=ge=`, `=le=`).** UUIDs are opaque identifiers; lexicographic ordering is meaningful for v6/v7/v8 only and a footgun for v4. Restricting to `==` / `!=` matches `FIQLBool`/`FIQLEnum` precedent.

- **`=in=` / `=out=` (set membership).** Not present in the existing FIQL grammar for any type. Adding set membership is a separate cross-cutting feature touching the parser (`fiql.go:434-462`) and every typed field — not a UUID-specific concern.

- **UUID as edge / foreign-key filter.** This scope only covers UUID-typed scalar fields. Filtering across edges by the related row's UUID PK is unchanged and remains out of FIQL's scope generally.

- **Non-canonical UUID input forms.** Parsing accepts what `uuid.Parse` accepts (canonical 36-char hyphenated, plus the loose forms that `uuid.Parse` already tolerates — braced, urn:, hex). No additional normalization layer.

- **Generator-time validation that ent's UUID type is actually `github.com/google/uuid.UUID`.** ent supports custom UUID-shaped types via `GoType(...)`. We assume the standard `uuid.UUID`; non-standard underlying types fall back to today's silent-skip behavior. Detection is left to a future scope.

## Risks

- **Breaking existing users with annotated UUID fields.** Today, an `entdomain.FIQL(...)` annotation on a UUID field is a no-op (silent skip). After this change, the same annotation produces a working filter — but any caller currently relying on the field being absent (e.g. allowlist-style validation that assumed UUID would never appear) sees new behavior. *Mitigation:* the silent-skip is itself a defect; nobody should be depending on it. Note in the release commit message and the README's FIQL section.

- **`uuid.Parse` accepting more than canonical 36-char form.** `uuid.Parse` tolerates braced (`{...}`), urn (`urn:uuid:...`), and 32-char hex without hyphens. APIs that want to enforce a single canonical form gain that strictness only by validating upstream. *Mitigation:* document the accepted forms in the `FIQLUUID` Godoc; note that strict-canonical enforcement is an upstream concern.

- **ent UUID fields with custom `GoType(...)`.** A schema using `field.UUID("id", customType{})` where `customType` is not `uuid.UUID` will still fall through to silent-skip (out of scope). The generator does not detect this and won't warn. *Mitigation:* accepted limitation — a follow-up scope can add codegen-time detection and a clear "unsupported UUID GoType" diagnostic. Track as a known gap in the job's Discoveries section if encountered.

- **Living list** — update during implementation as new failure modes surface.

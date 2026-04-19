# Scope: codegen field-kind consolidation

| Field    | Value                |
|----------|----------------------|
| Status   | Accepted             |
| Created  | 2026-04-19           |
| Author   | danhtran94           |

## Problem

Three independent functions duplicate the ent field-type switch:

- `template.go:fieldFIQLKindFn` — emits `"String"`/`"Int"`/`"UUID"` etc. for the FIQL template
- `proto_types.go:resolveEntFieldProtoSpec` — emits proto type + import
- `generate.go:fieldToDomainTypeWithEnum` — emits Go domain type + import

Adding a new ent field type means hunting all three. The recent UUID/GoType gate (ENTD-001) lives only in `template.go:fieldFIQLKindFn`; the proto and domain generators have no equivalent guard, so a custom-GoType UUID field would still produce a working domain struct and a working proto field — only FIQL silently skips it. This silent split is invisible until a user lands a custom GoType.

The FIQL operator strings have the same shape of duplication. `fiql.go` defines:

    const (
        EQ        FIQLOp = "=="
        ...
        In        FIQLOp = "=in="
        NotIn     FIQLOp = "=out="
    )

But `template/fiql.tmpl` hard-codes the literals in 12+ branches:

    {{- if eq $op "=in=" }} In: ...
    {{- if eq $op "=out=" }} NotIn: ...

ENTD-002 had to touch both the constant and every template branch. Adding a new operator (or renaming one) means edits in two places that have no compile-time link.

A second-order consequence sits in `fiql.go` itself. The six per-type `apply` methods (`FIQLString`, `Int`, `Float`, `Time`, `Bool`, `UUID`) follow nearly identical control flow — `check-op → parse → check-nil-fn → call`. ~350 lines of repetition, with inconsistent op-check ordering: `Bool`/`Int`/`Float`/`UUID` check op first (most fixed during ENTD-002 review), but `String` and `Time` still parse-then-check, so a "wrong op" error is reachable only when the value happens to parse. Three CodeRabbit cycles in the last two PRs went into this exact inconsistency.

A third silent-failure mode lives in `proto_types.go`. `ProtoFieldSpec.IsExcluded = true` is returned for unsupported fields with no reason string. A typo'd annotation, an unsupported GoType, or a missed type case all collapse to the same silent skip — diagnosable only by reading proto output side-by-side with the schema.

## Success Criteria

1. **Single canonical kind enum + resolver** in a new `kinds.go` (or in `template.go` if cleaner): `type FieldKind int` with values `KindString`, `KindInt`, `KindFloat`, `KindBool`, `KindTime`, `KindEnum`, `KindUUID`, `KindUnsupported`. One function `resolveFieldKind(f *gen.Field) (FieldKind, string)` returning the kind and a reason string when `KindUnsupported`. `template.go:fieldFIQLKindFn`, `proto_types.go`, and `generate.go:fieldToDomainTypeWithEnum` all dispatch on the result instead of switching on `f.Type.Type` independently.

2. **Operator names addressable from templates by symbolic name.** A new template helper `isOp` lets the template write `{{ if isOp "In" $op }}` instead of `{{ if eq $op "=in=" }}`. The helper consults a Go-defined registry keyed by the operator constant identifier (`"EQ"` → `EQ`, `"In"` → `In`). Adding a new operator means one line in the registry; templates reference by name.

3. **`applyListTyped` helper** in `fiql.go` — *scoped down from the originally-planned full `applyTyped` unification*. The genuinely-identical 25-line In/NotIn block in `FIQLInt`/`Float`/`UUID` reduces to one helper call. The full unified `apply` dispatch was rejected mid-implementation because preserving the existing per-type error-message wording (e.g. parse-noun "integer" vs field-noun "int") ballooned the generic signature past readability — see Discoveries in the job doc. `FIQLString`/`Time`/`Bool`/`Enum` keep their existing apply shape; the parse-then-check ordering inconsistency in String/Time is documented as known but not fixed in this scope.

4. **`ProtoFieldSpec` carries an `ExcludedReason string`** field, populated whenever `IsExcluded = true`. The proto generator collects skipped fields per message and emits a final summary in `.entdomain.skipped.json` listing every excluded field with its reason. Optional: also `fmt.Fprintln(os.Stderr, ...)` at codegen time.

5. **`make gen` zero-diff after the refactor.** The end-to-end output of `make gen` on `examples/basic` and `examples/custom` is byte-identical before and after the refactor. This is the load-bearing canary — the consolidation must be observably-equivalent.

6. **Test surface unchanged or expanded.** All existing `*_test.go` files pass without modification. New tests: `TestResolveFieldKind` covering each ent field type + the GoType gate for UUID; `TestOpRegistryCovered` + `TestIsOpFn` guarding the op-name registry against drift; expanded `TestResolveEntFieldProtoSpec_JSONField` subtests asserting that unsupported GoType / bytes fields populate `ExcludedReason`. (Originally planned `TestApplyTyped` was scoped down to the existing per-type FIQL apply tests, which already exercise `applyListTyped` end-to-end via the `=in=`/`=out=` cases.)

7. **No public API breakage.** `FIQLOp`, `FIQLField`, `FIQLFields`, `FIQLString`/`Int`/`Float`/`Time`/`Bool`/`Enum`/`UUID`, `ParseFIQL` all keep their current signatures. The `applyListTyped` helper is unexported. End users (and existing generated code) need no changes.

## Out of Scope

- **Relocating `FIQLOp`/`FIQLField`/`FIQLFields` into an internal package.** Touching the public API surface is a separate scope — this job's contract is "consolidate without breaking." The unused-by-end-users observation from the audit becomes its own job.

- **Adding new ent field types** (e.g. `field.Bytes`, `field.JSON` with typed payload as a first-class FIQL kind). The unified resolver makes this easier, but adding kinds is a follow-up.

- **Whitespace trimming in `parseInListValue`** (audit minor #5). Five-minute fix that's better landed independently with its own one-line PR — bundling here muddies the diff.

- **`helpersInDomain` bool entanglement** in `proto_mapper_generate.go` (audit minor #12). Real refactor candidate but orthogonal to the kind/op consolidation.

- **Replacing `string.Contains` test assertions** with golden files or AST comparison (audit major #7). Worth doing, but separately — bundling test infra changes with a load-bearing refactor doubles the review surface.

- **Logging the excluded-field summary to stderr at codegen time.** Implementation may include this as a one-liner, but the contract is the lock-file/summary-file artifact, not the stderr line.

- **Renaming any existing operator constants or kinds.** The names `EQ`, `In`, `String`, etc. stay as-is. The op-name registry uses the *Go identifier* string (`"In"`), not a renamed alias.

## Risks

- **Originally-planned generic `applyTyped[T]` does not compose cleanly with the parser interface — scoped down during implementation.** Threading a second type parameter through `applyTyped[T any, P Predicate]` is workable in current Go, but preserving the existing per-type error-message wording (parse-noun "integer" vs field-noun "int", time's RFC3339 hint, etc.) ballooned the generic signature past readability. *Resolution as shipped:* `applyListTyped` extracts only the In/NotIn list-handling block (genuinely identical across `FIQLInt`/`Float`/`UUID`); `FIQLString`/`Time`/`Bool`/`Enum` keep their existing apply shape. Captured in the job's Discoveries.

- **`make gen` zero-diff is the only check that the consolidation is observably-equivalent.** A typo in a kind enum or an off-by-one in the op-name registry could change generated output without breaking compilation. *Mitigation:* run `make gen` after every workstream task, not just at the end. Treat any unexplained diff as a hard regression — investigate before continuing.

- **Template helper `isOp` adds a runtime lookup to every template render.** Negligible at codegen time but easy to overlook for completeness if a generator someday runs templates in a hot path. *Mitigation:* the registry is a package-level `var opByName = map[string]FIQLOp{...}` literal evaluated at package init; lookup is O(1). Document as "codegen-only, not for hot paths."

- **`ExcludedReason` is technically a public-API addition to `ProtoFieldSpec`.** Adding a field to an exported struct is backward-compatible at the language level, but downstream consumers using `ProtoFieldSpec{...}` literally (rather than `ProtoFieldSpec{IsExcluded: true, ExcludedReason: "..."}`) keep working. *Mitigation:* document the new field; existing zero-value `ExcludedReason: ""` for non-excluded fields is harmless.

- **The op-name registry could drift from the FIQLOp constants if a constant is added without registry update.** *Mitigation:* `TestOpRegistryCovered` iterates a `wantOps` slice and asserts every constant appears in the registry; an inverse loop also catches stale entries with no matching constant. Both directions guarded.

- **`applyListTyped` falling through to `NotIn` for any non-`In` op was a latent caller-bug hazard.** *Mitigation as shipped:* an explicit `switch op { case In, NotIn: default: error }` guards the entry. Future callers cannot silently route the wrong predicate.

- **Living list** — update during implementation as new failure modes surface.

# ADR-005: Transformer Runtime Context (ctx + error + Sibling Access)

## Status

Implemented

## Context

entdomain generates per-field transformer hooks that let users customize how ent builders are populated during `ApplyDomain*` calls. The current signature is:

```go
ent.HealthLogTransformer = &ent.HealthLogDomainTransformer{
    SetValuePlainOnCreate: func(c *ent.HealthLogCreate, val float64) {
        c.SetValue(encode(strconv.FormatFloat(val, 'f', -1, 64)))
    },
}
```

Transformers are registered globally at process startup. The design was built on the assumption that transformers are **pure, synchronous, stateless encoders** — e.g. formatting, trimming, unit conversion. That assumption breaks down for real cases:

- **Field-level encryption** (KMS-backed) where the subkey is derived from `TenantID` on the same record, and the encrypt call is a network request
- **Deterministic encryption** where the record ID is used as Additional Authenticated Data (AAD)
- **Per-record hash salting** where the salt is another field on the same row
- **Signing** with a key selected by record metadata, often via a remote signing service
- **Tokenization** against an external vault

The current `func(c *Builder, val T)` signature gives the transformer access to only the value being set. It cannot see sibling fields on the record. It cannot receive a `context.Context` to propagate timeouts, cancellation, or tracing spans into downstream I/O. It cannot return an error — forcing users to either panic or silently swallow failures.

A downstream user reported this limit concretely: they needed `encode(key, val)` where `key` was derived from record metadata and fetched from KMS. The global transformer could not reach either.

### ent ecosystem convention

entdomain is a code-generation extension to [ent](https://entgo.io). ent's own extensibility surfaces use `ctx context.Context` + `error` return consistently:

| ent API | Signature |
|---|---|
| Mutation hooks ([docs](https://entgo.io/docs/hooks)) | `func(ctx context.Context, m ent.Mutation) (ent.Value, error)` |
| Privacy policy rules ([docs](https://entgo.io/docs/privacy)) | `EvalMutation(ctx context.Context, m Mutation) error` |
| Query interceptors | `Intercept(ctx context.Context, q Query) (ent.Value, error)` |
| Every terminal builder method | `.Save(ctx)`, `.All(ctx)`, `.First(ctx)`, … |

Users already have `ctx` in scope at every entdomain call site because they are already invoking `client.X.Create().Save(ctx)` in the surrounding code. A transformer layer that breaks this convention forces users to either pass `context.Background()`, smuggle ctx through `context.Value` (discouraged in Go idiom), or move encoding out of the mapper entirely — which undoes the clean pipeline.

### Related decision: virtual fields stay pure

Virtual fields on the domain struct (populated by `ToDomain()` via user-supplied transformer functions) are kept as pure synchronous derivations — no ctx, no error, no I/O. They are read-only projections: `ApplyDomain*` skips them entirely because there is no corresponding storage on the ent side.

This ADR applies **only to the ent-side transformers** that customize how fields are written to builders during `ApplyDomain*`. The virtual-field contract is not revisited.

---

## Assumptions

- **A1 [EXTERNAL FACT]:** `context.Context` is the idiomatic Go mechanism for propagating request-scoped state and cancellation through call chains. Evidence: [Go standard-library docs](https://pkg.go.dev/context).
- **A2 [VERIFIED]:** ent's builder methods (`Create`, `Update`) do not themselves accept `context.Context` until `Save(ctx)` is called. Evidence: `examples/basic/ent/user_create.go` — the `*ent.UserCreate` methods are pure mutations; only `Save` takes ctx.
- **A3 [HYPOTHESIS]:** Transformer authors will need access to runtime state (KMS handles, signers, request metadata) at field-population time. Verification deferred — the use case was raised by an early consumer (HealthLog encryption); the new signature unblocks it without forcing a separate use-case layer.

## Options
<!-- Subsections below were previously titled "Options Considered"; renamed to "Options" for discipline schema compliance. -->


### 1. Status quo — pure transformers, enrichment elsewhere

Keep the current signature. Force all runtime-state-aware encoding to live outside the mapper in a separate use-case step.

- **Pro:** Mapper stays pure; separation of concerns is crisp in theory.
- **Con:** Field-level encryption needs to encode the value *before* it reaches the builder. Doing it "after mapping" means mutating the builder post-hoc, which undoes the clean pipeline. Users will work around the limit, often poorly.

### 2. Stringly-typed per-call encoder option

Add `entdomain.WithFieldEncoder(fieldName string, fn func(any) (any, error))` to `ApplyOption`.

- **Pro:** No breaking change to existing signatures.
- **Con:** Rejected for losing type safety — field names become strings, values become `any`, typos become runtime panics. Inconsistent with the codegen's fully-typed API elsewhere.

### 3. Typed per-call transformer override

Let callers pass the codegen-produced `*ent.XDomainTransformer` struct at the call site via a generated `WithXTransformer(t *ent.XDomainTransformer)` option. Per-call override wins; falls back to global field-by-field.

- **Pro:** Reuses the existing typed struct. Full type safety. Ctx captured in closure at the call site.
- **Con:** Only useful when encoding depends on caller-scoped ctx that cannot be reached from a global closure — a subset of the full ctx case. Still lacks sibling access. Adds a second registration path (global vs. per-call) requiring documentation.

### 4. Sibling field access, no ctx

Change the signature to receive the domain struct:

```go
SetValuePlainOnCreate: func(c *ent.HealthLogCreate, d *domain.HealthLog, val float64) { ... }
```

- **Pro:** Unlocks per-record key derivation and AAD binding. Type-safe. Minimal signature change.
- **Con:** Still no ctx. KMS calls, remote signing, traced encryption all require ctx — this option forces `context.Background()` or ctx smuggling. Breaks ent's convention. Needs a follow-up ADR the moment anyone integrates a real KMS.

### 5. Full ctx + error + sibling access

Change the signature to match ent convention:

```go
SetValuePlainOnCreate: func(ctx context.Context, c *ent.HealthLogCreate, d *domain.HealthLog, val float64) error { ... }
```

Lift `ApplyDomain*` accordingly:

```go
ApplyDomainCreate(ctx context.Context, client *ent.Client, opts ...ApplyOption) (*ent.HealthLogCreate, error)
```

- **Pro:** Matches ent's ctx-everywhere convention exactly. Unlocks KMS/signing/tokenization without workaround. Tracing, cancellation, and deadlines propagate naturally. Errors surface at the boundary where they occur. Sibling access included.
- **Con:** Larger breaking change — every `ApplyDomain*` call site takes ctx; every transformer handles ctx and error even when the logic is pure (one line: `return nil`). One-way door — hard to remove later.

---

## Options Comparison

| Driver | 1. Status quo | 2. Stringly encoder | 3. Typed override | 4. Sibling access | 5. Full ctx + error (chosen) |
|---|---|---|---|---|---|
| Runtime state access | No | Indirect | Indirect | No | Yes (via ctx) |
| Sibling field access | No | No | No | Yes | Yes |
| Error reporting | Panic-only | Panic-only | Panic-only | Panic-only | Returned error |
| Mid-apply cancellation | No | No | No | No | Yes (via ctx) |
| API change scope | None | Per-call | Per-call | Per-field | All ApplyDomain methods |

## Decision

**Adopt option 5.** Transformer signatures become:

```go
func(ctx context.Context, c *ent.XCreate, d *domain.X, val T) error
```

`ApplyDomain*` methods lift to accept `ctx` as the first argument and return an error:

```go
func (d *domain.HealthLog) ApplyDomainCreate(
    ctx context.Context,
    client *ent.Client,
    opts ...entdomain.ApplyOption,
) (*ent.HealthLogCreate, error)
```

Bootstrap DI via closure capture remains the idiomatic way to wire infrastructure handles (KMS, signer, cache):

```go
// Schema for HealthLog (abridged):
//   field.Bytes("value_enc"),                              // ent column: ciphertext
//   entdomain.VirtualField("value", entdomain.Float64),    // virtual: plaintext
// Codegen emits paired setter hooks: SetValueOnCreate and SetValueOnUpdate
// (the OnUpdate hook operates on *HealthLogUpdateOne — see the naming note
// in the Migration Path section).
func registerTransformers(kms KMS) {
    ent.HealthLogTransformer = &ent.HealthLogDomainTransformer{
        SetValueOnCreate: func(ctx context.Context, c *ent.HealthLogCreate, d *domain.HealthLog, val float64) error {
            subkey, err := kms.DeriveKey(ctx, d.TenantID)
            if err != nil {
                return fmt.Errorf("derive key: %w", err)
            }
            ciphertext, err := kms.Encrypt(ctx, subkey, []byte(strconv.FormatFloat(val, 'f', -1, 64)))
            if err != nil {
                return fmt.Errorf("encrypt value: %w", err)
            }
            c.SetValueEnc(ciphertext)
            return nil
        },
        SetValueOnUpdate: func(ctx context.Context, u *ent.HealthLogUpdateOne, d *domain.HealthLog, val float64) error {
            subkey, err := kms.DeriveKey(ctx, d.TenantID)
            if err != nil {
                return fmt.Errorf("derive key: %w", err)
            }
            ciphertext, err := kms.Encrypt(ctx, subkey, []byte(strconv.FormatFloat(val, 'f', -1, 64)))
            if err != nil {
                return fmt.Errorf("encrypt value: %w", err)
            }
            u.SetValueEnc(ciphertext)
            return nil
        },
    }
}
```

### Why option 5 beats option 4

My earlier draft picked option 4 on the theory that "transformers are pure, adding ctx is API noise." That reasoning fails in the exact case that motivated this ADR. Field-level encryption via KMS is network I/O. An encoding layer that claims to support encryption but cannot carry a `context.Context` forces users into anti-patterns: `context.Background()` at best, `context.Value` smuggling at worst. Both undermine the clean-architecture posture the rest of entdomain holds.

The "API noise" cost of ctx + error is real — pure transformers now carry a ctx they ignore and return `nil`. But this is the same cost every ent hook and privacy rule already pays. Consistency with the host ecosystem is worth more than saving three tokens per transformer.

### Why options 2 and 3 are rejected

Option 2 collapses type safety. Option 3 addresses only the per-call-override subset of what option 5 provides, and introduces a parallel registration path that must be documented, tested, and maintained forever. If option 5 is the right convention, option 3 is an incomplete shortcut.

### Why option 1 is rejected

It is tempting to say "transformers should be pure, put encryption in the use case layer." This works for in-memory projections but not for builder-stage encoding. Once the builder is sealed and `.Save` is called, the ciphertext must already be in place. Deferring to the use case means either (a) mutating the builder from outside, which is awkward and brittle, or (b) decrypting/re-encrypting around the mapping boundary, which is both wasteful and misses the design intent. The mapper is the correct seam for field-level encryption.

### Complementary: fluent-chaining `X`-variants

The `(builder, error)` return breaks the fluent-chain idiom ent users expect (`Create().ApplyDomain(d).Save(ctx)`). ent itself solves this with paired methods: `Save` / `SaveX`, `First` / `FirstX`, `Only` / `OnlyX` — the `X` suffix opts into panic-on-error, restoring chainability for tests and scripts while keeping the safe error-returning form as the default.

entdomain mirrors the convention exactly. Every fallible codegen-emitted method ships both forms:

| Error-returning (primary, request-safe) | Panicking wrapper (opt-in) |
|---|---|
| `*XCreate.ApplyDomain` | `ApplyDomainX` |
| `*XUpdateOne.ApplyDomain` | `ApplyDomainX` |
| `*XUpdate.ApplyDomain` | `ApplyDomainX` |
| `*XUpsertOne.ApplyDomain` | `ApplyDomainX` |
| `*XUpsertBulk.ApplyDomain` | `ApplyDomainX` |
| `*XClient.CreateBulkDomain` | `CreateBulkDomainX` |
| `*XClient.UpdateBulkDomain` | `UpdateBulkDomainX` |

The X variant is a 4-line wrapper: delegate, panic on non-nil err. It does **not** replace the error-returning form — it complements it. Transformer errors are typically I/O (KMS timeouts, signer unavailable, network partition); defaulting to panic would convert recoverable failures into goroutine crashes. The suffix makes the choice explicit at every call site.

This was considered and rejected as an alternative to `(builder, error)` — panic-by-default violates Go's convention that reserves panic for programmer errors. The X-suffix pattern is strictly additive and aligns with ent's visual grammar.

### Complementary helper: `entdomain.WithDomain` / `DomainFrom`

Typed transformers address mapper-stage encoding. A narrower class of problems — outbox event emission, cross-cutting audit logging, tenant-isolation checks that read virtual-field values — must run at ent-hook time rather than ApplyDomain time. Hooks receive `*ent.XMutation`, not the originating `*domain.X`, so virtual fields are invisible to hook bodies by default.

To serve this case without compromising the typed-transformer default path, entdomain ships a minimal pair of generic context helpers:

```go
func WithDomain[T any](ctx context.Context, d T) context.Context
func DomainFrom[T any](ctx context.Context) (T, bool)
```

- **Opt-in** — callers invoke `WithDomain` at the repository boundary before `Save`. The library does **not** auto-stash inside `ApplyDomain`; the ctx chain stays visible to the caller.
- **Type-scoped keys** — a type-parameterized empty struct is used as the context key, so `WithDomain(ctx, user)` and `WithDomain(ctx, post)` coexist without collision.
- **Absent handling is explicit** — `DomainFrom` returns `(T, bool)`; hook authors must handle `ok == false` for bare `.Save` paths that skipped `WithDomain`.

This is an **escape hatch**, not a second-class encoding path. Documentation positions it as "Advanced: ent hook integration" and explicitly notes that `context.Value` is officially reserved for request-scoped data — the smell is localized to call sites that opt in.

The rejected alternative was to make `ApplyDomain` automatically stash `d` into a derived ctx internally. This was rejected because `ApplyDomain` returns a builder, not a ctx — an internal stash would be invisible to callers and make hook behavior depend on whether `ApplyDomain` was called, a hidden coupling. Requiring the caller to write `ctx = entdomain.WithDomain(ctx, d)` keeps the opt-in explicit.

---

## Consequences

**Good:**

- Matches ent's ctx + error convention exactly. No cognitive switch for users who already know ent hooks and privacy rules.
- KMS, remote signing, tokenization, and any other I/O-based encoding work cleanly without workaround.
- Tracing spans propagate through `ApplyDomain*` into transformer bodies.
- Cancellation and deadlines are honored per-request.
- Errors surface at the encoding boundary where they occur, not deferred to `.Save` with misleading stack traces.
- Sibling field access is included — per-record key derivation and AAD binding work without additional API changes.
- Pure transformers still compose: accept ctx, return nil. Straightforward.

**Bad:**

- Breaking change for **every** `ApplyDomain*` call site in downstream code. Users must add `ctx` as the first argument and handle the new error return.
- Breaking change for **every** registered transformer. Signatures must be updated to accept ctx and return error.
- Pure transformers pay a small tax — returning `nil` and ignoring ctx. Unavoidable cost of convention alignment.
- `*domain.X` cannot be Go-enforced read-only. A buggy transformer that mutates the passed struct has unspecified behavior. This behavior is documented as part of the transformer contract: mutating the passed `*domain.X` from within a transformer is unsupported, and the framework does not make a defensive copy of the struct before invoking hooks.
- Siblings are read in the state the caller hands to `ApplyDomain*`. If a caller omits `TenantID` and a transformer reads it, the transformer sees the zero value. Documented as the contract.

---

## Migration Path

### Codegen changes

1. Update generated `*DomainTransformer` struct field types. For each virtual field `VF`, the codegen emits exactly two setter variants (plus the pure `Get{VF}` projection):

   ```go
   Set{VF}OnCreate func(ctx context.Context, c *ent.XCreate,    d *domain.X, val T) error
   Set{VF}OnUpdate func(ctx context.Context, u *ent.XUpdateOne, d *domain.X, val T) error
   ```

   Note the asymmetric naming: `Set{VF}OnUpdate` operates on `*XUpdateOne`, not `*XUpdate`. There is no `Set{VF}OnUpdateOne` hook — the `OnUpdate` name is preserved from the prior design for backward compatibility with existing user code. Only `*XUpdateOne` is supported as a hook receiver because that's the only update path where identity of a single record is guaranteed.

2. Update generated `ApplyDomain*` methods to:
   - Accept `ctx context.Context` as the first parameter
   - Invoke transformers with `(ctx, builder, d, val)` and check the returned error
   - Propagate any transformer error up — short-circuit the remaining field applications
   - Return one of `(*ent.XCreate, error)`, `(*ent.XUpdateOne, error)`, `(*ent.XUpdate, error)`, `(*ent.XUpsertOne, error)`, or `(*ent.XUpsertBulk, error)` as appropriate to the receiver. Note: `Update`, `UpsertOne`, and `UpsertBulk` paths never invoke transformers — their error return is always nil and accepted only for API symmetry.
3. Update any internal call sites in generated helper code (upsert, bulk) to thread ctx through.
4. Emit a CHANGELOG entry flagging the breaking signature change with before/after examples.

### User migration

One-time, compile-time-enforced:

1. `go generate ./...`
2. Update every `ApplyDomain*` call to pass `ctx` and handle the new error:

   ```go
   // before
   create := d.ApplyDomainCreate(client)
   _, err := create.Save(ctx)

   // after
   create, err := d.ApplyDomainCreate(ctx, client)
   if err != nil { return err }
   _, err = create.Save(ctx)
   ```

3. Update every registered transformer in `registerTransformers()` to accept ctx and domain struct, and return error. Pure transformers append `return nil` and ignore ctx.

Go's compiler enforces completeness — nothing can be silently missed.

### No runtime migration

The change is entirely compile-time. No data migration, no schema change, no coordinated deploy required.

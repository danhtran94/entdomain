# entdomain

[![CI](https://github.com/danhtran94/entdomain/actions/workflows/ci.yml/badge.svg)](https://github.com/danhtran94/entdomain/actions/workflows/ci.yml)
[![CodeRabbit Pull Request Reviews](https://img.shields.io/coderabbit/prs/github/danhtran94/entdomain?utm_source=oss&utm_medium=github&utm_campaign=danhtran94%2Fentdomain&labelColor=171717&color=FF570A&link=https%3A%2F%2Fcoderabbit.ai&label=CodeRabbit+Reviews)](https://coderabbit.ai)
[![Security](https://github.com/danhtran94/entdomain/actions/workflows/security.yml/badge.svg)](https://github.com/danhtran94/entdomain/actions/workflows/security.yml)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/danhtran94/entdomain/badge)](https://scorecard.dev/viewer/?uri=github.com/danhtran94/entdomain)
[![Go Reference](https://pkg.go.dev/badge/github.com/danhtran94/entdomain.svg)](https://pkg.go.dev/github.com/danhtran94/entdomain)
[![Go Report Card](https://goreportcard.com/badge/github.com/danhtran94/entdomain)](https://goreportcard.com/report/github.com/danhtran94/entdomain)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

An [ent](https://entgo.io/ent) extension that generates a pure Go domain layer from your ent schema — with zero ORM dependency in the domain package.

## Overview

When using ent in a clean architecture project, the generated types (`ent.User`, `ent.UserCreate`, etc.) carry DB-layer concerns and cannot be used directly as domain entities. `entdomain` solves this by generating:

- **`internal/domain/{entity}.go`** — Pure Go structs with no ent imports
- **`ent/domain.go`** — `ToDomain()` and `ApplyDomain()` mapping methods on ent types

The domain package stays in sync with your ent schema automatically — no manual drift.

## Installation

```sh
go get github.com/danhtran94/entdomain
```

## Setup

Register the extension in your `ent/entc.go`:

```go
//go:build ignore

package main

import (
    "log"

    "entgo.io/ent/entc"
    "entgo.io/ent/entc/gen"
    "github.com/danhtran94/entdomain"
)

func main() {
    ex, err := entdomain.NewExtension(
        entdomain.WithPackagePath("internal/domain"), // output dir (relative to module root)
        entdomain.WithPackageName("domain"),           // generated package name

        // Disable bulk generation for specific entities:
        entdomain.WithNoBulk("Post", "Order"),

        // Or disable bulk generation for all entities:
        // entdomain.WithNoBulk(),
    )
    if err != nil {
        log.Fatalf("creating entdomain extension: %v", err)
    }
    if err := entc.Generate("./schema",
        &gen.Config{
            // Optional: enable ent upsert support — entdomain auto-detects this
            // and generates ApplyDomain on *EntityUpsertOne / *EntityUpsertBulk.
            Features: []gen.Feature{gen.FeatureUpsert},
        },
        entc.Extensions(ex),
    ); err != nil {
        log.Fatalf("running ent codegen: %v", err)
    }
}
```

### Custom Layout

`WithPackagePath` and `WithProtoDir` are resolved relative to the **module root** (the directory containing `go.mod`), not relative to the `ent` directory. This means you can place `ent` anywhere in your project tree:

```
myproject/          ← go.mod here (module root)
├── repo/
│   ├── schema/
│   └── ent/        ← ent output
└── internal/
    └── domain/     ← WithPackagePath("internal/domain") resolves here ✓
```

```go
entdomain.WithPackagePath("internal/domain"),  // relative to go.mod, not to ent dir
```

**One caveat**: when the schema directory is outside the `ent` directory, ent derives the generated package name from the schema's parent directory rather than from `Target`. Set `gen.Config.Package` explicitly to get the correct import path:

```go
if err := entc.Generate("../schema",
    &gen.Config{
        Target:  ".",
        Package: "github.com/myorg/myproject/repo/ent", // required when schema is outside ent dir
    },
    entc.Extensions(ex),
); err != nil { ... }
```

See [`examples/custom/`](examples/custom/) for a working example of this layout.

## Schema Annotations

Opt in per entity and per edge. Entities without `entdomain.Entity()` are skipped entirely.

### Entity

```go
func (User) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entdomain.Entity(), // basic — scalar fields only

        // with virtual fields:
        entdomain.Entity(
            entdomain.VirtualField("full_name", entdomain.String),
            entdomain.VirtualField("is_premium", entdomain.Bool),
            entdomain.VirtualField("metadata", entdomain.GoType("map[string]any")),
        ),
    }
}
```

### Edges

```go
func (User) Edges() []ent.Edge {
    return []ent.Edge{
        edge.To("posts", Post.Type).
            Annotations(
                entdomain.Edge(entdomain.IDs()),              // → PostIDs []int
                // entdomain.Edge(entdomain.Nest()),          // → Posts []Post
                // entdomain.Edge(entdomain.IDs(), entdomain.Nest()), // → both
            ),

        edge.To("profile", Profile.Type).Unique().
            Annotations(
                entdomain.Edge(entdomain.IDs()),              // → ProfileID int
            ),
    }
}
```

| Annotation | Domain field | `ToDomain()` | `ApplyDomain` (create/update) |
|---|---|---|---|
| `IDs()` | `PostIDs []int` | from `Edges.Posts` | `AddPostIDs` (if len > 0) / replace by default |
| `Nest()` | `Posts []Post` | from `Edges.Posts` | skipped |
| `IDs(), Nest()` | both | from `Edges.Posts` | `AddPostIDs` only (if len > 0) |

### Virtual Fields

Virtual fields appear in the domain struct but have no corresponding ent schema field. They are set to their zero value by `ToDomain()` — the caller (or a Transformer) is responsible for hydrating them.

```go
entdomain.VirtualField("full_name", entdomain.String)          // → string
entdomain.VirtualField("is_premium", entdomain.Bool)           // → bool
entdomain.VirtualField("count",     entdomain.Int)             // → int
entdomain.VirtualField("ratio",     entdomain.Float64)         // → float64
entdomain.VirtualField("amount",   entdomain.GoType("Money"))                                               // → Money
entdomain.VirtualField("tags",     entdomain.GoType("[]string"))                                            // → []string
entdomain.VirtualField("metadata", entdomain.GoType("map[string]any"))                                      // → map[string]any
entdomain.VirtualField("price",    entdomain.GoType("Decimal", "github.com/shopspring/decimal"))             // → decimal.Decimal
entdomain.VirtualField("price2",   entdomain.GoType("*Decimal", "github.com/shopspring/decimal"))            // → *decimal.Decimal
entdomain.VirtualField("ext_id",   entdomain.GoType("UUID", "github.com/google/uuid"))                      // → uuid.UUID
entdomain.VirtualField("opt_ref",  entdomain.GoType("*Money"))                                              // → *Money
```

## Generated Output

### Domain struct (`internal/domain/user.go`)

No ent imports. Optional fields become pointers. Enum types are re-declared with the entity name as prefix to avoid cross-entity collisions.

```go
package domain

import "time"

type UserStatus string

const (
    UserStatusActive   UserStatus = "active"
    UserStatusInactive UserStatus = "inactive"
)

type User struct {
    ID          int
    Name        string
    Bio         *string        // optional → pointer
    Status      UserStatus
    CreatedAt   time.Time
    PostIDs     []int          // IDs edge
    Posts       PostList       // Nest edge (plural)
    PinnedPost  *Post          // Nest edge (singular) → pointer
    FullName    string         // virtual field
    IsPremium   bool           // virtual field
    Metadata    map[string]any // virtual field
}

// UserList is generated unless WithNoBulk is set for the entity.
type UserList []*User
```

### Mapping methods (`ent/domain.go`)

```go
// Read: ent → domain
func (e *User) ToDomain() *domain.User

// Create: domain → ent builder
func (c *UserCreate) ApplyDomain(ctx context.Context, d *domain.User, opts ...entdomain.ApplyOption) (*UserCreate, error)

// Update by ID: domain → ent builder
func (u *UserUpdateOne) ApplyDomain(ctx context.Context, d *domain.User, opts ...entdomain.ApplyOption) (*UserUpdateOne, error)

// Update by WHERE condition: domain → ent builder, chain .Where(...) after
func (u *UserUpdate) ApplyDomain(ctx context.Context, d *domain.User, opts ...entdomain.ApplyOption) (*UserUpdate, error)

// Upsert (generated only when gen.FeatureUpsert is enabled in gen.Config)
func (u *UserUpsertOne) ApplyDomain(ctx context.Context, d *domain.User, opts ...entdomain.ApplyOption) (*UserUpsertOne, error)
func (u *UserUpsertBulk) ApplyDomain(ctx context.Context, d *domain.User, opts ...entdomain.ApplyOption) (*UserUpsertBulk, error) // absent when NoBulk

// Every ApplyDomain* method has a panicking X-variant that mirrors ent's
// SaveX / FirstX / OnlyX convention. Use in tests and scripts for fluent
// chaining; prefer the error-returning form on request paths.
func (c *UserCreate) ApplyDomainX(ctx context.Context, d *domain.User, opts ...entdomain.ApplyOption) *UserCreate
func (u *UserUpdateOne) ApplyDomainX(ctx context.Context, d *domain.User, opts ...entdomain.ApplyOption) *UserUpdateOne
func (u *UserUpdate) ApplyDomainX(ctx context.Context, d *domain.User, opts ...entdomain.ApplyOption) *UserUpdate
func (u *UserUpsertOne) ApplyDomainX(ctx context.Context, d *domain.User, opts ...entdomain.ApplyOption) *UserUpsertOne
func (u *UserUpsertBulk) ApplyDomainX(ctx context.Context, d *domain.User, opts ...entdomain.ApplyOption) *UserUpsertBulk
```

### Bulk methods (`ent/domain.go`, unless `WithNoBulk` is set)

```go
// Slice mapper
func (es Users) ToDomain() domain.UserList

// Create bulk
func (c *UserClient) CreateBulkDomain(ctx context.Context, ds domain.UserList, opts ...entdomain.ApplyOption) (*UserCreateBulk, error)
func (c *UserClient) CreateBulkDomainX(ctx context.Context, ds domain.UserList, opts ...entdomain.ApplyOption) *UserCreateBulk

// Update bulk by ID — mirrors UserCreateBulk API
func (c *UserClient) UpdateBulkDomain(ctx context.Context, ds domain.UserList, opts ...entdomain.ApplyOption) (*UserUpdateOneBulk, error)
func (c *UserClient) UpdateBulkDomainX(ctx context.Context, ds domain.UserList, opts ...entdomain.ApplyOption) *UserUpdateOneBulk

func (b *UserUpdateOneBulk) Save(ctx context.Context) (domain.UserList, error)
func (b *UserUpdateOneBulk) SaveX(ctx context.Context) domain.UserList
func (b *UserUpdateOneBulk) Exec(ctx context.Context) error
func (b *UserUpdateOneBulk) ExecX(ctx context.Context)
```

### Fluent chaining via `X` variants (opt-in panic)

Every error-returning `ApplyDomain*` / `CreateBulkDomain` / `UpdateBulkDomain` has a panicking sibling with the `X` suffix. The `X` variant restores fluent chaining by converting the error return into a panic — exactly matching ent's `SaveX` / `FirstX` / `OnlyX` convention:

```go
// Error-returning, production-safe — recommended on request paths
builder, err := client.User.Create().ApplyDomain(ctx, d)
if err != nil { return err }
created, err := builder.Save(ctx)

// Panicking, fluent — idiomatic in tests and scripts
created := client.User.Create().ApplyDomainX(ctx, d).SaveX(ctx)
```

**Guidance:** transformer errors are typically I/O (KMS timeouts, signer unavailable, network partition) — recoverable, transient, routinely observed in production. The default (non-X) form returns an error so these failures are handled, not crashed. Reach for `ApplyDomainX` only where panic is acceptable: tests, one-off scripts, migrations with a supervisor, contexts where you know transformers are pure.

### Typed field constants

```go
const (
    UserDomainFieldName    UserDomainField = "name"
    UserDomainFieldStatus  UserDomainField = "status"
    UserDomainFieldPostIDs UserDomainField = "post_ids"
    // ...
)
```

## Upsert Support

When ent's `gen.FeatureUpsert` is enabled in `gen.Config.Features`, entdomain automatically detects it and generates `ApplyDomain` on the `*EntityUpsertOne` and `*EntityUpsertBulk` builders — no additional annotation is required.

```go
// Single upsert — conflict on "email", apply domain fields on conflict:
client.User.Create().
    ApplyDomain(d).
    OnConflict(sql.ConflictColumns("email")).
    ApplyDomain(d).   // ← on *UserUpsertOne
    Exec(ctx)

// Bulk upsert — uniform conflict resolution across all rows:
client.User.CreateBulkDomain(ds).
    OnConflict(sql.ConflictColumns("email")).
    ApplyDomain(d).   // ← on *UserUpsertBulk (absent when NoBulk is set)
    Exec(ctx)
```

### Field Handling in Upsert

`*EntityUpsert` has `SetX` / `ClearX` but no `SetNillableX`. The upsert methods handle each field category as follows:

| Field type | Upsert behaviour |
|---|---|
| Non-nillable scalar / enum | `uu.SetX(val)` — same as UpdateOne |
| Nillable scalar | `if d.X != nil { uu.SetX(*d.X) }` — nil means leave unchanged |
| Nillable enum | `if d.X != nil { uu.SetX(EntType(*d.X)) }` |
| Immutable field | **skipped** — same as UpdateOne |
| Edge IDs | **skipped** — ent upsert does not support edge mutations |
| Virtual fields | **skipped** — no corresponding `*EntityUpsert` setter exists |

`*EntityUpsertBulk.ApplyDomain` is suppressed for entities that have `WithNoBulk` set — consistent with the existing bulk generation policy.

## FIQL Filtering

entdomain generates a typed FIQL filter entry point per entity. [FIQL](https://tools.ietf.org/html/draft-nottingham-atompub-fiql) expressions are URI-safe without percent-encoding — ideal for GET query parameters.

### Schema Annotation

Fields opt in explicitly. No field is filterable unless annotated — sensitive fields are never accidentally exposed.

```go
func (User) Fields() []ent.Field {
    return []ent.Field{
        field.String("name").
            Annotations(entdomain.Field(
                entdomain.FIQL(entdomain.EQ, entdomain.NEQ, entdomain.Contains),
            )),

        field.Int("score").
            Annotations(entdomain.Field(
                entdomain.FIQL(entdomain.EQ, entdomain.GT, entdomain.LT, entdomain.GTE, entdomain.LTE),
            )),

        field.Enum("status").Values("active", "inactive").
            Annotations(entdomain.Field(
                entdomain.FIQL(entdomain.EQ, entdomain.NEQ),
            )),

        field.Time("created_at").
            Annotations(entdomain.Field(
                entdomain.FIQL(entdomain.GTE, entdomain.LTE),
            )),

        field.String("password_hash"), // no FIQL → never filterable
    }
}
```

### Operator Constants

| Constant | FIQL syntax | Valid for |
|---|---|---|
| `EQ` | `==` | all types (string, int, float, bool, time, enum, uuid) |
| `NEQ` | `!=` | all types except bool |
| `GT` | `=gt=` | int, float, time |
| `LT` | `=lt=` | int, float, time |
| `GTE` | `=ge=` | int, float, time |
| `LTE` | `=le=` | int, float, time |
| `Contains` | `=like=` | string |
| `HasPrefix` | `=prefix=` | string |

UUID values are parsed via `uuid.Parse` from `github.com/google/uuid` — accepts canonical 36-char hyphenated form, braced (`{...}`), `urn:uuid:...`, and 32-char hex without hyphens.

Logical: `;` = AND, `,` = OR, `(` `)` = grouping. AND binds tighter than OR (standard FIQL precedence).

### Generated Code (`ent/fiql.go`)

```go
// Code generated by entdomain. DO NOT EDIT.

var UserFIQLFields = entdomain.FIQLFields[predicate.User]{
    "name":       entdomain.FIQLString[predicate.User]{EQ: user.NameEQ, NEQ: user.NameNEQ, Contains: user.NameContains},
    "score":      entdomain.FIQLInt[predicate.User]{EQ: user.ScoreEQ, GT: user.ScoreGT, LT: user.ScoreLT, GTE: user.ScoreGTE, LTE: user.ScoreLTE},
    "status":     entdomain.FIQLEnum[predicate.User]{
        EQ:  map[string]predicate.User{"active": user.StatusEQ(user.StatusActive), "inactive": user.StatusEQ(user.StatusInactive)},
        NEQ: map[string]predicate.User{"active": user.StatusNEQ(user.StatusActive), "inactive": user.StatusNEQ(user.StatusInactive)},
    },
    "created_at": entdomain.FIQLTime[predicate.User]{GTE: user.CreatedAtGTE, LTE: user.CreatedAtLTE},
    "external_id": entdomain.FIQLUUID[predicate.User]{EQ: user.ExternalIDEQ, NEQ: user.ExternalIDNEQ},
}

func UserFIQL(expr string) (predicate.User, error) {
    return entdomain.ParseFIQL(expr, UserFIQLFields)
}
```

### HTTP Handler Usage

```go
func (h *UserHandler) List(w http.ResponseWriter, r *http.Request) {
    q := h.client.User.Query()
    if expr := r.URL.Query().Get("filter"); expr != "" {
        pred, err := ent.UserFIQL(expr)
        if err != nil {
            http.Error(w, err.Error(), http.StatusBadRequest)
            return
        }
        q = q.Where(pred)
    }
    users, err := q.All(r.Context())
    // ...
}
```

GET request — no percent-encoding required:
```
GET /users?filter=name==john;score=gt=25,status==active
GET /users?filter=(status==active,status==inactive);created_at=ge=2024-01-01T00:00:00Z
```

### Error Handling

```
unknown field "email" — annotate with entdomain.FIQL(...) to enable
operator "=gt=" not allowed on field "name" (String) — allowed: ==, !=, =like=
unknown enum value "pending" for field "status" — valid values: active, inactive
invalid integer value "abc" for field "score": strconv.Atoi: ...
invalid time value "not-a-time" for field "created_at": ...
```

### Known Limitations

- **UUID fields with custom `GoType(...)`** — only the canonical `github.com/google/uuid.UUID` type is wired. The codegen explicitly checks `f.Type.RType` and skips any other GoType, so a custom UUID field is omitted from the FIQL registry rather than producing a signature-mismatched generated file.
- **Time values** — must be RFC3339 format (`2006-01-02T15:04:05Z07:00`).
- **Nesting depth** — maximum 50 levels; deeper expressions return `"maximum nesting depth exceeded"`.
- **Edge fields** — cross-entity filtering (e.g. `owner.name==john`) is out of scope.
- **JSON/virtual fields** — no DB column mapping; cannot be used as FIQL fields.

## Apply Options

Control which fields `ApplyDomain` writes to the ent builder:

```go
entdomain.OmitZeroVal()                      // skip fields with zero values
entdomain.OmitNil()                          // skip nil pointer fields
entdomain.OmitFields("bio", "score")         // skip specific fields
entdomain.OnlyFields("name", "status")       // allowlist specific fields
entdomain.AppendEdge("post_ids")             // append edge IDs instead of replacing
```

> `OmitZeroVal` on create can silently skip intentional zero values — use with care.

### Example

```go
// Update only name and status, append new posts rather than replace
u.UpdateOneID(id).
    ApplyDomain(d,
        entdomain.OnlyFields(ent.UserDomainFieldName, ent.UserDomainFieldStatus),
        entdomain.AppendEdge(ent.UserDomainFieldPostIDs),
    ).
    Save(ctx)
```

## Transformer (Virtual Fields)

For virtual fields that require custom logic, wire a per-entity transformer at startup. Only set the functions you need — unset ones are skipped by `ToDomain()` and `ApplyDomain()`.

```go
// generated in ent/domain.go
type UserDomainTransformer struct {
    // Pure synchronous projection, invoked by ToDomain. No ctx, no error.
    GetFullName func(e *User) string

    // Invoked during ApplyDomain. Receives ctx + the full domain struct
    // (sibling-field access for key derivation / AAD binding) and may return
    // an error. May perform I/O (KMS-backed field encryption, signing, etc.).
    SetFullNameOnCreate func(ctx context.Context, c *UserCreate, d *domain.User, val string) error
    SetFullNameOnUpdate func(ctx context.Context, u *UserUpdateOne, d *domain.User, val string) error
    // one Get + two Set functions per virtual field
}

var UserTransformer *UserDomainTransformer // nil by default
```

Wire at app startup (infrastructure handles captured in closure):

```go
func registerTransformers(kms KMS) {
    ent.UserTransformer = &ent.UserDomainTransformer{
        // Pure projection — no ctx, no error.
        GetFullName: func(u *ent.User) string {
            return u.FirstName + " " + u.LastName
        },
        // Stateful transform with sibling field access + ctx-aware I/O.
        SetSecretTokenOnCreate: func(ctx context.Context, c *ent.UserCreate, d *domain.User, val string) error {
            subkey, err := kms.DeriveKey(ctx, d.TenantID) // sibling field used to scope encryption
            if err != nil {
                return fmt.Errorf("derive key: %w", err)
            }
            c.SetSecretToken(encrypt(subkey, val))
            return nil
        },
        // other functions left nil — skipped automatically
    }
}
```

See [ADR-005](adr/005-transformer-runtime-context.md) for the design rationale (why ctx+error, why sibling access, why this shape aligns with ent's hook/privacy conventions).

### Advanced: ent hook integration via `entdomain.WithDomain` (opt-in)

Most domain-to-ent encoding belongs in transformers above. For the narrower case where work must happen at ent-hook time — outbox event emission, cross-cutting audit logging, tenant-isolation checks that depend on virtual-field values — entdomain exposes a small escape hatch:

```go
// entdomain package
func WithDomain[T any](ctx context.Context, d T) context.Context
func DomainFrom[T any](ctx context.Context) (T, bool)
```

Usage at the repository boundary:

```go
func (r *UserRepo) Create(ctx context.Context, d *domain.User) (*domain.User, error) {
    ctx = entdomain.WithDomain(ctx, d) // explicit opt-in at call site

    builder, err := r.client.User.Create().ApplyDomain(ctx, d)
    if err != nil { return nil, err }

    created, err := builder.Save(ctx) // hooks now see d via DomainFrom[*domain.User](ctx)
    if err != nil { return nil, err }

    return created.ToDomain(), nil
}
```

Registered ent hook can then retrieve the domain struct, including virtual fields:

```go
client.User.Use(func(next ent.Mutator) ent.Mutator {
    return hook.UserFunc(func(ctx context.Context, m *ent.UserMutation) (ent.Value, error) {
        d, ok := entdomain.DomainFrom[*domain.User](ctx)
        if !ok {
            // Not an entdomain-originated mutation — skip domain-aware logic.
            return next.Mutate(ctx, m)
        }
        // d.TenantID, d.VirtualField, etc. available here.
        return next.Mutate(ctx, m)
    })
})
```

**When to use `WithDomain` vs a transformer:**

| Use case | Pick |
|---|---|
| Field-level encryption, signing, tokenization | Transformer |
| Virtual field that must flow through to an ent column | Transformer |
| Audit log / outbox event / cross-cutting concern needing domain context at Save time | Hook + `WithDomain` |
| Tenant-isolation check reading virtual-field values | Hook + `WithDomain` |

**Caveats:**

- `WithDomain` uses `context.Value`, which Go's guidance reserves for request-scoped data. Static analyzers may flag call sites. That is expected — the smell lives at the opt-in site, not inside the library. Transformers remain the first-class path.
- Hook authors must handle the absent case via the `ok` return. Bare `.Save(ctx)` calls that skipped `WithDomain` will see `ok == false`.
- Distinct `T` parameters produce distinct context keys, so `WithDomain(ctx, user)` and `WithDomain(ctx, post)` coexist cleanly.
- The library does **not** auto-stash inside `ApplyDomain`. Callers opt in explicitly so the ctx chain stays visible.

## Repository Adapter Example

```go
// This layer owns ent — domain package has zero knowledge of it.

func (r *UserRepo) GetByID(ctx context.Context, id int) (*domain.User, error) {
    u, err := r.client.User.Query().
        Where(user.ID(id)).
        WithPosts().
        Only(ctx)
    if err != nil {
        return nil, err
    }
    return u.ToDomain(), nil
}

func (r *UserRepo) Create(ctx context.Context, d *domain.User) (*domain.User, error) {
    builder, err := r.client.User.Create().ApplyDomain(ctx, d)
    if err != nil {
        return nil, err
    }
    created, err := builder.Save(ctx)
    if err != nil {
        return nil, err
    }
    return created.ToDomain(), nil
}

func (r *UserRepo) Update(ctx context.Context, d *domain.User) (*domain.User, error) {
    builder, err := r.client.User.UpdateOneID(d.ID).ApplyDomain(ctx, d,
        entdomain.OnlyFields(ent.UserDomainFieldName, ent.UserDomainFieldStatus),
    )
    if err != nil {
        return nil, err
    }
    updated, err := builder.Save(ctx)
    if err != nil {
        return nil, err
    }
    return updated.ToDomain(), nil
}

func (r *UserRepo) CreateBulk(ctx context.Context, ds domain.UserList) (domain.UserList, error) {
    bulk, err := r.client.User.CreateBulkDomain(ctx, ds)
    if err != nil {
        return nil, err
    }
    saved, err := bulk.Save(ctx)
    if err != nil {
        return nil, err
    }
    result := make(domain.UserList, len(saved))
    for i, u := range saved {
        result[i] = u.ToDomain()
    }
    return result, nil
}

func (r *UserRepo) UpdateBulk(ctx context.Context, ds domain.UserList) (domain.UserList, error) {
    bulk, err := r.client.User.UpdateBulkDomain(ctx, ds)
    if err != nil {
        return nil, err
    }
    return bulk.Save(ctx)
}

func (r *UserRepo) DeactivateAll(ctx context.Context) error {
    builder, err := r.client.User.Update().ApplyDomain(ctx,
        &domain.User{Status: domain.UserStatusInactive},
        entdomain.OnlyFields(ent.UserDomainFieldStatus),
    )
    if err != nil {
        return err
    }
    return builder.Where(user.StatusEQ(user.StatusActive)).Exec(ctx)
}

// Upsert — requires gen.FeatureUpsert in gen.Config.Features
func (r *UserRepo) Upsert(ctx context.Context, d *domain.User) error {
    createBuilder, err := r.client.User.Create().ApplyDomain(ctx, d)
    if err != nil {
        return err
    }
    upsertBuilder, err := createBuilder.
        OnConflict(sql.ConflictColumns("username")).
        ApplyDomain(ctx, d)
    if err != nil {
        return err
    }
    return upsertBuilder.Exec(ctx)
}

func (r *UserRepo) UpsertBulk(ctx context.Context, ds domain.UserList) error {
    bulk, err := r.client.User.CreateBulkDomain(ctx, ds)
    if err != nil {
        return err
    }
    upsertBulk, err := bulk.
        OnConflict(sql.ConflictColumns("username")).
        ApplyDomain(ctx, ds[0]) // same conflict resolution for all rows
    if err != nil {
        return err
    }
    return upsertBulk.Exec(ctx)
}
```

## Proto Generation

entdomain can generate `.proto` message files and domain↔proto mappers alongside the existing domain layer. This is opt-in and keeps the domain package itself proto-free.

### Enable in `entc.go`

```go
ex, err := entdomain.NewExtension(
    entdomain.WithPackagePath("internal/domain"),
    entdomain.WithPackageName("domain"),

    entdomain.WithProto(
        entdomain.WithProtoDir("proto"),              // output dir, relative to module root
        entdomain.WithProtoPackageName("entpb"),      // proto package name
        entdomain.WithProtoGoPackage("github.com/myorg/myrepo/proto/entpb;entpb"),
    ),
)
```

### Generated Output

```
proto/entpb/
  .entdomain.lock.json     ← stable field number registry — commit this file
  ent_messages.proto       ← all entity messages in one file

internal/domain/
  pbmap/                   ← domain ↔ proto mappers (package pbmap)
    user_proto_gen.go
    post_proto_gen.go
    proto_helpers_gen.go   ← shared helpers (ToInt64Slice, MapToProtoStruct, etc.)
```

### Mapper Usage

```go
import "github.com/myorg/myrepo/internal/domain/pbmap"

// domain → proto
p := pbmap.UserToProto(user)
ps := pbmap.UserListToProto(users)

// proto → domain
d := pbmap.UserFromProto(req.User)
ds := pbmap.UserListFromProto(req.Users)
```

### Field Opt-out (`SkipProto`)

Any ent field or edge can be excluded from proto output:

```go
func (User) Fields() []ent.Field {
    return []ent.Field{
        field.String("name"),
        field.String("password_hash").
            Annotations(entdomain.Field(entdomain.SkipProto())),
    }
}

func (User) Edges() []ent.Edge {
    return []ent.Edge{
        // Break a mutual Nest cycle on one side:
        edge.From("owner", User.Type).Ref("posts").Unique().
            Annotations(
                entdomain.Edge(entdomain.IDs(), entdomain.Nest()),
                entdomain.Field(entdomain.SkipProto()),
            ),
    }
}
```

### Custom Proto Type

`ProtoType()` works with both `VirtualField()` and `Field()`, making it usable in two contexts:

#### Virtual fields

Fields with a `GoType` that has no well-known mapping are excluded by default. Supply an explicit type to include them:

```go
entdomain.VirtualField("amount",
    entdomain.GoType("Decimal", "github.com/shopspring/decimal"),
    entdomain.ProtoType("google.type.Money", "google/type/money.proto"),
)
```

#### Ent JSON fields (`Field(ProtoType(...))`)

Typed JSON fields are excluded by default (except `map[string]any` which auto-maps to `google.protobuf.Struct`). Use `Field(ProtoType(...))` to opt any JSON field into proto:

```go
// Typed map → Struct (would otherwise be excluded)
field.JSON("labels", map[string]string{}).
    Annotations(entdomain.Field(
        entdomain.ProtoType("google.protobuf.Struct", "google/protobuf/struct.proto"),
    ))

// Slice → repeated scalar (IsRepeated is auto-inferred from the [] prefix)
field.JSON("tag_names", []string{}).
    Annotations(entdomain.Field(entdomain.ProtoType("string")))

// Custom struct → hand-defined proto message
// WithConversion provides the Go conversion expressions (%s = source expression).
// The actual converter functions must be hand-written in the pbmap package.
field.JSON("metadata", domain.UserMetadata{}).
    Annotations(entdomain.Field(
        entdomain.ProtoType("UserMetadata", "entpb/user_metadata.proto").
            WithConversion("UserMetadataToProto(%s)", "UserMetadataFromProto(%s)"),
    ))
```

### Proto Helper Functions

The generated `proto_helpers_gen.go` file in the `pbmap` package includes shared conversion helpers used across all entity mappers:

```go
func ToInt64Slice(ids []int) []int64
func FromInt64Slice(ids []int64) []int
func ToInt64Ptr(v *int) *int64
func FromInt64Ptr(v *int64) *int
func MapToProtoStruct(m map[string]any) *structpb.Struct  // for map[string]any fields
func ProtoStructToMap(s *structpb.Struct) map[string]any  // inverse of MapToProtoStruct
// ... and more
```

`MapToProtoStruct` and `ProtoStructToMap` are used automatically for `field.JSON("x", map[string]any{})` fields. When using `Field(ProtoType(...)).WithConversion(...)` on other JSON fields, the hand-written conversion functions in the `pbmap` package are called instead.

### Built-in Auto-Mappings

| entdomain / ent type | Proto type | Import |
|---|---|---|
| `entdomain.String` | `string` | — |
| `entdomain.Bool` | `bool` | — |
| `entdomain.Int` | `int64` | — |
| `entdomain.Float64` | `double` | — |
| `GoType("Time", "time")` | `google.protobuf.Timestamp` | `google/protobuf/timestamp.proto` |
| `GoType("Duration", "time")` | `google.protobuf.Duration` | `google/protobuf/duration.proto` |
| `GoType("UUID", "github.com/google/uuid")` | `string` | — |
| `field.Time(...)` | `google.protobuf.Timestamp` | `google/protobuf/timestamp.proto` |
| `field.UUID(...)` | `string` | — |
| `field.Enum(...)` | top-level enum | — |
| `field.JSON("x", map[string]any{})` | `google.protobuf.Struct` | `google/protobuf/struct.proto` |
| `field.JSON("x", []T{})` + `Field(ProtoType("T"))` | `repeated T` | depends on T |
| `field.JSON("x", OtherType{})` | **excluded** (use `Field(ProtoType(...))` to opt in) | — |

### Nest Edges in Proto

Nest edges are included in the proto output by default, generating embedded messages:

```protobuf
message User {
  repeated int64 post_ids = 8;   // IDs edge
  repeated Post  posts    = 9;   // Nest edge
}
```

Proto3 supports circular message types at the wire level (e.g. `User` nests `Tag` and `Tag` nests `User`). Use `SkipProto()` on one side to break the cycle in the generated proto if desired.

### Field Number Stability

Proto field numbers are tracked in `.entdomain.lock.json`. Commit this file — it ensures wire compatibility across schema changes. Removed fields are permanently reserved and never reused.

---

## Design Notes

- Nested edge mutations (creating/updating child entities) are intentionally not generated — manage them in the repository layer
- Virtual fields are always zero in `ToDomain()` unless a Transformer is wired; each transformer function field is nil-checked individually before calling
- Immutable ent fields are excluded from `UpdateOne.ApplyDomain`, `Update.ApplyDomain`, and `UpsertOne/Bulk.ApplyDomain`
- Nested edges (`Nest()`) are excluded from `ApplyDomain` entirely; only `IDs()` edges are written
- Edge IDs are additionally excluded from upsert `ApplyDomain` — ent's `*EntityUpsert` type does not support edge mutations
- Virtual field transformer hooks are excluded from upsert `ApplyDomain` — transformer setters are typed to `*UserCreate` / `*UserUpdateOne` only
- Upsert nillable fields use an explicit `d.X != nil` guard with dereference (`uu.SetX(*d.X)`) instead of `SetNillableX` — `*EntityUpsert` has no `SetNillable*` methods
- `(*UserUpdate).ApplyDomain` does not call virtual field transformer hooks — transformer setters are typed to `*UserUpdateOne`
- `Optional()` fields without `.Nillable()` are stored as base types in ent (`string`, `int`, …) but mapped to pointers in the domain struct by taking their address — `&e.Bio`. The zero value is never nil in this case; use `.Nillable()` in the schema if you need nil-distinguishable optionals
- `WithNoBulk` is configured at extension level, not per schema — keeps schema annotations focused on domain shape, not generation policy; it also suppresses `*EntityUpsertBulk.ApplyDomain`
- Upsert generation is auto-detected from `gen.Config.Features` — no entdomain annotation or config option is needed

See [`adr/001-entdomain-extension.md`](adr/001-entdomain-extension.md) for the full design rationale.

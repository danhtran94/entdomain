# ADR-001: entdomain — Domain Layer Code Generation Extension

## Status

Proposed

## Context

In clean architecture, the domain layer contains pure business entities with no dependency on infrastructure concerns (database, ORM, transport). However, when using `entgo.io/ent` as the ORM, the generated types (`ent.User`, `ent.UserCreate`, etc.) carry DB-layer concerns and are tightly coupled to the ent runtime.

Developers who want to enforce a clean architecture boundary are forced to manually write and maintain:
- Pure Go domain structs mirroring ent schema fields
- Mapping functions between domain structs and ent-generated types
- Enum re-declarations free of ent dependency

This creates duplication and drift between the ent schema and the domain layer over time.

## Assumptions

- **A1 [EXTERNAL FACT]:** ent v0.14.x supports third-party code generation via the `entc.Extension` interface. Evidence: [ent extension docs](https://entgo.io/docs/extensions/) and the public `gen.Hook` API.
- **A2 [HYPOTHESIS]:** Schema authors will opt into domain generation per entity (via annotation), not globally. Verification deferred — the annotation API was designed to support it; adoption pattern observed in `examples/basic/` and `examples/custom/`.
- **A3 [VERIFIED]:** ent's generated package can coexist in the same module as a separate domain package without cyclic imports as long as the domain package has no ent imports. Evidence: `examples/basic/internal/domain/` compiles cleanly with no `entgo.io/ent` references (`grep -L 'entgo.io/ent' examples/basic/internal/domain/*.go`).

## Options

Three approaches were weighed before adopting an ent extension:

- **Hand-written domain layer** — Engineers maintain domain structs and mappers manually for each entity.
- **Adopt a different ORM that already separates domain from DB** — e.g. an ORM where the user-facing types are pure structs.
- **ent extension that codegens the domain layer** — Annotation-driven generation runs as part of `entc.Generate`.

## Options Comparison

| Driver | Hand-written | Different ORM | ent extension (chosen) |
|---|---|---|---|
| Drift risk | High | None | None |
| Migration cost | None | Very high | Low |
| ent ecosystem | Kept | Lost | Kept |
| Per-entity opt-in | Manual | N/A | Annotation |

## Decision

Introduce a new ent extension — `entdomain` — that generates a pure Go domain package and mapping helpers from ent schema definitions, controlled via schema annotations.

---

## Design

### Extension Configuration

```go
entdomain.NewExtension(
    entdomain.WithPackagePath("internal/domain"), // output path for domain package
    entdomain.WithPackageName("domain"),           // generated package name

    // Disable bulk generation for specific entities:
    entdomain.WithNoBulk("Post", "Order"),

    // Or disable bulk generation for all entities:
    entdomain.WithNoBulk(),
)
```

`WithPackagePath` and `WithProtoDir` are resolved relative to the **module root** (the directory containing `go.mod`), not relative to the `ent` directory. When the schema lives outside the `ent` directory, ent derives the generated package name from the schema's parent — set `gen.Config.Package` explicitly to override:

```go
entc.Generate("../schema", &gen.Config{
    Target:  ".",
    Package: "github.com/myorg/myproject/repo/ent",
}, entc.Extensions(ex))
```

`WithNoBulk` suppresses generation of `XxxList`, `(Xxxs).ToDomain()`, `CreateBulkDomain`, `UpdateBulkDomain`, and `XxxUpdateOneBulk` for the named entities. Called with no arguments it applies to all entities.

### Annotation API

```go
// Opt-in entity generation
entdomain.Entity()

// Edge options — opt-in per edge via edge.Annotations()
entdomain.Edge(entdomain.IDs())                   // → PostIDs []int (plural)
entdomain.Edge(entdomain.Nest())                  // → Profile Profile (singular)
entdomain.Edge(entdomain.IDs(), entdomain.Nest()) // → both PostIDs []int and Posts []Post

// Virtual fields — built-in primitive types
entdomain.VirtualField("full_name", entdomain.String)
entdomain.VirtualField("is_premium", entdomain.Bool)
entdomain.VirtualField("count",      entdomain.Int)
entdomain.VirtualField("ratio",      entdomain.Float64)

// Virtual fields — arbitrary Go types via GoType(typeName, pkgPath?)
// typeName: Go type name as-is; prefix with "*" for pointer types.
// pkgPath: optional full import path; omit for same-package or stdlib.
// Generator emits the type name directly, prepending the package qualifier after any "*".
entdomain.VirtualField("amount",   entdomain.GoType("Money"))                                               // → Money
entdomain.VirtualField("tags",     entdomain.GoType("[]string"))                                            // → []string
entdomain.VirtualField("metadata", entdomain.GoType("map[string]any"))                                      // → map[string]any
entdomain.VirtualField("price",    entdomain.GoType("Decimal", "github.com/shopspring/decimal"))             // → decimal.Decimal
entdomain.VirtualField("price2",   entdomain.GoType("*Decimal", "github.com/shopspring/decimal"))            // → *decimal.Decimal
entdomain.VirtualField("ext_id",   entdomain.GoType("UUID", "github.com/google/uuid"))                      // → uuid.UUID
entdomain.VirtualField("opt_ref",  entdomain.GoType("*Money"))                                              // → *Money
```

### Output Structure

```
internal/domain/        ← configurable via WithPackagePath
  user.go               ← pure Go struct + entity-scoped enum types
  post.go
  order.go

ent/
  domain.go             ← generated mapping methods for all domain entities (single file)
```

### Generated Domain Struct (`internal/domain/user.go`)

Pure Go struct with no ent imports. Rules:
- **ID type** mirrors the ent schema ID type (`int`, `uuid.UUID`, etc.)
- **Optional fields** (`field.Optional()`) are pointers in the domain struct
- **Enum types** are re-declared per entity file with the entity name as prefix to avoid collision (e.g. `UserStatus`, `OrderStatus` — even if the underlying values are identical)
- **Singular edges** (`HasOne`/`BelongsTo`) produce a single value, not a slice
- **`XxxList`** is `[]*Xxx` (pointer slice) — only generated when bulk is not disabled

```go
package domain

import (
    "time"
    "github.com/google/uuid"
)

// Enum re-declared with entity-scoped name — avoids cross-entity collision
type UserStatus string

const (
    UserStatusActive   UserStatus = "active"
    UserStatusInactive UserStatus = "inactive"
)

type User struct {
    ID        uuid.UUID  // mirrors ent schema ID type
    Name      string
    Bio       *string    // optional field → pointer
    Status    UserStatus
    CreatedAt time.Time

    // Singular IDs edge (HasOne/BelongsTo)
    ProfileID int
    // Singular nested edge
    Profile   Profile

    // Plural IDs edge (HasMany)
    PostIDs []int
    // Plural nested edge
    Posts   []Post

    // Virtual fields — zero value in mapper, hydrate manually
    FullName  string           // entdomain.String
    IsPremium bool             // entdomain.Bool
    Amount    Money            // entdomain.GoType("Money")
    Tags      []string         // entdomain.GoType("[]string")
    Metadata  map[string]any   // entdomain.GoType("map[string]any")
}

// UserList is a pointer slice of User — generated unless WithNoBulk is set.
type UserList []*User
```

### Generated Mapping Methods (`ent/domain.go`)

Up to seven methods are generated per entity on the ent-generated types. All entities are emitted into a single `ent/domain.go` file. The two upsert methods (5 and 6) are only generated when `gen.FeatureUpsert` is present in `gen.Config.Features`.

#### 1. Read — `*ent.User → *domain.User`

Optional non-nillable fields (e.g. `field.Optional()` without `.Nillable()`) are stored as their base type in the ent struct but mapped to a pointer in the domain struct by taking the field's address.

```go
func (e *User) ToDomain() *domain.User {
    d := &domain.User{
        ID:     e.ID,
        Name:   e.Name,
        Bio:    &e.Bio,   // Optional(), non-Nillable → take address
        Status: domain.UserStatus(e.Status),
    }

    // IDs and Nest edges — populated only when eagerly loaded
    for _, p := range e.Edges.Posts {
        d.PostIDs = append(d.PostIDs, p.ID)
        d.Posts = append(d.Posts, *p.ToDomain()) // if Nest() also declared
    }

    // Transformer hook — each function field is nil-checked individually
    if UserTransformer != nil && UserTransformer.GetFullName != nil {
        d.FullName = UserTransformer.GetFullName(e)
    }

    return d
}
```

#### 2. Create — `*ent.UserCreate ← *domain.User`

IDs edges are mapped. Nested edges and virtual fields are skipped (unless a Transformer is wired).

```go
func (c *UserCreate) ApplyDomain(d *domain.User, opts ...entdomain.ApplyOption) *UserCreate {
    cfg := entdomain.NewApplyConfig(opts...)

    if cfg.ShouldApply("name", d.Name) {
        c = c.SetName(d.Name)
    }
    if cfg.ShouldApply("status", d.Status) {
        c = c.SetStatus(user.Status(d.Status))
    }

    // IDs edges mapped on create
    if len(d.PostIDs) > 0 {
        c = c.AddPostIDs(d.PostIDs...)
    }

    // Transformer hook — each function field is nil-checked individually
    if UserTransformer != nil && UserTransformer.SetFullNameOnCreate != nil {
        UserTransformer.SetFullNameOnCreate(c, d.FullName)
    }

    return c
}
```

#### 3. Update (by ID) — `*ent.UserUpdateOne ← *domain.User`

Same as create for scalars. IDs edges default to **replace** (desired final state semantics). Immutable fields are excluded. Nested edges are skipped. Virtual field transformer hooks are called when wired.

```go
func (u *UserUpdateOne) ApplyDomain(d *domain.User, opts ...entdomain.ApplyOption) *UserUpdateOne {
    cfg := entdomain.NewApplyConfig(opts...)

    if cfg.ShouldApply("name", d.Name) {
        u = u.SetName(d.Name)
    }

    // IDs edges — replace by default, opt-in append via AppendEdge option
    if cfg.IsAppendEdge("post_ids") {
        u = u.AddPostIDs(d.PostIDs...)
    } else {
        u = u.ClearPosts().AddPostIDs(d.PostIDs...)
    }

    // Transformer hook — each function field is nil-checked individually
    if UserTransformer != nil && UserTransformer.SetFullNameOnUpdate != nil {
        UserTransformer.SetFullNameOnUpdate(u, d.FullName)
    }

    return u
}
```

#### 4. Update (by WHERE) — `*ent.UserUpdate ← *domain.User`

Same field/edge logic as `UpdateOne` but operates on the WHERE-based `*UserUpdate` builder. Caller chains `.Where(...)` conditions. Virtual field transformer hooks are not called (transformer setters are typed to `*UserUpdateOne`).

```go
func (u *UserUpdate) ApplyDomain(d *domain.User, opts ...entdomain.ApplyOption) *UserUpdate {
    cfg := entdomain.NewApplyConfig(opts...)

    if cfg.ShouldApply("name", d.Name) {
        u = u.SetName(d.Name)
    }

    if cfg.IsAppendEdge("post_ids") {
        u = u.AddPostIDs(d.PostIDs...)
    } else {
        u = u.ClearPosts().AddPostIDs(d.PostIDs...)
    }

    return u
}
```

Usage:
```go
client.User.Update().
    ApplyDomain(&domain.User{Status: domain.UserStatusInactive},
        entdomain.OnlyFields("status"),
    ).
    Where(user.StatusEQ(user.StatusActive)).
    ExecX(ctx)
```

#### 5. Upsert (single) — `*ent.UserUpsertOne ← *domain.User`

Generated only when `gen.FeatureUpsert` is enabled in `gen.Config.Features`. Entdomain auto-detects the feature — no annotation or config option is needed. The method wraps `u.Update(func(uu *UserUpsert) {...})` and applies scalar fields only.

`*UserUpsert` has no `SetNillableX` methods, so nillable fields use an explicit nil guard with dereference. Edge IDs, immutable fields, and virtual fields are all excluded (ent's upsert type does not support edge or transformer setter calls).

```go
func (u *UserUpsertOne) ApplyDomain(d *domain.User, opts ...entdomain.ApplyOption) *UserUpsertOne {
    cfg := entdomain.NewApplyConfig(opts...)
    _ = cfg
    return u.Update(func(uu *UserUpsert) {
        if cfg.ShouldApply("name", d.Name) {
            uu.SetName(d.Name)
        }
        // nillable scalar — nil guard + dereference (no SetNillable* on *UserUpsert)
        if cfg.ShouldApplyPtr("bio", d.Bio) && d.Bio != nil {
            uu.SetBio(*d.Bio)
        }
        // enum
        if cfg.ShouldApply("status", d.Status) {
            uu.SetStatus(user.Status(d.Status))
        }
        // immutable fields (created_at, username) — excluded
        // edge IDs (post_ids, tag_ids) — excluded
        // virtual fields — excluded
    })
}
```

Usage:
```go
client.User.Create().
    ApplyDomain(d).
    OnConflict(sql.ConflictColumns("username")).
    ApplyDomain(d).
    Exec(ctx)
```

#### 6. Upsert (bulk) — `*ent.UserUpsertBulk ← *domain.User`

Same body as `UpsertOne`, applied to the bulk upsert builder. Suppressed for entities with `WithNoBulk` set — consistent with the existing `CreateBulkDomain` / `UpdateBulkDomain` gating.

```go
func (u *UserUpsertBulk) ApplyDomain(d *domain.User, opts ...entdomain.ApplyOption) *UserUpsertBulk {
    cfg := entdomain.NewApplyConfig(opts...)
    _ = cfg
    return u.Update(func(uu *UserUpsert) {
        // identical body to UpsertOne
    })
}
```

Usage:
```go
client.User.CreateBulkDomain(ds).
    OnConflict(sql.ConflictColumns("username")).
    ApplyDomain(ds[0]).
    Exec(ctx)
```

#### Field Handling Comparison

| Field type | Create | UpdateOne / Update | UpsertOne / UpsertBulk |
|---|---|---|---|
| Non-nillable scalar | `c.SetX(val)` | `u.SetX(val)` | `uu.SetX(val)` |
| Non-nillable enum | `c.SetX(EntType(val))` | `u.SetX(EntType(val))` | `uu.SetX(EntType(val))` |
| Nillable scalar | `c.SetNillableX(ptr)` | `u.SetNillableX(ptr)` | `if ptr != nil { uu.SetX(*ptr) }` |
| Nillable enum | `c.SetNillableX(&entVal)` | `u.SetNillableX(&entVal)` | `if ptr != nil { uu.SetX(EntType(*ptr)) }` |
| Immutable | included | **skipped** | **skipped** |
| Edge IDs | included | replace / append | **skipped** |
| Virtual fields | transformer | transformer | **skipped** |

---

### Bulk Operations

Generated unless `WithNoBulk` is set for the entity. Note: `XxxUpsertBulk.ApplyDomain` is also suppressed by `WithNoBulk` — see section 6 above.

#### Slice mapper — `ent.Users → domain.UserList`

```go
func (es Users) ToDomain() domain.UserList
```

#### Create bulk — `domain.UserList → *UserCreateBulk`

```go
func (c *UserClient) CreateBulkDomain(ds domain.UserList, opts ...entdomain.ApplyOption) *UserCreateBulk
```

#### Update bulk (by ID) — `domain.UserList → *UserUpdateOneBulk`

Each item is keyed on `ds[i].ID`. Returns a `*UserUpdateOneBulk` wrapper with the same API as ent's `*UserCreateBulk`.

```go
func (c *UserClient) UpdateBulkDomain(ds domain.UserList, opts ...entdomain.ApplyOption) *UserUpdateOneBulk

// UserUpdateOneBulk mirrors the UserCreateBulk API:
func (b *UserUpdateOneBulk) Save(ctx context.Context) (domain.UserList, error)
func (b *UserUpdateOneBulk) SaveX(ctx context.Context) domain.UserList
func (b *UserUpdateOneBulk) Exec(ctx context.Context) error
func (b *UserUpdateOneBulk) ExecX(ctx context.Context)
```

---

### Apply Options

Defined in the `entdomain` runtime package (not generated), shared across all entities.

```go
// Scalar field options
entdomain.OmitZeroVal()              // skip fields with zero values
entdomain.OmitNil()                  // skip pointer fields that are nil
entdomain.OmitFields(fields ...string) // skip specific fields by name
entdomain.OnlyFields(fields ...string) // allowlist specific fields

// Edge options
entdomain.AppendEdge(field string) // override default replace with append
```

Note: `OmitZeroVal` and `OmitNil` apply to both `UserCreate.ApplyDomain` and `UserUpdateOne.ApplyDomain`. On create, use with care — a zero value may be intentional.

### Typed Field Constants

Generated per entity for type-safe option usage with IDE autocomplete.

```go
// generated in ent/domain.go
type UserDomainField = string

const (
    UserDomainFieldName    UserDomainField = "name"
    UserDomainFieldStatus  UserDomainField = "status"
    UserDomainFieldPostIDs UserDomainField = "post_ids"
)
```

### Runtime Helpers (`entdomain` package)

```go
type ApplyConfig struct { ... }

func NewApplyConfig(opts ...ApplyOption) *ApplyConfig

// ShouldApply checks OmitZeroVal, OmitFields, OnlyFields for non-pointer fields
func (c *ApplyConfig) ShouldApply(field string, val any) bool

// ShouldApplyPtr checks OmitNil, OmitFields, OnlyFields for pointer fields
func (c *ApplyConfig) ShouldApplyPtr(field string, val any) bool

// IsAppendEdge returns true if the given edge field should use append instead of replace
func (c *ApplyConfig) IsAppendEdge(field string) bool
```

---

### Transformer (Virtual Fields & Type Coercion)

For virtual fields and custom type transformations that cannot be auto-generated, an optional per-entity transformer struct is generated. Each field is a nullable function — only set what you need, unset fields are skipped by the mapper.

```go
// generated in ent/domain.go

type UserDomainTransformer struct {
    // Virtual field getters (ent → domain)
    // Type is inferred from the annotation: entdomain.String → string, entdomain.GoType("Money") → Money
    GetFullName  func(e *User) string
    GetIsPremium func(e *User) bool
    GetAmount    func(e *User) Money
    GetTags      func(e *User) []string

    // Virtual field setters (domain → ent, typed to *UserCreate / *UserUpdateOne)
    SetFullNameOnCreate  func(c *UserCreate, val string)
    SetFullNameOnUpdate  func(u *UserUpdateOne, val string)
    SetIsPremiumOnCreate func(c *UserCreate, val bool)
    SetIsPremiumOnUpdate func(u *UserUpdateOne, val bool)
    SetAmountOnCreate    func(c *UserCreate, val Money)
    SetAmountOnUpdate    func(u *UserUpdateOne, val Money)
    SetTagsOnCreate      func(c *UserCreate, val []string)
    SetTagsOnUpdate      func(u *UserUpdateOne, val []string)
}

// package-level var — nil by default
var UserTransformer *UserDomainTransformer
```

Mapper checks both the transformer and each individual function field before calling:
```go
if UserTransformer != nil && UserTransformer.GetFullName != nil {
    d.FullName = UserTransformer.GetFullName(e)
}
```

Wired at startup — only the fields you need:
```go
ent.UserTransformer = &ent.UserDomainTransformer{
    GetFullName: func(u *ent.User) string {
        return u.FirstName + " " + u.LastName
    },
    // GetIsPremium not set — nil, skipped in mapper
}
```

---

### Edge Mapping Summary

| Edge annotation | Domain struct field | `ToDomain()` | `Create.ApplyDomain` | `UpdateOne/Update.ApplyDomain` |
|---|---|---|---|---|
| `IDs()` | `PostIDs []int` | from `Edges.Posts` | `AddPostIDs` (if len > 0) | replace by default |
| `Nest()` | `Posts []Post` | from `Edges.Posts` | skipped | skipped |
| `IDs(), Nest()` | both | from `Edges.Posts` | `AddPostIDs` only | replace by default |

Nested edge mutations (create/update of child entities) are intentionally left to the repository adapter layer.

---

### Example Usage in Repository Adapter

```go
// adapter/repository/user_repo.go
// This layer owns ent — domain package has zero knowledge of it

func (r *UserRepo) GetByID(ctx context.Context, id int) (*domain.User, error) {
    u, err := r.client.User.
        Query().
        Where(user.ID(id)).
        WithPosts(). // eager load if needed
        Only(ctx)
    if err != nil {
        return nil, err
    }
    return u.ToDomain(), nil
}

func (r *UserRepo) Create(ctx context.Context, u *domain.User) (*domain.User, error) {
    created, err := r.client.User.
        Create().
        ApplyDomain(u).
        Save(ctx)
    if err != nil {
        return nil, err
    }
    return created.ToDomain(), nil
}

func (r *UserRepo) Update(ctx context.Context, u *domain.User) (*domain.User, error) {
    updated, err := r.client.User.
        UpdateOneID(u.ID).
        ApplyDomain(u,
            entdomain.OnlyFields(ent.UserDomainFieldName, ent.UserDomainFieldStatus),
        ).
        Save(ctx)
    if err != nil {
        return nil, err
    }
    return updated.ToDomain(), nil
}

func (r *UserRepo) CreateBulk(ctx context.Context, users domain.UserList) (domain.UserList, error) {
    return r.client.User.
        CreateBulkDomain(users).
        Save(ctx) // returns ([]*ent.User, error); caller calls ToDomain on each
}

func (r *UserRepo) UpdateBulk(ctx context.Context, users domain.UserList) (domain.UserList, error) {
    return r.client.User.
        UpdateBulkDomain(users).
        Save(ctx) // returns (domain.UserList, error)
}

func (r *UserRepo) DeactivateAll(ctx context.Context) error {
    return r.client.User.
        Update().
        ApplyDomain(
            &domain.User{Status: domain.UserStatusInactive},
            entdomain.OnlyFields(ent.UserDomainFieldStatus),
        ).
        Where(user.StatusEQ(user.StatusActive)).
        Exec(ctx)
}
```

---

## Consequences

### Benefits
- Domain layer has **zero dependency on ent or any ORM** — fully portable and testable in isolation
- Mapping logic is **auto-generated and stays in sync** with schema changes — no manual drift
- `ApplyDomain` fits naturally into **ent's existing builder/fluent API**
- Opt-in via annotations — **no impact on existing ent users** who don't adopt `entdomain`
- Typed field constants provide **IDE autocomplete and refactor safety** for apply options
- Bulk helpers eliminate boilerplate loops in repository adapters
- Consistent pattern across all entities reduces boilerplate in repository adapters
- Upsert `ApplyDomain` is **zero-config** — auto-detected from `gen.Config.Features`; the upsert story is complete with no additional annotation or config option

### Trade-offs
- Virtual fields and nested edges require **manual hydration** after `ToDomain()` — caller must know what was eagerly loaded
- Optional non-nillable fields (`field.Optional()` without `.Nillable()`) always produce a non-nil pointer in `ToDomain()` — the zero value of the ent field becomes `&""`, `&0`, etc.
- `ApplyDomain` on create with `OmitZeroVal` can **silently skip intentional zero values** — developer must be aware
- Transformer interface requires **app-startup wiring** — easy to forget in tests or secondary binaries
- Nested edge mutations remain **fully manual** — no generation support, by design
- `(*UserUpdate).ApplyDomain` does not call virtual field transformer hooks — transformer setters are typed to `*UserUpdateOne`
- Upsert `ApplyDomain` **cannot set edge IDs or call transformers** — ent's `*EntityUpsert` type only exposes scalar/enum setters; these constraints are inherent to the ent upsert API and not a design choice of entdomain

### Out of Scope
- Repository interface generation (managed manually by developers)
- Nested edge create/update in mapper (owned by repository adapter layer)
- Use case / service layer generation

## History

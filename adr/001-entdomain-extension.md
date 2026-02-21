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

## Decision

Introduce a new ent extension — `entdomain` — that generates a pure Go domain package and mapping helpers from ent schema definitions, controlled via schema annotations.

---

## Design

### Extension Configuration

```go
entdomain.NewExtension(
    entdomain.WithPackagePath("internal/domain"), // output path for domain package
    entdomain.WithPackageName("domain"),           // generated package name
)
```

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

// Virtual fields — arbitrary Go types via GoType(pkgPath, typeName)
// pkgPath: full import path; empty string means local/stdlib, no import added.
// typeName: Go type name as-is; prefix with "*" for pointer types.
// Generator emits the type name directly, prepending the package qualifier after any "*".
entdomain.VirtualField("amount",   entdomain.GoType("", "Money"))                               // → Money
entdomain.VirtualField("tags",     entdomain.GoType("", "[]string"))                            // → []string
entdomain.VirtualField("metadata", entdomain.GoType("", "map[string]any"))                      // → map[string]any
entdomain.VirtualField("price",    entdomain.GoType("github.com/shopspring/decimal", "Decimal")) // → decimal.Decimal
entdomain.VirtualField("price2",   entdomain.GoType("github.com/shopspring/decimal", "*Decimal"))// → *decimal.Decimal
entdomain.VirtualField("ext_id",   entdomain.GoType("github.com/google/uuid", "UUID"))          // → uuid.UUID
entdomain.VirtualField("opt_ref",  entdomain.GoType("", "*Money"))                              // → *Money
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
    Amount    Money            // entdomain.GoType("", "Money")
    Tags      []string         // entdomain.GoType("", "[]string")
    Metadata  map[string]any   // entdomain.GoType("", "map[string]any")
}
```

### Generated Mapping Methods (`ent/domain.go`)

Three methods are generated per entity on the ent-generated types. All entities are emitted into a single `ent/domain.go` file.

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

    // Profile (nested) — not applied, manage edge in repository layer

    // Transformer hook — each function field is nil-checked individually
    if UserTransformer != nil && UserTransformer.SetFullNameOnCreate != nil {
        UserTransformer.SetFullNameOnCreate(c, d.FullName)
    }

    return c
}
```

#### 3. Update — `*ent.UserUpdateOne ← *domain.User`

Same as create for scalars. IDs edges default to **replace** (desired final state semantics). Immutable fields are excluded. Nested edges and virtual fields are skipped unless a Transformer is wired.

```go
func (u *UserUpdateOne) ApplyDomain(d *domain.User, opts ...entdomain.ApplyOption) *UserUpdateOne {
    cfg := entdomain.NewApplyConfig(opts...)

    if cfg.ShouldApply("name", d.Name) {
        u = u.SetName(d.Name)
    }
    if cfg.ShouldApply("status", d.Status) {
        u = u.SetStatus(user.Status(d.Status))
    }

    // IDs edges — replace by default, opt-in append via AppendEdge option
    if cfg.IsAppendEdge("post_ids") {
        u = u.AddPostIDs(d.PostIDs...)
    } else {
        u = u.ClearPosts().AddPostIDs(d.PostIDs...)
    }

    // Profile (nested) — not applied, manage edge in repository layer

    // Transformer hook — each function field is nil-checked individually
    if UserTransformer != nil && UserTransformer.SetFullNameOnUpdate != nil {
        UserTransformer.SetFullNameOnUpdate(u, d.FullName)
    }

    return u
}
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
    // Type is inferred from the annotation: entdomain.String → string, entdomain.GoType("", "Money") → Money
    GetFullName  func(e *User) string
    GetIsPremium func(e *User) bool
    GetAmount    func(e *User) Money
    GetTags      func(e *User) []string

    // Virtual field setters (domain → ent)
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

| Edge annotation | Domain struct field | `ToDomain()` | `Create.ApplyDomain` | `UpdateOne.ApplyDomain` |
|---|---|---|---|---|
| `IDs()` | `PostIDs []int` | from `Edges.Posts` | `AddPostIDs` (if len > 0) | replace by default |
| `Nest()` | `Posts []Post` | from `Edges.Posts` | skipped | skipped |
| `IDs(), Nest()` | both | from `Edges.Posts` | `AddPostIDs` only | replace by default |

Nested edge mutations (create/update of child entities) are intentionally left to the repository adapter layer. The mapper emits a comment indicating this.

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
```

---

## Consequences

### Benefits
- Domain layer has **zero dependency on ent or any ORM** — fully portable and testable in isolation
- Mapping logic is **auto-generated and stays in sync** with schema changes — no manual drift
- `ApplyDomain` fits naturally into **ent's existing builder/fluent API**
- Opt-in via annotations — **no impact on existing ent users** who don't adopt `entdomain`
- Typed field constants provide **IDE autocomplete and refactor safety** for apply options
- Consistent pattern across all entities reduces boilerplate in repository adapters

### Trade-offs
- Virtual fields and nested edges require **manual hydration** after `ToDomain()` — caller must know what was eagerly loaded
- Optional non-nillable fields (`field.Optional()` without `.Nillable()`) always produce a non-nil pointer in `ToDomain()` — the zero value of the ent field becomes `&""`, `&0`, etc.
- `ApplyDomain` on create with `OmitZeroVal` can **silently skip intentional zero values** — developer must be aware
- Transformer interface requires **app-startup wiring** — easy to forget in tests or secondary binaries
- Nested edge mutations remain **fully manual** — no generation support, by design

### Out of Scope
- Repository interface generation (managed manually by developers)
- Nested edge create/update in mapper (owned by repository adapter layer)
- Use case / service layer generation

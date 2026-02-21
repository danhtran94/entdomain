# entdomain

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
        &gen.Config{},
        entc.Extensions(ex),
    ); err != nil {
        log.Fatalf("running ent codegen: %v", err)
    }
}
```

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
            entdomain.VirtualField("metadata", entdomain.GoType("", "map[string]any")),
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
entdomain.VirtualField("amount",   entdomain.GoType("", "Money"))                               // → Money
entdomain.VirtualField("tags",     entdomain.GoType("", "[]string"))                            // → []string
entdomain.VirtualField("metadata", entdomain.GoType("", "map[string]any"))                      // → map[string]any
entdomain.VirtualField("price",    entdomain.GoType("github.com/shopspring/decimal", "Decimal")) // → decimal.Decimal
entdomain.VirtualField("price2",   entdomain.GoType("github.com/shopspring/decimal", "*Decimal"))// → *decimal.Decimal
entdomain.VirtualField("ext_id",   entdomain.GoType("github.com/google/uuid", "UUID"))          // → uuid.UUID
entdomain.VirtualField("opt_ref",  entdomain.GoType("", "*Money"))                              // → *Money
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
    ID        int
    Name      string
    Bio       *string        // optional → pointer
    Status    UserStatus
    CreatedAt time.Time
    PostIDs   []int          // IDs edge
    Posts     []Post         // Nest edge
    FullName  string         // virtual field
    IsPremium bool           // virtual field
    Metadata  map[string]any // virtual field
}

// UserList is generated unless WithNoBulk is set for the entity.
type UserList []*User
```

### Mapping methods (`ent/domain.go`)

```go
// Read: ent → domain
func (e *User) ToDomain() *domain.User

// Create: domain → ent builder
func (c *UserCreate) ApplyDomain(d *domain.User, opts ...entdomain.ApplyOption) *UserCreate

// Update by ID: domain → ent builder
func (u *UserUpdateOne) ApplyDomain(d *domain.User, opts ...entdomain.ApplyOption) *UserUpdateOne

// Update by WHERE condition: domain → ent builder, chain .Where(...) after
func (u *UserUpdate) ApplyDomain(d *domain.User, opts ...entdomain.ApplyOption) *UserUpdate
```

### Bulk methods (`ent/domain.go`, unless `WithNoBulk` is set)

```go
// Slice mapper
func (es Users) ToDomain() domain.UserList

// Create bulk
func (c *UserClient) CreateBulkDomain(ds domain.UserList, opts ...entdomain.ApplyOption) *UserCreateBulk

// Update bulk by ID — mirrors UserCreateBulk API
func (c *UserClient) UpdateBulkDomain(ds domain.UserList, opts ...entdomain.ApplyOption) *UserUpdateOneBulk

func (b *UserUpdateOneBulk) Save(ctx context.Context) (domain.UserList, error)
func (b *UserUpdateOneBulk) SaveX(ctx context.Context) domain.UserList
func (b *UserUpdateOneBulk) Exec(ctx context.Context) error
func (b *UserUpdateOneBulk) ExecX(ctx context.Context)
```

### Typed field constants

```go
const (
    UserDomainFieldName    UserDomainField = "name"
    UserDomainFieldStatus  UserDomainField = "status"
    UserDomainFieldPostIDs UserDomainField = "post_ids"
    // ...
)
```

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
    GetFullName          func(e *User) string
    SetFullNameOnCreate  func(c *UserCreate, val string)
    SetFullNameOnUpdate  func(u *UserUpdateOne, val string)
    // one Get + two Set functions per virtual field
}

var UserTransformer *UserDomainTransformer // nil by default
```

Wire at app startup:

```go
ent.UserTransformer = &ent.UserDomainTransformer{
    GetFullName: func(u *ent.User) string {
        return u.FirstName + " " + u.LastName
    },
    // other functions left nil — skipped automatically
}
```

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
    created, err := r.client.User.Create().
        ApplyDomain(d).
        Save(ctx)
    if err != nil {
        return nil, err
    }
    return created.ToDomain(), nil
}

func (r *UserRepo) Update(ctx context.Context, d *domain.User) (*domain.User, error) {
    updated, err := r.client.User.UpdateOneID(d.ID).
        ApplyDomain(d,
            entdomain.OnlyFields(ent.UserDomainFieldName, ent.UserDomainFieldStatus),
        ).
        Save(ctx)
    if err != nil {
        return nil, err
    }
    return updated.ToDomain(), nil
}

func (r *UserRepo) CreateBulk(ctx context.Context, ds domain.UserList) (domain.UserList, error) {
    saved, err := r.client.User.CreateBulkDomain(ds).Save(ctx)
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
    return r.client.User.UpdateBulkDomain(ds).Save(ctx)
}

func (r *UserRepo) DeactivateAll(ctx context.Context) error {
    return r.client.User.Update().
        ApplyDomain(
            &domain.User{Status: domain.UserStatusInactive},
            entdomain.OnlyFields(ent.UserDomainFieldStatus),
        ).
        Where(user.StatusEQ(user.StatusActive)).
        Exec(ctx)
}
```

## Design Notes

- Nested edge mutations (creating/updating child entities) are intentionally not generated — manage them in the repository layer
- Virtual fields are always zero in `ToDomain()` unless a Transformer is wired; each transformer function field is nil-checked individually before calling
- Immutable ent fields are excluded from `UpdateOne.ApplyDomain` and `Update.ApplyDomain`
- Nested edges (`Nest()`) are excluded from `ApplyDomain` entirely; only `IDs()` edges are written
- `(*UserUpdate).ApplyDomain` does not call virtual field transformer hooks — transformer setters are typed to `*UserUpdateOne`
- `Optional()` fields without `.Nillable()` are stored as base types in ent (`string`, `int`, …) but mapped to pointers in the domain struct by taking their address — `&e.Bio`. The zero value is never nil in this case; use `.Nillable()` in the schema if you need nil-distinguishable optionals
- `WithNoBulk` is configured at extension level, not per schema — keeps schema annotations focused on domain shape, not generation policy

See [`adr/001-entdomain-extension.md`](adr/001-entdomain-extension.md) for the full design rationale.

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
func (c *UserCreate) ApplyDomain(d *domain.User, opts ...entdomain.ApplyOption) *UserCreate

// Update by ID: domain → ent builder
func (u *UserUpdateOne) ApplyDomain(d *domain.User, opts ...entdomain.ApplyOption) *UserUpdateOne

// Update by WHERE condition: domain → ent builder, chain .Where(...) after
func (u *UserUpdate) ApplyDomain(d *domain.User, opts ...entdomain.ApplyOption) *UserUpdate

// Upsert (generated only when gen.FeatureUpsert is enabled in gen.Config)
func (u *UserUpsertOne) ApplyDomain(d *domain.User, opts ...entdomain.ApplyOption) *UserUpsertOne
func (u *UserUpsertBulk) ApplyDomain(d *domain.User, opts ...entdomain.ApplyOption) *UserUpsertBulk // absent when NoBulk
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

// Upsert — requires gen.FeatureUpsert in gen.Config.Features
func (r *UserRepo) Upsert(ctx context.Context, d *domain.User) error {
    return r.client.User.Create().
        ApplyDomain(d).
        OnConflict(sql.ConflictColumns("username")).
        ApplyDomain(d).
        Exec(ctx)
}

func (r *UserRepo) UpsertBulk(ctx context.Context, ds domain.UserList) error {
    return r.client.User.CreateBulkDomain(ds).
        OnConflict(sql.ConflictColumns("username")).
        ApplyDomain(ds[0]). // same conflict resolution for all rows
        Exec(ctx)
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
    proto_helpers_gen.go   ← shared helpers (ToInt64Slice, etc.)
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

### Custom Proto Type for Virtual Fields

Virtual fields with a `GoType` that has no well-known proto mapping are silently excluded from proto output. Supply an explicit type to include them:

```go
entdomain.VirtualField("amount",
    entdomain.GoType("github.com/shopspring/decimal", "Decimal"),
    entdomain.ProtoType("google.type.Money", "google/type/money.proto"),
),
```

### Built-in Auto-Mappings

| entdomain / ent type | Proto type | Import |
|---|---|---|
| `entdomain.String` | `string` | — |
| `entdomain.Bool` | `bool` | — |
| `entdomain.Int` | `int64` | — |
| `entdomain.Float64` | `double` | — |
| `GoType("time", "Time")` | `google.protobuf.Timestamp` | `google/protobuf/timestamp.proto` |
| `GoType("time", "Duration")` | `google.protobuf.Duration` | `google/protobuf/duration.proto` |
| `GoType("github.com/google/uuid", "UUID")` | `string` | — |
| `field.Time(...)` | `google.protobuf.Timestamp` | `google/protobuf/timestamp.proto` |
| `field.UUID(...)` | `string` | — |
| `field.Enum(...)` | top-level enum | — |
| `field.JSON(...)` | **excluded** | — |

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

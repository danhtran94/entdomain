# ADR-003: FIQL Filter Generation

## Status

Implemented

## Context

After ADR-001 and ADR-002, entdomain covers the domain layer (Go structs + ent mappers) and the transport layer (proto messages + mappers). A common remaining need is **query filtering** — allowing API callers to express filter conditions that translate into ent predicates.

The key constraint is HTTP GET requests: filter expressions must be **URI-safe without percent-encoding**. This rules out JSON body filters (`{"name":"john"}` requires encoding) and SQL-like syntax (spaces and quotes require encoding).

**FIQL** (Feed Item Query Language, [IETF draft](https://tools.ietf.org/html/draft-nottingham-atompub-fiql)) satisfies this constraint. Its syntax uses only characters that are safe in URI query strings without encoding:

```
name==john;age=gt=25,status==active
```

- `;` = AND, `,` = OR, `(` `)` = grouping
- `==` `!=` `=gt=` `=lt=` `=ge=` `=le=` = comparison operators

FIQL was adopted by the Java/Spring ecosystem specifically for this URI-safety property. It is the most ergonomic option for GET-based REST filtering.

At code generation time, entdomain already knows everything needed to build FIQL support per entity:

- All field names and their Go/ent types
- The ent predicate functions per field (`user.NameEQ`, `user.AgeGT`, etc.)
- Which operators are type-valid (e.g. `GT` is invalid on a string field)
- Enum values and their string representations

---

## Assumptions

- **A1 [EXTERNAL FACT]:** FIQL is a URI-safe filter syntax suitable for HTTP GET query parameters. Evidence: [draft-nottingham-atompub-fiql-00](https://datatracker.ietf.org/doc/html/draft-nottingham-atompub-fiql-00).
- **A2 [VERIFIED]:** ent generates per-field predicate functions (`<Field>EQ`, `<Field>NEQ`, etc.) for every typed field at codegen time. Evidence: `examples/basic/ent/user/where.go` contains the full predicate function set.
- **A3 [HYPOTHESIS]:** Per-field opt-in via schema annotation gives the right safety/usability balance — sensitive fields are never accidentally filterable. Verification deferred — observed usage in `examples/basic/ent/schema/user.go` matches the intent (e.g. `password_hash` has no FIQL annotation and is never wired into the registry).

## Options

Three approaches were weighed before adopting first-party FIQL codegen:

- **Adopt an existing FIQL Go library** — e.g. `github.com/jirenius/go-rsql` or similar; let it parse expressions and produce ent predicates via reflection.
- **GraphQL-style filter input objects** — Generate typed filter structs per entity (`UserWhereInput`); expose them via a Go API or HTTP body.
- **First-party FIQL generator + parser (chosen)** — entdomain owns the parser, the per-entity registry, and the ent-predicate wiring.

## Options Comparison

| Driver | External library | GraphQL-style | First-party (chosen) |
|---|---|---|---|
| URI-safe (HTTP GET) | Yes | No (POST body) | Yes |
| Per-field opt-in | Manual | Manual | Annotation-driven |
| Type safety at codegen | None | Strong | Strong |
| Runtime dependencies | Heavy | Generated | Self-contained |

## Decision

Extend entdomain with **opt-in FIQL filter generation** that:

1. Allows individual fields to declare FIQL support with explicit operator allowlists via schema annotation
2. Generates a typed field registry per entity mapping field names to ent predicate builders
3. Generates a `EntityFIQL(expr string) (predicate.Entity, error)` entry point per entity
4. Ships a generic FIQL parser in the entdomain runtime that the generated code uses

FIQL is **opt-in per field** — no field is filterable unless explicitly annotated. This is the safe default: sensitive fields (e.g. `password_hash`, `internal_notes`) are never accidentally exposed to callers.

---

## Design

### Schema Annotation

Fields opt into FIQL by adding a `FIQL(...)` option to `entdomain.Field(...)`, listing the allowed operators explicitly:

```go
func (User) Fields() []ent.Field {
    return []ent.Field{
        field.String("name").
            Annotations(entdomain.Field(
                entdomain.FIQL(entdomain.EQ, entdomain.NEQ, entdomain.Contains),
            )),

        field.Int("age").
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

Operators are defined as typed constants in entdomain. The generator validates at code generation time that each operator is compatible with the field's type — a generation error (not a runtime error) if mismatched:

| Constant | FIQL syntax | Valid for |
|---|---|---|
| `EQ` | `==` | all types |
| `NEQ` | `!=` | all types |
| `GT` | `=gt=` | int, float, time |
| `LT` | `=lt=` | int, float, time |
| `GTE` | `=ge=` | int, float, time |
| `LTE` | `=le=` | int, float, time |
| `Contains` | `=like=` | string |
| `HasPrefix` | `=prefix=` | string |

### Generated Code

For each entity with at least one FIQL-annotated field, entdomain generates:

**Field registry** — maps FIQL field names to typed predicate builders:

```go
// Code generated by entdomain, DO NOT EDIT.

var UserFIQLFields = entdomain.FIQLFields[predicate.User]{
    "name": entdomain.FIQLString[predicate.User]{
        EQ:       user.NameEQ,
        NEQ:      user.NameNEQ,
        Contains: user.NameContains,
    },
    "age": entdomain.FIQLInt[predicate.User]{
        EQ:  user.AgeEQ,
        GT:  user.AgeGT,
        LT:  user.AgeLT,
        GTE: user.AgeGTE,
        LTE: user.AgeLTE,
    },
    "status": entdomain.FIQLEnum[predicate.User]{
        EQ: map[string]predicate.User{
            "active":   user.StatusEQ(user.StatusActive),
            "inactive": user.StatusEQ(user.StatusInactive),
        },
        NEQ: map[string]predicate.User{
            "active":   user.StatusNEQ(user.StatusActive),
            "inactive": user.StatusNEQ(user.StatusInactive),
        },
    },
    "created_at": entdomain.FIQLTime[predicate.User]{
        GTE: user.CreatedAtGTE,
        LTE: user.CreatedAtLTE,
    },
}
```

**Entry point function** — wires the parser to the registry:

```go
// Code generated by entdomain, DO NOT EDIT.

func UserFIQL(expr string) (predicate.User, error) {
    return entdomain.ParseFIQL(expr, UserFIQLFields)
}
```

### Runtime Library (not generated)

The entdomain package ships a generic FIQL parser and typed field helpers. None of this is entity-specific:

```go
// entdomain runtime:

// Predicate constrains P to ent predicate types (all are ~func(*sql.Selector)).
type Predicate interface { ~func(*sql.Selector) }

// ParseFIQL parses a FIQL expression and returns an ent predicate using the
// provided field registry. Returns an error for unknown fields, disallowed
// operators, or malformed expressions.
func ParseFIQL[P Predicate](expr string, fields FIQLFields[P]) (P, error)

// FIQLFields is a map of field name → field descriptor.
type FIQLFields[P Predicate] map[string]FIQLField[P]

// FIQLField is implemented by FIQLString, FIQLInt, FIQLFloat, FIQLEnum, FIQLTime, FIQLBool.
type FIQLField[P Predicate] interface {
    apply(op FIQLOp, value string) (P, error)
}

type FIQLString[P Predicate] struct { EQ, NEQ func(string) P; Contains, HasPrefix func(string) P }
type FIQLInt[P Predicate]    struct { EQ, NEQ func(int) P; GT, LT, GTE, LTE func(int) P }
type FIQLFloat[P Predicate]  struct { EQ, NEQ func(float64) P; GT, LT, GTE, LTE func(float64) P }
type FIQLTime[P Predicate]   struct { EQ, NEQ func(time.Time) P; GT, LT, GTE, LTE func(time.Time) P }
type FIQLBool[P Predicate]   struct { EQ func(bool) P }
type FIQLEnum[P Predicate]   struct { EQ, NEQ map[string]P }
```

### Usage

```go
// HTTP handler:
func (h *UserHandler) List(w http.ResponseWriter, r *http.Request) {
    expr := r.URL.Query().Get("filter")

    q := h.client.User.Query()
    if expr != "" {
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

GET request:
```
GET /users?filter=name==john;age=gt=25,status==active
```

No percent-encoding required.

### Error Handling

The parser returns descriptive errors for invalid input:

```
unknown field "email" on User — annotate with entdomain.FIQL(...) to enable
operator "=gt=" not allowed on field "name" (string) — allowed: ==, !=, =like=
unknown enum value "pending" for field "status" — valid values: active, inactive
invalid value "abc" for field "age" (int): strconv.Atoi: parsing "abc": invalid syntax
```

---

## Scope

### In scope

- Scalar fields: string, int, float, bool, time
- Enum fields: EQ/NEQ with string→enum value mapping
- Optional fields: same operators, nil treated as non-match
- Logical operators: AND (`;`), OR (`,`), grouping (`(` `)`)

### Out of scope (future)

- UUID fields — ent UUID predicates require `uuid.UUID`, not `string`; a dedicated `FIQLUUID` type with `uuid.Parse` would be needed
- Edge fields (e.g. `owner.name==john`) — requires joins, deferred
- JSON/struct fields — no reliable SQL predicate mapping
- Virtual fields — no DB column, cannot become ent predicates
- Sorting (`sort=name,-age`) — separate concern, separate annotation

---

## Consequences

**Good:**
- URI-safe filter expressions — no percent-encoding in GET requests
- Secure by default — fields must be explicitly annotated to be filterable
- Operator validation at generation time — type mismatches caught before runtime
- Self-documenting schema — filterable fields and allowed operators visible in schema definition
- Consistent pattern with existing `entdomain.Field(...)` + `SkipProto()` design

**Bad:**
- Requires a FIQL parser in the entdomain runtime (small but non-zero new dependency surface)
- `gen.Config.Package` must be set manually when schema is outside the ent directory (existing limitation, documented)
- FIQL is niche — developers unfamiliar with it need to learn the syntax

---

## Implementation Notes

- **Files added**: `fiql.go` (runtime), `template/fiql.tmpl` (code generator)
- **Files modified**: `annotation.go` (FIQLOps field + FIQL constructor), `template.go` (FIQLTemplate + template funcs), `extension.go` (Templates method)
- **Generated file**: `ent/fiql.go` — one `EntityFIQLFields` var + one `EntityFIQL()` function per entity with FIQL-annotated fields; absent if no entity qualifies
- **Predicate combining**: uses `sql.AndPredicates[P]` / `sql.OrPredicates[P]` from ent v0.14.5 — generic helpers that accept any `P ~func(*sql.Selector)`
- **Operator precedence**: `;` (AND) binds tighter than `,` (OR), matching standard FIQL spec; parentheses override precedence
- **Nesting depth limit**: max 50 levels — returns error `"maximum nesting depth exceeded"` to prevent stack overflow from adversarial input
- **Empty value rejection**: `name==` returns `"empty value not allowed for field"` error
- **UUID fields**: mapped to `""` (unsupported) in the generator — skipped silently; ent's UUID predicates require `uuid.UUID` not `string`
- **Time values**: must be RFC3339 (`2006-01-02T15:04:05Z07:00`); `time.Parse` error surfaces as `"invalid time value"`
- **Enum maps**: pre-built at generation time — `map[string]predicate.Entity{"active": user.StatusEQ(user.StatusActive), ...}` — no runtime string→enum lookup
- **Table-qualified columns**: ent predicates produce `` `tablename`.`column` `` qualified names when a table is set on the selector (typical in multi-table queries)

## History

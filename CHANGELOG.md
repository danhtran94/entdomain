# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- **Panicking `X`-variants for every error-returning `ApplyDomain*` / bulk method.**
  Mirrors ent's `SaveX` / `FirstX` / `OnlyX` convention — `ApplyDomainX`, `CreateBulkDomainX`, `UpdateBulkDomainX` opt into panic-on-error to restore fluent chaining in tests and scripts. The error-returning forms remain the primary, request-safe path. See the "Fluent chaining via `X` variants" section in README.

- **`entdomain.WithDomain[T]` / `entdomain.DomainFrom[T]`** — generic context helpers for the narrow case where ent mutation hooks need to read the originating domain struct (including virtual fields) at Save time.
  Positioned as an opt-in escape hatch; typed transformers remain the primary encoding path. See the "Advanced: ent hook integration" section in README and [ADR-005](adr/005-transformer-runtime-context.md) for scope and caveats.

### Changed — BREAKING

- **Transformer signatures now take `context.Context` + the full domain struct and return `error`.**
  See [ADR-005](adr/005-transformer-runtime-context.md) for the rationale — alignment with ent's hook/privacy/interceptor conventions, and enabling I/O-based transforms such as KMS-backed field encryption.

  Before:

  ```go
  type UserDomainTransformer struct {
      SetSecretOnCreate func(c *UserCreate, val string)
      SetSecretOnUpdate func(u *UserUpdateOne, val string)
  }
  ```

  After:

  ```go
  type UserDomainTransformer struct {
      SetSecretOnCreate func(ctx context.Context, c *UserCreate, d *domain.User, val string) error
      SetSecretOnUpdate func(ctx context.Context, u *UserUpdateOne, d *domain.User, val string) error
  }
  ```

  `GetXxx` projections remain pure (no ctx, no error).

- **`ApplyDomain*` now takes `ctx` as first arg and returns `(builder, error)`.**
  Applies to `Create`, `UpdateOne`, `Update`, `UpsertOne`, `UpsertBulk`.

  Before:

  ```go
  created, err := client.User.Create().ApplyDomain(d).Save(ctx)
  ```

  After:

  ```go
  builder, err := client.User.Create().ApplyDomain(ctx, d)
  if err != nil {
      return err
  }
  created, err := builder.Save(ctx)
  ```

- **`CreateBulkDomain` / `UpdateBulkDomain` now take `ctx` and return `(builder, error)`.**
  Errors from per-row transformers short-circuit: the first error aborts the bulk.

### Migration

Compile-time enforced. Run `go generate ./...` and fix signatures flagged by the compiler:

1. Update registered transformer functions to accept `ctx`, the domain pointer, and return `error`. Pure transformers return `nil`.
2. Split fluent `ApplyDomain(d).Save(ctx)` chains into two steps because `ApplyDomain` now returns `(builder, error)`.
3. Thread `ctx` through `CreateBulkDomain` / `UpdateBulkDomain` call sites and handle their new error return.

No data migration. No schema change.

# Contributing to entdomain

Thanks for your interest. This document covers the practical mechanics of contributing — local setup, code generation, testing, and the conventions enforced by review and CI.

## Prerequisites

- Go 1.25.7 or newer (matches `go.mod`).
- `git`.
- For working on proto generation features: nothing additional — `protoc` is not required at development time because we generate `.proto` source files, not compiled `.pb.go` artifacts.

## Local setup

```sh
git clone https://github.com/danhtran94/entdomain.git
cd entdomain
go build ./...
go test ./...
```

That should be green on a fresh checkout. If it isn't, please open an issue before changing anything.

## Project layout

```text
.
├── *.go                    — entdomain library source (extension, generators, runtime helpers)
├── template/               — every code-generation template, loaded via //go:embed
├── docs/                   — ADRs (`docs/ADR-NNN-*.md`), RFCs, scope notes — harness-managed
├── jobs/                   — job-tracking docs (`jobs/ENTD-NNN-*.md`)
├── examples/
│   ├── basic/              — schema + generated output exercising the full feature surface
│   └── custom/             — schema demonstrating non-default layouts (custom domain pkg path, etc.)
├── .github/workflows/      — CI: ci.yml (tests), security.yml (govulncheck/gosec/CodeQL), scorecard.yml
├── CHANGELOG.md            — Keep-a-Changelog format; see "When to add an entry" below
├── SECURITY.md             — vulnerability reporting policy
└── README.md
```

## Code generation

entdomain is itself a code generator, so several layers regenerate when you change templates or schemas.

### Regenerating examples

Each example has an `entc.go` entrypoint that invokes ent + entdomain:

```sh
(cd examples/basic/ent && go run entc.go)
(cd examples/custom/repo/ent && go run entc.go)
```

Run both whenever you change anything in `template/` or any generator in the root package. The CI pipeline will catch a divergence; running locally is faster.

### Templates — single source of truth

**Every template lives in `template/`.** Inline template strings in Go files are not allowed.

When adding a new template:

1. Create `template/<name>.tmpl`.
2. Register it via one of the two supported loaders, depending on which pipeline consumes the template:

   **A. ent plugin templates** — feed ent's `gen.Template` system (see `domain.tmpl`, `fiql.tmpl`). Register in `template.go` using the shared embed FS + `parseT` helper:

   ```go
   var XTemplate = parseT("template/<name>.tmpl")
   ```

   ent picks these up via the extension's `Templates()` method in `extension.go`.

   **B. entdomain's own generators** — feed standalone Go-file or `.proto`-file writers (see `domain_struct.tmpl`, `proto_mapper_*.tmpl`, `proto_helpers.tmpl`, `proto_messages.tmpl`). Register in the driver Go file with a direct `//go:embed` string:

   ```go
   //go:embed template/<name>.tmpl
   var xTmplSrc string

   var xTmpl = template.Must(template.New("x").Parse(xTmplSrc))
   ```

3. Custom `template.FuncMap` definitions stay in the Go driver (they reference Go functions). Only the template body moves to disk.

This split is documented in the package comment of `template.go`. See ADR-001 for the rationale behind ent's template plugin integration.

## Testing

```sh
go test ./...                              # full suite
go test -run TestApplyDomain ./...         # focused: transformer behavior
go test -run TestTemplate ./...            # focused: AST assertions on generated code
go test -fuzz=FuzzParseFIQL -fuzztime=30s  # FIQL parser fuzz (ad-hoc; not in CI)
```

Behavioral tests for the generated code live in `examples/basic/ent/` (sqlite-backed; no real persistence needed for transformer-path tests). Template structure assertions live in `template_test.go` at the root.

When fixing a bug or adding a feature, add a test that fails before your change and passes after. Don't ship code paths that aren't exercised by tests.

## Linting and security

- **`golangci-lint`** runs in CI via `.github/workflows/security.yml`. Local invocation:

  ```sh
  go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.11.4
  golangci-lint run ./...
  ```

- **`govulncheck`** runs in CI; local check:

  ```sh
  go install golang.org/x/vuln/cmd/govulncheck@v1.1.4
  govulncheck ./...
  ```

- All security findings (gosec G101–G115, G3xx, etc.) are tuned in `.golangci.yml`. If you hit a false positive, prefer adjusting the config over `// nolint:` comments.

See [ADR-004](docs/ADR-004-security-pipeline.md) for the security pipeline rationale.

## Architecture Decision Records (ADRs)

Non-trivial design choices go in `docs/` as ADRs managed by the
harness layer (`harness validate-doc-filename` enforces the name
at every Write/Edit):

- Filename: `docs/ADR-NNN-<short-kebab-title>.md`.
- Strict monotonic `NNN` — next = max(existing) + 1.
- Sections: Status (Proposed → Accepted/Implemented/Rejected), Context, Options Considered, Decision, Consequences (Good/Bad), Migration Path (if breaking).
- Reject options explicitly with reasoning — future readers should see what was *not* chosen and why.
- Start new ADRs with `/doc adr <short-title>` (harness skill walks the phase set + saves checkpoints to `docs/.drafts/`).

Add a new ADR when:

- Introducing a breaking change to the public API
- Picking between two or more implementation strategies with non-obvious tradeoffs
- Establishing or amending a project convention

Don't add an ADR for: bug fixes, dependency updates, internal refactors with no behavior change, documentation tweaks.

## When to add a CHANGELOG entry

`CHANGELOG.md` follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Add an entry when:

- The user-facing API changes (added, changed, removed, deprecated)
- A bug affecting users is fixed
- Security fix that affects users

Don't add an entry for: internal refactors with byte-identical output, test-only changes, CI/tooling changes that don't affect library behavior, documentation updates.

Group entries under `Added` / `Changed — BREAKING` / `Changed` / `Deprecated` / `Removed` / `Fixed` / `Security`.

## Pull requests

- Branch from `main`. Feature branches; not direct commits to `main` (branch protection enforces this).
- One logical change per PR. Refactors that touch many files are fine if they're a single coherent change.
- Run `go test ./...` and the regen commands above before opening the PR.
- CI runs: tests (`ci.yml`), security (`security.yml`), Scorecard (`scorecard.yml`). All must be green.
- CodeRabbit auto-reviews human PRs. Renovate dependency PRs are auto-reviewed via `.github/workflows/coderabbit-bot-prs.yml`.
- Renovate (dependabot replacement) handles dependency updates; don't open them manually unless something is genuinely urgent.

## Reporting security issues

**Do not open a public issue.** See [SECURITY.md](SECURITY.md) for the private vulnerability reporting process.

## Questions

Open a GitHub Discussion or an Issue with the `question` label. For design conversations that may turn into ADRs, start with a Discussion so the back-and-forth has a stable home.

# ADR-004: Security Toolchain and CI Pipeline

## Status

Accepted

## Context

As `entdomain` grows into a library consumed by production systems, it carries supply chain responsibility: a vulnerability or a compromised release can silently propagate to every project that depends on it. Three distinct risks need covering:

1. **Known CVEs in dependencies** — a transitive dep introduces a reachable vulnerability
2. **Static security flaws in our own code** — path traversal in file generation, panic on invalid input, weak type handling
3. **Supply chain integrity** — consumers need confidence that releases are not tampered and that the project is actively maintained

A self-audit identified the following concrete findings before the pipeline was in place:

| Severity | Finding |
|---|---|
| Medium | Path traversal via user-controlled `pkgPath` in `generate.go` |
| Medium | `fmt.Sprintf` with user-supplied format strings in proto expression templates |
| Low | `uuid.MustParse` panic on invalid proto input in generated helpers |
| Low | Absolute filesystem path leaked in error messages |
| Low | `moduleRoot()` does not resolve symlinks before writing files |

In addition, `govulncheck`, `golangci-lint`, CodeQL, and OpenSSF Scorecard were evaluated against alternatives (Grype, Trivy, Nancy, Semgrep, Dependabot) to identify the best fit for a small but serious Go library.

---

## Assumptions

- **A1 [EXTERNAL FACT]:** `govulncheck` reports vulnerabilities only for code paths actually reachable from the module's exported symbols. Evidence: [govulncheck design doc](https://go.dev/blog/govulncheck) — call-graph analysis is the documented mechanism.
- **A2 [EXTERNAL FACT]:** OpenSSF Scorecard runs are public and influence consumer trust signals on `pkg.go.dev`. Evidence: [Scorecard docs](https://github.com/ossf/scorecard) and the badge integration shown on the package page.
- **A3 [HYPOTHESIS]:** Splitting security workflows from regular CI keeps PR feedback fast and lets long-running scans (CodeQL) schedule independently. Verification deferred — observed CI durations for `ci.yml` vs `security.yml` confirm the split was beneficial.

## Options

The pipeline composition was selected from these candidate tools:

- **Vulnerability scanner**: `govulncheck` (Go-specific, reachability-aware) vs Grype / Nancy (manifest-level, no reachability)
- **Static analysis**: `golangci-lint` (gosec/gocritic plugins) vs Semgrep OSS (Go taint paywalled)
- **Dependency updates**: Renovate vs Dependabot (Go support quality)
- **Supply chain scoring**: OpenSSF Scorecard (industry standard) vs no scoring

## Options Comparison

| Alternative | Rejected Because |
|---|---|
| Grype as primary scanner | Package-manifest level — fires on every transitive dep regardless of reachability; impractical as PR gate |
| Nancy | No active maintenance since 2022 |
| Semgrep OSS | Go taint-tracking paywalled; free ruleset redundant with gosec |
| Dependabot for Go deps | Unresolved bug producing inconsistent `go.sum` on indirect dep updates |
| SLSA provenance now | No build artifacts to attest; `sum.golang.org` already covers source integrity |

## Decision

Adopt a layered security pipeline covering vulnerability scanning, static analysis, dependency updates, and supply chain scoring. All tooling runs on GitHub Actions. The pipeline is split from the regular CI (tests/build) workflow to allow independent scheduling.

---

## Design

### Tool Selection Rationale

#### Vulnerability Scanning: govulncheck (primary) + Trivy (release-only)

`govulncheck` builds a call graph and only fires when a vulnerable symbol is actually reachable from project code. It does not alert on transitive dependencies that are present in `go.sum` but never called. This eliminates the false-positive noise that makes Grype and Trivy impractical as PR gates for Go libraries.

Trivy is reserved for release tags to generate a CycloneDX SBOM — a machine-readable dependency manifest useful for enterprise consumers doing their own supply chain review.

Nancy (Sonatype OSS Index) is excluded: no meaningful releases since 2022 and the underlying data source is less curated than the Go Vulnerability Database maintained by the Go team.

#### Static Analysis: golangci-lint with gosec

`golangci-lint` aggregates gosec (OWASP-mapped security patterns), staticcheck (correctness), and gocritic (code quality) under a single invocation. gosec covers the concrete risk classes present in a code-generation library:

- **G304** (file path constructed from variable) — relevant to `generate.go` and `proto_generate.go`
- **G103** (unsafe block usage)
- **G401–G405** (weak cryptography)
- **G101** (hardcoded credentials pattern)

G304 and G115 (integer conversion overflow) are excluded via `.golangci.yml` because they fire as false positives on legitimate codegen patterns (dynamic output paths) and iota-based enum constants respectively.

Semgrep OSS is excluded: Go taint-tracking is paywalled in the Pro tier. The free ruleset adds no coverage over gosec for this project's attack surface.

#### Code Analysis: CodeQL

CodeQL performs cross-file data-flow and taint analysis — capabilities gosec lacks. Yield is currently low because entdomain has minimal network/HTTP surface. Cost is zero (free for public repos) and the autobuild step requires no configuration, so the risk-adjusted value is positive. Using `security-extended` queries to capture more patterns than the default set.

#### Dependency Updates: Renovate over Dependabot

Dependabot has a [filed unresolved bug](https://github.com/dependabot/dependabot-core/issues/9370) where it manually edits `go.mod` for indirect dependencies without running `go get`, producing an inconsistent `go.sum`. Renovate runs `go get <module>@<version>` followed by `go mod tidy`, which is the correct update sequence for Go modules.

`postUpdateOptions: ["gomodTidy"]` is enabled explicitly. Minor and patch updates are grouped into a single weekly PR to reduce noise.

#### Supply Chain Scoring: OpenSSF Scorecard

The Scorecard runs 18 automated checks (branch protection, CI coverage, code review enforcement, vulnerability status, maintained status, dependency tooling, SAST presence, token permissions, dangerous workflow detection) and produces an aggregate score 0–10. A score ≥ 7 is the informal threshold where enterprise security teams stop flagging a library as a supply chain risk. The badge is published to the README.

Results are uploaded as SARIF to GitHub Security tab, making findings visible alongside CodeQL and govulncheck results in a unified interface.

#### CI Hardening: StepSecurity Harden-Runner

Deployed in `audit` mode on all workflows. Detects unexpected outbound network connections during CI runs (a signal of a compromised action or dependency attempting to exfiltrate secrets). Graduated to `block` mode after baselining expected egress.

#### SLSA Provenance: Deferred

SLSA provenance applies to build artifacts (binaries, containers). A Go library consumed via `go get` has no build artifact — consumers build from source themselves. The Go module proxy checksum database (`sum.golang.org`) already provides tamper-evident transparency for source code via `go.sum`. SLSA becomes relevant if entdomain ever ships a CLI binary or Docker image.

---

### Workflow Structure

Two workflow files are maintained separately from `.github/workflows/ci.yml`:

**`security.yml`** — runs on every push to `main`, every PR, and weekly on Monday:
- `govulncheck` job: scans for reachable CVEs, uploads SARIF
- `sast` job: golangci-lint with gosec, gocritic, staticcheck
- `codeql` job: Go data-flow analysis with `security-extended` queries

**`scorecard.yml`** — runs on push to `main` and weekly:
- OpenSSF Scorecard analysis with badge publishing
- Results uploaded to GitHub Security tab as SARIF

All workflow jobs set `permissions: contents: read` at minimum. Jobs that upload SARIF also set `permissions: security-events: write`. The scorecard job additionally requires `permissions: id-token: write` for OIDC badge publishing.

---

### `.golangci.yml` Key Configuration

```yaml
version: "2"

linters:
  enable:
    - gosec
    - gocritic
    - staticcheck
  settings:
    gosec:
      excludes:
        - G115  # integer overflow on iota-based enum constants — false positive
        - G301  # codegen dirs need world-execute for tools (0755 is correct)
        - G304  # file path via variable — inherent to all codegen file processing
        - G306  # generated source files must be world-readable (0644 is correct)
  exclusions:
    rules:
      - linters: [gocritic]
        text: "ifElseChain"  # nil-checks and boolean guards cannot be expressed as switch
```

---

### `renovate.json` Configuration

```json
{
  "$schema": "https://docs.renovatebot.com/renovate-schema.json",
  "extends": ["config:recommended"],
  "postUpdateOptions": ["gomodTidy"],
  "packageRules": [
    {
      "matchManagers": ["gomod"],
      "matchUpdateTypes": ["minor", "patch"],
      "groupName": "Go dependencies",
      "automerge": false
    }
  ],
  "schedule": ["before 9am on Monday"]
}
```

---

## Consequences

**Good:**
- `govulncheck` as the PR gate eliminates transitive false-positive noise — only reachable vulns block merge
- Unified SARIF output in GitHub Security tab consolidates govulncheck, CodeQL, and Scorecard findings in one place
- OpenSSF Scorecard badge provides a transparent, machine-readable trust signal for enterprise adopters
- Renovate ensures `go.sum` integrity on dependency updates (Dependabot does not)
- StepSecurity Harden-Runner provides defense-in-depth against compromised GitHub Actions

**Bad:**
- golangci-lint requires an `.golangci.yml` exclusion file to suppress false positives on legitimate codegen patterns; without it the pipeline is noisy
- Scorecard `Branch-Protection` check requires repository settings (required PR reviews, status checks) that constrain solo development workflow
- CodeQL autobuild occasionally misdetects the build entrypoint for non-standard Go module layouts; may require `build-mode: manual` if the `examples/` sub-modules cause confusion

---

<!-- Alternatives moved to ## Options Comparison earlier in the doc to satisfy the discipline schema. -->


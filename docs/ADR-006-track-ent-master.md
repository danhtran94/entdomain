# ADR-006: Track ent master instead of the latest ent release tag

| Field      | Value                    |
|------------|--------------------------|
| Status     | Accepted                 |
| Date       | 2026-11-17               |
| Deciders   | danhtran94               |
| Scope      | dependencies, release    |

---

## Context

entdomain generates code through ent's `entc/gen` package, so the ent version in `go.mod` decides both the generator entdomain runs on and, through Go's minimum version selection, the ent version every consumer builds with.

The latest ent release, `v0.14.6`, is dated 2026-03-17. Commit `7306211` (2026-11-13) moved `go.mod` to the ent master pseudo-version `v0.14.7-0.20261014065818-8037bc89bcab` (2026-10-14), and releases `v1.6.0` and `v1.7.0` shipped with it. The decision was deliberate, but no document recorded it, and the release notes did not mention it.

Master is 10 commits ahead of `v0.14.6` (`gh api repos/ent/ent/compare/v0.14.6...8037bc89bcab`):

| Kind | Commits |
|---|---|
| Fix | context propagated to the `From` selector (#4501); row lock when a MySQL update matches nothing (#4524); go1.27 support by updating `golang.org/x/tools` to v0.52.0 (#4530) |
| Feature | top-level UNION / INTERSECT / EXCEPT (#4503, #4505, #4506); CTE builder accepts `Querier` (#4504); blob storage (#4500) |
| Codegen refactor | most mutation logic moved into per-type packages (#4493) |
| CI | cd workflow removed (#4508) |

Two of these matter most to entdomain:

- **#4530.** ent's generator loads packages with `golang.org/x/tools`, which breaks on new Go releases when it falls behind. The update to x/tools v0.52.0 is also why `go.mod` declares `go 1.26.0`.
- **#4493.** It changes the shape of every consumer's generated ent code: `mutation.go` shrinks by about 1,900 lines in `examples/basic` and the logic moves into the per-type packages. entdomain's own template output (`ent/domain.go`) is unchanged.

## Assumptions

- **A1 [VERIFIED]:** The pinned master commit is 10 commits ahead of `v0.14.6` and contains the commits listed above. Evidence: `gh api repos/ent/ent/compare/v0.14.6...8037bc89bcab` (2026-11-17); release and commit dates from `go list -m -json entgo.io/ent@<version>`.
- **A2 [VERIFIED]:** MVS passes the pin and the `go` directive to consumers. Evidence: a fresh module with `go 1.25.0` and `entgo.io/ent v0.14.6`, after `go get github.com/danhtran94/entdomain@v1.7.0`, resolved the ent pseudo-version and `go 1.26.0`, and `GOTOOLCHAIN=go1.25.7 go build` failed with `go.mod requires go >= 1.26.0`.
- **A3 [VERIFIED]:** entdomain's own template output does not depend on the master changes. Evidence: commit `7306211` regenerated every example; only ent-owned files changed, and `ent/domain.go` stayed identical.
- **A4 [HYPOTHESIS]:** Consumers value the ent fixes and Go 1.27 readiness more than control over their ent version. Verification deferred: no consumer feedback yet; revisit if an issue reports a forced upgrade.

## Options

### A. Track an ent master pseudo-version

Pin a specific master commit in `go.mod` and move it forward deliberately.

### B. Stay on the latest ent release tag

Keep `entgo.io/ent v0.14.6` and `go 1.25.x` until ent cuts a new tag.

### C. Release tag in `go.mod`, master in CI

Keep `v0.14.6` in `go.mod`; add a CI job that runs `go get entgo.io/ent@master && make gen && go test ./...` to catch generator breakage early without passing master to consumers.

## Options Comparison

| Driver | A. Track master | B. Release tag | C. Tag + master CI |
|---|---|---|---|
| Consumers get ent fixes (#4501, #4524) | yes | no | no |
| Generator works on new Go releases (#4530) | yes | no, until ent tags | no, until ent tags |
| Consumer controls its ent version | no: MVS raises ent and the `go` directive | yes | yes |
| Consumer generated code stays stable on upgrade | no: #4493 reshapes mutation code | yes | yes |
| Early warning of ent master breaking entdomain | yes, at bump time | no | yes, every CI run |
| Maintenance cost | bump the pin by hand | none | one CI job |

## Decision

Option A: entdomain tracks a pinned ent master pseudo-version until ent publishes a release that contains the needed fixes.

## Consequences

- **Consumers are upgraded without asking.** Adding entdomain to a module that pins `entgo.io/ent v0.14.6` raises it to the pseudo-version (`go: upgraded entgo.io/ent v0.14.6 => v0.14.7-0.20261014065818-8037bc89bcab`), and a `go 1.25.0` directive becomes `go 1.26.0`. A Go 1.25 toolchain then refuses to build the module (`go.mod requires go >= 1.26.0`). Both observed on a fresh consumer module, 2026-11-17.
- **Consumer generated code changes shape.** After upgrading, `go generate` in the consumer's ent directory splits mutation logic per type (#4493). The diff is large but mechanical; entdomain's `ent/domain.go` is unaffected.
- **The README states the requirement** under "Requirements", and every release note names the pinned ent commit.
- **The pin moves only on purpose.** Bump it by hand (`go get entgo.io/ent@<commit>`), then `make gen`, inspect the diff of `examples/*/ent`, and `make test`. When ent tags `v0.14.7`, semver orders it above the current `v0.14.7-0.…` pre-release, so it arrives as an ordinary patch update.

### Exit criterion

Return to a release tag (Option B) when ent publishes `v0.14.7` or later containing #4530. Revisit Option C if ent master starts to break entdomain's generator between bumps.

## History

| Ticket    | Title                                                           | Created | Status   |
|-----------|-----------------------------------------------------------------|---------|----------|
| (initial) | ADR-006: Track ent master instead of the latest ent release tag |         | Accepted |

---
change_id: ci-and-release-flow
title: CI gate on every PR, with production auto-deploy from main gated by CI
status: implemented
created: 2026-09-06
updated: 2026-09-06
archived_at: null
---

## Notes

The repo had no CI. Railway builds the Docker image and auto-deploys from `main`.
This change adds a CI gate and a light release convention. No staging environment
— skipped on purpose at current scale. One `production` environment only.

### Implementation status (2026-09-06)

- **Phase 1** (`ci.yml` + docs) — done, PR #18 merged to `main`; `ci` + `bench`
  green on `main`.
- **Phase 3** (commit-SHA footer) — done, PR #20 merged into `release/v1.0.0`
  (not `main`, per the model switch below), reached production with the
  `release/v1.0.0 → main` merge (PR #25).
- **Phase 2** (branch protection) — user's ruleset "master" (id 22382181)
  enforces PR + no-force-push + no-deletion on the default branch, a
  `required_status_checks` rule for `ci` (strict) on the default branch, and
  Railway "Wait for CI" is on. A dedicated `release/*` ruleset was considered
  and decided out of scope at solo-maintainer scale — release branches are
  disposable after their tag, and the `ci` gate on `main` already protects the
  only branch that deploys.
- **PR #21** (`docs/release-flow` → `release/v1.0.0`) — rewrote the README
  release section, added `push: release/**` to `ci.yml`; merged.
- **Closed out:** PR #21 merged; `release/v1.0.0 → main` merged (PR #25); the
  `WAITING` Railway deploy and the production footer SHA both confirmed on that
  merge; `v1.0.0` tagged (`CHANGELOG.md`).

### BRANCH MODEL CHANGED (2026-09-06, user-directed): trunk → release-branch

The decisions below say **trunk**. The user switched to a **release-branch**
model during implementation: feature branches PR into `release/vX.Y.Z`, which is
then batched into `main`; the merge to `main` still deploys, `vX.Y.Z` tags stay
as markers. First release branch is `release/v1.0.0`. `plan.md` Progress carries
the full deviation note. The "Release convention" and "Flow" sections below are
superseded on the branch-model point only.

### Purpose of CI

One workflow, `.github/workflows/ci.yml`, runs on every pull request and on every
push to `main`. All jobs must pass:

1. `go build ./...`
2. `templ generate`, then fail if any committed `*_templ.go` changed (stale check)
3. `gofmt` + `goimports` check (`golangci-lint fmt`; local-prefix already set in `.golangci.yml`)
4. `go vet ./...`
5. `golangci-lint run` (golangci-lint v2, matches `.golangci.yml`)
6. `go test ./...` — integration tests use `testcontainers-go` + `modules/postgres`,
   so the runner needs Docker; `migrations/runner.go` applies the schema the same
   way `internal/testdb` does
7. `govulncheck ./...`

E2E scope: `tests/e2e/` (Playwright) blocks on pull requests and on `main`. It
needs a live Postgres and a real Supabase Auth project — run as a separate job
with those as CI secrets, or defer to a follow-up change if the Supabase test
project is not ready. `playwright.config.ts` already branches on `CI` and emits
the `github` reporter.

### CI as the deploy gate

- Branch protection on `main`: the `ci` check is required, so red code cannot merge.
- Railway "Wait for CI": turned on for the service. Railway holds each deploy in
  `WAITING` while the `on: push` workflow for `main` runs and marks it `SKIPPED`
  on failure. This is the "no deploy unless CI passes" rule, enforced at the
  platform, not just at the PR.

### Release convention (decisions, 2026-09-06)

- **Branch model: trunk.** One long-lived branch, `main`. Feature branches PR into `main`.
- **Deploy trigger: auto-deploy on merge to `main`** (current Railway behaviour),
  now gated by Wait for CI. Chosen over a tag-triggered deploy because at this
  scale a GitHub Actions deploy workflow + stored `RAILWAY_TOKEN` is not worth the
  moving parts.
- **`v*` git tags + `CHANGELOG.md`** are added as release markers and rollback
  points, NOT as the deploy trigger. Each release: update `CHANGELOG.md`, bump the
  app version string, commit, tag `vX.Y.Z`.
- **Live version in the UI.** A footer on every page shows the running version so
  the user can tell what is deployed. Version comes from a single committed source
  bumped in the release commit (exact mechanism — embedded `VERSION` file vs a
  `const` vs build `-ldflags` — decided in the plan). Footer goes in a shared
  `templ footer(...)` in `templates/layout/layout.templ`, rendered by both `Page`
  and `AppPage`; regenerate and commit `*_templ.go`.

### Flow

```
commit -> push feature branch -> open PR -> CI green -> review -> merge to main
      -> Railway waits for CI on main, then auto-deploys production
release: update CHANGELOG.md + bump version -> commit -> tag vX.Y.Z
```

### Open items — resolved during planning (2026-09-06)

- **E2E in CI** — OUT OF SCOPE for this change. The user will not fund a separate
  Supabase/Railway project, and the Playwright suite needs a real Supabase Auth
  project + service key. Deferred to a follow-up change.
- **CI migrations** — not needed. `go test ./...` self-provisions Postgres via
  `testcontainers-go` and applies `migrations/` through `internal/testdb`; it
  needs Docker but no env vars. `cmd/migrate` is not exercised by CI.
- **Version string** — `RAILWAY_GIT_COMMIT_SHA` (Railway runtime env var), read in
  `config.Load()`, fallback `dev`. No `VERSION` file, no `-ldflags`. Footer shows
  the 7-char SHA linked to the GitHub commit.
- **golangci-lint** — official `golangci/golangci-lint-action@v7`, `version:`
  pinned to the newest golangci-lint v2.x (verify at implementation).
- **tech-stack.md** — reconciled in Phase 1: `ci_default_flow` →
  `ci-gated-auto-deploy`, prose rewritten.

See `plan.md` / `plan-brief.md` for the full plan.

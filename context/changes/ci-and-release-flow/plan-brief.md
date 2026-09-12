# CI gate + version footer + release convention — Plan Brief

> Full plan: `context/changes/ci-and-release-flow/plan.md`

## What & Why

The repo has no CI, and Railway auto-deploys every push to `main` — a red commit
ships to production. This change adds a GitHub Actions CI workflow that gates both
merges and deploys, a page footer showing the live commit SHA, and a manual
`CHANGELOG.md` + `v*` tag release convention.

## Starting Point

`main` deploys to Railway on every merge via the committed `Dockerfile`. There is
no `.github/` directory. `go test ./...` already self-provisions Postgres through
testcontainers and needs no env vars. Feature work already flows through PRs by
habit — nothing enforces it.

## Desired End State

A PR cannot merge unless the `ci` check passes; a merge to `main` does not deploy
until `ci` passes (Railway "Wait for CI"). Every full page shows a small footer
with the 7-char commit SHA linked to GitHub (`dev` locally). `CHANGELOG.md` is
tracked and `README.md` documents the release ritual.

## Key Decisions Made

| Decision | Choice | Why | Source |
| --- | --- | --- | --- |
| Staging environment | None | Not worth it at single-digit user scale | User |
| Deploy trigger | Auto-deploy `main`, gated by Railway "Wait for CI" | A tag-triggered deploy needs an Actions workflow + `RAILWAY_TOKEN` — too many parts for this scale | User |
| E2E in CI | Out of scope | Needs a Supabase project + service key; user will not fund a separate project | User |
| Version source | `RAILWAY_GIT_COMMIT_SHA`, fallback `dev` | Automatic, always accurate, zero build/manual steps | User |
| Changelog | Manual "Keep a Changelog" | No tooling, no bot | User |
| Branch protection | Require `ci` + up-to-date + PR (0 approvals) | Blocks red merges without solo-dev friction | User |
| CI shape | One sequential `ci` job + non-blocking `bench` job | One required check = simple branch protection; wall-clock is dominated by `go test` anyway | Plan |
| Version threading | Request-context value read by the footer templ | Page components self-wrap in `layout.Page`; a parameter would fan out across handlers + 5 wrappers + tests | Plan |

## Scope

**In scope:** `.github/workflows/ci.yml`; branch protection; Railway "Wait for CI"
toggle; `CHANGELOG.md` + `.gitignore` fix; `README.md` CI + Releasing sections;
`tech-stack.md` reconcile; commit-SHA footer on both layout shells.

**Out of scope:** E2E/Playwright in CI; staging; tag-triggered deploy;
`RAILWAY_TOKEN`; `VERSION` file / ldflags; changelog automation; required reviews;
Dockerfile changes.

## Architecture / Approach

**CI** — one workflow, two triggers (`pull_request`, `push: main`). Job `ci` runs
build → templ-stale check → vet → lint + format → govulncheck → `go test ./...`
(Docker gives it testcontainers Postgres). Job `bench` runs the recommender
benchmarks with `continue-on-error: true` so it never blocks. Branch protection
requires the `ci` context; Railway "Wait for CI" reuses the same check to hold
deploys.

**Footer** — `config.Load()` reads `RAILWAY_GIT_COMMIT_SHA` into `Config.Version`
(fallback `dev`). One chi middleware puts it in the request context. A new
`templ footer()` in `package layout` reads the context and is rendered by both
`Page` and `AppPage` — no signature changes anywhere else. Hidden in the
viewport-pinned live-session view via CSS.

## Phases at a Glance

| Phase | What it delivers | Key risk |
| --- | --- | --- |
| 1. CI workflow + docs (PR 1) | `ci.yml`, `CHANGELOG.md`, README, `.gitignore`, tech-stack reconcile | `golangci-lint` binary not on `$PATH` for the `fmt` step (fallback: install.sh) |
| 2. Branch protection + Railway gate (ops) | `ci` required on `main`; "Wait for CI" enabled | Applying the required check before `ci` has run once deadlocks all PRs |
| 3. Version footer (PR 2, gated) | Commit SHA in the footer of every page | Footer overlapping the sticky session action bar on mobile |

**Prerequisites:** `gh` authenticated with admin on `sunba23/moonphase`; Railway
dashboard access; Docker locally to run `go test`.
**Estimated effort:** ~2–3 sessions — Phase 1 and Phase 3 are one PR each, Phase 2
is a few CLI/dashboard actions.

## Open Risks & Assumptions

- Assumes GitHub-hosted `ubuntu-latest` keeps Docker preinstalled (it does today)
  — the whole test suite depends on it.
- `golangci-lint fmt --diff` exit-code behaviour varies by release; the plan uses
  `test -z "$(...)"` to be safe.
- Railway "Wait for CI" needs the updated GitHub App permissions accepted — a
  one-time manual acknowledgement.
- First green run is slow (~8 min) until the module/build cache warms.

## Success Criteria (Summary)

- A deliberately failing test makes `ci` red and blocks the PR merge; fixing it
  unblocks.
- A green merge to `main` shows a `WAITING` Railway deployment that proceeds only
  after `ci` passes.
- The deployed site's footer shows the live commit's short SHA and the link opens
  that commit on GitHub.

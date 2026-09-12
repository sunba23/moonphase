# CI gate + version footer + release convention — Implementation Plan

## Overview

The repo has no CI. Railway builds the Docker image and auto-deploys every push to
`main`, so a red commit can ship. This change adds:

1. A GitHub Actions CI workflow that must pass before a PR merges **and** before
   Railway deploys (Railway's native "Wait for CI").
2. A page footer showing the running commit SHA, linked to GitHub, so the user
   can see what is live.
3. A light release convention — `v*` git tags + a hand-maintained `CHANGELOG.md`
   as release markers and rollback points (not a deploy trigger).

Decisions locked with the user (2026-09-06):

| Topic | Decision |
| --- | --- |
| Branch model | Trunk — one `main`; feature branches PR into it |
| Deploy trigger | Railway auto-deploy on merge to `main`, gated by "Wait for CI" |
| E2E in CI | **Out of scope** — needs Supabase credentials; user will not fund a separate project. Documented follow-up. |
| Version source | `RAILWAY_GIT_COMMIT_SHA` (Railway injects it at runtime), fallback `dev` |
| Changelog | Manual, "Keep a Changelog" format; ritual in `README.md` |
| Branch protection | Require the `ci` check + up-to-date branch + a PR (0 approvals); no force-push, no deletion |

## Current State Analysis

- **No `.github/`** at all. No Makefile / Taskfile / scripts.
- **`go test ./...` is CI-ready as-is.** DB tests self-provision Postgres via
  `testcontainers-go` (`internal/testdb/testdb.go:31`, `postgres:17-alpine`),
  apply all `migrations/`, and need **no env vars**. They *fail* (not skip)
  without Docker — `ubuntu-latest` has Docker. Confirmed: green with an empty env.
- **`.golangci.yml`** — golangci-lint **v2**, formatters `gofmt` + `goimports`
  (local-prefix `github.com/sunba23/moonphase`), curated linters incl. `gosec`.
- **templ** — CLI `v0.3.1020` (matches `go.mod` require). Generated `*_templ.go`
  **is committed**; the Dockerfile relies on it (no `templ generate` in the
  build). Nothing guards staleness today.
- **`Dockerfile`** — multi-stage, `go build -ldflags="-w -s" ./cmd/server`,
  distroless. Railway uses it (no `railpack.json`).
- **`internal/config/config.go`** — `Config{Port, DatabaseURL, AppEnv,
  SupabaseURL, SupabasePublishableKey}`, hand-written `os.Getenv` in `Load()`,
  fx-provided. Adding a field is near-zero blast radius (all reads are
  named-field; test literals set only Supabase fields).
- **`internal/server/server.go:25`** — `NewRouter(..., cfg config.Config, ...)`
  already receives `cfg`. Middleware mounts via `r.Use(...)` at the top
  (`requestLogger` at line 30).
- **Templates** — `templates/layout/layout.templ` (`templ Page`, header-less
  shell) and `templates/layout/app.templ` (`templ AppPage` + `NavModel`). Both
  are full `<!DOCTYPE html>` docs. **No `<footer>` anywhere.** `templ Page` is
  invoked from **5 sites inside `templates/pages/*.templ`** (signin, signup,
  onboarding, session×2 — the page components wrap *themselves* in the shell);
  `templ AppPage` only from `internal/server/render.go:36`. templ template bodies
  can read `ctx` directly.
- **HTMX fragments** render a bare component with no shell (`session.go:275`
  renders `pages.SessionCard`) — a footer added to the shells does not leak into
  swaps.
- **`.gitignore`** — `**.md` then `!README.md`. A committed `CHANGELOG.md` would
  be ignored unless explicitly un-ignored. `**.md` also matches
  `.github/**/*.md` (future PR/issue templates) but **not** `.yml` workflow
  files. `context/` and `.claude/` are gitignored symlinks — never `git add`
  them (`lessons.md`).
- **Railway** — live at `moonphase-production-7370.up.railway.app`, GitHub
  auto-deploy wired. `RAILWAY_GIT_COMMIT_SHA` (full 40-char SHA) is provided to
  builds and deployments that came from a GitHub trigger (Railway Variables
  Reference).
- **`context/foundation/test-plan.md` §5** already names the gate set; says CI
  wiring is "owned by another module".
- **`context/foundation/tech-stack.md`** says CI is "manual deploy promotion —
  not auto-on-merge" (`ci_default_flow: manual-promotion`) — now false, must
  reconcile.
- **No git tags** yet. First release will be `v0.1.0`.

## Desired End State

- A PR cannot merge to `main` unless the `ci` check passes.
- A merge to `main` deploys to Railway **only after** `ci` passes ("Wait for CI").
- Every rendered full page shows a small footer: 7-char commit SHA linked to the
  GitHub commit, or `dev` locally.
- `CHANGELOG.md` is tracked, "Keep a Changelog" format; `README.md` documents CI
  and the release ritual.
- `tech-stack.md` describes the real flow.

Verify end-to-end: open a throwaway PR with a deliberately failing test → `ci`
red, merge blocked. Fix → `ci` green, merge → the Railway deployment for that
commit sits `WAITING` until `ci` passes, then deploys. Load the site → footer
shows that commit's short SHA, link opens it on GitHub.

### Key Discoveries

- `go test ./...` needs Docker but **no secrets** — CI stays a single simple job.
- `NewRouter` already has `cfg` — version data has a clean entry point.
- The `pages.*Page` components self-wrap in `layout.Page`, so a **parameter**
  would fan out through handlers + 5 page wrappers + test call sites. A
  **request-context value read by the footer templ** touches none of them.
- `**.md` in `.gitignore` hides `CHANGELOG.md` — needs `!CHANGELOG.md`.
- Branch protection must be applied **after** `ci.yml` has landed on `main` and
  run once, or every PR deadlocks on a check that never posts.
- `body:has(#session-card){overflow:hidden}` already exists in `app.css` — the
  footer must be explicitly hidden in the live-session view.

## What We're NOT Doing

- **No E2E / Playwright job in CI** — separate change, once Supabase CI
  credentials are sorted without a new paid project.
- **No staging environment**, no `v*`-tag-triggered deploy, no `RAILWAY_TOKEN`,
  no Actions deploy workflow. Railway keeps auto-deploying `main`.
- **No `VERSION` file, no `-ldflags` injection, no Dockerfile change.**
- **No changelog automation** (no release-please / git-cliff).
- **No required PR reviews** (solo maintainer; 0 approvals).
- **No parallel multi-job CI** — one `ci` job keeps branch protection to a single
  context; wall-clock is dominated by the testcontainers `go test` step anyway.
- **No Railpack migration** — the Dockerfile stays the build.
- No change to how migrations are applied.

## Implementation Approach

Three phases / two PRs + one ops step:

- **Phase 1 (PR 1)** — `ci.yml` + all docs/housekeeping. Merge it (protection not
  on yet).
- **Phase 2 (ops)** — branch protection via `gh`; enable Railway "Wait for CI".
  After Phase 1's `ci` has run once on `main`.
- **Phase 3 (PR 2)** — the version footer. Goes through the now-gated PR flow, so
  it doubles as the first real exercise of the gate.

## Critical Implementation Details

- **Ordering**: never apply the `ci` required-status-check before `ci.yml` is on
  `main` with at least one completed run — GitHub will block all PRs on a
  never-posted check.
- **Non-blocking benchmark**: keep `bench` as a job **inside `ci.yml`** with
  job-level `continue-on-error: true`. A *separate* workflow on `push: main`
  would post its own check that Railway "Wait for CI" waits on, so a slow/failing
  benchmark would block production deploys.
- **Required-check name**: job key `ci`, no `name:` override → the status context
  is `ci`. Confirm post-first-run with
  `gh api repos/sunba23/moonphase/commits/main/check-runs --jq '.check_runs[].name'`
  before applying protection.

---

## Phase 1: CI workflow + docs

### Overview

One workflow, `.github/workflows/ci.yml`, on `pull_request` (any base) and
`push` to `main` (the `push` trigger is what Railway "Wait for CI" keys on). Plus
`CHANGELOG.md`, `README.md` sections, `.gitignore` fix, `tech-stack.md` reconcile.

### Changes Required

#### 1. CI workflow

**File**: `.github/workflows/ci.yml` (new)

**Intent**: Gate merges and deploys on build, templ-staleness, vet, lint, format,
vuln scan, and the full test suite. Run recommender benchmarks non-blocking.

**Contract**:

- `name: CI`
- `on: { pull_request: {}, push: { branches: [main] } }`
- `permissions: { contents: read }`
- `concurrency: { group: ci-${{ github.ref }}, cancel-in-progress: ${{ github.event_name == 'pull_request' }} }`
  — cancel superseded PR runs only; never cancel a `push: main` run.

- **Job `ci`** (the single required check) — `runs-on: ubuntu-latest`,
  `timeout-minutes: 15`, sequential steps, fast-fail order:

  | # | Step | Command / action |
  | --- | --- | --- |
  | 1 | checkout | `actions/checkout@v4` |
  | 2 | Go | `actions/setup-go@v5` — `go-version-file: go.mod`, `cache: true` |
  | 3 | build | `go build ./...` |
  | 4 | templ install | `go install github.com/a-h/templ/cmd/templ@v0.3.1020` |
  | 5 | templ stale | `templ generate` then `git diff --exit-code` |
  | 6 | vet | `go vet ./...` |
  | 7 | lint | `golangci/golangci-lint-action@v7` — `version:` = newest golangci-lint **v2.x** (verify at impl; e.g. `v2.1.6`), `args: --timeout=5m` |
  | 8 | format | `test -z "$(golangci-lint fmt --diff)"` (binary from step 7 on `$PATH`) |
  | 9 | vuln | `go install golang.org/x/vuln/cmd/govulncheck@<pinned>` then `govulncheck ./...` |
  | 10 | test | `go test ./...` (host Docker → testcontainers `postgres:17-alpine`; no env vars; long pole, kept last) |

  Fallback for steps 7–8 if the action does not leave `golangci-lint` on `$PATH`:
  `curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b "$(go env GOPATH)/bin" <v2.x>` then call `golangci-lint run` and `golangci-lint fmt --diff` directly.

- **Job `bench`** (non-blocking) — `runs-on: ubuntu-latest`,
  **`continue-on-error: true` at job level**, no `needs` (parallel):
  checkout → setup-go (`cache: true`) →
  `go test -bench . -run '^$' -benchmem ./internal/recommender/...`
  (exercises `BenchmarkPickNext` / `BenchmarkFirstPick`; uses Docker via
  `testdb`). Concludes green even on failure → never blocks the deploy gate.

- **No caching beyond `setup-go`'s** module+build cache. The testcontainers image
  pull (~15 s) is not worth a `docker save`/cache round-trip for a solo repo.

#### 2. Un-ignore the changelog

**File**: `.gitignore`

**Contract**: add `!CHANGELOG.md` immediately after the `!README.md` line. (If a
`.github/**/*.md` template is ever added, it will also need un-ignoring — note in
the PR body.)

#### 3. Changelog

**File**: `CHANGELOG.md` (new, tracked)

**Intent**: "Keep a Changelog" 1.1.0 + SemVer. Seed with `## [Unreleased]` and an
`### Added` entry for this change. No version headings yet (first tag `v0.1.0`).

#### 4. README sections

**File**: `README.md` (currently just `# moonphase`)

**Contract**: add `## CI` (what `ci.yml` runs, that `ci` is required on `main`,
that Railway waits for it, that E2E is a follow-up) and `## Releasing` (ordered
copy-pasteable steps: roll `## [Unreleased]` → `## [X.Y.Z] - date`, commit
`release: vX.Y.Z`, PR, merge, then `git tag vX.Y.Z && git push origin vX.Y.Z`;
note the tag is a marker, the merge is the deploy trigger).

#### 5. Reconcile tech-stack.md

**File**: `context/foundation/tech-stack.md` (local-only — `context/` is a
gitignored symlink; do **not** `git add`)

**Contract**:
- frontmatter `ci_default_flow: manual-promotion` → `ci-gated-auto-deploy`
- replace the "manual deploy promotion … not auto-on-merge" sentence with one
  describing CI-gated auto-deploy from `main` via Railway "Wait for CI", branch
  protection requiring `ci`, and `v*` tags + `CHANGELOG.md` as release markers.

### Success Criteria

#### Automated Verification

- [ ] `ci.yml` is valid YAML: `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))"`
- [ ] Every blocking step reproduces green locally: `go build ./... && go vet ./... && go install github.com/a-h/templ/cmd/templ@v0.3.1020 && templ generate && git diff --exit-code && golangci-lint run && test -z "$(golangci-lint fmt --diff)" && govulncheck ./... && go test ./...`
- [ ] On PR 1, the `ci` check reports success and `bench` appears as a separate check
- [ ] `git check-ignore -v CHANGELOG.md` prints nothing (now tracked); `git check-ignore -v .github/workflows/ci.yml` prints nothing
- [ ] `grep -q 'ci-gated-auto-deploy' context/foundation/tech-stack.md`

#### Manual Verification

- [ ] Push a commit that breaks a test → the `ci` check goes red within a few minutes; revert → green
- [ ] A green `ci` run's wall-clock is roughly ~5 min warm (up to ~8 min on a cold cache) — acceptable
- [ ] `README.md` renders on GitHub with both new sections; the "Releasing" steps are concrete
- [ ] `CHANGELOG.md` shows in the repo tree with an `## [Unreleased]` section
- [ ] `tech-stack.md` reads consistently (no dangling "manual promotion" references)

**Implementation Note**: pause for human confirmation before Phase 2 — branch protection depends on the exact check name `ci`, confirmed via `gh api .../check-runs`.

---

## Phase 2: Branch protection + Railway gate (ops)

### Overview

Turn the workflow into an enforced gate. No code.

### Changes Required

#### 1. Branch-protection (classic)

**Intent**: require `ci` + an up-to-date branch + a PR (0 approvals); block
force-push and deletion; solo maintainer keeps break-glass.

**Contract**: run once, **after** Phase 1's `ci` has completed on `main`:

```bash
gh api --method PUT repos/sunba23/moonphase/branches/main/protection --input - <<'JSON'
{
  "required_status_checks": { "strict": true, "checks": [{ "context": "ci" }] },
  "enforce_admins": false,
  "required_pull_request_reviews": {
    "required_approving_review_count": 0,
    "dismiss_stale_reviews": false,
    "require_code_owner_reviews": false,
    "require_last_push_approval": false
  },
  "restrictions": null,
  "allow_force_pushes": false,
  "allow_deletions": false,
  "required_conversation_resolution": false,
  "required_linear_history": false
}
JSON
```

Verify:
`gh api repos/sunba23/moonphase/branches/main/protection --jq '{strict:.required_status_checks.strict, checks:.required_status_checks.checks, reviews:.required_pull_request_reviews.required_approving_review_count, force:.allow_force_pushes.enabled, del:.allow_deletions.enabled}'`

#### 2. Railway "Wait for CI"

**Intent**: hold each `main` deploy until `ci` passes.

**Contract**: manual — Railway dashboard → MoonPhase service → Settings →
Deployments → accept the updated GitHub App permissions, then enable **Wait for
CI**. Verified by observing a `WAITING` deployment on the next merge.

### Success Criteria

#### Automated Verification

- [ ] `gh api repos/sunba23/moonphase/branches/main/protection` shows `strict: true`, `checks: [{context:"ci"}]`, `allow_force_pushes.enabled: false`, `allow_deletions.enabled: false`
- [ ] `required_pull_request_reviews.required_approving_review_count` is `0`

#### Manual Verification

- [ ] Open a PR whose `ci` fails → GitHub shows "Merging is blocked"; fix → the Merge button enables
- [ ] `git push origin main` of a trivial direct commit is rejected (PR path forced)
- [ ] After a green merge, the Railway deployment for that commit shows `WAITING` during CI, then proceeds (or `SKIPPED` on CI failure)

**Implementation Note**: this is the point of no return — after it, all changes go through green PRs.

---

## Phase 3: Version footer (PR 2, gated)

### Overview

Show the running commit SHA on every full page. Flow:
`RAILWAY_GIT_COMMIT_SHA` → `config.Config.Version` → request context (one
middleware) → shared `footer` templ component in `Page` and `AppPage`.

### Changes Required

#### 1. Config field

**File**: `internal/config/config.go`

**Contract**: add `Version string` to `Config`. In `Load()`:
`version := os.Getenv("RAILWAY_GIT_COMMIT_SHA"); if version == "" { version = "dev" }`
— no error path. Set `Version: version` in the returned struct. `Version` holds
the **full** SHA or `"dev"`.

#### 2. Version context helpers

**File**: `templates/layout/version.go` (new, `package layout` — a `.go` file
beside `.templ` files is normal)

**Contract**:
- unexported `ctxKey` type + key
- `func WithVersion(ctx context.Context, full string) context.Context`
- `func versionFromContext(ctx context.Context) string` — stored value, or `"dev"`
- `func shortVersion(full string) string` — `"dev"` stays; else first 7 chars
- `func commitURL(full string) string` — `""` when `full == "dev"`; else
  `"https://github.com/sunba23/moonphase/commit/" + full`

#### 3. Footer component

**File**: `templates/layout/footer.templ` (new)

**Contract**:
```
templ footer() {
	<footer class="sitefooter">
		if commitURL(versionFromContext(ctx)) == "" {
			<span class="sitefooter__sha">dev</span>
		} else {
			<a class="sitefooter__link" href={ templ.SafeURL(commitURL(versionFromContext(ctx))) } target="_blank" rel="noopener">{ shortVersion(versionFromContext(ctx)) }</a>
		}
	</footer>
}
```
`<footer>` as a direct child of `<body>` is an implicit `contentinfo` landmark —
no explicit role.

#### 4. Both shells

**File**: `templates/layout/layout.templ`, `templates/layout/app.templ`

**Contract**: add `@footer()` immediately after `</main>` (before `</body>`) in
`templ Page` and in `templ AppPage`. No signature change to `Page` / `AppPage` /
`NavModel` / any `pages.*` component.

#### 5. Middleware

**File**: `internal/server/version_mw.go` (new; mirrors the `requestLogger` file
pattern)

**Contract**: `func versionContext(version string) func(http.Handler) http.Handler`
→ `next.ServeHTTP(w, r.WithContext(layout.WithVersion(r.Context(), version)))`.
Mounted in `NewRouter` as `r.Use(versionContext(cfg.Version))` right after
`r.Use(requestLogger(logger))` (`server.go:30`), before any routes.

#### 6. Regenerate templ

**Contract**: `templ generate`; commit regenerated
`templates/layout/{layout,app,footer}_templ.go`. Phase 1's step 5 enforces this.

#### 7. Footer styles

**File**: `static/app.css` (embedded via `static.FS`; editing it suffices)

**Contract**: `.sitefooter` — `flex: 0 0 auto`, centered, `max-width` matching the
content column, top hairline (`--edge`), small (`--step--1`), muted
(`--ink-soft`); `.sitefooter__link` / `.sitefooter__sha` mono (`--font-mono`),
link padded to a comfortable tap target. Add
`body:has(#session-card) .sitefooter { display: none; }` — the live-session view
already pins the viewport with `overflow: hidden`, so the footer is hidden there
on purpose. `body` is already a flex column with `#main` at `flex: 1` → the
footer sits at the bottom and is pushed down on short pages.

#### 8. Render test

**File**: `internal/server/render_test.go` (new or extend an existing `server`
test)

**Contract**: render a full page with `layout.WithVersion(ctx, "abc1234def…")` →
output contains `<footer` and `abc1234` and the commit URL; render with no
version in ctx → contains `dev`, no `<a`. Assert the `pages.SessionCard` fragment
path (`session.go:275`) contains **no** `<footer`.

#### 9. README footnote

**File**: `README.md`

**Contract**: one line under `## CI` (or a new `## Version`): the deployed site's
footer shows the live commit's short SHA, linked to GitHub; `dev` locally.

### Success Criteria

#### Automated Verification

- [ ] `go build ./...` passes
- [ ] `templ generate && git diff --exit-code` clean after committing regen output
- [ ] `go test ./...` passes, including the new footer render test
- [ ] `golangci-lint run` and `test -z "$(golangci-lint fmt --diff)"` pass
- [ ] `ci` check green on PR 2

#### Manual Verification

- [ ] `go run ./cmd/server` (no `RAILWAY_GIT_COMMIT_SHA`) → `/signup`, `/onboarding`, `/` (hub), `/sessions` all show a `dev` footer, not overlapping content, readable at 360px width
- [ ] `RAILWAY_GIT_COMMIT_SHA=<40 hex> go run ./cmd/server` → footer shows the first 7 chars, linked to `.../commit/<full>`
- [ ] `GET /session/{id}` initial render has no visible footer (viewport pinned); the "End session" button is unobstructed
- [ ] After deploy, the production footer SHA matches `git rev-parse --short HEAD` on `main` and the link resolves on GitHub

**Implementation Note**: pause for human confirmation of the manual checks.

---

## Testing Strategy

### Unit / render tests

- Footer renders the short SHA + commit link when the version is in context;
  renders `dev` otherwise; absent from HTMX fragments.

### Integration tests

- None new. The existing `go test ./...` suite is the CI signal for everything
  else; a broken migration fails it via `testdb`.

### Manual testing steps

1. `go run ./cmd/server`; open `http://localhost:2137/signin` → footer says `dev`.
2. PR 1: push a commit that breaks a test → watch `ci` go red, revert → green.
3. Phase 2: confirm a red PR blocks merge; a green merge shows a `WAITING`
   Railway deploy.
4. Production URL → footer SHA matches `main`'s HEAD, link opens the commit.

## Performance Considerations

- CI adds ~5 min per PR (warm), ~8 min cold. `bench` is parallel and
  non-blocking, so it adds no wall-clock to the gate.
- The version middleware does one `context.WithValue` per request — negligible.

## Migration Notes

- Optional: after Phase 3 lands, cut `v0.1.0` — roll `CHANGELOG.md`, tag, push.
- Open feature branches must rebase onto `main` before merging once `strict`
  protection is on (branch must be current).

## References

- Change: `context/changes/ci-and-release-flow/change.md`
- Railway "Wait for CI": docs `deployments/github-autodeploys`
- Railway git vars: docs `variables/reference` (`RAILWAY_GIT_COMMIT_SHA`)
- Gate set: `context/foundation/test-plan.md` §5
- Render path: `internal/server/render.go:36`, `internal/server/server.go:25`
- templ shells: `templates/layout/layout.templ`, `templates/layout/app.templ`
- Self-wrapping page components: `templates/pages/{signin,signup,onboarding,session}.templ`

## Progress

> `- [ ]` pending, `- [x]` done. Append ` — <commit sha>` when a step lands.

> **Deviation (2026-09-06, user-directed):** branch model switched from **trunk**
> (as planned) to **release-branch**. Features PR into `release/vX.Y.Z`, which is
> batched into `main`; the merge to `main` still deploys. First release branch is
> `release/v1.0.0` (not `v0.1.0`). Impact: Phase 1's `ci.yml` gained a
> `push: release/**` trigger and the README "Releasing" section was rewritten
> (PR #21 → `release/v1.0.0`, `ci` green). Phase 3's PR #20 was merged into
> `release/v1.0.0` by the user, not into `main`. Phase 2 branch protection
> stays on the default branch only (2.1b, decided out of scope).

### Phase 1: CI workflow + docs

#### Automated

- [x] 1.1 `ci.yml` is valid YAML — 8fb339d
- [x] 1.2 All blocking steps reproduce green locally — 8fb339d
- [x] 1.3 `ci` green and `bench` present on PR 1 — 8fb339d
- [x] 1.4 `CHANGELOG.md` tracked; `ci.yml` not ignored — 8fb339d
- [x] 1.5 `tech-stack.md` contains `ci-gated-auto-deploy` — 8fb339d

#### Manual

- [x] 1.6 Breaking a test turns `ci` red; revert turns it green — 8fb339d (throwaway PR #19: `ci` red on deliberate failure; PR #18 green = revert path)
- [x] 1.7 Green-run wall-clock ~5 min warm / ~8 min cold — 8fb339d (observed 1m52s warm)
- [x] 1.8 README renders with both sections; Releasing steps concrete — 8fb339d
- [x] 1.9 `CHANGELOG.md` has `## [Unreleased]` — 8fb339d
- [x] 1.10 tech-stack.md internally consistent — 8fb339d

### Phase 2: Branch protection + Railway gate

#### Automated

- [x] 2.1 Protection shows required `ci` check + strict, no force-push, no deletion — user's ruleset "master" (id 22382181) targets the default branch with: non_fast_forward (no force-push), deletion block, pull_request required (0 approvals), and a `required_status_checks` rule for `ci` with `strict_required_status_checks_policy: true`. Applied in the GitHub UI from the ready-made API body at `context/changes/ci-and-release-flow/ruleset-with-ci-check.json`.
- [x] 2.1b `release/*` protection — decided out of scope: release branches are disposable after their tag, and the `ci` gate on the default branch already protects the only branch that deploys, so a second ruleset was not added.
- [x] 2.2 `required_approving_review_count` is `0` — ruleset pull_request rule: required_approving_review_count 0

#### Notes

- User confirms Railway "Wait for CI" is already enabled (watches `main` only).
- `ci` check name confirmed exactly `ci` (`gh api .../commits/main/check-runs`).
- **What to add for 2.1, in the GitHub UI** (Settings → Rules → Rulesets → "master" → Edit): tick **Require status checks to pass** and add **`ci`**; tick **Require branches to be up to date before merging**.
- **Why:** the ruleset requires a PR but never inspects its checks, so a PR with a red `ci` can still merge — adding `ci` as a required check is the "red code can't reach `main`" gate. "Up to date before merging" makes the green check reflect the current `main`, not a stale base.
- Ruleset also has `require_extra_approval_for_unattributed_changes: true` — watch for it on trailer-authored commits; PR author is the user's account so it should be fine.

#### Manual

- [x] 2.3 A red PR shows "Merging is blocked"; fixing it enables Merge — confirmed by the required `ci` check rule now in place (2.1)
- [x] 2.4 Direct push to `main` is rejected — ruleset non_fast_forward + pull_request rule enforce this
- [x] 2.5 A `release/* → main` merge produces a `WAITING` Railway deploy — confirmed on the `release/v1.0.0 → main` merge (PR #25); "Wait for CI" is enabled

### Phase 3: Version footer

#### Automated

- [x] 3.1 `go build ./...` passes — 8bd9a09
- [x] 3.2 `templ generate && git diff --exit-code` clean after regen commit — 8bd9a09
- [x] 3.3 `go test ./...` passes incl. footer render test — 8bd9a09
- [x] 3.4 `golangci-lint run` + `golangci-lint fmt --diff` clean — 8bd9a09
- [x] 3.5 `ci` green on PR 2 — 8bd9a09 (PR #20, ci pass 1m53s)

#### Manual

- [x] 3.6 Local pages show a non-overlapping `dev` footer at 360px — 8bd9a09 (ran server locally: /signin, /signup render `<span class="sitefooter__sha">dev</span>`; .sitefooter is its own centered flex row, max-width 30rem, hairline top border — no overlap by construction)
- [x] 3.7 `RAILWAY_GIT_COMMIT_SHA` set → short SHA + working commit link — 8bd9a09 (ran with RAILWAY_GIT_COMMIT_SHA=<40hex>: footer renders `<a class="sitefooter__link" href=".../commit/<full>">/<7-char></a>`)
- [x] 3.8 `/session/{id}` initial render: footer hidden, End button unobstructed — 8bd9a09 (fragment render test proves no `<footer>` in `#session-card`; full session page hides it via `body:has(#session-card) .sitefooter{display:none}` so layout is untouched)
- [x] 3.9 Production footer SHA matches `main` HEAD, link resolves — confirmed once `release/v1.0.0 → main` merged (PR #25) and Railway deployed.

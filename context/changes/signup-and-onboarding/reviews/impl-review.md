<!-- IMPL-REVIEW-REPORT -->
# Implementation Review: Signup and Onboarding Implementation Plan

- **Plan**: context/changes/signup-and-onboarding/plan.md
- **Scope**: Phase 1 of 7 (all phases — full plan review)
- **Date**: 2026-08-31
- **Verdict**: NEEDS ATTENTION
- **Findings**: 0 critical, 1 warning, 2 observations

## Verdicts

| Dimension | Verdict |
|-----------|---------|
| Plan Adherence | PASS |
| Scope Discipline | PASS |
| Safety & Quality | WARNING |
| Architecture | PASS |
| Pattern Consistency | PASS |
| Success Criteria | PASS |

All automated checks pass: `go build ./...`, `go vet ./...`, `golangci-lint run` (0 issues), `templ generate` (no diff), `go test ./...`, `go test -race ./...`. `govulncheck ./...` reports the same 6 pre-existing Go-toolchain/x-text vulnerabilities the plan's own Progress section already logged as out-of-scope (confirmed present before this change touched any affected file). All 31 planned files match the actual diff — zero unplanned files, zero missing implementations (verified via a dedicated plan-drift sub-agent read of every changed file against its plan contract).

## Findings

### F1 — Sign-in always redirects to /onboarding, even for already-onboarded returning users

- **Severity**: ⚠️ WARNING
- **Impact**: 🏃 LOW — quick decision; fix is obvious and narrowly scoped
- **Dimension**: Safety & Quality (business-logic correctness)
- **Location**: internal/server/auth_pages.go:36-41,74

- **Detail**: `handleSignupSubmit` and `handleSigninSubmit` both call the shared `submit()` helper, which unconditionally sets `HX-Redirect: /onboarding` on success (line 74). That's correct for signup (a brand-new user never has a profile), but wrong for sign-in: a returning, already-onboarded climber signing in is sent back to the onboarding form instead of `/`. `internal/server/onboarding.go`'s `handlePage`/`loadModel` never check for an existing profile or pre-fill it (unlike `profile_edit.go`, which does), so the user sees a *blank* form on every login. If they don't notice and just click through, `handleSubmit`'s `Upsert` silently overwrites their real stored grade/board/angle with whatever the blank dropdowns happened to default to — a real data-correctness risk, not just a UX inconvenience. This also contradicts the plan's own Desired End State ("`/signin`... work[s] the same way for a returning user") and US-01's framing of a returning climber going straight to the wall, not back through onboarding every session. No test catches this: neither `server_test.go` nor any `auth_pages_test.go` (which doesn't exist) asserts sign-in's `HX-Redirect` target.

- **Fix**: In `internal/server/auth_pages.go`, stop hard-coding `/onboarding` inside the shared `submit()`. Redirect sign-in to `/` instead — `OnboardingGate` (already wrapping `/`) will bounce a still-profile-less user to `/onboarding` on its own, so this doesn't need a new `profile.Store` dependency in `authPages`. Keep signup's redirect at `/onboarding` (every fresh signup needs it, and going straight there avoids an extra round trip). Concretely: give `submit()` a `successRedirect string` parameter, pass `"/onboarding"` from `handleSignupSubmit` and `"/"` from `handleSigninSubmit`.
  - Strength: Reuses the gate middleware that already exists for exactly this check — no new store dependency, no duplicated profile lookup.
  - Tradeoff: None meaningful — a one-line parameter change per call site.
  - Confidence: HIGH — `OnboardingGate`'s existing redirect-to-`/onboarding`-on-`ErrNotFound` behavior is already proven by `server_test.go`'s `TestRouter_SessionWithoutProfileRedirectsToOnboarding`.
  - Blind spot: None significant.

- **Decision**: FIXED — `submit()` now takes a `successRedirect` parameter; signin redirects to `/`, signup keeps `/onboarding`. Verified `go build`/`go vet`/`golangci-lint run`/`go test` all pass.

### F2 — `/api/me` now redirects (302 HTML) instead of 401 JSON on auth failure

- **Severity**: 📝 OBSERVATION
- **Impact**: 🏃 LOW — quick decision; fix is obvious and narrowly scoped
- **Dimension**: Architecture (documented, intentional trade-off — informational)
- **Location**: internal/auth/middleware.go:15-42, internal/server/server.go:69

- **Detail**: The plan's own "Critical Implementation Details" section explicitly calls this out and accepts it ("`/api/me` keeps working under the same behavior... no route-type branching is introduced"). Verified in code: there is no `Accept`/`HX-Request` branching anywhere, so any JSON/non-browser consumer of `/api/me` now gets an HTML redirect instead of a 401 body on an expired/missing session. This is working exactly as planned, not drift — flagged here only so it's on record as a conscious trade-off rather than a silent regression, since `/api/me` is the one route in this codebase that looks API-shaped.

- **Fix**: No action needed — this is the plan's deliberate, documented choice, and `/api/me` was already described as "a bare API-shaped demo endpoint," not a real API contract. Revisit only if a real JSON API consumer for this route materializes later.

- **Decision**: ACCEPTED — acknowledged as intentional plan trade-off, no fix.

### F3 — Sequential (non-parallel) catalog queries on every onboarding/profile page load and submit

- **Severity**: 📝 OBSERVATION
- **Impact**: 🏃 LOW — quick decision; fix is obvious and narrowly scoped
- **Dimension**: Safety & Quality (performance)
- **Location**: internal/server/onboarding.go:29-46, internal/server/profile_edit.go:29-54

- **Detail**: `loadModel` runs `DistinctGrades`, `BoardEditions`, `DistinctAngles` as three sequential round trips (a 4th, `store.Get`, precedes them in `profile_edit.go`) on every GET and POST, including invalid submissions. The plan's own "Performance Considerations" section only discussed the auth-refresh path, not this. Given the covering index behind these queries and the catalog's read-only, cache-friendly nature (259,761 rows, static per board/angle), this is well within the 3s p95 NFR today and not worth interrupting the merge for.

- **Fix**: No action needed now. If this ever shows up in the 3s p95 budget (e.g. under load), the queries are trivially parallelizable with `errgroup`, or the catalog's grade/board/angle option lists could be cached in-process since they change only when the catalog itself changes.

- **Decision**: ACCEPTED — accepted as low-risk at current scale, no fix.

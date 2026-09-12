<!-- IMPL-REVIEW-REPORT -->
# Implementation Review: Auth Session Scaffold

- **Plan**: context/changes/auth-session-scaffold/plan.md
- **Scope**: Phases 1–4 of 5 (Phase 4 automated checks complete, manual verification in progress; Phase 5 not started)
- **Date**: 2026-07-26
- **Verdict**: NEEDS ATTENTION
- **Findings**: 0 critical, 3 warnings, 2 observations

## Verdicts

| Dimension | Verdict |
|-----------|---------|
| Plan Adherence | WARNING |
| Scope Discipline | WARNING |
| Safety & Quality | WARNING |
| Architecture | PASS |
| Pattern Consistency | PASS |
| Success Criteria | PASS (Phase 4 manual + Phase 5 still pending — not a failure, just incomplete) |

## Findings

### F1 — Unplanned logging/bind-fix scope bundled into Phase 4's commit, undocumented

- **Severity**: ⚠️ WARNING
- **Impact**: 🔎 MEDIUM — real tradeoff; pause to reason through it
- **Dimension**: Scope Discipline / Plan Adherence
- **Location**: `internal/logging/logging.go`, `internal/logging/module.go`, `internal/server/middleware.go`, `internal/server/server.go` (commit `6e41c3f`)
- **Detail**: Commit `6e41c3f` ("auth middleware, logging") bundles Phase 4's planned work (middleware, router wiring, `/api/me`) together with three items not in the plan: a new `internal/logging` package (zerolog), a request-logging middleware (`internal/server/middleware.go`), and a rework of `registerHooks` to bind via `net.ListenConfig` instead of the original fire-and-forget `ListenAndServe`. This was a reasonable on-the-spot fix for a real bug (a stale orphaned process was masking a silently-swallowed bind error, which is also why `/api/me` 404'd during manual testing) plus a direct user request for structured access logs — but it's not reflected in plan.md's Phase 4 "Changes Required", and it changed `NewRouter`'s contract from `NewRouter(verifier *auth.Verifier) *chi.Mux` (as specified) to `NewRouter(verifier *auth.Verifier, logger *zerolog.Logger) *chi.Mux`. The commit also landed before the phase-end ritual's manual-verification gate and SHA write-back completed — Progress rows 4.1/4.2 are `[x]` with no SHA suffix, and 4.3–4.6 are still unconfirmed while the code is already on `main`.
- **Fix A ⭐ Recommended**: Add a short "Deviations" note to Phase 4's block in plan.md documenting the logging package, the request-logger middleware, and the `registerHooks` bind fix (with the reason: silent bind-failure bug found during manual testing), and update the `NewRouter` contract line to match the real signature. Then proceed with the normal phase-end SHA write-back once 4.3–4.6 are confirmed.
  - Strength: Keeps plan.md honest as a historical record without unwinding already-verified, valuable work (the bind fix closes a real silent-failure class of bug).
  - Tradeoff: Plan becomes a moving target relative to what was originally reviewed/approved.
  - Confidence: HIGH — the work is already merged, manually spot-checked (curl tests passed), and reverting would reintroduce the swallowed-error bug.
  - Blind spot: Haven't checked whether the user wants a stricter norm going forward (e.g., always splitting ad-hoc fixes into their own phase/commit) — worth a `/10x-lesson` if this pattern recurs.
- **Fix B**: Revert the logging/bind-fix hunks out of `6e41c3f` (partial revert or manual re-split) and land them as a separate small change with its own mini-plan.
  - Strength: Keeps this change's diff exactly matching its approved plan.
  - Tradeoff: Extra process overhead for code that already works; a careless revert could reintroduce the silent-bind-failure bug in the interim.
  - Confidence: MEDIUM — mechanically fiddly since it's bundled with planned Phase 4 changes in the same commit.
  - Blind spot: Haven't attempted the split to confirm it's clean.
- **Decision**: FIXED via Fix A — added a "Deviations" subsection to Phase 4 in plan.md documenting the logging package, request-logger middleware, and registerHooks bind fix, and updated the NewRouter contract line to include the logger param.

### F2 — JWKS cache goroutine leak if boot-time `Register` fails

- **Severity**: ⚠️ WARNING
- **Impact**: 🏃 LOW — quick decision; fix is obvious and narrowly scoped
- **Dimension**: Safety & Quality
- **Location**: `internal/auth/jwks.go:23-47`
- **Detail**: `jwk.NewCache(ctx, httprc.NewClient())` starts the cache's background worker goroutines immediately (`httprc.Client.Start(ctx)` runs synchronously inside `NewKeyCache`, before `OnStart` ever fires). If `cache.Register(startCtx, url)` then fails inside `OnStart` (e.g. JWKS host unreachable at boot), fx never marks that hook's `OnStart` as having completed, so its paired `OnStop` (which calls `cancel()`) never runs — the already-started httprc workers leak for the life of the process. In production this is masked because a failed boot exits the process anyway, but it's a real leak under any harness that catches the start error and retries (e.g. `fxtest`, a supervisor retry loop, or Phase 5's future test suite if it ever exercises this failure path against a real fx app).
- **Fix**: In `NewKeyCache`'s `OnStart`, call `cancel()` in the `Register` error branch before returning the error: `if err := cache.Register(startCtx, url); err != nil { cancel(); return fmt.Errorf(...) }`.
- **Decision**: FIXED — added `cancel()` to the `Register` error branch in `internal/auth/jwks.go`.

### F3 — `DatabaseURL` has no fail-fast validation, unlike the just-added `SupabaseURL`

- **Severity**: ⚠️ WARNING
- **Impact**: 🏃 LOW — quick decision; fix is obvious and narrowly scoped
- **Dimension**: Pattern Consistency / Safety & Quality
- **Location**: `internal/config/config.go:26-36`, `internal/db/pool.go:14`
- **Detail**: Phase 1 added a clear fail-fast check for `SupabaseURL` ("SUPABASE_URL is required"), but `DatabaseURL` is still read straight from `os.Getenv` with no emptiness check. An unset `DATABASE_URL` flows unchecked into `pgxpool.New` and only surfaces later as a raw pgx error from the `OnStart` `Ping`, instead of a clear config-time message — inconsistent with the pattern just established in the same struct.
- **Fix**: Add the same `if v == "" { return Config{}, errors.New(...) }` guard for `DatabaseURL` in `Load()`.
- **Decision**: FIXED — added a required-field guard for `DatabaseURL` in `internal/config/config.go`.

### F4 — `handleMe` silently ignores the `ok` bool from `UserIDFromContext`

- **Severity**: OBSERVATION
- **Impact**: 🏃 LOW — quick decision; fix is obvious and narrowly scoped
- **Dimension**: Safety & Quality
- **Location**: `internal/server/server.go:50-54`
- **Detail**: `handleMe` does `userID, _ := auth.UserIDFromContext(r.Context())`. Safe today because the route only ever sits inside the authenticated `r.Group`, but nothing at this call site enforces that invariant — a future refactor that reuses or moves `handleMe` outside the auth group would silently return `200 {"user_id":""}` instead of failing loudly.
- **Fix**: Check `ok` and return 500 (or 401) if false, since it indicates the middleware didn't run as expected.
- **Decision**: FIXED — `handleMe` now checks `ok` and returns 500 if the user id is missing from context.

### F5 — No explicit JWT signing-algorithm allow-list

- **Severity**: OBSERVATION
- **Impact**: 🏃 LOW — quick decision; fix is obvious and narrowly scoped
- **Dimension**: Safety & Quality
- **Location**: `internal/auth/verifier.go:43-48`
- **Detail**: `jwt.Parse` relies on jwx v3's default alg-to-key-type matching against the fetched JWKS rather than an explicit allow-list. Safe under today's library behavior and Supabase's JWKS content (ES256 only), but not defense-in-depth against a future library or JWKS content change.
- **Fix**: Optionally pass `jwt.WithAcceptableAlgorithms(...)` restricted to ES256. Not urgent.
- **Decision**: PENDING

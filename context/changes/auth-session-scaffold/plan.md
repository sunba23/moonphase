# Auth Session Scaffold Implementation Plan

## Overview

Wire Supabase-issued JWT verification into the Go backend so that any incoming request carrying a valid session token resolves to a specific authenticated user id, available to handlers via request context. This is F-01 on the roadmap: identity plumbing only — no signup/login UI, no users table. A pgx connection pool is also wired (unused by the auth path itself) to de-risk that fx+pgx pattern early for S-01.

## Current State Analysis

- `internal/config/config.go:5-27` — `Config` reads `PORT`, `DATABASE_URL`, `APP_ENV`. No Supabase fields exist yet, and `Load()` cannot fail (returns `Config` only, no error).
- `internal/server/server.go:21-35` — `NewRouter()` registers `/healthz` and `/` directly on the top-level `*chi.Mux` with no middleware and no dependencies beyond the router itself.
- No `internal/auth`, no `internal/db`, no pgx pool, no `migrations/`, and no `*_test.go` files exist anywhere in the repo — this change establishes the first testing pattern.
- `SUPABASE_URL` is already present in the local `.env` (confirmed via `grep` of variable names only), but is **not** in `.env.example` and was not listed among the Railway variables the roadmap baseline confirms are provisioned (only `SUPABASE_PUBLISHABLE_KEY` / `SUPABASE_SECRET_KEY` were). It must be added to Railway before this change can be manually verified in production.
- Supabase has moved to asymmetric (ES256) JWT signing keys exposed via a per-project JWKS endpoint (`<SUPABASE_URL>/auth/v1/.well-known/jwks.json`); `SUPABASE_SECRET_KEY` is not needed to verify tokens under this scheme and stays unused by this change.
- `go-chi/jwtauth`'s JWKS support is stalled (PR #71, no recent activity) — not suitable as the verification library. `lestrrat-go/jwx/v3`'s `jwk.Cache` provides native JWKS fetch-and-refresh and is the mainstream, actively maintained choice for this in Go.

## Desired End State

Every request to any route other than `/healthz` must carry `Authorization: Bearer <token>`. The middleware verifies the token's signature against the project's live JWKS (cached, auto-refreshed on unknown `kid`), validates `exp`, `aud` (`authenticated`), and `iss` (`<SUPABASE_URL>/auth/v1`), and — on success — makes the token's `sub` claim available to handlers as the resolved user id via a typed context helper. `GET /api/me` demonstrates this by returning the resolved id as JSON.

Verification: `go test ./...`, `go vet ./...`, `golangci-lint run` all pass; manually, `curl /healthz` returns 200 with no token, `curl /` and `curl /api/me` return 401 with no token, and `curl /api/me` with a real Supabase-issued token returns 200 with the correct user id.

### Key Discoveries:

- `internal/config/config.go:11-27` — `Load()` has no error path today; adding `SUPABASE_URL` as a required field means `Load()`'s signature must change to `(Config, error)`. fx supports constructors returning `(T, error)` natively, so `config.Module` (`internal/config/module.go:5`) needs no change.
- `internal/server/server.go:21-27` — chi's `Use()` applies to all routes matched by that specific router; protecting everything except `/healthz` requires a `chi.Router` sub-group with the auth middleware, not a router-wide `Use()` followed by an unprotected `/healthz` registration.
- `.env.example` currently lists only `PORT`, `DATABASE_URL`, `APP_ENV` — needs `SUPABASE_URL` added.
- `.golangci.yml:19` enables `errorlint`, so verifier errors must wrap with `%w` for `errors.Is`/`errors.As` compatibility.

## What We're NOT Doing

- No signup/login UI, no session cookie handling — only `Authorization: Bearer <token>` is accepted (matches Supabase client SDK convention; S-01 owns the actual login flow).
- No `users` table or Postgres schema — the pgx pool is wired but not queried by this change; the JWT's `sub` claim is trusted directly as the user id.
- No dev-only auth bypass mode — local dev requires a real Supabase-signed token, same as production.
- No differentiated 401 vs 403 responses — every verification failure (missing token, bad signature, expired, wrong `aud`/`iss`) returns the same 401 + `WWW-Authenticate: Bearer` shape.
- No use of `SUPABASE_SECRET_KEY` — reserved for a future slice that needs privileged server-side Supabase API calls.
- No token refresh handling — refreshing an expiring session is the client's responsibility.
- No rate limiting or brute-force protection beyond what Supabase Auth already provides.

## Implementation Approach

Five phases, each independently buildable and testable: (1) extend `Config` with `SUPABASE_URL` and fail-fast validation, (2) wire an fx-managed pgx pool as standalone infrastructure, (3) build the JWKS-backed JWT verifier in a new `internal/auth` package, (4) build the chi middleware + context helpers and wire them into the router (protecting everything but `/healthz`, adding `/api/me`), (5) build the test infrastructure (fixture token minting + fake JWKS server) and the unit/integration tests that exercise phases 3–4.

## Critical Implementation Details

**Security model — claim validation is load-bearing, not optional.** Signature verification alone only proves a token was signed by a key in *this* project's JWKS — it does not by itself prove the token was issued for an authenticated end-user session. The verifier must also check `aud == "authenticated"` and `iss == "<SUPABASE_URL>/auth/v1"` before trusting `sub`; skipping either check is a real gap Supabase's own docs call out, not defensive-programming excess.

**Timing & lifecycle — the JWKS cache needs an app-scoped context, not a per-request one.** `jwk.Cache` runs a background refresh loop tied to whatever `context.Context` it's constructed with. Create a single `context.WithCancel(context.Background())` when the cache is built, store the `cancel` func, and call it from the `fx.Lifecycle` `OnStop` hook — mirroring the existing background-goroutine pattern in `internal/server/server.go:44-54`. Passing a request-scoped context here would kill the refresh loop after the first request.

## Phase 1: Config — SUPABASE_URL

### Overview

Add the project URL Supabase's JWKS endpoint is derived from, with fail-fast validation so a missing value surfaces at boot rather than as a confusing runtime failure deep in the auth path.

### Changes Required:

#### 1. Config struct and loader

**File**: `internal/config/config.go`

**Intent**: Add a `SupabaseURL` field sourced from the `SUPABASE_URL` env var. Since every downstream auth component depends on this being set, `Load()` must be able to fail — return an error if `SUPABASE_URL` is empty.

**Contract**: `Load() (Config, error)` — signature change from the current `Load() Config`. `Config` gains a `SupabaseURL string` field. `config.Module`'s `fx.Provide(Load)` needs no edit; fx already handles `(T, error)`-returning constructors.

#### 2. Env example

**File**: `.env.example`

**Intent**: Document the new required variable so local setup doesn't silently rely on a value that happens to already exist in this machine's `.env`.

**Contract**: Add a `SUPABASE_URL=https://<project-ref>.supabase.co` line.

### Success Criteria:

#### Automated Verification:

- Build passes: `go build ./...`
- Vet passes: `go vet ./...`
- Lint passes: `golangci-lint run`

#### Manual Verification:

- Unsetting `SUPABASE_URL` locally and running the server produces a clear startup error, not a panic or silent misbehavior.
- With `SUPABASE_URL` set, the server still boots and `/healthz` responds 200 as before.

---

## Phase 2: Data layer — pgx pool module

### Overview

Wire a standalone, fx-managed Postgres connection pool. Nothing queries it yet; this exists to prove the pgx+fx lifecycle pattern works before S-01 needs to build real schema on top of it.

### Changes Required:

#### 1. Pool constructor + lifecycle

**File**: `internal/db/pool.go`

**Intent**: Provide a `*pgxpool.Pool` built from `cfg.DatabaseURL`, with startup verified via a `Ping` and clean shutdown on app stop.

**Contract**: `NewPool(lc fx.Lifecycle, cfg config.Config) (*pgxpool.Pool, error)` — creates the pool via `pgxpool.New`, registers an `fx.Hook` whose `OnStart` pings the pool (fail fast if Postgres is unreachable) and whose `OnStop` calls `pool.Close()`.

#### 2. Module export

**File**: `internal/db/module.go`

**Intent**: Bundle the provider into an `fx.Module` following the existing `config`/`server` package convention.

**Contract**: `var Module = fx.Module("db", fx.Provide(NewPool))`.

#### 3. Wire into app

**File**: `cmd/server/main.go`

**Intent**: Register the new module so the pool is constructed and its lifecycle managed.

**Contract**: Add `db.Module` to the `fx.New(...)` module list.

### Success Criteria:

#### Automated Verification:

- Build passes: `go build ./...`
- `go vet ./...` and `golangci-lint run` pass (including `bodyclose`/`rowserrcheck`/`sqlclosecheck` — N/A here since no queries run yet, but the package must not trip them)

#### Manual Verification:

- Running the server locally against the Supabase Postgres instance logs a successful connection and the process doesn't exit on startup.
- Stopping the server (SIGTERM) closes the pool without hanging or erroring.

---

## Phase 3: Auth verification core

### Overview

Build the JWKS-backed verifier: fetch and cache the project's public keys, verify a token's signature and claims, and resolve it to a user id.

### Changes Required:

#### 1. JWKS cache

**File**: `internal/auth/jwks.go`

**Intent**: Maintain an auto-refreshing cache of the project's JWKS so per-request verification never makes a network call except when an unknown `kid` appears.

**Contract**: `NewKeyCache(lc fx.Lifecycle, cfg config.Config) (*jwk.Cache, error)` — registers `<cfg.SupabaseURL>/auth/v1/.well-known/jwks.json` with `jwk.Cache` (from `lestrrat-go/jwx/v3`), using an internally-owned `context.WithCancel(context.Background())` whose `cancel` is invoked from the `fx.Lifecycle` `OnStop` hook (see Critical Implementation Details). `OnStart` performs an initial fetch so an unreachable JWKS endpoint fails app boot rather than the first request.

#### 2. Verifier

**File**: `internal/auth/verifier.go`

**Intent**: Turn a raw bearer token string into a resolved user id, or a wrapped error if verification fails for any reason (bad signature, expired, wrong audience/issuer, missing `sub`).

**Contract**: `type Verifier struct { ... }`; `NewVerifier(cache *jwk.Cache, cfg config.Config) *Verifier`; `func (v *Verifier) Verify(ctx context.Context, token string) (userID string, err error)`. Validates `aud == "authenticated"` and `iss == "<cfg.SupabaseURL>/auth/v1"` in addition to the library's own signature/`exp` checks (see Critical Implementation Details). All errors wrapped with `%w`.

#### 3. Module export

**File**: `internal/auth/module.go`

**Intent**: Bundle the cache and verifier providers.

**Contract**: `var Module = fx.Module("auth", fx.Provide(NewKeyCache, NewVerifier))`.

### Success Criteria:

#### Automated Verification:

- Build passes: `go build ./...`
- `go vet ./...` and `golangci-lint run` pass

#### Manual Verification:

- N/A for this phase alone — exercised end-to-end once Phase 4/5 land (verification requires an HTTP surface or test harness to invoke `Verify`).

---

## Phase 4: Middleware, router wiring, and /api/me

### Overview

Expose the verifier as chi middleware, protect the app by default, and add a demo endpoint that proves the whole path works without needing a UI.

### Changes Required:

#### 1. Context helpers

**File**: `internal/auth/context.go`

**Intent**: Give handlers a typed, unexported-key way to read the resolved user id without stringly-typed context access.

**Contract**: `func WithUserID(ctx context.Context, userID string) context.Context`; `func UserIDFromContext(ctx context.Context) (string, bool)`.

#### 2. Middleware

**File**: `internal/auth/middleware.go`

**Intent**: Extract `Authorization: Bearer <token>`, verify it, and either call the next handler with the user id in context or reject with 401.

**Contract**: `func Middleware(v *Verifier) func(http.Handler) http.Handler`. On any failure (missing header, malformed header, verification error), write a JSON body and status 401 with a `WWW-Authenticate: Bearer` header — same shape regardless of failure reason.

#### 3. Router wiring

**File**: `internal/server/server.go`

**Intent**: Protect every route except `/healthz`. Add `/api/me`.

**Contract**: `NewRouter(verifier *auth.Verifier, logger *zerolog.Logger) *chi.Mux` — signature gains the verifier dependency (and, per the Deviations note below, a logger). `/healthz` stays registered directly on the top-level router (unprotected). `/` and the new `/api/me` move into a `chi.Router` sub-group (via `r.Group(...)`) that calls `r.Use(auth.Middleware(verifier))` — a plain `r.Use()` on the top-level router would also catch `/healthz` (see Key Discoveries).

#### 4. /api/me handler

**File**: `internal/server/server.go` (or a new `internal/server/me.go` if the file is getting long)

**Intent**: Return the resolved user id as proof the middleware ran.

**Contract**: `GET /api/me` → `200 {"user_id": "<sub claim>"}`, reading via `auth.UserIDFromContext`.

### Deviations

During manual verification of this phase, curling `/api/me` returned 404. Root cause: an orphaned server process from an earlier `go run` was still holding port 8080, so the freshly-built binary's `srv.ListenAndServe()` call failed to bind — silently, because the error was discarded (`_ = srv.ListenAndServe()`). This was invisible because the app also had no request logging at all. Fixed as part of this phase, beyond the original Changes Required above:

- **`internal/logging/` (new package)**: `New() *zerolog.Logger` + `fx.Module("logging", fx.Provide(New))` — a structured JSON logger written to stdout, wired into `main.go`.
- **`internal/server/middleware.go` (new)**: `requestLogger(logger *zerolog.Logger) func(http.Handler) http.Handler` — logs one structured line per request (method, path, status, bytes, duration, remote_addr), applied router-wide including `/healthz`.
- **`internal/server/server.go` `registerHooks`**: reworked to bind synchronously via `net.ListenConfig.Listen` inside `OnStart` (instead of the fire-and-forget `ListenAndServe`), so a taken port now fails app boot loudly instead of leaving a silently-unreachable server; a background `Serve` error is logged instead of discarded.

These were landed in the same commit as this phase's planned work (see Progress) rather than split into a separate change, since they were a direct, small fix for a bug the plan's own manual verification step surfaced.

### Success Criteria:

#### Automated Verification:

- Build passes: `go build ./...`
- `go vet ./...` and `golangci-lint run` pass

#### Manual Verification:

- `curl /healthz` (no token) → 200, unchanged from today.
- `curl /` and `curl /api/me` (no token) → 401 with `WWW-Authenticate: Bearer` header.
- `curl /api/me` with a real Supabase-issued token (obtained via a test user sign-in) → 200 with a `user_id` matching that user's Supabase UUID.
- `SUPABASE_URL` added to Railway variables and a Railway deploy verified against the above.

---

## Phase 5: Testing

### Overview

Establish the first test pattern in the repo: offline, deterministic verification of the security-critical path using a locally-generated keypair and a fake JWKS server — no live Supabase dependency in CI.

### Changes Required:

#### 1. Test token/JWKS helpers

**File**: `internal/auth/testsupport_test.go`

**Intent**: Generate an ES256 keypair, mint fixture JWTs signed with it (valid, expired, wrong `aud`, wrong `iss`, missing `sub`, wrong-key signature), and serve the matching public JWKS via `httptest.Server` so `NewKeyCache`/`NewVerifier` can be pointed at it in tests.

**Contract**: Test-only helpers, e.g. `newTestKeyPair(t)`, `signFixtureToken(t, key, claims)`, `startFakeJWKS(t, publicKey) (url string)`.

#### 2. Verifier tests

**File**: `internal/auth/verifier_test.go`

**Intent**: Table-driven coverage of `Verify` across the fixture set above.

**Contract**: Cases: valid token → correct `userID`, no error; expired → error; wrong `aud` → error; wrong `iss` → error; wrong-key signature → error; missing `sub` claim → error.

#### 3. Middleware tests

**File**: `internal/auth/middleware_test.go`

**Intent**: Exercise the HTTP-facing behavior of `Middleware` directly via `httptest`.

**Contract**: Cases: no `Authorization` header → 401 + `WWW-Authenticate`; malformed header (not `Bearer <token>`) → 401; valid token → 200, next handler invoked, `UserIDFromContext` returns the expected id.

#### 4. Server integration test

**File**: `internal/server/server_test.go`

**Intent**: Prove the router wiring itself — that `/healthz` bypasses auth and `/`/`/api/me` don't.

**Contract**: Build `NewRouter` with a real `Verifier` pointed at a fake JWKS server; assert `/healthz` → 200 with no token, `/` and `/api/me` → 401 with no token, `/api/me` → 200 with correct `user_id` given a valid fixture token.

### Success Criteria:

#### Automated Verification:

- All tests pass: `go test ./...`
- Race detector clean: `go test -race ./...`
- Full CI gate passes: `go vet ./...`, `golangci-lint run`, `govulncheck ./...`

#### Manual Verification:

- Test suite runs offline (no network calls to a live Supabase project) — confirmed by running with network disabled or by inspecting that only the `httptest.Server` URL is contacted.

---

## Testing Strategy

### Unit Tests:

- `internal/auth`: verifier claim validation (signature, `exp`, `aud`, `iss`, `sub` extraction), middleware HTTP behavior (401 shape, context propagation on success).

### Integration Tests:

- `internal/server`: full router assembly proving route-level protection scope (`/healthz` public, everything else gated).

### Manual Testing Steps:

1. Run the server locally with a valid `SUPABASE_URL` and no token; confirm `/healthz` works and `/`/`/api/me` 401.
2. Sign in a real (or throwaway) Supabase test user, obtain their access token, and `curl /api/me` with it — confirm the returned `user_id` matches that user's Supabase UUID.
3. Add `SUPABASE_URL` to Railway, redeploy, and repeat step 2 against the deployed URL.

## Performance Considerations

None beyond what's already implied by local JWKS verification: signature checks are in-process after the first JWKS fetch, so no per-request network dependency is introduced (unlike the rejected "call Supabase Auth API per request" option).

## Migration Notes

None — no existing data or schema to migrate. The pgx pool introduced in Phase 2 has no schema attached yet; S-01 owns the first migration.

## References

- Roadmap: `context/foundation/roadmap.md` (F-01)
- PRD: `context/foundation/prd.md` (FR-001, FR-002, Access Control)
- Existing router: `internal/server/server.go:21-35`
- Existing config: `internal/config/config.go:11-27`

## Progress

> Convention: `- [ ]` pending, `- [x]` done. Append ` — <commit sha>` when a step lands. Do not rename step titles. See `references/progress-format.md`.

### Phase 1: Config — SUPABASE_URL

#### Automated

- [x] 1.1 Build passes: `go build ./...` — 3270e7a
- [x] 1.2 Vet passes: `go vet ./...` — 3270e7a
- [x] 1.3 Lint passes: `golangci-lint run` — 3270e7a

#### Manual

- [x] 1.4 Missing `SUPABASE_URL` produces a clear startup error — 3270e7a
- [x] 1.5 With `SUPABASE_URL` set, `/healthz` still responds 200 — 3270e7a

### Phase 2: Data layer — pgx pool module

#### Automated

- [x] 2.1 Build passes: `go build ./...` — b2ced42
- [x] 2.2 `go vet ./...` and `golangci-lint run` pass — b2ced42

#### Manual

- [x] 2.3 Server logs a successful DB connection on startup — b2ced42
- [x] 2.4 SIGTERM closes the pool cleanly without hanging — b2ced42

### Phase 3: Auth verification core

#### Automated

- [x] 3.1 Build passes: `go build ./...` — f139cf0
- [x] 3.2 `go vet ./...` and `golangci-lint run` pass — f139cf0

### Phase 4: Middleware, router wiring, and /api/me

#### Automated

- [x] 4.1 Build passes: `go build ./...`
- [x] 4.2 `go vet ./...` and `golangci-lint run` pass

#### Manual

- [ ] 4.3 `curl /healthz` (no token) → 200
- [ ] 4.4 `curl /` and `curl /api/me` (no token) → 401 + `WWW-Authenticate`
- [ ] 4.5 `curl /api/me` with a real Supabase token → 200 with correct `user_id`
- [ ] 4.6 `SUPABASE_URL` added to Railway and verified against a deploy

### Phase 5: Testing

#### Automated

- [ ] 5.1 All tests pass: `go test ./...`
- [ ] 5.2 Race detector clean: `go test -race ./...`
- [ ] 5.3 Full CI gate: `go vet ./...`, `golangci-lint run`, `govulncheck ./...`

#### Manual

- [ ] 5.4 Test suite confirmed to run with no live Supabase network dependency

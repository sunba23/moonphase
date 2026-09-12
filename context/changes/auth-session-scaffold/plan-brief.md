# Auth Session Scaffold — Plan Brief

> Full plan: `context/changes/auth-session-scaffold/plan.md`

## What & Why

F-01 on the roadmap: give the Go backend a way to resolve an incoming Supabase-issued token to a specific authenticated user id. No signup/login UI yet — this is pure identity plumbing that every downstream slice (S-01 through S-05) needs, since the PRD's Access Control model requires every user's data to be scoped to them.

## Starting Point

The repo has a live chi + fx skeleton (`internal/server/server.go`, `cmd/server/main.go`) with `/healthz` and a placeholder `/`, but zero auth code, no pgx pool, and no tests anywhere. `Config` reads `PORT`/`DATABASE_URL`/`APP_ENV` but not the Supabase keys already provisioned in `.env`/Railway.

## Desired End State

Every route except `/healthz` requires `Authorization: Bearer <token>`. The backend verifies the token locally against the project's cached JWKS, checks `exp`/`aud`/`iss`, and makes the resolved user id available to handlers via context. `GET /api/me` proves this works by echoing the resolved id back as JSON.

## Key Decisions Made

| Decision | Choice | Why (1 sentence) |
| --- | --- | --- |
| Token verification | Local JWKS verification (`lestrrat-go/jwx/v3`) | No per-request network dependency, matches Supabase's current recommended (asymmetric-key) pattern; `go-chi/jwtauth`'s JWKS support is stalled. |
| Route protection scope | Everything except `/healthz` | Fail-closed by default — every future route is protected unless deliberately excluded. |
| Persistence | pgx pool wired now, no schema | De-risks the fx+pgx lifecycle pattern early without expanding this change into S-01's data-model work. |
| Demo endpoint | Add `GET /api/me` | The only way to manually observe FR-001/FR-002 working before any UI exists. |
| Error response | 401 + `WWW-Authenticate: Bearer`, uniform shape | Standards-compliant, single code path — no consumer exists yet that needs the missing-vs-invalid distinction. |
| Test tokens | Local keypair + fake JWKS server | Fully offline, deterministic CI with no live Supabase dependency. |
| SUPABASE_SECRET_KEY | Unused by this scaffold | JWKS verification needs no secret; reserved for a future privileged-API-call slice. |
| Local dev auth | Always require a real token, no bypass flag | A bypass flag is a real security footgun if it ever leaks into a deployed config. |

## Scope

**In scope:** `SUPABASE_URL` config + fail-fast validation, fx-managed pgx pool (no schema), JWKS-backed JWT verifier, chi middleware + context helpers, router protection scope, `/api/me` demo endpoint, full test suite for the above.

**Out of scope:** signup/login UI, `users` table, cookie-based sessions, dev auth bypass, token refresh logic, differentiated 401/403 responses, any use of `SUPABASE_SECRET_KEY`.

## Architecture / Approach

`internal/auth` owns JWKS caching (`jwk.Cache`, app-scoped context tied to fx lifecycle) and a `Verifier.Verify(ctx, token) (userID, error)` that validates signature + `exp`/`aud`/`iss` and extracts `sub`. `internal/server` wires `auth.Middleware(verifier)` onto a chi sub-group covering everything but `/healthz`, exposing the resolved id to handlers via a typed context helper. `internal/db` adds a standalone fx-managed pgx pool alongside, unused by the auth path itself.

## Phases at a Glance

| Phase | What it delivers | Key risk |
| --- | --- | --- |
| 1. Config | `SUPABASE_URL` field + fail-fast `Load()` | Signature change to `Load()` ripples if anything else calls it directly (nothing does yet) |
| 2. Data layer | fx-managed pgx pool, no schema | First real external dependency (Postgres reachability) at boot |
| 3. Auth core | JWKS cache + `Verifier` | Getting `aud`/`iss` validation right — the actual security boundary |
| 4. Middleware + wiring | Route protection + `/api/me` | Chi middleware-scoping gotcha (`Use()` vs `Group()`) |
| 5. Testing | Offline fixture-token test suite | First test pattern in the repo — sets the convention others will copy |

**Prerequisites:** `SUPABASE_URL` must be added to Railway variables before Phase 4's manual verification can run against the deployed environment (it already exists locally).
**Estimated effort:** ~1–2 sessions across 5 phases.

## Open Risks & Assumptions

- Assumes tokens always arrive via `Authorization: Bearer <token>` header, not cookies — consistent with Supabase client SDK conventions; S-01 can revisit if the eventual templ+HTMX login flow needs cookie forwarding instead.
- Assumes Supabase's JWKS endpoint path (`/auth/v1/.well-known/jwks.json`) and claim shape (`aud: "authenticated"`, `iss: <url>/auth/v1`) remain stable — sourced from current Supabase docs, not yet hands-on validated against this specific project.
- `SUPABASE_URL` Railway variable gap must be closed before Phase 4/deploy verification — flagged, not yet actioned.

## Success Criteria (Summary)

- `/healthz` stays public; every other route returns 401 without a valid token and 200 with one.
- `GET /api/me` returns the correct Supabase user id for a real signed-in test user, both locally and on Railway.
- Full test suite (`go test ./...`, `-race`, `golangci-lint run`, `govulncheck ./...`) passes with zero live-network dependency.

# Signup and Onboarding Implementation Plan

## Overview

S-01 on the roadmap: let a climber create an account, sign in, and declare their max boulder grade + MoonBoard set/angle during onboarding. This is the first slice with any UI at all — no `.templ` files, no `static/` assets, and no session-cookie handling exist yet. It also reworks part of the already-shipped F-01 auth middleware, which currently only accepts an `Authorization: Bearer <token>` header (fine for a bare `/api/me` demo endpoint, not workable for a server-rendered HTMX app where the browser needs to carry a session automatically across page loads).

## Current State Analysis

- `internal/auth/middleware.go` — `Middleware(v *Verifier)` extracts `Authorization: Bearer <token>` only, responds 401 JSON on any failure. No cookie support, no session, no refresh.
- `internal/auth/verifier.go` — `Verifier.Verify(ctx, token) (userID string, err error)` is a pure, reusable token→userID function (signature/`aud`/`iss`/`exp` validated against Supabase's JWKS). This does not need to change.
- `internal/config/config.go` — reads `PORT`, `DATABASE_URL`, `APP_ENV`, `SUPABASE_URL`. `SUPABASE_PUBLISHABLE_KEY`/`SUPABASE_SECRET_KEY` exist in `.env`/Railway but are not loaded.
- No `templ` dependency in `go.mod`, no `templates/` or `static/` directory — confirmed by directory listing. `tech-stack.md` already commits to templ+HTMX; this slice is where that commitment first gets exercised.
- No `profiles`-style table exists. Live DB inspection (`\dt`) confirms only `board_editions`, `problems`, `problem_configurations`, `holds`, `problem_moves` in `public`; `auth.users` is Supabase-managed and already populated by GoTrue on signup.
- Catalog reality (queried live): 15 non-blank grades (`5+` through `8B+`, Font scale, shared across all boards — lexical `ORDER BY grade` already sorts them correctly), 4 boards × 2 angles (25°/40°) = 8 combos, all populated. ~37% of `problem_configurations` rows have a blank `grade` (irrelevant here — that's S-03/S-04's filtering concern, not onboarding's).
- No table in this schema uses Postgres RLS (`grep` across `migrations/*.sql` confirms zero `ROW LEVEL SECURITY`/`POLICY` statements) — every table is reached exclusively through the trusted backend via `DATABASE_URL`/pgx, never PostgREST/anon key.
- `github.com/supabase-community/auth-go` (v1.5.0, inspected via `go doc` against the actual installed source) is a real option for the GoTrue REST calls, but its error path (`endpoints/signup.go`) returns only `fmt.Errorf("response status code %d: %s", ...)` — an unstructured string, not a typed error `errors.As` can branch on. That breaks the already-decided requirement to distinguish duplicate-email / invalid-credentials / rate-limited for the HTMX error UI, and fighting the library's opaque error for 4 of its ~40 endpoints isn't worth the dependency. This plan hand-rolls a minimal typed client instead, matching F-01's own precedent of hand-rolling the JWT verifier rather than pulling in a heavier library.
- `github.com/lestrrat-go/jwx/v3 v3.1.1` (already a dependency) exposes `jwt.TokenExpiredError() error`, checked via `errors.Is(err, jwt.TokenExpiredError())`, letting the middleware distinguish "expired, try refresh" from "invalid, reject" without re-parsing claims itself.
- `a-h/templ`'s own documented HTMX pattern (confirmed via current docs) is a form with `hx-post`, `hx-select="#formID"`, `hx-swap="outerHTML"` that re-renders the *whole page* server-side and lets HTMX extract just the form fragment — simpler than a hand-rolled error-slot design and fits the already-decided "inline error, no full navigation" requirement directly.
- HTMX's `HX-Redirect` response header (confirmed via current docs) forces a full client-side navigation and must be paired with a 200 status, not a 3xx — htmx never sees headers on a 3xx because the browser's `XMLHttpRequest` follows the redirect transparently first.

## Desired End State

A climber can visit `/signup`, create an account (email+password), land signed-in on `/onboarding`, pick a max grade + board + angle from catalog-derived dropdowns, and reach `/` (today's placeholder page — S-03 owns building the real hub) with a stored profile. `/signin` and a sign-out action work the same way for a returning user. The session is carried by two `HttpOnly` cookies (access + refresh token); the access token is verified locally via F-01's existing `Verifier` on every request, and transparently refreshed via a GoTrue call only when it has actually expired. A signed-in-but-not-yet-onboarded user is redirected to `/onboarding` from every other authenticated route.

Verification: `go build ./...`, `go test ./...`, `go vet ./...`, `golangci-lint run` all pass; `templ generate` produces no uncommitted diff; manually, a fresh signup → onboarding → landing-on-`/` flow works end-to-end in a browser, and signing out then hitting `/` redirects to `/signin`.

### Key Discoveries:

- `internal/server/server.go:27-46` — the existing `chi.Router` `Group` (auth middleware) is the pattern to extend: this plan splits it into three groups (public, auth-only, auth+onboarding-complete) rather than one.
- `internal/auth/middleware.go:33-47` (`bearerToken`) is being replaced, not extended — cookie extraction is a different shape and the hybrid "accept both" option was explicitly rejected during planning.
- `cmd/server/main.go` registers modules in `fx.New(...)`; the new `profile.Module` needs adding alongside the existing five.
- The `.golangci.yml` `errorlint` rule means every new error (GoTrue client, profile store) must wrap with `%w` and expose typed sentinels (`errors.Is`/`errors.As`), matching F-01's existing convention in `verifier.go`.

## What We're NOT Doing

- No email confirmation flow — Supabase Auth's "Confirm email" setting must be turned off in the project dashboard (an out-of-band, non-code prerequisite — see Migration Notes) so signup always returns an immediate session.
- No password-confirmation field on signup, no password-strength meter beyond what GoTrue itself rejects — FR-001 asks for "email + password," nothing more.
- No special-casing for an already-signed-in user visiting `/signup`/`/signin` (they just see the form again) — not a real failure mode worth building around yet.
- No dynamic board→angle filtering (e.g. an HTMX round-trip that narrows angle options per selected board) — every board currently supports both 25° and 40° (confirmed live), so two independent dropdowns are correct today; revisit only if a future board breaks that symmetry.
- No hub page, no nav bar, no post-onboarding UI beyond redirecting to the existing `/` placeholder — that's S-03's job.
- No Row-Level Security on the new `profiles` table — matches the precedent already shipped on every other table (backend-only access via `DATABASE_URL`/pgx).
- No automated integration tests against a live Postgres — F-01's own pgx-pool phase set this precedent (manual verification only for real-DB behavior); this plan follows it and keeps `profile.Store` consumers (like the onboarding gate) testable via a small interface instead.
- No `SUPABASE_SECRET_KEY` usage — this slice only needs the publishable (anon) key for GoTrue calls, same boundary F-01 already drew.

## Implementation Approach

Seven phases, ordered so each is independently buildable and testable, and so the highest-risk change (reworking F-01's shipped middleware) lands early, before anything depends on its new shape: (1) foundations — config, templ toolchain, static assets, base layout; (2) a hand-rolled Supabase Auth REST client; (3) cookie-based sessions + middleware rework; (4) the `profiles` data layer + onboarding-gate middleware; (5) signup/signin/signout pages; (6) the onboarding page; (7) tests, including updates to F-01's existing test suite for the new cookie-based flow.

## Critical Implementation Details

**Cookie `Secure` flag must be environment-conditional.** Setting `Secure` unconditionally breaks local development: `cfg.AppEnv == "development"` runs over plain `http://localhost`, and browsers refuse to send/accept `Secure` cookies over a non-HTTPS origin. Cookie helpers must set `Secure: cfg.AppEnv != "development"` — `true` in production (Railway terminates TLS in front of the app), `false` locally.

**Generated `_templ.go` files must be committed, not gitignored.** Railway's Railpack build runs a plain `go build` with no custom build command configured (confirmed in `infrastructure.md` — Railpack auto-detects Go from `go.mod`). If `_templ.go` files aren't committed, the deployed build has no generated code to compile against. Every phase that adds `.templ` files must run `templ generate` and commit the output; automated verification includes a "no diff after regenerating" check to catch a forgotten regenerate.

**Middleware failure now means "redirect to `/signin`," not "401 JSON" — except where a JSON consumer still exists.** F-01 built `/api/me` as a bare API-shaped demo endpoint returning 401 JSON on failure; every route this plan adds is an HTML page. The reworked middleware redirects (302, cookies cleared) to `/signin` on any unrecoverable auth failure. `/api/me` keeps working under the same behavior (a curl without `-L` will simply see the redirect instead of a 401 body) — no route-type branching is introduced, keeping F-01's original "one failure shape" principle intact at the new default.

## Phase 1: Foundations — config, templ toolchain, static assets, base layout

### Overview

Everything later phases build on: the publishable-key config value, the templ dependency and generated-code convention, vendored HTMX, and a single shared page shell.

### Changes Required:

#### 1. Config

**File**: `internal/config/config.go`

**Intent**: Load the Supabase publishable (anon) key needed to call GoTrue endpoints in Phase 2. It's already provisioned in `.env`/Railway but unused.

**Contract**: `Config` gains `SupabasePublishableKey string`. `Load()` fails fast (same pattern as `SupabaseURL`) if it's empty.

#### 2. Env example

**File**: `.env.example`

**Intent**: Document the newly-required variable.

**Contract**: Add `SUPABASE_PUBLISHABLE_KEY=<anon-key>`.

#### 3. templ dependency

**File**: `go.mod`

**Intent**: Add the templating library the whole UI layer depends on.

**Contract**: `go get github.com/a-h/templ@latest` (current stable is v0.3.906 as of plan time). Developers additionally run `go install github.com/a-h/templ/cmd/templ@latest` once, matching the `templ generate` command already documented in this repo's `CLAUDE.md`.

#### 4. Vendored HTMX

**File**: `static/htmx.min.js`

**Intent**: Serve HTMX from this app's own origin rather than a CDN, avoiding an external runtime dependency for every page load.

**Contract**: Vendor htmx 2.0.4: `curl -o static/htmx.min.js https://unpkg.com/htmx.org@2.0.4/dist/htmx.min.js`.

#### 5. Static file serving

**File**: `internal/server/server.go`

**Intent**: Serve `static/` at `/static/*`, unprotected — same trust tier as `/healthz`.

**Contract**: `r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))`, registered outside the auth-required group.

#### 6. Base layout

**File**: `templates/layout/layout.templ`

**Intent**: One shared page shell (`<head>` + HTMX script tag + content slot) so every subsequent page avoids re-declaring boilerplate — matches templ's own documented composition pattern (`Page(content templ.Component)`).

**Contract**: `package layout`; `templ Page(title string, content templ.Component) templ.Component` renders `<!DOCTYPE html><html><head><title>{title} · MoonPhase</title><script src="/static/htmx.min.js"></script></head><body>@content</body></html>`.

### Success Criteria:

#### Automated Verification:

- Build passes: `go build ./...`
- `templ generate` produces no uncommitted diff (run it, then `git status --porcelain templates/` is empty)
- `go vet ./...` and `golangci-lint run` pass

#### Manual Verification:

- `curl -I http://localhost:8080/static/htmx.min.js` returns `200` with a non-trivial `Content-Length`.
- Server still boots locally with `SUPABASE_PUBLISHABLE_KEY` set; unsetting it produces a clear startup error (same fail-fast pattern as `SUPABASE_URL`).

---

## Phase 2: Supabase Auth REST client

### Overview

A small, typed client for exactly the 4 GoTrue calls this slice needs — signup, password-grant sign-in, refresh-grant, logout — with structured errors the handlers in Phase 5 can branch on.

### Changes Required:

#### 1. Client + typed errors

**File**: `internal/auth/gotrue.go`

**Intent**: Wrap GoTrue's REST contract (`{code, error_code, msg, error_id}` on failure) in a typed Go error instead of parsing an opaque string, and return a normalized `Session` on success regardless of which endpoint was called.

**Contract**:
```go
type Session struct {
    AccessToken  string
    RefreshToken string
    ExpiresIn    int
    UserID       string
}

type AuthAPIError struct {
    StatusCode int
    ErrorCode  string
    Message    string
}
func (e *AuthAPIError) Error() string { ... } // fmt.Sprintf("auth api: %s (%s)", e.Message, e.ErrorCode)

type AuthClient struct { ... }
func NewAuthClient(cfg config.Config) *AuthClient
func (c *AuthClient) SignUp(ctx context.Context, email, password string) (*Session, error)
func (c *AuthClient) SignInWithPassword(ctx context.Context, email, password string) (*Session, error)
func (c *AuthClient) RefreshSession(ctx context.Context, refreshToken string) (*Session, error)
func (c *AuthClient) SignOut(ctx context.Context, accessToken string) error
```
Each method POSTs to `<cfg.SupabaseURL>/auth/v1/{signup | token?grant_type=password | token?grant_type=refresh_token | logout}` with header `apikey: <cfg.SupabasePublishableKey>` (and `Authorization: Bearer <accessToken>` for `logout`). On a non-2xx response, decode the body into `AuthAPIError` (fields `code`→`StatusCode`, `error_code`→`ErrorCode`, `msg`→`Message`) and return it wrapped with `%w`; if the signup/sign-in response has a 2xx status but an empty `access_token`, return a distinct sentinel error (`errEmailConfirmationEnabled`) — a fail-loud signal that the "Confirm email" dashboard setting wasn't actually turned off (see What We're NOT Doing).

#### 2. Module wiring

**File**: `internal/auth/module.go`

**Intent**: Provide the new client alongside the existing verifier/cache providers.

**Contract**: `fx.Provide(NewKeyCache, NewVerifier, NewAuthClient)`.

### Success Criteria:

#### Automated Verification:

- Build passes: `go build ./...`
- `go vet ./...` and `golangci-lint run` pass

#### Manual Verification:

- N/A for this phase alone — exercised end-to-end once Phase 5 lands, same as F-01's own Phase 3.

---

## Phase 3: Cookie-based sessions + middleware rework

### Overview

The highest-risk phase: replaces F-01's Bearer-header extraction with cookie-based session handling, including transparent refresh. `Verifier.Verify` itself is untouched.

### Changes Required:

#### 1. Session cookie helpers

**File**: `internal/auth/session.go`

**Intent**: Centralize cookie shape so every handler that needs to set/clear a session (signup, signin, refresh, signout) does it identically.

**Contract**:
```go
func SetSessionCookies(w http.ResponseWriter, sess *Session, secure bool)
func ClearSessionCookies(w http.ResponseWriter, secure bool)
func sessionCookies(r *http.Request) (accessToken, refreshToken string, ok bool)
```
Two `HttpOnly`, `SameSite=Lax`, `Path=/` cookies: `mp_session` (access token, `Max-Age` from `sess.ExpiresIn`) and `mp_refresh` (refresh token, no `Max-Age` — a session cookie, cleared explicitly on sign-out rather than time-boxed, since GoTrue owns actual refresh-token expiry server-side). `Secure` is the passed-in `secure bool`, set by the caller from `cfg.AppEnv != "development"` (see Critical Implementation Details).

#### 2. Middleware rework

**File**: `internal/auth/middleware.go`

**Intent**: Extract the session from cookies instead of a header; on an expired (not merely invalid) access token, attempt one transparent refresh before giving up.

**Contract**: `Middleware(v *Verifier, ac *AuthClient, secure bool)` replaces `bearerToken()` with `sessionCookies()`. Flow: read cookies → `v.Verify(ctx, accessToken)` → on success, proceed with `WithUserID`. On failure, check `errors.Is(err, jwt.TokenExpiredError())`: if true and a refresh cookie is present, call `ac.RefreshSession(ctx, refreshToken)`; on success, `SetSessionCookies` with the new session and proceed with the new `UserID`; on any other failure (no refresh cookie, refresh call fails, or the original error wasn't expiry), `ClearSessionCookies` and redirect (302) to `/signin`.

### Success Criteria:

#### Automated Verification:

- Build passes: `go build ./...`
- `go vet ./...` and `golangci-lint run` pass

#### Manual Verification:

- N/A for this phase alone — exercised via the updated test suite in Phase 7 and manually via Phase 5's flow.

---

## Phase 4: Profiles data layer + onboarding gate

### Overview

Storage for max grade + board/angle, and the middleware that redirects a signed-in-but-incomplete user to `/onboarding`.

### Changes Required:

#### 1. Migration

**File**: `migrations/0006_profiles.up.sql` / `.down.sql`

**Intent**: One row per user, FK'd to Supabase's own `auth.users` — the standard Supabase pattern for app-owned profile data.

**Contract**:
```sql
CREATE TABLE profiles (
    id         UUID PRIMARY KEY REFERENCES auth.users (id) ON DELETE CASCADE,
    max_grade  TEXT NOT NULL,
    holdsetup  SMALLINT NOT NULL REFERENCES board_editions (holdsetup),
    angle      SMALLINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```
`down.sql`: `DROP TABLE profiles;`. No RLS (see Current State Analysis / What We're NOT Doing).

#### 2. Catalog read-queries

**File**: `internal/catalog/query.go`

**Intent**: The onboarding form's dropdown options and its server-side submit validation both need the same three lookups; centralize them in the package that already owns catalog data.

**Contract**:
```go
func DistinctGrades(ctx context.Context, pool *pgxpool.Pool) ([]string, error)   // grade <> '', ORDER BY grade
func BoardEditions(ctx context.Context, pool *pgxpool.Pool) ([]BoardEdition, error) // holdsetup, name
func DistinctAngles(ctx context.Context, pool *pgxpool.Pool) ([]int16, error)    // ORDER BY angle
```

#### 3. Profile store

**File**: `internal/profile/profile.go`, `internal/profile/store.go`, `internal/profile/module.go`

**Intent**: Read/write access to the `profiles` table, with a sentinel error the onboarding gate branches on.

**Contract**:
```go
type Profile struct {
    UserID    string
    MaxGrade  string
    Holdsetup int16
    Angle     int16
}

var ErrNotFound = errors.New("profile: not found")

type Store struct { ... }
func NewStore(pool *pgxpool.Pool) *Store
func (s *Store) Get(ctx context.Context, userID string) (*Profile, error) // wraps ErrNotFound on pgx.ErrNoRows
func (s *Store) Upsert(ctx context.Context, p Profile) error              // INSERT ... ON CONFLICT (id) DO UPDATE

var Module = fx.Module("profile", fx.Provide(NewStore))
```

#### 4. Onboarding-gate middleware

**File**: `internal/server/onboarding_gate.go`

**Intent**: Redirect any authenticated request to `/onboarding` when the user has no profile row yet, except for the onboarding route and public/auth routes themselves (avoiding a redirect loop).

**Contract**:
```go
type ProfileChecker interface {
    Get(ctx context.Context, userID string) (*profile.Profile, error)
}
func OnboardingGate(pc ProfileChecker) func(http.Handler) http.Handler
```
On `errors.Is(err, profile.ErrNotFound)`, redirect (302) to `/onboarding`; on any other error, respond 500; otherwise call `next`. `profile.Store` satisfies `ProfileChecker` structurally — the interface exists so Phase 7 can inject a fake without a live DB.

#### 5. Router regrouping

**File**: `internal/server/server.go`, `cmd/server/main.go`

**Intent**: Three trust tiers instead of two: public (`/healthz`, `/static/*`, `/signup`, `/signin`, `/signout`), auth-only (`/onboarding` — needs a user id but must NOT be gated by its own completeness check), and auth+onboarding-complete (everything else, including today's `/` and `/api/me`).

**Contract**: `NewRouter` gains a `ProfileChecker`/`profile.Store` dependency; the existing single `r.Group` becomes two nested groups as described. `cmd/server/main.go` adds `profile.Module` to the `fx.New(...)` list.

### Success Criteria:

#### Automated Verification:

- `go run ./cmd/migrate up` applies cleanly against a scratch DB
- Build passes: `go build ./...`
- `go vet ./...` and `golangci-lint run` pass

#### Manual Verification:

- `psql "$DATABASE_URL" -c "\d profiles"` shows the expected columns and the FK to `auth.users`.
- `go run ./cmd/migrate down` then `up` again round-trips cleanly.

---

## Phase 5: Signup, signin, and signout pages

### Overview

The first real user-facing flow: create an account or sign in, land with a session cookie set.

### Changes Required:

#### 1. Pages

**File**: `templates/pages/signup.templ`, `templates/pages/signin.templ`

**Intent**: Email+password forms using templ's documented HTMX pattern so a failed submit re-renders inline instead of navigating away.

**Contract**: Each form declares `hx-post="/signup"` (or `/signin`), `hx-select="#authForm"`, `hx-swap="outerHTML"`, `id="authForm"`, wraps its own error message (shown only when a `Model.Error` field is non-empty), and is rendered inside `layout.Page(...)`.

#### 2. Handlers

**File**: `internal/server/auth_pages.go`

**Intent**: Wire the forms to `AuthClient` and cookie helpers.

**Contract**: `handleSignupPage`/`handleSigninPage` (GET, render the empty form) and `handleSignupSubmit`/`handleSigninSubmit` (POST): parse form → call the matching `AuthClient` method → on success, `SetSessionCookies` + respond `200` with header `HX-Redirect: /onboarding` (per HTMX's documented header, which requires a non-3xx status to be seen by the client) → on `*auth.AuthAPIError`, re-render the same page with the error inlined (`422`); on the email-confirmation-enabled sentinel, return a `500` with a clear log message (an ops misconfiguration, not a user-facing error state). `handleSignout` (POST): if a session cookie is present, best-effort `ac.SignOut(ctx, accessToken)` (log, don't fail, on error), then `ClearSessionCookies` and `HX-Redirect: /signin`.

#### 3. Router wiring

**File**: `internal/server/server.go`

**Intent**: Register the new routes in the public group (`/signup`, `/signin` GET+POST) and `/signout` (POST, in the auth-only-not-gated tier alongside `/onboarding` — a signed-in user must be able to sign out regardless of onboarding completeness).

### Success Criteria:

#### Automated Verification:

- Build passes: `go build ./...`
- `templ generate` produces no uncommitted diff
- `go vet ./...` and `golangci-lint run` pass

#### Manual Verification:

- With Supabase's "Confirm email" setting off (see Migration Notes): `curl -i -c /tmp/mp.jar -d "email=test1@example.com&password=Str0ngPass!23" http://localhost:8080/signup` returns `200` with an `HX-Redirect: /onboarding` header and a `Set-Cookie: mp_session=...` header.
- Repeating the same signup returns the inline error state (`422`) rather than a crash or a 500.
- `curl -i -b /tmp/mp.jar -d "" http://localhost:8080/signout` returns `HX-Redirect: /signin` and clears both cookies (`Set-Cookie: mp_session=; Max-Age=0`, same for `mp_refresh`).

---

## Phase 6: Onboarding page

### Overview

Max grade + board + angle, gated behind having a session but not yet a profile.

### Changes Required:

#### 1. Page

**File**: `templates/pages/onboarding.templ`

**Intent**: Three single-select dropdowns (grade, board, angle) populated from the catalog queries added in Phase 4.

**Contract**: Same HTMX form pattern as Phase 5 (`hx-post="/onboarding"`, `hx-select`, `hx-swap="outerHTML"`). Options come from `catalog.DistinctGrades`, `catalog.BoardEditions`, `catalog.DistinctAngles`, fetched by the handler and passed into the templ component — no client-side fetching.

#### 2. Handler

**File**: `internal/server/onboarding.go`

**Intent**: Render the form (GET) and validate + persist the submission (POST).

**Contract**: `handleOnboardingPage` (GET): run the three catalog queries, render. `handleOnboardingSubmit` (POST): parse form, validate `max_grade` is in the live `DistinctGrades` set, `holdsetup` is a valid `BoardEditions` id, and `angle` is in the live `DistinctAngles` set (boundary validation against user input, per the same catalog data the dropdowns were built from) → on any invalid value, re-render with an inline error (`422`) → on success, `profile.Store.Upsert` → `200` + `HX-Redirect: /`.

### Success Criteria:

#### Automated Verification:

- Build passes: `go build ./...`
- `templ generate` produces no uncommitted diff
- `go vet ./...` and `golangci-lint run` pass

#### Manual Verification:

- Copy-pasteable end-to-end flow (assumes a fresh signup already produced `/tmp/mp.jar` per Phase 5's manual step):
  ```sh
  curl -i -b /tmp/mp.jar -c /tmp/mp.jar http://localhost:8080/onboarding
  # expect: 200, page includes <option>5+</option> ... <option>8B+</option>, and board options incl. "2016"/"2024"/"Masters 2017"/"Masters 2019"

  curl -i -b /tmp/mp.jar -c /tmp/mp.jar -d "max_grade=6B&holdsetup=1&angle=25" http://localhost:8080/onboarding
  # expect: 200 with HX-Redirect: /

  psql "$DATABASE_URL" -c "SELECT max_grade, holdsetup, angle FROM profiles;"
  # expect: one row — 6B | 1 | 25
  ```
- Hitting `/` with the same cookie jar (`curl -i -b /tmp/mp.jar http://localhost:8080/`) now returns `200` instead of redirecting to `/onboarding`.
- A fresh signup (new cookie jar, no profile yet) hitting `/` or `/api/me` redirects (302) to `/onboarding` instead of serving the page.

---

## Phase 7: Testing

### Overview

Brings the new code up to the same offline, deterministic testing standard F-01 established, and updates F-01's own tests for the cookie-based flow they now need to exercise.

### Changes Required:

#### 1. Fake GoTrue server

**File**: `internal/auth/gotrue_test.go`

**Intent**: Mirror F-01's fake-JWKS `httptest.Server` pattern for GoTrue, scripting both success and the specific error shapes the UI branches on.

**Contract**: A `startFakeGoTrue(t, scripted map[string]response)`-style helper serving `/signup`, `/token`, `/logout` with configurable status/body per test case. Cases: signup success → `Session`; signup duplicate email (`422`, `error_code: "user_already_exists"`) → `*AuthAPIError`; signup with empty `access_token` in a 2xx body → the email-confirmation sentinel; sign-in success; sign-in bad credentials (`400`, `invalid_credentials`); refresh success; refresh with an invalid token (`401`); logout success.

#### 2. Session + middleware tests

**File**: `internal/auth/session_test.go`, `internal/auth/middleware_test.go` (rewritten)

**Intent**: Cookie round-trip, and the full middleware decision tree.

**Contract**: Cases: valid access-token cookie → 200, `UserIDFromContext` populated; no cookies → 302 to `/signin`; expired access token + valid refresh cookie → transparent refresh, new `Set-Cookie` headers, 200, correct (possibly new) user id; expired access token + missing/invalid refresh cookie → 302 to `/signin`, cookies cleared; malformed (non-expiry) invalid access token → 302 to `/signin` without attempting a refresh call.

#### 3. Profile + gate tests

**File**: `internal/server/onboarding_gate_test.go`

**Intent**: Exercise `OnboardingGate` against a fake `ProfileChecker` — no live DB, per the precedent set in What We're NOT Doing.

**Contract**: Cases: `ErrNotFound` → 302 to `/onboarding`; existing profile → `next` called; other error → 500.

#### 4. Server integration test

**File**: `internal/server/server_test.go` (updated)

**Intent**: Prove the three-tier route grouping from Phase 4, replacing F-01's Bearer-header-based assertions with cookie-based ones.

**Contract**: Using a fake JWKS server (F-01's existing helper) + a fake `ProfileChecker`: `/healthz`/`/static/*` → 200 with no cookies; `/`/`/api/me` with no cookies → 302 to `/signin`; `/`/`/api/me` with a valid session cookie but no profile → 302 to `/onboarding`; `/onboarding` with a valid session cookie and no profile → 200 (not redirected); `/`/`/api/me` with a valid session cookie and an existing profile → 200.

### Success Criteria:

#### Automated Verification:

- All tests pass: `go test ./...`
- Race detector clean: `go test -race ./...`
- Full CI gate: `go vet ./...`, `golangci-lint run`, `govulncheck ./...`

#### Manual Verification:

- Test suite confirmed to run with no live Supabase or live-Postgres network dependency (same standard as F-01 Phase 5).

---

## Testing Strategy

### Unit Tests:

- `internal/auth`: GoTrue client error decoding (`AuthAPIError` shape, email-confirmation sentinel), session cookie set/clear/read, middleware's expired-vs-invalid branching and refresh flow.
- `internal/server`: onboarding-gate redirect logic against a fake `ProfileChecker`.

### Integration Tests:

- `internal/server`: full router assembly proving the three-tier route grouping (public / auth-only / auth+onboarding-complete).

### Manual Testing Steps:

1. Turn off "Confirm email" in the Supabase project's Auth settings (see Migration Notes) before any manual testing below.
2. Run the copy-pasteable `curl` sequence in Phase 5 and Phase 6's Manual Verification, in order, using the same cookie jar throughout.
3. Open `/signup` in an actual browser, submit a duplicate email, confirm the error appears inline without a full page navigation (network tab shows an XHR, not a document load).
4. Sign out, then attempt to load `/` directly — confirm the browser is redirected to `/signin`.

## Performance Considerations

The refresh path only calls GoTrue when the local JWKS-based verification specifically reports expiry — normal requests never make a network call, preserving F-01's original no-per-request-network-dependency property.

## Migration Notes

**Out-of-band prerequisite**: In the Supabase dashboard (Authentication → Settings), "Confirm email" must be disabled before this slice can be manually verified or used — this is a project setting, not something these code changes can enforce. If it's ever re-enabled, `AuthClient.SignUp`'s empty-access-token sentinel (Phase 2) will surface the mismatch loudly (a 500 with a clear log line) rather than silently misbehaving.

No existing data to migrate — `profiles` is a new, empty table.

## References

- Roadmap: `context/foundation/roadmap.md` (S-01)
- PRD: `context/foundation/prd.md` (FR-001, FR-002, FR-003, US-01)
- Prerequisite: `context/changes/auth-session-scaffold/plan.md` (F-01, impl_reviewed)
- Existing middleware: `internal/auth/middleware.go`
- Existing router: `internal/server/server.go`
- Existing verifier (unchanged): `internal/auth/verifier.go`

## Progress

> Convention: `- [ ]` pending, `- [x]` done. Append ` — <commit sha>` when a step lands. Do not rename step titles. See `references/progress-format.md`.

### Phase 1: Foundations — config, templ toolchain, static assets, base layout

#### Automated

- [x] 1.1 Build passes: `go build ./...` — 4248bc4
- [x] 1.2 `templ generate` produces no uncommitted diff — 4248bc4
- [x] 1.3 `go vet ./...` and `golangci-lint run` pass — 4248bc4

#### Manual

- [x] 1.4 `curl -I /static/htmx.min.js` returns 200 — 4248bc4
- [x] 1.5 Server boots with `SUPABASE_PUBLISHABLE_KEY` set; fails fast without it — 4248bc4

### Phase 2: Supabase Auth REST client

#### Automated

- [x] 2.1 Build passes: `go build ./...` — 5b96a3c
- [x] 2.2 `go vet ./...` and `golangci-lint run` pass — 5b96a3c

### Phase 3: Cookie-based sessions + middleware rework

#### Automated

- [x] 3.1 Build passes: `go build ./...` — c4bfed1
- [x] 3.2 `go vet ./...` and `golangci-lint run` pass — c4bfed1

### Phase 4: Profiles data layer + onboarding gate

#### Automated

- [x] 4.1 `go run ./cmd/migrate up` applies cleanly — 5753d96
- [x] 4.2 Build passes: `go build ./...` — 5753d96
- [x] 4.3 `go vet ./...` and `golangci-lint run` pass — 5753d96

#### Manual

- [x] 4.4 `psql "$DATABASE_URL" -c "\d profiles"` shows expected schema + FK — 5753d96
- [x] 4.5 `migrate down` then `up` round-trips cleanly — 5753d96

### Phase 5: Signup, signin, and signout pages

#### Automated

- [x] 5.1 Build passes: `go build ./...` — d4b5113
- [x] 5.2 `templ generate` produces no uncommitted diff — d4b5113
- [x] 5.3 `go vet ./...` and `golangci-lint run` pass — d4b5113

#### Manual

- [x] 5.4 Signup returns `HX-Redirect: /onboarding` + `Set-Cookie: mp_session` — d4b5113
- [x] 5.5 Duplicate signup returns inline 422 error, not a crash — d4b5113
- [x] 5.6 Sign-out returns `HX-Redirect: /signin` and clears both cookies — d4b5113

### Phase 6: Onboarding page

#### Automated

- [x] 6.1 Build passes: `go build ./...` — 27fd112
- [x] 6.2 `templ generate` produces no uncommitted diff — 27fd112
- [x] 6.3 `go vet ./...` and `golangci-lint run` pass — 27fd112

#### Manual

- [x] 6.4 GET `/onboarding` lists real catalog grade/board options — 27fd112
- [x] 6.5 POST `/onboarding` persists the row (`psql` check) and redirects to `/` — 27fd112
- [x] 6.6 `/` now returns 200 (not redirected) with the same cookie jar — 27fd112
- [x] 6.7 A fresh signup with no profile is redirected to `/onboarding` from `/` and `/api/me` — 27fd112

### Phase 7: Testing

#### Automated

- [x] 7.1 All tests pass: `go test ./...` — deb7785
- [x] 7.2 Race detector clean: `go test -race ./...` — deb7785
- [x] 7.3 Full CI gate: `go vet ./...`, `golangci-lint run`, `govulncheck ./...` (govulncheck reports 6 pre-existing Go-toolchain/x-text vulnerabilities unrelated to this change — confirmed present as of Phase 4's commit 5753d96, before any signup-and-onboarding code touched the affected files; out of this plan's scope) — deb7785

#### Manual

- [x] 7.4 Test suite confirmed to run with no live Supabase/Postgres dependency — deb7785

# Signup and Onboarding — Plan Brief

> Full plan: `context/changes/signup-and-onboarding/plan.md`

## What & Why

S-01 on the roadmap: let a climber create an account, sign in, and declare their max boulder grade + MoonBoard set/angle during onboarding. This is the plumbing US-01 assumes is already in place ("an authenticated climber with a stated max grade") — nothing downstream (starting a session, the adaptive loop, history) can be built or tested against a real user until this exists.

## Starting Point

F-01 (`auth-session-scaffold`) is done and shipped: JWT verification middleware resolves a Supabase-issued `Authorization: Bearer <token>` to a user id. But there's no UI anywhere in the repo yet — no `templ` dependency, no `templates/`/`static/` directories — and the middleware only understands header-based tokens, not the cookie-based session a server-rendered HTMX app needs. No `profiles` table exists either; only Supabase's own `auth.users` does.

## Desired End State

A climber visits `/signup`, creates an account, lands on `/onboarding`, picks a max grade + board + angle from catalog-derived dropdowns, and reaches `/` with a stored profile. Returning users use `/signin`. The session lives in two `HttpOnly` cookies, verified locally on every request and transparently refreshed only when actually expired. Anyone signed in but not yet onboarded is redirected to `/onboarding` from every other page.

## Key Decisions Made

| Decision | Choice | Why (1 sentence) | Source |
| --- | --- | --- | --- |
| Session transport | HTTP-only cookie, not header/localStorage | Matches the server-rendered HTMX pattern; browser sends it automatically, no client JS needed. | Plan |
| Backend↔Supabase integration | Backend calls GoTrue REST directly (hand-rolled client, not the community Go SDK) | The SDK's v1.5.0 errors are unstructured strings that can't drive the duplicate-email/bad-password/rate-limit UI split this plan needs. | Plan |
| Onboarding flow | Two steps: signup, then a gated `/onboarding` screen | Matches how FR-003 is worded, and lets an abandoned onboarding resume cleanly instead of leaving a half-created account. | Plan |
| Grade / board / angle options | Derived live from the catalog (`DISTINCT grade`, `board_editions`, `DISTINCT angle`) | Catalog is already the PRD's single source of truth (FR-015); no separate list to drift. | Plan |
| Onboarding enforcement | Central redirect middleware (`OnboardingGate`), not per-handler checks | Fail-closed by default, matching F-01's own pattern — can't be forgotten on a future page. | Plan |
| Session longevity | Auto-refresh via stored refresh token, not re-login on expiry | A 120-min Main Session shouldn't get logged out mid-wall; this is the one experience the PRD guardrails protect. | Plan |
| Email confirmation | Off — immediate sign-in after signup | Keeps this slice at "plumbing" scope; FR-001/002 don't mention a confirmation step. | Plan |
| Row-Level Security on `profiles` | None | Matches the precedent already shipped on every other table — access is backend-only via pgx. | Plan |
| CSRF defense | `SameSite=Lax` cookie attribute, no separate token | Built into the cookie-setting code already, no extra middleware for a single-origin app. | Plan |
| Sign-out | Clears cookies + calls GoTrue `/logout` to revoke the refresh token | A "sign out" that doesn't revoke server-side isn't really sign out. | Plan |
| Error UX | HTMX partial swap (templ's own `hx-select`/`hx-swap="outerHTML"` pattern) | Errors appear inline without a full navigation — the actual reason HTMX was chosen. | Plan |
| Testing | Fake GoTrue `httptest` server, no live Supabase/Postgres in tests | Mirrors F-01's own fake-JWKS precedent; keeps CI offline and deterministic. | Plan |
| Shared layout | One minimal `layout.templ` from day one | The 3 pages in this slice already need it to avoid duplicating `<head>`/HTMX-script boilerplate. | Plan |

## Scope

**In scope:** signup/signin/signout pages and handlers, cookie-based session with transparent refresh (reworking F-01's middleware), `profiles` table + store, onboarding page + gate middleware, a minimal shared page layout, vendored HTMX, full offline test coverage of the above.

**Out of scope:** email confirmation flow, password-confirmation field, dynamic board→angle filtering, the real hub/nav UI (S-03), Row-Level Security, live-DB/live-Supabase integration tests, `SUPABASE_SECRET_KEY` usage.

## Architecture / Approach

`internal/auth` grows a hand-rolled GoTrue REST client (`gotrue.go`) and cookie helpers (`session.go`), and its existing `Middleware` is reworked to read cookies instead of a header, verifying locally via the untouched `Verifier` and falling back to a refresh call only on detected expiry (`errors.Is(err, jwt.TokenExpiredError())`). A new `internal/profile` package owns the `profiles` table (FK'd to Supabase's `auth.users`); `internal/server` adds an `OnboardingGate` middleware and regroups routes into three trust tiers: public, auth-only-not-gated (`/onboarding`, `/signout`), and auth+onboarding-complete (everything else). `internal/catalog` gains three small read-queries so both the onboarding dropdowns and its own server-side validation share one source of truth. Pages are templ components using templ's documented `hx-post`/`hx-select`/`hx-swap="outerHTML"` idiom, with `HX-Redirect` driving navigation on success.

## Phases at a Glance

| Phase | What it delivers | Key risk |
| --- | --- | --- |
| 1. Foundations | Config, templ toolchain, vendored HTMX, base layout | First-ever frontend scaffold in the repo — sets conventions everything else follows |
| 2. Auth REST client | Typed signup/signin/refresh/logout calls to GoTrue | Hand-rolled REST client instead of the community SDK — more code, but structured errors |
| 3. Cookie + middleware rework | Session cookies, transparent refresh | Reworks F-01's already-shipped, already-tested middleware |
| 4. Profiles + gate | New table, store, `OnboardingGate` | Router regrouping into 3 trust tiers — easy to mis-scope a route |
| 5. Auth pages | Signup/signin/signout, cookie issuance | Getting `HX-Redirect` + non-3xx status right for HTMX to see it |
| 6. Onboarding page | Grade/board/angle form, profile upsert | Server-side validation must reuse the same catalog queries as the dropdowns |
| 7. Testing | Fake GoTrue server, updated F-01 tests, gate tests | F-01's existing middleware/server tests need real rewrites, not just additions |

**Prerequisites:** F-01 (`auth-session-scaffold`) is done. Before manual verification: disable "Confirm email" in the Supabase project's Auth dashboard settings (an out-of-band step this plan cannot automate).
**Estimated effort:** ~3-4 sessions across 7 phases — this is the first UI slice, so Phase 1 and Phase 3 carry more one-time setup/rework cost than later slices will.

## Open Risks & Assumptions

- Assumes "Confirm email" gets turned off in the Supabase dashboard before manual testing; if it's ever re-enabled, signup will 500 loudly (by design) rather than silently break — see Migration Notes in the full plan.
- Assumes every board continues to support both 25° and 40° angles (true for all 4 boards today, confirmed via a live query); a future board with only one angle would need the onboarding form revisited.
- Assumes `SameSite=Lax` is sufficient CSRF defense for this single-origin Railway deployment — would need revisiting if the app ever serves from multiple origins or embeds third-party content.

## Success Criteria (Summary)

- A climber can go from `/signup` to a stored profile (`max_grade`, `holdsetup`, `angle`) to landing on `/` in one continuous flow, with errors shown inline and no full page navigations.
- A signed-in user without a completed profile is redirected to `/onboarding` from any other page; a completed profile is never re-prompted.
- Signing out actually revokes the session (GoTrue `/logout`) and the browser can no longer reach protected pages without signing in again.

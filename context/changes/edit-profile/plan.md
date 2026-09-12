# Edit Profile Implementation Plan

## Overview

Implement roadmap slice S-02 (PRD FR-004): an authenticated, already-onboarded user can view and edit their max grade and MoonBoard set/angle from a `/profile` screen. The implementation closely mirrors the existing onboarding flow (`internal/server/onboarding.go`, `templates/pages/onboarding.templ`), differing only in that it pre-fills the form with the user's current values and is reached post-onboarding.

## Current State Analysis

- `internal/profile/store.go` already exposes `Store.Get(ctx, userID) (*Profile, error)` and `Store.Upsert(ctx, p Profile) error` (single INSERT...ON CONFLICT DO UPDATE writing all three fields at once — no partial-update method exists, and none is needed).
- `internal/server/onboarding.go` implements the exact handler shape needed: `loadModel` (three `catalog.*` reads), `handlePage` (GET, render), `handleSubmit` (POST, parse+validate+upsert+redirect), plus three validator functions (`gradeValid`, `boardValid`, `angleValid`) that currently have zero test coverage.
- `templates/pages/onboarding.templ` + `templates/pages/model.go` (`OnboardingModel`) implement the exact form markup needed, but never mark any `<option>` as pre-selected — new users have no prior value, so this capability doesn't exist yet.
- `internal/server/server.go`'s `NewRouter` has three chi tiers: public, auth-only (`/signout`, `/onboarding`), and auth+`OnboardingGate` (`/`, `/api/me`). `OnboardingGate` (`internal/server/onboarding_gate.go`) redirects an authenticated-but-profile-less user to `/onboarding`, and guarantees any route inside its group has a resolved profile.
- `internal/server/server_test.go` proves the three-tier gating behavior with a DB-free `newTestRouter` (trivial stand-in handlers + a hand-written `fakeProfileChecker`), not the real handlers — the real handler bodies (onboarding's included) have no automated test coverage today; that gap is closed only by manual verification.

## Desired End State

A signed-in, onboarded user can GET `/profile` and see their current max grade, board, and angle pre-selected in three dropdowns; submitting the form with new values upserts the profile and redirects to `/`; submitting invalid values re-renders the form with an inline error, still showing the previously-stored values selected. `/profile` is unreachable without a session (redirects to `/signin`) and unreachable without a profile (redirects to `/onboarding`, via the existing `OnboardingGate`).

Verify via: `go build ./...`, `templ generate`, `go vet ./...`, `golangci-lint run`, `go test ./internal/server/...`, plus the manual steps in each phase below.

### Key Discoveries:

- `internal/server/onboarding.go:115-140` — the three validators to extract are pure, stateless, and structurally trivial (`for` + `==` over a slice); no behavior change on extraction, just a location move.
- `internal/server/server.go:60-69` — the `OnboardingGate`-wrapped inner `r.Group` is where `/profile` belongs, not the auth-only tier at lines 50-58 (`/signout`, `/onboarding`).
- `templates/pages/onboarding.templ:16-36` — the three `<select>` blocks to mirror; templ's boolean-attribute syntax `attr?={ expr }` (confirmed against `a-h/templ`'s generator test fixtures) is what adds pre-selection without restructuring the existing `for` loops.
- `internal/server/server_test.go:119-150` — `newTestRouter` mirrors `NewRouter`'s tiering with stand-in handlers specifically to avoid a live Postgres pool; extending it (not the real `NewRouter`) is how `/profile`'s gating gets proven without new test infrastructure.

## What We're NOT Doing

- No new `profile.Store` method — `Upsert` already writes all three fields in one call.
- No handler-level unit tests for `handlePage`/`handleSubmit` — they're inherently coupled to a concrete `*pgxpool.Pool`-backed `*profile.Store` and to `catalog.*` DB queries, exactly like onboarding's untested equivalents. Covered by manual verification instead, matching existing project precedent.
- No new abstraction/interface layer over `profile.Store` or the catalog queries — out of scope for this PRD-characterized "plumbing" slice.
- No inline "Saved" success-message UX — redirect-on-save (`HX-Redirect: /`) matches every other form in the app (signup, signin, onboarding).
- No changes to `OnboardingGate`'s redirect logic — a missing profile inside `/profile` is treated as a bug surfaced via 500, not re-handled here.

## Implementation Approach

Three phases, each independently buildable and gated by the previous one only where necessary:

1. Extract the validators (zero behavior change, adds their first tests) — a safe, isolated first step.
2. Add the view layer (`ProfileModel` + `profile.templ`) — no server wiring yet, so `templ generate` output can be checked in isolation.
3. Add the handler, wire the route, and extend router-level gating tests — the integration point that ties everything together.

## Phase 1: Extract catalog validators

### Overview

Move `gradeValid`, `boardValid`, `angleValid` out of `onboarding.go` into a new shared file, and add the unit tests they've never had. Pure refactor — no behavior change, no new functionality yet.

### Changes Required:

#### 1. New shared validators file

**File**: `internal/server/catalog_validation.go`

**Intent**: Relocate the three validator functions so both `onboarding.go` and the new `profile_edit.go` (Phase 3) can call them without duplication.

**Contract**: Same package (`server`), same three function signatures as they currently have in `onboarding.go:115-140` (`gradeValid([]string, string) bool`, `boardValid([]catalog.BoardEdition, int16) bool`, `angleValid([]int16, int16) bool`) — a verbatim move, no signature or logic changes.

#### 2. Remove validators from onboarding.go

**File**: `internal/server/onboarding.go`

**Intent**: Delete the now-relocated function bodies; everything else in the file (handler logic, `loadModel`) is unchanged and continues to call the same function names, now resolved from `catalog_validation.go` via the shared package scope.

**Contract**: File shrinks by the three function definitions (`onboarding.go:115-140`); no other lines change.

#### 3. Validator unit tests

**File**: `internal/server/catalog_validation_test.go`

**Intent**: First-ever automated coverage for these three functions — true and false cases each, matching the project's discrete `Test<Scenario>` house style (see `internal/server/onboarding_gate_test.go`).

**Contract**: At minimum one passing-case and one failing-case test per function (`TestGradeValid_Found`, `TestGradeValid_NotFound`, and the equivalent pair for `boardValid`/`angleValid`), using plain literal slices — no fakes or fixtures needed since these functions take no DB/context arguments.

### Success Criteria:

#### Automated Verification:

- `go build ./...` compiles cleanly
- `go vet ./...` passes
- `golangci-lint run` passes
- `go test ./internal/server/... -run 'Valid'` passes (new validator tests)
- `go test ./internal/server/...` passes in full (confirms `onboarding.go` still compiles and its existing tests, if any exercise these paths indirectly, still pass)

#### Manual Verification:

- Skip — this phase is a pure internal refactor with no user-facing surface; automated verification is sufficient.

---

## Phase 2: Profile view (model + templ page)

### Overview

Add the `ProfileModel` struct and the `profile.templ` page, mirroring `onboarding.templ`'s structure with pre-selection added. No server-side wiring in this phase — verify via `templ generate` and a build, not a running server.

### Changes Required:

#### 1. Profile model

**File**: `templates/pages/model.go`

**Intent**: Carry the catalog-derived dropdown options plus the user's current values (for pre-selection) and any validation error, for the profile-edit form.

**Contract**:
```go
type ProfileModel struct {
	Grades []string
	Boards []catalog.BoardEdition
	Angles []int16

	CurrentGrade     string
	CurrentHoldsetup int16
	CurrentAngle     int16

	Error string
}
```
Added below the existing `OnboardingModel` (untouched) in the same file.

#### 2. Profile templ page

**File**: `templates/pages/profile.templ`

**Intent**: Render a form structurally identical to `onboarding.templ`'s (`<form id="authForm" hx-post="/profile" hx-select="#authForm" hx-swap="outerHTML">`, same three `<label><select>` blocks, same error-paragraph pattern), with each `<option>` additionally marked `selected?=` against the model's `Current*` fields.

**Contract**: Two templ components — `profileForm(model ProfileModel)` and `ProfilePage(model ProfileModel)` (wrapping via `layout.Page("Profile", profileForm(model))`, same pattern as `OnboardingPage`). The three `<select>` blocks use templ's boolean-attribute syntax:
```templ
<option value={ g } selected?={ g == model.CurrentGrade }>{ g }</option>
...
<option value={ strconv.Itoa(int(b.Holdsetup)) } selected?={ b.Holdsetup == model.CurrentHoldsetup }>{ b.Name }</option>
...
<option value={ strconv.Itoa(int(a)) } selected?={ a == model.CurrentAngle }>{ strconv.Itoa(int(a)) }°</option>
```
`hx-post` targets `/profile` (not `/onboarding`); submit button label can stay "Continue" or change to "Save" — implementer's call, no functional difference.

### Success Criteria:

#### Automated Verification:

- `templ generate` regenerates `templates/pages/profile_templ.go` without errors
- `go build ./...` compiles cleanly (confirms `ProfileModel` and the generated templ components type-check together)
- `go vet ./...` passes
- `golangci-lint run` passes

#### Manual Verification:

- Skip — no server route exists yet to render this page against a browser; visual verification happens in Phase 3's manual steps once the handler wires it up.

---

## Phase 3: Handler, routing, and gating tests

### Overview

Add `profilePages` (the handler), wire `/profile` into the `OnboardingGate`-protected router tier, and extend the existing DB-free router test to prove `/profile`'s gating behavior. This is the phase where the feature becomes reachable and testable end-to-end.

### Changes Required:

#### 1. Profile handler

**File**: `internal/server/profile_edit.go`

**Intent**: Serve GET `/profile` (load the current profile + catalog options, render pre-filled) and handle POST `/profile` (parse, validate, upsert, redirect), following `onboardingPages`' exact shape in `onboarding.go` with two differences: `loadModel` takes the current `profile.Profile` and folds its values into `Current*` on the returned `pages.ProfileModel`, and both `handlePage` and `handleSubmit` fetch that current profile via `store.Get` before building the model (both the GET pre-fill path and the invalid-POST re-render path need it, so a rejected submission still shows the user's *stored* values selected, not their invalid submission).

**Contract**:
- `type profilePages struct { pool *pgxpool.Pool; store *profile.Store; logger *zerolog.Logger }`, `func newProfilePages(pool *pgxpool.Pool, store *profile.Store, logger *zerolog.Logger) *profilePages`
- `func (p *profilePages) loadModel(ctx context.Context, current profile.Profile) (pages.ProfileModel, error)` — same three `catalog.DistinctGrades`/`catalog.BoardEditions`/`catalog.DistinctAngles` calls as `onboardingPages.loadModel`, then sets `CurrentGrade: current.MaxGrade, CurrentHoldsetup: current.Holdsetup, CurrentAngle: current.Angle` on the result.
- `func (p *profilePages) handlePage(w http.ResponseWriter, r *http.Request)` — GET: resolve `userID` via `auth.UserIDFromContext` (500 if missing, same message as `onboarding.go:96`); `p.store.Get(ctx, userID)` (log + 500 on any error, including `profile.ErrNotFound` — no redirect logic duplicated here, see "What We're NOT Doing"); `p.loadModel(ctx, *current)`; `renderPage(w, r, pages.ProfilePage(model), http.StatusOK)`.
- `func (p *profilePages) handleSubmit(w http.ResponseWriter, r *http.Request)` — POST: `http.MaxBytesReader` + `r.ParseForm()` (400 on error, same as `onboarding.go:60-64`); resolve `userID` + fetch current profile (same as GET, 500 on failure) *before* validation, so `model.Current*` is available for the invalid-re-render path; `p.loadModel(ctx, *current)`; read+validate `max_grade`/`holdsetup`/`angle` via the Phase 1 validators, exact same pattern as `onboarding.go:73-92` (422 + `pages.ProfilePage(model)` re-render on any invalid field); on all-valid, `p.store.Upsert(ctx, profile.Profile{UserID: userID, MaxGrade: maxGrade, Holdsetup: int16(holdsetup), Angle: int16(angle)})` (log + 500 on error); on success, `w.Header().Set("HX-Redirect", "/")` + `w.WriteHeader(http.StatusOK)`.

#### 2. Route wiring

**File**: `internal/server/server.go`

**Intent**: Make `/profile` reachable only for authenticated, onboarded users.

**Contract**: Instantiate `pp := newProfilePages(pool, profileStore, logger)` alongside the existing `op := newOnboardingPages(...)` (line 43); add `r.Get("/profile", pp.handlePage)` and `r.Post("/profile", pp.handleSubmit)` inside the inner `r.Group` at lines 60-69 (the one with `r.Use(OnboardingGate(profileStore))`), alongside `/` and `/api/me` — not the outer auth-only group that holds `/onboarding`/`/signout`.

#### 3. Router-level gating tests

**File**: `internal/server/server_test.go`

**Intent**: Prove `/profile` is gated identically to `/` and `/api/me` (redirects to `/signin` without a session, redirects to `/onboarding` without a profile, reaches the handler with both) using the existing DB-free test router, without needing a real `profilePages`/DB-backed handler.

**Contract**: In `newTestRouter` (lines 124-150), add a trivial stand-in `r.Get("/profile", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })` inside the `OnboardingGate` group (alongside the existing `/` and `/api/me` stand-ins). Add `"/profile"` to the `[]string{...}` path lists in `TestRouter_ProtectedRoutesRedirectWithoutSession` (line 168), `TestRouter_SessionWithoutProfileRedirectsToOnboarding` (line 186), and `TestRouter_SessionWithProfileReachesProtectedRoutes` (line 221).

### Success Criteria:

#### Automated Verification:

- `go build ./...` compiles cleanly
- `go vet ./...` passes
- `golangci-lint run` passes
- `go test ./internal/server/...` passes, including the three extended `TestRouter_*` cases now covering `/profile`

#### Manual Verification:

- Run `go run ./cmd/server` against a dev DB; sign in as a test user who already completed onboarding (has a `profiles` row)
- GET `/profile` in a browser — confirm the current max grade, board, and angle appear pre-selected in the three dropdowns (not blank, not defaulted to the first option)
- Change one or more values and submit — confirm the browser redirects to `/`, then re-visit `/profile` and confirm the new values are now the ones pre-selected (persisted correctly)
- Using browser devtools, submit a POST to `/profile` with an out-of-range `holdsetup` value (e.g. `999`) — confirm a 422 response re-rendering the form with an inline error message, and that the dropdowns still show the user's *previously stored* values selected, not the tampered submission
- In a private/incognito window (no session cookie), request `/profile` directly — confirm a redirect to `/signin`
- If a not-yet-onboarded test account is available, sign in as that account and request `/profile` — confirm a redirect to `/onboarding` (proves `OnboardingGate` still applies)

---

## Testing Strategy

### Unit Tests:

- `catalog_validation_test.go` (Phase 1): true/false cases for `gradeValid`, `boardValid`, `angleValid`.

### Integration Tests:

- Extended `server_test.go` `TestRouter_*` cases (Phase 3): prove `/profile`'s three-tier gating (no session → `/signin`; session, no profile → `/onboarding`; session + profile → reaches handler) via the existing DB-free `newTestRouter`, consistent with how `/` and `/api/me` are already covered.

### Manual Testing Steps:

See Phase 3's Manual Verification list — pre-fill correctness, successful save + persistence, invalid-submission re-render with stored values retained, and the two auth/onboarding gating checks.

## Performance Considerations

None beyond what onboarding already does — same three catalog queries per page load, same single upsert per save. No new N+1 or unbounded-query risk introduced.

## Migration Notes

None — no schema change. `profiles` table (`migrations/0006_profiles.up.sql`) already has all three columns this feature reads/writes.

## References

- Roadmap: `context/foundation/roadmap.md`, S-02 (line 105-115)
- PRD: `context/foundation/prd.md`, FR-004
- Pattern source: `internal/server/onboarding.go`, `templates/pages/onboarding.templ`, `templates/pages/model.go`
- Routing/gating precedent: `internal/server/server.go`, `internal/server/onboarding_gate.go`, `internal/server/server_test.go`

## Progress

> Convention: `- [ ]` pending, `- [x]` done. Append ` — <commit sha>` when a step lands. Do not rename step titles.

### Phase 1: Extract catalog validators

#### Automated

- [x] 1.1 `go build ./...` compiles cleanly — 08e334e
- [x] 1.2 `go vet ./...` passes — 08e334e
- [x] 1.3 `golangci-lint run` passes — 08e334e
- [x] 1.4 `go test ./internal/server/... -run 'Valid'` passes — 08e334e
- [x] 1.5 `go test ./internal/server/...` passes in full — 08e334e

### Phase 2: Profile view (model + templ page)

#### Automated

- [x] 2.1 `templ generate` regenerates `profile_templ.go` without errors — ad1b4ea
- [x] 2.2 `go build ./...` compiles cleanly — ad1b4ea
- [x] 2.3 `go vet ./...` passes — ad1b4ea
- [x] 2.4 `golangci-lint run` passes — ad1b4ea

### Phase 3: Handler, routing, and gating tests

#### Automated

- [x] 3.1 `go build ./...` compiles cleanly — 7f44835
- [x] 3.2 `go vet ./...` passes — 7f44835
- [x] 3.3 `golangci-lint run` passes — 7f44835
- [x] 3.4 `go test ./internal/server/...` passes, including extended `/profile` gating cases — 7f44835

#### Manual

- [x] 3.5 GET `/profile` shows current max grade/board/angle pre-selected — 7f44835
- [x] 3.6 Submitting new values redirects to `/` and persists (re-visit confirms new values selected) — 7f44835
- [x] 3.7 Submitting a tampered/invalid field re-renders with 422 + inline error, retaining previously-stored selections — 7f44835
- [x] 3.8 Unauthenticated request to `/profile` redirects to `/signin` — 7f44835
- [x] 3.9 Signed-in-but-not-onboarded request to `/profile` redirects to `/onboarding` — 7f44835

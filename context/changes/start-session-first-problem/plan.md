# Start Session, First Problem — Implementation Plan

## Overview

Roadmap slice **S-03** (PRD FR-005, FR-006, FR-007, FR-011; US-01 Given/When). A signed-in,
onboarded climber taps **Main Session** on the hub and immediately sees the first recommended
problem — at the catalog's **minimum grade for their board + angle** — with its name, grade,
hold layout, and per-hold type tags, inside the ~10 s guardrail.

This slice deliberately stops **before** the adaptive loop: no RPE submission, no next-problem
pick, no end-session action — all S-04. It is the first slice that exercises the catalog
end-to-end, and it sets the `sessions` / `session_problems` schema and the `internal/recommender`
package contract that S-04 builds directly on.

## Current State Analysis

- **Auth / profile / routing shipped.** `internal/server` = 3-tier chi router
  (public → auth-only → auth+`OnboardingGate`). `auth.UserIDFromContext(ctx)` → caller id;
  `profile.Store.Get(ctx, userID)` → `{MaxGrade string, Holdsetup int16, Angle int16}`.
  Handler house style (`onboarding.go`, `profile_edit.go`): unexported `xxxPages` struct +
  `newXxxPages(...)`, `loadModel` / `handlePage` / `handleSubmit`,
  `renderPage(w, r, component, status)` (`internal/server/auth_pages.go:93`).
- **Catalog ingested** (`problems`, `problem_configurations`, `holds`, `problem_moves`).
  `internal/catalog` is a plain package of free functions over `*pgxpool.Pool` — no fx module.
  No "problem by board/grade", "minimum grade", or "problem detail" query exists yet.
- **No session / recommender code or schema anywhere.** Migrations stop at `0006_profiles`.
  `CLAUDE.md` implies package path `internal/recommender`, entry point `PickNext`.
- **Grades**: Font-scale `TEXT`; lexical `ORDER BY grade` == difficulty order (relied on by
  onboarding already). ~37 % of `problem_configurations` rows have blank `grade` — every query
  must filter `grade <> ''`.
- **Min grade genuinely varies by board+angle** — verified live: holdsetup 1 @ 40° min = `6B`,
  holdsetup 21 @ 40° min = `6B+`, most others `5+`.
- **Only boards 2016 (holdsetup 1) and 2024 (holdsetup 21) are app-ready**: all holds tagged
  (140/140, 198/198) *and* a board image exists (`static/moonboard/2016.jpg`, `2024.jpg` —
  WebP despite the extension). Masters 2017/2019 are ingested but 0/198 tagged and have no
  image — yet onboarding currently offers all 4 boards.
- **`problem_moves.move_type`** (confirmed by sampling benchmark problems):
  `s` = start hold, `e` = end/finish hold, `l` / `r` = left / right hand, `p` = generic hand
  (legacy rows), `m` = match, `f` = foot, `o` = rare/unknown.
- **Board images**: columns `A`–`K` labelled across the top, rows `1`–`18` down the left
  (row 18 top, row 1 bottom), holds on a uniform grid. Both images share the same geometry.
- **HTMX grain**: server always re-renders the whole page; success → `HX-Redirect` header +
  200; validation error → 422 full page; no `HX-Request` branching. Generated `*_templ.go` is
  committed (Railway builds with a bare `go build`).
- **No DB-backed tests today**; gating proven via a DB-free `newTestRouter` in
  `internal/server/server_test.go`. Migrations are applied by `cmd/migrate`, embedded via
  `migrations/embed.go`.

### Decisions locked with the user

| # | Decision | Choice |
|---|---|---|
| 1 | Session persistence | Create `sessions` **and** `session_problems` now; first pick recorded as `seq 0` pinned to a `problem_configuration_id`. S-04 adds result columns via a later migration. |
| 2 | Recommender packaging | New `internal/recommender` package (`New` + `FirstPick` + fx `Module`); S-04 adds `PickNext` there. |
| 3 | First-pick strategy | Random among min-grade candidates, **quality-filtered** (`is_benchmark` OR `repeats >= threshold`); fall back to any min-grade problem if the filtered set is empty. |
| 4 | Hold layout | Overlay circle markers on the board image `static/moonboard/<year>.jpg`, positioned by a calibrated `(col,row) → (x%,y%)` map; markers coloured by role (start / finish / hand / foot). Plus a hold-by-hold list (grid ref → primary type + modifiers). |
| 5 | Untagged boards | **Restrict** the board dropdown in onboarding + profile-edit to app-ready boards (image present + fully tagged). A pre-existing profile on an unsupported board gets a "switch your board" page at session start. |
| 6 | Existing active session | Enforce **one active session per user** (partial unique index); tapping Main Session with one open resumes it and its recorded first problem. |
| 7 | Hub | Real `templ` `HubPage`: greeting, current board / angle / max grade, big "Main Session" button, link to Profile. Deletes the `indexHTML` stub. |
| 8 | Testing | Stand up the `testcontainers-go` Postgres harness **now** + real integration tests for the session store and the first-pick query, alongside recommender unit tests and router gating/ownership tests. |

> Decision 8 overlaps `context/foundation/test-plan.md` §3 Phase 1
> (`testing-db-harness-and-recommender`, currently "change opened" with no folder on disk).
> This slice delivers that harness. After S-03 lands, re-run `/10x-test-plan --status` so the
> orchestrator reconciles Phase 1.

## Desired End State

An onboarded climber on an app-ready board opens `/` and sees a hub showing their board / angle /
max grade and a **Main Session** button. Tapping it `POST`s to `/session`, which (if they have no
open session) creates a `sessions` row, asks `recommender.FirstPick` for a problem at the board+
angle minimum grade, records it as `session_problems` `seq 0`, and `HX-Redirect`s to
`/session/{id}`. That page shows the problem's name, grade, the board image with the used holds
circled and role-coloured, and a list of those holds with their hold-type tags. Refreshing the
page, or tapping Main Session again, returns the same session and the same first problem. A user
requesting another user's `/session/{id}` gets a 404. A profile on an unsupported board is sent
to a page telling them to change board in their profile.

Verify with: `go build ./...`, `templ generate` (no diff), `go vet ./...`, `golangci-lint run`,
`go test ./...` (unit + new integration, Docker required), plus the manual flow in Phase 6.

### Key Discoveries

- Handler/store/templ/routing has a near-line-for-line precedent in `internal/server/profile_edit.go`,
  `internal/profile/{store,module}.go`, `templates/pages/profile.templ`.
- `internal/catalog/gridref.go:13` `ParseGridRef` already turns `"C5"` → `("C", 5)` — reuse for
  overlay positioning.
- `internal/server/server_test.go:119` `newTestRouter` is the DB-free gating harness to extend.
- `cmd/migrate/main.go` `golang-migrate` + `iofs` + `migrations.FS` setup is the thing to factor
  into `internal/testdb`.
- `idx_problem_configurations_holdsetup_angle_grade` on `(holdsetup, angle, grade)` covers the
  min-grade candidate query.

## What We're NOT Doing

- No RPE / completion submission, no next-problem pick, no `recommender.PickNext`, no end-session
  action, no session status beyond `active` — **all S-04**.
- No result columns on `session_problems` yet (S-04's migration).
- No past-sessions list or detail view — **S-05**.
- No hold-type *balance* logic (FR-012) — S-04.
- No CI wiring for the new integration tests (`.github/workflows/` still absent; owned by a
  later module). They run locally against Docker for now.
- No change to `auth.Middleware` / `OnboardingGate` behaviour.
- No move-type semantic model beyond mapping the 7 observed codes to a render role.
- No re-tagging or imaging of Masters 2017/2019.

## Implementation Approach

Six phases, back to front: schema + store + test harness first (nothing user-facing, highest
schema risk), then the recommender and catalog queries (pure logic, densely tested), then the
small board-restriction change, then the hub, then the session routes + the image-overlay view
that ties it together, then calibration + end-to-end manual verification.

New packages mirror `internal/profile` exactly: `Store{ pool }`, `NewStore(pool)`,
`module.go` with `fx.Module("name", fx.Provide(NewStore))`, domain `Err*` sentinels, inline SQL
with `fmt.Errorf("...: %w", err)`.

## Critical Implementation Details

- **Board-image coordinate calibration.** The overlay is only correct if the `(col,row) → (%)`
  map matches the image. Model it as `x% = originX + col*pitchX`, `y% = originY + (18-row)*pitchY`
  with per-board constants. Starting estimates from the images: `originX ≈ 14`, `pitchX ≈ 7.7`,
  `originY ≈ 8.5`, `pitchY ≈ 4.97` (both boards look identical — verify each). These MUST be
  eyeballed against the rendered result in Phase 6 and adjusted; treat the estimates as
  placeholders, not facts.
- **One-active-session race.** `handleStart` reads `ActiveForUser` then inserts. Two concurrent
  taps race; the partial unique index `WHERE status = 'active'` makes the loser get a
  `23505` unique violation — catch it (`errors.As` on `*pgconn.PgError`, `.Code == "23505"`),
  re-read the active session, and redirect to it rather than erroring.
- **Ownership leak.** `GET /session/{id}` for a session owned by someone else returns **404**,
  not 403 — do not confirm the id exists (test-plan Risk #2).
- **`gen_random_uuid()`** is core Postgres ≥ 13 (Supabase is 15+); no extension needed. Use it
  as the `sessions.id` / `session_problems.id` default.
- **`session_problems` pins `problem_configuration_id`**, not just `problem_id` — a problem has
  one config per angle/grade and the session is at a fixed angle; the view and S-04 both need the
  exact variant that was shown.
- **`max_grade` is snapshotted onto `sessions`** at creation so a mid-session profile edit (S-02)
  can't move S-04's ceiling.

---

## Phase 1: Schema, session store, DB test harness

### Overview

Land the `sessions` + `session_problems` tables, the `internal/session` store, and the
`internal/testdb` integration harness. Nothing user-facing; highest schema risk goes first.

### Changes Required

#### 1. Migrations

**File**: `migrations/0007_sessions.up.sql` / `.down.sql`

**Intent**: the session container.

**Contract**: `sessions( id UUID PK DEFAULT gen_random_uuid(), user_id UUID NOT NULL
REFERENCES auth.users(id) ON DELETE CASCADE, holdsetup SMALLINT NOT NULL REFERENCES
board_editions(holdsetup), angle SMALLINT NOT NULL, max_grade TEXT NOT NULL,
status TEXT NOT NULL DEFAULT 'active', started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
ended_at TIMESTAMPTZ )`. Partial unique index
`CREATE UNIQUE INDEX sessions_one_active_per_user ON sessions(user_id) WHERE status = 'active';`
and `CREATE INDEX sessions_user_started ON sessions(user_id, started_at DESC);`. Down drops the
table. No `CHECK` on `status` (matches the repo's Go-side-validation convention; S-04 extends
the value set).

**File**: `migrations/0008_session_problems.up.sql` / `.down.sql`

**Intent**: the ordered list of problems a session has shown; `seq 0` is the first pick.

**Contract**: `session_problems( id UUID PK DEFAULT gen_random_uuid(), session_id UUID NOT NULL
REFERENCES sessions(id) ON DELETE CASCADE, seq INTEGER NOT NULL, problem_id BIGINT NOT NULL
REFERENCES problems(id), problem_configuration_id BIGINT NOT NULL REFERENCES
problem_configurations(id), recommended_at TIMESTAMPTZ NOT NULL DEFAULT now(),
UNIQUE (session_id, seq) )` + index on `(session_id, seq)`.

#### 2. Session domain + store

**File**: `internal/session/session.go`

**Intent**: domain types + sentinels.

**Contract**: `type Session struct { ID, UserID string; Holdsetup, Angle int16; MaxGrade,
Status string; StartedAt time.Time }`; `type SessionProblem struct { Seq int; ProblemID,
ConfigurationID int64 }`; `const StatusActive = "active"`;
`var ErrNoActiveSession = errors.New("session: no active session")`;
`var ErrNotFound = errors.New("session: not found")`;
`var ErrActiveExists = errors.New("session: an active session already exists")`.

**File**: `internal/session/store.go` — mirror `internal/profile/store.go`

**Intent**: DB-backed read/write for `sessions` + `session_problems`.

**Contract**:
- `NewStore(pool *pgxpool.Pool) *Store`
- `ActiveForUser(ctx, userID string) (*Session, error)` — `WHERE user_id=$1 AND status='active'`;
  `ErrNoActiveSession` on no rows.
- `Get(ctx, sessionID string) (*Session, error)` — by id; `ErrNotFound` on no rows. **Not**
  user-scoped — ownership is the handler's job.
- `StartSession(ctx, s Session, first SessionProblem) (*Session, error)` — one transaction:
  insert the `sessions` row (`RETURNING id, started_at`), then insert `session_problems`
  `seq 0`. A `23505` on `sessions_one_active_per_user` → return `ErrActiveExists`.
- `FirstProblem(ctx, sessionID string) (*SessionProblem, error)` — the `seq = 0` row.

**File**: `internal/session/module.go`

**Contract**: `var Module = fx.Module("session", fx.Provide(NewStore))`.

**File**: `cmd/server/main.go`

**Contract**: add `session.Module` to the `fx.New(...)` list (router wiring is Phase 5).

#### 3. Integration test harness

**File**: `internal/testdb/testdb.go`

**Intent**: an ephemeral Postgres with the real schema for integration tests.

**Contract**: `func New(t testing.TB) *pgxpool.Pool` — starts `postgres:17-alpine` via the
`testcontainers-go` postgres module
(`postgres.Run(ctx, "postgres:17-alpine", postgres.WithWaitStrategy(
wait.ForLog("database system is ready to accept connections").WithOccurrence(2)))`), takes
`ConnectionString(ctx, "sslmode=disable")`, creates a minimal `auth.users` shim
(`CREATE SCHEMA auth; CREATE TABLE auth.users(id uuid primary key)`) so the `profiles` /
`sessions` FKs resolve, applies `migrations.FS` (reuse the `golang-migrate` + `iofs` setup from
`cmd/migrate/main.go` — factor it into a shared helper both call), returns a `pgxpool.Pool`,
and registers `t.Cleanup` to close the pool + terminate the container.

**Contract (deps)**: `go.mod` adds `github.com/testcontainers/testcontainers-go` and
`.../modules/postgres`. `github.com/stretchr/testify` optional (decide during implementation;
std asserts acceptable).

**File**: `internal/session/store_test.go` — integration

**Contract**: seed one `auth.users` row (board_editions is migration-seeded); cover
`StartSession` creates a session + `seq 0` row; `ActiveForUser` returns it; a second
`StartSession` for the same user surfaces `ErrActiveExists`; `Get` for another user's session id
still returns the row (proving `Get` is not scoped).

### Success Criteria

#### Automated Verification:

- `go run ./cmd/migrate up` applies `0007` + `0008` cleanly; `down` twice rolls them back
- `go build ./...` passes
- `go vet ./...` passes
- `golangci-lint run` passes
- `go test ./internal/session/... ./internal/testdb/...` passes (Docker running)

#### Manual Verification:

- `psql "$DATABASE_URL" -c '\d sessions'` shows the `sessions_one_active_per_user` partial
  unique index and the `sessions_user_started` index
- `psql "$DATABASE_URL" -c '\d session_problems'` shows the `UNIQUE (session_id, seq)` constraint

**Implementation Note**: After completing this phase and all automated verification passes, pause
here for manual confirmation from the human before proceeding.

---

## Phase 2: Recommender + catalog queries

### Overview

Add the `internal/recommender` package (first-pick policy) and the catalog queries it and the
session view need. Pure selection logic is table-tested; the DB queries get integration tests.

### Changes Required

#### 1. Catalog queries

**File**: `internal/catalog/candidates.go`

**Intent**: the pool of problems the first pick chooses from.

**Contract**: `type ProblemCandidate struct { ProblemID, ConfigurationID int64; Grade string;
IsBenchmark bool; Repeats int }`.
`func MinGradeCandidates(ctx, pool *pgxpool.Pool, holdsetup, angle int16) ([]ProblemCandidate,
error)` — one query: the minimum `grade` for `(holdsetup, angle)` where `grade <> ''`
(subquery / `MIN`), then every `problem_configurations` row at that grade with its `problem_id`,
`id`, `is_benchmark`, `repeats`. Uses `idx_problem_configurations_holdsetup_angle_grade`.

**File**: `internal/catalog/problem_view.go`

**Intent**: everything the session page needs to render one problem.

**Contract**: `type ProblemView struct { ProblemID int64; Name, Grade, BoardYear string;
Angle int16; Holds []HoldPlacement }`;
`type HoldPlacement struct { Seq int; MoveType, GridRef, PrimaryType string; Modifiers []string;
Col int; Row int; Role string }`.
`func ProblemDetail(ctx, pool *pgxpool.Pool, configurationID int64) (*ProblemView, error)` —
join `problem_configurations → problems → board_editions` for the header, then
`problem_moves → holds` (LEFT JOIN on `(holdsetup, grid_ref)`) ordered by `seq`. `Col`/`Row`
from `ParseGridRef` (`internal/catalog/gridref.go:13`); `Role` from `MoveType` via a small map
(`s`→`start`, `e`→`finish`, `f`→`foot`, else `hand`); `BoardYear` from `catalog.BoardYears`.

**File**: `internal/catalog/support.go`

**Intent**: the app-ready board allow-list (image present + fully tagged).

**Contract**: `func SupportedBoards() []BoardEdition` — subset of `BoardEditions` keyed to the
shipped image filenames (`{1: "2016", 21: "2024"}`), not a runtime `holds` count.
`func BoardImageYear(holdsetup int16) (string, bool)`. `func BoardName(holdsetup int16) (string,
bool)` if the hub needs it and `BoardEditions` lookup is awkward.

#### 2. Recommender package

**File**: `internal/recommender/recommender.go`

**Intent**: own the *policy* of the first pick (FR-011 + quality filter + fallback).

**Contract**:
- `type Candidate struct { ProblemID, ConfigurationID int64; Grade string; IsBenchmark bool;
  Repeats int }` (recommender's own type; handler maps `catalog.ProblemCandidate` → this).
- `type Pick struct { ProblemID, ConfigurationID int64; Grade string }`.
- `var ErrNoCandidates = errors.New("recommender: no candidates")`.
- `const minQualityRepeats = 5` (tune during review).
- pure `func pickFrom(cands []Candidate, roll func(n int) int) (Pick, error)` — filter to
  `IsBenchmark || Repeats >= minQualityRepeats`; if empty, use all `cands`; if still empty,
  `ErrNoCandidates`; else `cands[roll(len(cands))]`.
- `type Recommender struct { pool *pgxpool.Pool; rng *rand.Rand }`; `func New(pool *pgxpool.Pool)
  *Recommender`.
- `func (r *Recommender) FirstPick(ctx, holdsetup, angle int16) (Pick, error)` —
  `catalog.MinGradeCandidates` → map to `[]Candidate` → `pickFrom(_, r.rng.Intn)`.

**File**: `internal/recommender/module.go`

**Contract**: `var Module = fx.Module("recommender", fx.Provide(New))`.

**File**: `cmd/server/main.go`

**Contract**: add `recommender.Module`.

#### 3. Tests

**File**: `internal/recommender/recommender_test.go`

**Contract**: table-driven `pickFrom` (all-benchmark; none-qualify → fallback to all; empty →
`ErrNoCandidates`; deterministic pick with a stubbed `roll`). Integration (`testdb.New`): seed
`problems` + `problem_configurations` at 2–3 grades for one `(holdsetup, angle)`; assert
`FirstPick` returns a config at the lowest seeded grade.

**File**: `internal/catalog/*_test.go`

**Contract**: unit-test the `MoveType→Role` map and `SupportedBoards` / `BoardImageYear`;
integration-test `MinGradeCandidates` + `ProblemDetail` against seeded rows.

### Success Criteria

#### Automated Verification:

- `go build ./...` passes
- `go vet ./...` passes
- `golangci-lint run` passes
- `go test ./internal/recommender/... -run TestPickFrom` covers all four table cases
- `go test ./internal/recommender/... ./internal/catalog/...` passes (Docker running)

#### Manual Verification:

- `psql "$DATABASE_URL" -c "SELECT DISTINCT grade FROM problem_configurations WHERE holdsetup=1
  AND angle=40 AND grade<>'' ORDER BY grade LIMIT 1"` returns `6B` (sanity-check the min-grade
  logic against a hand query)
- Same for `holdsetup=21 AND angle=25` → `5+`

**Implementation Note**: pause for human confirmation.

---

## Phase 3: Restrict board dropdown to app-ready boards

### Overview

Point onboarding + profile-edit at `catalog.SupportedBoards()` so a user can only pick a board
the app can actually run a session on.

### Changes Required

#### 1. Onboarding + profile-edit model loaders

**File**: `internal/server/onboarding.go` (`loadModel`)

**Intent**: offer only app-ready boards.

**Contract**: build `model.Boards` from `catalog.SupportedBoards()` instead of
`catalog.BoardEditions()`. `Grades` / `Angles` unchanged. `boardValid` already validates
submissions against `model.Boards`, so an out-of-list `holdsetup` is rejected via the existing
422 path — no new validation code.

**File**: `internal/server/profile_edit.go` (`loadModel`)

**Contract**: same swap. A stored `Holdsetup` not in the supported list simply isn't
pre-selected — the user must choose a supported board to save.

#### 2. Tests

**File**: `internal/catalog/support_test.go` (or extend `catalog_validation_test.go`)

**Contract**: `SupportedBoards()` returns exactly the 2016 + 2024 editions; `BoardImageYear`
returns `("2016", true)` / `("2024", true)` and `("", false)` for 15 / 17.

### Success Criteria

#### Automated Verification:

- `go build ./...` passes
- `go vet ./...` passes
- `golangci-lint run` passes
- `go test ./internal/server/... ./internal/catalog/...` passes

#### Manual Verification:

- `GET /onboarding` shows only **2016** and **2024** in the Board dropdown
- `GET /profile` shows only **2016** and **2024** in the Board dropdown
- `curl -X POST .../profile` (authed) with `holdsetup=15` returns 422 and the form re-renders
  with "Invalid board"

**Implementation Note**: pause for human confirmation.

---

## Phase 4: Real hub page

### Overview

Replace the `indexHTML` stub with a `templ` hub showing the user's training context and the
Main Session button. Introduce the app's first stylesheet.

### Changes Required

#### 1. Stylesheet + layout

**File**: `static/app.css` (new)

**Intent**: the app's first stylesheet; hub layout now, board-overlay rules in Phase 5. Small,
unframeworked.

**File**: `templates/layout/layout.templ`

**Contract**: add `<link rel="stylesheet" href="/static/app.css"/>` to `<head>`. Run
`templ generate`; commit `layout_templ.go`.

#### 2. Hub model + page

**File**: `templates/pages/model.go`

**Contract**: `type HubModel struct { BoardName string; Angle int16; MaxGrade string }`.

**File**: `templates/pages/hub.templ` (new)

**Contract**: `templ hubContent(m HubModel)` — greeting, a line
`{m.BoardName} · {m.Angle}° · max {m.MaxGrade}`, `<button hx-post="/session" hx-swap="none">Main
Session</button>`, `<a href="/profile">Profile</a>`. `templ HubPage(m HubModel)` wraps via
`layout.Page("MoonPhase", hubContent(m))`. Run `templ generate`.

#### 3. Handler + routing

**File**: `internal/server/hub.go` (new)

**Contract**: `type hubPages struct { store *profile.Store; logger *zerolog.Logger }`,
`newHubPages(store, logger)`. `handlePage` — `auth.UserIDFromContext` → `store.Get` →
board name for `Holdsetup` (via `catalog.BoardName` or a `BoardEditions` lookup) →
`renderPage(w, r, pages.HubPage(model), http.StatusOK)`.

**File**: `internal/server/server.go`

**Contract**: instantiate `hp := newHubPages(profileStore, logger)`; replace the inline `/`
closure (`server.go:64-67`) with `r.Get("/", hp.handlePage)`; delete the `indexHTML` const
(`server.go:22-26`).

### Success Criteria

#### Automated Verification:

- `templ generate` produces no uncommitted diff after commit
- `go build ./...` passes
- `go vet ./...` passes
- `golangci-lint run` passes
- `go test ./internal/server/...` passes

#### Manual Verification:

- Signed-in onboarded user loads `/` and sees the hub with their real board / angle / max grade
  and a Main Session button; the Profile link works
- Logged-out `/` still redirects to `/signin`
- Signed-in but un-onboarded `/` still redirects to `/onboarding`

**Implementation Note**: pause for human confirmation.

---

## Phase 5: Session routes, problem view, board overlay

### Overview

Wire `POST /session` (start/resume) and `GET /session/{id}` (view), the problem-card templ with
the board-image overlay, and the ownership check. This is where the slice becomes usable.

### Changes Required

#### 1. Geometry helper

**File**: `internal/catalog/geometry.go`

**Contract**: `type boardGeometry struct { originX, pitchX, originY, pitchY float64 }`;
`var geometryByYear = map[string]boardGeometry{ "2016": {...}, "2024": {...} }` (placeholder
constants per Critical Implementation Details); `func HoldXY(year string, col, row int) (x, y
float64)` → `originX + float64(col)*pitchX`, `originY + float64(18-row)*pitchY`.

#### 2. Session view templ

**File**: `templates/pages/model.go`

**Contract**: `type SessionModel struct { SessionID string; Problem catalog.ProblemView }`;
`type UnsupportedBoardModel struct { BoardName string }`.

**File**: `templates/pages/session.templ` (new)

**Contract**:
- `templ moonBoardOverlay(year string, holds []catalog.HoldPlacement)` — `<div class="board">`,
  `<img class="board__img" src={ "/static/moonboard/" + year + ".jpg" } alt="MoonBoard "+year>`,
  then per hold a `<span class="hold hold--{role}" style={ templ.SafeCSS(fmt.Sprintf(
  "left:%.2f%%;top:%.2f%%", x, y)) }>` where `x,y = catalog.HoldXY(year, h.Col, h.Row)`.
- `templ problemCard(m SessionModel)` — name, grade, angle, board; `@moonBoardOverlay(...)`;
  a `<ul>` of holds (`{GridRef} — {PrimaryType} ({Modifiers})`, role noted).
- `templ SessionPage(m SessionModel)` wraps via `layout.Page`.
- `templ UnsupportedBoardPage(m UnsupportedBoardModel)` — message + `<a href="/profile">`.

**File**: `static/app.css`

**Contract**: `.board{position:relative;max-width:600px;margin:0 auto}`
`.board__img{width:100%;display:block}`
`.hold{position:absolute;width:7%;aspect-ratio:1;border:3px solid;border-radius:50%;
transform:translate(-50%,-50%);box-sizing:border-box}` — role modifier classes set
`border-color` (start=green, finish=purple, hand=blue, foot=grey).

#### 3. Session handlers + routing

**File**: `internal/server/session.go` (new)

**Contract**:
- `type sessionPages struct { pool *pgxpool.Pool; profiles *profile.Store; sessions
  *session.Store; rec *recommender.Recommender; logger *zerolog.Logger }`, `newSessionPages(...)`.
- `handleStart` (POST `/session`): `auth.UserIDFromContext`; `profiles.Get`;
  `catalog.BoardImageYear(profile.Holdsetup)` — if not ok, `renderPage(w, r,
  pages.UnsupportedBoardPage(...), http.StatusOK)` and return; `sessions.ActiveForUser` — if
  found, `w.Header().Set("HX-Redirect", "/session/"+s.ID)` + 200; else
  `rec.FirstPick(ctx, profile.Holdsetup, profile.Angle)` →
  `sessions.StartSession(ctx, session.Session{UserID, Holdsetup, Angle, MaxGrade}, first)` —
  on `ErrActiveExists` re-read `ActiveForUser` and redirect to it; on success
  `HX-Redirect: /session/{id}` + 200. `ErrNoCandidates` → log + 500.
- `handleView` (GET `/session/{sessionID}`): `auth.UserIDFromContext`;
  `chi.URLParam(r, "sessionID")`; `sessions.Get` — `ErrNotFound` **or** `s.UserID != userID` →
  `http.NotFound(w, r)` (identical 404 for both); `sessions.FirstProblem` →
  `catalog.ProblemDetail(ctx, pool, sp.ConfigurationID)` →
  `renderPage(w, r, pages.SessionPage(SessionModel{SessionID: id, Problem: *view}), 200)`.

**File**: `internal/server/server.go`

**Contract**: `NewRouter` gains `sessionStore *session.Store`, `rec *recommender.Recommender`
params (fx injects). Instantiate `sp := newSessionPages(pool, profileStore, sessionStore, rec,
logger)`. Inside the tier-3 `OnboardingGate` group add `r.Post("/session", sp.handleStart)` and
`r.Get("/session/{sessionID}", sp.handleView)`.

#### 4. Tests

**File**: `internal/server/server_test.go`

**Contract**: extend `newTestRouter` with stand-in `POST /session` + `GET /session/{sessionID}`
inside the `OnboardingGate` group; add `"/session/x"` to the path tables in
`TestRouter_ProtectedRoutesRedirectWithoutSession`,
`TestRouter_SessionWithoutProfileRedirectsToOnboarding`,
`TestRouter_SessionWithProfileReachesProtectedRoutes`.

**File**: `internal/server/session_handler_test.go` (new, integration)

**Contract**: real `session.Store` via `testdb.New` + the real `handleView`; two seeded users;
assert user B `GET /session/{A's id}` → 404 and user A → 200.

### Success Criteria

#### Automated Verification:

- `templ generate` produces no uncommitted diff
- `go build ./...` passes
- `go vet ./...` passes
- `golangci-lint run` passes
- `go test ./...` passes (Docker running), including the new `/session` gating rows and the
  ownership 404 test

#### Manual Verification:

- Tapping **Main Session** on the hub redirects to `/session/{uuid}` and shows a problem name +
  grade
- Tapping **Main Session** again returns the same `/session/{uuid}`
- `GET /session/{a-random-uuid}` returns 404
- (visual overlay accuracy deferred to Phase 6)

**Implementation Note**: pause for human confirmation.

---

## Phase 6: Calibration + end-to-end verification

### Overview

Calibrate the overlay coordinates against the real board images and run the full manual flow.

### Changes Required

#### 1. Geometry calibration

**File**: `internal/catalog/geometry.go`

**Intent**: make the circles land on the holds.

**Contract**: finalise `geometryByYear` for 2016 and 2024 by comparing rendered circle centres
against the hold photos in each image; adjust `originX / pitchX / originY / pitchY` until markers
sit on the holds for a spread of grid refs (`A1`, `A18`, `K1`, `K18` + a few mid-board).

### Success Criteria

#### Automated Verification:

- `go test ./...` passes
- `golangci-lint run` passes
- `go vet ./...` passes
- `templ generate` produces no uncommitted diff

#### Manual Verification:

- Fresh onboarded user on board **2024**: hub → Main Session → problem page renders within
  ~10 s; the circled holds visually match a known benchmark problem (cross-check its grid refs
  via `psql`); start holds green, finish purple
- Repeat on board **2016**
- Refresh the session page → same problem; return to `/` → Main Session → same `/session/{id}`
- A second account cannot open the first account's `/session/{id}` (404)
- A profile switched (in DB) to `holdsetup=15` → Main Session → "switch your board" page

**Implementation Note**: pause for human confirmation; then hand off to `/10x-archive` prep and
re-run `/10x-test-plan --status`.

---

## Testing Strategy

### Unit Tests:

- `recommender.pickFrom` — table-driven (quality filter, empty → fallback, empty → error,
  deterministic pick with stubbed roll)
- `catalog` `MoveType→Role` map; `SupportedBoards` / `BoardImageYear`; `HoldXY` geometry math

### Integration Tests (`internal/testdb` + Docker):

- `session.Store` — `StartSession` + `seq 0`, `ActiveForUser`, one-active unique violation →
  `ErrActiveExists`, `Get` not scoped
- `recommender.FirstPick` picks a config at the min seeded grade
- `catalog.MinGradeCandidates` / `ProblemDetail` against seeded rows
- `sessionPages.handleView` ownership → 404 for a non-owner

### Router gating (DB-free):

- `/session` + `/session/{id}` added to the redirect / onboarding / reach tables in
  `server_test.go`

### Manual Testing Steps:

See each phase's Manual Verification; the Phase 6 checklist is the end-to-end pass (overlay
accuracy, resume, ownership, unsupported board, ~10 s guardrail).

## Performance Considerations

- First pick: one indexed `MIN(grade)` + one indexed candidate scan on
  `(holdsetup, angle, grade)` + one insert transaction — well inside the ~10 s guardrail.
- Problem view: one header join + one `problem_moves → holds` join (≤ ~12 rows) — trivial.
- No new N+1; the overlay renders from a single `[]HoldPlacement` already in hand.

## Migration Notes

- `0007` + `0008` are additive; no backfill. `cmd/migrate up` at deploy applies them.
- Rollback: `cmd/migrate down` twice (drops `session_problems` then `sessions`).
- Any pre-existing profile on holdsetup 15/17 keeps working everywhere except: it can't start a
  session (gets the switch-board page) and `/profile` won't pre-select its board until changed.

## References

- Roadmap: `context/foundation/roadmap.md` S-03 (lines 117-127)
- PRD: FR-005, FR-006, FR-007, FR-011; US-01; `prd.md` §Business Logic (first-pick rule)
- Test plan: `context/foundation/test-plan.md` §2 Risks #1/#2/#4/#7, §3 Phase 1 (harness overlap)
- Patterns to copy: `internal/profile/{store,module}.go`, `internal/server/profile_edit.go`,
  `templates/pages/profile.templ`, `internal/server/server_test.go:119` (`newTestRouter`),
  `cmd/migrate/main.go` (migrate + iofs setup to factor into `internal/testdb`)
- Lessons: `context/foundation/lessons.md` — concrete copy-pasteable manual steps; `cmd/*`
  binaries auto-load `.env`

## Progress

> Convention: `- [ ]` pending, `- [x]` done. Append ` — <commit sha>` when a step lands. Do not
> rename step titles.

### Phase 1: Schema, session store, DB test harness

#### Automated

- [x] 1.1 `go run ./cmd/migrate up` applies `0007` + `0008` cleanly; `down` twice rolls back — 4818a6d
- [x] 1.2 `go build ./...` passes — 4818a6d
- [x] 1.3 `go vet ./...` passes — 4818a6d
- [x] 1.4 `golangci-lint run` passes — 4818a6d
- [x] 1.5 `go test ./internal/session/... ./internal/testdb/...` passes (Docker running) — 4818a6d

#### Manual

- [x] 1.6 `\d sessions` shows the partial unique index + `sessions_user_started` index — 4818a6d
- [x] 1.7 `\d session_problems` shows `UNIQUE (session_id, seq)` — 4818a6d

### Phase 2: Recommender + catalog queries

#### Automated

- [x] 2.1 `go build ./...` passes — c4d0fda
- [x] 2.2 `go vet ./...` passes — c4d0fda
- [x] 2.3 `golangci-lint run` passes — c4d0fda
- [x] 2.4 `go test ./internal/recommender/... -run TestPickFrom` covers all four table cases — c4d0fda
- [x] 2.5 `go test ./internal/recommender/... ./internal/catalog/...` passes (Docker running) — c4d0fda

#### Manual

- [x] 2.6 Hand `psql` min-grade query for `(1, 40)` returns `6B` — c4d0fda
- [x] 2.7 Hand `psql` min-grade query for `(21, 25)` returns `5+` — c4d0fda

### Phase 3: Restrict board dropdown to app-ready boards

#### Automated

- [x] 3.1 `go build ./...` passes — b2c865d
- [x] 3.2 `go vet ./...` passes — b2c865d
- [x] 3.3 `golangci-lint run` passes — b2c865d
- [x] 3.4 `go test ./internal/server/... ./internal/catalog/...` passes — b2c865d

#### Manual

- [x] 3.5 `GET /onboarding` Board dropdown shows only 2016 and 2024 — b2c865d
- [x] 3.6 `GET /profile` Board dropdown shows only 2016 and 2024 — b2c865d
- [x] 3.7 POST `/profile` with `holdsetup=15` returns 422 + "Invalid board" — b2c865d

### Phase 4: Real hub page

#### Automated

- [x] 4.1 `templ generate` produces no uncommitted diff after commit — 374eaf5
- [x] 4.2 `go build ./...` passes — 374eaf5
- [x] 4.3 `go vet ./...` passes — 374eaf5
- [x] 4.4 `golangci-lint run` passes — 374eaf5
- [x] 4.5 `go test ./internal/server/...` passes — 374eaf5

#### Manual

- [x] 4.6 `/` shows the hub with real board / angle / max grade + Main Session button; Profile — 374eaf5
  link works
- [x] 4.7 Logged-out `/` redirects to `/signin`; un-onboarded `/` redirects to `/onboarding` — 374eaf5

### Phase 5: Session routes, problem view, board overlay

#### Automated

- [x] 5.1 `templ generate` produces no uncommitted diff — 7df0e70
- [x] 5.2 `go build ./...` passes — 7df0e70
- [x] 5.3 `go vet ./...` passes — 7df0e70
- [x] 5.4 `golangci-lint run` passes — 7df0e70
- [x] 5.5 `go test ./...` passes (Docker running), incl. `/session` gating rows + ownership 404 — 7df0e70

#### Manual

- [x] 5.6 Main Session → `/session/{uuid}` showing a problem name + grade — 7df0e70
- [x] 5.7 Main Session again → same `/session/{uuid}` — 7df0e70
- [x] 5.8 `GET /session/{random-uuid}` returns 404 — 7df0e70

### Phase 6: Calibration + end-to-end verification

#### Automated

- [x] 6.1 `go test ./...` passes — 6feb793
- [x] 6.2 `golangci-lint run` passes — 6feb793
- [x] 6.3 `go vet ./...` passes — 6feb793
- [x] 6.4 `templ generate` produces no uncommitted diff — 6feb793

#### Manual

- [x] 6.5 Board 2024: hub → Main Session → problem renders < ~10 s; circled holds match a — 6feb793
  benchmark problem's grid refs; start green, finish purple
- [x] 6.6 Board 2016: same — 6feb793
- [x] 6.7 Refresh session page → same problem; `/` → Main Session → same `/session/{id}` — 6feb793
- [x] 6.8 Second account gets 404 on the first account's `/session/{id}` — 6feb793
- [x] 6.9 Profile on `holdsetup=15` → Main Session → "switch your board" page — 6feb793

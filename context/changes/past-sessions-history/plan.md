# Past Sessions History Implementation Plan

## Overview

Roadmap slice **S-05** (PRD FR-013, FR-014). Give a signed-in, onboarded climber a read-only
history surface: a list of their finished Main Sessions ordered most-recent-first, a
per-session detail view showing every climbed problem with its RPE and completion status, and
a read-only problem card to review any climbed problem's MoonBoard layout.

This is the last slice of Stream B and, per the roadmap, "a read-only surface with no
independent risk of its own." It consumes data S-04 already produces and adds no schema.

## Current State Analysis

- **Schema is complete for this slice.** `migrations/0007_sessions.up.sql` gives `sessions`
  with `status` (`'active'` / `'ended'`), `started_at`, `ended_at`, and a snapshot of
  `holdsetup` / `angle` / `max_grade` taken at session start. `0008` + `0009` give
  `session_problems` with `seq`, `problem_id`, `problem_configuration_id`, and nullable
  `rpe` / `completion` / `climbed_at`. There is already an index
  `sessions_user_started ON sessions (user_id, started_at DESC)` — exactly the list's access
  path. **No migration is needed.**
- **`internal/session/store.go`** has `Get(ctx, sessionID)` (not user-scoped — the handler
  compares `UserID`, see `internal/server/session.go:120`), `ActiveForUser`, and
  `ShownProblems` (joins `problem_configurations.grade` + `problem_hold_types.dominant`, but
  not the problem `name`). It has no list query and no rated-only query.
- **Handler pattern** (`internal/server/session.go`): a `sessionPages` struct holding
  `pool`, `profiles`, `sessions`, `rec`, `logger`; routes mounted in `NewRouter`
  (`internal/server/server.go:66-80`) inside the `auth.Middleware` + `OnboardingGate` group.
  Non-owner / missing / wrong-status all collapse to an identical `http.NotFound` — the id's
  existence is never confirmed to a non-owner (`session.go:120`, `session.go:157`).
- **`renderPage(w, r, component, status)`** (`internal/server/auth_pages.go:93`) is the
  shared templ render helper. Full pages wrap content in `layout.Page(title, content)`
  (`templates/layout/layout.templ`).
- **templ** components live in `templates/pages/*.templ` with plain-struct models in
  `templates/pages/model.go`; `.templ` is compiled to `*_templ.go` by `templ generate`.
  `session.templ` already factors the board render into reusable components:
  `moonBoardOverlay(year, holds)`, `holdMarker`, and the `holdLabel(HoldPlacement) string`
  helper. `SessionCard` composes them.
- **`catalog.ProblemDetail(ctx, pool, configurationID) (*catalog.ProblemView, error)`**
  (`internal/catalog/problem_view.go:51`) returns everything the board render needs (name,
  grade, board year, angle, ordered holds with tags); returns
  `catalog.ErrConfigurationNotFound` on a missing id.
- **`catalog.BoardName(holdsetup int16) (string, bool)`** (`internal/catalog/support.go:41`)
  maps the snapshot to a display name.
- **Hub** (`templates/pages/hub.templ`) currently links only to `/profile`.
- **Test harness:** `internal/testdb.New(t) *pgxpool.Pool` (testcontainers `postgres:17-alpine`,
  all migrations applied, `auth.users` shim). `internal/server/session_handler_test.go` has
  reusable seeders: `seedUserRow`, `seedFirstProblem`, `seedCandidate`. DB tests **fail**
  (not skip) without Docker.
- **E2E:** TypeScript Playwright in `tests/e2e/`; `seed.spec.ts` is the exemplar
  (role/label/text locators, wait-for-state, `afterEach` deletes the throwaway user via FK
  cascade). `adaptive-session-loop.spec.ts` already drives sign up → onboard → session →
  rate → end.

### Structural quirk to handle

After each rated problem the adaptive loop **inserts the next recommended problem**
(`AdvanceSession`, `store.go:180`), and `EndSession` only flips `status`. So **every ended
session carries a trailing `session_problems` row with `rpe IS NULL`** — the last problem
recommended but never climbed. A session ended immediately after start has exactly one such
row (seq 0, unrated). The history surface must treat "climbed" as `rpe IS NOT NULL`
everywhere: the list count, the detail list, and the problem-card seq guard.

## Desired End State

A climber taps **Past sessions** on the hub and lands on `GET /sessions`: a list of their
`status = 'ended'` sessions, newest first, each row showing the started date, the board +
angle, and "N problems climbed". Tapping a row opens `GET /sessions/{sessionID}`: the same
header plus a compact list of every climbed problem — name, grade, RPE, completion status, in
climb order. Tapping a problem opens `GET /sessions/{sessionID}/problem/{seq}`: the read-only
MoonBoard layout (image + hold overlay + tag list, reusing the live-session render) with a
"you climbed this — RPE 6 · sent" line and a back-link to the session.

A climber with no finished sessions sees "No sessions yet — start a Main Session". Opening a
session that was ended before any problem was rated shows "No problems climbed in this
session". Every route returns an identical 404 for a non-owner, a missing id, a non-ended
session, or an unrated / out-of-range `seq`.

### Verification

- `go build ./...`, `go vet ./...`, `golangci-lint run`, `go test ./...` all pass.
- New integration tests for the three store queries pass against the testcontainers harness.
- New handler tests cover the 404 discipline and the happy paths.
- `tests/e2e/past-sessions-history.spec.ts` passes: full path from sign up to problem card.
- Manual: run a real session, end it, confirm it appears in the list with the right count,
  open it, open a problem.

## What We're NOT Doing

- **No schema change** — the tables and index already exist.
- **No pagination / "load more"** — every ended session is returned, `started_at DESC`.
- **No editing or deleting sessions** (PRD Non-Goal) — strictly read-only.
- **No end-of-session recap / summary stats** — no duration, no volume, no style-coverage
  chart (PRD Non-Goals: no analytics, no trend charts).
- **No "why this pick" copy** (PRD Non-Goal).
- **The active session is not shown in the list** and has no history detail — it stays
  reachable only through the hub's Main Session button, which resumes it.
- **No shared nav bar** — one hub link plus back-links, matching the existing hub/profile
  navigation style.
- **No catalog-wide problem route** — the problem card is session-scoped and reuses the
  session ownership check.
- **No changes to `auth.Middleware`, `OnboardingGate`, the recommender, or the live session
  loop.**

## Implementation Approach

Three read-only `GET` routes under a new `/sessions` (plural) namespace, kept separate from
the live loop's `/session` (singular) routes so the two surfaces never share semantics. All
three mount in the existing `auth.Middleware` + `OnboardingGate` group. A new `historyPages`
handler struct mirrors `sessionPages`. Three new `session.Store` methods supply the data; the
problem card additionally calls the existing `catalog.ProblemDetail`. A new
`templates/pages/history.templ` holds the list page, the detail page, and the problem card,
reusing `session.templ`'s `moonBoardOverlay` / `holdLabel`. One `<a>` is added to
`hub.templ`. Ownership and status checks live in the handler (SQL is filtered by
`user_id` + `status = 'ended'` where it can be, and the handler re-checks `Get`'s
`UserID` / `Status` for the detail and problem routes), returning an identical
`http.NotFound` for every failure mode.

## Critical Implementation Details

- **404 discipline.** The detail and problem handlers first call `sessions.Get(ctx, id)`
  and treat `err != nil || sess.UserID != userID || sess.Status != session.StatusEnded` as a
  single `http.NotFound` — identical response for "doesn't exist", "not yours", and "still
  active". This matches `session.go:120`. The list handler needs no id check; its query is
  `WHERE user_id = $1 AND status = 'ended'`.
- **"Climbed" filter.** `ListForUser`'s count is `COUNT(sp.rpe)` (SQL `COUNT(expr)` skips
  NULLs) over a `LEFT JOIN`. `ClimbedProblems` and `ClimbedProblemAt` filter
  `sp.rpe IS NOT NULL`. The trailing unrated row must never appear, and the seq in the
  problem URL must be rejected if unrated.
- **`seq` parsing.** `chi.URLParam(r, "seq")` → `strconv.Atoi`; a parse failure is a 404
  (not 400) to stay consistent with the "never confirm the shape of a valid id" stance.

## Phase 1: Store queries

### Overview

Add the three read queries to `internal/session` with integration tests. No handler or
template work yet.

### Changes Required:

#### 1. History value types

**File**: `internal/session/session.go`

**Intent**: Add the row structs the new queries return, alongside the existing `Session` /
`SessionProblem` / `ShownProblem` types.

**Contract**:
- `SessionSummary{ ID string; Holdsetup, Angle int16; StartedAt time.Time; ClimbedCount int }`
  — one row of the history list.
- `ClimbedProblem{ Seq int; Name, Grade string; RPE int16; Completion string; ClimbedAt time.Time }`
  — one climbed problem in the detail list.
- `ClimbedProblemRef{ ConfigurationID int64; RPE int16; Completion string }` — what the
  problem-card handler needs before calling `catalog.ProblemDetail`.

#### 2. `ListForUser`

**File**: `internal/session/store.go`

**Intent**: Return the caller's ended sessions, newest first, each with a count of climbed
(rated) problems.

**Contract**: `func (s *Store) ListForUser(ctx context.Context, userID string) ([]SessionSummary, error)`.
Query joins `session_problems` with a `LEFT JOIN` and `COUNT(sp.rpe)`, groups by
`sessions.id`, filters `s.user_id = $1 AND s.status = 'ended'`, orders `s.started_at DESC`.
Returns an empty slice (not an error) when the user has no ended sessions.

```sql
SELECT s.id, s.holdsetup, s.angle, s.started_at, COUNT(sp.rpe) AS climbed
FROM sessions s
LEFT JOIN session_problems sp ON sp.session_id = s.id
WHERE s.user_id = $1 AND s.status = 'ended'
GROUP BY s.id
ORDER BY s.started_at DESC
```

#### 3. `ClimbedProblems`

**File**: `internal/session/store.go`

**Intent**: Return every rated problem of one session in climb order, with the catalog name
and grade for display.

**Contract**: `func (s *Store) ClimbedProblems(ctx context.Context, sessionID string) ([]ClimbedProblem, error)`.
Joins `session_problems` → `problem_configurations` → `problems`, filters
`sp.session_id = $1 AND sp.rpe IS NOT NULL`, orders `sp.seq`. Not user-scoped — the handler
has already verified ownership via `Get`. Returns an empty slice for a session with no rated
problems.

#### 4. `ClimbedProblemAt`

**File**: `internal/session/store.go`

**Intent**: Resolve one `(sessionID, seq)` to the problem configuration id plus the recorded
RPE / completion, for the read-only problem card.

**Contract**: `func (s *Store) ClimbedProblemAt(ctx context.Context, sessionID string, seq int) (*ClimbedProblemRef, error)`.
`SELECT problem_configuration_id, rpe, completion FROM session_problems WHERE session_id = $1
AND seq = $2 AND rpe IS NOT NULL`. Returns `ErrNotFound` (reuse the existing sentinel) on
`pgx.ErrNoRows` — covers a missing seq, an out-of-range seq, and the trailing unrated row.

### Success Criteria:

#### Automated Verification:

- `go build ./...` passes
- `go vet ./...` passes
- `golangci-lint run` passes
- `go test ./internal/session/...` passes, including new cases:
  - `ListForUser` returns ended sessions newest-first, excludes the caller's active session,
    excludes other users' sessions, and reports `ClimbedCount` matching the rated rows
  - `ClimbedProblems` returns rated rows in seq order and omits the trailing unrated row
  - `ClimbedProblemAt` returns the ref for a rated seq and `ErrNotFound` for an unrated seq,
    an absent seq, and a wrong session id

#### Manual Verification:

- None for this phase — pure query layer, fully covered by integration tests.

**Implementation Note**: After completing this phase and all automated verification passes,
pause here for manual confirmation before proceeding (no manual steps here, so this is a
checkpoint only).

---

## Phase 2: Handlers + routes

### Overview

Add `internal/server/history.go` with the three handlers and wire the routes into the
onboarding-gated group. Handler tests cover the 404 discipline and happy paths. Templates do
not exist yet — this phase renders against them, so it lands together with Phase 3 for a
green build; split the commits but keep the phase boundary at "routes reachable, templates
stubbed or built".

> Practical note: because Go won't compile a handler that references an undefined
> `pages.HistoryListPage`, Phases 2 and 3 are implemented in one working session. The phase
> split is for review and progress tracking, not for a compilable checkpoint between them.

### Changes Required:

#### 1. `historyPages` handler struct

**File**: `internal/server/history.go` (new)

**Intent**: Mirror `sessionPages`. Holds `pool *pgxpool.Pool`, `sessions *session.Store`,
`logger *zerolog.Logger`. Constructor `newHistoryPages(pool, sessions, logger)`.

**Contract**: Three methods:
- `handleList(w, r)` — `GET /sessions`. Resolve `userID` from context; call
  `sessions.ListForUser`; map each `SessionSummary` to a `pages.HistorySessionRow`
  (`catalog.BoardName` for the display name, format `StartedAt`); render
  `pages.HistoryListPage`. Empty slice → the template shows the empty state.
- `handleDetail(w, r)` — `GET /sessions/{sessionID}`. `sessions.Get`; if
  `err != nil || sess.UserID != userID || sess.Status != session.StatusEnded` →
  `http.NotFound`. Call `sessions.ClimbedProblems`; render `pages.HistoryDetailPage` with
  the session header + rows. Empty slice → the detail empty state.
- `handleProblem(w, r)` — `GET /sessions/{sessionID}/problem/{seq}`. Same ownership+status
  guard via `Get`. Parse `seq`; a parse error → `http.NotFound`. Call
  `sessions.ClimbedProblemAt`; `ErrNotFound` → `http.NotFound`. Call
  `catalog.ProblemDetail(ctx, s.pool, ref.ConfigurationID)`; render
  `pages.HistoryProblemPage` with the `ProblemView`, the recorded RPE/completion, and the
  `sessionID` for the back-link.

All error paths log with `s.logger.Error()` and return `http.StatusInternalServerError`,
matching `sessionPages`.

#### 2. Route wiring

**File**: `internal/server/server.go`

**Intent**: Construct `historyPages` next to the other page structs and mount the three
routes inside the existing `r.Group` that already has `auth.Middleware` + `OnboardingGate`
(the block at lines 66-80).

**Contract**: Add `hsp := newHistoryPages(pool, sessionStore, logger)` beside `sp := ...`.
Inside the gated group add:
```go
r.Get("/sessions", hsp.handleList)
r.Get("/sessions/{sessionID}", hsp.handleDetail)
r.Get("/sessions/{sessionID}/problem/{seq}", hsp.handleProblem)
```
`NewRouter`'s signature already receives `sessionStore` and `pool`; no new parameters.

### Success Criteria:

#### Automated Verification:

- `go build ./...` passes (with Phase 3 templates present)
- `go vet ./...` passes
- `golangci-lint run` passes
- `go test ./internal/server/...` passes, including new `history_handler_test.go` cases:
  - `GET /sessions` returns only the caller's ended sessions (two seeded users, seeded
    active + ended sessions)
  - `GET /sessions/{id}` returns 404 for a non-owner, for an active session, and for a
    random UUID
  - `GET /sessions/{id}` for an owned ended session renders the climbed problems
  - `GET /sessions/{id}/problem/{seq}` returns 404 for a non-owner, an unrated seq, an
    out-of-range seq, and a non-numeric seq
  - `GET /sessions/{id}/problem/{seq}` for a rated seq renders the problem and shows the
    recorded RPE + completion

#### Manual Verification:

- None for this phase — handler behavior is fully covered by integration tests; visual
  verification happens in Phase 3.

**Implementation Note**: After completing this phase and all automated verification passes,
pause here for manual confirmation before proceeding to the next phase.

---

## Phase 3: Templates + hub link

### Overview

Add `templates/pages/history.templ` (list page, detail page, problem card, both empty
states) and the models in `model.go`, add the hub link, run `templ generate`.

### Changes Required:

#### 1. History view models

**File**: `templates/pages/model.go`

**Intent**: Add the plain structs the three templates bind to.

**Contract**:
- `HistoryListModel{ Sessions []HistorySessionRow }`
- `HistorySessionRow{ ID, BoardName, StartedAt string; Angle int16; ClimbedCount int }`
  (`StartedAt` pre-formatted in the handler — keep formatting out of the template)
- `HistoryDetailModel{ SessionID, BoardName, StartedAt string; Angle int16; Problems []HistoryProblemRow }`
- `HistoryProblemRow{ Seq int; Name, Grade, Completion string; RPE int16 }`
- `HistoryProblemModel{ SessionID string; Seq int; RPE int16; Completion string; Problem catalog.ProblemView }`

#### 2. History templates

**File**: `templates/pages/history.templ` (new)

**Intent**: Three `templ` page components plus their content sub-templates, styled with the
existing class vocabulary (`main`, `hub`-style headers) — no new CSS framework.

**Contract**:
- `HistoryListPage(m HistoryListModel)` → `layout.Page("Past sessions", …)`. Renders a list
  of rows; each row is an `<a href={ "/sessions/" + row.ID }>` showing
  `{ row.StartedAt } · { row.BoardName } · { Angle }° · { ClimbedCount } problems climbed`.
  When `len(m.Sessions) == 0`, render the empty state: "No sessions yet" + a link to `/`
  ("Start a Main Session").
- `HistoryDetailPage(m HistoryDetailModel)` → `layout.Page("Session", …)`. Header line with
  date + board + angle; a `<a href="/sessions">` back-link; then an ordered list of
  `HistoryProblemRow` as `<a href={ "/sessions/" + SessionID + "/problem/" + seq }>` lines
  showing `{ Name } · { Grade } · RPE { RPE } · { Completion }`. When `len(m.Problems) == 0`,
  render "No problems climbed in this session" + the back-link.
- `HistoryProblemPage(m HistoryProblemModel)` → `layout.Page("Problem", …)`. Reuses
  `moonBoardOverlay(m.Problem.BoardYear, m.Problem.Holds)` and the `holdLabel` list markup
  from `session.templ`; adds a line "You climbed this — RPE { RPE } · { Completion }" and a
  `<a href={ "/sessions/" + m.SessionID }>` back-link. **No result form, no End button** —
  this is the live card stripped to its read-only parts.

> `moonBoardOverlay` and `holdLabel` are unexported in package `pages` and `history.templ`
> is in the same package, so they are directly callable — no refactor needed.

#### 3. Hub link

**File**: `templates/pages/hub.templ`

**Intent**: Add a "Past sessions" link beside the existing Profile link.

**Contract**: Add `<p><a href="/sessions">Past sessions</a></p>` after the Profile link line.

#### 4. Regenerate templ Go

**File**: `templates/pages/*_templ.go`

**Intent**: `templ generate` compiles `history.templ` and the `hub.templ` edit.

**Contract**: `templ generate` run from repo root; the generated `history_templ.go` and
updated `hub_templ.go` are committed alongside the `.templ` sources.

### Success Criteria:

#### Automated Verification:

- `templ generate` produces no diff on a second run (generated files are current)
- `go build ./...` passes
- `go vet ./...` passes
- `golangci-lint run` passes
- `go test ./...` passes

#### Manual Verification:

- Start the app (`go run ./cmd/server`, self-loads `.env`), sign in, run one Main Session:
  rate two problems (e.g. Sent/RPE 4, then Failed/RPE 8), tap **End session**.
- On the hub, tap **Past sessions** → the session appears as the top row, showing today's
  date, the board + angle, and "2 problems climbed".
- Open the row → the detail view lists both climbed problems in order with the RPE and
  status just submitted; the trailing recommended problem is **not** listed.
- Tap the first problem → the MoonBoard image renders with the hold overlay and tag list,
  the "You climbed this — RPE 4 · sent" line shows, and the back-link returns to the detail
  view.
- Sign up a fresh account, onboard, open **Past sessions** → "No sessions yet" empty state.
- Start a session and immediately tap **End session**, then open it from **Past sessions** →
  "No problems climbed in this session" empty state.
- Confirm no full-page flash is expected here (these are plain navigations, not HTMX swaps)
  and the pages are readable one-handed on a phone-width viewport.

**Implementation Note**: After completing this phase and all automated verification passes,
pause here for manual confirmation from the human that the manual testing was successful
before proceeding to the next phase.

---

## Phase 4: E2E

### Overview

One Playwright spec covering the full history path, modeled on `seed.spec.ts` and
`adaptive-session-loop.spec.ts`.

### Changes Required:

#### 1. History E2E spec

**File**: `tests/e2e/past-sessions-history.spec.ts` (new)

**Intent**: Drive the browser path that proves the history surface works end to end and is
scoped to the caller.

**Contract**: One `test` named for the risk ("a climber reviews a finished session's climbed
problems from history"):
- Sign up + onboard a unique throwaway user (timestamp + random suffix), following the
  existing specs' helper pattern; `afterEach` deletes the user (FK cascade clears
  sessions/problems).
- Start a Main Session, submit two results via the status→RPE form, tap **End session**
  (reuse the interaction already written in `adaptive-session-loop.spec.ts`).
- `getByRole('link', { name: /past sessions/i })` from the hub → assert the list shows a row
  with "2 problems climbed".
- Open the row → assert both climbed problem names/grades are visible with their RPE and
  status (`getByText`).
- Open the first problem → assert the board image (`getByRole('img', { name: /moonboard/i })`)
  and the "You climbed this" text are visible; assert the back-link returns to the detail
  URL (`waitForURL`).
- Locators: `getByRole` / `getByText` only; waits are `toBeVisible()` / `waitForURL()`;
  no `waitForTimeout`.

### Success Criteria:

#### Automated Verification:

- `npm run test:e2e -- past-sessions-history` passes (Docker + reachable Postgres + Supabase
  Auth per `tests/e2e/README.md`)
- `npm run test:e2e` (full suite) passes — no regression in the existing specs
- `go test ./...` still passes

#### Manual Verification:

- Run `npm run test:e2e -- past-sessions-history` once locally and confirm the throwaway
  user is gone afterward (`afterEach` teardown ran).

**Implementation Note**: After completing this phase and all automated verification passes,
pause for manual confirmation. Then update `context/changes/past-sessions-history/change.md`
`status: implemented` and re-run `/10x-test-plan --status` (this slice adds a net-new
browser path for the history surface).

---

## Testing Strategy

### Unit / integration Tests:

- `internal/session`: `ListForUser` (ordering, ended-only, per-user scoping, climbed count),
  `ClimbedProblems` (rated-only, seq order), `ClimbedProblemAt` (found / unrated / missing).
- `internal/server`: `history_handler_test.go` — a route × caller-state table mirroring the
  `session_handler_test.go` style: owner vs non-owner vs anonymous-context, active vs ended,
  rated vs unrated vs out-of-range seq, plus the two happy paths.

### E2E Tests:

- `tests/e2e/past-sessions-history.spec.ts` — sign up → onboard → session (rate 2, end) →
  Past sessions list → detail → problem card, with the caller-scoping assertion built in
  (the fresh user only ever sees their own one session).

### Manual Testing Steps:

1. Run a real session with two rated problems of different RPE, end it, and verify the list
   row count and the detail RPE/status values match what was submitted.
2. Verify the trailing recommended-but-unclimbed problem is absent from the detail list.
3. Verify both empty states (new account; session ended before rating).
4. Verify the read-only problem card renders the board overlay and omits the result form /
   End button.
5. Verify all back-links and the hub link on a phone-width viewport.

## Performance Considerations

The list query is a single grouped scan over `sessions` (filtered by the
`sessions_user_started` index) with a `LEFT JOIN` to `session_problems` (indexed by
`idx_session_problems_session_seq`); at the product's stated scale (single/low-double-digit
users, a few sessions per week, ≤~15 problems each) this is trivially fast and needs no
pagination. `ClimbedProblems` and `ClimbedProblemAt` are keyed by `session_id` on the same
index. `catalog.ProblemDetail` is the one heavier query (holds join) and is issued only on
the problem-card route, one problem at a time — the same cost the live session view already
pays per problem.

## Migration Notes

None. No schema change.

## References

- Roadmap slice: `context/foundation/roadmap.md` → S-05
- Prior slice (data producer): `context/changes/adaptive-main-session-loop/plan.md`
- Session store + handler patterns: `internal/session/store.go`,
  `internal/server/session.go`
- Board render components to reuse: `templates/pages/session.templ`
  (`moonBoardOverlay`, `holdLabel`)
- Problem detail query: `internal/catalog/problem_view.go:51`
- Test harness + seeders: `internal/testdb/testdb.go`,
  `internal/server/session_handler_test.go`
- E2E exemplar: `tests/e2e/seed.spec.ts`, `tests/e2e/adaptive-session-loop.spec.ts`
- Lessons: `context/foundation/lessons.md` (concrete copy-pasteable manual steps)

## Progress

> Convention: `- [ ]` pending, `- [x]` done. Append ` — <commit sha>` when a step lands. Do not rename step titles. See `references/progress-format.md`.

### Phase 1: Store queries

#### Automated

- [x] 1.1 `go build ./...` passes — 2dd4733
- [x] 1.2 `go vet ./...` passes — 2dd4733
- [x] 1.3 `golangci-lint run` passes — 2dd4733
- [x] 1.4 `go test ./internal/session/...` passes with the new `ListForUser` / `ClimbedProblems` / `ClimbedProblemAt` cases — 2dd4733

### Phase 2: Handlers + routes

#### Automated

- [x] 2.1 `go build ./...` passes — a62479c
- [x] 2.2 `go vet ./...` passes — a62479c
- [x] 2.3 `golangci-lint run` passes — a62479c
- [x] 2.4 `go test ./internal/server/...` passes with the new `history_handler_test.go` 404-discipline + happy-path cases — a62479c

### Phase 3: Templates + hub link

#### Automated

- [x] 3.1 `templ generate` produces no diff on a second run — 21f5cab
- [x] 3.2 `go build ./...` passes — 21f5cab
- [x] 3.3 `go vet ./...` passes — 21f5cab
- [x] 3.4 `golangci-lint run` passes — 21f5cab
- [x] 3.5 `go test ./...` passes — 21f5cab

#### Manual

- [x] 3.6 Real session (rate 2, end) → Past sessions row shows date + board/angle + "2 problems climbed" — a0a6e59 (e2e, Pixel 7 viewport)
- [x] 3.7 Detail view lists both climbed problems in order with submitted RPE/status; trailing unrated problem absent — a0a6e59 (e2e) + a62479c (handler test asserts exactly 2 /problem/ links)
- [x] 3.8 Problem card renders board overlay + tag list + "You climbed this" line + working back-link; no result form / End button — a0a6e59 (e2e asserts img, back-link nav, End-button count 0)
- [x] 3.9 "No sessions yet" empty state on a fresh account — a0a6e59 (e2e second test)
- [x] 3.10 "No problems climbed in this session" empty state for a session ended before rating — a62479c (TestHistoryDetail_RendersClimbedProblems)
- [x] 3.11 Pages readable one-handed on a phone-width viewport — a0a6e59 (full e2e path passes under devices['Pixel 7'] portrait)

### Phase 4: E2E

#### Automated

- [x] 4.1 `npm run test:e2e -- past-sessions-history` passes — a0a6e59
- [x] 4.2 `npm run test:e2e` full suite passes (no regression) — a0a6e59
- [x] 4.3 `go test ./...` still passes — a0a6e59

#### Manual

- [x] 4.4 Throwaway user is deleted after the spec run (`afterEach` teardown verified) — a0a6e59

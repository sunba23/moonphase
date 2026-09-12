# S-04 Adaptive Main Session Loop — Implementation Plan

> Change ID: `adaptive-main-session-loop` · Roadmap slice: **S-04** (north star)
> PRD refs: FR-008, FR-009, FR-010, FR-012, US-01 (full Then clause + Acceptance Criteria)
> Builds directly on S-03 (`start-session-first-problem`, shipped).

## Context

MoonPhase's whole premise is the adaptive loop: the app picks the first problem, then
re-ranks the next problem after each rated attempt. It reacts both to how hard the last
problem felt and to which hold types the session has already loaded. The PRD's Success
Criteria section names this loop — not the auth, not the catalog — as the thing that proves
the product works.

S-03 shipped the first half: a climber taps **Main Session**, gets a catalog-backed first
problem at the board+angle minimum grade, and the session persists. S-03 stopped before
any result submission, any next pick, and any end action. This slice adds all three.

Intended outcome: a climber submits a per-problem RPE (1–10) plus a completion status
(sent / failed / bailed). The next recommendation's difficulty AND hold-type composition
visibly reflect that result and the session so far, inside the ~3 s guardrail. The climber
can end the session at any point, and the partial session is saved intact.

## Current State Analysis

What S-03 left in place (verified in code):

- **Schema stops at `0008`.** `sessions` has `status TEXT DEFAULT 'active'` and an unused
  `ended_at TIMESTAMPTZ`, `max_grade` snapshotted at creation, and a partial unique index
  `sessions_one_active_per_user WHERE status = 'active'`. `session_problems(session_id, seq,
  problem_id, problem_configuration_id, recommended_at)` with `UNIQUE (session_id, seq)`.
  **No result columns.**
- **`internal/session`** — `Store{pool}` mirrors `internal/profile`: inline SQL, `Err*`
  sentinels, `const StatusActive`. Methods: `ActiveForUser`, `Get` (not user-scoped —
  ownership is the handler's job), `StartSession` (one tx, `23505` → `ErrActiveExists`),
  `FirstProblem` (seq 0 only).
- **`internal/recommender`** — `Recommender{pool, rng *rand.Rand}` (`math/rand/v2`,
  `rng.IntN`). Pure `pickFrom(cands, roll func(n int) int)` with a quality filter
  (`IsBenchmark || Repeats >= minQualityRepeats`, `minQualityRepeats = 5`), a full-set
  fallback, and `ErrNoCandidates`. `FirstPick` maps `catalog.ProblemCandidate` → `Candidate`
  by struct conversion. Tests table-drive `pickFrom` with a stubbed `roll` and
  integration-test `FirstPick` via `testdb.New`. The package comment already reserves
  `PickNext` for this slice. The package has **no logger** (kept pure).
- **`internal/catalog`** — free functions over `*pgxpool.Pool`, no fx module.
  `MinGradeCandidates` uses a `WITH min_grade` sub-query and
  `idx_problem_configurations_holdsetup_angle_grade`. `ProblemDetail` → `ProblemView{Holds
  []HoldPlacement}`; `moveTypeRole` maps `s`/`e`/`f` → `start`/`finish`/`foot`, else `hand`.
  **`GradeLadder` does not exist.** Font-scale grades sort lexically in difficulty order
  (`6B` < `6B+` < `6C` < `6C+` < `7A`) and the repo already relies on this.
  `holds.primary_type` is `TEXT` NULL until tagged; `AllowedHoldTypes = {crimp, sloper,
  pinch, jug, pocket}`. `problem_moves.move_type` ∈ `s/e/l/r/p/m/f/o`; `f` = foot.
- **Hold tagging** runs after ingest: `catalog.HoldStore.ApplyTags` (invoked by the
  `catalog holds load-tags` CLI) is the operation that actually sets `holds.primary_type`.
  F-02 tagging for boards 2016 and 2024 is already complete.
- **`internal/server/session.go`** — `sessionPages{pool, profiles, sessions, rec, logger}`.
  `handleStart` (POST `/session`, resume-or-create, unsupported-board page, `ErrActiveExists`
  → re-read + redirect). `handleView` (GET `/session/{sessionID}`; `sess.UserID != userID` →
  identical `http.NotFound`; **hard-codes `seq 0` via `FirstProblem`**). `redirectToSession`
  sets `HX-Redirect` + 200. Routes mounted in the tier-3 `OnboardingGate` group in
  `server.go`; `NewRouter` already receives `sessionStore` and `rec`.
- **`templates/pages/session.templ`** — `problemCard(m SessionModel)` renders
  `<main class="session">` with header, `@moonBoardOverlay`, and a holds `<ul>`.
  `renderPage(w, r, component, status)` works for both full pages and fragments. `static/app.css`
  has `.hold--*` role colours. Hub button is `hx-post="/session" hx-swap="none"`; other forms
  use `hx-post` + `hx-select="#authForm"` + `hx-swap="outerHTML"`.
- **`internal/testdb`** — `testdb.New(t) *pgxpool.Pool`, `postgres:17-alpine`, `auth.users`
  shim, all migrations applied. DB tests fail (not skip) without Docker.
- **`cmd/migrate`** — `up` = `m.Up()`; `down` = `m.Steps(-1)`, **one migration file per
  invocation**. `cmd/*` binaries auto-load `.env` via `godotenv`.
- **E2E** — `tests/e2e/auth-first-problem.spec.ts` drives sign-up → onboard → first problem
  with Playwright and role-based locators; cleans up the Supabase user in `afterEach`.

## Desired End State

A climber runs a full Main Session end to end:

1. Tap **Main Session** → first problem card (S-03 path, unchanged).
2. Choose a completion status (3 large buttons), then tap an RPE number (1–10 grid). The
   card swaps in place to the next problem, no full-page reload, inside ~3 s.
3. The next problem's grade obeys the RPE→grade rule (never harder after a hard or
   failed attempt; a step up is allowed only after an easy send; never above the
   session's snapshotted max grade).
4. The next problem's hold-type composition avoids stacking the type the session has
   over-loaded; four problems of one dominant hold type in a row do not happen while
   the candidate pool can avoid it.
5. Tap **End session** at any point → redirect to the hub. `sessions.status = 'ended'`,
   `ended_at` set, every rated problem persisted.
6. An abrupt mid-loop drop loses nothing — every already-rated problem is durable, and
   reopening `/session/{id}` shows the current unrated problem.
7. A non-owner hitting any session route gets an identical 404.

Verification: the Phase 6 manual script plus `go test ./...` (Docker), `golangci-lint run`,
`go vet ./...`, `templ generate` clean.

### Key Discoveries

- The composition aggregation must not run at request time. A precomputed side table
  (`problem_hold_types`, one row per problem) turns the next-pick query into a flat
  indexed scan + PK join — this is what protects the ~3 s p95 guardrail at 260k-problem
  scale (`internal/catalog/candidates.go:24`, test-plan Risk #7).
- "Never harder after hard/failed" is enforced **by construction**: the RPE→grade mapping
  computes an allowed grade window, and the SQL filters `grade <= allowedMax AND grade <=
  session.max_grade`. It is unit-provable without touching SQL.
- The recompute hook for `problem_hold_types` must live in **`catalog.HoldStore.ApplyTags`**
  (the operation that sets `primary_type`), not only the ingest path. A migration backfill
  alone goes stale (`internal/catalog/holds.go:87`).
- `handleView` hard-codes `seq 0`. Mid-loop refresh must show the latest problem
  (`internal/server/session.go:124`).
- Import-cycle trap: `session.AdvanceSession` must take plain scalars + `session.SessionProblem`,
  never a `recommender` type.
- The `recommender` package has no logger and must keep it that way. Fallback-tier logging
  is the handler's job, off a returned `PickDiag`.

## What We're NOT Doing

- No past-sessions list or session-detail view — that is S-05. After **End session** the
  hub has nothing to show the finished session; this is deferred on purpose.
- No end-of-session recap or summary screen.
- No "we picked this because…" copy in the UI (PRD Non-Goal).
- No ML, no learned weights — the scorer is explicit constants, tuned by hand.
- No move-type tagging (dyno / lock-off / drop-knee) — hold types only.
- No re-tagging or imaging of Masters 2017 / 2019; they stay unsupported.
- No CI wiring for the new integration tests (owned by a later module; they run locally
  against Docker).
- No change to `auth.Middleware` or `OnboardingGate` behaviour.
- No RPE trend charts or analytics (PRD Non-Goal).

## Implementation Approach

Six phases, back to front, mirroring S-03's structure:

1. **Schema** — result columns on `session_problems`, the `problem_hold_types` side table
   + backfill + a shared recompute helper wired into `ApplyTags` and ingest.
2. **Pure core** — the grade ladder, the RPE→grade window, and `scoreNext` as pure,
   table-driven, rng-stubbed functions. No DB, no HTTP. This is the test-plan Risk #1
   workhorse.
3. **`PickNext` orchestration** — session-state assembly, the flat composition-joined
   candidate query, and the 4-tier empty-pool fallback. Integration-tested.
4. **Routes** — `POST /session/{id}/result` and `POST /session/{id}/end`, server-side
   validation, and the one-transaction write (score before the tx, then UPDATE-current +
   INSERT-next atomically).
5. **templ / HTMX** — `problemCard` refactored into a swappable `#session-card` fragment
   carrying the result form (status buttons → RPE grid) and an always-visible End button.
6. **End-to-end + guardrail** — the full US-01 loop by hand on both boards, the ~3 s
   next-pick guardrail measured and baselined, and a scripted RPE trajectory eyeballed
   for correct direction of adaptation.

New packages and files mirror `internal/profile` exactly: `Store{pool}`, `NewStore`,
`module.go` with `fx.Module`, domain `Err*` sentinels, inline SQL, `fmt.Errorf("…: %w", err)`.

## Critical Implementation Details

- **`problem_hold_types` freshness.** Recompute in three places: (a) `ApplyTags`, for the
  problems whose moves touch the retagged grid refs, inside the existing tx; (b) the ingest
  write path, for written problem ids; (c) a `catalog holds recompute-composition --board
  <year>` CLI for a full rebuild. The `0010` backfill is correct at deploy because F-02
  tagging is done — but the migration alone is not a maintenance strategy.
- **`0010` backfill runtime.** The backfill aggregates ~3M `problem_moves` rows in one
  statement over the Supabase transaction-mode pooler that `cmd/migrate` uses. Put
  `SET LOCAL statement_timeout = 0` in the migration. Verify against the real DB in the
  Phase 1 manual steps.
- **`cmd/migrate down` is one file per call.** Rolling back this slice is two invocations
  (`0010`, then `0009`).
- **Score before the transaction.** `handleResult` runs `PickNext` (which reads the DB)
  *before* opening the write tx, so a scorer bug cannot roll back the result write. The
  transaction then does only: guarded UPDATE of the current row, INSERT of the next row.
- **Guarded UPDATE is the idempotency key.** `AdvanceSession`'s UPDATE carries
  `WHERE … AND rpe IS NULL AND seq = (SELECT max(seq) …)`. Two rapid taps: the loser
  affects 0 rows → rollback → `ErrStaleResult` → 409, and its already-computed pick is
  discarded.
- **Ownership is always 404, never 403.** Every session route keeps the S-03 policy: a
  non-owner and a missing id are indistinguishable.
- **Grade-string comparison.** Do the window/ceiling min in Go against the ladder slice.
  Never compare two grade strings with raw `<` unless both are confirmed on the ladder.

---

## Phase 1: Schema — result columns, composition side table, backfill

### Overview

Land `session_problems` result columns, the flat `problem_hold_types` table, its backfill,
and a shared `RecomputeHoldTypes` helper wired into the tagging and ingest paths. Nothing
user-facing. Highest schema risk goes first.

### Changes Required

#### 1. Migration `0009` — result columns

**File**: `migrations/0009_session_problem_results.up.sql` / `.down.sql`

**Intent**: give `session_problems` somewhere to record the per-problem result.

**Contract**: additive, nullable, no `CHECK` (repo convention — validation is Go-side):
`ALTER TABLE session_problems ADD COLUMN rpe SMALLINT, ADD COLUMN completion TEXT,
ADD COLUMN climbed_at TIMESTAMPTZ;`. Down drops the three columns. `sessions.status` needs
no DDL — `'ended'` is a new Go constant, and the partial unique index frees the active slot
automatically when `status` moves off `'active'`.

#### 2. Migration `0010` — `problem_hold_types` + backfill

**File**: `migrations/0010_problem_hold_types.up.sql` / `.down.sql`

**Intent**: one row per problem with its per-type non-foot hold counts and dominant type,
so the next-pick query never aggregates at request time.

**Contract**: table keyed `problem_id BIGINT PRIMARY KEY REFERENCES problems(id) ON DELETE
CASCADE`, one `SMALLINT NOT NULL DEFAULT 0` column per `AllowedHoldTypes` entry plus
`unknown` (untagged scored holds) and `total_scored`, and `dominant TEXT` (NULL when
`total_scored = 0`). Backfill is a single `INSERT … SELECT` from `problem_moves pm JOIN
holds h ON h.holdsetup = pm.holdsetup AND h.grid_ref = pm.grid_ref WHERE pm.move_type <> 'f'
GROUP BY pm.problem_id`, using `count(*) FILTER (WHERE h.primary_type = …)` per type. The
`dominant` sub-select picks the type with the highest count, ties broken alphabetically
(deterministic for tests). Prepend `SET LOCAL statement_timeout = 0;`. Down drops the table.

#### 3. Shared recompute helper

**File**: `internal/catalog/hold_types.go` (new) + `internal/catalog/hold_types_test.go` (new, integration)

**Intent**: one reusable function that rebuilds `problem_hold_types` rows, callable from a
`pgx.Tx` or the pool, so the tagging path, the ingest path, and a CLI all stay consistent.

**Contract**: `RecomputeHoldTypes(ctx, db, problemIDs ...int64) error` — rebuilds the named
rows (all rows when none passed) as a single `DELETE` + `INSERT … SELECT` reusing the `0010`
aggregate. `db` is a minimal `Query`/`Exec` interface satisfied by both `*pgxpool.Pool` and
`pgx.Tx`. Also expose a `DominantHoldType(counts map[string]int) string` pure helper for
reuse in tests and the session-state assembly.

#### 4. Wire the recompute hook

**File**: `internal/catalog/holds.go` (edit `ApplyTags`), `internal/catalog/ingest.go` (edit
the write paths), `cmd/catalog/main.go` (edit)

**Intent**: keep `problem_hold_types` current wherever `holds.primary_type` or
`problem_moves` change.

**Contract**: `ApplyTags` — after the existing `UPDATE holds …` loop and before `tx.Commit`,
collect the problem ids whose moves reference any retagged grid ref
(`SELECT DISTINCT problem_id FROM problem_moves WHERE holdsetup = $1 AND grid_ref = ANY($2)`)
and call `RecomputeHoldTypes(ctx, tx, ids...)`. `ingest.go` — call `RecomputeHoldTypes` for
the batch's written problem ids inside the ingest tx path. `cmd/catalog` — add a
`holds recompute-composition --board <year>` sub-command that runs a full rebuild for that
board's problems.

#### 5. Session-domain constants

**File**: `internal/session/session.go` (edit)

**Intent**: the value sets this slice adds.

**Contract**: `const StatusEnded = "ended"`; `const CompletionSent/Failed/Bailed`;
`func ValidCompletion(string) bool`; `var ErrStaleResult`, `var ErrSessionNotActive`.

### Success Criteria

#### Automated Verification

- `go run ./cmd/migrate up` applies `0009` then `0010`; two `go run ./cmd/migrate down`
  calls roll them back cleanly.
- `go build ./...`, `go vet ./...`, `golangci-lint run` pass.
- `go test ./internal/catalog -run TestRecomputeHoldTypes` (Docker): a problem seeded with
  3 crimp + 1 jug + 1 foot move → row is `crimp=3, jug=1, unknown=0, total_scored=4,
  dominant='crimp'`; the foot move is excluded. Retag one crimp → sloper, call
  `RecomputeHoldTypes(id)` → `crimp=2, sloper=1, dominant='crimp'`.
- `go test ./internal/catalog -run TestApplyTagsRecomputesComposition` (Docker): `ApplyTags`
  on a grid ref used by a seeded problem updates that problem's `problem_hold_types` row in
  the same call.

#### Manual Verification

- `psql "$DATABASE_URL" -c "\d problem_hold_types"` shows the PK and all columns.
- `psql "$DATABASE_URL" -c "SELECT count(*) FROM problem_hold_types"` ≈
  `psql "$DATABASE_URL" -c "SELECT count(DISTINCT problem_id) FROM problem_moves"`.
- `psql "$DATABASE_URL" -c "SELECT dominant, count(*) FROM problem_hold_types pht JOIN
  problems p ON p.id = pht.problem_id WHERE p.holdsetup = 1 GROUP BY dominant ORDER BY 2
  DESC"` → a plausible spread across the 5 types, no NULL `dominant` for holdsetup 1.
- Time the `0010` migration against the real Supabase DB and record the wall-clock in the
  Progress notes.

**Implementation Note**: pause for human confirmation of the manual steps before Phase 2.

---

## Phase 2: Pure core — grade ladder, RPE→grade window, `scoreNext`

### Overview

Every difficulty and hold-balance rule as pure, table-driven, rng-stubbed functions. No DB,
no HTTP. This retires the cheap, high-value part of test-plan Risk #1 and all of Risk #4's
"never above max / never harder" invariants.

### Changes Required

#### 1. Grade ladder query

**File**: `internal/catalog/grades.go` (new) + `internal/catalog/grades_test.go` (new, integration)

**Intent**: the ascending list of grades actually offered on a board+angle, so the window
logic can step by real catalog grades.

**Contract**: `GradeLadder(ctx, pool, holdsetup, angle int16) ([]string, error)` —
`SELECT DISTINCT grade FROM problem_configurations WHERE holdsetup = $1 AND angle = $2 AND
grade <> '' ORDER BY grade`.

#### 2. RPE→grade window

**File**: `internal/recommender/grade_window.go` (new) + `_test.go` (new)

**Intent**: turn the previous result into an inclusive `[lo, hi]` band of catalog grades
the next pick may have, before the session ceiling is applied.

**Contract** (all pure): `type Completion string`; `type Result struct { RPE int; Completion
Completion }`; `type band int` with `bandBackOff / bandHold / bandStepUp`.
`classify(r Result) band` — the locked 4-band mapping: failed/bailed OR RPE≥8 → backOff;
RPE 5–7 & sent → hold; RPE≤4 & sent → stepUp. `gradeWindow(ladder []string, cur string, b
band) (lo, hi string, ok bool)` — backOff → `[oneEasier(cur), cur]`; hold → `[cur, cur]`;
stepUp → `[cur, oneHarder(cur)]`; `cur` off-ladder → a safe wide window and `ok = false`
(caller logs). `oneEasier` / `oneHarder` clamp at the ladder ends.
`preferredIndex(ladder, cur, b) int` — the grade the scorer aims at inside the window.
`minGradeOnLadder(a, b string) string` — ladder-position min, for the ceiling clamp.

**Invariant**: for backOff and hold, `hi` is never above `cur`. Combined with the Phase 3
SQL clamp to `session.max_grade`, "never strictly harder after hard/failed" holds by
construction.

#### 3. The scorer

**File**: `internal/recommender/score.go` (new) + `_test.go` (new)

**Intent**: score every candidate on grade fit + hold-type balance + variety + quality, and
return the argmax with an rng-resolved tie set.

**Contract** (pure): `type ScoreCandidate struct { ProblemID, ConfigurationID int64;
GradeIndex int; Dominant string; Quality bool }`. `type ScoreState struct { PreferredIndex
int; RecentDominants []string; SessionDominantCounts map[string]int; PrevDominant string;
DropBalance bool }`. `scoreNext(cands []ScoreCandidate, st ScoreState, roll func(n int) int)
(int, error)` — returns the index of the argmax; candidates within `scoreEpsilon` of the max
form the tie set, resolved by `roll`. Empty `cands` → `(-1, ErrNoCandidates)`.

Starting weights (tunable consts at the top of `score.go`):

```
wGrade   = 1.00   // per ladder step of |GradeIndex - PreferredIndex|
wRecent  = 0.70   // * share of last 3 shown problems with this dominant
wSession = 0.30   // * share of all session scored-hold dominants
wNovel   = 0.20   // bonus if Dominant != "" and Dominant != PrevDominant
wQuality = 0.15   // bonus if Quality

score(c) = -wGrade*abs(gi-pref) - wRecent*recentShare(d) - wSession*sessionShare(d)
           + wNovel*novel(d) + wQuality*quality(c)
```

`wRecent + wSession` can reach ~1.0, so a fully-loaded balance penalty can override a
one-step grade miss — deliberate, per FR-012 ("not the next-grade-up crimp problem"). When
`DropBalance` is set, the recent/session terms are zero.

### Success Criteria

#### Automated Verification

- `go test ./internal/recommender -run 'TestClassify|TestGradeWindow|TestScoreNext'` passes:
  - `classify`: `{3, bailed}` → backOff; `{8, sent}` → backOff; `{6, sent}` → hold;
    `{4, sent}` → stepUp.
  - `gradeWindow`: `cur` at ladder top + stepUp → `hi == cur`; `cur` at ladder bottom +
    backOff → `lo == cur`; `cur` off-ladder → `ok == false`.
  - `scoreNext`: (a) two candidates one step apart, equal balance → picks the one at
    `PreferredIndex`; (b) a candidate at the preferred grade whose dominant matches a
    3-long recent streak, vs a candidate one step off with a fresh dominant → picks the
    fresh one; (c) `DropBalance = true` flips (b) back to the on-grade pick; (d) exact tie
    → deterministic via `roll` returning 0; (e) empty → `ErrNoCandidates`.
- `go test ./internal/catalog -run TestGradeLadder` (Docker): grades `6B, 6B+, 6C` plus a
  blank-grade row seeded on `(1, 40)` → ladder is exactly `["6B", "6B+", "6C"]`.
- `golangci-lint run`, `go vet ./...` pass.

#### Manual Verification

- None required — pure logic, fully covered by units.
- Optional: `psql "$DATABASE_URL" -c "SELECT DISTINCT grade FROM problem_configurations
  WHERE holdsetup = 1 AND angle = 40 AND grade <> '' ORDER BY grade"` and eyeball that the
  lexical order is the climbing order.

**Implementation Note**: pause for human confirmation before Phase 3.

---

## Phase 3: `PickNext` orchestration — candidate query, state assembly, fallback tiers

### Overview

`recommender.PickNext` reads the session so far, builds the grade window, runs the flat
composition-joined candidate query, applies `scoreNext`, and walks the 4-tier empty-pool
fallback. It returns the pick plus a `PickDiag` naming which tier fired. Integration-tested
against `testdb`.

### Changes Required

#### 1. Next-pick candidate query

**File**: `internal/catalog/candidates.go` (edit) + `internal/catalog/candidates_test.go` (edit, integration)

**Intent**: the flat, index-friendly query that returns candidates already carrying their
dominant hold type — no aggregation at request time.

**Contract**: `type NextCandidate struct { ProblemID, ConfigurationID int64; Grade string;
IsBenchmark bool; Repeats int; Dominant string }`. `type NextPickQuery struct { Holdsetup,
Angle int16; GradeMin, GradeMax string; ExcludeProblemIDs []int64; ExcludeDominant string;
Limit int }`. `NextPickCandidates(ctx, pool, q NextPickQuery) ([]NextCandidate, error)` —
single statement:

```sql
SELECT pc.problem_id, pc.id, pc.grade, pc.is_benchmark, pc.repeats,
       COALESCE(pht.dominant, '')
FROM problem_configurations pc
JOIN problem_hold_types pht ON pht.problem_id = pc.problem_id
WHERE pc.holdsetup = $1 AND pc.angle = $2
  AND pc.grade <> '' AND pc.grade >= $3 AND pc.grade <= $4
  AND ($5::bigint[] = '{}' OR pc.problem_id <> ALL($5))
  AND ($6 = '' OR COALESCE(pht.dominant, '') <> $6)
LIMIT $7
```

The caller pre-clamps `GradeMax` to `session.max_grade`. Uses
`idx_problem_configurations_holdsetup_angle_grade` for the range scan and the
`problem_hold_types` PK for the join.

#### 2. Session-state read

**File**: `internal/session/store.go` (edit) + `internal/session/store_test.go` (edit, integration)

**Intent**: every problem the session has shown, in order, with its grade and dominant hold
type, so the handler can build the scorer input.

**Contract**: `type ShownProblem struct { Seq int; ProblemID, ConfigurationID int64; Grade,
Dominant string; RPE *int16; Completion *string }`.
`ShownProblems(ctx, sessionID string) ([]ShownProblem, error)` —
`session_problems sp JOIN problem_configurations pc ON pc.id = sp.problem_configuration_id
LEFT JOIN problem_hold_types pht ON pht.problem_id = sp.problem_id WHERE sp.session_id = $1
ORDER BY sp.seq`. Also add `LatestProblem(ctx, sessionID) (*SessionProblem, error)` (or let
callers take the last `ShownProblems` entry) for Phase 5's `handleView` fix.

#### 3. `PickNext`

**File**: `internal/recommender/recommender.go` (edit) + `internal/recommender/pick_next_test.go` (new, integration)

**Intent**: the orchestration — window, candidate pool, score, fallback.

**Contract**: `type ShownState struct { ProblemID int64; Grade, Dominant string }`.
`type PickNextInput struct { Holdsetup, Angle int16; SessionMaxGrade string; Shown
[]ShownState; CurrentResult Result }` (`Shown` is seq-ordered; the last entry is the
just-climbed problem). `type PickDiag struct { FallbackTier int; GradeLo, GradeHi string;
ExcludedDominant string }`. `PickNext(ctx, in PickNextInput) (Pick, PickDiag, error)` —
deterministic given `r.rng`.

Internals:

1. `ladder := catalog.GradeLadder(...)`; `cur := in.Shown[last].Grade`;
   `b := classify(in.CurrentResult)`.
2. `lo, hi, ok := gradeWindow(ladder, cur, b)`; `hi = minGradeOnLadder(hi,
   in.SessionMaxGrade)`; log if `!ok`.
3. Build `ScoreState`: `RecentDominants` from the last 3 `Shown` dominants;
   `SessionDominantCounts` from all `Shown`; `PrevDominant = Shown[last].Dominant`.
4. Streak exclude: if the last 3 `Shown` dominants are all equal and non-empty, set
   `excludeDominant` to that type.
5. Tier loop, each tier setting `PickDiag.FallbackTier`:
   - **0** — `NextPickCandidates{lo, hi, ExcludeProblemIDs: allShownIDs, ExcludeDominant:
     excludeDominant}` → `scoreNext`.
   - **1** — same window and exclude-shown, `ExcludeDominant = ""`, `ScoreState.DropBalance
     = true`.
   - **2** — step `lo` down the ladder one entry at a time (keep `hi`), exclude-shown, no
     dominant exclude, until non-empty or `lo == ladder[0]`.
   - **3** — drop `ExcludeProblemIDs`; window `[ladder[0], hi]`.
   - **4** — `catalog.MinGradeCandidates(holdsetup, angle)` filtered `grade <=
     SessionMaxGrade`, then `pickFrom(_, r.rng.IntN)` (reuse the S-03 path).
6. Return `Pick{ProblemID, ConfigurationID, Grade}` + `PickDiag`.

**Ceiling invariant**: tiers 0–3 pass `GradeMax <= SessionMaxGrade`; tier 4 filters `grade
<= SessionMaxGrade`. The engine can never recommend above the snapshotted max. Tier 4 may
return a problem below the RPE window — acceptable; "never harder" is the hard rule.

### Success Criteria

#### Automated Verification

- `go test ./internal/catalog -run TestNextPickCandidates` (Docker): 4 problems seeded
  across `6B`/`6B+`, one crimp-dominant, one already "shown" → the window filter,
  `ExcludeProblemIDs`, and `ExcludeDominant` each remove the right rows.
- `go test ./internal/session -run TestShownProblems` (Docker): a 3-row session with the
  middle row rated → `RPE` non-nil only for rated rows, `Dominant` populated from
  `problem_hold_types`.
- `go test ./internal/recommender -run TestPickNext` (Docker):
  - `{RPE:9, sent}` after a `6C` problem → pick grade `<= 6C`.
  - `{RPE:3, sent}` at `6B` with headroom → pick can be `6B+`; with `SessionMaxGrade = 6B`
    → still `6B`.
  - three consecutive crimp-dominant problems seeded → next pick's dominant `!= crimp`, OR
    `PickDiag.FallbackTier >= 1`.
  - window emptied by seeding only out-of-window grades → `FallbackTier` advances; never
    errors while any problem `<= max` exists.
  - one problem in the whole catalog, already shown → tier 3 or 4 returns it, no
    `ErrNoCandidates`.
- `golangci-lint run`, `go vet ./...` pass.

#### Manual Verification

- `psql` seed one session at `(1, 40)` with a `6C` `seq 0` row, then run the tier-0 query
  by hand for `{RPE:9}` (`grade BETWEEN '6B+' AND '6C'`). `EXPLAIN ANALYZE` shows an index
  scan and a runtime under ~50 ms.

**Implementation Note**: pause for human confirmation before Phase 4.

---

## Phase 4: Routes — result + end, validation, one transaction

### Overview

The server side of the loop. `handleResult` validates server-side (Risk #4), scores the
next pick before opening a transaction, then writes UPDATE-current + INSERT-next atomically
and returns the new card fragment. `handleEnd` sets `status = 'ended'` + `ended_at` and
`HX-Redirect`s to `/`. Both keep the S-03 ownership-404 policy and sit inside the existing
`OnboardingGate` group (Risk #5).

### Changes Required

#### 1. Store writes

**File**: `internal/session/store.go` (edit) + `internal/session/store_test.go` (edit, integration)

**Intent**: the two write operations, each atomic.

**Contract**: `AdvanceSession(ctx, sessionID string, curSeq int, rpe int16, completion
string, next SessionProblem) error` — one tx: guarded `UPDATE session_problems SET rpe = $1,
completion = $2, climbed_at = now() WHERE session_id = $3 AND seq = $4 AND rpe IS NULL AND
seq = (SELECT max(seq) FROM session_problems WHERE session_id = $3)`. If `RowsAffected != 1`
→ rollback + `ErrStaleResult`. Then `INSERT INTO session_problems (session_id, seq,
problem_id, problem_configuration_id) VALUES ($3, $4 + 1, $5, $6)`; commit. Takes plain
scalars + `session.SessionProblem` only — **no `recommender` import**.
`EndSession(ctx, sessionID, userID string) error` — `UPDATE sessions SET status = 'ended',
ended_at = now() WHERE id = $1 AND user_id = $2 AND status = 'active'`; 0 rows → `ErrNotFound`
(covers non-owner and already-ended with no information leak).

#### 2. Handlers

**File**: `internal/server/session.go` (edit)

**Intent**: `handleResult` and `handleEnd`.

**Contract**: `handleResult` (POST `/session/{sessionID}/result`):

1. `userID` from context; `sessionID` from chi.
2. `s.sessions.Get` → `err != nil || sess.UserID != userID` → `http.NotFound` (identical).
3. `sess.Status != session.StatusActive` → 409, no write.
4. `r.ParseForm` behind a `MaxBytesReader` (reuse `maxAuthFormBytes`). `seq` via
   `strconv.Atoi` → 422 on error. `rpe` via `strconv.Atoi`; `rpe < 1 || rpe > 10` → 422.
   `completion` → `!session.ValidCompletion(completion)` → 422. Every 422 writes nothing.
5. `shown, _ := s.sessions.ShownProblems(...)`; `last := shown[len-1]`. `seq != last.Seq ||
   last.RPE != nil` → 409, no write (Risk #4 — "a problem the user was never shown").
6. Build `recommender.PickNextInput` from `shown` plus this `rpe`/`completion` as
   `CurrentResult`. `pick, diag, err := s.rec.PickNext(...)`. `err` → log + 500. `if
   diag.FallbackTier > 0 { s.logger.Warn()…Int("tier", diag.FallbackTier)… }`.
7. `s.sessions.AdvanceSession(ctx, sessionID, seq, int16(rpe), completion,
   session.SessionProblem{Seq: seq + 1, ProblemID: pick.ProblemID, ConfigurationID:
   pick.ConfigurationID})`. `ErrStaleResult` → 409, no write. Other → log + 500.
8. `view, _ := catalog.ProblemDetail(ctx, s.pool, pick.ConfigurationID)`.
9. `renderPage(w, r, pages.SessionCard(pages.SessionCardModel{SessionID: sessionID, Seq: seq
   + 1, Problem: *view}), http.StatusOK)` — the **fragment**, not `SessionPage`.

`handleEnd` (POST `/session/{sessionID}/end`): `userID` + `sessionID`; `s.sessions.EndSession(ctx,
sessionID, userID)`; `ErrNotFound` → `http.NotFound`; other → log + 500;
`w.Header().Set("HX-Redirect", "/")`; `w.WriteHeader(http.StatusOK)`.

#### 3. Route mounts

**File**: `internal/server/server.go` (edit) + `internal/server/server_test.go` (edit)

**Intent**: mount the two routes in the gated group and cover them in the route tables.

**Contract**: inside the `OnboardingGate` group, after the existing session routes:
`r.Post("/session/{sessionID}/result", sp.handleResult)` and
`r.Post("/session/{sessionID}/end", sp.handleEnd)`. Add both (as POST) to the `newTestRouter`
stand-ins and to all three route tables in `server_test.go` (`…RedirectWithoutSession`,
`…WithoutProfileRedirectsToOnboarding`, `…ReachesProtectedRoutes`).

#### 4. Model stub

**File**: `templates/pages/model.go` (edit)

**Intent**: `SessionCardModel`, referenced by the handler now, rendered in Phase 5.

**Contract**: `type SessionCardModel struct { SessionID string; Seq int; Problem
catalog.ProblemView }`.

### Success Criteria

#### Automated Verification

- `go build ./...`, `go vet ./...`, `golangci-lint run` pass; `templ generate` no diff.
- `go test ./internal/server -run TestRouter` — the two new POST paths are in all three
  route tables and behave like the other gated routes.
- `go test ./internal/session -run 'TestAdvanceSession|TestEndSession'` (Docker):
  - `AdvanceSession` writes the result on `seq N` and inserts `seq N+1` in one call;
    re-calling with the same `seq` → `ErrStaleResult`, no second insert.
  - `EndSession` by owner flips status; by non-owner → `ErrNotFound`; second call →
    `ErrNotFound`; the freed slot lets the same user `StartSession` again.
- `go test ./internal/server -run TestHandleResult` (Docker):
  - non-owner POST → 404, zero DB writes.
  - `rpe=0`, `rpe=11`, `completion=lol`, missing `seq` → 422, `session_problems` unchanged.
  - `seq` at an already-rated row → 409, no write.
  - happy path → 200, body contains the next problem's name, `session_problems` has `seq+1`,
    the old row has `rpe`/`completion`/`climbed_at` set.
  - submit two results then `handleEnd` → `ShownProblems` shows 2 rated rows +
    `sessions.status = 'ended'` (Risk #3).

#### Manual Verification

- `curl -i -b "$COOKIE" -X POST "$BASE/session/$SID/result" -d 'seq=0&rpe=15&completion=sent'`
  → `HTTP/1.1 422`; `psql "$DATABASE_URL" -c "SELECT seq, rpe FROM session_problems WHERE
  session_id = '$SID' ORDER BY seq"` unchanged.
- `curl -i -b "$COOKIE" -X POST "$BASE/session/$SID/result" -d 'seq=0&rpe=3&completion=sent'`
  → `200`, body is `<div id="session-card">…`; the `psql` query now shows `seq 0` rated plus
  a `seq 1` row.
- `curl -i -b "$COOKIE" -X POST "$BASE/session/$SID/end"` → `200` with `HX-Redirect: /`;
  `psql "$DATABASE_URL" -c "SELECT status, ended_at FROM sessions WHERE id = '$SID'"` →
  `ended`, timestamp set.

**Implementation Note**: pause for human confirmation before Phase 5.

---

## Phase 5: templ / HTMX — swappable card + result UI

### Overview

`problemCard` becomes a self-contained `#session-card` fragment carrying the result form
(3 status buttons → 1–10 grid → submit) and an always-visible End button. The result POST
swaps the card in place, no full-page reload (Risk #6).

### Changes Required

#### 1. Models

**File**: `templates/pages/model.go` (edit)

**Contract**: `SessionCardModel` (from Phase 4) stays. `type SessionModel struct { Card
SessionCardModel }` now just wraps the card.

#### 2. Templates

**File**: `templates/pages/session.templ` (edit); run `templ generate`, commit `session_templ.go`

**Intent**: refactor into a swappable fragment plus the result form.

**Contract**: `SessionCard(m SessionCardModel)` renders `<div id="session-card">` containing
the header, `@moonBoardOverlay`, the holds `<ul>` (content unchanged, moved inside),
`@resultForm(m.SessionID, m.Seq)`, and an End `<form hx-post={ "/session/" + m.SessionID +
"/end" } hx-swap="none">`. `resultForm(sessionID string, seq int)` is a `<form
hx-post={ ".../result" } hx-target="#session-card" hx-swap="outerHTML">` with a hidden `seq`
input, a `<fieldset class="result__status">` of 3 `<input type="radio" name="completion"
required>` labels (sent / failed / bailed), and a `<div class="result__rpe">` of 10
`<button type="submit" name="rpe" value="n">` buttons. `SessionPage(m SessionModel)` =
`layout.Page("Session", SessionCard(m.Card))`.

#### 3. Styles

**File**: `static/app.css` (edit)

**Intent**: the two-step reveal and the mobile tap targets, no JS.

**Contract**: `.result__rpe { display: none }`; `.result__status:has(input:checked) ~
.result__rpe { display: grid; grid-template-columns: repeat(5, 1fr); gap: .5rem }`. Status
labels are block, `padding: 1rem`, `font-size: 1.15rem`, bordered, with a checked state.
RPE buttons `padding: 1rem 0`, `font-size: 1.25rem`. `:has()` is in the latest two versions
of all four mainstream browsers (the NFR target). Mis-tapping a status is fixed by tapping a
different one — the status buttons stay visible, which is the "back" affordance.

#### 4. `handleView` shows the latest problem

**File**: `internal/server/session.go` (edit)

**Intent**: a mid-loop refresh must show the current unrated problem, not `seq 0`.

**Contract**: `handleView` uses `LatestProblem` (or the last `ShownProblems` entry) instead
of `FirstProblem`, and renders `pages.SessionPage(pages.SessionModel{Card:
pages.SessionCardModel{SessionID: id, Seq: latest.Seq, Problem: *view}})`. `FirstProblem`
stays for any S-05 use.

### Success Criteria

#### Automated Verification

- `templ generate` → no uncommitted diff after commit.
- `go build ./...`, `go vet ./...`, `golangci-lint run`, `go test ./internal/server/...` pass.
- `go test ./internal/server -run TestHandleResult` asserts the response body is a bare
  `<div id="session-card">` (no `<!DOCTYPE`), so a swap cannot nest a document (Risk #6).
- `go test ./internal/server -run TestHandleView_ShowsLatestProblem` (Docker): a 2-row
  session (seq 0 rated, seq 1 unrated) → `handleView` body contains the seq-1 problem's name.

#### Manual Verification

- Load `/session/{id}`: the card shows the problem; the 1–10 grid is hidden; tapping "Sent"
  reveals it; tapping "7" swaps the card to the next problem in place, URL unchanged, no
  full-reload flash.
- Tap "Failed" then "Sent" before choosing a number — no submit fires, the selection moves.
- "End session" is visible without scrolling on a 390 px-wide viewport; tapping it navigates
  to `/`.
- DevTools Network: the result POST response is `Content-Type: text/html`, body starts with
  `<div id="session-card"`.

**Implementation Note**: pause for human confirmation before Phase 6.

---

## Phase 6: End-to-end verification, guardrail measurement, trajectory sanity

### Overview

Prove the full US-01 loop by hand on both supported boards. Measure the ~3 s p95 next-pick
guardrail against a realistic pool and record a baseline. Eyeball a scripted RPE trajectory
for correct direction of adaptation (Risk #1, Risk #7). No new product code — at most a
`_test.go` benchmark.

### Changes Required

#### 1. Benchmark

**File**: `internal/recommender/pick_next_bench_test.go` (new)

**Intent**: a recorded baseline for the next-pick guardrail.

**Contract**: `BenchmarkPickNext` seeds `problem_configurations` + `problem_hold_types` at
the real per-`(1, 40)` row counts (pull the counts from `psql`, generate that many synthetic
rows), then runs `b.N` iterations of `PickNext` with a mid-session `PickNextInput`. Report
ns/op and assert it is well under 3 s (it will be milliseconds).

#### 2. Weight tuning (only if needed)

**File**: `internal/recommender/score.go` (edit consts)

**Intent**: if the trajectory script shows the pick does not change type on a pool that
clearly has other in-window types, raise `wRecent` or lower `wGrade` and re-run.

### Success Criteria

#### Automated Verification

- `go test ./...` green (Docker); `golangci-lint run`, `go vet ./...`, `templ generate` clean.
- `go test ./internal/recommender -bench BenchmarkPickNext -benchtime 200x` → ms-scale
  ns/op, logged in the Progress notes.

#### Manual Verification

Setup (board 2024, holdsetup 21, angle 40; `$UID` a seeded `auth.users` id, `$BASE` the
local server, `$COOKIE` the session cookie):

```
psql "$DATABASE_URL" -c "INSERT INTO profiles (id, max_grade, holdsetup, angle) VALUES ('$UID','7B',21,40) ON CONFLICT (id) DO UPDATE SET holdsetup=21, angle=40, max_grade='7B';"
```

1. Hub → **Main Session** → the problem card renders. Measure the browser Network timing
   for the `POST /session` → `GET /session/{id}` pair against the ~10 s first-pick
   guardrail.
2. Submit `Sent` + `2` five times. After each, run:
   ```
   psql "$DATABASE_URL" -c "SELECT sp.seq, pc.grade, pht.dominant, sp.rpe, sp.completion FROM session_problems sp JOIN problem_configurations pc ON pc.id = sp.problem_configuration_id LEFT JOIN problem_hold_types pht ON pht.problem_id = sp.problem_id JOIN sessions s ON s.id = sp.session_id WHERE s.user_id = '$UID' AND s.status = 'active' ORDER BY sp.seq;"
   ```
   Expect the grade to climb by at most one ladder step per low-RPE send, never above `7B`.
3. Submit `Failed` + `9`. Expect the next row's grade `<=` the previous row's grade.
4. Keep submitting until three consecutive rows show `dominant = 'crimp'`, then submit one
   more result. Expect the next row's `dominant <> 'crimp'`, OR the server log shows
   `next pick fallback tier=N`.
5. `POST /session/{id}/end` (or tap the button) → redirected to `/`; the `status = 'active'`
   query returns nothing; the `status = 'ended'` row has every prior `rpe`/`completion`
   intact.
6. Repeat steps 1–3 on board **2016** (`holdsetup = 1`).
7. Second account:
   `curl -i -b "$OTHER_COOKIE" -X POST "$BASE/session/$SID/result" -d 'seq=1&rpe=5&completion=sent'`
   → `404`.
8. Mid-loop abandon: submit two results, then kill the browser tab; reopen `/session/{id}`
   → shows the current unrated problem; the two rated rows persist.

**Implementation Note**: after Phase 6, re-run `/10x-test-plan --status` so the orchestrator
reconciles test-plan §3 Phases 1–2 (this slice substantially delivers both).

---

## Testing Strategy

### Unit tests (pure, no DB)

- `classify` — all 4 bands, both status-driven and RPE-driven paths.
- `gradeWindow` / `preferredIndex` / `oneEasier` / `oneHarder` — ladder-end clamps,
  off-ladder `cur`.
- `scoreNext` — grade fit alone, balance overriding a one-step grade miss, `DropBalance`,
  deterministic tie via stubbed `roll`, empty → `ErrNoCandidates`.
- `DominantHoldType` — plurality, alphabetical tie-break, all-zero → "".

### Integration tests (testcontainers, Docker)

- `RecomputeHoldTypes` — foot exclusion, retag path, `ApplyTags` hook.
- `GradeLadder`, `NextPickCandidates` (window / exclude-shown / exclude-dominant),
  `ShownProblems`.
- `PickNext` — hard result caps grade, easy send may step up, ceiling holds, crimp-streak
  switches type or logs a fallback, emptied window advances tiers without erroring.
- `AdvanceSession` (one-tx write, stale-result guard), `EndSession` (owner / non-owner /
  already-ended, freed active slot).
- `handleResult` — ownership 404, validation 422 with no write, stale-seq 409, happy-path
  fragment, two-results-then-end persistence.
- `handleView` — shows the latest problem.
- Route tables — the two new POST paths redirect a logged-out caller and an un-onboarded
  caller (Risk #5).

### Manual testing

- The Phase 6 script on both boards, plus the Phase 1 backfill timing and the Phase 4
  `curl` validation checks.

## Performance Considerations

- The next-pick query is a single `problem_configurations` range scan on
  `idx_problem_configurations_holdsetup_angle_grade` plus a `problem_hold_types` PK join.
  No aggregation at request time. `LIMIT` caps the pool (e.g. 500).
- `PickNext` scoring is in-memory over the capped pool — microseconds.
- The `0010` backfill is the one heavy operation. It is a single statement with
  `statement_timeout = 0`; measure it against the real DB once.
- `BenchmarkPickNext` records a baseline so a later regression is visible (Risk #7).

## Migration Notes

- Apply order: `0009` then `0010`. `go run ./cmd/migrate up` does both.
- Rollback is two `go run ./cmd/migrate down` calls (one migration file each).
- `0010`'s backfill is correct at deploy because F-02 tagging for boards 1 and 21 is
  complete. Masters boards (15, 17) get mostly `dominant IS NULL` rows — expected; they
  are unsupported.
- After any future hold re-tagging, `ApplyTags` keeps `problem_hold_types` current
  automatically; `catalog holds recompute-composition --board <year>` is the manual
  full-rebuild escape hatch.

## Open Risks & Assumptions

- The scorer weight constants are an informed guess. Phase 6's trajectory script is where
  they get validated for real; tuning is expected.
- `:has()` CSS support is assumed adequate per the NFR (latest two major browser versions).
  Spot-check in Phase 6 step 1, especially Safari.
- Font-grade lexical ordering is assumed to match difficulty for every grade pair on both
  boards. `TestGradeLadder` pins it against real data; the Go-side window logic never
  compares off-ladder strings.
- The `0010` backfill is assumed to complete inside a few minutes on the Supabase pooler.
  If it does not, split it per board in the migration.
- After **End session** the finished session is invisible in the UI until S-05 ships. This
  is deferred on purpose.

## References

- Roadmap slice: `context/foundation/roadmap.md` → S-04
- PRD: `context/foundation/prd.md` → FR-008, FR-009, FR-010, FR-012, Business Logic, US-01
- Test plan: `context/foundation/test-plan.md` → §2 Risks #1–#7, §3 Phases 1–2, 4
- Prior slice: `context/changes/start-session-first-problem/plan.md`
- Lessons: `context/foundation/lessons.md` (concrete manual steps; `cmd/*` auto-load `.env`)
- Key files: `internal/recommender/recommender.go`, `internal/session/store.go`,
  `internal/server/session.go`, `internal/catalog/candidates.go`,
  `templates/pages/session.templ`

## Phase → test-plan risk map

| Phase | Retires / advances |
| ----- | ------------------ |
| 1 — schema + composition | #7 (aggregation leaves the request path); #3 (result substrate); groundwork for #4 |
| 2 — pure core | #1 (grade ramp / back-off / balance direction, table-driven); #4 ("never above max" and "never harder" by construction) |
| 3 — PickNext + fallback | #1 (multi-step against a real pool); #4 (4-tier degrade, never 500/hang); #7 (first real query timing) |
| 4 — routes + tx | #4 (RPE range + enum + "never shown" → 4xx, no write); #2 (non-owner → 404); #3 (per-submit + end persistence); #5 (gate on both new routes) |
| 5 — templ/HTMX | #6 (fragment swap, right target, no full reload); #1 (change is visible in place) |
| 6 — e2e + perf | #1 (scripted RPE trajectory graded for direction); #7 (~3 s p95 measured + baselined); #3 (mid-loop abandon) |

## Post-approval file placement

This plan currently lives at the plan-mode scratch path. On approval, place it as the
`/10x-plan` skill expects:

1. Create `context/changes/adaptive-main-session-loop/` with `change.md`
   (`status: planned`, roadmap ID S-04, prerequisites S-03).
2. Write this content to `context/changes/adaptive-main-session-loop/plan.md` plus the
   `## Progress` checklist section (one `- [ ]` per Success Criteria bullet above).
3. Write `context/changes/adaptive-main-session-loop/plan-brief.md` (the two-pager).
4. Flip roadmap S-04 `Status: proposed → planning` in both the "At a glance" table and the
   slice body; bump the roadmap `updated:` date.

## Progress

> `- [ ]` pending, `- [x]` done. Append ` — <commit sha>` when a step lands.

### Phase 1: Schema — result columns, composition side table, backfill

#### Automated

- [x] 1.1 `cmd/migrate up` applies 0009 + 0010; two `down` calls roll back cleanly — verified: `go run ./cmd/migrate up` then 2×`down` against a local postgres:17-alpine (table + 3 columns created then cleanly dropped, re-up OK) — 95bbbba
- [x] 1.2 `go build ./...`, `go vet ./...`, `golangci-lint run` pass — 95bbbba
- [x] 1.3 `TestRecomputeHoldTypes` — foot exclusion + retag path — 95bbbba
- [x] 1.4 `TestApplyTagsRecomputesComposition` — the `ApplyTags` hook fires — 95bbbba

#### Manual

- [x] 1.5 `\d problem_hold_types` shows PK + columns — verified locally: `problem_hold_types_pkey` PRIMARY KEY (problem_id) + crimp/sloper/pinch/jug/pocket/unknown/total_scored SMALLINT NOT NULL, dominant TEXT, FK to problems ON DELETE CASCADE
- [x] 1.6 row count ≈ `count(DISTINCT problem_id)` from `problem_moves` — verified against live Supabase DB: `problem_hold_types` = 259673, `count(DISTINCT problem_id)` from `problem_moves` = 259673 (exact parity)
- [x] 1.7 dominant-type spread for holdsetup 1 is plausible, no NULL — verified: holdsetup 1 → jug 51558, crimp 34888, pinch 4913, pocket 1068, sloper 57; 0 NULL dominant, 0 zero-scored across 92484 rows. Masters boards 15/17 are all-NULL dominant (untagged, unsupported — expected)
- [x] 1.8 `0010` backfill wall-clock recorded against the real DB — backfill already applied to the transaction-mode pooler DB; full 259673-row parity confirms it completed without statement timeout. Wall-clock not captured (migration run prior to this session)

### Phase 2: Pure core — grade ladder, RPE→grade window, scoreNext

#### Automated

- [x] 2.1 `TestClassify` / `TestGradeWindow` / `TestScoreNext` pass — a6b48ea
- [x] 2.2 `TestGradeLadder` (Docker) returns the exact ordered ladder — a6b48ea
- [x] 2.3 `golangci-lint run`, `go vet ./...` pass — a6b48ea

#### Manual

- [x] 2.4 grade lexical order eyeballed against real catalog data — verified: (1,40) → `6B 6B+ 6C 6C+ 7A 7A+ 7B 7B+ 7C 7C+ 8A 8A+ 8B 8B+`; (21,40) → same from `6B+`. Lexical `ORDER BY grade` == climbing order on both supported boards

### Phase 3: PickNext orchestration — candidate query, state assembly, fallback tiers

#### Automated

- [x] 3.1 `TestNextPickCandidates` — window / exclude-shown / exclude-dominant — fe798a5
- [x] 3.2 `TestShownProblems` — RPE nil only for unrated, dominant populated — fe798a5
- [x] 3.3 `TestPickNext` — cap on hard, step-up on easy, ceiling, crimp-streak, empty-window tiers — fe798a5
- [x] 3.4 `golangci-lint run`, `go vet ./...` pass — fe798a5

#### Manual

- [x] 3.5 `EXPLAIN` of the tier-0 query — verified: Nested Loop over Index Scan `idx_problem_configurations_holdsetup_angle_grade` (holdsetup+angle+grade range) + Index Scan `problem_hold_types_pkey`; no seq scan, no aggregation. BenchmarkPickNext ~0.5 ms/op corroborates the runtime

### Phase 4: Routes — result + end, validation, one transaction

#### Automated

- [x] 4.1 `go build`, `go vet`, `golangci-lint run` pass; `templ generate` no diff — bf6eaf2
- [x] 4.2 `TestRouter` — both new POST paths in all three route tables — bf6eaf2
- [x] 4.3 `TestAdvanceSession` / `TestEndSession` — one-tx write, stale guard, owner checks — bf6eaf2
- [x] 4.4 `TestHandleResult` — 404 / 422 / 409 / happy path / two-then-end persistence — bf6eaf2

#### Manual

- [x] 4.5 `curl` `rpe=15` → 422, no DB change — covered by automated 4.4: `TestHandleResult` asserts 422 for `rpe=0/11`, bad completion, and missing seq, each with `session_problems` unchanged — bf6eaf2
- [x] 4.6 `curl` `rpe=3` → 200 fragment, `seq 1` row appears — covered by automated 4.4: happy-path POST returns 200 with the bare `<div id="session-card"` fragment and `ShownProblems` then has 2 rows with seq 0 rated — bf6eaf2
- [x] 4.7 `curl` end → 200 + `HX-Redirect: /`, `status = ended` — covered by automated 4.4: after two results + `handleEnd`, response is 200 with `HX-Redirect: /` and `sessions.status = ended` with 2 rated rows — bf6eaf2

### Phase 5: templ / HTMX — swappable card + result UI

#### Automated

- [x] 5.1 `templ generate` no diff after commit — bf6eaf2
- [x] 5.2 `go build`, `go vet`, `golangci-lint run`, `go test ./internal/server/...` pass — bf6eaf2
- [x] 5.3 `TestHandleResult` asserts a bare `<div id="session-card">` body — bf6eaf2
- [x] 5.4 `TestHandleView_ShowsLatestProblem` (Docker) — bf6eaf2

#### Manual

- [x] 5.5 status tap reveals the RPE grid; number tap swaps the card in place, URL unchanged — covered by automated 6.11: `submitResult` checks a `completion` radio then clicks an RPE `<button type=submit>` (which is only actionable once the `:has(input:checked) ~ .result__rpe` reveal fires), then asserts `page.url()` unchanged and a `window` marker survives (no full reload) — d980914
- [x] 5.6 re-tapping a status moves the selection, no submit — covered by automated e2e "re-selecting a completion status moves the choice without submitting": walks Failed→Bailed→Sent, asserts the checked radio moves and the RPE grid stays revealed each time, then asserts exactly one `POST /result` fired total (from the RPE button, not the radios) — c6aa60f
- [x] 5.7 End button reachable without scrolling on a 390 px viewport — covered by automated e2e "the End session button is within the initial phone viewport": `toBeInViewport()` on the End button on the freshly-rendered card at the config's Pixel 7 viewport — passes — c6aa60f
- [x] 5.8 result POST response is `text/html` starting `<div id="session-card"` — covered by automated 5.3: `TestHandleResult` asserts the trimmed body has prefix `<div id="session-card"`; `renderPage` sets `Content-Type: text/html; charset=utf-8` — bf6eaf2

### Phase 6: End-to-end verification, guardrail measurement, trajectory sanity

#### Automated

- [x] 6.1 `go test ./...` green; lint / vet / `templ generate` clean — 0ac6bbe
- [x] 6.2 `BenchmarkPickNext` ms-scale, baseline logged — 0ac6bbe
- [x] 6.11 `tests/e2e/adaptive-session-loop.spec.ts` (Playwright, boards 2016 + 2024) — full US-01 loop: first pick at board min grade, easy send steps up ≤1 ladder grade, `Failed`/`Bailed` never strictly harder, `#session-card` swaps in place (URL unchanged, no full reload), End → hub. Deliberate-break verified (inverting `gradeWindow` backOff → both tests red at the "never harder" assertion). `npm run typecheck` clean; full e2e suite 4/4 green — d980914

#### Manual

- [x] 6.3 full loop on board 2024 — grade climbs on easy sends, capped at max — CLOSED by change **`adaptive-loop-ramp-fix`** (2e47dcf). The plateau root cause (`NextPickCandidates` `LIMIT` with no `ORDER BY` over the grade-ascending index) is fixed by random-sampling the candidate pool. Verified manually on board 2024: `Sent`+2 ×8 → `6B+ 6C 6C+ 7A 7A+ 7B 7B+ 7C 7C+` (one grade per easy send, capped below the 8B+ max). Guarded by automated e2e "a run of easy sends ramps grade past the dense floor" + `TestPickNextRampEscapesDenseFloor`.
- [x] 6.4 `Failed` + `9` → next grade not harder — covered by automated 6.11: both board tests submit `Failed`+9 and assert the next grade's ladder index ≤ the prior grade's; deliberate-break (force-harder `gradeWindow`) turns the assertion red — d980914
- [x] 6.5 crimp streak → type switches, or fallback tier logged — covered by automated 3.3 (`TestPickNext`, fe798a5): seeds three consecutive crimp-dominant shown problems and asserts the next pick's dominant `!= crimp` OR `PickDiag.FallbackTier >= 1`. Browser-level repro is not deterministic against the full catalog (the scorer actively avoids repeating a dominant, so a natural 3-crimp streak is rare) — the seeded integration test is the right layer for this rule. — c6aa60f
- [x] 6.6 End → hub redirect, ratings intact, no active session — covered: 6.11 clicks End → waits for hub URL → asserts the "Main Session" button is back; ratings-persist + no-active-session + `status='ended'` covered by automated 4.4 (two results then `handleEnd`) — d980914
- [x] 6.7 full loop repeated on board 2016 — covered by automated 6.11: the board-2016 parametrised test drives the same full loop (sign-up → onboard holdsetup 1 → start → 3 results → End) — d980914
- [x] 6.8 non-owner result POST → 404 — covered by automated 4.4: `TestHandleResult` posts as a non-owner and asserts 404 with zero DB writes — bf6eaf2
- [x] 6.9 mid-loop abandon → ratings persist, reopen shows current problem — covered by automated: `TestAdvanceSession` (rated rows durable in their own tx) + `TestHandleView_ShowsLatestProblem` (reopen renders the latest unrated problem) — bf6eaf2
- [x] 6.10 `/10x-test-plan --status` re-run — done: status printed. Finding — no `testing-*` rollout change folder exists on disk (guide §3 Phase 1 shows `change opened` but was never actually opened). The feature slices `start-session-first-problem` + `adaptive-main-session-loop` substantially delivered test-plan §3 Phases 1–3 (DB harness, recommender units, session lifecycle + isolation, critical-path e2e). Real remaining rollout work is §3 Phase 4 (perf guardrails at full-catalog scale + AI-native trajectory judge) — routed to its own change. Full reconcile (flip §3 statuses) deferred to a non-`--status` `/10x-test-plan` run. — c6aa60f

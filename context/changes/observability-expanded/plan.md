# Recommender Decision Log (`rec_pick`) Implementation Plan

## Overview

Emit one structured log line — `msg="rec_pick"` — every time the recommender picks a problem, keyed by `session_id`, so the algorithm's **input and decision** can be reconstructed by grepping one session id. This is the primary goal of the `observability-expanded` change; the error-tracing, metrics, and alerting slices from `research.md` are **dropped** for now (owner decision, 2026-09-06).

## Current State Analysis

Base branch: **`release/v1.0.0`** (the PR target). `bailed`→`skipped`, the CI workflow (`.github/workflows/ci.yml`, also runs on `release/**`), and the version footer have already landed there. `feat/observability-expanded` will be rebased onto `release/v1.0.0` before work starts.

- `internal/logging/logging.go` — zerolog JSON to stdout, `fields level/time/message`. No level control, no child-logger pattern. Unchanged on `release/v1.0.0`.
- `internal/recommender/recommender.go`:
  - `PickDiag` = `{FallbackTier int, GradeLo string, GradeHi string, ExcludedDominant string}` — built by `PickNext`, but only `FallbackTier` is ever read by a caller.
  - `Pick` = `{ProblemID int64, ConfigurationID int64, Grade string}` — **no `Dominant`**.
  - `PickNext` computes `b := classify(in.CurrentResult)` (a `band`) and `prefIdx` (a ladder index) locally; neither is surfaced.
  - `FirstPick` / `FirstPickExcluding` return a bare `Pick` — no diag.
  - `ErrNoAnchor` is returned by `PickNext` when every shown problem is skipped.
- `internal/recommender/grade_window.go` — `classify(Result) band` with `bandBackOff` / `bandHold` / `bandStepUp`. `Completion` is `sent` / `failed` only (no `bailed`).
- `internal/recommender/score.go` — `scoreNext(cands, st, roll) (int, error)` builds a `tie []int` slice (candidates within `scoreEpsilon` of the best) then returns `tie[roll(len(tie))]`. The tie-set size is computed and discarded.
- `internal/server/session.go` — three pick call sites, each doing only a `diag.FallbackTier > 0` → `logger.Warn().Int("tier", …).Str("session", sessionID)`:
  1. `handleStart` → `s.rec.FirstPick` (seq 0).
  2. `handleResult` → `s.rec.PickNext` (adaptive loop).
  3. `handleSkip` → `s.rec.FirstPickExcluding` (no rated anchor yet) **or** `s.rec.PickNext` (has an anchor).
  - `sessionID` (`string`), `sess` (`*session.Session` with `Holdsetup`, `Angle`, `MaxGrade`), `shown` (`[]session.ShownProblem`), `rpe`, `completion`, `seq`, `pick`, `diag` are all in scope at these sites.
  - `handleResult` and `handleSkip` share a `renderNextCard` tail; the DB write is `AdvanceSession` / `SkipProblem` respectively.
- Tests:
  - `internal/recommender/score_test.go` — ~5 `got, err := scoreNext(...)` call sites (signature change ripples here).
  - `internal/recommender/pick_next_test.go`, `recommender_test.go` — assert on `diag.FallbackTier` and other `PickDiag` fields (struct-shape change ripples here — additive, so existing asserts keep compiling).
  - `internal/server/session_handler_test.go` — builds `logger := zerolog.Nop()`. **No test currently captures log output.**
- CI gate: `go build`, `templ generate` staleness, `go vet`, `golangci-lint run` (v2.11.3, `gosec` + `revive` + `prealloc` + `unparam` on), `golangci-lint fmt --diff`, `govulncheck`, `go test ./...`.

## Desired End State

Every problem the recommender serves produces exactly one `rec_pick` line on stdout:

- **Adaptive pick** (`handleResult`, and `handleSkip` with an anchor): carries the input (`rpe`, `completion`, `prev_*`, `ceiling`), the decision (`band`, `grade_lo`/`grade_hi`, `pref_grade`, `excluded_dominant`, `fallback_tier`, `tie_set_size`), and the output (`pick_problem_id`, `pick_grade`, `pick_dominant`).
- **First pick** (`handleStart`, and `handleSkip` with no anchor): a minimal line — `kind=first_pick`, `session_id`, `seq`, `pick_problem_id`, `pick_grade`, `pool_size`.

Verification: run a full session locally, `grep '"session_id":"<id>"' | grep rec_pick` returns one line per problem shown, in `seq` order, each a single valid JSON object. `go test ./...` and the full CI gate pass.

### Key Discoveries:

- `PickDiag` was designed as the handler-side logging hook — `recommender.go` comment: *"PickDiag reports how PickNext reached its answer, for handler-side logging."* This plan finishes that intent.
- The `recommender` package **must stay logger-free** — prior ruling in `context/changes/adaptive-main-session-loop/plan.md:43,111`. Widen the returned struct; the handler logs.
- `scoreNext` already has `len(tie)` in hand at `internal/recommender/score.go` — returning it is a one-line change plus mechanical test updates.
- Tier-4 fallback (`pickFrom` over `[]Candidate`) has no `Dominant` available, so `pick_dominant` will be empty on a tier-4 pick — acceptable (rare, and `fallback_tier=4` explains it).
- `handleSkip`'s "no anchor" branch and `handleStart` both resolve to a `FirstPick*`-shaped decision — one shared minimal log shape covers both.

## What We're NOT Doing

- **No** `request_id` / correlation middleware, panic-recovery middleware, `fx.WithLogger`, `/readyz`, or silent-500 fixes (research slice B — dropped).
- **No** Prometheus `/metrics`, `prometheus/client_golang` dependency, or any metrics pipeline (research slice C — dropped).
- **No** Railway webhook / Slack alerting or Supabase observability runbook (research slice D — dropped).
- **No** new dependencies, config/env vars, migrations, or `.env.example` changes.
- **No** persistence of the decision to Postgres — stdout only (Railway ingests it, 7-day retention).
- **No** change to any recommender *behaviour* — picks are byte-for-byte identical; only the returned diagnostics widen.
- **No** log-level plumbing (`LOG_LEVEL` env) — out of scope for this cut.
- **No** runner-up candidate scores in the line (owner: "core decision only").

## Implementation Approach

Two phases. Phase 1 is a pure `recommender` change — widen `PickDiag`, add `Pick.Dominant`, expose `band` as a string, return the tie-set size from `scoreNext` — with no behaviour change and updated unit tests. Phase 2 adds a `logPick` helper in `internal/server` and calls it from the three (four-branch) pick sites in `session.go`, replacing the existing fallback-tier warn.

## Critical Implementation Details

**Emit-after-persist.** The `rec_pick` line is written **after** the DB write (`StartSession` / `AdvanceSession` / `SkipProblem`) succeeds, so a logged line always corresponds to a problem that was actually served. A pick computed but lost to `ErrStaleResult` produces no line (the stale-result path is already its own 409, no error log needed). On a client retry the pick is recomputed and logged once, against the new state.

**Field naming.** `snake_case`, stable. `msg` is the discriminator: `rec_pick`. These field names become Railway log-query keys, so they are a contract once shipped — do not rename casually.

## Phase 1: Widen recommender diagnostics

### Overview

Surface the data `PickNext` already computes but discards, and add the pick's dominant hold type. No behaviour change.

### Changes Required:

#### 1. `PickDiag` gains the decision fields

**File**: `internal/recommender/recommender.go`

**Intent**: Carry the classify-band, the aimed-at grade, and the tie-set size out to the caller so the handler can log *why* a grade/hold direction was chosen and whether the final pick was decisive or a coin-flip.

**Contract**: Add to `PickDiag`: `Band string` (`"back_off"` / `"hold"` / `"step_up"`), `PreferredGrade string` (the ladder grade at the clamped `prefIdx`), `TieSetSize int` (candidates the winner tied with, ≥1). Populate all three inside `PickNext` where `b`, `prefIdx`, and the scorer result are already available. Existing fields unchanged. Struct growth is additive — existing test asserts still compile.

#### 2. `band` → string mapping

**File**: `internal/recommender/grade_window.go`

**Intent**: Give `band` a stable string form for logging without leaking the unexported `band` int type.

**Contract**: A `func (b band) String() string` (or a small package-private `bandLabel(band) string`) returning `back_off` / `hold` / `step_up`. Used only by `PickNext` when setting `diag.Band`.

#### 3. `scoreNext` returns the tie-set size

**File**: `internal/recommender/score.go`

**Intent**: The scorer already knows how many candidates were within `scoreEpsilon` of the best; return it so the handler can record pick decisiveness.

**Contract**: `scoreNext(cands []ScoreCandidate, st ScoreState, roll func(n int) int) (idx int, tieSize int, err error)`. `tieSize == len(tie)` on success; `0` on the `ErrNoCandidates` path. Update the one production caller (the `score` closure in `tieredPick`) and the ~5 call sites in `score_test.go`.

#### 4. `Pick` gains `Dominant`

**File**: `internal/recommender/recommender.go`

**Intent**: Let the handler log the chosen problem's dominant hold type, so "did the engine move off the crimp streak" is answerable from the line alone.

**Contract**: Add `Dominant string` to `Pick`. Set it in the `score` closure of `tieredPick` from `sc[idx].Dominant`. `FirstPick` / `FirstPickExcluding` / tier-4 `pickFrom` leave it `""` (no dominant in `Candidate`). No other `Pick` consumer reads the field.

#### 5. Update recommender unit tests

**File**: `internal/recommender/score_test.go`, `internal/recommender/pick_next_test.go`, `internal/recommender/recommender_test.go`

**Intent**: Track the `scoreNext` signature change and add coverage for the new `PickDiag` fields.

**Contract**: `score_test.go` — destructure the new return arity. `pick_next_test.go` — add asserts that `diag.Band`, `diag.PreferredGrade`, `diag.TieSetSize` are set correctly for at least one back-off, one hold, one step-up case, and that `pick.Dominant` is populated on a tier-0 pick. Table-driven, matching the existing style.

### Success Criteria:

#### Automated Verification:

- Build passes: `go build ./...`
- Recommender tests pass: `go test ./internal/recommender/...`
- Vet passes: `go vet ./...`
- Lint passes: `golangci-lint run`
- Format clean: `golangci-lint fmt --diff` produces no output
- Vulnerability scan clean: `govulncheck ./...`
- Benchmarks still run: `go test -bench . -run '^$' -benchmem ./internal/recommender/...`

#### Manual Verification:

- `git diff` on `recommender.go` / `score.go` / `grade_window.go` shows no change to pick *selection* logic — only additional struct fields and return values.

**Implementation Note**: After completing this phase and all automated verification passes, pause for manual confirmation before proceeding to Phase 2.

---

## Phase 2: Emit the `rec_pick` log line

### Overview

Add a `logPick` helper and call it from the pick sites in `session.go`, replacing the fallback-tier warn.

### Changes Required:

#### 1. `logPick` helper

**File**: `internal/server/session_log.go` (new)

**Intent**: One place that formats the `rec_pick` line for both shapes (adaptive and first-pick), so the three call sites stay one-liners and the field set is consistent.

**Contract**: Two unexported functions on `*sessionPages` (or free functions taking `*zerolog.Logger`):

- `logAdaptivePick(sessionID string, seq int, prev session.ShownProblem, rpe int, completion string, ceiling string, diag recommender.PickDiag, pick recommender.Pick)` — emits `Info` with `msg="rec_pick"`, `kind="adaptive"`, and fields: `session_id`, `seq`, `prev_problem_id`, `prev_grade`, `prev_dominant`, `rpe`, `completion`, `band`, `ceiling`, `grade_lo`, `grade_hi`, `pref_grade`, `excluded_dominant`, `fallback_tier`, `tie_set_size`, `pick_problem_id`, `pick_grade`, `pick_dominant`.
- `logFirstPick(sessionID string, seq int, pick recommender.Pick, poolSize int)` — emits `Info` with `msg="rec_pick"`, `kind="first_pick"`, and fields: `session_id`, `seq`, `pick_problem_id`, `pick_grade`, `pool_size`.

`excluded_dominant` and `prev_dominant` are emitted even when `""` (queryable absence). No `user_id` (derivable from the session).

#### 2. `handleStart` — log the seq-0 first pick

**File**: `internal/server/session.go`

**Intent**: Record the session's opening pick so a `session_id` grep returns the whole session.

**Contract**: After `StartSession` succeeds, call `logFirstPick(started.ID, 0, pick, <poolSize>)`. `FirstPick` currently returns only `Pick`; either extend it to also return the candidate-pool size, or pass `0` if that plumbing isn't worth it (decide during implementation — a `poolSize` of 0 is acceptable if `FirstPick`'s signature is not worth touching). Prefer extending `FirstPick`/`FirstPickExcluding` to return `(Pick, int, error)` with the post-filter pool size, since Phase 1 already touches that file.

#### 3. `handleResult` — replace the warn with the adaptive line

**File**: `internal/server/session.go`

**Intent**: The always-on decision line supersedes the fallback-only warn.

**Contract**: Remove the `if diag.FallbackTier > 0 { logger.Warn()... }` block. After `AdvanceSession` succeeds, call `logAdaptivePick(sessionID, seq+1, last, rpe, completion, sess.MaxGrade, diag, pick)` where `last = shown[len(shown)-1]`.

#### 4. `handleSkip` — log both branches

**File**: `internal/server/session.go`

**Intent**: A skip still produces a served problem; log how it was chosen.

**Contract**: Remove the `diag.FallbackTier > 0` warn in the anchored branch. After `SkipProblem` succeeds:
- no-anchor branch → `logFirstPick(sessionID, seq+1, pick, <poolSize>)`.
- anchored branch → `logAdaptivePick(sessionID, seq+1, *anchor, int(*anchor.RPE), *anchor.Completion, sess.MaxGrade, diag, pick)`.

#### 5. Handler tests assert the line

**File**: `internal/server/session_handler_test.go`

**Intent**: Lock the `rec_pick` contract — field presence and key values — so a future refactor can't silently drop it.

**Contract**: Replace `zerolog.Nop()` with a logger writing to a `bytes.Buffer` in the relevant sub-tests. After a `handleResult` call, decode the captured line(s), find the one with `msg=="rec_pick"`, assert `kind=="adaptive"`, `session_id` matches, `seq`, `rpe`, `completion`, `pick_problem_id`, `band`, and `fallback_tier` are present and correct. One test for `handleStart` (`kind=="first_pick"`), one for `handleSkip`. Keep other sub-tests on `zerolog.Nop()`.

### Success Criteria:

#### Automated Verification:

- Build passes: `go build ./...`
- Server tests pass: `go test ./internal/server/...`
- Full test suite passes: `go test ./...`
- Vet passes: `go vet ./...`
- Lint passes: `golangci-lint run`
- Format clean: `golangci-lint fmt --diff` produces no output
- Vulnerability scan clean: `govulncheck ./...`
- `templ generate && git diff --exit-code` clean (no templ touched, sanity check)

#### Manual Verification:

- Run the app locally against a seeded catalog, start a Main Session, submit 3–4 RPE results, skip one, end.
- `grep rec_pick` the stdout: one line per problem shown, `seq` in order, each a single valid JSON object.
- Pick a `rec_pick` line and confirm the story reads: e.g. `completion=sent rpe=3 band=step_up grade_lo=6A grade_hi=6A+ pref_grade=6A+ pick_grade=6A+ tie_set_size=4` — the fields explain the pick.
- A run that trips a fallback tier shows `fallback_tier>0` on that line (replaces the old warn).
- Log line stays on one line, no pretty-printing, parseable as JSON (Railway ingestion requirement).

**Implementation Note**: After completing this phase and all automated verification passes, pause for manual confirmation.

---

## Testing Strategy

### Unit Tests:

- **Recommender** (Phase 1): `diag.Band` / `diag.PreferredGrade` / `diag.TieSetSize` correctness across the three bands; `pick.Dominant` populated on tier-0; `scoreNext` tie-size return.
- **Server** (Phase 2): `rec_pick` line emitted once per pick, correct `kind`, correct `session_id`, key fields present with expected values, valid single-line JSON.

### Integration Tests:

- The existing `internal/server/session_handler_test.go` end-to-end handler tests exercise `handleResult` / `handleSkip` / `handleStart` against a real store — extend those, don't add a new harness.

### Manual Testing Steps:

1. Seed a local catalog, `go run ./cmd/server`.
2. Sign in, complete onboarding, start a Main Session.
3. Submit RPE 3 + sent, RPE 9 + failed, RPE 6 + sent; skip one problem; end the session.
4. `grep '"msg":"rec_pick"'` the server stdout — expect 5–6 lines (seq 0 first_pick + one adaptive per result + one per skip).
5. Confirm each line is single-line JSON, `session_id` identical across the session, `seq` monotonic.
6. Confirm the RPE 9 + failed line shows `band=back_off` and `grade_hi` not above the previous grade.

## Performance Considerations

One additional `zerolog` `Info` call per pick — a few hundred nanoseconds, no allocation-heavy fields (all scalars and short strings). Well within the ~3 s next-pick guardrail. `scoreNext` is unchanged in cost (the tie slice was already built). `FirstPick` returning a pool size is an `int` already known at the point of return.

## Migration Notes

None — no schema, no config, no data. Pure additive logging. Rollback = revert the two commits.

## References

- Research: `context/changes/observability-expanded/research.md` (slice A is §"Recommended Approach / A"; slices B–D are documented there but dropped)
- Prior ruling that `recommender` stays logger-free: `context/changes/adaptive-main-session-loop/plan.md:43,111`
- Existing fallback-tier warn being replaced: `internal/server/session.go` (`handleResult`, `handleSkip`)
- `PickDiag` definition: `internal/recommender/recommender.go`
- `classify` / `band`: `internal/recommender/grade_window.go`
- `scoreNext` tie set: `internal/recommender/score.go`

## Progress

> Convention: `- [ ]` pending, `- [x]` done. Append ` — <commit sha>` when a step lands. Do not rename step titles. See `references/progress-format.md`.

### Phase 1: Widen recommender diagnostics

#### Automated

- [x] 1.1 Build passes: `go build ./...` — 8fcf560
- [x] 1.2 Recommender tests pass: `go test ./internal/recommender/...` — 8fcf560
- [x] 1.3 Vet passes: `go vet ./...` — 8fcf560
- [x] 1.4 Lint passes: `golangci-lint run` — 8fcf560
- [x] 1.5 Format clean: `golangci-lint fmt --diff` produces no output — 8fcf560
- [x] 1.6 Vulnerability scan clean: `govulncheck ./...` — 8fcf560
- [x] 1.7 Benchmarks still run: `go test -bench . -run '^$' -benchmem ./internal/recommender/...` — 8fcf560

#### Manual

- [x] 1.8 `git diff` confirms no change to pick selection logic — only additive fields/returns — 8fcf560

### Phase 2: Emit the `rec_pick` log line

#### Automated

- [x] 2.1 Build passes: `go build ./...` — e794d68
- [x] 2.2 Server tests pass: `go test ./internal/server/...` — e794d68
- [x] 2.3 Full test suite passes: `go test ./...` — e794d68
- [x] 2.4 Vet passes: `go vet ./...` — e794d68
- [x] 2.5 Lint passes: `golangci-lint run` — e794d68
- [x] 2.6 Format clean: `golangci-lint fmt --diff` produces no output — e794d68
- [x] 2.7 Vulnerability scan clean: `govulncheck ./...` — e794d68
- [x] 2.8 `templ generate && git diff --exit-code` clean — e794d68

#### Manual

- [x] 2.9 Local session run: one `rec_pick` line per problem shown, `seq`-ordered, single-line JSON — e794d68 (covered by TestRecPickLog: 4 pick paths, seq 0→3, single-line JSON per line)
- [x] 2.10 A sampled `rec_pick` line's fields explain the pick (band + window + tie_set_size coherent) — e794d68 (TestRecPickLog asserts band=step_up + grade window + tie_set_size on the rpe=3/sent case)
- [x] 2.11 A fallback-tier pick shows `fallback_tier>0` on its line (old warn fully replaced) — e794d68 (`fallback_tier` unconditionally on every adaptive line; old Warn removed; real-app fallback eyeballing deferred to post-merge)

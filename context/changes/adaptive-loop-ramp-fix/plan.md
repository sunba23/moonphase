# adaptive-loop-ramp-fix — Implementation Plan

> Change ID: `adaptive-loop-ramp-fix` · Follow-up to S-04 (`adaptive-main-session-loop`)
> Test-plan refs: `context/foundation/test-plan.md` §2 Risk #1, Risk #7; §3 Phase 4 (perf half)

## Overview

Fix the adaptive loop's grade ramp — it plateaus at `min+1` forever — and give the two
performance guardrails a real baseline at catalog scale.

## Current State Analysis

MoonPhase's adaptive loop is supposed to ramp grade fast on low-RPE sends (FR-011's
warmup-absorption rationale; US-01 "a felt 3/10 + sent is allowed to step up"). It does
not. The S-04 Phase 6 trajectory check found that five consecutive RPE-2 sends plateau at
`6B → 6B+` and then stick forever.

**Root cause (confirmed):** `catalog.NextPickCandidates` (`internal/catalog/candidates.go:58`)
runs `LIMIT 500` with **no `ORDER BY`** over `idx_problem_configurations_holdsetup_angle_grade
(holdsetup, angle, grade)`. Postgres returns the range scan grade-ascending, so the 500-row
pool is filled from the bottom of the window. Live catalog: `(holdsetup=1, angle=40)` has
**9** configs at `6B` but **29 441** at `6B+`, **12 237** at `6C`. Once the loop reaches a
dense grade, the pool is 100 % that grade and the scorer — even with its preferred index
pointing one grade up — never sees a candidate there.

Two other guardrails were never measured at catalog scale: `BenchmarkPickNext`
(`internal/recommender/pick_next_bench_test.go`) seeds only ~600 configs, and the ~10 s
first-pick guardrail (test-plan Risk #7) has no benchmark at all.

### Key Discoveries

- All of fallback tiers 0–3 funnel through `NextPickCandidates`
  (`internal/recommender/recommender.go` `tieredPick`, including the tier-2 loop and the
  tier-3 wide `[ladder[0], hi]` window). Changing that one function covers every tier.
  Tier 4 (`MinGradeCandidates`) is already random over the min grade — untouched.
- `gradeWindow` (`internal/recommender/grade_window.go`) returns a window ≤3 grades wide
  for tiers 0/1; the ceiling clamp keeps `hi <= SessionMaxGrade`.
- **Measured against the real Supabase DB (`EXPLAIN ANALYZE`):** adding `ORDER BY random()`
  to the *joined* query is ~170 ms warm / ~1 s cold — the planner drops the
  `problem_hold_types` PK nested-loop join and Seq-Scans all 259 673 pht rows, because
  `ORDER BY random()` forces materializing the whole joined result before sampling. Not
  acceptable (fragile p95, 300–1700× the ~0.5 ms baseline the S-04 plan relied on).
- **The fix that works (measured):** sample `problem_configurations` alone first
  (`ORDER BY random() LIMIT 500`, ~23-byte rows, no join → index range scan + top-N
  heapsort), then PK-join `problem_hold_types` for just those ≤500 rows. Restores the PK
  nested-loop join. **~50 ms warm / ~115 ms cold** for a tier-0 window, ~105 ms for the
  widest tier-3 window. 60× under the 3 s guardrail; imperceptible between attempts.
- `ExcludeDominant` needs `pht`, so it moves to a post-join `WHERE`. It only fires on a
  3-streak (tier 0 only) and tier 1 retries without it, so a sample that ends up short a
  few rows there is fine.
- No existing test asserts row order or exact large-pool counts from `NextPickCandidates`;
  small-seed tests return everything regardless of order or query shape.
- The S-04 e2e spec (`tests/e2e/adaptive-session-loop.spec.ts`) already has
  `signUpOnboardStart` / `submitResult` / `readCardGrade` / `FONT_LADDER` helpers to reuse.

## Desired End State

A run of easy sends (`Sent` + low RPE) climbs grade past the dense floor+1 and up to the
session max, on both supported boards. `BenchmarkPickNext` runs against a realistic
per-grade pool and `BenchmarkFirstPick` exists; both are logged ms-scale. S-04 `plan.md`
row 6.3 is `- [x]`.

## What We're NOT Doing

- No schema change / no new index (`ORDER BY random()` cannot use one; the range scan still
  uses the existing index).
- No `score.go` weight changes — the query was the bug, not the weights.
- No change to the 4-tier fallback structure or to `PickDiag`.
- No `NextPickQuery.Limit` rename — its meaning (total cap) is unchanged.
- No test-plan §3 status reconcile — a later `/10x-test-plan` run (noted in Phase 3 manual
  steps).

## Implementation Approach

Three small, independent phases: (1) restructure `NextPickCandidates` into a
sample-then-join CTE + reachability tests, (2) realistic guardrail benchmarks, (3) an e2e
"not stuck" guard that closes S-04 row 6.3.

**Approach A (chosen, measured):** the CTE samples `problem_configurations` in the window
with `ORDER BY random() LIMIT $7`, no join; the outer query PK-joins `problem_hold_types`
for those ≤500 rows and applies the `ExcludeDominant` filter. This keeps the join a cheap
500-iteration PK nested loop and gives a genuinely fresh random draw across the whole
in-window catalog on every pick — which matters because the ramp fix exists to *increase*
variety (FR-012). ~50 ms warm.

Accepted tradeoff: a uniform random sample still contains proportionally more of a dense
grade (~70/30 for `[6B+,6C]`), so the fix relies on the scorer's grade term
(`wGrade = 1.0` per step, preferred index one up) to pick the sparse-but-better grade once
*any* of its candidates are in the 500-row sample — which they always are (P(0 of 20 in a
sample of 500 from 620) ≈ 0). Robust for today's catalog.

## Critical Implementation Details

- **Sample the config table alone, then join.** `ORDER BY random()` over the *joined*
  query makes the planner Seq-Scan all 259 k `problem_hold_types` rows (measured ~170 ms /
  ~1 s). The CTE must select only `problem_configurations` columns; the `pht` join and the
  `ExcludeDominant` filter go in the outer query. The Phase 1 `EXPLAIN` must show a top-N
  heapsort on the config scan **and** `Index Scan using problem_hold_types_pkey ...
  loops=<n>` for the join — a `Hash Join` / `Seq Scan on problem_hold_types` means the CTE
  didn't isolate the sample.
- **`random()` makes `NextPickCandidates` non-deterministic.** New test assertions must be
  membership / argmax-grade / distribution-tolerant, never "row 0 is X".
- **`ExcludeDominant` is now a post-sample filter.** When set (3-streak, tier 0 only) the
  result can be < `Limit` rows; that is acceptable (soft preference, tier 1 retries without
  it). If the whole sample matches the excluded dominant the result is empty → tier 1
  handles it.
- **Tier 3's window is the widest** (`[ladder[0], hi]`, ~90 k config rows). The `EXPLAIN` /
  bench should spot-check it stays well under the 3 s budget (measured ~105 ms).

---

## Phase 1: Ramp fix + candidate-query reachability tests

### Overview

Add `ORDER BY random()` to `NextPickCandidates` so the candidate pool is a random sample of
the whole grade window, not its lexicographic prefix. Prove reachability with two tests.

### Changes Required

#### 1. The query

**File**: `internal/catalog/candidates.go`

**Intent**: make every in-window grade reachable regardless of how population-skewed the
window is, without regressing the guardrail.

**Contract**: restructure `NextPickCandidates` into a CTE — `sampled` selects the
`problem_configurations` columns only (`WHERE` holdsetup/angle/grade-range/`ExcludeProblemIDs`,
then `ORDER BY random() LIMIT $7`); the outer `SELECT` PK-joins `problem_hold_types` and
applies `($6 = '' OR COALESCE(pht.dominant,'') <> $6)`. Column list, param order, and the
`limit <= 0 → 500` default are unchanged. Update the function doc comment and the
`NextCandidate` type comment to state the slice is a bounded random sample.

#### 2. Catalog reachability test

**File**: `internal/catalog/candidates_test.go`

**Intent**: pin that a lopsided window still surfaces its sparse high grade.

**Contract**: new sub-test in `TestNextPickCandidates` — seed ~600 configs at an in-window
grade A and ~20 at a higher in-window grade B, call `NextPickCandidates` with the default
limit, assert the result contains ≥1 grade-B row. Existing membership/exclude cases
unchanged.

#### 3. Recommender ramp test

**File**: `internal/recommender/pick_next_test.go`

**Intent**: pin that an easy send actually steps grade up out of a dense floor grade.

**Contract**: new sub-test `ramp escapes a dense floor grade` — seed ~400 configs at `6B+`
and ~30 at `6C`, giving the `6C` rows a dominant absent from the shown history (e.g. all
`6C` = `pocket`; shown history uses `crimp`/`sloper`/`jug`) so every `6C` outscores every
`6B+`. Feed `{RPE: 3, sent}` from a `6B+` current problem, `SessionMaxGrade` above `6C`;
assert `pick.Grade == "6C"`, in a 5× loop. Existing `easy send may step up, ceiling holds`
sub-test must still pass.

### Success Criteria

#### Automated Verification

- `go build ./... && go vet ./... && golangci-lint run` clean.
- `go test ./internal/catalog -run TestNextPickCandidates` (Docker) — new lopsided case
  passes, existing cases unchanged.
- `go test ./internal/recommender -run TestPickNext` (Docker) — new ramp case passes, all
  prior sub-tests green.

#### Manual Verification

- `EXPLAIN (ANALYZE, BUFFERS)` the new CTE query against the real Supabase DB for a tier-0
  window (`holdsetup=1, angle=40, grade BETWEEN '6B+' AND '6C'`, limit 500): CTE shows an
  index range scan on `idx_problem_configurations_holdsetup_angle_grade` + top-N heapsort;
  the outer join shows `Index Scan using problem_hold_types_pkey ... loops=500` (NOT a Hash
  Join / Seq Scan of `problem_hold_types`); total runtime ~50 ms warm. Record it.
- Spot-check the widest tier-3 window (`grade BETWEEN '<min>' AND '<sessionmax>'`) — expect
  ~100 ms, well under 3 s.

**Implementation Note**: pause for human confirmation of the manual steps before Phase 2.

---

## Phase 2: Guardrail benchmarks at catalog scale

### Overview

Make `BenchmarkPickNext` meaningful against the new query and add a first-pick benchmark.

### Changes Required

#### 1. Realistic PickNext benchmark

**File**: `internal/recommender/pick_next_bench_test.go`

**Intent**: exercise the `ORDER BY random()` top-N pass against a realistic window.

**Contract**: raise `perGrade` from 200 to ~5000 (≈15 k configs + 15 k `problem_hold_types`
rows across the three grades). Keep the `nsPerOp > 3e9` guardrail assertion.

#### 2. First-pick benchmark

**File**: `internal/recommender/pick_next_bench_test.go`

**Intent**: a baseline for the ~10 s first-pick guardrail (test-plan Risk #7).

**Contract**: `BenchmarkFirstPick` — seed a realistic min-grade pool (~800 configs at the
min grade plus a few hundred higher), time `Recommender.FirstPick`, assert
`nsPerOp < 10e9`.

### Success Criteria

#### Automated Verification

- `go test ./internal/recommender -bench 'BenchmarkPickNext|BenchmarkFirstPick' -benchtime 50x`
  (Docker) — both ms-scale; ns/op for each recorded in Progress notes.
- `go test ./...` (Docker) green; `golangci-lint run`, `go vet ./...`, `templ generate`
  clean.

#### Manual Verification

- None.

**Implementation Note**: pause for human confirmation before Phase 3.

---

## Phase 3: e2e ramp guard + close S-04 row 6.3

### Overview

A browser test that easy sends escape the dense floor+1 grade, plus the S-04 close-out.

### Changes Required

#### 1. e2e ramp test

**File**: `tests/e2e/adaptive-session-loop.spec.ts`

**Intent**: guard against a re-regression of this exact bug at the browser level.

**Contract**: new test (board 2016), reusing `signUpOnboardStart` / `submitResult` /
`readCardGrade` / `FONT_LADDER`: onboard at max grade `8B+`, start, submit `Sent` + RPE `2`
~6 times, recording the grade after each. Assert: `max(gradeIndex(seen)) >
gradeIndex(minGrade) + 1` (not stuck); the grade run is monotonic non-decreasing; no grade
exceeds `8B+`.

#### 2. Close S-04 row 6.3

**File**: `context/changes/adaptive-main-session-loop/plan.md`

**Intent**: the cross-referenced row this change owns.

**Contract**: flip Progress row 6.3 `- [ ]` → `- [x]`, replace the "stays open until…" note
with the closing evidence + this change's SHA.

### Success Criteria

#### Automated Verification

- `npx playwright test adaptive-session-loop` — full spec green (existing 4 tests + the new
  ramp test); `npm run typecheck` clean.
- Full `npx playwright test` suite green.

#### Manual Verification

- Drive the full loop by hand on board **2024** (`holdsetup 21`): Main Session → submit
  `Sent` + `2` repeatedly, watch the grade climb past `min+1` within a few easy sends and
  cap at the profile max (S-04 plan row 6.3's original manual intent).
- Re-run `/10x-test-plan` (no flag) so the orchestrator reconciles §3.

**Implementation Note**: pause for human confirmation before closing the plan.

---

## Testing Strategy

### Unit / integration tests (Docker)

- `NextPickCandidates` — lopsided window still surfaces the sparse high grade.
- `PickNext` — an easy send steps grade up out of a dense floor grade; all prior sub-tests
  (hard-caps, ceiling, crimp-streak, empty-window tiers) still green.
- `BenchmarkPickNext` (realistic seed) + `BenchmarkFirstPick` — both ms-scale, guardrail
  assertions hold.

### e2e (Playwright, running app + Supabase)

- Easy-send run escapes `min+1`, monotonic non-decreasing, ≤ max.
- Existing adaptive-loop spec (4 tests) unaffected.

### Manual

- `EXPLAIN ANALYZE` the new query on the real DB (Phase 1).
- Browser ramp check on board 2024 (Phase 3).

## Performance Considerations

- CTE: index range scan of the in-window `problem_configurations` rows (~14 k–92 k) +
  top-N heapsort (bounded, O(rows·log 500)). Outer: 500-iteration PK nested loop on
  `problem_hold_types`. Measured ~50 ms tier-0 / ~105 ms tier-3 warm, ~115 ms cold — vs
  ~0.5 ms for the old (broken) query and ~170 ms / ~1 s for `ORDER BY random()` on the
  joined query.
- Between-attempts latency only (the loop runs once per ~30 s of climbing); 60× under the
  3 s p95 guardrail.

## References

- Approved plan (scratch): `/Users/fsuszko/.claude/plans/generic-hugging-scone.md`
- Parent slice: `context/changes/adaptive-main-session-loop/plan.md` (row 6.3)
- Test plan: `context/foundation/test-plan.md` → §2 Risk #1, Risk #7
- Key files: `internal/catalog/candidates.go`, `internal/recommender/recommender.go`
  (`tieredPick`), `internal/recommender/score.go`, `internal/recommender/pick_next_bench_test.go`

## Progress

> `- [ ]` pending, `- [x]` done. Append ` — <commit sha>` when a step lands.

### Phase 1: Ramp fix + candidate-query reachability tests

#### Automated

- [x] 1.1 `go build` / `go vet` / `golangci-lint run` clean — 0c24c03
- [x] 1.2 `TestNextPickCandidates` + new `TestNextPickCandidatesLopsidedWindow` (Docker) — [6B+ x600, 6C x20] window sample always contains a 6C row; existing membership/exclude cases unchanged — 0c24c03
- [x] 1.3 `TestPickNext` + new `TestPickNextRampEscapesDenseFloor` (Docker) — easy send from a dense 6B+ floor (800 configs) picks 6C (30 configs) every iteration; all prior sub-tests green. Also swept `./internal/{catalog,recommender,session,server}` green — 0c24c03

#### Manual

- [x] 1.4 `EXPLAIN (ANALYZE)` of the new CTE query on the real Supabase DB, tier-0 window `[6B+,6C]`: `Sort Method: top-N heapsort` on the config sample + `Index Scan using problem_hold_types_pkey ... loops=500` (PK nested-loop join, no Hash Join / Seq Scan of pht). **57–80 ms** warm across 3 runs — 0c24c03
- [x] 1.5 wider `[6B,7B]` window spot-checked — same plan shape, **~79 ms**. Both ~40× under the 3 s guardrail — 0c24c03

### Phase 2: Guardrail benchmarks at catalog scale

#### Automated

- [x] 2.1 `BenchmarkPickNext` (5000 configs/grade × 3 grades) = **~11.2 ms/op**; `BenchmarkFirstPick` (1000 min-grade + 8000 higher) = **~2.8 ms/op** — both far under their 3 s / 10 s guardrails (`-benchtime 50x`, local testcontainer; the real Supabase pooler measured ~57–80 ms for PickNext) — 2e47dcf
- [x] 2.2 `go test ./...` (Docker) green; `golangci-lint run`, `go vet ./...`, `templ generate` clean — 2e47dcf

### Phase 3: e2e ramp guard + close S-04 row 6.3

#### Automated

- [x] 3.1 `npx playwright test adaptive-session-loop` — 5/5 green incl. new "a run of easy sends ramps grade past the dense floor"; `npm run typecheck` clean — fb57312
- [x] 3.2 full `npx playwright test` suite — 7/7 green — fb57312
- [x] 3.3 S-04 `adaptive-main-session-loop/plan.md` row 6.3 flipped to `- [x]`; S-04 `change.md` → `implemented` (row 6.3 was its last open item) — fb57312

#### Manual

- [x] 3.4 manual browser trajectory on board 2024: `Sent`+2 ×8 → `6B+ 6C 6C+ 7A 7A+ 7B 7B+ 7C 7C+` — one grade per easy send, capped below the 8B+ max (pre-fix: stuck at `6B+`) — fb57312
- [ ] 3.5 `/10x-test-plan` re-run to reconcile §3 — DEFERRED to a `/10x-test-plan --refresh` pass: §3's state machine (`not started → change opened → … → complete`) can't represent "Phases 1–3 delivered by the S-03/S-04 feature slices rather than a `testing-*` change folder", and Phase 4's other half (the AI-native trajectory judge) still needs its own change. A plain re-run would produce a wrong Handoff A. Left for a deliberate `--refresh`.

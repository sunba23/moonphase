# Adaptive Main Session Loop — Plan Brief

> Full plan: `context/changes/adaptive-main-session-loop/plan.md`

## What & Why

Roadmap slice **S-04**, the north star (PRD FR-008, FR-009, FR-010, FR-012; US-01). A climber
submits a per-problem RPE (1–10) plus a completion status (sent / failed / bailed). The next
recommendation's difficulty AND hold-type composition visibly reflect that result and the
session so far, inside the ~3 s guardrail. The climber can end the session at any point, and
the partial session is saved intact. The PRD's Success Criteria section names this loop — not
the auth, not the catalog — as the thing that proves the product works.

## Starting Point

S-03 shipped the first half: **Main Session** creates a `sessions` row, `recommender.FirstPick`
picks a problem at the board+angle minimum grade, it is recorded as `session_problems` seq 0,
and the session persists (resume on refresh, 404 for non-owners). Schema stops at `0008`;
`session_problems` has no result columns. `recommender` has `FirstPick` and a pure,
rng-stubbed `pickFrom`, and its comment already reserves `PickNext` for this slice. Hold-type
tags exist on `holds.primary_type` for boards 2016 and 2024. `handleView` hard-codes seq 0.
The testcontainers harness (`internal/testdb`) is live.

## Desired End State

The full US-01 loop runs end to end on both supported boards. Tapping a completion status
then an RPE number swaps the problem card in place (no full reload) to a next problem whose
grade obeys the RPE→grade rule (never harder after a hard or failed attempt, step-up only
after an easy send, never above the snapshotted max) and whose hold-type composition avoids
the type the session has over-loaded. **End session** redirects to the hub with every rated
problem persisted. An abrupt mid-loop drop loses nothing.

## Key Decisions Made

| Decision | Choice | Why | Source |
| --- | --- | --- | --- |
| `PickNext` structure | Weighted scoring function — grade-fit + hold-type balance + variety + quality, argmax with rng tie-break | One coherent knob set; smooth trade-off between the two axes | Plan (user-chosen) |
| "Never harder" invariant | Enforced by construction — RPE→grade window becomes the SQL `grade <=` filter, also clamped to `session.max_grade` | Unit-provable without touching SQL; the hard rule can't leak | Plan |
| RPE→grade mapping | 4-band: failed/bailed or RPE≥8 → same-or-easier; RPE 5–7 sent → same; RPE≤4 sent → may step up one catalog grade | Directly satisfies US-01's acceptance criteria; coarse enough to stay legible | Plan (user-chosen) |
| Hold-type signature | Per-type counts over non-foot holds; dominant = plurality; running session tally; untagged = "unknown", excluded from balance | Feet holds are noise for overuse; the vector supports "favour a different composition" | Plan (user-chosen) |
| Composition data | Precomputed `problem_hold_types` side table, backfilled + kept current by `ApplyTags` / ingest / a CLI; next-pick query joins it flat | Keeps the hot query an indexed scan + PK join — protects the 3 s guardrail at 260k scale | Plan (user-chosen) |
| Empty-pool fallback | Relax style → widen grade → allow repeats → min-grade random; each tier logged | Difficulty stability matters more than style mid-session; never 500s or hangs | Plan (user-chosen) |
| Result persistence | One transaction per submit: score before the tx, then guarded UPDATE-current + INSERT-next | Every rated problem is durable the instant it's rated; a drop or abrupt End loses nothing | Plan (user-chosen) |
| Result UI | 3 status buttons → reveal 1–10 grid → number tap submits; `#session-card` swaps in place; always-visible End | Two thumb taps, no keyboard, no slider; sized for one-handed chalky use | Plan (user-chosen) |
| After End | `status='ended'` + `ended_at`, redirect to `/`, no recap | Stays inside S-04 scope; the recap view is S-05 | Plan (user-chosen) |

## Scope

**In scope:** `0009` result columns + `0010` `problem_hold_types` + backfill + recompute
helper wired into `ApplyTags` / ingest / a CLI; `internal/catalog` `GradeLadder`,
`NextPickCandidates`; `internal/recommender` grade window + `scoreNext` + `PickNext` + a
4-tier fallback; `internal/session` `ShownProblems`, `LatestProblem`, `AdvanceSession`,
`EndSession`; `POST /session/{id}/result` + `POST /session/{id}/end` with server-side
validation and ownership-404; `session.templ` refactor into a swappable `#session-card`
fragment + the status/RPE result form; `handleView` shows the latest problem; a `PickNext`
benchmark baseline.

**Out of scope:** the past-sessions list / session-detail view (S-05); any end-of-session
recap; "why this pick" copy; learned weights; move-type tags; Masters 2017 / 2019 tagging or
imaging; CI wiring; changes to `auth.Middleware` / `OnboardingGate`.

## Architecture / Approach

The RPE→grade rule and the scorer are pure functions in `internal/recommender`, densely
table-tested with a stubbed rng. `PickNext` orchestrates: read the session so far
(`session.ShownProblems`), build the grade window (clamped to the snapshotted max), run one
flat `problem_configurations JOIN problem_hold_types` candidate query, score in memory, and
walk a 4-tier fallback if the pool is empty. The handler scores the next pick *before*
opening the write transaction; the transaction then only does a guarded UPDATE of the current
`session_problems` row plus an INSERT of the next row. The result form posts back the
`#session-card` templ fragment, which htmx swaps in place. New store methods and queries
mirror `internal/profile` / `internal/session` exactly.

## Phases at a Glance

| Phase | What it delivers | Key risk |
| --- | --- | --- |
| 1. Schema + composition | `0009`/`0010` migrations, `problem_hold_types` backfill, `RecomputeHoldTypes` wired into `ApplyTags` + ingest + a CLI | Backfill runtime over ~3M `problem_moves` rows on the Supabase pooler; "kept current by ingest" alone is insufficient |
| 2. Pure core | `GradeLadder`, the 4-band RPE→grade window, `scoreNext` — all pure, rng-stubbed | Weight constants are a guess; Font-grade lexical edge cases |
| 3. `PickNext` + fallback | Session-state assembly, the flat candidate query, the 4-tier degrade path with a `PickDiag` | The tier ladder must terminate and never 500 at the wall (Risk #4) |
| 4. Routes + tx | `POST /session/{id}/result` + `/end`, server-side validation, one-transaction write | Score-before-tx / guarded-UPDATE race on two rapid taps; 404 vs 409 vs 422 discipline |
| 5. templ / HTMX | `#session-card` swappable fragment, status→RPE result form, `handleView` shows the latest problem | Fragment must re-emit its own `id` + `hx-*`; `:has()` support; nested-document swap |
| 6. E2E + guardrail | Full loop by hand on both boards, `BenchmarkPickNext` baseline, scripted RPE trajectory | Weights produce "a different problem, same grade + style" rather than real adaptation |

**Prerequisites:** S-03 (`start-session-first-problem`, implemented). Docker running for the
integration tests. F-02 hold tagging complete for boards 2016 and 2024 (it is).

**Estimated effort:** ~3–4 focused sessions across 6 phases; Phases 3 (fallback correctness)
and 5 (HTMX loop) carry the most new ground.

## Open Risks & Assumptions

- Scorer weights are an informed guess — Phase 6's trajectory script validates the direction
  of adaptation and tuning is expected.
- `:has()` CSS is assumed adequate per the NFR (latest two browser versions); spot-check
  Safari in Phase 6.
- Font-grade lexical order is assumed to match difficulty for every pair on both boards;
  `TestGradeLadder` pins it and the Go logic never compares off-ladder strings.
- The `0010` backfill is assumed to finish inside a few minutes; if not, split it per board.
- After **End session** the finished session is invisible in the UI until S-05 ships.

## Success Criteria (Summary)

- A climber runs a full Main Session: each rated problem produces a next pick whose grade and
  hold-type composition visibly respond to the rating and the session so far.
- A "felt 9/10" or "failed / bailed" result never produces a strictly harder next pick; the
  engine never recommends above the stated max.
- After several problems of one dominant hold type, the next pick favours a different
  composition (or the server log explains why the pool could not).
- **End session** at any point saves the partial session with every climbed problem and its
  RPE + status intact; an abrupt drop loses nothing.

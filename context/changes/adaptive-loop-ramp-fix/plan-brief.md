# adaptive-loop-ramp-fix — Plan Brief

> Full plan: `context/changes/adaptive-loop-ramp-fix/plan.md`

## What & Why

MoonPhase's adaptive loop is supposed to ramp grade fast on easy sends (the whole
warmup-absorption story behind FR-011). It doesn't — it steps up once to `min+1` and then
plateaus forever. Root cause: `NextPickCandidates` runs `LIMIT 500` with no `ORDER BY` over
a grade-ascending index, so the candidate pool is 100 % the densest grade in the window and
the scorer never sees a harder option. `(holdsetup=1, angle=40)` has 9 configs at `6B` but
29 441 at `6B+` — once you're at `6B+`, you're stuck.

## Starting Point

S-04 (`adaptive-main-session-loop`) shipped the full loop; its Phase 6 trajectory check
caught the plateau and parked plan row 6.3. `BenchmarkPickNext` exists but seeds only ~600
configs; the ~10 s first-pick guardrail has no benchmark.

## Desired End State

A run of easy sends climbs grade past the dense floor+1, up to the session max, on both
boards. Both perf guardrails have a realistic baseline. S-04 row 6.3 closes.

## Key Decisions Made

| Decision | Choice | Why | Source |
|---|---|---|---|
| Sampling strategy | CTE: `ORDER BY random() LIMIT 500` on `problem_configurations` alone, then PK-join `problem_hold_types` | `ORDER BY random()` on the *joined* query measured ~170ms/1s (planner Seq-Scans 259k pht rows); sampling the config table first keeps the PK nested-loop join → ~50ms, and gives a fresh whole-catalog random draw | Plan (revised after `EXPLAIN ANALYZE`) |
| Tier coverage | All tiers 0–3 (one shared query) | Uniform behavior; tier 3 also stops skewing to the ladder floor | Plan |
| Guardrail work | Realistic-seed `BenchmarkPickNext` + new `BenchmarkFirstPick` | Both NFR guardrails get a real regression baseline against the new query | Plan |
| e2e assertion | "not stuck" — max grade seen > min+1 | Robust to future weight tuning; still catches a re-regression of this bug | Plan |
| Scorer weights | Untouched | The query was the bug, not the weights | Plan |

## Scope

**In scope:** the `ORDER BY random()` line + doc comment; two reachability tests; a
realistic `BenchmarkPickNext` seed + `BenchmarkFirstPick`; an e2e "not stuck" test;
flipping S-04 row 6.3.

**Out of scope:** schema/index changes, `score.go` weights, the fallback-tier structure,
the AI-native trajectory judge (its own change), test-plan §3 status reconcile.

## Architecture / Approach

One production line changes: `ORDER BY random()` before `LIMIT` in
`catalog.NextPickCandidates`. All of `recommender.tieredPick`'s tiers 0–3 call it, so the
fix propagates everywhere at once. `LIMIT 500` keeps Postgres on a bounded top-N heapsort,
so the range scan stays index-backed and the cost stays single-digit ms.

## Phases at a Glance

| Phase | What it delivers | Key risk |
|---|---|---|
| 1. Ramp fix | `ORDER BY random()` + reachability tests | `EXPLAIN` shows a full sort instead of top-N heapsort |
| 2. Guardrail benchmarks | realistic `BenchmarkPickNext` + `BenchmarkFirstPick` | the bigger random sort pushes ns/op up (still expected far under budget) |
| 3. e2e + close-out | "not stuck" browser test; S-04 row 6.3 closed | ramp against the real catalog is slower than ~1 grade/send |

**Prerequisites:** S-04 implemented (bar row 6.3); Docker for integration tests; running
app + Supabase for e2e.
**Estimated effort:** ~1 session, 3 short phases.

## Open Risks & Assumptions

- A uniform random sample still over-represents a dense grade; the fix leans on the scorer
  to prefer the harder grade. Holds for today's catalog; a future 100×-denser grade could
  re-skew — documented, not mitigated.
- Manual `EXPLAIN` against the real DB is the check that the query plan is actually cheap.

## Success Criteria (Summary)

- Easy sends climb grade past `min+1` up to the session max, in the browser, on both boards.
- `BenchmarkPickNext` (realistic seed) and `BenchmarkFirstPick` both ms-scale.
- S-04 `plan.md` row 6.3 is `- [x]`.

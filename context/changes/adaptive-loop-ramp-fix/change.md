---
change_id: adaptive-loop-ramp-fix
title: Fix the adaptive loop's grade ramp and baseline the guardrails at catalog scale
status: implemented
created: 2026-09-02
updated: 2026-09-02
archived_at: null
---

## Notes

Follow-up to S-04 (`adaptive-main-session-loop`). Two coupled problems, both surfaced
by the S-04 Phase 6 trajectory check (`plan.md` row 6.3) and the `/10x-test-plan --status`
reconcile:

**A — the loop cannot ramp.** `catalog.NextPickCandidates` runs `LIMIT 500` with **no
`ORDER BY`** over `idx_problem_configurations_holdsetup_angle_grade` (holdsetup, angle,
grade). Postgres returns the range grade-ascending, so once the loop reaches a
high-population grade the 500-row pool is 100% that grade and the scorer can never reach
the top of its window. Confirmed against the live catalog: `(holdsetup=1, angle=40)` has
**9** configs at `6B` but **29441** at `6B+` — 5 consecutive RPE-2 sends plateau at
`6B` → `6B+` → stuck. This breaks FR-011's "engine ramps fast on low RPE + sent" and the
warmup-absorption rationale, and US-01's "a felt 3/10 + sent is allowed to step up".
- Fix must keep the ~3 s p95 next-pick guardrail. Options to weigh in planning: per-grade
  sampling (`row_number() OVER (PARTITION BY grade ORDER BY random())`), a bounded
  `ORDER BY random()` on the narrow tier-0/1 window only, raising/removing the cap for
  the (always narrow, ≤3-grade) tier-0/1 window, etc. Tier 2/3 widen downward and tier 3
  spans `[ladder[0], hi]` — the fix must not make those tiers expensive.
- Re-run the S-04 e2e trajectory (`tests/e2e/adaptive-session-loop.spec.ts`) with a
  stronger "grade climbs over N easy sends" assertion once fixed, and close S-04 plan
  row 6.3.

**B — guardrails never measured at catalog scale.** `BenchmarkPickNext` exists for the
next-pick path (~0.5 ms/op) but the ~10 s first-pick guardrail was never measured, and
neither was measured against full-board row counts after the ramp fix changes the query.
Add a first-pick benchmark and re-baseline both against realistic `(1,40)` / `(21,40)`
row counts. Non-blocking in CI (test-plan §5).

Maps to test-plan `context/foundation/test-plan.md` §2 Risk #1 and Risk #7, and §3
Phase 4 (perf guardrails half). The AI-native trajectory judge (§3 Phase 4's other half)
is a **separate** change — not this one.

Prerequisites: S-04 (`adaptive-main-session-loop`, implemented bar row 6.3).

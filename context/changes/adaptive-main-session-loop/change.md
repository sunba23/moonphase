---
change_id: adaptive-main-session-loop
title: Adaptive Main Session loop
status: implemented
created: 2026-09-02
updated: 2026-09-02

archived_at: null
---

## Notes

Roadmap ID: S-04 (north star). Outcome: a climber submits a per-problem RPE (1–10) plus a
completion status (sent / failed / bailed), and the next recommendation's difficulty AND
hold-type composition visibly reflect that result and the session so far, inside the ~3 s
guardrail; the climber can end the session at any point with the partial session saved
intact. PRD refs: FR-008, FR-009, FR-010, FR-012, US-01 (full Then clause + Acceptance
Criteria). Prerequisites: S-03 (start-session-first-problem, implemented).

Locked decisions (see `plan.md`):

1. `PickNext` = weighted scoring function (grade-fit + hold-type balance + variety +
   quality), argmax with rng tie-break. Pure `scoreNext` is table-driven, rng-stubbed.
2. "Never harder after hard/failed" is enforced by construction — the RPE→grade mapping
   computes an allowed grade window and the SQL filters `grade <= allowedMax AND grade <=
   session.max_grade`.
3. RPE→grade band (4-band): failed/bailed OR RPE≥8 → same-or-easier; RPE 5–7 & sent →
   same; RPE≤4 & sent → may step up one catalog grade. Ceiling: never above the
   snapshotted `sessions.max_grade`.
4. Hold-type signature: per-type counts over non-foot holds; dominant = plurality;
   running per-type session tally drives balance; untagged holds counted as "unknown",
   excluded from balance math.
5. Composition data precomputed into a `problem_hold_types` side table, backfilled by a
   migration and kept current by `catalog.HoldStore.ApplyTags` + the ingest path + a
   `catalog holds recompute-composition` CLI. The next-pick query joins it flat to
   protect the 3 s guardrail.
6. Empty-pool fallback, each tier logged: (1) keep grade window, drop the hold-type
   penalty; (2) widen the grade window downward; (3) allow an already-shown problem;
   (4) FirstPick-style min-grade random.
7. Result submit = one transaction: score the next pick before the tx, then UPDATE the
   current `session_problems` row with (rpe, completion, climbed_at) and INSERT the next
   row. Response is the new problem-card HTMX fragment.
8. Result UI: 3 large status buttons (Sent / Failed / Bailed) → reveal a 1–10 button
   grid → tapping a number submits; `hx-post` swaps the `#session-card` fragment in
   place. Always-visible End button.
9. End session: `status='ended'` + `ended_at`, `HX-Redirect` to `/`. No recap UI (S-05).

Schema delta: `0009` adds nullable `rpe` / `completion` / `climbed_at` to
`session_problems`; `0010` adds `problem_hold_types` + backfill. `sessions.status` gains
`'ended'` (Go-side constant, no CHECK).

Overlaps `test-plan.md` §3 Phases 1–2 — re-run `/10x-test-plan --status` after this lands.

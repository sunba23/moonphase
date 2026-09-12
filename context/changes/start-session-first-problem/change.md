---
change_id: start-session-first-problem
title: Start session, first problem
status: implemented
created: 2026-08-31
updated: 2026-08-31
archived_at: null
---

## Notes

Roadmap ID: S-03. Outcome: a signed-in, onboarded climber taps **Main Session** on the hub and
sees the first recommended problem — at the catalog's minimum grade for their board + angle —
with its name, grade, hold layout (circles overlaid on the board image), and per-hold type tags,
within the ~10 s guardrail. PRD refs: FR-005, FR-006, FR-007, FR-011; US-01 (Given/When).
Prerequisites: S-01 (signup-and-onboarding, impl_reviewed), F-02 (catalog-data-foundation,
impl_reviewed).

Stops before the adaptive loop — no RPE, no next-problem pick, no end-session (all S-04). Sets
the `sessions` / `session_problems` schema and the `internal/recommender` package contract that
S-04 builds on.

Locked decisions (see `plan.md` → "Decisions locked with the user"):
1. Persist `sessions` **and** `session_problems` now; first pick = `seq 0` pinned to a
   `problem_configuration_id`.
2. New `internal/recommender` package (`FirstPick` + fx `Module`); S-04 adds `PickNext`.
3. First pick = random among min-grade candidates, quality-filtered (`is_benchmark` OR
   `repeats >= threshold`), fallback to any min-grade problem.
4. Hold layout = circle markers overlaid on `static/moonboard/<year>.jpg` at calibrated
   `(col,row) → (%)` positions, role-coloured, plus a hold-by-hold tag list.
5. Restrict the board dropdown (onboarding + profile-edit) to app-ready boards (2016, 2024);
   a pre-existing profile on an unsupported board gets a "switch your board" page at session
   start.
6. One active session per user (partial unique index); Main Session resumes it.
7. Real `templ` hub page replacing the `indexHTML` stub.
8. Stand up the `testcontainers-go` Postgres harness now + integration tests for the session
   store and first-pick query. Overlaps `test-plan.md` §3 Phase 1
   (`testing-db-harness-and-recommender`) — re-run `/10x-test-plan --status` after this lands.

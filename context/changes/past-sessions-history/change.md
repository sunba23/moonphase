---
change_id: past-sessions-history
title: Past sessions history
status: implemented
created: 2026-09-02
updated: 2026-09-03
archived_at: null
---

## Notes

Roadmap ID: S-05 (last slice of Stream B). Outcome: a signed-in, onboarded climber opens
"Past sessions" from the hub, sees their finished sessions ordered most-recent-first, opens
one to see each climbed problem with its RPE and completion status, and taps a problem to
review its MoonBoard layout read-only. PRD refs: FR-013, FR-014. Prerequisites: S-04
(adaptive-main-session-loop, implemented) — history has nothing to show until a session has
been saved.

Read-only surface. **No schema change** — `sessions` (status / started_at / ended_at /
holdsetup / angle snapshot), `session_problems` (rpe / completion / climbed_at), and the
`sessions_user_started (user_id, started_at DESC)` index all already exist from S-03/S-04.

Locked decisions (see `plan.md`):

1. New `/sessions` (plural) namespace for the history surface; the live loop keeps
   `/session` (singular). Routes: `GET /sessions` (list), `GET /sessions/{sessionID}`
   (detail), `GET /sessions/{sessionID}/problem/{seq}` (read-only problem card).
2. List rows = started date + board/angle + "N problems climbed"; all from the `sessions`
   snapshot plus one `COUNT(sp.rpe)`. No pagination — every ended session, `started_at
   DESC`.
3. List and detail are `status = 'ended'` only. The active session is excluded (it is
   already reachable via the hub's Main Session button). `GET /sessions/{id}` for a
   non-ended or non-owned session returns an identical 404.
4. "Climbed" = `session_problems.rpe IS NOT NULL`. The trailing recommended-but-unrated
   row that every ended session carries is never shown.
5. Detail view = compact list per climbed problem: name, grade, RPE, completion status.
   Each row links to the read-only problem card.
6. Problem card is session-scoped: reuses the session ownership check, reuses
   `catalog.ProblemDetail` + the `moonBoardOverlay` / `holdLabel` templ helpers, and adds a
   "you climbed this — RPE 6 · sent" line. `seq` must exist for the session and be rated,
   else identical 404.
7. Two explicit empty states: "No sessions yet — start a Main Session" on the list;
   "No problems climbed in this session" in the detail view.
8. Navigation: a "Past sessions" link on the hub beside "Profile"; back-links detail →
   list and problem card → detail. Full-page `layout.Page` renders (no shared nav bar).

New code: `internal/session` store methods `ListForUser` / `ClimbedProblems` /
`ClimbedProblemAt`; `internal/server/history.go` (`historyPages`); `templates/pages/history.templ`
+ models; one hub link; `tests/e2e/past-sessions-history.spec.ts`.

Overlaps `test-plan.md` §3 — the history surface's browser path is net-new e2e. Re-run
`/10x-test-plan --status` after this lands.

# Past Sessions History — Plan Brief

> Full plan: `context/changes/past-sessions-history/plan.md`

## What & Why

Roadmap slice **S-05** (PRD FR-013, FR-014). A signed-in climber opens "Past sessions" from
the hub, sees their finished Main Sessions newest-first, opens one to see every climbed
problem with its RPE and completion status, and taps a problem to review its MoonBoard
layout read-only. History is the PRD's "second-most-valuable surface" after the live loop;
this slice makes the sessions S-04 saves actually visible.

## Starting Point

S-04 (`adaptive-main-session-loop`, implemented) already persists everything: `sessions`
carries `status` / `started_at` / `ended_at` and a board/angle/max-grade snapshot;
`session_problems` carries `rpe` / `completion` / `climbed_at`; there is even a
`sessions_user_started (user_id, started_at DESC)` index. The `session.Store` +
`sessionPages` handler + `session.templ` board-render components are all in place. After
**End session** a finished session is currently invisible in the UI — this slice is the
only thing missing.

## Desired End State

`GET /sessions` lists the caller's `status = 'ended'` sessions, each row showing the started
date, board + angle, and "N problems climbed". `GET /sessions/{id}` shows a compact list of
every climbed problem (name, grade, RPE, status) in climb order. `GET /sessions/{id}/problem/{seq}`
shows that problem's MoonBoard layout read-only with a "you climbed this — RPE 6 · sent"
line. Every route returns an identical 404 for a non-owner, a missing id, a still-active
session, or an unrated `seq`. Two empty states guide new users and quickly-ended sessions.

## Key Decisions Made

| Decision | Choice | Why | Source |
| --- | --- | --- | --- |
| Namespace | New `/sessions` (plural) routes; live loop keeps `/session` (singular) | The two surfaces never share semantics; history is strictly read-only | Plan |
| List row content | Date + board/angle + climbed count | All from the `sessions` snapshot + one `COUNT(sp.rpe)`; no heavy joins | Plan (user-chosen) |
| Detail view | Compact list: name, grade, RPE, status per climbed problem | Exactly FR-014; one lightweight query; scans well one-handed | Plan (user-chosen) |
| Problem card | Session-scoped `GET /sessions/{id}/problem/{seq}`, reuses the board render + ownership check, shows recorded RPE/status | No new privacy surface; contextual; reuses `moonBoardOverlay` / `holdLabel` | Plan (user-chosen) |
| Active session | Excluded — list + detail are `status = 'ended'` only; `/sessions/{id}` for a non-ended session → 404 | Literal match to "past"; the hub already resumes the active one | Plan (user-chosen) |
| "Climbed" | `rpe IS NOT NULL` everywhere | Every ended session carries a trailing recommended-but-unrated row; it must never show | Plan |
| Empty states | Two explicit: "No sessions yet" (list), "No problems climbed" (detail) | Both dead-ends guide the user forward | Plan (user-chosen) |
| Navigation | Hub link + back-links; full-page renders; no shared nav bar | Matches the existing hub/profile nav style; no shared nav component exists | Plan (user-chosen) |
| Pagination | None — all ended sessions, `started_at DESC` | Product targets single/low-double-digit users; the index already covers it | Plan (user-chosen) |

## Scope

**In scope:** `session.Store` methods `ListForUser` / `ClimbedProblems` / `ClimbedProblemAt`
+ integration tests; `internal/server/history.go` (`historyPages`) + 3 gated `GET` routes +
handler tests; `templates/pages/history.templ` (list, detail, read-only problem card, both
empty states) + models; one hub link; `tests/e2e/past-sessions-history.spec.ts`.

**Out of scope:** any schema change; pagination; session edit/delete (Non-Goal); recap /
duration / analytics (Non-Goals); "why this pick" copy (Non-Goal); showing the active
session in history; a catalog-wide problem route; a shared nav bar; any change to
`auth.Middleware`, `OnboardingGate`, the recommender, or the live session loop.

## Architecture / Approach

Three read-only `GET` handlers in a new `historyPages` struct (mirrors `sessionPages`),
mounted in the existing `auth.Middleware` + `OnboardingGate` group. The list handler's SQL
filters `user_id` + `status = 'ended'`; the detail and problem handlers re-check `Get`'s
`UserID` / `Status` and collapse every failure to `http.NotFound`, matching the existing
session handlers. Three new `session.Store` queries supply the data; the problem card also
calls the existing `catalog.ProblemDetail` and reuses `session.templ`'s `moonBoardOverlay`
/ `holdLabel` (same package — no refactor). One `<a>` added to `hub.templ`.

## Phases at a Glance

| Phase | What it delivers | Key risk |
| --- | --- | --- |
| 1. Store queries | `ListForUser`, `ClimbedProblems`, `ClimbedProblemAt` + integration tests | `COUNT(sp.rpe)` / `rpe IS NOT NULL` must exclude the trailing unrated row everywhere |
| 2. Handlers + routes | `history.go` 3 handlers, gated route wiring, handler tests | 404 discipline: non-owner / active / unrated-seq all identical; implemented with Phase 3 for a compilable build |
| 3. templ + hub link | `history.templ` (list, detail, problem card, 2 empty states), models, hub link, `templ generate` | Reusing the live card's parts without dragging in the result form / End button |
| 4. E2E | One Playwright spec: sign up → session → end → list → detail → problem card | Reusing the status→RPE interaction from the existing loop spec |

**Prerequisites:** S-04 implemented (it is). Docker + reachable Postgres/Supabase for the
integration and E2E suites.

**Estimated effort:** ~2 focused sessions across 4 phases. Phases 2 + 3 land together
(Go won't compile a handler referencing an undefined template).

## Open Risks & Assumptions

- Every ended session carries a trailing `rpe IS NULL` `session_problems` row (the
  recommender inserts the next problem after each result; End only flips status). The plan
  handles it with `rpe IS NOT NULL` filters — verified against `AdvanceSession` /
  `EndSession` in `internal/session/store.go`.
- Assumes the product stays at its stated scale, so no pagination is needed; revisit only if
  a heavy user reports a long list.
- Assumes `moonBoardOverlay` / `holdLabel` stay unexported in package `pages` (they are) so
  `history.templ` can call them directly.

## Success Criteria (Summary)

- A climber ends a Main Session, opens **Past sessions**, and sees that session with a
  correct climbed-problem count.
- Opening it shows every climbed problem with the exact RPE and completion status submitted,
  in climb order, with the trailing unclimbed problem absent.
- Opening a problem shows its MoonBoard layout read-only with the recorded result; every
  route 404s for a non-owner, a missing id, an active session, or an unrated seq.

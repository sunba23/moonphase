---
project: "MoonPhase"
version: 1
status: draft
created: 2026-06-13
context_type: greenfield
product_type: web-app
target_scale:
  users: small
  qps: low
  data_volume: small
timeline_budget:
  mvp_weeks: 4
  hard_deadline: null
  after_hours_only: true
---

# MoonPhase — Product Requirements Document

## Vision & Problem Statement

Self-coached intermediate-to-advanced MoonBoard climbers train without a real plan. They walk up to the board with the intent to "train hard", pick problems by feel, let ego dictate which ones they project, and over weeks or months accumulate overuse injuries — not from a single bad session, but from lopsided style coverage and ignored fatigue signals. Every session they face decision paralysis (hundreds of available problems, no framework for selection) compounded by a missing capability: no tool today turns a goal like "90 min volume" or "60 min anti-style" into an executable, adjusting session.

The insight is that real MoonBoard training is already adaptive — climbers reshape sessions on the fly based on warmup, fatigue, and how the last problem felt. Static planning apps and coach-written plans don't survive contact with the wall, and manual filtering assumes both the discipline and the vocabulary to self-correct mid-session. MoonPhase encodes the adjustment loop: it picks a starting problem, then re-ranks the next problem after each completion based on how the climber rated the previous one and what hold types the session has already loaded. The product is narrow on purpose — MoonBoard is a niche tool with a high entry bar, so the persona is small but high-intent and underserved.

## User & Persona

**Primary persona — Self-coached intermediate-to-advanced MoonBoard climber.** Trains on a MoonBoard 2–4 times a week. Has been climbing long enough to have hit at least one overuse niggle (finger, elbow, shoulder) or a visible plateau, and has connected that pain to "I just train whatever feels good". Knows the vocabulary of training types (volume, anti-style, project, technique) but lacks the structure to enforce them in the moment. No coach, no team — they pick their own sessions, and they're motivated to train smarter without paying for one.

The moment they reach for MoonPhase: standing in front of the board after warmup, deciding what to actually do for the next 30–120 minutes.

## Success Criteria

### Primary
- A self-coached climber completes a Main Session in MoonPhase, rates each climbed problem on an RPE-style scale, and the next-problem recommendations visibly respond to those ratings — when the last problem felt hard, the next is equal-or-easier; when it felt easy, the next can step up. The adaptive loop, not the catalog or the auth, is what proves MoonPhase works.

### Secondary
- After 4–8 weeks of regular use, the climber qualitatively reports more balanced style coverage across their MoonBoard sessions than before MoonPhase (i.e. the product nudges them off ego-driven single-style streaks).

### Guardrails
- First recommendation appears within ~10 s of the climber picking Main Session — no awkward wait at the wall.
- Next-problem recommendation appears within ~3 s of submitting an RPE rating — the loop has to feel responsive between attempts.
- The product is usable one-handed on a phone propped on the floor, with chalky/sweaty hands; tap targets and reading distance assume that context.
- Session data is private to the user — no public profiles, no leaderboards, no implicit sharing.
- The user's stated max grade and past ratings persist across devices and never get silently lost on logout/login.

## User Stories

### US-01: Climber completes one adaptive Main Session

- **Given** an authenticated climber with a stated max grade, standing in front of a MoonBoard after warmup
- **When** they tap "Main Session" on the hub and start climbing
- **Then** they receive a starting problem at the catalog's minimum grade for their selected board, climb it, submit an RPE + completion result, and see a next-problem recommendation whose difficulty AND hold-type composition visibly reflect the rating they just gave and the session so far — looping until they tap "End session"

#### Acceptance Criteria
- First recommendation appears within ~10 s of tapping "Main Session"
- Each next-problem recommendation appears within ~3 s of submitting a result
- A "felt 9/10" or "failed" result never produces a strictly harder next pick
- A "felt 3/10" + "sent" result is allowed to (but not required to) step up in grade
- Tapping "Skip" records no rating and returns another recommendation as if the skip had not happened — grade window and hold-type balance unchanged — except the skipped problem is not shown again in that session
- After several consecutive problems sharing a dominant hold type, the next recommendation favors a different hold-type composition
- Tapping "End session" mid-loop saves the partial session to history with the climbed problems intact

## Functional Requirements

### Authentication & profile
- FR-001: User can sign up with email + password. Priority: must-have
  > Socrates: Plumbing. Stands as written; no counter-argument worth recording.
- FR-002: User can sign in and sign out. Priority: must-have
  > Socrates: Plumbing. Stands as written.
- FR-003: User declares their current max boulder grade AND their MoonBoard set + angle during onboarding (single-select for each). Priority: must-have
  > Socrates: Expanded from grade-only after FR-016 went multi-set; onboarding now anchors both axes. Stands as revised.
- FR-004: User can edit their max grade and switch MoonBoard set/angle from a profile screen. Priority: must-have
  > Socrates: Plumbing for an infrequent operation. Stands as written.

### Session lifecycle (Main Session)
- FR-005: User can start a Main Session from the hub. Priority: must-have
  > Socrates: Plumbing. Stands as written.
- FR-006: User sees the first recommended problem within the session-start guardrail (~10 s). Priority: must-have
  > Socrates: Performance guardrail; stands as written.
- FR-007: User can view a recommended problem's catalog details (name, grade, hold layout, hold-type tags) before deciding to climb it. Priority: must-have
  > Socrates: Expanded to expose hold-type tags now that FR-012 uses them. Stands as revised.
- FR-008: For a problem the climber attempts, they submit a rated result: (a) an RPE rating on a 1–10 perceived-exertion scale and (b) a completion status (sent / failed). Alternatively they can "skip" a problem they choose not to attempt — a skip carries no RPE, is inert for the adaptive loop, and never appears in session history. Priority: must-have
  > Socrates: Counter-argument considered: "1–10 RPE is paradoxically more paralyzing than the original picking problem; 3 buttons would carry the same signal." Resolution: kept 1–10. User reasoning: at most ~15 problems per session, so per-problem reflection isn't a meaningful time tax, and the granularity matters for the hold-balance engine.
  > Update (2026-09-06, change `rename-bailed-to-skipped`): the third status "bailed" was renamed to "skipped" and made rating-free and loop-inert. A bail is not a graded attempt, so feeding it into the grade/hold-balance engine (as an equal-or-easier signal) was wrong; a skip now just re-picks (minimum grade if nothing has been rated yet), excluding the skipped problem for the rest of the session.
- FR-009: User sees the next recommended problem within the loop guardrail (~3 s) of submitting a result. Priority: must-have
  > Socrates: Performance guardrail; stands as written.
- FR-010: User can end the active session at any time via an explicit End-session action; partial sessions are saved as-is. Priority: must-have
  > Socrates: Plumbing. Stands as written.

### Adaptive recommendation
- FR-011: System recommends the first problem of every session at the minimum grade available on the user's chosen MoonBoard set/angle. Priority: must-have
  > Socrates: Counter-argument considered: "'below max' is too vague — pick a number." Resolution: REVISED. Every session starts at the catalog's minimum grade; the engine ramps fast on low RPE + sent results, naturally absorbing warmup into the loop. The user's max grade becomes a ceiling, not a starting anchor. This dodges the cold-start "is the stated max wrong?" problem and removes the need for a separate Warmup mode in the MVP.
- FR-012: System recommends each next problem with difficulty AND hold-type composition informed by the previous result and the session-so-far. High RPE or a "failed" attempt pushes the next pick to equal-or-easier grade and avoids stacking the same hold types; low RPE on a "sent" allows a grade step-up; the engine actively balances hold-type coverage across the session (e.g. avoids 4 crimp problems in a row). A "skipped" problem contributes nothing to either axis — it is neither an easier-grade signal nor a hold-type occurrence — but it is still not recommended again in the same session. Priority: must-have
  > Socrates: Counter-argument considered: "grade-only adaptation doesn't prevent style-driven overuse — climbing 4 crimpy problems at RPE 5 still cooks finger tendons." Resolution: REVISED. Engine now uses per-hold tags (crimp / sloper / pinch / jug / etc.) to balance hold-type coverage. Move tags (dyno, lock-off, drop-knee) remain out of scope — too hard to source/tag reliably.

### Past sessions (read-only)
- FR-013: User can view a list of their past sessions from the hub, ordered most-recent-first. Priority: must-have
  > Socrates: Plumbing for the secondary surface. Stands as written.
- FR-014: User can open a past session and see each climbed problem with its RPE and completion status. Priority: must-have
  > Socrates: Plumbing. Stands as written.

### Problem catalog
- FR-015: System exposes a catalog of MoonBoard problems covering all supported set/angle combinations (e.g. 2016, 2017, 2019, 2024 boards × 25°, 40°), sourced from a single unified data source. Catalog is read-only to users. Per-hold tags (hold types) are part of the catalog data. Priority: must-have
  > Socrates: Counter-argument considered: "single set/angle locks out a meaningful chunk of the persona." Resolution: REVISED. All sets/angles supported in MVP — they share a unified data format from the same source, so it's one integration not many.

> Note: a previously drafted FR — "system refines the user's stored max grade at the end of each session" — was DROPPED during the Socratic round because single-session refinement is too noisy (one bad day permanently moves the max). Max grade is user-controlled only via FR-004.

## Non-Functional Requirements

- The first problem recommendation of a session appears within 10 s p95 of the user choosing Main Session.
- Each next-problem recommendation appears within 3 s p95 of the user submitting a per-problem result.
- The product remains usable on the latest two major versions of the four mainstream mobile and desktop browsers.
- The product is operable one-handed on a phone in portrait orientation; primary tap targets remain reachable with the thumb during an active session.
- A user's session history, RPE ratings, and stated max grade are visible only to that user — never to other accounts, never on a public surface, never in aggregated/anonymized form.
- A user's stated max grade, board selection, and past session data persist across sign-out / sign-in and across devices without manual re-entry.

## Business Logic

Given a climber's stated max grade, their chosen MoonBoard set/angle, and their per-problem RPE + completion history within the current session, MoonPhase picks the next problem to recommend by jointly optimizing for grade progression (start at the catalog's minimum grade, ramp on low-RPE sends, hold or back off on hard or failed attempts, never recommend above the user's stated max) and hold-type balance (avoid stacking the same hold types across consecutive problems within a session).

The rule's inputs, in user-facing terms: the user's onboarding-declared max grade, the board they selected, and a running record of how the current session has gone — for each problem the user attempted, an RPE value (1–10) and a completion status (sent / failed). Problems the user skipped carry no RPE and are excluded from the rule's inputs entirely; they only stay on the "don't show again this session" list. Its output is a single next problem to recommend, drawn from the catalog of the user's chosen board.

The rule has two axes and both are load-bearing. Grade progression alone produces a generic difficulty-sliding app; hold-type balance alone produces a workout shuffler. Together they encode MoonPhase's specific claim — that the right next problem after a hard crimp send is *not* the next-grade-up crimp problem, even though that's exactly what the climber's ego would pick.

The user encounters the rule continuously: every time they submit a result, the next recommendation is the rule's output. The rule never explains itself in prose; a terse, non-prose rationale tag (e.g. "Holding · off crimp") appears only in the collapsed Session-balance panel, never in the primary session view — the climber experiences the feed as well-chosen problems that feels less ego-driven than what they'd pick on their own.

## Access Control

Authenticated, flat user model. Every climber signs in with their own credentials and gets the same capabilities — start sessions, run sessions, view their own history, edit their own profile. There are no admin, coach, or moderator roles in the MVP. A user only sees their own data; nothing is shared between accounts.

Sign-in is required because session history is the second-most-valuable surface (after the live session) and climbers train across devices (phone at the board, browser at home). A local-only model would lose the cross-device story; a passwordless-link model adds friction at the wall when sessions need to start fast.

## Non-Goals

- **No MoonBoard hardware integration** — no LED control, no Bluetooth pairing with the official board. The climber reads holds off MoonPhase and sets them up by hand.
- **No integration with the official MoonBoard app** — MoonPhase reads from the unified problem-data source only; it does not embed, scrape live, or sync the official app.
- **No user-submitted problems and no in-app catalog editing** — the catalog is read-only and externally sourced. Users cannot create, edit, or curate problems.
- **No social, sharing, or comparison features** — no friend lists, no leaderboards, no shared sessions, no public profiles. Sessions are private by design.
- **No Warmup or Winddown modes in MVP** — only Main Session. Warmup is naturally absorbed into the adaptive loop (every session starts at minimum grade and ramps).
- **No editing or deleting of past sessions** — past-sessions surface is strictly read-only in MVP.
- **No ML or learned models for recommendation** — the engine is rule-based with explicit, inspectable logic.
- **No analytics dashboards or trend charts** — no weekly volume, style-coverage-over-time, or progress visualizations. The product surfaces the live session and a flat history list, nothing more.
- **No prose explanation of recommendations** — the primary session view stays just the board; MVP surfaces no sentence-level "we picked this because…" copy. A terse, non-prose rationale tag (grade direction + hold-type shift) is allowed inside the collapsed "Session balance" panel only.
- **No coach-side or multi-user-management surface** — flat single-user role only; no athletes, no teams, no admins.
- **No offline-first guarantee** — active session requires connectivity; partial-session-survives-network-loss is not a v1 commitment.
- **No move-type tagging** — only hold-type tags are used by the engine. Move classifications (dyno, lock-off, drop-knee, etc.) are out of scope because they cannot be sourced/tagged reliably for MVP.

## Open Questions

All three questions below were resolved on 2026-07-26 during roadmap sequencing (see `context/foundation/roadmap.md`).

1. **Source of the unified MoonBoard problem catalog** — RESOLVED (2026-08-03, re-confirmed). Decision: the fallback chain from the original decision (`CSTDev/moonapi` → `spookykat/MoonBoard` → fork → static `.zip` export) was exercised by evaluating the two API candidates without integrating either: `CSTDev/moonapi` is 7 years stale and scrapes a legacy website login flow likely broken against MoonBoard's current backend; `spookykat/MoonBoard` is more current but still depends on reverse-engineered auth against personal MoonBoard credentials. The user obtained official static `.zip` exports directly instead — landing on the chain's own documented final fallback tier, not a deviation from it. Owner: user. Status: fully resolved — static exports for all 4 board editions (2016, 2024, Masters 2017, Masters 2019; 259,761 problems total) are the catalog source; see `context/changes/catalog-data-foundation/`.
2. **Hold-type taxonomy** — RESOLVED. Decision: none of the candidate datasets carry per-hold type detail, so hold types are indexed manually by the user for the catalog's scope. Owner: user. Status: resolved — manual tagging accepted; concrete taxonomy fixed at `primary_type` ∈ {crimp, sloper, pinch, jug, pocket} plus up to ~3 free-form modifiers, with full coverage across all 4 boards required before `catalog-data-foundation` is considered done (see `context/changes/catalog-data-foundation/`). FR-012's hold-balance axis depends on this tagging.
3. **Initial v1 user count and onboarding plan** — RESOLVED. Decision: v1 targets a wider beta, not just the author or a small friend group. Owner: user. Status: resolved — no new FR required (FR-001 self-signup already covers this); invite/waitlist-style onboarding beyond signup remains out of scope unless a future FR is added.

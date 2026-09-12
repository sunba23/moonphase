---
project: "MoonPhase"
context_type: greenfield
created: 2026-06-13
updated: 2026-06-13
checkpoint:
  current_phase: 8
  phases_completed: [1, 2, 3, 4, 5, 6, 7]
  gray_areas_resolved:
    - topic: "Primary persona scope"
      decision: "Self-coached intermediate-to-advanced MoonBoard climber who has already felt the injury/plateau cost of unstructured training."
    - topic: "Pain category"
      decision: "Decision paralysis (too many problems, no framework) compounded by missing capability (no tool turns a session goal into a structured, adjusting plan)."
    - topic: "Product shape (insight pivot)"
      decision: "Adaptive coach, not static planner — the differentiator is in-session adjustment after each problem, not just upfront generation."
    - topic: "Auth model"
      decision: "Email + password (or OAuth), flat single-user role; cross-device sync required because climbers train at gym + plan at home."
    - topic: "MVP scope"
      decision: "Main Session mode only (no warmup/winddown); read-only past-sessions history (no modify/delete); single MoonBoard set/angle for the catalog (e.g. 2019 40°); no system-side end-of-session prompts. Target ~3 weeks of after-hours work."
  frs_drafted: 15
  quality_check_status: accepted
---

# Shape Notes

Seed idea (from `idea-notes.md`): a MoonBoard training assistant that generates structured session plans (e.g. "90 min volume", "60 min anti-style", "30 min regular", "120 min project"), runs the user through the picked problems, and tracks completion (done / skipped / failed).

Explicit non-MVP per seed: no MoonBoard hardware/LED integration, no social features, no ML models, no analytics dashboards.

Stated success bars per seed: full session generated in < 10 s; user completes a session without ever opening the MoonBoard problem list manually.

## Vision & Problem Statement

Self-coached intermediate-to-advanced MoonBoard climbers train without a real plan. They walk up to the board with the intent to "train hard", pick problems by feel, let ego dictate which ones they project, and over weeks or months accumulate overuse injuries — not from a single bad session, but from lopsided style coverage and ignored fatigue signals. Every session they face decision paralysis (hundreds of available problems, no framework for selection) compounded by a missing capability: no tool today turns a goal like "90 min volume" or "60 min anti-style" into an executable, adjusting session.

The insight is that real MoonBoard training is already adaptive — climbers reshape sessions on the fly based on warmup, fatigue, and how the last problem felt. Static planning apps and coach-written plans don't survive contact with the wall, and manual filtering assumes both the discipline and the vocabulary to self-correct mid-session. MoonPhase encodes the adjustment loop: it picks a starting set against a stated goal, then re-ranks the next problem after each completion based on how the climber rated the previous one. The product is narrow on purpose — MoonBoard is a niche tool with a high entry bar, so the persona is small but high-intent and underserved.

## User & Persona

**Primary persona — Self-coached intermediate-to-advanced MoonBoard climber.** Trains on a MoonBoard 2–4 times a week. Has been climbing long enough to have hit at least one overuse niggle (finger, elbow, shoulder) or a visible plateau, and has connected that pain to "I just train whatever feels good". Knows the vocabulary of training types (volume, anti-style, project, technique) but lacks the structure to enforce them in the moment. No coach, no team — they pick their own sessions, and they're motivated to train smarter without paying for one.

The moment they reach for MoonPhase: standing in front of the board after warmup, deciding what to actually do for the next 30–120 minutes.

## Access Control

Authenticated, flat user model. Every climber signs in with email + password (or OAuth) and gets the same capabilities — generate sessions, run sessions, view their own history. There are no admin, coach, or moderator roles in the MVP. A user only sees their own data; nothing is shared between accounts.

Sign-in is required because session history is the second-most-valuable surface (after the live session) and climbers train across devices (phone in the gym, browser at home). A local-only model would lose the cross-device story; a magic-link model adds friction at the wall when sessions need to start fast.

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

## Timeline budget

Target: ~3 weeks of after-hours work for the scoped-down MVP (Main Session only, read-only history, single MoonBoard set/angle for the catalog, no warmup/winddown, no system-side end-prompts).

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
- FR-008: User submits a per-problem result consisting of (a) an RPE rating on a 1–10 perceived-exertion scale and (b) a completion status (sent / failed / bailed). Priority: must-have
  > Socrates: Counter-argument considered: "1–10 RPE is paradoxically more paralyzing than the original picking problem; 3 buttons would carry the same signal." Resolution: kept 1–10. User reasoning: at most ~15 problems per session, so per-problem reflection isn't a meaningful time tax, and the granularity matters for the hold-balance engine.
  > Update (2026-09-06, change `rename-bailed-to-skipped`): "bailed" renamed to "skipped" — it now carries no RPE and is fully inert for the recommender and history. A skip just re-picks (minimum grade if nothing has been rated yet), excluding the skipped problem for the rest of the session. See `context/foundation/prd.md` FR-008/FR-012 for the authoritative wording.
- FR-009: User sees the next recommended problem within the loop guardrail (~3 s) of submitting a result. Priority: must-have
  > Socrates: Performance guardrail; stands as written.
- FR-010: User can end the active session at any time via an explicit End-session action; partial sessions are saved as-is. Priority: must-have
  > Socrates: Plumbing. Stands as written.

### Adaptive recommendation
- FR-011: System recommends the first problem of every session at the minimum grade available on the user's chosen MoonBoard set/angle. Priority: must-have
  > Socrates: Counter-argument considered: "'below max' is too vague — pick a number." Resolution: REVISED. Every session starts at the catalog's minimum grade; the engine ramps fast on low RPE + sent results, naturally absorbing warmup into the loop. The user's max grade becomes a ceiling, not a starting anchor. This dodges the cold-start "is the stated max wrong?" problem and removes the need for a separate Warmup mode in the MVP.
- FR-012: System recommends each next problem with difficulty AND hold-type composition informed by the previous result and the session-so-far. High RPE or "failed/bailed" pushes the next pick to equal-or-easier grade and avoids stacking the same hold types; low RPE on a "sent" allows a grade step-up; the engine actively balances hold-type coverage across the session (e.g. avoids 4 crimp problems in a row). Priority: must-have
  > Socrates: Counter-argument considered: "grade-only adaptation doesn't prevent style-driven overuse — climbing 4 crimpy problems at RPE 5 still cooks finger tendons." Resolution: REVISED. Engine now uses per-hold tags (crimp / sloper / pinch / jug / etc.) to balance hold-type coverage. Move tags (dyno, lock-off, drop-knee) remain out of scope — too hard to source/tag reliably.
- FR-013: ~~System refines the user's stored max grade at the end of each session~~ DROPPED. Priority: n/a
  > Socrates: Counter-argument considered: "single-session refinement is noisy — one bad day permanently moves the user's max." Resolution: DROPPED. Max grade is user-controlled only; the climber edits it from profile (FR-004) when they feel it has shifted.

### Past sessions (read-only)
- FR-014: User can view a list of their past sessions from the hub, ordered most-recent-first. Priority: must-have
  > Socrates: Plumbing for the secondary surface. Stands as written.
- FR-015: User can open a past session and see each climbed problem with its RPE and completion status. Priority: must-have
  > Socrates: Plumbing. Stands as written.

### Problem catalog
- FR-016: System exposes a catalog of MoonBoard problems covering all supported set/angle combinations (e.g. 2016, 2017, 2019, 2024 boards × 25°, 40°), sourced from a single unified data source. Catalog is read-only to users. Per-hold tags (hold types) are part of the catalog data. Priority: must-have
  > Socrates: Counter-argument considered: "single set/angle locks out a meaningful chunk of the persona." Resolution: REVISED. All sets/angles supported in MVP — they share a unified data format from the same source, so it's one integration not many.

## Business Logic

Given a climber's stated max grade, their chosen MoonBoard set/angle, and their per-problem RPE + completion history within the current session, MoonPhase picks the next problem to recommend by jointly optimizing for grade progression (start at the catalog's minimum grade, ramp on low-RPE sends, hold or back off on hard or failed attempts, never recommend above the user's stated max) and hold-type balance (avoid stacking the same hold types across consecutive problems within a session).

The rule's inputs, in user-facing terms: the user's onboarding-declared max grade, the board they selected, and a running record of how the current session has gone — for each problem the user attempted, an RPE value (1–10) and a completion status (sent / failed / bailed). Its output is a single next problem to recommend, drawn from the catalog of the user's chosen board.

The rule has two axes and both are load-bearing. Grade progression alone produces a generic difficulty-sliding app; hold-type balance alone produces a workout shuffler. Together they encode MoonPhase's specific claim — that the right next problem after a hard crimp send is *not* the next-grade-up crimp problem, even though that's exactly what the climber's ego would pick.

The user encounters the rule continuously: every time they submit a result, the next recommendation is the rule's output. The rule never explains itself in the UI (no "we picked this because…" copy in MVP) — the climber experiences it as a feed of well-chosen problems that feels less ego-driven than what they'd pick on their own.

## Non-Functional Requirements

- The first problem recommendation of a session appears within 10 s p95 of the user choosing Main Session.
- Each next-problem recommendation appears within 3 s p95 of the user submitting a per-problem result.
- The product remains usable on the latest two major versions of the four mainstream mobile and desktop browsers.
- The product is operable one-handed on a phone in portrait orientation; primary tap targets remain reachable with the thumb during an active session.
- A user's session history, RPE ratings, and stated max grade are visible only to that user — never to other accounts, never on a public surface, never in aggregated/anonymized form.
- A user's stated max grade, board selection, and past session data persist across sign-out / sign-in and across devices without manual re-entry.

## Product framing

- Product type: web app
- Target scale (users): small (single-digit through low double-digits in v1)
- Timeline budget: ~3–4 weeks of after-hours work for the scoped MVP. The original 3-week target tightens once the multi-set catalog (FR-016) and hold-type-balanced engine (FR-012) are added back in Phase 4.5; honest re-estimate is ~4 weeks. Hard deadline: none. After-hours only: yes.
- Scale forward-look (informational): at ~100× user scale, the engine's structure doesn't need to change, but hold-type tagging would need to be richer/more reliable to keep recommendations from feeling samey across many users sharing one catalog.

## Non-Goals

- **No MoonBoard hardware integration** — no LED control, no Bluetooth pairing with the official board. The climber reads holds off MoonPhase and sets them up by hand.
- **No integration with the official MoonBoard app** — MoonPhase reads from the unified problem-data source only; it does not embed, scrape live, or sync the official app.
- **No user-submitted problems and no in-app catalog editing** — the catalog is read-only and externally sourced. Users cannot create, edit, or curate problems.
- **No social, sharing, or comparison features** — no friend lists, no leaderboards, no shared sessions, no public profiles. Sessions are private by design.
- **No Warmup or Winddown modes in MVP** — only Main Session. Warmup is naturally absorbed into the adaptive loop (every session starts at minimum grade and ramps).
- **No editing or deleting of past sessions** — past-sessions surface is strictly read-only in MVP.
- **No ML or learned models for recommendation** — the engine is rule-based with explicit, inspectable logic. ML is explicitly out of scope per the seed.
- **No analytics dashboards or trend charts** — no weekly volume, style-coverage-over-time, or progress visualizations. The product surfaces the live session and a flat history list, nothing more.
- **No in-UI explanation of recommendations** — MVP does not surface "we picked this because…" copy. The climber experiences the engine as well-chosen problems.
- **No coach-side or multi-user-management surface** — flat single-user role only; no athletes, no teams, no admins.
- **No offline-first guarantee** — active session requires connectivity; partial-session-survives-network-loss is not a v1 commitment.

## Quality cross-check

All five greenfield gates pass. No gaps to surface to `/10x-prd` as Open Questions.

- Access Control: present
- Business Logic: present (one-sentence rule + supporting paragraphs; not empty CRUD)
- Project artifacts: present
- Timeline-cost acknowledgment: present (~4 weeks honest re-estimate captured)
- Non-Goals: present (11 entries)

## User Stories

### US-01: Climber completes one adaptive Main Session

- **Given** an authenticated climber with a stated max grade, standing in front of a MoonBoard after warmup
- **When** they tap "Main Session" on the hub and start climbing
- **Then** they receive a starting problem below their max, climb it, submit an RPE + completion result, and see a next-problem recommendation whose difficulty visibly reflects the rating they just gave — looping until they tap "End session"

#### Acceptance Criteria
- First recommendation appears within ~10 s of tapping "Main Session"
- Each next-problem recommendation appears within ~3 s of submitting a result
- A "felt 9/10" or "failed" result never produces a strictly harder next pick
- A "felt 3/10" + "sent" result is allowed to (but not required to) step up in grade
- Tapping "End session" mid-loop saves the partial session to history with the climbed problems intact

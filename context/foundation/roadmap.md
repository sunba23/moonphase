---
project: "MoonPhase"
version: 1
status: draft
created: 2026-07-26
updated: 2026-09-03
prd_version: 1
main_goal: market-feedback
top_blocker: none
---

# Roadmap: MoonPhase

> Derived from `context/foundation/prd.md` (v1) + auto-researched codebase baseline.
> Edit-in-place; archive when superseded.
> Slices below are listed in dependency order. The "At a glance" table is the index.

## Vision recap

Self-coached MoonBoard climbers pick problems by feel and end up with lopsided training and, over time, overuse injuries. MoonPhase's bet is that a tool which picks the first problem and then re-ranks the next one after every rated attempt — reacting both to how hard it felt and to which hold types the session has already loaded — can replace that ego-driven picking with something that actually protects the climber and improves training balance.

## North star

**S-04: User can complete one full adaptive Main Session** — the PRD's own Success Criteria section states this directly: a climber rates each problem, and the next recommendation visibly changes based on that rating, both in difficulty and in which hold types it favors. Not the auth, not the catalog — this loop is what has to work for MoonPhase to be worth building at all.

> "North star" here means the smallest end-to-end slice that, if it works, proves the whole product's premise — it's placed as early in the sequence as its prerequisites allow, because none of the other slices matter if this one doesn't hold up.

## At a glance

| ID   | Change ID                    | Outcome (user can …)                                              | Prerequisites | PRD refs                          | Status   |
| ---- | ----------------------------- | ------------------------------------------------------------------- | -------------- | ---------------------------------- | -------- |
| F-01 | auth-session-scaffold          | (foundation) authenticated requests resolve to a user identity      | —              | FR-001, FR-002                     | done    |
| F-02 | catalog-data-foundation        | (foundation) minimal catalog + hold-type tags for one board/angle   | —              | FR-015, FR-007                     | done    |
| S-01 | signup-and-onboarding          | sign up, sign in, declare max grade + board/angle                   | F-01           | FR-001, FR-002, FR-003, US-01      | done |
| S-02 | edit-profile                   | edit max grade / switch board/angle from profile                    | S-01           | FR-004                             | done |
| S-03 | start-session-first-problem    | start a session and see a real, catalog-backed starting problem     | S-01, F-02     | FR-005, FR-006, FR-007, FR-011, US-01 | done |
| S-04 | adaptive-main-session-loop     | complete one full adaptive Main Session (north star)                 | S-03           | FR-008, FR-009, FR-010, FR-012, US-01 | done |
| S-05 | past-sessions-history           | view and open past sessions                                          | S-04           | FR-013, FR-014                     | done |

## Streams

Navigation aid — groups items that share a Prerequisites chain. Canonical ordering still lives in the dependency graph below; this table is the proposed reading order across parallel tracks.

| Stream | Theme                | Chain                                       | Note                                                                                                   |
| ------ | -------------------- | -------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| A      | Auth & profile        | `F-01` → `S-01` → `S-02`                     | Runs in parallel with Stream B from day one; gets an identity in place so Stream B has someone to scope data to. |
| B      | Catalog & adaptive loop | `F-02` → `S-03` (joins Stream A at `S-01`) → `S-04` → `S-05` | Carried the roadmap's decision risk (catalog source + hold tagging) — now resolved (`CSTDev/moonapi` chosen, tagging done manually) — but still sequenced early since it's the north star's track. |

## Baseline

What's already in place in the codebase as of `2026-07-26` (auto-researched + user-confirmed).
Foundations below assume these are present and do NOT re-scaffold them.

- **Frontend:** absent — `tech-stack.md` chose templ + HTMX, but no `.templ` files or `templates/` directory exist yet; the only HTML today is a placeholder string in `internal/server/server.go`.
- **Backend / API:** partial — a chi router + fx lifecycle scaffold is live and deployed (`internal/server/server.go`, `cmd/server/main.go`), but routes are limited to `/healthz` and `/`; no feature endpoints exist yet.
- **Data:** absent — `DATABASE_URL` to Supabase is wired and verified reachable (a `SELECT 1` was confirmed at deploy time), but there is no pgx pool in code, no `migrations/` directory, and no schema.
- **Auth:** absent — no auth code exists in the backend; `SUPABASE_PUBLISHABLE_KEY` / `SUPABASE_SECRET_KEY` are provisioned in `.env` and Railway variables but are not yet used by the application.
- **Deploy / infra:** present — live on Railway at `https://moonphase-production-7370.up.railway.app`, `Dockerfile` committed as an escape hatch, GitHub → Railway auto-deploy wired, `/healthz` verified returning 200.
- **Observability:** absent — only fx's default startup/lifecycle logging exists; no app-level structured logging, error tracking, or metrics.

## Foundations

### F-01: Auth session scaffold

- **Outcome:** (foundation) a request carrying a valid Supabase session/token resolves to a specific authenticated user id inside the Go backend — no signup/login UI exists yet, just the identity plumbing.
- **Change ID:** auth-session-scaffold
- **PRD refs:** FR-001, FR-002, Access Control
- **Unlocks:** S-01, S-02, S-03, S-04, S-05 — every downstream slice needs to know "which user is asking" so data stays scoped per the Access Control section's private, per-user data model.
- **Prerequisites:** — (external state: Supabase project + `SUPABASE_PUBLISHABLE_KEY`/`SUPABASE_SECRET_KEY` already provisioned and confirmed reachable)
- **Parallel with:** F-02
- **Blockers:** —
- **Unknowns:** —
- **Risk:** sequenced first because every other slice needs a way to identify the current user; low risk given Supabase Auth is a mature managed service and the keys are already live and verified.
- **Status:** done

### F-02: Catalog data foundation

- **Outcome:** (foundation) a queryable MoonBoard problem catalog exists in Postgres, ingested from static exports covering all 4 board editions (2016, 2024, Masters 2017, Masters 2019 — 259,761 problems total), with hold-type tags manually indexed for every physical hold across all 4 boards — each hold described by a primary type plus up to ~3 modifiers. Nothing recommends from it yet.
- **Change ID:** catalog-data-foundation
- **PRD refs:** FR-015, FR-007
- **Unlocks:** S-03 (first catalog-backed problem view), S-04 (the north star — needs hold-type tags for its balance axis).
- **Prerequisites:** —
- **Parallel with:** F-01
- **Blockers:** —
- **Unknowns:**
  - Catalog source — RE-RESOLVED (2026-08-03): `CSTDev/moonapi` was found to be 7 years stale, scraping a legacy MoonBoard website login flow likely broken against the site's current OAuth-based backend; its documented fallback `spookykat/MoonBoard` is more current but still depends on reverse-engineered auth against personal MoonBoard credentials. The user instead obtained official static JSON exports for all 4 board editions directly — this is the PRD's own pre-approved fallback tier (`prd.md` Open Question 1's static-`.zip`-export contingency), reached by skipping the two scrapers rather than deviating from the PRD. Reasoning: MoonBoard hasn't shipped a new hold setup in 2 years, so the problem catalog is mature and stable — a static export is a legitimate long-term source, not just an MVP shortcut. Scope widened accordingly: all 4 boards are ingested now (both angle configs each) instead of a single board/angle, since the marginal ingestion cost across boards is ~zero once the schema and CLI exist.
  - Hold-type tagging effort — RESOLVED: user will tag holds manually; taxonomy fixed at `primary_type` ∈ {crimp, sloper, pinch, jug, pocket} plus up to ~3 free-form modifiers. Full coverage across all 4 boards is a required completion gate for this slice (not deferred), made tractable by auto-generating a per-board fillable CSV inventory from the holds actually referenced by ingested problems.
- **Risk:** catalog-source risk is fully resolved (no live scraper dependency at all). Remaining risk was ordinary ingestion-engineering risk (verifying real post-filter row counts, one-shot ingestion throughput over ~260K rows) plus the real time cost of hand-tagging every physical hold across all 4 boards.
- **Status:** done

## Slices

### S-01: User can sign up, sign in, and complete onboarding

- **Outcome:** user can create an account, sign in, and declare their max boulder grade and MoonBoard set/angle during onboarding.
- **Change ID:** signup-and-onboarding
- **PRD refs:** FR-001, FR-002, FR-003, US-01 (Given clause: "an authenticated climber with a stated max grade")
- **Prerequisites:** F-01
- **Parallel with:** F-02
- **Blockers:** —
- **Unknowns:** —
- **Risk:** a standard auth + profile flow; sequenced right after F-01 since every later slice's starting state assumes an authenticated, onboarded user already exists.
- **Status:** done

### S-02: User can edit their profile

- **Outcome:** user can edit their max grade and switch MoonBoard set/angle from a profile screen.
- **Change ID:** edit-profile
- **PRD refs:** FR-004
- **Prerequisites:** S-01
- **Parallel with:** S-03
- **Blockers:** —
- **Unknowns:** —
- **Risk:** small, infrequent-operation slice; safe to build alongside or after the catalog/loop work since nothing else depends on it.
- **Status:** done

### S-03: User can start a session and see a real starting problem

- **Outcome:** user can start a Main Session from the hub and see the first recommended problem — at the catalog's minimum grade for their board — with its name, grade, hold layout, and hold-type tags, within the ~10s guardrail.
- **Change ID:** start-session-first-problem
- **PRD refs:** FR-005, FR-006, FR-007, FR-011, US-01 (Given/When clauses)
- **Prerequisites:** S-01, F-02
- **Parallel with:** S-02
- **Blockers:** —
- **Unknowns:** — (the catalog-source question this used to inherit from F-02 is resolved — see F-02)
- **Risk:** this is the first slice that proves the catalog integration end-to-end. The PRD explicitly calls the catalog secondary to the adaptive loop, so it's sequenced as its own step before the north star rather than folded into it — this keeps the loop's own risk isolated from the catalog's risk.
- **Status:** done

### S-04: User can complete one full adaptive Main Session

- **Outcome:** user submits a per-problem RPE (1–10) + completion status (sent / failed / bailed), and the next recommendation's difficulty AND hold-type composition visibly reflect that result and the session so far, within the ~3s guardrail; the user can end the session at any point with the partial session saved intact.
- **Change ID:** adaptive-main-session-loop
- **PRD refs:** FR-008, FR-009, FR-010, FR-012, US-01 (full Then clause + Acceptance Criteria)
- **Prerequisites:** S-03
- **Parallel with:** S-02
- **Blockers:** —
- **Unknowns:** — (the hold-tagging question this used to inherit from F-02 is resolved — see F-02)
- **Risk:** this is literally what the PRD's Success Criteria says proves the product works — sequenced as early as its prerequisites allow rather than deferred for symmetry with other work, since everything else only matters if this loop holds up.
- **Status:** done

### S-05: User can view and open past sessions

- **Outcome:** user can view a list of their past sessions ordered most-recent-first and open one to see each climbed problem with its RPE and completion status.
- **Change ID:** past-sessions-history
- **PRD refs:** FR-013, FR-014
- **Prerequisites:** S-04
- **Parallel with:** S-02
- **Blockers:** —
- **Unknowns:** — (no session history exists to browse until S-04 has run and saved at least one session, but that's an ordinary Prerequisite, not an open decision)
- **Risk:** a read-only surface with no independent risk of its own; safe to build last since it only consumes data S-04 already produces.
- **Status:** done

## Backlog Handoff

| Roadmap ID | Change ID                 | Suggested issue title                                              | Ready for `/10x-plan` | Notes                                  |
| ---------- | -------------------------- | --------------------------------------------------------------------- | ---------------------- | ---------------------------------------- |
| F-01       | auth-session-scaffold       | Wire Supabase Auth session verification into the backend               | done                     | Shipped                                       |
| F-02       | catalog-data-foundation     | Ingest MoonBoard catalog + hold-type tags for the first board/angle    | done                     | Shipped                                        |
| S-01       | signup-and-onboarding       | Sign up, sign in, onboarding (max grade + board/angle)                 | done                      | Shipped                         |
| S-02       | edit-profile                | Edit max grade / board/angle from profile screen                       | done                      | Shipped                         |
| S-03       | start-session-first-problem | Start session, see first catalog-backed problem                        | done                      | Shipped                  |
| S-04       | adaptive-main-session-loop  | Full adaptive Main Session loop (north star)                           | done                      | Shipped                         |
| S-05       | past-sessions-history       | View and open past sessions                                            | done                      | Shipped                         |

## Open Roadmap Questions

1. ~~Which catalog source will actually be used — `CSTDev/moonapi`, `spookykat/MoonBoard`, a fork of either, or a static `.zip` export?~~ **RESOLVED (2026-07-26):** `CSTDev/moonapi`. Fallback chain (`spookykat/MoonBoard` → fork → static `.zip`) kept as documented contingency.
2. ~~How much manual hold-type tagging is realistically achievable for the first board/angle, given after-hours-only availability?~~ **RESOLVED (2026-07-26):** manual tagging, judged to be little work; up to ~4 properties per hold.

## Parked

- **No MoonBoard hardware integration** — Why parked: PRD §Non-Goals.
- **No integration with the official MoonBoard app** — Why parked: PRD §Non-Goals.
- **No user-submitted problems and no in-app catalog editing** — Why parked: PRD §Non-Goals.
- **No social, sharing, or comparison features** — Why parked: PRD §Non-Goals.
- **No Warmup or Winddown modes in MVP** — Why parked: PRD §Non-Goals (absorbed into the adaptive loop's own ramp behavior).
- **No editing or deleting of past sessions** — Why parked: PRD §Non-Goals.
- **No ML or learned models for recommendation** — Why parked: PRD §Non-Goals.
- **No analytics dashboards or trend charts** — Why parked: PRD §Non-Goals.
- **No prose explanation of recommendations** — Why parked: PRD §Non-Goals. Narrowed 2026-09-06: a terse, non-prose rationale tag in the collapsed Session-balance panel is in scope via change `session-hold-balance-panel`; the primary session view stays just the board.
- **No coach-side or multi-user-management surface** — Why parked: PRD §Non-Goals.
- **No offline-first guarantee** — Why parked: PRD §Non-Goals.
- **No move-type tagging** — Why parked: PRD §Non-Goals (cannot be sourced/tagged reliably for MVP; only hold-type tags are used).

## Done

(Empty — `/10x-archive` will append entries here as changes archive.)

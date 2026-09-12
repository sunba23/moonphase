# Start Session, First Problem — Plan Brief

> Full plan: `context/changes/start-session-first-problem/plan.md`

## What & Why

Roadmap slice **S-03** (PRD FR-005, FR-006, FR-007, FR-011; US-01). A signed-in, onboarded
climber taps **Main Session** on the hub and immediately sees the first recommended problem —
at the catalog's minimum grade for their board + angle — with its name, grade, hold layout, and
per-hold type tags, inside the ~10 s guardrail. This is the first slice that runs the catalog
end-to-end; it stops before the adaptive loop (no RPE, no next pick, no end-session — all S-04).

## Starting Point

Auth, profile, onboarding, and a 3-tier chi router are shipped. The catalog is ingested for all
4 boards but only **2016** and **2024** have tagged holds *and* a board image. The hub `/` is a
hardcoded `indexHTML` string. There is **no session or recommender code or schema anywhere** —
migrations stop at `0006_profiles`. There are no DB-backed tests yet.

## Desired End State

Hub shows the user's board / angle / max grade and a Main Session button. Tapping it creates a
`sessions` row, picks a problem at the board+angle minimum grade via `recommender.FirstPick`,
records it as `session_problems` seq 0, and redirects to `/session/{id}` — which renders the
problem with the used holds circled on the board image (`static/moonboard/<year>.jpg`) and a
hold-type-tag list. Refreshing or re-tapping resumes the same session. Another user's session id
returns 404. A profile on an unsupported board gets a "switch your board" page.

## Key Decisions Made

| Decision | Choice | Why | Source |
| --- | --- | --- | --- |
| Session persistence | `sessions` + `session_problems` now; first pick = seq 0 pinned to a `problem_configuration_id` | Real vertical slice; clean table for S-04 to extend | Plan |
| Recommender packaging | New `internal/recommender` (`FirstPick` + fx Module); S-04 adds `PickNext` | The package the test-plan Phase 1 targets; FR-011 is a recommender rule | Plan |
| First-pick strategy | Random among min-grade candidates, quality-filtered (`is_benchmark` OR `repeats >= N`), fallback to any | Avoids surfacing obscure problems; varied openers; empty-set fallback = test-plan Risk #4 | Plan |
| Hold layout | Circle markers overlaid on the board image at calibrated `(col,row) → (%)` positions, role-coloured, + a tag list | User reads holds off the app to set the problem by hand; the images (`static/moonboard/`) already exist | Plan (user-specified) |
| Untagged boards | Restrict onboarding + profile board dropdown to app-ready boards (2016, 2024); unsupported profile → switch-board page at session start | Removes a live broken path; a session can't start without an image + tags | Plan |
| Existing active session | One active session per user (partial unique index); Main Session resumes it | No orphan rows; clean state for S-04's "end session" | Plan |
| Hub | Real `templ` HubPage with board/angle/max-grade context, replacing `indexHTML` | The button needs a home; context helps the user confirm before starting | Plan |
| Testing | Stand up the `testcontainers-go` Postgres harness now + integration tests for the session store & first-pick query | Covers test-plan Risks #1/#2/#4 for real; harness is needed regardless | Plan (user-chosen) |

## Scope

**In scope:** `sessions` + `session_problems` schema; `internal/session` store; `internal/recommender`
(`FirstPick`); `internal/testdb` harness + integration tests; catalog queries (`MinGradeCandidates`,
`ProblemDetail`, `SupportedBoards`); board-dropdown restriction; real hub page + first stylesheet;
`POST /session` + `GET /session/{id}` with ownership 404; board-image overlay component + calibration.

**Out of scope:** RPE / completion / next-pick / end-session / hold-type balance (S-04);
`session_problems` result columns; past-sessions surface (S-05); CI wiring; Masters 2017/2019
tagging or imaging; any change to auth/onboarding middleware behaviour.

## Architecture / Approach

New packages mirror `internal/profile` (`Store{pool}` + `NewStore` + `fx.Module` + `Err*`
sentinels, inline SQL). `internal/recommender` owns the first-pick *policy* (pure `pickFrom` +
a thin `FirstPick` that calls `catalog.MinGradeCandidates`); `internal/catalog` owns the SQL.
Session routes live in the existing tier-3 `OnboardingGate` chi group. The board overlay is a
`position:relative` container with the image and absolutely-positioned `%`-placed circle spans,
positions from a per-board calibrated linear `(col,row) → (x%,y%)` map.

## Phases at a Glance

| Phase | What it delivers | Key risk |
| --- | --- | --- |
| 1. Schema + session store + DB harness | `0007`/`0008` migrations, `internal/session`, `internal/testdb` + integration tests | New schema S-04 inherits; first testcontainers wiring + `auth.users` FK shim |
| 2. Recommender + catalog queries | `internal/recommender.FirstPick`, `MinGradeCandidates`, `ProblemDetail`, `SupportedBoards` | Getting the min-grade + quality-filter + fallback right; lexical grade ordering |
| 3. Board-dropdown restriction | Onboarding + profile offer only 2016/2024 | Tiny; pre-existing profiles on 15/17 lose pre-selection |
| 4. Real hub page | `templ` HubPage + `static/app.css`, `indexHTML` deleted | First stylesheet; layout change touches all pages (adds a `<link>`) |
| 5. Session routes + overlay view | `POST /session`, `GET /session/{id}`, problem card + board overlay, ownership 404 | Overlay geometry; one-active-session race; ownership leak |
| 6. Calibration + E2E verification | Calibrated overlay constants for both boards; full manual flow | Purely visual iteration against the images |

**Prerequisites:** S-01 (signup-and-onboarding) and F-02 (catalog-data-foundation) — both
impl_reviewed. Docker running for the integration tests.
**Estimated effort:** ~3–4 focused sessions across 6 phases; Phase 1 (harness) and Phase 5
(overlay) carry the most new ground.

## Open Risks & Assumptions

- Board-image overlay constants are placeholder estimates; calibration in Phase 6 is real
  iterative work and both images are assumed to share geometry (looks true — verify each).
- `move_type` role mapping (`s`/`e`/`f`/rest) is confirmed from sampled benchmark problems but
  `p`/`m`/`o` semantics are inferred; verify against more problems during Phase 5/6.
- `gen_random_uuid()` assumed available (Postgres ≥ 13 / Supabase 15+).
- The `testcontainers-go` harness overlaps `test-plan.md` §3 Phase 1 — re-run
  `/10x-test-plan --status` after this slice lands.

## Success Criteria (Summary)

- An onboarded climber taps Main Session and sees a real, catalog-backed first problem at the
  board+angle minimum grade, with its holds circled on the board image, within ~10 s.
- The session persists: refresh or re-tap resumes the same session and problem.
- A climber can never reach another climber's session (`/session/{id}` → 404 for non-owners).
- A profile on an unsupported board is guided to change it rather than hitting an error.

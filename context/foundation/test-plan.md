# Test Plan

> Phased test rollout for this project. Strategy is frozen at the top
> (§1–§5); cookbook patterns at the bottom (§6) fill in as phases ship.
> Read before writing any new test.
>
> Refresh: re-run `/10x-test-plan --refresh` when stale (see §8).
>
> Last updated: 2026-09-02 (rollout reconciled — Phases 1–3 delivered by the
> S-03/S-04 feature slices; Phase 4 rescoped to perf-only, the AI-native
> trajectory judge dropped)

## 1. Strategy

Tests follow three non-negotiable principles for this project:

1. **Cost × signal.** The cheapest test that gives a real signal for the
   risk wins. Do not promote to e2e because e2e "feels safer." Do not put a
   vision model on top of a deterministic visual diff that already catches
   the regression.
2. **User concerns are first-class evidence.** Risks anchored in "the
   team is worried about X, and the failure would surface somewhere in
   <area>" carry the same weight as PRD lines or hot-spot data.
3. **Risks are scenarios, not code locations.** This plan documents *what
   could fail* and *why we believe it's likely* — drawn from documents,
   interview, and codebase *signal* (churn, structure, test base). It does
   NOT claim to know which line owns the failure. That knowledge is
   produced by `/10x-research` during each rollout phase. If the plan and
   research disagree about where the failure lives, research is the
   ground truth.

Hot-spot scope used for likelihood weighting: `internal/`, `cmd/`,
`templates/`, `migrations/` (excluding docs, fixtures, `context/`, build
output).

## 2. Risk Map

The top failure scenarios this project must protect against, ordered by
risk = impact × likelihood. Risks are failure scenarios in user / business
terms, not test names. The Source column cites the *evidence that surfaced
this risk* — never a specific file as "where the failure lives" (that is
research's job, see §1 principle #3).

| # | Risk (failure scenario) | Impact | Likelihood | Source (evidence — not anchor) |
|---|---|---|---|---|
| 1 | The adaptive loop does not adapt: a hard-or-failed result still yields an equal-or-harder next pick, repeated easy sends never ramp grade, or hold-type balance never fires (four crimp problems in a row). The product's entire premise silently fails. | High | High | PRD §Success Criteria (primary); FR-012; US-01 acceptance criteria; interview Q1 (top fear), Q3 (most-changed, least-confident area). Recommender package does not exist yet → will be the highest-churn code. |
| 2 | A climber reaches another climber's sessions, RPE history, or profile by guessing or reusing an ID — the private-by-design guarantee breaks (IDOR / missing ownership check). | High | Medium | PRD §Access Control; NFR "visible only to that user"; interview Q1 (top fear), Q2 (burned by auth/session parsing), Q3 (low confidence: routing + gating). Abuse lens: authorization/ownership. |
| 3 | A climber taps End session (or the connection blips mid-loop) and the climbed problems + ratings do not persist — history is empty or truncated. | High | Medium | FR-010; US-01 acceptance criteria ("partial session saved with climbed problems intact"); interview Q4 (full lifecycle untested), Q2 (data-loss burn). History is the "second-most-valuable surface". |
| 4 | The engine recommends above the user's stated max, empties its candidate set (grade band exhausted or every hold type recently used) and hangs / errors at the wall, or accepts an out-of-contract result (RPE outside 1–10, unknown status, a problem the user was never shown) and records garbage. | High | Medium | FR-011; FR-012 ("never above stated max"); PRD §Business Logic; interview Q4. Abuse lens: untrusted input / resource exhaustion. |
| 5 | The auth / onboarding gate is not re-mounted on a new session / history / profile route: a logged-out request reaches a private page, or an un-onboarded user (no max grade) reaches Main Session and the recommender runs on nil inputs. | High | Medium | FR-003; interview Q2 (auth/session), Q3 (gating middleware); hot-spot dir `internal/server` (18 commits/30d). Regression-on-refactor shape. |
| 6 | An HTMX swap regresses: the next-problem response is a full page or the wrong fragment, the RPE form loses its target, or a partial renders detached from layout — the climber is stuck mid-session on a phone. | Medium | Medium | Interview Q2 (templ/HTMX named as a past burn); NFR one-handed mobile; hot-spot dir `templates/pages` (11 commits/30d). Generated `*_templ.go` is out of scope (§7); the swap wiring is not. |
| 7 | Guardrails are silently missed: first pick > ~10 s p95, next pick > ~3 s p95 once the candidate query joins hold-type composition over a full board. | Medium | Medium | NFRs; PRD §Guardrails; interview Q4. No measurement exists today; catalog is ~260k problems; the composition join is new. |

Order rows by impact × likelihood: protect Risk #1 (High × High) first, then
the High × Medium band (#2–#5), then the Medium × Medium band (#6–#7).

### Risk Response Guidance

| Risk | What would prove protection | Must challenge | Context `/10x-research` must ground | Likely cheapest layer | Anti-pattern to avoid |
|------|-----------------------------|----------------|--------------------------------------|-----------------------|-----------------------|
| #1 | Over a scripted RPE + completion sequence, difficulty and hold-type composition move in the direction the rating implies — down or flat after hard-or-failed, allowed up after easy-sent; no fourth consecutive same-dominant-type pick. | "The pick changed" is not "it changed correctly" — a different problem at the same difficulty and style is not adaptation. | The scoring function inputs and weights; how "dominant hold type" is derived; what the candidate pool is at each step. | Unit (pure function, table-driven) for single rules; a **deterministic** multi-step integration/e2e test (scripted RPE sequence, explicit assertions on grade-ladder-index direction and dominant balance) for the trajectory; manual eyeball for feel. | Asserting an exact problem ID (many picks are valid → brittle or tautological); expected values copied out of the scorer; **an LLM judge grading "coaching feel" over a fully-specified rule engine** (non-deterministic, per-run cost, inverted oracle). |
| #2 | A request from user B for user A's session / history / profile is refused (404 / 403); a list endpoint returns only the caller's rows. | "Logged in" is not "owns this resource" — a passing happy-path test says nothing about isolation. | Where user identity enters the query; whether scoping is in the SQL `WHERE` or a post-fetch check; the ID shape (sequential vs. UUID). | Integration (real handler + real DB, two seeded users). | Oracle lifted from the handler's own filter code; testing only the owner path. |
| #3 | After End session, and after an abrupt drop mid-loop, reloading history shows every problem climbed so far with its RPE and status. | "The row was inserted" is not "the session is coherent" — per-problem writes and one end-of-session write behave differently under interruption. | When results are persisted (per submit or on end); the transactional boundary; what "end" writes vs. what each submit writes. | Integration (real DB; simulate clean end and simulate mid-flow abandonment). | Asserting only the clean End-session path; over-mocking the store so persistence timing is never exercised. |
| #4 | The ceiling is never exceeded across a full session; an emptied candidate set degrades to a defined fallback (easier grade / relaxed style), not a 500 or a hang; a malformed result is rejected with a 4xx and no DB write. | "The final status was 200" is not "the input was valid"; "there is always a candidate" is an assumption, not a guarantee. | The candidate filter's fallback order; server-side validation of RPE range + status enum + problem-was-recommended; the transaction boundary on result submit. | Unit for the ceiling + filter fallback; integration for the rejection + no-write. | Only testing the populated-pool path; trusting the client to send a valid problem ID. |
| #5 | Every session / history / profile route redirects a logged-out caller and redirects an un-onboarded caller to onboarding; the recommender is never entered without a max grade. | "The gate is in the middleware list" is not "it runs on this route" — new route groups silently miss shared middleware. | How route groups mount the gate; whether the gate is per-router or per-route; what the recommender does with a nil profile. | Integration (a table of routes × caller states). | Testing the gate in isolation but never on the real mounted routes. |
| #6 | A next-problem request returns the problem-card fragment (not a full document), the response targets the right element, and the loop advances with no full-page reload. | "HTTP 200 with HTML" is not "the right fragment for the right target". | Which responses are fragments vs. full pages; the `hx-target` / `hx-swap` contract per interaction; how templ partials compose with the layout. | e2e (one browser path) — nothing cheaper sees a detached fragment or a lost target. | Snapshot-testing generated `*_templ.go`; asserting on whole HTML strings. |
| #7 | First pick and each next pick complete within budget against a full-board-sized catalog, measured, with the number recorded over time. | "It was fast with three seed rows" is not "fast at catalog scale". | The candidate query plan; index coverage on grade + board/angle + hold-type; realistic row counts per board. | Integration benchmark (seeded full board), non-blocking in CI. | Measuring against a tiny fixture; turning a flaky wall-clock assertion into a blocking gate. |

## 3. Phased Rollout

Each row is a rollout phase. The original plan expected each to open its own
`testing-*` change folder; in practice Phases 1–3 were **delivered inside the
feature slices** (`start-session-first-problem`, `adaptive-main-session-loop`,
`adaptive-loop-ramp-fix`) as those shipped, so no `testing-*` folders exist.
The 2026-09-02 refresh reconciled Status to reality.

| # | Phase name | Goal (one line) | Risks covered | Test types | Status | Change folder |
|---|---|---|---|---|---|---|
| 1 | DB harness + recommender units | Stand up an ephemeral-Postgres integration harness and densely cover the recommender's single-step rules (grade ramp, back-off, max-grade ceiling, empty-candidate fallback, out-of-contract rejection) | #1, #4 | integration harness, unit | complete | delivered by `start-session-first-problem` + `adaptive-main-session-loop` (`internal/testdb`; `classify`/`gradeWindow`/`scoreNext` + `RecomputeHoldTypes`/`GradeLadder`/`NextPickCandidates`/`PickNext` tests) |
| 2 | Session lifecycle + isolation | Prove the full loop against real handlers + real DB: start → view → submit → next → end → reload history persists partial sessions, and every read is scoped to the caller | #2, #3, #4, #5 | integration | complete | delivered by `adaptive-main-session-loop` (`TestAdvanceSession`, `TestEndSession`, `TestHandleResult` 404/422/409/persistence, `TestHandleView_ShowsLatestProblem`, route-table gate tests) |
| 3 | Critical-path e2e | One browser path: sign up → onboard → session → rate → next recommendation changes → end, with HTMX swaps landing the right fragment and no full-page reload | #6, #1, #5 | e2e | complete | delivered by `auth-first-problem` slice + `adaptive-main-session-loop` + `adaptive-loop-ramp-fix` (`tests/e2e/{auth-first-problem,adaptive-session-loop}.spec.ts` — full loop, HTMX swap, gate, both boards, ramp, End; deliberate-break verified) |
| 4 | Perf guardrails at catalog scale | Measure the ~10 s first-pick / ~3 s next-pick guardrails against realistic per-grade row counts, with the numbers recorded | #7 | integration benchmark | complete | delivered by `adaptive-loop-ramp-fix` (`BenchmarkPickNext` @ 5 k configs/grade; new `BenchmarkFirstPick`) |

**Status vocabulary** (fixed — parser literals): `not started` →
`change opened` → `researched` → `planned` → `implementing` → `complete`.

Phase 3's e2e is above the classic layer only because HTMX swap correctness
and the redirect / gate chain do not surface below a real browser.

**Phase 4 was "Perf guardrails + trajectory judge" — the AI-native judge half
was dropped on 2026-09-02.** It was a hypothesis from 2026-08-31, before the
recommender existed. The recommender shipped as a pure rule engine (PRD
Non-Goal: no ML), and every trajectory-relevant invariant is now pinned by
deterministic tests: grade direction (`classify`/`gradeWindow` units + e2e
deliberate-break), ramp on easy sends (`TestPickNextRampEscapesDenseFloor` +
e2e), hold-type balance / no 4-in-a-row (`TestPickNext` crimp-streak), ceiling
and 4-tier fallback (`TestPickNext`). An LLM judge over that would grade
subjective "coaching feel" — non-deterministic, per-run API cost, inverted
oracle. Revisit only if a manual trajectory review ever surfaces a defect the
unit/e2e layer misses.

## 4. Stack

The classic test base for this project. AI-native tools carry a `checked:`
date so future readers can see which lines need re-verification.
Recommendations are grounded in local manifests/configs plus the MCP/tools
exposed in the current session.

| Layer | Tool | Version | Notes |
|---|---|---|---|
| unit | Go std `testing`, table-driven | Go 1.26 | In use — ~28 `*_test.go` across `internal/{auth,catalog,recommender,session,server}`; pure functions table-driven, `roll func(n int) int` stubbed for rng. checked: 2026-09-02. |
| integration (DB) | `testcontainers-go` postgres module | in use | `internal/testdb.New(t) *pgxpool.Pool` — `postgres:17-alpine`, all `migrations/` applied, `auth.users` shim. One container per package suite; DB tests **fail** (not skip) without Docker. checked: 2026-09-02. |
| integration assertions | — | not adopted | Std `if … { t.Fatalf(…) }` throughout; no `testify`. Fine as-is. |
| e2e | **TypeScript Playwright** (`@playwright/test`) | in use | `tests/e2e/*.spec.ts` — role/label/text locators, Pixel 7 viewport, `webServer` runs `go run ./cmd/server`, real Supabase Auth, `afterEach` deletes the throwaway user (FK cascade). Deviates from the `playwright-go` originally named — see `tests/e2e/README.md`. checked: 2026-09-02. |
| AI-native | LLM-judge via the Claude API | **considered, not adopted (2026-09-02)** | The recommender is a pure rule engine (PRD Non-Goal: no ML); every trajectory invariant is deterministically testable (grade direction, ramp, hold-type balance, ceiling, fallback). An LLM judge would grade subjective "coaching feel" — non-deterministic, per-run API cost, inverted oracle. Revisit only if a manual trajectory review finds a defect the unit/e2e tests miss. |
| CI | GitHub Actions | in use | `.github/workflows/ci.yml` runs build, templ-staleness, vet, lint, format, `govulncheck`, and `go test ./...` on every PR and on push to the default branch; a non-blocking `bench` job runs the recommender benchmarks. Required as a branch-protection status check. Delivered by `ci-and-release-flow`, wiring owned by that module. checked: 2026-09-06. |

**Stack grounding tools (2026-09-02 refresh):**
- Docs: Context7 available — not re-queried this refresh (no new tool choices; the doc-only reconcile needed none); last verified 2026-08-31
- Search: none available in current session
- Runtime/browser: `claude-in-chrome` available — not used; the e2e layer runs headless Playwright via its own `webServer`
- Provider/platform: Railway + Supabase MCPs available — relevant to Risk #7 post-deploy observation (`http_response_time`, `service_metrics`); not used now

## 5. Quality Gates

The full set of gates that must pass before a change reaches production.
"Required after §3 Phase <N>" means the gate is enforced once that rollout
phase lands; before that, the gate is `planned`.

| Gate | Where | Required? | Catches |
|---|---|---|---|
| lint + format | local + CI | required — enforced by `ci` | style / import drift; `.golangci.yml` exists |
| vet + vuln | local + CI | required — enforced by `ci` | `go vet` issues, known CVEs via `govulncheck` |
| unit + integration | local + CI | required — enforced by `ci` | logic regressions; `go test ./...` needs Docker for the integration suite |
| e2e on critical flow | local only | required locally; not yet wired into CI (deferred — needs a real Supabase Auth project, see `ci-and-release-flow`) | broken session loop, HTMX swap, gate redirects, grade ramp |
| post-edit hook | local (agent loop) | recommended (Module 3 Lesson 3) | regressions at edit time — `golangci-lint fmt` |
| perf benchmark | CI | optional (non-blocking) — `BenchmarkPickNext` / `BenchmarkFirstPick` exist | guardrail drift at catalog scale |
| multimodal visual review | manual, 3 screens | optional | session card / RPE form / history detail — visual issues classic diff misses |

## 6. Cookbook Patterns

How to add new tests in this project.

### 6.1 Adding a unit test

- Pure functions only (no DB, no HTTP). Table-driven: a slice of
  `{name string; …inputs; want …}`, `t.Run(tc.name, …)`.
- For anything that draws randomness, take a `roll func(n int) int` parameter
  and stub it (`func(int) int { return 0 }`) — never call `rand` directly in
  the function under test.
- References: `internal/recommender/grade_window_test.go` (`classify`,
  `gradeWindow`, ladder-end clamps), `internal/recommender/score_test.go`
  (`scoreNext` — grade fit, balance override, `DropBalance`, tie via stubbed
  `roll`, empty → `ErrNoCandidates`).

### 6.2 Adding an integration test (real Postgres)

- `pool := testdb.New(t)` → a `*pgxpool.Pool` on a fresh `postgres:17-alpine`
  container with every migration applied. One container per package test
  binary; it **fails** (not skips) without Docker.
- Seed with inline `INSERT`s; for a pool larger than a query's `LIMIT`, use a
  one-statement CTE bulk seed — see `seedNextProblemsBulk` /
  `seedPickBulk` (`WITH new_problems AS (INSERT … SELECT generate_series …)`).
- Assert on real query output; keep assertions membership / count /
  direction, never row order (`NextPickCandidates` returns a random sample).
- References: `internal/catalog/candidates_test.go`,
  `internal/recommender/pick_next_test.go`, `internal/session/store_test.go`.

### 6.3 Adding an e2e test

- Model on `tests/e2e/seed.spec.ts`. Reuse the helpers in
  `adaptive-session-loop.spec.ts`: `signUpOnboardStart(page, board)`,
  `submitResult(page, completion, rpe)`, `readCardGrade`, `waitForCardSettled`
  (syncs on htmx's `htmx-settling` / `htmx-swapping` classes), `FONT_LADDER`.
- Locators: `getByRole` / `getByLabel` / `getByText` only. Wait on state
  (`waitForURL`, `toBeVisible`, `waitForResponse`, `waitForFunction` on an
  htmx class) — **never `waitForTimeout`**.
- Each test: own throwaway account (`moonphase-e2e+<timestamp>@example.com`),
  `afterEach` deletes the Supabase user (FK cascade clears profile + sessions).
- Prove the test bites: temporarily invert the production behavior it guards,
  confirm it goes red, revert. (See the `gradeWindow` deliberate-break note in
  `adaptive-session-loop.spec.ts`.)

### 6.4 Adding a test for a new HTTP handler

- Route mounting + gate: add the route to every table in
  `internal/server/server_test.go` (`…RedirectWithoutSession`,
  `…WithoutProfileRedirectsToOnboarding`, `…ReachesProtectedRoutes`) and to the
  `newTestRouter` stand-ins.
- Behavior: real handler + `testdb` pool + seeded `auth.users`. Cover
  ownership (non-owner → identical 404, zero writes), validation (each bad
  input → 4xx, DB unchanged), and the happy path (status + body shape + the
  row it wrote). Reference: `internal/server/session_test.go` `TestHandleResult`.

### 6.5 Adding a recommender trajectory check

- **Not an LLM judge** (see §3 Phase 4 / §4). A trajectory check is a scripted
  RPE + completion sequence fed through `PickNext` (integration) or driven in
  the browser (e2e), with explicit assertions on grade-ladder-index direction
  and dominant balance at each step.
- References: `internal/recommender/pick_next_test.go`
  (`TestPickNextRampEscapesDenseFloor`, crimp-streak, never-harder),
  `tests/e2e/adaptive-session-loop.spec.ts` ("a run of easy sends ramps grade
  past the dense floor", "a failed or bailed attempt never yields a strictly
  harder next problem").

### 6.6 Per-rollout-phase notes

- Phases 1–3 shipped inside the `start-session-first-problem` /
  `adaptive-main-session-loop` slices; Phase 4 (perf-only) inside
  `adaptive-loop-ramp-fix`. See each change folder's `plan.md` Progress
  section for the per-test evidence trail.

## 7. What We Deliberately Don't Test

Exclusions agreed during the rollout (Phase 2 interview, Q5). Future
contributors should respect these unless the underlying assumption changes.

- **Generated `*_templ.go` files and static layout / marketing markup** —
  the compiler is the test; visual review (§5) covers appearance.
  Re-evaluate if templ output starts carrying hand-edited logic. (Source:
  Phase 2 interview Q5.)
- **Supabase Auth / gotrue token issuance** — trust the managed service; we
  test only our own verification and gating. Re-evaluate if auth moves
  in-house. (Source: Phase 2 interview Q5.)
- **The one-shot catalog ingestion CLI (`cmd/catalog`)** — verified by
  row-count assertions at import time, not a standing suite. Re-evaluate if
  ingestion becomes an ongoing sync rather than a one-shot import. (Source:
  Phase 2 interview Q5; `CLAUDE.md` lesson on ingestion tooling.)
- **Per-hold tag correctness** (is a given hold really a "crimp") —
  subjective and hand-curated; `internal/catalog` already tests that tags
  are present and well-formed, not that they are right. Re-evaluate if
  tagging is ever automated. (Source: Phase 2 interview Q5.)
- **Cross-browser matrix** — one Chromium e2e path only; the NFR's "latest
  two versions of four browsers" is a manual spot-check budget, not
  automation. Re-evaluate if a browser-specific bug ships. (Source: Phase 2
  interview Q5; scope decision.)

## 8. Freshness Ledger

- Strategy (§1–§5) last reviewed: 2026-09-02
- Stack versions last verified: 2026-09-02
- AI-native tool references last verified: 2026-09-02 (LLM-judge dropped —
  rule engine is fully deterministic)
- Rollout §3 reconciled to reality: 2026-09-02

Refresh (`/10x-test-plan --refresh`) when:

- a new top-3 risk surfaces from the roadmap or archive,
- a recommended tool's `checked:` date is older than three months,
- the project's tech stack changes (new framework, new test runner),
- §7 negative-space no longer matches what the team believes.

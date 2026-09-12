# Recommender Decision Log (`rec_pick`) — Plan Brief

> Full plan: `context/changes/observability-expanded/plan.md`
> Research: `context/changes/observability-expanded/research.md`

## What & Why

MoonPhase's recommender picks the next problem from RPE + hold-type history, but today nothing records *why* it picked what it did — a normal pick logs nothing, and only a rare fallback-tier warn ever fires. This change emits one structured `rec_pick` log line per pick, keyed by `session_id`, carrying the input and the decision, so the owner can grep one session id and reconstruct/tune the algorithm.

## Starting Point

On `release/v1.0.0` (the PR base), `recommender.PickDiag` already exists as the intended "how PickNext reached its answer" struct, but only its `FallbackTier` field is ever read. `classify`'s band, the aimed-at grade, and the scorer's tie-set size are computed and thrown away. The `recommender` package is deliberately logger-free; the session HTTP handlers own logging.

## Desired End State

Every problem served produces exactly one single-line-JSON `rec_pick` entry on stdout (which Railway ingests and makes queryable for 7 days). Adaptive picks carry `rpe`, `completion`, `prev_*`, `band`, `grade_lo/hi`, `pref_grade`, `excluded_dominant`, `fallback_tier`, `tie_set_size`, `pick_*`. First picks (session start, skip-before-any-rating) carry a minimal `kind=first_pick` line. Pick *behaviour* is byte-for-byte unchanged.

## Key Decisions Made

| Decision | Choice | Why (1 sentence) | Source |
| --- | --- | --- | --- |
| Scope of this change | `rec_pick` decision log only | Owner cut error-tracing, metrics, and alerting slices after the metrics-consumption paths all implied a container or vendor. | Plan |
| Trace mechanism | Structured stdout log keyed by `session_id` | Grep-able, zero added network/compute, Railway parses JSON fields natively. | Research |
| Field set | Core decision (~16–18 fields), no runner-up scores | Enough to answer "why climb X" and "decisive or coin-flip"; more fields = more cost. | Plan |
| Recommender stays logger-free | Widen `PickDiag` + `Pick`, log from the handler | Honors the `adaptive-main-session-loop` ruling; keeps recommender unit-testable. | Research |
| First-pick lines | Emit a minimal line too | A `session_id` grep should return the whole session including its start. | Plan |
| Emit timing | After the DB write succeeds | A logged line always corresponds to a problem actually served. | Plan |
| Metrics / Grafana Cloud | Dropped | Free tier can't reach a private endpoint; every delivery path meant a container or vendor dep the owner rejected. | Plan |

## Scope

**In scope:**
- Add `Band`, `PreferredGrade`, `TieSetSize` to `PickDiag`; `Dominant` to `Pick`.
- `scoreNext` returns the tie-set size; `band` gets a string label.
- `FirstPick`/`FirstPickExcluding` return the candidate-pool size.
- A `logPick` helper + calls from `handleStart`, `handleResult`, both `handleSkip` branches.
- Unit tests (recommender) + handler tests asserting the line.

**Out of scope:**
- `request_id`/panic-recovery/`fx.WithLogger`/`/readyz`/silent-500 fixes (research slice B).
- Railway webhook/Slack alerting, Supabase runbook (slice D).
- New deps, config/env vars, migrations, Postgres persistence, log-level plumbing.
- Any change to pick selection behaviour.

## Architecture / Approach

Two phases. **Phase 1** is a pure `recommender` widening — surface data `PickNext` already computes, add `Pick.Dominant`, change `scoreNext`'s return arity — with no behaviour change and updated unit tests. **Phase 2** adds `internal/server/session_log.go` with a `logPick` helper and wires it into the three (four-branch) pick sites in `session.go`, replacing the existing `fallback_tier > 0` warn with an always-on `Info` line. `session_id` is already in scope at every call site.

## Phases at a Glance

| Phase | What it delivers | Key risk |
| --- | --- | --- |
| 1. Widen recommender diagnostics | `PickDiag`/`Pick` carry the full decision; `scoreNext` returns tie size | `scoreNext` signature change ripples to ~5 `score_test.go` call sites (mechanical) |
| 2. Emit the `rec_pick` log | One JSON line per pick, `session_id`-keyed, from all pick sites | Handler tests must switch off `zerolog.Nop()` to capture output; keeping the line to one JSON object |

**Prerequisites:** rebase `feat/observability-expanded` onto `release/v1.0.0` (where `bailed`→`skipped` and CI already landed). PR targets `release/v1.0.0`.
**Estimated effort:** ~1 session, 2 small commits.

## Open Risks & Assumptions

- Tier-4 fallback picks have no dominant available → `pick_dominant` is `""` on those lines (acceptable; `fallback_tier=4` explains it).
- `FirstPick` signature change (return pool size) touches a function with a few callers — Phase 1 already edits that file, so low marginal cost; falling back to `pool_size=0` is tolerable if it proves noisy.
- Field names become Railway log-query keys — a shipped contract; the plan flags "do not rename casually".
- Assumes `release/v1.0.0` is stable enough to branch from now (it carries merged PRs #16–#21).

## Success Criteria (Summary)

- Running a full local session yields one `rec_pick` line per problem shown, `seq`-ordered, each valid single-line JSON, same `session_id` throughout.
- A sampled line's fields tell a coherent story (`band` + grade window + `tie_set_size` + `pick_grade` agree).
- Full CI gate green; no change to recommender pick selection in `git diff`.

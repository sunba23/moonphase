# Catalog Data Foundation — Plan Brief

> Full plan: `context/changes/catalog-data-foundation/plan.md`

## What & Why

Build the Postgres schema and one-shot ingestion tooling to load MoonPhase's MoonBoard problem catalog — all 4 board editions, hold-type tags for every physical hold — so S-03 (first recommended problem) and S-04 (the adaptive session loop, the product's north star) have real data to work with. Nothing recommends from this catalog yet; this slice is purely data foundation.

## Starting Point

No `migrations/` directory, no schema, no query code exist yet. `internal/db/pool.go` already wires an fx-managed `*pgxpool.Pool`, unused until now. The roadmap originally named `CSTDev/moonapi` (a live scraper) as the catalog source.

## Desired End State

`problems`, `problem_configurations`, `holds`, and `problem_moves` tables in Postgres, populated from all 4 static exports (259,761 raw problems, filtered to active/non-deleted), with every physical hold across all 4 boards hand-tagged with a primary type. A `catalog` CLI and a `migrate` CLI exist as the repeatable tooling; no HTTP endpoints yet.

## Key Decisions Made

| Decision | Choice | Why (1 sentence) |
| --- | --- | --- |
| Catalog source | Static JSON exports (all 4 boards), not `CSTDev/moonapi` | The named source is 7 years stale and likely broken; this lands on the PRD's own pre-approved static-export fallback tier instead. |
| Scope | Ingest all 4 boards + both angles now | Marginal ingestion cost across boards is ~zero once the schema/CLI exist. |
| Source data location | External (`--file` path), not committed | 96MB uncompressed JSON per board is unconventional to carry in a code repo. |
| Ingestion mechanism | One-shot CLI (`cmd/catalog`), no fx | A batch job has no lifecycle to manage; fx's value is DI+lifecycle for long-running services. |
| Migrations | `golang-migrate`, verified against this repo's pgx version | De facto standard Go migration tool; matches CLAUDE.md's `migrations/` convention. |
| Hold-tag format | Hand-edited CSV per board | Std-lib `encoding/csv`, no new dependency, opens in a spreadsheet for the actual tagging work. |
| Hold taxonomy | `primary_type` ∈ {crimp, sloper, pinch, jug, pocket} + ≤3 modifiers, Go-validated | Matches the roadmap's own phrasing; DB-level enum would block taxonomy growth behind a migration. |
| Tag completion bar | Full coverage across all 4 boards required to close this slice | Explicit, confirmed user decision — not deferred. |
| Testing | Unit-test pure parsing logic against fixtures; DB-touching code verified manually | Real 96MB files / live Postgres don't belong in CI for a one-shot tool. |
| Board identifier (CLI/filenames) | Bare year string (2016/2017/2019/2024), not raw `holdsetup` codes (1/15/17/21) | The 4 boards happen to have 4 distinct years, so year alone is unambiguous; raw `holdsetup` ints are meaningless to a human filling in CSVs or typing flags. |
| Hold-type input | 2-letter abbreviations (`cr`/`sl`/`pi`/`ju`/`po`) accepted alongside full names in the CSV | Full names are slow to hand-type ~734 times; abbreviations are unambiguous since pinch/pocket get distinct 2-letter codes. |
| Hand-tagging tool | New `catalog holds tag` interactive command (raw keystrokes, digit-keyed primary type + capped-at-2 modifiers, `golang.org/x/term`) alongside the existing CSV-in-a-spreadsheet workflow | A single keypress per decision is dramatically faster than opening a spreadsheet for ~734 rows; kept as a pure CSV-output tool (no DB writes) so `load-tags` stays the one path that persists to Postgres. |

## Scope

**In scope:** Postgres schema (5 tables), `cmd/migrate`, `cmd/catalog` (ingest / holds inventory / holds load-tags / holds status), full ingestion of all 4 boards, full hold-tagging of all 4 boards, a runbook doc.

**Out of scope:** HTTP endpoints, recommendation/query logic (S-03/S-04), move-type semantic interpretation beyond storing codes verbatim, `holdsets`/`coordinates` relational modeling, Railway deploy-time migration automation, DB-backed integration tests in CI.

## Architecture / Approach

Source JSON → `internal/catalog` (pure parsing: problem/config/move/grid-ref/angle decoding) → `ingest.go` (per-problem transaction: upsert problem → replace configs → auto-discover+insert holds → replace moves, FK-enforced) → Postgres. Holds start untagged (auto-discovered from moves); `holds inventory`/`load-tags`/`status` form a bounded, CSV-based hand-tagging loop on top. Two thin, fx-free CLI binaries (`cmd/migrate`, `cmd/catalog`) are the only entrypoints — no runtime service changes.

## Phases at a Glance

| Phase | What it delivers | Key risk |
| --- | --- | --- |
| 1. Migration infra + schema | 5 tables live in Postgres via `cmd/migrate` | golang-migrate/pgx wiring — pre-verified against real package APIs during planning |
| 2. Pure parsing/domain logic | Fully unit-tested parsers, no DB | Low — pure logic, small fixtures |
| 3. Ingestion orchestration | `catalog ingest` loads real data | Real post-filter counts unknown until run; ~260K-row throughput over network |
| 4. Hold inventory/tagging tooling | `catalog holds inventory/load-tags/status` | Low — thin CRUD over `holds` |
| 5. Manual hold tagging | Full tag coverage, all 4 boards | Real time cost — the biggest manual effort in this slice |
| 6. Runbook doc | Reproducible command sequence | Low |

**Prerequisites:** Real Supabase `DATABASE_URL` reachable; user's 4 export zips available locally.
**Estimated effort:** Phases 1-4 are a few focused coding sessions; Phase 5 (manual tagging) is the long pole — hundreds of holds across 4 boards, paced at the user's discretion.

## Open Risks & Assumptions

- Cross-board uniqueness of MoonBoard's numeric `id` is unverified — schema hedges with a surrogate key, so this doesn't block anything.
- One-transaction-per-problem ingestion could be slow over a networked DB; accepted risk, batching is a low-risk follow-up if needed.
- `config.Load()` requires `SUPABASE_URL` even though these CLIs never touch Supabase Auth — minor, accepted coupling.

## Success Criteria (Summary)

- `go build/vet/test` and `golangci-lint run` pass at every phase.
- All 4 real exports ingest into Postgres with sane, filtered row counts.
- `catalog holds status` reports full tagged coverage across all 4 boards — the slice's explicit done-condition.

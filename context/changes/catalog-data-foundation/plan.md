# Catalog Data Foundation Implementation Plan

## Overview

Build the Postgres schema, a golang-migrate-based migration runner, and a `catalog` CLI that ingests four static MoonBoard JSON exports (2016, 2024, Masters 2017, Masters 2019) into a queryable schema, plus a bounded hold-inventory / hand-tag / load-tag workflow that gets every physical hold on all four boards tagged with a primary type and up to ~3 modifiers. No HTTP surface, no recommendation logic — this is schema + one-shot ingestion tooling only. This is F-02 on the roadmap: the catalog data foundation that S-03 (first recommended problem) and S-04 (the adaptive session loop, MoonPhase's north star) both depend on.

## Current State Analysis

- `internal/db/pool.go` — `NewPool(lc fx.Lifecycle, cfg config.Config) (*pgxpool.Pool, error)` couples pool construction to an `fx.Lifecycle` param; there is no fx-independent way to get a pool today, which matters because the new CLIs must not depend on fx.
- `internal/config/config.go` — `Load() (Config, error)` fails fast if `SUPABASE_URL` or `DATABASE_URL` is unset. Reusing it as-is for the CLIs means an operator running `catalog`/`migrate` needs `SUPABASE_URL` set even though ingestion never touches Supabase Auth — accepted as a minor, low-friction coupling (same `.env` already used by `cmd/server` already has it) rather than splitting config.
- `go.mod` has no migration library and no CSV/YAML dependency beyond std lib. `encoding/csv` is std lib and sufficient — no new dependency needed for the seed-file format.
- No `migrations/` directory and no schema exist yet — this change writes the first migrations and the first real schema. No query code exists anywhere in the repo.
- Every existing `internal/` package follows the same `module.go` → `var Module = fx.Module("<name>", fx.Provide(...))` shape (`internal/config`, `internal/db`, `internal/auth`, `internal/logging`). Error wrapping is consistently `fmt.Errorf("...: %w", err)` (`internal/db/pool.go`, `internal/auth/*.go`). Table-driven `*_test.go` co-located with the package is the established testing convention (set by `internal/auth` in the `auth-session-scaffold` change).
- The roadmap originally named `CSTDev/moonapi` as the catalog source. Research during planning found it 7 years stale, scraping a legacy MoonBoard website login flow likely broken against the site's current backend; its documented fallback `spookykat/MoonBoard` is more current but still depends on reverse-engineered auth against personal MoonBoard credentials. The user instead obtained official static JSON exports for all 4 board editions directly — the PRD's own pre-approved fallback tier (`prd.md` Open Question 1's static-`.zip`-export contingency), not a deviation from it. `context/foundation/roadmap.md` (F-02) and `context/foundation/prd.md` (Open Question 1) have been updated to record this.

## Desired End State

`problems`, `problem_configurations`, `holds`, and `problem_moves` tables exist in Postgres, populated from all 4 static exports (filtered to `Active == true && dateDeleted == null`), with every physical hold across all 4 boards hand-tagged with a primary type. No HTTP endpoints, no recommendation logic exist yet.

Verification: `go build ./...`, `go vet ./...`, `golangci-lint run`, `go test ./...` all pass. Manually: `migrate up` against the real Supabase `DATABASE_URL` creates the schema; all 4 real exports ingest with sane row counts; `catalog holds status` reports full tagged coverage on all 4 boards.

### Key Discoveries:

- `internal/db/pool.go` — must be split into an fx-independent `New(ctx, cfg)` (pool + ping) and a thin `NewPool(lc, cfg)` fx wrapper around it, so the new non-fx CLIs and the existing fx server share one real implementation instead of duplicating pool-construction logic.
- Source JSON shape (verified directly, not assumed): `{holdsetup, count, problems: [...]}`. Each problem: `id, name, dateInserted/dateUpdated/dateDeleted, holdsetup, Active, climbMethod, setbyId, setter, moves, coordinates, holdsets, betaVideos, configurations: [...]`. Each `configurations[]` entry: `apiId, grade, userGrade, userRating, dateUpdated, dateDeleted, isBenchmark, isCompetitionProblem, comment, benchMarkCreated, isPrimary, repeats, configuration ("25°"/"40°"), primaryAngle`.
- `moves` is a pipe/tilde-delimited string, e.g. `"s~C5~|p~C13~|p~D15~|e~D18~"` — type codes `s`/`e`/`p` confirmed, plus `l`/`r`/`m` seen in newer sets (exact semantics uncertain; store verbatim, don't interpret further in this slice). `coordinates` is a separate, mostly-null field — ignore/store-if-present, don't rely on it.
- A grid ref (e.g. `C5`) is scoped to its board — the same ref on a different `holdsetup` is a different physical hold. Hold-type tagging therefore happens once per `(holdsetup, grid_ref)` pair, not per problem-move — a much smaller tagging surface than "one entry per problem" would suggest.
- Whether MoonBoard's numeric `id` is unique *across* boards is unverified — the `problems` table uses a surrogate `BIGSERIAL` key + `UNIQUE (holdsetup, external_id)` so the schema doesn't need to care either way.
- golang-migrate's pgx/v5 integration was verified (not assumed) by test-installing `github.com/golang-migrate/migrate/v4` in an isolated scratch module against this repo's exact `pgx/v5 v5.10.0`: `database/pgx/v5.WithInstance(*sql.DB, *Config)`, `source/iofs.New(fs.FS, path)`, and `sql.Open("pgx", ...)` via `pgx/v5/stdlib`'s registered driver name all resolve exactly as designed.
- `go:embed` cannot reference `../` paths, so migration files are embedded via a small `package migrations` living inside `migrations/` itself, not from `cmd/migrate`.

## What We're NOT Doing

- No HTTP endpoints and no repository/query layer for serving recommendations — that's S-03/S-04's job.
- No semantic interpretation of `l`/`r`/`m` move-type codes — stored verbatim as opaque text (matches the roadmap's Parked "No move-type tagging").
- No relational modeling of the `holdsets` field — stored as raw opaque text on `problems`.
- No use of the `coordinates` field beyond storing it verbatim if present.
- No wiring of `cmd/migrate` into Railway's deploy pipeline — applied manually against `DATABASE_URL` for this slice.
- No DB-backed integration tests in CI — `internal/catalog`'s DB-touching code is exercised only via manual verification against the real Supabase instance.
- No DB-level `CHECK` constraint enforcing the hold taxonomy — validated at the Go/CLI layer only, so the taxonomy can grow without a migration.
- No historical audit trail across re-ingestions — re-running `ingest` replaces a problem's configs/moves wholesale rather than diffing.
- No committing the source JSON exports into the repo — they stay external; only the hand-filled hold-tag CSVs get committed.

## Implementation Approach

Six phases: (1) golang-migrate dependency + full schema + `cmd/migrate` + the `internal/db` pool refactor, (2) pure parsing/domain logic with unit tests (no DB), (3) DB ingestion orchestration + `cmd/catalog ingest`, (4) hold-inventory/load-tags/status tooling + unit tests, (5) the actual manual hold-tagging pass across all 4 boards (mostly non-code, but a required, trackable completion gate), (6) a short runbook doc. Each phase is independently buildable; Phase 3 onward requires the real Supabase `DATABASE_URL` for manual verification.

## Critical Implementation Details

**fx vs no-fx CLI decision — no fx in `cmd/catalog` or `cmd/migrate`.** fx's value proposition is DI + *lifecycle* (start/stop hooks, background goroutines) for a long-running process. A one-shot CLI does linear work and exits — there is no lifecycle to manage, and wiring `fx.New(...).Run()` for a batch job would need extra plumbing just to get back to "do work, then exit non-zero on error," which is more code than calling `config.Load()` and constructing a pool directly. Both CLIs call `config.Load()` and the new fx-independent `db.New(ctx, cfg)` directly.

**`problem_moves` → `holds` is a real, strictly-enforced FK, not a soft reference.** Moves are ingested before any hold is tagged, so `ingest.go` must, per problem, collect the distinct `(holdsetup, grid_ref)` pairs from its parsed moves and `INSERT INTO holds (holdsetup, grid_ref) VALUES (...) ON CONFLICT (holdsetup, grid_ref) DO NOTHING` *before* inserting the corresponding `problem_moves` rows, all inside the same per-problem transaction. This is exactly what auto-discovers the untagged hold inventory that Phase 4's tooling later fills in — no separate "enumerate the board layout" step is needed.

**Hold-tagging is a required completion gate, made tractable by inventory generation.** Tagging every physical hold across all 4 boards by hand is real, confirmed, in-scope work — not deferred. To keep it bounded rather than open-ended: after Phase 3's ingestion auto-discovers every `(board, grid_ref)` pair actually used by a move, Phase 4's `holds inventory` command emits a sorted, fillable CSV per board (already-tagged rows pre-filled, so re-running after a future re-ingest only appends new blank rows) — turning "tag every hold" into a bounded checklist instead of an open-ended task.

## Phase 1: Migration infrastructure + schema

### Overview

Add golang-migrate, write all 5 migration pairs, embed them, build `cmd/migrate`, and land the `internal/db` pool refactor both new binaries need.

### Changes Required:

#### 1. Dependency

**File**: `go.mod`

**Intent**: Add golang-migrate and its pgx/v5 + iofs source driver as real dependencies.

**Contract**: `go get github.com/golang-migrate/migrate/v4`. Imports needed: `github.com/golang-migrate/migrate/v4`, `github.com/golang-migrate/migrate/v4/database/pgx/v5` (as `pgxv5`), `github.com/golang-migrate/migrate/v4/source/iofs`, `github.com/jackc/pgx/v5/stdlib` (blank import to register the `"pgx"` `database/sql` driver name).

#### 2. Pool refactor

**File**: `internal/db/pool.go`

**Intent**: Split fx-independent pool construction (with ping) from the fx lifecycle wrapper, so both the fx server and the new non-fx CLIs share one implementation.

**Contract**: `func New(ctx context.Context, cfg config.Config) (*pgxpool.Pool, error)` does `pgxpool.New` + `Ping`, wrapping both errors with `%w`. `NewPool(lc fx.Lifecycle, cfg config.Config) (*pgxpool.Pool, error)` calls `New(context.Background(), cfg)` and registers only an `OnStop` close hook — note this moves the ping from inside `OnStart` to pool-construction time (strictly earlier-failing, same net effect). `internal/db/module.go` needs no change.

#### 3. Migrations

**Files**: `migrations/0001_board_editions.up.sql` / `.down.sql`, `migrations/0002_problems.up.sql` / `.down.sql`, `migrations/0003_problem_configurations.up.sql` / `.down.sql`, `migrations/0004_holds.up.sql` / `.down.sql`, `migrations/0005_problem_moves.up.sql` / `.down.sql`, `migrations/embed.go`

**Intent**: Establish the full catalog schema, in FK dependency order: board editions (seeded with the 4 known boards) → problems → per-angle configurations → holds → parsed moves (FK to holds).

**Contract**:
- `board_editions`: `holdsetup SMALLINT PRIMARY KEY, name TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now()`; seeded via `INSERT` in the same migration with rows `(1, '2016'), (15, 'Masters 2017'), (17, 'Masters 2019'), (21, '2024')`.
- `problems`: surrogate `id BIGSERIAL PRIMARY KEY`, `external_id INTEGER NOT NULL`, `holdsetup SMALLINT NOT NULL REFERENCES board_editions(holdsetup)`, `name TEXT NOT NULL`, `setter TEXT`, `setby_id TEXT`, `climb_method TEXT`, `holdsets TEXT`, `coordinates TEXT`, `beta_videos INTEGER NOT NULL DEFAULT 0`, `moves_raw TEXT NOT NULL`, `date_inserted TIMESTAMPTZ`, `date_updated TIMESTAMPTZ`, `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`, `UNIQUE (holdsetup, external_id)`; index on `holdsetup`.
- `problem_configurations`: `id BIGSERIAL PRIMARY KEY`, `problem_id BIGINT NOT NULL REFERENCES problems(id) ON DELETE CASCADE`, `holdsetup SMALLINT NOT NULL REFERENCES board_editions(holdsetup)`, `api_id INTEGER NOT NULL`, `angle SMALLINT NOT NULL`, `primary_angle SMALLINT`, `grade TEXT NOT NULL`, `user_grade TEXT`, `user_rating INTEGER`, `is_benchmark BOOLEAN NOT NULL DEFAULT false`, `is_competition_problem BOOLEAN NOT NULL DEFAULT false`, `is_primary BOOLEAN NOT NULL DEFAULT false`, `repeats INTEGER NOT NULL DEFAULT 0`, `comment TEXT`, `date_updated TIMESTAMPTZ`, `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`, `UNIQUE (problem_id, api_id)`; index on `(holdsetup, angle, grade)`.
- `holds`: `PRIMARY KEY (holdsetup, grid_ref)`, `holdsetup SMALLINT NOT NULL REFERENCES board_editions(holdsetup)`, `grid_ref TEXT NOT NULL`, `primary_type TEXT` (nullable until tagged), `modifiers TEXT[] NOT NULL DEFAULT '{}'`, `is_tagged BOOLEAN NOT NULL DEFAULT false`, `first_seen_at TIMESTAMPTZ NOT NULL DEFAULT now()`, `tagged_at TIMESTAMPTZ`, `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`.
- `problem_moves`: `id BIGSERIAL PRIMARY KEY`, `problem_id BIGINT NOT NULL REFERENCES problems(id) ON DELETE CASCADE`, `holdsetup SMALLINT NOT NULL`, `seq INTEGER NOT NULL`, `move_type TEXT NOT NULL`, `grid_ref TEXT NOT NULL`, `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`, `UNIQUE (problem_id, seq)`, `FOREIGN KEY (holdsetup, grid_ref) REFERENCES holds (holdsetup, grid_ref)`; indexes on `(holdsetup, grid_ref)` and `problem_id`.
- `embed.go`: `package migrations; import "embed"; //go:embed *.sql\nvar FS embed.FS`.

#### 4. Migrate CLI

**File**: `cmd/migrate/main.go`

**Intent**: One-shot, non-fx binary applying/rolling back migrations against `DATABASE_URL`.

**Contract**: `main()` calls `config.Load()`; on error, print to stderr and `os.Exit(1)`. Opens `sql.Open("pgx", cfg.DatabaseURL)`, builds `pgxv5.WithInstance(db, &pgxv5.Config{})`, `iofs.New(migrations.FS, ".")`, `migrate.NewWithInstance("iofs", src, "pgx5", dbDriver)`. `os.Args[1]` selects `up` (→ `m.Up()`) or `down` (→ `m.Steps(-1)`); `migrate.ErrNoChange` is treated as success, not an error.

### Success Criteria:

#### Automated Verification:

- Build passes: `go build ./...`
- Vet passes: `go vet ./...`
- Lint passes: `golangci-lint run`

#### Manual Verification:

- `go run ./cmd/migrate up` against the real Supabase `DATABASE_URL` creates all 5 tables plus golang-migrate's own `schema_migrations` table.
- `board_editions` contains exactly the 4 seeded rows (1, 15, 17, 21) with correct labels.
- `go run ./cmd/migrate down` drops `problem_moves` cleanly (one step), then `up` reapplies it, proving the down migration is correct.

---

## Phase 2: Pure parsing/domain logic (no DB)

### Overview

Build and unit-test everything that doesn't touch Postgres: JSON decoding + the active/deleted filter, moves-string parsing, grid-ref parsing/sorting, angle extraction.

### Changes Required:

#### 1. Domain structs + filter

**File**: `internal/catalog/problem.go`

**Intent**: Decode the source JSON shape and implement the ingestion filter.

**Contract**: `type ExportFile struct { Holdsetup int; Count int; Problems []Problem }`; `type Problem struct { ID int; Name string; DateInserted, DateUpdated, DateDeleted *time.Time; Holdsetup int; Active bool; ClimbMethod, SetbyID, Setter, Moves string; Coordinates *string; Holdsets string; BetaVideos int; Configurations []Configuration }` with matching `json:` tags; `type Configuration struct { APIID int; Grade, UserGrade string; UserRating int; DateUpdated, DateDeleted *time.Time; IsBenchmark, IsCompetitionProblem, IsPrimary bool; Comment *string; Repeats int; Configuration, PrimaryAngle string }`. `func ShouldIngest(p Problem) bool { return p.Active && p.DateDeleted == nil }`.

#### 2. Moves parser

**File**: `internal/catalog/moves.go`

**Intent**: Turn the pipe/tilde-delimited `moves` string into ordered, typed tokens.

**Contract**: `type Move struct { Seq int; Type string; GridRef string }`; `func ParseMoves(raw string) ([]Move, error)` — split on `|`, each non-empty token split on `~` expecting exactly `type, gridRef, ""`; returns a wrapped error naming the offending token on malformed input. `Seq` is the 0-based index in the raw string (source order, not inferred climbing-path order — the source data can contain two `s~` tokens for a two-hand start).

#### 3. Grid ref parsing/sorting

**File**: `internal/catalog/gridref.go`

**Intent**: Parse `"C5"`-style refs into a sortable/validatable form, shared by move validation and inventory-CSV sort order.

**Contract**: `func ParseGridRef(s string) (col string, row int, err error)` — lenient shape validation (one or more uppercase letters + 1-2 digit number); out-of-expected-range values (letter beyond `K`, row beyond `18`) are a soft warning, not a hard error, since real production data shouldn't be rejected mid-run on an unexpected edge. `func Less(a, b string) bool` for sorting by `(col, row)`.

#### 4. Angle parser

**File**: `internal/catalog/angle.go`

**Intent**: Turn `"25°"`/`"40°"` into `int` degrees.

**Contract**: `func ParseAngle(s string) (int, error)` — strips a trailing `°` (tolerates a missing one), parses the remainder as an integer.

#### 5. Unit tests

**Files**: `internal/catalog/problem_test.go`, `internal/catalog/moves_test.go`, `internal/catalog/gridref_test.go`, `internal/catalog/angle_test.go`

**Intent**: Table-driven coverage against small synthetic fixtures (not the real 96MB files).

**Contract**: `problem_test.go` — `ShouldIngest` cases (active+not-deleted → true; inactive → false; deleted → false) and a JSON-unmarshal case using a trimmed version of the real example record, confirming field mapping including `Active`'s casing and null `DateDeleted`. `moves_test.go` — valid multi-token string, single-start/single-end, two-start string, malformed token (missing `~`) → error. `gridref_test.go` — `"C5"` → `("C", 5)`; `"K18"` → `("K", 18)`; sort ordering `A1 < A2 < ... < A18 < B1`. `angle_test.go` — `"25°"` → 25, `"40°"` → 40, malformed → error.

### Success Criteria:

#### Automated Verification:

- Tests pass: `go test ./internal/catalog/...`
- Build passes: `go build ./...`
- `go vet ./...` and `golangci-lint run` pass

#### Manual Verification:

- N/A — pure logic, fully covered by automated tests.

---

## Phase 3: Ingestion orchestration + `catalog ingest`

### Overview

Wire the Phase 2 parsers to Postgres: per-problem transactional upsert of problems/configs/moves, with hold auto-discovery ahead of the FK insert.

### Changes Required:

#### 1. Ingester

**File**: `internal/catalog/ingest.go`

**Intent**: Stream-decode one export file, filter, and write each qualifying problem transactionally.

**Contract**: `type Ingester struct { pool *pgxpool.Pool }`; `func NewIngester(pool *pgxpool.Pool) *Ingester`; `func (i *Ingester) IngestFile(ctx context.Context, path string, dryRun bool) (Summary, error)`. Per problem (skipping non-`ShouldIngest` ones), inside one `pgx.Tx`: (a) `INSERT INTO problems (...) VALUES (...) ON CONFLICT (holdsetup, external_id) DO UPDATE SET ... RETURNING id`; (b) `DELETE FROM problem_configurations WHERE problem_id = $1` then bulk-insert current `configurations[]` (parsing `angle`/`primary_angle` via `ParseAngle`); (c) `ParseMoves(p.Moves)`, collect distinct `(holdsetup, grid_ref)` pairs, `INSERT INTO holds (holdsetup, grid_ref) VALUES (...) ON CONFLICT DO NOTHING` for each, then `DELETE FROM problem_moves WHERE problem_id = $1` and bulk-insert the fresh ordered moves. `dryRun=true` runs steps (a)-(c) parse/validation only, no writes. `Summary` carries counts: problems seen/ingested/skipped-inactive/skipped-deleted, configs written, moves written, new holds discovered, and any parse errors (collected, not fatal to the whole run — one bad problem shouldn't abort ~90K others; errored problem IDs are reported in the summary).

#### 2. CLI wiring

**File**: `cmd/catalog/main.go`

**Intent**: Non-fx dispatch: `config.Load()` → `db.New(ctx, cfg)` → `internal/catalog` call → print `Summary` → exit code reflects whether any problems errored.

**Contract**: `catalog ingest --file <path> [--dry-run]`. An unrecognized `holdsetup` (not one of the 4 seeded boards) is a fatal error for that file, not a per-problem skip — it signals a file/DB mismatch worth stopping for.

### Success Criteria:

#### Automated Verification:

- Build passes: `go build ./...`
- `go vet ./...` and `golangci-lint run` pass

#### Manual Verification:

- `catalog ingest --file <small hand-crafted sample with 2-3 problems, one active, one Active=false, one dateDeleted set> --dry-run` produces the expected skip counts and no DB writes.
- Same file without `--dry-run` lands exactly the expected rows in `problems`/`problem_configurations`/`problem_moves`/`holds`, spot-checked via `psql`.
- Run against all 4 real exports (unzipped from the user's local exports); confirm resulting `problems` counts per board are sane (≤ the raw totals of 92,515 / 36,231 / 73,682 / 57,333, since the Active/dateDeleted filter removes some) and `SELECT COUNT(*) FROM holds WHERE holdsetup = <n>` is a plausible per-board hold count (well under the 11×18=198 max grid size).
- `SELECT DISTINCT move_type FROM problem_moves` reviewed for unexpected codes beyond `s`/`p`/`e`/`l`/`r`/`m` — informational only, no scope change expected.

---

## Phase 4: Hold inventory / load-tags / status tooling

### Overview

Turn the auto-discovered `holds` rows into a bounded, hand-fillable checklist per board, and a way to load the filled checklist back and check completion.

### Changes Required:

#### 1. CSV format + validation (pure)

**File**: `internal/catalog/holdtags.go`

**Intent**: Define the seed-file shape and taxonomy, independent of the DB.

**Contract**: `var AllowedHoldTypes = []string{"crimp", "sloper", "pinch", "jug", "pocket"}` (exactly these 5); `func ValidateHoldType(s string) error`; `type HoldRow struct { GridRef, PrimaryType string; Modifiers []string }`; `func WriteInventoryCSV(w io.Writer, rows []HoldRow) error` (header `grid_ref,primary_type,modifiers`, sorted via `gridref.Less`); `func ReadTagsCSV(r io.Reader) ([]HoldRow, error)` (splits `modifiers` on `;`, trims whitespace; a row with more than 3 modifiers is a soft warning, not an error).

#### 2. Board year identity (external CLI/filename surface)

**File**: `internal/catalog/board.go` (new)

**Intent**: The DB's `holdsetup` codes (`1`, `15`, `17`, `21`) are a correct, clash-free internal board key (verified: `board_editions` seeds exactly 4 rows, one holdsetup per board), but the raw integers are meaningless to a human typing a CLI flag or reading a filename. Since the 4 boards happen to carry 4 distinct years (2016, Masters 2017, Masters 2019, 2024), the bare year is a sufficient, unambiguous, human-readable external identifier — no separate slug scheme needed. This map is the reverse of the existing `KnownBoards` (holdsetup → full display name, e.g. "Masters 2017"), which stays as-is for descriptive labels; `BoardYears`/`ResolveBoardYear` is the new externally-facing identity used everywhere a human supplies or reads a board reference.

**Contract**: `var BoardYears = map[int]string{1: "2016", 15: "2017", 17: "2019", 21: "2024"}`; `func ResolveBoardYear(year string) (holdsetup int, err error)` — reverse lookup; on no match, returns an error listing all 4 valid years (e.g. `catalog: unknown board year %q, expected one of: 2016, 2017, 2019, 2024`).

#### 3. DB-backed hold store

**File**: `internal/catalog/holds.go`

**Intent**: The three DB operations `holds inventory`/`load-tags`/`status` need. Unchanged by the board-year surface change below — these still operate on the real `holdsetup` DB key; only the CLI layer (item 4) translates a year to a holdsetup before calling in.

**Contract**: `type HoldStore struct { pool *pgxpool.Pool }`; `func (s *HoldStore) Inventory(ctx context.Context, holdsetup int) ([]HoldRow, error)` — `SELECT grid_ref, primary_type, modifiers FROM holds WHERE holdsetup = $1`, already-tagged rows come back pre-filled so `WriteInventoryCSV` preserves prior work on re-run. `func (s *HoldStore) ApplyTags(ctx context.Context, holdsetup int, rows []HoldRow) error` — validates every row's `PrimaryType` via `ValidateHoldType` first (collect all errors, return them together, apply nothing on any failure), then in one transaction, for each row with non-blank `PrimaryType`: `UPDATE holds SET primary_type=$1, modifiers=$2, is_tagged=true, tagged_at=now() WHERE holdsetup=$3 AND grid_ref=$4`. `func (s *HoldStore) Status(ctx context.Context, holdsetup *int) ([]BoardStatus, error)` — `SELECT holdsetup, count(*) FILTER (WHERE is_tagged), count(*) FROM holds GROUP BY holdsetup` (optionally filtered), returned per board.

#### 4. CLI wiring

**File**: `cmd/catalog/main.go`

**Intent**: Add the `holds` subcommand tree, identifying boards by year rather than raw `holdsetup` code.

**Contract**: `catalog holds inventory --board <year> --out <path>` (stdout if `--out` omitted); `catalog holds load-tags --file <path> --board <year>`; `catalog holds status [--board <year>]` — e.g. `--board 2016`, `--board 2017`. Each `--board` flag is a `flag.String`, resolved via `catalog.ResolveBoardYear` into the `holdsetup int` that `HoldStore` actually takes; flag help text reads `"board year (2016, 2017, 2019, or 2024)"`. `holds status`'s print line drops the raw `holdsetup %d` from its output and instead prints the year plus `KnownBoards`' full name, e.g. `"2017  Masters 2017: 0 / 198 tagged"` (was `"Masters 2017    (holdsetup 15): 0 / 198 tagged"`).

#### 5. Unit tests

**Files**: `internal/catalog/holdtags_test.go`, `internal/catalog/board_test.go` (new), `internal/catalog/tag_test.go` (new)

**Intent**: Round-trip and validation coverage without a DB.

**Contract**: `holdtags_test.go` — write→read round-trip preserves rows including multi-modifier fields; `ValidateHoldType` accepts all 5 allowed values and rejects garbage; a written inventory of unsorted input grid refs (e.g. `K2`, `A10`, `A2`) comes out in `(col, row)` order, not lexicographic; `ExpandHoldTypeAbbreviation` expands known abbreviations, passes unknown strings through unchanged, matches case-insensitively (see item 6 below). `board_test.go` — `ResolveBoardYear` round-trips all 4 known years (`"2016"`→`1`, `"2017"`→`15`, `"2019"`→`17`, `"2024"`→`21`) and rejects an unknown year (e.g. `"2020"`) with an error listing the valid set. `tag_test.go` — see item 7 below.

#### 6. Hold-type abbreviations (CSV hand-editing)

**File**: `internal/catalog/holdtags.go`

**Intent**: Let a hand-edited CSV cell use a short abbreviation instead of typing `crimp`/`sloper`/etc. in full — hand-typing the full name ~734 times across all 4 boards is real friction.

**Contract**: `var HoldTypeAbbreviations = map[string]string{"cr": "crimp", "sl": "sloper", "pi": "pinch", "ju": "jug", "po": "pocket"}` (2-letter, unambiguous — `pi`/`po` avoid the pinch/pocket clash a 1-letter scheme would hit); `func ExpandHoldTypeAbbreviation(s string) string` — lowercases/trims `s`, returns the canonical name if `s` matches a key, else returns `s` unchanged so `ValidateHoldType`'s existing rejection still fires on genuinely bad input. `ReadTagsCSV` calls this on the primary-type field before storing it into `HoldRow.PrimaryType`, so abbreviations flow transparently through the unchanged `ValidateHoldType`/`ApplyTags` path — no changes needed to either.

#### 7. Interactive hold tagger (`catalog holds tag`)

**Files**: `internal/catalog/tag.go` (new), `internal/catalog/tag_test.go` (new), `internal/catalog/holdtags.go` (small refactor), `cmd/catalog/main.go`, `go.mod`/`go.sum`

**Intent**: A faster alternative to hand-editing the CSV in a spreadsheet for the ~734-row tagging pass — press one key per hold for primary type, up to two more for modifiers, and the tool writes the CSV directly. Explicitly one-shot and deliberately minimal: no DB writes happen here; `catalog holds load-tags` remains the only path that persists tags to Postgres, unchanged.

**Contract**:
- New dependency `golang.org/x/term` (`go get golang.org/x/term`) for raw single-keystroke reads. `term.MakeRaw`/`term.Restore` wrap the read loop; `defer term.Restore(...)` guarantees the operator's terminal is never left broken by a panic, error, or early quit. `RunInteractiveTag` checks `term.IsTerminal(fd)` up front and errors clearly if stdin isn't a real TTY (e.g. piped input).
- `internal/catalog/holdtags.go` gains `func SortHoldRows(rows []HoldRow)`, extracted from `WriteInventoryCSV`'s existing sort-by-`gridref.Less` call (which now just calls it), so the interactive tagger walks holds in the same order the CSV uses.
- `internal/catalog/tag.go`: primary-type legend `1=crimp 2=sloper 3=pinch 4=jug 5=pocket`, printed at every prompt — digits, not letters, so pinch/pocket never clash and nothing needs memorizing. Modifier legend `1=sharp 2=rounded 3=incut 4=sloping 5=small 6=large 7=positive 8=textured`; any key outside `1`-`8` (including Enter/`0`) ends the modifier sub-loop, which is hard-capped at 2 iterations regardless of further input. `q`, `Q`, or Ctrl+C (arrives as byte `0x03` since raw mode disables signal generation) ends the whole session early — whatever's been tagged so far is already on disk (see the `save` behavior below), nothing is lost.
- Core loop is a pure, testable function decoupled from terminal plumbing: `func runTagLoop(rows []HoldRow, keys io.Reader, log io.Writer, save func([]HoldRow) error) ([]HoldRow, error)`. Reads one byte at a time from `keys`; rows with a non-blank `PrimaryType` are skipped without prompting; `save` is called after every hold (not just at the end), so the CSV on disk is never more than one hold stale. `RunInteractiveTag(ctx context.Context, store *HoldStore, holdsetup int, outPath string) error` is the outer wrapper: real stdin in raw mode, a `save` closure calling `WriteInventoryCSV` against `outPath`.
- Seeding: `RunInteractiveTag` first calls `store.Inventory(ctx, holdsetup)` (same DB call `holds inventory` uses, so holds discovered by a later re-ingest are always present); if `outPath` already exists on disk, it's read via `ReadTagsCSV` and any row with a non-blank `PrimaryType` there overrides the DB-seeded row for that `grid_ref` — so progress from an earlier `holds tag` run that hasn't been pushed to the DB yet via `load-tags` is never lost or re-prompted.
- `cmd/catalog/main.go`: new `catalog holds tag --board <year> [--out <path>]` subcommand, `--board` resolved via the existing `catalog.ResolveBoardYear` (same as `inventory`/`load-tags`/`status`). `--out` defaults to `migrations/seed/holds/<year>.csv` — one run per board outputs to the correct CSV with zero flags needed for the common case.
- `tag_test.go`: table-driven against `runTagLoop` with an in-memory `[]HoldRow` seed and a `strings.NewReader` standing in for the keystroke stream — cases: a full pass tags every row via digit keys; pressing more than 2 modifier digit keys in a row only keeps the first two; `q` mid-pass stops early and the returned rows are the untouched rest merged with whatever was tagged before the quit; a row that's already tagged in the seed is never prompted.

### Success Criteria:

#### Automated Verification:

- Tests pass: `go test ./internal/catalog/...`
- Build passes: `go build ./...`
- `go vet ./...` and `golangci-lint run` pass

#### Manual Verification:

- After Phase 3's real-data ingestion, generate inventory CSVs for all 4 boards; row counts match `SELECT COUNT(*) FROM holds WHERE holdsetup = <n>` from Phase 3's manual check.
- Spot-check sort order in one generated CSV (e.g. `A1` through `A18` before any `B*` row).
- `catalog holds status` (no `--board`) lists all 4 boards with `0 / <total>` tagged before any tagging has happened.
- Hand-edit one row of a generated CSV to use an abbreviation (e.g. `cr`), run `load-tags`, confirm `holds inventory` shows it applied as the canonical full name (`crimp`).
- Run `catalog holds tag --board <year>` against one real board: confirm the digit-key legends behave as documented, Ctrl+C mid-session leaves a CSV on disk with only the holds tagged so far filled in, re-running the command skips those already-tagged holds and only prompts the remainder, and pressing 3+ modifier keys in a row is capped at 2.

---

## Phase 5: Manual hold tagging (the actual data-entry pass)

### Overview

The bulk of the required "full coverage" gate — hand-fill all 4 CSVs and load them back. Mostly non-code; included as its own phase because it's a required, trackable gate before the slice is done.

### Changes Required:

#### 1. Hand-fill CSVs

**Files**: `migrations/seed/holds/2016.csv`, `2017.csv`, `2019.csv`, `2024.csv`

**Intent**: Fill `primary_type` (and up to ~3 `modifiers`) for every row generated in Phase 4, using whatever board photos/reference the user has. Filenames identify boards by year, not raw `holdsetup` code, matching Phase 4's `--board <year>` CLI surface. The 4 CSVs already generated locally under the old `holdsetup-<n>.csv` names are untracked and still fully blank (no hand-tagging has happened) — safe to rename or regenerate fresh under the new names with no data loss.

**Contract**: Committed to the repo once filled (unlike the source JSON exports, which stay external) — these are the actual authored artifact this slice produces.

#### 2. Load tags back

**Intent**: Apply each filled CSV via `catalog holds load-tags`.

**Contract**: `go run ./cmd/catalog holds load-tags --file migrations/seed/holds/2016.csv --board 2016` (repeat ×4, for 2017/2019/2024).

### Success Criteria:

#### Automated Verification:

- N/A — this phase is data entry, not code.

#### Manual Verification:

- `go run ./cmd/catalog holds status` reports `<total> / <total>` tagged for all 4 boards (0 untagged).
- Spot-check a handful of tagged rows against the physical/photographed board layout for plausibility.

---

## Phase 6: Runbook doc

### Overview

A short, committed note so this ingestion sequence is repeatable later (e.g., if MoonBoard publishes a 5th board edition) without re-deriving the command order from this plan file.

### Changes Required:

#### 1. Runbook

**File**: `migrations/seed/holds/README.md`

**Intent**: Document the exact command sequence (migrate up → ingest ×4 → inventory ×4 → hand-fill → load-tags ×4 → status), identifying boards by year (`--board 2016`/`2017`/`2019`/`2024`) throughout rather than raw `holdsetup` codes, plus the hold taxonomy list and where source JSON exports are expected to live (external, path passed via `--file`).

**Contract**: Plain markdown, no code.

### Success Criteria:

#### Automated Verification:

- N/A.

#### Manual Verification:

- A fresh reader unfamiliar with this plan can follow the README and reproduce the ingestion sequence end-to-end.

---

## Testing Strategy

### Unit Tests:

- `internal/catalog`: moves parsing, grid-ref parsing/sorting, angle extraction, `ShouldIngest` filter + JSON decode, CSV round-trip + hold-type validation — all against small synthetic fixtures, no DB, no real export files.

### Integration Tests:

- Deliberately not built in this slice. `ingest.go`/`holds.go` (the DB-touching half of `internal/catalog`) are covered only by the Manual Verification steps in Phases 3-5 against the real Supabase instance. A future slice wanting CI coverage here would need a Docker-based Postgres test helper — a documented future option, not built now.

### Manual Testing Steps:

1. Apply migrations against the real Supabase `DATABASE_URL`.
2. Ingest all 4 real exports; sanity-check row counts and `move_type` distinct values.
3. Generate, hand-fill, and load back all 4 hold-tag CSVs.
4. Confirm `catalog holds status` shows full coverage.

## Performance Considerations

One transaction per problem across ~260K total problems could be slow over Supabase's network (likely low tens of minutes worst case). Accepted for a one-shot local tool; batching commits every N problems is a small, low-risk follow-up to `ingest.go` if it proves too slow, not a redesign.

## Migration Notes

This is the repo's first schema — no existing data to migrate. `golang-migrate` is a new dependency, introduced by this change; `cmd/migrate` is not yet wired into Railway's deploy pipeline (applied manually for this slice).

## References

- Roadmap: `context/foundation/roadmap.md` (F-02)
- PRD: `context/foundation/prd.md` (FR-015, FR-007, Open Question 1)
- Existing pgx pool: `internal/db/pool.go`
- Existing config: `internal/config/config.go`
- Existing fx.Module pattern: `internal/auth/module.go`, `internal/logging/module.go`

## Progress

> Convention: `- [ ]` pending, `- [x]` done. Append ` — <commit sha>` when a step lands. Do not rename step titles. See `references/progress-format.md`.

### Phase 1: Migration infrastructure + schema

#### Automated

- [x] 1.1 Build passes: `go build ./...` — 3a19ea5
- [x] 1.2 Vet passes: `go vet ./...` — 3a19ea5
- [x] 1.3 Lint passes: `golangci-lint run` — 3a19ea5

#### Manual

- [x] 1.4 `migrate up` against real Supabase creates all 5 tables + seeds 4 board rows — 3a19ea5
- [x] 1.5 `migrate down` then `up` proves the down migration works — 3a19ea5

### Phase 2: Pure parsing/domain logic (no DB)

#### Automated

- [x] 2.1 Tests pass: `go test ./internal/catalog/...` — 748c078
- [x] 2.2 Build passes: `go build ./...` — 748c078
- [x] 2.3 `go vet ./...` and `golangci-lint run` pass — 748c078

### Phase 3: Ingestion orchestration + `catalog ingest`

> Implementation note (added during 2026-08-09 impl review, F6): the actual `internal/catalog/ingest.go` batches ~200 problems per pipelined `pgx.Batch`/`SendBatch` transaction, with a per-problem sequential fallback if a batch fails, rather than the literal one-transaction-per-problem text in this phase's Contract. This is the "batching commits every N problems" follow-up the plan's own Performance Considerations section pre-authorized as low-risk, not a redesign — recorded here so a future reader doesn't have to diff code against the Contract text to learn it.

#### Automated

- [x] 3.1 Build passes: `go build ./...` — 94b241f
- [x] 3.2 `go vet ./...` and `golangci-lint run` pass — 94b241f

#### Manual

- [x] 3.3 Dry-run + real-run against small hand-crafted fixture, spot-checked via `psql` — 94b241f
- [x] 3.4 All 4 real exports ingested; row counts sane — 94b241f
- [x] 3.5 `holds` counts per board plausible (≤ 198 max grid size) — 94b241f
- [x] 3.6 `SELECT DISTINCT move_type` reviewed for unexpected codes — 94b241f

### Phase 4: Hold inventory / load-tags / status tooling

> Reopened, now resolved: the `--board <int>` / `holdsetup %d` CLI surface verified by `b87b8ca` was replaced with a board-year identifier (`--board 2016`/`2017`/`2019`/`2024`, see item 2), plus hold-type abbreviations (item 6) and the interactive `catalog holds tag` tool (item 7). Reverification against the real DB also found and fixed a bug in `ApplyTags`: a nil `Modifiers` slice was sent to Postgres as `NULL` instead of `'{}'`, violating `modifiers`' `NOT NULL` constraint.

#### Automated

- [x] 4.1 Tests pass: `go test ./internal/catalog/...` — b413bb5
- [x] 4.2 Build passes: `go build ./...` — b413bb5
- [x] 4.3 `go vet ./...` and `golangci-lint run` pass — b413bb5

#### Manual

- [x] 4.4 All 4 real inventory CSVs generated, row counts match `holds` table — b413bb5
- [x] 4.5 Sort order confirmed in a generated CSV — b413bb5
- [x] 4.6 `catalog holds status` lists all 4 boards at 0 tagged before tagging starts — b413bb5
- [x] 4.7 Hand-typed abbreviation (e.g. `cr`) round-trips through `load-tags` to the canonical full name — b413bb5
- [x] 4.8 `catalog holds tag` verified: legends work, Ctrl+C preserves partial CSV, rerun skips already-tagged holds, modifier cap of 2 enforced — b413bb5

### Phase 5: Manual hold tagging

> Scope revised 2026-08-09 (see `change.md`): the "all 4 boards" gate is relaxed to "2016 + 2024 tagged and loaded now, Masters 2017 + Masters 2019 explicitly deferred". 5.1/5.2 below are literally about all 4 boards and stay unchecked to reflect that honestly; the 2-board subset is verified done as of `b413bb5`+this session (140/140 and 198/198 in `holds`, confirmed via `catalog holds status`). 2017/2019 pick up later via `catalog holds tag --board 2017`/`2019` + `load-tags`, documented in the Phase 6 runbook — no plan reopen needed.

#### Manual

- [ ] 5.1 All 4 CSVs hand-filled and loaded via `holds load-tags`
- [ ] 5.2 `catalog holds status` shows full coverage (0 untagged) on all 4 boards
- [x] 5.3 Spot-check tagged rows against physical/photographed board layout (2016 + 2024 subset) — a8ce9d5

### Phase 6: Runbook doc

#### Manual

- [x] 6.1 Runbook written and confirmed reproducible by a fresh reader — 2459448

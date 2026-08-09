# Catalog ingestion + hold-tagging runbook

Reproducible command sequence for loading a MoonBoard problem catalog and
hand-tagging every physical hold with a type. Written so this doesn't need
to be re-derived from `context/changes/catalog-data-foundation/plan.md` if
MoonBoard ever publishes a 5th board edition.

Every command below identifies a board by its **year** (`2016`, `2017`,
`2019`, `2024`), never by the raw internal `holdsetup` code. The mapping
lives in `internal/catalog/board.go` (`BoardYears`).

## Prerequisites

- `DATABASE_URL` set (via `.env`, auto-loaded by `cmd/catalog`/`cmd/migrate`)
  pointing at the target Postgres instance.
- The 4 static MoonBoard JSON exports (2016, Masters 2017, Masters 2019,
  2024), downloaded separately. These are **not committed to the repo** —
  pass their local path via `--file` at ingest time. Each export declares
  its own `holdsetup`, so `catalog ingest` doesn't take a `--board` flag.

## 1. Apply the schema

```sh
go run ./cmd/migrate up
```

Creates `board_editions` (seeded with the 4 known boards), `problems`,
`problem_configurations`, `holds`, and `problem_moves`.

## 2. Ingest each export

Repeat once per export file:

```sh
go run ./cmd/catalog ingest --file /path/to/export-2016.json
go run ./cmd/catalog ingest --file /path/to/export-masters2017.json
go run ./cmd/catalog ingest --file /path/to/export-masters2019.json
go run ./cmd/catalog ingest --file /path/to/export-2024.json
```

Add `--dry-run` first against an unfamiliar export to sanity-check parse
counts before writing. Ingestion auto-discovers every `(board, grid_ref)`
pair actually used by a move and inserts an untagged row into `holds` for
it — this is what seeds the inventory the next step reads from.

## 3. Generate a hold-tag CSV per board

```sh
go run ./cmd/catalog holds inventory --board 2016 --out migrations/seed/holds/2016.csv
go run ./cmd/catalog holds inventory --board 2017 --out migrations/seed/holds/2017.csv
go run ./cmd/catalog holds inventory --board 2019 --out migrations/seed/holds/2019.csv
go run ./cmd/catalog holds inventory --board 2024 --out migrations/seed/holds/2024.csv
```

Rows come out sorted by grid ref (column then row); already-tagged rows are
pre-filled, so re-running after a future re-ingest only appends new blank
rows — prior tagging work is never lost.

## 4. Hand-fill each CSV

Fill the `primary_type` column (required) and up to 3 `;`-separated
`modifiers` (optional) for every row. Two ways to do this:

**Spreadsheet / hand-edit.** Open the CSV directly. `primary_type` accepts
either the full name or a 2-letter abbreviation (expanded automatically on
load):

| Primary type | Abbreviation |
| --- | --- |
| crimp | `cr` |
| sloper | `sl` |
| pinch | `pi` |
| jug | `ju` |
| pocket | `po` |

**Interactive tagger.** One keystroke per hold, faster for a full board
pass:

```sh
go run ./cmd/catalog holds tag --board 2016
```

Defaults to writing `migrations/seed/holds/<year>.csv`. Legend is printed
at every prompt:

- Primary type (required, one digit): `1`=crimp `2`=sloper `3`=pinch
  `4`=jug `5`=pocket
- Modifiers (optional, up to 2 digits, any other key moves on):
  `1`=sharp `2`=rounded `3`=incut `4`=sloping `5`=small `6`=large
  `7`=positive `8`=textured
- `q` / Ctrl+C ends the session early. The CSV is written after every
  hold, so nothing tagged so far is lost. Re-running the command skips
  holds that are already tagged and only prompts the remainder.

## 5. Load the filled CSV back into Postgres

```sh
go run ./cmd/catalog holds load-tags --file migrations/seed/holds/2016.csv --board 2016
go run ./cmd/catalog holds load-tags --file migrations/seed/holds/2017.csv --board 2017
go run ./cmd/catalog holds load-tags --file migrations/seed/holds/2019.csv --board 2019
go run ./cmd/catalog holds load-tags --file migrations/seed/holds/2024.csv --board 2024
```

Rows with a blank `primary_type` are left untouched — loading a
partially-filled CSV is safe and expected mid-pass.

## 6. Confirm coverage

```sh
go run ./cmd/catalog holds status
```

Prints `<tagged> / <total>` per board. `--board <year>` filters to one.

## Current status

As of 2026-08-09: `2016` and `2024` are fully tagged and loaded
(140/140, 198/198). `2017` (Masters 2017) and `2019` (Masters 2019) are
ingested but untagged (0/198 each) — deferred by explicit decision, see
`context/changes/catalog-data-foundation/change.md`. To finish them later,
run step 4 (`catalog holds tag --board 2017` / `2019`) and step 5
(`load-tags`) for each — no other step in this runbook needs to be
repeated.

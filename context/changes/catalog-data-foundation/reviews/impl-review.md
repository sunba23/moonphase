<!-- IMPL-REVIEW-REPORT -->
# Implementation Review: Catalog Data Foundation

- **Plan**: context/changes/catalog-data-foundation/plan.md
- **Scope**: Full plan, Phases 1–6
- **Date**: 2026-08-09
- **Verdict**: NEEDS ATTENTION
- **Findings**: 0 critical, 1 warning, 5 observations

## Verdicts

| Dimension | Verdict |
|-----------|---------|
| Plan Adherence | WARNING |
| Scope Discipline | PASS |
| Safety & Quality | WARNING |
| Architecture | PASS |
| Pattern Consistency | WARNING |
| Success Criteria | PASS |

## Findings

### F1 — ApplyTags silently no-ops on a grid_ref that doesn't match any row

- **Severity**: ⚠️ WARNING
- **Impact**: 🔎 MEDIUM — real tradeoff; pause to reason through it
- **Dimension**: Safety & Quality
- **Location**: internal/catalog/holds.go:90-98 (ApplyTags)
- **Detail**: The per-row `UPDATE ... WHERE holdsetup = $3 AND grid_ref = $4` doesn't check `RowsAffected`. A CSV row with a typo'd `grid_ref`, or a grid_ref for a board that hasn't been ingested yet, matches zero rows and produces no error — `load-tags` reports "applied tags: N" counted from the CSV, not confirmed DB writes, so a silently-dropped tag looks identical to a successful one.
- **Fix A ⭐ Recommended**: Sum `RowsAffected` across the loop; if it doesn't equal the number of non-blank rows, return an error naming the mismatched grid_refs before `Commit`.
  - Strength: Fails closed — a typo can never silently vanish; matches `ApplyTags`' existing "validate everything, apply nothing on failure" philosophy for the primary-type check.
  - Tradeoff: A single bad row in an otherwise-good 198-row load now blocks the whole load until fixed, rather than applying the other 197.
  - Confidence: HIGH — the fix is a small, local addition to the existing loop; no signature changes.
  - Blind spot: Haven't checked whether any current CSV rows reference holds outside the DB's auto-discovered inventory (unlikely, since CSVs are generated from `holds inventory`).
- **Fix B**: Collect grid_refs with zero `RowsAffected` and print them as a warning list after commit, without failing the load.
  - Strength: Preserves today's "load what you can" UX; doesn't block on one bad row.
  - Tradeoff: A typo can still be missed if the operator doesn't read the warning output.
  - Confidence: MEDIUM — plausible but weaker safety guarantee than Fix A.
  - Blind spot: None significant.
- **Decision**: FIXED via Fix A. `internal/catalog/holds.go`: `ApplyTags` now tracks `RowsAffected() == 0` per row and errors naming the unmatched grid_refs before `Commit`, applying nothing on any mismatch. Verified against the real DB: a bad `grid_ref` (`ZZ99`) now errors with `applied nothing` and leaves existing tags untouched. — efc10aa

### F2 — Interactive tagger doesn't restore the terminal on an external signal

- **Severity**: 👁️ OBSERVATION
- **Impact**: 🏃 LOW — quick decision; fix is obvious and narrowly scoped
- **Dimension**: Safety & Quality
- **Location**: internal/catalog/tag.go (RunInteractiveTag, ~line 214)
- **Detail**: `defer term.Restore(fd, oldState)` covers panics, errors, and normal quit paths, but not an external SIGTERM/SIGHUP killing the process mid-session — the operator's shell is left in raw mode until they run `stty sane`. Low-severity since this is a local one-shot operator tool.
- **Fix**: Add a `signal.Notify` handler for SIGTERM/SIGINT that calls `term.Restore` before re-raising/exiting, if this becomes a recurring annoyance in practice.
- **Decision**: FIXED. `internal/catalog/tag.go` `RunInteractiveTag`: added a `signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGHUP)` goroutine that calls `term.Restore` before `os.Exit(1)`. — efc10aa

### F3 — ApplyTags' validate-then-write-nothing invariant has no test coverage

- **Severity**: 👁️ OBSERVATION
- **Impact**: 🏃 LOW — quick decision; fix is obvious and narrowly scoped
- **Dimension**: Pattern Consistency
- **Location**: internal/catalog/holds.go:65-77 (ApplyTags)
- **Detail**: The plan's contract explicitly requires "collect all errors, apply nothing on any failure." This invariant has no test, unlike the repo's otherwise dense table-driven coverage for load-bearing logic (e.g. holdtags_test.go, tag_test.go).
- **Fix**: Add a table-driven test (would need a DB fixture or an interface seam) confirming a mixed valid/invalid batch applies nothing.
- **Decision**: FIXED, partially. Extracted the validation phase of `ApplyTags` into a pure, DB-independent `validateHoldRows(rows []HoldRow) error` (the "collect all errors, apply nothing" guarantee lives entirely here, before any transaction opens). Added `internal/catalog/holds_test.go` with table-driven coverage: all-valid, blank-primary-type-is-untagged, one-bad-row-fails-the-batch, and multiple-bad-rows-all-named-in-the-error. The DB-write half of the invariant (nothing actually lands in Postgres on failure) remains covered only by manual verification, per the plan's own "Deliberately not built in this slice" stance on DB-backed integration tests — a Docker-based Postgres test helper is still the documented future option for that half. — efc10aa

### F4 — ingest.go discards batch reader Close() errors

- **Severity**: 👁️ OBSERVATION
- **Impact**: 🏃 LOW — quick decision; fix is obvious and narrowly scoped
- **Dimension**: Pattern Consistency
- **Location**: internal/catalog/ingest.go:~303 (batchUpsertProblems, batchWriteChildren)
- **Detail**: `br.Close()`'s error return is discarded in both functions, inconsistent with the codebase's otherwise exhaustive `fmt.Errorf("...: %w", err)` wrapping convention. Low likelihood of masking a real failure since prior `Exec`/`Scan` calls in the same batch would typically surface it first.
- **Fix**: Wrap and return (or log) `br.Close()`'s error at both call sites.
- **Decision**: FIXED. Both `batchUpsertProblems` and `batchWriteChildren` now use named returns and a `defer` that sets `err` from `br.Close()` when no earlier error already claimed the return value. — efc10aa

### F5 — DB connection construction deviates from the plan's Contract text, undocumented

- **Severity**: 👁️ OBSERVATION
- **Impact**: 🏃 LOW — quick decision; fix is obvious and narrowly scoped
- **Dimension**: Plan Adherence
- **Location**: internal/db/pool.go, cmd/migrate/main.go
- **Detail**: Phase 1's contract specified `pgxpool.New` + `Ping` and `sql.Open("pgx", ...)`. The actual code uses `pgxpool.ParseConfig`/`NewWithConfig` and `pgx.ParseConfig`/`stdlib.OpenDB`, both forcing `QueryExecModeSimpleProtocol` — a reasonable Supabase PgBouncer-compatibility fix, functionally a superset of the same contract (same signatures, same error wrapping), but never written back into the plan text or a Progress note the way Phase 4's reopen was.
- **Fix**: Add a short note to Phase 1's "Key Discoveries" (or a new note above its Progress block, matching the style already used for Phase 4's reopen) recording the PgBouncer/simple-protocol workaround.
- **Decision**: SKIPPED. User judged the code self-explanatory as-is.

### F6 — ingest.go batches transactions instead of the literal one-per-problem contract

- **Severity**: 👁️ OBSERVATION
- **Impact**: 🏃 LOW — quick decision; fix is obvious and narrowly scoped
- **Dimension**: Plan Adherence
- **Location**: internal/catalog/ingest.go
- **Detail**: Phase 3's contract states one transaction per problem. The actual implementation batches ~200 problems per pipelined transaction with a per-problem sequential fallback on batch failure. This is pre-authorized by the plan's own "Performance Considerations" section ("batching commits every N problems is a small, low-risk follow-up... not a redesign"), so it's a sanctioned evolution, not an error — just never reflected back into the plan text.
- **Fix**: Add a short note to Phase 3's "Key Discoveries" recording the batching approach actually used, so a future reader doesn't need to diff code against the literal Contract text to learn this.
- **Decision**: FIXED. Added an implementation note above Phase 3's Progress checklist in plan.md recording the actual batched-transaction approach.

---
change_id: catalog-data-foundation
title: Catalog data foundation
status: impl_reviewed
created: 2026-08-03
updated: 2026-08-09
archived_at: null
---

## Notes

Roadmap ID: F-02 (foundation). Outcome: a queryable MoonBoard problem catalog exists in Postgres, ingested from static JSON exports covering all 4 board editions (2016, 2024, Masters 2017, Masters 2019 — 259,761 problems total), with every physical hold across all 4 boards manually tagged with a hold type. No HTTP endpoints, no recommendation logic — schema + one-shot ingestion tooling only. PRD refs: FR-015, FR-007. Unlocks S-03, S-04.

Catalog source changed mid-planning from the roadmap's original `CSTDev/moonapi` to static exports the user already had — see `context/foundation/roadmap.md` F-02 entry and `context/foundation/prd.md` Open Question 1 for the updated resolution and reasoning. This is the PRD's own pre-approved fallback tier, not a deviation from it.

Full hold-tag coverage across all 4 boards is a required completion gate for this slice (explicit user decision, not deferred).

**Revised 2026-08-09**: the coverage gate is relaxed from "all 4 boards" to "at least 2 boards, tagged and loaded, proving the multi-board schema/tooling actually works end-to-end" — explicit user decision. 2016 and 2024 are tagged (`migrations/seed/holds/2016.csv`, `2024.csv`) and loaded via `catalog holds load-tags` (140/140 and 198/198 in `holds`). Masters 2017 and Masters 2019 remain untagged (0/198 each) and are left unavailable to the app until hand-tagged in a future pass — that future tagging is not expected to reopen this plan; `catalog holds tag --board 2017` / `2019` plus `load-tags` is the whole remaining procedure, documented in the Phase 6 runbook.

---
change_id: fix-skip-e2e-assertions
title: Repair the stale Bailed-radio E2E assertions left behind by the skip rename
status: implementing
created: 2026-09-12
updated: 2026-09-12
archived_at: null
---

## Notes

`npm run test:e2e` currently fails 3 of 9 tests, all in
`tests/e2e/adaptive-session-loop.spec.ts`, all with the same cause:
`waiting for getByRole('radio', { name: 'Bailed' })` — the spec never caught up
to the `rename-bailed-to-skipped` change (PRD FR-008 update, 2026-09-06,
migration `0011_rename_bailed_to_skipped`), which replaced the old "bailed"
completion status with a rating-free, engine-inert `Skip` button.

Verified this is a test staleness issue, not an app bug: `templates/pages/session.templ`,
`internal/server/session.go`'s `handleSkip`, and its unit test `TestHandleSkip` all
correctly implement the new skip semantics. Only the E2E spec (and one doc
cross-reference in `context/foundation/test-plan.md` §6.5) lags.

No prerequisites — this is a standalone test-only fix.

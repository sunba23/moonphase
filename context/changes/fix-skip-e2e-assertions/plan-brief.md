# Fix Stale Skip E2E Assertions — Plan Brief

> Full plan: `context/changes/fix-skip-e2e-assertions/plan.md`

## What & Why

`npm run test:e2e` fails 3 of 9 tests, all in `tests/e2e/adaptive-session-loop.spec.ts`,
all waiting on a `Bailed` radio that hasn't existed in the UI since the
`rename-bailed-to-skipped` change (PRD FR-008/FR-012, 2026-09-06). The test never
caught up to the rename. This plan rewrites it to match the shipped `Skip` button and
adds the browser-level proof of FR-012's skip-inertness rule that the original rename
never got.

## Starting Point

Verified live: 6/9 e2e tests pass; 3/3 failures share one root cause
(`waiting for getByRole('radio', { name: 'Bailed' })`). Production code is already
correct — `session.templ` renders `Sent`/`Failed` radios plus a `Skip` submit button;
`handleSkip` in `internal/server/session.go` and its unit test `TestHandleSkip`
correctly implement skip-is-inert, skip-excludes-the-problem semantics. Only the e2e
spec (and one doc cross-reference in `test-plan.md` §6.5) lags the rename.

## Desired End State

`npm run test:e2e` is 9/9 green. The spec models completion as it actually ships
(2 radios + 1 Skip button), and explicitly proves — at the browser level — that a
skip doesn't move the grade axis, is never re-shown, and never appears in session
history.

## Key Decisions Made

| Decision | Choice | Why (1 sentence) |
| --- | --- | --- |
| Scope | Repair the 3 tests + add one new skip-inertness assertion | Closes the actual FR-012 coverage gap the rename left open, not just a mechanical patch |
| Skip's grade bound | Assert against the anchor's grade (`g1`), not the immediately-prior pick (`g2`) | `handleSkip`'s anchor search skips over skip rows, so the correct invariant is "same bound the last *rated* result already set," not a fresh one |
| Re-select test | Keep Sent/Failed toggle assertions, add a separate Skip-submits-immediately check | Skip is a one-click submit button now, not a 3rd toggleable radio — its real behavior (fires once, no RPE needed) deserves its own proof |
| Doc sync | Update `test-plan.md` §6.5's stale phrase too | Keeps the doc's own references section trustworthy once the test title changes |
| Verification rigor | Deliberate-break the new assertion before done | Matches this repo's own established practice (`adaptive-main-session-loop/plan.md:968`) and proves the new assertion actually bites |

## Scope

**In scope:**
- Rewrite `tests/e2e/adaptive-session-loop.spec.ts`'s two broken tests
- Add a `skipCurrent` helper and new skip-inertness assertions (grade bound,
  never-reshown, never-in-history)
- Sync `test-plan.md` §6.5's cross-reference

**Out of scope:**
- Any production code change (`internal/server`, `internal/session`,
  `internal/recommender`, `.templ` files) — all already correct
- New spec files
- Broader FR-008/FR-012 audit beyond this one file

## Architecture / Approach

Single-file test rewrite. No app code touched. One new helper mirrors the shape of
the existing `submitResult` helper; new assertions reuse an existing locator pattern
from `past-sessions-history.spec.ts` (the "`N` problems climbed" history link) rather
than inventing a new one.

## Phases at a Glance

| Phase | What it delivers | Key risk |
| --- | --- | --- |
| 1. Rewrite the spec for Skip semantics | 9/9 green e2e suite, FR-012 skip-inertness proven at the browser level, doc synced | Asserting the wrong grade bound (equality to `g2` instead of the anchor `g1`) would make the new test flaky or wrong |

**Prerequisites:** None — standalone test-only fix.
**Estimated effort:** One sitting, single phase.

## Open Risks & Assumptions

- Assumes the candidate pool difference after excluding the skipped problem never
  pushes the post-skip pick *below* what's reachable in the window — the plan only
  asserts the upper bound (matches FR-012's "never harder" framing), consistent with
  how the existing `g1`→`g2` assertion is also upper-bound-only.
- Deliberate-break verification is a manual step — if skipped, the new assertion's
  correctness rests on code review alone.

## Success Criteria (Summary)

- `npm run test:e2e` passes 9/9
- `npm run typecheck` is clean
- No `Bailed` references remain anywhere in `tests/e2e/` or `test-plan.md`
- The new skip-inertness assertion is deliberate-break verified (confirmed red when
  the production inertness flag is temporarily removed, green after revert)

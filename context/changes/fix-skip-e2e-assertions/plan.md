# Fix Stale Skip E2E Assertions Implementation Plan

## Overview

`npm run test:e2e` fails 3 of 9 tests, all in `tests/e2e/adaptive-session-loop.spec.ts`,
all on the same cause: the spec still drives a `Bailed` radio that hasn't existed in
the UI since the `rename-bailed-to-skipped` change. This plan rewrites the spec to
match the shipped `Skip` button, adds the e2e coverage that change should have
carried for FR-012's skip-inertness rule, and syncs one stale doc cross-reference.

## Current State Analysis

Verified live (`npm run test:e2e`, full run against the real app + DB): 6/9 pass,
3/9 fail with `Error: locator.check: Test timeout of 60000ms exceeded. ... waiting
for getByRole('radio', { name: 'Bailed' })`.

This is a business-behavior change the test missed, not an app regression:

- **Production code is correct.** `templates/pages/session.templ:120-134` renders
  only two status radios (`Sent`, `Failed`) plus a `Skip` button
  (`hx-post=".../skip"`, `formnovalidate`, no RPE required). `internal/server/session.go`'s
  `handleSkip` (lines 313-425) records the skip with `rpe = NULL`, flags the shown
  row `Skipped: true` for the recommender, and excludes it from being re-shown. Its
  unit test `TestHandleSkip` (`internal/server/session_handler_test.go:163-247`)
  passes and covers this densely at the handler level.
- **Migration `0011_rename_bailed_to_skipped`** backs this at the DB layer.
- **PRD FR-008 / FR-012** (update 2026-09-06, change `rename-bailed-to-skipped`)
  document the rule: skip carries no RPE, contributes to neither the grade nor the
  hold-type axis, and never appears in session history — but is still excluded from
  being recommended again in that session.
- **Only the E2E layer lags.** `tests/e2e/adaptive-session-loop.spec.ts` still models
  completion as 3 toggleable radios (`Sent`/`Failed`/`Bailed`) and asserts the old
  "bailed never yields a strictly harder pick" rule. `context/foundation/test-plan.md`
  §6.5 has one matching stale phrase in a references list (local-only doc, not shipped).

### Key Discoveries

- `internal/server/session.go:368-375` — `handleSkip`'s anchor search walks the
  shown list for the **last entry with a non-nil RPE**, skipping over any skip rows
  entirely. The grade window for the post-skip pick is derived from that anchor's
  `Result` (its completion + RPE), via the exact same `recommender.PickNext` call
  `handleResult` would have made. Skip does not introduce a new band — it re-resolves
  the *same* band the last rated result already set.
- `internal/recommender/grade_window.go:97-113` — `gradeWindow`'s invariant for
  `bandBackOff`/`bandHold` is `hi` is never above the anchor's own grade (`cur`). The
  candidate pool differs after a skip (the skipped problem's own grade point is now
  excluded), so the post-skip pick is not guaranteed to equal the pre-skip pick's
  grade exactly — only to respect the **same upper bound** (the anchor's grade). The
  rewritten assertion must compare against the anchor's grade, not demand literal
  equality to the immediately-prior pick.
- `tests/e2e/past-sessions-history.spec.ts:142` establishes the existing pattern for
  asserting climbed-count: `page.getByRole('link', { name: /N problems climbed/i })`.
  This is the cleanest way to assert a skip never appears in history, reusing a
  pattern already proven in this repo rather than inventing a new one.
- `context/changes/adaptive-main-session-loop/plan.md:968` documents the prior
  deliberate-break technique for this exact spec: temporarily invert
  `recommender.gradeWindow`'s `backOff` branch, confirm the "never harder" assertion
  goes red, revert. The new skip-inertness assertion needs the same proof, via an
  analogous temporary break (see Critical Implementation Details).

## Desired End State

`npm run test:e2e` is 10/10 green (9 existing + 1 new dedicated Skip-submits-immediately
test). `tests/e2e/adaptive-session-loop.spec.ts` models
completion as it actually ships today (2 toggleable radios + 1 submit-on-click Skip
button), and explicitly proves FR-012's skip-inertness rule (grade axis unaffected,
never re-shown, never in history) at the browser level — closing a coverage gap the
original rename left open. `context/foundation/test-plan.md` §6.5's cross-reference
matches the live test name.

**Verification**: `npm run test:e2e` passes 9/9; `npm run typecheck` is clean; the new
inertness assertion is deliberate-break verified (see Phase 1).

## What We're NOT Doing

- No changes to `internal/server`, `internal/session`, `internal/recommender`, or any
  `.templ` file — production code is already correct.
- No new spec files — this stays inside the existing `adaptive-session-loop.spec.ts`
  and its one `test-plan.md` cross-reference.
- No broader audit of other FR-008/FR-012 skip edge cases beyond this file — server-side
  skip logic is already densely unit-tested (`TestHandleSkip`); this plan closes only
  the e2e gap the rename left.
- No change to the `auth-first-problem.spec.ts` or `past-sessions-history.spec.ts`
  specs — neither references `Bailed` and both already pass.

## Implementation Approach

Single-file, single-pass rewrite. Add one new helper (`skipCurrent`) mirroring the
existing `submitResult` shape. Replace the `Bailed` step in the parametrized "never
strictly harder" test with a real skip, re-deriving the correct bound from the
anchor (not the immediately-prior grade). Rescope the "re-selecting a completion
status" test to the two statuses that are still radios, and add a short
Skip-submits-immediately block to it rather than inventing a new test. Sync the one
doc cross-reference. Verify via a deliberate break before calling it done.

## Phase 1: Rewrite the spec for Skip semantics

### Overview

Fix `tests/e2e/adaptive-session-loop.spec.ts`'s two broken tests, add explicit
skip-inertness coverage, sync `test-plan.md`, and verify.

### Changes Required:

#### 1. Add a `skipCurrent` helper

**File**: `tests/e2e/adaptive-session-loop.spec.ts`

**Intent**: Skip is a one-click submit (`formnovalidate`, no RPE, posts to
`/session/{id}/skip`), not a radio-then-RPE-tap flow like `submitResult`. Give it its
own helper so both rewritten tests share one correct implementation.

**Contract**: `async function skipCurrent(page: Page): Promise<Response>` — clicks
the button named `Skip`, waits for the matching `POST .../skip` response (mirror
`submitResult`'s `waitForResponse` pattern but matching `.endsWith('/skip')`), asserts
`response.status() === 200`, awaits `waitForCardSettled`, and returns the response
(the caller needs it in the re-select test to count skip POSTs separately from result
POSTs). Place it directly below `submitResult` (after line 109).

#### 2. Rewrite the parametrized "never strictly harder" test

**File**: `tests/e2e/adaptive-session-loop.spec.ts` (currently lines 157-205)

**Intent**: Replace the `Bailed` step with a real skip, and assert the invariant FR-012
actually specifies — the skip inherits the *anchor's* grade bound, not a fresh
equal-or-easier comparison against the pick it replaces — plus the two behaviors the
rename explicitly promised: the skipped problem is excluded from the rest of the
session's picks, and never appears in session history.

**Contract**:
- Rename the test title to drop "bailed": `` `board ${board.year}: a failed or
  skipped attempt never yields a strictly harder next problem, and the card swaps in
  place` ``.
- Keep the `g0`/`g1`/`g2` steps (first pick, easy send, hard failure) unchanged —
  they don't touch skip.
- Replace `await submitResult(page, 'Bailed', 5); const g3 = ...; expect(gradeIndex(g3)).toBeLessThanOrEqual(gradeIndex(g2));`
  with: capture the current card's name (reuse the `getByRole('heading', { level: 1 })`
  pattern from `past-sessions-history.spec.ts:69`) as `skippedName` before skipping;
  call `skipCurrent(page)`; read the new grade as `g3`; assert
  `expect(gradeIndex(g3)).toBeLessThanOrEqual(gradeIndex(g1))` — bounded against `g1`
  (the anchor's own grade, i.e. the same bound already proven for `g2`), per the Key
  Discoveries note above, not against `g2`.
- After the loop's existing no-full-reload assertion, add: one more `submitResult`
  (any `Sent`/low-RPE call is fine) and assert the returned card's name is never
  `skippedName` — proves the skipped problem is excluded from the rest of the session.
- After `End session` navigates home, add: click the `past sessions` link, wait for
  `/sessions`, and assert `getByRole('link', { name: /3 problems climbed/i })` is
  visible (3 rated problems: the two pre-skip sends/fails plus the one post-skip send
  added above — the skip itself does not add to the count) — proves the skip never
  appears in history. (past-sessions-history.spec.ts:142 is the precedent for this
  locator shape.)

#### 3. Rescope the "re-selecting a completion status" test

**File**: `tests/e2e/adaptive-session-loop.spec.ts` (currently lines 207-254)

**Intent**: The original test proved 3 radios could be toggled without submitting.
Only 2 radios remain toggleable; `Skip` is a separate one-click submit path with
different behavior worth proving explicitly: it fires exactly once, immediately, with
no RPE tap required.

**Contract**:
- Drop the `Bailed` radio step entirely from the toggle walk (lines 230-232); keep
  the `Failed` → `Sent` toggle-without-submitting assertions as-is (they still hold).
- After the existing "exactly one submit total" assertion (current line 253), add a
  second account-free continuation *or* a second `test(...)` in the same file (author's
  call during implementation — either is fine as long as it reuses `signUpOnboardStart`
  and tears down via the existing `afterEach`): assert that clicking `Skip` fires
  exactly one `POST .../skip` (track via a `skipPosts` counter on the `page.on('request', ...)`
  listener, mirroring the existing `resultPosts` counter) and settles a fresh card
  without any RPE button ever being clicked — proving Skip needs no RPE step.
- Keep the test's existing name (it still accurately describes the Sent/Failed
  portion); a short title tweak acknowledging the added Skip coverage is fine but not
  required.

#### 4. Sync the test-plan cross-reference

**File**: `context/foundation/test-plan.md`

**Intent**: Keep the doc's own references section accurate once the test title
changes.

**Contract**: In §6.5 (around line 204), change `"a failed or bailed attempt never
yields a strictly harder next problem"` to match the new test title from Change 2
above (`"a failed or skipped attempt never yields a strictly harder next problem"`).

### Success Criteria:

#### Automated Verification:

- `npm run typecheck` passes with no errors
- `npm run test:e2e` passes 10/10 (the existing 6 stay green, the 3 rewritten ones go
  green with the new assertions in place, plus 1 new dedicated Skip-submits-immediately
  test added during implementation)
- `grep -rn "Bailed" tests/e2e/ context/foundation/test-plan.md` returns no matches

#### Manual Verification:

- Deliberate-break the new skip-inertness assertion before calling this done,
  mirroring the technique `adaptive-main-session-loop/plan.md:968` used for the
  "never harder" rule: temporarily remove the `states[len(states)-1].Skipped = true`
  line in `internal/server/session.go`'s `handleSkip` (so the recommender treats the
  skipped row as a normal shown entry instead of inert), re-run the rewritten
  parametrized test, confirm it goes red at the new bound/history assertions, then
  revert the temporary change and confirm green again.
- Visually spot-check one run of the rewritten tests (`npm run test:e2e -- --headed`
  or the Playwright HTML report) to confirm the Skip button behaves as expected on
  the Pixel 7 viewport the config drives.

**Implementation Note**: After completing this phase and all automated verification
passes, pause here for manual confirmation from the human that the deliberate-break
verification and visual spot-check were successful.

---

## Testing Strategy

### Unit Tests:

No unit test changes — `TestHandleSkip` already covers the server-side skip contract
this plan's e2e assertions are now proving at the browser level too.

### Integration Tests:

None needed — the existing `TestHandleSkip` integration coverage in
`internal/server/session_handler_test.go` is unaffected by this change.

### Manual Testing Steps:

1. Run `npm run test:e2e` and confirm 9/9 green.
2. Perform the deliberate-break step described in Phase 1's Manual Verification and
   confirm the new assertions catch the regression, then revert.
3. Spot-check the Skip button once in the Playwright HTML report or headed mode.

## Performance Considerations

None — this is a test-only change; no production code, query, or render path is
touched.

## Migration Notes

None — no schema or data changes.

## References

- Failing spec: `tests/e2e/adaptive-session-loop.spec.ts:158,213`
- Correct production behavior: `templates/pages/session.templ:112-145`,
  `internal/server/session.go:313-425`
- Server-side skip contract (unit-tested, unaffected): `internal/server/session_handler_test.go:163-247`
- Grade-window invariant referenced in Key Discoveries: `internal/recommender/grade_window.go:91-113`
- History climbed-count locator precedent: `tests/e2e/past-sessions-history.spec.ts:142`
- Prior deliberate-break precedent: `context/changes/adaptive-main-session-loop/plan.md:968`
- PRD context: `context/foundation/prd.md` FR-008, FR-012 (2026-09-06 update,
  change `rename-bailed-to-skipped`)

## Progress

> Convention: `- [ ]` pending, `- [x]` done. Append ` — <commit sha>` when a step
> lands. Do not rename step titles. See `references/progress-format.md`.

### Phase 1: Rewrite the spec for Skip semantics

#### Automated

- [x] 1.1 `npm run typecheck` passes with no errors
- [x] 1.2 `npm run test:e2e` passes 10/10
- [x] 1.3 `grep -rn "Bailed" tests/e2e/ context/foundation/test-plan.md` returns no matches

#### Manual

- [ ] 1.4 Deliberate-break the skip-inertness assertion, confirm red, revert, confirm green
- [ ] 1.5 Visual spot-check of the Skip button in headed mode / HTML report

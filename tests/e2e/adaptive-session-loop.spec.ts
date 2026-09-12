import { test, expect, type Page, type Response } from '@playwright/test';

/**
 * Protects: test-plan.md §2 Risk #1 (the adaptive loop's direction of
 * adaptation must be visible to the climber) and Risk #6 (the result submit
 * swaps the problem card in place — no full-page reload, URL unchanged).
 *
 * Cross-boundary chain: Supabase Auth cookie -> chi routing -> OnboardingGate
 * -> POST /session/{id}/result (server-side validation + one-tx write) ->
 * recommender.PickNext (grade window + hold-type balance + fallback tiers) ->
 * Postgres catalog + problem_hold_types read -> re-rendered #session-card
 * HTMX fragment.
 *
 * The failures this test catches:
 *  - a "failed" or "skipped" attempt produces a STRICTLY HARDER next problem
 *    (breaks US-01 AC "a felt 9/10 or failed/skipped result never produces a
 *    strictly harder next pick" — the one hard invariant of FR-012);
 *  - a skip moves the grade axis, gets re-recommended, or shows up in session
 *    history (FR-008 / FR-012: skip is rating-free and inert for the engine);
 *  - the first recommendation is not at the board's minimum grade (FR-011);
 *  - the result POST triggers a full navigation / document reload instead of
 *    an in-place #session-card swap (Risk #6);
 *  - End session does not return the climber to a hub that can start again.
 *
 * Seed: modelled on seed.spec.ts — role/label locators, wait-for-state, a
 * risk-tied name, per-account teardown in afterEach.
 *
 * This spec drives sign-up + onboarding through the UI (contrary to the usual
 * storageState rule) for the same reason auth-first-problem.spec.ts does: the
 * session it needs is a real authenticated session and there is no storageState
 * fixture wired up yet.
 */

const SUPABASE_URL = process.env.SUPABASE_URL;
const SUPABASE_SERVICE_KEY = process.env.SUPABASE_SECRET_KEY;

// Font grades in ascending difficulty order. The repo relies on the fact that
// this order is also the lexical order (plan item 2.4, verified against the
// live catalog); the array makes the comparison explicit and readable here.
const FONT_LADDER = [
  '5+', '6A', '6A+', '6B', '6B+', '6C', '6C+',
  '7A', '7A+', '7B', '7B+', '7C', '7C+', '8A', '8A+', '8B', '8B+',
];

function gradeIndex(grade: string): number {
  const i = FONT_LADDER.indexOf(grade);
  if (i < 0) throw new Error(`grade "${grade}" is not on the font ladder`);
  return i;
}

// Best-effort teardown: deleting the auth.users row cascades to the profile,
// session, and session_problems rows this test created (ON DELETE CASCADE).
async function deleteSupabaseUser(userId: string): Promise<void> {
  if (!SUPABASE_URL || !SUPABASE_SERVICE_KEY) {
    throw new Error(
      'cannot clean up: SUPABASE_URL / SUPABASE_SECRET_KEY not in env (.env)',
    );
  }
  const res = await fetch(`${SUPABASE_URL}/auth/v1/admin/users/${userId}`, {
    method: 'DELETE',
    headers: {
      apikey: SUPABASE_SERVICE_KEY,
      Authorization: `Bearer ${SUPABASE_SERVICE_KEY}`,
    },
  });
  if (!res.ok && res.status !== 404) {
    throw new Error(`admin delete user ${userId}: ${res.status} ${await res.text()}`);
  }
}

// Reads the grade off the problem card. The grade is its own chip now (the
// catalog-detail line became discrete chips); it carries a test hook because
// three plain-text sibling chips are ambiguous to a role/label locator.
async function readCardGrade(page: Page): Promise<string> {
  const grade = page.getByTestId('card-grade');
  await expect(grade).toBeVisible();
  return ((await grade.textContent()) ?? '').trim();
}

// Reads the current problem card's name (the level-1 heading).
async function readCardName(page: Page): Promise<string> {
  const heading = page.getByRole('heading', { level: 1 });
  await expect(heading).not.toBeEmpty();
  return ((await heading.textContent()) ?? '').trim();
}

// Waits for htmx to finish swapping AND settling the new #session-card, so its
// nested forms (the next result form, the End form) are re-bound before the
// test acts on them. Syncs on htmx's own transient lifecycle classes on the
// card landmark — not a locator for interaction.
async function waitForCardSettled(page: Page): Promise<void> {
  await page.waitForFunction(() => {
    const card = document.getElementById('session-card');
    return (
      !!card &&
      !card.classList.contains('htmx-swapping') &&
      !card.classList.contains('htmx-settling')
    );
  });
}

// Picks a completion status, then taps an RPE number to submit. Waits for the
// result response and for the fresh #session-card to fully settle (a new card
// has no status radio checked).
async function submitResult(page: Page, completion: string, rpe: number): Promise<void> {
  await page.getByRole('radio', { name: completion }).check();

  const [response] = await Promise.all([
    page.waitForResponse(
      (r) => r.url().endsWith('/result') && r.request().method() === 'POST',
    ),
    page.getByRole('button', { name: String(rpe), exact: true }).click(),
  ]);
  expect(response.status()).toBe(200);

  await waitForCardSettled(page);
  await expect(page.getByRole('radio', { name: 'Sent' })).not.toBeChecked();
}

// Clicks the Skip button — a one-click submit that needs no RPE tap. Waits for
// the matching /skip response and for the fresh #session-card to settle. Returns
// the response so callers can count skip submissions separately from results.
async function skipCurrent(page: Page): Promise<Response> {
  const [response] = await Promise.all([
    page.waitForResponse(
      (r) => r.url().endsWith('/skip') && r.request().method() === 'POST',
    ),
    page.getByRole('button', { name: 'Skip' }).click(),
  ]);
  expect(response.status()).toBe(200);

  await waitForCardSettled(page);
  return response;
}

type Board = { holdsetup: string; year: string; minGrade: string };

const BOARDS: Board[] = [
  { holdsetup: '1', year: '2016', minGrade: '6B' },
  { holdsetup: '21', year: '2024', minGrade: '6B+' },
];

let createdUserId: string | null = null;

test.afterEach(async () => {
  if (createdUserId) {
    await deleteSupabaseUser(createdUserId);
    createdUserId = null;
  }
});

// Sign up a throwaway account, onboard onto `board` at max grade 8B+ (full
// headroom), start a Main Session, and land on the first problem card. Sets
// createdUserId for afterEach teardown. Returns the session URL.
async function signUpOnboardStart(page: Page, board: Board): Promise<string> {
  const stamp = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;

  await page.goto('/signup');
  await page.getByLabel('Email').fill(`moonphase-e2e+${stamp}@example.com`);
  await page.getByLabel('Password').fill(`E2e-${stamp}-Pw`);
  await page.getByRole('button', { name: 'Sign up' }).click();
  await page.waitForURL('**/onboarding');

  await page.getByLabel('Max grade').selectOption('8B+');
  await page.getByLabel('Board').selectOption(board.holdsetup);
  await page.getByLabel('Angle').selectOption('40');
  await page.getByRole('button', { name: 'Continue' }).click();

  const startSession = page.getByRole('button', { name: 'Start session' });
  await expect(startSession).toBeVisible();

  const me = await page.request.get('/api/me');
  expect(me.ok()).toBeTruthy();
  createdUserId = (await me.json()).user_id as string;
  expect(createdUserId).toBeTruthy();

  await startSession.click();
  await page.waitForURL(/\/session\/[0-9a-f-]{36}$/);
  return page.url();
}

for (const board of BOARDS) {
  test(`board ${board.year}: a failed or skipped attempt never yields a strictly harder next problem, and the card swaps in place`, async ({ page }) => {
    const sessionUrl = await signUpOnboardStart(page, board);

    // Mark the document so a full reload during the loop is detectable.
    await page.evaluate(() => {
      (window as unknown as { __noReload?: boolean }).__noReload = true;
    });

    // FR-011: the first recommendation is at the board's minimum grade.
    const g0 = await readCardGrade(page);
    expect(g0).toBe(board.minGrade);

    // --- Easy send: allowed to step up at most one ladder grade, never below
    //     the minimum, never above the session max ---
    await submitResult(page, 'Sent', 2);
    const g1 = await readCardGrade(page);
    expect(page.url()).toBe(sessionUrl); // swapped in place, no navigation
    expect(gradeIndex(g1)).toBeGreaterThanOrEqual(gradeIndex(g0));
    expect(gradeIndex(g1)).toBeLessThanOrEqual(gradeIndex(g0) + 1);
    expect(gradeIndex(g1)).toBeLessThanOrEqual(gradeIndex('8B+'));

    // --- Hard failure: the next pick must NOT be strictly harder (FR-012 /
    //     US-01 hard invariant) ---
    await submitResult(page, 'Failed', 9);
    const g2 = await readCardGrade(page);
    expect(page.url()).toBe(sessionUrl);
    expect(gradeIndex(g2)).toBeLessThanOrEqual(gradeIndex(g1));

    // --- Skip: carries no RPE and is inert for the grade axis (FR-012). The
    //     anchor for the next pick is still the Failed/RPE-9 result above (skip
    //     rows are skipped over when the server looks for the last rated
    //     result), so the bound is the SAME one already proven for g2 — against
    //     g1, not g2. ---
    const skippedName = await readCardName(page);
    await skipCurrent(page);
    const g3 = await readCardGrade(page);
    expect(gradeIndex(g3)).toBeLessThanOrEqual(gradeIndex(g1));

    // The whole loop stayed on one URL and never full-reloaded.
    expect(page.url()).toBe(sessionUrl);
    const noReload = await page.evaluate(
      () => (window as unknown as { __noReload?: boolean }).__noReload === true,
    );
    expect(noReload).toBe(true);

    // --- The skipped problem is excluded from the rest of the session ---
    await submitResult(page, 'Sent', 3);
    expect(await readCardName(page)).not.toBe(skippedName);

    // --- End the session -> back to a hub that can start again ---
    // End session now lives in the header behind an hx-confirm prompt.
    page.on('dialog', (d) => d.accept());
    await page.getByRole('button', { name: 'End session' }).click();
    await page.waitForURL((url) => new URL(url).pathname === '/');
    await expect(page.getByRole('button', { name: 'Start session' })).toBeVisible();

    // --- The skip never appears in session history (FR-008) ---
    await page.getByRole('link', { name: /past sessions/i }).click();
    await page.waitForURL((url) => new URL(url).pathname === '/sessions');
    await expect(page.getByRole('link', { name: /3 problems climbed/i })).toBeVisible();
  });
}

/**
 * Protects test-plan.md Risk #6 / plan row 5.6: the 2 completion-status radios
 * are a re-selectable choice, not a trigger. Only the RPE button submits. If a
 * status radio ever gained an auto-submit, mis-tapping "Failed" at the wall
 * would lock in a wrong rating with no way back.
 */
test('board 2016: re-selecting a completion status moves the choice without submitting', async ({ page }) => {
  const board = BOARDS[0];
  const sessionUrl = await signUpOnboardStart(page, board);

  let resultPosts = 0;
  page.on('request', (r) => {
    if (r.url().endsWith('/result') && r.method() === 'POST') resultPosts += 1;
  });

  const g0 = await readCardGrade(page);

  // Walk through both statuses. Each selection reveals the RPE grid and moves
  // the checked radio; neither of them submits.
  await page.getByRole('radio', { name: 'Failed' }).check();
  await expect(page.getByRole('radio', { name: 'Failed' })).toBeChecked();
  await expect(page.getByRole('button', { name: '5', exact: true })).toBeVisible();

  await page.getByRole('radio', { name: 'Sent' }).check();
  await expect(page.getByRole('radio', { name: 'Sent' })).toBeChecked();
  await expect(page.getByRole('radio', { name: 'Failed' })).not.toBeChecked();
  await expect(page.getByRole('button', { name: '5', exact: true })).toBeVisible();

  // Nothing submitted: same card, same URL, zero /result requests so far.
  expect(page.url()).toBe(sessionUrl);
  expect(await readCardGrade(page)).toBe(g0);
  expect(resultPosts).toBe(0);

  // The RPE button is what submits — do it once, with the final selection.
  const [resp] = await Promise.all([
    page.waitForResponse((r) => r.url().endsWith('/result') && r.request().method() === 'POST'),
    page.getByRole('button', { name: '3', exact: true }).click(),
  ]);
  expect(resp.status()).toBe(200);
  await waitForCardSettled(page);

  // Exactly one submit total — the two status re-taps fired nothing.
  expect(resultPosts).toBe(1);
});

/**
 * Protects test-plan.md Risk #6: unlike the status radios above, Skip is a
 * one-click submit — it needs no RPE tap and must fire exactly once. If Skip
 * ever required a second tap to confirm, a climber at the wall skipping a
 * problem they don't want to attempt would be stuck on a card they can't move
 * past without rating something they didn't climb.
 */
test('board 2016: Skip submits immediately with no RPE tap', async ({ page }) => {
  const board = BOARDS[0];
  const sessionUrl = await signUpOnboardStart(page, board);

  let skipPosts = 0;
  page.on('request', (r) => {
    if (r.url().endsWith('/skip') && r.method() === 'POST') skipPosts += 1;
  });

  await skipCurrent(page);

  expect(page.url()).toBe(sessionUrl); // swapped in place, no navigation
  expect(skipPosts).toBe(1);
  // A fresh card has no status radio checked — no RPE tap was needed.
  await expect(page.getByRole('radio', { name: 'Sent' })).not.toBeChecked();
  await expect(page.getByRole('radio', { name: 'Failed' })).not.toBeChecked();
});

/**
 * Plan row 5.7 / NFR "operable one-handed on a phone in portrait; primary tap
 * targets remain reachable with the thumb during an active session". The End
 * session button must be reachable without scrolling on a phone viewport
 * (config drives the Pixel 7 device).
 */
test('board 2016: the End session button is within the initial phone viewport', async ({ page }) => {
  await signUpOnboardStart(page, BOARDS[0]);
  await expect(page.getByRole('button', { name: 'End session' })).toBeInViewport();
});

/**
 * Protects test-plan.md Risk #1 / S-04 plan row 6.3: a run of easy sends must
 * ramp grade — the loop cannot get stuck one grade above the minimum. This is
 * the regression `adaptive-loop-ramp-fix` fixes (NextPickCandidates used to
 * sample only the densest grade in the window). "Not stuck" = the grade seen
 * over an easy-send run gets past min+1; it need not climb every step (hold-type
 * balance legitimately holds a grade), and it never exceeds the session max.
 */
test('board 2016: a run of easy sends ramps grade past the dense floor', async ({ page }) => {
  const board = BOARDS[0];
  await signUpOnboardStart(page, board);

  const min = gradeIndex(board.minGrade);
  const maxAllowed = gradeIndex('8B+');
  const seen: number[] = [gradeIndex(await readCardGrade(page))];

  for (let i = 0; i < 6; i++) {
    await submitResult(page, 'Sent', 2);
    seen.push(gradeIndex(await readCardGrade(page)));
  }

  const peak = Math.max(...seen);
  expect(peak).toBeGreaterThan(min + 1); // not stuck at the dense floor+1 grade
  expect(peak).toBeLessThanOrEqual(maxAllowed); // never above the session max
  for (let i = 1; i < seen.length; i++) {
    expect(seen[i]).toBeGreaterThanOrEqual(seen[i - 1]); // monotonic on an all-easy-send run
  }
});

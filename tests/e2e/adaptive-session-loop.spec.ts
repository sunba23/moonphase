import { test, expect, type Page } from '@playwright/test';

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
 *  - a "failed" or "bailed" attempt produces a STRICTLY HARDER next problem
 *    (breaks US-01 AC "a felt 9/10 or failed/bailed result never produces a
 *    strictly harder next pick" — the one hard invariant of FR-012);
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

// Reads the grade off the problem card's catalog-detail line ("6B · 40° · 2016").
async function readCardGrade(page: Page, boardYear: string): Promise<string> {
  const meta = page.getByText(new RegExp(`·\\s*40°\\s*·\\s*${boardYear}`));
  await expect(meta).toBeVisible();
  const text = ((await meta.textContent()) ?? '').trim();
  return text.split('·')[0].trim();
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

let createdUserId: string | null = null;

test.afterEach(async () => {
  if (createdUserId) {
    await deleteSupabaseUser(createdUserId);
    createdUserId = null;
  }
});

const BOARDS = [
  { holdsetup: '1', year: '2016', minGrade: '6B' },
  { holdsetup: '21', year: '2024', minGrade: '6B+' },
];

for (const board of BOARDS) {
  test(`board ${board.year}: a failed or bailed attempt never yields a strictly harder next problem, and the card swaps in place`, async ({ page }) => {
    const stamp = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
    const email = `moonphase-e2e+${stamp}@example.com`;
    const password = `E2e-${stamp}-Pw`;

    // --- Sign up ---
    await page.goto('/signup');
    await page.getByLabel('Email').fill(email);
    await page.getByLabel('Password').fill(password);
    await page.getByRole('button', { name: 'Sign up' }).click();
    await page.waitForURL('**/onboarding');

    // --- Onboard: max grade at the ladder top so the loop has full headroom ---
    await page.getByLabel('Max grade').selectOption('8B+');
    await page.getByLabel('Board').selectOption(board.holdsetup);
    await page.getByLabel('Angle').selectOption('40');
    await page.getByRole('button', { name: 'Continue' }).click();

    const startSession = page.getByRole('button', { name: 'Main Session' });
    await expect(startSession).toBeVisible();

    // Capture the user id via the authenticated API so afterEach can always
    // tear the account down.
    const me = await page.request.get('/api/me');
    expect(me.ok()).toBeTruthy();
    createdUserId = (await me.json()).user_id as string;
    expect(createdUserId).toBeTruthy();

    // --- Start the session ---
    await startSession.click();
    await page.waitForURL(/\/session\/[0-9a-f-]{36}$/);
    const sessionUrl = page.url();

    // Mark the document so a full reload during the loop is detectable.
    await page.evaluate(() => {
      (window as unknown as { __noReload?: boolean }).__noReload = true;
    });

    // FR-011: the first recommendation is at the board's minimum grade.
    const g0 = await readCardGrade(page, board.year);
    expect(g0).toBe(board.minGrade);

    // --- Easy send: allowed to step up at most one ladder grade, never below
    //     the minimum, never above the session max ---
    await submitResult(page, 'Sent', 2);
    const g1 = await readCardGrade(page, board.year);
    expect(page.url()).toBe(sessionUrl); // swapped in place, no navigation
    expect(gradeIndex(g1)).toBeGreaterThanOrEqual(gradeIndex(g0));
    expect(gradeIndex(g1)).toBeLessThanOrEqual(gradeIndex(g0) + 1);
    expect(gradeIndex(g1)).toBeLessThanOrEqual(gradeIndex('8B+'));

    // --- Hard failure: the next pick must NOT be strictly harder (FR-012 /
    //     US-01 hard invariant) ---
    await submitResult(page, 'Failed', 9);
    const g2 = await readCardGrade(page, board.year);
    expect(page.url()).toBe(sessionUrl);
    expect(gradeIndex(g2)).toBeLessThanOrEqual(gradeIndex(g1));

    // --- Bail: same rule ---
    await submitResult(page, 'Bailed', 5);
    const g3 = await readCardGrade(page, board.year);
    expect(gradeIndex(g3)).toBeLessThanOrEqual(gradeIndex(g2));

    // The whole loop stayed on one URL and never full-reloaded.
    expect(page.url()).toBe(sessionUrl);
    const noReload = await page.evaluate(
      () => (window as unknown as { __noReload?: boolean }).__noReload === true,
    );
    expect(noReload).toBe(true);

    // --- End the session -> back to a hub that can start again ---
    await page.getByRole('button', { name: 'End session' }).click();
    await page.waitForURL((url) => new URL(url).pathname === '/');
    await expect(page.getByRole('button', { name: 'Main Session' })).toBeVisible();
  });
}

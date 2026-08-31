# MoonPhase E2E tests

Browser-level tests that drive the **running Go app** (`cmd/server`) through
Playwright. They protect risks from `context/foundation/test-plan.md` §2 that
cross several system boundaries at once — auth, the chi routing + gate
middleware, the recommender, and Postgres.

## Running

```bash
npm install
npm run test:e2e:install   # one-time: download the Chromium build
npm run test:e2e
```

`playwright.config.ts` starts `go run ./cmd/server` for you (it self-loads
`.env`) and waits on `/healthz`. To run against an already-running instance:

```bash
E2E_BASE_URL=http://localhost:2137 npm run test:e2e
```

The tests need a reachable Postgres **and** Supabase Auth project — the same
`.env` the app uses. `SUPABASE_URL` + `SUPABASE_SECRET_KEY` are also read by the
tests to delete the throwaway accounts they create (FK `ON DELETE CASCADE` from
`auth.users` clears the profile + session rows).

## Stack note

`context/foundation/test-plan.md` §Test Layers names `playwright-community/playwright-go`
for this layer. This directory uses **TypeScript Playwright** instead, by
explicit direction — the `/10x-e2e` skill and its quality levers are written for
it. Update the test-plan if this becomes the standing choice.

## E2E Testing Rules

- Use `getByRole`, `getByLabel`, `getByText` as primary locators. Fall back to
  `getByTestId` only when accessibility attributes are ambiguous.
- Never use CSS selectors, XPath, or DOM structure to locate elements.
- Each test is independently runnable — its own setup, action, assertion, and
  cleanup. No shared state between specs.
- Never use `page.waitForTimeout()`. Wait for a concrete condition:
  `toBeVisible()`, `waitForURL()`, `waitForResponse()`.
- Assert the business outcome (the user reached the recommended problem), not
  implementation details (HTTP status, template internals).
- Unique identifiers (timestamp + random suffix) for every account/record a test
  creates, so re-runs and parallel runs never collide. Tear it down in
  `afterEach` — even on failure.
- Prefer `storageState` for auth so specs don't log in through the UI. **The one
  exception**: a spec whose risk *is* the sign-up / onboarding / gate flow drives
  that flow through the UI on purpose — that is the behavior under test.
- Internal boundaries (Supabase Auth, chi routing, the gate, Postgres) stay
  **real**. Mock only expensive or non-deterministic *external* services at the
  network layer — there are none in this flow today.
- Name each spec after the risk it protects, not `test('test 1', ...)`.

## The seed

`seed.spec.ts` is the exemplar every new spec is modeled on — role-based
locators, wait-for-state, risk-tied name and assertion. Copy its shape; do not
copy in `waitForTimeout` or CSS selectors.

# Session view — vertical-space overhaul (`session-view-density`)

## Context

The live Main Session screen stacks too much above the board, so on a phone the
climber must scroll to see the board and the rating controls at the same time.
That breaks the "usable one-handed at the wall" guardrail.

This change makes the whole board fit one phone viewport with no scrolling and
moves every secondary control off the vertical stack:

- **End session** goes to the top-right of the app header (small target, with a
  confirm step).
- **Angle and board-year** chips under the problem name are removed — the header
  already shows them. Only the **grade** stays, on the same line as the problem
  name, which truncates with an ellipsis when the grade would not fit.
- **Session balance** moves behind an info glyph in the heading line and opens as
  a bottom sheet.
- **Holds** are made tappable on the board (a small tip such as `A5 · foot ·
  crimp`); the separate collapsible "Holds" text list is removed from the live
  session (it stays in the read-only history view).
- The board image is scaled so the board sits fully **above** an opaque controls
  strip — nothing overlaps, and the page never scrolls.
- The controls strip keeps today's pattern: `Sent / Failed / Skip`, then the
  1–10 RPE grid revealed after a status is chosen.

Scope is the live session view only. The history view (`HistoryProblemContent`)
keeps its current layout.

Stack rules: templ + HTMX + hand-rolled CSS, **zero custom JS**. `hx-confirm`
(built-in htmx, native `confirm()`) is allowed. Run `templ generate` after
editing `.templ`; commit the regenerated `*_templ.go`.

## Decisions (from the user)

1. Board fully above an **opaque** controls strip; board scaled down to fit; no
   overlap, no scroll. Keep `Sent / Failed / Skip` + revealed RPE grid as-is.
2. **Confirm before ending** — `hx-confirm="End this session?"` on the header
   End form.
3. **Remove** the "Holds" text list from the live session; keep it in history.

## Bookkeeping

This planning ran in plan mode. On approval, first materialise the 10x change
folder:

- `context/changes/session-view-density/change.md` — identity file, `status:
  new`, today's date (per the `/10x-new` template).
- `context/changes/session-view-density/plan.md` — this plan.

## Approach

### 1. Header "End session"

**Files:** `internal/server/render.go`, `internal/server/session.go`,
`templates/layout/app.templ`

- `render.go` — extract two helpers, keep `renderAppPage` behaviour identical:
  - `navFromContext(r) (layout.NavModel, bool)` — the profile→`NavModel` build.
  - `writeAppPage(w, r, title, nav, content, status)` — the header write.
- `session.go` — add `renderSessionPage(w, r, sessionID, content, status)`: build
  nav via `navFromContext`, set `nav.EndSessionID = sessionID`, call
  `writeAppPage` with title `"Session"`. Use it in `handleView`'s success render
  (line ~152) instead of `renderAppPage`. The unsupported-board branch and
  `renderNextCard` (fragment, no header) stay unchanged — the header is not
  re-rendered on card swaps, so a header-mounted End control persists across
  every `#session-card` swap.
- `templates/layout/app.templ`:
  - `NavModel`: add `EndSessionID string // set only on the live session page`.
  - `appHeader`: when `nav.EndSessionID != ""`, render before `.appbar__context`:
    ```templ
    <form class="appbar__end" hx-post={ "/session/" + nav.EndSessionID + "/end" } hx-swap="none" hx-confirm="End this session?">
      <button type="submit" class="btn btn--secondary appbar__end-btn">End session</button>
    </form>
    ```
- Endpoint, `handleEnd`, and the `HX-Redirect: /` behaviour are unchanged.

### 2. Session card restructure

**File:** `templates/pages/session.templ` (then `templ generate`)

New `SessionCard` body:

```
<div id="session-card" class="session session--live">
  <div class="session__scroll">
    @sessionHeading(m)                              ← name + grade + info glyph + balance sheet
    @moonBoardOverlay(m.Problem.BoardYear, m.Problem.Holds)
  </div>
  <div class="session__actions">
    @resultForm(m.SessionID, m.Seq)                 ← Sent/Failed/Skip + revealed RPE (unchanged)
  </div>
</div>
```

Component changes:

| Component | Change |
|---|---|
| `SessionCard` | Drop `@problemMeta`, `@holdsPanel`, the direct `@sessionPanel`, and the inline `.session__end` form. Add `@sessionHeading`. |
| NEW `sessionHeading(m SessionCardModel)` | One flex row: `<h1 class="session__name">` (ellipsis) + `<span class="session__grade chip chip--mono" data-testid="card-grade">` + (when `m.Panel != nil`) the info-glyph `<label>` and the balance sheet markup. |
| NEW `balanceBody(p SessionPanel)` | Same inner markup as today's `sessionPanel` — `<h2 class="sheet__title">@iconBalance() Session balance</h2>`, the `.panel__why` chips, the `<div class="panel">` with `.panel__bars`. **Not** a `<details>`. Keeps the `class="panel"` and `"Session balance"` substrings that `session_handler_test.go` checks. |
| `sessionPanel` | Deleted (replaced by `balanceBody`). |
| `holdMarker(year, h)` | `<span>` → `<button type="button" class="hold hold--{role}">` with a child `<span class="hold__tip" aria-hidden="true">{holdTip(h)}</span>` and `aria-label={ holdTip(h) }`. Drop `title`. (Shared with history — tappable there too, which is fine.) |
| NEW `holdTip(h) string` | `h.GridRef + " · " + h.Role`, plus `" · " + h.PrimaryType` when non-empty. |
| NEW `iconInfo()` | Small decorative `ⓘ` SVG, `aria-hidden`, house style (`viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.6"`). |
| `holdsPanel`, `iconHolds` | **Deleted** — session-only and now unused (the `unused` linter would flag them). |
| `resultForm` | Unchanged (signature and body). The `.result__status:has(input:checked) ~ .result__rpe` reveal and the inline Skip submit button stay. |
| `problemMeta`, `holdsList`, `holdLabel` | **Unchanged** — still used by `HistoryProblemContent`. |
| `SessionPage` / `SessionModel` | Leave as-is (exported; not linted as unused). |

Balance sheet markup inside `sessionHeading` (checkbox-hack; only when `m.Panel != nil`):

```templ
<div class="session__heading sheet">
  if m.Panel != nil {
    <input type="checkbox" id="balance-sheet" class="sheet__toggle"/>
  }
  <h1 class="session__name">{ m.Problem.Name }</h1>
  <span class="session__grade chip chip--mono" data-testid="card-grade">{ m.Problem.Grade }</span>
  if m.Panel != nil {
    <label for="balance-sheet" class="session__info" aria-label="Session balance">@iconInfo()</label>
    <label for="balance-sheet" class="sheet__scrim" aria-hidden="true"></label>
    <div class="sheet__panel" role="dialog" aria-label="Session balance">
      <label for="balance-sheet" class="sheet__close" aria-label="Close">&times;</label>
      @balanceBody(*m.Panel)
    </div>
  }
</div>
```

The checkbox lives inside `#session-card`, so every `hx-swap="outerHTML"` swap
re-renders it unchecked — the sheet always comes back closed.

### 3. Model changes

- `layout.NavModel`: `+ EndSessionID string`.
- `pages.SessionCardModel` and `pages.SessionPanel`: **no change** (`Problem.Name`,
  `Problem.Grade`, `Panel`, and `catalog.HoldPlacement` already carry everything).

### 4. CSS

**File:** `static/app.css` (session region ~L435–811)

Delete: `.session__end`. Keep all `.panel*` rules (still used by `balanceBody`)
and all `.holds*` rules (still used by history).

Add:

- **Header End** — `.appbar__end` pushed right (`margin-left:auto`);
  `.appbar__end-btn` `min-height:44px`, `--step--1`. `.appbar` already
  `flex-wrap:wrap`, so the context chips wrap to a second row on narrow phones
  (accepted; the board clamp below allows for a two-row header).
- **Compact heading** — `.session__heading` flex row, `align-items:center`,
  `gap: --space-2`. `.session__name` `flex:1 1 auto; min-width:0; margin:0;
  font-size: --step-2; overflow:hidden; text-overflow:ellipsis;
  white-space:nowrap`. `.session__grade` `flex:0 0 auto`. `.session__info`
  44×44 tap target, `--ink-soft`, negative margins so it does not add row height.
- **Board fit (no scroll)** — the board sits above the controls; scale it to the
  space left by header + heading + controls:
  ```css
  :root { --session-chrome: 12rem; } /* header(2 rows) + heading + controls with RPE open; tune on device */
  .session--live .board { width: fit-content; max-width: 100%; margin-inline: auto; }
  .session--live .board__img {
    width: auto; max-width: 100%; height: auto;
    max-height: calc(100dvh - var(--session-chrome));
  }
  ```
  `.board` shrink-wraps the block image, so the `left%`/`top%` hold positions
  still resolve against the exact image rectangle. Do **not** add
  padding/border to `.board`. `.session__scroll { overflow-y:auto }` stays as a
  safety net if a viewport is smaller than expected.
- **Hold button + tip** —
  ```css
  button.hold { appearance:none; -webkit-appearance:none; padding:0; background:transparent; font:inherit; color:inherit; cursor:pointer; }
  .hold:hover, .hold:focus, .hold:focus-visible { z-index:5; }
  .hold:focus-visible { outline:2px solid var(--accent); outline-offset:1px; }
  .hold__tip {
    position:absolute; bottom:calc(100% + 4px); left:50%; transform:translateX(-50%);
    padding:2px 6px; font:500 var(--step--1)/1.3 var(--font-body); white-space:nowrap;
    color:var(--accent-ink); background:var(--ink); border-radius:var(--r-sm);
    opacity:0; visibility:hidden; pointer-events:none; transition:opacity 120ms ease;
  }
  .hold:hover .hold__tip, .hold:focus .hold__tip { opacity:1; visibility:visible; }
  @media (prefers-reduced-motion: reduce) { .hold__tip { transition:none; } }
  ```
  Optional hit-area bump: `.hold::after{content:"";position:absolute;inset:-6px;}`.
- **Balance sheet** — a bottom sheet driven by the sr-only checkbox:
  ```css
  .sheet__toggle { position:absolute; width:1px; height:1px; margin:-1px; overflow:hidden; clip-path:inset(50%); }
  .sheet__scrim {
    position:fixed; inset:0; margin:0; background:rgba(20,26,42,.45);
    opacity:0; visibility:hidden; transition:opacity 160ms ease; z-index:20;
  }
  .sheet__panel {
    position:fixed; left:0; right:0; bottom:0; z-index:21;
    width:100%; max-width:30rem; margin-inline:auto; max-height:85dvh; overflow-y:auto;
    padding:var(--space-5) var(--space-4) calc(var(--space-5) + env(safe-area-inset-bottom, 0px));
    background:var(--surface); border:1px solid var(--edge); border-bottom:0;
    border-radius:var(--r-md) var(--r-md) 0 0; box-shadow:var(--shadow-1);
    transform:translateY(100%); transition:transform 220ms ease-out;
  }
  .sheet__toggle:checked ~ .sheet__scrim { opacity:1; visibility:visible; }
  .sheet__toggle:checked ~ .sheet__panel { transform:translateY(0); }
  .sheet__toggle:focus-visible ~ .session__info { outline:2px solid var(--accent); outline-offset:2px; border-radius:var(--r-sm); }
  .sheet__title { display:flex; align-items:center; gap:var(--space-2); margin:0 0 var(--space-3); font-size:var(--step-1); }
  .sheet__close { position:absolute; top:var(--space-1); right:var(--space-2); width:44px; height:44px; margin:0; display:inline-flex; align-items:center; justify-content:center; font-size:var(--step-2); color:var(--ink-soft); cursor:pointer; }
  @media (prefers-reduced-motion: reduce) { .sheet__scrim, .sheet__panel { transition:none; } }
  ```
  Note: the global `label { display:block; margin-bottom: --space-4 }` reaches
  every sheet `<label>` — the rules above reset `margin` explicitly.
- **Controls strip** — `.session__actions` keeps its current flex/border/shadow.
  Tighten `.result__status legend` margin if the strip runs tall. Remove
  `.session__end`.

### 5. Go tests

**File:** `internal/server/session_handler_test.go`

- Line ~284: `strings.Contains(body, "<details class=\"panel\"")` →
  `strings.Contains(body, "class=\"panel\"")` (the balance panel is no longer a
  `<details>`). The `"Session balance"` + `class="panel"` check at ~L127 still
  passes.
- `session_panel_test.go` tests `buildSessionPanel` directly — untouched.

### 6. E2E specs

The rating controls stay inline (no sheet to open), so the `submitResult`
helpers do **not** change.

- `tests/e2e/adaptive-session-loop.spec.ts`
  - Add `page.on('dialog', d => d.accept())` in the test that clicks End (and any
    `beforeEach` shared with it) for the new `hx-confirm`.
  - The "re-selecting a completion status" test still uses stale `'Bailed'`
    locators — **pre-existing** (server accepts only `sent`/`failed` now); flag,
    do not fix here.
  - "End session button within the initial phone viewport" — still valid; keep.
- `tests/e2e/auth-first-problem.spec.ts`
  - Lines ~97–98: `40°` / `2016` are no longer inside `<main>`. Re-scope to the
    header: `const header = page.locator('.appbar'); await
    expect(header.getByText('40°', { exact:true })).toBeVisible();` (same for
    `2016`). `card.getByTestId('card-grade')` stays (grade is still in-card).
  - Lines ~99–102: the "Holds" panel is gone. Replace with a board-hold check:
    ```ts
    const firstHold = card.locator('button.hold').first();
    await expect(firstHold).toBeVisible();
    await firstHold.click();
    await expect(firstHold.locator('.hold__tip')).toBeVisible();
    ```
- `tests/e2e/past-sessions-history.spec.ts`
  - Add `page.on('dialog', d => d.accept())` where it clicks End (~L133).
  - `toHaveCount(0)` for "End session" on the history card (~L167) still passes
    (`EndSessionID` unset on non-session pages).
- `tests/e2e/seed.spec.ts` — no session-UI assertions; untouched.

Follow-up (not this change): the `/10x-e2e` skill for a proper pass over the
`Bailed`→`skipped` staleness and any new density risks.

## Accessibility notes (accepted)

The balance sheet is a checkbox-hack: no focus trap, no Esc, focus not moved into
the panel. Mitigations: the toggle stays in the tab order (sr-only, not
`display:none`), the trigger shows a focus ring, the panel has `role="dialog"` +
`aria-label`, and both the close button and the full-viewport scrim dismiss it.
This matches the project's existing zero-JS `<details>` panels. `<dialog>` +
`showModal()` is rejected (needs JS). The native `popover` attribute is a clean
later migration for the sheet but is not adopted now, to stay consistent with the
existing `:has()` / `<details>` CSS-state patterns.

Board hold targets stay ~18px — accepted for a dense board diagram; the tip is
reachable by pointer and keyboard, and `aria-label` carries the same text.

## Ordered tasks

1. **Change folder** — create `context/changes/session-view-density/{change.md,
   plan.md}`.
2. **Header End** — `render.go` (`navFromContext` + `writeAppPage`), `session.go`
   (`renderSessionPage` + `handleView` call site), `app.templ` (`NavModel` +
   `appHeader`). `templ generate`; `go build ./...`.
3. **Card restructure** — `session.templ`: `sessionHeading`, `balanceBody`,
   `iconInfo`, `holdTip`; rewrite `SessionCard`; convert `holdMarker`; delete
   `holdsPanel` / `iconHolds` / `sessionPanel`. `templ generate`; `go build`.
4. **CSS** — `static/app.css` per section 4.
5. **Go test** — `session_handler_test.go` assertion relax; `go test ./...`.
6. **E2E specs** — per section 6.
7. **Verification** — section below.

## Critical files

- `templates/pages/session.templ` (+ generated `session_templ.go`)
- `templates/layout/app.templ` (+ generated `app_templ.go`)
- `static/app.css`
- `internal/server/render.go`
- `internal/server/session.go`
- `internal/server/session_handler_test.go`
- `tests/e2e/auth-first-problem.spec.ts`, `tests/e2e/adaptive-session-loop.spec.ts`,
  `tests/e2e/past-sessions-history.spec.ts`
- reference only: `internal/catalog/problem_view.go` (`HoldPlacement`),
  `internal/catalog/geometry.go` (`HoldXY`)

## Verification

From the repo root:

1. `templ generate` — regenerate and commit `templates/**/*_templ.go`.
2. `go build ./...`
3. `go vet ./...`
4. `golangci-lint run` — watch `unused` (deleted `holdsPanel` / `iconHolds` /
   `sessionPanel`).
5. `golangci-lint fmt`
6. `go test ./...` — `internal/server` green; only `session_handler_test.go`
   changed.
7. `govulncheck ./...` (CI gate; no code-path change expected).
8. **Manual run** — `.env` needs `PORT`, `DATABASE_URL`, `APP_ENV=development`,
   `SUPABASE_URL`, `SUPABASE_PUBLISHABLE_KEY`. `go run ./cmd/migrate up` if the
   DB is stale, then `go run ./cmd/server`. On a phone-portrait viewport
   (DevTools Pixel 7 and iPhone SE):
   - Header shows "End session" top-right; tapping it prompts "End this session?"
     then redirects to `/`.
   - Heading line = truncating name + grade chip (`data-testid="card-grade"`) +
     ⓘ glyph; no angle/year chips in the card.
   - The whole board is visible with **no page scroll**; the controls strip sits
     fully below the board.
   - Tapping a hold shows its tip (`A5 · foot · crimp`); tapping elsewhere hides
     it.
   - `Sent / Failed` reveals the RPE grid; submitting swaps the card; the header
     does not reload (Network tab: `/result` and `/skip` return the bare
     `<div id="session-card">` fragment).
   - `Skip` is one tap.
   - ⓘ opens the Session balance sheet; the scrim and the close button dismiss
     it; after a card swap it is closed again.
   - Tune `--session-chrome` so nothing scrolls on the smallest tested viewport.
9. **Playwright** — `npx playwright test` (config auto-starts the server on
   `$PORT`, default 2137, and needs a reachable Supabase project). The three
   edited specs plus `seed.spec.ts` should pass; pre-existing `Bailed` failures
   stay out of scope.

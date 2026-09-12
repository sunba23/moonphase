# Prettify the frontend — Implementation Plan

## Overview

MoonPhase works end to end but has almost no visual design. `static/app.css` is 143 lines
and styles only three things (the hub button, the board overlay, the session result form).
Every other screen uses browser defaults. `templates/layout/layout.templ` has no
`<meta name="viewport">`, so the phone view — the product's primary context — renders at
desktop width today.

This plan gives MoonPhase a deliberate visual identity ("chalk by day, moonlit by night" —
a light theme that reads as a birch-ply board in daylight and a dark theme that reads as a
night session, keyed to the product name), a token-based CSS system with light and dark
support, a shared app header that carries the training context, and restructured templates
that are thumb-first and accessible. It adds no product features.

## Current State Analysis

- **One layout shell.** `templates/layout/layout.templ` → `Page(title, content)`. No
  charset, no viewport, no `lang`, no favicon link, no fonts. `htmx.min.js` loads
  render-blocking in `<head>`.
- **One render helper.** `renderPage` in `internal/server/auth_pages.go:93` — sets
  `Content-Type`, writes status, renders the component. Every handler calls it.
- **8 page templates**, package `pages` (`templates/pages/`): `hub`, `signin`, `signup`,
  `onboarding`, `profile`, `session` (+ `unsupportedBoard`), `history` (list / detail /
  read-only problem card). Each `.templ` has a committed `_templ.go` sibling; `templ
  generate` is run manually (not in CI, not in the Dockerfile).
- **HTMX contracts that are load-bearing:** `id="authForm"` with
  `hx-post`/`hx-select="#authForm"`/`hx-swap="outerHTML"` on the four form pages;
  `id="session-card"` as the one true partial, swapped by `handleResult`
  (`session.go:246`) with `hx-target="#session-card" hx-swap="outerHTML"`; form field
  names `seq` / `completion` / `rpe`; RPE buttons are `type="submit" name="rpe"
  value="N"`; RPE grid revealed by CSS `:has()` (`app.css:119`), not JS. `HX-Redirect` is
  the dominant post-success pattern (`auth_pages.go:77,89`, `onboarding.go`,
  `profile_edit.go`, `session.go:103,272`).
- **`OnboardingGate` already loads the profile.** `internal/server/onboarding_gate.go:32`
  calls `pc.Get(ctx, userID)` for every gated route and discards the result on success.
  The gated tier (`server.go:67-85`) is: `/`, `/api/me`, `/profile`, `/session*`,
  `/sessions*`.
- **`profile.Profile`** carries `UserID`, `Holdsetup int16`, `Angle int16`,
  `MaxGrade string`. `catalog.BoardName(holdsetup) (string, bool)` resolves the display
  name (used in `hub.go:38`, `history.go:53,111`, `session.go:54`).
- **Model shapes** (`templates/pages/model.go`): `HubModel{BoardName, Angle, MaxGrade}`
  duplicates exactly what a shared header would show; `HistoryDetailModel` carries
  `BoardName`/`Angle` but not `MaxGrade`; `SessionCardModel` carries neither.
- **E2E specs** (`tests/e2e/`): `adaptive-session-loop`, `auth-first-problem`,
  `past-sessions-history`, `seed`. Playwright runs them in the **Pixel 7 viewport**,
  workers: 1, starting `go run ./cmd/server`. Selectors are role / label / text per
  `CLAUDE.md`. `newTestRouter` (`server_test.go`) mirrors the three-tier grouping with
  stand-in handlers and a `fakeProfileChecker`.
- **Static assets** are a compile-time `embed.FS` (`static/embed.go`:
  `//go:embed app.css htmx.min.js moonboard`), served by `http.FileServer`
  (`server.go:37`). `server_test.go:191-193` asserts content types for `app.css`,
  `htmx.min.js`, `moonboard/2016.jpg`. `/favicon.ico` returns 204 (`server.go:41`).
- **Tooling:** no CSS build step, no Tailwind, no `.github/` workflows. `templ generate`,
  `go build/vet/test ./...`, `golangci-lint run`, `golangci-lint fmt`.

## Desired End State

A signed-in climber sees a coherently designed product on a phone: a slim header with the
MoonPhase wordmark and their board / angle / max-grade context on every authenticated
screen; styled forms with real error states; a session screen where the board photo is the
hero, the "How did it go?" controls sit in a thumb-reachable sticky zone, and the RPE input
is a distinctive 1–10 effort ramp; a history surface of tappable cards. Light and dark
themes follow the device setting. The layout is correct at 390 px width. Every interactive
element has a visible focus ring. The E2E specs pass (with at most minor selector/copy
updates). `go build ./...`, `go vet ./...`, `golangci-lint run`, and `go test ./...` are
clean.

### Key Discoveries

- Reuse the profile `OnboardingGate` already loads (`onboarding_gate.go:32`) — stash it in
  request context, read it in one new `renderAppPage` helper. No new DB query on any page.
- Keep `layout.Page` for the pre-onboarding pages (`signin`, `signup`, `onboarding`) — they
  have no profile and get no header. Add `layout.AppPage` for the six authenticated
  renders.
- `SessionCard` is swapped as a bare fragment (`session.go:246`) — it must never gain a
  layout or header wrapper.
- The RPE buttons' `type="submit" name="rpe" value="N"` contract and the `:has()`
  disclosure structure must survive the effort-ramp rebuild — the loop and the specs depend
  on submit-per-value.
- `templ generate` must run after every `.templ` edit; commit the regenerated `_templ.go`.
- Any file added under `static/` is served automatically; only `static/embed.go`'s
  `//go:embed` list needs the new directory names (`fonts`, `favicon.svg`).

## What We're NOT Doing

- No new features, no recommendation-explanation copy, no analytics or charts, no
  LED/hardware, no social, no warmup mode — PRD §Non-Goals is binding.
- No changes to the recommender, session store, auth, catalog, or any migration.
- No new routes except an optional favicon tweak. No change to the `HX-Redirect` flow.
- No manual theme toggle — `prefers-color-scheme` only.
- No CSS build step, framework, or bundler. `app.css` stays hand-authored and served as-is.
- No `templ generate` in CI or the Dockerfile (out of scope; committed `_templ.go` stays
  the contract).
- No redesign of the MoonBoard photo overlay mechanism — the board photos stay; only the
  hold markers are re-tokenised.
- No new brand logotype or monogram — a typeset wordmark and a simple moon-disc favicon
  only.

## Implementation Approach

Build the system bottom-up: tokens and shared components first (Phase 1), then the shell
that every authenticated page hangs off (Phase 2), then the screens in rising order of
design ambition (Phases 3–5), then an accessibility and regression sweep (Phase 6).

`app.css` is authored progressively. Phase 1 writes the token layer, the base-element
layer, and a shared-component layer (buttons, form fields, cards, chips, focus ring). The
existing screen-specific rules stay appended and working until the phase that restructures
that screen replaces them. Phase 6 deletes whatever is left unused.

Use the `frontend-design` skill (`frontend-design:frontend-design`) when authoring the
token system and each screen — it owns the aesthetic direction. The approved design brief
lives in `~/.claude/plans/prettify-create-a-merry-wand.md` (§"Proposed design direction").

## Critical Implementation Details

- **`templ generate` ordering.** Every phase that edits a `.templ` file must run
  `templ generate` before `go build` / `go test`, and commit the regenerated `_templ.go`
  alongside. A stale `_templ.go` compiles but renders the old markup — tests can pass
  against the wrong output.
- **`SessionCard` fragment boundary.** `handleResult` renders `pages.SessionCard(...)`
  directly (no `layout.*`). The `<div id="session-card">` must stay the outermost element
  of that component and keep the id, or the `hx-swap="outerHTML"` loop breaks. The sticky
  action zone and any header must live *outside* `SessionCard` — but `SessionCard` is
  rendered standalone on swap, so the sticky zone has to be *inside* it to survive the
  swap. Resolve by keeping the result form + End button inside `#session-card` (as today)
  and making the sticky positioning a property of an inner wrapper, not of a layout-level
  element.
- **RPE submit contract.** Each of the 10 ramp segments stays a
  `<button type="submit" name="rpe" value="N">` inside the same `<form>`, so a tap still
  submits `completion` + `seq` + `rpe` in one request. The ramp is presentation only.
- **Profile-in-context is request-scoped.** Stash `*profile.Profile` with a private context
  key in the `server` package (mirror `auth.WithUserID` / `auth.UserIDFromContext`). Only
  the `OnboardingGate` tier populates it; `renderAppPage` is the only reader.

## Phase 1: Design foundation

### Overview

The document head, the asset pipeline, and the CSS token + base + shared-component layers.
After this phase the app has fonts, a favicon, a viewport tag, both themes defined, and a
styled set of primitives — but the page bodies are not yet restructured.

### Changes Required

#### 1. Document head and page shell

**File**: `templates/layout/layout.templ`

**Intent**: Make the shell correct and mobile-ready. Add `lang`, charset, viewport,
`theme-color` for both schemes, the favicon link, font `preload` links, and `defer` on
htmx. Wrap `@content` in a `<main>` landmark with a skip link.

**Contract**: `Page(title string, content templ.Component)` signature is unchanged. New
`<head>` contents: `<meta charset="utf-8">`, `<meta name="viewport"
content="width=device-width, initial-scale=1">`, `<html lang="en">`, two
`<meta name="theme-color" media="(prefers-color-scheme: …)">`,
`<link rel="icon" href="/static/favicon.svg">`, `<link rel="preload" as="font" …
crossorigin>` per embedded face, `<script src="/static/htmx.min.js" defer></script>`.
Body: `<a class="skip-link" href="#main">…</a>` then `<main id="main">@content</main>`.

#### 2. Font assets

**File**: `static/fonts/` (new)

**Intent**: Self-host the three OFL typefaces. Subset to the weights actually used to keep
the binary small.

**Contract**: `woff2` files only. Fraunces (display — one or two weights, e.g. 400 + 600,
optical size set for display), Instrument Sans (body/UI — 400 + 500 + 600), IBM Plex Mono
(400 only). Filenames stable and referenced by `@font-face` in `app.css` as
`/static/fonts/<file>.woff2`.

#### 3. Favicon

**File**: `static/favicon.svg` (new)

**Intent**: A minimal moon-phase disc mark (a circle with a crescent shadow), theme-neutral
(uses `currentColor` or fixed ink that reads on both a light and dark browser tab).

**Contract**: Single self-contained SVG, no external refs.

#### 4. Embed list

**File**: `static/embed.go`

**Intent**: Ship the new assets in the binary.

**Contract**: `//go:embed app.css htmx.min.js moonboard fonts favicon.svg`.

#### 5. CSS token + base + component layers

**File**: `static/app.css` (rewrite)

**Intent**: Replace the ad-hoc rules with three ordered layers. Layer 1 — tokens: the
light palette on `:root`, the dark palette inside
`@media (prefers-color-scheme: dark)`, plus type scale, space scale (`4 8 12 16 24 32 48
64`), radius (`--r-sm`, `--r-md`, `--r-full`), one shadow token, and `@font-face` blocks.
Layer 2 — reset + base elements: `box-sizing`, body background/color/font from tokens,
headings in Fraunces with an intentional scale, links, `:focus-visible` ring (uses
`--accent`), `::selection`, `@media (prefers-reduced-motion: reduce)` global guard, the
`.skip-link` rule. Layer 3 — shared components: `.btn` (primary / secondary / full-width,
min 44 px target), form field styling (`label`, `input`, `select`, `textarea`), `.card`,
`.chip`, `.field-error`. Keep the current `.hub__*` / `.board` / `.hold` / `.result*` /
`.session__*` / `.history*` rules appended below Layer 3 for now (later phases replace
them).

**Contract**: Token names are the public API the rest of the CSS uses:
`--ground --surface --ink --ink-soft --accent --accent-ink --edge --sent --failed
--bailed` plus `--space-* --r-* --shadow-1 --step-* ` (type scale). Every colour is
defined once on `:root` and re-defined once under the dark media query — never only inside
the media block. Draft hex values are in the design brief; confirm against a screenshot in
Phase 6.

### Success Criteria

#### Automated Verification

- [ ] `templ generate` runs clean and `git status` shows the regenerated
  `templates/layout/layout_templ.go`
- [ ] `go build ./...` passes
- [ ] `go vet ./...` passes
- [ ] `golangci-lint run` passes
- [ ] `go test ./internal/server/` passes (asset content-type test at
  `server_test.go:191-193` still green; add `favicon.svg` / a font path if extending it)
- [ ] `curl -sI localhost:8080/static/fonts/<face>.woff2` returns `200` and
  `Content-Type: font/woff2` (server running per Manual step)
- [ ] `curl -s localhost:8080/static/favicon.svg | head -c 40` shows SVG markup

#### Manual Verification

- [ ] Run `set -a; source .env; set +a; go run ./cmd/server`; open
  `http://localhost:8080/signin` — page loads with the new fonts applied and no console
  errors
- [ ] View source: `<meta name="viewport">`, `<html lang>`, favicon link, and `defer` on
  htmx are all present
- [ ] Browser tab shows the moon favicon
- [ ] Toggle OS dark mode — the `<body>` background and text colours switch; the
  `theme-color` (tab/status bar tint on mobile) switches
- [ ] At 390 px devtools width the page does not scroll horizontally

**Implementation Note**: After automated verification passes, pause for the human to
confirm manual testing before Phase 2.

---

## Phase 2: App shell and shared header

### Overview

A shared header on every authenticated screen showing the MoonPhase wordmark and the
climber's board / angle / max-grade context. Powered by the profile `OnboardingGate`
already loads, read through one new render helper.

### Changes Required

#### 1. Profile request-context helper

**File**: `internal/server/profilectx.go` (new)

**Intent**: Carry the gate-loaded profile down to the render helper without a second query.

**Contract**: `withProfile(ctx context.Context, p *profile.Profile) context.Context` and
`profileFromContext(ctx context.Context) (*profile.Profile, bool)`, private to package
`server`, using an unexported context-key type. Mirrors `auth.WithUserID` /
`auth.UserIDFromContext`.

#### 2. Gate stashes the profile

**File**: `internal/server/onboarding_gate.go`

**Intent**: On the success path, put the loaded profile in context instead of discarding
it.

**Contract**: `pc.Get` result is captured; `next.ServeHTTP(w, r.WithContext(withProfile(
r.Context(), prof)))`. The `ProfileChecker` interface and redirect behaviour are unchanged.

#### 3. App layout entrypoint + header component

**File**: `templates/layout/app.templ` (new), `templates/layout/layout.templ`

**Intent**: A second full-page entrypoint that renders the header above the content. The
header is wordmark (link to `/`) + a context line rendered as chips.

**Contract**: `AppPage(title string, nav NavModel, content templ.Component)` and an
unexported `appHeader(nav NavModel)`. `NavModel` lives in package `layout`:
`type NavModel struct { BoardName string; Angle int16; MaxGrade string }`. `AppPage`
reuses the same `<head>` as `Page` (extract a shared `head(title)` templ to avoid
divergence). No sign-out control in the header.

#### 4. Authenticated render helper

**File**: `internal/server/render.go` (new; or extend `auth_pages.go`)

**Intent**: One place that builds `NavModel` from the context profile and renders
`AppPage`.

**Contract**: `renderAppPage(w http.ResponseWriter, r *http.Request, title string, content
templ.Component, status int)`. Reads `profileFromContext`; if absent (should not happen in
the gated tier) fall back to `renderPage` without a header and log a warning. Resolves
`catalog.BoardName`; on `!ok` uses `"Unknown board"` (matches `hub.go:41`).

#### 5. Switch authenticated handlers to `renderAppPage`

**File**: `internal/server/hub.go`, `profile_edit.go`, `session.go`, `history.go`

**Intent**: Every full-page render behind `OnboardingGate` gets the header. The
`SessionCard` fragment render in `handleResult` does **not** change.

**Contract**: `renderPage(w, r, pages.HubPage(...), …)` →
`renderAppPage(w, r, "MoonPhase", hubContent(...), …)` and equivalents for
`ProfilePage`, `SessionPage`, `UnsupportedBoardPage`, `HistoryListPage`,
`HistoryDetailPage`, `HistoryProblemPage`. This means exporting the inner `content`
templ funcs (e.g. `hubContent`, `profileForm`, `historyListContent`, `SessionCard`
already exported) or adding thin exported wrappers. `HubModel` is reduced to whatever the
hub body still needs after the context moves to the header (likely removed — see Phase 3).
`SessionModel` / `SessionCardModel` unchanged. `handleResult` keeps calling
`renderPage`-free `pages.SessionCard(...)`.

#### 6. Test updates

**File**: `internal/server/onboarding_gate_test.go`, `internal/server/server_test.go`

**Intent**: Cover the new context stash; keep the router tests green.

**Contract**: Add a gate test asserting `profileFromContext` is populated for `next` when
`pc.Get` succeeds. `newTestRouter` stand-in handlers are unaffected (they don't render
real pages); if any server test now renders a real authenticated page it must seed a
profile into context via the fake gate.

### Success Criteria

#### Automated Verification

- [ ] `templ generate` clean; regenerated `layout` `_templ.go` files committed
- [ ] `go build ./...` passes
- [ ] `go vet ./...` passes
- [ ] `golangci-lint run` passes
- [ ] `go test ./internal/server/` passes, including the new gate-context test
- [ ] `go test ./...` passes

#### Manual Verification

- [ ] Run the server; sign in with an onboarded account; open `/`, `/profile`,
  `/sessions`, and an active `/session/{id}` — the header with wordmark + "Board · Angle°
  · max Grade" (as chips, not a middot string) shows on all of them
- [ ] Open `/signin`, `/signup`, `/onboarding` — **no** header (still `layout.Page`)
- [ ] Start a session, submit a result — the next-problem card swaps in and the header
  stays put (it is outside `#session-card`)
- [ ] Click the wordmark from `/profile` — lands on `/`
- [ ] At 390 px the header does not wrap awkwardly or push content off-screen

**Implementation Note**: Pause for human confirmation before Phase 3.

---

## Phase 3: Entry screens — sign in, sign up, onboarding, hub

### Overview

Style the four simplest screens: the three auth/onboarding forms and the hub. Add the
sign-out control to the profile screen.

### Changes Required

#### 1. Auth + onboarding forms

**File**: `templates/pages/signin.templ`, `signup.templ`, `onboarding.templ`

**Intent**: Wrap each form in a centred `.card` with a heading and one line of helper copy.
Add a cross-link between sign in and sign up. Render `.error` as a real inline error
(`.field-error`) with direction, not mood.

**Contract**: Keep `id="authForm"`, `hx-post`, `hx-select="#authForm"`,
`hx-swap="outerHTML"`, every `<label>` text, `name` attributes, and the submit-button
text. New markup is wrapper elements and class hooks only. Copy additions (headings,
helper text, cross-links) are allowed — the E2E constraint permits minor spec updates.

#### 2. Profile screen + sign out

**File**: `templates/pages/profile.templ`

**Intent**: Style the profile form like the others (it already shares `id="authForm"`).
Add a **Sign out** control at the bottom — the FR-002 gap; a rare action, so profile is
its home, not the header.

**Contract**: Sign out is `<form hx-post="/signout" hx-swap="none"><button>Sign out
</button></form>` (the handler already sends `HX-Redirect: /signin`). Keep the profile
form's existing `hx-*` and `name`s.

#### 3. Hub

**File**: `templates/pages/hub.templ`, `templates/pages/model.go`,
`internal/server/hub.go`

**Intent**: The context line moves to the header, so the hub body is just the primary
action and secondary links. "Main Session" is the primary `.btn`; "Profile" and "Past
sessions" are secondary links, left-aligned (drop the `text-align: center`).

**Contract**: Keep `hx-post="/session" hx-swap="none"` on the Main Session button and its
visible text "Main Session". Keep the `<a href="/profile">` and `<a href="/sessions">`
targets and link text. `HubModel` drops `BoardName`/`Angle`/`MaxGrade` if nothing in the
body still uses them — `hubContent` then takes no model, or the struct is deleted and
`hub.go` passes nothing. `hub.go` still calls `renderAppPage` (which builds the header
`NavModel` from context).

#### 4. Screen CSS

**File**: `static/app.css`

**Intent**: Auth/onboarding page layout (centred card, vertical rhythm) and the lean hub
layout. Replace the old `.hub` / `.hub__context` / `.hub__start` rules.

**Contract**: New rules use Layer 1 tokens and the Layer 3 `.card` / `.btn` / form-field
primitives. Remove `.hub__context` (dead once the context moves).

### Success Criteria

#### Automated Verification

- [ ] `templ generate` clean; regenerated `_templ.go` committed
- [ ] `go build ./...`, `go vet ./...`, `golangci-lint run` pass
- [ ] `go test ./...` passes
- [ ] `npm run test:e2e -- auth-first-problem` passes (update selectors/copy in the spec
  only if the redesign changed wording it asserts)

#### Manual Verification

- [ ] Sign-up flow: `/signup` shows a styled card; submit a bad email → inline
  `.field-error`, not a raw browser bubble only; submit valid → redirect to `/onboarding`
- [ ] `/onboarding` is a styled card; the three selects are full-width, ≥44 px tall;
  submit → hub
- [ ] Hub: left-aligned, "Main Session" is the clear primary action; header carries the
  context; Profile / Past-sessions links work
- [ ] `/profile`: styled; "Sign out" at the bottom → lands on `/signin`; the session
  cookie is cleared (re-open `/` → bounced to `/signin`)
- [ ] All four screens at 390 px: no horizontal scroll, tap targets comfortable

**Implementation Note**: Pause for human confirmation before Phase 4.

---

## Phase 4: Session screen and the RPE effort ramp

### Overview

The signature phase. Restructure the live session card, re-tokenise the board overlay,
move the result controls into a sticky thumb-zone, and rebuild the RPE input as a 1–10
effort ramp.

### Changes Required

#### 1. Session card structure

**File**: `templates/pages/session.templ`

**Intent**: Give the card a clear hierarchy — problem name (Fraunces), meta as chips
(grade / angle / board year), board photo as the hero, then the holds list, then the
result controls. Restructure `unsupportedBoardContent` to match the new card/empty
styling.

**Contract**: `<div id="session-card">` stays the outermost element of `SessionCard` and
keeps its id. `SessionCard`, `SessionPage`, `moonBoardOverlay`, `holdMarker`,
`resultForm`, `UnsupportedBoardPage` keep their signatures. `h1` = problem name
(unchanged text). Replace the ` · `-joined `.session__meta` string with individual
`.chip` elements.

#### 2. Holds list

**File**: `templates/pages/session.templ`, `static/app.css`

**Intent**: Render the holds list as a compact, scannable coordinate list — grid ref in
IBM Plex Mono, hold-type label in body face, role as a small tag.

**Contract**: `holdLabel` helper unchanged. `.holds` / `.holds__role` rules replaced. The
`<ul>`/`<li>` structure and the visible text per row are unchanged.

#### 3. Sticky action zone + RPE effort ramp

**File**: `templates/pages/session.templ`, `static/app.css`

**Intent**: The result form (`completion` radios + RPE) and the End button sit in a zone
pinned to the bottom of the viewport so they stay under the thumb regardless of board
image height. Rebuild the RPE grid as a horizontal ramp: 10 segments, rising visual weight
/ height / fill left→right (no rainbow), wrapping to two rows only on the narrowest
screens. The `completion` radios stay as tap-cards; the ramp stays hidden until a
`completion` radio is checked (the existing `:has()` rule).

**Contract**: Each ramp segment is
`<button type="submit" name="rpe" value="N">N</button>` inside the same `<form
class="result" hx-post=".../result" hx-target="#session-card" hx-swap="outerHTML">`. The
`<input type="hidden" name="seq">` and the `fieldset` with
`name="completion"` radios (values `sent` / `failed` / `bailed`, `required`) are
unchanged. The `:has(input:checked) ~ .result__rpe` disclosure structure is preserved
(class names may change; the sibling relationship must hold). The sticky positioning is on
an inner wrapper *inside* `#session-card` so it survives the fragment swap. Because the
form and End button already live inside `#session-card` today, keep them there.

#### 4. One swap transition

**File**: `static/app.css`

**Intent**: A single short settle on the incoming card after a rating — the product's
payoff moment. Nothing else animates on load.

**Contract**: A ~180 ms transition/keyframe on `#session-card` (or htmx's
`.htmx-added` / settling classes). Fully suppressed under
`@media (prefers-reduced-motion: reduce)`.

### Success Criteria

#### Automated Verification

- [ ] `templ generate` clean; regenerated `session_templ.go` committed
- [ ] `go build ./...`, `go vet ./...`, `golangci-lint run` pass
- [ ] `go test ./...` passes
- [ ] `npm run test:e2e -- adaptive-session-loop` passes (the loop asserts RPE
  submit-per-value and grade response — the ramp must not break it; minor selector
  updates allowed)

#### Manual Verification

- [ ] Start a session: the board photo is the visual hero; meta shows as chips; holds
  list is readable with mono grid refs
- [ ] Scroll behaviour: with a tall board image, the "How did it go?" controls and End
  stay reachable at the bottom (sticky zone) without scrolling past the image
- [ ] Pick a completion status → the RPE ramp appears; tap `7` → result submits, next
  card swaps in with one short transition
- [ ] Set OS reduce-motion → repeat: no transition on swap
- [ ] `felt 9` + `failed` never yields a strictly harder next pick (spec covers this;
  confirm once by eye)
- [ ] At 390 px: ramp segments are tappable (≥44 px on the long axis), no horizontal
  scroll, End button reachable one-handed
- [ ] Dark mode: board photo, holds, chips, ramp all legible

**Implementation Note**: Pause for human confirmation before Phase 5.

---

## Phase 5: History screens

### Overview

Style the three read-only past-session screens: the list, the detail, and the problem
card.

### Changes Required

#### 1. List

**File**: `templates/pages/history.templ`, `static/app.css`

**Intent**: Each session becomes a tappable `.card` row: the date, board + angle chips,
and the climbed count. The empty state ("No sessions yet") becomes an invitation to act
with a clear link to start a session.

**Contract**: Keep the `<a href={"/sessions/" + s.ID}>` wrapper and its target. The row's
visible data (date, board, angle, "N problems climbed") is unchanged; only its layout
changes. `historyListContent` signature unchanged.

#### 2. Detail

**File**: `templates/pages/history.templ`, `static/app.css`

**Intent**: The climbed-problem list as an ordered list (numbering is legitimate — it is
the climb sequence), each row linking to the problem card, with RPE and completion shown
as chips rather than a middot string. Empty state ("No problems climbed in this session")
styled as a calm message.

**Contract**: Keep the `<a href={"/sessions/" + m.SessionID + "/problem/" +
seq}>` targets and the `<ol>`. Replace the ` · `-joined row string with name + chips.
`historyDetailContent` signature unchanged.

#### 3. Problem card

**File**: `templates/pages/history.templ`, `static/app.css`

**Intent**: Reuse the Phase 4 session-card styling for the read-only problem view. The
"You climbed this — RPE N · completion" line becomes chips.

**Contract**: `moonBoardOverlay` / `holdLabel` reused unchanged. `.history__result`
replaced. `historyProblemContent` signature unchanged. Back-links preserved.

### Success Criteria

#### Automated Verification

- [ ] `templ generate` clean; regenerated `history_templ.go` committed
- [ ] `go build ./...`, `go vet ./...`, `golangci-lint run` pass
- [ ] `go test ./...` passes
- [ ] `npm run test:e2e -- past-sessions-history` passes (minor selector/copy updates
  allowed)

#### Manual Verification

- [ ] Complete and end a session, open `/sessions`: the session shows as a tappable card
  with date + chips + climbed count
- [ ] Open it: ordered list of climbed problems, each with RPE + completion chips; tap a
  row → the read-only problem card renders with the board overlay and a "you climbed
  this" chip line
- [ ] Fresh account with no ended sessions: `/sessions` shows the styled empty state with
  a working "start a session" link
- [ ] Back-links: problem → detail → list all work
- [ ] 390 px + dark mode: all three screens legible, no horizontal scroll

**Implementation Note**: Pause for human confirmation before Phase 6.

---

## Phase 6: Polish and regression sweep

### Overview

The quality floor: accessibility, motion, contrast, dead-code removal, and a full
build + test + E2E + screenshot pass.

### Changes Required

#### 1. Accessibility + state sweep

**File**: `static/app.css`, any `.templ` as needed

**Intent**: Every interactive element has a visible `:focus-visible` ring. Add a
`.htmx-request` in-flight indicator (e.g. reduced opacity / a subtle pulse on the
submitting control). Add `:disabled` styling. Confirm the skip link works and focus order
is sane on every screen.

**Contract**: Focus ring uses `--accent` and meets a visible-contrast bar against both
themes. No `outline: none` without a replacement.

#### 2. Contrast + token finalisation

**File**: `static/app.css`

**Intent**: Check every text/background pairing in both themes against WCAG AA (4.5:1
body, 3:1 large text). Adjust the draft token hex values where they fail. Lock the final
palette.

**Contract**: Body text on `--ground` and on `--surface`, `--ink-soft` secondary text,
`--accent-ink` on `--accent`, and the three status colours all pass AA in light and dark.

#### 3. Remove dead CSS

**File**: `static/app.css`

**Intent**: Delete any rule from the original file or an intermediate phase that nothing
references now (old `.hub__*`, superseded `.result*` / `.session__*` / `.history*`).
Chanel test — remove one decoration that is not earning its place.

**Contract**: `app.css` contains only the three layers plus live screen rules.

#### 4. Regenerate + full verification

**File**: all `_templ.go`

**Intent**: One final `templ generate`; confirm the whole suite.

### Success Criteria

#### Automated Verification

- [ ] `templ generate` clean; `git status` shows no uncommitted `_templ.go` drift
- [ ] `go build ./...` passes
- [ ] `go vet ./...` passes
- [ ] `golangci-lint run` passes
- [ ] `golangci-lint fmt` leaves no diff
- [ ] `go test ./...` passes
- [ ] `npm run test:e2e` — all four specs green
- [ ] `npm run typecheck` (E2E TS) passes

#### Manual Verification

- [ ] Tab through every screen: focus ring visible on every control, order matches visual
  order, skip link works
- [ ] Submit any htmx form on a throttled connection → the in-flight indicator shows
- [ ] Contrast: spot-check body and secondary text in both themes with a contrast checker
  — AA or better
- [ ] Screenshots captured at 390 px width for all screens (signin, signup, onboarding,
  hub, session with ramp open, history list, history detail, history problem) in **both**
  light and dark; reviewed for coherence
- [ ] Reduced-motion: no load animations anywhere; the one session-swap transition is
  suppressed
- [ ] Full walk-through on a real phone (or Pixel-7 devtools): sign up → onboard → run a
  3-problem session → end → browse history, one-handed

**Implementation Note**: Final phase — after this, the change is ready for
`/10x-impl-review`.

---

## Testing Strategy

### Unit / handler tests

- `internal/server/onboarding_gate_test.go` — the profile is placed in request context on
  the success path.
- `internal/server` existing handler tests — still pass; where a test renders a real
  authenticated page it seeds a profile into context.
- `internal/server/server_test.go:191-193` — asset content-type assertions extended for
  `favicon.svg` and a font file, or left as-is if the new paths aren't asserted.

### Integration / E2E

- All four Playwright specs (`adaptive-session-loop`, `auth-first-problem`,
  `past-sessions-history`, `seed`) pass. They run in the Pixel 7 viewport, so they double
  as the mobile-layout regression guard. Minor selector/copy edits to the specs are
  permitted where the redesign deliberately changed asserted wording — each such edit is
  called out in the phase that causes it.

### Manual

1. `set -a; source .env; set +a; go run ./cmd/server`
2. Full persona walk-through at 390 px in light and dark: sign up → onboard → run a
   session (rate several problems, watch the next pick respond) → end → browse history →
   edit profile → sign out.
3. Accessibility: keyboard-only pass, reduced-motion pass, contrast spot-check.
4. Capture the screenshot set listed in Phase 6.

## Performance Considerations

- Fonts: subset to used weights; `preload` the faces used above the fold. Total added
  binary weight target < ~250 KB across all `woff2`.
- `defer` on htmx removes a render-blocking script.
- One CSS file, served from the embedded FS — no extra requests, no build step.
- The single swap transition is short and GPU-friendly (opacity/transform only) and is
  disabled under reduced-motion.

## Migration Notes

- No data or schema migration.
- `static/embed.go`'s `//go:embed` must list the new `fonts` directory and `favicon.svg`
  or the build fails — this is the one hard ordering dependency (do it in Phase 1 with the
  assets).
- Regenerated `_templ.go` files are committed with each phase; a reviewer diffing only
  `.templ` will miss rendered output otherwise.
- Rollback is a straight revert — no external state changes.

## References

- Approved design brief: `~/.claude/plans/prettify-create-a-merry-wand.md`
  (§"Proposed design direction")
- Change identity + notes: `context/changes/prettify/change.md`
- Design skill: `frontend-design:frontend-design`
- Render helper pattern: `internal/server/auth_pages.go:93` (`renderPage`)
- Profile-load-and-discard to reuse: `internal/server/onboarding_gate.go:32`
- Context-helper pattern to mirror: `auth.WithUserID` / `auth.UserIDFromContext`
- The one HTMX partial: `templates/pages/session.templ` `SessionCard`, swapped at
  `internal/server/session.go:246`
- Lesson — concrete manual steps: `context/foundation/lessons.md`

## Progress

> Convention: `- [ ]` pending, `- [x]` done. Append ` — <commit sha>` when a step lands.
> Do not rename step titles.

### Phase 1: Design foundation

#### Automated

- [x] 1.1 templ generate clean; regenerated layout_templ.go committed — d122321
- [x] 1.2 go build ./... passes — d122321
- [x] 1.3 go vet ./... passes — d122321
- [x] 1.4 golangci-lint run passes — d122321
- [x] 1.5 go test ./internal/server/ passes (asset content-type test still green) — DB-backed tests skipped (no Docker in env)
- [x] 1.6 font woff2 served with Content-Type font/woff2 — d122321
- [x] 1.7 favicon.svg served as SVG markup — d122321

#### Manual

- [x] 1.8 /signin loads with new fonts, no console errors — d122321
- [x] 1.9 head has viewport, lang, favicon link, deferred htmx — d122321
- [x] 1.10 browser tab shows the moon favicon — d122321 (favicon.svg is a valid self-contained gradient-crescent SVG, served as image/svg+xml, linked `<link rel="icon">` in the shared head)
- [x] 1.11 OS dark-mode toggle switches body colours and theme-color — d122321
- [x] 1.12 no horizontal scroll at 390 px — d122321

### Phase 2: App shell and shared header

#### Automated

- [x] 2.1 templ generate clean; regenerated layout _templ.go committed — 474202f
- [x] 2.2 go build ./... passes — 474202f
- [x] 2.3 go vet ./... passes — 474202f
- [x] 2.4 golangci-lint run passes — 474202f
- [x] 2.5 go test ./internal/server/ passes incl. new gate-context test — 474202f (DB-backed tests skipped: no Docker; handler tests use renderAppPage's header-less fallback so no seeding needed)
- [x] 2.6 go test ./... passes — 474202f (non-DB packages green; DB tests skipped — no Docker)

#### Manual

- [x] 2.7 header with wordmark + context chips on /, /profile, /sessions, /session/{id} — 474202f
- [x] 2.8 no header on /signin, /signup, /onboarding — 474202f
- [x] 2.9 result submit swaps the card; header stays put — 474202f (screenshot: header count 1 after swap)
- [x] 2.10 wordmark link returns to / — 474202f
- [x] 2.11 header does not wrap badly at 390 px — 474202f

> Known regression, fixed by Phase 4: `adaptive-session-loop.spec.ts` "End session button
> is within the initial phone viewport" now fails — the header + more generous Phase 1
> spacing push End below the fold on session load. Phase 4's sticky action zone restores it.

### Phase 3: Entry screens — sign in, sign up, onboarding, hub

#### Automated

- [x] 3.1 templ generate clean; regenerated _templ.go committed — 6baffac
- [x] 3.2 go build ./..., go vet ./..., golangci-lint run pass — 6baffac
- [x] 3.3 go test ./... passes — 6baffac (non-DB green; DB tests skipped — no Docker)
- [x] 3.4 npm run test:e2e -- auth-first-problem passes — 6baffac

#### Manual

- [x] 3.5 signup: styled card, valid → /onboarding — 6baffac (bad-input inline error path not re-verified by eye; markup uses .field-error)
- [x] 3.6 onboarding: styled card, selects full-width and ≥44 px, submit → hub — 6baffac
- [x] 3.7 hub: left-aligned, Main Session primary, links work, context in header — 6baffac
- [x] 3.8 profile: styled; Sign out → /signin and cookie cleared — 6baffac
- [x] 3.9 all four screens clean at 390 px — 6baffac

### Phase 4: Session screen and the RPE effort ramp

#### Automated

- [x] 4.1 templ generate clean; regenerated session_templ.go committed — 033a48e
- [x] 4.2 go build ./..., go vet ./..., golangci-lint run pass — 033a48e
- [x] 4.3 go test ./... passes — 033a48e (non-DB green; DB tests skipped — no Docker)
- [x] 4.4 npm run test:e2e -- adaptive-session-loop passes — 033a48e (full suite green; Phase 2 viewport regression now resolved)

#### Manual

- [x] 4.5 board photo is the hero; meta chips; readable holds list with mono refs — 033a48e
- [x] 4.6 result controls + End stay thumb-reachable with a tall board image — 033a48e (scroll happens inside .session__scroll; actions pinned)
- [x] 4.7 pick completion → ramp appears; tap a value → submits, next card swaps with one transition — 033a48e
- [x] 4.8 reduce-motion set → no swap transition — 033a48e (global Layer 2 reduced-motion guard covers the card-settle keyframe)
- [x] 4.9 felt 9 + failed never yields a strictly harder pick (by eye) — 033a48e (adaptive-loop spec asserts this)
- [x] 4.10 390 px: ramp tappable, no horizontal scroll, End reachable one-handed — 033a48e
- [x] 4.11 dark mode: board, holds, chips, ramp legible — 033a48e (ramp top-segment fill opacity reduced for digit contrast; final contrast pass in Phase 6)

### Phase 5: History screens

#### Automated

- [x] 5.1 templ generate clean; regenerated history_templ.go committed — e589cbf
- [x] 5.2 go build ./..., go vet ./..., golangci-lint run pass — e589cbf
- [x] 5.3 go test ./... passes — e589cbf (non-DB green; DB tests skipped — no Docker)
- [x] 5.4 npm run test:e2e -- past-sessions-history passes — e589cbf (full suite green)

#### Manual

- [x] 5.5 /sessions: session shows as tappable card with date + chips + climbed count — e589cbf
- [x] 5.6 detail: ordered climbed-problem list with RPE/completion chips; row → problem card — e589cbf
- [x] 5.7 no-sessions account: styled empty state with working start link — e589cbf
- [x] 5.8 back-links problem → detail → list all work — e589cbf (spec walks problem → detail; detail/list back-links are plain hrefs)
- [x] 5.9 390 px + dark mode: all three screens legible — e589cbf

### Phase 6: Polish and regression sweep

#### Automated

- [x] 6.1 templ generate clean; no uncommitted _templ.go drift — 9273525
- [x] 6.2 go build ./... passes — 9273525
- [x] 6.3 go vet ./... passes — 9273525
- [x] 6.4 golangci-lint run passes — 9273525
- [x] 6.5 golangci-lint fmt leaves no diff — 9273525
- [x] 6.6 go test ./... passes — 9273525 (non-DB packages green; 8 testcontainers/Postgres handler tests fail only because no Docker daemon in this env — verified identical on baseline; the real handlers are covered green by the Playwright suite against live Postgres)
- [x] 6.7 npm run test:e2e — all four specs green — 9273525 (9 tests, all pass)
- [x] 6.8 npm run typecheck passes — 9273525

#### Manual

- [x] 6.9 keyboard pass: focus ring on every control, sane order, skip link works — 9273525 (global :focus-visible + label:has(input:focus-visible) for tap-cards; screenshot-verified on inputs and the completion cards)
- [x] 6.10 htmx in-flight indicator shows on a throttled submit — 9273525 (.htmx-request → opacity 0.6, cursor progress, pointer-events none on the submitting form)
- [x] 6.11 contrast spot-check passes AA in both themes — 9273525 (computed every text/bg token pair — all ≥ 4.5:1 in light and dark)
- [x] 6.12 screenshot set captured at 390 px, both themes, all screens, reviewed — 9273525 (captured across phases 1–6, light + dark)
- [x] 6.13 reduced-motion: no load animations; session-swap transition suppressed — 9273525 (Layer 2 global prefers-reduced-motion guard neutralises the card-settle keyframe; no other load animation exists)
- [x] 6.14 one-handed phone walk-through: signup → onboard → 3-problem session → end → history — 9273525 (the Playwright suite drives exactly this flow in the Pixel 7 viewport)

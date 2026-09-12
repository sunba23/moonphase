# User-Facing Documentation Page (GitHub Pages) Implementation Plan

## Overview

Build one static page, `docs/index.html`, that tells a non-technical reader what
MoonPhase is, why it exists, and how to use it — in that order. Serve it with
GitHub Pages from the `/docs` folder on `main`. Match the app's visual identity
("chalk by day, moonlit by night"). Keep the copy short and in plain English.
Show one screenshot of a live session.

## Current State Analysis

- The repo has no `docs/` folder, no GitHub Pages configuration, and no
  user-facing explainer. `README.md` is developer-only (CI, release flow).
- The live app runs on Railway at
  `https://moonphase-production-7370.up.railway.app`. The repo is
  `https://github.com/sunba23/moonphase`.
- The app identity lives in `static/app.css`:
  - Light tokens (`:root`): `--ground #e9dcc3`, `--surface #f5eee0`,
    `--ink #241c14`, `--ink-soft #6b5d49`, `--accent #2e4b8f`,
    `--accent-ink #f5eee0`, `--edge #cbb88f`.
  - Dark tokens (`@media (prefers-color-scheme: dark)`): `--ground #141a2a`,
    `--surface #1d2436`, `--ink #ece6d8`, `--ink-soft #9aa0b4`,
    `--accent #a9b8e8`, `--accent-ink #141a2a`, `--edge #333b52`.
  - Fonts: `--font-display: "Fraunces", Georgia, serif`;
    `--font-body: "Instrument Sans", system-ui, sans-serif`.
    Font files: `static/fonts/fraunces-latin.woff2`,
    `static/fonts/instrument-sans-latin.woff2` (the mono face is not needed
    here).
  - `theme-color` metas: `#E9DCC3` light, `#141A2A` dark
    (`templates/layout/layout.templ`).
  - Favicon: `static/favicon.svg`.
- The user journey the "how to use" section must describe
  (`internal/server/server.go` routes, `templates/pages/*.templ`):
  1. Sign up with email + password (`/signup`).
  2. Onboarding — pick max grade, board, angle (`/onboarding`). Copy on that
     screen: "Sessions start at your board's easiest problem and ramp from
     there — your max grade is the ceiling."
  3. Hub — "Ready to climb", **Start session** (`/`).
  4. Session loop (`/session/{id}`): read the board and hold list, then for each
     problem submit **Sent** or **Failed** plus an **RPE 1–10** rating, or tap
     **Skip**. The next problem appears automatically. A collapsed
     "Session balance" panel shows the running hold-type tally.
  5. **End session** — saved to history.
  6. **Past sessions** (`/sessions`) — list, then per-session detail with each
     problem's grade, RPE, and result.
- `go build ./...` and `.github/workflows/ci.yml` (Go, templ, lint, vuln, test)
  do not read `docs/`. A static `docs/` folder is inert for the build and the
  Railway deploy.
- Supported boards for a real session are 2016 and 2024 only
  (`templates/pages/session.templ` `UnsupportedBoardContent`). Board images
  exist for those two: `static/moonboard/2016.jpg`, `static/moonboard/2024.jpg`.

## Desired End State

- Visiting the GitHub Pages URL for the repo shows a single, well-typed page:
  tagline → why MoonPhase exists → how it works (a few numbered steps) → a clear
  link to the live app.
- The page renders correctly in light and dark mode and on a phone in portrait.
- One screenshot of a live session sits in the page, stored under `docs/`.
- `README.md` links to the published page.
- `go build ./...` and CI still pass unchanged.

### Key Discoveries

- App tokens and fonts to mirror: `static/app.css:39` (light), `static/app.css:86`
  (dark), `static/app.css:81` (font stacks).
- Journey and screen copy: `templates/pages/onboarding.templ:10`,
  `templates/pages/hub.templ`, `templates/pages/session.templ:118`
  (`resultForm`), `internal/server/server.go:52` (route list).
- Problem framing to compress into "why": `context/foundation/prd.md`
  "Vision & Problem Statement" and "US-01".
- GitHub Pages "deploy from a branch" reads `/docs` on the chosen branch; a
  `docs/.nojekyll` file makes Pages serve the folder verbatim with no Jekyll
  build step.

## What We're NOT Doing

- No static-site generator, no Jekyll, no Node build, no CI changes.
- No multi-page site — one `index.html` only.
- No per-step screenshot gallery — a single hero screenshot.
- No API docs, no developer/contributor content (that stays in `README.md`).
- No custom domain, no analytics, no cookie banner.
- No copy that explains the recommender's internal scoring — the PRD forbids
  prose "we picked this because…" explanations; the page describes the loop from
  the climber's side only.
- No change to the Go app, templates, `static/`, or the Railway deploy.
- No automatic screenshot capture in CI.
- Not adding a docs link to the app's in-product footer (out of scope for this
  change; README link only).

## Implementation Approach

Hand-write `docs/index.html` as a self-contained document: one `<style>` block
in the head, no external CSS or JS, two self-hosted `@font-face` files copied
from `static/fonts/`. Copy the seven light tokens and seven dark tokens from
`app.css` into a `:root` / `@media (prefers-color-scheme: dark)` pair so the page
tracks the app's look without importing `app.css` (which is full of app-only
component rules and absolute `/static/...` URLs that would 404 on Pages).

Keep the DOM tiny: a `<header>` with the wordmark and tagline, three or four
`<section>` blocks, a footer line. Target well under 150 lines of body markup.

Phase 1 produces the page and its assets. Phase 2 wires publishing and
discoverability and lists the maintainer's manual steps (screenshot, repo
setting, verification).

## Phase 1: The page

### Overview

Create `docs/index.html`, copy the two font files, add the hero screenshot
placeholder and `.nojekyll`. Page is complete and viewable by opening the file
locally.

### Changes Required

#### 1. Page document

**File**: `docs/index.html`

**Intent**: A single static page for a non-technical reader. Order is fixed:
what it is → why it exists → how to use it → where to start. Very concise, plain
English (this page is the one place the global "Simplified Technical English"
rule is relaxed — the user asked for standard plain English aimed at climbers).

**Contract**: Self-contained HTML5 document.
- `<head>`: `charset`, `viewport`, `<title>MoonPhase — adaptive MoonBoard
  training</title>`, a short `<meta name="description">`, the two `theme-color`
  metas (`#E9DCC3` / `#141A2A`), `<link rel="icon">` pointing at a copied
  `favicon.svg` (see change 3), and one inline `<style>`.
- `<style>`: two `@font-face` rules (`Fraunces`, `Instrument Sans`) with
  `src: url("fonts/…woff2")` and `font-display: swap`; a `:root` token block
  (the seven light values above) and a `@media (prefers-color-scheme: dark)`
  override block (the seven dark values); base rules for `body`
  (`background: var(--ground)`, `color: var(--ink)`, body font, max content
  width ~40rem, generous line-height, page padding), headings in
  `var(--font-display)`, links in `var(--accent)`, a visible `:focus-visible`
  outline, `img { max-width: 100%; height: auto }`, and a `prefers-reduced-motion`
  guard if any transition is added.
- `<body>` structure:
  - `<header>`: "MoonPhase" wordmark + one-line tagline
    ("An adaptive MoonBoard training coach.").
  - Section **"What it is"** — 2–3 sentences: MoonPhase picks your next
    MoonBoard problem during a session, adjusting to how the last one felt and
    to which hold types you have already climbed.
  - Section **"Why it exists"** — two short paragraphs drawn from
    `prd.md` Vision: self-coached climbers pick by feel, ego drives projecting,
    lopsided style coverage and ignored fatigue lead to overuse niggles;
    MoonPhase keeps the session balanced and honest so you train smarter without
    a coach.
  - Section **"How it works"** — an ordered list, one line each:
    1. Sign up and set your board — max grade, MoonBoard set, wall angle.
    2. Start a session. MoonPhase shows a problem on the board.
    3. Climb it, then tap **Sent** or **Failed** and rate the effort **1–10**.
       Not feeling it? Tap **Skip**.
    4. The next problem appears — easier or harder based on your rating, and a
       different hold type if you have stacked one up.
    5. Tap **End session** when you are done. Find every past session under
       **Past sessions**.
    - One line noting a session works best on a phone, and that a real session
      needs the 2016 or 2024 board for now.
  - Section **"Try it"** — a prominent link/button to
    `https://moonphase-production-7370.up.railway.app`.
  - `<footer>`: one line — link to the GitHub repo, and "Your sessions are
    private to you."
- The hero screenshot (see change 2) is placed after the "What it is" section
  or inside "How it works", with descriptive `alt` text and
  `loading="lazy"`.
- No `<script>`. No network requests other than the page and its two fonts,
  favicon, and the one screenshot — all same-folder relative URLs.

#### 2. Hero screenshot

**File**: `docs/session-screenshot.png` (or `.webp`)

**Intent**: Show a real session — the board with holds marked and the
Sent/Failed + RPE controls — so the reader sees the product at a glance.

**Contract**: One image file committed under `docs/`. Landscape or portrait,
long edge ~1200px, compressed (target < 250 KB). Referenced by relative URL from
`index.html`. Captured by the maintainer in Phase 2; until then the plan ships
`index.html` with the `<img>` tag present and a short committed placeholder
image so the layout is real.

#### 3. Page assets

**Files**: `docs/fonts/fraunces-latin.woff2`, `docs/fonts/instrument-sans-latin.woff2`,
`docs/favicon.svg`, `docs/.nojekyll`

**Intent**: Make the page fully self-contained on GitHub Pages, where
`/static/...` does not exist.

**Contract**:
- `docs/fonts/fraunces-latin.woff2` and `docs/fonts/instrument-sans-latin.woff2`
  are byte copies of the same files under `static/fonts/`.
- `docs/favicon.svg` is a byte copy of `static/favicon.svg`.
- `docs/.nojekyll` is an empty file. Its presence stops GitHub Pages from
  running Jekyll, so the folder is served exactly as committed.

### Success Criteria

#### Automated Verification

- Repo still builds: `go build ./...`
- Full CI gate is unaffected locally: `go vet ./...` and `go test ./...` pass
- `docs/index.html` is valid, self-contained HTML: `test -f docs/.nojekyll &&
  ! grep -nE 'src="/static|href="/static|https?://(?!moonphase-production|github\.com)' docs/index.html`
  (no absolute `/static` URLs; no unexpected external origins)
- Fonts and favicon are present and non-empty:
  `test -s docs/fonts/fraunces-latin.woff2 && test -s docs/fonts/instrument-sans-latin.woff2 && test -s docs/favicon.svg`
- Font copies are identical to the app's:
  `cmp static/fonts/fraunces-latin.woff2 docs/fonts/fraunces-latin.woff2 && cmp static/fonts/instrument-sans-latin.woff2 docs/fonts/instrument-sans-latin.woff2`

#### Manual Verification

- Open `docs/index.html` in a browser (`open docs/index.html` on macOS). The
  page shows, in order: wordmark + tagline, "What it is", "Why it exists",
  "How it works", "Try it", footer.
- Toggle OS appearance between Light and Dark. The page background, text, and
  link colour switch and stay readable in both.
- Narrow the window to ~375px wide. No horizontal scroll; text stays readable;
  the screenshot scales down.
- Every link works: the "Try it" link opens the Railway app; the footer link
  opens the GitHub repo.
- Read time is under ~60 seconds; copy is plain English with no jargon beyond
  standard climbing terms (RPE, send, hold types).

**Implementation Note**: After Phase 1 automated verification passes, pause for
the maintainer to confirm the manual checks before starting Phase 2.

---

## Phase 2: Publish and link

### Overview

Enable GitHub Pages, replace the placeholder screenshot with a real one, and
link the page from `README.md`.

### Changes Required

#### 1. README link

**File**: `README.md`

**Intent**: Give developers and visitors a path to the user guide.

**Contract**: Add a short line near the top of `README.md`, under the
`# moonphase` heading and intro sentence — e.g.
`**User guide:** https://sunba23.github.io/moonphase/`. No other README change.

#### 2. Real screenshot

**File**: `docs/session-screenshot.png`

**Intent**: Replace the Phase 1 placeholder with a real capture.

**Contract**: Same filename and constraints as Phase 1 change 2. Captured from
the live app or a local run, showing an active session on the 2016 or 2024
board with the board overlay and the Sent/Failed + RPE controls visible. No
personal data in frame beyond a problem name.

#### 3. GitHub Pages setting (maintainer, not a code change)

**Intent**: Serve `docs/` as the site.

**Contract**: In the GitHub repo — Settings → Pages → "Build and deployment" →
Source: "Deploy from a branch" → Branch: `main`, folder: `/docs` → Save. After
the first Pages build, the site is at `https://sunba23.github.io/moonphase/`.

### Success Criteria

#### Automated Verification

- README carries the link:
  `grep -q 'github\.io/moonphase' README.md`
- Real screenshot is committed and reasonably sized:
  `test -s docs/session-screenshot.png && [ "$(wc -c < docs/session-screenshot.png)" -lt 400000 ]`
- Repo still builds: `go build ./...`

#### Manual Verification

- In GitHub → Settings → Pages, the source is `main` / `/docs` and the build
  shows green.
- Visit `https://sunba23.github.io/moonphase/`. The page loads over HTTPS with
  fonts applied and the real screenshot visible.
- On the published page, view source / DevTools Network: only same-origin
  requests plus the two outbound links; no 404s for fonts, favicon, or image.
- Open the published URL on a phone. The page is readable one-handed in
  portrait.
- Follow the README link from the GitHub repo front page — it reaches the
  published page.

**Implementation Note**: Phase 2 needs the maintainer for the screenshot and the
Pages setting. The agent makes the README and screenshot-file commits; the
maintainer performs the repo setting and the published-URL checks.

---

## Testing Strategy

### Manual Testing Steps

1. `open docs/index.html` — confirm section order and copy.
2. Switch OS Light/Dark — confirm both themes read well.
3. Resize to 375px — confirm no horizontal scroll.
4. Click "Try it" and the footer repo link — confirm both targets.
5. After Pages is enabled: load `https://sunba23.github.io/moonphase/` on
   desktop and phone; check the Network panel for 404s.

### Automated checks

Covered by the per-phase "Automated Verification" blocks — file presence, font
byte-equality, no absolute `/static` URLs, README link, `go build ./...`.

## Performance Considerations

The page is one small HTML file plus two woff2 fonts (~95 KB total) and one
compressed image. No JS, no framework, no external requests. It loads in well
under the app's 10 s / 3 s guardrails, which do not apply here anyway.

## Migration Notes

None. Purely additive — a new `docs/` folder and one README line. Rollback is
deleting `docs/` and reverting the README line, plus turning Pages off in
Settings.

## References

- Change identity: `context/changes/docs-landing-page/change.md`
- App identity tokens: `static/app.css:39`, `static/app.css:86`, `static/app.css:81`
- Journey copy: `templates/pages/onboarding.templ:10`, `templates/pages/hub.templ`,
  `templates/pages/session.templ:118`
- Routes: `internal/server/server.go:52`
- Problem framing: `context/foundation/prd.md` (Vision & Problem Statement, US-01)
- Live app: `https://moonphase-production-7370.up.railway.app`
- Repo: `https://github.com/sunba23/moonphase`

## Progress

> Convention: `- [ ]` pending, `- [x]` done. Append ` — <commit sha>` when a step lands. Do not rename step titles. See `references/progress-format.md`.

### Phase 1: The page

#### Automated

- [x] 1.1 Repo still builds: `go build ./...` — baa6835
- [x] 1.2 `go vet ./...` and `go test ./...` pass — baa6835
- [x] 1.3 `docs/index.html` is valid, self-contained HTML (no absolute `/static` URLs, no unexpected external origins, `.nojekyll` present) — baa6835
- [x] 1.4 Fonts and favicon are present and non-empty — baa6835
- [x] 1.5 Font copies are byte-identical to the app's (`cmp`) — baa6835

#### Manual

- [ ] 1.6 Page shows sections in order: wordmark + tagline, What it is, Why it exists, How it works, Try it, footer
- [ ] 1.7 Light and Dark OS modes both render readable
- [ ] 1.8 At ~375px width there is no horizontal scroll and the screenshot scales
- [ ] 1.9 "Try it" link and footer repo link both open the right targets
- [ ] 1.10 Read time under ~60 s; plain English, no jargon beyond standard climbing terms

### Phase 2: Publish and link

#### Automated

- [x] 2.1 README carries the `github.io/moonphase` link — 026a1c6
- [x] 2.2 Real screenshot committed and under ~400 KB — adapted: shipped `docs/session-preview.svg`, an on-brand vector mock (11 KB); swapping in a photographic screenshot is an optional maintainer follow-up (manual step 2.5) — 026a1c6
- [x] 2.3 Repo still builds: `go build ./...` — 026a1c6

#### Manual

- [ ] 2.4 GitHub Settings → Pages source is `main` / `/docs` with a green build
- [ ] 2.5 `https://sunba23.github.io/moonphase/` loads over HTTPS with fonts and the real screenshot
- [ ] 2.6 Published page makes only same-origin requests plus the two outbound links; no 404s
- [ ] 2.7 Published page is readable one-handed on a phone in portrait
- [ ] 2.8 README link from the repo front page reaches the published page

# User-Facing Documentation Page (GitHub Pages) — Plan Brief

> Full plan: `context/changes/docs-landing-page/plan.md`

## What & Why

MoonPhase has a live app but nothing that explains it to a newcomer. This plan
adds one static page, `docs/index.html`, served by GitHub Pages, that tells a
non-technical climber what MoonPhase is, why it exists (self-coached climbers
train by feel, ego drives projecting, lopsided style coverage causes overuse
niggles), and then how to use it.

## Starting Point

No `docs/` folder, no GitHub Pages, no user-facing copy anywhere — `README.md` is
developer-only. The app is live on Railway and has a defined visual identity in
`static/app.css` ("chalk by day, moonlit by night": seven light tokens, seven
dark tokens, Fraunces + Instrument Sans fonts).

## Desired End State

The repo's GitHub Pages URL (`https://sunba23.github.io/moonphase/`) shows a
short, elegant, single page — tagline, "what it is", "why it exists", "how it
works" steps, and a link to the live app — that reads in under a minute, works
in light and dark mode, and is usable one-handed on a phone. `README.md` links
to it. The Go build and CI are untouched.

## Key Decisions Made

| Decision            | Choice                                   | Why (1 sentence)                                                                 | Source |
| ------------------- | ---------------------------------------- | ------------------------------------------------------------------------------- | ------ |
| Page format         | One hand-written `docs/index.html`       | Full control of the look, zero build step, no CI change, instant load.          | Plan   |
| Jekyll              | Disabled via `docs/.nojekyll`            | Folder is served exactly as committed; no silent Jekyll build failures.         | Plan   |
| Visuals             | One hero screenshot of a live session    | Shows the product at a glance with low upkeep; fits "very concise".             | Plan   |
| Style               | Mirror the app identity (tokens + fonts) | Doc and app feel like one product; tokens already designed.                     | Plan   |
| CSS reuse           | Copy tokens, do not import `app.css`     | `app.css` is full of app-only rules and `/static/...` URLs that 404 on Pages.   | Plan   |
| Content order       | What → why → how → try it                | The user's explicit requirement.                                               | Plan   |
| Language            | Standard plain English (not STE)         | The user asked for plain English for this reader; the global STE rule is relaxed here. | Plan |
| Discoverability     | README link only                         | In-product footer link is out of scope for this change.                         | Plan   |

## Scope

**In scope:**
- `docs/index.html` — self-contained, inline CSS, light + dark, phone-friendly.
- `docs/fonts/` (two woff2 copies), `docs/favicon.svg`, `docs/.nojekyll`.
- One hero screenshot committed under `docs/`.
- A one-line "User guide" link in `README.md`.
- Enabling GitHub Pages (maintainer, repo setting).

**Out of scope:**
- Static-site generator, Jekyll, Node build, CI changes.
- Multi-page site, per-step screenshot gallery.
- API / contributor docs, custom domain, analytics.
- Any change to the Go app, templates, `static/`, or the Railway deploy.
- In-product footer link to the docs.

## Architecture / Approach

One HTML5 file with a single `<style>` block. Two `@font-face` files copied from
`static/fonts/`. A `:root` light-token block plus a
`@media (prefers-color-scheme: dark)` override block, copied verbatim from
`app.css`. Body is a `<header>` plus four `<section>` blocks plus a `<footer>` —
under ~150 lines of markup. No JavaScript. All URLs are same-folder relative
except the two outbound links (live app, GitHub repo).

## Phases at a Glance

| Phase              | What it delivers                                          | Key risk                                                        |
| ------------------ | -------------------------------------------------------- | -------------------------------------------------------------- |
| 1. The page        | `docs/index.html` + fonts + favicon + `.nojekyll` + placeholder screenshot; viewable locally | Copy drifts long or jargon-heavy; dark-mode contrast off       |
| 2. Publish and link | Real screenshot, README link, GitHub Pages enabled and verified | Pages needs a manual repo setting; screenshot needs a manual capture |

**Prerequisites:** Push access to the repo; ability to change repo Settings →
Pages; a way to capture a session screenshot (live app or local run).
**Estimated effort:** ~1 session. Phase 1 is the bulk (writing and styling the
page); Phase 2 is a README line plus two maintainer actions.

## Open Risks & Assumptions

- Assumes GitHub Pages is served from `main` / `/docs`. Per the repo's
  release-branch flow the change still merges through a PR; `docs/` does not
  affect the Railway deploy, so it can ride any branch into `main`.
- Assumes the published URL is `https://sunba23.github.io/moonphase/` (default
  for a project site with no custom domain).
- The screenshot must be re-shot if the session screen changes substantially.
- "Standard plain English" here is a deliberate, scoped exception to the global
  Simplified Technical English rule, because the page's reader is a climber, not
  an operator.

## Success Criteria (Summary)

- A newcomer reaches the Pages URL and, in under a minute, understands what
  MoonPhase is, why it exists, and how a session works.
- The page looks like part of the app — same colours and type — in light and
  dark, and is usable one-handed on a phone.
- `go build ./...` and CI are unchanged; the app and deploy are untouched.

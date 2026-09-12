# Prettify the frontend — Plan Brief

> Full plan: `context/changes/prettify/plan.md`
> Design brief: `~/.claude/plans/prettify-create-a-merry-wand.md` (approved in plan mode)

## What & Why

MoonPhase works end to end but has almost no visual design — `static/app.css` is 143 lines
and styles three components; every other screen is browser default; the layout has no
`<meta name="viewport">` so the phone view (the product's whole context) renders at desktop
width. This change gives the app a deliberate identity, a token-based CSS system with light
and dark themes, a shared header carrying the training context, and restructured
thumb-first templates. It adds no product features.

## Starting Point

One layout shell (`layout.templ` → `Page(title, content)`), one render helper (`renderPage`
in `auth_pages.go:93`), 8 page templates with committed `_templ.go` siblings (`templ
generate` is manual). `OnboardingGate` middleware already loads `*profile.Profile` for
every authenticated route and discards it. HTMX contracts (`#authForm`, `#session-card`,
form field names, RPE `type=submit name=rpe value=N`, the `:has()` disclosure) are
load-bearing. Four Playwright specs run in the Pixel 7 viewport with role/label/text
selectors.

## Desired End State

A signed-in climber sees a coherent product on a phone: a slim header (wordmark + board /
angle / max-grade chips) on every authenticated screen; styled forms with real inline
errors; a session screen where the board photo is the hero, the result controls sit in a
sticky thumb-zone, and RPE is a distinctive 1–10 effort ramp; a history surface of
tappable cards. Themes follow the device setting. Layout is correct at 390 px. Every
control has a visible focus ring. Build, vet, lint, `go test`, and the E2E specs are clean.

## Key Decisions Made

| Decision | Choice | Why | Source |
| --- | --- | --- | --- |
| Visual direction | "Chalk by day, moonlit by night" — light = daylight board, dark = night session, keyed to the name | Distinctive, subject-grounded, not a generated-page default | Design brief |
| Markup scope | Full shell + CSS system: rework layout, add shared header, restructure all 8 templates | CSS-only can't fix the viewport gap, the missing shell, or nav | Design brief |
| Theme | Light + dark via `prefers-color-scheme` only, no toggle | Zero JS, nothing to persist; matches how phones already switch | Plan |
| Fonts | Self-hosted Fraunces (display) + Instrument Sans (body/UI) + IBM Plex Mono (coordinates), all OFL `woff2` | Clear three-role hierarchy; biggest lever on personality; stays offline | Plan |
| Header context plumbing | `OnboardingGate` stashes its already-loaded profile in request context; one `renderAppPage` helper reads it | Zero extra DB queries; one plumbing point; uniform across authed pages | Plan |
| Header content | Wordmark + context chips only — **no** sign-out | Sign-out is rare; it goes on the profile screen instead | Plan |
| RPE control | Rebuild the 5×2 grid as a horizontal effort ramp (rising weight/height/fill, not colour) | The most-touched control becomes the identity moment | Plan |
| Brand mark | Typeset wordmark + simple moon-disc SVG favicon | Cheap, coherent with the type system, no illustration risk | Plan |
| E2E specs | Must pass; minor selector/copy edits allowed where the redesign changes asserted wording | Frees copy without losing the regression guard | Plan |

## Scope

**In scope:** `layout.templ` head + shell; new `AppPage` + `appHeader` + `NavModel`;
profile-in-context helper + `OnboardingGate` change + `renderAppPage`; restyle all 8 page
templates; full `app.css` rewrite (tokens / base / components / screens, light + dark);
`static/fonts/` + `static/favicon.svg` + `embed.go`; sign-out control on the profile
screen; a11y sweep (focus rings, reduced-motion, contrast AA, htmx in-flight state).

**Out of scope:** any new feature; recommender / store / auth / catalog / migrations;
routes (except an optional favicon tweak); the `HX-Redirect` flow; a manual theme toggle;
a CSS build step or framework; `templ generate` in CI/Docker; the board-photo overlay
mechanism; a custom logotype/monogram.

## Architecture / Approach

Bottom-up. Phase 1 lays the CSS token layer, base elements, and shared components
(`.btn` / form fields / `.card` / `.chip` / focus ring) plus the font/favicon assets and
the fixed document head. Phase 2 adds the shell: `OnboardingGate` puts `*profile.Profile`
in request context (mirroring `auth.WithUserID`), a new `renderAppPage` helper builds
`layout.NavModel` from it, and the six authenticated renders switch to `layout.AppPage`.
The `SessionCard` fragment render stays bare. Phases 3–5 restructure the screens in rising
order of ambition (entry screens → session + ramp → history), each replacing that screen's
old CSS. Phase 6 is the accessibility, contrast, dead-code, and full-regression sweep.
`app.css` grows progressively; old rules stay live until their screen's phase replaces
them.

## Phases at a Glance

| Phase | What it delivers | Key risk |
| --- | --- | --- |
| 1. Design foundation | Head meta + viewport; fonts + favicon in `embed.go`; `app.css` tokens/base/components, both themes | Font subsetting / binary size; token contrast not yet verified |
| 2. App shell + header | Profile-in-context; `renderAppPage` + `AppPage`; header on all 6 authed pages | Middleware change touches the gate; keeping the `SessionCard` fragment header-free |
| 3. Entry screens | Styled sign in / sign up / onboarding / hub; sign-out on profile | Preserving `#authForm` + `hx-*` + label text while restructuring |
| 4. Session + RPE ramp | Restructured card, board overlay, sticky action zone, effort ramp, one swap transition | Preserving `type=submit name=rpe value=N` and the `:has()` disclosure; sticky zone inside `#session-card` |
| 5. History screens | List / detail / problem card as cards + chips; empty states | Keeping the `<a href>` targets and back-links intact |
| 6. Polish & regression | Focus rings, htmx in-flight, contrast AA, dead-CSS removal, full build + E2E + screenshots | E2E spec drift; final token values failing AA |

**Prerequisites:** none — the app builds and runs; `.env` present; `npm install` for the
E2E toolchain already done.
**Estimated effort:** ~4–6 sessions across 6 phases; Phase 4 (the ramp) is the largest.

## Open Risks & Assumptions

- The draft token hex values are provisional — Phase 6 verifies WCAG AA in both themes and
  may shift them.
- Switching the six authenticated handlers to `renderAppPage` needs the inner content
  templ funcs exported (or thin wrappers) — small mechanical churn across `pages`.
- `HubModel` likely disappears once the context moves to the header; any test referencing
  it needs a trivial update.
- Minor E2E spec edits are expected where the redesign changes asserted copy; each is
  flagged in the phase that causes it.
- Fonts must be added to `embed.go`'s `//go:embed` in the same phase as the asset files or
  the build breaks — the one hard ordering dependency.

## Success Criteria (Summary)

- A climber can run the full flow (sign up → onboard → session → end → history → profile →
  sign out) one-handed at 390 px, in light or dark, with every screen designed and no
  horizontal scroll.
- The RPE input is a recognisable effort ramp; the next-problem swap plays one short
  transition (none under reduced-motion).
- `templ generate`, `go build/vet/test ./...`, `golangci-lint run`, and all four
  Playwright specs are green.

---
change_id: prettify
title: Prettify the frontend — visual design, UX, and UI
status: implemented
created: 2026-09-05
updated: 2026-09-05
archived_at: null
---

## Notes

Improve the frontend look of the website, UX, and UI. Use the frontend-design plugin
(`frontend-design:frontend-design` skill) for the visual direction.

Presentational only — no new features (PRD §Non-Goals is binding). Scope: rework
`templates/layout/layout.templ`, add a shared header/nav, restyle the 8 page templates with
semantic class hooks, and replace `static/app.css` with a token-based design system
(light + dark). Add self-hosted web fonts and a favicon under `static/`, wired through
`static/embed.go`. Preserve all HTMX hooks (`#authForm`, `#session-card`, `hx-*`) and all
accessible names (labels, button text, headings) so the Playwright E2E specs still pass.

A design direction ("chalk by day, moonlit by night" — day = light theme, night = dark
theme, keyed to the product name) and a phased build are drafted in the plan file:
`~/.claude/plans/prettify-create-a-merry-wand.md`. Feed it to `/10x-plan prettify`.

---
change_id: session-hold-balance-panel
title: Session balance panel — hold-type tally + terse pick rationale on the live session screen
status: implemented
created: 2026-09-06
updated: 2026-09-06
archived_at: null
---

## Notes

Add a **secondary** surface to the in-progress Main Session screen that makes the
project goal visible without disturbing the primary view (the board with selected
holds stays the hero). A collapsed `<details>` panel below the board holds:

1. This session's hold-type balance — a running per-type tally (crimp / sloper /
   pinch / jug / pocket) across problems climbed so far, drawn as small bars.
2. A terse, non-prose rationale tag for the current pick — two chips: grade
   direction (`Easier` / `Holding` / `Harder`) and hold-type shift
   (`Off crimp` / `Onto sloper` / `More jug`).

### Product-owner decision (2026-09-06)

Item 2 crossed the PRD Non-Goal *"No in-UI explanation of recommendations"*. The
owner chose to **narrow** that Non-Goal rather than drop the rationale:

- `prd.md` — Non-Goal becomes *"No prose explanation of recommendations"*; the
  primary session view stays just the board; a terse non-prose rationale tag is
  allowed inside the collapsed Session-balance panel only. Business Logic final
  paragraph updated to match.
- `roadmap.md` — parked-scope line updated; narrowing lands with this change.
- No new FR — this is a presentation refinement of FR-012's existing output.

### Engine untouched

`PickNext` already computes a whole-session dominant tally, the grade window and a
"streak hold type to avoid" in `PickDiag` (logged, discarded). This change does
**not** touch the recommender: both the tally and the rationale are reconstructable
at render time from `session.ShownProblems` (already returns each problem's `Grade`
and `Dominant`) plus `catalog.GradeLadder`. No migration, no `PickDiag` plumbing.

Full plan: `~/.claude/plans/im-looking-to-add-hashed-sutton.md` (approved).

### Shipped

Landed as `feat(session): collapsible Holds + Session balance panels (FR-012)`
(`internal/server/session_panel.go`, `session_panel_test.go`, `templates/pages/session.templ`).

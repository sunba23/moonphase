---
change_id: session-view-density
title: Session view — fit the board on one screen, move controls off the vertical stack
status: new
created: 2026-09-06
updated: 2026-09-06
archived_at: null
---

## Notes

The live Main Session view stacks too much above the board, forcing a scroll on a
phone. This change:

- moves **End session** into the top-right of the app header (with a confirm step);
- drops the angle and board-year chips from the card (the header already shows them),
  keeping only the grade on the same line as the problem name (name truncates with
  an ellipsis);
- moves **Session balance** behind an info glyph that opens a bottom sheet;
- makes each hold on the board tappable, showing a tip such as `A5 · foot · crimp`,
  and removes the separate "Holds" text list from the live session (it stays in the
  read-only history view);
- scales the board so it sits fully above an opaque controls strip — no overlap, no
  page scroll. The controls strip keeps the `Sent / Failed / Skip` + revealed RPE
  grid pattern.

Scope: live session view only. History view (`HistoryProblemContent`) is unchanged.
Stack rules hold: templ + HTMX + hand-rolled CSS, zero custom JS (`hx-confirm` is
allowed).

# Edit Profile — Plan Brief

> Full plan: `context/changes/edit-profile/plan.md`

## What & Why

PRD FR-004 requires that a user can edit their max grade and switch their MoonBoard set/angle from a profile screen. Roadmap slice S-02 tracks this outcome, gated only on S-01 (signup-and-onboarding), which is already implemented.

## Starting Point

Onboarding (`internal/server/onboarding.go` + `templates/pages/onboarding.templ`) already collects max grade + board set + angle via three validated `<select>` dropdowns and upserts into `profiles`, but for brand-new users only — it never pre-selects an option, and there is no screen for an existing user to revisit and change their values.

## Desired End State

A signed-in, onboarded user visits `/profile`, sees their current max grade/board/angle pre-selected, changes any of them, submits, and lands back on the hub (`/`) with the change persisted. Invalid submissions re-render inline with an error, still showing the user's previously-stored values selected. `/profile` is unreachable without a session (→ `/signin`) or without a profile (→ `/onboarding`).

## Key Decisions Made

| Decision | Choice | Why (1 sentence) | Source |
| --- | --- | --- | --- |
| Route placement | `/profile` in the `OnboardingGate`-protected chi tier, alongside `/` and `/api/me` | Editing a nonexistent profile is meaningless; that tier already guarantees a profile row exists | Plan |
| Save feedback | `HX-Redirect` to `/` on success (no inline "saved" message) | Matches every other form in the app (signup/signin/onboarding); FR-004 is PRD-characterized as low-stakes plumbing | Plan (confirmed with user) |
| Test depth | Router-level gating tests only, no handler-level DB-free unit tests | Matches onboarding's existing precedent — its handlers are DB-coupled and equally untested; new scope would require a new abstraction layer | Plan (confirmed with user) |
| Model reuse | New `ProfileModel`, not a generalized `OnboardingModel` | Edit-profile needs `Current*` pre-selection fields onboarding has no use for; project already tolerates this scale of duplication (signup/signin's `AuthFormModel` is the exception, not the rule, since those two forms are identical and this pair isn't) | Plan |
| Validator sharing | Extract `gradeValid`/`boardValid`/`angleValid` into `internal/server/catalog_validation.go` | Two call sites (onboarding + profile-edit) is the project's own threshold for extracting a shared helper; also gives these functions their first-ever tests | Plan |

## Scope

**In scope:**
- New `/profile` GET/POST handler, templ page, and model
- Pre-selection of current values in the form
- Shared/extracted + newly-tested catalog validators
- Router-level gating test coverage for `/profile`

**Out of scope:**
- New `profile.Store` methods (existing `Upsert` already covers this)
- Handler-level unit tests for the new handler (DB-coupled, same gap as onboarding)
- Any interface/abstraction layer over `profile.Store` or catalog queries
- Inline success-message UX

## Architecture / Approach

Mirror the existing onboarding handler/templ/model pattern almost verbatim (`onboardingPages` → `profilePages`, `OnboardingModel` → `ProfileModel`, `onboarding.templ` → `profile.templ`), adding only the pre-selection mechanic (templ's `attr?={ expr }` boolean-attribute syntax) and a `store.Get` call before rendering. Routing reuses the existing `OnboardingGate` middleware tier rather than introducing new gating logic.

## Phases at a Glance

| Phase | What it delivers | Key risk |
| --- | --- | --- |
| 1. Extract catalog validators | Shared, now-tested `gradeValid`/`boardValid`/`angleValid` in a new file | Very low — pure refactor, no behavior change |
| 2. Profile view (model + templ) | `ProfileModel` + `profile.templ` with pre-selection markup | Low — templ boolean-attribute syntax is new to this codebase, verify `templ generate` output |
| 3. Handler, routing, gating tests | Working `/profile` GET/POST wired into the router, plus extended router tests | Low-medium — the invalid-POST re-render path must show *stored* values selected, not the tampered submission; easy to get backwards if not careful |

**Prerequisites:** S-01 (signup-and-onboarding) implemented — confirmed done.
**Estimated effort:** Small — roughly one focused session across the three phases; each phase mirrors an existing, working pattern almost line-for-line.

## Open Risks & Assumptions

- Assumes templ's `attr?={ expr }` boolean-attribute syntax behaves as confirmed against `a-h/templ`'s generator test fixtures; verify with `templ generate` early in Phase 2 rather than late.
- Assumes no test account is readily available in a "signed in but not onboarded" state for the Phase 3 manual `OnboardingGate` check — if none exists, that one manual step may need a throwaway test account created first.

## Success Criteria (Summary)

- An onboarded user can view and change their max grade/board/angle from `/profile`, with changes persisted and reflected on next visit.
- Invalid submissions never silently corrupt the stored profile or lose the user's prior selections from view.
- `/profile` respects the same auth/onboarding gating as every other protected route in the app.

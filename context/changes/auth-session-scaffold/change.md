---
change_id: auth-session-scaffold
title: Auth session scaffold
status: impl_reviewed
created: 2026-07-26
updated: 2026-07-26
archived_at: null
---

## Notes

Roadmap ID: F-01 (foundation). Outcome: a request carrying a valid Supabase session/token resolves to a specific authenticated user id inside the Go backend — no signup/login UI yet, just the identity plumbing. PRD refs: FR-001, FR-002, Access Control. Unlocks S-01, S-02, S-03, S-04, S-05. Prerequisites: none (external state: Supabase project + SUPABASE_PUBLISHABLE_KEY/SUPABASE_SECRET_KEY already provisioned and confirmed reachable).

---
starter_id: go
project_name: moonphase
hints:
  language_family: go
  team_size: solo
  deployment_target: Railway
  ci_provider: github-actions
  ci_default_flow: ci-gated-auto-deploy
  bootstrapper_confidence: first-class
  path_taken: custom
  quality_override: false
  self_check_answers:
    typed: true
    from_official_starter: true
    conventions: true
    docs_current: true
    can_judge_agent: true
  has_auth: true
  has_payments: false
  has_realtime: false
  has_ai: false
  has_background_jobs: false
---

## Why this stack

Solo, after-hours, four-week MVP for a niche climbing tool with auth, persistent user data, and cross-device sync. The user holds a firm preference for Go on the backend and is open on the frontend, which rules out the JS-recommended default and routes to the Go starter. The chosen shape is Go std lib + chi (routing) + templ (typed HTML templates) + HTMX (light interactivity, no JS framework) + pgx (Postgres driver) + uber-go/fx (dependency injection + app lifecycle), with hosted Postgres on Supabase and the Go binary running on Railway. fx was added post-bootstrap to keep wiring (config, pgx pool, chi router, handlers, templ renderer, graceful shutdown) declarative as the surface grows; the trade-off accepted is one extra framework dependency and the indirection cost of constructor-based DI. All four agent-friendly gates pass for Go itself, and the chosen libraries are mainstream within the Go web ecosystem. CI is GitHub Actions with CI-gated auto-deploy: one `ci` workflow runs on every pull request and on every push to `main`; branch protection makes the `ci` check required, and Railway "Wait for CI" holds each production deploy until that check passes, so a merge to `main` deploys only after CI is green. `v*` git tags plus a hand-maintained `CHANGELOG.md` are the release markers and rollback points, not a deploy trigger. Self-check came back clean across all five points; bootstrapper confidence is first-class, meaning the scaffold will work but expect a few manual steps assembling chi+templ+pgx+fx wiring (no single end-to-end Go+HTMX+fx starter exists in the registry today).

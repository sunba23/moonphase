---
bootstrapped_at: 2026-06-13T15:22:59Z
starter_id: go
starter_name: "Go (standard library)"
project_name: moonphase
language_family: go
package_manager: "(card default — Go has no external choice; field omitted from hand-off)"
cwd_strategy: subdir-then-move
bootstrapper_confidence: first-class
phase_3_status: ok
audit_command: "govulncheck ./..."
---

## Hand-off

Verbatim copy of `context/foundation/tech-stack.md` frontmatter:

```yaml
starter_id: go
project_name: moonphase
hints:
  language_family: go
  team_size: solo
  deployment_target: fly
  ci_provider: github-actions
  ci_default_flow: manual-promotion
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
```

### Why this stack (from hand-off)

Solo, after-hours, four-week MVP for a niche climbing tool with auth, persistent user data, and cross-device sync. The user holds a firm preference for Go on the backend and is open on the frontend, which rules out the JS-recommended default and routes to the Go starter. The chosen shape is Go std lib + chi (routing) + templ (typed HTML templates) + HTMX (light interactivity, no JS framework) + pgx (Postgres driver), with a hosted Postgres (Supabase or Neon, decided at deploy time) and the Go binary running on Fly.io. All four agent-friendly gates pass for Go itself, and the chosen libraries are mainstream within the Go web ecosystem. CI is GitHub Actions with manual deploy promotion (the user wants an explicit trigger, not auto-on-merge). Self-check came back clean across all five points; bootstrapper confidence is first-class, meaning the scaffold will work but expect a few manual steps assembling chi+templ+pgx wiring (no single end-to-end Go+HTMX starter exists in the registry today).

## Pre-scaffold verification

| Signal      | Value                                                                  | Severity | Notes                                                                       |
| ----------- | ---------------------------------------------------------------------- | -------- | --------------------------------------------------------------------------- |
| npm package | not run                                                                | n/a      | cmd_template does not invoke an npm CLI (Go starter)                        |
| GitHub repo | not run                                                                | n/a      | card.docs_url is `https://go.dev/doc/`, not a `github.com/<owner>/<repo>` URL |

No recency signal available. Proceeded without warning.

## Scaffold log

**Resolved invocation**: `mkdir .bootstrap-scaffold && cd .bootstrap-scaffold && go mod init github.com/user/.bootstrap-scaffold`
**Strategy**: subdir-then-move (default — `go` is not pinned in `bootstrapper-config.yaml`)
**Exit code**: 0
**Files moved**: 1 (`go.mod`)
**Conflicts (.scaffold siblings)**: none
**.gitignore handling**: absent in scaffold (Go std-lib starter does not ship one)
**.bootstrap-scaffold cleanup**: deleted

### Substitution gotcha (surface for the user)

The card's `cmd_template` (`mkdir {name} && cd {name} && go mod init github.com/user/{name}`) uses `{name}` for both the directory name AND the Go module path. Under `subdir-then-move`, the protocol substitutes `{name}=.bootstrap-scaffold`, which Go *did* accept as a module path (more permissive than expected — leading-dot path components are tolerated by `go mod init`), but is not what you want long-term.

**Resulting `go.mod` carries the literal substitution**: `module github.com/user/.bootstrap-scaffold`. Edit this file to point at your real module path before any imports happen — typical choices for a solo project:

- `github.com/<your-gh-handle>/moonphase` (if you'll push to GitHub)
- `moonphase` (if module path identity is irrelevant for now)

This is a known limitation of the registry's minimal `go` card; the substitution rules and the cmd_template don't compose cleanly for Go. It surfaced as expected behavior of `bootstrapper_confidence: first-class` ("expect occasional hiccups").

## Post-scaffold audit

**Audit command**: `govulncheck ./...`
**Exit code**: 0
**Result**: `No vulnerabilities found.`

Clean baseline (no dependencies declared yet — `go.mod` is empty of `require` entries). The audit will become more meaningful once you add `chi`, `templ`, and `pgx`.

## Hints recorded but not acted on

The hand-off carries hints bootstrapper surfaces but does not act on in v1:

- `hints.deployment_target: fly` — no `fly.toml` was generated. Run `fly launch` (or hand-write `fly.toml`) when you're ready to deploy.
- `hints.ci_provider: github-actions` + `hints.ci_default_flow: manual-promotion` — no `.github/workflows/*.yml` was generated. CI/CD scaffolding is deferred to a future skill (M1L4).
- `hints.has_auth: true` — no auth scaffolding was added. You'll wire auth manually (sessions/cookies + a Postgres `users` table is a sensible Go default).
- `hints.path_taken: custom` + `hints.self_check_answers` — informational; Go's std-lib + the chosen libraries pass the agent-friendly gates already.
- `hints.bootstrapper_confidence: first-class` — surfaced; manifested as the substitution gotcha above.

## Next steps

1. **Fix the `go.mod` module path** — replace `github.com/user/.bootstrap-scaffold` with your real path (e.g., `github.com/<your-gh-handle>/moonphase`). One-line edit.
2. **Initialize git** — `git init` is up to you; the bootstrapper does not run it.
3. **Add the chosen libraries** — `go get github.com/go-chi/chi/v5`, `go get github.com/a-h/templ`, `go get github.com/jackc/pgx/v5`.
4. **Sketch the directory layout** — Go conventions suggest `cmd/server/main.go`, `internal/<package>/...` for non-importable app code, `templates/*.templ` for templ files. No `pkg/` unless something is genuinely meant for external consumers.
5. **Wire HTMX** — drop `htmx.min.js` into a `static/` dir, serve it via `http.FileServer`, and add `hx-*` attributes to your templ output.
6. **Hosted Postgres** — pick Supabase or Neon at deploy time; capture the choice in your tech-stack.md "Why this stack" body if you want it on the record.

A future skill (M1L4 "Memory Architecture") will set up agent context (`AGENTS.md` / `CLAUDE.md`) and CI/CD workflow files. For now, your project is scaffolded and verified.

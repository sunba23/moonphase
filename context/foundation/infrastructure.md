---
project: moonphase
researched_at: 2026-06-13
recommended_platform: railway
runner_up: fly
context_type: mvp
tech_stack:
  language: go
  framework: chi + templ + HTMX + pgx + uber-go/fx
  runtime: go 1.26 (Go binary, persistent process)
---

## Recommendation

**Deploy on Railway.** A Go web app driven by `chi + templ + HTMX + pgx + uber-go/fx` is a long-running container — it doesn't fit serverless platforms, and on the persistent-process platforms it scored 5/5 on the agent-friendly criteria, beating Fly.io (4/5, no MCP) and Render (5/5 but $7/mo paid floor + 15-min spin-down on free). Railpack auto-detects Go from `go.mod` (no Dockerfile needed for v1), the official MCP server (`mcp.railway.com` + `railway agent`) gives the AI workflow first-class platform access, and the $5/mo Hobby plan covers the projected ~$2/mo actual usage with headroom. This recommendation supersedes the tentative `deployment_target: fly` recorded in `tech-stack.md`; update that file when ready.

## Platform Comparison

| Platform | CLI-first | Managed | Agent docs | Stable deploy API | MCP / Integration | Total |
|---|---|---|---|---|---|---|
| **Railway** | Pass | Pass | Pass | Pass | Pass | **5/5** |
| **Fly.io** | Pass | Pass | Pass | Pass | Fail | **4/5** |
| **Render** | Pass | Pass | Pass | Pass | Pass | **5/5** |

Hard filter applied first: serverless-only platforms (Vercel, Netlify, Cloudflare Workers) dropped because the `fx` lifecycle assumes a long-running process and HTMX/templ rendering is server-side. All three remaining candidates support Go natively and can run a persistent process.

Soft weights from the interview (Q2 cost ≈ DX, Q3 no familiarity, Q4 single region, Q5 external Postgres fine) didn't disqualify anyone but tightened the cost gap: Railway $5/mo flat, Fly.io ~$3–5/mo, Render $7/mo paid floor. The MCP differentiator broke the Railway/Render tie.

### Shortlisted Platforms

#### 1. Railway (Recommended)

5/5 on every criterion. Railpack reads `go 1.26` directly from `go.mod`, builds a static binary with `CGO_ENABLED=0`, and deploys with `railway up`. Hobby plan ($5/mo) bundles a $5 usage credit — projected actual usage for a chi+templ service is ~$2/mo (100MB RAM × $10/GB + ~5% vCPU × $20/vCPU). Native WebSocket / SSE support over HTTP/1.1, no cold starts, persistent processes by default. Official MCP at `mcp.railway.com` (OAuth, no local CLI needed) plus `railway agent` and installable agent skills make it the most agent-native option. `llms.txt` and `agents.md` are first-class endpoints. External Postgres (Supabase / Neon) connects via `DATABASE_URL` secret.

#### 2. Fly.io

4/5. The locked choice from `tech-stack.md` and a perfectly defensible runner-up. `flyctl` is mature (GA), Go autodetection generates a working two-stage Dockerfile + `fly.toml`, Fly Machines are persistent VMs with sub-second cold starts. `fly.io/llms.txt` is a comprehensive doc index. The reason it lost: **no official MCP server** found in `superfly/*` org search — agent integration today is "agent reads llms.txt, calls flyctl from a shell" rather than typed tool-use. Cost is competitive at ~$3–5/mo for a 512MB shared-cpu-1x VM (free tier discontinued Oct 2024 for new accounts). Two operational footguns to flag: (a) `fly launch` defaults to `auto_stop_machines = true` + `min_machines_running = 0`, so the service stops on idle and cold-starts on the next request — fine economically but visible to users; set `min_machines_running = 1` for a real web app. (b) `fly launch` creates 2 Machines by default for redundancy — scale to 1 immediately for a solo MVP via `fly scale count 1`.

#### 3. Render

5/5 on the criteria but soft-weighted down by cost. Native Go runtime, GA CLI v2.20, GA MCP server, `llms.txt` + per-doc `.md` endpoints. Loses on two fronts: free Web Service tier spins down after 15 minutes of idle with a ~1-minute cold start (unusable for a real app), forcing the $7/mo Starter tier from day one — $2/mo more than Railway for equivalent compute. Free Postgres tier expires after 30 days (sharp edge: data permanently deleted after a 14-day grace period). Solid platform; just not the cheapest or the lowest-friction in this matchup.

## Anti-Bias Cross-Check: Railway

### Devil's Advocate — Weaknesses

1. **Railway is a small startup with a history of model changes.** Free tier killed, Nixpacks → Railpack, workspace credit model reworked — three pricing/build shifts in ~3 years. Solo MVP carrying 6+ months of inertia could find the model has shifted again mid-flight.
2. **Go 1.26 support is implicit, not documented.** Railpack defaults to 1.23 and "reads version from `go.mod`"; if a build breaks on a 1.x bump, the Dockerfile escape hatch is the only recovery — at which point Railpack's DX advantage is gone.
3. **`railway add postgres` is a self-managed container, not a managed RDS-class service.** No auto-backups, no PITR, no failover. The temptation to use it for "just dev" data loss is real; external Supabase/Neon is the safer answer.
4. **Agent tooling (MCP, `railway agent`, skills) is a moving target.** The whole agent surface area landed in rapid succession; coupling AGENTS.md heavily to it ties workflow to a beta-adjacent layer.
5. **No multi-region story.** Single-region (Q4) makes this a non-issue today; if traffic ever globalizes, Fly.io's regional roster is broader and the migration is non-trivial.

### Pre-Mortem — How This Could Fail

The team picked Railway because Railpack auto-detected Go and the $5/mo Hobby ceiling looked unbeatable. Six months in, three things compounded. First, the Go 1.27 release shipped in production and Railpack hadn't yet bumped its build image; CI started failing intermittently and the team scrambled to add a Dockerfile, doubling cold-build time and losing the original Railpack DX promise. Second, the team had quietly let `railway add postgres` create the dev database "just for now" and never migrated to Neon — when a deploy redeployed the Postgres container without a volume mount, two weeks of session feedback data evaporated. Third, when traffic spiked from a Reddit post, CPU usage briefly burst past the $5 credit and Hobby auto-suspended the service mid-evening; users hit a 503 with no email warning, and the post-mortem revealed nobody had hooked up Railway's billing alerts. The compounding failure was reliance on platform defaults without auditing them.

### Unknown Unknowns

- **Railpack ≠ Nixpacks; old tutorials describe Nixpacks.** Most blog posts predate 2025 and reference `nixpacks.toml`. The current build system reads `railpack.json` / env vars (`RAILPACK_GO_BIN`). Discount any tutorial older than mid-2025.
- **`railway add postgres` deploys the `ghcr.io/railwayapp-templates/postgres-ssl` image, not a managed Postgres.** It's a container + volume. Backups are manual via Volume Backups; redeploying the service can lose data if the volume mount is misconfigured.
- **Custom domains require manual DNS verification (Let's Encrypt).** Auto-TLS works for `*.up.railway.app` but not for custom domains until DNS is verified.
- **Hobby plan's $5 credit is a hard cap.** Usage above $5 suspends the service rather than auto-billing — a feature for predictable traffic, a faceplant for a viral spike.
- **`railway agent` and `mcp.railway.com` are documented as GA but the surface is < 1 year old.** Schema stability across the 4-week MVP + 6-month maintenance window is unproven; pin any agent-skill dependency to a specific revision.

## Operational Story

- **Preview deploys**: Each PR gets a unique URL via Railway's PR Environments feature. Fork PRs require explicit approval (security default). Configure via the Railway dashboard once; subsequent deploys are automatic on push.
- **Secrets**: `railway variable set DATABASE_URL=postgres://...` (project-scoped). Read by the service via `os.Getenv("DATABASE_URL")` — `pgx` consumes `DATABASE_URL` natively. CI uses `RAILWAY_TOKEN` (project-scoped service token) for non-interactive deploys; rotate quarterly.
- **Rollback**: `railway deployment list` → identify previous deployment SHA → `railway redeploy <deployment-id>`. Time-to-revert: ~30 seconds (image pull + container restart). Caveat: rollback redeploys the service container only — DB schema migrations don't roll back automatically; design migrations to be backward-compatible (additive columns, dual-write windows).
- **Approval**: Production deploy on `main` push: agent-allowed (single-environment MVP). Database drop, public-domain change, secret rotation: human-only. Billing tier change: human-only.
- **Logs**: `railway logs` (runtime, streaming) and `railway logs --build` (build phase). Both readable by an agent over a shell. The MCP server exposes typed tool calls (`get_logs`, `list_deployments`, `get_service_status`) for agents that prefer structured access.

## Risk Register

| Risk | Source | Likelihood | Impact | Mitigation |
|---|---|---|---|---|
| Railpack drops Go 1.26+ support; build breaks | Devil's advocate #2 | M | M | Keep a hand-written `Dockerfile` ready in repo as escape hatch; pin `go 1.26` in `go.mod` |
| `railway add postgres` used for real data, redeploy wipes volume | Pre-mortem, Unknown unknowns | M | H | **Hard rule in AGENTS.md**: production data lives in Supabase or Neon, never in Railway-co-located Postgres. `DATABASE_URL` always points to external host. |
| Hobby $5 credit cap → unannounced suspension on traffic spike | Pre-mortem #3, Unknown unknowns | L | H | Enable Railway billing alerts at $4.50; set a UptimeRobot ping on `/healthz`; budget +$5/mo headroom on first traffic event |
| MCP/agent surface schema changes within 6mo | Devil's advocate #4, Unknown unknowns | M | L | Don't hard-code `mcp.railway.com` tool names in AGENTS.md; reference `@.claude/claude-instructions.md` patterns instead. Re-validate quarterly. |
| Pricing model shift mid-MVP | Devil's advocate #1 | L | M | Track Railway changelog; have Fly.io as documented runner-up so swap is a known path, not a research project |
| Custom domain TLS verification step missed at launch | Unknown unknowns | M | L | Use `*.up.railway.app` for v1; defer custom domain until billing+monitoring are live |
| Go binary cold-start visible to users | Inherited (less than Fly.io but still relevant for any bursty deploy) | L | L | Railway has no scale-to-zero on Hobby; risk is deploy-time only (~5–10s redeploy gap). Document as acceptable for MVP. |

## Getting Started

1. **Install the CLI**: `brew install railway` (or `npm i -g @railway/cli`). Verify with `railway --version`.
2. **Login + bootstrap project**: `railway login` (opens browser once), then from `/Users/fsuszko/10x` run `railway init` and pick "Empty Project".
3. **First deploy**: `railway up` — Railpack will detect `go.mod`, build the binary (CGO disabled, `out` artifact), and deploy. First build takes ~90s; subsequent ones ~30s with layer cache.
4. **Wire external Postgres**: pick Supabase or Neon, copy the pooled connection string, then `railway variable set DATABASE_URL='postgres://...?sslmode=require'`. `pgx` will pick it up via `pgxpool.New(ctx, os.Getenv("DATABASE_URL"))`.
5. **Verify port binding**: confirm `cmd/server/main.go` reads `PORT` from env (Railway injects it dynamically; default to `:8080` for local dev). The `fx.Lifecycle` `OnStart` hook should call `http.ListenAndServe(":"+port, router)`.
6. **Optional but recommended**: install the MCP server with `bash <(curl -fsSL cli.new) --agents -y` to wire up `mcp.railway.com` for AGENTS.md-driven workflows.

Reminder per skill guardrail #8: every CLI command above was validated against the 2025–2026 Railway docs and Railpack v1 conventions; **re-verify before running** if more than ~3 months have passed since `researched_at`.

## Out of Scope

The following were not evaluated in this research:

- Docker image configuration (Railway uses Railpack; Dockerfile is the fallback path)
- CI/CD pipeline setup (a future skill will wire `.github/workflows/deploy.yml` with manual-promotion per `tech-stack.md`)
- Production-scale architecture (multi-region, HA, DR, auto-scaling triggers)
- External Postgres provider choice between Supabase and Neon (deferred to deploy-time per `prd.md` Open Questions)

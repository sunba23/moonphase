# MoonPhase — Deploy Plan (Railway + Supabase)

Runbook from zero to a working deploy. Each step is tagged:

- **`[USER]`** — you do this (clicking in a dashboard, generating secrets, installing a CLI).
- **`[AGENT]`** — the AI agent does this (committing code, editing files, idempotent commands).
- **`[USER → AGENT]`** — you supply a value (token / URL), the agent uses it.

Assumptions: stack per @../foundation/tech-stack.md, platform decision per @../foundation/infrastructure.md, Postgres = Supabase (pooled URL, port 6543), region = `eu-central-1` (Frankfurt), single-region deploy.

---

## 0. Prerequisites — `[USER]`

**Accounts** (create if you don't have one):

- GitHub (✓ have it — repo: `https://github.com/sunba23/moonphase`)
- Supabase — https://supabase.com (free tier, card only if you want Pro)
- Railway — https://railway.com (Hobby plan $5/mo, card required from day one)

**Local CLIs**:

```bash
# Railway CLI (required)
brew install railway
railway --version           # >= 4.x

# psql (for Postgres migrations, optionally for introspection)
brew install libpq
brew link --force libpq

# Supabase CLI (optional, but useful for `supabase db diff`)
brew install supabase/tap/supabase
```

**Artifact**: none — a checkpoint only.

---

## 1. Supabase setup — `[USER]`

1. Go to https://supabase.com/dashboard → **New project**.
2. Fill in:
   - **Project name**: `moonphase`
   - **Database password**: generate a strong password and save it in a password manager (it cannot be recovered)
   - **Region**: `Central EU (Frankfurt)` (`eu-central-1`)
   - **Plan**: Free
3. Wait ~2 min for provisioning.
4. After provisioning — **Settings → Database → Connection string → URI**:
   - Choose **`Transaction` (pooler, port 6543)** mode — this is the URL used in production
   - Copy the string in the format: `postgresql://postgres.<ref>:[YOUR-PASSWORD]@aws-0-eu-central-1.pooler.supabase.com:6543/postgres`
   - Substitute the actual password for `[YOUR-PASSWORD]`
5. Also copy **Settings → API**:
   - `Project URL` → keep as `SUPABASE_URL`
   - `anon public` key → keep as `SUPABASE_ANON_KEY`
   - `service_role` key → keep as `SUPABASE_SERVICE_KEY` *(treat like a password — never to the repo, never to the frontend)*

**Caveat (from `infrastructure.md` Unknown Unknowns)**: the pooled URL (port 6543) works great with `pgx` in request/response mode, but **does not support `LISTEN/NOTIFY` or prepared statements across multiple transactions**. Not a problem for the MVP; if a change-subscription feature is added later, use the direct URL (port 5432) as a second secret.

**Artifact**: the full `DATABASE_URL` + 3 Supabase keys, stored locally in a password manager / `.env` (gitignored).

---

## 2. Minimal Go skeleton — `[AGENT]`

The agent commits the minimum needed for Railway to have something to build and run:

- `cmd/server/main.go` — `fx.New(config.Module, server.Module).Run()`
- `internal/config/config.go` — reads `PORT` (Railway injects it), `DATABASE_URL`, `APP_ENV`, exports `Module = fx.Module("config", fx.Provide(Load))`
- `internal/server/server.go` — chi router with `/healthz` (returns `{"status":"ok"}`) and `/` (placeholder HTML "MoonPhase v0"); `fx.Lifecycle.Append(OnStart=ListenAndServe, OnStop=Shutdown)`; exports `Module`
- `Dockerfile` — multi-stage `golang:1.26-alpine` → `gcr.io/distroless/static-debian12`; **escape hatch in case Railpack drops Go 1.26 support** (risk #2 in the risk register)
- `.dockerignore` — excludes `.git`, `context/`, `.golangci.yml`, `*_test.go` from the image
- `.env.example` — template: `PORT=8080`, `DATABASE_URL=postgresql://...`, `APP_ENV=development`
- `.gitignore` update — add `.env`, `bin/`, `tmp/`

The agent does not commit `.env` itself (local only; never in the repo).

**Artifact**: commit `feat: minimal server skeleton (fx + chi + /healthz)` on the `main` branch, pushed to GitHub.

---

## 3. Update foundation files — `[AGENT]`

Consistency with the decision in `infrastructure.md`:

- `context/foundation/tech-stack.md`:
  - frontmatter `deployment_target: fly` → `railway`
  - body paragraph: replace the "Fly.io" mention with "Railway"; mention Supabase as the chosen Postgres (instead of "Supabase or Neon")
- `CLAUDE.md`:
  - intro line: "Deploy: Fly.io" → "Deploy: Railway (Postgres: Supabase)"
  - Hard Rules: "Fly.io secrets and a local `.env`" → "Railway variables and a local `.env`"

**Artifact**: a second commit `chore(context): align foundation files with railway+supabase decision`.

---

## 4. Railway setup — `[USER]` → `[AGENT]`

### 4a. `[USER]` — login + project init

```bash
railway login            # opens the browser once, token is cached locally
cd /Users/fsuszko/10x
railway init             # choose "Empty Project", name: moonphase
railway link             # confirms linking cwd ↔ railway project
```

After `railway init`, a project with one empty service appears in the Railway dashboard.

### 4b. `[USER → AGENT]` — hand the agent secrets and let it set variables

On your side: `railway login` caches the token in `~/.railway/auth.json`. The agent can invoke the `railway` CLI in your shell, so it continues on its own from here — provided you give it the `DATABASE_URL` from step 1.

```bash
# AGENT runs (once you provide DATABASE_URL):
railway variables --set "DATABASE_URL=<pooled URL pasted from step 1>"
railway variables --set "APP_ENV=production"
# Optionally, if you want to use the Supabase Auth/SDK server-side:
railway variables --set "SUPABASE_URL=<from step 1>"
railway variables --set "SUPABASE_SERVICE_KEY=<from step 1>"
```

Railway sets `PORT` itself — don't override it.

### 4c. `[AGENT]` — first deploy

```bash
railway up                # build via Railpack (reads go.mod), upload, deploy
# first build ~90s; subsequent ones ~30s with layer cache
```

On success the agent calls `railway domain` (if the service doesn't have a public domain yet) and captures the `*.up.railway.app` URL.

### 4d. `[AGENT]` — smoke test

```bash
curl -fsS https://<deploy-url>.up.railway.app/healthz
# expected: {"status":"ok"}
railway logs --tail 50    # verify fx started, no panic
```

**Artifact**: a working deploy URL + a terminal entry showing a 200 response from `/healthz`.

---

## 5. Post-deploy hardening — `[USER]`

Direct mitigations for the risks in `infrastructure.md`:

1. **Billing alert (mitigates risk #3 — Hobby $5 hard cap)**:
   - Railway dashboard → **Account Settings → Billing → Usage alerts** → threshold `$4.50`, email = yours.
2. **External uptime ping (mitigates risk #3 — early suspension detection)**:
   - https://uptimerobot.com (free tier, 50 monitors every 5 min)
   - Add an HTTP monitor on `https://<deploy-url>.up.railway.app/healthz`, alert to email
3. **Hard rule "no co-located Postgres" (mitigates risk #2)**:
   - Already recorded in `CLAUDE.md` (after step 3) — never run `railway add postgres`. If you ever click it out of curiosity, **remove it immediately** before anything gets written there.
4. **Railpack escape hatch (mitigates risk #1)**:
   - `Dockerfile` from step 2 is already committed. If `railway up` stops building correctly after a Go update, add `RAILPACK_NO_AUTO=true` to the project variables — Railway will switch to the Dockerfile.

---

## 6. Done when ✓

The deploy is considered complete when **all of the following are true**:

1. `curl https://<deploy-url>.up.railway.app/healthz` → `200 OK` with body `{"status":"ok"}`
2. `railway logs --tail 20` shows `fx` started (`[Fx] RUNNING` or a server-start message) with no `panic` or `connection refused`
3. The connection to Supabase works (e.g. a short `SELECT 1` via a diagnostic handler, or `psql "$DATABASE_URL" -c 'SELECT 1'` locally with the same URL)
4. Rollback tested: `railway redeploy <previous-deployment-id>` (from `railway deployment list`) returns the previous version in <60s
5. `context/foundation/tech-stack.md` has `deployment_target: railway` and mentions Supabase
6. `CLAUDE.md`'s intro says "Deploy: Railway", and Hard Rules mention Railway variables (not Fly secrets)

If any point doesn't hold — go back to the relevant step; don't leave `context/` out of sync.

---

## 7. Out of scope (deliberately skipped in this iteration)

- **CI/CD via GitHub Actions** — `tech-stack.md` assumes `manual-promotion`, but the workflow YAML is a separate step. For now, deploy = `railway up` from local.
- **Custom domain + TLS** — the `*.up.railway.app` domain is enough for v1; a custom domain has its own risk-register entry (DNS verification).
- **SQL migrations** — no tables exist yet; the first migration lands together with FR-001 (auth) or FR-016 (catalog seed). At that point `migrations/` is added along with a `psql "$DATABASE_URL" -f migrations/0001_init.sql` step before deploy.
- **FR implementation** — all out of scope for this plan. The next skill (`/10x-frame` / `/10x-implement`) picks up the first FR from the PRD and carries it through.
- **fx + pgx pool performance tuning** — the default `pgxpool.MaxConns` (4) is right for Hobby. Tune only once real traffic shows the need.

---

## Quick reference — most common commands after deploy

```bash
railway logs                              # streaming runtime logs
railway logs --build                      # logs from the last build
railway variables                         # list variables
railway redeploy                          # redeploy with no changes
railway deployment list                   # list deployments (for rollback)
railway redeploy <deployment-id>          # rollback
railway ssh                               # shell in the running container (debug)
railway status                            # service status + URL
```

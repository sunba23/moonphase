---
date: 2026-09-06T13:00:52+0200
researcher: Franek Suszko
git_commit: cdf1466fd787e65a36a7ed4114dcabf5308f490e
branch: feat/observability-expanded
repository: moonphase
topic: "Expand observability — trace/monitor session-suggestion input and decision, error tracing, native + low cost"
tags: [research, codebase, observability, logging, recommender, metrics, railway, supabase]
status: complete
last_updated: 2026-09-06
last_updated_by: Franek Suszko
---

# Research: Expand observability for MoonPhase

**Date**: 2026-09-06T13:00:52+0200
**Researcher**: Franek Suszko
**Git Commit**: cdf1466fd787e65a36a7ed4114dcabf5308f490e
**Branch**: feat/observability-expanded
**Repository**: moonphase

## Research Question

Improve observability of the application. Main goals:

1. Monitor the generated session suggestions — **input and decision** ("reason why climb X was picked", keyed by session id, grep-able, only fields that help improve the algorithm later, no waste).
2. **Traces of anything that goes wrong.**
3. Prefer **native Railway/Supabase** solutions (easy integration).
4. Traces, logs and metrics must **not add excessive network traffic and compute** — everything costs.

Owner rulings during scoping:

- Recommender decision tracing = **one structured log line per pick, keyed by session id**. Not OpenTelemetry spans.
- Metrics **are in scope**, via a Prometheus `/metrics` endpoint.
- Research ends in **one recommended approach**, ready for `/10x-plan`.

## Summary

The app already has the right skeleton: zerolog JSON to stdout (`internal/logging`), one access-log middleware, and a purpose-built `recommender.PickDiag` struct that was *designed* to be the handler-side logging hook but is only half-used today (logged only when a fallback tier fires). Railway ingests stdout JSON natively and makes every field queryable — so the cheapest, most "native" path is to **lean entirely on structured stdout logs** and add almost no network/compute.

**Recommended approach (details in [Recommended Approach](#recommended-approach)):**

| Pillar | What to do | New deps | Cost |
|---|---|---|---|
| **Recommender decision log** | Extend `PickDiag` (+ a `FirstPickDiag`) to carry the full input→decision, emit **one `rec_pick` JSON line per pick** from the session handler, keyed by `session_id`. Tight field set (~15). | none | ~1 log line per RPE submit |
| **Error / "anything wrong" tracing** | `request_id` correlation via `zerolog/hlog`; a ~15-line panic-recovery middleware that logs `panic`+`stack`; close the silent-500 gaps; route `fx` lifecycle logs through zerolog (kills the unparseable `[Fx]` stderr noise). | none | zero steady-state |
| **Metrics** | `prometheus/client_golang` + `/metrics` on a chi route (bounded-label RED metrics + recommender tier/band counters). Protect the route. | `prometheus/client_golang` (1 direct) | tiny in-process; **see the consumption gap below** |
| **Alerting** | Railway **project webhook → Slack/Discord**, filtered to `Deployment.failed` + `Deployment.crashed`. Native, $0. | none | none |
| **DB / auth visibility** | Use Supabase **Logs Explorer** (Postgres + Auth sources) and the **Query Performance** report — already on. Add a `/readyz` that pings the pool. | none | none |

**The one gap to decide before planning:** the owner's premise "Railway has a Prometheus Exporter that uses `/metrics`" is **not how Railway works**. Railway does **not** scrape your app's `/metrics`. It only collects platform CPU/RAM/net, plus free **edge HTTP metrics** (request rate, status buckets, error rate, latency p50/p90/p99) for any public service. To actually consume an app `/metrics` endpoint you must run your own scraper+store (Prometheus / VictoriaMetrics) as an extra always-on Railway service — which is exactly the "everything costs" compute the owner wants to avoid. See [Open Questions](#open-questions) for the three ways to resolve this.

## Detailed Findings

### 1. Current observability state (codebase)

#### 1.1 The logging package — minimal, no knobs

- `internal/logging/logging.go:9-13` — `New()` returns `zerolog.New(os.Stdout).With().Timestamp().Logger()`. Compact JSON, fields `level` / `time` / `message`.
- `internal/logging/module.go:5` — `fx.Module("logging", fx.Provide(New))`.
- **No level control** anywhere — no `zerolog.SetGlobalLevel`, no `LOG_LEVEL` env, no build flag (grep-confirmed). All levels emit in all environments.
- **No child/context logger pattern.** The single `.With()` is inside `New()` itself. No `logger.With().Str(...).Logger()`, no `zerolog.Ctx(ctx)`, no `hlog`. Every handler struct holds the same `*zerolog.Logger` pointer from `NewRouter`.
- `cfg.AppEnv` never reaches logging — its only effect is the cookie `Secure` flag (`internal/server/server.go:28`). Log format is identical dev vs prod.
- Origin: change `auth-session-scaffold`, Phase 4, commit `6e41c3f` — added reactively after "an orphaned process held port 8080 … invisible because the app had no request logging at all" (`context/changes/auth-session-scaffold/plan.md:221-224`). zerolog was chosen, never argued against slog/logrus.

#### 1.2 HTTP middleware — one custom logger, nothing else

- `internal/server/server.go:30` — `r.Use(requestLogger(logger))` is the **only** top-level `r.Use`. Group-level: `auth.Middleware` (`:57`), `OnboardingGate` (`:67`).
- `internal/server/middleware.go:12-29` — `requestLogger` emits one `Info` line `Msg("http_request")` with `method`, `path` (raw `r.URL.Path`, **not** the route pattern), `status`, `bytes`, `duration`, `remote_addr`. Emitted **after** `next.ServeHTTP` returns — **not deferred**, so a panicking handler emits **no access-log line at all**.
- **Not wired:** `middleware.RequestID`, `middleware.Recoverer`, `middleware.RealIP`, `middleware.Timeout`, `middleware.Heartbeat`.
- **No panic recovery anywhere** — `grep recover()` = 0 hits. A handler panic hits `net/http`'s per-connection recovery → unstructured stack to stderr, connection dropped, no app log, no 500 access line.
- **No request-id / correlation-id concept.** The only per-request identifier in any log is `session` (the `sessionID` URL param) at `internal/server/session.go:132` and `:237`. Access-log lines and `session: …` error lines for the same request **cannot be correlated**.

#### 1.3 Error handling — consistent shape, several silent holes

Canonical pattern (~30 sites, e.g. `internal/server/session.go:46-51`):
```go
x, err := store.Op(ctx, ...)
if err != nil {
    s.logger.Error().Err(err).Msg("session: load profile failed")
    http.Error(w, "internal error", http.StatusInternalServerError)
    return
}
```
- Lower layers wrap: `fmt.Errorf("session: <op>: %w", err)` in stores/catalog/recommender. Handlers log the wrapped error via `.Err(err)` and discard it. No detail leaks to response bodies (fixed strings only). Auth form errors intentionally render `apiErr.Message` into HTML (`internal/server/auth_pages.go:64`).
- **Silent 500s (no log):**
  - `internal/server/onboarding_gate.go:38` — a non-`ErrNotFound` DB error in the gate is **completely swallowed** → 500 with zero log. *Biggest hole* — a DB outage here is invisible.
  - `internal/server/server.go:93` — `handleMe` no-user-id branch.
  - `internal/server/session.go:42,114,165,287` etc. — no-user-id branches log nothing.
  - `internal/server/session.go:120-123` (and `:171`, history detail) — `s.sessions.Get` error is folded into the ownership check (`err != nil || sess.UserID != userID`) → real DB error silently becomes a 404, no log.
  - `internal/server/render.go:36`, `auth_pages.go:96` — `_ = c.Render(...)` — template render errors explicitly discarded.
- `internal/server/session.go:132` — logs `Error` with `Str("session", …)` but **no `.Err()`** (data-invariant violation, no error object).
- Message-prefix convention drifts: `"session: …"`, `"history: …"`, `"profile: …"`, `"onboarding: …"`, but `auth_pages.go:71` = `"auth submit failed unexpectedly"`, `server.go:120/124` = `"server …"`.

#### 1.4 The recommender decision data — `PickDiag`, mostly unused

- `internal/recommender/recommender.go:98-105`:
  ```go
  type PickDiag struct {
      FallbackTier     int
      GradeLo          string
      GradeHi          string
      ExcludedDominant string
  }
  ```
  Populated in `PickNext`: window at `:169`, `ExcludedDominant` at `:201`, `FallbackTier` as the tiered walk progresses (`:233,246,261,277,290`). **Does not carry the chosen pick** — that's on the returned `Pick` (`ProblemID`, `ConfigurationID`, `Grade`; `:28-32`).
- `internal/recommender/grade_window.go:33-44` — `classify(Result) band` is the core decision: `failed/bailed OR RPE>=8 → backOff`; `RPE 5-7 & sent → hold`; `RPE<=4 & sent → stepUp`. The resulting `band` is **not surfaced in `PickDiag`** — it's a local var in `PickNext` (`:162`).
- `internal/recommender/score.go:89-109` — `scoreNext` returns only the winning index. The winning **score**, and the **tie-set size** (how many candidates were within `scoreEpsilon` and resolved by coin-flip), are computed and discarded. These directly answer "was this pick decisive or arbitrary".
- Consumption in `internal/server/session.go:223-238` — `diag` is read at **exactly one place**:
  ```go
  if diag.FallbackTier > 0 {
      s.logger.Warn().Int("tier", diag.FallbackTier).Str("session", sessionID).Msg("session: next pick used a fallback tier")
  }
  ```
  `diag.GradeLo`, `diag.GradeHi`, `diag.ExcludedDominant` are **never read anywhere** (grep-confirmed). The **success path logs nothing** — a normal pick produces zero recommender output.
- Data available at that call site but **not logged**: `rpe` (`:191`), `completion` (`:196`), `seq` (`:186`), `shown` list (`:202`, each carries `ProblemID/Grade/Dominant/RPE/Completion`), `last` climbed problem (`:212`), `sess.MaxGrade`, `sess.Holdsetup`, `sess.Angle`, `pick.ProblemID/ConfigurationID/Grade`, `userID` (`:163`). Session id is `sessionID string` (`:169`), also `sess.ID`.
- Prior ruling (do not break): the `recommender` package **stays pure / no logger**; fallback-tier logging is the handler's job driven off `PickDiag` — `context/changes/adaptive-main-session-loop/plan.md:43,111,559,769`.
- `recommender` unit tests assert directly on `PickDiag` fields (`internal/recommender/pick_next_test.go`, `recommender_test.go`) — **changing `PickDiag`'s shape breaks tests**; changing log strings does not (no test asserts on logs; server tests use `zerolog.Nop()`).

#### 1.5 Storage / catalog / db — zero logging, wrapped errors only

- `internal/session/store.go`, `internal/profile/store.go`, `internal/catalog/*.go` — no logger injected, every path returns `fmt.Errorf("<pkg>: <op>: %w", err)`. Sentinels (`ErrNoActiveSession`, `ErrNotFound`, `ErrActiveExists`, `ErrStaleResult`) returned bare for `errors.Is`.
- `internal/db/pool.go` — **no pgx tracing**. `poolCfg` is touched only at `:28` (`DefaultQueryExecMode = QueryExecModeSimpleProtocol`, PgBouncer workaround). No `ConnConfig.Tracer`, no `tracelog`, no slow-query threshold, no pool hooks. `OnStop` hook closes the pool silently.

#### 1.6 fx lifecycle logging — unstructured stderr noise

- `cmd/server/main.go:20-29` — `fx.New(config, logging, db, auth, profile, session, recommender, server).Run()`. **No `fx.WithLogger`** (grep-confirmed).
- Consequence: fx's default `fxevent.ConsoleLogger` writes verbose `[Fx] …` **text** to **stderr** for every provide/invoke/hook. Railway captures it, but it is **not single-line JSON**, so Railway can't parse it — it pollutes the log stream and defeats attribute queries around startup.

#### 1.7 Health check — static stub

- `internal/server/server.go:32-35` — `/healthz` always returns `200 {"status":"ok"}`. Does **not** ping the DB / pool, even though `pool *pgxpool.Pool` is in scope. Unauthenticated (correct), behind `requestLogger`. `infrastructure.md:79` assumes a UptimeRobot ping here.

#### 1.8 Config — no observability vars

- `internal/config/config.go` reads exactly: `PORT` (def `8080`), `APP_ENV` (def `development`), `SUPABASE_URL` (required), `SUPABASE_PUBLISHABLE_KEY` (required), `DATABASE_URL` (required). No `LOG_LEVEL`, `OTEL_*`, `SENTRY_DSN`, metrics auth, etc.
- In-flight on `origin/feat/ci` (`context/changes/ci-and-release-flow/`): `RAILWAY_GIT_COMMIT_SHA` added to `config.Load()` (fallback `dev`) for a version footer. Any new env var must be added here (lesson `context/foundation/lessons.md:19-24` — CLIs must auto-load `.env`).

#### 1.9 Deploy shape

- Root `Dockerfile` — multi-stage → `gcr.io/distroless/static-debian12`, `EXPOSE 8080`, `ENTRYPOINT ["/server"]`. Railway will build from this Dockerfile (not Railpack) since it is present. A `/metrics` route in a distroless image is fine (no shell needed).
- No `.github/` on disk yet; CI planned in `ci-and-release-flow`. `origin/feat/bailed-to-skipped` — the `bailed`→`skipped` rename (PRD FR-008 update dated today) is **not merged**; `internal/session/session.go:71-85` still has `CompletionBailed = "bailed"` and the handler has no skip path. Not blocking, but the decision log's `completion` field should tolerate whatever values exist.

### 2. Railway — native observability (2026)

Sources: `docs.railway.com/observability/logs`, `/observability/metrics`, `/cli/metrics`, `/guides/structured-logging-production`, `/guides/alerts-crashes-failed-deploys`, `/observability/webhooks`, `/guides/third-party-observability`, `/networking/private-networking`, `/pricing/cost-control`.

- **Logs — the strong suit.** Captures all stdout/stderr, no config. **Valid single-line JSON is parsed**: `message`/`msg` → text, `level` (or numeric) → severity, **every other field → queryable attribute** (`@field:value`, numeric `>`/`<`/ranges `100..500`, `AND`/`OR`/`-`, parentheses). Views: per-deploy panel, environment Log Explorer, `railway logs`. **Retention: 7 days on Hobby** (30 Pro). **No billed log quota** — only a rate limit of **500 lines/replica/second** (excess dropped). Hard requirement: one JSON object per line, no pretty-printing.
- **Edge HTTP logs/metrics — free, zero code.** Any service with a public domain gets edge-recorded HTTP logs (`@method`, `@path`, `@httpStatus`, `@responseTime`, `@totalDuration`, `@srcIp`, `@edgeRegion`) **and** HTTP metrics: request totals, status-code buckets, **error rate, latency p50/p90/p99** — dashboard + `railway metrics --http` (filter `--method` / `--path`). This alone covers the PRD's two latency guardrails at the HTTP layer.
- **App `/metrics` scraping — NOT a thing.** The Metrics tab collects only CPU/Memory/Disk/Network (≤30 days). Docs, verbatim: *"Application-level metrics such as request latency, error rates, or business KPIs are not collected by Railway. To capture these, ship telemetry to a third-party tool."* The **"Prometheus Exporter" template** (`railway.com/deploy/prometheus-exporter`) is a **separate always-on container** that re-exports Railway's *own platform metrics* (via Railway's GraphQL API) for an *external* Prometheus — it does not touch your app. Configured by env vars (`RAILWAY_API_KEY`, `ENVIRONMENT_TARGETS`, `PORT`); billed as compute like any service.
- **No native OTLP endpoint, no hosted collector, no tracing product.** Guidance is "run your own OTel Collector as a service". Also verbatim: *"No log drain feature. Railway does not have a setting to forward stdout to an external intake URL."* (you'd run Vector/Fluent Bit yourself).
- **Alerting:**
  - **Project webhooks** (free, all plans): `Deployment.failed` (build/deploy error), `Deployment.crashed` (running deploy exited), volume-usage. JSON POST, 3 retries. **Native Slack + Discord muxers** — paste the incoming-webhook URL, Railway reshapes the payload, no middleware. Not signed → put a secret segment in the URL.
  - **Metric monitors** (CPU/RAM/disk/egress thresholds → email/in-app/webhook): **Pro plan only**. Not available on Hobby.
  - **No log-pattern alerting** anywhere.
  - **Billing alerts:** Workspace Usage page — soft (email) + hard (shutdown) limits, reminders at 75/90/100%.
- **Private networking:** WireGuard, IPv6, zero-config, `SERVICE.railway.internal:PORT`. A metrics/telemetry service can be private-only (just don't add a public domain). **Intra-environment private traffic is not billed**; public egress is $0.05/GB. Listener must bind `[::]:PORT` (dual-stack).

### 3. Supabase — native observability (2026)

Sources: `supabase.com/docs/guides/telemetry/logs`, `/telemetry/metrics`, `/guides/database/extensions/pg_stat_statements`, `/guides/database/extensions/index_advisor`, `/guides/monitoring-and-debugging/log-drains`, `/guides/auth/audit-logs`, `supabase.com/pricing`.

- **Logs Explorer** — 9 sources (API Gateway, Postgres, PostgREST, **Auth**, Storage, Edge Function, Realtime, Supavisor, PgBouncer), Logflare-backed, SQL-queryable from Studio/MCP/API. Filters: time, type, level, status, method, path, message, user. **Retention: Free 1 day**, Pro 7 days, Team 28 days.
- **Reports** — per-product dashboards + a **Query Performance** report (built on `pg_stat_statements`, on by default): queries by total exec time / call count / avg time. **Performance Advisor** + `index_advisor` extension suggest indexes. This is the native "which queries are slow" answer, $0.
- **Failing queries** (errors, constraint violations) → **Postgres** log source, filter to error level.
- **Auth / GoTrue** — sign-in errors, failed sign-ups, 500s, JWKS/token errors → **Auth** log source; plus **Auth Audit Logs** (`auth.audit_log_entries` table, API-queryable).
- **Metrics API** — Prometheus-format endpoint `https://<ref>.supabase.co/customer/v1/privileged/metrics` (Basic auth, ~200 Postgres series), scrape 1/min. Pricing page lists it as **not included on Free** (historically worked on Free with `service_role` — treat "Pro required" as current-but-uncertain). Pairs with the published `supabase/grafana` dashboard (~200 charts).
- **Log Drains** (Pro paid add-on since 2026-03-05) — HTTP / **OTLP** / Datadog / Loki / S3 / Sentry / Axiom / Last9 / Syslog. **$60 per drain per project per month** + $0.20/M events + $0.09/GB. Out of budget for this change.

### 4. Go library landscape (2026)

Verified versions: `chi/v5` v5.3.2, `rs/zerolog` v1.35 (`hlog` published 2026-04-20, still canonical), `prometheus/client_golang` v1.24.1, `fx` v1.24.

- **Session/request-scoped logging — no new deps.** `zerolog/hlog` middlewares are plain `func(http.Handler) http.Handler`: `hlog.NewHandler(base)` (logger→ctx, copied per request so concurrent `UpdateContext` is race-free) + `hlog.RequestIDHandler("request_id","X-Request-Id")` (generates id, sets response header) + `hlog.AccessHandler(fn)` (one line per request, sees final status). Downstream reads `zerolog.Ctx(ctx)`. Add `session_id` in the handler once resolved: `zerolog.Ctx(r.Context()).UpdateContext(func(c) { return c.Str("session_id", sid) })`.
  - `hlog` vs chi `middleware.RequestID` + manual: both zero-dep. `hlog` wins — it sets the response header (client correlation) and auto-attaches the id as a log field. Pick one id source, not both.
- **Panic recovery.** chi `middleware.Recoverer` recovers + writes 500 + **does not re-panic**, but dumps a colorized stack to `os.Stderr` with **no exported hook** for a structured logger. Recommendation: **drop it**, use a ~15-line `Recover` middleware that logs `Interface("panic", rvr).Bytes("stack", debug.Stack())` through `zerolog.Ctx(ctx)` at `Error` and lets `http.ErrAbortHandler` propagate. Captures `request_id`/`session_id` for free.
- **`middleware.RealIP` is deprecated + IP-spoofable** (GHSA-3fxj-6jh8-hvhx et al.). If client IP is wanted behind Railway's edge, use `middleware.ClientIPFromXFFTrustedProxies` (chi ≥ v5.3.0) — but client IP is low value here; `hlog.RemoteAddrHandler` logging the raw peer is enough, or omit.
- **Middleware order:** `hlog.NewHandler` first → `hlog.RequestIDHandler` → prom middleware (outermost timer, sees 500s) → `Recover` (inside metrics/access so a recovered panic is still counted+logged) → `hlog.AccessHandler` → `middleware.Timeout`.
- **fx quieting — no new dep.** Hand-roll `fxevent.Logger` (one method, `LogEvent`), route to zerolog: errors at `Error`, one `Info` "fx started", the rest `Debug`. `fx.WithLogger(func(log *zerolog.Logger) fxevent.Logger { return fxZerolog{...} })`. (`fxevent.SlogLogger` exists but we're not on slog; `github.com/ipfans/fxlogger` is the prebuilt option if hand-rolling is unwanted — 1 dep.)
- **Metrics — `prometheus/client_golang` only, not the OTel SDK.** OTel metrics SDK + Prometheus exporter adds a much larger module tree and ~+50% CPU/mem vs native `client_golang` for zero benefit when the consumer is a Prometheus scrape. (`otel` at v1.44 being *indirect* doesn't make the SDK/exporter cheap.) Keep OTel in reserve for traces only, if ever.
  - Minimal: `promauto.NewCounterVec` + `promauto.NewHistogramVec` (set both `Buckets: prometheus.DefBuckets` and `NativeHistogramBucketFactor: 1.1` for forward-compat), `r.Handle("/metrics", promhttp.Handler())`.
  - chi middleware: hand-rolled ~20 lines using `chi.RouteContext(r.Context()).RoutePattern()` (templated path — **IDs never leak into labels**) + `middleware.NewWrapResponseWriter` for status.
  - Default registry already carries Go runtime + process collectors (~60-90 series, tens of KB, negligible CPU). Fine to keep.
- **Label cardinality rule (confirmed):** never label with `session_id` / `user_id` / `problem_id` / names / raw paths / error strings. **Safe labels for MoonPhase:** `route` (chi pattern), `method`, `status_class` (`2xx`…`5xx`), recommender `tier` (`0`…`4`) and `band` (`back_off`/`hold`/`step_up`), `completion` (`sent`/`failed`[/`skipped`]), optionally `board_set` (4) / `angle` (2). Histogram: `route`×`method` only (don't multiply buckets by status).
- **slog vs zerolog:** keep zerolog — migration is churn with no user benefit; zerolog is still the allocation leader and maintained. Bridge with `samber/slog-zerolog` only if a library forces `*slog.Logger` (fx and pgx both take adapter interfaces, so no forced dep).

## Recommended Approach

One change, four slices. Everything below is stdout-only except the `/metrics` endpoint; steady-state added network traffic is **one JSON log line per RPE submit** plus whatever a future scraper pulls.

### A. Recommender decision log (the primary goal)

**Keep `recommender` pure.** Extend the diag structs; the session handler emits the line.

1. **`internal/recommender`:**
   - Add `Band string` to `PickDiag` (values `back_off` / `hold` / `step_up` from `classify`) — set it in `PickNext` where `b := classify(...)` already runs (`recommender.go:162`).
   - Add `PreferredGrade string` to `PickDiag` (the ladder grade at `prefIdx` after clamping — `recommender.go:181-187`).
   - Add `TieSetSize int` and `WinningScore float64` to `PickDiag` — thread them out of `scoreNext` (`score.go:89`) via the tiered `score` closure (`recommender.go:219-230`). Cheap (2 numbers), directly answers "decisive vs coin-flip".
   - Add a `FirstPick` variant that returns a small `FirstPickDiag{Grade string, PoolSize int, Quality bool}` so session-start (seq 0) also logs a decision. (Or a shared minimal shape.)
   - Update `internal/recommender/*_test.go` for the widened structs (tests already assert on `PickDiag` — expected, low effort).
2. **`internal/server/session.go` — `handleResult` (after `PickNext`, before/around `AdvanceSession`):** emit **one line**, always (not just on fallback), replacing the current `diag.FallbackTier > 0` warn:
   ```
   msg  "rec_pick"
   session_id      <sessionID>
   seq             <seq+1>            // the pick's seq
   prev_problem_id <last.ProblemID>
   prev_grade      <last.Grade>
   prev_dominant   <last.Dominant>
   rpe             <rpe>
   completion      <completion>
   band            <diag.Band>        // classify() decision
   ceiling         <sess.MaxGrade>
   grade_lo        <diag.GradeLo>
   grade_hi        <diag.GradeHi>
   pref_grade      <diag.PreferredGrade>
   excluded_dominant <diag.ExcludedDominant>   // streak-break, "" if none
   fallback_tier   <diag.FallbackTier>
   tie_set_size    <diag.TieSetSize>
   pick_problem_id <pick.ProblemID>
   pick_grade      <pick.Grade>
   pick_dominant   <lookup or carry from candidate>   // "off crimp?" answer
   ```
   ~18 fields. Drop `user_id` (derivable via session), drop raw shown-list (too heavy; `prev_*` + counts are enough — reconstruct history by grepping `session_id`). `pick_dominant` needs the chosen candidate's dominant — either add `Dominant` to `recommender.Pick` or look it up; adding to `Pick` is cleaner and cheap.
   - Keep it `Info` level. Railway makes every field a filter: `@session_id:"…" AND @msg:rec_pick` reconstructs the whole session's decision trail; `@fallback_tier:>0`, `@band:back_off AND @pick_grade:…` etc. for algorithm tuning.
3. **`internal/server/session.go` — `handleStart`:** emit `msg "rec_pick" seq:0 session_id:… pick_problem_id:… pick_grade:… + FirstPickDiag fields`.

### B. Error / "anything that goes wrong" tracing (no new deps)

1. **New `internal/server` middleware file** (or extend `middleware.go`):
   - `hlog.NewHandler(*logger)` + `hlog.RequestIDHandler("request_id","X-Request-Id")` as the first two `r.Use` in `NewRouter` (`server.go:30`).
   - Replace `requestLogger` with `hlog.AccessHandler` emitting the same fields **plus** `request_id` and `route` (`chi.RouteContext(r.Context()).RoutePattern()` instead of raw `path`). Keep `Msg("http_request")` for continuity.
   - New `Recover` middleware (panic → `zerolog.Ctx(ctx).Error().Interface("panic",…).Bytes("stack", debug.Stack())` → 500), placed inside access/metrics, outside handlers.
2. **Thread `session_id` into the ctx logger** in the session + history handlers once `sessionID` is known: `zerolog.Ctx(r.Context()).UpdateContext(c => c.Str("session_id", sessionID))`. Then every downstream `session: … failed` line auto-carries `session_id` + `request_id` — full correlation.
3. **Close the silent-500 gaps** (small, mechanical):
   - `internal/server/onboarding_gate.go:38` — log the swallowed DB error at `Error`.
   - `internal/server/session.go:120-123`, `:171`, history detail — split the `Get` error from the ownership check; log a real error, keep the 404 body.
   - `render.go:36`, `auth_pages.go:96` — log discarded render errors at `Error`.
   - no-user-id branches — log at `Error` (they're "impossible" post-auth-middleware, so a hit is a real bug).
   - `session.go:132` — attach a sentinel/error so the line isn't error-less.
4. **fx logs → zerolog:** hand-rolled `fxevent.Logger` adapter + `fx.WithLogger(...)` in `cmd/server/main.go`. Errors at `Error`, `Info` "fx started", rest `Debug`. Removes unparseable `[Fx]` text from the stream.
5. **`/readyz`** — new route that `pool.Ping(ctx)` with a 2s timeout → 200/503. Keep `/healthz` as the liveness stub. (`/readyz` for UptimeRobot / Railway healthcheck path.)
6. **Optional `LOG_LEVEL` env** in `internal/config/config.go` (default `info`) → `zerolog.SetGlobalLevel` in `logging.New()` or `main`. Lets you silence `Debug` in prod and crank it when debugging without a redeploy-code change. One field, cheap.

### C. Metrics (`prometheus/client_golang`, 1 direct dep)

1. **New `internal/metrics` package** exporting `Module = fx.Module("metrics", fx.Provide(New))` (matches the repo's per-package fx-module convention):
   - `moonphase_http_requests_total{route,method,status_class}` — counter.
   - `moonphase_http_request_duration_seconds{route,method}` — histogram (`DefBuckets` + `NativeHistogramBucketFactor: 1.1`).
   - `moonphase_recommender_picks_total{band,tier}` — counter (bump in `handleResult` next to the `rec_pick` log).
   - `moonphase_recommender_first_picks_total{}` — counter.
   - `moonphase_result_submissions_total{completion}` — counter.
   - Keep default registry (Go runtime + process collectors).
2. **`/metrics` route** in `NewRouter` — `promhttp.Handler()`, **protected**: bearer/basic check against a `METRICS_TOKEN` env var (add to `config.go` + `.env.example`), OR bind metrics on a second `http.Server` on a private-only port. Simplest first pass: token middleware on the one route.
3. **Prom middleware** — hand-rolled ~20 lines, added to the `r.Use` chain as the outermost timer.

### D. Alerting + platform (native, $0, mostly dashboard config — document, don't code)

1. **Railway project webhook** → Slack or Discord incoming webhook, filtered to `Deployment.failed` + `Deployment.crashed`. Secret segment in the URL. (Dashboard step — capture in the plan's manual-verification / a `context/changes/observability-expanded/` runbook note.)
2. **Railway billing alert** at ~$4.50 (Workspace Usage) — the `infrastructure.md` risk register already calls for this.
3. **Supabase** — no code. Document: Logs Explorer (Auth + Postgres sources), Query Performance report, Performance Advisor as the DB-side observability surface. Note Free = 1-day log retention; upgrade to Pro ($25) only if that bites.
4. **Latency guardrails** (~10s first pick / ~3s next pick, `prd.md` NFRs, `test-plan.md` Risk #7 "no measurement exists today") — now covered three ways: Railway edge HTTP latency percentiles (free), the `moonphase_http_request_duration_seconds` histogram, and `@duration` on the access log.

### What this deliberately does NOT do

- No OpenTelemetry (SDK, collector, OTLP) — owner ruled decision-tracing = structured logs; no distributed-tracing need for a single service.
- No Sentry / Grafana Cloud / Better Stack — revisit only if "alert on error-rate spike / caught-but-not-crashed panics" becomes a real need (Railway can't do it; the codebase inventory shows crashes are the main failure mode today).
- No Supabase Log Drains ($60/drain).
- No `slog` migration.
- No always-on exporter/scraper container **in this change** — see Open Questions.

## Code References

- `internal/logging/logging.go:9-13` — the entire logger factory (no level, no child pattern).
- `internal/server/middleware.go:12-29` — `requestLogger`; not deferred, no request id, raw path.
- `internal/server/server.go:30` — the only top-level `r.Use`.
- `internal/server/server.go:32-35` — static `/healthz`.
- `internal/server/session.go:223-238` — `PickNext` call + the only `diag` consumption (`FallbackTier > 0` warn).
- `internal/server/session.go:169` — `sessionID` available for correlation.
- `internal/server/onboarding_gate.go:38` — swallowed DB error (worst silent-500).
- `internal/recommender/recommender.go:98-105` — `PickDiag` (extend here).
- `internal/recommender/recommender.go:162` — `b := classify(...)` (surface `band`).
- `internal/recommender/score.go:89-109` — `scoreNext` discards score + tie-set size.
- `internal/recommender/grade_window.go:33-44` — `classify` (the RPE→band rule).
- `internal/db/pool.go:28` — the only pool config; no tracer.
- `cmd/server/main.go:20-29` — `fx.New(...).Run()`, no `fx.WithLogger`.
- `internal/config/config.go:16-49` — all env vars (add `LOG_LEVEL`, `METRICS_TOKEN` here).
- `Dockerfile` — distroless; `/metrics` route is fine.

## Architecture Insights

- **The design already anticipated this work.** `PickDiag` exists precisely as "how PickNext reached its answer, for handler-side logging" (`recommender.go:98`). The change is finishing that intent, not retrofitting.
- **"Keep the engine pure" is a load-bearing convention** (`adaptive-main-session-loop` ruling). Widening a returned diag struct respects it; injecting a logger into `recommender` would break it and its dense unit tests.
- **zerolog JSON to stdout + Railway's parser = the whole platform.** Every structured field is a free queryable dimension with 7-day retention and no volume bill. This is why "native + low cost" points overwhelmingly at logs, not at a metrics/tracing pipeline.
- **The one real correlation gap** is request_id — without it the access line and the `session: … failed` lines can't be tied together. `hlog` closes it with zero deps.
- **Metrics is the awkward pillar** — Railway gives platform + edge-HTTP metrics free but won't scrape the app. A `/metrics` endpoint is cheap to expose and standard, but *consuming* it costs a container. The edge HTTP metrics already cover the latency NFRs, so app metrics are a "nice to have that needs a consumer decision", not a blocker.
- **Log field naming needs a convention** now that fields become query keys — `snake_case`, stable names, `msg` as a discriminator (`http_request`, `rec_pick`, `panic recovered`). Worth a short `CLAUDE.md` / `AGENTS.md` entry (none exists today).

## Historical Context (from prior changes)

- `context/changes/auth-session-scaffold/plan.md:217-231` + `reviews/impl-review.md:23-40` — `internal/logging` (zerolog) + request-logger middleware were an **unplanned but accepted** fix for an invisible port-bind failure. `NewRouter` contract updated to carry `*zerolog.Logger`. No slog-vs-zerolog rationale recorded.
- `context/changes/adaptive-main-session-loop/plan.md:43,111,559,769` — **recommender stays pure/no-logger**; fallback-tier logging is the handler's job via `PickDiag`. `handler` logs `Warn().Int("tier", …)`.
- `context/changes/ci-and-release-flow/` (status: implementing, `origin/feat/ci`) — adds `RAILWAY_GIT_COMMIT_SHA` to `config.Load()` (fallback `dev`) + a deployed-version footer. A non-blocking `bench` CI job runs `BenchmarkPickNext`/`BenchmarkFirstPick` for guardrail drift. Coordinate: the new config vars land in the same `config.go`.
- `context/changes/adaptive-loop-ramp-fix/plan.md` — references the fallback-tier funnel + perf benchmark (Risk #7); no new logging decisions.
- `context/foundation/roadmap.md:59` — Baseline: *"Observability: absent … no app-level structured logging, error tracking, or metrics."* No dedicated observability slice/milestone exists.
- `context/foundation/test-plan.md:52,67,122` — Risk #7: latency guardrails "silently missed … No measurement exists today"; notes "Railway + Supabase MCPs available — relevant to Risk #7 post-deploy observation (`http_response_time`, `service_metrics`); not used now". LLM "trajectory judge" for the recommender was **dropped** 2026-09-02 (cost, non-determinism).
- `context/foundation/prd.md:143` — Non-Goal *"No analytics dashboards or trend charts"* (user-facing product analytics — distinct from ops observability, but the aversion to dashboards is real; `CLAUDE.md` lists "no analytics" as binding).
- `context/foundation/prd.md` Access-Control NFR — session history / RPE / max grade *"never … in aggregated/anonymized form"* — bounds what any telemetry may capture/export (the `rec_pick` log stays on Railway, private; no third-party export of per-user data).
- `context/foundation/infrastructure.md:71,79` — ops story already assumes `railway logs`, `get_logs`/`list_deployments` MCP calls, billing alerts at $4.50, UptimeRobot on `/healthz`.
- `context/foundation/lessons.md:19-24` — new env vars must be auto-loaded from `.env` by CLIs (godotenv already wired in `main.go`).

## Related Research

None — `context/archive/` has no changes yet; this is the first `research.md` touching observability.

## Open Questions

1. **Metrics consumption — the premise gap.** Railway will not scrape the app's `/metrics`. Pick one before planning the metrics slice:
   - **(a) Ship `/metrics` now, consume later (recommended).** Expose the endpoint + instrument the code this change. For now rely on Railway edge HTTP metrics + `moonphase_http_request_duration_seconds` visible only on-demand (`curl` through the private network). Add a scraper when it actually hurts. Lowest cost, endpoint is ready.
   - **(b) Add a single lightweight scraper container** — VictoriaMetrics single-node (lighter than Prometheus+Grafana) as a private Railway service scraping `moonphase.railway.internal:PORT/metrics`. ~1 small always-on container (~$1-3/mo compute). Real dashboards + alerting-capable.
   - **(c) Push to Grafana Cloud free tier** — app remote-writes (no scrape target to expose/protect); 10k series / 14-day retention free. One vendor dependency, minimal egress, no container. Best dashboards for $0 but not "native".
   - Owner's stated model ("Railway Prometheus Exporter uses /metrics") maps to none of these exactly — worth a quick confirm of which tradeoff they want.
2. **`rec_pick` — include the per-candidate score distribution?** The winning score + tie-set size are in. Do we also want the top-3 runner-up `(problem_id, score)` for real algorithm tuning? Adds ~6 fields per line. Lean no for v1 (grep by `session_id` + re-run the scorer offline if needed), but it's the highest-value "improve the algorithm" data.
3. **`/metrics` protection** — token middleware (simple, one env var) vs a second private-only `http.Server` (cleaner isolation, more wiring). Recommend token for v1.
4. **Retention** — Hobby = 7-day Railway logs, Free = 1-day Supabase. Is 7 days enough for algorithm analysis, or should `rec_pick` lines also be persisted (e.g. a `session_problem_results` column already holds RPE/completion — the *decision* is what's missing)? Persisting the pick rationale in Postgres is a bigger scope; flag for a follow-up change, not this one.
5. **`bailed`→`skipped` rename** (`origin/feat/bailed-to-skipped`, unmerged) — the decision log's `completion` / `band` handling should be written against whichever lands first. Sequence this change after that merge, or make the field pass-through.

# Repository Guidelines

MoonPhase is an adaptive MoonBoard training coach: a Go web app that picks the next problem during a session from per-problem RPE + completion feedback. Stack: Go std lib + chi (routing) + templ (typed HTML) + HTMX + pgx (Postgres) + uber-go/fx (DI + lifecycle). Deploy: Railway. See @go.mod.

## Read Before Coding

The repo is greenfield — only `go.mod` exists. Foundation docs are authoritative; do not invent product decisions.

- **@context/foundation/prd.md** — 15 functional requirements (FR-001…FR-016, FR-013 dropped), NFRs, business logic, non-goals. Treat FR IDs as the unit of work.
- **@context/foundation/tech-stack.md** — locked stack + deployment.
- **@context/foundation/shape-notes.md** — discovery trail with Socratic rulings (e.g. RPE 1–10 over 3-button, no separate warmup mode, multi-set catalog).

## Hard Rules

- Do not add features outside the PRD's Functional Requirements. The 12 Non-Goals in `prd.md` are binding (no LEDs, no social, no ML, no analytics, no warmup mode, no end-of-session grade refinement).
- Do not swap stack pieces (chi/templ/HTMX/pgx/fx) without updating `tech-stack.md` first.
- Never write under `context/archive/`. Use `context/changes/` for new change folders.
- Keep secrets out of the repo — Railway variables and a local `.env` (gitignored) are the only sources.

## Project Structure

Current tree is flat (`go.mod`, `context/`, `idea-notes.md`, `.github/skills/`). When adding code, follow Go community layout:

- `cmd/server/main.go` — entrypoint; `fx.New(...).Run()` plus the top-level `fx.Module` list.
- `internal/` — handlers, recommender, storage (pgx), templ components. Each subpackage exports an `fx.Module` bundling its `fx.Provide` + lifecycle hooks.
- `templates/` — `.templ` files compiled via `templ generate`.
- `static/` — `htmx.min.js` and assets, served via `http.FileServer`.
- `migrations/` — SQL schema, applied at deploy.

## Build, Test, and Development

No scripts wired yet. Use these directly:

- `go build ./...` — compile.
- `go test ./...` — run tests.
- `go vet ./...`, `golangci-lint run`, `govulncheck ./...` — static + lint + vuln check (CI gate).
- `golangci-lint fmt` — apply `gofmt` + `goimports` per @.golangci.yml.
- `templ generate` — regenerate templ Go after editing `.templ` files.

## Coding Style & Naming

- Lint + format via `golangci-lint run` and `golangci-lint fmt`. Linter set + formatters: see @.golangci.yml.
- Wrap errors with `fmt.Errorf("…: %w", err)`; surface via `errors.Is`/`errors.As` (enforced by `errorlint`).
- HTTP handlers: `func (h *Handler) HandleSessionStart(w, r)` shape; mount under chi sub-routers per feature.
- DI: depend on interfaces in constructors; export `Module = fx.Module("name", fx.Provide(...))` per package. Resource lifecycle (pgx pool, http server) goes through `fx.Lifecycle.Append` — never in `init()` or package-level `var`.

## Testing

Co-locate `*_test.go` with the package. Std `testing` + table-driven tests; cover the recommender (grade + hold-type balance) densely. Single test: `go test ./internal/recommender -run TestPickNext`.

## Commits & PRs

No history yet. Use Conventional Commits (`feat:`, `fix:`, `chore:`, `docs:`) and reference FR IDs in the body (e.g. `feat(session): adaptive next-problem pick (FR-008, FR-012)`). PRs target `main` at https://github.com/sunba23/moonphase.

<!-- BEGIN @przeprogramowani/10x-cli -->

## 10xDevs AI Toolkit - Module 3, Lesson 4 (E2E Tests)

**For E2E tests, use the `/10x-e2e` skill.** It is the single source of truth
for the workflow — risk → seed test + rules → generate → review against the five
anti-patterns → re-prompt → verify. The skill's `references/` carry the full
rules, anti-patterns, seed pattern, and prompt-template.

A few hard rules that hold even before you invoke the skill:

- **Locators:** `getByRole` / `getByLabel` / `getByText` first; `getByTestId`
  only when accessibility attributes are ambiguous. Never CSS selectors, XPath,
  or DOM structure.
- **Never `page.waitForTimeout()`.** Wait for state: `toBeVisible()`,
  `waitForURL()`, `waitForResponse()`.
- **Test independence + cleanup.** Each test runs standalone — its own setup,
  action, assertion, and cleanup; unique ids (timestamp suffix) so parallel runs
  and re-runs don't collide.

Two boundaries to keep straight:

- **DOM (snapshot) is the default.** Vision (`--caps=vision`) is a supplement for
  visual-only risks (layout, z-index, animation); for pixel regression prefer
  deterministic tools (`toMatchSnapshot`, Argos, Lost Pixel). VLM model
  selection/cost is a debugging topic (Lesson 5), not testing.
- **Healer helps on selectors, harms on logic.** A changed selector → healer
  re-finds it (route through PR review). A changed business behavior → healer
  masks the bug; that failing-test-to-fix case is Lesson 5.

<!-- END @przeprogramowani/10x-cli -->

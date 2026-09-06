# Changelog

Notable changes, newest first. Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versioning: [SemVer](https://semver.org/spec/v2.0.0.html).

## [1.0.0] - 2026-09-06

### Added

- `rec_pick` decision log: one structured line per recommender pick, keyed by
  session id, capturing the inputs and the chosen problem.
- GitHub Actions CI (build, templ check, vet, lint, format, `govulncheck`,
  tests) on PRs and pushes to `main` / `release/*`; non-blocking recommender
  benchmarks. `main` branch-protected; Railway waits for CI before deploying.
- Release-branch flow: features land on `release/vX.Y.Z`, merged to `main` as
  one batch; `vX.Y.Z` tags mark releases.
- Page footer showing the running commit SHA (`dev` locally).
- Dependabot for Go modules, GitHub Actions, and npm.

### Changed

- Go 1.26.6; `golang.org/x/crypto` v0.56.0, `github.com/moby/go-archive` v0.3.0
  (clears `govulncheck`).

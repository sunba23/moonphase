# Changelog

All notable changes to this project are recorded in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- GitHub Actions CI workflow (`.github/workflows/ci.yml`): build, templ
  staleness check, `go vet`, `golangci-lint`, format check, `govulncheck`, and
  the full test suite. A non-blocking `bench` job runs the recommender
  benchmarks.
- `main` is branch-protected: the `ci` check must pass before a pull request
  merges, and Railway holds each production deploy until `ci` passes
  ("Wait for CI").
- Page footer that shows the running commit SHA, linked to the GitHub commit
  (`dev` when run locally).
- `CHANGELOG.md` and a release procedure in `README.md`.

### Changed

- Go toolchain directive raised to 1.26.6; `golang.org/x/crypto` to v0.56.0 and
  `github.com/moby/go-archive` to v0.3.0, to clear `govulncheck` findings.

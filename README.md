# moonphase

Adaptive MoonBoard training coach. During a session the app picks the next problem from per-problem RPE and completion feedback.

**User guide:** https://sunba23.github.io/moonphase/ (source in `docs/`, served by GitHub Pages from `main` / `/docs`).

## CI

Every pull request and every push to `main` or a `release/*` branch runs `.github/workflows/ci.yml`. The `ci` job builds, checks that the committed `*_templ.go` files are not stale, runs `go vet`, `golangci-lint run`, a `gofmt`/`goimports` diff check, `govulncheck`, and `go test ./...`. The test suite starts its own Postgres with testcontainers, so the runner needs Docker but no secrets. A second job, `bench`, runs the recommender benchmarks and never blocks a merge.

`ci` is a required check on `main`, so a red pull request cannot merge. Railway also holds each production deploy until the `main` run of `ci` passes.

End-to-end browser tests are not in CI yet. They need a live Supabase Auth project and are a follow-up change.

The deployed site shows the running commit's short SHA in the page footer, linked to the commit on GitHub. Run locally with no `RAILWAY_GIT_COMMIT_SHA` and the footer shows `dev`.

## Releasing

Work does not go straight to `main`. A feature branch is opened as a pull request into the current release branch, `release/vX.Y.Z`. That branch collects a batch of changes.

When the batch is ready, update `CHANGELOG.md` on the release branch: rename `## [Unreleased]` to `## [X.Y.Z] - YYYY-MM-DD` and add a fresh empty `## [Unreleased]` above it. Then open a pull request from `release/vX.Y.Z` into `main` and merge it once `ci` is green.

The merge to `main` is what deploys production. After it deploys, tag the merge commit on `main` with `git tag vX.Y.Z` and `git push origin vX.Y.Z`. The tag is a marker and a rollback point, not a deploy trigger.

To roll back, redeploy the previous tag's commit from the Railway dashboard.

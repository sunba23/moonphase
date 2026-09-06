# moonphase

Adaptive MoonBoard training coach. During a session the app picks the next
problem from per-problem RPE and completion feedback.

## CI

Every pull request and every push to `main` runs `.github/workflows/ci.yml`.

The `ci` job runs, in order:

1. `go build ./...`
2. `go install` of `templ`, then `templ generate` and `git diff --exit-code`
   (fails if a committed `*_templ.go` file is stale)
3. `go vet ./...`
4. `golangci-lint run` (golangci-lint v2, pinned to the version in the workflow)
5. `golangci-lint fmt --diff` (fails if `gofmt` or `goimports` would change code)
6. `govulncheck ./...`
7. `go test ./...` — the DB tests start their own Postgres with
   `testcontainers-go`, so the runner needs Docker but no secrets

A second job, `bench`, runs the recommender benchmarks with
`continue-on-error: true`. It never blocks a merge or a deploy.

`ci` is a required check on `main`: a red pull request cannot merge. Railway
also holds each production deploy in `WAITING` until the `main` run of `ci`
passes ("Wait for CI"), and marks it `SKIPPED` if `ci` fails.

End-to-end browser tests (`tests/e2e/`, Playwright) are **not** in CI yet. They
need a live Supabase Auth project; adding them is a follow-up change.

The deployed site shows the running commit's short SHA in the page footer,
linked to the commit on GitHub. Locally, with no `RAILWAY_GIT_COMMIT_SHA` set,
the footer shows `dev`.

## Releasing

A release is a marker and a rollback point. The merge to `main` is what
deploys; the tag does not trigger anything.

1. In `CHANGELOG.md`, rename `## [Unreleased]` to `## [X.Y.Z] - YYYY-MM-DD` and
   add a fresh empty `## [Unreleased]` section above it.
2. Commit: `git commit -m "release: vX.Y.Z"`.
3. Open a pull request, wait for `ci` to pass, and merge it to `main`.
4. Tag the merge commit and push the tag:

   ```sh
   git checkout main && git pull
   git tag vX.Y.Z
   git push origin vX.Y.Z
   ```

To roll back, redeploy the commit of the previous `vX.Y.Z` tag from the Railway
dashboard.

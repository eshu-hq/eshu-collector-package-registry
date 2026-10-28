# Contributing

## Local gates

`make ci` runs the same lint, test, and race commands CI runs: `go vet`,
`golangci-lint run` plus the gofumpt format check, `go mod tidy -diff`,
the 500-line `.go` file cap, `go build`, the full test suite, and the
race suite. Run it before every push. It needs Go 1.26.10 or newer;
`go.mod` is the version of record and CI installs exactly that.

Shell scripts pass `shellcheck` (see the `shell` lane). Keep them
`set -euo pipefail` clean like `scripts/pin-digest.sh`.

## CI lanes

Job names are frozen because branch protection requires `ci-complete`:

| Lane | Runs when |
| --- | --- |
| `lint` | Go sources, `manifest.yaml`, module or lint-config changes, plus unclassified files |
| `test` | Above, plus fixtures and config the tests read, plus unclassified files |
| `race` | Go sources, module, lint-config, or workflow changes, plus unclassified files, plus a coverage artifact and floor check |
| `boundary` | Go sources or workflow changes (standalone import boundary) |
| `shell` | Shell script changes |
| `image` | Go sources (the Dockerfile builds the binary from them), `Dockerfile`, `.dockerignore`, `manifest.yaml`, or module changes (build only, no push) |
| `govulncheck` | Dependency changes, pushes to `main`, weekly schedule, manual dispatch |
| `gosec` | Pushes to `main`, weekly schedule, and manual dispatch; advisory, SARIF to the Security tab |
| `ci-complete` | Always; fails on any needed lane failure, passes on legitimate skips |

Docs-only PRs (`README.md`, `LICENSE`) run no lane and stay mergeable
through `ci-complete`. Anything unclassified (for example `Makefile`,
`CODEOWNERS`, `.gitignore`) runs `lint`, `test`, and `race`.
`scripts/record-grants.sh` runs `shell` plus `test` because the grant
test opens it; `.github/dependabot.yml` runs the workflow lanes.
A manual `workflow_dispatch` run records every lane; the weekly schedule
runs `govulncheck` and `gosec` only.

## Coverage floor

The `race` job fails below `COVERAGE_FLOOR` in `ci.yml`. The floor starts
at the measured post-cleanup baseline and may only rise toward the 90%
target as tests land. Never lower it, exclude packages silently, or
delete assertions to move the number.

## Fixture-only CI

PR lanes never touch a live registry: tests run against inline fixtures.
`manifest.yaml` is embedded into the binary, so editing it runs the full
Go lanes. `grant.example.json` is read by the grant tests, so editing it
runs `test` only. `scripts/record-grants.sh` is opened by the grant test,
so editing it runs `shell` plus `test`. Base images stay digest-pinned;
re-pin with the `scripts/pin-digest.sh` flow and keep the Go base in sync
with `go.mod`.

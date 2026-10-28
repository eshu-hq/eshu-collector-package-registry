# Epic #7 closeout evidence (re-proved 2026-10-28)

Epic: `eshu-hq/eshu-collector-package-registry#7` (open, type:epic).
Worktree branch `drive/7-ci-gates-closeout` at `2d9a5e1` (== `origin/main`).
Toolchain: Go 1.26.10 (per `go.mod`; CI `setup-go` reads `go-version-file`).
Local runs below use a clean `PATH` with Go 1.26.10 and no `GOROOT` override.

## AC re-proof (current main)

| # | AC | Result |
|---|---|---|
| 1 | Seeded vet/lint/race/format/tidy/501-line red-then-green | PASS, exact exits below |
| 2 | Docs-only PR runs no lanes, `ci-complete` ok, mergeable | PASS (`changes`/`if` + proof PR #9) |
| 3 | Shell-only runs `shell` only; fixture-only runs `test` only | PASS (proof PRs #10/#11, plus #14/#15) |
| 4 | Go PR uploads coverage artifact, summary shows per-package, below-floor fails; floor 60.4 rising to 90 via #8 | PASS (run 40655058087 artifact + local floor trip) |
| 5 | Seeded 501-line file fails `lint` cap | PASS (exit 1 seeded / 0 clean) |
| 6 | `golangci-lint run ./...` zero findings, no exclusions | PASS (`0 issues.`) |
| 7 | Ruleset shows `ci-complete` required on `main` | PASS (ruleset 26007780, active) |
| 8 | Fork PR lanes need no secrets, read-only token | PASS (`rg secrets` none; perms/timeouts/concurrency) |
| 9 | Lint/test/race wall times recorded; all far below single-digit-minute budget | PASS (1.1s / 0.36s / 2.9s) |

## AC1/AC5 seeded red-then-green (exact exits, tree clean after each)

| Seed | Red | Green |
|---|---|---|
| `fmt.Sprintf("%d", "not-a-number")` | `go vet` exit 1, `wrong type string` | exit 0, empty |
| unused `zzSeedUnusedHelper` | `golangci-lint run` exit 1, `(unused)` | exit 0, `0 issues.` |
| goroutine write + unsync read | `go test -race` exit 1, `WARNING: DATA RACE` | exit 0, both packages ok |
| `func zzSeedFmt()  { ... }` (double space) | `fmt --diff` exit 1, diff shown | exit 0, empty |
| `go mod edit -droprequire=gopkg.in/yaml.v3` | `go mod tidy -diff` exit 1, `diff current/go.mod tidy/go.mod` | exit 0 (restored via `git checkout`) |
| 603-line `zz_seed_501.go` (`git add -N`) | cap `test -z` exit 1, file listed | exit 0, empty |
| Floor math on measured profile | floor 99 exit 1 (fails) | floor 60.4 exit 0 (passes) |

`git status --short` empty after every seed (throwaway files removed, `go.mod`/`go.sum` restored).

## AC2/AC3 lane lists

`changes` job classifies: go (`**/*.go`, `go.mod`, `go.sum`, `.golangci.yml`,
`manifest.yaml`, `testdata/**`), fixtures (`testdata/**`, `**/*.json`,
`**/*.yaml|yml`, `*.example.*`, `scripts/record-grants.sh`), shell
(`**/*.sh`), deps (`go.mod`/`go.sum`), image (`Dockerfile`, `.dockerignore`,
`manifest.yaml`, `go.mod`, `go.sum`), workflows, `misc` catch-all (`**` minus
every classified pattern, `some-with-excludes`). Docs (`README.md`, `LICENSE`)
match nothing, so no lane runs and `ci-complete` succeeds.

Proof PRs (closed unmerged, per merged PR #12): #9 docs-only (no lanes),
#10 shell-only (`shell` only), #11 fixture-only (`test` only, no lint/format/
race), #14 Makefile-only (`lint`+`test`+`race` via `misc`), #15
record-grants-only (`shell`+`test`).

## AC4 coverage

Measured: 1154 covered / 1908 statements = 60.5% (`go tool cover -func`;
`COVERAGE_FLOOR: 60.4` in `ci.yml`). Gap to 90%: 1718-1154 = 564, tracked in
open issue #8 — the floor may only rise. Main push run 40655058087
(head `2d9a5e1`): all 10 jobs success including `ci-complete`;
`coverage-profile` artifact present (15097B, 14-day retention, not expired).

## AC6 lint set

`.golangci.yml` (v2.12.2): govet, staticcheck, ineffassign, unused, errcheck;
formatter gofumpt; `max-issues-per-linter: 0` (uncapped census). Zero
exclusions. `actionlint` clean on both workflows.

## AC7 ruleset

Ruleset 26007780 `require-ci-complete-on-main` (target branch, active):
`refs/heads/main`, `required_status_checks: [{context: ci-complete}]`,
strict, no bypass actors. Sibling ruleset 25780044 `main protection`.

## AC8 fork safety

- `rg 'secrets\.' .github/workflows/ci.yml`: no matches.
- Top-level `permissions: {}` (default deny); every lane grants only what it
  needs (`contents: read` everywhere; `pull-requests: read` for `changes`;
  `issues: write` for govulncheck weekly filing; `security-events: write` for
  gosec SARIF). `ci-complete` runs on `{}`.
- Every job sets `timeout-minutes` (5 changes/shell/ci-complete, 10 boundary/
  govulncheck, 15 lint/test, 20 image, 30 race/gosec).
- `concurrency: group ci-<PR-or-ref>, cancel-in-progress` per PR.
- All `uses:` SHA-pinned with version comments (checkout v7.0.1, setup-go
  v7.0.0, upload-artifact v7.0.2, codeql upload-sarif v4.39.1, paths-filter
  v4.0.3).

## Image pins

`Dockerfile`: `golang:1.26.10-alpine@sha256:a2012b…` and
`distroless/static-debian12:nonroot@sha256:afa5c8…` — every `FROM` pinned.
`manifest.yaml` image ref carries `@sha256:<64 hex>` (all-zero placeholder by
design: the digest is stamped at seed time, see `scripts/pin-digest.sh`).
`go.mod` 1.26.10 == Dockerfile Go base 1.26.10. `.dockerignore` present.

## AC9 wall times (observed, cache-warm, Go 1.26.10)

- `go vet ./...`: real 0m0.351s, exit 0
- `golangci-lint run ./...` (Makefile `go run …@v2.12.2`): real 0m1.143s, `0 issues.`
- `go test ./... -count=1`: real 0m0.356s, both packages ok
- `go test -race -covermode=atomic`: real 0m2.947s, 60.4%/62.4% per package
- format check 0.73s, tidy 0.02s, cap 0.005s, shellcheck 0.02s, actionlint 0
- `govulncheck -scan package` (v1.9.0, Go 1.26.10): `No vulnerabilities found.`
  (A local go1.27.2-stdlib scan flags GO-2026-6809; that toolchain is not the
  repo pin — CI installs 1.26.10 per `go-version-file`.)

## Reusable move to eshu-hq/.github (issue #21)

Repo `eshu-hq/.github` exists (public): `go-collector-ci.yml` v2
(peeled SHA `4d01ab90baf5dd77c21cc1ee6bdf701332613449`) with frozen inner jobs
`lint test race shell image boundary govulncheck gosec actionlint ci-complete`;
established caller pattern in sibling collectors (e.g. tempo `ci.yml` nests
checks as `go-ci / <lane>`). Filed issue #21 (`enhancement`, `Part of #7`
with goal ledger): move all lane bodies upstream, keep this repo's `changes`
filters, pass `coverage-floor: 60.4` + `gosec-version: v2.30.0` (reusable
default is v2.27.1), keep caller-side findings-issue + SARIF jobs, then update
the ruleset required context to `go-ci / ci-complete` (owner act).

## Review threads

- PR #12 (merged): 1 review thread, resolved; 2 `COMMENTED` reviews (self-review + fix confirmation).
- PR #19 (merged Dependabot bump): no threads, 1 `COMMENTED` review with triage note.
- PR #20 (merged image-guard harden): no threads, 1 `COMMENTED` review.
- No unresolved threads on #12/#19/#20.

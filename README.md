# eshu-collector-package-registry

Status: pre-release. Do not rely on it in production yet.

This collector reads package details (names, versions, dependencies) from one package feed and hands Eshu a checked summary of what it found.

Eshu is the core system. A collector is a small helper program: it reads one outside system, turns what it finds into facts (small, typed records), and gives those facts back to Eshu. The collector runs through the Eshu component-extension host with the `process` runner. The host writes one bounded `collector-sdk/v1alpha1` request to stdin and reads one result from stdout. The collector gets no Postgres, graph, queue, lease, or workflow handle, and it never imports `github.com/eshu-hq/eshu/go/internal/...` (`boundary_test.go` enforces this). The host validates the result and owns claims and commits.

## What it collects

For the package-registry scope named by the claim it fetches one bounded
metadata document (20 MiB cap) over HTTPS and parses it with the registered
ecosystem parser (npm, PyPI, Go module proxy, Maven, NuGet, generic JSON, or
the Artifactory package wrapper):

- package identity, versions, dependencies, artifacts, and source/homepage/SCM
  hints from the native metadata;
- advisories, registry events, repository topology, and collection warnings
  from generic documents.

It emits the nine core `package_registry.*` kinds at schema `1.0.0`, all with confidence `reported` (the registry said so; Eshu did not check the package bytes itself):

| Fact kind | One fact describes |
| --- | --- |
| `package_registry.package` | one package |
| `package_registry.package_version` | one published version |
| `package_registry.package_dependency` | one dependency a version declares |
| `package_registry.package_artifact` | one downloadable file for a version |
| `package_registry.source_hint` | a source, homepage, or code-hosting hint |
| `package_registry.vulnerability_hint` | one advisory hint from a generic document |
| `package_registry.registry_event` | one registry event from a generic document |
| `package_registry.repository_hosting` | repository hosting details from a generic document |
| `package_registry.warning` | something the read had to skip or report |

URLs are sanitized fail-closed before emission: userinfo is dropped, sensitive query keys are stripped, and a value that cannot be taken apart is replaced.

Result states: `complete`; `partial` when a derived scope 404s (the
`registry_not_found` warning) or the document parses as malformed; `retryable`
for rate limits, transport errors, and 5xx; `terminal` for auth denials, bad
configuration, invalid evidence, and exhausted budgets. A failed claim emits no
facts. Duplicate detection rides on `generation.freshness_hint`, a digest of
every emitted fact.

## How data flows

```mermaid
flowchart LR
  A["Package registry<br/>one metadata document"] --> B["This collector"] --> C["Eshu host process"] --> D["Eshu core"]
```

Text version: package registry → this collector → Eshu host process (checks and stores the result) → Eshu core. The collector never talks to Eshu's database directly.

## When to use it, and when not to

Use it when Eshu needs current package, version, and dependency details from a feed it can read with one bounded HTTPS document per claim, refreshed on a schedule Eshu controls. Do not use it to publish packages, to install or run package code, or to prove a package is safe — vulnerability facts here are hints from the document, not a scan. It reads metadata only.

## How Eshu runs it

Eshu starts this program for one claim at a time (`collector --sdk-stdio`): one request on stdin, one result on stdout, as described at the top. Stdin means standard input; stdout means standard output. The image that carries the binary — and how it installs itself into a component folder — is covered under "Image and seeding" below.

## Configuration

`config.example.yaml` is an activation config. The `config` map holds `targets` (one entry per feed), an optional `derive_from_owned_packages` block (npm/PyPI scope-derived targets, used when no explicit target matches), and an optional `limits` block. Unknown keys fail closed.

| Target key | Meaning |
| --- | --- |
| `provider`, `ecosystem` | Feed flavor and package ecosystem (for example `npm` and `npm`) |
| `registry`, `scope_id` | Registry address and the stable feed identity recorded on every fact |
| `packages` | Package names to read for this feed |
| `package_limit`, `version_limit` | Most packages and versions to keep from the document |
| `visibility` | `unknown` (default), `public`, or `private`; any other value fails terminal with `invalid_config` |
| `metadata_url`, `document_format` | The exact document to fetch, and how to parse it (for example `native`) |
| `bearer_token_env`, `username_env`, `password_env` | Names of environment variables that hold the login; leave unset for a public feed |

Secrets never go in the config file: `*_env` entries name environment variables the worker exports. Secret values never appear in facts, source URIs, logs, or statuses, and config rejections name keys without quoting what was configured.

A target with a large metadata window returns more than the host's default
4 MiB stdout cap. Raise `process.stdout_limit_bytes` and
`limits.maxResultBytes` together; the collector fails terminal with
`result_budget_exceeded` rather than letting the host drop an oversized
result.

## Claim time budget

The host sets a claim's deadline to the **initial lease expiry** and kills the process there. Heartbeats do not extend it. The collector stops starting provider calls at the deadline minus `min(2s, 5% of the time left)` and returns a `retryable` result with failure class `claim_deadline_exceeded` and no facts. A missing deadline leaves only `limits.claimTimeoutSeconds` (default 300) as the bound.

## Eshu dependencies (pins)

Exact versions from `go.mod` — the published SDK pair from
[SDK compatibility](https://github.com/eshu-hq/eshu/blob/main/docs/public/extend/sdk-compatibility.md).
Do not substitute another SDK line.

| Module | Version |
| --- | --- |
| Go | `1.26.10` |
| `github.com/eshu-hq/eshu-sdk/collector` | `v0.2.0-rc.1` |
| `github.com/eshu-hq/eshu-sdk/factschema` | `v0.1.0-rc.5` |
| `gopkg.in/yaml.v3` | `v3.0.1` |

No `replace` directives and no `go.work`. Verify with
`GOPROXY=proxy.golang.org go list -m <module>@<version>` for each pin.
The component ID is `dev.eshu.collector.package_registry`, version `0.1.0`. It declares support for Eshu core `>=0.0.5 <0.2.0`, speaks `collector-sdk/v1alpha1` over the process adapter, and uses collector kind `package_registry` (see `manifest.yaml`).

## Operator signals

Today the extension host passes one thing from a claim's result to the work
item: the **failure class** (`registry_rate_limited`, `invalid_config`,
`claim_deadline_exceeded`, ...). The collector writes one JSON line per claim
to stderr for local runs and tests; the host's process runner drops the text.

The in-tree collector emitted fetch spans and counters. This collector has no
equivalent and declares no `telemetry.metricsPrefix` (eshu-hq/eshu#9032): a
per-claim process whose stderr the host drops, reporting over an SDK result
with no metrics field, has no channel to emit metrics under a declared prefix.

## Image and seeding

The image holds one static binary that installs itself, so an init container
needs no shell:

```bash
collector seed --dest /component [--image-ref ghcr.io/eshu-hq/eshu-collector-package-registry@sha256:<digest>]
```

writes `/component/collector` (mode 0755) and `/component/manifest.yaml`.
`--image-ref REF` (or the `COLLECTOR_IMAGE_REF` environment variable; the flag
wins) stamps the deployed digest-pinned reference into the manifest's artifact
entry, because an image cannot embed its own digest. The repository manifest
keeps the all-zero placeholder digest permanently (`manifest_test.go` fails on
a real one), because it is embedded in the binary and a committed digest could
only describe an older build. `scripts/pin-digest.sh <repo:tag>` only prints
the `repo@sha256:...` reference of a pushed image; it edits nothing.

## Keeping the same identity when you switch

Replacing the in-tree package-registry collector with this one must not re-key
a single fact. These activation values have to equal what the in-tree
collector uses for the same feed (including the instance ID — note this differs from some sibling collectors, so do not guess: read the live values first):

| Activation value | Must equal the in-tree value | If it differs |
| --- | --- | --- |
| `host.scope.id` | the in-tree target's `scope_id` | every stable key and `scope_id` changes: the feed is ingested as a new scope |
| `host.scope.kind` | `package_registry` | the scope no longer matches the in-tree scope kind |
| `host.sourceSystem` | the in-tree source system, `package_registry` | `source_ref` changes on every fact |
| activation instance id | the in-tree collector instance id | `collector_instance_id` in payloads changes |

The collector copies `scope_id`, `collector_instance_id`, and `source_system`
from the claim the host sends; it has no setting of its own for them.

Before enabling an activation, read the values from the live in-tree
collector instance configuration and write them into the activation. Do not
copy the placeholders in `config.example.yaml`. Run only one of the two
collectors per feed: with the same scope and instance id they would write the
same stable keys.

## Producer grants

All nine kinds are core-owned, so the host refuses them without a live producer
grant. `grant.example.json` lists the nine requests. Each grant is bound to
this component id and version and scoped to the manifest's collector kind
(`package_registry`), not to a feed: core matches a grant's scope against the
collector kinds the manifest declares. `scripts/record-grants.sh` records them
with `eshu component grant` in a component home. Core issues grants; this
repository only ships the request.

## What it never does

- It never publishes, installs, or runs packages, and it never stores anything itself — the host validates and commits.
- It never imports Eshu core internals (`go/internal/...`) or another collector; `boundary_test.go` fails the build if that changes. Beyond the SDK it uses only the standard library and YAML parsing.
- It never puts secrets or unsanitized addresses into facts, source URIs, logs, or statuses. An address that cannot be taken apart safely is replaced, never passed through.

## Build and test

```bash
go build ./...
go test ./... -count=1
go test -race ./... -count=1
make ci   # vet + lint + format + tidy + 500-line cap + build + test + race
```

`make ci` is the full local check CI runs (`.github/workflows/ci.yml`, reported as `go-ci / ci-complete`); see `CONTRIBUTING.md` for the lane contract. It needs Go 1.26.10 or newer; `go.mod` is the version of record.

## Releasing

`.github/workflows/release.yml` runs on every push to `main` and on every
pushed tag `vMAJOR.MINOR.PATCH[-pre]`. Nothing runs on a pull request, and
the workflow deploys nothing.

A push to `main` publishes the moving `main`, `latest`, and
`sha-<full-sha>` tags with the same signature and attestation hardening as a
version release. Use a moving tag only where floating is intended; releases
stay pinned to digests.

The workflow refuses to publish a version release unless the tag points at
the protected `main` head, so a `v*` tag cut from any other commit publishes
nothing. As defense in depth, restrict who can create `v*` tags with a tag
ruleset limited to maintainers.

1. Set `metadata.version` in `manifest.yaml` to the version you are releasing
   and merge that change to `main`. Then tag the merge commit and push the tag.
   The workflow refuses to publish unless the tag equals `v` plus that version
   (a `-pre-release` suffix on the tag is allowed when the manifest version has
   none), and it checks this before it builds or pushes anything.
2. The workflow builds `linux/amd64` (the only platform the manifest declares),
   pushes `ghcr.io/eshu-hq/eshu-collector-package-registry:<tag>` (or `:main`,
   `:latest`, and `:sha-<full-sha>` for a `main` push), writes the
   digest to the job summary, signs the digest with cosign (keyless), and
   attaches a SLSA build-provenance attestation and an SPDX SBOM attestation.
   It needs no secret beyond the job's `GITHUB_TOKEN`.
3. Copy the `ghcr.io/eshu-hq/eshu-collector-package-registry@sha256:<digest>`
   reference from the job summary into the activation's `seed.image`. The
   deployment stamps it into the manifest with `collector seed --image-ref`.

## Docs and related projects

- `config.example.yaml` — full setup example with comments; `manifest.yaml` — component identity, fact list, and core range; `grant.example.json` — the nine grant requests.
- [Eshu core](https://github.com/eshu-hq/eshu) — the host that runs this collector, including the [SDK compatibility](https://github.com/eshu-hq/eshu/blob/main/docs/public/extend/sdk-compatibility.md) page the pins come from.
- [eshu-sdk](https://github.com/eshu-hq/eshu-sdk) — the SDK modules pinned above. [eshu-deploy](https://github.com/eshu-hq/eshu-deploy) — deployment examples and reference setups.

## Contributing

See `CONTRIBUTING.md`. In short: run `make ci` before every push. Shell scripts must stay `shellcheck`-clean (`set -euo pipefail`, like `scripts/pin-digest.sh`).

## License

MIT. See `LICENSE`.

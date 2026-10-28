# eshu-collector-package-registry

Eshu package-registry collector, extracted from the Eshu core repository.

**Pre-release. Holds no producer authority until its reviewed cutover
(eshu-hq/eshu#8862).**

The collector runs through the Eshu component-extension host with the
`process` runner. The host writes one bounded `collector-sdk/v1alpha1` request
to stdin and reads one result from stdout. The collector gets no Postgres,
graph, queue, lease, or workflow handle, and it never imports
`github.com/eshu-hq/eshu/go/internal/...` (`boundary_test.go` enforces this).
The host validates the result and owns claims and commits.

## What it collects

For the package-registry scope named by the claim it fetches one bounded
metadata document (20 MiB cap) over HTTPS and parses it with the registered
ecosystem parser (npm, PyPI, Go module proxy, Maven, NuGet, generic JSON, or
the Artifactory package wrapper):

- package identity, versions, dependencies, artifacts, and source/homepage/SCM
  hints from the native metadata;
- advisories, registry events, repository topology, and collection warnings
  from generic documents.

It emits the nine core `package_registry.*` kinds at schema `1.0.0`:
`package`, `package_version`, `package_dependency`, `package_artifact`,
`source_hint`, `vulnerability_hint`, `registry_event`, `repository_hosting`,
and `warning`. URLs are sanitized fail-closed before emission: userinfo is
dropped, sensitive query keys are stripped, and a value that cannot be taken
apart is replaced.

Result states: `complete`; `partial` when a derived scope 404s (the
`registry_not_found` warning) or the document parses as malformed; `retryable`
for rate limits, transport errors, and 5xx; `terminal` for auth denials, bad
configuration, invalid evidence, and exhausted budgets. A failed claim emits no
facts. Duplicate detection rides on `generation.freshness_hint`, a digest of
every emitted fact.

## Configuration

`config.example.yaml` is an activation config. The `config` map holds
`targets` (one entry per feed: `provider`, `ecosystem`, `registry`,
`scope_id`, `packages`, limits, `visibility`, `metadata_url`,
`document_format`, and `*_env` credential names), an optional
`derive_from_owned_packages` block (npm/PyPI scope-derived targets), and an
optional `limits` block. Unknown keys fail closed. `visibility` accepts
`unknown`, `public`, or `private` (default `unknown`); any other value fails
the claim terminal with `invalid_config`. `*_env` entries name
environment variables the worker exports. Secret values never appear in facts,
source URIs, logs, or statuses, and config rejections name keys without
quoting what was configured.

A target with a large metadata window returns more than the host's default
4 MiB stdout cap. Raise `process.stdout_limit_bytes` and
`limits.maxResultBytes` together; the collector fails terminal with
`result_budget_exceeded` rather than letting the host drop an oversized
result.

## Claim time budget

The host sets a claim's deadline to the **initial lease expiry** and kills the
process there. Heartbeats do not extend it. The collector stops starting
provider calls at the deadline minus `min(2s, 5% of the time left)` and returns
a `retryable` result with failure class `claim_deadline_exceeded` and no
facts. A missing deadline leaves only `limits.claimTimeoutSeconds` (default
300) as the bound.

## Pins

Exact published SDK pair from
[SDK compatibility](https://github.com/eshu-hq/eshu/blob/main/docs/public/extend/sdk-compatibility.md):

- `github.com/eshu-hq/eshu-sdk/collector@v0.1.0-rc.3`
- `github.com/eshu-hq/eshu-sdk/factschema@v0.1.0-rc.5`

No `replace` directives and no `go.work`. Verify with
`GOPROXY=proxy.golang.org go list -m <module>@<version>` for each pin.

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

## Cutover identity

Replacing the in-tree package-registry collector with this one must not re-key
a single fact. These activation values have to equal what the in-tree
collector uses for the same feed:

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

## Releasing

`.github/workflows/release.yml` runs only when a tag `vMAJOR.MINOR.PATCH[-pre]`
is pushed. Nothing runs on a pull request or a branch push, and the workflow
deploys nothing.

The workflow refuses to publish unless the tag points at the protected
`main` head, so a `v*` tag cut from any other commit publishes nothing. As
defense in depth, restrict who can create `v*` tags with a tag ruleset
limited to maintainers.

1. Set `metadata.version` in `manifest.yaml` to the version you are releasing
   and merge that change to `main`. Then tag the merge commit and push the tag.
   The workflow refuses to publish unless the tag equals `v` plus that version
   (a `-pre-release` suffix on the tag is allowed when the manifest version has
   none), and it checks this before it builds or pushes anything.
2. The workflow builds `linux/amd64` (the only platform the manifest declares),
   pushes `ghcr.io/eshu-hq/eshu-collector-package-registry:<tag>`, writes the
   digest to the job summary, signs the digest with cosign (keyless), and
   attaches a SLSA build-provenance attestation and an SPDX SBOM attestation.
   It needs no secret beyond the job's `GITHUB_TOKEN`.
3. Copy the `ghcr.io/eshu-hq/eshu-collector-package-registry@sha256:<digest>`
   reference from the job summary into the activation's `seed.image`. The
   deployment stamps it into the manifest with `collector seed --image-ref`.

See CONTRIBUTING.md for the CI lane contract.

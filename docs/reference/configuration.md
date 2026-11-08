# Configuration reference: package registry collector

Start from the copyable [example config](../../config.example.yaml); component identity lives in the [manifest](../../manifest.yaml). For the facts these settings produce, see [facts](facts.md).

`config.example.yaml` is an activation config you can copy. The
`config` map holds `targets` (one entry per feed), an optional
`derive_from_owned_packages` block (npm and PyPI scope-derived
targets, used when no explicit target matches), and an optional
`limits` block. Unknown keys fail closed.

## Target settings

| Target key | What it does |
| --- | --- |
| `provider`, `ecosystem` | Feed flavour and package ecosystem, for example `npm` and `npm` |
| `registry`, `scope_id` | Registry address, and the stable feed identity recorded on every fact |
| `packages` | Package names to read for this feed |
| `package_limit`, `version_limit` | Most packages and versions to keep from the document |
| `visibility` | `unknown` (the default), `public`, or `private`; anything else fails terminal with `invalid_config` |
| `metadata_url`, `document_format` | The exact document to fetch, and how to parse it, for example `native` |
| `bearer_token_env`, `username_env`, `password_env` | Names of environment variables that hold the login; leave them unset for a public feed |

The derived-targets block (`derive_from_owned_packages`) takes
`enabled`, `ecosystems`, `package_limit`, and `version_limit`. It only
kicks in for npm and PyPI when no explicit target matches the claim
scope, and derived targets are public by construction.

## Credentials

Secrets never go in the config file: the `*_env` entries name
environment variables the worker exports, and you leave all three
unset for a credential-free public feed. Secret values never appear in
facts, source URIs, logs, or statuses, and config rejections name keys
without quoting what was configured. So if a config is refused, you'll
know which key to look at, and nothing sensitive leaks in the telling.

## Claim time budget and sizes

The host sets a claim's deadline to the initial lease expiry and kills
the process there. Heartbeats don't extend it. The collector stops
starting provider calls at the deadline minus the smaller of 2 seconds
or 5% of the time left, and returns a `retryable` result with
`claim_deadline_exceeded` and no facts. With no deadline,
`limits.claimTimeoutSeconds` (default 300) is the only bound.

The other caps are `maxRecordsPerClaim`, `maxPayloadBytes` for one
fact, and `maxResultBytes` for the whole result. A target with a large
metadata window returns more than the host's default 4 MiB stdout cap,
so raise `process.stdout_limit_bytes` and `limits.maxResultBytes`
together. The collector fails terminal with `result_budget_exceeded`
rather than letting the host drop an oversized result.

## Keeping the same identity when you switch

| Activation value | Must equal the in-tree value | If it differs |
| --- | --- | --- |
| `host.scope.id` | the in-tree target's `scope_id` | every stable key and `scope_id` changes, and the feed is ingested as a new scope |
| `host.scope.kind` | `package_registry` | the scope no longer matches the in-tree scope kind |
| `host.sourceSystem` | the in-tree source system, `package_registry` | `source_ref` changes on every fact |
| activation instance id | the in-tree collector instance id | `collector_instance_id` in payloads changes |

Before you enable an activation, read the values from the live in-tree
collector instance configuration and write them into the activation.
Don't copy the placeholders in `config.example.yaml`. Run only one of
the two collectors per feed: with the same scope and instance ID they
would write the same stable keys.

## Pins

Exact versions from `go.mod`, matching the published SDK pair from
[SDK compatibility](https://github.com/eshu-hq/eshu/blob/main/docs/public/extend/sdk-compatibility.md).
Please don't substitute another SDK line. There are no `replace`
directives and no `go.work` here, and you can check a pin with
`GOPROXY=proxy.golang.org go list -m <module>@<version>`. This repo
builds against `eshu-sdk`; that's simply what its `go.mod` requires.

| Module | Version |
| --- | --- |
| Go | `1.26.10` |
| `github.com/eshu-hq/eshu-sdk/collector` | `v0.2.0-rc.1` |
| `github.com/eshu-hq/eshu-sdk/factschema` | `v0.1.0-rc.5` |
| `gopkg.in/yaml.v3` | `v3.0.1` |

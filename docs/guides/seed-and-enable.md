# Seed and enable

From a published image to a live activation, in order.

## Image and seeding

The image holds one static binary that installs itself, so an init
container needs no shell:

```bash
collector seed --dest /component [--image-ref ghcr.io/eshu-hq/eshu-collector-package-registry@sha256:<digest>]
```

That writes `/component/collector` (mode 0755) and
`/component/manifest.yaml`. `--image-ref REF` (or
`COLLECTOR_IMAGE_REF`; the flag wins) stamps the deployed
digest-pinned reference into the manifest's artifact entry, because an
image can't embed its own digest. The repo manifest keeps the all-zero
placeholder digest permanently, and `manifest_test.go` fails on a real
one, because the manifest is embedded in the binary and a committed
digest could only ever describe an older build.
`scripts/pin-digest.sh <repo:tag>` only prints the `repo@sha256:...`
reference of a pushed image; it edits nothing.

## Then record grants and enable

The host refuses this component's facts without a producer grant for
each kind. The nine requests are in
[grant.example.json](../../grant.example.json), tied to `dev.eshu.collector.package_registry`
and its manifest version. Record them before you enable (see
[concepts](../concepts/README.md) for why grants exist), enable one
activation per scope from your copy of
[config.example.yaml](../../config.example.yaml), then let Eshu run a
first claim and check it with [operate](operate.md).

If you're replacing Eshu's built-in collector, do the identity checks
in the [configuration reference](../reference/configuration.md) first.
Getting those wrong re-keys stored data, and that's much easier to
prevent than to undo.

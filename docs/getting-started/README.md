# Getting started

Status: pre-release. Please don't rely on this in production yet.

The honest first thing to know: you don't run this collector as a
daemon. Eshu's host starts it for one claim (one job) at a time, sends
one bounded request on stdin, and reads one result from stdout. So
"getting started" really means getting one activation enabled and one
claim to succeed.

1. Check your core version fits the range in the [manifest](../../manifest.yaml)
   (`>=0.0.5 <0.2.0`).
2. Copy [config.example.yaml](../../config.example.yaml). Add one target
   per feed (`provider`, `ecosystem`, `registry`, `scope_id`,
   `packages`). Leave the `*_env` credential names unset for a public
   feed. See the [configuration reference](../reference/configuration.md).
3. Record the nine producer grants from
   [grant.example.json](../../grant.example.json) with
   `scripts/record-grants.sh`.
4. Seed the image and enable the activation (see
   [seed and enable](../guides/seed-and-enable.md)). Heads-up if you are
   replacing the in-tree collector: here the activation values must
   equal the in-tree ones, including the instance ID. That's different
   from some sibling collectors, so read the live values first and run
   only one of the two per feed.
5. Let Eshu run a first claim. `complete`, or `partial` from a derived
   scope that's missing upstream, means it's working.

Where to go next: [concepts](../concepts/README.md) for the mental
model, the [configuration reference](../reference/configuration.md)
for every setting, [facts](../reference/facts.md) for what comes out,
and [troubleshoot](../guides/troubleshoot.md) when the first claim
doesn't land.

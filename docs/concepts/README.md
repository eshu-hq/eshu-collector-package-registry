# Concepts: how the package registry collector works

For the package-registry scope named by the claim, it fetches one
bounded metadata document (20 MiB cap) over HTTPS and parses it with
the registered ecosystem parser: npm, PyPI, Go module proxy, Maven,
NuGet, generic JSON, or the Artifactory package wrapper.

Out of that document come package identity, versions, dependencies,
artifacts, and source, homepage, and SCM hints from the native
metadata. Generic documents can also bring advisories, registry
events, repository topology, and collection warnings.

URLs are sanitised fail-closed before anything is emitted. Userinfo is
dropped, sensitive query keys are stripped, and a value that can't be
taken apart safely is replaced rather than passed through. If that
sounds strict, it is, and deliberately so.

## Nine kinds of fact

The full table lives in the [facts reference](../reference/facts.md) and mirrors `manifest.yaml`. If you change one, change the
other in the same commit; the manifest is the source of truth. Every
kind is at schema `1.0.0` with confidence `reported`, meaning the
registry said so and Eshu hasn't checked the package bytes itself.


The component is `dev.eshu.collector.package_registry`, version
`0.1.0`, collector kind `package_registry`. It declares Eshu core
`>=0.0.5 <0.2.0` and speaks `collector-sdk/v1alpha1` over the process
adapter.

## One claim, start to finish

The step-by-step flow is the
[claim lifecycle diagram](../diagrams/README.md#claim-lifecycle);
the wider picture is the
[system context](../diagrams/README.md#system-context).
Results come back as `complete`; as `partial` when a derived scope
404s (the `registry_not_found` warning) or the document parses as
malformed; as `retryable` for rate limits, transport errors, and 5xx;
and as `terminal` for auth denials, bad configuration, invalid
evidence, and exhausted budgets. A failed claim emits no facts.
Duplicate detection rides on `generation.freshness_hint`, a digest of
every emitted fact.

## Identity and grants

Replacing the in-tree package-registry collector shouldn't re-key a
single fact, so the activation values have to equal what the in-tree
collector uses for the same feed, including the instance ID. That's
different from some sibling collectors, so don't guess: read the live
values first. The full table sits in
[configuration](../reference/configuration.md). The collector itself copies
`scope_id`, `collector_instance_id`, and `source_system` from the
claim the host sends; it has no setting of its own for them.

All nine kinds are core-owned, so the host refuses them without a live
producer grant. `grant.example.json` lists the nine requests. Each
grant is bound to this component ID and version and scoped to the
manifest's collector kind (`package_registry`), not to a feed, because
core matches a grant's scope against the collector kinds the manifest
declares. `scripts/record-grants.sh` records them with
`eshu component grant` in a component home. Core issues grants; this
repo only ships the request.

It never publishes, installs, or runs packages, and it never stores
anything itself. It never imports Eshu core internals
(`go/internal/...`) or another collector, and `boundary_test.go` fails
the build if that changes. Beyond the SDK it uses only the standard
library and YAML parsing. Secrets and unsanitised addresses stay out
of facts, source URIs, logs, and statuses, always.

## Pictures

The diagrams for this page (system context and claim lifecycle) live
in one place: [diagrams](../diagrams/README.md#system-context) and
[the claim lifecycle](../diagrams/README.md#claim-lifecycle). Each
one has a short text version next to it.

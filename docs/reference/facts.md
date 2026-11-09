# Facts reference: package registry collector

This table mirrors [manifest.yaml](../../manifest.yaml). The manifest
is the source of truth, so if you change one you need to update the
other in the same change.

| Fact kind | Schema | Confidence | One fact is |
| --- | --- | --- | --- |
| `package_registry.package` | `1.0.0` | `reported` | one package |
| `package_registry.package_version` | `1.0.0` | `reported` | one published version |
| `package_registry.package_dependency` | `1.0.0` | `reported` | one dependency a version declares |
| `package_registry.package_artifact` | `1.0.0` | `reported` | one downloadable file for a version |
| `package_registry.source_hint` | `1.0.0` | `reported` | a source, homepage, or code-hosting hint |
| `package_registry.vulnerability_hint` | `1.0.0` | `reported` | one advisory hint from a generic document |
| `package_registry.registry_event` | `1.0.0` | `reported` | one registry event from a generic document |
| `package_registry.repository_hosting` | `1.0.0` | `reported` | repository hosting details from a generic document |
| `package_registry.warning` | `1.0.0` | `reported` | something the read had to skip or report |

The component is `dev.eshu.collector.package_registry`, version
`0.1.0`, collector kind `package_registry`. It declares Eshu core
`>=0.0.5 <0.2.0` and speaks `collector-sdk/v1alpha1` over the process
adapter.

Every kind above belongs to Eshu core, so the host won't accept it
without a producer grant. The nine requests live in
[grant.example.json](../../grant.example.json); see
[concepts](../concepts/README.md) for how grants and identity work,
and [seed and enable](../guides/seed-and-enable.md) for recording
them.

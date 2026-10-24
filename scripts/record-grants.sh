#!/usr/bin/env bash
# Records the nine core producer grants this collector needs, one per
# package_registry.* kind, in a component home. The grant scope is the
# manifest's collector kind ("package_registry"), not a feed: core binds a
# grant to the kinds the manifest declares. Core issues grants; run this only
# against a home you are authorized to grant in, after the grant request is
# approved.
#
#   COMPONENT_HOME=/path/to/home ESHU=eshu scripts/record-grants.sh [version] [lifetime]
set -euo pipefail
: "${COMPONENT_HOME:?COMPONENT_HOME is not set: no grants recorded}"
eshu="${ESHU:-eshu}"
version="${1:-0.1.0}"
lifetime="${2:-720h}"
for kind in \
  package_registry.package package_registry.package_version package_registry.package_dependency \
  package_registry.package_artifact package_registry.source_hint package_registry.vulnerability_hint \
  package_registry.registry_event package_registry.repository_hosting package_registry.warning; do
  "$eshu" component grant dev.eshu.collector.package_registry \
    --kind "$kind" --schema-version 1.0.0 --scope package_registry \
    --version "$version" --expires-in "$lifetime" \
    --component-home "$COMPONENT_HOME"
done

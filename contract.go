// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	sdk "github.com/eshu-hq/eshu-sdk/collector"
)

const (
	// ComponentID is the manifest identity of this collector. Keep it in sync
	// with metadata.id in manifest.yaml.
	ComponentID = "dev.eshu.collector.package_registry"
	// CollectorKind is the durable collector family name for package-registry
	// facts.
	CollectorKind = "package_registry"
	// SourceSystem is the source system name facts carry when the claim does
	// not name one. Hosted claims carry the activation's source system.
	SourceSystem = "package_registry"
	// MetricsPrefix is the component-owned metric prefix the manifest declares.
	// Nothing in this repository emits a metric under it yet; see the README
	// section "Operator signals".
	MetricsPrefix = "eshu_dp_package_registry_collector_"
	// SchemaVersion is the schema version of every emitted package_registry
	// fact.
	SchemaVersion = "1.0.0"
)

// Core package-registry fact kinds. They are core-owned: the host requires a
// producer grant for each before accepting them from this component.
const (
	// FactKindPackage identifies one normalized package identity.
	FactKindPackage = "package_registry.package"
	// FactKindPackageVersion identifies one observed package version.
	FactKindPackageVersion = "package_registry.package_version"
	// FactKindPackageDependency identifies one source-reported dependency.
	FactKindPackageDependency = "package_registry.package_dependency"
	// FactKindPackageArtifact identifies one source-reported artifact.
	FactKindPackageArtifact = "package_registry.package_artifact"
	// FactKindSourceHint identifies one source/homepage/SCM hint.
	FactKindSourceHint = "package_registry.source_hint"
	// FactKindVulnerabilityHint identifies one registry-reported advisory.
	FactKindVulnerabilityHint = "package_registry.vulnerability_hint"
	// FactKindRegistryEvent identifies one source-reported registry event.
	FactKindRegistryEvent = "package_registry.registry_event"
	// FactKindRepositoryHosting identifies one provider topology record.
	FactKindRepositoryHosting = "package_registry.repository_hosting"
	// FactKindWarning identifies one non-fatal collection warning.
	FactKindWarning = "package_registry.warning"
)

// FactKinds returns every fact kind this collector may emit, in the order the
// manifest declares them.
func FactKinds() []string {
	return []string{
		FactKindPackage,
		FactKindPackageVersion,
		FactKindPackageDependency,
		FactKindPackageArtifact,
		FactKindSourceHint,
		FactKindVulnerabilityHint,
		FactKindRegistryEvent,
		FactKindRepositoryHosting,
		FactKindWarning,
	}
}

// Contract returns the SDK fact families this collector may emit.
func Contract() sdk.Contract {
	kinds := FactKinds()
	facts := make([]sdk.FactDeclaration, 0, len(kinds))
	for _, kind := range kinds {
		facts = append(facts, sdk.FactDeclaration{
			Kind:             kind,
			SchemaVersions:   []string{SchemaVersion},
			SourceConfidence: []sdk.SourceConfidence{sdk.SourceConfidenceReported},
		})
	}
	return sdk.Contract{ProtocolVersion: sdk.ProtocolVersionV1Alpha1, Facts: facts}
}

// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"strings"

	sdk "github.com/eshu-hq/eshu-sdk/collector"
	stableid "github.com/eshu-hq/eshu-sdk/factschema/identity"
)

// NewPackageDependencyEnvelope builds the durable dependency fact for one
// source-reported package version dependency.
func NewPackageDependencyEnvelope(observation PackageDependencyObservation, sourceSystem string) (sdk.Fact, error) {
	pkg, version, versionID, err := packageVersionID(observation.Package, observation.Version, "package dependency")
	if err != nil {
		return sdk.Fact{}, err
	}
	dependency, err := NormalizePackageIdentity(observation.Dependency)
	if err != nil {
		return sdk.Fact{}, err
	}
	if err := validateObservationBoundary(
		observation.ScopeID,
		observation.GenerationID,
		observation.CollectorInstanceID,
		"package dependency observation",
	); err != nil {
		return sdk.Fact{}, err
	}

	dependencyType := strings.TrimSpace(observation.DependencyType)
	stableFactKey := stableid.StableID(FactKindPackageDependency, map[string]any{
		"dependency_package_id": dependency.PackageID,
		"dependency_range":      strings.TrimSpace(observation.Range),
		"dependency_type":       dependencyType,
		"excluded":              observation.Excluded,
		"marker":                strings.TrimSpace(observation.Marker),
		"optional":              observation.Optional,
		"target_framework":      strings.TrimSpace(observation.TargetFramework),
		"version_id":            versionID,
	})
	payload := map[string]any{
		"collector_instance_id":  observation.CollectorInstanceID,
		"ecosystem":              string(pkg.Ecosystem),
		"registry":               pkg.Registry,
		"package_id":             pkg.PackageID,
		"version_id":             versionID,
		"version":                version,
		"dependency_package_id":  dependency.PackageID,
		"dependency_ecosystem":   string(dependency.Ecosystem),
		"dependency_registry":    dependency.Registry,
		"dependency_namespace":   dependency.Namespace,
		"dependency_normalized":  dependency.NormalizedName,
		"dependency_purl":        dependency.PURL,
		"dependency_bom_ref":     dependency.BOMRef,
		"dependency_manager":     dependency.PackageManager,
		"dependency_range":       strings.TrimSpace(observation.Range),
		"dependency_type":        dependencyType,
		"target_framework":       strings.TrimSpace(observation.TargetFramework),
		"marker":                 strings.TrimSpace(observation.Marker),
		"optional":               observation.Optional,
		"excluded":               observation.Excluded,
		"source_record_package":  pkg.PackageID,
		"source_record_version":  version,
		"source_record_dep_kind": dependencyType,
		"correlation_anchors":    correlationAnchors(pkg.PackageID, versionID, pkg.PURL, dependency.PackageID, dependency.PURL, dependency.BOMRef),
	}

	return newFact(factInput{
		factKind:            FactKindPackageDependency,
		stableFactKey:       stableFactKey,
		schemaVersion:       SchemaVersion,
		scopeID:             observation.ScopeID,
		generationID:        observation.GenerationID,
		collectorInstanceID: observation.CollectorInstanceID,
		observedAt:          observation.ObservedAt,
		sourceSystem:        sourceSystem,
		sourceURI:           observation.SourceURI,
		sourceRecordID:      versionID + "->" + dependency.PackageID,
		registry:            pkg.Registry,
		payload:             payload,
	}), nil
}

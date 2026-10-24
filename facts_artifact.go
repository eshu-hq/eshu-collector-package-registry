// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"fmt"
	"strings"

	sdk "github.com/eshu-hq/eshu-sdk/collector"
	stableid "github.com/eshu-hq/eshu-sdk/factschema/identity"
)

// NewPackageArtifactEnvelope builds the durable artifact fact for one package
// version artifact.
func NewPackageArtifactEnvelope(observation PackageArtifactObservation, sourceSystem string) (sdk.Fact, error) {
	pkg, version, versionID, err := packageVersionID(observation.Package, observation.Version, "package artifact")
	if err != nil {
		return sdk.Fact{}, err
	}
	if err := validateObservationBoundary(
		observation.ScopeID,
		observation.GenerationID,
		observation.CollectorInstanceID,
		"package artifact observation",
	); err != nil {
		return sdk.Fact{}, err
	}
	artifactKey := strings.TrimSpace(observation.ArtifactKey)
	if artifactKey == "" {
		return sdk.Fact{}, fmt.Errorf("package artifact key must not be blank")
	}

	stableFactKey := stableid.StableID(FactKindPackageArtifact, map[string]any{
		"artifact_key": artifactKey,
		"version_id":   versionID,
	})
	payload := map[string]any{
		"collector_instance_id": observation.CollectorInstanceID,
		"ecosystem":             string(pkg.Ecosystem),
		"registry":              pkg.Registry,
		"package_id":            pkg.PackageID,
		"version_id":            versionID,
		"version":               version,
		"artifact_key":          artifactKey,
		"artifact_type":         strings.TrimSpace(observation.ArtifactType),
		"artifact_url":          sanitizeURL(observation.ArtifactURL),
		"artifact_path":         strings.TrimSpace(observation.ArtifactPath),
		"size_bytes":            observation.SizeBytes,
		"hashes":                cloneStringMap(observation.Hashes),
		"classifier":            strings.TrimSpace(observation.Classifier),
		"platform_tags":         cloneStrings(observation.PlatformTags),
		"correlation_anchors":   correlationAnchors(pkg.PackageID, versionID, artifactKey),
	}

	return newFact(factInput{
		factKind:            FactKindPackageArtifact,
		stableFactKey:       stableFactKey,
		schemaVersion:       SchemaVersion,
		scopeID:             observation.ScopeID,
		generationID:        observation.GenerationID,
		collectorInstanceID: observation.CollectorInstanceID,
		observedAt:          observation.ObservedAt,
		sourceSystem:        sourceSystem,
		sourceURI:           observation.SourceURI,
		sourceRecordID:      versionID + "#" + artifactKey,
		registry:            pkg.Registry,
		payload:             payload,
	}), nil
}

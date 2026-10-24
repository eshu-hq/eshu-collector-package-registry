// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"fmt"
	"strings"

	sdk "github.com/eshu-hq/eshu-sdk/collector"
	stableid "github.com/eshu-hq/eshu-sdk/factschema/identity"
)

// NewSourceHintEnvelope builds the durable source-hint fact for one package
// metadata source, homepage, SCM, or provenance hint.
func NewSourceHintEnvelope(observation SourceHintObservation, sourceSystem string) (sdk.Fact, error) {
	pkg, err := NormalizePackageIdentity(observation.Package)
	if err != nil {
		return sdk.Fact{}, err
	}
	if err := validateObservationBoundary(
		observation.ScopeID,
		observation.GenerationID,
		observation.CollectorInstanceID,
		"source hint observation",
	); err != nil {
		return sdk.Fact{}, err
	}
	hintKind := strings.TrimSpace(observation.HintKind)
	if hintKind == "" {
		return sdk.Fact{}, fmt.Errorf("source hint kind must not be blank")
	}
	rawURL := sanitizeURL(observation.RawURL)
	normalizedURL := sanitizeURL(observation.NormalizedURL)
	stableURL := normalizedURL
	if stableURL == "" {
		stableURL = rawURL
	}
	if stableURL == "" {
		return sdk.Fact{}, fmt.Errorf("source hint URL must not be blank")
	}
	version := strings.TrimSpace(observation.Version)
	versionID := ""
	if version != "" {
		versionID = pkg.PackageID + "@" + version
	}

	stableFactKey := stableid.StableID(FactKindSourceHint, map[string]any{
		"hint_kind":      hintKind,
		"package_id":     pkg.PackageID,
		"source_url":     stableURL,
		"source_version": version,
	})
	payload := map[string]any{
		"collector_instance_id": observation.CollectorInstanceID,
		"ecosystem":             string(pkg.Ecosystem),
		"registry":              pkg.Registry,
		"package_id":            pkg.PackageID,
		"version_id":            versionID,
		"version":               version,
		"hint_kind":             hintKind,
		"raw_url":               rawURL,
		"normalized_url":        normalizedURL,
		"confidence_reason":     strings.TrimSpace(observation.ConfidenceReason),
		"correlation_anchors":   correlationAnchors(pkg.PackageID, versionID, stableURL),
	}

	return newFact(factInput{
		factKind:            FactKindSourceHint,
		stableFactKey:       stableFactKey,
		schemaVersion:       SchemaVersion,
		scopeID:             observation.ScopeID,
		generationID:        observation.GenerationID,
		collectorInstanceID: observation.CollectorInstanceID,
		observedAt:          observation.ObservedAt,
		sourceSystem:        sourceSystem,
		sourceURI:           observation.SourceURI,
		sourceRecordID:      pkg.PackageID + "#" + hintKind + "#" + stableURL,
		registry:            pkg.Registry,
		payload:             payload,
	}), nil
}

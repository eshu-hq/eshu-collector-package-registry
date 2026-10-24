// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"fmt"
	"strings"

	sdk "github.com/eshu-hq/eshu-sdk/collector"
	stableid "github.com/eshu-hq/eshu-sdk/factschema/identity"
)

// NewRegistryEventEnvelope builds the durable source-reported registry event
// fact for one package or package version.
func NewRegistryEventEnvelope(observation RegistryEventObservation, sourceSystem string) (sdk.Fact, error) {
	pkg, version, versionID, err := optionalPackageVersionID(
		observation.Package,
		observation.Version,
	)
	if err != nil {
		return sdk.Fact{}, err
	}
	if err := validateObservationBoundary(
		observation.ScopeID,
		observation.GenerationID,
		observation.CollectorInstanceID,
		"registry event observation",
	); err != nil {
		return sdk.Fact{}, err
	}
	eventKey := strings.TrimSpace(observation.EventKey)
	if eventKey == "" {
		return sdk.Fact{}, fmt.Errorf("registry event key must not be blank")
	}
	eventType := strings.TrimSpace(observation.EventType)
	if eventType == "" {
		return sdk.Fact{}, fmt.Errorf("registry event type must not be blank")
	}
	artifactKey := strings.TrimSpace(observation.ArtifactKey)

	stableFactKey := stableid.StableID(FactKindRegistryEvent, map[string]any{
		"artifact_key": artifactKey,
		"event_key":    eventKey,
		"event_type":   eventType,
		"package_id":   pkg.PackageID,
		"version":      version,
	})
	payload := map[string]any{
		"collector_instance_id": observation.CollectorInstanceID,
		"ecosystem":             string(pkg.Ecosystem),
		"registry":              pkg.Registry,
		"package_id":            pkg.PackageID,
		"version_id":            versionID,
		"version":               version,
		"event_key":             eventKey,
		"event_type":            eventType,
		"artifact_key":          artifactKey,
		"actor":                 strings.TrimSpace(observation.Actor),
		"message":               sanitizeText(observation.Message),
		"occurred_at":           observation.OccurredAt,
		"correlation_anchors":   correlationAnchors(pkg.PackageID, versionID, eventKey, artifactKey),
	}

	return newFact(factInput{
		factKind:            FactKindRegistryEvent,
		stableFactKey:       stableFactKey,
		schemaVersion:       SchemaVersion,
		scopeID:             observation.ScopeID,
		generationID:        observation.GenerationID,
		collectorInstanceID: observation.CollectorInstanceID,
		observedAt:          observation.ObservedAt,
		sourceSystem:        sourceSystem,
		sourceURI:           observation.SourceURI,
		sourceRecordID:      eventKey,
		registry:            pkg.Registry,
		payload:             payload,
	}), nil
}

// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"fmt"
	"strings"

	sdk "github.com/eshu-hq/eshu-sdk/collector"
	stableid "github.com/eshu-hq/eshu-sdk/factschema/identity"
)

// NewRepositoryHostingEnvelope builds the durable provider topology fact for a
// package repository or feed.
func NewRepositoryHostingEnvelope(observation RepositoryHostingObservation, sourceSystem string) (sdk.Fact, error) {
	if err := validateObservationBoundary(
		observation.ScopeID,
		observation.GenerationID,
		observation.CollectorInstanceID,
		"repository hosting observation",
	); err != nil {
		return sdk.Fact{}, err
	}
	provider := strings.TrimSpace(observation.Provider)
	if provider == "" {
		return sdk.Fact{}, fmt.Errorf("repository hosting provider must not be blank")
	}
	registry := normalizeRegistry(observation.Registry)
	if registry == "" {
		return sdk.Fact{}, fmt.Errorf("repository hosting registry must not be blank")
	}
	repository := strings.TrimSpace(observation.Repository)
	if repository == "" {
		return sdk.Fact{}, fmt.Errorf("repository hosting repository must not be blank")
	}
	repositoryType := strings.TrimSpace(observation.RepositoryType)
	repositoryID := provider + "://" + registry + "/" + repository

	stableFactKey := stableid.StableID(FactKindRepositoryHosting, map[string]any{
		"provider":        provider,
		"registry":        registry,
		"repository":      repository,
		"repository_type": repositoryType,
	})
	payload := map[string]any{
		"collector_instance_id": observation.CollectorInstanceID,
		"provider":              provider,
		"registry":              registry,
		"repository":            repository,
		"repository_type":       repositoryType,
		"ecosystem":             string(observation.Ecosystem),
		"upstream_id":           strings.TrimSpace(observation.UpstreamID),
		"upstream_url":          sanitizeURL(observation.UpstreamURL),
		"correlation_anchors":   correlationAnchors(repositoryID, strings.TrimSpace(observation.UpstreamID), sanitizeURL(observation.UpstreamURL)),
	}

	return newFact(factInput{
		factKind:            FactKindRepositoryHosting,
		stableFactKey:       stableFactKey,
		schemaVersion:       SchemaVersion,
		scopeID:             observation.ScopeID,
		generationID:        observation.GenerationID,
		collectorInstanceID: observation.CollectorInstanceID,
		observedAt:          observation.ObservedAt,
		sourceSystem:        sourceSystem,
		sourceURI:           observation.SourceURI,
		sourceRecordID:      repositoryID,
		registry:            registry,
		payload:             payload,
	}), nil
}

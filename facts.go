// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"fmt"
	"strings"
	"time"

	sdk "github.com/eshu-hq/eshu-sdk/collector"
	"github.com/eshu-hq/eshu-sdk/factschema"
	stableid "github.com/eshu-hq/eshu-sdk/factschema/identity"
	packageregistryv1 "github.com/eshu-hq/eshu-sdk/factschema/packageregistry/v1"
)

type factInput struct {
	factKind            string
	stableFactKey       string
	schemaVersion       string
	scopeID             string
	generationID        string
	collectorInstanceID string
	observedAt          time.Time
	sourceSystem        string
	sourceURI           string
	sourceRecordID      string
	registry            string
	payload             map[string]any
}

// newFact builds the public SDK fact for one package-registry observation.
// The host, not this collector, derives fencing, claim identity, and commit
// mechanics from the claim, so none of those appear here. The source URI
// falls back from the observation URI to the registry to the scope ID so the
// fact always carries a URI without leaking credential material.
func newFact(input factInput) sdk.Fact {
	return sdk.Fact{
		Kind:             input.factKind,
		SchemaVersion:    input.schemaVersion,
		StableKey:        input.stableFactKey,
		SourceConfidence: sdk.SourceConfidenceReported,
		ObservedAt:       normalizedObservedAt(input.observedAt),
		SourceRef: sdk.SourceRef{
			SourceSystem: input.sourceSystem,
			ScopeID:      input.scopeID,
			GenerationID: input.generationID,
			FactKey:      input.stableFactKey,
			URI:          factURI(input.sourceURI, input.registry, input.scopeID),
			RecordID:     input.sourceRecordID,
		},
		Payload: input.payload,
	}
}

// factURI resolves the emitted source URI fail-closed: the sanitized
// observation URI when it survives, else the sanitized registry, else the
// scope ID (already carried on the fact, so no new disclosure).
func factURI(sourceURI, registry, scopeID string) string {
	if uri := sanitizeURL(sourceURI); uri != "" {
		return uri
	}
	if uri := sanitizeURL(registry); uri != "" {
		return uri
	}
	return strings.TrimSpace(scopeID)
}

func correlationAnchors(values ...string) []string {
	anchors := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		anchors = append(anchors, trimmed)
	}
	if len(anchors) == 0 {
		return nil
	}
	return anchors
}

func mergeContractPayload(payload map[string]any, encode func() (map[string]any, error)) error {
	encoded, err := encode()
	if err != nil {
		return err
	}
	for key, value := range encoded {
		if _, exists := payload[key]; !exists {
			payload[key] = value
		}
	}
	return nil
}

func stringPtr(value string) *string {
	return &value
}

func optionalStringPtr(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func boolPtr(value bool) *bool {
	return &value
}

func payloadString(payload map[string]any, key string) string {
	value, ok := payload[key].(string)
	if !ok {
		return ""
	}
	return value
}

func payloadStringSlice(payload map[string]any, key string) []string {
	value, ok := payload[key].([]string)
	if !ok {
		return nil
	}
	return value
}

func validateObservationBoundary(scopeID, generationID, collectorInstanceID, noun string) error {
	if scopeID == "" {
		return fmt.Errorf("%s scope_id must not be blank", noun)
	}
	if generationID == "" {
		return fmt.Errorf("%s generation_id must not be blank", noun)
	}
	if collectorInstanceID == "" {
		return fmt.Errorf("%s collector_instance_id must not be blank", noun)
	}
	return nil
}

func packageVersionID(identity PackageIdentity, version, noun string) (NormalizedPackageIdentity, string, string, error) {
	trimmedVersion := strings.TrimSpace(version)
	if trimmedVersion == "" {
		return NormalizedPackageIdentity{}, "", "", fmt.Errorf("%s version must not be blank", noun)
	}
	identity.Version = trimmedVersion
	normalized, err := NormalizePackageIdentity(identity)
	if err != nil {
		return NormalizedPackageIdentity{}, "", "", err
	}
	return normalized, trimmedVersion, normalized.PackageID + "@" + trimmedVersion, nil
}

func optionalPackageVersionID(
	identity PackageIdentity,
	version string,
) (NormalizedPackageIdentity, string, string, error) {
	trimmedVersion := strings.TrimSpace(version)
	identity.Version = trimmedVersion
	normalized, err := NormalizePackageIdentity(identity)
	if err != nil {
		return NormalizedPackageIdentity{}, "", "", err
	}
	versionID := ""
	if trimmedVersion != "" {
		versionID = normalized.PackageID + "@" + trimmedVersion
	}
	return normalized, trimmedVersion, versionID, nil
}

func normalizedObservedAt(observedAt time.Time) time.Time {
	if observedAt.IsZero() {
		return time.Now().UTC()
	}
	return observedAt.UTC()
}

func cloneStrings(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	cloned := make([]string, len(input))
	copy(cloned, input)
	return cloned
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}

// NewPackageEnvelope builds the durable package identity fact for one package
// observation.
func NewPackageEnvelope(observation PackageObservation, sourceSystem string) (sdk.Fact, error) {
	normalized, err := NormalizePackageIdentity(observation.Identity)
	if err != nil {
		return sdk.Fact{}, err
	}
	if observation.ScopeID == "" {
		return sdk.Fact{}, fmt.Errorf("package observation scope_id must not be blank")
	}
	if observation.GenerationID == "" {
		return sdk.Fact{}, fmt.Errorf("package observation generation_id must not be blank")
	}
	if observation.CollectorInstanceID == "" {
		return sdk.Fact{}, fmt.Errorf("package observation collector_instance_id must not be blank")
	}
	visibility := observation.Visibility
	if visibility == "" {
		visibility = VisibilityUnknown
	}

	stableFactKey := stableid.StableID(FactKindPackage, map[string]any{
		"package_id": normalized.PackageID,
	})
	payload := map[string]any{
		"collector_instance_id": observation.CollectorInstanceID,
		"ecosystem":             string(normalized.Ecosystem),
		"registry":              normalized.Registry,
		"raw_name":              normalized.RawName,
		"normalized_name":       normalized.NormalizedName,
		"namespace":             normalized.Namespace,
		"classifier":            normalized.Classifier,
		"package_id":            normalized.PackageID,
		"purl":                  normalized.PURL,
		"bom_ref":               normalized.BOMRef,
		"package_manager":       normalized.PackageManager,
		"source_path":           normalized.SourcePath,
		"source_specific_id":    normalized.SourceSpecificID,
		"visibility":            string(visibility),
		"correlation_anchors":   correlationAnchors(normalized.PackageID, normalized.PURL, normalized.BOMRef),
	}
	if err := mergeContractPayload(payload, func() (map[string]any, error) {
		return factschema.EncodePackageRegistryPackage(packageregistryv1.Package{
			PackageID:           normalized.PackageID,
			Ecosystem:           stringPtr(string(normalized.Ecosystem)),
			Registry:            stringPtr(normalized.Registry),
			RawName:             stringPtr(normalized.RawName),
			NormalizedName:      stringPtr(normalized.NormalizedName),
			Namespace:           stringPtr(normalized.Namespace),
			Classifier:          stringPtr(normalized.Classifier),
			PURL:                stringPtr(normalized.PURL),
			BOMRef:              stringPtr(normalized.BOMRef),
			PackageManager:      stringPtr(normalized.PackageManager),
			SourcePath:          stringPtr(normalized.SourcePath),
			SourceSpecificID:    stringPtr(normalized.SourceSpecificID),
			Visibility:          stringPtr(string(visibility)),
			CollectorInstanceID: stringPtr(observation.CollectorInstanceID),
			CorrelationAnchors:  correlationAnchors(normalized.PackageID, normalized.PURL, normalized.BOMRef),
		})
	}); err != nil {
		return sdk.Fact{}, err
	}

	return newFact(factInput{
		factKind:            FactKindPackage,
		stableFactKey:       stableFactKey,
		schemaVersion:       SchemaVersion,
		scopeID:             observation.ScopeID,
		generationID:        observation.GenerationID,
		collectorInstanceID: observation.CollectorInstanceID,
		observedAt:          observation.ObservedAt,
		sourceSystem:        sourceSystem,
		sourceURI:           observation.SourceURI,
		sourceRecordID:      normalized.PackageID,
		registry:            normalized.Registry,
		payload:             payload,
	}), nil
}

// NewPackageVersionEnvelope builds the durable package version fact for one
// registry observation.
func NewPackageVersionEnvelope(observation PackageVersionObservation, sourceSystem string) (sdk.Fact, error) {
	version := strings.TrimSpace(observation.Version)
	if version == "" {
		return sdk.Fact{}, fmt.Errorf("package version must not be blank")
	}
	identity := observation.Package
	identity.Version = version
	normalized, err := NormalizePackageIdentity(identity)
	if err != nil {
		return sdk.Fact{}, err
	}
	if observation.ScopeID == "" {
		return sdk.Fact{}, fmt.Errorf("package version observation scope_id must not be blank")
	}
	if observation.GenerationID == "" {
		return sdk.Fact{}, fmt.Errorf("package version observation generation_id must not be blank")
	}
	if observation.CollectorInstanceID == "" {
		return sdk.Fact{}, fmt.Errorf("package version observation collector_instance_id must not be blank")
	}

	versionID := normalized.PackageID + "@" + version
	stableFactKey := stableid.StableID(FactKindPackageVersion, map[string]any{
		"version_id": versionID,
	})
	publishedAt := ""
	if !observation.PublishedAt.IsZero() {
		publishedAt = observation.PublishedAt.UTC().Format(time.RFC3339)
	}
	payload, err := factschema.EncodePackageRegistryPackageVersion(packageregistryv1.PackageVersion{
		PackageID:           normalized.PackageID,
		VersionID:           versionID,
		Version:             version,
		Ecosystem:           stringPtr(string(normalized.Ecosystem)),
		Registry:            stringPtr(normalized.Registry),
		PURL:                stringPtr(normalized.PURL),
		BOMRef:              stringPtr(normalized.BOMRef),
		PackageManager:      stringPtr(normalized.PackageManager),
		PublishedAt:         optionalStringPtr(publishedAt),
		IsYanked:            boolPtr(observation.Yanked),
		IsUnlisted:          boolPtr(observation.Unlisted),
		IsDeprecated:        boolPtr(observation.Deprecated),
		IsRetracted:         boolPtr(observation.Retracted),
		ArtifactURLs:        cloneStrings(observation.ArtifactURLs),
		Checksums:           cloneStringMap(observation.Checksums),
		CollectorInstanceID: stringPtr(observation.CollectorInstanceID),
		CorrelationAnchors:  correlationAnchors(normalized.PackageID, versionID, normalized.PURL, normalized.BOMRef),
	})
	if err != nil {
		return sdk.Fact{}, fmt.Errorf("encode package version payload: %w", err)
	}

	return newFact(factInput{
		factKind:            FactKindPackageVersion,
		stableFactKey:       stableFactKey,
		schemaVersion:       SchemaVersion,
		scopeID:             observation.ScopeID,
		generationID:        observation.GenerationID,
		collectorInstanceID: observation.CollectorInstanceID,
		observedAt:          observation.ObservedAt,
		sourceSystem:        sourceSystem,
		sourceURI:           observation.SourceURI,
		sourceRecordID:      versionID,
		registry:            normalized.Registry,
		payload:             payload,
	}), nil
}

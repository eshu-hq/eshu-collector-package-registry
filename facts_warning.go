// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"fmt"
	"strings"

	sdk "github.com/eshu-hq/eshu-sdk/collector"
	"github.com/eshu-hq/eshu-sdk/factschema"
	stableid "github.com/eshu-hq/eshu-sdk/factschema/identity"
	packageregistryv1 "github.com/eshu-hq/eshu-sdk/factschema/packageregistry/v1"
)

// NewWarningEnvelope builds the durable warning fact for one non-fatal
// package-registry collection issue.
func NewWarningEnvelope(observation WarningObservation, sourceSystem string) (sdk.Fact, error) {
	if err := validateObservationBoundary(
		observation.ScopeID,
		observation.GenerationID,
		observation.CollectorInstanceID,
		"warning observation",
	); err != nil {
		return sdk.Fact{}, err
	}
	warningKey := strings.TrimSpace(observation.WarningKey)
	if warningKey == "" {
		return sdk.Fact{}, fmt.Errorf("warning key must not be blank")
	}
	warningCode := strings.TrimSpace(observation.WarningCode)
	if warningCode == "" {
		return sdk.Fact{}, fmt.Errorf("warning code must not be blank")
	}

	payload := map[string]any{
		"collector_instance_id": observation.CollectorInstanceID,
		"warning_key":           warningKey,
		"warning_code":          warningCode,
		"severity":              strings.TrimSpace(observation.Severity),
		"message":               sanitizeText(observation.Message),
		"correlation_anchors":   correlationAnchors(observation.ScopeID),
	}
	if ecosystem := strings.TrimSpace(string(observation.Ecosystem)); ecosystem != "" {
		payload["ecosystem"] = ecosystem
	}
	stableIdentity := map[string]any{
		"scope_id":     observation.ScopeID,
		"warning_code": warningCode,
		"warning_key":  warningKey,
	}
	if observation.Package != nil {
		normalized, err := NormalizePackageIdentity(*observation.Package)
		if err != nil {
			return sdk.Fact{}, err
		}
		version := strings.TrimSpace(observation.Version)
		versionID := ""
		if version != "" {
			versionID = normalized.PackageID + "@" + version
		}
		payload["ecosystem"] = string(normalized.Ecosystem)
		payload["registry"] = normalized.Registry
		payload["package_id"] = normalized.PackageID
		payload["version"] = version
		payload["version_id"] = versionID
		payload["correlation_anchors"] = correlationAnchors(observation.ScopeID, normalized.PackageID, versionID)
		stableIdentity["package_id"] = normalized.PackageID
		stableIdentity["version"] = version
	}
	if err := mergeContractPayload(payload, func() (map[string]any, error) {
		return factschema.EncodePackageRegistryWarning(packageregistryv1.Warning{
			WarningKey:          warningKey,
			WarningCode:         warningCode,
			Severity:            optionalStringPtr(observation.Severity),
			Message:             optionalStringPtr(sanitizeText(observation.Message)),
			Ecosystem:           optionalStringPtr(payloadString(payload, "ecosystem")),
			Registry:            optionalStringPtr(payloadString(payload, "registry")),
			PackageID:           optionalStringPtr(payloadString(payload, "package_id")),
			Version:             optionalStringPtr(payloadString(payload, "version")),
			VersionID:           optionalStringPtr(payloadString(payload, "version_id")),
			CollectorInstanceID: stringPtr(observation.CollectorInstanceID),
			CorrelationAnchors:  payloadStringSlice(payload, "correlation_anchors"),
		})
	}); err != nil {
		return sdk.Fact{}, err
	}

	stableFactKey := stableid.StableID(FactKindWarning, stableIdentity)
	return newFact(factInput{
		factKind:            FactKindWarning,
		stableFactKey:       stableFactKey,
		schemaVersion:       SchemaVersion,
		scopeID:             observation.ScopeID,
		generationID:        observation.GenerationID,
		collectorInstanceID: observation.CollectorInstanceID,
		observedAt:          observation.ObservedAt,
		sourceSystem:        sourceSystem,
		sourceURI:           observation.SourceURI,
		sourceRecordID:      warningKey,
		registry:            payloadString(payload, "registry"),
		payload:             payload,
	}), nil
}

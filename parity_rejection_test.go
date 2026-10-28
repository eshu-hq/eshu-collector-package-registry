// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"strings"
	"testing"

	sdk "github.com/eshu-hq/eshu-sdk/collector"
)

// TestExtractionParityKindRequiredFieldsRejectBlank pins the fail-closed half
// of the contract: each constructor rejects its blank required field instead
// of emitting an identity-free fact.
func TestExtractionParityKindRequiredFieldsRejectBlank(t *testing.T) {
	t.Parallel()

	scopeID, generationID, collectorInstanceID := parityBoundary()
	identity := parityIdentity()

	boundary := func() (PackageIdentity, string, string, string, string) {
		return identity, scopeID, generationID, collectorInstanceID, "1.3.0"
	}

	cases := []struct {
		name    string
		wantErr string
		build   func() error
	}{
		{
			name:    "package_version_blank_version",
			wantErr: "package version must not be blank",
			build: func() error {
				pkg, scope, generation, collector, _ := boundary()
				_, err := NewPackageVersionEnvelope(PackageVersionObservation{
					Package: pkg, ScopeID: scope, GenerationID: generation,
					CollectorInstanceID: collector,
				}, paritySourceSystem)
				return err
			},
		},
		{
			name:    "package_dependency_blank_version",
			wantErr: "package dependency version must not be blank",
			build: func() error {
				pkg, scope, generation, collector, _ := boundary()
				_, err := NewPackageDependencyEnvelope(PackageDependencyObservation{
					Package: pkg, Dependency: identity,
					ScopeID: scope, GenerationID: generation,
					CollectorInstanceID: collector,
				}, paritySourceSystem)
				return err
			},
		},
		{
			name:    "package_artifact_blank_key",
			wantErr: "package artifact key must not be blank",
			build: func() error {
				pkg, scope, generation, collector, version := boundary()
				_, err := NewPackageArtifactEnvelope(PackageArtifactObservation{
					Package: pkg, Version: version,
					ScopeID: scope, GenerationID: generation,
					CollectorInstanceID: collector,
				}, paritySourceSystem)
				return err
			},
		},
		{
			name:    "source_hint_blank_kind",
			wantErr: "source hint kind must not be blank",
			build: func() error {
				pkg, scope, generation, collector, version := boundary()
				_, err := NewSourceHintEnvelope(SourceHintObservation{
					Package: pkg, Version: version,
					RawURL:  "https://github.com/camaraoovel/left-pad",
					ScopeID: scope, GenerationID: generation,
					CollectorInstanceID: collector,
				}, paritySourceSystem)
				return err
			},
		},
		{
			name:    "vulnerability_hint_blank_advisory",
			wantErr: "vulnerability hint advisory_id must not be blank",
			build: func() error {
				pkg, scope, generation, collector, version := boundary()
				_, err := NewVulnerabilityHintEnvelope(VulnerabilityHintObservation{
					Package: pkg, Version: version,
					ScopeID: scope, GenerationID: generation,
					CollectorInstanceID: collector,
				}, paritySourceSystem)
				return err
			},
		},
		{
			name:    "registry_event_blank_type",
			wantErr: "registry event type must not be blank",
			build: func() error {
				pkg, scope, generation, collector, version := boundary()
				_, err := NewRegistryEventEnvelope(RegistryEventObservation{
					Package: pkg, Version: version,
					EventKey: "left-pad@1.3.0:publish",
					ScopeID:  scope, GenerationID: generation,
					CollectorInstanceID: collector,
				}, paritySourceSystem)
				return err
			},
		},
		{
			name:    "repository_hosting_blank_provider",
			wantErr: "repository hosting provider must not be blank",
			build: func() error {
				_, scope, generation, collector, _ := boundary()
				_, err := NewRepositoryHostingEnvelope(RepositoryHostingObservation{
					Registry:   "https://registry.npmjs.org",
					Repository: "registry",
					ScopeID:    scope, GenerationID: generation,
					CollectorInstanceID: collector,
				}, paritySourceSystem)
				return err
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if err := tc.build(); err == nil {
				t.Fatalf("blank required field accepted for %s, want rejection", tc.name)
			} else if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("wrong rejection for %s: got %q, want substring %q", tc.name, err, tc.wantErr)
			}
		})
	}
}

// TestExtractionParityHostileDocuments pins source-parity failure modes: the
// destination must reject malformed and identity-free documents with errors
// (the malformed_metadata path) and must refuse unregistered ecosystems (the
// unsupported_metadata_source path) instead of emitting facts.
func TestExtractionParityHostileDocuments(t *testing.T) {
	t.Parallel()

	registry := DefaultMetadataParserRegistry()
	ctx := parityContext(EcosystemNPM, "https://registry.npmjs.org")

	if _, err := registry.Parse(ctx, []byte(`{"name":`)); err == nil {
		t.Fatal("Parse(malformed npm) error = nil, want malformed_metadata error")
	}
	if _, err := registry.Parse(ctx, []byte(`{"name": "", "versions": {}}`)); err == nil {
		t.Fatal("Parse(blank npm name) error = nil, want identity error")
	}
	if _, err := registry.Parse(ctx, nil); err == nil {
		t.Fatal("Parse(empty document) error = nil, want error")
	}

	unregistered := parityContext(EcosystemCargo, "https://crates.io")
	if _, err := registry.Parse(unregistered, []byte(`{}`)); err == nil {
		t.Fatal("Parse(unregistered ecosystem) error = nil, want unsupported error")
	} else if !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("Parse(unregistered ecosystem) error = %q, want %q", err, "not registered")
	}

	// Hostile context: a blank scope must fail at the fact boundary, never
	// emit a fact with an empty key.
	parsed, err := registry.Parse(ctx, []byte(`{"name": "left-pad", "versions": {}}`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	bad := parsed.Packages[0]
	bad.ScopeID = ""
	if _, err := NewPackageEnvelope(bad, paritySourceSystem); err == nil {
		t.Fatal("NewPackageEnvelope(blank scope) error = nil, want boundary error")
	}
}

// TestExtractionParityWarningTaxonomy pins the fact-layer warning contract:
// every known collection warning code builds a durable warning fact with that
// code and correlation anchors, blank keys/codes are rejected, and credential
// material in messages is redacted before emission.
func TestExtractionParityWarningTaxonomy(t *testing.T) {
	t.Parallel()

	codes := []string{
		"version_limit_truncated",
		"registry_not_found",
		"metadata_too_large",
		"unsupported_metadata_source",
		"malformed_metadata",
		"credentials_missing",
	}
	for _, code := range codes {
		t.Run(code, func(t *testing.T) {
			t.Parallel()

			fact, err := NewWarningEnvelope(WarningObservation{
				WarningKey:          code,
				WarningCode:         code,
				Severity:            "warning",
				Message:             "parity probe for " + code,
				ScopeID:             "package-registry://probe/scope",
				GenerationID:        "probe-generation",
				CollectorInstanceID: "probe-collector",
				SourceURI:           "https://registry.example/probe",
			}, paritySourceSystem)
			if err != nil {
				t.Fatalf("NewWarningEnvelope(%s) error = %v", code, err)
			}
			if fact.Kind != FactKindWarning {
				t.Fatalf("Kind = %q, want %q", fact.Kind, FactKindWarning)
			}
			if fact.SchemaVersion != SchemaVersion {
				t.Fatalf("SchemaVersion = %q, want %q", fact.SchemaVersion, SchemaVersion)
			}
			if got := fact.Payload["warning_code"]; got != code {
				t.Fatalf("warning_code = %#v, want %q", got, code)
			}
		})
	}

	blank := WarningObservation{
		ScopeID:             "package-registry://probe/scope",
		GenerationID:        "probe-generation",
		CollectorInstanceID: "probe-collector",
	}
	if _, err := NewWarningEnvelope(blank, paritySourceSystem); err == nil {
		t.Fatal("NewWarningEnvelope(blank key/code) error = nil, want rejection")
	}

	leaky, err := NewWarningEnvelope(WarningObservation{
		WarningKey:          "parity-leak",
		WarningCode:         "malformed_metadata",
		Severity:            "warning",
		Message:             "see https://user:secret@example.com/x?token=abc for detail",
		ScopeID:             "package-registry://probe/scope",
		GenerationID:        "probe-generation",
		CollectorInstanceID: "probe-collector",
	}, paritySourceSystem)
	if err != nil {
		t.Fatalf("NewWarningEnvelope() error = %v", err)
	}
	if message, ok := leaky.Payload["message"].(string); ok && (strings.Contains(message, "secret") ||
		strings.Contains(message, "token=abc")) {
		t.Fatalf("warning message leaks credential material: %q", message)
	}
}

func parityContext(ecosystem Ecosystem, registry string) MetadataParserContext {
	return MetadataParserContext{
		Ecosystem:           ecosystem,
		Registry:            registry,
		ScopeID:             string(ecosystem) + "://parity-scope",
		GenerationID:        "fixture-generation",
		CollectorInstanceID: "fixture-collector",
		FencingToken:        7,
		ObservedAt:          parityObservedAt,
		SourceURI:           registry + "/parity",
	}
}

func assertParityFact(t *testing.T, fact sdk.Fact, scopeID string) {
	t.Helper()

	if fact.Kind != FactKindPackage {
		t.Fatalf("Kind = %q, want %q", fact.Kind, FactKindPackage)
	}
	if fact.SchemaVersion != SchemaVersion {
		t.Fatalf("SchemaVersion = %q, want %q", fact.SchemaVersion, SchemaVersion)
	}
	if fact.SourceConfidence != sdk.SourceConfidenceReported {
		t.Fatalf("SourceConfidence = %q, want reported", fact.SourceConfidence)
	}
	if fact.SourceRef.ScopeID != scopeID {
		t.Fatalf("SourceRef.ScopeID = %q, want %q", fact.SourceRef.ScopeID, scopeID)
	}
	if fact.StableKey == "" {
		t.Fatal("StableKey must be set")
	}
	if fact.SourceRef.FactKey != fact.StableKey {
		t.Fatal("SourceRef.FactKey must equal StableKey")
	}
	if fact.SourceRef.URI == "" {
		t.Fatal("SourceRef.URI must be set")
	}
}

func containsAnchor(anchors []string, want string) bool {
	for _, anchor := range anchors {
		if anchor == want {
			return true
		}
	}
	return false
}

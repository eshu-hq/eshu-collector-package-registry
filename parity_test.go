// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"testing"
	"time"

	sdk "github.com/eshu-hq/eshu-sdk/collector"
)

var parityObservedAt = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

const paritySourceSystem = "package_registry"

func parityIdentity() PackageIdentity {
	return PackageIdentity{
		Ecosystem: EcosystemNPM,
		Registry:  "https://registry.npmjs.org",
		RawName:   "left-pad",
	}
}

func parityBoundary() (scopeID, generationID, collectorInstanceID string) {
	return "npm://parity-scope", "fixture-generation", "fixture-collector"
}

// TestExtractionParityRegisteredEcosystems pins the parser-to-fact-to-
// factschema contract the extraction destination preserves: every registered
// ecosystem parser emits observations that build facts with the pinned fact
// kind, schema version, reported confidence, scope/generation propagation,
// stable keys, and correlation anchors.
func TestExtractionParityRegisteredEcosystems(t *testing.T) {
	t.Parallel()

	fixtures := []struct {
		name      string
		ecosystem Ecosystem
		registry  string
		document  string
	}{
		{
			name:      "npm",
			ecosystem: EcosystemNPM,
			registry:  "https://registry.npmjs.org",
			document:  `{"name": "left-pad", "versions": {"1.3.0": {}}}`,
		},
		{
			name:      "pypi",
			ecosystem: EcosystemPyPI,
			registry:  "https://pypi.org",
			document:  `{"info": {"name": "requests"}, "releases": {"2.0.0": []}}`,
		},
		{
			name:      "gomodule",
			ecosystem: EcosystemGoModule,
			registry:  "https://proxy.golang.org",
			document:  `{"module": "example.com/mod", "version": "v1.0.0"}`,
		},
		{
			name:      "maven",
			ecosystem: EcosystemMaven,
			registry:  "https://repo.maven.apache.org/maven2",
			document:  `<project><groupId>com.example</groupId><artifactId>app</artifactId><version>1.0.0</version></project>`,
		},
		{
			name:      "nuget",
			ecosystem: EcosystemNuGet,
			registry:  "https://api.nuget.org/v3/index.json",
			document:  `<package><metadata><id>Newtonsoft.Json</id><version>13.0.1</version></metadata></package>`,
		},
		{
			name:      "generic",
			ecosystem: EcosystemGeneric,
			registry:  "https://registry.example",
			document:  `{"name": "acme-widget", "version": "1.0.0"}`,
		},
	}

	registry := DefaultMetadataParserRegistry()
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()

			ctx := parityContext(fixture.ecosystem, fixture.registry)
			parsed, err := registry.Parse(ctx, []byte(fixture.document))
			if err != nil {
				t.Fatalf("Parse(%s) error = %v", fixture.name, err)
			}
			if len(parsed.Packages) == 0 {
				t.Fatalf("Parse(%s) emitted no package observations", fixture.name)
			}

			fact, err := NewPackageEnvelope(parsed.Packages[0], paritySourceSystem)
			if err != nil {
				t.Fatalf("NewPackageEnvelope(%s) error = %v", fixture.name, err)
			}
			assertParityFact(t, fact, ctx.ScopeID)

			anchors, ok := fact.Payload["correlation_anchors"].([]string)
			if !ok || len(anchors) == 0 {
				t.Fatalf("correlation_anchors = %#v, want non-empty", fact.Payload["correlation_anchors"])
			}
			packageID, ok := fact.Payload["package_id"].(string)
			if !ok || packageID == "" {
				t.Fatalf("package_id payload is empty for %s", fixture.name)
			}
			if !containsAnchor(anchors, packageID) {
				t.Fatalf("correlation_anchors %q missing package_id %q", anchors, packageID)
			}
			// Factschema parity: the contract-encoded payload keys must be
			// present alongside the hand-built ones.
			for _, key := range []string{"ecosystem", "registry", "purl", "bom_ref"} {
				if _, ok := fact.Payload[key]; !ok {
					t.Fatalf("payload missing factschema parity key %q", key)
				}
			}
		})
	}
}

// TestExtractionParityStableKeyAcrossGenerations pins the key contract the
// extracted producer and core reducer share: the same package identity under
// the same scope keeps its StableKey across generations. Generation advance
// is the host's concern; the stable key is the collector's.
func TestExtractionParityStableKeyAcrossGenerations(t *testing.T) {
	t.Parallel()

	first := parityContext(EcosystemNPM, "https://registry.npmjs.org")
	first.GenerationID = "generation-a"
	parsed, err := DefaultMetadataParserRegistry().Parse(first, []byte(`{"name": "left-pad", "versions": {}}`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	firstFact, err := NewPackageEnvelope(parsed.Packages[0], paritySourceSystem)
	if err != nil {
		t.Fatalf("NewPackageEnvelope() error = %v", err)
	}

	second := first
	second.GenerationID = "generation-b"
	parsed, err = DefaultMetadataParserRegistry().Parse(second, []byte(`{"name": "left-pad", "versions": {}}`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	secondFact, err := NewPackageEnvelope(parsed.Packages[0], paritySourceSystem)
	if err != nil {
		t.Fatalf("NewPackageEnvelope() error = %v", err)
	}

	if firstFact.StableKey == "" {
		t.Fatal("StableKey is empty")
	}
	if firstFact.StableKey != secondFact.StableKey {
		t.Fatalf("StableKey changed across generations: %q vs %q",
			firstFact.StableKey, secondFact.StableKey)
	}
	if firstFact.SourceRef.GenerationID == secondFact.SourceRef.GenerationID {
		t.Fatal("SourceRef generation_id did not advance across generations")
	}
}

// TestExtractionParityAllKindsAdmitAtV1 pins the fact contract for every
// package-registry fact kind the census locks: each constructor admits a
// well-formed observation with its pinned kind, 1.0.0 schema, reported
// confidence, scope propagation, and a stable key. A drift in any moved
// family fails here, not just in package/warning.
func TestExtractionParityAllKindsAdmitAtV1(t *testing.T) {
	t.Parallel()

	scopeID, generationID, collectorInstanceID := parityBoundary()
	identity := parityIdentity()

	cases := []struct {
		name   string
		kind   string
		schema string
		build  func() (sdk.Fact, error)
	}{
		{
			name:   "package",
			kind:   FactKindPackage,
			schema: SchemaVersion,
			build: func() (sdk.Fact, error) {
				return NewPackageEnvelope(PackageObservation{
					Identity:            identity,
					ScopeID:             scopeID,
					GenerationID:        generationID,
					CollectorInstanceID: collectorInstanceID,
					FencingToken:        7,
					ObservedAt:          parityObservedAt,
					SourceURI:           "https://registry.npmjs.org/left-pad",
				}, paritySourceSystem)
			},
		},
		{
			name:   "package_version",
			kind:   FactKindPackageVersion,
			schema: SchemaVersion,
			build: func() (sdk.Fact, error) {
				return NewPackageVersionEnvelope(PackageVersionObservation{
					Package:             identity,
					Version:             "1.3.0",
					ScopeID:             scopeID,
					GenerationID:        generationID,
					CollectorInstanceID: collectorInstanceID,
					FencingToken:        7,
					ObservedAt:          parityObservedAt,
					SourceURI:           "https://registry.npmjs.org/left-pad",
				}, paritySourceSystem)
			},
		},
		{
			name:   "package_dependency",
			kind:   FactKindPackageDependency,
			schema: SchemaVersion,
			build: func() (sdk.Fact, error) {
				return NewPackageDependencyEnvelope(PackageDependencyObservation{
					Package:             identity,
					Version:             "1.3.0",
					Dependency:          PackageIdentity{Ecosystem: EcosystemNPM, Registry: "https://registry.npmjs.org", RawName: "concat-stream"},
					Range:               "^1.6.0",
					DependencyType:      "runtime",
					ScopeID:             scopeID,
					GenerationID:        generationID,
					CollectorInstanceID: collectorInstanceID,
					FencingToken:        7,
					ObservedAt:          parityObservedAt,
					SourceURI:           "https://registry.npmjs.org/left-pad",
				}, paritySourceSystem)
			},
		},
		{
			name:   "package_artifact",
			kind:   FactKindPackageArtifact,
			schema: SchemaVersion,
			build: func() (sdk.Fact, error) {
				return NewPackageArtifactEnvelope(PackageArtifactObservation{
					Package:             identity,
					Version:             "1.3.0",
					ArtifactKey:         "left-pad-1.3.0.tgz",
					ArtifactType:        "tarball",
					ArtifactURL:         "https://registry.npmjs.org/left-pad/-/left-pad-1.3.0.tgz",
					ScopeID:             scopeID,
					GenerationID:        generationID,
					CollectorInstanceID: collectorInstanceID,
					FencingToken:        7,
					ObservedAt:          parityObservedAt,
					SourceURI:           "https://registry.npmjs.org/left-pad",
				}, paritySourceSystem)
			},
		},
		{
			name:   "source_hint",
			kind:   FactKindSourceHint,
			schema: SchemaVersion,
			build: func() (sdk.Fact, error) {
				return NewSourceHintEnvelope(SourceHintObservation{
					Package:             identity,
					Version:             "1.3.0",
					HintKind:            "repository",
					RawURL:              "https://github.com/camaraoovel/left-pad",
					ScopeID:             scopeID,
					GenerationID:        generationID,
					CollectorInstanceID: collectorInstanceID,
					FencingToken:        7,
					ObservedAt:          parityObservedAt,
					SourceURI:           "https://registry.npmjs.org/left-pad",
				}, paritySourceSystem)
			},
		},
		{
			name:   "vulnerability_hint",
			kind:   FactKindVulnerabilityHint,
			schema: SchemaVersion,
			build: func() (sdk.Fact, error) {
				return NewVulnerabilityHintEnvelope(VulnerabilityHintObservation{
					Package:             identity,
					Version:             "1.3.0",
					AdvisoryID:          "GHSA-xxxx-yyyy-zzzz",
					AdvisorySource:      "github",
					VulnerabilityID:     "CVE-2026-0001",
					ScopeID:             scopeID,
					GenerationID:        generationID,
					CollectorInstanceID: collectorInstanceID,
					FencingToken:        7,
					ObservedAt:          parityObservedAt,
					SourceURI:           "https://registry.npmjs.org/left-pad",
				}, paritySourceSystem)
			},
		},
		{
			name:   "registry_event",
			kind:   FactKindRegistryEvent,
			schema: SchemaVersion,
			build: func() (sdk.Fact, error) {
				return NewRegistryEventEnvelope(RegistryEventObservation{
					Package:             identity,
					Version:             "1.3.0",
					EventKey:            "left-pad@1.3.0:publish",
					EventType:           "publish",
					Actor:               "registry",
					ScopeID:             scopeID,
					GenerationID:        generationID,
					CollectorInstanceID: collectorInstanceID,
					FencingToken:        7,
					ObservedAt:          parityObservedAt,
					SourceURI:           "https://registry.npmjs.org/left-pad",
				}, paritySourceSystem)
			},
		},
		{
			name:   "repository_hosting",
			kind:   FactKindRepositoryHosting,
			schema: SchemaVersion,
			build: func() (sdk.Fact, error) {
				return NewRepositoryHostingEnvelope(RepositoryHostingObservation{
					Provider:            "npm",
					Registry:            "https://registry.npmjs.org",
					Repository:          "registry",
					RepositoryType:      "package-feed",
					Ecosystem:           EcosystemNPM,
					ScopeID:             scopeID,
					GenerationID:        generationID,
					CollectorInstanceID: collectorInstanceID,
					FencingToken:        7,
					ObservedAt:          parityObservedAt,
					SourceURI:           "https://registry.npmjs.org",
				}, paritySourceSystem)
			},
		},
		{
			name:   "warning",
			kind:   FactKindWarning,
			schema: SchemaVersion,
			build: func() (sdk.Fact, error) {
				return NewWarningEnvelope(WarningObservation{
					WarningKey:          "parity-warning",
					WarningCode:         "metadata_truncated",
					Severity:            "info",
					Message:             "parity probe",
					Ecosystem:           EcosystemNPM,
					ScopeID:             scopeID,
					GenerationID:        generationID,
					CollectorInstanceID: collectorInstanceID,
					FencingToken:        7,
					ObservedAt:          parityObservedAt,
					SourceURI:           "https://registry.npmjs.org/left-pad",
				}, paritySourceSystem)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fact, err := tc.build()
			if err != nil {
				t.Fatalf("fact build(%s) error = %v", tc.name, err)
			}
			if fact.Kind != tc.kind {
				t.Fatalf("Kind = %q, want %q", fact.Kind, tc.kind)
			}
			if fact.SchemaVersion != tc.schema {
				t.Fatalf("SchemaVersion = %q, want %q", fact.SchemaVersion, tc.schema)
			}
			if fact.SourceConfidence != sdk.SourceConfidenceReported {
				t.Fatalf("SourceConfidence = %q, want reported", fact.SourceConfidence)
			}
			if fact.SourceRef.ScopeID != scopeID {
				t.Fatalf("SourceRef.ScopeID = %q, want %q", fact.SourceRef.ScopeID, scopeID)
			}
			if fact.SourceRef.GenerationID != generationID {
				t.Fatalf("SourceRef.GenerationID = %q, want %q", fact.SourceRef.GenerationID, generationID)
			}
			if !fact.ObservedAt.Equal(parityObservedAt) {
				t.Fatalf("ObservedAt = %v, want %v", fact.ObservedAt, parityObservedAt)
			}
			if fact.StableKey == "" {
				t.Fatalf("StableKey must be set for %s", tc.name)
			}
			if fact.SourceRef.FactKey != fact.StableKey {
				t.Fatalf("SourceRef.FactKey must equal StableKey for %s", tc.name)
			}
		})
	}
}

// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"strings"
	"testing"
)

// ratchetBoundary returns the observation boundary the envelope constructors
// require: scope, generation, and collector instance ids.
func ratchetBoundary() (scopeID, generationID, collectorInstanceID string) {
	return "npm://ratchet-scope", "ratchet-generation", "ratchet-collector"
}

// TestRatchetArtifactEnvelopeRejections pins the artifact fail-closed
// branches: an unnormalizable identity, a blank boundary, and a blank
// artifact key.
func TestRatchetArtifactEnvelopeRejections(t *testing.T) {
	t.Parallel()

	scope, generation, instance := ratchetBoundary()
	base := PackageArtifactObservation{
		Package: ratchetIdentity("left-pad"), Version: "1.0", ArtifactKey: "pkg-1.0.tgz",
		ScopeID: scope, GenerationID: generation, CollectorInstanceID: instance,
	}
	badIdentity := base
	badIdentity.Package = PackageIdentity{}
	if _, err := NewPackageArtifactEnvelope(badIdentity, paritySourceSystem); err == nil {
		t.Fatal("NewPackageArtifactEnvelope(bad identity) error = nil, want rejection")
	}
	blankBoundary := base
	blankBoundary.ScopeID = ""
	if _, err := NewPackageArtifactEnvelope(blankBoundary, paritySourceSystem); err == nil {
		t.Fatal("NewPackageArtifactEnvelope(blank scope) error = nil, want rejection")
	} else if !strings.Contains(err.Error(), "scope_id") {
		t.Fatalf("error = %q, want the boundary noun", err)
	}
	blankKey := base
	blankKey.ArtifactKey = "  "
	if _, err := NewPackageArtifactEnvelope(blankKey, paritySourceSystem); err == nil {
		t.Fatal("NewPackageArtifactEnvelope(blank key) error = nil, want rejection")
	} else if !strings.Contains(err.Error(), "artifact key") {
		t.Fatalf("error = %q, want the key branch", err)
	}
}

// TestRatchetDependencyEnvelopeRejections pins the dependency fail-closed
// branches: an unnormalizable dependency identity and a blank boundary.
func TestRatchetDependencyEnvelopeRejections(t *testing.T) {
	t.Parallel()

	scope, generation, instance := ratchetBoundary()
	base := PackageDependencyObservation{
		Package: ratchetIdentity("left-pad"), Version: "1.0", Dependency: ratchetIdentity("dep"),
		ScopeID: scope, GenerationID: generation, CollectorInstanceID: instance,
	}
	badDependency := base
	badDependency.Dependency = PackageIdentity{}
	if _, err := NewPackageDependencyEnvelope(badDependency, paritySourceSystem); err == nil {
		t.Fatal("NewPackageDependencyEnvelope(bad dependency) error = nil, want rejection")
	}
	blankBoundary := base
	blankBoundary.GenerationID = ""
	if _, err := NewPackageDependencyEnvelope(blankBoundary, paritySourceSystem); err == nil {
		t.Fatal("NewPackageDependencyEnvelope(blank generation) error = nil, want rejection")
	} else if !strings.Contains(err.Error(), "generation_id") {
		t.Fatalf("error = %q, want the boundary noun", err)
	}
}

// TestRatchetRegistryEventEnvelopeRejections pins the event fail-closed
// branches: an unnormalizable identity, a blank boundary, and a blank event
// key.
func TestRatchetRegistryEventEnvelopeRejections(t *testing.T) {
	t.Parallel()

	scope, generation, instance := ratchetBoundary()
	base := RegistryEventObservation{
		Package: ratchetIdentity("left-pad"), Version: "1.0", EventKey: "e1", EventType: "publish",
		ScopeID: scope, GenerationID: generation, CollectorInstanceID: instance,
	}
	badIdentity := base
	badIdentity.Package = PackageIdentity{}
	badIdentity.Version = ""
	if _, err := NewRegistryEventEnvelope(badIdentity, paritySourceSystem); err == nil {
		t.Fatal("NewRegistryEventEnvelope(bad identity) error = nil, want rejection")
	}
	blankBoundary := base
	blankBoundary.CollectorInstanceID = ""
	if _, err := NewRegistryEventEnvelope(blankBoundary, paritySourceSystem); err == nil {
		t.Fatal("NewRegistryEventEnvelope(blank instance) error = nil, want rejection")
	} else if !strings.Contains(err.Error(), "collector_instance_id") {
		t.Fatalf("error = %q, want the boundary noun", err)
	}
	blankKey := base
	blankKey.EventKey = "  "
	if _, err := NewRegistryEventEnvelope(blankKey, paritySourceSystem); err == nil {
		t.Fatal("NewRegistryEventEnvelope(blank key) error = nil, want rejection")
	} else if !strings.Contains(err.Error(), "event key") {
		t.Fatalf("error = %q, want the key branch", err)
	}
}

// TestRatchetHostingEnvelopeRejections pins the hosting fail-closed
// branches: a blank boundary, provider, and registry.
func TestRatchetHostingEnvelopeRejections(t *testing.T) {
	t.Parallel()

	scope, generation, instance := ratchetBoundary()
	base := RepositoryHostingObservation{
		Provider: "acme", Registry: "https://registry.example", Repository: "main",
		ScopeID: scope, GenerationID: generation, CollectorInstanceID: instance,
	}
	blankBoundary := base
	blankBoundary.ScopeID = ""
	if _, err := NewRepositoryHostingEnvelope(blankBoundary, paritySourceSystem); err == nil {
		t.Fatal("NewRepositoryHostingEnvelope(blank scope) error = nil, want rejection")
	}
	blankProvider := base
	blankProvider.Provider = "  "
	if _, err := NewRepositoryHostingEnvelope(blankProvider, paritySourceSystem); err == nil {
		t.Fatal("NewRepositoryHostingEnvelope(blank provider) error = nil, want rejection")
	} else if !strings.Contains(err.Error(), "provider") {
		t.Fatalf("error = %q, want the provider branch", err)
	}
	blankRegistry := base
	blankRegistry.Registry = " "
	if _, err := NewRepositoryHostingEnvelope(blankRegistry, paritySourceSystem); err == nil {
		t.Fatal("NewRepositoryHostingEnvelope(blank registry) error = nil, want rejection")
	} else if !strings.Contains(err.Error(), "registry") {
		t.Fatalf("error = %q, want the registry branch", err)
	}
}

// TestRatchetVulnerabilityEnvelopeRejections pins the advisory fail-closed
// branches: an unnormalizable identity, a blank boundary, and a blank
// advisory id.
func TestRatchetVulnerabilityEnvelopeRejections(t *testing.T) {
	t.Parallel()

	scope, generation, instance := ratchetBoundary()
	base := VulnerabilityHintObservation{
		Package: ratchetIdentity("left-pad"), Version: "1.0", AdvisoryID: "ADV-1", AdvisorySource: "acme-sec",
		ScopeID: scope, GenerationID: generation, CollectorInstanceID: instance,
	}
	badIdentity := base
	badIdentity.Package = PackageIdentity{}
	if _, err := NewVulnerabilityHintEnvelope(badIdentity, paritySourceSystem); err == nil {
		t.Fatal("NewVulnerabilityHintEnvelope(bad identity) error = nil, want rejection")
	}
	blankBoundary := base
	blankBoundary.ScopeID = ""
	if _, err := NewVulnerabilityHintEnvelope(blankBoundary, paritySourceSystem); err == nil {
		t.Fatal("NewVulnerabilityHintEnvelope(blank scope) error = nil, want rejection")
	}
	blankAdvisory := base
	blankAdvisory.AdvisoryID = " "
	if _, err := NewVulnerabilityHintEnvelope(blankAdvisory, paritySourceSystem); err == nil {
		t.Fatal("NewVulnerabilityHintEnvelope(blank advisory) error = nil, want rejection")
	} else if !strings.Contains(err.Error(), "advisory_id") {
		t.Fatalf("error = %q, want the advisory branch", err)
	}
}

// TestRatchetSourceHintEnvelopeRejections pins the hint fail-closed
// branches: an unnormalizable package, a blank boundary, and a blank URL.
func TestRatchetSourceHintEnvelopeRejections(t *testing.T) {
	t.Parallel()

	scope, generation, instance := ratchetBoundary()
	base := SourceHintObservation{
		Package: ratchetIdentity("left-pad"), Version: "1.0", HintKind: "repository",
		RawURL: "https://github.com/example/x", NormalizedURL: "https://github.com/example/x",
		ScopeID: scope, GenerationID: generation, CollectorInstanceID: instance,
	}
	badPackage := base
	badPackage.Package = PackageIdentity{}
	if _, err := NewSourceHintEnvelope(badPackage, paritySourceSystem); err == nil {
		t.Fatal("NewSourceHintEnvelope(bad package) error = nil, want rejection")
	}
	blankBoundary := base
	blankBoundary.ScopeID = ""
	if _, err := NewSourceHintEnvelope(blankBoundary, paritySourceSystem); err == nil {
		t.Fatal("NewSourceHintEnvelope(blank scope) error = nil, want rejection")
	}
	blankURL := base
	blankURL.RawURL = ""
	blankURL.NormalizedURL = " "
	if _, err := NewSourceHintEnvelope(blankURL, paritySourceSystem); err == nil {
		t.Fatal("NewSourceHintEnvelope(blank URL) error = nil, want rejection")
	} else if !strings.Contains(err.Error(), "URL") {
		t.Fatalf("error = %q, want the URL branch", err)
	}
}

// TestRatchetWarningEnvelopeRejections pins the warning fail-closed
// branches plus the versioned success shape: blank boundary, key, and code,
// an unnormalizable package, and a versioned payload carrying its version
// id.
func TestRatchetWarningEnvelopeRejections(t *testing.T) {
	t.Parallel()

	scope, generation, instance := ratchetBoundary()
	identity := ratchetIdentity("left-pad")
	base := WarningObservation{
		WarningKey: "k", WarningCode: "c", Severity: "warning", Message: "m",
		Ecosystem: EcosystemNPM, Package: &identity,
		ScopeID: scope, GenerationID: generation, CollectorInstanceID: instance,
	}
	blankBoundary := base
	blankBoundary.ScopeID = ""
	if _, err := NewWarningEnvelope(blankBoundary, paritySourceSystem); err == nil {
		t.Fatal("NewWarningEnvelope(blank scope) error = nil, want rejection")
	}
	blankKey := base
	blankKey.WarningKey = "  "
	if _, err := NewWarningEnvelope(blankKey, paritySourceSystem); err == nil {
		t.Fatal("NewWarningEnvelope(blank key) error = nil, want rejection")
	} else if !strings.Contains(err.Error(), "warning key") {
		t.Fatalf("error = %q, want the key branch", err)
	}
	blankCode := base
	blankCode.WarningCode = " "
	if _, err := NewWarningEnvelope(blankCode, paritySourceSystem); err == nil {
		t.Fatal("NewWarningEnvelope(blank code) error = nil, want rejection")
	} else if !strings.Contains(err.Error(), "warning code") {
		t.Fatalf("error = %q, want the code branch", err)
	}
	badPackage := base
	bad := PackageIdentity{}
	badPackage.Package = &bad
	if _, err := NewWarningEnvelope(badPackage, paritySourceSystem); err == nil {
		t.Fatal("NewWarningEnvelope(bad package) error = nil, want rejection")
	}
	versioned := base
	versioned.Version = "1.0"
	fact, err := NewWarningEnvelope(versioned, paritySourceSystem)
	if err != nil {
		t.Fatalf("NewWarningEnvelope(versioned) error = %v", err)
	}
	versionID, ok := fact.Payload["version_id"].(string)
	if !ok {
		t.Fatalf("version_id = %#v, want a string", fact.Payload["version_id"])
	}
	if !strings.HasSuffix(versionID, "@1.0") {
		t.Fatalf("version_id = %q, want the version-qualified id", versionID)
	}
}

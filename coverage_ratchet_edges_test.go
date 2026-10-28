// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"errors"
	"strings"
	"testing"
)

// TestRatchetWarningErrorBranches pins the warning fail-closed branches: an
// unsupported-source warning that cannot be built reports its error, a
// parser warning with an unbuildable fact reports its error unhandled, and a
// native npm target is fetchable (never an unsupported source).
func TestRatchetWarningErrorBranches(t *testing.T) {
	t.Parallel()

	blank := FactContext{}
	native := FetchTarget{Base: TargetConfig{Ecosystem: EcosystemMaven, ScopeID: "maven://ratchet"}}
	if _, err := prefetchWarningFact(native, blank); err == nil {
		t.Fatal("prefetchWarningFact(unsupported, blank boundary) error = nil, want rejection")
	} else if !strings.Contains(err.Error(), "generation_id") {
		t.Fatalf("error = %q, want the boundary noun", err)
	}
	target := FetchTarget{Base: TargetConfig{Ecosystem: EcosystemCargo, ScopeID: "cargo://ratchet"}}
	if _, handled, err := parserWarningFact(target, blank, errors.New("boom")); err == nil || handled {
		t.Fatalf("parserWarningFact(unbuildable) = handled=%t err=%v, want unhandled error", handled, err)
	}
	fetchable := FetchTarget{
		Base:        TargetConfig{Ecosystem: EcosystemNPM, Visibility: VisibilityPublic, ScopeID: "npm://ratchet"},
		MetadataURL: "https://registry.example/left-pad",
	}
	if targetHasNoNativeMetadataSource(fetchable) {
		t.Fatal("targetHasNoNativeMetadataSource(npm with URL) = true, want false")
	}
	if !targetHasNoNativeMetadataSource(FetchTarget{Base: TargetConfig{Ecosystem: EcosystemGoModule}}) {
		t.Fatal("targetHasNoNativeMetadataSource(gomod without URL) = false, want true")
	}
	if targetHasNoNativeMetadataSource(FetchTarget{Base: TargetConfig{Ecosystem: EcosystemNPM}}) {
		t.Fatal("targetHasNoNativeMetadataSource(npm without URL) = true, want false: npm has a native adapter")
	}
	if targetHasNoNativeMetadataSource(FetchTarget{
		Base:           TargetConfig{Ecosystem: EcosystemGoModule},
		DocumentFormat: DocumentFormatArtifactoryPackage,
	}) {
		t.Fatal("targetHasNoNativeMetadataSource(artifactory format) = true, want false")
	}
}

// TestRatchetBoundVersionsDirectErrors pins the version-bound guards on
// direct calls: an unnormalizable package identity fails before any version
// filtering, and an empty package list with a bad identity fails the same
// way.
func TestRatchetBoundVersionsDirectErrors(t *testing.T) {
	t.Parallel()

	target := boundTestTarget()
	var bounded ParsedMetadata
	if _, _, err := boundVersions(target, ParsedMetadata{Packages: []PackageObservation{{Identity: PackageIdentity{}}}}, map[string]struct{}{}, &bounded); err == nil {
		t.Fatal("boundVersions(bad package identity) error = nil, want rejection")
	}
	if _, err := packageObservationsByID([]PackageObservation{{Identity: PackageIdentity{}}}); err == nil {
		t.Fatal("packageObservationsByID(bad identity) error = nil, want rejection")
	}
}

// TestRatchetBoundVersionsSkipsDisallowed pins the version filter through
// the full bound pass: with configured packages, a version for an
// unconfigured package drops while the configured version stays allowed.
func TestRatchetBoundVersionsSkipsDisallowed(t *testing.T) {
	t.Parallel()

	target := boundTestTarget()
	parsed := ParsedMetadata{
		Packages: []PackageObservation{boundTestPackage("left-pad")},
		Versions: []PackageVersionObservation{boundTestVersion("left-pad", "1.0"), boundTestVersion("other-package", "1.0")},
	}
	bounded, err := boundedParsedMetadata(target, parsed)
	if err != nil {
		t.Fatalf("boundedParsedMetadata() error = %v", err)
	}
	if len(bounded.Versions) != 1 || bounded.Versions[0].Version != "1.0" || bounded.Versions[0].Package.RawName != "left-pad" {
		t.Fatalf("versions = %+v, want only the configured package version", bounded.Versions)
	}
}

// TestRatchetArtifactoryWrapperBranches pins the wrapper edges: a missing
// package_type skips validation and parses natively, while native metadata
// the registry cannot parse fails the pass.
func TestRatchetArtifactoryWrapperBranches(t *testing.T) {
	t.Parallel()

	untyped := `{"repository":"npm-remote","metadata":{"name":"left-pad","versions":{"1.3.0":{}}}}`
	parsed, err := ParseArtifactoryPackageMetadata(ratchetParserContext(EcosystemNPM), []byte(untyped))
	if err != nil {
		t.Fatalf("ParseArtifactoryPackageMetadata(untyped) error = %v", err)
	}
	if len(parsed.Packages) != 1 {
		t.Fatalf("packages = %d, want 1 without a package_type", len(parsed.Packages))
	}
	badNative := `{"package_type":"npm","metadata":"not json"}`
	if _, err := ParseArtifactoryPackageMetadata(ratchetParserContext(EcosystemNPM), []byte(badNative)); err == nil {
		t.Fatal("ParseArtifactoryPackageMetadata(bad native) error = nil, want rejection")
	}
}

// TestRatchetAppendUniqueStringEdges pins the append guard: a blank value
// keeps the input, a duplicate keeps the input, and a fresh value appends.
func TestRatchetAppendUniqueStringEdges(t *testing.T) {
	t.Parallel()

	if got := appendUniqueString([]string{"a"}, ""); len(got) != 1 {
		t.Fatalf("blank append = %v, want unchanged", got)
	}
	if got := appendUniqueString([]string{"a"}, "a"); len(got) != 1 {
		t.Fatalf("duplicate append = %v, want unchanged", got)
	}
	if got := appendUniqueString(nil, "b"); len(got) != 1 || got[0] != "b" {
		t.Fatalf("fresh append = %v, want [b]", got)
	}
}

// TestRatchetNuGetMalformed pins the nuspec rejection: broken XML fails
// before identity checks.
func TestRatchetNuGetMalformed(t *testing.T) {
	t.Parallel()

	if _, err := ParseNuGetPackageMetadata(ratchetParserContext(EcosystemNuGet), []byte(`<oops`)); err == nil {
		t.Fatal("ParseNuGetPackageMetadata(malformed) error = nil, want rejection")
	}
}

// TestRatchetMustRegisterParserPanics pins the fail-fast guard: registering
// onto a registry that already holds the ecosystem panics instead of
// silently replacing the parser.
func TestRatchetMustRegisterParserPanics(t *testing.T) {
	t.Parallel()

	defer func() {
		if recover() == nil {
			t.Fatal("mustRegisterParser(duplicate) did not panic")
		}
	}()
	registry := DefaultMetadataParserRegistry()
	mustRegisterParser(&registry, EcosystemNPM, ParseNPMPackumentMetadata)
}

// TestRatchetEnvelopeBlankIdentifiers pins the remaining identifier guards:
// an advisory without a source and a hosting record without a repository
// both fail.
func TestRatchetEnvelopeBlankIdentifiers(t *testing.T) {
	t.Parallel()

	scope, generation, instance := ratchetBoundary()
	vulnerable := VulnerabilityHintObservation{
		Package: ratchetIdentity("left-pad"), Version: "1.0", AdvisoryID: "ADV-1", AdvisorySource: "  ",
		ScopeID: scope, GenerationID: generation, CollectorInstanceID: instance,
	}
	if _, err := NewVulnerabilityHintEnvelope(vulnerable, paritySourceSystem); err == nil {
		t.Fatal("NewVulnerabilityHintEnvelope(blank source) error = nil, want rejection")
	} else if !strings.Contains(err.Error(), "advisory_source") {
		t.Fatalf("error = %q, want the source branch", err)
	}
	hosting := RepositoryHostingObservation{
		Provider: "acme", Registry: "https://registry.example", Repository: "  ",
		ScopeID: scope, GenerationID: generation, CollectorInstanceID: instance,
	}
	if _, err := NewRepositoryHostingEnvelope(hosting, paritySourceSystem); err == nil {
		t.Fatal("NewRepositoryHostingEnvelope(blank repository) error = nil, want rejection")
	} else if !strings.Contains(err.Error(), "repository") {
		t.Fatalf("error = %q, want the repository branch", err)
	}
}

// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"testing"
	"time"
)

// npmBatchIdentity returns the package identity tuple used across the batch-3
// bounds tests.
func npmBatchIdentity(name string) PackageIdentity {
	return PackageIdentity{
		Ecosystem: EcosystemNPM,
		Registry:  "https://registry.npmjs.org",
		RawName:   name,
	}
}

// npmBatchTarget returns a bounded npm target with explicit package and
// version limits so every bound has a deterministic edge.
func npmBatchTarget(packages []string, packageLimit, versionLimit int) TargetConfig {
	return TargetConfig{
		Provider:     "npm",
		Ecosystem:    EcosystemNPM,
		Registry:     "https://registry.npmjs.org",
		ScopeID:      "npm://registry.npmjs.org/left-pad",
		Packages:     packages,
		PackageLimit: packageLimit,
		VersionLimit: versionLimit,
	}
}

// npmBatchParsed returns a two-package, three-version observation set: two
// versions for left-pad and one for right-pad.
func npmBatchParsed() ParsedMetadata {
	leftPad := npmBatchIdentity("left-pad")
	rightPad := npmBatchIdentity("right-pad")
	stamp := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	observation := func(identity PackageIdentity) PackageObservation {
		return PackageObservation{
			Identity:            identity,
			ScopeID:             "npm://registry.npmjs.org/left-pad",
			GenerationID:        "generation-1",
			CollectorInstanceID: "package-registry-qa",
			ObservedAt:          stamp,
		}
	}
	version := func(identity PackageIdentity, name string) PackageVersionObservation {
		return PackageVersionObservation{
			Package:             identity,
			Version:             name,
			ScopeID:             "npm://registry.npmjs.org/left-pad",
			GenerationID:        "generation-1",
			CollectorInstanceID: "package-registry-qa",
			ObservedAt:          stamp,
		}
	}
	return ParsedMetadata{
		Packages: []PackageObservation{observation(leftPad), observation(rightPad)},
		Versions: []PackageVersionObservation{
			version(leftPad, "1.0.0"),
			version(leftPad, "1.3.0"),
			version(leftPad, "2.0.0"),
			version(rightPad, "9.9.9"),
		},
	}
}

// TestBoundedParsedMetadataTruncatesVersionsWithWarning pins the version-limit
// path: the first N versions survive, the rest collapse into one truncation
// warning, and hosting passes through untouched.
func TestBoundedParsedMetadataTruncatesVersionsWithWarning(t *testing.T) {
	t.Parallel()

	parsed := npmBatchParsed()
	parsed.Hosting = []RepositoryHostingObservation{{Provider: "npm", Registry: "https://registry.npmjs.org"}}
	bounded, err := boundedParsedMetadata(npmBatchTarget(nil, 20, 2), parsed)
	if err != nil {
		t.Fatalf("boundedParsedMetadata() error = %v", err)
	}
	if len(bounded.Versions) != 3 {
		t.Fatalf("versions=%d, want 3 (2 for left-pad, 1 for right-pad)", len(bounded.Versions))
	}
	if len(bounded.Warnings) != 1 || bounded.Warnings[0].WarningCode != "version_limit_truncated" {
		t.Fatalf("warnings=%v, want one version_limit_truncated warning", bounded.Warnings)
	}
	if len(bounded.Hosting) != 1 {
		t.Fatalf("hosting=%d, want the hosting records passed through", len(bounded.Hosting))
	}
}

// TestBoundedParsedMetadataFiltersUnconfiguredPackages pins the configured
// filter: only listed packages and their versions survive.
func TestBoundedParsedMetadataFiltersUnconfiguredPackages(t *testing.T) {
	t.Parallel()

	bounded, err := boundedParsedMetadata(npmBatchTarget([]string{"left-pad"}, 20, 20), npmBatchParsed())
	if err != nil {
		t.Fatalf("boundedParsedMetadata() error = %v", err)
	}
	if len(bounded.Packages) != 1 {
		t.Fatalf("packages=%d, want only the configured package", len(bounded.Packages))
	}
	for _, version := range bounded.Versions {
		if version.Package.RawName != "left-pad" {
			t.Fatalf("version package=%q, want only left-pad versions", version.Package.RawName)
		}
	}
	if len(bounded.Versions) != 3 {
		t.Fatalf("versions=%d, want 3 left-pad versions", len(bounded.Versions))
	}
}

// TestBoundedParsedMetadataRejectsConfiguredMismatch pins the fail-closed
// filter: parsed metadata matching none of the configured packages is an
// error, not an empty success.
func TestBoundedParsedMetadataRejectsConfiguredMismatch(t *testing.T) {
	t.Parallel()

	if _, err := boundedParsedMetadata(npmBatchTarget([]string{"other-package"}, 20, 20), npmBatchParsed()); err == nil {
		t.Fatal("boundedParsedMetadata() must fail when nothing matches the configured packages")
	}
}

// TestBoundedParsedMetadataRejectsPackageLimit pins the package-count guard.
func TestBoundedParsedMetadataRejectsPackageLimit(t *testing.T) {
	t.Parallel()

	if _, err := boundedParsedMetadata(npmBatchTarget(nil, 1, 20), npmBatchParsed()); err == nil {
		t.Fatal("boundedParsedMetadata() must fail past the package limit")
	}
}

// TestBoundedParsedMetadataRejectsUnnormalizableObservations pins the
// fail-closed identity checks in every bound stage: an observation whose
// identity cannot normalize aborts the whole bound with an error.
func TestBoundedParsedMetadataRejectsUnnormalizableObservations(t *testing.T) {
	t.Parallel()

	blankName := npmBatchIdentity("")
	versionStage := npmBatchParsed()
	versionStage.Versions[0].Package = blankName
	dependencyStage := npmBatchParsed()
	dependencyStage.Dependencies = []PackageDependencyObservation{{
		Package: blankName, Version: "1.0.0",
		Dependency: npmBatchIdentity("dep"),
		ScopeID:    "npm://registry.npmjs.org/left-pad", GenerationID: "generation-1",
		CollectorInstanceID: "package-registry-qa",
	}}
	warningStage := npmBatchParsed()
	badPackage := blankName
	warningStage.Warnings = []WarningObservation{{
		WarningKey: "k", WarningCode: "c", Package: &badPackage,
		ScopeID: "npm://registry.npmjs.org/left-pad", GenerationID: "generation-1",
		CollectorInstanceID: "package-registry-qa",
	}}

	for _, tc := range []struct {
		name   string
		parsed ParsedMetadata
	}{
		{name: "versions", parsed: versionStage},
		{name: "dependencies", parsed: dependencyStage},
		{name: "warnings", parsed: warningStage},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if _, err := boundedParsedMetadata(npmBatchTarget(nil, 20, 20), tc.parsed); err == nil {
				t.Fatalf("boundedParsedMetadata(%s) must fail for an unnormalizable identity", tc.name)
			}
		})
	}
}

// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"strings"
	"testing"
)

// boundTestTarget returns a validated-shape target: npm left-pad with roomy
// bounds. Tests narrow the limits they pin.
func boundTestTarget() TargetConfig {
	return TargetConfig{
		Provider:     "npm",
		Ecosystem:    EcosystemNPM,
		Registry:     "https://registry.npmjs.org",
		ScopeID:      "npm://registry.npmjs.org/left-pad",
		Packages:     []string{"left-pad"},
		PackageLimit: 20,
		VersionLimit: 20,
	}
}

// boundTestPackage returns one package observation for rawName, and
// boundTestVersion returns one version observation for rawName at version.
func boundTestPackage(rawName string) PackageObservation {
	return PackageObservation{Identity: ratchetIdentity(rawName), ScopeID: "npm://ratchet-scope"}
}

func boundTestVersion(rawName, version string) PackageVersionObservation {
	return PackageVersionObservation{Package: ratchetIdentity(rawName), Version: version, ScopeID: "npm://ratchet-scope"}
}

// TestRatchetConfiguredPackageIDsError pins fail-closed configuration: a
// package the identity rules reject fails the whole bound pass.
func TestRatchetConfiguredPackageIDsError(t *testing.T) {
	t.Parallel()

	target := boundTestTarget()
	target.Registry = ""
	if _, err := configuredPackageIDs(target); err == nil {
		t.Fatal("configuredPackageIDs(blank registry) error = nil, want rejection")
	} else if !strings.Contains(err.Error(), "package registry") {
		t.Fatalf("error = %q, want the registry branch", err)
	}
	if _, err := boundedParsedMetadata(target, ParsedMetadata{Packages: []PackageObservation{boundTestPackage("left-pad")}}); err == nil {
		t.Fatal("boundedParsedMetadata(blank registry) error = nil, want rejection")
	}
}

// TestRatchetBoundPackagesErrors pins the package bound failures: an
// unnormalizable observed identity, a document disjoint from the configured
// packages, and a package count over the limit.
func TestRatchetBoundPackagesErrors(t *testing.T) {
	t.Parallel()

	target := boundTestTarget()
	bad := ParsedMetadata{Packages: []PackageObservation{{Identity: PackageIdentity{}}}}
	if _, err := boundedParsedMetadata(target, bad); err == nil {
		t.Fatal("boundedParsedMetadata(bad identity) error = nil, want rejection")
	}
	disjoint := ParsedMetadata{Packages: []PackageObservation{boundTestPackage("other-package")}}
	if _, err := boundedParsedMetadata(target, disjoint); err == nil {
		t.Fatal("boundedParsedMetadata(disjoint) error = nil, want rejection")
	} else if !strings.Contains(err.Error(), "does not match configured packages") {
		t.Fatalf("error = %q, want the mismatch branch", err)
	}
	limited := boundTestTarget()
	limited.PackageLimit = 0
	if _, err := boundedParsedMetadata(limited, ParsedMetadata{Packages: []PackageObservation{boundTestPackage("left-pad")}}); err == nil {
		t.Fatal("boundedParsedMetadata(over limit) error = nil, want rejection")
	} else if !strings.Contains(err.Error(), "exceeds package_limit") {
		t.Fatalf("error = %q, want the limit branch", err)
	}
}

// TestRatchetBoundPackagesSkipsUnconfigured pins the configured filter: with
// configured packages, an unlisted observed package drops while a listed one
// stays allowed.
func TestRatchetBoundPackagesSkipsUnconfigured(t *testing.T) {
	t.Parallel()

	target := boundTestTarget()
	parsed := ParsedMetadata{Packages: []PackageObservation{boundTestPackage("left-pad"), boundTestPackage("other-package")}}
	configuredID, err := packageID(ratchetIdentity("left-pad"))
	if err != nil {
		t.Fatalf("packageID() error = %v", err)
	}
	bounded, allowed, err := boundPackages(target, parsed, map[string]struct{}{configuredID: {}})
	if err != nil {
		t.Fatalf("boundPackages() error = %v", err)
	}
	if len(bounded.Packages) != 1 {
		t.Fatalf("packages = %d, want only the configured one", len(bounded.Packages))
	}
	if len(allowed) != 1 {
		t.Fatalf("allowed = %d, want 1", len(allowed))
	}
}

// TestRatchetBoundVersionsErrorsAndTruncation pins the version bound: a bad
// version identity fails, a disallowed version drops, and versions past the
// limit truncate with a version_limit_truncated warning first in line.
func TestRatchetBoundVersionsErrorsAndTruncation(t *testing.T) {
	t.Parallel()

	target := boundTestTarget()
	badVersions := ParsedMetadata{
		Packages: []PackageObservation{boundTestPackage("left-pad")},
		Versions: []PackageVersionObservation{{Package: PackageIdentity{}, Version: "1.0"}},
	}
	if _, err := boundedParsedMetadata(target, badVersions); err == nil {
		t.Fatal("boundedParsedMetadata(bad version identity) error = nil, want rejection")
	}

	versioned := ParsedMetadata{
		Packages: []PackageObservation{boundTestPackage("left-pad")},
		Versions: []PackageVersionObservation{boundTestVersion("left-pad", "2.0"), boundTestVersion("left-pad", "1.0")},
		Dependencies: []PackageDependencyObservation{
			{Package: ratchetIdentity("left-pad"), Version: "2.0", Dependency: ratchetIdentity("dep")},
			{Package: ratchetIdentity("other-package"), Version: "1.0", Dependency: ratchetIdentity("dep")},
		},
	}
	truncated := boundTestTarget()
	truncated.VersionLimit = 1
	bounded, err := boundedParsedMetadata(truncated, versioned)
	if err != nil {
		t.Fatalf("boundedParsedMetadata() error = %v", err)
	}
	if len(bounded.Versions) != 1 || bounded.Versions[0].Version != "2.0" {
		t.Fatalf("versions = %+v, want only the first", bounded.Versions)
	}
	if len(bounded.Warnings) != 1 || bounded.Warnings[0].WarningCode != "version_limit_truncated" {
		t.Fatalf("warnings = %+v, want the truncation warning first", bounded.Warnings)
	}
	if len(bounded.Dependencies) != 1 || bounded.Dependencies[0].Version != "2.0" {
		t.Fatalf("dependencies = %+v, want only the kept version", bounded.Dependencies)
	}
}

// TestRatchetVersionLimitWarningsFallback pins the warning attribution: with
// a package observation the warning carries its boundary, without one it
// falls back to the version observation boundary.
func TestRatchetVersionLimitWarningsFallback(t *testing.T) {
	t.Parallel()

	target := boundTestTarget()
	target.VersionLimit = 1
	identity, err := NormalizePackageIdentity(ratchetIdentity("left-pad"))
	if err != nil {
		t.Fatalf("NormalizePackageIdentity() error = %v", err)
	}
	packageID := identity.PackageID
	version := boundTestVersion("left-pad", "2.0")
	version.ScopeID = "npm://version-scope"
	version.GenerationID = "generation-9"
	counts := map[string]int{packageID: 3}
	allowed := map[string]struct{}{packageID: {}}

	direct := versionLimitWarnings(target, allowed, counts,
		map[string]PackageObservation{packageID: boundTestPackage("left-pad")},
		map[string]PackageVersionObservation{packageID: version})
	if len(direct) != 1 || direct[0].ScopeID != "npm://ratchet-scope" {
		t.Fatalf("direct warnings = %+v, want the package boundary", direct)
	}
	fallback := versionLimitWarnings(target, allowed, counts,
		map[string]PackageObservation{},
		map[string]PackageVersionObservation{packageID: version})
	if len(fallback) != 1 || fallback[0].ScopeID != "npm://version-scope" || fallback[0].GenerationID != "generation-9" {
		t.Fatalf("fallback warnings = %+v, want the version boundary", fallback)
	}
	under := versionLimitWarnings(target, allowed, map[string]int{packageID: 1},
		map[string]PackageObservation{}, map[string]PackageVersionObservation{})
	if len(under) != 0 {
		t.Fatalf("under-limit warnings = %+v, want none", under)
	}
}

// TestRatchetKeepPackageVersionObservation pins the keep predicate: a bad
// identity errors, an unallowed package drops, a blank version on an allowed
// package keeps, a version with no allowlist drops, and a listed version
// keeps.
func TestRatchetKeepPackageVersionObservation(t *testing.T) {
	t.Parallel()

	identity, err := NormalizePackageIdentity(ratchetIdentity("left-pad"))
	if err != nil {
		t.Fatalf("NormalizePackageIdentity() error = %v", err)
	}
	packageID := identity.PackageID
	allowed := map[string]struct{}{packageID: {}}
	allowedVersions := map[string]struct{}{versionKey(packageID, "1.0"): {}}

	if _, err := keepPackageVersionObservation(PackageIdentity{}, "1.0", allowed, allowedVersions); err == nil {
		t.Fatal("keep(bad identity) error = nil, want rejection")
	}
	if keep, err := keepPackageVersionObservation(ratchetIdentity("other"), "1.0", allowed, allowedVersions); err != nil || keep {
		t.Fatalf("keep(unallowed) = %t, %v, want false, nil", keep, err)
	}
	if keep, err := keepPackageVersionObservation(ratchetIdentity("left-pad"), "  ", allowed, allowedVersions); err != nil || !keep {
		t.Fatalf("keep(blank version) = %t, %v, want true, nil", keep, err)
	}
	if keep, err := keepPackageVersionObservation(ratchetIdentity("left-pad"), "1.0", allowed, map[string]struct{}{}); err != nil || keep {
		t.Fatalf("keep(empty allowlist) = %t, %v, want false, nil", keep, err)
	}
	if keep, err := keepPackageVersionObservation(ratchetIdentity("left-pad"), "1.0", allowed, allowedVersions); err != nil || !keep {
		t.Fatalf("keep(listed) = %t, %v, want true, nil", keep, err)
	}
	if keep, err := keepPackageVersionObservation(ratchetIdentity("left-pad"), "2.0", allowed, allowedVersions); err != nil || keep {
		t.Fatalf("keep(unlisted version) = %t, %v, want false, nil", keep, err)
	}
}

// TestRatchetBoundObservationsPropagateIdentityErrors pins the per-kind
// bound loops: a bad identity in dependencies, artifacts, source hints,
// vulnerabilities, events, or warnings fails the pass with the error.
func TestRatchetBoundObservationsPropagateIdentityErrors(t *testing.T) {
	t.Parallel()

	target := boundTestTarget()
	base := ParsedMetadata{Packages: []PackageObservation{boundTestPackage("left-pad")}}
	bad := PackageIdentity{}
	cases := map[string]ParsedMetadata{
		"dependencies": {Packages: base.Packages, Dependencies: []PackageDependencyObservation{{Package: bad}}},
		"artifacts":    {Packages: base.Packages, Artifacts: []PackageArtifactObservation{{Package: bad}}},
		"sourcehints":  {Packages: base.Packages, SourceHints: []SourceHintObservation{{Package: bad}}},
		"vulnerables":  {Packages: base.Packages, Vulnerables: []VulnerabilityHintObservation{{Package: bad}}},
		"events":       {Packages: base.Packages, Events: []RegistryEventObservation{{Package: bad}}},
		"warnings":     {Packages: base.Packages, Warnings: []WarningObservation{{Package: &bad}}},
	}
	for name, parsed := range cases {
		if _, err := boundedParsedMetadata(target, parsed); err == nil {
			t.Errorf("boundedParsedMetadata(bad %s identity) error = nil, want rejection", name)
		}
	}
}

// TestRatchetBoundWarningsKeepsPackageless pins the warning bound: warnings
// without a package always keep, while packaged warnings filter on the
// allowlists.
func TestRatchetBoundWarningsKeepsPackageless(t *testing.T) {
	t.Parallel()

	identity, err := NormalizePackageIdentity(ratchetIdentity("left-pad"))
	if err != nil {
		t.Fatalf("NormalizePackageIdentity() error = %v", err)
	}
	packageID := identity.PackageID
	allowed := map[string]struct{}{packageID: {}}
	allowedVersions := map[string]struct{}{versionKey(packageID, "1.0"): {}}
	packaged := ratchetIdentity("left-pad")
	warnings := []WarningObservation{
		{WarningKey: "global"},
		{WarningKey: "kept", Package: &packaged, Version: "1.0"},
		{WarningKey: "dropped", Package: &packaged, Version: "9.9"},
	}
	bounded, err := boundWarnings(warnings, allowed, allowedVersions)
	if err != nil {
		t.Fatalf("boundWarnings() error = %v", err)
	}
	if len(bounded) != 2 || bounded[0].WarningKey != "global" || bounded[1].WarningKey != "kept" {
		t.Fatalf("bounded = %+v, want global plus kept", bounded)
	}
}

// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"reflect"
	"strings"
	"testing"
)

func derivedTestConfig(ecosystems ...Ecosystem) DerivedTargetConfig {
	return normalizeDerivedTargetConfig(DerivedTargetConfig{
		Enabled:      true,
		Ecosystems:   ecosystems,
		PackageLimit: 7,
		VersionLimit: 9,
	})
}

// TestNormalizeDerivedTargetConfigDisabled pins the disabled fast path: a
// disabled config normalizes to the zero value regardless of contents.
func TestNormalizeDerivedTargetConfigDisabled(t *testing.T) {
	t.Parallel()

	got := normalizeDerivedTargetConfig(DerivedTargetConfig{
		Enabled:      false,
		Ecosystems:   []Ecosystem{EcosystemNPM},
		PackageLimit: 5,
		VersionLimit: 5,
	})
	if !reflect.DeepEqual(got, DerivedTargetConfig{}) {
		t.Fatalf("normalizeDerivedTargetConfig(disabled) = %+v, want zero value", got)
	}
}

// TestNormalizeDerivedTargetConfigDedupesAndDefaults pins dedupe, blank
// filtering, and the limit floors.
func TestNormalizeDerivedTargetConfigDedupesAndDefaults(t *testing.T) {
	t.Parallel()

	got := normalizeDerivedTargetConfig(DerivedTargetConfig{
		Enabled:    true,
		Ecosystems: []Ecosystem{EcosystemNPM, EcosystemNPM, "", EcosystemPyPI},
	})
	if !got.Enabled {
		t.Fatal("normalizeDerivedTargetConfig() Enabled = false, want true")
	}
	if len(got.Ecosystems) != 2 || got.Ecosystems[0] != EcosystemNPM || got.Ecosystems[1] != EcosystemPyPI {
		t.Fatalf("Ecosystems = %v, want [npm pypi]", got.Ecosystems)
	}
	if got.PackageLimit != 1 || got.VersionLimit != 1 {
		t.Fatalf("limits = %d/%d, want 1/1 floors", got.PackageLimit, got.VersionLimit)
	}
}

// TestDerivedTargetForScopeExplicitHit pins the explicit-target precedence:
// a configured scope wins without consulting derivation.
func TestDerivedTargetForScopeExplicitHit(t *testing.T) {
	t.Parallel()

	explicit := FetchTarget{Base: TargetConfig{Provider: "npm", ScopeID: "npm://registry.npmjs.org/left-pad"}}
	got, derived, err := derivedTargetForScope(
		map[string]FetchTarget{"npm://registry.npmjs.org/left-pad": explicit},
		derivedTestConfig(EcosystemNPM),
		"  npm://registry.npmjs.org/left-pad  ",
	)
	if err != nil {
		t.Fatalf("derivedTargetForScope() error = %v", err)
	}
	if derived {
		t.Fatal("derived = true, want false for an explicit target")
	}
	if got.Base.ScopeID != explicit.Base.ScopeID {
		t.Fatalf("ScopeID = %q, want %q", got.Base.ScopeID, explicit.Base.ScopeID)
	}
}

// TestDerivedTargetForScopeErrors pins both miss paths: derivation disabled
// and derivation enabled but unresolvable.
func TestDerivedTargetForScopeErrors(t *testing.T) {
	t.Parallel()

	const scopeID = "npm://registry.npmjs.org/left-pad"
	if _, _, err := derivedTargetForScope(nil, DerivedTargetConfig{}, scopeID); err == nil {
		t.Fatal("derivedTargetForScope(disabled) error = nil, want rejection")
	} else if !strings.Contains(err.Error(), "is not configured") {
		t.Fatalf("error %q must report the scope as unconfigured", err)
	}
	if _, _, err := derivedTargetForScope(nil, derivedTestConfig(EcosystemPyPI), scopeID); err == nil {
		t.Fatal("derivedTargetForScope(unresolvable) error = nil, want rejection")
	}
}

// TestDerivedTargetForScopeDerives pins the derived hit: the returned flag
// marks the target as derived and limits flow through.
func TestDerivedTargetForScopeDerives(t *testing.T) {
	t.Parallel()

	const scopeID = "npm://registry.npmjs.org/left-pad"
	got, derived, err := derivedTargetForScope(nil, derivedTestConfig(EcosystemNPM), scopeID)
	if err != nil {
		t.Fatalf("derivedTargetForScope() error = %v", err)
	}
	if !derived {
		t.Fatal("derived = false, want true for a derived target")
	}
	if got.Base.ScopeID != scopeID || got.Base.PackageLimit != 7 || got.Base.VersionLimit != 9 {
		t.Fatalf("target = %+v, want scope passthrough with limits 7/9", got.Base)
	}
}

// TestDerivedNPMTargetEdges pins the npm derivation guards.
func TestDerivedNPMTargetEdges(t *testing.T) {
	t.Parallel()

	const scopeID = "npm://registry.npmjs.org/left-pad"
	config := derivedTestConfig(EcosystemNPM)
	got, ok := derivedNPMTarget(scopeID, config)
	if !ok {
		t.Fatal("derivedNPMTarget(valid) = false, want true")
	}
	if got.MetadataURL != "https://registry.npmjs.org/left-pad" {
		t.Fatalf("MetadataURL = %q, want the registry metadata URL", got.MetadataURL)
	}
	if len(got.Base.Packages) != 1 || got.Base.Packages[0] != "left-pad" {
		t.Fatalf("Packages = %v, want [left-pad]", got.Base.Packages)
	}
	if got.Base.Namespace != "" {
		t.Fatalf("Namespace = %q, want empty for an unscoped package", got.Base.Namespace)
	}

	for name, tc := range map[string]struct {
		scopeID string
		config  DerivedTargetConfig
	}{
		"ecosystem disabled":      {scopeID, derivedTestConfig(EcosystemPyPI)},
		"wrong scheme":            {"pypi://pypi.org/pypi/left-pad", config},
		"wrong host":              {"npm://example.com/left-pad", config},
		"blank name":              {"npm://registry.npmjs.org/  ", config},
		"unnormalized case":       {"npm://registry.npmjs.org/Left-Pad", config},
		"not a url":               {"npm://registry.npmjs.org/\x7f", config},
		"scoped escaped mismatch": {"npm://registry.npmjs.org/%40scope%2Fname", config},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, ok := derivedNPMTarget(tc.scopeID, tc.config); ok {
				t.Fatalf("derivedNPMTarget(%q) = true, want false", tc.scopeID)
			}
		})
	}
}

// TestDerivedNPMTargetScoped pins scoped-name derivation including the
// namespace split and escaped metadata URL.
func TestDerivedNPMTargetScoped(t *testing.T) {
	t.Parallel()

	got, ok := derivedNPMTarget("npm://registry.npmjs.org/@scope/name", derivedTestConfig(EcosystemNPM))
	if !ok {
		t.Fatal("derivedNPMTarget(scoped) = false, want true")
	}
	if got.Base.Namespace != "scope" {
		t.Fatalf("Namespace = %q, want scope", got.Base.Namespace)
	}
	if got.MetadataURL != "https://registry.npmjs.org/@scope%2Fname" {
		t.Fatalf("MetadataURL = %q, want the escaped scoped registry URL", got.MetadataURL)
	}
}

// TestDerivedPyPITargetEdges pins the PyPI derivation guards.
func TestDerivedPyPITargetEdges(t *testing.T) {
	t.Parallel()

	const scopeID = "pypi://pypi.org/pypi/requests"
	got, ok := derivedPyPITarget(scopeID, derivedTestConfig(EcosystemPyPI))
	if !ok {
		t.Fatal("derivedPyPITarget(valid) = false, want true")
	}
	if got.MetadataURL != "https://pypi.org/pypi/requests/json" {
		t.Fatalf("MetadataURL = %q, want the PyPI JSON URL", got.MetadataURL)
	}
	if got.Base.Provider != "pypi" {
		t.Fatalf("Provider = %q, want pypi", got.Base.Provider)
	}

	for name, tc := range map[string]struct {
		scopeID string
		config  DerivedTargetConfig
	}{
		"ecosystem disabled": {scopeID, derivedTestConfig(EcosystemNPM)},
		"wrong scheme":       {"npm://registry.npmjs.org/requests", derivedTestConfig(EcosystemPyPI)},
		"empty name":         {"pypi://pypi.org/pypi/   ", derivedTestConfig(EcosystemPyPI)},
		"unnormalized name":  {"pypi://pypi.org/pypi/Requests", derivedTestConfig(EcosystemPyPI)},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, ok := derivedPyPITarget(tc.scopeID, tc.config); ok {
				t.Fatalf("derivedPyPITarget(%q) = true, want false", tc.scopeID)
			}
		})
	}
}

// TestDerivedUnsupportedTargetEcosystems pins one successful derivation per
// supported non-npm/pypi ecosystem plus the disabled and miss guards.
func TestDerivedUnsupportedTargetEcosystems(t *testing.T) {
	t.Parallel()

	all := []Ecosystem{
		EcosystemGoModule, EcosystemMaven, EcosystemNuGet,
		EcosystemComposer, EcosystemRubyGems, EcosystemCargo,
	}
	config := derivedTestConfig(all...)
	cases := []string{
		"gomod://proxy.golang.org/example.com/mod",
		"maven://repo.maven.apache.org/maven2/org.apache:commons-lang3",
		"nuget://api.nuget.org/v3/index.json/newtonsoft.json",
		"composer://repo.packagist.org/vendor/package",
		"rubygems://rubygems.org/rails",
		"cargo://crates.io/serde",
	}
	for _, scopeID := range cases {
		got, ok := derivedUnsupportedTarget(scopeID, config)
		if !ok {
			t.Fatalf("derivedUnsupportedTarget(%q) = false, want true", scopeID)
		}
		if got.Base.ScopeID != scopeID {
			t.Fatalf("ScopeID = %q, want %q", got.Base.ScopeID, scopeID)
		}
		if got.Base.SourceURI != scopeID {
			t.Fatalf("SourceURI = %q, want the scope ID", got.Base.SourceURI)
		}
		if got.Base.Ecosystem == "" || got.Base.Provider == "" {
			t.Fatalf("ecosystem/provider = %q/%q, want both set", got.Base.Ecosystem, got.Base.Provider)
		}
	}
	if _, ok := derivedUnsupportedTarget("npm://registry.npmjs.org/left-pad", config); ok {
		t.Fatal("derivedUnsupportedTarget(npm scope) = true, want false")
	}
	if _, ok := derivedUnsupportedTarget("gomod://proxy.golang.org/example.com/mod", derivedTestConfig(EcosystemNPM)); ok {
		t.Fatal("derivedUnsupportedTarget(disabled ecosystem) = true, want false")
	}
	if _, ok := derivedUnsupportedTarget("bogus-scope", config); ok {
		t.Fatal("derivedUnsupportedTarget(garbage) = true, want false")
	}
}

// TestDerivedTargetDispatchCoversFallback pins the derivedTarget fallback
// order: an npm miss falls through to a PyPI hit, and total misses reach the
// unsupported path. It also pins the maven colon-form round-trip guard and
// the empty-ecosystem default.
func TestDerivedTargetDispatchCoversFallback(t *testing.T) {
	t.Parallel()

	config := derivedTestConfig(EcosystemNPM, EcosystemPyPI)
	got, ok := derivedTarget("pypi://pypi.org/pypi/requests", config)
	if !ok {
		t.Fatal("derivedTarget(pypi scope) = false, want the PyPI fallback hit")
	}
	if got.Base.Provider != "pypi" {
		t.Fatalf("Provider = %q, want pypi", got.Base.Provider)
	}
	if _, ok := derivedTarget("bogus-scope", config); ok {
		t.Fatal("derivedTarget(garbage) = true, want false")
	}
	if _, ok := normalizedDerivedIdentity("maven://repo.maven.apache.org/maven2/nocolon", EcosystemMaven); ok {
		t.Fatal("normalizedDerivedIdentity(maven without colon) = true, want false")
	}
	mavenIdentity, ok := normalizedDerivedIdentity("maven://repo.maven.apache.org/maven2/org.apache:commons-lang3", EcosystemMaven)
	if !ok {
		t.Fatal("normalizedDerivedIdentity(maven group:artifact) = false, want true")
	}
	if mavenIdentity.Namespace != "org.apache" || mavenIdentity.NormalizedName != "commons-lang3" {
		t.Fatalf("maven identity = %+v, want group org.apache and commons-lang3", mavenIdentity)
	}
	defaulted := normalizeDerivedTargetConfig(DerivedTargetConfig{Enabled: true})
	if !defaulted.Enabled || len(defaulted.Ecosystems) != 1 || defaulted.Ecosystems[0] != EcosystemNPM {
		t.Fatalf("empty ecosystems = %+v, want enabled with [npm]", defaulted)
	}
}

// TestNormalizedDerivedIdentityGuards pins the scheme and round-trip guards.
func TestNormalizedDerivedIdentityGuards(t *testing.T) {
	t.Parallel()

	if _, ok := normalizedDerivedIdentity("npm://registry.npmjs.org/left-pad", EcosystemPyPI); ok {
		t.Fatal("normalizedDerivedIdentity(scheme mismatch) = true, want false")
	}
	if _, ok := normalizedDerivedIdentity("pypi://pypi.org/pypi/Requests", EcosystemPyPI); ok {
		t.Fatal("normalizedDerivedIdentity(unnormalized) = true, want false")
	}
	if _, ok := normalizedDerivedIdentity("not a url", EcosystemPyPI); ok {
		t.Fatal("normalizedDerivedIdentity(garbage) = true, want false")
	}
	got, ok := normalizedDerivedIdentity("pypi://pypi.org/pypi/requests", EcosystemPyPI)
	if !ok {
		t.Fatal("normalizedDerivedIdentity(valid) = false, want true")
	}
	if got.NormalizedName != "requests" || got.Registry != "pypi.org/pypi" {
		t.Fatalf("identity = %+v, want requests on pypi.org/pypi", got)
	}
}

// TestDerivedRegistryAndRawPathPrefixes pins every known prefix, the
// pass-through rejections, and the blank/escape guards.
func TestDerivedRegistryAndRawPathPrefixes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		eco      Ecosystem
		scopeID  string
		registry string
		rawPath  string
	}{
		{"pypi", EcosystemPyPI, "pypi://pypi.org/pypi/requests", "https://pypi.org/pypi", "requests"},
		{"gomod", EcosystemGoModule, "gomod://proxy.golang.org/example.com/mod", "https://proxy.golang.org", "example.com/mod"},
		{"maven", EcosystemMaven, "maven://repo.maven.apache.org/maven2/org.apache:commons-lang3", "https://repo.maven.apache.org/maven2", "org.apache:commons-lang3"},
		{"nuget", EcosystemNuGet, "nuget://api.nuget.org/v3/index.json/newtonsoft.json", "https://api.nuget.org/v3/index.json", "newtonsoft.json"},
		{"composer", EcosystemComposer, "composer://repo.packagist.org/vendor/package", "https://repo.packagist.org", "vendor/package"},
		{"rubygems", EcosystemRubyGems, "rubygems://rubygems.org/rails", "https://rubygems.org", "rails"},
		{"cargo", EcosystemCargo, "cargo://crates.io/serde", "https://crates.io", "serde"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			registry, rawPath, ok := derivedRegistryAndRawPath(tc.scopeID, tc.eco)
			if !ok || registry != tc.registry || rawPath != tc.rawPath {
				t.Fatalf("derivedRegistryAndRawPath(%q) = %q, %q, %v; want %q, %q, true",
					tc.scopeID, registry, rawPath, ok, tc.registry, tc.rawPath)
			}
		})
	}

	registry, rawPath, ok := derivedRegistryAndRawPath("pypi://pypi.org/pypi/requests", EcosystemPyPI)
	if !ok || registry != "https://pypi.org/pypi" || rawPath != "requests" {
		t.Fatalf("derivedRegistryAndRawPath(pypi) = %q, %q, %v; want registry, requests, true", registry, rawPath, ok)
	}
	if _, _, ok := derivedRegistryAndRawPath("pypi://pypi.org/pypi/requests", EcosystemNPM); ok {
		t.Fatal("derivedRegistryAndRawPath(ecosystem mismatch) = true, want false")
	}
	if _, _, ok := derivedRegistryAndRawPath("pypi://other.example/requests", EcosystemPyPI); ok {
		t.Fatal("derivedRegistryAndRawPath(unknown prefix) = true, want false")
	}
	if _, _, ok := derivedRegistryAndRawPath("pypi://pypi.org/pypi/   ", EcosystemPyPI); ok {
		t.Fatal("derivedRegistryAndRawPath(blank path) = true, want false")
	}
	if _, _, ok := derivedRegistryAndRawPath("pypi://pypi.org/pypi/%zz", EcosystemPyPI); ok {
		t.Fatal("derivedRegistryAndRawPath(bad escape) = true, want false")
	}
}

// TestRawNameAndNamespaceFromDerivedPathEdges pins blank, passthrough, and
// maven group:artifact handling.
func TestRawNameAndNamespaceFromDerivedPathEdges(t *testing.T) {
	t.Parallel()

	if _, _, ok := rawNameAndNamespaceFromDerivedPath("   ", EcosystemPyPI); ok {
		t.Fatal("rawNameAndNamespaceFromDerivedPath(blank) = true, want false")
	}
	raw, ns, ok := rawNameAndNamespaceFromDerivedPath("requests", EcosystemPyPI)
	if !ok || raw != "requests" || ns != "" {
		t.Fatalf("pypi = %q, %q, %v; want requests, empty, true", raw, ns, ok)
	}
	raw, ns, ok = rawNameAndNamespaceFromDerivedPath("org.apache:commons-lang3", EcosystemMaven)
	if !ok || raw != "commons-lang3" || ns != "org.apache" {
		t.Fatalf("maven = %q, %q, %v; want commons-lang3, org.apache, true", raw, ns, ok)
	}
	if _, _, ok := rawNameAndNamespaceFromDerivedPath("no-colon-here", EcosystemMaven); ok {
		t.Fatal("rawNameAndNamespaceFromDerivedPath(maven without colon) = true, want false")
	}
	raw, ns, ok = rawNameAndNamespaceFromDerivedPath("  org : name  ", EcosystemMaven)
	if !ok || raw != "name" || ns != "org" {
		t.Fatalf("maven trims = %q, %q, %v; want name, org, true", raw, ns, ok)
	}
}

// TestNormalizeArtifactoryPackageTypeAliases pins every alias branch,
// case/space tolerance, and the unknown passthrough.
func TestNormalizeArtifactoryPackageTypeAliases(t *testing.T) {
	t.Parallel()

	cases := map[string]Ecosystem{
		"":          "",
		"   ":       "",
		"npm":       EcosystemNPM,
		" NPM ":     EcosystemNPM,
		"pypi":      EcosystemPyPI,
		"python":    EcosystemPyPI,
		"Python":    EcosystemPyPI,
		"go":        EcosystemGoModule,
		"gomod":     EcosystemGoModule,
		"go-module": EcosystemGoModule,
		"go_module": EcosystemGoModule,
		"maven":     EcosystemMaven,
		"gradle":    EcosystemMaven,
		"nuget":     EcosystemNuGet,
		"NuGet":     EcosystemNuGet,
		"generic":   EcosystemGeneric,
		"GENERIC":   EcosystemGeneric,
		"docker":    Ecosystem("docker"),
		" Docker ":  Ecosystem("Docker"),
		"conan":     Ecosystem("conan"),
	}
	for input, want := range cases {
		if got := normalizeArtifactoryPackageType(input); got != want {
			t.Fatalf("normalizeArtifactoryPackageType(%q) = %q, want %q", input, got, want)
		}
	}
}

// TestParseGoModRequiresEdges pins block, single-line, and skip behaviors.
func TestParseGoModRequiresEdges(t *testing.T) {
	t.Parallel()

	mod := "module example.com/mod\n" +
		"\n" +
		"go 1.21\n" +
		"\n" +
		"require (\n" +
		"\texample.com/direct v1.2.3\n" +
		"\texample.com/indirect v0.9.0 // indirect\n" +
		")\n" +
		"require example.com/single v2.0.0\n" +
		"require example.com/commented v3.0.0 // some comment\n" +
		"// a full-line comment\n" +
		"require\n" +
		"require (\n" +
		"\t// empty block comment only\n" +
		")\n"
	requirements := parseGoModRequires(mod)
	byModule := map[string]goModRequirement{}
	for _, requirement := range requirements {
		byModule[requirement.module] = requirement
	}
	if len(requirements) != 4 {
		t.Fatalf("parseGoModRequires() returned %d requirements, want 4: %+v", len(requirements), requirements)
	}
	if byModule["example.com/indirect"].indirect != true {
		t.Fatalf("indirect flag = %+v, want indirect=true", byModule["example.com/indirect"])
	}
	if byModule["example.com/direct"].indirect {
		t.Fatal("direct requirement marked indirect, want runtime")
	}
	if byModule["example.com/commented"].version != "v3.0.0" || byModule["example.com/commented"].indirect {
		t.Fatalf("commented = %+v, want version v3.0.0 without indirect", byModule["example.com/commented"])
	}
	if got := parseGoModRequires(""); len(got) != 0 {
		t.Fatalf("parseGoModRequires(empty) = %v, want empty", got)
	}
	if got := parseGoModRequires("module example.com/mod\n\ngo 1.21\n"); len(got) != 0 {
		t.Fatalf("parseGoModRequires(no requires) = %v, want empty", got)
	}
	unclosed := parseGoModRequires("require (\n\texample.com/late v1.0.0\n")
	if len(unclosed) != 1 || unclosed[0].module != "example.com/late" {
		t.Fatalf("parseGoModRequires(unclosed block) = %+v, want the late entry", unclosed)
	}
}

// TestParseGoModRequireLineEdges pins comment stripping and field-count
// guards.
func TestParseGoModRequireLineEdges(t *testing.T) {
	t.Parallel()

	got, ok := parseGoModRequireLine("example.com/mod v1.2.3")
	if !ok || got.module != "example.com/mod" || got.version != "v1.2.3" || got.indirect {
		t.Fatalf("plain line = %+v, %v; want module/version without indirect", got, ok)
	}
	got, ok = parseGoModRequireLine("example.com/mod v1.2.3 // indirect")
	if !ok || !got.indirect || got.version != "v1.2.3" {
		t.Fatalf("indirect line = %+v, %v; want indirect with stripped comment", got, ok)
	}
	got, ok = parseGoModRequireLine("example.com/mod v1.2.3 extra-field // indirect")
	if !ok || got.module != "example.com/mod" || got.version != "v1.2.3" {
		t.Fatalf("extra fields = %+v, %v; want first two fields", got, ok)
	}
	for _, line := range []string{"", "   ", "onlyone", "// just a comment", "require"} {
		if _, ok := parseGoModRequireLine(line); ok {
			t.Fatalf("parseGoModRequireLine(%q) = true, want false", line)
		}
	}
}

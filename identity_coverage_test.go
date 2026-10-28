// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"strings"
	"testing"
)

// TestNormalizeNPMNameCoversScopesAndErrors pins scoped, unscoped, and
// malformed npm names: scope extraction, case folding, and the
// @scope/name requirement.
func TestNormalizeNPMNameCoversScopesAndErrors(t *testing.T) {
	t.Parallel()

	name, namespace, err := normalizeNPMName("Left-Pad")
	if err != nil {
		t.Fatalf("normalizeNPMName(unscoped) error = %v", err)
	}
	if name != "left-pad" || namespace != "" {
		t.Fatalf("normalizeNPMName(unscoped) = (%q, %q), want (%q, %q)", name, namespace, "left-pad", "")
	}

	name, namespace, err = normalizeNPMName("@Scope/Name")
	if err != nil {
		t.Fatalf("normalizeNPMName(scoped) error = %v", err)
	}
	if name != "@scope/name" || namespace != "scope" {
		t.Fatalf("normalizeNPMName(scoped) = (%q, %q), want (%q, %q)", name, namespace, "@scope/name", "scope")
	}

	for _, raw := range []string{"@scope", "@/name", "@scope/", "@scope/ ", "@"} {
		if _, _, err := normalizeNPMName(raw); err == nil {
			t.Fatalf("normalizeNPMName(%q) = nil error, want @scope/name error", raw)
		}
	}
}

// TestNormalizeTwoSegmentNameCoversComposerShapes pins composer identity
// assembly: split on first slash when no namespace is given, errors for
// missing segments, prefix passthrough, and namespace prepending.
func TestNormalizeTwoSegmentNameCoversComposerShapes(t *testing.T) {
	t.Parallel()

	name, namespace, err := normalizeTwoSegmentName("Vendor/Pkg", "", "composer")
	if err != nil {
		t.Fatalf("normalizeTwoSegmentName(split) error = %v", err)
	}
	if name != "vendor/pkg" || namespace != "vendor" {
		t.Fatalf("normalizeTwoSegmentName(split) = (%q, %q), want (%q, %q)", name, namespace, "vendor/pkg", "vendor")
	}

	for _, raw := range []string{"single", "/pkg", "vendor/", "/"} {
		if _, _, err := normalizeTwoSegmentName(raw, "", "composer"); err == nil {
			t.Fatalf("normalizeTwoSegmentName(%q, no namespace) = nil error, want namespace/name error", raw)
		}
	}

	name, namespace, err = normalizeTwoSegmentName("VENDOR/Pkg", "Vendor", "composer")
	if err != nil {
		t.Fatalf("normalizeTwoSegmentName(prefixed) error = %v", err)
	}
	if name != "vendor/pkg" || namespace != "vendor" {
		t.Fatalf("normalizeTwoSegmentName(prefixed) = (%q, %q), want (%q, %q)", name, namespace, "vendor/pkg", "vendor")
	}

	name, namespace, err = normalizeTwoSegmentName("Other", "Vendor", "composer")
	if err != nil {
		t.Fatalf("normalizeTwoSegmentName(prepend) error = %v", err)
	}
	if name != "vendor/other" || namespace != "vendor" {
		t.Fatalf("normalizeTwoSegmentName(prepend) = (%q, %q), want (%q, %q)", name, namespace, "vendor/other", "vendor")
	}
}

// TestNormalizeSwiftNameCoversRegistryShapes pins swift normalization:
// registry-derived namespaces, .git trimming, namespace-prefix stripping,
// last-segment selection, and both blank errors.
func TestNormalizeSwiftNameCoversRegistryShapes(t *testing.T) {
	t.Parallel()

	name, namespace, err := normalizeSwiftName("MyLib", "github.com/Example/Repo.git")
	if err != nil {
		t.Fatalf("normalizeSwiftName(basic) error = %v", err)
	}
	if name != "mylib" || namespace != "github.com/example/repo" {
		t.Fatalf("normalizeSwiftName(basic) = (%q, %q), want (%q, %q)", name, namespace, "mylib", "github.com/example/repo")
	}

	name, namespace, err = normalizeSwiftName("github.com/acme/widget/widget.git", "github.com/Acme/Widget")
	if err != nil {
		t.Fatalf("normalizeSwiftName(prefixed) error = %v", err)
	}
	if name != "widget" || namespace != "github.com/acme/widget" {
		t.Fatalf("normalizeSwiftName(prefixed) = (%q, %q), want (%q, %q)", name, namespace, "widget", "github.com/acme/widget")
	}

	name, namespace, err = normalizeSwiftName("other/org/thing", "github.com/acme/widget")
	if err != nil {
		t.Fatalf("normalizeSwiftName(nested) error = %v", err)
	}
	if name != "thing" || namespace != "github.com/acme/widget" {
		t.Fatalf("normalizeSwiftName(nested) = (%q, %q), want (%q, %q)", name, namespace, "thing", "github.com/acme/widget")
	}

	if _, _, err := normalizeSwiftName("mylib", ""); err == nil {
		t.Fatal("normalizeSwiftName(blank namespace) = nil error, want error")
	}
	if _, _, err := normalizeSwiftName("", "github.com/acme/widget"); err == nil {
		t.Fatal("normalizeSwiftName(blank name) = nil error, want error")
	}
	name, namespace, err = normalizeSwiftName("github.com/acme/widget", "github.com/acme/widget")
	if err != nil {
		t.Fatalf("normalizeSwiftName(namespace-only name) error = %v", err)
	}
	if name != "widget" || namespace != "github.com/acme/widget" {
		t.Fatalf("normalizeSwiftName(namespace-only name) = (%q, %q), want (%q, %q)",
			name, namespace, "widget", "github.com/acme/widget")
	}
	for _, raw := range []string{".git", "/"} {
		if _, _, err := normalizeSwiftName(raw, "github.com/acme/widget"); err == nil {
			t.Fatalf("normalizeSwiftName(%q) = nil error, want blank-name error", raw)
		}
	}
}

// TestNormalizeComposerAndSwiftEndToEnd pins the normalizeName dispatch for
// the composer and swift branches through the public Normalize entry point.
func TestNormalizeComposerAndSwiftEndToEnd(t *testing.T) {
	t.Parallel()

	composer, err := Normalize(RawIdentity{
		Ecosystem: EcosystemComposer,
		Registry:  "packagist.org",
		RawName:   "Vendor/Pkg",
	})
	if err != nil {
		t.Fatalf("Normalize(composer) error = %v", err)
	}
	if composer.NormalizedName != "vendor/pkg" || composer.Namespace != "vendor" {
		t.Fatalf("Normalize(composer) = (%q, %q), want (%q, %q)",
			composer.NormalizedName, composer.Namespace, "vendor/pkg", "vendor")
	}

	swift, err := Normalize(RawIdentity{
		Ecosystem: EcosystemSwift,
		Registry:  "ignored.example",
		RawName:   "MyLib",
		Namespace: "github.com/Example/Repo.git",
	})
	if err != nil {
		t.Fatalf("Normalize(swift) error = %v", err)
	}
	if swift.NormalizedName != "mylib" || swift.Namespace != "github.com/example/repo" {
		t.Fatalf("Normalize(swift) = (%q, %q), want (%q, %q)",
			swift.NormalizedName, swift.Namespace, "mylib", "github.com/example/repo")
	}
	if swift.Registry != "github.com/example/repo" {
		t.Fatalf("Normalize(swift) registry = %q, want namespace %q", swift.Registry, "github.com/example/repo")
	}
}

// TestPackageIDForCoversEcosystemShapes pins every packageIDFor branch:
// maven group coordinates, swift registry-free IDs, hex with and without an
// organization, and the default registry/name form.
func TestPackageIDForCoversEcosystemShapes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name                            string
		ecosystem                       Ecosystem
		registry, namespace, normalized string
		want                            string
	}{
		{"maven", EcosystemMaven, "repo.maven.apache.org/maven2", "com.example", "app", "maven://repo.maven.apache.org/maven2/com.example:app"},
		{"swift", EcosystemSwift, "ignored", "github.com/acme/widget", "mylib", "swift://github.com/acme/widget/mylib"},
		{"hex with org", EcosystemHex, "repo.hex.pm", "acme", "widget", "hex://repo.hex.pm/acme/widget"},
		{"hex without org", EcosystemHex, "repo.hex.pm", "", "widget", "hex://repo.hex.pm/widget"},
		{"default", EcosystemNPM, "registry.npmjs.org", "", "left-pad", "npm://registry.npmjs.org/left-pad"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := packageIDFor(test.ecosystem, test.registry, test.namespace, test.normalized); got != test.want {
				t.Fatalf("packageIDFor() = %q, want %q", got, test.want)
			}
		})
	}
}

// TestPurlTypeForCoversEcosystemMapping pins the golang/gem aliases, the OS
// package-manager override with its generic fallback, and the default
// ecosystem passthrough.
func TestPurlTypeForCoversEcosystemMapping(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name                 string
		ecosystem            Ecosystem
		packageManager, want string
	}{
		{"gomod", EcosystemGoModule, "gomod", "golang"},
		{"rubygems", EcosystemRubyGems, "rubygems", "gem"},
		{"os override", EcosystemOS, "apk", "apk"},
		{"os blank falls back", EcosystemOS, "", "generic"},
		{"os self falls back", EcosystemOS, "os", "generic"},
		{"default", EcosystemNPM, "npm", "npm"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := purlTypeFor(test.ecosystem, test.packageManager); got != test.want {
				t.Fatalf("purlTypeFor() = %q, want %q", got, test.want)
			}
		})
	}
}

// TestPurlPathForCoversEcosystemShapes pins every purlPathFor branch,
// including scoped-npm encoding, maven/swift coordinates, hex with and
// without a namespace, OS registry-qualified paths, and the default form.
func TestPurlPathForCoversEcosystemShapes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name                            string
		ecosystem                       Ecosystem
		registry, namespace, normalized string
		want                            string
	}{
		{"npm scoped", EcosystemNPM, "registry.npmjs.org", "scope", "@scope/name", "%40scope/name"},
		{"npm unscoped", EcosystemNPM, "registry.npmjs.org", "", "left-pad", "left-pad"},
		{"maven", EcosystemMaven, "repo.maven.apache.org/maven2", "com.example", "app", "com.example/app"},
		{"swift", EcosystemSwift, "ignored", "github.com/acme/widget", "mylib", "github.com/acme/widget/mylib"},
		{"hex with org", EcosystemHex, "repo.hex.pm", "acme", "widget", "acme/widget"},
		{"hex without org", EcosystemHex, "repo.hex.pm", "", "widget", "widget"},
		{"os", EcosystemOS, "alpine", "", "musl", "alpine/musl"},
		{"default", EcosystemPyPI, "pypi.org", "", "requests", "requests"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := purlPathFor(test.ecosystem, test.registry, test.namespace, test.normalized); got != test.want {
				t.Fatalf("purlPathFor() = %q, want %q", got, test.want)
			}
		})
	}
}

// TestAllDigitsCoversVersionSuffixes pins the go-module version-suffix
// helper: pure digits pass, qualified or empty values fail.
func TestAllDigitsCoversVersionSuffixes(t *testing.T) {
	t.Parallel()

	for value, want := range map[string]bool{
		"2":   true,
		"123": true,
		"2x":  false,
		"v2":  false,
		"":    false,
	} {
		if got := allDigits(value); got != want {
			t.Fatalf("allDigits(%q) = %t, want %t", value, got, want)
		}
	}
	if got := goModuleNamespace("example.com/mod/v2"); got != "example.com/mod" {
		t.Fatalf("goModuleNamespace(v2 suffix) = %q, want %q", got, "example.com/mod")
	}
}

// TestVersionLimitWarningsCoversTruncation pins truncation warnings: the
// warning key/code/message contract, skipping packages under the limit or
// outside the allowed set, and the version-observation fallback when no
// package observation exists.
func TestVersionLimitWarningsCoversTruncation(t *testing.T) {
	t.Parallel()

	target := TargetConfig{VersionLimit: 2}
	allowed := map[string]struct{}{
		"pkg-a": {},
		"pkg-b": {},
	}
	counts := map[string]int{"pkg-a": 5, "pkg-b": 1, "pkg-c": 3}
	packages := map[string]PackageObservation{
		"pkg-a": {
			Identity:            PackageIdentity{Ecosystem: EcosystemNPM, Registry: "example.invalid", RawName: "a"},
			ScopeID:             "scope-a",
			GenerationID:        "gen-a",
			CollectorInstanceID: "collector-a",
		},
	}
	versions := map[string]PackageVersionObservation{
		"pkg-a": {
			Package:   PackageIdentity{Ecosystem: EcosystemNPM, Registry: "example.invalid", RawName: "a"},
			Version:   "1.0.0",
			ScopeID:   "version-scope-a",
			SourceURI: "https://example.invalid/a",
		},
	}

	warnings := versionLimitWarnings(target, allowed, counts, packages, versions)
	if len(warnings) != 1 {
		t.Fatalf("versionLimitWarnings() = %d warnings, want 1", len(warnings))
	}
	warning := warnings[0]
	if warning.WarningKey != "version-limit:pkg-a" {
		t.Fatalf("WarningKey = %q, want %q", warning.WarningKey, "version-limit:pkg-a")
	}
	if warning.WarningCode != "version_limit_truncated" || warning.Severity != "warning" {
		t.Fatalf("warning code/severity = (%q, %q), want (%q, %q)",
			warning.WarningCode, warning.Severity, "version_limit_truncated", "warning")
	}
	wantMessage := "metadata emitted 5 versions; kept first 2 due to version_limit 2"
	if warning.Message != wantMessage {
		t.Fatalf("Message = %q, want %q", warning.Message, wantMessage)
	}
	if warning.Package == nil || warning.Package.RawName != "a" {
		t.Fatalf("warning package = %+v, want package observation identity", warning.Package)
	}
	if warning.ScopeID != "scope-a" {
		t.Fatalf("ScopeID = %q, want package observation scope %q", warning.ScopeID, "scope-a")
	}
}

// TestVersionLimitWarningsFallsBackToVersionObservation pins the fallback
// branch: with no package observation for a truncated package, the warning
// carries the version observation identity and scope fields.
func TestVersionLimitWarningsFallsBackToVersionObservation(t *testing.T) {
	t.Parallel()

	target := TargetConfig{VersionLimit: 1}
	allowed := map[string]struct{}{"pkg-d": {}}
	counts := map[string]int{"pkg-d": 3}
	versions := map[string]PackageVersionObservation{
		"pkg-d": {
			Package:             PackageIdentity{Ecosystem: EcosystemNPM, Registry: "example.invalid", RawName: "d"},
			Version:             "1.0.0",
			ScopeID:             "version-scope-d",
			GenerationID:        "version-gen-d",
			CollectorInstanceID: "version-collector-d",
			SourceURI:           "https://example.invalid/d",
		},
	}

	warnings := versionLimitWarnings(target, allowed, counts, map[string]PackageObservation{}, versions)
	if len(warnings) != 1 {
		t.Fatalf("versionLimitWarnings() = %d warnings, want 1", len(warnings))
	}
	warning := warnings[0]
	if warning.Package == nil || warning.Package.RawName != "d" {
		t.Fatalf("warning package = %+v, want version observation identity", warning.Package)
	}
	if warning.ScopeID != "version-scope-d" || warning.GenerationID != "version-gen-d" {
		t.Fatalf("warning scope = (%q, %q), want version observation scope",
			warning.ScopeID, warning.GenerationID)
	}
	if warning.SourceURI != "https://example.invalid/d" {
		t.Fatalf("SourceURI = %q, want version observation URI", warning.SourceURI)
	}
	if got := versionLimitWarnings(target, allowed, map[string]int{}, map[string]PackageObservation{}, versions); len(got) != 0 {
		t.Fatalf("versionLimitWarnings(empty) = %d warnings, want 0", len(got))
	}
}

// boundWarningFixture returns a warning bound to identity with version, plus
// the allowed package/version sets that keep exactly that pair.
func boundWarningFixture(identity PackageIdentity, version string) (WarningObservation, map[string]struct{}, map[string]struct{}) {
	normalized, err := NormalizePackageIdentity(identity)
	if err != nil {
		panic(err)
	}
	allowedPackages := map[string]struct{}{normalized.PackageID: {}}
	allowedVersions := map[string]struct{}{versionKey(normalized.PackageID, version): {}}
	return WarningObservation{Package: &identity, Version: version}, allowedPackages, allowedVersions
}

// TestBoundWarningsCoversKeepAndDrop pins boundWarnings: package-less
// warnings pass through, allowed pairs are kept, unknown packages, pruned
// versions, and blank versions resolve per keepPackageVersionObservation.
func TestBoundWarningsCoversKeepAndDrop(t *testing.T) {
	t.Parallel()

	identity := parityIdentity()
	warning, allowedPackages, allowedVersions := boundWarningFixture(identity, "1.3.0")

	kept, err := boundWarnings([]WarningObservation{
		{WarningKey: "global"},
		warning,
		{Package: &PackageIdentity{Ecosystem: EcosystemNPM, Registry: "https://registry.npmjs.org", RawName: "other"}, Version: "1.0.0"},
	}, allowedPackages, allowedVersions)
	if err != nil {
		t.Fatalf("boundWarnings() error = %v", err)
	}
	if len(kept) != 2 {
		t.Fatalf("boundWarnings() kept %d, want 2 (global + allowed)", len(kept))
	}
	if kept[0].WarningKey != "global" {
		t.Fatalf("kept[0].WarningKey = %q, want %q", kept[0].WarningKey, "global")
	}

	pruned, err := boundWarnings([]WarningObservation{{Package: &identity, Version: "9.9.9"}}, allowedPackages, allowedVersions)
	if err != nil {
		t.Fatalf("boundWarnings(pruned version) error = %v", err)
	}
	if len(pruned) != 0 {
		t.Fatalf("boundWarnings(pruned version) kept %d, want 0", len(pruned))
	}

	unversioned, err := boundWarnings([]WarningObservation{{Package: &identity}}, allowedPackages, allowedVersions)
	if err != nil {
		t.Fatalf("boundWarnings(blank version) error = %v", err)
	}
	if len(unversioned) != 1 {
		t.Fatalf("boundWarnings(blank version) kept %d, want 1", len(unversioned))
	}

	noVersions, err := boundWarnings([]WarningObservation{warning}, allowedPackages, map[string]struct{}{})
	if err != nil {
		t.Fatalf("boundWarnings(empty versions) error = %v", err)
	}
	if len(noVersions) != 0 {
		t.Fatalf("boundWarnings(empty versions) kept %d, want 0", len(noVersions))
	}

	bad := WarningObservation{Package: &PackageIdentity{}}
	if _, err := boundWarnings([]WarningObservation{bad}, allowedPackages, allowedVersions); err == nil {
		t.Fatal("boundWarnings(invalid identity) = nil error, want normalize error")
	}
}

// TestCloneStringMapCoversNilAndIsolation pins cloneStringMap: nil and empty
// inputs stay nil, and clones detach from later caller mutation.
func TestCloneStringMapCoversNilAndIsolation(t *testing.T) {
	t.Parallel()

	if got := cloneStringMap(nil); got != nil {
		t.Fatalf("cloneStringMap(nil) = %v, want nil", got)
	}
	if got := cloneStringMap(map[string]string{}); got != nil {
		t.Fatalf("cloneStringMap(empty) = %v, want nil", got)
	}

	input := map[string]string{"sha256": "abc", "sha512": "def"}
	cloned := cloneStringMap(input)
	if len(cloned) != 2 || cloned["sha256"] != "abc" || cloned["sha512"] != "def" {
		t.Fatalf("cloneStringMap() = %v, want copy of input", cloned)
	}
	cloned["sha256"] = "mutated"
	if input["sha256"] != "abc" {
		t.Fatalf("cloneStringMap() aliases input: input = %v", input)
	}
	if strings.Contains(cloned["sha256"], "abc") {
		t.Fatalf("cloneStringMap() mutation leaked: cloned = %v", cloned)
	}
}

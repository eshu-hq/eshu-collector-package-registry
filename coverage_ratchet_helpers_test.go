// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"strings"
	"testing"
	"time"
)

// ratchetIdentity returns a valid npm package identity for envelope and bound
// tests. The registry and names are documentation fixtures only.
func ratchetIdentity(rawName string) PackageIdentity {
	return PackageIdentity{
		Ecosystem: EcosystemNPM,
		Registry:  "https://registry.npmjs.org",
		RawName:   rawName,
	}
}

// TestRatchetValidateParserContextBranches pins the parser context guard: a
// matching context passes, a wrong ecosystem and a blank registry fail with
// the parser name, and a blank boundary fails on the boundary noun.
func TestRatchetValidateParserContextBranches(t *testing.T) {
	t.Parallel()

	if err := validateParserContext(ratchetParserContext(EcosystemNPM), EcosystemNPM, "npm packument"); err != nil {
		t.Fatalf("validateParserContext(match) error = %v", err)
	}
	if err := validateParserContext(ratchetParserContext(EcosystemNPM), EcosystemPyPI, "pypi project"); err == nil {
		t.Fatal("validateParserContext(mismatch) error = nil, want rejection")
	} else if !strings.Contains(err.Error(), "pypi project") || !strings.Contains(err.Error(), `want "pypi"`) {
		t.Fatalf("mismatch error = %q, want parser name and wanted ecosystem", err)
	}
	blankRegistry := ratchetParserContext(EcosystemNPM)
	blankRegistry.Registry = ""
	if err := validateParserContext(blankRegistry, EcosystemNPM, "npm packument"); err == nil {
		t.Fatal("validateParserContext(blank registry) error = nil, want rejection")
	} else if !strings.Contains(err.Error(), "must not be blank") {
		t.Fatalf("registry error = %q, want blank rejection", err)
	}
	blankScope := ratchetParserContext(EcosystemNPM)
	blankScope.ScopeID = ""
	if err := validateParserContext(blankScope, EcosystemNPM, "npm packument"); err == nil {
		t.Fatal("validateParserContext(blank scope) error = nil, want rejection")
	} else if !strings.Contains(err.Error(), "scope_id") {
		t.Fatalf("scope error = %q, want boundary noun", err)
	}
}

// TestRatchetObservationVisibilityDefaults pins the visibility fallback: an
// explicit visibility flows through, a blank one becomes unknown.
func TestRatchetObservationVisibilityDefaults(t *testing.T) {
	t.Parallel()

	identity := ratchetIdentity("left-pad")
	explicit := ratchetParserContext(EcosystemNPM)
	explicit.Visibility = VisibilityPublic
	if got := packageObservation(explicit, identity).Visibility; got != VisibilityPublic {
		t.Fatalf("visibility = %q, want public", got)
	}
	if got := packageObservationWithVisibility(ratchetParserContext(EcosystemNPM), identity, "").Visibility; got != VisibilityUnknown {
		t.Fatalf("blank visibility = %q, want unknown", got)
	}
}

// TestRatchetObservationTrimming pins whitespace trimming on version,
// dependency, artifact, and source-hint observations.
func TestRatchetObservationTrimming(t *testing.T) {
	t.Parallel()

	ctx := ratchetParserContext(EcosystemNPM)
	identity := ratchetIdentity("left-pad")
	if got := versionObservation(ctx, identity, "  1.3.0 ").Version; got != "1.3.0" {
		t.Fatalf("version = %q, want trimmed", got)
	}
	dep := dependencyObservation(ctx, identity, " 1.3.0 ", ratchetIdentity("dep"), " ^2.0 ", " runtime ")
	if dep.Version != "1.3.0" || dep.Range != "^2.0" || dep.DependencyType != "runtime" {
		t.Fatalf("dependency = %+v, want trimmed fields", dep)
	}
	before := map[string]string{"sha256": "abc"}
	artifact := artifactObservation(ctx, identity, "1.3.0", " key ", " tarball ",
		"https://registry.npmjs.org/left-pad/-/left-pad-1.3.0.tgz", " path ", 12, before)
	before["sha256"] = "mutated"
	if artifact.Hashes["sha256"] != "abc" {
		t.Fatalf("hashes = %v, want an isolated copy", artifact.Hashes)
	}
	if artifact.ArtifactKey != "key" || artifact.ArtifactType != "tarball" || artifact.ArtifactPath != "path" {
		t.Fatalf("artifact = %+v, want trimmed key/type/path", artifact)
	}
	hint := sourceHintObservation(ctx, identity, "1.3.0", " repository ", "git+https://github.com/example/left-pad.git", " reason ")
	if hint.HintKind != "repository" || hint.RawURL != "https://github.com/example/left-pad" {
		t.Fatalf("hint = %+v, want trimmed kind and stripped git decorations", hint)
	}
}

// TestRatchetGenericVulnerabilityHints pins advisory merging: the advisories
// list appends to vulnerabilities, blank entries skip, and a missing source
// falls back to the provider, then to generic.
func TestRatchetGenericVulnerabilityHints(t *testing.T) {
	t.Parallel()

	ctx := ratchetParserContext(EcosystemGeneric)
	identity := ratchetIdentity("widget")
	metadata := genericMetadata{
		Provider: "acme",
		Vulnerabilities: []genericVulnerability{
			{},
			{AdvisoryID: "ADV-1", VulnerabilityID: "V-1", SourceSeverity: " high ", PublishedAt: "2026-01-01T00:00:00Z"},
		},
		Advisories: []genericVulnerability{
			{VulnerabilityID: "V-2", AdvisorySource: "acme-sec"},
		},
	}
	observations := genericVulnerabilityHints(ctx, identity, " 1.0 ", metadata)
	if len(observations) != 2 {
		t.Fatalf("hints = %d, want 2 with the blank entry skipped", len(observations))
	}
	first := observations[0]
	if first.AdvisoryID != "ADV-1" || first.AdvisorySource != "acme" || first.Version != "1.0" || first.SourceSeverity != "high" {
		t.Fatalf("first hint = %+v, want advisory with provider fallback source", first)
	}
	if first.PublishedAt.IsZero() {
		t.Fatal("published_at must parse the RFC3339 timestamp")
	}
	if second := observations[1]; second.AdvisoryID != "V-2" || second.AdvisorySource != "acme-sec" {
		t.Fatalf("second hint = %+v, want the advisory-list entry", second)
	}
	sourceless := genericVulnerabilityHints(ctx, identity, "1.0", genericMetadata{
		Vulnerabilities: []genericVulnerability{{AdvisoryID: "ADV-9"}},
	})
	if len(sourceless) != 1 || sourceless[0].AdvisorySource != "generic" {
		t.Fatalf("sourceless = %+v, want the generic fallback source", sourceless)
	}
}

// TestRatchetGenericRegistryEvents pins event filtering: entries without a
// key or type skip, complete entries carry trimmed fields and timestamps.
func TestRatchetGenericRegistryEvents(t *testing.T) {
	t.Parallel()

	ctx := ratchetParserContext(EcosystemGeneric)
	observations := genericRegistryEvents(ctx, ratchetIdentity("widget"), "1.0", []genericRegistryEvent{
		{EventType: "publish"},
		{EventKey: "e0"},
		{EventKey: " e1 ", EventType: " publish ", Actor: " bot ", Message: " hello ", OccurredAt: "2026-02-02T00:00:00Z"},
	})
	if len(observations) != 1 {
		t.Fatalf("events = %d, want 1 with blanks skipped", len(observations))
	}
	event := observations[0]
	if event.EventKey != "e1" || event.EventType != "publish" || event.Actor != "bot" || event.Message != "hello" {
		t.Fatalf("event = %+v, want trimmed fields", event)
	}
	if event.OccurredAt.IsZero() {
		t.Fatal("occurred_at must parse the RFC3339 timestamp")
	}
}

// TestRatchetNPMDependencySets pins the four npm dependency sets: runtime,
// development, optional (flagged), and peer, emitted in sorted name order.
func TestRatchetNPMDependencySets(t *testing.T) {
	t.Parallel()

	ctx := ratchetParserContext(EcosystemNPM)
	entry := npmVersion{
		Dependencies:         map[string]string{"b-lib": "^1.0.0", "a-lib": "~1.2.0"},
		DevDependencies:      map[string]string{"dev-lib": "3.0.0"},
		OptionalDependencies: map[string]string{"opt-lib": "4.0.0"},
		PeerDependencies:     map[string]string{"peer-lib": ">=5.0.0"},
	}
	dependencies := npmDependencies(ctx, ratchetIdentity("left-pad"), "1.3.0", entry)
	if len(dependencies) != 5 {
		t.Fatalf("dependencies = %d, want 5 across the four sets", len(dependencies))
	}
	byName := map[string]PackageDependencyObservation{}
	for _, dependency := range dependencies {
		byName[dependency.Dependency.RawName] = dependency
	}
	if byName["a-lib"].DependencyType != "runtime" || byName["dev-lib"].DependencyType != "development" ||
		byName["peer-lib"].DependencyType != "peer" || byName["opt-lib"].DependencyType != "optional" {
		t.Fatalf("types = %v, want runtime/development/peer/optional", dependencies)
	}
	if !byName["opt-lib"].Optional || byName["a-lib"].Optional {
		t.Fatal("only the optional set must set the optional flag")
	}
	if dependencies[0].Dependency.RawName != "a-lib" || dependencies[1].Dependency.RawName != "b-lib" {
		t.Fatalf("order = %q %q, want sorted runtime names first", dependencies[0].Dependency.RawName, dependencies[1].Dependency.RawName)
	}
	if byName["a-lib"].Dependency.Ecosystem != EcosystemNPM || byName["a-lib"].Dependency.Registry != ctx.Registry {
		t.Fatalf("dependency identity = %+v, want the npm registry scope", byName["a-lib"].Dependency)
	}
}

// TestRatchetNPMRepositoryURLForms pins the repository field shapes: a plain
// string, an object with a string url, an object without one, and a
// non-string value.
func TestRatchetNPMRepositoryURLForms(t *testing.T) {
	t.Parallel()

	if got := npmRepositoryURL("https://github.com/example/x"); got != "https://github.com/example/x" {
		t.Fatalf("string repository = %q", got)
	}
	if got := npmRepositoryURL(map[string]any{"url": "https://github.com/example/y", "type": "git"}); got != "https://github.com/example/y" {
		t.Fatalf("object repository = %q", got)
	}
	if got := npmRepositoryURL(map[string]any{"type": "git"}); got != "" {
		t.Fatalf("object without url = %q, want blank", got)
	}
	if got := npmRepositoryURL(42); got != "" {
		t.Fatalf("non-string repository = %q, want blank", got)
	}
}

// TestRatchetNPMHintsAndDeprecated pins repository/homepage hints and the
// three deprecated shapes: bool, blank/non-blank string, and absent.
func TestRatchetNPMHintsAndDeprecated(t *testing.T) {
	t.Parallel()

	ctx := ratchetParserContext(EcosystemNPM)
	identity := ratchetIdentity("left-pad")
	hints := npmSourceHints(ctx, identity, "1.0", npmVersion{
		Repository: map[string]any{"url": "git+https://github.com/example/x.git"},
		Homepage:   "https://example.invalid/x",
	})
	if len(hints) != 2 || hints[0].HintKind != "repository" || hints[1].HintKind != "homepage" {
		t.Fatalf("hints = %+v, want repository plus homepage", hints)
	}
	if hints[0].RawURL != "https://github.com/example/x" {
		t.Fatalf("repository hint = %q, want stripped git decorations", hints[0].RawURL)
	}
	if got := len(npmSourceHints(ctx, identity, "1.0", npmVersion{Repository: "https://github.com/example/y"})); got != 1 {
		t.Fatalf("string-repository hints = %d, want 1", got)
	}
	if got := len(npmSourceHints(ctx, identity, "1.0", npmVersion{})); got != 0 {
		t.Fatalf("empty hints = %d, want 0", got)
	}
	for value, want := range map[any]bool{true: true, false: false, "deprecated": true, "  ": false, nil: false} {
		if got := (npmVersion{Deprecated: value}).deprecated(); got != want {
			t.Fatalf("deprecated(%#v) = %t, want %t", value, got, want)
		}
	}
}

// TestRatchetNPMChecksums pins checksum merging: integrity and shasum keys,
// and nil when neither is present.
func TestRatchetNPMChecksums(t *testing.T) {
	t.Parallel()

	checksums := npmChecksums(npmDist{Integrity: " sha512-abc ", Shasum: " def "})
	if checksums["integrity"] != "sha512-abc" || checksums["sha1"] != "def" {
		t.Fatalf("checksums = %v, want trimmed integrity and sha1", checksums)
	}
	if got := npmChecksums(npmDist{Integrity: "only"}); len(got) != 1 {
		t.Fatalf("integrity-only = %v, want one entry", got)
	}
	if got := npmChecksums(npmDist{}); got != nil {
		t.Fatalf("empty = %v, want nil", got)
	}
}

// TestRatchetPyPIDependencies pins requirement parsing: blank entries skip,
// ranges and markers split, and bracket extras stay on the range side.
func TestRatchetPyPIDependencies(t *testing.T) {
	t.Parallel()

	ctx := ratchetParserContext(EcosystemPyPI)
	dependencies := pypiDependencies(ctx, ratchetIdentity("requests"), "2.0", []string{
		"  ",
		"urllib3>=1.21",
		"flask[async]==3.0; python_version>'3.8'",
	})
	if len(dependencies) != 2 {
		t.Fatalf("dependencies = %d, want 2 with the blank skipped", len(dependencies))
	}
	if dependencies[0].Dependency.RawName != "urllib3" || dependencies[0].Range != ">=1.21" {
		t.Fatalf("first = %+v, want split name and range", dependencies[0])
	}
	if dependencies[1].Dependency.RawName != "flask" || dependencies[1].Marker != "python_version>'3.8'" {
		t.Fatalf("second = %+v, want extras stripped and marker kept", dependencies[1])
	}
	if dependencies[1].Dependency.Ecosystem != EcosystemPyPI {
		t.Fatalf("ecosystem = %q, want pypi", dependencies[1].Dependency.Ecosystem)
	}
}

// TestRatchetParseRequirementForms pins the requirement splitter on plain,
// ranged, marked, and padded inputs.
func TestRatchetParseRequirementForms(t *testing.T) {
	t.Parallel()

	name, rng, marker := parseRequirement("requests")
	if name != "requests" || rng != "" || marker != "" {
		t.Fatalf("plain = %q %q %q", name, rng, marker)
	}
	name, rng, marker = parseRequirement("requests>=2.0 ; python_version>='3.8'")
	if name != "requests" || rng != ">=2.0" || marker != "python_version>='3.8'" {
		t.Fatalf("ranged = %q %q %q", name, rng, marker)
	}
	name, _, marker = parseRequirement("  padded  ")
	if name != "padded" || marker != "" {
		t.Fatalf("padded = %q %q", name, marker)
	}
}

// TestRatchetPyPISourceHints pins project-URL classification: source and
// repo labels become repository hints, home labels become homepage hints,
// and anything else stays a project hint with the normalized label reason.
func TestRatchetPyPISourceHints(t *testing.T) {
	t.Parallel()

	ctx := ratchetParserContext(EcosystemPyPI)
	hints := pypiSourceHints(ctx, ratchetIdentity("requests"), "2.0", map[string]string{
		"Source Code": "https://github.com/psf/requests",
		"Homepage":    "https://requests.readthedocs.io",
		"Docs":        "https://example.invalid/docs",
	})
	if len(hints) != 3 {
		t.Fatalf("hints = %d, want 3", len(hints))
	}
	byKind := map[string]string{}
	for _, hint := range hints {
		byKind[hint.RawURL] = hint.HintKind
	}
	if byKind["https://github.com/psf/requests"] != "repository" ||
		byKind["https://requests.readthedocs.io"] != "homepage" ||
		byKind["https://example.invalid/docs"] != "project" {
		t.Fatalf("kinds = %v", byKind)
	}
	if hints[0].ConfidenceReason != "pypi-project-url:docs" {
		t.Fatalf("first reason = %q, want sorted labels with docs first", hints[0].ConfidenceReason)
	}
}

// TestRatchetGenericArtifactHashes pins hash merging: explicit SHA fields
// join the cloned map, and an empty artifact reports nil.
func TestRatchetGenericArtifactHashes(t *testing.T) {
	t.Parallel()

	hashes := (genericArtifact{Hashes: map[string]string{"md5": "m", "sha256": "old"}, SHA256: " new ", SHA1: "s1"}).hashes()
	if hashes["sha256"] != "new" || hashes["sha1"] != "s1" || hashes["md5"] != "m" {
		t.Fatalf("hashes = %v, want merged trimmed fields", hashes)
	}
	if got := (genericArtifact{}).hashes(); got != nil {
		t.Fatalf("empty = %v, want nil", got)
	}
	if got := (genericArtifact{MD5: "m"}).hashes(); len(got) != 1 || got["md5"] != "m" {
		t.Fatalf("md5-only = %v", got)
	}
}

// TestRatchetArtifactKeyForms pins key derivation: a URL path without its
// leading slash, a host-only URL falling back to the sanitized value, and an
// untakable value failing closed to blank.
func TestRatchetArtifactKeyForms(t *testing.T) {
	t.Parallel()

	if got := artifactKey("https://cdn.example/packages/pkg-1.0.tgz"); got != "packages/pkg-1.0.tgz" {
		t.Fatalf("key = %q", got)
	}
	if got := artifactKey("https://example.invalid"); got != "https://example.invalid" {
		t.Fatalf("host-only key = %q", got)
	}
	if got := artifactKey("http://[::1"); got != "" {
		t.Fatalf("unparseable key = %q, want blank", got)
	}
}

// TestRatchetSmallStringHelpers pins the git, blank, version, timestamp, and
// visibility helpers on their edge inputs.
func TestRatchetSmallStringHelpers(t *testing.T) {
	t.Parallel()

	if got := stripGitURLDecorations("git+https://github.com/example/x.git"); got != "https://github.com/example/x" {
		t.Fatalf("stripped = %q", got)
	}
	if got := stripGitURLDecorations("https://github.com/example/x"); got != "https://github.com/example/x" {
		t.Fatalf("plain = %q, want unchanged", got)
	}
	if got := firstNonBlank("", "  ", "a", "b"); got != "a" {
		t.Fatalf("firstNonBlank = %q, want a", got)
	}
	if got := firstNonBlank(" ", ""); got != "" {
		t.Fatalf("all-blank = %q, want blank", got)
	}
	versions := []PackageVersionObservation{{Version: "2.0"}, {Version: "1.0"}}
	if got := firstParsedVersion(versions, "fallback"); got != "2.0" {
		t.Fatalf("first = %q, want 2.0", got)
	}
	if got := firstParsedVersion(nil, "  fallback "); got != "fallback" {
		t.Fatalf("fallback = %q, want trimmed", got)
	}
	if got := parseTimestamp(""); !got.IsZero() {
		t.Fatalf("blank timestamp = %v, want zero", got)
	}
	if got := parseTimestamp("not-a-time"); !got.IsZero() {
		t.Fatalf("bad timestamp = %v, want zero", got)
	}
	parsed := parseTimestamp("2026-10-01T14:00:00+02:00")
	if want := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC); !parsed.Equal(want) {
		t.Fatalf("timestamp = %v, want %v in UTC", parsed, want)
	}
	for raw, want := range map[string]Visibility{"PUBLIC": VisibilityPublic, "private": VisibilityPrivate, "x": VisibilityUnknown, "": VisibilityUnknown} {
		if got := parseVisibility(raw); got != want {
			t.Fatalf("parseVisibility(%q) = %q, want %q", raw, got, want)
		}
	}
}

// TestRatchetSanitizeUserinfoFailClosed pins the fail-closed branch: a value
// net/url cannot take apart reports userinfo so callers drop it.
func TestRatchetSanitizeUserinfoFailClosed(t *testing.T) {
	t.Parallel()

	if !carriesUserinfo("http://[::1") {
		t.Fatal("carriesUserinfo(unparseable) = false, want true")
	}
	if carriesUserinfo("https://example.invalid/packages/x.tgz") {
		t.Fatal("carriesUserinfo(clean URL) = true, want false")
	}
	if carriesUserinfo("pkg:npm/left-pad@1.3.0") {
		t.Fatal("carriesUserinfo(purl) = true, want false: the @ sits after the first /")
	}
	if got := sanitizeURL("https://example.invalid/x?token=abc&keep=1"); got != "https://example.invalid/x?keep=1" {
		t.Fatalf("sanitized = %q, want the sensitive key stripped", got)
	}
}

// TestRatchetParserContextMismatch pins the per-parser context guard: every
// parser rejects a context scoped to another ecosystem without panicking.
func TestRatchetParserContextMismatch(t *testing.T) {
	t.Parallel()

	// The artifactory wrapper validates its package_type instead of the
	// parser context; its mismatch branch lives in
	// TestRatchetArtifactoryParser.
	parsers := map[string]func(MetadataParserContext, []byte) (ParsedMetadata, error){
		"npm":     ParseNPMPackumentMetadata,
		"pypi":    ParsePyPIProjectMetadata,
		"gomod":   ParseGoModuleProxyMetadata,
		"maven":   ParseMavenPackageMetadata,
		"nuget":   ParseNuGetPackageMetadata,
		"generic": ParseGenericPackageMetadata,
	}
	for name, parser := range parsers {
		ctx := ratchetParserContext(EcosystemNPM)
		if name == "npm" {
			ctx = ratchetParserContext(EcosystemPyPI)
		}
		if _, err := parser(ctx, []byte(`{}`)); err == nil {
			t.Errorf("%s parser accepted a foreign ecosystem context", name)
		} else if !strings.Contains(err.Error(), "parser ecosystem") {
			t.Errorf("%s error = %q, want the ecosystem guard", name, err)
		}
	}
}

// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"strings"
	"testing"
	"time"
)

func TestMetadataHelperValidateParserContext(t *testing.T) {
	t.Parallel()

	valid := ratchetParserContext(EcosystemNPM)
	if err := validateParserContext(valid, EcosystemNPM, "npm packument"); err != nil {
		t.Fatalf("validateParserContext() error = %v", err)
	}
	if err := validateParserContext(valid, EcosystemPyPI, "npm packument"); err == nil {
		t.Fatal("validateParserContext() with wrong ecosystem must fail")
	}
	blankRegistry := valid
	blankRegistry.Registry = ""
	if err := validateParserContext(blankRegistry, EcosystemNPM, "npm packument"); err == nil {
		t.Fatal("validateParserContext() with blank registry must fail")
	}
	for _, tc := range []struct {
		name  string
		blank func(*MetadataParserContext)
	}{
		{"scope", func(c *MetadataParserContext) { c.ScopeID = "" }},
		{"generation", func(c *MetadataParserContext) { c.GenerationID = "" }},
		{"collector", func(c *MetadataParserContext) { c.CollectorInstanceID = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := valid
			tc.blank(&ctx)
			if err := validateParserContext(ctx, EcosystemNPM, "npm packument"); err == nil {
				t.Fatalf("validateParserContext() with blank %s must fail", tc.name)
			}
		})
	}
}

func TestMetadataHelperGenericVulnerabilityHints(t *testing.T) {
	t.Parallel()

	ctx := ratchetParserContext(EcosystemGeneric)
	identity := PackageIdentity{Ecosystem: EcosystemGeneric, Registry: ctx.Registry, RawName: "acme-widget"}
	metadata := genericMetadata{
		Provider: "acme",
		Vulnerabilities: []genericVulnerability{
			{
				AdvisoryID: "ACME-2026-001", VulnerabilityID: "CVE-2026-0001",
				SourceSeverity: "high", AffectedRange: "<2.0.0", FixedVersion: "2.0.0",
				URL: "https://example.com/advisories/1", Summary: "bad bug",
				PublishedAt: "2026-01-02T03:04:05Z", ModifiedAt: "not-a-time",
			},
			{AdvisorySource: "custom-source", VulnerabilityID: "CVE-2026-0002"},
			{Summary: "no identifiers"},
		},
		Advisories: []genericVulnerability{
			{AdvisoryID: "ACME-2026-009", Summary: "advisory list entry"},
		},
	}
	got := genericVulnerabilityHints(ctx, identity, "1.0.0", metadata)
	if len(got) != 3 {
		t.Fatalf("genericVulnerabilityHints() = %d observations, want 3", len(got))
	}
	first := got[0]
	if first.AdvisoryID != "ACME-2026-001" || first.AdvisorySource != "acme" {
		t.Fatalf("first hint = %+v, want advisory ACME-2026-001 from acme", first)
	}
	if first.VulnerabilityID != "CVE-2026-0001" || first.FixedVersion != "2.0.0" {
		t.Fatalf("first hint fields = %+v", first)
	}
	if first.PublishedAt.IsZero() {
		t.Fatal("first hint PublishedAt must parse")
	}
	if !first.ModifiedAt.IsZero() {
		t.Fatal("first hint ModifiedAt must be zero for invalid input")
	}
	if got[1].AdvisoryID != "CVE-2026-0002" || got[1].AdvisorySource != "custom-source" {
		t.Fatalf("fallback advisory = %+v", got[1])
	}
	if got[2].AdvisorySource != "acme" {
		t.Fatalf("advisory-list hint source = %q, want provider fallback", got[2].AdvisorySource)
	}
	empty := genericVulnerabilityHints(ctx, identity, "1.0.0", genericMetadata{})
	if len(empty) != 0 {
		t.Fatalf("genericVulnerabilityHints() empty = %d, want 0", len(empty))
	}
	providerless := genericVulnerabilityHints(ctx, identity, "1.0.0", genericMetadata{
		Vulnerabilities: []genericVulnerability{{AdvisoryID: "X-1"}},
	})
	if len(providerless) != 1 || providerless[0].AdvisorySource != "generic" {
		t.Fatalf("providerless hint = %+v, want generic source", providerless)
	}
}

func TestMetadataHelperGenericRegistryEvents(t *testing.T) {
	t.Parallel()

	ctx := ratchetParserContext(EcosystemGeneric)
	identity := PackageIdentity{Ecosystem: EcosystemGeneric, Registry: ctx.Registry, RawName: "acme-widget"}
	events := []genericRegistryEvent{
		{EventKey: "evt-1", EventType: "publish", ArtifactKey: "a.tgz", Actor: "bot",
			Message: "published", OccurredAt: "2026-02-03T04:05:06Z"},
		{EventType: "publish"},
		{EventKey: "evt-2"},
	}
	got := genericRegistryEvents(ctx, identity, "1.0.0", events)
	if len(got) != 1 {
		t.Fatalf("genericRegistryEvents() = %d observations, want 1", len(got))
	}
	if got[0].EventKey != "evt-1" || got[0].EventType != "publish" || got[0].Actor != "bot" {
		t.Fatalf("event = %+v", got[0])
	}
	if got[0].OccurredAt.IsZero() {
		t.Fatal("event OccurredAt must parse")
	}
}

func TestMetadataHelperNPMHelpers(t *testing.T) {
	t.Parallel()

	ctx := ratchetParserContext(EcosystemNPM)
	identity := PackageIdentity{Ecosystem: EcosystemNPM, Registry: ctx.Registry, RawName: "left-pad"}

	set := npmDependencySet(ctx, identity, "1.3.0",
		map[string]string{"b-dep": "^2.0.0", "a-dep": "~1.0.0"}, "runtime", false)
	if len(set) != 2 || set[0].Dependency.RawName != "a-dep" || set[1].Dependency.RawName != "b-dep" {
		t.Fatalf("npmDependencySet() order = %+v, want sorted", set)
	}
	if set[0].Range != "~1.0.0" || set[0].DependencyType != "runtime" || set[0].Optional {
		t.Fatalf("npmDependencySet() first = %+v", set[0])
	}
	optional := npmDependencySet(ctx, identity, "1.3.0", map[string]string{"o": "1.x"}, "optional", true)
	if len(optional) != 1 || !optional[0].Optional {
		t.Fatalf("npmDependencySet() optional = %+v", optional)
	}
	if out := npmDependencySet(ctx, identity, "1.3.0", nil, "runtime", false); len(out) != 0 {
		t.Fatalf("npmDependencySet() nil = %+v, want empty", out)
	}

	hints := npmSourceHints(ctx, identity, "1.3.0", npmVersion{
		Repository: map[string]any{"url": "git+https://example.com/repo.git"},
		Homepage:   "https://example.com/home",
	})
	if len(hints) != 2 || hints[0].HintKind != "repository" || hints[1].HintKind != "homepage" {
		t.Fatalf("npmSourceHints() = %+v, want repository + homepage", hints)
	}
	if out := npmSourceHints(ctx, identity, "1.3.0", npmVersion{}); len(out) != 0 {
		t.Fatalf("npmSourceHints() empty = %+v, want none", out)
	}

	if got := npmRepositoryURL("https://example.com/r"); got != "https://example.com/r" {
		t.Fatalf("npmRepositoryURL(string) = %q", got)
	}
	if got := npmRepositoryURL(map[string]any{"url": "https://example.com/m"}); got != "https://example.com/m" {
		t.Fatalf("npmRepositoryURL(map) = %q", got)
	}
	for _, raw := range []any{map[string]any{"directory": "x"}, 42, nil} {
		if got := npmRepositoryURL(raw); got != "" {
			t.Fatalf("npmRepositoryURL(%v) = %q, want empty", raw, got)
		}
	}

	if !(npmVersion{Deprecated: true}).deprecated() {
		t.Fatal("deprecated bool true must report true")
	}
	if (npmVersion{Deprecated: false}).deprecated() || (npmVersion{}).deprecated() {
		t.Fatal("deprecated bool false / missing must report false")
	}
	if !(npmVersion{Deprecated: "use something else"}).deprecated() {
		t.Fatal("deprecated non-blank string must report true")
	}
	if (npmVersion{Deprecated: "  "}).deprecated() {
		t.Fatal("deprecated blank string must report false")
	}

	if got := npmChecksums(npmDist{Integrity: "sha512-abc", Shasum: "deadbeef"}); len(got) != 2 {
		t.Fatalf("npmChecksums() both = %+v", got)
	}
	if got := npmChecksums(npmDist{Integrity: "sha512-abc"}); len(got) != 1 || got["integrity"] == "" {
		t.Fatalf("npmChecksums() integrity = %+v", got)
	}
	if got := npmChecksums(npmDist{Shasum: "deadbeef"}); len(got) != 1 || got["sha1"] == "" {
		t.Fatalf("npmChecksums() shasum = %+v", got)
	}
	if got := npmChecksums(npmDist{}); got != nil {
		t.Fatalf("npmChecksums() empty = %+v, want nil", got)
	}
}

func TestMetadataHelperPyPIHelpers(t *testing.T) {
	t.Parallel()

	ctx := ratchetParserContext(EcosystemPyPI)
	identity := PackageIdentity{Ecosystem: EcosystemPyPI, Registry: ctx.Registry, RawName: "requests"}
	deps := pypiDependencies(ctx, identity, "2.0.0", []string{
		"urllib3>=1.21.1,<3",
		`chardet<6,>=3.0.2; python_version > "2.7"`,
		"",
		"   ",
	})
	if len(deps) != 2 {
		t.Fatalf("pypiDependencies() = %d, want 2 (blanks skipped)", len(deps))
	}
	if deps[0].Dependency.RawName != "urllib3" || deps[0].Range != ">=1.21.1,<3" {
		t.Fatalf("pypiDependencies()[0] = %+v", deps[0])
	}
	if deps[1].Dependency.RawName != "chardet" || deps[1].Marker == "" {
		t.Fatalf("pypiDependencies()[1] = %+v, want marker", deps[1])
	}

	hints := pypiSourceHints(ctx, identity, "2.0.0", map[string]string{
		"Source Code": "git+https://example.com/org/repo.git",
		"Homepage":    "https://example.com/home",
		"Docs":        "https://example.com/docs",
	})
	if len(hints) != 3 {
		t.Fatalf("pypiSourceHints() = %d, want 3", len(hints))
	}
	byReason := map[string]SourceHintObservation{}
	for _, hint := range hints {
		byReason[hint.ConfidenceReason] = hint
	}
	if byReason["pypi-project-url:source code"].HintKind != "repository" {
		t.Fatalf("source hint = %+v, want repository kind", byReason["pypi-project-url:source code"])
	}
	if byReason["pypi-project-url:homepage"].HintKind != "homepage" {
		t.Fatalf("homepage hint = %+v, want homepage kind", byReason["pypi-project-url:homepage"])
	}
	if byReason["pypi-project-url:docs"].HintKind != "project" {
		t.Fatalf("docs hint = %+v, want project kind", byReason["pypi-project-url:docs"])
	}
	if out := pypiSourceHints(ctx, identity, "2.0.0", nil); len(out) != 0 {
		t.Fatalf("pypiSourceHints() nil = %+v, want empty", out)
	}
}

func TestMetadataHelperScalarHelpers(t *testing.T) {
	t.Parallel()

	var acc []string
	acc = appendUniqueString(acc, "")
	if len(acc) != 0 {
		t.Fatalf("appendUniqueString(empty value) = %v", acc)
	}
	acc = appendUniqueString(acc, "a")
	acc = appendUniqueString(acc, "a")
	acc = appendUniqueString(acc, "b")
	if len(acc) != 2 || acc[0] != "a" || acc[1] != "b" {
		t.Fatalf("appendUniqueString() = %v, want [a b]", acc)
	}

	versions := []PackageVersionObservation{{Version: "9.9.9"}}
	if got := firstParsedVersion(versions, "fallback"); got != "9.9.9" {
		t.Fatalf("firstParsedVersion() = %q, want first version", got)
	}
	if got := firstParsedVersion(nil, "  fallback  "); got != "fallback" {
		t.Fatalf("firstParsedVersion() fallback = %q, want trimmed", got)
	}

	if got := parseTimestamp(""); !got.IsZero() {
		t.Fatalf("parseTimestamp(blank) = %v, want zero", got)
	}
	if got := parseTimestamp("not-a-time"); !got.IsZero() {
		t.Fatalf("parseTimestamp(invalid) = %v, want zero", got)
	}
	want := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	if got := parseTimestamp("2026-03-04T05:06:07Z"); !got.Equal(want) {
		t.Fatalf("parseTimestamp(valid) = %v, want %v", got, want)
	}

	if parseVisibility("public") != VisibilityPublic || parseVisibility(" PUBLIC ") != VisibilityPublic {
		t.Fatal("parseVisibility(public) must be public")
	}
	if parseVisibility("private") != VisibilityPrivate {
		t.Fatal("parseVisibility(private) must be private")
	}
	for _, raw := range []string{"", "unknown", "internal", "bogus"} {
		if got := parseVisibility(raw); got != VisibilityUnknown {
			t.Fatalf("parseVisibility(%q) = %q, want unknown", raw, got)
		}
	}
}

func TestParsePyPIProjectMetadataCoverage(t *testing.T) {
	t.Parallel()

	ctx := ratchetParserContext(EcosystemPyPI)
	doc := `{"info": {"name": "requests", "version": "2.0.0",
		"requires_dist": ["urllib3>=1.21.1,<3", ""],
		"project_urls": {"Source": "git+https://example.com/psf/requests.git", "Homepage": "https://example.com"}},
		"releases": {"2.0.0": [
			{"packagetype": "bdist_wheel", "url": "https://example.com/requests-2.0.0-py3-none-any.whl",
				"filename": "requests-2.0.0-py3-none-any.whl", "size": 42,
				"upload_time_iso_8601": "2026-01-02T03:04:05Z", "yanked": true,
				"digests": {"sha256": "abc"}},
			{"packagetype": "sdist", "url": "not a url at all",
				"filename": "requests-2.0.0.tar.gz", "size": 7,
				"upload_time_iso_8601": "bogus", "digests": {}}]}}`
	parsed, err := ParsePyPIProjectMetadata(ctx, []byte(doc))
	if err != nil {
		t.Fatalf("ParsePyPIProjectMetadata() error = %v", err)
	}
	if len(parsed.Packages) != 1 || len(parsed.Versions) != 1 {
		t.Fatalf("packages/versions = %d/%d, want 1/1", len(parsed.Packages), len(parsed.Versions))
	}
	version := parsed.Versions[0]
	if !version.Yanked || version.PublishedAt.IsZero() {
		t.Fatalf("version = %+v, want yanked with PublishedAt", version)
	}
	if len(version.ArtifactURLs) != 2 {
		t.Fatalf("ArtifactURLs = %v, want both file URLs", version.ArtifactURLs)
	}
	if len(parsed.Artifacts) != 2 {
		t.Fatalf("artifacts = %d, want 2", len(parsed.Artifacts))
	}
	if len(parsed.Dependencies) != 1 || parsed.Dependencies[0].Dependency.RawName != "urllib3" {
		t.Fatalf("dependencies = %+v, want urllib3 only", parsed.Dependencies)
	}
	if len(parsed.SourceHints) != 2 {
		t.Fatalf("source hints = %d, want 2", len(parsed.SourceHints))
	}

	fallback, err := ParsePyPIProjectMetadata(ctx,
		[]byte(`{"info": {"name": "solo", "version": "0.1.0"}, "releases": {}}`))
	if err != nil {
		t.Fatalf("ParsePyPIProjectMetadata() fallback error = %v", err)
	}
	if len(fallback.Versions) != 1 || fallback.Versions[0].Version != "0.1.0" {
		t.Fatalf("fallback versions = %+v, want info.version", fallback.Versions)
	}

	for _, tc := range []struct {
		name string
		ctx  MetadataParserContext
		doc  string
	}{
		{"wrong ecosystem", ratchetParserContext(EcosystemNPM), `{"info":{"name":"x"}}`},
		{"malformed", ctx, `{oops`},
		{"blank name", ctx, `{"info": {"name": "  "}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := ParsePyPIProjectMetadata(tc.ctx, []byte(tc.doc)); err == nil {
				t.Fatalf("ParsePyPIProjectMetadata(%s) must fail", tc.name)
			}
		})
	}
}

func TestParseGenericPackageMetadataCoverage(t *testing.T) {
	t.Parallel()

	ctx := ratchetParserContext(EcosystemGeneric)
	doc := `{"provider": "acme", "repository": "widgets", "repository_type": "hosted",
		"name": "acme-widget", "namespace": "acme", "version": "3.0.0", "visibility": "PUBLIC",
		"source_url": "git+https://example.com/acme/widgets.git",
		"artifacts": [
			{"key": "a.tgz", "url": "https://example.com/a.tgz", "path": "a.tgz",
				"type": "tarball", "size": 9, "sha256": "abc", "hashes": {"sha512": "def"}},
			{"key": "a.tgz", "url": "https://example.com/a.tgz"},
			{"url": "https://example.com/b.tgz", "sha1": "s1", "md5": "m1"},
			{"key": "", "path": "", "url": "not a url at all"}],
		"vulnerabilities": [{"advisory_id": "ACME-1"}],
		"advisories": [{"advisory_id": "ACME-2"}],
		"events": [{"event_key": "e1", "event_type": "publish"}, {"event_key": "", "event_type": ""}]}`
	parsed, err := ParseGenericPackageMetadata(ctx, []byte(doc))
	if err != nil {
		t.Fatalf("ParseGenericPackageMetadata() error = %v", err)
	}
	if len(parsed.Packages) != 1 || parsed.Packages[0].Visibility != VisibilityPublic {
		t.Fatalf("packages = %+v, want public visibility", parsed.Packages)
	}
	if len(parsed.Versions) != 1 {
		t.Fatalf("versions = %d, want 1", len(parsed.Versions))
	}
	if len(parsed.Artifacts) != 3 {
		t.Fatalf("artifacts = %d, want 3 (duplicate + keyless skipped)", len(parsed.Artifacts))
	}
	if len(parsed.Vulnerables) != 2 || len(parsed.Events) != 1 {
		t.Fatalf("vulns/events = %d/%d, want 2/1", len(parsed.Vulnerables), len(parsed.Events))
	}
	if len(parsed.SourceHints) != 1 || len(parsed.Hosting) != 1 {
		t.Fatalf("hints/hosting = %d/%d, want 1/1", len(parsed.SourceHints), len(parsed.Hosting))
	}

	versionless, err := ParseGenericPackageMetadata(ctx, []byte(`{"name": "w", "visibility": "private"}`))
	if err != nil {
		t.Fatalf("ParseGenericPackageMetadata() versionless error = %v", err)
	}
	if len(versionless.Versions) != 0 || versionless.Packages[0].Visibility != VisibilityPrivate {
		t.Fatalf("versionless = %+v", versionless.Packages)
	}

	for _, tc := range []struct {
		name string
		ctx  MetadataParserContext
		doc  string
	}{
		{"wrong ecosystem", ratchetParserContext(EcosystemNPM), `{"name":"w"}`},
		{"malformed", ctx, `{oops`},
		{"blank name", ctx, `{"name": "  "}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := ParseGenericPackageMetadata(tc.ctx, []byte(tc.doc)); err == nil {
				t.Fatalf("ParseGenericPackageMetadata(%s) must fail", tc.name)
			}
		})
	}
}

func TestMetadataHelperMavenGoModuleCoverage(t *testing.T) {
	t.Parallel()

	ctx := ratchetParserContext(EcosystemMaven)
	deps := mavenDependencies(ctx,
		PackageIdentity{Ecosystem: EcosystemMaven, Registry: ctx.Registry, RawName: "app"},
		"1.0.0", []mavenDependency{
			{GroupID: "org.example", ArtifactID: "lib", Version: "2.0", Scope: "test",
				Optional: "true", Classifier: "sources"},
			{GroupID: "org.bare", ArtifactID: "core", Version: "1.0"},
			{GroupID: "", ArtifactID: "skip"},
			{GroupID: "org.skip", ArtifactID: ""},
		})
	if len(deps) != 2 {
		t.Fatalf("mavenDependencies() = %d, want 2", len(deps))
	}
	if deps[0].DependencyType != "test" || !deps[0].Optional || deps[0].Dependency.Classifier != "sources" {
		t.Fatalf("mavenDependencies()[0] = %+v", deps[0])
	}
	if deps[1].DependencyType != "compile" || deps[1].Optional {
		t.Fatalf("mavenDependencies()[1] = %+v, want compile default", deps[1])
	}

	withSCM := mavenSourceHints(ctx,
		PackageIdentity{Ecosystem: EcosystemMaven, Registry: ctx.Registry, RawName: "app"},
		"1.0.0", mavenProject{SCM: mavenSCM{URL: "scm:git:https://example.com/app.git"}})
	if len(withSCM) != 1 || withSCM[0].HintKind != "repository" || !strings.Contains(withSCM[0].RawURL, "example.com") {
		t.Fatalf("mavenSourceHints(scm) = %+v", withSCM)
	}
	withURL := mavenSourceHints(ctx,
		PackageIdentity{Ecosystem: EcosystemMaven, Registry: ctx.Registry, RawName: "app"},
		"1.0.0", mavenProject{URL: "https://example.com/app"})
	if len(withURL) != 1 || withURL[0].HintKind != "homepage" {
		t.Fatalf("mavenSourceHints(url) = %+v", withURL)
	}
	if out := mavenSourceHints(ctx,
		PackageIdentity{Ecosystem: EcosystemMaven, Registry: ctx.Registry, RawName: "app"},
		"1.0.0", mavenProject{}); len(out) != 0 {
		t.Fatalf("mavenSourceHints(empty) = %+v, want none", out)
	}

	gctx := ratchetParserContext(EcosystemGoModule)
	gdeps := goModuleDependencies(gctx,
		PackageIdentity{Ecosystem: EcosystemGoModule, Registry: gctx.Registry, RawName: "example.com/mod"},
		"v1.0.0", "module example.com/mod\n\nrequire (\n\tz.example/a v1.0.0\n\ta.example/b v2.0.0 // indirect\n)\n")
	if len(gdeps) != 2 {
		t.Fatalf("goModuleDependencies() = %d, want 2", len(gdeps))
	}
	if gdeps[0].Dependency.RawName != "a.example/b" || gdeps[0].DependencyType != "indirect" {
		t.Fatalf("goModuleDependencies()[0] = %+v, want sorted indirect first", gdeps[0])
	}
	if gdeps[1].DependencyType != "runtime" {
		t.Fatalf("goModuleDependencies()[1] = %+v, want runtime", gdeps[1])
	}
	single := goModuleDependencies(gctx,
		PackageIdentity{Ecosystem: EcosystemGoModule, Registry: gctx.Registry, RawName: "example.com/mod"},
		"v1.0.0", "module example.com/mod\n\nrequire z.example/a v1.0.0\n")
	if len(single) != 1 || single[0].DependencyType != "runtime" {
		t.Fatalf("goModuleDependencies(single) = %+v", single)
	}
}

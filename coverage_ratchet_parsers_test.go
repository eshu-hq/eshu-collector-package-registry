// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"testing"
)

// TestRatchetNPMParserBranches pins the packument branches: malformed and
// blank-name rejection plus a full document covering deprecated forms, all
// four dependency sets, dist checksums, and both repository shapes.
func TestRatchetNPMParserBranches(t *testing.T) {
	t.Parallel()

	ctx := ratchetParserContext(EcosystemNPM)
	if _, err := ParseNPMPackumentMetadata(ctx, []byte(`{oops`)); err == nil {
		t.Fatal("ParseNPMPackumentMetadata(malformed) error = nil, want rejection")
	}
	if _, err := ParseNPMPackumentMetadata(ctx, []byte(`{"name":"  "}`)); err == nil {
		t.Fatal("ParseNPMPackumentMetadata(blank name) error = nil, want rejection")
	}
	document := `{"name": "left-pad", "versions": {
		"1.3.0": {
			"deprecated": true,
			"dependencies": {"b-lib": "^1.0.0", "a-lib": "~1.2.0"},
			"devDependencies": {"dev-lib": "3.0.0"},
			"optionalDependencies": {"opt-lib": "4.0.0"},
			"peerDependencies": {"peer-lib": ">=5.0.0"},
			"dist": {"tarball": "https://registry.npmjs.org/left-pad/-/left-pad-1.3.0.tgz", "integrity": "sha512-abc", "shasum": "def"},
			"repository": {"url": "git+https://github.com/example/left-pad.git"},
			"homepage": "https://example.invalid/left-pad"
		},
		"1.2.0": {"deprecated": "use 1.3.0", "repository": "https://github.com/example/left-pad", "dist": {}},
		"1.1.0": {}
	}}`
	parsed, err := ParseNPMPackumentMetadata(ctx, []byte(document))
	if err != nil {
		t.Fatalf("ParseNPMPackumentMetadata() error = %v", err)
	}
	if len(parsed.Packages) != 1 || len(parsed.Versions) != 3 {
		t.Fatalf("packages/versions = %d/%d, want 1/3", len(parsed.Packages), len(parsed.Versions))
	}
	byVersion := map[string]PackageVersionObservation{}
	for _, version := range parsed.Versions {
		byVersion[version.Version] = version
	}
	if !byVersion["1.3.0"].Deprecated || !byVersion["1.2.0"].Deprecated || byVersion["1.1.0"].Deprecated {
		t.Fatalf("deprecated flags = %v/%v/%v, want true/true/false",
			byVersion["1.3.0"].Deprecated, byVersion["1.2.0"].Deprecated, byVersion["1.1.0"].Deprecated)
	}
	if len(byVersion["1.3.0"].ArtifactURLs) != 1 || len(byVersion["1.3.0"].Checksums) != 2 {
		t.Fatalf("1.3.0 artifacts/checksums = %+v, want tarball plus integrity and sha1", byVersion["1.3.0"])
	}
	if len(parsed.Dependencies) != 5 {
		t.Fatalf("dependencies = %d, want 5 across the four sets", len(parsed.Dependencies))
	}
	if len(parsed.Artifacts) != 1 || parsed.Artifacts[0].ArtifactKey != "left-pad/-/left-pad-1.3.0.tgz" {
		t.Fatalf("artifacts = %+v, want the tarball path key", parsed.Artifacts)
	}
	if len(parsed.SourceHints) != 3 {
		t.Fatalf("hints = %d, want repository plus homepage for 1.3.0 and repository for 1.2.0", len(parsed.SourceHints))
	}
}

// TestRatchetPyPIParserBranches pins the PyPI branches: release files with
// yanked and timestamp handling, blank-URL files, the info-version fallback,
// requirement and project-URL mapping, and malformed/blank rejection.
func TestRatchetPyPIParserBranches(t *testing.T) {
	t.Parallel()

	ctx := ratchetParserContext(EcosystemPyPI)
	if _, err := ParsePyPIProjectMetadata(ctx, []byte(`{oops`)); err == nil {
		t.Fatal("ParsePyPIProjectMetadata(malformed) error = nil, want rejection")
	}
	if _, err := ParsePyPIProjectMetadata(ctx, []byte(`{"info":{"name":" "}}`)); err == nil {
		t.Fatal("ParsePyPIProjectMetadata(blank name) error = nil, want rejection")
	}
	document := `{"info": {"name": "requests", "version": "2.31.0",
		"requires_dist": ["urllib3>=1.21", "  ", "chardet; python_version<'3.0'"],
		"project_urls": {"Source": "https://github.com/psf/requests", "Homepage": "https://requests.readthedocs.io"}},
		"releases": {"2.31.0": [
			{"packagetype": "bdist_wheel", "url": "https://files.example/r-2.31-py3.whl", "filename": "r-2.31-py3.whl",
				"size": 100, "upload_time_iso_8601": "2023-01-01T00:00:00Z", "yanked": true, "digests": {"sha256": "abc"}},
			{"packagetype": "sdist", "url": "", "filename": "r.tar.gz", "size": 50,
				"upload_time_iso_8601": "bad-time", "yanked": false, "digests": {}}
		]}}`
	parsed, err := ParsePyPIProjectMetadata(ctx, []byte(document))
	if err != nil {
		t.Fatalf("ParsePyPIProjectMetadata() error = %v", err)
	}
	if len(parsed.Versions) != 1 {
		t.Fatalf("versions = %d, want 1", len(parsed.Versions))
	}
	version := parsed.Versions[0]
	if !version.Yanked || version.PublishedAt.IsZero() {
		t.Fatalf("version = %+v, want yanked with the first file timestamp", version)
	}
	if len(version.ArtifactURLs) != 1 {
		t.Fatalf("artifact URLs = %v, want only the non-blank file URL", version.ArtifactURLs)
	}
	if len(parsed.Artifacts) != 2 {
		t.Fatalf("artifacts = %d, want one per release file", len(parsed.Artifacts))
	}
	if len(parsed.Dependencies) != 2 || parsed.Dependencies[1].Marker != "python_version<'3.0'" {
		t.Fatalf("dependencies = %+v, want blank skipped and marker kept", parsed.Dependencies)
	}
	if len(parsed.SourceHints) != 2 {
		t.Fatalf("hints = %d, want repository plus homepage", len(parsed.SourceHints))
	}
	fallback, err := ParsePyPIProjectMetadata(ctx, []byte(`{"info": {"name": "x", "version": "9.9"}}`))
	if err != nil {
		t.Fatalf("ParsePyPIProjectMetadata(versionless) error = %v", err)
	}
	if len(fallback.Versions) != 1 || fallback.Versions[0].Version != "9.9" {
		t.Fatalf("fallback versions = %+v, want the info version", fallback.Versions)
	}
}

// TestRatchetGenericParserBranches pins the generic branches: vulnerability
// and advisory merging with skips, event filtering, artifact key fallback
// with duplicate and blank skips, visibility mapping, source URL, hosting,
// and malformed/blank rejection.
func TestRatchetGenericParserBranches(t *testing.T) {
	t.Parallel()

	ctx := ratchetParserContext(EcosystemGeneric)
	if _, err := ParseGenericPackageMetadata(ctx, []byte(`{oops`)); err == nil {
		t.Fatal("ParseGenericPackageMetadata(malformed) error = nil, want rejection")
	}
	if _, err := ParseGenericPackageMetadata(ctx, []byte(`{"name":""}`)); err == nil {
		t.Fatal("ParseGenericPackageMetadata(blank name) error = nil, want rejection")
	}
	document := `{"provider": "acme", "repository": "main", "repository_type": "remote",
		"name": "widget", "namespace": "acme", "version": "1.0", "visibility": "private",
		"source_url": "https://example.invalid/widget",
		"artifacts": [
			{"key": "a1", "url": "https://example.invalid/a1", "type": "binary", "size": 10, "sha256": "s", "hashes": {"md5": "m"}},
			{"key": "a1", "url": "https://example.invalid/a1b"},
			{"key": "", "path": "", "url": "http://[::1"}
		],
		"vulnerabilities": [
			{"advisory_id": "", "vulnerability_id": ""},
			{"advisory_id": "ADV-1", "vulnerability_id": "V-1", "source_severity": "high", "published_at": "2026-01-01T00:00:00Z"}
		],
		"advisories": [{"vulnerability_id": "V-2", "advisory_source": "acme-sec"}],
		"events": [
			{"event_key": "", "event_type": "publish"},
			{"event_key": "e1", "event_type": "publish", "actor": "bot", "occurred_at": "2026-02-02T00:00:00Z"}
		]}`
	parsed, err := ParseGenericPackageMetadata(ctx, []byte(document))
	if err != nil {
		t.Fatalf("ParseGenericPackageMetadata() error = %v", err)
	}
	if len(parsed.Packages) != 1 || parsed.Packages[0].Visibility != VisibilityPrivate {
		t.Fatalf("packages = %+v, want one private package", parsed.Packages)
	}
	if len(parsed.Versions) != 1 {
		t.Fatalf("versions = %d, want 1", len(parsed.Versions))
	}
	if len(parsed.Artifacts) != 1 || parsed.Artifacts[0].ArtifactKey != "a1" {
		t.Fatalf("artifacts = %+v, want only the first a1 entry", parsed.Artifacts)
	}
	if len(parsed.Artifacts[0].Hashes) != 2 {
		t.Fatalf("artifact hashes = %v, want sha256 plus md5", parsed.Artifacts[0].Hashes)
	}
	if len(parsed.Vulnerables) != 2 {
		t.Fatalf("vulnerables = %d, want the full entry plus the advisory", len(parsed.Vulnerables))
	}
	if len(parsed.Events) != 1 || parsed.Events[0].EventKey != "e1" {
		t.Fatalf("events = %+v, want only the keyed entry", parsed.Events)
	}
	if len(parsed.SourceHints) != 1 || parsed.SourceHints[0].HintKind != "repository" {
		t.Fatalf("hints = %+v, want the source-url repository hint", parsed.SourceHints)
	}
	if len(parsed.Hosting) != 1 || parsed.Hosting[0].Provider != "acme" || parsed.Hosting[0].Repository != "main" {
		t.Fatalf("hosting = %+v, want the acme main record", parsed.Hosting)
	}
	versionless, err := ParseGenericPackageMetadata(ctx, []byte(`{"name": "x"}`))
	if err != nil {
		t.Fatalf("ParseGenericPackageMetadata(versionless) error = %v", err)
	}
	if len(versionless.Versions) != 0 || len(versionless.Packages) != 1 {
		t.Fatalf("versionless = %+v, want package without versions", versionless.Packages)
	}
}

// TestRatchetMavenParserBranches pins the Maven branches: parent group and
// version fallback, packaging-derived artifacts, dependency scope and
// optional handling with blank skips, SCM and homepage hints, the
// version-less shape, and malformed/blank rejection.
func TestRatchetMavenParserBranches(t *testing.T) {
	t.Parallel()

	ctx := ratchetParserContext(EcosystemMaven)
	if _, err := ParseMavenPackageMetadata(ctx, []byte(`<oops`)); err == nil {
		t.Fatal("ParseMavenPackageMetadata(malformed) error = nil, want rejection")
	}
	if _, err := ParseMavenPackageMetadata(ctx, []byte(`<project><artifactId>x</artifactId></project>`)); err == nil {
		t.Fatal("ParseMavenPackageMetadata(blank group) error = nil, want rejection")
	}
	document := `<project><groupId>org.example</groupId><artifactId>demo</artifactId><version>1.0</version>
		<packaging>war</packaging><url>https://example.invalid/demo</url>
		<scm><connection>scm:git:https://github.com/example/demo.git</connection></scm>
		<dependencies>
		<dependency><groupId>org.x</groupId><artifactId>lib</artifactId><version>2.0</version><scope>test</scope><optional>true</optional></dependency>
		<dependency><groupId> </groupId><artifactId>blank</artifactId></dependency>
		</dependencies></project>`
	parsed, err := ParseMavenPackageMetadata(ctx, []byte(document))
	if err != nil {
		t.Fatalf("ParseMavenPackageMetadata() error = %v", err)
	}
	if len(parsed.Versions) != 1 || len(parsed.Artifacts) != 1 {
		t.Fatalf("versions/artifacts = %d/%d, want 1/1", len(parsed.Versions), len(parsed.Artifacts))
	}
	if parsed.Artifacts[0].ArtifactType != "war" || parsed.Artifacts[0].ArtifactKey != "org/example/demo/1.0/demo-1.0.war" {
		t.Fatalf("artifact = %+v, want the war layout key", parsed.Artifacts[0])
	}
	if len(parsed.Dependencies) != 1 {
		t.Fatalf("dependencies = %d, want 1 with the blank skipped", len(parsed.Dependencies))
	}
	dependency := parsed.Dependencies[0]
	if dependency.DependencyType != "test" || !dependency.Optional || dependency.Range != "2.0" {
		t.Fatalf("dependency = %+v, want test scope, optional, pinned range", dependency)
	}
	if len(parsed.SourceHints) != 2 || parsed.SourceHints[0].HintKind != "repository" || parsed.SourceHints[1].HintKind != "homepage" {
		t.Fatalf("hints = %+v, want scm repository plus homepage", parsed.SourceHints)
	}
	if parsed.SourceHints[0].RawURL != "https://github.com/example/demo" {
		t.Fatalf("scm hint = %q, want the prefix-stripped URL", parsed.SourceHints[0].RawURL)
	}
	inherited, err := ParseMavenPackageMetadata(ctx, []byte(`<project><artifactId>demo</artifactId><parent><groupId>org.p</groupId><version>9</version></parent></project>`))
	if err != nil {
		t.Fatalf("ParseMavenPackageMetadata(parent fallback) error = %v", err)
	}
	if len(inherited.Versions) != 1 || inherited.Versions[0].Version != "9" {
		t.Fatalf("inherited versions = %+v, want the parent version", inherited.Versions)
	}
	versionless, err := ParseMavenPackageMetadata(ctx, []byte(`<project><groupId>g</groupId><artifactId>a</artifactId></project>`))
	if err != nil {
		t.Fatalf("ParseMavenPackageMetadata(versionless) error = %v", err)
	}
	if len(versionless.Versions) != 0 || len(versionless.Packages) != 1 {
		t.Fatalf("versionless = %d/%d, want package without versions", len(versionless.Packages), len(versionless.Versions))
	}
}

// TestRatchetGoModParserBranches pins the module-proxy branches: info-time
// publishing, zip artifacts with sums, direct and indirect requirements,
// the version fallback and zip-less shapes, and malformed/blank rejection.
func TestRatchetGoModParserBranches(t *testing.T) {
	t.Parallel()

	ctx := ratchetParserContext(EcosystemGoModule)
	if _, err := ParseGoModuleProxyMetadata(ctx, []byte(`{oops`)); err == nil {
		t.Fatal("ParseGoModuleProxyMetadata(malformed) error = nil, want rejection")
	}
	if _, err := ParseGoModuleProxyMetadata(ctx, []byte(`{"module":"  "}`)); err == nil {
		t.Fatal("ParseGoModuleProxyMetadata(blank module) error = nil, want rejection")
	}
	document := `{"module": "example.invalid/mod", "info": {"Version": "v1.2.3", "Time": "2026-03-03T00:00:00Z"},
		"mod": "module example.invalid/mod\n\ngo 1.21\n\nrequire (\n\texample.invalid/a v1.0.0\n\texample.invalid/b v2.0.0 // indirect\n\tbadline\n)\n",
		"zip_url": "https://proxy.example/mod/@v/v1.2.3.zip", "sum": "h1:abc="}`
	parsed, err := ParseGoModuleProxyMetadata(ctx, []byte(document))
	if err != nil {
		t.Fatalf("ParseGoModuleProxyMetadata() error = %v", err)
	}
	if len(parsed.Versions) != 1 || parsed.Versions[0].PublishedAt.IsZero() {
		t.Fatalf("versions = %+v, want the info time published", parsed.Versions)
	}
	if len(parsed.Versions[0].ArtifactURLs) != 1 {
		t.Fatalf("artifact URLs = %v, want the zip URL", parsed.Versions[0].ArtifactURLs)
	}
	if len(parsed.Artifacts) != 1 || parsed.Artifacts[0].Hashes["sum"] != "h1:abc=" {
		t.Fatalf("artifacts = %+v, want the zip with the sum hash", parsed.Artifacts)
	}
	if len(parsed.Dependencies) != 2 {
		t.Fatalf("dependencies = %d, want direct plus indirect with the bad line skipped", len(parsed.Dependencies))
	}
	if parsed.Dependencies[0].Dependency.RawName != "example.invalid/a" || parsed.Dependencies[0].DependencyType != "runtime" {
		t.Fatalf("first dependency = %+v, want sorted direct runtime", parsed.Dependencies[0])
	}
	if parsed.Dependencies[1].DependencyType != "indirect" {
		t.Fatalf("second dependency = %+v, want indirect", parsed.Dependencies[1])
	}
	fallback, err := ParseGoModuleProxyMetadata(ctx, []byte(`{"module": "example.invalid/mod", "version": "v9.9.9", "info": {}, "mod": ""}`))
	if err != nil {
		t.Fatalf("ParseGoModuleProxyMetadata(fallback) error = %v", err)
	}
	if len(fallback.Versions) != 1 || fallback.Versions[0].Version != "v9.9.9" || !fallback.Versions[0].PublishedAt.IsZero() {
		t.Fatalf("fallback versions = %+v, want the bundle version without time", fallback.Versions)
	}
	if len(fallback.Artifacts) != 1 || fallback.Artifacts[0].ArtifactKey != "example.invalid/mod/@v/v9.9.9.zip" {
		t.Fatalf("fallback artifacts = %+v, want the conventional zip key", fallback.Artifacts)
	}
	if len(fallback.Artifacts[0].Hashes) != 0 {
		t.Fatalf("fallback hashes = %v, want none without a sum", fallback.Artifacts[0].Hashes)
	}
	if len(fallback.Dependencies) != 0 {
		t.Fatalf("fallback dependencies = %+v, want none for an empty mod file", fallback.Dependencies)
	}
}

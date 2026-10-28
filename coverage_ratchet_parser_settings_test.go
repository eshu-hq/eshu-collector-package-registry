// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func ratchetParserContext(ecosystem Ecosystem) MetadataParserContext {
	return MetadataParserContext{
		Ecosystem:           ecosystem,
		Registry:            "https://registry.example",
		ScopeID:             string(ecosystem) + "://ratchet-scope",
		GenerationID:        "ratchet-generation",
		CollectorInstanceID: "ratchet-collector",
		FencingToken:        7,
		ObservedAt:          time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC),
		SourceURI:           "https://registry.example/ratchet",
	}
}

func ratchetFactContext() FactContext {
	return FactContext{
		ScopeID:             "npm://ratchet-scope",
		GenerationID:        "ratchet-generation",
		CollectorInstanceID: "ratchet-collector",
		SourceSystem:        paritySourceSystem,
		ObservedAt:          time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC),
		SourceURI:           "https://registry.example/ratchet",
	}
}

// TestRatchetArtifactoryParser pins the wrapper paths: native object
// metadata, string-wrapped native metadata, hosting propagation, mismatch
// rejection, and malformed or missing metadata.
func TestRatchetArtifactoryParser(t *testing.T) {
	t.Parallel()

	native := `{"name":"left-pad","versions":{"1.3.0":{}}}`
	wrapper := `{"provider":"","repository":"npm-remote","repository_type":"remote",` +
		`"package_type":"npm","upstream_id":"left-pad","upstream_url":"https://registry.npmjs.org/left-pad",` +
		`"metadata":` + native + `}`
	parsed, err := ParseArtifactoryPackageMetadata(ratchetParserContext(EcosystemNPM), []byte(wrapper))
	if err != nil {
		t.Fatalf("ParseArtifactoryPackageMetadata() error = %v", err)
	}
	if len(parsed.Packages) != 1 {
		t.Fatalf("packages = %d, want 1", len(parsed.Packages))
	}
	if len(parsed.Hosting) != 1 {
		t.Fatalf("hosting = %d, want repository hosting", len(parsed.Hosting))
	}
	if parsed.Hosting[0].Provider != "jfrog" || parsed.Hosting[0].Repository != "npm-remote" {
		t.Fatalf("hosting = %+v, want jfrog npm-remote", parsed.Hosting[0])
	}

	quoted := `{"repository":"npm-remote","package_type":"npm","metadata":"` +
		strings.ReplaceAll(native, `"`, `\"`) + `"}`
	if _, err := ParseArtifactoryPackageMetadata(ratchetParserContext(EcosystemNPM), []byte(quoted)); err != nil {
		t.Fatalf("ParseArtifactoryPackageMetadata(string metadata) error = %v", err)
	}

	unhosted := `{"package_type":"npm","metadata":` + native + `}`
	unhostedParsed, err := ParseArtifactoryPackageMetadata(ratchetParserContext(EcosystemNPM), []byte(unhosted))
	if err != nil {
		t.Fatalf("ParseArtifactoryPackageMetadata(no repository) error = %v", err)
	}
	if len(unhostedParsed.Hosting) != 0 {
		t.Fatalf("hosting = %d, want none without repository", len(unhostedParsed.Hosting))
	}

	mismatched := strings.ReplaceAll(wrapper, `"package_type":"npm"`, `"package_type":"pypi"`)
	if _, err := ParseArtifactoryPackageMetadata(ratchetParserContext(EcosystemNPM), []byte(mismatched)); err == nil {
		t.Fatal("ParseArtifactoryPackageMetadata(mismatch) error = nil, want rejection")
	} else if !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("mismatch error = %q, want package_type branch", err)
	}

	if _, err := ParseArtifactoryPackageMetadata(ratchetParserContext(EcosystemNPM), []byte(`{oops`)); err == nil {
		t.Fatal("ParseArtifactoryPackageMetadata(malformed) error = nil, want rejection")
	}
	if _, err := ParseArtifactoryPackageMetadataWithRegistry(ratchetParserContext(EcosystemNPM), []byte(`{"package_type":"npm","metadata":null}`), DefaultMetadataParserRegistry()); err == nil {
		t.Fatal("ParseArtifactoryPackageMetadata(null metadata) error = nil, want rejection")
	}
	if _, err := ParseArtifactoryPackageMetadata(ratchetParserContext(EcosystemNPM), []byte(`{"package_type":"npm","metadata":"  "}`)); err == nil {
		t.Fatal("ParseArtifactoryPackageMetadata(blank metadata) error = nil, want rejection")
	}
}

// TestRatchetNuGetParser pins grouped and direct dependencies, blank-id
// skipping, source hints, and identity rejection.
func TestRatchetNuGetParser(t *testing.T) {
	t.Parallel()

	document := `<package><metadata><id>Newtonsoft.Json</id><version>13.0.1</version>` +
		`<projectUrl>https://github.com/JamesNK/Newtonsoft.Json</projectUrl>` +
		`<repository url="https://github.com/JamesNK/Newtonsoft.Json" type="git"/>` +
		`<dependencies><group targetFramework="netstandard2.0">` +
		`<dependency id="System.Text.Json" version="8.0.0"/>` +
		`<dependency id=" " version="1.0.0"/>` +
		`</group><dependency id="Direct.Dep" version="2.0.0"/></dependencies>` +
		`</metadata></package>`
	parsed, err := ParseNuGetPackageMetadata(ratchetParserContext(EcosystemNuGet), []byte(document))
	if err != nil {
		t.Fatalf("ParseNuGetPackageMetadata() error = %v", err)
	}
	if len(parsed.Versions) != 1 || len(parsed.Artifacts) != 1 {
		t.Fatalf("versions/artifacts = %d/%d, want 1/1", len(parsed.Versions), len(parsed.Artifacts))
	}
	if len(parsed.Dependencies) != 2 {
		t.Fatalf("dependencies = %d, want 2 with blank skipped", len(parsed.Dependencies))
	}
	if parsed.Dependencies[0].TargetFramework != "netstandard2.0" {
		t.Fatalf("group framework = %q, want netstandard2.0", parsed.Dependencies[0].TargetFramework)
	}
	if len(parsed.SourceHints) != 2 {
		t.Fatalf("source hints = %d, want repository plus homepage", len(parsed.SourceHints))
	}
	if _, err := ParseNuGetPackageMetadata(ratchetParserContext(EcosystemNuGet), []byte(`<package><metadata><version>1.0.0</version></metadata></package>`)); err == nil {
		t.Fatal("ParseNuGetPackageMetadata(blank id) error = nil, want rejection")
	}
	versionless, err := ParseNuGetPackageMetadata(ratchetParserContext(EcosystemNuGet), []byte(`<package><metadata><id>x</id></metadata></package>`))
	if err != nil {
		t.Fatalf("ParseNuGetPackageMetadata(versionless) error = %v, want version-optional success", err)
	}
	if len(versionless.Packages) != 1 || len(versionless.Versions) != 0 {
		t.Fatalf("versionless packages/versions = %d/%d, want 1/0", len(versionless.Packages), len(versionless.Versions))
	}
	if _, ok := nugetDependencyObservation(ratchetParserContext(EcosystemNuGet), PackageIdentity{}, "1.0.0", nugetDependency{}); ok {
		t.Fatal("nugetDependencyObservation(blank) = true, want false")
	}
	emptyHints := nugetSourceHints(ratchetParserContext(EcosystemNuGet), PackageIdentity{}, "1.0.0", nugetMetadata{})
	if len(emptyHints) != 0 {
		t.Fatalf("nugetSourceHints(empty) = %d, want 0", len(emptyHints))
	}
}

// TestRatchetParserRegistryGuards pins the registry rejection branches.
func TestRatchetParserRegistryGuards(t *testing.T) {
	t.Parallel()

	var registry MetadataParserRegistry
	if err := registry.Register(Ecosystem(""), ParseNPMPackumentMetadata); err == nil {
		t.Fatal("Register(blank ecosystem) error = nil, want rejection")
	}
	if err := registry.Register(EcosystemNPM, nil); err == nil {
		t.Fatal("Register(nil parser) error = nil, want rejection")
	}
	if err := registry.Register(EcosystemNPM, ParseNPMPackumentMetadata); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := registry.Register(EcosystemNPM, ParseNPMPackumentMetadata); err == nil {
		t.Fatal("Register(duplicate) error = nil, want rejection")
	}
	if _, err := registry.Parse(MetadataParserContext{}, []byte(`{}`)); err == nil {
		t.Fatal("Parse(blank ecosystem) error = nil, want rejection")
	}
}

// TestRatchetSettingsRejections pins the settings fail-closed branches.
func TestRatchetSettingsRejections(t *testing.T) {
	t.Parallel()

	t.Run("unsupported_visibility", func(t *testing.T) {
		t.Parallel()
		cfg := validTargetConfig()
		cfg["visibility"] = "classified"
		if _, err := ParseSettings(map[string]any{"targets": []any{cfg}}, "collector-1", func(string) string { return "" }); err == nil {
			t.Fatal("ParseSettings(bad visibility) error = nil, want rejection")
		} else if !strings.Contains(err.Error(), "unsupported visibility") {
			t.Fatalf("error = %q, want visibility branch", err)
		}
	})

	t.Run("missing_ecosystem", func(t *testing.T) {
		t.Parallel()
		cfg := validTargetConfig()
		delete(cfg, "ecosystem")
		if _, err := ParseSettings(map[string]any{"targets": []any{cfg}}, "collector-1", func(string) string { return "" }); err == nil {
			t.Fatal("ParseSettings(missing ecosystem) error = nil, want rejection")
		} else if !strings.Contains(err.Error(), "ecosystem is required") {
			t.Fatalf("error = %q, want required branch", err)
		}
	})

	t.Run("blank_collector", func(t *testing.T) {
		t.Parallel()
		if _, err := ParseSettings(map[string]any{"targets": []any{validTargetConfig()}}, "  ", func(string) string { return "" }); err == nil {
			t.Fatal("ParseSettings(blank collector) error = nil, want rejection")
		}
	})

	t.Run("empty_config", func(t *testing.T) {
		t.Parallel()
		if _, err := ParseSettings(map[string]any{}, "collector-1", func(string) string { return "" }); err == nil {
			t.Fatal("ParseSettings(empty) error = nil, want rejection")
		} else if !strings.Contains(err.Error(), "at least one") {
			t.Fatalf("error = %q, want at-least-one branch", err)
		}
	})

	t.Run("non_serializable", func(t *testing.T) {
		t.Parallel()
		raw := map[string]any{"targets": []any{validTargetConfig()}, "poison": make(chan int)}
		if _, err := ParseSettings(raw, "collector-1", func(string) string { return "" }); err == nil {
			t.Fatal("ParseSettings(chan) error = nil, want rejection")
		} else if !strings.Contains(err.Error(), "not JSON serializable") {
			t.Fatalf("error = %q, want serializable branch", err)
		}
	})

	t.Run("limit_ceilings", func(t *testing.T) {
		t.Parallel()
		limits := map[string]any{
			"maxPayloadBytes":     maxPayloadCeiling + 1,
			"maxResultBytes":      maxResultCeiling + 1,
			"claimTimeoutSeconds": maxTimeoutCeiling + 1,
		}
		for key, value := range limits {
			bad := map[string]any{
				"targets": []any{validTargetConfig()},
				"limits":  map[string]any{key: value},
			}
			if _, err := ParseSettings(bad, "collector-1", func(string) string { return "" }); err == nil {
				t.Fatalf("ParseSettings(%s over ceiling) error = nil, want rejection", key)
			}
		}
	})

	t.Run("config_error_class", func(t *testing.T) {
		t.Parallel()
		custom := ConfigError{msg: "probe", class: "custom-class"}
		if custom.FailureClass() != "custom-class" {
			t.Fatalf("FailureClass() = %q, want custom-class", custom.FailureClass())
		}
		if custom.Error() != "probe" {
			t.Fatalf("Error() = %q, want probe", custom.Error())
		}
		if got := (ConfigError{msg: "probe"}).FailureClass(); got != FailureInvalidConfig {
			t.Fatalf("FailureClass() = %q, want %q", got, FailureInvalidConfig)
		}
	})
}

// TestRatchetWarningMapping pins the prefetch and parser warning branches.
func TestRatchetWarningMapping(t *testing.T) {
	t.Parallel()

	ctx := ratchetFactContext()
	private := FetchTarget{
		Base:        TargetConfig{Ecosystem: EcosystemNPM, Visibility: VisibilityPrivate, ScopeID: "npm://ratchet"},
		MetadataURL: "https://registry.example/left-pad",
	}
	warning, err := prefetchWarningFact(private, ctx)
	if err != nil {
		t.Fatalf("prefetchWarningFact(private) error = %v", err)
	}
	if warning == nil {
		t.Fatal("prefetchWarningFact(private) = nil, want credentials_missing warning")
	}

	native := FetchTarget{
		Base: TargetConfig{Ecosystem: EcosystemMaven, Visibility: VisibilityPublic, ScopeID: "maven://ratchet"},
	}
	warning, err = prefetchWarningFact(native, ctx)
	if err != nil {
		t.Fatalf("prefetchWarningFact(native) error = %v", err)
	}
	if warning == nil {
		t.Fatal("prefetchWarningFact(maven without URL) = nil, want unsupported warning")
	}

	fetchable := FetchTarget{
		Base:        TargetConfig{Ecosystem: EcosystemNPM, Visibility: VisibilityPublic, ScopeID: "npm://ratchet"},
		MetadataURL: "https://registry.example/left-pad",
	}
	warning, err = prefetchWarningFact(fetchable, ctx)
	if err != nil || warning != nil {
		t.Fatalf("prefetchWarningFact(fetchable) = %v, %v, want nil, nil", warning, err)
	}

	target := FetchTarget{Base: TargetConfig{Ecosystem: EcosystemCargo, Visibility: VisibilityPublic, ScopeID: "cargo://ratchet"}}
	unregistered := errors.New(`metadata parser for ecosystem "cargo" is not registered`)
	fact, handled, err := parserWarningFact(target, ctx, unregistered)
	if err != nil || !handled {
		t.Fatalf("parserWarningFact(unregistered) = %v, %v, %v, want handled fact", fact, handled, err)
	}
	if fact.Payload["warning_code"] != warningCodeUnsupportedMetadataSource {
		t.Fatalf("warning_code = %#v, want unsupported", fact.Payload["warning_code"])
	}
	malformed, handled, err := parserWarningFact(target, ctx, errors.New("boom"))
	if err != nil || !handled {
		t.Fatalf("parserWarningFact(malformed) = %v, %v, %v, want handled fact", malformed, handled, err)
	}
	if malformed.Payload["warning_code"] != warningCodeMalformedMetadata {
		t.Fatalf("warning_code = %#v, want malformed", malformed.Payload["warning_code"])
	}
	if got := parserWarningCode(unregistered); got != warningCodeUnsupportedMetadataSource {
		t.Fatalf("parserWarningCode() = %q, want unsupported", got)
	}

	missing, err := missingMetadataWarningFact(target, ctx)
	if err != nil {
		t.Fatalf("missingMetadataWarningFact() error = %v", err)
	}
	if missing.Payload["warning_code"] != warningCodeMetadataNotFound {
		t.Fatalf("warning_code = %#v, want not_found", missing.Payload["warning_code"])
	}
	tooLarge, err := metadataTooLargeWarningFact(target, ctx)
	if err != nil {
		t.Fatalf("metadataTooLargeWarningFact() error = %v", err)
	}
	if tooLarge.Payload["warning_code"] != warningCodeMetadataTooLarge {
		t.Fatalf("warning_code = %#v, want too_large", tooLarge.Payload["warning_code"])
	}
}

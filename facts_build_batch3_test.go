// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"strings"
	"testing"
)

// TestValidateFactContextRejectsBlankFields pins the fail-closed claim
// boundary: each blank boundary field is rejected with a named error so a
// fact can never be emitted outside its scope or generation.
func TestValidateFactContextRejectsBlankFields(t *testing.T) {
	t.Parallel()

	base := testFactContext()
	cases := []struct {
		name   string
		mutate func(*FactContext)
		want   string
	}{
		{"scope", func(ctx *FactContext) { ctx.ScopeID = "" }, "scope_id"},
		{"scope_whitespace", func(ctx *FactContext) { ctx.ScopeID = "   " }, "scope_id"},
		{"generation", func(ctx *FactContext) { ctx.GenerationID = "" }, "generation_id"},
		{"collector_instance", func(ctx *FactContext) { ctx.CollectorInstanceID = "" }, "collector_instance_id"},
		{"source_system", func(ctx *FactContext) { ctx.SourceSystem = "" }, "source_system"},
		{"source_system_whitespace", func(ctx *FactContext) { ctx.SourceSystem = "  " }, "source_system"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := base
			tc.mutate(&ctx)
			err := validateFactContext(ctx)
			if err == nil {
				t.Fatalf("validateFactContext() = nil, want error mentioning %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("validateFactContext() error = %q, want it to mention %q", err.Error(), tc.want)
			}
		})
	}
}

// TestValidateFactContextAcceptsValid pins the happy path: a fully populated
// claim boundary passes validation.
func TestValidateFactContextAcceptsValid(t *testing.T) {
	t.Parallel()

	if err := validateFactContext(testFactContext()); err != nil {
		t.Fatalf("validateFactContext() error = %v, want nil", err)
	}
}

// TestWithFactContextStampsBlankFields pins the safety-net stamp for package
// observations built off the parser path.
func TestWithFactContextStampsBlankFields(t *testing.T) {
	t.Parallel()

	ctx := testFactContext()
	got := withFactContext(PackageObservation{}, ctx)
	if got.ScopeID != ctx.ScopeID || got.GenerationID != ctx.GenerationID || got.CollectorInstanceID != ctx.CollectorInstanceID {
		t.Fatalf("boundary not stamped: %+v", got)
	}
	if got.SourceURI != ctx.SourceURI || !got.ObservedAt.Equal(ctx.ObservedAt) {
		t.Fatalf("source not stamped: %+v", got)
	}
}

// TestWithFactContextPreservesExisting pins that parser-set package boundary
// fields survive the safety net.
func TestWithFactContextPreservesExisting(t *testing.T) {
	t.Parallel()

	scope, generation, instance, uri, observedAt := existingBoundary()
	got := withFactContext(PackageObservation{
		ScopeID: scope, GenerationID: generation, CollectorInstanceID: instance,
		SourceURI: uri, ObservedAt: observedAt,
	}, testFactContext())
	if got.ScopeID != scope || got.GenerationID != generation || got.CollectorInstanceID != instance {
		t.Fatalf("boundary overwritten: %+v", got)
	}
	if got.SourceURI != uri || !got.ObservedAt.Equal(observedAt) {
		t.Fatalf("source overwritten: %+v", got)
	}
}

// TestWithFactContextVersionStampsBlankFields pins the safety-net stamp for
// package version observations assembled by warning paths.
func TestWithFactContextVersionStampsBlankFields(t *testing.T) {
	t.Parallel()

	ctx := testFactContext()
	got := withFactContextVersion(PackageVersionObservation{}, ctx)
	if got.ScopeID != ctx.ScopeID || got.GenerationID != ctx.GenerationID || got.CollectorInstanceID != ctx.CollectorInstanceID {
		t.Fatalf("boundary not stamped: %+v", got)
	}
	if got.SourceURI != ctx.SourceURI || !got.ObservedAt.Equal(ctx.ObservedAt) {
		t.Fatalf("source not stamped: %+v", got)
	}
}

// TestWithFactContextVersionPreservesExisting pins that parser-set version
// boundary fields survive the safety net.
func TestWithFactContextVersionPreservesExisting(t *testing.T) {
	t.Parallel()

	scope, generation, instance, uri, observedAt := existingBoundary()
	got := withFactContextVersion(PackageVersionObservation{
		ScopeID: scope, GenerationID: generation, CollectorInstanceID: instance,
		SourceURI: uri, ObservedAt: observedAt,
	}, testFactContext())
	if got.ScopeID != scope || got.GenerationID != generation || got.CollectorInstanceID != instance {
		t.Fatalf("boundary overwritten: %+v", got)
	}
	if got.SourceURI != uri || !got.ObservedAt.Equal(observedAt) {
		t.Fatalf("source overwritten: %+v", got)
	}
}

// TestWithFactContextArtifactStampsBlankFields pins the safety-net stamp for
// package artifact observations assembled off the parser path.
func TestWithFactContextArtifactStampsBlankFields(t *testing.T) {
	t.Parallel()

	ctx := testFactContext()
	got := withFactContextArtifact(PackageArtifactObservation{}, ctx)
	if got.ScopeID != ctx.ScopeID || got.GenerationID != ctx.GenerationID || got.CollectorInstanceID != ctx.CollectorInstanceID {
		t.Fatalf("boundary not stamped: %+v", got)
	}
	if got.SourceURI != ctx.SourceURI || !got.ObservedAt.Equal(ctx.ObservedAt) {
		t.Fatalf("source not stamped: %+v", got)
	}
}

// TestWithFactContextArtifactPreservesExisting pins that parser-set artifact
// boundary fields survive the safety net.
func TestWithFactContextArtifactPreservesExisting(t *testing.T) {
	t.Parallel()

	scope, generation, instance, uri, observedAt := existingBoundary()
	got := withFactContextArtifact(PackageArtifactObservation{
		ScopeID: scope, GenerationID: generation, CollectorInstanceID: instance,
		SourceURI: uri, ObservedAt: observedAt,
	}, testFactContext())
	if got.ScopeID != scope || got.GenerationID != generation || got.CollectorInstanceID != instance {
		t.Fatalf("boundary overwritten: %+v", got)
	}
	if got.SourceURI != uri || !got.ObservedAt.Equal(observedAt) {
		t.Fatalf("source overwritten: %+v", got)
	}
}

// batch3ParsedMetadata returns one valid observation per fact kind with blank
// boundary fields, so buildFacts must stamp every observation with the claim
// boundary before emitting envelopes.
func batch3ParsedMetadata() ParsedMetadata {
	identity := parityIdentity()
	dependency := PackageIdentity{Ecosystem: EcosystemNPM, Registry: "https://registry.npmjs.org", RawName: "concat-stream"}
	return ParsedMetadata{
		Packages:     []PackageObservation{{Identity: identity}},
		Versions:     []PackageVersionObservation{{Package: identity, Version: "1.3.0"}},
		Dependencies: []PackageDependencyObservation{{Package: identity, Version: "1.3.0", Dependency: dependency, Range: "^1.6.0", DependencyType: "runtime"}},
		Artifacts:    []PackageArtifactObservation{{Package: identity, Version: "1.3.0", ArtifactKey: "left-pad-1.3.0.tgz"}},
		SourceHints:  []SourceHintObservation{{Package: identity, Version: "1.3.0", HintKind: "repository", RawURL: "https://github.com/example/left-pad"}},
		Vulnerables:  []VulnerabilityHintObservation{{Package: identity, Version: "1.3.0", AdvisoryID: "GHSA-xxxx-yyyy-zzzz", AdvisorySource: "github"}},
		Events:       []RegistryEventObservation{{Package: identity, Version: "1.3.0", EventKey: "left-pad@1.3.0:publish", EventType: "publish"}},
		Hosting:      []RepositoryHostingObservation{{Provider: "npm", Registry: "https://registry.npmjs.org", Repository: "registry", RepositoryType: "package-feed", Ecosystem: EcosystemNPM}},
		Warnings:     []WarningObservation{{WarningKey: "batch3-warning", WarningCode: "metadata_truncated"}},
	}
}

// TestBuildFactsEmitsAllKindsInOrder pins the envelope emission order
// buildFacts promises: package, version, dependency, artifact, source hint,
// vulnerability hint, registry event, hosting, then warning observations.
func TestBuildFactsEmitsAllKindsInOrder(t *testing.T) {
	t.Parallel()

	ctx := testFactContext()
	facts, err := buildFacts(ctx, batch3ParsedMetadata())
	if err != nil {
		t.Fatalf("buildFacts() error = %v", err)
	}
	wantKinds := []string{
		FactKindPackage,
		FactKindPackageVersion,
		FactKindPackageDependency,
		FactKindPackageArtifact,
		FactKindSourceHint,
		FactKindVulnerabilityHint,
		FactKindRegistryEvent,
		FactKindRepositoryHosting,
		FactKindWarning,
	}
	if len(facts) != len(wantKinds) {
		t.Fatalf("buildFacts() emitted %d facts, want %d", len(facts), len(wantKinds))
	}
	for i, want := range wantKinds {
		if facts[i].Kind != want {
			t.Fatalf("facts[%d].Kind = %q, want %q", i, facts[i].Kind, want)
		}
		if facts[i].SourceRef.ScopeID != ctx.ScopeID {
			t.Fatalf("facts[%d].SourceRef.ScopeID = %q, want %q", i, facts[i].SourceRef.ScopeID, ctx.ScopeID)
		}
		if facts[i].SourceRef.GenerationID != ctx.GenerationID {
			t.Fatalf("facts[%d].SourceRef.GenerationID = %q, want %q", i, facts[i].SourceRef.GenerationID, ctx.GenerationID)
		}
		if facts[i].StableKey == "" {
			t.Fatalf("facts[%d].StableKey is empty for kind %q", i, want)
		}
	}
}

// TestBuildFactsRejectsInvalidContext pins the fail-closed entry: a claim
// boundary that fails validation emits no facts.
func TestBuildFactsRejectsInvalidContext(t *testing.T) {
	t.Parallel()

	ctx := testFactContext()
	ctx.ScopeID = ""
	facts, err := buildFacts(ctx, batch3ParsedMetadata())
	if err == nil {
		t.Fatal("buildFacts() = nil error, want invalid context error")
	}
	if facts != nil {
		t.Fatalf("buildFacts() facts = %v, want nil on invalid context", facts)
	}
}

// TestBuildFactsPropagatesEnvelopeError pins that one malformed observation
// fails the whole bounded collection: no partial fact set is returned.
func TestBuildFactsPropagatesEnvelopeError(t *testing.T) {
	t.Parallel()

	identity := parityIdentity()
	cases := []struct {
		name   string
		parsed ParsedMetadata
	}{
		{"package", ParsedMetadata{Packages: []PackageObservation{{Identity: PackageIdentity{}}}}},
		{"version", ParsedMetadata{Versions: []PackageVersionObservation{{Package: identity}}}},
		{"dependency", ParsedMetadata{Dependencies: []PackageDependencyObservation{{Package: identity, Version: "1.3.0"}}}},
		{"artifact", ParsedMetadata{Artifacts: []PackageArtifactObservation{{Package: identity, Version: "1.3.0"}}}},
		{"source_hint", ParsedMetadata{SourceHints: []SourceHintObservation{{Package: identity}}}},
		{"vulnerability", ParsedMetadata{Vulnerables: []VulnerabilityHintObservation{{Package: identity, AdvisorySource: "github"}}}},
		{"event", ParsedMetadata{Events: []RegistryEventObservation{{Package: identity, EventType: "publish"}}}},
		{"hosting", ParsedMetadata{Hosting: []RepositoryHostingObservation{{Registry: "https://registry.npmjs.org", Repository: "registry"}}}},
		{"warning", ParsedMetadata{Warnings: []WarningObservation{{WarningCode: "metadata_truncated"}}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			facts, err := buildFacts(testFactContext(), tc.parsed)
			if err == nil {
				t.Fatalf("buildFacts(%s) = nil error, want envelope error", tc.name)
			}
			if facts != nil {
				t.Fatalf("buildFacts(%s) facts = %v, want nil on envelope error", tc.name, facts)
			}
		})
	}
}

// TestBuildFactsEmptyYieldsNoFacts pins the degenerate remainder: an empty
// bounded collection builds zero facts without an error.
func TestBuildFactsEmptyYieldsNoFacts(t *testing.T) {
	t.Parallel()

	facts, err := buildFacts(testFactContext(), ParsedMetadata{})
	if err != nil {
		t.Fatalf("buildFacts() error = %v, want nil", err)
	}
	if len(facts) != 0 {
		t.Fatalf("buildFacts() emitted %d facts, want 0", len(facts))
	}
}

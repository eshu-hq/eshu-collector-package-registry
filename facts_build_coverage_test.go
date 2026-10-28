// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"testing"
	"time"
)

// testFactContext returns a fully populated claim boundary for the
// withFactContext family tests.
func testFactContext() FactContext {
	return FactContext{
		ScopeID:             "npm://registry.npmjs.org/left-pad",
		GenerationID:        "generation-1",
		CollectorInstanceID: "package-registry-qa",
		SourceSystem:        "package_registry",
		ObservedAt:          time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC),
		SourceURI:           "https://registry.npmjs.org/left-pad",
	}
}

// existingBoundary returns pre-filled boundary fields that must survive a
// withFactContext call unchanged.
func existingBoundary() (scope, generation, instance, uri string, observedAt time.Time) {
	return "preset-scope", "preset-generation", "preset-instance",
		"https://preset.invalid/pkg",
		time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
}

// TestWithFactContextDependencyStampsBlankFields pins the safety-net stamp:
// a parser that bypassed the parser context still emits facts under the
// claim boundary.
func TestWithFactContextDependencyStampsBlankFields(t *testing.T) {
	t.Parallel()

	ctx := testFactContext()
	got := withFactContextDependency(PackageDependencyObservation{}, ctx)
	if got.ScopeID != ctx.ScopeID {
		t.Fatalf("ScopeID=%q, want %q", got.ScopeID, ctx.ScopeID)
	}
	if got.GenerationID != ctx.GenerationID {
		t.Fatalf("GenerationID=%q, want %q", got.GenerationID, ctx.GenerationID)
	}
	if got.CollectorInstanceID != ctx.CollectorInstanceID {
		t.Fatalf("CollectorInstanceID=%q, want %q", got.CollectorInstanceID, ctx.CollectorInstanceID)
	}
	if got.SourceURI != ctx.SourceURI {
		t.Fatalf("SourceURI=%q, want %q", got.SourceURI, ctx.SourceURI)
	}
	if !got.ObservedAt.Equal(ctx.ObservedAt) {
		t.Fatalf("ObservedAt=%v, want %v", got.ObservedAt, ctx.ObservedAt)
	}
}

// TestWithFactContextDependencyPreservesExisting pins the other half: fields
// the parser already set are never overwritten by the claim boundary.
func TestWithFactContextDependencyPreservesExisting(t *testing.T) {
	t.Parallel()

	scope, generation, instance, uri, observedAt := existingBoundary()
	got := withFactContextDependency(PackageDependencyObservation{
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

// TestWithFactContextSourceHintStampsBlankFields pins the stamp for source,
// homepage, SCM, and provenance hints built off the parser path.
func TestWithFactContextSourceHintStampsBlankFields(t *testing.T) {
	t.Parallel()

	ctx := testFactContext()
	got := withFactContextSourceHint(SourceHintObservation{}, ctx)
	if got.ScopeID != ctx.ScopeID || got.GenerationID != ctx.GenerationID || got.CollectorInstanceID != ctx.CollectorInstanceID {
		t.Fatalf("boundary not stamped: %+v", got)
	}
	if got.SourceURI != ctx.SourceURI || !got.ObservedAt.Equal(ctx.ObservedAt) {
		t.Fatalf("source not stamped: %+v", got)
	}
}

// TestWithFactContextSourceHintPreservesExisting pins that parser-set hint
// boundary fields survive the safety net.
func TestWithFactContextSourceHintPreservesExisting(t *testing.T) {
	t.Parallel()

	scope, generation, instance, uri, observedAt := existingBoundary()
	got := withFactContextSourceHint(SourceHintObservation{
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

// TestWithFactContextVulnerabilityStampsBlankFields pins the stamp for
// advisory observations assembled by warning paths.
func TestWithFactContextVulnerabilityStampsBlankFields(t *testing.T) {
	t.Parallel()

	ctx := testFactContext()
	got := withFactContextVulnerability(VulnerabilityHintObservation{}, ctx)
	if got.ScopeID != ctx.ScopeID || got.GenerationID != ctx.GenerationID || got.CollectorInstanceID != ctx.CollectorInstanceID {
		t.Fatalf("boundary not stamped: %+v", got)
	}
	if got.SourceURI != ctx.SourceURI || !got.ObservedAt.Equal(ctx.ObservedAt) {
		t.Fatalf("source not stamped: %+v", got)
	}
}

// TestWithFactContextVulnerabilityPreservesExisting pins that parser-set
// advisory boundary fields survive the safety net.
func TestWithFactContextVulnerabilityPreservesExisting(t *testing.T) {
	t.Parallel()

	scope, generation, instance, uri, observedAt := existingBoundary()
	got := withFactContextVulnerability(VulnerabilityHintObservation{
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

// TestWithFactContextEventStampsBlankFields pins the stamp for registry
// lifecycle events (publish, delete, yank, relist, metadata mutation).
func TestWithFactContextEventStampsBlankFields(t *testing.T) {
	t.Parallel()

	ctx := testFactContext()
	got := withFactContextEvent(RegistryEventObservation{}, ctx)
	if got.ScopeID != ctx.ScopeID || got.GenerationID != ctx.GenerationID || got.CollectorInstanceID != ctx.CollectorInstanceID {
		t.Fatalf("boundary not stamped: %+v", got)
	}
	if got.SourceURI != ctx.SourceURI || !got.ObservedAt.Equal(ctx.ObservedAt) {
		t.Fatalf("source not stamped: %+v", got)
	}
}

// TestWithFactContextEventPreservesExisting pins that parser-set event
// boundary fields survive the safety net.
func TestWithFactContextEventPreservesExisting(t *testing.T) {
	t.Parallel()

	scope, generation, instance, uri, observedAt := existingBoundary()
	got := withFactContextEvent(RegistryEventObservation{
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

// TestWithFactContextHostingStampsBlankFields pins the stamp for
// provider-reported repository and feed topology records.
func TestWithFactContextHostingStampsBlankFields(t *testing.T) {
	t.Parallel()

	ctx := testFactContext()
	got := withFactContextHosting(RepositoryHostingObservation{}, ctx)
	if got.ScopeID != ctx.ScopeID || got.GenerationID != ctx.GenerationID || got.CollectorInstanceID != ctx.CollectorInstanceID {
		t.Fatalf("boundary not stamped: %+v", got)
	}
	if got.SourceURI != ctx.SourceURI || !got.ObservedAt.Equal(ctx.ObservedAt) {
		t.Fatalf("source not stamped: %+v", got)
	}
}

// TestWithFactContextHostingPreservesExisting pins that parser-set hosting
// boundary fields survive the safety net.
func TestWithFactContextHostingPreservesExisting(t *testing.T) {
	t.Parallel()

	scope, generation, instance, uri, observedAt := existingBoundary()
	got := withFactContextHosting(RepositoryHostingObservation{
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

// TestWithFactContextWarningStampsBlankFields pins the stamp for non-fatal
// collection warnings, which are built without a parser context.
func TestWithFactContextWarningStampsBlankFields(t *testing.T) {
	t.Parallel()

	ctx := testFactContext()
	got := withFactContextWarning(WarningObservation{}, ctx)
	if got.ScopeID != ctx.ScopeID || got.GenerationID != ctx.GenerationID || got.CollectorInstanceID != ctx.CollectorInstanceID {
		t.Fatalf("boundary not stamped: %+v", got)
	}
	if got.SourceURI != ctx.SourceURI || !got.ObservedAt.Equal(ctx.ObservedAt) {
		t.Fatalf("source not stamped: %+v", got)
	}
}

// TestWithFactContextWarningPreservesExisting pins that warning-path boundary
// fields survive the safety net.
func TestWithFactContextWarningPreservesExisting(t *testing.T) {
	t.Parallel()

	scope, generation, instance, uri, observedAt := existingBoundary()
	got := withFactContextWarning(WarningObservation{
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

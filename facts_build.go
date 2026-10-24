// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"fmt"
	"strings"
	"time"

	sdk "github.com/eshu-hq/eshu-sdk/collector"
)

// FactContext carries the claim boundary fields for one package-registry
// observation. The host, not this collector, derives fencing and commit
// mechanics from the claim, so none of those appear here.
type FactContext struct {
	ScopeID             string
	GenerationID        string
	CollectorInstanceID string
	SourceSystem        string
	ObservedAt          time.Time
	SourceURI           string
}

func validateFactContext(ctx FactContext) error {
	if strings.TrimSpace(ctx.ScopeID) == "" {
		return fmt.Errorf("package registry fact scope_id must not be blank")
	}
	if strings.TrimSpace(ctx.GenerationID) == "" {
		return fmt.Errorf("package registry fact generation_id must not be blank")
	}
	if strings.TrimSpace(ctx.CollectorInstanceID) == "" {
		return fmt.Errorf("package registry fact collector_instance_id must not be blank")
	}
	if strings.TrimSpace(ctx.SourceSystem) == "" {
		return fmt.Errorf("package registry fact source_system must not be blank")
	}
	return nil
}

// buildFacts converts one bounded collection into SDK facts in the same order
// the in-tree collector emitted envelopes: package, version, dependency,
// artifact, source hint, vulnerability hint, registry event, hosting, then
// warning observations.
func buildFacts(ctx FactContext, parsed ParsedMetadata) ([]sdk.Fact, error) {
	if err := validateFactContext(ctx); err != nil {
		return nil, err
	}
	facts := make([]sdk.Fact, 0,
		len(parsed.Packages)+len(parsed.Versions)+len(parsed.Dependencies)+
			len(parsed.Artifacts)+len(parsed.SourceHints)+len(parsed.Vulnerables)+
			len(parsed.Events)+len(parsed.Hosting)+len(parsed.Warnings))
	appendFact := func(fact sdk.Fact, err error) error {
		if err != nil {
			return err
		}
		if fact.SourceRef.ScopeID == "" {
			fact.SourceRef.ScopeID = ctx.ScopeID
		}
		if fact.SourceRef.GenerationID == "" {
			fact.SourceRef.GenerationID = ctx.GenerationID
		}
		facts = append(facts, fact)
		return nil
	}
	for _, observation := range parsed.Packages {
		if err := appendFact(NewPackageEnvelope(withFactContext(observation, ctx), ctx.SourceSystem)); err != nil {
			return nil, err
		}
	}
	for _, observation := range parsed.Versions {
		if err := appendFact(NewPackageVersionEnvelope(withFactContextVersion(observation, ctx), ctx.SourceSystem)); err != nil {
			return nil, err
		}
	}
	for _, observation := range parsed.Dependencies {
		if err := appendFact(NewPackageDependencyEnvelope(withFactContextDependency(observation, ctx), ctx.SourceSystem)); err != nil {
			return nil, err
		}
	}
	for _, observation := range parsed.Artifacts {
		if err := appendFact(NewPackageArtifactEnvelope(withFactContextArtifact(observation, ctx), ctx.SourceSystem)); err != nil {
			return nil, err
		}
	}
	for _, observation := range parsed.SourceHints {
		if err := appendFact(NewSourceHintEnvelope(withFactContextSourceHint(observation, ctx), ctx.SourceSystem)); err != nil {
			return nil, err
		}
	}
	for _, observation := range parsed.Vulnerables {
		if err := appendFact(NewVulnerabilityHintEnvelope(withFactContextVulnerability(observation, ctx), ctx.SourceSystem)); err != nil {
			return nil, err
		}
	}
	for _, observation := range parsed.Events {
		if err := appendFact(NewRegistryEventEnvelope(withFactContextEvent(observation, ctx), ctx.SourceSystem)); err != nil {
			return nil, err
		}
	}
	for _, observation := range parsed.Hosting {
		if err := appendFact(NewRepositoryHostingEnvelope(withFactContextHosting(observation, ctx), ctx.SourceSystem)); err != nil {
			return nil, err
		}
	}
	for _, observation := range parsed.Warnings {
		if err := appendFact(NewWarningEnvelope(withFactContextWarning(observation, ctx), ctx.SourceSystem)); err != nil {
			return nil, err
		}
	}
	return facts, nil
}

// The withFactContext family stamps the claim boundary onto observations the
// parsers produced. Parser contexts already carry the same boundary, so these
// are a safety net for observations built by warning paths that bypass the
// parser context.
func withFactContext(observation PackageObservation, ctx FactContext) PackageObservation {
	if observation.ScopeID == "" {
		observation.ScopeID = ctx.ScopeID
	}
	if observation.GenerationID == "" {
		observation.GenerationID = ctx.GenerationID
	}
	if observation.CollectorInstanceID == "" {
		observation.CollectorInstanceID = ctx.CollectorInstanceID
	}
	if observation.SourceURI == "" {
		observation.SourceURI = ctx.SourceURI
	}
	if observation.ObservedAt.IsZero() {
		observation.ObservedAt = ctx.ObservedAt
	}
	return observation
}

func withFactContextVersion(observation PackageVersionObservation, ctx FactContext) PackageVersionObservation {
	if observation.ScopeID == "" {
		observation.ScopeID = ctx.ScopeID
	}
	if observation.GenerationID == "" {
		observation.GenerationID = ctx.GenerationID
	}
	if observation.CollectorInstanceID == "" {
		observation.CollectorInstanceID = ctx.CollectorInstanceID
	}
	if observation.SourceURI == "" {
		observation.SourceURI = ctx.SourceURI
	}
	if observation.ObservedAt.IsZero() {
		observation.ObservedAt = ctx.ObservedAt
	}
	return observation
}

func withFactContextDependency(observation PackageDependencyObservation, ctx FactContext) PackageDependencyObservation {
	if observation.ScopeID == "" {
		observation.ScopeID = ctx.ScopeID
	}
	if observation.GenerationID == "" {
		observation.GenerationID = ctx.GenerationID
	}
	if observation.CollectorInstanceID == "" {
		observation.CollectorInstanceID = ctx.CollectorInstanceID
	}
	if observation.SourceURI == "" {
		observation.SourceURI = ctx.SourceURI
	}
	if observation.ObservedAt.IsZero() {
		observation.ObservedAt = ctx.ObservedAt
	}
	return observation
}

func withFactContextArtifact(observation PackageArtifactObservation, ctx FactContext) PackageArtifactObservation {
	if observation.ScopeID == "" {
		observation.ScopeID = ctx.ScopeID
	}
	if observation.GenerationID == "" {
		observation.GenerationID = ctx.GenerationID
	}
	if observation.CollectorInstanceID == "" {
		observation.CollectorInstanceID = ctx.CollectorInstanceID
	}
	if observation.SourceURI == "" {
		observation.SourceURI = ctx.SourceURI
	}
	if observation.ObservedAt.IsZero() {
		observation.ObservedAt = ctx.ObservedAt
	}
	return observation
}

func withFactContextSourceHint(observation SourceHintObservation, ctx FactContext) SourceHintObservation {
	if observation.ScopeID == "" {
		observation.ScopeID = ctx.ScopeID
	}
	if observation.GenerationID == "" {
		observation.GenerationID = ctx.GenerationID
	}
	if observation.CollectorInstanceID == "" {
		observation.CollectorInstanceID = ctx.CollectorInstanceID
	}
	if observation.SourceURI == "" {
		observation.SourceURI = ctx.SourceURI
	}
	if observation.ObservedAt.IsZero() {
		observation.ObservedAt = ctx.ObservedAt
	}
	return observation
}

func withFactContextVulnerability(observation VulnerabilityHintObservation, ctx FactContext) VulnerabilityHintObservation {
	if observation.ScopeID == "" {
		observation.ScopeID = ctx.ScopeID
	}
	if observation.GenerationID == "" {
		observation.GenerationID = ctx.GenerationID
	}
	if observation.CollectorInstanceID == "" {
		observation.CollectorInstanceID = ctx.CollectorInstanceID
	}
	if observation.SourceURI == "" {
		observation.SourceURI = ctx.SourceURI
	}
	if observation.ObservedAt.IsZero() {
		observation.ObservedAt = ctx.ObservedAt
	}
	return observation
}

func withFactContextEvent(observation RegistryEventObservation, ctx FactContext) RegistryEventObservation {
	if observation.ScopeID == "" {
		observation.ScopeID = ctx.ScopeID
	}
	if observation.GenerationID == "" {
		observation.GenerationID = ctx.GenerationID
	}
	if observation.CollectorInstanceID == "" {
		observation.CollectorInstanceID = ctx.CollectorInstanceID
	}
	if observation.SourceURI == "" {
		observation.SourceURI = ctx.SourceURI
	}
	if observation.ObservedAt.IsZero() {
		observation.ObservedAt = ctx.ObservedAt
	}
	return observation
}

func withFactContextHosting(observation RepositoryHostingObservation, ctx FactContext) RepositoryHostingObservation {
	if observation.ScopeID == "" {
		observation.ScopeID = ctx.ScopeID
	}
	if observation.GenerationID == "" {
		observation.GenerationID = ctx.GenerationID
	}
	if observation.CollectorInstanceID == "" {
		observation.CollectorInstanceID = ctx.CollectorInstanceID
	}
	if observation.SourceURI == "" {
		observation.SourceURI = ctx.SourceURI
	}
	if observation.ObservedAt.IsZero() {
		observation.ObservedAt = ctx.ObservedAt
	}
	return observation
}

func withFactContextWarning(observation WarningObservation, ctx FactContext) WarningObservation {
	if observation.ScopeID == "" {
		observation.ScopeID = ctx.ScopeID
	}
	if observation.GenerationID == "" {
		observation.GenerationID = ctx.GenerationID
	}
	if observation.CollectorInstanceID == "" {
		observation.CollectorInstanceID = ctx.CollectorInstanceID
	}
	if observation.SourceURI == "" {
		observation.SourceURI = ctx.SourceURI
	}
	if observation.ObservedAt.IsZero() {
		observation.ObservedAt = ctx.ObservedAt
	}
	return observation
}

// dedupeFacts collapses facts that share a kind and stable key, keeping the
// last occurrence at the position of the first. The host validator rejects a
// conflicting duplicate outright, so collapsing here keeps one artifact with
// two reports from failing the whole claim.
func dedupeFacts(facts []sdk.Fact) []sdk.Fact {
	index := make(map[string]int, len(facts))
	out := make([]sdk.Fact, 0, len(facts))
	for _, fact := range facts {
		key := fact.Kind + "\x00" + fact.StableKey
		if at, ok := index[key]; ok {
			out[at] = fact
			continue
		}
		index[key] = len(out)
		out = append(out, fact)
	}
	return out
}

// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"context"
	"strings"
	"testing"
	"time"

	sdk "github.com/eshu-hq/eshu-sdk/collector"
)

// stubProvider serves canned metadata documents keyed by metadata URL.
type stubProvider struct {
	documents map[string]MetadataDocument
	err       error
}

func (s stubProvider) FetchMetadata(_ context.Context, target FetchTarget) (MetadataDocument, error) {
	if s.err != nil {
		return MetadataDocument{}, s.err
	}
	document, ok := s.documents[strings.TrimSpace(target.MetadataURL)]
	if !ok {
		return MetadataDocument{}, RegistryFailure{Class: RegistryFailureNotFound, Operation: "fetch_metadata", StatusCode: 404}
	}
	return document, nil
}

func testClaim(scopeID string) sdk.Claim {
	return sdk.Claim{
		ComponentID: "dev.eshu.collector.package_registry", InstanceID: "package-registry-qa",
		CollectorKind: CollectorKind, SourceSystem: "package_registry",
		Scope:       sdk.Scope{ID: scopeID, Kind: "package_registry"},
		SourceRunID: "run-1", GenerationID: "generation-1",
		WorkItemID: "work-1", FencingToken: "7", Attempt: 1,
		Deadline: time.Now().UTC().Add(time.Hour), ConfigHandle: "component-config:package-registry",
	}
}

func testConfig(targets ...map[string]any) map[string]any {
	raw := make([]any, 0, len(targets))
	for _, target := range targets {
		raw = append(raw, target)
	}
	return map[string]any{"targets": raw}
}

func npmTarget(scopeID, metadataURL string) map[string]any {
	return map[string]any{
		"provider": "npm", "ecosystem": "npm", "registry": "https://registry.npmjs.org",
		"scope_id": scopeID, "packages": []any{"left-pad"},
		"metadata_url": metadataURL,
	}
}

// TestHandlerCollectsPackumentFacts drives one claim through a stub provider
// and asserts the result is complete, carries package/version/artifact facts,
// and passes the SDK contract validator.
func TestHandlerCollectsPackumentFacts(t *testing.T) {
	t.Parallel()

	const scopeID = "npm://registry.npmjs.org/left-pad"
	const metadataURL = "https://registry.invalid/left-pad"
	provider := stubProvider{documents: map[string]MetadataDocument{
		metadataURL: {
			Body:       []byte(`{"name": "left-pad", "versions": {"1.3.0": {"dist": {"tarball": "https://registry.npmjs.org/left-pad/-/left-pad-1.3.0.tgz"}}}}`),
			SourceURI:  metadataURL,
			ObservedAt: time.Now().UTC(),
		},
	}}
	handler := Handler{
		Provider: provider,
		Now:      func() time.Time { return time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC) },
	}
	claim := testClaim(scopeID)
	request := Request{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
		Claim:           claim,
		Contract:        Contract(),
		Config:          testConfig(npmTarget(scopeID, metadataURL)),
	}
	result := handler.Handle(context.Background(), request)
	if result.State != sdk.ResultComplete {
		t.Fatalf("State=%q, want complete", result.State)
	}
	if len(result.Facts) != 3 {
		t.Fatalf("facts=%d, want 3 (package, version, artifact)", len(result.Facts))
	}
	if result.Claim != claim {
		t.Fatal("result must echo the claim unchanged")
	}
	if result.Generation.ID != claim.GenerationID {
		t.Fatalf("generation.id=%q, want the claim generation", result.Generation.ID)
	}
	for _, fact := range result.Facts {
		if fact.SourceRef.ScopeID != scopeID || fact.SourceRef.GenerationID != claim.GenerationID {
			t.Fatalf("fact %q carries scope %q generation %q", fact.Kind, fact.SourceRef.ScopeID, fact.SourceRef.GenerationID)
		}
		if fact.SourceRef.SourceSystem != claim.SourceSystem {
			t.Fatalf("fact %q source_system=%q, want the claim source system", fact.Kind, fact.SourceRef.SourceSystem)
		}
	}
	if _, err := sdk.NewValidator(Contract()).ValidateResult(result); err != nil {
		t.Fatalf("ValidateResult() error = %v", err)
	}
}

// TestHandlerPrivateTargetWithoutCredentialsCompletesWithWarning pins the
// credentials_missing short-circuit: no fetch, one warning fact, no leak.
func TestHandlerPrivateTargetWithoutCredentialsCompletesWithWarning(t *testing.T) {
	t.Parallel()

	const scopeID = "npm://private.invalid/pkg"
	target := npmTarget(scopeID, "https://private.invalid/pkg")
	target["visibility"] = "private"
	provider := stubProvider{documents: map[string]MetadataDocument{}}
	handler := Handler{Provider: provider}
	claim := testClaim(scopeID)
	request := Request{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
		Claim:           claim,
		Contract:        Contract(),
		Config:          testConfig(target),
	}
	result := handler.Handle(context.Background(), request)
	if result.State != sdk.ResultComplete {
		t.Fatalf("State=%q, want complete", result.State)
	}
	if len(result.Facts) != 1 || result.Facts[0].Kind != FactKindWarning {
		t.Fatalf("facts=%v, want one warning fact", result.Facts)
	}
	if got := result.Facts[0].Payload["warning_code"]; got != warningCodeCredentialsMissing {
		t.Fatalf("warning_code=%v, want %q", got, warningCodeCredentialsMissing)
	}
	if _, err := sdk.NewValidator(Contract()).ValidateResult(result); err != nil {
		t.Fatalf("ValidateResult() error = %v", err)
	}
}

// TestHandlerTooLargeMetadataCompletesWithWarning pins the terminal oversize
// path: the claim completes with warning evidence instead of retrying.
func TestHandlerTooLargeMetadataCompletesWithWarning(t *testing.T) {
	t.Parallel()

	const scopeID = "npm://registry.npmjs.org/left-pad"
	handler := Handler{Provider: stubProvider{err: newMetadataTooLargeError(maxMetadataDocumentBytes)}}
	claim := testClaim(scopeID)
	request := Request{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
		Claim:           claim,
		Contract:        Contract(),
		Config:          testConfig(npmTarget(scopeID, "https://registry.invalid/left-pad")),
	}
	result := handler.Handle(context.Background(), request)
	if result.State != sdk.ResultComplete {
		t.Fatalf("State=%q, want complete", result.State)
	}
	if len(result.Facts) != 1 || result.Facts[0].Payload["warning_code"] != warningCodeMetadataTooLarge {
		t.Fatalf("facts=%v, want one metadata_too_large warning", result.Facts)
	}
}

// TestHandlerTransportFailureIsRetryable pins the retry path: a 500 from the
// registry releases the claim with a bounded retryable failure and no facts.
func TestHandlerTransportFailureIsRetryable(t *testing.T) {
	t.Parallel()

	const scopeID = "npm://registry.npmjs.org/left-pad"
	handler := Handler{Provider: stubProvider{
		err: RegistryFailure{Class: RegistryFailureRetryable, Operation: "fetch_metadata", StatusCode: 500},
	}}
	claim := testClaim(scopeID)
	request := Request{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
		Claim:           claim,
		Contract:        Contract(),
		Config:          testConfig(npmTarget(scopeID, "https://registry.invalid/left-pad")),
	}
	result := handler.Handle(context.Background(), request)
	if result.State != sdk.ResultRetryable {
		t.Fatalf("State=%q, want retryable", result.State)
	}
	if len(result.Facts) != 0 {
		t.Fatalf("facts=%d, want 0 with a failure state", len(result.Facts))
	}
	if len(result.Statuses) != 1 || result.Statuses[0].Class != sdk.StatusFailure {
		t.Fatalf("statuses=%v, want one failure status", result.Statuses)
	}
	if result.Statuses[0].RetryAfterSeconds == 0 {
		t.Fatal("retryable status requires retry_after_seconds")
	}
}

// TestHandlerAuthDenialIsTerminal pins the terminal path: a 403 fails the
// claim without a retry.
func TestHandlerAuthDenialIsTerminal(t *testing.T) {
	t.Parallel()

	const scopeID = "npm://registry.npmjs.org/left-pad"
	handler := Handler{Provider: stubProvider{
		err: RegistryFailure{Class: RegistryFailureAuthDenied, Operation: "fetch_metadata", StatusCode: 403},
	}}
	claim := testClaim(scopeID)
	request := Request{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
		Claim:           claim,
		Contract:        Contract(),
		Config:          testConfig(npmTarget(scopeID, "https://registry.invalid/left-pad")),
	}
	result := handler.Handle(context.Background(), request)
	if result.State != sdk.ResultTerminal {
		t.Fatalf("State=%q, want terminal", result.State)
	}
}

// TestHandlerDerivedNotFoundIsPartial pins the derived-target missing path: a
// 404 on a derived scope completes partial with registry_not_found warning
// evidence.
func TestHandlerDerivedNotFoundIsPartial(t *testing.T) {
	t.Parallel()

	handler := Handler{
		Provider: stubProvider{documents: map[string]MetadataDocument{}},
	}
	claim := testClaim("npm://registry.npmjs.org/left-pad")
	request := Request{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
		Claim:           claim,
		Contract:        Contract(),
		Config: map[string]any{
			"derive_from_owned_packages": map[string]any{
				"enabled": true, "ecosystems": []any{"npm"},
			},
		},
	}
	result := handler.Handle(context.Background(), request)
	if result.State != sdk.ResultPartial {
		t.Fatalf("State=%q, want partial", result.State)
	}
	if len(result.Facts) != 1 || result.Facts[0].Payload["warning_code"] != warningCodeMetadataNotFound {
		t.Fatalf("facts=%v, want one registry_not_found warning", result.Facts)
	}
	if _, err := sdk.NewValidator(Contract()).ValidateResult(result); err != nil {
		t.Fatalf("ValidateResult() error = %v", err)
	}
}

// TestHandlerUnknownScopeIsInvalidConfig pins the fail-closed scope lookup: a
// claim for an unconfigured scope with no derived targets is terminal
// invalid_config.
func TestHandlerUnknownScopeIsInvalidConfig(t *testing.T) {
	t.Parallel()

	handler := Handler{Provider: stubProvider{documents: map[string]MetadataDocument{}}}
	claim := testClaim("npm://registry.npmjs.org/unknown")
	request := Request{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
		Claim:           claim,
		Contract:        Contract(),
		Config:          testConfig(npmTarget("npm://registry.npmjs.org/left-pad", "https://registry.invalid/left-pad")),
	}
	result := handler.Handle(context.Background(), request)
	if result.State != sdk.ResultTerminal {
		t.Fatalf("State=%q, want terminal", result.State)
	}
	if len(result.Statuses) != 1 || result.Statuses[0].FailureClass != FailureInvalidConfig {
		t.Fatalf("statuses=%v, want one invalid_config status", result.Statuses)
	}
}

// TestHandlerDedupesConflictingStableKeys pins dedupeFacts on the claim path:
// a document that parses the same version twice still validates.
func TestHandlerDedupesEmittedFacts(t *testing.T) {
	t.Parallel()

	const scopeID = "npm://registry.npmjs.org/left-pad"
	const metadataURL = "https://registry.invalid/left-pad"
	provider := stubProvider{documents: map[string]MetadataDocument{
		metadataURL: {
			Body:       []byte(`{"name": "left-pad", "versions": {"1.0.0": {}}}`),
			ObservedAt: time.Now().UTC(),
		},
	}}
	handler := Handler{Provider: provider}
	claim := testClaim(scopeID)
	request := Request{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
		Claim:           claim,
		Contract:        Contract(),
		Config:          testConfig(npmTarget(scopeID, metadataURL)),
	}
	result := handler.Handle(context.Background(), request)
	if result.State != sdk.ResultComplete {
		t.Fatalf("State=%q, want complete", result.State)
	}
	seen := map[string]int{}
	for _, fact := range result.Facts {
		seen[fact.Kind+"\x00"+fact.StableKey]++
	}
	for key, count := range seen {
		if count > 1 {
			t.Fatalf("stable key %q emitted %d times", key, count)
		}
	}
}

// TestClampedDeadlineTimeoutPinsMargin pins the lease-deadline margin math.
func TestClampedDeadlineTimeoutPinsMargin(t *testing.T) {
	t.Parallel()

	now := time.Now()
	if _, ok := clampedDeadlineTimeout(now, time.Time{}); ok {
		t.Fatal("zero deadline must report false")
	}
	if _, ok := clampedDeadlineTimeout(now, now.Add(-time.Second)); ok {
		t.Fatal("spent deadline must report false")
	}
	got, ok := clampedDeadlineTimeout(now, now.Add(100*time.Second))
	if !ok {
		t.Fatal("future deadline must report true")
	}
	// 5% of 100s is 5s, capped at the 2s margin: 98s of work time.
	if got != 98*time.Second {
		t.Fatalf("timeout=%v, want 98s", got)
	}
}

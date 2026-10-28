// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	sdk "github.com/eshu-hq/eshu-sdk/collector"
)

// failWriter drops every diagnostic write. It proves a failed log write has
// no recovery action and never perturbs the claim result.
type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("boom") }

// malformedPackumentRequest serves a syntactically broken npm document for
// the configured package.
func malformedPackumentRequest(t *testing.T, scopeID, metadataURL string, mutateClaim func(*sdk.Claim)) Request {
	t.Helper()
	claim := testClaim(scopeID)
	if mutateClaim != nil {
		mutateClaim(&claim)
	}
	return Request{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
		Claim:           claim,
		Contract:        Contract(),
		Config: testConfig(map[string]any{
			"provider": "npm", "ecosystem": "npm", "registry": "https://registry.npmjs.org",
			"scope_id": scopeID, "packages": []any{"left-pad"},
			"metadata_url": metadataURL,
		}),
	}
}

func stubMalformedProvider(metadataURL string) stubProvider {
	return stubProvider{documents: map[string]MetadataDocument{
		metadataURL: {Body: []byte(`{"name": "left-pad", "versions": {oops}}`), ObservedAt: time.Now().UTC()},
	}}
}

// TestRatchetHandlerMalformedDocumentIsPartial pins the parse-failure lane: a
// syntactically broken document completes partial with malformed_metadata
// warning evidence instead of failing the claim.
func TestRatchetHandlerMalformedDocumentIsPartial(t *testing.T) {
	t.Parallel()

	const scopeID = "npm://registry.npmjs.org/left-pad"
	const metadataURL = "https://registry.invalid/left-pad"
	request := malformedPackumentRequest(t, scopeID, metadataURL, nil)
	result := Handler{Provider: stubMalformedProvider(metadataURL)}.Handle(context.Background(), request)
	if result.State != sdk.ResultPartial {
		t.Fatalf("State=%q, want partial", result.State)
	}
	if len(result.Facts) != 1 || result.Facts[0].Payload["warning_code"] != warningCodeMalformedMetadata {
		t.Fatalf("facts=%v, want one malformed_metadata warning", result.Facts)
	}
	if _, err := sdk.NewValidator(Contract()).ValidateResult(result); err != nil {
		t.Fatalf("ValidateResult() error = %v", err)
	}
}

// TestRatchetHandlerMalformedDocumentErrorIsTerminal pins the Join branch: a
// broken document degrades to terminal invalid_evidence when the warning
// fact itself cannot be built.
func TestRatchetHandlerMalformedDocumentErrorIsTerminal(t *testing.T) {
	t.Parallel()

	const scopeID = "npm://registry.npmjs.org/left-pad"
	const metadataURL = "https://registry.invalid/left-pad"
	request := malformedPackumentRequest(t, scopeID, metadataURL, func(claim *sdk.Claim) {
		claim.GenerationID = ""
	})
	result := Handler{Provider: stubMalformedProvider(metadataURL)}.Handle(context.Background(), request)
	if result.State != sdk.ResultTerminal {
		t.Fatalf("State=%q, want terminal", result.State)
	}
	if len(result.Facts) != 0 {
		t.Fatalf("facts=%d, want 0 when the warning cannot be identified", len(result.Facts))
	}
	if len(result.Statuses) != 1 || result.Statuses[0].FailureClass != FailureInvalidEvidence {
		t.Fatalf("statuses=%v, want one invalid_evidence status", result.Statuses)
	}
}

// TestRatchetHandlerUnvalidatableClaimIsTerminal pins result validation on
// both result paths: facts collect fine under the defaulted source system,
// but the echoed claim still carries the blank source system, so the SDK
// validator rejects the complete result and the derived partial result.
// Both end terminal with invalid_evidence and keep no facts on the failure.
func TestRatchetHandlerUnvalidatableClaimIsTerminal(t *testing.T) {
	t.Parallel()

	const scopeID = "npm://registry.npmjs.org/left-pad"
	const metadataURL = "https://registry.invalid/left-pad"
	provider := stubProvider{documents: map[string]MetadataDocument{
		metadataURL: {Body: []byte(`{"name": "left-pad", "versions": {"1.3.0": {}}}`), ObservedAt: time.Now().UTC()},
	}}
	claim := testClaim(scopeID)
	claim.SourceSystem = ""
	complete := Request{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
		Claim:           claim,
		Contract:        Contract(),
		Config:          testConfig(npmTarget(scopeID, metadataURL)),
	}
	result := Handler{Provider: provider}.Handle(context.Background(), complete)
	if result.State != sdk.ResultTerminal {
		t.Fatalf("complete State=%q, want terminal", result.State)
	}
	if len(result.Facts) != 0 {
		t.Fatalf("complete facts=%d, want 0 on the failure result", len(result.Facts))
	}
	if len(result.Statuses) != 1 || result.Statuses[0].FailureClass != FailureInvalidEvidence {
		t.Fatalf("complete statuses=%v, want one invalid_evidence status", result.Statuses)
	}

	partial := Request{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
		Claim:           claim,
		Contract:        Contract(),
		Config: map[string]any{
			"derive_from_owned_packages": map[string]any{
				"enabled": true, "ecosystems": []any{"npm"},
			},
		},
	}
	partialResult := Handler{Provider: stubProvider{documents: map[string]MetadataDocument{}}}.Handle(context.Background(), partial)
	if partialResult.State != sdk.ResultTerminal {
		t.Fatalf("partial State=%q, want terminal", partialResult.State)
	}
	if len(partialResult.Statuses) != 1 || partialResult.Statuses[0].FailureClass != FailureInvalidEvidence {
		t.Fatalf("partial statuses=%v, want one invalid_evidence status", partialResult.Statuses)
	}
}

// TestRatchetHandlerDiagnosticLogWritesLine pins the operator log: with a
// writer configured, a failed claim emits one JSON diagnostic line carrying
// the failure class, and a failed write still returns the claim result.
func TestRatchetHandlerDiagnosticLogWritesLine(t *testing.T) {
	t.Parallel()

	const scopeID = "npm://registry.npmjs.org/left-pad"
	request := ratchetPackumentRequest(t, scopeID, "https://registry.invalid/left-pad", nil)
	var logged bytes.Buffer
	result := Handler{
		Provider: stubProvider{err: RegistryFailure{Class: RegistryFailureRetryable, Operation: "fetch_metadata", StatusCode: 500}},
		Log:      &logged,
	}.Handle(context.Background(), request)
	if result.State != sdk.ResultRetryable {
		t.Fatalf("State=%q, want retryable", result.State)
	}
	line := strings.TrimSpace(logged.String())
	if !strings.Contains(line, `"class":"registry_retryable_failure"`) {
		t.Fatalf("log line = %q, want the failure class", line)
	}
	if !strings.Contains(line, `"scope_id":"`+scopeID+`"`) {
		t.Fatalf("log line = %q, want the claim scope", line)
	}

	failing := Handler{
		Provider: stubProvider{err: RegistryFailure{Class: RegistryFailureRetryable, Operation: "fetch_metadata", StatusCode: 500}},
		Log:      failWriter{},
	}.Handle(context.Background(), request)
	if failing.State != sdk.ResultRetryable {
		t.Fatalf("failing-log State=%q, want the claim result regardless", failing.State)
	}
}

// TestRatchetHandlerGetenvOverride pins the credential hook: an injected
// getenv resolves names instead of the process environment.
func TestRatchetHandlerGetenvOverride(t *testing.T) {
	t.Parallel()

	handler := Handler{Getenv: func(key string) string { return "resolved:" + key }}
	if got := handler.getenv("TOKEN_ENV"); got != "resolved:TOKEN_ENV" {
		t.Fatalf("getenv = %q, want the override", got)
	}
}

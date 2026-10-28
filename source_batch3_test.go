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

// TestHandlerGetenvReadsConfiguredValue pins the getenv indirection: a
// configured lookup wins, a nil lookup resolves to the zero value without
// touching the process environment.
func TestHandlerGetenvReadsConfiguredValue(t *testing.T) {
	t.Parallel()

	configured := Handler{Getenv: func(key string) string {
		if key == "TOKEN_ENV" {
			return "secret"
		}
		return ""
	}}
	if got := configured.getenv("TOKEN_ENV"); got != "secret" {
		t.Fatalf("getenv(TOKEN_ENV) = %q, want the configured value", got)
	}
	if got := (Handler{}).getenv("TOKEN_ENV"); got != "" {
		t.Fatalf("getenv() without a lookup = %q, want zero value", got)
	}
}

// TestHandlerProviderSelectsConfiguredOrDefault pins the provider switch: an
// injected provider is used as-is, the zero value selects the live HTTP
// provider.
func TestHandlerProviderSelectsConfiguredOrDefault(t *testing.T) {
	t.Parallel()

	stub := &stubProvider{}
	configured := Handler{Provider: stub}
	if configured.provider() != stub {
		t.Fatal("provider() must return the injected provider")
	}
	if _, ok := (Handler{}).provider().(HTTPMetadataProvider); !ok {
		t.Fatalf("provider() = %T, want the default HTTPMetadataProvider", (Handler{}).provider())
	}
}

// TestClaimSourceSystemTrimsOrDefaults pins the source-system resolution: a
// blank claim falls back to the component default, a named claim is trimmed
// and kept verbatim.
func TestClaimSourceSystemTrimsOrDefaults(t *testing.T) {
	t.Parallel()

	blank := testClaim("npm://registry.npmjs.org/left-pad")
	blank.SourceSystem = "  "
	if got := claimSourceSystem(blank); got != SourceSystem {
		t.Fatalf("claimSourceSystem(blank) = %q, want the component default %q", got, SourceSystem)
	}
	named := testClaim("npm://registry.npmjs.org/left-pad")
	named.SourceSystem = "  upstream-mirror  "
	if got := claimSourceSystem(named); got != "upstream-mirror" {
		t.Fatalf("claimSourceSystem(named) = %q, want the trimmed claim value", got)
	}
}

// TestClaimContextBoundsClaimWork pins the lease/timeout bound: no deadline
// keeps the configured timeout, a near deadline narrows it by the margin,
// and a spent lease refuses to start.
func TestClaimContextBoundsClaimWork(t *testing.T) {
	t.Parallel()

	started := time.Now().UTC()
	limits := ResourceUse{ClaimTimeoutSeconds: 300}

	unbounded, cancel, ok := claimContext(context.Background(), sdk.Claim{}, started, limits)
	cancel()
	if !ok {
		t.Fatal("claimContext() without a deadline must start")
	}
	deadline, hasDeadline := unbounded.Deadline()
	if !hasDeadline {
		t.Fatal("claimContext() must bound the run even without a claim deadline")
	}
	if want := started.Add(300 * time.Second); deadline.Sub(want).Abs() > time.Second {
		t.Fatalf("claimContext() deadline = %v, want about %v", deadline, want)
	}

	near := testClaim("npm://registry.npmjs.org/left-pad")
	near.Deadline = started.Add(10 * time.Second)
	narrowed, cancel, ok := claimContext(context.Background(), near, started, limits)
	cancel()
	if !ok {
		t.Fatal("claimContext() with a future deadline must start")
	}
	narrowDeadline, _ := narrowed.Deadline()
	// 5% of 10s is 0.5s, below the 2s cap: 9.5s of work time.
	if want := started.Add(9500 * time.Millisecond); narrowDeadline.Sub(want).Abs() > time.Second {
		t.Fatalf("claimContext() deadline = %v, want about %v", narrowDeadline, want)
	}

	// A spent lease carries no usable bound, so the run keeps the
	// configured claim timeout instead of refusing to start.
	spent := testClaim("npm://registry.npmjs.org/left-pad")
	spent.Deadline = started.Add(-time.Second)
	spentCtx, cancel, ok := claimContext(context.Background(), spent, started, limits)
	defer cancel()
	if !ok {
		t.Fatal("claimContext() with a spent lease must fall back to the claim timeout")
	}
	spentDeadline, _ := spentCtx.Deadline()
	if want := started.Add(300 * time.Second); spentDeadline.Sub(want).Abs() > time.Second {
		t.Fatalf("claimContext() deadline = %v, want about %v", spentDeadline, want)
	}

	zeroLimits := testClaim("npm://registry.npmjs.org/left-pad")
	if _, cancel, ok := claimContext(context.Background(), zeroLimits, started, ResourceUse{}); !ok {
		cancel()
		t.Fatal("claimContext() with zero limits must fall back to defaults and start")
	} else {
		cancel()
	}
}

// TestStoppedAtClaimDeadlineNamesLeaseExpiry pins the cause check: only a run
// context ended by the lease bound reports true.
func TestStoppedAtClaimDeadlineNamesLeaseExpiry(t *testing.T) {
	t.Parallel()

	bounded, cancel := context.WithCancelCause(context.Background())
	cancel(errClaimDeadline)
	defer cancel(nil)
	if !stoppedAtClaimDeadline(bounded) {
		t.Fatal("stoppedAtClaimDeadline() must report the lease-bound cause")
	}
	if stoppedAtClaimDeadline(context.Background()) {
		t.Fatal("stoppedAtClaimDeadline() must be false without a cause")
	}
	canceled, stop := context.WithCancelCause(context.Background())
	stop(context.Canceled)
	if stoppedAtClaimDeadline(canceled) {
		t.Fatal("stoppedAtClaimDeadline() must be false for a host cancel")
	}
}

// TestParseMetadataRoutesDocumentFormats pins the format switch: empty and
// native route to the ecosystem parser, the Artifactory wrapper unwraps to
// the same observations, and anything else fails closed.
func TestParseMetadataRoutesDocumentFormats(t *testing.T) {
	t.Parallel()

	handler := Handler{}
	ctx := MetadataParserContext{
		Ecosystem: EcosystemNPM, Registry: "https://registry.npmjs.org",
		ScopeID: "npm://registry.npmjs.org/left-pad", GenerationID: "generation-1",
		CollectorInstanceID: "package-registry-qa",
	}
	body := []byte(`{"name": "left-pad", "versions": {"1.3.0": {}}}`)

	for _, format := range []string{"", DocumentFormatNative} {
		parsed, err := handler.parseMetadata(ctx, FetchTarget{DocumentFormat: format}, body)
		if err != nil {
			t.Fatalf("parseMetadata(%q) error = %v", format, err)
		}
		if len(parsed.Packages) != 1 {
			t.Fatalf("parseMetadata(%q) packages = %d, want 1", format, len(parsed.Packages))
		}
	}

	wrapper := []byte(`{"package_type": "npm", "metadata": {"name": "left-pad", "versions": {"1.3.0": {}}}}`)
	parsed, err := handler.parseMetadata(ctx, FetchTarget{DocumentFormat: DocumentFormatArtifactoryPackage}, wrapper)
	if err != nil {
		t.Fatalf("parseMetadata(artifactory_package) error = %v", err)
	}
	if len(parsed.Packages) != 1 {
		t.Fatalf("parseMetadata(artifactory_package) packages = %d, want 1", len(parsed.Packages))
	}

	if _, err := handler.parseMetadata(ctx, FetchTarget{DocumentFormat: "tarball"}, body); err == nil {
		t.Fatal("parseMetadata() must reject an unsupported document_format")
	} else if !strings.Contains(err.Error(), "tarball") {
		t.Fatalf("parseMetadata() error = %v, want the format named", err)
	}
	if _, err := handler.parseMetadata(ctx, FetchTarget{}, []byte("{not json")); err == nil {
		t.Fatal("parseMetadata() must surface a malformed native document")
	}
}

// TestCollectDocumentMalformedBodyIsPartial pins the parse-failure warning
// path through collectDocument: malformed metadata completes partial with
// malformed_metadata evidence instead of failing the claim.
func TestCollectDocumentMalformedBodyIsPartial(t *testing.T) {
	t.Parallel()

	const scopeID = "npm://registry.npmjs.org/left-pad"
	handler := Handler{Provider: stubProvider{documents: map[string]MetadataDocument{
		"https://registry.invalid/left-pad": {Body: []byte(`{"versions": {}}`)},
	}}}
	claim := testClaim(scopeID)
	result := handler.Handle(context.Background(), Request{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
		Claim:           claim,
		Contract:        Contract(),
		Config:          testConfig(npmTarget(scopeID, "https://registry.invalid/left-pad")),
	})
	if result.State != sdk.ResultPartial {
		t.Fatalf("State=%q, want partial for malformed metadata", result.State)
	}
	if len(result.Facts) != 1 || result.Facts[0].Payload["warning_code"] != warningCodeMalformedMetadata {
		t.Fatalf("facts=%v, want one malformed_metadata warning", result.Facts)
	}
}

// TestParseMetadataUnregisteredEcosystemFails pins the fail-closed route: an
// ecosystem without a registered parser cannot produce observations.
func TestParseMetadataUnregisteredEcosystemFails(t *testing.T) {
	t.Parallel()

	ctx := MetadataParserContext{
		Ecosystem: EcosystemSwift, Registry: "https://example.invalid",
		ScopeID: "swift://example.invalid/pkg", GenerationID: "generation-1",
		CollectorInstanceID: "package-registry-qa",
	}
	if _, err := (Handler{}).parseMetadata(ctx, FetchTarget{}, []byte(`{}`)); err == nil {
		t.Fatal("parseMetadata() must fail for an unregistered ecosystem")
	} else if !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("parseMetadata() error = %v, want the not-registered cause", err)
	}
}

// TestCollectDocumentConfiguredMismatchIsTerminal pins the bound-metadata
// failure path: a document that matches none of the configured packages is
// terminal invalid evidence with no facts.
func TestCollectDocumentConfiguredMismatchIsTerminal(t *testing.T) {
	t.Parallel()

	const scopeID = "npm://registry.npmjs.org/left-pad"
	target := npmTarget(scopeID, "https://registry.invalid/left-pad")
	target["packages"] = []any{"other-package"}
	handler := Handler{Provider: stubProvider{documents: map[string]MetadataDocument{
		"https://registry.invalid/left-pad": {
			Body: []byte(`{"name": "left-pad", "versions": {"1.3.0": {}}}`),
		},
	}}}
	claim := testClaim(scopeID)
	result := handler.Handle(context.Background(), Request{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
		Claim:           claim,
		Contract:        Contract(),
		Config:          testConfig(target),
	})
	if result.State != sdk.ResultTerminal {
		t.Fatalf("State=%q, want terminal for a configured-package mismatch", result.State)
	}
	if len(result.Facts) != 0 {
		t.Fatalf("facts=%d, want 0 with a failure state", len(result.Facts))
	}
	if len(result.Statuses) != 1 || result.Statuses[0].FailureClass != FailureInvalidEvidence {
		t.Fatalf("statuses=%v, want one invalid_evidence status", result.Statuses)
	}
}

// TestClaimContextLeaseBoundEndsTheRun pins the end-to-end lease bound: the
// run context from claimContext ends with the lease cause, so a fetch that
// outlives the lease reports it via stoppedAtClaimDeadline.
func TestClaimContextLeaseBoundEndsTheRun(t *testing.T) {
	t.Parallel()

	started := time.Now().UTC()
	claim := testClaim("npm://registry.npmjs.org/left-pad")
	claim.Deadline = started.Add(60 * time.Millisecond)
	runCtx, cancel, ok := claimContext(context.Background(), claim, started, ResourceUse{ClaimTimeoutSeconds: 300})
	defer cancel()
	if !ok {
		t.Fatal("claimContext() with a future deadline must start")
	}
	<-runCtx.Done()
	if !stoppedAtClaimDeadline(runCtx) {
		t.Fatalf("stoppedAtClaimDeadline() = false, cause = %v, want the lease bound", context.Cause(runCtx))
	}
}

// validCompleteFacts builds two contract-valid facts for the completeResult
// budget tests.
func validCompleteFacts(t *testing.T) []sdk.Fact {
	t.Helper()

	first, err := NewPackageEnvelope(PackageObservation{
		Identity:            PackageIdentity{Ecosystem: EcosystemNPM, Registry: "https://registry.npmjs.org", RawName: "left-pad"},
		ScopeID:             "npm://registry.npmjs.org/left-pad",
		GenerationID:        "generation-1",
		CollectorInstanceID: "package-registry-qa",
	}, SourceSystem)
	if err != nil {
		t.Fatalf("NewPackageEnvelope() error = %v", err)
	}
	second, err := NewPackageVersionEnvelope(PackageVersionObservation{
		Package:             PackageIdentity{Ecosystem: EcosystemNPM, Registry: "https://registry.npmjs.org", RawName: "left-pad"},
		Version:             "1.3.0",
		ScopeID:             "npm://registry.npmjs.org/left-pad",
		GenerationID:        "generation-1",
		CollectorInstanceID: "package-registry-qa",
	}, SourceSystem)
	if err != nil {
		t.Fatalf("NewPackageVersionEnvelope() error = %v", err)
	}
	return []sdk.Fact{first, second}
}

// TestCompleteResultEnforcesBudgets pins the three terminal budget guards:
// the record count, the per-fact payload size, and the encoded result size.
func TestCompleteResultEnforcesBudgets(t *testing.T) {
	t.Parallel()

	claim := testClaim("npm://registry.npmjs.org/left-pad")
	started := time.Now().UTC()
	settings := Settings{CollectorInstanceID: "package-registry-qa", Limits: DefaultResourceUse()}

	if got := (Handler{}).completeResult(claim, settings, validCompleteFacts(t), 0, started); got.State != sdk.ResultComplete {
		t.Fatalf("completeResult() State=%q, want complete within budgets", got.State)
	}

	tightRecords := settings
	tightRecords.Limits.MaxRecordsPerClaim = 1
	if got := (Handler{}).completeResult(claim, tightRecords, validCompleteFacts(t), 0, started); got.State != sdk.ResultTerminal {
		t.Fatalf("completeResult() State=%q, want terminal past the record budget", got.State)
	} else if got.Statuses[0].FailureClass != FailureRecordBudget {
		t.Fatalf("statuses=%v, want one record_budget_exceeded status", got.Statuses)
	}

	tightPayload := settings
	tightPayload.Limits.MaxPayloadBytes = 16
	if got := (Handler{}).completeResult(claim, tightPayload, validCompleteFacts(t), 0, started); got.State != sdk.ResultTerminal {
		t.Fatalf("completeResult() State=%q, want terminal past the payload budget", got.State)
	} else if got.Statuses[0].FailureClass != FailurePayloadBudget {
		t.Fatalf("statuses=%v, want one payload_budget_exceeded status", got.Statuses)
	}

	tightResult := settings
	tightResult.Limits.MaxResultBytes = 16
	if got := (Handler{}).completeResult(claim, tightResult, validCompleteFacts(t), 0, started); got.State != sdk.ResultTerminal {
		t.Fatalf("completeResult() State=%q, want terminal past the result budget", got.State)
	} else if got.Statuses[0].FailureClass != FailureResultBudget {
		t.Fatalf("statuses=%v, want one result_budget_exceeded status", got.Statuses)
	}

	// A result that fits every budget but fails contract validation is
	// terminal invalid evidence with no facts leaking.
	junk := validCompleteFacts(t)[:1]
	junk[0].Kind = ""
	if got := (Handler{}).completeResult(claim, settings, junk, 0, started); got.State != sdk.ResultTerminal {
		t.Fatalf("completeResult() State=%q, want terminal for an invalid result", got.State)
	} else if got.Statuses[0].FailureClass != FailureInvalidEvidence {
		t.Fatalf("statuses=%v, want one invalid_evidence status", got.Statuses)
	}
}

// blockingProvider waits for the run context to end, then reports the
// context error as a fetch failure.
type blockingProvider struct{}

func (blockingProvider) FetchMetadata(ctx context.Context, _ FetchTarget) (MetadataDocument, error) {
	<-ctx.Done()
	return MetadataDocument{}, ctx.Err()
}

// TestHandlerFetchHittingLeaseDeadlineIsRetryable pins the in-fetch expiry
// path: when the lease bound ends the fetch, the claim releases retryable
// with no facts instead of surfacing the context error.
func TestHandlerFetchHittingLeaseDeadlineIsRetryable(t *testing.T) {
	t.Parallel()

	const scopeID = "npm://registry.npmjs.org/left-pad"
	handler := Handler{Provider: blockingProvider{}}
	claim := testClaim(scopeID)
	claim.Deadline = time.Now().UTC().Add(80 * time.Millisecond)
	result := handler.Handle(context.Background(), Request{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
		Claim:           claim,
		Contract:        Contract(),
		Config:          testConfig(npmTarget(scopeID, "https://registry.invalid/left-pad")),
	})
	if result.State != sdk.ResultRetryable {
		t.Fatalf("State=%q, want retryable when the lease ends the fetch", result.State)
	}
	if len(result.Facts) != 0 {
		t.Fatalf("facts=%d, want 0 with a failure state", len(result.Facts))
	}
	if len(result.Statuses) != 1 || result.Statuses[0].FailureClass != FailureClaimDeadline {
		t.Fatalf("statuses=%v, want one claim_deadline_exceeded status", result.Statuses)
	}
}

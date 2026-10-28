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

// deadlineProvider blocks until the run context ends, then reports the
// context error. It proves the lease-deadline read path without sleeping.
type deadlineProvider struct{}

func (deadlineProvider) FetchMetadata(ctx context.Context, _ FetchTarget) (MetadataDocument, error) {
	<-ctx.Done()
	return MetadataDocument{}, ctx.Err()
}

// ratchetPackumentRequest builds a claim request for a stub-served npm
// packument, with mutate adjusting the target map before encoding.
func ratchetPackumentRequest(t *testing.T, scopeID, metadataURL string, mutate func(map[string]any)) Request {
	t.Helper()
	target := npmTarget(scopeID, metadataURL)
	if mutate != nil {
		mutate(target)
	}
	return Request{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
		Claim:           testClaim(scopeID),
		Contract:        Contract(),
		Config:          testConfig(target),
	}
}

// TestRatchetHandlerRejectsInvalidConfig pins the fail-closed config path: an
// empty activation config ends terminal with invalid_config and no facts.
func TestRatchetHandlerRejectsInvalidConfig(t *testing.T) {
	t.Parallel()

	claim := testClaim("npm://registry.npmjs.org/left-pad")
	request := Request{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
		Claim:           claim,
		Contract:        Contract(),
		Config:          map[string]any{},
	}
	result := Handler{}.Handle(context.Background(), request)
	if result.State != sdk.ResultTerminal {
		t.Fatalf("State=%q, want terminal", result.State)
	}
	if len(result.Facts) != 0 {
		t.Fatalf("facts=%d, want 0 with a failure state", len(result.Facts))
	}
	if len(result.Statuses) != 1 || result.Statuses[0].FailureClass != FailureInvalidConfig {
		t.Fatalf("statuses=%v, want one invalid_config status", result.Statuses)
	}
}

// TestRatchetHandlerFetchStoppedAtDeadline pins the lease-deadline read path:
// the provider blocks, the claim lease expires first, and the claim ends
// retryable with the claim-deadline class and no facts.
func TestRatchetHandlerFetchStoppedAtDeadline(t *testing.T) {
	t.Parallel()

	const scopeID = "npm://registry.npmjs.org/left-pad"
	request := ratchetPackumentRequest(t, scopeID, "https://registry.invalid/left-pad", nil)
	request.Claim.Deadline = time.Now().UTC().Add(300 * time.Millisecond)
	result := Handler{Provider: deadlineProvider{}}.Handle(context.Background(), request)
	if result.State != sdk.ResultRetryable {
		t.Fatalf("State=%q, want retryable", result.State)
	}
	if len(result.Facts) != 0 {
		t.Fatalf("facts=%d, want 0 when the lease expires mid-fetch", len(result.Facts))
	}
	if len(result.Statuses) != 1 || result.Statuses[0].FailureClass != FailureClaimDeadline {
		t.Fatalf("statuses=%v, want one claim-deadline status", result.Statuses)
	}
}

// TestRatchetHandlerPrefetchErrorIsTerminal pins the fail-closed warning
// path: when the warning fact itself cannot be built (here a blank claim
// generation), the claim ends terminal with no facts instead of emitting a
// half-identified warning.
func TestRatchetHandlerPrefetchErrorIsTerminal(t *testing.T) {
	t.Parallel()

	const scopeID = "npm://private.invalid/pkg"
	target := npmTarget(scopeID, "https://private.invalid/pkg")
	target["visibility"] = "private"
	claim := testClaim(scopeID)
	claim.GenerationID = ""
	request := Request{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
		Claim:           claim,
		Contract:        Contract(),
		Config:          testConfig(target),
	}
	result := Handler{Provider: stubProvider{documents: map[string]MetadataDocument{}}}.Handle(context.Background(), request)
	if result.State != sdk.ResultTerminal {
		t.Fatalf("State=%q, want terminal", result.State)
	}
	if len(result.Facts) != 0 {
		t.Fatalf("facts=%d, want 0 when the warning cannot be identified", len(result.Facts))
	}
}

// TestRatchetHandlerWarningErrorsDegradeToFailures pins the Join branches: an
// oversize document and a derived missing document each degrade to a failure
// (never a warning) when the warning fact cannot be built.
func TestRatchetHandlerWarningErrorsDegradeToFailures(t *testing.T) {
	t.Parallel()

	const scopeID = "npm://registry.npmjs.org/left-pad"
	oversize := ratchetPackumentRequest(t, scopeID, "https://registry.invalid/left-pad", nil)
	oversize.Claim.GenerationID = ""
	result := Handler{Provider: stubProvider{err: newMetadataTooLargeError(maxMetadataDocumentBytes)}}.Handle(context.Background(), oversize)
	if result.State != sdk.ResultTerminal {
		t.Fatalf("oversize State=%q, want terminal", result.State)
	}
	if len(result.Facts) != 0 {
		t.Fatalf("oversize facts=%d, want 0", len(result.Facts))
	}
	if len(result.Statuses) != 1 || result.Statuses[0].FailureClass != RegistryFailureTerminal {
		t.Fatalf("oversize statuses=%v, want one terminal registry failure", result.Statuses)
	}

	derived := testClaim(scopeID)
	derived.GenerationID = ""
	derivedRequest := Request{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
		Claim:           derived,
		Contract:        Contract(),
		Config: map[string]any{
			"derive_from_owned_packages": map[string]any{
				"enabled": true, "ecosystems": []any{"npm"},
			},
		},
	}
	derivedResult := Handler{Provider: stubProvider{documents: map[string]MetadataDocument{}}}.Handle(context.Background(), derivedRequest)
	if derivedResult.State != sdk.ResultRetryable {
		t.Fatalf("derived State=%q, want retryable", derivedResult.State)
	}
	if len(derivedResult.Facts) != 0 {
		t.Fatalf("derived facts=%d, want 0", len(derivedResult.Facts))
	}
	if len(derivedResult.Statuses) != 1 || derivedResult.Statuses[0].FailureClass != RegistryFailureNotFound {
		t.Fatalf("derived statuses=%v, want one not-found status", derivedResult.Statuses)
	}
}

// TestRatchetHandlerZeroObservedAtUsesNow pins the observation-time fallback:
// a document without its own timestamp collects against the handler clock.
func TestRatchetHandlerZeroObservedAtUsesNow(t *testing.T) {
	t.Parallel()

	const scopeID = "npm://registry.npmjs.org/left-pad"
	const metadataURL = "https://registry.invalid/left-pad"
	provider := stubProvider{documents: map[string]MetadataDocument{
		metadataURL: {Body: []byte(`{"name": "left-pad", "versions": {"1.3.0": {}}}`)},
	}}
	fixed := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	handler := Handler{Provider: provider, Now: func() time.Time { return fixed }}
	request := ratchetPackumentRequest(t, scopeID, metadataURL, nil)
	result := handler.Handle(context.Background(), request)
	if result.State != sdk.ResultComplete {
		t.Fatalf("State=%q, want complete", result.State)
	}
	if result.Generation.ObservedAt.IsZero() {
		t.Fatal("generation observed_at must fall back to the handler clock")
	}
}

// TestRatchetParseMetadataRejectsUnknownFormat pins the format switch: only
// native (or blank) and the artifactory wrapper route; anything else is an
// unsupported-format error naming the value.
func TestRatchetParseMetadataRejectsUnknownFormat(t *testing.T) {
	t.Parallel()

	_, err := Handler{}.parseMetadata(ratchetParserContext(EcosystemNPM), FetchTarget{DocumentFormat: "bogus"}, []byte(`{}`))
	if err == nil {
		t.Fatal("parseMetadata(bogus format) error = nil, want rejection")
	} else if !strings.Contains(err.Error(), `"bogus"`) {
		t.Fatalf("error = %q, want the format value named", err)
	}
}

// TestRatchetHandlerCollectsArtifactoryWrappedFacts pins the artifactory
// format lane through the claim path: the wrapper unwraps to native npm
// facts and the result validates.
func TestRatchetHandlerCollectsArtifactoryWrappedFacts(t *testing.T) {
	t.Parallel()

	const scopeID = "npm://registry.npmjs.org/left-pad"
	const metadataURL = "https://registry.invalid/left-pad"
	wrapper := `{"package_type":"npm","repository":"npm-remote",` +
		`"metadata":{"name":"left-pad","versions":{"1.3.0":{}}}}`
	provider := stubProvider{documents: map[string]MetadataDocument{
		metadataURL: {Body: []byte(wrapper), ObservedAt: time.Now().UTC()},
	}}
	request := ratchetPackumentRequest(t, scopeID, metadataURL, func(target map[string]any) {
		target["document_format"] = DocumentFormatArtifactoryPackage
	})
	result := Handler{Provider: provider}.Handle(context.Background(), request)
	if result.State != sdk.ResultComplete {
		t.Fatalf("State=%q, want complete", result.State)
	}
	if len(result.Facts) != 3 {
		t.Fatalf("facts=%d, want package plus version plus repository hosting", len(result.Facts))
	}
	if _, err := sdk.NewValidator(Contract()).ValidateResult(result); err != nil {
		t.Fatalf("ValidateResult() error = %v", err)
	}
}

// TestRatchetHandlerPackageMismatchIsInvalidEvidence pins the bound-metadata
// failure: a document for an unconfigured package ends terminal with
// invalid_evidence and no facts.
func TestRatchetHandlerPackageMismatchIsInvalidEvidence(t *testing.T) {
	t.Parallel()

	const scopeID = "npm://registry.npmjs.org/left-pad"
	const metadataURL = "https://registry.invalid/left-pad"
	provider := stubProvider{documents: map[string]MetadataDocument{
		metadataURL: {Body: []byte(`{"name": "other-package", "versions": {"1.0.0": {}}}`), ObservedAt: time.Now().UTC()},
	}}
	request := ratchetPackumentRequest(t, scopeID, metadataURL, nil)
	result := Handler{Provider: provider}.Handle(context.Background(), request)
	if result.State != sdk.ResultTerminal {
		t.Fatalf("State=%q, want terminal", result.State)
	}
	if len(result.Facts) != 0 {
		t.Fatalf("facts=%d, want 0 for an unconfigured package", len(result.Facts))
	}
	if len(result.Statuses) != 1 || result.Statuses[0].FailureClass != FailureInvalidEvidence {
		t.Fatalf("statuses=%v, want one invalid_evidence status", result.Statuses)
	}
}

// TestRatchetHandlerBudgetFailures pins the three budget guards: a fact
// count over the claim bound, a payload over its byte bound, and an encoded
// result over its byte bound each end terminal with the named class.
func TestRatchetHandlerBudgetFailures(t *testing.T) {
	t.Parallel()

	const scopeID = "npm://registry.npmjs.org/left-pad"
	const metadataURL = "https://registry.invalid/left-pad"
	document := []byte(`{"name": "left-pad", "versions": {"1.3.0": {"dist": {"tarball": "https://registry.npmjs.org/left-pad/-/left-pad-1.3.0.tgz"}}}}`)

	newRequest := func(limits map[string]any) Request {
		target := npmTarget(scopeID, metadataURL)
		request := Request{
			ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
			Claim:           testClaim(scopeID),
			Contract:        Contract(),
			Config:          testConfig(target),
		}
		request.Config["limits"] = limits
		return request
	}
	provider := func() stubProvider {
		return stubProvider{documents: map[string]MetadataDocument{
			metadataURL: {Body: document, ObservedAt: time.Now().UTC()},
		}}
	}

	records := Handler{Provider: provider()}.Handle(context.Background(), newRequest(map[string]any{"maxRecordsPerClaim": 1}))
	if records.State != sdk.ResultTerminal {
		t.Fatalf("records State=%q, want terminal", records.State)
	}
	if len(records.Statuses) != 1 || records.Statuses[0].FailureClass != FailureRecordBudget {
		t.Fatalf("records statuses=%v, want one record-budget status", records.Statuses)
	}

	payload := Handler{Provider: provider()}.Handle(context.Background(), newRequest(map[string]any{"maxPayloadBytes": 10}))
	if payload.State != sdk.ResultTerminal {
		t.Fatalf("payload State=%q, want terminal", payload.State)
	}
	if len(payload.Statuses) != 1 || payload.Statuses[0].FailureClass != FailurePayloadBudget {
		t.Fatalf("payload statuses=%v, want one payload-budget status", payload.Statuses)
	}

	encoded := Handler{Provider: provider()}.Handle(context.Background(), newRequest(map[string]any{"maxResultBytes": 10}))
	if encoded.State != sdk.ResultTerminal {
		t.Fatalf("encoded State=%q, want terminal", encoded.State)
	}
	if len(encoded.Statuses) != 1 || encoded.Statuses[0].FailureClass != FailureResultBudget {
		t.Fatalf("encoded statuses=%v, want one result-budget status", encoded.Statuses)
	}
}

// TestRatchetHandlerPartialResultBudgetIsTerminal pins the partial-path
// budget: a derived missing document that would complete partial still ends
// terminal when the encoded result exceeds its byte bound.
func TestRatchetHandlerPartialResultBudgetIsTerminal(t *testing.T) {
	t.Parallel()

	claim := testClaim("npm://registry.npmjs.org/left-pad")
	request := Request{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
		Claim:           claim,
		Contract:        Contract(),
		Config: map[string]any{
			"derive_from_owned_packages": map[string]any{
				"enabled": true, "ecosystems": []any{"npm"},
			},
			"limits": map[string]any{"maxResultBytes": 10},
		},
	}
	result := Handler{Provider: stubProvider{documents: map[string]MetadataDocument{}}}.Handle(context.Background(), request)
	if result.State != sdk.ResultTerminal {
		t.Fatalf("State=%q, want terminal", result.State)
	}
	if len(result.Statuses) != 1 || result.Statuses[0].FailureClass != FailureResultBudget {
		t.Fatalf("statuses=%v, want one result-budget status", result.Statuses)
	}
}

// TestRatchetHandlerAccessors pins the Handler zero-value fallbacks: the
// process environment, wall clock, and HTTP provider are selected at the
// call site, and the source system resolves from the claim first.
func TestRatchetHandlerAccessors(t *testing.T) {
	t.Parallel()

	var handler Handler
	if got := handler.getenv("ESHU_TEST_MISSING_VARIABLE"); got != "" {
		t.Fatalf("getenv without override = %q, want blank", got)
	}
	if _, ok := any(handler.provider()).(HTTPMetadataProvider); !ok {
		t.Fatalf("provider without override = %T, want the HTTP provider", handler.provider())
	}
	before := time.Now().UTC()
	got := handler.now()
	if got.Before(before) || got.After(time.Now().UTC()) {
		t.Fatalf("now() = %v, want the wall clock in UTC", got)
	}
	if system := claimSourceSystem(sdk.Claim{}); system != SourceSystem {
		t.Fatalf("blank source system = %q, want the component default", system)
	}
	if system := claimSourceSystem(sdk.Claim{SourceSystem: " custom "}); system != "custom" {
		t.Fatalf("claim source system = %q, want trimmed", system)
	}
	if stoppedAtClaimDeadline(context.Background()) {
		t.Fatal("stoppedAtClaimDeadline(background) = true, want false")
	}
	expired, cancel := context.WithTimeoutCause(context.Background(), time.Nanosecond, errClaimDeadline)
	defer cancel()
	<-expired.Done()
	if !stoppedAtClaimDeadline(expired) {
		t.Fatal("stoppedAtClaimDeadline(lease cause) = false, want true")
	}
}

// TestRatchetClaimContextVariants pins the claim-context bounds: zero limits
// take the default timeout, a nearer lease deadline wins, and a non-positive
// timeout reports false instead of arming a context.
func TestRatchetClaimContextVariants(t *testing.T) {
	t.Parallel()

	started := time.Now().UTC()
	far := testClaim("npm://registry.npmjs.org/left-pad")
	if _, cancel, ok := claimContext(context.Background(), far, started, ResourceUse{}); !ok {
		t.Fatal("claimContext(default limits) ok = false, want true")
	} else {
		cancel()
	}
	near := testClaim("npm://registry.npmjs.org/left-pad")
	near.Deadline = started.Add(time.Minute)
	if _, cancel, ok := claimContext(context.Background(), near, started, ResourceUse{}); !ok {
		t.Fatal("claimContext(near deadline) ok = false, want true")
	} else {
		cancel()
	}
	spent := testClaim("npm://registry.npmjs.org/left-pad")
	spent.Deadline = started.Add(-time.Second)
	if _, _, ok := claimContext(context.Background(), spent, started, ResourceUse{}); ok {
		t.Fatal("claimContext(spent lease) ok = true, want false: no work may start")
	}
	nodeadline := testClaim("npm://registry.npmjs.org/left-pad")
	nodeadline.Deadline = time.Time{}
	if _, cancel, ok := claimContext(context.Background(), nodeadline, started, ResourceUse{}); !ok {
		t.Fatal("claimContext(no deadline) ok = false, want true")
	} else {
		cancel()
	}
}

// TestRatchetHandlerSpentLeaseFailsFast is the regression test for a
// fail-open lease bug: a claim arriving with an expired lease must end
// retryable with the claim-deadline class before any provider call, never
// collect facts against a lease the host already spent.
func TestRatchetHandlerSpentLeaseFailsFast(t *testing.T) {
	t.Parallel()

	const scopeID = "npm://registry.npmjs.org/left-pad"
	const metadataURL = "https://registry.invalid/left-pad"
	provider := stubProvider{documents: map[string]MetadataDocument{
		metadataURL: {Body: []byte(`{"name": "left-pad", "versions": {"1.3.0": {}}}`), ObservedAt: time.Now().UTC()},
	}}
	request := ratchetPackumentRequest(t, scopeID, metadataURL, nil)
	request.Claim.Deadline = time.Now().UTC().Add(-time.Second)
	result := Handler{Provider: provider}.Handle(context.Background(), request)
	if result.State != sdk.ResultRetryable {
		t.Fatalf("State=%q, want retryable", result.State)
	}
	if len(result.Facts) != 0 {
		t.Fatalf("facts=%d, want 0: a spent lease must not collect", len(result.Facts))
	}
	if len(result.Statuses) != 1 || result.Statuses[0].FailureClass != FailureClaimDeadline {
		t.Fatalf("statuses=%v, want one claim-deadline status", result.Statuses)
	}
}

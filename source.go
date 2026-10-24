// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	sdk "github.com/eshu-hq/eshu-sdk/collector"
)

const (
	// maxDeadlineMargin caps how long before the claim deadline the collector
	// stops starting provider calls.
	maxDeadlineMargin = 2 * time.Second
	// deadlineMarginDivisor makes the margin 5% of the remaining claim time.
	deadlineMarginDivisor = 20
)

// errClaimDeadline is the cancellation cause of a context that ended because
// the claim lease deadline (less its margin) was reached. It tells that stop
// apart from a host cancel and from the configured claim timeout.
var errClaimDeadline = errors.New("claim lease deadline reached")

// Handler runs one claim. The zero value is usable: it reads the process
// environment, the wall clock, and the real registry HTTP client.
type Handler struct {
	// Getenv resolves credential environment variables; nil selects os.Getenv
	// at the call site in cmd/collector.
	Getenv func(string) string
	// Now supplies the clock; nil selects time.Now.
	Now func() time.Time
	// Provider fetches metadata documents; nil selects the HTTP provider.
	Provider MetadataProvider
	// Log receives one bounded JSON diagnostic line per claim; nil discards
	// it. The binary points it at stderr, which the extension host's process
	// runner does not surface (it keeps only a byte count), so the line helps
	// a local run and a test, not an operator in production.
	Log io.Writer
}

func (h Handler) now() time.Time {
	if h.Now != nil {
		return h.Now().UTC()
	}
	return time.Now().UTC()
}

func (h Handler) getenv(key string) string {
	if h.Getenv != nil {
		return h.Getenv(key)
	}
	return ""
}

func (h Handler) provider() MetadataProvider {
	if h.Provider != nil {
		return h.Provider
	}
	return HTTPMetadataProvider{}
}

// Handle collects the package-registry target named by the claim and returns
// the SDK result the host validates and commits. It never returns facts
// together with a failure state, never lets a credential into a result, and
// echoes the claim unchanged so the host's identity check passes.
func (h Handler) Handle(ctx context.Context, request Request) sdk.Result {
	started := h.now()
	claim := request.Claim
	settings, err := ParseSettings(request.Config, claim.InstanceID, h.getenv)
	if err != nil {
		// ConfigError text names config keys only and never quotes a
		// configured value, so it is safe for the diagnostic log line.
		h.logConfigFailure(claim, err)
		return budgetResult(claim, started, FailureInvalidConfig)
	}
	runCtx, cancel, ok := claimContext(ctx, claim, started, settings.Limits)
	defer cancel()
	if !ok {
		h.logFailure(claim, FailureClaimDeadline, "", 0, 0, 0)
		return failureResult(claim, started, Failure{Class: FailureClaimDeadline}, 0)
	}
	targets := make(map[string]FetchTarget, len(settings.Targets))
	for _, target := range settings.Targets {
		targets[target.Base.ScopeID] = target
	}
	target, derived, err := derivedTargetForScope(targets, settings.DerivedTargets, claim.Scope.ID)
	if err != nil {
		h.logConfigFailure(claim, err)
		return budgetResult(claim, started, FailureInvalidConfig)
	}
	factCtx := FactContext{
		ScopeID:             target.Base.ScopeID,
		GenerationID:        claim.GenerationID,
		CollectorInstanceID: settings.CollectorInstanceID,
		SourceSystem:        claimSourceSystem(claim),
		ObservedAt:          started,
		SourceURI:           firstNonBlank(target.Base.SourceURI, target.MetadataURL),
	}
	if warning, err := prefetchWarningFact(target, factCtx); err != nil {
		h.logFailure(claim, FailureInvalidEvidence, "prefetch", 0, 0, 0)
		return failureResult(claim, started, classifyFailure(err), h.now().Sub(started))
	} else if warning != nil {
		return h.completeResult(claim, settings, []sdk.Fact{*warning}, 1, started)
	}
	document, err := h.provider().FetchMetadata(runCtx, target)
	if err != nil {
		if stoppedAtClaimDeadline(runCtx) {
			// The lease deadline ended the read. Whatever was collected so
			// far is incomplete, so it is dropped: the claim ends retryable
			// with no facts and the next attempt starts again.
			latency := h.now().Sub(started)
			h.logFailure(claim, FailureClaimDeadline, "fetch_metadata", 0, 0, 0)
			return failureResult(claim, started, Failure{Class: FailureClaimDeadline}, latency)
		}
		if isMetadataTooLarge(err) {
			warning, warningErr := metadataTooLargeWarningFact(target, factCtx)
			if warningErr == nil {
				return h.completeResult(claim, settings, []sdk.Fact{warning}, 1, started)
			}
			err = errors.Join(err, warningErr)
		}
		if derived && isRegistryNotFound(err) {
			warning, warningErr := missingMetadataWarningFact(target, factCtx)
			if warningErr == nil {
				return h.partialResult(claim, settings, []sdk.Fact{warning}, started)
			}
			err = errors.Join(err, warningErr)
		}
		failure := classifyFailure(err)
		latency := h.now().Sub(started)
		h.logFailure(claim, failure.Class, failure.Operation, failure.HTTPStatus, 0, 0)
		return failureResult(claim, started, failure, latency)
	}
	return h.collectDocument(runCtx, claim, settings, target, factCtx, document, started)
}

func (h Handler) collectDocument(
	ctx context.Context,
	claim sdk.Claim,
	settings Settings,
	target FetchTarget,
	factCtx FactContext,
	document MetadataDocument,
	started time.Time,
) sdk.Result {
	observedAt := document.ObservedAt.UTC()
	if observedAt.IsZero() {
		observedAt = h.now().UTC()
	}
	factCtx.ObservedAt = observedAt
	sourceURI := safeSourceURI(firstNonBlank(document.SourceURI, target.Base.SourceURI, target.MetadataURL))
	factCtx.SourceURI = sourceURI
	parserCtx := MetadataParserContext{
		Ecosystem:           target.Base.Ecosystem,
		Registry:            target.Base.Registry,
		ScopeID:             target.Base.ScopeID,
		GenerationID:        claim.GenerationID,
		CollectorInstanceID: settings.CollectorInstanceID,
		ObservedAt:          observedAt,
		SourceURI:           sourceURI,
		Visibility:          target.Base.Visibility,
	}
	parsed, err := h.parseMetadata(parserCtx, target, document.Body)
	if err != nil {
		if warning, handled, warningErr := parserWarningFact(target, factCtx, err); warningErr == nil && handled {
			return h.partialResult(claim, settings, []sdk.Fact{warning}, started)
		} else if warningErr != nil {
			err = errors.Join(err, warningErr)
		}
		failure := classifyFailure(InvalidEvidenceError{Operation: "parse_metadata", Err: err})
		latency := h.now().Sub(started)
		h.logFailure(claim, failure.Class, failure.Operation, 0, 0, 0)
		return failureResult(claim, started, failure, latency)
	}
	parsed, err = boundedParsedMetadata(target.Base, parsed)
	if err != nil {
		failure := classifyFailure(InvalidEvidenceError{Operation: "bound_metadata", Err: err})
		latency := h.now().Sub(started)
		h.logFailure(claim, failure.Class, failure.Operation, 0, 0, 0)
		return failureResult(claim, started, failure, latency)
	}
	facts, err := buildFacts(factCtx, parsed)
	if err != nil {
		failure := classifyFailure(InvalidEvidenceError{Operation: "build_facts", Err: err})
		latency := h.now().Sub(started)
		h.logFailure(claim, failure.Class, failure.Operation, 0, 0, 0)
		return failureResult(claim, started, failure, latency)
	}
	facts = dedupeFacts(facts)
	return h.completeResult(claim, settings, facts, countWarnings(parsed), started)
}

func (h Handler) parseMetadata(
	ctx MetadataParserContext,
	target FetchTarget,
	document []byte,
) (ParsedMetadata, error) {
	registry := DefaultMetadataParserRegistry()
	switch target.DocumentFormat {
	case "", DocumentFormatNative:
		return registry.Parse(ctx, document)
	case DocumentFormatArtifactoryPackage:
		return ParseArtifactoryPackageMetadataWithRegistry(ctx, document, registry)
	default:
		return ParsedMetadata{}, fmt.Errorf("unsupported package registry document_format %q", target.DocumentFormat)
	}
}

func countWarnings(parsed ParsedMetadata) int {
	return len(parsed.Warnings)
}

// claimSourceSystem resolves the source system facts carry: the claim's when
// the host names one, else the component default.
func claimSourceSystem(claim sdk.Claim) string {
	if trimmed := strings.TrimSpace(claim.SourceSystem); trimmed != "" {
		return trimmed
	}
	return SourceSystem
}

// clampedDeadlineTimeout returns how long the collector may work on a claim:
// the time left until deadline minus a safety margin of min(2s, 5% of the time
// left). It reports false when there is no deadline or none left.
func clampedDeadlineTimeout(now, deadline time.Time) (time.Duration, bool) {
	if deadline.IsZero() {
		return 0, false
	}
	remaining := deadline.Sub(now)
	if remaining <= 0 {
		return 0, false
	}
	margin := min(maxDeadlineMargin, remaining/deadlineMarginDivisor)
	return remaining - margin, true
}

// claimContext bounds the collection by the claim lease deadline (less its
// margin) and the configured claim timeout, whichever is shorter. It works in
// durations measured on the handler clock and applies them with
// context.WithTimeoutCause, so an injected clock never skews the real-time
// bound. ok is false when the lease is already spent.
func claimContext(
	ctx context.Context,
	claim sdk.Claim,
	started time.Time,
	limits ResourceUse,
) (context.Context, context.CancelFunc, bool) {
	timeout := time.Duration(limits.ClaimTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = time.Duration(DefaultResourceUse().ClaimTimeoutSeconds) * time.Second
	}
	if remaining, ok := clampedDeadlineTimeout(started, claim.Deadline); ok && remaining < timeout {
		timeout = remaining
	}
	if timeout <= 0 {
		return ctx, func() {}, false
	}
	bounded, cancel := context.WithTimeoutCause(ctx, timeout, errClaimDeadline)
	return bounded, cancel, true
}

// stoppedAtClaimDeadline reports whether the run context ended at the lease
// bound rather than by host cancel or timeout.
func stoppedAtClaimDeadline(ctx context.Context) bool {
	return errors.Is(context.Cause(ctx), errClaimDeadline)
}

func (h Handler) completeResult(
	claim sdk.Claim,
	settings Settings,
	facts []sdk.Fact,
	warningCount int,
	started time.Time,
) sdk.Result {
	latency := h.now().Sub(started)
	if len(facts) > settings.Limits.MaxRecordsPerClaim {
		h.logFailure(claim, FailureRecordBudget, "", 0, 0, 0)
		return budgetResult(claim, started, FailureRecordBudget)
	}
	for _, fact := range facts {
		raw, err := json.Marshal(fact.Payload)
		if err != nil || len(raw) > settings.Limits.MaxPayloadBytes {
			h.logFailure(claim, FailurePayloadBudget, "", 0, 0, 0)
			return budgetResult(claim, started, FailurePayloadBudget)
		}
	}
	result := completeResult(claim, started, facts, warningCount, packageFreshnessHint(facts), latency)
	if over, terminal := h.checkResultBudget(claim, settings, result); over {
		return terminal
	}
	if _, err := sdk.NewValidator(Contract()).ValidateResult(result); err != nil {
		h.logFailure(claim, FailureInvalidEvidence, "validate_result", 0, 0, 0)
		return failureResult(claim, started, Failure{Class: FailureInvalidEvidence, Terminal: true}, latency)
	}
	return result
}

func (h Handler) partialResult(
	claim sdk.Claim,
	settings Settings,
	facts []sdk.Fact,
	started time.Time,
) sdk.Result {
	latency := h.now().Sub(started)
	result := partialResult(claim, started, facts, packageFreshnessHint(facts), latency)
	if over, terminal := h.checkResultBudget(claim, settings, result); over {
		return terminal
	}
	if _, err := sdk.NewValidator(Contract()).ValidateResult(result); err != nil {
		h.logFailure(claim, FailureInvalidEvidence, "validate_result", 0, 0, 0)
		return failureResult(claim, started, Failure{Class: FailureInvalidEvidence, Terminal: true}, latency)
	}
	return result
}

// checkResultBudget fails terminal when the encoded result would exceed the
// host's stdout cap. It reports false when the result fits.
func (h Handler) checkResultBudget(claim sdk.Claim, settings Settings, result sdk.Result) (bool, sdk.Result) {
	raw, err := json.Marshal(result)
	if err != nil || len(raw) > settings.Limits.MaxResultBytes {
		h.logFailure(claim, FailureResultBudget, "", 0, 0, 0)
		return true, budgetResult(claim, result.Generation.ObservedAt, FailureResultBudget)
	}
	return false, sdk.Result{}
}

func (h Handler) logConfigFailure(claim sdk.Claim, err error) {
	h.logLine(claim, "config_failure", err.Error(), 0, 0, 0)
}

func (h Handler) logFailure(claim sdk.Claim, class, operation string, httpStatus, factCount, warningCount int) {
	h.logLine(claim, class, operation, httpStatus, factCount, warningCount)
}

func (h Handler) logLine(claim sdk.Claim, class, detail string, httpStatus, factCount, warningCount int) {
	if h.Log == nil {
		return
	}
	line, err := json.Marshal(map[string]any{
		"collector":        CollectorKind,
		"component_id":     claim.ComponentID,
		"instance_id":      claim.InstanceID,
		"scope_id":         claim.Scope.ID,
		"generation_id":    claim.GenerationID,
		"class":            class,
		"detail":           detail,
		"http_status":      httpStatus,
		"fact_count":       factCount,
		"warning_count":    warningCount,
		"source_system":    claim.SourceSystem,
		"collector_kind":   claim.CollectorKind,
		"attempt":          claim.Attempt,
		"observed_at":      h.now().UTC().Format(time.RFC3339Nano),
		"protocol":         sdk.ProtocolVersionV1Alpha1,
		"contract_facts":   len(Contract().Facts),
		"manifest_version": SchemaVersion,
	})
	if err != nil {
		return
	}
	_, _ = h.Log.Write(append(line, '\n'))
}

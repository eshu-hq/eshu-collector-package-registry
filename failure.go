// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"context"
	"errors"
	"math"
	"time"

	sdk "github.com/eshu-hq/eshu-sdk/collector"
)

const (
	// FailureInvalidConfig marks an activation config the collector cannot run.
	FailureInvalidConfig = "invalid_config"
	// FailureInvalidEvidence marks provider data that cannot become a fact.
	FailureInvalidEvidence = "invalid_evidence"
	// FailureClaimDeadline marks a claim whose lease deadline has passed.
	FailureClaimDeadline = "claim_deadline_exceeded"
	// FailureRecordBudget marks a claim past its fact-count bound.
	FailureRecordBudget = "record_budget_exceeded"
	// FailurePayloadBudget marks a fact past its payload-size bound.
	FailurePayloadBudget = "payload_budget_exceeded"
	// FailureResultBudget marks an encoded result past its size bound.
	FailureResultBudget = "result_budget_exceeded"

	defaultRetryAfterSeconds          = 30
	defaultRateLimitRetryAfterSeconds = 60
)

// FailureRateLimited names the rate-limited class for retry-after defaults.
const FailureRateLimited = RegistryFailureRateLimited

// Failure describes one bounded claim failure: the class and operation tokens
// are fixed vocabularies and never carry URLs, bodies, or credentials.
type Failure struct {
	Class      string
	Operation  string
	HTTPStatus int
	Terminal   bool
	RetryAfter time.Duration
}

// classifyFailure maps a collection error to its bounded failure. Registry
// failures keep their class, terminal bit, and retry-after; a metadata
// document that cannot become facts is terminal invalid evidence; anything
// else is a terminal failure with a named registry class.
func classifyFailure(err error) Failure {
	var invalid InvalidEvidenceError
	if errors.As(err, &invalid) {
		return Failure{Class: FailureInvalidEvidence, Operation: invalid.Operation, Terminal: true}
	}
	var registry RegistryFailure
	if errors.As(err, &registry) {
		return Failure{
			Class:      registry.FailureClass(),
			Operation:  registry.Operation,
			HTTPStatus: registry.FailureStatusCode(),
			Terminal:   registry.TerminalFailure(),
			RetryAfter: registry.RetryAfterDelay(),
		}
	}
	if errors.Is(err, context.Canceled) {
		return Failure{Class: RegistryFailureCanceled, Terminal: true}
	}
	return Failure{Class: RegistryFailureTerminal, Terminal: true}
}

// InvalidEvidenceError marks provider data that cannot become a fact. The
// operation names the pipeline stage; it never carries the document.
type InvalidEvidenceError struct {
	Operation string
	Err       error
}

func (e InvalidEvidenceError) Error() string {
	if e.Err != nil {
		return "invalid evidence in " + e.Operation + ": " + e.Err.Error()
	}
	return "invalid evidence in " + e.Operation
}

func (e InvalidEvidenceError) Unwrap() error { return e.Err }

// failureResult builds the SDK result for a failed claim: terminal when
// retrying cannot help, retryable otherwise. Neither carries facts.
func failureResult(claim sdk.Claim, observedAt time.Time, failure Failure, latency time.Duration) sdk.Result {
	state := sdk.ResultRetryable
	status := sdk.Status{
		Class:           sdk.StatusFailure,
		FailureClass:    failure.Class,
		SourceLatencyMS: latencyMillis(latency),
	}
	if failure.Terminal {
		state = sdk.ResultTerminal
	} else {
		status.RetryAfterSeconds = retryAfterSeconds(failure)
	}
	return sdk.Result{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
		State:           state,
		Claim:           claim,
		Generation:      sdk.Generation{ID: claim.GenerationID, ObservedAt: observedAt.UTC()},
		Statuses:        []sdk.Status{status},
	}
}

// budgetResult is the bounded terminal shape for exhausted budgets and bad
// configuration: an explicit terminal state with a named class.
func budgetResult(claim sdk.Claim, observedAt time.Time, class string) sdk.Result {
	return failureResult(claim, observedAt, Failure{Class: class, Terminal: true}, 0)
}

// completeResult builds the SDK result for a fully collected claim.
func completeResult(claim sdk.Claim, observedAt time.Time, facts []sdk.Fact, warningCount int, freshnessHint string, latency time.Duration) sdk.Result {
	return sdk.Result{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
		State:           sdk.ResultComplete,
		Claim:           claim,
		Generation: sdk.Generation{
			ID:            claim.GenerationID,
			ObservedAt:    observedAt.UTC(),
			FreshnessHint: freshnessHint,
		},
		Facts: facts,
		Statuses: []sdk.Status{{
			Class:           sdk.StatusComplete,
			WarningCount:    warningCount,
			FactCount:       len(facts),
			SourceLatencyMS: latencyMillis(latency),
		}},
	}
}

// partialResult builds the SDK result for a claim that retrieved evidence but
// did not complete it: warning-only evidence with a warning status.
func partialResult(claim sdk.Claim, observedAt time.Time, facts []sdk.Fact, freshnessHint string, latency time.Duration) sdk.Result {
	return sdk.Result{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
		State:           sdk.ResultPartial,
		Claim:           claim,
		Generation: sdk.Generation{
			ID:            claim.GenerationID,
			ObservedAt:    observedAt.UTC(),
			FreshnessHint: freshnessHint,
		},
		Facts: facts,
		Statuses: []sdk.Status{{
			Class:           sdk.StatusWarning,
			Partial:         true,
			WarningCount:    len(facts),
			FactCount:       len(facts),
			SourceLatencyMS: latencyMillis(latency),
		}},
	}
}

func retryAfterSeconds(failure Failure) int {
	if failure.RetryAfter > 0 {
		return int(math.Ceil(failure.RetryAfter.Seconds()))
	}
	if failure.Class == FailureRateLimited {
		return defaultRateLimitRetryAfterSeconds
	}
	return defaultRetryAfterSeconds
}

func latencyMillis(latency time.Duration) int {
	if latency <= 0 {
		return 0
	}
	return int(latency / time.Millisecond)
}

// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"errors"
	"testing"
)

// TestInvalidEvidenceErrorWithoutCause pins the fail-soft message when no
// underlying error is carried: only the pipeline stage is named.
func TestInvalidEvidenceErrorWithoutCause(t *testing.T) {
	t.Parallel()

	err := InvalidEvidenceError{Operation: "parse_npm"}
	if got := err.Error(); got != "invalid evidence in parse_npm" {
		t.Fatalf("Error() = %q, want %q", got, "invalid evidence in parse_npm")
	}
	if err.Unwrap() != nil {
		t.Fatalf("Unwrap() = %v, want nil", err.Unwrap())
	}
}

// TestInvalidEvidenceErrorWithCause pins the message with a cause and that
// errors.As finds the InvalidEvidenceError while errors.Is reaches the cause.
func TestInvalidEvidenceErrorWithCause(t *testing.T) {
	t.Parallel()

	cause := errors.New("boom")
	err := InvalidEvidenceError{Operation: "parse_npm", Err: cause}
	if got := err.Error(); got != "invalid evidence in parse_npm: boom" {
		t.Fatalf("Error() = %q, want %q", got, "invalid evidence in parse_npm: boom")
	}
	if err.Unwrap() != cause {
		t.Fatalf("Unwrap() = %v, want the cause", err.Unwrap())
	}
	var invalid InvalidEvidenceError
	if !errors.As(err, &invalid) {
		t.Fatal("errors.As must find InvalidEvidenceError")
	}
	if !errors.Is(err, cause) {
		t.Fatal("errors.Is must reach the wrapped cause")
	}
}

// TestClassifyFailureMapsInvalidEvidence pins the classifyFailure branch that
// turns unparseable provider data into terminal invalid evidence.
func TestClassifyFailureMapsInvalidEvidence(t *testing.T) {
	t.Parallel()

	failure := classifyFailure(InvalidEvidenceError{Operation: "parse_npm", Err: errors.New("boom")})
	if failure.Class != FailureInvalidEvidence {
		t.Fatalf("Class=%q, want %q", failure.Class, FailureInvalidEvidence)
	}
	if failure.Operation != "parse_npm" {
		t.Fatalf("Operation=%q, want parse_npm", failure.Operation)
	}
	if !failure.Terminal {
		t.Fatal("invalid evidence must be terminal")
	}
}

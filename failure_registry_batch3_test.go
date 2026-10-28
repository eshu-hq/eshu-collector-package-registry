// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestClassifyFailureMapsRegistryFailure pins the registry branch: a wrapped
// provider failure keeps its class, operation, status, terminal bit, and
// retry-after instead of collapsing to a generic terminal failure.
func TestClassifyFailureMapsRegistryFailure(t *testing.T) {
	t.Parallel()

	cause := errors.New("connection reset")
	registryErr := RegistryFailure{
		Class:      RegistryFailureRetryable,
		Operation:  "fetch_metadata",
		StatusCode: 503,
		RetryAfter: 5 * time.Second,
		Cause:      cause,
	}
	failure := classifyFailure(fmt.Errorf("fetch: %w", registryErr))
	if failure.Class != RegistryFailureRetryable {
		t.Fatalf("Class=%q, want %q", failure.Class, RegistryFailureRetryable)
	}
	if failure.Operation != "fetch_metadata" {
		t.Fatalf("Operation=%q, want fetch_metadata", failure.Operation)
	}
	if failure.HTTPStatus != 503 {
		t.Fatalf("HTTPStatus=%d, want 503", failure.HTTPStatus)
	}
	if failure.Terminal {
		t.Fatal("retryable registry failure must not be terminal")
	}
	if failure.RetryAfter != 5*time.Second {
		t.Fatalf("RetryAfter=%v, want 5s", failure.RetryAfter)
	}
}

// TestClassifyFailureMapsTerminalRegistryFailure pins that a terminal
// registry class (auth denied) keeps its terminal bit through classification.
func TestClassifyFailureMapsTerminalRegistryFailure(t *testing.T) {
	t.Parallel()

	failure := classifyFailure(RegistryFailure{Class: RegistryFailureAuthDenied, Operation: "fetch_metadata", StatusCode: 403})
	if failure.Class != RegistryFailureAuthDenied {
		t.Fatalf("Class=%q, want %q", failure.Class, RegistryFailureAuthDenied)
	}
	if failure.HTTPStatus != 403 {
		t.Fatalf("HTTPStatus=%d, want 403", failure.HTTPStatus)
	}
	if !failure.Terminal {
		t.Fatal("auth-denied registry failure must be terminal")
	}
}

// TestClassifyFailureMapsCancellation pins the cancellation branch: a
// canceled collection context reports the canceled class and stays terminal.
func TestClassifyFailureMapsCancellation(t *testing.T) {
	t.Parallel()

	failure := classifyFailure(context.Canceled)
	if failure.Class != RegistryFailureCanceled {
		t.Fatalf("Class=%q, want %q", failure.Class, RegistryFailureCanceled)
	}
	if !failure.Terminal {
		t.Fatal("canceled failure must be terminal")
	}
}

// TestClassifyFailureDefaultsToTerminal pins the catch-all: an unrecognized
// error is a terminal failure with the named registry terminal class.
func TestClassifyFailureDefaultsToTerminal(t *testing.T) {
	t.Parallel()

	failure := classifyFailure(errors.New("boom"))
	if failure.Class != RegistryFailureTerminal {
		t.Fatalf("Class=%q, want %q", failure.Class, RegistryFailureTerminal)
	}
	if !failure.Terminal {
		t.Fatal("unknown failure must be terminal")
	}
}

// TestRetryAfterSeconds pins the retry-after defaults: an explicit
// server-advised delay wins (rounded up), a rate-limited failure without one
// waits 60s, and anything else waits 30s.
func TestRetryAfterSeconds(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		failure Failure
		want    int
	}{
		{"explicit_rounds_up", Failure{Class: RegistryFailureRetryable, RetryAfter: 1500 * time.Millisecond}, 2},
		{"explicit_exact", Failure{Class: RegistryFailureRetryable, RetryAfter: 5 * time.Second}, 5},
		{"explicit_wins_over_ratelimit", Failure{Class: FailureRateLimited, RetryAfter: 90 * time.Second}, 90},
		{"ratelimit_default", Failure{Class: FailureRateLimited}, 60},
		{"generic_default", Failure{Class: RegistryFailureRetryable}, 30},
		{"terminal_default", Failure{Class: RegistryFailureTerminal, Terminal: true}, 30},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := retryAfterSeconds(tc.failure); got != tc.want {
				t.Fatalf("retryAfterSeconds() = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestRegisterDuplicateEcosystemIsGuarded pins the guard mustRegisterParser
// relies on: registering the same ecosystem twice returns an error instead
// of replacing the parser. The panic itself is never exercised.
func TestRegisterDuplicateEcosystemIsGuarded(t *testing.T) {
	t.Parallel()

	registry := MetadataParserRegistry{}
	stub := func(MetadataParserContext, []byte) (ParsedMetadata, error) { return ParsedMetadata{}, nil }
	if err := registry.Register(EcosystemNPM, stub); err != nil {
		t.Fatalf("first Register() error = %v, want nil", err)
	}
	err := registry.Register(EcosystemNPM, stub)
	if err == nil {
		t.Fatal("second Register() = nil, want already-registered error")
	}
	if !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("second Register() error = %q, want it to mention already registered", err.Error())
	}
}

// TestRegisterTrimsDuplicateEcosystem pins that the duplicate guard applies
// after whitespace normalization: " npm " collides with "npm".
func TestRegisterTrimsDuplicateEcosystem(t *testing.T) {
	t.Parallel()

	registry := MetadataParserRegistry{}
	stub := func(MetadataParserContext, []byte) (ParsedMetadata, error) { return ParsedMetadata{}, nil }
	if err := registry.Register(EcosystemNPM, stub); err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	if err := registry.Register(Ecosystem("  npm  "), stub); err == nil {
		t.Fatal("Register(padded duplicate) = nil, want already-registered error")
	}
}

// TestRegisterRejectsBlankEcosystemAndNilParser pins the remaining Register
// guards: a blank ecosystem and a nil parser are both rejected.
func TestRegisterRejectsBlankEcosystemAndNilParser(t *testing.T) {
	t.Parallel()

	registry := MetadataParserRegistry{}
	stub := func(MetadataParserContext, []byte) (ParsedMetadata, error) { return ParsedMetadata{}, nil }
	if err := registry.Register(Ecosystem("   "), stub); err == nil {
		t.Fatal("Register(blank ecosystem) = nil, want required-ecosystem error")
	}
	if err := registry.Register(EcosystemGeneric, nil); err == nil {
		t.Fatal("Register(nil parser) = nil, want required-parser error")
	}
}

// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"testing"
)

// TestPayloadStringDecodesOrZero pins the fail-soft contract: a missing or
// wrong-typed payload value decodes to the zero value instead of panicking,
// so one malformed fact field cannot take down a claim.
func TestPayloadStringDecodesOrZero(t *testing.T) {
	t.Parallel()

	payload := map[string]any{
		"present": "left-pad",
		"number":  42,
	}
	if got := payloadString(payload, "present"); got != "left-pad" {
		t.Fatalf("payloadString(present) = %q, want %q", got, "left-pad")
	}
	if got := payloadString(payload, "missing"); got != "" {
		t.Fatalf("payloadString(missing) = %q, want zero", got)
	}
	if got := payloadString(payload, "number"); got != "" {
		t.Fatalf("payloadString(wrong type) = %q, want zero", got)
	}
}

// TestPayloadStringSliceDecodesOrNil pins the same contract for string
// slices: only a real []string survives, everything else decodes to nil.
func TestPayloadStringSliceDecodesOrNil(t *testing.T) {
	t.Parallel()

	want := []string{"a", "b"}
	payload := map[string]any{
		"present": want,
		"single":  "a",
	}
	if got := payloadStringSlice(payload, "present"); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("payloadStringSlice(present) = %q, want %q", got, want)
	}
	if got := payloadStringSlice(payload, "missing"); got != nil {
		t.Fatalf("payloadStringSlice(missing) = %q, want nil", got)
	}
	if got := payloadStringSlice(payload, "single"); got != nil {
		t.Fatalf("payloadStringSlice(wrong type) = %q, want nil", got)
	}
}

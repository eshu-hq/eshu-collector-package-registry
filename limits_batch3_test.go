// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"testing"
)

// TestResourceUseWithDefaultsFillsGaps pins the defaults contract: every
// non-positive field falls back to DefaultResourceUse while a customized
// value keeps the caller's setting.
func TestResourceUseWithDefaultsFillsGaps(t *testing.T) {
	t.Parallel()

	if got := (ResourceUse{}).withDefaults(); got != DefaultResourceUse() {
		t.Fatalf("withDefaults() = %+v, want the full defaults %+v", got, DefaultResourceUse())
	}
	custom := ResourceUse{
		MaxRecordsPerClaim:  7,
		MaxPayloadBytes:     8,
		MaxResultBytes:      9,
		ClaimTimeoutSeconds: 10,
	}
	if got := custom.withDefaults(); got != custom {
		t.Fatalf("withDefaults() = %+v, want the caller's values untouched", got)
	}
	partial := ResourceUse{MaxRecordsPerClaim: 7, ClaimTimeoutSeconds: 10}
	got := partial.withDefaults()
	defaults := DefaultResourceUse()
	if got.MaxRecordsPerClaim != 7 || got.ClaimTimeoutSeconds != 10 {
		t.Fatalf("withDefaults() = %+v, want the caller's values kept", got)
	}
	if got.MaxPayloadBytes != defaults.MaxPayloadBytes || got.MaxResultBytes != defaults.MaxResultBytes {
		t.Fatalf("withDefaults() = %+v, want the gaps filled from %+v", got, defaults)
	}
	if err := got.validate(); err != nil {
		t.Fatalf("validate() error = %v, want the merged bounds to stay valid", err)
	}
}

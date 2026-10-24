// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"context"
	"strings"
	"testing"

	sdk "github.com/eshu-hq/eshu-sdk/collector"
)

// TestParseSettingsRejectsUnknownVisibility pins the fail-closed visibility
// allowlist: a value outside the known set must be rejected at config load,
// and the error must name the key without quoting the configured value.
func TestParseSettingsRejectsUnknownVisibility(t *testing.T) {
	t.Parallel()

	cfg := validTargetConfig()
	cfg["visibility"] = "internal"
	_, err := ParseSettings(map[string]any{"targets": []any{cfg}}, "collector-1", func(string) string { return "" })
	if err == nil {
		t.Fatal("ParseSettings(unknown visibility) error = nil, want rejection")
	}
	if !strings.Contains(err.Error(), "visibility") {
		t.Fatalf("error %q must name the visibility key", err)
	}
	if strings.Contains(err.Error(), "internal") {
		t.Fatalf("error %q must not quote the configured value", err)
	}
}

// TestParseSettingsAcceptsKnownVisibilities pins the allowlist members: every
// known value, plus the empty default, loads without error.
func TestParseSettingsAcceptsKnownVisibilities(t *testing.T) {
	t.Parallel()

	for _, visibility := range []string{"", "unknown", "public", "private"} {
		cfg := validTargetConfig()
		cfg["visibility"] = visibility
		settings, err := ParseSettings(map[string]any{"targets": []any{cfg}}, "collector-1", func(string) string { return "" })
		if err != nil {
			t.Fatalf("ParseSettings(visibility=%q) error = %v", visibility, err)
		}
		want := Visibility(visibility)
		if want == "" {
			want = VisibilityUnknown
		}
		if settings.Targets[0].Base.Visibility != want {
			t.Fatalf("visibility=%q, want %q", settings.Targets[0].Base.Visibility, want)
		}
	}
}

// TestTargetConfigValidateRejectsUnknownVisibility pins the same allowlist on
// the direct runtime-config path, outside activation parsing.
func TestTargetConfigValidateRejectsUnknownVisibility(t *testing.T) {
	t.Parallel()

	base, err := RuntimeConfig{
		CollectorInstanceID: "collector-1",
		Targets: []TargetConfig{{
			Provider:   "npm",
			Ecosystem:  "npm",
			Registry:   "https://registry.npmjs.org",
			ScopeID:    "npm://registry.npmjs.org/left-pad",
			Visibility: "internal",
		}},
	}.Validate()
	if err == nil {
		t.Fatalf("Validate(unknown visibility) error = nil, want rejection (got %+v)", base)
	}
	if !strings.Contains(err.Error(), "visibility") {
		t.Fatalf("error %q must name the visibility key", err)
	}
}

// TestHandlerUnknownVisibilityIsTerminal pins the terminal config-load path:
// an unknown visibility fails the claim without a retry.
func TestHandlerUnknownVisibilityIsTerminal(t *testing.T) {
	t.Parallel()

	const scopeID = "npm://registry.npmjs.org/left-pad"
	target := npmTarget(scopeID, "https://registry.invalid/left-pad")
	target["visibility"] = "internal"
	handler := Handler{Provider: stubProvider{documents: map[string]MetadataDocument{}}}
	claim := testClaim(scopeID)
	request := Request{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
		Claim:           claim,
		Contract:        Contract(),
		Config:          testConfig(target),
	}
	result := handler.Handle(context.Background(), request)
	if result.State != sdk.ResultTerminal {
		t.Fatalf("State=%q, want terminal", result.State)
	}
	if len(result.Statuses) != 1 || result.Statuses[0].FailureClass != FailureInvalidConfig {
		t.Fatalf("statuses=%v, want one invalid_config status", result.Statuses)
	}
}

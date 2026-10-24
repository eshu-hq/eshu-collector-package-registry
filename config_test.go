// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"strings"
	"testing"
)

// TestTargetConfigValidateCanonicalizesEcosystemAlias pins the #9029 contract:
// a documented alias must normalize to its canonical ecosystem at config load
// instead of passing through and degrading to a warning at parse time.
func TestTargetConfigValidateCanonicalizesEcosystemAlias(t *testing.T) {
	t.Parallel()

	cases := map[Ecosystem]Ecosystem{
		"pip":     EcosystemPyPI,
		"nodejs":  EcosystemNPM,
		"golang":  EcosystemGoModule,
		"dotnet":  EcosystemNuGet,
		"gradle":  EcosystemMaven,
		"generic": EcosystemGeneric,
	}
	for alias, canonical := range cases {
		validated, err := TargetConfig{
			Provider:  "test",
			Ecosystem: alias,
			Registry:  "https://example.com",
			ScopeID:   "test://example.com/scope",
		}.Validate()
		if err != nil {
			t.Fatalf("Validate(ecosystem=%q) error = %v, want canonical %q", alias, err, canonical)
		}
		if validated.Ecosystem != canonical {
			t.Fatalf("Validate(ecosystem=%q) = %q, want canonical %q", alias, validated.Ecosystem, canonical)
		}
	}
}

// TestTargetConfigValidateRejectsUnknownEcosystem pins the #9029 fail-fast
// contract: an ecosystem no parser can serve must fail at config load, never
// degrade to a runtime warning.
func TestTargetConfigValidateRejectsUnknownEcosystem(t *testing.T) {
	t.Parallel()

	unknown := TargetConfig{
		Provider:  "test",
		Ecosystem: "not-a-real-ecosystem",
		Registry:  "https://example.com",
		ScopeID:   "test://example.com/scope",
	}
	if _, err := unknown.Validate(); err == nil {
		t.Fatal("Validate(unknown ecosystem) error = nil, want rejection")
	}
}

// TestParseSettingsCanonicalizesEcosystemAlias pins the activation-config
// path: an alias in the JSON config must reach the fetch target canonicalized.
func TestParseSettingsCanonicalizesEcosystemAlias(t *testing.T) {
	t.Parallel()

	cfg := validTargetConfig()
	cfg["ecosystem"] = "pip"
	settings, err := ParseSettings(map[string]any{"targets": []any{cfg}}, "collector-1", func(string) string { return "" })
	if err != nil {
		t.Fatalf("ParseSettings() error = %v", err)
	}
	if settings.Targets[0].Base.Ecosystem != EcosystemPyPI {
		t.Fatalf("ecosystem = %q, want canonical %q", settings.Targets[0].Base.Ecosystem, EcosystemPyPI)
	}
}

// TestParseSettingsRejectsUnknownEcosystem pins the activation-config
// fail-fast path: an unknown ecosystem must fail closed, and the error must
// name the key without quoting the configured value.
func TestParseSettingsRejectsUnknownEcosystem(t *testing.T) {
	t.Parallel()

	cfg := validTargetConfig()
	cfg["ecosystem"] = "not-a-real-ecosystem"
	_, err := ParseSettings(map[string]any{"targets": []any{cfg}}, "collector-1", func(string) string { return "" })
	if err == nil {
		t.Fatal("ParseSettings(unknown ecosystem) error = nil, want rejection")
	}
	if strings.Contains(err.Error(), "not-a-real-ecosystem") {
		t.Fatalf("error %q must not quote the configured value", err.Error())
	}
}

// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"errors"
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
// contract: an ecosystem value NormalizeEcosystem cannot map must fail at
// config load. Canonical-but-unregistered ecosystems (e.g. composer, which
// maps but has no parser) still pass load today and degrade to a runtime
// unsupported_metadata_source warning; see
// TestTargetConfigValidateAdmitsCanonicalButUnregisteredEcosystem.
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
	} else if !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("Validate() error = %q, want the unsupported branch", err)
	}
}

// TestTargetConfigValidateAdmitsCanonicalButUnregisteredEcosystem pins the
// disclosed load behavior: composer normalizes but has no registered parser,
// so it passes config load and degrades to a runtime warning instead of
// failing fast. Narrowing this to a load-time rejection is future work.
func TestTargetConfigValidateAdmitsCanonicalButUnregisteredEcosystem(t *testing.T) {
	t.Parallel()

	composer := TargetConfig{
		Provider:  "test",
		Ecosystem: "composer",
		Registry:  "https://example.com",
		ScopeID:   "test://example.com/scope",
	}
	if _, err := composer.Validate(); err != nil {
		t.Fatalf("Validate(composer) error = %v, want load success with runtime warning", err)
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
// fail-fast path: an unmappable ecosystem must fail closed with the
// unsupported branch (not a wrong-branch error), the invalid_config failure
// class, and an error that names the key without quoting the value.
func TestParseSettingsRejectsUnknownEcosystem(t *testing.T) {
	t.Parallel()

	cfg := validTargetConfig()
	cfg["ecosystem"] = "not-a-real-ecosystem"
	_, err := ParseSettings(map[string]any{"targets": []any{cfg}}, "collector-1", func(string) string { return "" })
	if err == nil {
		t.Fatal("ParseSettings(unknown ecosystem) error = nil, want rejection")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("ParseSettings() error = %q, want the unsupported branch", err)
	}
	var configErr ConfigError
	if !errors.As(err, &configErr) {
		t.Fatalf("ParseSettings() error type = %T, want ConfigError", err)
	}
	if configErr.FailureClass() != FailureInvalidConfig {
		t.Fatalf("FailureClass() = %q, want %q", configErr.FailureClass(), FailureInvalidConfig)
	}
	if strings.Contains(err.Error(), "not-a-real-ecosystem") {
		t.Fatalf("error %q must not quote the configured value", err.Error())
	}
}

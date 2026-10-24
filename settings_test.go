// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"errors"
	"strings"
	"testing"
)

func validTargetConfig() map[string]any {
	return map[string]any{
		"provider": "npm", "ecosystem": "npm", "registry": "https://registry.npmjs.org",
		"scope_id": "npm://registry.npmjs.org/left-pad", "packages": []any{"left-pad"},
		"metadata_url": "https://registry.npmjs.org/left-pad",
	}
}

// TestParseSettingsAcceptsMinimalTarget pins the happy path: one explicit
// target with defaults for limits and bounds.
func TestParseSettingsAcceptsMinimalTarget(t *testing.T) {
	t.Parallel()

	settings, err := ParseSettings(map[string]any{"targets": []any{validTargetConfig()}}, "collector-1", func(string) string { return "" })
	if err != nil {
		t.Fatalf("ParseSettings() error = %v", err)
	}
	if len(settings.Targets) != 1 {
		t.Fatalf("targets=%d, want 1", len(settings.Targets))
	}
	target := settings.Targets[0]
	if target.Base.PackageLimit != defaultPackageLimit || target.Base.VersionLimit != defaultVersionLimit {
		t.Fatalf("limits=%d/%d, want defaults %d/%d",
			target.Base.PackageLimit, target.Base.VersionLimit, defaultPackageLimit, defaultVersionLimit)
	}
	if settings.Limits.MaxRecordsPerClaim != DefaultResourceUse().MaxRecordsPerClaim {
		t.Fatalf("record bound=%d, want the default", settings.Limits.MaxRecordsPerClaim)
	}
}

// TestParseSettingsRejectsUnknownKeys pins fail-closed config parsing: a
// misspelled key must fail, never be silently ignored.
func TestParseSettingsRejectsUnknownKeys(t *testing.T) {
	t.Parallel()

	cfg := validTargetConfig()
	cfg["package_limt"] = 5
	if _, err := ParseSettings(map[string]any{"targets": []any{cfg}}, "collector-1", func(string) string { return "" }); err == nil {
		t.Fatal("ParseSettings(misspelled key) error = nil, want rejection")
	} else if !strings.Contains(err.Error(), "decode package registry collector config") {
		t.Fatalf("error %q must identify the config as the problem without quoting values", err)
	}
}

// TestParseSettingsRejectsOutOfRangeLimits pins the bound ceilings.
func TestParseSettingsRejectsOutOfRangeLimits(t *testing.T) {
	t.Parallel()

	bad := map[string]any{
		"targets": []any{validTargetConfig()},
		"limits":  map[string]any{"maxRecordsPerClaim": maxRecordsCeiling + 1},
	}
	if _, err := ParseSettings(bad, "collector-1", func(string) string { return "" }); err == nil {
		t.Fatal("ParseSettings(over-ceiling limit) error = nil, want rejection")
	}
}

// TestParseSettingsRejectsDuplicateScopes pins the duplicate-scope guard.
func TestParseSettingsRejectsDuplicateScopes(t *testing.T) {
	t.Parallel()

	cfg := map[string]any{"targets": []any{validTargetConfig(), validTargetConfig()}}
	if _, err := ParseSettings(cfg, "collector-1", func(string) string { return "" }); err == nil {
		t.Fatal("ParseSettings(duplicate scope) error = nil, want rejection")
	}
}

// TestParseSettingsRejectsBadDocumentFormat pins the format allowlist.
func TestParseSettingsRejectsBadDocumentFormat(t *testing.T) {
	t.Parallel()

	cfg := validTargetConfig()
	cfg["document_format"] = "carrier-pigeon"
	if _, err := ParseSettings(map[string]any{"targets": []any{cfg}}, "collector-1", func(string) string { return "" }); err == nil {
		t.Fatal("ParseSettings(bad document_format) error = nil, want rejection")
	}
}

// TestParseSettingsAcceptsDerivedOnlyConfig pins the derived-target path: no
// explicit target is fine when derivation is enabled.
func TestParseSettingsAcceptsDerivedOnlyConfig(t *testing.T) {
	t.Parallel()

	settings, err := ParseSettings(map[string]any{
		"derive_from_owned_packages": map[string]any{"enabled": true, "ecosystems": []any{"npm"}},
	}, "collector-1", func(string) string { return "" })
	if err != nil {
		t.Fatalf("ParseSettings() error = %v", err)
	}
	if !settings.DerivedTargets.Enabled {
		t.Fatal("derived targets must be enabled")
	}
}

// TestParseSettingsRejectsUnknownDerivedEcosystem pins the #9029 fail-closed
// derived path: an enabled derive block listing an ecosystem
// NormalizeEcosystem cannot map must fail as invalid_config, naming the key
// without quoting the value, instead of silently degrading to a disabled
// derived set.
func TestParseSettingsRejectsUnknownDerivedEcosystem(t *testing.T) {
	t.Parallel()

	_, err := ParseSettings(map[string]any{
		"derive_from_owned_packages": map[string]any{"enabled": true, "ecosystems": []any{"not-a-real-ecosystem"}},
	}, "collector-1", func(string) string { return "" })
	if err == nil {
		t.Fatal("ParseSettings(unknown derived ecosystem) error = nil, want rejection")
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

// TestParseSettingsRejectsUnknownDerivedAlongsideExplicitTargets pins the
// mixed case: valid explicit targets do not rescue an unknown derived entry.
func TestParseSettingsRejectsUnknownDerivedAlongsideExplicitTargets(t *testing.T) {
	t.Parallel()

	_, err := ParseSettings(map[string]any{
		"targets":                    []any{validTargetConfig()},
		"derive_from_owned_packages": map[string]any{"enabled": true, "ecosystems": []any{"npm", "not-a-real-ecosystem"}},
	}, "collector-1", func(string) string { return "" })
	if err == nil {
		t.Fatal("ParseSettings(explicit targets plus unknown derived) error = nil, want rejection")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("ParseSettings() error = %q, want the unsupported branch", err)
	}
}

// TestParseSettingsResolvesCredentialsFromEnv pins credential resolution: the
// resolved values travel in the target, raw names never enter errors.
func TestParseSettingsResolvesCredentialsFromEnv(t *testing.T) {
	t.Parallel()

	const token = "bearer-token-value"
	cfg := validTargetConfig()
	cfg["visibility"] = "private"
	cfg["bearer_token_env"] = "PKGREG_TOKEN"
	settings, err := ParseSettings(map[string]any{"targets": []any{cfg}}, "collector-1", func(key string) string {
		if key == "PKGREG_TOKEN" {
			return token
		}
		return ""
	})
	if err != nil {
		t.Fatalf("ParseSettings() error = %v", err)
	}
	if settings.Targets[0].BearerToken != token {
		t.Fatal("bearer token was not resolved from the environment")
	}
}

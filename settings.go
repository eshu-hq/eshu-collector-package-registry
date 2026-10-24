// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ConfigError reports an invalid activation config. Its text names config keys
// only. It never carries a resolved secret, and it never quotes a configured
// value (an environment variable name included), because a credential pasted
// into a *_env key would otherwise reach the diagnostic log line.
type ConfigError struct {
	msg   string
	class string
}

// Error returns the bounded configuration failure text.
func (e ConfigError) Error() string { return e.msg }

// FailureClass returns the bounded status class for this rejection.
func (e ConfigError) FailureClass() string {
	if e.class != "" {
		return e.class
	}
	return FailureInvalidConfig
}

func configErrorf(format string, args ...any) error {
	return ConfigError{msg: fmt.Sprintf(format, args...)}
}

// Settings is the validated per-claim configuration.
type Settings struct {
	CollectorInstanceID string
	Targets             []FetchTarget
	DerivedTargets      DerivedTargetConfig
	Limits              ResourceUse
}

// activationConfig is the activation `config` map shape. The field names
// mirror the in-tree collector's per-target JSON.
type activationConfig struct {
	Targets []targetJSON `json:"targets"`
	Derive  derivedJSON  `json:"derive_from_owned_packages"`
	Limits  *limitsJSON  `json:"limits"`
}

type targetJSON struct {
	Provider       string   `json:"provider"`
	Ecosystem      string   `json:"ecosystem"`
	Registry       string   `json:"registry"`
	ScopeID        string   `json:"scope_id"`
	Namespace      string   `json:"namespace"`
	Packages       []string `json:"packages"`
	PackageLimit   int      `json:"package_limit"`
	VersionLimit   int      `json:"version_limit"`
	Visibility     string   `json:"visibility"`
	SourceURI      string   `json:"source_uri"`
	MetadataURL    string   `json:"metadata_url"`
	DocumentFormat string   `json:"document_format"`
	UsernameEnv    string   `json:"username_env"`
	PasswordEnv    string   `json:"password_env"`
	BearerTokenEnv string   `json:"bearer_token_env"`
}

type derivedJSON struct {
	Enabled      bool     `json:"enabled"`
	Ecosystems   []string `json:"ecosystems"`
	PackageLimit int      `json:"package_limit"`
	VersionLimit int      `json:"version_limit"`
}

type limitsJSON struct {
	MaxRecordsPerClaim  *int `json:"maxRecordsPerClaim"`
	MaxPayloadBytes     *int `json:"maxPayloadBytes"`
	MaxResultBytes      *int `json:"maxResultBytes"`
	ClaimTimeoutSeconds *int `json:"claimTimeoutSeconds"`
}

// ParseSettings validates the activation config for the claimed scope. getenv
// resolves the credential environment variables the config names. Unknown
// keys fail closed so a misspelled limit cannot be silently ignored.
func ParseSettings(raw map[string]any, instanceID string, getenv func(string) string) (Settings, error) {
	var cfg activationConfig
	encoded, err := json.Marshal(raw)
	if err != nil {
		return Settings{}, configErrorf("package registry collector config is not JSON serializable")
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return Settings{}, configErrorf("decode package registry collector config: %s", decodeReason(err))
	}
	limits := DefaultResourceUse()
	if cfg.Limits != nil {
		if cfg.Limits.MaxRecordsPerClaim != nil {
			limits.MaxRecordsPerClaim = *cfg.Limits.MaxRecordsPerClaim
		}
		if cfg.Limits.MaxPayloadBytes != nil {
			limits.MaxPayloadBytes = *cfg.Limits.MaxPayloadBytes
		}
		if cfg.Limits.MaxResultBytes != nil {
			limits.MaxResultBytes = *cfg.Limits.MaxResultBytes
		}
		if cfg.Limits.ClaimTimeoutSeconds != nil {
			limits.ClaimTimeoutSeconds = *cfg.Limits.ClaimTimeoutSeconds
		}
	}
	if err := limits.validate(); err != nil {
		return Settings{}, err
	}
	targets := make([]FetchTarget, 0, len(cfg.Targets))
	bases := make([]TargetConfig, 0, len(cfg.Targets))
	for i, target := range cfg.Targets {
		mapped, err := mapTarget(target, getenv)
		if err != nil {
			return Settings{}, ConfigError{msg: fmt.Sprintf("targets[%d]: %s", i, err.Error())}
		}
		targets = append(targets, mapped)
		bases = append(bases, mapped.Base)
	}
	collectorID := strings.TrimSpace(instanceID)
	if collectorID == "" {
		return Settings{}, configErrorf("collector instance id is required")
	}
	if len(targets) > 0 {
		validated, err := RuntimeConfig{
			CollectorInstanceID: collectorID,
			Targets:             bases,
		}.Validate()
		if err != nil {
			return Settings{}, configErrorf("invalid package registry target: %s", err.Error())
		}
		seen := map[string]struct{}{}
		for i, base := range validated.Targets {
			if _, dup := seen[base.ScopeID]; dup {
				return Settings{}, configErrorf("duplicate package registry target scope_id")
			}
			seen[base.ScopeID] = struct{}{}
			targets[i].Base = base
		}
	}
	derived := mapDerivedTargets(cfg.Derive)
	if len(targets) == 0 && !derived.Enabled {
		return Settings{}, configErrorf("at least one package-registry target is required")
	}
	return Settings{
		CollectorInstanceID: collectorID,
		Targets:             targets,
		DerivedTargets:      derived,
		Limits:              limits.withDefaults(),
	}, nil
}

// mapTarget resolves one activation target, reading credentials from the
// environment variables the config names. Raw values never enter facts, logs,
// or statuses: only resolved values travel in the FetchTarget, and errors
// name keys, never values.
func mapTarget(target targetJSON, getenv func(string) string) (FetchTarget, error) {
	visibility := Visibility(strings.TrimSpace(target.Visibility))
	if visibility == "" {
		visibility = VisibilityUnknown
	}
	documentFormat := strings.TrimSpace(target.DocumentFormat)
	if documentFormat == "" {
		documentFormat = DocumentFormatNative
	}
	if documentFormat != DocumentFormatNative && documentFormat != DocumentFormatArtifactoryPackage {
		return FetchTarget{}, configErrorf("unsupported document_format")
	}
	ecosystem := NormalizeEcosystem(Ecosystem(target.Ecosystem))
	if ecosystem == "" {
		if strings.TrimSpace(target.Ecosystem) == "" {
			return FetchTarget{}, configErrorf("ecosystem is required")
		}
		return FetchTarget{}, configErrorf("ecosystem is unsupported")
	}
	return FetchTarget{
		Base: TargetConfig{
			Provider:     strings.TrimSpace(target.Provider),
			Ecosystem:    ecosystem,
			Registry:     strings.TrimRight(strings.TrimSpace(target.Registry), "/"),
			ScopeID:      strings.TrimSpace(target.ScopeID),
			Namespace:    strings.TrimSpace(target.Namespace),
			Packages:     target.Packages,
			PackageLimit: target.PackageLimit,
			VersionLimit: target.VersionLimit,
			Visibility:   visibility,
			SourceURI:    strings.TrimSpace(firstNonBlank(target.SourceURI, target.MetadataURL)),
		},
		MetadataURL:    strings.TrimRight(strings.TrimSpace(target.MetadataURL), "/"),
		DocumentFormat: documentFormat,
		Username:       getenv(strings.TrimSpace(target.UsernameEnv)),
		Password:       getenv(strings.TrimSpace(target.PasswordEnv)),
		BearerToken:    getenv(strings.TrimSpace(target.BearerTokenEnv)),
	}, nil
}

func mapDerivedTargets(config derivedJSON) DerivedTargetConfig {
	if !config.Enabled {
		return DerivedTargetConfig{}
	}
	ecosystems := make([]Ecosystem, 0, len(config.Ecosystems))
	for _, ecosystem := range config.Ecosystems {
		ecosystems = append(ecosystems, Ecosystem(strings.TrimSpace(ecosystem)))
	}
	return normalizeDerivedTargetConfig(DerivedTargetConfig{
		Enabled:      true,
		Ecosystems:   ecosystems,
		PackageLimit: config.PackageLimit,
		VersionLimit: config.VersionLimit,
	})
}

func envDuration(getenv func(string) string, key string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(getenv(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	if value <= 0 {
		return 0, fmt.Errorf("%s must be positive", key)
	}
	return value, nil
}

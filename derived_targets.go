// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"fmt"
	"net/url"
	"strings"
)

func normalizeDerivedTargetConfig(config DerivedTargetConfig) DerivedTargetConfig {
	if !config.Enabled {
		return DerivedTargetConfig{}
	}
	ecosystems := make([]Ecosystem, 0, len(config.Ecosystems))
	seen := map[Ecosystem]struct{}{}
	source := derivedTargetEcosystems(config.Ecosystems)
	for _, ecosystem := range source {
		if ecosystem == "" {
			continue
		}
		if _, ok := seen[ecosystem]; ok {
			continue
		}
		seen[ecosystem] = struct{}{}
		ecosystems = append(ecosystems, ecosystem)
	}
	packageLimit := config.PackageLimit
	if packageLimit <= 0 {
		packageLimit = 1
	}
	versionLimit := config.VersionLimit
	if versionLimit <= 0 {
		versionLimit = 1
	}
	return DerivedTargetConfig{
		Enabled:      len(ecosystems) > 0,
		Ecosystems:   ecosystems,
		PackageLimit: packageLimit,
		VersionLimit: versionLimit,
	}
}

func derivedTargetForScope(targets map[string]FetchTarget, derived DerivedTargetConfig, scopeID string) (FetchTarget, bool, error) {
	if target, ok := targets[strings.TrimSpace(scopeID)]; ok {
		return target, false, nil
	}
	if !derived.Enabled {
		return FetchTarget{}, false, fmt.Errorf("package registry target scope_id %q is not configured", scopeID)
	}
	target, ok := derivedTarget(scopeID, derived)
	if !ok {
		return FetchTarget{}, false, fmt.Errorf("package registry target scope_id %q is not configured", scopeID)
	}
	return target, true, nil
}

func derivedTarget(scopeID string, config DerivedTargetConfig) (FetchTarget, bool) {
	if target, ok := derivedNPMTarget(scopeID, config); ok {
		return target, true
	}
	if target, ok := derivedPyPITarget(scopeID, config); ok {
		return target, true
	}
	return derivedUnsupportedTarget(scopeID, config)
}

func derivedNPMTarget(scopeID string, config DerivedTargetConfig) (FetchTarget, bool) {
	if !derivedTargetEcosystemEnabled(config, EcosystemNPM) {
		return FetchTarget{}, false
	}
	parsed, err := url.Parse(scopeID)
	if err != nil || parsed.Scheme != string(EcosystemNPM) || parsed.Host != "registry.npmjs.org" {
		return FetchTarget{}, false
	}
	packageName, err := url.PathUnescape(strings.TrimPrefix(parsed.EscapedPath(), "/"))
	if err != nil || strings.TrimSpace(packageName) == "" {
		return FetchTarget{}, false
	}
	identity, err := NormalizePackageIdentity(PackageIdentity{
		Ecosystem: EcosystemNPM,
		Registry:  "https://registry.npmjs.org",
		RawName:   packageName,
	})
	if err != nil || identity.PackageID != scopeID {
		return FetchTarget{}, false
	}
	metadataURL := "https://registry.npmjs.org/" + url.PathEscape(identity.NormalizedName)
	return FetchTarget{
		Base: TargetConfig{
			Provider:     "npm",
			Ecosystem:    EcosystemNPM,
			Registry:     "https://registry.npmjs.org",
			ScopeID:      identity.PackageID,
			Namespace:    identity.Namespace,
			Packages:     []string{identity.NormalizedName},
			PackageLimit: config.PackageLimit,
			VersionLimit: config.VersionLimit,
			Visibility:   VisibilityUnknown,
			SourceURI:    metadataURL,
		},
		MetadataURL: metadataURL,
	}, true
}

func derivedPyPITarget(scopeID string, config DerivedTargetConfig) (FetchTarget, bool) {
	if !derivedTargetEcosystemEnabled(config, EcosystemPyPI) {
		return FetchTarget{}, false
	}
	identity, ok := normalizedDerivedIdentity(scopeID, EcosystemPyPI)
	if !ok || identity.Registry != "pypi.org/pypi" {
		return FetchTarget{}, false
	}
	metadataURL := "https://pypi.org/pypi/" + url.PathEscape(identity.NormalizedName) + "/json"
	return FetchTarget{
		Base: TargetConfig{
			Provider:     "pypi",
			Ecosystem:    EcosystemPyPI,
			Registry:     "https://pypi.org/pypi",
			ScopeID:      identity.PackageID,
			Packages:     []string{identity.NormalizedName},
			PackageLimit: config.PackageLimit,
			VersionLimit: config.VersionLimit,
			Visibility:   VisibilityUnknown,
			SourceURI:    metadataURL,
		},
		MetadataURL: metadataURL,
	}, true
}

func derivedUnsupportedTarget(scopeID string, config DerivedTargetConfig) (FetchTarget, bool) {
	for _, ecosystem := range []Ecosystem{
		EcosystemGoModule,
		EcosystemMaven,
		EcosystemNuGet,
		EcosystemComposer,
		EcosystemRubyGems,
		EcosystemCargo,
	} {
		if !derivedTargetEcosystemEnabled(config, ecosystem) {
			continue
		}
		identity, ok := normalizedDerivedIdentity(scopeID, ecosystem)
		if !ok {
			continue
		}
		return FetchTarget{
			Base: TargetConfig{
				Provider:     string(ecosystem),
				Ecosystem:    ecosystem,
				Registry:     identity.Registry,
				ScopeID:      identity.PackageID,
				Namespace:    identity.Namespace,
				Packages:     []string{identity.NormalizedName},
				PackageLimit: config.PackageLimit,
				VersionLimit: config.VersionLimit,
				Visibility:   VisibilityUnknown,
				SourceURI:    identity.PackageID,
			},
		}, true
	}
	return FetchTarget{}, false
}

func normalizedDerivedIdentity(scopeID string, ecosystem Ecosystem) (NormalizedPackageIdentity, bool) {
	parsed, err := url.Parse(scopeID)
	if err != nil || parsed.Scheme != string(ecosystem) {
		return NormalizedPackageIdentity{}, false
	}
	registry, rawPath, ok := derivedRegistryAndRawPath(scopeID, ecosystem)
	if !ok {
		return NormalizedPackageIdentity{}, false
	}
	rawName, namespace, ok := rawNameAndNamespaceFromDerivedPath(rawPath, ecosystem)
	if !ok {
		return NormalizedPackageIdentity{}, false
	}
	identity, err := NormalizePackageIdentity(PackageIdentity{
		Ecosystem: ecosystem,
		Registry:  registry,
		RawName:   rawName,
		Namespace: namespace,
	})
	if err != nil || identity.PackageID != scopeID {
		return NormalizedPackageIdentity{}, false
	}
	return identity, true
}

func derivedRegistryAndRawPath(scopeID string, ecosystem Ecosystem) (string, string, bool) {
	for _, candidate := range []struct {
		ecosystem Ecosystem
		prefix    string
		registry  string
	}{
		{EcosystemPyPI, "pypi://pypi.org/pypi/", "https://pypi.org/pypi"},
		{EcosystemGoModule, "gomod://proxy.golang.org/", "https://proxy.golang.org"},
		{EcosystemMaven, "maven://repo.maven.apache.org/maven2/", "https://repo.maven.apache.org/maven2"},
		{EcosystemNuGet, "nuget://api.nuget.org/v3/index.json/", "https://api.nuget.org/v3/index.json"},
		{EcosystemComposer, "composer://repo.packagist.org/", "https://repo.packagist.org"},
		{EcosystemRubyGems, "rubygems://rubygems.org/", "https://rubygems.org"},
		{EcosystemCargo, "cargo://crates.io/", "https://crates.io"},
	} {
		if candidate.ecosystem != ecosystem || !strings.HasPrefix(scopeID, candidate.prefix) {
			continue
		}
		rawPath, err := url.PathUnescape(strings.TrimPrefix(scopeID, candidate.prefix))
		if err != nil || strings.TrimSpace(rawPath) == "" {
			return "", "", false
		}
		return candidate.registry, rawPath, true
	}
	return "", "", false
}

func rawNameAndNamespaceFromDerivedPath(pathValue string, ecosystem Ecosystem) (string, string, bool) {
	if strings.TrimSpace(pathValue) == "" {
		return "", "", false
	}
	if ecosystem != EcosystemMaven {
		return pathValue, "", true
	}
	namespace, rawName, ok := strings.Cut(pathValue, ":")
	if !ok {
		return "", "", false
	}
	return strings.TrimSpace(rawName), strings.TrimSpace(namespace), true
}

func derivedTargetEcosystems(raw []Ecosystem) []Ecosystem {
	if len(raw) == 0 {
		return []Ecosystem{EcosystemNPM}
	}
	out := make([]Ecosystem, 0, len(raw))
	for _, ecosystem := range raw {
		out = append(out, NormalizeEcosystem(ecosystem))
	}
	return out
}

func derivedTargetEcosystemEnabled(config DerivedTargetConfig, ecosystem Ecosystem) bool {
	for _, candidate := range config.Ecosystems {
		if candidate == ecosystem {
			return true
		}
	}
	return false
}

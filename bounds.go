// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"fmt"
	"sort"
	"strings"
)

func boundedParsedMetadata(
	target TargetConfig,
	parsed ParsedMetadata,
) (ParsedMetadata, error) {
	configured, err := configuredPackageIDs(target)
	if err != nil {
		return ParsedMetadata{}, err
	}
	bounded, allowedPackages, err := boundPackages(target, parsed, configured)
	if err != nil {
		return ParsedMetadata{}, err
	}
	truncationWarnings, allowedVersions, err := boundVersions(target, parsed, allowedPackages, &bounded)
	if err != nil {
		return ParsedMetadata{}, err
	}
	bounded.Dependencies, err = boundPackageVersionObservations(
		parsed.Dependencies,
		func(observation PackageDependencyObservation) PackageIdentity {
			return observation.Package
		},
		func(observation PackageDependencyObservation) string {
			return observation.Version
		},
		allowedPackages,
		allowedVersions,
	)
	if err != nil {
		return ParsedMetadata{}, err
	}
	bounded.Artifacts, err = boundPackageVersionObservations(
		parsed.Artifacts,
		func(observation PackageArtifactObservation) PackageIdentity {
			return observation.Package
		},
		func(observation PackageArtifactObservation) string {
			return observation.Version
		},
		allowedPackages,
		allowedVersions,
	)
	if err != nil {
		return ParsedMetadata{}, err
	}
	bounded.SourceHints, err = boundPackageVersionObservations(
		parsed.SourceHints,
		func(observation SourceHintObservation) PackageIdentity {
			return observation.Package
		},
		func(observation SourceHintObservation) string {
			return observation.Version
		},
		allowedPackages,
		allowedVersions,
	)
	if err != nil {
		return ParsedMetadata{}, err
	}
	bounded.Vulnerables, err = boundPackageVersionObservations(
		parsed.Vulnerables,
		func(observation VulnerabilityHintObservation) PackageIdentity {
			return observation.Package
		},
		func(observation VulnerabilityHintObservation) string {
			return observation.Version
		},
		allowedPackages,
		allowedVersions,
	)
	if err != nil {
		return ParsedMetadata{}, err
	}
	bounded.Events, err = boundPackageVersionObservations(
		parsed.Events,
		func(observation RegistryEventObservation) PackageIdentity {
			return observation.Package
		},
		func(observation RegistryEventObservation) string {
			return observation.Version
		},
		allowedPackages,
		allowedVersions,
	)
	if err != nil {
		return ParsedMetadata{}, err
	}
	bounded.Warnings, err = boundWarnings(parsed.Warnings, allowedPackages, allowedVersions)
	if err != nil {
		return ParsedMetadata{}, err
	}
	bounded.Warnings = append(truncationWarnings, bounded.Warnings...)
	bounded.Hosting = parsed.Hosting
	return bounded, nil
}

func configuredPackageIDs(target TargetConfig) (map[string]struct{}, error) {
	configured := map[string]struct{}{}
	for _, rawPackage := range target.Packages {
		normalized, err := NormalizePackageIdentity(PackageIdentity{
			Ecosystem: target.Ecosystem,
			Registry:  target.Registry,
			RawName:   rawPackage,
			Namespace: target.Namespace,
		})
		if err != nil {
			return nil, fmt.Errorf("normalize configured package %q: %w", strings.TrimSpace(rawPackage), err)
		}
		configured[normalized.PackageID] = struct{}{}
	}
	return configured, nil
}

func boundPackages(
	target TargetConfig,
	parsed ParsedMetadata,
	configured map[string]struct{},
) (ParsedMetadata, map[string]struct{}, error) {
	bounded := ParsedMetadata{}
	allowed := map[string]struct{}{}
	for _, observation := range parsed.Packages {
		packageID, err := packageID(observation.Identity)
		if err != nil {
			return ParsedMetadata{}, nil, err
		}
		if len(configured) > 0 {
			if _, ok := configured[packageID]; !ok {
				continue
			}
		}
		bounded.Packages = append(bounded.Packages, observation)
		allowed[packageID] = struct{}{}
	}
	if len(parsed.Packages) > 0 && len(bounded.Packages) == 0 {
		return ParsedMetadata{}, nil,
			fmt.Errorf("package registry metadata does not match configured packages")
	}
	if len(bounded.Packages) > target.PackageLimit {
		return ParsedMetadata{}, nil,
			fmt.Errorf("package registry metadata emits %d packages, exceeds package_limit %d",
				len(bounded.Packages), target.PackageLimit)
	}
	return bounded, allowed, nil
}

func boundVersions(
	target TargetConfig,
	parsed ParsedMetadata,
	allowedPackages map[string]struct{},
	bounded *ParsedMetadata,
) ([]WarningObservation, map[string]struct{}, error) {
	allowedVersions := map[string]struct{}{}
	packageObservations, err := packageObservationsByID(parsed.Packages)
	if err != nil {
		return nil, nil, err
	}
	versionObservations := map[string]PackageVersionObservation{}
	versionCounts := map[string]int{}
	for _, observation := range parsed.Versions {
		packageID, err := packageID(observation.Package)
		if err != nil {
			return nil, nil, err
		}
		if !containsID(allowedPackages, packageID) {
			continue
		}
		if _, ok := versionObservations[packageID]; !ok {
			versionObservations[packageID] = observation
		}
		versionCounts[packageID]++
		if versionCounts[packageID] > target.VersionLimit {
			continue
		}
		bounded.Versions = append(bounded.Versions, observation)
		allowedVersions[versionKey(packageID, observation.Version)] = struct{}{}
	}
	warnings := versionLimitWarnings(target, allowedPackages, versionCounts, packageObservations, versionObservations)
	return warnings, allowedVersions, nil
}

func packageObservationsByID(
	observations []PackageObservation,
) (map[string]PackageObservation, error) {
	byID := map[string]PackageObservation{}
	for _, observation := range observations {
		packageID, err := packageID(observation.Identity)
		if err != nil {
			return nil, err
		}
		byID[packageID] = observation
	}
	return byID, nil
}

func versionLimitWarnings(
	target TargetConfig,
	allowedPackages map[string]struct{},
	versionCounts map[string]int,
	packageObservations map[string]PackageObservation,
	versionObservations map[string]PackageVersionObservation,
) []WarningObservation {
	warnings := make([]WarningObservation, 0)
	for _, packageID := range sortedIDs(versionCounts) {
		count := versionCounts[packageID]
		if !containsID(allowedPackages, packageID) || count <= target.VersionLimit {
			continue
		}
		packageObservation, hasPackageObservation := packageObservations[packageID]
		versionObservation := versionObservations[packageID]
		warningPackage := packageObservation.Identity
		if !hasPackageObservation {
			warningPackage = versionObservation.Package
		}
		warning := WarningObservation{
			WarningKey:          "version-limit:" + packageID,
			WarningCode:         "version_limit_truncated",
			Severity:            "warning",
			Message:             fmt.Sprintf("metadata emitted %d versions; kept first %d due to version_limit %d", count, target.VersionLimit, target.VersionLimit),
			Package:             &warningPackage,
			ScopeID:             packageObservation.ScopeID,
			GenerationID:        packageObservation.GenerationID,
			CollectorInstanceID: packageObservation.CollectorInstanceID,
			FencingToken:        packageObservation.FencingToken,
			ObservedAt:          packageObservation.ObservedAt,
			SourceURI:           packageObservation.SourceURI,
		}
		if warning.ScopeID == "" {
			warning.ScopeID = versionObservation.ScopeID
			warning.GenerationID = versionObservation.GenerationID
			warning.CollectorInstanceID = versionObservation.CollectorInstanceID
			warning.FencingToken = versionObservation.FencingToken
			warning.ObservedAt = versionObservation.ObservedAt
			warning.SourceURI = versionObservation.SourceURI
		}
		warnings = append(warnings, warning)
	}
	return warnings
}

func sortedIDs[T any](values map[string]T) []string {
	ids := make([]string, 0, len(values))
	for id := range values {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func boundPackageVersionObservations[T any](
	observations []T,
	identity func(T) PackageIdentity,
	version func(T) string,
	allowedPackages map[string]struct{},
	allowedVersions map[string]struct{},
) ([]T, error) {
	bounded := make([]T, 0, len(observations))
	for _, observation := range observations {
		if keep, err := keepPackageVersionObservation(
			identity(observation),
			version(observation),
			allowedPackages,
			allowedVersions,
		); err != nil {
			return nil, err
		} else if keep {
			bounded = append(bounded, observation)
		}
	}
	return bounded, nil
}

func boundWarnings(
	warnings []WarningObservation,
	allowedPackages map[string]struct{},
	allowedVersions map[string]struct{},
) ([]WarningObservation, error) {
	bounded := make([]WarningObservation, 0, len(warnings))
	for _, warning := range warnings {
		if warning.Package == nil {
			bounded = append(bounded, warning)
			continue
		}
		keep, err := keepPackageVersionObservation(*warning.Package, warning.Version, allowedPackages, allowedVersions)
		if err != nil {
			return nil, err
		}
		if keep {
			bounded = append(bounded, warning)
		}
	}
	return bounded, nil
}

func keepPackageVersionObservation(
	identity PackageIdentity,
	version string,
	allowedPackages map[string]struct{},
	allowedVersions map[string]struct{},
) (bool, error) {
	packageID, err := packageID(identity)
	if err != nil {
		return false, err
	}
	if !containsID(allowedPackages, packageID) {
		return false, nil
	}
	version = strings.TrimSpace(version)
	if version == "" {
		return true, nil
	}
	if len(allowedVersions) == 0 {
		return false, nil
	}
	return containsID(allowedVersions, versionKey(packageID, version)), nil
}

func packageID(identity PackageIdentity) (string, error) {
	normalized, err := NormalizePackageIdentity(identity)
	if err != nil {
		return "", err
	}
	return normalized.PackageID, nil
}

func containsID(values map[string]struct{}, value string) bool {
	_, ok := values[value]
	return ok
}

func versionKey(packageID, version string) string {
	return packageID + "\x00" + strings.TrimSpace(version)
}

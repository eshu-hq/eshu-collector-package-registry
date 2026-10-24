// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"strings"

	sdk "github.com/eshu-hq/eshu-sdk/collector"
)

const (
	warningCodeMetadataNotFound          = "registry_not_found"
	warningCodeMetadataTooLarge          = "metadata_too_large"
	warningCodeUnsupportedMetadataSource = "unsupported_metadata_source"
	warningCodeMalformedMetadata         = "malformed_metadata"
	warningCodeCredentialsMissing        = "credentials_missing"
)

// prefetchWarningFact short-circuits before any fetch: a private-visibility
// target without credentials yields credentials_missing; an ecosystem with no
// native metadata source and no explicit URL or format yields
// unsupported_metadata_source. It returns nil when the target may be fetched.
func prefetchWarningFact(target FetchTarget, ctx FactContext) (*sdk.Fact, error) {
	switch {
	case targetNeedsCredentials(target):
		fact, err := warningFact(target, ctx, warningCodeCredentialsMissing,
			"package registry metadata requires credentials that were not configured; package evidence remains missing")
		if err != nil {
			return nil, err
		}
		return &fact, nil
	case targetHasNoNativeMetadataSource(target):
		fact, err := warningFact(target, ctx, warningCodeUnsupportedMetadataSource,
			"package registry metadata source is not supported by a native adapter; package evidence remains missing")
		if err != nil {
			return nil, err
		}
		return &fact, nil
	default:
		return nil, nil
	}
}

// parserWarningFact maps a parse failure onto warning evidence when the
// failure class is a known warning code: unregistered ecosystems become
// unsupported_metadata_source, everything else malformed_metadata. It returns
// handled=false for failures the caller must surface as claim errors.
func parserWarningFact(target FetchTarget, ctx FactContext, parseErr error) (sdk.Fact, bool, error) {
	var fact sdk.Fact
	var err error
	switch parserWarningCode(parseErr) {
	case warningCodeUnsupportedMetadataSource:
		fact, err = warningFact(target, ctx, warningCodeUnsupportedMetadataSource,
			"package registry metadata source is not supported by a native adapter; package evidence remains missing")
	case warningCodeMalformedMetadata:
		fact, err = warningFact(target, ctx, warningCodeMalformedMetadata,
			"package registry metadata was malformed; package evidence remains missing")
	default:
		return sdk.Fact{}, false, nil
	}
	if err != nil {
		return sdk.Fact{}, false, err
	}
	return fact, true, nil
}

// missingMetadataWarningFact builds the derived-target not-found warning when
// the registry reports a missing document.
func missingMetadataWarningFact(target FetchTarget, ctx FactContext) (sdk.Fact, error) {
	return warningFact(target, ctx, warningCodeMetadataNotFound,
		"derived package registry metadata was not found; package evidence remains missing")
}

// metadataTooLargeWarningFact builds the terminal oversize warning: the claim
// completes with warning evidence instead of burning retries.
func metadataTooLargeWarningFact(target FetchTarget, ctx FactContext) (sdk.Fact, error) {
	return warningFact(target, ctx, warningCodeMetadataTooLarge,
		"package registry metadata exceeded the configured byte limit; package evidence remains unsupported")
}

func warningFact(target FetchTarget, ctx FactContext, warningCode, message string) (sdk.Fact, error) {
	return NewWarningEnvelope(WarningObservation{
		WarningKey:          warningCode,
		WarningCode:         warningCode,
		Severity:            "warning",
		Message:             message,
		Ecosystem:           target.Base.Ecosystem,
		Package:             targetWarningPackageIdentity(target, warningCode),
		ScopeID:             target.Base.ScopeID,
		GenerationID:        ctx.GenerationID,
		CollectorInstanceID: ctx.CollectorInstanceID,
		ObservedAt:          ctx.ObservedAt,
		SourceURI:           targetWarningSourceURI(target, warningCode),
	}, ctx.SourceSystem)
}

func targetNeedsCredentials(target FetchTarget) bool {
	return target.Base.Visibility == VisibilityPrivate &&
		strings.TrimSpace(target.MetadataURL) != "" &&
		strings.TrimSpace(target.BearerToken) == "" &&
		strings.TrimSpace(target.Username) == "" &&
		target.Password == ""
}

func targetHasNoNativeMetadataSource(target FetchTarget) bool {
	if strings.TrimSpace(target.MetadataURL) != "" ||
		strings.TrimSpace(target.DocumentFormat) == DocumentFormatArtifactoryPackage {
		return false
	}
	switch target.Base.Ecosystem {
	case EcosystemGoModule,
		EcosystemMaven,
		EcosystemNuGet,
		EcosystemComposer,
		EcosystemRubyGems,
		EcosystemCargo:
		return true
	default:
		return false
	}
}

func parserWarningCode(err error) string {
	message := err.Error()
	if strings.Contains(message, "not registered") {
		return warningCodeUnsupportedMetadataSource
	}
	return warningCodeMalformedMetadata
}

func targetWarningPackageIdentity(target FetchTarget, warningCode string) *PackageIdentity {
	if suppressPrivateTargetDetail(target, warningCode) {
		return nil
	}
	return targetPackageIdentity(target)
}

func targetWarningSourceURI(target FetchTarget, warningCode string) string {
	if suppressPrivateTargetDetail(target, warningCode) {
		return ""
	}
	return safeSourceURI(firstNonBlank(target.Base.SourceURI, target.MetadataURL))
}

func suppressPrivateTargetDetail(target FetchTarget, warningCode string) bool {
	return warningCode == warningCodeCredentialsMissing ||
		target.Base.Visibility == VisibilityPrivate
}

func targetPackageIdentity(target FetchTarget) *PackageIdentity {
	name := ""
	if len(target.Base.Packages) > 0 {
		name = strings.TrimSpace(target.Base.Packages[0])
	}
	if name == "" {
		return nil
	}
	identity := PackageIdentity{
		Ecosystem: target.Base.Ecosystem,
		Registry:  target.Base.Registry,
		Namespace: target.Base.Namespace,
		RawName:   name,
	}
	return &identity
}

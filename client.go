// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// DocumentFormatNative routes metadata directly to the ecosystem parser.
	DocumentFormatNative = "native"
	// DocumentFormatArtifactoryPackage routes metadata through the Artifactory
	// package wrapper before package-native parsing.
	DocumentFormatArtifactoryPackage = "artifactory_package"
)

const (
	maxMetadataDocumentBytes    = 20 << 20
	defaultMetadataFetchTimeout = 30 * time.Second

	npmMetadataAcceptHeader  = "application/vnd.npm.install-v1+json; q=1.0, application/json; q=0.8, */*;q=0.1"
	jsonMetadataAcceptHeader = "application/json, application/xml;q=0.5, text/xml;q=0.4, */*;q=0.1"
	xmlMetadataAcceptHeader  = "application/xml, text/xml;q=0.9, application/json;q=0.5, */*;q=0.1"

	failureClassMetadataTooLarge = "registry_metadata_too_large"
)

// Registry failure classes. The error text and details intentionally avoid
// registry hosts, repositories, package names, paths, tags, digests,
// accounts, and credential references so operators can diagnose failure class
// without leaking private registry material.
const (
	// RegistryFailureAuthDenied reports authentication or authorization denial.
	RegistryFailureAuthDenied = "registry_auth_denied"
	// RegistryFailureNotFound reports a missing registry repository, package,
	// or metadata endpoint.
	RegistryFailureNotFound = "registry_not_found"
	// RegistryFailureRateLimited reports registry throttling.
	RegistryFailureRateLimited = "registry_rate_limited"
	// RegistryFailureRetryable reports a transient registry or transport
	// failure.
	RegistryFailureRetryable = "registry_retryable_failure"
	// RegistryFailureCanceled reports collector cancellation before completion.
	RegistryFailureCanceled = "registry_canceled"
	// RegistryFailureTerminal reports a non-retryable registry failure class.
	RegistryFailureTerminal = "registry_terminal_failure"
)

// ErrRateLimited marks provider responses that should be visible as rate limit
// telemetry without leaking feed URLs or credentials.
var ErrRateLimited = errors.New("package registry metadata request rate limited")

// FetchTarget couples shared package-registry identity with runtime-only
// provider material such as metadata endpoints and resolved credentials.
type FetchTarget struct {
	Base           TargetConfig
	MetadataURL    string
	DocumentFormat string
	Username       string
	Password       string
	BearerToken    string
}

// DerivedTargetConfig enables bounded public package target resolution from a
// claim scope ID.
type DerivedTargetConfig struct {
	Enabled      bool
	Ecosystems   []Ecosystem
	PackageLimit int
	VersionLimit int
}

// MetadataDocument is the parser input returned by a registry client.
type MetadataDocument struct {
	Body         []byte
	SourceURI    string
	DocumentType string
	ObservedAt   time.Time
}

// MetadataProvider fetches one bounded package metadata document for a target.
type MetadataProvider interface {
	FetchMetadata(context.Context, FetchTarget) (MetadataDocument, error)
}

// RegistryFailure carries bounded registry failure metadata into result
// statuses. Error text and details avoid hosts, packages, and credentials.
type RegistryFailure struct {
	Class      string
	Operation  string
	StatusCode int
	RetryAfter time.Duration
	Cause      error
}

// Error returns the bounded operator-facing message.
func (e RegistryFailure) Error() string {
	if class := strings.TrimSpace(e.Class); class != "" {
		return "registry collector failed: " + class
	}
	return "registry collector failed"
}

// Unwrap returns the underlying cause for errors.Is and errors.As checks.
func (e RegistryFailure) Unwrap() error {
	return e.Cause
}

// FailureClass returns the bounded status failure class.
func (e RegistryFailure) FailureClass() string {
	return strings.TrimSpace(e.Class)
}

// TerminalFailure reports whether retrying cannot help.
func (e RegistryFailure) TerminalFailure() bool {
	switch strings.TrimSpace(e.Class) {
	case RegistryFailureTerminal, RegistryFailureAuthDenied:
		return true
	default:
		return false
	}
}

// RetryAfterDelay returns the server-advised wait before a retry.
func (e RegistryFailure) RetryAfterDelay() time.Duration {
	return e.RetryAfter
}

// FailureStatusCode returns the HTTP status that produced the failure, if any.
func (e RegistryFailure) FailureStatusCode() int {
	return e.StatusCode
}

type metadataTooLargeError struct {
	limitBytes int
}

func newMetadataTooLargeError(limitBytes int) error {
	return metadataTooLargeError{limitBytes: limitBytes}
}

func (e metadataTooLargeError) Error() string {
	return fmt.Sprintf("package registry metadata exceeds configured byte limit %d bytes", e.limitBytes)
}

// FailureClass marks the oversize outcome terminal at the source: the claim
// completes with metadata_too_large warning evidence instead of retrying.
func (e metadataTooLargeError) FailureClass() string {
	return failureClassMetadataTooLarge
}

func (e metadataTooLargeError) TerminalFailure() bool {
	return true
}

func isMetadataTooLarge(err error) bool {
	var tooLarge metadataTooLargeError
	return errors.As(err, &tooLarge)
}

func isRegistryNotFound(err error) bool {
	var failure interface{ FailureClass() string }
	return errors.As(err, &failure) &&
		strings.TrimSpace(failure.FailureClass()) == RegistryFailureNotFound
}

// HTTPMetadataProvider fetches parser-ready package metadata from an explicit
// package feed endpoint.
type HTTPMetadataProvider struct {
	Client *http.Client
}

// FetchMetadata retrieves a bounded metadata document and never includes
// credentials in returned errors.
func (p HTTPMetadataProvider) FetchMetadata(ctx context.Context, target FetchTarget) (MetadataDocument, error) {
	metadataURL := strings.TrimSpace(target.MetadataURL)
	if metadataURL == "" {
		return MetadataDocument{}, fmt.Errorf("metadata_url is required")
	}
	client := p.httpClient()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, metadataURL, nil)
	if err != nil {
		return MetadataDocument{}, fmt.Errorf("build package metadata request: %w", err)
	}
	request.Header.Set("Accept", metadataAcceptHeader(target.Base.Ecosystem))
	if strings.TrimSpace(target.BearerToken) != "" {
		request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(target.BearerToken))
	} else if strings.TrimSpace(target.Username) != "" || target.Password != "" {
		request.SetBasicAuth(strings.TrimSpace(target.Username), target.Password)
	}

	startedAt := time.Now()
	response, err := client.Do(request)
	if err != nil {
		class := RegistryFailureRetryable
		if errors.Is(err, context.Canceled) {
			class = RegistryFailureCanceled
		}
		return MetadataDocument{}, RegistryFailure{
			Class: class, Operation: "fetch_metadata", Cause: err,
		}
	}
	defer func() {
		// The body is consumed end-to-end before Close; a close error
		// carries no recovery action, so it is checked here and dropped.
		if cerr := response.Body.Close(); cerr != nil {
			return
		}
	}()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		var cause error
		if response.StatusCode == http.StatusTooManyRequests {
			cause = ErrRateLimited
		}
		return MetadataDocument{}, RegistryFailure{
			Class:      registryFailureClassForHTTPStatus(response.StatusCode),
			Operation:  "fetch_metadata",
			StatusCode: response.StatusCode,
			RetryAfter: parseRetryAfterHeader(response.Header.Get("Retry-After")),
			Cause:      cause,
		}
	}
	body, err := readBoundedMetadata(response.Body)
	if err != nil {
		return MetadataDocument{}, err
	}
	return MetadataDocument{
		Body:         body,
		SourceURI:    safeSourceURI(metadataURL),
		DocumentType: string(target.Base.Ecosystem),
		ObservedAt:   startedAt.UTC(),
	}, nil
}

func (p HTTPMetadataProvider) httpClient() *http.Client {
	if p.Client != nil {
		return p.Client
	}
	return &http.Client{Timeout: defaultMetadataFetchTimeout}
}

// registryFailureClassForHTTPStatus maps HTTP status codes to stable registry
// failure classes.
func registryFailureClassForHTTPStatus(statusCode int) string {
	switch {
	case statusCode == 401 || statusCode == 403:
		return RegistryFailureAuthDenied
	case statusCode == 404:
		return RegistryFailureNotFound
	case statusCode == 429:
		return RegistryFailureRateLimited
	case statusCode == 408 || statusCode >= 500:
		return RegistryFailureRetryable
	default:
		return RegistryFailureTerminal
	}
}

func metadataAcceptHeader(ecosystem Ecosystem) string {
	switch ecosystem {
	case EcosystemNPM:
		return npmMetadataAcceptHeader
	case EcosystemMaven, EcosystemNuGet:
		return xmlMetadataAcceptHeader
	default:
		return jsonMetadataAcceptHeader
	}
}

func readBoundedMetadata(reader io.Reader) ([]byte, error) {
	limited := io.LimitReader(reader, maxMetadataDocumentBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read package metadata: %w", err)
	}
	if len(body) > maxMetadataDocumentBytes {
		return nil, newMetadataTooLargeError(maxMetadataDocumentBytes)
	}
	return body, nil
}

// parseRetryAfterHeader parses a Retry-After header into a wait duration: a
// non-negative delay in seconds, or an HTTP date in the future. Anything else
// (including a past date) reports zero.
func parseRetryAfterHeader(raw string) time.Duration {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(trimmed); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(trimmed); err == nil {
		if delay := time.Until(at); delay > 0 {
			return delay
		}
	}
	return 0
}

// safeSourceURI strips credentials, query, and fragment from a source URI so
// it can be emitted in a fact. A purl keeps flowing (its "@" sits after the
// first "/"); an authority-shaped credential drops the value to empty.
func safeSourceURI(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" {
		return ""
	}
	if parsed.Host == "" && carriesUserinfo(trimmed) {
		// Not hierarchical, so User can be nil with a credential in plain
		// sight (`svc:SECRET@host/x` keeps it in Opaque, which String()
		// round-trips verbatim). A purl's "@" sits after the first "/", so
		// purls keep flowing; only an authority-shaped credential drops.
		return ""
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	return parsed.String()
}

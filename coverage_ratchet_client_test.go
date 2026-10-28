// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestRatchetFetchMetadataSuccess pins the client happy path: the body is
// returned verbatim, credentials never leak into the returned document, and
// the source URI is stripped to host plus path.
func TestRatchetFetchMetadataSuccess(t *testing.T) {
	t.Parallel()

	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if got := r.Header.Get("Accept"); !strings.Contains(got, "application/json") {
			t.Errorf("Accept = %q, want JSON metadata accept", got)
		}
		w.Header().Set("Content-Type", "application/json")
		if _, writeErr := w.Write([]byte(`{"name":"left-pad"}`)); writeErr != nil {
			t.Errorf("Write() error = %v", writeErr)
		}
	}))
	defer server.Close()

	provider := HTTPMetadataProvider{Client: server.Client()}
	document, err := provider.FetchMetadata(context.Background(), FetchTarget{
		Base:        TargetConfig{Ecosystem: EcosystemNPM},
		MetadataURL: server.URL + "/left-pad?token=secret#frag",
		BearerToken: "ratchet-token",
	})
	if err != nil {
		t.Fatalf("FetchMetadata() error = %v", err)
	}
	if string(document.Body) != `{"name":"left-pad"}` {
		t.Fatalf("Body = %q, want fixture bytes", document.Body)
	}
	if strings.Contains(document.SourceURI, "token=secret") || strings.Contains(document.SourceURI, "frag") {
		t.Fatalf("SourceURI = %q, want query and fragment stripped", document.SourceURI)
	}
	if document.DocumentType != string(EcosystemNPM) {
		t.Fatalf("DocumentType = %q, want %q", document.DocumentType, EcosystemNPM)
	}
	if document.ObservedAt.IsZero() {
		t.Fatal("ObservedAt must be set")
	}
	if gotAuth != "Bearer ratchet-token" {
		t.Fatalf("Authorization = %q, want bearer token", gotAuth)
	}
}

// TestRatchetFetchMetadataRejectsBlankURL pins the fail-closed fetch guard.
func TestRatchetFetchMetadataRejectsBlankURL(t *testing.T) {
	t.Parallel()

	provider := HTTPMetadataProvider{}
	if _, err := provider.FetchMetadata(context.Background(), FetchTarget{}); err == nil {
		t.Fatal("FetchMetadata(blank URL) error = nil, want rejection")
	} else if !strings.Contains(err.Error(), "metadata_url") {
		t.Fatalf("FetchMetadata() error = %q, want the metadata_url branch", err)
	}
}

// TestRatchetFetchMetadataStatusClasses pins the HTTP status to failure-class
// mapping, including Retry-After propagation and terminal bits.
func TestRatchetFetchMetadataStatusClasses(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		status     int
		retryAfter string
		class      string
		terminal   bool
	}{
		{name: "auth_denied", status: http.StatusUnauthorized, class: RegistryFailureAuthDenied, terminal: true},
		{name: "forbidden", status: http.StatusForbidden, class: RegistryFailureAuthDenied, terminal: true},
		{name: "not_found", status: http.StatusNotFound, class: RegistryFailureNotFound, terminal: false},
		{name: "rate_limited", status: http.StatusTooManyRequests, retryAfter: "2", class: RegistryFailureRateLimited, terminal: false},
		{name: "retryable", status: http.StatusBadGateway, class: RegistryFailureRetryable, terminal: false},
		{name: "terminal", status: http.StatusTeapot, class: RegistryFailureTerminal, terminal: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tc.retryAfter != "" {
					w.Header().Set("Retry-After", tc.retryAfter)
				}
				w.WriteHeader(tc.status)
			}))
			defer server.Close()

			provider := HTTPMetadataProvider{Client: server.Client()}
			_, err := provider.FetchMetadata(context.Background(), FetchTarget{
				Base:        TargetConfig{Ecosystem: EcosystemNPM},
				MetadataURL: server.URL + "/pkg",
			})
			if err == nil {
				t.Fatalf("FetchMetadata(%d) error = nil, want %q", tc.status, tc.class)
			}
			var failure RegistryFailure
			if !errors.As(err, &failure) {
				t.Fatalf("FetchMetadata() error type = %T, want RegistryFailure", err)
			}
			if failure.FailureClass() != tc.class {
				t.Fatalf("FailureClass() = %q, want %q", failure.FailureClass(), tc.class)
			}
			if failure.FailureStatusCode() != tc.status {
				t.Fatalf("FailureStatusCode() = %d, want %d", failure.FailureStatusCode(), tc.status)
			}
			if failure.TerminalFailure() != tc.terminal {
				t.Fatalf("TerminalFailure() = %v, want %v", failure.TerminalFailure(), tc.terminal)
			}
			if tc.retryAfter != "" && failure.RetryAfterDelay() != 2*time.Second {
				t.Fatalf("RetryAfterDelay() = %v, want 2s", failure.RetryAfterDelay())
			}
			if failure.Error() == "" || !strings.Contains(failure.Error(), tc.class) {
				t.Fatalf("Error() = %q, want class text", failure.Error())
			}
			if tc.name == "rate_limited" && !errors.Is(err, ErrRateLimited) {
				t.Fatal("429 must carry ErrRateLimited for telemetry")
			}
		})
	}
}

// TestRatchetFetchMetadataTransportFailures pins transport error classes: a
// canceled context reports registry_canceled while other transport errors
// stay retryable, and a malformed URL fails at request build time.
func TestRatchetFetchMetadataTransportFailures(t *testing.T) {
	t.Parallel()

	provider := HTTPMetadataProvider{}
	if _, err := provider.FetchMetadata(context.Background(), FetchTarget{MetadataURL: "http://ex ample.invalid/x"}); err == nil {
		t.Fatal("FetchMetadata(bad URL) error = nil, want build error")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := provider.FetchMetadata(ctx, FetchTarget{MetadataURL: "https://registry.example/pkg"}); err == nil {
		t.Fatal("FetchMetadata(canceled) error = nil, want registry_canceled")
	} else {
		var failure RegistryFailure
		if !errors.As(err, &failure) || failure.FailureClass() != RegistryFailureCanceled {
			t.Fatalf("FetchMetadata(canceled) class = %v, want %q", err, RegistryFailureCanceled)
		}
	}

	closed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	closedURL := closed.URL
	closed.Close()
	if _, err := provider.FetchMetadata(context.Background(), FetchTarget{MetadataURL: closedURL}); err == nil {
		t.Fatal("FetchMetadata(transport) error = nil, want registry_retryable_failure")
	} else {
		var failure RegistryFailure
		if !errors.As(err, &failure) || failure.FailureClass() != RegistryFailureRetryable {
			t.Fatalf("FetchMetadata(transport) class = %v, want %q", err, RegistryFailureRetryable)
		}
	}
}

// TestRatchetClientPureHelpers pins the branchy client helpers without a
// network round trip.
func TestRatchetClientPureHelpers(t *testing.T) {
	t.Parallel()

	statusCases := map[int]string{
		401: RegistryFailureAuthDenied,
		403: RegistryFailureAuthDenied,
		404: RegistryFailureNotFound,
		429: RegistryFailureRateLimited,
		408: RegistryFailureRetryable,
		500: RegistryFailureRetryable,
		418: RegistryFailureTerminal,
	}
	for status, want := range statusCases {
		if got := registryFailureClassForHTTPStatus(status); got != want {
			t.Fatalf("registryFailureClassForHTTPStatus(%d) = %q, want %q", status, got, want)
		}
	}

	acceptCases := map[Ecosystem]string{
		EcosystemNPM:     npmMetadataAcceptHeader,
		EcosystemMaven:   xmlMetadataAcceptHeader,
		EcosystemNuGet:   xmlMetadataAcceptHeader,
		EcosystemPyPI:    jsonMetadataAcceptHeader,
		EcosystemGeneric: jsonMetadataAcceptHeader,
	}
	for ecosystem, want := range acceptCases {
		if got := metadataAcceptHeader(ecosystem); got != want {
			t.Fatalf("metadataAcceptHeader(%q) = %q, want %q", ecosystem, got, want)
		}
	}

	if got := parseRetryAfterHeader(""); got != 0 {
		t.Fatalf("parseRetryAfterHeader(blank) = %v, want 0", got)
	}
	if got := parseRetryAfterHeader("3"); got != 3*time.Second {
		t.Fatalf("parseRetryAfterHeader(3) = %v, want 3s", got)
	}
	if got := parseRetryAfterHeader("bogus"); got != 0 {
		t.Fatalf("parseRetryAfterHeader(bogus) = %v, want 0", got)
	}
	if got := parseRetryAfterHeader("-5"); got != 0 {
		t.Fatalf("parseRetryAfterHeader(negative) = %v, want 0", got)
	}
	future := time.Now().Add(time.Hour).UTC().Format(http.TimeFormat)
	if got := parseRetryAfterHeader(future); got <= 0 {
		t.Fatalf("parseRetryAfterHeader(future date) = %v, want positive", got)
	}
	past := time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat)
	if got := parseRetryAfterHeader(past); got != 0 {
		t.Fatalf("parseRetryAfterHeader(past date) = %v, want 0", got)
	}

	body, err := readBoundedMetadata(bytes.NewReader([]byte(`{"ok":true}`)))
	if err != nil || string(body) != `{"ok":true}` {
		t.Fatalf("readBoundedMetadata() = %q, %v, want fixture bytes", body, err)
	}
	if _, err := readBoundedMetadata(failingReader{}); err == nil {
		t.Fatal("readBoundedMetadata(failing) error = nil, want read error")
	}
	oversize := make([]byte, maxMetadataDocumentBytes+1)
	if _, err := readBoundedMetadata(bytes.NewReader(oversize)); err == nil {
		t.Fatal("readBoundedMetadata(oversize) error = nil, want metadata_too_large")
	} else if !isMetadataTooLarge(err) {
		t.Fatalf("isMetadataTooLarge() = false, want true for %v", err)
	} else {
		var tooLarge metadataTooLargeError
		if !errors.As(err, &tooLarge) {
			t.Fatalf("oversize error type = %T, want metadataTooLargeError", err)
		}
		if tooLarge.FailureClass() != failureClassMetadataTooLarge || !tooLarge.TerminalFailure() {
			t.Fatalf("oversize failure = %q/%v, want terminal %q", tooLarge.FailureClass(), tooLarge.TerminalFailure(), failureClassMetadataTooLarge)
		}
		if !strings.Contains(tooLarge.Error(), "byte limit") {
			t.Fatalf("oversize Error() = %q, want byte limit text", tooLarge.Error())
		}
	}

	nested := RegistryFailure{
		Class: " " + RegistryFailureNotFound + " ", Operation: "fetch_metadata", StatusCode: 404,
		Cause: errors.New("missing"),
	}
	if nested.FailureClass() != RegistryFailureNotFound {
		t.Fatalf("FailureClass() = %q, want trimmed not_found", nested.FailureClass())
	}
	if !isRegistryNotFound(nested) {
		t.Fatal("isRegistryNotFound() = false, want true")
	}
	other := RegistryFailure{Class: RegistryFailureRetryable, Operation: "fetch_metadata"}
	if isRegistryNotFound(other) {
		t.Fatal("isRegistryNotFound(retryable) = true, want false")
	}
	if got := (RegistryFailure{Class: "  "}).Error(); got != "registry collector failed" {
		t.Fatalf("blank-class Error() = %q, want bare failure text", got)
	}
	if err := (RegistryFailure{Cause: errors.New("boom")}).Unwrap(); err == nil || err.Error() != "boom" {
		t.Fatalf("Unwrap() = %v, want cause", err)
	}
	tooLargeDirect := metadataTooLargeError{limitBytes: 8}
	if newMetadataTooLargeError(8).Error() == "" || tooLargeDirect.FailureClass() == "" {
		t.Fatal("metadataTooLargeError must carry message and class")
	}
	if (metadataTooLargeError{}).TerminalFailure() != true {
		t.Fatal("oversize must be terminal")
	}
	plain := HTTPMetadataProvider{}
	if plain.httpClient() == nil {
		t.Fatal("httpClient() must default to a usable client")
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("ratchet read failure") }

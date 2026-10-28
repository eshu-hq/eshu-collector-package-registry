// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// errReadBody fails every read but closes cleanly. It proves a transport
// read error surfaces as a bounded fetch error.
type errReadBody struct{}

func (errReadBody) Read([]byte) (int, error) { return 0, errors.New("boom") }
func (errReadBody) Close() error             { return nil }

// errCloseBody reads cleanly but fails the close. It proves a close error
// carries no recovery action: the document still completes.
type errCloseBody struct {
	*bytes.Buffer
}

func (errCloseBody) Close() error { return errors.New("boom") }

// roundTripFunc adapts a function to http.RoundTripper for fault injection
// without a server.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

// TestRatchetFetchMetadataBasicAuth pins the credential branch: a configured
// username authenticates with basic auth when no bearer token is set.
func TestRatchetFetchMetadataBasicAuth(t *testing.T) {
	t.Parallel()

	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if _, writeErr := w.Write([]byte(`{}`)); writeErr != nil {
			t.Errorf("Write() error = %v", writeErr)
		}
	}))
	defer server.Close()

	provider := HTTPMetadataProvider{Client: server.Client()}
	document, err := provider.FetchMetadata(context.Background(), FetchTarget{
		Base:        TargetConfig{Ecosystem: EcosystemNPM},
		MetadataURL: server.URL + "/left-pad",
		Username:    "user",
		Password:    "pass",
	})
	if err != nil {
		t.Fatalf("FetchMetadata() error = %v", err)
	}
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("user:pass"))
	if gotAuth != want {
		t.Fatalf("Authorization = %q, want basic auth", gotAuth)
	}
	if len(document.Body) == 0 {
		t.Fatal("body must still be returned")
	}
}

// TestRatchetFetchMetadataBodyErrors pins the body guards: a read error
// fails the fetch, while a close error is checked and dropped.
func TestRatchetFetchMetadataBodyErrors(t *testing.T) {
	t.Parallel()

	readErr := HTTPMetadataProvider{Client: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: errReadBody{}, Header: http.Header{}}, nil
	})}}
	if _, err := readErr.FetchMetadata(context.Background(), FetchTarget{MetadataURL: "https://registry.invalid/x"}); err == nil {
		t.Fatal("FetchMetadata(read error) error = nil, want rejection")
	} else if !strings.Contains(err.Error(), "read package metadata") {
		t.Fatalf("error = %q, want the read branch", err)
	}

	closeErr := HTTPMetadataProvider{Client: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       errCloseBody{Buffer: bytes.NewBufferString(`{}`)},
			Header:     http.Header{},
		}, nil
	})}}
	document, err := closeErr.FetchMetadata(context.Background(), FetchTarget{MetadataURL: "https://registry.invalid/x"})
	if err != nil {
		t.Fatalf("FetchMetadata(close error) error = %v, want the document anyway", err)
	}
	if string(document.Body) != `{}` {
		t.Fatalf("Body = %q", document.Body)
	}
}

// TestRatchetSafeSourceURIFailClosed pins the URI guards: an untakable value
// and an authority-shaped credential both drop to blank, while a purl keeps
// flowing because its @ sits after the first slash.
func TestRatchetSafeSourceURIFailClosed(t *testing.T) {
	t.Parallel()

	if got := safeSourceURI("http://[::1"); got != "" {
		t.Fatalf("unparseable = %q, want blank", got)
	}
	if got := safeSourceURI("svc:SECRET@host/x"); got != "" {
		t.Fatalf("opaque credential = %q, want blank", got)
	}
	if got := safeSourceURI("pkg:npm/left-pad@1.3.0"); got == "" {
		t.Fatal("purl must keep flowing")
	}
	if got := safeSourceURI("https://user:pass@example.invalid/x?token=abc#frag"); strings.Contains(got, "pass") || strings.Contains(got, "token") {
		t.Fatalf("hierarchical = %q, want credentials, query, and fragment stripped", got)
	}
}

// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	collector "github.com/eshu-hq/eshu-collector-package-registry"
	sdk "github.com/eshu-hq/eshu-sdk/collector"
)

func claim() sdk.Claim {
	now := time.Now().UTC()
	return sdk.Claim{
		ComponentID: collector.ComponentID, InstanceID: "package-registry-qa",
		CollectorKind: "package_registry", SourceSystem: "package_registry",
		Scope:       sdk.Scope{ID: "npm://registry.npmjs.org/left-pad", Kind: "package_registry"},
		SourceRunID: "run-1", GenerationID: "generation-1",
		WorkItemID: "work-1", FencingToken: "7", Attempt: 1,
		Deadline: now.Add(time.Hour), ConfigHandle: "component-config:package-registry",
	}
}

// fakeRegistry serves one npm packument over real HTTP.
func fakeRegistry(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if !strings.HasSuffix(r.URL.Path, "/left-pad") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if got := r.Header.Get("Accept"); !strings.Contains(got, "application/json") {
			w.WriteHeader(http.StatusNotAcceptable)
			return
		}
		if _, err := io.WriteString(w, `{"name": "left-pad", "versions": {"1.3.0": {"dist": {"tarball": "https://registry.npmjs.org/left-pad/-/left-pad-1.3.0.tgz"}}}}`); err != nil {
			t.Errorf("io.WriteString() error = %v", err)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func requestFor(t *testing.T, claimScope, configScope, metadataURL string) string {
	t.Helper()
	raw, err := json.Marshal(collector.Request{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1, Claim: claimWithScope(t, claimScope), Contract: collector.Contract(),
		Config: map[string]any{
			"targets": []any{map[string]any{
				"provider": "npm", "ecosystem": "npm", "registry": "https://registry.npmjs.org",
				"scope_id": configScope, "packages": []any{"left-pad"},
				"metadata_url": metadataURL,
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func claimWithScope(t *testing.T, scopeID string) sdk.Claim {
	t.Helper()
	c := claim()
	c.Scope.ID = scopeID
	return c
}

func getenv(key string) string {
	return map[string]string{}[key]
}

// TestRunClaimWritesOneValidResultToStdout serves one claim against the fake
// registry and asserts stdout carries exactly one valid complete SDK result.
func TestRunClaimWritesOneValidResultToStdout(t *testing.T) {
	t.Parallel()

	server := fakeRegistry(t)
	scopeID := "npm://registry.npmjs.org/left-pad"
	for _, args := range [][]string{nil, {"--sdk-stdio"}} {
		var stdout, stderr bytes.Buffer
		err := run(context.Background(), args, strings.NewReader(requestFor(t, scopeID, scopeID, server.URL+"/left-pad")), &stdout, &stderr, getenv)
		if err != nil {
			t.Fatalf("run(%v) error = %v", args, err)
		}
		decoder := json.NewDecoder(&stdout)
		decoder.DisallowUnknownFields()
		var result sdk.Result
		if err := decoder.Decode(&result); err != nil {
			t.Fatalf("stdout is not one SDK result: %v", err)
		}
		if decoder.More() {
			t.Fatal("stdout carries more than one JSON value")
		}
		if result.State != sdk.ResultComplete {
			t.Fatalf("State=%q, want complete", result.State)
		}
		if len(result.Facts) != 3 {
			t.Fatalf("facts=%d, want 3 (one package, one version, one artifact)", len(result.Facts))
		}
		if _, err := sdk.NewValidator(collector.Contract()).ValidateResult(result); err != nil {
			t.Fatalf("ValidateResult() error = %v", err)
		}
	}
}

// TestRunClaimRejectsUnknownScope fails terminal with invalid_config when the
// claim names a scope the activation does not configure.
func TestRunClaimRejectsUnknownScope(t *testing.T) {
	t.Parallel()

	server := fakeRegistry(t)
	var stdout, stderr bytes.Buffer
	err := run(context.Background(), nil,
		strings.NewReader(requestFor(t, "npm://registry.npmjs.org/unknown", "npm://registry.npmjs.org/left-pad", server.URL+"/left-pad")),
		&stdout, &stderr, getenv)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	var result sdk.Result
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &result); err != nil {
		t.Fatalf("stdout is not one SDK result: %v", err)
	}
	if result.State != sdk.ResultTerminal {
		t.Fatalf("State=%q, want terminal", result.State)
	}
	if len(result.Facts) != 0 {
		t.Fatalf("facts=%d, want 0 with a failure state", len(result.Facts))
	}
}

// TestSeedWritesBinaryAndManifest runs the seed subcommand into a temp dir
// and asserts the binary executes and the manifest matches the embedded one.
func TestSeedWritesBinaryAndManifest(t *testing.T) {
	t.Parallel()

	dest := t.TempDir()
	var stdout bytes.Buffer
	if err := run(context.Background(), []string{"seed", "--dest", dest}, strings.NewReader(""), &stdout, io.Discard, getenv); err != nil {
		t.Fatalf("run(seed) error = %v", err)
	}
	binary := filepath.Join(dest, "collector")
	info, err := os.Stat(binary)
	if err != nil {
		t.Fatalf("seeded binary missing: %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("seeded binary %s is not executable", binary)
	}
	manifest, err := os.ReadFile(filepath.Join(dest, "manifest.yaml"))
	if err != nil {
		t.Fatalf("seeded manifest missing: %v", err)
	}
	if !bytes.Equal(manifest, collector.ManifestYAML) {
		t.Fatal("seeded manifest differs from the embedded manifest")
	}
}

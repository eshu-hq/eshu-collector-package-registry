// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// credentialSentinel is planted inside credential positions below and must
// never survive into a sanitized value or a persisted fact.
const credentialSentinel = "EMBEDDED-CRED-SENTINEL-6119"

// TestSanitizeURLDropsOpaqueCredentials sweeps the sentinel through the
// userinfo spellings url.Parse does NOT surface as User: an opaque body
// (`svc:SECRET@host/x` — an operator omitting "https://" produces the same
// shape), a scheme-relative authority, and values net/url cannot parse at
// all. The old shape returned those verbatim because a scheme/host guard read
// "not hierarchical" as "nothing to redact".
//
// The two wantKept rows are positive controls: a path-segment "@" and an
// opaque tool name are accepted on purpose, so the sweep cannot pass by
// returning "" for everything.
func TestSanitizeURLDropsOpaqueCredentials(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    string
		wantKept bool
	}{
		{name: "opaque password position", value: "svc:" + credentialSentinel + "@h.internal:5432/tool"},
		{name: "opaque mid-password", value: "svc:pw" + credentialSentinel + "@h.internal/tool"},
		{name: "opaque under an uppercase scheme", value: "SVC:" + credentialSentinel + "@h.internal/tool"},
		{name: "scheme-relative userinfo", value: "//svc:" + credentialSentinel + "@h.internal/tool"},
		{name: "unparseable value", value: "https://svc:pw\"" + credentialSentinel + "@h.internal/tool"},
		{name: "unparseable only as an authority", value: "svc:pw]" + credentialSentinel + "@h.internal/tool"},
		{name: "scp-style git ref is credential-shaped", value: "git@github.example:" + credentialSentinel + "/repo"},
		{name: "hierarchical password stays redacted", value: "https://svc:" + credentialSentinel + "@h.internal/tool"},
		{name: "path-segment at sign survives (positive control)", value: "https://h.internal/owners/" + credentialSentinel + "@example.com/x", wantKept: true},
		{name: "opaque tool name survives (positive control)", value: "mcp:tool/" + credentialSentinel, wantKept: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := sanitizeURL(tt.value)
			if tt.wantKept {
				if !strings.Contains(got, credentialSentinel) {
					t.Fatalf("sanitizeURL(%q) = %q dropped a value that carries no credential; the sweep is over-redacting", tt.value, got)
				}
				return
			}
			if strings.Contains(got, credentialSentinel) {
				t.Errorf("sanitizeURL(%q) = %q kept the credential sentinel", tt.value, got)
			}
		})
	}
}

// TestPackageArtifactFactNeverPersistsOpaqueCredential drives the real fact
// constructor with a credentialed opaque SourceURI and ArtifactURL and asserts
// the sentinel is absent from the marshaled fact — the bytes that persist.
// The second run is the positive control: a path-segment sentinel must reach
// the same bytes, so the absence assertion is reading a real fact.
func TestPackageArtifactFactNeverPersistsOpaqueCredential(t *testing.T) {
	t.Parallel()

	build := func(t *testing.T, uri string) string {
		t.Helper()
		fact, err := NewPackageArtifactEnvelope(PackageArtifactObservation{
			Package: PackageIdentity{
				Ecosystem: EcosystemMaven,
				Registry:  "https://repo.maven.apache.org/maven2/",
				Namespace: "org.example",
				RawName:   "core-api",
			},
			Version:             "1.2.3",
			ArtifactKey:         "org/example/core-api/1.2.3/core-api-1.2.3.jar",
			ArtifactType:        "jar",
			ArtifactURL:         uri,
			ScopeID:             "maven://repo.maven.apache.org/maven2/org.example:core-api",
			GenerationID:        "sha256:artifact",
			CollectorInstanceID: "central",
			FencingToken:        10,
			ObservedAt:          time.Date(2026, 5, 12, 11, 5, 0, 0, time.UTC),
			SourceURI:           uri,
		}, paritySourceSystem)
		if err != nil {
			t.Fatalf("NewPackageArtifactEnvelope() error = %v, want nil", err)
		}
		raw, err := json.Marshal(fact)
		if err != nil {
			t.Fatalf("marshal fact: %v", err)
		}
		return string(raw)
	}

	persisted := build(t, "svc:"+credentialSentinel+"@h.internal:5432/maven2/")
	if strings.Contains(persisted, credentialSentinel) {
		t.Errorf("credential sentinel reached the persisted fact:\n%s", persisted)
	}

	control := build(t, "https://h.internal/owners/"+credentialSentinel+"@example.com/maven2/")
	if !strings.Contains(control, credentialSentinel) {
		t.Fatalf("positive control: path-segment sentinel missing from the marshaled fact, so the absence assertion above is blind:\n%s", control)
	}
}

// TestCollectorErrorsNeverCarryCredentials pins the leak probe the credential
// matrix requires: fetch failures and config rejections name classes and
// keys, never hosts, URLs, or resolved secrets.
func TestCollectorErrorsNeverCarryCredentials(t *testing.T) {
	t.Parallel()

	const secret = "SUPER-SECRET-TOKEN-6119"
	getenv := func(key string) string {
		if key == "PKGREG_TOKEN" {
			return secret
		}
		return ""
	}

	_, err := ParseSettings(map[string]any{
		"targets": []any{map[string]any{
			"provider":         "npm",
			"ecosystem":        "npm",
			"registry":         "https://registry.npmjs.org",
			"scope_id":         "npm://registry.npmjs.org/left-pad",
			"metadata_url":     "https://registry.npmjs.org/left-pad",
			"bearer_token_env": "PKGREG_TOKEN",
		}},
	}, "collector-1", getenv)
	if err != nil {
		t.Fatalf("ParseSettings() error = %v", err)
	}

	bad, err := ParseSettings(map[string]any{
		"targets": []any{map[string]any{
			"provider":         "npm",
			"ecosystem":        "npm",
			"scope_id":         "npm://registry.npmjs.org/left-pad",
			"metadata_url":     "https://registry.npmjs.org/left-pad",
			"bearer_token_env": "PKGREG_TOKEN",
		}},
	}, "collector-1", getenv)
	_ = bad
	if err == nil {
		t.Fatal("ParseSettings(missing registry) error = nil, want rejection")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("config error leaks the resolved secret: %q", err)
	}

	failure := classifyFailure(RegistryFailure{Class: RegistryFailureAuthDenied, Operation: "fetch_metadata"})
	if strings.Contains(failure.Class, secret) {
		t.Fatalf("failure class leaks the secret: %q", failure.Class)
	}
	if got := newMetadataTooLargeError(20 << 20).Error(); strings.Contains(got, secret) {
		t.Fatalf("oversize error leaks the secret: %q", got)
	}
}

// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	sdk "github.com/eshu-hq/eshu-sdk/collector"
)

func validRequestClaim() sdk.Claim {
	return sdk.Claim{
		ComponentID: "dev.eshu.collector.package_registry", InstanceID: "package-registry-qa",
		CollectorKind: CollectorKind, SourceSystem: "package_registry",
		Scope:       sdk.Scope{ID: "npm://registry.npmjs.org/left-pad", Kind: "package_registry"},
		SourceRunID: "run-1", GenerationID: "generation-1",
		WorkItemID: "work-1", FencingToken: "7", Attempt: 1,
		Deadline: time.Now().UTC().Add(time.Hour), ConfigHandle: "component-config:package-registry",
	}
}

func encodeTestRequest(t *testing.T, claim sdk.Claim, mutate func(map[string]any)) string {
	t.Helper()
	raw := map[string]any{
		"protocol_version": sdk.ProtocolVersionV1Alpha1,
		"claim": map[string]any{
			"component_id": claim.ComponentID, "instance_id": claim.InstanceID,
			"collector_kind": claim.CollectorKind, "source_system": claim.SourceSystem,
			"scope":         map[string]any{"id": claim.Scope.ID, "kind": claim.Scope.Kind},
			"source_run_id": claim.SourceRunID, "generation_id": claim.GenerationID,
			"work_item_id": claim.WorkItemID, "fencing_token": claim.FencingToken,
			"attempt": claim.Attempt, "deadline": claim.Deadline.Format(time.RFC3339Nano),
			"config_handle": claim.ConfigHandle,
		},
		"contract": map[string]any{"protocol_version": sdk.ProtocolVersionV1Alpha1, "facts": []any{}},
	}
	if mutate != nil {
		mutate(raw)
	}
	var sb strings.Builder
	if err := json.NewEncoder(&sb).Encode(raw); err != nil {
		t.Fatal(err)
	}
	return sb.String()
}

// TestParseRequestAcceptsWellFormedClaim pins the happy path.
func TestParseRequestAcceptsWellFormedClaim(t *testing.T) {
	t.Parallel()

	request, err := ParseRequest(strings.NewReader(encodeTestRequest(t, validRequestClaim(), nil)))
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if request.Claim.Scope.ID != "npm://registry.npmjs.org/left-pad" {
		t.Fatalf("scope=%q", request.Claim.Scope.ID)
	}
}

// TestParseRequestRejectsUnknownFields pins strict decoding.
func TestParseRequestRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	raw := encodeTestRequest(t, validRequestClaim(), func(m map[string]any) {
		m["surprise"] = true
	})
	if _, err := ParseRequest(strings.NewReader(raw)); err == nil {
		t.Fatal("ParseRequest(unknown field) error = nil, want rejection")
	}
}

// TestParseRequestRejectsTrailingJSON pins the single-document rule.
func TestParseRequestRejectsTrailingJSON(t *testing.T) {
	t.Parallel()

	raw := encodeTestRequest(t, validRequestClaim(), nil) + "{}\n"
	if _, err := ParseRequest(strings.NewReader(raw)); err == nil {
		t.Fatal("ParseRequest(trailing JSON) error = nil, want rejection")
	}
}

// TestParseRequestRejectsWrongCollectorKind pins kind routing.
func TestParseRequestRejectsWrongCollectorKind(t *testing.T) {
	t.Parallel()

	claim := validRequestClaim()
	claim.CollectorKind = "jira"
	if _, err := ParseRequest(strings.NewReader(encodeTestRequest(t, claim, nil))); err == nil {
		t.Fatal("ParseRequest(wrong kind) error = nil, want rejection")
	}
}

// TestParseRequestRejectsOversizeDocument pins the stdin bound.
func TestParseRequestRejectsOversizeDocument(t *testing.T) {
	t.Parallel()

	raw := strings.Repeat(" ", MaxRequestBytes+1)
	if _, err := ParseRequest(strings.NewReader(raw)); err == nil {
		t.Fatal("ParseRequest(oversize) error = nil, want rejection")
	}
}

// TestContractDeclaresNineKindsAtV1 pins the contract surface: nine dotted
// kinds, each at 1.0.0 with reported confidence.
func TestContractDeclaresNineKindsAtV1(t *testing.T) {
	t.Parallel()

	kinds := FactKinds()
	if len(kinds) != 9 {
		t.Fatalf("FactKinds()=%d, want 9", len(kinds))
	}
	seen := map[string]struct{}{}
	for _, kind := range kinds {
		if !strings.HasPrefix(kind, "package_registry.") {
			t.Fatalf("kind %q is not in the package_registry family", kind)
		}
		if _, dup := seen[kind]; dup {
			t.Fatalf("kind %q declared twice", kind)
		}
		seen[kind] = struct{}{}
	}
	contract := Contract()
	if contract.ProtocolVersion != sdk.ProtocolVersionV1Alpha1 {
		t.Fatalf("protocol=%q", contract.ProtocolVersion)
	}
	if len(contract.Facts) != len(kinds) {
		t.Fatalf("contract facts=%d, want %d", len(contract.Facts), len(kinds))
	}
	for i, declared := range contract.Facts {
		if declared.Kind != kinds[i] {
			t.Fatalf("contract fact %d=%q, want %q", i, declared.Kind, kinds[i])
		}
		if len(declared.SchemaVersions) != 1 || declared.SchemaVersions[0] != SchemaVersion {
			t.Fatalf("kind %q versions=%v, want [1.0.0]", declared.Kind, declared.SchemaVersions)
		}
	}
}

// TestFreshnessHintIsStableAndDriftSensitive pins the generation digest: same
// facts share a hint, payload drift mints a new one.
func TestFreshnessHintIsStableAndDriftSensitive(t *testing.T) {
	t.Parallel()

	first, err := NewPackageEnvelope(PackageObservation{
		Identity:            parityIdentity(),
		ScopeID:             "npm://parity-scope",
		GenerationID:        "generation-a",
		CollectorInstanceID: "fixture-collector",
		SourceURI:           "https://registry.npmjs.org/left-pad",
	}, paritySourceSystem)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewPackageEnvelope(PackageObservation{
		Identity:            parityIdentity(),
		ScopeID:             "npm://parity-scope",
		GenerationID:        "generation-b",
		CollectorInstanceID: "fixture-collector",
		SourceURI:           "https://registry.npmjs.org/left-pad",
	}, paritySourceSystem)
	if err != nil {
		t.Fatal(err)
	}
	if packageFreshnessHint([]sdk.Fact{first}) != packageFreshnessHint([]sdk.Fact{second}) {
		t.Fatal("identical facts must share a freshness hint")
	}
	if packageFreshnessHint(nil) == "" {
		t.Fatal("empty fact set must still mint a hint")
	}
	changed := second
	changed.Payload["visibility"] = "private"
	if packageFreshnessHint([]sdk.Fact{first}) == packageFreshnessHint([]sdk.Fact{changed}) {
		t.Fatal("payload drift must mint a new freshness hint")
	}
}

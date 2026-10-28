// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	sdk "github.com/eshu-hq/eshu-sdk/collector"
)

// TestRatchetRuntimeConfigValidation pins the runtime guard rails: a blank
// collector id, a negative poll interval, a missing target list, and a bad
// nested target each fail naming the cause, while a minimal target validates
// with normalized defaults.
func TestRatchetRuntimeConfigValidation(t *testing.T) {
	t.Parallel()

	if _, err := (RuntimeConfig{}).Validate(); err == nil {
		t.Fatal("Validate(blank collector) error = nil, want rejection")
	} else if !strings.Contains(err.Error(), "collector instance ID is required") {
		t.Fatalf("error = %q, want the collector branch", err)
	}
	negative := RuntimeConfig{CollectorInstanceID: "c", PollInterval: -time.Second, Targets: []TargetConfig{{Provider: "npm"}}}
	if _, err := negative.Validate(); err == nil {
		t.Fatal("Validate(negative poll) error = nil, want rejection")
	} else if !strings.Contains(err.Error(), "must not be negative") {
		t.Fatalf("error = %q, want the poll branch", err)
	}
	empty := RuntimeConfig{CollectorInstanceID: "c"}
	if _, err := empty.Validate(); err == nil {
		t.Fatal("Validate(no targets) error = nil, want rejection")
	} else if !strings.Contains(err.Error(), "at least one") {
		t.Fatalf("error = %q, want the targets branch", err)
	}
	badTarget := RuntimeConfig{CollectorInstanceID: "c", Targets: []TargetConfig{{}}}
	if _, err := badTarget.Validate(); err == nil {
		t.Fatal("Validate(bad target) error = nil, want rejection")
	} else if !strings.Contains(err.Error(), "target 0:") {
		t.Fatalf("error = %q, want the target index", err)
	}
	minimal := RuntimeConfig{
		CollectorInstanceID: " collector-1 ",
		Targets: []TargetConfig{{
			Provider: "npm", Ecosystem: EcosystemNPM, Registry: "https://registry.npmjs.org",
			ScopeID: "npm://registry.npmjs.org/left-pad",
		}},
	}
	validated, err := minimal.Validate()
	if err != nil {
		t.Fatalf("Validate(minimal) error = %v", err)
	}
	if validated.CollectorInstanceID != "collector-1" || validated.PollInterval != defaultRuntimePollInterval {
		t.Fatalf("validated = %+v, want trimmed id and default poll", validated)
	}
	if validated.Targets[0].PackageLimit != defaultPackageLimit || validated.Targets[0].VersionLimit != defaultVersionLimit {
		t.Fatalf("target limits = %+v, want defaults", validated.Targets[0])
	}
	if validated.Targets[0].Visibility != VisibilityUnknown {
		t.Fatalf("visibility = %q, want unknown by default", validated.Targets[0].Visibility)
	}
}

// TestRatchetTargetConfigRejections pins every target rejection branch:
// blank provider, missing and unsupported ecosystems, blank registry and
// scope, out-of-range package and version limits.
func TestRatchetTargetConfigRejections(t *testing.T) {
	t.Parallel()

	valid := TargetConfig{
		Provider: "npm", Ecosystem: EcosystemNPM, Registry: "https://registry.npmjs.org",
		ScopeID: "npm://registry.npmjs.org/left-pad",
	}
	cases := map[string]struct {
		mutate string
		want   string
	}{
		"blank provider":  {mutate: "provider", want: "provider is required"},
		"blank ecosystem": {mutate: "ecosystem", want: "ecosystem is required"},
		"blank registry":  {mutate: "registry", want: "registry is required"},
		"blank scope":     {mutate: "scope", want: "scope_id is required"},
		"package low":     {mutate: "packagelow", want: "package_limit must be between"},
		"package high":    {mutate: "packagehigh", want: "package_limit must be between"},
		"version low":     {mutate: "versionlow", want: "version_limit must be between"},
		"version high":    {mutate: "versionhigh", want: "version_limit must be between"},
	}
	for name, test := range cases {
		candidate := valid
		switch test.mutate {
		case "provider":
			candidate.Provider = "  "
		case "ecosystem":
			candidate.Ecosystem = "  "
		case "registry":
			candidate.Registry = " "
		case "scope":
			candidate.ScopeID = ""
		case "packagelow":
			candidate.PackageLimit = -1
		case "packagehigh":
			candidate.PackageLimit = maxPackageLimit + 1
		case "versionlow":
			candidate.VersionLimit = -1
		case "versionhigh":
			candidate.VersionLimit = maxVersionLimit + 1
		}
		if _, err := candidate.Validate(); err == nil {
			t.Errorf("%s: Validate() error = nil, want rejection", name)
		} else if !strings.Contains(err.Error(), test.want) {
			t.Errorf("%s: error = %q, want %q", name, err, test.want)
		}
	}
	unsupported := valid
	unsupported.Ecosystem = "bogus"
	if _, err := unsupported.Validate(); err == nil {
		t.Fatal("Validate(unsupported ecosystem) error = nil, want rejection")
	} else if !strings.Contains(err.Error(), "ecosystem is unsupported") {
		t.Fatalf("error = %q, want the unsupported branch", err)
	}
}

// TestRatchetCompactPackageList pins list compaction: padding trims,
// blanks and repeats drop.
func TestRatchetCompactPackageList(t *testing.T) {
	t.Parallel()

	got := compactPackageList([]string{" b ", "", "a", "b", "  "})
	if len(got) != 2 || got[0] != "b" || got[1] != "a" {
		t.Fatalf("compacted = %q, want [b a]", got)
	}
}

// errReader fails every read. It proves request parsing reports I/O errors
// instead of hanging or emitting an empty request.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

// TestRatchetParseRequestRejections pins the request guards: read errors,
// oversize documents, wrong protocol versions, a missing claim field, a
// foreign collector kind, a non-positive attempt, and a missing deadline.
func TestRatchetParseRequestRejections(t *testing.T) {
	t.Parallel()

	if _, err := ParseRequest(errReader{}); err == nil {
		t.Fatal("ParseRequest(read error) error = nil, want rejection")
	} else if !strings.Contains(err.Error(), "read request") {
		t.Fatalf("error = %q, want the read branch", err)
	}
	oversize := strings.NewReader(strings.Repeat("x", MaxRequestBytes+1))
	if _, err := ParseRequest(oversize); err == nil {
		t.Fatal("ParseRequest(oversize) error = nil, want rejection")
	} else if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("error = %q, want the size branch", err)
	}
	badProtocol := encodeTestRequest(t, validRequestClaim(), func(m map[string]any) {
		m["protocol_version"] = "v0"
	})
	if _, err := ParseRequest(strings.NewReader(badProtocol)); err == nil {
		t.Fatal("ParseRequest(bad protocol) error = nil, want rejection")
	} else if !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("error = %q, want the protocol branch", err)
	}
	badContract := encodeTestRequest(t, validRequestClaim(), func(m map[string]any) {
		contract, ok := m["contract"].(map[string]any)
		if !ok {
			t.Fatal("test contract is not an object")
		}
		contract["protocol_version"] = "v0"
	})
	if _, err := ParseRequest(strings.NewReader(badContract)); err == nil {
		t.Fatal("ParseRequest(bad contract protocol) error = nil, want rejection")
	}
	missingField := encodeTestRequest(t, validRequestClaim(), func(m map[string]any) {
		claim, ok := m["claim"].(map[string]any)
		if !ok {
			t.Fatal("test claim is not an object")
		}
		claim["component_id"] = " "
	})
	if _, err := ParseRequest(strings.NewReader(missingField)); err == nil {
		t.Fatal("ParseRequest(blank component) error = nil, want rejection")
	} else if !strings.Contains(err.Error(), "claim.component_id is required") {
		t.Fatalf("error = %q, want the field branch", err)
	}
	foreignKind := encodeTestRequest(t, validRequestClaim(), func(m map[string]any) {
		claim, ok := m["claim"].(map[string]any)
		if !ok {
			t.Fatal("test claim is not an object")
		}
		claim["collector_kind"] = "other-collector"
	})
	if _, err := ParseRequest(strings.NewReader(foreignKind)); err == nil {
		t.Fatal("ParseRequest(foreign kind) error = nil, want rejection")
	}
	noAttempt := validRequestClaim()
	noAttempt.Attempt = 0
	if _, err := ParseRequest(strings.NewReader(encodeTestRequest(t, noAttempt, nil))); err == nil {
		t.Fatal("ParseRequest(zero attempt) error = nil, want rejection")
	} else if !strings.Contains(err.Error(), "claim.attempt") {
		t.Fatalf("error = %q, want the attempt branch", err)
	}
	noDeadline := validRequestClaim()
	noDeadline.Deadline = time.Time{}
	if _, err := ParseRequest(strings.NewReader(encodeTestRequest(t, noDeadline, nil))); err == nil {
		t.Fatal("ParseRequest(zero deadline) error = nil, want rejection")
	} else if !strings.Contains(err.Error(), "claim.deadline is required") {
		t.Fatalf("error = %q, want the deadline branch", err)
	}
}

// TestRatchetDecodeReasonBranches pins the error classifier: syntax errors,
// wrong field types naming the field, and anything else as invalid.
func TestRatchetDecodeReasonBranches(t *testing.T) {
	t.Parallel()

	if got := decodeReason(&json.SyntaxError{}); got != "malformed JSON" {
		t.Fatalf("syntax = %q", got)
	}
	if got := decodeReason(&json.UnmarshalTypeError{Field: "protocol_version"}); !strings.Contains(got, `"protocol_version"`) {
		t.Fatalf("type error = %q, want the field named", got)
	}
	if got := decodeReason(errors.New("boom")); got != "invalid request document" {
		t.Fatalf("other = %q", got)
	}
}

// TestRatchetResourceUseDefaults pins limit defaulting: every non-positive
// field fills from the default while explicit values survive.
func TestRatchetResourceUseDefaults(t *testing.T) {
	t.Parallel()

	if got := (ResourceUse{}).withDefaults(); got != DefaultResourceUse() {
		t.Fatalf("defaults = %+v, want %+v", got, DefaultResourceUse())
	}
	partial := ResourceUse{MaxRecordsPerClaim: 7}.withDefaults()
	if partial.MaxRecordsPerClaim != 7 {
		t.Fatalf("explicit bound = %d, want 7 kept", partial.MaxRecordsPerClaim)
	}
	if partial.MaxPayloadBytes != DefaultResourceUse().MaxPayloadBytes ||
		partial.MaxResultBytes != DefaultResourceUse().MaxResultBytes ||
		partial.ClaimTimeoutSeconds != DefaultResourceUse().ClaimTimeoutSeconds {
		t.Fatalf("partial = %+v, want the rest defaulted", partial)
	}
}

// TestRatchetFreshnessHintUnmarshalable pins the freshness fail-safe: a fact
// that cannot serialize yields an empty hint instead of an error.
func TestRatchetFreshnessHintUnmarshalable(t *testing.T) {
	t.Parallel()

	unmarshalable := []sdk.Fact{{Kind: "warning", StableKey: "k", Payload: map[string]any{"fn": func() {}}}}
	if got := packageFreshnessHint(unmarshalable); got != "" {
		t.Fatalf("hint = %q, want blank for an unserializable fact", got)
	}
	if got := packageFreshnessHint(nil); got == "" {
		t.Fatal("empty hint must still be a stable non-blank value")
	}
}

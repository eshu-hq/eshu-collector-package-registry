// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"strings"
	"testing"
)

// TestValidateObservationBoundaryRequiresFields pins the boundary contract:
// each blank field fails naming its noun, and a complete boundary passes.
func TestValidateObservationBoundaryRequiresFields(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name                string
		scopeID             string
		generationID        string
		collectorInstanceID string
		want                string
	}{
		{name: "blank scope", generationID: "g", collectorInstanceID: "c", want: "scope_id"},
		{name: "blank generation", scopeID: "s", collectorInstanceID: "c", want: "generation_id"},
		{name: "blank collector", scopeID: "s", generationID: "g", want: "collector_instance_id"},
		{name: "complete", scopeID: "s", generationID: "g", collectorInstanceID: "c", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validateObservationBoundary(tc.scopeID, tc.generationID, tc.collectorInstanceID, "package observation")
			if tc.want == "" {
				if err != nil {
					t.Fatalf("validateObservationBoundary() error = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validateObservationBoundary() = nil, want an error naming %q", tc.want)
			}
			got := err.Error()
			for _, token := range []string{tc.want, "package observation"} {
				if !strings.Contains(got, token) {
					t.Fatalf("validateObservationBoundary() error = %q, want it to name %q", got, token)
				}
			}
		})
	}
}

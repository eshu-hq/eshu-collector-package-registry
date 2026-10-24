// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"encoding/json"
	"sort"

	stableid "github.com/eshu-hq/eshu-sdk/factschema/identity"

	sdk "github.com/eshu-hq/eshu-sdk/collector"
)

// packageFreshnessHint returns the stable content digest for one package
// metadata collection: the sorted keyed payload digests of the emitted facts.
// Each entry covers its fact kind, stable key, and full payload, so same-key
// payload drift — yanked, deprecated, retracted, unlisted, checksum, URL,
// published_at, visibility, and warning-text changes — mints a new generation
// instead of skipping as unchanged. It covers what the collector actually
// emits — in-window package, version, artifact, hint, event, hosting, and
// warning facts — and never the raw document body, which can carry
// out-of-window content that would mint a new generation while the emitted
// facts stand still. Every entry is JSON-encoded before sorting, so the hint
// is insensitive to fact emission order while nested maps stay canonical
// through StableID's JSON encoding.
func packageFreshnessHint(facts []sdk.Fact) string {
	encoded := make([]string, 0, len(facts))
	for _, fact := range facts {
		entry := map[string]any{
			"fact_kind":  fact.Kind,
			"payload":    fact.Payload,
			"stable_key": fact.StableKey,
		}
		raw, err := json.Marshal(entry)
		if err != nil {
			return ""
		}
		encoded = append(encoded, string(raw))
	}
	sort.Strings(encoded)
	return stableid.StableID("package-registry.collection.freshness", map[string]any{"envelopes": encoded})
}

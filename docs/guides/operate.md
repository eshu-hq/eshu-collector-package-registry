# Operate

What you'll actually see once claims are running, and what every
ending means. Run states (`complete`, `partial`, `retryable`,
`terminal`) are explained in [concepts](../concepts/README.md).

## What an operator actually gets

Today the extension host passes one thing from a claim's result to the
work item: the failure class (`registry_rate_limited`,
`invalid_config`, `claim_deadline_exceeded`, and so on). The collector
writes one JSON line per claim to stderr for local runs and tests, and
the host's process runner drops the text.

The in-tree collector emitted fetch spans and counters. This one has no
equivalent and declares no `telemetry.metricsPrefix` (see
`eshu-hq/eshu#9032`): a per-claim process whose stderr the host drops,
reporting over an SDK result with no metrics field, simply has no
channel to emit metrics under a declared prefix. So don't go looking
for a dashboard; the failure class is the signal.

## Failure classes

Terminal means a person needs to change something. Retryable means
the host can try again later. These come from `failure.go` and the
registry classes in `client.go`.

Terminal:

| Class | What happened |
| --- | --- |
| `invalid_config` | The activation config can't run, including a bad `visibility` value |
| `invalid_evidence` | The metadata document can't become facts |
| `registry_auth_denied` | The registry refused the credentials (401 or 403) |
| `registry_terminal_failure` | Any other non-retryable registry failure, and the fallback for unclassified errors |
| `registry_canceled` | The claim's context was cancelled |
| `record_budget_exceeded` | Past the fact-count bound |
| `payload_budget_exceeded` | One fact past its payload-size bound |
| `result_budget_exceeded` | The encoded result past its size bound |

Retryable:

| Class | What happened |
| --- | --- |
| `claim_deadline_exceeded` | The claim deadline passed; no facts are returned |
| `registry_rate_limited` | The registry throttled the fetch (429) |
| `registry_retryable_failure` | A transient transport error or a 5xx response |

Some outcomes that look like failures don't carry a failure class at
all, and they're worth knowing about. A derived scope that 404s comes
back `partial` with a `registry_not_found` warning fact, and a document
that parses as malformed also lands as partial with warnings rather
than as a failed claim. A document past the 20 MiB cap is odder still:
the error itself is the terminal class `registry_metadata_too_large`,
but the claim normally completes with a single `metadata_too_large`
warning fact instead of burning retries (see `source.go`).

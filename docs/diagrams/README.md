# Diagrams

Every diagram for this collector lives on this page, so there's one
place to keep them honest. Other pages link here instead of copying
the picture.

## System context

How a source system, this collector, the Eshu host, and Eshu core
fit together. The collector only talks to its source and to the
host over stdin/stdout.

```mermaid
flowchart LR
  A["Package registry, one metadata document"] --> B["This collector"] --> C["Eshu host process"] --> D["Eshu core"]
```

Text version: package registry, then this collector, then the Eshu
host process (checks and stores the result), then Eshu core.

## Claim lifecycle

What happens to one claim, from the host sending it on stdin to the
host committing the result or recording a failure class. The
collector checks config and credentials before any provider call.

```mermaid
sequenceDiagram
  participant H as Eshu host
  participant C as This collector
  participant R as Package registry
  H->>C: one claim on stdin
  C->>C: find or derive the target for the claim scope
  C->>R: fetch one bounded metadata document over HTTPS
  R-->>C: document
  C->>C: parse, sanitise addresses, build facts
  C-->>H: one result on stdout
  H->>H: validate and commit, or record the failure class
```

Text version: the host sends a claim, the collector finds (or, for npm
and PyPI, derives) the target, fetches one document, parses it, and
returns facts for the host to validate and commit.

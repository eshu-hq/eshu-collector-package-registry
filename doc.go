// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

// Package collector is the Eshu package-registry collector, extracted from
// the Eshu core repository (eshu-hq/eshu#8862).
//
// It runs through the Eshu component-extension host with the process runner:
// the host passes one bounded collector-sdk/v1alpha1 request on stdin and the
// collector returns one result on stdout. A claim names one package-registry
// scope; the collector fetches the ecosystem metadata document over HTTPS,
// parses it with the registered ecosystem parsers (npm, PyPI, Go module
// proxy, Maven, NuGet, generic JSON, Artifactory wrapper), bounds the
// observations by the configured package and version limits, and emits the
// nine core package_registry.* facts. It is pre-release and holds no producer
// authority until its reviewed cutover.
//
// Boundary: this package imports only the published SDK modules
// (eshu-sdk/collector, eshu-sdk/factschema), the standard library, and its
// own internal helpers. It never imports go/internal, another collector
// implementation, or any core Postgres, graph, queue, or lease handle; the
// import-boundary test enforces this. The host, not this package, validates
// the result and owns claims and commits.
//
// Credentials are read from environment variables named in the activation
// config and never enter facts, source URIs, logs, or statuses. URLs are
// sanitized fail-closed before emission: userinfo is dropped, sensitive query
// keys are stripped, and a value that cannot be taken apart is replaced rather
// than passed through.
package collector

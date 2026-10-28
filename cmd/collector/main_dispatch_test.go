// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package main

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
)

// TestRunSeedRejectsExtraPositionalArg pins the seed dispatch guard: extra
// positional arguments fail instead of seeding into an ambiguous layout.
func TestRunSeedRejectsExtraPositionalArg(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	err := run(context.Background(), []string{"seed", "extra"}, strings.NewReader(""), &stdout, io.Discard, getenv)
	if err == nil {
		t.Fatal("run(seed extra) must fail")
	}
	if !strings.Contains(err.Error(), "unexpected argument") {
		t.Fatalf("error = %v, want an unexpected-argument error", err)
	}
}

// TestRunSeedRejectsUnknownFlag pins the seed flag guard: a misspelled flag
// fails before any file is written.
func TestRunSeedRejectsUnknownFlag(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	if err := run(context.Background(), []string{"seed", "--bogus"}, strings.NewReader(""), &stdout, io.Discard, getenv); err == nil {
		t.Fatal("run(seed --bogus) must fail")
	}
}

// TestRunClaimRejectsUnknownFlag pins the claim flag guard: only --sdk-stdio
// is accepted, so a misspelled adapter flag fails fast.
func TestRunClaimRejectsUnknownFlag(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	if err := run(context.Background(), []string{"--bogus"}, strings.NewReader(""), &stdout, &stderr, getenv); err == nil {
		t.Fatal("run(--bogus) must fail")
	}
}

// TestRunClaimRejectsPositionalArg pins the claim dispatch guard: serving
// one claim reads stdin, so a positional argument is a usage error.
func TestRunClaimRejectsPositionalArg(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	err := run(context.Background(), []string{"extra"}, strings.NewReader(""), &stdout, &stderr, getenv)
	if err == nil {
		t.Fatal("run(extra) must fail")
	}
	if !strings.Contains(err.Error(), "unexpected argument") {
		t.Fatalf("error = %v, want an unexpected-argument error", err)
	}
}

// TestRunClaimRejectsMalformedRequest pins the stdin guard: a body that is
// not a collector SDK request fails instead of emitting a result.
func TestRunClaimRejectsMalformedRequest(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	if err := run(context.Background(), nil, strings.NewReader("{not json"), &stdout, &stderr, getenv); err == nil {
		t.Fatal("run() with malformed stdin must fail")
	}
}

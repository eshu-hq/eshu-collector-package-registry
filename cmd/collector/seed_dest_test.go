// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSeedDestDirStaysWorldTraversable pins the G301 triage decision
// (eshu-hq/eshu-collector-package-registry#13): the seed destination keeps
// other read/traverse bits because the seeded binary (0755) and manifest
// (0644) are public and secret-free, and a consumer running under a
// different UID (init container seeds, another container reads) must still
// reach them. Tightening the directory to 0750 must fail this test.
func TestSeedDestDirStaysWorldTraversable(t *testing.T) {
	t.Parallel()

	dest := filepath.Join(t.TempDir(), "component")
	var stdout bytes.Buffer
	if err := run(context.Background(), []string{"seed", "--dest", dest}, strings.NewReader(""), &stdout, io.Discard, getenv); err != nil {
		t.Fatalf("run(seed) error = %v", err)
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("seeded dest missing: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("seeded dest %s is not a directory", dest)
	}
	if perm := info.Mode().Perm(); perm&0o005 != 0o005 {
		t.Fatalf("seeded dest mode %04o lacks other read/traverse bits, want xx5", perm)
	}
	if info, err := os.Stat(filepath.Join(dest, "collector")); err != nil {
		t.Fatalf("seeded binary missing: %v", err)
	} else if perm := info.Mode().Perm(); perm&0o044 != 0o044 {
		t.Fatalf("seeded binary mode %04o is not world-readable, want xx5", perm)
	}
	if info, err := os.Stat(filepath.Join(dest, "manifest.yaml")); err != nil {
		t.Fatalf("seeded manifest missing: %v", err)
	} else if perm := info.Mode().Perm(); perm&0o044 != 0o044 {
		t.Fatalf("seeded manifest mode %04o is not world-readable, want 644", perm)
	}
}

// TestSeedG301DispositionCarriesReason keeps the gosec G301 triage in code,
// not in an exclusion list: the MkdirAll line must carry a reasoned
// `#nosec G301` (eshu-hq/eshu-collector-package-registry#13).
func TestSeedG301DispositionCarriesReason(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("seed.go")
	if err != nil {
		t.Fatal(err)
	}
	for line := range strings.Lines(string(raw)) {
		if !strings.Contains(line, "os.MkdirAll(dest") {
			continue
		}
		if !strings.Contains(line, "#nosec G301") {
			t.Fatalf("MkdirAll line lacks a reasoned #nosec G301: %q", strings.TrimSpace(line))
		}
		if _, reason, _ := strings.Cut(line[strings.Index(line, "#nosec G301")+len("#nosec G301"):], "--"); strings.TrimSpace(reason) == "" {
			t.Fatalf("MkdirAll #nosec G301 carries no reason: %q", strings.TrimSpace(line))
		}
		return
	}
	t.Fatal("seed.go has no os.MkdirAll(dest) line")
}

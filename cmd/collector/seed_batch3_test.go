// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	collector "github.com/eshu-hq/eshu-collector-package-registry"
)

// TestSelectImageRefPrefersFlag pins the reference precedence: a non-empty
// flag wins over the environment, one trailing newline is trimmed from
// either, and a lone newline counts as unset.
func TestSelectImageRefPrefersFlag(t *testing.T) {
	t.Parallel()

	env := map[string]string{imageRefEnv: "env-ref"}
	lookup := func(key string) string { return env[key] }

	if got := selectImageRef("flag-ref", lookup); got != "flag-ref" {
		t.Fatalf("selectImageRef(flag) = %q, want the flag value", got)
	}
	if got := selectImageRef("flag-ref\n", lookup); got != "flag-ref" {
		t.Fatalf("selectImageRef(flag with newline) = %q, want the newline trimmed", got)
	}
	if got := selectImageRef("", lookup); got != "env-ref" {
		t.Fatalf("selectImageRef(empty flag) = %q, want the environment value", got)
	}
	if got := selectImageRef("\n", lookup); got != "env-ref" {
		t.Fatalf("selectImageRef(lone newline flag) = %q, want the environment value", got)
	}
	env[imageRefEnv] = "env-ref\n"
	if got := selectImageRef("", lookup); got != "env-ref" {
		t.Fatalf("selectImageRef() = %q, want the environment newline trimmed", got)
	}
	if got := selectImageRef("", nil); got != "" {
		t.Fatalf("selectImageRef(nil lookup) = %q, want empty", got)
	}
	if got := selectImageRef("", func(string) string { return "" }); got != "" {
		t.Fatalf("selectImageRef(unset env) = %q, want empty", got)
	}
}

// TestSeedRejectsMissingDest pins the destination guard: a blank destination
// fails before the executable is read.
func TestSeedRejectsMissingDest(t *testing.T) {
	t.Parallel()

	for _, dest := range []string{"", "   "} {
		if _, err := seed(dest, os.Args[0], collector.ManifestYAML); err == nil {
			t.Fatalf("seed(%q) must fail without a destination", dest)
		}
	}
}

// TestSeedCopiesExecutableAndManifest pins the happy path through seed: the
// binary is copied executable, the manifest lands byte-identical, and the
// digest names the copied binary.
func TestSeedCopiesExecutableAndManifest(t *testing.T) {
	t.Parallel()

	executable, err := os.Executable()
	if err != nil {
		t.Skipf("os.Executable() unavailable: %v", err)
	}
	dest := t.TempDir()
	digest, err := seed(dest, executable, collector.ManifestYAML)
	if err != nil {
		t.Fatalf("seed() error = %v", err)
	}
	if len(digest) != 64 || strings.Trim(digest, "0123456789abcdef") != "" {
		t.Fatalf("seed() digest = %q, want lowercase hex sha256", digest)
	}
	binary, err := os.ReadFile(filepath.Join(dest, seededBinaryName))
	if err != nil {
		t.Fatalf("seeded binary missing: %v", err)
	}
	want, err := os.ReadFile(executable)
	if err != nil {
		t.Fatalf("os.ReadFile(executable) error = %v", err)
	}
	if !bytes.Equal(binary, want) {
		t.Fatal("seeded binary differs from the source executable")
	}
	if info, err := os.Stat(filepath.Join(dest, seededBinaryName)); err != nil {
		t.Fatalf("seeded binary missing: %v", err)
	} else if info.Mode().Perm()&0o111 == 0 {
		t.Fatal("seeded binary is not executable")
	}
	manifest, err := os.ReadFile(filepath.Join(dest, seededManifestName))
	if err != nil {
		t.Fatalf("seeded manifest missing: %v", err)
	}
	if !bytes.Equal(manifest, collector.ManifestYAML) {
		t.Fatal("seeded manifest differs from the embedded manifest")
	}
}

// TestWriteAtomicFailsClosed pins the atomic-write guards: a writer error
// leaves no target behind, and an unwritable directory fails before any
// partial file is visible.
func TestWriteAtomicFailsClosed(t *testing.T) {
	t.Parallel()

	target := filepath.Join(t.TempDir(), "out.yaml")
	boom := errors.New("boom")
	if err := writeAtomic(target, 0o644, func(io.Writer) error { return boom }); err == nil {
		t.Fatal("writeAtomic() must surface the writer error")
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatal("writeAtomic() must leave no target behind after a writer error")
	}
	missingDirTarget := filepath.Join(t.TempDir(), "no-such-dir", "out.yaml")
	if err := writeAtomic(missingDirTarget, 0o644, func(w io.Writer) error {
		_, err := w.Write([]byte("x"))
		return err
	}); err == nil {
		t.Fatal("writeAtomic() must fail when the target directory does not exist")
	}
}

// TestWriteAtomicRoundTrip pins the success path: the bytes land intact with
// the requested mode.
func TestWriteAtomicRoundTrip(t *testing.T) {
	t.Parallel()

	target := filepath.Join(t.TempDir(), "out.yaml")
	if err := writeAtomic(target, 0o644, func(w io.Writer) error {
		_, err := w.Write([]byte("manifest-bytes"))
		return err
	}); err != nil {
		t.Fatalf("writeAtomic() error = %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("atomic target missing: %v", err)
	}
	if string(got) != "manifest-bytes" {
		t.Fatalf("writeAtomic() bytes = %q, want the written content", got)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("atomic target stat: %v", err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("writeAtomic() mode = %o, want 644", info.Mode().Perm())
	}
}

// TestRunSeedStampsImageRefEndToEnd pins the operator path through runSeed: a
// valid --image-ref lands in the seeded manifest, and an invalid one fails
// before any file is written without echoing the value.
func TestRunSeedStampsImageRefEndToEnd(t *testing.T) {
	t.Parallel()

	image := testImageRef()
	dest := t.TempDir()
	var stdout bytes.Buffer
	if err := runSeed([]string{"--dest", dest, "--image-ref", image}, &stdout, nil); err != nil {
		t.Fatalf("runSeed() error = %v", err)
	}
	manifest, err := os.ReadFile(filepath.Join(dest, seededManifestName))
	if err != nil {
		t.Fatalf("seeded manifest missing: %v", err)
	}
	if !strings.Contains(string(manifest), "image: "+image) {
		t.Fatal("seeded manifest does not carry the stamped image reference")
	}
	if !strings.Contains(stdout.String(), "seeded ") {
		t.Fatalf("runSeed() stdout = %q, want the seeded summary line", stdout.String())
	}

	badDest := filepath.Join(t.TempDir(), "rejected")
	bad := "ghcr.io/eshu-hq/eshu-collector-package-registry:latest"
	var badStdout bytes.Buffer
	if err := runSeed([]string{"--dest", badDest, "--image-ref", bad}, &badStdout, nil); err == nil {
		t.Fatal("runSeed() must reject a non-digest image reference")
	} else if strings.Contains(err.Error(), bad) {
		t.Fatalf("runSeed() error must not echo the reference: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(badDest, seededManifestName)); !os.IsNotExist(statErr) {
		t.Fatal("runSeed() must write no manifest after rejecting the image reference")
	}
}

// TestSeedRejectsUnreadableExecutable pins the binary failure path: a
// missing source executable fails before any manifest is written.
func TestSeedRejectsUnreadableExecutable(t *testing.T) {
	t.Parallel()

	dest := filepath.Join(t.TempDir(), "component")
	if _, err := seed(dest, filepath.Join(t.TempDir(), "no-such-binary"), collector.ManifestYAML); err == nil {
		t.Fatal("seed() must fail for an unreadable executable")
	}
	if _, statErr := os.Stat(filepath.Join(dest, seededManifestName)); !os.IsNotExist(statErr) {
		t.Fatal("seed() must write no manifest after the binary copy fails")
	}
}

// TestSeedRejectsFileAsDest pins the destination-creation guard: when the
// destination cannot be created, seed fails before touching the binary.
func TestSeedRejectsFileAsDest(t *testing.T) {
	t.Parallel()

	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	if _, err := seed(filepath.Join(blocker, "sub"), os.Args[0], collector.ManifestYAML); err == nil {
		t.Fatal("seed() must fail when the destination directory cannot be created")
	}
}

// TestRunSeedRequiresDest pins the missing-destination failure through the
// flag path.
func TestRunSeedRequiresDest(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	if err := runSeed([]string{}, &stdout, nil); err == nil {
		t.Fatal("runSeed() must fail without --dest")
	}
}

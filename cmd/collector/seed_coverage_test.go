// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package main

import (
	"strings"
	"testing"

	collector "github.com/eshu-hq/eshu-collector-package-registry"
)

// testImageRef returns a digest-pinned reference that satisfies
// imageRefPattern.
func testImageRef() string {
	return "ghcr.io/eshu-hq/eshu-collector-package-registry@sha256:" + strings.Repeat("ab", 32)
}

// TestManifestForEmptyImageLeavesManifestUntouched pins the default seed path:
// without a reference the embedded placeholder ships unchanged.
func TestManifestForEmptyImageLeavesManifestUntouched(t *testing.T) {
	t.Parallel()

	got, err := manifestFor(collector.ManifestYAML, "")
	if err != nil {
		t.Fatalf("manifestFor() error = %v", err)
	}
	if string(got) != string(collector.ManifestYAML) {
		t.Fatal("empty image must leave the manifest unchanged")
	}
}

// TestManifestForRejectsBadImage pins fail-closed stamping: an invalid
// reference aborts before any file is written, and the seeded error never
// echoes the value.
func TestManifestForRejectsBadImage(t *testing.T) {
	t.Parallel()

	bad := "ghcr.io/eshu-hq/eshu-collector-package-registry:latest"
	if _, err := manifestFor(collector.ManifestYAML, bad); err == nil {
		t.Fatal("manifestFor() must reject a non-digest reference")
	} else if strings.Contains(err.Error(), bad) {
		t.Fatalf("error must not echo the reference: %v", err)
	}
	if _, err := manifestFor(collector.ManifestYAML, "  "); err == nil {
		t.Fatal("manifestFor() must reject a blank reference")
	}
}

// TestManifestForStampsEmbeddedManifest pins the operator path: a valid
// digest-pinned reference replaces the placeholder artifact image and
// nothing else.
func TestManifestForStampsEmbeddedManifest(t *testing.T) {
	t.Parallel()

	image := testImageRef()
	got, err := manifestFor(collector.ManifestYAML, image)
	if err != nil {
		t.Fatalf("manifestFor() error = %v", err)
	}
	if count := strings.Count(string(got), "image: "+image); count != 1 {
		t.Fatalf("stamped manifest carries %d stamped image lines, want 1", count)
	}
	if strings.Contains(string(got), "sha256:"+strings.Repeat("0", 64)) {
		t.Fatal("stamped manifest still carries the placeholder digest")
	}
}

// TestStampImageRejectsZeroImageLines pins that a manifest without an
// artifact image cannot be stamped: the reference would be silently dropped.
func TestStampImageRejectsZeroImageLines(t *testing.T) {
	t.Parallel()

	manifest := []byte("apiVersion: eshu.dev/v1alpha1\nkind: ComponentPackage\n")
	if _, err := stampImage(manifest, testImageRef()); err == nil {
		t.Fatal("stampImage() must fail without an artifact image line")
	}
}

// TestStampImageRejectsTwoImageLines pins that a manifest with two artifact
// images cannot be stamped: the stamp would otherwise apply twice.
func TestStampImageRejectsTwoImageLines(t *testing.T) {
	t.Parallel()

	manifest := []byte("artifacts:\n  - image: old-one\n  - image: old-two\n")
	if _, err := stampImage(manifest, testImageRef()); err == nil {
		t.Fatal("stampImage() must fail with two artifact image lines")
	}
}

// TestStampImagePreservesLinePrefix pins the literal replacement: only the
// value changes, so indentation and YAML quoting survive and no "$1"-style
// template expansion can alter the digest.
func TestStampImagePreservesLinePrefix(t *testing.T) {
	t.Parallel()

	image := testImageRef()
	manifest := []byte("artifacts:\n    - platform: linux/amd64\n      image: old-value\n")
	got, err := stampImage(manifest, image)
	if err != nil {
		t.Fatalf("stampImage() error = %v", err)
	}
	if want := "      image: " + image + "\n"; !strings.Contains(string(got), want) {
		t.Fatalf("stampImage() = %q, want line %q", got, want)
	}
}

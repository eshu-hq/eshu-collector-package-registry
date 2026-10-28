// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"strings"
	"testing"
)

// TestRatchetNormalizeRejectsBlanks pins the normalization guard rails: a
// blank ecosystem, registry, or name fails naming the missing field, and a
// Maven identity without a group namespace fails on the namespace.
func TestRatchetNormalizeRejectsBlanks(t *testing.T) {
	t.Parallel()

	if _, err := Normalize(RawIdentity{Registry: "https://registry.npmjs.org", RawName: "x"}); err == nil {
		t.Fatal("Normalize(blank ecosystem) error = nil, want rejection")
	} else if !strings.Contains(err.Error(), "ecosystem must not be blank") {
		t.Fatalf("error = %q, want the ecosystem branch", err)
	}
	if _, err := Normalize(RawIdentity{Ecosystem: EcosystemNPM, RawName: "x"}); err == nil {
		t.Fatal("Normalize(blank registry) error = nil, want rejection")
	} else if !strings.Contains(err.Error(), "registry must not be blank") {
		t.Fatalf("error = %q, want the registry branch", err)
	}
	if _, err := Normalize(RawIdentity{Ecosystem: EcosystemNPM, Registry: "https://registry.npmjs.org", RawName: "  "}); err == nil {
		t.Fatal("Normalize(blank name) error = nil, want rejection")
	} else if !strings.Contains(err.Error(), "package name must not be blank") {
		t.Fatalf("error = %q, want the name branch", err)
	}
	maven := RawIdentity{Ecosystem: EcosystemMaven, Registry: "https://repo.maven.apache.org/maven2", RawName: "demo"}
	if _, err := Normalize(maven); err == nil {
		t.Fatal("Normalize(maven without namespace) error = nil, want rejection")
	} else if !strings.Contains(err.Error(), "requires group namespace") {
		t.Fatalf("error = %q, want the namespace branch", err)
	}
}

// TestRatchetEcosystemAliasCoverage pins the remaining manager aliases:
// hex, pub, and OS spellings canonicalize, and unknown spellings stay
// blank for the fail-closed guard above.
func TestRatchetEcosystemAliasCoverage(t *testing.T) {
	t.Parallel()

	for alias, want := range map[string]Ecosystem{
		"hexpm":  EcosystemHex,
		"dart":   EcosystemPub,
		"deb":    EcosystemOS,
		"spm":    EcosystemSwift,
		"python": EcosystemPyPI,
	} {
		if got := NormalizeEcosystem(Ecosystem(alias)); got != want {
			t.Errorf("NormalizeEcosystem(%q) = %q, want %q", alias, got, want)
		}
	}
	if got := NormalizeEcosystem("bogus"); got != "" {
		t.Fatalf("NormalizeEcosystem(bogus) = %q, want blank", got)
	}
}

// TestRatchetNormalizeEcosystemShapes pins normalization through the less
// common ecosystems: pub rewrites its legacy registry, hex lowercases name
// and namespace, generic preserves casing, and swift derives its registry
// from the namespace.
func TestRatchetNormalizeEcosystemShapes(t *testing.T) {
	t.Parallel()

	pub, err := Normalize(RawIdentity{Ecosystem: EcosystemPub, Registry: "pub.dartlang.org", RawName: "x"})
	if err != nil {
		t.Fatalf("Normalize(pub legacy) error = %v", err)
	}
	if pub.Registry != "pub.dev" {
		t.Fatalf("pub registry = %q, want the rewritten pub.dev", pub.Registry)
	}
	hex, err := Normalize(RawIdentity{Ecosystem: EcosystemHex, Registry: "https://hex.pm", RawName: "MyPkg", Namespace: " MyNS "})
	if err != nil {
		t.Fatalf("Normalize(hex) error = %v", err)
	}
	if hex.NormalizedName != "mypkg" || hex.Namespace != "myns" {
		t.Fatalf("hex = %+v, want lowercased name and namespace", hex)
	}
	generic, err := Normalize(RawIdentity{Ecosystem: EcosystemGeneric, Registry: "https://example.invalid", RawName: "Name", Namespace: "NS"})
	if err != nil {
		t.Fatalf("Normalize(generic) error = %v", err)
	}
	if generic.NormalizedName != "Name" || generic.Namespace != "NS" {
		t.Fatalf("generic = %+v, want preserved casing", generic)
	}
	swift, err := Normalize(RawIdentity{Ecosystem: EcosystemSwift, Registry: "https://github.com", RawName: "Tool", Namespace: "github.com/example/tool"})
	if err != nil {
		t.Fatalf("Normalize(swift) error = %v", err)
	}
	if swift.Ecosystem != EcosystemSwift || !strings.HasPrefix(swift.PackageID, "swift://") || !strings.Contains(swift.PackageID, "tool") {
		t.Fatalf("swift = %+v, want a swift package id", swift)
	}
}

// TestRatchetNormalizeManagerAndModule pins the manager default and the
// module namespace: an explicit manager lowercases, a missing one takes the
// ecosystem, and a slash-less module has no namespace.
func TestRatchetNormalizeManagerAndModule(t *testing.T) {
	t.Parallel()

	explicit, err := Normalize(RawIdentity{Ecosystem: EcosystemNPM, Registry: "https://registry.npmjs.org", RawName: "x", PackageManager: " NPM "})
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if explicit.PackageManager != "npm" {
		t.Fatalf("manager = %q, want lowercased explicit value", explicit.PackageManager)
	}
	def, err := Normalize(RawIdentity{Ecosystem: EcosystemNPM, Registry: "https://registry.npmjs.org", RawName: "x"})
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if def.PackageManager != string(EcosystemNPM) {
		t.Fatalf("manager = %q, want the ecosystem default", def.PackageManager)
	}
	plain, err := Normalize(RawIdentity{Ecosystem: EcosystemGoModule, Registry: "https://proxy.golang.org", RawName: "plain"})
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if plain.Namespace != "" {
		t.Fatalf("namespace = %q, want blank for a slash-less module", plain.Namespace)
	}
}

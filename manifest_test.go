// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// componentManifest is the subset of manifest.yaml this repository asserts.
type componentManifest struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		ID      string `yaml:"id"`
		Name    string `yaml:"name"`
		Version string `yaml:"version"`
	} `yaml:"metadata"`
	Spec struct {
		CompatibleCore string   `yaml:"compatibleCore"`
		ComponentType  string   `yaml:"componentType"`
		CollectorKinds []string `yaml:"collectorKinds"`
		Runtime        struct {
			SDKProtocol string `yaml:"sdkProtocol"`
			Adapter     string `yaml:"adapter"`
		} `yaml:"runtime"`
		Artifacts []struct {
			Platform string `yaml:"platform"`
			Image    string `yaml:"image"`
		} `yaml:"artifacts"`
		EmittedFacts []struct {
			Kind             string   `yaml:"kind"`
			PayloadSchemaRef string   `yaml:"payloadSchemaRef"`
			SchemaVersions   []string `yaml:"schemaVersions"`
			SourceConfidence []string `yaml:"sourceConfidence"`
		} `yaml:"emittedFacts"`
		Telemetry struct {
			MetricsPrefix string `yaml:"metricsPrefix"`
		} `yaml:"telemetry"`
	} `yaml:"spec"`
}

func loadManifest(t *testing.T) componentManifest {
	t.Helper()
	raw, err := os.ReadFile("manifest.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var manifest componentManifest
	if err := yaml.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("manifest.yaml: %v", err)
	}
	return manifest
}

var digestPattern = regexp.MustCompile(`@sha256:[A-Fa-f0-9]{64}$`)

// placeholderDigest is the artifact digest the repository manifest carries
// permanently. manifest.yaml is go:embed'ed into the binary, so a digest
// written into it would belong to an older build than the image that embeds
// it. The deployed digest is stamped at deploy time by `collector seed
// --image-ref` instead (README, "Image and seeding").
const placeholderDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"

// isPlaceholderImage reports whether image names the placeholder digest.
func isPlaceholderImage(image string) bool {
	return strings.HasSuffix(image, "@"+placeholderDigest)
}

// TestPlaceholderImageGate pins the gate on both sides: the placeholder passes
// and a real-looking digest fails.
func TestPlaceholderImageGate(t *testing.T) {
	t.Parallel()

	const repo = "ghcr.io/eshu-hq/eshu-collector-package-registry@"
	if !isPlaceholderImage(repo + placeholderDigest) {
		t.Fatal("the placeholder digest must pass the gate")
	}
	if isPlaceholderImage(repo + "sha256:" + strings.Repeat("a", 64)) {
		t.Fatal("a real-looking digest must fail the gate")
	}
}

// TestManifestDeclaresNineKinds proves the component manifest matches the
// collector contract: the id, the SDK protocol, one digest-pinned placeholder
// artifact, and exactly the nine package_registry.* kinds at 1.0.0 reported.
func TestManifestDeclaresNineKinds(t *testing.T) {
	t.Parallel()

	manifest := loadManifest(t)
	if strings.TrimSpace(manifest.APIVersion) == "" || strings.TrimSpace(manifest.Kind) == "" {
		t.Fatal("manifest missing apiVersion/kind")
	}
	if manifest.Metadata.ID != ComponentID {
		t.Fatalf("manifest id = %q, want %q", manifest.Metadata.ID, ComponentID)
	}
	if manifest.Metadata.Version == "" {
		t.Fatal("manifest missing metadata version")
	}
	if manifest.Spec.Runtime.SDKProtocol != "collector-sdk/v1alpha1" {
		t.Fatalf("sdkProtocol = %q, want collector-sdk/v1alpha1", manifest.Spec.Runtime.SDKProtocol)
	}
	if len(manifest.Spec.CollectorKinds) != 1 || manifest.Spec.CollectorKinds[0] != CollectorKind {
		t.Fatalf("collectorKinds = %v, want [%s]", manifest.Spec.CollectorKinds, CollectorKind)
	}
	if len(manifest.Spec.Artifacts) == 0 {
		t.Fatal("manifest declares no artifacts")
	}
	for _, artifact := range manifest.Spec.Artifacts {
		if !digestPattern.MatchString(artifact.Image) {
			t.Fatalf("artifact image %q is not digest-pinned", artifact.Image)
		}
		if strings.Contains(artifact.Image, ":latest") {
			t.Fatalf("artifact image %q uses a mutable tag", artifact.Image)
		}
		// The repository manifest names the placeholder, never a real digest:
		// the manifest is embedded in the image, so a committed digest could
		// only describe an older build. Deployments stamp the digest they
		// pulled with `collector seed --image-ref REF@sha256:<digest>`.
		if !isPlaceholderImage(artifact.Image) {
			t.Fatalf("artifact image %q is not the placeholder digest: do not commit a real digest "+
				"(the manifest is embedded in the image); stamp the deployed one with collector seed --image-ref", artifact.Image)
		}
	}
	declared := map[string][]string{}
	for _, family := range manifest.Spec.EmittedFacts {
		declared[family.Kind] = family.SchemaVersions
		if len(family.SourceConfidence) != 1 || family.SourceConfidence[0] != "reported" {
			t.Fatalf("kind %q confidence=%v, want [reported]", family.Kind, family.SourceConfidence)
		}
	}
	want := map[string][]string{}
	for _, kind := range FactKinds() {
		want[kind] = []string{SchemaVersion}
	}
	if !reflect.DeepEqual(declared, want) {
		t.Fatalf("manifest kinds = %v, contract kinds = %v", declared, want)
	}
	if manifest.Spec.Telemetry.MetricsPrefix != MetricsPrefix {
		t.Fatalf("metricsPrefix = %q, want %q", manifest.Spec.Telemetry.MetricsPrefix, MetricsPrefix)
	}
}

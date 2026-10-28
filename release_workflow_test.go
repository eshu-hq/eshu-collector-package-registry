// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

var secretUsePattern = regexp.MustCompile(`secrets\.([A-Za-z0-9_]+)`)

const (
	// tagShapeCheck is the exact tag filter the workflow must run before it
	// publishes anything.
	tagShapeCheck = `grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$'`
	// signerIdentity is the certificate identity a keyless signature from this
	// workflow carries, derived from the running workflow reference.
	signerIdentity = "https://github.com/${{ github.workflow_ref }}"
	// oidcIssuerFlag pins the GitHub Actions OIDC issuer on the verify step.
	oidcIssuerFlag = `--certificate-oidc-issuer "https://token.actions.githubusercontent.com"`
	// sbomFile is the SBOM the generate and attest steps must agree on.
	sbomFile = "eshu-collector-package-registry.spdx.json"
)

// stepIndex returns the position of the first step for which match is true,
// or -1.
func stepIndex(steps []workflowStep, match func(workflowStep) bool) int {
	for i, step := range steps {
		if match(step) {
			return i
		}
	}
	return -1
}

// contractProblems checks the steps that make a published image trustworthy:
// the tag and manifest checks run before anything is pushed, one platform is
// built, the signature is verified against a pinned identity and issuer, and
// the provenance and SBOM attestations are generated and verified.
func contractProblems(steps []workflowStep) []string {
	var problems []string
	tagIdx := stepIndex(steps, func(s workflowStep) bool { return strings.Contains(s.Run, tagShapeCheck) })
	if tagIdx < 0 {
		problems = append(problems, "no step filters the tag with "+tagShapeCheck)
	}
	versionIdx := stepIndex(steps, func(s workflowStep) bool {
		return strings.Contains(s.Run, "manifest.yaml") && strings.Contains(s.Run, "TAG_NAME") && strings.Contains(s.Run, "exit 1")
	})
	if versionIdx < 0 {
		problems = append(problems, "no step fails unless the tag matches manifest.yaml metadata.version")
	}
	publishIdx := stepIndex(steps, func(s workflowStep) bool { return fmt.Sprint(s.With["push"]) == "true" })
	if publishIdx < 0 {
		problems = append(problems, "no step pushes the image")
	} else {
		if tagIdx >= 0 && tagIdx > publishIdx {
			problems = append(problems, "the tag shape check runs after the image is pushed")
		}
		if versionIdx >= 0 && versionIdx > publishIdx {
			problems = append(problems, "the manifest version check runs after the image is pushed")
		}
		if got := fmt.Sprint(steps[publishIdx].With["platforms"]); got != "linux/amd64" {
			problems = append(problems, fmt.Sprintf("build platforms = %q, want exactly linux/amd64", got))
		}
	}
	verifyIdx := stepIndex(steps, func(s workflowStep) bool { return strings.Contains(s.Run, "cosign verify") })
	if verifyIdx < 0 {
		problems = append(problems, "no step runs cosign verify")
	} else {
		verify := steps[verifyIdx]
		if verify.Env["CERTIFICATE_IDENTITY"] != signerIdentity {
			problems = append(problems, fmt.Sprintf("cosign identity = %q, want %q", verify.Env["CERTIFICATE_IDENTITY"], signerIdentity))
		}
		for _, want := range []string{`--certificate-identity "${CERTIFICATE_IDENTITY}"`, oidcIssuerFlag} {
			if !strings.Contains(verify.Run, want) {
				problems = append(problems, "cosign verify lacks "+want)
			}
		}
		if strings.Contains(verify.Run, "regexp") {
			problems = append(problems, "cosign verify matches the identity by regular expression, want the exact identity")
		}
	}
	attestIdx := stepIndex(steps, func(s workflowStep) bool { return strings.Contains(s.Run, "gh attestation verify") })
	if attestIdx < 0 {
		problems = append(problems, "no step runs gh attestation verify")
	} else {
		run := steps[attestIdx].Run
		if strings.Count(run, "gh attestation verify") < 2 {
			problems = append(problems, "gh attestation verify must run for provenance and for the SBOM")
		}
		for _, want := range []string{`-R "${GITHUB_REPOSITORY}"`, "--predicate-type https://spdx.dev/Document/v2.3"} {
			if !strings.Contains(run, want) {
				problems = append(problems, "gh attestation verify lacks "+want)
			}
		}
	}
	for uses, input := range map[string]string{"anchore/sbom-action@": "output-file", "actions/attest@": "sbom-path"} {
		idx := stepIndex(steps, func(s workflowStep) bool { return strings.HasPrefix(s.Uses, uses) })
		if idx < 0 {
			problems = append(problems, "no step uses "+uses)
		} else if got := fmt.Sprint(steps[idx].With[input]); got != sbomFile {
			problems = append(problems, fmt.Sprintf("%s input %s = %q, want %q", uses, input, got, sbomFile))
		}
	}
	return problems
}

// releaseWorkflowProblems returns every way raw breaks the release workflow's
// safety properties: it must run only for pushed v* tags, hold only the
// permissions publishing and attesting an image need, use no secret but
// GITHUB_TOKEN, pin every action by full commit SHA, deploy nothing, and keep
// the tag, platform, signature, and attestation checks.
func releaseWorkflowProblems(raw []byte) []string {
	var workflow workflowFile
	if err := yaml.Unmarshal(raw, &workflow); err != nil {
		return []string{fmt.Sprintf("release.yml does not parse: %v", err)}
	}
	var problems []string
	push, ok := workflow.On["push"].(map[string]any)
	if !ok {
		push = nil
	}
	tags, ok := push["tags"].([]any)
	if !ok {
		tags = nil
	}
	if len(workflow.On) != 1 || len(push) != 1 || len(tags) != 1 || tags[0] != "v*" {
		problems = append(problems, fmt.Sprintf("triggers = %v, want only push of tags [v*]", workflow.On))
	}
	if len(workflow.Permissions) != 1 || workflow.Permissions["contents"] != "read" {
		problems = append(problems, fmt.Sprintf("workflow permissions = %v, want contents: read only", workflow.Permissions))
	}
	want := map[string]string{"contents": "read", "packages": "write", "id-token": "write", "attestations": "write"}
	for name, job := range workflow.Jobs {
		if len(job.Permissions) != len(want) {
			problems = append(problems, fmt.Sprintf("job %s permissions = %v, want %v", name, job.Permissions, want))
		}
		for key, value := range want {
			if job.Permissions[key] != value {
				problems = append(problems, fmt.Sprintf("job %s permission %s = %q, want %q", name, key, job.Permissions[key], value))
			}
		}
		for _, step := range job.Steps {
			if step.Uses != "" && !pinnedAction.MatchString(step.Uses) {
				problems = append(problems, fmt.Sprintf("job %s step %q uses %q, want a full commit SHA pin", name, step.Name, step.Uses))
			}
			for _, word := range []string{"kubectl", "helm ", "argocd", "terraform", "aws "} {
				if strings.Contains(step.Run, word) {
					problems = append(problems, fmt.Sprintf("job %s step %q runs %q: the release workflow must not deploy", name, step.Name, word))
				}
			}
		}
	}
	image, ok := workflow.Jobs["image"]
	if !ok || len(workflow.Jobs) != 1 {
		problems = append(problems, "want exactly one job, named image")
	}
	problems = append(problems, contractProblems(image.Steps)...)
	for _, match := range secretUsePattern.FindAllStringSubmatch(string(raw), -1) {
		if match[1] != "GITHUB_TOKEN" {
			problems = append(problems, fmt.Sprintf("release.yml uses secrets.%s, want only GITHUB_TOKEN", match[1]))
		}
	}
	return problems
}

// mainHeadGateStepName is the release workflow step that stops a tag pointing
// anywhere but the protected main head from publishing an image.
const mainHeadGateStepName = "Require the tag to point at the protected main head"

// mainHeadGateStep returns the release workflow step that compares the pushed
// tag's commit with the protected main head.
func mainHeadGateStep(t *testing.T) workflowStep {
	t.Helper()
	raw, err := os.ReadFile(".github/workflows/release.yml")
	if err != nil {
		t.Fatal(err)
	}
	var workflow workflowFile
	if err := yaml.Unmarshal(raw, &workflow); err != nil {
		t.Fatal(err)
	}
	steps := workflow.Jobs["image"].Steps
	idx := stepIndex(steps, func(s workflowStep) bool { return s.Name == mainHeadGateStepName })
	if idx < 0 {
		t.Fatalf("release.yml has no step named %q", mainHeadGateStepName)
	}
	return steps[idx]
}

// TestReleaseWorkflowGatesTagOnMainHead pins the release workflow shape: a
// step before anything is pushed must fail unless the pushed tag points at
// the protected main head, so a v* tag cut from any other commit publishes
// nothing.
func TestReleaseWorkflowGatesTagOnMainHead(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(".github/workflows/release.yml")
	if err != nil {
		t.Fatal(err)
	}
	var workflow workflowFile
	if err := yaml.Unmarshal(raw, &workflow); err != nil {
		t.Fatal(err)
	}
	steps := workflow.Jobs["image"].Steps
	gateIdx := stepIndex(steps, func(s workflowStep) bool { return s.Name == mainHeadGateStepName })
	if gateIdx < 0 {
		t.Fatalf("release.yml has no step named %q", mainHeadGateStepName)
	}
	publishIdx := stepIndex(steps, func(s workflowStep) bool { return fmt.Sprint(s.With["push"]) == "true" })
	if publishIdx < 0 {
		t.Fatal("release.yml has no step that pushes the image")
	}
	if gateIdx > publishIdx {
		t.Fatalf("step %q runs after the image is pushed", mainHeadGateStepName)
	}
	gate := steps[gateIdx]
	if !strings.Contains(gate.Run, "origin/main") {
		t.Errorf("step %q never resolves the protected main head (want origin/main in the script)", mainHeadGateStepName)
	}
	if !strings.Contains(gate.Run, "github.sha") && !strings.Contains(gate.Env["TAG_COMMIT"], "github.sha") {
		t.Errorf("step %q never reads the pushed tag's commit (want github.sha in the script or TAG_COMMIT)", mainHeadGateStepName)
	}
	if !strings.Contains(gate.Run, "exit 1") {
		t.Errorf("step %q never fails closed (want exit 1 on mismatch)", mainHeadGateStepName)
	}
}

// TestReleaseMainHeadStepComparesTagCommitWithMainHead runs the workflow's own
// gate script the way the runner does (bash -e) in a scratch repo, so a tag
// off the main head cannot publish a signed image while a tag on it can.
func TestReleaseMainHeadStepComparesTagCommitWithMainHead(t *testing.T) {
	t.Parallel()

	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is not available")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	step := mainHeadGateStep(t)

	dir := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		full := append([]string{"-c", "user.email=test@example.com", "-c", "user.name=test", "-c", "protocol.file.allow=always"}, args...)
		cmd := exec.Command("git", full...)
		cmd.Dir = dir
		cmd.Env = []string{"PATH=" + os.Getenv("PATH")}
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "file"), []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", "file")
	git("commit", "-m", "one")
	git("remote", "add", "origin", dir)
	first := git("rev-parse", "HEAD")
	git("commit", "--allow-empty", "-m", "two")
	head := git("rev-parse", "HEAD")
	git("checkout", "-b", "feature", first)
	git("commit", "--allow-empty", "-m", "off main")
	offMain := git("rev-parse", "HEAD")
	git("checkout", "-q", "main")

	for name, tc := range map[string]struct {
		commit string
		want   bool
	}{
		"tag on the main head passes": {head, true},
		"older commit on main fails":  {first, false},
		"commit off main fails":       {offMain, false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cmd := exec.Command(bash, "-e", "-c", step.Run)
			cmd.Dir = dir
			cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "TAG_NAME=v0.1.0", "TAG_COMMIT=" + tc.commit}
			out, err := cmd.CombinedOutput()
			if got := err == nil; got != tc.want {
				t.Fatalf("tag commit %q: passed = %t, want %t\n%s", tc.commit, got, tc.want, out)
			}
		})
	}
}

// TestReleaseWorkflowHoldsSafetyProperties pins the release workflow shape.
func TestReleaseWorkflowHoldsSafetyProperties(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(".github/workflows/release.yml")
	if err != nil {
		t.Fatal(err)
	}
	if problems := releaseWorkflowProblems(raw); len(problems) > 0 {
		t.Fatalf("release.yml problems:\n%s", strings.Join(problems, "\n"))
	}
}

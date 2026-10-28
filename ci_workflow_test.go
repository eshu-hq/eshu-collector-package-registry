// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type workflowStep struct {
	Name string            `yaml:"name"`
	Uses string            `yaml:"uses"`
	Run  string            `yaml:"run"`
	With map[string]any    `yaml:"with"`
	Env  map[string]string `yaml:"env"`
}

type workflowFile struct {
	On          map[string]any    `yaml:"on"`
	Permissions map[string]string `yaml:"permissions"`
	Jobs        map[string]struct {
		If          string            `yaml:"if"`
		Uses        string            `yaml:"uses"`
		With        map[string]any    `yaml:"with"`
		Permissions map[string]string `yaml:"permissions"`
		Steps       []workflowStep    `yaml:"steps"`
	} `yaml:"jobs"`
}

var pinnedAction = regexp.MustCompile(`^[A-Za-z0-9._-]+/[A-Za-z0-9._/-]+@[0-9a-f]{40}$`)

// TestCIWorkflowPinsActionsByCommitSHA keeps a moved tag from changing what
// pull-request CI executes. CI holds only contents: read, but it still runs
// third-party action code on every pull request.
func TestCIWorkflowPinsActionsByCommitSHA(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(".github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	var workflow workflowFile
	if err := yaml.Unmarshal(raw, &workflow); err != nil {
		t.Fatal(err)
	}
	if len(workflow.Jobs) == 0 {
		t.Fatal("ci.yml defines no jobs")
	}
	for name, job := range workflow.Jobs {
		if job.Uses != "" && !pinnedAction.MatchString(job.Uses) {
			t.Errorf("job %s uses %q, want a full commit SHA pin", name, job.Uses)
		}
		for _, step := range job.Steps {
			if step.Uses != "" && !pinnedAction.MatchString(step.Uses) {
				t.Errorf("job %s uses %q, want a full commit SHA pin", name, step.Uses)
			}
		}
	}
	if _, ok := workflow.Jobs["go-ci"]; !ok {
		t.Error("ci.yml has no go-ci reusable call; lane bodies must live upstream at a pinned SHA")
	}
}

// imageGoClauseProblems returns every way cond breaks the image-lane
// go-source contract: the image embeds Go sources, so the lane must run
// when the go change class is true. A bare substring check accepts an
// inverted comparison, which selects the lane exactly when Go sources did
// not change and skips it when they did.
func imageGoClauseProblems(cond string) []string {
	var problems []string
	if !strings.Contains(cond, "needs.changes.outputs.go == 'true'") {
		problems = append(problems, "image if lacks \"needs.changes.outputs.go == 'true'\": Go sources ship in the image")
	}
	for _, inverted := range []string{
		"needs.changes.outputs.go != 'true'",
		"needs.changes.outputs.go == 'false'",
		"needs.changes.outputs.go != 'false'",
	} {
		if strings.Contains(cond, inverted) {
			problems = append(problems, "image if contains inverted clause "+inverted)
		}
	}
	return problems
}

// TestImageLaneRunsOnGoSources keeps the shipped artifact validated on
// PRs that change what it embeds: the Dockerfile COPYs go.mod/go.sum,
// COPYs the whole context (which keeps .go sources), and go-builds the
// binary, so a Go-source or go.mod PR must select the image lane. A
// go.mod toolchain bump past the pinned base would otherwise pass every
// Go lane yet break docker build, surfacing only as post-merge red main.
// The live ci.yml text must pass; an inverted or removed go clause must
// fail. The image lane body lives upstream; the caller selects it with
// the go-ci run-image input, which the same clause rules judge.
func TestImageLaneRunsOnGoSources(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(".github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	var workflow workflowFile
	if err := yaml.Unmarshal(raw, &workflow); err != nil {
		t.Fatal(err)
	}
	job, ok := workflow.Jobs["go-ci"]
	if !ok {
		t.Fatal("ci.yml has no go-ci reusable call")
	}
	cond, ok := job.With["run-image"].(string)
	if !ok {
		t.Fatal("go-ci run-image input is not a string")
	}
	if problems := imageGoClauseProblems(cond); len(problems) > 0 {
		t.Errorf("image lane breaks its go-source contract:\n%s", strings.Join(problems, "\n"))
	}

	inverted := strings.Replace(cond, "needs.changes.outputs.go == 'true'", "needs.changes.outputs.go != 'true'", 1)
	if len(imageGoClauseProblems(inverted)) == 0 {
		t.Errorf("inverted go clause: the gate accepted a lane that skips Go-source PRs")
	}
	removed := strings.ReplaceAll(cond, "needs.changes.outputs.go == 'true'", "")
	if len(imageGoClauseProblems(removed)) == 0 {
		t.Errorf("removed go clause: the gate accepted a lane with no go selection")
	}
}

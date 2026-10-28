// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package collector

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type scheduleStep struct {
	Name string `yaml:"name"`
	If   string `yaml:"if"`
	Run  string `yaml:"run"`
}

type scheduleWorkflow struct {
	On   map[string]any `yaml:"on"`
	Jobs map[string]struct {
		If          string            `yaml:"if"`
		Needs       []string          `yaml:"needs"`
		Uses        string            `yaml:"uses"`
		With        map[string]any    `yaml:"with"`
		Permissions map[string]string `yaml:"permissions"`
		Steps       []scheduleStep    `yaml:"steps"`
	} `yaml:"jobs"`
}

// TestCIScheduleFilesGovulncheckFindings pins the weekly scanner's filing
// path on both sides: the schedule trigger must exist, the go-ci call must
// run govulncheck on it, and the caller-side scheduled-findings job must
// open an issue with issues:write when the scheduled scan fails. Without
// any one of these the finding dies silently.
func TestCIScheduleFilesGovulncheckFindings(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(".github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	var workflow scheduleWorkflow
	if err := yaml.Unmarshal(raw, &workflow); err != nil {
		t.Fatal(err)
	}
	schedules, ok := workflow.On["schedule"].([]any)
	if !ok || len(schedules) == 0 {
		t.Fatal("ci.yml has no on.schedule trigger for the weekly scanner runs")
	}
	var hasCron bool
	for _, entry := range schedules {
		if entryMap, ok := entry.(map[string]any); ok {
			if cron, ok := entryMap["cron"].(string); ok && strings.TrimSpace(cron) != "" {
				hasCron = true
			}
		}
	}
	if !hasCron {
		t.Fatal("ci.yml on.schedule has no cron entry")
	}
	goCI, ok := workflow.Jobs["go-ci"]
	if !ok {
		t.Fatal("ci.yml has no go-ci reusable call")
	}
	runGovulncheck, ok := goCI.With["run-govulncheck"].(string)
	if !ok {
		t.Fatal("go-ci run-govulncheck input is not a string")
	}
	if !strings.Contains(runGovulncheck, "schedule") {
		t.Errorf("go-ci run-govulncheck %q does not run on the schedule event", runGovulncheck)
	}
	job, ok := workflow.Jobs["scheduled-findings"]
	if !ok {
		t.Fatal("ci.yml has no scheduled-findings job")
	}
	if !strings.Contains(job.If, "failure()") || !strings.Contains(job.If, "schedule") {
		t.Errorf("scheduled-findings condition %q must gate on failure() and the schedule event", job.If)
	}
	if job.Permissions["issues"] != "write" {
		t.Errorf("scheduled-findings issues permission = %q, want write so the schedule run can file findings", job.Permissions["issues"])
	}
	var filesIssue bool
	for _, step := range job.Steps {
		if strings.Contains(step.Run, "gh issue create") &&
			strings.Contains(job.If, "failure()") &&
			strings.Contains(job.If, "schedule") {
			filesIssue = true
		}
	}
	if !filesIssue {
		t.Error("scheduled-findings has no step that files an issue when the scheduled scan fails")
	}
}

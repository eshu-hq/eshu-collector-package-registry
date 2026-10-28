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
		Permissions map[string]string `yaml:"permissions"`
		Steps       []scheduleStep    `yaml:"steps"`
	} `yaml:"jobs"`
}

// TestCIScheduleFilesGovulncheckFindings pins the weekly scanner's filing
// path on both sides: the schedule trigger must exist, the govulncheck lane
// must run on it with issues:write, and a step must open an issue when the
// scheduled scan fails. Without any one of these the finding dies silently.
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
	job, ok := workflow.Jobs["govulncheck"]
	if !ok {
		t.Fatal("ci.yml has no govulncheck job")
	}
	if !strings.Contains(job.If, "schedule") {
		t.Errorf("govulncheck condition %q does not run on the schedule event", job.If)
	}
	if job.Permissions["issues"] != "write" {
		t.Errorf("govulncheck issues permission = %q, want write so the schedule run can file findings", job.Permissions["issues"])
	}
	var filesIssue bool
	for _, step := range job.Steps {
		if strings.Contains(step.Run, "gh issue create") &&
			strings.Contains(step.If, "failure()") &&
			strings.Contains(step.If, "schedule") {
			filesIssue = true
		}
	}
	if !filesIssue {
		t.Error("govulncheck has no step that files an issue when the scheduled scan fails")
	}
}

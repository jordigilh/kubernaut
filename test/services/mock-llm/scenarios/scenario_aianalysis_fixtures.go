/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package scenarios

import (
	"regexp"
	"strings"

	"github.com/jordigilh/kubernaut/pkg/shared/uuid"
)

var rePromptSeverity = regexp.MustCompile(`(?i)(?:^|[^a-z0-9_])["']?severity["']?\s*[:=]\s*["']?([a-z0-9_-]+)`)

type aiAnalysisFixtureSpec struct {
	ScenarioName  string
	Fingerprint   string
	SignalName    string
	Severity      string
	MatchSeverity string
	WorkflowName  string
	ActionType    string
	Title         string
	Rationale     string
	RootCause     string
	ResourceKind  string
	ResourceNS    string
	ResourceName  string
	APIVersion    string
	Parameters    map[string]string
}

// aiAnalysisFixtureScenarios keeps E2E workflows isolated by test context. The
// workflow catalog has scalar priority labels, so a shared fixture cannot
// safely represent the P0/P1/P2 contexts exercised by these specs.
func aiAnalysisFixtureScenarios() []*configScenario {
	specs := []aiAnalysisFixtureSpec{
		{
			ScenarioName: "aa_e2e_staging_oom",
			Fingerprint:  "e2e-fingerprint-002",
			SignalName:   "OOMKilled",
			Severity:     "warning",
			WorkflowName: "oomkill-increase-memory-aa-staging-v1",
			ActionType:   "IncreaseMemoryLimits",
			Title:        "OOMKill Recovery - Increase Memory Limits",
			Rationale:    "Container exceeded memory limits in staging; increasing limits is the safest remediation",
			RootCause:    "Container exceeded memory limits due to a traffic spike",
			ResourceKind: "Pod",
			ResourceNS:   "staging",
			ResourceName: "web-app",
			APIVersion:   "v1",
			Parameters:   map[string]string{"MEMORY_LIMIT_NEW": "512Mi"},
		},
		{
			ScenarioName:  "aa_e2e_approval_crashloop",
			Fingerprint:   "e2e-audit-approval",
			SignalName:    "CrashLoopBackOff",
			Severity:      "critical",
			MatchSeverity: "high",
			WorkflowName:  "crashloop-config-fix-aa-approval-v1",
			ActionType:    "RestartDeployment",
			Title:         "CrashLoopBackOff - Configuration Fix",
			Rationale:     "A production configuration regression requires an explicitly approved restart",
			RootCause:     "Deployment configuration is invalid",
			ResourceKind:  "Deployment",
			ResourceNS:    "payments",
			ResourceName:  "payment-service",
			APIVersion:    "apps/v1",
			Parameters: map[string]string{
				"NAMESPACE":       "payments",
				"DEPLOYMENT_NAME": "payment-service",
			},
		},
		{
			ScenarioName: "aa_e2e_audit_crashloop",
			Fingerprint:  "e2e-audit-fingerprint",
			SignalName:   "CrashLoopBackOff",
			Severity:     "warning",
			WorkflowName: "crashloop-config-fix-aa-audit-v1",
			ActionType:   "RestartDeployment",
			Title:        "CrashLoopBackOff - Audit Configuration Fix",
			Rationale:    "A production configuration regression requires a controlled restart",
			RootCause:    "Deployment configuration is invalid",
			ResourceKind: "Deployment",
			ResourceNS:   "payments",
			ResourceName: "payment-service",
			APIVersion:   "apps/v1",
			Parameters: map[string]string{
				"NAMESPACE":       "payments",
				"DEPLOYMENT_NAME": "payment-service",
			},
		},
		{
			ScenarioName: "aa_e2e_rego_crashloop",
			Fingerprint:  "e2e-audit-rego",
			SignalName:   "CrashLoopBackOff",
			Severity:     "warning",
			WorkflowName: "crashloop-config-fix-aa-rego-v1",
			ActionType:   "RestartDeployment",
			Title:        "CrashLoopBackOff - Rego Configuration Fix",
			Rationale:    "A staging configuration regression requires a controlled restart",
			RootCause:    "Deployment configuration is invalid",
			ResourceKind: "Deployment",
			ResourceNS:   "default",
			ResourceName: "frontend",
			APIVersion:   "apps/v1",
			Parameters: map[string]string{
				"NAMESPACE":       "default",
				"DEPLOYMENT_NAME": "frontend",
			},
		},
		{
			ScenarioName: "aa_e2e_session_crashloop",
			Fingerprint:  "e2e-fingerprint-session-001",
			SignalName:   "CrashLoopBackOff",
			Severity:     "warning",
			WorkflowName: "crashloop-config-fix-aa-session-v1",
			ActionType:   "RestartDeployment",
			Title:        "CrashLoopBackOff - Configuration Fix",
			Rationale:    "The staging workload has a configuration regression that requires a restart",
			RootCause:    "Workload configuration is invalid",
			ResourceKind: "Pod",
			ResourceNS:   "staging",
			ResourceName: "session-test-pod",
			APIVersion:   "v1",
			Parameters: map[string]string{
				"NAMESPACE":       "staging",
				"DEPLOYMENT_NAME": "session-test-pod",
			},
		},
		{
			ScenarioName: "aa_e2e_detected_labels_crashloop",
			Fingerprint:  "e2e-fp-056-001",
			SignalName:   "CrashLoopBackOff",
			Severity:     "critical",
			WorkflowName: "crashloop-config-fix-aa-detected-labels-v1",
			ActionType:   "RestartDeployment",
			Title:        "CrashLoopBackOff - Configuration Fix",
			Rationale:    "The production deployment has a configuration regression that requires a restart",
			RootCause:    "Deployment configuration is invalid",
			ResourceKind: "Deployment",
			// ADR-056 creates the deployment in a random namespace per spec.
			ResourceNS:   "",
			ResourceName: "app-e2e-001",
			APIVersion:   "apps/v1",
			Parameters: map[string]string{
				"NAMESPACE":       "default",
				"DEPLOYMENT_NAME": "app-e2e-001",
			},
		},
		{
			ScenarioName: "aa_e2e_data_quality_crashloop",
			Fingerprint:  "e2e-fingerprint-004",
			SignalName:   "CrashLoopBackOff",
			Severity:     "warning",
			WorkflowName: "crashloop-config-fix-aa-data-quality-v1",
			ActionType:   "RestartDeployment",
			Title:        "CrashLoopBackOff - Configuration Fix",
			Rationale:    "The production workload has a configuration regression that requires a restart",
			RootCause:    "Workload configuration is invalid",
			ResourceKind: "Pod",
			ResourceNS:   "production",
			ResourceName: "test-app",
			APIVersion:   "v1",
			Parameters: map[string]string{
				"NAMESPACE":       "production",
				"DEPLOYMENT_NAME": "test-app",
			},
		},
	}

	result := make([]*configScenario, 0, len(specs))
	for _, spec := range specs {
		result = append(result, newSelectorScenario(spec.ScenarioName, ScenarioSelector{
			// Fingerprints are not propagated into AgentSession because they are
			// correlation data, not workflow-selection input. Match the fields
			// that are visible in KA's prompt instead, while retaining the
			// fingerprint for direct handler/unit-test contexts.
			CustomMatch: func(ctx *DetectionContext) (bool, float64) {
				return matchAIAnalysisFixture(ctx, spec)
			},
			Confidence: 1.0,
		}, aiAnalysisFixtureConfig(spec)))
	}
	return result
}

func matchAIAnalysisFixture(ctx *DetectionContext, spec aiAnalysisFixtureSpec) (bool, float64) {
	if ctx == nil {
		return false, 0
	}

	combined := strings.ToLower(ctx.Content + " " + ctx.AllText)
	if spec.Fingerprint != "" && strings.Contains(combined, strings.ToLower(spec.Fingerprint)) {
		return true, 1.0
	}

	signal := strings.ToLower(strings.TrimSpace(extractSignal(ctx)))
	if spec.SignalName != "" && !strings.Contains(signal, strings.ToLower(strings.TrimSpace(spec.SignalName))) {
		return false, 0
	}
	resourceName := strings.ToLower(strings.TrimSpace(spec.ResourceName))
	if resourceName == "" || !strings.Contains(combined, resourceName) {
		return false, 0
	}
	resourceNamespace := strings.ToLower(strings.TrimSpace(spec.ResourceNS))
	if resourceNamespace != "" && !strings.Contains(combined, resourceNamespace) {
		return false, 0
	}
	severity := strings.ToLower(strings.TrimSpace(spec.MatchSeverity))
	if severity == "" {
		severity = strings.ToLower(strings.TrimSpace(spec.Severity))
	}
	if severity != "" && promptSeverity(ctx) != severity {
		return false, 0
	}

	return true, 1.0
}

// promptSeverity uses the first severity field: workflow-selection prompts
// carry the original signal severity before the later Phase 1 RCA assessment,
// which may classify the incident differently.
func promptSeverity(ctx *DetectionContext) string {
	for _, text := range []string{ctx.Content, ctx.AllText} {
		match := rePromptSeverity.FindStringSubmatch(text)
		if len(match) > 1 {
			return strings.ToLower(strings.TrimSpace(match[1]))
		}
	}
	return ""
}

func aiAnalysisFixtureConfig(spec aiAnalysisFixtureSpec) MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName:         spec.ScenarioName,
		SignalName:           spec.SignalName,
		Severity:             spec.Severity,
		WorkflowName:         spec.WorkflowName,
		WorkflowID:           uuid.DeterministicUUID(spec.WorkflowName),
		ActionType:           spec.ActionType,
		WorkflowTitle:        spec.Title,
		Confidence:           0.95,
		Rationale:            spec.Rationale,
		RootCause:            spec.RootCause,
		ResourceKind:         spec.ResourceKind,
		ResourceNS:           spec.ResourceNS,
		ResourceName:         spec.ResourceName,
		APIVersion:           spec.APIVersion,
		Parameters:           spec.Parameters,
		ExecutionEngine:      "job",
		Contributing:         []string{"configuration_regression", "recent_deployment_update"},
		InvestigationOutcome: "actionable",
		IsActionable:         BoolPtr(true),
		ForceText:            BoolPtr(false),
	}
}

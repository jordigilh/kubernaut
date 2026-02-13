/*
Copyright 2025 Jordi Gil.

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

package holmesgptapi

import (
	"crypto/sha256"
	"fmt"
)

// HAPIWorkflowFixture defines test workflow data for HAPI integration tests
// Pattern: Matches holmesgpt-api/tests/fixtures/workflow_fixtures.py structure
type HAPIWorkflowFixture struct {
	WorkflowName    string
	Version         string
	DisplayName     string
	Description     string
	ActionType      string // DD-WORKFLOW-016: FK to action_type_taxonomy (e.g., "AdjustResources", "ScaleReplicas")
	SignalType      string
	Severity        string
	Component       string
	Environment     string
	Priority        string
	RiskTolerance   string
	ContainerImage  string
	ContainerDigest string
	ContentHash     string
}

// ToYAMLContent generates workflow YAML content (matches Python fixture)
func (wf *HAPIWorkflowFixture) ToYAMLContent() string {
	return fmt.Sprintf(`apiVersion: kubernaut.io/v1alpha1
kind: WorkflowSchema
metadata:
  workflow_id: %s
  version: "%s"
  description: %s
labels:
  signal_type: %s
  severity: %s
  component: %s
  environment: %s
  priority: %s
  risk_tolerance: %s
parameters:
  - name: NAMESPACE
    type: string
    required: true
    description: Target namespace for the operation
  - name: TARGET_NAME
    type: string
    required: true
    description: Target resource name
execution:
  engine: tekton
  bundle: %s`, wf.WorkflowName, wf.Version, wf.Description, wf.SignalType,
		wf.Severity, wf.Component, wf.Environment, wf.Priority, wf.RiskTolerance, wf.ContainerImage)
}

// GetHAPITestWorkflows returns test workflows for HAPI integration tests
// Pattern: Matches holmesgpt-api/tests/fixtures/workflow_fixtures.py TEST_WORKFLOWS
func GetHAPITestWorkflows() []HAPIWorkflowFixture {
	workflows := []HAPIWorkflowFixture{
		{
			WorkflowName:    "oomkill-increase-memory-v1", // MUST match Mock LLM and AIAnalysis test_workflows.go
			Version:         "1.0.0",
			DisplayName:     "OOMKill Remediation - Increase Memory Limits",
			Description:     "Increases memory limits for pods experiencing OOMKilled events",
			ActionType:      "AdjustResources", // DD-WORKFLOW-016: Modify resource requests/limits
			SignalType:      "OOMKilled",
			Severity:        "critical",
			Component:       "pod",
			Environment:     "production",
			Priority:        "P0",
			RiskTolerance:   "low",
			ContainerImage:  "ghcr.io/kubernaut/workflows/oomkill-increase-memory:v1.0.0@sha256:0000000000000000000000000000000000000000000000000000000000000001",
			ContainerDigest: "sha256:0000000000000000000000000000000000000000000000000000000000000001",
		},
		{
			WorkflowName:    "memory-optimize-v1", // MUST match Mock LLM and AIAnalysis test_workflows.go
			Version:         "1.0.0",
			DisplayName:     "OOMKill Remediation - Scale Down Replicas",
			Description:     "Reduces replica count for deployments experiencing OOMKilled",
			ActionType:      "ScaleReplicas", // DD-WORKFLOW-016: Horizontally scale workload
			SignalType:      "OOMKilled",
			Severity:        "high",
			Component:       "deployment",
			Environment:     "staging",
			Priority:        "P1",
			RiskTolerance:   "medium",
			ContainerImage:  "ghcr.io/kubernaut/workflows/oomkill-scale-down:v1.0.0@sha256:0000000000000000000000000000000000000000000000000000000000000002",
			ContainerDigest: "sha256:0000000000000000000000000000000000000000000000000000000000000002",
		},
		{
			WorkflowName:    "crashloop-config-fix-v1", // MUST match Mock LLM and AIAnalysis test_workflows.go
			Version:         "1.0.0",
			DisplayName:     "CrashLoopBackOff - Fix Configuration",
			Description:     "Identifies and fixes configuration issues causing CrashLoopBackOff",
			ActionType:      "ReconfigureService", // DD-WORKFLOW-016: Update ConfigMap/Secret values
			SignalType:      "CrashLoopBackOff",
			Severity:        "high",
			Component:       "pod",
			Environment:     "production",
			Priority:        "P1",
			RiskTolerance:   "low",
			ContainerImage:  "ghcr.io/kubernaut/workflows/crashloop-fix-config:v1.0.0@sha256:0000000000000000000000000000000000000000000000000000000000000003",
			ContainerDigest: "sha256:0000000000000000000000000000000000000000000000000000000000000003",
		},
		{
			WorkflowName:    "node-drain-reboot-v1", // MUST match Mock LLM and AIAnalysis test_workflows.go
			Version:         "1.0.0",
			DisplayName:     "NodeNotReady - Drain and Reboot",
			Description:     "Safely drains and reboots nodes in NotReady state",
			ActionType:      "RestartPod", // DD-WORKFLOW-016: Delete and recreate to recover
			SignalType:      "NodeNotReady",
			Severity:        "critical",
			Component:       "node",
			Environment:     "production",
			Priority:        "P0",
			RiskTolerance:   "low",
			ContainerImage:  "ghcr.io/kubernaut/workflows/node-drain-reboot:v1.0.0@sha256:0000000000000000000000000000000000000000000000000000000000000004",
			ContainerDigest: "sha256:0000000000000000000000000000000000000000000000000000000000000004",
		},
		{
			WorkflowName:    "image-pull-backoff-fix-credentials",
			Version:         "1.0.0",
			DisplayName:     "ImagePullBackOff - Fix Registry Credentials",
			Description:     "Fixes ImagePullBackOff errors by updating registry credentials",
			ActionType:      "ReconfigureService", // DD-WORKFLOW-016: Update credentials = configuration
			SignalType:      "ImagePullBackOff",
			Severity:        "high",
			Component:       "pod",
			Environment:     "production",
			Priority:        "P1",
			RiskTolerance:   "medium",
			ContainerImage:  "ghcr.io/kubernaut/workflows/imagepull-fix-creds:v1.0.0@sha256:0000000000000000000000000000000000000000000000000000000000000005",
			ContainerDigest: "sha256:0000000000000000000000000000000000000000000000000000000000000005",
		},
	}

	// Calculate content hash for each workflow
	for i := range workflows {
		content := workflows[i].ToYAMLContent()
		hash := sha256.Sum256([]byte(content))
		workflows[i].ContentHash = fmt.Sprintf("%x", hash)
	}

	return workflows
}

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
// BR-WORKFLOW-004: riskTolerance removed (deprecated, never stored in DB)
type HAPIWorkflowFixture struct {
	WorkflowName    string
	Version         string
	DisplayName     string
	Description     string
	ActionType      string // DD-WORKFLOW-016 V1.0: FK to action_type_taxonomy (e.g., "IncreaseMemoryLimits", "ScaleReplicas")
	SignalName      string // Maps to HAPI signal_name (Issue #166: was SignalType)
	Severity        string
	Component       string
	Environment     string
	Priority        string
	ContainerImage  string
	ContainerDigest string
	ContentHash     string
}

// ToYAMLContent generates workflow YAML content per BR-WORKFLOW-004 format
func (wf *HAPIWorkflowFixture) ToYAMLContent() string {
	return fmt.Sprintf(`metadata:
  workflowId: %s
  version: "%s"
  description:
    what: %s
    whenToUse: Test workflow for %s
actionType: %s
labels:
  signalType: %s
  severity: %s
  component: %s
  environment: %s
  priority: %s
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
  bundle: %s`, wf.WorkflowName, wf.Version, wf.Description, wf.ActionType,
		wf.ActionType, wf.SignalName,
		wf.Severity, wf.Component, wf.Environment, wf.Priority, wf.ContainerImage)
}

// GetHAPITestWorkflows returns test workflows for HAPI integration tests
// Pattern: Matches holmesgpt-api/tests/fixtures/workflow_fixtures.py TEST_WORKFLOWS
// DD-WORKFLOW-017: ContainerImage references real OCI images at quay.io/kubernaut-cicd/test-workflows (same as E2E)
func GetHAPITestWorkflows() []HAPIWorkflowFixture {
	workflows := []HAPIWorkflowFixture{
		{
			WorkflowName:    "oomkill-increase-memory-v1", // MUST match Mock LLM and AIAnalysis test_workflows.go
			Version:         "1.0.0",
			DisplayName:     "OOMKill Remediation - Increase Memory Limits",
			Description:     "Increases memory limits for pods experiencing OOMKilled events",
			ActionType:      "IncreaseMemoryLimits", // DD-WORKFLOW-016 V1.0: Increase memory limits
			SignalName:      "OOMKilled",
			Severity:        "critical",
			Component:       "pod",
			Environment:     "production",
			Priority:        "P0",
			ContainerImage:  "quay.io/kubernaut-cicd/test-workflows/oomkill-increase-memory:v1.0.0",
			ContainerDigest: "", // Tag-based ref; digest not required for pull
		},
		{
			WorkflowName:    "memory-optimize-v1", // MUST match Mock LLM and AIAnalysis test_workflows.go
			Version:         "1.0.0",
			DisplayName:     "OOMKill Remediation - Scale Down Replicas",
			Description:     "Reduces replica count for deployments experiencing OOMKilled",
			ActionType:      "ScaleReplicas", // DD-WORKFLOW-016: Horizontally scale workload
			SignalName:      "OOMKilled",
			Severity:        "high",
			Component:       "deployment",
			Environment:     "staging",
			Priority:        "P1",
			ContainerImage:  "quay.io/kubernaut-cicd/test-workflows/memory-optimize:v1.0.0",
			ContainerDigest: "", // Tag-based ref
		},
		{
			WorkflowName:    "crashloop-config-fix-v1", // MUST match Mock LLM and AIAnalysis test_workflows.go
			Version:         "1.0.0",
			DisplayName:     "CrashLoopBackOff - Fix Configuration",
			Description:     "Identifies and fixes configuration issues causing CrashLoopBackOff",
			ActionType:      "RestartDeployment", // DD-WORKFLOW-016 V1.0: Rolling restart for config fix
			SignalName:      "CrashLoopBackOff",
			Severity:        "high",
			Component:       "pod",
			Environment:     "production",
			Priority:        "P1",
			ContainerImage:  "quay.io/kubernaut-cicd/test-workflows/crashloop-config-fix:v1.0.0",
			ContainerDigest: "", // Tag-based ref
		},
		{
			WorkflowName:    "node-drain-reboot-v1", // MUST match Mock LLM and AIAnalysis test_workflows.go
			Version:         "1.0.0",
			DisplayName:     "NodeNotReady - Drain and Reboot",
			Description:     "Safely drains and reboots nodes in NotReady state",
			ActionType:      "RestartPod", // DD-WORKFLOW-016: Delete and recreate to recover
			SignalName:      "NodeNotReady",
			Severity:        "critical",
			Component:       "node",
			Environment:     "production",
			Priority:        "P0",
			ContainerImage:  "quay.io/kubernaut-cicd/test-workflows/node-drain-reboot:v1.0.0",
			ContainerDigest: "", // Tag-based ref
		},
		{
			WorkflowName:    "image-pull-backoff-fix-credentials",
			Version:         "1.0.0",
			DisplayName:     "ImagePullBackOff - Fix Registry Credentials",
			Description:     "Fixes ImagePullBackOff errors by updating registry credentials",
			ActionType:      "RollbackDeployment", // DD-WORKFLOW-016 V1.0: Revert to previous revision
			SignalName:      "ImagePullBackOff",
			Severity:        "high",
			Component:       "pod",
			Environment:     "production",
			Priority:        "P1",
			ContainerImage:  "quay.io/kubernaut-cicd/test-workflows/imagepull-fix-creds:v1.0.0",
			ContainerDigest: "", // Tag-based ref
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

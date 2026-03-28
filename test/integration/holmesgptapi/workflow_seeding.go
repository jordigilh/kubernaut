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
	"fmt"
	"io"
	"os"
	"strings"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// SeedTestWorkflowsInDataStorage seeds test workflows into DataStorage and captures UUIDs
// Pattern: DD-TEST-011 v2.0 - Shared workflow seeding library (REFACTORED)
// DD-AUTH-014: Uses authenticated DataStorage client
//
// REFACTOR: Now uses shared infrastructure.SeedWorkflowsInDataStorage()
// Eliminates ~100 lines of duplicate registration logic
//
// Returns: Map of "workflow_name:environment" → "actual-uuid-from-datastorage"
func SeedTestWorkflowsInDataStorage(client *ogenclient.Client, output io.Writer) (map[string]string, error) {
	// Convert HAPI-specific HAPIWorkflowFixture to shared infrastructure.TestWorkflow
	hapiWorkflows := GetHAPITestWorkflows()
	sharedWorkflows := make([]infrastructure.TestWorkflow, len(hapiWorkflows))
	for i, wf := range hapiWorkflows {
		sharedWorkflows[i] = infrastructure.TestWorkflow{
			WorkflowID:     wf.WorkflowName, // HAPI uses WorkflowName as WorkflowID
			Name:           wf.DisplayName,
			Description:    wf.Description,
			ActionType:     wf.ActionType, // DD-WORKFLOW-016: FK to action_type_taxonomy
			Severity:       wf.Severity,
			Component:      wf.Component,
			Environment:    wf.Environment,
			Priority:       wf.Priority,
			SchemaImage: wf.ContainerImage, // HAPI integration: Use full image ref with digest
		}
	}

	// Delegate to shared infrastructure function (eliminates OLD registerWorkflowInDataStorage)
	return infrastructure.SeedWorkflowsInDataStorage(client, sharedWorkflows, "HAPI Integration", output)
}

// REMOVED: registerWorkflowInDataStorage() - Now uses infrastructure.RegisterWorkflowInDataStorage()
// See: test/infrastructure/workflow_seeding.go for shared implementation
// Eliminated ~100 lines of duplicate code (severity enum conversion, priority enum conversion,
// OpenAPI request building, error handling, UUID extraction)

// Deprecated: WriteMockLLMConfigFile is part of the legacy ConfigMap sync infrastructure.
// The Go Mock LLM uses deterministic UUIDs (pkg/shared/uuid) and optional YAML overrides.
//
// WriteMockLLMConfigFile writes Mock LLM configuration file with workflow UUIDs
// Pattern: DD-TEST-011 v2.0 - File-Based Configuration
// Format: YAML with "workflow_name:environment" → "uuid" mappings
func WriteMockLLMConfigFile(configPath string, workflowUUIDs map[string]string, output io.Writer) error {
	_, _ = fmt.Fprintf(output, "\n📝 Writing Mock LLM configuration file: %s\n", configPath)

	// Build YAML content with deterministic key order
	var yamlContent strings.Builder
	yamlContent.WriteString("scenarios:\n")
	for _, key := range infrastructure.SortedWorkflowUUIDKeys(workflowUUIDs) {
		yamlContent.WriteString(fmt.Sprintf("  %s: %s\n", key, workflowUUIDs[key]))
	}

	// Write to file
	if err := os.WriteFile(configPath, []byte(yamlContent.String()), 0644); err != nil {
		return fmt.Errorf("failed to write Mock LLM config file: %w", err)
	}

	_, _ = fmt.Fprintf(output, "✅ Mock LLM config file written (%d scenarios)\n\n", len(workflowUUIDs))
	return nil
}

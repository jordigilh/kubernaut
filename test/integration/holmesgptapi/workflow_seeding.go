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
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// SeedTestWorkflowsInDataStorage seeds test workflows into DataStorage and captures UUIDs
// Pattern: DD-TEST-011 v2.0 - File-Based Configuration (matches AIAnalysis pattern)
// DD-AUTH-014: Uses authenticated DataStorage client
//
// Returns: Map of "workflow_name:environment" → "actual-uuid-from-datastorage"
func SeedTestWorkflowsInDataStorage(client *ogenclient.Client, output io.Writer) (map[string]string, error) {
	_, _ = fmt.Fprintf(output, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(output, "🌱 Seeding Test Workflows in DataStorage (HAPI)\n")
	_, _ = fmt.Fprintf(output, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	workflows := GetHAPITestWorkflows()
	_, _ = fmt.Fprintf(output, "📋 Registering %d test workflows...\n", len(workflows))

	// Map to store workflow_name:environment → UUID
	workflowUUIDs := make(map[string]string)

	for _, wf := range workflows {
		workflowID, err := registerWorkflowInDataStorage(client, wf, output)
		if err != nil {
			return nil, fmt.Errorf("failed to register workflow %s: %w", wf.WorkflowName, err)
		}

		// Store UUID with key format: "workflow_name:environment"
		// DD-TEST-011 v2.0: Key format matches Mock LLM scenario loading
		key := fmt.Sprintf("%s:%s", wf.WorkflowName, wf.Environment)
		workflowUUIDs[key] = workflowID

		_, _ = fmt.Fprintf(output, "  ✅ %s (%s) → %s\n", wf.WorkflowName, wf.Environment, workflowID)
	}

	_, _ = fmt.Fprintf(output, "✅ All test workflows registered (%d UUIDs captured)\n\n", len(workflowUUIDs))
	return workflowUUIDs, nil
}

// registerWorkflowInDataStorage registers a single workflow via DataStorage OpenAPI Client
// Pattern: DD-API-001 (OpenAPI Generated Client MANDATORY)
// Matches AIAnalysis pattern exactly
func registerWorkflowInDataStorage(client *ogenclient.Client, wf HAPIWorkflowFixture, output io.Writer) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Convert string severity to OpenAPI enum type
	var severity ogenclient.MandatoryLabelsSeverity
	switch wf.Severity {
	case "critical":
		severity = ogenclient.MandatoryLabelsSeverityCritical
	case "high":
		severity = ogenclient.MandatoryLabelsSeverityHigh
	case "medium":
		severity = ogenclient.MandatoryLabelsSeverityMedium
	case "low":
		severity = ogenclient.MandatoryLabelsSeverityLow
	default:
		severity = ogenclient.MandatoryLabelsSeverityCritical
	}

	// Convert string priority to OpenAPI enum type
	var priority ogenclient.MandatoryLabelsPriority
	switch wf.Priority {
	case "P0":
		priority = ogenclient.MandatoryLabelsPriority_P0
	case "P1":
		priority = ogenclient.MandatoryLabelsPriority_P1
	case "P2":
		priority = ogenclient.MandatoryLabelsPriority_P2
	case "P3":
		priority = ogenclient.MandatoryLabelsPriority_P3
	default:
		priority = ogenclient.MandatoryLabelsPriority_P2
	}

	// Build workflow request using OpenAPI generated types
	workflowReq := &ogenclient.RemediationWorkflow{
		WorkflowName:    wf.WorkflowName,
		Version:         wf.Version,
		Name:            wf.DisplayName,
		Description:     wf.Description,
		Content:         wf.ToYAMLContent(),
		ContentHash:     wf.ContentHash,
		ExecutionEngine: "tekton",
		ContainerImage:  ogenclient.NewOptString(wf.ContainerImage),
		ContainerDigest: ogenclient.NewOptString(wf.ContainerDigest),
		Labels: ogenclient.MandatoryLabels{
			SignalType:  wf.SignalType,
			Severity:    severity,
			Component:   wf.Component,
			Environment: []ogenclient.MandatoryLabelsEnvironmentItem{ogenclient.MandatoryLabelsEnvironmentItem(wf.Environment)},
			Priority:    priority,
		},
		Status: "active",
	}

	// POST to DataStorage workflow creation endpoint
	resp, err := client.CreateWorkflow(ctx, workflowReq)
	if err != nil {
		// If creation fails (likely 409 Conflict), query for existing UUID
		_, _ = fmt.Fprintf(output, "  ⚠️  Workflow may already exist (%v), querying for UUID...\n", err)
	}

	// Extract workflow_id from successful response
	if err == nil {
		switch r := resp.(type) {
		case *ogenclient.RemediationWorkflow:
			return r.WorkflowID.Value.String(), nil
		default:
			return "", fmt.Errorf("unexpected response type from CreateWorkflow: %T", resp)
		}
	}

	// For 409 Conflict or other errors, query by workflow_name to get existing UUID
	listResp, err := client.ListWorkflows(ctx, ogenclient.ListWorkflowsParams{
		WorkflowName: ogenclient.NewOptString(wf.WorkflowName),
		Limit:        ogenclient.NewOptInt(1),
	})
	if err != nil {
		return "", fmt.Errorf("failed to query existing workflow: %w", err)
	}

	// Extract workflow_id from response
	switch r := listResp.(type) {
	case *ogenclient.WorkflowListResponse:
		if len(r.Workflows) == 0 {
			return "", fmt.Errorf("workflow exists but query returned no results")
		}
		return r.Workflows[0].WorkflowID.Value.String(), nil
	default:
		return "", fmt.Errorf("unexpected response type from ListWorkflows: %T", listResp)
	}
}

// WriteMockLLMConfigFile writes Mock LLM configuration file with workflow UUIDs
// Pattern: DD-TEST-011 v2.0 - File-Based Configuration
// Format: YAML with "workflow_name:environment" → "uuid" mappings
func WriteMockLLMConfigFile(configPath string, workflowUUIDs map[string]string, output io.Writer) error {
	_, _ = fmt.Fprintf(output, "\n📝 Writing Mock LLM configuration file: %s\n", configPath)

	// Build YAML content
	var yamlContent strings.Builder
	yamlContent.WriteString("scenarios:\n")
	for key, uuid := range workflowUUIDs {
		yamlContent.WriteString(fmt.Sprintf("  %s: %s\n", key, uuid))
	}

	// Write to file
	if err := os.WriteFile(configPath, []byte(yamlContent.String()), 0644); err != nil {
		return fmt.Errorf("failed to write Mock LLM config file: %w", err)
	}

	_, _ = fmt.Fprintf(output, "✅ Mock LLM config file written (%d scenarios)\n\n", len(workflowUUIDs))
	return nil
}

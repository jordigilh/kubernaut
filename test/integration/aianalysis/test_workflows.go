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

package aianalysis

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// TestWorkflow represents a workflow for AIAnalysis integration tests
// These workflows match the Mock LLM responses to enable end-to-end testing
type TestWorkflow struct {
	WorkflowID       string // Must match Mock LLM workflow_id (e.g., "oomkill-increase-memory-v1")
	Name             string
	Description      string
	SignalType       string // Must match test scenarios (e.g., "OOMKilled")
	Severity         string
	Component        string
	Environment      string
	Priority         string
	SchemaParameters []models.WorkflowParameter // BR-HAPI-191: Must match Mock LLM parameters
}

// GetAIAnalysisTestWorkflows returns the workflows that Mock LLM expects
// These must be registered in DataStorage before tests run
//
// Pattern: Test data alignment between Mock LLM and DataStorage
//   - Mock LLM returns workflow IDs (e.g., "oomkill-increase-memory-v1")
//   - HAPI validates workflows via DataStorage API
//   - Tests fail if workflows don't exist in catalog
//   - Workflows created for BOTH staging and production environments
//     (tests use staging by default, but some use production)
func GetAIAnalysisTestWorkflows() []TestWorkflow {
	// BR-HAPI-191: SchemaParameters MUST match Mock LLM scenario parameters
	// Mock LLM scenarios defined in test/services/mock-llm/src/server.py
	// HAPI validates LLM response parameters against workflow schema from DataStorage
	// If parameters don't match, HAPI returns parameter_validation_failed BEFORE confidence check
	// DD-WORKFLOW-017: SchemaParameters mirror OCI image's /workflow-schema.yaml for documentation.
	// Actual schema comes from OCI image via pullspec-only registration.
	baseWorkflows := []TestWorkflow{
		{
			WorkflowID:  "oomkill-increase-memory-v1",
			Name:        "OOMKill Recovery - Increase Memory Limits",
			Description: "Increase memory limits for pods hitting OOMKill",
			SignalType:  "OOMKilled",
			Severity:    "critical",
			Component:   "deployment",
			Priority:    "P0",
			// Mock LLM "oomkilled" scenario returns: NAMESPACE, DEPLOYMENT_NAME, MEMORY_INCREASE_PERCENT
			SchemaParameters: []models.WorkflowParameter{
				{Name: "NAMESPACE", Type: "string", Required: true, Description: "Target namespace containing the affected deployment"},
				{Name: "DEPLOYMENT_NAME", Type: "string", Required: true, Description: "Name of the deployment to update memory limits"},
				{Name: "MEMORY_INCREASE_PERCENT", Type: "integer", Required: false, Description: "Percentage to increase memory limits by"},
			},
		},
		{
			WorkflowID:  "crashloop-config-fix-v1",
			Name:        "CrashLoopBackOff - Configuration Fix",
			Description: "Fix missing configuration causing CrashLoopBackOff",
			SignalType:  "CrashLoopBackOff",
			Severity:    "high",
			Component:   "deployment",
			Priority:    "P1",
			// Mock LLM "crashloop" scenario returns: NAMESPACE, DEPLOYMENT_NAME
			SchemaParameters: []models.WorkflowParameter{
				{Name: "NAMESPACE", Type: "string", Required: true, Description: "Target namespace"},
				{Name: "DEPLOYMENT_NAME", Type: "string", Required: true, Description: "Name of the deployment to restart"},
				{Name: "GRACE_PERIOD_SECONDS", Type: "integer", Required: false, Description: "Graceful shutdown period in seconds"},
			},
		},
		{
			WorkflowID:  "node-drain-reboot-v1",
			Name:        "NodeNotReady - Drain and Reboot",
			Description: "Drain node and reboot to resolve NodeNotReady",
			SignalType:  "NodeNotReady",
			Severity:    "critical",
			Component:   "node",
			Priority:    "P0",
			// Mock LLM "node_not_ready" scenario returns: NODE_NAME
			SchemaParameters: []models.WorkflowParameter{
				{Name: "NODE_NAME", Type: "string", Required: true, Description: "Name of the node to drain and reboot"},
				{Name: "DRAIN_TIMEOUT_SECONDS", Type: "integer", Required: false, Description: "Timeout for drain operation in seconds"},
			},
		},
		{
			WorkflowID:  "memory-optimize-v1",
			Name:        "Memory Optimization - Alternative Approach",
			Description: "Optimize memory usage after failed scaling attempt",
			SignalType:  "OOMKilled",
			Severity:    "critical",
			Component:   "deployment",
			Priority:    "P0",
			// Mock LLM "recovery" scenario returns: NAMESPACE, DEPLOYMENT_NAME
			SchemaParameters: []models.WorkflowParameter{
				{Name: "NAMESPACE", Type: "string", Required: true, Description: "Target namespace"},
				{Name: "DEPLOYMENT_NAME", Type: "string", Required: true, Description: "Name of the deployment to scale"},
				{Name: "REPLICA_COUNT", Type: "integer", Required: false, Description: "Target number of replicas"},
			},
		},
		{
			WorkflowID:  "generic-restart-v1",
			Name:        "Generic Pod Restart",
			Description: "Generic pod restart for unknown issues",
			SignalType:  "Unknown",
			Severity:    "medium",
			Component:   "deployment",
			Priority:    "P2",
			// Mock LLM "low_confidence" scenario returns: NAMESPACE, POD_NAME
			SchemaParameters: []models.WorkflowParameter{
				{Name: "NAMESPACE", Type: "string", Required: true, Description: "Target namespace"},
				{Name: "POD_NAME", Type: "string", Required: true, Description: "Name of the pod to restart"},
			},
		},
		{
			WorkflowID:  "test-signal-handler-v1",
			Name:        "Test Signal Handler",
			Description: "Generic workflow for test signals (graceful shutdown tests)",
			SignalType:  "TestSignal",
			Severity:    "critical",
			Component:   "pod",
			Priority:    "P1",
			// Mock LLM "test_signal" scenario returns: NAMESPACE, POD_NAME
			SchemaParameters: []models.WorkflowParameter{
				{Name: "NAMESPACE", Type: "string", Required: true, Description: "Target namespace"},
				{Name: "POD_NAME", Type: "string", Required: true, Description: "Name of the pod to delete"},
			},
		},
	}

	// Create workflows for staging, production, AND test environments
	// Pattern: Environment-specific workflow instances
	// - Most tests use staging (metrics_integration_test.go)
	// - Some tests use production (approval decision tests)
	// - Graceful shutdown tests use test (graceful_shutdown_test.go)
	// - DataStorage filters by environment, so we need all three
	var allWorkflows []TestWorkflow
	for _, wf := range baseWorkflows {
		// Staging version
		stagingWf := wf
		stagingWf.Environment = "staging"
		allWorkflows = append(allWorkflows, stagingWf)

		// Production version
		prodWf := wf
		prodWf.Environment = "production"
		allWorkflows = append(allWorkflows, prodWf)

		// Test version (for graceful shutdown and infrastructure tests)
		testWf := wf
		testWf.Environment = "test"
		allWorkflows = append(allWorkflows, testWf)
	}

	return allWorkflows
}

// SeedTestWorkflowsInDataStorage registers test workflows in DataStorage
// Called during SynchronizedBeforeSuite Phase 1 to prepare test data
//
// Pattern: DD-TEST-010 Multi-Controller Pattern - Shared Infrastructure Setup
// - Process 1 seeds workflows in DataStorage (shared resource)
// - All processes can reference these workflows during tests
// - Prevents "workflow not found" errors during HAPI validation
//
// Returns: map[workflow_name]workflow_id (UUID) for Mock LLM configuration
// DD-WORKFLOW-002 v3.0: DataStorage generates UUIDs (cannot be specified by client)
// DD-AUTH-014: Updated to accept authenticated client for real K8s authentication
//
// REFACTOR: Now uses shared infrastructure.SeedWorkflowsInDataStorage()
func SeedTestWorkflowsInDataStorage(client *ogenclient.Client, output io.Writer) (map[string]string, error) {
	// Convert AIAnalysis-specific TestWorkflow to shared infrastructure.TestWorkflow
	workflows := GetAIAnalysisTestWorkflows()
	sharedWorkflows := make([]infrastructure.TestWorkflow, len(workflows))
	for i, wf := range workflows {
		sharedWorkflows[i] = infrastructure.TestWorkflow{
			WorkflowID:       wf.WorkflowID,
			Name:             wf.Name,
			Description:      wf.Description,
			SignalName:       wf.SignalType,
			Severity:         wf.Severity,
			Component:        wf.Component,
			Environment:      wf.Environment,
			Priority:         wf.Priority,
			SchemaImage:     "", // AIAnalysis uses default pattern (empty = auto-generate)
			SchemaParameters: wf.SchemaParameters, // BR-HAPI-191: Pass through for HAPI validation
		}
	}

	// Delegate to shared infrastructure function
	return infrastructure.SeedWorkflowsInDataStorage(client, sharedWorkflows, "AIAnalysis Integration", output)
}

// REMOVED: registerWorkflowInDataStorage() - Now uses infrastructure.RegisterWorkflowInDataStorage()
// See: test/infrastructure/workflow_seeding.go for shared implementation

// WriteMockLLMConfigFile writes a YAML configuration file for Mock LLM
// Pattern: DD-TEST-011 v2.0 - File-Based Configuration
// Mock LLM reads workflow UUIDs from YAML file at startup (no HTTP calls)
// Input: Map of "workflow_name:environment" → "actual-uuid-from-datastorage"
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

// UpdateMockLLMWithUUIDs sends the actual workflow UUIDs to Mock LLM
// DEPRECATED: Use WriteMockLLMConfigFile for DD-TEST-011 v2.0 file-based pattern
// Pattern: DD-WORKFLOW-002 v3.0 UUID synchronization
// DataStorage auto-generates UUIDs, so Mock LLM must be updated with actual values
// This ensures LLM responses contain UUIDs that exist in DataStorage catalog
func UpdateMockLLMWithUUIDs(mockLLMConfig infrastructure.MockLLMConfig, workflowUUIDs map[string]string, output io.Writer) error {
	_, _ = fmt.Fprintf(output, "\n⚠️  DEPRECATED: UpdateMockLLMWithUUIDs() - Use WriteMockLLMConfigFile() instead\n")
	_, _ = fmt.Fprintf(output, "\n🔄 Updating Mock LLM scenarios with actual DataStorage UUIDs...\n")

	// Convert workflowUUIDs map to format Mock LLM expects
	// Input format: "workflow_name:environment" → "uuid"
	// Mock LLM format: {"workflow_name:environment": "uuid"}
	updatePayload := workflowUUIDs

	jsonPayload, err := json.Marshal(updatePayload)
	if err != nil {
		return fmt.Errorf("failed to marshal UUID update payload: %w", err)
	}

	// HTTP PUT to Mock LLM's update endpoint
	mockLLMURL := infrastructure.GetMockLLMEndpoint(mockLLMConfig)
	updateEndpoint := fmt.Sprintf("%s/api/test/update-uuids", mockLLMURL)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "PUT", updateEndpoint, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create Mock LLM update request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to update Mock LLM UUIDs: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("mock LLM returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var responseData map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&responseData); err != nil {
		return fmt.Errorf("failed to parse Mock LLM response: %w", err)
	}

	updatedCount, _ := responseData["updated_scenarios"].(float64)
	_, _ = fmt.Fprintf(output, "✅ Mock LLM updated: %d scenarios synchronized with DataStorage UUIDs\n", int(updatedCount))

	return nil
}

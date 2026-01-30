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
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// TestWorkflow represents a workflow for AIAnalysis integration tests
// These workflows match the Mock LLM responses to enable end-to-end testing
type TestWorkflow struct {
	WorkflowID  string // Must match Mock LLM workflow_id (e.g., "oomkill-increase-memory-v1")
	Name        string
	Description string
	SignalType  string // Must match test scenarios (e.g., "OOMKilled")
	Severity    string
	Component   string
	Environment string
	Priority    string
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
	baseWorkflows := []TestWorkflow{
		{
			WorkflowID:  "oomkill-increase-memory-v1",
			Name:        "OOMKill Recovery - Increase Memory Limits",
			Description: "Increase memory limits for pods hitting OOMKill",
			SignalType:  "OOMKilled",
			Severity:    "critical",
			Component:   "deployment",
			Priority:    "P0",
		},
		{
			WorkflowID:  "crashloop-config-fix-v1",
			Name:        "CrashLoopBackOff - Configuration Fix",
			Description: "Fix missing configuration causing CrashLoopBackOff",
			SignalType:  "CrashLoopBackOff",
			Severity:    "high",
			Component:   "deployment",
			Priority:    "P1",
		},
		{
			WorkflowID:  "node-drain-reboot-v1",
			Name:        "NodeNotReady - Drain and Reboot",
			Description: "Drain node and reboot to resolve NodeNotReady",
			SignalType:  "NodeNotReady",
			Severity:    "critical",
			Component:   "node",
			Priority:    "P0",
		},
		{
			WorkflowID:  "memory-optimize-v1",
			Name:        "Memory Optimization - Alternative Approach",
			Description: "Optimize memory usage after failed scaling attempt",
			SignalType:  "OOMKilled",
			Severity:    "critical",
			Component:   "deployment",
			Priority:    "P0",
		},
		{
			WorkflowID:  "generic-restart-v1",
			Name:        "Generic Pod Restart",
			Description: "Generic pod restart for unknown issues",
			SignalType:  "Unknown",
			Severity:    "medium",
			Component:   "deployment",
			Priority:    "P2",
		},
		{
			WorkflowID:  "test-signal-handler-v1",
			Name:        "Test Signal Handler",
			Description: "Generic workflow for test signals (graceful shutdown tests)",
			SignalType:  "TestSignal",
			Severity:    "critical",
			Component:   "pod",
			Priority:    "P1",
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
func SeedTestWorkflowsInDataStorage(client *ogenclient.Client, output io.Writer) (map[string]string, error) {
	_, _ = fmt.Fprintf(output, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(output, "🌱 Seeding Test Workflows in DataStorage\n")
	_, _ = fmt.Fprintf(output, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	workflows := GetAIAnalysisTestWorkflows()
	_, _ = fmt.Fprintf(output, "📋 Registering %d test workflows (staging + production + test)...\n", len(workflows))

	// Map to store workflow_name → workflow_id (UUID)
	workflowUUIDs := make(map[string]string)

	for _, wf := range workflows {
		workflowID, err := registerWorkflowInDataStorage(client, wf, output)
		if err != nil {
			return nil, fmt.Errorf("failed to register workflow %s: %w", wf.WorkflowID, err)
		}

		// Store the UUID for this workflow (keyed by workflow_name + environment)
		// Format: "workflow_name:environment" → "uuid"
		key := fmt.Sprintf("%s:%s", wf.WorkflowID, wf.Environment)
		workflowUUIDs[key] = workflowID

		_, _ = fmt.Fprintf(output, "  ✅ %s (%s) → %s\n", wf.WorkflowID, wf.Environment, workflowID)
	}

	_, _ = fmt.Fprintf(output, "✅ All test workflows registered (%d UUIDs captured)\n\n", len(workflowUUIDs))
	return workflowUUIDs, nil
}

// registerWorkflowInDataStorage registers a single workflow via DataStorage OpenAPI Client
// Pattern: DD-API-001 (OpenAPI Generated Client MANDATORY)
// Authority: .cursor/rules/* - All Go services must use OpenAPI clients
// DD-WORKFLOW-002 v3.0: DataStorage generates UUID (security - cannot be specified by client)
// Returns the actual UUID assigned by DataStorage
// DD-AUTH-014: Updated to accept authenticated client instead of creating unauthenticated one
func registerWorkflowInDataStorage(client *ogenclient.Client, wf TestWorkflow, output io.Writer) (string, error) {
	version := "1.0.0"
	content := fmt.Sprintf("# Test workflow %s\nversion: %s\ndescription: %s", wf.WorkflowID, version, wf.Description)
	contentBytes := []byte(content)
	hash := sha256.Sum256(contentBytes)
	contentHash := fmt.Sprintf("%x", hash)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// DD-WORKFLOW-002 v3.0: workflow_id is NOT included in request (generated by DataStorage)
	// Security: Prevents ID collision attacks by letting DataStorage control UUID generation
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
		severity = ogenclient.MandatoryLabelsSeverityCritical // Default to critical
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
		priority = ogenclient.MandatoryLabelsPriority_P2 // Default to P2
	}

	// Build workflow request using OpenAPI generated types (compile-time validation)
	workflowReq := &ogenclient.RemediationWorkflow{
		// Note: WorkflowID is NOT set - DataStorage auto-generates it
		WorkflowName:    wf.WorkflowID, // Human-readable identifier (workflow_name field)
		Version:         version,
		Name:            wf.Name,
		Description:     wf.Description,
		Content:         content,
		ContentHash:     contentHash,
		ExecutionEngine: "tekton",
		ContainerImage:  ogenclient.NewOptString(fmt.Sprintf("quay.io/jordigilh/test-workflows/%s:%s", wf.WorkflowID, version)),
		Labels: ogenclient.MandatoryLabels{
			SignalType:  wf.SignalType,
			Severity:    severity,
			Component:   wf.Component,
			Environment: []ogenclient.MandatoryLabelsEnvironmentItem{ogenclient.MandatoryLabelsEnvironmentItem(wf.Environment)}, // Model 2: wrap string in array
			Priority:    priority,
		},
		Status: "active",
	}

	// POST to DataStorage workflow creation endpoint
	// BR-STORAGE-014: Workflow catalog management
	resp, err := client.CreateWorkflow(ctx, workflowReq)
	if err != nil {
		// DD-WORKFLOW-002 v3.0: If creation fails (likely 409 Conflict), query for existing UUID
		// OpenAPI client returns errors for non-2xx responses
		// We handle this by falling through to query logic - idempotent workflow registration
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
	// Authority: DD-WORKFLOW-002 v3.0 (UUID primary key, workflow_name is metadata)
	// This is idempotent - safe to call in tests even if workflow exists
	// DD-API-001: Use OpenAPI client (added workflow_name filter to listWorkflows endpoint)
	listResp, err := client.ListWorkflows(ctx, ogenclient.ListWorkflowsParams{
		WorkflowName: ogenclient.NewOptString(wf.WorkflowID),
		Limit:        ogenclient.NewOptInt(1), // Only need first match
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
		return fmt.Errorf("Mock LLM returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var responseData map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&responseData); err != nil {
		return fmt.Errorf("failed to parse Mock LLM response: %w", err)
	}

	updatedCount, _ := responseData["updated_scenarios"].(float64)
	_, _ = fmt.Fprintf(output, "✅ Mock LLM updated: %d scenarios synchronized with DataStorage UUIDs\n", int(updatedCount))

	return nil
}

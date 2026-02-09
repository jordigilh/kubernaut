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

package infrastructure

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"strings"
	"time"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// TestWorkflow represents a workflow for test seeding in DataStorage
// Pattern: Shared data structure for AIAnalysis integration tests and HAPI E2E tests
// This struct consolidates workflow definitions from both test suites
type TestWorkflow struct {
	WorkflowID      string // Must match Mock LLM workflow_id or Python fixture workflow_name
	Name            string
	Description     string
	SignalType      string // Must match test scenarios (e.g., "OOMKilled")
	Severity        string // "critical", "high", "medium", "low"
	Component       string // "deployment", "pod", "node", etc.
	Environment     string // "staging", "production", "test"
	Priority        string // "P0", "P1", "P2", "P3"
	ContainerImage  string // Full image ref with optional digest (e.g., "ghcr.io/org/image:tag@sha256:...")
	ExecutionEngine string // "tekton" or "job" - defaults to "tekton" if empty (BR-WE-014)
}

// SeedWorkflowsInDataStorage registers test workflows in DataStorage
// Called during test suite setup (e.g., SynchronizedBeforeSuite Phase 1)
//
// Pattern: DD-TEST-010 Multi-Controller Pattern - Shared Infrastructure Setup
// - Process 1 seeds workflows in DataStorage (shared resource)
// - All processes can reference these workflows during tests
// - Prevents "workflow not found" errors during HAPI validation
//
// Pattern: DD-TEST-011 v2.0 - Go-based workflow seeding
// - Prevents pytest-xdist race conditions (BR-TEST-008)
// - Prevents TokenReview rate limiting under concurrent access
//
// Returns: map[workflow_name]workflow_id (UUID) for test reference
// DD-WORKFLOW-002 v3.0: DataStorage generates UUIDs (cannot be specified by client)
// DD-AUTH-014: Accepts authenticated client for real K8s authentication
func SeedWorkflowsInDataStorage(client *ogenclient.Client, workflows []TestWorkflow, testSuiteName string, output io.Writer) (map[string]string, error) {
	_, _ = fmt.Fprintf(output, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(output, "🌱 Seeding Test Workflows in DataStorage (%s)\n", testSuiteName)
	_, _ = fmt.Fprintf(output, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	_, _ = fmt.Fprintf(output, "📋 Registering %d test workflows...\n", len(workflows))

	// Map to store workflow_name → workflow_id (UUID)
	workflowUUIDs := make(map[string]string)

	for _, wf := range workflows {
		workflowID, err := RegisterWorkflowInDataStorage(client, wf, output)
		if err != nil {
			return nil, fmt.Errorf("failed to register workflow %s: %w", wf.WorkflowID, err)
		}

		// Store the UUID for this workflow (keyed by workflow_name + environment)
		// Format: "workflow_name:environment" → "uuid"
		// Some tests only use workflow_name, others use the full key
		key := fmt.Sprintf("%s:%s", wf.WorkflowID, wf.Environment)
		workflowUUIDs[key] = workflowID

		_, _ = fmt.Fprintf(output, "  ✅ %s (%s) → %s\n", wf.WorkflowID, wf.Environment, workflowID)
	}

	_, _ = fmt.Fprintf(output, "✅ All test workflows registered (%d UUIDs captured)\n\n", len(workflowUUIDs))
	return workflowUUIDs, nil
}

// RegisterWorkflowInDataStorage registers a single workflow via DataStorage OpenAPI Client
// Pattern: DD-API-001 (OpenAPI Generated Client MANDATORY)
// Authority: .cursor/rules/* - All Go services must use OpenAPI clients
//
// DD-WORKFLOW-002 v3.0: DataStorage generates UUID (security - cannot be specified by client)
// DD-AUTH-014: Accepts authenticated client instead of creating unauthenticated one
//
// This function is idempotent - safe to call multiple times for the same workflow
// If workflow already exists (409 Conflict), it queries DataStorage to retrieve the existing UUID
//
// Returns: The actual UUID assigned by DataStorage (either from creation or query)
func RegisterWorkflowInDataStorage(client *ogenclient.Client, wf TestWorkflow, output io.Writer) (string, error) {
	version := "1.0.0"
	content := fmt.Sprintf("# Test workflow %s\nversion: %s\ndescription: %s", wf.WorkflowID, version, wf.Description)
	contentBytes := []byte(content)
	hash := sha256.Sum256(contentBytes)
	contentHash := fmt.Sprintf("%x", hash)

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

	// Handle container image: use provided value or generate default pattern
	containerImage := wf.ContainerImage
	if containerImage == "" {
		// Default pattern for tests that don't specify a container image
		containerImage = fmt.Sprintf("quay.io/jordigilh/test-workflows/%s:%s", wf.WorkflowID, version)
	}

	// Extract container_digest from container_image if present
	// Pattern: Matches Python workflow_fixtures.py logic (line 91-94)
	// BR-AI-075: Container digest field for audit trail
	// Example: "ghcr.io/kubernaut/workflows/oomkill:v1.0.0@sha256:abc123..." → "sha256:abc123..."
	var containerDigest ogenclient.OptString
	if strings.Contains(containerImage, "@sha256:") {
		parts := strings.Split(containerImage, "@")
		if len(parts) == 2 {
			// Found digest: image@sha256:abc123...
			containerDigest = ogenclient.NewOptString(parts[1]) // "sha256:abc123..."
		}
	}

	// BR-WE-014: Use ExecutionEngine from TestWorkflow, default to "tekton" for backwards compatibility
	executionEngine := wf.ExecutionEngine
	if executionEngine == "" {
		executionEngine = "tekton"
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
		ExecutionEngine: executionEngine,
		ContainerImage:  ogenclient.NewOptString(containerImage),
		ContainerDigest: containerDigest, // BR-AI-075: Extract from ContainerImage
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
		case *ogenclient.CreateWorkflowConflict:
			// DS-BUG-001: 409 Conflict - workflow already exists
			// Fall through to query logic below to retrieve existing UUID
			// This maintains idempotency for test workflows
			_, _ = fmt.Fprintf(output, "  ⚠️  Workflow already exists (409 Conflict), querying for UUID...\n")
		default:
			return "", fmt.Errorf("unexpected response type from CreateWorkflow: %T", resp)
		}
	}

	// For 409 Conflict or other errors, query by workflow_name to get existing UUID
	// Authority: DD-WORKFLOW-002 v3.0 (UUID primary key, workflow_name is metadata)
	// This is idempotent - safe to call in tests even if workflow exists
	// DD-API-001: Use OpenAPI client (workflow_name filter in listWorkflows endpoint)
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

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
	"fmt"
	"io"
	"regexp"
	"time"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)


// workflowIDToImageName maps WorkflowID to OCI image name.
// WorkflowIDs include version suffix (e.g., oomkill-increase-memory-v1) but fixture
// directories and built images use base names (oomkill-increase-memory). Makefile
// build-test-workflows uses basename of fixture dir, so we strip -vN suffix.
var workflowVersionSuffix = regexp.MustCompile(`-v\d+$`)

func workflowIDToImageName(workflowID string) string {
	return workflowVersionSuffix.ReplaceAllString(workflowID, "")
}

// TestWorkflow represents a workflow for test seeding in DataStorage
// Pattern: Shared data structure for AIAnalysis integration tests and HAPI E2E tests
// This struct consolidates workflow definitions from both test suites
type TestWorkflow struct {
	WorkflowID      string // Must match Mock LLM workflow_id or Python fixture workflow_name
	Name            string
	Description     string
	ActionType      string // DD-WORKFLOW-016: FK to action_type_taxonomy (e.g., "ScaleReplicas", "RestartPod")
	SignalType      string // Must match test scenarios (e.g., "OOMKilled")
	Severity        string // "critical", "high", "medium", "low"
	Component       string // "deployment", "pod", "node", etc.
	Environment     string // "staging", "production", "test"
	Priority        string // "P0", "P1", "P2", "P3"
	ContainerImage  string // Full image ref with optional digest (e.g., "ghcr.io/org/image:tag@sha256:...")
	ExecutionEngine string // "tekton" or "job" - defaults to "tekton" if empty (BR-WE-014)
	// SchemaParameters defines workflow input parameters per ADR-043 (BR-HAPI-191)
	// Used to generate valid workflow-schema.yaml content that DataStorage will parse
	// and store in the parameters JSONB column for HAPI validation and MCP tool results
	SchemaParameters []models.WorkflowParameter
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
// DD-WORKFLOW-017: Pullspec-only registration — sends only containerImage.
// DataStorage pulls the image, extracts /workflow-schema.yaml, and populates all fields.
//
// DD-WORKFLOW-002 v3.0: DataStorage generates UUID (security - cannot be specified by client)
// DD-AUTH-014: Accepts authenticated client instead of creating unauthenticated one
//
// This function is idempotent - safe to call multiple times for the same workflow
// If workflow already exists (409 Conflict), it queries DataStorage to retrieve the existing UUID
//
// Returns: The actual UUID assigned by DataStorage (either from creation or query)
func RegisterWorkflowInDataStorage(client *ogenclient.Client, wf TestWorkflow, output io.Writer) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Handle container image: use provided value or generate default pattern
	containerImage := wf.ContainerImage
	if containerImage == "" {
		// Default pattern for tests that don't specify a container image
		containerImage = fmt.Sprintf("quay.io/kubernaut-cicd/test-workflows/%s:v1.0.0", workflowIDToImageName(wf.WorkflowID))
	}

	// DD-WORKFLOW-017: Pullspec-only registration request
	workflowReq := &ogenclient.CreateWorkflowFromOCIRequest{
		ContainerImage: containerImage,
	}

	// POST to DataStorage workflow creation endpoint
	resp, err := client.CreateWorkflow(ctx, workflowReq)
	if err != nil {
		// DD-WORKFLOW-002 v3.0: If creation fails (likely 409 Conflict), query for existing UUID
		_, _ = fmt.Fprintf(output, "  ⚠️  Workflow may already exist (%v), querying for UUID...\n", err)
	}

	// Extract workflow_id from successful response
	if err == nil {
		switch r := resp.(type) {
		case *ogenclient.RemediationWorkflow:
			return r.WorkflowID.Value.String(), nil
		case *ogenclient.CreateWorkflowConflict:
			// DS-BUG-001: 409 Conflict - workflow already exists
			_, _ = fmt.Fprintf(output, "  ⚠️  Workflow already exists (409 Conflict), querying for UUID...\n")
		default:
			return "", fmt.Errorf("unexpected response type from CreateWorkflow: %T", resp)
		}
	}

	// For 409 Conflict or other errors, query by workflow_name to get existing UUID
	listResp, err := client.ListWorkflows(ctx, ogenclient.ListWorkflowsParams{
		WorkflowName: ogenclient.NewOptString(wf.WorkflowID),
		Limit:        ogenclient.NewOptInt(1),
	})
	if err != nil {
		return "", fmt.Errorf("failed to query existing workflow: %w", err)
	}

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

// Note: buildWorkflowSchemaContent removed — DD-WORKFLOW-017 pullspec-only registration
// means DataStorage extracts schema from OCI image, not from client-provided content.

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
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)


// workflowIDToImageName maps WorkflowID to fixture directory name.
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
//
// ADR-058: Registration uses inline CRD YAML content read from fixture files.
// DataStorage parses the CRD envelope to populate labels (severity, environment, etc.).
// The string metadata fields below (Severity, Component, Environment, Priority) are
// NOT sent to the API; they serve as human-readable documentation and as key
// components for workflowUUIDs map lookups (key format: "workflowID:environment").
type TestWorkflow struct {
	WorkflowID      string // Must match Mock LLM workflow_id or Python fixture workflow_name
	Name            string
	Description     string
	ActionType      string // DD-WORKFLOW-016: FK to action_type_taxonomy (e.g., "ScaleReplicas", "RestartPod")
	Severity        string // Metadata only: "critical", "high", "medium", "low" (actual value from fixture)
	Component       string // Metadata only: "deployment", "pod", "node", etc. (actual value from fixture)
	Environment     string // Metadata only + map key: "staging", "production", "test" (actual value from fixture)
	Priority        string // Metadata only: "P0", "P1", "P2", "P3" (actual value from fixture)
	SchemaImage     string // Legacy: retained for fixture directory name mapping
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

// RegisterWorkflowInDataStorage registers a single workflow via DataStorage OpenAPI Client.
// ADR-058: Inline schema registration — reads CRD YAML from test fixtures and posts inline.
//
// DD-WORKFLOW-002 v3.0: DataStorage generates UUID (security - cannot be specified by client)
// DD-AUTH-014: Accepts authenticated client instead of creating unauthenticated one
//
// This function is idempotent - safe to call multiple times for the same workflow.
// If the DS returns 409 Conflict, it falls back to a ListWorkflows query to retrieve the
// existing UUID. For any other error (400, 401, 403, 500, transport), the original error
// is returned immediately — no misleading fallback.
//
// Returns: The actual UUID assigned by DataStorage (either from creation or query)
func RegisterWorkflowInDataStorage(client *ogenclient.Client, wf TestWorkflow, output io.Writer) (string, error) {
	fixtureDir := workflowIDToImageName(wf.WorkflowID)
	content, readErr := readWorkflowFixtureContent(fixtureDir)
	if readErr != nil {
		return "", fmt.Errorf("read fixture for %s: %w", wf.WorkflowID, readErr)
	}

	uuid, _, err := callCreateWorkflowInline(client, content, "e2e-test-seeder")
	if err == nil {
		return uuid, nil
	}

	// Only fall back to ListWorkflows for 409 Conflict (workflow already exists).
	// All other errors (400 bad request, 500 internal, transport, etc.) are fatal.
	var ce *workflowConflictError
	if !errors.As(err, &ce) {
		return "", fmt.Errorf("register workflow %s: %w", wf.WorkflowID, err)
	}

	_, _ = fmt.Fprintf(output, "  ⚠️  Workflow already exists (409), querying for existing UUID...\n")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	listResp, listErr := client.ListWorkflows(ctx, ogenclient.ListWorkflowsParams{
		WorkflowName: ogenclient.NewOptString(wf.WorkflowID),
		Limit:        ogenclient.NewOptInt(1),
	})
	if listErr != nil {
		return "", fmt.Errorf("failed to query existing workflow %s (after 409): %w", wf.WorkflowID, listErr)
	}

	switch r := listResp.(type) {
	case *ogenclient.WorkflowListResponse:
		if len(r.Workflows) == 0 {
			return "", fmt.Errorf("registration conflict but no existing workflow found (name=%s): %w", wf.WorkflowID, err)
		}
		return r.Workflows[0].WorkflowId.Value.String(), nil
	default:
		return "", fmt.Errorf("unexpected response type from ListWorkflows: %T", listResp)
	}
}

// SortedWorkflowUUIDKeys returns the keys of a workflowUUIDs map sorted so that
// ":production" entries come after all other environments for the same workflow name.
// This is critical because the Mock LLM's load_scenarios_from_file does last-write-wins
// when the same workflow_name matches multiple config entries. Without deterministic
// ordering, Go's randomized map iteration can cause the Mock LLM to load a staging or
// test UUID instead of the production UUID that tests assert against.
func SortedWorkflowUUIDKeys(workflowUUIDs map[string]string) []string {
	keys := make([]string, 0, len(workflowUUIDs))
	for k := range workflowUUIDs {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		iProd := strings.HasSuffix(keys[i], ":production")
		jProd := strings.HasSuffix(keys[j], ":production")
		if iProd != jProd {
			return !iProd
		}
		return keys[i] < keys[j]
	})
	return keys
}

// Note: buildWorkflowSchemaContent removed — ADR-058 inline registration reads
// content directly from fixture YAML files via readWorkflowFixtureContent.

// WorkflowSeedSpec describes a workflow fixture to seed via kubectl apply.
type WorkflowSeedSpec struct {
	FixtureDir  string // fixture directory name under test/fixtures/workflows/ (e.g., "crashloop-config-fix")
	Environment string // environment label for the workflowUUIDs map key (e.g., "production")
}

// SeedWorkflowsViaKubectlApply registers workflows declaratively using kubectl apply -f,
// which triggers the authwebhook → DataStorage registration → CRD status update pipeline.
// This mirrors production deployments where operators apply CRD manifests directly.
//
// For each workflow fixture, the function:
//  1. Reads the fixture YAML from test/fixtures/workflows/<dir>/workflow-schema.yaml
//  2. Applies it via kubectl apply -f - -n <namespace>
//  3. Polls until .status.workflowId is populated (authwebhook async update)
//  4. Returns map["<crd-name>:<environment>"] = "<uuid>"
//
// Prerequisites: AuthWebhook deployed, DataStorage healthy, ActionTypes seeded.
func SeedWorkflowsViaKubectlApply(kubeconfigPath, namespace string, workflows []WorkflowSeedSpec, output io.Writer) (map[string]string, error) {
	_, _ = fmt.Fprintf(output, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(output, "🌱 Seeding %d workflows via kubectl apply (declarative)\n", len(workflows))
	_, _ = fmt.Fprintf(output, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	workflowUUIDs := make(map[string]string)
	var appliedNames []string

	for _, wf := range workflows {
		content, err := readWorkflowFixtureContent(wf.FixtureDir)
		if err != nil {
			return nil, fmt.Errorf("read fixture %s: %w", wf.FixtureDir, err)
		}

		cmd := exec.Command("kubectl", "apply",
			"--kubeconfig", kubeconfigPath,
			"-n", namespace,
			"-f", "-")
		cmd.Stdin = strings.NewReader(content)

		cmdOutput, err := cmd.CombinedOutput()
		if err != nil {
			_, _ = fmt.Fprintf(output, "  ❌ %s: %s\n", wf.FixtureDir, cmdOutput)
			return nil, fmt.Errorf("kubectl apply for %s: %w", wf.FixtureDir, err)
		}

		name := extractCRDName(content)
		if name == "" {
			return nil, fmt.Errorf("could not extract metadata.name from fixture %s", wf.FixtureDir)
		}

		appliedNames = append(appliedNames, name)
		key := fmt.Sprintf("%s:%s", name, wf.Environment)
		workflowUUIDs[key] = "" // placeholder until UUID is resolved

		_, _ = fmt.Fprintf(output, "  ✅ Applied: %s (waiting for UUID...)\n", name)
	}

	_, _ = fmt.Fprintf(output, "\n⏳ Waiting for authwebhook to populate .status.workflowId...\n")

	for i, wf := range workflows {
		name := appliedNames[i]
		key := fmt.Sprintf("%s:%s", name, wf.Environment)

		uuid, err := waitForWorkflowUUID(kubeconfigPath, namespace, name, 90*time.Second)
		if err != nil {
			return nil, fmt.Errorf("workflow %s UUID not populated: %w", name, err)
		}
		workflowUUIDs[key] = uuid
		_, _ = fmt.Fprintf(output, "  ✅ %s → %s\n", name, uuid)
	}

	_, _ = fmt.Fprintf(output, "✅ All workflows seeded via kubectl apply (%d UUIDs captured)\n\n", len(workflowUUIDs))
	return workflowUUIDs, nil
}

// waitForWorkflowUUID polls a RemediationWorkflow's .status.workflowId until it
// is non-empty or the timeout expires.
func waitForWorkflowUUID(kubeconfigPath, namespace, name string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		cmd := exec.Command("kubectl", "get",
			fmt.Sprintf("remediationworkflow/%s", name),
			"-n", namespace,
			"--kubeconfig", kubeconfigPath,
			"-o", "jsonpath={.status.workflowId}")

		out, err := cmd.Output()
		if err == nil {
			uuid := strings.TrimSpace(string(out))
			if uuid != "" && uuid != "{}" {
				return uuid, nil
			}
		}
		time.Sleep(2 * time.Second)
	}
	return "", fmt.Errorf("timed out after %s waiting for .status.workflowId on %s/%s", timeout, namespace, name)
}

// extractCRDName parses metadata.name from a YAML manifest.
func extractCRDName(yamlContent string) string {
	inMetadata := false
	for _, line := range strings.Split(yamlContent, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "metadata:" {
			inMetadata = true
			continue
		}
		if inMetadata && strings.HasPrefix(trimmed, "name:") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "name:"))
		}
		if inMetadata && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && trimmed != "" {
			break
		}
	}
	return ""
}

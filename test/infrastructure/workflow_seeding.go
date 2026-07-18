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
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
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
// Pattern: Shared data structure for AIAnalysis integration tests and KA E2E tests
// This struct consolidates workflow definitions from both test suites
//
// ADR-058: Registration uses inline CRD YAML content read from fixture files.
// DataStorage parses the CRD envelope to populate labels (severity, environment, etc.).
// The string metadata fields below (Severity, Environment, Priority) and the
// component slice (mirroring workflow schema labels) are
// NOT sent to the API; they serve as human-readable documentation and as key
// components for workflowUUIDs map lookups (key format: "workflowID:environment").
type TestWorkflow struct {
	WorkflowID      string // Must match Mock LLM workflow_id in scenarios registry
	Name            string
	Description     string
	ActionType      string // DD-WORKFLOW-016: FK to action_type_taxonomy (e.g., "ScaleReplicas", "RestartPod")
	Severity        string // Metadata only: "critical", "high", "warning", "info" (ADR-066 canonical values)
	Component       []string // Metadata only: e.g. []string{"deployment"}, []string{"pod"} (actual value from fixture)
	Environment     string // Metadata only + map key: "staging", "production", "test" (actual value from fixture)
	Priority        string // Metadata only: "P0", "P1", "P2", "P3" (actual value from fixture)
	SchemaImage     string // Legacy: retained for fixture directory name mapping
	ExecutionEngine string // "tekton" or "job" - defaults to "tekton" if empty (BR-WE-014)
	// SchemaParameters defines workflow input parameters per ADR-043 (BR-HAPI-191)
	// Used to generate valid workflow-schema.yaml content that DataStorage will parse
	// and store in the parameters JSONB column for KA validation and MCP tool results
	SchemaParameters []models.WorkflowParameter
}

// Deprecated: SortedWorkflowUUIDKeys is part of the legacy ConfigMap sync infrastructure.
// The Go Mock LLM uses deterministic UUIDs and no longer requires external UUID synchronization.
//
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

// testWorkflowsToSeedSpecs converts legacy TestWorkflow entries (the
// Postgres-inline-registration shape) into WorkflowSeedSpecs for
// SeedWorkflowsViaKubectlApply (Issue #1661 Phase 55: E2E suites move off
// the Postgres-backed SeedWorkflowsInDataStorage path onto AuthWebhook's
// real admission pipeline). FixtureDir is derived the same way
// RegisterWorkflowInDataStorage does (workflowIDToImageName strips any
// "-vN" version suffix from WorkflowID) -- every fixture's metadata.name
// equals its TestWorkflow.WorkflowID (verified across all callers), so the
// resulting map key (name:environment) is identical to the retired path's
// key (WorkflowID:environment).
func testWorkflowsToSeedSpecs(workflows []TestWorkflow) []WorkflowSeedSpec {
	specs := make([]WorkflowSeedSpec, 0, len(workflows))
	for _, wf := range workflows {
		specs = append(specs, WorkflowSeedSpec{
			FixtureDir:  workflowIDToImageName(wf.WorkflowID),
			Environment: wf.Environment,
		})
	}
	return specs
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

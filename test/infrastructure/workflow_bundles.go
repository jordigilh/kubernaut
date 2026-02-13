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
	"net/http"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
	"gopkg.in/yaml.v3"
)

// Workflow Bundle Infrastructure for WorkflowExecution E2E Tests
//
// This implements the production-like workflow registration flow:
// 1. Build Tekton Pipeline as OCI bundle (tkn bundle push)
// 2. Load bundle OCI image into Kind cluster
// 3. Register workflow in DataStorage (POST /api/v1/workflows)
// 4. WorkflowExecution references the bundle via container_image field
//
// **Design Pattern**: Mirrors production workflow authoring flow
// - Operators author Tekton Pipelines and package as OCI bundles
// - Workflows are registered in DataStorage for AI Analysis catalog
// - WorkflowExecution controller resolves bundles via Tekton bundle resolver

const (
	// TestWorkflowBundleRegistry is the OCI registry for test workflow bundles
	// Uses quay.io/kubernaut-cicd namespace (multi-arch support: amd64 + arm64)
	TestWorkflowBundleRegistry = "quay.io/kubernaut-cicd/test-workflows"

	// TestWorkflowBundleVersion is the version tag for E2E test bundles
	TestWorkflowBundleVersion = "v1.0.0"
)

// BuildAndRegisterTestWorkflows registers test workflows in DataStorage
// This creates the workflow catalog needed for E2E tests:
// - test-hello-world: Successful execution test workflow
// - test-intentional-failure: Failure handling test workflow
//
// **Bundle Strategy**: Uses pre-built multi-arch bundles from quay.io/kubernaut-cicd
// - Bundles are built manually and pushed to quay.io (amd64 + arm64)
// - Tekton pulls bundles directly from quay.io at runtime (no pre-loading needed)
// - No dynamic bundle building in CI/CD (simplifies pipeline)
//
// Returns the registered workflow bundle references for use in WorkflowExecution specs.
func BuildAndRegisterTestWorkflows(clusterName, kubeconfigPath, dataStorageURL, saToken string, output io.Writer) (map[string]string, error) {
	_, _ = fmt.Fprintf(output, "\n📦 Setting up test workflows from %s...\n", TestWorkflowBundleRegistry)

	bundles := make(map[string]string)

	// Use pre-built bundles from quay.io (multi-arch: amd64 + arm64)
	// Tekton's bundle resolver will pull these directly at runtime
	helloWorldRef := fmt.Sprintf("%s/hello-world:%s", TestWorkflowBundleRegistry, TestWorkflowBundleVersion)
	failingRef := fmt.Sprintf("%s/failing:%s", TestWorkflowBundleRegistry, TestWorkflowBundleVersion)

	bundles["test-hello-world"] = helloWorldRef
	bundles["test-intentional-failure"] = failingRef

	_, _ = fmt.Fprintf(output, "  ✅ Using bundles from %s\n", TestWorkflowBundleRegistry)

	// Register workflows in DataStorage using OpenAPI client (DD-API-001)
	_, _ = fmt.Fprintf(output, "\n📝 Registering workflows in DataStorage...\n")
	if err := registerTestBundleWorkflow(
		dataStorageURL,
		saToken,
		"test-hello-world",
		"v1.0.0",
		bundles["test-hello-world"],
		"Simple hello-world workflow for E2E testing",
		output,
	); err != nil {
		return nil, fmt.Errorf("failed to register hello-world workflow: %w", err)
	}

	if err := registerTestBundleWorkflow(
		dataStorageURL,
		saToken,
		"test-intentional-failure",
		"v1.0.0",
		bundles["test-intentional-failure"],
		"Intentionally failing workflow for E2E failure handling tests",
		output,
	); err != nil {
		return nil, fmt.Errorf("failed to register failing workflow: %w", err)
	}

	_, _ = fmt.Fprintf(output, "✅ Test workflows ready\n")
	return bundles, nil
}

// registerTestBundleWorkflow registers a workflow in DataStorage using OpenAPI client
// POST /api/v1/workflows per DD-WORKFLOW-005 v1.0
// Uses dsgen.RemediationWorkflow OpenAPI types for compile-time type safety (DD-API-001)
// Includes DD-AUTH-014 ServiceAccount authentication
func registerTestBundleWorkflow(dataStorageURL, saToken, workflowName, version, containerImage, description string, output io.Writer) error {
	_, _ = fmt.Fprintf(output, "  Registering: %s (version %s)\n", workflowName, version)

	// ADR-043: Generate valid workflow-schema.yaml content
	// DataStorage's HandleCreateWorkflow will parse and validate this (BR-HAPI-191)
	schema := models.WorkflowSchema{
		APIVersion: "kubernaut.io/v1alpha1",
		Kind:       "WorkflowSchema",
		Metadata: models.WorkflowSchemaMetadata{
			WorkflowID:  workflowName,
			Version:     version,
			Description: description,
		},
		Labels: models.WorkflowSchemaLabels{
			SignalType:    "test-signal",
			Severity:      "low",
			RiskTolerance: "high",
			Environment:   "test",
			Component:     "deployment",
			Priority:      "p3",
		},
		Execution: &models.WorkflowExecution{
			Engine: "tekton",
		},
		Parameters: []models.WorkflowParameter{
			{
				Name:        "TARGET_RESOURCE",
				Type:        "string",
				Required:    true,
				Description: "Target resource being remediated",
			},
		},
	}
	yamlBytes, err := yaml.Marshal(&schema)
	if err != nil {
		return fmt.Errorf("failed to generate workflow-schema.yaml for %s: %w", workflowName, err)
	}
	content := string(yamlBytes)
	contentBytes := []byte(content)
	hash := sha256.Sum256(contentBytes)
	contentHash := fmt.Sprintf("%x", hash)

	// Build payload using OpenAPI client types (DD-API-001)
	// CRITICAL: Environment must be []dsgen.MandatoryLabelsEnvironmentItem, not string!
	workflow := dsgen.RemediationWorkflow{
		WorkflowName:    workflowName,
		ActionType:      "ScaleReplicas", // DD-WORKFLOW-016: Default action type for bundle test workflows
		Version:         version,
		Name:            fmt.Sprintf("Test Workflow: %s", workflowName),
		Description:     description,
		Content:         content,
		ContentHash:     contentHash,
		ExecutionEngine: "tekton", // String field, not enum
		ContainerImage:  dsgen.NewOptString(containerImage),
		Labels: dsgen.MandatoryLabels{
			SignalType:  "test-signal",
			Severity:    dsgen.MandatoryLabelsSeverity_low,
			Component:   "deployment",
			Environment: []dsgen.MandatoryLabelsEnvironmentItem{dsgen.MandatoryLabelsEnvironmentItem_test}, // ✅ Array!
			Priority:    dsgen.MandatoryLabelsPriority_P3,
		},
		Status: dsgen.RemediationWorkflowStatusActive,
	}

	// Create authenticated HTTP client (DD-AUTH-014)
	httpClient := &http.Client{
		Transport: testauth.NewServiceAccountTransport(saToken),
	}

	// Create OpenAPI client with authentication
	client, err := dsgen.NewClient(dataStorageURL, dsgen.WithClient(httpClient))
	if err != nil {
		return fmt.Errorf("failed to create DataStorage client: %w", err)
	}

	// Register workflow via OpenAPI client
	ctx := context.Background()
	resp, err := client.CreateWorkflow(ctx, &workflow)
	if err != nil {
		return fmt.Errorf("failed to register workflow: %w", err)
	}

	// Validate response - success returns *RemediationWorkflow
	if createdWorkflow, ok := resp.(*dsgen.RemediationWorkflow); ok {
		_, _ = fmt.Fprintf(output, "    ✅ Registered in DataStorage: %s\n", workflowName)
		if wfID, exists := createdWorkflow.WorkflowID.Get(); exists {
			_, _ = fmt.Fprintf(output, "       UUID: %s\n", wfID.String())
		}
		return nil
	}

	// Handle error responses
	return fmt.Errorf("unexpected response type: %T", resp)
}

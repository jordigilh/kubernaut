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
	"net/http"

	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
)

// Workflow Bundle Infrastructure for WorkflowExecution E2E Tests
//
// This implements the production-like workflow registration flow:
// 1. Build Tekton Pipeline as OCI bundle (tkn bundle push)
// 2. Load bundle OCI image into Kind cluster
// 3. Register workflow in DataStorage (POST /api/v1/workflows)
// 4. WorkflowExecution references the bundle via schema_image field
//
// **Design Pattern**: Mirrors production workflow authoring flow
// - Operators author Tekton Pipelines and package as OCI bundles
// - Workflows are registered in DataStorage for AI Analysis catalog
// - WorkflowExecution controller resolves bundles via Tekton bundle resolver

const (
	// TestWorkflowBundleRegistry is the OCI registry for test workflow schema images.
	// Schema images contain only /workflow-schema.yaml (FROM scratch) and are used
	// by DataStorage for OCI-based workflow registration (DD-WORKFLOW-017).
	// Uses quay.io/kubernaut-cicd namespace (multi-arch support: amd64 + arm64)
	TestWorkflowBundleRegistry = "quay.io/kubernaut-cicd/test-workflows"

	// TestTektonBundleRegistry is the OCI registry for Tekton Pipeline bundles.
	// Tekton bundles are built with `tkn bundle push` and contain Tekton Pipeline
	// resources with required annotations (dev.tekton.image.apiVersion, etc.).
	// Used by WorkflowExecution controller via Tekton's bundle resolver.
	TestTektonBundleRegistry = "quay.io/kubernaut-cicd/tekton-bundles"

	// TestWorkflowBundleVersion is the version tag for E2E test bundles
	TestWorkflowBundleVersion = "v1.0.0"
)

// RegisteredWorkflowUUIDs maps workflow names to their DS-assigned UUIDs.
// Populated by BuildAndRegisterTestWorkflows; used by E2E tests that need
// the real UUID for dependency resolution (DD-WE-006).
var RegisteredWorkflowUUIDs = make(map[string]string)

// BuildAndRegisterTestWorkflows registers test workflows in DataStorage
// This creates the workflow catalog needed for E2E tests:
// - test-hello-world: Successful execution test workflow
// - test-intentional-failure: Failure handling test workflow
// - test-dep-secret-job: DD-WE-006 dependency injection test workflow
//
// **Bundle Strategy**: Uses pre-built multi-arch bundles from quay.io/kubernaut-cicd
// - Bundles are built manually and pushed to quay.io (amd64 + arm64)
// - Tekton pulls bundles directly from quay.io at runtime (no pre-loading needed)
// - No dynamic bundle building in CI/CD (simplifies pipeline)
//
// Returns the registered workflow bundle references for use in WorkflowExecution specs.
// Also populates RegisteredWorkflowUUIDs for tests that need DS UUIDs (DD-WE-006).
func BuildAndRegisterTestWorkflows(clusterName, kubeconfigPath, dataStorageURL, saToken string, output io.Writer) (map[string]string, error) {
	_, _ = fmt.Fprintf(output, "\n📦 Setting up test workflows from %s...\n", TestWorkflowBundleRegistry)

	bundles := make(map[string]string)

	// Use pre-built bundles from quay.io (multi-arch: amd64 + arm64)
	// Tekton's bundle resolver will pull these directly at runtime
	helloWorldRef := fmt.Sprintf("%s/hello-world:%s", TestWorkflowBundleRegistry, TestWorkflowBundleVersion)
	failingRef := fmt.Sprintf("%s/failing:%s", TestWorkflowBundleRegistry, TestWorkflowBundleVersion)
	depSecretJobRef := fmt.Sprintf("%s/dep-secret-job:%s", TestWorkflowBundleRegistry, TestWorkflowBundleVersion)
	depSecretTektonRef := fmt.Sprintf("%s/dep-secret-tekton:%s", TestWorkflowBundleRegistry, TestWorkflowBundleVersion)

	bundles["test-hello-world"] = helloWorldRef
	bundles["test-intentional-failure"] = failingRef
	bundles["test-dep-secret-job"] = depSecretJobRef
	bundles["test-dep-secret-tekton"] = depSecretTektonRef

	_, _ = fmt.Fprintf(output, "  ✅ Using bundles from %s\n", TestWorkflowBundleRegistry)

	// Register workflows in DataStorage using OpenAPI client (DD-API-001)
	_, _ = fmt.Fprintf(output, "\n📝 Registering workflows in DataStorage...\n")

	wfUUID, err := registerTestBundleWorkflow(
		dataStorageURL,
		saToken,
		"test-hello-world",
		"v1.0.0",
		bundles["test-hello-world"],
		"Simple hello-world workflow for E2E testing",
		output,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to register hello-world workflow: %w", err)
	}
	RegisteredWorkflowUUIDs["test-hello-world"] = wfUUID

	wfUUID, err = registerTestBundleWorkflow(
		dataStorageURL,
		saToken,
		"test-intentional-failure",
		"v1.0.0",
		bundles["test-intentional-failure"],
		"Intentionally failing workflow for E2E failure handling tests",
		output,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to register failing workflow: %w", err)
	}
	RegisteredWorkflowUUIDs["test-intentional-failure"] = wfUUID

	// DD-WE-006: Register workflow with secret dependency for dependency injection E2E tests.
	// The Secret "e2e-dep-secret" must exist in the execution namespace BEFORE registration
	// because DS validates dependencies at registration time.
	wfUUID, err = registerTestBundleWorkflow(
		dataStorageURL,
		saToken,
		"test-dep-secret-job",
		"v1.0.0",
		bundles["test-dep-secret-job"],
		"Job workflow with Secret dependency for DD-WE-006 E2E testing",
		output,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to register dep-secret-job workflow: %w", err)
	}
	RegisteredWorkflowUUIDs["test-dep-secret-job"] = wfUUID

	// DD-WE-006: Register Tekton workflow with secret dependency for Tekton dependency injection E2E tests.
	wfUUID, err = registerTestBundleWorkflow(
		dataStorageURL,
		saToken,
		"test-dep-secret-tekton",
		"v1.0.0",
		bundles["test-dep-secret-tekton"],
		"Tekton workflow with Secret dependency for DD-WE-006 E2E testing",
		output,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to register dep-secret-tekton workflow: %w", err)
	}
	RegisteredWorkflowUUIDs["test-dep-secret-tekton"] = wfUUID

	_, _ = fmt.Fprintf(output, "✅ Test workflows ready\n")
	return bundles, nil
}

// registerTestBundleWorkflow registers a workflow in DataStorage using OpenAPI client.
// DD-WORKFLOW-017: Pullspec-only registration — sends only schemaImage.
// DataStorage pulls the image, extracts /workflow-schema.yaml, and populates all fields.
// Returns the DS-assigned UUID for use in WorkflowExecution specs (DD-WE-006).
// Includes DD-AUTH-014 ServiceAccount authentication.
func registerTestBundleWorkflow(dataStorageURL, saToken, workflowName, version, schemaImage, description string, output io.Writer) (string, error) {
	_, _ = fmt.Fprintf(output, "  Registering: %s (version %s) from %s\n", workflowName, version, schemaImage)

	// Create authenticated HTTP client (DD-AUTH-014)
	httpClient := &http.Client{
		Transport: testauth.NewServiceAccountTransport(saToken),
	}

	// Create OpenAPI client with authentication
	client, err := dsgen.NewClient(dataStorageURL, dsgen.WithClient(httpClient))
	if err != nil {
		return "", fmt.Errorf("failed to create DataStorage client: %w", err)
	}

	// DD-WORKFLOW-017: Pullspec-only registration request
	req := &dsgen.CreateWorkflowFromOCIRequest{
		SchemaImage: schemaImage,
	}

	// Register workflow via OpenAPI client
	ctx := context.Background()
	resp, err := client.CreateWorkflow(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to register workflow: %w", err)
	}

	// Validate response - success returns *RemediationWorkflow
	if createdWorkflow, ok := resp.(*dsgen.RemediationWorkflow); ok {
		_, _ = fmt.Fprintf(output, "    ✅ Registered in DataStorage: %s\n", workflowName)
		if wfID, exists := createdWorkflow.WorkflowId.Get(); exists {
			_, _ = fmt.Fprintf(output, "       UUID: %s\n", wfID.String())
			return wfID.String(), nil
		}
		return "", fmt.Errorf("workflow registered but UUID not returned in response")
	}

	// Handle error responses
	return "", fmt.Errorf("unexpected response type: %T", resp)
}

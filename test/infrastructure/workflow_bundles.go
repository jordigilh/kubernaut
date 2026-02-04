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
	"os"
	"os/exec"
	"path/filepath"

	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
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
	// Uses quay.io for production-like testing (bundles pre-built and pushed once)
	TestWorkflowBundleRegistry = "quay.io/jordigilh/test-workflows"

	// TestWorkflowBundleVersion is the version tag for E2E test bundles
	TestWorkflowBundleVersion = "v1.0.0"
)

// getLocalBundleRegistry returns the registry to use for local bundle builds.
// In CI/CD: Uses IMAGE_REGISTRY env var (e.g., ghcr.io/jordigilh/kubernaut)
// Local dev: Uses ghcr.io as fallback (tkn bundle push rejects "localhost/")
//
// Pattern: CI/CD-aware registry selection for Tekton bundles
func getLocalBundleRegistry() string {
	// Check if IMAGE_REGISTRY is set (CI/CD mode)
	if registry := os.Getenv("IMAGE_REGISTRY"); registry != "" {
		return registry + "/test-workflows"
	}

	// Local dev fallback: Use ghcr.io (requires authentication)
	// Note: Developers must run `podman login ghcr.io` before building bundles locally
	return "ghcr.io/jordigilh/kubernaut/test-workflows"
}

// BuildAndRegisterTestWorkflows registers test workflows in DataStorage
// This creates the workflow catalog needed for E2E tests:
// - test-hello-world: Successful execution test workflow
// - test-intentional-failure: Failure handling test workflow
//
// **Smart Build Strategy** (Option B):
// 1. Check if bundles exist on quay.io/jordigilh/ (production-like)
// 2. If YES: Use existing bundles (fast, no build needed)
// 3. If NO: Build locally and load into Kind (automatic fallback for new contributors)
//
// Returns the registered workflow bundle references for use in WorkflowExecution specs.
func BuildAndRegisterTestWorkflows(clusterName, kubeconfigPath, dataStorageURL, saToken string, output io.Writer) (map[string]string, error) {
	_, _ = fmt.Fprintf(output, "\n📦 Setting up test workflows...\n")

	bundles := make(map[string]string)

	// Check if production bundles exist on quay.io
	helloWorldRef := fmt.Sprintf("%s/hello-world:%s", TestWorkflowBundleRegistry, TestWorkflowBundleVersion)
	failingRef := fmt.Sprintf("%s/failing:%s", TestWorkflowBundleRegistry, TestWorkflowBundleVersion)

	_, _ = fmt.Fprintf(output, "  Checking for existing bundles on %s...\n", TestWorkflowBundleRegistry)
	helloWorldExists := checkBundleExists(helloWorldRef, output)
	failingExists := checkBundleExists(failingRef, output)

	if helloWorldExists && failingExists {
		// Option A: Bundles exist on quay.io (production registry)
		// Pattern: Load from remote registry into Kind for offline test execution
		_, _ = fmt.Fprintf(output, "  ✅ Using existing bundles from quay.io\n")
		_, _ = fmt.Fprintf(output, "  📥 Loading into Kind for offline execution...\n")
		
		// Load existing bundles into Kind (ensures imagePullPolicy: Never works)
		if err := pullAndLoadBundleToKind(clusterName, helloWorldRef, output); err != nil {
			return nil, fmt.Errorf("failed to load hello-world bundle from quay.io: %w", err)
		}
		if err := pullAndLoadBundleToKind(clusterName, failingRef, output); err != nil {
			return nil, fmt.Errorf("failed to load failing bundle from quay.io: %w", err)
		}
		
		// Use quay.io references (now cached in Kind)
		bundles["test-hello-world"] = helloWorldRef
		bundles["test-intentional-failure"] = failingRef
		
		_, _ = fmt.Fprintf(output, "  ✅ Bundles loaded into Kind (quay.io refs cached)\n")
	} else {
		// Option B: Bundles don't exist - build and push to CI registry
		_, _ = fmt.Fprintf(output, "  ⚠️  Bundles not found on quay.io, building and pushing...\n")
		
		// Get registry for bundle builds (IMAGE_REGISTRY env var or ghcr.io fallback)
		bundleRegistry := getLocalBundleRegistry()
		_, _ = fmt.Fprintf(output, "  📦 Target registry: %s\n", bundleRegistry)
		_, _ = fmt.Fprintf(output, "  💡 TIP: Pre-build and push bundles to skip this step:\n")
		_, _ = fmt.Fprintf(output, "      tkn bundle push %s/hello-world:%s -f test/fixtures/tekton/hello-world-pipeline.yaml\n",
			TestWorkflowBundleRegistry, TestWorkflowBundleVersion)

		// Find project root for accessing test fixtures
		projectRoot, err := findProjectRoot()
		if err != nil {
			return nil, fmt.Errorf("failed to find project root: %w", err)
		}

		fixturesDir := filepath.Join(projectRoot, "test", "fixtures", "tekton")

		// Build with IMAGE_REGISTRY (CI) or ghcr.io (local dev)
		// Pattern: Push to remote registry, then load into Kind for offline execution
		localHelloRef := fmt.Sprintf("%s/hello-world:%s", bundleRegistry, TestWorkflowBundleVersion)
		localFailingRef := fmt.Sprintf("%s/failing:%s", bundleRegistry, TestWorkflowBundleVersion)

		// Build, push, and load hello-world bundle
		if err := buildAndLoadWorkflowBundle(
			clusterName,
			"test-hello-world",
			localHelloRef,
			filepath.Join(fixturesDir, "hello-world-pipeline.yaml"),
			output,
		); err != nil {
			return nil, fmt.Errorf("failed to build hello-world bundle: %w", err)
		}
		bundles["test-hello-world"] = localHelloRef

		// Build, push, and load failing bundle
		if err := buildAndLoadWorkflowBundle(
			clusterName,
			"test-intentional-failure",
			localFailingRef,
			filepath.Join(fixturesDir, "failing-pipeline.yaml"),
			output,
		); err != nil {
			return nil, fmt.Errorf("failed to build failing bundle: %w", err)
		}
		bundles["test-intentional-failure"] = localFailingRef
		
		_, _ = fmt.Fprintf(output, "  ✅ Bundles built, pushed to %s, and loaded into Kind\n", bundleRegistry)
	}

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

// pullAndLoadBundleToKind pulls a bundle from a remote registry and loads it into Kind.
// This enables offline test execution by caching remote bundles in the Kind cluster.
//
// Pattern: Same as service image loading (podman pull → podman save → kind load)
func pullAndLoadBundleToKind(clusterName, bundleRef string, output io.Writer) error {
	_, _ = fmt.Fprintf(output, "    Pulling %s...\n", bundleRef)
	
	// Pull bundle from remote registry
	pullCmd := exec.Command("podman", "pull", bundleRef)
	pullCmd.Stdout = output
	pullCmd.Stderr = output
	if err := pullCmd.Run(); err != nil {
		return fmt.Errorf("failed to pull bundle %s: %w", bundleRef, err)
	}
	
	// Create temp file for bundle archive
	tmpFile, err := os.CreateTemp("", "bundle-*.tar")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()
	_ = tmpFile.Close()
	
	// Save bundle to tar archive
	saveCmd := exec.Command("podman", "save", "-o", tmpFile.Name(), bundleRef)
	saveCmd.Stdout = output
	saveCmd.Stderr = output
	if err := saveCmd.Run(); err != nil {
		return fmt.Errorf("failed to save bundle %s: %w", bundleRef, err)
	}
	
	// Load bundle archive into Kind cluster
	loadCmd := exec.Command("kind", "load", "image-archive", tmpFile.Name(), "--name", clusterName)
	loadCmd.Stdout = output
	loadCmd.Stderr = output
	if err := loadCmd.Run(); err != nil {
		return fmt.Errorf("failed to load bundle %s into Kind: %w", bundleRef, err)
	}
	
	_, _ = fmt.Fprintf(output, "    ✅ Loaded: %s\n", bundleRef)
	return nil
}

// checkBundleExists checks if an OCI bundle exists in the registry
// Returns true if the bundle can be pulled, false otherwise
func checkBundleExists(bundleRef string, output io.Writer) bool {
	// Use skopeo to check bundle existence without pulling
	// Note: This is faster than podman pull and doesn't require credentials for public images
	cmd := exec.Command("skopeo", "inspect", "--raw", fmt.Sprintf("docker://%s", bundleRef))
	cmd.Stdout = nil // Suppress output
	cmd.Stderr = nil // Suppress errors
	err := cmd.Run()

	if err == nil {
		_, _ = fmt.Fprintf(output, "    ✅ Found: %s\n", bundleRef)
		return true
	}

	// Fallback: Try podman manifest inspect (if skopeo not available)
	cmd = exec.Command("podman", "manifest", "inspect", bundleRef)
	cmd.Stdout = nil
	cmd.Stderr = nil
	err = cmd.Run()

	if err == nil {
		_, _ = fmt.Fprintf(output, "    ✅ Found: %s\n", bundleRef)
		return true
	}

	_, _ = fmt.Fprintf(output, "    ❌ Not found: %s\n", bundleRef)
	return false
}

// buildAndLoadWorkflowBundle builds a Tekton Pipeline bundle and loads it into Kind
func buildAndLoadWorkflowBundle(clusterName, workflowName, bundleRef, pipelineYAML string, output io.Writer) error {
	_, _ = fmt.Fprintf(output, "  Building workflow: %s\n", workflowName)

	// Verify tkn CLI is installed
	if _, err := exec.LookPath("tkn"); err != nil {
		return fmt.Errorf("tkn CLI not found - install from https://tekton.dev/docs/cli/: %w", err)
	}

	// Verify pipeline YAML exists
	if _, err := os.Stat(pipelineYAML); err != nil {
		return fmt.Errorf("pipeline YAML not found at %s: %w", pipelineYAML, err)
	}

	// Build Tekton bundle using tkn CLI
	// NOTE: Tekton bundles contain ONLY Tekton resources (Pipeline, Task)
	// workflow-schema.yaml is Kubernaut metadata registered separately in DataStorage
	_, _ = fmt.Fprintf(output, "    Bundling Tekton Pipeline:\n")
	_, _ = fmt.Fprintf(output, "      Pipeline: %s\n", filepath.Base(pipelineYAML))
	_, _ = fmt.Fprintf(output, "      Bundle:   %s\n", bundleRef)

	buildCmd := exec.Command("tkn", "bundle", "push", bundleRef,
		"-f", pipelineYAML,
		// Note: --override flag does not exist in tkn CLI - bundles are naturally overwritable
	)
	buildCmd.Stdout = output
	buildCmd.Stderr = output

	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("tkn bundle push failed for %s: %w", bundleRef, err)
	}

	// Load bundle OCI image into Kind cluster
	_, _ = fmt.Fprintf(output, "    Loading bundle into Kind...\n")
	if err := loadBundleIntoKind(clusterName, bundleRef, output); err != nil {
		return fmt.Errorf("failed to load bundle into Kind: %w", err)
	}

	return nil
}

// loadBundleIntoKind loads a Tekton bundle OCI image into the Kind cluster
// Uses the same pattern as other E2E image loading: podman save + kind load image-archive
func loadBundleIntoKind(clusterName, bundleRef string, output io.Writer) error {
	// Create temp file for image archive
	tmpFile, err := os.CreateTemp("", "workflow-bundle-*.tar")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()
	_ = tmpFile.Close()

	// Save bundle image to tar file using podman
	// Note: Tekton bundles are OCI images, so standard container tools work
	saveCmd := exec.Command("podman", "save", "-o", tmpFile.Name(), bundleRef)
	saveCmd.Stdout = output
	saveCmd.Stderr = output
	if err := saveCmd.Run(); err != nil {
		return fmt.Errorf("failed to save bundle %s: %w", bundleRef, err)
	}

	// Load bundle archive into Kind cluster
	loadCmd := exec.Command("kind", "load", "image-archive", tmpFile.Name(), "--name", clusterName)
	loadCmd.Stdout = output
	loadCmd.Stderr = output
	if err := loadCmd.Run(); err != nil {
		return fmt.Errorf("failed to load bundle %s into Kind: %w", bundleRef, err)
	}

	return nil
}

// registerTestBundleWorkflow registers a workflow in DataStorage using OpenAPI client
// POST /api/v1/workflows per DD-WORKFLOW-005 v1.0
// Uses dsgen.RemediationWorkflow OpenAPI types for compile-time type safety (DD-API-001)
// Includes DD-AUTH-014 ServiceAccount authentication
func registerTestBundleWorkflow(dataStorageURL, saToken, workflowName, version, containerImage, description string, output io.Writer) error {
	_, _ = fmt.Fprintf(output, "  Registering: %s (version %s)\n", workflowName, version)

	// Generate ADR-043 compliant content
	content := fmt.Sprintf("# Test workflow %s\nversion: %s\ndescription: %s", workflowName, version, description)
	contentBytes := []byte(content)
	hash := sha256.Sum256(contentBytes)
	contentHash := fmt.Sprintf("%x", hash)

	// Build payload using OpenAPI client types (DD-API-001)
	// CRITICAL: Environment must be []dsgen.MandatoryLabelsEnvironmentItem, not string!
	workflow := dsgen.RemediationWorkflow{
		WorkflowName:    workflowName,
		Version:         version,
		Name:            fmt.Sprintf("Test Workflow: %s", workflowName),
		Description:     description,
		Content:         content,
		ContentHash:     contentHash,
		ExecutionEngine: "tekton", // String field, not enum
		ContainerImage:  dsgen.NewOptString(containerImage),
		Labels: dsgen.MandatoryLabels{
			SignalType:  "test-signal",
			Severity:    dsgen.MandatoryLabelsSeverityLow,
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

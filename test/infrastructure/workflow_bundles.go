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
	"os"
	"path/filepath"

	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// workflowConflictError is returned when the DS API reports a 409 Conflict,
// indicating the workflow already exists. The caller can use errors.As to
// distinguish this from fatal errors and fall back to a ListWorkflows query.
type workflowConflictError struct{ detail string }

func (e *workflowConflictError) Error() string {
	return fmt.Sprintf("conflict (409): %s", e.detail)
}

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
	// DD-WORKFLOW-016: Seed action types before workflow registration (FK constraint)
	// #1661 Phase 53: direct CRD creation -- no AuthWebhook/DataStorage round-trip
	// dependency for ActionType, unlike the removed SeedActionTypesViaAPIWithTLS.
	if err := SeedActionTypesViaCRD(kubeconfigPath, WorkflowExecutionNamespace, output); err != nil {
		return nil, fmt.Errorf("failed to seed action types: %w", err)
	}

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

	// Register workflows via kubectl apply (#1661 Phase 53: CRD-native registration,
	// exercising the same AuthWebhook admission path production traffic uses -- no
	// DataStorage inline-registration REST call, per DD-WORKFLOW-018 Change 6).
	_, _ = fmt.Fprintf(output, "\n📝 Registering workflows via kubectl apply...\n")

	// The fixture directory name maps to the workflow name (without "test-" prefix for some).
	// version/description are declared in each fixture's workflow-schema.yaml and no
	// longer need to be passed separately now that registration is CRD-native.
	bundleWorkflows := []struct {
		name       string
		fixtureDIR string
	}{
		{"test-hello-world", "hello-world"},
		{"test-intentional-failure", "failing"},
		{"test-dep-secret-job", "dep-secret-job"},
		{"test-dep-secret-tekton", "dep-secret-tekton"},
		{"test-dep-configmap-job", "dep-configmap-job"},
		{"test-dep-configmap-tekton", "dep-configmap-tekton"},
		{"test-job-hello-world", "job-hello-world"},
		{"test-job-intentional-failure", "job-failing"},
		{"test-job-oomkill", "job-oomkill"},
		{"test-ansible-success", "ansible-success"},
		{"test-ansible-failure", "ansible-failure"},
		{"test-dep-secret-ansible", "dep-secret-ansible"},
		{"test-dep-configmap-ansible", "dep-configmap-ansible"},
	}

	specs := make([]WorkflowSeedSpec, 0, len(bundleWorkflows))
	for _, bw := range bundleWorkflows {
		specs = append(specs, WorkflowSeedSpec{FixtureDir: bw.fixtureDIR})
	}

	seededUUIDs, seedErr := SeedWorkflowsViaKubectlApply(kubeconfigPath, WorkflowExecutionNamespace, specs, output)
	if seedErr != nil {
		return nil, fmt.Errorf("failed to register test workflow bundles: %w", seedErr)
	}
	for _, bw := range bundleWorkflows {
		// SeedWorkflowsViaKubectlApply keys its map "<crd-name>:<environment>"; these
		// fixtures declare no environment, so the key carries a trailing colon.
		uuid, ok := seededUUIDs[bw.name+":"]
		if !ok {
			return nil, fmt.Errorf("workflow %s missing from seeded UUIDs", bw.name)
		}
		RegisteredWorkflowUUIDs[bw.name] = uuid
	}

	// Populate bundles map with execution bundle references (still OCI images for Tekton/Job runtime)
	bundles["test-ansible-success"] = fmt.Sprintf("%s/ansible-success:%s", TestWorkflowBundleRegistry, TestWorkflowBundleVersion)
	bundles["test-ansible-failure"] = fmt.Sprintf("%s/ansible-failure:%s", TestWorkflowBundleRegistry, TestWorkflowBundleVersion)

	_, _ = fmt.Fprintf(output, "✅ Test workflows ready\n")
	return bundles, nil
}

// readWorkflowFixtureContent reads workflow-schema.yaml from the test fixtures directory.
// Uses findWorkspaceRoot() so the path resolves correctly regardless of the
// working directory (ginkgo sets cwd to the test package directory).
func readWorkflowFixtureContent(fixtureName string) (string, error) {
	root, err := findWorkspaceRoot()
	if err != nil {
		return "", fmt.Errorf("find workspace root: %w", err)
	}
	path := filepath.Join(root, "test", "fixtures", "workflows", fixtureName, "workflow-schema.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return string(data), nil
}

// callCreateWorkflowInline sends an inline registration request to DataStorage and
// extracts the UUID from the response. Shared by both bundle and seeding flows.
// Returns (uuid, reEnabled, error).
//
// All ogen response types are handled so that the caller receives an actionable
// error with the DS detail message instead of a generic "unexpected response type".
// A *workflowConflictError is returned for 409 so callers can fall back to query.
func callCreateWorkflowInline(client *dsgen.Client, content, registeredBy string) (string, bool, error) {
	req := &dsgen.CreateWorkflowInlineRequest{
		Content: content,
	}
	req.Source.SetTo("e2e-test")
	req.RegisteredBy.SetTo(registeredBy)

	ctx := context.Background()
	resp, err := client.CreateWorkflow(ctx, req)
	if err != nil {
		return "", false, fmt.Errorf("transport error: %w", err)
	}

	switch v := resp.(type) {
	case *dsgen.CreateWorkflowCreated:
		rw := (*dsgen.RemediationWorkflow)(v)
		if wfID, exists := rw.WorkflowId.Get(); exists {
			return wfID.String(), false, nil
		}
		return "", false, fmt.Errorf("workflow registered but UUID not returned")
	case *dsgen.CreateWorkflowOK:
		rw := (*dsgen.RemediationWorkflow)(v)
		if wfID, exists := rw.WorkflowId.Get(); exists {
			return wfID.String(), true, nil
		}
		return "", false, fmt.Errorf("workflow re-enabled but UUID not returned")
	case *dsgen.CreateWorkflowConflict:
		p := (*dsgen.RFC7807Problem)(v)
		return "", false, &workflowConflictError{detail: p.Detail.Value}
	case *dsgen.CreateWorkflowBadRequest:
		p := (*dsgen.RFC7807Problem)(v)
		return "", false, fmt.Errorf("DS rejected registration (400): %s", p.Detail.Value)
	case *dsgen.CreateWorkflowUnauthorized:
		p := (*dsgen.RFC7807Problem)(v)
		return "", false, fmt.Errorf("DS unauthorized (401): %s", p.Detail.Value)
	case *dsgen.CreateWorkflowForbidden:
		p := (*dsgen.RFC7807Problem)(v)
		return "", false, fmt.Errorf("DS forbidden (403): %s", p.Detail.Value)
	case *dsgen.CreateWorkflowInternalServerError:
		p := (*dsgen.RFC7807Problem)(v)
		return "", false, fmt.Errorf("DS internal error (500): %s", p.Detail.Value)
	default:
		return "", false, fmt.Errorf("unexpected response type: %T", resp)
	}
}

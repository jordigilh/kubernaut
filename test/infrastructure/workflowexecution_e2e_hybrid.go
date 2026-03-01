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
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	// WorkflowExecutionClusterName is the default Kind cluster name
	WorkflowExecutionClusterName = "workflowexecution-e2e"

	// TektonPipelinesVersion is the Tekton Pipelines version to install
	// NOTE: v1.7.0+ uses ghcr.io which doesn't require auth (gcr.io requires auth since 2025)
	TektonPipelinesVersion = "v1.7.0"

	// WorkflowExecutionNamespace is where the controller runs
	WorkflowExecutionNamespace = "kubernaut-system"

	// ExecutionNamespace is where PipelineRuns are created
	ExecutionNamespace = "kubernaut-workflows"

	// WorkflowExecutionMetricsHostPort is the host port for metrics endpoint
	// Mapped via Kind NodePort extraPortMappings (container: 30185 -> host: 9185)
	WorkflowExecutionMetricsHostPort = 9185
)

// SetupWorkflowExecutionInfrastructureHybridWithCoverage implements DD-TEST-002 hybrid parallel strategy:
// 1. Build images in parallel (FASTEST - before cluster creation)
// 2. Create Kind cluster AFTER builds complete (NO IDLE TIMEOUT)
// 3. Load images immediately into fresh cluster (RELIABLE)
// 4. Deploy services in parallel
//
// This is the AUTHORITATIVE pattern per DD-TEST-002:
// - Images build in parallel: ~2-3 minutes (not 7+ sequential minutes)
// - Cluster created when ready: No idle time, no timeout risk
// - Total time: ~5-6 minutes (faster AND more reliable)
//
// Per DD-TEST-007: E2E Coverage Capture Standard
func SetupWorkflowExecutionInfrastructureHybridWithCoverage(ctx context.Context, clusterName, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "🚀 WorkflowExecution E2E Infrastructure (HYBRID PARALLEL + COVERAGE)")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "  Strategy: Build parallel → Create cluster → Load → Deploy")
	_, _ = fmt.Fprintln(writer, "  Standard: DD-TEST-002 (Parallel Test Execution Standard)")
	_, _ = fmt.Fprintln(writer, "  Benefits: Fast builds + No cluster timeout + Reliable")
	_, _ = fmt.Fprintln(writer, "  Per DD-TEST-007: Coverage instrumentation enabled")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// DD-TEST-007: Create coverdata directory BEFORE everything
	projectRoot, err := findProjectRoot()
	if err != nil {
		return fmt.Errorf("failed to find project root: %w", err)
	}

	if os.Getenv("E2E_COVERAGE") == "true" {
		coverdataPath := filepath.Join(projectRoot, "test/e2e/workflowexecution/coverdata")
		_, _ = fmt.Fprintf(writer, "📁 Creating coverage directory: %s\n", coverdataPath)
		if err := os.MkdirAll(coverdataPath, 0777); err != nil {
			return fmt.Errorf("failed to create coverdata directory: %w", err)
		}
	}

	// ═══════════════════════════════════════════════════════════════════════
	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 1: Build images IN PARALLEL (before cluster creation)
	// ═══════════════════════════════════════════════════════════════════════
	// Per Consolidated API Migration (January 2026):
	// - Uses BuildImageForKind() for all images
	// - Returns dynamic image names for later use
	// - No manual tag generation (PHASE 0 removed)
	// - Authority: docs/handoff/CONSOLIDATED_API_MIGRATION_GUIDE_JAN07.md
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 1: Building images in parallel...")
	_, _ = fmt.Fprintln(writer, "  ├── WorkflowExecution controller (WITH COVERAGE)")
	_, _ = fmt.Fprintln(writer, "  ├── DataStorage image")
	_, _ = fmt.Fprintln(writer, "  └── AuthWebhook (FOR SOC2 CC8.1)")
	_, _ = fmt.Fprintln(writer, "  ⏱️  Expected: ~2-3 minutes (parallel)")

	type buildResult struct {
		name      string
		imageName string
		err       error
	}

	buildResults := make(chan buildResult, 3)
	builtImages := make(map[string]string)

	// Build WorkflowExecution controller with coverage in parallel
	// TEMPORARY FIX (Jan 9, 2026): Disable coverage on ARM64 due to Go runtime crash
	// See: docs/handoff/WE_E2E_RUNTIME_CRASH_JAN09.md
	go func() {
		// Disable coverage on ARM64 (Go runtime crash workaround)
		enableCoverage := os.Getenv("E2E_COVERAGE") == "true" && runtime.GOARCH != "arm64"
		cfg := E2EImageConfig{
			ServiceName:      "workflowexecution", // Operator SDK convention: no -controller suffix in image name
			ImageName:        "kubernaut/workflowexecution",
			DockerfilePath:   "docker/workflowexecution-controller.Dockerfile", // Dockerfile can have suffix
			BuildContextPath: "",
			EnableCoverage:   enableCoverage,
		}
		imageName, err := BuildImageForKind(cfg, writer)
		buildResults <- buildResult{name: "WorkflowExecution (coverage)", imageName: imageName, err: err}
	}()

	// Build DataStorage in parallel
	go func() {
		cfg := E2EImageConfig{
			ServiceName:      "datastorage",
			ImageName:        "kubernaut/datastorage",
			DockerfilePath:   "docker/data-storage.Dockerfile",
			BuildContextPath: "",
			EnableCoverage:   os.Getenv("E2E_COVERAGE") == "true",
		}
		imageName, err := BuildImageForKind(cfg, writer)
		buildResults <- buildResult{name: "DataStorage", imageName: imageName, err: err}
	}()

	// Build AuthWebhook in parallel
	go func() {
		cfg := E2EImageConfig{
			ServiceName:      "authwebhook",
			ImageName:        "authwebhook",
			DockerfilePath:   "docker/authwebhook.Dockerfile",
			BuildContextPath: "",
			EnableCoverage:   os.Getenv("E2E_COVERAGE") == "true",
		}
		imageName, err := BuildImageForKind(cfg, writer)
		buildResults <- buildResult{name: "AuthWebhook", imageName: imageName, err: err}
	}()

	// Wait for all 4 builds to complete (TD-E2E-001: Now includes OAuth2-Proxy)
	_, _ = fmt.Fprintln(writer, "\n⏳ Waiting for all builds to complete...")
	var buildErrors []error
	for i := 0; i < 3; i++ {
		result := <-buildResults
		if result.err != nil {
			_, _ = fmt.Fprintf(writer, "  ❌ %s build failed: %v\n", result.name, result.err)
			buildErrors = append(buildErrors, result.err)
		} else {
			_, _ = fmt.Fprintf(writer, "  ✅ %s build completed: %s\n", result.name, result.imageName)
			builtImages[result.name] = result.imageName
		}
	}

	if len(buildErrors) > 0 {
		return fmt.Errorf("image builds failed: %v", buildErrors)
	}

	_, _ = fmt.Fprintln(writer, "\n✅ All images built successfully!")

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 2: Create Kind cluster (now that images are ready)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 2: Creating Kind cluster...")
	_, _ = fmt.Fprintln(writer, "  ⏱️  Expected: ~15-20 seconds")

	// Find Kind config file
	configPath, err := findKindConfig("kind-workflowexecution-config.yaml")
	if err != nil {
		return fmt.Errorf("failed to find Kind config: %w", err)
	}

	// Create Kind cluster
	createCmd := exec.Command("kind", "create", "cluster",
		"--name", clusterName,
		"--config", configPath,
		"--kubeconfig", kubeconfigPath,
	)
	createCmd.Stdout = writer
	createCmd.Stderr = writer
	if err := createCmd.Run(); err != nil {
		return fmt.Errorf("failed to create Kind cluster: %w", err)
	}

	// Deploy WorkflowExecution CRD
	_, _ = fmt.Fprintln(writer, "📋 Installing WorkflowExecution CRD...")
	crdPath := filepath.Join(projectRoot, "config/crd/bases/kubernaut.ai_workflowexecutions.yaml")
	crdCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", crdPath)
	crdCmd.Stdout = writer
	crdCmd.Stderr = writer
	if err := crdCmd.Run(); err != nil {
		return fmt.Errorf("failed to install WorkflowExecution CRD: %w", err)
	}

	// Create namespaces (idempotent - ignore AlreadyExists errors)
	_, _ = fmt.Fprintf(writer, "📁 Creating namespace %s...\n", WorkflowExecutionNamespace)
	nsCmd := exec.Command("kubectl", "create", "namespace", WorkflowExecutionNamespace,
		"--kubeconfig", kubeconfigPath)
	nsOutput, nsErr := nsCmd.CombinedOutput()
	if nsErr != nil && !strings.Contains(string(nsOutput), "AlreadyExists") {
		_, _ = fmt.Fprintf(writer, "❌ Failed to create namespace %s: %s\n", WorkflowExecutionNamespace, string(nsOutput))
		return fmt.Errorf("failed to create namespace %s: %w", WorkflowExecutionNamespace, nsErr)
	}
	_, _ = fmt.Fprintf(writer, "   ✅ Namespace %s ready\n", WorkflowExecutionNamespace)
	// BR-SCOPE-002: Infrastructure namespace (kubernaut-system) must NOT be labeled as managed.
	// Only application/workload namespaces should have kubernaut.ai/managed=true.

	_, _ = fmt.Fprintf(writer, "📁 Creating namespace %s...\n", ExecutionNamespace)
	execNsCmd := exec.Command("kubectl", "create", "namespace", ExecutionNamespace,
		"--kubeconfig", kubeconfigPath)
	execNsOutput, execNsErr := execNsCmd.CombinedOutput()
	if execNsErr != nil && !strings.Contains(string(execNsOutput), "AlreadyExists") {
		_, _ = fmt.Fprintf(writer, "❌ Failed to create namespace %s: %s\n", ExecutionNamespace, string(execNsOutput))
		return fmt.Errorf("failed to create namespace %s: %w", ExecutionNamespace, execNsErr)
	}
	_, _ = fmt.Fprintf(writer, "   ✅ Namespace %s ready\n", ExecutionNamespace)
	// BR-SCOPE-001: Label namespace as managed by Kubernaut
	_ = exec.Command("kubectl", "label", "namespace", ExecutionNamespace,
		"kubernaut.ai/managed=true", "--overwrite", "--kubeconfig", kubeconfigPath).Run()

	_, _ = fmt.Fprintln(writer, "\n✅ Kind cluster ready!")

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 3: Load images into fresh cluster (parallel)
	// ═══════════════════════════════════════════════════════════════════════
	// Per Consolidated API Migration (January 2026):
	// - Uses LoadImageToKind() for all images
	// - Uses image names from builtImages map
	// - Authority: docs/handoff/CONSOLIDATED_API_MIGRATION_GUIDE_JAN07.md
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 3: Loading images into Kind cluster...")
	_, _ = fmt.Fprintln(writer, "  ├── WorkflowExecution controller (coverage)")
	_, _ = fmt.Fprintln(writer, "  ├── DataStorage image")
	_, _ = fmt.Fprintln(writer, "  └── AuthWebhook (SOC2)")
	_, _ = fmt.Fprintln(writer, "  ⏱️  Expected: ~30-45 seconds")

	type loadResult struct {
		name string
		err  error
	}

	loadResults := make(chan loadResult, 3)

	// Load WorkflowExecution controller image
	go func() {
		wfeImage := builtImages["WorkflowExecution (coverage)"]
		err := LoadImageToKind(wfeImage, "workflowexecution-controller", clusterName, writer)
		loadResults <- loadResult{name: "WorkflowExecution coverage", err: err}
	}()

	// Load DataStorage image
	go func() {
		dsImage := builtImages["DataStorage"]
		err := LoadImageToKind(dsImage, "datastorage", clusterName, writer)
		loadResults <- loadResult{name: "DataStorage", err: err}
	}()

	// Load AuthWebhook image
	go func() {
		awImage := builtImages["AuthWebhook"]
		err := LoadImageToKind(awImage, "authwebhook", clusterName, writer)
		loadResults <- loadResult{name: "AuthWebhook", err: err}
	}()

	// Wait for all 4 loads to complete (TD-E2E-001: Now includes OAuth2-Proxy)
	_, _ = fmt.Fprintln(writer, "\n⏳ Waiting for images to load...")
	var loadErrors []error
	for i := 0; i < 3; i++ {
		result := <-loadResults
		if result.err != nil {
			_, _ = fmt.Fprintf(writer, "  ❌ %s load failed: %v\n", result.name, result.err)
			loadErrors = append(loadErrors, result.err)
		} else {
			_, _ = fmt.Fprintf(writer, "  ✅ %s loaded\n", result.name)
		}
	}

	if len(loadErrors) > 0 {
		return fmt.Errorf("image loads failed: %v", loadErrors)
	}

	_, _ = fmt.Fprintln(writer, "\n✅ All images loaded into cluster!")

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 3.5: Create DataStorage RBAC (DD-AUTH-014)
	// ═══════════════════════════════════════════════════════════════════════
	// Step 0: Deploy data-storage-client ClusterRole (DD-AUTH-014)
	// CRITICAL: This must be deployed BEFORE RoleBindings that reference it
	_, _ = fmt.Fprintf(writer, "\n🔐 Deploying data-storage-client ClusterRole (DD-AUTH-014)...\n")
	if err := deployDataStorageClientClusterRole(ctx, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy client ClusterRole: %w", err)
	}

	// Step 1: Deploy DataStorage ServiceAccount + auth middleware RBAC
	// Required for DataStorage to call TokenReview and SubjectAccessReview APIs
	_, _ = fmt.Fprintf(writer, "🔐 Creating DataStorage ServiceAccount + auth middleware RBAC (DD-AUTH-014)...\n")
	if err := deployDataStorageServiceRBAC(ctx, WorkflowExecutionNamespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create DataStorage ServiceAccount RBAC: %w", err)
	}

	// Step 2: Create RoleBinding for DataStorage to access its own service (client RBAC)
	_, _ = fmt.Fprintf(writer, "🔐 Creating RoleBinding for DataStorage client access (DD-AUTH-014)...\n")
	if err := CreateDataStorageAccessRoleBinding(ctx, WorkflowExecutionNamespace, kubeconfigPath, "data-storage-service", writer); err != nil {
		return fmt.Errorf("failed to create DataStorage client RoleBinding: %w", err)
	}

	// Step 3: Create ServiceAccount + RBAC for WorkflowExecution controller audit writes (DD-AUTH-014)
	// Per RCA (Jan 30, 2026): WE controller needs SA for DataStorage audit emission
	// Pattern: Follow RemediationOrchestrator E2E infrastructure
	_, _ = fmt.Fprintf(writer, "🔐 Creating ServiceAccount for WorkflowExecution controller audit writes (DD-AUTH-014)...\n")
	if err := CreateE2EServiceAccountWithDataStorageAccess(ctx, WorkflowExecutionNamespace, kubeconfigPath, "workflowexecution-controller", writer); err != nil {
		return fmt.Errorf("failed to create WorkflowExecution controller ServiceAccount: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 4: Deploy services in PARALLEL (DD-TEST-002 MANDATE)
	// ═══════════════════════════════════════════════════════════════════════
	// Per Consolidated API Migration (January 2026):
	// - Use image names from builtImages map (built in Phase 1)
	// - Authority: docs/handoff/CONSOLIDATED_API_MIGRATION_GUIDE_JAN07.md
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 4: Deploying services in parallel...")
	_, _ = fmt.Fprintln(writer, "  (Kubernetes will handle dependencies and reconciliation)")

	type deployResult struct {
		name string
		err  error
	}
	deployResults := make(chan deployResult, 6)

	// Launch ALL kubectl apply commands concurrently
	go func() {
		err := installTektonPipelines(kubeconfigPath, writer)
		deployResults <- deployResult{"Tekton Pipelines", err}
	}()
	go func() {
		err := deployPostgreSQLInNamespace(ctx, WorkflowExecutionNamespace, kubeconfigPath, writer)
		deployResults <- deployResult{"PostgreSQL", err}
	}()
	go func() {
		err := deployRedisInNamespace(ctx, WorkflowExecutionNamespace, kubeconfigPath, writer)
		deployResults <- deployResult{"Redis", err}
	}()
	go func() {
		err := ApplyAllMigrations(ctx, WorkflowExecutionNamespace, kubeconfigPath, writer)
		deployResults <- deployResult{"Migrations", err}
	}()
	go func() {
		// Per Consolidated API Migration (January 2026):
		// Use DataStorage image name from builtImages map (built in Phase 1)
		dsImage := builtImages["DataStorage"]
		// TD-E2E-001 Phase 1: Deploy DataStorage with OAuth2-Proxy sidecar
		err := deployDataStorageServiceInNamespace(ctx, WorkflowExecutionNamespace, kubeconfigPath, dsImage, writer)
		deployResults <- deployResult{"DataStorage", err}
	}()
	go func() {
		// Per Consolidated API Migration (January 2026):
		// Use WorkflowExecution image name from builtImages map (built in Phase 1)
		wfeImage := builtImages["WorkflowExecution (coverage)"]
		err := DeployWorkflowExecutionController(ctx, WorkflowExecutionNamespace, kubeconfigPath, wfeImage, writer)
		deployResults <- deployResult{"WorkflowExecution", err}
	}()

	// Collect ALL results before proceeding (MANDATORY)
	var deployErrors []error
	for i := 0; i < 6; i++ {
		result := <-deployResults
		if result.err != nil {
			_, _ = fmt.Fprintf(writer, "  ❌ %s deployment failed: %v\n", result.name, result.err)
			deployErrors = append(deployErrors, result.err)
		} else {
			_, _ = fmt.Fprintf(writer, "  ✅ %s manifests applied\n", result.name)
		}
	}

	if len(deployErrors) > 0 {
		return fmt.Errorf("one or more service deployments failed: %v", deployErrors)
	}
	_, _ = fmt.Fprintln(writer, "  ✅ All manifests applied! (Kubernetes reconciling...)")

	// Single wait for ALL services ready (Kubernetes handles dependencies)
	_, _ = fmt.Fprintln(writer, "\n⏳ Waiting for all services to be ready (Kubernetes reconciling dependencies)...")
	if err := waitForWEServicesReady(ctx, WorkflowExecutionNamespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("services not ready: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 4.5: Deploy AuthWebhook manifests (using pre-built + pre-loaded image)
	// ═══════════════════════════════════════════════════════════════════════
	// Per DD-WEBHOOK-001: Required for WorkflowExecution block clearance
	// Per SOC2 CC8.1: Captures WHO cleared execution blocks after failures
	_, _ = fmt.Fprintln(writer, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "🔐 PHASE 4.5: Deploying AuthWebhook Manifests")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	awImage := builtImages["AuthWebhook"]
	if err := DeployAuthWebhookManifestsOnly(ctx, clusterName, WorkflowExecutionNamespace, kubeconfigPath, awImage, writer); err != nil {
		return fmt.Errorf("failed to deploy AuthWebhook manifests: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "✅ AuthWebhook deployed - SOC2 CC8.1 block clearance attribution enabled")

	// ═══════════════════════════════════════════════════════════════════════
	// POST-DEPLOYMENT: Build workflow bundles & create pipeline (requires DataStorage ready)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n🎯 Building and registering test workflow bundles...")

	// DD-AUTH-014: Create ServiceAccount for workflow registration with DataStorage
	workflowRegSAName := "workflow-registration-sa"
	_, _ = fmt.Fprintf(writer, "🔐 Creating ServiceAccount for workflow registration (DD-AUTH-014)...\n")
	if err := CreateE2EServiceAccountWithDataStorageAccess(ctx, WorkflowExecutionNamespace, kubeconfigPath, workflowRegSAName, writer); err != nil {
		return fmt.Errorf("failed to create workflow registration ServiceAccount: %w", err)
	}

	// Get ServiceAccount token for authenticated workflow registration
	saToken, err := GetServiceAccountToken(ctx, WorkflowExecutionNamespace, workflowRegSAName, kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to get ServiceAccount token: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "✅ ServiceAccount token retrieved for authenticated workflow registration\n")

	// DD-WE-006: Create dependency Secret in execution namespace BEFORE workflow registration.
	// DS validates that declared dependencies exist at registration time; the Secret must
	// be present for the dep-secret-job workflow to register successfully.
	_, _ = fmt.Fprintf(writer, "🔑 Creating DD-WE-006 dependency Secret in %s...\n", ExecutionNamespace)
	depSecretCmd := exec.Command("kubectl", "create", "secret", "generic", "e2e-dep-secret",
		"--from-literal=token=e2e-test-value",
		"--namespace", ExecutionNamespace,
		"--kubeconfig", kubeconfigPath)
	depSecretOut, depSecretErr := depSecretCmd.CombinedOutput()
	if depSecretErr != nil && !strings.Contains(string(depSecretOut), "AlreadyExists") {
		_, _ = fmt.Fprintf(writer, "⚠️  Failed to create DD-WE-006 dep Secret (non-fatal): %s\n", string(depSecretOut))
	} else {
		_, _ = fmt.Fprintf(writer, "   ✅ Secret e2e-dep-secret ready in %s\n", ExecutionNamespace)
	}

	dataStorageURL := "http://localhost:8092" // DD-TEST-001: WE → DataStorage dependency port
	if _, err = BuildAndRegisterTestWorkflows(clusterName, kubeconfigPath, dataStorageURL, saToken, writer); err != nil {
		return fmt.Errorf("failed to build and register test workflows: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "\n📋 Creating test pipeline...")
	if err := CreateSimpleTestPipeline(kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create test pipeline: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "\n🔑 Creating image pull secret...")
	if err := createQuayPullSecret(kubeconfigPath, ExecutionNamespace, writer); err != nil {
		_, _ = fmt.Fprintf(writer, "⚠️  Warning: Could not create quay.io pull secret: %v\n", err)
		// Non-fatal - repos may be public
	}

	_, _ = fmt.Fprintln(writer, "\n✅ All services ready and configured!")

	_, _ = fmt.Fprintln(writer, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "✅ WorkflowExecution E2E Infrastructure Ready!")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "  🚀 Strategy: Hybrid parallel (build parallel → cluster → load)")
	_, _ = fmt.Fprintln(writer, "  📊 Coverage: Enabled (GOCOVERDIR=/coverdata)")
	_, _ = fmt.Fprintln(writer, "  🎯 DataStorage URL: http://localhost:8092") // DD-TEST-001: WE → DataStorage dependency port
	_, _ = fmt.Fprintln(writer, "  📦 Namespace: kubernaut-system")
	_, _ = fmt.Fprintln(writer, "  ⏱️  Total time: ~5-6 minutes (per DD-TEST-002)")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	return nil
}

// BuildWorkflowExecutionImageWithCoverage builds the WorkflowExecution controller image with coverage instrumentation
func BuildWorkflowExecutionImageWithCoverage(projectRoot string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "  🔨 Building WorkflowExecution controller image (with coverage)...")

	dockerfilePath := filepath.Join(projectRoot, "docker/workflowexecution-controller.Dockerfile")
	imageName := "localhost/kubernaut-workflowexecution:e2e-test-workflowexecution"

	buildArgs := []string{
		"build",
		"--no-cache", // DD-TEST-002: Force fresh build to include latest code changes
		"-t", imageName,
		"-f", dockerfilePath,
		"--build-arg", fmt.Sprintf("GOARCH=%s", runtime.GOARCH),
	}

	// DD-TEST-007: E2E Coverage Collection
	// TEMPORARY FIX (Jan 9, 2026): Disable coverage on ARM64 due to Go runtime crash
	// See: docs/handoff/WE_E2E_RUNTIME_CRASH_JAN09.md
	// Root cause: taggedPointerPack fatal error in Go 1.25.3 (Red Hat) on ARM64
	// TODO: Re-enable after switching to upstream Go builder (Solution B)
	if os.Getenv("E2E_COVERAGE") == "true" && runtime.GOARCH != "arm64" {
		buildArgs = append(buildArgs, "--build-arg", "GOFLAGS=-cover")
		_, _ = fmt.Fprintln(writer, "     📊 Building with coverage instrumentation (GOFLAGS=-cover)")
	} else if os.Getenv("E2E_COVERAGE") == "true" && runtime.GOARCH == "arm64" {
		_, _ = fmt.Fprintln(writer, "     ⚠️  Coverage disabled on ARM64 (Go runtime crash workaround)")
	}

	buildArgs = append(buildArgs, projectRoot)

	buildCmd := exec.Command("podman", buildArgs...)
	buildCmd.Stdout = writer
	buildCmd.Stderr = writer
	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("failed to build WorkflowExecution controller image: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "     ✅ WorkflowExecution controller image built")
	return nil
}

// LoadWorkflowExecutionCoverageImage loads the WorkflowExecution controller image into Kind cluster
func LoadWorkflowExecutionCoverageImage(clusterName, projectRoot string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "  📦 Loading WorkflowExecution controller image into Kind...")

	imageName := "localhost/kubernaut-workflowexecution:e2e-test-workflowexecution"

	// Save image to tarball for Kind loading (Podman images need explicit save/load)
	tarPath := filepath.Join(projectRoot, "workflowexecution-controller.tar")
	saveCmd := exec.Command("podman", "save", "-o", tarPath, imageName)
	saveCmd.Stdout = writer
	saveCmd.Stderr = writer
	if err := saveCmd.Run(); err != nil {
		return fmt.Errorf("failed to save controller image: %w", err)
	}
	defer func() { _ = os.Remove(tarPath) }()

	// Load image into Kind from tarball
	loadCmd := exec.Command("kind", "load", "image-archive", tarPath,
		"--name", clusterName,
	)
	loadCmd.Stdout = writer
	loadCmd.Stderr = writer
	if err := loadCmd.Run(); err != nil {
		return fmt.Errorf("failed to load image into Kind: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "     ✅ WorkflowExecution controller image loaded")
	return nil
}

// waitForWEServicesReady waits for DataStorage and WorkflowExecution pods to be ready
// Per DD-TEST-002: Single readiness check after parallel deployment
func waitForWEServicesReady(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	// Build Kubernetes clientset
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to build kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create clientset: %w", err)
	}

	// Wait for DataStorage pod to be ready
	_, _ = fmt.Fprintf(writer, "   ⏳ Waiting for DataStorage pod to be ready...\n")
	Eventually(func() bool {
		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app=datastorage",
		})
		if err != nil || len(pods.Items) == 0 {
			return false
		}
		for _, pod := range pods.Items {
			if pod.Status.Phase == corev1.PodRunning {
				for _, condition := range pod.Status.Conditions {
					if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
						return true
					}
				}
			}
		}
		return false
	}, 2*time.Minute, 5*time.Second).Should(BeTrue(), "DataStorage pod should become ready")
	_, _ = fmt.Fprintf(writer, "   ✅ DataStorage ready\n")

	// Wait for WorkflowExecution controller pod to be ready
	_, _ = fmt.Fprintf(writer, "   ⏳ Waiting for WorkflowExecution controller pod to be ready...\n")
	Eventually(func() bool {
		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app=workflowexecution-controller",
		})
		if err != nil || len(pods.Items) == 0 {
			return false
		}
		for _, pod := range pods.Items {
			if pod.Status.Phase == corev1.PodRunning {
				for _, condition := range pod.Status.Conditions {
					if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
						return true
					}
				}
			}
		}
		return false
	}, 3*time.Minute, 5*time.Second).Should(BeTrue(), "WorkflowExecution controller pod should become ready")
	_, _ = fmt.Fprintf(writer, "   ✅ WorkflowExecution controller ready\n")

	return nil
}

// buildDataStorageImageWithTag and loadDataStorageImageWithTag are defined in datastorage.go
// and shared across all E2E infrastructure files in this package

// findKindConfig locates Kind configuration files in standard test paths
// Restored from git history: a906a3767~1:test/infrastructure/workflowexecution.go
func findKindConfig(filename string) (string, error) {
	projectRoot, err := findProjectRoot()
	if err != nil {
		return "", err
	}
	cwd, _ := os.Getwd()

	// Try paths relative to project root
	paths := []string{
		filepath.Join(projectRoot, "test", "infrastructure", filename),
		filepath.Join(cwd, "test", "infrastructure", filename),
		filepath.Join(cwd, "..", "..", "test", "infrastructure", filename),
		filepath.Join(cwd, "..", "infrastructure", filename),
		filename,
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf("kind config file %s not found in any expected location (tried from %s)", filename, projectRoot)
}
func installTektonPipelines(kubeconfigPath string, output io.Writer) error {
	// Install Tekton Pipelines from GitHub releases (v1.0+ use GitHub releases)
	// NOTE: storage.googleapis.com/tekton-releases requires auth since 2025
	releaseURL := fmt.Sprintf("https://github.com/tektoncd/pipeline/releases/download/%s/release.yaml", TektonPipelinesVersion)

	_, _ = fmt.Fprintf(output, "  Applying Tekton release from: %s\n", releaseURL)

	// Retry logic for transient GitHub CDN failures (503 Service Unavailable)
	// GitHub's CDN occasionally returns 503 errors during high load
	maxRetries := 3
	backoffSeconds := []int{5, 10, 20} // Exponential backoff: 5s, 10s, 20s

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			_, _ = fmt.Fprintf(output, "  ⚠️  Attempt %d/%d failed, retrying in %ds...\n", attempt, maxRetries, backoffSeconds[attempt-1])
			time.Sleep(time.Duration(backoffSeconds[attempt-1]) * time.Second)
			_, _ = fmt.Fprintf(output, "  🔄 Retry attempt %d/%d...\n", attempt+1, maxRetries)
		}

		applyCmd := exec.Command("kubectl", "apply",
			"-f", releaseURL,
			"--kubeconfig", kubeconfigPath,
		)
		applyCmd.Stdout = output
		applyCmd.Stderr = output

		if err := applyCmd.Run(); err != nil {
			lastErr = err
			continue // Retry
		}

		// Success!
		if attempt > 0 {
			_, _ = fmt.Fprintf(output, "  ✅ Tekton release applied successfully on attempt %d\n", attempt+1)
		}
		lastErr = nil
		break
	}

	if lastErr != nil {
		return fmt.Errorf("failed to apply Tekton release after %d attempts: %w", maxRetries, lastErr)
	}

	// Wait for Tekton controller to be ready
	// Phase 1 E2E Stabilization: Increased timeout to 1 hour (3600s) to prevent timeout failures
	// Root cause: Slow Tekton image pulls in Kind cluster (see WE_E2E_INFRASTRUCTURE_STABILIZATION_PLAN.md)
	_, _ = fmt.Fprintf(output, "  ⏳ Waiting for Tekton Pipelines controller (up to 1 hour)...\n")
	waitCmd := exec.Command("kubectl", "wait",
		"-n", "tekton-pipelines",
		"--for=condition=available",
		"deployment/tekton-pipelines-controller",
		"--timeout=3600s",
		"--kubeconfig", kubeconfigPath,
	)
	waitCmd.Stdout = output
	waitCmd.Stderr = output
	if err := waitCmd.Run(); err != nil {
		return fmt.Errorf("tekton controller did not become ready: %w", err)
	}

	// Wait for Tekton webhook to be ready
	// Phase 1 E2E Stabilization: Increased timeout to 1 hour (3600s)
	_, _ = fmt.Fprintf(output, "  ⏳ Waiting for Tekton webhook (up to 1 hour)...\n")
	webhookWaitCmd := exec.Command("kubectl", "wait",
		"-n", "tekton-pipelines",
		"--for=condition=available",
		"deployment/tekton-pipelines-webhook",
		"--timeout=3600s",
		"--kubeconfig", kubeconfigPath,
	)
	webhookWaitCmd.Stdout = output
	webhookWaitCmd.Stderr = output
	if err := webhookWaitCmd.Run(); err != nil {
		return fmt.Errorf("tekton webhook did not become ready: %w", err)
	}

	return nil
}

func DeployWorkflowExecutionController(ctx context.Context, namespace, kubeconfigPath, imageName string, output io.Writer) error {
	_, _ = fmt.Fprintf(output, "\n🚀 Deploying WorkflowExecution Controller to %s...\n", namespace)
	_, _ = fmt.Fprintf(output, "  Using pre-built image: %s\n", imageName)

	projectRoot, err := findProjectRoot()
	if err != nil {
		return fmt.Errorf("failed to find project root: %w", err)
	}

	// CRDs must exist before any CR can be created
	crdPath := filepath.Join(projectRoot, "config/crd/bases/kubernaut.ai_workflowexecutions.yaml")
	_, _ = fmt.Fprintf(output, "  Applying WorkflowExecution CRDs...\n")
	crdCmd := exec.Command("kubectl", "apply",
		"-f", crdPath,
		"--kubeconfig", kubeconfigPath,
	)
	crdCmd.Stdout = output
	crdCmd.Stderr = output
	if err := crdCmd.Run(); err != nil {
		return fmt.Errorf("failed to apply CRDs: %w", err)
	}

	// All remaining resources (Namespaces, RBAC, ConfigMap, Deployment, Service)
	// are applied in a single kubectl apply call.
	_, _ = fmt.Fprintf(output, "  Applying all controller resources (single kubectl apply)...\n")
	if err := deployWorkflowExecutionControllerDeployment(ctx, namespace, kubeconfigPath, imageName, output); err != nil {
		return fmt.Errorf("failed to deploy controller: %w", err)
	}

	_, _ = fmt.Fprintf(output, "✅ WorkflowExecution Controller deployed\n")
	return nil
}

func CreateSimpleTestPipeline(kubeconfigPath string, output io.Writer) error {
	_, _ = fmt.Fprintf(output, "\n📝 Creating test pipelines (success + failure)...\n")

	pipelineYAML := `
apiVersion: tekton.dev/v1
kind: Pipeline
metadata:
  name: test-hello-world
  namespace: kubernaut-workflows
spec:
  params:
    - name: TARGET_RESOURCE
      type: string
      description: Target resource being remediated
    - name: MESSAGE
      type: string
      default: "Hello from Kubernaut!"
  tasks:
    - name: echo-hello
      taskRef:
        name: test-echo-task
      params:
        - name: message
          value: $(params.MESSAGE)
---
apiVersion: tekton.dev/v1
kind: Task
metadata:
  name: test-echo-task
  namespace: kubernaut-workflows
spec:
  params:
    - name: message
      type: string
  steps:
    - name: echo
      image: registry.access.redhat.com/ubi9/ubi-minimal:latest
      script: |
        #!/bin/sh
        echo "$(params.message)"
        echo "Test task completed successfully"
        sleep 2
---
# Intentionally failing pipeline for BR-WE-004 failure details testing
apiVersion: tekton.dev/v1
kind: Pipeline
metadata:
  name: test-intentional-failure
  namespace: kubernaut-workflows
spec:
  params:
    - name: TARGET_RESOURCE
      type: string
      description: Target resource being remediated
    - name: FAILURE_REASON
      type: string
      default: "Simulated failure for E2E testing"
  tasks:
    - name: fail-task
      taskRef:
        name: test-fail-task
      params:
        - name: reason
          value: $(params.FAILURE_REASON)
---
apiVersion: tekton.dev/v1
kind: Task
metadata:
  name: test-fail-task
  namespace: kubernaut-workflows
spec:
  params:
    - name: reason
      type: string
  steps:
    - name: fail
      image: registry.access.redhat.com/ubi9/ubi-minimal:latest
      script: |
        #!/bin/sh
        echo "Task will fail with reason: $(params.reason)"
        echo "This is an intentional failure for BR-WE-004 E2E testing"
        exit 1
`

	// Write to temp file and apply
	tmpFile, err := os.CreateTemp("", "test-pipeline-*.yaml")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	if _, err := tmpFile.WriteString(pipelineYAML); err != nil {
		return fmt.Errorf("failed to write pipeline YAML: %w", err)
	}
	_ = tmpFile.Close()

	applyCmd := exec.Command("kubectl", "apply",
		"-f", tmpFile.Name(),
		"--kubeconfig", kubeconfigPath,
	)
	applyCmd.Stdout = output
	applyCmd.Stderr = output
	if err := applyCmd.Run(); err != nil {
		return fmt.Errorf("failed to create test pipeline: %w", err)
	}

	_, _ = fmt.Fprintf(output, "✅ Test pipeline created\n")
	return nil
}

func createQuayPullSecret(kubeconfigPath, namespace string, output io.Writer) error {
	_, _ = fmt.Fprintf(output, "🔐 Creating quay.io pull secret...\n")

	// Get the auth config from podman
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	authFile := filepath.Join(homeDir, ".config/containers/auth.json")
	if _, err := os.Stat(authFile); os.IsNotExist(err) {
		return fmt.Errorf("podman auth file not found at %s", authFile)
	}

	// Create the secret in the execution namespace
	secretCmd := exec.Command("kubectl", "create", "secret", "docker-registry", "quay-pull-secret",
		"--from-file=.dockerconfigjson="+authFile,
		"--namespace", namespace,
		"--kubeconfig", kubeconfigPath,
	)
	secretCmd.Stdout = output
	secretCmd.Stderr = output
	if err := secretCmd.Run(); err != nil {
		return fmt.Errorf("failed to create pull secret: %w", err)
	}

	// Create the secret in tekton-pipelines-resolvers namespace for bundle resolver
	secretResolverCmd := exec.Command("kubectl", "create", "secret", "docker-registry", "quay-pull-secret",
		"--from-file=.dockerconfigjson="+authFile,
		"--namespace", "tekton-pipelines-resolvers",
		"--kubeconfig", kubeconfigPath,
	)
	secretResolverCmd.Stdout = output
	secretResolverCmd.Stderr = output
	_ = secretResolverCmd.Run() // Ignore error if namespace doesn't exist

	// Patch the service account to use the pull secret
	patchCmd := exec.Command("kubectl", "patch", "serviceaccount", "kubernaut-workflow-runner",
		"-n", namespace,
		"--kubeconfig", kubeconfigPath,
		"-p", `{"imagePullSecrets": [{"name": "quay-pull-secret"}]}`,
	)
	patchCmd.Stdout = output
	patchCmd.Stderr = output
	// Ignore error if service account doesn't exist yet
	_ = patchCmd.Run()

	// Also patch the default service account in execution namespace
	patchDefaultCmd := exec.Command("kubectl", "patch", "serviceaccount", "default",
		"-n", namespace,
		"--kubeconfig", kubeconfigPath,
		"-p", `{"imagePullSecrets": [{"name": "quay-pull-secret"}]}`,
	)
	patchDefaultCmd.Stdout = output
	patchDefaultCmd.Stderr = output
	_ = patchDefaultCmd.Run()

	// Patch the tekton-pipelines-resolvers service account
	patchResolverCmd := exec.Command("kubectl", "patch", "serviceaccount", "tekton-pipelines-resolvers",
		"-n", "tekton-pipelines-resolvers",
		"--kubeconfig", kubeconfigPath,
		"-p", `{"imagePullSecrets": [{"name": "quay-pull-secret"}]}`,
	)
	patchResolverCmd.Stdout = output
	patchResolverCmd.Stderr = output
	_ = patchResolverCmd.Run()

	_, _ = fmt.Fprintf(output, "✅ Pull secret created and linked to service accounts\n")
	return nil
}

func deployWorkflowExecutionControllerDeployment(_ context.Context, namespace, kubeconfigPath, imageName string, output io.Writer) error {
	coverageEnabled := os.Getenv("E2E_COVERAGE") == "true"
	_, _ = fmt.Fprintf(output, "   DD-TEST-007: E2E_COVERAGE=%s (enabled=%v)\n", os.Getenv("E2E_COVERAGE"), coverageEnabled)

	// DD-TEST-007: Coverage-conditional sections injected into the YAML template
	var coverageEnv, coverageVolumeMount, coverageVolume, securityContext string
	if coverageEnabled {
		coverageEnv = `
        - name: GOCOVERDIR
          value: /coverdata`
		coverageVolumeMount = `
        - name: coverage
          mountPath: /coverdata`
		coverageVolume = `
      - name: coverage
        hostPath:
          path: /coverdata
          type: DirectoryOrCreate`
		securityContext = `
      securityContext:
        runAsUser: 0
        runAsGroup: 0`
	}

	manifest := fmt.Sprintf(`apiVersion: v1
kind: Namespace
metadata:
  name: %[1]s
---
apiVersion: v1
kind: Namespace
metadata:
  name: %[2]s
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: workflowexecution-controller
  namespace: %[1]s
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: kubernaut-workflow-runner
  namespace: %[2]s
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: workflowexecution-controller
rules:
- apiGroups: ["kubernaut.ai"]
  resources: ["workflowexecutions"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["kubernaut.ai"]
  resources: ["workflowexecutions/status"]
  verbs: ["get", "update", "patch"]
- apiGroups: ["kubernaut.ai"]
  resources: ["workflowexecutions/finalizers"]
  verbs: ["update"]
- apiGroups: ["tekton.dev"]
  resources: ["pipelineruns"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["tekton.dev"]
  resources: ["taskruns"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["batch"]
  resources: ["jobs"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]
- apiGroups: ["coordination.k8s.io"]
  resources: ["leases"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: workflowexecution-controller
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: workflowexecution-controller
subjects:
- kind: ServiceAccount
  name: workflowexecution-controller
  namespace: %[1]s
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: workflowexecution-config
  namespace: %[1]s
data:
  workflowexecution.yaml: |
    controller:
      metricsAddr: ":9090"
      healthProbeAddr: ":8081"
      leaderElection: false
      leaderElectionId: workflowexecution.kubernaut.ai
    execution:
      namespace: %[2]s
      cooldownPeriod: 1m
      serviceAccount: kubernaut-workflow-runner
    datastorage:
      url: "http://data-storage-service.%[1]s:8080"
      timeout: 10s
      buffer:
        bufferSize: 10000
        batchSize: 50
        flushInterval: 100ms
        maxRetries: 3
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: workflowexecution-controller
  namespace: %[1]s
  labels:
    app: workflowexecution-controller
spec:
  replicas: 1
  selector:
    matchLabels:
      app: workflowexecution-controller
  template:
    metadata:
      labels:
        app: workflowexecution-controller
    spec:
      serviceAccountName: workflowexecution-controller%[5]s
      containers:
      - name: controller
        image: %[3]s
        imagePullPolicy: %[4]s
        args:
        - --config=/etc/config/workflowexecution.yaml
        - --zap-devel=true
        ports:
        - containerPort: 9090
          name: metrics
        - containerPort: 8081
          name: health
        env:%[6]s
        volumeMounts:
        - name: config
          mountPath: /etc/config
          readOnly: true%[7]s
        livenessProbe:
          httpGet:
            path: /healthz
            port: health
          initialDelaySeconds: 15
          periodSeconds: 20
        readinessProbe:
          httpGet:
            path: /readyz
            port: health
          initialDelaySeconds: 5
          periodSeconds: 10
        resources:
          limits:
            cpu: 500m
            memory: 256Mi
          requests:
            cpu: 100m
            memory: 64Mi
      volumes:
      - name: config
        configMap:
          name: workflowexecution-config%[8]s
---
apiVersion: v1
kind: Service
metadata:
  name: workflowexecution-controller-metrics
  namespace: %[1]s
spec:
  type: NodePort
  selector:
    app: workflowexecution-controller
  ports:
  - name: metrics
    port: 9090
    targetPort: 9090
    nodePort: 30185
`,
		namespace,          // [1] controller namespace (kubernaut-system)
		ExecutionNamespace, // [2] execution namespace (kubernaut-workflows)
		imageName,          // [3] controller image
		GetImagePullPolicy(), // [4] pull policy
		securityContext,    // [5] pod security context (coverage only)
		coverageEnv,        // [6] GOCOVERDIR env var (coverage only)
		coverageVolumeMount, // [7] /coverdata volume mount (coverage only)
		coverageVolume,     // [8] hostPath volume (coverage only)
	)

	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "--server-side", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to apply WorkflowExecution controller resources: %w", err)
	}

	return nil
}

func waitForDeploymentReady(kubeconfigPath, deploymentName string, output io.Writer) error {
	waitCmd := exec.Command("kubectl", "wait",
		"-n", WorkflowExecutionNamespace,
		"--for=condition=available",
		"deployment/"+deploymentName,
		"--timeout=3600s",
		"--kubeconfig", kubeconfigPath,
	)
	waitCmd.Stdout = output
	waitCmd.Stderr = output
	if err := waitCmd.Run(); err != nil {
		return fmt.Errorf("deployment %s did not become available: %w", deploymentName, err)
	}
	return nil
}
func DeleteWorkflowExecutionCluster(clusterName string, testsFailed bool, output io.Writer) error {
	// Use shared cleanup function with log export on failure
	// Note: WorkflowExecution has history of hung deletions, but DeleteCluster doesn't have timeout
	// If this becomes an issue again, we may need to add timeout support to shared function
	return DeleteCluster(clusterName, "workflowexecution", testsFailed, output)
}

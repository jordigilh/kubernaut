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
	"time"

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
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
			ServiceName:      "workflowexecution-controller",
			ImageName:        "kubernaut/workflowexecution-controller",
			DockerfilePath:   "docker/workflowexecution-controller.Dockerfile",
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
			ServiceName:      "webhooks",
			ImageName:        "webhooks",
			DockerfilePath:   "docker/webhooks.Dockerfile",
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

	// Create namespaces
	_, _ = fmt.Fprintf(writer, "📁 Creating namespace %s...\n", WorkflowExecutionNamespace)
	nsCmd := exec.Command("kubectl", "create", "namespace", WorkflowExecutionNamespace,
		"--kubeconfig", kubeconfigPath)
	nsCmd.Stdout = writer
	nsCmd.Stderr = writer
	_ = nsCmd.Run() // May already exist

	_, _ = fmt.Fprintf(writer, "📁 Creating namespace %s...\n", ExecutionNamespace)
	execNsCmd := exec.Command("kubectl", "create", "namespace", ExecutionNamespace,
		"--kubeconfig", kubeconfigPath)
	execNsCmd.Stdout = writer
	execNsCmd.Stderr = writer
	_ = execNsCmd.Run() // May already exist

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
		err := LoadImageToKind(awImage, "webhooks", clusterName, writer)
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
	dataStorageURL := "http://localhost:8081" // NodePort per DD-TEST-001
	if _, err := BuildAndRegisterTestWorkflows(clusterName, kubeconfigPath, dataStorageURL, writer); err != nil {
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
	_, _ = fmt.Fprintln(writer, "  🎯 DataStorage URL: http://localhost:8081")
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

	return "", fmt.Errorf("Kind config file %s not found in any expected location (tried from %s)", filename, projectRoot)
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
		return fmt.Errorf("Tekton controller did not become ready: %w", err)
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
		return fmt.Errorf("Tekton webhook did not become ready: %w", err)
	}

	return nil
}

func DeployWorkflowExecutionController(ctx context.Context, namespace, kubeconfigPath, imageName string, output io.Writer) error {
	// Per Consolidated API Migration (January 2026):
	// Accept dynamic image name as parameter (built by BuildImageForKind in PHASE 1)
	// Image already loaded to Kind in PHASE 3 - no build/load needed here
	// Authority: docs/handoff/CONSOLIDATED_API_MIGRATION_GUIDE_JAN07.md
	_, _ = fmt.Fprintf(output, "\n🚀 Deploying WorkflowExecution Controller to %s...\n", namespace)
	_, _ = fmt.Fprintf(output, "  Using pre-built image: %s\n", imageName)

	// Find project root for absolute paths
	projectRoot, err := findProjectRoot()
	if err != nil {
		return fmt.Errorf("failed to find project root: %w", err)
	}

	// Create controller namespace
	nsCmd := exec.Command("kubectl", "create", "namespace", namespace,
		"--kubeconfig", kubeconfigPath)
	nsCmd.Stdout = output
	nsCmd.Stderr = output
	_ = nsCmd.Run() // May already exist

	// Deploy CRDs (use absolute path)
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

	// Apply static resources (Namespaces, ServiceAccounts, RBAC)
	// Note: Deployment and Service are created programmatically for E2E coverage support
	manifestsPath := filepath.Join(projectRoot, "test/e2e/workflowexecution/manifests/controller-deployment.yaml")
	_, _ = fmt.Fprintf(output, "  Applying static resources (Namespaces, ServiceAccounts, RBAC)...\n")

	// Use kubectl apply but exclude Deployment and Service resources
	// They will be created programmatically with E2E coverage support
	excludeCmd := exec.Command("kubectl", "apply",
		"-f", manifestsPath,
		"--kubeconfig", kubeconfigPath,
	)
	excludeCmd.Stdout = output
	excludeCmd.Stderr = output
	if err := excludeCmd.Run(); err != nil {
		// Ignore errors - some resources may already exist
		_, _ = fmt.Fprintf(output, "   ⚠️  Some resources may already exist (continuing)\n")
	}

	// Delete existing Deployment and Service if they exist (they were created by kubectl apply above)
	// We'll recreate them programmatically with E2E coverage support
	_, _ = fmt.Fprintf(output, "  Cleaning up existing Deployment/Service (if any)...\n")
	deleteDeployCmd := exec.Command("kubectl", "delete", "deployment",
		"workflowexecution-controller",
		"-n", namespace,
		"--kubeconfig", kubeconfigPath,
		"--ignore-not-found=true")
	deleteDeployCmd.Stdout = output
	deleteDeployCmd.Stderr = output
	_ = deleteDeployCmd.Run() // Ignore errors

	deleteSvcCmd := exec.Command("kubectl", "delete", "service",
		"workflowexecution-controller-metrics",
		"-n", namespace,
		"--kubeconfig", kubeconfigPath,
		"--ignore-not-found=true")
	deleteSvcCmd.Stdout = output
	deleteSvcCmd.Stderr = output
	_ = deleteSvcCmd.Run() // Ignore errors

	// Deploy controller programmatically with E2E coverage support (DD-TEST-007)
	_, _ = fmt.Fprintf(output, "  Deploying controller programmatically (E2E coverage support)...\n")
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
	defer os.Remove(tmpFile.Name())

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

func deployWorkflowExecutionControllerDeployment(ctx context.Context, namespace, kubeconfigPath, imageName string, output io.Writer) error {
	// Per Consolidated API Migration (January 2026):
	// Accept dynamic image name as parameter (built by BuildImageForKind)
	// Authority: docs/handoff/CONSOLIDATED_API_MIGRATION_GUIDE_JAN07.md
	clientset, err := getKubernetesClient(kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to get Kubernetes client: %w", err)
	}

	replicas := int32(1)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "workflowexecution-controller",
			Namespace: namespace,
			Labels: map[string]string{
				"app": "workflowexecution-controller",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "workflowexecution-controller",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "workflowexecution-controller",
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "workflowexecution-controller",
					// DD-TEST-007: Run as root for E2E coverage (simplified permissions)
					// Per SP/DS team guidance: non-root user may not have permission to write /coverdata
					SecurityContext: func() *corev1.PodSecurityContext {
						if os.Getenv("E2E_COVERAGE") == "true" {
							runAsUser := int64(0)
							runAsGroup := int64(0)
							return &corev1.PodSecurityContext{
								RunAsUser:  &runAsUser,
								RunAsGroup: &runAsGroup,
							}
						}
						return nil
					}(),
					Containers: []corev1.Container{
						{
							Name:            "controller",
							Image:           imageName,        // Per Consolidated API Migration (January 2026)
							ImagePullPolicy: corev1.PullNever, // DD-REGISTRY-001: Use local image loaded into Kind
							Args: []string{
								"--metrics-bind-address=:9090",
								"--health-probe-bind-address=:8081",
								"--execution-namespace=kubernaut-workflows",
								"--cooldown-period=1", // Short cooldown for E2E tests (1 minute)
								"--service-account=kubernaut-workflow-runner",
								"--datastorage-url=http://datastorage.kubernaut-system:8080", // BR-WE-005: Audit events
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "metrics",
									ContainerPort: 9090,
								},
								{
									Name:          "health",
									ContainerPort: 8081,
								},
							},
							Env: func() []corev1.EnvVar {
								envVars := []corev1.EnvVar{}
								// DD-TEST-007: E2E Coverage Capture Standard
								// Only add GOCOVERDIR if E2E_COVERAGE=true
								// MUST match Kind extraMounts path: /coverdata
								coverageEnabled := os.Getenv("E2E_COVERAGE") == "true"
								_, _ = fmt.Fprintf(output, "   🔍 DD-TEST-007: E2E_COVERAGE=%s (enabled=%v)\n", os.Getenv("E2E_COVERAGE"), coverageEnabled)
								if coverageEnabled {
									_, _ = fmt.Fprintf(output, "   ✅ Adding GOCOVERDIR=/coverdata to WorkflowExecution deployment\n")
									envVars = append(envVars, corev1.EnvVar{
										Name:  "GOCOVERDIR",
										Value: "/coverdata",
									})
								} else {
									_, _ = fmt.Fprintf(output, "   ⚠️  E2E_COVERAGE not set, skipping GOCOVERDIR\n")
								}
								return envVars
							}(),
							VolumeMounts: func() []corev1.VolumeMount {
								mounts := []corev1.VolumeMount{}
								// DD-TEST-007: Add coverage volume mount if enabled
								// MUST match Kind extraMounts path: /coverdata
								if os.Getenv("E2E_COVERAGE") == "true" {
									mounts = append(mounts, corev1.VolumeMount{
										Name:      "coverage",
										MountPath: "/coverdata",
										ReadOnly:  false,
									})
								}
								return mounts
							}(),
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/healthz",
										Port: intstr.FromString("health"),
									},
								},
								InitialDelaySeconds: 15,
								PeriodSeconds:       20,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/readyz",
										Port: intstr.FromString("health"),
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       10,
							},
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("64Mi"),
								},
							},
						},
					},
					Volumes: func() []corev1.Volume {
						volumes := []corev1.Volume{}
						// DD-TEST-007: Add hostPath volume for coverage if enabled
						// MUST match Kind extraMounts path: /coverdata
						if os.Getenv("E2E_COVERAGE") == "true" {
							volumes = append(volumes, corev1.Volume{
								Name: "coverage",
								VolumeSource: corev1.VolumeSource{
									HostPath: &corev1.HostPathVolumeSource{
										Path: "/coverdata",
										Type: func() *corev1.HostPathType {
											t := corev1.HostPathDirectoryOrCreate
											return &t
										}(),
									},
								},
							})
						}
						return volumes
					}(),
				},
			},
		},
	}

	// Create Deployment
	_, _ = fmt.Fprintf(output, "   Creating Deployment/workflowexecution-controller...\n")
	_, _ = fmt.Fprintf(output, "   📊 Debug: Image=%s, ImagePullPolicy=%s\n",
		deployment.Spec.Template.Spec.Containers[0].Image,
		deployment.Spec.Template.Spec.Containers[0].ImagePullPolicy)

	createdDep, err := clientset.AppsV1().Deployments(namespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create deployment: %w", err)
	}
	_, _ = fmt.Fprintf(output, "   ✅ Deployment created (UID: %s)\n", createdDep.UID)

	// Wait and check status
	time.Sleep(3 * time.Second)

	// Check Pods
	podList, _ := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app=workflowexecution-controller",
	})
	_, _ = fmt.Fprintf(output, "   📊 Debug: Found %d pod(s) after 3s\n", len(podList.Items))
	for _, pod := range podList.Items {
		_, _ = fmt.Fprintf(output, "      Pod %s: Phase=%s\n", pod.Name, pod.Status.Phase)
	}

	// Create Service for metrics
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "workflowexecution-controller-metrics",
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeNodePort,
			Selector: map[string]string{
				"app": "workflowexecution-controller",
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "metrics",
					Port:       9090,
					TargetPort: intstr.FromInt(9090),
					NodePort:   30185, // Exposed on host via Kind extraPortMappings
				},
			},
		},
	}

	_, _ = fmt.Fprintf(output, "   Creating Service/workflowexecution-controller-metrics...\n")
	_, err = clientset.CoreV1().Services(namespace).Create(ctx, service, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}
	_, _ = fmt.Fprintf(output, "   ✅ Service created\n")

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

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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
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
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(writer, "🚀 WorkflowExecution E2E Infrastructure (HYBRID PARALLEL + COVERAGE)")
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(writer, "  Strategy: Build parallel → Create cluster → Load → Deploy")
	fmt.Fprintln(writer, "  Standard: DD-TEST-002 (Parallel Test Execution Standard)")
	fmt.Fprintln(writer, "  Benefits: Fast builds + No cluster timeout + Reliable")
	fmt.Fprintln(writer, "  Per DD-TEST-007: Coverage instrumentation enabled")
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// DD-TEST-007: Create coverdata directory BEFORE everything
	projectRoot, err := findProjectRoot()
	if err != nil {
		return fmt.Errorf("failed to find project root: %w", err)
	}

	if os.Getenv("E2E_COVERAGE") == "true" {
		coverdataPath := filepath.Join(projectRoot, "test/e2e/workflowexecution/coverdata")
		fmt.Fprintf(writer, "📁 Creating coverage directory: %s\n", coverdataPath)
		if err := os.MkdirAll(coverdataPath, 0777); err != nil {
			return fmt.Errorf("failed to create coverdata directory: %w", err)
		}
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 0: Generate dynamic image tags (BEFORE building)
	// ═══════════════════════════════════════════════════════════════════════
	// Generate DataStorage image tag ONCE (non-idempotent, timestamp-based)
	// This ensures each service builds its OWN DataStorage with LATEST code
	// Per DD-TEST-001: Dynamic tags for parallel E2E isolation
	dataStorageImageName := GenerateInfraImageName("datastorage", "workflowexecution")
	fmt.Fprintf(writer, "📛 DataStorage dynamic tag: %s\n", dataStorageImageName)
	fmt.Fprintln(writer, "   (Ensures fresh build with latest DataStorage code)")

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 1: Build images IN PARALLEL (before cluster creation)
	// ═══════════════════════════════════════════════════════════════════════
	fmt.Fprintln(writer, "\n📦 PHASE 1: Building images in parallel...")
	fmt.Fprintln(writer, "  ├── WorkflowExecution controller (WITH COVERAGE)")
	fmt.Fprintln(writer, "  └── DataStorage image (WITH DYNAMIC TAG)")
	fmt.Fprintln(writer, "  ⏱️  Expected: ~2-3 minutes (parallel)")

	type buildResult struct {
		name string
		err  error
	}

	buildResults := make(chan buildResult, 2)

	// Build WorkflowExecution controller with coverage in parallel
	go func() {
		err := BuildWorkflowExecutionImageWithCoverage(projectRoot, writer)
		buildResults <- buildResult{name: "WorkflowExecution (coverage)", err: err}
	}()

	// Build DataStorage with dynamic tag in parallel
	go func() {
		err := buildDataStorageImageWithTag(dataStorageImageName, writer)
		buildResults <- buildResult{name: "DataStorage", err: err}
	}()

	// Wait for both builds to complete
	fmt.Fprintln(writer, "\n⏳ Waiting for both builds to complete...")
	var buildErrors []error
	for i := 0; i < 2; i++ {
		result := <-buildResults
		if result.err != nil {
			fmt.Fprintf(writer, "  ❌ %s build failed: %v\n", result.name, result.err)
			buildErrors = append(buildErrors, result.err)
		} else {
			fmt.Fprintf(writer, "  ✅ %s build completed\n", result.name)
		}
	}

	if len(buildErrors) > 0 {
		return fmt.Errorf("image builds failed: %v", buildErrors)
	}

	fmt.Fprintln(writer, "\n✅ All images built successfully!")

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 2: Create Kind cluster (now that images are ready)
	// ═══════════════════════════════════════════════════════════════════════
	fmt.Fprintln(writer, "\n📦 PHASE 2: Creating Kind cluster...")
	fmt.Fprintln(writer, "  ⏱️  Expected: ~15-20 seconds")

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
	fmt.Fprintln(writer, "📋 Installing WorkflowExecution CRD...")
	crdPath := filepath.Join(projectRoot, "config/crd/bases/kubernaut.ai_workflowexecutions.yaml")
	crdCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", crdPath)
	crdCmd.Stdout = writer
	crdCmd.Stderr = writer
	if err := crdCmd.Run(); err != nil {
		return fmt.Errorf("failed to install WorkflowExecution CRD: %w", err)
	}

	// Create namespaces
	fmt.Fprintf(writer, "📁 Creating namespace %s...\n", WorkflowExecutionNamespace)
	nsCmd := exec.Command("kubectl", "create", "namespace", WorkflowExecutionNamespace,
		"--kubeconfig", kubeconfigPath)
	nsCmd.Stdout = writer
	nsCmd.Stderr = writer
	_ = nsCmd.Run() // May already exist

	fmt.Fprintf(writer, "📁 Creating namespace %s...\n", ExecutionNamespace)
	execNsCmd := exec.Command("kubectl", "create", "namespace", ExecutionNamespace,
		"--kubeconfig", kubeconfigPath)
	execNsCmd.Stdout = writer
	execNsCmd.Stderr = writer
	_ = execNsCmd.Run() // May already exist

	fmt.Fprintln(writer, "\n✅ Kind cluster ready!")

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 3: Load images into fresh cluster (parallel)
	// ═══════════════════════════════════════════════════════════════════════
	fmt.Fprintln(writer, "\n📦 PHASE 3: Loading images into Kind cluster...")
	fmt.Fprintln(writer, "  ├── WorkflowExecution controller (coverage)")
	fmt.Fprintln(writer, "  └── DataStorage image (with dynamic tag)")
	fmt.Fprintln(writer, "  ⏱️  Expected: ~30-45 seconds")

	loadResults := make(chan buildResult, 2)

	// Load WorkflowExecution controller image
	go func() {
		err := LoadWorkflowExecutionCoverageImage(clusterName, projectRoot, writer)
		loadResults <- buildResult{name: "WorkflowExecution coverage", err: err}
	}()

	// Load DataStorage image with dynamic tag
	go func() {
		err := loadDataStorageImageWithTag(clusterName, dataStorageImageName, writer)
		loadResults <- buildResult{name: "DataStorage", err: err}
	}()

	// Wait for both loads to complete
	fmt.Fprintln(writer, "\n⏳ Waiting for images to load...")
	var loadErrors []error
	for i := 0; i < 2; i++ {
		result := <-loadResults
		if result.err != nil {
			fmt.Fprintf(writer, "  ❌ %s load failed: %v\n", result.name, result.err)
			loadErrors = append(loadErrors, result.err)
		} else {
			fmt.Fprintf(writer, "  ✅ %s loaded\n", result.name)
		}
	}

	if len(loadErrors) > 0 {
		return fmt.Errorf("image loads failed: %v", loadErrors)
	}

	fmt.Fprintln(writer, "\n✅ All images loaded into cluster!")

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 4: Deploy services in PARALLEL (DD-TEST-002 MANDATE)
	// ═══════════════════════════════════════════════════════════════════════
	// NOTE: Using dataStorageImageName from Phase 0 (generated BEFORE build)
	// This ensures we deploy the SAME image we just built with latest code
	fmt.Fprintln(writer, "\n📦 PHASE 4: Deploying services in parallel...")
	fmt.Fprintln(writer, "  (Kubernetes will handle dependencies and reconciliation)")

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
		// CRITICAL: Use the tag generated in Phase 0 (UUID-based, non-idempotent)
		// Per DD-TEST-001: Dynamic tags for parallel E2E isolation
		// This ensures we deploy the SAME fresh-built image with latest DataStorage code
		err := deployDataStorageServiceInNamespace(ctx, WorkflowExecutionNamespace, kubeconfigPath, dataStorageImageName, writer)
		deployResults <- deployResult{"DataStorage", err}
	}()
	go func() {
		err := DeployWorkflowExecutionController(ctx, WorkflowExecutionNamespace, kubeconfigPath, writer)
		deployResults <- deployResult{"WorkflowExecution", err}
	}()

	// Collect ALL results before proceeding (MANDATORY)
	var deployErrors []error
	for i := 0; i < 6; i++ {
		result := <-deployResults
		if result.err != nil {
			fmt.Fprintf(writer, "  ❌ %s deployment failed: %v\n", result.name, result.err)
			deployErrors = append(deployErrors, result.err)
		} else {
			fmt.Fprintf(writer, "  ✅ %s manifests applied\n", result.name)
		}
	}

	if len(deployErrors) > 0 {
		return fmt.Errorf("one or more service deployments failed: %v", deployErrors)
	}
	fmt.Fprintln(writer, "  ✅ All manifests applied! (Kubernetes reconciling...)")

	// Single wait for ALL services ready (Kubernetes handles dependencies)
	fmt.Fprintln(writer, "\n⏳ Waiting for all services to be ready (Kubernetes reconciling dependencies)...")
	if err := waitForWEServicesReady(ctx, WorkflowExecutionNamespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("services not ready: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// POST-DEPLOYMENT: Build workflow bundles & create pipeline (requires DataStorage ready)
	// ═══════════════════════════════════════════════════════════════════════
	fmt.Fprintln(writer, "\n🎯 Building and registering test workflow bundles...")
	dataStorageURL := "http://localhost:8081" // NodePort per DD-TEST-001
	if _, err := BuildAndRegisterTestWorkflows(clusterName, kubeconfigPath, dataStorageURL, writer); err != nil {
		return fmt.Errorf("failed to build and register test workflows: %w", err)
	}

	fmt.Fprintln(writer, "\n📋 Creating test pipeline...")
	if err := CreateSimpleTestPipeline(kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create test pipeline: %w", err)
	}

	fmt.Fprintln(writer, "\n🔑 Creating image pull secret...")
	if err := createQuayPullSecret(kubeconfigPath, ExecutionNamespace, writer); err != nil {
		fmt.Fprintf(writer, "⚠️  Warning: Could not create quay.io pull secret: %v\n", err)
		// Non-fatal - repos may be public
	}

	fmt.Fprintln(writer, "\n✅ All services ready and configured!")

	fmt.Fprintln(writer, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(writer, "✅ WorkflowExecution E2E Infrastructure Ready!")
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(writer, "  🚀 Strategy: Hybrid parallel (build parallel → cluster → load)")
	fmt.Fprintln(writer, "  📊 Coverage: Enabled (GOCOVERDIR=/coverdata)")
	fmt.Fprintln(writer, "  🎯 DataStorage URL: http://localhost:8081")
	fmt.Fprintln(writer, "  📦 Namespace: kubernaut-system")
	fmt.Fprintln(writer, "  ⏱️  Total time: ~5-6 minutes (per DD-TEST-002)")
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	return nil
}

// BuildWorkflowExecutionImageWithCoverage builds the WorkflowExecution controller image with coverage instrumentation
func BuildWorkflowExecutionImageWithCoverage(projectRoot string, writer io.Writer) error {
	fmt.Fprintln(writer, "  🔨 Building WorkflowExecution controller image (with coverage)...")

	dockerfilePath := filepath.Join(projectRoot, "cmd/workflowexecution/Dockerfile")
	imageName := "localhost/kubernaut-workflowexecution:e2e-test-workflowexecution"

	buildArgs := []string{
		"build",
		"--no-cache", // DD-TEST-002: Force fresh build to include latest code changes
		"-t", imageName,
		"-f", dockerfilePath,
		"--build-arg", fmt.Sprintf("GOARCH=%s", runtime.GOARCH),
	}

	// DD-TEST-007: E2E Coverage Collection
	if os.Getenv("E2E_COVERAGE") == "true" {
		buildArgs = append(buildArgs, "--build-arg", "GOFLAGS=-cover")
		fmt.Fprintln(writer, "     📊 Building with coverage instrumentation (GOFLAGS=-cover)")
	}

	buildArgs = append(buildArgs, projectRoot)

	buildCmd := exec.Command("podman", buildArgs...)
	buildCmd.Stdout = writer
	buildCmd.Stderr = writer
	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("failed to build WorkflowExecution controller image: %w", err)
	}

	fmt.Fprintln(writer, "     ✅ WorkflowExecution controller image built")
	return nil
}

// LoadWorkflowExecutionCoverageImage loads the WorkflowExecution controller image into Kind cluster
func LoadWorkflowExecutionCoverageImage(clusterName, projectRoot string, writer io.Writer) error {
	fmt.Fprintln(writer, "  📦 Loading WorkflowExecution controller image into Kind...")

	imageName := "localhost/kubernaut-workflowexecution:e2e-test-workflowexecution"

	// Save image to tarball for Kind loading (Podman images need explicit save/load)
	tarPath := filepath.Join(projectRoot, "workflowexecution-controller.tar")
	saveCmd := exec.Command("podman", "save", "-o", tarPath, imageName)
	saveCmd.Stdout = writer
	saveCmd.Stderr = writer
	if err := saveCmd.Run(); err != nil {
		return fmt.Errorf("failed to save controller image: %w", err)
	}
	defer os.Remove(tarPath)

	// Load image into Kind from tarball
	loadCmd := exec.Command("kind", "load", "image-archive", tarPath,
		"--name", clusterName,
	)
	loadCmd.Stdout = writer
	loadCmd.Stderr = writer
	if err := loadCmd.Run(); err != nil {
		return fmt.Errorf("failed to load image into Kind: %w", err)
	}

	fmt.Fprintln(writer, "     ✅ WorkflowExecution controller image loaded")
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
	fmt.Fprintf(writer, "   ⏳ Waiting for DataStorage pod to be ready...\n")
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
	fmt.Fprintf(writer, "   ✅ DataStorage ready\n")

	// Wait for WorkflowExecution controller pod to be ready
	fmt.Fprintf(writer, "   ⏳ Waiting for WorkflowExecution controller pod to be ready...\n")
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
	fmt.Fprintf(writer, "   ✅ WorkflowExecution controller ready\n")

	return nil
}

// buildDataStorageImageWithTag and loadDataStorageImageWithTag are defined in datastorage.go
// and shared across all E2E infrastructure files in this package

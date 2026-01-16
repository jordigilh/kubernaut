package infrastructure

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// SignalProcessing Integration test ports (restored from git history)
const (
// SignalProcessingIntegrationDataStoragePort and SignalProcessingIntegrationMetricsPort
// are defined in signalprocessing_integration.go to avoid duplicate declarations
)

// SetupSignalProcessingInfrastructureHybridWithCoverage implements hybrid parallel strategy
// per DD-TEST-002: Parallel Test Execution Standard (validated Dec 25, 2025)
//
// Strategy (AUTHORITATIVE):
// 1. Build images in PARALLEL (FASTEST - both build simultaneously)
// 2. Create Kind cluster AFTER builds complete (NO IDLE TIMEOUT)
// 3. Load images immediately into fresh cluster (RELIABLE)
// 4. Deploy services (parallel)
//
// This combines the best of sequential (no timeouts) and parallel (speed):
// - Images build in parallel: ~2-3 minutes (not 7 sequential minutes)
// - Cluster created when ready: No idle time, no timeout risk
// - Total time: ~5-6 minutes (faster than sequential, more reliable than old parallel)
//
// Per DD-TEST-007: E2E Coverage Capture Standard
func SetupSignalProcessingInfrastructureHybridWithCoverage(ctx context.Context, clusterName, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "🚀 SignalProcessing E2E Infrastructure (HYBRID PARALLEL + COVERAGE)")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "  Strategy: Build parallel → Create cluster → Load → Deploy")
	_, _ = fmt.Fprintln(writer, "  Benefits: Fast builds + No cluster timeout + Reliable")
	_, _ = fmt.Fprintln(writer, "  Per DD-TEST-007: Coverage instrumentation enabled")
	_, _ = fmt.Fprintln(writer, "  Per DD-TEST-001: Port 30082 (API), 30182 (Metrics)")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	namespace := "kubernaut-system"

	// DD-TEST-007: Create coverdata directory BEFORE everything
	projectRoot := getProjectRoot()
	coverdataPath := filepath.Join(projectRoot, "coverdata")
	_, _ = fmt.Fprintf(writer, "📁 Creating coverage directory: %s\n", coverdataPath)
	if err := os.MkdirAll(coverdataPath, 0777); err != nil {
		return fmt.Errorf("failed to create coverdata directory: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 1: Build images IN PARALLEL (before cluster creation)
	// ═══════════════════════════════════════════════════════════════════════
	// Per Consolidated API Migration (January 2026):
	// - Uses BuildImageForKind() for all images
	// - Returns dynamic image names for later use
	// - No manual tag generation (PHASE 0 removed)
	// - Authority: docs/handoff/CONSOLIDATED_API_MIGRATION_GUIDE_JAN07.md
	phase1Start := time.Now()
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 1: Building images in parallel...")
	_, _ = fmt.Fprintf(writer, "  ⏱️  Start: %s\n", phase1Start.Format("15:04:05.000"))
	_, _ = fmt.Fprintln(writer, "  ├── SignalProcessing controller (WITH COVERAGE)")
	_, _ = fmt.Fprintln(writer, "  └── DataStorage image")
	_, _ = fmt.Fprintln(writer, "  ⏱️  Expected: ~2-3 minutes (parallel)")

	type buildResult struct {
		name      string
		imageName string
		err       error
	}

	buildResults := make(chan buildResult, 2)
	builtImages := make(map[string]string)

	// Build SignalProcessing with coverage in parallel
	go func() {
		cfg := E2EImageConfig{
			ServiceName:      "signalprocessing-controller",
			ImageName:        "kubernaut/signalprocessing-controller",
			DockerfilePath:   "docker/signalprocessing-controller.Dockerfile",
			BuildContextPath: "",
			EnableCoverage:   true,
		}
		imageName, err := BuildImageForKind(cfg, writer)
		buildResults <- buildResult{name: "SignalProcessing (coverage)", imageName: imageName, err: err}
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

	// Wait for both builds to complete
	_, _ = fmt.Fprintln(writer, "\n⏳ Waiting for both builds to complete...")
	var buildErrors []error
	for i := 0; i < 2; i++ {
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

	phase1End := time.Now()
	phase1Duration := phase1End.Sub(phase1Start)
	_, _ = fmt.Fprintln(writer, "\n✅ All images built successfully!")
	_, _ = fmt.Fprintf(writer, "  ⏱️  Phase 1 Duration: %.1f seconds\n", phase1Duration.Seconds())

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 2: Create Kind cluster (now that images are ready)
	// ═══════════════════════════════════════════════════════════════════════
	phase2Start := time.Now()
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 2: Creating Kind cluster...")
	_, _ = fmt.Fprintf(writer, "  ⏱️  Start: %s\n", phase2Start.Format("15:04:05.000"))
	_, _ = fmt.Fprintln(writer, "  ⏱️  Expected: ~10-15 seconds")

	if err := createSignalProcessingKindCluster(clusterName, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create Kind cluster: %w", err)
	}

	// OPTIMIZATION #2: Install both CRDs in a single kubectl apply (3-5s savings)
	_, _ = fmt.Fprintln(writer, "📋 Installing CRDs (batched: SignalProcessing + RemediationRequest)...")
	if err := installSignalProcessingCRDsBatched(kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to install CRDs (batched): %w", err)
	}

	// Create kubernaut-system namespace
	_, _ = fmt.Fprintf(writer, "📁 Creating namespace %s...\n", namespace)
	if err := createSignalProcessingNamespace(kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	// Deploy Rego policy ConfigMaps
	_, _ = fmt.Fprintln(writer, "📜 Deploying Rego policy ConfigMaps...")
	if err := deploySignalProcessingPolicies(kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy policies: %w", err)
	}

	phase2End := time.Now()
	phase2Duration := phase2End.Sub(phase2Start)
	_, _ = fmt.Fprintln(writer, "\n✅ Kind cluster ready!")
	_, _ = fmt.Fprintf(writer, "  ⏱️  Phase 2 Duration: %.1f seconds\n", phase2Duration.Seconds())

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 3: Load images into fresh cluster (parallel)
	// ═══════════════════════════════════════════════════════════════════════
	// Per Consolidated API Migration (January 2026):
	// - Uses LoadImageToKind() for all images
	// - Uses image names from builtImages map
	// - Authority: docs/handoff/CONSOLIDATED_API_MIGRATION_GUIDE_JAN07.md
	phase3Start := time.Now()
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 3: Loading images into Kind cluster...")
	_, _ = fmt.Fprintf(writer, "  ⏱️  Start: %s\n", phase3Start.Format("15:04:05.000"))
	_, _ = fmt.Fprintln(writer, "  ├── SignalProcessing coverage image")
	_, _ = fmt.Fprintln(writer, "  └── DataStorage image")
	_, _ = fmt.Fprintln(writer, "  ⏱️  Expected: ~30-45 seconds")

	type loadResult struct {
		name string
		err  error
	}

	loadResults := make(chan loadResult, 2)

	// Load SignalProcessing coverage image
	go func() {
		spImage := builtImages["SignalProcessing (coverage)"]
		err := LoadImageToKind(spImage, "signalprocessing-controller", clusterName, writer)
		loadResults <- loadResult{name: "SignalProcessing coverage", err: err}
	}()

	// Load DataStorage image
	go func() {
		dsImage := builtImages["DataStorage"]
		err := LoadImageToKind(dsImage, "datastorage", clusterName, writer)
		loadResults <- loadResult{name: "DataStorage", err: err}
	}()

	// Wait for both loads to complete
	_, _ = fmt.Fprintln(writer, "\n⏳ Waiting for images to load...")
	var loadErrors []error
	for i := 0; i < 2; i++ {
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

	phase3End := time.Now()
	phase3Duration := phase3End.Sub(phase3Start)
	_, _ = fmt.Fprintln(writer, "\n✅ All images loaded into cluster!")
	_, _ = fmt.Fprintf(writer, "  ⏱️  Phase 3 Duration: %.1f seconds\n", phase3Duration.Seconds())

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 4: Deploy services in PARALLEL (DD-TEST-002 MANDATE)
	// ═══════════════════════════════════════════════════════════════════════
	phase4Start := time.Now()
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 4: Deploying services in parallel...")
	_, _ = fmt.Fprintf(writer, "  ⏱️  Start: %s\n", phase4Start.Format("15:04:05.000"))
	_, _ = fmt.Fprintln(writer, "  (Kubernetes will handle dependencies and reconciliation)")

	type deployResult struct {
		name string
		err  error
	}
	deployResults := make(chan deployResult, 5)

	// Launch ALL kubectl apply commands concurrently
	go func() {
		err := deployPostgreSQLInNamespace(ctx, namespace, kubeconfigPath, writer)
		deployResults <- deployResult{"PostgreSQL", err}
	}()
	go func() {
		err := deployRedisInNamespace(ctx, namespace, kubeconfigPath, writer)
		deployResults <- deployResult{"Redis", err}
	}()
	go func() {
		err := ApplyAllMigrations(ctx, namespace, kubeconfigPath, writer)
		deployResults <- deployResult{"Migrations", err}
	}()
	go func() {
		// Per Consolidated API Migration (January 2026):
		// Use DataStorage image name from builtImages map (built in Phase 1)
		dsImage := builtImages["DataStorage"]
		err := deployDataStorageServiceInNamespace(ctx, namespace, kubeconfigPath, dsImage, writer)
		deployResults <- deployResult{"DataStorage", err}
	}()
	go func() {
		// Per Consolidated API Migration (January 2026):
		// Use SignalProcessing image name from builtImages map (built in Phase 1)
		spImage := builtImages["SignalProcessing (coverage)"]
		err := DeploySignalProcessingControllerWithCoverage(kubeconfigPath, spImage, writer)
		deployResults <- deployResult{"SignalProcessing", err}
	}()

	// Collect ALL results before proceeding (MANDATORY)
	var deployErrors []error
	for i := 0; i < 5; i++ {
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
	if err := waitForSPServicesReady(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("services not ready: %w", err)
	}

	phase4End := time.Now()
	phase4Duration := phase4End.Sub(phase4Start)
	totalDuration := phase4End.Sub(phase1Start)
	_, _ = fmt.Fprintln(writer, "\n✅ All services ready!")
	_, _ = fmt.Fprintf(writer, "  ⏱️  Phase 4 Duration: %.1f seconds\n", phase4Duration.Seconds())

	_, _ = fmt.Fprintln(writer, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "✅ SignalProcessing E2E Infrastructure Ready!")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "  🚀 Strategy: Hybrid parallel (build parallel → cluster → load)")
	_, _ = fmt.Fprintln(writer, "  📊 Coverage: Enabled (GOCOVERDIR=/coverdata)")
	_, _ = fmt.Fprintln(writer, "  🎯 SP API: http://localhost:30082")
	_, _ = fmt.Fprintln(writer, "  📊 SP Metrics: http://localhost:30182")
	_, _ = fmt.Fprintln(writer, "  📦 Namespace: kubernaut-system")
	_, _ = fmt.Fprintln(writer, "")
	_, _ = fmt.Fprintln(writer, "⏱️  PROFILING SUMMARY (per SP_E2E_OPTIMIZATION_TRIAGE_DEC_25_2025.md):")
	_, _ = fmt.Fprintf(writer, "  Phase 1 (Build Images):     %.1fs\n", phase1Duration.Seconds())
	_, _ = fmt.Fprintf(writer, "  Phase 2 (Create Cluster):   %.1fs\n", phase2Duration.Seconds())
	_, _ = fmt.Fprintf(writer, "  Phase 3 (Load Images):      %.1fs\n", phase3Duration.Seconds())
	_, _ = fmt.Fprintf(writer, "  Phase 4 (Deploy Services):  %.1fs\n", phase4Duration.Seconds())
	_, _ = fmt.Fprintf(writer, "  TOTAL SETUP TIME:           %.1fs (%.1f min)\n", totalDuration.Seconds(), totalDuration.Minutes())
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	return nil
}

// waitForSPServicesReady waits for DataStorage and SignalProcessing pods to be ready
// Per DD-TEST-002: Single readiness check after parallel deployment
func waitForSPServicesReady(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
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

	// Wait for SignalProcessing pod to be ready (coverage-enabled may take longer)
	_, _ = fmt.Fprintf(writer, "   ⏳ Waiting for SignalProcessing pod to be ready...\n")
	Eventually(func() bool {
		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app=signalprocessing-controller",
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
	}, 3*time.Minute, 5*time.Second).Should(BeTrue(), "SignalProcessing pod should become ready")
	_, _ = fmt.Fprintf(writer, "   ✅ SignalProcessing ready\n")

	return nil
}

// ============================================================================
// Missing E2E Infrastructure Functions (Restored from git history)
// ============================================================================

// BuildSignalProcessingImageWithCoverage builds the SignalProcessing image with coverage instrumentation
// This is used by E2E tests to enable coverage collection
func BuildSignalProcessingImageWithCoverage(writer io.Writer) error {
	projectRoot := getProjectRoot()
	if projectRoot == "" {
		return fmt.Errorf("project root not found")
	}

	dockerfilePath := filepath.Join(projectRoot, "docker", "signalprocessing-ubi9.Dockerfile")
	if _, err := os.Stat(dockerfilePath); os.IsNotExist(err) {
		return fmt.Errorf("SignalProcessing Dockerfile not found at %s", dockerfilePath)
	}

	containerCmd := "podman"
	if _, err := exec.LookPath("podman"); err != nil {
		containerCmd = "docker"
	}

	// Use unique image tag with coverage suffix
	imageTag := "e2e-test-coverage"
	imageName := fmt.Sprintf("localhost/kubernaut-signalprocessing:%s", imageTag)
	_, _ = fmt.Fprintf(writer, "  📦 Building SignalProcessing with coverage: %s\n", imageName)

	// Build with GOFLAGS=-cover for E2E coverage
	// CRITICAL: --no-cache ensures latest code changes are included (DD-TEST-002)
	cmd := exec.Command(containerCmd, "build",
		"--no-cache", // Force fresh build to include latest code changes
		"-t", imageName,
		"-f", dockerfilePath,
		"--build-arg", "GOFLAGS=-cover",
		projectRoot,
	)
	cmd.Stdout = writer
	cmd.Stderr = writer
	cmd.Dir = projectRoot

	return cmd.Run()
}

// createSignalProcessingKindCluster creates a Kind cluster for SignalProcessing E2E tests
// createSignalProcessingKindCluster creates a Kind cluster for SignalProcessing E2E tests
// REFACTORED: Now uses shared CreateKindClusterWithConfig() helper
// Authority: docs/handoff/TEST_INFRASTRUCTURE_REFACTORING_TRIAGE_JAN07.md (Phase 1)
func createSignalProcessingKindCluster(clusterName, kubeconfigPath string, writer io.Writer) error {
	opts := KindClusterOptions{
		ClusterName:    clusterName,
		KubeconfigPath: kubeconfigPath,
		ConfigPath:     "test/infrastructure/kind-signalprocessing-config.yaml",
		WaitTimeout:    "60s",
		DeleteExisting: false,
		ReuseExisting:  true, // Original behavior: reuse if exists
	}
	return CreateKindClusterWithConfig(opts, writer)
}

// ============================================================================
// SignalProcessing E2E Package Variables and Helper Functions (Restored from git history)
// Authority: git show a906a3767~1:test/infrastructure/signalprocessing.go
// ============================================================================

// signalProcessingImageTag holds the unique tag for this test run (set once, reused)
var signalProcessingImageTag string

func installSignalProcessingCRDsBatched(kubeconfigPath string, writer io.Writer) error {
	// Find SignalProcessing CRD file
	spCRDPaths := []string{
		"config/crd/bases/kubernaut.ai_signalprocessings.yaml",
		"../../../config/crd/bases/kubernaut.ai_signalprocessings.yaml",
	}

	var spCRDPath string
	for _, p := range spCRDPaths {
		if _, err := os.Stat(p); err == nil {
			spCRDPath, _ = filepath.Abs(p)
			break
		}
	}

	if spCRDPath == "" {
		return fmt.Errorf("SignalProcessing CRD not found")
	}

	// Find RemediationRequest CRD file
	rrCRDPaths := []string{
		"config/crd/bases/kubernaut.ai_remediationrequests.yaml",
		"../../../config/crd/bases/kubernaut.ai_remediationrequests.yaml",
	}

	var rrCRDPath string
	for _, p := range rrCRDPaths {
		if _, err := os.Stat(p); err == nil {
			rrCRDPath, _ = filepath.Abs(p)
			break
		}
	}

	if rrCRDPath == "" {
		return fmt.Errorf("RemediationRequest CRD not found")
	}

	// Apply both CRDs in a single kubectl call (OPTIMIZATION #2)
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
		"apply", "-f", spCRDPath, "-f", rrCRDPath)
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install CRDs (batched): %w", err)
	}

	// Wait for both CRDs to be established
	_, _ = fmt.Fprintln(writer, "  Waiting for CRDs to be established...")

	// Check SignalProcessing CRD
	for i := 0; i < 30; i++ {
		cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
			"get", "crd", "signalprocessings.kubernaut.ai")
		if err := cmd.Run(); err == nil {
			_, _ = fmt.Fprintln(writer, "  ✓ SignalProcessing CRD established")
			break
		}
		if i == 29 {
			return fmt.Errorf("SignalProcessing CRD not established after 30 seconds")
		}
		time.Sleep(time.Second)
	}

	// Check RemediationRequest CRD
	for i := 0; i < 30; i++ {
		cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
			"get", "crd", "remediationrequests.kubernaut.ai")
		if err := cmd.Run(); err == nil {
			_, _ = fmt.Fprintln(writer, "  ✓ RemediationRequest CRD established")
			return nil
		}
		if i == 29 {
			return fmt.Errorf("RemediationRequest CRD not established after 30 seconds")
		}
		time.Sleep(time.Second)
	}

	return nil
}

func createSignalProcessingNamespace(kubeconfigPath string, writer io.Writer) error {
	manifest := `
apiVersion: v1
kind: Namespace
metadata:
  name: kubernaut-system
  labels:
    app.kubernetes.io/name: kubernaut
    app.kubernetes.io/component: signalprocessing
`
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

func deploySignalProcessingPolicies(kubeconfigPath string, writer io.Writer) error {
	// OPTIMIZATION #1: Batch all 5 Rego ConfigMaps into single kubectl apply
	// Eliminates 4 kubectl invocations + API server round trips (20-40s savings)
	// Per SP_E2E_OPTIMIZATION_TRIAGE_DEC_25_2025.md

	// Combine all 5 policy ConfigMaps into a single YAML manifest
	// NOTE: Using OPA v1.0 syntax with 'if' keyword before rule bodies
	// Includes severity policy (BR-SP-105) for SignalProcessing controller startup
	combinedPolicies := `---
# 1. Environment Classification Policy (BR-SP-051)
# Input: {"namespace": {"name": string, "labels": map}, "signal": {"labels": map}}
# Output: {"environment": string, "source": string}
apiVersion: v1
kind: ConfigMap
metadata:
  name: signalprocessing-environment-policy
  namespace: kubernaut-system
data:
  environment.rego: |
    package signalprocessing.environment

    # Default result: unknown environment
    # OPA v1.0 syntax: requires 'if' keyword before rule body
    default result := {"environment": "unknown", "source": "default"}

    # Primary: Check namespace label kubernaut.ai/environment (BR-SP-051)
    result := {"environment": env, "source": "namespace-labels"} if {
      env := input.namespace.labels["kubernaut.ai/environment"]
      env != ""
    }

    # Fallback: Check namespace name patterns
    result := {"environment": "production", "source": "namespace-name"} if {
      not input.namespace.labels["kubernaut.ai/environment"]
      input.namespace.name == "production"
    }
    result := {"environment": "production", "source": "namespace-name"} if {
      not input.namespace.labels["kubernaut.ai/environment"]
      input.namespace.name == "prod"
    }
    result := {"environment": "staging", "source": "namespace-name"} if {
      not input.namespace.labels["kubernaut.ai/environment"]
      input.namespace.name == "staging"
    }
    result := {"environment": "development", "source": "namespace-name"} if {
      not input.namespace.labels["kubernaut.ai/environment"]
      input.namespace.name == "development"
    }
    result := {"environment": "development", "source": "namespace-name"} if {
      not input.namespace.labels["kubernaut.ai/environment"]
      input.namespace.name == "dev"
    }
---
# 2. Priority Assignment Policy (BR-SP-070)
# Input: {"environment": string, "signal": {"severity": string}}
# Output: {"priority": "P0-P3", "confidence": 0.9}
apiVersion: v1
kind: ConfigMap
metadata:
  name: signalprocessing-priority-policy
  namespace: kubernaut-system
data:
  priority.rego: |
    package signalprocessing.priority

    # Priority assignment based on environment and severity
    # OPA v1.0 syntax: requires 'if' keyword before rule body
    # DD-SEVERITY-001 v1.1: Uses critical/high/medium/low/unknown severity levels
    default result := {"priority": "P3", "confidence": 0.6}

    # Production + critical = P0 (highest urgency)
    result := {"priority": "P0", "confidence": 0.9} if {
      input.environment == "production"
      input.signal.severity == "critical"
    }
    # Production + high = P1 (high urgency) - DD-SEVERITY-001 v1.1
    result := {"priority": "P1", "confidence": 0.9} if {
      input.environment == "production"
      input.signal.severity == "high"
    }
    # Staging + critical = P2 (medium urgency per BR-SP-070)
    result := {"priority": "P2", "confidence": 0.9} if {
      input.environment == "staging"
      input.signal.severity == "critical"
    }
    # Staging + high = P2 (medium urgency) - DD-SEVERITY-001 v1.1
    result := {"priority": "P2", "confidence": 0.9} if {
      input.environment == "staging"
      input.signal.severity == "high"
    }
    # Development = P3 (lowest urgency, regardless of severity)
    result := {"priority": "P3", "confidence": 0.9} if {
      input.environment == "development"
    }
---
# 3. Business Classification Policy (BR-SP-071)
apiVersion: v1
kind: ConfigMap
metadata:
  name: signalprocessing-business-policy
  namespace: kubernaut-system
data:
  business.rego: |
    package signalprocessing.business

    import rego.v1

    # Default: Return "unknown" with low confidence when no specific rule matches
    # Operators MUST define their own default rules.
    default result := {"business_unit": "unknown", "confidence": 0.0, "policy_name": "operator-default"}

    # Example business unit mappings based on namespace labels
    result := {"business_unit": input.namespace.labels["kubernaut.io/business-unit"], "confidence": 0.95, "policy_name": "namespace-label"} if {
      input.namespace.labels["kubernaut.io/business-unit"]
    }
---
# 4. Severity Determination Policy (BR-SP-105, DD-SEVERITY-001)
apiVersion: v1
kind: ConfigMap
metadata:
  name: signalprocessing-severity-policy
  namespace: kubernaut-system
data:
  severity.rego: |
    package signalprocessing.severity
    import rego.v1

    # BR-SP-105: Severity Determination via Rego Policy
    # DD-SEVERITY-001 v1.1: Strategy B - Policy-Defined Fallback + REFACTOR (lowercase normalization)
    # Maps external severity values to normalized values: critical/high/medium/low/unknown
    determine_severity := "critical" if {
      input.signal.severity == "sev1"
    } else := "critical" if {
      input.signal.severity == "p0"
    } else := "critical" if {
      input.signal.severity == "p1"
    } else := "high" if {
      input.signal.severity == "sev2"
    } else := "high" if {
      input.signal.severity == "p2"
    } else := "medium" if {
      input.signal.severity == "sev3"
    } else := "low" if {
      input.signal.severity == "p3"
    } else := "unknown" if {
      # Default fallback for unknown severity values
      # Per DD-SEVERITY-001 v1.1: Unknown external values map to "unknown"
      true
    }
---
# 5. Custom Labels Extraction Policy (BR-SP-071)
apiVersion: v1
kind: ConfigMap
metadata:
  name: signalprocessing-customlabels-policy
  namespace: kubernaut-system
data:
  customlabels.rego: |
    package signalprocessing.customlabels

    import rego.v1

    # Default: Return empty labels when no specific rule matches
    # Operators define their own label extraction rules.
    default result := {"labels": {}, "policy_name": "operator-default"}

    # Example: Extract labels from namespace annotations
    result := {"labels": extracted, "policy_name": "namespace-annotations"} if {
      input.namespace.annotations
      extracted := {k: v | some k, v in input.namespace.annotations; startswith(k, "kubernaut.io/label-")}
      count(extracted) > 0
    }
`

	// Single kubectl apply for all 5 ConfigMaps (includes severity policy for BR-SP-105)
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(combinedPolicies)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to deploy Rego policies (batched): %w", err)
	}

	_, _ = fmt.Fprintln(writer, "  ✓ Rego policies deployed (batched: environment, priority, business, customlabels)")
	return nil
}

func waitForSignalProcessingController(ctx context.Context, kubeconfigPath string, writer io.Writer) error {
	timeout := 120 * time.Second
	interval := 5 * time.Second
	deadline := time.Now().Add(timeout)
	attempt := 0

	for time.Now().Before(deadline) {
		attempt++
		cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
			"rollout", "status", "deployment/signalprocessing-controller",
			"-n", "kubernaut-system", "--timeout=5s")
		if err := cmd.Run(); err == nil {
			_, _ = fmt.Fprintln(writer, "  ✓ Controller ready")
			return nil
		}

		// Print diagnostic info every 5 attempts (25 seconds)
		if attempt%5 == 0 {
			_, _ = fmt.Fprintf(writer, "  ⏳ Controller not ready yet (attempt %d)...\n", attempt)
			// Get pod status
			podCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
				"get", "pods", "-n", "kubernaut-system", "-l", "app=signalprocessing-controller", "-o", "wide")
			podCmd.Stdout = writer
			podCmd.Stderr = writer
			_ = podCmd.Run()

			// Get pod logs (last 10 lines)
			logsCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
				"logs", "-n", "kubernaut-system", "-l", "app=signalprocessing-controller", "--tail=10")
			logsCmd.Stdout = writer
			logsCmd.Stderr = writer
			_ = logsCmd.Run()
		}
		time.Sleep(interval)
	}

	// Final diagnostic dump before failure
	_, _ = fmt.Fprintln(writer, "  ❌ Controller not ready after timeout. Final diagnostics:")
	describeCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
		"describe", "pod", "-n", "kubernaut-system", "-l", "app=signalprocessing-controller")
	describeCmd.Stdout = writer
	describeCmd.Stderr = writer
	_ = describeCmd.Run()

	logsCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
		"logs", "-n", "kubernaut-system", "-l", "app=signalprocessing-controller", "--tail=50")
	logsCmd.Stdout = writer
	logsCmd.Stderr = writer
	_ = logsCmd.Run()

	return fmt.Errorf("controller not ready after %v", timeout)
}

func GetSignalProcessingCoverageImageTag() string {
	return GetSignalProcessingImageTag() + "-coverage"
}

// GetSignalProcessingCoverageFullImageName returns the full image name with coverage tag
func GetSignalProcessingCoverageFullImageName() string {
	return fmt.Sprintf("localhost/signalprocessing-controller:%s", GetSignalProcessingCoverageImageTag())
}

func LoadSignalProcessingCoverageImage(clusterName string, writer io.Writer) error {
	imageTag := GetSignalProcessingCoverageImageTag()
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("signalprocessing-controller-%s.tar", imageTag))
	imageName := GetSignalProcessingCoverageFullImageName()

	_, _ = fmt.Fprintf(writer, "  Saving coverage image to tar file: %s...\n", tmpFile)
	saveCmd := exec.Command("podman", "save",
		"-o", tmpFile,
		imageName,
	)
	saveCmd.Stdout = writer
	saveCmd.Stderr = writer
	if err := saveCmd.Run(); err != nil {
		return fmt.Errorf("failed to save image: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "  Loading coverage image into Kind...")
	loadCmd := exec.Command("kind", "load", "image-archive",
		tmpFile,
		"--name", clusterName,
	)
	loadCmd.Stdout = writer
	loadCmd.Stderr = writer
	if err := loadCmd.Run(); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed to load image: %w", err)
	}

	_ = os.Remove(tmpFile)

	// CRITICAL: Remove Podman image immediately to free disk space
	// Image is now in Kind, Podman copy is duplicate
	_, _ = fmt.Fprintf(writer, "  🗑️  Removing Podman image to free disk space...\n")
	rmiCmd := exec.Command("podman", "rmi", "-f", imageName)
	rmiCmd.Stdout = writer
	rmiCmd.Stderr = writer
	if err := rmiCmd.Run(); err != nil {
		_, _ = fmt.Fprintf(writer, "  ⚠️  Failed to remove Podman image (non-fatal): %v\n", err)
	} else {
		_, _ = fmt.Fprintf(writer, "  ✅ Podman image removed: %s\n", imageName)
	}

	_, _ = fmt.Fprintf(writer, "  ✅ Coverage image loaded and temp file cleaned\n")
	return nil
}

func signalProcessingControllerCoverageManifest(imageName string) string {
	// Per Consolidated API Migration (January 2026):
	// Accept dynamic image name as parameter (built by BuildImageForKind)
	// No longer generates own tag - uses pre-built image
	// Authority: docs/handoff/CONSOLIDATED_API_MIGRATION_GUIDE_JAN07.md

	return fmt.Sprintf(`
apiVersion: v1
kind: ServiceAccount
metadata:
  name: signalprocessing-controller
  namespace: kubernaut-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: signalprocessing-controller
rules:
- apiGroups: ["kubernaut.ai"]
  resources: ["signalprocessings", "remediationrequests"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["kubernaut.ai"]
  resources: ["signalprocessings/status", "signalprocessings/finalizers"]
  verbs: ["get", "update", "patch"]
- apiGroups: [""]
  resources: ["pods", "services", "namespaces", "nodes", "configmaps", "secrets", "events"]
  verbs: ["get", "list", "watch", "create", "update", "patch"]
- apiGroups: ["apps"]
  resources: ["deployments", "replicasets", "statefulsets", "daemonsets"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["autoscaling"]
  resources: ["horizontalpodautoscalers"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["policy"]
  resources: ["poddisruptionbudgets"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["networking.k8s.io"]
  resources: ["networkpolicies"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["coordination.k8s.io"]
  resources: ["leases"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: signalprocessing-controller
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: signalprocessing-controller
subjects:
- kind: ServiceAccount
  name: signalprocessing-controller
  namespace: kubernaut-system
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: signalprocessing-controller
  namespace: kubernaut-system
  labels:
    app: signalprocessing-controller
spec:
  replicas: 1
  selector:
    matchLabels:
      app: signalprocessing-controller
  template:
    metadata:
      labels:
        app: signalprocessing-controller
    spec:
      serviceAccountName: signalprocessing-controller
      terminationGracePeriodSeconds: 30
      # E2E Coverage: Run as root to write to hostPath volume (acceptable for E2E tests)
      securityContext:
        runAsUser: 0
        runAsGroup: 0
      containers:
      - name: controller
        image: %s
        imagePullPolicy: Never
        env:
        - name: NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        # BR-SP-090: Point to DataStorage service in kubernaut-system namespace
        - name: DATA_STORAGE_URL
          value: "http://datastorage.kubernaut-system.svc.cluster.local:8080"
        # E2E Coverage: Set GOCOVERDIR to enable coverage capture
        - name: GOCOVERDIR
          value: /coverdata
        ports:
        - containerPort: 9090
          name: metrics
        - containerPort: 8081
          name: health
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8081
          initialDelaySeconds: 15
          periodSeconds: 20
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8081
          initialDelaySeconds: 5
          periodSeconds: 10
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 500m
            memory: 256Mi
        volumeMounts:
        # Mount policies at /etc/signalprocessing/policies (same as standard manifest)
        - name: policies
          mountPath: /etc/signalprocessing/policies
          readOnly: true
        # E2E Coverage: Mount coverage directory
        - name: coverdata
          mountPath: /coverdata
      volumes:
      # Projected volume for all policies (includes severity policy for BR-SP-105)
      - name: policies
        projected:
          sources:
          - configMap:
              name: signalprocessing-environment-policy
          - configMap:
              name: signalprocessing-priority-policy
          - configMap:
              name: signalprocessing-business-policy
          - configMap:
              name: signalprocessing-severity-policy
          - configMap:
              name: signalprocessing-customlabels-policy
      # E2E Coverage: hostPath volume for coverage data
      - name: coverdata
        hostPath:
          path: /coverdata
          type: DirectoryOrCreate
---
apiVersion: v1
kind: Service
metadata:
  name: signalprocessing-controller-metrics
  namespace: kubernaut-system
  labels:
    app: signalprocessing-controller
spec:
  type: NodePort
  ports:
  - port: 9090
    targetPort: 9090
    nodePort: 30182
    name: metrics
  selector:
    app: signalprocessing-controller
`, imageName)
}

func DeploySignalProcessingControllerWithCoverage(kubeconfigPath, imageName string, writer io.Writer) error {
	// Per Consolidated API Migration (January 2026):
	// Accept dynamic image name as parameter (built by BuildImageForKind)
	// Authority: docs/handoff/CONSOLIDATED_API_MIGRATION_GUIDE_JAN07.md
	manifest := signalProcessingControllerCoverageManifest(imageName)

	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to apply coverage controller manifest: %w", err)
	}

	// Wait for controller to be ready
	_, _ = fmt.Fprintln(writer, "⏳ Waiting for coverage-enabled controller to be ready...")
	ctx := context.Background()
	return waitForSignalProcessingController(ctx, kubeconfigPath, writer)
}

func GetSignalProcessingImageTag() string {
	// Check if already set (avoid regenerating)
	if signalProcessingImageTag != "" {
		return signalProcessingImageTag
	}

	// Check if IMAGE_TAG env var is set (from Makefile or CI)
	if tag := os.Getenv("IMAGE_TAG"); tag != "" {
		signalProcessingImageTag = tag
		return tag
	}

	// Generate unique tag per DD-TEST-001: {service}-{user}-{git-hash}-{timestamp}
	user := os.Getenv("USER")
	if user == "" {
		user = "unknown"
	}

	gitHash := getSignalProcessingGitHash()
	timestamp := time.Now().Unix()

	signalProcessingImageTag = fmt.Sprintf("signalprocessing-%s-%s-%d", user, gitHash, timestamp)

	// Export for cleanup in AfterSuite
	_ = os.Setenv("IMAGE_TAG", signalProcessingImageTag)

	return signalProcessingImageTag
}

func getSignalProcessingGitHash() string {
	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}

func GetProjectRoot() (string, error) {
	root := findSignalProcessingProjectRoot()
	if root == "" {
		return "", fmt.Errorf("project root not found (go.mod not found)")
	}
	return root, nil
}

func ExtractCoverageFromKind(clusterName, coverDir string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "📦 Extracting coverage data from Kind node...")

	// Get the worker node container name
	workerNode := clusterName + "-worker"

	// Create local coverage directory if not exists
	if err := os.MkdirAll(coverDir, 0755); err != nil {
		return fmt.Errorf("failed to create coverage directory: %w", err)
	}

	// Copy coverage files from Kind node to host
	cmd := exec.Command("docker", "cp",
		workerNode+":/coverdata/.",
		coverDir)
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		// Try with podman if docker fails
		cmd = exec.Command("podman", "cp",
			workerNode+":/coverdata/.",
			coverDir)
		cmd.Stdout = writer
		cmd.Stderr = writer
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to copy coverage data: %w", err)
		}
	}

	// List extracted files
	files, _ := os.ReadDir(coverDir)
	if len(files) == 0 {
		_, _ = fmt.Fprintln(writer, "⚠️  No coverage files found (controller may not have processed any requests)")
	} else {
		_, _ = fmt.Fprintf(writer, "✅ Extracted %d coverage files\n", len(files))
	}

	return nil
}

func GenerateCoverageReport(coverDir string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "📊 Generating E2E coverage report...")

	// Check if coverage data exists
	files, err := os.ReadDir(coverDir)
	if err != nil || len(files) == 0 {
		_, _ = fmt.Fprintln(writer, "⚠️  No coverage data to report")
		return nil
	}

	// Generate percent summary
	cmd := exec.Command("go", "tool", "covdata", "percent", "-i="+coverDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to generate coverage percent: %w\n%s", err, output)
	}
	_, _ = fmt.Fprintf(writer, "\n%s\n", output)

	// Generate text format for HTML conversion
	textFile := filepath.Join(coverDir, "e2e-coverage.txt")
	cmd = exec.Command("go", "tool", "covdata", "textfmt",
		"-i="+coverDir,
		"-o="+textFile)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to generate text format: %w", err)
	}

	// Generate HTML report
	htmlFile := filepath.Join(coverDir, "e2e-coverage.html")
	cmd = exec.Command("go", "tool", "cover",
		"-html="+textFile,
		"-o="+htmlFile)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to generate HTML report: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "📄 Text report: %s\n", textFile)
	_, _ = fmt.Fprintf(writer, "📄 HTML report: %s\n", htmlFile)
	_, _ = fmt.Fprintln(writer, "✅ E2E coverage report generated")

	return nil
}

func DeleteSignalProcessingCluster(clusterName, kubeconfigPath string, testsFailed bool, writer io.Writer) error {
	// Use shared cleanup function with log export on failure
	if err := DeleteCluster(clusterName, "signalprocessing", testsFailed, writer); err != nil {
		return err
	}

	// Remove kubeconfig file
	if kubeconfigPath != "" {
		_ = os.Remove(kubeconfigPath)
	}

	return nil
}

func GetSignalProcessingFullImageName() string {
	return fmt.Sprintf("localhost/signalprocessing-controller:%s", GetSignalProcessingImageTag())
}

func GetDataStorageImageTagForSP() string {
	return GenerateInfraImageName("datastorage", "signalprocessing")
}

func findSignalProcessingProjectRoot() string {
	// Try to find go.mod to determine project root
	paths := []string{
		".",
		"..",
		"../..",
		"../../..",
	}
	for _, p := range paths {
		goMod := filepath.Join(p, "go.mod")
		if _, err := os.Stat(goMod); err == nil {
			absPath, _ := filepath.Abs(p)
			return absPath
		}
	}
	return ""
}

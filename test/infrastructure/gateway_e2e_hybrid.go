package infrastructure

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// SetupGatewayInfrastructureHybridWithCoverage implements hybrid parallel strategy:
// 1. Build images in parallel (FASTEST - both build simultaneously)
// 2. Create Kind cluster AFTER builds complete (NO IDLE TIMEOUT)
// 3. Load images immediately into fresh cluster (RELIABLE)
// 4. Deploy services
//
// This combines the best of sequential (no timeouts) and parallel (speed):
// - Images build in parallel: ~2-3 minutes (not 7 sequential minutes)
// - Cluster created when ready: No idle time, no timeout risk
// - Total time: ~5-6 minutes (faster than sequential, more reliable than old parallel)
//
// Per DD-TEST-007: E2E Coverage Capture Standard
func SetupGatewayInfrastructureHybridWithCoverage(ctx context.Context, clusterName, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "🚀 Gateway E2E Infrastructure (HYBRID PARALLEL + COVERAGE)")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "  Strategy: Build parallel → Create cluster → Load → Deploy")
	_, _ = fmt.Fprintln(writer, "  Benefits: Fast builds + No cluster timeout + Reliable")
	_, _ = fmt.Fprintln(writer, "  Per DD-TEST-007: Coverage instrumentation enabled")
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
	// PHASE 0: Generate dynamic image tags (BEFORE building)
	// ═══════════════════════════════════════════════════════════════════════
	// Generate DataStorage image tag ONCE (non-idempotent, timestamp-based)
	// This ensures each service builds its OWN DataStorage with LATEST code
	// Per DD-TEST-001: Dynamic tags for parallel E2E isolation
	dataStorageImageName := GenerateInfraImageName("datastorage", "gateway")
	_, _ = fmt.Fprintf(writer, "📛 DataStorage dynamic tag: %s\n", dataStorageImageName)
	_, _ = fmt.Fprintln(writer, "   (Ensures fresh build with latest DataStorage code)")

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 1: Build images IN PARALLEL (before cluster creation)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 1: Building images in parallel...")
	_, _ = fmt.Fprintln(writer, "  ├── Gateway image (WITH COVERAGE)")
	_, _ = fmt.Fprintln(writer, "  └── DataStorage image (WITH DYNAMIC TAG)")
	_, _ = fmt.Fprintln(writer, "  ⏱️  Expected: ~2-3 minutes (parallel)")

	type buildResult struct {
		name string
		err  error
	}

	buildResults := make(chan buildResult, 2)

	// Build Gateway with coverage in parallel
	go func() {
		err := BuildGatewayImageWithCoverage(writer)
		buildResults <- buildResult{name: "Gateway (coverage)", err: err}
	}()

	// Build DataStorage with dynamic tag in parallel
	// NOTE: Cannot use BuildAndLoadImageToKind() here because this function
	// uses build-before-cluster optimization pattern (Phase 3 analysis)
	// Authority: docs/handoff/TEST_INFRASTRUCTURE_PHASE3_PLAN_JAN07.md
	go func() {
		err := buildDataStorageImageWithTag(dataStorageImageName, writer)
		buildResults <- buildResult{name: "DataStorage", err: err}
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
			_, _ = fmt.Fprintf(writer, "  ✅ %s build completed\n", result.name)
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
	_, _ = fmt.Fprintln(writer, "  ⏱️  Expected: ~10-15 seconds")

	if err := createGatewayKindCluster(clusterName, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create Kind cluster: %w", err)
	}

	// Install RemediationRequest CRD
	_, _ = fmt.Fprintln(writer, "📋 Installing RemediationRequest CRD...")
	crdPath := getProjectRoot() + "/config/crd/bases/kubernaut.ai_remediationrequests.yaml"
	crdCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", crdPath)
	crdCmd.Stdout = writer
	crdCmd.Stderr = writer
	if err := crdCmd.Run(); err != nil {
		return fmt.Errorf("failed to install RemediationRequest CRD: %w", err)
	}

	// Create namespace
	_, _ = fmt.Fprintf(writer, "📁 Creating namespace %s...\n", namespace)
	if err := createTestNamespace(namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "\n✅ Kind cluster ready!")

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 3: Load images into fresh cluster (parallel)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 3: Loading images into Kind cluster...")
	_, _ = fmt.Fprintln(writer, "  ├── Gateway coverage image")
	_, _ = fmt.Fprintln(writer, "  └── DataStorage image (with dynamic tag)")
	_, _ = fmt.Fprintln(writer, "  ⏱️  Expected: ~30-45 seconds")

	loadResults := make(chan buildResult, 2)

	// Load Gateway coverage image
	go func() {
		err := LoadGatewayCoverageImage(clusterName, writer)
		loadResults <- buildResult{name: "Gateway coverage", err: err}
	}()

	// Load DataStorage image with dynamic tag
	// NOTE: Cannot use BuildAndLoadImageToKind() here because this function
	// uses build-before-cluster optimization pattern (Phase 3 analysis)
	// Authority: docs/handoff/TEST_INFRASTRUCTURE_PHASE3_PLAN_JAN07.md
	go func() {
		err := loadDataStorageImageWithTag(clusterName, dataStorageImageName, writer)
		loadResults <- buildResult{name: "DataStorage", err: err}
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

	_, _ = fmt.Fprintln(writer, "\n✅ All images loaded into cluster!")

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 4: Deploy services in PARALLEL (DD-TEST-002 MANDATE)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 4: Deploying services in parallel...")
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
		// CRITICAL: Use the tag generated in Phase 0 (UUID-based, non-idempotent)
		// Per DD-TEST-001: Dynamic tags for parallel E2E isolation
		// This ensures we deploy the SAME fresh-built image with latest DataStorage code
		err := deployDataStorageServiceInNamespace(ctx, namespace, kubeconfigPath, dataStorageImageName, writer)
		deployResults <- deployResult{"DataStorage", err}
	}()
	go func() {
		err := DeployGatewayCoverageManifest(kubeconfigPath, writer)
		deployResults <- deployResult{"Gateway", err}
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
	if err := waitForGatewayServicesReady(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("services not ready: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "✅ Gateway E2E Infrastructure Ready!")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "  🚀 Strategy: Hybrid parallel (build parallel → cluster → load)")
	_, _ = fmt.Fprintln(writer, "  📊 Coverage: Enabled (GOCOVERDIR=/coverdata)")
	_, _ = fmt.Fprintln(writer, "  🎯 Gateway URL: http://localhost:8080")
	_, _ = fmt.Fprintln(writer, "  📦 Namespace: kubernaut-system")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	return nil
}

// waitForGatewayServicesReady waits for DataStorage and Gateway pods to be ready
// Per DD-TEST-002: Single readiness check after parallel deployment
func waitForGatewayServicesReady(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
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

	// Wait for Gateway pod to be ready (coverage-enabled may take longer)
	_, _ = fmt.Fprintf(writer, "   ⏳ Waiting for Gateway pod to be ready...\n")
	Eventually(func() bool {
		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app=gateway",
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
	}, 3*time.Minute, 5*time.Second).Should(BeTrue(), "Gateway pod should become ready")
	_, _ = fmt.Fprintf(writer, "   ✅ Gateway ready\n")

	return nil
}

package infrastructure

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
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
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(writer, "🚀 SignalProcessing E2E Infrastructure (HYBRID PARALLEL + COVERAGE)")
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(writer, "  Strategy: Build parallel → Create cluster → Load → Deploy")
	fmt.Fprintln(writer, "  Benefits: Fast builds + No cluster timeout + Reliable")
	fmt.Fprintln(writer, "  Per DD-TEST-007: Coverage instrumentation enabled")
	fmt.Fprintln(writer, "  Per DD-TEST-001: Port 30082 (API), 30182 (Metrics)")
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	namespace := "kubernaut-system"

	// DD-TEST-007: Create coverdata directory BEFORE everything
	projectRoot := getProjectRoot()
	coverdataPath := filepath.Join(projectRoot, "coverdata")
	fmt.Fprintf(writer, "📁 Creating coverage directory: %s\n", coverdataPath)
	if err := os.MkdirAll(coverdataPath, 0777); err != nil {
		return fmt.Errorf("failed to create coverdata directory: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 0: Generate dynamic image tags (BEFORE building)
	// ═══════════════════════════════════════════════════════════════════════
	// Generate DataStorage image tag ONCE (non-idempotent, timestamp-based)
	// This ensures each service builds its OWN DataStorage with LATEST code
	// Per DD-TEST-001: Dynamic tags for parallel E2E isolation
	dataStorageImageName := GenerateInfraImageName("datastorage", "signalprocessing")
	fmt.Fprintf(writer, "📛 DataStorage dynamic tag: %s\n", dataStorageImageName)
	fmt.Fprintln(writer, "   (Ensures fresh build with latest DataStorage code)")

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 1: Build images IN PARALLEL (before cluster creation)
	// ═══════════════════════════════════════════════════════════════════════
	phase1Start := time.Now()
	fmt.Fprintln(writer, "\n📦 PHASE 1: Building images in parallel...")
	fmt.Fprintf(writer, "  ⏱️  Start: %s\n", phase1Start.Format("15:04:05.000"))
	fmt.Fprintln(writer, "  ├── SignalProcessing controller (WITH COVERAGE)")
	fmt.Fprintln(writer, "  └── DataStorage image (WITH DYNAMIC TAG)")
	fmt.Fprintln(writer, "  ⏱️  Expected: ~2-3 minutes (parallel)")

	type buildResult struct {
		name string
		err  error
	}

	buildResults := make(chan buildResult, 2)

	// Build SignalProcessing with coverage in parallel
	go func() {
		err := BuildSignalProcessingImageWithCoverage(writer)
		buildResults <- buildResult{name: "SignalProcessing (coverage)", err: err}
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

	phase1End := time.Now()
	phase1Duration := phase1End.Sub(phase1Start)
	fmt.Fprintln(writer, "\n✅ All images built successfully!")
	fmt.Fprintf(writer, "  ⏱️  Phase 1 Duration: %.1f seconds\n", phase1Duration.Seconds())

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 2: Create Kind cluster (now that images are ready)
	// ═══════════════════════════════════════════════════════════════════════
	phase2Start := time.Now()
	fmt.Fprintln(writer, "\n📦 PHASE 2: Creating Kind cluster...")
	fmt.Fprintf(writer, "  ⏱️  Start: %s\n", phase2Start.Format("15:04:05.000"))
	fmt.Fprintln(writer, "  ⏱️  Expected: ~10-15 seconds")

	if err := createSignalProcessingKindCluster(clusterName, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create Kind cluster: %w", err)
	}

	// OPTIMIZATION #2: Install both CRDs in a single kubectl apply (3-5s savings)
	fmt.Fprintln(writer, "📋 Installing CRDs (batched: SignalProcessing + RemediationRequest)...")
	if err := installSignalProcessingCRDsBatched(kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to install CRDs (batched): %w", err)
	}

	// Create kubernaut-system namespace
	fmt.Fprintf(writer, "📁 Creating namespace %s...\n", namespace)
	if err := createSignalProcessingNamespace(kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	// Deploy Rego policy ConfigMaps
	fmt.Fprintln(writer, "📜 Deploying Rego policy ConfigMaps...")
	if err := deploySignalProcessingPolicies(kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy policies: %w", err)
	}

	phase2End := time.Now()
	phase2Duration := phase2End.Sub(phase2Start)
	fmt.Fprintln(writer, "\n✅ Kind cluster ready!")
	fmt.Fprintf(writer, "  ⏱️  Phase 2 Duration: %.1f seconds\n", phase2Duration.Seconds())

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 3: Load images into fresh cluster (parallel)
	// ═══════════════════════════════════════════════════════════════════════
	phase3Start := time.Now()
	fmt.Fprintln(writer, "\n📦 PHASE 3: Loading images into Kind cluster...")
	fmt.Fprintf(writer, "  ⏱️  Start: %s\n", phase3Start.Format("15:04:05.000"))
	fmt.Fprintln(writer, "  ├── SignalProcessing coverage image")
	fmt.Fprintln(writer, "  └── DataStorage image (with dynamic tag)")
	fmt.Fprintln(writer, "  ⏱️  Expected: ~30-45 seconds")

	loadResults := make(chan buildResult, 2)

	// Load SignalProcessing coverage image
	go func() {
		err := LoadSignalProcessingCoverageImage(clusterName, writer)
		loadResults <- buildResult{name: "SignalProcessing coverage", err: err}
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

	phase3End := time.Now()
	phase3Duration := phase3End.Sub(phase3Start)
	fmt.Fprintln(writer, "\n✅ All images loaded into cluster!")
	fmt.Fprintf(writer, "  ⏱️  Phase 3 Duration: %.1f seconds\n", phase3Duration.Seconds())

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 4: Deploy services in PARALLEL (DD-TEST-002 MANDATE)
	// ═══════════════════════════════════════════════════════════════════════
	phase4Start := time.Now()
	fmt.Fprintln(writer, "\n📦 PHASE 4: Deploying services in parallel...")
	fmt.Fprintf(writer, "  ⏱️  Start: %s\n", phase4Start.Format("15:04:05.000"))
	fmt.Fprintln(writer, "  (Kubernetes will handle dependencies and reconciliation)")

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
		err := DeploySignalProcessingControllerWithCoverage(kubeconfigPath, writer)
		deployResults <- deployResult{"SignalProcessing", err}
	}()

	// Collect ALL results before proceeding (MANDATORY)
	var deployErrors []error
	for i := 0; i < 5; i++ {
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
	if err := waitForSPServicesReady(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("services not ready: %w", err)
	}

	phase4End := time.Now()
	phase4Duration := phase4End.Sub(phase4Start)
	totalDuration := phase4End.Sub(phase1Start)
	fmt.Fprintln(writer, "\n✅ All services ready!")
	fmt.Fprintf(writer, "  ⏱️  Phase 4 Duration: %.1f seconds\n", phase4Duration.Seconds())

	fmt.Fprintln(writer, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(writer, "✅ SignalProcessing E2E Infrastructure Ready!")
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(writer, "  🚀 Strategy: Hybrid parallel (build parallel → cluster → load)")
	fmt.Fprintln(writer, "  📊 Coverage: Enabled (GOCOVERDIR=/coverdata)")
	fmt.Fprintln(writer, "  🎯 SP API: http://localhost:30082")
	fmt.Fprintln(writer, "  📊 SP Metrics: http://localhost:30182")
	fmt.Fprintln(writer, "  📦 Namespace: kubernaut-system")
	fmt.Fprintln(writer, "")
	fmt.Fprintln(writer, "⏱️  PROFILING SUMMARY (per SP_E2E_OPTIMIZATION_TRIAGE_DEC_25_2025.md):")
	fmt.Fprintf(writer, "  Phase 1 (Build Images):     %.1fs\n", phase1Duration.Seconds())
	fmt.Fprintf(writer, "  Phase 2 (Create Cluster):   %.1fs\n", phase2Duration.Seconds())
	fmt.Fprintf(writer, "  Phase 3 (Load Images):      %.1fs\n", phase3Duration.Seconds())
	fmt.Fprintf(writer, "  Phase 4 (Deploy Services):  %.1fs\n", phase4Duration.Seconds())
	fmt.Fprintf(writer, "  TOTAL SETUP TIME:           %.1fs (%.1f min)\n", totalDuration.Seconds(), totalDuration.Minutes())
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

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

	// Wait for SignalProcessing pod to be ready (coverage-enabled may take longer)
	fmt.Fprintf(writer, "   ⏳ Waiting for SignalProcessing pod to be ready...\n")
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
	fmt.Fprintf(writer, "   ✅ SignalProcessing ready\n")

	return nil
}


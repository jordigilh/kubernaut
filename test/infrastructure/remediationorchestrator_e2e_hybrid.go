package infrastructure

import (
	"bytes"
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

// SetupROInfrastructureHybridWithCoverage implements hybrid parallel strategy
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
func SetupROInfrastructureHybridWithCoverage(ctx context.Context, clusterName, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "🚀 RemediationOrchestrator E2E Infrastructure (HYBRID PARALLEL + COVERAGE)")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "  Strategy: Build parallel → Create cluster → Load → Deploy")
	_, _ = fmt.Fprintln(writer, "  Benefits: Fast builds + No cluster timeout + Reliable")
	_, _ = fmt.Fprintln(writer, "  Per DD-TEST-007: Coverage instrumentation enabled")
	_, _ = fmt.Fprintln(writer, "  Per DD-TEST-001: Port 30083 (API), 30183 (Metrics)")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	namespace := "kubernaut-system"

	// DD-TEST-007: Create coverdata directory BEFORE everything
	projectRoot := getProjectRoot()
	coverdataPath := filepath.Join(projectRoot, "coverdata")
	_, _ = fmt.Fprintf(writer, "📁 Creating coverage directory: %s\n", coverdataPath)
	if err := os.MkdirAll(coverdataPath, 0777); err != nil {
		return fmt.Errorf("failed to create coverdata directory: %w", err)
	}

	// PHASE 0 is now integrated into consolidated API (BuildImageForKind generates dynamic tags automatically)
	// Per DD-TEST-001: Dynamic tags for parallel E2E isolation

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 1: Build images IN PARALLEL (before cluster creation)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 1: Building images in parallel...")
	_, _ = fmt.Fprintln(writer, "  ├── RemediationOrchestrator controller (WITH COVERAGE)")
	_, _ = fmt.Fprintln(writer, "  ├── DataStorage image (WITH DYNAMIC TAG)")
	_, _ = fmt.Fprintln(writer, "  └── AuthWebhook (FOR SOC2 CC8.1)")
	_, _ = fmt.Fprintln(writer, "  ⏱️  Expected: ~2 minutes (parallel)")
	_, _ = fmt.Fprintln(writer, "  Using consolidated API: BuildImageForKind()")
	_, _ = fmt.Fprintln(writer, "  Note: Notification controller NOT needed - only CRD validation required")

	type imageBuildResult struct {
		name  string
		image string
		err   error
	}

	buildResults := make(chan imageBuildResult, 3)

	// Build RemediationOrchestrator with coverage in parallel using consolidated API
	go func() {
		cfg := E2EImageConfig{
			ServiceName:      "remediationorchestrator", // Operator SDK convention: no -controller suffix in image name
			ImageName:        "kubernaut/remediationorchestrator",
			DockerfilePath:   "docker/remediationorchestrator-controller.Dockerfile", // Dockerfile can have suffix
			BuildContextPath: "",                                                     // Will use project root
			EnableCoverage:   os.Getenv("E2E_COVERAGE") == "true" || os.Getenv("GOCOVERDIR") != "",
		}
		roImage, err := BuildImageForKind(cfg, writer)
		buildResults <- imageBuildResult{name: "RemediationOrchestrator (coverage)", image: roImage, err: err}
	}()

	// Build DataStorage in parallel using consolidated API
	go func() {
		cfg := E2EImageConfig{
			ServiceName:      "datastorage",
			ImageName:        "kubernaut/datastorage",
			DockerfilePath:   "docker/data-storage.Dockerfile",
			BuildContextPath: "", // Will use project root
			EnableCoverage:   os.Getenv("E2E_COVERAGE") == "true",
		}
		dsImage, err := BuildImageForKind(cfg, writer)
		buildResults <- imageBuildResult{name: "DataStorage", image: dsImage, err: err}
	}()

	// Build AuthWebhook in parallel using consolidated API
	go func() {
		cfg := E2EImageConfig{
			ServiceName:      "authwebhook",
			ImageName:        "authwebhook",
			DockerfilePath:   "docker/authwebhook.Dockerfile",
			BuildContextPath: "", // Will use project root
			EnableCoverage:   os.Getenv("E2E_COVERAGE") == "true",
		}
		awImage, err := BuildImageForKind(cfg, writer)
		buildResults <- imageBuildResult{name: "AuthWebhook", image: awImage, err: err}
	}()

	// Wait for all three builds to complete and collect image names
	_, _ = fmt.Fprintln(writer, "\n⏳ Waiting for all builds to complete...")
	builtImages := make(map[string]string) // name -> full image name
	var buildErrors []error
	for i := 0; i < 3; i++ {
		result := <-buildResults
		if result.err != nil {
			_, _ = fmt.Fprintf(writer, "  ❌ %s build failed: %v\n", result.name, result.err)
			buildErrors = append(buildErrors, result.err)
		} else {
			_, _ = fmt.Fprintf(writer, "  ✅ %s build completed: %s\n", result.name, result.image)
			builtImages[result.name] = result.image
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

	// Create Kind cluster with coverage extraMount
	// Use absolute paths to ensure correct mount resolution regardless of working directory
	extraMounts := []ExtraMount{
		// Coverage data collection (DD-TEST-007) - use absolute path from earlier calculation
		{
			HostPath:      coverdataPath, // Already absolute: /workspace/coverdata
			ContainerPath: "/coverdata",
			ReadOnly:      false,
		},
	}

	kindConfigPath := "test/infrastructure/kind-remediationorchestrator-config.yaml"
	if err := CreateKindClusterWithExtraMounts(
		clusterName,
		kubeconfigPath,
		kindConfigPath,
		extraMounts,
		writer,
	); err != nil {
		return fmt.Errorf("failed to create Kind cluster: %w", err)
	}

	// Install ALL CRDs required for RO orchestration
	_, _ = fmt.Fprintln(writer, "📋 Installing CRDs...")
	crdFiles := []string{
		"kubernaut.ai_remediationrequests.yaml",
		"kubernaut.ai_remediationapprovalrequests.yaml", // Required for RO approval workflow
		"kubernaut.ai_aianalyses.yaml",
		"kubernaut.ai_workflowexecutions.yaml",
		"kubernaut.ai_signalprocessings.yaml",
		"kubernaut.ai_notificationrequests.yaml",
		"kubernaut.ai_effectivenessassessments.yaml", // ADR-EM-001: EA CRD for EA creation on terminal phases
	}

	for _, crdFile := range crdFiles {
		crdPath := filepath.Join(projectRoot, "config/crd/bases", crdFile)
		_, _ = fmt.Fprintf(writer, "  ├── Installing %s...\n", crdFile)
		crdCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", crdPath)
		crdCmd.Stdout = writer
		crdCmd.Stderr = writer
		if err := crdCmd.Run(); err != nil {
			return fmt.Errorf("failed to install %s: %w", crdFile, err)
		}
	}

	// Create kubernaut-system namespace
	_, _ = fmt.Fprintf(writer, "📁 Creating namespace %s...\n", namespace)
	if err := createTestNamespace(namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "\n✅ Kind cluster ready!")

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 3: Load images into fresh cluster (parallel)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 3: Loading images into Kind cluster...")
	_, _ = fmt.Fprintln(writer, "  ├── RemediationOrchestrator coverage image")
	_, _ = fmt.Fprintln(writer, "  ├── DataStorage image (with dynamic tag)")
	_, _ = fmt.Fprintln(writer, "  └── AuthWebhook image (SOC2 CC8.1 user attribution)")
	_, _ = fmt.Fprintln(writer, "  ⏱️  Expected: ~30-45 seconds")
	_, _ = fmt.Fprintln(writer, "  Using consolidated API: LoadImageToKind()")
	_, _ = fmt.Fprintln(writer, "  Note: OAuth2-Proxy pulled automatically from quay.io during deployment")

	type loadResult struct {
		name string
		err  error
	}
	loadResults := make(chan loadResult, 3)

	// Load RemediationOrchestrator coverage image using consolidated API
	go func() {
		roImage := builtImages["RemediationOrchestrator (coverage)"]
		err := LoadImageToKind(roImage, "remediationorchestrator-controller", clusterName, writer)
		loadResults <- loadResult{name: "RemediationOrchestrator coverage", err: err}
	}()

	// Load DataStorage image using consolidated API
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

	// Wait for all three loads to complete
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

	// Step 1: Create RoleBinding for DataStorage ServiceAccount (DD-AUTH-014)
	_, _ = fmt.Fprintf(writer, "🔐 Creating RoleBinding for DataStorage ServiceAccount (DD-AUTH-014)...\n")
	if err := CreateDataStorageAccessRoleBinding(ctx, namespace, kubeconfigPath, "data-storage-service", writer); err != nil {
		return fmt.Errorf("failed to create DataStorage ServiceAccount RoleBinding: %w", err)
	}

	// Step 2: Create RoleBinding for RemediationOrchestrator controller (DD-AUTH-014)
	// CRITICAL: RO controller needs this to emit audit events to DataStorage
	// Authority: DD-AUTH-014 (Middleware-based SAR Authentication)
	_, _ = fmt.Fprintf(writer, "🔐 Creating RoleBinding for RemediationOrchestrator controller (DD-AUTH-014)...\n")
	if err := CreateDataStorageAccessRoleBinding(ctx, namespace, kubeconfigPath, "remediationorchestrator-controller", writer); err != nil {
		return fmt.Errorf("failed to create RemediationOrchestrator controller RoleBinding: %w", err)
	}

	// Step 3: Create RoleBinding for AuthWebhook (DD-AUTH-014)
	// CRITICAL: AuthWebhook needs this to emit audit events to DataStorage (Gap #8)
	// Per RCA (Jan 30, 2026): AuthWebhook was missing DataStorage RBAC, causing Gap #8 failure
	_, _ = fmt.Fprintf(writer, "🔐 Creating RoleBinding for AuthWebhook (DD-AUTH-014)...\n")
	if err := CreateDataStorageAccessRoleBinding(ctx, namespace, kubeconfigPath, "authwebhook", writer); err != nil {
		return fmt.Errorf("failed to create AuthWebhook RoleBinding: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 4: Deploy services in PARALLEL (DD-TEST-002 MANDATE)
	// ═══════════════════════════════════════════════════════════════════════
	// NOTE: Using dynamically generated image names from consolidated API (BuildImageForKind)
	// This ensures we deploy the SAME images we just built with latest code
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
		// DD-AUTH-014: Deploy client ClusterRole FIRST (required for SAR checks)
		// This enables all services to pass SAR checks when calling DataStorage
		_, _ = fmt.Fprintf(writer, "🔐 Deploying data-storage-client ClusterRole (DD-AUTH-014)...\n")
		if clientRBACErr := deployDataStorageClientClusterRole(ctx, kubeconfigPath, writer); clientRBACErr != nil {
			deployResults <- deployResult{"DataStorage", fmt.Errorf("failed to deploy client ClusterRole: %w", clientRBACErr)}
			return
		}

		// Use the dynamically generated image from build phase
		// Per DD-TEST-001: Dynamic tags for parallel E2E isolation
		dsImage := builtImages["DataStorage"]

		// DD-AUTH-014: Deploy ServiceAccount and RBAC (required for pod creation)
		_, _ = fmt.Fprintf(writer, "🔐 Deploying DataStorage service RBAC for auth middleware (DD-AUTH-014)...\n")
		if rbacErr := deployDataStorageServiceRBAC(ctx, namespace, kubeconfigPath, writer); rbacErr != nil {
			deployResults <- deployResult{"DataStorage", fmt.Errorf("failed to deploy service RBAC: %w", rbacErr)}
			return
		}

		// DD-AUTH-014: Deploy DataStorage with middleware-based auth
		err := deployDataStorageServiceInNamespace(ctx, namespace, kubeconfigPath, dsImage, writer)
		deployResults <- deployResult{"DataStorage", err}
	}()
	go func() {
		roImage := builtImages["RemediationOrchestrator (coverage)"]
		err := DeployROCoverageManifest(kubeconfigPath, roImage, writer)
		deployResults <- deployResult{"RemediationOrchestrator", err}
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
	if err := waitForROServicesReady(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("services not ready: %w", err)
	}

	// PHASE 4.5: Deploy AuthWebhook manifests (using pre-built + pre-loaded image)
	// Per DD-WEBHOOK-001: Required for RemediationApprovalRequest approval decisions
	// Per SOC2 CC8.1: Captures WHO approved/rejected remediation requests
	_, _ = fmt.Fprintln(writer, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "🔐 PHASE 4.5: Deploying AuthWebhook Manifests")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	awImage := builtImages["AuthWebhook"]
	if err := DeployAuthWebhookManifestsOnly(ctx, clusterName, namespace, kubeconfigPath, awImage, writer); err != nil {
		return fmt.Errorf("failed to deploy AuthWebhook manifests: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "✅ AuthWebhook deployed - SOC2 CC8.1 user attribution enabled")

	_, _ = fmt.Fprintln(writer, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "✅ RemediationOrchestrator E2E Infrastructure Ready!")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "  🚀 Strategy: Hybrid parallel (build parallel → cluster → load)")
	_, _ = fmt.Fprintln(writer, "  📊 Coverage: Enabled (GOCOVERDIR=/coverdata)")
	_, _ = fmt.Fprintln(writer, "  🎯 RO Metrics: http://localhost:30183")
	_, _ = fmt.Fprintln(writer, "  📦 Namespace: kubernaut-system")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	return nil
}

// ============================================================================
// Build Functions (with coverage support per DD-TEST-007)
// ============================================================================

// BuildROImageWithCoverage builds the RemediationOrchestrator controller image with coverage instrumentation
// Per DD-TEST-007: E2E Coverage Capture Standard
func BuildROImageWithCoverage(writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "🏗️  Building RemediationOrchestrator controller image (WITH COVERAGE)...")

	projectRoot := getProjectRoot()
	dockerfilePath := filepath.Join(projectRoot, "docker", "remediationorchestrator-controller.Dockerfile")

	// Build with coverage instrumentation
	// Use localhost/ prefix to match Podman's default tagging
	// CRITICAL: --no-cache ensures latest code changes are included (DD-TEST-002)
	cmd := exec.Command("podman", "build",
		"--no-cache", // Force fresh build to include latest code changes
		"--build-arg", "GOFLAGS=-cover",
		"-t", "localhost/remediationorchestrator-controller:e2e-coverage",
		"-f", dockerfilePath,
		projectRoot,
	)
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to build RemediationOrchestrator image with coverage: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "  ✅ RemediationOrchestrator controller image built (WITH COVERAGE)")
	return nil
}

// tagDataStorageImageInKind is now OBSOLETE - removed per Phase 0 tag generation pattern
// Each service builds DataStorage with dynamic tag directly (no re-tagging needed)
// Shared helper functions buildDataStorageImageWithTag() and loadDataStorageImageWithTag()
// are defined in datastorage.go

// LoadROCoverageImage loads the RemediationOrchestrator coverage-instrumented image into Kind cluster
// Uses podman save + kind load image-archive pattern to work around Kind+Podman localhost/ prefix issue
func LoadROCoverageImage(clusterName string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "📦 Loading RemediationOrchestrator coverage image into Kind cluster...")

	// Save image to tar (following Gateway/DataStorage pattern for Kind+Podman compatibility)
	saveCmd := exec.Command("podman", "save", "localhost/remediationorchestrator-controller:e2e-coverage", "-o", "/tmp/remediationorchestrator-e2e-coverage.tar")
	saveCmd.Stdout = writer
	saveCmd.Stderr = writer

	if err := saveCmd.Run(); err != nil {
		return fmt.Errorf("failed to save image: %w", err)
	}

	// Load tar into Kind cluster
	loadCmd := exec.Command("kind", "load", "image-archive", "/tmp/remediationorchestrator-e2e-coverage.tar", "--name", clusterName)
	loadCmd.Stdout = writer
	loadCmd.Stderr = writer

	if err := loadCmd.Run(); err != nil {
		return fmt.Errorf("failed to load image archive into Kind: %w", err)
	}

	// Clean up tar file
	_ = os.Remove("/tmp/remediationorchestrator-e2e-coverage.tar")

	// CRITICAL: Remove Podman image immediately to free disk space
	// Image is now in Kind, Podman copy is duplicate
	_, _ = fmt.Fprintln(writer, "  🗑️  Removing Podman image to free disk space...")
	rmiCmd := exec.Command("podman", "rmi", "-f", "localhost/remediationorchestrator-controller:e2e-coverage")
	rmiCmd.Stdout = writer
	rmiCmd.Stderr = writer
	if err := rmiCmd.Run(); err != nil {
		_, _ = fmt.Fprintf(writer, "  ⚠️  Failed to remove Podman image (non-fatal): %v\n", err)
	} else {
		_, _ = fmt.Fprintln(writer, "  ✅ Podman image removed: localhost/remediationorchestrator-controller:e2e-coverage")
	}

	_, _ = fmt.Fprintln(writer, "  ✅ RemediationOrchestrator coverage image loaded")
	return nil
}

// ============================================================================
// Deploy Functions
// ============================================================================

// DeployROCoverageManifest deploys the RemediationOrchestrator controller with coverage enabled
// Per DD-TEST-007: Mounts coverdata directory as hostPath volume
// Per ADR-030: Mounts audit config file for E2E audit testing
// Per consolidated API migration: Accepts dynamic image name as parameter
func DeployROCoverageManifest(kubeconfigPath, imageName string, writer io.Writer) error {
	// Create manifest with coverage volume mount + audit config
	manifest := fmt.Sprintf(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: remediationorchestrator-config
  namespace: kubernaut-system
data:
  remediationorchestrator.yaml: |
    # RemediationOrchestrator E2E Configuration
    # Per ADR-030: YAML-based service configuration
    # Per CRD_FIELD_NAMING_CONVENTION.md: camelCase for YAML fields
    audit:
      dataStorageUrl: http://data-storage-service:8080  # DD-AUTH-011: Match Service name
      timeout: 10s
      buffer:
        bufferSize: 10000
        batchSize: 50       # E2E: Standard pattern (same as HAPI, AIAnalysis)
        flushInterval: 100ms  # E2E: Fast flush for test visibility (0.1s)
        maxRetries: 3
    controller:
      metricsAddr: :9093
      healthProbeAddr: :8084
      leaderElection: false
      leaderElectionId: remediationorchestrator.kubernaut.ai
    effectivenessAssessment:
      stabilizationWindow: 10s  # E2E: Allow OOMKill-restarted pods to recover before EM assesses health
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: remediationorchestrator-controller
  namespace: kubernaut-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: remediationorchestrator-controller
  template:
    metadata:
      labels:
        app: remediationorchestrator-controller
    spec:
      serviceAccountName: remediationorchestrator-controller
      containers:
      - name: controller
        image: %s
        imagePullPolicy: %s
        args:
        - --config=/etc/config/remediationorchestrator.yaml
        ports:
        - containerPort: 8084
          name: health
          protocol: TCP
        - containerPort: 9093
          name: metrics
          protocol: TCP
        env:
        - name: GOCOVERDIR
          value: /coverdata
        volumeMounts:
        - name: coverdata
          mountPath: /coverdata
        - name: config
          mountPath: /etc/config
          readOnly: true
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
      volumes:
      - name: coverdata
        hostPath:
          path: /coverdata
          type: DirectoryOrCreate
      - name: config
        configMap:
          name: remediationorchestrator-config
---
apiVersion: v1
kind: Service
metadata:
  name: remediationorchestrator-controller
  namespace: kubernaut-system
spec:
  type: NodePort
  ports:
  - port: 9093
    targetPort: 9093
    nodePort: 30183
    protocol: TCP
    name: metrics
  selector:
    app: remediationorchestrator-controller
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: remediationorchestrator-controller
  namespace: kubernaut-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: remediationorchestrator-controller
rules:
- apiGroups: ["kubernaut.ai"]
  resources: ["remediationrequests", "remediationapprovalrequests"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["kubernaut.ai"]
  resources: ["remediationrequests/status", "remediationapprovalrequests/status"]
  verbs: ["get", "update", "patch"]
- apiGroups: ["kubernaut.ai"]
  resources: ["signalprocessings"]
  verbs: ["get", "list", "watch", "create", "update", "patch"]
- apiGroups: ["kubernaut.ai"]
  resources: ["signalprocessings/status"]
  verbs: ["get"]
- apiGroups: ["kubernaut.ai"]
  resources: ["aianalyses"]
  verbs: ["get", "list", "watch", "create", "update", "patch"]
- apiGroups: ["kubernaut.ai"]
  resources: ["aianalyses/status"]
  verbs: ["get"]
- apiGroups: ["kubernaut.ai"]
  resources: ["workflowexecutions"]
  verbs: ["get", "list", "watch", "create", "update", "patch"]
- apiGroups: ["kubernaut.ai"]
  resources: ["workflowexecutions/status"]
  verbs: ["get"]
- apiGroups: ["kubernaut.ai"]
  resources: ["notificationrequests"]
  verbs: ["get", "list", "watch", "create", "update", "patch"]
- apiGroups: ["kubernaut.ai"]
  resources: ["notificationrequests/status"]
  verbs: ["get"]
- apiGroups: ["kubernaut.ai"]
  resources: ["effectivenessassessments"]
  verbs: ["get", "list", "watch", "create", "update", "patch"]
- apiGroups: ["kubernaut.ai"]
  resources: ["effectivenessassessments/status"]
  verbs: ["get"]
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]
# Scope validation: metadata-only cache (ADR-053 Decision #5, BR-SCOPE-010)
# RO uses controller-runtime metadata-only informers for scope label lookups.
# Includes cluster-scoped resources (nodes, persistentvolumes) for opt-in label checks.
- apiGroups: [""]
  resources: ["pods", "nodes", "services", "configmaps", "secrets", "namespaces", "persistentvolumes"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["apps"]
  resources: ["deployments", "replicasets", "statefulsets", "daemonsets"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["batch"]
  resources: ["jobs", "cronjobs"]
  verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: remediationorchestrator-controller
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: remediationorchestrator-controller
subjects:
- kind: ServiceAccount
  name: remediationorchestrator-controller
  namespace: kubernaut-system
`, imageName, GetImagePullPolicy())

	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = bytes.NewReader([]byte(manifest))
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to deploy RemediationOrchestrator: %w", err)
	}

	// Wait for RemediationOrchestrator to be ready with retry loop (matches PostgreSQL pattern)
	// Give Kubernetes time to schedule the pod, pull the image, and start the controller
	_, _ = fmt.Fprintln(writer, "   ⏳ Waiting for RemediationOrchestrator to be ready...")
	deadline := time.Now().Add(3 * time.Minute) // Longer timeout for controller startup
	for time.Now().Before(deadline) {
		waitCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "-n", "kubernaut-system",
			"wait", "--for=condition=ready", "pod", "-l", "app=remediationorchestrator-controller",
			"--timeout=10s")
		if err := waitCmd.Run(); err == nil {
			_, _ = fmt.Fprintln(writer, "   ✅ RemediationOrchestrator ready")
			return nil
		}
		time.Sleep(5 * time.Second)
	}

	// Capture diagnostics before failing
	_, _ = fmt.Fprintln(writer, "")
	_, _ = fmt.Fprintln(writer, "   ❌ RemediationOrchestrator not ready - capturing diagnostics...")
	_, _ = fmt.Fprintln(writer, "")

	// 1. Pod status
	_, _ = fmt.Fprintln(writer, "   📋 Pod Status:")
	statusCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "-n", "kubernaut-system",
		"get", "pods", "-l", "app=remediationorchestrator-controller", "-o", "wide")
	statusCmd.Stdout = writer
	statusCmd.Stderr = writer
	_ = statusCmd.Run()
	_, _ = fmt.Fprintln(writer, "")

	// 2. Pod describe (events, image status, readiness probe)
	_, _ = fmt.Fprintln(writer, "   📋 Pod Details & Events:")
	describeCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "-n", "kubernaut-system",
		"describe", "pod", "-l", "app=remediationorchestrator-controller")
	describeCmd.Stdout = writer
	describeCmd.Stderr = writer
	_ = describeCmd.Run()
	_, _ = fmt.Fprintln(writer, "")

	// 3. Pod logs (startup errors)
	_, _ = fmt.Fprintln(writer, "   📋 Pod Logs (last 50 lines):")
	logsCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "-n", "kubernaut-system",
		"logs", "-l", "app=remediationorchestrator-controller", "--tail=50", "--all-containers")
	logsCmd.Stdout = writer
	logsCmd.Stderr = writer
	_ = logsCmd.Run()
	_, _ = fmt.Fprintln(writer, "")

	return fmt.Errorf("RemediationOrchestrator not ready within timeout")
}

// waitForROServicesReady waits for DataStorage and RemediationOrchestrator pods to be ready
// Per DD-TEST-002: Single readiness check after parallel deployment
func waitForROServicesReady(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
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

	// Wait for RemediationOrchestrator pod to be ready (coverage-enabled may take longer)
	_, _ = fmt.Fprintf(writer, "   ⏳ Waiting for RemediationOrchestrator pod to be ready...\n")
	Eventually(func() bool {
		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app=remediationorchestrator-controller",
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
	}, 3*time.Minute, 5*time.Second).Should(BeTrue(), "RemediationOrchestrator pod should become ready")
	_, _ = fmt.Fprintf(writer, "   ✅ RemediationOrchestrator ready\n")

	return nil
}

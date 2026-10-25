package infrastructure

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

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

	namespace := kubernautSystem

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
	builtImages, err := buildROImages(ctx, writer)
	if err != nil {
		return err
	}

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
		ctx,
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
		// Issue #1661 (DD-WORKFLOW-018): DataStorage's workflow cache indexes
		// RemediationWorkflow by .spec.actionType at startup -- the CRDs must
		// already be registered with the apiserver or DS's informer cache
		// setup fails hard ("no matches for kind"), crash-looping the pod.
		"kubernaut.ai_remediationworkflows.yaml",
		"kubernaut.ai_actiontypes.yaml",
	}

	for _, crdFile := range crdFiles {
		crdPath := filepath.Join(projectRoot, "config/crd/bases", crdFile)
		_, _ = fmt.Fprintf(writer, "  ├── Installing %s...\n", crdFile)
		crdCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", crdPath)
		crdCmd.Stdout = writer
		crdCmd.Stderr = writer
		if err := crdCmd.Run(); err != nil {
			return fmt.Errorf("failed to install %s: %w", crdFile, err)
		}
	}

	// Create kubernaut-system namespace
	_, _ = fmt.Fprintf(writer, "📁 Creating namespace %s...\n", namespace)
	if err := CreateTestNamespace(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "\n✅ Kind cluster ready!")

	if err := loadROImages(ctx, clusterName, builtImages, writer); err != nil {
		return err
	}

	if err := deployROServices(ctx, namespace, kubeconfigPath, builtImages, writer); err != nil {
		return err
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
	cmd := exec.CommandContext(context.Background(), "podman", "build",
		"--no-cache", // Force fresh build to include latest code changes
		"--build-arg", fmt.Sprintf("GOARCH=%s", runtime.GOARCH),
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
	saveCmd := exec.CommandContext(context.Background(), "podman", "save", "localhost/remediationorchestrator-controller:e2e-coverage", "-o", "/tmp/remediationorchestrator-e2e-coverage.tar")
	saveCmd.Stdout = writer
	saveCmd.Stderr = writer

	if err := saveCmd.Run(); err != nil {
		return fmt.Errorf("failed to save image: %w", err)
	}

	// Load tar into Kind cluster
	loadCmd := exec.CommandContext(context.Background(), "kind", "load", "image-archive", "/tmp/remediationorchestrator-e2e-coverage.tar", "--name", clusterName)
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
	rmiCmd := exec.CommandContext(context.Background(), "podman", "rmi", "-f", "localhost/remediationorchestrator-controller:e2e-coverage")
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

// roRBACManifest returns ServiceAccount + ClusterRole + ClusterRoleBinding YAML.
// Applied first to avoid Kubernetes API propagation race where SA may not be visible
// when the pod is created.
func roRBACManifest() string {
	return `---
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
- apiGroups: ["cert-manager.io"]
  resources: ["certificates"]
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
`
}

// roWorkloadManifest returns ConfigMap + Deployment + Service YAML.
// Applied after RBAC with 2s propagation delay.
// verifyingTimeout optionally sets the Verifying phase safety-net timeout in the ConfigMap.
// When omitted, the RO binary defaults to 30m (#280). Pass "5s" for test tiers without EM.
func roWorkloadManifest(imageName string, verifyingTimeout ...string) string {
	verifyingLine := ""
	if len(verifyingTimeout) > 0 && verifyingTimeout[0] != "" {
		verifyingLine = fmt.Sprintf("\n      verifying: \"%s\"", verifyingTimeout[0])
	}
	return fmt.Sprintf(`
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
    controller:
      metricsAddr: ":9090"
      healthProbeAddr: ":8081"
      leaderElection: false
      leaderElectionId: "remediationorchestrator.kubernaut.ai"
    timeouts:
      global: "1h"
      processing: "5m"
      analyzing: "10m"
      executing: "30m"
      awaitingApproval: "3s"%s
    datastorage:
      url: "https://data-storage-service:8080"
      healthUrl: "https://data-storage-service:8080/readyz"
      timeout: "10s"
      buffer:
        bufferSize: 10000
        batchSize: 50
        flushInterval: "100ms"
        maxRetries: 3
    effectivenessAssessment:
      stabilizationWindow: "10s"
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
        - containerPort: 8081
          name: health
          protocol: TCP
        - containerPort: 9090
          name: metrics
          protocol: TCP
        env:
        - name: GOCOVERDIR
          value: /coverdata
        - name: TLS_CA_FILE
          value: /etc/tls-ca/ca.crt
        volumeMounts:
        - name: coverdata
          mountPath: /coverdata
        - name: config
          mountPath: /etc/config
          readOnly: true
        - name: tls-ca
          mountPath: /etc/tls-ca
          readOnly: true
        startupProbe:
          httpGet:
            path: /healthz
            port: health
          initialDelaySeconds: 5
          periodSeconds: 5
          failureThreshold: 30
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
      - name: tls-ca
        configMap:
          name: inter-service-ca
---
apiVersion: v1
kind: Service
metadata:
  name: remediationorchestrator-controller
  namespace: kubernaut-system
spec:
  type: NodePort
  ports:
  - port: 9090
    targetPort: 9090
    nodePort: 30183
    protocol: TCP
    name: metrics
  selector:
    app: remediationorchestrator-controller
`, verifyingLine, imageName, GetImagePullPolicy())
}

// DeployROCoverageManifest deploys the RemediationOrchestrator controller with coverage enabled
// Per DD-TEST-007: Mounts coverdata directory as hostPath volume
// Per ADR-030: Mounts audit config file for E2E audit testing
// Per consolidated API migration: Accepts dynamic image name as parameter
// Applies RBAC first, then workload after 2s propagation delay to avoid API race.
// verifyingTimeout optionally sets the Verifying phase safety-net (#280). Pass "5s" for tiers without EM.
func DeployROCoverageManifest(ctx context.Context, kubeconfigPath, imageName string, writer io.Writer, verifyingTimeout ...string) error {
	// Apply RBAC first (SA + ClusterRole + ClusterRoleBinding)
	rbacManifest := roRBACManifest()
	rbacCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	rbacCmd.Stdin = bytes.NewReader([]byte(rbacManifest))
	rbacCmd.Stdout = writer
	rbacCmd.Stderr = writer
	if err := rbacCmd.Run(); err != nil {
		return fmt.Errorf("failed to apply RemediationOrchestrator RBAC: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "   Waiting 2s for RBAC API propagation...")
	time.Sleep(2 * time.Second)

	// Apply workload (ConfigMap + Deployment + Service)
	workloadManifest := roWorkloadManifest(imageName, verifyingTimeout...)
	workloadCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	workloadCmd.Stdin = bytes.NewReader([]byte(workloadManifest))
	workloadCmd.Stdout = writer
	workloadCmd.Stderr = writer
	if err := workloadCmd.Run(); err != nil {
		return fmt.Errorf("failed to deploy RemediationOrchestrator workload: %w", err)
	}

	// Wait for RemediationOrchestrator to be ready with retry loop (matches PostgreSQL pattern)
	// Give Kubernetes time to schedule the pod, pull the image, and start the controller
	_, _ = fmt.Fprintln(writer, "   ⏳ Waiting for RemediationOrchestrator to be ready...")
	deadline := time.Now().Add(3 * time.Minute) // Longer timeout for controller startup
	for time.Now().Before(deadline) {
		waitCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "-n", kubernautSystem,
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
	statusCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "-n", kubernautSystem,
		"get", "pods", "-l", "app=remediationorchestrator-controller", "-o", "wide")
	statusCmd.Stdout = writer
	statusCmd.Stderr = writer
	_ = statusCmd.Run()
	_, _ = fmt.Fprintln(writer, "")

	// 2. Pod describe (events, image status, readiness probe)
	_, _ = fmt.Fprintln(writer, "   📋 Pod Details & Events:")
	describeCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "-n", kubernautSystem,
		"describe", "pod", "-l", "app=remediationorchestrator-controller")
	describeCmd.Stdout = writer
	describeCmd.Stderr = writer
	_ = describeCmd.Run()
	_, _ = fmt.Fprintln(writer, "")

	// 3. Pod logs (startup errors)
	_, _ = fmt.Fprintln(writer, "   📋 Pod Logs (last 50 lines):")
	logsCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "-n", kubernautSystem,
		"logs", "-l", "app=remediationorchestrator-controller", "--tail=50", "--all-containers")
	logsCmd.Stdout = writer
	logsCmd.Stderr = writer
	_ = logsCmd.Run()
	_, _ = fmt.Fprintln(writer, "")

	return fmt.Errorf("RemediationOrchestrator not ready within timeout")
}

func buildROImages(ctx context.Context, writer io.Writer) (map[string]string, error) {
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 1: Building images in parallel...")
	configs := []E2EImageConfig{
		{ServiceName: "remediationorchestrator", ImageName: "kubernaut/remediationorchestrator", DockerfilePath: "docker/remediationorchestrator-controller.Dockerfile", EnableCoverage: os.Getenv("E2E_COVERAGE") == trueFixture || os.Getenv("GOCOVERDIR") != ""},
		{ServiceName: "datastorage", ImageName: "kubernaut/datastorage", DockerfilePath: "docker/data-storage.Dockerfile", EnableCoverage: os.Getenv("E2E_COVERAGE") == trueFixture},
		{ServiceName: "authwebhook", ImageName: "authwebhook", DockerfilePath: "docker/authwebhook.Dockerfile", EnableCoverage: os.Getenv("E2E_COVERAGE") == trueFixture},
	}
	results := make(chan struct {
		name  string
		image string
		err   error
	}, len(configs))
	for _, cfg := range configs {
		cfg := cfg
		go func() {
			image, err := BuildImageForKind(ctx, cfg, writer)
			results <- struct {
				name  string
				image string
				err   error
			}{cfg.ServiceName, image, err}
		}()
	}
	images := make(map[string]string, len(configs))
	var errs []error
	for range configs {
		result := <-results
		if result.err != nil {
			errs = append(errs, result.err)
			_, _ = fmt.Fprintf(writer, "  ❌ %s build failed: %v\n", result.name, result.err)
			continue
		}
		images[result.name] = result.image
		_, _ = fmt.Fprintf(writer, "  ✅ %s build completed: %s\n", result.name, result.image)
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("image builds failed: %v", errs)
	}
	_, _ = fmt.Fprintln(writer, "\n✅ All images built successfully!")
	return images, nil
}

func loadROImages(ctx context.Context, clusterName string, images map[string]string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 3: Loading images into Kind cluster...")
	results := make(chan struct {
		name string
		err  error
	}, 3)
	entries := []struct {
		name      string
		imageKey  string
		container string
	}{{"RemediationOrchestrator coverage", "RemediationOrchestrator (coverage)", "remediationorchestrator-controller"}, {"DataStorage", "DataStorage", "datastorage"}, {"AuthWebhook", "AuthWebhook", "authwebhook"}}
	for _, entry := range entries {
		entry := entry
		go func() {
			results <- struct {
				name string
				err  error
			}{entry.name, LoadImageToKind(ctx, images[entry.imageKey], entry.container, clusterName, writer)}
		}()
	}
	var errs []error
	for range entries {
		result := <-results
		if result.err != nil {
			errs = append(errs, result.err)
			_, _ = fmt.Fprintf(writer, "  ❌ %s load failed: %v\n", result.name, result.err)
			continue
		}
		_, _ = fmt.Fprintf(writer, "  ✅ %s loaded\n", result.name)
	}
	if len(errs) > 0 {
		return fmt.Errorf("image loads failed: %v", errs)
	}
	_, _ = fmt.Fprintln(writer, "\n✅ All images loaded into cluster!")
	return nil
}

func deployROServices(ctx context.Context, namespace, kubeconfigPath string, images map[string]string, writer io.Writer) error {
	if err := deployDataStorageClientClusterRole(ctx, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy client ClusterRole: %w", err)
	}
	for _, service := range []string{"data-storage-service", "remediationorchestrator-controller", "authwebhook"} {
		if err := CreateDataStorageAccessRoleBinding(ctx, namespace, kubeconfigPath, service, writer); err != nil {
			return fmt.Errorf("create %s DataStorage RoleBinding: %w", service, err)
		}
	}
	if _, err := GenerateInterServiceTLS(ctx, kubeconfigPath, namespace, writer); err != nil {
		return fmt.Errorf("failed to generate inter-service TLS: %w", err)
	}
	if err := GenerateSigningCertSecret(ctx, kubeconfigPath, namespace, writer); err != nil {
		return fmt.Errorf("failed to generate signing certificate: %w", err)
	}
	results := make(chan struct {
		name string
		err  error
	}, 5)
	go func() {
		results <- struct {
			name string
			err  error
		}{"PostgreSQL", deployPostgreSQLInNamespace(ctx, namespace, kubeconfigPath, writer)}
	}()
	go func() {
		results <- struct {
			name string
			err  error
		}{"Redis", deployRedisInNamespace(ctx, namespace, kubeconfigPath, writer)}
	}()
	go func() {
		results <- struct {
			name string
			err  error
		}{"Migrations", ApplyAllMigrations(ctx, namespace, kubeconfigPath, writer)}
	}()
	go func() {
		err := deployDataStorageServiceRBAC(ctx, namespace, kubeconfigPath, writer)
		if err == nil {
			err = deployDataStorageServiceInNamespace(ctx, namespace, kubeconfigPath, images["DataStorage"], writer)
		}
		results <- struct {
			name string
			err  error
		}{"DataStorage", err}
	}()
	go func() {
		results <- struct {
			name string
			err  error
		}{"RemediationOrchestrator", DeployROCoverageManifest(ctx, kubeconfigPath, images["RemediationOrchestrator (coverage)"], writer, "5s")}
	}()
	var errs []error
	for range 5 {
		result := <-results
		if result.err != nil {
			_, _ = fmt.Fprintf(writer, "  ❌ %s deployment failed: %v\n", result.name, result.err)
			errs = append(errs, result.err)
			continue
		}
		_, _ = fmt.Fprintf(writer, "  ✅ %s manifests applied\n", result.name)
	}
	if len(errs) > 0 {
		return fmt.Errorf("one or more service deployments failed: %v", errs)
	}
	if err := waitForROServicesReady(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("services not ready: %w", err)
	}
	if err := SeedActionTypesViaCRD(ctx, kubeconfigPath, namespace, writer); err != nil {
		return fmt.Errorf("failed to seed action types: %w", err)
	}
	return nil
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
	waitForReadyServicePod(ctx, clientset, namespace, "app=datastorage", 2*time.Minute, "DataStorage pod should become ready")
	_, _ = fmt.Fprintf(writer, "   ✅ DataStorage ready\n")

	// Wait for RemediationOrchestrator pod to be ready (coverage-enabled may take longer)
	_, _ = fmt.Fprintf(writer, "   ⏳ Waiting for RemediationOrchestrator pod to be ready...\n")
	waitForReadyServicePod(ctx, clientset, namespace, "app=remediationorchestrator-controller", 3*time.Minute, "RemediationOrchestrator pod should become ready")
	_, _ = fmt.Fprintf(writer, "   ✅ RemediationOrchestrator ready\n")

	return nil
}

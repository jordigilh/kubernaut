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
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// SetupHAPIInfrastructure sets up HolmesGPT API E2E infrastructure
// Deploys: PostgreSQL + Redis + Data Storage + HAPI to Kind cluster
// Uses sequential builds to avoid OOM with Python pip install
//
// Port Allocations (per DD-TEST-001 v2.5):
// - HAPI: NodePort 30120 → Container 8080
// - Data Storage: NodePort 30089 → Container 8080 (Host Port 8089)
// - PostgreSQL: NodePort 30439 → Container 5432
// - Redis: NodePort 30387 → Container 6379
func SetupHAPIInfrastructure(ctx context.Context, clusterName, kubeconfigPath, namespace string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "🚀 HAPI E2E Infrastructure Setup")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "  Strategy: Parallel builds → Create cluster → Deploy services")
	_, _ = fmt.Fprintln(writer, "  Duration: ~3-5 minutes (parallel builds per DD-TEST-002)")
	_, _ = fmt.Fprintln(writer, "  Per DD-TEST-001 v1.8: Dedicated HAPI ports")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	projectRoot := getProjectRoot()

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 1: Build images IN PARALLEL (per DD-TEST-002 MANDATE)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 1: Building images in parallel...")
	_, _ = fmt.Fprintln(writer, "  ├── Data Storage (1-2 min)")
	_, _ = fmt.Fprintln(writer, "  ├── HolmesGPT-API (2-3 min)")
	_, _ = fmt.Fprintln(writer, "  └── Mock LLM (<1 min)")

	type imageBuildResult struct {
		name  string
		image string
		err   error
	}

	buildResults := make(chan imageBuildResult, 3)

	// Build Data Storage in parallel
	go func() {
		cfg := E2EImageConfig{
			ServiceName:      "datastorage",
			ImageName:        "datastorage", // No kubernaut/ prefix (matches old behavior)
			DockerfilePath:   "docker/data-storage.Dockerfile",
			BuildContextPath: "",
			EnableCoverage:   false,
		}
		imageName, err := BuildImageForKind(cfg, writer)
		buildResults <- imageBuildResult{"datastorage", imageName, err}
	}()

	// Build HAPI in parallel (using E2E Dockerfile with minimal dependencies)
	go func() {
		cfg := E2EImageConfig{
			ServiceName:      "holmesgpt-api",
			ImageName:        "holmesgpt-api", // No kubernaut/ prefix (matches old behavior)
			DockerfilePath:   "holmesgpt-api/Dockerfile.e2e",
			BuildContextPath: "",
			EnableCoverage:   false, // HAPI does not support coverage
		}
		imageName, err := BuildImageForKind(cfg, writer)
		buildResults <- imageBuildResult{"holmesgpt-api", imageName, err}
	}()

	// Build Mock LLM in parallel
	go func() {
		cfg := E2EImageConfig{
			ServiceName:      "mock-llm",
			ImageName:        "mock-llm", // No kubernaut/ prefix (matches old behavior)
			DockerfilePath:   "test/services/mock-llm/Dockerfile",
			BuildContextPath: filepath.Join(projectRoot, "test/services/mock-llm"),
			EnableCoverage:   false,
		}
		imageName, err := BuildImageForKind(cfg, writer)
		buildResults <- imageBuildResult{"mock-llm", imageName, err}
	}()

	// Collect build results
	builtImages := make(map[string]string)
	for i := 0; i < 3; i++ {
		result := <-buildResults
		if result.err != nil {
			return fmt.Errorf("failed to build %s image: %w", result.name, result.err)
		}
		builtImages[result.name] = result.image
		_, _ = fmt.Fprintf(writer, "  ✅ %s image built: %s\n", result.name, result.image)
	}

	dataStorageImage := builtImages["datastorage"]
	hapiImage := builtImages["holmesgpt-api"]
	mockLLMImage := builtImages["mock-llm"]

	_, _ = fmt.Fprintln(writer, "\n✅ All images built in parallel! (~3-5 min)")

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 2: Create Kind cluster
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 2: Creating Kind cluster...")
	if err := createHAPIKindCluster(clusterName, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create Kind cluster: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 3: Load images in PARALLEL (DD-TEST-002 MANDATE)
	// ═══════════════════════════════════════════════════════════════════════
	// FIXED: Skip image loading when using registry images (IMAGE_REGISTRY + IMAGE_TAG set)
	// Pattern matches buildImageWithArgs() and BuildImageForKind() in shared_integration_utils.go
	// Local dev: Load images into Kind from local podman registry
	// CI/CD: Images already in GHCR, Kubernetes nodes pull directly via imagePullPolicy
	if os.Getenv("IMAGE_REGISTRY") != "" && os.Getenv("IMAGE_TAG") != "" {
		_, _ = fmt.Fprintln(writer, "\n📦 PHASE 3: Skipping image load (registry mode - Kubernetes pulls from registry)")
		_, _ = fmt.Fprintf(writer, "  ℹ️  IMAGE_REGISTRY=%s, IMAGE_TAG=%s\n", os.Getenv("IMAGE_REGISTRY"), os.Getenv("IMAGE_TAG"))
		_, _ = fmt.Fprintf(writer, "  📦 Images will be pulled directly by Kubernetes nodes:\n")
		_, _ = fmt.Fprintf(writer, "     - DataStorage: %s\n", dataStorageImage)
		_, _ = fmt.Fprintf(writer, "     - HolmesGPT-API: %s\n", hapiImage)
		_, _ = fmt.Fprintf(writer, "     - Mock LLM: %s\n", mockLLMImage)
	} else {
		_, _ = fmt.Fprintln(writer, "\n📦 PHASE 3: Loading images in parallel...")
		type imageLoadResult struct {
			name string
			err  error
		}
		loadResults := make(chan imageLoadResult, 3)

		go func() {
			defer GinkgoRecover()
			err := loadImageToKind(clusterName, dataStorageImage, writer)
			loadResults <- imageLoadResult{"DataStorage", err}
		}()
		go func() {
			defer GinkgoRecover()
			err := loadImageToKind(clusterName, hapiImage, writer)
			loadResults <- imageLoadResult{"HolmesGPT-API", err}
		}()
		go func() {
			defer GinkgoRecover()
			err := loadImageToKind(clusterName, mockLLMImage, writer)
			loadResults <- imageLoadResult{"Mock LLM", err}
		}()
		for i := 0; i < 3; i++ {
			result := <-loadResults
			if result.err != nil {
				return fmt.Errorf("failed to load %s: %w", result.name, result.err)
			}
			_, _ = fmt.Fprintf(writer, "  ✅ %s image loaded\n", result.name)
		}
		_, _ = fmt.Fprintln(writer, "✅ All images loaded!")
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 3.5: Deploy DataStorage RBAC (DD-AUTH-014)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n🔐 PHASE 3.5: Deploying DataStorage RBAC (DD-AUTH-014)...")

	// Deploy data-storage-client ClusterRole (DD-AUTH-014)
	// CRITICAL: This must be deployed BEFORE RoleBindings that reference it
	// Required for SAR checks to pass when E2E SA seeds workflows
	_, _ = fmt.Fprintf(writer, "  🔐 Deploying data-storage-client ClusterRole...\n")
	if err := deployDataStorageClientClusterRole(ctx, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy client ClusterRole: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "  ✅ DataStorage client ClusterRole deployed\n")

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 4a: Deploy DataStorage infrastructure FIRST (required for workflow seeding)
	// ═══════════════════════════════════════════════════════════════════════
	// Pattern: Match AA E2E workflow seeding approach (aianalysis_e2e.go Phase 7)
	// DataStorage must be ready BEFORE Mock LLM deployment so workflows can be seeded
	// and ConfigMap created with actual UUIDs
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 4a: Deploying DataStorage infrastructure...")
	_, _ = fmt.Fprintln(writer, "  ⏱️  Must complete before workflow seeding")

	// Create namespace FIRST (required for all subsequent deployments)
	_, _ = fmt.Fprintf(writer, "📁 Creating namespace %s...\n", namespace)
	if err := createTestNamespace(namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	type deployResult struct {
		name string
		err  error
	}
	deployResults := make(chan deployResult, 4)

	// Deploy DataStorage dependencies in parallel
	go func() {
		defer GinkgoRecover()
		err := deployPostgreSQLInNamespace(ctx, namespace, kubeconfigPath, writer)
		deployResults <- deployResult{"PostgreSQL", err}
	}()
	go func() {
		defer GinkgoRecover()
		err := deployRedisInNamespace(ctx, namespace, kubeconfigPath, writer)
		deployResults <- deployResult{"Redis", err}
	}()
	go func() {
		defer GinkgoRecover()
		err := ApplyAllMigrations(ctx, namespace, kubeconfigPath, writer)
		deployResults <- deployResult{"Migrations", err}
	}()
	go func() {
		defer GinkgoRecover()
		// DD-AUTH-014: Create ServiceAccount BEFORE deployment
		_, _ = fmt.Fprintf(writer, "  🔐 Creating DataStorage ServiceAccount + RBAC...\n")
		if err := deployDataStorageServiceRBAC(ctx, namespace, kubeconfigPath, writer); err != nil {
			deployResults <- deployResult{"DataStorage", fmt.Errorf("failed to create ServiceAccount: %w", err)}
			return
		}

		err := deployDataStorageServiceInNamespaceWithNodePort(ctx, namespace, kubeconfigPath, dataStorageImage, 30089, writer)
		deployResults <- deployResult{"DataStorage", err}
	}()

	// Wait for DataStorage infrastructure
	var deployErrors []error
	for i := 0; i < 4; i++ {
		result := <-deployResults
		if result.err != nil {
			_, _ = fmt.Fprintf(writer, "  ❌ %s deployment failed: %v\n", result.name, result.err)
			deployErrors = append(deployErrors, result.err)
		} else {
			_, _ = fmt.Fprintf(writer, "  ✅ %s manifests applied\n", result.name)
		}
	}

	if len(deployErrors) > 0 {
		return fmt.Errorf("DataStorage infrastructure deployment failed: %v", deployErrors)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 4b: Wait for DataStorage to be ready
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 4b: Waiting for DataStorage to be ready...")
	if err := waitForDataStorageReady(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("DataStorage not ready: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 4c: Seed workflows and create ConfigMap (DD-TEST-011)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 4c: Seeding test workflows and creating ConfigMap...")

	// Create E2E ServiceAccount for workflow seeding (DD-AUTH-014)
	_, _ = fmt.Fprintf(writer, "  🔐 Creating E2E ServiceAccount for workflow seeding...\n")
	if err := createHolmesGPTAPIE2EServiceAccount(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create E2E ServiceAccount: %w", err)
	}

	// Get ServiceAccount token for DataStorage authentication
	_, _ = fmt.Fprintf(writer, "  🔐 Creating authenticated DataStorage client for workflow seeding...\n")
	e2eSAName := "holmesgpt-api-e2e-sa"
	saToken, err := GetServiceAccountToken(ctx, namespace, e2eSAName, kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to get ServiceAccount token: %w", err)
	}

	// Create authenticated DataStorage client
	dsURL := "http://localhost:8089" // DataStorage NodePort
	seedClient, err := createAuthenticatedDataStorageClient(dsURL, saToken)
	if err != nil {
		return fmt.Errorf("failed to create DataStorage client: %w", err)
	}

	// Get test workflows (from shared library)
	testWorkflows := GetHAPIE2ETestWorkflows()
	_, _ = fmt.Fprintf(writer, "  📋 Preparing %d test workflows...\n", len(testWorkflows))

	// Seed workflows and capture UUIDs
	workflowUUIDs, err := SeedWorkflowsInDataStorage(seedClient, testWorkflows, "HAPI E2E (via infrastructure)", writer)
	if err != nil {
		return fmt.Errorf("failed to seed test workflows: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "  ✅ Seeded %d workflows in DataStorage\n", len(workflowUUIDs))

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 4d: Deploy Mock LLM with workflow UUIDs
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 4d: Deploying Mock LLM with workflow UUIDs...")
	if err := deployMockLLMInNamespace(ctx, namespace, kubeconfigPath, mockLLMImage, workflowUUIDs, writer); err != nil {
		return fmt.Errorf("failed to deploy Mock LLM: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "  ✅ Mock LLM deployed with ConfigMap")

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 4e: Deploy HAPI
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 4e: Deploying HAPI...")

	// Create HAPI ServiceAccount
	_, _ = fmt.Fprintf(writer, "  🔐 Creating HAPI ServiceAccount + RBAC...\n")
	if err := deployHAPIServiceRBAC(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create HAPI ServiceAccount: %w", err)
	}

	if err := deployHAPIOnly(clusterName, kubeconfigPath, namespace, hapiImage, writer); err != nil {
		return fmt.Errorf("failed to deploy HAPI: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "  ✅ HAPI deployed")

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 4f: Wait for all services to be ready
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n⏳ Waiting for all services to be ready...")
	if err := waitForHAPIServicesReady(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("services not ready: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "✅ HAPI E2E Infrastructure Ready")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	return nil
}

// createHAPIKindCluster creates a Kind cluster with HAPI-specific port mappings
// Per DD-TEST-001 v1.8
func createHAPIKindCluster(clusterName, kubeconfigPath string, writer io.Writer) error {
	// REFACTORED: Now uses shared CreateKindClusterWithConfig() helper
	// Authority: Aligns with RO, Gateway, and other E2E tests that successfully use Podman
	// Fixes: Kind + Podman compatibility issues (exit status 126, /dev/mapper mount failures)

	// DD-TEST-007: Create coverdata directory for E2E coverage collection
	// The Kind config extraMount uses ./coverdata (relative to project root)
	if os.Getenv("E2E_COVERAGE") == "true" {
		projectRoot := getProjectRoot()
		coverdataPath := filepath.Join(projectRoot, "coverdata")
		if err := os.MkdirAll(coverdataPath, 0777); err != nil {
			_, _ = fmt.Fprintf(writer, "⚠️  Failed to create coverdata directory: %v\n", err)
		} else {
			// CRITICAL: os.MkdirAll applies umask (0022), resulting in 0755 (rwxr-xr-x).
			// Container processes (UID 1001) need write access to /coverdata via hostPath volume.
			// os.Chmod bypasses umask, ensuring world-writable permissions propagate through
			// the Kind bind mount → pod hostPath chain.
			if err := os.Chmod(coverdataPath, 0777); err != nil {
				_, _ = fmt.Fprintf(writer, "  ⚠️  Failed to chmod coverdata directory: %v\n", err)
			}
			_, _ = fmt.Fprintf(writer, "  ✅ Created %s for Python coverage collection (mode=0777)\n", coverdataPath)
		}
	}

	// Use shared helper with Podman support (fixes Kind compatibility issues)
	opts := KindClusterOptions{
		ClusterName:               clusterName,
		KubeconfigPath:            kubeconfigPath,
		ConfigPath:                "test/infrastructure/kind-holmesgpt-api-config.yaml", // Static config (like RO, Gateway, etc.)
		WaitTimeout:               "5m",
		DeleteExisting:            true,  // Original behavior
		ReuseExisting:             false, // Original behavior
		CleanupOrphanedContainers: true,  // Podman cleanup on macOS
		UsePodman:                 true,  // CRITICAL: Sets KIND_EXPERIMENTAL_PROVIDER=podman
		ProjectRootAsWorkingDir:   true,  // DD-TEST-007: Required for ./coverdata extraMount resolution
	}
	return CreateKindClusterWithConfig(opts, writer)
}

// deployDataStorageForHAPI deploys Data Storage service to Kind cluster
// Uses HAPI-specific NodePort (30089) per DD-TEST-001 v2.5
// deployHAPIOnly deploys HAPI service to Kind cluster
// Per DD-TEST-001 v2.5: NodePort 30120
func deployHAPIOnly(clusterName, kubeconfigPath, namespace, imageTag string, writer io.Writer) error {
	// DD-TEST-007: Conditionally add Python coverage instrumentation
	coverageEnv := ""
	coverageVolumeMount := ""
	coverageVolume := ""
	if os.Getenv("E2E_COVERAGE") == "true" {
		coverageEnv = `- name: E2E_COVERAGE
          value: "true"
        - name: COVERAGE_FILE
          value: "/coverdata/.coverage"`
		coverageVolumeMount = `- name: coverdata
          mountPath: /coverdata`
		coverageVolume = `- name: coverdata
        hostPath:
          path: /coverdata
          type: DirectoryOrCreate`
		// NOTE: fsGroup removed — it has NO effect on hostPath volumes.
		// Write permissions are ensured by:
		// 1. os.Chmod(coverdataPath, 0777) on the host directory
		// 2. ensureCoverdataWritableInKindNode() chmod 777 inside Kind node
		_, _ = fmt.Fprintln(writer, "  📊 DD-TEST-007: Python E2E coverage instrumentation ENABLED")
	}

	// ADR-030: Create HAPI ConfigMap with minimal config
	deployment := fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: holmesgpt-api-config
  namespace: %s
data:
  config.yaml: |
    logging:
      level: "INFO"
    llm:
      provider: "openai"
      model: "mock-model"
      endpoint: "http://mock-llm:8080"
    data_storage:
      url: "http://data-storage-service:8080"  # DD-AUTH-011: Match Service name
    audit:
      flush_interval_seconds: 0.1
      buffer_size: 10000
      batch_size: 50
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: holmesgpt-api
  namespace: %s
spec:
  replicas: 1
  selector:
    matchLabels:
      app: holmesgpt-api
  template:
    metadata:
      labels:
        app: holmesgpt-api
    spec:
      serviceAccountName: holmesgpt-api-sa  # DD-AUTH-014: Required for DataStorage authentication
      containers:
      - name: holmesgpt-api
        image: %s
        imagePullPolicy: %s
        ports:
        - containerPort: 8080
        args:
        - "-config"
        - "/etc/holmesgpt/config.yaml"
        env:
        - name: LLM_ENDPOINT
          value: "http://mock-llm:8080"
        - name: LLM_MODEL
          value: "mock-model"
        - name: LLM_PROVIDER
          value: "openai"
        - name: OPENAI_API_KEY
          value: "mock-api-key-for-e2e"
        - name: DATA_STORAGE_URL
          value: "http://data-storage-service:8080"  # DD-AUTH-011: Match Service name
        - name: LITELLM_LOG
          value: "DEBUG"  # Enable LiteLLM debug logging
        %s
        volumeMounts:
        - name: config
          mountPath: /etc/holmesgpt
          readOnly: true
        %s
      volumes:
      - name: config
        configMap:
          name: holmesgpt-api-config
      %s
---
apiVersion: v1
kind: Service
metadata:
  name: holmesgpt-api
  namespace: %s
spec:
  type: NodePort
  ports:
  - port: 8080
    targetPort: 8080
    nodePort: 30120
  selector:
    app: holmesgpt-api
`, namespace, namespace, imageTag, GetImagePullPolicy(), coverageEnv, coverageVolumeMount, coverageVolume, namespace)

	// Apply manifest
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(deployment)
	cmd.Stdout = writer
	cmd.Stderr = writer

	return cmd.Run()
}

// deployHAPIServiceRBAC creates ServiceAccount + RoleBinding for HAPI to access DataStorage
// DD-AUTH-014: HAPI uses ServiceAccountAuthPoolManager to inject Bearer tokens
// Token is read from /var/run/secrets/kubernetes.io/serviceaccount/token (auto-mounted by K8s)
//
// CRITICAL: ServiceAccount alone is NOT enough!
// DataStorage middleware performs SubjectAccessReview (SAR) to check permissions.
// Without RoleBinding → SAR fails → 401 Unauthorized
func deployHAPIServiceRBAC(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	// HAPI RBAC Strategy (DD-AUTH-014):
	// 1. holmesgpt-api-sa: HAPI pod identity (TokenReview/SAR + DataStorage client)
	// 2. holmesgpt-api-e2e-sa: E2E test identity (mimics AIAnalysis calling HAPI)
	//    Pattern matches other E2E tests: aianalysis-e2e-sa, gateway-e2e-sa, etc.
	rbacManifest := fmt.Sprintf(`---
# HAPI Pod ServiceAccount (for HAPI pod itself)
apiVersion: v1
kind: ServiceAccount
metadata:
  name: holmesgpt-api-sa
  namespace: %s
  labels:
    app: holmesgpt-api
    component: auth
    authorization: dd-auth-014
---
# ClusterRole: DataStorage client permissions (for middleware SAR check)
# NOTE: This ClusterRole should already exist from DataStorage deployment
# If not, E2E test will fail - this is expected (DataStorage must be deployed first)
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: data-storage-client
  labels:
    app: data-storage-service
    component: rbac
    authorization: dd-auth-014
rules:
  # Middleware SAR check: Full CRUD permissions for DataStorage REST API
  - apiGroups: [""]
    resources: ["services"]
    resourceNames: ["data-storage-service"]
    verbs: ["create", "get", "list", "update", "delete"]
---
# RoleBinding: Grant HAPI pod access to DataStorage
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: holmesgpt-api-data-storage-client
  namespace: %s
  labels:
    app: holmesgpt-api
    component: rbac
    authorization: dd-auth-014
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: data-storage-client
subjects:
  - kind: ServiceAccount
    name: holmesgpt-api-sa
    namespace: %s
---
# ClusterRoleBinding: Grant HAPI auth middleware permissions (TokenReview + SAR)
# DD-AUTH-014: HAPI middleware needs to validate incoming Bearer tokens
# Uses existing data-storage-auth-middleware ClusterRole (same permissions needed)
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: holmesgpt-api-auth-middleware
  labels:
    app: holmesgpt-api
    component: rbac
    authorization: dd-auth-014
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: data-storage-auth-middleware
subjects:
  - kind: ServiceAccount
    name: holmesgpt-api-sa
    namespace: %s
---
# ClusterRole: K8s investigation permissions for HolmesGPT kubernetes/core toolset
# Required for kubectl-based pod/event investigation during incident analysis
# Without this, the LLM cannot gather evidence and skips workflow catalog search
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: holmesgpt-api-investigator
  labels:
    app: holmesgpt-api
    component: investigation
rules:
  - apiGroups: [""]
    resources: ["pods", "pods/log", "events", "services", "configmaps", "nodes", "namespaces", "replicationcontrollers", "persistentvolumeclaims"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["apps"]
    resources: ["deployments", "replicasets", "statefulsets", "daemonsets"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["batch"]
    resources: ["jobs", "cronjobs"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["events.k8s.io"]
    resources: ["events"]
    verbs: ["get", "list", "watch"]
---
# ClusterRoleBinding: Grant HAPI investigation permissions cluster-wide
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: holmesgpt-api-investigator-binding
  labels:
    app: holmesgpt-api
    component: investigation
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: holmesgpt-api-investigator
subjects:
  - kind: ServiceAccount
    name: holmesgpt-api-sa
    namespace: %s
`, namespace, namespace, namespace, namespace, namespace)

	// Apply manifest
	applyCmd := exec.CommandContext(ctx, "kubectl", "apply", "--kubeconfig", kubeconfigPath, "-f", "-")
	applyCmd.Stdin = strings.NewReader(rbacManifest)
	applyCmd.Stdout = writer
	applyCmd.Stderr = writer

	if err := applyCmd.Run(); err != nil {
		return fmt.Errorf("failed to apply HAPI service RBAC: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "✅ HAPI ServiceAccount deployed (for DataStorage authentication)\n")
	return nil
}

// waitForHAPIServicesReady waits for DataStorage and HolmesGPT-API pods to be ready
// Per DD-TEST-002: Single readiness check after parallel deployment
func waitForHAPIServicesReady(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
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

	// Wait for HolmesGPT-API pod to be ready
	_, _ = fmt.Fprintf(writer, "   ⏳ Waiting for HolmesGPT-API pod to be ready...\n")
	Eventually(func() bool {
		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app=holmesgpt-api",
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
	}, 3*time.Minute, 5*time.Second).Should(BeTrue(), "HolmesGPT-API pod should become ready")
	_, _ = fmt.Fprintf(writer, "   ✅ HolmesGPT-API ready\n")

	return nil
}

// createHolmesGPTAPIE2EServiceAccount creates the E2E test ServiceAccount with HAPI + DataStorage client permissions
// Pattern matches other E2E tests: aianalysis-e2e-sa, gateway-e2e-sa, etc.
// DD-AUTH-014: E2E SA needs permissions to:
//  1. Call HAPI endpoints (mimics AIAnalysis)
//  2. Call DataStorage endpoints (for workflow seeding in BeforeSuite)
func createHolmesGPTAPIE2EServiceAccount(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	saName := "holmesgpt-api-e2e-sa"

	// Create ServiceAccount
	if err := CreateServiceAccount(ctx, namespace, kubeconfigPath, saName, writer); err != nil {
		return fmt.Errorf("failed to create E2E ServiceAccount: %w", err)
	}

	// RBAC Part 1: HAPI client access (Role + RoleBinding)
	hapiRBACYAML := fmt.Sprintf(`---
# Role: HAPI client access (for E2E test SA - mimics AIAnalysis)
# DD-AUTH-014: E2E SA needs permission to call HAPI endpoints
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: holmesgpt-api-e2e-client-access
  namespace: %s
  labels:
    app: holmesgpt-api
    component: e2e-testing
    authorization: dd-auth-014
rules:
  # SAR check: POST /api/v1/incident/analyze and /api/v1/recovery/analyze
  - apiGroups: [""]
    resources: ["services"]
    resourceNames: ["holmesgpt-api"]
    verbs: ["create", "get"]  # create=POST, get=GET
---
# RoleBinding: Grant E2E SA access to HAPI
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: holmesgpt-api-e2e-client-access
  namespace: %s
  labels:
    app: holmesgpt-api
    component: e2e-testing
    authorization: dd-auth-014
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: holmesgpt-api-e2e-client-access
subjects:
  - kind: ServiceAccount
    name: %s
    namespace: %s
`, namespace, namespace, saName, namespace)

	cmd := exec.CommandContext(ctx, "kubectl", "apply", "--kubeconfig", kubeconfigPath, "-f", "-")
	cmd.Stdin = strings.NewReader(hapiRBACYAML)
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to apply HAPI E2E RBAC: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "  ✅ HAPI client RBAC created\n")

	// RBAC Part 2: DataStorage client access (RoleBinding to ClusterRole)
	// DD-TEST-011 v2.0: E2E SA needs to seed workflows in BeforeSuite
	// Pattern: Same as aianalysis-e2e-sa (binds to data-storage-client ClusterRole)
	_, _ = fmt.Fprintf(writer, "  🔐 Creating DataStorage client RoleBinding for workflow seeding...\n")
	if err := CreateDataStorageAccessRoleBinding(ctx, namespace, kubeconfigPath, saName, writer); err != nil {
		return fmt.Errorf("failed to create DataStorage client RoleBinding: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "  ✅ E2E ServiceAccount created with HAPI + DataStorage client permissions\n")
	return nil
}

// deployMockLLMInNamespace deploys the standalone Mock LLM service to a namespace
// This is the V2.0 Mock LLM service extracted from HAPI business code
// Uses ClusterIP for internal access only (no NodePort needed for E2E)
func deployMockLLMInNamespace(ctx context.Context, namespace, kubeconfigPath, imageTag string, workflowUUIDs map[string]string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "   📦 Deploying Mock LLM service (image: %s)...\n", imageTag)

	// Create ConfigMap with scenarios for Mock LLM
	// If workflowUUIDs provided (AIAnalysis E2E): Use actual UUIDs
	// If workflowUUIDs nil/empty (HAPI E2E): Use empty scenarios
	var scenariosYAML string
	if len(workflowUUIDs) > 0 {
		// Build YAML map with workflow UUIDs (AIAnalysis E2E)
		scenariosYAML = "scenarios:\n"
		for key, uuid := range workflowUUIDs {
			scenariosYAML += fmt.Sprintf("      %s: %s\n", key, uuid)
		}
	} else {
		// Empty scenarios (HAPI E2E - no workflows seeded)
		scenariosYAML = "scenarios: {}"
	}

	configMap := fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: mock-llm-scenarios
  namespace: %s
  labels:
    app: mock-llm
    component: test-infrastructure
data:
  scenarios.yaml: |
    %s
---`, namespace, scenariosYAML)

	_, _ = fmt.Fprintf(writer, "   📦 Creating Mock LLM ConfigMap...\n")
	cmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", "-", "--kubeconfig", kubeconfigPath)
	cmd.Stdin = strings.NewReader(configMap)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create Mock LLM ConfigMap: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "   ✅ ConfigMap created\n")

	// Use the manifests from deploy/mock-llm/ with the provided image tag
	deployment := fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: mock-llm
  namespace: %s
  labels:
    app: mock-llm
    component: test-infrastructure
spec:
  replicas: 1
  selector:
    matchLabels:
      app: mock-llm
  template:
    metadata:
      labels:
        app: mock-llm
        component: test-infrastructure
    spec:
      containers:
      - name: mock-llm
        image: %s
        imagePullPolicy: %s
        ports:
        - containerPort: 8080
          name: http
          protocol: TCP
        env:
        - name: MOCK_LLM_HOST
          value: "0.0.0.0"
        - name: MOCK_LLM_PORT
          value: "8080"
        - name: MOCK_LLM_FORCE_TEXT
          value: "false"
        - name: MOCK_LLM_CONFIG_PATH
          value: "/config/scenarios.yaml"
        volumeMounts:
        - name: scenarios-config
          mountPath: /config
          readOnly: true
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
            scheme: HTTP
          initialDelaySeconds: 10
          periodSeconds: 10
          timeoutSeconds: 3
          successThreshold: 1
          failureThreshold: 3
        readinessProbe:
          httpGet:
            path: /health
            port: 8080
            scheme: HTTP
          initialDelaySeconds: 5
          periodSeconds: 5
          timeoutSeconds: 3
          successThreshold: 1
          failureThreshold: 3
        resources:
          requests:
            memory: "64Mi"
            cpu: "100m"
          limits:
            memory: "128Mi"
            cpu: "200m"
        securityContext:
          allowPrivilegeEscalation: false
          runAsNonRoot: true
          runAsUser: 1001
          capabilities:
            drop:
            - ALL
      volumes:
      - name: scenarios-config
        configMap:
          name: mock-llm-scenarios
      securityContext:
        fsGroup: 1001
      restartPolicy: Always
---
apiVersion: v1
kind: Service
metadata:
  name: mock-llm
  namespace: %s
  labels:
    app: mock-llm
    component: test-infrastructure
spec:
  type: ClusterIP
  ports:
  - port: 8080
    targetPort: 8080
    protocol: TCP
    name: http
  selector:
    app: mock-llm
`, namespace, imageTag, GetImagePullPolicy(), namespace)

	cmd = exec.CommandContext(ctx, "kubectl", "apply", "-f", "-", "--kubeconfig", kubeconfigPath)
	cmd.Stdin = strings.NewReader(deployment)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to deploy Mock LLM: %w", err)
	}

	// Get Kubernetes client
	clientset, err := getKubernetesClient(kubeconfigPath)
	if err != nil {
		return err
	}

	// Wait for Mock LLM pod to be ready
	_, _ = fmt.Fprintf(writer, "   ⏳ Waiting for Mock LLM pod to be ready...\n")
	Eventually(func() bool {
		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app=mock-llm",
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
	}, 2*time.Minute, 5*time.Second).Should(BeTrue(), "Mock LLM pod should become ready")
	_, _ = fmt.Fprintf(writer, "   ✅ Mock LLM ready\n")

	return nil
}

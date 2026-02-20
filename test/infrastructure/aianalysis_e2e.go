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
	"net/http"
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

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
)

func CreateAIAnalysisClusterHybrid(clusterName, kubeconfigPath string, writer io.Writer) error {
	ctx := context.Background()
	namespace := "kubernaut-system" // Infrastructure always in kubernaut-system; tests use dynamic namespaces

	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "🚀 AIAnalysis E2E Infrastructure (HYBRID PARALLEL + DISK OPTIMIZATION)")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "  Strategy: Build → Export → Prune → Cluster → Load → Deploy")
	_, _ = fmt.Fprintln(writer, "  Benefits: Fast builds + Aggressive cleanup + Disk tracking")
	_, _ = fmt.Fprintln(writer, "  Per DD-TEST-002: Hybrid Parallel Setup Standard")
	_, _ = fmt.Fprintln(writer, "  Per DD-TEST-008: Disk Space Management")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Track initial disk space
	LogDiskSpace("START", writer)

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 1: Build images IN PARALLEL (before cluster creation)
	// ═══════════════════════════════════════════════════════════════════════
	// Per Consolidated API Migration (January 2026):
	// - Uses BuildImageForKind() for all images
	// - Returns dynamic image names for later use
	// - No manual tag generation
	// - Authority: docs/handoff/CONSOLIDATED_API_MIGRATION_GUIDE_JAN07.md
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 1: Building images in parallel...")
	_, _ = fmt.Fprintln(writer, "  ├── Data Storage (1-2 min)")
	_, _ = fmt.Fprintln(writer, "  ├── HolmesGPT-API (2-3 min)")
	_, _ = fmt.Fprintln(writer, "  ├── Mock LLM (1-2 min)")
	_, _ = fmt.Fprintln(writer, "  └── AIAnalysis controller (3-4 min)")

	type imageBuildResult struct {
		name  string
		image string
		err   error
	}

	buildResults := make(chan imageBuildResult, 4)

	go func() {
		cfg := E2EImageConfig{
			ServiceName:      "datastorage",
			ImageName:        "kubernaut/datastorage",
			DockerfilePath:   "docker/data-storage.Dockerfile",
			BuildContextPath: "",
			EnableCoverage:   os.Getenv("E2E_COVERAGE") == "true",
		}
		imageName, err := BuildImageForKind(cfg, writer)
		buildResults <- imageBuildResult{"datastorage", imageName, err}
	}()

	go func() {
		cfg := E2EImageConfig{
			ServiceName:      "holmesgpt-api",
			ImageName:        "kubernaut/holmesgpt-api",
			DockerfilePath:   "holmesgpt-api/Dockerfile",
			BuildContextPath: "",
			EnableCoverage:   false, // HAPI does not support coverage
		}
		imageName, err := BuildImageForKind(cfg, writer)
		buildResults <- imageBuildResult{"holmesgpt-api", imageName, err}
	}()

	go func() {
		cfg := E2EImageConfig{
			ServiceName:      "aianalysis", // Operator SDK convention: no -controller suffix in image name
			ImageName:        "kubernaut/aianalysis",
			DockerfilePath:   "docker/aianalysis.Dockerfile", // Dockerfile can have suffix (but this one doesn't)
			BuildContextPath: "",
			EnableCoverage:   os.Getenv("E2E_COVERAGE") == "true",
		}
		imageName, err := BuildImageForKind(cfg, writer)
		buildResults <- imageBuildResult{"aianalysis", imageName, err}
	}()

	go func() {
		projectRoot := getProjectRoot()
		cfg := E2EImageConfig{
			ServiceName:      "mock-llm",
			ImageName:        "kubernaut/mock-llm",
			DockerfilePath:   "test/services/mock-llm/Dockerfile",
			BuildContextPath: filepath.Join(projectRoot, "test/services/mock-llm"),
			EnableCoverage:   false,
		}
		imageName, err := BuildImageForKind(cfg, writer)
		buildResults <- imageBuildResult{"mock-llm", imageName, err}
	}()

	builtImages := make(map[string]string)
	for i := 0; i < 4; i++ {
		result := <-buildResults
		if result.err != nil {
			return fmt.Errorf("failed to build %s image: %w", result.name, result.err)
		}
		builtImages[result.name] = result.image
		_, _ = fmt.Fprintf(writer, "  ✅ %s image built: %s\n", result.name, result.image)
	}
	_, _ = fmt.Fprintln(writer, "\n✅ All images built! (~3-4 min parallel)")
	LogDiskSpace("IMAGES_BUILT", writer)

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 2-3: Export images to .tar and aggressive Podman cleanup
	// ═══════════════════════════════════════════════════════════════════════
	// This frees ~5-9 GB of disk space by removing Podman cache and intermediate layers
	// FIX: Skip export/prune in CI/CD mode (images pushed to registry, not stored locally)
	var tarFiles map[string]string
	var err error
	if ShouldSkipImageExportAndPrune() {
		_, _ = fmt.Fprintln(writer, "\n⏩ PHASE 2-3: Skipping image export/prune (CI/CD registry mode)")
		_, _ = fmt.Fprintln(writer, "   Images already pushed to GHCR and will be pulled directly by Kind")
		LogDiskSpace("EXPORT_SKIPPED", writer)
	} else {
		// Local mode: export and prune to save disk space
		_, _ = fmt.Fprintln(writer, "\n📦 PHASE 2-3: Exporting images to .tar and pruning Podman cache...")
		tarFiles, err = ExportImagesAndPrune(builtImages, "/tmp", writer)
		if err != nil {
			return fmt.Errorf("failed to export images and prune: %w", err)
		}
	}

	// DD-TEST-007: Create coverdata directory BEFORE Kind cluster creation
	// The Kind config extraMount uses ./coverdata relative to project root
	if os.Getenv("E2E_COVERAGE") == "true" {
		projectRoot := getProjectRoot()
		coverdataPath := filepath.Join(projectRoot, "coverdata")
		_, _ = fmt.Fprintf(writer, "📁 Creating coverage directory: %s\n", coverdataPath)
		if err := os.MkdirAll(coverdataPath, 0777); err != nil {
			return fmt.Errorf("failed to create coverdata directory: %w", err)
		}
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 4: Create Kind cluster (AFTER cleanup to maximize available space)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 4: Creating Kind cluster...")
	if err := createAIAnalysisKindCluster(clusterName, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create Kind cluster: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "📁 Creating namespace...")
	createNsCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
		"create", "namespace", namespace)
	nsOutput := &strings.Builder{}
	createNsCmd.Stdout = io.MultiWriter(writer, nsOutput)
	createNsCmd.Stderr = io.MultiWriter(writer, nsOutput)
	if err := createNsCmd.Run(); err != nil {
		if !strings.Contains(nsOutput.String(), "AlreadyExists") {
			return fmt.Errorf("failed to create namespace: %w", err)
		}
	}

	_, _ = fmt.Fprintln(writer, "📋 Installing AIAnalysis CRD...")
	if err := installAIAnalysisCRD(kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to install AIAnalysis CRD: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 5-6: Load images from .tar into Kind and cleanup .tar files
	// ═══════════════════════════════════════════════════════════════════════
	// Uses shared helpers for efficient loading and cleanup
	// FIX: Skip image loading in CI/CD mode (images will be pulled from registry)
	if os.Getenv("IMAGE_REGISTRY") != "" {
		_, _ = fmt.Fprintln(writer, "\n⏩ PHASE 5-6: Skipping .tar image loading (CI/CD registry mode)")
		_, _ = fmt.Fprintln(writer, "   Kind will pull images directly from GHCR as needed")
	} else {
		// Local mode: load images from .tar files
		_, _ = fmt.Fprintln(writer, "\n📦 PHASE 5-6: Loading images from .tar into Kind...")
		if err := LoadImagesAndCleanup(clusterName, tarFiles, writer); err != nil {
			return fmt.Errorf("failed to load images and cleanup: %w", err)
		}
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 6.5: Deploy DataStorage RBAC (DD-AUTH-014)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n🔐 PHASE 6.5: Deploying DataStorage RBAC (DD-AUTH-014)...")

	// Step 0: Deploy data-storage-client ClusterRole (DD-AUTH-014)
	// CRITICAL: This must be deployed BEFORE RoleBindings that reference it
	_, _ = fmt.Fprintf(writer, "  🔐 Deploying data-storage-client ClusterRole...\n")
	if err := deployDataStorageClientClusterRole(ctx, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy client ClusterRole: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "  ✅ DataStorage RBAC deployed\n")

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 7a: Deploy DataStorage infrastructure FIRST (required for workflow seeding)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 7a: Deploying DataStorage infrastructure...")
	_, _ = fmt.Fprintln(writer, "  └── PostgreSQL + Redis + DataStorage + Migrations")
	_, _ = fmt.Fprintln(writer, "  ⏱️  Must complete before workflow seeding")

	// Deploy Data Storage infrastructure with OAuth2-Proxy (TD-E2E-001 Phase 1)
	if err := DeployDataStorageTestServices(ctx, namespace, kubeconfigPath, builtImages["datastorage"], writer); err != nil {
		return fmt.Errorf("DataStorage infrastructure deployment failed: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "  ✅ DataStorage infrastructure deployed successfully")

	// Create ServiceAccount for workflow seeding with DataStorage access (DD-AUTH-014)
	// This MUST happen before workflow seeding (Phase 7b) since seeding needs the SA token
	_, _ = fmt.Fprintln(writer, "  🔐 Creating ServiceAccount for workflow seeding...")
	if err := createAIAnalysisE2EServiceAccount(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create E2E ServiceAccount: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "  ✅ aianalysis-e2e-sa created with DataStorage access")

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 7b: Seed workflows and create ConfigMap (DD-TEST-011 Alt 2)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 7b: Seeding test workflows and creating ConfigMap...")
	_, _ = fmt.Fprintln(writer, "  └── DD-TEST-011 Alt 2: ConfigMap Pattern")

	// Wait for DataStorage to be ready (use port-forward)
	_, _ = fmt.Fprintln(writer, "  ⏳ Waiting for DataStorage to be ready...")
	dataStorageURL := fmt.Sprintf("http://localhost:%d", 38080)

	// Start port-forward to DataStorage
	// Service name is "data-storage-service" per DD-AUTH-011 (matches production)
	portForwardCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
		"-n", namespace, "port-forward", "svc/data-storage-service", "38080:8080")
	if err := portForwardCmd.Start(); err != nil {
		return fmt.Errorf("failed to start DataStorage port-forward: %w", err)
	}
	defer func() {
		if portForwardCmd.Process != nil {
			_ = portForwardCmd.Process.Kill()
		}
	}()

	// Wait for port-forward to be ready (active polling, not fixed sleep)
	_, _ = fmt.Fprintln(writer, "  ⏳ Waiting for port-forward to be ready (active polling)...")
	ready := false
	for i := 0; i < 30; i++ { // 30 seconds max
		time.Sleep(1 * time.Second)
		resp, err := http.Get(fmt.Sprintf("%s/health", dataStorageURL))
		if err == nil && resp.StatusCode == 200 {
			_ = resp.Body.Close() // Explicitly ignore - health check cleanup
			ready = true
			_, _ = fmt.Fprintf(writer, "  ✅ Port-forward ready after %d seconds\n", i+1)
			break
		}
		if resp != nil {
			_ = resp.Body.Close() // Explicitly ignore - health check cleanup
		}
	}
	if !ready {
		return fmt.Errorf("port-forward not ready after 30 seconds")
	}

	// Seed workflows and capture UUIDs (with DD-AUTH-014 authentication)
	// Pattern: Uses shared workflow_seeding.go library (refactored from duplicate code)
	_, _ = fmt.Fprintln(writer, "  🔐 Creating authenticated DataStorage client for workflow seeding...")

	// Get ServiceAccount token for authentication
	// GetServiceAccountToken signature: (ctx, namespace, saName, kubeconfigPath)
	saToken, err := GetServiceAccountToken(context.Background(), namespace, "aianalysis-e2e-sa", kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to get ServiceAccount token: %w", err)
	}

	// Create authenticated OpenAPI client for DataStorage
	seedClient, err := ogenclient.NewClient(
		dataStorageURL,
		ogenclient.WithClient(&http.Client{
			Transport: testauth.NewServiceAccountTransport(saToken),
			Timeout:   30 * time.Second,
		}),
	)
	if err != nil {
		return fmt.Errorf("failed to create DataStorage client: %w", err)
	}

	// Inline workflow definitions (CANNOT use test/integration/aianalysis wrapper - import cycle)
	// Pattern: DD-TEST-011 v2.0 - Use shared SeedWorkflowsInDataStorage() function
	// Note: test/integration/aianalysis imports test/infrastructure, creating circular dependency
	// Acceptable trade-off: Small duplication avoids architectural issues
	// Source of truth: test/integration/aianalysis/test_workflows.go:GetAIAnalysisTestWorkflows()
	// BR-HAPI-191: SchemaParameters MUST match Mock LLM scenario parameters
	// HAPI validates LLM response parameters against workflow schema from DataStorage
	// DD-WORKFLOW-017: SchemaParameters mirror OCI image's /workflow-schema.yaml for documentation.
	// Actual schema comes from OCI image via pullspec-only registration.
	oomkillParams := []models.WorkflowParameter{
		{Name: "NAMESPACE", Type: "string", Required: true, Description: "Target namespace containing the affected deployment"},
		{Name: "DEPLOYMENT_NAME", Type: "string", Required: true, Description: "Name of the deployment to update memory limits"},
		{Name: "MEMORY_INCREASE_PERCENT", Type: "integer", Required: false, Description: "Percentage to increase memory limits by"},
	}
	crashloopParams := []models.WorkflowParameter{
		{Name: "NAMESPACE", Type: "string", Required: true, Description: "Target namespace"},
		{Name: "DEPLOYMENT_NAME", Type: "string", Required: true, Description: "Name of the deployment to restart"},
		{Name: "GRACE_PERIOD_SECONDS", Type: "integer", Required: false, Description: "Graceful shutdown period in seconds"},
	}
	nodeDrainParams := []models.WorkflowParameter{
		{Name: "NODE_NAME", Type: "string", Required: true, Description: "Name of the node to drain and reboot"},
		{Name: "DRAIN_TIMEOUT_SECONDS", Type: "integer", Required: false, Description: "Timeout for drain operation in seconds"},
	}
	memOptimizeParams := []models.WorkflowParameter{
		{Name: "NAMESPACE", Type: "string", Required: true, Description: "Target namespace"},
		{Name: "DEPLOYMENT_NAME", Type: "string", Required: true, Description: "Name of the deployment to scale"},
		{Name: "REPLICA_COUNT", Type: "integer", Required: false, Description: "Target number of replicas"},
	}
	genericRestartParams := []models.WorkflowParameter{
		{Name: "NAMESPACE", Type: "string", Required: true, Description: "Target namespace"},
		{Name: "POD_NAME", Type: "string", Required: true, Description: "Name of the pod to restart"},
	}
	testSignalParams := []models.WorkflowParameter{
		{Name: "NAMESPACE", Type: "string", Required: true, Description: "Target namespace"},
		{Name: "POD_NAME", Type: "string", Required: true, Description: "Name of the pod to delete"},
	}
	// DD-WORKFLOW-017: SchemaImage references real OCI images at quay.io/kubernaut-cicd/test-workflows
	// Image names don't include the workflow version suffix (e.g., oomkill-increase-memory, not oomkill-increase-memory-v1)
	const aaWorkflowRegistry = "quay.io/kubernaut-cicd/test-workflows"
	testWorkflows := []TestWorkflow{
		{WorkflowID: "oomkill-increase-memory-v1", Name: "OOMKill Recovery - Increase Memory Limits", Description: "Increase memory limits for pods hitting OOMKill", SignalType: "OOMKilled", Severity: "critical", Component: "deployment", Environment: "staging", Priority: "P0", SchemaImage: aaWorkflowRegistry + "/oomkill-increase-memory:v1.0.0", SchemaParameters: oomkillParams},
		{WorkflowID: "oomkill-increase-memory-v1", Name: "OOMKill Recovery - Increase Memory Limits", Description: "Increase memory limits for pods hitting OOMKill", SignalType: "OOMKilled", Severity: "critical", Component: "deployment", Environment: "production", Priority: "P0", SchemaImage: aaWorkflowRegistry + "/oomkill-increase-memory:v1.0.0", SchemaParameters: oomkillParams},
		{WorkflowID: "oomkill-increase-memory-v1", Name: "OOMKill Recovery - Increase Memory Limits", Description: "Increase memory limits for pods hitting OOMKill", SignalType: "OOMKilled", Severity: "critical", Component: "deployment", Environment: "test", Priority: "P0", SchemaImage: aaWorkflowRegistry + "/oomkill-increase-memory:v1.0.0", SchemaParameters: oomkillParams},
		{WorkflowID: "crashloop-config-fix-v1", Name: "CrashLoopBackOff - Configuration Fix", Description: "Fix missing configuration causing CrashLoopBackOff", SignalType: "CrashLoopBackOff", Severity: "high", Component: "deployment", Environment: "staging", Priority: "P1", SchemaImage: aaWorkflowRegistry + "/crashloop-config-fix:v1.0.0", SchemaParameters: crashloopParams},
		{WorkflowID: "crashloop-config-fix-v1", Name: "CrashLoopBackOff - Configuration Fix", Description: "Fix missing configuration causing CrashLoopBackOff", SignalType: "CrashLoopBackOff", Severity: "high", Component: "deployment", Environment: "production", Priority: "P1", SchemaImage: aaWorkflowRegistry + "/crashloop-config-fix:v1.0.0", SchemaParameters: crashloopParams},
		{WorkflowID: "crashloop-config-fix-v1", Name: "CrashLoopBackOff - Configuration Fix", Description: "Fix missing configuration causing CrashLoopBackOff", SignalType: "CrashLoopBackOff", Severity: "high", Component: "deployment", Environment: "test", Priority: "P1", SchemaImage: aaWorkflowRegistry + "/crashloop-config-fix:v1.0.0", SchemaParameters: crashloopParams},
		{WorkflowID: "node-drain-reboot-v1", Name: "NodeNotReady - Drain and Reboot", Description: "Drain node and reboot to resolve NodeNotReady", SignalType: "NodeNotReady", Severity: "critical", Component: "node", Environment: "staging", Priority: "P0", SchemaImage: aaWorkflowRegistry + "/node-drain-reboot:v1.0.0", SchemaParameters: nodeDrainParams},
		{WorkflowID: "node-drain-reboot-v1", Name: "NodeNotReady - Drain and Reboot", Description: "Drain node and reboot to resolve NodeNotReady", SignalType: "NodeNotReady", Severity: "critical", Component: "node", Environment: "production", Priority: "P0", SchemaImage: aaWorkflowRegistry + "/node-drain-reboot:v1.0.0", SchemaParameters: nodeDrainParams},
		{WorkflowID: "node-drain-reboot-v1", Name: "NodeNotReady - Drain and Reboot", Description: "Drain node and reboot to resolve NodeNotReady", SignalType: "NodeNotReady", Severity: "critical", Component: "node", Environment: "test", Priority: "P0", SchemaImage: aaWorkflowRegistry + "/node-drain-reboot:v1.0.0", SchemaParameters: nodeDrainParams},
		{WorkflowID: "memory-optimize-v1", Name: "Memory Optimization - Alternative Approach", Description: "Optimize memory usage after failed scaling attempt", SignalType: "OOMKilled", Severity: "critical", Component: "deployment", Environment: "staging", Priority: "P0", SchemaImage: aaWorkflowRegistry + "/memory-optimize:v1.0.0", SchemaParameters: memOptimizeParams},
		{WorkflowID: "memory-optimize-v1", Name: "Memory Optimization - Alternative Approach", Description: "Optimize memory usage after failed scaling attempt", SignalType: "OOMKilled", Severity: "critical", Component: "deployment", Environment: "production", Priority: "P0", SchemaImage: aaWorkflowRegistry + "/memory-optimize:v1.0.0", SchemaParameters: memOptimizeParams},
		{WorkflowID: "memory-optimize-v1", Name: "Memory Optimization - Alternative Approach", Description: "Optimize memory usage after failed scaling attempt", SignalType: "OOMKilled", Severity: "critical", Component: "deployment", Environment: "test", Priority: "P0", SchemaImage: aaWorkflowRegistry + "/memory-optimize:v1.0.0", SchemaParameters: memOptimizeParams},
		{WorkflowID: "generic-restart-v1", Name: "Generic Pod Restart", Description: "Generic pod restart for unknown issues", SignalType: "Unknown", Severity: "medium", Component: "deployment", Environment: "staging", Priority: "P2", SchemaImage: aaWorkflowRegistry + "/generic-restart:v1.0.0", SchemaParameters: genericRestartParams},
		{WorkflowID: "generic-restart-v1", Name: "Generic Pod Restart", Description: "Generic pod restart for unknown issues", SignalType: "Unknown", Severity: "medium", Component: "deployment", Environment: "production", Priority: "P2", SchemaImage: aaWorkflowRegistry + "/generic-restart:v1.0.0", SchemaParameters: genericRestartParams},
		{WorkflowID: "generic-restart-v1", Name: "Generic Pod Restart", Description: "Generic pod restart for unknown issues", SignalType: "Unknown", Severity: "medium", Component: "deployment", Environment: "test", Priority: "P2", SchemaImage: aaWorkflowRegistry + "/generic-restart:v1.0.0", SchemaParameters: genericRestartParams},
		{WorkflowID: "test-signal-handler-v1", Name: "Test Signal Handler", Description: "Generic workflow for test signals (graceful shutdown tests)", SignalType: "TestSignal", Severity: "critical", Component: "pod", Environment: "staging", Priority: "P1", SchemaImage: aaWorkflowRegistry + "/test-signal-handler:v1.0.0", SchemaParameters: testSignalParams},
		{WorkflowID: "test-signal-handler-v1", Name: "Test Signal Handler", Description: "Generic workflow for test signals (graceful shutdown tests)", SignalType: "TestSignal", Severity: "critical", Component: "pod", Environment: "production", Priority: "P1", SchemaImage: aaWorkflowRegistry + "/test-signal-handler:v1.0.0", SchemaParameters: testSignalParams},
		{WorkflowID: "test-signal-handler-v1", Name: "Test Signal Handler", Description: "Generic workflow for test signals (graceful shutdown tests)", SignalType: "TestSignal", Severity: "critical", Component: "pod", Environment: "test", Priority: "P1", SchemaImage: aaWorkflowRegistry + "/test-signal-handler:v1.0.0", SchemaParameters: testSignalParams},
	}

	workflowUUIDs, err := SeedWorkflowsInDataStorage(seedClient, testWorkflows, "AIAnalysis E2E (via infrastructure)", writer)
	if err != nil {
		return fmt.Errorf("failed to seed test workflows: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "  ✅ Seeded %d workflows in DataStorage\n", len(workflowUUIDs))

	// NOTE: ConfigMap creation moved to deployMockLLMInNamespace() (Phase 7c)
	// This avoids duplication and ensures workflows are passed correctly

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 7c: Deploy remaining services IN PARALLEL (ConfigMap ready)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 7c: Deploying remaining services in parallel...")
	_, _ = fmt.Fprintln(writer, "  ├── Mock LLM service (with ConfigMap)")
	_, _ = fmt.Fprintln(writer, "  ├── HolmesGPT-API")
	_, _ = fmt.Fprintln(writer, "  └── AIAnalysis controller")
	_, _ = fmt.Fprintln(writer, "  ⏱️  Kubernetes will handle dependencies via readiness probes")

	type deployResult struct {
		name string
		err  error
	}

	deployResults := make(chan deployResult, 3)

	// Deploy Mock LLM service (HAPI dependency)
	// NOTE: Images already loaded in Phase 5-6, skip image loading in deployment
	go func() {
		err := deployMockLLMInNamespace(ctx, namespace, kubeconfigPath, builtImages["mock-llm"], workflowUUIDs, writer)
		deployResults <- deployResult{"Mock LLM", err}
	}()

	// Deploy HAPI (AIAnalysis dependency)
	// NOTE: Images already loaded in Phase 5-6, skip image loading in deployment
	go func() {
		err := deployHolmesGPTAPIManifestOnly(kubeconfigPath, builtImages["holmesgpt-api"], writer)
		deployResults <- deployResult{"HolmesGPT-API", err}
	}()

	// Deploy AIAnalysis controller (service under test)
	// NOTE: Images already loaded in Phase 5-6, skip image loading in deployment
	go func() {
		err := deployAIAnalysisControllerManifestOnly(kubeconfigPath, builtImages["aianalysis"], writer)
		deployResults <- deployResult{"AIAnalysis", err}
	}()

	// Collect deployment results (kubectl apply results)
	_, _ = fmt.Fprintln(writer, "\n⏳ Waiting for manifest applications...")
	for i := 0; i < 3; i++ {
		result := <-deployResults
		if result.err != nil {
			return fmt.Errorf("failed to deploy %s: %w", result.name, result.err)
		}
		_, _ = fmt.Fprintf(writer, "  ✅ %s deployed\n", result.name)
	}
	_, _ = fmt.Fprintln(writer, "✅ All services deployed! (Kubernetes reconciling...)")

	// Wait for ALL services to be ready (handles dependencies via readiness probes)
	// Per DD-TEST-002: Coverage-instrumented binaries take longer to start (2-5 min vs 30s)
	// Kubernetes reconciles dependencies:
	// - DataStorage waits for PostgreSQL + Redis (retry logic + readiness probe)
	// - HolmesGPT-API waits for PostgreSQL (retry logic + readiness probe)
	// - AIAnalysis waits for HAPI + DataStorage (retry logic + readiness probe)
	// This single wait point validates the entire dependency chain
	_, _ = fmt.Fprintln(writer, "\n⏳ Waiting for all services to be ready (Kubernetes reconciling dependencies)...")
	if err := waitForAllServicesReady(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("services not ready: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "✅ AIAnalysis E2E Infrastructure Ready (DD-TEST-002 + DD-TEST-008)")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	LogDiskSpace("FINAL", writer)
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	return nil
}

func DeleteAIAnalysisCluster(clusterName, kubeconfigPath string, testsFailed bool, writer io.Writer) error {
	// Use shared cleanup function with log export on failure
	if err := DeleteCluster(clusterName, "aianalysis", testsFailed, writer); err != nil {
		return err
	}

	// Remove kubeconfig
	if kubeconfigPath != "" {
		_ = os.Remove(kubeconfigPath)
	}

	return nil
}

func createAIAnalysisKindCluster(clusterName, kubeconfigPath string, writer io.Writer) error {
	// REFACTORED: Now uses shared CreateKindClusterWithConfig() helper
	// Authority: docs/handoff/TEST_INFRASTRUCTURE_REFACTORING_TRIAGE_JAN07.md (Phase 1)
	opts := KindClusterOptions{
		ClusterName:               clusterName,
		KubeconfigPath:            kubeconfigPath,
		ConfigPath:                "test/infrastructure/kind-aianalysis-config.yaml",
		WaitTimeout:               "60s",
		DeleteExisting:            false,
		ReuseExisting:             true, // Original behavior: reuse if exists
		CleanupOrphanedContainers: true, // Original behavior: cleanup Podman containers on macOS
		ProjectRootAsWorkingDir:   true, // DD-TEST-007: For ./coverdata resolution in Kind config
	}
	if err := CreateKindClusterWithConfig(opts, writer); err != nil {
		return err
	}

	// Wait for cluster to be ready (original behavior preserved)
	return waitForClusterReady(kubeconfigPath, writer)
}

func installAIAnalysisCRD(kubeconfigPath string, writer io.Writer) error {
	// Find CRD file
	crdPath := findCRDFile("kubernaut.ai_aianalyses.yaml")
	if crdPath == "" {
		return fmt.Errorf("AIAnalysis CRD not found")
	}

	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
		"apply", "-f", crdPath)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to apply CRD: %w", err)
	}

	// Wait for CRD to be established
	_, _ = fmt.Fprintln(writer, "  Waiting for CRD to be established...")
	for i := 0; i < 30; i++ {
		cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
			"get", "crd", "aianalyses.kubernaut.ai")
		if err := cmd.Run(); err == nil {
			return nil
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("timeout waiting for CRD")
}

// createMockLLMConfigMap creates a Kubernetes ConfigMap with workflow UUIDs for Mock LLM
// DD-TEST-011 Alt 2: ConfigMap Pattern
// - Test suite seeds workflows in DataStorage FIRST (captures actual UUIDs)
// - Creates ConfigMap with workflow_name → UUID mapping
// - Mock LLM reads ConfigMap at startup (no HTTP self-discovery needed)
// - Deterministic ordering, no timing issues
// Removed: createMockLLMConfigMap (unused) - Mock LLM now uses direct deployment with seeded workflows

func deployHolmesGPTAPIManifestOnly(kubeconfigPath, imageName string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "  Applying HolmesGPT-API manifest (image already in Kind)...")
	// ADR-030: Deploy manifest with ConfigMap
	manifest := fmt.Sprintf(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: holmesgpt-api-config
  namespace: kubernaut-system
data:
  config.yaml: |
    llm:
      provider: "openai"
      model: "mock-model"
      endpoint: "http://mock-llm:8080"
    data_storage:
      url: "http://data-storage-service:8080"  # DD-AUTH-011: Match Service name
    logging:
      level: "INFO"
    audit:
      flush_interval_seconds: 0.1
      buffer_size: 10000
      batch_size: 50
    auth:
      resource_name: "holmesgpt-api"  # Match actual Service name (DD-AUTH-014)
---
# ServiceAccount: HolmesGPT-API (DD-AUTH-014 middleware needs TokenReview permissions)
apiVersion: v1
kind: ServiceAccount
metadata:
  name: holmesgpt-api
  namespace: kubernaut-system
automountServiceAccountToken: true
---
# ClusterRole: HolmesGPT-API Middleware Permissions (DD-AUTH-014)
# Grants TokenReview permission so middleware can validate incoming Bearer tokens
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: holmesgpt-api-middleware
rules:
- apiGroups: ["authentication.k8s.io"]
  resources: ["tokenreviews"]
  verbs: ["create"]
- apiGroups: ["authorization.k8s.io"]
  resources: ["subjectaccessreviews"]
  verbs: ["create"]
---
# ClusterRoleBinding: Grant HolmesGPT-API SA middleware permissions
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: holmesgpt-api-middleware
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: holmesgpt-api-middleware
subjects:
- kind: ServiceAccount
  name: holmesgpt-api
  namespace: kubernaut-system
---
# RoleBinding: Grant HolmesGPT-API access to DataStorage for audit writes (DD-AUTH-014)
# Authority: DD-AUTH-014 (Middleware-based authentication) + BR-HAPI-197 (Audit trail)
# Required for: HAPI audit events → DataStorage REST API
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: holmesgpt-api-datastorage-access
  namespace: kubernaut-system
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: data-storage-client
subjects:
- kind: ServiceAccount
  name: holmesgpt-api
  namespace: kubernaut-system
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: holmesgpt-api
  namespace: kubernaut-system
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
      serviceAccountName: holmesgpt-api
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
        - name: MOCK_LLM_MODE
          value: "true"
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
        volumeMounts:
        - name: config
          mountPath: /etc/holmesgpt
          readOnly: true
      volumes:
      - name: config
        configMap:
          name: holmesgpt-api-config
---
apiVersion: v1
kind: Service
metadata:
  name: holmesgpt-api
  namespace: kubernaut-system
spec:
  type: NodePort
  selector:
    app: holmesgpt-api
  ports:
  - port: 8080
    targetPort: 8080
    nodePort: 30088
`, imageName, GetImagePullPolicy())
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

func deployAIAnalysisControllerManifestOnly(kubeconfigPath, imageName string, writer io.Writer) error {
	// Per Consolidated API Migration (January 2026):
	// Use dynamic image name parameter (built by BuildImageForKind)
	// Authority: docs/handoff/CONSOLIDATED_API_MIGRATION_GUIDE_JAN07.md
	_, _ = fmt.Fprintln(writer, "  Applying AIAnalysis controller manifest (image already in Kind)...")
	// Deploy controller with RBAC (extracted from deployAIAnalysisController)
	manifest := fmt.Sprintf(`
# ADR-030: AIAnalysis controller configuration (YAML ConfigMap)
# Per CRD_FIELD_NAMING_CONVENTION.md: camelCase for YAML fields
apiVersion: v1
kind: ConfigMap
metadata:
  name: aianalysis-config
  namespace: kubernaut-system
data:
  config.yaml: |
    controller:
      metricsAddr: ":9090"
      healthProbeAddr: ":8081"
      leaderElection: false
      leaderElectionId: "aianalysis.kubernaut.ai"
    holmesgpt:
      url: "http://holmesgpt-api:8080"
      timeout: "60s"
      sessionPollInterval: "2s"
    datastorage:
      url: "http://data-storage-service:8080"
      timeout: "10s"
      buffer:
        bufferSize: 20000
        batchSize: 1000
        flushInterval: "1s"
        maxRetries: 3
    rego:
      policyPath: "/etc/aianalysis/policies/approval.rego"
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: aianalysis-controller
  namespace: kubernaut-system
automountServiceAccountToken: true  # Kubernetes 1.24+ - ensure token is mounted
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: aianalysis-controller
rules:
- apiGroups: ["kubernaut.ai"]
  resources: ["aianalyses"]
  verbs: ["get", "list", "watch", "update", "patch"]
- apiGroups: ["kubernaut.ai"]
  resources: ["aianalyses/status"]
  verbs: ["get", "update", "patch"]
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: aianalysis-controller
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: aianalysis-controller
subjects:
- kind: ServiceAccount
  name: aianalysis-controller
  namespace: kubernaut-system
---
# ClusterRole: HolmesGPT API Client Access (DD-AUTH-014 middleware)
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: holmesgpt-api-client
rules:
- apiGroups: [""]
  resources: ["services"]
  resourceNames: ["holmesgpt-api"]
  verbs: ["create", "get"]  # BR-AA-HAPI-064: 'create' for POST (submit), 'get' for GET (session poll/result)
---
# RoleBinding: Grant AIAnalysis controller access to HolmesGPT API
# Required for DD-AUTH-014 middleware SubjectAccessReview check
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: aianalysis-controller-holmesgpt-access
  namespace: kubernaut-system
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: holmesgpt-api-client
subjects:
- kind: ServiceAccount
  name: aianalysis-controller
  namespace: kubernaut-system
---
# RoleBinding: Grant AIAnalysis controller access to DataStorage for audit writes (DD-AUTH-014)
# Authority: DD-AUTH-014 (Middleware-based authentication) + BR-AI-009 (Audit trail)
# Required for: AIAnalysis audit events → DataStorage REST API
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: aianalysis-controller-datastorage-access
  namespace: kubernaut-system
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: data-storage-client
subjects:
- kind: ServiceAccount
  name: aianalysis-controller
  namespace: kubernaut-system
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: aianalysis-controller
  namespace: kubernaut-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: aianalysis-controller
  template:
    metadata:
      labels:
        app: aianalysis-controller
    spec:
      serviceAccountName: aianalysis-controller
      containers:
      - name: aianalysis
        image: %s
        imagePullPolicy: %s
        ports:
        - containerPort: 8080
        - containerPort: 9090
        - containerPort: 8081
        readinessProbe:
          httpGet:
            path: /healthz
            port: 8081
          initialDelaySeconds: 30
          periodSeconds: 5
          timeoutSeconds: 3
          failureThreshold: 3
        # ADR-030: Single -config flag; all functional config in YAML ConfigMap
        env:
        - name: CONFIG_PATH
          value: /etc/aianalysis/config.yaml
        # DD-TEST-007: GOCOVERDIR for E2E binary coverage (added dynamically below)
        %s
        args:
        - "-config"
        - "$(CONFIG_PATH)"
        volumeMounts:
        - name: config
          mountPath: /etc/aianalysis
          readOnly: true
        - name: rego-policies
          mountPath: /etc/aianalysis/policies
          readOnly: true
        # DD-TEST-007: Coverage data mount (added dynamically below)
        %s
      volumes:
      - name: config
        configMap:
          name: aianalysis-config
      - name: rego-policies
        configMap:
          name: aianalysis-policies
      # DD-TEST-007: Coverage data volume (added dynamically below)
      %s
---
apiVersion: v1
kind: Service
metadata:
  name: aianalysis-controller
  namespace: kubernaut-system
spec:
  type: NodePort
  selector:
    app: aianalysis-controller
  ports:
  - name: api
    port: 8080
    targetPort: 8080
    nodePort: 30084
  - name: metrics
    port: 9090
    targetPort: 9090
    nodePort: 30184
  - name: health
    port: 8081
    targetPort: 8081
    nodePort: 30284
`, imageName, GetImagePullPolicy(),
		coverageEnvYAML("aianalysis"),
		coverageVolumeMountYAML(),
		coverageVolumeYAML())
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return err
	}

	// Deploy Rego policy ConfigMap
	return deployRegoPolicyConfigMap(kubeconfigPath, writer)
}

func waitForAllServicesReady(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
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

	// Track polling attempts for debugging
	pollCount := 0
	maxPolls := int((2 * time.Minute) / (5 * time.Second)) // 24 polls expected

	Eventually(func() bool {
		pollCount++
		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app=holmesgpt-api",
		})
		if err != nil {
			_, _ = fmt.Fprintf(writer, "      [Poll %d/%d] Error listing HAPI pods: %v\n", pollCount, maxPolls, err)
			return false
		}
		if len(pods.Items) == 0 {
			_, _ = fmt.Fprintf(writer, "      [Poll %d/%d] No HAPI pods found\n", pollCount, maxPolls)
			return false
		}

		// Debug: Show pod status every 4 polls (~20 seconds)
		for _, pod := range pods.Items {
			if pollCount%4 == 0 {
				_, _ = fmt.Fprintf(writer, "      [Poll %d/%d] HAPI pod '%s': Phase=%s, Ready=",
					pollCount, maxPolls, pod.Name, pod.Status.Phase)
				isReady := false
				for _, condition := range pod.Status.Conditions {
					if condition.Type == corev1.PodReady {
						_, _ = fmt.Fprintf(writer, "%s", condition.Status)
						if condition.Status != corev1.ConditionTrue {
							_, _ = fmt.Fprintf(writer, " (Reason: %s, Message: %s)", condition.Reason, condition.Message)
						}
						isReady = condition.Status == corev1.ConditionTrue
						break
					}
				}
				if !isReady {
					// Show container statuses for debugging
					for _, containerStatus := range pod.Status.ContainerStatuses {
						if !containerStatus.Ready {
							_, _ = fmt.Fprintf(writer, "\n         Container '%s': Ready=%t, RestartCount=%d",
								containerStatus.Name, containerStatus.Ready, containerStatus.RestartCount)
							if containerStatus.State.Waiting != nil {
								_, _ = fmt.Fprintf(writer, ", Waiting: %s (%s)",
									containerStatus.State.Waiting.Reason, containerStatus.State.Waiting.Message)
							}
							if containerStatus.State.Terminated != nil {
								_, _ = fmt.Fprintf(writer, ", Terminated: ExitCode=%d, Reason=%s",
									containerStatus.State.Terminated.ExitCode, containerStatus.State.Terminated.Reason)
							}
						}
					}
				}
				_, _ = fmt.Fprintf(writer, "\n")
			}

			if pod.Status.Phase == corev1.PodRunning {
				for _, condition := range pod.Status.Conditions {
					if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
						return true
					}
				}
			}
		}
		return false
	}, 2*time.Minute, 5*time.Second).Should(BeTrue(), "HolmesGPT-API pod should become ready")
	_, _ = fmt.Fprintf(writer, "   ✅ HolmesGPT-API ready\n")

	// Wait for AIAnalysis controller pod to be ready
	// Note: Coverage-instrumented binaries may take longer to start
	_, _ = fmt.Fprintf(writer, "   ⏳ Waiting for AIAnalysis controller pod to be ready...\n")
	Eventually(func() bool {
		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app=aianalysis-controller",
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
	}, 5*time.Minute, 5*time.Second).Should(BeTrue(), "AIAnalysis controller pod should become ready")
	_, _ = fmt.Fprintf(writer, "   ✅ AIAnalysis controller ready\n")

	return nil
}
func waitForClusterReady(kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "  Waiting for cluster to be ready...")
	for i := 0; i < 60; i++ {
		cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
			"get", "nodes", "-o", "jsonpath={.items[*].status.conditions[?(@.type=='Ready')].status}")
		output, err := cmd.Output()
		if err == nil && containsReady(string(output)) {
			_, _ = fmt.Fprintln(writer, "  Cluster nodes ready")
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timeout waiting for cluster")
}

func findCRDFile(name string) string {
	// Try to find via runtime caller location first
	_, currentFile, _, ok := runtime.Caller(0)
	if ok {
		// Go up to project root (from test/infrastructure/)
		projectRoot := filepath.Dir(filepath.Dir(filepath.Dir(currentFile)))
		crdPath := filepath.Join(projectRoot, "config/crd/bases", name)
		if _, err := os.Stat(crdPath); err == nil {
			return crdPath
		}
	}

	candidates := []string{
		"config/crd/bases/" + name,
		"../config/crd/bases/" + name,
		"../../config/crd/bases/" + name,
		"../../../config/crd/bases/" + name,
		"config/crd/" + name,
		"../config/crd/" + name,
		"../../config/crd/" + name,
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			absPath, _ := filepath.Abs(path)
			return absPath
		}
	}
	return ""
}

func deployRegoPolicyConfigMap(kubeconfigPath string, writer io.Writer) error {
	policyPath := findRegoPolicy()
	if policyPath == "" {
		// Use inline policy
		return createInlineRegoPolicyConfigMap(kubeconfigPath, writer)
	}

	// Create ConfigMap from file
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
		"create", "configmap", "aianalysis-policies",
		"--from-file=approval.rego="+policyPath,
		"-n", "kubernaut-system",
		"--dry-run=client", "-o", "yaml")

	applyCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")

	// Use io.Pipe to connect stdout of cmd to stdin of applyCmd
	pipeReader2, pipeWriter2 := io.Pipe()
	cmd.Stdout = pipeWriter2
	applyCmd.Stdin = pipeReader2
	applyCmd.Stdout = writer
	applyCmd.Stderr = writer

	if err := cmd.Start(); err != nil {
		return err
	}
	if err := applyCmd.Start(); err != nil {
		return err
	}
	if err := cmd.Wait(); err != nil {
		return err
	}
	_ = pipeWriter2.Close()
	return applyCmd.Wait()
}
func containsReady(s string) bool {
	return len(s) > 0 && s != "" && (s == "True" || s == "True True")
}

func findRegoPolicy() string {
	// Try to find via runtime caller location first
	_, currentFile, _, ok := runtime.Caller(0)
	if ok {
		// Go up to project root (from test/infrastructure/)
		projectRoot := filepath.Dir(filepath.Dir(filepath.Dir(currentFile)))
		policyPath := filepath.Join(projectRoot, "config/rego/aianalysis/approval.rego")
		if _, err := os.Stat(policyPath); err == nil {
			return policyPath
		}
	}

	candidates := []string{
		"config/rego/aianalysis/approval.rego",
		"../config/rego/aianalysis/approval.rego",
		"../../config/rego/aianalysis/approval.rego",
		"../../../config/rego/aianalysis/approval.rego",
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			absPath, _ := filepath.Abs(path)
			return absPath
		}
	}
	return ""
}

func createInlineRegoPolicyConfigMap(kubeconfigPath string, writer io.Writer) error {
	// Simplified E2E test policy - requires approval for all production
	// This is intentionally simpler than production policy for E2E test predictability
	manifest := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: aianalysis-policies
  namespace: kubernaut-system
data:
  approval.rego: |
    package aianalysis.approval
    import rego.v1

    default require_approval := false

    require_approval if { input.environment == "production" }
    require_approval if { input.is_recovery_attempt == true; input.recovery_attempt_number >= 3 }
    require_approval if { input.environment == "production"; count(input.warnings) > 0 }
    require_approval if { input.environment == "production"; count(input.failed_detections) > 0 }

    # Scored risk factors for reason generation (issue #98)
    risk_factors contains {"score": 100, "reason": msg} if {
        input.is_recovery_attempt == true
        input.recovery_attempt_number >= 3
        msg := sprintf("Multiple recovery attempts (%d) - human approval required", [input.recovery_attempt_number])
    }
    risk_factors contains {"score": 70, "reason": "Data quality warnings in production environment"} if {
        input.environment == "production"
        count(input.warnings) > 0
    }
    risk_factors contains {"score": 60, "reason": "Data quality issues detected in production environment"} if {
        input.environment == "production"
        count(input.failed_detections) > 0
    }
    risk_factors contains {"score": 40, "reason": "Production environment requires manual approval"} if {
        input.environment == "production"
    }

    all_scores contains f.score if { some f in risk_factors }
    max_risk_score := max(all_scores) if { count(all_scores) > 0 }
    reason := f.reason if { some f in risk_factors; f.score == max_risk_score }
    default reason := "Auto-approved"
`
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

// createAIAnalysisE2EServiceAccount creates the ServiceAccount for workflow seeding
// with DataStorage access using DD-AUTH-014 (SubjectAccessReview authorization)
func createAIAnalysisE2EServiceAccount(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	// Create a fresh context (workaround for potential context issues)
	freshCtx := context.Background()

	// Create ServiceAccount
	saName := "aianalysis-e2e-sa"
	if err := CreateServiceAccount(freshCtx, namespace, kubeconfigPath, saName, writer); err != nil {
		return fmt.Errorf("failed to create ServiceAccount: %w", err)
	}

	// Bind to data-storage-client ClusterRole (already deployed in Phase 6.5)
	// This ClusterRole has SubjectAccessReview permissions for DataStorage access
	roleBindingYAML := fmt.Sprintf(`apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: aianalysis-e2e-datastorage-access
  namespace: %s
subjects:
- kind: ServiceAccount
  name: %s
  namespace: %s
roleRef:
  kind: ClusterRole
  name: data-storage-client
  apiGroup: rbac.authorization.k8s.io
`, namespace, saName, namespace)

	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(roleBindingYAML)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create RoleBinding: %w", err)
	}

	return nil
}

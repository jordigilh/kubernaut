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

// Package aianalysis contains integration tests for the AIAnalysis controller.
// These tests verify the complete reconciliation loop with real Kubernetes API.
//
// Business Requirements:
// - BR-AI-001: AI Analysis CRD lifecycle management
// - BR-AI-002: HolmesGPT-API integration
// - BR-AI-003: Rego policy evaluation
//
// Test Strategy: Two integration test categories:
// 1. **Envtest-only tests** (this file): Use mock HAPI for fast controller testing
// 2. **Real service tests** (recovery_integration_test.go): Use real HAPI (auto-started)
//
// Defense-in-Depth (per 03-testing-strategy.mdc):
// - Unit tests (70%+): Mock K8s client + mock HAPI
// - Integration tests (>50%): Real K8s API (envtest) + mock/real HAPI
// - E2E tests (10-15%): Real K8s API (KIND) + real HAPI
//
// DD-TEST-010: Multi-Controller Architecture (Controller-Per-Process Pattern)
// Infrastructure (AUTO-STARTED in Phase 1, process 1 only):
// - PostgreSQL (port 15438): Persistence layer
// - Redis (port 16384): Caching layer
// - Data Storage API (port 18095): Audit trail
// - Mock LLM Service (port 18141): Standalone OpenAI-compatible mock (AIAnalysis-specific)
// - HolmesGPT API (port 18120): AI analysis service (uses Mock LLM at 18141)
//
// Per-Process Setup (Phase 2, all processes):
// - envtest: In-memory Kubernetes API server (per process)
// - Controller Manager: AIAnalysis reconciler (per process)
// - Handlers: Investigating, Analyzing (per process)
// - Metrics: Isolated Prometheus registry (per process)
// - Audit Store: Buffered audit client (per process)
package aianalysis

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	aianalysisv1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/controller/aianalysis"
	aiaudit "github.com/jordigilh/kubernaut/pkg/aianalysis/audit"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/rego"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/status"
	"github.com/jordigilh/kubernaut/pkg/audit"
	hgclient "github.com/jordigilh/kubernaut/pkg/holmesgpt/client"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
	"github.com/jordigilh/kubernaut/test/shared/integration"
)

// DD-TEST-010: Per-process variables (no shared state between processes)
var (
	ctx        context.Context
	cancel     context.CancelFunc
	testEnv    *envtest.Environment
	k8sClient  client.Client
	k8sManager ctrl.Manager
	auditStore audit.AuditStore

	// DD-AUTH-014: Authenticated DataStorage clients (audit + OpenAPI with ServiceAccount tokens)
	dsClients *integration.AuthenticatedDataStorageClients

	// DD-AUTH-014: ServiceAccount token for creating authenticated clients
	serviceAccountToken string

	// Per-process HAPI client (each process gets its own)
	realHGClient *hgclient.HolmesGPTClient

	// Per-process Rego evaluator
	realRegoEvaluator *rego.Evaluator
	regoCtx           context.Context
	regoCancel        context.CancelFunc

	// DD-TEST-002: Unique namespace per test for parallel execution
	testNamespace string

	// DD-METRICS-001: Per-process isolated Prometheus registry
	testRegistry *prometheus.Registry
	testMetrics  *metrics.Metrics

	// DD-TEST-010: Per-process reconciler instance for metrics access
	// WorkflowExecution pattern: Store reconciler to access metrics directly
	reconciler *aianalysis.AIAnalysisReconciler

	// DD-TEST-010: Track infrastructure for cleanup (shared reference)
	dsInfra *infrastructure.DSBootstrapInfra

	// Shared infrastructure for cleanup (SynchronizedAfterSuite second function)
	sharedTestEnv     *envtest.Environment
	sharedCfg         *rest.Config
	hapiContainer     *infrastructure.ContainerInstance
	mockLLMConfig     infrastructure.MockLLMConfig
	mockLLMConfigPath string
	hapiSATokenDir    string

	// DD-WORKFLOW-002 v3.0: Workflow UUID mapping for test assertions
	// Map format: "workflow_name:environment" → "actual-uuid-from-datastorage"
	// Example: "oomkill-increase-memory-v1:production" → "02fad812-0ad1-4da6-b3bb-cc322a1fda47"
	workflowUUIDs map[string]string
)

func TestAIAnalysisIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AIAnalysis Controller Integration Suite (Envtest)")
}

// DD-TEST-010: Multi-Controller Architecture
// Phase 1: Infrastructure ONLY (Process 1 ONLY)
// Phase 2: Per-Process Controller Environment (ALL Processes)
//
// TIMEOUT NOTE: Infrastructure startup takes ~70-90 seconds locally, but up to 3+ minutes in CI.
// CI environments (GitHub Actions) have slower container startup times, especially HAPI.
// Default Ginkgo timeout (60s) is insufficient, causing INTERRUPTED in parallel mode.
// NodeTimeout(5*time.Minute) ensures sufficient time for complete infrastructure startup in CI.
var _ = SynchronizedBeforeSuite(NodeTimeout(10*time.Minute), func(specCtx SpecContext) []byte {
	// ═══════════════════════════════════════════════════════════════════════════════
	// Phase 1: Infrastructure ONLY (Process 1 ONLY)
	// ═══════════════════════════════════════════════════════════════════════════════
	// Per DD-TEST-010: Phase 1 starts ONLY shared infrastructure containers
	// NO envtest, NO controller, NO metrics - these are created per-process in Phase 2
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	GinkgoWriter.Println("AIAnalysis Integration - DD-TEST-010 + DD-AUTH-014")
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	GinkgoWriter.Println("Phase 1: Infrastructure Startup (process 1 only)")
	GinkgoWriter.Println("  • Shared envtest (for DataStorage auth)")
	GinkgoWriter.Println("  • PostgreSQL (port 15438)")
	GinkgoWriter.Println("  • Redis (port 16384)")
	GinkgoWriter.Println("  • Data Storage API (port 18095)")
	GinkgoWriter.Println("  • Mock LLM Service (port 18141 - AIAnalysis-specific)")
	GinkgoWriter.Println("  • HolmesGPT-API HTTP service (port 18120, uses Mock LLM)")
	GinkgoWriter.Println("")
	GinkgoWriter.Println("Phase 2 will create PER-PROCESS (all processes):")
	GinkgoWriter.Println("  • envtest (in-memory K8s API server)")
	GinkgoWriter.Println("  • Controller manager + AIAnalysis reconciler")
	GinkgoWriter.Println("  • Handlers, metrics, audit store")
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// DD-AUTH-014: Start shared envtest FIRST (before building images)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	By("Starting shared envtest for DataStorage authentication (DD-AUTH-014)")

	// Force envtest to bind to IPv4 (critical for macOS!)
	_ = os.Setenv("KUBEBUILDER_CONTROLPLANE_START_TIMEOUT", "60s") // Explicitly ignore - test setup

	sharedTestEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
		ControlPlane: envtest.ControlPlane{
			APIServer: &envtest.APIServer{
				SecureServing: envtest.SecureServing{
					ListenAddr: envtest.ListenAddr{
						Address: "127.0.0.1", // Force IPv4 binding (DD-TEST-012)
					},
				},
			},
		},
	}
	var err error
	sharedCfg, err = sharedTestEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(sharedCfg).NotTo(BeNil())

	GinkgoWriter.Printf("✅ Shared envtest started\n")
	GinkgoWriter.Printf("   📍 envtest URL: %s\n", sharedCfg.Host)

	// NOTE: Cleanup moved to SynchronizedAfterSuite (cannot use DeferCleanup in first function)

	// Create ServiceAccount + RBAC for DataStorage access
	// This creates:
	// - aianalysis-ds-client ServiceAccount (for AIAnalysis controller to call DataStorage)
	// - datastorage-service ServiceAccount (for DataStorage to validate tokens)
	By("Creating ServiceAccount with DataStorage RBAC in shared envtest")
	authConfig, err := infrastructure.CreateIntegrationServiceAccountWithDataStorageAccess(
		sharedCfg,
		"aianalysis-ds-client",
		"default",
		GinkgoWriter,
	)
	Expect(err).ToNot(HaveOccurred())
	GinkgoWriter.Println("✅ ServiceAccount + RBAC created for AIAnalysis → DataStorage")

	// DD-AUTH-014: Grant AIAnalysis controller SA permission to call HAPI
	// In production: holmesgpt-api-client ClusterRole grants `get` verb on holmesgpt-api resource
	// In integration: Create similar RBAC for envtest
	By("Granting AIAnalysis controller SA permission to call HAPI")
	k8sClient, err := client.New(sharedCfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())

	hapiClientRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "holmesgpt-api-client",
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups:     []string{""},
				Resources:     []string{"services"},
				ResourceNames: []string{"holmesgpt-api"}, // Must match HAPI middleware config (main.py)
				Verbs:         []string{"create", "get"},  // BR-AA-HAPI-064: "create" for POST, "get" for session poll/result
			},
		},
	}
	err = k8sClient.Create(context.Background(), hapiClientRole)
	if !apierrors.IsAlreadyExists(err) {
		Expect(err).ToNot(HaveOccurred())
	}

	hapiClientBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "aianalysis-holmesgpt-api-client",
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "holmesgpt-api-client",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "aianalysis-ds-client",
				Namespace: "default",
			},
		},
	}
	err = k8sClient.Create(context.Background(), hapiClientBinding)
	if !apierrors.IsAlreadyExists(err) {
		Expect(err).ToNot(HaveOccurred())
	}
	GinkgoWriter.Println("✅ AIAnalysis controller granted HAPI access permissions")

	// DD-AUTH-014: Create ServiceAccount for HAPI service (for TokenReview/SAR validation)
	// HAPI is an HTTP server (like DataStorage) that validates incoming Bearer tokens
	// Platform-specific: Linux uses host network, macOS uses bridge network
	By("Creating ServiceAccount for HAPI service with TokenReview/SAR permissions")
	useHostNetworkForHAPI := runtime.GOOS == "linux"
	hapiServiceAuthConfig, err := infrastructure.CreateServiceAccountForHTTPService(
		sharedCfg,
		"holmesgpt-service",
		"default",
		useHostNetworkForHAPI, // Linux: host network (127.0.0.1), macOS: bridge network (host.containers.internal)
		GinkgoWriter,
	)
	Expect(err).ToNot(HaveOccurred())
	GinkgoWriter.Println("✅ ServiceAccount + RBAC created for HAPI → envtest (TokenReview/SAR)")

	// DD-AUTH-014: Grant HAPI ServiceAccount permission to write audit events to DataStorage
	// HAPI needs BOTH:
	//   1. TokenReview/SAR permissions (to validate incoming requests) - ✅ Already has
	//   2. DataStorage client permissions (to write audit events) - Add now
	By("Granting HAPI ServiceAccount permission to write audit events to DataStorage")
	hapiDSClientBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "holmesgpt-service-datastorage-client",
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "data-storage-client", // Reuse existing ClusterRole (created by CreateIntegrationServiceAccountWithDataStorageAccess)
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "holmesgpt-service",
				Namespace: "default",
			},
		},
	}
	err = k8sClient.Create(context.Background(), hapiDSClientBinding)
	if !apierrors.IsAlreadyExists(err) {
		Expect(err).ToNot(HaveOccurred())
	}
	GinkgoWriter.Println("✅ HAPI ServiceAccount granted DataStorage write permissions")

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// OPTIMIZATION: Build images in parallel (saves ~100 seconds)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	By("Building DataStorage, Mock LLM, and HAPI images in parallel")
	var (
		dsImageName      string
		mockLLMImageName string
		hapiImageName    string
		dsErr            error
		mockErr          error
		hapiErr          error
		wg               sync.WaitGroup
	)

	wg.Add(3)

	// Goroutine 1: Build DataStorage image (~60s)
	go func() {
		defer wg.Done()
		defer GinkgoRecover() // Ensure Ginkgo failures are captured
		dsImageName, dsErr = infrastructure.BuildDataStorageImage(specCtx, "aianalysis", GinkgoWriter)
	}()

	// Goroutine 2: Build Mock LLM image (~40s)
	go func() {
		defer wg.Done()
		defer GinkgoRecover() // Ensure Ginkgo failures are captured
		mockLLMImageName, mockErr = infrastructure.BuildMockLLMImage(specCtx, "aianalysis", GinkgoWriter)
	}()

	// Goroutine 3: Build HAPI image (~100s)
	go func() {
		defer wg.Done()
		defer GinkgoRecover() // Ensure Ginkgo failures are captured
		hapiImageName, hapiErr = infrastructure.BuildHAPIImage(specCtx, "aianalysis", GinkgoWriter)
	}()

	wg.Wait() // Wait for all three builds to complete

	// Check for errors after parallel builds
	Expect(dsErr).ToNot(HaveOccurred(), "DataStorage image must build successfully")
	Expect(mockErr).ToNot(HaveOccurred(), "Mock LLM image must build successfully")
	Expect(hapiErr).ToNot(HaveOccurred(), "HAPI image must build successfully")
	GinkgoWriter.Printf("✅ All three images built in parallel: DS=%s, MockLLM=%s, HAPI=%s\n", dsImageName, mockLLMImageName, hapiImageName)

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// SEQUENTIAL DEPLOYMENT: Start DataStorage, seed workflows, start Mock LLM
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	By("Starting AIAnalysis integration infrastructure (PostgreSQL, Redis, DataStorage)")
	// Per DD-TEST-001 v2.2: PostgreSQL=15438, Redis=16384, DS=18095
	// DD-AUTH-014: Helper function ensures auth is properly configured
	cfg := infrastructure.NewDSBootstrapConfigWithAuth(
		"aianalysis",
		15438, 16384, 18095, 19095,
		"test/integration/aianalysis/config",
		authConfig,
	)
	dsInfra, err = infrastructure.StartDSBootstrap(cfg, GinkgoWriter)
	Expect(err).ToNot(HaveOccurred(), "Infrastructure must start successfully")
	GinkgoWriter.Println("✅ DataStorage infrastructure started (PostgreSQL, Redis, DataStorage)")

	// NOTE: Cleanup moved to SynchronizedAfterSuite (cannot use DeferCleanup in first function)

	// DD-AUTH-014: Create authenticated DataStorage client for workflow seeding
	// Pattern: Use same helper as HAPI integration tests (matches working pattern)
	By("Creating authenticated DataStorage client for workflow seeding")
	dataStorageURL := "http://127.0.0.1:18095" // AIAnalysis integration test DS port
	seedClient := integration.NewAuthenticatedDataStorageClients(
		dataStorageURL,
		authConfig.Token,
		30*time.Second,
	)
	GinkgoWriter.Println("✅ Authenticated DataStorage client created for workflow seeding")

	// Seed test workflows into DataStorage BEFORE starting Mock LLM
	// Pattern: DD-TEST-011 v2.0 - File-Based Configuration
	// Must seed workflows first so Mock LLM can load UUIDs at startup
	// DD-AUTH-014: Now uses authenticated client (matches HAPI pattern)
	By("Seeding test workflows into DataStorage (with authentication)")
	workflowUUIDs, err := SeedTestWorkflowsInDataStorage(seedClient.OpenAPIClient, GinkgoWriter)
	Expect(err).ToNot(HaveOccurred(), "Test workflows must be seeded successfully")

	// Write Mock LLM config file with workflow UUIDs
	// Pattern: DD-TEST-011 v2.0 - File-Based Configuration
	// Mock LLM will read this file at startup (no HTTP calls required)
	By("Writing Mock LLM configuration file with workflow UUIDs")
	// Use absolute path in test directory (not /tmp which may be cleared)
	mockLLMConfigPath, err = filepath.Abs("mock-llm-config.yaml")
	Expect(err).ToNot(HaveOccurred(), "Must get absolute path for config file")
	err = WriteMockLLMConfigFile(mockLLMConfigPath, workflowUUIDs, GinkgoWriter)
	Expect(err).ToNot(HaveOccurred(), "Mock LLM config file must be written successfully")

	// NOTE: Cleanup moved to SynchronizedAfterSuite (cannot use DeferCleanup in first function)

	By("Starting Mock LLM service with configuration file (DD-TEST-011 v2.0)")
	// Per DD-TEST-001 v2.3: Port 18141 (AIAnalysis-specific, unique from HAPI's 18140)
	// Per MOCK_LLM_MIGRATION_PLAN.md v1.3.0: Standalone service for test isolation
	mockLLMConfig = infrastructure.GetMockLLMConfigForAIAnalysis()
	mockLLMConfig.ImageTag = mockLLMImageName        // Use the built image tag
	mockLLMConfig.ConfigFilePath = mockLLMConfigPath // DD-TEST-011 v2.0: Mount config file
	// DD-AUTH-014: Platform-specific network (must match HAPI's network mode)
	if runtime.GOOS == "linux" {
		mockLLMConfig.Network = "host" // Linux CI: Host network (HAPI will reach via 127.0.0.1)
	} else {
		mockLLMConfig.Network = "aianalysis_test_network" // macOS: Bridge network with container-to-container DNS
	}
	mockLLMContainerID, err := infrastructure.StartMockLLMContainer(specCtx, mockLLMConfig, GinkgoWriter)
	Expect(err).ToNot(HaveOccurred(), "Mock LLM container must start successfully")
	Expect(mockLLMContainerID).ToNot(BeEmpty(), "Mock LLM container ID must be returned")
	GinkgoWriter.Printf("✅ Mock LLM service started with config file (port %d)\n", mockLLMConfig.Port)

	// NOTE: Cleanup moved to SynchronizedAfterSuite (cannot use DeferCleanup in first function)

	By("Starting HolmesGPT-API HTTP service (using pre-built image)")
	// AA integration tests use OpenAPI HAPI client (HTTP-based)
	// DD-TEST-001 v2.3: HAPI port 18120, Mock LLM port 18141
	// OPTIMIZATION: Use pre-built image from parallel build (no BuildContext/BuildDockerfile)
	hapiConfigDir, err := filepath.Abs("hapi-config")
	Expect(err).ToNot(HaveOccurred(), "Failed to get absolute path for hapi-config")

	// DD-AUTH-014: Create ServiceAccount secrets directory structure for HAPI container
	// HAPI's ServiceAccountAuthPoolManager expects token at /var/run/secrets/kubernetes.io/serviceaccount/token
	// CRITICAL: Use HAPI's own ServiceAccount token (hapiServiceAuthConfig.Token), NOT the client token!
	// This token identifies HAPI service when calling DataStorage
	hapiSATokenDir = filepath.Join(os.TempDir(), fmt.Sprintf("aianalysis-hapi-sa-secrets-%d", time.Now().UnixNano()))
	err = os.MkdirAll(hapiSATokenDir, 0755)
	Expect(err).ToNot(HaveOccurred(), "Failed to create HAPI ServiceAccount secrets directory")
	hapiSATokenFilePath := filepath.Join(hapiSATokenDir, "token")
	err = os.WriteFile(hapiSATokenFilePath, []byte(hapiServiceAuthConfig.Token), 0644)
	Expect(err).ToNot(HaveOccurred(), "Failed to write HAPI ServiceAccount token to file")
	GinkgoWriter.Printf("✅ HAPI ServiceAccount token written to: %s (for DataStorage auth)\n", hapiSATokenFilePath)

	// NOTE: Cleanup moved to SynchronizedAfterSuite (cannot use DeferCleanup in first function)

	// DD-AUTH-014: Platform-specific network configuration for HAPI
	// Linux CI: Use host network (can reach envtest on 127.0.0.1 directly)
	// macOS Dev: Use bridge network with host.containers.internal rewriting
	useHostNetwork := runtime.GOOS == "linux"

	hapiConfig := infrastructure.GenericContainerConfig{
		Name:  "aianalysis_hapi_test",
		Image: hapiImageName, // Use pre-built image from parallel build
		Env: map[string]string{
			"LLM_MODEL":      "mock-model",
			"LLM_PROVIDER":   "openai",                             // Required by litellm
			"OPENAI_API_KEY": "mock-api-key-for-integration-tests", // Required by litellm even for mock endpoints
			"API_PORT":       "18120",                              // HAPI uses API_PORT, not PORT (see entrypoint.sh)
			"LOG_LEVEL":      "DEBUG",
			"KUBECONFIG":     "/tmp/kubeconfig", // DD-AUTH-014: Real K8s auth with envtest (file-based kubeconfig)
			"POD_NAMESPACE":  "default",         // Required for K8s client
		},
		Volumes: map[string]string{
			hapiConfigDir:                        "/etc/holmesgpt:ro",                                // Mount HAPI config directory
			hapiServiceAuthConfig.KubeconfigPath: "/tmp/kubeconfig:ro",                               // DD-AUTH-014: Mount envtest kubeconfig (real auth!)
			hapiSATokenDir:                       "/var/run/secrets/kubernetes.io/serviceaccount:ro", // DD-AUTH-014: Mount HAPI ServiceAccount secrets (HAPI's own token for DataStorage auth)
		},
		// BuildContext and BuildDockerfile removed - image already built in parallel
		HealthCheck: &infrastructure.HealthCheckConfig{
			URL:     "http://127.0.0.1:18120/health",
			Timeout: 120 * time.Second, // Reduced: only startup time (no build), ~20-30s expected
		},
	}

	if useHostNetwork {
		// Linux CI: Host network mode (can reach localhost directly)
		hapiConfig.Network = "host"
		hapiConfig.Env["LLM_ENDPOINT"] = fmt.Sprintf("http://127.0.0.1:%d", mockLLMConfig.Port) // Mock LLM also on host network (AIAnalysis port: 18141)
		hapiConfig.Env["DATA_STORAGE_URL"] = "http://127.0.0.1:18095"
		GinkgoWriter.Printf("   🌐 HAPI using host network (Linux CI) - localhost access enabled\n")
	} else {
		// macOS Dev: Bridge network with host.containers.internal
		hapiConfig.Network = "aianalysis_test_network"
		hapiConfig.Ports = map[int]int{18120: 18120}                                               // container:host (both use 18120 now)
		hapiConfig.Env["LLM_ENDPOINT"] = infrastructure.GetMockLLMContainerEndpoint(mockLLMConfig) // http://mock-llm-aianalysis:8080 (container-to-container)
		hapiConfig.Env["DATA_STORAGE_URL"] = "http://host.containers.internal:18095"
		hapiConfig.ExtraHosts = []string{
			"host.containers.internal:host-gateway", // macOS: Explicit host resolution (redundant but harmless)
		}
		GinkgoWriter.Printf("   🌐 HAPI using bridge network (macOS) - host.containers.internal routing enabled\n")
	}
	hapiContainer, err = infrastructure.StartGenericContainer(hapiConfig, GinkgoWriter)
	Expect(err).ToNot(HaveOccurred(), "HAPI container must start successfully")
	GinkgoWriter.Printf("✅ HolmesGPT-API started at http://127.0.0.1:18120 (container: %s)\n", hapiContainer.ID)
	GinkgoWriter.Printf("   Using Mock LLM at %s\n", infrastructure.GetMockLLMEndpoint(mockLLMConfig))

	// NOTE: Cleanup moved to SynchronizedAfterSuite (cannot use DeferCleanup in first function)

	GinkgoWriter.Println("✅ Infrastructure startup complete (Phase 1)")
	GinkgoWriter.Println("  Phase 2 will now run on ALL processes (per-process controller setup)")
	GinkgoWriter.Println("")

	// DD-AUTH-014 + DD-TEST-010: Phase 1 → Phase 2 data passing
	// Serialize BOTH token and workflowUUIDs for ALL processes
	type Phase1Data struct {
		Token         string            `json:"token"`
		WorkflowUUIDs map[string]string `json:"workflow_uuids"`
	}
	phase1Data := Phase1Data{
		Token:         authConfig.Token,
		WorkflowUUIDs: workflowUUIDs,
	}
	phase1DataJSON, err := json.Marshal(phase1Data)
	Expect(err).ToNot(HaveOccurred(), "Phase 1 data must serialize for Phase 2")
	return phase1DataJSON
}, func(specCtx SpecContext, data []byte) {
	// ═══════════════════════════════════════════════════════════════════════════════
	// Phase 2: Per-Process Controller Environment (ALL Processes)
	// ═══════════════════════════════════════════════════════════════════════════════
	// Per DD-TEST-010: Each process gets its own controller, envtest, metrics, etc.
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	// DD-AUTH-014 + DD-TEST-010: Deserialize token and workflow UUIDs from Phase 1
	type Phase1Data struct {
		Token         string            `json:"token"`
		WorkflowUUIDs map[string]string `json:"workflow_uuids"`
	}
	var phase1Data Phase1Data
	deserializeErr := json.Unmarshal(data, &phase1Data)
	Expect(deserializeErr).ToNot(HaveOccurred(), "Phase 1 data must deserialize successfully")

	// Extract values
	token := phase1Data.Token
	workflowUUIDs = phase1Data.WorkflowUUIDs

	if token == "" {
		Fail("ServiceAccount token from Phase 1 is empty")
	}

	// DD-AUTH-014: Store token globally for tests that need to create custom authenticated clients
	serviceAccountToken = token

	processNum := GinkgoParallelProcess()
	GinkgoWriter.Printf("━━━ [Process %d] Phase 2: Per-Process Controller Setup ━━━\n", processNum)
	GinkgoWriter.Printf("✅ [Process %d] Received ServiceAccount token (length: %d bytes)\n", processNum, len(token))
	GinkgoWriter.Printf("✅ [Process %d] Received %d workflow UUID mappings from Phase 1\n", processNum, len(workflowUUIDs))

	// Declare Phase 2 variables
	var err error
	var cfg *rest.Config

	By(fmt.Sprintf("[Process %d] Creating per-process context", processNum))
	ctx, cancel = context.WithCancel(context.Background())

	By(fmt.Sprintf("[Process %d] Registering AIAnalysis CRD scheme", processNum))
	err = aianalysisv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	By(fmt.Sprintf("[Process %d] Bootstrapping per-process envtest environment", processNum))
	// DD-TEST-010: Each process gets its OWN Kubernetes API server (envtest)
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	// KUBEBUILDER_ASSETS is set by Makefile via setup-envtest dependency

	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	By(fmt.Sprintf("[Process %d] Creating per-process K8s client", processNum))
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	By(fmt.Sprintf("[Process %d] Creating per-process namespaces", processNum))
	// Create kubernaut-system namespace for controller
	// Static namespace name - add managed label directly
	systemNs := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kubernaut-system",
			Labels: map[string]string{
				"kubernaut.ai/managed": "true",
			},
		},
	}
	err = k8sClient.Create(ctx, systemNs)
	Expect(err).NotTo(HaveOccurred())

	// Create default namespace for tests
	// Static namespace name - add managed label directly
	defaultNs := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
			Labels: map[string]string{
				"kubernaut.ai/managed": "true",
			},
		},
	}
	_ = k8sClient.Create(ctx, defaultNs) // May already exist

	By(fmt.Sprintf("[Process %d] Setting up per-process controller manager", processNum))
	k8sManager, err = ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0", // Random port per process (no conflicts)
		},
	})
	Expect(err).ToNot(HaveOccurred())

	By(fmt.Sprintf("[Process %d] Creating per-process isolated metrics registry", processNum))
	// DD-METRICS-001: Each process needs isolated Prometheus registry
	testRegistry = prometheus.NewRegistry()
	testMetrics = metrics.NewMetricsWithRegistry(testRegistry)

	By(fmt.Sprintf("[Process %d] Creating per-process audit store", processNum))
	// DD-AUTH-014: Create authenticated DataStorage clients (assign to global variable)
	// Each process gets its own client but uses the same ServiceAccount token from Phase 1
	dsClients = integration.NewAuthenticatedDataStorageClients(
		"http://127.0.0.1:18095", // AIAnalysis integration test DS port
		token,
		5*time.Second,
	)
	GinkgoWriter.Printf("✅ [Process %d] Authenticated DataStorage clients created\n", processNum)

	auditConfig := audit.DefaultConfig()
	auditConfig.FlushInterval = 100 * time.Millisecond // Faster flush for tests
	auditLogger := zap.New(zap.WriteTo(GinkgoWriter))

	auditStore, err = audit.NewBufferedStore(dsClients.AuditClient, auditConfig, "aianalysis", auditLogger)
	Expect(err).ToNot(HaveOccurred(), "Audit store creation must succeed for DD-AUDIT-003")

	// Create audit client for handlers
	auditClient := aiaudit.NewAuditClient(auditStore, auditLogger)

	By(fmt.Sprintf("[Process %d] Setting up per-process HAPI client with authentication", processNum))
	// DD-AUTH-014: HAPI middleware requires Bearer token (real K8s auth via envtest)
	// Use ServiceAccount transport (HAPI will mock-validate the token)
	hapiAuthTransport := testauth.NewServiceAccountTransport(token)
	realHGClient, err = hgclient.NewHolmesGPTClientWithTransport(hgclient.Config{
		BaseURL: "http://localhost:18120",
		Timeout: 30 * time.Second,
	}, hapiAuthTransport)
	Expect(err).ToNot(HaveOccurred(), "failed to create real HAPI client")

	By(fmt.Sprintf("[Process %d] Setting up per-process Rego evaluator", processNum))
	// Per user requirement: "real rego evaluator for all 3 tiers"
	policyPath := filepath.Join("..", "..", "..", "config", "rego", "aianalysis", "approval.rego")
	realRegoEvaluator = rego.NewEvaluator(rego.Config{
		PolicyPath: policyPath,
	}, ctrl.Log.WithName("rego"))

	// Create context for Rego evaluator lifecycle
	regoCtx, regoCancel = context.WithCancel(context.Background())

	// ADR-050: Startup validation required
	err = realRegoEvaluator.StartHotReload(regoCtx)
	Expect(err).NotTo(HaveOccurred(), "Production policy should load successfully in integration tests")

	By(fmt.Sprintf("[Process %d] Setting up per-process controller with handlers", processNum))
	// Create handlers with REAL HAPI client, metrics, and REAL audit client
	eventRecorder := k8sManager.GetEventRecorderFor("aianalysis-controller")
	investigatingHandler := handlers.NewInvestigatingHandler(realHGClient, ctrl.Log.WithName("investigating-handler"), testMetrics, auditClient,
		handlers.WithRecorder(eventRecorder),  // DD-EVENT-001: Session lifecycle events
		handlers.WithSessionMode())            // BR-AA-HAPI-064: Async submit/poll/result flow
	analyzingHandler := handlers.NewAnalyzingHandler(realRegoEvaluator, ctrl.Log.WithName("analyzing-handler"), testMetrics, auditClient)

	// Create per-process controller instance and STORE IT (WorkflowExecution pattern)
	// Storing reconciler enables tests to access metrics via reconciler.Metrics
	reconciler = &aianalysis.AIAnalysisReconciler{
		Metrics:              testMetrics, // DD-METRICS-001: Per-process metrics
		Client:               k8sManager.GetClient(),
		Scheme:               k8sManager.GetScheme(),
		Recorder:             eventRecorder,
		Log:                  ctrl.Log.WithName("aianalysis-controller"),
		StatusManager:        status.NewManager(k8sManager.GetClient(), k8sManager.GetAPIReader()), // DD-PERF-001 + AA-HAPI-001: Cache-bypassed refetch
		InvestigatingHandler: investigatingHandler,
		AnalyzingHandler:     analyzingHandler,
		AuditClient:          auditClient,
	}
	err = reconciler.SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	By(fmt.Sprintf("[Process %d] Starting per-process controller manager", processNum))
	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()

	By(fmt.Sprintf("[Process %d] Waiting for per-process controller manager to be ready", processNum))
	Eventually(func() bool {
		return k8sManager.GetCache().WaitForCacheSync(ctx)
	}, 10*time.Second, 100*time.Millisecond).Should(BeTrue(), "Controller manager cache should sync within 10s")

	GinkgoWriter.Printf("✅ [Process %d] Controller ready\n", processNum)
	GinkgoWriter.Printf("  • envtest: %s\n", cfg.Host)
	GinkgoWriter.Printf("  • Controller: AIAnalysisReconciler\n")
	GinkgoWriter.Printf("  • Metrics: Isolated registry (per-process)\n")
	GinkgoWriter.Printf("  • Audit: Buffered store → DataStorage\n")
	GinkgoWriter.Println("")
})

// SynchronizedAfterSuite ensures proper cleanup in parallel execution
var _ = SynchronizedAfterSuite(func() {
	// This runs on ALL parallel processes - cleanup per-process resources
	processNum := GinkgoParallelProcess()
	GinkgoWriter.Printf("[Process %d] Cleaning up per-process resources...\n", processNum)

	By(fmt.Sprintf("[Process %d] Stopping Rego evaluator", processNum))
	if regoCancel != nil {
		regoCancel() // Stop hot-reload goroutine
	}

	By(fmt.Sprintf("[Process %d] Flushing audit store", processNum))
	flushCtx, flushCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer flushCancel()

	if auditStore != nil {
		if err := auditStore.Flush(flushCtx); err != nil {
			GinkgoWriter.Printf("⚠️  [Process %d] Failed to flush audit store: %v\n", processNum, err)
		}
		if err := auditStore.Close(); err != nil {
			GinkgoWriter.Printf("⚠️  [Process %d] Audit store close error: %v\n", processNum, err)
		}
	}

	By(fmt.Sprintf("[Process %d] Stopping controller manager", processNum))
	if cancel != nil {
		cancel()
	}

	By(fmt.Sprintf("[Process %d] Tearing down envtest environment", processNum))
	if testEnv != nil {
		err := testEnv.Stop()
		if err != nil {
			GinkgoWriter.Printf("⚠️  [Process %d] Failed to stop envtest: %v\n", processNum, err)
		}
	}

	GinkgoWriter.Printf("✅ [Process %d] Per-process cleanup complete\n", processNum)
}, func() {
	// This runs ONCE on the last parallel process - cleanup shared infrastructure
	GinkgoWriter.Println("━━━ [Last Process] Cleaning up shared infrastructure ━━━")

	// DD-TEST-DIAGNOSTICS: Must-gather container logs for post-mortem analysis
	// ALWAYS collect logs - failures may have occurred on other parallel processes
	// The overhead is minimal (~2s) and logs are invaluable for debugging flaky tests
	if dsInfra != nil {
		GinkgoWriter.Println("📦 Collecting container logs for post-mortem analysis...")
		infrastructure.MustGatherContainerLogs("aianalysis", []string{
			dsInfra.DataStorageContainer,
			dsInfra.PostgresContainer,
			dsInfra.RedisContainer,
			"mock-llm-aianalysis",  // Mock LLM service
			"aianalysis_hapi_test", // HolmesGPT-API service
		}, GinkgoWriter)
	}

	// Check if containers should be preserved for debugging
	preserveContainers := os.Getenv("PRESERVE_CONTAINERS") == "true"

	if preserveContainers {
		GinkgoWriter.Println("⚠️  Tests may have failed - preserving containers for debugging")
		GinkgoWriter.Println("📋 To inspect container logs:")
		GinkgoWriter.Println("   podman logs aianalysis_hapi_test")
		GinkgoWriter.Println("   podman logs aianalysis_datastorage_test")
		GinkgoWriter.Println("   podman logs aianalysis_postgres_test")
		GinkgoWriter.Println("   podman logs aianalysis_redis_test")
		GinkgoWriter.Println("📋 To manually clean up:")
		GinkgoWriter.Println("   podman stop aianalysis_hapi_test aianalysis_datastorage_test aianalysis_redis_test aianalysis_postgres_test")
		GinkgoWriter.Println("   podman rm aianalysis_hapi_test aianalysis_datastorage_test aianalysis_redis_test aianalysis_postgres_test")
		GinkgoWriter.Println("   podman network rm aianalysis_test_network")
	} else {
		// FIX: Ginkgo API Compliance - DeferCleanup cannot be used in SynchronizedBeforeSuite first function
		// All cleanup must happen here in SynchronizedAfterSuite second function (process 1 only)
		// Cleanup in reverse order of setup

		// 1. Stop HAPI container (capture logs first for debugging)
		if hapiContainer != nil {
			GinkgoWriter.Println("\n📋 Capturing HAPI container logs before cleanup:")
			logsCmd := exec.Command("podman", "logs", "--tail", "100", hapiContainer.Name)
			logsCmd.Stdout = GinkgoWriter
			logsCmd.Stderr = GinkgoWriter
			_ = logsCmd.Run()
			GinkgoWriter.Println("")

			if err := infrastructure.StopGenericContainer(hapiContainer, GinkgoWriter); err != nil {
				GinkgoWriter.Printf("⚠️  Failed to stop HAPI container: %v\n", err)
			}
		}

		// 2. Stop Mock LLM container
		if mockLLMConfig.ServiceName != "" {
			stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer stopCancel()
			if err := infrastructure.StopMockLLMContainer(stopCtx, mockLLMConfig, GinkgoWriter); err != nil {
				GinkgoWriter.Printf("⚠️  Failed to stop Mock LLM container: %v\n", err)
			}
		}

		// 3. Remove Mock LLM config file
		if mockLLMConfigPath != "" {
			_ = os.Remove(mockLLMConfigPath)
		}

		// 4. Remove HAPI ServiceAccount token directory
		if hapiSATokenDir != "" {
			_ = os.RemoveAll(hapiSATokenDir)
		}

		// 5. Stop DataStorage infrastructure (PostgreSQL, Redis, DataStorage container)
		// Per DD-TEST-001 v1.3: StopDSBootstrap removes DataStorage image by name
		if dsInfra != nil {
			_ = infrastructure.StopDSBootstrap(dsInfra, GinkgoWriter)
		}

		// 6. Stop shared envtest
		if sharedTestEnv != nil {
			GinkgoWriter.Println("\n🛑 Stopping shared envtest")
			err := sharedTestEnv.Stop()
			if err != nil {
				GinkgoWriter.Printf("⚠️  Failed to stop shared envtest: %v\n", err)
			}
		}
	}

	GinkgoWriter.Println("✅ Shared infrastructure cleanup complete")
})

// DD-TEST-002 Compliance: Unique namespace per test for parallel execution
// This enables -procs=4 parallel execution (matching Notification pattern)
// Each test gets its own namespace to prevent resource conflicts

var _ = BeforeEach(func() {
	// DD-TEST-002: Create unique namespace per test (enables parallel execution)
	testNamespace = helpers.CreateTestNamespace(context.Background(), k8sClient, "test-aa")

	GinkgoWriter.Printf("📦 [AA] Test namespace created: %s (DD-TEST-002 compliance)\n", testNamespace)
})

var _ = AfterEach(func() {
	// DD-TEST-002: Clean up namespace and ALL resources (instant cleanup)
	// This is MUCH faster than deleting individual AIAnalysis resources
	if testNamespace != "" {
		helpers.DeleteTestNamespace(context.Background(), k8sClient, testNamespace)
		GinkgoWriter.Printf("🗑️  [AA] Namespace %s deleted (DD-TEST-002 cleanup)\n", testNamespace)
	}
})

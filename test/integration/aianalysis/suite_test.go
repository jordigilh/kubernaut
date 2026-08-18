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
// - BR-AI-002: KA integration
// - BR-AI-003: Rego policy evaluation
//
// Test Strategy: Two integration test categories:
// 1. **Envtest-only tests** (this file): Use mock agent client for fast controller testing
// 2. **Real service tests**: Use real Kubernaut Agent (auto-started)
//
// Defense-in-Depth (per 03-testing-strategy.mdc):
// - Unit tests (70%+): Mock K8s client + mock agent
// - Integration tests (>50%): Real K8s API (envtest) + mock/real Kubernaut Agent
// - E2E tests (10-15%): Real K8s API (KIND) + real Kubernaut Agent
//
// DD-TEST-010: Multi-Controller Architecture (Controller-Per-Process Pattern)
// Infrastructure (AUTO-STARTED in Phase 1, process 1 only):
// - PostgreSQL (port 15438): Persistence layer
// - Redis (port 16384): Caching layer
// - Data Storage API (port 18095): Audit trail
// - Mock LLM Service (port 18141): Standalone OpenAI-compatible mock (AIAnalysis-specific)
// - KA (port 18120): AI analysis service (uses Mock LLM at 18141)
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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	aianalysisv1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	isv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/controller/aianalysis"
	"github.com/jordigilh/kubernaut/pkg/agentclient"
	aiaudit "github.com/jordigilh/kubernaut/pkg/aianalysis/audit"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/creator"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/rego"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/status"
	"github.com/jordigilh/kubernaut/pkg/audit"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
	"github.com/jordigilh/kubernaut/test/shared/integration"
)

// goosLinux identifies the Linux GOOS value, used to gate host-network mode
// (only supported on Linux runners/CI).
const goosLinux = "linux"

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

	// Per-process agent client (each process gets its own)
	realAgentClient *agentclient.KubernautAgentClient

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
	mockLLMConfig     infrastructure.MockLLMConfig
	mockLLMConfigPath string

	// DD-AA-KA-001: KA now runs as a per-process container (Phase 2, all
	// processes) watching that process's own envtest for AgentSession CRDs
	// -- cleaned up per-process in SynchronizedAfterSuite's first function,
	// not the shared/last-process-only second function.
	kaContainer  *infrastructure.ContainerInstance
	kaSATokenDir string

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
// CI environments (GitHub Actions) have slower container startup times, especially KA.
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
	GinkgoWriter.Println("  • KA HTTP service (port 18120, uses Mock LLM)")
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

	// DD-AUTH-014: Grant AIAnalysis controller SA permission to call Kubernaut Agent
	By("Granting AIAnalysis controller SA permission to call Kubernaut Agent")
	// #1661 Phase 55: rwv1alpha1 registered so this client can seed
	// RemediationWorkflow CRDs directly below (SeedTestWorkflowsViaDirectCRDCreation).
	Expect(rwv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
	k8sClient, err := client.New(sharedCfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())

	agentClientRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kubernaut-agent-client",
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups:     []string{""},
				Resources:     []string{"services"},
				ResourceNames: []string{"kubernaut-agent"},
				Verbs:         []string{"create", "get"},
			},
		},
	}
	err = k8sClient.Create(context.Background(), agentClientRole)
	if !apierrors.IsAlreadyExists(err) {
		Expect(err).ToNot(HaveOccurred())
	}

	agentClientBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "aianalysis-kubernaut-agent-client",
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "kubernaut-agent-client",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "aianalysis-ds-client",
				Namespace: "default",
			},
		},
	}
	err = k8sClient.Create(context.Background(), agentClientBinding)
	if !apierrors.IsAlreadyExists(err) {
		Expect(err).ToNot(HaveOccurred())
	}
	GinkgoWriter.Println("✅ AIAnalysis controller granted Kubernaut Agent access permissions")

	// DD-AA-KA-001: KA itself moves to a per-process instance below (Phase 2),
	// one per Ginkgo process, each pointed at that process's OWN envtest --
	// AA's channel to KA is now the AgentSession CRD, so KA's dispatch
	// watcher must watch the exact same cluster AA's per-process controller
	// writes to (DD-TEST-010's single shared envtest, used only for
	// DataStorage auth above, cannot also host N per-process controllers'
	// AgentSession writes without cross-process reconcile races). KA's
	// ServiceAccount/RBAC/enrichment-fixtures/container therefore move from
	// this shared Phase 1 into per-process Phase 2 as well.

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// OPTIMIZATION: Build images in parallel (saves ~100 seconds)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	By("Building DataStorage, Mock LLM, and Kubernaut Agent images in parallel")
	var (
		dsImageName      string
		mockLLMImageName string
		kaImageName      string
		dsErr            error
		mockErr          error
		kaErr            error
		wg               sync.WaitGroup
	)

	wg.Add(3)

	go func() {
		defer wg.Done()
		defer GinkgoRecover()
		dsImageName, _, dsErr = infrastructure.BuildDataStorageImage(specCtx, "aianalysis", GinkgoWriter)
	}()

	go func() {
		defer wg.Done()
		defer GinkgoRecover()
		mockLLMImageName, mockErr = infrastructure.BuildMockLLMImage(specCtx, "aianalysis", GinkgoWriter)
	}()

	go func() {
		defer wg.Done()
		defer GinkgoRecover()
		kaImageName, kaErr = infrastructure.BuildKubernautAgentImage(specCtx, "aianalysis", GinkgoWriter)
	}()

	wg.Wait()

	Expect(dsErr).ToNot(HaveOccurred(), "DataStorage image must build successfully")
	Expect(mockErr).ToNot(HaveOccurred(), "Mock LLM image must build successfully")
	Expect(kaErr).ToNot(HaveOccurred(), "KA image must build successfully")
	GinkgoWriter.Printf("✅ All three images built in parallel: DS=%s, MockLLM=%s, KA=%s\n", dsImageName, mockLLMImageName, kaImageName)

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
	dsInfra, err = infrastructure.StartDSBootstrap(context.Background(), cfg, GinkgoWriter)
	Expect(err).ToNot(HaveOccurred(), "Infrastructure must start successfully")
	GinkgoWriter.Println("✅ DataStorage infrastructure started (PostgreSQL, Redis, DataStorage)")

	// NOTE: Cleanup moved to SynchronizedAfterSuite (cannot use DeferCleanup in first function)

	// Seed test workflows directly as RemediationWorkflow CRDs BEFORE starting
	// Mock LLM (#1661 Phase 55: this suite runs no AuthWebhook, so status is
	// patched locally -- see SeedTestWorkflowsViaDirectCRDCreation).
	// Pattern: DD-TEST-011 v2.0 - File-Based Configuration
	// Must seed workflows first so Mock LLM can load UUIDs at startup
	By("Seeding test workflows via direct CRD creation")
	workflowUUIDs, err := SeedTestWorkflowsViaDirectCRDCreation(specCtx, k8sClient, "default", GinkgoWriter)
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
	// Per DD-TEST-001 v2.3: Port 18141 (AIAnalysis-specific, unique from KA's 18140)
	// Per MOCK_LLM_MIGRATION_PLAN.md v1.3.0: Standalone service for test isolation
	mockLLMConfig = infrastructure.GetMockLLMConfigForAIAnalysis()
	mockLLMConfig.ImageTag = mockLLMImageName        // Use the built image tag
	mockLLMConfig.ConfigFilePath = mockLLMConfigPath // DD-TEST-011 v2.0: Mount config file
	// DD-AUTH-014: Platform-specific network (must match KA's network mode)
	if runtime.GOOS == goosLinux {
		mockLLMConfig.Network = "host" // Linux CI: Host network (KA will reach via 127.0.0.1)
	} else {
		mockLLMConfig.Network = "aianalysis_test_network" // macOS: Bridge network with container-to-container DNS
	}
	mockLLMContainerID, err := infrastructure.StartMockLLMContainer(specCtx, mockLLMConfig, GinkgoWriter)
	Expect(err).ToNot(HaveOccurred(), "Mock LLM container must start successfully")
	Expect(mockLLMContainerID).ToNot(BeEmpty(), "Mock LLM container ID must be returned")
	GinkgoWriter.Printf("✅ Mock LLM service started with config file (port %d)\n", mockLLMConfig.Port)

	// NOTE: Cleanup moved to SynchronizedAfterSuite (cannot use DeferCleanup in first function)

	// DD-AA-KA-001: KA container itself is started per-process in Phase 2
	// below (see startPerProcessKubernautAgent), not here -- see the note
	// above the removed shared-KA RBAC block for why.

	GinkgoWriter.Println("✅ Infrastructure startup complete (Phase 1)")
	GinkgoWriter.Println("  Phase 2 will now run on ALL processes (per-process controller setup)")
	GinkgoWriter.Println("")

	// DD-AUTH-014 + DD-TEST-010: Phase 1 → Phase 2 data passing
	// Serialize token, workflowUUIDs, and the pre-built KA image tag for ALL
	// processes -- DD-AA-KA-001: each process builds its OWN KA container in
	// Phase 2 (see startPerProcessKubernautAgent) from this same image, so it
	// must know the tag Phase 1 already built rather than rebuilding it.
	type Phase1Data struct {
		Token         string            `json:"token"`
		WorkflowUUIDs map[string]string `json:"workflow_uuids"`
		KAImageName   string            `json:"ka_image_name"`
	}
	phase1Data := Phase1Data{
		Token:         authConfig.Token,
		WorkflowUUIDs: workflowUUIDs,
		KAImageName:   kaImageName,
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

	// DD-AUTH-014 + DD-TEST-010: Deserialize token, workflow UUIDs, and the
	// pre-built KA image tag from Phase 1
	type Phase1Data struct {
		Token         string            `json:"token"`
		WorkflowUUIDs map[string]string `json:"workflow_uuids"`
		KAImageName   string            `json:"ka_image_name"`
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

	By(fmt.Sprintf("[Process %d] Registering InvestigationSession CRD scheme (BR-INTERACTIVE-010)", processNum))
	err = isv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	By(fmt.Sprintf("[Process %d] Registering AgentSession CRD scheme (DD-AA-KA-001)", processNum))
	err = agentsessionv1.AddToScheme(scheme.Scheme)
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

	// BR-INTERACTIVE-010: Register field indexes for IS and AA RR name lookups.
	// Note: Cache ByObject namespace restriction is omitted in integration tests because
	// DD-TEST-002 uses per-test namespaces (testNamespace), not kubernaut-system only.
	By(fmt.Sprintf("[Process %d] Registering InvestigationSession field index (BR-INTERACTIVE-010)", processNum))
	err = k8sManager.GetFieldIndexer().IndexField(ctx,
		&isv1alpha1.InvestigationSession{},
		handlers.ISFieldIndexRRName,
		func(obj client.Object) []string {
			is := obj.(*isv1alpha1.InvestigationSession)
			if is.Spec.RemediationRequestRef.Name == "" {
				return nil
			}
			return []string{is.Spec.RemediationRequestRef.Name}
		},
	)
	Expect(err).NotTo(HaveOccurred())

	By(fmt.Sprintf("[Process %d] Registering AIAnalysis RR name field index (BR-INTERACTIVE-010)", processNum))
	err = k8sManager.GetFieldIndexer().IndexField(ctx,
		&aianalysisv1alpha1.AIAnalysis{},
		aianalysis.AIAnalysisRRNameIndex(),
		func(obj client.Object) []string {
			aa := obj.(*aianalysisv1alpha1.AIAnalysis)
			if aa.Spec.RemediationRequestRef.Name == "" {
				return nil
			}
			return []string{aa.Spec.RemediationRequestRef.Name}
		},
	)
	Expect(err).NotTo(HaveOccurred())

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

	// DD-AA-KA-001: KA itself now moves per-process, right alongside its own
	// AIAnalysisReconciler -- AA's channel to KA is the AgentSession CRD, so
	// KA's dispatch watcher must watch this SAME per-process cfg/envtest,
	// not the shared one DataStorage auth used above (see the Phase 1 note
	// this replaces). Enrichment fixtures move here too, since KA's own
	// live K8s enrichment queries now target this per-process cluster.
	By(fmt.Sprintf("[Process %d] Creating enrichment fixture resources (#704)", processNum))
	createITAAEnrichmentFixtures(k8sClient)

	By(fmt.Sprintf("[Process %d] Starting per-process Kubernaut Agent HTTP service", processNum))
	kaBaseURL, kaCallerToken := startPerProcessKubernautAgent(processNum, cfg, phase1Data.KAImageName)

	By(fmt.Sprintf("[Process %d] Setting up per-process agent client with authentication", processNum))
	// DD-AUTH-014: KA middleware requires Bearer token (real K8s auth against
	// THIS process's own envtest, minted in startPerProcessKubernautAgent
	// above -- the shared Phase-1 token above is only valid against
	// sharedCfg's signing key, not this process's per-process cfg).
	// Wrap with RetryOn429Transport to absorb transient rate-limit rejections
	// from the per-IP token-bucket limiter during parallel test execution.
	kaAuthTransport := testauth.NewRetryOn429Transport(
		testauth.NewServiceAccountTransport(kaCallerToken),
	)
	realAgentClient, err = agentclient.NewKubernautAgentClientWithTransport(agentclient.Config{
		BaseURL: kaBaseURL,
		Timeout: 30 * time.Second,
	}, kaAuthTransport)
	Expect(err).ToNot(HaveOccurred(), "failed to create real agent client")

	By(fmt.Sprintf("[Process %d] Setting up per-process Rego evaluator", processNum))
	// Test-owned policy fixture decoupled from production config.
	policyPath := filepath.Join("testdata", "policies", "approval.rego")
	realRegoEvaluator = rego.NewEvaluator(rego.Config{
		PolicyPath: policyPath,
	}, ctrl.Log.WithName("rego"))

	// Create context for Rego evaluator lifecycle
	regoCtx, regoCancel = context.WithCancel(context.Background())

	// ADR-050: Startup validation required
	err = realRegoEvaluator.StartHotReload(regoCtx)
	Expect(err).NotTo(HaveOccurred(), "Production policy should load successfully in integration tests")

	By(fmt.Sprintf("[Process %d] Setting up per-process controller with handlers", processNum))
	// DD-AA-KA-001: AgentSessionCreator replaces the retired HTTP submit/poll
	// channel (realAgentClient is kept only for the legacy direct-HTTP KA
	// test in agentclient_integration_test.go, not for the reconciler).
	eventRecorder := k8sManager.GetEventRecorderFor("aianalysis-controller")
	const controllerNS = "kubernaut-system"
	isPhaseUpdater := handlers.NewK8sISPhaseUpdater(k8sManager.GetClient(), controllerNS)
	agentSessionCreator := creator.NewAgentSessionCreator(k8sManager.GetClient(), k8sManager.GetScheme())
	investigatingHandler := handlers.NewInvestigatingHandler(agentSessionCreator, ctrl.Log.WithName("investigating-handler"), testMetrics, auditClient,
		handlers.WithRecorder(eventRecorder),            // DD-EVENT-001: Session lifecycle events
		handlers.WithSessionPollInterval(2*time.Second), // Fast requeue for tests (production default: 15s)
		handlers.WithISPhaseUpdater(isPhaseUpdater))     // #1376: Write-only IS terminal-close
	// #225: Mock LLM current_scenario persists across analyses (statefulness),
	// so unrecognized signals inherit high confidence (e.g., 0.88 from crashloop).
	// Threshold 0.9 ensures mock scenarios requiring approval stay below threshold.
	testThreshold := 0.9
	analyzingHandler := handlers.NewAnalyzingHandler(realRegoEvaluator, ctrl.Log.WithName("analyzing-handler"), testMetrics, auditClient).
		WithConfidenceThreshold(&testThreshold)

	// Create per-process controller instance and STORE IT (WorkflowExecution pattern)
	// Storing reconciler enables tests to access metrics via reconciler.Metrics
	reconciler = &aianalysis.AIAnalysisReconciler{
		Metrics:          testMetrics, // DD-METRICS-001: Per-process metrics
		Client:           k8sManager.GetClient(),
		Scheme:           k8sManager.GetScheme(),
		Recorder:         eventRecorder,
		Log:              ctrl.Log.WithName("aianalysis-controller"),
		StatusManager:    status.NewManager(k8sManager.GetClient(), k8sManager.GetAPIReader()), // DD-PERF-001 + AA-KA-001: Cache-bypassed refetch
		AnalyzingHandler: analyzingHandler,
		AuditClient:      auditClient,
	}
	reconciler.InvestigatingHandler.Store(investigatingHandler)
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

	// DD-AA-KA-001: KA runs per-process now -- stop THIS process's own
	// container before tearing down the envtest it was watching.
	By(fmt.Sprintf("[Process %d] Stopping per-process Kubernaut Agent container", processNum))
	if kaContainer != nil {
		GinkgoWriter.Printf("\n📋 [Process %d] Capturing KA container logs before cleanup:\n", processNum)
		logsCmd := exec.Command("podman", "logs", "--tail", "100", kaContainer.Name)
		logsCmd.Stdout = GinkgoWriter
		logsCmd.Stderr = GinkgoWriter
		_ = logsCmd.Run()

		if err := infrastructure.StopGenericContainer(kaContainer, GinkgoWriter); err != nil {
			GinkgoWriter.Printf("⚠️  [Process %d] Failed to stop KA container: %v\n", processNum, err)
		}
	}
	if kaSATokenDir != "" {
		_ = os.RemoveAll(kaSATokenDir)
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
	// DD-AA-KA-001: per-process KA containers are captured/stopped in the
	// per-process cleanup function above, not here.
	if dsInfra != nil {
		GinkgoWriter.Println("📦 Collecting container logs for post-mortem analysis...")
		infrastructure.MustGatherContainerLogs("aianalysis", []string{
			dsInfra.DataStorageContainer,
			dsInfra.PostgresContainer,
			dsInfra.RedisContainer,
			"mock-llm-aianalysis", // Mock LLM service
		}, GinkgoWriter)
	}

	// Check if containers should be preserved for debugging
	preserveContainers := os.Getenv("PRESERVE_CONTAINERS") == "true"

	if preserveContainers {
		GinkgoWriter.Println("⚠️  Tests may have failed - preserving containers for debugging")
		GinkgoWriter.Println("📋 To inspect container logs:")
		GinkgoWriter.Println("   podman logs aianalysis_ka_test_<processNum>")
		GinkgoWriter.Println("   podman logs aianalysis_datastorage_test")
		GinkgoWriter.Println("   podman logs aianalysis_postgres_test")
		GinkgoWriter.Println("   podman logs aianalysis_redis_test")
		GinkgoWriter.Println("📋 To manually clean up:")
		GinkgoWriter.Println("   podman stop aianalysis_datastorage_test aianalysis_redis_test aianalysis_postgres_test")
		GinkgoWriter.Println("   podman rm aianalysis_datastorage_test aianalysis_redis_test aianalysis_postgres_test")
		GinkgoWriter.Println("   podman network rm aianalysis_test_network")
	} else {
		// FIX: Ginkgo API Compliance - DeferCleanup cannot be used in SynchronizedBeforeSuite first function
		// All cleanup must happen here in SynchronizedAfterSuite second function (process 1 only)
		// Cleanup in reverse order of setup

		// 1. Stop Mock LLM container
		if mockLLMConfig.ServiceName != "" {
			stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer stopCancel()
			if err := infrastructure.StopMockLLMContainer(stopCtx, mockLLMConfig, GinkgoWriter); err != nil {
				GinkgoWriter.Printf("⚠️  Failed to stop Mock LLM container: %v\n", err)
			}
		}

		// 2. Remove Mock LLM config file
		if mockLLMConfigPath != "" {
			_ = os.Remove(mockLLMConfigPath)
		}

		// 3. Stop DataStorage infrastructure (PostgreSQL, Redis, DataStorage container)
		// Per DD-TEST-001 v1.3: StopDSBootstrap removes DataStorage image by name
		if dsInfra != nil {
			_ = infrastructure.StopDSBootstrap(dsInfra, GinkgoWriter)
		}

		// 4. Stop shared envtest
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

// DD-TEST-002 Compliance (amended by DD-AA-KA-001 per-process KA): a fresh
// namespace per test previously enabled parallel execution across processes
// sharing one envtest. Now that each Ginkgo process owns its own isolated
// envtest (DD-TEST-010), that cross-process collision risk no longer exists
// -- but a new constraint replaces it: the per-process Kubernaut Agent
// container's AgentSession dispatcher watches exactly one fixed namespace
// (detectNamespace() in cmd/kubernautagent/health.go, which falls back to
// "kubernaut-system" when no ServiceAccount "namespace" file is mounted --
// see startPerProcessKubernautAgent, which deliberately does not mount one).
// Any AIAnalysis/AgentSession created in a fresh per-test namespace would be
// invisible to that watch, so tests needing real KA dispatch would poll
// forever. testNamespace is therefore now the shared, already-existing
// "kubernaut-system" namespace (created once per process above) rather than
// a fresh namespace per test; per-test isolation instead comes from each
// test's own timestamp/uuid-suffixed object names.

var _ = BeforeEach(func() {
	testNamespace = "kubernaut-system"
})

var _ = AfterEach(func() {
	// testNamespace is shared for the whole process's lifetime now (see
	// BeforeEach) -- delete only this test's AIAnalysis/AgentSession objects
	// rather than the namespace itself.
	ctx := context.Background()
	_ = k8sClient.DeleteAllOf(ctx, &aianalysisv1alpha1.AIAnalysis{}, client.InNamespace(testNamespace))
	_ = k8sClient.DeleteAllOf(ctx, &agentsessionv1.AgentSession{}, client.InNamespace(testNamespace))
})

// startPerProcessKubernautAgent starts a dedicated Kubernaut Agent HTTP
// container for THIS Ginkgo parallel process, with its dispatch watcher
// pointed at this process's own envtest (cfg) via a freshly-minted
// kubeconfig -- not the shared envtest DataStorage auth uses (DD-AA-KA-001).
//
// AA's own channel to KA is the AgentSession CRD (creator.AgentSessionCreator,
// wired by the caller), watched natively by KA's in-process dispatcher
// against this same cfg. The HTTP endpoint started here is kept only for
// the legacy direct-HTTP KA test (agentclient_integration_test.go, exercising
// KA's not-yet-deleted raw /investigate endpoint per DD-AA-KA-001) --
// the reconciler itself no longer calls it.
//
// Returns the per-process KA base URL and a caller Bearer token valid
// against cfg's TokenReview API (for realAgentClient's direct-HTTP tests).
func startPerProcessKubernautAgent(processNum int, cfg *rest.Config, kaImageName string) (baseURL string, callerToken string) {
	// DD-AUTH-014: KA's ServiceAccount binding reuses the "datastorage-tokenreview"
	// ClusterRole (generic TokenReview/SAR create verbs) -- CreateServiceAccountForHTTPService
	// expects it to already exist; the shared envtest gets it from
	// CreateIntegrationServiceAccountWithDataStorageAccess in Phase 1, but this
	// process's own cfg needs its own copy.
	tokenReviewRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "datastorage-tokenreview",
			Labels: map[string]string{
				"app":       "datastorage",
				"component": "rbac",
				"test":      "integration",
			},
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"authentication.k8s.io"},
				Resources: []string{"tokenreviews"},
				Verbs:     []string{"create"},
			},
			{
				APIGroups: []string{"authorization.k8s.io"},
				Resources: []string{"subjectaccessreviews"},
				Verbs:     []string{"create"},
			},
		},
	}
	createErr := k8sClient.Create(context.Background(), tokenReviewRole)
	Expect(client.IgnoreAlreadyExists(createErr)).ToNot(HaveOccurred())

	useHostNetworkForKA := runtime.GOOS == goosLinux
	kaServiceAuthConfig, err := infrastructure.CreateServiceAccountForHTTPService(
		cfg,
		"kubernaut-agent-service",
		"default",
		useHostNetworkForKA,
		GinkgoWriter,
	)
	Expect(err).ToNot(HaveOccurred())
	GinkgoWriter.Printf("✅ [Process %d] ServiceAccount + RBAC created for KA → per-process envtest (TokenReview/SAR)\n", processNum)

	// #704: K8s investigation + label detection RBAC for KA. Mirrors
	// kubernaut-agent-investigator ClusterRole from E2E (aianalysis_e2e.go).
	investigatorRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "kubernaut-agent-investigator"},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods", "pods/log", "events", "services", "configmaps", "nodes", "namespaces", "replicationcontrollers", "persistentvolumeclaims"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{"apps"},
				Resources: []string{"deployments", "replicasets", "statefulsets", "daemonsets"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{"batch"},
				Resources: []string{"jobs", "cronjobs"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{"events.k8s.io"},
				Resources: []string{"events"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{"policy"},
				Resources: []string{"poddisruptionbudgets"},
				Verbs:     []string{"get", "list"},
			},
			{
				APIGroups: []string{"autoscaling"},
				Resources: []string{"horizontalpodautoscalers"},
				Verbs:     []string{"get", "list"},
			},
			{
				APIGroups: []string{"networking.k8s.io"},
				Resources: []string{"networkpolicies"},
				Verbs:     []string{"get", "list"},
			},
			// DD-AA-KA-001: KA's dispatch watcher needs AgentSession
			// get/list/watch + status update, InvestigationSession read-only
			// (dispatch-time interactive check), workflow catalog
			// (RemediationWorkflow/ActionType, DD-WORKFLOW-019), and Lease
			// for dispatch coordination -- mirrors charts/kubernaut/templates/
			// kubernaut-agent/kubernaut-agent.yaml's RBAC.
			{
				APIGroups: []string{"kubernaut.ai"},
				Resources: []string{"agentsessions"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{"kubernaut.ai"},
				Resources: []string{"agentsessions/status"},
				Verbs:     []string{"get", "update", "patch"},
			},
			{
				APIGroups: []string{"kubernaut.ai"},
				Resources: []string{"investigationsessions"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{"kubernaut.ai"},
				Resources: []string{"remediationworkflows", "actiontypes"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{"kubernaut.ai"},
				Resources: []string{"remediationrequests"},
				Verbs:     []string{"get", "list"},
			},
			{
				APIGroups: []string{"coordination.k8s.io"},
				Resources: []string{"leases"},
				Verbs:     []string{"get", "list", "create", "update", "delete"},
			},
		},
	}
	Expect(client.IgnoreAlreadyExists(k8sClient.Create(context.Background(), investigatorRole))).ToNot(HaveOccurred())

	investigatorBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "kubernaut-agent-service-investigator"},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "kubernaut-agent-investigator",
		},
		Subjects: []rbacv1.Subject{
			{Kind: "ServiceAccount", Name: "kubernaut-agent-service", Namespace: "default"},
		},
	}
	Expect(client.IgnoreAlreadyExists(k8sClient.Create(context.Background(), investigatorBinding))).ToNot(HaveOccurred())
	GinkgoWriter.Printf("✅ [Process %d] KA ServiceAccount granted K8s investigation + AgentSession RBAC (#704, DD-AA-KA-001)\n", processNum)

	// DD-AUTH-014: Create ServiceAccount secrets directory for KA container
	kaSATokenDir = filepath.Join(os.TempDir(), fmt.Sprintf("aianalysis-ka-sa-secrets-%d-%d", processNum, time.Now().UnixNano()))
	Expect(os.MkdirAll(kaSATokenDir, 0755)).To(Succeed(), "Failed to create KA ServiceAccount secrets directory")
	kaTokenFilePath := filepath.Join(kaSATokenDir, "token")
	Expect(os.WriteFile(kaTokenFilePath, []byte(kaServiceAuthConfig.Token), 0644)).To(Succeed(), "Failed to write KA ServiceAccount token to file")

	// Create KA config file for the container
	kaConfigDir := filepath.Join(os.TempDir(), fmt.Sprintf("aianalysis-ka-config-%d-%d", processNum, time.Now().UnixNano()))
	Expect(os.MkdirAll(kaConfigDir, 0755)).To(Succeed())

	// DD-AA-KA-001 port de-confliction: each process's KA container gets its
	// own port triple so N per-process containers can coexist, whether they
	// share the host network namespace (Linux CI) or are individually
	// port-mapped (macOS bridge network, mapped N:N so KA's own config file
	// and the health-check URL agree regardless of platform).
	kaPort := 18120 + (processNum-1)*10
	kaHealthPort := kaPort + 1
	kaMetricsPort := kaPort + 2

	mockLLMCfg := infrastructure.GetMockLLMConfigForAIAnalysis()
	var llmEndpoint, dsURL string
	if useHostNetworkForKA {
		llmEndpoint = fmt.Sprintf("http://127.0.0.1:%d", mockLLMCfg.Port)
		dsURL = "http://127.0.0.1:18095"
	} else {
		llmEndpoint = infrastructure.GetMockLLMContainerEndpoint(mockLLMCfg)
		dsURL = "http://host.containers.internal:18095"
	}

	kaConfigContent := fmt.Sprintf(`runtime:
  logging:
    level: "debug"
  server:
    port: %d
    healthAddr: ":%d"
    metricsAddr: ":%d"
    rateLimit:
      requestsPerSecond: 50
      burst: 100
  audit:
    flushIntervalSeconds: 0.1
    bufferSize: 10000
    batchSize: 50
ai:
  llm:
    provider: "openai"
    apiKeyFile: "/etc/kubernautagent-llm-runtime/api-key"
integrations:
  dataStorage:
    url: "%s"
`, kaPort, kaHealthPort, kaMetricsPort, dsURL)
	kaConfigPath := filepath.Join(kaConfigDir, "config.yaml")
	Expect(os.WriteFile(kaConfigPath, []byte(kaConfigContent), 0644)).To(Succeed())

	kaLLMRuntimeDir, err := os.MkdirTemp("", fmt.Sprintf("ka-llm-runtime-%d-*", processNum))
	Expect(err).ToNot(HaveOccurred())
	Expect(os.Chmod(kaLLMRuntimeDir, 0755)).To(Succeed())
	kaLLMRuntimeContent := fmt.Sprintf(`model: "mock-model"
endpoint: "%s"
temperature: 0.7
maxRetries: 3
timeoutSeconds: 120
`, llmEndpoint)
	Expect(os.WriteFile(filepath.Join(kaLLMRuntimeDir, "llm-runtime.yaml"), []byte(kaLLMRuntimeContent), 0644)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(kaLLMRuntimeDir, "api-key"), []byte("mock-api-key-for-integration-tests"), 0644)).To(Succeed())

	kaContainerConfig := infrastructure.GenericContainerConfig{
		Name:  fmt.Sprintf("aianalysis_ka_test_%d", processNum),
		Image: kaImageName,
		Env: map[string]string{
			"KUBECONFIG":    "/tmp/kubeconfig",
			"POD_NAMESPACE": "default",
		},
		Cmd: []string{"-config", "/etc/kubernautagent/config.yaml", "-llm-runtime", "/etc/kubernautagent-llm-runtime/llm-runtime.yaml"},
		Volumes: map[string]string{
			kaConfigDir:                        "/etc/kubernautagent:ro",
			kaLLMRuntimeDir:                    "/etc/kubernautagent-llm-runtime:ro",
			kaServiceAuthConfig.KubeconfigPath: "/tmp/kubeconfig:ro",
			kaSATokenDir:                       "/var/run/secrets/kubernetes.io/serviceaccount:ro",
		},
		HealthCheck: &infrastructure.HealthCheckConfig{
			URL:     fmt.Sprintf("http://127.0.0.1:%d/healthz", kaHealthPort),
			Timeout: 120 * time.Second,
		},
	}

	if useHostNetworkForKA {
		kaContainerConfig.Network = "host"
		GinkgoWriter.Printf("   🌐 [Process %d] KA using host network (Linux CI), port %d\n", processNum, kaPort)
	} else {
		kaContainerConfig.Network = "aianalysis_test_network"
		kaContainerConfig.Ports = map[int]int{kaPort: kaPort, kaHealthPort: kaHealthPort}
		kaContainerConfig.ExtraHosts = []string{"host.containers.internal:host-gateway"}
		GinkgoWriter.Printf("   🌐 [Process %d] KA using bridge network (macOS), port %d\n", processNum, kaPort)
	}
	kaContainer, err = infrastructure.StartGenericContainer(kaContainerConfig, GinkgoWriter)
	Expect(err).ToNot(HaveOccurred(), "KA container must start successfully")
	GinkgoWriter.Printf("✅ [Process %d] Kubernaut Agent started at http://127.0.0.1:%d (container: %s)\n", processNum, kaPort, kaContainer.ID)

	// KA's TokenReview only needs ANY valid token minted from this SAME
	// cluster (cfg) -- reuse KA's own ServiceAccount token as the direct-HTTP
	// test caller's Bearer token rather than minting a separate identity.
	return fmt.Sprintf("http://localhost:%d", kaPort), kaServiceAuthConfig.Token
}

// createITAAEnrichmentFixtures creates namespaces and minimal workloads in the
// shared envtest so that KA's HAPI-default enrichment (MaxRetries=3) can resolve
// mock LLM remediation_target resources. Without these, re-enrichment returns
// NotFound → HardFail → rca_incomplete, breaking tests that expect normal outcomes.
// Mirrors createEnrichmentFixtures() from E2E infrastructure (kubectl-based).
func createITAAEnrichmentFixtures(c client.Client) {
	ctx := context.Background()
	one := int32(1)
	pauseImage := "registry.k8s.io/pause:3.9"
	pauseContainer := corev1.Container{
		Name:  "pause",
		Image: pauseImage,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("8Mi"),
				corev1.ResourceCPU:    resource.MustParse("10m"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("16Mi"),
				corev1.ResourceCPU:    resource.MustParse("50m"),
			},
		},
	}

	for _, ns := range []string{"production", "staging"} {
		nsObj := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}
		if err := c.Create(ctx, nsObj); err != nil && !apierrors.IsAlreadyExists(err) {
			Expect(err).ToNot(HaveOccurred())
		}
	}

	// Deployment/api-server/production — oomkilled + predictive scenarios
	apiDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api-server", Namespace: "production"},
		Spec: appsv1.DeploymentSpec{
			Replicas: &one,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api-server"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "api-server"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{pauseContainer}},
			},
		},
	}
	Expect(c.Create(ctx, apiDeploy)).To(Succeed())

	// Deployment/worker/staging — crashloop scenario
	workerDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "worker", Namespace: "staging"},
		Spec: appsv1.DeploymentSpec{
			Replicas: &one,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "worker"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "worker"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{pauseContainer}},
			},
		},
	}
	Expect(c.Create(ctx, workerDeploy)).To(Succeed())

	// PDB for worker — pdbProtected label detection
	minAvail := intstr.FromInt32(1)
	workerPDB := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-pdb", Namespace: "staging"},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &minAvail,
			Selector:     &metav1.LabelSelector{MatchLabels: map[string]string{"app": "worker"}},
		},
	}
	Expect(c.Create(ctx, workerPDB)).To(Succeed())

	pods := []struct{ name, ns string }{
		{"recovered-pod", "production"},
		{"api-server-def456", "production"},
		{"ambiguous-pod", "production"},
		{"failing-pod", "production"},
		{"failed-analysis-pod", "production"},
		{"test-pod", "default"},
	}
	for _, p := range pods {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: p.name, Namespace: p.ns,
				Labels: map[string]string{"app": p.name},
			},
			Spec: corev1.PodSpec{
				RestartPolicy: corev1.RestartPolicyNever,
				Containers:    []corev1.Container{pauseContainer},
			},
		}
		Expect(c.Create(ctx, pod)).To(Succeed())
	}

	// PVC/batch-job-pvc-expired/production — not_actionable scenario
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "batch-job-pvc-expired", Namespace: "production"},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Mi")},
			},
		},
	}
	Expect(c.Create(ctx, pvc)).To(Succeed())
}

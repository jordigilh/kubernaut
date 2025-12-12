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
// Infrastructure (AUTO-STARTED in SynchronizedBeforeSuite):
// - PostgreSQL (port 15434): Persistence layer
// - Redis (port 16380): Caching layer
// - Data Storage API (port 18091): Audit trail
// - HolmesGPT API (port 18120): AI analysis service (MOCK_LLM_MODE=true)
// - All services started via podman-compose programmatically
package aianalysis

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
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
	hgclient "github.com/jordigilh/kubernaut/pkg/aianalysis/client"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/rego"
	"github.com/jordigilh/kubernaut/pkg/testutil"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

var (
	ctx        context.Context
	cancel     context.CancelFunc
	testEnv    *envtest.Environment
	cfg        *rest.Config
	k8sClient  client.Client
	k8sManager ctrl.Manager

	// Mock dependencies for integration tests
	mockHGClient *testutil.MockHolmesGPTClient
)

func TestAIAnalysisIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AIAnalysis Controller Integration Suite (Envtest)")
}

// SynchronizedBeforeSuite runs ONCE globally before all parallel processes start
// This follows Gateway/Notification pattern for automated infrastructure startup
var _ = SynchronizedBeforeSuite(func() []byte {
	// This runs ONCE on process 1 only - creates shared infrastructure
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	GinkgoWriter.Println("AIAnalysis Integration Test Suite - Automated Setup")
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	GinkgoWriter.Println("Creating test infrastructure...")
	GinkgoWriter.Println("  • envtest (in-memory K8s API server)")
	GinkgoWriter.Println("  • PostgreSQL + pgvector (port 15434)")
	GinkgoWriter.Println("  • Redis (port 16380)")
	GinkgoWriter.Println("  • Data Storage API (port 18091)")
	GinkgoWriter.Println("  • HolmesGPT API (port 18120, MOCK_LLM_MODE=true)")
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	ctx, cancel = context.WithCancel(context.TODO())

	By("Starting AIAnalysis integration infrastructure (podman-compose)")
	// This starts: PostgreSQL, Redis, DataStorage, HolmesGPT-API
	// Per DD-TEST-001: Ports 15434, 16380, 18091, 18120
	err := infrastructure.StartAIAnalysisIntegrationInfrastructure(GinkgoWriter)
	Expect(err).ToNot(HaveOccurred(), "Infrastructure must start successfully")
	GinkgoWriter.Println("✅ All services started and healthy")

	By("Registering AIAnalysis CRD scheme")
	err = aianalysisv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	By("Bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	// Retrieve the first found binary directory to allow running tests from IDEs
	if getFirstFoundEnvTestBinaryDir() != "" {
		testEnv.BinaryAssetsDirectory = getFirstFoundEnvTestBinaryDir()
	}

	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	By("Creating controller-runtime client")
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	By("Creating required namespaces")
	// Create kubernaut-system namespace for controller
	systemNs := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kubernaut-system",
		},
	}
	err = k8sClient.Create(ctx, systemNs)
	Expect(err).NotTo(HaveOccurred())

	// Create default namespace for tests
	defaultNs := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
		},
	}
	_ = k8sClient.Create(ctx, defaultNs) // May already exist

	GinkgoWriter.Println("✅ Namespaces created: kubernaut-system, default")

	By("Setting up mock dependencies")
	// Create mock HolmesGPT client with default success response including workflow
	// (Without workflow + high confidence triggers "Problem Resolved" path per BR-HAPI-200)
	mockHGClient = testutil.NewMockHolmesGPTClient()
	mockHGClient.WithFullResponse(
		"Integration test: Issue identified and workflow selected",
		0.85,
		true, // targetInOwnerChain
		[]string{},
		nil, // No RCA for basic tests
		&hgclient.SelectedWorkflow{
			WorkflowID:     "wf-restart-pod",
			ContainerImage: "kubernaut/workflow-restart-pod:v1.0",
			Confidence:     0.85,
			Rationale:      "Restarts the target pod to recover from CrashLoopBackOff",
			Parameters: map[string]string{
				"TARGET_POD":       "test-pod",
				"TARGET_NAMESPACE": "default",
			},
		},
		nil, // No alternatives for basic tests
	)

	// Create mock Rego evaluator that auto-approves staging, requires approval for production
	mockRegoEvaluator := &MockRegoEvaluator{}

	By("Setting up the controller manager")
	k8sManager, err = ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0", // Use random port to avoid conflicts in parallel tests
		},
	})
	Expect(err).ToNot(HaveOccurred())

	By("Setting up the AIAnalysis controller with handlers")
	// Create handlers with mock dependencies
	investigatingHandler := handlers.NewInvestigatingHandler(mockHGClient, ctrl.Log.WithName("investigating-handler"))
	analyzingHandler := handlers.NewAnalyzingHandler(mockRegoEvaluator, ctrl.Log.WithName("analyzing-handler"))

	// Create controller with wired handlers
	err = (&aianalysis.AIAnalysisReconciler{
		Client:               k8sManager.GetClient(),
		Scheme:               k8sManager.GetScheme(),
		Recorder:             k8sManager.GetEventRecorderFor("aianalysis-controller"),
		Log:                  ctrl.Log.WithName("aianalysis-controller"),
		InvestigatingHandler: investigatingHandler,
		AnalyzingHandler:     analyzingHandler,
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	By("Starting the controller manager")
	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()

	// Wait for manager to be ready
	time.Sleep(2 * time.Second)

	GinkgoWriter.Println("✅ AIAnalysis integration test environment ready!")
	GinkgoWriter.Println("")
	GinkgoWriter.Println("Environment:")
	GinkgoWriter.Println("  • ENVTEST with real Kubernetes API (etcd + kube-apiserver)")
	GinkgoWriter.Println("  • AIAnalysis CRD installed")
	GinkgoWriter.Println("  • AIAnalysis controller running with mock HolmesGPT client")
	GinkgoWriter.Println("  • Mock Rego evaluator (staging=auto-approve, production=manual)")
	GinkgoWriter.Println("  • REAL services available for recovery_integration_test.go:")
	GinkgoWriter.Println("    - PostgreSQL: localhost:15434")
	GinkgoWriter.Println("    - Redis: localhost:16380")
	GinkgoWriter.Println("    - Data Storage: http://localhost:18091")
	GinkgoWriter.Println("    - HolmesGPT API: http://localhost:18120 (mock LLM)")
	GinkgoWriter.Println("")

	// Serialize REST config to pass to all processes
	// Each process needs to create its own k8s client
	configBytes, err := json.Marshal(struct {
		Host     string
		CAData   []byte
		CertData []byte
		KeyData  []byte
	}{
		Host:     cfg.Host,
		CAData:   cfg.CAData,
		CertData: cfg.CertData,
		KeyData:  cfg.KeyData,
	})
	Expect(err).NotTo(HaveOccurred())

	return configBytes
}, func(data []byte) {
	// This runs on ALL parallel processes (including process 1)
	// Each process creates its own k8s client and context
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	// Deserialize REST config from process 1
	var configData struct {
		Host     string
		CAData   []byte
		CertData []byte
		KeyData  []byte
	}
	err := json.Unmarshal(data, &configData)
	Expect(err).NotTo(HaveOccurred())

	// Register AIAnalysis CRD scheme (MUST be done before creating client)
	err = aianalysisv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// Create per-process REST config
	cfg = &rest.Config{
		Host: configData.Host,
		TLSClientConfig: rest.TLSClientConfig{
			CAData:   configData.CAData,
			CertData: configData.CertData,
			KeyData:  configData.KeyData,
		},
	}

	// Create per-process k8s client
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	// Create per-process context ONLY if not already set (process 1 has it from first function)
	// Process 1: ctx already set and used by controller manager - don't overwrite!
	// Processes 2-4: Need ctx for test operations
	if ctx == nil {
		ctx, cancel = context.WithCancel(context.Background())
	}

	// Create per-process mock client (each process gets its own mock)
	mockHGClient = testutil.NewMockHolmesGPTClient()
	mockHGClient.WithFullResponse(
		"Integration test: Issue identified and workflow selected",
		0.85,
		true,
		[]string{},
		nil,
		&hgclient.SelectedWorkflow{
			WorkflowID:     "wf-restart-pod",
			ContainerImage: "kubernaut/workflow-restart-pod:v1.0",
			Confidence:     0.85,
			Rationale:      "Restarts the target pod to recover from CrashLoopBackOff",
			Parameters: map[string]string{
				"TARGET_POD":       "test-pod",
				"TARGET_NAMESPACE": "default",
			},
		},
		nil,
	)
})

// SynchronizedAfterSuite ensures proper cleanup in parallel execution
var _ = SynchronizedAfterSuite(func() {
	// This runs on ALL parallel processes - no cleanup needed per process
}, func() {
	// This runs ONCE on the last parallel process - cleanup shared infrastructure
	By("Tearing down the test environment")
	cancel()

	if testEnv != nil {
		err := testEnv.Stop()
		Expect(err).NotTo(HaveOccurred())
	}

	By("Stopping AIAnalysis integration infrastructure")
	err := infrastructure.StopAIAnalysisIntegrationInfrastructure(GinkgoWriter)
	Expect(err).NotTo(HaveOccurred())

	GinkgoWriter.Println("✅ Cleanup complete - all services stopped")
})

// getFirstFoundEnvTestBinaryDir locates the first binary in the specified path.
func getFirstFoundEnvTestBinaryDir() string {
	basePath := filepath.Join("..", "..", "..", "bin", "k8s")
	entries, err := os.ReadDir(basePath)
	if err != nil {
		logf.Log.Error(err, "Failed to read directory", "path", basePath)
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return filepath.Join(basePath, entry.Name())
		}
	}
	return ""
}

// MockRegoEvaluator is a simple mock for integration tests.
// It approves staging environments automatically, requires approval for production
// and for recovery attempts with multiple retries (escalation).
type MockRegoEvaluator struct{}

func (m *MockRegoEvaluator) Evaluate(ctx context.Context, input *rego.PolicyInput) (*rego.PolicyResult, error) {
	// Check for recovery escalation (recovery attempt >= 3 always requires approval)
	if input.RecoveryAttemptNumber >= 3 {
		return &rego.PolicyResult{
			ApprovalRequired: true,
			Reason:           "Multiple recovery attempts require manual approval",
		}, nil
	}

	// Simple policy: staging auto-approves, production requires approval
	if input.Environment == "staging" || input.Environment == "dev" {
		return &rego.PolicyResult{
			ApprovalRequired: false,
			Reason:           "Auto-approved for non-production environment",
		}, nil
	}

	// Production or unknown environments require approval
	return &rego.PolicyResult{
		ApprovalRequired: true,
		Reason:           "Production environment requires manual approval",
	}, nil
}

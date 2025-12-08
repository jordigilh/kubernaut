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
// Test Strategy: envtest with mock HolmesGPT-API client (per ADR-004)
// Defense-in-Depth (per 03-testing-strategy.mdc):
// - Unit tests (70%+): Mock K8s client + mock HAPI
// - Integration tests (>50%): Real K8s API (envtest) + mock HAPI
// - E2E tests (10-15%): Real K8s API (KIND) + real HAPI
package aianalysis

import (
	"context"
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

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	By("Registering AIAnalysis CRD scheme")
	err := aianalysisv1alpha1.AddToScheme(scheme.Scheme)
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
	GinkgoWriter.Println("")
})

var _ = AfterSuite(func() {
	By("Tearing down the test environment")
	cancel()

	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())

	GinkgoWriter.Println("✅ Cleanup complete")
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

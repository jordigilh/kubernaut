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

// Package remediationorchestrator_test contains integration tests for the RemediationOrchestrator controller.
// These tests use ENVTEST with real Kubernetes API (etcd + kube-apiserver).
//
// Defense-in-Depth Strategy (per 03-testing-strategy.mdc):
// - Unit tests (70%+): Business logic in isolation (test/unit/remediationorchestrator/)
// - Integration tests (>50%): Infrastructure interaction, microservices coordination (this file)
// - E2E tests (10-15%): Complete workflow validation (test/e2e/remediationorchestrator/)
//
// Test Execution (parallel, 4 procs):
//
//	ginkgo -p --procs=4 ./test/integration/remediationorchestrator/...
//
// MANDATORY: All tests use unique namespaces for parallel execution isolation.
package remediationorchestrator_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	// Import ALL CRD types that RO interacts with
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"

	// Import RO controller
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/controller"
)

// Test constants for timeout and polling intervals
const (
	timeout  = 60 * time.Second // Longer timeout for RO orchestration
	interval = 250 * time.Millisecond
)

// Package-level variables for test environment
var (
	ctx        context.Context
	cancel     context.CancelFunc
	testEnv    *envtest.Environment
	cfg        *rest.Config
	k8sClient  client.Client
	k8sManager ctrl.Manager
)

func TestRemediationOrchestratorIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "RemediationOrchestrator Controller Integration Suite (ENVTEST)")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	By("Registering ALL CRD schemes for RO orchestration")

	// RemediationRequest and RemediationApprovalRequest (RO owns these)
	err := remediationv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// SignalProcessing (RO creates these)
	err = signalprocessingv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// AIAnalysis (RO creates these)
	err = aianalysisv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// WorkflowExecution (RO creates these)
	err = workflowexecutionv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// NotificationRequest (RO creates these)
	err = notificationv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	By("Bootstrapping test environment with ALL CRDs")
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

	By("Creating namespaces for testing")
	// Create kubernaut-system namespace for controller
	systemNs := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kubernaut-system",
		},
	}
	err = k8sClient.Create(ctx, systemNs)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		Expect(err).NotTo(HaveOccurred())
	}

	// Create default namespace for tests
	defaultNs := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
		},
	}
	_ = k8sClient.Create(ctx, defaultNs) // May already exist

	GinkgoWriter.Println("✅ Namespaces created: kubernaut-system, default")

	By("Setting up the controller manager")
	k8sManager, err = ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0", // Use random port to avoid conflicts in parallel tests
		},
	})
	Expect(err).ToNot(HaveOccurred())

	By("Setting up the RemediationOrchestrator controller")
	// Create RO reconciler with manager client and scheme
	// Audit store is nil for tests (DD-AUDIT-003 compliant - audit is optional for testing)
	reconciler := controller.NewReconciler(
		k8sManager.GetClient(),
		k8sManager.GetScheme(),
		nil, // No audit store for integration tests
	)
	err = reconciler.SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	By("Starting the controller manager")
	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()

	// Wait for manager to be ready
	time.Sleep(2 * time.Second)

	GinkgoWriter.Println("✅ RemediationOrchestrator integration test environment ready!")
	GinkgoWriter.Println("")
	GinkgoWriter.Println("Environment:")
	GinkgoWriter.Println("  • ENVTEST with real Kubernetes API (etcd + kube-apiserver)")
	GinkgoWriter.Println("  • ALL CRDs installed:")
	GinkgoWriter.Println("    - RemediationRequest")
	GinkgoWriter.Println("    - RemediationApprovalRequest")
	GinkgoWriter.Println("    - SignalProcessing")
	GinkgoWriter.Println("    - AIAnalysis")
	GinkgoWriter.Println("    - WorkflowExecution")
	GinkgoWriter.Println("    - NotificationRequest")
	GinkgoWriter.Println("  • RemediationOrchestrator controller running")
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
// ENVTEST-based tests depend on specific binaries, usually located in paths set by
// controller-runtime. When running tests directly (e.g., via an IDE) without using
// Makefile targets, the 'BinaryAssetsDirectory' must be explicitly configured.
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

// ============================================================================
// TEST HELPER FUNCTIONS (for parallel execution isolation)
// ============================================================================

// createTestNamespace creates a unique namespace for test isolation in parallel execution.
// MANDATORY per 03-testing-strategy.mdc: Each test must use unique identifiers.
func createTestNamespace(prefix string) string {
	ns := fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: ns,
		},
	}
	Expect(k8sClient.Create(ctx, namespace)).To(Succeed())
	GinkgoWriter.Printf("✅ Created test namespace: %s\n", ns)
	return ns
}

// deleteTestNamespace cleans up a test namespace.
// Called in defer to ensure cleanup even on test failure.
func deleteTestNamespace(ns string) {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: ns,
		},
	}
	err := k8sClient.Delete(ctx, namespace)
	if err != nil && !apierrors.IsNotFound(err) {
		GinkgoWriter.Printf("⚠️ Warning: Failed to delete namespace %s: %v\n", ns, err)
	} else {
		GinkgoWriter.Printf("✅ Deleted test namespace: %s\n", ns)
	}
}

// waitForRRPhase waits for a RemediationRequest to reach a specific phase.
func waitForRRPhase(name, namespace, expectedPhase string, timeout time.Duration) error {
	return wait.PollImmediate(interval, timeout, func() (bool, error) {
		rr := &remediationv1.RemediationRequest{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, rr)
		if err != nil {
			return false, err
		}
		return rr.Status.OverallPhase == expectedPhase, nil
	})
}

// waitForChildCRD waits for a child CRD to be created by RO.
func waitForChildCRD(name, namespace string, obj client.Object, timeout time.Duration) error {
	return wait.PollImmediate(interval, timeout, func() (bool, error) {
		err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, obj)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil // Keep waiting
			}
			return false, err
		}
		return true, nil
	})
}

// createRemediationRequest creates a RemediationRequest for testing.
func createRemediationRequest(namespace, name string) *remediationv1.RemediationRequest {
	now := metav1.Now()
	rr := &remediationv1.RemediationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: remediationv1.RemediationRequestSpec{
			// Valid 64-char hex fingerprint (SHA256 format per CRD validation)
			SignalFingerprint: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
			SignalName:        "TestHighMemoryAlert",
			Severity:          "critical",
			SignalType:        "prometheus",
			TargetType:        "kubernetes",
			TargetResource: remediationv1.ResourceIdentifier{
				Kind:      "Deployment",
				Name:      "test-app",
				Namespace: namespace,
			},
			FiringTime:   now,
			ReceivedTime: now,
			Deduplication: sharedtypes.DeduplicationInfo{
				FirstOccurrence: now,
				LastOccurrence:  now,
				OccurrenceCount: 1,
			},
		},
	}
	Expect(k8sClient.Create(ctx, rr)).To(Succeed())
	GinkgoWriter.Printf("✅ Created RemediationRequest: %s/%s\n", namespace, name)
	return rr
}

// updateAIAnalysisStatus updates the AIAnalysis status to simulate completion.
func updateAIAnalysisStatus(namespace, name string, phase string, workflow *aianalysisv1.SelectedWorkflow) error {
	ai := &aianalysisv1.AIAnalysis{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, ai); err != nil {
		return err
	}

	ai.Status.Phase = phase
	ai.Status.SelectedWorkflow = workflow
	if phase == "Completed" {
		now := metav1.Now()
		ai.Status.CompletedAt = &now
	}

	return k8sClient.Status().Update(ctx, ai)
}

// updateSPStatus updates the SignalProcessing status to simulate completion.
func updateSPStatus(namespace, name string, phase signalprocessingv1.SignalProcessingPhase) error {
	sp := &signalprocessingv1.SignalProcessing{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, sp); err != nil {
		return err
	}

	sp.Status.Phase = phase
	if phase == signalprocessingv1.PhaseCompleted {
		now := metav1.Now()
		sp.Status.CompletionTime = &now
		// Set environment classification for downstream use
		sp.Status.EnvironmentClassification = &signalprocessingv1.EnvironmentClassification{
			Environment: "production",
			Confidence:  0.95,
		}
		sp.Status.PriorityAssignment = &signalprocessingv1.PriorityAssignment{
			Priority:   "P1",
			Confidence: 0.90,
		}
	}

	return k8sClient.Status().Update(ctx, sp)
}


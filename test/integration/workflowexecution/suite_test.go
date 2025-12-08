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

package workflowexecution

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sync"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/utils/clock"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"knative.dev/pkg/apis"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	workflowexecution "github.com/jordigilh/kubernaut/internal/controller/workflowexecution"
	"github.com/jordigilh/kubernaut/pkg/audit"
	// +kubebuilder:scaffold:imports
)

// WorkflowExecution Integration Test Suite
//
// Defense-in-Depth Strategy (per 03-testing-strategy.mdc):
// - Unit tests (70%+): Business logic in isolation - 71.7% achieved
// - Integration tests (>50%): Controller reconciliation with real K8s API + Tekton CRDs
// - E2E tests (10-15%): Complete workflow validation with Kind + Tekton controller
//
// Integration tests focus on (EnvTest WITH Tekton CRDs):
// - Controller reconciliation lifecycle
// - PipelineRun creation and status sync
// - Resource locking during reconciliation
// - Cooldown enforcement
// - Exponential backoff state tracking
// - Cross-namespace coordination
//
// V2.0 COMPLIANCE UPDATE (2025-12-07):
// - Controller IS started in EnvTest integration tests
// - Tekton CRDs ARE installed (config/crd/tekton/)
// - Full controller behavior tested (not just CRD persistence)
// - Complies with testing-strategy.md >50% integration coverage requirement

var (
	ctx            context.Context
	cancel         context.CancelFunc
	testEnv        *envtest.Environment
	cfg            *rest.Config
	k8sClient      client.Client
	k8sManager     ctrl.Manager
	testAuditStore *testableAuditStore
)

// testableAuditStore is a thread-safe audit store for integration testing
// Implements audit.AuditStore interface
type testableAuditStore struct {
	mu     sync.Mutex
	events []audit.AuditEvent
}

func newTestableAuditStore() *testableAuditStore {
	return &testableAuditStore{
		events: make([]audit.AuditEvent, 0),
	}
}

// StoreAudit implements audit.AuditStore
func (s *testableAuditStore) StoreAudit(ctx context.Context, event *audit.AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if event != nil {
		s.events = append(s.events, *event)
	}
	return nil
}

// Close implements audit.AuditStore
func (s *testableAuditStore) Close() error {
	return nil
}

// GetEvents returns a copy of all captured audit events (test helper)
func (s *testableAuditStore) GetEvents() []audit.AuditEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Return a copy to avoid race conditions
	result := make([]audit.AuditEvent, len(s.events))
	copy(result, s.events)
	return result
}

// ClearEvents clears all captured events (test helper)
func (s *testableAuditStore) ClearEvents() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = make([]audit.AuditEvent, 0)
}

// EventCount returns the number of captured events (test helper)
func (s *testableAuditStore) EventCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.events)
}

// GetEventsOfType returns events matching the given type (test helper)
func (s *testableAuditStore) GetEventsOfType(eventType string) []audit.AuditEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []audit.AuditEvent
	for _, e := range s.events {
		if e.EventType == eventType {
			result = append(result, e)
		}
	}
	return result
}

// Test namespaces (unique per test run for parallel safety)
const (
	DefaultNamespace          = "default"
	WorkflowExecutionNS       = "kubernaut-workflows"
	IntegrationTestNamePrefix = "int-test-"

	// Controller configuration
	DefaultCooldownPeriod         = 5 * time.Minute
	DefaultBaseCooldownPeriod     = 1 * time.Minute
	DefaultMaxCooldownPeriod      = 10 * time.Minute
	DefaultMaxConsecutiveFailures = 5
)

// testClock is a real clock for integration tests
var testClock clock.Clock = clock.RealClock{}

func TestWorkflowExecutionIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "WorkflowExecution Controller Integration Suite (EnvTest + Tekton CRDs)")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	By("Registering CRD schemes")
	err := workflowexecutionv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = tektonv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	By("Bootstrapping test environment with WorkflowExecution AND Tekton CRDs")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "config", "crd", "bases"),
			filepath.Join("..", "..", "..", "config", "crd", "tekton"), // Tekton CRDs for PipelineRun
		},
		ErrorIfCRDPathMissing: true, // FAIL if CRDs not found - no silent fallback
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
	// Create kubernaut-workflows namespace for PipelineRuns
	workflowsNs := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: WorkflowExecutionNS,
		},
	}
	err = k8sClient.Create(ctx, workflowsNs)
	Expect(err).NotTo(HaveOccurred())

	// Create default namespace for tests
	defaultNs := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: DefaultNamespace,
		},
	}
	_ = k8sClient.Create(ctx, defaultNs) // May already exist

	GinkgoWriter.Println("✅ Namespaces created: kubernaut-workflows, default")

	By("Setting up the controller manager")
	k8sManager, err = ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0", // Use random port to avoid conflicts in parallel tests
		},
	})
	Expect(err).ToNot(HaveOccurred())

	By("Setting up the testable audit store")
	testAuditStore = newTestableAuditStore()

	By("Setting up the WorkflowExecution controller")
	// Create controller with integration test configuration
	reconciler := &workflowexecution.WorkflowExecutionReconciler{
		Client:                 k8sManager.GetClient(),
		Scheme:                 k8sManager.GetScheme(),
		Recorder:               k8sManager.GetEventRecorderFor("workflowexecution-controller"),
		ExecutionNamespace:     WorkflowExecutionNS,
		ServiceAccountName:     "kubernaut-workflow-runner",
		CooldownPeriod:         DefaultCooldownPeriod,
		BaseCooldownPeriod:     DefaultBaseCooldownPeriod,
		MaxCooldownPeriod:      DefaultMaxCooldownPeriod,
		MaxConsecutiveFailures: DefaultMaxConsecutiveFailures,
		AuditStore:             testAuditStore, // Enable audit tracking for integration tests
	}
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

	GinkgoWriter.Println("✅ WorkflowExecution integration test environment ready!")
	GinkgoWriter.Println("")
	GinkgoWriter.Println("Environment:")
	GinkgoWriter.Println("  • EnvTest with real Kubernetes API (etcd + kube-apiserver)")
	GinkgoWriter.Println("  • WorkflowExecution CRD installed")
	GinkgoWriter.Println("  • Tekton CRDs installed (PipelineRun, TaskRun, etc.)")
	GinkgoWriter.Println("  • WorkflowExecution controller RUNNING")
	GinkgoWriter.Println("  • Tests: Full controller reconciliation, status sync, resource locking")
	GinkgoWriter.Println("")
	GinkgoWriter.Println("V2.0 COMPLIANCE: Integration tests now exercise controller reconciliation")
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

// ========================================
// Test Helpers - Parallel-Safe (4 procs)
// ========================================

// createUniqueWFE creates a WorkflowExecution with unique name for parallel test isolation
func createUniqueWFE(testID, targetResource string) *workflowexecutionv1alpha1.WorkflowExecution {
	name := IntegrationTestNamePrefix + testID + "-" + time.Now().Format("150405000")
	return &workflowexecutionv1alpha1.WorkflowExecution{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: DefaultNamespace,
		},
		Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
			RemediationRequestRef: corev1.ObjectReference{
				APIVersion: "remediation.kubernaut.ai/v1alpha1",
				Kind:       "RemediationRequest",
				Name:       "test-rr-" + testID,
				Namespace:  DefaultNamespace,
			},
			WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
				WorkflowID:     "test-workflow",
				Version:        "v1.0.0",
				ContainerImage: "ghcr.io/kubernaut/workflows/test@sha256:abc123",
			},
			TargetResource: targetResource,
		},
	}
}

// createUniqueWFEWithParams creates a WorkflowExecution with parameters
func createUniqueWFEWithParams(testID, targetResource string, params map[string]string) *workflowexecutionv1alpha1.WorkflowExecution {
	wfe := createUniqueWFE(testID, targetResource)
	wfe.Spec.Parameters = params
	return wfe
}

// getWFE gets a WorkflowExecution by name
func getWFE(name, namespace string) (*workflowexecutionv1alpha1.WorkflowExecution, error) {
	wfe := &workflowexecutionv1alpha1.WorkflowExecution{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, wfe)
	return wfe, err
}

// waitForWFEPhase waits for a WorkflowExecution to reach a specific phase
func waitForWFEPhase(name, namespace string, expectedPhase string, timeout time.Duration) (*workflowexecutionv1alpha1.WorkflowExecution, error) {
	var wfe *workflowexecutionv1alpha1.WorkflowExecution

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	err := wait.PollUntilContextTimeout(timeoutCtx, 100*time.Millisecond, timeout, true, func(ctx context.Context) (bool, error) {
		var err error
		wfe, err = getWFE(name, namespace)
		if err != nil {
			return false, nil // Keep waiting on error
		}
		return wfe.Status.Phase == expectedPhase, nil
	})

	return wfe, err
}

// waitForPipelineRunCreation waits for a PipelineRun to be created for a WFE
func waitForPipelineRunCreation(wfeName, wfeNamespace string, timeout time.Duration) (*tektonv1.PipelineRun, error) {
	var pr *tektonv1.PipelineRun

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	err := wait.PollUntilContextTimeout(timeoutCtx, 100*time.Millisecond, timeout, true, func(ctx context.Context) (bool, error) {
		// List PipelineRuns with the WFE label
		prList := &tektonv1.PipelineRunList{}
		err := k8sClient.List(ctx, prList, client.InNamespace(WorkflowExecutionNS), client.MatchingLabels{
			"kubernaut.ai/workflow-execution": wfeName,
		})
		if err != nil {
			return false, nil
		}
		if len(prList.Items) > 0 {
			pr = &prList.Items[0]
			return true, nil
		}
		return false, nil
	})

	return pr, err
}

// simulatePipelineRunCompletion updates a PipelineRun to simulate completion
func simulatePipelineRunCompletion(pr *tektonv1.PipelineRun, succeeded bool) error {
	pr.Status.InitializeConditions(testClock)
	if succeeded {
		pr.Status.SetCondition(&apis.Condition{
			Type:    apis.ConditionSucceeded,
			Status:  corev1.ConditionTrue,
			Reason:  "Completed",
			Message: "PipelineRun completed successfully",
		})
	} else {
		pr.Status.SetCondition(&apis.Condition{
			Type:    apis.ConditionSucceeded,
			Status:  corev1.ConditionFalse,
			Reason:  "Failed",
			Message: "PipelineRun failed",
		})
	}
	return k8sClient.Status().Update(ctx, pr)
}

// deleteWFEAndWait deletes a WorkflowExecution and waits for it to be fully removed
func deleteWFEAndWait(wfe *workflowexecutionv1alpha1.WorkflowExecution, timeout time.Duration) error {
	if err := k8sClient.Delete(ctx, wfe); err != nil {
		return err
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return wait.PollUntilContextTimeout(timeoutCtx, 100*time.Millisecond, timeout, true, func(ctx context.Context) (bool, error) {
		err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      wfe.Name,
			Namespace: wfe.Namespace,
		}, &workflowexecutionv1alpha1.WorkflowExecution{})

		if err != nil {
			// Object not found = deletion complete
			return true, nil
		}

		// Still exists, keep waiting
		return false, nil
	})
}

// getPipelineRun gets a PipelineRun by name
func getPipelineRun(name, namespace string) (*tektonv1.PipelineRun, error) {
	pr := &tektonv1.PipelineRun{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, pr)
	return pr, err
}

// cleanupWFE cleans up a WFE and its associated PipelineRun
func cleanupWFE(wfe *workflowexecutionv1alpha1.WorkflowExecution) {
	// Delete WFE (will cascade to PipelineRun via owner reference)
	_ = k8sClient.Delete(ctx, wfe)

	// Also directly clean up any PipelineRuns
	prList := &tektonv1.PipelineRunList{}
	if err := k8sClient.List(ctx, prList, client.InNamespace(WorkflowExecutionNS), client.MatchingLabels{
		"kubernaut.ai/workflow-execution": wfe.Name,
	}); err == nil {
		for _, pr := range prList.Items {
			_ = k8sClient.Delete(ctx, &pr)
		}
	}
}

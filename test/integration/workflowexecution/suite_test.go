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
	"fmt"
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

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/utils/clock"
	"knative.dev/pkg/apis"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	workflowexecution "github.com/jordigilh/kubernaut/internal/controller/workflowexecution"
	"github.com/jordigilh/kubernaut/pkg/audit"
	weaudit "github.com/jordigilh/kubernaut/pkg/workflowexecution/audit"
	wemetrics "github.com/jordigilh/kubernaut/pkg/workflowexecution/metrics"
	westatus "github.com/jordigilh/kubernaut/pkg/workflowexecution/status"
	"github.com/jordigilh/kubernaut/test/infrastructure" // Shared infrastructure (PostgreSQL + Redis + DS)
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
	"github.com/prometheus/client_golang/prometheus"
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
	ctx                context.Context
	cancel             context.CancelFunc
	testEnv            *envtest.Environment
	cfg                *rest.Config
	k8sClient          client.Client
	k8sManager         ctrl.Manager
	dataStorageBaseURL string                                         = fmt.Sprintf("http://127.0.0.1:%d", infrastructure.WEIntegrationDataStoragePort) // WE integration port (IPv4 explicit for CI, DD-TEST-001)
	auditStore         audit.AuditStore                                                                                                                 // REAL audit store (DD-AUDIT-003 compliance)
	reconciler         *workflowexecution.WorkflowExecutionReconciler                                                                                   // Controller instance for metrics access
)

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

var _ = SynchronizedBeforeSuite(func() []byte {
	// Phase 1: Runs ONCE on parallel process #1
	// Start integration infrastructure (PostgreSQL, Redis, DataStorage)
	// This runs once to avoid container name collisions when TEST_PROCS > 1
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	By("Starting WorkflowExecution integration infrastructure (Process #1 only, DD-TEST-002)")
	// Use shared DSBootstrap infrastructure (replaces custom StartWEIntegrationInfrastructure)
	// Per DD-TEST-001 v2.6: WE uses PostgreSQL=15441, Redis=16388, DS=18097
	dsInfra, err := infrastructure.StartDSBootstrap(infrastructure.DSBootstrapConfig{
		ServiceName:     "workflowexecution",
		PostgresPort:    15441, // DD-TEST-001 v2.2
		RedisPort:       16388, // DD-TEST-001 v2.2 (unique, resolved conflict with HAPI)
		DataStoragePort: 18097, // DD-TEST-001 v2.2
		MetricsPort:     19097,
		ConfigDir:       "test/integration/workflowexecution/config",
	}, GinkgoWriter)
	Expect(err).ToNot(HaveOccurred(), "Infrastructure must start successfully")
	GinkgoWriter.Println("✅ All services started and healthy (PostgreSQL, Redis, DataStorage - shared across all processes)")

	// Clean up infrastructure on exit
	DeferCleanup(func() {
		_ = infrastructure.StopDSBootstrap(dsInfra, GinkgoWriter)
	})

	return []byte{} // No data to share between processes
}, func(data []byte) {
	// Phase 2: Runs on ALL parallel processes (receives data from phase 1)
	// Set up envtest, K8s client, and controller manager per process
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	var err error

	By("Registering CRD schemes")
	err = workflowexecutionv1alpha1.AddToScheme(scheme.Scheme)
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
	// KUBEBUILDER_ASSETS is set by Makefile via setup-envtest dependency

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

	// Note: DataStorage health check already done by StartWEIntegrationInfrastructure()
	GinkgoWriter.Printf("✅ DataStorage confirmed healthy at %s\n", dataStorageBaseURL)

	By("Creating REAL audit store with DataStorage OpenAPI client (DD-API-001)")
	// Create OpenAPI DataStorage client (DD-API-001 compliant)
	// DD-AUTH-005: Integration tests use mock user transport (no oauth-proxy)
	mockTransport := testauth.NewMockUserTransport("test-workflowexecution@integration.test")
	dsClient, err := audit.NewOpenAPIClientAdapterWithTransport(
		dataStorageBaseURL,
		5*time.Second,
		mockTransport, // ← Mock user header injection (simulates oauth-proxy)
	)
	if err != nil {
		Fail(fmt.Sprintf("Failed to create OpenAPI DataStorage client: %v", err))
	}

	// Create REAL buffered audit store (Defense-in-Depth Layer 4)
	auditStore, err = audit.NewBufferedStore(
		dsClient,
		audit.DefaultConfig(),
		"workflowexecution-controller",
		ctrl.Log.WithName("audit"),
	)
	Expect(err).ToNot(HaveOccurred(), "Failed to create real audit store")

	GinkgoWriter.Println("✅ Real audit store created (connected to DataStorage)")

	By("Creating metrics with test registry for isolation (DD-METRICS-001)")
	// Create isolated Prometheus registry for integration tests to prevent conflicts
	testRegistry := prometheus.NewRegistry()
	testMetrics := wemetrics.NewMetricsWithRegistry(testRegistry)
	GinkgoWriter.Println("✅ Test metrics created with isolated registry")

	By("Setting up the WorkflowExecution controller with REAL audit store")
	// Integration tests validate audit traces are properly stored (Defense-in-Depth)
	// E2E tests validate audit client wiring (simpler smoke test)

	// Initialize status manager (DD-PERF-001)
	statusManager := westatus.NewManager(k8sManager.GetClient())

	// Initialize audit manager (P3: Audit Manager pattern)
	auditManager := weaudit.NewManager(auditStore, ctrl.Log.WithName("audit"))

	reconciler = &workflowexecution.WorkflowExecutionReconciler{
		Client:                 k8sManager.GetClient(),
		Scheme:                 k8sManager.GetScheme(),
		Recorder:               k8sManager.GetEventRecorderFor("workflowexecution-controller"),
		ExecutionNamespace:     WorkflowExecutionNS,
		ServiceAccountName:     "kubernaut-workflow-runner",
		CooldownPeriod:         10 * time.Second, // Short cooldown for integration tests (default 5min too long)
		BaseCooldownPeriod:     DefaultBaseCooldownPeriod,
		MaxCooldownPeriod:      DefaultMaxCooldownPeriod,
		MaxConsecutiveFailures: DefaultMaxConsecutiveFailures,
		AuditStore:             auditStore,    // REAL audit store for integration tests
		Metrics:                testMetrics,   // Test-isolated metrics (DD-METRICS-001)
		StatusManager:          statusManager, // DD-PERF-001: Atomic status updates
		AuditManager:           auditManager,  // P3: Audit Manager pattern
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

	// Note: Metrics server uses dynamic port allocation (":0") to prevent conflicts
	// Port discovery is not exposed by controller-runtime Manager interface

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

var _ = SynchronizedAfterSuite(func() {
	// Phase 1: Runs on ALL parallel processes (per-process cleanup)
	By("Tearing down per-process test environment")

	// Close REAL audit store to flush remaining events (DD-AUDIT-003)
	// WE-SHUTDOWN-001: Flush audit store BEFORE stopping DataStorage
	// This prevents "connection refused" errors during cleanup when the
	// background writer tries to flush buffered events after DataStorage is stopped.
	// Integration tests MUST always use real DataStorage (DD-TESTING-001)
	By("Flushing audit store before infrastructure shutdown")
	flushCtx, flushCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer flushCancel()

	err := auditStore.Flush(flushCtx)
	if err != nil {
		GinkgoWriter.Printf("⚠️  Warning: Failed to flush audit store: %v\n", err)
	} else {
		GinkgoWriter.Println("✅ Audit store flushed (all buffered events written)")
	}

	By("Closing audit store")
	err = auditStore.Close()
	if err != nil {
		GinkgoWriter.Printf("⚠️  Warning: Failed to close audit store: %v\n", err)
	} else {
		GinkgoWriter.Println("✅ Audit store closed")
	}

	cancel()

	err = testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())

	GinkgoWriter.Println("✅ Per-process cleanup complete")
}, func() {
	// Phase 2: Runs ONCE on parallel process #1 (shared infrastructure cleanup)
	// Infrastructure cleanup handled by DeferCleanup (StopDSBootstrap)
	// WE-SHUTDOWN-001: Safe to stop now - all processes flushed audit events

	// DD-TEST-DIAGNOSTICS: Must-gather container logs for post-mortem analysis
	// ALWAYS collect logs - failures may have occurred on other parallel processes
	// The overhead is minimal (~2s) and logs are invaluable for debugging flaky tests
	GinkgoWriter.Println("📦 Collecting container logs for post-mortem analysis...")
	infrastructure.MustGatherContainerLogs("workflowexecution", []string{
		"workflowexecution_datastorage_test",
		"workflowexecution_postgres_test",
		"workflowexecution_redis_test",
	}, GinkgoWriter)

	GinkgoWriter.Println("✅ Shared infrastructure cleanup complete")
})
// ========================================
// Test Helpers - Parallel-Safe (4 procs)
// ========================================

// createUniqueWFE creates a WorkflowExecution with unique name for parallel test isolation
func createUniqueWFE(testID, targetResource string) *workflowexecutionv1alpha1.WorkflowExecution {
	name := IntegrationTestNamePrefix + testID + "-" + time.Now().Format("150405000")
	return &workflowexecutionv1alpha1.WorkflowExecution{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  DefaultNamespace,
			Generation: 1, // K8s increments on create/update
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
// flushAuditBuffer flushes the buffered audit store to ensure all events are written to DataStorage
// MANDATORY before querying audit events to prevent flaky tests due to buffering
func flushAuditBuffer() {
	flushCtx, flushCancel := context.WithTimeout(ctx, 5*time.Second)
	defer flushCancel()

	err := auditStore.Flush(flushCtx)
	if err != nil {
		GinkgoWriter.Printf("⚠️  Warning: Failed to flush audit buffer: %v\n", err)
	}
}

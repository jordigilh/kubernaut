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
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// WorkflowExecution E2E Test Suite
//
// Defense-in-Depth Strategy (per 03-testing-strategy.mdc):
// - Unit tests (70%+): Business logic in isolation
// - Integration tests (>50%): CRD operations with EnvTest
// - E2E tests (10-15%): Complete workflow validation with KIND + Tekton
//
// This suite tests:
// - BR-WE-001: Remediation completes within SLA
// - BR-WE-003: Monitor Execution Status (status sync)
// - BR-WE-004: Failure details actionable for recovery
// - BR-WE-005: Audit events for execution lifecycle
// - BR-WE-007: Handle externally deleted PipelineRun
// - BR-WE-008: Prometheus metrics for execution outcomes
// - BR-WE-009: Parallel execution prevention
// - BR-WE-010: Cooldown enforcement

func TestWorkflowExecutionE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "WorkflowExecution E2E Suite")
}

var (
	ctx    context.Context
	cancel context.CancelFunc
	logger logr.Logger

	// Cluster configuration
	clusterName    string
	kubeconfigPath string
	cfg            *rest.Config
	k8sClient      client.Client
	apiReader      client.Reader // Direct API reader to bypass client cache for Eventually() blocks

	// Controller namespace
	controllerNamespace string = infrastructure.WorkflowExecutionNamespace

	// DD-AUTH-014: ServiceAccount token for DataStorage authentication
	e2eAuthToken string

	// Track test failures
	anyTestFailed bool
)

// SynchronizedBeforeSuite runs cluster setup ONCE on process 1, then each process connects
var _ = SynchronizedBeforeSuite(
	// This runs ONCE on process 1 only - sets up shared cluster
	func() []byte {
		// Initialize context
		ctx, cancel = context.WithCancel(context.Background())

		// Initialize logger
		logger = kubelog.NewLogger(kubelog.Options{
			Development: true,
			Level:       0, // INFO
			ServiceName: "workflowexecution-e2e-test",
		})

		anyTestFailed = false

		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("WorkflowExecution E2E Test Suite - Cluster Setup (ONCE)")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("Creating Kind cluster with Tekton Pipelines...")
		logger.Info("  • Kind cluster (2 nodes: control-plane + worker)")
		logger.Info("  • Tekton Pipelines (for workflow execution)")
		logger.Info("  • WorkflowExecution CRD")
		logger.Info("  • WorkflowExecution Controller")
		logger.Info("  • Test pipeline for E2E validation")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Set cluster configuration
		clusterName = infrastructure.WorkflowExecutionClusterName
		homeDir, err := os.UserHomeDir()
		Expect(err).ToNot(HaveOccurred())
		// Standard kubeconfig location: ~/.kube/{service}-e2e-config
		// Per RO team kubeconfig standardization (REQUEST_WORKFLOWEXECUTION_KUBECONFIG_STANDARDIZATION.md)
		kubeconfigPath = fmt.Sprintf("%s/.kube/workflowexecution-e2e-config", homeDir)

		// Delete any existing cluster first (cleanup operation, no log export needed)
		logger.Info("Checking for existing cluster...")
		_ = infrastructure.DeleteWorkflowExecutionCluster(clusterName, false, GinkgoWriter)

		// Create Kind cluster with Tekton (using HYBRID PARALLEL infrastructure setup)
		// DD-TEST-002: Hybrid parallel approach (build parallel → cluster → load → deploy)
		// This is 4x faster AND 100% reliable (no cluster timeout issues)
		// Reference: test/infrastructure/gateway_e2e_hybrid.go (Gateway reference implementation)
		err = infrastructure.SetupWorkflowExecutionInfrastructureHybridWithCoverage(ctx, clusterName, kubeconfigPath, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())

		// Set KUBECONFIG environment variable
		err = os.Setenv("KUBECONFIG", kubeconfigPath)
		Expect(err).ToNot(HaveOccurred())

		// Note: WorkflowExecution Controller is already deployed by hybrid infrastructure setup
		// Wait for deployment to be available first (ensures ReplicaSet creates pod)
		// Phase 1 E2E Stabilization: Increased timeout to 1 hour (3600s) to prevent timeout failures
		// Root cause: Slow Tekton image pulls in Kind cluster (see WE_E2E_INFRASTRUCTURE_STABILIZATION_PLAN.md)
		logger.Info("⏳ Waiting for WorkflowExecution Controller deployment to be available (timeout: 1 hour)...")
		waitDeployCmd := exec.Command("kubectl", "wait",
			"-n", controllerNamespace,
			"--for=condition=available",
			"deployment/workflowexecution-controller",
			"--timeout=3600s",
			"--kubeconfig", kubeconfigPath)
		waitDeployCmd.Stdout = GinkgoWriter
		waitDeployCmd.Stderr = GinkgoWriter
		err = waitDeployCmd.Run()
		Expect(err).ToNot(HaveOccurred(), "WorkflowExecution Controller deployment did not become available")

		// Then wait for controller pod to be ready
		// Phase 1 E2E Stabilization: Increased timeout to 1 hour (3600s)
		logger.Info("⏳ Waiting for WorkflowExecution Controller pod to be ready (timeout: 1 hour)...")
		waitCmd := exec.Command("kubectl", "wait",
			"-n", controllerNamespace,
			"--for=condition=ready",
			"pod",
			"-l", "app=workflowexecution-controller",
			"--timeout=3600s",
			"--kubeconfig", kubeconfigPath)
		waitCmd.Stdout = GinkgoWriter
		waitCmd.Stderr = GinkgoWriter
		err = waitCmd.Run()
		Expect(err).ToNot(HaveOccurred(), "WorkflowExecution Controller pod did not become ready")
		logger.Info("✅ WorkflowExecution Controller pod is ready")

		// Note: Test pipeline is already created by hybrid infrastructure setup

		// DD-AUTH-014: Create E2E ServiceAccount for DataStorage authentication
		logger.Info("🔐 Creating E2E ServiceAccount for DataStorage audit queries (DD-AUTH-014)")
		e2eSAName := "workflowexecution-e2e-sa"
		namespace := infrastructure.WorkflowExecutionNamespace

		err = infrastructure.CreateE2EServiceAccountWithDataStorageAccess(ctx, namespace, kubeconfigPath, e2eSAName, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred(), "Failed to create E2E ServiceAccount")

		// Get ServiceAccount token for Bearer authentication
		token, err := infrastructure.GetServiceAccountToken(ctx, namespace, e2eSAName, kubeconfigPath)
		Expect(err).ToNot(HaveOccurred(), "Failed to get E2E ServiceAccount token")
		logger.Info("✅ E2E ServiceAccount token retrieved for authenticated DataStorage access")

		logger.Info("✅ WorkflowExecution E2E environment ready!")

		// Return kubeconfig path and auth token for other processes
		return []byte(fmt.Sprintf("%s|%s", kubeconfigPath, token))
	},
	// This runs on ALL processes - connects to the shared cluster
	func(kubeconfigBytes []byte) {
		// Initialize context for this process
		ctx, cancel = context.WithCancel(context.Background())

		// Initialize logger for this process
		logger = kubelog.NewLogger(kubelog.Options{
			Development: true,
			Level:       0, // INFO
			ServiceName: "workflowexecution-e2e-test",
		})

		// Parse data: "kubeconfig|authToken"
		parts := strings.Split(string(kubeconfigBytes), "|")
		kubeconfigPath = parts[0]
		if len(parts) > 1 {
			e2eAuthToken = parts[1] // DD-AUTH-014: Store token for authenticated DataStorage access
		}

		// Set KUBECONFIG environment variable
		err := os.Setenv("KUBECONFIG", kubeconfigPath)
		Expect(err).ToNot(HaveOccurred())

		// Create Kubernetes client
		cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		Expect(err).ToNot(HaveOccurred())

		// Register CRD schemes
		err = workflowexecutionv1alpha1.AddToScheme(scheme.Scheme)
		Expect(err).ToNot(HaveOccurred())
		err = tektonv1.AddToScheme(scheme.Scheme)
		Expect(err).ToNot(HaveOccurred())
		// Note: batch/v1 is already registered by k8s.io/client-go/kubernetes/scheme (BR-WE-014)

		k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
		Expect(err).ToNot(HaveOccurred())

		// Create direct API reader for Eventually() blocks to bypass client cache
		// This ensures fresh reads from API server for status polling
		apiReader, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
		Expect(err).ToNot(HaveOccurred())

		logger.Info("✅ Connected to WorkflowExecution E2E cluster",
			"kubeconfig", kubeconfigPath,
			"ginkgoProcess", GinkgoParallelProcess())
	},
)

var _ = SynchronizedAfterSuite(
	// This runs on ALL processes after their tests complete
	func() {
		if cancel != nil {
			cancel()
		}
		if logger.GetSink() != nil {
			logger.Info("Process cleanup complete", "ginkgoProcess", GinkgoParallelProcess())
		}
	},
	// This runs ONCE on process 1 after ALL processes complete
	func() {
		// Initialize logger if not already done (in case BeforeSuite failed)
		if logger.GetSink() == nil {
			logger = kubelog.NewLogger(kubelog.Options{
				Development: true,
				Level:       0,
				ServiceName: "workflowexecution-e2e-test",
			})
		}

		// DD-TEST-007: Collect E2E binary coverage BEFORE cluster deletion
		if os.Getenv("E2E_COVERAGE") == "true" {
			if err := infrastructure.CollectE2EBinaryCoverage(infrastructure.E2ECoverageOptions{
				ServiceName:    "workflowexecution",
				ClusterName:    clusterName,
				DeploymentName: "workflowexecution-controller",
				Namespace:      controllerNamespace,
				KubeconfigPath: kubeconfigPath,
			}, GinkgoWriter); err != nil {
				logger.Error(err, "Failed to collect E2E binary coverage (non-fatal)")
			}
		}

		// Detect setup failure: if k8sClient is nil, BeforeSuite failed
		setupFailed := k8sClient == nil
		if setupFailed {
			logger.Info("⚠️  Setup failure detected (k8sClient is nil)")
		}

		// Determine cleanup strategy
		anyFailure := setupFailed || anyTestFailed
		preserveCluster := os.Getenv("KEEP_CLUSTER") == "true"

		if preserveCluster {
			logger.Info("⚠️  CLUSTER PRESERVED FOR DEBUGGING (KEEP_CLUSTER=true)")
			logger.Info("    Cluster: " + clusterName)
			logger.Info("    Kubeconfig: " + kubeconfigPath)
			logger.Info("    Delete manually: kind delete cluster --name " + clusterName)
		} else {
			// Delete cluster (with must-gather log export on failure)
			logger.Info("🗑️  Cleaning up Kind cluster...")
			err := infrastructure.DeleteWorkflowExecutionCluster(clusterName, anyFailure, GinkgoWriter)
			if err != nil {
				logger.Error(err, "Failed to delete cluster")
			}
		}

		// DD-TEST-001 v1.1: E2E Test Image Cleanup (MANDATORY)
		// Clean up service images built for Kind to prevent disk space exhaustion
		// This runs regardless of test success/failure to prevent image accumulation
		imageRegistry := os.Getenv("IMAGE_REGISTRY")
		imageTag := os.Getenv("IMAGE_TAG")

		// Skip cleanup when using registry images (CI/CD mode)
		if imageRegistry != "" && imageTag != "" {
			logger.Info("ℹ️  Registry mode detected - skipping local image removal",
				"registry", imageRegistry, "tag", imageTag)
		} else {
			// Local build mode: Remove locally built images
			logger.Info("🗑️  Cleaning up service images built for Kind...")

			// Service-specific images built during E2E test setup
			// Per DD-TEST-001: Use service-specific tags to avoid conflicts when multiple services run E2E tests
			imagesToClean := []string{
				"localhost/kubernaut-workflowexecution:e2e-test-workflowexecution", // This service's controller
				"localhost/kubernaut-datastorage:e2e-test-datastorage",             // DataStorage infrastructure
			}

			// DD-TEST-001: Clean up IMAGE_TAG if set (local builds with unique tags)
			if imageTag != "" {
				imagesToClean = append(imagesToClean,
					fmt.Sprintf("workflowexecution:%s", imageTag))
			}

			// Remove each image
			for _, imageName := range imagesToClean {
				rmiCmd := exec.Command("podman", "rmi", "-f", imageName)
				rmiOutput, rmiErr := rmiCmd.CombinedOutput()
				if rmiErr != nil {
					logger.Info("⚠️  Failed to remove image (may not exist)",
						"image", imageName,
						"output", string(rmiOutput))
				} else {
					logger.Info("✅ Image removed", "image", imageName)
				}
			}
		}

		// Prune dangling images from Kind builds (best effort)
		logger.Info("🗑️  Pruning dangling images from Kind builds...")
		pruneCmd := exec.Command("podman", "image", "prune", "-f")
		pruneOutput, pruneErr := pruneCmd.CombinedOutput()
		if pruneErr != nil {
			logger.Info("⚠️  Image prune failed (non-critical)",
				"error", pruneErr,
				"output", string(pruneOutput))
		} else {
			logger.Info("✅ Dangling images pruned")
		}

		logger.Info("✅ E2E cleanup complete")
	},
)

// Track test failures for cleanup decision
var _ = ReportAfterEach(func(report SpecReport) {
	if report.Failed() {
		anyTestFailed = true
	}
})

// ========================================
// Test Helpers
// ========================================

// createTestWFE creates a WorkflowExecution for testing
func createTestWFE(name, targetResource string) *workflowexecutionv1alpha1.WorkflowExecution {
	return &workflowexecutionv1alpha1.WorkflowExecution{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
			ExecutionEngine: "tekton", // BR-WE-014: Required field (enum: tekton, job)
			// Required reference to parent RemediationRequest
			RemediationRequestRef: corev1.ObjectReference{
				APIVersion: "remediationorchestrator.kubernaut.ai/v1alpha1",
				Kind:       "RemediationRequest",
				Name:       "test-rr-" + name,
				Namespace:  "default",
			},
			WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
				WorkflowID: "test-hello-world",
				Version:    "v1.0.0",
				// Use multi-arch bundle from quay.io/kubernaut-cicd (amd64 + arm64)
				// Per ADR-043: Bundle contains pipeline.yaml + workflow-schema.yaml
				// Per test/infrastructure/workflow_bundles.go: BuildAndRegisterTestWorkflows
				ContainerImage: "quay.io/kubernaut-cicd/test-workflows/hello-world:v1.0.0",
			},
			TargetResource: targetResource,
			Parameters: map[string]string{
				"MESSAGE": "E2E test message",
			},
		},
	}
}

// getWFE gets a WorkflowExecution by name
func getWFE(name, namespace string) (*workflowexecutionv1alpha1.WorkflowExecution, error) {
	wfe := &workflowexecutionv1alpha1.WorkflowExecution{}
	err := k8sClient.Get(context.Background(), types.NamespacedName{Name: name, Namespace: namespace}, wfe)
	return wfe, err
}

// getWFEDirect reads WorkflowExecution directly from API server, bypassing client cache.
// Use this in Eventually() blocks for status polling to avoid cache consistency issues.
func getWFEDirect(name, namespace string) (*workflowexecutionv1alpha1.WorkflowExecution, error) {
	wfe := &workflowexecutionv1alpha1.WorkflowExecution{}
	err := apiReader.Get(context.Background(), types.NamespacedName{Name: name, Namespace: namespace}, wfe)
	return wfe, err
}

// deleteWFE deletes a WorkflowExecution
func deleteWFE(wfe *workflowexecutionv1alpha1.WorkflowExecution) error {
	return k8sClient.Delete(context.Background(), wfe)
}

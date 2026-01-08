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
	"time"

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

	// Controller namespace
	controllerNamespace string = infrastructure.WorkflowExecutionNamespace

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

		logger.Info("✅ WorkflowExecution E2E environment ready!")

		// Return kubeconfig path for other processes
		return []byte(kubeconfigPath)
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

		// Get kubeconfig path from process 1
		kubeconfigPath = string(kubeconfigBytes)

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

		k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
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

		// DD-TEST-007: Extract E2E coverage if E2E_COVERAGE=true
		if os.Getenv("E2E_COVERAGE") == "true" {
			logger.Info("📊 Extracting E2E coverage data...")

			// 1. Scale down controller to flush coverage (graceful shutdown)
			logger.Info("  Scaling down controller to flush coverage...")
			scaleCmd := exec.Command("kubectl", "scale",
				"-n", controllerNamespace,
				"deployment/workflowexecution-controller",
				"--replicas=0",
				"--kubeconfig", kubeconfigPath)
			scaleCmd.Stdout = GinkgoWriter
			scaleCmd.Stderr = GinkgoWriter
			if err := scaleCmd.Run(); err != nil {
				logger.Error(err, "Failed to scale down controller")
			} else {
				// Wait for graceful shutdown to flush coverage
				logger.Info("  Waiting 10s for graceful shutdown to flush coverage...")
				time.Sleep(10 * time.Second)
			}

			// 2. Copy coverage data from Kind node
			logger.Info("  Copying coverage data from Kind node...")
			coverageDir := "test/e2e/workflowexecution/coverdata"
			_ = os.RemoveAll(coverageDir) // Clean old coverage
			_ = os.MkdirAll(coverageDir, 0755)

			// Get Kind node container name
			nodeListCmd := exec.Command("podman", "ps", "--filter",
				fmt.Sprintf("name=%s-control-plane", clusterName),
				"--format", "{{.Names}}")
			nodeOutput, err := nodeListCmd.Output()
			if err != nil {
				logger.Error(err, "Failed to find Kind node container")
			} else {
				nodeName := strings.TrimSpace(string(nodeOutput))

				// Copy /coverdata from Kind node to local
				cpCmd := exec.Command("podman", "cp",
					fmt.Sprintf("%s:/coverdata", nodeName),
					coverageDir)
				cpCmd.Stdout = GinkgoWriter
				cpCmd.Stderr = GinkgoWriter
				if err := cpCmd.Run(); err != nil {
					logger.Error(err, "Failed to copy coverage data")
				} else {
					logger.Info("✅ Coverage data extracted", "dir", coverageDir)

					// 3. Generate coverage reports
					logger.Info("  Generating coverage reports...")

					// Text report
					textReportPath := "test/e2e/workflowexecution/e2e-coverage.txt"
					textCmd := exec.Command("go", "tool", "covdata", "textfmt",
						"-i="+coverageDir,
						"-o="+textReportPath)
					textCmd.Stdout = GinkgoWriter
					textCmd.Stderr = GinkgoWriter
					if err := textCmd.Run(); err != nil {
						logger.Error(err, "Failed to generate text report")
					} else {
						logger.Info("✅ Text report", "file", textReportPath)
					}

					// HTML report
					htmlReportPath := "test/e2e/workflowexecution/e2e-coverage.html"
					htmlCmd := exec.Command("go", "tool", "cover",
						"-html="+textReportPath,
						"-o="+htmlReportPath)
					htmlCmd.Stdout = GinkgoWriter
					htmlCmd.Stderr = GinkgoWriter
					if err := htmlCmd.Run(); err != nil {
						logger.Error(err, "Failed to generate HTML report")
					} else {
						logger.Info("✅ HTML report", "file", htmlReportPath)
					}

					// Coverage percentage
					percentCmd := exec.Command("go", "tool", "covdata", "percent",
						"-i="+coverageDir)
					percentOutput, err := percentCmd.CombinedOutput()
					if err != nil {
						logger.Error(err, "Failed to calculate coverage percentage")
					} else {
						logger.Info("📊 E2E Coverage Results:\n" + string(percentOutput))
					}
				}
			}
		}

		// Determine cleanup strategy
		preserveCluster := os.Getenv("KEEP_CLUSTER") == "true"

		if preserveCluster {
			logger.Info("⚠️  CLUSTER PRESERVED FOR DEBUGGING (KEEP_CLUSTER=true)")
			logger.Info("    Cluster: " + clusterName)
			logger.Info("    Kubeconfig: " + kubeconfigPath)
			logger.Info("    Delete manually: kind delete cluster --name " + clusterName)
		} else {
			// Delete cluster (with must-gather log export on failure)
			logger.Info("🗑️  Cleaning up Kind cluster...")
			err := infrastructure.DeleteWorkflowExecutionCluster(clusterName, anyTestFailed, GinkgoWriter)
			if err != nil {
				logger.Error(err, "Failed to delete cluster")
			}
		}

		// DD-TEST-001 v1.1: E2E Test Image Cleanup (MANDATORY)
		// Clean up service images built for Kind to prevent disk space exhaustion
		// This runs regardless of test success/failure to prevent image accumulation
		logger.Info("🗑️  Cleaning up service images built for Kind...")

		// Service-specific images built during E2E test setup
		// Per DD-TEST-001: Use service-specific tags to avoid conflicts when multiple services run E2E tests
		imagesToClean := []string{
			"localhost/kubernaut-workflowexecution:e2e-test-workflowexecution", // This service's controller
			"localhost/kubernaut-datastorage:e2e-test-datastorage",             // DataStorage infrastructure
		}

		// DD-TEST-001: Clean up IMAGE_TAG if set (CI/CD builds with unique tags)
		// Format: {service}:{service}-{user}-{git-hash}-{timestamp}
		imageTag := os.Getenv("IMAGE_TAG")
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
				// Use production bundle from quay.io (Option B: builds locally only if not found)
				// Per ADR-043: Bundle contains pipeline.yaml + workflow-schema.yaml
				// Per test/infrastructure/workflow_bundles.go: BuildAndRegisterTestWorkflows
				ContainerImage: "quay.io/jordigilh/test-workflows/hello-world:v1.0.0",
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

// deleteWFE deletes a WorkflowExecution
func deleteWFE(wfe *workflowexecutionv1alpha1.WorkflowExecution) error {
	return k8sClient.Delete(context.Background(), wfe)
}

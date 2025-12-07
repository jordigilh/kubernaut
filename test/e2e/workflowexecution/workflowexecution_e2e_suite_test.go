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
// - BR-WE-004: Failure details actionable for recovery
// - BR-WE-009: Parallel execution prevention
// - BR-WE-010: Cooldown enforcement
// - BR-WE-012: Exponential backoff skip reasons

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
		kubeconfigPath = fmt.Sprintf("%s/.kube/workflowexecution-kubeconfig", homeDir)

		// Delete any existing cluster first
		logger.Info("Checking for existing cluster...")
		_ = infrastructure.DeleteWorkflowExecutionCluster(clusterName, GinkgoWriter)

		// Create Kind cluster with Tekton
		err = infrastructure.CreateWorkflowExecutionCluster(clusterName, kubeconfigPath, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())

		// Set KUBECONFIG environment variable
		err = os.Setenv("KUBECONFIG", kubeconfigPath)
		Expect(err).ToNot(HaveOccurred())

		// Deploy WorkflowExecution Controller
		logger.Info("Deploying WorkflowExecution Controller...")
		err = infrastructure.DeployWorkflowExecutionController(ctx, controllerNamespace, kubeconfigPath, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())

		// Wait for deployment to be available first (ensures ReplicaSet creates pod)
		logger.Info("⏳ Waiting for WorkflowExecution Controller deployment to be available...")
		waitDeployCmd := exec.Command("kubectl", "wait",
			"-n", controllerNamespace,
			"--for=condition=available",
			"deployment/workflowexecution-controller",
			"--timeout=120s",
			"--kubeconfig", kubeconfigPath)
		waitDeployCmd.Stdout = GinkgoWriter
		waitDeployCmd.Stderr = GinkgoWriter
		err = waitDeployCmd.Run()
		Expect(err).ToNot(HaveOccurred(), "WorkflowExecution Controller deployment did not become available")

		// Then wait for controller pod to be ready
		logger.Info("⏳ Waiting for WorkflowExecution Controller pod to be ready...")
		waitCmd := exec.Command("kubectl", "wait",
			"-n", controllerNamespace,
			"--for=condition=ready",
			"pod",
			"-l", "app=workflowexecution-controller",
			"--timeout=120s",
			"--kubeconfig", kubeconfigPath)
		waitCmd.Stdout = GinkgoWriter
		waitCmd.Stderr = GinkgoWriter
		err = waitCmd.Run()
		Expect(err).ToNot(HaveOccurred(), "WorkflowExecution Controller pod did not become ready")
		logger.Info("✅ WorkflowExecution Controller pod is ready")

		// Create test pipeline
		err = infrastructure.CreateSimpleTestPipeline(kubeconfigPath, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())

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

		if anyTestFailed {
			logger.Info("⚠️  Tests failed - preserving cluster for investigation")
			logger.Info("    Cluster: " + clusterName)
			logger.Info("    Kubeconfig: " + kubeconfigPath)
			logger.Info("    Delete manually: kind delete cluster --name " + clusterName)
		} else {
			logger.Info("🗑️  Cleaning up Kind cluster...")
			err := infrastructure.DeleteWorkflowExecutionCluster(clusterName, GinkgoWriter)
			if err != nil {
				logger.Error(err, "Failed to delete cluster")
			}
		}
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
				WorkflowID:     "test-hello-world",
				Version:        "v1.0.0",
				ContainerImage: "quay.io/kubernaut/workflows/test-hello-world:v1.0.0",
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


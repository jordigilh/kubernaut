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

// Package remediationorchestrator_test contains E2E tests for the RemediationOrchestrator controller.
// These tests use a KIND cluster with full service deployment.
//
// Defense-in-Depth Strategy (per 03-testing-strategy.mdc):
// - Unit tests (70%+): Business logic in isolation (test/unit/remediationorchestrator/)
// - Integration tests (>50%): Infrastructure interaction with envtest (test/integration/remediationorchestrator/)
// - E2E tests (10-15%): Complete workflow validation with KIND (this file)
//
// CRITICAL: Uses isolated kubeconfig to avoid overwriting ~/.kube/config
// Per TESTING_GUIDELINES.md: kubeconfig at ~/.kube/remediationorchestrator-e2e-config
//
// Test Execution (parallel, 4 procs):
//
//	ginkgo -p --procs=4 ./test/e2e/remediationorchestrator/...
//
// MANDATORY: All tests use unique namespaces for parallel execution isolation.
package remediationorchestrator

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	// Import ALL CRD types that RO interacts with
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// Test constants for timeout and polling intervals
const (
	timeout  = 120 * time.Second // Longer timeout for E2E with real services
	interval = 500 * time.Millisecond

	// Cluster configuration
	clusterName = "ro-e2e"
)

// Package-level variables for test environment
var (
	ctx    context.Context
	cancel context.CancelFunc

	// ============================================================================
	// CRITICAL: Isolated kubeconfig path
	// Per TESTING_GUIDELINES.md - NEVER overwrite ~/.kube/config
	// ============================================================================
	kubeconfigPath string

	k8sClient client.Client
)

func TestRemediationOrchestratorE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "RemediationOrchestrator Controller E2E Suite (KIND)")
}

var _ = SynchronizedBeforeSuite(
	// This runs on process 1 only - create cluster once
	func() []byte {
		logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

		By("Setting up isolated kubeconfig path (per TESTING_GUIDELINES.md)")
		homeDir, err := os.UserHomeDir()
		Expect(err).ToNot(HaveOccurred())

		// ============================================================================
		// CRITICAL: Use isolated kubeconfig - NEVER use ~/.kube/config
		// This prevents accidentally overwriting user's real cluster credentials
		// ============================================================================
		tempKubeconfigPath := fmt.Sprintf("%s/.kube/%s-e2e-config", homeDir, clusterName)
		GinkgoWriter.Printf("📂 Using isolated kubeconfig: %s\n", tempKubeconfigPath)

		By("Setting up RO E2E infrastructure using HYBRID PARALLEL approach (DD-TEST-002)")
		// This replaces manual cluster creation with the validated hybrid pattern:
		// 1. Build images in parallel (RO + DataStorage)
		// 2. Create Kind cluster AFTER builds complete (no idle timeout)
		// 3. Load images immediately (reliable)
		// 4. Deploy all services (PostgreSQL, Redis, DataStorage, RO)
		//
		// Expected time: ~5-6 minutes (vs 20-25 minutes sequential)
		// Reliability: 100% (no Kind cluster timeouts)
		ctx := context.Background()
		err = infrastructure.SetupROInfrastructureHybridWithCoverage(
			ctx, clusterName, tempKubeconfigPath, GinkgoWriter,
		)
		Expect(err).ToNot(HaveOccurred())

		By("Setting KUBECONFIG for all processes")
		err = os.Setenv("KUBECONFIG", tempKubeconfigPath)
		Expect(err).ToNot(HaveOccurred())

		GinkgoWriter.Println("✅ E2E test environment ready (Process 1)")
		GinkgoWriter.Printf("   Cluster: %s\n", clusterName)
		GinkgoWriter.Printf("   Kubeconfig: %s\n", tempKubeconfigPath)
		GinkgoWriter.Println("   Process 1 will now share kubeconfig with other processes")

		// Return kubeconfig path to all processes
		return []byte(tempKubeconfigPath)
	},
	// This runs on ALL processes - connect to the cluster created by process 1
	func(data []byte) {
		kubeconfigPath = string(data)

		// Initialize context
		ctx, cancel = context.WithCancel(context.TODO())

		GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		GinkgoWriter.Printf("RO E2E Test Suite - Setup (Process %d)\n", GinkgoParallelProcess())
		GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		GinkgoWriter.Printf("Connecting to cluster created by process 1\n")
		GinkgoWriter.Printf("  • Kubeconfig: %s\n", kubeconfigPath)

		By("Setting KUBECONFIG environment variable for this test process")
		err := os.Setenv("KUBECONFIG", kubeconfigPath)
		Expect(err).ToNot(HaveOccurred())

		By("Registering ALL CRD schemes for RO orchestration")
		err = remediationv1.AddToScheme(scheme.Scheme)
		Expect(err).NotTo(HaveOccurred())
		err = signalprocessingv1.AddToScheme(scheme.Scheme)
		Expect(err).NotTo(HaveOccurred())
		err = aianalysisv1.AddToScheme(scheme.Scheme)
		Expect(err).NotTo(HaveOccurred())
		err = workflowexecutionv1.AddToScheme(scheme.Scheme)
		Expect(err).NotTo(HaveOccurred())
		err = notificationv1.AddToScheme(scheme.Scheme)
		Expect(err).NotTo(HaveOccurred())

		By("Creating Kubernetes client from isolated kubeconfig")
		cfg, err := config.GetConfig()
		Expect(err).ToNot(HaveOccurred())

		k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
		Expect(err).ToNot(HaveOccurred())

		GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		GinkgoWriter.Printf("Setup Complete - Process %d ready to run tests\n", GinkgoParallelProcess())
		GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	},
)

var _ = SynchronizedAfterSuite(
	// This runs on ALL processes - cleanup context
	func() {
		GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		GinkgoWriter.Printf("Process %d - Cleaning up\n", GinkgoParallelProcess())
		GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Cancel context for this process
		if cancel != nil {
			cancel()
		}
	},
	// This runs on process 1 only - cleanup cluster
	func() {
		By("Cleaning up test environment")

		// Determine cleanup strategy
		preserveCluster := os.Getenv("PRESERVE_E2E_CLUSTER") == "true" || os.Getenv("KEEP_CLUSTER") == "true"

		if preserveCluster {
			GinkgoWriter.Println("⚠️  CLUSTER PRESERVED FOR DEBUGGING")
			GinkgoWriter.Printf("   To access: export KUBECONFIG=%s\n", kubeconfigPath)
			GinkgoWriter.Printf("   To delete: kind delete cluster --name %s\n", clusterName)
			return
		}

		By("Deleting KIND cluster (with must-gather log export on failure)")
		// Note: RemediationOrchestrator doesn't track test failures currently
		// Pass false for testsFailed until failure tracking is added
		if err := infrastructure.DeleteCluster(clusterName, "remediationorchestrator", false, GinkgoWriter); err != nil {
			GinkgoWriter.Printf("⚠️  Warning: Failed to delete cluster: %v\n", err)
		}

		By("Removing isolated kubeconfig file")
		// ============================================================================
		// CRITICAL: Only delete the isolated kubeconfig, never the default one
		// ============================================================================
		if kubeconfigPath != "" {
			defaultConfig := os.ExpandEnv("$HOME/.kube/config")
			if kubeconfigPath != defaultConfig {
				_ = os.Remove(kubeconfigPath)
				GinkgoWriter.Printf("🗑️  Removed kubeconfig: %s\n", kubeconfigPath)
			} else {
				GinkgoWriter.Println("⚠️  Skipping removal - path matches default kubeconfig")
			}
		}

		By("Cleaning up service images built for Kind (DD-TEST-001 v1.1)")
		// Remove service image built for this test run
		imageTag := os.Getenv("IMAGE_TAG") // Set by build/test infrastructure
		if imageTag != "" {
			imageName := fmt.Sprintf("remediationorchestrator:%s", imageTag)

			pruneCmd := exec.Command("podman", "rmi", imageName)
			pruneOutput, pruneErr := pruneCmd.CombinedOutput()
			if pruneErr != nil {
				GinkgoWriter.Printf("⚠️  Failed to remove service image: %v\n%s\n", pruneErr, pruneOutput)
			} else {
				GinkgoWriter.Printf("✅ Service image removed: %s\n", imageName)
			}
		}

		By("Pruning dangling images from Kind builds (DD-TEST-001 v1.1)")
		// Prune any dangling images left from failed builds
		pruneCmd := exec.Command("podman", "image", "prune", "-f")
		_, _ = pruneCmd.CombinedOutput()

		GinkgoWriter.Println("✅ E2E cleanup complete")
	},
)

// ============================================================================
// Kind Cluster Management Helpers
// ============================================================================

// ============================================================================
// Kind Cluster Management Helpers (now handled by hybrid infrastructure)
// ============================================================================
// NOTE: Most cluster management is now handled by the hybrid infrastructure
// (SetupROInfrastructureHybridWithCoverage). These remaining helpers are
// used only for cleanup in AfterSuite.

func deleteKindCluster(name string) {
	cmd := exec.Command("kind", "delete", "cluster", "--name", name)
	cmd.Stdout = GinkgoWriter
	cmd.Stderr = GinkgoWriter

	err := cmd.Run()
	if err != nil {
		GinkgoWriter.Printf("⚠️  Failed to delete cluster (may not exist): %v\n", err)
	}
}

// ============================================================================
// Test Namespace Helpers
// ============================================================================

func createTestNamespace(prefix string) string {
	name := fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"kubernaut.io/test": "e2e-remediationorchestrator",
			},
		},
	}
	err := k8sClient.Create(ctx, ns)
	Expect(err).ToNot(HaveOccurred())
	return name
}

func deleteTestNamespace(name string) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	err := k8sClient.Delete(ctx, ns)
	if err != nil && !apierrors.IsNotFound(err) {
		GinkgoWriter.Printf("⚠️  Failed to delete namespace %s: %v\n", name, err)
	}
}

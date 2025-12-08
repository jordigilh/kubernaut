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
package remediationorchestrator_test

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

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	By("Setting up isolated kubeconfig path (per TESTING_GUIDELINES.md)")
	homeDir, err := os.UserHomeDir()
	Expect(err).ToNot(HaveOccurred())

	// ============================================================================
	// CRITICAL: Use isolated kubeconfig - NEVER use ~/.kube/config
	// This prevents accidentally overwriting user's real cluster credentials
	// ============================================================================
	kubeconfigPath = fmt.Sprintf("%s/.kube/%s-e2e-config", homeDir, clusterName)
	GinkgoWriter.Printf("📂 Using isolated kubeconfig: %s\n", kubeconfigPath)

	By("Checking for existing Kind cluster")
	if !clusterExists(clusterName) {
		By("Creating KIND cluster with isolated kubeconfig")
		createKindCluster(clusterName, kubeconfigPath)
	} else {
		GinkgoWriter.Println("♻️  Reusing existing cluster")
		exportKubeconfig(clusterName, kubeconfigPath)
	}

	By("Setting KUBECONFIG environment variable for this test process")
	err = os.Setenv("KUBECONFIG", kubeconfigPath)
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

	By("Installing ALL CRDs required for RO E2E tests")
	installCRDs()

	// TODO: Deploy services when teams respond with availability status
	// - SignalProcessing controller
	// - AIAnalysis controller
	// - WorkflowExecution controller
	// - Notification controller
	// - RemediationOrchestrator controller

	GinkgoWriter.Println("✅ E2E test environment ready")
	GinkgoWriter.Printf("   Cluster: %s\n", clusterName)
	GinkgoWriter.Printf("   Kubeconfig: %s\n", kubeconfigPath)
})

var _ = AfterSuite(func() {
	By("Cleaning up test environment")
	cancel()

	// Check if we should preserve the cluster for debugging
	if os.Getenv("PRESERVE_E2E_CLUSTER") == "true" {
		GinkgoWriter.Println("⚠️  PRESERVE_E2E_CLUSTER=true, keeping cluster for debugging")
		GinkgoWriter.Printf("   To access: export KUBECONFIG=%s\n", kubeconfigPath)
		GinkgoWriter.Printf("   To delete: kind delete cluster --name %s\n", clusterName)
		return
	}

	By("Deleting KIND cluster")
	deleteKindCluster(clusterName)

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
})

// ============================================================================
// Kind Cluster Management Helpers
// ============================================================================

func clusterExists(name string) bool {
	cmd := exec.Command("kind", "get", "clusters")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	// Check if cluster name appears in the output
	clusters := string(output)
	for _, line := range splitLines(clusters) {
		if line == name {
			return true
		}
	}
	return false
}

func splitLines(s string) []string {
	var lines []string
	var current string
	for _, c := range s {
		if c == '\n' {
			if current != "" {
				lines = append(lines, current)
			}
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func createKindCluster(name, kubeconfig string) {
	// ============================================================================
	// CRITICAL: Pass --kubeconfig to Kind to use isolated file
	// ============================================================================
	cmd := exec.Command("kind", "create", "cluster",
		"--name", name,
		"--kubeconfig", kubeconfig,
		"--wait", "120s",
	)
	cmd.Stdout = GinkgoWriter
	cmd.Stderr = GinkgoWriter

	err := cmd.Run()
	Expect(err).ToNot(HaveOccurred(), "Failed to create Kind cluster")

	GinkgoWriter.Printf("✅ Created Kind cluster '%s' with isolated kubeconfig\n", name)
}

func exportKubeconfig(name, kubeconfig string) {
	cmd := exec.Command("kind", "export", "kubeconfig",
		"--name", name,
		"--kubeconfig", kubeconfig,
	)
	cmd.Stdout = GinkgoWriter
	cmd.Stderr = GinkgoWriter

	err := cmd.Run()
	Expect(err).ToNot(HaveOccurred(), "Failed to export kubeconfig")
}

func deleteKindCluster(name string) {
	cmd := exec.Command("kind", "delete", "cluster", "--name", name)
	cmd.Stdout = GinkgoWriter
	cmd.Stderr = GinkgoWriter

	err := cmd.Run()
	if err != nil {
		GinkgoWriter.Printf("⚠️  Failed to delete cluster (may not exist): %v\n", err)
	}
}

func installCRDs() {
	crdPaths := []string{
		"config/crd/bases/remediation.kubernaut.io_remediationrequests.yaml",
		"config/crd/bases/remediation.kubernaut.io_remediationapprovalrequests.yaml",
		"config/crd/bases/signalprocessing.kubernaut.io_signalprocessings.yaml",
		"config/crd/bases/aianalysis.kubernaut.io_aianalyses.yaml",
		"config/crd/bases/workflowexecution.kubernaut.io_workflowexecutions.yaml",
		"config/crd/bases/notification.kubernaut.io_notificationrequests.yaml",
	}

	for _, crdPath := range crdPaths {
		// Find CRD file from project root
		fullPath := findProjectFile(crdPath)
		if fullPath == "" {
			GinkgoWriter.Printf("⚠️  CRD not found: %s\n", crdPath)
			continue
		}

		cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
			"apply", "-f", fullPath)
		cmd.Stdout = GinkgoWriter
		cmd.Stderr = GinkgoWriter

		err := cmd.Run()
		if err != nil {
			GinkgoWriter.Printf("⚠️  Failed to install CRD %s: %v\n", crdPath, err)
		}
	}
}

func findProjectFile(relativePath string) string {
	// Try from current directory
	if _, err := os.Stat(relativePath); err == nil {
		return relativePath
	}

	// Try from project root (3 levels up from test/e2e/remediationorchestrator/)
	projectRoot := "../../../" + relativePath
	if _, err := os.Stat(projectRoot); err == nil {
		return projectRoot
	}

	return ""
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


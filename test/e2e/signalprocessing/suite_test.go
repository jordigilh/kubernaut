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

// Package signalprocessing_e2e contains E2E tests for SignalProcessing.
// These tests validate complete business workflows with real Kind cluster.
//
// Defense-in-Depth Strategy (per 03-testing-strategy.mdc):
// - Unit tests (70%+): Business logic in isolation (test/unit/signalprocessing/)
// - Integration tests (>50%): CRD coordination, ENVTEST (test/integration/signalprocessing/)
// - E2E tests (10-15%): Complete workflow validation (this directory)
//
// Kubeconfig Convention (per TESTING_GUIDELINES.md):
// - Pattern: ~/.kube/{service}-e2e-config
// - Path: ~/.kube/signalprocessing-e2e-config
// - Cluster Name: signalprocessing-e2e
//
// Port Allocation (per DD-TEST-001):
// - NodePort (Metrics): 30182 -> localhost:9182
// - NodePort (API): 30082 -> localhost:8082
//
// Business Requirements Validated:
// - BR-SP-051: Environment classification
// - BR-SP-070: Priority assignment
// - BR-SP-100: Owner chain traversal
// - BR-SP-101: Detected labels
// - BR-SP-102: CustomLabels from Rego
package signalprocessing_e2e

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// Global test variables
var (
	ctx            context.Context
	cancel         context.CancelFunc
	k8sClient      client.Client
	clientset      *kubernetes.Clientset
	kubeconfigPath string
	metricsURL     string
)

const (
	clusterName = "signalprocessing-e2e"
	serviceName = "signalprocessing"
	timeout     = 60 * time.Second
	interval    = 2 * time.Second
)

func TestSignalProcessingE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SignalProcessing E2E Suite")
}

var _ = SynchronizedBeforeSuite(
	// This runs on process 1 only - create cluster once
	func() []byte {
		By("Setting up SignalProcessing E2E cluster (process 1 only)")

		// Get home directory for kubeconfig
		homeDir, err := os.UserHomeDir()
		Expect(err).ToNot(HaveOccurred())

		// Standard kubeconfig location: ~/.kube/{service}-e2e-config
		// Per docs/development/business-requirements/TESTING_GUIDELINES.md
		kubeconfigPath = filepath.Join(homeDir, ".kube", fmt.Sprintf("%s-e2e-config", serviceName))

		By(fmt.Sprintf("Creating Kind cluster '%s'", clusterName))
		By(fmt.Sprintf("  • Kubeconfig: %s", kubeconfigPath))
		By(fmt.Sprintf("  • Metrics URL: http://localhost:9182/metrics"))

		// Create Kind cluster with SignalProcessing infrastructure
		err = infrastructure.CreateSignalProcessingCluster(clusterName, kubeconfigPath, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())

		// BR-SP-090: Deploy DataStorage infrastructure for audit testing
		// This must be done BEFORE deploying the controller
		ctx := context.Background()
		By("Deploying DataStorage for BR-SP-090 audit testing")
		err = infrastructure.DeployDataStorageForSignalProcessing(ctx, kubeconfigPath, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())

		// Deploy SignalProcessing controller
		err = infrastructure.DeploySignalProcessingController(ctx, kubeconfigPath, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())

		return []byte(kubeconfigPath)
	},
	// This runs on ALL processes - connect to cluster
	func(data []byte) {
		kubeconfigPath = string(data)
		ctx, cancel = context.WithCancel(context.Background())

		By(fmt.Sprintf("Connecting to cluster (kubeconfig: %s)", kubeconfigPath))

		// Build REST config from kubeconfig
		config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		Expect(err).ToNot(HaveOccurred())

		// Create controller-runtime client
		k8sClient, err = client.New(config, client.Options{})
		Expect(err).ToNot(HaveOccurred())
		Expect(k8sClient).ToNot(BeNil())

		// Register SignalProcessing scheme
		err = signalprocessingv1alpha1.AddToScheme(k8sClient.Scheme())
		Expect(err).ToNot(HaveOccurred())

		// Register RemediationRequest scheme (parent of SignalProcessing)
		err = remediationv1alpha1.AddToScheme(k8sClient.Scheme())
		Expect(err).ToNot(HaveOccurred())

		// Create standard clientset for native K8s resources
		clientset, err = kubernetes.NewForConfig(config)
		Expect(err).ToNot(HaveOccurred())

		// Set metrics URL (NodePort via Kind extraPortMappings)
		metricsURL = "http://localhost:9182/metrics"

		By("E2E setup complete - ready for tests")
	},
)

var _ = SynchronizedAfterSuite(
	// This runs on ALL processes
	func() {
		By("Cleaning up test resources")
		if cancel != nil {
			cancel()
		}
	},
	// This runs on process 1 only - delete cluster
	func() {
		By("Deleting Kind cluster (process 1 only)")

		// Check if KEEP_CLUSTER env var is set (useful for debugging)
		if os.Getenv("KEEP_CLUSTER") != "" {
			By("KEEP_CLUSTER set - preserving cluster for debugging")
			By(fmt.Sprintf("  • Cluster: %s", clusterName))
			By(fmt.Sprintf("  • Kubeconfig: %s", kubeconfigPath))
			By(fmt.Sprintf("  • To connect: export KUBECONFIG=%s", kubeconfigPath))
			return
		}

		err := infrastructure.DeleteSignalProcessingCluster(clusterName, kubeconfigPath, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())
	},
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Test Helper Functions
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// createTestNamespace creates a uniquely named namespace for test isolation.
func createTestNamespace(prefix string, labels map[string]string) string {
	ns := fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   ns,
			Labels: labels,
		},
	}

	err := k8sClient.Create(ctx, namespace)
	Expect(err).ToNot(HaveOccurred())

	return ns
}

// deleteTestNamespace cleans up a test namespace.
func deleteTestNamespace(ns string) {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: ns},
	}
	_ = k8sClient.Delete(ctx, namespace)
}

// waitForSignalProcessingComplete waits for a SignalProcessing CR to reach Completed phase.
func waitForSignalProcessingComplete(name, namespace string) *signalprocessingv1alpha1.SignalProcessing {
	sp := &signalprocessingv1alpha1.SignalProcessing{}
	Eventually(func() string {
		err := k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, sp)
		if err != nil {
			return ""
		}
		return string(sp.Status.Phase)
	}, timeout, interval).Should(Equal("Completed"))
	return sp
}


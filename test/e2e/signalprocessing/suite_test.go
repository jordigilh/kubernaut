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
package signalprocessing

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

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
	coverageMode   bool   // E2E coverage capture mode (per E2E_COVERAGE_COLLECTION.md)
	coverDir       string // Coverage data directory
	anyTestFailed  bool   // Track test failures for cluster cleanup decision
	e2eAuthToken   string // DD-AUTH-014: ServiceAccount token for DataStorage authentication
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

		// Check for coverage mode (per E2E_COVERAGE_COLLECTION.md)
		coverageMode = os.Getenv("COVERAGE_MODE") == "true"
		if coverageMode {
			By("📊 E2E Coverage Mode ENABLED (per E2E_COVERAGE_COLLECTION.md)")
		}

		// Get home directory for kubeconfig
		homeDir, err := os.UserHomeDir()
		Expect(err).ToNot(HaveOccurred())

		// Standard kubeconfig location: ~/.kube/{service}-e2e-config
		// Per docs/development/business-requirements/TESTING_GUIDELINES.md
		kubeconfigPath = filepath.Join(homeDir, ".kube", fmt.Sprintf("%s-e2e-config", serviceName))

		By(fmt.Sprintf("Creating Kind cluster '%s'", clusterName))
		By(fmt.Sprintf("  • Kubeconfig: %s", kubeconfigPath))
		By(fmt.Sprintf("  • Metrics URL: http://localhost:9182/metrics"))

		ctx := context.Background()

		// Use hybrid parallel infrastructure setup per DD-TEST-002 (Dec 25, 2025)
		// Strategy: Build images in parallel → Create cluster → Load → Deploy
		// Benefits:
		// - 4x faster than sequential (5min vs 20min)
		// - 100% reliable (no Kind timeout issues)
		// - Coverage-enabled by default (per DD-TEST-007)
		//
		// This replaces both coverage and parallel approaches with a unified strategy
		err = infrastructure.SetupSignalProcessingInfrastructureHybridWithCoverage(ctx, clusterName, kubeconfigPath, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())

		// DD-AUTH-014: Create E2E ServiceAccount for DataStorage authentication
		By("🔐 Creating E2E ServiceAccount for DataStorage audit queries (DD-AUTH-014)")
		e2eSAName := "signalprocessing-e2e-sa"
		namespace := "kubernaut-system"
		
		err = infrastructure.CreateE2EServiceAccountWithDataStorageAccess(ctx, namespace, kubeconfigPath, e2eSAName, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred(), "Failed to create E2E ServiceAccount")
		
		// Get ServiceAccount token for Bearer authentication
		token, err := infrastructure.GetServiceAccountToken(ctx, namespace, e2eSAName, kubeconfigPath)
		Expect(err).ToNot(HaveOccurred(), "Failed to get E2E ServiceAccount token")
		By("✅ E2E ServiceAccount token retrieved for authenticated DataStorage access")

		// Return kubeconfig path, coverage mode flag, and auth token
		return []byte(fmt.Sprintf("%s|%t|%s", kubeconfigPath, coverageMode, token))
	},
	// This runs on ALL processes - connect to cluster
	func(data []byte) {
		// Parse data: "kubeconfig|coverageMode|authToken"
		parts := strings.Split(string(data), "|")
		kubeconfigPath = parts[0]
		if len(parts) > 1 {
			coverageMode = parts[1] == "true"
		}
		if len(parts) > 2 {
			e2eAuthToken = parts[2] // DD-AUTH-014: Store token for authenticated DataStorage access
		}

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

// Track test failures for cluster cleanup decision
var _ = ReportAfterEach(func(report SpecReport) {
	if report.Failed() {
		anyTestFailed = true
	}
})

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

		// Detect setup failure: if k8sClient is nil, BeforeSuite failed
		setupFailed := k8sClient == nil
		if setupFailed {
			By("⚠️  Setup failure detected (k8sClient is nil)")
		}

		// Determine test results for log export decision
		anyFailure := setupFailed || anyTestFailed
		preserveCluster := os.Getenv("KEEP_CLUSTER") != ""

		if preserveCluster {
			By("KEEP_CLUSTER set - preserving cluster for debugging")
			By(fmt.Sprintf("  • Cluster: %s", clusterName))
			By(fmt.Sprintf("  • Kubeconfig: %s", kubeconfigPath))
			By(fmt.Sprintf("  • To connect: export KUBECONFIG=%s", kubeconfigPath))
			return
		}

		// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
		// E2E_COVERAGE_COLLECTION.md: Coverage Extraction (before cluster deletion)
		// Coverage data is written when the controller binary exits gracefully.
		// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
		if coverageMode {
			By("📊 E2E Coverage Mode: Extracting coverage data before cleanup")

			// Get project root for coverage directory
			projectRoot, err := infrastructure.GetProjectRoot()
			if err == nil {
				coverDir = filepath.Join(projectRoot, "coverdata")
			}

			// Step 1: Scale down controller to trigger graceful exit
			By("Scaling down controller for graceful shutdown (coverage flush)")
			scaleCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
				"scale", "deployment", "signalprocessing-controller",
				"-n", "kubernaut-system", "--replicas=0")
			scaleOutput, scaleErr := scaleCmd.CombinedOutput()
			if scaleErr != nil {
				GinkgoWriter.Printf("⚠️  Scale down failed: %v\n%s\n", scaleErr, scaleOutput)
			}

			// Step 2: Wait for pod termination using Eventually (NOT time.Sleep - anti-pattern)
			By("Waiting for controller pod termination...")
			Eventually(func() bool {
				checkCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
					"get", "pods", "-n", "kubernaut-system",
					"-l", "app=signalprocessing-controller", "-o", "name")
				output, _ := checkCmd.Output()
				return len(strings.TrimSpace(string(output))) == 0
			}).WithTimeout(60*time.Second).WithPolling(2*time.Second).Should(BeTrue(),
				"Controller pod should terminate for coverage flush")

			// Step 3: Extract coverage from Kind node
			By("Extracting coverage data from Kind worker node")
			err = infrastructure.ExtractCoverageFromKind(clusterName, coverDir, GinkgoWriter)
			if err != nil {
				GinkgoWriter.Printf("⚠️  Coverage extraction failed: %v\n", err)
			}

			// Step 4: Generate coverage report
			By("Generating coverage report")
			err = infrastructure.GenerateCoverageReport(coverDir, GinkgoWriter)
			if err != nil {
				GinkgoWriter.Printf("⚠️  Coverage report generation failed: %v\n", err)
			}
		}

		// Delete cluster with must-gather log export
		// Delete Kind cluster using infrastructure helper (with failure tracking)
		Eventually(func() error {
			return infrastructure.DeleteSignalProcessingCluster(clusterName, kubeconfigPath, anyFailure, GinkgoWriter)
		}).WithTimeout(30*time.Second).WithPolling(5*time.Second).Should(Succeed(),
			"Cluster deletion should succeed (transient Podman connectivity handled via retry)")

		// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
		// DD-TEST-001 v1.1: Comprehensive Image Cleanup
		// Clean ALL images built for this E2E run to prevent disk exhaustion
		// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

		By("Cleaning up SignalProcessing service image (DD-TEST-001 v1.1)")
		spImageName := infrastructure.GetSignalProcessingFullImageName()
		spPruneCmd := exec.Command("podman", "rmi", spImageName)
		spPruneOutput, spPruneErr := spPruneCmd.CombinedOutput()
		if spPruneErr != nil {
			GinkgoWriter.Printf("⚠️  Failed to remove SP image %s: %v\n%s\n", spImageName, spPruneErr, spPruneOutput)
		} else {
			GinkgoWriter.Printf("✅ SP image removed: %s\n", spImageName)
		}

		By("Cleaning up DataStorage service image (DD-TEST-001 v1.1)")
		dsImageName := infrastructure.GetDataStorageImageTagForSP()
		dsPruneCmd := exec.Command("podman", "rmi", dsImageName)
		dsPruneOutput, dsPruneErr := dsPruneCmd.CombinedOutput()
		if dsPruneErr != nil {
			GinkgoWriter.Printf("⚠️  Failed to remove DS image %s: %v\n%s\n", dsImageName, dsPruneErr, dsPruneOutput)
		} else {
			GinkgoWriter.Printf("✅ DS image removed: %s\n", dsImageName)
		}

		By("Cleaning up temp tar files from image loading")
		imageTag := infrastructure.GetSignalProcessingImageTag()
		tmpFiles := []string{
			fmt.Sprintf("/tmp/signalprocessing-controller-%s.tar", imageTag),
			"/tmp/datastorage-e2e-sp.tar",
		}
		for _, tmpFile := range tmpFiles {
			if err := os.Remove(tmpFile); err == nil {
				GinkgoWriter.Printf("✅ Temp file removed: %s\n", tmpFile)
			}
		}

		By("Pruning dangling images from Kind builds (DD-TEST-001 v1.1)")
		pruneCmd := exec.Command("podman", "image", "prune", "-f")
		_, _ = pruneCmd.CombinedOutput()

		GinkgoWriter.Println("✅ E2E cleanup complete (DD-TEST-001 v1.1 compliant)")
	},
)

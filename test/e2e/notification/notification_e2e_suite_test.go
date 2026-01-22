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

package notification

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// Test suite for Notification E2E tests
// This suite sets up a complete production-like environment:
// - Kind cluster (2 nodes: 1 control-plane + 1 worker)
// - Notification Controller (deployed to Kind cluster)
// - FileService for message validation
//
// NOTE: Tests validate complete notification lifecycle with real Kubernetes infrastructure

func TestNotificationE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Notification E2E Suite")
}

var (
	ctx    context.Context
	cancel context.CancelFunc
	logger logr.Logger

	// Cluster configuration (shared across all tests)
	clusterName    string
	kubeconfigPath string
	cfg            *rest.Config
	k8sClient      client.Client

	// Shared Notification Controller configuration (deployed ONCE for all tests)
	controllerNamespace string = "notification-e2e"
	// E2E file output directory (for FileService validation)
	e2eFileOutputDir string

	// Data Storage NodePort for audit E2E tests (0 if not deployed)
	// When audit infrastructure is deployed, this is set to the NodePort for external access
	dataStorageNodePort int

	// Track if any test failed (for cluster cleanup decision)
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
			ServiceName: "notification-e2e-test",
		})

		// Initialize failure tracking
		anyTestFailed = false

		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("Notification E2E Test Suite - Cluster Setup (ONCE - Process 1)")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("Creating Kind cluster and deploying Notification Controller...")
		logger.Info("  • Kind cluster (2 nodes: control-plane + worker)")
		logger.Info("  • NotificationRequest CRD (cluster-wide)")
		logger.Info("  • Notification Controller Docker image (build + load)")
		logger.Info("  • Shared Notification Controller (notification-e2e namespace)")
		logger.Info("  • Kubeconfig: ~/.kube/notification-e2e-config")
		logger.Info("")
		logger.Info("Note: All tests share the same controller instance")
		logger.Info("      Tests use FileService for message validation")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Set cluster configuration
		clusterName = "notification-e2e"
		homeDir, err := os.UserHomeDir()
		Expect(err).ToNot(HaveOccurred())
		kubeconfigPath = fmt.Sprintf("%s/.kube/notification-e2e-config", homeDir)

		// Skip cluster deletion on initial run to avoid infrastructure hang
		// Clean cluster state ensured by manual cleanup between test runs
		// TODO: Fix podman `kind get clusters` hang for automated cleanup
		logger.Info("Skipping cluster deletion (clean state assumed)...")

		// E2E file delivery directory is created by infrastructure.CreateNotificationCluster
		// No need to create it here - it's done before Kind cluster creation

		// Create Kind cluster (ONCE for all tests) - returns notification image name
		var notificationImageName string
		notificationImageName, err = infrastructure.CreateNotificationCluster(clusterName, kubeconfigPath, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())
		Expect(notificationImageName).ToNot(BeEmpty(), "Notification image name must not be empty")

		// Set KUBECONFIG environment variable
		err = os.Setenv("KUBECONFIG", kubeconfigPath)
		Expect(err).ToNot(HaveOccurred())

		// Deploy shared Notification Controller (ONCE for all tests) - pass image name from setup
		logger.Info("Deploying shared Notification Controller...")
		logger.Info("  • Using image: " + notificationImageName)
		err = infrastructure.DeployNotificationController(ctx, controllerNamespace, kubeconfigPath, notificationImageName, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())

		// Wait for controller pod to be ready
		logger.Info("⏳ Waiting for Notification Controller pod to be ready...")
		waitCmd := exec.Command("kubectl", "wait",
			"-n", controllerNamespace,
			"--for=condition=ready",
			"pod",
			"-l", "app=notification-controller",
			"--timeout=120s",
			"--kubeconfig", kubeconfigPath)
		waitCmd.Stdout = GinkgoWriter
		waitCmd.Stderr = GinkgoWriter
		err = waitCmd.Run()
		Expect(err).ToNot(HaveOccurred(), "Notification Controller pod did not become ready")
		logger.Info("✅ Notification Controller pod is ready")

		// Deploy Audit Infrastructure (PostgreSQL + Data Storage + migrations)
		// Required for BR-NOT-062, BR-NOT-063, BR-NOT-064 E2E tests
		logger.Info("Deploying Audit Infrastructure (PostgreSQL + Data Storage)...")
		err = infrastructure.DeployNotificationAuditInfrastructure(ctx, controllerNamespace, kubeconfigPath, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred(), "Audit infrastructure deployment should succeed")
		logger.Info("✅ Audit infrastructure ready")

		// Deploy AuthWebhook manifests (using pre-built + pre-loaded image from PHASE 1 & 3)
		// Per DD-WEBHOOK-001: Required for NotificationRequest DELETE operations
		// Per SOC2 CC8.1: Captures WHO cancelled notifications
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("🔐 Deploying AuthWebhook Manifests")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		awImage := os.Getenv("E2E_AUTHWEBHOOK_IMAGE")
		Expect(awImage).ToNot(BeEmpty(), "AuthWebhook image should be set by infrastructure")
		err = infrastructure.DeployAuthWebhookManifestsOnly(ctx, clusterName, controllerNamespace, kubeconfigPath, awImage, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred(), "AuthWebhook manifest deployment should succeed")
		logger.Info("✅ AuthWebhook deployed - SOC2 CC8.1 cancellation attribution enabled")

		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("Cluster Setup Complete - Ready for parallel processes")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Return kubeconfig path to all processes
		return []byte(kubeconfigPath)
	},
	// This runs on ALL processes (including process 1) - connects to cluster
	func(data []byte) {
		// Initialize context for this process
		ctx, cancel = context.WithCancel(context.Background())

		// Initialize logger for this process
		logger = kubelog.NewLogger(kubelog.Options{
			Development: true,
			Level:       0, // INFO
			ServiceName: fmt.Sprintf("notification-e2e-test-process-%d", GinkgoParallelProcess()),
		})

		// Initialize failure tracking
		anyTestFailed = false

		// Get kubeconfig path from process 1
		kubeconfigPath = string(data)

		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━",
			"process", GinkgoParallelProcess())
		logger.Info("Notification E2E Process Setup",
			"process", GinkgoParallelProcess(),
			"kubeconfig", kubeconfigPath)

		// Load kubeconfig
		config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		Expect(err).ToNot(HaveOccurred())
		cfg = config

		// Create Kubernetes client
		err = notificationv1alpha1.AddToScheme(scheme.Scheme)
		Expect(err).ToNot(HaveOccurred())

		k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
		Expect(err).ToNot(HaveOccurred())
		Expect(k8sClient).ToNot(BeNil())

		// Set up E2E file output directory for FileService validation
		// Use platform-appropriate HostPath directory (created by infrastructure.CreateNotificationCluster)
		e2eFileOutputDir, err = infrastructure.GetE2EFileOutputDir()
		Expect(err).ToNot(HaveOccurred())
		// Directory already created in SynchronizedBeforeSuite before Kind cluster creation
		// No need to create here - just verify it exists
		_, err = os.Stat(e2eFileOutputDir)
		Expect(err).ToNot(HaveOccurred(), "HostPath directory should exist")

		// Wait for Notification Controller metrics NodePort to be responsive
		// NodePort 30186 (in cluster) → localhost:9186 (on host via Kind extraPortMappings)
		// Per DD-TEST-001 port allocation strategy
		logger.Info("⏳ Waiting for Notification Controller metrics NodePort to be responsive...")

		// Give Kind a moment to set up port forwarding after deployment
		logger.Info("Waiting 5 seconds for Kind port mapping to stabilize...")
		time.Sleep(5 * time.Second)

		metricsURL := "http://localhost:9186/metrics" // Per DD-TEST-001
		httpClient := &http.Client{Timeout: 10 * time.Second}

		attemptCount := 0
		Eventually(func() error {
			attemptCount++
			resp, err := httpClient.Get(metricsURL)
			if err != nil {
				if attemptCount%10 == 0 { // Log every 10th attempt
					logger.Info("Still waiting for metrics endpoint...",
						"attempt", attemptCount,
						"error", err.Error())
				}
				return err
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("metrics endpoint returned status %d", resp.StatusCode)
			}
			return nil
		}, 90*time.Second, 2*time.Second).Should(Succeed(), "Notification Controller metrics NodePort did not become responsive")
		logger.Info("✅ Notification Controller metrics accessible via NodePort",
			"process", GinkgoParallelProcess(),
			"url", metricsURL,
			"attempts", attemptCount)

		// Set Data Storage NodePort for E2E audit tests
		// Per DD-TEST-001: Data Storage uses NodePort 30090 in Kind clusters
		// Per TESTING_GUIDELINES.md: E2E tests MUST use real services (Skip() forbidden)
		dataStorageNodePort = 30090
		logger.Info("✅ Data Storage NodePort configured for audit E2E tests",
			"process", GinkgoParallelProcess(),
			"port", dataStorageNodePort)

		logger.Info("✅ Process ready",
			"process", GinkgoParallelProcess(),
			"fileOutputDir", e2eFileOutputDir)
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	},
)

// Track test failures for cluster cleanup decision
var _ = ReportAfterEach(func(report SpecReport) {
	if report.Failed() {
		anyTestFailed = true
	}
})

// SynchronizedAfterSuite runs process cleanup then cluster cleanup
var _ = SynchronizedAfterSuite(
	// This runs on ALL processes - cleans up per-process resources
	func() {
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━",
			"process", GinkgoParallelProcess())
		logger.Info("Notification E2E Process Cleanup", "process", GinkgoParallelProcess())

		// NOTE: Do NOT clean up e2eFileOutputDir here!
		// It's a shared HostPath directory used by all parallel processes.
		// If we clean it here, parallel processes will race and delete each other's files.
		// Cleanup happens in cluster cleanup (second function) after all tests complete.

		// Cancel context
		if cancel != nil {
			cancel()
		}

		logger.Info("✅ Process cleanup complete", "process", GinkgoParallelProcess())
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	},
	// This runs ONCE on process 1 - cleans up shared cluster
	func() {
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("Notification E2E Cluster Cleanup (ONCE - Process 1)")

		// Detect setup failure: if k8sClient is nil, BeforeSuite failed
		setupFailed := k8sClient == nil
		if setupFailed {
			logger.Info("⚠️  Setup failure detected (k8sClient is nil)")
		}

		// Determine test results for log export decision
		anyFailure := setupFailed || anyTestFailed
		preserveCluster := os.Getenv("KEEP_CLUSTER") == "true"

		// Keep cluster alive only if explicitly requested for manual debugging
		if preserveCluster {
			logger.Info("⚠️  CLUSTER PRESERVED FOR DEBUGGING (KEEP_CLUSTER=true)")
			logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			logger.Info("🔍 CLUSTER DEBUGGING INFORMATION:")
			logger.Info("  Cluster name: notification-e2e")
			logger.Info("  Namespace: notification-e2e")
			logger.Info("  Kubeconfig: ~/.kube/notification-e2e-config")
			logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			logger.Info("📋 REQUIRED DIAGNOSTIC COMMANDS (per DS team):")
			logger.Info("  Step 1: kubectl get pods -n notification-e2e -l app=datastorage -o wide")
			logger.Info("  Step 2: kubectl logs -n notification-e2e -l app=datastorage --tail=100")
			logger.Info("  Step 3: kubectl get events -n notification-e2e --field-selector involvedObject.kind=Pod | grep datastorage")
			logger.Info("  Step 4a: kubectl get configmap datastorage-config -n notification-e2e -o yaml")
			logger.Info("  Step 4b: kubectl get secret datastorage-secret -n notification-e2e -o jsonpath='{.data}'")
			logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			logger.Info("🗑️  TO DELETE CLUSTER WHEN DONE:")
			logger.Info("  kind delete cluster --name notification-e2e")
			logger.Info("  rm ~/.kube/notification-e2e-config")
			logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			return // Skip cluster deletion
		}

		// Delete cluster (with must-gather log export on failure)
		logger.Info("Deleting Kind cluster...")
		err := infrastructure.DeleteNotificationCluster(clusterName, kubeconfigPath, anyFailure, GinkgoWriter)
		if err != nil {
			logger.Error(err, "Failed to delete Kind cluster (non-fatal)")
		} else {
			logger.Info("✅ Kind cluster deleted")
		}

		// DD-TEST-001 v1.1: Clean up service images built for Kind
		// Prevents disk space accumulation (~200-500MB per run)
		logger.Info("Cleaning up Notification controller image built for Kind...")
		imageTag := "e2e-test" // Default tag used for E2E tests
		imageName := "localhost/kubernaut-notification:" + imageTag

		pruneCmd := exec.Command("podman", "rmi", imageName)
		_, pruneErr := pruneCmd.CombinedOutput()
		if pruneErr != nil {
			logger.Info("⚠️  Failed to remove service image (may not exist)",
				"image", imageName, "error", pruneErr)
		} else {
			logger.Info("✅ Service image removed", "image", imageName)
		}

		// Prune dangling images from failed builds
		logger.Info("Pruning dangling images from Kind builds...")
		danglingPruneCmd := exec.Command("podman", "image", "prune", "-f")
		_, _ = danglingPruneCmd.CombinedOutput()
		logger.Info("✅ Dangling images pruned")

		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("Notification E2E Cluster Cleanup Complete")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	},
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Helper Functions
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// WaitForNotificationPhase waits for a NotificationRequest to reach a specific phase
func WaitForNotificationPhase(ctx context.Context, client client.Client, namespace, name string, expectedPhase notificationv1alpha1.NotificationPhase, timeout time.Duration) {
	notif := &notificationv1alpha1.NotificationRequest{}
	Eventually(func() notificationv1alpha1.NotificationPhase {
		err := client.Get(ctx, clientKey(namespace, name), notif)
		if err != nil {
			return ""
		}
		return notif.Status.Phase
	}, timeout, 500*time.Millisecond).Should(Equal(expectedPhase))
}

// clientKey creates a types.NamespacedName for namespace/name lookups
func clientKey(namespace, name string) types.NamespacedName {
	return types.NamespacedName{Namespace: namespace, Name: name}
}

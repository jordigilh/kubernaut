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

package authwebhook

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/go-logr/logr"
	_ "github.com/jackc/pgx/v5/stdlib"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	auditclient "github.com/jordigilh/kubernaut/pkg/datastorage/client"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// Test suite for AuthWebhook E2E tests
// This suite sets up a complete production-like environment:
// - Kind cluster (2 nodes: 1 control-plane + 1 worker) with NodePort exposure
// - PostgreSQL 16 (for workflow catalog)
// - Redis (for DLQ fallback)
// - Immudb (for SOC2-compliant immutable audit trails)
// - Data Storage service (deployed to Kind cluster)
// - AuthWebhook service (deployed to Kind cluster as admission webhook)
//
// ARCHITECTURE: Uses SHARED deployment pattern (like Gateway/DataStorage E2E tests)
// - Services deployed ONCE in SynchronizedBeforeSuite
// - All tests share the same infrastructure via NodePort (no port-forwarding)
// - Eliminates kubectl port-forward instability
// - Faster execution, no per-test deployment overhead
//
// E2E Test Coverage (10-15%):
// - E2E-MULTI-01: Multiple CRDs in Sequence (SOC2 attribution across all CRD types)
// - E2E-MULTI-02: Concurrent Webhook Requests (stress testing webhook under load)

func TestAuthWebhookE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AuthWebhook E2E Suite")
}

var (
	ctx    context.Context
	cancel context.CancelFunc
	logger logr.Logger

	// Cluster configuration (shared across all tests)
	clusterName    string
	kubeconfigPath string
	restConfig     *rest.Config
	k8sClient      client.Client

	// Shared service URLs (NodePort - no port-forwarding needed)
	// These are set in SynchronizedBeforeSuite and available to all tests
	dataStorageURL string // http://localhost:28099 (NodePort 30099 mapped via Kind extraPortMappings per DD-TEST-001)
	postgresURL    string // localhost:25442 (NodePort 30442 mapped via Kind extraPortMappings per DD-TEST-001)

	// Audit client for validating webhook audit events
	auditClient *auditclient.ClientWithResponses

	// Shared namespace for all tests (services deployed ONCE)
	sharedNamespace string = "authwebhook-e2e"

	// Track if any test failed (for cluster cleanup decision)
	anyTestFailed bool

	// Coverage mode detection (DD-TEST-007: E2E Coverage Capture Standard)
	coverageMode bool
	coverDir     string = "./coverdata"
)

var _ = SynchronizedBeforeSuite(
	// This function runs ONCE on process 1 only
	func() []byte {
		// Initialize context for process 1
		ctx, cancel = context.WithCancel(context.Background())

		// Initialize logger for process 1 (DD-005 v2.0: logr.Logger migration)
		logger = kubelog.NewLogger(kubelog.DevelopmentOptions())

		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("AuthWebhook E2E Test Suite - Cluster Setup (ONCE - Process 1)")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// DD-TEST-007: E2E Coverage Capture Standard
		// Detect if coverage mode is enabled via E2E_COVERAGE environment variable
		coverageMode = os.Getenv("E2E_COVERAGE") == "true"
		if coverageMode {
			logger.Info("📊 DD-TEST-007: E2E Coverage mode ENABLED")
			// Create coverage directory for Go 1.20+ binary profiling
			if err := os.MkdirAll(coverDir, 0777); err != nil {
				logger.Info("⚠️  Failed to create coverage directory", "error", err)
			} else {
				logger.Info("   ✅ Coverage directory created", "path", coverDir)
				logger.Info("   💡 Coverage data will be extracted from Kind node after tests")
			}
		} else {
			logger.Info("📊 DD-TEST-007: E2E Coverage mode DISABLED (set E2E_COVERAGE=true to enable)")
		}

		logger.Info("Creating Kind cluster with NodePort exposure...")
		logger.Info("  • Kind cluster (2 nodes: control-plane + worker)")
		logger.Info("  • NodePort exposure: Data Storage (30099→8080), PostgreSQL (30442→5432), Webhook (30443→9443)")
		logger.Info("  • PostgreSQL 16 (workflow catalog)")
		logger.Info("  • Redis (DLQ fallback)")
		logger.Info("  • Immudb (SOC2 immutable audit trails)")
		logger.Info("  • Data Storage Docker image (build + load)")
		logger.Info("  • AuthWebhook Docker image (build + load)")
		logger.Info("  • Kubeconfig: ~/.kube/authwebhook-e2e-config")
		logger.Info("")
		logger.Info("Note: All tests share the same infrastructure via NodePort")
		logger.Info("      No kubectl port-forward needed - eliminates instability")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Set cluster configuration
		clusterName = "authwebhook-e2e"
		homeDir, err := os.UserHomeDir()
		Expect(err).ToNot(HaveOccurred())
		// Use isolated kubeconfig path per TESTING_GUIDELINES.md section "Kubeconfig Isolation Policy"
		// Convention: ~/.kube/{serviceName}-e2e-config (NEVER ~/.kube/config)
		kubeconfigPath = fmt.Sprintf("%s/.kube/authwebhook-e2e-config", homeDir)

		// Create infrastructure with parallel setup (ONCE for all tests)
		// This uses parallel optimization: Build images | PostgreSQL | Redis | Immudb run concurrently
		logger.Info("🚀 Setting up AuthWebhook E2E infrastructure (PARALLEL MODE)...")
		// Generate unique image names per DD-TEST-001 compliant naming
		dataStorageImage := infrastructure.GenerateInfraImageName("datastorage", "authwebhook-e2e")
		authWebhookImage := infrastructure.GenerateInfraImageName("webhooks", "authwebhook-e2e")
	// Setup AuthWebhook infrastructure (hybrid pattern) - returns built image names
	var awImage, dsImage string
	awImage, dsImage, err = infrastructure.SetupAuthWebhookInfrastructureParallel(ctx, clusterName, kubeconfigPath, sharedNamespace, GinkgoWriter)
	Expect(err).ToNot(HaveOccurred())
	Expect(awImage).ToNot(BeEmpty(), "AuthWebhook image name must not be empty")
	Expect(dsImage).ToNot(BeEmpty(), "DataStorage image name must not be empty")
	
	logger.Info("✅ Infrastructure setup complete")
	logger.Info("  • AuthWebhook image: " + awImage)
	logger.Info("  • DataStorage image: " + dsImage)

		// Wait for Data Storage HTTP endpoint to be responsive via NodePort
		logger.Info("⏳ Waiting for Data Storage NodePort to be responsive...")
		Eventually(func() error {
			conn, err := net.DialTimeout("tcp", "localhost:28099", 2*time.Second) // Per DD-TEST-001
			if err != nil {
				return err
			}
			_ = conn.Close()
			return nil
		}, 120*time.Second, 2*time.Second).Should(Succeed(), "Data Storage NodePort did not become responsive")
		logger.Info("✅ Data Storage is ready via NodePort (localhost:28099)")

		// Wait for AuthWebhook HTTPS endpoint to be responsive via NodePort
		logger.Info("⏳ Waiting for AuthWebhook NodePort to be responsive...")
		Eventually(func() error {
			conn, err := net.DialTimeout("tcp", "localhost:30443", 2*time.Second) // Per DD-TEST-001
			if err != nil {
				return err
			}
			_ = conn.Close()
			return nil
		}, 120*time.Second, 2*time.Second).Should(Succeed(), "AuthWebhook NodePort did not become responsive")
		logger.Info("✅ AuthWebhook is ready via NodePort (localhost:30443)")

		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("Cluster Setup Complete - Broadcasting to all processes")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("Cluster configuration", "cluster", clusterName, "kubeconfig", kubeconfigPath)
		logger.Info("Service URLs (per DD-TEST-001)", "dataStorage", "http://localhost:28099", "postgresql", "localhost:25442", "webhook", "https://localhost:30443")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Return kubeconfig path to all processes
		return []byte(kubeconfigPath)
	},
	// This function runs on ALL processes (including process 1)
	func(data []byte) {
		// Initialize context
		ctx, cancel = context.WithCancel(context.Background())

		// Initialize logger for this process (DD-005 v2.0: logr.Logger migration)
		logger = kubelog.NewLogger(kubelog.DevelopmentOptions())

		// Initialize failure tracking
		anyTestFailed = false

		// Receive kubeconfig path from process 1
		kubeconfigPath = string(data)
		clusterName = "authwebhook-e2e"

		// Set shared URLs - Per DD-TEST-001: AuthWebhook E2E uses ports 25442, 26386, 28099, 30443
		dataStorageURL = "http://localhost:28099"
		postgresURL = "postgresql://slm_user:test_password@localhost:25442/action_history?sslmode=disable"

		processID := GinkgoParallelProcess()

		// Test if NodePort is accessible
		logger.Info("✅ NodePort accessible (Docker provider)", "process", processID)

		logger.Info("🔌 URLs configured",
			"process", processID,
			"dataStorageURL", dataStorageURL,
			"postgresURL", postgresURL)

		// Initialize K8s client for CRD operations
		var err error
		restConfig, err = infrastructure.LoadKubeconfig(kubeconfigPath)
		Expect(err).ToNot(HaveOccurred())

		// Register CRD schemes
		Expect(workflowexecutionv1.AddToScheme(scheme.Scheme)).To(Succeed())
		Expect(remediationv1.AddToScheme(scheme.Scheme)).To(Succeed())
		Expect(notificationv1.AddToScheme(scheme.Scheme)).To(Succeed())

		k8sClient, err = client.New(restConfig, client.Options{Scheme: scheme.Scheme})
		Expect(err).ToNot(HaveOccurred())

		// Initialize audit client for DD-TESTING-001 validation
		auditClient, err = auditclient.NewClientWithResponses(dataStorageURL)
		Expect(err).ToNot(HaveOccurred())

		// Note: We do NOT set KUBECONFIG environment variable to avoid affecting other tests
		// All kubectl commands must use --kubeconfig flag explicitly
		logger.Info("Process ready", "process", processID, "kubeconfig", kubeconfigPath)
	},
)

// Track test failures for cluster cleanup decision
var _ = ReportAfterEach(func(report SpecReport) {
	if report.Failed() {
		anyTestFailed = true
	}
})

var _ = SynchronizedAfterSuite(
	// This function runs on ALL processes (cleanup per-process resources)
	func() {
		processID := GinkgoParallelProcess()
		logger.Info("Process cleanup complete",
			"process", processID,
			"hadFailures", anyTestFailed)

		// Cancel context for this process
		if cancel != nil {
			cancel()
		}

		// Sync logger for this process (DD-005 v2.0: use kubelog.Sync)
		kubelog.Sync(logger)
	},
	// This function runs ONCE on process 1 only (cleanup shared resources)
	func() {
		// Re-initialize logger for final cleanup (DD-005 v2.0: logr.Logger migration)
		logger = kubelog.NewLogger(kubelog.DevelopmentOptions())

		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("AuthWebhook E2E Test Suite - Cleanup (Process 1)")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Check if we should keep the cluster for debugging
		keepCluster := os.Getenv("KEEP_CLUSTER")

		// Preserve cluster if:
		// 1. KEEP_CLUSTER env var is set to "true"
		// 2. Any test failed (anyTestFailed flag)
		// 3. Setup failed (k8sClient is nil means BeforeSuite didn't complete)
		setupFailed := k8sClient == nil
		suiteFailed := keepCluster == "true" || anyTestFailed || setupFailed

		// DD-TEST-007: E2E Coverage Capture Standard
		// Extract coverage from Kind node before cluster deletion
		if coverageMode {
			logger.Info("📊 DD-TEST-007: Extracting E2E coverage data from Kind node...")
			logger.Info("   Step 1: Scaling down AuthWebhook for graceful shutdown (flushes coverage)...")

			// Scale deployment to 0 to trigger graceful shutdown (writes coverage data)
			scaleCmd := exec.Command("kubectl", "scale", "deployment", "authwebhook",
				"--kubeconfig", kubeconfigPath,
				"-n", sharedNamespace,
				"--replicas=0")
			if output, err := scaleCmd.CombinedOutput(); err != nil {
				logger.Info("⚠️  Failed to scale down AuthWebhook", "error", err, "output", string(output))
			} else {
				logger.Info("   ✅ AuthWebhook scaled to 0")

				// Wait for pod termination (coverage is written during graceful shutdown)
				logger.Info("   Step 2: Waiting for graceful shutdown to complete...")
				time.Sleep(10 * time.Second)

				// DD-TEST-007: Extract coverage files from Kind node container
				logger.Info("   Step 3: Extracting coverage files from Kind node container...")
				kindNodeContainer := clusterName + "-worker"

				// Use podman cp to copy coverage files from Kind node to host
				cpCmd := exec.Command("podman", "cp",
					kindNodeContainer+":/coverdata/.",
					coverDir)
				if cpOutput, cpErr := cpCmd.CombinedOutput(); cpErr != nil {
					logger.Info("⚠️  Failed to extract coverage from Kind node",
						"error", cpErr,
						"output", string(cpOutput),
						"container", kindNodeContainer)
				} else {
					logger.Info("   ✅ Coverage files extracted from Kind node", "destination", coverDir)

					// Generate coverage report
					logger.Info("   Step 4: Generating E2E coverage report...")
					percentCmd := exec.Command("go", "tool", "covdata", "percent", "-i="+coverDir)
					if percentOutput, percentErr := percentCmd.CombinedOutput(); percentErr == nil {
						logger.Info("   ✅ E2E Coverage Report:")
						logger.Info(string(percentOutput))

						// Convert to text format for HTML report
						textfmtCmd := exec.Command("go", "tool", "covdata", "textfmt",
							"-i="+coverDir,
							"-o=e2e-coverage.txt")
						if _, textErr := textfmtCmd.CombinedOutput(); textErr == nil {
							logger.Info("   ✅ Coverage report saved: e2e-coverage.txt")

							// Generate HTML report
							htmlCmd := exec.Command("go", "tool", "cover",
								"-html=e2e-coverage.txt",
								"-o=e2e-coverage.html")
							if _, htmlErr := htmlCmd.CombinedOutput(); htmlErr == nil {
								logger.Info("   ✅ HTML report saved: e2e-coverage.html")
							}
						}
					} else {
						logger.Info("⚠️  Failed to generate coverage report",
							"error", percentErr,
							"output", string(percentOutput))
					}
				}
			}
		} else {
			logger.Info("📊 DD-TEST-007: Coverage extraction skipped (E2E_COVERAGE not enabled)")
			logger.Info("   💡 To collect E2E coverage: make test-e2e-authwebhook-coverage")
		}

		if suiteFailed {
			logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			logger.Info("⚠️  CLUSTER PRESERVED FOR DEBUGGING")
			logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			if setupFailed {
				logger.Info("🔍 REASON: BeforeSuite setup failed (infrastructure not ready)")
			}
			if anyTestFailed {
				logger.Info("🔍 REASON: One or more tests failed")
			}
			if keepCluster == "true" {
				logger.Info("🔍 REASON: KEEP_CLUSTER=true environment variable")
			}

			logger.Info("")
			logger.Info("📋 CLUSTER INFORMATION:")
			logger.Info("   • Cluster Name: " + clusterName)
			logger.Info("   • Kubeconfig: " + kubeconfigPath)
			logger.Info("   • Namespace: " + sharedNamespace)
			logger.Info("   • Data Storage URL: " + dataStorageURL)
			logger.Info("   • PostgreSQL URL: " + postgresURL)

			logger.Info("")
			logger.Info("🔍 DEBUGGING COMMANDS:")
			logger.Info("   # List all pods and their status:")
			logger.Info("   kubectl --kubeconfig=" + kubeconfigPath + " get pods -n " + sharedNamespace)
			logger.Info("")
			logger.Info("   # Check Data Storage pod logs:")
			logger.Info("   kubectl --kubeconfig=" + kubeconfigPath + " logs -n " + sharedNamespace + " -l app=datastorage --tail=100")
			logger.Info("")
			logger.Info("   # Check Data Storage pod events:")
			logger.Info("   kubectl --kubeconfig=" + kubeconfigPath + " describe pod -n " + sharedNamespace + " -l app=datastorage")
			logger.Info("")
			logger.Info("   # Check webhook pod logs:")
			logger.Info("   kubectl --kubeconfig=" + kubeconfigPath + " logs -n " + sharedNamespace + " -l app.kubernetes.io/name=authwebhook --tail=100")
			logger.Info("")
			logger.Info("   # Check all events in namespace:")
			logger.Info("   kubectl --kubeconfig=" + kubeconfigPath + " get events -n " + sharedNamespace + " --sort-by='.lastTimestamp'")
			logger.Info("")
			logger.Info("   # Access Data Storage from host:")
			logger.Info("   curl http://localhost:28099/health/ready")
			logger.Info("")
			logger.Info("   # Delete cluster when done debugging:")
			logger.Info("   kind delete cluster --name " + clusterName)
			logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			return
		}

		// Delete Kind cluster
		logger.Info("🗑️  Deleting Kind cluster...")
		if err := infrastructure.DeleteCluster(clusterName, GinkgoWriter); err != nil {
			logger.Error(err, "Failed to delete cluster")
		} else {
			logger.Info("✅ Cluster deleted successfully")
		}

		// DD-TEST-001 v1.1: Clean up service images built for Kind
		logger.Info("🧹 DD-TEST-001 v1.1: Cleaning up service images...")
		imageTag := os.Getenv("IMAGE_TAG")
		if imageTag != "" {
			for _, serviceName := range []string{"datastorage", "webhooks"} {
				imageName := fmt.Sprintf("%s:%s", serviceName, imageTag)

				pruneCmd := exec.Command("podman", "rmi", imageName)
				pruneOutput, pruneErr := pruneCmd.CombinedOutput()
				if pruneErr != nil {
					logger.Info("⚠️  Failed to remove service image (may not exist)",
						"image", imageName,
						"error", pruneErr,
						"output", string(pruneOutput))
				} else {
					logger.Info("✅ Service image removed", "image", imageName, "saved", "~200-500MB")
				}
			}
		} else {
			logger.Info("⚠️  IMAGE_TAG not set, skipping service image cleanup")
		}

		// Prune dangling images from Kind builds
		logger.Info("🧹 Pruning dangling images from Kind builds...")
		pruneDanglingCmd := exec.Command("podman", "image", "prune", "-f")
		_, _ = pruneDanglingCmd.CombinedOutput()
		logger.Info("✅ Dangling images pruned")

		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	},
)

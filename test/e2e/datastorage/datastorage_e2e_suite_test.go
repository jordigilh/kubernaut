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

package datastorage

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/go-logr/logr"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// Test suite for Data Storage E2E tests
// This suite sets up a complete production-like environment:
// - Kind cluster (2 nodes: 1 control-plane + 1 worker) with NodePort exposure
// - PostgreSQL with pgvector (for audit events storage)
// - Redis (for DLQ fallback)
// - Data Storage service (deployed to Kind cluster)
//
// ARCHITECTURE: Uses SHARED deployment pattern (like Gateway E2E tests)
// - Services deployed ONCE in SynchronizedBeforeSuite
// - All tests share the same infrastructure via NodePort (no port-forwarding)
// - Eliminates kubectl port-forward instability
// - Faster execution, no per-test deployment overhead
//
// E2E Test Coverage (10-15%):
// - Scenario 1: Happy Path - Complete remediation audit trail
// - Scenario 2: DLQ Fallback - Data Storage Service outage recovery
// - Scenario 3: Query API - Timeline retrieval with filtering
// - Scenario 4: Workflow Search - Hybrid weighted scoring
// - Scenario 5: Embedding Service - Automatic embedding generation
// - Scenario 6: Workflow Search Audit Trail - Audit event generation

func TestDataStorageE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Data Storage E2E Suite")
}

var (
	ctx    context.Context
	cancel context.CancelFunc
	logger logr.Logger

	// Cluster configuration (shared across all tests)
	clusterName    string
	kubeconfigPath string

	// Shared service URLs (NodePort - no port-forwarding needed)
	// These are set in SynchronizedBeforeSuite and available to all tests
	dataStorageURL string // http://localhost:8081 (NodePort 30081 mapped via Kind extraPortMappings)
	postgresURL    string // localhost:5432 (NodePort 30432 mapped via Kind extraPortMappings)

	// Shared namespace for all tests (services deployed ONCE)
	sharedNamespace string = "datastorage-e2e"

	// Track if any test failed (for cluster cleanup decision)
	anyTestFailed bool
)

// Note: Helper functions (generateUniqueNamespace, createNamespace, deleteNamespace, etc.)
// are defined in helpers.go to avoid duplication

var _ = SynchronizedBeforeSuite(
	// This function runs ONCE on process 1 only
	func() []byte {
		// Initialize context for process 1
		ctx, cancel = context.WithCancel(context.Background())

		// Initialize logger for process 1 (DD-005 v2.0: logr.Logger migration)
		logger = kubelog.NewLogger(kubelog.DevelopmentOptions())

		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("Data Storage E2E Test Suite - Cluster Setup (ONCE - Process 1)")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("Creating Kind cluster with NodePort exposure...")
		logger.Info("  • Kind cluster (2 nodes: control-plane + worker)")
		logger.Info("  • NodePort exposure: Data Storage (30081→8081), PostgreSQL (30432→5432)")
		logger.Info("  • PostgreSQL with pgvector (audit events storage)")
		logger.Info("  • Redis (DLQ fallback)")
		logger.Info("  • Data Storage Docker image (build + load)")
		logger.Info("  • Kubeconfig: ~/.kube/datastorage-e2e-config")
		logger.Info("")
		logger.Info("Note: All tests share the same infrastructure via NodePort")
		logger.Info("      No kubectl port-forward needed - eliminates instability")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Set cluster configuration
		clusterName = "datastorage-e2e"
		homeDir, err := os.UserHomeDir()
		Expect(err).ToNot(HaveOccurred())
		// Use isolated kubeconfig path per TESTING_GUIDELINES.md section "Kubeconfig Isolation Policy"
		// Convention: ~/.kube/{serviceName}-e2e-config (NEVER ~/.kube/config)
		kubeconfigPath = fmt.Sprintf("%s/.kube/datastorage-e2e-config", homeDir)

		// Create Kind cluster with NodePort exposure (ONCE for all tests)
		err = infrastructure.CreateDataStorageCluster(clusterName, kubeconfigPath, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())

		// Deploy shared services (PostgreSQL, Redis, Data Storage) ONCE
		logger.Info("🚀 Deploying SHARED services in namespace: " + sharedNamespace)
		err = infrastructure.DeployDataStorageTestServices(ctx, sharedNamespace, kubeconfigPath, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())

		// Wait for Data Storage HTTP endpoint to be responsive via NodePort
		logger.Info("⏳ Waiting for Data Storage NodePort to be responsive...")
		httpClient := &http.Client{Timeout: 10 * time.Second}
		Eventually(func() error {
			resp, err := httpClient.Get("http://localhost:8081/health/ready")
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("health check returned status %d", resp.StatusCode)
			}
			return nil
		}, 120*time.Second, 2*time.Second).Should(Succeed(), "Data Storage NodePort did not become responsive")
		logger.Info("✅ Data Storage is ready via NodePort (localhost:8081)")

		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("Cluster Setup Complete - Broadcasting to all processes")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("Cluster configuration", "cluster", clusterName, "kubeconfig", kubeconfigPath)
		logger.Info("Service URLs", "dataStorage", "http://localhost:8081", "postgresql", "localhost:5432")
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
		clusterName = "datastorage-e2e"

		// Set shared URLs (NodePort - no port-forwarding needed)
		// These are exposed via Kind extraPortMappings in kind-datastorage-config.yaml
		dataStorageURL = "http://localhost:8081"                                                          // NodePort 30081 mapped to localhost:8081
		postgresURL = "postgresql://slm_user:test_password@localhost:5432/action_history?sslmode=disable" // NodePort 30432 mapped to localhost:5432

		processID := GinkgoParallelProcess()
		logger.Info("🔌 Using NodePort URLs (no port-forward needed)",
			"process", processID,
			"dataStorageURL", dataStorageURL,
			"postgresURL", postgresURL)

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
		logger.Info("Data Storage E2E Test Suite - Cleanup (Process 1)")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Check if we should keep the cluster for debugging
		keepCluster := os.Getenv("KEEP_CLUSTER")
		suiteReport := CurrentSpecReport()
		suiteFailed := suiteReport.Failed() || anyTestFailed || keepCluster == "true"

		if suiteFailed {
			logger.Info("⚠️  Keeping cluster for debugging (KEEP_CLUSTER=true or test failed)")
			logger.Info("Cluster details for debugging",
				"cluster", clusterName,
				"kubeconfig", kubeconfigPath,
				"dataStorageURL", dataStorageURL,
				"postgresURL", postgresURL)
			logger.Info("To delete the cluster manually: kind delete cluster --name " + clusterName)
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

		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	},
)

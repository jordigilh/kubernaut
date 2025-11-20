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
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// Test suite for Data Storage E2E tests
// This suite sets up a complete production-like environment:
// - Kind cluster (2 nodes: 1 control-plane + 1 worker)
// - PostgreSQL with pgvector (for audit events storage)
// - Redis (for DLQ fallback)
// - Data Storage service (deployed to Kind cluster)
//
// E2E Test Coverage (10-15%):
// - Scenario 1: Happy Path - Complete remediation audit trail
// - Scenario 2: DLQ Fallback - Data Storage Service outage recovery
// - Scenario 3: Query API - Timeline retrieval with filtering

func TestDataStorageE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Data Storage E2E Suite")
}

var (
	ctx    context.Context
	cancel context.CancelFunc
	logger *zap.Logger

	// Cluster configuration (shared across all tests)
	clusterName    string
	kubeconfigPath string

	// Track if any test failed (for cluster cleanup decision)
	anyTestFailed bool
)

// Note: Helper functions (generateUniqueNamespace, createNamespace, deleteNamespace, etc.)
// are defined in helpers.go to avoid duplication

var _ = SynchronizedBeforeSuite(
	// This function runs ONCE on process 1 only
	func() []byte {
		// Initialize logger for process 1
		var err error
		logger, err = zap.NewDevelopment()
		Expect(err).ToNot(HaveOccurred())

		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("Data Storage E2E Test Suite - Cluster Setup (ONCE - Process 1)")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("Creating Kind cluster for all E2E tests...")
		logger.Info("  • Kind cluster (2 nodes: control-plane + worker)")
		logger.Info("  • PostgreSQL with pgvector (audit events storage)")
		logger.Info("  • Redis (DLQ fallback)")
		logger.Info("  • Data Storage Docker image (build + load)")
		logger.Info("  • Kubeconfig: ~/.kube/kind-config-datastorage")
		logger.Info("")
		logger.Info("Note: Each test will deploy services in a unique namespace")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Set cluster configuration
		clusterName = "datastorage-e2e"
		homeDir, err := os.UserHomeDir()
		Expect(err).ToNot(HaveOccurred())
		// Use unique kubeconfig path to avoid conflicts with Gateway E2E tests
		kubeconfigPath = fmt.Sprintf("%s/.kube/kind-config-datastorage", homeDir)

		// Create Kind cluster (ONCE for all tests)
		err = infrastructure.CreateDataStorageCluster(clusterName, kubeconfigPath, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())

		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("Cluster Setup Complete - Broadcasting to all processes")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info(fmt.Sprintf("  • Cluster: %s", clusterName))
		logger.Info(fmt.Sprintf("  • Kubeconfig: %s", kubeconfigPath))
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Return kubeconfig path to all processes
		return []byte(kubeconfigPath)
	},
	// This function runs on ALL processes (including process 1)
	func(data []byte) {
		// Initialize context
		ctx, cancel = context.WithCancel(context.Background())

		// Initialize logger for this process
		var err error
		logger, err = zap.NewDevelopment()
		Expect(err).ToNot(HaveOccurred())

		// Initialize failure tracking
		anyTestFailed = false

		// Receive kubeconfig path from process 1
		kubeconfigPath = string(data)
		clusterName = "datastorage-e2e"

		// Set KUBECONFIG environment variable for this process
		err = os.Setenv("KUBECONFIG", kubeconfigPath)
		Expect(err).ToNot(HaveOccurred())

		logger.Info(fmt.Sprintf("Process %d ready - Using kubeconfig: %s", GinkgoParallelProcess(), kubeconfigPath))
	},
)

var _ = SynchronizedAfterSuite(
	// This function runs on ALL processes (cleanup per-process resources)
	func() {
		// Cancel context for this process
		if cancel != nil {
			cancel()
		}
		logger.Info(fmt.Sprintf("Process %d cleanup complete", GinkgoParallelProcess()))
	},
	// This function runs ONCE on process 1 only (cleanup shared resources)
	func() {
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("Data Storage E2E Test Suite - Cleanup (Process 1)")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Check if we should keep the cluster for debugging
		keepCluster := os.Getenv("KEEP_CLUSTER")
		if keepCluster == "true" || anyTestFailed {
			logger.Info("⚠️  Keeping cluster for debugging (KEEP_CLUSTER=true or test failed)")
			logger.Info(fmt.Sprintf("  • Cluster: %s", clusterName))
			logger.Info(fmt.Sprintf("  • Kubeconfig: %s", kubeconfigPath))
			logger.Info("")
			logger.Info("To delete the cluster manually:")
			logger.Info(fmt.Sprintf("  kind delete cluster --name %s", clusterName))
			logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			return
		}

		// Delete Kind cluster
		logger.Info("🗑️  Deleting Kind cluster...")
		err := infrastructure.DeleteCluster(clusterName, GinkgoWriter)
		if err != nil {
			logger.Error("Failed to delete cluster", zap.Error(err))
		} else {
			logger.Info("✅ Cluster deleted successfully")
		}

		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	},
)

// AfterEach tracks test failures for cluster cleanup decision
var _ = AfterEach(func() {
	if CurrentSpecReport().Failed() {
		anyTestFailed = true
	}
})

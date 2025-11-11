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

package toolset

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

// Test suite for Dynamic Toolset E2E tests
// This suite sets up a complete production-like environment:
// - Kind cluster (single control-plane node) - SHARED across parallel tests
// - Dynamic Toolset controller (deployed per-namespace for isolation)
// - Mock services (nginx pods with toolset annotations)
//
// PARALLEL EXECUTION: Tests run in parallel, each with unique namespace
//
// Business Requirements:
// - BR-TOOLSET-016: Service discovery configuration
// - BR-TOOLSET-017: Priority-based service ordering
// - BR-TOOLSET-018: Service health monitoring
// - BR-TOOLSET-019: Multi-namespace service discovery
// - BR-TOOLSET-020: Real-time toolset configuration updates

func TestDynamicToolsetE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Dynamic Toolset E2E Suite")
}

var (
	ctx    context.Context
	cancel context.CancelFunc
	logger *zap.Logger

	// Cluster configuration (shared across all parallel test procs)
	clusterName    string
	kubeconfigPath string

	// Track if any test failed (for cluster cleanup decision)
	anyTestFailed bool
)

// SynchronizedBeforeSuite runs cluster setup ONCE on proc 1, then shares config with all procs
// This enables parallel test execution while sharing the same Kind cluster
var _ = SynchronizedBeforeSuite(func() []byte {
	// This runs ONCE on proc 1 only
	var err error
	logger, err = zap.NewDevelopment()
	Expect(err).ToNot(HaveOccurred())

	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	logger.Info("Dynamic Toolset E2E Test Suite - Cluster Setup (ONCE - Proc 1)")
	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	logger.Info("Creating Kind cluster for all E2E tests...")
	logger.Info("  • Kind cluster (single control-plane node)")
	logger.Info("  • Dynamic Toolset Docker image (build + load)")
	logger.Info("  • Kubeconfig: ~/.kube/kind-toolset-config")
	logger.Info("")
	logger.Info("Note: Tests will run in PARALLEL, each with unique namespace")
	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Set cluster configuration
	clusterName = "toolset-e2e"
	homeDir, err := os.UserHomeDir()
	Expect(err).ToNot(HaveOccurred())
	kubeconfigPath = fmt.Sprintf("%s/.kube/kind-toolset-config", homeDir)

	// Create Kind cluster (ONCE for all tests)
	err = infrastructure.CreateToolsetCluster(clusterName, kubeconfigPath, GinkgoWriter)
	Expect(err).ToNot(HaveOccurred())

	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	logger.Info("Cluster Setup Complete - Sharing config with parallel workers")
	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	logger.Info(fmt.Sprintf("  • Cluster: %s", clusterName))
	logger.Info(fmt.Sprintf("  • Kubeconfig: %s", kubeconfigPath))
	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Return kubeconfig path to share with all procs
	return []byte(kubeconfigPath)
}, func(data []byte) {
	// This runs on ALL procs (including proc 1)
	// Initialize context
	ctx, cancel = context.WithCancel(context.Background())

	// Initialize logger on each proc
	var err error
	logger, err = zap.NewDevelopment()
	Expect(err).ToNot(HaveOccurred())

	// Initialize failure tracking
	anyTestFailed = false

	// Get shared kubeconfig path from proc 1
	kubeconfigPath = string(data)
	clusterName = "toolset-e2e"

	// Set KUBECONFIG environment variable on each proc
	err = os.Setenv("KUBECONFIG", kubeconfigPath)
	Expect(err).ToNot(HaveOccurred())

	logger.Info(fmt.Sprintf("Worker initialized - using cluster: %s", clusterName))
})

// Track test failures for cluster cleanup decision
var _ = ReportAfterEach(func(report SpecReport) {
	if report.Failed() {
		anyTestFailed = true
	}
})

var _ = SynchronizedAfterSuite(func() {
	// This runs on each proc after all its tests complete
	// Nothing to do here - cleanup happens on proc 1
}, func() {
	// This runs ONCE on proc 1 after all procs finish
	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	logger.Info("Dynamic Toolset E2E Test Suite - Cluster Teardown")
	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Check if any test failed - preserve cluster for debugging
	if anyTestFailed || os.Getenv("SKIP_CLEANUP") == "true" {
		logger.Warn("⚠️  Test FAILED - Keeping cluster alive for debugging")
		logger.Info("To debug:")
		logger.Info(fmt.Sprintf("  export KUBECONFIG=%s", kubeconfigPath))
		logger.Info("  kubectl get namespaces | grep toolset-")
		logger.Info("  kubectl get pods -n <namespace>")
		logger.Info("  kubectl logs -n <namespace> deployment/kubernaut-dynamic-toolsets")
		logger.Info("  kubectl get configmap -n <namespace>")
		logger.Info("To cleanup manually:")
		logger.Info(fmt.Sprintf("  kind delete cluster --name %s", clusterName))
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		return
	}

	// All tests passed - cleanup cluster
	logger.Info("✅ All tests passed - cleaning up cluster...")
	err := infrastructure.DeleteToolsetCluster(clusterName, kubeconfigPath, GinkgoWriter)
	if err != nil {
		logger.Warn("Failed to delete cluster", zap.Error(err))
	}

	// Cancel context
	if cancel != nil {
		cancel()
	}

	// Sync logger
	if logger != nil {
		_ = logger.Sync()
	}

	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	logger.Info("Cluster Teardown Complete")
	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
})

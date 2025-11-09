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

package gateway

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"

	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// Suite-level resources for cleanup
var (
	suiteK8sClient *K8sTestClient  // Shared K8s client for cleanup
	suiteCtx       context.Context // Suite context
	suiteLogger    *zap.Logger     // Suite logger
	clusterName    string          // Cluster name
	kubeconfigPath string          // Kubeconfig path
)

var _ = BeforeSuite(func() {
	// Initialize suite context
	suiteCtx = context.Background()

	// Initialize logger
	var err error
	suiteLogger, err = zap.NewDevelopment()
	Expect(err).ToNot(HaveOccurred())

	suiteLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	suiteLogger.Info("Gateway Integration Test Suite - Infrastructure Setup")
	suiteLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	suiteLogger.Info("Creating Kind cluster + Redis for integration tests...")
	suiteLogger.Info("  • Kind cluster (2 nodes: control-plane + worker)")
	suiteLogger.Info("  • RemediationRequest CRD (cluster-wide)")
	suiteLogger.Info("  • Redis container (localhost:6379)")
	suiteLogger.Info("  • Kubeconfig: ~/.kube/kind-config")
	suiteLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Set cluster configuration
	clusterName = "gateway-integration"
	homeDir, err := os.UserHomeDir()
	Expect(err).ToNot(HaveOccurred())
	kubeconfigPath = fmt.Sprintf("%s/.kube/kind-config", homeDir)

	// Create Kind cluster (same as E2E tests)
	err = infrastructure.CreateGatewayCluster(clusterName, kubeconfigPath, GinkgoWriter)
	Expect(err).ToNot(HaveOccurred())

	// Set KUBECONFIG environment variable
	err = os.Setenv("KUBECONFIG", kubeconfigPath)
	Expect(err).ToNot(HaveOccurred())

	// Start Redis container for integration tests (with cleanup first)
	suiteLogger.Info("Cleaning up existing Redis container...")
	_ = infrastructure.StopRedisContainer("redis-integration", GinkgoWriter)

	suiteLogger.Info("Starting Redis container...")
	err = infrastructure.StartRedisContainer("redis-integration", 6379, GinkgoWriter)
	Expect(err).ToNot(HaveOccurred(), "Redis container must start for integration tests")

	suiteLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	suiteLogger.Info("Infrastructure Setup Complete")
	suiteLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Initialize shared K8s client for tests
	suiteK8sClient = SetupK8sTestClient(suiteCtx)
	Expect(suiteK8sClient).ToNot(BeNil(), "Failed to setup K8s client for suite")

	// Ensure kubernaut-system namespace exists for fallback tests
	EnsureTestNamespace(suiteCtx, suiteK8sClient, "kubernaut-system")
})

var _ = AfterSuite(func() {
	// TDD FIX: Batch delete all test namespaces after suite completes
	// This prevents "namespace is being terminated" errors during storm aggregation

	testNamespacesMutex.Lock()
	namespaceCount := len(testNamespaces)
	namespaceList := make([]string, 0, namespaceCount)
	for ns := range testNamespaces {
		namespaceList = append(namespaceList, ns)
	}
	testNamespacesMutex.Unlock()

	if namespaceCount == 0 {
		fmt.Println("\n✅ No test namespaces to clean up")
		return
	}

	fmt.Printf("\n🧹 Cleaning up %d test namespaces...\n", namespaceCount)

	// Wait for storm aggregation windows to complete
	// Test configuration: AggregationWindow = 1 second (from helpers.go StartTestGateway)
	// Buffer: 3 seconds for goroutines to complete and Redis operations to finish
	testAggregationWindow := 1 * time.Second
	bufferTime := 3 * time.Second
	totalWait := testAggregationWindow + bufferTime

	fmt.Printf("⏳ Waiting %v for storm aggregation windows to complete...\n", totalWait)
	time.Sleep(totalWait)

	// Delete all namespaces
	deletedCount := 0
	for _, nsName := range namespaceList {
		ns := &corev1.Namespace{}
		ns.Name = nsName
		err := suiteK8sClient.Client.Delete(suiteCtx, ns)
		if err != nil && !strings.Contains(err.Error(), "not found") {
			fmt.Printf("⚠️  Warning: Failed to delete namespace %s: %v\n", nsName, err)
		} else {
			deletedCount++
		}
	}

	fmt.Printf("✅ Deleted %d/%d test namespaces\n", deletedCount, len(namespaceList))

	// Cleanup K8s client
	if suiteK8sClient != nil {
		suiteK8sClient.Cleanup(suiteCtx)
	}

	// Cleanup infrastructure
	suiteLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	suiteLogger.Info("Gateway Integration Test Suite - Infrastructure Teardown")
	suiteLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Stop Redis container
	suiteLogger.Info("Stopping Redis container...")
	err := infrastructure.StopRedisContainer("redis-integration", GinkgoWriter)
	if err != nil {
		suiteLogger.Warn("Failed to stop Redis container", zap.Error(err))
	}

	// Delete Kind cluster
	suiteLogger.Info("Deleting Kind cluster...")
	err = infrastructure.DeleteGatewayCluster(clusterName, kubeconfigPath, GinkgoWriter)
	if err != nil {
		suiteLogger.Warn("Failed to delete cluster", zap.Error(err))
	}

	// Sync logger
	if suiteLogger != nil {
		_ = suiteLogger.Sync()
	}

	suiteLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	suiteLogger.Info("Infrastructure Teardown Complete")
	suiteLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
})

func TestGatewayIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gateway Integration Suite")
}

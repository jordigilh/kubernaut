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
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	kubelog "github.com/jordigilh/kubernaut/pkg/log"
	"github.com/jordigilh/kubernaut/test/infrastructure" // Shared DS infrastructure (PostgreSQL + Redis + DS)

	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL driver (pgx != pgvector extension)
)

// Suite-level resources (envtest migration)
var (
	suiteK8sClient *K8sTestClient       // Shared K8s client for cleanup
	suiteCtx       context.Context      // Suite context
	suiteLogger    logr.Logger          // Suite logger (DD-005: logr.Logger)
	testEnv        *envtest.Environment // envtest environment (in-memory K8s)
	k8sConfig      *rest.Config         // Kubernetes client config from envtest
)

// SynchronizedBeforeSuite runs ONCE globally before all parallel processes start
// envtest Migration: Replaces Kind cluster with in-memory K8s API server
var _ = SynchronizedBeforeSuite(func() []byte {
	// This runs ONCE on process 1 only - creates shared infrastructure
	// DD-005: Use shared logging library (logr.Logger interface)
	suiteLogger = kubelog.NewLogger(kubelog.Options{
		Development: true,
		Level:       0, // INFO
		ServiceName: "gateway-integration-test",
	})

	suiteLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	suiteLogger.Info("Gateway Integration Test Suite - envtest Setup (Parallel)")
	suiteLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	suiteLogger.Info("Creating test infrastructure...")
	suiteLogger.Info("  • envtest (in-memory K8s API server)")
	suiteLogger.Info("  • RemediationRequest CRD (cluster-wide)")
	suiteLogger.Info("  • Data Storage infrastructure (PostgreSQL + Redis + DS service)")
	suiteLogger.Info("  • Parallel Execution: 4 concurrent processors")
	suiteLogger.Info("  • Pattern: DD-TEST-002 (Sequential podman run)")
	suiteLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	var err error
	ctx := context.Background()

	// DD-TEST-002: Sequential startup pattern eliminates need for pre-cleanup
	// Cleanup is now handled internally by StartGatewayIntegrationInfrastructure

	// 1. Start Gateway integration infrastructure (podman-compose)
	//    This starts: PostgreSQL, Redis, Immudb, DataStorage (with migrations)
	//    Per DD-TEST-001 v2.2: PostgreSQL=15437, Redis=16380, Immudb=13323, DS=18091
	suiteLogger.Info("📦 Starting Gateway integration infrastructure (DD-TEST-002)...")
	dsInfra, err := infrastructure.StartDSBootstrap(infrastructure.DSBootstrapConfig{
		ServiceName:     "gateway",
		PostgresPort:    15437, // DD-TEST-001 v2.2
		RedisPort:       16380, // DD-TEST-001 v2.2
		DataStoragePort: 18091, // DD-TEST-001 v2.2
		MetricsPort:     19091,
		ConfigDir:       "test/integration/gateway/config",
	}, GinkgoWriter)
	Expect(err).ToNot(HaveOccurred(), "Infrastructure must start successfully")
	suiteLogger.Info("   ✅ All services started and healthy (PostgreSQL, Redis, Immudb, DataStorage)")

	// Clean up infrastructure on exit
	DeferCleanup(func() {
		infrastructure.StopDSBootstrap(dsInfra, GinkgoWriter)
	})

	// Store Data Storage URL for tests (IPv4 explicit for CI compatibility)
	dataStorageURL := fmt.Sprintf("http://127.0.0.1:%d", infrastructure.GatewayIntegrationDataStoragePort)

	// 2. Start envtest (in-memory K8s API server)
	suiteLogger.Info("📦 Starting envtest (in-memory K8s API)...")

	// Set KUBEBUILDER_ASSETS if not already set
	// This tells envtest where to find the K8s binaries (etcd, kube-apiserver)
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		// Use setup-envtest to get the path
		cmd := exec.Command("go", "run", "sigs.k8s.io/controller-runtime/tools/setup-envtest@latest", "use", "-p", "path")
		output, err := cmd.Output()
		if err != nil {
			suiteLogger.Error(err, "Failed to get KUBEBUILDER_ASSETS path")
			Expect(err).ToNot(HaveOccurred(), "Should get KUBEBUILDER_ASSETS path from setup-envtest")
		}
		assetsPath := strings.TrimSpace(string(output))
		_ = os.Setenv("KUBEBUILDER_ASSETS", assetsPath)
		suiteLogger.Info(fmt.Sprintf("   📍 Set KUBEBUILDER_ASSETS: %s", assetsPath))
	}

	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			"../../../config/crd/bases", // Relative path from test/integration/gateway/
		},
		ErrorIfCRDPathMissing: true,
	}

	k8sConfig, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred(), "envtest should start successfully")
	Expect(k8sConfig).ToNot(BeNil(), "K8s config should not be nil")

	// envtest uses self-signed certificates, so we need to skip TLS verification
	k8sConfig.TLSClientConfig.Insecure = true
	k8sConfig.TLSClientConfig.CAData = nil
	k8sConfig.TLSClientConfig.CAFile = ""

	// Disable client-side rate limiting for integration tests
	// envtest is an in-memory K8s API server - no reason to throttle
	// Per client-go source: setting RateLimiter to nil disables rate limiting entirely
	k8sConfig.RateLimiter = nil // Disable rate limiter completely
	k8sConfig.QPS = 1000        // High QPS (used if RateLimiter is not nil)
	k8sConfig.Burst = 2000      // High burst (used if RateLimiter is not nil)

	// Wait for API server to be fully ready by testing connectivity
	// envtest starts the API server asynchronously, so we need to wait for it to be responsive
	suiteLogger.Info("   ⏳ Waiting for API server to be ready...")

	// Create a temporary client to test API server readiness
	scheme := k8sruntime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	testClient, err := client.New(k8sConfig, client.Options{Scheme: scheme})
	Expect(err).ToNot(HaveOccurred(), "Should create test client")

	// Wait for API server to respond
	Eventually(func() error {
		nsList := &corev1.NamespaceList{}
		return testClient.List(ctx, nsList)
	}, 10*time.Second, 500*time.Millisecond).Should(Succeed(), "API server should be ready")

	suiteLogger.Info(fmt.Sprintf("   ✅ envtest started (K8s API: %s)", k8sConfig.Host))

	// AIAnalysis Pattern: Log complete infrastructure summary for debugging
	suiteLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	suiteLogger.Info("Gateway Integration Test Infrastructure - Ready")
	suiteLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	suiteLogger.Info(fmt.Sprintf("  K8s API:        %s", k8sConfig.Host))
	suiteLogger.Info(fmt.Sprintf("  DataStorage:    %s", dataStorageURL))
	suiteLogger.Info(fmt.Sprintf("  PostgreSQL:     localhost:%d", infrastructure.GatewayIntegrationPostgresPort))
	suiteLogger.Info(fmt.Sprintf("  Redis:          localhost:%d", infrastructure.GatewayIntegrationRedisPort))
	suiteLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Validate Data Storage health before proceeding (already validated by infrastructure startup)
	healthURL := dataStorageURL + "/health"
	healthResp, err := http.Get(healthURL)
	if err != nil || healthResp.StatusCode != http.StatusOK {
		Fail(fmt.Sprintf("Data Storage health check failed at %s (status: %d, err: %v)", healthURL, healthResp.StatusCode, err))
	}
	if healthResp != nil {
		_ = healthResp.Body.Close()
	}
	suiteLogger.Info("   ✅ Data Storage health re-validated")

	suiteLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	suiteLogger.Info("Infrastructure Setup Complete - Ready for Parallel Tests")
	suiteLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// DD-GATEWAY-012 + DD-TEST-001: Share Data Storage URL with all parallel processes
	// Per DD-TEST-001: Return structured config (not just kubeconfig) for parallel process sharing
	// Environment variables DON'T propagate between Ginkgo parallel processes
	type SharedConfig struct {
		Kubeconfig     []byte `json:"kubeconfig"`
		DataStorageURL string `json:"data_storage_url"`
	}
	config := SharedConfig{
		Kubeconfig:     testEnv.KubeConfig,
		DataStorageURL: dataStorageURL,
	}
	configBytes, err := json.Marshal(config)
	if err != nil {
		panic(fmt.Sprintf("Failed to marshal shared config: %v", err))
	}

	suiteLogger.Info("📦 Shared config prepared for parallel processes",
		"data_storage_url", dataStorageURL,
		"kubeconfig_size", len(testEnv.KubeConfig))

	return configBytes

}, func(data []byte) {
	// This runs on ALL processes (including process 1) - initializes per-process state
	suiteCtx = context.Background()

	// DD-005: Initialize logger for this process using shared logging library
	suiteLogger = kubelog.NewLogger(kubelog.Options{
		Development: true,
		Level:       0, // INFO
		ServiceName: "gateway-integration-test",
	})

	// DD-GATEWAY-012 + DD-TEST-001: Unmarshal shared config from process 1
	// Per DD-TEST-001: All parallel processes receive structured config
	type SharedConfig struct {
		Kubeconfig     []byte `json:"kubeconfig"`
		DataStorageURL string `json:"data_storage_url"`
	}
	var sharedConfig SharedConfig
	err := json.Unmarshal(data, &sharedConfig)
	Expect(err).ToNot(HaveOccurred(), "Failed to unmarshal shared config from process 1")

	// Set Data Storage URL for this process (critical for parallel execution)
	_ = os.Setenv("TEST_DATA_STORAGE_URL", sharedConfig.DataStorageURL)

	// Create Kubernetes client from shared kubeconfig
	k8sConfig, err = clientcmd.RESTConfigFromKubeConfig(sharedConfig.Kubeconfig)
	Expect(err).ToNot(HaveOccurred(), "Should create rest.Config from kubeconfig")

	// CRITICAL FIX: Reapply rate limiter settings for parallel processes
	// These settings are NOT serialized in kubeconfig bytes, so must be reapplied
	// envtest is in-memory, no need to throttle concurrent requests
	k8sConfig.RateLimiter = nil // Disable rate limiter completely
	k8sConfig.QPS = 1000        // High QPS (used if RateLimiter is not nil)
	k8sConfig.Burst = 2000      // High burst (used if RateLimiter is not nil)

	suiteLogger.Info(fmt.Sprintf("Process %d initialized with K8s API: %s, Data Storage: %s",
		GinkgoParallelProcess(), k8sConfig.Host, sharedConfig.DataStorageURL))

	// Set the K8s config for helpers.go to use (instead of loading from file)
	SetSuiteK8sConfig(k8sConfig)

	// Initialize K8s client for this process (uses reconstructed k8sConfig via SetSuiteK8sConfig)
	// Each process creates its own client.Client - clients are thread-safe and stateless
	suiteK8sClient = SetupK8sTestClient(suiteCtx)
	Expect(suiteK8sClient).ToNot(BeNil(), "Failed to setup K8s client for suite")

	// Ensure kubernaut-system namespace exists for fallback tests
	EnsureTestNamespace(suiteCtx, suiteK8sClient, "kubernaut-system")

	// Ensure static test namespaces exist for new integration tests
	// ✅ FIX: Removed shared namespace pre-creation
	// All tests now create unique namespaces per parallel process to prevent data pollution
	// This eliminates flakiness caused by tests interfering with each other's data
})

// SynchronizedAfterSuite runs cleanup in two phases for parallel execution
var _ = SynchronizedAfterSuite(func() {
	// This runs on ALL processes - cleanup per-process K8s client
	if suiteK8sClient != nil {
		suiteK8sClient.Cleanup(suiteCtx)
	}
}, func() {
	// This runs ONCE on process 1 only - tears down shared infrastructure
	suiteLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	suiteLogger.Info("Gateway Integration Test Suite - Infrastructure Teardown")
	suiteLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Note: SynchronizedAfterSuite already ensures all parallel processes finish
	// the first cleanup function before this second function runs.
	// No manual synchronization (time.Sleep) needed.

	// Collect namespace statistics
	testNamespacesMutex.Lock()
	namespaceCount := len(testNamespaces)
	testNamespacesMutex.Unlock()

	if namespaceCount > 0 {
		fmt.Printf("\n📝 %d test namespaces created (will be cleaned up with envtest)\n", namespaceCount)
	} else {
		fmt.Println("\n✅ No test namespaces created")
	}

	// Infrastructure cleanup handled by DeferCleanup (StopDSBootstrap)

	// Stop envtest
	if testEnv != nil {
		suiteLogger.Info("Stopping envtest...")
		err := testEnv.Stop()
		if err != nil {
			suiteLogger.Info("Failed to stop envtest", "error", err)
		}
	}

	// DD-TEST-001 v1.1: Clean up infrastructure images to prevent disk space issues
	suiteLogger.Info("🧹 Cleaning up infrastructure images (DD-TEST-001 v1.1)...")
	pruneCmd := exec.Command("podman", "image", "prune", "-f",
		"--filter", "label=io.podman.compose.project=gateway-integration-test")
	pruneOutput, pruneErr := pruneCmd.CombinedOutput()
	if pruneErr != nil {
		suiteLogger.Info("⚠️  Failed to prune images", "error", pruneErr, "output", string(pruneOutput))
	} else {
		suiteLogger.Info("   ✅ Infrastructure images pruned")
	}

	// DD-005: Sync logger using shared library
	kubelog.Sync(suiteLogger)

	suiteLogger.Info("   ✅ All services stopped and images cleaned")
	suiteLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	suiteLogger.Info("Infrastructure Teardown Complete")
	suiteLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
})

func TestGatewayIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gateway Integration Suite (envtest)")
}

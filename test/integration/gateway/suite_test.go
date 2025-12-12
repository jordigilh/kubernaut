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
)

// Suite-level resources (envtest migration)
var (
	suiteK8sClient        *K8sTestClient                      // Shared K8s client for cleanup
	suiteCtx              context.Context                     // Suite context
	suiteLogger           logr.Logger                         // Suite logger (DD-005: logr.Logger)
	testEnv               *envtest.Environment                // envtest environment (in-memory K8s)
	suiteDataStorageInfra *infrastructure.DataStorageInfrastructure // Shared DS infrastructure (PostgreSQL + Redis + DS)
	k8sConfig             *rest.Config                        // Kubernetes client config from envtest
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
	suiteLogger.Info("  • Using shared infrastructure pattern (test/infrastructure/datastorage.go)")
	suiteLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	var err error
	ctx := context.Background()

	// 1. Start envtest (in-memory K8s API server)
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
		os.Setenv("KUBEBUILDER_ASSETS", assetsPath)
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

	// 2. Start Data Storage infrastructure (PostgreSQL + Redis + DS)
	//    Using shared infrastructure pattern (test/infrastructure/datastorage.go)
	//    This handles PostgreSQL, Redis, migrations, and Data Storage service startup
	suiteLogger.Info("📦 Starting Data Storage infrastructure (shared pattern)...")

	// Use shared infrastructure with default config (migrations expect slm_user)
	// Note: Using defaults to match migration scripts expectations
	dsInfra, err := infrastructure.StartDataStorageInfrastructure(nil, GinkgoWriter)
	Expect(err).ToNot(HaveOccurred(), "Data Storage infrastructure must start successfully")
	Expect(dsInfra).ToNot(BeNil(), "Data Storage infrastructure must not be nil")

	// Store infrastructure handle for cleanup
	suiteDataStorageInfra = dsInfra
	dataStorageURL := dsInfra.ServiceURL

	suiteLogger.Info(fmt.Sprintf("   ✅ Data Storage infrastructure started (URL: %s)", dataStorageURL))

	// AIAnalysis Pattern: Log complete infrastructure summary for debugging
	suiteLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	suiteLogger.Info("Gateway Integration Test Infrastructure")
	suiteLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	suiteLogger.Info(fmt.Sprintf("  K8s API:        %s", k8sConfig.Host))
	suiteLogger.Info(fmt.Sprintf("  DataStorage:    %s (shared infrastructure)", dataStorageURL))
	suiteLogger.Info(fmt.Sprintf("  PostgreSQL:     %s", dsInfra.PostgresContainer))
	suiteLogger.Info(fmt.Sprintf("  Redis:          %s", dsInfra.RedisContainer))
	suiteLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Validate Data Storage health before proceeding
	healthURL := dataStorageURL + "/healthz"
	healthResp, err := http.Get(healthURL)
	if err != nil || healthResp.StatusCode != http.StatusOK {
		Fail(fmt.Sprintf("Data Storage health check failed at %s", healthURL))
	}
	healthResp.Body.Close()
	suiteLogger.Info("✅ Data Storage is healthy")

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
	os.Setenv("TEST_DATA_STORAGE_URL", sharedConfig.DataStorageURL)

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

	// Wait for all parallel processes to finish
	suiteLogger.Info("Waiting for all parallel processes to finish cleanup...")
	time.Sleep(1 * time.Second)

	// Collect namespace statistics
	testNamespacesMutex.Lock()
	namespaceCount := len(testNamespaces)
	testNamespacesMutex.Unlock()

	if namespaceCount > 0 {
		fmt.Printf("\n📝 %d test namespaces created (will be cleaned up with envtest)\n", namespaceCount)
	} else {
		fmt.Println("\n✅ No test namespaces created")
	}

	// Stop Data Storage infrastructure (PostgreSQL + Redis + DS)
	if suiteDataStorageInfra != nil {
		suiteLogger.Info("Stopping Data Storage infrastructure...")
		suiteDataStorageInfra.Stop(GinkgoWriter)
	}

	// Stop envtest
	if testEnv != nil {
		suiteLogger.Info("Stopping envtest...")
		err := testEnv.Stop()
		if err != nil {
			suiteLogger.Info("Failed to stop envtest", "error", err)
		}
	}

	// DD-005: Sync logger using shared library
	kubelog.Sync(suiteLogger)

	suiteLogger.Info("   ✅ All services stopped")
	suiteLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	suiteLogger.Info("Infrastructure Teardown Complete")
	suiteLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
})

func TestGatewayIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gateway Integration Suite (envtest)")
}

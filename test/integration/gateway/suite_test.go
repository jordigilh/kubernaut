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
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// Suite-level resources (envtest migration)
var (
	suiteK8sClient   *K8sTestClient         // Shared K8s client for cleanup
	suiteCtx         context.Context        // Suite context
	suiteLogger      *zap.Logger            // Suite logger
	testEnv          *envtest.Environment   // envtest environment (in-memory K8s)
	suitePgClient    *PostgresTestClient    // PostgreSQL container
	suiteDataStorage *DataStorageTestServer // Data Storage service
	// suiteRedisPort and k8sConfig are declared in helpers.go to be accessible by both test and non-test files
)

// SynchronizedBeforeSuite runs ONCE globally before all parallel processes start
// envtest Migration: Replaces Kind cluster with in-memory K8s API server
var _ = SynchronizedBeforeSuite(func() []byte {
	// This runs ONCE on process 1 only - creates shared infrastructure
	var err error
	suiteLogger, err = zap.NewDevelopment()
	Expect(err).ToNot(HaveOccurred())

	suiteLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	suiteLogger.Info("Gateway Integration Test Suite - envtest Setup (Parallel)")
	suiteLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	suiteLogger.Info("Creating test infrastructure...")
	suiteLogger.Info("  • envtest (in-memory K8s API server)")
	suiteLogger.Info("  • RemediationRequest CRD (cluster-wide)")
	suiteLogger.Info("  • Redis container (Podman)")
	suiteLogger.Info("  • PostgreSQL container (Podman)")
	suiteLogger.Info("  • Data Storage service (httptest.Server)")
	suiteLogger.Info("  • Parallel Execution: 4 concurrent processors")
	suiteLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

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
			suiteLogger.Error("Failed to get KUBEBUILDER_ASSETS path", zap.Error(err))
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

	// 2. Start Redis container
	suiteLogger.Info("📦 Starting Redis container...")
	_ = infrastructure.StopRedisContainer("redis-integration", GinkgoWriter)
	time.Sleep(500 * time.Millisecond) // Wait for port to be released

	redisPort, err := infrastructure.StartRedisContainer("redis-integration", 16380, GinkgoWriter) // DD-TEST-001: Gateway integration Redis port
	Expect(err).ToNot(HaveOccurred(), "Redis container must start for integration tests")
	suiteRedisPort = redisPort
	suiteLogger.Info(fmt.Sprintf("   ✅ Redis started (port: %d)", redisPort))

	// 3. Start PostgreSQL container
	suiteLogger.Info("📦 Starting PostgreSQL container...")
	suitePgClient = SetupPostgresTestClient(ctx)
	Expect(suitePgClient).ToNot(BeNil(), "PostgreSQL container must start")
	suiteLogger.Info(fmt.Sprintf("   ✅ PostgreSQL started (port: %d)", suitePgClient.Port))

	// 4. Start Data Storage service
	suiteLogger.Info("📦 Starting Data Storage service...")
	suiteDataStorage = SetupDataStorageTestServer(ctx, suitePgClient)
	Expect(suiteDataStorage).ToNot(BeNil(), "Data Storage service must start")
	suiteLogger.Info(fmt.Sprintf("   ✅ Data Storage started (URL: %s)", suiteDataStorage.Server.URL))

	suiteLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	suiteLogger.Info("Infrastructure Setup Complete - Ready for Parallel Tests")
	suiteLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Return both kubeconfig and Redis port for other processes
	// Format: kubeconfig_length(4 bytes) + kubeconfig + redis_port(string)
	type SharedConfig struct {
		KubeConfig []byte
		RedisPort  int
	}
	configData, err := json.Marshal(SharedConfig{
		KubeConfig: testEnv.KubeConfig,
		RedisPort:  redisPort,
	})
	Expect(err).ToNot(HaveOccurred(), "Should serialize shared config")
	return configData

}, func(data []byte) {
	// This runs on ALL processes (including process 1) - initializes per-process state
	suiteCtx = context.Background()

	// Initialize logger for this process
	var err error
	suiteLogger, err = zap.NewDevelopment()
	Expect(err).ToNot(HaveOccurred())

	// Deserialize shared config (kubeconfig + Redis port)
	type SharedConfig struct {
		KubeConfig []byte
		RedisPort  int
	}
	var sharedConfig SharedConfig
	err = json.Unmarshal(data, &sharedConfig)
	Expect(err).ToNot(HaveOccurred(), "Should deserialize shared config")

	// Create rest.Config from kubeconfig bytes
	k8sConfig, err = clientcmd.RESTConfigFromKubeConfig(sharedConfig.KubeConfig)
	Expect(err).ToNot(HaveOccurred(), "Should create rest.Config from kubeconfig")

	// Set Redis port for this process
	suiteRedisPort = sharedConfig.RedisPort

	suiteLogger.Info(fmt.Sprintf("Process %d initialized with K8s API: %s, Redis port: %d",
		GinkgoParallelProcess(), k8sConfig.Host, suiteRedisPort))

	// Initialize K8s client for this process (uses reconstructed k8sConfig)
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

	ctx := context.Background()

	// Stop Data Storage service
	if suiteDataStorage != nil {
		suiteLogger.Info("Stopping Data Storage service...")
		suiteDataStorage.Cleanup()
	}

	// Stop PostgreSQL container
	if suitePgClient != nil {
		suiteLogger.Info("Stopping PostgreSQL container...")
		suitePgClient.Cleanup(ctx)
	}

	// Stop Redis container
	suiteLogger.Info("Stopping Redis container...")
	err := infrastructure.StopRedisContainer("redis-integration", GinkgoWriter)
	if err != nil {
		suiteLogger.Warn("Failed to stop Redis container", zap.Error(err))
	}

	// Stop envtest
	if testEnv != nil {
		suiteLogger.Info("Stopping envtest...")
		err := testEnv.Stop()
		if err != nil {
			suiteLogger.Warn("Failed to stop envtest", zap.Error(err))
		}
	}

	// Sync logger
	if suiteLogger != nil {
		_ = suiteLogger.Sync()
	}

	suiteLogger.Info("   ✅ All services stopped")
	suiteLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	suiteLogger.Info("Infrastructure Teardown Complete")
	suiteLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
})

func TestGatewayIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gateway Integration Suite (envtest)")
}

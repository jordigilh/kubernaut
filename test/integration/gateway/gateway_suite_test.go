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

	goredis "github.com/go-redis/redis/v8"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/gateway/redis"
	"github.com/jordigilh/kubernaut/pkg/gateway"
	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	"github.com/jordigilh/kubernaut/pkg/testutil/kind"
)

// Test suite variables
var (
	suite         *kind.IntegrationSuite
	redisClient   *goredis.Client
	gatewayServer *gateway.Server
	k8sClient     client.Client // Controller-runtime client for CRD access

	// Test token (from environment or file)
	testToken string
)

func TestGatewayIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gateway Integration Suite (Kind)")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	By("Connecting to existing Kind cluster using template")
	// Use Kind template for standardized test setup
	// See: docs/testing/KIND_CLUSTER_TEST_TEMPLATE.md
	suite = kind.Setup("gateway-test", "kubernaut-system")

	By("Registering CRD schemes for controller-runtime client")
	// Register RemediationRequest CRD scheme
	err := remediationv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	By("Creating controller-runtime client for CRD access")
	// Get Kind cluster REST config using kubeconfig
	cfg, err := config.GetConfig()
	Expect(err).NotTo(HaveOccurred(), "Failed to get Kind cluster REST config")

	// Create controller-runtime client for CRD operations
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred(), "Failed to create controller-runtime client")
	Expect(k8sClient).NotTo(BeNil(), "Controller-runtime client should not be nil")

	GinkgoWriter.Println("✅ Controller-runtime client initialized for CRD access")

	By("Getting test token for authentication")
	testToken = getTestToken()

	By("Connecting to Redis in Kind cluster")
	redisClient = setupRedisClient(suite)

	By("Starting Gateway server")
	gatewayServer = setupGatewayServer(suite)

	GinkgoWriter.Println("✅ Gateway integration test environment ready!")
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")

	// Stop Gateway server
	if gatewayServer != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		err := gatewayServer.Stop(shutdownCtx)
		Expect(err).NotTo(HaveOccurred())
	}

	// Close Redis
	if redisClient != nil {
		err := redisClient.Close()
		Expect(err).NotTo(HaveOccurred())
	}

	// Cleanup Kind resources (namespaces, registered resources)
	suite.Cleanup()

	GinkgoWriter.Println("✅ Gateway integration test environment cleaned up!")
})

// getTestToken retrieves the test token from environment or file.
func getTestToken() string {
	token := os.Getenv("TEST_TOKEN")
	if token == "" {
		// Try reading from file
		tokenBytes, err := os.ReadFile("/tmp/test-gateway-token.txt")
		if err != nil {
			Fail("TEST_TOKEN not set and /tmp/test-gateway-token.txt not found. Run: make test-gateway-setup")
		}
		token = strings.TrimSpace(string(tokenBytes))
	}
	Expect(token).NotTo(BeEmpty(), "Test token is required")
	GinkgoWriter.Printf("Using test token (length: %d)\n", len(token))
	return token
}

// setupRedisClient connects to Redis in the Kind cluster.
func setupRedisClient(suite *kind.IntegrationSuite) *goredis.Client {
	// Connect to Redis via Kind port mapping (NodePort 30379 → localhost:6379)
	// Tests run on host machine, not inside cluster, so use localhost
	client, err := redis.NewClient(&redis.Config{
		Addr:     "localhost:6379",
		Password: "",
		DB:       15, // Use DB 15 for testing
		PoolSize: 10,
	})
	Expect(err).NotTo(HaveOccurred())

	// Verify Redis connectivity
	err = client.Ping(suite.Context).Err()
	Expect(err).NotTo(HaveOccurred(), "Redis must be accessible in Kind cluster")

	// Clear test database
	err = client.FlushDB(suite.Context).Err()
	Expect(err).NotTo(HaveOccurred())

	GinkgoWriter.Println("✅ Connected to Redis in Kind cluster")
	return client
}

// setupGatewayServer creates and starts the Gateway server.
// Note: Gateway creates its own Redis client from config. The test suite's
// redisClient is used separately for test setup/cleanup operations (FlushDB).
// This provides proper isolation between test infrastructure and application logic.
func setupGatewayServer(suite *kind.IntegrationSuite) *gateway.Server {
	prometheusAdapter := adapters.NewPrometheusAdapter()

	// Create logrus logger for Gateway
	logger := logrus.New()
	logger.SetOutput(GinkgoWriter)
	logger.SetLevel(logrus.DebugLevel)

	serverConfig := &gateway.ServerConfig{
		ListenAddr:   ":8090", // Use non-standard port for testing
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,

		// Higher rate limits for integration testing
		// - 2000 req/min (20x production) accommodates storm tests sending 52 alerts in ~2.6s (~1200 alerts/min)
		// - Burst of 100 (5x production) provides sufficient capacity for rapid storm alert bursts
		// - Still validates rate limiting behavior (rate limiting tests use unique source IPs)
		// - Rationale: Storm tests simulate realistic AlertManager storm behavior (many alerts from same source)
		//   Per-source isolation via X-Forwarded-For ensures non-storm tests remain isolated
		RateLimitRequestsPerMinute: 2000,
		RateLimitBurst:             100,

		Redis: &redis.Config{
			Addr:     "localhost:6379", // Kind port mapping: NodePort 30379 → localhost:6379
			DB:       15,
			PoolSize: 10,
		},

		// Use 5-second TTL for fast integration testing (production: 5 minutes)
		DeduplicationTTL: 5 * time.Second,

		// Storm detection thresholds for testing (HIGH to prevent interference)
		// - Production default: 10 alerts/minute (rate), 5 similar alerts (pattern)
		// - Test default: 50 (HIGH threshold prevents accidental storm triggers in non-storm tests)
		// - Storm-specific tests can send >50 alerts to explicitly validate storm behavior
		// - Rationale: Storm detection uses alertname-only fingerprinting, causing
		//   non-storm tests (e.g., rate limiting burst test with 50 alerts) to accidentally
		//   trigger storm aggregation. High thresholds ensure test isolation.
		StormRateThreshold:    50, // >50 alerts/minute triggers storm (prevents test interference)
		StormPatternThreshold: 50, // >50 similar alerts triggers pattern storm (prevents test interference)

		// Storm aggregation window for testing (much shorter than production)
		// - Production default: 1 minute (60 seconds)
		// - Test: 5 seconds (speeds up integration tests by 12x)
		// - This reduces test execution time from 5+ minutes to ~30 seconds
		StormAggregationWindow: 5 * time.Second,

		// Environment classification cache TTL for testing (shorter than production)
		// - Production default: 30 seconds
		// - Test: 5 seconds (allows tests to verify cache expiry behavior)
		// - Tests can wait 6 seconds to ensure cache entries expire
		EnvironmentCacheTTL: 5 * time.Second,

		EnvConfigMapNamespace: "kubernaut-system",
		EnvConfigMapName:      "kubernaut-environment-overrides",
	}

	server, err := gateway.NewServer(serverConfig, logger)
	Expect(err).NotTo(HaveOccurred())

	// Register Prometheus adapter
	err = server.RegisterAdapter(prometheusAdapter)
	Expect(err).NotTo(HaveOccurred())

	// Start Gateway server in background
	go func() {
		defer GinkgoRecover()
		err := server.Start(suite.Context)
		if err != nil && err.Error() != "http: Server closed" {
			Fail(fmt.Sprintf("Gateway server failed: %v", err))
		}
	}()

	// Wait for server to be ready
	time.Sleep(500 * time.Millisecond)

	GinkgoWriter.Println("✅ Gateway server started")
	return server
}

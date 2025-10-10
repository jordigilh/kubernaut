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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/gateway/redis"
	"github.com/jordigilh/kubernaut/pkg/gateway"
	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
)

// Test suite variables
var (
	k8sClient     client.Client
	redisClient   *goredis.Client
	gatewayServer *gateway.Server
	ctx           context.Context
	cancel        context.CancelFunc

	// Test token (from environment or file)
	testToken string
)

func TestGatewayIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gateway Integration Suite (Kind)")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.Background())

	By("Connecting to existing Kind cluster (setup via make test-gateway-setup)")

	// 1. Get Kubernetes client (cluster already exists via make)
	config := ctrl.GetConfigOrDie()

	// Register RemediationRequest CRD scheme
	err := remediationv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// Create controller-runtime client
	k8sClient, err = client.New(config, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	// 2. Get test token (from env or file)
	testToken = os.Getenv("TEST_TOKEN")
	if testToken == "" {
		// Try reading from file
		tokenBytes, err := os.ReadFile("/tmp/test-gateway-token.txt")
		if err != nil {
			Fail("TEST_TOKEN not set and /tmp/test-gateway-token.txt not found. Run: make test-gateway-setup")
		}
		testToken = strings.TrimSpace(string(tokenBytes))
	}
	Expect(testToken).NotTo(BeEmpty(), "Test token is required")
	GinkgoWriter.Printf("Using test token (length: %d)\n", len(testToken))

	// 3. Connect to Redis (already deployed via make test-gateway-setup)
	// Use 127.0.0.1 explicitly to avoid IPv6 localhost resolution issues on macOS
	redisClient, err = redis.NewClient(&redis.Config{
		Addr:     "127.0.0.1:6379", // Port-forward setup by test-gateway-setup.sh
		Password: "",
		DB:       15, // Use DB 15 for testing
		PoolSize: 10,
	})
	Expect(err).NotTo(HaveOccurred())

	// Verify Redis connectivity
	err = redisClient.Ping(ctx).Err()
	Expect(err).NotTo(HaveOccurred(), "Redis must be accessible. Run: make test-gateway-setup (includes port-forward)")

	// Clear test database
	err = redisClient.FlushDB(ctx).Err()
	Expect(err).NotTo(HaveOccurred())

	// 4. Setup Gateway server
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

		// Realistic production rate limits for testing
		// - 100 req/min is standard production rate (prevents DoS)
		// - Burst of 20 allows short traffic spikes (e.g., storm detection sends 12 requests)
		// - Rate limiting test sends 150 requests and expects ~127 to be blocked
		RateLimitRequestsPerMinute: 100,
		RateLimitBurst:             20,

		Redis: &redis.Config{
			Addr:     "127.0.0.1:6379", // Port-forward to Kind cluster
			DB:       15,
			PoolSize: 10,
		},

		// Use 5-second TTL for fast integration testing (production: 5 minutes)
		DeduplicationTTL: 5 * time.Second,

		EnvConfigMapNamespace: "kubernaut-system",
		EnvConfigMapName:      "kubernaut-environment-overrides",
	}

	gatewayServer, err = gateway.NewServer(serverConfig, logger)
	Expect(err).NotTo(HaveOccurred())

	// Register Prometheus adapter
	err = gatewayServer.RegisterAdapter(prometheusAdapter)
	Expect(err).NotTo(HaveOccurred())

	// Start Gateway server in background
	go func() {
		defer GinkgoRecover()
		err := gatewayServer.Start(ctx)
		if err != nil && err.Error() != "http: Server closed" {
			Fail(fmt.Sprintf("Gateway server failed: %v", err))
		}
	}()

	// Wait for server to be ready
	time.Sleep(500 * time.Millisecond)

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

	// Note: Kind cluster is NOT deleted (persistent for debugging)
	// Run: make test-gateway-teardown to clean up

	cancel()
})

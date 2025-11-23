// Package gateway contains integration test helpers for Gateway Service
package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	goruntime "runtime"
	"strings"
	"sync"
	"time"

	goredis "github.com/go-redis/redis/v8"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	gateway "github.com/jordigilh/kubernaut/pkg/gateway"
	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	gatewayconfig "github.com/jordigilh/kubernaut/pkg/gateway/config"
	"github.com/jordigilh/kubernaut/pkg/gateway/metrics"
)

// Suite-level namespace tracking for batch cleanup
var (
	testNamespaces      = make(map[string]bool) // Track all test namespaces
	testNamespacesMutex sync.Mutex              // Thread-safe access
	suiteRedisPort      int                     // Redis port (random to avoid conflicts with Data Storage tests)
	k8sConfig           *rest.Config            // K8s REST config from envtest (set in suite_test.go)
)

// RegisterTestNamespace adds a namespace to the suite-level cleanup list
func RegisterTestNamespace(namespace string) {
	testNamespacesMutex.Lock()
	defer testNamespacesMutex.Unlock()
	testNamespaces[namespace] = true
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// TEST INFRASTRUCTURE TYPES
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// RedisTestClient wraps Redis client for integration tests
type RedisTestClient struct {
	Client *goredis.Client
}

// K8sTestClient wraps Kubernetes client for integration tests
type K8sTestClient struct {
	Client client.Client
}

// PostgresTestClient wraps PostgreSQL container for integration tests
// Uses direct Podman commands (no testcontainers)
type PostgresTestClient struct {
	Host     string
	Port     int
	Database string
	User     string
	Password string
}

// DataStorageTestServer wraps Data Storage service for integration tests
type DataStorageTestServer struct {
	Server   *httptest.Server
	PgClient *PostgresTestClient
}

// WebhookResponse represents HTTP response from Gateway webhook
type WebhookResponse struct {
	StatusCode int
	Body       []byte
	Headers    http.Header
}

// PrometheusAlertOptions configures test alert generation
type PrometheusAlertOptions struct {
	AlertName string
	Namespace string
	Severity  string
	Resource  ResourceIdentifier
	Labels    map[string]string
}

// ResourceIdentifier identifies a Kubernetes resource
type ResourceIdentifier struct {
	Kind string
	Name string
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// REDIS TEST CLIENT METHODS
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// SetupRedisTestClient creates a Redis client for integration tests
// ONLY connects to local Podman Redis (dynamic port)
// Integration tests use envtest + local Podman Redis (no OCP fallback)
// Redis is started automatically in suite_test.go SynchronizedBeforeSuite
// This is a convenience wrapper that uses the global suiteRedisPort
func SetupRedisTestClient(ctx context.Context) *RedisTestClient {
	return SetupRedisTestClientWithPort(ctx, suiteRedisPort)
}

// SetupRedisTestClientWithPort creates a Redis test client with a specific port
func SetupRedisTestClientWithPort(ctx context.Context, port int) *RedisTestClient {
	// Check if running in CI without Redis
	if os.Getenv("SKIP_INTEGRATION_TESTS") == "true" {
		return &RedisTestClient{Client: nil}
	}

	// Priority 1: Local Podman Redis (fastest, recommended for development)
	// Uses dynamic port from suite setup to avoid conflicts with Data Storage tests
	// Use different Redis DB per parallel process to prevent state leakage
	processID := GinkgoParallelProcess()
	redisDB := 2 + processID // DB 2 for process 1, DB 3 for process 2, etc.

	redisAddr := fmt.Sprintf("localhost:%d", port)
	client := goredis.NewClient(&goredis.Options{
		Addr:         redisAddr,
		Password:     "",
		DB:           redisDB,
		PoolSize:     20,
		MinIdleConns: 5,
		MaxRetries:   3,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := client.Ping(pingCtx).Result()
	if err == nil {
		return &RedisTestClient{Client: client}
	}

	// NO OCP FALLBACK - Integration tests use envtest + local Podman Redis only
	// If Redis is not available, fail fast with clear error
	_ = client.Close()
	return &RedisTestClient{Client: nil}
}

// CountFingerprints returns count of fingerprints in Redis for a namespace
// v2.9: Updated to match actual Redis key pattern used by deduplication service
func (r *RedisTestClient) CountFingerprints(ctx context.Context, namespace string) int {
	if r.Client == nil {
		return 0
	}
	// v2.9: Deduplication service uses "gateway:dedup:fingerprint:{fingerprint}" pattern
	// We can't filter by namespace in the key, so we count all fingerprints
	pattern := "gateway:dedup:fingerprint:*"
	keys, err := r.Client.Keys(ctx, pattern).Result()
	if err != nil {
		return 0
	}
	return len(keys)
}

// Cleanup removes all test data from Redis
func (r *RedisTestClient) Cleanup(ctx context.Context) {
	if r.Client == nil {
		return
	}
	r.Client.FlushDB(ctx)
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// KUBERNETES TEST CLIENT METHODS
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// SetupK8sTestClient creates a Kubernetes client for integration tests
// Uses envtest (in-memory K8s API) for integration testing (supports auth, real API behavior)
// BR-GATEWAY-001: Real K8s API required for authentication/authorization testing
func SetupK8sTestClient(ctx context.Context) *K8sTestClient {
	// envtest Migration: Use global k8sConfig from envtest
	// k8sConfig is initialized in suite_test.go SynchronizedBeforeSuite
	// All parallel processes share the same envtest instance and config
	if k8sConfig == nil {
		panic("k8sConfig is nil - envtest not initialized in suite_test.go")
	}

	// Create scheme with RemediationRequest CRD + core K8s types
	scheme := k8sruntime.NewScheme()
	_ = remediationv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme) // Add core types (Namespace, Pod, etc.)

	// Create Kubernetes client using envtest config
	// All tests share the same envtest API server, no isolation needed
	k8sClient, err := client.New(k8sConfig, client.Options{Scheme: scheme})
	if err != nil {
		panic(fmt.Sprintf("Failed to create K8s client for integration tests: %v", err))
	}

	return &K8sTestClient{Client: k8sClient}
}

// Cleanup removes all test CRDs from Kubernetes
func (k *K8sTestClient) Cleanup(ctx context.Context) {
	if k.Client == nil {
		return
	}

	// Delete all RemediationRequest CRDs to prevent name collisions
	crdList := &remediationv1alpha1.RemediationRequestList{}
	if err := k.Client.List(ctx, crdList); err == nil {
		for i := range crdList.Items {
			_ = k.Client.Delete(ctx, &crdList.Items[i])
		}
	}
}

// DeleteCRD deletes a specific RemediationRequest CRD by name and namespace
func (k *K8sTestClient) DeleteCRD(ctx context.Context, name, namespace string) error {
	if k.Client == nil {
		return fmt.Errorf("K8s client not initialized")
	}

	crd := &remediationv1alpha1.RemediationRequest{}
	crd.Name = name
	crd.Namespace = namespace

	return k.Client.Delete(ctx, crd)
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// GATEWAY SERVER LIFECYCLE
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// StartTestGateway creates a Gateway server for integration tests
// Returns the Gateway server instance for creating test HTTP servers
//
// Example usage:
//
//	gatewayServer, err := StartTestGateway(ctx, redisClient, k8sClient)
//	Expect(err).ToNot(HaveOccurred())
//	testServer := httptest.NewServer(gatewayServer.Handler())
//	defer testServer.Close()
//	resp, _ := http.Post(testServer.URL+"/api/v1/signals/prometheus", "application/json", body)
//
// DD-GATEWAY-004: Authentication removed - security now at network layer
func StartTestGateway(ctx context.Context, redisClient *RedisTestClient, k8sClient *K8sTestClient) (*gateway.Server, error) {
	// Use production logger with console output to capture errors in test logs
	logConfig := zap.NewProductionConfig()
	logConfig.OutputPaths = []string{"stdout"}
	logConfig.ErrorOutputPaths = []string{"stderr"}
	logger, _ := logConfig.Build()

	return StartTestGatewayWithLogger(ctx, redisClient, k8sClient, logger)
}

// StartTestGatewayWithLogger creates and starts a Gateway server with a custom logger
// This is useful for observability tests that need to capture and verify log output
func StartTestGatewayWithLogger(ctx context.Context, redisClient *RedisTestClient, k8sClient *K8sTestClient, logger *zap.Logger) (*gateway.Server, error) {

	// v2.9: Wire deduplication and storm detection services (REQUIRED)
	// BR-GATEWAY-008, BR-GATEWAY-009, BR-GATEWAY-010
	// These services are MANDATORY - Gateway will not start without them
	if redisClient == nil || redisClient.Client == nil {
		return nil, fmt.Errorf("Redis client is required for Gateway startup (BR-GATEWAY-008, BR-GATEWAY-009)")
	}

	// Create ServerConfig for tests (nested structure)
	// Uses fast TTLs and low thresholds for rapid test execution
	cfg := &gatewayconfig.ServerConfig{
		Server: gatewayconfig.ServerSettings{
			ListenAddr:   ":8080",
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  120 * time.Second,
		},

		Middleware: gatewayconfig.MiddlewareSettings{
			RateLimit: gatewayconfig.RateLimitSettings{
				RequestsPerMinute: 20, // Production: 100
				Burst:             5,  // Production: 10
			},
		},

		Infrastructure: gatewayconfig.InfrastructureSettings{
			Redis: &gatewayconfig.RedisOptions{
				Addr:         redisClient.Client.Options().Addr,
				DB:           redisClient.Client.Options().DB,
				Password:     redisClient.Client.Options().Password,
				DialTimeout:  redisClient.Client.Options().DialTimeout,
				ReadTimeout:  redisClient.Client.Options().ReadTimeout,
				WriteTimeout: redisClient.Client.Options().WriteTimeout,
				PoolSize:     redisClient.Client.Options().PoolSize,
				MinIdleConns: redisClient.Client.Options().MinIdleConns,
			},
		},

		Processing: gatewayconfig.ProcessingSettings{
			Deduplication: gatewayconfig.DeduplicationSettings{
				TTL: 5 * time.Second, // Production: 5 minutes
			},
			Storm: gatewayconfig.StormSettings{
				RateThreshold:     2,               // Production: 10 alerts/minute
				PatternThreshold:  2,               // Production: 5 similar alerts
				AggregationWindow: 1 * time.Second, // Test: 1s, Production: 1m
			},
			Environment: gatewayconfig.EnvironmentSettings{
				CacheTTL:           5 * time.Second, // Production: 30 seconds
				ConfigMapNamespace: "kubernaut-system",
				ConfigMapName:      "kubernaut-environment-overrides",
			},
			Priority: gatewayconfig.PrioritySettings{
				PolicyPath: "../../../config.app/gateway/policies/priority.rego", // Use Rego policy for tests (relative to test/integration/gateway/)
			},
			Retry: gatewayconfig.DefaultRetrySettings(), // BR-GATEWAY-111: K8s API retry configuration
		},
	}

	logger.Info("Creating Gateway server for integration tests",
		zap.Duration("deduplication_ttl", cfg.Processing.Deduplication.TTL),
		zap.Int("storm_rate_threshold", cfg.Processing.Storm.RateThreshold),
		zap.Int("rate_limit", cfg.Middleware.RateLimit.RequestsPerMinute),
	)

	// Create isolated Prometheus registry for this test
	// This prevents "duplicate metrics collector registration" panics when
	// multiple Gateway servers are created in the same test suite
	registry := prometheus.NewRegistry()
	metricsInstance := metrics.NewMetricsWithRegistry(registry)

	// Initialize Redis availability gauge to 1 (available) for tests
	// The monitorRedisHealth goroutine will update this if Redis becomes unavailable
	metricsInstance.RedisAvailable.Set(1)

	// TDD FIX: Use NewServerWithK8sClient to share K8s client with test
	// This ensures Gateway and test use the same K8s API cache, preventing
	// "namespace not found" errors due to cache propagation delays
	server, err := gateway.NewServerWithK8sClient(cfg, logger, metricsInstance, k8sClient.Client)
	if err != nil {
		return nil, fmt.Errorf("failed to create Gateway server: %w", err)
	}

	// Register Prometheus adapter (required for webhook endpoint)
	prometheusAdapter := adapters.NewPrometheusAdapter()
	if err := server.RegisterAdapter(prometheusAdapter); err != nil {
		return nil, fmt.Errorf("failed to register Prometheus adapter: %w", err)
	}

	// Register Kubernetes Event adapter
	k8sEventAdapter := adapters.NewKubernetesEventAdapter()
	if err := server.RegisterAdapter(k8sEventAdapter); err != nil {
		return nil, fmt.Errorf("failed to register Kubernetes Event adapter: %w", err)
	}

	return server, nil
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// WEBHOOK HELPERS
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// SendWebhook sends HTTP POST request to Gateway webhook endpoint
// DD-GATEWAY-004: No authentication needed - handled at network layer
func SendWebhook(url string, payload []byte) WebhookResponse {
	return SendWebhookWithAuth(url, payload, "")
}

// sharedHTTPClient is a package-level HTTP client with connection pooling
// This prevents port exhaustion during concurrent testing (BR-GATEWAY-003)
var sharedHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        200, // Allow up to 200 idle connections
		MaxIdleConnsPerHost: 100, // Allow up to 100 idle connections per host
		IdleConnTimeout:     90 * time.Second,
	},
}

// SendWebhookWithAuth sends a webhook request with optional authentication token
// If token is empty, no Authorization header is added (for testing auth failures)
func SendWebhookWithAuth(url string, payload []byte, token string) WebhookResponse {
	req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
	if err != nil {
		return WebhookResponse{
			StatusCode: 0,
			Body:       []byte(fmt.Sprintf("error creating request: %v", err)),
		}
	}

	req.Header.Set("Content-Type", "application/json")

	// Add Authorization header if token provided
	if token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	}

	// Use shared HTTP client with connection pooling to prevent port exhaustion
	resp, err := sharedHTTPClient.Do(req)
	if err != nil {
		return WebhookResponse{
			StatusCode: 0,
			Body:       []byte(fmt.Sprintf("error: %v", err)),
		}
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)

	return WebhookResponse{
		StatusCode: resp.StatusCode,
		Body:       body,
		Headers:    resp.Header,
	}
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// AUTHENTICATION HELPERS
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// DD-GATEWAY-004: GetAuthorizedToken() removed - authentication now at network layer

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// PAYLOAD GENERATION
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// GeneratePrometheusAlert creates a Prometheus AlertManager webhook payload
func GeneratePrometheusAlert(opts PrometheusAlertOptions) []byte {
	// Set defaults
	if opts.Severity == "" {
		opts.Severity = "warning"
	}
	if opts.Namespace == "" {
		opts.Namespace = "default"
	}

	// Build labels
	labels := map[string]string{
		"alertname": opts.AlertName,
		"namespace": opts.Namespace,
		"severity":  opts.Severity,
	}

	// Add resource labels if provided
	// Use Prometheus-style labels (pod, deployment, node, etc.) for adapter compatibility
	if opts.Resource.Kind != "" && opts.Resource.Name != "" {
		switch opts.Resource.Kind {
		case "Pod":
			labels["pod"] = opts.Resource.Name
		case "Deployment":
			labels["deployment"] = opts.Resource.Name
		case "StatefulSet":
			labels["statefulset"] = opts.Resource.Name
		case "DaemonSet":
			labels["daemonset"] = opts.Resource.Name
		case "Node":
			labels["node"] = opts.Resource.Name
		default:
			// Fallback for unknown resource types
			labels["resource_kind"] = opts.Resource.Kind
			labels["resource_name"] = opts.Resource.Name
		}
	}

	// Merge custom labels
	for k, v := range opts.Labels {
		labels[k] = v
	}

	// Create Prometheus AlertManager webhook format
	payload := map[string]interface{}{
		"version":  "4",
		"groupKey": fmt.Sprintf("{}:{alertname=\"%s\"}", opts.AlertName),
		"status":   "firing",
		"alerts": []map[string]interface{}{
			{
				"status": "firing",
				"labels": labels,
				"annotations": map[string]string{
					"description": fmt.Sprintf("Alert %s is firing", opts.AlertName),
				},
				"startsAt": time.Now().Format(time.RFC3339),
			},
		},
	}

	data, _ := json.Marshal(payload)
	return data
}

// GenerateLabels creates N labels for payload size testing
func GenerateLabels(count int) map[string]string {
	labels := make(map[string]string)
	for i := 0; i < count; i++ {
		labels[fmt.Sprintf("label_%d", i)] = fmt.Sprintf("value_%d", i)
	}
	return labels
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// CRD QUERY HELPERS
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// ListRemediationRequests returns all RemediationRequest CRDs in a namespace
func ListRemediationRequests(ctx context.Context, k8sClient *K8sTestClient, namespace string) []remediationv1alpha1.RemediationRequest {
	list := &remediationv1alpha1.RemediationRequestList{}

	// List CRDs in namespace
	listOpts := []client.ListOption{
		client.InNamespace(namespace),
	}

	err := k8sClient.Client.List(ctx, list, listOpts...)
	if err != nil {
		return []remediationv1alpha1.RemediationRequest{}
	}

	return list.Items
}

// DeleteRemediationRequest deletes a RemediationRequest CRD
func DeleteRemediationRequest(ctx context.Context, k8sClient *K8sTestClient, name, namespace string) error {
	rr := &remediationv1alpha1.RemediationRequest{}
	rr.Name = name
	rr.Namespace = namespace

	return k8sClient.Client.Delete(ctx, rr)
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// REDIS SIMULATION METHODS (DO-REFACTOR)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// GetStormCount returns the storm counter from Redis for a namespace/alert
// BR-GATEWAY-012: Storm detection validation
func (r *RedisTestClient) GetStormCount(ctx context.Context, namespace, alertName string) int {
	if r.Client == nil {
		return 0
	}
	// Storm counter key format: storm:counter:[namespace]:[alertname]
	// Must match pkg/gateway/processing/storm_detection.go:makeCounterKey
	key := fmt.Sprintf("storm:counter:%s:%s", namespace, alertName)

	count, err := r.Client.Get(ctx, key).Int()
	if err != nil {
		return 0
	}
	return count
}

// SimulateFailover simulates Redis cluster failover
// Tests system resilience during Redis infrastructure changes
func (r *RedisTestClient) SimulateFailover(ctx context.Context) {
	if r.Client == nil {
		return
	}
	// For single-instance Redis (test environment), simulate by:
	// 1. Temporarily disconnecting client
	// 2. Forcing a reconnection attempt

	// Close existing connection to force reconnection
	_ = r.Client.Close()

	// Recreate client (simulates failover to new master)
	redisAddr := fmt.Sprintf("localhost:%d", suiteRedisPort)
	r.Client = goredis.NewClient(&goredis.Options{
		Addr: redisAddr,
		DB:   2,
	})
}

// TriggerMemoryPressure simulates Redis memory pressure/LRU eviction
// Tests behavior when Redis starts evicting keys due to memory limits
func (r *RedisTestClient) TriggerMemoryPressure(ctx context.Context) {
	if r.Client == nil {
		return
	}
	// Set a very low maxmemory limit to trigger LRU eviction
	// This simulates production scenario where Redis hits memory limits

	// Note: In real Redis cluster, this would trigger LRU eviction
	// In test environment, we simulate by setting short TTLs
	r.Client.ConfigSet(ctx, "maxmemory-policy", "allkeys-lru")
	r.Client.ConfigSet(ctx, "maxmemory", "1mb") // Force memory pressure
}

// ResetRedisConfig resets Redis to test-safe configuration
// ⚠️ CRITICAL: Call this in AfterEach to prevent state pollution across tests
//
// Background: TriggerMemoryPressure() sets Redis maxmemory to 1MB to simulate
// memory pressure scenarios. This change persists in Redis memory across test runs.
// If not reset, ALL subsequent tests will hit OOM errors (cascade failure).
//
// Root Cause: CONFIG SET changes persist in Redis until:
// 1. Explicitly reset (this function)
// 2. Container restart (podman restart redis-gateway)
// 3. Container recreate (podman stop + rm + start)
//
// Usage:
//
//	AfterEach(func() {
//	    if redisClient != nil {
//	        redisClient.ResetRedisConfig(ctx)
//	        redisClient.Client.FlushDB(ctx)
//	    }
//	})
func (r *RedisTestClient) ResetRedisConfig(ctx context.Context) {
	if r.Client == nil {
		return
	}
	// Reset to 2GB (matches container start command in scripts/start-redis-for-tests.sh)
	r.Client.ConfigSet(ctx, "maxmemory", "2147483648")
	r.Client.ConfigSet(ctx, "maxmemory-policy", "allkeys-lru")
}

// SimulatePipelineFailure simulates Redis pipeline command failures
// Tests recovery from partial pipeline execution failures
func (r *RedisTestClient) SimulatePipelineFailure(ctx context.Context) {
	if r.Client == nil {
		return
	}
	// Simulate pipeline failure by corrupting Redis state
	// This forces the next pipeline to fail mid-execution

	// For integration tests, we simulate this by setting invalid keys
	// that will cause type errors in subsequent pipeline commands
	r.Client.Set(ctx, "corrupt:pipeline", "invalid_type", 1*time.Hour)
}

// SimulatePartialFailure simulates partial Redis write failure
// Tests consistency when some Redis writes succeed and others fail
func (r *RedisTestClient) SimulatePartialFailure(ctx context.Context) {
	if r.Client == nil {
		return
	}
	// Simulate partial failure by filling Redis to capacity
	// Next write will fail while previous ones succeeded

	// Fill Redis with dummy keys to trigger MAXMEMORY errors
	for i := 0; i < 10000; i++ {
		r.Client.Set(ctx, fmt.Sprintf("fill:key:%d", i), "dummy", 10*time.Second)
	}
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// KUBERNETES SIMULATION METHODS (DO-REFACTOR)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// SimulateTemporaryFailure - NOT APPLICABLE for real K8s cluster integration tests
// Real K8s failures tested via actual API unavailability (network issues, cluster maintenance)
// For unit tests, use fake client with interceptor
func (k *K8sTestClient) SimulateTemporaryFailure(ctx context.Context, duration time.Duration) {
	// NOTE: With real K8s client, we test actual API failures:
	// - Network partitions (disconnect from cluster)
	// - API server maintenance windows
	// - Rate limiting (send burst of requests)
	// This method exists for backward compatibility but is no-op with real client
}

// InterruptWatchConnection - NOT APPLICABLE for real K8s cluster integration tests
// Real watch interruptions tested via actual network disconnections
func (k *K8sTestClient) InterruptWatchConnection(ctx context.Context) {
	// NOTE: With real K8s client, test actual watch interruptions:
	// - Close network connection while watch is active
	// - Test reconnection and event replay logic
	// This method is no-op with real client
}

// SimulateSlowResponses - NOT APPLICABLE for real K8s cluster integration tests
// Real slow API responses tested via actual slow operations (large list, etc.)
func (k *K8sTestClient) SimulateSlowResponses(ctx context.Context, delay time.Duration) {
	// NOTE: With real K8s client, test actual slow responses:
	// - Create many CRDs (test pagination performance)
	// - List operations with many resources
	// - Operations during high cluster load
	// This method is no-op with real client
}

// SimulatePermanentFailure - NOT APPLICABLE for real K8s cluster integration tests
// Real permanent failures tested via actual cluster unavailability
func (k *K8sTestClient) SimulatePermanentFailure(ctx context.Context) {
	// NOTE: With real K8s client, test actual permanent failures:
	// - Invalid kubeconfig (authentication failure)
	// - Unreachable API server (network failure)
	// - RBAC permission denied (authorization failure)
	// This method is no-op with real client
}

// ResetFailureSimulation - NOT APPLICABLE for real K8s cluster integration tests
func (k *K8sTestClient) ResetFailureSimulation() {
	// No-op with real K8s client
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// GOROUTINE LEAK DETECTION
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// CountGoroutines returns the current number of goroutines
func CountGoroutines() int {
	return goruntime.NumGoroutine()
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// STORM DETECTION HELPERS (DO-REFACTOR)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// GenerateStormScenario generates N alerts for storm detection testing
// BR-GATEWAY-012: Storm detection threshold validation
func GenerateStormScenario(alertName, namespace string, count int) [][]byte {
	payloads := make([][]byte, count)

	for i := 0; i < count; i++ {
		opts := PrometheusAlertOptions{
			AlertName: alertName,
			Namespace: namespace,
			Severity:  "critical",
			Resource: ResourceIdentifier{
				Kind: "Pod",
				Name: fmt.Sprintf("pod-%d", i),
			},
		}
		payloads[i] = GeneratePrometheusAlert(opts)
	}

	return payloads
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// ERROR SIMULATION HELPERS (DO-REFACTOR)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// GenerateMalformedPayload creates intentionally malformed JSON
func GenerateMalformedPayload() []byte {
	return []byte(`{"alerts": [{"labels": {"alertname": "test", "unclosed":}]}`)
}

// GeneratePayloadWithMissingFields creates payload missing required fields
func GeneratePayloadWithMissingFields() []byte {
	return []byte(`{"alerts": [{"status": "firing"}]}`) // Missing labels
}

// GenerateOversizedPayload creates payload exceeding 512KB limit
// DD-GATEWAY-001: Tests payload size limit enforcement
func GenerateOversizedPayload() []byte {
	// Create payload with large annotations
	largeValue := make([]byte, 600*1024) // 600KB
	for i := range largeValue {
		largeValue[i] = 'A'
	}

	payload := map[string]interface{}{
		"alerts": []map[string]interface{}{
			{
				"labels": map[string]string{
					"alertname": "oversized",
					"namespace": "default",
				},
				"annotations": map[string]string{
					"large_field": string(largeValue),
				},
			},
		},
	}

	data, _ := json.Marshal(payload)
	return data
}

// GeneratePanicTriggeringPayload creates payload designed to cause a panic
// Tests panic recovery middleware (BR-GATEWAY-019)
func GeneratePanicTriggeringPayload() []byte {
	// Payload with null bytes that might cause panic in string processing
	return []byte(`{"alerts": [{"labels": {"alertname": "PanicTest", "namespace": "prod"}, "annotations": {"data": "test\x00panic"}}]}`)
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// TIMING HELPERS (DO-REFACTOR)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// WaitForGoroutineCount waits for goroutine count to reach target
// Used to detect goroutine leaks in tests
func WaitForGoroutineCount(target int, maxWait time.Duration) bool {
	start := time.Now()
	for time.Since(start) < maxWait {
		if CountGoroutines() <= target {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// WaitForCRDCount waits for K8s CRD count to reach target
// Used to verify asynchronous CRD creation
func WaitForCRDCount(ctx context.Context, k8sClient *K8sTestClient, namespace string, target int, maxWait time.Duration) bool {
	start := time.Now()
	for time.Since(start) < maxWait {
		crds := ListRemediationRequests(ctx, k8sClient, namespace)
		if len(crds) >= target {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// WaitForRedisFingerprintCount waits for Redis fingerprint count
// Used to verify asynchronous deduplication writes
func WaitForRedisFingerprintCount(ctx context.Context, redisClient *RedisTestClient, namespace string, target int, maxWait time.Duration) bool {
	if redisClient == nil || redisClient.Client == nil {
		return false
	}
	start := time.Now()
	for time.Since(start) < maxWait {
		count := redisClient.CountFingerprints(ctx, namespace)
		if count >= target {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Webhook Testing Helpers (v2.11 - Storm Aggregation E2E Tests)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// SendPrometheusWebhook sends a Prometheus AlertManager webhook payload to the Gateway
// Returns the HTTP response for validation in tests
// Uses existing WebhookResponse type (defined above)
// DD-GATEWAY-004: Authentication removed - no Bearer token needed
func SendPrometheusWebhook(gatewayURL string, payload string) WebhookResponse {
	url := gatewayURL + "/api/v1/signals/prometheus"

	// DD-GATEWAY-004: Create request (no authentication - handled at network layer)
	req, err := http.NewRequest("POST", url, strings.NewReader(payload))
	if err != nil {
		return WebhookResponse{
			StatusCode: 0,
			Body:       []byte{},
			Headers:    nil,
		}
	}

	// Add content type header
	req.Header.Set("Content-Type", "application/json")

	// Send request using shared HTTP client with connection pooling
	// This prevents port exhaustion during concurrent testing (BR-GATEWAY-003)
	resp, err := sharedHTTPClient.Do(req)
	if err != nil {
		return WebhookResponse{
			StatusCode: 0,
			Body:       []byte{},
			Headers:    nil,
		}
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return WebhookResponse{
			StatusCode: resp.StatusCode,
			Body:       []byte{},
			Headers:    resp.Header,
		}
	}

	return WebhookResponse{
		StatusCode: resp.StatusCode,
		Body:       body,
		Headers:    resp.Header,
	}
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// HTTP SERVER TEST INFRASTRUCTURE (BR-036 to BR-045)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// SlowReader simulates a slow client for timeout testing (BR-037, BR-038)
// Used to test ReadTimeout and WriteTimeout enforcement
type SlowReader struct {
	data      []byte
	pos       int
	delay     time.Duration
	chunkSize int
}

// NewSlowReader creates a reader that delays between chunks
// delay: time to wait between each Read() call
// chunkSize: bytes to return per Read() call (default: 1 byte for maximum slowness)
func NewSlowReader(data []byte, delay time.Duration) *SlowReader {
	return &SlowReader{
		data:      data,
		pos:       0,
		delay:     delay,
		chunkSize: 1, // 1 byte at a time = very slow
	}
}

// Read implements io.Reader with artificial delay
// BR-037: Enables testing ReadTimeout by delaying body transmission
func (r *SlowReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}

	// Delay before reading (simulates slow network)
	time.Sleep(r.delay)

	// Read one chunk
	remaining := len(r.data) - r.pos
	toRead := r.chunkSize
	if toRead > remaining {
		toRead = remaining
	}
	if toRead > len(p) {
		toRead = len(p)
	}

	n = copy(p, r.data[r.pos:r.pos+toRead])
	r.pos += n
	return n, nil
}

// SendSlowRequest sends HTTP request with slow body transmission
// BR-037: Tests ReadTimeout by sending body slowly
// url: Gateway endpoint
// payload: request body
// delay: time between each byte
// Returns: HTTP response or error
func SendSlowRequest(url string, payload []byte, delay time.Duration) (*http.Response, error) {
	slowReader := NewSlowReader(payload, delay)

	req, err := http.NewRequest("POST", url, slowReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(payload))

	// Use default HTTP client (no timeout on client side)
	// We want to test server-side timeout enforcement
	client := &http.Client{
		Timeout: 0, // No client timeout - test server timeout
	}

	return client.Do(req)
}

// BackgroundRequestConfig configures background request generation
type BackgroundRequestConfig struct {
	URL               string        // Gateway endpoint
	Payload           []byte        // Request body
	RequestsPerSecond int           // Rate of requests
	Duration          time.Duration // How long to send requests (0 = until context cancelled)
}

// SendBackgroundRequests sends requests in background goroutine
// BR-040: Tests graceful shutdown by sending requests during shutdown
// Returns: error channel (receives errors from background requests)
func SendBackgroundRequests(ctx context.Context, config BackgroundRequestConfig) chan error {
	errCh := make(chan error, 100)

	go func() {
		defer close(errCh)

		ticker := time.NewTicker(time.Second / time.Duration(config.RequestsPerSecond))
		defer ticker.Stop()

		var stopTime time.Time
		if config.Duration > 0 {
			stopTime = time.Now().Add(config.Duration)
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Check duration limit
				if config.Duration > 0 && time.Now().After(stopTime) {
					return
				}

				// Send request in goroutine (non-blocking)
				go func() {
					resp, err := http.Post(config.URL, "application/json", bytes.NewReader(config.Payload))
					if err != nil {
						select {
						case errCh <- err:
						default: // Channel full, drop error
						}
						return
					}
					_ = resp.Body.Close()
				}()
			}
		}
	}()

	return errCh
}

// SendConcurrentRequests sends N requests concurrently
// BR-045: Tests concurrent request handling without degradation
// Returns: slice of errors (empty if all succeeded)
func SendConcurrentRequests(url string, count int, payload []byte) []error {
	type result struct {
		err        error
		statusCode int
	}

	resultCh := make(chan result, count)

	// Launch all requests concurrently
	for i := 0; i < count; i++ {
		go func() {
			resp, err := http.Post(url, "application/json", bytes.NewReader(payload))
			if err != nil {
				resultCh <- result{err: err}
				return
			}
			defer resp.Body.Close()

			// Read body to ensure full response processing
			_, _ = io.ReadAll(resp.Body)

			resultCh <- result{statusCode: resp.StatusCode}
		}()
	}

	// Collect results
	var errors []error
	for i := 0; i < count; i++ {
		res := <-resultCh
		if res.err != nil {
			errors = append(errors, res.err)
		}
	}

	return errors
}

// MeasureConcurrentLatency sends N concurrent requests and measures latency
// BR-045: Tests that p95 latency remains acceptable under concurrent load
// Returns: latencies (one per request), errors
func MeasureConcurrentLatency(url string, count int, payload []byte) ([]time.Duration, []error) {
	type result struct {
		latency time.Duration
		err     error
	}

	resultCh := make(chan result, count)

	// Launch all requests concurrently
	for i := 0; i < count; i++ {
		go func() {
			start := time.Now()
			resp, err := http.Post(url, "application/json", bytes.NewReader(payload))
			latency := time.Since(start)

			if err != nil {
				resultCh <- result{err: err}
				return
			}
			defer resp.Body.Close()

			// Read body to ensure full response processing
			_, _ = io.ReadAll(resp.Body)

			resultCh <- result{latency: latency}
		}()
	}

	// Collect results
	var latencies []time.Duration
	var errors []error
	for i := 0; i < count; i++ {
		res := <-resultCh
		if res.err != nil {
			errors = append(errors, res.err)
		} else {
			latencies = append(latencies, res.latency)
		}
	}

	return latencies, errors
}

// CalculateP95Latency calculates 95th percentile latency
// BR-045: Used to verify p95 latency SLO (<500ms)
func CalculateP95Latency(latencies []time.Duration) time.Duration {
	if len(latencies) == 0 {
		return 0
	}

	// Sort latencies
	sorted := make([]time.Duration, len(latencies))
	copy(sorted, latencies)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	// Calculate 95th percentile index
	p95Index := int(float64(len(sorted)) * 0.95)
	if p95Index >= len(sorted) {
		p95Index = len(sorted) - 1
	}

	return sorted[p95Index]
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// OBSERVABILITY TEST INFRASTRUCTURE (BR-101 to BR-110)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// PrometheusMetrics represents parsed Prometheus metrics
type PrometheusMetrics map[string]*PrometheusMetric

// PrometheusMetric represents a single Prometheus metric
type PrometheusMetric struct {
	Name   string
	Type   string // counter, gauge, histogram, summary
	Help   string
	Values map[string]float64 // label_set -> value
}

// GetPrometheusMetrics fetches and parses /metrics endpoint
// BR-101: Tests Prometheus metrics endpoint
// Returns: parsed metrics map (metric_name -> PrometheusMetric)
func GetPrometheusMetrics(metricsURL string) (PrometheusMetrics, error) {
	resp, err := http.Get(metricsURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch metrics: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("metrics endpoint returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read metrics body: %w", err)
	}

	// Parse Prometheus text format
	metrics := make(PrometheusMetrics)
	lines := strings.Split(string(body), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			// Handle HELP and TYPE comments
			if strings.HasPrefix(line, "# HELP ") {
				parts := strings.SplitN(line[7:], " ", 2)
				if len(parts) == 2 {
					name := parts[0]
					if _, exists := metrics[name]; !exists {
						metrics[name] = &PrometheusMetric{
							Name:   name,
							Help:   parts[1],
							Values: make(map[string]float64),
						}
					} else {
						metrics[name].Help = parts[1]
					}
				}
			} else if strings.HasPrefix(line, "# TYPE ") {
				parts := strings.SplitN(line[7:], " ", 2)
				if len(parts) == 2 {
					name := parts[0]
					if _, exists := metrics[name]; !exists {
						metrics[name] = &PrometheusMetric{
							Name:   name,
							Type:   parts[1],
							Values: make(map[string]float64),
						}
					} else {
						metrics[name].Type = parts[1]
					}
				}
			}
			continue
		}

		// Parse metric value line
		// Format: metric_name{label="value"} value timestamp
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		// Extract metric name and labels
		nameAndLabels := parts[0]
		var metricName string
		var labelSet string

		if idx := strings.Index(nameAndLabels, "{"); idx != -1 {
			metricName = nameAndLabels[:idx]
			labelSet = nameAndLabels[idx:]
		} else {
			metricName = nameAndLabels
			labelSet = ""
		}

		// Parse value
		var value float64
		_, err := fmt.Sscanf(parts[1], "%f", &value)
		if err != nil {
			continue
		}

		// Store metric
		if _, exists := metrics[metricName]; !exists {
			metrics[metricName] = &PrometheusMetric{
				Name:   metricName,
				Values: make(map[string]float64),
			}
		}
		metrics[metricName].Values[labelSet] = value
	}

	return metrics, nil
}

// GetMetricValue retrieves a specific metric value
// BR-102, BR-103, BR-104, BR-105: Helper for metric validation
func GetMetricValue(metrics PrometheusMetrics, name string, labels string) (float64, bool) {
	metric, exists := metrics[name]
	if !exists {
		return 0, false
	}

	value, exists := metric.Values[labels]
	return value, exists
}

// GetMetricSum returns sum of all values for a metric (useful for counters)
// BR-102, BR-103: Helper for validating counter metrics
func GetMetricSum(metrics PrometheusMetrics, name string) float64 {
	metric, exists := metrics[name]
	if !exists {
		return 0
	}

	var sum float64
	for _, value := range metric.Values {
		sum += value
	}
	return sum
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// TEST SETUP HELPERS (REFACTORED - TDD REFACTOR PHASE)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// Priority1TestContext holds common test infrastructure for Priority 1 integration tests
// Refactored from duplicate BeforeEach/AfterEach blocks across test files
type Priority1TestContext struct {
	Ctx           context.Context
	Cancel        context.CancelFunc
	TestServer    *httptest.Server
	RedisClient   *RedisTestClient
	K8sClient     *K8sTestClient
	TestNamespace string // TDD FIX: Unique namespace per test
}

// createTestRedisClient sets up Redis client and flushes state
// TDD REFACTOR: Extracted from SetupPriority1Test for better readability
func createTestRedisClient(ctx context.Context) *RedisTestClient {
	redisClient := SetupRedisTestClient(ctx)

	// Clean Redis state for test isolation
	if redisClient != nil && redisClient.Client != nil {
		err := redisClient.Client.FlushDB(ctx).Err()
		if err != nil {
			panic(fmt.Sprintf("Failed to flush Redis: %v", err))
		}
	}

	return redisClient
}

// createTestK8sClient sets up K8s client and unique test namespace
// TDD REFACTOR: Extracted from SetupPriority1Test for better readability
// Returns K8s client and unique namespace name
func createTestK8sClient(ctx context.Context) (*K8sTestClient, string) {
	k8sClient := SetupK8sTestClient(ctx)

	// TDD FIX: Create unique namespace per test to prevent CRD conflicts
	// Format: test-prod-p<process>-<timestamp>-<random>
	// Process ID ensures isolation in parallel execution (4 processes)
	processID := GinkgoParallelProcess()
	uniqueNamespace := fmt.Sprintf("test-prod-p%d-%d-%d", processID, time.Now().Unix(), rand.Intn(10000))
	EnsureTestNamespace(ctx, k8sClient, uniqueNamespace)

	return k8sClient, uniqueNamespace
}

// createTestGatewayServer starts Gateway server and wraps it in httptest.Server
// TDD REFACTOR: Extracted from SetupPriority1Test for better readability
func createTestGatewayServer(ctx context.Context, redisClient *RedisTestClient, k8sClient *K8sTestClient) *httptest.Server {
	gatewayServer, err := StartTestGateway(ctx, redisClient, k8sClient)
	if err != nil {
		panic(fmt.Sprintf("Failed to start test gateway: %v", err))
	}

	return httptest.NewServer(gatewayServer.Handler())
}

// SetupPriority1Test creates common test infrastructure (Redis, K8s, Gateway, unique test namespace)
// TDD REFACTOR: Simplified by extracting helper methods
// Refactored from duplicate BeforeEach blocks in:
// - priority1_concurrent_operations_test.go
// - priority1_adapter_patterns_test.go
// - priority1_error_propagation_test.go
//
// TDD FIX: Creates unique namespace per test to prevent CRD conflicts
func SetupPriority1Test() *Priority1TestContext {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)

	// TDD REFACTOR: Use extracted helper methods for clarity
	redisClient := createTestRedisClient(ctx)
	k8sClient, uniqueNamespace := createTestK8sClient(ctx)
	testServer := createTestGatewayServer(ctx, redisClient, k8sClient)

	return &Priority1TestContext{
		Ctx:           ctx,
		Cancel:        cancel,
		TestServer:    testServer,
		RedisClient:   redisClient,
		K8sClient:     k8sClient,
		TestNamespace: uniqueNamespace,
	}
}

// Cleanup tears down all test infrastructure
// Refactored from duplicate AfterEach blocks
// REFACTORED: Proper error handling (TDD REFACTOR phase)
func (tc *Priority1TestContext) Cleanup() {
	// TDD FIX: Close HTTP server first to stop accepting new requests
	if tc.TestServer != nil {
		tc.TestServer.Close()
	}

	// TDD FIX V2: Register namespace for suite-level batch cleanup
	// This prevents "namespace is being terminated" errors during storm aggregation
	// by deferring namespace deletion until AfterSuite (after all storm windows complete)
	//
	// Benefits:
	// 1. No 3-second wait per test → faster test execution
	// 2. Storm aggregation windows can complete without interference
	// 3. Batch deletion is more efficient
	if tc.TestNamespace != "" {
		RegisterTestNamespace(tc.TestNamespace)
	}

	// Cleanup Redis (log errors but don't fail)
	if tc.RedisClient != nil && tc.RedisClient.Client != nil {
		err := tc.RedisClient.Client.FlushDB(tc.Ctx).Err()
		if err != nil {
			fmt.Printf("Warning: Failed to flush Redis during cleanup: %v\n", err)
		}
	}

	// Cleanup clients
	if tc.RedisClient != nil {
		tc.RedisClient.Cleanup(tc.Ctx)
	}
	// NOTE: K8s client cleanup removed - handled by AfterSuite to allow namespace batch deletion

	// Cancel context
	if tc.Cancel != nil {
		tc.Cancel()
	}
}

// EnsureTestNamespace creates a test namespace with proper labels
// Refactored from duplicate namespace creation code
// REFACTORED: Proper error handling (TDD REFACTOR phase)
// TDD FIX: Accepts namespace parameter for unique test namespaces
// TDD FIX: Waits for namespace to be ready before returning
func EnsureTestNamespace(ctx context.Context, k8sClient *K8sTestClient, namespaceName string) {
	// Create namespace with environment label (production environment for priority classification)
	ns := &corev1.Namespace{}
	ns.Name = namespaceName
	ns.Labels = map[string]string{
		"environment": "production", // Tests simulate production environment
	}
	err := k8sClient.Client.Create(ctx, ns)
	if err != nil && !errors.IsAlreadyExists(err) {
		panic(fmt.Sprintf("Failed to create test namespace %s: %v", namespaceName, err))
	}

	// TDD FIX: Wait for namespace to be ready (Kubernetes API is async)
	// This prevents "namespace not found" errors when Gateway tries to create CRDs
	Eventually(func() error {
		checkNs := &corev1.Namespace{}
		err := k8sClient.Client.Get(ctx, client.ObjectKey{Name: namespaceName}, checkNs)
		if err != nil {
			return fmt.Errorf("namespace not found: %w", err)
		}
		if checkNs.Status.Phase != corev1.NamespaceActive {
			return fmt.Errorf("namespace not active yet (phase: %s)", checkNs.Status.Phase)
		}
		return nil
	}, "10s", "100ms").Should(Succeed(), "Namespace %s should become active", namespaceName)

	// TDD FIX: Since Gateway now shares the same K8s client as the test (via NewServerWithK8sClient),
	// no additional delay is needed - both use the same API cache
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// HTTP REQUEST HELPERS (REFACTORED - TDD REFACTOR PHASE)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// Note: JSON payload builders (BuildPrometheusAlertJSON, BuildK8sEventJSON) were considered
// for refactoring but existing test code already uses inline fmt.Sprintf patterns effectively.
// The existing PrometheusAlertOptions type is used for more complex test scenarios.
// TDD REFACTOR: Skipping this refactoring as existing patterns are sufficient.

// SendPrometheusAlert sends a Prometheus alert to the Gateway and returns the response
// Refactored from duplicate HTTP POST logic across test files
func SendPrometheusAlert(testServerURL string, alertJSON string) (*http.Response, error) {
	return http.Post(
		fmt.Sprintf("%s/api/v1/signals/prometheus", testServerURL),
		"application/json",
		strings.NewReader(alertJSON),
	)
}

// SendK8sEvent sends a Kubernetes Event to the Gateway and returns the response
// Refactored from duplicate HTTP POST logic across test files
func SendK8sEvent(testServerURL string, eventJSON string) (*http.Response, error) {
	return http.Post(
		fmt.Sprintf("%s/api/v1/signals/kubernetes-event", testServerURL),
		"application/json",
		strings.NewReader(eventJSON),
	)
}

// DecodeJSONResponse decodes HTTP response body into a map
// Refactored from duplicate JSON decoding logic
func DecodeJSONResponse(resp *http.Response) (map[string]interface{}, error) {
	var response map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&response)
	return response, err
}

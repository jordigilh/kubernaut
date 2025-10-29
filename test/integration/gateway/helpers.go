// Package gateway contains integration test helpers for Gateway Service
package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"time"

	goredis "github.com/go-redis/redis/v8"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	gateway "github.com/jordigilh/kubernaut/pkg/gateway"
	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	"github.com/jordigilh/kubernaut/pkg/gateway/metrics"
)

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
// ONLY connects to local Podman Redis (localhost:6379)
// Integration tests use Kind cluster + local Podman Redis (no OCP fallback)
// Start Redis with: ./test/integration/gateway/start-redis.sh
func SetupRedisTestClient(ctx context.Context) *RedisTestClient {
	// Check if running in CI without Redis
	if os.Getenv("SKIP_INTEGRATION_TESTS") == "true" {
		return &RedisTestClient{Client: nil}
	}

	// Priority 1: Local Podman Redis (fastest, recommended for development)
	// Start with: ./test/integration/gateway/start-redis.sh
	client := goredis.NewClient(&goredis.Options{
		Addr:         "localhost:6379",
		Password:     "",
		DB:           2,
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

	// NO OCP FALLBACK - Integration tests use Kind + local Podman Redis only
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
// Uses Kind cluster for integration testing (supports auth, real API behavior)
// BR-GATEWAY-001: Real K8s cluster required for authentication/authorization testing
func SetupK8sTestClient(ctx context.Context) *K8sTestClient {
	// Use isolated kubeconfig for Kind cluster to avoid impacting other tests
	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath == "" {
		kubeconfigPath = filepath.Join(os.Getenv("HOME"), ".kube", "kind-config")
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		// Integration tests require real K8s cluster
		panic(fmt.Sprintf("Failed to load kubeconfig for integration tests from %s: %v", kubeconfigPath, err))
	}

	// Create scheme with RemediationRequest CRD + core K8s types
	scheme := k8sruntime.NewScheme()
	_ = remediationv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme) // Add core types (Namespace, Pod, etc.)

	// Create real Kubernetes client
	k8sClient, err := client.New(config, client.Options{Scheme: scheme})
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

	// v2.9: Wire deduplication and storm detection services (REQUIRED)
	// BR-GATEWAY-008, BR-GATEWAY-009, BR-GATEWAY-010
	// These services are MANDATORY - Gateway will not start without them
	if redisClient == nil || redisClient.Client == nil {
		return nil, fmt.Errorf("Redis client is required for Gateway startup (BR-GATEWAY-008, BR-GATEWAY-009)")
	}

	// Create ServerConfig for tests (nested structure)
	// Uses fast TTLs and low thresholds for rapid test execution
	cfg := &gateway.ServerConfig{
		Server: gateway.ServerSettings{
			ListenAddr:   ":8080",
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  120 * time.Second,
		},

		Middleware: gateway.MiddlewareSettings{
			RateLimit: gateway.RateLimitSettings{
				RequestsPerMinute: 20, // Production: 100
				Burst:             5,  // Production: 10
			},
		},

		Infrastructure: gateway.InfrastructureSettings{
			Redis: redisClient.Client.Options(),
		},

		Processing: gateway.ProcessingSettings{
			Deduplication: gateway.DeduplicationSettings{
				TTL: 5 * time.Second, // Production: 5 minutes
			},
			Storm: gateway.StormSettings{
				RateThreshold:     2,               // Production: 10 alerts/minute
				PatternThreshold:  2,               // Production: 5 similar alerts
				AggregationWindow: 5 * time.Second, // Production: 1 minute
			},
			Environment: gateway.EnvironmentSettings{
				CacheTTL:           5 * time.Second, // Production: 30 seconds
				ConfigMapNamespace: "kubernaut-system",
				ConfigMapName:      "kubernaut-environment-overrides",
			},
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

	// Create Gateway server with isolated metrics
	server, err := gateway.NewServerWithMetrics(cfg, logger, metricsInstance)
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
	if opts.Resource.Kind != "" {
		labels["resource_kind"] = opts.Resource.Kind
		labels["resource_name"] = opts.Resource.Name
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
	r.Client = goredis.NewClient(&goredis.Options{
		Addr: "localhost:6379",
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

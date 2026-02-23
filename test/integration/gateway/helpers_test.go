// Package gateway contains integration test helpers for Gateway Service
package gateway

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	sharedconfig "github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/audit"
	gateway "github.com/jordigilh/kubernaut/pkg/gateway"
	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	"github.com/jordigilh/kubernaut/pkg/gateway/config"
	"github.com/jordigilh/kubernaut/pkg/gateway/metrics"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// Suite-level namespace tracking for batch cleanup
var (
	testNamespaces      = make(map[string]bool) // Track all test namespaces
	testNamespacesMutex sync.Mutex              // Thread-safe access
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

// TestServerOptions configures Gateway server for integration tests
// Used by StartTestGatewayWithOptions() to create customized test servers
type TestServerOptions struct {
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

// DefaultTestServerOptions returns default options for test Gateway servers
func DefaultTestServerOptions() *TestServerOptions {
	return &TestServerOptions{
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// KUBERNETES TEST CLIENT METHODS
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// suiteK8sConfig holds the K8s config from envtest, set by suite_test.go
// This allows SetupK8sTestClient to use envtest config instead of loading from file
var suiteK8sConfig interface{} // *rest.Config but interface{} to avoid import cycles

// SetSuiteK8sConfig sets the K8s config for the test suite (called from suite_test.go)
func SetSuiteK8sConfig(config interface{}) {
	suiteK8sConfig = config
}

// SetupK8sTestClient creates a Kubernetes client for integration tests
// Uses envtest config if available (set via SetSuiteK8sConfig), otherwise falls back to kubeconfig file
// BR-GATEWAY-001: Real K8s cluster required for authentication/authorization testing
func SetupK8sTestClient(ctx context.Context) *K8sTestClient {
	var config interface{}

	// Priority 1: Use suite-level config from envtest (set by SetSuiteK8sConfig)
	if suiteK8sConfig != nil {
		config = suiteK8sConfig
	} else {
		// Priority 2: Fall back to loading from kubeconfig file (for standalone tests)
		kubeconfigPath := os.Getenv("KUBECONFIG")
		if kubeconfigPath == "" {
			kubeconfigPath = filepath.Join(os.Getenv("HOME"), ".kube", "kind-config")
		}

		fileConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			// Integration tests require real K8s cluster
			panic(fmt.Sprintf("Failed to load kubeconfig for integration tests from %s: %v", kubeconfigPath, err))
		}
		config = fileConfig
	}

	// Create scheme with RemediationRequest CRD + core K8s types
	scheme := k8sruntime.NewScheme()
	_ = remediationv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme) // Add core types (Namespace, Pod, etc.)

	// Create real Kubernetes client using the config (type assert from interface{})
	restConfig, ok := config.(*rest.Config)
	if !ok {
		panic("Invalid K8s config type - expected *rest.Config")
	}

	k8sClient, err := client.New(restConfig, client.Options{Scheme: scheme})
	if err != nil {
		panic(fmt.Sprintf("Failed to create K8s client for integration tests: %v", err))
	}

	return &K8sTestClient{Client: k8sClient}
}

// Cleanup removes all test CRDs from Kubernetes
func (k *K8sTestClient) Cleanup(ctx context.Context) {
	// NOTE: CRD cleanup removed to prevent parallel test interference
	// CRDs are now cleaned up automatically when their namespaces are deleted
	// This prevents race conditions where one test's cleanup deletes another test's CRDs
	// (BR-GATEWAY-TESTING: Parallel test isolation)
	if k.Client == nil {
		return
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
// DD-GATEWAY-012: Redis REMOVED - Gateway is now Redis-free, K8s-native
// DD-AUDIT-003: Gateway emits audit events to Data Storage
//
// Example usage:
//
//	dataStorageURL := fmt.Sprintf("http://127.0.0.1:%d", infrastructure.GatewayIntegrationDataStoragePort)
//	gatewayServer, err := StartTestGateway(ctx, k8sClient, dataStorageURL)
//	Expect(err).ToNot(HaveOccurred())
//	testServer := httptest.NewServer(gatewayServer.Handler())
//	defer testServer.Close()
//	resp, _ := http.Post(testServer.URL+"/api/v1/signals/prometheus", "application/json", body)
//
// DD-GATEWAY-004: Authentication removed - security now at network layer
func StartTestGateway(ctx context.Context, k8sClient *K8sTestClient, dataStorageURL string) (*gateway.Server, error) {
	// STANDARDIZED PATTERN: Respect caller's intent - empty string means no audit store
	// Callers should pass explicit URL: fmt.Sprintf("http://127.0.0.1:%d", infrastructure.GatewayIntegrationDataStoragePort)

	// Use production logger with console output to capture errors in test logs
	logConfig := zap.NewProductionConfig()
	logConfig.OutputPaths = []string{"stdout"}
	logConfig.ErrorOutputPaths = []string{"stderr"}
	logger, _ := logConfig.Build()

	return StartTestGatewayWithLogger(ctx, k8sClient, dataStorageURL, logger)
}

// StartTestGatewayWithLogger creates and starts a Gateway server with a custom logger
// This is useful for observability tests that need to capture and verify log output
// DD-GATEWAY-012: Redis REMOVED - Gateway is now Redis-free
func StartTestGatewayWithLogger(ctx context.Context, k8sClient *K8sTestClient, dataStorageURL string, logger *zap.Logger) (*gateway.Server, error) {
	return StartTestGatewayWithOptions(ctx, k8sClient, dataStorageURL, DefaultTestServerOptions())
}

// StartTestGatewayWithOptions creates a Gateway server with custom timeout options
// Used for testing HTTP timeout behavior (BR-GATEWAY-019, BR-GATEWAY-020)
//
// DD-GATEWAY-012: Redis REMOVED - Gateway is now Redis-free, K8s-native
// DD-AUDIT-003: Gateway emits audit events to Data Storage
func StartTestGatewayWithOptions(ctx context.Context, k8sClient *K8sTestClient, dataStorageURL string, opts *TestServerOptions) (*gateway.Server, error) {
	// Use production logger with console output to capture errors in test logs
	logConfig := zap.NewProductionConfig()
	logConfig.OutputPaths = []string{"stdout"}
	logConfig.ErrorOutputPaths = []string{"stderr"}
	logger, _ := logConfig.Build()

	// DD-GATEWAY-011: Gateway uses K8s CRD Status for deduplication tracking

	// Use options if provided, otherwise use defaults
	if opts == nil {
		opts = DefaultTestServerOptions()
	}

	// Create ServerConfig for tests (nested structure)
	// Uses fast TTLs and low thresholds for rapid test execution
	// DD-GATEWAY-012: NO Redis configuration - Gateway is Redis-free
	cfg := &config.ServerConfig{
		Server: config.ServerSettings{
			ListenAddr:   ":8080",
			ReadTimeout:  opts.ReadTimeout,
			WriteTimeout: opts.WriteTimeout,
			IdleTimeout:  opts.IdleTimeout,
		},

		// Middleware: Rate limiting removed (ADR-048) - delegated to proxy

		// ADR-030: DataStorage connectivity
		DataStorage: sharedconfig.DataStorageConfig{
			URL:     dataStorageURL,
			Timeout: 10 * time.Second,
			Buffer:  sharedconfig.DefaultDataStorageConfig().Buffer,
		},

		Processing: config.ProcessingSettings{
			// DD-GATEWAY-011: Status-based deduplication
			// Note: Environment and Priority settings removed (2025-12-06)
			// Classification now owned by Signal Processing per DD-CATEGORIZATION-001
			Retry: config.DefaultRetrySettings(), // BR-GATEWAY-111: K8s API retry configuration
			Deduplication: config.DeduplicationSettings{
				TTL: 10 * time.Second, // Integration test TTL (production default: 5 minutes)
			},
		},
	}

	logger.Info("Creating Gateway server for integration tests")

	// Create isolated Prometheus registry for this test
	// This prevents "duplicate metrics collector registration" panics when
	// multiple Gateway servers are created in the same test suite
	registry := prometheus.NewRegistry()
	metricsInstance := metrics.NewMetricsWithRegistry(registry)

	// DD-GATEWAY-012: Redis availability metric REMOVED - Gateway is now Redis-free

	// TDD FIX: Use NewServerWithK8sClient to share K8s client with test
	// This ensures Gateway and test use the same K8s API cache, preventing
	// "namespace not found" errors due to cache propagation delays
	// DD-005: Convert zap.Logger to logr.Logger for unified logging (per migration decision)
	logrLogger := zapr.NewLogger(logger)
	server, err := gateway.NewServerWithK8sClient(cfg, logrLogger, metricsInstance, k8sClient.Client)
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

	// BR-GATEWAY-074: Add mandatory timestamp header for security validation
	req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

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

	// Add resource labels using the format expected by Prometheus adapter
	// The adapter extracts Kind/Name from specific labels: pod, deployment, statefulset, etc.
	if opts.Resource.Kind != "" {
		// Use lowercase kind as label key (e.g., "pod", "deployment", "node")
		kindLabel := strings.ToLower(opts.Resource.Kind)
		labels[kindLabel] = opts.Resource.Name // e.g., "pod": "my-pod-name"
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

// GenerateDuplicateScenario generates N alerts for deduplication testing
// BR-GATEWAY-011: Deduplication occurrence count validation
func GenerateDuplicateScenario(alertName, namespace string, count int) [][]byte {
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

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Webhook Testing Helpers (v2.11 - Deduplication Integration Tests)
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

	// BR-GATEWAY-074: Add mandatory timestamp header for security validation
	req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

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
					req, err := http.NewRequest("POST", config.URL, bytes.NewReader(config.Payload))
					if err != nil {
						select {
						case errCh <- err:
						default: // Channel full, drop error
						}
						return
					}
					req.Header.Set("Content-Type", "application/json")
					req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
					resp, err := http.DefaultClient.Do(req)
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
			req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
			if err != nil {
				resultCh <- result{err: err}
				return
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				resultCh <- result{err: err}
				return
			}
			defer func() { _ = resp.Body.Close() }()

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
			req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
			if err != nil {
				resultCh <- result{err: err}
				return
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
			resp, err := http.DefaultClient.Do(req)
			latency := time.Since(start)

			if err != nil {
				resultCh <- result{err: err}
				return
			}
			defer func() { _ = resp.Body.Close() }()

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
	defer func() { _ = resp.Body.Close() }()

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
// DD-GATEWAY-012: Redis removed - Gateway is now Redis-free
type Priority1TestContext struct {
	Ctx           context.Context
	Cancel        context.CancelFunc
	TestServer    *httptest.Server
	K8sClient     *K8sTestClient
	TestNamespace string // TDD FIX: Unique namespace per test
}

// createTestK8sClient sets up K8s client and unique test namespace
// TDD REFACTOR: Extracted from SetupPriority1Test for better readability
// Returns K8s client and unique namespace name
func createTestK8sClient(ctx context.Context) (*K8sTestClient, string) {
	k8sClient := SetupK8sTestClient(ctx)

	// TDD FIX: Create unique namespace per test to prevent CRD conflicts
	// Format: test-prod-<timestamp>-<random>
	uniqueNamespace := fmt.Sprintf("test-prod-%d-%d", time.Now().Unix(), rand.Intn(10000))
	EnsureTestNamespace(ctx, k8sClient, uniqueNamespace)

	return k8sClient, uniqueNamespace
}

// createTestGatewayServer starts Gateway server and wraps it in httptest.Server
// TDD REFACTOR: Extracted from SetupPriority1Test for better readability
// DD-GATEWAY-012: Redis removed - Gateway is now Redis-free
func createTestGatewayServer(ctx context.Context, k8sClient *K8sTestClient) *httptest.Server {
	// STANDARDIZED PATTERN: Explicit DataStorage URL construction from infrastructure constant
	dataStorageURL := fmt.Sprintf("http://127.0.0.1:%d", infrastructure.GatewayIntegrationDataStoragePort)

	gatewayServer, err := StartTestGateway(ctx, k8sClient, dataStorageURL)
	if err != nil {
		panic(fmt.Sprintf("Failed to start test gateway: %v", err))
	}

	return httptest.NewServer(gatewayServer.Handler())
}

// SetupPriority1Test creates common test infrastructure (K8s, Gateway, unique test namespace)
// TDD REFACTOR: Simplified by extracting helper methods
// Refactored from duplicate BeforeEach blocks in:
// - priority1_concurrent_operations_test.go
// - priority1_adapter_patterns_test.go
// - priority1_error_propagation_test.go
//
// DD-GATEWAY-012: Redis removed - Gateway is now Redis-free
// TDD FIX: Creates unique namespace per test to prevent CRD conflicts
func SetupPriority1Test() *Priority1TestContext {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)

	// TDD REFACTOR: Use extracted helper methods for clarity
	k8sClient, uniqueNamespace := createTestK8sClient(ctx)
	testServer := createTestGatewayServer(ctx, k8sClient)

	return &Priority1TestContext{
		Ctx:           ctx,
		Cancel:        cancel,
		TestServer:    testServer,
		K8sClient:     k8sClient,
		TestNamespace: uniqueNamespace,
	}
}

// Cleanup tears down all test infrastructure
// Refactored from duplicate AfterEach blocks
// REFACTORED: Proper error handling (TDD REFACTOR phase)
// DD-GATEWAY-012: Redis removed - Gateway is now Redis-free
func (tc *Priority1TestContext) Cleanup() {
	// TDD FIX: Close HTTP server first to stop accepting new requests
	if tc.TestServer != nil {
		tc.TestServer.Close()
	}

	// TDD FIX V2: Register namespace for suite-level batch cleanup
	// This prevents "namespace is being terminated" errors during test execution
	// by deferring namespace deletion until AfterSuite
	//
	// Benefits:
	// 1. No 3-second wait per test → faster test execution
	// 2. Status updates can complete without interference
	// 3. Batch deletion is more efficient
	if tc.TestNamespace != "" {
		RegisterTestNamespace(tc.TestNamespace)
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
	// First, check if namespace exists and is in Terminating state
	// If so, wait for it to be fully deleted before creating new one
	checkNs := &corev1.Namespace{}
	err := k8sClient.Client.Get(ctx, client.ObjectKey{Name: namespaceName}, checkNs)
	if err == nil && checkNs.Status.Phase == corev1.NamespaceTerminating {
		// Namespace exists but is being deleted - wait for deletion to complete
		Eventually(func() bool {
			err := k8sClient.Client.Get(ctx, client.ObjectKey{Name: namespaceName}, checkNs)
			return errors.IsNotFound(err) // Wait until namespace is gone
		}, "60s", "1s").Should(BeTrue(), "Namespace %s should be fully deleted", namespaceName)
	}

	// Create namespace with environment label and managed label (BR-SCOPE-001)
	ns := &corev1.Namespace{}
	ns.Name = namespaceName
	ns.Labels = map[string]string{
		"kubernaut.ai/managed": "true",       // BR-SCOPE-001: Managed by Kubernaut
		"environment":          "production", // Tests simulate production environment
	}
	err = k8sClient.Client.Create(ctx, ns)
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
	url := fmt.Sprintf("%s/api/v1/signals/prometheus", testServerURL)
	req, err := http.NewRequest("POST", url, strings.NewReader(alertJSON))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
	return http.DefaultClient.Do(req)
}

// SendK8sEvent sends a Kubernetes Event to the Gateway and returns the response
// Refactored from duplicate HTTP POST logic across test files
func SendK8sEvent(testServerURL string, eventJSON string) (*http.Response, error) {
	url := fmt.Sprintf("%s/api/v1/signals/kubernetes-event", testServerURL)
	req, err := http.NewRequest("POST", url, strings.NewReader(eventJSON))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
	return http.DefaultClient.Do(req)
}

// DecodeJSONResponse decodes HTTP response body into a map
// Refactored from duplicate JSON decoding logic
func DecodeJSONResponse(resp *http.Response) (map[string]interface{}, error) {
	var response map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&response)
	return response, err
}

// ========================================
// INTEGRATION TEST HELPERS (Added Jan 2026)
// ========================================

// createGatewayConfig creates a Gateway config for integration tests
// STANDARDIZED PATTERN: Respects caller's intent for DataStorage URL
//   - Explicit URL: Tests WITH audit (use shared audit store)
//   - Empty string: Tests WITHOUT audit (no DataStorage dependency)
func createGatewayConfig(dataStorageURL string) *config.ServerConfig {
	return &config.ServerConfig{
		Server: config.ServerSettings{
			ListenAddr: ":0", // Random port (we don't use HTTP in integration tests)
		},
		DataStorage: sharedconfig.DataStorageConfig{
			URL:     dataStorageURL,
			Timeout: 10 * time.Second,
			Buffer:  sharedconfig.DefaultDataStorageConfig().Buffer,
		},
		Processing: config.ProcessingSettings{
			Retry: config.DefaultRetrySettings(), // Enable K8s API retry (3 attempts)
		},
		// Middleware uses defaults
	}
}

// createGatewayServer creates a Gateway server with shared K8s client for integration tests
// DD-AUTH-014: Uses SHARED audit store to prevent event loss during per-test server lifecycle
//
// ARCHITECTURE NOTE:
// Gateway tests create isolated server instances per test (stateless service testing pattern).
// However, audit events must use a SHARED store to prevent buffer flush issues:
// - Each server instance has short lifecycle (create → test → destroy)
// - NEW audit store = NEW background flusher goroutine
// - Server destruction cancels context → flusher stops → buffered events LOST
// - SHARED audit store = ONE continuous flusher → events reliably flushed
//
// This pattern differs from controller tests (WE, NT, RO) which share both
// controller AND audit store, but matches Gateway's stateless service nature.
func createGatewayServer(cfg *config.ServerConfig, testLogger logr.Logger, k8sClient client.Client, sharedAuditStore audit.AuditStore) (*gateway.Server, error) {
	// Create isolated Prometheus registry for this Gateway instance
	// This prevents "duplicate metrics collector registration" panics when
	// multiple Gateway servers are created in parallel tests
	registry := prometheus.NewRegistry()
	metricsInstance := metrics.NewMetricsWithRegistry(registry)

	// DD-AUTH-014 + DD-AUDIT-003: Use SHARED audit store from suite_test.go
	// The sharedAuditStore has continuous background flusher across all tests
	// BR-SCOPE-002: nil scope checker = no scope filtering in integration tests
	return gateway.NewServerForTesting(cfg, testLogger, metricsInstance, k8sClient, sharedAuditStore, nil)
}

// SignalBuilder provides optional fields for creating test signals
type SignalBuilder struct {
	AlertName    string
	Namespace    string
	Kind         string // Alias for ResourceKind
	ResourceKind string
	Name         string // Alias for ResourceName
	ResourceName string
	Severity     string
	Source       string // Source adapter name
	Labels       map[string]string
	Annotations  map[string]string
}

// createNormalizedSignal creates a NormalizedSignal for integration tests
func createNormalizedSignal(builder SignalBuilder) *types.NormalizedSignal {
	// Handle field aliases
	if builder.Kind != "" {
		builder.ResourceKind = builder.Kind
	}
	if builder.Name != "" {
		builder.ResourceName = builder.Name
	}

	// Defaults
	if builder.AlertName == "" {
		builder.AlertName = "TestAlert"
	}
	if builder.Namespace == "" {
		builder.Namespace = "default"
	}
	if builder.ResourceKind == "" {
		builder.ResourceKind = "Pod"
	}
	if builder.ResourceName == "" {
		builder.ResourceName = "test-pod"
	}
	if builder.Severity == "" {
		builder.Severity = "critical"
	}
	if builder.Source == "" {
		builder.Source = "prometheus-adapter"
	}

	// Generate fingerprint
	fingerprint := generateFingerprint(builder.AlertName, builder.Namespace, builder.ResourceKind, builder.ResourceName)

	now := time.Now()

	// Default empty maps if nil
	if builder.Labels == nil {
		builder.Labels = map[string]string{}
	}
	if builder.Annotations == nil {
		builder.Annotations = map[string]string{}
	}

	signal := &types.NormalizedSignal{
		Fingerprint: fingerprint,
		SignalName:   builder.AlertName,
		Severity:    builder.Severity,
		Namespace:   builder.Namespace,
		Resource: types.ResourceIdentifier{
			Kind:      builder.ResourceKind,
			Name:      builder.ResourceName,
			Namespace: builder.Namespace, // Set resource namespace
		},
		Labels:       builder.Labels,
		Annotations:  builder.Annotations,
		FiringTime:   now,
		ReceivedTime: now,
		SourceType:   "alert",
		Source:       builder.Source,
		RawPayload:   json.RawMessage("{}"),
	}

	return signal
}

// generateFingerprint generates a deterministic fingerprint for a signal (integration test helper)
func generateFingerprint(alertName, namespace, kind, name string) string {
	input := fmt.Sprintf("%s|%s|%s|%s", alertName, namespace, kind, name)
	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", hash)
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// METRICS HELPERS
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// getCounterValue retrieves the current value of a Counter metric
// Returns 0 if the metric doesn't exist or has no matching labels
func getCounterValue(registry *prometheus.Registry, metricName string, labels map[string]string) float64 {
	metricFamilies, err := registry.Gather()
	if err != nil {
		return 0
	}

	for _, mf := range metricFamilies {
		if mf.GetName() != metricName {
			continue
		}

		for _, metric := range mf.GetMetric() {
			if labelsMatch(metric, labels) {
				if metric.Counter != nil {
					return metric.Counter.GetValue()
				}
			}
		}
	}

	return 0
}

// getGaugeValue retrieves the current value of a Gauge metric
func getGaugeValue(registry *prometheus.Registry, metricName string, labels map[string]string) float64 {
	metricFamilies, err := registry.Gather()
	if err != nil {
		return 0
	}

	for _, mf := range metricFamilies {
		if mf.GetName() != metricName {
			continue
		}

		for _, metric := range mf.GetMetric() {
			if labelsMatch(metric, labels) {
				if metric.Gauge != nil {
					return metric.Gauge.GetValue()
				}
			}
		}
	}

	return 0
}

// getHistogramSampleCount retrieves the sample count of a Histogram metric
func getHistogramSampleCount(registry *prometheus.Registry, metricName string, labels map[string]string) uint64 {
	metricFamilies, err := registry.Gather()
	if err != nil {
		return 0
	}

	for _, mf := range metricFamilies {
		if mf.GetName() != metricName {
			continue
		}

		for _, metric := range mf.GetMetric() {
			if labelsMatch(metric, labels) {
				if metric.Histogram != nil {
					return metric.Histogram.GetSampleCount()
				}
			}
		}
	}

	return 0
}

// labelsMatch checks if a metric's labels match the given label map
func labelsMatch(metric *dto.Metric, labels map[string]string) bool {
	if len(labels) == 0 {
		return true
	}

	metricLabels := make(map[string]string)
	for _, labelPair := range metric.GetLabel() {
		metricLabels[labelPair.GetName()] = labelPair.GetValue()
	}

	for key, value := range labels {
		if metricLabels[key] != value {
			return false
		}
	}

	return true
}

// createGatewayServerWithMetrics creates a Gateway server with custom metrics registry
// DD-AUTH-014: Uses SHARED audit store (same rationale as createGatewayServer)
func createGatewayServerWithMetrics(cfg *config.ServerConfig, logger logr.Logger, k8sClient client.Client, metricsInstance *metrics.Metrics, sharedAuditStore audit.AuditStore) (*gateway.Server, error) {
	// DD-AUTH-014 + DD-AUDIT-003: Use SHARED audit store from suite_test.go
	// The sharedAuditStore has continuous background flusher across all tests
	// BR-SCOPE-002: nil scope checker = no scope filtering in integration tests
	return gateway.NewServerForTesting(cfg, logger, metricsInstance, k8sClient, sharedAuditStore, nil)
}

// createPrometheusAlert creates a Prometheus AlertManager webhook payload
// Used for testing Gateway adapter pass-through behavior
func createPrometheusAlert(namespace, alertName, severity, fingerprint, correlationID string) []byte {
	return createPrometheusAlertForPod(namespace, alertName, severity, fingerprint, correlationID, "test-pod-123")
}

// createPrometheusAlertForPod creates a Prometheus AlertManager webhook payload targeting a specific pod.
// Use this when tests need different resources to produce different fingerprints (Issue #63:
// alertname is excluded from fingerprint, so different alertnames for the same pod produce the same fingerprint).
func createPrometheusAlertForPod(namespace, alertName, severity, fingerprint, correlationID, podName string) []byte {
	payload := fmt.Sprintf(`{
		"alerts": [{
			"labels": {
				"alertname": "%s",
				"severity": "%s",
				"namespace": "%s",
				"pod": "%s"
			},
			"annotations": {
				"summary": "Test alert",
				"description": "Test description",
				"correlation_id": "%s"
			},
			"startsAt": "2025-01-15T10:00:00Z"
		}]
	}`, alertName, severity, namespace, podName, correlationID)

	return []byte(payload)
}

// createPrometheusAlertWithoutSeverity creates a Prometheus alert without severity label
// Used for testing BR-GATEWAY-181 default behavior when severity is missing
func createPrometheusAlertWithoutSeverity(namespace, alertName string) []byte {
	payload := fmt.Sprintf(`{
		"alerts": [{
			"labels": {
				"alertname": "%s",
				"namespace": "%s",
				"pod": "test-pod-123"
			},
			"annotations": {
				"summary": "Test alert without severity"
			},
			"startsAt": "2026-01-16T12:00:00Z"
		}]
	}`, alertName, namespace)
	return []byte(payload)
}

// createK8sEvent creates a Kubernetes Event payload
// Used for testing Gateway K8s Event adapter pass-through behavior (BR-GATEWAY-181)
func createK8sEvent(eventType, reason, namespace, kind, name string) []byte {
	payload := fmt.Sprintf(`{
		"type": "%s",
		"reason": "%s",
		"involvedObject": {
			"kind": "%s",
			"name": "%s",
			"namespace": "%s"
		},
		"message": "Test K8s event",
		"firstTimestamp": "2026-01-16T12:00:00Z",
		"lastTimestamp": "2026-01-16T12:00:00Z"
	}`, eventType, reason, kind, name, namespace)
	return []byte(payload)
}

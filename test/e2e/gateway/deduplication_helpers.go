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
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"

	"github.com/google/uuid"
)

// GatewayResponse represents the Gateway API response
type GatewayResponse struct {
	Status                      string `json:"status"`
	Message                     string `json:"message"`
	Fingerprint                 string `json:"fingerprint"`
	Duplicate                   bool   `json:"duplicate"`
	RemediationRequestName      string `json:"remediationRequestName,omitempty"`
	RemediationRequestNamespace string `json:"remediationRequestNamespace,omitempty"`
}

// PrometheusAlertPayload represents a Prometheus AlertManager webhook payload
type PrometheusAlertPayload struct {
	AlertName   string             `json:"alertName"`
	Namespace   string             `json:"namespace"`
	Severity    string             `json:"severity"`
	PodName     string             `json:"podName"`
	Resource    ResourceIdentifier `json:"resource"`
	Labels      map[string]string  `json:"labels"`
	Annotations map[string]string  `json:"annotations"`
}

// WebhookResponse represents an HTTP response
type WebhookResponse struct {
	StatusCode int
	Body       []byte
	Headers    http.Header
}

// createPrometheusWebhookPayload creates a realistic Prometheus webhook payload
func createPrometheusWebhookPayload(payload PrometheusAlertPayload) []byte {
	// Merge required labels with custom labels
	labels := make(map[string]interface{})
	labels["alertname"] = payload.AlertName
	labels["namespace"] = payload.Namespace
	labels["severity"] = payload.Severity
	if payload.PodName != "" {
		labels["pod"] = payload.PodName
	}
	// Support Resource field for resource-aware payloads (audit tests, etc.)
	if payload.Resource.Name != "" {
		switch payload.Resource.Kind {
		case "Pod":
			labels["pod"] = payload.Resource.Name
		case "Deployment":
			labels["deployment"] = payload.Resource.Name
		case "StatefulSet":
			labels["statefulset"] = payload.Resource.Name
		case "DaemonSet":
			labels["daemonset"] = payload.Resource.Name
		case "Node":
			labels["node"] = payload.Resource.Name
		case "Service":
			labels["service"] = payload.Resource.Name
		}
	}
	// Add custom labels
	for k, v := range payload.Labels {
		labels[k] = v
	}

	alert := map[string]interface{}{
		"receiver": "kubernaut",
		"status":   "firing",
		"alerts": []map[string]interface{}{
			{
				"status":      "firing",
				"labels":      labels,
				"annotations": payload.Annotations,
				"startsAt":    time.Now().Format(time.RFC3339),
				"endsAt":      "0001-01-01T00:00:00Z",
			},
		},
		"groupLabels": map[string]string{
			"alertname": payload.AlertName,
		},
		"commonLabels":      labels,
		"commonAnnotations": payload.Annotations,
	}

	body, _ := json.Marshal(alert)
	return body
}

// createPrometheusWebhookPayloadWithTimestamp creates a realistic Prometheus webhook payload with a fixed timestamp
// Used for deterministic fingerprinting in Test 11: Fingerprint Stability
func createPrometheusWebhookPayloadWithTimestamp(payload PrometheusAlertPayload, startsAt string) []byte {
	// Merge required labels with custom labels
	labels := make(map[string]interface{})
	labels["alertname"] = payload.AlertName
	labels["namespace"] = payload.Namespace
	labels["severity"] = payload.Severity
	if payload.PodName != "" {
		labels["pod"] = payload.PodName
	}
	// Support Resource field for resource-aware payloads (audit tests, etc.)
	if payload.Resource.Name != "" {
		switch payload.Resource.Kind {
		case "Pod":
			labels["pod"] = payload.Resource.Name
		case "Deployment":
			labels["deployment"] = payload.Resource.Name
		case "StatefulSet":
			labels["statefulset"] = payload.Resource.Name
		case "DaemonSet":
			labels["daemonset"] = payload.Resource.Name
		case "Node":
			labels["node"] = payload.Resource.Name
		case "Service":
			labels["service"] = payload.Resource.Name
		}
	}
	// Add custom labels
	for k, v := range payload.Labels {
		labels[k] = v
	}

	alert := map[string]interface{}{
		"receiver": "kubernaut",
		"status":   "firing",
		"alerts": []map[string]interface{}{
			{
				"status":      "firing",
				"labels":      labels,
				"annotations": payload.Annotations,
				"startsAt":    startsAt, // Use provided timestamp instead of time.Now()
				"endsAt":      "0001-01-01T00:00:00Z",
			},
		},
		"groupLabels": map[string]string{
			"alertname": payload.AlertName,
		},
		"commonLabels":      labels,
		"commonAnnotations": payload.Annotations,
	}

	body, _ := json.Marshal(alert)
	return body
}

// sendWebhookRequest sends an HTTP POST request to Gateway webhook endpoint
// with mandatory X-Timestamp header for replay attack prevention
func sendWebhookRequest(gatewayURL, path string, body []byte) *WebhookResponse { //nolint:unused
	req, err := http.NewRequest("POST", gatewayURL+path, bytes.NewReader(body))
	Expect(err).ToNot(HaveOccurred(), "HTTP request creation should succeed")

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

	resp, err := http.DefaultClient.Do(req)
	Expect(err).ToNot(HaveOccurred(), "HTTP request should succeed")
	defer func() { _ = resp.Body.Close() }()

	bodyBytes, err := io.ReadAll(resp.Body)
	Expect(err).ToNot(HaveOccurred(), "Should read response body")

	return &WebhookResponse{
		StatusCode: resp.StatusCode,
		Body:       bodyBytes,
		Headers:    resp.Header,
	}
}

// getKubernetesClient creates a Kubernetes client for CRD verification
// This function may panic on error - use getKubernetesClientSafe for Eventually calls
//
// DEPRECATED (DD-E2E-K8S-CLIENT-001): Use suite-level k8sClient instead
// This function creates a new K8s client on every call, leading to rate limiter contention
// when 100+ tests run in parallel (1200 clients total). Suite-level client creates 1 client
// per process (12 total), eliminating rate limiting issues.
// See docs/handoff/E2E_RATE_LIMITER_ROOT_CAUSE_JAN13_2026.md for details.
func getKubernetesClient() client.Client {
	// Load kubeconfig from standard Kind location
	homeDir, err := os.UserHomeDir()
	Expect(err).ToNot(HaveOccurred(), "Failed to get home directory")
	kubeconfigPath := fmt.Sprintf("%s/.kube/gateway-e2e-config", homeDir)

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	Expect(err).ToNot(HaveOccurred(), "Failed to load kubeconfig")

	// Create scheme with RemediationRequest CRD
	scheme := k8sruntime.NewScheme()
	_ = remediationv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	// Create K8s client
	k8sClient, err := client.New(config, client.Options{Scheme: scheme})
	Expect(err).ToNot(HaveOccurred(), "Failed to create K8s client")

	return k8sClient
}

// lastK8sClientError holds the last error from getKubernetesClientSafe for debugging
var lastK8sClientError error

// getKubernetesClientSafe creates a Kubernetes client without panicking on error
// Returns nil if client creation fails - suitable for use inside Eventually
// Check lastK8sClientError for the actual error if nil is returned
func getKubernetesClientSafe() client.Client {
	// Load kubeconfig from standard Kind location
	homeDir, err := os.UserHomeDir()
	if err != nil {
		lastK8sClientError = fmt.Errorf("failed to get home directory: %w", err)
		return nil
	}
	kubeconfigPath := fmt.Sprintf("%s/.kube/gateway-e2e-config", homeDir)

	// Check if kubeconfig file exists
	if _, err := os.Stat(kubeconfigPath); err != nil {
		lastK8sClientError = fmt.Errorf("kubeconfig file check failed: %w", err)
		return nil
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		lastK8sClientError = fmt.Errorf("failed to build config from kubeconfig: %w", err)
		return nil
	}

	// Create scheme with RemediationRequest CRD
	scheme := k8sruntime.NewScheme()
	_ = remediationv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	// Create K8s client
	k8sClient, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		lastK8sClientError = fmt.Errorf("failed to create K8s client: %w", err)
		return nil
	}

	lastK8sClientError = nil
	return k8sClient
}

// GetLastK8sClientError returns the last error from getKubernetesClientSafe
func GetLastK8sClientError() error {
	return lastK8sClientError
}

// GenerateUniqueNamespace generates a unique namespace name for E2E tests
// Format: <prefix>-<process-id>-<timestamp>
// This ensures test isolation when running in parallel
func GenerateUniqueNamespace(prefix string) string {
	processID := GinkgoParallelProcess()
	timestamp := uuid.New().String()[:8]
	return fmt.Sprintf("%s-%d-%s", prefix, processID, timestamp)
}

// CreateNamespaceAndWait creates a namespace and waits for it to be ready
// This prevents race conditions where Gateway tries to create CRDs in non-existent namespaces
func CreateNamespaceAndWait(ctx context.Context, k8sClient client.Client, namespaceName string) error {
	// Create namespace with aggressive retry logic for K8s API rate limiting
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: namespaceName},
	}

	// Retry creation with exponential backoff (handles API rate limiting)
	maxRetries := 5
	var createErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		createErr = k8sClient.Create(ctx, ns)

		// Success or already exists - continue to waiting phase
		if createErr == nil {
			break
		}

		// Check if namespace already exists (race condition in parallel tests)
		var existingNs corev1.Namespace
		if getErr := k8sClient.Get(ctx, client.ObjectKey{Name: namespaceName}, &existingNs); getErr == nil {
			// Namespace exists, treat as success
			createErr = nil
			break
		}

		// Retry with exponential backoff (1s, 2s, 4s, 8s, 16s)
		if attempt < maxRetries-1 {
			backoff := time.Duration(1<<uint(attempt)) * time.Second
			GinkgoWriter.Printf("⚠️  Namespace creation attempt %d/%d failed (will retry in %v): %v\n",
				attempt+1, maxRetries, backoff, createErr)
			time.Sleep(backoff)
		}
	}

	if createErr != nil {
		return fmt.Errorf("failed to create namespace after %d attempts: %w", maxRetries, createErr)
	}

	// Wait for namespace to be active (with longer timeout for overloaded clusters)
	// This is critical for parallel tests to avoid namespace conflicts
	Eventually(func() bool {
		var createdNs corev1.Namespace
		if err := k8sClient.Get(ctx, client.ObjectKey{Name: namespaceName}, &createdNs); err != nil {
			GinkgoWriter.Printf("⚠️  Namespace %s not ready yet (Get failed): %v\n", namespaceName, err)
			return false
		}
		if createdNs.Status.Phase != corev1.NamespaceActive {
			GinkgoWriter.Printf("⚠️  Namespace %s phase: %v (waiting for Active)\n", namespaceName, createdNs.Status.Phase)
		}
		return createdNs.Status.Phase == corev1.NamespaceActive
	}, "60s", "500ms").Should(BeTrue(), fmt.Sprintf("Namespace %s should become active", namespaceName))

	return nil
}

// =============================================================================
// TEMPORARY STUBS for E2E test compilation
// TODO (GW Team): Properly refactor tests to use E2E patterns
// =============================================================================

// TODO (GW Team): These are temporary stubs to make tests compile.
// Tests using these need refactoring to E2E patterns (HTTP → gatewayURL)

type PrometheusAlertOptions struct {
	AlertName   string
	Namespace   string
	Severity    string
	PodName     string
	Resource    ResourceIdentifier
	Labels      map[string]string
	Annotations map[string]string
}

type ResourceIdentifier struct {
	Kind string
	Name string
}

// createPrometheusAlertPayload is a compatibility shim
// TODO (GW Team): Replace calls with createPrometheusWebhookPayload
func createPrometheusAlertPayload(opts PrometheusAlertOptions) []byte {
	return createPrometheusWebhookPayload(PrometheusAlertPayload{
		AlertName:   opts.AlertName,
		Namespace:   opts.Namespace,
		Severity:    opts.Severity,
		PodName:     opts.PodName,
		Labels:      opts.Labels,
		Annotations: opts.Annotations,
	})
}

// sendWebhook is a compatibility shim for E2E tests
// TODO (GW Team): Replace calls with direct HTTP requests to gatewayURL
func sendWebhook(baseURL, path string, payload []byte) *WebhookResponse {
	req, err := http.NewRequest("POST", baseURL+path, bytes.NewBuffer(payload))
	if err != nil {
		return &WebhookResponse{StatusCode: 500, Body: []byte(err.Error())}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return &WebhookResponse{StatusCode: 500, Body: []byte(err.Error())}
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	return &WebhookResponse{
		StatusCode: resp.StatusCode,
		Body:       bodyBytes,
		Headers:    resp.Header,
	}
}

// TODO (GW Team): Additional stubs for compilation

// GeneratePrometheusAlert generates a Prometheus alert payload (accepts either type)
func GeneratePrometheusAlert(opts PrometheusAlertPayload) []byte {
	return createPrometheusWebhookPayload(opts)
}

// SendWebhook sends a webhook request (compatibility shim)
func SendWebhook(url string, payload []byte) *WebhookResponse {
	return sendWebhookRequest(url, "/api/v1/signals/prometheus", payload)
}

// GetPrometheusMetrics fetches and parses metrics from a Prometheus /metrics endpoint
// Returns a map of metric names (without labels) to their numeric values
// Supports Prometheus text exposition format
func GetPrometheusMetrics(url string) (map[string]float64, error) {
	resp, err := http.Get(url) //nolint:gosec,noctx // E2E test helper, URL is controlled
	if err != nil {
		return nil, fmt.Errorf("failed to fetch metrics: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	metrics := make(map[string]float64)
	scanner := bufio.NewScanner(resp.Body)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse metric line: "metric_name{labels} value timestamp"
		// or simple format: "metric_name value"
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		// Extract metric name (before '{' if labels exist)
		metricName := parts[0]
		if idx := strings.Index(metricName, "{"); idx > 0 {
			metricName = metricName[:idx]
		}

		// Parse value (second field)
		value, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			// Skip unparseable values (e.g., "NaN", "Inf")
			continue
		}

		// Store or sum metrics with same name (for different label combinations)
		metrics[metricName] += value
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading metrics: %w", err)
	}

	return metrics, nil
}

// GetMetricSum sums all metric values that start with the given prefix
// Used to aggregate counter values across different label combinations
// Example: GetMetricSum(metrics, "gateway_signals_received_total")
func GetMetricSum(metrics map[string]float64, prefix string) float64 {
	sum := 0.0
	for metricName, value := range metrics {
		if strings.HasPrefix(metricName, prefix) {
			sum += value
		}
	}
	return sum
}

// PrometheusMetrics placeholder type
type PrometheusMetrics map[string]float64

// ListRemediationRequests lists RRs in a namespace
// Returns empty slice if listing fails
func ListRemediationRequests(ctx context.Context, k8sClient client.Client, namespace string) []remediationv1alpha1.RemediationRequest {
	rrList := &remediationv1alpha1.RemediationRequestList{}
	err := k8sClient.List(ctx, rrList, client.InNamespace(namespace))
	if err != nil {
		// Return empty slice on error - caller can check length
		return []remediationv1alpha1.RemediationRequest{}
	}
	return rrList.Items
}

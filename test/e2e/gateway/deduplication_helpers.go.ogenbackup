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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
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
	AlertName   string            `json:"alertName"`
	Namespace   string            `json:"namespace"`
	Severity    string            `json:"severity"`
	PodName     string            `json:"podName"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
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
	timestamp := time.Now().UnixNano()
	return fmt.Sprintf("%s-%d-%d", prefix, processID, timestamp)
}

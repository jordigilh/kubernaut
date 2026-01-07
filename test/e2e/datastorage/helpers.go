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

package datastorage

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"

	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"
	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // Ginkgo/Gomega convention
	. "github.com/onsi/gomega"    //nolint:revive,staticcheck // Ginkgo/Gomega convention
)

// generateUniqueNamespace creates a unique namespace for parallel test execution
// Format: datastorage-e2e-p{process}-{timestamp}
// This enables parallel E2E tests by providing complete namespace isolation
func generateUniqueNamespace() string { //nolint:unused
	return fmt.Sprintf("datastorage-e2e-p%d-%d",
		GinkgoParallelProcess(),
		time.Now().Unix())
}

// waitForPodReady waits for a pod to be ready in the specified namespace
func waitForPodReady(namespace, labelSelector, kubeconfigPath string, timeout time.Duration) error { //nolint:unused
	GinkgoWriter.Printf("⏳ Waiting for pod with label %s in namespace %s to be ready...\n", labelSelector, namespace)

	Eventually(func() bool {
		cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "get", "pods",
			"-n", namespace,
			"-l", labelSelector,
			"-o", "jsonpath={.items[0].status.phase}")
		output, err := cmd.CombinedOutput()
		if err != nil {
			return false
		}
		phase := string(output)
		return phase == "Running"
	}, timeout, 2*time.Second).Should(BeTrue(),
		fmt.Sprintf("Pod with label %s should be ready in namespace %s", labelSelector, namespace))

	GinkgoWriter.Printf("✅ Pod ready: %s in namespace %s\n", labelSelector, namespace)
	return nil
}

// portForwardService starts port-forwarding for a service in the background
// Returns a context cancel function to stop port-forwarding
func portForwardService(ctx context.Context, namespace, serviceName, kubeconfigPath string, localPort, remotePort int) (context.CancelFunc, error) { //nolint:unused
	portForwardCtx, cancel := context.WithCancel(ctx)

	cmd := exec.CommandContext(portForwardCtx, "kubectl", "--kubeconfig", kubeconfigPath, "port-forward",
		"-n", namespace,
		fmt.Sprintf("service/%s", serviceName),
		fmt.Sprintf("%d:%d", localPort, remotePort))

	// Start port-forward in background
	err := cmd.Start()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to start port-forward for %s/%s: %w", namespace, serviceName, err)
	}

	// Per TESTING_GUIDELINES.md: Use Eventually() to verify port-forward is ready
	Eventually(func() bool {
		// Test port is accessible
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", localPort), 500*time.Millisecond)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	}, 30*time.Second, 1*time.Second).Should(BeTrue(), "Port-forward should be established")

	GinkgoWriter.Printf("✅ Port-forward started: %s/%s %d:%d\n", namespace, serviceName, localPort, remotePort)

	// Return cancel function to stop port-forwarding
	return cancel, nil
}

// scalePod scales a deployment to the specified number of replicas
func scalePod(namespace, deploymentName, kubeconfigPath string, replicas int) error {
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "scale", "deployment",
		"-n", namespace,
		deploymentName,
		fmt.Sprintf("--replicas=%d", replicas))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to scale deployment %s/%s to %d replicas: %w, output: %s",
			namespace, deploymentName, replicas, err, output)
	}

	GinkgoWriter.Printf("✅ Scaled deployment %s/%s to %d replicas\n", namespace, deploymentName, replicas)
	return nil
}

// deleteNamespace deletes a namespace
func deleteNamespace(ctx context.Context, namespace, kubeconfigPath string) error { //nolint:unused
	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "delete", "namespace", namespace, "--wait=false")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete namespace %s: %w, output: %s", namespace, err, output)
	}

	GinkgoWriter.Printf("✅ Namespace deletion initiated: %s\n", namespace)
	return nil
}

// createPostgresNetworkPartition creates a NetworkPolicy that blocks DataStorage → PostgreSQL traffic
// This simulates a network partition / cross-AZ failure (more realistic than pod termination for HA scenarios)
func createPostgresNetworkPartition(namespace, kubeconfigPath string) error {
	networkPolicyYAML := fmt.Sprintf(`
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: block-datastorage-to-postgres
  namespace: %s
spec:
  podSelector:
    matchLabels:
      app: postgresql
  policyTypes:
  - Ingress
  ingress:
  # Allow all traffic EXCEPT from DataStorage
  - from:
    - podSelector:
        matchExpressions:
        - key: app
          operator: NotIn
          values:
          - datastorage
`, namespace)

	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(networkPolicyYAML)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create NetworkPolicy in %s: %w, output: %s", namespace, err, output)
	}

	GinkgoWriter.Printf("✅ NetworkPolicy created: DataStorage → PostgreSQL traffic blocked in %s\n", namespace)
	return nil
}

// deletePostgresNetworkPartition deletes the NetworkPolicy that blocks DataStorage → PostgreSQL traffic
func deletePostgresNetworkPartition(namespace, kubeconfigPath string) error {
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "delete", "networkpolicy",
		"-n", namespace,
		"block-datastorage-to-postgres",
		"--ignore-not-found=true")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete NetworkPolicy in %s: %w, output: %s", namespace, err, output)
	}

	GinkgoWriter.Printf("✅ NetworkPolicy deleted: DataStorage → PostgreSQL traffic restored in %s\n", namespace)
	return nil
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// DD-API-001: OpenAPI Client Helper Functions
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// createAuditEventOpenAPI creates an audit event using the OpenAPI client (type-safe)
// Returns the CreateAuditEventResponse which contains JSON201/JSON202 for success
//
// Authority: DD-API-001 (OpenAPI Client Mandate)
// Replaces: postAuditEvent (raw HTTP helper)
func createAuditEventOpenAPI(ctx context.Context, client *dsgen.ClientWithResponses, event dsgen.AuditEventRequest) *dsgen.CreateAuditEventResponse {
	resp, err := client.CreateAuditEventWithResponse(ctx, event)
	Expect(err).ToNot(HaveOccurred(), "Failed to create audit event via OpenAPI client")

	// Log response details if not 2xx status
	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		_, _ = fmt.Fprintf(GinkgoWriter, "❌ HTTP %d Response Body: %s\n", resp.StatusCode(), string(resp.Body))
	}

	return resp
}

// convertMapToAuditEventRequest converts map[string]interface{} to typed AuditEventRequest
// This helper provides backward compatibility during migration from raw HTTP to OpenAPI client
//
// Usage: During Step 3 migration, tests can continue using map[string]interface{} temporarily
// Future: Tests should use typed dsgen.AuditEventRequest directly
func convertMapToAuditEventRequest(eventMap map[string]interface{}) dsgen.AuditEventRequest {
	req := dsgen.AuditEventRequest{}

	// Extract required fields
	if v, ok := eventMap["correlation_id"].(string); ok {
		req.CorrelationId = v
	}
	if v, ok := eventMap["event_type"].(string); ok {
		req.EventType = v
	}
	if v, ok := eventMap["event_category"].(string); ok {
		category := dsgen.AuditEventRequestEventCategory(v)
		req.EventCategory = category
	}
	if v, ok := eventMap["event_outcome"].(string); ok {
		outcome := dsgen.AuditEventRequestEventOutcome(v)
		req.EventOutcome = outcome // EventOutcome is not a pointer
	}
	if v, ok := eventMap["event_action"].(string); ok {
		req.EventAction = v // EventAction is required
	}
	if v, ok := eventMap["cluster_name"].(string); ok {
		clusterName := v
		req.ClusterName = &clusterName
	}
	if v, ok := eventMap["version"].(string); ok {
		req.Version = v // Version is string, not pointer
	}

	// Extract optional fields
	if v, ok := eventMap["actor_id"].(string); ok {
		req.ActorId = &v
	}
	if v, ok := eventMap["actor_type"].(string); ok {
		req.ActorType = &v
	}
	if v, ok := eventMap["resource_type"].(string); ok {
		req.ResourceType = &v
	}
	if v, ok := eventMap["resource_id"].(string); ok {
		req.ResourceId = &v
	}
	if v, ok := eventMap["severity"].(string); ok {
		req.Severity = &v
	}
	if v, ok := eventMap["namespace"].(string); ok {
		req.Namespace = &v
	}

	// event_data is interface{} in the generated client, so pass through
	if v, ok := eventMap["event_data"]; ok {
		req.EventData = v
	}

	// event_timestamp - if provided use it, otherwise will be set by server
	if v, ok := eventMap["event_timestamp"].(time.Time); ok {
		req.EventTimestamp = v
	} else {
		// Default to current time if not provided
		req.EventTimestamp = time.Now()
	}

	return req
}

// createAuditEventFromMap is a convenience wrapper for backward compatibility
// Converts map[string]interface{} to AuditEventRequest and calls createAuditEventOpenAPI
//
// Usage: Allows incremental migration - tests using map[string]interface{} can switch
//        from postAuditEvent to this function without changing event construction
func createAuditEventFromMap(ctx context.Context, client *dsgen.ClientWithResponses, eventMap map[string]interface{}) *dsgen.CreateAuditEventResponse {
	event := convertMapToAuditEventRequest(eventMap)
	return createAuditEventOpenAPI(ctx, client, event)
}

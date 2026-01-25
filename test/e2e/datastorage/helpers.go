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
	"os/exec"
	"strings"
	"time"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // Ginkgo/Gomega convention
	. "github.com/onsi/gomega"    //nolint:revive,staticcheck // Ginkgo/Gomega convention
)

// postAuditEventBatch posts multiple audit events using the ogen client and returns the event IDs
func postAuditEventBatch( //nolint:unused
	ctx context.Context,
	client *ogenclient.Client,
	events []ogenclient.AuditEventRequest,
) ([]string, error) {
	resp, err := client.CreateAuditEventsBatch(ctx, events)
	if err != nil {
		return nil, fmt.Errorf("failed to create audit events batch: %w", err)
	}

	// Extract event IDs from response
	eventIDs := make([]string, len(resp.EventIds))
	for i, id := range resp.EventIds {
		eventIDs[i] = id.String()
	}
	return eventIDs, nil
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

// Minimal Payload Constructors for E2E API Testing
// These create minimal valid payloads to test DataStorage API functionality

func newMinimalGatewayPayload(signalType, alertName string) ogenclient.AuditEventRequestEventData {
	return ogenclient.AuditEventRequestEventData{
		Type: ogenclient.AuditEventRequestEventDataGatewaySignalReceivedAuditEventRequestEventData,
		GatewayAuditPayload: ogenclient.GatewayAuditPayload{
			EventType:   ogenclient.GatewayAuditPayloadEventTypeGatewaySignalReceived,
			SignalType:  ogenclient.GatewayAuditPayloadSignalType(signalType),
			AlertName:   alertName,
			Namespace:   "default",
			Fingerprint: "test-fingerprint",
		},
	}
}

func newMinimalAIAnalysisPayload(analysisName string) ogenclient.AuditEventRequestEventData {
	return ogenclient.AuditEventRequestEventData{
		Type: ogenclient.AuditEventRequestEventDataAianalysisAnalysisCompletedAuditEventRequestEventData,
		AIAnalysisAuditPayload: ogenclient.AIAnalysisAuditPayload{
			EventType:        ogenclient.AIAnalysisAuditPayloadEventTypeAianalysisAnalysisCompleted,
			AnalysisName:     analysisName,
			Namespace:        "default",
			Phase:            "Completed",
			ApprovalRequired: false,
		},
	}
}

func newMinimalWorkflowPayload(workflowID string) ogenclient.AuditEventRequestEventData {
	return ogenclient.AuditEventRequestEventData{
		Type: ogenclient.AuditEventRequestEventDataWorkflowexecutionExecutionStartedAuditEventRequestEventData,
		WorkflowExecutionAuditPayload: ogenclient.WorkflowExecutionAuditPayload{
			EventType:       ogenclient.WorkflowExecutionAuditPayloadEventTypeWorkflowexecutionExecutionStarted,
			WorkflowID:      workflowID,
			WorkflowVersion: "1.0.0",
			TargetResource:  "test-resource",
			Phase:           "Running",
		},
	}
}

func newMinimalGenericPayload() ogenclient.AuditEventRequestEventData {
	// Use WorkflowSearchAuditPayload as a minimal generic payload for testing
	return ogenclient.AuditEventRequestEventData{
		Type: ogenclient.WorkflowSearchAuditPayloadAuditEventRequestEventData,
		WorkflowSearchAuditPayload: ogenclient.WorkflowSearchAuditPayload{
			EventType: ogenclient.WorkflowSearchAuditPayloadEventTypeWorkflowCatalogSearchCompleted,
			Query: ogenclient.QueryMetadata{
				TopK: 10,
			},
			Results: ogenclient.ResultsMetadata{
				TotalFound: 0,
				Returned:   0,
				Workflows:  []ogenclient.WorkflowResultAudit{},
			},
			SearchMetadata: ogenclient.SearchExecutionMetadata{
				DurationMs:          100,
				EmbeddingDimensions: 1536,
				EmbeddingModel:      "text-embedding-ada-002",
			},
		},
	}
}

// createAuditEventOpenAPI creates an audit event using the OpenAPI client (type-safe)
// Returns the event ID from the ogen response
//
// Authority: DD-API-001 (OpenAPI Client Mandate)
// Replaces: postAuditEvent (raw HTTP helper)
func createAuditEventOpenAPI(ctx context.Context, client *ogenclient.Client, event ogenclient.AuditEventRequest) string {
	resp, err := client.CreateAuditEvent(ctx, &event)
	Expect(err).ToNot(HaveOccurred(), "Failed to create audit event via OpenAPI client")

	// Ogen returns concrete types - extract event ID or handle errors
	switch r := resp.(type) {
	case *ogenclient.CreateAuditEventCreated:
		return r.EventID.String()
	case *ogenclient.CreateAuditEventAccepted:
		return r.EventID.String()
	case *ogenclient.CreateAuditEventBadRequest:
		Fail(fmt.Sprintf("API returned 400 Bad Request: %+v", r))
		return ""
	case *ogenclient.CreateAuditEventInternalServerError:
		Fail(fmt.Sprintf("API returned 500 Internal Server Error: %+v", r))
		return ""
	default:
		Fail(fmt.Sprintf("Unexpected response type: %T (full response: %+v)", resp, resp))
		return ""
	}
}

// DD-API-001: Backward compatibility helpers removed
// Tests now use typed dsgen.AuditEventRequest directly for full type safety
// This eliminates the need for map[string]interface{} conversion

// ========================================
// Additional Helpers for Moved HTTP API Tests
// ========================================

// generateTestID creates a unique ID for test data isolation
// Format: test-{process}-{timestamp}
// This enables parallel test execution by ensuring each test has unique data
func generateTestID() string {
	return fmt.Sprintf("test-%d-%d", GinkgoParallelProcess(), time.Now().UnixNano())
}

// createOpenAPIClient creates an ogen client for the DataStorage API
func createOpenAPIClient(baseURL string) (*ogenclient.Client, error) {
	return ogenclient.NewClient(baseURL)
}

// postAuditEvent posts an audit event using the ogen client and returns the event ID
func postAuditEvent(
	ctx context.Context,
	client *ogenclient.Client,
	event ogenclient.AuditEventRequest,
) (string, error) {
	resp, err := client.CreateAuditEvent(ctx, &event)
	if err != nil {
		return "", fmt.Errorf("failed to create audit event: %w", err)
	}

	// Extract event ID from response (ogen unions require type checking)
	switch r := resp.(type) {
	case *ogenclient.CreateAuditEventCreated:
		return r.EventID.String(), nil
	case *ogenclient.CreateAuditEventAccepted:
		return r.EventID.String(), nil
	case *ogenclient.CreateAuditEventBadRequest:
		return "", fmt.Errorf("bad request: %s", r.Detail.Value)
	default:
		return "", fmt.Errorf("unexpected response type: %T", resp)
	}
}

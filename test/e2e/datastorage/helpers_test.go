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

	"github.com/google/uuid"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/ogenx"
	"github.com/jordigilh/kubernaut/test/testutil"
	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // Ginkgo/Gomega convention
	. "github.com/onsi/gomega"    //nolint:revive,staticcheck // Ginkgo/Gomega convention
)

const e2eBundleRef = "quay.io/kubernaut-cicd/test-workflows/placeholder-execution:v1.0.0@sha256:c7583414d1d027de60adb1bd0fac2f1550c3c449365c480fc39a4cab7e39f190"

// e2eTestWorkflowStubContent is valid YAML content for CreateWorkflowInlineRequest.
// Aligns with DS E2E test expectations: discovery queries (ScaleReplicas, critical/production/P0),
// detected labels tests (hpaEnabled, gitOpsTool), and duplicate detection.
// Issue #330: Generated via builder pattern instead of inline YAML.
var e2eTestWorkflowStubContent string

// e2eTestAllDetectedLabelsContent is a workflow with all 8 detectedLabels fields populated.
// Used by E2E-DS-043-005 to verify the full OCI -> DB -> HTTP round-trip for every field.
// Issue #330: Generated via builder pattern instead of inline YAML.
var e2eTestAllDetectedLabelsContent string

func init() {
	stub := testutil.NewTestWorkflowCRD("e2e-stub", "ScaleReplicas", "tekton")
	stub.Spec.Description.What = "Stub workflow for E2E test registration"
	stub.Spec.Description.WhenToUse = "For E2E tests that need a valid CreateWorkflow request body"
	stub.Spec.Labels.Priority = "P0"
	stub.Spec.Execution.Bundle = e2eBundleRef
	stub.Spec.Parameters = []models.WorkflowParameter{
		{Name: "TARGET_RESOURCE", Type: "string", Required: true, Description: "Target resource for remediation"},
	}
	stub.Spec.DetectedLabels = &models.DetectedLabelsSchema{
		HPAEnabled:      "true",
		GitOpsTool:      "argocd",
		PopulatedFields: []string{"hpaEnabled", "gitOpsTool"},
	}
	e2eTestWorkflowStubContent = testutil.MarshalWorkflowCRD(stub)

	allLabels := testutil.NewTestWorkflowCRD("e2e-all-labels", "RestartPod", "tekton")
	allLabels.Spec.Description.What = "Workflow with all 8 detectedLabels fields for round-trip E2E testing"
	allLabels.Spec.Description.WhenToUse = "E2E-DS-043-005: validates every detectedLabels field survives storage"
	allLabels.Spec.Labels.Priority = "P0"
	allLabels.Spec.Execution.Bundle = e2eBundleRef
	allLabels.Spec.Parameters = []models.WorkflowParameter{
		{Name: "TARGET_RESOURCE", Type: "string", Required: true, Description: "Target resource for remediation"},
	}
	allLabels.Spec.DetectedLabels = &models.DetectedLabelsSchema{
		HPAEnabled:      "true",
		PDBProtected:    "true",
		Stateful:        "true",
		HelmManaged:     "true",
		NetworkIsolated: "true",
		GitOpsManaged:   "true",
		GitOpsTool:      "flux",
		ServiceMesh:     "istio",
		PopulatedFields: []string{"hpaEnabled", "pdbProtected", "stateful", "helmManaged", "networkIsolated", "gitOpsManaged", "gitOpsTool", "serviceMesh"},
	}
	e2eTestAllDetectedLabelsContent = testutil.MarshalWorkflowCRD(allLabels)
}

// generateWorkflowContent returns valid inline YAML for CreateWorkflowInlineRequest
// with the given workflowName and version. Useful for tests that need distinct
// workflow versions or names to avoid idempotent 200 OK responses.
// Issue #330: Uses builder pattern instead of brittle fmt.Sprintf.
func generateWorkflowContent(workflowName, version string) string {
	crd := testutil.NewTestWorkflowCRD(workflowName, "ScaleReplicas", "tekton")
	crd.Spec.Version = version
	crd.Spec.Description.What = fmt.Sprintf("Generated workflow %s v%s for E2E testing", workflowName, version)
	crd.Spec.Description.WhenToUse = "E2E tests that need distinct workflow versions"
	crd.Spec.Labels.Priority = "P0"
	crd.Spec.Execution.Bundle = e2eBundleRef
	crd.Spec.Parameters = []models.WorkflowParameter{
		{Name: "TARGET_RESOURCE", Type: "string", Required: true, Description: "Target resource for remediation"},
	}
	crd.Spec.DetectedLabels = &models.DetectedLabelsSchema{
		HPAEnabled:      "true",
		GitOpsTool:      "argocd",
		PopulatedFields: []string{"hpaEnabled", "gitOpsTool"},
	}
	return testutil.MarshalWorkflowCRD(crd)
}

// ensureWorkflowRegistered creates a workflow or retrieves the existing one.
// Handles all CreateWorkflow response types including the parallel race 500
// (deterministic UUID PK collision when multiple Ginkgo processes register the
// same content concurrently). On 409 Conflict or 500, falls back to querying
// the workflow by name via ListWorkflows.
func ensureWorkflowRegistered(ctx context.Context, client *dsgen.Client, content, workflowName string) (string, uuid.UUID) {
	createReq := &dsgen.CreateWorkflowInlineRequest{Content: content}
	createReq.Source.SetTo("e2e-test")

	resp, err := client.CreateWorkflow(ctx, createReq)
	Expect(err).ToNot(HaveOccurred(), "CreateWorkflow HTTP call should succeed")

	switch r := resp.(type) {
	case *dsgen.CreateWorkflowCreated:
		wf := (*dsgen.RemediationWorkflow)(r)
		return wf.WorkflowId.Value.String(), wf.WorkflowId.Value
	case *dsgen.CreateWorkflowOK:
		wf := (*dsgen.RemediationWorkflow)(r)
		return wf.WorkflowId.Value.String(), wf.WorkflowId.Value
	case *dsgen.CreateWorkflowConflict,
		*dsgen.CreateWorkflowInternalServerError:
		// 409: different content for same name+version (conflict)
		// 500: parallel PK race — another process won the INSERT
		// Both cases: the workflow exists in the DB, query it by name.
		listResp, listErr := client.ListWorkflows(ctx, dsgen.ListWorkflowsParams{
			WorkflowName: dsgen.NewOptString(workflowName),
			Limit:        dsgen.NewOptInt(1),
		})
		listErr = ogenx.ToError(listResp, listErr)
		Expect(listErr).ToNot(HaveOccurred(), "ListWorkflows should succeed for existing workflow")
		listResult, ok := listResp.(*dsgen.WorkflowListResponse)
		Expect(ok).To(BeTrue(), "Expected WorkflowListResponse, got %T", listResp)
		Expect(listResult.Workflows).ToNot(BeEmpty(),
			"Workflow '%s' returned %T but query found no results", workflowName, resp)
		return listResult.Workflows[0].WorkflowId.Value.String(), listResult.Workflows[0].WorkflowId.Value
	default:
		Fail(fmt.Sprintf("Unexpected CreateWorkflow response type: %T", resp))
		return "", uuid.Nil
	}
}

// postAuditEventBatch posts multiple audit events using the ogen client and returns the event IDs
func postAuditEventBatch(
	ctx context.Context,
	client *dsgen.Client,
	events []dsgen.AuditEventRequest,
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

func newMinimalGatewayPayload(signalType, alertName string) dsgen.AuditEventRequestEventData {
	return dsgen.AuditEventRequestEventData{
		Type: dsgen.AuditEventRequestEventDataGatewaySignalReceivedAuditEventRequestEventData,
		GatewayAuditPayload: dsgen.GatewayAuditPayload{
			EventType:   dsgen.GatewayAuditPayloadEventTypeGatewaySignalReceived,
			SignalType:  dsgen.GatewayAuditPayloadSignalType(signalType),
			SignalName:   alertName,
			Namespace:   "default",
			Fingerprint: "test-fingerprint",
		},
	}
}

func newMinimalAIAnalysisPayload(analysisName string) dsgen.AuditEventRequestEventData {
	return dsgen.AuditEventRequestEventData{
		Type: dsgen.AuditEventRequestEventDataAianalysisAnalysisCompletedAuditEventRequestEventData,
		AIAnalysisAuditPayload: dsgen.AIAnalysisAuditPayload{
			EventType:        dsgen.AIAnalysisAuditPayloadEventTypeAianalysisAnalysisCompleted,
			AnalysisName:     analysisName,
			Namespace:        "default",
			Phase:            "Completed",
			ApprovalRequired: false,
		},
	}
}

func newMinimalWorkflowPayload(workflowID string) dsgen.AuditEventRequestEventData {
	return dsgen.AuditEventRequestEventData{
		Type: dsgen.AuditEventRequestEventDataWorkflowexecutionExecutionStartedAuditEventRequestEventData,
		WorkflowExecutionAuditPayload: dsgen.WorkflowExecutionAuditPayload{
			EventType:       dsgen.WorkflowExecutionAuditPayloadEventTypeWorkflowexecutionExecutionStarted,
			WorkflowID:      workflowID,
			WorkflowVersion: "1.0.0",
			TargetResource:  "test-resource",
			Phase:           "Running",
		},
	}
}

func newMinimalGenericPayload() dsgen.AuditEventRequestEventData {
	// Use WorkflowDiscoveryAuditPayload as a minimal generic payload for testing
	return dsgen.NewAuditEventRequestEventDataWorkflowCatalogActionsListedAuditEventRequestEventData(
		dsgen.WorkflowDiscoveryAuditPayload{
			EventType: dsgen.WorkflowDiscoveryAuditPayloadEventTypeWorkflowCatalogActionsListed,
			Query: dsgen.QueryMetadata{
				TopK: 10,
			},
			Results: dsgen.ResultsMetadata{
				TotalFound: 0,
				Returned:   0,
				Workflows:  []dsgen.WorkflowResultAudit{},
			},
			SearchMetadata: dsgen.SearchExecutionMetadata{
				DurationMs: 100,
			},
		},
	)
}

// createAuditEventOpenAPI creates an audit event using the OpenAPI client (type-safe)
// Returns the event ID from the ogen response
//
// Authority: DD-API-001 (OpenAPI Client Mandate)
// Replaces: postAuditEvent (raw HTTP helper)
func createAuditEventOpenAPI(ctx context.Context, client *dsgen.Client, event dsgen.AuditEventRequest) string {
	resp, err := client.CreateAuditEvent(ctx, &event)
	Expect(err).ToNot(HaveOccurred(), "Failed to create audit event via OpenAPI client")

	switch r := resp.(type) {
	case *dsgen.AuditEventResponse:
		return r.EventID.String()
	case *dsgen.AsyncAcceptanceResponse:
		Fail(fmt.Sprintf("DB write failed (DLQ fallback returned 202 Accepted): event not persisted synchronously, correlation_id=%s", event.CorrelationID))
		return ""
	case *dsgen.CreateAuditEventBadRequest:
		Fail(fmt.Sprintf("API returned 400 Bad Request: %+v", r))
		return ""
	case *dsgen.CreateAuditEventInternalServerError:
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

// createOpenAPIClient returns the shared authenticated DSClient from suite setup (DD-AUTH-014)
// The baseURL parameter is ignored - all E2E tests use the same DataStorage deployment
// with authentication provided by ServiceAccount Bearer token.
//
// Authority: DD-AUTH-014 (Middleware-based Authentication)
func createOpenAPIClient(baseURL string) (*dsgen.Client, error) {
	// DD-AUTH-014: Return shared authenticated DSClient instead of creating new unauthenticated client
	return DSClient, nil
}

// postAuditEvent posts an audit event using the ogen client and returns the event ID
func postAuditEvent(
	ctx context.Context,
	client *dsgen.Client,
	event dsgen.AuditEventRequest,
) (string, error) {
	resp, err := client.CreateAuditEvent(ctx, &event)
	if err != nil {
		return "", fmt.Errorf("failed to create audit event: %w", err)
	}

	switch r := resp.(type) {
	case *dsgen.AuditEventResponse:
		return r.EventID.String(), nil
	case *dsgen.AsyncAcceptanceResponse:
		return "", fmt.Errorf("DB write failed (DLQ fallback returned 202 Accepted): event not persisted synchronously, correlation_id=%s", event.CorrelationID)
	case *dsgen.CreateAuditEventBadRequest:
		return "", fmt.Errorf("bad request: %s", r.Detail.Value)
	case *dsgen.CreateAuditEventInternalServerError:
		return "", fmt.Errorf("internal server error (500): %+v", r)
	default:
		return "", fmt.Errorf("unexpected response type: %T", resp)
	}
}

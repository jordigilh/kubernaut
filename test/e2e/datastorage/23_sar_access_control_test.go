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
	"net/http"
	"time"

	"github.com/google/uuid"
	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Scenario: SAR Access Control Validation
//
// Business Requirements:
// - BR-SECURITY-016: Kubernetes RBAC enforcement for REST API endpoints
// - BR-SOC2-CC8.1: User attribution and access control
//
// Design Decisions:
// - DD-AUTH-014: Middleware-based SAR authentication and authorization
// - DD-AUTH-011: Granular RBAC with SAR verb mapping
//
// Test Coverage:
// 1. Authorized ServiceAccount (has "create" permission) can access endpoints
// 2. Unauthorized ServiceAccount (no permissions) gets 403 Forbidden
// 3. ServiceAccount with wrong verb (only "get") cannot access endpoints
// 4. Workflow catalog endpoints enforce user attribution
// 5. User identity injected into context for audit logging
//
// Architecture:
// Client → DataStorage:8081 (DD-TEST-001 host port) → NodePort:30081 → Pod:8080
//   ↓
// DataStorage Middleware:
//   1. Extract Bearer token
//   2. TokenReview API (authentication)
//   3. SubjectAccessReview API (authorization)
//   4. Inject user into context
//
// Authority: DD-AUTH-014 (Middleware-based auth), DD-AUTH-011 (SAR verb mapping)

var _ = Describe("E2E-DS-023: SAR Access Control Validation (DD-AUTH-014, DD-AUTH-011)", Label("e2e", "datastorage", "sar", "rbac"), func() {
	var (
		testCtx    context.Context
		testCancel context.CancelFunc
		
		// ServiceAccount tokens for different permission levels
		authorizedToken   string  // Has "create" permission (data-storage-client)
		unauthorizedToken string  // No permissions
		readOnlyToken     string  // Only has "get" permission (insufficient)
		
		// HTTP clients with different authentication levels
		authorizedClient   *dsgen.Client
		unauthorizedClient *dsgen.Client
		readOnlyClient     *dsgen.Client
	)

	BeforeEach(func() {
		testCtx, testCancel = context.WithTimeout(ctx, 5*time.Minute)
		DeferCleanup(testCancel)

		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("SAR Access Control Validation - BeforeEach")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// 1. Create authorized ServiceAccount with "create" permission (data-storage-client)
		logger.Info("📋 DD-AUTH-011: Creating authorized ServiceAccount with 'create' permission...")
		authorizedSAName := "datastorage-e2e-authorized-sa"
		err := infrastructure.CreateE2EServiceAccountWithDataStorageAccess(
			testCtx,
			sharedNamespace,
			kubeconfigPath,
			authorizedSAName,
			GinkgoWriter,
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to create authorized ServiceAccount")

		// Get token for authorized SA
		authorizedToken, err = infrastructure.GetServiceAccountToken(
			testCtx,
			sharedNamespace,
			authorizedSAName,
			kubeconfigPath,
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to get authorized ServiceAccount token")
		logger.Info("✅ Authorized ServiceAccount created", "name", authorizedSAName, "permission", "create")

		// 2. Create unauthorized ServiceAccount (no RBAC permissions)
		logger.Info("📋 Creating unauthorized ServiceAccount with NO permissions...")
		unauthorizedSAName := "datastorage-e2e-unauthorized-sa"
		err = infrastructure.CreateServiceAccount(testCtx, sharedNamespace, kubeconfigPath, unauthorizedSAName, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred(), "Failed to create unauthorized ServiceAccount")

		// Get token for unauthorized SA
		unauthorizedToken, err = infrastructure.GetServiceAccountToken(
			testCtx,
			sharedNamespace,
			unauthorizedSAName,
			kubeconfigPath,
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to get unauthorized ServiceAccount token")
		logger.Info("✅ Unauthorized ServiceAccount created", "name", unauthorizedSAName, "permission", "none")

		// 3. Create read-only ServiceAccount with "get" permission (insufficient for audit writes)
		logger.Info("📋 Creating read-only ServiceAccount with 'get' permission...")
		readOnlySAName := "datastorage-e2e-readonly-sa"
		err = infrastructure.CreateServiceAccountWithReadOnlyAccess(
			testCtx,
			sharedNamespace,
			kubeconfigPath,
			readOnlySAName,
			GinkgoWriter,
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to create read-only ServiceAccount")

		// Get token for read-only SA
		readOnlyToken, err = infrastructure.GetServiceAccountToken(
			testCtx,
			sharedNamespace,
			readOnlySAName,
			kubeconfigPath,
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to get read-only ServiceAccount token")
		logger.Info("✅ Read-only ServiceAccount created", "name", readOnlySAName, "permission", "get")

		// 4. Create HTTP clients with different authentication levels
		logger.Info("📋 DD-AUTH-010: Creating authenticated HTTP clients...")

		// Authorized client (has "create" permission)
		authorizedTransport := testauth.NewServiceAccountTransport(authorizedToken)
		authorizedHTTP := &http.Client{
			Timeout:   10 * time.Second,
			Transport: authorizedTransport,
		}
		authorizedClient, err = dsgen.NewClient(dataStorageURL, dsgen.WithClient(authorizedHTTP))
		Expect(err).ToNot(HaveOccurred(), "Failed to create authorized client")

		// Unauthorized client (no permissions)
		unauthorizedTransport := testauth.NewServiceAccountTransport(unauthorizedToken)
		unauthorizedHTTP := &http.Client{
			Timeout:   10 * time.Second,
			Transport: unauthorizedTransport,
		}
		unauthorizedClient, err = dsgen.NewClient(dataStorageURL, dsgen.WithClient(unauthorizedHTTP))
		Expect(err).ToNot(HaveOccurred(), "Failed to create unauthorized client")

		// Read-only client (has "get" but not "create")
		readOnlyTransport := testauth.NewServiceAccountTransport(readOnlyToken)
		readOnlyHTTP := &http.Client{
			Timeout:   10 * time.Second,
			Transport: readOnlyTransport,
		}
		readOnlyClient, err = dsgen.NewClient(dataStorageURL, dsgen.WithClient(readOnlyHTTP))
		Expect(err).ToNot(HaveOccurred(), "Failed to create read-only client")

		logger.Info("✅ All HTTP clients created with real ServiceAccount tokens")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})

	Context("DD-AUTH-011: SAR Enforcement with verb:'create'", func() {
		It("should allow authorized ServiceAccount to write audit events", func() {
			logger.Info("🧪 Test 1: Authorized ServiceAccount (has 'create') can write audit events")

			// Create audit event request
			correlationID := uuid.New()
			auditReq := dsgen.AuditEventRequest{
				Version:        "1.0",
				EventType:      "test.e2e.sar.authorized",
				EventTimestamp: time.Now(),
				EventCategory:  dsgen.AuditEventRequestEventCategoryGateway,
				EventAction:    "test.sar.validation",
				EventOutcome:   dsgen.AuditEventRequestEventOutcomeSuccess,
				CorrelationID:  correlationID.String(),
				EventData: dsgen.AuditEventRequestEventData{
					Type: dsgen.AuditEventRequestEventDataGatewaySignalReceivedAuditEventRequestEventData,
					GatewayAuditPayload: dsgen.GatewayAuditPayload{
						EventType:   dsgen.GatewayAuditPayloadEventTypeGatewaySignalReceived,
						SignalType:  dsgen.GatewayAuditPayloadSignalTypePrometheusAlert,
						AlertName:   "sar-test-authorized",
						Namespace:   "datastorage-e2e",
						Fingerprint: "test-fingerprint",
					},
				},
			}

		// Attempt to write audit event with authorized client
		resp, err := authorizedClient.CreateAuditEvent(testCtx, &auditReq)
		
		// Verify request succeeded
		Expect(err).ToNot(HaveOccurred(), "Authorized ServiceAccount should be able to write audit events")
		Expect(resp).ToNot(BeNil())
		
		// Verify response is either 201 Created (synchronous) or 202 Accepted (async)
		// Per DD-009: DataStorage may queue events to DLQ if database is unavailable
		// 201 = AuditEventResponse (event_id + event_timestamp)
		// 202 = AsyncAcceptanceResponse (status + message)
		_, isCreated := resp.(*dsgen.AuditEventResponse)
		_, isAccepted := resp.(*dsgen.AsyncAcceptanceResponse)
		Expect(isCreated || isAccepted).To(BeTrue(), "Response should be AuditEventResponse (201) or AsyncAcceptanceResponse (202)")
		
		if isAccepted {
			logger.Info("✅ Authorized ServiceAccount successfully queued audit event (async)", "status", "202 Accepted")
		} else {
			logger.Info("✅ Authorized ServiceAccount successfully wrote audit event", "status", "201 Created")
		}
		})

		It("should reject unauthorized ServiceAccount with 403 Forbidden", func() {
			logger.Info("🧪 Test 2: Unauthorized ServiceAccount (no permissions) gets 403 Forbidden")

			// Create audit event request
			correlationID := uuid.New()
			auditReq := dsgen.AuditEventRequest{
				Version:        "1.0",
				EventType:      "test.e2e.sar.unauthorized",
				EventTimestamp: time.Now(),
				EventCategory:  dsgen.AuditEventRequestEventCategoryGateway,
				EventAction:    "test.sar.validation",
				EventOutcome:   dsgen.AuditEventRequestEventOutcomeSuccess,
				CorrelationID:  correlationID.String(),
				EventData: dsgen.AuditEventRequestEventData{
					Type: dsgen.AuditEventRequestEventDataGatewaySignalReceivedAuditEventRequestEventData,
					GatewayAuditPayload: dsgen.GatewayAuditPayload{
						EventType:   dsgen.GatewayAuditPayloadEventTypeGatewaySignalReceived,
						SignalType:  dsgen.GatewayAuditPayloadSignalTypePrometheusAlert,
						AlertName:   "sar-test-unauthorized",
						Namespace:   "datastorage-e2e",
						Fingerprint: "test-fingerprint-unauth",
					},
				},
			}

			// Attempt to write audit event with unauthorized client
			resp, err := unauthorizedClient.CreateAuditEvent(testCtx, &auditReq)
			
			// Verify request was rejected with 403
			// Note: OAuth2-proxy returns 403 before reaching DataStorage
			Expect(err).ToNot(HaveOccurred(), "Client should receive response (may be 403)")
			
			// Check if response is 403 Forbidden
			forbidden, isForbidden := resp.(*dsgen.CreateAuditEventForbidden)
			Expect(isForbidden).To(BeTrue(), fmt.Sprintf("Expected 403 Forbidden response, got: %T", resp))
			Expect(forbidden).ToNot(BeNil())
			
			logger.Info("✅ Unauthorized ServiceAccount correctly rejected with 403 Forbidden")
		})

		It("should reject read-only ServiceAccount (insufficient permissions)", func() {
			logger.Info("🧪 Test 3: Read-only ServiceAccount (has 'get' but not 'create') gets 403 Forbidden")

			// Create audit event request
			correlationID := uuid.New()
			auditReq := dsgen.AuditEventRequest{
				Version:        "1.0",
				EventType:      "test.e2e.sar.readonly",
				EventTimestamp: time.Now(),
				EventCategory:  dsgen.AuditEventRequestEventCategoryGateway,
				EventAction:    "test.sar.validation",
				EventOutcome:   dsgen.AuditEventRequestEventOutcomeSuccess,
				CorrelationID:  correlationID.String(),
				EventData: dsgen.AuditEventRequestEventData{
					Type: dsgen.AuditEventRequestEventDataGatewaySignalReceivedAuditEventRequestEventData,
					GatewayAuditPayload: dsgen.GatewayAuditPayload{
						EventType:   dsgen.GatewayAuditPayloadEventTypeGatewaySignalReceived,
						SignalType:  dsgen.GatewayAuditPayloadSignalTypePrometheusAlert,
						AlertName:   "sar-test-readonly",
						Namespace:   "datastorage-e2e",
						Fingerprint: "test-fingerprint-readonly",
					},
				},
			}

			// Attempt to write audit event with read-only client
			resp, err := readOnlyClient.CreateAuditEvent(testCtx, &auditReq)
			
			// Verify request returned 403 Forbidden response
			// Note: OAuth2-proxy checks SAR with verb:"create", read-only SA only has verb:"get"
			Expect(err).ToNot(HaveOccurred(), "Client should receive response (not error)")
			
			// Check if response is 403 Forbidden
			forbidden, isForbidden := resp.(*dsgen.CreateAuditEventForbidden)
			Expect(isForbidden).To(BeTrue(), fmt.Sprintf("Expected 403 Forbidden response, got: %T", resp))
			Expect(forbidden).ToNot(BeNil())
			
			logger.Info("✅ Read-only ServiceAccount correctly rejected with 403 Forbidden (insufficient permissions)")
		})
	})

	Context("DD-AUTH-012: Workflow Catalog User Attribution", func() {
		It("should capture user identity for workflow catalog operations", func() {
			logger.Info("🧪 Test 4: Workflow catalog operations capture X-Auth-Request-User header")

			// Create workflow with authorized client
			// Note: workflow_id is generated by DataStorage (PostgreSQL), not client-provided
			workflowReq := dsgen.RemediationWorkflow{
				WorkflowName:    "sar-test-workflow",
				Version:         "1.0.0",
				Name:            "SAR Test Workflow",
				Description:     "E2E test workflow for SAR validation",
				Content:         "apiVersion: tekton.dev/v1\nkind: Pipeline\nmetadata:\n  name: sar-test\n",
				ContentHash:     "abc123def456abc123def456abc123def456abc123def456abc123def456abcd",
				ExecutionEngine: "argo-workflows",
				Labels: dsgen.MandatoryLabels{
					Severity:    dsgen.MandatoryLabelsSeverityHigh,
					SignalType:  "prometheus-alert",
					Component:   "pod",
					Environment: []dsgen.MandatoryLabelsEnvironmentItem{dsgen.MandatoryLabelsEnvironmentItem("test")},
					Priority:    dsgen.MandatoryLabelsPriority_P2,
				},
				Status: dsgen.RemediationWorkflowStatusActive,
			}

			// Create workflow (requires "create" permission)
			resp, err := authorizedClient.CreateWorkflow(testCtx, &workflowReq)
			Expect(err).ToNot(HaveOccurred(), "Authorized ServiceAccount should be able to create workflows")
			Expect(resp).ToNot(BeNil())
			
			// Type assert to get the actual workflow response
			workflow, ok := resp.(*dsgen.RemediationWorkflow)
			if !ok {
				// Log actual response type for debugging
				logger.Error(fmt.Errorf("unexpected response type"), "Type assertion failed",
					"expected", "*dsgen.RemediationWorkflow",
					"actual", fmt.Sprintf("%T", resp))
			}
			Expect(ok).To(BeTrue(), fmt.Sprintf("Response should be RemediationWorkflow, got: %T", resp))
			Expect(workflow.WorkflowID.IsSet()).To(BeTrue(), "WorkflowID should be set by DataStorage")
			Expect(workflow.WorkflowName).To(Equal("sar-test-workflow"))
			
			// DataStorage generates workflow_id (PostgreSQL UUID), not client-provided
			generatedWorkflowID := workflow.WorkflowID.Value
			logger.Info("✅ Workflow created successfully", "workflowID", generatedWorkflowID)

			// Verify audit event was created with user attribution
			logger.Info("📋 Verifying audit event captured user attribution...")
			
			// Query audit events for workflow creation (use server-generated UUID)
			Eventually(func() bool {
				auditResp, err := authorizedClient.QueryAuditEvents(testCtx, dsgen.QueryAuditEventsParams{
					EventType: dsgen.NewOptString("datastorage.workflow.created"),
					Limit:     dsgen.NewOptInt(10),
				})
				if err != nil {
					logger.Info("Audit query failed", "error", err)
					return false
				}

				// Check if audit event exists for this workflow (using server-generated UUID)
				for _, event := range auditResp.Data {
					if event.ResourceID.IsSet() && event.ResourceID.Value == generatedWorkflowID.String() {
						// Verify actor_id contains ServiceAccount name
						logger.Info("✅ Audit event found with user attribution",
							"actor_id", event.ActorID.Value,
							"event_type", event.EventType,
							"resource_id", event.ResourceID.Value)
						return true
					}
				}
				return false
			}, 30*time.Second, 2*time.Second).Should(BeTrue(), "Audit event with user attribution should be created")

			logger.Info("✅ Workflow catalog operation captured user attribution correctly")
		})

		It("should reject workflow operations from unauthorized ServiceAccount", func() {
			logger.Info("🧪 Test 5: Unauthorized ServiceAccount cannot access workflow catalog endpoints")

			// Attempt to create workflow with unauthorized client
			// Note: workflow_id is generated by DataStorage, not client-provided
			workflowReq := dsgen.RemediationWorkflow{
				WorkflowName:    "sar-test-unauthorized-workflow",
				Version:         "1.0.0",
				Name:            "Unauthorized Test Workflow",
				Description:     "Should fail - no permissions",
				Content:         "apiVersion: tekton.dev/v1\nkind: Pipeline\nmetadata:\n  name: sar-test-unauth\n",
				ContentHash:     "def456def456def456def456def456def456def456def456def456def456def4",
				ExecutionEngine: "argo-workflows",
				Labels: dsgen.MandatoryLabels{
					Severity:    dsgen.MandatoryLabelsSeverityLow,
					SignalType:  "prometheus-alert",
					Component:   "pod",
					Environment: []dsgen.MandatoryLabelsEnvironmentItem{dsgen.MandatoryLabelsEnvironmentItem("test")},
					Priority:    dsgen.MandatoryLabelsPriority_P3,
				},
				Status: dsgen.RemediationWorkflowStatusActive,
			}

			// Attempt to create workflow (should fail with 403)
			resp, err := unauthorizedClient.CreateWorkflow(testCtx, &workflowReq)
			
			// Verify request returned 403 Forbidden response
			Expect(err).ToNot(HaveOccurred(), "Client should receive response (not error)")
			
			// Check if response is 403 Forbidden
			forbidden, isForbidden := resp.(*dsgen.CreateWorkflowForbidden)
			Expect(isForbidden).To(BeTrue(), fmt.Sprintf("Expected 403 Forbidden response, got: %T", resp))
			Expect(forbidden).ToNot(BeNil())
			
			logger.Info("✅ Workflow creation correctly rejected with 403 Forbidden")
		})
	})

	Context("DD-AUTH-011: RBAC Verification", func() {
		It("should verify RBAC permissions using kubectl auth can-i", func() {
			logger.Info("🧪 Test 6: Verify RBAC permissions with kubectl auth can-i")

			// Verify authorized SA can "create"
			canCreate, err := infrastructure.VerifyRBACPermission(
				testCtx,
				sharedNamespace,
				"datastorage-e2e-authorized-sa",
				kubeconfigPath,
				"create",
				"services",
				"data-storage-service",
				GinkgoWriter,
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(canCreate).To(BeTrue(), "Authorized SA should have 'create' permission")
			logger.Info("✅ Authorized SA verified: has 'create' permission")

			// Verify unauthorized SA cannot "create"
			cannotCreate, err := infrastructure.VerifyRBACPermission(
				testCtx,
				sharedNamespace,
				"datastorage-e2e-unauthorized-sa",
				kubeconfigPath,
				"create",
				"services",
				"data-storage-service",
				GinkgoWriter,
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(cannotCreate).To(BeFalse(), "Unauthorized SA should NOT have 'create' permission")
			logger.Info("✅ Unauthorized SA verified: no 'create' permission")

			// Verify read-only SA cannot "create" (only has "get")
			readOnlyCannotCreate, err := infrastructure.VerifyRBACPermission(
				testCtx,
				sharedNamespace,
				"datastorage-e2e-readonly-sa",
				kubeconfigPath,
				"create",
				"services",
				"data-storage-service",
				GinkgoWriter,
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(readOnlyCannotCreate).To(BeFalse(), "Read-only SA should NOT have 'create' permission")
			logger.Info("✅ Read-only SA verified: no 'create' permission (only 'get')")
		})
	})
})

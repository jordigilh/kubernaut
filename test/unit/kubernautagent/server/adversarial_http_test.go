/*
Copyright 2026 Jordi Gil.

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

package server_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	hapiclient "github.com/jordigilh/kubernaut/pkg/agentclient"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/server"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
)

var _ = Describe("TP-433-ADV P6: HTTP Contract — GAP-004/015/016/018", func() {

	var (
		store   *session.Store
		manager *session.Manager
		handler *server.Handler
		logger  *slog.Logger
	)

	BeforeEach(func() {
		store = session.NewStore(5 * time.Minute)
		logger = slog.Default()
		manager = session.NewManager(store, logger)
		handler = server.NewHandler(manager, nil, logger)
	})

	Describe("UT-KA-433-HTTP-001: Error response has RFC 7807 fields (GAP-004)", func() {
		It("should return error with detail field when investigator is nil", func() {
			req := &hapiclient.IncidentRequest{
				IncidentID:        "test-001",
				SignalName:        "OOMKilled",
				ResourceNamespace: "prod",
				Severity:          hapiclient.SeverityCritical,
				ErrorMessage:      "OOMKilled",
				ResourceKind:      "Pod",
				ResourceName:      "api-server",
			}
			resp, err := handler.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePost(context.Background(), req)
			Expect(err).NotTo(HaveOccurred())

			errResp, ok := resp.(*hapiclient.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePostApplicationJSONInternalServerError)
			Expect(ok).To(BeTrue(), "error response should be internal server error type")
			Expect(errResp.Detail).To(ContainSubstring("investigator not configured"))
		})
	})

	Describe("UT-KA-433-HTTP-005: HR reason mapping from HumanReviewReason field (GAP-015)", func() {
		It("should map HumanReviewReason to the correct enum value", func() {
			result := &katypes.InvestigationResult{
				RCASummary:        "Low confidence finding",
				HumanReviewNeeded: true,
				HumanReviewReason: "low_confidence",
				Confidence:        0.3,
			}

			sessionID, err := manager.StartInvestigation(
				context.Background(),
				func(_ context.Context) (interface{}, error) { return result, nil },
				map[string]string{"incident_id": "hr-test-001"},
			)
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() session.Status {
				s, _ := manager.GetSession(sessionID)
				return s.Status
			}).Should(Equal(session.StatusCompleted))

			params := hapiclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetParams{
				SessionID: sessionID,
			}
			resp, err := handler.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGet(context.Background(), params)
			Expect(err).NotTo(HaveOccurred())

			incResp, ok := resp.(*hapiclient.IncidentResponse)
			Expect(ok).To(BeTrue())
			Expect(incResp.NeedsHumanReview.Value).To(BeTrue())
			hrReason, hasReason := incResp.HumanReviewReason.Get()
			Expect(hasReason).To(BeTrue())
			Expect(hrReason).To(Equal(hapiclient.HumanReviewReasonLowConfidence))
		})
	})

	Describe("UT-KA-433-HTTP-006: HR reason from Reason field (legacy) (GAP-015)", func() {
		It("should map legacy Reason field correctly", func() {
			result := &katypes.InvestigationResult{
				RCASummary:        "Max turns exhausted",
				HumanReviewNeeded: true,
				Reason:            "max turns (3) exhausted during RCA",
				Confidence:        0.0,
			}

			sessionID, err := manager.StartInvestigation(
				context.Background(),
				func(_ context.Context) (interface{}, error) { return result, nil },
				map[string]string{"incident_id": "hr-test-002"},
			)
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() session.Status {
				s, _ := manager.GetSession(sessionID)
				return s.Status
			}).Should(Equal(session.StatusCompleted))

			params := hapiclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetParams{
				SessionID: sessionID,
			}
			resp, err := handler.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGet(context.Background(), params)
			Expect(err).NotTo(HaveOccurred())

			incResp, ok := resp.(*hapiclient.IncidentResponse)
			Expect(ok).To(BeTrue())
			hrReason, hasReason := incResp.HumanReviewReason.Get()
			Expect(hasReason).To(BeTrue())
			Expect(hrReason).To(Equal(hapiclient.HumanReviewReasonRcaIncomplete))
		})
	})

	Describe("UT-KA-433-HTTP-010: execution_bundle included in selected_workflow response (GAP-009)", func() {
		It("should include execution_bundle in selected_workflow when present", func() {
			result := &katypes.InvestigationResult{
				RCASummary:      "OOMKill",
				WorkflowID:      "oom-recovery",
				ExecutionBundle: "ghcr.io/kubernaut/oom-recovery:v1.0@sha256:abc",
				Confidence:      0.9,
			}

			sessionID, err := manager.StartInvestigation(
				context.Background(),
				func(_ context.Context) (interface{}, error) { return result, nil },
				map[string]string{"incident_id": "eb-test-001"},
			)
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() session.Status {
				s, _ := manager.GetSession(sessionID)
				return s.Status
			}).Should(Equal(session.StatusCompleted))

			params := hapiclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetParams{
				SessionID: sessionID,
			}
			resp, err := handler.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGet(context.Background(), params)
			Expect(err).NotTo(HaveOccurred())

			incResp, ok := resp.(*hapiclient.IncidentResponse)
			Expect(ok).To(BeTrue())

			sw, hasSW := incResp.SelectedWorkflow.Get()
			Expect(hasSW).To(BeTrue())

			ebRaw, hasEB := sw["execution_bundle"]
			Expect(hasEB).To(BeTrue(), "selected_workflow should include execution_bundle")
			var ebValue string
			Expect(json.Unmarshal(ebRaw, &ebValue)).To(Succeed())
			Expect(ebValue).To(Equal("ghcr.io/kubernaut/oom-recovery:v1.0@sha256:abc"))
		})
	})

	// ===== Audit findings =====

	Describe("AUDIT-H1: MapIncidentRequestToSignal wires GAP-008/014 fields", func() {
		It("should populate RemediationID", func() {
			req := &hapiclient.IncidentRequest{
				IncidentID:        "h1-test",
				RemediationID:     "rem-uuid-12345",
				SignalName:        "OOMKilled",
				Severity:          hapiclient.SeverityHigh,
				ResourceNamespace: "prod",
				ResourceKind:      "Pod",
				ResourceName:      "test-pod",
				ErrorMessage:      "OOM",
				Environment:       "production",
				Priority:          "high",
				RiskTolerance:     "medium",
				BusinessCategory:  "test",
				ClusterName:       "test-cluster",
				SignalSource:      "kubernetes",
			}

			signal := server.MapIncidentRequestToSignal(req)
			Expect(signal.RemediationID).To(Equal("rem-uuid-12345"))
		})

		It("should populate FiringTime and ReceivedTime when present", func() {
			req := &hapiclient.IncidentRequest{
				IncidentID:        "h1-time-test",
				SignalName:        "HighMemory",
				Severity:          hapiclient.SeverityCritical,
				ResourceNamespace: "prod",
				ResourceKind:      "Pod",
				ResourceName:      "api-pod",
				ErrorMessage:      "Memory high",
				Environment:       "production",
				Priority:          "critical",
				RiskTolerance:     "low",
				BusinessCategory:  "core",
				ClusterName:       "prod-1",
				SignalSource:      "prometheus",
			}
			req.FiringTime.SetTo("2026-03-01T12:00:00Z")
			req.ReceivedTime.SetTo("2026-03-01T12:00:05Z")

			signal := server.MapIncidentRequestToSignal(req)
			Expect(signal.FiringTime).To(Equal("2026-03-01T12:00:00Z"))
			Expect(signal.ReceivedTime).To(Equal("2026-03-01T12:00:05Z"))
		})

		It("should populate IsDuplicate and OccurrenceCount", func() {
			req := &hapiclient.IncidentRequest{
				IncidentID:        "h1-dedup-test",
				SignalName:        "OOMKilled",
				Severity:          hapiclient.SeverityHigh,
				ResourceNamespace: "prod",
				ResourceKind:      "Pod",
				ResourceName:      "test-pod",
				ErrorMessage:      "OOM",
				Environment:       "production",
				Priority:          "high",
				RiskTolerance:     "medium",
				BusinessCategory:  "test",
				ClusterName:       "test-cluster",
				SignalSource:      "kubernetes",
			}
			req.IsDuplicate.SetTo(true)
			req.OccurrenceCount.SetTo(5)

			signal := server.MapIncidentRequestToSignal(req)
			Expect(signal.IsDuplicate).NotTo(BeNil())
			Expect(*signal.IsDuplicate).To(BeTrue())
			Expect(signal.OccurrenceCount).NotTo(BeNil())
			Expect(*signal.OccurrenceCount).To(Equal(5))
		})
	})

	Describe("AUDIT-H2: alternative_workflows mapped in response", func() {
		It("should include alternative_workflows in IncidentResponse when present", func() {
			result := &katypes.InvestigationResult{
				RCASummary: "Memory leak",
				WorkflowID: "oom-recovery",
				Confidence: 0.85,
				AlternativeWorkflows: []katypes.AlternativeWorkflow{
					{WorkflowID: "memory-optimize", Confidence: 0.6, Rationale: "Could optimize memory"},
					{WorkflowID: "horizontal-scale", Confidence: 0.4, Rationale: "Scale out"},
				},
			}

			sessionID, err := manager.StartInvestigation(
				context.Background(),
				func(_ context.Context) (interface{}, error) { return result, nil },
				map[string]string{"incident_id": "h2-test"},
			)
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() session.Status {
				s, _ := manager.GetSession(sessionID)
				return s.Status
			}).Should(Equal(session.StatusCompleted))

			params := hapiclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetParams{
				SessionID: sessionID,
			}
			resp, err := handler.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGet(context.Background(), params)
			Expect(err).NotTo(HaveOccurred())

			incResp, ok := resp.(*hapiclient.IncidentResponse)
			Expect(ok).To(BeTrue())
			Expect(incResp.AlternativeWorkflows).To(HaveLen(2),
				"H2: alternative_workflows must be mapped to response")
		})
	})
})

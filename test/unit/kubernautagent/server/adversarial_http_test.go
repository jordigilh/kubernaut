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

	"github.com/jordigilh/kubernaut/pkg/agentclient"

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
		It("should return 500 problem+json with detail field when investigator is nil", func() {
			req := &agentclient.IncidentRequest{
				IncidentID:        "test-001",
				RemediationID:     "rem-001",
				SignalName:        "OOMKilled",
				ResourceNamespace: "prod",
				Severity:          agentclient.SeverityCritical,
				ErrorMessage:      "OOMKilled",
				ResourceKind:      "Pod",
				ResourceName:      "api-server",
			}
			resp, err := handler.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePost(context.Background(), req)
			Expect(err).NotTo(HaveOccurred())

			errResp, ok := resp.(*agentclient.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePostInternalServerErrorApplicationProblemJSON)
			Expect(ok).To(BeTrue(), "error response should be problem+json 500 type")
			Expect(errResp.Detail).To(ContainSubstring("investigator not configured"))
			Expect(errResp.Type).To(Equal("https://kubernaut.ai/problems/internal-error"))
			Expect(errResp.Status).To(Equal(500))
		})
	})

	Describe("UT-KA-433-HTTP-002: Missing remediation_id returns 422 problem+json", func() {
		It("should return 422 with validation error when remediation_id is missing", func() {
			req := &agentclient.IncidentRequest{
				IncidentID:        "test-val-001",
				SignalName:        "OOMKilled",
				ResourceNamespace: "prod",
				Severity:          agentclient.SeverityCritical,
				ErrorMessage:      "OOMKilled",
				ResourceKind:      "Pod",
				ResourceName:      "api-server",
			}
			resp, err := handler.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePost(context.Background(), req)
			Expect(err).NotTo(HaveOccurred())

			errResp, ok := resp.(*agentclient.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePostUnprocessableEntityApplicationProblemJSON)
			Expect(ok).To(BeTrue(), "missing remediation_id should return 422 problem+json")
			Expect(errResp.Status).To(Equal(422))
			Expect(errResp.Type).To(Equal("https://kubernaut.ai/problems/validation-error"))
			Expect(errResp.Title).To(Equal("Validation Error"))
			Expect(errResp.Detail).To(ContainSubstring("remediation_id"))
		})
	})

	Describe("UT-KA-433-HTTP-003: Missing incident_id returns 422 problem+json", func() {
		It("should return 422 with validation error when incident_id is missing", func() {
			req := &agentclient.IncidentRequest{
				RemediationID:     "rem-test",
				SignalName:        "OOMKilled",
				ResourceNamespace: "prod",
				Severity:          agentclient.SeverityCritical,
				ErrorMessage:      "OOMKilled",
				ResourceKind:      "Pod",
				ResourceName:      "api-server",
			}
			resp, err := handler.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePost(context.Background(), req)
			Expect(err).NotTo(HaveOccurred())

			errResp, ok := resp.(*agentclient.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePostUnprocessableEntityApplicationProblemJSON)
			Expect(ok).To(BeTrue(), "missing incident_id should return 422 problem+json")
			Expect(errResp.Status).To(Equal(422))
			Expect(errResp.Detail).To(ContainSubstring("incident_id"))
		})
	})

	Describe("UT-KA-433-HTTP-004: 422 response includes instance field", func() {
		It("should set instance to /api/v1/incident/analyze", func() {
			req := &agentclient.IncidentRequest{
				SignalName:        "OOMKilled",
				ResourceNamespace: "prod",
				Severity:          agentclient.SeverityCritical,
				ErrorMessage:      "OOMKilled",
				ResourceKind:      "Pod",
				ResourceName:      "api-server",
			}
			resp, err := handler.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePost(context.Background(), req)
			Expect(err).NotTo(HaveOccurred())

			errResp, ok := resp.(*agentclient.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePostUnprocessableEntityApplicationProblemJSON)
			Expect(ok).To(BeTrue(), "expected type assertion to *IncidentAnalyzePostUnprocessableEntityApplicationProblemJSON to succeed")
			Expect(errResp.Instance).To(Equal("/api/v1/incident/analyze"))
		})
	})

	Describe("UT-KA-433-HTTP-007: 500 error returns problem+json", func() {
		It("should return problem+json with type/title/status when investigation cannot start", func() {
			req := &agentclient.IncidentRequest{
				IncidentID:        "test-500",
				RemediationID:     "rem-500",
				SignalName:        "OOMKilled",
				ResourceNamespace: "prod",
				Severity:          agentclient.SeverityCritical,
				ErrorMessage:      "OOMKilled",
				ResourceKind:      "Pod",
				ResourceName:      "api-server",
			}
			resp, err := handler.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePost(context.Background(), req)
			Expect(err).NotTo(HaveOccurred())

			errResp, ok := resp.(*agentclient.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePostInternalServerErrorApplicationProblemJSON)
			Expect(ok).To(BeTrue(), "500 errors should use problem+json type")
			Expect(errResp.Type).To(Equal("https://kubernaut.ai/problems/internal-error"))
			Expect(errResp.Title).To(Equal("Internal Server Error"))
			Expect(errResp.Status).To(Equal(500))
			Expect(errResp.Instance).To(Equal("/api/v1/incident/analyze"))
		})
	})

	Describe("UT-KA-433-HTTP-008: Session not found returns 404", func() {
		It("should return 404 empty response for unknown session_id on result endpoint", func() {
			params := agentclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetParams{
				SessionID: "non-existent-uuid",
			}
			resp, err := handler.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGet(context.Background(), params)
			Expect(err).NotTo(HaveOccurred())

			_, ok := resp.(*agentclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetNotFound)
			Expect(ok).To(BeTrue(), "unknown session should return 404")
		})
	})

	Describe("UT-KA-433-HTTP-009: Session not completed returns 409", func() {
		It("should return 409 when session is still running", func() {
			sessionID, err := manager.StartInvestigation(
				context.Background(),
				func(bgCtx context.Context) (interface{}, error) {
					<-bgCtx.Done()
					return nil, bgCtx.Err()
				},
				map[string]string{"incident_id": "running-test"},
			)
			Expect(err).NotTo(HaveOccurred())

			params := agentclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetParams{
				SessionID: sessionID,
			}
			resp, err := handler.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGet(context.Background(), params)
			Expect(err).NotTo(HaveOccurred())

			_, ok := resp.(*agentclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetConflict)
			Expect(ok).To(BeTrue(), "running session should return 409")
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

			params := agentclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetParams{
				SessionID: sessionID,
			}
			resp, err := handler.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGet(context.Background(), params)
			Expect(err).NotTo(HaveOccurred())

			incResp, ok := resp.(*agentclient.IncidentResponse)
			Expect(ok).To(BeTrue(), "expected type assertion to *IncidentResponse to succeed (HumanReviewReason from field)")
			Expect(incResp.NeedsHumanReview.Value).To(BeTrue())
			hrReason, hasReason := incResp.HumanReviewReason.Get()
			Expect(hasReason).To(BeTrue())
			Expect(hrReason).To(Equal(agentclient.HumanReviewReasonLowConfidence))
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

			params := agentclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetParams{
				SessionID: sessionID,
			}
			resp, err := handler.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGet(context.Background(), params)
			Expect(err).NotTo(HaveOccurred())

			incResp, ok := resp.(*agentclient.IncidentResponse)
			Expect(ok).To(BeTrue(), "expected type assertion to *IncidentResponse to succeed (legacy Reason field)")
			hrReason, hasReason := incResp.HumanReviewReason.Get()
			Expect(hasReason).To(BeTrue())
			Expect(hrReason).To(Equal(agentclient.HumanReviewReasonRcaIncomplete))
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

			params := agentclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetParams{
				SessionID: sessionID,
			}
			resp, err := handler.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGet(context.Background(), params)
			Expect(err).NotTo(HaveOccurred())

			incResp, ok := resp.(*agentclient.IncidentResponse)
			Expect(ok).To(BeTrue(), "expected type assertion to *IncidentResponse to succeed (execution_bundle)")

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
			req := &agentclient.IncidentRequest{
				IncidentID:        "h1-test",
				RemediationID:     "rem-uuid-12345",
				SignalName:        "OOMKilled",
				Severity:          agentclient.SeverityHigh,
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
			req := &agentclient.IncidentRequest{
				IncidentID:        "h1-time-test",
				SignalName:        "HighMemory",
				Severity:          agentclient.SeverityCritical,
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
			req := &agentclient.IncidentRequest{
				IncidentID:        "h1-dedup-test",
				SignalName:        "OOMKilled",
				Severity:          agentclient.SeverityHigh,
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

		It("UT-KA-462-001: should populate SignalAnnotations when present", func() {
			req := &agentclient.IncidentRequest{
				IncidentID:        "h1-annot-test",
				SignalName:        "OOMKilled",
				Severity:          agentclient.SeverityHigh,
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
			req.SignalAnnotations.SetTo(agentclient.IncidentRequestSignalAnnotations{
				"description": "Pod OOMKilled in production",
				"summary":     "Memory limit exceeded",
			})

			signal := server.MapIncidentRequestToSignal(req)
			Expect(signal.SignalAnnotations).To(Equal(map[string]string{
				"description": "Pod OOMKilled in production",
				"summary":     "Memory limit exceeded",
			}))
		})

		It("UT-KA-462-001b: should populate SignalLabels when present (pre-existing gap fix)", func() {
			req := &agentclient.IncidentRequest{
				IncidentID:        "h1-labels-test",
				SignalName:        "OOMKilled",
				Severity:          agentclient.SeverityHigh,
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
			req.SignalLabels.SetTo(agentclient.IncidentRequestSignalLabels{
				"alertname": "OOMKilled",
				"severity":  "critical",
			})

			signal := server.MapIncidentRequestToSignal(req)
			Expect(signal.SignalLabels).To(Equal(map[string]string{
				"alertname": "OOMKilled",
				"severity":  "critical",
			}))
		})
	})

	Describe("UT-KA-743: MapIncidentRequestToSignal dedup timing fields (#743)", func() {
		It("UT-KA-743-005: maps deduplication_window_minutes, first_seen, last_seen from request", func() {
			req := &agentclient.IncidentRequest{
				IncidentID:        "dedup-timing-test",
				SignalName:        "HighMemoryUsage",
				Severity:          agentclient.SeverityMedium,
				ResourceNamespace: "production",
				ResourceKind:      "Deployment",
				ResourceName:      "api-server",
				ErrorMessage:      "Memory above 90%",
				Environment:       "production",
				Priority:          "medium",
				RiskTolerance:     "medium",
				BusinessCategory:  "test",
				ClusterName:       "test-cluster",
				SignalSource:      "prometheus",
			}
			req.IsDuplicate.SetTo(true)
			req.OccurrenceCount.SetTo(3)
			req.DeduplicationWindowMinutes.SetTo(60)
			req.FirstSeen.SetTo("2026-04-01T10:00:00Z")
			req.LastSeen.SetTo("2026-04-01T11:00:00Z")

			signal := server.MapIncidentRequestToSignal(req)
			Expect(signal.DeduplicationWindowMinutes).NotTo(BeNil(),
				"DeduplicationWindowMinutes must be populated from request")
			Expect(*signal.DeduplicationWindowMinutes).To(Equal(60))
			Expect(signal.FirstSeen).To(Equal("2026-04-01T10:00:00Z"))
			Expect(signal.LastSeen).To(Equal("2026-04-01T11:00:00Z"))
		})

		It("UT-KA-743-006: leaves dedup timing fields empty when not set in request", func() {
			req := &agentclient.IncidentRequest{
				IncidentID:        "dedup-empty-test",
				SignalName:        "OOMKilled",
				Severity:          agentclient.SeverityHigh,
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
			Expect(signal.DeduplicationWindowMinutes).To(BeNil(),
				"DeduplicationWindowMinutes must be nil when not set")
			Expect(signal.FirstSeen).To(BeEmpty(),
				"FirstSeen must be empty when not set")
			Expect(signal.LastSeen).To(BeEmpty(),
				"LastSeen must be empty when not set")
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

			params := agentclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetParams{
				SessionID: sessionID,
			}
			resp, err := handler.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGet(context.Background(), params)
			Expect(err).NotTo(HaveOccurred())

			incResp, ok := resp.(*agentclient.IncidentResponse)
			Expect(ok).To(BeTrue(), "expected type assertion to *IncidentResponse to succeed (alternative_workflows mapping)")
			Expect(incResp.AlternativeWorkflows).To(HaveLen(2),
				"H2: alternative_workflows must be mapped to response")
		})
	})
})

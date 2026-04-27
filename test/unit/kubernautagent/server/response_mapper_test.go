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
	"fmt"
	"log/slog"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/agentclient"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/server"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
)

var _ = Describe("Response Mapper — #433", func() {

	var (
		store   *session.Store
		manager *session.Manager
		handler *server.Handler
		logger  *slog.Logger
	)

	BeforeEach(func() {
		store = session.NewStore(5 * time.Minute)
		logger = slog.Default()
		manager = session.NewManager(store, logger, nil)
		handler = server.NewHandler(manager, nil, logger)
	})

	Describe("UT-KA-433-MAPPER-001: IncidentID is populated from session metadata", func() {
		It("should set IncidentID in the response from session metadata", func() {
			metadata := map[string]string{"incident_id": "e2e-ka-001-oom"}
			id, err := manager.StartInvestigation(context.Background(), func(_ context.Context) (interface{}, error) {
				return &katypes.InvestigationResult{
					RCASummary: "OOMKilled due to memory limit",
					Confidence: 0.85,
				}, nil
			}, metadata)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				sess, _ := manager.GetSession(id)
				if sess == nil {
					return ""
				}
				return sess.Status
			}, 2*time.Second, 10*time.Millisecond).Should(Equal(session.StatusCompleted))

			params := agentclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetParams{
				SessionID: id,
			}
			resp, err := handler.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGet(context.Background(), params)
			Expect(err).NotTo(HaveOccurred())

			incidentResp, ok := resp.(*agentclient.IncidentResponse)
			Expect(ok).To(BeTrue(), "response should be *IncidentResponse")
			Expect(incidentResp.IncidentID).To(Equal("e2e-ka-001-oom"))
		})
	})

	Describe("UT-KA-433-MAPPER-002: Timestamp is set to a non-empty RFC3339 value", func() {
		It("should set a valid Timestamp on the response", func() {
			id, err := manager.StartInvestigation(context.Background(), func(_ context.Context) (interface{}, error) {
				return &katypes.InvestigationResult{
					RCASummary: "CrashLoopBackOff",
					Confidence: 0.70,
				}, nil
			}, nil)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				sess, _ := manager.GetSession(id)
				if sess == nil {
					return ""
				}
				return sess.Status
			}, 2*time.Second, 10*time.Millisecond).Should(Equal(session.StatusCompleted))

			params := agentclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetParams{
				SessionID: id,
			}
			resp, err := handler.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGet(context.Background(), params)
			Expect(err).NotTo(HaveOccurred())

			incidentResp, ok := resp.(*agentclient.IncidentResponse)
			Expect(ok).To(BeTrue(), "response should be *IncidentResponse")
			Expect(incidentResp.Timestamp).NotTo(BeEmpty(), "timestamp must be set")
			_, parseErr := time.Parse(time.RFC3339, incidentResp.Timestamp)
			Expect(parseErr).NotTo(HaveOccurred(), "timestamp should be valid RFC3339")
		})
	})

	Describe("UT-KA-433-MAPPER-003: RootCauseAnalysis is populated from RCASummary", func() {
		It("should set RootCauseAnalysis as a structured map", func() {
			id, err := manager.StartInvestigation(context.Background(), func(_ context.Context) (interface{}, error) {
				return &katypes.InvestigationResult{
					RCASummary: "Pod killed due to exceeding memory limits",
					Confidence: 0.90,
				}, nil
			}, nil)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				sess, _ := manager.GetSession(id)
				if sess == nil {
					return ""
				}
				return sess.Status
			}, 2*time.Second, 10*time.Millisecond).Should(Equal(session.StatusCompleted))

			params := agentclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetParams{
				SessionID: id,
			}
			resp, err := handler.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGet(context.Background(), params)
			Expect(err).NotTo(HaveOccurred())

			incidentResp, ok := resp.(*agentclient.IncidentResponse)
			Expect(ok).To(BeTrue(), "response should be *IncidentResponse")
			Expect(incidentResp.RootCauseAnalysis).NotTo(BeEmpty(), "root_cause_analysis must not be empty")
		})
	})

	Describe("UT-KA-PR9-MAPPER-004: Full InvestigationResult mapping exercises all optional branches", func() {
		It("should map all optional fields to the response when populated", func() {
			actionable := true
			result := &katypes.InvestigationResult{
				RCASummary:          "OOMKilled due to container memory limit exceeded",
				Severity:            "critical",
				SignalName:          "OOMKilled",
				ContributingFactors: []string{"memory_limit_too_low", "memory_leak_in_app"},
				CausalChain:         []string{"memory_leak", "oom_kill", "pod_restart"},
				WorkflowID:          "oom-recovery-v1",
				WorkflowVersion:     "1.2.0",
				WorkflowRationale:   "Best match for OOM recovery",
				ExecutionBundle:     "oci://registry/oom-recovery:v1",
				ExecutionBundleDigest: "sha256:abc123",
				ExecutionEngine:     "tekton",
				ServiceAccountName:  "remediation-sa",
				Confidence:          0.92,
				Parameters:          map[string]interface{}{"memory_increase_pct": 50},
				HumanReviewNeeded:   false,
				IsActionable:        &actionable,
				Warnings:            []string{"memory increase may affect pod scheduling"},
				DetectedLabels:      map[string]interface{}{"app": "api-server", "team": "platform"},
				RemediationTarget:   katypes.RemediationTarget{Kind: "Deployment", Name: "api-server", Namespace: "production"},
				DueDiligence:        &katypes.DueDiligenceReview{CausalCompleteness: "verified", TargetAccuracy: "high"},
				AlternativeWorkflows: []katypes.AlternativeWorkflow{
					{WorkflowID: "oom-aggressive-v1", Confidence: 0.78, Rationale: "Aggressive recovery", ExecutionBundle: "oci://registry/oom-aggressive:v1"},
					{WorkflowID: "oom-restart-v1", Confidence: 0.65, Rationale: "Simple restart"},
				},
				ValidationAttemptsHistory: []katypes.ValidationAttemptRecord{
					{Attempt: 1, WorkflowID: "oom-recovery-v1", IsValid: true, Timestamp: "2026-04-26T20:00:00Z"},
					{Attempt: 0, IsValid: false, Errors: []string{"missing param"}, Timestamp: "2026-04-26T19:55:00Z"},
				},
			}
			id, err := manager.StartInvestigation(context.Background(), func(_ context.Context) (interface{}, error) {
				return result, nil
			}, map[string]string{"incident_id": "inc-full-mapper"})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				sess, _ := manager.GetSession(id)
				if sess == nil {
					return ""
				}
				return sess.Status
			}, 2*time.Second, 10*time.Millisecond).Should(Equal(session.StatusCompleted))

			params := agentclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetParams{SessionID: id}
			resp, err := handler.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGet(context.Background(), params)
			Expect(err).NotTo(HaveOccurred())

			ir, ok := resp.(*agentclient.IncidentResponse)
			Expect(ok).To(BeTrue())
			Expect(ir.IncidentID).To(Equal("inc-full-mapper"))
			Expect(ir.Analysis).To(ContainSubstring("OOMKilled"))
			Expect(ir.Confidence).To(BeNumerically("~", 0.92, 0.01))

			Expect(ir.RootCauseAnalysis).To(HaveKey("summary"))
			Expect(ir.RootCauseAnalysis).To(HaveKey("severity"))
			Expect(ir.RootCauseAnalysis).To(HaveKey("signal_name"))
			Expect(ir.RootCauseAnalysis).To(HaveKey("contributing_factors"))
			Expect(ir.RootCauseAnalysis).To(HaveKey("causal_chain"))
			Expect(ir.RootCauseAnalysis).To(HaveKey("remediationTarget"))
			Expect(ir.RootCauseAnalysis).To(HaveKey("due_diligence"))

			sw, hasSW := ir.SelectedWorkflow.Get()
			Expect(hasSW).To(BeTrue(), "selected_workflow must be present")
			Expect(sw).To(HaveKey("workflow_id"))
			Expect(sw).To(HaveKey("parameters"))
			Expect(sw).To(HaveKey("confidence"))
			Expect(sw).To(HaveKey("execution_bundle"))
			Expect(sw).To(HaveKey("execution_bundle_digest"))
			Expect(sw).To(HaveKey("execution_engine"))
			Expect(sw).To(HaveKey("service_account_name"))
			Expect(sw).To(HaveKey("version"))
			Expect(sw).To(HaveKey("rationale"))

			Expect(ir.NeedsHumanReview.Value).To(BeFalse())
			isAct, hasAct := ir.IsActionable.Get()
			Expect(hasAct).To(BeTrue())
			Expect(isAct).To(BeTrue())

			dl, hasDL := ir.DetectedLabels.Get()
			Expect(hasDL).To(BeTrue())
			Expect(dl).To(HaveKey("app"))
			Expect(dl).To(HaveKey("team"))

			Expect(ir.AlternativeWorkflows).To(HaveLen(2))
			eb, hasEB := ir.AlternativeWorkflows[0].ExecutionBundle.Get()
			Expect(hasEB).To(BeTrue())
			Expect(eb).To(Equal("oci://registry/oom-aggressive:v1"))

			Expect(ir.ValidationAttemptsHistory).To(HaveLen(2))
			wfID, hasWF := ir.ValidationAttemptsHistory[0].WorkflowID.Get()
			Expect(hasWF).To(BeTrue())
			Expect(wfID).To(Equal("oom-recovery-v1"))
			Expect(ir.ValidationAttemptsHistory[1].Errors).To(ContainElement("missing param"))

			Expect(ir.Warnings).To(HaveLen(1))
		})
	})

	Describe("UT-KA-PR9-MAPPER-005: HumanReviewReason mapping covers all enum variants", func() {
		type hrEntry struct {
			reason   string
			hrReason string
			label    string
		}
		entries := []hrEntry{
			{"rca_incomplete", "rca_incomplete", "exact match"},
			{"investigation_inconclusive", "investigation_inconclusive", "exact match"},
			{"workflow_not_found", "workflow_not_found", "exact match"},
			{"no_matching_workflows", "no_matching_workflows", "exact match"},
			{"image_mismatch", "image_mismatch", "exact match"},
			{"parameter_validation_failed", "parameter_validation_failed", "exact match"},
			{"low_confidence", "low_confidence", "exact match"},
			{"llm_parsing_error", "llm_parsing_error", "exact match"},
			{"alignment_check_failed", "investigation_inconclusive", "alignment maps to inconclusive"},
			{"turns exhausted during RCA phase", "rca_incomplete", "contains 'exhausted during RCA'"},
			{"turns exhausted during workflow selection", "investigation_inconclusive", "contains 'exhausted during workflow selection'"},
			{"workflow not found in catalog", "workflow_not_found", "contains 'not found' + 'catalog'"},
			{"no matching remediation", "no_matching_workflows", "contains 'no matching'"},
			{"container image mismatch", "image_mismatch", "contains 'mismatch' or 'image'"},
			{"parameter injection validation failure", "parameter_validation_failed", "contains 'parameter' or 'validation'"},
			{"very low confidence score", "low_confidence", "contains 'confidence'"},
			{"failed to parse LLM output", "llm_parsing_error", "contains 'parse' or 'parsing'"},
			{"unknown_reason_xyz", "investigation_inconclusive", "default fallback"},
		}

		for _, e := range entries {
			e := e
			It("maps '"+e.reason+"' correctly ("+e.label+")", func() {
				result := &katypes.InvestigationResult{
					RCASummary:        "test",
					Confidence:        0.5,
					HumanReviewNeeded: true,
					HumanReviewReason: e.reason,
				}
				id, err := manager.StartInvestigation(context.Background(), func(_ context.Context) (interface{}, error) {
					return result, nil
				}, map[string]string{"incident_id": "hr-" + e.reason})
				Expect(err).NotTo(HaveOccurred())

				Eventually(func() session.Status {
					sess, _ := manager.GetSession(id)
					if sess == nil {
						return ""
					}
					return sess.Status
				}, 2*time.Second, 10*time.Millisecond).Should(Equal(session.StatusCompleted))

				params := agentclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetParams{SessionID: id}
				resp, err := handler.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGet(context.Background(), params)
				Expect(err).NotTo(HaveOccurred())

				ir, ok := resp.(*agentclient.IncidentResponse)
				Expect(ok).To(BeTrue())
				Expect(ir.NeedsHumanReview.Value).To(BeTrue())
				hrReason, hasReason := ir.HumanReviewReason.Get()
				Expect(hasReason).To(BeTrue())
				Expect(string(hrReason)).To(Equal(e.hrReason),
					"HumanReviewReason for '%s' should be '%s'", e.reason, e.hrReason)
			})
		}
	})

	Describe("UT-KA-PR9-MAPPER-006: HumanReview with no explicit warnings synthesizes a warning", func() {
		It("should generate a synthetic warning when HumanReviewNeeded=true and Warnings is empty", func() {
			result := &katypes.InvestigationResult{
				RCASummary:        "Inconclusive analysis",
				Confidence:        0.3,
				HumanReviewNeeded: true,
				HumanReviewReason: "low_confidence",
			}
			id, err := manager.StartInvestigation(context.Background(), func(_ context.Context) (interface{}, error) {
				return result, nil
			}, map[string]string{"incident_id": "hr-synth-warn"})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				sess, _ := manager.GetSession(id)
				if sess == nil {
					return ""
				}
				return sess.Status
			}, 2*time.Second, 10*time.Millisecond).Should(Equal(session.StatusCompleted))

			params := agentclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetParams{SessionID: id}
			resp, err := handler.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGet(context.Background(), params)
			Expect(err).NotTo(HaveOccurred())

			ir, ok := resp.(*agentclient.IncidentResponse)
			Expect(ok).To(BeTrue())
			Expect(ir.Warnings).To(HaveLen(1))
			Expect(ir.Warnings[0]).To(ContainSubstring("Human review required"))
		})
	})

	Describe("UT-KA-PR9-MAPPER-008: Result endpoint returns 409 when result is not InvestigationResult type", func() {
		It("should return 409 conflict when session result is a non-InvestigationResult type", func() {
			id, err := manager.StartInvestigation(context.Background(), func(_ context.Context) (interface{}, error) {
				return "raw string result", nil
			}, nil)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				sess, _ := manager.GetSession(id)
				if sess == nil {
					return ""
				}
				return sess.Status
			}, 2*time.Second, 10*time.Millisecond).Should(Equal(session.StatusCompleted))

			params := agentclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetParams{SessionID: id}
			resp, err := handler.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGet(context.Background(), params)
			Expect(err).NotTo(HaveOccurred())

			_, ok := resp.(*agentclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetConflict)
			Expect(ok).To(BeTrue(), "non-InvestigationResult session should return 409")
		})
	})

	Describe("UT-KA-PR9-MAPPER-009: HumanReview with Reason but no HumanReviewReason falls back to Reason", func() {
		It("should use legacy Reason field when HumanReviewReason is empty", func() {
			result := &katypes.InvestigationResult{
				RCASummary:        "test",
				Confidence:        0.4,
				HumanReviewNeeded: true,
				Reason:            "low_confidence",
			}
			id, err := manager.StartInvestigation(context.Background(), func(_ context.Context) (interface{}, error) {
				return result, nil
			}, map[string]string{"incident_id": "hr-legacy-reason"})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				sess, _ := manager.GetSession(id)
				if sess == nil {
					return ""
				}
				return sess.Status
			}, 2*time.Second, 10*time.Millisecond).Should(Equal(session.StatusCompleted))

			params := agentclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetParams{SessionID: id}
			resp, err := handler.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGet(context.Background(), params)
			Expect(err).NotTo(HaveOccurred())

			ir, ok := resp.(*agentclient.IncidentResponse)
			Expect(ok).To(BeTrue())
			hrReason, hasReason := ir.HumanReviewReason.Get()
			Expect(hasReason).To(BeTrue())
			Expect(string(hrReason)).To(Equal("low_confidence"))
			Expect(ir.Warnings).To(HaveLen(1))
			Expect(ir.Warnings[0]).To(ContainSubstring("low_confidence"))
		})
	})

	Describe("UT-KA-PR9-MAPPER-010: HumanReview with empty reason generates generic warning", func() {
		It("should synthesize a generic warning when both Reason and HumanReviewReason are empty", func() {
			result := &katypes.InvestigationResult{
				RCASummary:        "test",
				Confidence:        0.2,
				HumanReviewNeeded: true,
			}
			id, err := manager.StartInvestigation(context.Background(), func(_ context.Context) (interface{}, error) {
				return result, nil
			}, map[string]string{"incident_id": "hr-no-reason"})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				sess, _ := manager.GetSession(id)
				if sess == nil {
					return ""
				}
				return sess.Status
			}, 2*time.Second, 10*time.Millisecond).Should(Equal(session.StatusCompleted))

			params := agentclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetParams{SessionID: id}
			resp, err := handler.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGet(context.Background(), params)
			Expect(err).NotTo(HaveOccurred())

			ir, ok := resp.(*agentclient.IncidentResponse)
			Expect(ok).To(BeTrue())
			Expect(ir.Warnings).To(HaveLen(1))
			Expect(ir.Warnings[0]).To(ContainSubstring("could not determine automated remediation"))
		})
	})

	Describe("UT-KA-PR9-MAPPER-007: Status mapping covers all session statuses via status endpoint", func() {
		It("should map 'completed' status correctly", func() {
			id, err := manager.StartInvestigation(context.Background(), func(_ context.Context) (interface{}, error) {
				return &katypes.InvestigationResult{RCASummary: "done"}, nil
			}, nil)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				sess, _ := manager.GetSession(id)
				return sess.Status
			}, 2*time.Second, 10*time.Millisecond).Should(Equal(session.StatusCompleted))

			params := agentclient.IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGetParams{SessionID: id}
			resp, err := handler.IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGet(context.Background(), params)
			Expect(err).NotTo(HaveOccurred())

			raw, ok := resp.(*agentclient.IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGetOKApplicationJSON)
			Expect(ok).To(BeTrue())
			var body map[string]string
			Expect(json.Unmarshal([]byte(*raw), &body)).To(Succeed())
			Expect(body["status"]).To(Equal("completed"))
		})

		It("should map 'investigating' for running session", func() {
			id, err := manager.StartInvestigation(context.Background(), func(bgCtx context.Context) (interface{}, error) {
				<-bgCtx.Done()
				return nil, bgCtx.Err()
			}, nil)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				sess, _ := manager.GetSession(id)
				return sess.Status
			}, 2*time.Second, 10*time.Millisecond).Should(Equal(session.StatusRunning))

			params := agentclient.IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGetParams{SessionID: id}
			resp, err := handler.IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGet(context.Background(), params)
			Expect(err).NotTo(HaveOccurred())

			raw, ok := resp.(*agentclient.IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGetOKApplicationJSON)
			Expect(ok).To(BeTrue())
			var body map[string]string
			Expect(json.Unmarshal([]byte(*raw), &body)).To(Succeed())
			Expect(body["status"]).To(Equal("investigating"))
		})

		It("should map 'failed' status correctly", func() {
			id, err := manager.StartInvestigation(context.Background(), func(_ context.Context) (interface{}, error) {
				return nil, fmt.Errorf("LLM provider unavailable")
			}, nil)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				sess, _ := manager.GetSession(id)
				return sess.Status
			}, 2*time.Second, 10*time.Millisecond).Should(Equal(session.StatusFailed))

			params := agentclient.IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGetParams{SessionID: id}
			resp, err := handler.IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGet(context.Background(), params)
			Expect(err).NotTo(HaveOccurred())

			raw, ok := resp.(*agentclient.IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGetOKApplicationJSON)
			Expect(ok).To(BeTrue())
			var body map[string]string
			Expect(json.Unmarshal([]byte(*raw), &body)).To(Succeed())
			Expect(body["status"]).To(Equal("failed"))
		})
	})
})

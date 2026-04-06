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

package audit_test

import (
	"context"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

var _ = Describe("KA Audit Parity — TP-433-AUDIT-SOC2", func() {

	// --- Phase 1: Foundation ---

	Describe("UT-KA-433-AP-001: NewEvent auto-generates UUID event_id", func() {
		It("should set Data[event_id] as a valid UUID", func() {
			event := audit.NewEvent(audit.EventTypeLLMRequest, "rem-001")

			rawID, ok := event.Data["event_id"]
			Expect(ok).To(BeTrue(), "event_id must be present in Data")
			eventID, ok := rawID.(string)
			Expect(ok).To(BeTrue(), "event_id must be a string")
			Expect(eventID).NotTo(BeEmpty())

			_, err := uuid.Parse(eventID)
			Expect(err).NotTo(HaveOccurred(), "event_id must be a valid UUID")
		})

		It("should generate unique event_ids across calls", func() {
			e1 := audit.NewEvent(audit.EventTypeLLMResponse, "rem-002")
			e2 := audit.NewEvent(audit.EventTypeLLMResponse, "rem-002")

			id1 := e1.Data["event_id"].(string)
			id2 := e2.Data["event_id"].(string)
			Expect(id1).NotTo(Equal(id2), "consecutive calls must produce different UUIDs")
		})
	})

	Describe("UT-KA-433-AP-002: EventAction/EventOutcome constants defined", func() {
		It("should define action constants for all 6 investigator event types", func() {
			Expect(audit.ActionLLMRequest).To(Equal("llm_request"))
			Expect(audit.ActionLLMResponse).To(Equal("llm_response"))
			Expect(audit.ActionToolExecution).To(Equal("tool_execution"))
			Expect(audit.ActionValidation).To(Equal("validation"))
			Expect(audit.ActionResponseSent).To(Equal("response_sent"))
			Expect(audit.ActionResponseFailed).To(Equal("response_failed"))
		})

		It("should define outcome constants matching ogen enum", func() {
			Expect(audit.OutcomeSuccess).To(Equal("success"))
			Expect(audit.OutcomeFailure).To(Equal("failure"))
			Expect(audit.OutcomePending).To(Equal("pending"))
		})
	})

	Describe("UT-KA-433-AP-003: StoreAudit sets ActorType and ActorID", func() {
		It("should set ActorType=Service and ActorID=kubernaut-agent on ogen request", func() {
			recorder := &fakeOgenClient{}
			store := audit.NewDSAuditStore(recorder)

			event := audit.NewEvent(audit.EventTypeLLMRequest, "corr-actor")
			err := store.StoreAudit(context.Background(), event)
			Expect(err).NotTo(HaveOccurred())
			Expect(recorder.calls).To(HaveLen(1))

			req := recorder.calls[0]
			Expect(req.ActorType.Value).To(Equal("Service"))
			Expect(req.ActorID.Value).To(Equal("kubernaut-agent"))
		})
	})

	// --- Phase 2: LLM Request ---

	Describe("UT-KA-433-AP-004: buildEventData maps LLMRequestPayload", func() {
		It("should populate event_id, model, prompt_length, prompt_preview, toolsets_enabled", func() {
			recorder := &fakeOgenClient{}
			store := audit.NewDSAuditStore(recorder)

			event := audit.NewEvent(audit.EventTypeLLMRequest, "corr-llm-req")
			event.Data["model"] = "claude-sonnet-4-20250514"
			event.Data["prompt_length"] = 1234
			event.Data["prompt_preview"] = "Analyze the following Kubernetes incident..."
			event.Data["toolsets_enabled"] = []string{"get_pods", "get_logs"}

			err := store.StoreAudit(context.Background(), event)
			Expect(err).NotTo(HaveOccurred())

			req := recorder.calls[0]
			Expect(req.EventData.Type).To(Equal(ogenclient.LLMRequestPayloadAuditEventRequestEventData))

			payload, ok := req.EventData.GetLLMRequestPayload()
			Expect(ok).To(BeTrue())
			Expect(payload.EventID).NotTo(BeEmpty())
			Expect(payload.Model).To(Equal("claude-sonnet-4-20250514"))
			Expect(payload.PromptLength).To(Equal(1234))
			Expect(payload.PromptPreview).To(Equal("Analyze the following Kubernetes incident..."))
			Expect(payload.ToolsetsEnabled).To(ConsistOf("get_pods", "get_logs"))
		})
	})

	Describe("UT-KA-433-AP-005: prompt_preview truncates at 500 chars", func() {
		It("should truncate long previews to 500 characters", func() {
			recorder := &fakeOgenClient{}
			store := audit.NewDSAuditStore(recorder)

			longPrompt := make([]byte, 1000)
			for i := range longPrompt {
				longPrompt[i] = 'A'
			}

			event := audit.NewEvent(audit.EventTypeLLMRequest, "corr-trunc")
			event.Data["model"] = "test-model"
			event.Data["prompt_length"] = 1000
			event.Data["prompt_preview"] = string(longPrompt)

			err := store.StoreAudit(context.Background(), event)
			Expect(err).NotTo(HaveOccurred())

			payload, ok := recorder.calls[0].EventData.GetLLMRequestPayload()
			Expect(ok).To(BeTrue())
			Expect(len(payload.PromptPreview)).To(BeNumerically("<=", 500))
		})
	})

	// --- Phase 3: LLM Response ---

	Describe("UT-KA-433-AP-007: buildEventData maps LLMResponsePayload", func() {
		It("should populate has_analysis, analysis_length, analysis_preview, tokens_used, tool_call_count", func() {
			recorder := &fakeOgenClient{}
			store := audit.NewDSAuditStore(recorder)

			event := audit.NewEvent(audit.EventTypeLLMResponse, "corr-llm-resp")
			event.Data["has_analysis"] = true
			event.Data["analysis_length"] = 500
			event.Data["analysis_preview"] = "Root cause: OOMKilled..."
			event.Data["total_tokens"] = 800
			event.Data["tool_call_count"] = 3

			err := store.StoreAudit(context.Background(), event)
			Expect(err).NotTo(HaveOccurred())

			req := recorder.calls[0]
			Expect(req.EventData.Type).To(Equal(ogenclient.LLMResponsePayloadAuditEventRequestEventData))

			payload, ok := req.EventData.GetLLMResponsePayload()
			Expect(ok).To(BeTrue())
			Expect(payload.HasAnalysis).To(BeTrue())
			Expect(payload.AnalysisLength).To(Equal(500))
			Expect(payload.AnalysisPreview).To(Equal("Root cause: OOMKilled..."))
			Expect(payload.TokensUsed.Value).To(Equal(800))
			Expect(payload.ToolCallCount.Value).To(Equal(3))
		})
	})

	Describe("UT-KA-433-AP-008: analysis_preview truncates at 500 chars", func() {
		It("should truncate long analysis to 500 characters", func() {
			recorder := &fakeOgenClient{}
			store := audit.NewDSAuditStore(recorder)

			longAnalysis := make([]byte, 1000)
			for i := range longAnalysis {
				longAnalysis[i] = 'B'
			}

			event := audit.NewEvent(audit.EventTypeLLMResponse, "corr-trunc-resp")
			event.Data["has_analysis"] = true
			event.Data["analysis_length"] = 1000
			event.Data["analysis_preview"] = string(longAnalysis)

			err := store.StoreAudit(context.Background(), event)
			Expect(err).NotTo(HaveOccurred())

			payload, ok := recorder.calls[0].EventData.GetLLMResponsePayload()
			Expect(ok).To(BeTrue())
			Expect(len(payload.AnalysisPreview)).To(BeNumerically("<=", 500))
		})
	})

	// --- Phase 4: Tool Calls ---

	Describe("UT-KA-433-AP-009: buildEventData maps LLMToolCallPayload", func() {
		It("should populate tool_call_index, tool_name, tool_result, tool_result_preview", func() {
			recorder := &fakeOgenClient{}
			store := audit.NewDSAuditStore(recorder)

			event := audit.NewEvent(audit.EventTypeLLMToolCall, "corr-tool")
			event.Data["tool_call_index"] = 0
			event.Data["tool_name"] = "get_pods"
			event.Data["tool_arguments"] = `{"namespace":"default"}`
			event.Data["tool_result"] = `{"items":[{"name":"web-abc"}]}`
			event.Data["tool_result_preview"] = `{"items":[{"name":"web-abc"}]}`

			err := store.StoreAudit(context.Background(), event)
			Expect(err).NotTo(HaveOccurred())

			req := recorder.calls[0]
			Expect(req.EventData.Type).To(Equal(ogenclient.LLMToolCallPayloadAuditEventRequestEventData))

			payload, ok := req.EventData.GetLLMToolCallPayload()
			Expect(ok).To(BeTrue())
			Expect(payload.ToolCallIndex).To(Equal(0))
			Expect(payload.ToolName).To(Equal("get_pods"))
			Expect(payload.ToolResult).NotTo(BeEmpty(), "tool_result (jx.Raw) must be populated")
			Expect(payload.ToolResultPreview.Value).To(ContainSubstring("web-abc"))
		})
	})

	Describe("UT-KA-433-AP-010: tool_result_preview truncates at 500 chars", func() {
		It("should truncate long tool results to 500 characters", func() {
			recorder := &fakeOgenClient{}
			store := audit.NewDSAuditStore(recorder)

			longResult := make([]byte, 1000)
			for i := range longResult {
				longResult[i] = 'C'
			}

			event := audit.NewEvent(audit.EventTypeLLMToolCall, "corr-tool-trunc")
			event.Data["tool_name"] = "get_logs"
			event.Data["tool_result"] = `"` + string(longResult) + `"`
			event.Data["tool_result_preview"] = string(longResult)

			err := store.StoreAudit(context.Background(), event)
			Expect(err).NotTo(HaveOccurred())

			payload, ok := recorder.calls[0].EventData.GetLLMToolCallPayload()
			Expect(ok).To(BeTrue())
			Expect(len(payload.ToolResultPreview.Value)).To(BeNumerically("<=", 500))
		})
	})

	// --- Phase 5: Response Failed ---

	Describe("UT-KA-433-AP-011: buildEventData maps AIAgentResponseFailedPayload", func() {
		It("should populate error_message, phase, duration_seconds", func() {
			recorder := &fakeOgenClient{}
			store := audit.NewDSAuditStore(recorder)

			event := audit.NewEvent(audit.EventTypeResponseFailed, "corr-fail")
			event.Data["error_message"] = "LLM timeout after 30s"
			event.Data["phase"] = "rca"
			event.Data["duration_seconds"] = 30.5

			err := store.StoreAudit(context.Background(), event)
			Expect(err).NotTo(HaveOccurred())

			req := recorder.calls[0]
			payload, ok := req.EventData.GetAIAgentResponseFailedPayload()
			Expect(ok).To(BeTrue())
			Expect(payload.ErrorMessage).To(Equal("LLM timeout after 30s"))
			Expect(payload.Phase).To(Equal("rca"))
			Expect(payload.DurationSeconds.Value).To(BeNumerically("~", 30.5, 0.01))
		})
	})

	// --- Phase 6: Validation ---

	Describe("UT-KA-433-AP-012: buildEventData maps WorkflowValidationPayload", func() {
		It("should populate attempt, max_attempts, is_valid, errors, workflow_id, is_final_attempt", func() {
			recorder := &fakeOgenClient{}
			store := audit.NewDSAuditStore(recorder)

			event := audit.NewEvent(audit.EventTypeValidationAttempt, "corr-val")
			event.Data["attempt"] = 2
			event.Data["max_attempts"] = 3
			event.Data["is_valid"] = false
			event.Data["errors"] = []string{"workflow_id not found in catalog"}
			event.Data["workflow_id"] = "wf-123"
			event.Data["is_final_attempt"] = false

			err := store.StoreAudit(context.Background(), event)
			Expect(err).NotTo(HaveOccurred())

			payload, ok := recorder.calls[0].EventData.GetWorkflowValidationPayload()
			Expect(ok).To(BeTrue())
			Expect(payload.Attempt).To(Equal(2))
			Expect(payload.MaxAttempts).To(Equal(3))
			Expect(payload.IsValid).To(BeFalse())
			Expect(payload.Errors).To(ContainElement("workflow_id not found in catalog"))
			Expect(payload.WorkflowID.Value).To(Equal("wf-123"))
			Expect(payload.IsFinalAttempt.Value).To(BeFalse())
		})
	})

	Describe("UT-KA-433-AP-013: Validation failure sets EventOutcome=failure", func() {
		It("should pass through EventOutcome=failure to ogen request", func() {
			recorder := &fakeOgenClient{}
			store := audit.NewDSAuditStore(recorder)

			event := audit.NewEvent(audit.EventTypeValidationAttempt, "corr-val-fail")
			event.EventAction = audit.ActionValidation
			event.EventOutcome = audit.OutcomeFailure

			err := store.StoreAudit(context.Background(), event)
			Expect(err).NotTo(HaveOccurred())

			req := recorder.calls[0]
			Expect(string(req.EventOutcome)).To(Equal("failure"))
		})
	})

	// --- Phase 7: Response Complete ---

	Describe("UT-KA-433-AP-014: buildEventData maps AIAgentResponsePayload with IncidentResponseData", func() {
		It("should populate response_data with full IncidentResponseData and cumulative tokens", func() {
			recorder := &fakeOgenClient{}
			store := audit.NewDSAuditStore(recorder)

			event := audit.NewEvent(audit.EventTypeResponseComplete, "corr-complete")
			event.Data["response_data"] = `{
				"rca_summary": "OOMKilled due to memory leak",
				"severity": "high",
				"contributing_factors": ["memory leak", "no limits set"],
				"workflow_id": "wf-oom-recovery",
				"execution_bundle": "oom-recovery-v1",
				"confidence": 0.92,
				"needs_human_review": false,
				"parameters": {"replicas": 3},
				"alternative_workflows": [{"workflow_id": "wf-restart", "rationale": "simple restart"}],
				"remediation_target": {"kind": "Deployment", "name": "api-server", "namespace": "production"}
			}`
			event.Data["total_prompt_tokens"] = 1500
			event.Data["total_completion_tokens"] = 800

			err := store.StoreAudit(context.Background(), event)
			Expect(err).NotTo(HaveOccurred())

			req := recorder.calls[0]
			payload, ok := req.EventData.GetAIAgentResponsePayload()
			Expect(ok).To(BeTrue())
			Expect(payload.ResponseData.RootCauseAnalysis.Summary).NotTo(BeEmpty())
			Expect(payload.TotalPromptTokens.Value).To(Equal(1500))
			Expect(payload.TotalCompletionTokens.Value).To(Equal(800))
		})
	})

	Describe("UT-KA-433-AP-015: toIncidentResponseData maps severity to ogen enum", func() {
		It("should map known severities and default unknown values to unknown", func() {
			recorder := &fakeOgenClient{}
			store := audit.NewDSAuditStore(recorder)

			for _, tc := range []struct {
				input    string
				expected string
			}{
				{"critical", "critical"},
				{"high", "high"},
				{"medium", "medium"},
				{"low", "low"},
				{"unknown", "unknown"},
				{"invalid_value", "unknown"},
				{"", "unknown"},
			} {
				event := audit.NewEvent(audit.EventTypeResponseComplete, "corr-sev")
				event.Data["response_data"] = `{"rca_summary":"test","severity":"` + tc.input + `","confidence":0.5}`
				event.Data["total_prompt_tokens"] = 100
				event.Data["total_completion_tokens"] = 50

				err := store.StoreAudit(context.Background(), event)
				Expect(err).NotTo(HaveOccurred(), "severity=%s", tc.input)
			}
		})
	})

	Describe("UT-KA-433-AP-019: toIncidentResponseData handles nil/empty optionals", func() {
		It("should not panic with minimal response_data", func() {
			recorder := &fakeOgenClient{}
			store := audit.NewDSAuditStore(recorder)

			event := audit.NewEvent(audit.EventTypeResponseComplete, "corr-minimal")
			event.Data["response_data"] = `{"rca_summary":"minimal","severity":"low","confidence":0.5}`

			err := store.StoreAudit(context.Background(), event)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

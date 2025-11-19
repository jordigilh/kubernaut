// Copyright 2025 Jordi Gil.
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/audit"
)

// ========================================
// TDD RED PHASE: AI Analysis Event Builder Tests
// BR-STORAGE-033: Event Data Helpers
// ========================================
//
// These tests define the contract for the AI Analysis event builder.
// AI Analysis Service uses this builder to create audit events for:
// - LLM analysis lifecycle
// - Token usage tracking
// - Root cause analysis (RCA) results
// - Workflow selection
// - Tool invocations (MCP)
//
// Business Requirements:
// - BR-STORAGE-033-007: AI Analysis-specific event data structure
// - BR-STORAGE-033-008: LLM metrics tracking (provider, model, tokens)
// - BR-STORAGE-033-009: RCA and workflow selection metadata
//
// ========================================

var _ = Describe("AIAnalysisEventBuilder", func() {
	Context("BR-STORAGE-033-007: AI Analysis-specific event data structure", func() {
		It("should create AI analysis event with base structure", func() {
			builder := audit.NewAIAnalysisEvent("analysis.started")
			Expect(builder).ToNot(BeNil())

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())
			Expect(eventData).To(HaveKey("version"))
			Expect(eventData["version"]).To(Equal("1.0"))
			Expect(eventData).To(HaveKey("service"))
			Expect(eventData["service"]).To(Equal("aianalysis"))
			Expect(eventData).To(HaveKey("event_type"))
			Expect(eventData["event_type"]).To(Equal("analysis.started"))
		})

		It("should include AI analysis data in nested structure", func() {
			builder := audit.NewAIAnalysisEvent("analysis.completed").
				WithAnalysisID("analysis-2025-001").
				WithLLM("anthropic", "claude-haiku-4-5")

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())
			Expect(eventData).To(HaveKey("data"))

			data, ok := eventData["data"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(data).To(HaveKey("ai_analysis"))

			aiData, ok := data["ai_analysis"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(aiData).To(HaveKeyWithValue("analysis_id", "analysis-2025-001"))
			Expect(aiData).To(HaveKeyWithValue("llm_provider", "anthropic"))
			Expect(aiData).To(HaveKeyWithValue("llm_model", "claude-haiku-4-5"))
		})
	})

	Context("BR-STORAGE-033-008: LLM metrics tracking", func() {
		It("should track LLM provider and model", func() {
			builder := audit.NewAIAnalysisEvent("analysis.completed").
				WithAnalysisID("test-001").
				WithLLM("openai", "gpt-4-turbo")

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			data, _ := eventData["data"].(map[string]interface{})
			aiData, _ := data["ai_analysis"].(map[string]interface{})

			Expect(aiData).To(HaveKeyWithValue("llm_provider", "openai"))
			Expect(aiData).To(HaveKeyWithValue("llm_model", "gpt-4-turbo"))
		})

		It("should track token usage", func() {
			builder := audit.NewAIAnalysisEvent("analysis.completed").
				WithAnalysisID("test-001").
				WithLLM("anthropic", "claude-haiku").
				WithTokenUsage(1500, 500)

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			data, _ := eventData["data"].(map[string]interface{})
			aiData, _ := data["ai_analysis"].(map[string]interface{})

			Expect(aiData).To(HaveKeyWithValue("prompt_tokens", float64(1500)))
			Expect(aiData).To(HaveKeyWithValue("completion_tokens", float64(500)))
			Expect(aiData).To(HaveKeyWithValue("total_tokens", float64(2000)))
		})

		It("should track analysis duration", func() {
			builder := audit.NewAIAnalysisEvent("analysis.completed").
				WithAnalysisID("test-001").
				WithLLM("anthropic", "claude-haiku").
				WithDuration(3500) // 3.5 seconds

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			data, _ := eventData["data"].(map[string]interface{})
			aiData, _ := data["ai_analysis"].(map[string]interface{})

			Expect(aiData).To(HaveKeyWithValue("duration_ms", float64(3500)))
		})
	})

	Context("BR-STORAGE-033-009: RCA and workflow selection metadata", func() {
		It("should track RCA results", func() {
			builder := audit.NewAIAnalysisEvent("analysis.completed").
				WithAnalysisID("test-001").
				WithLLM("anthropic", "claude-haiku").
				WithRCA("OOMKilled", "critical", 0.95)

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			data, _ := eventData["data"].(map[string]interface{})
			aiData, _ := data["ai_analysis"].(map[string]interface{})

			Expect(aiData).To(HaveKeyWithValue("rca_signal_type", "OOMKilled"))
			Expect(aiData).To(HaveKeyWithValue("rca_severity", "critical"))
			Expect(aiData).To(HaveKeyWithValue("confidence", float64(0.95)))
		})

		It("should track workflow selection", func() {
			builder := audit.NewAIAnalysisEvent("analysis.completed").
				WithAnalysisID("test-001").
				WithLLM("anthropic", "claude-haiku").
				WithWorkflow("workflow-pod-restart-001")

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			data, _ := eventData["data"].(map[string]interface{})
			aiData, _ := data["ai_analysis"].(map[string]interface{})

			Expect(aiData).To(HaveKeyWithValue("workflow_id", "workflow-pod-restart-001"))
		})

		It("should track MCP tools invoked", func() {
			tools := []string{
				"kubernetes/describe_pod",
				"kubernetes/get_logs",
				"workflow/search_catalog",
			}

			builder := audit.NewAIAnalysisEvent("analysis.completed").
				WithAnalysisID("test-001").
				WithLLM("anthropic", "claude-haiku").
				WithToolsInvoked(tools)

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			data, _ := eventData["data"].(map[string]interface{})
			aiData, _ := data["ai_analysis"].(map[string]interface{})

			toolsInvoked, ok := aiData["tools_invoked"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(toolsInvoked).To(HaveLen(3))
			Expect(toolsInvoked[0]).To(Equal("kubernetes/describe_pod"))
			Expect(toolsInvoked[1]).To(Equal("kubernetes/get_logs"))
			Expect(toolsInvoked[2]).To(Equal("workflow/search_catalog"))
		})
	})

	Context("Complete AI analysis lifecycle", func() {
		It("should build complete successful analysis event", func() {
			builder := audit.NewAIAnalysisEvent("analysis.completed").
				WithAnalysisID("analysis-2025-11-18-001").
				WithLLM("anthropic", "claude-haiku-4-5-20251001").
				WithTokenUsage(2500, 750).
				WithDuration(4200).
				WithRCA("OOMKilled", "critical", 0.92).
				WithWorkflow("workflow-increase-memory-limits").
				WithToolsInvoked([]string{
					"kubernetes/describe_pod",
					"kubernetes/get_logs",
					"workflow/search_catalog",
				})

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			data, _ := eventData["data"].(map[string]interface{})
			aiData, _ := data["ai_analysis"].(map[string]interface{})

			// Verify all fields present
			Expect(aiData).To(HaveKeyWithValue("analysis_id", "analysis-2025-11-18-001"))
			Expect(aiData).To(HaveKeyWithValue("llm_provider", "anthropic"))
			Expect(aiData).To(HaveKeyWithValue("llm_model", "claude-haiku-4-5-20251001"))
			Expect(aiData).To(HaveKeyWithValue("prompt_tokens", float64(2500)))
			Expect(aiData).To(HaveKeyWithValue("completion_tokens", float64(750)))
			Expect(aiData).To(HaveKeyWithValue("total_tokens", float64(3250)))
			Expect(aiData).To(HaveKeyWithValue("duration_ms", float64(4200)))
			Expect(aiData).To(HaveKeyWithValue("rca_signal_type", "OOMKilled"))
			Expect(aiData).To(HaveKeyWithValue("rca_severity", "critical"))
			Expect(aiData).To(HaveKeyWithValue("confidence", float64(0.92)))
			Expect(aiData).To(HaveKeyWithValue("workflow_id", "workflow-increase-memory-limits"))
			Expect(aiData).To(HaveKey("tools_invoked"))
		})

		It("should build failed analysis event with error code", func() {
			builder := audit.NewAIAnalysisEvent("analysis.failed").
				WithAnalysisID("analysis-2025-11-18-002").
				WithLLM("anthropic", "claude-haiku").
				WithTokenUsage(100, 0).
				WithDuration(500).
				WithErrorCode("LLM_TIMEOUT")

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			data, _ := eventData["data"].(map[string]interface{})
			aiData, _ := data["ai_analysis"].(map[string]interface{})

			Expect(aiData).To(HaveKeyWithValue("error_code", "LLM_TIMEOUT"))
		})
	})

	Context("Edge Cases", func() {
		It("should handle minimal AI analysis event (only analysis ID)", func() {
			builder := audit.NewAIAnalysisEvent("analysis.started").
				WithAnalysisID("test-minimal")

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())
			Expect(eventData).ToNot(BeEmpty())
		})

		It("should handle zero token usage", func() {
			builder := audit.NewAIAnalysisEvent("analysis.completed").
				WithAnalysisID("test-001").
				WithTokenUsage(0, 0)

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			data, _ := eventData["data"].(map[string]interface{})
			aiData, _ := data["ai_analysis"].(map[string]interface{})

			Expect(aiData).To(HaveKeyWithValue("prompt_tokens", float64(0)))
			Expect(aiData).To(HaveKeyWithValue("completion_tokens", float64(0)))
			Expect(aiData).To(HaveKeyWithValue("total_tokens", float64(0)))
		})

		It("should handle empty tools list", func() {
			builder := audit.NewAIAnalysisEvent("analysis.completed").
				WithAnalysisID("test-001").
				WithToolsInvoked([]string{})

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			data, _ := eventData["data"].(map[string]interface{})
			aiData, _ := data["ai_analysis"].(map[string]interface{})

			toolsInvoked, ok := aiData["tools_invoked"].([]interface{})
			if ok {
				Expect(toolsInvoked).To(BeEmpty())
			}
		})

		It("should handle confidence score edge values", func() {
			builder := audit.NewAIAnalysisEvent("analysis.completed").
				WithAnalysisID("test-001").
				WithRCA("Unknown", "low", 0.01)

			eventData, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			data, _ := eventData["data"].(map[string]interface{})
			aiData, _ := data["ai_analysis"].(map[string]interface{})

			Expect(aiData).To(HaveKeyWithValue("confidence", float64(0.01)))
		})
	})
})


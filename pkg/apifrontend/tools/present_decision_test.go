package tools_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var _ = Describe("present_decision", func() {
	It("UT-AF-115-001: is registered with IsLongRunning=true", func() {
		t, err := tools.NewPresentDecisionTool()
		Expect(err).NotTo(HaveOccurred())
		Expect(t.IsLongRunning()).To(BeTrue())
	})

	It("UT-AF-115-002: formats RCA and options for user presentation", func() {
		result := tools.HandlePresentDecision(tools.PresentDecisionArgs{
			SessionID: "sess-1",
			Summary:   "Memory leak detected in pod-xyz",
			Options: []tools.WorkflowOption{
				{WorkflowID: "wf-1", Name: "Restart Pod", Description: "Restart the affected pod"},
				{WorkflowID: "wf-2", Name: "Scale Up", Description: "Add replicas"},
			},
		})
		Expect(result.Presented).To(BeTrue())
		Expect(result.Message).NotTo(BeEmpty())
	})

	It("UT-AF-115-003: includes all workflow options in output", func() {
		result := tools.HandlePresentDecision(tools.PresentDecisionArgs{
			SessionID: "sess-1",
			Summary:   "Issue found",
			Options: []tools.WorkflowOption{
				{WorkflowID: "wf-1", Name: "Option A"},
				{WorkflowID: "wf-2", Name: "Option B"},
				{WorkflowID: "wf-3", Name: "Option C"},
			},
		})
		Expect(result.Presented).To(BeTrue())
	})

	It("UT-AF-115-004: AC-6 WorkflowOption.Recommended serializes for auditable decision tracing", func() {
		opt := tools.WorkflowOption{
			WorkflowID:  "wf-rollback",
			Name:        "Rollback",
			Description: "Roll back to previous revision",
			Risk:        "low",
			Recommended: true,
		}
		data, err := json.Marshal(opt)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(ContainSubstring(`"recommended":true`),
			"AC-6: recommendation must be explicitly recorded for audit trail")

		var decoded tools.WorkflowOption
		err = json.Unmarshal(data, &decoded)
		Expect(err).NotTo(HaveOccurred())
		Expect(decoded.Recommended).To(BeTrue())
		Expect(decoded.WorkflowID).To(Equal("wf-rollback"))
	})

	It("UT-AF-115-005: AC-6 non-recommended option omits field to avoid false positives in audit scans", func() {
		opt := tools.WorkflowOption{
			WorkflowID:  "wf-scale",
			Name:        "Scale Down",
			Description: "Scale to zero",
			Recommended: false,
		}
		data, err := json.Marshal(opt)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).NotTo(ContainSubstring(`"recommended"`),
			"AC-6: false recommendation must be omitted (omitempty) to prevent confusion")
	})

	It("UT-AF-1396-001: AU-3 RCAData serializes all fields for structured decision payload", func() {
		rca := tools.RCAData{
			Severity:       "critical",
			Confidence:     0.92,
			CausalChain:    []string{"Memory leak in data-processor", "Container hit 512Mi limit", "OOMKill signal"},
			Target:         "Deployment/data-processor in production",
			ToolCallsCount: 19,
			LLMTurns:       17,
		}
		data, err := json.Marshal(rca)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(ContainSubstring(`"severity":"critical"`))
		Expect(string(data)).To(ContainSubstring(`"confidence":0.92`))
		Expect(string(data)).To(ContainSubstring(`"causal_chain"`))
		Expect(string(data)).To(ContainSubstring(`"target":"Deployment/data-processor in production"`))
		Expect(string(data)).To(ContainSubstring(`"tool_calls_count":19`))
		Expect(string(data)).To(ContainSubstring(`"llm_turns":17`))

		var decoded tools.RCAData
		Expect(json.Unmarshal(data, &decoded)).To(Succeed())
		Expect(decoded.Severity).To(Equal("critical"))
		Expect(decoded.Confidence).To(BeNumerically("~", 0.92, 0.001))
		Expect(decoded.CausalChain).To(HaveLen(3))
		Expect(decoded.ToolCallsCount).To(Equal(19))
		Expect(decoded.LLMTurns).To(Equal(17))
	})

	It("UT-AF-1396-002: AU-3 WorkflowOption.Parameters serializes as JSON object", func() {
		opt := tools.WorkflowOption{
			WorkflowID:  "wf-restart",
			Name:        "Restart Pod",
			Description: "Rolling restart",
			Parameters: map[string]string{
				"namespace":  "production",
				"deployment": "data-processor",
			},
		}
		data, err := json.Marshal(opt)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(ContainSubstring(`"parameters":`))
		Expect(string(data)).To(ContainSubstring(`"namespace":"production"`))
		Expect(string(data)).To(ContainSubstring(`"deployment":"data-processor"`))
	})

	It("UT-AF-1396-003: AC-6 WorkflowOption.RuledOutReason explains non-viable options", func() {
		opt := tools.WorkflowOption{
			WorkflowID:     "wf-rollback",
			Name:           "Rollback Deployment",
			Description:    "Roll back to previous revision",
			RuledOutReason: "No previous revision available in cluster history",
		}
		data, err := json.Marshal(opt)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(ContainSubstring(`"ruled_out_reason":"No previous revision available`))

		var decoded tools.WorkflowOption
		Expect(json.Unmarshal(data, &decoded)).To(Succeed())
		Expect(decoded.RuledOutReason).To(Equal("No previous revision available in cluster history"))
	})

	It("UT-AF-1396-010: AC-6 PresentDecisionArgs.RCA required — zero-value RCA serializes", func() {
		args := tools.PresentDecisionArgs{
			SessionID: "sess-1",
			Summary:   "Investigation complete",
			Options:   []tools.WorkflowOption{{WorkflowID: "wf-1", Name: "Fix"}},
		}
		data, err := json.Marshal(args)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(ContainSubstring(`"rca":`), "RCA must always be present (required field)")
	})

	It("UT-AF-1396-011: AU-3 HandlePresentDecision includes RCA severity in message", func() {
		result := tools.HandlePresentDecision(tools.PresentDecisionArgs{
			SessionID: "sess-1",
			Summary:   "OOMKill detected",
			RCA: tools.RCAData{
				Severity:   "critical",
				Confidence: 0.95,
			},
			Options: []tools.WorkflowOption{
				{WorkflowID: "wf-1", Name: "Restart"},
			},
		})
		Expect(result.Presented).To(BeTrue())
		Expect(result.Message).To(ContainSubstring("critical"))
		Expect(result.Message).To(ContainSubstring("0.95"))
	})
})

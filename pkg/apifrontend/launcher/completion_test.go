package launcher_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
)

var _ = Describe("Decision artifact recovery (#2365)", func() {
	It("UT-AF-2365-006 (AU-3, SI-10, ASVS 5.1): builds options from authoritative RCA and discovery data", func() {
		result, err := launcher.BuildRecoveredDecisionArtifact(launcher.DecisionRecoveryInput{
			SessionID: "sess-2365",
			RRID:      "rr-2365",
			RCA: map[string]any{
				"explanation":      "Deployment is restarting after a bad configuration.",
				"severity":         "critical",
				"confidence":       0.94,
				"target":           "Deployment/worker",
				"tool_calls_count": 8,
				"llm_turns":        4,
			},
			Discovery: &ka.DiscoverWorkflowsResult{
				Workflows: []ka.DiscoveredWorkflow{
					{WorkflowID: "wf-restart", Name: "Restart deployment", Description: "Restarts the deployment", Confidence: 0.91},
					{WorkflowID: "wf-rollout", Name: "Roll out previous version", Description: "Reverts the deployment", Confidence: 0.72},
				},
			},
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Complete).To(BeTrue())
		Expect(result.Data).To(HaveKeyWithValue("session_id", "sess-2365"))
		Expect(result.Data).To(HaveKeyWithValue("rr_id", "rr-2365"))
		Expect(result.Data).To(HaveKeyWithValue("source", "af_completion_recovery"))
		Expect(result.Data).To(HaveKeyWithValue("recovery_reason", "missing_present_decision"))
		Expect(result.Data["options"]).To(HaveLen(2))
		Expect(result.Data["summary"]).To(Equal("Deployment is restarting after a bad configuration."))
	})

	It("UT-AF-2365-007 (AU-3, SI-10): emits truthful failure data when authoritative RCA is missing", func() {
		result, err := launcher.BuildRecoveredDecisionArtifact(launcher.DecisionRecoveryInput{
			SessionID: "sess-2365",
			RRID:      "rr-2365",
			Discovery: &ka.DiscoverWorkflowsResult{
				Workflows: []ka.DiscoveredWorkflow{{WorkflowID: "wf-restart", Name: "Restart deployment"}},
			},
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Complete).To(BeFalse())
		Expect(result.Data).To(HaveKeyWithValue("status", "failure"))
		Expect(result.Data).To(HaveKeyWithValue("failure_reason", "missing_authoritative_data"))
		Expect(result.Data["options"]).To(BeEmpty())
		rca, ok := result.Data["rca"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(rca["explanation"]).To(Equal("Authoritative investigation details were unavailable."))
		Expect(rca).NotTo(HaveKey("severity"))
		Expect(rca).NotTo(HaveKey("target"))
	})

	It("UT-AF-2365-011 (SI-10, ASVS 5.5.2): recovered artifact satisfies investigation_summary schema", func() {
		result, err := launcher.BuildRecoveredDecisionArtifact(launcher.DecisionRecoveryInput{
			SessionID: "sess-2365",
			RCA:       map[string]any{"explanation": "A valid grounded explanation."},
			Discovery: &ka.DiscoverWorkflowsResult{},
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(launcher.ValidatePayloadForTest("investigation_summary", result.Data)).To(Succeed())
	})
})

package remediationorchestrator_test

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator"
)

func TestRemediationOrchestrator(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "RemediationOrchestrator Types Suite")
}

// BR-ORCH-025: Core Orchestration Configuration
// BR-ORCH-026: Approval Orchestration
// BR-ORCH-027: Global Timeout Management
// BR-ORCH-028: Per-Phase Timeout Management
var _ = Describe("BR-ORCH-025: Orchestrator Configuration", func() {

	// PhaseTimeouts validates timeout configuration for business SLAs
	// Reference: SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md - DescribeTable pattern
	Describe("PhaseTimeouts", func() {
		DescribeTable("DefaultPhaseTimeouts should return configured timeout values",
			func(phaseName string, getTimeout func(remediationorchestrator.PhaseTimeouts) time.Duration, expected time.Duration, brRef string) {
				timeouts := remediationorchestrator.DefaultPhaseTimeouts()
				Expect(getTimeout(timeouts)).To(Equal(expected), "%s: %s", brRef, phaseName)
			},
			Entry("Processing timeout (BR-ORCH-028)",
				"Processing",
				func(t remediationorchestrator.PhaseTimeouts) time.Duration { return t.Processing },
				5*time.Minute,
				"BR-ORCH-028"),
			Entry("Analyzing timeout (BR-ORCH-028)",
				"Analyzing",
				func(t remediationorchestrator.PhaseTimeouts) time.Duration { return t.Analyzing },
				10*time.Minute,
				"BR-ORCH-028"),
			Entry("Executing timeout (BR-ORCH-028)",
				"Executing",
				func(t remediationorchestrator.PhaseTimeouts) time.Duration { return t.Executing },
				30*time.Minute,
				"BR-ORCH-028"),
			Entry("Global timeout (BR-ORCH-027)",
				"Global",
				func(t remediationorchestrator.PhaseTimeouts) time.Duration { return t.Global },
				60*time.Minute,
				"BR-ORCH-027"),
			Entry("AwaitingApproval timeout (BR-ORCH-026)",
				"AwaitingApproval",
				func(t remediationorchestrator.PhaseTimeouts) time.Duration { return t.AwaitingApproval },
				24*time.Hour,
				"BR-ORCH-026"),
		)
	})

	// OrchestratorConfig validates default configuration values
	Describe("OrchestratorConfig", func() {
		DescribeTable("DefaultConfig should return configured operational values",
			func(configName string, validateFunc func(remediationorchestrator.OrchestratorConfig), brRef string) {
				config := remediationorchestrator.DefaultConfig()
				validateFunc(config)
			},
			Entry("Global timeout configured (BR-ORCH-027)",
				"Timeouts.Global",
				func(c remediationorchestrator.OrchestratorConfig) {
					Expect(c.Timeouts.Global).To(Equal(60 * time.Minute))
				},
				"BR-ORCH-027"),
			Entry("24h retention period (BR-ORCH-025)",
				"RetentionPeriod",
				func(c remediationorchestrator.OrchestratorConfig) {
					Expect(c.RetentionPeriod).To(Equal(24 * time.Hour))
				},
				"BR-ORCH-025"),
			Entry("10 max concurrent reconciles (BR-ORCH-025)",
				"MaxConcurrentReconciles",
				func(c remediationorchestrator.OrchestratorConfig) {
					Expect(c.MaxConcurrentReconciles).To(Equal(10))
				},
				"BR-ORCH-025"),
			Entry("Metrics enabled by default (BR-ORCH-025)",
				"EnableMetrics",
				func(c remediationorchestrator.OrchestratorConfig) {
					Expect(c.EnableMetrics).To(BeTrue())
				},
				"BR-ORCH-025"),
		)
	})

	// ChildCRDRefs validates orchestration progress tracking
	// Business behavior focus: HasAllCore determines if orchestration can proceed
	Describe("ChildCRDRefs", func() {
		DescribeTable("HasAllCore should validate orchestration progress",
			func(refs remediationorchestrator.ChildCRDRefs, expectComplete bool, description string) {
				Expect(refs.HasAllCore()).To(Equal(expectComplete), description)
			},
			// Complete scenarios (BR-ORCH-025)
			Entry("complete with all 3 core refs",
				remediationorchestrator.ChildCRDRefs{
					SignalProcessing:  "sp-test",
					AIAnalysis:        "ai-test",
					WorkflowExecution: "we-test",
				},
				true,
				"All core child CRDs created"),
			Entry("complete with all refs including notification",
				remediationorchestrator.ChildCRDRefs{
					SignalProcessing:    "sp-test",
					AIAnalysis:          "ai-test",
					WorkflowExecution:   "we-test",
					NotificationRequest: "nr-test",
				},
				true,
				"All child CRDs including notification created"),

			// Incomplete scenarios (BR-ORCH-025)
			Entry("incomplete with no refs",
				remediationorchestrator.ChildCRDRefs{},
				false,
				"No child CRDs created yet"),
			Entry("incomplete with only SignalProcessing",
				remediationorchestrator.ChildCRDRefs{
					SignalProcessing: "sp-test",
				},
				false,
				"Only in Processing phase"),
			Entry("incomplete with SignalProcessing + AIAnalysis",
				remediationorchestrator.ChildCRDRefs{
					SignalProcessing: "sp-test",
					AIAnalysis:       "ai-test",
				},
				false,
				"Missing WorkflowExecution - in Analyzing phase"),
			Entry("incomplete with only AIAnalysis + WorkflowExecution",
				remediationorchestrator.ChildCRDRefs{
					AIAnalysis:        "ai-test",
					WorkflowExecution: "we-test",
				},
				false,
				"Missing SignalProcessing - invalid state"),
			Entry("notification only does not make it complete (BR-ORCH-001)",
				remediationorchestrator.ChildCRDRefs{
					NotificationRequest: "nr-test",
				},
				false,
				"NotificationRequest alone is insufficient"),
		)
	})

	// ReconcileResult validates requeue decision logic
	// Business behavior focus: ShouldRequeue determines if reconciliation continues
	Describe("ReconcileResult", func() {
		DescribeTable("ShouldRequeue should validate requeue decision logic",
			func(result remediationorchestrator.ReconcileResult, expectRequeue bool, description string) {
				Expect(result.ShouldRequeue()).To(Equal(expectRequeue), description)
			},
			// Requeue scenarios (BR-ORCH-025)
			Entry("requeue when Requeue is true",
				remediationorchestrator.ReconcileResult{Requeue: true},
				true,
				"Explicit requeue request"),
			Entry("requeue when RequeueAfter is set",
				remediationorchestrator.ReconcileResult{RequeueAfter: time.Second},
				true,
				"Delayed requeue for status polling"),
			Entry("requeue when both Requeue and RequeueAfter set",
				remediationorchestrator.ReconcileResult{Requeue: true, RequeueAfter: time.Second},
				true,
				"Both requeue signals active"),

			// No requeue scenarios (terminal states, BR-ORCH-025)
			Entry("no requeue when empty result",
				remediationorchestrator.ReconcileResult{},
				false,
				"Terminal state reached - no further action"),
			Entry("no requeue when only ChildCreated set",
				remediationorchestrator.ReconcileResult{ChildCreated: "sp-test"},
				false,
				"Child created but no explicit requeue"),
			Entry("no requeue when only Error set",
				remediationorchestrator.ReconcileResult{Error: nil},
				false,
				"No requeue flags set"),
		)

		// ChildCreated tracking (distinct business behavior)
		It("should track created child CRD name for audit trail", func() {
			result := remediationorchestrator.ReconcileResult{
				ChildCreated: "sp-abc123",
				Requeue:      true,
			}
			Expect(result.ChildCreated).To(Equal("sp-abc123"))
			Expect(result.ShouldRequeue()).To(BeTrue())
		})
	})
})

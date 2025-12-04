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

var _ = Describe("RemediationOrchestrator Types", func() {
	// BR-ORCH-027, BR-ORCH-028: Timeout configuration
	Describe("PhaseTimeouts", func() {
		Describe("DefaultPhaseTimeouts", func() {
			var timeouts remediationorchestrator.PhaseTimeouts

			BeforeEach(func() {
				timeouts = remediationorchestrator.DefaultPhaseTimeouts()
			})

			It("should have 5 minute Processing timeout", func() {
				Expect(timeouts.Processing).To(Equal(5 * time.Minute))
			})

			It("should have 10 minute Analyzing timeout", func() {
				Expect(timeouts.Analyzing).To(Equal(10 * time.Minute))
			})

			It("should have 30 minute Executing timeout", func() {
				Expect(timeouts.Executing).To(Equal(30 * time.Minute))
			})

			// BR-ORCH-027: Global timeout
			It("should have 60 minute Global timeout", func() {
				Expect(timeouts.Global).To(Equal(60 * time.Minute))
			})

			// BR-ORCH-026: Approval timeout (24h by default)
			It("should have 24 hour AwaitingApproval timeout", func() {
				Expect(timeouts.AwaitingApproval).To(Equal(24 * time.Hour))
			})
		})
	})

	// BR-ORCH-025: Configuration
	Describe("OrchestratorConfig", func() {
		Describe("DefaultConfig", func() {
			var config remediationorchestrator.OrchestratorConfig

			BeforeEach(func() {
				config = remediationorchestrator.DefaultConfig()
			})

			It("should have default timeouts", func() {
				Expect(config.Timeouts.Global).To(Equal(60 * time.Minute))
			})

			It("should have 24h retention period", func() {
				Expect(config.RetentionPeriod).To(Equal(24 * time.Hour))
			})

			It("should have 10 max concurrent reconciles", func() {
				Expect(config.MaxConcurrentReconciles).To(Equal(10))
			})

			It("should have metrics enabled by default", func() {
				Expect(config.EnableMetrics).To(BeTrue())
			})
		})
	})

	// BR-ORCH-025: Child CRD references
	Describe("ChildCRDRefs", func() {
		It("should store SignalProcessing reference", func() {
			refs := remediationorchestrator.ChildCRDRefs{
				SignalProcessing: "sp-test-123",
			}
			Expect(refs.SignalProcessing).To(Equal("sp-test-123"))
		})

		It("should store AIAnalysis reference", func() {
			refs := remediationorchestrator.ChildCRDRefs{
				AIAnalysis: "ai-test-123",
			}
			Expect(refs.AIAnalysis).To(Equal("ai-test-123"))
		})

		It("should store WorkflowExecution reference", func() {
			refs := remediationorchestrator.ChildCRDRefs{
				WorkflowExecution: "we-test-123",
			}
			Expect(refs.WorkflowExecution).To(Equal("we-test-123"))
		})

		// BR-ORCH-001: Approval notification reference
		It("should store NotificationRequest reference", func() {
			refs := remediationorchestrator.ChildCRDRefs{
				NotificationRequest: "nr-test-123",
			}
			Expect(refs.NotificationRequest).To(Equal("nr-test-123"))
		})

		Describe("HasAll", func() {
			It("should return true when all core refs are set", func() {
				refs := remediationorchestrator.ChildCRDRefs{
					SignalProcessing:  "sp-test",
					AIAnalysis:        "ai-test",
					WorkflowExecution: "we-test",
				}
				Expect(refs.HasAllCore()).To(BeTrue())
			})

			It("should return false when SignalProcessing is missing", func() {
				refs := remediationorchestrator.ChildCRDRefs{
					AIAnalysis:        "ai-test",
					WorkflowExecution: "we-test",
				}
				Expect(refs.HasAllCore()).To(BeFalse())
			})
		})
	})

	// BR-ORCH-025: Reconcile result
	Describe("ReconcileResult", func() {
		It("should indicate requeue needed", func() {
			result := remediationorchestrator.ReconcileResult{
				Requeue: true,
			}
			Expect(result.Requeue).To(BeTrue())
		})

		It("should indicate requeue after duration", func() {
			result := remediationorchestrator.ReconcileResult{
				RequeueAfter: 30 * time.Second,
			}
			Expect(result.RequeueAfter).To(Equal(30 * time.Second))
		})

		It("should track created child CRD name", func() {
			result := remediationorchestrator.ReconcileResult{
				ChildCreated: "sp-abc123",
			}
			Expect(result.ChildCreated).To(Equal("sp-abc123"))
		})

		Describe("ShouldRequeue", func() {
			It("should return true when Requeue is true", func() {
				result := remediationorchestrator.ReconcileResult{Requeue: true}
				Expect(result.ShouldRequeue()).To(BeTrue())
			})

			It("should return true when RequeueAfter > 0", func() {
				result := remediationorchestrator.ReconcileResult{RequeueAfter: time.Second}
				Expect(result.ShouldRequeue()).To(BeTrue())
			})

			It("should return false when no requeue needed", func() {
				result := remediationorchestrator.ReconcileResult{}
				Expect(result.ShouldRequeue()).To(BeFalse())
			})
		})
	})
})


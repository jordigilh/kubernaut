package phase_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/phase"
)

func TestPhase(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Phase Suite")
}

var _ = Describe("Phase Types", func() {
	// BR-ORCH-025: Core orchestration phases
	Describe("Phase Constants", func() {
		It("should define Pending phase", func() {
			Expect(phase.Pending).To(Equal(phase.Phase("Pending")))
		})

		It("should define Processing phase", func() {
			Expect(phase.Processing).To(Equal(phase.Phase("Processing")))
		})

		It("should define Analyzing phase", func() {
			Expect(phase.Analyzing).To(Equal(phase.Phase("Analyzing")))
		})

		It("should define AwaitingApproval phase", func() {
			Expect(phase.AwaitingApproval).To(Equal(phase.Phase("AwaitingApproval")))
		})

		It("should define Executing phase", func() {
			Expect(phase.Executing).To(Equal(phase.Phase("Executing")))
		})

		It("should define Completed phase", func() {
			Expect(phase.Completed).To(Equal(phase.Phase("Completed")))
		})

		It("should define Failed phase", func() {
			Expect(phase.Failed).To(Equal(phase.Phase("Failed")))
		})

		// BR-ORCH-027, BR-ORCH-028: Timeout phases
		It("should define TimedOut phase", func() {
			Expect(phase.TimedOut).To(Equal(phase.Phase("TimedOut")))
		})

		// BR-ORCH-032: Skipped phase for resource lock deduplication
		It("should define Skipped phase", func() {
			Expect(phase.Skipped).To(Equal(phase.Phase("Skipped")))
		})
	})

	// BR-ORCH-025: Phase transition validation
	Describe("IsTerminal", func() {
		DescribeTable("terminal phases",
			func(p phase.Phase, expected bool) {
				Expect(phase.IsTerminal(p)).To(Equal(expected))
			},
			Entry("Pending is not terminal", phase.Pending, false),
			Entry("Processing is not terminal", phase.Processing, false),
			Entry("Analyzing is not terminal", phase.Analyzing, false),
			Entry("AwaitingApproval is not terminal", phase.AwaitingApproval, false),
			Entry("Executing is not terminal", phase.Executing, false),
			Entry("Completed is terminal", phase.Completed, true),
			Entry("Failed is terminal", phase.Failed, true),
			Entry("TimedOut is terminal", phase.TimedOut, true),
			Entry("Skipped is terminal", phase.Skipped, true),
		)
	})

	// BR-ORCH-025: Valid phase transitions
	Describe("CanTransition", func() {
		Context("from Pending phase", func() {
			It("should allow transition to Processing", func() {
				Expect(phase.CanTransition(phase.Pending, phase.Processing)).To(BeTrue())
			})

			It("should not allow transition to Analyzing", func() {
				Expect(phase.CanTransition(phase.Pending, phase.Analyzing)).To(BeFalse())
			})

			It("should not allow transition to Completed", func() {
				Expect(phase.CanTransition(phase.Pending, phase.Completed)).To(BeFalse())
			})
		})

		Context("from Processing phase", func() {
			It("should allow transition to Analyzing", func() {
				Expect(phase.CanTransition(phase.Processing, phase.Analyzing)).To(BeTrue())
			})

			It("should allow transition to Failed", func() {
				Expect(phase.CanTransition(phase.Processing, phase.Failed)).To(BeTrue())
			})

			It("should allow transition to TimedOut", func() {
				Expect(phase.CanTransition(phase.Processing, phase.TimedOut)).To(BeTrue())
			})

			It("should not allow transition to Completed", func() {
				Expect(phase.CanTransition(phase.Processing, phase.Completed)).To(BeFalse())
			})
		})

		Context("from Analyzing phase", func() {
			// BR-ORCH-026: Approval orchestration
			It("should allow transition to AwaitingApproval", func() {
				Expect(phase.CanTransition(phase.Analyzing, phase.AwaitingApproval)).To(BeTrue())
			})

			It("should allow transition to Executing (auto-approved)", func() {
				Expect(phase.CanTransition(phase.Analyzing, phase.Executing)).To(BeTrue())
			})

			It("should allow transition to Failed", func() {
				Expect(phase.CanTransition(phase.Analyzing, phase.Failed)).To(BeTrue())
			})

			It("should allow transition to TimedOut", func() {
				Expect(phase.CanTransition(phase.Analyzing, phase.TimedOut)).To(BeTrue())
			})
		})

		Context("from AwaitingApproval phase", func() {
			It("should allow transition to Executing", func() {
				Expect(phase.CanTransition(phase.AwaitingApproval, phase.Executing)).To(BeTrue())
			})

			It("should allow transition to Failed (rejected)", func() {
				Expect(phase.CanTransition(phase.AwaitingApproval, phase.Failed)).To(BeTrue())
			})

			It("should allow transition to TimedOut", func() {
				Expect(phase.CanTransition(phase.AwaitingApproval, phase.TimedOut)).To(BeTrue())
			})
		})

		Context("from Executing phase", func() {
			It("should allow transition to Completed", func() {
				Expect(phase.CanTransition(phase.Executing, phase.Completed)).To(BeTrue())
			})

			It("should allow transition to Failed", func() {
				Expect(phase.CanTransition(phase.Executing, phase.Failed)).To(BeTrue())
			})

			It("should allow transition to TimedOut", func() {
				Expect(phase.CanTransition(phase.Executing, phase.TimedOut)).To(BeTrue())
			})

			// BR-ORCH-032: Skipped due to resource lock
			It("should allow transition to Skipped", func() {
				Expect(phase.CanTransition(phase.Executing, phase.Skipped)).To(BeTrue())
			})
		})

		Context("from terminal phases", func() {
			It("should not allow transition from Completed", func() {
				Expect(phase.CanTransition(phase.Completed, phase.Failed)).To(BeFalse())
				Expect(phase.CanTransition(phase.Completed, phase.Processing)).To(BeFalse())
			})

			It("should not allow transition from Failed", func() {
				Expect(phase.CanTransition(phase.Failed, phase.Completed)).To(BeFalse())
				Expect(phase.CanTransition(phase.Failed, phase.Processing)).To(BeFalse())
			})

			It("should not allow transition from TimedOut", func() {
				Expect(phase.CanTransition(phase.TimedOut, phase.Processing)).To(BeFalse())
			})

			It("should not allow transition from Skipped", func() {
				Expect(phase.CanTransition(phase.Skipped, phase.Processing)).To(BeFalse())
			})
		})
	})

	// BR-ORCH-025: Phase validation
	Describe("Validate", func() {
		DescribeTable("valid phases",
			func(p phase.Phase) {
				Expect(phase.Validate(p)).To(Succeed())
			},
			Entry("Pending", phase.Pending),
			Entry("Processing", phase.Processing),
			Entry("Analyzing", phase.Analyzing),
			Entry("AwaitingApproval", phase.AwaitingApproval),
			Entry("Executing", phase.Executing),
			Entry("Completed", phase.Completed),
			Entry("Failed", phase.Failed),
			Entry("TimedOut", phase.TimedOut),
			Entry("Skipped", phase.Skipped),
		)

		It("should return error for invalid phase", func() {
			err := phase.Validate(phase.Phase("InvalidPhase"))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid phase"))
		})
	})
})


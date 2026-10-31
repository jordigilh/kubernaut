package phase_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/workflowexecution/phase"
)

var _ = Describe("UT-WE-2582-002: verified cancellation state machine [BR-WE-014, AC-4]", func() {
	It("allows Running to enter Cancelling and only Cancelling to become Cancelled", func() {
		Expect(phase.CanTransition(phase.Running, phase.Cancelling)).To(BeTrue())
		Expect(phase.CanTransition(phase.Cancelling, phase.Cancelled)).To(BeTrue())
		Expect(phase.IsTerminal(phase.Cancelling)).To(BeFalse())
		Expect(phase.IsTerminal(phase.Cancelled)).To(BeTrue())
		Expect(phase.CanTransition(phase.Running, phase.Cancelled)).To(BeFalse())
	})
})

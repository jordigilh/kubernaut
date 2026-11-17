package phase_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/workflowexecution/phase"
)

func TestPhase(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "WorkflowExecution phase suite")
}

var _ = Describe("WorkflowExecution cancellation phases", func() {
	It("UT-WE-2588-PHASE-001: validates and terminalizes cancellation phases", func() {
		Expect(phase.Validate(phase.Cancelling)).To(Succeed())
		Expect(phase.Validate(phase.Cancelled)).To(Succeed())
		Expect(phase.IsTerminal(phase.Cancelled)).To(BeTrue())
	})

	It("UT-WE-2588-PHASE-002: permits active work to enter and complete cancellation", func() {
		for _, active := range []phase.Phase{phase.Pending, phase.Running} {
			Expect(phase.CanTransition(active, phase.Cancelling)).To(BeTrue(), "active phase %s", active)
		}
		Expect(phase.CanTransition(phase.Cancelling, phase.Cancelled)).To(BeTrue())
	})
})

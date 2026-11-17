package phase_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/aianalysis/phase"
)

func TestPhase(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AIAnalysis phase suite")
}

var _ = Describe("AIAnalysis cancellation phases", func() {
	It("UT-AI-2588-001: validates and terminalizes cancellation phases", func() {
		Expect(phase.Validate(phase.Cancelling)).To(Succeed())
		Expect(phase.Validate(phase.Cancelled)).To(Succeed())
		Expect(phase.IsTerminal(phase.Cancelled)).To(BeTrue())
	})

	It("UT-AI-2588-006: permits active work to enter and complete cancellation", func() {
		for _, active := range []phase.Phase{phase.Pending, phase.Investigating, phase.Analyzing} {
			Expect(phase.CanTransition(active, phase.Cancelling)).To(BeTrue(), "active phase %s", active)
		}
		Expect(phase.CanTransition(phase.Cancelling, phase.Cancelled)).To(BeTrue())
	})
})

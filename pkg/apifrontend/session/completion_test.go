package session_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/session"
)

var _ = Describe("Business-outcome completion policy (#2365)", func() {
	It("UT-AF-2365-003 (BR-INTERACTIVE-010, AC-6): full remediation requires presentation but remains execution-blocked", func() {
		obligation := session.DecisionObligation(session.InteractionModeFullRemediation, true, true)

		Expect(obligation.PresentationRequired).To(BeTrue())
		Expect(obligation.ExecutionAllowed).To(BeFalse())
	})

	It("UT-AF-2365-003b (BR-INTERACTIVE-010, AC-6): autonomous remediation does not activate presentation recovery", func() {
		obligation := session.DecisionObligation(session.InteractionModeFullRemediationAutonomous, true, false)

		Expect(obligation.PresentationRequired).To(BeFalse())
		Expect(obligation.ExecutionAllowed).To(BeTrue())
	})

	It("UT-AF-2365-005 (BR-INTERACTIVE-010, AC-6): interactive discovery requires presentation without granting execution", func() {
		obligation := session.DecisionObligation(session.InteractionModeInteractive, true, true)

		Expect(obligation.PresentationRequired).To(BeTrue())
		Expect(obligation.ExecutionAllowed).To(BeFalse())
	})

	It("UT-AF-2365-005b (SI-10): invalid mode fails safe to interactive policy", func() {
		obligation := session.DecisionObligation("invalid", true, true)

		Expect(obligation.PresentationRequired).To(BeTrue())
		Expect(obligation.ExecutionAllowed).To(BeFalse())
	})

	It("UT-AF-2365-005c (BR-SESS-013): discovery has no presentation obligation before it succeeds", func() {
		obligation := session.DecisionObligation(session.InteractionModeFullRemediation, false, false)

		Expect(obligation.PresentationRequired).To(BeFalse())
		Expect(obligation.ExecutionAllowed).To(BeTrue())
	})
})

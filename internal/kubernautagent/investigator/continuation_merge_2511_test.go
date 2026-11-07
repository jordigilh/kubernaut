/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
*/

package investigator

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// BR-KA-016 / DD-KA-2511; FedRAMP SI-10/AU-3; SOC 2 CC7.2.
var _ = Describe("RCA continuation merge semantics", func() {
	It("UT-KA-2511-012: preserves prior non-actionable state when continuation omits it", func() {
		previousActionable := false
		previous := &katypes.InvestigationResult{
			RCASummary:        "provisional",
			IsActionable:      &previousActionable,
			RemediationTarget: katypes.RemediationTarget{Kind: "Deployment", Name: "api"},
		}
		updated := &katypes.InvestigationResult{RCASummary: "reviewed"}

		merged := mergeRCAContinuationResult(previous, updated)

		Expect(merged.IsActionable).To(HaveValue(BeFalse()))
	})

	It("UT-KA-2511-013: preserves an explicit zero confidence from continuation", func() {
		previous := &katypes.InvestigationResult{Confidence: 0.8, ConfidenceSet: true}
		updated := &katypes.InvestigationResult{Confidence: 0, ConfidenceSet: true}

		merged := mergeRCAContinuationResult(previous, updated)

		Expect(merged.Confidence).To(BeZero())
		Expect(merged.ConfidenceSet).To(BeTrue())
	})

	It("UT-KA-2586-008: a continuation's explicit zero supersedes the prior RCA score (BR-AI-012, AU-3)", func() {
		prior := .8
		zero := 0.0
		previous := &katypes.InvestigationResult{Confidence: prior, ConfidenceSet: true, RCAConfidence: &prior}
		updated := &katypes.InvestigationResult{Confidence: 0, ConfidenceSet: true, RCAConfidence: &zero}
		merged := mergeRCAContinuationResult(previous, updated)
		phase1 := BuildPhase1Context(merged)
		Expect(phase1.RCAConfidence).To(HaveValue(Equal(0.0)))
	})

	It("UT-KA-2586-009: a continuation omitting confidence retains prior presence, including unknown (BR-AI-012, SI-10)", func() {
		zero := 0.0
		merged := mergeRCAContinuationResult(&katypes.InvestigationResult{RCAConfidence: &zero}, &katypes.InvestigationResult{})
		Expect(merged.RCAConfidence).To(HaveValue(Equal(0.0)))
		merged = mergeRCAContinuationResult(&katypes.InvestigationResult{}, &katypes.InvestigationResult{})
		Expect(merged.RCAConfidence).To(BeNil())
	})

	It("UT-KA-2586-033: legacy continuation confidence does not create RCA confidence (BR-AI-012, SI-10)", func() {
		merged := mergeRCAContinuationResult(&katypes.InvestigationResult{}, &katypes.InvestigationResult{Confidence: .95, ConfidenceSet: true})
		Expect(merged.RCAConfidence).To(BeNil())
	})
})

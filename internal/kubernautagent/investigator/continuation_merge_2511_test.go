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
})

/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
*/

package investigator_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// BR-AUDIT-005 / DD-KA-2511; FedRAMP AU-3; SOC 2 CC7.2; ASVS V16.2.x.
var _ = Describe("#2511 recurrent RCA audit projection", func() {
	It("UT-KA-2511-007: preserves causal mechanism status in the audit-safe result", func() {
		data := investigator.ResultToAuditJSON(&katypes.InvestigationResult{
			RCASummary:            "The mechanism remains uncertain",
			CausalMechanismStatus: "unknown",
		})
		Expect(data).To(HaveKeyWithValue("causal_mechanism_status", "unknown"))
	})
})

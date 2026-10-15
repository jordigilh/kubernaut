/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
*/

package enrichment_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// BR-KA-016 / DD-KA-2511; FedRAMP AU-3/SI-10; SOC 2 CC7.2.
var _ = Describe("#2510 applied-state enrichment mapping", func() {
	It("UT-KA-2510-002: maps server-derived state and current hash into the Agent DTO", func() {
		client := &stubDSClient{response: &ogenclient.RemediationHistoryContext{
			TargetResource:     "prod/Deployment/api",
			CurrentSpecHash:    "sha256:current",
			RegressionDetected: false,
			Tier1: ogenclient.RemediationHistoryTier1{Chain: []ogenclient.RemediationHistoryEntry{{
				RemediationUID:          "rr-2510",
				AppliedState:            ogenclient.NewOptRemediationHistoryEntryAppliedState(ogenclient.RemediationHistoryEntryAppliedStateConfirmed),
				SpecChangeStatus:        ogenclient.NewOptRemediationHistoryEntrySpecChangeStatus(ogenclient.RemediationHistoryEntrySpecChangeStatusPostHashMatchesCurrent),
				PreRemediationSpecHash:  ogenclient.NewOptString("sha256:before"),
				PostRemediationSpecHash: ogenclient.NewOptString("sha256:current"),
			}}},
		}}

		result, err := enrichment.NewDSAdapter(client).GetRemediationHistory(context.Background(), "Deployment", "api", "prod", "", "sha256:current")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.CurrentSpecHash).To(Equal("sha256:current"))
		Expect(result.Tier1).To(HaveLen(1))
		Expect(result.Tier1[0].AppliedState).To(Equal("confirmed"))
		Expect(result.Tier1[0].SpecChangeStatus).To(Equal("post_hash_matches_current"))
		Expect(result.Tier1[0].PostRemediationSpecHash).To(Equal("sha256:current"))
	})
})

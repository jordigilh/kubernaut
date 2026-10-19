/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package enrichment_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

type issue2533HistoryContextClient struct {
	response ogenclient.GetRemediationHistoryContextRes
}

func (c *issue2533HistoryContextClient) GetRemediationHistoryContext(
	_ context.Context,
	_ ogenclient.GetRemediationHistoryContextParams,
) (ogenclient.GetRemediationHistoryContextRes, error) {
	return c.response, nil
}

var _ = Describe("Issue #2533 DataStorage history projection", func() {
	It("IT-KA-2533-002: maps both historical decision fields through the production adapter", func() {
		expectedJustification := sharedtypes.RepeatJustification{
			EvidenceBasis:         "Current evidence basis",
			PriorOutcomeRelevance: "The prior outcome was ineffective",
			ExpectedMechanism:     "The workflow addresses the observed mechanism",
		}
		justification := ogenclient.RepeatJustification{
			EvidenceBasis:         expectedJustification.EvidenceBasis,
			PriorOutcomeRelevance: expectedJustification.PriorOutcomeRelevance,
			ExpectedMechanism:     expectedJustification.ExpectedMechanism,
		}
		client := &issue2533HistoryContextClient{response: &ogenclient.RemediationHistoryContext{
			TargetResource:  "prod/Deployment/api",
			CurrentSpecHash: "sha256:current",
			Tier1: ogenclient.RemediationHistoryTier1{Chain: []ogenclient.RemediationHistoryEntry{{
				RemediationUID:      "rr-2533",
				WorkflowRationale:   ogenclient.NewOptString("The current target evidence supports the catalog workflow."),
				RepeatJustification: ogenclient.NewOptRepeatJustification(justification),
			}}},
			Tier2: ogenclient.RemediationHistoryTier2{Chain: []ogenclient.RemediationHistorySummary{{
				RemediationUID:      "rr-tier2-2533",
				WorkflowRationale:   ogenclient.NewOptString("Older target evidence supports the historical mechanism."),
				RepeatJustification: ogenclient.NewOptRepeatJustification(justification),
			}}},
		}}
		result, err := enrichment.NewDSAdapter(client).GetRemediationHistory(context.Background(), "Deployment", "api", "prod", "cluster-2533", "sha256:current")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Tier1).To(HaveLen(1))
		Expect(result.Tier1[0].WorkflowRationale).To(Equal("The current target evidence supports the catalog workflow."))
		Expect(result.Tier1[0].RepeatJustification).To(HaveValue(Equal(expectedJustification)))
		Expect(result.Tier2).To(HaveLen(1))
		Expect(result.Tier2[0].WorkflowRationale).To(Equal("Older target evidence supports the historical mechanism."))
		Expect(result.Tier2[0].RepeatJustification).To(HaveValue(Equal(expectedJustification)))
	})
})

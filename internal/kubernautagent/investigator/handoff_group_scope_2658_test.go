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

package investigator

import (
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// #2658: the handoff target filter must treat the group/version path parts as part of
// the identity, not as evidence of a different target. Before this, every
// group-bearing history result was discarded as "another target's history",
// silently starving the workflow-selection phase of prior attempts
// (BR-KA-016, DD-KA-2496, AU-3).
var _ = Describe("scopedHandoffInput keeps group-bearing history (#2658)", func() {
	inv := &Investigator{logger: logr.Discard()}

	handoffWith := func(target string) handoffInput {
		return handoffInput{
			Signal: katypes.SignalContext{
				Name: "api-server-abc", Namespace: "production",
				ResourceKind: "Deployment", ResourceName: "api-server",
			},
			EnrichData: &prompt.EnrichmentData{
				HistoryResult: &enrichment.RemediationHistoryResult{
					TargetResource: target,
					Tier1:          []enrichment.Tier1Entry{{WorkflowID: "wf-1"}},
				},
			},
		}
	}

	It("retains history whose target carries the same base identity plus a group", func() {
		out := inv.scopedHandoffInput(handoffWith("production/apps/v1/Deployment/api-server"))
		Expect(out.EnrichData).NotTo(BeNil())
		Expect(out.EnrichData.HistoryResult).NotTo(BeNil(),
			"group and version are identity parts and must not invalidate the match")
		Expect(out.EnrichData.HistoryResult.Tier1).To(HaveLen(1))
	})

	It("retains the unresolved form for the same base identity", func() {
		out := inv.scopedHandoffInput(handoffWith("production/unresolved/unresolved/Deployment/api-server"))
		Expect(out.EnrichData.HistoryResult).NotTo(BeNil())
	})

	It("still discards a different target even when it carries a group", func() {
		out := inv.scopedHandoffInput(handoffWith("production/apps/v1/Deployment/other-app"))
		Expect(out.EnrichData.HistoryResult).To(BeNil(),
			"cross-target history must stay fail-closed")
	})

	It("still discards a cross-namespace sibling", func() {
		out := inv.scopedHandoffInput(handoffWith("staging/apps/v1/Deployment/api-server"))
		Expect(out.EnrichData.HistoryResult).To(BeNil())
	})
})

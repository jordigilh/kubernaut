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

package prompt_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
)

// BR-WORKFLOW-016 / #779: Enrichment data must be fully rendered in the
// workflow selection prompt (Phase 3) so the LLM has visibility into detected
// labels when choosing a remediation workflow. Investigation (Phase 1) does NOT
// include enrichment data — it should discover context via tools.

var _ = Describe("UT-KA-779-EP: Complete enrichment-to-prompt rendering", func() {

	var builder *prompt.Builder

	BeforeEach(func() {
		var err error
		builder, err = prompt.NewBuilder()
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("UT-KA-779-EP-003: RenderWorkflowSelection includes all DetectedLabels in enrichment context", func() {
		It("should include all detected label keys in the workflow selection prompt", func() {
			enrichData := &prompt.EnrichmentData{
				DetectedLabels: map[string]string{
					"gitOpsManaged": "true",
					"gitOpsTool":    "fluxcd",
					"helmManaged":   "true",
					"serviceMesh":   "linkerd",
				},
				OwnerChain: []string{"Deployment/web-app", "ReplicaSet/web-app-abc"},
			}

			rendered, err := builder.RenderWorkflowSelection(prompt.WorkflowSelectionInput{
				Signal: prompt.SignalData{
					Name:      "web-app",
					Namespace: "production",
					Severity:  "critical",
					Message:   "OOMKilled",
				},
				RCASummary: "OOM due to memory leak in web container",
				EnrichData: enrichData,
			})
			Expect(err).NotTo(HaveOccurred())

			for key := range enrichData.DetectedLabels {
				Expect(rendered).To(ContainSubstring(key),
					"DetectedLabels key %q must appear in workflow selection prompt", key)
			}
			Expect(rendered).To(ContainSubstring("Deployment/web-app"),
				"Owner chain must appear in workflow selection prompt")
		})
	})
})

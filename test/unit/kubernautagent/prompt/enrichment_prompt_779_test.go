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

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
)

// BR-WORKFLOW-016 / #779: Enrichment data must be fully rendered in prompts
// so the LLM has complete visibility into detected labels and quota details.

var _ = Describe("UT-KA-779-EP: Complete enrichment-to-prompt rendering", func() {

	var builder *prompt.Builder

	BeforeEach(func() {
		var err error
		builder, err = prompt.NewBuilder()
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("UT-KA-779-EP-001: RenderInvestigation renders all DetectedLabels keys and values", func() {
		It("should include every key=value pair from DetectedLabels in the prompt", func() {
			enrichData := &prompt.EnrichmentData{
				DetectedLabels: map[string]string{
					"gitOpsManaged":            "true",
					"gitOpsTool":               "argocd",
					"hpaEnabled":               "true",
					"pdbProtected":             "true",
					"stateful":                 "true",
					"helmManaged":              "true",
					"networkIsolated":          "true",
					"serviceMesh":              "istio",
					"resourceQuotaConstrained": "true",
				},
			}

			rendered, err := builder.RenderInvestigation(prompt.SignalData{
				Name:      "api-server",
				Namespace: "production",
				Severity:  "critical",
				Message:   "OOMKilled",
			}, enrichData)
			Expect(err).NotTo(HaveOccurred())

			for key, value := range enrichData.DetectedLabels {
				Expect(rendered).To(ContainSubstring(key),
					"DetectedLabels key %q must appear in rendered prompt", key)
				Expect(rendered).To(ContainSubstring(value),
					"DetectedLabels value %q for key %q must appear in rendered prompt", value, key)
			}
		})
	})

	Describe("UT-KA-779-EP-002: RenderInvestigation renders QuotaDetails in the prompt", func() {
		It("should include quota resource names and usage in the prompt", func() {
			enrichData := &prompt.EnrichmentData{
				QuotaDetails: map[string]enrichment.QuotaResourceUsage{
					"cpu": {Hard: "4", Used: "3.5"},
					"memory": {Hard: "8Gi", Used: "7Gi"},
				},
			}

			rendered, err := builder.RenderInvestigation(prompt.SignalData{
				Name:      "api-server",
				Namespace: "production",
				Severity:  "critical",
				Message:   "OOMKilled",
			}, enrichData)
			Expect(err).NotTo(HaveOccurred())

			Expect(rendered).To(ContainSubstring("cpu"),
				"QuotaDetails resource 'cpu' must appear in rendered prompt")
			Expect(rendered).To(ContainSubstring("memory"),
				"QuotaDetails resource 'memory' must appear in rendered prompt")
			Expect(rendered).To(SatisfyAny(
				ContainSubstring("4"), ContainSubstring("Hard")),
				"Quota hard limit should appear in rendered prompt")
			Expect(rendered).To(SatisfyAny(
				ContainSubstring("3.5"), ContainSubstring("Used")),
				"Quota used value should appear in rendered prompt")
		})
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

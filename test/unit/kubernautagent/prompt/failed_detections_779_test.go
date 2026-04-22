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

// BR-WORKFLOW-016 / #779: FailedDetections must be forwarded to the prompt so
// the LLM knows which label detection categories were unavailable. Without this,
// the LLM silently treats missing labels as "not detected" when they may simply
// have failed to evaluate.

var _ = Describe("UT-KA-779-FD: FailedDetections in prompt rendering", func() {

	var builder *prompt.Builder

	BeforeEach(func() {
		var err error
		builder, err = prompt.NewBuilder()
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("UT-KA-779-FD-001: RenderInvestigation includes failedDetections in prompt when present", func() {
		It("should render failedDetections key in the detected labels section", func() {
			enrichData := &prompt.EnrichmentData{
				DetectedLabels: map[string]string{
					"gitOpsManaged":    "true",
					"failedDetections": "hpa,resourceQuota",
				},
			}

			rendered, err := builder.RenderInvestigation(prompt.SignalData{
				Name:      "crash-pod",
				Namespace: "production",
				Severity:  "critical",
				Message:   "OOMKilled",
			}, enrichData)
			Expect(err).NotTo(HaveOccurred())
			Expect(rendered).To(ContainSubstring("failedDetections"),
				"failedDetections key should appear in the rendered prompt")
			Expect(rendered).To(ContainSubstring("hpa"),
				"Failed detection categories should be visible in the prompt")
			Expect(rendered).To(ContainSubstring("resourceQuota"),
				"Failed detection categories should be visible in the prompt")
		})
	})

	Describe("UT-KA-779-FD-002: RenderInvestigation omits failedDetections from prompt when empty", func() {
		It("should not render failedDetections when no detections failed", func() {
			enrichData := &prompt.EnrichmentData{
				DetectedLabels: map[string]string{
					"gitOpsManaged": "true",
					"hpaEnabled":    "true",
				},
			}

			rendered, err := builder.RenderInvestigation(prompt.SignalData{
				Name:      "crash-pod",
				Namespace: "production",
				Severity:  "critical",
				Message:   "OOMKilled",
			}, enrichData)
			Expect(err).NotTo(HaveOccurred())
			Expect(rendered).NotTo(ContainSubstring("failedDetections"),
				"failedDetections should not appear when all detections succeeded")
		})
	})

	Describe("UT-KA-779-FD-003: RenderWorkflowSelection includes failedDetections in enrichment context", func() {
		It("should render failedDetections in workflow selection enrichment context", func() {
			enrichData := &prompt.EnrichmentData{
				DetectedLabels: map[string]string{
					"stateful":         "true",
					"failedDetections": "serviceMesh,networkPolicy",
				},
			}

			rendered, err := builder.RenderWorkflowSelection(prompt.WorkflowSelectionInput{
				Signal: prompt.SignalData{
					Name:      "crash-pod",
					Namespace: "production",
					Severity:  "critical",
					Message:   "OOMKilled",
				},
				RCASummary: "OOMKilled due to memory limit",
				EnrichData: enrichData,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(rendered).To(ContainSubstring("failedDetections"),
				"failedDetections key should appear in workflow selection enrichment context")
			Expect(rendered).To(ContainSubstring("serviceMesh"),
				"Failed detection categories should be visible in workflow selection prompt")
		})
	})
})

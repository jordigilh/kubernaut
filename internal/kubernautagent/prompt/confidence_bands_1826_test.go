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

// Package prompt_test: BR-KA-213 / Issue #1826 — KA's investigation-outcome
// confidence bands (Outcome A "resolved" / Outcome B "inconclusive" in
// phase3_workflow_selection.tmpl) were hardcoded as literal 0.7/0.5 text.
// RenderWorkflowSelection now templates the operator-configured (or
// default) values in, so the LLM is always instructed with the actual
// configured bar.
package prompt_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
)

var _ = Describe("RenderWorkflowSelection confidence bands — BR-KA-213 / Issue #1826", func() {
	baseSignal := prompt.SignalData{
		Name: "test-signal", Namespace: "default", Severity: "high", Message: "Test",
	}

	Describe("UT-KA-1826-001: default confidence bands render unchanged when unset (backward compatibility)", func() {
		It("should render the V1.0 0.7/0.5 defaults when the input fields are left zero", func() {
			builder, err := prompt.NewBuilder()
			Expect(err).NotTo(HaveOccurred())

			rendered, err := builder.RenderWorkflowSelection(prompt.WorkflowSelectionInput{
				Signal:     baseSignal,
				RCASummary: "OOMKilled root cause",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(rendered).To(ContainSubstring(`"resolved" (confidence >= 0.7)`),
				"Outcome A must default to the V1.0 0.7 resolved-floor when unset")
			Expect(rendered).To(ContainSubstring(`"inconclusive" (confidence < 0.5)`),
				"Outcome B must default to the V1.0 0.5 inconclusive-ceiling when unset")
		})
	})

	Describe("UT-KA-1826-002: operator-configured confidence bands are templated into the prompt", func() {
		It("should render the configured ResolvedConfidenceThreshold in Outcome A", func() {
			builder, err := prompt.NewBuilder()
			Expect(err).NotTo(HaveOccurred())

			rendered, err := builder.RenderWorkflowSelection(prompt.WorkflowSelectionInput{
				Signal:                      baseSignal,
				RCASummary:                  "OOMKilled root cause",
				ResolvedConfidenceThreshold: 0.85,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(rendered).To(ContainSubstring(`"resolved" (confidence >= 0.85)`),
				"Outcome A must reflect the operator-configured resolved threshold, not the hardcoded 0.7")
			Expect(rendered).ToNot(ContainSubstring(`>= 0.7)`))
		})

		It("should render the configured InconclusiveConfidenceThreshold in Outcome B", func() {
			builder, err := prompt.NewBuilder()
			Expect(err).NotTo(HaveOccurred())

			rendered, err := builder.RenderWorkflowSelection(prompt.WorkflowSelectionInput{
				Signal:                          baseSignal,
				RCASummary:                      "OOMKilled root cause",
				InconclusiveConfidenceThreshold: 0.3,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(rendered).To(ContainSubstring(`"inconclusive" (confidence < 0.3)`),
				"Outcome B must reflect the operator-configured inconclusive threshold, not the hardcoded 0.5")
			Expect(rendered).ToNot(ContainSubstring(`< 0.5)`))
		})
	})
})

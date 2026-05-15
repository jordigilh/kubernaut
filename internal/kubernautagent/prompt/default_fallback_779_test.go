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

// BR-WORKFLOW-016 / #779: withDefault fallbacks in builder.go must be documented
// through behavioral tests so silent substitution of defaults is explicitly tested
// rather than accidentally relied upon.

var _ = Describe("UT-KA-779-PD: Prompt builder default fallback behavior", func() {

	var builder *prompt.Builder

	BeforeEach(func() {
		var err error
		builder, err = prompt.NewBuilder()
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("UT-KA-779-PD-001: RenderInvestigation defaults ResourceKind to Pod when empty", func() {
		It("should render Pod as resource kind in the prompt", func() {
			rendered, err := builder.RenderInvestigation(prompt.SignalData{
				Name:      "crash-pod",
				Namespace: "default",
				Severity:  "critical",
				Message:   "OOMKilled",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(rendered).To(ContainSubstring("Pod"),
				"Empty ResourceKind should default to 'Pod' in rendered prompt")
		})
	})

	Describe("UT-KA-779-PD-002: RenderInvestigation defaults Environment to Namespace when empty", func() {
		It("should use namespace value as environment fallback", func() {
			rendered, err := builder.RenderInvestigation(prompt.SignalData{
				Name:      "crash-pod",
				Namespace: "staging-ns",
				Severity:  "critical",
				Message:   "OOMKilled",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(rendered).To(ContainSubstring("staging-ns"),
				"Empty Environment should fall back to the Namespace value")
		})
	})

	Describe("UT-KA-779-PD-003: RenderInvestigation defaults SignalMode to reactive when empty", func() {
		It("should not render proactive-specific sections in the prompt", func() {
			rendered, err := builder.RenderInvestigation(prompt.SignalData{
				Name:      "crash-pod",
				Namespace: "default",
				Severity:  "critical",
				Message:   "OOMKilled",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(rendered).NotTo(ContainSubstring("Proactive Signal Mode"),
				"Empty SignalMode should default to reactive -- proactive section must not render")
			Expect(rendered).NotTo(ContainSubstring("PROACTIVE MODE"),
				"Empty SignalMode should not trigger proactive mode heading")
		})
	})

	Describe("UT-KA-779-PD-004: RenderInvestigation defaults SignalSource to kubernaut-gateway when empty", func() {
		It("should render kubernaut-gateway as signal source", func() {
			rendered, err := builder.RenderInvestigation(prompt.SignalData{
				Name:      "crash-pod",
				Namespace: "default",
				Severity:  "critical",
				Message:   "OOMKilled",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(rendered).To(ContainSubstring("kubernaut-gateway"),
				"Empty SignalSource should default to 'kubernaut-gateway' in rendered prompt")
		})
	})

	Describe("UT-KA-779-PD-005: RenderInvestigation preserves real values over defaults", func() {
		It("should use provided ResourceKind, not default Pod", func() {
			rendered, err := builder.RenderInvestigation(prompt.SignalData{
				Name:         "crash-sts",
				Namespace:    "production",
				Severity:     "high",
				Message:      "CrashLoopBackOff",
				ResourceKind: "StatefulSet",
				Environment:  "staging",
				SignalMode:   "proactive",
				SignalSource: "prometheus-alertmanager",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(rendered).To(ContainSubstring("StatefulSet"),
				"Provided ResourceKind should appear in prompt")
			Expect(rendered).To(ContainSubstring("staging"),
				"Provided Environment should appear in prompt")
			Expect(rendered).To(ContainSubstring("Proactive Signal Mode"),
				"Proactive signal mode section should render when SignalMode=proactive")
			Expect(rendered).To(ContainSubstring("prometheus-alertmanager"),
				"Provided SignalSource should appear in prompt")
		})
	})

	Describe("UT-KA-779-PD-006: RenderWorkflowSelection defaults Severity to critical when empty", func() {
		It("should render critical as severity in workflow selection prompt", func() {
			rendered, err := builder.RenderWorkflowSelection(prompt.WorkflowSelectionInput{
				Signal: prompt.SignalData{
					Name:      "crash-pod",
					Namespace: "default",
					Message:   "OOMKilled",
				},
				RCASummary: "OOMKilled due to memory limit",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(rendered).To(ContainSubstring("critical"),
				"Empty Severity in workflow selection should default to 'critical'")
		})
	})

	Describe("UT-KA-779-PD-007: RenderWorkflowSelection defaults Environment to Namespace when empty", func() {
		It("should use namespace as environment fallback in workflow selection prompt", func() {
			rendered, err := builder.RenderWorkflowSelection(prompt.WorkflowSelectionInput{
				Signal: prompt.SignalData{
					Name:      "crash-pod",
					Namespace: "staging-ns",
					Severity:  "critical",
					Message:   "OOMKilled",
				},
				RCASummary: "OOMKilled due to memory limit",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(rendered).To(ContainSubstring("staging-ns"),
				"Empty Environment in workflow selection should fall back to Namespace")
		})
	})

	Describe("UT-KA-779-PD-008: RenderWorkflowSelection defaults ResourceKind to Pod when empty", func() {
		It("should render Pod as resource kind in workflow selection prompt", func() {
			rendered, err := builder.RenderWorkflowSelection(prompt.WorkflowSelectionInput{
				Signal: prompt.SignalData{
					Name:      "crash-pod",
					Namespace: "default",
					Severity:  "critical",
					Message:   "OOMKilled",
				},
				RCASummary: "OOMKilled due to memory limit",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(rendered).To(ContainSubstring("Pod"),
				"Empty ResourceKind in workflow selection should default to 'Pod'")
		})
	})
})

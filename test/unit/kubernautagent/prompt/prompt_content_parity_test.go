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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
)

var _ = Describe("Prompt Content Parity — TP-433-PARITY (#433)", func() {

	Describe("UT-KA-433-PB-001: Cluster context sections in investigation prompt", func() {
		It("should render namespace, resource kind, resource name, cluster, and environment in prompt when provided", func() {
			builder, err := prompt.NewBuilder()
			Expect(err).NotTo(HaveOccurred())

			rendered, err := builder.RenderInvestigation(prompt.SignalData{
				Name:         "web-deploy-oom",
				Namespace:    "finance-prod",
				Severity:     "critical",
				Message:      "OOMKilled",
				ResourceKind: "Deployment",
				ResourceName: "web-deploy",
				ClusterName:  "us-east-1-prod",
				Environment:  "production",
			}, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(rendered).To(ContainSubstring("finance-prod"))
			Expect(rendered).To(ContainSubstring("Deployment"))
			Expect(rendered).To(ContainSubstring("web-deploy"))
			Expect(rendered).To(ContainSubstring("us-east-1-prod"))
			Expect(rendered).To(ContainSubstring("production"))
		})
	})

	Describe("UT-KA-433-PB-002: Detected labels section rendered when populated", func() {
		It("should include detected labels in prompt when EnrichmentData.DetectedLabels is non-empty", func() {
			builder, err := prompt.NewBuilder()
			Expect(err).NotTo(HaveOccurred())

			enrichData := &prompt.EnrichmentData{
				DetectedLabels: map[string]string{
					"gitOpsManaged": "true",
					"hpaEnabled":    "true",
					"pdbProtected":  "false",
				},
			}
			rendered, err := builder.RenderInvestigation(prompt.SignalData{
				Name:      "web-deploy-oom",
				Namespace: "production",
				Severity:  "critical",
				Message:   "OOMKilled",
			}, enrichData)
			Expect(err).NotTo(HaveOccurred())
			Expect(rendered).To(ContainSubstring("Detected Labels"))
			Expect(rendered).To(ContainSubstring("gitOpsManaged"))
			Expect(rendered).To(ContainSubstring("hpaEnabled"))
		})
	})

	Describe("UT-KA-433-SM-001: Proactive signal mode renders proactive prompt sections", func() {
		It("should include proactive-specific language when SignalMode is proactive", func() {
			builder, err := prompt.NewBuilder()
			Expect(err).NotTo(HaveOccurred())

			rendered, err := builder.RenderInvestigation(prompt.SignalData{
				Name:       "mem-exhaustion-predicted",
				Namespace:  "production",
				Severity:   "warning",
				Message:    "Memory exhaustion predicted in 2h",
				SignalMode: "proactive",
			}, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(rendered).To(ContainSubstring("Proactive Signal Mode"))
			Expect(rendered).To(ContainSubstring("Anticipated Incident"))
		})

		It("should include proactive-specific language in workflow selection when SignalMode is proactive", func() {
			builder, err := prompt.NewBuilder()
			Expect(err).NotTo(HaveOccurred())

			rendered, err := builder.RenderWorkflowSelection(prompt.WorkflowSelectionInput{
				Signal: prompt.SignalData{
					Name:       "mem-exhaustion-predicted",
					Namespace:  "production",
					Severity:   "warning",
					Message:    "Memory exhaustion predicted in 2h",
					SignalMode: "proactive",
				},
				RCASummary: "Memory trending toward limit",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.ToLower(rendered)).To(ContainSubstring("proactive"))
			Expect(rendered).To(ContainSubstring("is predicted"))
		})
	})

	Describe("UT-KA-433-SM-002: Reactive signal mode renders reactive prompt sections", func() {
		It("should NOT include proactive-specific language when SignalMode is reactive", func() {
			builder, err := prompt.NewBuilder()
			Expect(err).NotTo(HaveOccurred())

			rendered, err := builder.RenderInvestigation(prompt.SignalData{
				Name:       "api-server-crash",
				Namespace:  "production",
				Severity:   "critical",
				Message:    "CrashLoopBackOff",
				SignalMode: "reactive",
			}, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(rendered).NotTo(ContainSubstring("Proactive Signal Mode"))
			Expect(rendered).NotTo(ContainSubstring("Anticipated Incident"))
		})
	})

	Describe("UT-KA-433-SM-003: Empty signal mode defaults to reactive", func() {
		It("should default to reactive prompt when SignalMode is empty", func() {
			builder, err := prompt.NewBuilder()
			Expect(err).NotTo(HaveOccurred())

			rendered, err := builder.RenderInvestigation(prompt.SignalData{
				Name:      "api-server-crash",
				Namespace: "production",
				Severity:  "critical",
				Message:   "CrashLoopBackOff",
			}, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(rendered).NotTo(ContainSubstring("Proactive Signal Mode"))
			Expect(rendered).NotTo(ContainSubstring("Anticipated Incident"))
		})
	})

	Describe("UT-KA-433-PB-003: Detected labels section omitted when nil", func() {
		It("should not include detected labels section when EnrichmentData.DetectedLabels is nil", func() {
			builder, err := prompt.NewBuilder()
			Expect(err).NotTo(HaveOccurred())

			rendered, err := builder.RenderInvestigation(prompt.SignalData{
				Name:      "web-deploy-oom",
				Namespace: "production",
				Severity:  "critical",
				Message:   "OOMKilled",
			}, &prompt.EnrichmentData{})
			Expect(err).NotTo(HaveOccurred())
			Expect(rendered).NotTo(ContainSubstring("Detected Labels"))
		})

		It("should not include detected labels section when enrichment is nil", func() {
			builder, err := prompt.NewBuilder()
			Expect(err).NotTo(HaveOccurred())

			rendered, err := builder.RenderInvestigation(prompt.SignalData{
				Name:      "web-deploy-oom",
				Namespace: "production",
				Severity:  "critical",
				Message:   "OOMKilled",
			}, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(rendered).NotTo(ContainSubstring("Detected Labels"))
		})
	})
})

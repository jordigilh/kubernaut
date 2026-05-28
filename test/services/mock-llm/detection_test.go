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
package mockllm_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

var _ = Describe("Scenario Detection Rules", func() {

	var registry *scenarios.Registry

	BeforeEach(func() {
		registry = scenarios.DefaultRegistry()
	})

	DescribeTable("UT-MOCK-021: Mock keyword detection (highest priority)",
		func(keyword, expectedScenario string) {
			ctx := &scenarios.DetectionContext{
				Content: "analyze this issue: " + keyword,
				AllText: "analyze this issue: " + keyword,
			}
			result := registry.Detect(ctx)
			Expect(result).NotTo(BeNil(), "expected scenario detection for keyword: %s", keyword)
			Expect(result.Scenario.Name()).To(Equal(expectedScenario))
		},
		Entry("UT-MOCK-021-001: mock_no_workflow_found", "mock_no_workflow_found", "no_workflow_found"),
		Entry("UT-MOCK-021-002: mock_low_confidence", "mock_low_confidence", "low_confidence"),
		Entry("UT-MOCK-021-003: mock_problem_resolved", "mock_problem_resolved", "problem_resolved"),
		Entry("UT-MOCK-021-004: mock_problem_resolved_contradiction", "mock_problem_resolved_contradiction", "problem_resolved_contradiction"),
		Entry("UT-MOCK-021-005: mock_not_reproducible → problem_resolved", "mock_not_reproducible", "problem_resolved"),
		Entry("UT-MOCK-021-006: mock_rca_incomplete", "mock_rca_incomplete", "rca_incomplete"),
		Entry("UT-MOCK-021-007: mock_max_retries_exhausted", "mock_max_retries_exhausted", "max_retries_exhausted"),
	)

	DescribeTable("UT-MOCK-022: Signal Name regex detection",
		func(signalContent, expectedScenario string) {
			ctx := &scenarios.DetectionContext{
				Content:    signalContent,
				AllText:    signalContent,
				SignalName: "", // Will be extracted inside Detect
			}
			result := registry.Detect(ctx)
			Expect(result).NotTo(BeNil(), "expected signal detection for: %s", signalContent)
			Expect(result.Scenario.Name()).To(Equal(expectedScenario))
		},
		Entry("UT-MOCK-022-001: OOMKilled → oomkilled", "- Signal Name: OOMKilled\n- Namespace: default", "oomkilled"),
		Entry("UT-MOCK-022-002: CrashLoopBackOff → crashloop", "- Signal Name: CrashLoopBackOff\n- Namespace: staging", "crashloop"),
		Entry("UT-MOCK-022-003: NodeNotReady → node_not_ready", "- Signal Name: NodeNotReady\n- Node: worker-1", "node_not_ready"),
		Entry("UT-MOCK-022-004: CertManagerCertNotReady → cert_not_ready", "- Signal Name: CertManagerCertNotReady\n- Namespace: cert-manager", "cert_not_ready"),
		Entry("UT-MOCK-022-005: MemoryExceedsLimit → oomkilled", "- Signal Name: MemoryExceedsLimit\n- Namespace: prod", "oomkilled"),
	)

	Describe("UT-MOCK-023: Test signal detection", func() {
		It("should detect 'test signal' keyword → test_signal", func() {
			ctx := &scenarios.DetectionContext{
				Content: "handle this test signal gracefully",
				AllText: "handle this test signal gracefully",
			}
			result := registry.Detect(ctx)
			Expect(result).NotTo(BeNil())
			Expect(result.Scenario.Name()).To(Equal("test_signal"))
		})
	})

	Describe("UT-MOCK-024: Proactive signal detection", func() {
		It("should detect proactive OOMKilled → oomkilled_predictive", func() {
			ctx := &scenarios.DetectionContext{
				Content:     "investigate in proactive mode.\n- Signal Name: OOMKilled\nThis is a predicted issue that has not yet occurred.",
				AllText:     "investigate in proactive mode.\n- Signal Name: OOMKilled\nThis is a predicted issue that has not yet occurred.",
				IsProactive: true,
			}
			result := registry.Detect(ctx)
			Expect(result).NotTo(BeNil())
			Expect(result.Scenario.Name()).To(Equal("oomkilled_predictive"))
		})

		It("should detect predictive_no_action keyword in proactive mode", func() {
			ctx := &scenarios.DetectionContext{
				Content:     "investigate in proactive mode.\n- Signal Name: OOMKilled\nmock_predictive_no_action\nhas not yet occurred.",
				AllText:     "investigate in proactive mode.\n- Signal Name: OOMKilled\nmock_predictive_no_action\nhas not yet occurred.",
				IsProactive: true,
			}
			result := registry.Detect(ctx)
			Expect(result).NotTo(BeNil())
			Expect(result.Scenario.Name()).To(Equal("predictive_no_action"))
		})
	})

	Describe("UT-MOCK-1292-001: Cross-namespace af_create_rr scenario extracts workload namespace", func() {
		DescribeTable("should select af_create_rr_cross_ns and extract the workload namespace",
			func(prompt, expectedNS, expectedName string) {
				ctx := &scenarios.DetectionContext{
					Content:         prompt,
					AllText:         prompt,
					LastUserContent: prompt,
				}
				result := registry.Detect(ctx)
				Expect(result).NotTo(BeNil())
				Expect(result.Scenario.Name()).To(Equal("af_create_rr_cross_ns"),
					"cross-namespace remediation prompt must match the cross-NS scenario")

				cfgScenario, ok := result.Scenario.(scenarios.ScenarioWithConfig)
				Expect(ok).To(BeTrue())
				cfg := cfgScenario.Config()
				Expect(cfg.ResourceNS).To(Equal(expectedNS),
					"ResourceNS must be the workload namespace extracted from the prompt, not the default controller namespace")
				Expect(cfg.ResourceName).To(Equal(expectedName))
			},
			Entry("UT-MOCK-1292-001a: single-line prompt with spaces",
				"cross-namespace remediation for deployment memory-eater in demo-workload namespace",
				"demo-workload", "memory-eater"),
			Entry("UT-MOCK-1292-001b: newline before namespace (ADK Gemini restructure)",
				"cross-namespace remediation for deployment memory-eater in\ndemo-workload namespace",
				"demo-workload", "memory-eater"),
			Entry("UT-MOCK-1292-001c: tab-separated tokens",
				"cross-namespace remediation for deployment\tmemory-eater\tin\tdemo-workload\tnamespace",
				"demo-workload", "memory-eater"),
			Entry("UT-MOCK-1292-001d: dynamic namespace with digits and hyphens",
				"cross-namespace remediation for deployment memory-eater in fp-e2e-1292-1779885620 namespace",
				"fp-e2e-1292-1779885620", "memory-eater"),
		)

		It("UT-MOCK-1292-002: stale extractedNS does not leak across calls when regex fails", func() {
			// First call succeeds — namespace extracted.
			ctx1 := &scenarios.DetectionContext{
				Content:         "cross-namespace remediation for deployment memory-eater in staging namespace",
				AllText:         "cross-namespace remediation for deployment memory-eater in staging namespace",
				LastUserContent: "cross-namespace remediation for deployment memory-eater in staging namespace",
			}
			r1 := registry.Detect(ctx1)
			Expect(r1).NotTo(BeNil())
			cfg1 := r1.Scenario.(scenarios.ScenarioWithConfig).Config()
			Expect(cfg1.ResourceNS).To(Equal("staging"))

			// Second call has the keyword but a malformed resource pattern — regex cannot extract NS.
			ctx2 := &scenarios.DetectionContext{
				Content:         "cross-namespace remediation — no structured deployment reference here",
				AllText:         "cross-namespace remediation — no structured deployment reference here",
				LastUserContent: "cross-namespace remediation — no structured deployment reference here",
			}
			r2 := registry.Detect(ctx2)
			Expect(r2).NotTo(BeNil())
			Expect(r2.Scenario.Name()).To(Equal("af_create_rr_cross_ns"))
			cfg2 := r2.Scenario.(scenarios.ScenarioWithConfig).Config()
			Expect(cfg2.ResourceNS).To(Equal("kubernaut-system"),
				"when regex fails, Config() must return the base default — not a stale value from a prior match")
		})
	})

	Describe("UT-MOCK-025: Default fallback when no rule matches", func() {
		It("should return default scenario for unrecognized content", func() {
			ctx := &scenarios.DetectionContext{
				Content: "some completely unknown content with no keywords",
				AllText: "some completely unknown content with no keywords",
			}
			result := registry.Detect(ctx)
			Expect(result).NotTo(BeNil())
			Expect(result.Scenario.Name()).To(Equal("default"))
		})
	})
})

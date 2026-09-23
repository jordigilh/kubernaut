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

package infrastructure

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	utilyaml "sigs.k8s.io/yaml"
)

var _ = Describe("FullPipeline A2A severity grounding rules [BR-SEVERITY-001, BR-INTERACTIVE-010, BR-TESTING-001]", func() {
	It("UT-FP-2443-001: scopes firing triage evidence to each A2A namespace and suppresses Gateway relay", func() {
		rulesYAML, ruleNames := fullPipelineA2AGroundingRuleFile(map[string]string{
			"combined-investigate": "fp-combined-1234",
			"interactive":          "fp-interactive-abcd",
		})

		Expect(ruleNames).To(Equal([]string{
			fullPipelineA2AGroundingRulePrefix + "fp_combined_1234",
			fullPipelineA2AGroundingRulePrefix + "fp_interactive_abcd",
		}))

		var ruleFile struct {
			Groups []struct {
				Name  string `yaml:"name"`
				Rules []struct {
					Alert  string            `yaml:"alert"`
					Expr   string            `yaml:"expr"`
					For    string            `yaml:"for"`
					Labels map[string]string `yaml:"labels"`
				} `yaml:"rules"`
			} `yaml:"groups"`
		}
		Expect(utilyaml.Unmarshal([]byte(rulesYAML), &ruleFile)).To(Succeed())
		Expect(ruleFile.Groups).To(HaveLen(1))
		Expect(ruleFile.Groups[0].Name).To(Equal("fullpipeline-a2a-severity-grounding"))
		Expect(ruleFile.Groups[0].Rules).To(HaveLen(2))

		rulesByNamespace := make(map[string]struct {
			Alert  string
			Expr   string
			For    string
			Labels map[string]string
		})
		for _, rule := range ruleFile.Groups[0].Rules {
			rulesByNamespace[rule.Labels["namespace"]] = struct {
				Alert  string
				Expr   string
				For    string
				Labels map[string]string
			}{Alert: rule.Alert, Expr: rule.Expr, For: rule.For, Labels: rule.Labels}
		}

		for namespace, alertName := range map[string]string{
			"fp-combined-1234":    fullPipelineA2AGroundingRulePrefix + "fp_combined_1234",
			"fp-interactive-abcd": fullPipelineA2AGroundingRulePrefix + "fp_interactive_abcd",
		} {
			rule, found := rulesByNamespace[namespace]
			Expect(found).To(BeTrue(), "expected one grounding rule for namespace %s", namespace)
			Expect(rule.Alert).To(Equal(alertName))
			Expect(rule.Expr).To(Equal("vector(1) > 0"))
			Expect(rule.For).To(Equal("0s"))
			Expect(rule.Labels).To(HaveKeyWithValue("namespace", namespace))
			Expect(rule.Labels).To(HaveKeyWithValue("severity", "warning"))
			Expect(rule.Labels).NotTo(HaveKey("kind"))
			Expect(rule.Labels).NotTo(HaveKey("name"))
			Expect(rule.Labels).To(HaveKeyWithValue(SkipGatewayRouteLabelKey, SkipGatewayRouteLabelValue))
		}
	})

	It("UT-FP-2443-002: returns no rules when there are no fullpipeline A2A targets", func() {
		rulesYAML, ruleNames := fullPipelineA2AGroundingRuleFile(nil)

		Expect(rulesYAML).To(BeEmpty())
		Expect(ruleNames).To(BeEmpty())
	})
})

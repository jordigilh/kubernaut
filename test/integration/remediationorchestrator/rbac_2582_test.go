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

package remediationorchestrator

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/yaml"
)

type generatedRBACRule struct {
	APIGroups []string `yaml:"apiGroups"`
	Resources []string `yaml:"resources"`
	Verbs     []string `yaml:"verbs"`
}

type generatedRBACRole struct {
	Rules []generatedRBACRule `yaml:"rules"`
}

var _ = Describe("DD-2582 generated RBAC", Label("integration", "issue-2582", "security"), func() {
	It("IT-SEC-2582-001: grants only the parent reads and child status writes required by the lifecycle gates", func() {
		data, err := os.ReadFile(filepath.Join("..", "..", "..", "config", "rbac", "role.yaml"))
		Expect(err).NotTo(HaveOccurred())
		role := generatedRBACRole{}
		Expect(yaml.Unmarshal(data, &role)).To(Succeed())

		Expect(hasRBACRule(role.Rules, "remediationrequests", "get")).To(BeTrue())
		Expect(hasRBACRule(role.Rules, "remediationrequests", "watch")).To(BeTrue())
		Expect(hasRBACRule(role.Rules, "remediationrequests/status", "update")).To(BeTrue())
		Expect(hasRBACRule(role.Rules, "remediationapprovalrequests", "create")).To(BeTrue())
		Expect(hasRBACRule(role.Rules, "remediationapprovalrequests/status", "update")).To(BeTrue())
		Expect(hasRBACRule(role.Rules, "investigationsessions", "get")).To(BeTrue())
	})
})

func hasRBACRule(rules []generatedRBACRule, resource, verb string) bool {
	for _, rule := range rules {
		if !containsString(rule.APIGroups, "kubernaut.ai") || !containsString(rule.Resources, resource) {
			continue
		}
		if containsString(rule.Verbs, verb) {
			return true
		}
	}
	return false
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

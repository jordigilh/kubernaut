/*
Copyright 2025 Jordi Gil.

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
	"os"
	"path/filepath"
	"slices"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// parseClusterRoleRules reads a multi-document YAML file and returns the rules
// for the named ClusterRole. Returns nil if not found.
func parseClusterRoleRules(path, name string) ([]rbacv1.PolicyRule, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: known project path
	if err != nil {
		return nil, err
	}
	for _, doc := range strings.Split(string(data), "---") {
		doc = strings.TrimSpace(doc)
		if doc == "" || !strings.Contains(doc, "kind: ClusterRole") {
			continue
		}
		var cr rbacv1.ClusterRole
		if err := yaml.NewYAMLOrJSONDecoder(strings.NewReader(doc), 4096).Decode(&cr); err != nil {
			return nil, err
		}
		if cr.Name == name {
			return cr.Rules, nil
		}
	}
	return nil, nil
}

// UT-INFRA-RBAC-001: Structural parity test for 02-rbac.yaml.
//
// Both AF E2E and FP E2E read deploy/apifrontend/base/02-rbac.yaml at
// runtime (via afDeployE2ERBAC). This test ensures the base file always
// contains the minimum RBAC rules the API Frontend needs to operate.
// If a rule is removed from the base file, this test catches it before
// E2E suites fail with cryptic "forbidden" errors.
var _ = Describe("AF RBAC parity (UT-INFRA-RBAC-001)", func() {
	var rules []rbacv1.PolicyRule

	BeforeEach(func() {
		rbacPath := filepath.Join(getProjectRoot(), "deploy", "apifrontend", "base", "02-rbac.yaml")
		var err error
		rules, err = parseClusterRoleRules(rbacPath, "apifrontend")
		Expect(err).NotTo(HaveOccurred(), "02-rbac.yaml must be parseable")
		Expect(rules).NotTo(BeEmpty(), "ClusterRole 'apifrontend' must have rules")
	})

	DescribeTable("required rule present",
		func(apiGroup, resource string, verbs []string) {
			for _, rule := range rules {
				if !slices.Contains(rule.APIGroups, apiGroup) || !slices.Contains(rule.Resources, resource) {
					continue
				}
				for _, v := range verbs {
					Expect(rule.Verbs).To(ContainElement(v),
						"rule %s/%s must include verb %q", apiGroup, resource, v)
				}
				return
			}
			Fail("02-rbac.yaml must contain a rule for " + apiGroup + "/" + resource)
		},
		Entry("IS CRD",              "kubernaut.ai", "investigationsessions",          []string{"get", "list", "watch", "create", "update", "delete"}),
		Entry("IS status",           "kubernaut.ai", "investigationsessions/status",    []string{"get", "update"}),
		Entry("RR CRD",              "kubernaut.ai", "remediationrequests",             []string{"get", "list", "watch", "create", "update", "patch"}),
		Entry("RAR CRD",             "kubernaut.ai", "remediationapprovalrequests",     []string{"get", "list", "create", "update", "patch"}),
		Entry("RAR status",          "kubernaut.ai", "remediationapprovalrequests/status", []string{"get", "update", "patch"}),
		Entry("events",              "",             "events",                          []string{"get", "list", "create", "patch"}),
		Entry("pods",                "",             "pods",                            []string{"get", "list"}),
		Entry("services",            "",             "services",                        []string{"get", "list"}),
		Entry("endpoints",           "",             "endpoints",                       []string{"get", "list"}),
		Entry("configmaps",          "",             "configmaps",                      []string{"get", "list"}),
		Entry("secrets",             "",             "secrets",                         []string{"get", "list"}),
		Entry("namespaces",          "",             "namespaces",                      []string{"get", "list"}),
		Entry("nodes",               "",             "nodes",                           []string{"get", "list"}),
		Entry("pvcs",                "",             "persistentvolumeclaims",          []string{"get", "list"}),
		Entry("deployments",         "apps",         "deployments",                     []string{"get", "list"}),
		Entry("jobs",                "batch",        "jobs",                            []string{"get", "list"}),
		Entry("cronjobs",            "batch",        "cronjobs",                        []string{"get", "list"}),
		Entry("ingresses",           "networking.k8s.io", "ingresses",                  []string{"get", "list"}),
		Entry("networkpolicies",     "networking.k8s.io", "networkpolicies",             []string{"get", "list"}),
		Entry("hpas",                "autoscaling",  "horizontalpodautoscalers",         []string{"get", "list"}),
		Entry("pdbs",                "policy",       "poddisruptionbudgets",             []string{"get", "list"}),
		Entry("leases",              "coordination.k8s.io", "leases",                   []string{"get", "list", "watch"}),
		Entry("aianalyses",          "kubernaut.ai", "aianalyses",                      []string{"get", "list", "watch"}),
		Entry("IT-AF-1460-040: EA CRD", "kubernaut.ai", "effectivenessassessments",     []string{"get", "list", "watch"}),
		Entry("SAR",                 "authorization.k8s.io", "subjectaccessreviews",    []string{"create"}),
		Entry("token reviews",       "authentication.k8s.io", "tokenreviews",           []string{"create"}),
	)
})

var _ = Describe("IT-AF-1460-021: StatusHandler production wiring", func() {
	It("StatusHandler is constructed in cmd/apifrontend/main.go", func() {
		mainPath := filepath.Join(getProjectRoot(), "cmd", "apifrontend", "main.go")
		data, err := os.ReadFile(mainPath) //nolint:gosec // G304: known project path
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(ContainSubstring("NewStatusHandler"),
			"cmd/apifrontend/main.go must construct StatusHandler")
		Expect(string(data)).To(ContainSubstring("StatusHandler:"),
			"cmd/apifrontend/main.go must wire StatusHandler into RouterConfig")
	})
})

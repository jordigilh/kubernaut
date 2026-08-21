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

// afPersonaGroupNamesForParity mirrors afPersonaGroupNames
// (fullpipeline_e2e_helm.go) -- the fixed, rarely-changing list of persona
// group names AF's SAR authorization recognizes. Duplicated here (rather
// than importing afPersonaGroupNames directly) only to keep this parity
// test decoupled from an unrelated file's private variable; both must stay
// in sync with charts/kubernaut/values.yaml's apifrontend.config.rbac.personas keys.
var afPersonaGroupNamesForParity = []string{
	"sre", "ai-orchestrator", "cicd", "observability", "l3-audit", "remediation-approver",
}

// parseClusterRoleRules reads a multi-document YAML file and returns the rules
// for the named ClusterRole. Returns nil if not found.
func parseClusterRoleRules(path, name string) ([]rbacv1.PolicyRule, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: known project path
	if err != nil {
		return nil, err
	}
	return parseClusterRoleRulesFromYAML(data, name)
}

// parseClusterRoleRulesFromYAML is parseClusterRoleRules' shared core, split
// out so callers already holding in-memory multi-document YAML (e.g.
// PersonaToolClusterRolesYAML's generated output) don't need a scratch file.
func parseClusterRoleRulesFromYAML(data []byte, name string) ([]rbacv1.PolicyRule, error) {
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
		// #2214: AgentSessionTerminalCloseReconciler watches AgentSession and
		// adds/removes its own metadata-only finalizer. This entry was missing
		// when 02-rbac.yaml drifted from charts/kubernaut/templates/apifrontend/
		// apifrontend.yaml (CI run 32529373504, "E2E (apifrontend)"): the
		// AgentSession informer could never complete its initial List, so the
		// cache-sync timeout crashed the session controller manager mid-run.
		Entry("agentsessions (#2214)", "kubernaut.ai", "agentsessions",                 []string{"get", "list", "watch", "update", "patch"}),
		Entry("SAR",                 "authorization.k8s.io", "subjectaccessreviews",    []string{"create"}),
		Entry("token reviews",       "authentication.k8s.io", "tokenreviews",           []string{"create"}),
	)

	// IT-AF-1409-008 [AC-6]: #1409's takeover-path context-reconstruction fetch
	// (ka_investigate_mcp.go's resolveInvestigationRR) relies on a client.Get()
	// against RemediationRequest. This regression-pins that the ClusterRole
	// already grants "get" (least privilege — no broader verb is required for
	// the new fetch), so a future RBAC edit that narrows this rule fails CI
	// before it breaks the takeover-path Console banner in production.
	It("IT-AF-1409-008: AC-6 — ClusterRole grants get (not list/watch/delete) on remediationrequests for the takeover-path fetch", func() {
		var rrRule *rbacv1.PolicyRule
		for i := range rules {
			if slices.Contains(rules[i].APIGroups, "kubernaut.ai") && slices.Contains(rules[i].Resources, "remediationrequests") {
				rrRule = &rules[i]
				break
			}
		}
		Expect(rrRule).NotTo(BeNil(), "02-rbac.yaml must contain a rule for kubernaut.ai/remediationrequests")
		Expect(rrRule.Verbs).To(ContainElement("get"),
			"AC-6: least privilege — AF's takeover-path client.Get() fetch requires the 'get' verb on remediationrequests")
	})
})

// UT-INFRA-RBAC-002: Structural parity test for PersonaToolClusterRolesYAML.
//
// Incident (2026-08-03): #1869/#1884 added kubernaut_get_approval_request and
// kubernaut_complete_no_action to the sre persona in values.yaml (plus other
// tools added over #1367/#1372's history for other personas), but
// PersonaToolClusterRolesYAML's hand-copied tool-list literals silently
// drifted out of sync for 5 of 6 personas -- with zero build/lint signal,
// because both sides compiled and ran fine independently. This left
// test/e2e/apifrontend/interactive_wiring_e2e_test.go's E2E-KA-1418-001/002
// (which call kubernaut_complete_no_action as the sre persona through AF's
// real /mcp endpoint) failing on SAR denial in that suite's Kind cluster,
// even though the same calls work correctly in the fullpipeline E2E suite
// (which binds ClusterRoleBindings to the chart's own Helm-rendered
// ClusterRoles instead of a second hand-copied list -- see
// bindAFPersonaToolClusterRoles's doc comment, fullpipeline_e2e_helm.go).
//
// This test pins PersonaToolClusterRolesYAML's generated tool set to exactly
// match values.yaml's apifrontend.config.rbac.personas (via
// LoadPersonaToolsFromValuesYAML), so any future persona/tool change that
// isn't reflected in both places fails CI instead of silently breaking the
// AF-only E2E suite.
var _ = Describe("AF persona tool RBAC parity (UT-INFRA-RBAC-002)", func() {
	var (
		personaTools  map[string][]string
		generatedYAML string
	)

	BeforeEach(func() {
		var err error
		personaTools, err = LoadPersonaToolsFromValuesYAML()
		Expect(err).NotTo(HaveOccurred(), "values.yaml apifrontend.config.rbac.personas must be parseable")
		Expect(personaTools).NotTo(BeEmpty(), "values.yaml must define at least one persona")

		generatedYAML, err = PersonaToolClusterRolesYAML()
		Expect(err).NotTo(HaveOccurred(), "PersonaToolClusterRolesYAML must succeed")
	})

	DescribeTable("generated ClusterRole tool set matches values.yaml persona",
		func(persona string) {
			expectedTools, ok := personaTools[persona]
			Expect(ok).To(BeTrue(), "values.yaml apifrontend.config.rbac.personas must define persona %q", persona)

			rules, err := parseClusterRoleRulesFromYAML([]byte(generatedYAML), "kubernaut-tool-"+persona)
			Expect(err).NotTo(HaveOccurred())
			Expect(rules).NotTo(BeEmpty(), "generated ClusterRole kubernaut-tool-%s must have rules", persona)

			var actualTools []string
			for _, r := range rules {
				actualTools = append(actualTools, r.ResourceNames...)
			}
			Expect(actualTools).To(ConsistOf(toAnySlice(expectedTools)...),
				"kubernaut-tool-%s ClusterRole tool set drifted from values.yaml apifrontend.config.rbac.personas.%s -- "+
					"PersonaToolClusterRolesYAML must derive from values.yaml, not a hand-copied literal", persona, persona)
		},
		Entry("sre", "sre"),
		Entry("ai-orchestrator", "ai-orchestrator"),
		Entry("cicd", "cicd"),
		Entry("observability", "observability"),
		Entry("l3-audit", "l3-audit"),
		Entry("remediation-approver", "remediation-approver"),
	)

	It("covers every persona group name AF's SAR authorization recognizes", func() {
		for _, name := range afPersonaGroupNamesForParity {
			Expect(personaTools).To(HaveKey(name),
				"values.yaml apifrontend.config.rbac.personas must define persona %q referenced by afPersonaGroupNames", name)
		}
	})
})

// toAnySlice adapts a []string to []interface{} for Gomega's ConsistOf,
// which needs variadic interface{} elements rather than a typed slice.
func toAnySlice(s []string) []interface{} {
	out := make([]interface{}, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}

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

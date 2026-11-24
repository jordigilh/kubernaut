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
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// Issue #2703 (umbrella #2701, operator kubernaut-operator#682): after the
// operator RBAC decoupling the Kubernaut CR no longer accepts
// spec.apiFrontend.rbac.roleBindings or consoleAccessGroups, the operator
// renders roles only (console-access, console-reader,
// console-inventory-reader -- all deliberately unbound), and the six built-in
// persona ClusterRoles are gone. The demo owns role composition and external
// bindings as its SOP. These tests pin that contract.
//
// Business requirement: BR-PLATFORM-014 (demo CR contract). Control
// objectives: FedRAMP AC-6 (least privilege -- capability SARs are
// deny-by-default, so a capability exists only where a demo-owned role grants
// it), SOC2 CC6.1 (logical access controls are external to the platform CR).

func decodeClusterRoleBindings(data string) []rbacv1.ClusterRoleBinding {
	var crbs []rbacv1.ClusterRoleBinding
	for _, doc := range strings.Split(data, "---") {
		doc = strings.TrimSpace(doc)
		if doc == "" || !strings.Contains(doc, "kind: ClusterRoleBinding") {
			continue
		}
		var crb rbacv1.ClusterRoleBinding
		Expect(yaml.NewYAMLOrJSONDecoder(strings.NewReader(doc), 4096).Decode(&crb)).To(Succeed())
		crbs = append(crbs, crb)
	}
	return crbs
}

// UT-INFRA-2703-001 [BR-PLATFORM-014, AC-6]: the demo console RBAC manifest
// binds exactly the three operator-rendered, deliberately-unbound ClusterRoles
// to the demo IdP group and adds no roles and no extra grants of its own --
// the roles-only model means a binding is the only demo-owned authorization
// artifact, and every capability SAR stays deny-by-default without it.
var _ = Describe("Demo console RBAC manifest (UT-INFRA-2703-001)", func() {
	const (
		demoNS   = "kubernaut-system"
		demoGrp  = "sre"
		crbCount = 3
	)

	manifest := func() []rbacv1.ClusterRoleBinding {
		return decodeClusterRoleBindings(BuildDemoConsoleRBACManifest(demoNS, demoGrp))
	}

	It("binds exactly the operator-rendered console roles to the demo group", func() {
		crbs := manifest()
		Expect(crbs).To(HaveLen(crbCount))
		roleRefs := map[string]string{}
		for _, crb := range crbs {
			Expect(crb.RoleRef.Kind).To(Equal("ClusterRole")) // BR-PLATFORM-014: operator-rendered role
			Expect(crb.Subjects).To(HaveLen(1))
			Expect(crb.Subjects[0].Kind).To(Equal("Group")) // BR-PLATFORM-014: external IdP group binding
			Expect(crb.Subjects[0].Name).To(Equal(demoGrp))
			roleRefs[crb.RoleRef.Name] = crb.Name
		}
		Expect(roleRefs).To(HaveKey(demoNS + "-console-access"))
		Expect(roleRefs).To(HaveKey(demoNS + "-console-reader"))
		Expect(roleRefs).To(HaveKey(demoNS + "-console-inventory-reader"))
	})

	It("grants nothing itself: bindings only, never roles (deny-by-default AC-6)", func() {
		Expect(BuildDemoConsoleRBACManifest(demoNS, demoGrp)).NotTo(ContainSubstring("\nkind: ClusterRole\n"),
			"the demo must not render ClusterRoles for console access; the operator owns the roles")
	})
})

// UT-INFRA-2703-002 [BR-PLATFORM-014]: the rendered demo Kubernaut CR must
// stay compatible with the post-#682 CRD: no roleBindings, no
// consoleAccessGroups -- role composition is the demo operator's SOP, applied
// as external ClusterRoleBindings after reconciliation.
var _ = Describe("Demo CR roles-only compatibility (UT-INFRA-2703-002)", func() {
	It("renders no removed RBAC CRD fields and enables the console-access gate", func() {
		opts := buildDemoOperatorCROptions(DemoHelmOptions{
			LLMProvider: "openai", LLMModel: "mock-model", LLMEndpoint: "http://mock-llm.kubernaut-system.svc.cluster.local:8080",
		}, nil)
		Expect(opts.ConsoleAccessAuthorizationCheckEnabled).NotTo(BeNil())
		Expect(*opts.ConsoleAccessAuthorizationCheckEnabled).To(BeTrue(),
			"the demo binds console-access externally, so the gate's authorization check must be enforced")

		manifest, err := BuildKubernautCRManifest(opts)
		Expect(err).NotTo(HaveOccurred())
		Expect(manifest).NotTo(ContainSubstring("roleBindings"),
			"the post-#682 CRD rejects spec.apiFrontend.rbac.roleBindings")
		Expect(manifest).NotTo(ContainSubstring("consoleAccessGroups"),
			"the post-#682 CRD rejects spec.apiFrontend.rbac.consoleAccessGroups")
		Expect(manifest).To(ContainSubstring("consoleAccessAuthorizationCheckEnabled: true"))
	})
})

// UT-INFRA-2703-003 [BR-PLATFORM-014]: the static Fleet qualification CR
// fixture must not carry fields the post-#682 CRD rejects.
var _ = Describe("Fleet CR fixture roles-only compatibility (UT-INFRA-2703-003)", func() {
	It("contains no removed RBAC CRD fields", func() {
		data, err := os.ReadFile(filepath.Join(getProjectRoot(), "test", "fixtures", "operator", "kubernaut-fleet.yaml")) //nolint:gosec // G304: known project path
		Expect(err).NotTo(HaveOccurred())

		var fixture struct {
			Spec struct {
				APIFrontend struct {
					RBAC map[string]any `yaml:"rbac"`
				} `yaml:"apiFrontend"`
			} `yaml:"spec"`
		}
		Expect(yaml.NewYAMLOrJSONDecoder(strings.NewReader(string(data)), 4096).Decode(&fixture)).To(Succeed())
		Expect(fixture.Spec.APIFrontend.RBAC).NotTo(HaveKey("roleBindings"),
			"kubernaut-fleet.yaml must not set spec.apiFrontend.rbac.roleBindings (removed by operator #682)")
		Expect(fixture.Spec.APIFrontend.RBAC).NotTo(HaveKey("consoleAccessGroups"),
			"kubernaut-fleet.yaml must not set spec.apiFrontend.rbac.consoleAccessGroups (removed by operator #682)")
	})
})

// UT-INFRA-2703-004 [AC-6]: the demo-owned persona tool-role manifest (the
// human SOP artifact replacing the operator's retired built-in personas) must
// stay in parity with the shared persona RBAC contract the harness applies
// (LoadPersonaTools), plus the operator #682/#683 both-names rule: alert-
// capable bundles carry list_alerts, get_alert_details AND
// kubernaut_list_alerts, because AF authorizes the invoked tool name and the
// raw MCP bridge names the alert list tool differently than the A2A surface.
var _ = Describe("Demo persona tool role manifest parity (UT-INFRA-2703-004)", func() {
	const manifestPath = "test/fixtures/demo/rbac/persona-tool-roles.yaml"

	alertCapablePersonas := map[string]bool{"sre": true, "ai-orchestrator": true}

	It("covers every persona with a ClusterRole and an external group binding", func() {
		data, err := os.ReadFile(filepath.Join(getProjectRoot(), manifestPath)) //nolint:gosec // G304: known project path
		Expect(err).NotTo(HaveOccurred())

		personaTools, err := LoadPersonaTools()
		Expect(err).NotTo(HaveOccurred())

		var crbs []rbacv1.ClusterRoleBinding
		for _, doc := range strings.Split(string(data), "---") {
			doc = strings.TrimSpace(doc)
			if doc == "" {
				continue
			}
			if strings.Contains(doc, "kind: ClusterRoleBinding") {
				var crb rbacv1.ClusterRoleBinding
				Expect(yaml.NewYAMLOrJSONDecoder(strings.NewReader(doc), 4096).Decode(&crb)).To(Succeed())
				crbs = append(crbs, crb)
				continue
			}
			if !strings.Contains(doc, "kind: ClusterRole") {
				continue // leading comment block / separator
			}
			var cr rbacv1.ClusterRole
			Expect(yaml.NewYAMLOrJSONDecoder(strings.NewReader(doc), 4096).Decode(&cr)).To(Succeed())
			Expect(cr.Rules).To(HaveLen(1), "persona role %s must have exactly one rule", cr.Name)
			Expect(cr.Rules[0].APIGroups).To(ConsistOf("kubernaut.ai"))
			Expect(cr.Rules[0].Resources).To(ConsistOf("tools"))
			Expect(cr.Rules[0].Verbs).To(ConsistOf("use"))

			persona := strings.TrimPrefix(cr.Name, "demo-tool-")
			Expect(persona).NotTo(Equal(cr.Name), "persona role names must use the demo-tool-<persona> convention")
			expected, ok := personaTools[persona]
			Expect(ok).To(BeTrue(), "demo manifest persona %q must exist in the shared persona contract", persona)
			if alertCapablePersonas[persona] {
				expected = append(append([]string(nil), expected...), "kubernaut_list_alerts")
			}
			Expect(cr.Rules[0].ResourceNames).To(ConsistOf(toAnySlice(expected)...),
				"demo-tool-%s drifted from the shared persona contract (plus the both-names alert rule)", persona)
		}

		Expect(crbs).To(HaveLen(len(personaTools)))
		for _, crb := range crbs {
			persona := strings.TrimSuffix(strings.TrimPrefix(crb.Name, "demo-tool-"), "-binding")
			Expect(crb.RoleRef.Name).To(Equal("demo-tool-" + persona))
			Expect(crb.RoleRef.Kind).To(Equal("ClusterRole")) // BR-PLATFORM-014: demo-owned persona role
			Expect(crb.Subjects).To(HaveLen(1))
			Expect(crb.Subjects[0].Kind).To(Equal("Group")) // BR-PLATFORM-014: external IdP group binding
			Expect(crb.Subjects[0].Name).To(Equal(persona),
				"the demo IdP issues one group per persona (keycloak-realm-demo.json)")
		}
	})

	// Fixture ground-truth fidelity: every group the persona bindings target
	// must actually exist in the demo Keycloak realm, or a human following
	// the README applies bindings that can never match a token's groups
	// claim (five of six would be inert).
	It("issues every persona group the demo bindings target", func() {
		data, err := os.ReadFile(filepath.Join(getProjectRoot(), "test", "infrastructure", "keycloak-realm-demo.json")) //nolint:gosec // G304: known project path
		Expect(err).NotTo(HaveOccurred())

		var realm struct {
			Groups []struct {
				Name string `json:"name"`
			} `json:"groups"`
		}
		Expect(json.Unmarshal(data, &realm)).To(Succeed())

		realmGroups := map[string]bool{}
		for _, g := range realm.Groups {
			realmGroups[g.Name] = true
		}
		for persona := range alertCapablePersonas {
			Expect(realmGroups).To(HaveKey(persona))
		}
		for _, persona := range []string{"cicd", "observability", "l3-audit", "remediation-approver"} {
			Expect(realmGroups).To(HaveKey(persona))
		}
	})
})

// UT-INFRA-2703-005 [BR-PLATFORM-014, AC-6]: the demo-owned console binding
// manifest (human SOP artifact) must reference the operator-rendered,
// deliberately-unbound role names for the demo namespace and bind them to the
// demo IdP group, mirroring BuildDemoConsoleRBACManifest exactly.
var _ = Describe("Demo console binding manifest (UT-INFRA-2703-005)", func() {
	It("mirrors the harness-generated console bindings", func() {
		data, err := os.ReadFile(filepath.Join(getProjectRoot(), "test/fixtures/demo/rbac/console-bindings.yaml")) //nolint:gosec // G304: known project path
		Expect(err).NotTo(HaveOccurred())

		harnessCRBs := decodeClusterRoleBindings(BuildDemoConsoleRBACManifest(kubernautSystem, "sre"))
		fixtureCRBs := decodeClusterRoleBindings(string(data))
		Expect(fixtureCRBs).To(HaveLen(len(harnessCRBs)))

		harnessByRoleRef := map[string]rbacv1.ClusterRoleBinding{}
		for _, crb := range harnessCRBs {
			harnessByRoleRef[crb.RoleRef.Name] = crb
		}
		for _, crb := range fixtureCRBs {
			harness, ok := harnessByRoleRef[crb.RoleRef.Name]
			Expect(ok).To(BeTrue(), "fixture binds role %q the harness does not", crb.RoleRef.Name)
			// BR-PLATFORM-014: same CRB name and demo-rbac label, so the
			// documented teardown (label-selector delete) covers both paths.
			Expect(crb.Name).To(Equal(harness.Name),
				"fixture CRB name must mirror the harness-generated name")
			Expect(crb.Labels["kubernaut.ai/demo-rbac"]).To(Equal(harness.Labels["kubernaut.ai/demo-rbac"]),
				"fixture must carry the same demo-rbac teardown label")
			Expect(crb.Subjects).To(Equal(harness.Subjects),
				"fixture must bind the same (role, group) pair the demo harness applies")
		}
	})
})

// UT-INFRA-2703-006 (CHECKPOINT W wiring proof): the demo deployment path and
// the manual make target must both apply the demo-owned console bindings --
// the operator no longer renders any user-facing ClusterRoleBinding
// (kubernaut-operator#682), so without this call the console-access gate and
// console read capabilities are deny-by-default for every demo user. The
// regexp anchors on an actual (non-commented) call statement; runtime
// dispatch proof is the SAR wait inside BindDemoConsoleRBAC itself.
var wiringCallPattern = regexp.MustCompile(`(?m)^\s*(if err := )?(infrastructure\.)?BindDemoConsoleRBAC\(`)

var _ = Describe("Demo console RBAC wiring (UT-INFRA-2703-006)", func() {
	It("finalizeDemoOperatorApplication applies the demo console bindings", func() {
		data, err := os.ReadFile(filepath.Join(getProjectRoot(), "test", "infrastructure", "demo_operator.go")) //nolint:gosec // G304: known project path
		Expect(err).NotTo(HaveOccurred())
		Expect(wiringCallPattern.FindString(string(data))).NotTo(BeEmpty(),
			"demo_operator.go must call BindDemoConsoleRBAC (an actual call, not a comment) after operator reconciliation")
	})

	It("hack/bind-fleet-af-rbac applies the demo console bindings", func() {
		data, err := os.ReadFile(filepath.Join(getProjectRoot(), "hack", "bind-fleet-af-rbac", "main.go")) //nolint:gosec // G304: known project path
		Expect(err).NotTo(HaveOccurred())
		Expect(wiringCallPattern.FindString(string(data))).NotTo(BeEmpty(),
			"the manual make target must also bind the operator-rendered console roles")
	})
})

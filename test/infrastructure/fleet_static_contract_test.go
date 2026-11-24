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
	"os"
	"path/filepath"
	"regexp"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Fleet demo host-port isolation", func() {
	It("UT-2492-FLEET-PORT-001 [BR-PLATFORM-014]: offsets the host gateway probe without changing its NodePort", func() {
		original, wasSet := os.LookupEnv(demoHostPortOffsetEnv)
		DeferCleanup(func() {
			if wasSet {
				Expect(os.Setenv(demoHostPortOffsetEnv, original)).To(Succeed())
				return
			}
			Expect(os.Unsetenv(demoHostPortOffsetEnv)).To(Succeed())
		})
		Expect(os.Setenv(demoHostPortOffsetEnv, "20000")).To(Succeed())
		Expect(fleetGatewayHostPort(31975)).To(Equal(51975))
		Expect(31975).To(BeNumerically("<", 32768))
	})
})

var _ = Describe("Standalone FMC target-read RBAC ordering", func() {
	It("UT-FMC-E2E-002 [BR-SCOPE-001] [BR-SECURITY-016]: grants exchanged identity reads before waiting for FMC readiness", func() {
		source, err := os.ReadFile(filepath.Join(getProjectRoot(), "test", "infrastructure", "fleetmetadatacache_e2e.go"))
		Expect(err).NotTo(HaveOccurred())

		text := string(source)
		start := strings.Index(text, "func runFMCFleetCorePhase(")
		end := strings.Index(text, "\n// applyExchangedIdentityRBAC binds")
		Expect(start).To(BeNumerically(">=", 0))
		Expect(end).To(BeNumerically(">", start))
		phase := text[start:end]
		rbacIndex := strings.Index(phase, "applyExchangedIdentityRBAC(")
		deploymentIndex := strings.Index(phase, "deployFMCFleetCore(")
		Expect(rbacIndex).To(BeNumerically(">=", 0), "the exchanged target-read identity must be authorized")
		Expect(deploymentIndex).To(BeNumerically(">=", 0), "the standalone fixture must deploy FMC")
		Expect(rbacIndex).To(BeNumerically("<", deploymentIndex), "FMC rollout requires clean observations, so target-read RBAC must already exist")
	})
})

var _ = Describe("Local demo OIDC CA ordering", func() {
	It("UT-INFRA-OIDC-2566-001 [BR-PLATFORM-014]: prepares the operator CA before OIDC setup", func() {
		source, err := os.ReadFile(filepath.Join(getProjectRoot(), "hack", "setup-demo-infra", "main.go"))
		Expect(err).NotTo(HaveOccurred())

		text := string(source)
		operatorCAIndex := strings.Index(text, "infrastructure.EnsureOperatorInternalCA(")
		oidcIndex := strings.Index(text, "infrastructure.SetupOIDCInfrastructure(")
		Expect(operatorCAIndex).To(BeNumerically(">=", 0), "local demo must seed the operator CA")
		Expect(oidcIndex).To(BeNumerically(">=", 0), "local demo must configure OIDC")
		Expect(operatorCAIndex).To(BeNumerically("<", oidcIndex), "operator CA must exist before Keycloak TLS is provisioned")
	})
})

var _ = Describe("Thanos Querier release contract", func() {
	It("UT-2492-FLEET-THANOS-001 [BR-PLATFORM-014]: uses the Thanos v0.37 HTTP config flag", func() {
		Expect(thanosQuerierHTTPConfigArg()).To(Equal("--http.config=/etc/monitoring-web-config/web-config.yml"))
	})
})

var _ = Describe("Fleet static operator contract", func() {
	It("UT-FLEET-2593-001 [BR-INTEGRATION-054]: gives the remote enrichment Pod fixture its actual core API version", func() {
		source, err := os.ReadFile(filepath.Join(getProjectRoot(), "test", "e2e", "fleet", "02_sp_remote_enrichment_test.go"))
		Expect(err).NotTo(HaveOccurred())
		target := regexp.MustCompile(`(?s)TargetResource: signalprocessingv1.ResourceIdentifier\{([^}]+)\}`).FindSubmatch(source)
		Expect(target).To(HaveLen(2))
		Expect(string(target[1])).To(MatchRegexp(`APIVersion:\s*"v1"`))
		Expect(string(target[1])).To(MatchRegexp(`Kind:\s*"Pod"`))
	})

	It("UT-2492-CR-STATIC-001 [BR-FLEET-004]: renders the checked-in Fleet CR without dynamic EAIGW names", func() {
		fixture := filepath.Join(getProjectRoot(), "test", "fixtures", "operator", "kubernaut-fleet.yaml")
		manifest, err := RenderStaticOperatorCRManifest(fixture, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(manifest).To(ContainSubstring("endpoint: https://mcp-gateway.envoy-gateway-system.svc.cluster.local:8080/mcp"))
		Expect(manifest).To(ContainSubstring("namespace: mcp-system"))
		Expect(manifest).To(ContainSubstring("confidenceThreshold: 0.85"))
		Expect(manifest).NotTo(ContainSubstring(`confidenceThreshold: "0.85"`))
		Expect(manifest).To(ContainSubstring("apiFrontend:\n    enabled: true"))
		Expect(manifest).To(ContainSubstring("groups: claims.groups"))
		// #2702: spec.apiFrontend.rbac.roleBindings (and consoleAccessGroups)
		// were removed from the operator CRD. Persona ClusterRoles/CRBs are
		// harness-applied (BindAFPersonaRBAC), never operator-rendered, so the
		// CR must not carry any persona RBAC stanza.
		Expect(manifest).NotTo(ContainSubstring("roleBindings:"))
		Expect(manifest).NotTo(ContainSubstring("role: sre"))
		Expect(manifest).NotTo(ContainSubstring("consoleAccessGroups"))
		Expect(manifest).To(ContainSubstring("url: https://prometheus-svc.monitoring.svc.cluster.local:9090"))
		Expect(manifest).To(ContainSubstring("url: https://alertmanager-svc.monitoring.svc.cluster.local:9093"))
		Expect(manifest).To(ContainSubstring("prometheus:\n      enabled: true"))
		Expect(manifest).To(ContainSubstring("alertManager:\n      enabled: true"))
		Expect(manifest).To(ContainSubstring("caCertSecretRef:"))
		Expect(manifest).To(ContainSubstring("name: monitoring-client-ca"))
		Expect(manifest).To(ContainSubstring("mode: manual"))
		Expect(manifest).To(ContainSubstring("internalCASecretName: kubernaut-internal-ca"))
		Expect(manifest).To(ContainSubstring("maxConcurrentInvestigations: 50"))
		Expect(manifest).NotTo(ContainSubstring("source: interService"))
		Expect(manifest).NotTo(ContainSubstring("DevelopmentSelfSigned"))
		Expect(manifest).NotTo(ContainSubstring("developmentSelfSigned:"))
		Expect(manifest).NotTo(ContainSubstring("hooks:"))
		Expect(manifest).NotTo(ContainSubstring("envoy-mcp-system-mcp-gateway-"))
		Expect(manifest).NotTo(ContainSubstring("networkPolicies:"))
		Expect(manifest).To(ContainSubstring("credentialsSecretRef: fleet-oauth2-creds"))
		Expect(manifest).To(ContainSubstring("oauth2CredentialsSecretRef: fleet-workflowexecution-oauth2-creds"))
		Expect(strings.Count(manifest, "\n")).To(BeNumerically("<", 120))
	})
})

var _ = Describe("EAIGW stable Service alias", func() {
	It("UT-FMC-E2E-001 [BR-INTEGRATION-065]: does not require pod identity in the FMC deployment manifest", func() {
		source, err := os.ReadFile(filepath.Join(getProjectRoot(), "test", "infrastructure", "fleet_e2e.go"))
		Expect(err).NotTo(HaveOccurred())
		text := string(source)
		Expect(text).NotTo(ContainSubstring("- name: POD_UID"))
		Expect(text).NotTo(ContainSubstring("fieldPath: metadata.uid"))
	})

	It("UT-2492-CR-STATIC-005 [BR-FLEET-004]: gives FMC's MCP client the gateway CA", func() {
		source, err := os.ReadFile(filepath.Join(getProjectRoot(), "test", "infrastructure", "fleet_e2e.go"))
		Expect(err).NotTo(HaveOccurred())
		text := string(source)
		Expect(text).To(ContainSubstring("namespace: \"%[14]s\"\n      tlsCAFile: \"/etc/fleetmetadatacache/tls-ca/ca.crt\""))
	})

	It("UT-2492-CR-STATIC-002 [BR-FLEET-004]: selects the generated proxy by stable ownership labels", func() {
		manifest := BuildEAIGWStableServiceManifest("mcp-system")
		Expect(manifest).To(ContainSubstring("name: mcp-gateway"))
		Expect(manifest).To(ContainSubstring("namespace: envoy-gateway-system"))
		Expect(manifest).To(ContainSubstring("owning-gateway-name: mcp-gateway"))
		Expect(manifest).To(ContainSubstring("owning-gateway-namespace: mcp-system"))
		Expect(manifest).To(ContainSubstring("port: 8080"))
	})

	It("UT-2492-CR-STATIC-003 [BR-FLEET-004]: keeps the Kind host exception outside the authored CR", func() {
		manifest := BuildOperatorHostIngressNetworkPolicyManifest("kubernaut-system", "10.89.0.1/32")
		Expect(manifest).To(ContainSubstring("name: kubernaut-e2e-host-ingress"))
		Expect(manifest).To(ContainSubstring("kubernaut.ai/e2e-owner: upstream-2492"))
		Expect(manifest).To(ContainSubstring("cidr: 10.89.0.1/32"))
		Expect(manifest).To(ContainSubstring("operator: In"))
		Expect(manifest).To(ContainSubstring("- data-storage"))
		Expect(manifest).To(ContainSubstring("- kubernaut-agent"))
	})

	It("UT-2492-CR-STATIC-004 [BR-PLATFORM-014]: allows monitoring namespace traffic to Gateway without changing the CR", func() {
		manifest := BuildOperatorMonitoringIngressNetworkPolicyManifest("kubernaut-system", "monitoring")
		Expect(manifest).To(ContainSubstring("name: kubernaut-e2e-monitoring-ingress"))
		Expect(manifest).To(ContainSubstring("kubernetes.io/metadata.name: monitoring"))
		Expect(manifest).To(ContainSubstring("app: gateway"))
	})
})

var _ = Describe("AF persona RBAC wiring", func() {
	// #2702 (kubernaut-operator#682): the operator no longer renders persona
	// ClusterRoles (kubernaut-tool-<persona>) or user-facing
	// ClusterRoleBindings, and spec.apiFrontend.rbac.roleBindings /
	// consoleAccessGroups are gone from the CRD. The E2E harness owns this
	// provisioning end to end: PersonaToolClusterRolesYAML renders BOTH the
	// per-persona ClusterRoles and their group bindings, applied by
	// afDeployE2ERBAC (AF-only lane) and BindAFPersonaRBAC (operator-managed
	// AF lanes). This test pins that harness contract so the persona lists --
	// which duplicate the operator's former persona contract -- cannot drift
	// or silently lose the roles half.
	It("UT-2492-RBAC-001 [BR-SECURITY-001]: harness renders every AF persona ClusterRole and its OIDC group binding", func() {
		fullManifest, err := PersonaToolClusterRolesYAML()
		Expect(err).NotTo(HaveOccurred())

		personaTools, err := LoadPersonaTools()
		Expect(err).NotTo(HaveOccurred(), "harness persona tool contract must load")

		for _, persona := range []string{"sre", "ai-orchestrator", "cicd", "observability", "l3-audit", "remediation-approver"} {
			Expect(personaTools).To(HaveKey(persona),
				"harness persona tool contract must define persona %q", persona)

			rules, err := parseClusterRoleRulesFromYAML([]byte(fullManifest), "kubernaut-tool-"+persona)
			Expect(err).NotTo(HaveOccurred())
			Expect(rules).NotTo(BeEmpty(), "harness manifest must render ClusterRole kubernaut-tool-%s", persona)

			Expect(fullManifest).To(ContainSubstring("name: kubernaut-tool-" + persona + "-binding"),
				"harness manifest must render the ClusterRoleBinding for %s", persona)
			Expect(fullManifest).To(ContainSubstring("kind: ClusterRoleBinding"))
		}

		// The bindings-only manifest (used by the fullpipeline phase-6b wave)
		// must stay consistent with the full harness manifest's subjects.
		bindingsManifest := BuildAFPersonaToolClusterRoleBindingsManifest()
		for _, persona := range []string{"sre", "ai-orchestrator", "cicd", "observability", "l3-audit", "remediation-approver"} {
			Expect(bindingsManifest).To(ContainSubstring("name: kubernaut-tool-" + persona + "-binding"))
			Expect(bindingsManifest).To(ContainSubstring("name: kubernaut-tool-" + persona))
			Expect(bindingsManifest).To(ContainSubstring("name: " + persona))
		}
	})

	// #2702: the fullpipeline lane previously relied on the operator CR's
	// rbac.roleBindings to render persona ClusterRoles when a fleet
	// provisioner was active (waitForAFPersonaRBAC only, no harness apply).
	// With the operator RBAC decoupling, every operator-managed AF path must
	// apply the harness-owned persona RBAC via BindAFPersonaRBAC. Assertions
	// are deliberately coarse (function name presence, not exact source
	// lines) so harmless refactors do not break the contract, while the
	// negative assertion pins that no fullpipeline path may gate persona
	// RBAC on the operator having rendered it.
	It("UT-2702-RBAC-001 [BR-SECURITY-001]: fullpipeline applies harness persona RBAC on every operator-managed AF path", func() {
		source, err := os.ReadFile(filepath.Join(getProjectRoot(), "test", "infrastructure", "fullpipeline_e2e.go"))
		Expect(err).NotTo(HaveOccurred())
		text := string(source)
		Expect(text).To(ContainSubstring("BindAFPersonaRBAC(ctx"),
			"fullpipeline must apply harness-owned persona ClusterRoles/bindings via BindAFPersonaRBAC")
		Expect(text).NotTo(ContainSubstring("waitForAFPersonaRBAC"),
			"no fullpipeline path may rely on operator-rendered persona ClusterRoles (#2702); the SAR wait belongs to BindAFPersonaRBAC")
	})
})

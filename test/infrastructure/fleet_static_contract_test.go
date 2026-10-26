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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Fleet static operator contract", func() {
	It("UT-2492-CR-STATIC-001 [BR-FLEET-004]: renders the checked-in Fleet CR without dynamic EAIGW names", func() {
		fixture := filepath.Join(getProjectRoot(), "test", "fixtures", "operator", "kubernaut-fleet.yaml")
		manifest, err := RenderStaticOperatorCRManifest(fixture, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(manifest).To(ContainSubstring("endpoint: https://mcp-gateway.envoy-gateway-system.svc.cluster.local:8080/mcp"))
		Expect(manifest).To(ContainSubstring("apiFrontend:\n    enabled: true"))
		Expect(manifest).To(ContainSubstring("groups: claims.groups"))
		Expect(manifest).To(ContainSubstring("role: sre"))
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
	It("UT-2492-RBAC-001 [BR-SECURITY-001]: binds every AF persona ClusterRole to its OIDC group", func() {
		manifest := BuildAFPersonaToolClusterRoleBindingsManifest()
		for _, persona := range []string{"sre", "ai-orchestrator", "cicd", "observability", "l3-audit", "remediation-approver"} {
			Expect(manifest).To(ContainSubstring("name: kubernaut-tool-" + persona + "-binding"))
			Expect(manifest).To(ContainSubstring("name: kubernaut-tool-" + persona))
			Expect(manifest).To(ContainSubstring("name: " + persona))
		}
		fullManifest, err := PersonaToolClusterRolesYAML()
		Expect(err).NotTo(HaveOccurred())
		Expect(fullManifest).To(ContainSubstring("kind: ClusterRole"))
		Expect(fullManifest).To(ContainSubstring("name: kubernaut-tool-sre"))
	})
})

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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-PLATFORM-014 (Issue #2337): hack/setup-fleet-infra's demo entry point
// grows a `helm install` step of its own, mirroring InstallFullPipelineHelmChart's
// exec.Command("helm", ...) pattern without any of its E2E-only overrides.
// These specs cover the pure options-to-args/options-to-manifest builders --
// the exec.Command call itself and BindFleetAFPersonaRBAC aren't unit-testable
// (same as the existing E2E helpers), verified manually against a live Kind
// cluster instead.

var _ = Describe("FleetDemoHelmOptions.Validate", func() {
	DescribeTable("reporting every missing required LLM flag in one pass",
		func(opts FleetDemoHelmOptions, expectErr bool, expectedSubstrings []string) {
			err := opts.Validate()
			if !expectErr {
				Expect(err).NotTo(HaveOccurred())
				return
			}
			Expect(err).To(HaveOccurred())
			for _, s := range expectedSubstrings {
				Expect(err.Error()).To(ContainSubstring(s))
			}
		},
		Entry("UT-INFRA-FLEETDEMO-001: all required fields set passes",
			FleetDemoHelmOptions{
				LLMProvider:   "openai_compatible",
				LLMModel:      "gpt-4o",
				LLMEndpoint:   "https://api.openai.com/v1",
				LLMAPIKeyFile: "/tmp/key.txt",
			}, false, nil),
		Entry("UT-INFRA-FLEETDEMO-002: missing everything reports every flag",
			FleetDemoHelmOptions{}, true,
			[]string{"-llm-provider", "-llm-model", "-llm-endpoint", "-llm-api-key-file"}),
		Entry("UT-INFRA-FLEETDEMO-003: missing only the API key file reports just that one",
			FleetDemoHelmOptions{
				LLMProvider: "openai_compatible",
				LLMModel:    "gpt-4o",
				LLMEndpoint: "https://api.openai.com/v1",
			}, true, []string{"-llm-api-key-file"}),
	)
})

// buildFleetOAuth2HelmArgs lives in fullpipeline_e2e_helm.go (it's the
// pre-existing fleet-OAuth2 `--set` block extracted out of
// InstallFullPipelineHelmChart, Issue #2337 REFACTOR), but is exercised here
// alongside buildFleetDemoHelmArgs since this is the spec file that first
// needed it as an independently-callable, unit-testable function.
var _ = Describe("buildFleetOAuth2HelmArgs", func() {
	It("UT-INFRA-FLEETDEMO-030: returns nil when fleetOpts is nil", func() {
		Expect(buildFleetOAuth2HelmArgs(nil)).To(BeNil())
	})

	It("UT-INFRA-FLEETDEMO-031: renders the full fleet OAuth2 block, including indexed scopes", func() {
		args := buildFleetOAuth2HelmArgs(&FleetHelmOptions{
			MCPGatewayEndpoint:          "http://envoy-ai-gateway.gateway-system.svc:8080/mcp",
			MCPGatewayType:              "eaigw",
			OAuth2TokenURL:              "https://keycloak:8443/realms/kubernaut-fleet/protocol/openid-connect/token",
			OAuth2CredentialsSecret:     "fleet-oauth2-creds",
			WEOAuth2CredentialsSecret:   "we-fleet-oauth2-creds",
			OAuth2Scopes:                []string{"fleet.read", "fleet.write"},
			SignalProcessingNamespace:   "kubernaut-system",
			FleetMetadataCacheNamespace: "kubernaut-system",
		})
		Expect(args).To(ContainElements(
			"--set", "global.fleet.enabled=true",
			"--set", "global.fleet.mcpGatewayEndpoint=http://envoy-ai-gateway.gateway-system.svc:8080/mcp",
			"--set", "global.fleet.mcpGatewayType=eaigw",
			"--set", "global.fleet.oauth2.enabled=true",
			"--set", "global.fleet.oauth2.tokenURL=https://keycloak:8443/realms/kubernaut-fleet/protocol/openid-connect/token",
			"--set", "global.fleet.oauth2.credentialsSecretRef=fleet-oauth2-creds",
			"--set", "workflowexecution.fleet.oauth2.credentialsSecretRef=we-fleet-oauth2-creds",
			"--set", "global.fleet.oauth2.scopes[0]=fleet.read",
			"--set", "global.fleet.oauth2.scopes[1]=fleet.write",
			"--set", "signalprocessing.fleet.namespace=kubernaut-system",
			"--set", "fleetmetadatacache.namespace=kubernaut-system",
		))
	})

	It("UT-INFRA-FLEETDEMO-032: omits signalprocessing.fleet.namespace when empty (cluster-wide watch)", func() {
		args := buildFleetOAuth2HelmArgs(&FleetHelmOptions{FleetMetadataCacheNamespace: "kubernaut-system"})
		for i, a := range args {
			if a == "--set" && i+1 < len(args) {
				Expect(args[i+1]).NotTo(HavePrefix("signalprocessing.fleet.namespace="))
			}
		}
	})

	It("UT-INFRA-FLEETDEMO-033: always sets fleetmetadatacache.namespace, even when empty (Issue #2298: no safe default)", func() {
		args := buildFleetOAuth2HelmArgs(&FleetHelmOptions{})
		Expect(args).To(ContainElements("--set", "fleetmetadatacache.namespace="))
	})
})

var _ = Describe("buildFleetDemoHelmArgs", func() {
	baseOpts := FleetDemoHelmOptions{
		LLMProvider: "openai_compatible",
		LLMModel:    "gpt-4o",
		LLMEndpoint: "https://api.openai.com/v1",
	}
	baseFleetOpts := &FleetHelmOptions{
		MCPGatewayEndpoint:          "http://envoy-ai-gateway.gateway-system.svc:8080/mcp",
		MCPGatewayType:              "eaigw",
		OAuth2TokenURL:              "https://keycloak:8443/realms/kubernaut-fleet/protocol/openid-connect/token",
		OAuth2CredentialsSecret:     "fleet-oauth2-creds",
		OAuth2Scopes:                []string{"fleet.read", "fleet.write"},
		WEOAuth2CredentialsSecret:   "fleet-oauth2-creds",
		SignalProcessingNamespace:   "",
		FleetMetadataCacheNamespace: "kubernaut-system",
	}

	It("UT-INFRA-FLEETDEMO-010: sets gateway.enabled=false by default (Console-first)", func() {
		args := buildFleetDemoHelmArgs("/tmp/kubeconfig", "charts/kubernaut", "kubernaut-system", baseFleetOpts, baseOpts, "/tmp/sp.rego", "/tmp/aa.rego")
		Expect(args).To(ContainElements("--set", "gateway.enabled=false"))
	})

	It("UT-INFRA-FLEETDEMO-011: sets gateway.enabled=true when Autonomous is requested", func() {
		opts := baseOpts
		opts.Autonomous = true
		args := buildFleetDemoHelmArgs("/tmp/kubeconfig", "charts/kubernaut", "kubernaut-system", baseFleetOpts, opts, "/tmp/sp.rego", "/tmp/aa.rego")
		Expect(args).To(ContainElements("--set", "gateway.enabled=true"))
	})

	It("UT-INFRA-FLEETDEMO-012: never includes --wait (post-install migration hook would deadlock)", func() {
		args := buildFleetDemoHelmArgs("/tmp/kubeconfig", "charts/kubernaut", "kubernaut-system", baseFleetOpts, baseOpts, "/tmp/sp.rego", "/tmp/aa.rego")
		Expect(args).NotTo(ContainElement("--wait"))
	})

	It("UT-INFRA-FLEETDEMO-013: always enables Console regardless of Autonomous", func() {
		args := buildFleetDemoHelmArgs("/tmp/kubeconfig", "charts/kubernaut", "kubernaut-system", baseFleetOpts, baseOpts, "/tmp/sp.rego", "/tmp/aa.rego")
		Expect(args).To(ContainElements("--set", "console.enabled=true"))
	})

	It("UT-INFRA-FLEETDEMO-014: wires LLM profile fields from options", func() {
		args := buildFleetDemoHelmArgs("/tmp/kubeconfig", "charts/kubernaut", "kubernaut-system", baseFleetOpts, baseOpts, "/tmp/sp.rego", "/tmp/aa.rego")
		Expect(args).To(ContainElements(
			"--set", "global.llmProfiles.primary.provider=openai_compatible",
			"--set", "global.llmProfiles.primary.model=gpt-4o",
			"--set", "global.llmProfiles.primary.endpoint=https://api.openai.com/v1",
		))
	})

	It("UT-INFRA-FLEETDEMO-015: wires fleet OAuth2 block from FleetHelmOptions", func() {
		args := buildFleetDemoHelmArgs("/tmp/kubeconfig", "charts/kubernaut", "kubernaut-system", baseFleetOpts, baseOpts, "/tmp/sp.rego", "/tmp/aa.rego")
		Expect(args).To(ContainElements(
			"--set", "global.fleet.enabled=true",
			"--set", "global.fleet.mcpGatewayEndpoint=http://envoy-ai-gateway.gateway-system.svc:8080/mcp",
			"--set", "global.fleet.oauth2.scopes[0]=fleet.read",
			"--set", "global.fleet.oauth2.scopes[1]=fleet.write",
			"--set", "fleetmetadatacache.namespace=kubernaut-system",
		))
	})

	It("UT-INFRA-FLEETDEMO-016: omits the fleet block entirely when fleetOpts is nil", func() {
		args := buildFleetDemoHelmArgs("/tmp/kubeconfig", "charts/kubernaut", "kubernaut-system", nil, baseOpts, "/tmp/sp.rego", "/tmp/aa.rego")
		for _, a := range args {
			Expect(a).NotTo(ContainSubstring("global.fleet."))
		}
	})

	It("UT-INFRA-FLEETDEMO-017: points APIFrontend/Console OIDC at Keycloak, not DEX", func() {
		args := buildFleetDemoHelmArgs("/tmp/kubeconfig", "charts/kubernaut", "kubernaut-system", baseFleetOpts, baseOpts, "/tmp/sp.rego", "/tmp/aa.rego")
		Expect(args).To(ContainElements(
			"--set", "apifrontend.config.auth.issuerURL=https://keycloak:8443/realms/kubernaut-fleet",
		))
	})

	It("UT-INFRA-FLEETDEMO-018: uses --set-file for the Rego policy paths passed in", func() {
		args := buildFleetDemoHelmArgs("/tmp/kubeconfig", "charts/kubernaut", "kubernaut-system", baseFleetOpts, baseOpts, "/tmp/sp.rego", "/tmp/aa.rego")
		Expect(args).To(ContainElements(
			"--set-file", "signalprocessing.policies.content=/tmp/sp.rego",
			"--set-file", "aianalysis.policies.content=/tmp/aa.rego",
		))
	})

	It("UT-INFRA-FLEETDEMO-019: installs into whatever --namespace the caller passes in", func() {
		args := buildFleetDemoHelmArgs("/tmp/kubeconfig", "charts/kubernaut", "some-other-namespace", baseFleetOpts, baseOpts, "/tmp/sp.rego", "/tmp/aa.rego")
		Expect(args).To(ContainElements("--namespace", "some-other-namespace"))
	})
})

var _ = Describe("buildFleetDemoHelmSecretsManifest", func() {
	It("UT-INFRA-FLEETDEMO-020: renders all four required Secrets with the values passed in", func() {
		manifest := buildFleetDemoHelmSecretsManifest("kubernaut-system", "pgpass123", "valkeypass456", "sk-my-real-key", "cookiesecretvalue")
		Expect(manifest).To(ContainSubstring("name: postgresql-secret"))
		Expect(manifest).To(ContainSubstring("POSTGRES_PASSWORD: pgpass123"))
		Expect(manifest).To(ContainSubstring("name: valkey-secret"))
		Expect(manifest).To(ContainSubstring("password: valkeypass456"))
		Expect(manifest).To(ContainSubstring("name: llm-credentials-primary"))
		Expect(manifest).To(ContainSubstring("api_key: sk-my-real-key"))
		Expect(manifest).To(ContainSubstring("name: console-oauth-creds"))
		Expect(manifest).To(ContainSubstring("client-id: kubernaut-console"))
		Expect(manifest).To(ContainSubstring("client-secret: e2e-console-secret"))
		Expect(manifest).To(ContainSubstring("cookie-secret: cookiesecretvalue"))
	})

	It("UT-INFRA-FLEETDEMO-021: scopes every Secret to the namespace passed in", func() {
		manifest := buildFleetDemoHelmSecretsManifest("my-ns", "a", "b", "c", "d")
		Expect(manifest).To(ContainSubstring("namespace: my-ns"))
		Expect(manifest).NotTo(ContainSubstring("namespace: kubernaut-system"))
	})
})

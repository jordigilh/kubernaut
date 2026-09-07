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
	"strings"

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

// helmSetFlag is the helm arg separator preceding every rendered value in the
// builders under test. A const (not a literal) so goconst stays quiet as the
// prefix-assertion loops below multiply.
const helmSetFlag = "--set"

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
				LLMProvider: "openai_compatible",
				LLMModel:    "gpt-4o",
				LLMEndpoint: "https://api.openai.com/v1",
			}, false, nil),
		Entry("UT-INFRA-FLEETDEMO-002: missing everything reports every flag",
			FleetDemoHelmOptions{}, true,
			[]string{"-llm-provider", "-llm-model", "-llm-endpoint"}),
		Entry("UT-INFRA-FLEETDEMO-003: missing only the endpoint reports just that one",
			FleetDemoHelmOptions{
				LLMProvider: "openai_compatible",
				LLMModel:    "gpt-4o",
			}, true, []string{"-llm-endpoint"}),
		Entry("UT-INFRA-FLEETDEMO-038: vertex_ai without project/location reports both flags",
			FleetDemoHelmOptions{
				LLMProvider: "vertex_ai",
				LLMModel:    "claude-haiku-4-5@20251001",
				LLMEndpoint: "https://us-central1-aiplatform.googleapis.com",
			}, true, []string{"-vertex-project", "-vertex-location"}),
		Entry("UT-INFRA-FLEETDEMO-039: vertex_ai with project/location passes",
			FleetDemoHelmOptions{
				LLMProvider:    "vertex_ai",
				LLMModel:       "claude-haiku-4-5@20251001",
				LLMEndpoint:    "https://us-central1-aiplatform.googleapis.com",
				VertexProject:  "my-gcp-project",
				VertexLocation: "us-central1",
			}, false, nil),
		Entry("UT-INFRA-FLEETDEMO-040: non-vertex provider without project/location passes",
			FleetDemoHelmOptions{
				LLMProvider: "openai_compatible",
				LLMModel:    "gpt-4o",
				LLMEndpoint: "https://api.openai.com/v1",
			}, false, nil),
		Entry("UT-INFRA-FLEETDEMO-043: vertex_ai without endpoint passes (endpoint derived from project/location, issue #2355)",
			FleetDemoHelmOptions{
				LLMProvider:    "vertex_ai",
				LLMModel:       "claude-haiku-4-5@20251001",
				VertexProject:  "my-gcp-project",
				VertexLocation: "us-central1",
			}, false, nil),
		Entry("UT-INFRA-FLEETDEMO-044: openai_compatible without endpoint still fails",
			FleetDemoHelmOptions{
				LLMProvider: "openai_compatible",
				LLMModel:    "gpt-4o",
			}, true, []string{"-llm-endpoint"}),
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
			ImageTag:                    "latest",
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
			"--set", "global.image.tag=latest",
		))
	})

	It("UT-INFRA-FLEETDEMO-032: omits signalprocessing.fleet.namespace when empty (cluster-wide watch)", func() {
		args := buildFleetOAuth2HelmArgs(&FleetHelmOptions{FleetMetadataCacheNamespace: "kubernaut-system"})
		for i, a := range args {
			if a == helmSetFlag && i+1 < len(args) {
				Expect(args[i+1]).NotTo(HavePrefix("signalprocessing.fleet.namespace="))
			}
		}
	})

	It("UT-INFRA-FLEETDEMO-033: always sets fleetmetadatacache.namespace, even when empty (Issue #2298: no safe default)", func() {
		args := buildFleetOAuth2HelmArgs(&FleetHelmOptions{})
		Expect(args).To(ContainElements("--set", "fleetmetadatacache.namespace="))
	})

	It("UT-INFRA-FLEETDEMO-035: omits global.image.tag when ImageTag is empty (a trailing empty --set would clobber the demo override, helm last-wins)", func() {
		args := buildFleetOAuth2HelmArgs(&FleetHelmOptions{FleetMetadataCacheNamespace: "kubernaut-system"})
		for i, a := range args {
			if a == helmSetFlag && i+1 < len(args) {
				Expect(args[i+1]).NotTo(HavePrefix("global.image.tag="))
			}
		}
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

	It("UT-INFRA-FLEETDEMO-046: wires AF alert tools to the fleet monitoring stack (Thanos Querier + Alertmanager)", func() {
		args := buildFleetDemoHelmArgs("/tmp/kubeconfig", "charts/kubernaut", "kubernaut-system", baseFleetOpts, baseOpts, "/tmp/sp.rego", "/tmp/aa.rego")
		Expect(args).To(ContainElements(
			"--set", "monitoring.prometheus.enabled=true",
			"--set", "monitoring.prometheus.url=http://thanos-querier-svc.monitoring.svc.cluster.local:9090",
			"--set", "monitoring.alertManager.enabled=true",
			"--set", "monitoring.alertManager.url=http://alertmanager-svc.monitoring.svc.cluster.local:9093",
		))
	})

	It("UT-INFRA-FLEETDEMO-017: points APIFrontend/Console OIDC at Keycloak, not DEX", func() {
		args := buildFleetDemoHelmArgs("/tmp/kubeconfig", "charts/kubernaut", "kubernaut-system", baseFleetOpts, baseOpts, "/tmp/sp.rego", "/tmp/aa.rego")
		Expect(args).To(ContainElements(
			"--set", "apifrontend.config.auth.issuerURL=https://keycloak:8443/realms/kubernaut-fleet",
		))
	})

	It("UT-INFRA-FLEETDEMO-036: omits global.image.tag when ImageTag is empty (chart falls back to .Chart.AppVersion)", func() {
		args := buildFleetDemoHelmArgs("/tmp/kubeconfig", "charts/kubernaut", "kubernaut-system", baseFleetOpts, baseOpts, "/tmp/sp.rego", "/tmp/aa.rego")
		for i, a := range args {
			if a == helmSetFlag && i+1 < len(args) {
				Expect(args[i+1]).NotTo(HavePrefix("global.image.tag="))
			}
		}
	})

	It("UT-INFRA-FLEETDEMO-037: sets exactly one global.image.tag when ImageTag is provided (no trailing empty duplicate)", func() {
		opts := baseOpts
		opts.ImageTag = "demo-v1.0"
		args := buildFleetDemoHelmArgs("/tmp/kubeconfig", "charts/kubernaut", "kubernaut-system", baseFleetOpts, opts, "/tmp/sp.rego", "/tmp/aa.rego")
		var tags []string
		for i, a := range args {
			if a == "--set" && i+1 < len(args) && strings.HasPrefix(args[i+1], "global.image.tag=") {
				tags = append(tags, args[i+1])
			}
		}
		Expect(tags).To(Equal([]string{"global.image.tag=demo-v1.0"}))
	})

	It("UT-INFRA-FLEETDEMO-041: omits vertexProject/vertexLocation --set when empty (non-Vertex providers)", func() {
		args := buildFleetDemoHelmArgs("/tmp/kubeconfig", "charts/kubernaut", "kubernaut-system", baseFleetOpts, baseOpts, "/tmp/sp.rego", "/tmp/aa.rego")
		for i, a := range args {
			if a == helmSetFlag && i+1 < len(args) {
				Expect(args[i+1]).NotTo(HavePrefix("global.llmProfiles.primary.vertexProject="))
				Expect(args[i+1]).NotTo(HavePrefix("global.llmProfiles.primary.vertexLocation="))
			}
		}
	})

	It("UT-INFRA-FLEETDEMO-042: sets vertexProject/vertexLocation --set when provided (BR-PLATFORM-014 Vertex AI demo)", func() {
		opts := baseOpts
		opts.VertexProject = "my-gcp-project"
		opts.VertexLocation = "us-central1"
		args := buildFleetDemoHelmArgs("/tmp/kubeconfig", "charts/kubernaut", "kubernaut-system", baseFleetOpts, opts, "/tmp/sp.rego", "/tmp/aa.rego")
		Expect(args).To(ContainElements(
			"--set", "global.llmProfiles.primary.vertexProject=my-gcp-project",
			"--set", "global.llmProfiles.primary.vertexLocation=us-central1",
		))
	})

	It("UT-INFRA-FLEETDEMO-045: omits the endpoint --set when empty (vertex_ai derives it; issue #2355)", func() {
		opts := baseOpts
		opts.LLMEndpoint = ""
		opts.VertexProject = "my-gcp-project"
		opts.VertexLocation = "us-central1"
		args := buildFleetDemoHelmArgs("/tmp/kubeconfig", "charts/kubernaut", "kubernaut-system", baseFleetOpts, opts, "/tmp/sp.rego", "/tmp/aa.rego")
		for i, a := range args {
			if a == helmSetFlag && i+1 < len(args) {
				Expect(args[i+1]).NotTo(HavePrefix("global.llmProfiles.primary.endpoint="))
			}
		}
	})

	It("UT-INFRA-FLEETDEMO-034: sets apifrontend.config.auth.audience to kubernaut-apifrontend (issue #2352, else AF 401s every console token -> 'Unable to Verify Access')", func() {
		args := buildFleetDemoHelmArgs("/tmp/kubeconfig", "charts/kubernaut", "kubernaut-system", baseFleetOpts, baseOpts, "/tmp/sp.rego", "/tmp/aa.rego")
		Expect(args).To(ContainElements(
			"--set", "apifrontend.config.auth.audience=kubernaut-apifrontend",
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
	It("UT-INFRA-FLEETDEMO-020: renders the three tool-generated Secrets with the values passed in", func() {
		manifest := buildFleetDemoHelmSecretsManifest("kubernaut-system", "pgpass123", "valkeypass456", "cookiesecretvalue")
		Expect(manifest).To(ContainSubstring("name: postgresql-secret"))
		Expect(manifest).To(ContainSubstring("POSTGRES_PASSWORD: pgpass123"))
		Expect(manifest).To(ContainSubstring("name: valkey-secret"))
		Expect(manifest).To(ContainSubstring("password: valkeypass456"))
		Expect(manifest).To(ContainSubstring("name: console-oauth-creds"))
		Expect(manifest).To(ContainSubstring("client-id: kubernaut-console"))
		Expect(manifest).To(ContainSubstring("client-secret: e2e-console-secret"))
		Expect(manifest).To(ContainSubstring("cookie-secret: cookiesecretvalue"))
	})

	It("UT-INFRA-FLEETDEMO-020b: never renders the LLM credentials Secret -- the user creates it themselves", func() {
		manifest := buildFleetDemoHelmSecretsManifest("kubernaut-system", "pgpass123", "valkeypass456", "cookiesecretvalue")
		Expect(manifest).NotTo(ContainSubstring("name: llm-credentials-primary"))
		Expect(manifest).NotTo(ContainSubstring("api_key:"))
	})

	It("UT-INFRA-FLEETDEMO-021: scopes every Secret to the namespace passed in", func() {
		manifest := buildFleetDemoHelmSecretsManifest("my-ns", "a", "b", "c")
		Expect(manifest).To(ContainSubstring("namespace: my-ns"))
		Expect(manifest).NotTo(ContainSubstring("namespace: kubernaut-system"))
	})
})

var _ = Describe("fleetDemoDefaultAAPolicyRego", func() {
	It("UT-INFRA-FLEETDEMO-022: always requires approval, unconditionally", func() {
		Expect(fleetDemoDefaultAAPolicyRego).To(ContainSubstring("default require_approval := true"))
	})
})

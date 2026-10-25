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

var _ = Describe("DemoHelmOptions.Validate", func() {
	DescribeTable("reporting every missing required LLM flag in one pass",
		func(opts DemoHelmOptions, expectErr bool, expectedSubstrings []string) {
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
			DemoHelmOptions{
				LLMProvider: "openai_compatible",
				LLMModel:    "gpt-4o",
				LLMEndpoint: "https://api.openai.com/v1",
			}, false, nil),
		Entry("UT-INFRA-FLEETDEMO-002: missing everything reports every flag",
			DemoHelmOptions{}, true,
			[]string{"-llm-provider", "-llm-model", "-llm-endpoint"}),
		Entry("UT-INFRA-FLEETDEMO-003: missing only the endpoint reports just that one",
			DemoHelmOptions{
				LLMProvider: "openai_compatible",
				LLMModel:    "gpt-4o",
			}, true, []string{"-llm-endpoint"}),
		Entry("UT-INFRA-FLEETDEMO-038: vertex_ai without project/location reports both flags",
			DemoHelmOptions{
				LLMProvider: "vertex_ai",
				LLMModel:    "claude-haiku-4-5@20251001",
				LLMEndpoint: "https://us-central1-aiplatform.googleapis.com",
			}, true, []string{"-vertex-project", "-vertex-location"}),
		Entry("UT-INFRA-FLEETDEMO-039: vertex_ai with project/location passes",
			DemoHelmOptions{
				LLMProvider:    "vertex_ai",
				LLMModel:       "claude-haiku-4-5@20251001",
				LLMEndpoint:    "https://us-central1-aiplatform.googleapis.com",
				VertexProject:  "my-gcp-project",
				VertexLocation: "us-central1",
			}, false, nil),
		Entry("UT-INFRA-FLEETDEMO-040: non-vertex provider without project/location passes",
			DemoHelmOptions{
				LLMProvider: "openai_compatible",
				LLMModel:    "gpt-4o",
				LLMEndpoint: "https://api.openai.com/v1",
			}, false, nil),
		Entry("UT-INFRA-FLEETDEMO-043: vertex_ai without endpoint passes (endpoint derived from project/location, issue #2355)",
			DemoHelmOptions{
				LLMProvider:    "vertex_ai",
				LLMModel:       "claude-haiku-4-5@20251001",
				VertexProject:  "my-gcp-project",
				VertexLocation: "us-central1",
			}, false, nil),
		Entry("UT-INFRA-FLEETDEMO-044: openai_compatible without endpoint still fails",
			DemoHelmOptions{
				LLMProvider: "openai_compatible",
				LLMModel:    "gpt-4o",
			}, true, []string{"-llm-endpoint"}),
	)
})

func demoBoolPointer(value bool) *bool {
	return &value
}

var _ = Describe("resolveDemoReasoning", func() {
	DescribeTable("resolves model-family defaults and explicit overrides",
		func(opts DemoHelmOptions, wantEnabled bool, wantEffort, wantOverride string) {
			enabled, effort, capabilityOverride := resolveDemoReasoning(opts)
			Expect(enabled).To(Equal(wantEnabled))
			Expect(effort).To(Equal(wantEffort))
			Expect(capabilityOverride).To(Equal(wantOverride))
		},
		Entry("UT-INFRA-FLEETDEMO-052 [BR-PLATFORM-014]: gpt-5.6-luna defaults to no reasoning tokens",
			DemoHelmOptions{LLMProvider: "openai", LLMModel: "gpt-5.6-luna"},
			true, "none", ""),
		Entry("UT-INFRA-FLEETDEMO-062 [BR-PLATFORM-014]: GPT Luna model versions default to no reasoning effort for function-tool compatibility",
			DemoHelmOptions{LLMProvider: "openai", LLMModel: "gpt-6-luna"},
			true, "none", ""),
		Entry("UT-INFRA-FLEETDEMO-063 [BR-PLATFORM-014]: newer GPT versions use the same default across model variants",
			DemoHelmOptions{LLMProvider: "openai", LLMModel: "GPT-7.2-SOL-preview"},
			true, "none", ""),
		Entry("UT-INFRA-FLEETDEMO-065 [BR-PLATFORM-014]: GPT 5.6 Terra defaults to no reasoning effort",
			DemoHelmOptions{LLMProvider: "openai", LLMModel: "gpt-5.6-terra"},
			true, "none", ""),
		Entry("UT-INFRA-FLEETDEMO-066 [BR-PLATFORM-014]: GPT 5.6 base alias defaults to no reasoning effort",
			DemoHelmOptions{LLMProvider: "openai", LLMModel: "gpt-5.6"},
			true, "none", ""),
		Entry("UT-INFRA-FLEETDEMO-053 [BR-PLATFORM-014]: gpt-5 defaults to minimal effort",
			DemoHelmOptions{LLMProvider: "openai", LLMModel: "gpt-5"},
			true, "minimal", ""),
		Entry("UT-INFRA-FLEETDEMO-054 [BR-PLATFORM-014]: o1 defaults to low effort",
			DemoHelmOptions{LLMProvider: "openai", LLMModel: "o1"},
			true, "low", ""),
		Entry("UT-INFRA-FLEETDEMO-055 [BR-PLATFORM-014]: o3 defaults to low effort",
			DemoHelmOptions{LLMProvider: "openai", LLMModel: "o3-mini"},
			true, "low", ""),
		Entry("UT-INFRA-FLEETDEMO-056 [BR-PLATFORM-014]: o4 defaults to low effort",
			DemoHelmOptions{LLMProvider: "openai", LLMModel: "o4"},
			true, "low", ""),
		Entry("UT-INFRA-FLEETDEMO-057 [BR-PLATFORM-014]: gpt-4o remains disabled",
			DemoHelmOptions{LLMProvider: "openai", LLMModel: "gpt-4o"},
			false, "", ""),
		Entry("UT-INFRA-FLEETDEMO-058 [BR-PLATFORM-014]: custom OpenAI-compatible GPT-like names do not infer support",
			DemoHelmOptions{LLMProvider: "openai_compatible", LLMModel: "gpt-5"},
			false, "", ""),
		Entry("UT-INFRA-FLEETDEMO-059 [BR-PLATFORM-014]: explicit custom enablement opts into the OpenAI effort dialect",
			DemoHelmOptions{
				LLMProvider:         "openai_compatible",
				LLMModel:            "custom-reasoning-model",
				LLMReasoningEnabled: demoBoolPointer(true),
				LLMReasoningEffort:  "low",
			},
			true, "low", "force_on"),
		Entry("UT-INFRA-FLEETDEMO-060 [BR-PLATFORM-014]: explicit disablement wins over a model default",
			DemoHelmOptions{
				LLMProvider:         "openai",
				LLMModel:            "gpt-5",
				LLMReasoningEnabled: demoBoolPointer(false),
			},
			false, "", "force_off"),
	)

	It("UT-INFRA-FLEETDEMO-061 [SI-10]: rejects an unsupported effort value", func() {
		opts := DemoHelmOptions{
			LLMProvider:        "openai",
			LLMModel:           "gpt-5",
			LLMEndpoint:        "https://api.openai.com/v1",
			LLMReasoningEffort: "extreme",
		}
		Expect(opts.Validate()).To(MatchError(ContainSubstring("reasoning.effort")))
	})
})

var _ = Describe("demo local image loading", func() {
	It("UT-INFRA-FLEETDEMO-049 [BR-PLATFORM-014]: builds all native local image references without mock-llm", func() {
		expected := []string{
			"localhost/datastorage:demo-tag",
			"localhost/gateway:demo-tag",
			"localhost/aianalysis:demo-tag",
			"localhost/authwebhook:demo-tag",
			"localhost/notification:demo-tag",
			"localhost/remediationorchestrator:demo-tag",
			"localhost/signalprocessing:demo-tag",
			"localhost/workflowexecution:demo-tag",
			"localhost/effectivenessmonitor:demo-tag",
			"localhost/kubernautagent:demo-tag",
			"localhost/apifrontend:demo-tag",
			"localhost/fleetmetadatacache:demo-tag",
			"localhost/db-migrate:demo-tag",
		}

		Expect(demoLocalImageReferences("localhost/", "demo-tag")).To(Equal(expected))
	})

	It("UT-INFRA-FLEETDEMO-050 [BR-PLATFORM-014]: recognizes localhost repositories for Kind loading", func() {
		Expect(isLocalDemoImageRepository("localhost")).To(BeTrue())
		Expect(isLocalDemoImageRepository("localhost/kubernaut")).To(BeTrue())
		Expect(isLocalDemoImageRepository("quay.io/kubernaut-ai")).To(BeFalse())
	})
})

var _ = Describe("demoDefaultAAPolicyRego", func() {
	It("UT-INFRA-FLEETDEMO-022: always requires approval, unconditionally", func() {
		Expect(demoDefaultAAPolicyRego).To(ContainSubstring("default require_approval := true"))
	})
})

var _ = Describe("operator-backed demo CR wiring", func() {
	It("UT-2492-DEMO-003 [BR-PLATFORM-014]: creates a test-owned Console Ingress for the operator Service", func() {
		manifest := BuildDemoConsoleIngressManifest("kubernaut-system")
		Expect(manifest).To(ContainSubstring("kind: Ingress"))
		Expect(manifest).To(ContainSubstring("name: kubernaut-console"))
		Expect(manifest).To(ContainSubstring("ingressClassName: traefik"))
		Expect(manifest).To(ContainSubstring("service:"))
		Expect(strings.Count(manifest, "name: kubernaut-console")).To(Equal(2))
		Expect(manifest).To(ContainSubstring("secretName: console-tls"))
		Expect(manifest).To(ContainSubstring("kubernaut.ai/e2e-owner: upstream-2492"))
		Expect(manifest).NotTo(ContainSubstring("ownerReferences:"))
	})

	It("UT-2492-DEMO-001 [BR-PLATFORM-014]: local mode uses Keycloak and the bootstrap-owned Console credential", func() {
		opts := DemoHelmOptions{
			Mode:        DemoModeLocal,
			Autonomous:  true,
			LLMProvider: "openai_compatible",
			LLMModel:    "gpt-4o",
			LLMEndpoint: "https://api.example.invalid/v1",
		}
		cr, err := BuildKubernautCRManifest(buildDemoOperatorCROptions(opts, nil))
		Expect(err).NotTo(HaveOccurred())
		Expect(cr).To(ContainSubstring("issuerURL: https://keycloak:8443/realms/kubernaut-demo"))
		Expect(cr).To(ContainSubstring("jwksURL: https://keycloak.idp.svc.cluster.local:8443/realms/kubernaut-demo/protocol/openid-connect/certs"))
		Expect(cr).To(ContainSubstring("secretName: kubernaut-console-oidc"))
		Expect(cr).To(ContainSubstring("redirectURL: https://kubernaut-console.local:8843/oauth2/callback"))
		Expect(cr).NotTo(ContainSubstring("ingressClassName:"))
		Expect(cr).NotTo(ContainSubstring("tlsSecretName:"))
		Expect(cr).NotTo(ContainSubstring("ingress:"))
		Expect(cr).NotTo(ContainSubstring("dex"))
		Expect(cr).NotTo(ContainSubstring("console-oauth-creds"))
	})

	It("UT-2492-DEMO-002 [BR-PLATFORM-014]: fleet mode keeps read and WorkflowExecution write credentials distinct", func() {
		fleet := &FleetHelmOptions{
			MCPGatewayEndpoint:        "https://mcp-gateway.example.invalid/mcp",
			MCPGatewayType:            "eaigw",
			MCPGatewayNamespace:       "mcp-system",
			OAuth2TokenURL:            "https://keycloak.idp.svc/token",
			OAuth2CredentialsSecret:   "fleet-read",
			WEOAuth2CredentialsSecret: "fleet-write",
			OAuth2Scopes:              []string{"kube-mcp-server-audience"},
		}
		opts := DemoHelmOptions{Mode: DemoModeFleet, LLMProvider: "openai", LLMModel: "gpt-4o", LLMEndpoint: "https://api.example.invalid/v1"}
		cr, err := BuildKubernautCRManifest(buildDemoOperatorCROptions(opts, fleet))
		Expect(err).NotTo(HaveOccurred())
		Expect(cr).To(ContainSubstring("credentialsSecretRef: fleet-read"))
		Expect(cr).To(ContainSubstring("oauth2CredentialsSecretRef: fleet-write"))
		Expect(cr).NotTo(ContainSubstring("oauth2CredentialsSecretRef: fleet-read"))
	})
})

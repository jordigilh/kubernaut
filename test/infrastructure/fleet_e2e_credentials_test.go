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
	"context"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"

	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
)

// Issue found 2026-09-01 (FLEET_DEMO_QUICKSTART.md): the docs told users to
// pre-create the llm-credentials-primary Secret before running
// `setup-fleet-demo-infra` at all -- impossible, since the cluster meant to
// hold it doesn't exist yet at that point. SetupFleetCoreInfrastructureWithGateway
// always creates a mock placeholder regardless (createFullPipelineHelmSecrets),
// so the "actionable error if missing" InstallDemoHelmChart's
// checkSecretExists promised was actually unreachable. buildLLMCredentialsSecretManifest
// is the pure builder behind the fix: -llm-credentials-file overwrites that
// placeholder with real content once the cluster exists, closing the gap in
// one command instead of two.
var _ = Describe("buildLLMCredentialsSecretManifest", func() {
	It("UT-INFRA-FLEETDEMO-023: base64-encodes the credentials into both data.api_key and data[credentials.json]", func() {
		manifest := buildLLMCredentialsSecretManifest("kubernaut-system", []byte(`{"type":"authorized_user"}`))
		encoded := base64.StdEncoding.EncodeToString([]byte(`{"type":"authorized_user"}`))
		Expect(manifest).To(ContainSubstring("name: llm-credentials-primary"))
		Expect(manifest).To(ContainSubstring("namespace: kubernaut-system"))
		Expect(manifest).To(ContainSubstring("data:"))
		Expect(manifest).To(ContainSubstring("api_key: " + encoded))
		Expect(manifest).To(ContainSubstring("credentials.json: " + encoded))
	})

	It("UT-INFRA-FLEETDEMO-024: scopes the Secret to the namespace passed in", func() {
		manifest := buildLLMCredentialsSecretManifest("some-other-namespace", []byte("a-plain-api-key"))
		Expect(manifest).To(ContainSubstring("namespace: some-other-namespace"))
		Expect(manifest).NotTo(ContainSubstring("namespace: kubernaut-system"))
	})

	It("UT-INFRA-FLEETDEMO-025: round-trips credential bytes containing YAML-unsafe characters (quotes, newlines)", func() {
		tricky := []byte("{\n  \"private_key\": \"-----BEGIN KEY-----\\nabc\\n-----END KEY-----\"\n}\n")
		manifest := buildLLMCredentialsSecretManifest("kubernaut-system", tricky)
		Expect(manifest).To(ContainSubstring("api_key: " + base64.StdEncoding.EncodeToString(tricky)))
	})
})

// Issue found 2026-09-02 (demo team report): the fleet demo's AlertManager
// runs in a dedicated "monitoring" namespace (DD-EM-005's fleet-wide
// platform-monitoring instance), but Gateway's Service lives in
// "kubernaut-system". DeployAlertManager previously took a single namespace
// argument and used it for BOTH AlertManager's own manifest AND the
// gateway-webhook receiver's URL, so the fleet caller's monitoringNamespace
// leaked into the webhook URL too -- producing an unresolvable
// gateway-service.monitoring.svc.cluster.local address. AlertManager's own
// logs confirmed it: "dial tcp: lookup gateway-service.monitoring.svc.
// cluster.local ... no such host". buildAlertManagerManifest now takes
// gatewayNamespace separately; these tests prove the webhook URL tracks it,
// independent of namespace.
var _ = Describe("buildAlertManagerManifest", func() {
	It("UT-INFRA-FLEETDEMO-026: gateway-webhook URL uses gatewayNamespace, not namespace, when they differ", func() {
		manifest := buildAlertManagerManifest("monitoring", "kubernaut-system", "")
		Expect(manifest).To(ContainSubstring("https://gateway-service.kubernaut-system.svc.cluster.local:8080/api/v1/signals/prometheus"))
		Expect(manifest).NotTo(ContainSubstring("gateway-service.monitoring.svc.cluster.local"))
	})

	It("UT-INFRA-FLEETDEMO-027: AlertManager's own ConfigMap/Deployment/Service still use namespace, not gatewayNamespace", func() {
		manifest := buildAlertManagerManifest("monitoring", "kubernaut-system", "")
		Expect(manifest).To(ContainSubstring("name: alertmanager-config\n  namespace: monitoring"))
		Expect(manifest).To(ContainSubstring("name: alertmanager\n  namespace: monitoring"))
		Expect(manifest).To(ContainSubstring("name: alertmanager-svc\n  namespace: monitoring"))
	})

	It("UT-INFRA-FLEETDEMO-028: single-cluster callers passing the same value for both still resolve correctly", func() {
		manifest := buildAlertManagerManifest("kubernaut-system", "kubernaut-system", "")
		Expect(manifest).To(ContainSubstring("https://gateway-service.kubernaut-system.svc.cluster.local:8080/api/v1/signals/prometheus"))
	})

	It("UT-INFRA-FLEETDEMO-029: BR-GATEWAY-036/037 bearer token is still added to the webhook's http_config when provided", func() {
		manifest := buildAlertManagerManifest("kubernaut-system", "kubernaut-system", "test-token")
		Expect(manifest).To(ContainSubstring("bearer_token: 'test-token'"))
	})

	It("UT-INFRA-FLEET-TLS-001: AlertManager uses the Gateway HTTPS endpoint and mounted CA", func() {
		manifest := buildAlertManagerManifest("monitoring", "kubernaut-system", "test-token")
		Expect(manifest).To(ContainSubstring("https://gateway-service.kubernaut-system.svc.cluster.local:8080/api/v1/signals/prometheus"))
		Expect(manifest).To(ContainSubstring("tls_config:"))
		Expect(manifest).To(ContainSubstring("ca_file: /etc/tls-ca/ca.crt"))
		Expect(manifest).To(ContainSubstring("mountPath: /etc/tls-ca"))
		Expect(manifest).To(ContainSubstring("name: inter-service-ca"))
	})

	It("UT-INFRA-FLEET-TLS-004: AlertManager's required CA volume is namespace-local", func() {
		manifest := buildAlertManagerManifest("monitoring", "kubernaut-system", "test-token")
		Expect(manifest).To(ContainSubstring("name: alertmanager\n  namespace: monitoring"))
		Expect(manifest).To(ContainSubstring("- name: inter-service-ca\n          mountPath: /etc/tls-ca"))
		Expect(manifest).To(ContainSubstring("- name: inter-service-ca\n        configMap:\n          name: inter-service-ca"))
	})

	It("UT-INFRA-FLEET-TLS-003: AlertManager embedded configuration is valid YAML", func() {
		manifest := buildAlertManagerManifest("monitoring", "kubernaut-system", "test-token")
		config := strings.SplitN(manifest, "  alertmanager.yml: |\n", 2)
		Expect(config).To(HaveLen(2))
		configLines := strings.SplitN(config[1], "\n---\n", 2)
		Expect(configLines).To(HaveLen(2))
		indentedLines := strings.Split(configLines[0], "\n")
		for i, line := range indentedLines {
			indentedLines[i] = strings.TrimPrefix(line, "    ")
		}
		var parsed yaml.Node
		Expect(yaml.Unmarshal([]byte(strings.Join(indentedLines, "\n")), &parsed)).To(Succeed())
	})
})

var _ = Describe("Fleet-only identity and webhook setup", func() {
	It("UT-INFRA-FLEET-OIDC-001: skips Dex when Fleet infrastructure is provisioned", func() {
		Expect(shouldDeployDexForAF(nil)).To(BeTrue())
		Expect(shouldDeployDexForAF(func(context.Context, string, string, io.Writer) (*FleetHelmOptions, error) {
			return nil, nil
		})).To(BeFalse())
	})

	It("UT-INFRA-FLEET-OIDC-002: excludes Dex from Fleet readiness checks", func() {
		fleetDeployments := fullPipelineReadinessDeployments(func(context.Context, string, string, io.Writer) (*FleetHelmOptions, error) {
			return nil, nil
		})
		Expect(fleetDeployments).ToNot(ContainElement("dex"))

		fullPipelineDeployments := fullPipelineReadinessDeployments(nil)
		Expect(fullPipelineDeployments).To(ContainElement("dex"))
	})

	It("UT-INFRA-FLEET-TLS-002: event exporter uses the Gateway HTTPS endpoint and mounted CA", func() {
		manifest := buildEventExporterManifest("kubernaut-system", "test-token")
		Expect(manifest).To(ContainSubstring("https://gateway-service.kubernaut-system.svc.cluster.local:8080/api/v1/signals/kubernetes-event"))
		Expect(manifest).To(ContainSubstring("Authorization: \"Bearer test-token\""))
		Expect(manifest).To(ContainSubstring("tls:"))
		Expect(manifest).To(ContainSubstring("caFile: /etc/tls-ca/ca.crt"))
		Expect(manifest).To(ContainSubstring("mountPath: /etc/tls-ca"))
		Expect(manifest).To(ContainSubstring("name: inter-service-ca"))
	})
})

var _ = Describe("kube-mcp-server E2E configuration", func() {
	It("UT-INFRA-FLEET-030: pins the E2E image to the current validated digest", func() {
		Expect(KubeMCPServerImage).To(Equal("ghcr.io/containers/kubernetes-mcp-server@sha256:4219880ffae9b61f5cf8e27c0536d4001336016a6af77dc5a63dfaf9ce938b97"))
	})

	It("UT-INFRA-FLEET-031: renders the current RFC 8693 token exchange schema", func() {
		config := KubeMCPServerAuthConfig{
			Mode:             KubeMCPServerAuthModePassthrough,
			RequireOAuth:     true,
			AuthorizationURL: "https://keycloak.example/realms/fleet",
			OAuthAudience:    "kube-mcp-server",
			StsClientID:      "kube-mcp-server",
			StsClientSecret:  "secret",
			StsAudience:      "k8s-api",
			StsScopes:        []string{"k8s-api-audience"},
			CAFilePath:       "/etc/tls-ca/ca.crt",
		}

		toml := config.tomlString()
		Expect(toml).To(ContainSubstring("[token_exchange]"))
		Expect(toml).To(ContainSubstring("strategy = \"rfc8693\""))
		Expect(toml).To(ContainSubstring("audience = \"k8s-api\""))
		Expect(toml).To(ContainSubstring("scopes = [\"k8s-api-audience\"]"))
		Expect(toml).To(ContainSubstring("[token_exchange.client_auth]"))
		Expect(toml).To(ContainSubstring("method = \"client_secret_basic\""))
		Expect(toml).To(ContainSubstring("client_id = \"kube-mcp-server\""))
		Expect(toml).To(ContainSubstring("client_secret = \"secret\""))
		Expect(toml).NotTo(ContainSubstring("sts_client_id"))
		Expect(toml).NotTo(ContainSubstring("sts_audience"))
	})
})

var _ = Describe("fleet gateway cluster registration identities", func() {
	It("UT-INFRA-FLEET-2441-001: uses hub and EAIGW's hub__ tool prefix for the demo", func() {
		clusterID, toolPrefix := fleetHubRegistrationIdentity(KubeMCPServerAuthConfig{
			GatewayType:  registry.GatewayEAIGW,
			HubClusterID: "hub",
		})

		Expect(clusterID).To(Equal("hub"))
		Expect(toolPrefix).To(Equal("hub__"))
	})

	It("UT-INFRA-FLEET-2441-002: keeps the remote-only EAIGW identity unchanged", func() {
		clusterID, toolPrefix := fleetClusterRegistrationIdentity(KubeMCPServerAuthConfig{
			GatewayType:            registry.GatewayEAIGW,
			AllRegistrationsRemote: true,
			RemoteBridge:           &RemoteClusterBridgeConfig{},
			HubClusterID:           "hub",
		})

		Expect(clusterID).To(Equal("remote-cluster"))
		Expect(toolPrefix).To(Equal("remote-cluster__"))
	})

	It("UT-INFRA-FLEET-2441-003: preserves the Kuadrant prefix convention", func() {
		clusterID, toolPrefix := fleetHubRegistrationIdentity(KubeMCPServerAuthConfig{
			GatewayType:  registry.GatewayKuadrant,
			HubClusterID: "hub",
		})

		Expect(clusterID).To(Equal("hub"))
		Expect(toolPrefix).To(Equal("hub_"))
	})

	It("UT-INFRA-FLEET-2441-004: keeps the primary remote identity while adding hub separately", func() {
		clusterID, toolPrefix := fleetClusterRegistrationIdentity(KubeMCPServerAuthConfig{
			GatewayType:            registry.GatewayEAIGW,
			HubClusterID:           "hub",
			AllRegistrationsRemote: true,
		})
		Expect(clusterID).To(Equal("remote-cluster"))
		Expect(toolPrefix).To(Equal("remote-cluster__"))
	})

	It("UT-INFRA-FLEET-2441-005: aliases the dedicated IdP namespace to the stable issuer hostname", func() {
		manifest := buildKeycloakServiceAliasManifest("kubernaut-system", "idp")

		Expect(manifest).To(ContainSubstring("type: ExternalName"))
		Expect(manifest).To(ContainSubstring("namespace: kubernaut-system"))
		Expect(manifest).To(ContainSubstring("externalName: keycloak.idp.svc.cluster.local"))
	})

	It("UT-INFRA-FLEET-2441-006: skips the alias when IdP and application share a namespace", func() {
		Expect(buildKeycloakServiceAliasManifest("kubernaut-system", "kubernaut-system")).To(BeEmpty())
	})

	It("IT-INFRA-FLEET-2441-007: renders hub and remote EAIGW backends with authorization forwarding", func() {
		manifest := captureKubectlManifest(func() error {
			return deployEnvoyAIGatewayRegistrations(context.Background(), "kubernaut-system", "test-kubeconfig", "http://gateway/mcp", KubeMCPServerAuthConfig{
				GatewayType:            registry.GatewayEAIGW,
				AuthorizationURL:       "https://keycloak:8443/realms/kubernaut-demo",
				OAuthAudience:          "kube-mcp-server",
				HubClusterID:           "hub",
				AllRegistrationsRemote: true,
			}, io.Discard)
		})

		Expect(manifest).To(ContainSubstring("name: hub"))
		Expect(manifest).To(ContainSubstring("name: remote-cluster"))
		Expect(manifest).To(ContainSubstring("name: prod-east"))
		Expect(manifest).To(ContainSubstring("name: prod-west"))
		Expect(manifest).To(ContainSubstring("forwardHeaders:"))
		expectValidYAMLDocuments(manifest)
	})

	It("IT-INFRA-FLEET-2441-008: renders hub and remote Kuadrant registrations", func() {
		manifest := captureKubectlManifest(func() error {
			return deployKuadrantRegistrations(context.Background(), "kubernaut-system", "test-kubeconfig", KubeMCPServerAuthConfig{
				GatewayType:            registry.GatewayKuadrant,
				HubClusterID:           "hub",
				AllRegistrationsRemote: true,
				BrokerCredentialToken:  "token",
			}, io.Discard)
		})

		Expect(manifest).To(ContainSubstring("name: hub"))
		Expect(manifest).To(ContainSubstring("prefix: \"hub_\""))
		Expect(manifest).To(ContainSubstring("name: remote-cluster"))
		Expect(manifest).To(ContainSubstring("prefix: \"remote_cluster_\""))
		Expect(manifest).To(ContainSubstring("name: prod-east"))
		Expect(manifest).To(ContainSubstring("name: prod-west"))
		expectValidYAMLDocuments(manifest)
	})
})

func captureKubectlManifest(run func() error) string {
	dir, err := os.MkdirTemp("", "fleet-kubectl-test-")
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() { _ = os.RemoveAll(dir) })

	manifestPath := filepath.Join(dir, "manifest.yaml")
	kubectlPath := filepath.Join(dir, "kubectl")
	script := "#!/bin/sh\ncat > \"$FLEET_TEST_MANIFEST\"\n"
	Expect(os.WriteFile(kubectlPath, []byte(script), 0o755)).To(Succeed())

	previousPath := os.Getenv("PATH")
	previousManifest := os.Getenv("FLEET_TEST_MANIFEST")
	Expect(os.Setenv("PATH", dir+string(os.PathListSeparator)+previousPath)).To(Succeed())
	Expect(os.Setenv("FLEET_TEST_MANIFEST", manifestPath)).To(Succeed())
	DeferCleanup(func() {
		_ = os.Setenv("PATH", previousPath)
		_ = os.Setenv("FLEET_TEST_MANIFEST", previousManifest)
	})

	Expect(run()).To(Succeed())
	manifest, err := os.ReadFile(manifestPath)
	Expect(err).NotTo(HaveOccurred())
	return string(manifest)
}

func expectValidYAMLDocuments(manifest string) {
	decoder := yaml.NewDecoder(strings.NewReader(manifest))
	for {
		var document yaml.Node
		err := decoder.Decode(&document)
		if errors.Is(err, io.EOF) {
			return
		}
		Expect(err).NotTo(HaveOccurred(), "rendered manifest:\n%s", manifest)
	}
}

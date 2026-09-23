package infrastructure

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"github.com/jordigilh/kubernaut/pkg/fleet"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
)

var _ = Describe("standalone AF Fleet configuration [BR-FLEET-054, BR-INTEGRATION-065]", func() {
	It("UT-INFRA-AF-FLEET-2462-004 [BR-FLEET-054, BR-INTEGRATION-065]: renders matching Keycloak, FMC, and Gateway settings for AF", func() {
		baseConfig, err := os.ReadFile(filepath.Join(getProjectRoot(), "deploy/apifrontend/overlays/e2e/config.yaml"))
		Expect(err).NotTo(HaveOccurred())
		fleetOptions := &FleetHelmOptions{
			MCPGatewayEndpoint:          "http://envoy-ai-gateway.kubernaut-system.svc.cluster.local:8080/mcp",
			MCPGatewayType:              "eaigw",
			OAuth2TokenURL:              "https://keycloak:8443/realms/kubernaut-demo/protocol/openid-connect/token",
			OAuth2CredentialsSecret:     fleetOAuth2SecretName,
			OAuth2Scopes:                []string{"kube-mcp-server-audience"},
			FleetMetadataCacheNamespace: "kubernaut-system",
		}

		rendered, err := buildAPIFrontendFleetConfig(baseConfig, "kubernaut-system", fleetOptions)
		Expect(err).NotTo(HaveOccurred())

		var config struct {
			Auth struct {
				IssuerURL  string `yaml:"issuerURL"`
				JWKSURL    string `yaml:"jwksURL"`
				Audience   string `yaml:"audience"`
				OIDCCaFile string `yaml:"oidcCaFile"`
			} `yaml:"auth"`
			Fleet fleet.FleetConfig `yaml:"fleet"`
		}
		Expect(yaml.Unmarshal(rendered, &config)).To(Succeed())
		Expect(config.Auth.IssuerURL).To(Equal("https://keycloak:8443/realms/kubernaut-demo"))
		Expect(config.Auth.JWKSURL).To(Equal("https://keycloak.kubernaut-system.svc.cluster.local:8443/realms/kubernaut-demo/protocol/openid-connect/certs"))
		Expect(config.Auth.Audience).To(Equal("kubernaut-apifrontend"))
		Expect(config.Auth.OIDCCaFile).To(Equal("/etc/apifrontend/inter-service-ca/ca.crt"))
		Expect(config.Fleet.Enabled).To(BeTrue())
		Expect(config.Fleet.Backend).To(Equal(fleet.BackendFMC))
		Expect(config.Fleet.Endpoint).To(Equal("https://fleetmetadatacache-service.kubernaut-system.svc.cluster.local:8080"))
		Expect(config.Fleet.MCPGatewayEndpoint).To(Equal(fleetOptions.MCPGatewayEndpoint))
		Expect(config.Fleet.EffectiveMCPGatewayType()).To(Equal(fleet.GatewayEAIGW))
		Expect(config.Fleet.OAuth2.Enabled).To(BeTrue())
		Expect(config.Fleet.OAuth2.TokenURL).To(Equal(fleetOptions.OAuth2TokenURL))
		Expect(config.Fleet.OAuth2.CredentialsSecretRef).To(Equal(fleetOAuth2SecretName))
		Expect(config.Fleet.OAuth2.TLSCAFile).To(Equal("/etc/apifrontend/inter-service-ca/ca.crt"))
		Expect(config.Fleet.Validate()).To(Succeed())
	})

	It("UT-INFRA-AF-FLEET-2462-005 [BR-FLEET-054, BR-INTEGRATION-065]: renders KA Gateway auth and Keycloak JWT claims", func() {
		baseConfig := []byte(`runtime:
  logging:
    level: debug
interactive:
  rateLimitPerUser: 100
integrations:
  dataStorage:
    url: https://data-storage-service:8080
`)
		fleetOptions := &FleetHelmOptions{
			MCPGatewayEndpoint:      "http://envoy-ai-gateway.kubernaut-system.svc.cluster.local:8080/mcp",
			MCPGatewayType:          "eaigw",
			OAuth2TokenURL:          "https://keycloak:8443/realms/kubernaut-demo/protocol/openid-connect/token",
			OAuth2CredentialsSecret: fleetOAuth2SecretName,
			OAuth2Scopes:            []string{"kube-mcp-server-audience"},
		}

		rendered, err := buildKubernautAgentFleetConfig(baseConfig, "kubernaut-system", fleetOptions)
		Expect(err).NotTo(HaveOccurred())
		var config struct {
			Integrations struct {
				Fleet struct {
					Endpoint    string `yaml:"endpoint"`
					GatewayType string `yaml:"gatewayType"`
					OAuth2      struct {
						Enabled              bool     `yaml:"enabled"`
						TokenURL             string   `yaml:"tokenURL"`
						CredentialsSecretRef string   `yaml:"credentialsSecretRef"`
						Scopes               []string `yaml:"scopes"`
						TLSCAFile            string   `yaml:"tlsCaFile"`
					} `yaml:"oauth2"`
				} `yaml:"fleet"`
			} `yaml:"integrations"`
			Interactive struct {
				JWTProviders []struct {
					Issuer        string            `yaml:"issuer"`
					JWKSURL       string            `yaml:"jwksURL"`
					Audience      string            `yaml:"audience"`
					TLSCAFile     string            `yaml:"tlsCaFile"`
					ClaimMappings map[string]string `yaml:"claimMappings"`
				} `yaml:"jwtProviders"`
			} `yaml:"interactive"`
		}
		Expect(yaml.Unmarshal(rendered, &config)).To(Succeed())
		Expect(config.Integrations.Fleet.Endpoint).To(Equal(fleetOptions.MCPGatewayEndpoint))
		Expect(config.Integrations.Fleet.GatewayType).To(Equal("eaigw"))
		Expect(config.Integrations.Fleet.OAuth2.Enabled).To(BeTrue())
		Expect(config.Integrations.Fleet.OAuth2.TokenURL).To(Equal(fleetOptions.OAuth2TokenURL))
		Expect(config.Integrations.Fleet.OAuth2.CredentialsSecretRef).To(Equal(fleetOAuth2SecretName))
		Expect(config.Integrations.Fleet.OAuth2.Scopes).To(Equal(fleetOptions.OAuth2Scopes))
		Expect(config.Integrations.Fleet.OAuth2.TLSCAFile).To(Equal("/etc/tls-ca/ca.crt"))
		Expect(config.Interactive.JWTProviders).To(HaveLen(1))
		Expect(config.Interactive.JWTProviders[0].Issuer).To(Equal("https://keycloak:8443/realms/kubernaut-demo"))
		Expect(config.Interactive.JWTProviders[0].JWKSURL).To(ContainSubstring("keycloak.kubernaut-system.svc.cluster.local"))
		Expect(config.Interactive.JWTProviders[0].Audience).To(Equal("kubernaut-apifrontend"))
		Expect(config.Interactive.JWTProviders[0].TLSCAFile).To(Equal("/etc/tls-ca/ca.crt"))
		Expect(config.Interactive.JWTProviders[0].ClaimMappings).To(HaveKeyWithValue("groups", "groups"))
	})

	It("UT-INFRA-AF-FLEET-2462-006 [BR-RBAC-020, BR-FLEET-054]: grants AF only the scoped EAIGW registry permissions it needs", func() {
		manifest := buildAFleetRBACManifest("kubernaut-system")
		Expect(manifest).To(ContainSubstring("resources: [\"backends\"]"))
		Expect(manifest).To(ContainSubstring("resources: [\"mcproutes\"]"))
		expectValidYAMLDocuments(manifest)
	})

	It("IT-INFRA-AF-FLEET-2462-007 [BR-FLEET-054]: deploys AF with the Fleet OAuth secret and routing config", func() {
		fleetOptions := &FleetHelmOptions{
			MCPGatewayEndpoint:          "http://envoy-ai-gateway.kubernaut-system.svc.cluster.local:8080/mcp",
			MCPGatewayType:              "eaigw",
			OAuth2TokenURL:              "https://keycloak:8443/realms/kubernaut-demo/protocol/openid-connect/token",
			OAuth2CredentialsSecret:     fleetOAuth2SecretName,
			OAuth2Scopes:                []string{"kube-mcp-server-audience"},
			FleetMetadataCacheNamespace: "kubernaut-system",
		}
		manifest := captureKubectlManifest(func() error {
			return deployAPIFrontendService(context.Background(), "test-kubeconfig", "kubernaut-system", "localhost/apifrontend:test", false, fleetOptions, io.Discard)
		})

		Expect(manifest).To(ContainSubstring("mcpGatewayEndpoint: http://envoy-ai-gateway.kubernaut-system.svc.cluster.local:8080/mcp"))
		Expect(manifest).To(ContainSubstring("issuerURL: https://keycloak:8443/realms/kubernaut-demo"))
		Expect(manifest).To(ContainSubstring("mountPath: /etc/apifrontend/" + fleetOAuth2SecretName))
		Expect(manifest).To(ContainSubstring("secretName: " + fleetOAuth2SecretName))
		expectValidYAMLDocuments(manifest)
	})

	It("UT-INFRA-AF-FLEET-2462-008 [BR-INTEGRATION-065]: Fleet AF Kind config exposes only the required Fleet NodePorts", func() {
		data, err := os.ReadFile(filepath.Join(getProjectRoot(), "test/infrastructure/kind-apifrontend-fleet-config.yaml"))
		Expect(err).NotTo(HaveOccurred())

		var config struct {
			Nodes []struct {
				ExtraPortMappings []struct {
					ContainerPort int `yaml:"containerPort"`
					HostPort      int `yaml:"hostPort"`
				} `yaml:"extraPortMappings"`
			} `yaml:"nodes"`
		}
		Expect(yaml.Unmarshal(data, &config)).To(Succeed())
		Expect(config.Nodes).NotTo(BeEmpty())
		var mappedPorts []int
		for _, mapping := range config.Nodes[0].ExtraPortMappings {
			if mapping.ContainerPort == 30557 || mapping.ContainerPort == eaigwGatewayNodePort {
				Expect(mapping.HostPort).To(Equal(mapping.ContainerPort), "Fleet port mappings are not offset")
				mappedPorts = append(mappedPorts, mapping.ContainerPort)
			}
		}
		Expect(mappedPorts).To(ConsistOf(30557, eaigwGatewayNodePort),
			"Fleet AF needs host access to Keycloak and the EAIGW Gateway")
	})

	It("UT-INFRA-AF-FLEET-2462-009 [BR-INTEGRATION-065]: KA mounts the shared Fleet OAuth2 secret at its configured path", func() {
		patch, err := buildKubernautAgentFleetDeploymentPatch(&FleetHelmOptions{
			OAuth2CredentialsSecret: fleetOAuth2SecretName,
		})
		Expect(err).NotTo(HaveOccurred())
		var deploymentPatch struct {
			Spec struct {
				Template struct {
					Spec struct {
						Containers []struct {
							Name         string `json:"name"`
							VolumeMounts []struct {
								Name      string `json:"name"`
								MountPath string `json:"mountPath"`
								ReadOnly  bool   `json:"readOnly"`
							} `json:"volumeMounts"`
						} `json:"containers"`
						Volumes []struct {
							Name   string `json:"name"`
							Secret struct {
								SecretName string `json:"secretName"`
							} `json:"secret"`
						} `json:"volumes"`
					} `json:"spec"`
				} `json:"template"`
			} `json:"spec"`
		}
		Expect(json.Unmarshal(patch, &deploymentPatch)).To(Succeed())
		Expect(deploymentPatch.Spec.Template.Spec.Containers).To(HaveLen(1))
		Expect(deploymentPatch.Spec.Template.Spec.Containers[0].Name).To(Equal("kubernaut-agent"))
		Expect(deploymentPatch.Spec.Template.Spec.Containers[0].VolumeMounts).To(HaveLen(1))
		Expect(deploymentPatch.Spec.Template.Spec.Containers[0].VolumeMounts[0].Name).To(Equal("fleet-oauth2-credentials"))
		Expect(deploymentPatch.Spec.Template.Spec.Containers[0].VolumeMounts[0].MountPath).To(Equal("/etc/kubernaut-agent/" + fleetOAuth2SecretName))
		Expect(deploymentPatch.Spec.Template.Spec.Containers[0].VolumeMounts[0].ReadOnly).To(BeTrue())
		Expect(deploymentPatch.Spec.Template.Spec.Volumes).To(HaveLen(1))
		Expect(deploymentPatch.Spec.Template.Spec.Volumes[0].Secret.SecretName).To(Equal(fleetOAuth2SecretName))
	})
})

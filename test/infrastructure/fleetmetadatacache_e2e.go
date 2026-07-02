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
	"fmt"
	"io"
	"os"

	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
)

// keycloakHostPortFMC is the Kind extraPortMappings host port for Keycloak in
// kind-fleetmetadatacache-config.yaml. Matches the NodePort (30557) directly,
// following the Dex convention this lane previously used -- see
// waitForKeycloakReady in keycloak_e2e.go.
const keycloakHostPortFMC = 30557

// SetupFMCE2EInfrastructure deploys the dedicated Fleet Metadata Cache (FMC)
// E2E stack: Keycloak (OIDC IdP + RFC 8693 token exchange) + the fleet-core
// stack (Istio + Kuadrant MCP Gateway + kube-mcp-server + Valkey + FMC) via
// DeployFleetCoreInfra.
//
// DataStorage is deliberately NOT deployed here: FMC is audit-exempt
// (DD-AUDIT-003) and never calls DataStorage's API, so unlike other E2E
// lanes this one has no audit-trail dependency to satisfy.
//
// Unlike the "fleet" E2E suite (test/e2e/fleet/), this lane does NOT deploy
// Gateway, RemediationOrchestrator, or the other 8+ Kubernaut services --
// it proves FMC's own journeys in isolation (BR-INTEGRATION-065):
//   - Real OAuth2 client_credentials token acquisition from Keycloak
//   - Real discovery of MCPServerRegistrations via the Kuadrant MCP Gateway
//   - Real Valkey-backed scope resolution served by FMC's HTTP API
//   - Real RFC 8693 Standard Token Exchange: kube-mcp-server runs in
//     passthrough mode and exchanges FMC's token for a K8s-API-scoped token,
//     preserving FMC's identity end-to-end (Spike S17/S18; see
//     docs/testing/BR-INTEGRATION-054/TEST_PLAN.md E2E-FMC-054-014).
//
// Keycloak replaces Dex in this lane only -- the full "fleet" suite
// (test/infrastructure/fleet_e2e.go DeployFleetInfra) still uses Dex, since
// Dex does not implement RFC 8693 Standard Token Exchange.
//
// Authority: Issue #54, ADR-068, BR-INTEGRATION-065.
func SetupFMCE2EInfrastructure(ctx context.Context, clusterName, kubeconfigPath string, writer io.Writer) (fmcImage string, err error) {
	return setupFMCE2EInfrastructure(ctx, clusterName, kubeconfigPath, registry.GatewayKuadrant, writer)
}

// SetupFMCE2EInfrastructureEAIGW is the Envoy AI Gateway (EAIGW) sibling of
// SetupFMCE2EInfrastructure (Spike S18, Phase B): identical Phases 1-6
// (image, Kind cluster, TLS, Keycloak, API server OIDC patch) and the same
// passthrough+STS kube-mcp-server token-exchange design, but Phase 7 fronts
// kube-mcp-server with Envoy AI Gateway instead of Kuadrant
// (deployEnvoyAIGatewayInfra/deployEnvoyAIGatewayRegistrations in
// fleet_e2e.go) -- see the "Key design decision" section of the EAIGW plan
// doc for why the RFC 8693 exchange itself needs no gateway-specific changes.
//
// Uses kind-fleetmetadatacache-eaigw-config.yaml (a separate, isolated Kind
// cluster) so this lane can run alongside the Kuadrant lane in CI without
// port collisions.
func SetupFMCE2EInfrastructureEAIGW(ctx context.Context, clusterName, kubeconfigPath string, writer io.Writer) (fmcImage string, err error) {
	return setupFMCE2EInfrastructure(ctx, clusterName, kubeconfigPath, registry.GatewayEAIGW, writer)
}

func setupFMCE2EInfrastructure(ctx context.Context, clusterName, kubeconfigPath string, gatewayType registry.MCPGatewayType, writer io.Writer) (fmcImage string, err error) {
	gatewayLabel := "Kuadrant MCP Gateway"
	kindConfigPath := "test/infrastructure/kind-fleetmetadatacache-config.yaml"
	mcpGatewayNodePort := 31975
	loopbackToolPrefix := "loopback_cluster_"
	if gatewayType == registry.GatewayEAIGW {
		gatewayLabel = "Envoy AI Gateway (EAIGW)"
		kindConfigPath = "test/infrastructure/kind-fleetmetadatacache-eaigw-config.yaml"
		mcpGatewayNodePort = eaigwGatewayNodePort
		// EAIGW does not support an explicit tool-name prefix (unlike Kuadrant's
		// MCPServerRegistration.spec.prefix) -- it auto-derives "<Backend name>__"
		// from the Backend, confirmed against the Backend named "loopback-cluster"
		// registered in deployEnvoyAIGatewayRegistrations (Spike S18).
		loopbackToolPrefix = "loopback-cluster__"
	}

	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "🚀 Fleet Metadata Cache (FMC) E2E Infrastructure")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintf(writer, "  Deploys: Keycloak + Fleet Core (%s/kube-mcp-server/Valkey/FMC)\n", gatewayLabel)
	_, _ = fmt.Fprintln(writer, "  Skips: Gateway, RemediationOrchestrator, and other Kubernaut services")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	namespace := "kubernaut-system"

	// ── Phase 1: Build fleetmetadatacache image (before cluster creation) ──
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 1: Building fleetmetadatacache image (NO CLUSTER YET)...")
	buildCfg := E2EImageConfig{
		ServiceName:      "fleetmetadatacache",
		ImageName:        "fleetmetadatacache",
		DockerfilePath:   "docker/fleetmetadatacache.Dockerfile",
		BuildContextPath: "",
		EnableCoverage:   os.Getenv("E2E_COVERAGE") == "true",
	}
	fmcImage, err = BuildImageForKind(buildCfg, writer)
	if err != nil {
		return "", fmt.Errorf("fleetmetadatacache image build failed: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "  ✅ FleetMetadataCache image built: %s\n", fmcImage)

	// ── Phase 2: Create Kind cluster + namespace ─────────────────────────
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 2: Creating Kind cluster + namespace...")
	opts := KindClusterOptions{
		ClusterName:             clusterName,
		KubeconfigPath:          kubeconfigPath,
		ConfigPath:              kindConfigPath,
		WaitTimeout:             "5m",
		DeleteExisting:          false,
		ReuseExisting:           false,
		UsePodman:               true,
		ProjectRootAsWorkingDir: true, // DD-TEST-007: For ./coverdata resolution
	}
	if clusterErr := CreateKindClusterWithConfig(opts, writer); clusterErr != nil {
		return "", fmt.Errorf("failed to create Kind cluster: %w", clusterErr)
	}

	if nsErr := createTestNamespace(namespace, kubeconfigPath, writer); nsErr != nil {
		return "", fmt.Errorf("failed to create namespace: %w", nsErr)
	}

	// ── Phase 3: Load image into Kind ────────────────────────────────────
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 3: Loading image into Kind...")
	if loadErr := LoadImageToKind(fmcImage, "fleetmetadatacache", clusterName, writer); loadErr != nil {
		return "", fmt.Errorf("failed to load fleetmetadatacache image: %w", loadErr)
	}

	// ── Phase 4: Inter-service TLS (must precede Keycloak) ───────────────
	// No audit signing cert here (GenerateSigningCertSecret): that secret is
	// only consumed by DataStorage's hash-chain signing, and this lane does
	// not deploy DataStorage (FMC is audit-exempt, DD-AUDIT-003).
	_, _ = fmt.Fprintln(writer, "\n🔐 PHASE 4: Generating inter-service TLS...")
	if _, tlsErr := GenerateInterServiceTLS(ctx, kubeconfigPath, namespace, writer); tlsErr != nil {
		return "", fmt.Errorf("failed to generate inter-service TLS: %w", tlsErr)
	}

	// ── Phase 5: Keycloak OIDC + token-exchange provider (must be ready before API server OIDC patch) ─
	_, _ = fmt.Fprintln(writer, "\n🔑 PHASE 5: Deploying Keycloak OIDC provider...")
	if kcErr := DeployKeycloakInfra(ctx, namespace, kubeconfigPath, keycloakHostPortFMC, writer); kcErr != nil {
		return "", fmt.Errorf("failed to deploy Keycloak: %w", kcErr)
	}

	// ── Phase 6: Patch API server for OIDC (needs Keycloak already running) ──
	// ClientID is "k8s-api", NOT "kubernaut-fleet-read": the API server must
	// validate the *exchanged* token's audience (kube-mcp-server presents it
	// after RFC 8693 exchange), not the original caller's token audience.
	// UsernameClaim=preferred_username: Spike S18 proved this claim survives
	// the exchange unchanged, preserving FMC's identity for RBAC purposes.
	_, _ = fmt.Fprintln(writer, "\n🔑 PHASE 6: Patching API server for OIDC (Keycloak issuer, k8s-api audience)...")
	oidcCfg := OIDCPatchConfig{
		IssuerURL:      "https://keycloak:8443/realms/kubernaut-fleet",
		ClientID:       "k8s-api",
		UsernameClaim:  "preferred_username",
		UsernamePrefix: "keycloak:",
	}
	if oidcErr := patchAPIServerForOIDCConfig(ctx, clusterName, kubeconfigPath, oidcCfg, writer); oidcErr != nil {
		return "", fmt.Errorf("API server OIDC patching failed: %w", oidcErr)
	}

	// ── Phase 7: Fleet core (Istio + Kuadrant + kube-mcp-server + Valkey + FMC) ─
	// kube-mcp-server runs in passthrough mode with RFC 8693 token exchange
	// (Spike S17/S18): it forwards FMC's incoming Bearer token, exchanges it
	// for a token scoped to the "k8s-api" audience, and presents that
	// exchanged token to the Kubernetes API server -- validating the real
	// production token-exchange wiring end-to-end (E2E-FMC-054-014).
	_, _ = fmt.Fprintln(writer, "\n🌐 PHASE 7: Deploying fleet-core infrastructure (kube-mcp-server: passthrough + token exchange)...")

	// FMC's own client_credentials grant now goes to Keycloak instead of DEX
	// (Keycloak replaces DEX in this lane -- Spike S17/S18).
	fmcOAuth2Config := FMCOAuth2Config{
		TokenURL:     "https://keycloak:8443/realms/kubernaut-fleet/protocol/openid-connect/token",
		ClientID:     "kubernaut-fleet-read",
		ClientSecret: "e2e-fleet-secret",
		// Keycloak's kubernaut-fleet-read client has no "openid"/"groups"
		// scope assigned (pkg/fleet/fmc/config's DEX-flavored default);
		// request the audience-mapper scope instead (Spike S17/S18).
		Scopes: []string{"kube-mcp-server-audience"},
	}

	kubeMCPAuthConfig := KubeMCPServerAuthConfig{
		Mode:             KubeMCPServerAuthModePassthrough,
		GatewayType:      gatewayType,
		RequireOAuth:     true,
		AuthorizationURL: "https://keycloak:8443/realms/kubernaut-fleet",
		OAuthAudience:    "kube-mcp-server",
		StsClientID:      "kube-mcp-server",
		StsClientSecret:  "e2e-kube-mcp-server-secret",
		StsAudience:      "k8s-api",
		StsScopes:        []string{"k8s-api-audience"},
		CAFilePath:       "/etc/tls-ca/ca.crt",
	}

	// Only Kuadrant needs a static broker credential: its broker maintains
	// its own upstream tool-discovery connection to kube-mcp-server,
	// separate from per-request tools/call proxying, and that connection is
	// itself subject to kube-mcp-server's RequireOAuth check. EAIGW has no
	// separate broker component (MCPRoute.backendRefs aggregates natively),
	// so there is no discovery connection needing its own credential.
	if gatewayType != registry.GatewayEAIGW {
		brokerCredToken, brokerCredErr := GetKeycloakClientCredentialsToken(KeycloakFleetTokenConfig{
			TokenEndpoint: fmt.Sprintf("https://localhost:%d/realms/kubernaut-fleet/protocol/openid-connect/token", keycloakHostPortFMC),
			ClientID:      fmcOAuth2Config.ClientID,
			ClientSecret:  fmcOAuth2Config.ClientSecret,
			Scopes:        fmcOAuth2Config.Scopes,
		})
		if brokerCredErr != nil {
			return "", fmt.Errorf("failed to obtain Kuadrant broker's kube-mcp-server discovery credential: %w", brokerCredErr)
		}
		kubeMCPAuthConfig.BrokerCredentialToken = brokerCredToken
	}

	if coreErr := DeployFleetCoreInfra(ctx, namespace, kubeconfigPath, fmcImage, kubeMCPAuthConfig, fmcOAuth2Config, writer); coreErr != nil {
		return "", fmt.Errorf("fleet-core infra deployment failed: %w", coreErr)
	}

	// ── Phase 7b: RBAC for the token-exchange-preserved FMC identity ────
	// kube-mcp-server's exchanged token carries FMC's original identity
	// (service-account-kubernaut-fleet-read, Spike S18), not kube-mcp-server's
	// own. Bind that identity directly to "view" so the exchanged calls
	// authorize the same way FMC's own dedicated ServiceAccount would have
	// under cluster_auth_mode=kubeconfig.
	_, _ = fmt.Fprintln(writer, "\n🔑 PHASE 7b: Creating RBAC for the exchanged FMC identity...")
	if rbacErr := applyExchangedIdentityRBAC(ctx, kubeconfigPath, writer); rbacErr != nil {
		return "", fmt.Errorf("exchanged-identity RBAC creation failed: %w", rbacErr)
	}

	// Keycloak-based token func for the readiness probe's authenticated
	// tools/call, matching FMC's own runtime OAuth2 config above.
	keycloakFleetReadTokenFunc := func() (string, error) {
		return GetKeycloakClientCredentialsToken(KeycloakFleetTokenConfig{
			TokenEndpoint: fmt.Sprintf("https://localhost:%d/realms/kubernaut-fleet/protocol/openid-connect/token", keycloakHostPortFMC),
			ClientID:      fmcOAuth2Config.ClientID,
			ClientSecret:  fmcOAuth2Config.ClientSecret,
			Scopes:        fmcOAuth2Config.Scopes,
		})
	}
	if readyErr := WaitForFleetReady(keycloakFleetReadTokenFunc, mcpGatewayNodePort, loopbackToolPrefix, writer); readyErr != nil {
		return "", fmt.Errorf("fleet readiness check failed: %w", readyErr)
	}

	// ── Phase 8: Expose FMC's own API via NodePort ───────────────────────
	// DD-TEST-001 mandates NodePort over kubectl port-forward for E2E test
	// stability. DeployFleetCoreInfra only creates a ClusterIP Service for
	// FMC (fleetmetadatacache-service, for in-cluster GW/RO callers), so this
	// is an additive Service selecting the same pods -- no shared-code change.
	_, _ = fmt.Fprintln(writer, "\n🔌 PHASE 8: Exposing FMC API via NodePort (DD-TEST-001)...")
	fmcNodePortManifest := fmt.Sprintf(`---
apiVersion: v1
kind: Service
metadata:
  name: fleetmetadatacache-e2e-nodeport
  namespace: %s
  labels:
    app: fleetmetadatacache
    component: fleet-e2e
spec:
  type: NodePort
  selector:
    app: fleetmetadatacache
  ports:
  - name: api
    port: 8080
    targetPort: 8080
    nodePort: 30150
`, namespace)
	if npErr := kubectlApplyManifest(ctx, kubeconfigPath, writer, fmcNodePortManifest); npErr != nil {
		return "", fmt.Errorf("failed to expose FMC API via NodePort: %w", npErr)
	}
	_, _ = fmt.Fprintln(writer, "    ✅ FMC API reachable at http://localhost:8150")

	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "✅ FMC E2E Infrastructure READY")
	_, _ = fmt.Fprintln(writer, "  FMC API:      http://localhost:8150")
	_, _ = fmt.Fprintf(writer, "  MCP Gateway:  http://localhost:%d/mcp (%s)\n", mcpGatewayNodePort, gatewayLabel)
	_, _ = fmt.Fprintln(writer, "  Keycloak:     https://localhost:30557/realms/kubernaut-fleet")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	return fmcImage, nil
}

// applyExchangedIdentityRBAC binds the K8s identity that survives kube-mcp-server's
// RFC 8693 token exchange (Spike S18: preferred_username=service-account-kubernaut-fleet-read,
// namespaced by --oidc-username-prefix=keycloak: in patchAPIServerForOIDCConfig)
// to the "view" ClusterRole, matching the read-only scope FMC's dedicated
// ServiceAccount would have under cluster_auth_mode=kubeconfig.
func applyExchangedIdentityRBAC(ctx context.Context, kubeconfigPath string, writer io.Writer) error {
	rbacManifest := `---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: fmc-exchanged-identity-binding
  labels:
    app: fleetmetadatacache
    component: fleet-e2e
    authorization: token-exchange-spike-s17-s18
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: view
subjects:
- kind: User
  name: "keycloak:service-account-kubernaut-fleet-read"
  apiGroup: rbac.authorization.k8s.io
`
	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, rbacManifest); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(writer, "  ✅ RBAC created for keycloak:service-account-kubernaut-fleet-read (view)")
	return nil
}

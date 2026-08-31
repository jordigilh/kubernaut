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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
)

// goconst dedup: test-fixture literals deduplicated below.
const (
	kubeMcpServerRoute       = "kube-mcp-server-route"
	kubeMcpServerRemoteRoute = "kube-mcp-server-remote-route"
)

// KubeMCPServerImage is the Go-native K8s MCP server image.
// v0.0.63: supports HTTP mode, in-cluster auth, core toolsets.
const KubeMCPServerImage = "ghcr.io/containers/kubernetes-mcp-server:latest"

const (
	kuadrantControllerImage  = "ghcr.io/kuadrant/mcp-controller:v0.7.1"
	kuadrantBrokerImage      = "ghcr.io/kuadrant/mcp-gateway:v0.7.1"
	valkeyImage              = "docker.io/valkey/valkey:8.1"
	gatewayAPICRDsURL        = "https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.4.1/standard-install.yaml"
	kuadrantCRDsKustomize    = "https://github.com/Kuadrant/mcp-gateway/config/crd?ref=v0.7.1"
	kuadrantOverlayKustomize = "https://github.com/Kuadrant/mcp-gateway/config/mcp-gateway/overlays/mcp-system?ref=v0.7.1"
	istioHelmRepoURL         = "https://istio-release.storage.googleapis.com/charts"
	// traefikHelmRepoURL: ingress controller for manual setup-e2e-fleet-infra
	// Console access only (SetupFleetCoreInfrastructure). ingress-nginx was
	// considered but is EOL (best-effort maintenance ended March 2026, no
	// further security fixes) -- Traefik is actively maintained and needs no
	// CRDs for plain networking.k8s.io/v1 Ingress (its Kubernetes Ingress
	// provider is enabled by default). Not used by the "fleet"/"fullpipeline"
	// Ginkgo suites, which never open a browser against Console.
	traefikHelmRepoURL       = "https://traefik.github.io/charts"
	traefikWebNodePort       = 30880
	traefikWebsecureNodePort = 30843

	// Envoy AI Gateway (EAIGW): two separate Helm installs layered on CNCF
	// Envoy Gateway, per Spike S18. ai-gateway-helm v1.0.0 requires exactly
	// Envoy Gateway v1.8.1 (mismatched versions crash-loop the AI Gateway
	// controller) and does NOT bundle its own CRDs (a separate
	// ai-gateway-crds-helm chart is required).
	envoyGatewayHelmChart   = "oci://docker.io/envoyproxy/gateway-helm"
	envoyGatewayHelmVersion = "v1.8.1"
	aiGatewayCRDsHelmChart  = "oci://docker.io/envoyproxy/ai-gateway-crds-helm"
	aiGatewayHelmChart      = "oci://docker.io/envoyproxy/ai-gateway-helm"
	aiGatewayHelmVersion    = "v1.0.0"
	// eaigwGatewayNodePort is the DD-TEST-001-allocated NodePort for the
	// EAIGW FMC E2E lane's Gateway listener (next in the Kuadrant-31975 block).
	eaigwGatewayNodePort = 31976

	// gatewayLabelKuadrant/gatewayLabelEAIGW: human-readable gateway-type
	// labels shared across provisionFleetCoreInfra/DeployFleetCoreInfra
	// (fleet_e2e.go) and setupFMCE2EInfrastructure (fleetmetadatacache_e2e.go)
	// -- goconst (Issue #1546 Tier 4).
	gatewayLabelKuadrant = "Kuadrant MCP Gateway"
	gatewayLabelEAIGW    = "Envoy AI Gateway (EAIGW)"

	// KubeMCPServerAuthModeKubeconfig makes kube-mcp-server ignore any
	// caller-forwarded Authorization header and always use its own
	// ServiceAccount (ADR-068 Decision #9, "no token delegation"). This is
	// the "fleet" full-pipeline E2E suite's mode (Issue #54 RCA).
	KubeMCPServerAuthModeKubeconfig = "kubeconfig"
	// KubeMCPServerAuthModePassthrough makes kube-mcp-server forward the
	// caller's Bearer token to the target Kubernetes API server, optionally
	// exchanging it first via RFC 8693 Standard Token Exchange when the STS
	// fields below are set. Used by the FMC E2E lane (Spike S17/S18) to
	// validate the real production token-exchange wiring end-to-end.
	KubeMCPServerAuthModePassthrough = "passthrough"
)

// KubeMCPServerAuthConfig configures how kube-mcp-server authenticates to the
// target Kubernetes API server. See KubeMCPServerAuthModeKubeconfig and
// KubeMCPServerAuthModePassthrough for the two supported modes.
type KubeMCPServerAuthConfig struct {
	// Mode is KubeMCPServerAuthModeKubeconfig or KubeMCPServerAuthModePassthrough.
	// Empty defaults to KubeMCPServerAuthModeKubeconfig.
	Mode string

	// GatewayType selects which MCP Gateway implementation fronts
	// kube-mcp-server: registry.GatewayKuadrant (default, zero value) or
	// registry.GatewayEAIGW (Spike S17/S18). The RFC 8693 token-exchange
	// wiring below (StsClientID etc.) lives entirely inside kube-mcp-server
	// and is identical for both gateways -- only the edge routing/OAuth
	// validation layer differs (ADR-068 Decision #9).
	GatewayType registry.MCPGatewayType

	// The following fields only apply when Mode == KubeMCPServerAuthModePassthrough.

	// RequireOAuth/AuthorizationURL/OAuthAudience configure kube-mcp-server as
	// an OAuth resource server, validating the caller's incoming Bearer token
	// (require_oauth/authorization_url/oauth_audience).
	RequireOAuth     bool
	AuthorizationURL string
	OAuthAudience    string
	// StsClientID/StsClientSecret/StsAudience drive the RFC 8693 token
	// exchange. Deliberately NOT setting token_exchange_strategy: Spike S18
	// found the pluggable "keycloak-v1" exchanger never sets
	// subject_token_type (pkg/kubernetes/provider_token_exchange.go), which
	// real Keycloak rejects. Leaving the strategy unset routes through the
	// older pkg/kubernetes/sts.go path (Google's externalaccount package),
	// which hardcodes the correct subject_token_type and speaks plain
	// RFC 8693 -- exactly what Keycloak implements.
	StsClientID     string
	StsClientSecret string
	StsAudience     string
	// StsScopes is REQUIRED for Keycloak targets even when the requested
	// scope is already one of the STS client's defaultClientScopes: the
	// externalaccount library always sends a "scope" request parameter
	// (empty string when StsScopes is nil), and Keycloak's token-exchange
	// endpoint rejects an explicitly-empty scope with "invalid_scope:
	// Invalid scopes: " rather than treating it as "no scope filter".
	// kube-mcp-server's own docs (KEYCLOAK_OIDC_SETUP.md) confirm this
	// exact pattern: sts_scopes = ["mcp:openshift"] is set even though
	// "mcp:openshift" is already a default scope of the exchanging client.
	StsScopes []string
	// CAFilePath is the in-container path to the CA bundle trusted for the
	// authorization/STS endpoint's TLS certificate (certificate_authority).
	CAFilePath string

	// BrokerCredentialToken, when set, is a static Bearer token given to the
	// Kuadrant MCP Gateway broker (via MCPServerRegistration.credentialRef)
	// for its own upstream tool-discovery/session-management connection to
	// kube-mcp-server. Kuadrant docs are explicit that this credential is
	// SEPARATE from, and never injected into, client tools/call requests
	// (https://docs.kuadrant.io/dev/mcp-gateway/docs/reference/mcpserverregistration/):
	// the broker still needs its own credential to keep its discovery
	// connection authenticated when RequireOAuth=true, because the broker's
	// discovery/health probe is not itself a forwarded client request. Empty
	// when RequireOAuth=false (kubeconfig mode), where the broker's
	// unauthenticated discovery connection is accepted as-is.
	//
	// Must carry the OAuthAudience claim kube-mcp-server's resource-server
	// validation expects, and must outlive the E2E run (the token is static
	// for the lifetime of the cluster -- see accessTokenLifespan in
	// keycloak-realm-fleet.json).
	BrokerCredentialToken string

	// RemoteBridge, when non-nil, makes the "prod-east" registration target
	// a genuinely separate Kind cluster's kube-mcp-server via a
	// Service+Endpoints bridge (DD-TEST-013, Spike S19) instead of the
	// local loopback kube-mcp-server every other registration uses. Nil
	// (the default, zero value) preserves the original loopback-only
	// behavior for every existing caller -- only the FMC E2E lanes
	// (fleetmetadatacache_e2e.go) set this field.
	RemoteBridge *RemoteClusterBridgeConfig

	// AllRegistrationsRemote, when true (requires RemoteBridge to be
	// non-nil), makes ALL THREE registrations (the first one renamed
	// "remote-cluster" instead of "loopback-cluster", plus prod-east,
	// prod-west) target the remote bridge instead of only "prod-east" --
	// and skips deploying a local kube-mcp-server entirely
	// (deployKubeMCPServerAndRegister). This is the "fleet" full-pipeline
	// suite's mode: every fleet-routed reconciliation must hit a genuinely
	// separate Kubernetes control plane (no local/loopback fallback that
	// could mask the wiring gaps this topology exists to catch -- see
	// AGENTS.md pyramid invariant). FMC's E2E lanes leave this false,
	// keeping their narrower "prove isolation via one remote registration"
	// scope (DD-TEST-013) unaffected.
	AllRegistrationsRemote bool
}

// RemoteClusterBridgeConfig describes the bridge Service that makes a
// second, independent Kind cluster's kube-mcp-server reachable from the
// primary cluster's MCP Gateway, backing the "prod-east" registration with
// a genuinely separate Kubernetes control plane (DD-TEST-013). Built by
// SetupRemoteClusterForFMC.
type RemoteClusterBridgeConfig struct {
	// BridgeServiceName is the Service name to create in the PRIMARY
	// cluster (e.g. "kube-mcp-server-remote"), used as the "prod-east"
	// backend hostname in place of the local kube-mcp-server Service.
	BridgeServiceName string
	// BridgeServicePort is the port in-cluster Gateway clients dial --
	// must match the remote kube-mcp-server's container port (8080).
	BridgeServicePort int
	// RemoteNodeIP is the remote cluster's control-plane node's IP on the
	// shared podman "kind" bridge network (see KindNodeBridgeIP).
	RemoteNodeIP string
	// RemoteNodePort is the NodePort exposing kube-mcp-server on the
	// remote cluster.
	RemoteNodePort int
}

// FMCOAuth2Config configures how FMC authenticates to the Kuadrant MCP
// Gateway via OAuth2 client_credentials (see fleetmetadatacache-config
// ConfigMap in DeployFleetCoreInfra's Phase 4).
type FMCOAuth2Config struct {
	TokenURL     string
	ClientID     string
	ClientSecret string
	// Scopes, if non-empty, renders an explicit oauth2.scopes YAML list.
	// Leave empty to rely on pkg/fleet/fmc/config's built-in default
	// ["openid", "groups"] (DEX-compatible: DEX's "groups" scope carries the
	// mcp-read/mcp-write role claims Kuadrant's AuthPolicy checks). Keycloak's
	// kubernaut-fleet-read client has no "openid"/"groups" scope assigned, so
	// requesting them fails with "invalid_scope" -- the FMC E2E lane must
	// instead request ["kube-mcp-server-audience"], the scope that carries
	// the audience-mapper gating the RFC 8693 exchange (Spike S17/S18).
	Scopes []string
}

// tomlString renders a TOML config for kube-mcp-server per the configured
// auth mode. See KubeMCPServerAuthConfig for field semantics.
func (c KubeMCPServerAuthConfig) tomlString() string {
	if c.Mode != KubeMCPServerAuthModePassthrough {
		// Issue #54 RCA (see historical comment at the kube-mcp-server-config
		// ConfigMap call site): cluster_auth_mode=kubeconfig makes
		// kube-mcp-server always use its own ServiceAccount and ignore any
		// caller-forwarded Authorization header, matching ADR-068 Decision #9.
		return `cluster_auth_mode = "kubeconfig"`
	}

	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "require_oauth = %t\n", c.RequireOAuth)
	_, _ = fmt.Fprintf(&b, "authorization_url = %q\n", c.AuthorizationURL)
	_, _ = fmt.Fprintf(&b, "oauth_audience = %q\n", c.OAuthAudience)
	_, _ = fmt.Fprintf(&b, "cluster_auth_mode = %q\n", KubeMCPServerAuthModePassthrough)
	_, _ = fmt.Fprintf(&b, "sts_client_id = %q\n", c.StsClientID)
	_, _ = fmt.Fprintf(&b, "sts_client_secret = %q\n", c.StsClientSecret)
	_, _ = fmt.Fprintf(&b, "sts_audience = %q\n", c.StsAudience)
	if len(c.StsScopes) > 0 {
		quoted := make([]string, len(c.StsScopes))
		for i, s := range c.StsScopes {
			quoted[i] = fmt.Sprintf("%q", s)
		}
		_, _ = fmt.Fprintf(&b, "sts_scopes = [%s]\n", strings.Join(quoted, ", "))
	}
	_, _ = fmt.Fprintf(&b, "certificate_authority = %q", c.CAFilePath)
	return b.String()
}

// SetupFleetE2EInfrastructure deploys the complete fleet E2E stack:
// all fullpipeline services + Kuadrant MCP Gateway + FMC + Valkey.
//
// It composes on the fullpipeline setup (which already deploys GW, SP, RO, WE,
// AA, EM, KA, AF, DS, DEX, Prometheus, AlertManager, etc.) and adds the fleet-
// specific infrastructure on top. The Kind cluster config must include the fleet
// NodePort mapping (31976 for EAIGW MCP) -- already present in
// kind-fullpipeline-config.yaml.
//
// Unlike the FMC E2E lanes' loopback pattern, this suite backs EVERY
// registration (including the one named "remote-cluster") with a genuinely
// separate second Kind cluster (AllRegistrationsRemote, DD-TEST-013) so no
// fleet-routed reconciliation can silently fall back to the primary cluster.
//
// Total additional memory over fullpipeline: ~388 MB
// (Istio ~250 MB + Kuadrant ~60 MB + kube-mcp-server ~16 MB + Valkey ~30 MB + FMC ~32 MB).
//
// Authority: Issue #54, ADR-068
// keycloakHostPortFleet is the Kind extraPortMappings host port for Keycloak
// in the "fleet" suite's Kind config, mirroring keycloakHostPortFMC.
const keycloakHostPortFleet = 30557

// SetupFleetE2EInfrastructure returns remoteKubeconfigPath, the second Kind
// cluster's kubeconfig (DD-TEST-013) backing remote-cluster/prod-east/
// prod-west (AllRegistrationsRemote) -- callers (suite_test.go) must
// populate a remote K8s client from it and tear that cluster down alongside
// the primary one.
//
// DD-TEST-015 ("deploy correctly the first time"): fleet infrastructure
// (Keycloak, the Kuadrant MCP Gateway, kube-mcp-server, the remote Kind
// cluster) is provisioned by a FleetProvisioner callback that
// SetupFullPipelineInfrastructure invokes AFTER the target namespace exists
// but BEFORE `helm install` renders global.fleet.enabled=true -- so every
// fleet-aware service (GW, RO, SP, AF, EM, WE, FMC) boots for the first
// time against a live, authenticated MCP Gateway, and the chart's OWN fleet
// templates (RBAC, ConfigMap fleet blocks, oauth2-credentials/
// inter-service-ca volume mounts) are the ones actually exercised end to
// end. This replaces the old design, which deployed every service with
// fleet disabled, then kubectl-patched fleet config into already-running
// Deployments (forcing 1-3 extra restarts per service and never exercising
// the chart's own fleet-enabled render path at all).
func SetupFleetE2EInfrastructure(ctx context.Context, clusterName, kubeconfigPath string, writer io.Writer) (builtImages map[string]string, seededUUIDs map[string]string, afRemediateNS map[string]string, remoteKubeconfigPath string, err error) {
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "🚀 Fleet E2E Infrastructure (Issue #54)")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "  Base: Full Pipeline (all services), fleet-enabled from the FIRST helm install (DD-TEST-015)")
	_, _ = fmt.Fprintln(writer, "  Fleet: Envoy AI Gateway (EAIGW) + chart-managed FMC + Valkey, ALL registrations remote (DD-TEST-013)")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	cleanStaleTarFiles(writer)

	// provisioner deploys Keycloak (IdP), the Kuadrant MCP Gateway +
	// kube-mcp-server, and a genuinely separate remote Kind cluster
	// (DD-TEST-013) -- SetupFullPipelineInfrastructure invokes this once the
	// namespace exists but before its own `helm install`. See
	// FleetProvisioner's doc comment for why this must be a callback.
	//
	// The actual provisioning logic lives in provisionFleetCoreInfra, shared
	// with SetupFleetCoreInfrastructure (infra-only, no helm install --
	// `make setup-e2e-fleet-infra`) so the two entry points never duplicate
	// this ~200-line sequence. This adapter closure only exists to satisfy
	// FleetProvisioner's signature (no room for a remoteKubeconfigPath
	// return) by writing it into the outer named return instead.
	provisioner := func(ctx context.Context, kubeconfigPath, namespace string, writer io.Writer) (*FleetHelmOptions, error) {
		// keycloakNamespace=namespace: unchanged behavior for this Ginkgo-
		// suite path (see provisionFleetCoreInfra's doc comment).
		// gatewayType=GatewayEAIGW: kubernaut#2309 found Kuadrant's static
		// broker credential (used for its self-initiated tool-discovery
		// connection to kube-mcp-server) to be a structural SPOF -- no
		// hot-reload, manual rotation only -- so this suite (the primary
		// fleet journey coverage) moved to EAIGW, which forwards the
		// caller's own token instead of relying on a cached credential.
		// Kuadrant itself stays covered by its own standalone FMC E2E lane
		// (test/e2e/fleetmetadatacache, non-eaigw variant).
		fleetOpts, rkp, err := provisionFleetCoreInfra(ctx, clusterName, kubeconfigPath, namespace, namespace, registry.GatewayEAIGW, writer)
		remoteKubeconfigPath = rkp
		return fleetOpts, err
	}

	builtImages, seededUUIDs, afRemediateNS, err = SetupFullPipelineInfrastructure(ctx, clusterName, kubeconfigPath, provisioner, writer)
	if err != nil {
		return builtImages, seededUUIDs, afRemediateNS, remoteKubeconfigPath, fmt.Errorf("fullpipeline base setup (fleet-enabled) failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "✅ Fleet E2E Infrastructure READY")
	_, _ = fmt.Fprintln(writer, "  Remote tool prefix: remote-cluster__ (EAIGW convention)")
	_, _ = fmt.Fprintf(writer, "  Remote kubeconfig:   %s\n", remoteKubeconfigPath)
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	return builtImages, seededUUIDs, afRemediateNS, remoteKubeconfigPath, nil
}

// SetupFleetCoreInfrastructure creates the hub+spoke Kind cluster pair and
// deploys ONLY the fleet-core infrastructure (Keycloak IdP, Kuadrant MCP
// Gateway, kube-mcp-server, the genuinely separate remote/spoke cluster,
// Traefik so Console has an Ingress controller to attach to, plus
// Prometheus+AlertManager so AF's monitoring-backed tools have something to
// query) -- it does NOT build any Kubernaut service image and does NOT
// `helm install charts/kubernaut`. This is the split-infra entry point for
// `make setup-e2e-fleet-infra`: QE/local environments that want a live
// fleet-core stack and then run `helm install` themselves, once, with their
// own LLM credentials and Rego policy files (which legitimately vary per
// setup and don't belong baked into shared test-infra Go code).
//
// It shares its provisioning logic with SetupFleetE2EInfrastructure via
// provisionFleetCoreInfra -- see that function's doc comment.
//
// The Kind cluster is left running (no teardown): tear it down manually with
// `kind delete cluster --name <clusterName>` (and the "-remote" sibling)
// when done.
// SetupFleetCoreInfrastructure is the backward-compatible entry point for
// callers that don't care which MCP Gateway implementation is used --
// defaults to registry.GatewayKuadrant (this function's long-standing
// behavior before gateway-type selection existed). Prefer
// SetupFleetCoreInfrastructureWithGateway for new callers.
func SetupFleetCoreInfrastructure(ctx context.Context, clusterName, kubeconfigPath string, writer io.Writer) (fleetOpts *FleetHelmOptions, remoteKubeconfigPath string, err error) {
	return SetupFleetCoreInfrastructureWithGateway(ctx, clusterName, kubeconfigPath, registry.GatewayKuadrant, writer)
}

// SetupFleetCoreInfrastructureWithGateway is SetupFleetCoreInfrastructure
// with an explicit gatewayType (registry.GatewayKuadrant or
// registry.GatewayEAIGW; empty defaults to Kuadrant). See
// provisionFleetCoreInfra's doc comment for what differs between the two.
func SetupFleetCoreInfrastructureWithGateway(ctx context.Context, clusterName, kubeconfigPath string, gatewayType registry.MCPGatewayType, writer io.Writer) (fleetOpts *FleetHelmOptions, remoteKubeconfigPath string, err error) {
	if gatewayType == "" {
		gatewayType = registry.GatewayKuadrant
	}
	gatewayLabel := gatewayLabelKuadrant
	if gatewayType == registry.GatewayEAIGW {
		gatewayLabel = gatewayLabelEAIGW
	}
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "🚀 Fleet-core infrastructure only (no Kubernaut helm install)")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintf(writer, "  Hub + spoke Kind clusters, Keycloak, %s,\n", gatewayLabel)
	_, _ = fmt.Fprintln(writer, "  kube-mcp-server, Traefik (Console Ingress). Run `helm install")
	_, _ = fmt.Fprintln(writer, "  charts/kubernaut` yourself afterward with your own LLM")
	_, _ = fmt.Fprintln(writer, "  credentials + Rego policies.")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	namespace := kubernautSystem

	_, _ = fmt.Fprintln(writer, "\n🏗️  Creating hub Kind cluster...")
	kindConfigPath := "test/infrastructure/kind-fullpipeline-config.yaml"
	if err := CreateKindClusterWithExtraMounts(ctx, clusterName, kubeconfigPath, kindConfigPath, nil, writer); err != nil {
		return nil, "", fmt.Errorf("hub Kind cluster creation failed: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "\n📁 Creating %s namespace + Helm prerequisite Secrets...\n", namespace)
	if err := CreateTestNamespace(ctx, namespace, kubeconfigPath, writer); err != nil {
		return nil, "", fmt.Errorf("failed to create namespace: %w", err)
	}
	// postgresql-secret/valkey-secret are fixed E2E credentials the chart
	// requires regardless of setup. llm-credentials-primary is also created
	// here (mock-llm-e2e-key placeholder) so `helm install` doesn't fail on
	// a missing Secret -- overwrite it with real LLM credentials before
	// installing if you're not using Mock LLM (see the summary printed below).
	if err := createFullPipelineHelmSecrets(ctx, namespace, kubeconfigPath, writer); err != nil {
		return nil, "", fmt.Errorf("failed to create Helm chart prerequisite secrets: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "\n🌐 Provisioning fleet-core infrastructure (IdP + MCP Gateway + spoke cluster)...")
	// keycloakNamespace=idpNamespace ("idp"), NOT namespace: this demo-only
	// entry point deploys the IdP in its own namespace, matching production
	// (Keycloak wouldn't share a namespace with the app it authenticates
	// for). See idpNamespace's doc comment (keycloak_e2e.go) for why this
	// is scoped here only, not the shared Ginkgo-suite provisioner closure.
	fleetOpts, remoteKubeconfigPath, err = provisionFleetCoreInfra(ctx, clusterName, kubeconfigPath, namespace, idpNamespace, gatewayType, writer)
	if err != nil {
		return nil, remoteKubeconfigPath, fmt.Errorf("fleet-core infrastructure provisioning failed: %w", err)
	}

	// Console-only addition, not shared with provisionFleetCoreInfra (the
	// "fleet"/"fullpipeline" Ginkgo suites never open a browser against
	// Console, so they don't need an Ingress controller).
	_, _ = fmt.Fprintln(writer, "\n🌐 Installing Traefik (Console Ingress controller)...")
	if err := deployTraefikForKind(ctx, kubeconfigPath, writer); err != nil {
		return nil, remoteKubeconfigPath, fmt.Errorf("traefik install failed: %w", err)
	}

	// Fleet-wide monitoring (Prometheus+Thanos-sidecar per cluster + hub
	// Thanos Querier + AlertManager): deliberately NOT added to
	// provisionFleetCoreInfra (shared with the "fleet"/"fullpipeline"
	// Ginkgo suites, which run a single shared Prometheus/AlertManager with
	// no Thanos federation by design -- TESTING_GUIDELINES.md -- and must
	// not change here). This entry point skips SetupFullPipelineInfrastructure
	// entirely (no Kubernaut images, no `helm install`), so without this it
	// would otherwise leave AF's monitoring-backed tools (get_alerts/
	// get_silences/severity triage) permanently "not configured" for anyone
	// using this to demo fleet remediation locally (confirmed via manual
	// fleet E2E QE run, 2026-08-29).
	//
	// Thanos, not plain Prometheus federation, matches this repo's OWN
	// documented production architecture for fleet monitoring aggregation
	// (ADR-068, DD-EM-005 v1.3, DD-INT-020 Part E -- see thanos_e2e.go's
	// file-top comment) -- confirmed 2026-08-30 before building this, since
	// the two are NOT interchangeable for a "matches production" demo goal.
	//
	// Dedicated "monitoring" namespace on BOTH clusters -- NOT namespace
	// (kubernaut-system, the Kubernaut app namespace) and NOT
	// remoteMCPServerNamespace (mcp-system, Kuadrant/MCP's namespace).
	// Prometheus/Thanos/AlertManager are platform monitoring infrastructure
	// that predates and outlives any single application (mirrors real
	// deployments: OCP's "openshift-monitoring", a typical kube-prometheus-
	// stack's "monitoring") -- same reasoning already applied to
	// kube-mcp-server's own "mcp-system" move, just not conflating the two
	// platform concerns into one shared namespace. Confirmed via `helm
	// template`/chart source read, 2026-08-30: AF's and EM's NetworkPolicy
	// egress rules to Prometheus/AlertManager (kubernaut.np.prometheusEgress,
	// effectivenessmonitor/networkpolicy.yaml) are already CIDR/port-only
	// with no namespace restriction, so this move needs no chart changes.
	if err := CreateTestNamespace(ctx, monitoringNamespace, kubeconfigPath, writer); err != nil {
		return nil, remoteKubeconfigPath, fmt.Errorf("failed to create hub monitoring namespace: %w", err)
	}
	if err := CreateTestNamespace(ctx, monitoringNamespace, remoteKubeconfigPath, writer); err != nil {
		return nil, remoteKubeconfigPath, fmt.Errorf("failed to create spoke monitoring namespace: %w", err)
	}

	// AlertManager: single instance in the hub only (matches DD-EM-005's
	// "AlertManager... aggregates alerts fleet-wide" -- one sink, not one
	// per cluster). Both clusters' Prometheus instances point their
	// `alerting:` sink at it: the hub's via in-cluster Service DNS, the
	// spoke's via the hub's node-bridge IP:NodePort (no in-cluster route
	// from spoke to a hub Service). gatewayToken="" -- no AlertManager->
	// Gateway webhook forwarding wired up front (same as kubernautagent.go's
	// Phase 5.7 and effectivenessmonitor_e2e.go, both of which also pass "");
	// AlertManager's default "gateway-webhook" receiver still resolves
	// correctly (gateway-service is in kubernaut-system, unaffected by
	// which namespace AlertManager itself lives in).
	_, _ = fmt.Fprintln(writer, "\n📊 Installing fleet-wide monitoring (Prometheus+Thanos per cluster, hub AlertManager)...")
	if err := DeployAlertManager(ctx, monitoringNamespace, kubeconfigPath, "", writer); err != nil {
		return nil, remoteKubeconfigPath, fmt.Errorf("alertmanager install failed: %w", err)
	}

	// kube-state-metrics on BOTH clusters, before each cluster's Prometheus
	// (whose generated scrape config already targets it by Service name --
	// see DeployKubeStateMetrics's doc comment). Without this, AF's
	// monitoring-backed tools that depend on pod/deployment/node object-
	// state metrics (as opposed to cAdvisor's container-resource metrics)
	// come back empty -- confirmed via manual QE run on fleet-e2e-remote's
	// spoke, 2026-08-30, which is why this now covers the hub too.
	if err := DeployKubeStateMetrics(ctx, monitoringNamespace, kubeconfigPath, writer); err != nil {
		return nil, remoteKubeconfigPath, fmt.Errorf("hub kube-state-metrics install failed: %w", err)
	}
	if err := DeployKubeStateMetrics(ctx, monitoringNamespace, remoteKubeconfigPath, writer); err != nil {
		return nil, remoteKubeconfigPath, fmt.Errorf("spoke kube-state-metrics install failed: %w", err)
	}

	// Demo alerting rules (with a STATIC cluster label baked into each rule,
	// not left to Thanos's external_labels -- see DeployDemoAlertingRules's
	// doc comment for why) must exist before Prometheus starts, since its
	// Deployment mounts the ConfigMap they create.
	if err := DeployDemoAlertingRules(ctx, monitoringNamespace, kubeconfigPath, "hub", writer); err != nil {
		return nil, remoteKubeconfigPath, fmt.Errorf("hub demo alerting rules install failed: %w", err)
	}
	// "remote-cluster", not "spoke": matches the MCPServerRegistration/MCPRoute
	// identity resource tools already use for this same physical cluster
	// (test/e2e/fleet's canonical fixture, confirmed 2026-08-30) -- using a
	// different string here for the SAME cluster would leave AF's alert
	// cluster_id and its resource-tool cluster_id permanently unable to
	// cross-reference each other for one physical cluster.
	if err := DeployDemoAlertingRules(ctx, monitoringNamespace, remoteKubeconfigPath, "remote-cluster", writer); err != nil {
		return nil, remoteKubeconfigPath, fmt.Errorf("spoke demo alerting rules install failed: %w", err)
	}

	hubAlertManagerTarget := fmt.Sprintf("alertmanager-svc.%s.svc.cluster.local:9093", monitoringNamespace)
	if err := DeployPrometheusWithThanosSidecar(ctx, monitoringNamespace, kubeconfigPath, "hub", hubAlertManagerTarget, writer); err != nil {
		return nil, remoteKubeconfigPath, fmt.Errorf("hub prometheus+thanos-sidecar install failed: %w", err)
	}

	remoteClusterName := clusterName + "-remote"
	spokeAlertManagerTarget, err := HubNodeBridgeIPAndPort(ctx, clusterName, AlertManagerNodePort)
	if err != nil {
		return nil, remoteKubeconfigPath, fmt.Errorf("failed to resolve hub AlertManager bridge address for spoke: %w", err)
	}
	// "remote-cluster", not "spoke" -- see DeployDemoAlertingRules call above.
	if err := DeployPrometheusWithThanosSidecar(ctx, monitoringNamespace, remoteKubeconfigPath, "remote-cluster", spokeAlertManagerTarget, writer); err != nil {
		return nil, remoteKubeconfigPath, fmt.Errorf("spoke prometheus+thanos-sidecar install failed: %w", err)
	}

	spokeSidecarStoreAddr, err := BridgeSpokeThanosSidecar(ctx, kubeconfigPath, monitoringNamespace, remoteKubeconfigPath, monitoringNamespace, remoteClusterName, writer)
	if err != nil {
		return nil, remoteKubeconfigPath, fmt.Errorf("spoke thanos sidecar bridge failed: %w", err)
	}
	hubSidecarStoreAddr := fmt.Sprintf("thanos-sidecar-grpc.%s.svc.cluster.local:%d", monitoringNamespace, ThanosSidecarGRPCPort)
	if err := DeployThanosQuerier(ctx, monitoringNamespace, kubeconfigPath, []string{hubSidecarStoreAddr, spokeSidecarStoreAddr}, writer); err != nil {
		return nil, remoteKubeconfigPath, fmt.Errorf("thanos querier install failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "✅ Fleet-core infrastructure READY -- Kubernaut is NOT installed")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintf(writer, "  Kubeconfig (hub):     %s\n", kubeconfigPath)
	_, _ = fmt.Fprintf(writer, "  Kubeconfig (remote):  %s\n", remoteKubeconfigPath)
	_, _ = fmt.Fprintf(writer, "  Namespace:            %s\n", namespace)
	_, _ = fmt.Fprintln(writer, "\n  Values needed for your `helm install charts/kubernaut ...`:")
	_, _ = fmt.Fprintf(writer, "    global.fleet.enabled=true\n")
	_, _ = fmt.Fprintf(writer, "    global.fleet.mcpGatewayEndpoint=%s\n", fleetOpts.MCPGatewayEndpoint)
	_, _ = fmt.Fprintf(writer, "    global.fleet.mcpGatewayType=%s\n", fleetOpts.MCPGatewayType)
	_, _ = fmt.Fprintf(writer, "    global.fleet.oauth2.enabled=true\n")
	_, _ = fmt.Fprintf(writer, "    global.fleet.oauth2.tokenURL=%s\n", fleetOpts.OAuth2TokenURL)
	_, _ = fmt.Fprintf(writer, "    global.fleet.oauth2.credentialsSecretRef=%s\n", fleetOpts.OAuth2CredentialsSecret)
	_, _ = fmt.Fprintf(writer, "    global.fleet.oauth2.scopes=%v\n", fleetOpts.OAuth2Scopes)
	_, _ = fmt.Fprintf(writer, "    workflowexecution.fleet.oauth2.credentialsSecretRef=%s\n", fleetOpts.WEOAuth2CredentialsSecret)
	_, _ = fmt.Fprintf(writer, "    signalprocessing.fleet.namespace=%s\n", fleetOpts.SignalProcessingNamespace)
	_, _ = fmt.Fprintf(writer, "    fleetmetadatacache.namespace=%s\n", fleetOpts.FleetMetadataCacheNamespace)
	_, _ = fmt.Fprintln(writer, "\n  For AF's monitoring-backed tools (get_alerts, get_silences, severity triage),")
	_, _ = fmt.Fprintln(writer, "  fleet-wide via Thanos Querier (hub+spoke, ADR-068/DD-EM-005/DD-INT-020),")
	_, _ = fmt.Fprintf(writer, "  deployed in its own %q namespace (platform infra, not kubernaut-system):\n", monitoringNamespace)
	_, _ = fmt.Fprintln(writer, "    monitoring.prometheus.enabled=true")
	_, _ = fmt.Fprintf(writer, "    monitoring.prometheus.url=http://thanos-querier-svc.%s.svc.cluster.local:9090\n", monitoringNamespace)
	_, _ = fmt.Fprintln(writer, "    monitoring.alertManager.enabled=true")
	_, _ = fmt.Fprintf(writer, "    monitoring.alertManager.url=http://alertmanager-svc.%s.svc.cluster.local:9093\n", monitoringNamespace)
	_, _ = fmt.Fprintln(writer, "\n  ⚠️  Adding a NEW alerting rule for this demo? Bake `cluster: hub`/`cluster: remote-cluster`")
	_, _ = fmt.Fprintln(writer, "      (matching the MCPServerRegistration identity, NOT \"spoke\") into the rule's")
	_, _ = fmt.Fprintln(writer, "      own static `labels:` block -- Thanos does NOT propagate")
	_, _ = fmt.Fprintln(writer, "      external_labels to fired alert instances (thanos-io/thanos#7327), so AF's")
	_, _ = fmt.Fprintln(writer, "      cluster_id filter will silently drop any alert missing it. See")
	_, _ = fmt.Fprintln(writer, "      DeployDemoAlertingRules's doc comment (thanos_e2e.go) for details.")
	_, _ = fmt.Fprintln(writer, "\n  ⚠️  llm-credentials-primary currently holds a MOCK key --")
	_, _ = fmt.Fprintln(writer, "      kubectl apply your real LLM credentials Secret before")
	_, _ = fmt.Fprintln(writer, "      `helm install` if you're not using Mock LLM.")
	_, _ = fmt.Fprintf(writer, "\n  Keycloak (IdP) lives in its own %q namespace, not %s (production parity).\n", idpNamespace, namespace)
	_, _ = fmt.Fprintln(writer, "  For AF/Console browser login (reuses the same Keycloak, no DEX):")
	_, _ = fmt.Fprintln(writer, "    apifrontend.config.auth.issuerURL=https://keycloak:8443/realms/kubernaut-fleet")
	_, _ = fmt.Fprintf(writer, "    apifrontend.config.auth.jwksURL=https://keycloak.%s.svc.cluster.local:8443/realms/kubernaut-fleet/protocol/openid-connect/certs\n", idpNamespace)
	_, _ = fmt.Fprintln(writer, "    console.enabled=true, console.ingress.className=traefik,")
	_, _ = fmt.Fprintln(writer, "    console.ingress.host=kubernaut-console.local, console.ingress.port=8843")
	_, _ = fmt.Fprintln(writer, "    Add to /etc/hosts:  127.0.0.1 keycloak kubernaut-console.local")
	_, _ = fmt.Fprintln(writer, "    Browse to: https://kubernaut-console.local:8843")
	_, _ = fmt.Fprintln(writer, "    Login: sre-user / password (Keycloak \"sre\" group)")
	_, _ = fmt.Fprintln(writer, "    console.oauth2Proxy: Keycloak is in a different namespace than Console's")
	_, _ = fmt.Fprintln(writer, "    oauth2-proxy, so it needs split browser/in-cluster OIDC endpoints:")
	_, _ = fmt.Fprintln(writer, "      console.oauth2Proxy.skipDiscovery=true")
	_, _ = fmt.Fprintln(writer, "      console.oauth2Proxy.loginURL=https://keycloak:8443/realms/kubernaut-fleet/protocol/openid-connect/auth")
	_, _ = fmt.Fprintf(writer, "      console.oauth2Proxy.redeemURL=https://keycloak.%s.svc.cluster.local:8443/realms/kubernaut-fleet/protocol/openid-connect/token\n", idpNamespace)
	_, _ = fmt.Fprintf(writer, "      console.oauth2Proxy.jwksURL=https://keycloak.%s.svc.cluster.local:8443/realms/kubernaut-fleet/protocol/openid-connect/certs\n", idpNamespace)
	_, _ = fmt.Fprintln(writer, "    AFTER `helm install`: make bind-fleet-af-rbac KUBECONFIG="+kubeconfigPath)
	_, _ = fmt.Fprintln(writer, "    See ~/.kubernaut/helm/fleet-e2e-values.yaml for the full worked example.")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	return fleetOpts, remoteKubeconfigPath, nil
}

// provisionFleetCoreInfra deploys Keycloak (IdP), the genuinely separate
// remote Kind cluster (DD-TEST-013), and the Kuadrant MCP Gateway +
// kube-mcp-server -- the full fleet-core infrastructure sequence shared by:
//
//   - SetupFleetE2EInfrastructure: invokes this via a FleetProvisioner
//     adapter closure, then proceeds to `helm install` with the returned
//     FleetHelmOptions (DD-TEST-015).
//   - SetupFleetCoreInfrastructure: invokes this directly and stops --
//     no kubernaut images, no `helm install` -- for local/QE environments
//     that need a live fleet-core stack before a manual, per-setup
//     `helm install` with real LLM credentials and Rego policies
//     (`make setup-e2e-fleet-infra`).
//
// namespace must already exist (both callers create it via
// CreateTestNamespace before invoking this). gatewayType selects
// registry.GatewayKuadrant or registry.GatewayEAIGW (empty defaults to
// Kuadrant, matching DeployFleetGatewayInfra's own zero-value handling) --
// see KubeMCPServerAuthConfig.GatewayType's doc comment for what differs
// between the two at the edge routing/OAuth validation layer. Returns the
// FleetHelmOptions (MCP Gateway endpoint/type, OAuth2 token URL/secret/
// scopes) needed to render a fleet-enabled `helm install`, and the remote
// cluster's kubeconfig path.
func provisionFleetCoreInfra(ctx context.Context, clusterName, kubeconfigPath, namespace, keycloakNamespace string, gatewayType registry.MCPGatewayType, writer io.Writer) (*FleetHelmOptions, string, error) {
	var remoteKubeconfigPath string

	// Backward-compatible default (all existing callers pre-dating this
	// parameter expect Kuadrant), mirroring DeployFleetGatewayInfra's own
	// zero-value handling.
	if gatewayType == "" {
		gatewayType = registry.GatewayKuadrant
	}
	gatewayLabel := gatewayLabelKuadrant
	// DD-TEST-001-allocated NodePorts: 31975 (Kuadrant) / 31976 (EAIGW).
	mcpGatewayNodePort := 31975
	// provisionFleetCoreInfra always sets AllRegistrationsRemote=true below
	// (both its callers -- the "fleet" Ginkgo suite and the demo entry point
	// -- back every registration with a genuinely remote cluster), so
	// EAIGW's first Backend/MCPRoute registration is always renamed
	// "remote-cluster" (mirroring Kuadrant's MCPServerRegistration renaming
	// -- see loopbackClusterName in deployEnvoyAIGatewayRegistrations and
	// deployKuadrantRegistrations), and its auto-derived tool-name prefix is
	// "remote-cluster__" accordingly.
	remoteToolPrefix := "remote_cluster_"
	if gatewayType == registry.GatewayEAIGW {
		gatewayLabel = gatewayLabelEAIGW
		mcpGatewayNodePort = eaigwGatewayNodePort
		remoteToolPrefix = "remote-cluster__"
	}

	// ── Keycloak OIDC + RFC 8693 token-exchange provider (replaces Dex) ──
	// Dex has no Standard Token Exchange (Spike S20); Keycloak is the same
	// proven IdP the FMC E2E lane already uses in CI for passthrough+STS.
	//
	// keycloakNamespace is namespace (kubernautSystem) for the "fleet"/
	// "fullpipeline" Ginkgo suites' provisioner closure -- unchanged
	// behavior -- and idpNamespace ("idp") for the demo-only entry point
	// (SetupFleetCoreInfrastructure). See idpNamespace's doc comment
	// (keycloak_e2e.go) for why this is scoped to the demo path only.
	//
	// provisionInterServiceCA (DD-TEST-015) must run first: it creates
	// authwebhook-tls (with ca.crt/ca.key) and the inter-service-ca
	// ConfigMap BEFORE `helm install`'s own pre-install hook would
	// otherwise generate them, so Keycloak's leaf cert below is signed
	// from the SAME CA the chart's hook will detect as already-valid and
	// reuse for gateway-tls/datastorage-tls/kubernautagent-tls/
	// fleetmetadatacache-tls/apifrontend-tls, and kube-mcp-server's
	// required tls-ca volume (mounted further down) has something to
	// mount.
	if err := provisionInterServiceCA(ctx, kubeconfigPath, namespace, writer); err != nil {
		return nil, "", fmt.Errorf("inter-service CA provisioning failed: %w", err)
	}
	if keycloakNamespace != namespace {
		if err := CreateTestNamespace(ctx, keycloakNamespace, kubeconfigPath, writer); err != nil {
			return nil, "", fmt.Errorf("failed to create %s namespace for Keycloak: %w", keycloakNamespace, err)
		}
	}
	if kcTLSErr := ensureKeycloakTLSFromChartCA(ctx, kubeconfigPath, namespace, keycloakNamespace, writer); kcTLSErr != nil {
		return nil, "", fmt.Errorf("keycloak-tls provisioning failed: %w", kcTLSErr)
	}
	_, _ = fmt.Fprintln(writer, "\n🔑 Deploying Keycloak OIDC provider (replaces Dex -- RFC 8693 token exchange, Spike S17/S20)...")
	if kcErr := DeployKeycloakInfra(ctx, keycloakNamespace, kubeconfigPath, keycloakHostPortFleet, writer); kcErr != nil {
		return nil, "", fmt.Errorf("failed to deploy Keycloak: %w", kcErr)
	}

	oidcCfg := OIDCPatchConfig{
		IssuerURL:      "https://keycloak:8443/realms/kubernaut-fleet",
		ClientID:       "k8s-api",
		UsernameClaim:  "preferred_username",
		UsernamePrefix: "keycloak:",
	}
	if oidcErr := patchAPIServerForOIDCConfig(ctx, clusterName, kubeconfigPath, oidcCfg, keycloakNamespace, writer); oidcErr != nil {
		return nil, "", fmt.Errorf("API server OIDC patching failed: %w", oidcErr)
	}

	// ── Remote cluster (DD-TEST-013, Spike S19) ──────────────────────────
	// Backs EVERY registration (AllRegistrationsRemote) with a genuinely
	// separate Kubernetes control plane -- unlike the FMC E2E lane, which
	// only bridges "prod-east" for isolation testing, this suite's whole
	// point is that "remote-cluster" (the identity nearly every fleet
	// test targets) is a genuinely separate physical cluster, not the
	// primary one.
	_, _ = fmt.Fprintln(writer, "\n🌍 Provisioning remote cluster (ALL registrations remote, DD-TEST-013)...")
	remoteClusterName := clusterName + "-remote"
	remoteKubeconfigPath = filepath.Join(filepath.Dir(kubeconfigPath), remoteClusterName+"-config")
	sharedAuthConfig := KubeMCPServerAuthConfig{
		Mode:             KubeMCPServerAuthModePassthrough,
		GatewayType:      gatewayType,
		RequireOAuth:     true,
		AuthorizationURL: oidcCfg.IssuerURL,
		OAuthAudience:    "kube-mcp-server",
		StsClientID:      "kube-mcp-server",
		StsClientSecret:  "e2e-kube-mcp-server-secret",
		StsAudience:      "k8s-api",
		StsScopes:        []string{"k8s-api-audience"},
		CAFilePath:       "/etc/tls-ca/ca.crt",
	}
	remoteBridge, remoteErr := SetupRemoteClusterForFMC(ctx, clusterName, kubeconfigPath, remoteClusterName, remoteKubeconfigPath, oidcCfg.IssuerURL, keycloakHostPortFleet, sharedAuthConfig, writer)
	if remoteErr != nil {
		return nil, "", fmt.Errorf("remote cluster provisioning failed: %w", remoteErr)
	}

	// Issue #1542: job-backend workflows (e.g. crashloop-config-fix-v1) run
	// their Job on the REMOTE cluster when RemediationRequest.ClusterID is
	// set, via WE's mcpClientFactory routing. The "kubernaut-workflows"
	// namespace only pre-existed on the hub cluster; without it, both the
	// SA creation below and the Job itself would fail with "namespace not found".
	_, _ = fmt.Fprintf(writer, "\n📁 Creating %s namespace on the remote cluster (Issue #1542)...\n", ExecutionNamespace)
	if err := CreateTestNamespace(ctx, ExecutionNamespace, remoteKubeconfigPath, writer); err != nil {
		return nil, "", fmt.Errorf("failed to create %s namespace on remote cluster: %w", ExecutionNamespace, err)
	}

	// Without this, the Job pod's serviceAccountName: workflow-job-executor
	// reference would fail to resolve on the remote cluster (SA only
	// pre-existed on the hub).
	_, _ = fmt.Fprintln(writer, "🔐 Creating workflow-job-executor SA + RBAC on the remote cluster (Issue #1542)...")
	if err := createWorkflowJobExecutorRBAC(ctx, remoteKubeconfigPath, ExecutionNamespace, writer); err != nil {
		return nil, "", fmt.Errorf("failed to create workflow-job-executor RBAC on remote cluster: %w", err)
	}

	// Issue #1542: the WE Job executor dispatches the Job to the remote
	// cluster's API server via kube-mcp-server passthrough, authenticated as
	// the exchanged Keycloak identity (keycloak:service-account-kubernaut-fleet-read).
	// applyExchangedIdentityRBAC (below, inside SetupRemoteClusterForFMC) only
	// grants read-only "view" access -- the FMC-only lane must stay read-only,
	// so this ADDITIONAL grant is fleet-suite-only and strictly additive
	// (batch/jobs create/delete, nothing else).
	_, _ = fmt.Fprintln(writer, "🔐 Granting batch/jobs write access to the exchanged fleet identity (Issue #1542, fleet-only)...")
	if err := applyExchangedIdentityWriteRBAC(ctx, remoteKubeconfigPath, writer); err != nil {
		return nil, "", fmt.Errorf("failed to grant exchanged identity write RBAC on remote cluster: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintf(writer, "🌐 FLEET PHASE: Deploying %s infrastructure...\n", gatewayLabel)
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	_, _ = fmt.Fprintln(writer, "  📦 Pre-loading fleet external images...")
	// EAIGW's controller/CRDs are installed separately (deployEnvoyAIGatewayInfra
	// applies its own upstream manifests, not preloaded images here) --
	// kube-mcp-server is the only image both gateways need preloaded.
	preloadImages := []string{KubeMCPServerImage}
	if gatewayType != registry.GatewayEAIGW {
		preloadImages = append(preloadImages, kuadrantControllerImage, kuadrantBrokerImage)
	}
	for _, img := range preloadImages {
		if loadErr := PreloadExternalImage(ctx, img, clusterName, writer); loadErr != nil {
			_, _ = fmt.Fprintf(writer, "  ⚠️  Image preload failed (will pull on-demand): %s: %v\n", img, loadErr)
		}
	}

	kubeMCPAuthConfig := sharedAuthConfig
	kubeMCPAuthConfig.RemoteBridge = remoteBridge
	kubeMCPAuthConfig.AllRegistrationsRemote = true

	// FMC's own client_credentials grant (and every other fleet-aware
	// service's, via the FleetHelmOptions returned below) goes to
	// Keycloak instead of Dex.
	const (
		fleetClientID     = "kubernaut-fleet-read"
		fleetClientSecret = "e2e-fleet-secret"
	)
	fleetScopes := []string{"kube-mcp-server-audience"}

	// Kuadrant broker's own upstream discovery connection needs a static
	// credential when RequireOAuth=true (see BrokerCredentialToken doc
	// comment) -- mirrors the FMC E2E lane's Phase 7 broker credential.
	// EAIGW has no equivalent broker hop (deployEnvoyAIGatewayRegistrations
	// forwards the caller's own Authorization header instead), so skip
	// minting a credential that would never be read.
	if gatewayType != registry.GatewayEAIGW {
		brokerCredToken, brokerCredErr := GetKeycloakClientCredentialsToken(ctx, KeycloakFleetTokenConfig{
			TokenEndpoint:  fmt.Sprintf("https://localhost:%d/realms/kubernaut-fleet/protocol/openid-connect/token", keycloakHostPortFleet),
			ClientID:       fleetClientID,
			ClientSecret:   fleetClientSecret,
			Scopes:         fleetScopes,
			KubeconfigPath: kubeconfigPath,
		})
		if brokerCredErr != nil {
			return nil, "", fmt.Errorf("failed to obtain Kuadrant broker's kube-mcp-server discovery credential: %w", brokerCredErr)
		}
		kubeMCPAuthConfig.BrokerCredentialToken = brokerCredToken
	}

	mcpGatewayEndpoint, gwErr := DeployFleetGatewayInfra(ctx, namespace, kubeconfigPath, kubeMCPAuthConfig, writer)
	if gwErr != nil {
		return nil, "", fmt.Errorf("fleet gateway infra deployment failed: %w", gwErr)
	}

	_, _ = fmt.Fprintln(writer, "\n  🔑 Creating RBAC for the exchanged Keycloak identity...")
	if err := applyExchangedIdentityRBAC(ctx, kubeconfigPath, writer); err != nil {
		return nil, "", fmt.Errorf("fleet exchanged-identity RBAC creation failed: %w", err)
	}

	// ── Gateway convergence gate (Issue #1737 regression) ────────────────
	// Must run BEFORE `helm install` (below): deploying the Kuadrant MCP
	// Gateway only proves the *pod* is Ready and an unauthenticated
	// `initialize` succeeds; Kuadrant's AuthPolicy/Envoy ext_authz config
	// converges asynchronously on its own tail, per WaitForFleetReady's
	// doc comment (same race as the Issue #54 FMC RCA: a bare
	// `initialize` can succeed while authenticated calls still fail until
	// Envoy's xDS config catches up). AF and EM both start a
	// registry.ClusterRegistry against this same gateway on process
	// startup (buildFleetReaderDeps -> mcpclient.NewResilient +
	// ClusterRegistry.Start's blocking cache.WaitForCacheSync) -- letting
	// them boot before the gateway has *provably* converged races their
	// startup against Envoy's convergence, with their liveness probe as
	// the tiebreaker (confirmed via live crash-loop debugging: List() on
	// MCPServerRegistration hung/failed for 10+ minutes, self-resolving
	// once Envoy caught up -- not an RBAC or networking gap).
	// DD-TEST-015 moves this check before the chart's `helm install`
	// entirely, instead of merely before a later kubectl-patch step.
	keycloakFleetReadTokenFunc := func() (string, error) {
		return GetKeycloakClientCredentialsToken(ctx, KeycloakFleetTokenConfig{
			TokenEndpoint:  fmt.Sprintf("https://localhost:%d/realms/kubernaut-fleet/protocol/openid-connect/token", keycloakHostPortFleet),
			ClientID:       fleetClientID,
			ClientSecret:   fleetClientSecret,
			Scopes:         fleetScopes,
			KubeconfigPath: kubeconfigPath,
		})
	}
	if readyErr := WaitForFleetReady(ctx, keycloakFleetReadTokenFunc, mcpGatewayNodePort, remoteToolPrefix, writer); readyErr != nil {
		return nil, "", fmt.Errorf("fleet readiness check failed: %w", readyErr)
	}

	// ── Shared OAuth2 credentials Secret for every fleet-aware service ───
	// Must exist BEFORE `helm install` (DD-TEST-015): the chart's own
	// fleet templates mount this Secret as a volume whenever
	// global.fleet.oauth2.enabled=true (see apifrontend.yaml et al.'s
	// oauth2-credentials volume) -- if it doesn't exist yet, the
	// resulting Pod gets stuck in ContainerCreating (missing volume
	// source).
	_, _ = fmt.Fprintln(writer, "\n🔑 Creating shared fleet OAuth2 credentials Secret...")
	if err := deployFleetOAuth2Secret(ctx, namespace, kubeconfigPath, writer); err != nil {
		return nil, "", err
	}

	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "✅ Fleet infrastructure ready")
	_, _ = fmt.Fprintf(writer, "  MCP Gateway:        http://localhost:%d/mcp (%s)\n", mcpGatewayNodePort, gatewayLabel)
	_, _ = fmt.Fprintln(writer, "  Remote cluster ID:  remote-cluster (genuinely remote, DD-TEST-013)")
	_, _ = fmt.Fprintf(writer, "  Remote kubeconfig:  %s\n", remoteKubeconfigPath)
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	return &FleetHelmOptions{
		MCPGatewayEndpoint:        mcpGatewayEndpoint,
		MCPGatewayType:            string(gatewayType),
		OAuth2TokenURL:            keycloakFleetTokenURLFor(keycloakNamespace, namespace),
		OAuth2CredentialsSecret:   fleetOAuth2SecretName,
		OAuth2Scopes:              fleetScopes,
		WEOAuth2CredentialsSecret: fleetOAuth2SecretName,
		SignalProcessingNamespace: namespace,
		// Issue #2298: MCPServerRegistrations are created in `namespace`
		// by deployKuadrantRegistrations above (via DeployFleetGatewayInfra
		// -> deployKubeMCPServerAndRegister), so FMC's own watch must be
		// scoped there too -- there's no safe default to fall back to.
		FleetMetadataCacheNamespace: namespace,
	}, remoteKubeconfigPath, nil
}

// fleetOAuth2SecretName is the shared Keycloak client_credentials Secret
// mounted into every fleet-aware service (AF, EM, GW, RO, SP, WE) once the
// "fleet" suite switches kube-mcp-server to passthrough+STS. Once
// AllRegistrationsRemote collapses every registration onto a single
// kube-mcp-server instance with RequireOAuth=true, that ONE resource-server
// check gates every caller through the gateway -- not just FMC's syncer --
// so every service that reads fleet data via the MCP Gateway now needs a
// valid Bearer token to get past it. All six services share Keycloak's
// "kubernaut-fleet-read" client (the same one FMC uses): this E2E suite
// does not test per-service RBAC differentiation, and
// applyExchangedIdentityRBAC already binds that one exchanged identity to
// "view" for every caller.
const fleetOAuth2SecretName = "fleet-oauth2-creds"

// fleetKeycloakTokenURL is the Keycloak client_credentials token endpoint
// every fleet-aware service's fleet.oauth2.tokenURL points at (in-cluster
// hostname, matching keycloak_e2e.go's Service), for the case where
// Keycloak shares appNamespace (the "fleet"/"fullpipeline" Ginkgo suites,
// unchanged). See keycloakFleetTokenURLFor for the idpNamespace case.
const fleetKeycloakTokenURL = "https://keycloak:8443/realms/kubernaut-fleet/protocol/openid-connect/token"

// keycloakFleetTokenURLFor returns the token endpoint every fleet-aware
// service dials directly (a real client_credentials grant call, not a
// string-matched issuer) to authenticate against the MCP Gateway. Those
// services all run in appNamespace, so when Keycloak lives elsewhere
// (keycloakNamespace != appNamespace, the demo-only idpNamespace case) the
// bare hostname doesn't resolve across namespaces and an FQDN is required;
// when they're equal (the shared Ginkgo-suite provisioner closure), the
// bare hostname is unchanged from before this function existed.
func keycloakFleetTokenURLFor(keycloakNamespace, appNamespace string) string {
	if keycloakNamespace == appNamespace {
		return fleetKeycloakTokenURL
	}
	return fmt.Sprintf("https://keycloak.%s.svc.cluster.local:8443/realms/kubernaut-fleet/protocol/openid-connect/token", keycloakNamespace)
}

// deployFleetOAuth2Secret creates the shared client_credentials Secret every
// fleet-aware service mounts to authenticate its own MCP Gateway calls.
// pkg/fleet/mcpclient.ReloadableOAuth2Config (and its per-service
// equivalents in pkg/signalprocessing/config, pkg/workflowexecution/config)
// expect "client-id" and "client-secret" keys -- see the
// buildFleetReaderFactory-style call sites in cmd/*/main.go.
func deployFleetOAuth2Secret(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	manifest := fmt.Sprintf(`---
apiVersion: v1
kind: Secret
metadata:
  name: %[2]s
  namespace: %[1]s
  labels:
    component: fleet
type: Opaque
stringData:
  client-id: "kubernaut-fleet-read"
  client-secret: "e2e-fleet-secret"
`, namespace, fleetOAuth2SecretName)
	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, manifest); err != nil {
		return fmt.Errorf("fleet OAuth2 secret creation failed: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "    ✅ Fleet OAuth2 credentials Secret created")
	return nil
}

// DeployFleetCoreInfra deploys the fleet-core infrastructure in the Kind
// cluster, independent of any Kubernaut service:
//
// Phase 1: Gateway API CRDs + Istio (control plane only, mesh disabled)
// Phase 2: Istio Gateway + NodePort + Kuadrant MCP Gateway
// Phase 3: kube-mcp-server backend + MCPServerRegistration
// Phase 4: Valkey + FMC
//
// Istio is deployed via `helm template | kubectl apply` (Helm as renderer only,
// no Helm release). All other components use `kubectl apply` with inline YAML
// or upstream Kustomize URLs.
//
// This function (Phase 4 in particular) is only used by callers that need a
// raw-manifest FMC/Valkey stack alongside the gateway -- the dedicated
// fleetmetadatacache E2E lane (SetupFMCE2EInfrastructure), which deploys
// only DataStorage + Dex + this core alongside FMC. The full "fleet" suite
// (SetupFleetE2EInfrastructure) chart-manages FMC/Valkey instead (FMC's
// enabled state is derived by the chart itself from global.fleet.enabled,
// DD-TEST-015 / DD-PLATFORM-006 Decision Area 10) and calls
// DeployFleetGatewayInfra (Phases 1-3 only) directly.
//
// authConfig controls how kube-mcp-server authenticates to the target
// Kubernetes API server -- see KubeMCPServerAuthConfig. Both the "fleet"
// suite and the FMC E2E lane pass a passthrough+STS config (Keycloak +
// RFC 8693 token exchange) to validate the real token-exchange wiring.
//
// fmcOAuth2Config controls FMC's own OAuth2 client_credentials IdP endpoint
// -- see FMCOAuth2Config. Both lanes point this at Keycloak's token
// endpoint (issue #1554 retired the "fleet" suite's earlier Dex-based
// kubeconfig-mode config).
//
// Total memory: ~1.7-2.5 GB (passthrough mode, Keycloak).
func DeployFleetCoreInfra(ctx context.Context, namespace, kubeconfigPath, fmcImage string, authConfig KubeMCPServerAuthConfig, fmcOAuth2Config FMCOAuth2Config, enableCoverage bool, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "🚀 Deploying Fleet Core E2E Infrastructure...")

	mcpGatewayEndpoint, err := DeployFleetGatewayInfra(ctx, namespace, kubeconfigPath, authConfig, writer)
	if err != nil {
		return err
	}

	// ── Phase 4: FMC Stack (Valkey + FMC) ───────────────────────────────
	return deployValkeyAndFMC(ctx, namespace, kubeconfigPath, fmcImage, mcpGatewayEndpoint, authConfig, fmcOAuth2Config, enableCoverage, writer)
}

// DeployFleetGatewayInfra deploys Phases 1-3 of DeployFleetCoreInfra --
// Gateway API CRDs + Istio (or Envoy AI Gateway), the Kuadrant/EAIGW MCP
// Gateway itself, and the kube-mcp-server backend + registrations -- WITHOUT
// Phase 4 (Valkey + FMC). Extracted (DD-TEST-015) so callers that
// chart-manage FMC/Valkey themselves via `helm install` (FMC's enabled state
// derived from global.fleet.enabled per DD-PLATFORM-006 Decision Area 10;
// valkey.enabled defaults true -- see SetupFleetE2EInfrastructure) can
// deploy just the gateway, instead of DeployFleetCoreInfra's Phase 4
// deploying a redundant, raw-manifest FMC/Valkey stack alongside the
// chart-managed one. DeployFleetCoreInfra remains the entry point for
// callers that still need Phase 4 (the standalone FMC E2E lane, which does
// not install the Helm chart at all).
//
// Returns mcpGatewayEndpoint for wiring into whatever consumes it (chart
// --set values via FleetHelmOptions, FMCOAuth2Config-adjacent callers, etc.).
func DeployFleetGatewayInfra(ctx context.Context, namespace, kubeconfigPath string, authConfig KubeMCPServerAuthConfig, writer io.Writer) (string, error) {
	// ── Phase 1-2: Gateway (Kuadrant or Envoy AI Gateway) ────────────────
	if authConfig.GatewayType == "" {
		authConfig.GatewayType = registry.GatewayKuadrant // backward-compatible default
	}
	var mcpGatewayEndpoint string
	switch authConfig.GatewayType {
	case registry.GatewayEAIGW:
		_, _ = fmt.Fprintln(writer, "\n  🌐 Phase 1-2: Deploying Envoy AI Gateway (EAIGW, Spike S18)...")
		svcFQDN, eaigwErr := deployEnvoyAIGatewayInfra(ctx, namespace, kubeconfigPath, writer)
		if eaigwErr != nil {
			return "", fmt.Errorf("envoy AI Gateway deployment failed: %w", eaigwErr)
		}
		mcpGatewayEndpoint = fmt.Sprintf("http://%s:8080/mcp", svcFQDN)
	default:
		if kuadrantErr := deployKuadrantGatewayInfra(ctx, kubeconfigPath, writer); kuadrantErr != nil {
			return "", fmt.Errorf("kuadrant gateway deployment failed: %w", kuadrantErr)
		}
		mcpGatewayEndpoint = "http://mcp-gateway-istio.gateway-system.svc:8080/mcp"
	}

	// ── Phase 3: Backend MCP Server ─────────────────────────────────────
	_, _ = fmt.Fprintln(writer, "\n  🔌 Phase 3: Deploying kube-mcp-server backend...")
	if err := deployKubeMCPServerAndRegister(ctx, namespace, kubeconfigPath, mcpGatewayEndpoint, authConfig, writer); err != nil {
		return "", err
	}
	return mcpGatewayEndpoint, nil
}

// deployKuadrantGatewayInfra installs the Istio-based Kuadrant MCP Gateway
// stack (CRDs, controller, broker) -- the default/original gateway for the
// FMC E2E lane. See deployEnvoyAIGatewayInfra for the EAIGW alternative
// (Spike S18).
func deployKuadrantGatewayInfra(ctx context.Context, kubeconfigPath string, writer io.Writer) error {
	// ── Phase 1: CRDs and Istio ─────────────────────────────────────────
	_, _ = fmt.Fprintln(writer, "\n  📋 Phase 1: Installing CRDs and Istio control plane...")

	_, _ = fmt.Fprintln(writer, "    Installing Gateway API CRDs...")
	if err := runKubectl(ctx, kubeconfigPath, writer, "apply", "-f", gatewayAPICRDsURL); err != nil {
		return fmt.Errorf("gateway API CRDs install failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "    Adding Istio Helm repo...")
	addRepo := exec.CommandContext(ctx, "helm", "repo", "add", "istio", istioHelmRepoURL)
	addRepo.Stdout = writer
	addRepo.Stderr = writer
	_ = addRepo.Run() // ignore if already exists

	updateRepo := exec.CommandContext(ctx, "helm", "repo", "update", "istio")
	updateRepo.Stdout = writer
	updateRepo.Stderr = writer
	if err := updateRepo.Run(); err != nil {
		return fmt.Errorf("helm repo update failed: %w", err)
	}

	// Create istio-system namespace before applying Istio base CRDs.
	// helm template renders namespaced resources (e.g. ValidatingWebhookConfiguration
	// with service references) that fail if the namespace doesn't exist yet.
	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, `
apiVersion: v1
kind: Namespace
metadata:
  name: istio-system
`); err != nil {
		return fmt.Errorf("istio-system namespace creation failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "    Installing Istio base (CRDs)...")
	if err := runHelmTemplateApply(ctx, kubeconfigPath, writer,
		"istio-base", "istio/base", "istio-system",
		"--version", "1.30.2",
	); err != nil {
		return fmt.Errorf("istio base install failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "    Installing Istio control plane (mesh disabled)...")
	if err := runHelmTemplateApply(ctx, kubeconfigPath, writer,
		"istiod", "istio/istiod", "istio-system",
		"--version", "1.30.2",
		"--set", "global.proxy.autoInject=disabled",
		"--set", "sidecarInjectorWebhook.enableNamespacesByDefault=false",
	); err != nil {
		return fmt.Errorf("istio istiod install failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "    Waiting for istiod to be ready...")
	if err := waitForDeployment(ctx, "istiod", "istio-system", kubeconfigPath, 180*time.Second, writer); err != nil {
		return fmt.Errorf("istiod rollout failed: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "    ✅ Istio control plane ready")

	// ── Phase 2: Gateway and Kuadrant ───────────────────────────────────
	_, _ = fmt.Fprintln(writer, "\n  🌐 Phase 2: Creating Gateway and deploying Kuadrant...")

	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, `
apiVersion: v1
kind: Namespace
metadata:
  name: gateway-system
`); err != nil {
		return fmt.Errorf("gateway-system namespace creation failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "    Creating Istio Gateway (listener mcp:8080)...")
	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, `
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: mcp-gateway
  namespace: gateway-system
  annotations:
    networking.istio.io/service-type: NodePort
spec:
  gatewayClassName: istio
  listeners:
  - name: mcp
    port: 8080
    protocol: HTTP
    hostname: "*.127-0-0-1.sslip.io"
    allowedRoutes:
      namespaces:
        from: All
`); err != nil {
		return fmt.Errorf("gateway resource creation failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "    Waiting for Istio gateway service...")
	if err := waitForResource(ctx, kubeconfigPath, "service", "mcp-gateway-istio", "gateway-system", 60*time.Second); err != nil {
		return fmt.Errorf("istio gateway service not found: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "    Patching NodePort to 31975 (MCP) and 31500 (status)...")
	if err := runKubectl(ctx, kubeconfigPath, writer,
		"patch", "service", "mcp-gateway-istio", "-n", "gateway-system",
		"--type=json",
		`-p=[{"op":"replace","path":"/spec/ports/0/nodePort","value":31500},{"op":"replace","path":"/spec/ports/1/nodePort","value":31975}]`,
	); err != nil {
		return fmt.Errorf("nodePort patch failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "    Installing Kuadrant CRDs...")
	if err := runKubectl(ctx, kubeconfigPath, writer, "apply", "-k", kuadrantCRDsKustomize); err != nil {
		return fmt.Errorf("kuadrant CRDs install failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "    Deploying Kuadrant MCP Gateway (controller + broker + HTTPRoute)...")
	if err := runKubectl(ctx, kubeconfigPath, writer, "apply", "-k", kuadrantOverlayKustomize); err != nil {
		return fmt.Errorf("kuadrant deployment failed: %w", err)
	}

	// ReferenceGrant: MCPGatewayExtension in mcp-system references a Gateway
	// in gateway-system. Without this grant the controller refuses to create
	// the broker deployment (status: ReferenceGrantRequired).
	// Authority: https://docs.kuadrant.io/dev/mcp-gateway/docs/guides/isolated-gateway-deployment/
	_, _ = fmt.Fprintln(writer, "    Creating ReferenceGrant (mcp-system → gateway-system)...")
	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, `
apiVersion: gateway.networking.k8s.io/v1beta1
kind: ReferenceGrant
metadata:
  name: allow-mcp-extension
  namespace: gateway-system
spec:
  from:
  - group: mcp.kuadrant.io
    kind: MCPGatewayExtension
    namespace: mcp-system
  to:
  - group: gateway.networking.k8s.io
    kind: Gateway
`); err != nil {
		return fmt.Errorf("ReferenceGrant creation failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "    Waiting for Kuadrant controller...")
	if err := waitForDeployment(ctx, "mcp-gateway-controller", "mcp-system", kubeconfigPath, 120*time.Second, writer); err != nil {
		return fmt.Errorf("kuadrant controller rollout failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "    Waiting for Kuadrant broker (created by controller)...")
	if err := waitForDeployment(ctx, "mcp-gateway", "mcp-system", kubeconfigPath, 120*time.Second, writer); err != nil {
		return fmt.Errorf("kuadrant broker rollout failed: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "    ✅ Kuadrant MCP Gateway ready")
	return nil
}

// deployTraefikForKind installs Traefik as the hub cluster's Ingress
// controller, exposed via a fixed-NodePort Service matching the
// kind-fullpipeline-config.yaml host port mappings (8880/8843, deliberately
// unprivileged -- most CI runners can't bind a non-root process to 80/443).
// charts/kubernaut's console.ingress.port field (see console.yaml's
// oauth2-proxy --redirect-url) tells the chart to render the OIDC redirect
// URL with the matching ":8843" suffix instead of assuming port-less HTTPS.
//
// Manual setup-e2e-fleet-infra (Console) use only -- called from
// SetupFleetCoreInfrastructure, never from provisionFleetCoreInfra (shared
// with the "fleet"/"fullpipeline" Ginkgo suites, which don't use Console).
func deployTraefikForKind(ctx context.Context, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "    Adding Traefik Helm repo...")
	addRepo := exec.CommandContext(ctx, "helm", "repo", "add", "traefik", traefikHelmRepoURL)
	addRepo.Stdout = writer
	addRepo.Stderr = writer
	_ = addRepo.Run() // ignore if already exists

	updateRepo := exec.CommandContext(ctx, "helm", "repo", "update", "traefik")
	updateRepo.Stdout = writer
	updateRepo.Stderr = writer
	if err := updateRepo.Run(); err != nil {
		return fmt.Errorf("helm repo update failed: %w", err)
	}

	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, `
apiVersion: v1
kind: Namespace
metadata:
  name: traefik-system
`); err != nil {
		return fmt.Errorf("traefik-system namespace creation failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "    Installing Traefik (web→NodePort "+fmt.Sprint(traefikWebNodePort)+"→host 8880, websecure→NodePort "+fmt.Sprint(traefikWebsecureNodePort)+"→host 8843)...")
	if err := runHelmTemplateApply(ctx, kubeconfigPath, writer,
		"traefik", "traefik/traefik", "traefik-system",
		"--set", "service.type=NodePort",
		fmt.Sprintf("--set=ports.web.nodePort=%d", traefikWebNodePort),
		fmt.Sprintf("--set=ports.websecure.nodePort=%d", traefikWebsecureNodePort),
		"--set", "ingressClass.enabled=true",
		"--set", "ingressClass.isDefaultClass=true",
	); err != nil {
		return fmt.Errorf("traefik install failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "    Waiting for Traefik to be ready...")
	if err := waitForDeployment(ctx, "traefik", "traefik-system", kubeconfigPath, 120*time.Second, writer); err != nil {
		return fmt.Errorf("traefik rollout failed: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "    ✅ Traefik ready (ingressClassName: traefik)")
	return nil
}

// deployEnvoyAIGatewayInfra installs the Envoy AI Gateway (EAIGW) stack --
// CNCF Envoy Gateway + the AI Gateway layer on top -- as an alternative to
// Kuadrant, per Spike S18 (Phase A spike + Phase B mini-spike). Returns the
// in-cluster FQDN of Envoy Gateway's dynamically-named generated Service
// (envoy-<gw-namespace>-<gw-name>-<8-char-hash>, discovered via label
// selector -- there is no static Service name to hardcode, unlike Kuadrant's
// Istio-provisioned mcp-gateway-istio).
func deployEnvoyAIGatewayInfra(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) (string, error) {
	const (
		egNamespace   = "envoy-gateway-system"
		aiegNamespace = "envoy-ai-gateway-system"
		gatewayName   = "mcp-gateway"
	)

	// Envoy Gateway's Helm chart bundles the Gateway API CRDs by default
	// (v1.8.x), so -- unlike Kuadrant -- no separate gatewayAPICRDsURL apply
	// is needed here.
	_, _ = fmt.Fprintln(writer, "    Installing Envoy Gateway (bundles Gateway API CRDs)...")
	if err := runHelmUpgradeInstall(ctx, kubeconfigPath, writer, "eg", envoyGatewayHelmChart, egNamespace,
		"--version", envoyGatewayHelmVersion); err != nil {
		return "", fmt.Errorf("envoy gateway helm install failed: %w", err)
	}
	if err := waitForDeployment(ctx, "envoy-gateway", egNamespace, kubeconfigPath, 180*time.Second, writer); err != nil {
		return "", fmt.Errorf("envoy-gateway controller rollout failed: %w", err)
	}

	// ai-gateway-helm v1.0.0 does NOT bundle its own CRDs (Spike S18 gap #2)
	// -- the CRDs chart must be installed explicitly first.
	_, _ = fmt.Fprintln(writer, "    Installing Envoy AI Gateway CRDs + controller...")
	if err := runHelmUpgradeInstall(ctx, kubeconfigPath, writer, "aieg-crds", aiGatewayCRDsHelmChart, aiegNamespace,
		"--version", aiGatewayHelmVersion); err != nil {
		return "", fmt.Errorf("ai-gateway CRDs helm install failed: %w", err)
	}
	if err := runHelmUpgradeInstall(ctx, kubeconfigPath, writer, "aieg", aiGatewayHelmChart, aiegNamespace,
		"--version", aiGatewayHelmVersion); err != nil {
		return "", fmt.Errorf("ai-gateway helm install failed: %w", err)
	}
	if err := waitForDeployment(ctx, "ai-gateway-controller", aiegNamespace, kubeconfigPath, 180*time.Second, writer); err != nil {
		return "", fmt.Errorf("ai-gateway-controller rollout failed: %w", err)
	}

	// Spike S18 mini-spike gap #5: neither Helm chart grants the
	// envoy-gateway ServiceAccount RBAC to watch MCPRoute, even though
	// extensionManager.resources (below) declares it as a watched extension
	// resource. Without this, the envoy-gateway controller's cache sync
	// never completes and the data-plane pod never becomes ready.
	_, _ = fmt.Fprintln(writer, "    Granting envoy-gateway RBAC for MCPRoute (Spike S18 gap #5)...")
	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, fmt.Sprintf(`
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: envoy-gateway-mcproute-reader
rules:
- apiGroups: ["aigateway.envoyproxy.io"]
  resources: ["mcproutes", "mcproutes/status"]
  verbs: ["get", "list", "watch", "patch", "update"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: envoy-gateway-mcproute-reader-binding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: envoy-gateway-mcproute-reader
subjects:
- kind: ServiceAccount
  name: envoy-gateway
  namespace: %[1]s
`, egNamespace)); err != nil {
		return "", fmt.Errorf("envoy-gateway MCPRoute RBAC creation failed: %w", err)
	}

	// Spike S18 gap #3 (Phase A) + gap #6 (Phase B mini-spike): the
	// extensionManager needs enableBackend, the AI Gateway controller's
	// extension-server address, AND a full xdsTranslator.translation block
	// (not just hooks.xdsTranslator.post) -- omitting the translation block
	// reproduces the exact 192.0.2.42:9856 connection_timeout symptom even
	// with RBAC fixed, because the placeholder cluster address the MCP
	// sidecar starts with is never rewritten to 127.0.0.1:9856.
	_, _ = fmt.Fprintln(writer, "    Configuring envoy-gateway-config extensionManager (Spike S18 gap #3/#6)...")
	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, fmt.Sprintf(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: envoy-gateway-config
  namespace: %[1]s
data:
  envoy-gateway.yaml: |
    apiVersion: gateway.envoyproxy.io/v1alpha1
    kind: EnvoyGateway
    provider:
      type: Kubernetes
    extensionApis:
      enableEnvoyPatchPolicy: true
      enableBackend: true
    extensionManager:
      hooks:
        xdsTranslator:
          translation:
            listener: {includeAll: true}
            route: {includeAll: true}
            cluster: {includeAll: true}
            secret: {includeAll: true}
          post: [Translation, Cluster, Route]
      service:
        fqdn:
          hostname: ai-gateway-controller.%[2]s.svc.cluster.local
          port: 1063
      resources:
      - group: aigateway.envoyproxy.io
        version: v1beta1
        kind: MCPRoute
    gateway:
      controllerName: gateway.envoyproxy.io/gatewayclass-controller
    logging:
      level:
        default: info
`, egNamespace, aiegNamespace)); err != nil {
		return "", fmt.Errorf("envoy-gateway-config patch failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "    Restarting envoy-gateway to pick up extensionManager config...")
	if err := runKubectl(ctx, kubeconfigPath, writer, "rollout", "restart", "deployment/envoy-gateway", "-n", egNamespace); err != nil {
		return "", fmt.Errorf("envoy-gateway restart failed: %w", err)
	}
	if err := waitForDeployment(ctx, "envoy-gateway", egNamespace, kubeconfigPath, 120*time.Second, writer); err != nil {
		return "", fmt.Errorf("envoy-gateway rollout (post-restart) failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "    Creating GatewayClass + Gateway...")
	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, fmt.Sprintf(`
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: envoy-ai-gateway
spec:
  controllerName: gateway.envoyproxy.io/gatewayclass-controller
---
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: %[2]s
  namespace: %[1]s
spec:
  gatewayClassName: envoy-ai-gateway
  listeners:
  - name: mcp
    port: 8080
    protocol: HTTP
`, namespace, gatewayName)); err != nil {
		return "", fmt.Errorf("GatewayClass/Gateway creation failed: %w", err)
	}

	// Spike S18 mini-spike finding: Envoy Gateway's own generated Service
	// name (envoy-<gw-namespace>-<gw-name>-<8-char-hash>) cannot be
	// predicted ahead of time -- discover it via the owning-gateway labels
	// Envoy Gateway stamps on it, always created in egNamespace regardless
	// of which namespace the Gateway itself lives in.
	_, _ = fmt.Fprintln(writer, "    Discovering generated Gateway Service...")
	svcName, err := waitForLabeledService(ctx, kubeconfigPath, egNamespace,
		fmt.Sprintf("gateway.envoyproxy.io/owning-gateway-name=%s,gateway.envoyproxy.io/owning-gateway-namespace=%s", gatewayName, namespace),
		120*time.Second)
	if err != nil {
		return "", fmt.Errorf("gateway service discovery failed: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "    Patching NodePort to %d on generated service %s...\n", eaigwGatewayNodePort, svcName)
	if err := runKubectl(ctx, kubeconfigPath, writer,
		"patch", "service", svcName, "-n", egNamespace,
		"--type=json",
		fmt.Sprintf(`-p=[{"op":"replace","path":"/spec/type","value":"NodePort"},{"op":"replace","path":"/spec/ports/0/nodePort","value":%d}]`, eaigwGatewayNodePort),
	); err != nil {
		return "", fmt.Errorf("gateway service NodePort patch failed: %w", err)
	}

	svcFQDN := fmt.Sprintf("%s.%s.svc.cluster.local", svcName, egNamespace)
	_, _ = fmt.Fprintf(writer, "    ✅ Envoy AI Gateway ready (service: %s)\n", svcFQDN)
	return svcFQDN, nil
}

// deployEnvoyAIGatewayRegistrations creates the three Backends
// (loopback-cluster/remote-cluster, prod-east, prod-west) plus the single
// shared MCPRoute that aggregates them -- EAIGW's equivalent of Kuadrant's
// HTTPRoute + MCPServerRegistrations. EAIGW has no separate broker component:
// MCPRoute.spec.backendRefs natively aggregates multiple Backends, and each
// backend's tools are auto-prefixed "{backendRefs[].name}__{toolName}" with
// zero extra config (Spike S18 mini-spike, confirmed for 3 simultaneous
// backends). See loopbackClusterName below for why the first Backend is
// renamed "remote-cluster" when AllRegistrationsRemote is set.
func deployEnvoyAIGatewayRegistrations(ctx context.Context, namespace, kubeconfigPath, mcpGatewayEndpoint string, authConfig KubeMCPServerAuthConfig, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "    Creating Backends + MCPRoute (with OAuth SecurityPolicy)...")

	kubeMCPHostname := fmt.Sprintf("kube-mcp-server.%s.svc.cluster.local", namespace)
	keycloakHostname := fmt.Sprintf("keycloak.%s.svc.cluster.local", namespace)
	jwksURI := authConfig.AuthorizationURL + "/protocol/openid-connect/certs"

	// prod-east's Backend targets a genuinely separate Kind cluster's
	// kube-mcp-server via a Service+Endpoints bridge when RemoteBridge is
	// set (DD-TEST-013, Spike S19); otherwise it shares the loopback
	// hostname like every other Backend -- the original, unmodified
	// behavior for any caller that leaves RemoteBridge nil. When
	// AllRegistrationsRemote is also set (the "fleet" suite), loopback-cluster
	// and prod-west also route through the remote bridge hostname -- see the
	// matching comment in deployKuadrantRegistrations for why.
	loopbackHostname := kubeMCPHostname
	prodEastHostname, prodEastPort := kubeMCPHostname, 8080
	prodWestHostname := kubeMCPHostname

	// The first Backend is named/prefixed "loopback-cluster" EXCEPT when
	// AllRegistrationsRemote is set (the "fleet" suite): there, it is backed
	// by the genuinely remote bridge cluster, so it is renamed
	// "remote-cluster" (tool prefix becomes "remote-cluster__") to match
	// deployKuadrantRegistrations' identical loopbackClusterName renaming --
	// every fleet test hardcodes "remote-cluster" as its target identity, so
	// both gateway implementations must expose that same cluster name.
	loopbackClusterName := "loopback-cluster"
	if authConfig.AllRegistrationsRemote {
		loopbackClusterName = "remote-cluster"
	}
	if rb := authConfig.RemoteBridge; rb != nil {
		_, _ = fmt.Fprintln(writer, "    Bridging prod-east to remote cluster's kube-mcp-server (DD-TEST-013)...")
		if err := CreateServiceBridge(ctx, kubeconfigPath, namespace, rb.BridgeServiceName, rb.BridgeServicePort, rb.RemoteNodeIP, rb.RemoteNodePort, writer); err != nil {
			return fmt.Errorf("prod-east remote bridge Service creation failed: %w", err)
		}
		prodEastHostname = fmt.Sprintf("%s.%s.svc.cluster.local", rb.BridgeServiceName, namespace)
		prodEastPort = rb.BridgeServicePort
		if authConfig.AllRegistrationsRemote {
			loopbackHostname = prodEastHostname
			prodWestHostname = prodEastHostname
		}
	}

	manifest := fmt.Sprintf(`---
apiVersion: gateway.envoyproxy.io/v1alpha1
kind: Backend
metadata:
  name: keycloak-jwks
  namespace: %[1]s
spec:
  endpoints:
  - fqdn:
      hostname: %[2]s
      port: 8443
---
apiVersion: gateway.networking.k8s.io/v1alpha3
kind: BackendTLSPolicy
metadata:
  name: keycloak-jwks-tls
  namespace: %[1]s
spec:
  targetRefs:
  - group: gateway.envoyproxy.io
    kind: Backend
    name: keycloak-jwks
  validation:
    caCertificateRefs:
    - name: inter-service-ca
      group: ""
      kind: ConfigMap
    hostname: keycloak
---
apiVersion: gateway.envoyproxy.io/v1alpha1
kind: Backend
metadata:
  name: %[12]s
  namespace: %[1]s
  labels:
    kubernaut.ai/managed: "true"
    # BR-FLEET-003 (#1511): fleet onboarding label consumed by SP's Rego
    # cluster rule (input.cluster.labels.environment) via ClusterRegistry --
    # mirrors deployKuadrantRegistrations' identical label on its renamed
    # "remote-cluster" MCPServerRegistration.
    environment: "production"
spec:
  endpoints:
  - fqdn:
      hostname: %[10]s
      port: 8080
---
apiVersion: gateway.envoyproxy.io/v1alpha1
kind: Backend
metadata:
  name: prod-east
  namespace: %[1]s
  labels:
    kubernaut.ai/managed: "true"
spec:
  endpoints:
  - fqdn:
      hostname: %[8]s
      port: %[9]d
---
apiVersion: gateway.envoyproxy.io/v1alpha1
kind: Backend
metadata:
  name: prod-west
  namespace: %[1]s
  labels:
    kubernaut.ai/managed: "true"
spec:
  endpoints:
  - fqdn:
      hostname: %[11]s
      port: 8080
---
apiVersion: aigateway.envoyproxy.io/v1beta1
kind: MCPRoute
metadata:
  name: kube-mcp-server-route
  namespace: %[1]s
spec:
  parentRefs:
  - name: mcp-gateway
  path: /mcp
  # forwardHeaders is mandatory per backend: EAIGW's mcp-proxy does NOT
  # forward the client's validated Authorization header to backend MCP
  # servers by default (securityPolicy.oauth only authenticates the
  # downstream/edge hop) -- without it, kube-mcp-server's passthrough+STS
  # mode 401s with "Bearer token required" on every backend session the
  # proxy establishes (Spike S18 gap #7).
  backendRefs:
  - group: gateway.envoyproxy.io
    kind: Backend
    name: %[12]s
    forwardHeaders:
    - name: Authorization
  - group: gateway.envoyproxy.io
    kind: Backend
    name: prod-east
    forwardHeaders:
    - name: Authorization
  - group: gateway.envoyproxy.io
    kind: Backend
    name: prod-west
    forwardHeaders:
    - name: Authorization
  securityPolicy:
    oauth:
      issuer: %[4]q
      audiences: [%[5]q]
      jwks:
        remoteJWKS:
          uri: %[6]q
          backendRefs:
          - group: gateway.envoyproxy.io
            kind: Backend
            name: keycloak-jwks
            port: 8443
      protectedResourceMetadata:
        resource: %[7]q
`, namespace, keycloakHostname, kubeMCPHostname, authConfig.AuthorizationURL, authConfig.OAuthAudience, jwksURI, mcpGatewayEndpoint, prodEastHostname, prodEastPort, loopbackHostname, prodWestHostname, loopbackClusterName)

	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, manifest); err != nil {
		return fmt.Errorf("backend/MCPRoute creation failed: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "    ✅ Backends + MCPRoute created (%s, prod-east, prod-west)\n", loopbackClusterName)
	return nil
}

// deployKubeMCPServerAndRegister deploys kube-mcp-server (gateway-agnostic)
// and then registers it as three managed clusters (loopback-cluster,
// prod-east, prod-west) via the gateway-specific registration mechanism
// selected by authConfig.GatewayType: Kuadrant's HTTPRoute+MCPServerRegistration
// or EAIGW's Backend+MCPRoute (Spike S18).
func deployKubeMCPServerAndRegister(ctx context.Context, namespace, kubeconfigPath, mcpGatewayEndpoint string, authConfig KubeMCPServerAuthConfig, writer io.Writer) error {
	// AllRegistrationsRemote means every registration targets the remote
	// bridge (see registration deploy functions below), so a local
	// kube-mcp-server would be deployed but never referenced by any
	// registration -- skip it entirely (Issue #54: "remove the kube mcp
	// for the local cluster").
	if !authConfig.AllRegistrationsRemote {
		if err := deployKubeMCPServer(ctx, namespace, kubeconfigPath, authConfig, writer); err != nil {
			return err
		}
	} else {
		_, _ = fmt.Fprintln(writer, "    Skipping local kube-mcp-server (AllRegistrationsRemote: all registrations target the remote cluster)...")
	}

	if authConfig.GatewayType == registry.GatewayEAIGW {
		return deployEnvoyAIGatewayRegistrations(ctx, namespace, kubeconfigPath, mcpGatewayEndpoint, authConfig, writer)
	}
	return deployKuadrantRegistrations(ctx, namespace, kubeconfigPath, authConfig, writer)
}

// deployKubeMCPServer deploys the gateway-agnostic kube-mcp-server
// Deployment+Service (ServiceAccount, RBAC, ConfigMap, Deployment, Service)
// into the given cluster/namespace and waits for its rollout, without
// creating any gateway registration (HTTPRoute/MCPServerRegistration or
// Backend/MCPRoute -- see deployKubeMCPServerAndRegister for that).
//
// Extracted so SetupRemoteClusterForFMC (DD-TEST-013) can deploy a second,
// independent kube-mcp-server into a remote Kind cluster using the exact
// same manifest/auth-config logic as the primary cluster's loopback
// instance, without registering it as a local Gateway backend (the primary
// cluster's registration functions bridge to it instead -- see
// KubeMCPServerAuthConfig.RemoteBridge).
func deployKubeMCPServer(ctx context.Context, namespace, kubeconfigPath string, authConfig KubeMCPServerAuthConfig, writer io.Writer) error {
	// Issue #54 RCA background: kube-mcp-server v0.0.63 defaults
	// cluster_auth_mode to "passthrough", which forwards any incoming
	// Authorization: Bearer header straight to the Kubernetes API. FMC's
	// syncer authenticates to the Kuadrant MCP Gateway with an OAuth2
	// client_credentials JWT (Boundary 1, ADR-068); Kuadrant/Authorino does
	// not strip that header before proxying to this backend. ADR-068
	// Decision #9 / "Boundary 2: MCP Gateway -> Backend MCP Server" mandates
	// "no token delegation" by default -- see KubeMCPServerAuthModeKubeconfig.
	// The FMC E2E lane opts into KubeMCPServerAuthModePassthrough +
	// RFC 8693 token exchange instead, to validate that wiring for real
	// (Spike S17/S18); see KubeMCPServerAuthConfig.
	kubeMCPTOMLConfig := authConfig.tomlString()

	var kubeMCPExtraVolume, kubeMCPExtraVolumeMount string
	if authConfig.Mode == KubeMCPServerAuthModePassthrough {
		kubeMCPExtraVolume = TLSCAVolumeYAML(6)
		kubeMCPExtraVolumeMount = TLSCAVolumeMountYAML(8)
	}

	kubeMCPManifest := fmt.Sprintf(`---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: kube-mcp-server
  namespace: %[1]s
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kube-mcp-server-binding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: view
subjects:
- kind: ServiceAccount
  name: kube-mcp-server
  namespace: %[1]s
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: kube-mcp-server-config
  namespace: %[1]s
  labels:
    app: kube-mcp-server
    component: fleet
data:
  config.toml: |
%[3]s
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kube-mcp-server
  namespace: %[1]s
  labels:
    app: kube-mcp-server
    component: fleet
spec:
  replicas: 1
  selector:
    matchLabels:
      app: kube-mcp-server
  template:
    metadata:
      labels:
        app: kube-mcp-server
        component: fleet
    spec:
      serviceAccountName: kube-mcp-server
      containers:
      - name: kube-mcp-server
        image: %[2]s
        args:
        - "--port=8080"
        - "--cluster-provider=in-cluster"
        - "--toolsets=core"
        - "--stateless"
        - "--list-output=yaml"
        - "--config=/etc/kubernetes-mcp-server/config.toml"
        # --log-level=6 surfaces client-go's REST request/response detail in
        # must-gather captures, which was instrumental in diagnosing the
        # passthrough-401 root cause above (Issue #54 RCA).
        - "--log-level=6"
        ports:
        - name: http
          containerPort: 8080
        volumeMounts:
        - name: config
          mountPath: /etc/kubernetes-mcp-server
          readOnly: true%[4]s
        readinessProbe:
          httpGet:
            path: /healthz
            port: 8080
          initialDelaySeconds: 3
          periodSeconds: 5
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 10
        resources:
          requests:
            memory: "32Mi"
            cpu: "50m"
          limits:
            memory: "128Mi"
            cpu: "250m"
      volumes:
      - name: config
        configMap:
          name: kube-mcp-server-config%[5]s
---
apiVersion: v1
kind: Service
metadata:
  name: kube-mcp-server
  namespace: %[1]s
  labels:
    app: kube-mcp-server
    component: fleet
spec:
  ports:
  - name: http
    port: 8080
    targetPort: 8080
  selector:
    app: kube-mcp-server
`, namespace, KubeMCPServerImage, indentPEM(kubeMCPTOMLConfig), kubeMCPExtraVolumeMount, kubeMCPExtraVolume)

	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, kubeMCPManifest); err != nil {
		return fmt.Errorf("kube-mcp-server deployment failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "    Waiting for kube-mcp-server...")
	if err := waitForDeployment(ctx, "kube-mcp-server", namespace, kubeconfigPath, 120*time.Second, writer); err != nil {
		return fmt.Errorf("kube-mcp-server rollout failed: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "    ✅ kube-mcp-server ready")
	return nil
}

// deployKuadrantRegistrations creates the HTTPRoute + three
// MCPServerRegistrations (loopback-cluster, prod-east, prod-west) that
// register kube-mcp-server with the Kuadrant MCP Gateway broker.
func deployKuadrantRegistrations(ctx context.Context, namespace, kubeconfigPath string, authConfig KubeMCPServerAuthConfig, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "    Creating HTTPRoute + MCPServerRegistration...")

	// The broker maintains its own upstream tool-discovery/session-management
	// connection to kube-mcp-server, separate from per-request tools/call
	// proxying (which forwards the caller's own Authorization header
	// unmodified). When RequireOAuth=true, that discovery connection is
	// itself subject to kube-mcp-server's OAuth resource-server check, so the
	// broker needs its own static credential -- see BrokerCredentialToken doc
	// comment and https://docs.kuadrant.io/dev/mcp-gateway/docs/reference/mcpserverregistration/.
	var brokerCredSecretManifest, brokerCredRefYAML string
	if authConfig.BrokerCredentialToken != "" {
		// Kuadrant uses this Secret's value verbatim as the Authorization
		// header sent to the upstream MCP server -- it does not prepend the
		// "Bearer " scheme itself (confirmed against the "Bearer $GITHUB_PAT"
		// example in docs/guides/external-mcp-server.md, Step 4).
		brokerCredSecretManifest = fmt.Sprintf(`---
apiVersion: v1
kind: Secret
metadata:
  name: kube-mcp-server-broker-cred
  namespace: %s
  labels:
    mcp.kuadrant.io/secret: "true"
type: Opaque
stringData:
  token: "Bearer %s"
`, namespace, authConfig.BrokerCredentialToken)
		brokerCredRefYAML = "  credentialRef:\n    name: kube-mcp-server-broker-cred\n"
	}

	// prod-east routes through a dedicated HTTPRoute bridged to a genuinely
	// separate Kind cluster's kube-mcp-server when RemoteBridge is set
	// (DD-TEST-013, Spike S19); otherwise it shares the loopback HTTPRoute
	// like every other registration -- the original, unmodified behavior
	// for any caller that leaves RemoteBridge nil. When AllRegistrationsRemote
	// is also set (the "fleet" suite), the first registration (renamed
	// "remote-cluster", see loopbackClusterName below) and prod-west route
	// through the same remote HTTPRoute too, instead of only prod-east --
	// every fleet test hardcodes "remote-cluster" as its target identity
	// (not "prod-east"), so that name must be the one backed by the remote
	// cluster for the suite to exercise genuinely remote reads end-to-end.
	loopbackRouteName := kubeMcpServerRoute
	prodEastRouteName := kubeMcpServerRoute
	prodWestRouteName := kubeMcpServerRoute
	var remoteRouteManifest string
	if rb := authConfig.RemoteBridge; rb != nil {
		_, _ = fmt.Fprintln(writer, "    Bridging prod-east to remote cluster's kube-mcp-server (DD-TEST-013)...")
		if err := CreateServiceBridge(ctx, kubeconfigPath, namespace, rb.BridgeServiceName, rb.BridgeServicePort, rb.RemoteNodeIP, rb.RemoteNodePort, writer); err != nil {
			return fmt.Errorf("prod-east remote bridge Service creation failed: %w", err)
		}
		prodEastRouteName = kubeMcpServerRemoteRoute
		if authConfig.AllRegistrationsRemote {
			loopbackRouteName = kubeMcpServerRemoteRoute
			prodWestRouteName = kubeMcpServerRemoteRoute
		}
		remoteRouteManifest = fmt.Sprintf(`---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: %[3]s
  namespace: %[1]s
spec:
  hostnames:
  - kube-mcp-server-remote.127-0-0-1.sslip.io
  parentRefs:
  - name: mcp-gateway
    namespace: gateway-system
    sectionName: mcp
  rules:
  - backendRefs:
    - name: %[2]s
      port: %[4]d
`, namespace, rb.BridgeServiceName, prodEastRouteName, rb.BridgeServicePort)
	}

	// The local kube-mcp-server-route HTTPRoute backs the local
	// kube-mcp-server Service; when AllRegistrationsRemote skips deploying
	// that Service (deployKubeMCPServerAndRegister), skip creating this
	// dangling route too -- no registration references it in that mode
	// (loopbackRouteName/prodWestRouteName are both the remote route then).
	var localRouteManifest string
	if !authConfig.AllRegistrationsRemote {
		localRouteManifest = fmt.Sprintf(`---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: kube-mcp-server-route
  namespace: %[1]s
spec:
  hostnames:
  - kube-mcp-server.127-0-0-1.sslip.io
  parentRefs:
  - name: mcp-gateway
    namespace: gateway-system
    sectionName: mcp
  rules:
  - backendRefs:
    - name: kube-mcp-server
      port: 8080
`, namespace)
	}

	// The first registration is named/prefixed "loopback-cluster" /
	// "loopback_cluster_" everywhere EXCEPT when AllRegistrationsRemote is
	// set (the "fleet" suite): there, it is backed by the genuinely remote
	// bridge cluster (loopbackRouteName above), so it is renamed
	// "remote-cluster" / "remote_cluster_" to avoid implying it's the local
	// loopback cluster it named for every other caller of this shared
	// function (FMC E2E lanes).
	loopbackClusterName := "loopback-cluster"
	loopbackClusterPrefix := "loopback_cluster_"
	if authConfig.AllRegistrationsRemote {
		loopbackClusterName = "remote-cluster"
		loopbackClusterPrefix = "remote_cluster_"
	}

	routeManifest := fmt.Sprintf(`%[2]s%[5]s%[8]s---
apiVersion: mcp.kuadrant.io/v1alpha1
kind: MCPServerRegistration
metadata:
  name: %[9]s
  namespace: %[1]s
  labels:
    kubernaut.ai/managed: "true"
    # BR-FLEET-003 (#1511): fleet onboarding label consumed by SP's Rego
    # cluster rule (input.cluster.labels.environment) via ClusterRegistry.
    environment: "production"
spec:
  prefix: %[10]q
%[3]s  targetRef:
    group: gateway.networking.k8s.io
    kind: HTTPRoute
    name: %[6]s
    namespace: %[1]s
---
apiVersion: mcp.kuadrant.io/v1alpha1
kind: MCPServerRegistration
metadata:
  name: prod-east
  namespace: %[1]s
  labels:
    kubernaut.ai/managed: "true"
spec:
  prefix: "prod_east_"
%[3]s  targetRef:
    group: gateway.networking.k8s.io
    kind: HTTPRoute
    name: %[4]s
    namespace: %[1]s
---
apiVersion: mcp.kuadrant.io/v1alpha1
kind: MCPServerRegistration
metadata:
  name: prod-west
  namespace: %[1]s
  labels:
    kubernaut.ai/managed: "true"
spec:
  prefix: "prod_west_"
%[3]s  targetRef:
    group: gateway.networking.k8s.io
    kind: HTTPRoute
    name: %[7]s
    namespace: %[1]s
`, namespace, brokerCredSecretManifest, brokerCredRefYAML, prodEastRouteName, remoteRouteManifest, loopbackRouteName, prodWestRouteName, localRouteManifest, loopbackClusterName, loopbackClusterPrefix)

	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, routeManifest); err != nil {
		return fmt.Errorf("httpRoute/MCPServerRegistration creation failed: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "    ✅ MCPServerRegistrations created (%s, prod-east, prod-west)\n", loopbackClusterName)
	return nil
}

// deployValkeyAndFMC deploys Valkey and FMC itself, wiring FMC's
// mcpGateway.endpoint/gatewayType to whichever gateway was deployed in
// Phase 1-2.
//
// Valkey is deployed conditionally (Issue #1737 regression): this function
// is now ONLY reached via DeployFleetCoreInfra's Phase 4, used by the
// standalone FMC E2E lane (SetupFMCE2EInfrastructure), which deploys only
// DataStorage + Dex + this core -- no chart-managed Valkey exists there.
// (The full "fleet" suite, SetupFleetE2EInfrastructure, chart-manages both
// Valkey and FMC via `helm install` instead -- DD-TEST-015 -- and calls
// DeployFleetGatewayInfra directly, never reaching this function at all.)
// The check below guards against the case where a chart-managed Valkey
// Deployment/Service already exists in the target namespace regardless
// (charts/kubernaut/templates/infrastructure/valkey.yaml, valkey.enabled
// defaults true, backing APIFrontend's session/rate-limit cache): applying
// this function's own minimal valkey manifest on top of the chart's richer
// one (ServiceAccount, PVC, exec readinessProbe) previously caused a
// strategic-merge-patch collision -- the API server rejected the merged
// Deployment ("readinessProbe.tcpSocket: Forbidden: may not specify more
// than 1 handler type") since the object lacks the
// kubectl.kubernetes.io/last-applied-configuration annotation Helm-created
// resources don't carry. Same instance, same port 6379, no auth on either
// side -- safe to reuse as-is.
// applyStandaloneValkeyManifest creates a minimal, unauthenticated Valkey
// Deployment+Service for E2E lanes that don't install the Helm chart (see
// deployValkeyAndFMC's doc comment for why this is only called conditionally).
func applyStandaloneValkeyManifest(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	valkeyManifest := fmt.Sprintf(`---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: valkey
  namespace: %[1]s
  labels:
    app: valkey
    component: fleet
spec:
  replicas: 1
  selector:
    matchLabels:
      app: valkey
  template:
    metadata:
      labels:
        app: valkey
        component: fleet
    spec:
      containers:
      - name: valkey
        image: %[2]s
        ports:
        - name: valkey
          containerPort: 6379
        readinessProbe:
          tcpSocket:
            port: 6379
          initialDelaySeconds: 3
          periodSeconds: 5
        resources:
          requests:
            memory: "30Mi"
            cpu: "25m"
          limits:
            memory: "64Mi"
            cpu: "100m"
---
apiVersion: v1
kind: Service
metadata:
  name: valkey
  namespace: %[1]s
  labels:
    app: valkey
    component: fleet
spec:
  ports:
  - name: valkey
    port: 6379
    targetPort: 6379
  selector:
    app: valkey
`, namespace, valkeyImage)

	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, valkeyManifest); err != nil {
		return fmt.Errorf("valkey deployment failed: %w", err)
	}
	return nil
}

func deployValkeyAndFMC(ctx context.Context, namespace, kubeconfigPath, fmcImage, mcpGatewayEndpoint string, authConfig KubeMCPServerAuthConfig, fmcOAuth2Config FMCOAuth2Config, enableCoverage bool, writer io.Writer) error {
	// ── Phase 4: FMC Stack (Valkey + FMC) ───────────────────────────────
	_, _ = fmt.Fprintln(writer, "\n  💾 Phase 4: Deploying FMC stack (Valkey + FMC)...")

	checkCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "-n", namespace,
		"get", "deployment", "valkey", "-o", "name")
	if checkErr := checkCmd.Run(); checkErr == nil {
		_, _ = fmt.Fprintln(writer, "    Valkey already deployed (chart-managed, shared with APIFrontend) -- skipping redundant apply")
	} else if err := applyStandaloneValkeyManifest(ctx, namespace, kubeconfigPath, writer); err != nil {
		return err
	}

	_, _ = fmt.Fprintln(writer, "    Waiting for Valkey...")
	if err := waitForDeployment(ctx, "valkey", namespace, kubeconfigPath, 60*time.Second, writer); err != nil {
		return fmt.Errorf("valkey rollout failed: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "    ✅ Valkey ready")

	var fmcOAuth2ScopesYAML string
	if len(fmcOAuth2Config.Scopes) > 0 {
		var b strings.Builder
		b.WriteString("\n      scopes:")
		for _, s := range fmcOAuth2Config.Scopes {
			_, _ = fmt.Fprintf(&b, "\n        - %q", s)
		}
		fmcOAuth2ScopesYAML = b.String()
	}

	// FMC's ClusterRole is scoped to the specific gateway CRD it watches
	// (registry.MCPGatewayType, discovery.go): Kuadrant's
	// MCPServerRegistration+Gateway/HTTPRoute, or EAIGW's Backend
	// (gateway.envoyproxy.io/v1alpha1, eaigw_registry.go BackendGVR --
	// MCPRoute itself is not watched by FMC, only Backends).
	//
	// Issue #1993 (ADR-068 gap closure, IA-2/AC-3): tokenreviews/
	// subjectaccessreviews are appended unconditionally (gateway-agnostic)
	// -- FMC's own auth middleware (wireFMCDependencies,
	// cmd/fleetmetadatacache/main.go) needs these to validate the bearer
	// token GW/RO present on every scope-check request. Mirrors
	// charts/kubernaut/templates/rbac/fmc-scope-check-client-rbac.yaml's
	// fleetmetadatacache-auth-middleware ClusterRole (this lane deploys raw
	// manifests, not the Helm chart, so that template never renders here).
	fmcGatewayRBACRules := `
- apiGroups: ["mcp.kuadrant.io"]
  resources: ["mcpserverregistrations"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["gateway.networking.k8s.io"]
  resources: ["gateways", "httproutes"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["authentication.k8s.io"]
  resources: ["tokenreviews"]
  verbs: ["create"]
- apiGroups: ["authorization.k8s.io"]
  resources: ["subjectaccessreviews"]
  verbs: ["create"]`
	if authConfig.GatewayType == registry.GatewayEAIGW {
		fmcGatewayRBACRules = `
- apiGroups: ["gateway.envoyproxy.io"]
  resources: ["backends"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["authentication.k8s.io"]
  resources: ["tokenreviews"]
  verbs: ["create"]
- apiGroups: ["authorization.k8s.io"]
  resources: ["subjectaccessreviews"]
  verbs: ["create"]`
	}

	// DD-TEST-007 coverage instrumentation (#2300 Gap 4b): FMC's Dockerfile
	// already builds a GOFLAGS=-cover binary when requested
	// (docker/fleetmetadatacache.Dockerfile), but until now this manifest
	// never mounted a /coverdata volume or set GOCOVERDIR, so the
	// instrumented binary had nowhere to flush counters -- E2E runs with
	// E2E_COVERAGE=true silently produced zero FMC E2E coverage data.
	// Mirrors notificationControllerManifest's injection pattern
	// (notification_e2e.go) using the same shared fixtures.
	coverageEnvYAML := ""
	coverageVolumeMountYAML := ""
	coverageVolumeYAML := ""
	coverageSecurityContextYAML := ""
	if enableCoverage {
		coverageEnvYAML = "\n        env:" + coverageEnvYAMLFixture
		coverageVolumeMountYAML = `
        - name: coverdata
          mountPath: /coverdata`
		coverageVolumeYAML = `
      - name: coverdata
        hostPath:
          path: /coverdata
          type: DirectoryOrCreate`
		coverageSecurityContextYAML = coverageSecurityContextYAMLFixture
	}

	fmcManifest := fmt.Sprintf(`---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: fleetmetadatacache
  namespace: %[1]s
  labels:
    app: fleetmetadatacache
    component: fleet
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: fleetmetadatacache
  labels:
    app: fleetmetadatacache
rules:%[9]s
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: fleetmetadatacache
  labels:
    app: fleetmetadatacache
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: fleetmetadatacache
subjects:
- kind: ServiceAccount
  name: fleetmetadatacache
  namespace: %[1]s
---
# Issue #1993 (ADR-068 gap closure, IA-2/AC-3): dedicated identity for the E2E
# test harness itself to call FMC's now-authenticated scope-check API as a
# real, RBAC-authorized caller -- structurally mirrors
# charts/kubernaut/templates/rbac/fmc-scope-check-client-rbac.yaml's
# fmc-scope-check-client (gateway/remediationorchestrator-controller
# bindings), but this lane deploys raw manifests, not the Helm chart, and
# has no gateway/RO ServiceAccount of its own to reuse (see
# SetupFMCE2EInfrastructure's doc comment: this lane deploys FMC in
# isolation). See suite_test.go's SynchronizedBeforeSuite for the
# TokenRequest + testauth.NewStaticTokenTransport wiring that consumes this
# identity.
apiVersion: v1
kind: ServiceAccount
metadata:
  name: fmc-e2e-scope-check-client
  namespace: %[1]s
  labels:
    app: fleetmetadatacache
    component: fleet-test-client
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: fmc-e2e-scope-check-client
  labels:
    app: fleetmetadatacache
rules:
- apiGroups: [""]
  resources: ["services"]
  resourceNames: ["fleetmetadatacache-service"]
  verbs: ["get"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: fmc-e2e-scope-check-client
  labels:
    app: fleetmetadatacache
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: fmc-e2e-scope-check-client
subjects:
- kind: ServiceAccount
  name: fmc-e2e-scope-check-client
  namespace: %[1]s
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: fleetmetadatacache-config
  namespace: %[1]s
  labels:
    app: fleetmetadatacache
    component: fleet
data:
  config.yaml: |
    server:
      apiAddr: ":8080"
      healthAddr: ":8081"
      metricsAddr: ":9090"
      tls:
        certDir: /etc/tls
    # Issue #1683: proves SC-13 cipher/version restriction end-to-end against
    # a real Kind-deployed pod (E2E-FMC-1683-016). Not exposed via Helm
    # (operator-managed in production, Section 4.3 design decision) -- set
    # directly here since this lane deploys raw manifests, not the chart.
    tlsProfile: Intermediate
    mcpGateway:
      endpoint: "%[7]s"
      gatewayType: "%[8]s"
      namespace: "%[1]s"
    valkey:
      addr: "valkey.%[1]s.svc:6379"
    sync:
      interval: "10s"
      keyTtl: "30s"
    oauth2:
      tokenUrl: "%[3]s"
      credentialsDir: "/etc/fleetmetadatacache/fleet-oauth2"
      tlsCaFile: "/etc/fleetmetadatacache/tls-ca/ca.crt"%[6]s
---
apiVersion: v1
kind: Secret
metadata:
  name: fleetmetadatacache-oauth2
  namespace: %[1]s
  labels:
    app: fleetmetadatacache
    component: fleet
type: Opaque
stringData:
  client-id: %[4]s
  client-secret: %[5]s
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: fleetmetadatacache
  namespace: %[1]s
  labels:
    app: fleetmetadatacache
    component: fleet
spec:
  replicas: 1
  selector:
    matchLabels:
      app: fleetmetadatacache
  template:
    metadata:
      labels:
        app: fleetmetadatacache
        component: fleet
    spec:
      serviceAccountName: fleetmetadatacache%[10]s
      containers:
      - name: fleetmetadatacache
        image: %[2]s
        imagePullPolicy: IfNotPresent%[11]s
        volumeMounts:
        - name: config
          mountPath: /etc/fleetmetadatacache
          readOnly: true
        - name: oauth2-creds
          mountPath: /etc/fleetmetadatacache/fleet-oauth2
          readOnly: true
        - name: tls-ca
          mountPath: /etc/fleetmetadatacache/tls-ca
          readOnly: true
        - name: tls-certs
          mountPath: /etc/tls
          readOnly: true%[12]s
        ports:
        - name: api
          containerPort: 8080
        - name: health
          containerPort: 8081
        - name: metrics
          containerPort: 9090
        livenessProbe:
          httpGet:
            path: /healthz
            port: health
          initialDelaySeconds: 5
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /readyz
            port: health
          initialDelaySeconds: 3
          periodSeconds: 5
        resources:
          requests:
            memory: "64Mi"
            cpu: "50m"
          limits:
            # Issue #54 fleet E2E RCA (CI run 30464667745): 64Mi/100m was too
            # tight for FMC's actual workload (TLS/mTLS termination, OAuth2
            # token refresh, MCP client, concurrent scope-check serving under
            # ~12 parallel Ginkgo processes). Under load the process couldn't
            # service its own /healthz probe (default 1s timeout) in time,
            # kubelet killed+restarted the container (exitCode 137, liveness
            # probe "context deadline exceeded"), and every scope check
            # in-flight during that window failed with "context canceled" --
            # which the (pre-fix) handler indistinguishably reported as
            # managed=false, causing Gateway to reject alerts as unmanaged.
            # Raised to match/exceed the production Helm chart's default
            # (charts/kubernaut/values.yaml fleetmetadatacache.resources:
            # 128Mi/200m) since this test infra was actually *tighter* than
            # production.
            memory: "128Mi"
            cpu: "200m"
      volumes:
      - name: config
        configMap:
          name: fleetmetadatacache-config
      - name: oauth2-creds
        secret:
          secretName: fleetmetadatacache-oauth2
      - name: tls-ca
        configMap:
          name: inter-service-ca
      - name: tls-certs
        secret:
          secretName: fleetmetadatacache-tls
          optional: true%[13]s
---
apiVersion: v1
kind: Service
metadata:
  name: fleetmetadatacache-service
  namespace: %[1]s
  labels:
    app: fleetmetadatacache
    component: fleet
spec:
  ports:
  - name: api
    port: 8080
    targetPort: api
  - name: health
    port: 8081
    targetPort: health
  - name: metrics
    port: 9090
    targetPort: metrics
  selector:
    app: fleetmetadatacache
`, namespace, fmcImage, fmcOAuth2Config.TokenURL, fmcOAuth2Config.ClientID, fmcOAuth2Config.ClientSecret, fmcOAuth2ScopesYAML, mcpGatewayEndpoint, string(authConfig.GatewayType), fmcGatewayRBACRules, coverageSecurityContextYAML, coverageEnvYAML, coverageVolumeMountYAML, coverageVolumeYAML)

	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, fmcManifest); err != nil {
		return fmt.Errorf("fmc deployment failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "    Waiting for FMC...")
	if err := waitForDeployment(ctx, "fleetmetadatacache", namespace, kubeconfigPath, 120*time.Second, writer); err != nil {
		return fmt.Errorf("fmc rollout failed: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "    ✅ FMC ready")

	_, _ = fmt.Fprintln(writer, "✅ Fleet Core E2E infrastructure deployed (~388 MB)")
	return nil
}

// WaitForFleetReady verifies the MCP Gateway (Kuadrant or EAIGW) is reachable
// via NodePort by performing an MCP initialize handshake, then a real
// authenticated tools/call using tokenFunc (the FMC lane's Keycloak-based
// token func in fleetmetadatacache_e2e.go).
// nodePort/toolPrefix select the gateway-specific NodePort (Kuadrant 31975 /
// EAIGW 31976 per DD-TEST-001) and loopback-cluster tool-name prefix
// (Kuadrant's MCPServerRegistration "loopback_cluster_" vs EAIGW's
// auto-generated "loopback-cluster__", Spike S18).
func WaitForFleetReady(ctx context.Context, tokenFunc func() (string, error), nodePort int, toolPrefix string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "  ⏳ Verifying MCP Gateway reachability via NodePort...")

	gatewayURL := fmt.Sprintf("http://localhost:%d/mcp", nodePort)
	deadline := time.Now().Add(120 * time.Second)
	client := &http.Client{Timeout: 5 * time.Second}

	initReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": "2025-03-26",
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]string{
				"name":    "fleet-e2e-healthcheck",
				"version": "0.1",
			},
		},
	}
	body, _ := json.Marshal(initReq)

	for time.Now().Before(deadline) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, gatewayURL, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")

		resp, err := client.Do(req)
		if err == nil {
			// EAIGW's SecurityPolicy.oauth enforces auth on the entire MCPRoute,
			// including the `initialize` handshake (unlike Kuadrant's AuthPolicy,
			// which lets an unauthenticated `initialize` through) -- a 401 here
			// still proves the Gateway is up and the route has converged (Spike
			// S18); the authenticated tools/call probe below is the real
			// convergence check in both cases.
			if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusUnauthorized {
				_ = resp.Body.Close()
				_, _ = fmt.Fprintf(writer, "  ✅ MCP Gateway reachable (initialize → %d)\n", resp.StatusCode)
				return waitForAuthenticatedMCPGateway(ctx, tokenFunc, gatewayURL, toolPrefix, writer)
			}
			_ = resp.Body.Close()
		}
		time.Sleep(3 * time.Second)
	}
	return fmt.Errorf("mcp gateway not responsive at %s after 120 seconds", gatewayURL)
}

// waitForAuthenticatedMCPGateway performs a real authenticated tools/call against
// the Kuadrant MCP Gateway to verify Kuadrant's AuthPolicy/Envoy config has fully
// converged -- not just that the gateway pod is Ready and answers a bare,
// unauthenticated `initialize` (see the check above).
//
// Issue #54 FMC E2E RCA: fleetmetadatacache's syncer failed every syncKind call
// with `unsupported content type ""` -- the modelcontextprotocol/go-sdk client's
// error for a response with no Content-Type header, the signature of an Envoy
// local-reply auth denial rather than a real backend response -- even though
// WaitForFleetReady's plain `initialize` probe had already succeeded. kube-mcp-server's
// own logs showed zero resources_list invocations reaching it, confirming the calls
// were rejected upstream at Kuadrant/Envoy. The dedicated fleetmetadatacache E2E lane
// reaches FMC's first sync tick far sooner than the "fleet" suite (which spends ~10
// extra minutes deploying 10+ other services first), exposing a readiness race that
// "fleet" was accidentally masking via its longer boot time.
//
// This probe mirrors the real call FMC's syncer makes (pkg/fleet/fmc/syncer.go):
// an OAuth2 client_credentials token via tokenFunc, then a tools/call against
// the "loopback-cluster" MCPServerRegistration created earlier in Phase 3.
func waitForAuthenticatedMCPGateway(ctx context.Context, tokenFunc func() (string, error), gatewayURL, toolPrefix string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "  ⏳ Verifying authenticated tools/call succeeds (gateway AuthPolicy/SecurityPolicy convergence)...")

	// 180s (not the original 90s): PR #1755's CI run hit this exact timeout once
	// ("unsupported content type \"\"" -- Envoy local-reply, AuthPolicy not yet
	// converged) while an identical local run against the same CI-built images
	// converged well within 90s -- a CI-runner-resource-contention timing gap,
	// not a code defect, for the same reason DD-PLATFORM-008 gives fleet-aware
	// services a generous startupProbe budget instead of tightening a probe
	// that's occasionally-but-not-reliably fast enough.
	deadline := time.Now().Add(180 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := probeAuthenticatedResourcesList(ctx, tokenFunc, gatewayURL, toolPrefix); err != nil {
			lastErr = err
			time.Sleep(3 * time.Second)
			continue
		}
		_, _ = fmt.Fprintln(writer, "  ✅ Authenticated tools/call succeeded (gateway AuthPolicy/SecurityPolicy converged)")
		return nil
	}
	return fmt.Errorf("authenticated MCP tools/call did not succeed within 180s (gateway convergence failure): %w", lastErr)
}

// probeAuthenticatedResourcesList performs a single authenticated resources_list
// call against the loopback-cluster MCPServerRegistration, returning nil only on
// a genuinely successful (non-error) MCP response.
//
// Queries Pod across all namespaces (unfiltered) rather than mirroring FMC's
// actual kubernaut.ai/managed=true-filtered queries: this probe runs during
// infrastructure setup, before any test has labeled a resource, so a
// label-filtered query would legitimately return zero items. kube-mcp-server
// (with --list-output=yaml) omits structuredContent for empty result sets, and
// pkg/fleet/mcpclient.Client.List requires it -- an empty-but-successful result
// would otherwise be indistinguishable from an unconverged AuthPolicy here.
// Pod is always non-empty in a Kind cluster (kube-system's coredns/kube-proxy)
// and, unlike Node, is included in kube-mcp-server's bound "view" ClusterRole
// (the built-in "view" role does not grant list access to cluster-scoped Node
// resources -- confirmed by a prior run of this probe against Node, which
// failed with a clear RBAC "forbidden" error, not a convergence timeout).
func probeAuthenticatedResourcesList(ctx context.Context, tokenFunc func() (string, error), gatewayURL, toolPrefix string) error {
	token, err := tokenFunc()
	if err != nil {
		return fmt.Errorf("acquire IdP token: %w", err)
	}

	authClient := &http.Client{
		Timeout:   10 * time.Second,
		Transport: &bearerTokenTransport{token: token, base: http.DefaultTransport},
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	mcpConn, err := mcpclient.New(ctx, gatewayURL,
		mcpclient.WithToolPrefix(toolPrefix),
		mcpclient.WithHTTPClient(authClient),
	)
	if err != nil {
		return fmt.Errorf("connect to MCP Gateway: %w", err)
	}
	defer func() { _ = mcpConn.Close() }()

	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "PodList"})
	if err := mcpConn.List(ctx, list); err != nil {
		return fmt.Errorf("authenticated resources_list call: %w", err)
	}
	if len(list.Items) == 0 {
		return fmt.Errorf("authenticated resources_list call: succeeded but returned zero Pods (unexpected)")
	}
	return nil
}

// bearerTokenTransport injects a static Authorization: Bearer header into every
// outbound request. Used for the short-lived readiness probe above, where a
// single fetched token is sufficient (unlike production's WithReloadableOAuth2Transport,
// which refreshes on expiry for a long-running process).
type bearerTokenTransport struct {
	token string
	base  http.RoundTripper
}

func (t *bearerTokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(cloned)
}

// OIDCPatchConfig configures the OIDC flags patchAPIServerForOIDCConfig
// inserts into the Kind node's static kube-apiserver manifest.
type OIDCPatchConfig struct {
	// IssuerURL must match the IdP's configured issuer exactly so the `iss`
	// claim in JWTs matches the API server's expected value.
	// e.g. "https://dex:5556/dex" or "https://keycloak:8443/realms/kubernaut-fleet".
	IssuerURL string
	// ClientID must match a value present in the `aud` claim of tokens the
	// API server will see. For Dex (no token exchange) this is the caller's
	// own client ID; for Keycloak (with token exchange) this must be the
	// *exchanged* token's audience (e.g. "k8s-api"), not the original caller.
	ClientID string
	// UsernameClaim/UsernamePrefix select and namespace the K8s username.
	UsernameClaim  string
	UsernamePrefix string
	// GroupsClaim/GroupsPrefix are optional; leave both empty to omit
	// group-based RBAC entirely (e.g. Keycloak's exchanged identity is bound
	// directly by username, not by group).
	GroupsClaim  string
	GroupsPrefix string
}

// patchAPIServerForOIDCConfig adds OIDC flags to the API server static pod
// manifest on the Kind node per the given OIDCPatchConfig, enabling the K8s
// API server to accept JWTs from the configured issuer.
//
// This must be called AFTER the issuer (Dex or Keycloak) is deployed and
// running (the API server performs OIDC discovery on restart, requiring the
// issuer to be reachable). The kubelet detects the manifest change and
// automatically restarts the API server pod.
//
// issuerServiceNamespace is where the issuer's Service (e.g. "keycloak")
// lives, used to resolve its ClusterIP for the hostNetwork static pod's
// hostAliases patch (see patchAPIServerPodHostsForIssuer) -- callers must
// pass whatever namespace ACTUALLY holds that Service, which can differ
// from the app namespace (the fleet DEMO entry point's idpNamespace) or
// even from cfg's own cluster context (SetupRemoteClusterForFMC's remote
// cluster resolves its *local* "keycloak" bridge Service, not the primary
// cluster's real one).
//
// Re-run requirement: this function is only invoked once, during initial
// provisioning. The oidc-ca.crt file it writes below IS durable across API
// server restarts (a real file on the node's persistent filesystem, unlike
// the hostAliases patch's target which is baked into the manifest for the
// same reason) -- but it is NOT automatically kept in sync if the issuer's
// signing CA is ever regenerated later (e.g. the authwebhook-tls Secret,
// see provisionInterServiceCA, being deleted and recreated outside this
// function's control). If that ever happens post-provisioning, re-invoke
// this function to refresh both the CA file and the hostAliases ClusterIP
// entry in one pass -- there is no drift-detection/auto-refresh mechanism.
func patchAPIServerForOIDCConfig(ctx context.Context, clusterName, kubeconfigPath string, cfg OIDCPatchConfig, issuerServiceNamespace string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "\n🔑 Patching API server for OIDC (fleet kube-mcp-server passthrough auth)...")
	nodeName := clusterName + "-control-plane"

	// Copy the inter-service CA to the Kind node so the API server can verify
	// the issuer's TLS certificate during OIDC discovery. Dex and Keycloak
	// leaf certs are both signed by the same inter-service CA (interservice_tls.go),
	// so this file is shared regardless of which issuer is configured.
	caPEMPath := InterServiceCAPath(kubeconfigPath)
	caPEM, err := os.ReadFile(caPEMPath)
	if err != nil {
		return fmt.Errorf("failed to read inter-service CA from %s: %w", caPEMPath, err)
	}

	const nodeCAPath = "/etc/kubernetes/pki/oidc-ca.crt"
	writeCACmd := fmt.Sprintf("cat > %s << 'CAPEM'\n%sCAPEM", nodeCAPath, string(caPEM))
	cpCmd := exec.CommandContext(ctx, "podman", "exec", nodeName, "bash", "-c", writeCACmd)
	cpCmd.Stdout = writer
	cpCmd.Stderr = writer
	if err := cpCmd.Run(); err != nil {
		return fmt.Errorf("failed to write CA to Kind node at %s: %w", nodeCAPath, err)
	}
	_, _ = fmt.Fprintf(writer, "  CA certificate written to Kind node at %s\n", nodeCAPath)

	// Values ending with ':' must be quoted in YAML (otherwise parsed as mapping keys)
	oidcFlags := []string{
		fmt.Sprintf(`"--oidc-username-prefix=%s"`, cfg.UsernamePrefix),
		fmt.Sprintf("--oidc-username-claim=%s", cfg.UsernameClaim),
		fmt.Sprintf("--oidc-client-id=%s", cfg.ClientID),
		fmt.Sprintf("--oidc-ca-file=%s", nodeCAPath),
		fmt.Sprintf(`"--oidc-issuer-url=%s"`, cfg.IssuerURL),
	}
	if cfg.GroupsClaim != "" {
		oidcFlags = append(oidcFlags,
			fmt.Sprintf(`"--oidc-groups-prefix=%s"`, cfg.GroupsPrefix),
			fmt.Sprintf("--oidc-groups-claim=%s", cfg.GroupsClaim),
		)
	}

	// Insert flags one at a time (reverse order) so they appear in correct order
	// after the anchor line. Each `sed -i` appends one line after --tls-private-key-file.
	manifest := "/etc/kubernetes/manifests/kube-apiserver.yaml"
	for _, flag := range oidcFlags {
		sedCmd := fmt.Sprintf(`sed -i '/--tls-private-key-file/a\    - %s' %s`, flag, manifest)
		cmd := exec.CommandContext(ctx, "podman", "exec", nodeName, "bash", "-c", sedCmd)
		cmd.Stdout = writer
		cmd.Stderr = writer
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to insert %s: %w", flag, err)
		}
	}
	_, _ = fmt.Fprintln(writer, "  Manifest patched, waiting for API server restart...")

	// The kubelet detects the manifest change and restarts the API server.
	// It can take multiple restart cycles, so we wait for the OIDC flags to
	// appear in the running API server process, then confirm readyz stability.
	_, _ = fmt.Fprintln(writer, "  Waiting for API server to restart with OIDC flags...")

	deadline := time.Now().Add(120 * time.Second)

	// Phase 1: Wait until the OIDC flag appears in the running kube-apiserver process
	for time.Now().Before(deadline) {
		checkCmd := exec.CommandContext(ctx, "podman", "exec", nodeName, "bash", "-c",
			"pgrep -a kube-apiserver | grep -q oidc-issuer-url")
		if err := checkCmd.Run(); err == nil {
			_, _ = fmt.Fprintln(writer, "  OIDC flags detected in running API server process")
			break
		}
		time.Sleep(3 * time.Second)
	}

	// The static kube-apiserver pod runs with hostNetwork: true, but that only
	// shares the node's network *namespace* (interfaces, routes, resolv.conf
	// upstream servers) -- NOT its /etc/hosts. kubelet bind-mounts a
	// per-pod-UID hosts file (/var/lib/kubelet/pods/<uid>/etc-hosts) into every
	// pod, including hostNetwork ones, so the API server can never resolve the
	// issuer's bare in-cluster Service name (e.g. "keycloak", "dex") via
	// CoreDNS: it isn't on the pod network and the node's own DNS resolver has
	// no knowledge of cluster-internal names. Discovered via a live 401 loop
	// showing "oidc: authenticator not initialized" / "dial tcp: lookup
	// keycloak ... no such host" even after the issuer was healthy and
	// reachable from every actual cluster pod. Patch the *new* pod's
	// kubelet-managed hosts file with a static entry so OIDC discovery
	// (.well-known/openid-configuration + JWKS refresh) can resolve the
	// issuer host directly, bypassing CoreDNS entirely.
	if err := patchAPIServerPodHostsForIssuer(ctx, nodeName, kubeconfigPath, cfg.IssuerURL, issuerServiceNamespace, writer); err != nil {
		return fmt.Errorf("failed to patch API server pod hosts file for OIDC issuer resolution: %w", err)
	}

	// Phase 2: Wait for the new API server to become fully ready (5 consecutive readyz)
	consecutiveOK := 0
	for time.Now().Before(deadline) {
		checkCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
			"get", "--raw", "/readyz")
		if err := checkCmd.Run(); err == nil {
			consecutiveOK++
			if consecutiveOK >= 5 {
				_, _ = fmt.Fprintln(writer, "  ✅ API server restarted with OIDC enabled (readyz stable)")
				return nil
			}
		} else {
			consecutiveOK = 0
		}
		time.Sleep(3 * time.Second)
	}
	return fmt.Errorf("API server did not recover after OIDC patching within 120s")
}

// patchAPIServerPodHostsForIssuer resolves the OIDC issuer URL's hostname to
// its in-cluster Service ClusterIP and declares it as a hostAliases entry in
// the kube-apiserver static pod manifest, so the API server's OIDC
// discovery/JWKS HTTP client can resolve the issuer without relying on
// CoreDNS (unreachable from the hostNetwork static pod -- see the call
// site's doc comment). No-op if the issuer URL has no hostname (e.g.
// malformed).
//
// Issue found live 2026-08-30 (fleet demo Keycloak idp-namespace migration):
// an earlier version of this function appended the entry directly to
// kubelet's per-pod-UID hosts file (/var/lib/kubelet/pods/<uid>/etc-hosts)
// instead of the manifest. That file is NOT durable -- kubelet regenerates
// it from scratch (no custom entries) whenever it actually recreates the
// container, which happens for reasons entirely outside this function's
// control (node reboot, OOM kill, a manual restart during live debugging).
// The migration's Keycloak Service got a new ClusterIP, but the *previous*
// entry (from initial provisioning) kept silently working until the next
// unrelated container restart wiped it, at which point OIDC/JWKS resolution
// failed with an opaque connection timeout instead of a clear "stale config"
// signal. hostAliases is part of the Pod's declared spec, so kubelet bakes
// it into the generated hosts file on EVERY (re)creation -- durable across
// any future restart, not just the one immediately following this call.
//
// serviceNamespace is where the issuer's Service actually lives -- passed
// through from patchAPIServerForOIDCConfig's caller, not assumed to be
// kubernautSystem (that assumption broke once kube-mcp-server's remote
// bridge Service moved to mcp-system; see this function's callers for the
// namespace each one actually needs).
func patchAPIServerPodHostsForIssuer(ctx context.Context, nodeName, kubeconfigPath, issuerURL, serviceNamespace string, writer io.Writer) error {
	u, err := url.Parse(issuerURL)
	if err != nil || u.Hostname() == "" {
		return fmt.Errorf("failed to parse issuer hostname from %q: %w", issuerURL, err)
	}
	host := u.Hostname()

	// Retry: immediately after the API server static pod restarts (to pick
	// up the OIDC flags), there is a brief window where the freshly-started
	// process serves requests but its RBAC authorizer cache has not yet
	// synced ClusterRoleBindings from etcd, so even kubernetes-admin
	// (group kubeadm:cluster-admins) can transiently get a Forbidden on an
	// ordinary read. This resolves within a few seconds without any
	// intervention, so poll instead of failing on the first attempt.
	var clusterIP string
	svcDeadline := time.Now().Add(30 * time.Second)
	var svcErr error
	for time.Now().Before(svcDeadline) {
		svcIPCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
			"get", "svc", host, "-n", serviceNamespace, "-o", "jsonpath={.spec.clusterIP}")
		svcIPOut, err := svcIPCmd.Output()
		clusterIP = strings.TrimSpace(string(svcIPOut))
		if err == nil && clusterIP != "" {
			svcErr = nil
			break
		}
		svcErr = err
		clusterIP = ""
		time.Sleep(2 * time.Second)
	}
	if clusterIP == "" {
		return fmt.Errorf("failed to resolve ClusterIP for issuer service %q: %w", host, svcErr)
	}

	manifest := "/etc/kubernetes/manifests/kube-apiserver.yaml"

	// Idempotent: drop any hostAliases block this function previously wrote
	// (e.g. a re-run after the issuer Service's ClusterIP changed) before
	// inserting the current one, so re-invoking always converges on the
	// live ClusterIP instead of accumulating stale/duplicate entries.
	// kubeadm never generates hostAliases on its own, so any such block
	// present is guaranteed to be ours.
	removeScript := `/^  hostAliases:/,/^  containers:/{/^  containers:/!d}`
	rmCmd := exec.CommandContext(ctx, "podman", "exec", nodeName, "sed", "-i", removeScript, manifest)
	rmCmd.Stdout = writer
	rmCmd.Stderr = writer
	if err := rmCmd.Run(); err != nil {
		return fmt.Errorf("failed to remove any stale hostAliases block from %s: %w", manifest, err)
	}

	// Multi-line GNU/BSD sed insert syntax: each line but the last ends in
	// a literal trailing backslash. Passed as a single exec arg (no shell
	// involved), so no additional quoting is needed for the embedded
	// newlines.
	insertScript := fmt.Sprintf("/^  containers:/i\\\n  hostAliases:\\\n  - ip: %q\\\n    hostnames:\\\n    - %q", clusterIP, host)
	insCmd := exec.CommandContext(ctx, "podman", "exec", nodeName, "sed", "-i", insertScript, manifest)
	insCmd.Stdout = writer
	insCmd.Stderr = writer
	if err := insCmd.Run(); err != nil {
		return fmt.Errorf("failed to insert hostAliases entry for %q -> %q into %s: %w", host, clusterIP, manifest, err)
	}
	_, _ = fmt.Fprintf(writer, "  Patched API server manifest hostAliases: %s -> %s (durable across restarts)\n", host, clusterIP)
	return nil
}

// ── Helpers ─────────────────────────────────────────────────────────────

// runKubectl executes `kubectl --kubeconfig <path> <args...>`.
func runKubectl(ctx context.Context, kubeconfigPath string, writer io.Writer, args ...string) error {
	fullArgs := append([]string{"--kubeconfig", kubeconfigPath}, args...)
	cmd := exec.CommandContext(ctx, "kubectl", fullArgs...)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

// kubectlApplyManifest applies an inline YAML manifest via stdin.
func kubectlApplyManifest(ctx context.Context, kubeconfigPath string, writer io.Writer, manifest string) error {
	cmd := exec.CommandContext(ctx, "kubectl", "apply", "--kubeconfig", kubeconfigPath, "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

// runHelmTemplateApply renders a Helm chart and pipes the output to kubectl apply.
// Helm is used purely as a YAML renderer -- no Helm release is created.
func runHelmTemplateApply(ctx context.Context, kubeconfigPath string, writer io.Writer, releaseName, chart, namespace string, extraArgs ...string) error {
	helmArgs := []string{"template", releaseName, chart, "-n", namespace}
	helmArgs = append(helmArgs, extraArgs...)
	helmCmd := strings.Join(append([]string{"helm"}, helmArgs...), " ")
	kubectlCmd := fmt.Sprintf("kubectl apply --kubeconfig %s -f -", kubeconfigPath)

	script := helmCmd + " | " + kubectlCmd
	cmd := exec.CommandContext(ctx, "sh", "-c", script)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

// runHelmUpgradeInstall runs a real `helm upgrade --install` (as opposed to
// runHelmTemplateApply's render-only approach) -- required for the EAIGW
// Helm charts, which rely on Helm-native install ordering/CRD handling
// (Spike S18) rather than being safe to just template-and-apply.
func runHelmUpgradeInstall(ctx context.Context, kubeconfigPath string, writer io.Writer, releaseName, chart, namespace string, extraArgs ...string) error {
	args := append([]string{
		"upgrade", "--install", releaseName, chart,
		"--kubeconfig", kubeconfigPath,
		"-n", namespace, "--create-namespace",
	}, extraArgs...)
	cmd := exec.CommandContext(ctx, "helm", args...)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

// waitForLabeledService polls until exactly one Service matching the given
// label selector exists in namespace, returning its name. Used to discover
// Envoy Gateway's hash-suffixed generated Service
// (envoy-<gw-namespace>-<gw-name>-<8-char-hash>), which cannot be predicted
// ahead of time (Spike S18 mini-spike finding).
func waitForLabeledService(ctx context.Context, kubeconfigPath, namespace, labelSelector string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		cmd := exec.CommandContext(ctx, "kubectl", "get", "service",
			"-n", namespace, "--kubeconfig", kubeconfigPath,
			"-l", labelSelector, "-o", "jsonpath={.items[0].metadata.name}")
		var out strings.Builder
		cmd.Stdout = &out
		if err := cmd.Run(); err == nil && out.String() != "" {
			return out.String(), nil
		}
		time.Sleep(3 * time.Second)
	}
	return "", fmt.Errorf("no service matching label %q found in %s within %v", labelSelector, namespace, timeout)
}

// waitForDeployment polls until a deployment exists and then waits for rollout.
func waitForDeployment(ctx context.Context, name, namespace, kubeconfigPath string, timeout time.Duration, writer io.Writer) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		check := exec.CommandContext(ctx, "kubectl", "get", "deployment", name,
			"-n", namespace, "--kubeconfig", kubeconfigPath,
			"-o", "name")
		if check.Run() == nil {
			break
		}
		time.Sleep(5 * time.Second)
	}

	remaining := time.Until(deadline)
	if remaining <= 0 {
		return fmt.Errorf("deployment %s/%s not found within %v", namespace, name, timeout)
	}

	rollout := exec.CommandContext(ctx, "kubectl", "rollout", "status",
		fmt.Sprintf("deployment/%s", name),
		"-n", namespace,
		"--kubeconfig", kubeconfigPath,
		fmt.Sprintf("--timeout=%ds", int(remaining.Seconds())))
	rollout.Stdout = writer
	rollout.Stderr = writer
	return rollout.Run()
}

// waitForResource polls until a Kubernetes resource exists.
func waitForResource(ctx context.Context, kubeconfigPath, kind, name, namespace string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		check := exec.CommandContext(ctx, "kubectl", "get", kind, name,
			"-n", namespace, "--kubeconfig", kubeconfigPath,
			"-o", "name")
		if check.Run() == nil {
			return nil
		}
		time.Sleep(3 * time.Second)
	}
	return fmt.Errorf("%s %s/%s not found within %v", kind, namespace, name, timeout)
}

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
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
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
}

// DefaultKubeMCPServerAuthConfig returns the "fleet" full-pipeline E2E
// suite's auth config: kubeconfig mode, matching the Issue #54 RCA fix.
func DefaultKubeMCPServerAuthConfig() KubeMCPServerAuthConfig {
	return KubeMCPServerAuthConfig{Mode: KubeMCPServerAuthModeKubeconfig}
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

// DefaultFMCOAuth2Config returns the "fleet" full-pipeline E2E suite's FMC
// OAuth2 config, pointed at DEX.
func DefaultFMCOAuth2Config() FMCOAuth2Config {
	return FMCOAuth2Config{
		TokenURL:     "https://dex:5556/dex/token",
		ClientID:     "kubernaut-fleet-read",
		ClientSecret: "e2e-fleet-secret",
	}
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
// NodePort mapping (31975 for Kuadrant MCP) -- already present in
// kind-fullpipeline-config.yaml.
//
// The loopback pattern is used: the K8s MCP Server connects to the same cluster
// where it runs, but kubernaut treats it as a remote cluster with clusterID
// "loopback-cluster". This validates the full remote code path without needing
// a second Kind cluster.
//
// Total additional memory over fullpipeline: ~388 MB
// (Istio ~250 MB + Kuadrant ~60 MB + kube-mcp-server ~16 MB + Valkey ~30 MB + FMC ~32 MB).
//
// Authority: Issue #54, ADR-068
func SetupFleetE2EInfrastructure(ctx context.Context, clusterName, kubeconfigPath string, writer io.Writer) (builtImages map[string]string, seededUUIDs map[string]string, afRemediateNS map[string]string, err error) {
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "🚀 Fleet E2E Infrastructure (Issue #54)")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "  Base: Full Pipeline (all services)")
	_, _ = fmt.Fprintln(writer, "  Fleet: Kuadrant MCP Gateway + FMC + Valkey (loopback pattern)")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	cleanStaleTarFiles(writer)

	builtImages, seededUUIDs, afRemediateNS, err = SetupFullPipelineInfrastructure(ctx, clusterName, kubeconfigPath, writer)
	if err != nil {
		return builtImages, seededUUIDs, afRemediateNS, fmt.Errorf("fullpipeline base setup failed: %w", err)
	}

	if oidcErr := patchAPIServerForOIDC(ctx, clusterName, kubeconfigPath, writer); oidcErr != nil {
		return builtImages, seededUUIDs, afRemediateNS, fmt.Errorf("API server OIDC patching failed: %w", oidcErr)
	}

	namespace := "kubernaut-system"

	_, _ = fmt.Fprintln(writer, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "🌐 FLEET PHASE: Deploying Kuadrant MCP Gateway infrastructure...")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	_, _ = fmt.Fprintln(writer, "  📦 Pre-loading fleet external images...")
	for _, img := range []string{KubeMCPServerImage, kuadrantControllerImage, kuadrantBrokerImage, valkeyImage} {
		if loadErr := PreloadExternalImage(img, clusterName, writer); loadErr != nil {
			_, _ = fmt.Fprintf(writer, "  ⚠️  Image preload failed (will pull on-demand): %s: %v\n", img, loadErr)
		}
	}

	fmcImage := builtImages["fleetmetadatacache"]
	if fmcImage == "" {
		return builtImages, seededUUIDs, afRemediateNS, fmt.Errorf("fmc image not found in builtImages (was it built in Phase 1?)")
	}

	if deployErr := DeployFleetInfra(ctx, namespace, kubeconfigPath, fmcImage, writer); deployErr != nil {
		return builtImages, seededUUIDs, afRemediateNS, fmt.Errorf("fleet infra deployment failed: %w", deployErr)
	}

	if readyErr := WaitForFleetReady(DefaultDexFleetReadTokenFunc(), writer); readyErr != nil {
		return builtImages, seededUUIDs, afRemediateNS, fmt.Errorf("fleet readiness check failed: %w", readyErr)
	}

	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "✅ Fleet E2E Infrastructure READY")
	_, _ = fmt.Fprintln(writer, "  MCP Gateway:  http://localhost:31975/mcp")
	_, _ = fmt.Fprintln(writer, "  Loopback cluster ID: loopback-cluster")
	_, _ = fmt.Fprintln(writer, "  Loopback tool prefix: loopback_cluster_")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	return builtImages, seededUUIDs, afRemediateNS, nil
}

// DeployFleetInfra deploys the fleet E2E infrastructure in the Kind cluster,
// then enables fleet scope checking in Gateway and RemediationOrchestrator.
//
// This is a thin wrapper around DeployFleetCoreInfra (Phases 1-4, shared with
// the dedicated FMC E2E lane) plus Phase 5/5b, which assume Gateway and RO are
// already deployed in the cluster (true for the "fleet" full-pipeline suite,
// not true for a lighter FMC-only lane -- see DeployFleetCoreInfra).
//
// Total memory: ~388 MB.
func DeployFleetInfra(ctx context.Context, namespace, kubeconfigPath, fmcImage string, writer io.Writer) error {
	if err := DeployFleetCoreInfra(ctx, namespace, kubeconfigPath, fmcImage, DefaultKubeMCPServerAuthConfig(), DefaultFMCOAuth2Config(), writer); err != nil {
		return err
	}

	// ── Phase 4b: RBAC for Dex OIDC identities (fleet passthrough auth) ──
	// Only relevant to this "fleet" full-pipeline suite: DeployFleetCoreInfra
	// runs kube-mcp-server in kubeconfig mode (Issue #54 RCA), so this
	// group-based RBAC does not gate kube-mcp-server's own K8s calls today,
	// but is retained for OIDC-authenticated identities calling the cluster
	// directly through other fleet paths.
	_, _ = fmt.Fprintln(writer, "\n  🔑 Phase 4b: Creating RBAC for Dex-authenticated fleet identities...")
	if err := applyDexOIDCGroupRBAC(ctx, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("fleet OIDC RBAC creation failed: %w", err)
	}

	// ── Phase 5: Enable fleet scope checking in Gateway ──────────────────
	_, _ = fmt.Fprintln(writer, "\n  🔧 Phase 5: Patching Gateway config with fleet scope checking...")

	gatewayConfigPatch := fmt.Sprintf(`---
apiVersion: v1
kind: ConfigMap
metadata:
  name: gateway-config
  namespace: %[1]s
data:
  config.yaml: |
    server:
      listenAddr: ":8080"
      maxConcurrentRequests: 100
      readTimeout: 30s
      writeTimeout: 30s
      idleTimeout: 120s
    datastorage:
      url: "https://data-storage-service.%[1]s.svc.cluster.local:8080"
      timeout: 10s
      buffer:
        bufferSize: 10000
        batchSize: 100
        flushInterval: 1s
        maxRetries: 3
    processing:
      environment:
        cacheTtl: 5s
        configmapNamespace: "%[1]s"
        configmapName: "kubernaut-environment-overrides"
    fleet:
      enabled: true
      backend: fleetmetadatacache
      mcpGatewayEndpoint: "http://mcp-gateway-istio.gateway-system.svc:8080/mcp"
      mcpGatewayType: kuadrant
`, namespace)

	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, gatewayConfigPatch); err != nil {
		return fmt.Errorf("gateway-config fleet patch failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "    Restarting Gateway to pick up fleet config...")
	if err := runKubectl(ctx, kubeconfigPath, writer,
		"rollout", "restart", "deployment/gateway", "-n", namespace); err != nil {
		return fmt.Errorf("gateway restart failed: %w", err)
	}
	if err := waitForDeployment(ctx, "gateway", namespace, kubeconfigPath, 120*time.Second, writer); err != nil {
		return fmt.Errorf("gateway rollout after fleet config patch failed: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "    ✅ Gateway restarted with fleet scope checking enabled")

	// ── Phase 5b: Enable fleet scope checking in RemediationOrchestrator ─
	// Without this, RO's fleet.NewScopeChecker factory (cmd/remediationorchestrator/main.go)
	// falls back to the plain (non-federated) scope.Manager, which unconditionally
	// rejects any ClusterID-scoped resource with "local Manager cannot resolve
	// remote cluster; use a fleet adapter" — blocking every fleet RR at the
	// CheckUnmanagedResource pre-analysis gate (routing/blocking.go).
	_, _ = fmt.Fprintln(writer, "\n  🔧 Phase 5b: Patching RemediationOrchestrator config with fleet scope checking...")
	if err := patchRemediationOrchestratorConfigForFleet(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("remediationorchestrator-config fleet patch failed: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "    ✅ RemediationOrchestrator restarted with fleet scope checking enabled")

	_, _ = fmt.Fprintln(writer, "✅ Fleet E2E infrastructure deployed (~388 MB)")
	return nil
}

// applyDexOIDCGroupRBAC creates RBAC bindings for Dex-authenticated fleet
// identities (dex:mcp-read, dex:mcp-write groups, matching the
// --oidc-groups-claim/--oidc-groups-prefix flags patchAPIServerForOIDC sets).
// Only used by the "fleet" full-pipeline E2E suite (DeployFleetInfra) --
// extracted out of DeployFleetCoreInfra so the FMC-only lane (which uses
// Keycloak + passthrough/token-exchange, not Dex groups) doesn't apply RBAC
// for an identity shape it never authenticates.
func applyDexOIDCGroupRBAC(ctx context.Context, kubeconfigPath string, writer io.Writer) error {
	oidcRBACManifest := `---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: fleet-mcp-reader
rules:
- apiGroups: ["", "apps"]
  resources: ["pods", "services", "nodes", "deployments", "statefulsets", "daemonsets"]
  verbs: ["get", "list", "watch"]
- apiGroups: [""]
  resources: ["namespaces"]
  verbs: ["get", "list"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: fleet-dex-mcp-read
  labels:
    app: fleet-oidc
    component: fleet
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: fleet-mcp-reader
subjects:
- kind: Group
  name: "dex:mcp-read"
  apiGroup: rbac.authorization.k8s.io
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: fleet-dex-mcp-write
  labels:
    app: fleet-oidc
    component: fleet
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: fleet-mcp-reader
subjects:
- kind: Group
  name: "dex:mcp-write"
  apiGroup: rbac.authorization.k8s.io
`

	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, oidcRBACManifest); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(writer, "    ✅ Fleet OIDC RBAC bindings created (dex:mcp-read, dex:mcp-write)")
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
// This function has no dependency on Gateway or RemediationOrchestrator being
// deployed, unlike DeployFleetInfra's Phase 5/5b -- it is shared between the
// full "fleet" E2E suite (via DeployFleetInfra) and the dedicated
// fleetmetadatacache E2E lane (SetupFMCE2EInfrastructure), which deploys only
// DataStorage + Dex + this core alongside FMC.
//
// authConfig controls how kube-mcp-server authenticates to the target
// Kubernetes API server -- see KubeMCPServerAuthConfig. The "fleet" suite
// passes DefaultKubeMCPServerAuthConfig() (kubeconfig mode); the FMC E2E lane
// passes a passthrough+STS config to validate the real token-exchange wiring.
//
// fmcOAuth2Config controls FMC's own OAuth2 client_credentials IdP endpoint
// -- see FMCOAuth2Config. The "fleet" suite passes DefaultFMCOAuth2Config()
// (DEX); the FMC E2E lane passes Keycloak's token endpoint.
//
// Total memory: ~388 MB (kubeconfig mode) / ~1.7-2.5 GB (passthrough mode,
// Keycloak replacing Dex -- see suite_test.go).
func DeployFleetCoreInfra(ctx context.Context, namespace, kubeconfigPath, fmcImage string, authConfig KubeMCPServerAuthConfig, fmcOAuth2Config FMCOAuth2Config, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "🚀 Deploying Fleet Core E2E Infrastructure...")

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

	// ── Phase 3: Backend MCP Server ─────────────────────────────────────
	_, _ = fmt.Fprintln(writer, "\n  🔌 Phase 3: Deploying kube-mcp-server backend...")

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
`, namespace, KubeMCPServerImage, indentPEM(kubeMCPTOMLConfig, 4), kubeMCPExtraVolumeMount, kubeMCPExtraVolume)

	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, kubeMCPManifest); err != nil {
		return fmt.Errorf("kube-mcp-server deployment failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "    Waiting for kube-mcp-server...")
	if err := waitForDeployment(ctx, "kube-mcp-server", namespace, kubeconfigPath, 120*time.Second, writer); err != nil {
		return fmt.Errorf("kube-mcp-server rollout failed: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "    ✅ kube-mcp-server ready")

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

	routeManifest := fmt.Sprintf(`%[2]s---
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
---
apiVersion: mcp.kuadrant.io/v1alpha1
kind: MCPServerRegistration
metadata:
  name: loopback-cluster
  namespace: %[1]s
  labels:
    kubernaut.ai/managed: "true"
spec:
  prefix: "loopback_cluster_"
%[3]s  targetRef:
    group: gateway.networking.k8s.io
    kind: HTTPRoute
    name: kube-mcp-server-route
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
    name: kube-mcp-server-route
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
    name: kube-mcp-server-route
    namespace: %[1]s
`, namespace, brokerCredSecretManifest, brokerCredRefYAML)

	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, routeManifest); err != nil {
		return fmt.Errorf("httpRoute/MCPServerRegistration creation failed: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "    ✅ MCPServerRegistrations created (loopback-cluster, prod-east, prod-west)")

	// ── Phase 4: FMC Stack (Valkey + FMC) ───────────────────────────────
	_, _ = fmt.Fprintln(writer, "\n  💾 Phase 4: Deploying FMC stack (Valkey + FMC)...")

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
rules:
- apiGroups: ["mcp.kuadrant.io"]
  resources: ["mcpserverregistrations"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["gateway.networking.k8s.io"]
  resources: ["gateways", "httproutes"]
  verbs: ["get", "list", "watch"]
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
      metricsAddr: ":8081"
    mcpGateway:
      endpoint: "http://mcp-gateway-istio.gateway-system.svc:8080/mcp"
      gatewayType: "kuadrant"
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
      serviceAccountName: fleetmetadatacache
      containers:
      - name: fleetmetadatacache
        image: %[2]s
        imagePullPolicy: IfNotPresent
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
        ports:
        - name: api
          containerPort: 8080
        - name: metrics
          containerPort: 8081
        livenessProbe:
          httpGet:
            path: /healthz
            port: api
          initialDelaySeconds: 5
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /healthz
            port: api
          initialDelaySeconds: 3
          periodSeconds: 5
        resources:
          requests:
            memory: "32Mi"
            cpu: "25m"
          limits:
            memory: "64Mi"
            cpu: "100m"
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
  - name: metrics
    port: 8081
    targetPort: metrics
  selector:
    app: fleetmetadatacache
`, namespace, fmcImage, fmcOAuth2Config.TokenURL, fmcOAuth2Config.ClientID, fmcOAuth2Config.ClientSecret, fmcOAuth2ScopesYAML)

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

// patchRemediationOrchestratorConfigForFleet appends a `fleet:` section to RO's
// existing remediationorchestrator.yaml ConfigMap data (owned by the shared
// hybrid E2E infra) so that fleet.NewScopeChecker wraps RO's local scope.Manager
// in a FederatedScopeChecker backed by FMC, instead of leaving ClusterID-scoped
// resources permanently unresolvable (ADR-068).
func patchRemediationOrchestratorConfigForFleet(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	getCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
		"get", "configmap", "remediationorchestrator-config", "-n", namespace,
		"-o", "jsonpath={.data.remediationorchestrator\\.yaml}")
	currentConfig, err := getCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to read existing remediationorchestrator-config: %w", err)
	}

	fleetBlock := fmt.Sprintf(`
fleet:
  enabled: true
  backend: fleetmetadatacache
  endpoint: "http://fleetmetadatacache-service.%s.svc.cluster.local:8080"
`, namespace)
	patchedConfig := string(currentConfig) + fleetBlock

	patchPayload, err := json.Marshal(map[string]interface{}{
		"data": map[string]string{
			"remediationorchestrator.yaml": patchedConfig,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to marshal remediationorchestrator-config patch: %w", err)
	}

	patchCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
		"patch", "configmap", "remediationorchestrator-config", "-n", namespace,
		"--type", "merge", "-p", string(patchPayload))
	patchCmd.Stdout = writer
	patchCmd.Stderr = writer
	if err := patchCmd.Run(); err != nil {
		return fmt.Errorf("failed to patch remediationorchestrator-config: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "    Restarting RemediationOrchestrator to pick up fleet config...")
	if err := runKubectl(ctx, kubeconfigPath, writer,
		"rollout", "restart", "deployment/remediationorchestrator-controller", "-n", namespace); err != nil {
		return fmt.Errorf("remediationorchestrator restart failed: %w", err)
	}
	if err := waitForDeployment(ctx, "remediationorchestrator-controller", namespace, kubeconfigPath, 120*time.Second, writer); err != nil {
		return fmt.Errorf("remediationorchestrator rollout after fleet config patch failed: %w", err)
	}
	return nil
}

// DefaultDexFleetReadTokenFunc returns a token-fetch function using DEX's
// client_credentials grant, matching the "fleet" full-pipeline suite's IdP.
func DefaultDexFleetReadTokenFunc() func() (string, error) {
	tokenCfg := DefaultDexFleetReadConfig()
	tokenCfg.TokenEndpoint = "https://localhost:30556/dex/token"
	return func() (string, error) {
		return GetDexClientCredentialsToken(tokenCfg)
	}
}

// WaitForFleetReady verifies the Kuadrant MCP Gateway is reachable via NodePort
// by performing an MCP initialize handshake, then a real authenticated
// tools/call using tokenFunc (DEX for the "fleet" suite, Keycloak for the FMC
// E2E lane -- see DefaultDexFleetReadTokenFunc and the FMC lane's own
// Keycloak-based token func in fleetmetadatacache_e2e.go).
func WaitForFleetReady(tokenFunc func() (string, error), writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "  ⏳ Verifying MCP Gateway reachability via NodePort...")

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
		req, _ := http.NewRequest("POST", "http://localhost:31975/mcp", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")

		resp, err := client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			_ = resp.Body.Close()
			_, _ = fmt.Fprintln(writer, "  ✅ MCP Gateway reachable (initialize → 200 OK)")
			return waitForAuthenticatedMCPGateway(tokenFunc, writer)
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
		time.Sleep(3 * time.Second)
	}
	return fmt.Errorf("mcp gateway not responsive at http://localhost:31975/mcp after 120 seconds")
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
func waitForAuthenticatedMCPGateway(tokenFunc func() (string, error), writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "  ⏳ Verifying authenticated tools/call succeeds (Kuadrant AuthPolicy convergence)...")

	deadline := time.Now().Add(90 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := probeAuthenticatedResourcesList(tokenFunc); err != nil {
			lastErr = err
			time.Sleep(3 * time.Second)
			continue
		}
		_, _ = fmt.Fprintln(writer, "  ✅ Authenticated tools/call succeeded (Kuadrant AuthPolicy converged)")
		return nil
	}
	return fmt.Errorf("authenticated MCP tools/call did not succeed within 90s (Kuadrant AuthPolicy convergence failure): %w", lastErr)
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
func probeAuthenticatedResourcesList(tokenFunc func() (string, error)) error {
	token, err := tokenFunc()
	if err != nil {
		return fmt.Errorf("acquire IdP token: %w", err)
	}

	authClient := &http.Client{
		Timeout:   10 * time.Second,
		Transport: &bearerTokenTransport{token: token, base: http.DefaultTransport},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	mcpConn, err := mcpclient.New(ctx, "http://localhost:31975/mcp",
		mcpclient.WithToolPrefix("loopback_cluster_"),
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

// patchAPIServerForOIDC adds OIDC flags to the API server static pod manifest
// on the Kind node, enabling the K8s API server to accept Dex-issued JWTs.
// Thin Dex-flavored wrapper around patchAPIServerForOIDCConfig for the
// "fleet" full-pipeline E2E suite; kept so existing callers don't need to
// change. The FMC E2E lane calls patchAPIServerForOIDCConfig directly with
// Keycloak's issuer/audience (Spike S17/S18).
func patchAPIServerForOIDC(ctx context.Context, clusterName, kubeconfigPath string, writer io.Writer) error {
	return patchAPIServerForOIDCConfig(ctx, clusterName, kubeconfigPath, OIDCPatchConfig{
		IssuerURL:      "https://dex:5556/dex",
		ClientID:       "kubernaut-fleet-read",
		UsernameClaim:  "sub",
		UsernamePrefix: "dex:",
		GroupsClaim:    "groups",
		GroupsPrefix:   "dex:",
	}, writer)
}

// patchAPIServerForOIDCConfig adds OIDC flags to the API server static pod
// manifest on the Kind node per the given OIDCPatchConfig, enabling the K8s
// API server to accept JWTs from the configured issuer.
//
// This must be called AFTER the issuer (Dex or Keycloak) is deployed and
// running (the API server performs OIDC discovery on restart, requiring the
// issuer to be reachable). The kubelet detects the manifest change and
// automatically restarts the API server pod.
func patchAPIServerForOIDCConfig(ctx context.Context, clusterName, kubeconfigPath string, cfg OIDCPatchConfig, writer io.Writer) error {
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
	if err := patchAPIServerPodHostsForIssuer(ctx, nodeName, kubeconfigPath, cfg.IssuerURL, writer); err != nil {
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
// its in-cluster Service ClusterIP and appends a static entry to the
// kube-apiserver static pod's kubelet-managed /etc/hosts file, so the API
// server's OIDC discovery/JWKS HTTP client can resolve the issuer without
// relying on CoreDNS (unreachable from the hostNetwork static pod -- see the
// call site's doc comment). No-op if the issuer URL has no hostname (e.g.
// malformed) or the entry already exists.
func patchAPIServerPodHostsForIssuer(ctx context.Context, nodeName, kubeconfigPath, issuerURL string, writer io.Writer) error {
	u, err := url.Parse(issuerURL)
	if err != nil || u.Hostname() == "" {
		return fmt.Errorf("failed to parse issuer hostname from %q: %w", issuerURL, err)
	}
	host := u.Hostname()

	svcIPCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
		"get", "svc", host, "-n", "kubernaut-system", "-o", "jsonpath={.spec.clusterIP}")
	svcIPOut, err := svcIPCmd.Output()
	clusterIP := strings.TrimSpace(string(svcIPOut))
	if err != nil || clusterIP == "" {
		return fmt.Errorf("failed to resolve ClusterIP for issuer service %q: %w", host, err)
	}

	// The manifest edit can trigger more than one kubelet restart cycle
	// (each of the several `sed -i` inserts is its own file-write event), so
	// the API server's pod UID -- and therefore its kubelet-managed hosts
	// file path -- can still be changing for a few seconds after Phase 1
	// observes OIDC flags in the running process. Poll for a UID whose
	// hosts file actually exists on the node rather than resolving once,
	// to avoid a "no such file" race against an already-superseded pod.
	deadline := time.Now().Add(30 * time.Second)
	var hostsPath string
	for time.Now().Before(deadline) {
		// NOTE: .metadata.uid is the *mirror pod's* API-server-assigned UID,
		// which does NOT match the on-disk /var/lib/kubelet/pods/<id>
		// directory for static pods. kubelet keys that directory by the
		// static manifest's config hash, exposed as the
		// kubernetes.io/config.hash annotation (equal to config.mirror).
		// Confirmed by live inspection: metadata.uid pointed at a
		// nonexistent directory while config.hash matched exactly.
		uidCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
			"get", "pod", "kube-apiserver-"+nodeName, "-n", "kube-system",
			"-o", `jsonpath={.metadata.annotations.kubernetes\.io/config\.hash}`)
		uidOut, uidErr := uidCmd.Output()
		podUID := strings.TrimSpace(string(uidOut))
		if uidErr == nil && podUID != "" {
			candidate := fmt.Sprintf("/var/lib/kubelet/pods/%s/etc-hosts", podUID)
			checkCmd := exec.CommandContext(ctx, "podman", "exec", nodeName, "test", "-f", candidate)
			if checkCmd.Run() == nil {
				hostsPath = candidate
				break
			}
		}
		time.Sleep(2 * time.Second)
	}
	if hostsPath == "" {
		return fmt.Errorf("kube-apiserver pod hosts file did not stabilize within 30s")
	}

	patchCmd := fmt.Sprintf(
		`grep -q ' %s$' %s || echo '%s %s' >> %s`,
		host, hostsPath, clusterIP, host, hostsPath)
	cmd := exec.CommandContext(ctx, "podman", "exec", nodeName, "sh", "-c", patchCmd)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to append %q -> %q entry to %s: %w", host, clusterIP, hostsPath, err)
	}
	_, _ = fmt.Fprintf(writer, "  Patched API server pod hosts file: %s -> %s (%s)\n", host, clusterIP, hostsPath)
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

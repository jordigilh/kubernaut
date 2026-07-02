//go:build ignore

/*
SPIKE S19 -- FMC E2E second Kind cluster (cross-cluster isolation bridge).

Validated end-to-end on 2026-07-02. See README.md in this directory for
the goal, architecture, and findings.

This file is kept for reference / future revision, NOT for standalone
compilation: it depends on unexported helpers in package `infrastructure`
(patchAPIServerForOIDCConfig, KubeMCPServerAuthConfig, kubectlApplyManifest,
CreateKindClusterWithConfig, waitForDeployment, InterServiceCAPath,
TLSCAVolumeYAML/MountYAML, applyExchangedIdentityRBAC,
probeAuthenticatedResourcesList, indentPEM, KubeMCPServerImage,
KubeMCPServerAuthModePassthrough). The `//go:build ignore` tag above keeps
`go build ./...` / `go vet ./...` / golangci-lint from picking this up in
its current location.

To re-run: copy this file (and kind-spike-remote-config.yaml, adjusting its
ConfigPath below to test/infrastructure/kind-spike-remote-config.yaml) into
test/infrastructure/ as a `_test.go` file with `package infrastructure`,
remove the `//go:build ignore` line, then:

  RUN_SPIKE=true go test -run TestSpikeA_PrimarySetup -v -timeout 20m ./test/infrastructure/...
  RUN_SPIKE=true go test -run TestSpikeB_RemoteClusterAndKeycloakBridge -v -timeout 10m ./test/infrastructure/...
  RUN_SPIKE=true go test -run TestSpikeC_KubeMCPOnRemote -v -timeout 10m ./test/infrastructure/...
  RUN_SPIKE=true go test -run TestSpikeD_BridgeBackIntoPrimary -v -timeout 5m ./test/infrastructure/...

Cleanup:
  kind delete cluster --name spike-primary
  kind delete cluster --name spike-remote
  rm -rf /tmp/kubernaut-spike
*/

package infrastructure

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
)

const (
	spikeDir            = "/tmp/kubernaut-spike"
	spikePrimaryCluster = "spike-primary"
	spikeRemoteCluster  = "spike-remote"
	spikeNamespace      = "kubernaut-system"
)

func spikePrimaryKubeconfig() string { return filepath.Join(spikeDir, "primary-kubeconfig.yaml") }
func spikeRemoteKubeconfig() string  { return filepath.Join(spikeDir, "remote-kubeconfig.yaml") }

func skipUnlessSpike(t *testing.T) {
	if os.Getenv("RUN_SPIKE") != "true" {
		t.Skip("set RUN_SPIKE=true to run this throwaway spike")
	}
}

// TestSpikeA_PrimarySetup stands up the primary FMC cluster exactly as the
// real Kuadrant lane does today (unmodified SetupFMCE2EInfrastructure).
func TestSpikeA_PrimarySetup(t *testing.T) {
	skipUnlessSpike(t)
	require(t, os.MkdirAll(spikeDir, 0o755))

	ctx := context.Background()
	_, err := SetupFMCE2EInfrastructure(ctx, spikePrimaryCluster, spikePrimaryKubeconfig(), os.Stdout)
	if err != nil {
		t.Fatalf("primary cluster setup failed: %v", err)
	}
	t.Log("✅ Phase A done: primary cluster fully up (Keycloak + Kuadrant + kube-mcp-server + FMC)")
}

// TestSpikeB_RemoteClusterAndKeycloakBridge creates the second Kind cluster,
// bridges its "keycloak" hostname back to the primary cluster's real
// Keycloak over the shared podman bridge network, and OIDC-patches the
// remote cluster's own API server against it. This is the highest-risk,
// never-before-tested part of the design.
//
// NOTE (finding from this spike): the API server's readyz going stable here
// does NOT by itself prove the bridge works -- kube-apiserver validates the
// OIDC flags syntactically at startup but only does an EAGER connectivity
// check to the issuer lazily, on first real token validation. Phase C is
// what actually proves this bridge is live end-to-end.
func TestSpikeB_RemoteClusterAndKeycloakBridge(t *testing.T) {
	skipUnlessSpike(t)
	ctx := context.Background()
	w := os.Stdout

	t.Log("── creating remote Kind cluster ──")
	opts := KindClusterOptions{
		ClusterName:             spikeRemoteCluster,
		KubeconfigPath:          spikeRemoteKubeconfig(),
		ConfigPath:              "test/infrastructure/kind-spike-remote-config.yaml",
		WaitTimeout:             "5m",
		UsePodman:               true,
		ProjectRootAsWorkingDir: true,
	}
	if err := CreateKindClusterWithConfig(opts, w); err != nil {
		t.Fatalf("remote cluster creation failed: %v", err)
	}

	if err := createTestNamespace(spikeNamespace, spikeRemoteKubeconfig(), w); err != nil {
		t.Fatalf("remote namespace creation failed: %v", err)
	}

	t.Log("── discovering primary cluster's node bridge IP ──")
	primaryIP, err := spikeNodeBridgeIP(spikePrimaryCluster + "-control-plane")
	if err != nil {
		t.Fatalf("failed to discover primary node bridge IP: %v", err)
	}
	t.Logf("primary node bridge IP: %s", primaryIP)

	t.Log("── bridging 'keycloak' Service in remote cluster -> primary Keycloak NodePort ──")
	// Service port MUST be 8443 (matching the hardcoded "https://keycloak:8443/..."
	// issuer/authorization URLs) -- NOT the NodePort number. Bug found during
	// this spike: an 8080 Service port caused "connection refused" (DNS
	// resolved fine, but nothing listens on ClusterIP:8443).
	if err := spikeCreateServiceBridge(ctx, spikeRemoteKubeconfig(), spikeNamespace, "keycloak", 8443, primaryIP, 30557, w); err != nil {
		t.Fatalf("keycloak bridge Service creation failed: %v", err)
	}

	t.Log("── copying primary's inter-service CA to remote (local file + ConfigMap) ──")
	if err := spikeCopyInterServiceCA(ctx, spikePrimaryKubeconfig(), spikeRemoteKubeconfig(), spikeNamespace, w); err != nil {
		t.Fatalf("CA copy failed: %v", err)
	}

	t.Log("── OIDC-patching remote cluster's API server against the bridged Keycloak ──")
	oidcCfg := OIDCPatchConfig{
		IssuerURL:      "https://keycloak:8443/realms/kubernaut-fleet",
		ClientID:       "k8s-api",
		UsernameClaim:  "preferred_username",
		UsernamePrefix: "keycloak:",
	}
	if err := patchAPIServerForOIDCConfig(ctx, spikeRemoteCluster, spikeRemoteKubeconfig(), oidcCfg, w); err != nil {
		t.Fatalf("remote API server OIDC patch failed: %v", err)
	}

	t.Log("✅ Phase B done: remote cluster's own API server trusts primary's real Keycloak over the bridge")
}

// TestSpikeC_KubeMCPOnRemote deploys a second, independent kube-mcp-server
// into the remote cluster and proves it can authenticate an incoming
// Keycloak-issued token, exchange it via RFC 8693, and successfully read
// real Pods from the REMOTE cluster's own (now OIDC-trusting) API server.
//
// RESULT: passed on first successful run (after the Phase B port fix),
// confirming applyExchangedIdentityRBAC IS required on the remote cluster
// too (kube-mcp-server's own ServiceAccount RBAC is vestigial in
// passthrough mode -- only the exchanged identity's RBAC matters).
func TestSpikeC_KubeMCPOnRemote(t *testing.T) {
	skipUnlessSpike(t)
	ctx := context.Background()
	w := os.Stdout

	authConfig := KubeMCPServerAuthConfig{
		Mode:             KubeMCPServerAuthModePassthrough,
		GatewayType:      registry.GatewayKuadrant,
		RequireOAuth:     true,
		AuthorizationURL: "https://keycloak:8443/realms/kubernaut-fleet",
		OAuthAudience:    "kube-mcp-server",
		StsClientID:      "kube-mcp-server",
		StsClientSecret:  "e2e-kube-mcp-server-secret",
		StsAudience:      "k8s-api",
		StsScopes:        []string{"k8s-api-audience"},
		CAFilePath:       "/etc/tls-ca/ca.crt",
	}

	t.Log("── deploying kube-mcp-server-2 into remote cluster ──")
	if err := spikeDeployKubeMCPServer(ctx, spikeNamespace, spikeRemoteKubeconfig(), authConfig, w); err != nil {
		t.Fatalf("kube-mcp-server-2 deployment failed: %v", err)
	}

	t.Log("── applying exchanged-identity RBAC in remote cluster ──")
	if err := applyExchangedIdentityRBAC(ctx, spikeRemoteKubeconfig(), w); err != nil {
		t.Fatalf("exchanged-identity RBAC failed: %v", err)
	}

	t.Log("── exposing kube-mcp-server-2 via NodePort 30180 ──")
	npManifest := fmt.Sprintf(`---
apiVersion: v1
kind: Service
metadata:
  name: kube-mcp-server-nodeport
  namespace: %s
spec:
  type: NodePort
  selector:
    app: kube-mcp-server
  ports:
  - port: 8080
    targetPort: 8080
    nodePort: 30180
`, spikeNamespace)
	if err := kubectlApplyManifest(ctx, spikeRemoteKubeconfig(), w, npManifest); err != nil {
		t.Fatalf("NodePort expose failed: %v", err)
	}

	t.Log("── authenticated tools/call against kube-mcp-server-2 (proves Keycloak bridge + STS + remote API server trust all work) ──")
	tokenFunc := func() (string, error) {
		return GetKeycloakClientCredentialsToken(KeycloakFleetTokenConfig{
			TokenEndpoint: "https://localhost:30557/realms/kubernaut-fleet/protocol/openid-connect/token",
			ClientID:      "kubernaut-fleet-read",
			ClientSecret:  "e2e-fleet-secret",
			Scopes:        []string{"kube-mcp-server-audience"},
		})
	}

	deadline := time.Now().Add(90 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := probeAuthenticatedResourcesList(tokenFunc, "http://localhost:8180/mcp", ""); err != nil {
			lastErr = err
			time.Sleep(3 * time.Second)
			continue
		}
		t.Log("✅ Phase C done: kube-mcp-server-2 authenticated + exchanged + read real Pods from the REMOTE cluster's own API server")
		return
	}
	t.Fatalf("authenticated tools/call against kube-mcp-server-2 never succeeded: %v", lastErr)
}

// TestSpikeD_BridgeBackIntoPrimary proves the reverse direction: a bridge
// Service created in the PRIMARY cluster, pointing at the remote cluster's
// kube-mcp-server NodePort, is reachable via pure in-cluster Service DNS --
// exactly how a Kuadrant HTTPRoute / EAIGW Backend would reach it for real.
//
// RESULT: passed, confirmed via a manual verbose curl too (HTTP/1.1 200 OK
// from kube-mcp-server-remote.kubernaut-system.svc.cluster.local:8080/healthz).
func TestSpikeD_BridgeBackIntoPrimary(t *testing.T) {
	skipUnlessSpike(t)
	ctx := context.Background()
	w := os.Stdout

	t.Log("── discovering remote cluster's node bridge IP ──")
	remoteIP, err := spikeNodeBridgeIP(spikeRemoteCluster + "-control-plane")
	if err != nil {
		t.Fatalf("failed to discover remote node bridge IP: %v", err)
	}
	t.Logf("remote node bridge IP: %s", remoteIP)

	t.Log("── bridging 'kube-mcp-server-remote' Service in primary cluster -> remote kube-mcp-server NodePort ──")
	if err := spikeCreateServiceBridge(ctx, spikePrimaryKubeconfig(), spikeNamespace, "kube-mcp-server-remote", 8080, remoteIP, 30180, w); err != nil {
		t.Fatalf("reverse bridge Service creation failed: %v", err)
	}

	t.Log("── proving in-cluster reachability from a throwaway pod in the primary cluster ──")
	curlCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", spikePrimaryKubeconfig(),
		"run", "spike-curl", "--rm", "-i", "--restart=Never", "--image=curlimages/curl",
		"--namespace", spikeNamespace, "--command", "--",
		"curl", "-sS", "-m", "5", "http://kube-mcp-server-remote.kubernaut-system.svc.cluster.local:8080/healthz")
	out, err := curlCmd.CombinedOutput()
	t.Logf("curl output: %s", string(out))
	if err != nil {
		t.Fatalf("in-cluster bridge reachability check failed: %v", err)
	}
	t.Log("✅ Phase D done: primary cluster reaches remote cluster's kube-mcp-server via a pure Service+Endpoints bridge")
}

// ── spike-only helpers (throwaway; NOT the real implementation) ──────────

func spikeNodeBridgeIP(nodeName string) (string, error) {
	cmd := exec.Command("podman", "inspect", nodeName,
		"--format", "{{.NetworkSettings.Networks.kind.IPAddress}}")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("podman inspect %s failed: %w (%s)", nodeName, err, string(out))
	}
	ip := strings.TrimSpace(string(out))
	if ip == "" {
		return "", fmt.Errorf("empty bridge IP for node %s (is it on the 'kind' podman network?)", nodeName)
	}
	return ip, nil
}

// servicePort is what in-cluster DNS clients dial (must match whatever
// hostname:port is hardcoded elsewhere, e.g. "keycloak:8443"); remotePort is
// the actual NodePort listening on remoteIP.
func spikeCreateServiceBridge(ctx context.Context, kubeconfigPath, namespace, serviceName string, servicePort int, remoteIP string, remotePort int, w *os.File) error {
	manifest := fmt.Sprintf(`---
apiVersion: v1
kind: Service
metadata:
  name: %[1]s
  namespace: %[2]s
spec:
  ports:
  - port: %[4]d
    targetPort: %[5]d
---
apiVersion: v1
kind: Endpoints
metadata:
  name: %[1]s
  namespace: %[2]s
subsets:
- addresses:
  - ip: %[3]s
  ports:
  - port: %[5]d
`, serviceName, namespace, remoteIP, servicePort, remotePort)
	return kubectlApplyManifest(ctx, kubeconfigPath, w, manifest)
}

func spikeCopyInterServiceCA(ctx context.Context, primaryKubeconfig, remoteKubeconfig, namespace string, w *os.File) error {
	caBytes, err := os.ReadFile(InterServiceCAPath(primaryKubeconfig))
	if err != nil {
		return fmt.Errorf("read primary CA: %w", err)
	}
	if err := os.WriteFile(InterServiceCAPath(remoteKubeconfig), caBytes, 0o600); err != nil {
		return fmt.Errorf("write CA next to remote kubeconfig: %w", err)
	}
	cmName := "inter-service-ca"
	cmManifest := fmt.Sprintf(`---
apiVersion: v1
kind: ConfigMap
metadata:
  name: %s
  namespace: %s
data:
  ca.crt: |
%s
`, cmName, namespace, indentPEM(string(caBytes), 4))
	return kubectlApplyManifest(ctx, remoteKubeconfig, w, cmManifest)
}

// spikeDeployKubeMCPServer duplicates (does not refactor) the kube-mcp-server
// manifest from deployKubeMCPServerAndRegister -- for the spike only. The
// real implementation extracts a shared deployKubeMCPServer function instead
// of duplicating this (see the FMC E2E second-cluster plan).
func spikeDeployKubeMCPServer(ctx context.Context, namespace, kubeconfigPath string, authConfig KubeMCPServerAuthConfig, w *os.File) error {
	manifest := fmt.Sprintf(`---
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
spec:
  replicas: 1
  selector:
    matchLabels:
      app: kube-mcp-server
  template:
    metadata:
      labels:
        app: kube-mcp-server
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
`, namespace, KubeMCPServerImage, indentPEM(authConfig.tomlString(), 4), TLSCAVolumeMountYAML(8), TLSCAVolumeYAML(6))

	if err := kubectlApplyManifest(ctx, kubeconfigPath, w, manifest); err != nil {
		return fmt.Errorf("kube-mcp-server-2 deployment failed: %w", err)
	}
	return waitForDeployment(ctx, "kube-mcp-server", namespace, kubeconfigPath, 120*time.Second, w)
}

func require(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("setup precondition failed: %v", err)
	}
}

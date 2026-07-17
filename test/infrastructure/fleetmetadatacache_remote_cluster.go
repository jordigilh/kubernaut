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
)

// remoteKubeMCPServerNodePort is the fixed NodePort exposing the remote
// cluster's kube-mcp-server. Safe to hardcode: this cluster has no host
// port mapping (see kind-fleetmetadatacache-remote-config.yaml) and is
// reached only via KindNodeBridgeIP from the primary cluster, so there is
// no cross-lane collision risk even when both FMC E2E lanes run in
// parallel CI jobs (each pair of clusters is fully isolated).
const remoteKubeMCPServerNodePort = 30180

// remoteBridgeServiceName is the Service name created in the PRIMARY
// cluster to bridge to the remote cluster's kube-mcp-server, backing the
// "prod-east" registration (DD-TEST-013).
const remoteBridgeServiceName = "kube-mcp-server-remote"

// SetupRemoteClusterForFMC provisions a second, independent Kind cluster
// and wires it into the FMC E2E lane's cross-cluster bridge (DD-TEST-013,
// validated in Spike S19): a genuinely separate Kubernetes control plane
// backing the "prod-east" registration, instead of the loopback pattern
// every other registered cluster uses.
//
// Must be called AFTER Keycloak is deployed and reachable in the primary
// cluster (Keycloak must already be running for the remote cluster's own
// API server OIDC patch to succeed) and AFTER the primary cluster's node
// exists (its bridge IP is discovered here). authConfig must be the same
// passthrough+STS config used for the primary cluster's kube-mcp-server --
// this function deploys an identical kube-mcp-server into the remote
// cluster so both perform the same RFC 8693 exchange against the same
// bridged Keycloak.
//
// Returns a RemoteClusterBridgeConfig for the caller to set on
// KubeMCPServerAuthConfig.RemoteBridge before calling DeployFleetCoreInfra
// for the primary cluster.
func SetupRemoteClusterForFMC(ctx context.Context, primaryClusterName, primaryKubeconfigPath, remoteClusterName, remoteKubeconfigPath, namespace string, keycloakIssuerURL string, keycloakNodePort int, authConfig KubeMCPServerAuthConfig, writer io.Writer) (*RemoteClusterBridgeConfig, error) {
	_, _ = fmt.Fprintln(writer, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "🌍 Provisioning remote cluster for cross-cluster isolation (DD-TEST-013)...")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	opts := KindClusterOptions{
		ClusterName:             remoteClusterName,
		KubeconfigPath:          remoteKubeconfigPath,
		ConfigPath:              "test/infrastructure/kind-fleetmetadatacache-remote-config.yaml",
		WaitTimeout:             "5m",
		UsePodman:               true,
		ProjectRootAsWorkingDir: true,
	}
	if err := CreateKindClusterWithConfig(ctx, opts, writer); err != nil {
		return nil, fmt.Errorf("remote cluster creation failed: %w", err)
	}

	if err := createTestNamespace(namespace, remoteKubeconfigPath, writer); err != nil {
		return nil, fmt.Errorf("remote namespace creation failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "  Discovering primary cluster's node bridge IP...")
	primaryIP, err := KindNodeBridgeIP(ctx, primaryClusterName + "-control-plane")
	if err != nil {
		return nil, fmt.Errorf("failed to discover primary node bridge IP: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "  Bridging 'keycloak' Service in remote cluster -> primary Keycloak NodePort...")
	// Service port MUST match the port embedded in keycloakIssuerURL
	// (currently 8443, the port kube-apiserver's OIDC discovery dials) --
	// NOT the NodePort number. A mismatch here reproduces the exact
	// "connection refused" bug found in Spike S19.
	if err := CreateServiceBridge(ctx, remoteKubeconfigPath, namespace, "keycloak", 8443, primaryIP, keycloakNodePort, writer); err != nil {
		return nil, fmt.Errorf("keycloak bridge Service creation failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "  Copying primary's inter-service CA to remote cluster...")
	if err := copyInterServiceCAToRemoteCluster(primaryKubeconfigPath, remoteKubeconfigPath); err != nil {
		return nil, fmt.Errorf("CA copy to remote cluster failed: %w", err)
	}

	// deployKubeMCPServer mounts the "inter-service-ca" ConfigMap as the
	// tls-ca volume (TLSCAVolumeYAML); it must exist in-cluster before that
	// deployment is applied, not just on-disk next to the kubeconfig.
	_, _ = fmt.Fprintln(writer, "  Replicating inter-service-ca ConfigMap into remote cluster...")
	if err := ReplicateInterServiceCAConfigMap(ctx, remoteKubeconfigPath, namespace, writer); err != nil {
		return nil, fmt.Errorf("CA ConfigMap replication to remote cluster failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "  OIDC-patching remote cluster's API server against the bridged Keycloak...")
	oidcCfg := OIDCPatchConfig{
		IssuerURL:      keycloakIssuerURL,
		ClientID:       "k8s-api",
		UsernameClaim:  "preferred_username",
		UsernamePrefix: "keycloak:",
	}
	if err := patchAPIServerForOIDCConfig(ctx, remoteClusterName, remoteKubeconfigPath, oidcCfg, writer); err != nil {
		return nil, fmt.Errorf("remote API server OIDC patching failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "  Deploying kube-mcp-server into remote cluster...")
	if err := deployKubeMCPServer(ctx, namespace, remoteKubeconfigPath, authConfig, writer); err != nil {
		return nil, fmt.Errorf("remote kube-mcp-server deployment failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "  Applying exchanged-identity RBAC in remote cluster...")
	if err := applyExchangedIdentityRBAC(ctx, remoteKubeconfigPath, writer); err != nil {
		return nil, fmt.Errorf("remote exchanged-identity RBAC failed: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "  Exposing remote kube-mcp-server via NodePort %d...\n", remoteKubeMCPServerNodePort)
	npManifest := fmt.Sprintf(`---
apiVersion: v1
kind: Service
metadata:
  name: kube-mcp-server-nodeport
  namespace: %[1]s
spec:
  type: NodePort
  selector:
    app: kube-mcp-server
  ports:
  - port: 8080
    targetPort: 8080
    nodePort: %[2]d
`, namespace, remoteKubeMCPServerNodePort)
	if err := kubectlApplyManifest(ctx, remoteKubeconfigPath, writer, npManifest); err != nil {
		return nil, fmt.Errorf("remote kube-mcp-server NodePort expose failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "  Discovering remote cluster's node bridge IP...")
	remoteIP, err := KindNodeBridgeIP(ctx, remoteClusterName + "-control-plane")
	if err != nil {
		return nil, fmt.Errorf("failed to discover remote node bridge IP: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "✅ Remote cluster ready for cross-cluster bridging")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	return &RemoteClusterBridgeConfig{
		BridgeServiceName: remoteBridgeServiceName,
		BridgeServicePort: 8080,
		RemoteNodeIP:      remoteIP,
		RemoteNodePort:    remoteKubeMCPServerNodePort,
	}, nil
}

// copyInterServiceCAToRemoteCluster copies the primary cluster's
// inter-service CA bytes to the path InterServiceCAPath expects next to the
// remote cluster's kubeconfig, so patchAPIServerForOIDCConfig (called
// unmodified against the remote cluster) trusts the bridged Keycloak's TLS
// certificate -- both are signed by the same CA (interservice_tls.go).
//
// primaryKubeconfigPath and remoteKubeconfigPath must differ (each cluster
// has its own kubeconfig file); passing the same path for both is a caller
// error since it would make the copy a no-op self-read.
func copyInterServiceCAToRemoteCluster(primaryKubeconfigPath, remoteKubeconfigPath string) error {
	caPEM, err := os.ReadFile(InterServiceCAPath(primaryKubeconfigPath))
	if err != nil {
		return fmt.Errorf("read primary inter-service CA: %w", err)
	}
	if err := os.WriteFile(InterServiceCAPath(remoteKubeconfigPath), caPEM, 0o600); err != nil {
		return fmt.Errorf("write inter-service CA next to remote kubeconfig: %w", err)
	}
	return nil
}

// TeardownRemoteClusterForFMC deletes the remote cluster created by
// SetupRemoteClusterForFMC. Mirrors infrastructure.DeleteCluster's
// preserve-on-failure semantics so both clusters are kept together for
// debugging when preserveCluster/anyFailure applies.
func TeardownRemoteClusterForFMC(remoteClusterName, remoteKubeconfigPath, serviceName string, testsFailed bool, writer io.Writer) error {
	if delErr := DeleteCluster(remoteClusterName, serviceName, testsFailed, writer); delErr != nil {
		return fmt.Errorf("remote cluster deletion failed: %w", delErr)
	}
	defaultConfig := os.ExpandEnv("$HOME/.kube/config")
	if remoteKubeconfigPath != "" && remoteKubeconfigPath != defaultConfig {
		_ = os.Remove(remoteKubeconfigPath)
		_ = os.Remove(InterServiceCAPath(remoteKubeconfigPath))
	}
	return nil
}

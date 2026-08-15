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
	"os/exec"
	"strings"
)

// KindNodeBridgeIP returns the given Kind node's IP address on the podman
// "kind" bridge network. Two Kind clusters created by the same podman
// daemon share this network, so any node's IP is directly routable from any
// other node's containers -- no host port mapping required. This is the
// foundation of the FMC E2E second-cluster cross-cluster bridge (DD-TEST-013,
// validated empirically in Spike S19).
//
// nodeName is conventionally "<clusterName>-control-plane" for a
// single-node Kind cluster (the only topology this project's E2E lanes use).
func KindNodeBridgeIP(ctx context.Context, nodeName string) (string, error) {
	cmd := exec.CommandContext(ctx, "podman", "inspect", nodeName,
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

// KindBridgeGatewayIP returns the podman "kind" bridge network's gateway
// address -- the host machine's own IP on that bridge, reachable from any
// container/pod attached to the network (e.g. "10.89.1.1"). Empirically
// validated on a real Linux Podman host (helios08, matching CI's
// KIND_EXPERIMENTAL_PROVIDER=podman environment): a host-bound listener is
// reachable from inside a Kind pod via this exact address (#1985 spike,
// 2026-08-14). This is the correct, portable target for host-side fault
// injection (e.g. InterruptibleProxy bound via NewInterruptibleProxyOn) --
// NOT "host.containers.internal", which is a Podman-Machine/macOS-only DNS
// alias that does not resolve on plain Linux Podman and is, in any case,
// structurally unreachable from macOS's gvproxy backend for arbitrary
// guest-to-host connections (see the E2E test's runtime.GOOS=="darwin"
// skip for that permanent limitation).
func KindBridgeGatewayIP(ctx context.Context, networkName string) (string, error) {
	cmd := exec.CommandContext(ctx, "podman", "network", "inspect", networkName,
		"--format", "{{(index .Subnets 0).Gateway}}")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("podman network inspect %s failed: %w (%s)", networkName, err, string(out))
	}
	ip := strings.TrimSpace(string(out))
	if ip == "" {
		return "", fmt.Errorf("empty gateway IP for podman network %s", networkName)
	}
	return ip, nil
}

// CreateServiceBridge creates a Service+Endpoints pair in the cluster
// identified by kubeconfigPath that makes serviceName resolvable via normal
// in-cluster DNS (e.g. "keycloak", "kube-mcp-server-remote"), routing
// traffic to a NodePort listening on a peer cluster's node bridge IP
// (see KindNodeBridgeIP). No selector is set on the Service -- Endpoints are
// hand-authored, exactly like an ExternalName-style bridge but resolvable by
// plain ClusterIP (some callers, e.g. kube-apiserver's OIDC discovery, do
// not honor ExternalName DNS CNAMEs).
//
// servicePort is what in-cluster clients dial (must match whatever
// hostname:port is hardcoded elsewhere, e.g. the OIDC issuer URL
// "https://keycloak:8443/..."); remotePort is the actual NodePort listening
// on remoteIP. These are frequently different numbers -- Spike S19 found a
// bridge created with servicePort equal to the NodePort value caused
// "connection refused" because in-cluster clients dial the well-known port,
// not the peer's NodePort number.
func CreateServiceBridge(ctx context.Context, kubeconfigPath, namespace, serviceName string, servicePort int, remoteIP string, remotePort int, writer io.Writer) error {
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
	return kubectlApplyManifest(ctx, kubeconfigPath, writer, manifest)
}

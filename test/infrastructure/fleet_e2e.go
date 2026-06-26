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
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// KubeMCPServerImage is the Go-native K8s MCP server image.
// v0.0.63: supports HTTP mode, in-cluster auth, core toolsets.
const KubeMCPServerImage = "ghcr.io/containers/kubernetes-mcp-server:latest"

// SetupFleetE2EInfrastructure deploys the complete fleet E2E stack:
// all fullpipeline services + EAIGW + K8s MCP Server.
//
// It composes on the fullpipeline setup (which already deploys GW, SP, RO, WE,
// AA, EM, KA, AF, DS, DEX, Prometheus, AlertManager, etc.) and adds the fleet-
// specific infrastructure on top. The Kind cluster config must include the fleet
// NodePort mappings (31975 for EAIGW MCP, 31064 for EAIGW health) -- these are
// already present in kind-fullpipeline-config.yaml.
//
// The loopback pattern is used: the K8s MCP Server connects to the same cluster
// where it runs, but kubernaut treats it as a remote cluster with clusterID
// "loopback-cluster". This validates the full remote code path without needing
// a second Kind cluster.
//
// Total additional memory over fullpipeline: ~66 MB (EAIGW 50 MB + K8s MCP 16 MB).
//
// Authority: Issue #54, ADR-068
func SetupFleetE2EInfrastructure(ctx context.Context, clusterName, kubeconfigPath string, writer io.Writer) (builtImages map[string]string, seededUUIDs map[string]string, afRemediateNS map[string]string, err error) {
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "🚀 Fleet E2E Infrastructure (Issue #54)")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "  Base: Full Pipeline (all services)")
	_, _ = fmt.Fprintln(writer, "  Fleet: EAIGW + K8s MCP Server (loopback pattern)")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	builtImages, seededUUIDs, afRemediateNS, err = SetupFullPipelineInfrastructure(ctx, clusterName, kubeconfigPath, writer)
	if err != nil {
		return builtImages, seededUUIDs, afRemediateNS, fmt.Errorf("fullpipeline base setup failed: %w", err)
	}

	namespace := "kubernaut-system"

	_, _ = fmt.Fprintln(writer, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "🌐 FLEET PHASE: Deploying MCP Gateway infrastructure...")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	_, _ = fmt.Fprintln(writer, "  📦 Pre-loading fleet external images...")
	if loadErr := PreloadExternalImage(KubeMCPServerImage, clusterName, writer); loadErr != nil {
		_, _ = fmt.Fprintf(writer, "  ⚠️  K8s MCP Server image preload failed (will pull on-demand): %v\n", loadErr)
	}
	if loadErr := PreloadExternalImage(EAIGWImage, clusterName, writer); loadErr != nil {
		_, _ = fmt.Fprintf(writer, "  ⚠️  EAIGW image preload failed (will pull on-demand): %v\n", loadErr)
	}

	if deployErr := DeployFleetInfra(ctx, namespace, kubeconfigPath, writer); deployErr != nil {
		return builtImages, seededUUIDs, afRemediateNS, fmt.Errorf("fleet infra deployment failed: %w", deployErr)
	}

	if readyErr := WaitForFleetReady(writer); readyErr != nil {
		return builtImages, seededUUIDs, afRemediateNS, fmt.Errorf("fleet readiness check failed: %w", readyErr)
	}

	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "✅ Fleet E2E Infrastructure READY")
	_, _ = fmt.Fprintln(writer, "  EAIGW MCP:    http://localhost:31975")
	_, _ = fmt.Fprintln(writer, "  EAIGW Health: http://localhost:31064")
	_, _ = fmt.Fprintln(writer, "  Loopback cluster ID: loopback-cluster")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	return builtImages, seededUUIDs, afRemediateNS, nil
}

// DeployFleetInfra deploys the fleet E2E infrastructure in the Kind cluster:
// 1. K8s MCP Server (in-cluster, ServiceAccount-based auth)
// 2. EAIGW (Envoy AI Gateway CLI, --mcp-config JSON, single chokepoint)
//
// The K8s MCP Server runs with --cluster-provider in-cluster and uses a
// scoped ServiceAccount with ClusterRoleBinding for least-privilege access.
// EAIGW routes tool calls to the K8s MCP Server via the loopback-cluster backend.
//
// Total memory: ~66 MB (EAIGW 50 MB + K8s MCP Server 16 MB).
func DeployFleetInfra(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "🚀 Deploying Fleet E2E Infrastructure...")

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
        - "--transport=http"
        ports:
        - name: http
          containerPort: 8080
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
            memory: "16Mi"
            cpu: "25m"
          limits:
            memory: "32Mi"
            cpu: "100m"
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
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: eaigw-mcp-config
  namespace: %[1]s
data:
  mcp-servers.json: |
    [{"name": "loopback-cluster", "host": "http://kube-mcp-server.%[1]s.svc.cluster.local:8080/mcp"}]
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: eaigw
  namespace: %[1]s
  labels:
    app: eaigw
    component: fleet
spec:
  replicas: 1
  selector:
    matchLabels:
      app: eaigw
  template:
    metadata:
      labels:
        app: eaigw
        component: fleet
    spec:
      containers:
      - name: eaigw
        image: %[3]s
        args:
        - "run"
        - "--mcp-config=/etc/aigw/mcp-servers.json"
        - "--run-id=0"
        ports:
        - name: mcp
          containerPort: 1975
        - name: health
          containerPort: 1064
        readinessProbe:
          httpGet:
            path: /health
            port: 1064
          initialDelaySeconds: 5
          periodSeconds: 5
        livenessProbe:
          httpGet:
            path: /health
            port: 1064
          initialDelaySeconds: 10
          periodSeconds: 10
        volumeMounts:
        - name: mcp-config
          mountPath: /etc/aigw
          readOnly: true
        resources:
          requests:
            memory: "50Mi"
            cpu: "50m"
          limits:
            memory: "100Mi"
            cpu: "200m"
      volumes:
      - name: mcp-config
        configMap:
          name: eaigw-mcp-config
---
apiVersion: v1
kind: Service
metadata:
  name: eaigw
  namespace: %[1]s
  labels:
    app: eaigw
    component: fleet
spec:
  type: NodePort
  ports:
  - name: mcp
    port: 1975
    targetPort: 1975
    nodePort: 31975
  - name: health
    port: 1064
    targetPort: 1064
    nodePort: 31064
  selector:
    app: eaigw
`, namespace, KubeMCPServerImage, EAIGWImage)

	cmd := exec.CommandContext(ctx, "kubectl", "apply", "--kubeconfig", kubeconfigPath, "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to deploy fleet infra: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "  ✅ Fleet manifests applied")

	_, _ = fmt.Fprintln(writer, "  ⏳ Waiting for K8s MCP Server to be ready...")
	waitMCP := exec.CommandContext(ctx, "kubectl", "rollout", "status", "deployment/kube-mcp-server",
		"-n", namespace, "--kubeconfig", kubeconfigPath, "--timeout=120s")
	waitMCP.Stdout = writer
	waitMCP.Stderr = writer
	if err := waitMCP.Run(); err != nil {
		return fmt.Errorf("K8s MCP Server rollout failed: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "  ✅ K8s MCP Server ready")

	_, _ = fmt.Fprintln(writer, "  ⏳ Waiting for EAIGW to be ready...")
	waitGW := exec.CommandContext(ctx, "kubectl", "rollout", "status", "deployment/eaigw",
		"-n", namespace, "--kubeconfig", kubeconfigPath, "--timeout=120s")
	waitGW.Stdout = writer
	waitGW.Stderr = writer
	if err := waitGW.Run(); err != nil {
		return fmt.Errorf("EAIGW rollout failed: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "  ✅ EAIGW ready")
	_, _ = fmt.Fprintln(writer, "✅ Fleet E2E infrastructure deployed (~66 MB)")

	return nil
}

// WaitForFleetReady polls the EAIGW health endpoint via NodePort until ready.
func WaitForFleetReady(writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "  ⏳ Waiting for EAIGW health endpoint...")
	deadline := time.Now().Add(90 * time.Second)
	client := &http.Client{Timeout: 5 * time.Second}
	for time.Now().Before(deadline) {
		resp, err := client.Get("http://localhost:31064/health")
		if err == nil && resp.StatusCode == http.StatusOK {
			_ = resp.Body.Close()
			_, _ = fmt.Fprintln(writer, "  ✅ EAIGW health endpoint reachable")
			return nil
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("EAIGW health endpoint not responsive after 90 seconds")
}

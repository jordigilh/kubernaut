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
	"os/exec"
	"strings"
	"time"
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
)

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

	builtImages, seededUUIDs, afRemediateNS, err = SetupFullPipelineInfrastructure(ctx, clusterName, kubeconfigPath, writer)
	if err != nil {
		return builtImages, seededUUIDs, afRemediateNS, fmt.Errorf("fullpipeline base setup failed: %w", err)
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

	if readyErr := WaitForFleetReady(writer); readyErr != nil {
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

// DeployFleetInfra deploys the fleet E2E infrastructure in the Kind cluster:
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
// Total memory: ~388 MB.
func DeployFleetInfra(ctx context.Context, namespace, kubeconfigPath, fmcImage string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "🚀 Deploying Fleet E2E Infrastructure...")

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
`, namespace, KubeMCPServerImage)

	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, kubeMCPManifest); err != nil {
		return fmt.Errorf("kube-mcp-server deployment failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "    Waiting for kube-mcp-server...")
	if err := waitForDeployment(ctx, "kube-mcp-server", namespace, kubeconfigPath, 120*time.Second, writer); err != nil {
		return fmt.Errorf("kube-mcp-server rollout failed: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "    ✅ kube-mcp-server ready")

	_, _ = fmt.Fprintln(writer, "    Creating HTTPRoute + MCPServerRegistration...")
	routeManifest := fmt.Sprintf(`---
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
  targetRef:
    group: gateway.networking.k8s.io
    kind: HTTPRoute
    name: kube-mcp-server-route
    namespace: %[1]s
`, namespace)

	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, routeManifest); err != nil {
		return fmt.Errorf("httpRoute/MCPServerRegistration creation failed: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "    ✅ MCPServerRegistration 'loopback-cluster' created")

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
      endpoint: "http://mcp-gateway.mcp-system.svc:8080/mcp"
      gatewayType: "kuadrant"
      namespace: "%[1]s"
    valkey:
      addr: "valkey.%[1]s.svc:6379"
    sync:
      interval: "10s"
      keyTtl: "30s"
    oauth2:
      tokenUrl: "http://dex.%[1]s.svc:5556/dex/token"
      credentialsDir: "/etc/fleetmetadatacache/fleet-oauth2"
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
  client-id: kubernaut-fleet-read
  client-secret: e2e-fleet-secret
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
`, namespace, fmcImage)

	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, fmcManifest); err != nil {
		return fmt.Errorf("fmc deployment failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "    Waiting for FMC...")
	if err := waitForDeployment(ctx, "fleetmetadatacache", namespace, kubeconfigPath, 120*time.Second, writer); err != nil {
		return fmt.Errorf("fmc rollout failed: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "    ✅ FMC ready")

	_, _ = fmt.Fprintln(writer, "✅ Fleet E2E infrastructure deployed (~388 MB)")
	return nil
}

// WaitForFleetReady verifies the Kuadrant MCP Gateway is reachable via NodePort
// by performing an MCP initialize handshake.
func WaitForFleetReady(writer io.Writer) error {
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
			return nil
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
		time.Sleep(3 * time.Second)
	}
	return fmt.Errorf("mcp gateway not responsive at http://localhost:31975/mcp after 120 seconds")
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

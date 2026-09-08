package infrastructure

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

// ============================================================================
// Thanos Fleet-Wide Metrics/Alerts Aggregation (manual setup-e2e-fleet-infra
// / Console demo use only)
// ============================================================================
//
// Deploys a Thanos sidecar alongside each cluster's Prometheus (hub AND
// spoke) plus a Thanos Querier in the hub cluster, so a single endpoint
// (the Querier) serves fleet-wide PromQL queries AND active-alert lookups
// spanning both clusters -- matching this repo's OWN documented production
// architecture for fleet monitoring aggregation (ADR-068 "Signal ingestion
// from remote clusters via Thanos multi-cluster Prometheus"; DD-EM-005 v1.3
// "Thanos/Prometheus federates metrics from every managed cluster into one
// queryable view, distinguished only by an external `cluster` label";
// DD-INT-020 Part E "KA uses Thanos Querier as the cross-cluster
// observability layer... just point cfg.Integrations.Tools.Prometheus.URL
// at Thanos Querier"). Deliberately NOT wired into provisionFleetCoreInfra
// (shared with the "fleet"/"fullpipeline" Ginkgo suites, which run a single
// shared Prometheus/AlertManager with no Thanos federation by design --
// TESTING_GUIDELINES.md Section on E2E fleet monitoring scope -- and must
// not change here).
//
// Thanos sidecar does NOT require object storage (--objstore.config) to
// serve cross-cluster queries: without it, the sidecar simply skips
// long-term block upload/shipping and serves recent local TSDB data (via
// StoreAPI) and the underlying Prometheus's active alerts/rules (via
// RulesAPI, confirmed supported for sidecar backends, not just Ruler --
// https://thanos.io/tip/proposals-done/202003-thanos-rules-federation.md/)
// straight off the running Prometheus -- exactly what a local demo needs,
// with zero extra components (no MinIO).
//
// Known Thanos limitation (thanos-io/thanos#7327, open as of 2026-08):
// external_labels (e.g. "cluster") are applied to /api/v1/rules alert
// DEFINITIONS proxied through Querier, but NOT to individual alert
// INSTANCES at /api/v1/alerts. This is a pre-existing gap in the
// architecture this mirrors, not something introduced here -- cluster-
// scoped alert filtering (DD-EM-005/Issue #2274) may not fully round-trip
// through Thanos Querier's /api/v1/alerts until upstream fixes it.
//
// WORKAROUND (confirmed live 2026-08-30, reproduced with a real firing
// alert): don't rely on external_labels for
// alert-instance cluster attribution at all. Bake `cluster: <name>` into
// each cluster's alerting rule as a STATIC rule label instead (alongside
// e.g. `severity: critical`) -- a rule's own static labels ARE attached to
// every alert instance it fires, independent of the Thanos federation gap
// above. Any NEW alerting rule added to this demo (or to a production
// Kubernaut fleet deployment using Thanos for cross-cluster alert
// aggregation) MUST follow this pattern, or AF's cluster_id filter
// (af_alerts.go's fleetClusterLabelKey) will silently exclude it from
// every cluster-scoped query.
//
// Cluster label VALUES matter too, not just presence: the non-hub cluster's
// value here is "remote-cluster" (confirmed 2026-08-30), matching the
// MCPServerRegistration/MCPRoute identity resource tools already use for
// that same physical cluster (test/e2e/fleet's canonical fixture) -- NOT a
// separate "spoke" label. AF's monitoring (cluster_id on alerts) and
// resource tools (cluster_id on kubectl_get/list_clusters) must agree on
// one identity per physical cluster, or a user/LLM correlating "this alert"
// with "that cluster's pods" has no way to know the two strings mean the
// same thing.
//
// Port allocation: this demo-only Thanos wiring uses 30195-30196, inside
// DD-TEST-001's reserved 30180-30199 "Metrics" Kind NodePort block but
// intentionally NOT added to that doc's registry -- those NodePorts are
// pure in-cluster (no Kind hostPort mapping, no Ginkgo suite dependency),
// unlike every other entry there.
const (
	// ThanosImage is the official Thanos container image, sidecar and
	// querier alike.
	ThanosImage = "quay.io/thanos/thanos:v0.37.2"

	// KubeStateMetricsImage is the official kube-state-metrics container
	// image, pinned to the version manually validated by QE on
	// fleet-e2e-remote 2026-08-30.
	KubeStateMetricsImage = "registry.k8s.io/kube-state-metrics/kube-state-metrics:v2.13.0"

	// ThanosSidecarGRPCPort is the Thanos sidecar's StoreAPI/RulesAPI gRPC
	// port, identical in both clusters (container-internal, no collision
	// risk since each cluster is its own network namespace).
	ThanosSidecarGRPCPort = 10901
	// thanosSidecarRemoteNodePort exposes the SPOKE cluster's Thanos
	// sidecar gRPC port for the hub-side Service+Endpoints bridge (same
	// DD-TEST-013 pattern as remoteKubeMCPServerNodePort).
	thanosSidecarRemoteNodePort = 30195
	// ThanosQuerierNodePort exposes the hub's Thanos Querier HTTP API on
	// the host for manual debugging; AF/EM consume it in-cluster via
	// ClusterIP DNS (thanos-querier-svc), not this NodePort.
	ThanosQuerierNodePort = 30196

	// thanosSidecarBridgeServiceName is the Service name created in the
	// hub cluster to bridge to the spoke's Thanos sidecar gRPC port.
	thanosSidecarBridgeServiceName = "thanos-sidecar-remote"

	// monitoringNamespace hosts the whole monitoring stack (Prometheus,
	// Thanos sidecars, Thanos Querier, AlertManager) on BOTH clusters --
	// deliberately separate from kubernautSystem (the Kubernaut app
	// namespace) and remoteMCPServerNamespace (Kuadrant/MCP's namespace).
	// Monitoring is platform infrastructure that predates and outlives any
	// single application (mirrors OCP's "openshift-monitoring", a typical
	// kube-prometheus-stack's "monitoring"); bundling it into either of
	// those would misrepresent that ownership relationship, same reasoning
	// that already moved kube-mcp-server to its own "mcp-system" (see
	// fleetmetadatacache_remote_cluster.go's remoteMCPServerNamespace doc
	// comment) -- confirmed by the user, 2026-08-30, when the first version
	// of this file landed the stack in kubernaut-system/mcp-system instead.
	monitoringNamespace = "monitoring"
)

// DeployKubeStateMetrics deploys kube-state-metrics into the given cluster,
// exposing Kubernetes object-state metrics (pod/deployment/replicaset/
// statefulset/daemonset/node phase, replica counts, etc.) that cAdvisor's
// container-resource metrics don't cover. The fleet demo's operator-managed
// Prometheus discovers this Service through its baseline ServiceMonitor, so
// call this before the Prometheus resource is reconciled.
func DeployKubeStateMetrics(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "  📈 Deploying kube-state-metrics in namespace %s...\n", namespace)

	manifest := buildKubeStateMetricsManifest(namespace)

	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, manifest); err != nil {
		return fmt.Errorf("failed to deploy kube-state-metrics: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "    Waiting for kube-state-metrics...")
	if err := waitForDeployment(ctx, "kube-state-metrics", namespace, kubeconfigPath, 60*time.Second, writer); err != nil {
		return fmt.Errorf("kube-state-metrics rollout failed: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "  ✅ kube-state-metrics ready")
	return nil
}

func buildKubeStateMetricsManifest(namespace string) string {
	return fmt.Sprintf(`---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: kube-state-metrics
  namespace: %[1]s
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kube-state-metrics
rules:
- apiGroups: [""]
  resources: ["pods", "nodes", "namespaces", "persistentvolumeclaims"]
  verbs: ["list", "watch"]
- apiGroups: ["apps"]
  resources: ["deployments", "replicasets", "statefulsets", "daemonsets"]
  verbs: ["list", "watch"]
- apiGroups: ["autoscaling"]
  resources: ["horizontalpodautoscalers"]
  verbs: ["list", "watch"]
- apiGroups: ["policy"]
  resources: ["poddisruptionbudgets"]
  verbs: ["list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kube-state-metrics
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: kube-state-metrics
subjects:
- kind: ServiceAccount
  name: kube-state-metrics
  namespace: %[1]s
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kube-state-metrics
  namespace: %[1]s
  labels:
    app: kube-state-metrics
spec:
  replicas: 1
  selector:
    matchLabels:
      app: kube-state-metrics
  template:
    metadata:
      labels:
        app: kube-state-metrics
    spec:
      serviceAccountName: kube-state-metrics
      containers:
      - name: kube-state-metrics
        image: %[2]s
        args:
        - "--resources=pods,deployments,replicasets,statefulsets,daemonsets,nodes,horizontalpodautoscalers,persistentvolumeclaims,poddisruptionbudgets"
        ports:
        - containerPort: 8080
          name: http-metrics
        - containerPort: 8081
          name: telemetry
        resources:
          requests:
            cpu: "50m"
            memory: "64Mi"
          limits:
            memory: "128Mi"
---
apiVersion: v1
kind: Service
metadata:
  name: kube-state-metrics
  namespace: %[1]s
  labels:
    app: kube-state-metrics
spec:
  selector:
    app: kube-state-metrics
  ports:
  - name: http-metrics
    port: 8080
    targetPort: 8080
`, namespace, KubeStateMetricsImage)
}

// exposeThanosSidecarNodePort creates a fixed-NodePort Service in the given
// cluster/namespace targeting the Thanos sidecar's gRPC port, so a remote
// cluster's hub-side bridge (CreateServiceBridge) has a stable IP:NodePort
// to dial. Mirrors SetupRemoteClusterForFMC's kube-mcp-server-nodeport
// pattern.
func exposeThanosSidecarNodePort(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	manifest := fmt.Sprintf(`---
apiVersion: v1
kind: Service
metadata:
  name: thanos-sidecar-nodeport
  namespace: %[1]s
spec:
  type: NodePort
  selector:
    app: prometheus
  ports:
  - port: %[2]d
    targetPort: %[2]d
    nodePort: %[3]d
`, namespace, ThanosSidecarGRPCPort, thanosSidecarRemoteNodePort)
	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, manifest); err != nil {
		return fmt.Errorf("thanos sidecar NodePort expose failed: %w", err)
	}
	return nil
}

// DeployThanosQuerier deploys a Thanos Querier into the hub cluster,
// fanning out to every storeAddress (each a "host:port" gRPC StoreAPI/
// RulesAPI endpoint -- the hub's own local sidecar via in-cluster DNS, and
// the spoke's via its hub-side bridge Service). Its HTTP API is Prometheus-
// API-compatible (including /api/v1/query, /api/v1/query_range, and
// /api/v1/alerts /api/v1/rules fanned out from every connected sidecar) --
// this is the single endpoint AF/EM's monitoring.prometheus.url should
// point at for fleet-wide visibility.
func DeployThanosQuerier(ctx context.Context, namespace, kubeconfigPath string, storeAddresses []string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "  🔭 Deploying Thanos Querier (fleet-wide metrics/alerts)...")

	var storeArgs strings.Builder
	for _, addr := range storeAddresses {
		storeArgs.WriteString(fmt.Sprintf("\n        - \"--store=%s\"", addr))
	}

	manifest := fmt.Sprintf(`---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: thanos-querier
  namespace: %[1]s
  labels:
    app: thanos-querier
spec:
  replicas: 1
  selector:
    matchLabels:
      app: thanos-querier
  template:
    metadata:
      labels:
        app: thanos-querier
    spec:
      containers:
      - name: thanos-querier
        image: %[2]s
        args:
        - "query"
        - "--http-address=0.0.0.0:9090"
        - "--grpc-address=0.0.0.0:%[3]d"
        - "--query.replica-label=replica"%[4]s
        ports:
        - containerPort: 9090
          name: http
        readinessProbe:
          httpGet:
            path: /-/ready
            port: 9090
          initialDelaySeconds: 5
          periodSeconds: 5
        resources:
          requests:
            memory: "64Mi"
            cpu: "50m"
          limits:
            memory: "256Mi"
            cpu: "250m"
---
apiVersion: v1
kind: Service
metadata:
  name: thanos-querier-svc
  namespace: %[1]s
spec:
  type: NodePort
  selector:
    app: thanos-querier
  ports:
  - name: http
    port: 9090
    targetPort: 9090
    nodePort: %[5]d
    protocol: TCP
`, namespace, ThanosImage, ThanosSidecarGRPCPort, storeArgs.String(), ThanosQuerierNodePort)

	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, manifest); err != nil {
		return fmt.Errorf("failed to deploy Thanos Querier: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "    Waiting for Thanos Querier...")
	if err := waitForDeployment(ctx, "thanos-querier", namespace, kubeconfigPath, 120*time.Second, writer); err != nil {
		return fmt.Errorf("thanos querier rollout failed: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "  ✅ Thanos Querier ready (NodePort %d, stores: %v)\n", ThanosQuerierNodePort, storeAddresses)
	return nil
}

// BridgeSpokeThanosSidecar exposes the spoke cluster's Thanos sidecar gRPC
// port via a fixed NodePort and creates the matching Service+Endpoints
// bridge in the hub cluster (same DD-TEST-013 pattern CreateServiceBridge
// already provides for kube-mcp-server-remote), returning the hub-local
// "host:port" address DeployThanosQuerier's storeAddresses should use to
// reach it.
func BridgeSpokeThanosSidecar(ctx context.Context, hubKubeconfigPath, hubNamespace, spokeKubeconfigPath, spokeNamespace, spokeClusterName string, writer io.Writer) (string, error) {
	if err := exposeThanosSidecarNodePort(ctx, spokeNamespace, spokeKubeconfigPath, writer); err != nil {
		return "", err
	}
	spokeIP, err := KindNodeBridgeIP(ctx, spokeClusterName+"-control-plane")
	if err != nil {
		return "", fmt.Errorf("failed to discover spoke node bridge IP: %w", err)
	}
	if err := CreateServiceBridge(ctx, hubKubeconfigPath, hubNamespace, thanosSidecarBridgeServiceName, ThanosSidecarGRPCPort, spokeIP, thanosSidecarRemoteNodePort, writer); err != nil {
		return "", fmt.Errorf("thanos sidecar bridge Service creation failed: %w", err)
	}
	return fmt.Sprintf("%s.%s.svc.cluster.local:%d", thanosSidecarBridgeServiceName, hubNamespace, ThanosSidecarGRPCPort), nil
}

// HubNodeBridgeIPAndPort resolves a "host:port" string a cluster OTHER than
// the hub can dial directly (no in-cluster DNS) to reach a Service exposed
// via NodePort on the hub -- used so the spoke's Prometheus can send alerts
// straight to the hub's AlertManager without a dedicated bridge Service (a
// Prometheus alertmanagers static_config target can be a raw IP:port).
func HubNodeBridgeIPAndPort(ctx context.Context, hubClusterName string, nodePort int) (string, error) {
	hubIP, err := KindNodeBridgeIP(ctx, hubClusterName+"-control-plane")
	if err != nil {
		return "", fmt.Errorf("failed to discover hub node bridge IP: %w", err)
	}
	return fmt.Sprintf("%s:%d", hubIP, nodePort), nil
}

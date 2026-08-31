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
// alert -- see DeployDemoAlertingRules): don't rely on external_labels for
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
	// thanosSidecarHTTPPort is the sidecar's own HTTP port (unused by
	// Querier, which talks gRPC only; kept for completeness/debugging).
	thanosSidecarHTTPPort = 10902

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

// DeployPrometheusWithThanosSidecar deploys a minimal Prometheus (kubelet-
// cadvisor scrape only -- no synthetic Ginkgo-test alerting-rule fixtures,
// unlike DeployPrometheus) plus a Thanos sidecar container in the same pod,
// sharing an emptyDir TSDB volume. clusterLabel becomes the Prometheus
// external_labels.cluster value Thanos uses to distinguish this cluster's
// series/alerts once federated through the Querier. alertManagerTarget is a
// raw host:port Prometheus dials directly for its alerting sink -- a
// same-cluster Service DNS name for the hub, or the hub's node-bridge
// IP:NodePort for the spoke (which has no in-cluster route to the hub's
// AlertManager Service).
//
// Mounts a "rules" volume from a "prometheus-rules" ConfigMap at
// /etc/prometheus/rules (rule_files: /etc/prometheus/rules/*.yml) --
// DeployDemoAlertingRules creates that ConfigMap and MUST be called first
// for each cluster, or this Deployment's pod will stick in
// ContainerCreating waiting on a ConfigMap that doesn't exist yet.
func DeployPrometheusWithThanosSidecar(ctx context.Context, namespace, kubeconfigPath, clusterLabel, alertManagerTarget string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "  📊 Deploying Prometheus+Thanos-sidecar (cluster=%s) in namespace %s...\n", clusterLabel, namespace)

	manifest := fmt.Sprintf(`---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: prometheus
  namespace: %[1]s
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: prometheus
rules:
- apiGroups: [""]
  resources: ["nodes", "nodes/proxy", "nodes/metrics", "pods", "services", "endpoints"]
  verbs: ["get", "list", "watch"]
- nonResourceURLs: ["/metrics", "/metrics/cadvisor"]
  verbs: ["get"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: prometheus
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: prometheus
subjects:
- kind: ServiceAccount
  name: prometheus
  namespace: %[1]s
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: prometheus-config
  namespace: %[1]s
data:
  prometheus.yml: |
    global:
      scrape_interval: 15s
      evaluation_interval: 15s
      external_labels:
        cluster: %[3]s
    rule_files:
    - /etc/prometheus/rules/*.yml
    scrape_configs:
    - job_name: 'kubelet-cadvisor'
      scrape_interval: 10s
      kubernetes_sd_configs:
      - role: node
      scheme: https
      tls_config:
        insecure_skip_verify: true
      bearer_token_file: /var/run/secrets/kubernetes.io/serviceaccount/token
      relabel_configs:
      - action: labelmap
        regex: __meta_kubernetes_node_label_(.+)
      - source_labels: [__meta_kubernetes_node_address_InternalIP]
        target_label: __address__
        replacement: ${1}:10250
      - target_label: __metrics_path__
        replacement: /metrics/cadvisor
      metric_relabel_configs:
      - source_labels: [__name__]
        regex: 'container_(cpu_usage_seconds_total|memory_working_set_bytes|memory_usage_bytes|spec_memory_limit_bytes)'
        action: keep
    - job_name: 'kube-state-metrics'
      scrape_interval: 15s
      static_configs:
      - targets: ['kube-state-metrics.%[1]s.svc.cluster.local:8080']
    alerting:
      alertmanagers:
      - static_configs:
        - targets: ['%[4]s']
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: prometheus
  namespace: %[1]s
  labels:
    app: prometheus
spec:
  replicas: 1
  selector:
    matchLabels:
      app: prometheus
  template:
    metadata:
      labels:
        app: prometheus
    spec:
      serviceAccountName: prometheus
      containers:
      - name: prometheus
        image: %[2]s
        args:
        - "--config.file=/etc/prometheus/prometheus.yml"
        - "--storage.tsdb.path=/prometheus"
        - "--storage.tsdb.retention.time=6h"
        - "--web.enable-lifecycle"
        - "--web.listen-address=:9090"
        ports:
        - containerPort: 9090
          name: http
        readinessProbe:
          httpGet:
            path: /-/ready
            port: 9090
          initialDelaySeconds: 5
          periodSeconds: 5
        livenessProbe:
          httpGet:
            path: /-/healthy
            port: 9090
          initialDelaySeconds: 10
          periodSeconds: 10
        volumeMounts:
        - name: config
          mountPath: /etc/prometheus
        - name: tsdb-data
          mountPath: /prometheus
        - name: rules
          mountPath: /etc/prometheus/rules
        resources:
          requests:
            memory: "256Mi"
            cpu: "100m"
          limits:
            memory: "512Mi"
            cpu: "500m"
      - name: thanos-sidecar
        image: %[5]s
        args:
        - "sidecar"
        - "--tsdb.path=/prometheus"
        - "--prometheus.url=http://localhost:9090"
        - "--grpc-address=0.0.0.0:%[6]d"
        - "--http-address=0.0.0.0:%[7]d"
        ports:
        - containerPort: %[6]d
          name: grpc
        - containerPort: %[7]d
          name: sidecar-http
        volumeMounts:
        - name: tsdb-data
          mountPath: /prometheus
        resources:
          requests:
            memory: "64Mi"
            cpu: "50m"
          limits:
            memory: "256Mi"
            cpu: "250m"
      volumes:
      - name: config
        configMap:
          name: prometheus-config
      - name: tsdb-data
        emptyDir: {}
      - name: rules
        configMap:
          name: prometheus-rules
---
apiVersion: v1
kind: Service
metadata:
  name: prometheus-svc
  namespace: %[1]s
spec:
  type: NodePort
  selector:
    app: prometheus
  ports:
  - name: http
    port: 9090
    targetPort: 9090
    nodePort: %[8]d
    protocol: TCP
---
apiVersion: v1
kind: Service
metadata:
  name: thanos-sidecar-grpc
  namespace: %[1]s
  labels:
    app: prometheus
spec:
  selector:
    app: prometheus
  ports:
  - name: grpc
    port: %[6]d
    targetPort: %[6]d
    protocol: TCP
`, namespace, PrometheusImage, clusterLabel, alertManagerTarget, ThanosImage, ThanosSidecarGRPCPort, thanosSidecarHTTPPort, PrometheusNodePort)

	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, manifest); err != nil {
		return fmt.Errorf("failed to deploy Prometheus+Thanos-sidecar: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "    Waiting for Prometheus+Thanos-sidecar...")
	if err := waitForDeployment(ctx, "prometheus", namespace, kubeconfigPath, 120*time.Second, writer); err != nil {
		return fmt.Errorf("prometheus+thanos-sidecar rollout failed: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "  ✅ Prometheus+Thanos-sidecar ready (cluster=%s, NodePort %d)\n", clusterLabel, PrometheusNodePort)
	return nil
}

// DeployKubeStateMetrics deploys kube-state-metrics into the given cluster,
// exposing Kubernetes object-state metrics (pod/deployment/replicaset/
// statefulset/daemonset/node phase, replica counts, etc.) that cAdvisor's
// container-resource metrics don't cover. DeployPrometheusWithThanosSidecar's
// generated config already scrapes this Service by name
// ("kube-state-metrics.<namespace>.svc.cluster.local:8080"), so call this
// BEFORE (or alongside) that function for each cluster -- Prometheus will
// otherwise just log scrape failures until this exists, matching manual QE
// setup confirmed 2026-08-30 (fleet-e2e-remote's spoke cluster) that
// exposed AF's monitoring-backed tools as returning empty pod/deployment
// data without it.
func DeployKubeStateMetrics(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "  📈 Deploying kube-state-metrics in namespace %s...\n", namespace)

	manifest := fmt.Sprintf(`---
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
  resources: ["pods", "nodes", "namespaces"]
  verbs: ["list", "watch"]
- apiGroups: ["apps"]
  resources: ["deployments", "replicasets", "statefulsets", "daemonsets"]
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
        - "--resources=pods,deployments,replicasets,statefulsets,daemonsets,nodes"
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

// DeployDemoAlertingRules creates the "prometheus-rules" ConfigMap that
// DeployPrometheusWithThanosSidecar mounts at /etc/prometheus/rules. MUST be
// called before that function for the same cluster (the Deployment it
// creates references this ConfigMap by name).
//
// The rule's cluster attribution is baked in as a STATIC rule label
// (labels.cluster below), not left to Thanos's external_labels -- Thanos
// Querier only merges external_labels into a rule's own definition
// (/api/v1/rules[].labels), NOT into the fired alert instances nested
// underneath it or returned by the flat /api/v1/alerts endpoint AF actually
// queries (pkg/apifrontend/prometheus.Client.GetAlerts). That gap is a
// confirmed, open upstream limitation (thanos-io/thanos#7327) reproduced
// live 2026-08-30: an alert firing on the spoke came back from Thanos
// Querier's /api/v1/alerts with zero "cluster" label, so AF's cluster_id
// filter (af_alerts.go's fleetClusterLabelKey) silently excluded it from
// every cluster-scoped query. A rule's own static labels, by contrast, ARE
// attached to every alert instance it fires -- confirmed by this same rule's
// pre-existing "severity: critical" label showing up correctly -- so that's
// the layer this needs to be set at, not the Prometheus/Thanos federation
// layer.
//
// Deploys the same KubePodCrashLooping rule to both hub and spoke
// regardless of where the demo-checkout namespace/workload actually lives
// (spoke only, as of 2026-08-30): the PromQL selector simply returns no
// series and the rule stays inactive on a cluster without a matching
// namespace, so this is harmless and makes the demo scenario portable to
// either cluster without further code changes.
func DeployDemoAlertingRules(ctx context.Context, namespace, kubeconfigPath, clusterLabel string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "  🔔 Deploying demo alerting rules (cluster=%s) in namespace %s...\n", clusterLabel, namespace)

	manifest := fmt.Sprintf(`---
apiVersion: v1
kind: ConfigMap
metadata:
  name: prometheus-rules
  namespace: %[1]s
data:
  demo-app-alerts.yml: |
    groups:
    - name: demo-app
      rules:
      - alert: KubePodCrashLooping
        expr: |
          max_over_time(
            kube_pod_container_status_waiting_reason{
              namespace="demo-checkout",
              reason="CrashLoopBackOff"
            }[2m]
          ) > 0
        for: 30s
        labels:
          severity: critical
          cluster: %[2]s
        annotations:
          summary: 'Container {{ $labels.container }} in pod {{ $labels.pod }} is restarting repeatedly ({{ $value | humanize }} restarts in 5m).'
          description: 'A container in namespace {{ $labels.namespace }} is failing to reach a stable running state. Elevated restart rate may indicate service degradation.'
`, namespace, clusterLabel)

	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, manifest); err != nil {
		return fmt.Errorf("failed to deploy demo alerting rules: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "  ✅ demo alerting rules ready")
	return nil
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
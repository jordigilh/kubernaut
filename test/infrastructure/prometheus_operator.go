package infrastructure

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

const (
	// The chart installs the Prometheus Operator and its CRDs without requiring
	// client-side annotation storage for the large CRD definitions.
	prometheusOperatorHelmChart   = "prometheus-community/kube-prometheus-stack"
	prometheusOperatorHelmVersion = "88.1.5"
	prometheusOperatorNamespace   = "prometheus-operator"
	managedPrometheusName         = "fleet-spoke"
	managedPrometheusStatefulSet  = "prometheus-fleet-spoke"
	localPrometheusName           = "local"
	localPrometheusStatefulSet    = "prometheus-local"
)

type managedPrometheusOptions struct {
	clusterLabel       string
	alertManagerTarget string
	prometheusName     string
	statefulSetName    string
	thanosEnabled      bool
}

// InstallPrometheusOperator installs the CRDs and controller used by the fleet
// demo monitoring clusters. Both hub and spoke use this same path.
func InstallPrometheusOperator(ctx context.Context, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "  Installing Prometheus Operator in namespace %s...\n", prometheusOperatorNamespace)
	if err := runHelmUpgradeInstall(ctx, kubeconfigPath, writer, "prometheus-operator",
		prometheusOperatorHelmChart, prometheusOperatorNamespace,
		"--version", prometheusOperatorHelmVersion,
		"--set", "prometheusOperator.fullnameOverride=prometheus-operator",
		"--set", "defaultRules.create=false",
		"--set", "alertmanager.enabled=false",
		"--set", "grafana.enabled=false",
		"--set", "kubeStateMetrics.enabled=false",
		"--set", "nodeExporter.enabled=false",
		"--set", "prometheus.enabled=false",
	); err != nil {
		return fmt.Errorf("prometheus operator Helm install failed: %w", err)
	}
	if err := waitForDeployment(ctx, "prometheus-operator", prometheusOperatorNamespace, kubeconfigPath, 180*time.Second, writer); err != nil {
		return fmt.Errorf("prometheus operator rollout failed: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "  Prometheus Operator ready")
	return nil
}

// DeployManagedPrometheusWithThanosSidecar installs the common fleet demo
// Prometheus path. Scenario monitoring is expressed through ServiceMonitor,
// PodMonitor, Probe, and PrometheusRule resources; only infrastructure-owned
// kubelet scraping remains an additional scrape config.
func DeployManagedPrometheusWithThanosSidecar(ctx context.Context, namespace, kubeconfigPath, clusterLabel, alertManagerTarget string, writer io.Writer) error {
	return deployManagedPrometheus(ctx, namespace, kubeconfigPath, managedPrometheusOptions{
		clusterLabel:       clusterLabel,
		alertManagerTarget: alertManagerTarget,
		prometheusName:     managedPrometheusName,
		statefulSetName:    managedPrometheusStatefulSet,
		thanosEnabled:      true,
	}, writer)
}

func DeployManagedPrometheus(ctx context.Context, namespace, kubeconfigPath, clusterLabel, alertManagerTarget string, writer io.Writer) error {
	return deployManagedPrometheus(ctx, namespace, kubeconfigPath, managedPrometheusOptions{
		clusterLabel:       clusterLabel,
		alertManagerTarget: alertManagerTarget,
		prometheusName:     localPrometheusName,
		statefulSetName:    localPrometheusStatefulSet,
		thanosEnabled:      false,
	}, writer)
}

func deployManagedPrometheus(ctx context.Context, namespace, kubeconfigPath string, options managedPrometheusOptions, writer io.Writer) error {
	manifest, err := buildManagedPrometheusManifestWithOptionsChecked(namespace, options)
	if err != nil {
		return err
	}

	component := "Prometheus"
	if options.thanosEnabled {
		component += "+Thanos"
	}
	_, _ = fmt.Fprintf(writer, "  Deploying operator-managed %s (cluster=%s) in namespace %s...\n", component, options.clusterLabel, namespace)
	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, manifest); err != nil {
		return fmt.Errorf("managed Prometheus manifest apply failed: %w", err)
	}
	if err := waitForResource(ctx, kubeconfigPath, "statefulset", options.statefulSetName, namespace, 180*time.Second); err != nil {
		return fmt.Errorf("managed Prometheus StatefulSet was not created: %w", err)
	}
	if err := runKubectl(ctx, kubeconfigPath, writer, "rollout", "status",
		"statefulset/"+options.statefulSetName, "-n", namespace, "--timeout=180s"); err != nil {
		return fmt.Errorf("managed Prometheus rollout failed: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "  operator-managed %s ready (cluster=%s, NodePort %d)\n", component, options.clusterLabel, PrometheusNodePort)
	return nil
}

//nolint:unparam // tests exercise namespace substitution in the shared manifest builder.
func buildManagedPrometheusManifest(namespace, clusterLabel, alertManagerTarget string) string {
	manifest, err := buildManagedPrometheusManifestWithOptionsChecked(namespace, managedPrometheusOptions{
		clusterLabel:       clusterLabel,
		alertManagerTarget: alertManagerTarget,
		prometheusName:     managedPrometheusName,
		statefulSetName:    managedPrometheusStatefulSet,
		thanosEnabled:      true,
	})
	if err != nil {
		return ""
	}
	return manifest
}

func buildLocalManagedPrometheusManifest(namespace, clusterLabel, alertManagerTarget string) string {
	manifest, err := buildManagedPrometheusManifestWithOptionsChecked(namespace, managedPrometheusOptions{
		clusterLabel:       clusterLabel,
		alertManagerTarget: alertManagerTarget,
		prometheusName:     localPrometheusName,
		statefulSetName:    localPrometheusStatefulSet,
		thanosEnabled:      false,
	})
	if err != nil {
		return ""
	}
	return manifest
}

func buildManagedPrometheusManifestChecked(namespace, clusterLabel, alertManagerTarget string) (string, error) {
	return buildManagedPrometheusManifestWithOptionsChecked(namespace, managedPrometheusOptions{
		clusterLabel:       clusterLabel,
		alertManagerTarget: alertManagerTarget,
		prometheusName:     managedPrometheusName,
		statefulSetName:    managedPrometheusStatefulSet,
		thanosEnabled:      true,
	})
}

func buildManagedPrometheusManifestWithOptionsChecked(namespace string, options managedPrometheusOptions) (string, error) {
	host, portText, err := net.SplitHostPort(options.alertManagerTarget)
	if err != nil {
		return "", fmt.Errorf("invalid Alertmanager bridge address %q: %w", options.alertManagerTarget, err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return "", fmt.Errorf("invalid Alertmanager bridge address %q: invalid port", options.alertManagerTarget)
	}

	alertManagerName := "alertmanager-remote"
	alertManagerPortName := "web"
	alertManagerBridge := fmt.Sprintf(`apiVersion: v1
kind: Service
metadata:
  name: alertmanager-remote
  namespace: %s
spec:
  ports:
  - name: web
    port: 9093
    targetPort: 9093
---
apiVersion: v1
kind: Endpoints
metadata:
  name: alertmanager-remote
  namespace: %s
subsets:
- addresses:
  - ip: %s
  ports:
  - name: web
    port: %d
`, namespace, namespace, host, port)
	if net.ParseIP(host) == nil {
		alertManagerName = strings.Split(host, ".")[0]
		alertManagerPortName = "http"
		alertManagerBridge = ""
	}
	thanosConfig := ""
	if options.thanosEnabled {
		thanosConfig = fmt.Sprintf("  thanos:\n    image: %s\n", ThanosImage)
	}

	return fmt.Sprintf(`---
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
  resources: ["nodes", "nodes/proxy", "nodes/metrics", "pods", "services", "endpoints", "namespaces"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["discovery.k8s.io"]
  resources: ["endpointslices"]
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
kind: Secret
metadata:
  name: prometheus-additional-scrape-configs
  namespace: %[1]s
stringData:
  additional-scrape-configs.yaml: |
    - job_name: kubelet-cadvisor
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
---
%[2]s
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
    nodePort: %[3]d
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
---
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: kube-state-metrics
  namespace: %[1]s
spec:
  selector:
    matchLabels:
      app: kube-state-metrics
  endpoints:
  - port: http-metrics
    interval: 15s
    honorLabels: true
---
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: demo-alerts
  namespace: %[1]s
spec:
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
        cluster: %[4]s
      annotations:
        summary: 'Container {{ $labels.container }} in pod {{ $labels.pod }} is restarting repeatedly ({{ $value | humanize }} restarts in 5m).'
        description: 'A container in namespace {{ $labels.namespace }} is failing to reach a stable running state. Elevated restart rate may indicate service degradation.'
---
apiVersion: monitoring.coreos.com/v1
kind: Prometheus
metadata:
  name: %[5]s
  namespace: %[1]s
spec:
  image: %[6]s
  replicas: 1
  serviceAccountName: prometheus
  podMetadata:
    labels:
      app: prometheus
  externalLabels:
    cluster: %[4]s
  scrapeInterval: 15s
  evaluationInterval: 15s
  retention: 6h
  storage:
    emptyDir: {}
  serviceMonitorSelector: {}
  serviceMonitorNamespaceSelector: {}
  podMonitorSelector: {}
  podMonitorNamespaceSelector: {}
  probeSelector: {}
  probeNamespaceSelector: {}
  ruleSelector: {}
  ruleNamespaceSelector: {}
  additionalScrapeConfigs:
    name: prometheus-additional-scrape-configs
    key: additional-scrape-configs.yaml
  alerting:
    alertmanagers:
    - name: %[7]s
      namespace: %[1]s
      port: %[9]s
%[8]s`, namespace, alertManagerBridge, PrometheusNodePort, options.clusterLabel, options.prometheusName, PrometheusImage, alertManagerName, thanosConfig, alertManagerPortName), nil
}

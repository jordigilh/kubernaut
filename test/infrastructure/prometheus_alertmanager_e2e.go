package infrastructure

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"time"
)

// ============================================================================
// Prometheus & AlertManager E2E Infrastructure
// ============================================================================
//
// Deploys real Prometheus and AlertManager instances into a Kind cluster for
// E2E testing of the Effectiveness Monitor (EM) service.
//
// This infrastructure enables:
//   - Real PromQL query validation (catches API contract mismatches)
//   - Real AlertManager API validation
//   - Metric injection via Prometheus remote write API
//   - Alert injection via AlertManager REST API
//
// Port Allocation (DD-TEST-001 v2.8):
//   - Prometheus: NodePort 30190, host port 9190
//   - AlertManager: NodePort 30193, host port 9193
//
// References:
//   - ADR-EM-001: Effectiveness Monitor integration architecture
//   - TESTING_GUIDELINES.md v2.6.0 Section 4a: Prom/AM mocking policy
// ============================================================================

const (
	// PrometheusNodePort is the Kind NodePort for Prometheus (DD-TEST-001 v2.8)
	PrometheusNodePort = 30190
	// PrometheusHostPort is the host port mapped to the Prometheus NodePort
	PrometheusHostPort = 9190

	// AlertManagerNodePort is the Kind NodePort for AlertManager (DD-TEST-001 v2.8)
	AlertManagerNodePort = 30193
	// AlertManagerHostPort is the host port mapped to the AlertManager NodePort
	AlertManagerHostPort = 9193

	// PrometheusImage is the official Prometheus container image
	PrometheusImage = "prom/prometheus:latest"
	// AlertManagerImage is the official AlertManager container image
	AlertManagerImage = "prom/alertmanager:latest"
)

// DeployPrometheus deploys a real Prometheus instance into the Kind cluster.
//
// Configuration:
//   - --web.enable-remote-write-receiver: Accepts remote write for test data injection
//   - --web.enable-otlp-receiver: Accepts OTLP/HTTP JSON for metric injection (used by InjectMetrics)
//   - --storage.tsdb.retention.time=1h: Minimal retention for test data
//   - --storage.tsdb.min-block-duration=5m: Fast compaction for testing
//
// The deployment includes:
//   - ServiceAccount + ClusterRole + ClusterRoleBinding (for kubelet/cAdvisor scraping)
//   - ConfigMap with cAdvisor scrape job (kubernetes_sd_configs: role: node)
//   - Deployment with single replica (serviceAccountName: prometheus)
//   - NodePort Service for test runner access
//
// Parameters:
//   - ctx: Context for cancellation
//   - namespace: Target namespace (e.g., "kubernaut-system")
//   - kubeconfigPath: Path to kubeconfig for kubectl commands
//   - writer: Output writer for progress logging
func DeployPrometheus(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "  📊 Deploying Prometheus in namespace %s...\n", namespace)

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
      - target_label: __address__
        replacement: kubernetes.default.svc:443
      - source_labels: [__meta_kubernetes_node_name]
        regex: (.+)
        target_label: __metrics_path__
        replacement: /api/v1/nodes/${1}/proxy/metrics/cadvisor
      metric_relabel_configs:
      - source_labels: [__name__]
        regex: 'container_(cpu_usage_seconds_total|memory_working_set_bytes|memory_usage_bytes|spec_memory_limit_bytes)'
        action: keep
    alerting:
      alertmanagers:
      - static_configs:
        - targets: ['alertmanager-svc.%[1]s.svc.cluster.local:9093']
    rule_files:
    - /etc/prometheus/rules/*.yml
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: prometheus-rules
  namespace: %[1]s
data:
  memory-eater.yml: |
    groups:
    - name: memory-eater-oom.rules
      interval: 10s
      rules:
      - alert: MemoryExceedsLimit
        expr: |
          (container_memory_working_set_bytes{namespace=~"fp-am-.*", pod=~"memory-eater-.*"}
           / container_spec_memory_limit_bytes{namespace=~"fp-am-.*", pod=~"memory-eater-.*"}) >= 0.90
        for: 10s
        labels:
          severity: critical
        annotations:
          summary: "Container memory exceeds limit"
          description: "Pod {{ $labels.pod }} using {{ $value | humanizePercentage }} of memory limit"
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
        - "--web.enable-remote-write-receiver"
        - "--web.enable-otlp-receiver"
        - "--storage.tsdb.retention.time=1h"
        - "--storage.tsdb.min-block-duration=5m"
        - "--web.listen-address=:9090"
        ports:
        - containerPort: 9090
          name: http
          protocol: TCP
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
        - name: rules
          mountPath: /etc/prometheus/rules
        resources:
          requests:
            memory: "128Mi"
            cpu: "100m"
          limits:
            memory: "256Mi"
            cpu: "500m"
      volumes:
      - name: config
        configMap:
          name: prometheus-config
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
    nodePort: %[3]d
    protocol: TCP
`, namespace, PrometheusImage, PrometheusNodePort)

	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = bytes.NewBufferString(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to deploy Prometheus: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "  ✅ Prometheus deployed (NodePort %d)\n", PrometheusNodePort)
	return nil
}

// DeployAlertManager deploys a real AlertManager instance into the Kind cluster.
//
// Configuration:
//   - Minimal routing config (all alerts go to a null receiver)
//   - Single replica for testing
//
// Parameters:
//   - ctx: Context for cancellation
//   - namespace: Target namespace (e.g., "kubernaut-system")
//   - kubeconfigPath: Path to kubeconfig for kubectl commands
//   - writer: Output writer for progress logging
func DeployAlertManager(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "  🔔 Deploying AlertManager in namespace %s...\n", namespace)

	manifest := fmt.Sprintf(`---
apiVersion: v1
kind: ConfigMap
metadata:
  name: alertmanager-config
  namespace: %[1]s
data:
  alertmanager.yml: |
    global:
      resolve_timeout: 1m
    route:
      receiver: 'null'
      group_wait: 5s
      group_interval: 5s
      repeat_interval: 1h
      routes:
      - match:
          alertname: MemoryExceedsLimit
        receiver: gateway-webhook
    receivers:
    - name: 'null'
    - name: gateway-webhook
      webhook_configs:
      - url: 'http://gateway-service.%[1]s.svc.cluster.local:8080/api/v1/signals/prometheus'
        send_resolved: false
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: alertmanager
  namespace: %[1]s
  labels:
    app: alertmanager
spec:
  replicas: 1
  selector:
    matchLabels:
      app: alertmanager
  template:
    metadata:
      labels:
        app: alertmanager
    spec:
      containers:
      - name: alertmanager
        image: %[2]s
        args:
        - "--config.file=/etc/alertmanager/alertmanager.yml"
        - "--web.listen-address=:9093"
        - "--log.level=debug"
        ports:
        - containerPort: 9093
          name: http
          protocol: TCP
        readinessProbe:
          httpGet:
            path: /-/ready
            port: 9093
          initialDelaySeconds: 5
          periodSeconds: 5
        livenessProbe:
          httpGet:
            path: /-/healthy
            port: 9093
          initialDelaySeconds: 10
          periodSeconds: 10
        volumeMounts:
        - name: config
          mountPath: /etc/alertmanager
        resources:
          requests:
            memory: "64Mi"
            cpu: "50m"
          limits:
            memory: "128Mi"
            cpu: "250m"
      volumes:
      - name: config
        configMap:
          name: alertmanager-config
---
apiVersion: v1
kind: Service
metadata:
  name: alertmanager-svc
  namespace: %[1]s
spec:
  type: NodePort
  selector:
    app: alertmanager
  ports:
  - name: http
    port: 9093
    targetPort: 9093
    nodePort: %[3]d
    protocol: TCP
`, namespace, AlertManagerImage, AlertManagerNodePort)

	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = bytes.NewBufferString(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to deploy AlertManager: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "  ✅ AlertManager deployed (NodePort %d)\n", AlertManagerNodePort)
	return nil
}

// WaitForPrometheusReady polls the Prometheus readiness endpoint until it responds 200 OK.
func WaitForPrometheusReady(promURL string, timeout time.Duration, writer io.Writer) error {
	return waitForHTTPReady(promURL+"/-/ready", "Prometheus", timeout, writer)
}

// WaitForAlertManagerReady polls the AlertManager readiness endpoint until it responds 200 OK.
func WaitForAlertManagerReady(amURL string, timeout time.Duration, writer io.Writer) error {
	return waitForHTTPReady(amURL+"/-/ready", "AlertManager", timeout, writer)
}

func waitForHTTPReady(url, serviceName string, timeout time.Duration, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "  ⏳ Waiting for %s to be ready (%s)...\n", serviceName, url)
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 5 * time.Second}

	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil && resp.StatusCode == http.StatusOK {
			_ = resp.Body.Close()
			_, _ = fmt.Fprintf(writer, "  ✅ %s is ready\n", serviceName)
			return nil
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("timeout waiting for %s at %s after %v", serviceName, url, timeout)
}

// ============================================================================
// Test Data Injection Helpers
// ============================================================================

// TestMetric represents a single metric sample for injection into Prometheus.
type TestMetric struct {
	Name      string            // Metric name (e.g., "container_cpu_usage_seconds_total")
	Labels    map[string]string // Label set for the metric
	Value     float64           // Metric value
	Timestamp time.Time         // Timestamp for the sample
}

// TestAlert represents an alert for injection into AlertManager.
type TestAlert struct {
	Name         string            // Alert name (alertname label)
	Labels       map[string]string // Additional labels
	Annotations  map[string]string // Alert annotations
	Status       string            // "firing" or "resolved"
	StartsAt     time.Time         // When the alert started firing
	EndsAt       time.Time         // When the alert was resolved (zero for firing)
	GeneratorURL string            // URL of the alert generator
}

// InjectAlerts posts alerts to the AlertManager API for testing.
//
// AlertManager API v2 accepts alerts as a JSON array via POST /api/v2/alerts.
// Alerts are immediately queryable after injection.
//
// Parameters:
//   - amURL: AlertManager base URL (e.g., "http://127.0.0.1:9193")
//   - alerts: Slice of test alerts to inject
func InjectAlerts(amURL string, alerts []TestAlert) error {
	type amAlert struct {
		Labels       map[string]string `json:"labels"`
		Annotations  map[string]string `json:"annotations,omitempty"`
		StartsAt     string            `json:"startsAt,omitempty"`
		EndsAt       string            `json:"endsAt,omitempty"`
		GeneratorURL string            `json:"generatorURL,omitempty"`
	}

	var amAlerts []amAlert
	for _, a := range alerts {
		labels := make(map[string]string)
		for k, v := range a.Labels {
			labels[k] = v
		}
		labels["alertname"] = a.Name

		alert := amAlert{
			Labels:       labels,
			Annotations:  a.Annotations,
			GeneratorURL: a.GeneratorURL,
		}

		if !a.StartsAt.IsZero() {
			alert.StartsAt = a.StartsAt.UTC().Format(time.RFC3339)
		}
		if !a.EndsAt.IsZero() {
			alert.EndsAt = a.EndsAt.UTC().Format(time.RFC3339)
		} else if a.Status == "resolved" {
			// For resolved alerts, set EndsAt to now
			alert.EndsAt = time.Now().UTC().Format(time.RFC3339)
		}

		amAlerts = append(amAlerts, alert)
	}

	body, err := json.Marshal(amAlerts)
	if err != nil {
		return fmt.Errorf("failed to marshal alerts: %w", err)
	}

	resp, err := http.Post(amURL+"/api/v2/alerts", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to POST alerts to AlertManager: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("AlertManager returned status %d for POST /api/v2/alerts", resp.StatusCode)
	}

	return nil
}

// InjectMetrics injects metric samples into Prometheus via the OTLP HTTP JSON endpoint.
//
// Prometheus must be started with --web.enable-otlp-receiver to accept OTLP metrics.
// Metrics are immediately queryable via PromQL after injection.
//
// This implementation uses the OTLP/HTTP JSON protocol, requiring only net/http and
// encoding/json from the Go standard library (no external dependencies).
//
// Metrics are injected as OTLP Gauge data points. For test purposes, all metrics are
// treated as gauges since the EM only reads instantaneous values via PromQL.
//
// Parameters:
//   - promURL: Prometheus base URL (e.g., "http://127.0.0.1:9190")
//   - metrics: Slice of test metrics to inject
func InjectMetrics(promURL string, metrics []TestMetric) error {
	if len(metrics) == 0 {
		return nil
	}

	// Group metrics by name so each unique metric name becomes one OTLP Metric
	// with multiple data points (if the same metric has different label sets).
	metricsByName := make(map[string][]otlpDataPoint)
	for _, m := range metrics {
		attrs := make([]otlpAttribute, 0, len(m.Labels))
		for k, v := range m.Labels {
			attrs = append(attrs, otlpAttribute{
				Key:   k,
				Value: otlpAttributeValue{StringValue: v},
			})
		}
		ts := m.Timestamp
		if ts.IsZero() {
			ts = time.Now()
		}
		dp := otlpDataPoint{
			AsDouble:     m.Value,
			TimeUnixNano: fmt.Sprintf("%d", ts.UnixNano()),
			Attributes:   attrs,
		}
		metricsByName[m.Name] = append(metricsByName[m.Name], dp)
	}

	otlpMetrics := make([]otlpMetric, 0, len(metricsByName))
	for name, dps := range metricsByName {
		otlpMetrics = append(otlpMetrics, otlpMetric{
			Name:  name,
			Gauge: &otlpGauge{DataPoints: dps},
		})
	}

	payload := otlpExportMetricsRequest{
		ResourceMetrics: []otlpResourceMetrics{{
			Resource: otlpResource{},
			ScopeMetrics: []otlpScopeMetrics{{
				Scope:   otlpScope{Name: "kubernaut-e2e-test"},
				Metrics: otlpMetrics,
			}},
		}},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal OTLP metrics: %w", err)
	}

	resp, err := http.Post(
		promURL+"/api/v1/otlp/v1/metrics",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("failed to POST OTLP metrics to Prometheus: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Prometheus OTLP endpoint returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// ============================================================================
// OTLP JSON Types (minimal subset for metric injection)
// ============================================================================
//
// These types represent the subset of the OpenTelemetry Metrics JSON schema
// needed for injecting gauge metrics into Prometheus. Only gauge is supported
// because EM test scenarios use instantaneous metric values (e.g., CPU, memory
// at a point in time).
//
// Reference: https://opentelemetry.io/docs/specs/otlp/#otlphttp
// ============================================================================

type otlpExportMetricsRequest struct {
	ResourceMetrics []otlpResourceMetrics `json:"resourceMetrics"`
}

type otlpResourceMetrics struct {
	Resource     otlpResource       `json:"resource"`
	ScopeMetrics []otlpScopeMetrics `json:"scopeMetrics"`
}

type otlpResource struct {
	Attributes []otlpAttribute `json:"attributes,omitempty"`
}

type otlpScopeMetrics struct {
	Scope   otlpScope    `json:"scope"`
	Metrics []otlpMetric `json:"metrics"`
}

type otlpScope struct {
	Name string `json:"name"`
}

type otlpMetric struct {
	Name  string     `json:"name"`
	Gauge *otlpGauge `json:"gauge,omitempty"`
}

type otlpGauge struct {
	DataPoints []otlpDataPoint `json:"dataPoints"`
}

type otlpDataPoint struct {
	AsDouble     float64         `json:"asDouble"`
	TimeUnixNano string          `json:"timeUnixNano"`
	Attributes   []otlpAttribute `json:"attributes,omitempty"`
}

type otlpAttribute struct {
	Key   string             `json:"key"`
	Value otlpAttributeValue `json:"value"`
}

type otlpAttributeValue struct {
	StringValue string `json:"stringValue"`
}

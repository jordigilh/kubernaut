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
// Port Allocation (DD-TEST-001 v3.8):
//   - Prometheus: NodePort 30190, host port 9190
//   - AlertManager: NodePort 30191, host port 9193
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

	// AlertManagerNodePort is the Kind NodePort for AlertManager (DD-TEST-001 v3.8).
	// Moved from 30193 (v2.8) after a "fleet" E2E CI failure ("Service
	// \"alertmanager-svc\" is invalid: spec.ports[0].nodePort: Invalid value:
	// 30193: provided port is already allocated") -- 30193 sat outside every
	// other statically-verified allocation, so the collision was never
	// reproduced from static analysis; relocating to the next verified-free
	// slot in the same 30180-30199 metrics/EM block removes the recurrence
	// risk regardless of root cause. Host port intentionally left at 9193
	// (unchanged) -- NodePort and host port are already decoupled for
	// Prometheus above (30190 vs 9190), so no test using AlertManagerHostPort
	// needs updating.
	//
	// TRACKING (Issue #2014): root cause of the original 30193 collision was
	// never confirmed (working hypothesis: an unrelated Service's
	// dynamically-auto-allocated NodePort, possibly from Istio/Gateway-API
	// machinery, transiently landed on 30193 by chance). If a
	// "provided port is already allocated" error recurs on 30191 or any
	// other statically-pinned NodePort in a Kind-based E2E suite, log it on
	// #2014 -- a second data point would confirm this as a genuinely
	// recurring dynamic-allocation problem (see that issue for suggested
	// next actions: diagnostic Service dump on failure, pinning every
	// Gateway-API-provisioned Service port, or escalating upstream).
	AlertManagerNodePort = 30191
	// AlertManagerHostPort is the host port mapped to the AlertManager NodePort
	AlertManagerHostPort = 9193

	// PrometheusImage is the official Prometheus container image
	PrometheusImage = "prom/prometheus:latest"
	// AlertManagerImage is the official AlertManager container image
	AlertManagerImage = "prom/alertmanager:latest"

	// ClusterLabelKey is the label key Thanos/AlertManager federation uses to
	// identify a fleet alert/metric's source cluster (mirrors
	// pkg/gateway/types.ClusterLabelKey; duplicated here rather than
	// imported to keep this test-infra package dependency-free of
	// production gateway code). Fleet E2E tests simulating a cluster-ID
	// collision against this package's shared Prometheus/AlertManager
	// instance (TESTING_GUIDELINES.md Section 4b, Issue #2274) should use
	// this constant as the TestMetric/TestAlert label key rather than a
	// raw "cluster" string literal.
	ClusterLabelKey = "cluster"
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
  af-severity-grounding.yml: |
    # #1839: AF's severity-triage pipeline (pkg/apifrontend/severity) correlates
    # a kubernaut_remediate target via namespace/kind/name labels (see
    # resolveCreateRRSeverity's TriageInput.Labels), not via the real cAdvisor
    # "pod" label the MemoryExceedsLimit rule above uses -- so that rule can
    # never ground severity for AF-tool-driven RR creation, regardless of its
    # namespace scope. Every fullpipeline test that calls kubernaut_remediate
    # against the shared memory-eater Deployment fixture (fleet cluster_id,
    # interactive/autonomous/streaming, cross-namespace placement -- none of
    # which are testing severity triage itself) needs a real, always-present
    # rule correlating on namespace/kind/name so Tier 2 finds a match and, since
    # this synthetic metric is never actually emitted, falls through to Tier
    # 2.5 (LLM classification informed by real rule context) rather than the
    # removed Tier 3 (LLM classification from zero evidence) or a hard failure.
    # Matches every fullpipeline namespace (all use the "fp-" prefix) so new
    # AF-tool-driven tests are covered automatically without needing their own
    # bespoke alert/metric injection.
    groups:
    - name: af-severity-grounding.rules
      interval: 10s
      rules:
      - alert: MemoryEaterResourcePressure
        expr: memory_eater_grounding_signal{namespace=~"fp-.*", kind="Deployment", name="memory-eater"} > 0
        for: 0s
        labels:
          severity: high
        annotations:
          summary: "memory-eater Deployment resource pressure (AF severity-triage grounding fixture)"
  fleet-interactive-bridge-grounding.yml: |
    # CI RCA (run 30833443049, job 91756267907, E2E-FLEET-018): after Tier 3
    # (pure-LLM severity invention) was removed, AF's kubernaut_remediate and
    # kubernaut_investigate tool calls against the dedicated
    # "ka-interactive-fleet-target" marker Deployment (kubernaut-system
    # namespace, test/e2e/fleet/18_af_ka_interactive_fleet_bridge_test.go)
    # both failed closed with "no active alert or prometheus rule correlates
    # to this resource" -- that fixture is intentionally its OWN dedicated
    # Deployment (not the shared memory-eater fixture above, per the #1839
    # "no fixtures in a shared namespace" precedent), so neither
    # MemoryExceedsLimit (fp-am-.* namespace, memory-eater-.* pod) nor
    # MemoryEaterResourcePressure (name="memory-eater") can ground it.
    # Mirrors AFInvestigateGrounding (test/infrastructure/apifrontend_prometheus_e2e.go):
    # a synthetic vector(1) > 0 expression has no underlying series, so it can
    # never go Prometheus-stale and fires deterministically for the whole
    # suite lifetime. labelsOverlap (pkg/apifrontend/severity/triage.go)
    # explicitly skips the "namespace" label when correlating, matching
    # purely on kind+name here, so the namespace label below is carried for
    # informational parity only.
    groups:
    - name: fleet-interactive-bridge-grounding.rules
      interval: 10s
      rules:
      - alert: KAInteractiveFleetBridgeGrounding
        expr: vector(1) > 0
        for: 0s
        labels:
          severity: warning
          source: prometheus
          namespace: kubernaut-system
          kind: Deployment
          name: ka-interactive-fleet-target
        annotations:
          summary: "Synthetic grounding alert for E2E-FLEET-018 KA interactive-bridge fixture (issue #1768)"
  fleet-alerts-cluster-scoped-2274.yml: |
    # CI RCA (PR #2286, E2E-FLEET-020): AF's kubernaut_list_alerts/
    # get_alert_details tools call pkg/apifrontend/prometheus.Client.GetAlerts,
    # which queries THIS Prometheus's own /api/v1/alerts (alerting-rule-derived
    # active alerts) -- see that client's baseURL, wired from
    # severityTriage.prometheusURL (monitoring.prometheus.url), a config value
    # entirely separate from monitoring.alertManager.url. AF's alert tools
    # never read AlertManager's /api/v2/alerts store at all (confirmed:
    # cmd/apifrontend/backend_deps.go only wires deps.PromClient from
    # prometheusURL). The original E2E-FLEET-020 attempt injected alerts
    # directly into AlertManager (infrastructure.InjectAlerts) and could
    # therefore never be observed by AF, regardless of timing/polling fixes --
    # confirmed against the pre-existing, passing precedent
    # test/e2e/apifrontend/alert_prioritization_e2e_test.go, which grounds
    # AF's list_alerts via a real Prometheus alerting rule instead.
    #
    # Mirrors KAInteractiveFleetBridgeGrounding immediately above: two
    # synthetic vector(1) > 0 alerts (never stale, deterministic, firing from
    # Prometheus startup) sharing every label EXCEPT "cluster", so
    # test/e2e/fleet/19_af_alerts_fleet_scoped_test.go (E2E-FLEET-020) can
    # prove af_alerts.go's cluster_id filter surfaces only the matching
    # cluster's alert via a real AF binary + real Prometheus round-trip.
    groups:
    - name: fleet-alerts-cluster-scoped-2274.rules
      interval: 10s
      rules:
      - alert: Fleet2274MatchingClusterAlert
        expr: vector(1) > 0
        for: 0s
        labels:
          severity: warning
          namespace: fleet-alerts-e2e-ns
          cluster: remote-cluster
        annotations:
          summary: "marker-remote-cluster-visible"
      - alert: Fleet2274CollisionClusterAlert
        expr: vector(1) > 0
        for: 0s
        labels:
          severity: warning
          namespace: fleet-alerts-e2e-ns
          cluster: collision-cluster
        annotations:
          summary: "marker-collision-cluster-hidden"
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
            memory: "256Mi"
            cpu: "200m"
          limits:
            # #1839 RCA: 500m (-> GOMAXPROCS=1 per automaxprocs) was
            # sufficient while the removed Tier 3 pure-LLM fallback silently
            # absorbed every Prometheus timeout in fullpipeline's much larger,
            # longer-running E2E suite (many sequential memory-eater
            # Deployments across many namespaces, all scraped every 10s by
            # the cluster-wide kubelet-cadvisor job, plus rule evaluation
            # every 10s, plus concurrent AF severity-triage API calls). With
            # Tier 3 removed, GetAlerts/GetRules genuinely timing out at the
            # 10s client deadline (Resilience.Prometheus.RequestTimeout) now
            # fails RR creation outright instead of being masked.
            #
            # A first attempt raised this to 1500m, but automaxprocs
            # (go.uber.org/automaxprocs) uses math.Floor on the CPU quota by
            # design (v1.2.0+, to bias against throttling over
            # utilization) -- floor(1.5)=1, so GOMAXPROCS stayed pinned at 1
            # ("determined from CPU quota" in Prometheus's own log) and the
            # timeouts recurred identically. 3000m floors to a genuine 3,
            # giving real multi-core parallelism on this 4-vCPU GH-hosted
            # runner (headroom to spare: it's a ceiling, not a reservation,
            # and the request stays low).
            memory: "768Mi"
            cpu: "3000m"
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
//   - Minimal routing config (all alerts go to gateway-webhook receiver)
//   - Single replica for testing
//   - BR-GATEWAY-036/037: When gatewayToken is non-empty, adds Bearer auth to webhook requests
//
// If this fails with "Service \"alertmanager-svc\" is invalid: ...nodePort:
// ...provided port is already allocated", see Issue #2014 -- this exact
// symptom was seen once (2026-08-08, PR #2001, NodePort 30193) with no
// confirmed root cause. Please add a comment to #2014 with the new
// AlertManagerNodePort value and CI run link so we can tell whether this is
// a recurring dynamic-NodePort-allocation problem.
//
// Parameters:
//   - ctx: Context for cancellation
//   - namespace: Target namespace (e.g., "kubernaut-system")
//   - kubeconfigPath: Path to kubeconfig for kubectl commands
//   - gatewayToken: Bearer token for Gateway signal endpoints (BR-GATEWAY-036/037). If empty, no auth is added.
//   - writer: Output writer for progress logging
func DeployAlertManager(ctx context.Context, namespace, kubeconfigPath, gatewayToken string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "  🔔 Deploying AlertManager in namespace %s...\n", namespace)

	// BR-GATEWAY-036/037: http_config with bearer_token for authenticated Gateway webhooks
	webhookAuthYaml := ""
	if gatewayToken != "" {
		webhookAuthYaml = `
        http_config:
          bearer_token: '` + strings.ReplaceAll(gatewayToken, "'", "''") + `'`
	}

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
      receiver: gateway-webhook
      group_wait: 5s
      group_interval: 5s
      repeat_interval: 1h
    receivers:
    - name: gateway-webhook
      webhook_configs:
      - url: 'http://gateway-service.%[1]s.svc.cluster.local:8080/api/v1/signals/prometheus'
        send_resolved: false`+webhookAuthYaml+`
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
		dumpNodePortAllocations(ctx, kubeconfigPath, writer)
		return fmt.Errorf("failed to deploy AlertManager: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "  ✅ AlertManager deployed (NodePort %d)\n", AlertManagerNodePort)
	return nil
}

// dumpNodePortAllocations lists every Service (all namespaces) with its
// NodePort(s) and writes it to writer. Issue #2014: two "provided port is
// already allocated" collisions (30193, then 30191 after relocation) have
// now been observed with no static claimant found by source-code audit --
// this dump gives the next occurrence a live culprit (the actual Service
// object holding the port at failure time) instead of another negative
// static audit. Best-effort: a dump failure is logged, never promoted to
// the caller's error (the real failure is the original apply, not this).
func dumpNodePortAllocations(ctx context.Context, kubeconfigPath string, writer io.Writer) {
	_, _ = fmt.Fprintln(writer, "  🔎 Issue #2014 diagnostic: dumping all Service NodePort allocations...")
	dumpCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(dumpCtx, "kubectl", "--kubeconfig", kubeconfigPath,
		"get", "svc", "--all-namespaces",
		"-o", `custom-columns=NAMESPACE:.metadata.namespace,NAME:.metadata.name,TYPE:.spec.type,NODEPORTS:.spec.ports[*].nodePort,CREATED:.metadata.creationTimestamp`,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		_, _ = fmt.Fprintf(writer, "  ⚠️  Issue #2014 diagnostic dump failed (non-fatal): %v\n", err)
		return
	}
	_, _ = fmt.Fprintln(writer, "  ── Service NodePort allocations at failure time (Issue #2014) ──")
	_, _ = writer.Write(out)
	_, _ = fmt.Fprintln(writer, "  ── end diagnostic dump ──")
}

// WaitForPrometheusReady polls the Prometheus readiness endpoint until it responds 200 OK.
func WaitForPrometheusReady(ctx context.Context, promURL string, timeout time.Duration, writer io.Writer) error {
	return waitForHTTPReady(ctx, promURL+"/-/ready", "Prometheus", timeout, writer)
}

// WaitForAlertManagerReady polls the AlertManager readiness endpoint until it responds 200 OK.
func WaitForAlertManagerReady(ctx context.Context, amURL string, timeout time.Duration, writer io.Writer) error {
	return waitForHTTPReady(ctx, amURL+"/-/ready", "AlertManager", timeout, writer)
}

func waitForHTTPReady(ctx context.Context, url, serviceName string, timeout time.Duration, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "  ⏳ Waiting for %s to be ready (%s)...\n", serviceName, url)
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 5 * time.Second}

	for time.Now().Before(deadline) {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
		if reqErr != nil {
			return fmt.Errorf("failed to build readiness request: %w", reqErr)
		}
		resp, err := client.Do(req)
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

// WaitForPrometheusCadvisorTarget polls the Prometheus targets API until the
// kubelet-cadvisor scrape job has at least one target in "up" state.
// This detects cadvisor scraping failures early (within seconds of setup)
// rather than surfacing as a mysterious metrics timeout minutes later in the EM.
func WaitForPrometheusCadvisorTarget(ctx context.Context, promURL string, timeout time.Duration, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "  ⏳ Waiting for Prometheus cadvisor scrape target to be UP...\n")
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 5 * time.Second}

	type targetResult struct {
		ActiveTargets []struct {
			ScrapePool string `json:"scrapePool"`
			Health     string `json:"health"`
			ScrapeURL  string `json:"scrapeUrl"`
			LastError  string `json:"lastError"`
		} `json:"activeTargets"`
	}
	type apiResponse struct {
		Status string       `json:"status"`
		Data   targetResult `json:"data"`
	}

	var lastErr string
	for time.Now().Before(deadline) {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, promURL+"/api/v1/targets", http.NoBody)
		if reqErr != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		var result apiResponse
		decodeErr := json.NewDecoder(resp.Body).Decode(&result)
		_ = resp.Body.Close()
		if decodeErr != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		for _, t := range result.Data.ActiveTargets {
			if t.ScrapePool == "kubelet-cadvisor" {
				if t.Health == "up" {
					_, _ = fmt.Fprintf(writer, "  ✅ Prometheus cadvisor target is UP (%s)\n", t.ScrapeURL)
					return nil
				}
				lastErr = t.LastError
				_, _ = fmt.Fprintf(writer, "  ⏳ cadvisor target found but health=%s (error: %s)\n", t.Health, t.LastError)
			}
		}

		time.Sleep(3 * time.Second)
	}

	if lastErr != "" {
		return fmt.Errorf("timeout: cadvisor target never became healthy (last error: %s)", lastErr)
	}
	return fmt.Errorf("timeout: no cadvisor target discovered by Prometheus after %v", timeout)
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
	IsCounter bool              // When true, emit as OTLP Sum (cumulative monotonic) for rate() compatibility
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

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, amURL+"/api/v2/alerts", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to build POST alerts request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to POST alerts to AlertManager: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("AlertManager returned status %d for POST /api/v2/alerts", resp.StatusCode)
	}

	return nil
}

// ResolveActiveAlerts queries AlertManager for all currently active alerts and
// re-injects them with endsAt=now so they transition to "resolved". This prevents
// stale alerts from being batched with newly injected alerts (the Gateway only
// processes Alerts[0] in each AlertManager webhook batch).
func ResolveActiveAlerts(amURL string) error {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, amURL+"/api/v2/alerts?active=true&silenced=false&inhibited=false", http.NoBody)
	if err != nil {
		return fmt.Errorf("failed to build active alerts request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to query active alerts: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("AlertManager returned status %d for GET /api/v2/alerts", resp.StatusCode)
	}

	var alerts []struct {
		Labels map[string]string `json:"labels"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&alerts); err != nil {
		return fmt.Errorf("failed to decode active alerts: %w", err)
	}
	if len(alerts) == 0 {
		return nil
	}

	now := time.Now().UTC().Format(time.RFC3339)
	type resolvedAlert struct {
		Labels map[string]string `json:"labels"`
		EndsAt string            `json:"endsAt"`
	}
	var resolved []resolvedAlert
	for _, a := range alerts {
		resolved = append(resolved, resolvedAlert{Labels: a.Labels, EndsAt: now})
	}

	body, err := json.Marshal(resolved)
	if err != nil {
		return fmt.Errorf("failed to marshal resolved alerts: %w", err)
	}

	postReq, err := http.NewRequestWithContext(context.Background(), http.MethodPost, amURL+"/api/v2/alerts", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to build POST resolved alerts request: %w", err)
	}
	postReq.Header.Set("Content-Type", "application/json")
	postResp, err := http.DefaultClient.Do(postReq)
	if err != nil {
		return fmt.Errorf("failed to POST resolved alerts: %w", err)
	}
	defer func() { _ = postResp.Body.Close() }()

	if postResp.StatusCode != http.StatusOK {
		return fmt.Errorf("AlertManager returned status %d resolving alerts", postResp.StatusCode)
	}
	return nil
}

// HasActiveAlerts returns true if AlertManager has any active (non-silenced,
// non-inhibited) alerts. Useful for polling until stale alerts are fully resolved.
func HasActiveAlerts(amURL string) bool {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, amURL+"/api/v2/alerts?active=true&silenced=false&inhibited=false", http.NoBody)
	if err != nil {
		return true
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return true
	}
	defer func() { _ = resp.Body.Close() }()

	var alerts []json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&alerts); err != nil {
		return true
	}
	return len(alerts) > 0
}

// InjectMetrics injects metric samples into Prometheus via the OTLP HTTP JSON endpoint.
//
// Prometheus must be started with --web.enable-otlp-receiver to accept OTLP metrics.
// Metrics are immediately queryable via PromQL after injection.
//
// This implementation uses the OTLP/HTTP JSON protocol, requiring only net/http and
// encoding/json from the Go standard library (no external dependencies).
//
// Metrics are injected as OTLP Gauge by default. When TestMetric.IsCounter is true,
// the metric is emitted as an OTLP Sum (cumulative, monotonic) so Prometheus stores it
// as a counter type, enabling correct rate() and increase() evaluation.
//
// Parameters:
//   - promURL: Prometheus base URL (e.g., "http://127.0.0.1:9190")
//   - metrics: Slice of test metrics to inject
func InjectMetrics(ctx context.Context, promURL string, metrics []TestMetric) error {
	if len(metrics) == 0 {
		return nil
	}

	type metricGroup struct {
		dataPoints []otlpDataPoint
		isCounter  bool
	}
	groups := make(map[string]*metricGroup)
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
		g, ok := groups[m.Name]
		if !ok {
			g = &metricGroup{isCounter: m.IsCounter}
			groups[m.Name] = g
		}
		g.dataPoints = append(g.dataPoints, dp)
	}

	otlpMetrics := make([]otlpMetric, 0, len(groups))
	for name, g := range groups {
		om := otlpMetric{Name: name}
		if g.isCounter {
			om.Sum = &otlpSum{
				DataPoints:             g.dataPoints,
				AggregationTemporality: 2, // AGGREGATION_TEMPORALITY_CUMULATIVE
				IsMonotonic:            true,
			}
		} else {
			om.Gauge = &otlpGauge{DataPoints: g.dataPoints}
		}
		otlpMetrics = append(otlpMetrics, om)
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, promURL+"/api/v1/otlp/v1/metrics", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to build OTLP metrics request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to POST OTLP metrics to Prometheus: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("prometheus OTLP endpoint returned status %d: %s", resp.StatusCode, string(respBody))
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
	Sum   *otlpSum   `json:"sum,omitempty"`
}

type otlpSum struct {
	DataPoints             []otlpDataPoint `json:"dataPoints"`
	AggregationTemporality int             `json:"aggregationTemporality"`
	IsMonotonic            bool            `json:"isMonotonic"`
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

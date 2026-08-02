# Effectiveness Monitor Service - Observability & Logging

**Version**: v2.0
**Last Updated**: 2026-08-02
**Logging Library**: `github.com/go-logr/logr` (controller-runtime convention), backed by `go.uber.org/zap` via `zapr`

---

## Changelog

| Version | Date | Changes | Reference |
|---------|------|---------|-----------|
| **v2.0** | 2026-08-02 | **#1806 CORRECTION**: Full rewrite. Removed a fictional standalone `zap.NewProductionConfig()` initializer, HTTP-header-based correlation ID middleware, all AI-analysis logging (trigger decisions, KA cost tracking), and 9 fictional Prometheus metrics (`effectiveness_assessment_duration_seconds`, `effectiveness_traditional_score`, `effectiveness_data_availability_weeks`, `effectiveness_insufficient_data_responses_total`, `effectiveness_side_effects_detected_total`, `effectiveness_assessments_total`, `effectiveness_data_storage_query_duration_seconds`, `effectiveness_infrastructure_monitoring_query_duration_seconds`, `effectiveness_circuit_breaker_state`). Replaced with the real controller-runtime `logr.Logger` pattern and the 5 metrics actually registered in `pkg/effectivenessmonitor/metrics/metrics.go`. | [#1806](https://github.com/jordigilh/kubernaut/issues/1806) |
| v1.0 | 2025-10-06 | ⚠️ **STALE (superseded by v2.0)** — Described a fictional stateless HTTP service with `go.uber.org/zap` direct usage, HTTP correlation-ID middleware, and AI cost-tracking logs | — |

---

## Structured Logging

### Logging Pattern: controller-runtime `logr.Logger`

EM does **not** call `zap.NewProductionConfig()` directly. It follows the standard controller-runtime pattern: a `zap.Logger` is constructed once via `sigs.k8s.io/controller-runtime/pkg/log/zap`, installed globally with `ctrl.SetLogger`, and every package obtains a scoped `logr.Logger` from it.

```go
// cmd/effectivenessmonitor/main.go
var setupLog = ctrl.Log.WithName("setup")

func run() int {
    atomicLevel := internalconfig.DefaultLoggingConfig().NewAtomicLevel()
    ctrl.SetLogger(zap.New(zap.Level(atomicLevel)))
    // ...
    setupLog.Info("Starting EffectivenessMonitor Controller",
        "version", version.Version, "gitCommit", version.GitCommit, "buildDate", version.BuildDate)
```

Other components request their own named logger from the same underlying zap sink, e.g. the audit manager:

```go
auditManager := emaudit.NewManager(deps.AuditStore, ctrl.Log.WithName("em-audit"))
```

The reconciler retrieves the contextual logger controller-runtime injects per-reconcile via `log.FromContext(ctx)` — this is the same zap-backed logger, not a separate instance:

```go
// internal/controller/effectivenessmonitor/reconciler.go
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    logger := log.FromContext(ctx)
    // ...
    logger = logger.WithValues(
        "ea", ea.Name,
        "namespace", ea.Namespace,
        "correlationID", ea.Spec.CorrelationID,
        "phase", ea.Status.Phase,
    )
```

Every subsequent log call in that reconcile carries `ea`, `namespace`, `correlationID`, and `phase` automatically — there is no manual field repetition and no HTTP-header-based correlation ID (EM has no HTTP request path to extract one from).

**Log-level hot reload**: `main.go` watches the config file (`hotreload.NewFileWatcher`) and re-applies `logging.level` to the shared `zap.AtomicLevel` without a restart (Issue #875).

---

### Log Levels

| Level | `logr` call | Purpose | Example |
|-------|-------------|---------|---------|
| **Error** | `logger.Error(err, msg, ...)` | Unrecoverable-for-this-reconcile failures | Failed to fetch EA, DataStorage write failure |
| **Info** (V(0)) | `logger.Info(msg, ...)` | Normal state transitions, startup | Phase transitions, "Audit store initialized", assessment completed |
| **Info V(1)** | `logger.V(1).Info(msg, ...)` | Verbose operational detail | "EA in terminal state, skipping reconciliation", audit-store-nil skip notices |
| **Info V(2)** | `logger.V(2).Info(msg, ...)` | Per-event audit trail confirmation | "Component audit event stored", "Assessment scheduled audit event stored" |

There is no `DEBUG`/`WARN` distinction in `logr` — verbosity is controlled via `V(n)`, and warnings are logged as `Info` with descriptive context (e.g. Prometheus/AlertManager unreachable-at-startup messages in `pkg/effectivenessmonitor/startup/readiness.go`) since they are expected, retried conditions rather than errors.

---

### Correlation ID Propagation

There is **no HTTP request path**, so there is no `X-Correlation-ID` header or middleware. Correlation is entirely CRD- and audit-event-based:

1. **Source**: `ea.Spec.CorrelationID` (set once by the RO at EA creation time, equal to `RemediationRequest.Name`)
2. **Logs**: Attached via `logger.WithValues("correlationID", ea.Spec.CorrelationID, ...)` in `Reconcile()` — every subsequent log line in that reconcile inherits it
3. **Audit events**: Attached via `pkgaudit.SetCorrelationID(event, ea.Spec.CorrelationID)` in every `Manager.Record*` call (`pkg/effectivenessmonitor/audit/manager.go`) — this is what lets DataStorage reconstruct a full remediation lifecycle for SOC2 CC8.1
4. **Cluster provenance**: When `ea.Spec.ClusterID` is non-empty (Fleet-managed target), `pkgaudit.SetClusterID` is also attached (DD-AUDIT-003 v2.2, CC8.1)

**No distributed tracing exists** — there is no trace-header propagation, no OpenTelemetry span creation, anywhere in `pkg/effectivenessmonitor/` or `internal/controller/effectivenessmonitor/`.

---

### Example Log Sequence (real fields, illustrative values)

```json
{"level":"info","ts":"2026-08-02T14:05:00.123Z","logger":"em-audit","msg":"EM audit manager initialized (DD-AUDIT-003, Pattern 2)"}
{"level":"info","ts":"2026-08-02T14:05:12.456Z","msg":"Component audit event stored","component":"health","correlationID":"rr-a1b2c3d4","ea":"rr-a1b2c3d4","namespace":"kubernaut-system","phase":"Assessing"}
{"level":"info","ts":"2026-08-02T14:05:12.789Z","msg":"Component audit event stored","component":"alert","correlationID":"rr-a1b2c3d4","ea":"rr-a1b2c3d4","namespace":"kubernaut-system","phase":"Assessing"}
{"level":"info","ts":"2026-08-02T14:05:13.012Z","msg":"Assessment completed audit event stored","correlationID":"rr-a1b2c3d4","reason":"Full","signalName":"KubePodCrashLooping","componentsAssessed":["health","hash","alert","metrics"]}
```

---

## Prometheus Metrics

### Metric Definitions (`pkg/effectivenessmonitor/metrics/metrics.go`)

All 5 metrics follow DD-005 v3.0 Pattern B (full metric names, no `Namespace`/`Subsystem` split) and are registered against the controller-runtime metrics registry (`ctrlmetrics.Registry`) — the same registry `sigs.k8s.io/controller-runtime/pkg/metrics/server` serves on `/metrics`.

```go
const (
    MetricNameComponentAssessmentsTotal = "kubernaut_effectivenessmonitor_component_assessments_total"
    MetricNameComponentScores           = "kubernaut_effectivenessmonitor_component_scores"
    MetricNameAssessmentsCompletedTotal = "kubernaut_effectivenessmonitor_assessments_completed_total"
    MetricNameExternalCallErrors        = "kubernaut_effectivenessmonitor_external_call_errors_total"
    MetricNameValidityExpirationsTotal  = "kubernaut_effectivenessmonitor_validity_expirations_total"
)
```

| Metric | Type | Labels | Purpose |
|--------|------|--------|---------|
| `kubernaut_effectivenessmonitor_component_assessments_total` | Counter | `component` (health\|alert\|metrics\|hash), `result` (success\|error\|skipped) | Count of component assessment completions |
| `kubernaut_effectivenessmonitor_component_scores` | Histogram (buckets `0.0`–`1.0` in `0.1` steps) | `component` | Distribution of component scores |
| `kubernaut_effectivenessmonitor_assessments_completed_total` | Counter | `reason` (full\|partial\|expired\|no_execution\|metrics_timed_out\|spec_drift\|...) | Count of full assessment completions by outcome |
| `kubernaut_effectivenessmonitor_external_call_errors_total` | Counter | `service`, `operation`, `error_type` (timeout\|connection\|http_error) | External dependency call errors |
| `kubernaut_effectivenessmonitor_validity_expirations_total` | Counter | `cluster` (empty = local cluster, BR-FLEET-054) | Assessments that expired before completing |

There are **no** `effectiveness_ai_*`, `effectiveness_data_availability_weeks`, or `insufficient_data_responses_total` metrics — these were fictional artifacts of the never-built HolmesGPT selective-analysis architecture.

### Instrumentation call sites

```go
// pkg/effectivenessmonitor/metrics/metrics.go
func (m *Metrics) RecordComponentAssessment(component, result string, score *float64) {
    m.ComponentAssessmentsTotal.WithLabelValues(component, result).Inc()
    if score != nil {
        m.ComponentScores.WithLabelValues(component).Observe(*score)
    }
}

func (m *Metrics) RecordAssessmentCompleted(reason string)                     { /* ... */ }
func (m *Metrics) RecordExternalCallError(service, operation, errorType string) { /* ... */ }
func (m *Metrics) RecordValidityExpiration(cluster string)                     { /* ... */ }
```

These are invoked from `internal/controller/effectivenessmonitor/assess_components.go` and the completion/expiration handling paths — not from a bespoke HTTP handler wrapper.

### Metrics Endpoint

- **Port**: `9090` (`Controller.MetricsAddr` config, `":9090"`)
- **Path**: `/metrics`
- **Served by**: `sigs.k8s.io/controller-runtime/pkg/metrics/server`, configured in `ctrl.Options{Metrics: metricsserver.Options{BindAddress: cfg.Controller.MetricsAddr}}` (`cmd/effectivenessmonitor/main.go`, `buildManager`)
- **Also exposed via**: a separate `effectivenessmonitor-metrics` ClusterIP `Service` (`charts/kubernaut/templates/effectivenessmonitor/effectivenessmonitor.yaml`), scraped by a `ServiceMonitor` (`servicemonitor.yaml`)

---

## Health Probes

- **Port**: `8081` (`Controller.HealthProbeAddr` config, `":8081"`)
- **Liveness**: `/healthz` → `healthz.Ping` (`mgr.AddHealthzCheck("healthz", healthz.Ping)`)
- **Readiness**: `/readyz` → `healthz.Ping` (`mgr.AddReadyzCheck("readyz", healthz.Ping)`)
- **Fleet readiness** (only when Fleet federation enabled): a third check, `mgr.AddReadyzCheck("fleet", fleetGate.Check)`, backed by `pkg/fleet/readiness.Gate` — fails `/readyz` closed if the MCP Gateway becomes unreachable (Issue #1553, ADR-068, BR-INTEGRATION-065)

```go
// cmd/effectivenessmonitor/main.go
func registerHealthChecks(mgr ctrl.Manager, fleetGate *readiness.Gate) error {
    mgr.AddHealthzCheck("healthz", healthz.Ping)
    mgr.AddReadyzCheck("readyz", healthz.Ping)
    if fleetGate != nil {
        mgr.AddReadyzCheck("fleet", fleetGate.Check)
    }
    return nil
}
```

**Kubernetes probe configuration** (`charts/kubernaut/templates/effectivenessmonitor/effectivenessmonitor.yaml`):

```yaml
readinessProbe:
  httpGet: { path: /readyz, port: 8081 }
  initialDelaySeconds: 5
  periodSeconds: 5
  timeoutSeconds: 5
  failureThreshold: 3
livenessProbe:
  httpGet: { path: /healthz, port: 8081 }
  initialDelaySeconds: 10
  periodSeconds: 10
  timeoutSeconds: 5
  failureThreshold: 3
```

There is also a `startupProbe` on the same port, because EM's fleet `ClusterRegistry` bootstrap against the MCP Gateway can legitimately take longer than the liveness probe's budget under load.

Neither probe checks Prometheus/AlertManager/DataStorage reachability directly (all three are best-effort/retried at query or write time — see [Integration Points](./integration-points.md)); only the Fleet MCP Gateway has a dedicated fail-closed readiness check.

---

## Prometheus Alert Rules (based on the real metrics only)

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: effectivenessmonitor-alerts
  namespace: kubernaut-system
spec:
  groups:
    - name: effectivenessmonitor.critical
      interval: 30s
      rules:
        - alert: EffectivenessMonitorDown
          expr: up{job="effectivenessmonitor-metrics"} == 0
          for: 2m
          labels: { severity: critical, service: effectivenessmonitor }
          annotations:
            summary: "Effectiveness Monitor controller is down"

        - alert: EffectivenessMonitorHighComponentErrorRate
          expr: |
            sum(rate(kubernaut_effectivenessmonitor_component_assessments_total{result="error"}[10m]))
            / sum(rate(kubernaut_effectivenessmonitor_component_assessments_total[10m])) > 0.2
          for: 10m
          labels: { severity: critical, service: effectivenessmonitor }
          annotations:
            summary: "Effectiveness Monitor component assessment error rate above 20%"

    - name: effectivenessmonitor.warnings
      interval: 30s
      rules:
        - alert: EffectivenessMonitorValidityExpirationsRising
          expr: increase(kubernaut_effectivenessmonitor_validity_expirations_total[1h]) > 5
          for: 15m
          labels: { severity: warning, service: effectivenessmonitor }
          annotations:
            summary: "Assessments are expiring before completing (health/metrics/alert never all reported)"

        - alert: EffectivenessMonitorExternalCallErrorsRising
          expr: increase(kubernaut_effectivenessmonitor_external_call_errors_total[15m]) > 10
          for: 5m
          labels: { severity: warning, service: effectivenessmonitor }
          annotations:
            summary: "Rising external call errors (Prometheus/AlertManager/DataStorage) -- see 'service'/'operation' labels"
```

⚠️ These rules are **illustrative examples for this doc**, not a claim that a `PrometheusRule` resource with this exact name is currently deployed — verify against `charts/kubernaut/templates/` before assuming it exists in the cluster.

---

## Grafana Dashboard (suggested panels, based on real metrics)

1. **Component assessment rate**: `sum(rate(kubernaut_effectivenessmonitor_component_assessments_total[5m])) by (component, result)`
2. **Component score distribution**: heatmap of `kubernaut_effectivenessmonitor_component_scores` by `component`
3. **Assessment completion outcomes**: `sum(rate(kubernaut_effectivenessmonitor_assessments_completed_total[1h])) by (reason)`
4. **External call error rate**: `sum(rate(kubernaut_effectivenessmonitor_external_call_errors_total[5m])) by (service, operation, error_type)`
5. **Validity expirations**: `sum(rate(kubernaut_effectivenessmonitor_validity_expirations_total[1h])) by (cluster)`
6. **Controller health**: standard controller-runtime metrics (`controller_runtime_reconcile_total{controller="effectivenessassessment"}`, `controller_runtime_reconcile_errors_total`, `workqueue_depth`) — emitted automatically by controller-runtime, not EM-specific code

---

## Observability Checklist

### Pre-deployment
- [ ] `ctrl.SetLogger` installed before any component logs (verify in `main.go`)
- [ ] All 5 EM metrics visible on `:9090/metrics` after a test assessment
- [ ] `/healthz` and `/readyz` respond on `:8081`
- [ ] `fleet` readyz check registered if Fleet federation is enabled
- [ ] ServiceMonitor scraping the `effectivenessmonitor-metrics` Service

### Runtime monitoring
- [ ] `correlationID` present on every reconcile log line for a given EA
- [ ] Audit events visible in DataStorage queryable by the same `correlationID`
- [ ] `kubernaut_effectivenessmonitor_external_call_errors_total` not climbing unexpectedly
- [ ] `kubernaut_effectivenessmonitor_validity_expirations_total` near zero in steady state

---

## Related Documents

- [Overview](./overview.md)
- [CRD Schema](./crd-schema.md)
- [Integration Points](./integration-points.md)
- [Audit Event Catalog](./security/AUDIT_EVENT_CATALOG.md)
- [ADR-EM-001](../../../architecture/decisions/ADR-EM-001-effectiveness-monitor-service-integration.md)

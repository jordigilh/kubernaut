# DD-EM-005: Cluster-Scoped Metrics and Alert Assessment (Node, PersistentVolume)

**Version**: 1.3
**Date**: 2026-08-24
**Status**: ✅ APPROVED
**Author**: EffectivenessMonitor Team

---

## Context

Issue #193: `assessMetrics()` and `assessAlert()` produce no meaningful signal for cluster-scoped
`SignalTarget` kinds (`Node`, `PersistentVolume`, i.e. `Namespace == ""`).

**Metrics side**: `assessMetrics()` always calls `buildMetricQuerySpecs(ns)`, which builds 5 PromQL
queries filtered by `namespace="%s"`. When `ns == ""` (cluster-scoped), every query matches zero
series, `comparisons` is empty, and the component is unconditionally marked
`Assessed=false, Details="no metric data available for comparison"`
(`internal/controller/effectivenessmonitor/assess_components.go:253-256`).

**Alert side**: `assessAlert()` builds an `alert.AlertContext{AlertName, Namespace}` — `AlertLabels`
is never populated in production code. `buildMatchers()`
(`pkg/effectivenessmonitor/alert/alert.go:126-138`) already loops over `AlertLabels` to add precise
matchers and already omits the namespace matcher when empty, but since `AlertLabels` is always nil,
cluster-scoped alert resolution degrades to alertname-only matching — unable to distinguish
`Node/worker-1` firing `KubeNodeNotReady` from `Node/worker-2` firing the same alert.

### Why a ConfigMap was initially considered, and rejected

The issue's own example PromQL assumed Prometheus queries for cluster-scoped resources are
inherently non-deterministic across environments, implying a per-environment configurable
query template (`MetricTemplates` ConfigMap) would be required.

A due-diligence spike (live queries against a real OCP cluster's `openshift-monitoring` stack,
cross-checked against upstream documentation) found this assumption incorrect for the metrics
this issue targets:

- `kube_node_status_condition` and `kube_persistentvolume_status_phase` /
  `kube_persistentvolume_capacity_bytes` (from `kube-state-metrics`) are K8s-API-driven, not
  subject to scrape-relabeling variance, and keyed deterministically by `node=`/`persistentvolume=`
  matching the resource's `.metadata.name` exactly.
- These metrics and the labels they carry are documented `STABLE` in the upstream
  [`kubernetes/kube-state-metrics`](https://github.com/kubernetes/kube-state-metrics/blob/main/docs/metrics/storage/persistentvolume-metrics.md)
  project — the same unmodified binary vendored by both OCP's `cluster-monitoring-operator` and
  vanilla-k8s's `kube-prometheus-stack` Helm chart. Not an OCP-specific behavior.
- The issue's own example query (assuming PV name == PVC name) was structurally incorrect; the
  correct PV-usage query requires a `label_replace`-based join between `kubelet_volume_stats_used_bytes`
  (PVC-keyed) and `kube_persistentvolume_claim_ref` (PV-keyed) — verified live end-to-end.

Because the required queries are deterministic and vendor-neutral, a ConfigMap-driven
`MetricTemplates` schema would add config surface (new YAML schema, ADR-030 3-layer wiring, CRD
growth) to solve a problem that doesn't exist for the Node/PersistentVolume kinds in scope.

### Why no new AlertManager-side code or config was needed

`AlertContext.AlertLabels` (`pkg/effectivenessmonitor/alert/alert.go:43`) already exists and
`buildMatchers()` already consumes it correctly — it is simply never populated by
`assessAlert()` today. Confirmed empirically (live `ALERTS{alertstate="firing"}` query) that
firing Prometheus alert instances inherit ALL labels from the underlying PromQL query result,
not just the alerting rule's static `labels:` block — so a `KubeNodeNotReady` alert (whose query
is `kube_node_status_condition{...} == 0`) carries `node=<name>` at fire time. This is standard
Prometheus alerting-rule behavior (not OCP-specific): confirmed the `KubeNodeNotReady`,
`KubePersistentVolumeFillingUp`, `KubePersistentVolumeErrors` rules originate from
`kubernetes-monitoring/kubernetes-mixin`, the vendor-neutral rule set used by both OCP and
`kube-prometheus-stack`.

---

## Decision

**Add deterministic, hardcoded Go query builders and a Kind-to-label-key mapping — no new
config, CRD fields, or AlertManager-side code.**

### Metrics: Kind-dispatch in `assessMetrics()`

When `ea.Spec.SignalTarget.Namespace == ""`, dispatch on `ea.Spec.SignalTarget.Kind`:

- `"Node"` → `buildNodeMetricQuerySpecs(name)`: `kube_node_status_condition` for `Ready`
  (inverted: `status="false"`), `MemoryPressure`, `DiskPressure` (all `LowerIsBetter: true`).
- `"PersistentVolume"` → `buildPVMetricQuerySpecs(name)`: `kube_persistentvolume_status_phase`
  for `Failed`/`Pending` (`LowerIsBetter: true`), plus the verified usage-join ratio.
- Any other cluster-scoped `Kind` → unchanged existing fallback (`Assessed=false`,
  `"no metric data available for comparison"`).
- Namespace-scoped path (`ns != ""`) is completely unchanged — still calls
  `buildMetricQuerySpecs(ns)`.

This mirrors the existing pattern: the 5 namespace-scoped queries are themselves hardcoded,
non-configurable Go functions, not YAML config.

### Alert: `AlertLabels` population in `assessAlert()`

A new `clusterScopedAlertLabelKey(kind string) (string, bool)` maps `"Node"` → `"node"` and
`"PersistentVolume"` → `"persistentvolume"` (the exact `kube-state-metrics` / kubernetes-mixin
label keys verified live). When `ea.Spec.SignalTarget.Namespace == ""` and the Kind is
recognized, `assessAlert()` sets `alertCtx.AlertLabels = map[string]string{labelKey: name}`
before calling the existing (unchanged) `alertScorer.Score()`. `buildMatchers()` in
`pkg/effectivenessmonitor/alert/alert.go` requires no changes — it already iterates
`AlertLabels` into precise matchers.

### No changes required to:

- `cmd/effectivenessmonitor/main.go` (no new wiring)
- `internal/config/effectivenessmonitor/config.go` (no new config fields)
- Any CRD type (`EffectivenessAssessment` schema unchanged)
- `pkg/effectivenessmonitor/alert/alert.go` (extension point already existed)

---

## Considered Alternatives

| Approach | Why Discarded |
|---|---|
| ConfigMap-driven `MetricTemplates` per Kind (original proposal) | Solves a non-existent problem: the required Node/PV metrics are deterministic and vendor-neutral (verified live + upstream docs); adds YAML schema, ADR-030 wiring, and maintenance burden with no accuracy benefit |
| New `alertLabelKey` config field per Kind | `AlertContext.AlertLabels` already exists and is already consumed correctly by `buildMatchers()` — the actual gap was that production code never populated it, not that the matching mechanism was insufficient |
| `node_exporter`-based Node CPU/memory as the primary metric source | `node_exporter`'s `instance` label format (hostname vs IP vs custom) varies by scrape-config relabeling across environments — genuinely non-deterministic. `kube_node_status_condition` avoids this by being K8s-API-driven |
| Treat cAdvisor's `node`-labeled `container_cpu_usage_seconds_total` as load-bearing for Node CPU | Verified present on this cluster's kube-prometheus-stack setup, but relabeling-config-dependent (not guaranteed identical across every chart's kubelet ServiceMonitor) — kept as optional/best-effort only, protected by the existing per-query `Available=false` graceful-skip |

---

## Consequences

### Positive

- Zero new config surface: no ConfigMap, CRD field, or `cmd/` wiring changes
- Reuses an existing, already-tested extension point (`AlertContext.AlertLabels`) instead of
  building new AlertManager-matching logic
- Deterministic, vendor-neutral metrics — verified to work identically on OCP and vanilla
  Kubernetes + `kube-prometheus-stack`
- `SignalTarget.Kind` propagation for Node/PersistentVolume already works today (Gateway's
  `APIResourceRegistry.LabelToKind` dynamic K8s API discovery, pre-existing/tested) — no upstream
  changes needed
- No regression risk to the namespace-scoped path or the `#639` golden-string test suite (neither
  is touched)

### Negative

- `buildNodeMetricQuerySpecs`/`buildPVMetricQuerySpecs` are Kind-specific and not extensible to
  arbitrary future cluster-scoped kinds without additional code (acceptable: issue #193 scopes
  to Node + PersistentVolume only; a future cluster-scoped kind would need its own builder,
  matching how the namespace-scoped builder is also not generically extensible)
- `clusterScopedAlertLabelKey` is a small static map, not derived from live API discovery like
  Gateway's `LabelToKind` — acceptable because the label key needed here (matching
  `kube-state-metrics` output label names) is a narrower, different concern than Gateway's
  Kind-detection problem

### Risks

- `kube_persistentvolume_claim_ref` join depends on the label names `name`/`claim_namespace` —
  confirmed live and matches the upstream `kube-state-metrics` docs (`STABLE`), but not
  exhaustively tested against every KSM version in the wild
  - **Mitigation**: code comment citing the verified upstream schema; existing per-query
    `Available=false` graceful-skip means an unexpected schema change degrades to "no data"
    rather than an incorrect score
- Assumes `kube-state-metrics` is deployed alongside Prometheus (near-universal default in
  `kube-prometheus-stack` and OCP, but a technically separate component from bare Prometheus)
  - **Mitigation**: already-existing graceful degradation handles absence the same as any other
    missing metric today; no crash, no incorrect data

---

## v1.1 Addendum: Audit `metric_deltas` Extension (SOC2 CC8.1, FedRAMP AU-3)

### Gap found

v1.0 fixed `Score` computation for Node/PersistentVolume targets (via the query builders and
`AlertLabels` above), but the `effectiveness.metrics.assessed` audit event's `metric_deltas`
sub-object remained **empty** for these Kinds: `populateMetricsAssessResult`
(`internal/controller/effectivenessmonitor/assess_components.go`) only recognized the 5
namespace-scoped `metricQuerySpec.Name` values (`cpu_utilization`, `memory_utilization`,
`http_request_duration_p95_ms`, `http_error_rate`, `http_throughput_rps`), so the 6 new
cluster-scoped query names introduced in v1.0 (`kube_node_status_condition_ready`,
`..._memorypressure`, `..._diskpressure`, `kube_persistentvolume_status_phase_failed`,
`..._pending`, `kubelet_volume_stats_used_bytes_ratio`) fell through with no mapping.

This is a gap against:

- **SOC2 CC8.1** (complete audit-trail reconstruction): a Node/PV remediation's audit trail could
  not be reconstructed with the same metric-level detail as a namespace-scoped remediation.
- **FedRAMP AU-3** (structured content of audit records): the audit record's `metric_deltas`
  sub-object silently omitted data that was actually queried and available.

Decision and full pipeline design discussed in
[issue #193 comment](https://github.com/jordigilh/kubernaut/issues/193#issuecomment-4908176229).

### Fix: additive named field pairs (Proposal A), full pipeline

Added 6 new before/after field pairs to both the raw audit schema
(`EffectivenessAssessmentAuditPayloadMetricDeltas`, nullable `OptNilFloat64`) and the DataStorage
projection schema (`RemediationMetricDeltas`, `OptFloat64`) in `api/openapi/data-storage-v1.yaml`:
`node_not_ready_{before,after}`, `node_memory_pressure_{before,after}`,
`node_disk_pressure_{before,after}`, `pv_phase_failed_{before,after}`,
`pv_phase_pending_{before,after}`, `pv_usage_ratio_{before,after}`.

Wired end-to-end through all 3 services, mirroring the exact precedent of commit `21e592475`
(the original Phase A -> Phase B metric_deltas field extension):

`metricQuerySpec.Name` (EM) -> `populateMetricsAssessResult` -> `metricsAssessResult` struct ->
`emitMetricsEvent` -> `MetricsAssessedData` / `RecordMetricsAssessed` (EM audit) -> `metric_deltas`
audit table column -> `mapMetricDeltas` (DataStorage) -> `RemediationMetricDeltas` projection ->
`ds_adapter.go mapMetricDeltas` (Kubernaut Agent) -> `enrichment.MetricDeltas` DTO ->
`FormatMetricDeltas` (LLM prompt text).

**Opportunistic backfill**: while tracing the pipeline, found `throughput_before_rps`/
`throughput_after_rps` had been present in the raw audit payload and EM producer since
`21e592475`, but were never propagated to `RemediationMetricDeltas`, `enrichment.MetricDeltas`,
or `FormatMetricDeltas`. Backfilled in the same change (flagged separately in the issue comment
as pre-existing and unrelated to #193, but touching the same files/functions).

Rejected alternative: a generic `entries: [{name, before, after}]` array structure (Proposal B) —
would have required a breaking migration of all 5 existing Phase A/B fields to avoid two
audit-record shapes co-existing, with no accuracy or extensibility benefit over the additive
named-pair pattern already proven twice in this codebase (Phase A->B, and now this v1.1 extension).

### Control mapping

| Control | Requirement | How this change satisfies it |
|---|---|---|
| SOC2 CC8.1 | Complete audit-trail reconstruction | `effectiveness.metrics.assessed` events for Node/PV targets now carry the same metric-level detail as namespace-scoped targets; proven end-to-end by `IT-EM-193-001/002` querying the audit trail back via `QueryAuditEvents` and asserting `metric_deltas` content |
| FedRAMP AU-3 | Structured content of audit records | New fields follow the existing typed-sub-object OpenAPI schema pattern (`OptNilFloat64`), not an untyped/free-form extension |
| FedRAMP AU-2 | Audit events capture the actions that were actually taken/observed | Fields are populated only when the corresponding PromQL query succeeds (`Available=true`); absent/failed queries leave the field unset rather than a misleading zero value |
| BR-KA-016 | Audit data usably reaches LLM remediation-history context, not just the audit store | The v1.1 UTs (`UT-RH-LOGIC-025/026`, `UT-KA-433W-014`) proved per-layer mapping in isolation only; v1.2 closes the pyramid-invariant gap by extending the pre-existing real-DS `IT-KA-433-ENR-008` to prove these fields survive the actual EM->DS->KA HTTP wire |

### Wiring manifest (v1.1 addendum)

| Component | Production Entry Point | Test ID |
|---|---|---|
| `metric_deltas` / `RemediationMetricDeltas` schemas (+6 pairs, +throughput backfill) | `api/openapi/data-storage-v1.yaml` | `make gen-diff` (CI gate) |
| `populateMetricsAssessResult` (+6 switch cases) | `assess_components.go` | UT-EM-193-008..010 |
| `MetricsAssessedData` / `RecordMetricsAssessed` (+6 Opt-wraps) | `pkg/effectivenessmonitor/audit/manager.go` | UT-EM-AM-013..016 |
| `emitMetricsEvent` (+6 passthrough) | `internal/controller/effectivenessmonitor/events.go` | IT-EM-193-001/002 (extended, asserts real audit-trail content via `QueryAuditEvents`) |
| `mapMetricDeltas` (DataStorage, +6 + throughput) | `pkg/datastorage/server/remediation_history_logic.go` | UT-RH-LOGIC-025/026; IT-KA-433-ENR-008 (extended, exercises this function via a real HTTP round-trip) |
| `enrichment.MetricDeltas` DTO + `ds_adapter.go mapMetricDeltas` (+6 + throughput) | `internal/kubernautagent/enrichment/` | UT-KA-433W-014; IT-KA-433-ENR-008 (extended, real DS+HTTP wiring proof) |
| `FormatMetricDeltas` (+6 + throughput rendering) | `internal/kubernautagent/prompt/history.go` | UT-KA-433-HP-003 (extended) |

---

## v1.3 Addendum: Fleet Cluster-Scoping (Issue #2274, BR-FLEET-054)

### Gap found

v1.0-1.2 made EM's cluster-scoped-**Kind** (Node/PersistentVolume) queries deterministic and
audit-complete, but neither those queries nor the namespace-scoped path (`buildMetricQuerySpecs`)
nor the AlertManager query (`buildMatchers`) ever filtered on the **fleet cluster** a remediation
actually ran against. In a fleet (multi-cluster) deployment, Thanos/Prometheus federates metrics
from every managed cluster into one queryable view, distinguished only by an external `cluster`
label; AlertManager similarly aggregates alerts fleet-wide. Without a `cluster=` matcher:

- **Metrics**: `sum(container_memory_working_set_bytes{namespace="prod"})` (and all 4 sibling
  namespace-scoped queries, plus the Node/PV cluster-scoped queries from v1.0) silently blend
  series from every fleet cluster that happens to report a same-named `namespace`/`node`/
  `persistentvolume` — a remote-cluster remediation's before/after comparison could be corrupted
  by an unrelated cluster's concurrent metric movement for a same-named resource.
- **Alerts**: `buildMatchers` filtered on `alertname`(+`namespace`/`AlertLabels`) only. A
  same-correlation-ID alert definition firing on a *different* fleet cluster could mask (or
  falsely confirm) resolution of the alert that actually triggered this remediation.

This is a gap against:

- **BR-FLEET-054** (fleet cluster routing/scoping — the same business requirement `ReaderFor`
  satisfies for K8s API reads; EM's Prometheus/AlertManager reads had no equivalent).
- **SOC2 CC7.2** (monitoring/reconstruction assurance): a remediation's effectiveness score could
  not be trusted to reflect only that remediation's cluster.
- **FedRAMP AU-3** (structured content of audit records): `metric_deltas`/alert audit fields could
  silently carry cross-cluster-contaminated values with no indication in the record itself.

Root cause of why this escaped the fleet E2E lane for as long as it did, and the hardening applied
to close that gap, are documented in the [Issue #2274](https://github.com/jordigilh/kubernaut/issues/2274)
investigation and `test/e2e/fleet/07_em_fleet_metrics_test.go` (E2E-FLEET-019a/b).

### Fix: `ea.Spec.ClusterID` is the Thanos `cluster` external label

No new lookup or CRD field was needed: `ea.Spec.ClusterID` (already populated end-to-end from the
originating Prometheus/AlertManager webhook's `cluster` label, see
`pkg/gateway/types/fingerprint.go` `ClusterLabelKey = "cluster"` and
`pkg/gateway/adapters/prometheus_adapter.go`) **is** the exact Thanos external label value to
match on — spike-confirmed, eliminating the need for a `ClusterInfo`/`ReaderFor`-style registry
lookup that the issue's initial framing assumed.

- Every PromQL query builder (`buildMetricQuerySpecs`, `buildNodeMetricQuerySpecs`,
  `buildPVMetricQuerySpecs`, `buildKSMFlagQuerySpec`) now takes a `clusterID string` parameter,
  appending a `cluster="<id>"` matcher via the new `clusterMatcherSuffix` helper when non-empty.
- `buildPVMetricQuerySpecs`'s usage-ratio query joins 3 metrics via
  `on(namespace, persistentvolumeclaim)`/`on(persistentvolume)` — vector-matching operators that
  only restrict matching on the *listed* labels, so `cluster=` must be applied to **all three**
  raw selectors independently, not just the outer expression (spike finding).
- `alert.AlertContext` gained a `ClusterID` field; `buildMatchers` appends a `cluster=` matcher
  when non-empty, mirroring the existing `Namespace`/`AlertLabels` pattern.
- Empty `ClusterID` (hub/local, non-fleet remediations) produces byte-identical queries to
  pre-#2274 behavior in every builder — proven by `UT-EM-2274-BC-001..004` / `IT-EM-2274-BC-005`.

### Control mapping

| Control | Requirement | How this change satisfies it |
|---|---|---|
| BR-FLEET-054 | Fleet remediations are scoped to their originating cluster | PromQL/AlertManager queries built by EM now carry the same `cluster=`/`ClusterID` scoping already used by `pkg/fleet.ReaderFor` for K8s API reads |
| SOC2 CC7.2 | Monitoring provides reasonable assurance of remediation reconstruction | A remote-cluster remediation's effectiveness score can no longer be corrupted by a same-named resource's metrics/alerts on a different fleet cluster |
| FedRAMP AU-3 | Structured content of audit records | No new audit schema needed — `metric_deltas`/alert fields already recorded; this fix ensures the *source query* that produced them is cluster-isolated |

### Wiring manifest (v1.3 addendum)

| Component | Production Entry Point | Test ID |
|---|---|---|
| `buildMetricQuerySpecsForTarget`/`buildMetricQuerySpecs`/`buildNodeMetricQuerySpecs`/`buildPVMetricQuerySpecs`/`buildKSMFlagQuerySpec` (+`clusterID` param) | `assess_components.go` `assessMetrics` (passes `ea.Spec.ClusterID`) | UT-EM-2274-001..005, UT-EM-2274-BC-001..003 |
| `AlertContext.ClusterID` + `buildMatchers` (+cluster matcher) | `assess_components.go` `assessAlert` (passes `ea.Spec.ClusterID`) | UT-EM-2274-006/007 |
| Full reconcile wiring (real envtest + mock Prometheus/AlertManager) | `internal/controller/effectivenessmonitor/reconciler.go` `Reconcile` | IT-EM-2274-001..003, IT-EM-2274-BC-005 |
| Real Prometheus/AlertManager, real EM controller, fleet Kind cluster | `test/e2e/fleet/07_em_fleet_metrics_test.go` | E2E-FLEET-019a (metrics collision), E2E-FLEET-019b (alert-resolution collision) |

---

## Related Decisions

- **DD-EM-003**: Dual-target assessment — noted cluster-scoped resources as a "neutral
  consequence" where namespace-scoped queries "operate at cluster scope when namespace is
  empty"; DD-EM-005 corrects this — an empty namespace does not make the existing
  namespace-filtered PromQL queries operate at cluster scope, it makes them match zero series.
- **BR-EM-002**: Alert resolution check via AlertManager API — extended to cluster-scoped targets
- **BR-EM-003**: Prometheus metric comparison — extended to cluster-scoped targets
- **Issue #193**: EM cluster-scoped resource assessment gap (this decision's origin)
- **Issue #639**: EM metrics query golden-string tests — confirmed unaffected (only asserts
  against the untouched namespace-scoped `buildMetricQuerySpecs`)

---

## Document Maintenance

| Date | Version | Changes |
|------|---------|---------|
| 2026-07-07 | 1.0 | Initial decision: Kind-dispatch metric query builders + `AlertLabels` population for cluster-scoped (Node, PersistentVolume) targets. No new config/CRD surface. |
| 2026-07-07 | 1.1 | Audit `metric_deltas` extension: 6 new field pairs for cluster-scoped Node/PersistentVolume metrics, wired full-pipeline (EM -> DataStorage -> Kubernaut Agent), closing a SOC2 CC8.1 / FedRAMP AU-3 audit-completeness gap. Opportunistic backfill of a pre-existing `throughput_before_rps`/`after_rps` propagation gap in the same files. |
| 2026-07-07 | 1.2 | Pyramid invariant closure: v1.1's DataStorage/Kubernaut-Agent wiring points had only isolated per-layer UT coverage (`UT-RH-LOGIC-025/026`, `UT-KA-433W-014`), no IT proving the real EM->DS->KA wire. Extended the pre-existing real-DS `IT-KA-433-ENR-008` (already the authoritative wiring proof for Phase A `metric_deltas` fields) to also seed/assert the v1.1 fields, closing the gap without adding a new test suite. |
| 2026-08-24 | 1.3 | Fleet cluster-scoping (Issue #2274, BR-FLEET-054): every PromQL/AlertManager query builder now accepts `clusterID` (= `ea.Spec.ClusterID`, the Thanos `cluster` external label) and appends a `cluster=` matcher, preventing a remote-cluster remediation's metrics/alert assessment from being corrupted by a same-named resource on a different fleet cluster. Empty `ClusterID` (hub/local) is byte-identical to pre-#2274 behavior. Also rebuilt the fleet E2E lane's `07_em_fleet_metrics_test.go` from pure infra-reachability smoke checks into genuine behavioral cluster-collision tests (E2E-FLEET-019a/b), closing the gap that let this bug ship undetected. |

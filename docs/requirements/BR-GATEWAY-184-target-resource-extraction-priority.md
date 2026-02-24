# BR-GATEWAY-184: Target Resource Extraction Priority Order

**Status**: ✅ APPROVED
**Version**: 2.0.0
**Date**: 2026-02-17
**Owner**: Gateway Service Team
**Stakeholders**: SRE Team, AI Analysis Team, Demo/QA Team
**GitHub Issues**: [#178](https://github.com/jordigilh/kubernaut/issues/178), [#191](https://github.com/jordigilh/kubernaut/issues/191) (v2.0: monitoring metadata label filtering)

---

## Executive Summary

Gateway's Prometheus adapter MUST check specific Kubernetes resource labels before the generic `pod` label when extracting the target resource from AlertManager webhooks. The `pod` label in kube-state-metrics resource-level alerts (e.g., `kube_hpa_*`, `kube_deployment_*`) points to the metrics exporter pod, not the affected resource. Incorrect extraction causes the entire remediation pipeline (SP, AA, WFE) to target the wrong resource.

**Business Value**: Ensures correct target resource identification for all Prometheus-sourced signals, enabling accurate LLM investigation and workflow selection across all supported Kubernetes resource types. v2.0 adds monitoring metadata label filtering (Issue #191) and corrects the Job resource label from `job` to `job_name`.

---

## Business Need

### Problem Statement

Prometheus kube-state-metrics exposes metrics for Kubernetes resources (HPAs, Deployments, StatefulSets, PDBs, PVCs, etc.). Alerts based on these metrics include a `pod` label injected by Prometheus target discovery that points to the kube-state-metrics exporter pod (e.g., `kube-prometheus-stack-kube-state-metrics-abc123`), **not** the affected resource.

When Gateway's target extraction checks `pod` first (v1.x: `extractResourceKind`; v2.0: `extractTargetResource`), it misidentifies the target:

- **Expected**: `HorizontalPodAutoscaler/api-frontend` (from `horizontalpodautoscaler` label)
- **Actual**: `Pod/kube-prometheus-stack-kube-state-metrics-abc123` (from `pod` label)

This causes the LLM to investigate the wrong resource, find it healthy, and conclude "self-resolved" -- wasting the entire remediation cycle.

### Affected Scenarios

Any Prometheus alert originating from kube-state-metrics resource-level metrics where both a specific resource label and a `pod` label are present. This includes but is not limited to:

- `KubeHpaMaxedOut` (HPA)
- `KubeDeploymentReplicasMismatch` (Deployment)
- `KubeStatefulSetReplicasMismatch` (StatefulSet)
- `KubePodDisruptionBudgetAtLimit` (PDB)
- `KubePersistentVolumeFillingUp` (PVC)

---

## Functional Requirements

### FR-1: Resource Kind Extraction Priority Order

Gateway MUST extract the target resource kind from Prometheus alert labels using the following priority order (first match wins):

| Priority | Label Key                  | Extracted Kind              | Notes |
|----------|----------------------------|-----------------------------|-------|
| 1        | `horizontalpodautoscaler`  | `HorizontalPodAutoscaler`   | |
| 2        | `poddisruptionbudget`      | `PodDisruptionBudget`       | |
| 3        | `persistentvolumeclaim`    | `PersistentVolumeClaim`     | |
| 4        | `deployment`               | `Deployment`                | |
| 5        | `statefulset`              | `StatefulSet`               | |
| 6        | `daemonset`                | `DaemonSet`                 | |
| 7        | `node`                     | `Node`                      | |
| 8        | `service`                  | `Service`                   | Subject to FR-5 filtering |
| 9        | `job_name`                 | `Job`                       | v2.0: Changed from `job` (see FR-6) |
| 10       | `cronjob`                  | `CronJob`                   | |
| 11       | `pod`                      | `Pod`                       | |
| 12       | (none matched)             | `Unknown`                   | |

**Rationale**: For `kube_pod_*` metrics, the `pod` label IS the correct target (it is the metric's subject). For resource-level metrics (`kube_hpa_*`, `kube_deployment_*`, etc.), `pod` is injected by Prometheus target discovery and points to the metrics exporter. Checking specific resource labels first ensures the correct target is extracted in both cases.

### FR-2: Resource Name Extraction Priority Order

Gateway MUST extract the target resource name using the same label priority order as FR-1. The resource name MUST come from the same label that determined the resource kind.

### FR-3: Backward Compatibility

Alerts with only a `pod` label (no specific resource labels) MUST continue to extract `Pod` as the resource kind and the pod name as the resource name. This preserves existing behavior for pod-level metrics (`kube_pod_container_status_restarts_total`, `kube_pod_status_phase`, etc.).

### FR-4: Excluded Scrape Metadata Labels (v2.0)

The following Prometheus scrape configuration labels MUST be excluded from the target resource candidate list entirely. They are injected by Prometheus and always refer to the scraper/scrape target, never to the affected workload resource:

- `job`: The Prometheus scrape job name (e.g., `"kube-state-metrics"`). Not to be confused with `job_name` which is the actual Kubernetes Job resource name.
- `endpoint`: The ServiceMonitor endpoint name (e.g., `"http"`, `"metrics"`).
- `instance`: The scrape target address (e.g., `"10.244.0.5:8443"`).

**Rationale**: These labels are always present in Prometheus alerts and always refer to monitoring infrastructure. Excluding them from the candidate list prevents false matches without requiring heuristic filtering. See Issue #191, SME review.

### FR-5: Monitoring Metadata Label Filtering (v2.0)

When a `LabelFilter` is configured, the `service` label (priority 8) MUST be checked against known monitoring infrastructure naming patterns before being accepted as a target resource. If the filter determines the value refers to monitoring infrastructure (e.g., `"kube-prometheus-stack-kube-state-metrics"`), that candidate is skipped and extraction continues to the next priority level.

Known naming patterns (SME-approved, Issue #191):
- **Substrings**: `prometheus`, `kube-state-metrics`, `alertmanager`, `grafana`, `thanos`, `exporter`
- **Prefixes**: `victoria`, `loki`, `jaeger`
- **Suffixes**: `-operator`

When no `LabelFilter` is configured (nil), the `service` label passes through without filtering (backward-compatible).

**Rationale**: The `service` label in Prometheus alerts often refers to the Kubernetes Service fronting the metrics exporter (e.g., `kube-prometheus-stack-kube-state-metrics`), not the workload's own Service. Pattern-based filtering covers 90%+ of monitoring stack deployments. The LLM's `affectedResource` field provides a safety net for edge cases. See Issue #191 for SME review.

### FR-6: job_name Semantics for Kubernetes Jobs (v2.0)

Gateway MUST use the `job_name` label (not `job`) to identify Kubernetes Job resources. The `job` label in kube-state-metrics alerts is the Prometheus scrape job name (see FR-4), while `job_name` is the actual Kubernetes Job resource name emitted by kube-state-metrics for Job-related metrics (`kube_job_status_succeeded`, `kube_job_failed`, etc.).

---

## Downstream Impact

### Signal Fingerprint

The fingerprint formula `SHA256(namespace:kind:name)` will produce different hashes for previously misidentified alerts. This is correct and desired -- the old fingerprints targeted the wrong resource and should not be deduplicated against new, correctly-targeted signals.

### HAPI Prompt (No Changes Required)

The HAPI prompt builder (`holmesgpt-api/src/extensions/incident/prompt_builder.py`) receives `resource_kind` and `resource_name` from the AA spec and passes them directly to the LLM. The LLM investigates whatever resource it is told about using `kubectl` tools. The prompt output schema already instructs the LLM to trace up OwnerReferences for the `affectedResource` field. No prompt changes are needed -- the fix is fully contained in the Gateway.

### Signal Processing, AI Analysis, Workflow Execution

No changes required. These services consume the `ResourceIdentifier` from the RemediationRequest CRD. Once Gateway populates it correctly, the entire pipeline operates on the right target.

---

## Implementation

**Files**:
- `pkg/gateway/adapters/prometheus_adapter.go` — `extractTargetResource` (unified, replaces `extractResourceKind`/`extractResourceName`), `resourceCandidates` priority list
- `pkg/gateway/adapters/label_filter.go` — `LabelFilter` interface, `monitoringMetadataFilter` implementation
- `cmd/gateway/main.go` — Wiring: `NewMonitoringMetadataFilter(logger)` injected into `NewPrometheusAdapter`

---

## Test Coverage

See [Test Plan: ISSUE-191](../testing/ISSUE-191/TEST_PLAN.md) for the complete test plan including unit tests (GW-RE-01 to GW-RE-14) and integration tests (IT-GW-184-001 to IT-GW-184-004).

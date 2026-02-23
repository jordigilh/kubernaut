# BR-GATEWAY-184: Target Resource Extraction Priority Order

**Status**: ✅ APPROVED
**Version**: 1.0.0
**Date**: 2026-02-23
**Owner**: Gateway Service Team
**Stakeholders**: SRE Team, AI Analysis Team, Demo/QA Team
**GitHub Issue**: [#178](https://github.com/jordigilh/kubernaut/issues/178)

---

## Executive Summary

Gateway's Prometheus adapter MUST check specific Kubernetes resource labels before the generic `pod` label when extracting the target resource from AlertManager webhooks. The `pod` label in kube-state-metrics resource-level alerts (e.g., `kube_hpa_*`, `kube_deployment_*`) points to the metrics exporter pod, not the affected resource. Incorrect extraction causes the entire remediation pipeline (SP, AA, WFE) to target the wrong resource.

**Business Value**: Ensures correct target resource identification for all Prometheus-sourced signals, enabling accurate LLM investigation and workflow selection across all supported Kubernetes resource types.

---

## Business Need

### Problem Statement

Prometheus kube-state-metrics exposes metrics for Kubernetes resources (HPAs, Deployments, StatefulSets, PDBs, PVCs, etc.). Alerts based on these metrics include a `pod` label injected by Prometheus target discovery that points to the kube-state-metrics exporter pod (e.g., `kube-prometheus-stack-kube-state-metrics-abc123`), **not** the affected resource.

When Gateway's `extractResourceKind` checks `pod` first, it misidentifies the target:

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

| Priority | Label Key                  | Extracted Kind              |
|----------|----------------------------|-----------------------------|
| 1        | `horizontalpodautoscaler`  | `HorizontalPodAutoscaler`   |
| 2        | `poddisruptionbudget`      | `PodDisruptionBudget`       |
| 3        | `persistentvolumeclaim`    | `PersistentVolumeClaim`     |
| 4        | `deployment`               | `Deployment`                |
| 5        | `statefulset`              | `StatefulSet`               |
| 6        | `daemonset`                | `DaemonSet`                 |
| 7        | `node`                     | `Node`                      |
| 8        | `service`                  | `Service`                   |
| 9        | `job`                      | `Job`                       |
| 10       | `cronjob`                  | `CronJob`                   |
| 11       | `pod`                      | `Pod`                       |
| 12       | (none matched)             | `Unknown`                   |

**Rationale**: For `kube_pod_*` metrics, the `pod` label IS the correct target (it is the metric's subject). For resource-level metrics (`kube_hpa_*`, `kube_deployment_*`, etc.), `pod` is injected by Prometheus target discovery and points to the metrics exporter. Checking specific resource labels first ensures the correct target is extracted in both cases.

### FR-2: Resource Name Extraction Priority Order

Gateway MUST extract the target resource name using the same label priority order as FR-1. The resource name MUST come from the same label that determined the resource kind.

### FR-3: Backward Compatibility

Alerts with only a `pod` label (no specific resource labels) MUST continue to extract `Pod` as the resource kind and the pod name as the resource name. This preserves existing behavior for pod-level metrics (`kube_pod_container_status_restarts_total`, `kube_pod_status_phase`, etc.).

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

**File**: `pkg/gateway/adapters/prometheus_adapter.go`
**Functions**: `extractResourceKind`, `extractResourceName`

---

## Test Coverage

| Test ID   | Description                                       | Type |
|-----------|---------------------------------------------------|------|
| GW-RE-01  | HPA takes priority over pod                       | Unit |
| GW-RE-02  | Deployment takes priority over pod                | Unit |
| GW-RE-03  | StatefulSet takes priority over pod               | Unit |
| GW-RE-04  | PDB takes priority over pod                       | Unit |
| GW-RE-05  | Pod-only alerts still extract Pod (backward compat)| Unit |
| GW-RE-06  | PVC recognized as valid resource kind             | Unit |
| GW-RE-07  | Job takes priority over pod                       | Unit |
| GW-RE-08  | CronJob takes priority over pod                   | Unit |

**Test File**: `test/unit/gateway/adapters/prometheus_adapter_test.go`

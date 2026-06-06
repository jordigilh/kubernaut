# DD-AF-003: Alert Label Taxonomy and Investigation Scoping Analysis

**Status**: APPROVED
**Decision Date**: 2026-06-06
**Version**: 1.0
**Confidence**: 95%
**Applies To**: Gateway (GW), API Frontend (AF) -- Alert ingestion and investigation pipeline

---

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-06-06 | Architecture Team | Initial analysis: alert label taxonomy, scoped investigation evaluation, GW cluster-scope fix identified. |

---

## Context & Problem

### Question Raised

After implementing #1367 (AF alert toolset) and #1369 (namespace/cluster-scoped alert correlation in severity triage), the question arose: should `kubernaut_investigate` be split into three scoped tools (`kubernaut_investigate_workload`, `kubernaut_investigate_namespace`, `kubernaut_investigate_cluster`) to handle alerts at different Kubernetes scopes?

### Current Architecture

The entire investigation pipeline is resource-scoped by design:

```
kubernaut_investigate(namespace, kind, name)
    -> HandleCreateRR(triager)
        -> triager.Triage() -> bestAlertMatch() -> GetAlerts()
            -> deriveSignalName()
    -> RR CRD (spec.targetResource: {kind, name, namespace?})
        -> SP -> AA -> KA (SignalContext: ResourceKind, ResourceName, Namespace)
```

Every downstream consumer -- SignalProcessing enrichment, AIAnalysis Rego policies, KA investigator, effectiveness assessment -- expects a concrete K8s object identified by `kind + name` (with `namespace` optional for cluster-scoped resources).

### Investigation Question

Does the alert label taxonomy in real-world Prometheus deployments produce alerts where both `namespace` and resource-identifying labels are absent, requiring a fundamentally different investigation entry point?

---

## Analysis

### How Prometheus Alert Labels Are Produced

Alert labels come from three sources:

1. **Metric labels** -- preserved from the scraped metric based on the PromQL `by()` clause
2. **Static labels** -- defined in the alerting rule's `labels:` block
3. **External labels** -- from the Prometheus server configuration

Prometheus scrapes metrics from **targets** (pods, nodes, service endpoints). Kubernetes service discovery (`kubernetes_sd_configs`) attaches metadata labels to scraped metrics, including `namespace` and `pod` for pod targets.

### Alert Label Taxonomy

Analysis of kube-prometheus-stack, OpenShift cluster-monitoring-operator, and 37 Kubernaut demo scenarios reveals three categories:

#### Category 1: Namespace + Resource Labels (Most Common)

Alerts about namespaced workloads. Both `namespace` and at least one resource-identifying label (e.g., `pod`, `deployment`, `horizontalpodautoscaler`) are present.

| Alert | Labels | GW Resolution |
|-------|--------|---------------|
| `KubePodCrashLooping` | `namespace`, `pod` | Pod -> owner -> Deployment |
| `KubeHpaMaxedOut` | `namespace`, `horizontalpodautoscaler` | HPA |
| `KubePodDisruptionBudgetAtLimit` | `namespace`, `poddisruptionbudget` | PDB |
| `KubePodSchedulingFailed` | `namespace`, `pod` | Pod -> owner -> Deployment |

**GW handling**: `extractNamespace` finds `namespace` label. `extractTargetResource` maps a resource label to a K8s kind via `APIResourceRegistry.LabelToKind`. Owner resolution walks `ownerReferences` to find the top-level controller. RR created successfully.

#### Category 2: Resource Label, No Namespace (Node/Cluster-Scoped)

Alerts about cluster-scoped K8s resources. A resource-identifying label exists (e.g., `node`), but no `namespace` label because the resource is cluster-scoped.

| Alert | Labels | GW Resolution |
|-------|--------|---------------|
| `KubeNodeNotReady` | `node`, `severity` | Node |
| `NodeFilesystemSpaceFillingUp` | `node`, `device` | Node |
| `NodeClockSkewDetected` | `node` | Node |

**Source**: `kube_node_status_condition` from kube-state-metrics. The metric has labels `node`, `condition`, `status` -- **no `namespace`** because Node is a cluster-scoped K8s resource (confirmed by [kube-state-metrics documentation](https://github.com/kubernetes/kube-state-metrics/blob/main/docs/metrics/cluster/node-metrics.md)).

**GW handling**: `extractNamespace` finds no `namespace` label and falls back to `"default"`. `extractTargetResource` resolves `node` label to `Kind: "Node"`. RR created with `targetResource.namespace: "default"`.

**BUG IDENTIFIED**: The namespace `"default"` is semantically incorrect for Node. The CRD schema explicitly supports empty namespace for cluster-scoped resources (`targetResource.namespace` is `+optional`). GW should set namespace to `""` when the resolved kind is cluster-scoped.

#### Category 3: Infrastructure Component Alerts

Alerts about platform infrastructure (etcd, API server). These are scraped from pods running in specific namespaces.

| Alert | PromQL | Metric Source | Labels on Fired Alert |
|-------|--------|---------------|----------------------|
| `etcdHighCommitDurations` | `histogram_quantile(0.99, rate(etcd_disk_backend_commit_duration_seconds_bucket{job=~".*etcd.*"}[5m]))` | etcd pods in `openshift-etcd` | `instance`, `job` (no `by()` clause, so all scrape labels preserved including `pod`, `namespace`) |
| `KubeAPIErrorBudgetBurn` | `sum(rate(apiserver_request_total...))` | API server pods | Depends on `by()` grouping |

**Key finding**: When the PromQL expression does not aggregate away labels (no `sum by(...)` or `without(...)`), `histogram_quantile` preserves all labels from the underlying metric, including `pod` and `namespace` from the scrape target. Infrastructure component alerts typically retain `instance` and often `pod`/`namespace` because losing them would make the alert unactionable (you couldn't tell which instance is affected).

Rules that explicitly aggregate with `by(job)` or `without(instance)` may lose `pod`/`namespace`, but retain `job`. The `job` label alone is insufficient for GW resolution (it's in `PrometheusReservedLabels`).

### Key Finding: Alerts Always Have Either Namespace OR a Resource Label

No well-designed Prometheus alerting rule produces an alert with **both** `namespace` and all resource-identifying labels absent. This is because:

1. **Namespaced workload metrics** inherently carry `namespace` from Kubernetes service discovery
2. **Cluster-scoped resource metrics** (Node, PV) carry resource-identifying labels (`node`, `persistentvolume`) because kube-state-metrics emits them by design
3. **Infrastructure component metrics** (etcd, API server) are scraped from pods in namespaces; labels are preserved unless explicitly aggregated away
4. **Aggregating away all identifying labels is an anti-pattern** -- it makes the alert unactionable because you can't identify which instance is affected

### Demo Scenario Validation

All 37 Kubernaut demo scenarios and 97 golden transcripts confirm this taxonomy. The `node-notready` scenario explicitly demonstrates the Category 2 case by injecting `namespace: demo-compute` as a static label workaround for GW's `"default"` fallback.

---

## Decision

### 1. Scoped Investigation Tools: NOT NEEDED

The proposal to split `kubernaut_investigate` into `kubernaut_investigate_workload`, `kubernaut_investigate_namespace`, and `kubernaut_investigate_cluster` is **not needed** because:

- Every real-world Prometheus alert carries at least one resource-identifying label that GW can resolve to a K8s kind
- The AF path (`kubernaut_investigate`) requires `namespace/kind/name` from the LLM, which the LLM can always determine by calling `list_alerts` + `kubectl_list` to identify affected resources
- The severity triage pipeline (#1369) already correlates namespace-scoped and cluster-scoped alerts as lower-priority fallback tiers, providing alert context even when the investigation target is a specific workload
- The `list_alerts` and `get_alert_details` tools (#1367) provide the LLM with ad-hoc alert visibility independent of the investigation flow

### 2. GW Cluster-Scope Fix: REQUIRED

GW's `extractNamespace` must be updated to handle cluster-scoped resources correctly:

**Current behavior** (bug):
```
Alert: {alertname: "KubeNodeNotReady", node: "worker-2", severity: "critical"}
extractNamespace() -> "default"  (WRONG: Node has no namespace)
RR.spec.targetResource: {kind: "Node", name: "worker-2", namespace: "default"}
```

**Required behavior**:
```
Alert: {alertname: "KubeNodeNotReady", node: "worker-2", severity: "critical"}
extractNamespace() -> ""  (after checking that resolved kind is cluster-scoped)
RR.spec.targetResource: {kind: "Node", name: "worker-2", namespace: ""}
```

**Implementation**: After `extractTargetResource` resolves the kind, check whether the kind is cluster-scoped via `APIResourceRegistry`. If cluster-scoped AND no `namespace`/`exported_namespace` label exists in the alert, set namespace to `""` instead of `"default"`. This distinction must happen during alert parsing, before the webhook response is sent to Alertmanager.

**Downstream compatibility**: Already supported.
- CRD schema: `targetResource.namespace` is `+optional`
- Gateway `validateResourceInfo`: explicitly documents empty namespace for cluster-scoped resources
- SignalProcessing controller: handles `isClusterScoped := sp.Spec.Signal.TargetResource.Namespace == ""`
- KA OpenAPI: `resource_namespace` accepts empty string

---

## Alert Label Reference

### Prometheus Reserved Labels (GW: excluded from kind resolution)

```go
var PrometheusReservedLabels = map[string]bool{
    "job": true, "service": true, "instance": true,
    "endpoint": true, "container": true,
    "namespace": true,
}
```

### kube-state-metrics: Cluster-Scoped Resource Metrics

| Metric | Labels | Namespace? |
|--------|--------|------------|
| `kube_node_status_condition` | `node`, `condition`, `status` | No |
| `kube_node_info` | `node`, `kernel_version`, ... | No |
| `kube_persistentvolume_status_phase` | `persistentvolume` | No |
| `kube_namespace_status_phase` | `namespace` | Yes (is the namespace itself) |

### kube-state-metrics: Namespaced Resource Metrics

| Metric | Labels | Namespace? |
|--------|--------|------------|
| `kube_pod_status_phase` | `namespace`, `pod` | Yes |
| `kube_deployment_status_replicas` | `namespace`, `deployment` | Yes |
| `kube_hpa_status_current_replicas` | `namespace`, `hpa` | Yes |

---

## Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Custom alerting rule with no resource labels | Low | Alert dropped by GW | Documentation: recommend including at least one resource label in custom rules |
| GW cluster-scope fix breaks existing RR fingerprints | Low | Dedup mismatches during rollout | Fingerprint includes kind+name; namespace change from "default" to "" affects existing dedup keys. Mitigate with a one-time fingerprint migration or accept brief dedup window |
| AF LLM cannot determine target for cluster-scoped alert | Low | LLM falls back to broader investigation | `list_alerts` provides context; LLM prompt instructs use of kubectl to find affected resources |

---

## Related

- **#1367** -- AF Alertmanager/Thanos alerts toolset (list_alerts, get_alert_details)
- **#1369** -- Severity triager namespace-scoped and cluster-scoped alert correlation
- **DD-AF-001** -- Pod-based alert correlation for severity triage
- **GW extractNamespace** -- `pkg/gateway/adapters/prometheus_adapter.go:457-474`
- **GW extractTargetResource** -- `pkg/gateway/adapters/prometheus_adapter.go:393-454`
- **CRD targetResource schema** -- `config/crd/bases/kubernaut.ai_remediationrequests.yaml:177-224`

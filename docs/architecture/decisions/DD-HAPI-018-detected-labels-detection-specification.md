# DD-HAPI-018: DetectedLabels Detection Specification

**Status**: APPROVED
**Decision Date**: 2026-02-12
**Version**: 1.3
**Confidence**: 96%
**Applies To**: HolmesGPT API (Python, authoritative implementation per ADR-056)

---

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-02-12 | Architecture Team | Initial specification extracted from SP reference implementation (`pkg/signalprocessing/detection/labels.go`). Cross-language contract for HAPI Python reimplementation. |
| 1.1 | 2026-02-20 | Architecture Team | Update consumer guidance: detected labels are now both used as DS query filters AND surfaced to the LLM as read-only `cluster_context` in `list_available_actions` response. `failedDetections` fields excluded from both uses. See ADR-056 v1.3, DD-HAPI-017 v1.3. |
| 1.2 | 2026-02-25 | Architecture Team | Add non-workload target context building (Issue #196). When the RCA target is a PodDisruptionBudget, `_build_k8s_context` resolves pod context from the PDB's selector. Node target deferred to Issue #203. New K8s client method: `list_pods_by_selector`. New conformance vectors: DL-HP-10, DL-EC-04. |
| 1.3 | 2026-02-24 | Architecture Team | Add ArgoCD v3 annotation-based tracking support (Issue #218). ArgoCD v3.x defaults to `argocd.argoproj.io/tracking-id` annotation instead of v2's `argocd.argoproj.io/instance` label. Updated GitOps detection precedence table, input contract (`deploymentDetails.annotations`), and conformance vectors (DL-HP-11 through DL-HP-14). Removed stale reference to SP Go implementation (removed per ADR-056). |

---

## Context & Problem

### Current State

DetectedLabels auto-detection is implemented in HolmesGPT API (HAPI) in Python (`holmesgpt-api/src/detection/labels.py`). Per ADR-056, this computation runs post-RCA against the actual remediation target resource (identified by the LLM) rather than the signal source, because the signal and the root cause may be different resources with different GitOps/infrastructure characteristics. The original SP Go reference implementation (`pkg/signalprocessing/detection/labels.go`) has been removed.

### Problem

Without a formal, language-agnostic specification, the two implementations (Go in SP, Python in HAPI) may diverge on:
- Which annotations/labels trigger each detection
- Precedence order when multiple sources are present
- The distinction between "resource absent" (valid false) and "query failed" (unknown)
- K8s API resources queried and RBAC requirements

### Business Requirements

- **BR-SP-101**: DetectedLabels Auto-Detection -- 7 cluster characteristics
- **BR-SP-103**: FailedDetections Tracking -- RBAC, timeout, network errors
- **BR-HAPI-250/252**: DetectedLabels in workflow search

---

## Decision

Establish this document as the authoritative, language-agnostic specification for DetectedLabels detection. Both SP's Go implementation and HAPI's Python implementation MUST conform to this specification. Where the specification and an implementation disagree, the specification is authoritative.

---

## Output Schema

All implementations MUST produce a `DetectedLabels` object with the following fields:

| Field | Type | JSON Key | Description |
|-------|------|----------|-------------|
| FailedDetections | string[] | `failedDetections` | Field names where detection QUERY failed (RBAC, timeout, network). Empty if all succeeded. |
| GitOpsManaged | bool | `gitOpsManaged` | True if managed by a GitOps controller |
| GitOpsTool | string | `gitOpsTool` | `"argocd"`, `"flux"`, or `""` |
| PDBProtected | bool | `pdbProtected` | True if a PodDisruptionBudget matches this workload |
| HPAEnabled | bool | `hpaEnabled` | True if an HPA targets this workload |
| Stateful | bool | `stateful` | True if owner chain contains a StatefulSet |
| HelmManaged | bool | `helmManaged` | True if managed by Helm |
| NetworkIsolated | bool | `networkIsolated` | True if any NetworkPolicy exists in namespace |
| ServiceMesh | string | `serviceMesh` | `"istio"`, `"linkerd"`, or `""` |

Valid values for `FailedDetections` entries: `gitOpsManaged`, `pdbProtected`, `hpaEnabled`, `stateful`, `helmManaged`, `networkIsolated`, `serviceMesh`.

---

## Input Contract

### Input 1: KubernetesContext

Resource metadata for the target resource. Implementations obtain this from their respective K8s clients.

| Field | Type | Used By |
|-------|------|---------|
| `namespace` | string | PDB, HPA, NetworkPolicy queries |
| `podDetails.name` | string | Reference only |
| `podDetails.labels` | map[string]string | PDB selector matching |
| `podDetails.annotations` | map[string]string | GitOps (ArgoCD), ServiceMesh (Istio, Linkerd) |
| `deploymentDetails.name` | string | HPA target matching |
| `deploymentDetails.labels` | map[string]string | GitOps (ArgoCD v2, Flux), Helm |
| `deploymentDetails.annotations` | map[string]string | GitOps (ArgoCD v3 tracking-id) |
| `namespaceLabels` | map[string]string | GitOps (ArgoCD, Flux) |
| `namespaceAnnotations` | map[string]string | GitOps (ArgoCD, Flux) |

### Input 2: OwnerChain

Ordered list of owner references from the target resource up to the root owner.

| Field | Type | Used By |
|-------|------|---------|
| `kind` | string | Stateful detection (StatefulSet check), HPA target matching |
| `name` | string | HPA target matching |
| `namespace` | string | Reference only |

### HAPI-Specific Input Mapping

HAPI computes labels for the **RCA target resource** (not the signal source). The `get_resource_context` tool already resolves the root owner (owner chain traversal). HAPI MUST fetch the following via the Kubernetes Python client for the target resource:

1. **Pod details**: `GET /api/v1/namespaces/{ns}/pods/{name}` -- labels and annotations
2. **Deployment details**: `GET /apis/apps/v1/namespaces/{ns}/deployments/{name}` -- labels and annotations (v1.3: annotations added for ArgoCD v3 tracking-id)
3. **Namespace metadata**: `GET /api/v1/namespaces/{ns}` -- labels and annotations
4. **Owner chain**: Already resolved by `get_resource_context`

---

## Detection Specifications

### Detection 1: GitOps Management

**Fields**: `gitOpsManaged` (bool), `gitOpsTool` (string)
**K8s API Call**: None -- uses metadata already fetched
**FailedDetections**: Not applicable (no K8s API query)

**Detection Logic** (MUST follow this precedence order -- first match wins):

| Priority | Source | Annotation/Label Key | Result | Notes |
|----------|--------|---------------------|--------|-------|
| 1 | Pod annotations | `argocd.argoproj.io/tracking-id` | `gitOpsManaged=true`, `gitOpsTool="argocd"` | ArgoCD v3 default (v1.3) |
| 2 | Pod annotations | `argocd.argoproj.io/instance` | `gitOpsManaged=true`, `gitOpsTool="argocd"` | ArgoCD v2 custom label config |
| 3 | Deployment labels | `fluxcd.io/sync-gc-mark` | `gitOpsManaged=true`, `gitOpsTool="flux"` | |
| 4 | Deployment labels | `argocd.argoproj.io/instance` | `gitOpsManaged=true`, `gitOpsTool="argocd"` | ArgoCD v2 custom label config |
| 5 | Deployment annotations | `argocd.argoproj.io/tracking-id` | `gitOpsManaged=true`, `gitOpsTool="argocd"` | ArgoCD v3 default (v1.3) |
| 6 | Namespace labels | `argocd.argoproj.io/instance` | `gitOpsManaged=true`, `gitOpsTool="argocd"` | |
| 7 | Namespace labels | `fluxcd.io/sync-gc-mark` | `gitOpsManaged=true`, `gitOpsTool="flux"` | |
| 8 | Namespace annotations | `argocd.argoproj.io/tracking-id` | `gitOpsManaged=true`, `gitOpsTool="argocd"` | ArgoCD v3 default (v1.3) |
| 9 | Namespace annotations | `argocd.argoproj.io/managed` | `gitOpsManaged=true`, `gitOpsTool="argocd"` | |
| 10 | Namespace annotations | `fluxcd.io/sync-status` | `gitOpsManaged=true`, `gitOpsTool="flux"` | |

**ArgoCD version compatibility** (v1.3):
- **ArgoCD v2** (label-based tracking, custom config): Sets `argocd.argoproj.io/instance` as a label. Detected at priorities 2, 4, 6.
- **ArgoCD v3** (annotation-based tracking, default): Sets `argocd.argoproj.io/tracking-id` as an annotation on all managed resources. Detected at priorities 1, 5, 8. The tracking-id value has the format `<app-name>:<group>/<kind>:<namespace>/<name>`.
- **ArgoCD v3 annotation+label mode**: Sets both `argocd.argoproj.io/tracking-id` annotation and `app.kubernetes.io/instance` label. Detected at priority 1 (tracking-id wins).

**Default** (no match): `gitOpsManaged=false`, `gitOpsTool=""`

**Key rule**: Presence of the key is sufficient. The value is not inspected (any non-empty value matches).

**Null safety**: If `podDetails` is nil/None, skip priorities 1-2. If `deploymentDetails` is nil/None, skip priorities 3-5. If `namespaceLabels` is nil/None, skip priorities 6-7. If `namespaceAnnotations` is nil/None, skip priorities 8-10.

---

### Detection 2: PDB Protection

**Field**: `pdbProtected` (bool)
**K8s API Call**: `List policy/v1 PodDisruptionBudget` in namespace
**RBAC Required**: `get`, `list` on `poddisruptionbudgets` in `policy` API group
**Cache TTL**: 5 minutes (PDBs are relatively static)
**FailedDetections**: `"pdbProtected"` on query failure

**Detection Logic**:

1. If `podDetails` is nil/None OR pod has no labels: `pdbProtected=false`, return (no error)
2. List all PDBs in namespace via K8s API
3. For each PDB with a non-nil `spec.selector`:
   a. Convert `spec.selector` (LabelSelector) to a label selector
   b. If selector matches pod labels: `pdbProtected=true`, return
4. No matching PDB found: `pdbProtected=false`, return (no error)

**Error handling**: If the List API call fails (RBAC denied, timeout, network error): `pdbProtected=false`, add `"pdbProtected"` to `FailedDetections`.

---

### Detection 3: HPA Enabled

**Field**: `hpaEnabled` (bool)
**K8s API Call**: `List autoscaling/v2 HorizontalPodAutoscaler` in namespace
**RBAC Required**: `get`, `list` on `horizontalpodautoscalers` in `autoscaling` API group
**Cache TTL**: 1 minute (HPAs change more frequently than PDBs)
**FailedDetections**: `"hpaEnabled"` on query failure

**Detection Logic**:

1. List all HPAs in namespace via K8s API
2. For each HPA, examine `spec.scaleTargetRef`:
   a. **Direct match**: If `deploymentDetails` is present and `scaleTargetRef.kind == "Deployment"` and `scaleTargetRef.name == deploymentDetails.name`: `hpaEnabled=true`, return
   b. **Owner chain match**: For each entry in `ownerChain`, if `scaleTargetRef.kind == entry.kind` and `scaleTargetRef.name == entry.name`: `hpaEnabled=true`, return
3. No matching HPA found: `hpaEnabled=false`, return (no error)

**Error handling**: If the List API call fails: `hpaEnabled=false`, add `"hpaEnabled"` to `FailedDetections`.

---

### Detection 4: Stateful Workload

**Field**: `stateful` (bool)
**K8s API Call**: None -- uses owner chain data
**FailedDetections**: Not applicable (no K8s API query)

**Detection Logic**:

1. For each entry in `ownerChain`:
   a. If `entry.kind == "StatefulSet"`: `stateful=true`, return
2. No StatefulSet in chain: `stateful=false`

---

### Detection 5: Helm Managed

**Field**: `helmManaged` (bool)
**K8s API Call**: None -- uses deployment labels
**FailedDetections**: Not applicable (no K8s API query)

**Detection Logic**:

1. If `deploymentDetails` is nil/None: `helmManaged=false`, return
2. If deployment label `app.kubernetes.io/managed-by` exists and equals `"Helm"` (case-sensitive): `helmManaged=true`, return
3. If deployment label `helm.sh/chart` exists (any value): `helmManaged=true`, return
4. Default: `helmManaged=false`

---

### Detection 6: Network Isolation

**Field**: `networkIsolated` (bool)
**K8s API Call**: `List networking/v1 NetworkPolicy` in namespace
**RBAC Required**: `get`, `list` on `networkpolicies` in `networking.k8s.io` API group
**Cache TTL**: 5 minutes (NetworkPolicies are relatively static)
**FailedDetections**: `"networkIsolated"` on query failure

**Detection Logic**:

1. List all NetworkPolicies in namespace via K8s API
2. If count > 0: `networkIsolated=true`
3. If count == 0: `networkIsolated=false`

**Note**: This checks for the existence of any NetworkPolicy in the namespace, not whether a specific policy targets the workload. Any policy presence implies network isolation enforcement.

**Error handling**: If the List API call fails: `networkIsolated=false`, add `"networkIsolated"` to `FailedDetections`.

---

### Detection 7: Service Mesh

**Field**: `serviceMesh` (string)
**K8s API Call**: None -- uses pod annotations
**FailedDetections**: Not applicable (no K8s API query)

**Detection Logic** (first match wins):

1. If `podDetails` is nil/None OR pod has no annotations: `serviceMesh=""`, return
2. If pod annotation `sidecar.istio.io/status` exists (any value): `serviceMesh="istio"`, return
3. If pod annotation `linkerd.io/proxy-version` exists (any value): `serviceMesh="linkerd"`, return
4. Default: `serviceMesh=""`

**Note**: Istio is checked before Linkerd. If both annotations are present, Istio takes precedence.

---

## FailedDetections Contract

This section defines the critical distinction between "resource absent" and "detection failed."

### Semantics

| Scenario | Field Value | FailedDetections | Meaning |
|----------|-------------|------------------|---------|
| PDB exists, matches pod | `pdbProtected=true` | `[]` | Workload is PDB-protected -- use for filtering |
| No PDB in namespace | `pdbProtected=false` | `[]` | No PDB protection -- use for filtering |
| PDB query RBAC denied | `pdbProtected=false` | `["pdbProtected"]` | Unknown -- DO NOT use for filtering |

### Rules

1. Only K8s API-based detections (PDB, HPA, NetworkPolicy) can produce FailedDetections entries
2. Annotation/label-based detections (GitOps, Stateful, Helm, ServiceMesh) NEVER produce FailedDetections entries because they use already-fetched metadata
3. Consumers MUST check FailedDetections before trusting a `false` value. A `false` with the field in FailedDetections means "unknown," not "absent"
4. A `true` value is always trustworthy regardless of FailedDetections (the query succeeded and found a match)

### Consumer Guidance (for HAPI workflow discovery)

Detected labels have two consumer paths (ADR-056 v1.3, DD-HAPI-017 v1.3):

1. **DataStorage query filters**: Labels are injected as `detected_labels` query parameter into all three workflow discovery endpoints (list actions, list workflows, get workflow). This filters the workflow catalog to return only workflows whose `detectedLabels` requirements match the target resource.

2. **LLM-facing `cluster_context`** (v1.1): Labels are surfaced to the LLM as a read-only `cluster_context` section in the `list_available_actions` tool response. This gives the LLM explicit infrastructure context for informed action type selection (e.g., "this is ArgoCD-managed, so GitRevertCommit is the right action type"). The LLM cannot set or override these labels -- they are computed by HAPI and injected into the response.

**FailedDetections exclusion** (applies to BOTH consumer paths):
- Fields listed in `failedDetections` MUST be excluded from DataStorage workflow discovery query filters (DD-WORKFLOW-001 v2.1)
- Fields listed in `failedDetections` MUST be excluded from the LLM-facing `cluster_context.detected_labels` object
- The `failedDetections` array itself MUST NOT appear in the LLM-facing `cluster_context`
- Use `strip_failed_detections()` (in `workflow_discovery.py`) to apply this exclusion consistently

**Reference implementation**: `holmesgpt-api/src/toolsets/workflow_discovery.py` (`_build_context_params` for DS filters, `ListAvailableActionsTool._invoke` for `cluster_context` injection)

---

## Execution Order

Detections MUST execute in this order. API-based detections are independent and MAY execute concurrently, but the overall sequence ensures consistent FailedDetections accumulation.

1. GitOps detection (no API call)
2. PDB detection (API call)
3. HPA detection (API call)
4. Stateful detection (no API call)
5. Helm detection (no API call)
6. NetworkPolicy detection (API call)
7. ServiceMesh detection (no API call)

Detections 2, 3, and 6 are the only ones that make K8s API calls. They MUST NOT short-circuit on failure -- all detections MUST be attempted regardless of prior failures. This ensures maximum information collection even when some queries fail.

---

## Caching Strategy

API-based detections SHOULD use a TTL cache keyed by namespace to reduce K8s API server load.

| Detection | Cache Key | TTL | Rationale |
|-----------|-----------|-----|-----------|
| PDB | namespace | 5 minutes | PDBs are created during deployment, rarely change |
| HPA | namespace | 1 minute | HPAs may adjust more frequently (scaling events) |
| NetworkPolicy | namespace | 5 minutes | NetworkPolicies are relatively static |

**HAPI note**: Caching is per-investigation in HAPI (each `investigate_issues()` call is independent). Within a single investigation, the `get_resource_context` tool typically runs once, so caching has limited benefit. However, if HAPI processes multiple investigations concurrently, a shared cache across investigations for the same namespace reduces API load. This is an implementation optimization, not a conformance requirement.

---

## Non-Workload Target Context Building (v1.2)

The original specification assumes workload resources (Pod, Deployment, StatefulSet) as the RCA target. ADR-056 notes that ~30-40% of RCAs identify a resource outside the signal source's owner chain (e.g., Node, PodDisruptionBudget, ConfigMap). This section defines how `_build_k8s_context` MUST handle non-workload targets so that label detection can still produce meaningful results.

### PodDisruptionBudget as RCA Target

**Scenario**: The LLM identifies a PodDisruptionBudget as the root cause (e.g., "PDB is blocking drain/rollout"). The PDB itself does not have pod labels, but it has a `spec.selector` that selects pods.

**Context Building Logic**:

1. Find the target PDB: List PDBs in the target namespace and match by name.
2. Read the PDB's `spec.selector.matchLabels`.
3. If matchLabels is non-empty: List pods in the same namespace matching those labels via `list_pods_by_selector(namespace, matchLabels)`.
4. If matching pods are found: Populate `pod_details` from the first matched pod (labels, annotations). This allows PDB detection to correctly match the PDB's selector against the pod's labels.
5. If no pods match or the PDB has no selector: `pod_details` remains absent. PDB detection will return `pdbProtected=false` per Detection 2 rules.

**K8s API Call**: `List core/v1 Pod` with label selector (new method: `list_pods_by_selector`)
**RBAC Required**: `list` on `pods` in `""` (core) API group (already granted in existing RBAC)

### Node as RCA Target

**Deferred to Issue #203** (v1.1 milestone). Node targets are cluster-scoped with no namespace and no owner chain. All namespace-scoped detections (PDB, HPA, NetworkPolicy) would need a strategy for selecting the relevant namespace (e.g., from the alert's namespace). The current behavior (`pdbProtected=false`, all-default labels) is safe and conservative.

---

## Conformance Test Vectors

HAPI's test suite MUST pass the following test vectors. Test IDs are from the authoritative HAPI test suite (`holmesgpt-api/tests/unit/test_label_detector.py`).

### Happy Path Vectors

| Test ID | Input | Expected Output |
|---------|-------|-----------------|
| DL-HP-01 | Pod annotation `argocd.argoproj.io/instance: my-app` | `gitOpsManaged=true`, `gitOpsTool="argocd"` |
| DL-HP-02 | Deployment label `fluxcd.io/sync-gc-mark: sha256:abc123` | `gitOpsManaged=true`, `gitOpsTool="flux"` |
| DL-HP-03 | PDB in namespace with selector matching pod labels `{app: api}` | `pdbProtected=true` |
| DL-HP-04 | HPA with `scaleTargetRef: {kind: Deployment, name: api-deployment}` | `hpaEnabled=true` |
| DL-HP-05 | Owner chain contains `{kind: StatefulSet, name: db}` | `stateful=true` |
| DL-HP-06 | Deployment label `app.kubernetes.io/managed-by: Helm` | `helmManaged=true` |
| DL-HP-07 | NetworkPolicy `deny-all` exists in namespace | `networkIsolated=true` |
| DL-HP-08 | Pod annotation `sidecar.istio.io/status: {"version":"1.18.0"}` | `serviceMesh="istio"` |
| DL-HP-09 | Pod annotation `linkerd.io/proxy-version: stable-2.14.0` | `serviceMesh="linkerd"` |

### ArgoCD v3 Vectors (v1.3)

| Test ID | Input | Expected Output |
|---------|-------|-----------------|
| DL-HP-11 | Pod annotation `argocd.argoproj.io/tracking-id: app:apps/Deployment:ns/name` (ArgoCD v3) | `gitOpsManaged=true`, `gitOpsTool="argocd"` |
| DL-HP-12 | Deployment annotation `argocd.argoproj.io/tracking-id: app:apps/Deployment:ns/name` (ArgoCD v3, no pod annotations) | `gitOpsManaged=true`, `gitOpsTool="argocd"` |
| DL-HP-13 | Namespace annotation `argocd.argoproj.io/tracking-id: app:v1/Namespace:ns` (ArgoCD v3, no pod/deploy markers) | `gitOpsManaged=true`, `gitOpsTool="argocd"` |
| DL-HP-14 | Pod annotation `argocd.argoproj.io/tracking-id` AND deployment label `argocd.argoproj.io/instance` (v3+v2 coexist) | `gitOpsManaged=true`, `gitOpsTool="argocd"` (v3 tracking-id wins at priority 1) |

### Non-Workload Target Vectors (v1.2)

| Test ID | Input | Expected Output |
|---------|-------|-----------------|
| DL-HP-10 | RCA target is PDB `my-pdb` with `selector.matchLabels: {app: api}`. Pods in namespace have `labels: {app: api}`. A PDB exists whose selector matches those pod labels. | `pdbProtected=true` (pod context resolved from PDB selector) |

### Mutual Exclusivity Vectors (v1.3, BR-HAPI-254)

> **Constraint**: `gitOpsTool` MUST resolve to exactly one value. When multiple GitOps indicators coexist on a resource, the highest-precedence match wins. In practice, ArgoCD + Flux on the same namespace is extremely unlikely, but the spec must define deterministic behavior for every case.

| Test ID | Input | Expected Output |
|---------|-------|-----------------|
| DL-MX-01 | Pod annotation `argocd.argoproj.io/tracking-id` (v3) + Deployment label `fluxcd.io/sync-gc-mark` (Flux) | `gitOpsTool="argocd"` (pod v3 annotation at priority 1 beats deployment Flux label at priority 3) |
| DL-MX-02 | Pod annotation `argocd.argoproj.io/instance` (v2) + Deployment label `fluxcd.io/sync-gc-mark` (Flux) | `gitOpsTool="argocd"` (pod v2 annotation at priority 2 beats deployment Flux label at priority 3) |
| DL-MX-03 | No ArgoCD on pod + Deployment label `fluxcd.io/sync-gc-mark` (Flux) + Namespace label `argocd.argoproj.io/instance` (ArgoCD v2) | `gitOpsTool="flux"` (deployment Flux label at priority 3 beats namespace ArgoCD label at priority 6) |
| DL-MX-04 | Pod annotations with both `argocd.argoproj.io/tracking-id` (v3) AND `argocd.argoproj.io/instance` (v2) + Deployment labels/annotations with both | `gitOpsTool="argocd"` (v3 and v2 both resolve to `"argocd"` — version is internal, not consumer-facing) |

### Edge Case Vectors

| Test ID | Input | Expected Output |
|---------|-------|-----------------|
| DL-EC-01 | Plain deployment, no special annotations/labels/resources | All fields `false`/empty, `failedDetections=[]` |
| DL-EC-02 | Nil/None KubernetesContext | Return nil/None (no DetectedLabels object) |
| DL-EC-03 | ArgoCD + PDB + HPA all present simultaneously | `gitOpsManaged=true`, `pdbProtected=true`, `hpaEnabled=true` |
| DL-EC-04 | RCA target is PDB with no `spec.selector` or no matching pods | `pdbProtected=false`, `failedDetections=[]` (no query failure, just no matching pods) |

### Error Handling Vectors

| Test ID | Input | Expected Output |
|---------|-------|-----------------|
| DL-ER-01 | PDB query returns RBAC forbidden | `pdbProtected=false`, `failedDetections=["pdbProtected"]` |
| DL-ER-02 | HPA query returns timeout | `hpaEnabled=false`, `failedDetections=["hpaEnabled"]` |
| DL-ER-03 | PDB + HPA + NetworkPolicy queries all fail | `failedDetections=["pdbProtected", "hpaEnabled", "networkIsolated"]` |
| DL-ER-04 | Context cancellation during HPA query | `failedDetections` contains `"hpaEnabled"`, partial results returned |

---

## Blast Radius

### SP (Go) -- No Detection Logic (ADR-056)

Per ADR-056, DetectedLabels computation was relocated from SP to HAPI. SP's original `pkg/signalprocessing/detection/labels.go` has been removed. SP still captures raw K8s metadata (annotations, labels) via K8sEnricher into `KubernetesContext`, but this metadata is used only for business classification and custom labels — not for DetectedLabels.

| File | Status |
|------|--------|
| `pkg/signalprocessing/detection/labels.go` | Removed (ADR-056) |
| `pkg/shared/types/enrichment.go` | Type definitions (unchanged — `DetectedLabels` struct consumed by AA from HAPI response) |

### HAPI (Python) -- Authoritative Implementation

| File | Change | Version |
|------|--------|---------|
| `holmesgpt-api/src/detection/labels.py` | Label detector: ArgoCD v3 `tracking-id` detection added (priorities 1, 5, 8) | v1.3 |
| `holmesgpt-api/src/toolsets/resource_context.py` | Context builder: `deployment_details.annotations` now populated for ArgoCD v3 | v1.3 |
| `holmesgpt-api/tests/unit/test_label_detector.py` | Conformance tests: DL-HP-11 through DL-HP-14 added | v1.3 |

---

## Related Documents

| Document | Relationship |
|----------|-------------|
| **ADR-056** | Parent architectural decision: relocate DetectedLabels to HAPI post-RCA |
| **ADR-055** | LLM-driven context enrichment: `get_resource_context` tool that will host label detection |
| **DD-WORKFLOW-001 v2.3** | Original detection method documentation (SP-specific) |
| **DD-HAPI-017 v1.3** | Flow enforcement: `get_resource_context` must be called before workflow discovery. v1.3 adds `cluster_context` to `list_available_actions` response. |
| **BR-SP-101** | DetectedLabels auto-detection business requirement |
| **BR-SP-103** | FailedDetections tracking business requirement |
| **Issue #102** | Implementation tracking issue |

---

## Review & Evolution

**When to Revisit**:
- If new detection characteristics are added (e.g., Kustomize management, OPA/Gatekeeper policies)
- If K8s API groups change (e.g., autoscaling/v2 deprecation)
- If annotation keys change for GitOps tools (new ArgoCD or Flux versions)
- If the specification and an implementation diverge (specification takes precedence)

**Conformance Enforcement**:
- HAPI CI pipeline SHOULD include conformance test runs
- The test vectors in this document are the minimum set; the implementation MAY add additional tests

---

**Document Version**: 1.3
**Last Updated**: February 24, 2026
**Status**: APPROVED
**Authority**: Authoritative detection specification for DetectedLabels (HAPI)
**Confidence**: 96%

**Confidence Gap (4%)**:
- Python K8s client behavioral differences (~2%): The Kubernetes Python client (`kubernetes` package) has different error types and timeout behavior than Go's controller-runtime. Label selector matching utilities may differ. Mitigated by conformance test vectors.
- RBAC configuration parity (~2%): HAPI's service account needs PDB, HPA, and NetworkPolicy RBAC. If RBAC is not granted, all API-based detections fail gracefully via FailedDetections. Mitigated by Helm chart RBAC templates.

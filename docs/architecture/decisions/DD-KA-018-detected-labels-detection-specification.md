# DD-KA-018: DetectedLabels Detection Specification

**Status**: APPROVED
**Decision Date**: 2026-02-12
**Version**: 2.0 (Go rewrite)
**Confidence**: 93%
**Applies To**: Kubernaut Agent (KA) — Go, authoritative implementation per ADR-056

---

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0–1.5 | 2026-02-12 to 2026-03-24 | Architecture Team | Historical evolution under the Python-era implementation: initial 7-characteristic spec extracted from the original SP Go reference implementation, ArgoCD v3 tracking-id support, ResourceQuota detection, non-workload target handling for PodDisruptionBudget. Superseded by v2.0 below; see git history for the original entries. |
| 2.0 | 2026-08-01 | — | Rewritten directly from `internal/kubernautagent/enrichment/label_detector.go` as part of [Issue #1806](https://github.com/jordigilh/kubernaut/issues/1806). Corrected the detection count from 9 fields (7 characteristics + `gitOpsTool` + the v1.4 ResourceQuota addition) to the current **12 `FailedDetections` categories** (`AllDetectionCategories` in source), adding 4 CNV/KubeVirt detections (`virtualMachine`, `liveMigratable`, `cdiManaged`, `storageBackend`, #1378) not present in any prior version. Corrected the GitOps precedence table and the non-workload-target strategy to match the actual Go implementation, which fetches only the resolved root owner (via REST-mapper GVR resolution, #679) rather than separately fetching Pod + Deployment + Namespace. Corrected the consumer guidance per [DD-WORKFLOW-019](DD-WORKFLOW-019-ka-owned-workflow-discovery.md) (KA's own in-process catalog, not a DataStorage REST call). |

---

## Context & Problem

### Current State (Go KA)

DetectedLabels auto-detection is implemented in `internal/kubernautagent/enrichment/label_detector.go` (`LabelDetector.DetectLabels`). Per ADR-056, this computation runs during enrichment against the **resolved root owner** of the RCA target (not the raw signal source or the RCA target resource itself if it has an owner chain), because the signal and the root cause may be different resources with different GitOps/infrastructure characteristics.

Unlike the historical design (which fetched Pod, Deployment, and Namespace as three separate K8s objects), the Go implementation fetches **only the root owner object**, using the REST mapper to resolve its GVR generically (`fetchResource`, #679). This means label detection works uniformly for any root-owner kind — including non-workload kinds like ConfigMap, Secret, or Service — without the workload-specific special-casing the historical Python design needed for PodDisruptionBudget targets.

### Problem

Without a formal, language-agnostic specification, implementations may diverge on:
- Which annotations/labels trigger each detection, and in what precedence order
- The distinction between "resource absent" (valid `false`) and "query failed" (unknown)
- K8s API resources queried and RBAC requirements

### Business Requirements

- **BR-SP-101**: DetectedLabels Auto-Detection (SP's own, narrower, signal-time use of a subset of these characteristics)
- **BR-SP-103**: FailedDetections Tracking — RBAC, timeout, network errors
- **BR-KA-264/265**: Post-RCA label detection and use in workflow discovery

---

## Decision

This document is the authoritative specification for DetectedLabels detection as implemented by KA. Where this specification and the implementation disagree, treat `internal/kubernautagent/enrichment/label_detector.go` as the ground truth and file a correction.

---

## Output Schema

All 12 categories below can appear in `FailedDetections` (`AllDetectionCategories` in source). `GitOpsTool` shares a category with `GitOpsManaged` (`gitOpsManaged`) since they are always detected together.

| Field | Type | JSON Key | Description |
|-------|------|----------|-------------|
| FailedDetections | string[] | `failedDetections` | Category names where detection QUERY failed (RBAC, timeout, network). Empty if all succeeded. |
| GitOpsManaged | bool | `gitOpsManaged` | True if managed by a GitOps controller |
| GitOpsTool | string | `gitOpsTool` | `"argocd"`, `"flux"`, or `""` |
| PDBProtected | bool | `pdbProtected` | True if a PodDisruptionBudget matches this workload |
| HPAEnabled | bool | `hpaEnabled` | True if an HPA targets this workload |
| Stateful | bool | `stateful` | True if owner chain contains a StatefulSet |
| HelmManaged | bool | `helmManaged` | True if managed by Helm |
| NetworkIsolated | bool | `networkIsolated` | True if any NetworkPolicy exists in namespace |
| ServiceMesh | string | `serviceMesh` | `"istio"`, `"linkerd"`, or `""` |
| ResourceQuotaConstrained | bool | `resourceQuotaConstrained` | True if any ResourceQuota exists in namespace |
| VirtualMachine | bool | `virtualMachine` | True if the target or an owner-chain entry is a CNV/KubeVirt kind (#1378) |
| LiveMigratable | bool | `liveMigratable` | True if a VirtualMachine's `evictionStrategy` is `LiveMigrate` (conditional on `virtualMachine=true`) |
| CDIManaged | bool | `cdiManaged` | True if any namespace PVC carries a `cdi.kubevirt.io/storage.import.*` annotation |
| StorageBackend | string | `storageBackend` | Canonical CSI backend name (`"odf-ceph"`, `"lvms"`, `"local"`, or `""`) resolved from PVC → StorageClass → provisioner |

---

## Input Contract

### Root Owner Object

`fetchResource` resolves the root owner's GVR via the REST mapper (supports namespaced and cluster-scoped kinds, any registered CRD) and fetches it as a single `unstructured.Unstructured`. Two metadata views are derived from it:

| View | Source | Used By |
|------|--------|---------|
| Root-level labels/annotations | `obj.GetLabels()` / `obj.GetAnnotations()` | GitOps (several priorities), Helm, ServiceMesh legacy fallback |
| Pod-template labels/annotations | `spec.template.metadata.{labels,annotations}` (present only on workload kinds with a pod template — Deployment, StatefulSet, DaemonSet) | GitOps (highest-priority checks), PDB selector matching, ServiceMesh primary check |

### Namespace Object

Fetched separately (`fetchNamespace`) when the root owner is namespaced. Best-effort: fetch failure is silently skipped (not a `FailedDetections` entry), since namespace-level GitOps markers are a fallback, not a primary signal.

### Owner Chain

Ordered list of owner references from the target resource up to the root owner (from the enrichment phase's owner-chain resolution). Used by Stateful detection (any `StatefulSet` in the chain) and HPA `scaleTargetRef` matching (matches against every entry, not just the immediate root).

---

## Detection Specifications

### Detection 1: GitOps Management

**Fields**: `gitOpsManaged` (bool), `gitOpsTool` (string)
**K8s API Call**: None — uses metadata already fetched
**FailedDetections**: Not applicable (no K8s API query)

**Detection Logic** (`detectGitOps` — first match wins, in this exact order):

| Priority | Source | Key | Tool | Notes |
|----------|--------|-----|------|-------|
| P1 | Pod-template annotations | `argocd.argoproj.io/tracking-id` | argocd | ArgoCD v3 default |
| P2 | Pod-template labels | `argocd.argoproj.io/instance` | argocd | ArgoCD v2 |
| P3 | Root annotations | `argocd.argoproj.io/tracking-id` | argocd | ArgoCD v3, no pod-template markers |
| P4 | Root labels | `fluxcd.io/sync-gc-mark` | flux | |
| P5 | Root annotations OR root labels | `argocd.argoproj.io/instance` | argocd | ArgoCD v2, checked on both annotations and labels at equal priority |
| P6 | Namespace labels | `argocd.argoproj.io/instance` | argocd | |
| P7 | Namespace annotations | `argocd.argoproj.io/tracking-id` | argocd | ArgoCD v3 |
| P8 | Namespace annotations | `fluxcd.io/sync-gc-mark` | flux | |
| P9 | Namespace annotations | `argocd.argoproj.io/managed` | argocd | |
| P10 | Namespace annotations | `fluxcd.io/sync-status` | flux | |
| L11 (legacy) | Root annotations | `argocd.argoproj.io/managed-by` | argocd | Backward-compat fallback, not part of the core 10-priority cascade |
| L12 (legacy) | Root annotations | `fluxcd.io/sync-checksum` | flux | Backward-compat fallback |
| L13 (legacy) | Root annotations | `kustomize.toolkit.fluxcd.io/name` | flux | Backward-compat fallback |

**Key rule**: Presence of the key is sufficient and non-empty. The value is not otherwise inspected.

**ArgoCD version compatibility**: ArgoCD v2 (label-based tracking) is detected via `argocd.argoproj.io/instance`. ArgoCD v3 (annotation-based tracking, default) is detected via `argocd.argoproj.io/tracking-id`, which — per this precedence table — always wins over a coexisting v2 label at the same source level.

**Default** (no match): `gitOpsManaged=false`, `gitOpsTool=""`

---

### Detection 2: PDB Protection

**Field**: `pdbProtected` (bool)
**K8s API Call**: `List policy/v1 PodDisruptionBudget` in namespace
**RBAC Required**: `get`, `list` on `poddisruptionbudgets` in `policy` API group
**FailedDetections**: `"pdbProtected"` on query failure

**Detection Logic** (`detectPDB`):
1. If both pod-template labels and root labels are empty: `pdbProtected=false`, return (no error, no query — nothing to match against)
2. List all PDBs in namespace
3. For each PDB with a non-empty `spec.selector.matchLabels`: if the selector is a subset of the pod-template labels **or** a subset of the root labels: `pdbProtected=true`, return
4. No match: `pdbProtected=false`

**Error handling**: List failure → `pdbProtected=false`, add `"pdbProtected"` to `FailedDetections`.

---

### Detection 3: HPA Enabled

**Field**: `hpaEnabled` (bool)
**K8s API Call**: `List autoscaling/v2 HorizontalPodAutoscaler` in namespace
**RBAC Required**: `get`, `list` on `horizontalpodautoscalers` in `autoscaling` API group
**FailedDetections**: `"hpaEnabled"` on query failure or when namespace is unknown

**Detection Logic** (`detectHPA`):
1. Build the set of `(kind, name)` pairs from every entry in the owner chain
2. List all HPAs in namespace; for each, extract `spec.scaleTargetRef.{kind,name}`
3. If any HPA's `scaleTargetRef` matches an entry in the owner-chain set: `hpaEnabled=true`, return
4. No match: `hpaEnabled=false`

**Error handling**: Empty namespace or List failure → `hpaEnabled=false`, add `"hpaEnabled"` to `FailedDetections`.

---

### Detection 4: Stateful Workload

**Field**: `stateful` (bool)
**K8s API Call**: None — uses owner chain data
**FailedDetections**: Not applicable

**Detection Logic** (`detectStateful`): `stateful=true` if any owner-chain entry has `Kind == "StatefulSet"`.

---

### Detection 5: Helm Managed

**Field**: `helmManaged` (bool)
**K8s API Call**: None — uses root owner labels
**FailedDetections**: Not applicable

**Detection Logic** (`detectHelm`):
1. If root owner label `app.kubernetes.io/managed-by` equals `"Helm"` (case-insensitive): `helmManaged=true`
2. Else if root owner label `helm.sh/chart` exists (any value): `helmManaged=true`
3. Default: `helmManaged=false`

---

### Detection 6: Network Isolation

**Field**: `networkIsolated` (bool)
**K8s API Call**: `List networking.k8s.io/v1 NetworkPolicy` in namespace
**RBAC Required**: `get`, `list` on `networkpolicies` in `networking.k8s.io` API group
**FailedDetections**: `"networkIsolated"` on query failure or when namespace is unknown

**Detection Logic** (`detectNetworkPolicy`): `networkIsolated = (count of NetworkPolicies in namespace) > 0`. Checks existence only, not whether a specific policy targets the workload.

---

### Detection 7: Service Mesh

**Field**: `serviceMesh` (string)
**K8s API Call**: None — uses pod-template and root annotations
**FailedDetections**: Not applicable

**Detection Logic** (`detectServiceMesh`, first match wins):
1. Pod-template annotation `sidecar.istio.io/status` (any value) → `serviceMesh="istio"`
2. Pod-template annotation `linkerd.io/proxy-version` (any value) → `serviceMesh="linkerd"`
3. Legacy fallback — root owner annotation `sidecar.istio.io/inject == "true"` → `serviceMesh="istio"`
4. Legacy fallback — root owner annotation `linkerd.io/inject == "enabled"` → `serviceMesh="linkerd"`
5. Default: `serviceMesh=""`

---

### Detection 8: ResourceQuota Constrained

**Field**: `resourceQuotaConstrained` (bool), plus a separate quota-usage summary (not part of `DetectedLabels` — returned alongside it)
**K8s API Call**: `List core/v1 ResourceQuota` in namespace
**RBAC Required**: `get`, `list` on `resourcequotas` in the core API group
**FailedDetections**: `"resourceQuotaConstrained"` on query failure or when namespace is unknown

**Detection Logic** (`detectResourceQuota` + `summarizeQuotas`):
1. List ResourceQuotas in namespace
2. Zero found → `resourceQuotaConstrained=false`, no summary
3. One or more found → `resourceQuotaConstrained=true`; build a summary keyed by resource name, with `{hard, used}` raw K8s quantity strings (e.g. `"4"`, `"8Gi"`). First quota that defines a given resource key wins if multiple quotas overlap.

---

### Detections 9–12: CNV / KubeVirt (#1378)

Gated by a REST-mapper pre-check (`cnvAvailable`) confirming the `VirtualMachine` CRD (`kubevirt.io`) is registered. On non-CNV clusters, all four detections are skipped with **no** `FailedDetections` entries (CRD absence is expected, not a failure).

**Detection 9 — VirtualMachine** (`virtualMachine`, bool): No API call. `true` if the RCA target's kind or any owner-chain entry's kind is one of `VirtualMachine`, `VirtualMachineInstance`, `VirtualMachineInstanceMigration`, `DataVolume`.

**Detection 10 — LiveMigratable** (`liveMigratable`, bool): Conditional on `virtualMachine=true` and root kind `VirtualMachine`. Fetches the VirtualMachine object; `true` if `spec.template.spec.evictionStrategy == "LiveMigrate"`. API failure → `FailedDetections += "liveMigratable"`.

**Detection 11 — CDIManaged** (`cdiManaged`, bool): Conditional on `virtualMachine=true`. Lists namespace PVCs; `true` if any PVC carries an annotation prefixed `cdi.kubevirt.io/storage.import.`. PVC list failure → `FailedDetections += "cdiManaged", "storageBackend"` (both detections share the same PVC listing call).

**Detection 12 — StorageBackend** (`storageBackend`, string): Conditional on `virtualMachine=true`. For each PVC with a `storageClassName`, fetches the StorageClass and maps its `provisioner` to a canonical backend: `rbd.csi.ceph.com` substring → `"odf-ceph"`; `topolvm.io` → `"lvms"`; `kubernetes.io/no-provisioner` → `"local"`; otherwise unmapped (tries the next PVC). StorageClass fetch failure (with no backend ultimately resolved) → `FailedDetections += "storageBackend"`.

---

## FailedDetections Contract

### Semantics

| Scenario | Field Value | FailedDetections | Meaning |
|----------|-------------|-------------------|---------|
| PDB exists, matches | `pdbProtected=true` | `[]` | Workload is PDB-protected |
| No PDB in namespace | `pdbProtected=false` | `[]` | No PDB protection |
| PDB query RBAC denied | `pdbProtected=false` | `["pdbProtected"]` | Unknown — DO NOT use for filtering |
| Root owner fetch fails entirely | all fields default | **all 12 categories** | Total detection failure — see below |

### Rules

1. Only K8s API-based detections (PDB, HPA, NetworkPolicy, ResourceQuota, LiveMigratable, CDIManaged, StorageBackend) can produce `FailedDetections` entries under normal operation.
2. Annotation/label-based detections (GitOps, Stateful, Helm, ServiceMesh, VirtualMachine) never independently produce `FailedDetections` entries — they use already-fetched metadata.
3. **Total failure mode**: if the root owner object itself cannot be fetched, `DetectLabels` short-circuits and marks **all 12** `AllDetectionCategories` as failed, rather than attempting partial detection against missing data.
4. Consumers MUST check `FailedDetections` before trusting a `false` value. A `false` with the category listed means "unknown," not "absent."
5. A `true` value is always trustworthy regardless of `FailedDetections`.

### Consumer Guidance

Per [DD-WORKFLOW-019](DD-WORKFLOW-019-ka-owned-workflow-discovery.md), workflow discovery is owned by KA itself (not DataStorage). Detected labels have two consumer paths:

1. **Workflow discovery catalog filters**: `internal/kubernautagent/investigator/investigator_discovery.go` (`attachRCADetectedLabelsJSON`) marshals `DetectedLabels` onto `SignalContext.DetectedLabelsJSON`; `internal/kubernautagent/tools/custom/tools.go` unmarshals it and sets `filters.DetectedLabels` on the `workflowcatalog.Catalog` call for `list_available_actions`/`list_workflows`. This filters the in-process catalog to workflows whose `detectedLabels` requirements match the target.
2. **LLM prompt context**: rendered via `internal/kubernautagent/prompt` templates as part of the investigation prompt, giving the LLM explicit infrastructure context for action-type reasoning.

`FailedDetections` exclusion applies to both paths — fields listed there must not be used as positive/negative filter criteria and should not be presented to the LLM as trustworthy.

---

## Execution Order

Label/annotation-based detections run first (no I/O), then the API-based ones. All are attempted regardless of prior failures — no short-circuiting on error, to maximize information collection:

1. GitOps (no API call)
2. Helm (no API call)
3. Stateful (no API call)
4. ServiceMesh (no API call)
5. HPA (API call)
6. PDB (API call)
7. NetworkPolicy (API call)
8. ResourceQuota (API call)
9. CNV cascade (VirtualMachine → LiveMigratable → CDIManaged/StorageBackend; only runs if the CNV CRD is registered and `virtualMachine=true`)

---

## Non-Workload Root Owners

Unlike the historical Python design (which special-cased PodDisruptionBudget-as-target by resolving pods via the PDB's selector), the Go implementation handles non-workload root owners generically: `fetchResource` resolves any Kind via the REST mapper and dispatches to the namespaced or cluster-scoped dynamic client accordingly (#679/#762). Detections that depend on a pod template (GitOps priorities 1–2, PDB pod-label matching, ServiceMesh priority 1) simply find no pod-template metadata on non-workload kinds and fall through to the next applicable check or default — no special-casing required per target kind.

---

## Blast Radius

### SP (Go) — No Detection Logic (ADR-056)

Per ADR-056, DetectedLabels computation was relocated from SP to the post-RCA phase. SP's original `pkg/signalprocessing/detection/labels.go` has been removed. SP still captures raw K8s metadata via its enricher, but only for business classification and custom labels — not for `DetectedLabels`.

### KA (Go) — Authoritative Implementation

| File | Role |
|------|------|
| `internal/kubernautagent/enrichment/label_detector.go` | `LabelDetector.DetectLabels` — all 12 detections |
| `internal/kubernautagent/investigator/investigator_discovery.go` | `attachRCADetectedLabelsJSON` — plumbs results into `SignalContext.DetectedLabelsJSON` |
| `internal/kubernautagent/tools/custom/tools.go` | Discovery tools read `DetectedLabelsJSON` and set catalog query filters |
| `pkg/shared/types/enrichment.go` | `DetectedLabels` struct, shared with AIAnalysis's own copy of the schema |

---

## Related Documents

| Document | Relationship |
|----------|-------------|
| **ADR-056** | Parent architectural decision: relocate DetectedLabels computation to post-RCA |
| **DD-KA-006** | Owner-chain / root-owner resolution that this spec's root-owner fetch depends on |
| **DD-KA-017** | Workflow discovery tool contract that consumes `DetectedLabels` as catalog filters |
| **DD-WORKFLOW-019** | Discovery/catalog ownership (KA, not DS) — governs the consumer-path description above |
| **DD-WORKFLOW-001** | `DetectedLabels` schema definition shared with SignalProcessing/AIAnalysis |
| **BR-SP-101 / BR-SP-103** | SP's own (narrower, signal-time) detection and failure-tracking requirements |
| **Issue #1378** | CNV/KubeVirt detection additions |
| **Issue #679 / #762** | Generic root-owner GVR resolution (REST mapper, scope-aware client dispatch) |

---

## Review & Evolution

**When to Revisit**:
- If new detection characteristics are added
- If K8s API groups change (e.g., `autoscaling/v2` deprecation)
- If annotation keys change for GitOps tools or CNV/KubeVirt
- If this specification and `label_detector.go` diverge (the source is authoritative; update this doc to match)

---

**Document Version**: 2.0
**Last Updated**: August 1, 2026
**Status**: APPROVED
**Authority**: Authoritative detection specification for DetectedLabels (KA)
**Confidence**: 93%

**Confidence Gap (7%)**:
- This rewrite was derived by reading `label_detector.go` directly rather than from a separate conformance test suite; exact behavior for rare multi-source coexistence cases (e.g., v2 and v3 ArgoCD markers present at multiple source levels simultaneously) should be double-checked against `internal/kubernautagent/enrichment` test files if a discrepancy is ever suspected.

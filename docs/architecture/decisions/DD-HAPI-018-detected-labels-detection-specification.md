# DD-HAPI-018: DetectedLabels Detection Specification

**Status**: APPROVED
**Decision Date**: 2026-02-12
**Version**: 1.0
**Confidence**: 96%
**Applies To**: SignalProcessing (Go, reference implementation), HolmesGPT API (Python, new implementation per ADR-056)

---

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-02-12 | Architecture Team | Initial specification extracted from SP reference implementation (`pkg/signalprocessing/detection/labels.go`). Cross-language contract for HAPI Python reimplementation. |

---

## Context & Problem

### Current State

SignalProcessing (SP) implements `DetectedLabels` auto-detection in Go (`pkg/signalprocessing/detection/labels.go`). Per ADR-056, this computation must be relocated to HolmesGPT API (HAPI) in Python, where it runs post-RCA against the actual remediation target resource instead of the signal source.

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
| `deploymentDetails.labels` | map[string]string | GitOps (ArgoCD, Flux), Helm |
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
2. **Deployment details**: `GET /apis/apps/v1/namespaces/{ns}/deployments/{name}` -- labels
3. **Namespace metadata**: `GET /api/v1/namespaces/{ns}` -- labels and annotations
4. **Owner chain**: Already resolved by `get_resource_context`

---

## Detection Specifications

### Detection 1: GitOps Management

**Fields**: `gitOpsManaged` (bool), `gitOpsTool` (string)
**K8s API Call**: None -- uses metadata already fetched
**FailedDetections**: Not applicable (no K8s API query)

**Detection Logic** (MUST follow this precedence order -- first match wins):

| Priority | Source | Annotation/Label Key | Result |
|----------|--------|---------------------|--------|
| 1 | Pod annotations | `argocd.argoproj.io/instance` | `gitOpsManaged=true`, `gitOpsTool="argocd"` |
| 2 | Deployment labels | `fluxcd.io/sync-gc-mark` | `gitOpsManaged=true`, `gitOpsTool="flux"` |
| 3 | Deployment labels | `argocd.argoproj.io/instance` | `gitOpsManaged=true`, `gitOpsTool="argocd"` |
| 4 | Namespace labels | `argocd.argoproj.io/instance` | `gitOpsManaged=true`, `gitOpsTool="argocd"` |
| 5 | Namespace labels | `fluxcd.io/sync-gc-mark` | `gitOpsManaged=true`, `gitOpsTool="flux"` |
| 6 | Namespace annotations | `argocd.argoproj.io/managed` | `gitOpsManaged=true`, `gitOpsTool="argocd"` |
| 7 | Namespace annotations | `fluxcd.io/sync-status` | `gitOpsManaged=true`, `gitOpsTool="flux"` |

**Default** (no match): `gitOpsManaged=false`, `gitOpsTool=""`

**Key rule**: Presence of the key is sufficient. The value is not inspected (any non-empty value matches).

**Null safety**: If `podDetails` is nil/None, skip priorities 1. If `deploymentDetails` is nil/None, skip priorities 2-3. If `namespaceLabels` is nil/None, skip priorities 4-5. If `namespaceAnnotations` is nil/None, skip priorities 6-7.

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

### Consumer Guidance (for HAPI prompt builder and workflow discovery)

- Fields in FailedDetections MUST be excluded from workflow discovery filters (DD-WORKFLOW-001 v2.1)
- Fields in FailedDetections MUST be excluded from LLM cluster context descriptions
- See `holmesgpt-api/src/extensions/incident/prompt_builder.py` for the reference consumer implementation

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

## Conformance Test Vectors

Both SP and HAPI test suites MUST pass the following test vectors. Test IDs are from the SP reference test suite (`test/unit/signalprocessing/label_detector_test.go`).

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

### Edge Case Vectors

| Test ID | Input | Expected Output |
|---------|-------|-----------------|
| DL-EC-01 | Plain deployment, no special annotations/labels/resources | All fields `false`/empty, `failedDetections=[]` |
| DL-EC-02 | Nil/None KubernetesContext | Return nil/None (no DetectedLabels object) |
| DL-EC-03 | ArgoCD + PDB + HPA all present simultaneously | `gitOpsManaged=true`, `pdbProtected=true`, `hpaEnabled=true` |

### Error Handling Vectors

| Test ID | Input | Expected Output |
|---------|-------|-----------------|
| DL-ER-01 | PDB query returns RBAC forbidden | `pdbProtected=false`, `failedDetections=["pdbProtected"]` |
| DL-ER-02 | HPA query returns timeout | `hpaEnabled=false`, `failedDetections=["hpaEnabled"]` |
| DL-ER-03 | PDB + HPA + NetworkPolicy queries all fail | `failedDetections=["pdbProtected", "hpaEnabled", "networkIsolated"]` |
| DL-ER-04 | Context cancellation during HPA query | `failedDetections` contains `"hpaEnabled"`, partial results returned |

---

## Blast Radius

### SP (Go) -- Reference Implementation (No Changes Required)

The current SP implementation in `pkg/signalprocessing/detection/labels.go` is the reference that this specification was extracted from. No changes required -- it already conforms.

| File | Status |
|------|--------|
| `pkg/signalprocessing/detection/labels.go` | Reference implementation (unchanged) |
| `pkg/shared/types/enrichment.go` | Type definitions (unchanged) |
| `test/unit/signalprocessing/label_detector_test.go` | Reference test suite (unchanged) |

### HAPI (Python) -- New Implementation Required

| File | Change | Phase |
|------|--------|-------|
| `holmesgpt-api/src/toolsets/resource_context.py` | Add label detection logic to `get_resource_context` tool | ADR-056 Phase 1 |
| `holmesgpt-api/src/detection/labels.py` (new) | Python label detector conforming to this specification | ADR-056 Phase 1 |
| `holmesgpt-api/tests/unit/test_label_detector.py` (new) | Python conformance tests matching test vectors above | ADR-056 Phase 1 |

---

## Related Documents

| Document | Relationship |
|----------|-------------|
| **ADR-056** | Parent architectural decision: relocate DetectedLabels to HAPI post-RCA |
| **ADR-055** | LLM-driven context enrichment: `get_resource_context` tool that will host label detection |
| **DD-WORKFLOW-001 v2.3** | Original detection method documentation (SP-specific) |
| **DD-HAPI-017 v1.2** | Flow enforcement: `get_resource_context` must be called before workflow discovery |
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
- Both SP and HAPI CI pipelines SHOULD include conformance test runs
- The test vectors in this document are the minimum set; implementations MAY add additional tests

---

**Document Version**: 1.0
**Last Updated**: February 12, 2026
**Status**: APPROVED
**Authority**: Cross-team detection specification for DetectedLabels (SP + HAPI)
**Confidence**: 96%

**Confidence Gap (4%)**:
- Python K8s client behavioral differences (~2%): The Kubernetes Python client (`kubernetes` package) has different error types and timeout behavior than Go's controller-runtime. Label selector matching utilities may differ. Mitigated by conformance test vectors.
- RBAC configuration parity (~2%): HAPI's service account needs PDB, HPA, and NetworkPolicy RBAC that SP already has. If RBAC is not granted, all API-based detections fail gracefully via FailedDetections. Mitigated by Helm chart RBAC templates.

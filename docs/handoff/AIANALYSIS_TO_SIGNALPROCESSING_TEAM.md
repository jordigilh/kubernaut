# Questions from AIAnalysis Team

**From**: AIAnalysis Service Team
**To**: SignalProcessing Team
**Date**: December 1, 2025
**Context**: Shared types package creation and contract alignment

---

## 🔔 SCHEMA CHANGE: DetectedLabels (Dec 2, 2025)

**Status**: ⚠️ **BREAKING CHANGE** - DetectedLabels schema updated per DD-WORKFLOW-001 v2.1

**Change**: Added `FailedDetections []string` field to track detection failures. Avoids `*bool` anti-pattern.

**New Schema** (`pkg/shared/types/enrichment.go`):
```go
type DetectedLabels struct {
    // NEW: Lists fields where detection failed (RBAC, timeout, etc.)
    // If a field is in this array, ignore its value
    FailedDetections []string `json:"failedDetections,omitempty" validate:"omitempty,dive,oneof=gitOpsManaged pdbProtected hpaEnabled stateful helmManaged networkIsolated podSecurityLevel serviceMesh"`

    // Feature flags (plain bool - no *bool pointers)
    GitOpsManaged    bool   `json:"gitOpsManaged"`
    GitOpsTool       string `json:"gitOpsTool,omitempty" validate:"omitempty,oneof=argocd flux"`
    PDBProtected     bool   `json:"pdbProtected"`
    HPAEnabled       bool   `json:"hpaEnabled"`
    Stateful         bool   `json:"stateful"`
    HelmManaged      bool   `json:"helmManaged"`
    NetworkIsolated  bool   `json:"networkIsolated"`
    PodSecurityLevel string `json:"podSecurityLevel,omitempty" validate:"omitempty,oneof=privileged baseline restricted"`
    ServiceMesh      string `json:"serviceMesh,omitempty" validate:"omitempty,oneof=istio linkerd"`
}
```

**SignalProcessing Implementation Pattern**:
```go
func (d *LabelDetector) DetectLabels(ctx context.Context, k8sCtx *KubernetesContext) *DetectedLabels {
    labels := &DetectedLabels{}
    var failedDetections []string

    // PDB detection
    hasPDB, err := d.detectPDB(ctx, k8sCtx)
    if err != nil {
        log.Error(err, "Failed to detect PDB (RBAC?)")
        failedDetections = append(failedDetections, "pdbProtected")
    } else {
        labels.PDBProtected = hasPDB
    }

    // ... other detections ...

    if len(failedDetections) > 0 {
        labels.FailedDetections = failedDetections
    }
    return labels
}
```

**Validation**: `FailedDetections` only accepts known field names (go-playground/validator enum).

**Authoritative Source**: DD-WORKFLOW-001 v2.1

### ✅ SignalProcessing Team Acknowledgment (December 2, 2025)

The SignalProcessing team confirms:
1. **Schema change received** - `FailedDetections []string` field understood
2. **Implementation plan updated** - v1.17 → v1.18 with FailedDetections pattern
3. **Code examples updated** - `DetectLabels()` now tracks failures in array
4. **No issues expected** - Pattern aligns with our error handling approach

**Important Clarification** (from implementation review):
- `FailedDetections` tracks **query failures** only (RBAC denied, timeout, network error)
- Resource not existing (e.g., no PDB for this pod) → `false` value (NOT a failure)
- Only when we **cannot determine** the answer do we add to `FailedDetections`

**Example**:
| Scenario | `pdbProtected` | `FailedDetections` |
|----------|----------------|-------------------|
| PDB exists for pod | `true` | `[]` |
| No PDB for pod | `false` | `[]` |
| RBAC denied querying PDBs | `false` | `["pdbProtected"]` |

**Go Type Updated**: `pkg/shared/types/enrichment.go` now includes `FailedDetections` field ✅

**Updated Reference**: `IMPLEMENTATION_PLAN_V1.18.md` (Day 8: DetectedLabels section)

---

## Background

We've created a shared types package at `pkg/shared/types/enrichment.go` to serve as the **single source of truth** for enrichment types. Both AIAnalysis and SignalProcessing now use type aliases pointing to this shared package.

**Changes made**:
- Created `pkg/shared/types/enrichment.go` with all enrichment types
- Updated `api/signalprocessing/v1alpha1/signalprocessing_types.go` to use type aliases
- Updated `api/aianalysis/v1alpha1/aianalysis_types.go` to use type aliases
- Ran `make generate && make manifests` successfully

---

## Questions

### Q1: Type Alias Compatibility

We've updated SignalProcessing types to use aliases:

```go
// api/signalprocessing/v1alpha1/signalprocessing_types.go

import sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"

// Type aliases for shared enrichment types
type EnrichmentResults = sharedtypes.EnrichmentResults
type OwnerChainEntry = sharedtypes.OwnerChainEntry
type DetectedLabels = sharedtypes.DetectedLabels
type KubernetesContext = sharedtypes.KubernetesContext
// ... etc
```

**Question**: Does your existing controller code compile with these alias changes? The aliases should be transparent, but please confirm.

---

### Q2: EnrichmentResults Population

The shared `EnrichmentResults` type has these fields that SignalProcessing must populate:

```go
type EnrichmentResults struct {
    KubernetesContext *KubernetesContext `json:"kubernetesContext,omitempty"`
    DetectedLabels    *DetectedLabels    `json:"detectedLabels,omitempty"`
    OwnerChain        []OwnerChainEntry  `json:"ownerChain,omitempty"`
    CustomLabels      map[string][]string `json:"customLabels,omitempty"`
    EnrichmentQuality float64            `json:"enrichmentQuality,omitempty"`
}
```

**Question**: Are you currently populating all these fields? Specifically:
- [ ] `KubernetesContext` - Pod/Deployment/Node details
- [ ] `DetectedLabels` - Auto-detected cluster characteristics
- [ ] `OwnerChain` - K8s owner reference traversal
- [ ] `CustomLabels` - Rego-extracted subdomain labels
- [ ] `EnrichmentQuality` - Overall quality score (0.0-1.0)

---

### Q3: OwnerChain Population

Per DD-WORKFLOW-001 v1.7, `OwnerChain` is **mandatory** for HolmesGPT-API's DetectedLabels validation. SignalProcessing must traverse `metadata.ownerReferences` to build this chain.

> ⚠️ **SCHEMA CORRECTION** (December 2, 2025): The schema below is **OUTDATED**. See authoritative schema in Q3 Response section.

**~~Expected structure~~ (OUTDATED - see Q3 Response)**:
```go
// ❌ OUTDATED SCHEMA - DO NOT USE
type OwnerChainEntry struct {
    Kind       string `json:"kind"`                 // Resource kind (e.g., "ReplicaSet")
    Name       string `json:"name"`                 // Resource name
    APIVersion string `json:"apiVersion,omitempty"` // ❌ REMOVED - Not used by HolmesGPT-API
    UID        string `json:"uid,omitempty"`        // ❌ REMOVED - Not used by HolmesGPT-API
}
```

**Question**: Is your owner traversal logic implemented? What's the traversal depth limit (if any)?

---

### Q4: CustomLabels Extraction

Per `HANDOFF_CUSTOM_LABELS_EXTRACTION_V1.md`, CustomLabels use subdomain-based format:

```
Format: <subdomain>.kubernaut.io/<key>[:<value>]
Key = subdomain (filter dimension)
Value = list of extracted labels

Example output:
{
  "constraint": ["cost-constrained", "risk-tolerance=low"],
  "business": ["category=payments"],
  "team": ["name=platform"]
}
```

**Question**: Is your Rego policy evaluation implemented for CustomLabels extraction? Do you need additional documentation on the expected output format?

---

### Q5: DetectedLabels Auto-Detection

`DetectedLabels` are **auto-detected** by SignalProcessing (no customer config needed).

**AUTHORITATIVE SOURCE**: `pkg/shared/types/enrichment.go`

```go
type DetectedLabels struct {
    // GitOps Management
    GitOpsManaged bool   `json:"gitOpsManaged"`           // Auto-detected from ArgoCD/Flux annotations
    GitOpsTool    string `json:"gitOpsTool,omitempty"`    // argocd, flux, ""

    // Workload Protection
    PDBProtected bool `json:"pdbProtected"`              // Has PodDisruptionBudget
    HPAEnabled   bool `json:"hpaEnabled"`                // Has HorizontalPodAutoscaler

    // Workload Characteristics
    Stateful    bool `json:"stateful"`                   // StatefulSet or has PVCs
    HelmManaged bool `json:"helmManaged"`                // Has helm.sh/chart label

    // Security Posture
    NetworkIsolated  bool   `json:"networkIsolated"`             // Has NetworkPolicy in namespace
    PodSecurityLevel string `json:"podSecurityLevel,omitempty"`  // privileged, baseline, restricted, ""
    ServiceMesh      string `json:"serviceMesh,omitempty"`       // istio, linkerd, ""
}
```

**Question**: What detection methods are you using for each field? Are there any fields you cannot reliably detect?

---

## SignalProcessing Team Response

**Date**: December 2, 2025
**Respondent**: SignalProcessing Team
**Status**: ✅ **COMPLETE**

---

### 📋 Response Summary

| Question | Status | Key Response |
|----------|--------|--------------|
| Q1: Type Alias Compatibility | ✅ **Compiles** | Type aliases working, build passes |
| Q2: EnrichmentResults Population | 🟡 **Planned** | 4 of 5 fields planned; `EnrichmentQuality` NOT implementing |
| Q3: OwnerChain Traversal | 🟡 **Planned** | Depth limit: None (traverse to root) |
| Q4: CustomLabels Rego Extraction | 🟡 **Planned** | Sandboxed OPA runtime with ConfigMap policies |
| Q5: DetectedLabels Auto-Detection | 🟡 **Planned** | All 9 fields with documented detection methods |

**Implementation Timeline**: Days 7-9 in `IMPLEMENTATION_PLAN_V1.18.md`

---

### Q1 Response: Type Alias Compatibility ✅ CONFIRMED

- [x] **Compiles successfully**

**Evidence**:
- `make generate && make manifests` executed successfully
- `go build ./api/signalprocessing/...` passes
- Type aliases are transparent as expected

---

### Q2 Response: EnrichmentResults Population 🟡 PLANNED

| Field | Status | Implementation Notes |
|-------|--------|---------------------|
| `KubernetesContext` | 🟡 Planned | Day 3-4: K8s Enricher implementation |
| `DetectedLabels` | 🟡 Planned | Day 7-9: Auto-detection from K8s resources |
| `OwnerChain` | 🟡 Planned | Day 7: ownerReference traversal |
| `CustomLabels` | 🟡 Planned | Day 8-9: Rego policy extraction |
| `EnrichmentQuality` | ❌ **Not Planned** | See below |

**EnrichmentQuality Decision**: ❌ **NOT IMPLEMENTING**

Per Dec 2, 2025 decision, DetectedLabels are **deterministic lookups**, not predictions:
- Detection succeeds → explicit `true` or `false`
- Detection fails (RBAC, timeout) → `false` + **error log**

**Rationale**: No partial success concept. Error logs provide observability. Field is optional (`omitempty`).

---

### Q3 Response: OwnerChain Traversal 🟡 PLANNED

- [x] **Planned** with depth limit: **None** (traverse to root)

**Algorithm** (per DD-WORKFLOW-001 v1.8):
- Traverse `ownerReferences` using `controller: true` owner
- Natural termination when no more owners
- Typical depth: 3 (Pod → ReplicaSet → Deployment)

**✅ AUTHORITATIVE Schema** (per DD-WORKFLOW-001 v1.8 + `pkg/shared/types/enrichment.go`):
```go
type OwnerChainEntry struct {
    Namespace string `json:"namespace,omitempty"`  // ✅ Empty for cluster-scoped (Node)
    Kind      string `json:"kind"`                 // ✅ REQUIRED
    Name      string `json:"name"`                 // ✅ REQUIRED
}
```

**Schema Comparison** (why Q3 original was outdated):
| Field | Q3 Original | Authoritative | Notes |
|-------|-------------|---------------|-------|
| `namespace` | ❌ Missing | ✅ **REQUIRED** | Empty for cluster-scoped (Node) |
| `kind` | ✅ Present | ✅ **REQUIRED** | e.g., ReplicaSet, Deployment |
| `name` | ✅ Present | ✅ **REQUIRED** | Owner resource name |
| `apiVersion` | ✅ Present | ❌ **REMOVED** | Not used by HolmesGPT-API |
| `uid` | ✅ Present | ❌ **REMOVED** | Not used by HolmesGPT-API |

**Why the Change** (per DD-WORKFLOW-001 v1.8):
1. **`namespace` added**: Required for HolmesGPT-API validation - allows matching resources across namespaced scope
2. **`apiVersion` removed**: Not used in validation logic - unnecessary data
3. **`uid` removed**: Not used in validation logic - creates brittleness (UIDs change on recreation)

**Implementation Example**:
```go
// Pod owned by ReplicaSet owned by Deployment
ownerChain := []sharedtypes.OwnerChainEntry{
    {Namespace: "production", Kind: "ReplicaSet", Name: "payment-api-7d8f9c6b5"},
    {Namespace: "production", Kind: "Deployment", Name: "payment-api"},
}
```

**Important**: Do NOT include `apiVersion` or `uid` - not used by HolmesGPT-API.

---

### Q4 Response: CustomLabels Rego Extraction 🟡 PLANNED

- [x] **Planned** - Days 8-9

**Architecture**: ConfigMap `signal-processing-policies` → Sandboxed OPA → `map[string][]string`

**Security & Validation** (per **DD-WORKFLOW-001 v1.9**):
- Sandboxed OPA: No network, no filesystem, 5s timeout, 128MB memory
- Limits: Max 10 keys, 5 values/key, 63 char keys, 100 char values
- Security wrapper: Blocks override of 5 mandatory labels

**No additional documentation needed** - DD-WORKFLOW-001 v1.9 is comprehensive.

---

### Q5 Response: DetectedLabels Auto-Detection 🟡 PLANNED

**All 9 fields can be reliably detected** (deterministic lookups):

| Field | Detection Method | Reliability |
|-------|------------------|-------------|
| `gitOpsManaged` | ArgoCD/Flux annotations | ✅ High |
| `gitOpsTool` | ArgoCD → "argocd", Flux → "flux" | ✅ High |
| `pdbProtected` | PodDisruptionBudget exists | ✅ High |
| `hpaEnabled` | HorizontalPodAutoscaler targets workload | ✅ High |
| `stateful` | StatefulSet OR has PVCs | ✅ High |
| `helmManaged` | `helm.sh/chart` label | ✅ High |
| `networkIsolated` | NetworkPolicy in namespace | ✅ High |
| `podSecurityLevel` | Namespace PSS label | ✅ High |
| `serviceMesh` | Istio/Linkerd sidecar or labels | ✅ High |

**Error Handling** (per Dec 2 decision):
- Detection fails → `false` + error log
- Downstream consumers receive valid booleans

---

## Follow-Up Questions for AIAnalysis Team - ✅ ALL RESOLVED

### SP→AIA-001: EnrichmentResults Data Path (HIGH) - ✅ RESOLVED

**Context**: SignalProcessing populates `status.enrichmentResults`. Need confirmation on consumption.

**Question**: How does AIAnalysis consume enrichment data?

**AIAnalysis Response (Dec 2, 2025)**: **B) Receive via RO when AIAnalysis CRD is created** ✅

⚠️ **PATH CORRECTION**: See [NOTICE_AIANALYSIS_PATH_CORRECTION.md](./NOTICE_AIANALYSIS_PATH_CORRECTION.md)

**Data Flow Confirmed**:
```
SignalProcessing.Status.EnrichmentResults
        │
        ▼ (RO copies)
AIAnalysis.Spec.AnalysisRequest.SignalContext.EnrichmentResults
```

**Correct AIAnalysis Path**: `spec.analysisRequest.signalContext.enrichmentResults.*`

**SignalProcessing Action**: None needed - we just populate `status.enrichmentResults`. RO handles the copy.

---

### SP→AIA-002: Which DetectedLabels Fields Are Used? (MEDIUM) - ✅ RESOLVED

**Context**: SignalProcessing detects 9 fields. Want to confirm usage.

**Question**: Which DetectedLabels fields does AIAnalysis/HolmesGPT actually use?

**AIAnalysis Response (Dec 2, 2025)**: **D) Passed through to HolmesGPT only - AIAnalysis doesn't interpret** ✅

AIAnalysis does **NOT** interpret `DetectedLabels` fields directly. It passes the entire struct to HolmesGPT-API, which uses ALL fields for:
1. **Workflow Filtering** (SQL WHERE clause)
2. **LLM Context** (natural language in prompt)

**Recommendation**: SignalProcessing should populate **ALL 9 DetectedLabels fields** accurately:
1. `gitOpsManaged` (bool)
2. `gitOpsTool` (string)
3. `pdbProtected` (bool)
4. `hpaEnabled` (bool)
5. `stateful` (bool)
6. `helmManaged` (bool)
7. `networkIsolated` (bool)
8. `podSecurityLevel` (string)
9. `serviceMesh` (string)

**SignalProcessing Action**: Our current plan to detect all 9 fields is correct. ✅

---

## References

| Document | Purpose |
|----------|---------|
| `pkg/shared/types/enrichment.go` | Authoritative enrichment types (source of truth) |
| `DD-WORKFLOW-001 v1.9` | Label schema, OwnerChain, DetectedLabels, CustomLabels validation |
| `IMPLEMENTATION_PLAN_V1.18.md` | SignalProcessing implementation schedule |
| `api/signalprocessing/v1alpha1/signalprocessing_types.go` | CRD type definitions with aliases |

---

## Changelog

| Date | Change |
|------|--------|
| Dec 1, 2025 | Initial questions from AIAnalysis team |
| Dec 2, 2025 | SignalProcessing team response (Q1-Q5) + follow-up questions (SP→AIA-001/002) |
| Dec 2, 2025 | AIAnalysis team response: SP→AIA-001 (path correction), SP→AIA-002 (passthrough confirmed) |
| Dec 2, 2025 | **Q3 Schema Clarification**: OwnerChainEntry schema corrected (removed apiVersion/uid, added namespace) |
| Dec 2, 2025 | **ALL QUESTIONS RESOLVED** - Document complete |
| Dec 2, 2025 | **SCHEMA CHANGE**: DetectedLabels FailedDetections field added (DD-WORKFLOW-001 v2.1) |
| Dec 2, 2025 | **SP ACKNOWLEDGMENT**: Implementation plan updated to v1.18 with FailedDetections pattern |
| Dec 2, 2025 | **SP CLARIFICATION**: FailedDetections tracks QUERY failures only, not "resource doesn't exist" |
| Dec 2, 2025 | **SP SCHEMA UPDATE**: Added `FailedDetections` field to `pkg/shared/types/enrichment.go` (DD-WORKFLOW-001 v2.1) |


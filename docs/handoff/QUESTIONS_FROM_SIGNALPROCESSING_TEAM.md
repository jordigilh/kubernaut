# Questions from SignalProcessing Team

**From**: SignalProcessing Team
**Date**: December 1, 2025
**Context**: SignalProcessing v1.0 Implementation - Integration Questions

---

## 📊 Status Dashboard

| To Team | Questions | Priority | Status |
|---------|-----------|----------|--------|
| **AIAnalysis** | 3 | 🔴 High | ✅ **RESOLVED** (Dec 2, 2025) |
| **Gateway** | 3 | 🟡 Medium | ✅ **Resolved** (Dec 2, 2025) |
| **RO** | 3 | 🔴 High | ✅ Resolved |

**Note**: AIAnalysis Q&A consolidated in `AIANALYSIS_TO_SIGNALPROCESSING_TEAM.md` (Dec 2, 2025)

---

## Questions for AIAnalysis Team

### SP→AIA-001: Enrichment Data Path Confirmation (HIGH)

**Context**: Per previous validation (RESPONSE_SIGNALPROCESSING_INTEGRATION_VALIDATION.md), AIAnalysis reads enrichment data from:

```
spec.analysisRequest.signalContext.enrichmentResults.*
```

**Question**: Can you confirm this is still the correct path?

**Fields we're passing**:
- `ownerChain` - Kubernetes ownership hierarchy
- `detectedLabels` - Auto-detected cluster characteristics
- `customLabels` - User-defined labels from Rego

**Options**:
- [ ] A) Confirmed - path is correct
- [ ] B) Path changed to: _______________
- [ ] C) Different fields at different paths - provide mapping

---

### SP→AIA-002: DetectedLabels Usage (MEDIUM)

**Context**: SignalProcessing auto-detects cluster characteristics:

```go
type DetectedLabels struct {
    IsGitOpsManaged     bool   // ArgoCD/Flux detected
    GitOpsTool          string // "argocd", "flux", ""
    HasPDB              bool   // PodDisruptionBudget exists
    HasHPA              bool   // HorizontalPodAutoscaler exists
    IsStatefulSet       bool   // StatefulSet workload
    IsHelmManaged       bool   // Helm release detected
    HasNetworkPolicy    bool   // NetworkPolicy applies
    PodSecurityStandard string // "privileged", "baseline", "restricted"
    IsServiceMesh       bool   // Istio/Linkerd detected
    ServiceMeshType     string // "istio", "linkerd", ""
}
```

**Question**: Does AIAnalysis use all these fields, or only a subset?

**Why it matters**: HolmesGPT-API confirmed `detected_labels` ARE included in LLM prompts for environment reasoning. We need to prioritize detection accuracy for fields the LLM actually uses.

**Options**:
- [ ] A) All fields are used
- [ ] B) Only these fields: _______________
- [ ] C) Currently unused - future feature
- [ ] D) Passed through to HolmesGPT only - AIAnalysis doesn't interpret

---

### SP→AIA-003: Missing Enrichment Data Behavior (MEDIUM) - ✅ RESOLVED

**Context**: SignalProcessing might fail to detect certain characteristics (e.g., RBAC denies PDB access).

**Decision (SP Team - Dec 1, 2025)**:
- If detection fails (`nil`): **treat as `false` + emit error log**
- If detection succeeds with negative result: explicit `false`

**Rationale**:
- Simple boolean logic for downstream consumers
- Error log provides observability for RBAC/detection issues
- No "unknown" state to handle

**AIAnalysis/HolmesGPT can assume**: All `DetectedLabels` fields are valid booleans. If a field is `false`, it means either:
1. Resource genuinely doesn't have that characteristic, OR
2. Detection failed (check SignalProcessing logs)

**No action required from AIAnalysis** - this is informational.

---

## Questions for Gateway Team - ✅ ALL RESOLVED

### SP→GW-001: CustomLabels Field Order (LOW) - ✅ RESOLVED

**Original Question**: In the HolmesGPT prompt template, what order are these labels presented?

**Update (Dec 1, 2025)**: HolmesGPT-API confirmed that `custom_labels` are **NOT included in the LLM prompt** - only used for search query filtering. This likely makes ordering irrelevant.

**Gateway Team Response (Dec 2, 2025)**: Gateway does not handle CustomLabels. See detailed response below.

**Options**:
- [x] A) Resolved - ordering doesn't matter for search filtering
- [ ] B) Still relevant because: _______________

---

### SP→GW-002: CustomLabels Size Limits (MEDIUM) - ✅ RESOLVED

**Original Question**: Is there a size limit for `CustomLabels`?

**Update (Dec 1, 2025)**: HolmesGPT-API confirmed no hard limit, 64k tokens soft recommendation. Our limits (10 keys × 5 values × 100 chars ≈ 5KB) are well within bounds.

**Current SignalProcessing Limits** (implemented):
- Max 10 custom label keys
- Max 5 values per key
- Max 100 chars per value

**Gateway Team Response (Dec 2, 2025)**: Gateway doesn't enforce limits - not in CustomLabels path. SP limits are sufficient.

**Options**:
- [ ] A) Limits are acceptable
- [ ] B) Gateway has stricter limits: _______________
- [x] C) Gateway doesn't enforce limits - SP limits are sufficient

---

### SP→GW-003: CustomLabels Validation/Sanitization (MEDIUM) - ✅ RESOLVED

**Context**: `CustomLabels` come from user Rego policies (sandboxed but user-defined).

**Original Concern**: Malicious Rego could output labels designed to manipulate LLM behavior (prompt injection).

**Update (Dec 1, 2025)**: HolmesGPT-API confirmed `custom_labels` are NOT in LLM prompt - only search filtering. This reduces prompt injection risk.

**Gateway Team Response (Dec 2, 2025)**: Gateway has NO Rego policies (per DD-CATEGORIZATION-001). SignalProcessing owns Rego sandbox. Data Storage uses parameterized queries.

**Options**:
- [ ] A) Yes, Gateway validates - specify what
- [ ] B) Data Storage validates - no action needed
- [ ] C) SignalProcessing should validate - specify requirements
- [x] D) No validation needed - Rego sandbox + search parameterization is sufficient

---

## Questions for RO Team - ✅ RESOLVED

See [QUESTIONS_FOR_RO_TEAM.md](../services/crd-controllers/01-signalprocessing/QUESTIONS_FOR_RO_TEAM.md)

| Question | Answer | Status |
|----------|--------|--------|
| Q1: `correlationId` case | A - Intentional (JSON camelCase) | ✅ Resolved |
| Q2: Recovery data flow | A - Confirmed (RO populates from WE) | ✅ Resolved |
| Q3: Sequence diagram | A - DD-CONTRACT-002 | ✅ Resolved |

---

## SignalProcessing Team Concerns

### Concern 1: DD-CONTRACT-002 Review (MEDIUM)

**Issue**: RO team referenced `DD-CONTRACT-002-service-integration-contracts.md` as the authoritative sequence diagram.

**Action**: SignalProcessing team will review before implementation to ensure alignment.

**No question required** - just noting for tracking.

---

## Response Sections

### AIAnalysis Team Response

**Date**: December 2, 2025
**Respondent**: AIAnalysis Service Team

| Question | Response | Notes |
|----------|----------|-------|
| SP→AIA-001 (Data path) | **C** ✅ | ⚠️ Path correction - see details below |
| SP→AIA-002 (DetectedLabels usage) | **D** ✅ | Passthrough to HolmesGPT-API |
| SP→AIA-003 (Missing data) | ✅ **RESOLVED** | SP treats nil as false + error log. No AIAnalysis action needed. |

**Detailed Responses**:

#### SP→AIA-001: Enrichment Data Path Confirmation - ✅ RESOLVED

**Response**: **C) Different fields at different paths - provide mapping**

⚠️ **CRITICAL PATH CORRECTION** - See [NOTICE_AIANALYSIS_PATH_CORRECTION.md](./NOTICE_AIANALYSIS_PATH_CORRECTION.md)

| Field | **Correct AIAnalysis Path** |
|-------|----------------------------|
| `ownerChain` | `spec.analysisRequest.signalContext.enrichmentResults.ownerChain` |
| `detectedLabels` | `spec.analysisRequest.signalContext.enrichmentResults.detectedLabels` |
| `customLabels` | `spec.analysisRequest.signalContext.enrichmentResults.customLabels` |

**NOT** `spec.signalContext.*` (this path does not exist in AIAnalysis CRD)

**Authoritative Source**: `api/aianalysis/v1alpha1/aianalysis_types.go`

---

#### SP→AIA-002: DetectedLabels Usage - ✅ RESOLVED

**Response**: **D) Passed through to HolmesGPT only - AIAnalysis doesn't interpret**

AIAnalysis does **NOT** interpret `DetectedLabels` fields directly. It passes the entire struct to HolmesGPT-API, which uses ALL fields for:

1. **Workflow Filtering** (SQL WHERE clause)
2. **LLM Context** (natural language in prompt)

**Recommendation**: SignalProcessing should populate **ALL DetectedLabels fields** accurately.

**DetectedLabels Schema** (authoritative: `pkg/shared/types/enrichment.go`):
- `gitOpsManaged`, `gitOpsTool`
- `pdbProtected`, `hpaEnabled`
- `stateful`, `helmManaged`
- `networkIsolated`, `podSecurityLevel`, `serviceMesh`

---

### Gateway Team Response

**Date**: December 2, 2025
**Respondent**: Gateway Team

| Question | Response | Notes |
|----------|----------|-------|
| SP→GW-001 (Field order) | **A** ✅ | Gateway doesn't handle CustomLabels; ordering irrelevant for search filtering |
| SP→GW-002 (Size limits) | **C** ✅ | Gateway doesn't handle CustomLabels; SP's 5KB limit is appropriate |
| SP→GW-003 (Validation) | **D** ✅ | Gateway has NO Rego (DD-CATEGORIZATION-001); SP owns Rego sandbox |

**Detailed Responses**:

#### SP→GW-001: CustomLabels Field Order - ✅ RESOLVED

**Response**: **A) Resolved - ordering doesn't matter for search filtering**

This question is **out of Gateway scope**. Gateway does not handle `CustomLabels`:

| Component | Role with CustomLabels |
|-----------|----------------------|
| **Gateway** | ❌ Does NOT process CustomLabels |
| **SignalProcessing** | ✅ Generates CustomLabels via Rego policies |
| **HolmesGPT-API** | ✅ Uses CustomLabels for workflow search filtering |

**Key Fact** (from HolmesGPT-API, Dec 1, 2025):
> `custom_labels` are **NOT included in the LLM prompt** - only used for search query filtering.

Since CustomLabels are only used for Data Storage query filtering (not LLM context), field ordering is irrelevant.

---

#### SP→GW-002: CustomLabels Size Limits - ✅ RESOLVED

**Response**: **C) Gateway doesn't enforce limits - SP limits are sufficient**

Gateway does not process `CustomLabels`. The data flow is:

```
SignalProcessing → RO → AIAnalysis → HolmesGPT-API → Data Storage
                                      (search filter)
```

Gateway is not in this path. SP's proposed limits (10 keys × 5 values × 100 chars ≈ 5KB) are well within bounds for Data Storage query parameters and HolmesGPT-API's 64k token soft recommendation.

**Recommendation**: SignalProcessing should enforce its proposed limits. Gateway has no role here.

---

#### SP→GW-003: CustomLabels Validation/Sanitization - ✅ RESOLVED

**Response**: **D) No validation needed at Gateway - SignalProcessing owns Rego + Data Storage uses parameterized queries**

Gateway does not process `CustomLabels`, so Gateway validation is not applicable.

**Per DD-CATEGORIZATION-001** (Approved):
> Gateway will NOT have Rego policies. All Rego-based categorization (including CustomLabels extraction) is consolidated in SignalProcessing Service.

**Security Assessment**:

| Risk | Mitigation | Owner | Status |
|------|-----------|-------|--------|
| **Prompt Injection** | CustomLabels NOT in LLM prompt | HolmesGPT-API | ✅ Mitigated |
| **SQL Injection** | Parameterized queries | Data Storage | ✅ Mitigated |
| **Malicious Rego** | Rego sandbox with resource limits | **SignalProcessing** | ✅ Mitigated |

**Reference**: `DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md`

---

## Document History

| Date | Change |
|------|--------|
| Dec 1, 2025 | Initial creation - consolidated from individual question files |
| Dec 1, 2025 | Updated Gateway questions based on HolmesGPT-API responses |
| Dec 2, 2025 | **Gateway Team Response**: All 3 questions resolved (SP→GW-001/002/003) |
| Dec 2, 2025 | **AIAnalysis Team Response**: All 3 questions resolved (SP→AIA-001/002/003) - Path correction issued |

---

**SignalProcessing Team**


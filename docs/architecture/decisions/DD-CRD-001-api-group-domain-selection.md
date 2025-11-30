# DD-CRD-001: CRD API Group Domain Selection

## Status
**✅ APPROVED** (2025-11-30)
**Last Reviewed**: 2025-11-30
**Confidence**: 90%

**Implementation Status**: DEFERRED MIGRATION
- New CRD services: MUST use `.ai`
- Existing CRDs: Migrate in future release (tracked separately)

## Context & Problem

Kubernaut is an **AIOps platform** for Kubernetes that uses Custom Resource Definitions (CRDs) to manage remediation workflows. We need to decide on the appropriate domain suffix for our CRD API groups.

**Current State**: Mixed usage
- 6 CRDs use `*.kubernaut.io`
- 1 CRD uses `*.kubernaut.ai` (notification)

**Key Requirements**:
- Consistent API group naming across all CRDs
- Alignment with industry conventions for AI/cloud-native projects
- Brand alignment with platform positioning as an AIOps solution

## Alternatives Considered

### Alternative 1: Use `.io` Domain (`*.kubernaut.io`)

**Approach**: Follow traditional Kubernetes project conventions using `.io` TLD.

**Pros**:
- ✅ Most common convention in Kubernetes ecosystem (Istio, Argo, Cert-Manager)
- ✅ Familiar to Kubernetes administrators
- ✅ Established, stable TLD

**Cons**:
- ❌ Does not signal AI focus
- ❌ Generic - doesn't differentiate from infrastructure tooling
- ❌ Misaligned with K8sGPT precedent (uses `.ai`)

**Confidence**: 60% (rejected)

---

### Alternative 2: Use `.ai` Domain (`*.kubernaut.ai`)

**Approach**: Use `.ai` TLD to signal AI-first platform positioning.

**Pros**:
- ✅ Immediately signals AI-powered platform
- ✅ Aligns with K8sGPT precedent (`core.k8sgpt.ai`)
- ✅ Brand differentiation from traditional K8s tooling
- ✅ Matches project positioning as AIOps platform
- ✅ Memorable and distinctive

**Cons**:
- ❌ Less common in broader K8s ecosystem
- ❌ Slightly newer convention

**Confidence**: 90% (approved)

---

### Alternative 3: Use `.dev` Domain (`*.kubernaut.dev`)

**Approach**: Use `.dev` TLD following HolmesGPT/Robusta pattern.

**Pros**:
- ✅ Developer-focused signal
- ✅ Used by HolmesGPT (`holmesgpt.dev`)

**Cons**:
- ❌ Signals "developer tool" more than "AI platform"
- ❌ Less specific than `.ai` for AIOps positioning

**Confidence**: 50% (rejected)

---

## Decision

**APPROVED: Alternative 2** - Use `*.kubernaut.ai` for all CRD API groups.

**Rationale**:
1. **K8sGPT Precedent**: The most direct comparable open-source AI K8s project uses `.ai`
2. **Brand Alignment**: AIOps is the core value proposition - domain should reflect this
3. **Differentiation**: Stands out from traditional infrastructure tooling
4. **Industry Trend**: AI-native projects increasingly adopt `.ai`

**Key Insight**: For AI-native platforms (not platforms that merely use AI), `.ai` is strategically appropriate and has established precedent.

## Implementation

**Template Reference**: All new CRD services MUST use `.ai` domain as specified in:
- [`docs/services/SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md`](../../services/SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md)

**Primary Implementation Files**:
- `api/remediation/v1alpha1/groupversion_info.go` → `remediation.kubernaut.ai`
- `api/signalprocessing/v1alpha1/groupversion_info.go` → `signalprocessing.kubernaut.ai`
- `api/aianalysis/v1alpha1/groupversion_info.go` → `aianalysis.kubernaut.ai`
- `api/workflowexecution/v1alpha1/groupversion_info.go` → `workflowexecution.kubernaut.ai`
- `api/kubernetesexecution/v1alpha1/groupversion_info.go` → `kubernetesexecution.kubernaut.ai`
- `api/remediationorchestrator/v1alpha1/groupversion_info.go` → `remediationorchestrator.kubernaut.ai`
- `api/notification/v1alpha1/groupversion_info.go` → `notification.kubernaut.ai` (already correct)

**API Group Format**:
```
<resource-type>.kubernaut.ai/v1alpha1
```

**Examples**:
```yaml
apiVersion: remediation.kubernaut.ai/v1alpha1
kind: RemediationRequest

apiVersion: aianalysis.kubernaut.ai/v1alpha1
kind: AIAnalysis

apiVersion: workflowexecution.kubernaut.ai/v1alpha1
kind: WorkflowExecution
```

## Consequences

**Positive**:
- ✅ Consistent branding across all CRDs
- ✅ Clear AI platform positioning
- ✅ Aligns with K8sGPT convention
- ✅ Memorable and distinctive

**Negative**:
- ⚠️ Less conventional than `.io` - **Mitigation**: AI K8s projects are establishing new conventions
- ⚠️ Requires updating existing docs - **Mitigation**: Pre-release, no migration needed

**Neutral**:
- 🔄 Notification CRD already uses `.ai` (consistency improved)

## Validation Results

**Comparable Projects Research**:
| Project | Type | CRD API Group |
|---------|------|---------------|
| K8sGPT | AI K8s diagnostics | `core.k8sgpt.ai` ✅ |
| HolmesGPT | AI troubleshooting | `robusta.dev` |
| KServe | Model serving | `serving.kserve.io` |
| Kubeflow | ML platform | `*.kubeflow.org` |

**Key Validation**: K8sGPT (closest comparable) uses `.ai`.

## Related Decisions

- **Supersedes**: `docs/architecture/decisions/001-crd-api-group-rationale.md` (partial - updates domain choice)
- **Supports**: BR-PLATFORM-001 (AIOps platform branding)

## Review & Evolution

**When to Revisit**:
- If industry convention shifts significantly
- If domain availability issues arise
- If enterprise customers express strong preference for `.io`

**Success Metrics**:
- 100% CRD consistency with `.ai` domain
- Clear brand recognition as AI platform


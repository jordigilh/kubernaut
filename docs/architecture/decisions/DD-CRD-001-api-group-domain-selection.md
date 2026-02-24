# DD-CRD-001: CRD API Group Domain Selection

## Status
**✅ APPROVED** (2025-11-30)
**Last Reviewed**: 2025-12-13 (Updated for single API group)
**Confidence**: 95%

**Implementation Status**: MIGRATION IN PROGRESS
- Domain: All CRDs MUST use `.ai` domain (not `.io`)
- API Group: All CRDs MUST use single API group `kubernaut.ai` (not resource-specific groups)
- Existing CRDs: Migration tracked in shared document

## Context & Problem

Kubernaut is an **AIOps platform** for Kubernetes that uses Custom Resource Definitions (CRDs) to manage remediation workflows. We need to decide on the appropriate domain suffix and grouping strategy for our CRD API groups.

**Current State** (as of 2025-12-13): Resource-specific groups with `.ai` domain
- 7 CRDs use resource-specific groups: `<resource>.kubernaut.ai` (e.g., `remediation.kubernaut.ai`)
- 1 CRD ~~still uses legacy `.io`~~: `kubernetesexecution.kubernaut.io` (DEPRECATED - ADR-025, CRD eliminated)

**Key Requirements**:
- Consistent API group naming across all CRDs
- Alignment with industry conventions for AI/cloud-native projects
- Brand alignment with platform positioning as an AIOps solution
- Simplicity: Avoid unnecessary complexity for tightly-coupled workflow phases

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

**APPROVED: Alternative 2** - Use `kubernaut.ai` as a **single API group** for all CRDs.

**Domain Rationale** (`.ai` suffix):
1. **K8sGPT Precedent**: The most direct comparable open-source AI K8s project uses `.ai`
2. **Brand Alignment**: AIOps is the core value proposition - domain should reflect this
3. **Differentiation**: Stands out from traditional infrastructure tooling
4. **Industry Trend**: AI-native projects increasingly adopt `.ai`

**Grouping Rationale** (single group):
1. **Architectural Fit**: All CRDs are tightly-coupled workflow phases, not distinct services
2. **Industry Pattern**: Unified platforms (Prometheus, Cert-Manager, ArgoCD) use single groups
3. **Simplicity**: Reduces complexity from 7 API groups to 1 API group
4. **Original Decision**: Honors `001-crd-api-group-rationale.md` (95% confidence)
5. **Best Practice**: Kubernetes community recommends "simplify to top-level domain"

**Key Insight**: For AI-native platforms (not platforms that merely use AI), `.ai` is strategically appropriate and has established precedent. For tightly-coupled workflow systems, a single API group provides clarity without unnecessary complexity.

## Implementation

**Template Reference**: All CRD services MUST use single API group `kubernaut.ai` as specified in:
- [`docs/services/SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md`](../../services/SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md)

**Primary Implementation Files** (requires migration):
- `api/remediation/v1alpha1/groupversion_info.go` → `kubernaut.ai`
- `api/signalprocessing/v1alpha1/groupversion_info.go` → `kubernaut.ai`
- `api/aianalysis/v1alpha1/groupversion_info.go` → `kubernaut.ai`
- `api/workflowexecution/v1alpha1/groupversion_info.go` → `kubernaut.ai`
- `api/kubernetesexecution/v1alpha1/groupversion_info.go` → `kubernaut.ai` (also migrate from `.io`)
- `api/remediationorchestrator/v1alpha1/groupversion_info.go` → `kubernaut.ai`
- `api/notification/v1alpha1/groupversion_info.go` → `kubernaut.ai`

**API Group Format**:
```
kubernaut.ai/v1alpha1
```

**Examples**:
```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: RemediationRequest

apiVersion: kubernaut.ai/v1alpha1
kind: SignalProcessing

apiVersion: kubernaut.ai/v1alpha1
kind: AIAnalysis

apiVersion: kubernaut.ai/v1alpha1
kind: WorkflowExecution

apiVersion: kubernaut.ai/v1alpha1
kind: NotificationRequest

apiVersion: kubernaut.ai/v1alpha1
kind: RemediationOrchestrator

apiVersion: kubernaut.ai/v1alpha1
kind: KubernetesExecution

apiVersion: kubernaut.ai/v1alpha1
kind: RemediationApprovalRequest
```

**Migration Guide**: See `docs/handoff/SHARED_APIGROUP_MIGRATION_NOTICE.md` for detailed migration instructions.

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
| Project | Type | CRD API Group | Grouping Strategy |
|---------|------|---------------|-------------------|
| K8sGPT | AI K8s diagnostics | `core.k8sgpt.ai` ✅ | Single group for all core resources |
| HolmesGPT | AI troubleshooting | `robusta.dev` | Single group |
| Prometheus Operator | Monitoring | `monitoring.coreos.com` | Single group |
| Cert-Manager | Certificates | `cert-manager.io` | Single group |
| ArgoCD | GitOps | `argoproj.io` | Single group |
| Istio | Service Mesh | `networking.istio.io`, `security.istio.io` | Multiple groups (distinct feature domains) |

**Key Validation**:
- K8sGPT (closest comparable AI project) uses `.ai` domain ✅
- K8sGPT uses **single API group** `core.k8sgpt.ai` for all core resources ✅
- Unified platforms (Prometheus, Cert-Manager, ArgoCD) use **single groups** ✅
- Only projects with distinct feature domains (like Istio's networking vs security) use multiple groups

## API Grouping Strategy

### Single Group vs Resource-Specific Groups

**Decision**: Use **single API group** `kubernaut.ai` for all CRDs

**Question**: Should we use `kubernaut.ai` or `<resource>.kubernaut.ai`?

**Analysis**:
1. **Architecture**: All Kubernaut CRDs are **tightly-coupled workflow phases**:
   - RemediationRequest → SignalProcessing → AIAnalysis → WorkflowExecution → Notification
   - These are sequential phases, not independent services

2. **Industry Pattern**:
   - **Single Group**: Unified platforms (Prometheus, Cert-Manager, ArgoCD, K8sGPT)
   - **Multiple Groups**: Distinct feature domains (Istio: networking vs security vs telemetry)

3. **Kubernetes Best Practice** (2025 web research):
   - "Simplify to top-level domain when possible"
   - "Use subdomains only for large number of subresources requiring further categorization"

4. **Original Decision**: `001-crd-api-group-rationale.md` (Oct 2025) decided on single group:
   - Explicitly rejected subdomains: "Redundant, no need for subdomain"
   - 95% confidence (highest confidence of both decisions)

**Benefits of Single Group**:
- ✅ Simpler kubectl commands: `kubectl get remediationrequests.kubernaut.ai`
- ✅ Clear project identity: All resources under one umbrella
- ✅ Easier RBAC: Single API group for permissions
- ✅ Reduced cognitive load: 1 API group vs 7 resource-specific groups
- ✅ Aligns with comparable projects (K8sGPT, Prometheus, Cert-Manager)

**When to Use Multiple Groups** (not applicable to Kubernaut):
- Truly distinct feature domains (e.g., Istio's networking vs security)
- Independent services with different lifecycles
- Large number of unrelated subresources

**Conclusion**: Kubernaut's CRDs are tightly-coupled workflow phases in a unified remediation pipeline. A single API group is appropriate, simpler, and follows industry patterns for unified platforms.

---

## Related Decisions

- **Supersedes**: `docs/architecture/decisions/001-crd-api-group-rationale.md` (October 2025)
  - Retains: Single API group strategy (95% confidence)
  - Updates: Domain from `.io` to `.ai` (AIOps branding)
- **Supports**: BR-PLATFORM-001 (AIOps platform branding)
- **Migration Guide**: `docs/handoff/SHARED_APIGROUP_MIGRATION_NOTICE.md` (December 2025)

## Review & Evolution

**When to Revisit**:
- If industry convention shifts significantly
- If domain availability issues arise
- If enterprise customers express strong preference for `.io`
- If CRDs become truly distinct services (not tightly-coupled workflow phases)

**Success Metrics**:
- 100% CRD consistency with `.ai` domain ✅
- 100% CRD consistency with single API group `kubernaut.ai` (in progress)
- Clear brand recognition as AI platform

**Update History**:
- **2025-11-30**: Initial approval - `.ai` domain with resource-specific groups (90% confidence)
- **2025-12-13**: Updated to single API group `kubernaut.ai` (95% confidence)
  - Aligned with original `001-crd-api-group-rationale.md` decision
  - Aligned with industry best practices (Kubernetes community guidance)
  - Aligned with comparable projects (K8sGPT, Prometheus, Cert-Manager, ArgoCD)


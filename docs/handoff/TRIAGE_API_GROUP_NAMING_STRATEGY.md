# API Group Naming Strategy Triage

**Date**: December 13, 2025
**Issue**: Conflicting decisions on API group naming - single group vs resource-specific groups
**Status**: 🔍 **INVESTIGATION REQUIRED**

---

## Executive Summary

**Critical Finding**: Two approved design decisions contradict each other regarding API group naming strategy:

| Document | Date | Decision | Status |
|----------|------|----------|--------|
| **001-crd-api-group-rationale.md** | Oct 6, 2025 | Single group: `kubernaut.io/v1` | ✅ APPROVED (95% confidence) |
| **DD-CRD-001-api-group-domain-selection.md** | Nov 30, 2025 | Resource-specific: `<resource>.kubernaut.ai/v1alpha1` | ✅ APPROVED (90% confidence) |

**User Concern**: "I don't see why we have to have the subdomain in the group when we don't plan to have so many subresources in this project."

**Impact**: All 7 CRDs currently use resource-specific groups. Migration to single group would affect entire platform.

---

## Historical Timeline

### October 6, 2025: Original Decision (001-crd-api-group-rationale.md)

**Decision**: Use **single API group** `kubernaut.io` for all CRDs

**Key Rationale** (from original document):
- Line 26-27: "Project-Scoped Grouping - `kubernaut.io` clearly identifies all CRDs as part of the Kubernaut project"
- Line 102: **"Redundant: `kubernaut.io` is sufficient, no need for subdomain"** (explicitly rejecting `alerts.kubernaut.io`)
- Line 235: "Current Decision: Not needed for V1 or V2. Stick with `kubernaut.io/v1`"

**Explicitly Rejected Alternatives**:
- ❌ `alerts.kubernaut.io` - "Too specific, redundant, no need for subdomain"
- ❌ `k8s.kubernaut.io` - "Unnecessary: kubernaut.io already implies Kubernetes context"

**Confidence**: 95% (highest confidence of both documents)

---

### November 30, 2025: Domain Change (DD-CRD-001)

**Decision**: Change domain from `.io` → `.ai` **AND** introduce resource-specific groups

**What Changed**:
1. ✅ Domain TLD: `kubernaut.io` → `kubernaut.ai` (justified for AIOps branding)
2. ⚠️ **Grouping Strategy**: Single group → Resource-specific groups (not explicitly justified)

**Implemented Format**:
```yaml
apiVersion: remediation.kubernaut.ai/v1alpha1
apiVersion: signalprocessing.kubernaut.ai/v1alpha1
apiVersion: kubernaut.ai/v1alpha1
apiVersion: kubernaut.ai/v1alpha1
apiVersion: notification.kubernaut.ai/v1alpha1
apiVersion: remediationorchestrator.kubernaut.ai/v1alpha1
apiVersion: kubernetesexecution.kubernaut.ai/v1alpha1  # (still .io - pending migration)
```

**Confidence**: 90%

---

## Industry Best Practices Research

### Web Search Results (December 13, 2025)

**Kubernetes Community Guidance**:
1. ✅ Use domain you own (kubernaut.ai)
2. ✅ Avoid subdomains unless necessary
3. ✅ Simplify to top-level domain when possible
4. ⚠️ Use subdomains only for "large number of subresources requiring further categorization"

**Key Quote from Research**:
> "If your project doesn't anticipate a vast number of subresources or hierarchical structures, using the top-level domain (e.g., `kubernaut.ai`) simplifies the API and aligns with common industry practices."

### Ecosystem Comparison

| Project | Pattern | Justification |
|---------|---------|---------------|
| **K8sGPT** | `core.k8sgpt.ai` | Single group with "core" prefix for main resources |
| **Istio** | `networking.istio.io`, `security.istio.io`, `telemetry.istio.io` | **Truly distinct feature domains** (networking vs security vs telemetry) |
| **Prometheus Operator** | `monitoring.coreos.com` | Single group for all monitoring resources |
| **Cert-Manager** | `cert-manager.io` | Single group for all certificate resources |
| **ArgoCD** | `argoproj.io` | Single group for all GitOps resources |

**Pattern Recognition**:
- **Single Group**: Projects with unified purpose (monitoring, certificates, GitOps)
- **Multiple Groups**: Projects with distinct feature domains (Istio's networking/security/telemetry are fundamentally different concerns)

---

## Kubernaut Architecture Analysis

### Are Our CRDs "Distinct Services"?

**Current CRDs and Their Role**:

| CRD | Purpose | Integration Level |
|-----|---------|-------------------|
| `RemediationRequest` | Entry point for all workflows | **Core orchestration** |
| `SignalProcessing` | Signal enrichment (phase 1) | **Tightly coupled to RR** |
| `AIAnalysis` | AI-powered root cause analysis (phase 2) | **Tightly coupled to RR** |
| `WorkflowExecution` | Execute remediation workflows (phase 3) | **Tightly coupled to RR** |
| `NotificationRequest` | Send notifications about workflow state | **Tightly coupled to RR** |
| `RemediationOrchestrator` | Orchestrate multi-phase workflow | **Core orchestration** |
| `KubernetesExecution` (DEPRECATED - ADR-025) | Execute K8s commands | **Tightly coupled to WE** |

**Analysis**:
- ✅ All CRDs are part of a **single unified workflow**
- ✅ They are **sequential phases** of remediation orchestration
- ✅ They are **tightly coupled** - not independent services
- ❌ They are NOT distinct feature domains (unlike Istio's networking vs security)
- ❌ No clear organizational benefit from separate API groups

**Conclusion**: These are **workflow phases**, not distinct services. They belong in a single API group.

---

## Comparison: Single vs Resource-Specific Groups

### Option A: Single API Group (Original Decision)

**Format**: `kubernaut.ai/v1alpha1`

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
```

**Pros**:
- ✅ Simpler kubectl commands: `kubectl get remediationrequests.kubernaut.ai`
- ✅ Clear project identity: All resources under one group
- ✅ Follows industry pattern for unified platforms (Prometheus, Cert-Manager, ArgoCD)
- ✅ Aligns with original decision (95% confidence)
- ✅ Reduces cognitive load: One API group to remember
- ✅ Easier RBAC: One API group for permissions
- ✅ Matches web research recommendations: "simplify to top-level domain"

**Cons**:
- ⚠️ Requires migration of all 7 CRDs
- ⚠️ Breaking change for existing manifests (pre-release, so acceptable)
- ⚠️ CRD manifest regeneration required

**Migration Impact**: Medium (7 CRDs × 3 files each = ~21 files)

---

### Option B: Resource-Specific Groups (Current Implementation)

**Format**: `<resource>.kubernaut.ai/v1alpha1`

**Examples**:
```yaml
apiVersion: remediation.kubernaut.ai/v1alpha1
kind: RemediationRequest

apiVersion: signalprocessing.kubernaut.ai/v1alpha1
kind: SignalProcessing

apiVersion: kubernaut.ai/v1alpha1
kind: AIAnalysis
```

**Pros**:
- ✅ Already implemented (no migration needed)
- ✅ Explicit resource type in API group
- ✅ Follows Istio pattern (though Istio has distinct feature domains)

**Cons**:
- ❌ Contradicts original decision (95% confidence)
- ❌ Verbose kubectl commands: `kubectl get remediationrequests.remediation.kubernaut.ai`
- ❌ Unnecessary complexity for unified platform
- ❌ No clear organizational benefit (all CRDs are tightly coupled workflow phases)
- ❌ Violates "no subdomain unless necessary" guidance
- ❌ Explicitly rejected in original decision: "Redundant, no need for subdomain"

**Cognitive Load**: High (7 different API groups to remember)

---

## User Feedback Analysis

**User Statement**:
> "I don't see why we have to have the subdomain in the group when we don't plan to have so many subresources in this project. And I thought we had already discussed this to be just 'kubernaut.ai'. Perhaps at that time we were still using 'kubernaut.io'."

**User's Memory Validation**: ✅ **CORRECT**

The user recalls the original decision correctly:
- October decision: Use single group (then `kubernaut.io`)
- November decision: Changed domain to `.ai` (correct) BUT also changed grouping strategy (unintentional?)

**Hypothesis**: DD-CRD-001 intended to change **only** the domain TLD (`.io` → `.ai`), not the grouping strategy. The resource-specific groups may have been introduced inadvertently when referencing K8sGPT's `core.k8sgpt.ai` pattern.

---

## K8sGPT Pattern Analysis

**K8sGPT Uses**: `core.k8sgpt.ai`

**Interpretation Question**:
- Option 1: K8sGPT uses subdomain `core` to distinguish from future groups (`analysis.k8sgpt.ai`, `remediation.k8sgpt.ai`)
- Option 2: K8sGPT uses `core` as a single namespace for all core resources

**Evidence for Option 2** (Single Group):
- K8sGPT has one primary CRD: `K8sGPT`
- `core` indicates "core functionality" not "distinct service"
- No evidence of other API groups like `analysis.k8sgpt.ai`

**Conclusion**: K8sGPT uses `core.k8sgpt.ai` as a **single API group**, not multiple resource-specific groups.

**Analogy for Kubernaut**:
- K8sGPT: `core.k8sgpt.ai` (single group with "core" prefix)
- Kubernaut: `kubernaut.ai` (single group, no prefix needed) OR `core.kubernaut.ai` (if prefix desired)

---

## Recommendation Matrix

### Recommended Approach: **Option A - Single API Group**

**Recommended Format**: `kubernaut.ai/v1alpha1`

**Rationale**:
1. ✅ **Aligns with original decision** (95% confidence - highest confidence)
2. ✅ **Matches industry best practices** (web research: "simplify to top-level domain")
3. ✅ **Follows comparable projects** (Prometheus, Cert-Manager, ArgoCD use single groups)
4. ✅ **User expectation** (user recalls original single-group decision)
5. ✅ **Reduces complexity** (7 API groups → 1 API group)
6. ✅ **Appropriate for architecture** (unified workflow, not distinct services)
7. ✅ **Easier RBAC** (one API group for permissions)

**Confidence**: **95%** (matches original decision confidence)

---

### Alternative: Option A-Modified - Single Group with Prefix

**Recommended Format**: `core.kubernaut.ai/v1alpha1` (following K8sGPT pattern exactly)

**Rationale**:
- Explicitly follows K8sGPT precedent
- `core` indicates primary platform functionality
- Leaves room for future non-core groups if needed (e.g., `extensions.kubernaut.ai` for plugins)

**Confidence**: **85%**

---

### Not Recommended: Option B - Current Resource-Specific Groups

**Reason for Rejection**:
1. ❌ Contradicts original 95% confidence decision
2. ❌ Violates "no subdomain unless necessary" guidance
3. ❌ Adds unnecessary complexity
4. ❌ No organizational benefit for tightly-coupled workflow phases

**Confidence**: **40%** (only keep if there's a compelling reason not yet identified)

---

## Migration Impact Assessment

### If Migrating to Single API Group

**Files Requiring Changes** (per CRD):

1. **Go API Definition**: `api/<resource>/v1alpha1/groupversion_info.go`
   - Change: `Group: "<resource>.kubernaut.ai"` → `Group: "kubernaut.ai"`
   - Kubebuilder annotation: `+groupName=kubernaut.ai`

2. **CRD Manifest**: `config/crd/bases/<resource>.kubernaut.ai_<resources>.yaml`
   - Regenerate via `make manifests`
   - New filename: `kubernaut.ai_<resources>.yaml`

3. **Controller RBAC**: `internal/controller/<resource>/*_controller.go`
   - Kubebuilder annotations: `//+kubebuilder:rbac:groups=kubernaut.ai,...`

**Total Files**: ~21 files (7 CRDs × 3 files each)

**Estimated Effort**: 2-4 hours (mechanical change + testing)

**Breaking Change**: Yes, but acceptable (pre-release product)

**Test Impact**: Update E2E test manifests (already use variables in most cases)

---

## Questions for Decision Maker

1. **Intent of DD-CRD-001**: Was the November decision intended to:
   - A) Change ONLY the domain TLD (`.io` → `.ai`) while keeping single group?
   - B) Change BOTH domain AND grouping strategy (single → resource-specific)?

2. **Organizational Value**: Do the resource-specific groups provide any organizational value that justifies the added complexity?

3. **Future Roadmap**: Are there plans for truly distinct feature domains (like Istio's networking/security/telemetry) that would benefit from separate API groups?

4. **User Preference**: Given the user's feedback, should we honor the original single-group decision?

---

## Proposed Resolution

### Recommendation: Update DD-CRD-001 to Single API Group

**Proposed Change to DD-CRD-001**:

**Section: API Group Format**
```diff
- <resource-type>.kubernaut.ai/v1alpha1
+ kubernaut.ai/v1alpha1
```

**Section: Examples**
```diff
- apiVersion: remediation.kubernaut.ai/v1alpha1
+ apiVersion: kubernaut.ai/v1alpha1
  kind: RemediationRequest

- apiVersion: kubernaut.ai/v1alpha1
+ apiVersion: kubernaut.ai/v1alpha1
  kind: AIAnalysis
```

**Updated Rationale** (add to DD-CRD-001):
```markdown
## Grouping Strategy

**Decision**: Use single API group `kubernaut.ai` for all CRDs (not resource-specific groups)

**Rationale**:
1. All CRDs are tightly-coupled workflow phases, not distinct services
2. Follows industry pattern for unified platforms (Prometheus, Cert-Manager, ArgoCD)
3. Aligns with Kubernetes best practice: "simplify to top-level domain"
4. Reduces complexity: 1 API group vs 7 resource-specific groups
5. Honors original architectural decision (001-crd-api-group-rationale.md)

**Note**: K8sGPT uses `core.k8sgpt.ai` as a **single group** for all core resources,
not multiple resource-specific groups. Kubernaut follows this pattern with `kubernaut.ai`.
```

---

## Implementation Plan (If Approved)

### Phase 1: Update Design Decision (1 hour)
- [ ] Update DD-CRD-001 to specify single API group
- [ ] Add clarification about K8sGPT pattern interpretation
- [ ] Document rationale for single group vs resource-specific groups

### Phase 2: Code Migration (2-3 hours)
- [ ] Update all 7 `groupversion_info.go` files
- [ ] Regenerate CRD manifests (`make manifests`)
- [ ] Update controller RBAC annotations
- [ ] Update E2E test manifests

### Phase 3: Documentation Updates (1 hour)
- [ ] Update all service documentation
- [ ] Update examples in handoff documents
- [ ] Update README files

### Phase 4: Testing (1-2 hours)
- [ ] Run unit tests
- [ ] Run integration tests
- [ ] Run E2E tests
- [ ] Validate kubectl commands

**Total Estimated Effort**: 5-7 hours

**Risk Level**: Low (pre-release, mechanical change, well-defined scope)

---

## Confidence Assessment

**Analysis Confidence**: 95%

**Justification**:
1. ✅ Clear evidence of conflicting decisions
2. ✅ User feedback validates original decision memory
3. ✅ Web research supports single group approach
4. ✅ Industry pattern analysis conclusive
5. ✅ Architecture analysis shows tightly-coupled workflow (not distinct services)

**Recommendation Confidence**: 95%

**Justification**:
1. ✅ Aligns with original 95% confidence decision
2. ✅ Matches web research best practices
3. ✅ Follows industry patterns for unified platforms
4. ✅ User expectation alignment
5. ✅ Clear architectural fit

**Implementation Risk**: Low

**Justification**:
1. ✅ Pre-release product (no customer impact)
2. ✅ Mechanical change (well-defined scope)
3. ✅ Automated tooling available (`make manifests`)
4. ✅ Comprehensive test suite for validation

---

## Related Documentation

- **Original Decision**: [001-crd-api-group-rationale.md](../architecture/decisions/001-crd-api-group-rationale.md) (Oct 6, 2025)
- **Domain Decision**: [DD-CRD-001-api-group-domain-selection.md](../architecture/decisions/DD-CRD-001-api-group-domain-selection.md) (Nov 30, 2025)
- **Notification Compliance**: [NOTIFICATION_API_GROUP_TRIAGE.md](NOTIFICATION_API_GROUP_TRIAGE.md) (Dec 13, 2025)

---

**Triage Date**: December 13, 2025
**Triaged By**: AI Assistant
**Status**: 🔍 **AWAITING DECISION**
**Next Action**: User decision on API group naming strategy
**Recommended Decision**: Migrate to single API group `kubernaut.ai/v1alpha1`


# Request: SP Timeout Passthrough Clarification

**From**: RemediationOrchestrator Team
**To**: SignalProcessing Team
**Date**: 2025-12-09
**Priority**: 🔵 Low
**Status**: ✅ SP Responded - Option C Approved (Not Needed)

---

## 📋 Context

During V1.0 compliance audit and cross-service timeout consistency review, we discovered inconsistency in how RO passes timeouts to child CRDs:

| CRD | Has Timeout in Spec? | RO Passes Timeout? | Status |
|---|---|---|---|
| SignalProcessing | ❌ No | ❌ No | ✅ **Option C: Not Needed** (bounded ops) |
| AIAnalysis | ❌ No | ❌ No | 🟡 In Progress (Option A approved) |
| WorkflowExecution | ✅ Yes | ✅ Yes | ✅ Consistent |
| Notification | N/A | N/A | ✅ Not applicable |

---

## ❓ Question for SP Team

**RemediationRequest already has** `spec.TimeoutConfig.RemediationProcessingTimeout`:

```go
// api/remediation/v1alpha1/remediationrequest_types.go
type TimeoutConfig struct {
    // Timeout for RemediationProcessing phase (default: 5m)
    RemediationProcessingTimeout metav1.Duration `json:"remediationProcessingTimeout,omitempty"`
    // ...
}
```

**Question**: Does SignalProcessing need timeout passthrough from RO?

---

### Option A: RO Passes Timeout to SignalProcessing Spec

```go
// In RO's createSignalProcessing()
sp := &signalprocessingv1.SignalProcessing{
    Spec: signalprocessingv1.SignalProcessingSpec{
        // ... other fields ...
        TimeoutConfig: &signalprocessingv1.SPTimeoutConfig{
            EnrichmentTimeout: rr.Status.TimeoutConfig.RemediationProcessingTimeout,
        },
    },
}
```

**Pros**:
- ✅ Consistent with WorkflowExecution pattern
- ✅ Operators can configure timeouts at RR level
- ✅ No timeout disconnect between RO and SP

**Cons**:
- ⚠️ Requires SP spec change
- ⚠️ Requires RO code change

---

### Option B: SignalProcessing Uses Own Defaults (No Passthrough)

```go
// SignalProcessing uses its own defaults, ignores RR.Status.TimeoutConfig.RemediationProcessingTimeout
// RO only uses RemediationProcessingTimeout for its own phase timeout detection
```

**Pros**:
- ✅ No code changes needed
- ✅ Enrichment is typically quick (sub-minute)
- ✅ SP is self-contained

**Cons**:
- ⚠️ Inconsistent with WE pattern
- ⚠️ RR.Status.TimeoutConfig.RemediationProcessingTimeout only affects RO's phase detection

---

### Option C: Not Needed (SP operations are always fast)

```
SignalProcessing enrichment operations are bounded by:
- Kubernetes API calls (typically <1s)
- Namespace/pod lookups (typically <1s)
- No long-running external API calls

Timeout passthrough adds complexity without benefit.
```

**Pros**:
- ✅ Simplest solution
- ✅ No changes needed

**Cons**:
- ⚠️ Inconsistent architecture (if we decide consistency matters)

---

## 🎯 RO Team Perspective

**Lean toward Option C** - SP enrichment is typically fast and bounded, unlike:
- AIAnalysis (LLM calls can hang indefinitely)
- WorkflowExecution (Tekton pipelines can take minutes)

However, we want SP team input on whether external enrichment sources (future) might need timeout configuration.

---

## 📝 Response Section

### SP Team Response

**Date**: 2025-12-09
**Responder**: SignalProcessing Team

**Decision**: [✅] Option C (Not needed)

**Rationale**:
```
SignalProcessing has NO unbounded or external network operations:

1. ALL K8s API calls use controller-runtime client with built-in timeouts
2. Rego policy evaluation is sandboxed with 5s timeout (DD-WORKFLOW-001 v1.9)
3. Classification/categorization is pure local computation (<10ms)

Evidence from signalprocessing_controller.go:
- Phases: Pending → Enriching → Classifying → Categorizing → Completed
- Each phase performs only:
  - K8s API lookups (Pod, Namespace, Deployment, PDB, HPA, NetworkPolicy)
  - Local OPA Rego evaluation (sandboxed)
  - Local classification logic

Total typical reconciliation time: <3 seconds

Unlike AIAnalysis (LLM calls can hang indefinitely) or WorkflowExecution
(Tekton pipelines can take minutes), SP operations are bounded by design.
```

**Future Considerations** (Option C chosen):
```
V1.0: No external enrichment sources planned. All enrichment is K8s-local.

If future versions add external enrichment (e.g., CMDB lookup, cloud provider APIs):
- Option A could be revisited
- Would require DD-SP-XXX architectural decision
- Not in current roadmap

For V1.0: Option C is architecturally sound.
```

**Additional Notes**:
```
SP aligns with RO team's assessment. Timeout passthrough adds complexity
without benefit for bounded operations. We recommend:

1. Document this decision as "Not Required" for SP
2. Keep architecture consistent where meaningful (WE, AIAnalysis need it; SP doesn't)
3. Revisit if external enrichment sources are added post-V1.0
```

---

## 📚 Related Documents

- `api/remediation/v1alpha1/remediationrequest_types.go` - TimeoutConfig definition
- `docs/services/crd-controllers/01-signalprocessing/crd-schema.md` - SP spec
- `docs/handoff/REQUEST_RO_TIMEOUT_PASSTHROUGH_CLARIFICATION.md` - AIAnalysis timeout request (Option A approved)
- `pkg/remediationorchestrator/creator/workflowexecution.go` - Reference implementation (lines 169-175)

---

## 📝 Document History

| Date | Author | Change |
|------|--------|--------|
| 2025-12-09 | RO Team | Initial request (part of cross-service timeout consistency review) |
| 2025-12-09 | SP Team | Response: **Option C approved** - Timeout not needed for bounded operations |


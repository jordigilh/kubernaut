# Request: RO Timeout Passthrough Clarification

**From**: AIAnalysis Team
**To**: RemediationOrchestrator Team
**Date**: 2025-12-09
**Priority**: 🟡 Medium
**Status**: ✅ RO Responded - Option A Approved (Conditional)

---

## 📋 Context

During V1.0 compliance audit, we discovered that AIAnalysis uses **annotations** for timeout configuration, which is:
1. **Inconsistent** with other CRDs (RO, WE use `spec.TimeoutConfig`)
2. **Security risk** (annotations can be modified mid-reconciliation)
3. **Not validated** (no kubebuilder validation on annotations)

We plan to migrate AIAnalysis to use `spec.TimeoutConfig` pattern.

---

## ❓ Question for RO Team

**RemediationRequest already has** `spec.TimeoutConfig.AIAnalysisTimeout`:

```go
// api/remediation/v1alpha1/remediationrequest_types.go
type TimeoutConfig struct {
    // Timeout for AIAnalysis phase (default: 10m)
    AIAnalysisTimeout metav1.Duration `json:"aiAnalysisTimeout,omitempty"`
    // ...
}
```

**Question**: When RO creates AIAnalysis, should it pass through the `AIAnalysisTimeout` to the new AIAnalysis CRD's `spec.TimeoutConfig`?

### Option A: RO Passes Timeout to AIAnalysis Spec

```go
// In RO's createAIAnalysis()
aiAnalysis := &aianalysisv1.AIAnalysis{
    Spec: aianalysisv1.AIAnalysisSpec{
        // ... other fields ...
        TimeoutConfig: &aianalysisv1.AIAnalysisTimeoutConfig{
            InvestigatingTimeout: rr.Spec.TimeoutConfig.AIAnalysisTimeout,
        },
    },
}
```

**Pros**:
- ✅ Consistent timeout chain from RR → AIAnalysis
- ✅ Operators can configure timeouts at RR level

**Cons**:
- ⚠️ Requires RO code change

---

### Option B: AIAnalysis Uses Own Defaults (No Passthrough)

```go
// AIAnalysis uses its own defaults, ignores RR.Spec.TimeoutConfig.AIAnalysisTimeout
// RO only uses AIAnalysisTimeout for its own phase timeout detection
```

**Pros**:
- ✅ No RO code change needed
- ✅ AIAnalysis is self-contained

**Cons**:
- ⚠️ RR.Spec.TimeoutConfig.AIAnalysisTimeout only affects RO's phase detection, not AIAnalysis behavior

---

## 🎯 AIAnalysis Team Recommendation

**Recommend Option A** (RO passes through) for consistency, but we need RO team confirmation before implementing.

---

## 📝 Response Section

### RO Team Response

**Date**: December 9, 2025
**Responder**: RO Team (via AI Assistant)

**Decision**: [✅] Option A (Pass through) / [ ] Option B (No passthrough)

**Rationale**:
```
1. CONSISTENCY: RO already passes WorkflowExecutionTimeout to WorkflowExecution CRD
   (see pkg/remediationorchestrator/creator/workflowexecution.go:169-175)

2. SINGLE SOURCE OF TRUTH: Operators configure timeout at RR level, flows downstream

3. NO DISCONNECT: Currently RO uses AIAnalysisTimeout for its own phase detection,
   but AIAnalysis doesn't know this timeout. This creates risk where RO timeouts
   AIAnalysis while AIAnalysis thinks it has more time.

4. SECURITY: Moving from annotations to validated spec fields is correct approach
```

**Implementation Notes** (if Option A):
```
⚠️ BLOCKING PREREQUISITE:
AIAnalysis CRD MUST add TimeoutConfig field to spec BEFORE RO can pass through.

Current api/aianalysis/v1alpha1/aianalysis_types.go has NO timeout field.

REQUIRED CHANGES:

1. AIAnalysis Team adds to aianalysis_types.go:

   type AIAnalysisSpec struct {
       // ... existing fields ...

       // TimeoutConfig for investigation phase (optional)
       // If nil, AIAnalysis controller uses defaults
       // +optional
       TimeoutConfig *AIAnalysisTimeoutConfig `json:"timeoutConfig,omitempty"`
   }

   type AIAnalysisTimeoutConfig struct {
       // Timeout for investigation (default: 10m)
       InvestigatingTimeout metav1.Duration `json:"investigatingTimeout,omitempty"`
   }

2. RO Team adds to pkg/remediationorchestrator/creator/aianalysis.go (after line 117):

   // Pass through timeout from RR if configured
   if rr.Spec.TimeoutConfig != nil && rr.Spec.TimeoutConfig.AIAnalysisTimeout.Duration > 0 {
       ai.Spec.TimeoutConfig = &aianalysisv1.AIAnalysisTimeoutConfig{
           InvestigatingTimeout: rr.Spec.TimeoutConfig.AIAnalysisTimeout,
       }
   }

3. Update documentation:
   - BR-ORCH-028 (Per-Phase Timeouts)
   - DD-TIMEOUT-001 (Global Remediation Timeout)
   - docs/services/crd-controllers/02-aianalysis/crd-schema.md

TIMELINE:
- AIAnalysis adds spec field: [AIAnalysis team to estimate]
- RO adds passthrough: 1 hour (after AIAnalysis field exists)
- Documentation updates: 30 minutes
```

### Cross-Reference: Current Inconsistency

| CRD | Timeout Passthrough from RO | Evidence |
|---|---|---|
| SignalProcessing | ❌ No | No TimeoutConfig in SP spec |
| AIAnalysis | ❌ No (proposed: ✅) | No TimeoutConfig in AI spec (yet) |
| WorkflowExecution | ✅ **Yes** | `creator/workflowexecution.go:169-175` |

### Action Items

| # | Owner | Action | Status |
|---|---|---|---|
| 1 | AIAnalysis Team | Add `TimeoutConfig` field to AIAnalysis spec | ⏳ Pending |
| 2 | RO Team | Add passthrough in `creator/aianalysis.go` | 🔒 Blocked on #1 |
| 3 | Both Teams | Update BR-ORCH-028, DD-TIMEOUT-001 docs | ⏳ After #2 |

---

## 📚 Related Documents

- `api/remediation/v1alpha1/remediationrequest_types.go` - TimeoutConfig definition
- `docs/services/crd-controllers/02-aianalysis/crd-schema.md` - AIAnalysis spec
- `docs/services/crd-controllers/02-aianalysis/reconciliation-phases.md` - Current annotation approach

---

## 📝 Document History

| Date | Author | Change |
|------|--------|--------|
| 2025-12-09 | AIAnalysis Team | Initial request |
| 2025-12-09 | RO Team | Response: Option A approved (conditional on AIAnalysis spec change) |


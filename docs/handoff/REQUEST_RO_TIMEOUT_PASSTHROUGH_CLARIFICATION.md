# Request: RO Timeout Passthrough Clarification

**From**: AIAnalysis Team
**To**: RemediationOrchestrator Team
**Date**: 2025-12-09
**Priority**: 🟡 Medium
**Status**: ⏳ Awaiting RO Response

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

**Date**: _____________
**Responder**: _____________

**Decision**: [ ] Option A (Pass through) / [ ] Option B (No passthrough)

**Rationale**:
```
[RO team to fill in]
```

**Implementation Notes** (if Option A):
```
[RO team to specify any constraints or requirements]
```

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


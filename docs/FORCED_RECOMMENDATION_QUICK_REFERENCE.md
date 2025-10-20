# Forced Recommendation V2 Feature - Quick Reference

**Status**: ✅ APPROVED FOR V2  
**Date**: October 20, 2025

---

## 📋 **WHAT IS THIS?**

**Problem**: In V1, when operators reject AI recommendations, they must use manual `kubectl` commands (bypassing Kubernaut tracking).

**Solution**: V2 will add `forcedRecommendation` and `bypassAIAnalysis` fields to allow operators to execute specific actions within Kubernaut.

---

## �� **DOCUMENTATION**

| Document | Purpose | Location |
|----------|---------|----------|
| **Business Requirement** | Detailed requirements and use cases | `docs/requirements/BR-RR-001-FORCED-RECOMMENDATION-MANUAL-OVERRIDE.md` |
| **Architecture Decision** | Technical design and rationale | `docs/architecture/decisions/ADR-026-forced-recommendation-manual-override.md` |
| **Feature Summary** | Complete overview and timeline | `docs/FORCED_RECOMMENDATION_V2_FEATURE_SUMMARY.md` |
| **Rejection Behavior** | V1 workarounds and V2 preview | `docs/APPROVAL_REJECTION_BEHAVIOR_DETAILED.md` |

---

## 🎯 **V1 vs V2**

### **V1 (Current) - Q4 2025**

**Rejection Flow**:
```
AI recommends → Operator rejects → Manual kubectl required
❌ No audit trail for manual actions
```

**Workaround**: Use `kubectl` directly (bypasses tracking)

---

### **V2 (Approved) - Q1-Q2 2026**

**Forced Recommendation Flow**:
```yaml
apiVersion: remediation.kubernaut.io/v1alpha1
kind: RemediationRequest
spec:
  forcedRecommendation:
    action: "scale-deployment"
    parameters:
      deployment: "webapp"
      targetReplicas: 3
    justification: "Resource constraints"
  bypassAIAnalysis: true
```

**Benefits**:
- ✅ Complete audit trail
- ✅ Effectiveness tracking
- ✅ Time savings (bypass AI)
- ✅ Safety validation (Rego)

---

## ⏱️ **TIMELINE**

| Phase | Timeline | Effort |
|-------|----------|--------|
| **V1 Launch** | Q4 2025 | - |
| **V1 Feedback** | Q1 2026 | - |
| **V2 Implementation** | Q1-Q2 2026 | 6 weeks |
| **V2 Launch** | Q2 2026 | - |

---

## 🔑 **KEY DECISIONS**

### **Why V2, Not V1?**

1. ✅ V1 validates core AI flow first
2. ✅ Gather usage data before adding override
3. ✅ Simpler V1 (faster to production)
4. ✅ V2 informed by V1 metrics

### **Safety First**

- ✅ Rego policies validate forced actions
- ✅ Production restrictions enforced
- ✅ Complete audit trail required

---

## 📊 **SUCCESS METRICS**

| Metric | Target | Purpose |
|--------|--------|---------|
| **Adoption** | 20% of rejections → forced retry | Measure feature usage |
| **Effectiveness** | 85% success rate | Compare AI vs operator |
| **Time Savings** | 1.5 min average | Validate bypass benefit |
| **Audit** | 100% complete | Ensure compliance |

---

## ✅ **APPROVAL**

**Business Requirement**: BR-RR-001 ✅  
**Architecture Decision**: ADR-026 ✅  
**Priority**: Medium  
**Target**: V2 (Q1-Q2 2026)  
**Effort**: 6 weeks

---

**For Complete Details**: See `docs/FORCED_RECOMMENDATION_V2_FEATURE_SUMMARY.md`

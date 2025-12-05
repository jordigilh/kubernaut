# 📋 NOTICE: Classifier Implementation Spec Violations

**From**: Gateway Team
**To**: Signal Processing Team
**Date**: 2025-12-05
**Priority**: 🔴 HIGH
**Affects**: `pkg/signalprocessing/classifier/classifier.go`

---

## 📝 Summary

During triage of environment categorization requirements, **8 specification violations** were identified in `classifier.go`. These issues affect environment classification (BR-SP-051-053) and priority assignment (BR-SP-070-072) functionality.

**Reference Documents**:
- [DD-WORKFLOW-001 v2.2](../architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md) - AUTHORITATIVE label schema
- [BR-SP-051-053](../services/crd-controllers/01-signalprocessing/BUSINESS_REQUIREMENTS.md) - Environment Classification
- [BR-SP-070-072](../services/crd-controllers/01-signalprocessing/BUSINESS_REQUIREMENTS.md) - Priority Assignment

---

## 🚨 Issues Requiring Immediate Attention

### Issue 1: Wrong BR References in Comments

**Location**: Lines 30-32, 107-108
**Severity**: 🟡 Medium

**Current Code**:
```go
// ENVIRONMENT CLASSIFIER (BR-SP-070)  ← WRONG
// ...
// PRIORITY CLASSIFIER (BR-SP-071)     ← WRONG
```

**Correct References**:
- Environment Classifier → **BR-SP-051-053**
- Priority Classifier → **BR-SP-070-072**

---

### Issue 2: Missing `kubernaut.ai/environment` Label Key

**Location**: Line 98
**Severity**: 🔴 High
**BR**: BR-SP-051

**Current Code**:
```go
envKeys := []string{"environment", "env", "deployment-environment", "app.kubernetes.io/environment"}
```

**Required per BR-SP-051**:
```go
envKeys := []string{"environment", "env", "kubernaut.ai/environment"}
```

**Issues**:
| Label Key | Status |
|-----------|--------|
| `environment` | ✅ Correct |
| `env` | ✅ Correct |
| `kubernaut.ai/environment` | ❌ **MISSING** (required) |
| `deployment-environment` | ❌ Not in spec (remove) |
| `app.kubernetes.io/environment` | ❌ Not in spec (remove) |

---

### Issue 3: Wrong Default Environment

**Location**: Lines 85-92
**Severity**: 🔴 High
**BR**: BR-SP-053

**Current Code**:
```go
return &EnvironmentClassification{
    Environment: "unknown",  // ❌ WRONG
    Confidence:  0.0,
    Source:      "default",
}
```

**Required per BR-SP-053**:
```go
return &EnvironmentClassification{
    Environment: "development",  // ✅ CORRECT
    Confidence:  0.4,            // Default fallback confidence
    Source:      "default",
}
```

**Note**: `"unknown"` is NOT a valid environment per DD-WORKFLOW-001. Valid values are: `production`, `staging`, `development`, `test`.

---

### Issue 4: Missing `test` Environment Category

**Location**: Lines 220-251
**Severity**: 🔴 High
**BR**: DD-WORKFLOW-001 v2.2

**Current**: Only 3 environment check functions exist:
- `isProductionEnvironment()`
- `isStagingEnvironment()`
- `isDevelopmentEnvironment()`

**Required**: 4 canonical environments per DD-WORKFLOW-001:

```yaml
environment:
  - production
  - staging
  - development
  - test        # ← MISSING FUNCTION
```

**Action**: Add `isTestEnvironment()` function.

---

### Issue 5: `test` Incorrectly Grouped with Development

**Location**: Line 244
**Severity**: 🔴 High

**Current Code**:
```go
devEnvs := []string{"development", "dev", "local", "test", "testing", "qa"}
                                          // ↑ WRONG - test is separate category
```

**Required**:
```go
// test is its own canonical environment, not development
devEnvs := []string{"development", "dev", "local"}
testEnvs := []string{"test", "testing", "qa"}  // New function needed
```

---

### Issue 6: Priority Matrix Mismatch

**Location**: Lines 135-141 (comment), Lines 195-198 (implementation)
**Severity**: 🔴 High
**BR**: BR-SP-071

**Current Matrix** (from code comment):
```
| Severity | Prod | Staging | Dev | Unknown |
|----------|------|---------|-----|---------|
| critical | P0   | P1      | P2  | P1      |
| warning  | P1   | P2      | P3  | P2      |
| info     | P3   | P3      | P3  | P3      |  ← WRONG
```

**Required Matrix** (from BR-SP-071):
```
| Severity \ Environment | production | staging | development | test |
|------------------------|------------|---------|-------------|------|
| critical               | P0         | P1      | P2          | P3   |
| warning                | P1         | P2      | P3          | P3   |
| info                   | P2         | P3      | P3          | P3   |
                         // ↑ P2 not P3!
```

**Key Discrepancy**:
- `info` severity in `production` should be **P2**, not P3
- Missing `test` column (should be P3 for all severities)

---

### Issue 7: Missing ConfigMap Fallback (BR-SP-052)

**Location**: `extractEnvironment()` function
**Severity**: 🟡 Medium
**BR**: BR-SP-052

**Current**: Only checks labels, no ConfigMap fallback.

**Required per BR-SP-052**:
```
- Load environment mapping from `kubernaut-environment-config` ConfigMap
- Support namespace name → environment mapping
- Support namespace pattern → environment mapping (regex)
- Hot-reload mapping without restart
```

**Note**: This may be a V1.1 feature. Please confirm scope.

---

### Issue 8: Hardcoded Aliases (Questionable)

**Location**: Lines 222-228, 232-239, 243-250
**Severity**: 🟢 Low

**Current**: Aliases are hardcoded:
```go
prodEnvs := []string{"production", "prod", "prd", "live"}
stagingEnvs := []string{"staging", "stage", "stg", "pre-prod", "preprod", "uat"}
devEnvs := []string{"development", "dev", "local", "test", "testing", "qa"}
```

**Concern**: DD-WORKFLOW-001 doesn't define aliases. Per BR-SP-052, aliases should come from ConfigMap for flexibility.

**Recommendation**: Consider making aliases configurable via ConfigMap or keep current hardcoded approach for V1.0 simplicity.

---

## ✅ What's Correct

| Feature | Status | Notes |
|---------|--------|-------|
| Priority fallback for unknown severity (P2) | ✅ Correct | |
| Confidence scoring structure | ✅ Correct | |
| Namespace → Signal → Default priority order | ✅ Correct | Per BR-SP-051 |
| ADR-041 compliance (no K8s calls in classifier) | ✅ Correct | |
| Structured logging | ✅ Correct | |

---

## 📊 Issue Summary

| # | Issue | Severity | BR Reference | Action |
|---|-------|----------|--------------|--------|
| 1 | Wrong BR references | 🟡 Medium | - | Fix comments |
| 2 | Missing `kubernaut.ai/environment` key | 🔴 High | BR-SP-051 | Add key, remove non-standard |
| 3 | Default is `unknown` not `development` | 🔴 High | BR-SP-053 | Change to `development` |
| 4 | Missing `test` environment category | 🔴 High | DD-WORKFLOW-001 | Add `isTestEnvironment()` |
| 5 | `test` grouped with development | 🔴 High | DD-WORKFLOW-001 | Separate `test` category |
| 6 | Priority matrix mismatch | 🔴 High | BR-SP-071 | Fix `info`+`prod`=P2, add `test` |
| 7 | No ConfigMap fallback | 🟡 Medium | BR-SP-052 | Implement or defer to V1.1 |
| 8 | Hardcoded aliases | 🟢 Low | - | Consider making configurable |

**Total**: 5 High, 2 Medium, 1 Low severity issues

---

## 🛠️ Recommended Fix Priority

1. **Immediate (blocks correctness)**:
   - Issue 2: Add `kubernaut.ai/environment` label key
   - Issue 3: Change default to `development`
   - Issue 4 & 5: Add `isTestEnvironment()` and separate `test` category
   - Issue 6: Fix priority matrix

2. **Soon (documentation/clarity)**:
   - Issue 1: Fix BR references in comments

3. **V1.1 or Deferred**:
   - Issue 7: ConfigMap fallback
   - Issue 8: Configurable aliases

---

## 📝 Acknowledgment Required

Please acknowledge receipt and provide estimated fix timeline:

### Signal Processing Team
- [ ] Acknowledged (Date: ___________)
- [ ] Estimated Fix Date: ___________
- [ ] Notes: ___________

---

## 📚 Related Documents

- [DD-WORKFLOW-001 v2.2](../architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md) - AUTHORITATIVE
- [BR-SP-051-053](../services/crd-controllers/01-signalprocessing/BUSINESS_REQUIREMENTS.md) - Environment Classification
- [BR-SP-070-072](../services/crd-controllers/01-signalprocessing/BUSINESS_REQUIREMENTS.md) - Priority Assignment
- [ADR-041](../architecture/decisions/ADR-041-rego-policy-data-fetching-separation.md) - K8s Enricher/Classifier separation

---

**Document Version**: 1.0
**Last Updated**: 2025-12-05
**Status**: PENDING ACKNOWLEDGMENT

---

## 📝 Changelog

| Version | Date | Change |
|---------|------|--------|
| 1.0 | 2025-12-05 | Initial notice from Gateway Team |


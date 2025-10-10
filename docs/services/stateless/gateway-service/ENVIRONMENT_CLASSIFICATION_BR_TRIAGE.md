# Environment Classification BR Alignment Triage

**Date**: October 10, 2025
**Scope**: BR-GATEWAY-051 to BR-GATEWAY-053 (Environment Classification)
**Issue**: Hardcoded environment values in implementation contradict dynamic configuration principle

---

## Executive Summary

### Root Cause
The `EnvironmentClassifier` implementation hardcoded valid environment values to `"prod"`, `"staging"`, `"dev"` (plus aliases), which defeats the purpose of using Kubernetes labels for **dynamic configuration**.

### Resolution
✅ **Code Fixed**: `classification.go` now accepts **ANY non-empty string** as a valid environment
⚠️ **Documentation Misaligned**: Several documents still reference hardcoded values

### Impact
- **Organizations can now define custom environments**: `canary`, `qa-eu`, `prod-west`, etc.
- **Downstream services (Rego, Priority)** are already designed to handle arbitrary environment values
- **No breaking changes**: Existing `prod`/`staging`/`dev` configurations continue to work

---

## Business Requirements Analysis

### BR-GATEWAY-051: Environment Detection (Namespace Labels)
**Definition**: K8s namespace label lookup with cache
**Testing**: Integration test: namespace label retrieval

**✅ ALIGNED**: BR does NOT restrict valid environment values. The BR states:
- Gateway reads the `environment` label from namespace
- No specification of valid values
- **Interpretation**: ANY non-empty label value is valid

**Confidence**: 95% - BR is correctly designed for dynamic configuration

---

### BR-GATEWAY-052: ConfigMap Fallback for Environment
**Definition**: ConfigMap override check
**Testing**: Unit test: ConfigMap precedence over labels

**✅ ALIGNED**: BR does NOT restrict valid environment values. The BR states:
- Gateway checks ConfigMap `kubernaut-environment-overrides` for namespace override
- No specification of valid values
- **Interpretation**: ANY non-empty ConfigMap value is valid

**Confidence**: 95% - BR is correctly designed for dynamic configuration

---

### BR-GATEWAY-053: Default Environment (Unknown)
**Definition**: Fallback value when no labels/ConfigMap
**Testing**: Unit test: unknown environment handling

**✅ ALIGNED**: BR explicitly specifies `"unknown"` as default. The implementation now returns `"unknown"` (previously returned `"dev"`).

**Confidence**: 100% - BR and implementation aligned

---

## Documentation Gaps (Misalignment)

### ❌ Gap 1: README.md Line 144
**Current**:
```markdown
| **Environment** | BR-GATEWAY-051 to BR-GATEWAY-053 | Environment classification (prod/staging/dev) |
```

**Issue**: `(prod/staging/dev)` restricts perceived valid values

**Fix Required**:
```markdown
| **Environment** | BR-GATEWAY-051 to BR-GATEWAY-053 | Environment classification (dynamic: any label value) |
```

---

### ❌ Gap 2: overview.md Line 250
**Current**:
```markdown
**Result**: "prod", "staging", "dev", or "unknown"
```

**Issue**: Lists specific values, implies these are the only valid ones

**Fix Required**:
```markdown
**Result**: Any non-empty environment string from labels/ConfigMap, or "unknown" if not found

**Examples**: "prod", "staging", "dev", "canary", "qa-eu", "prod-west", "blue", "green"
```

---

### ❌ Gap 3: overview.md Lines 273-279 (Fallback Table)
**Current**:
```markdown
**Fallback** (if Rego fails):
| Severity | Environment | Priority |
|----------|-------------|----------|
| critical | prod        | P0       |
| critical | staging/prod| P1       |
| warning  | prod        | P1       |
| warning  | staging     | P2       |
| info     | any         | P2       |
```

**Issue**: Table only shows `prod`, `staging` as examples, could be misinterpreted as exhaustive list

**Fix Required**: Add note clarifying these are **examples**, not exhaustive:
```markdown
**Fallback** (if Rego fails):
| Severity | Environment | Priority |
|----------|-------------|----------|
| critical | production* | P0       |
| critical | staging/production | P1       |
| warning  | production* | P1       |
| warning  | staging     | P2       |
| info     | any         | P2       |

*Note: "production" matches any environment containing "prod" (e.g., prod, production, prod-east).
Organizations define their own environment taxonomy. This table shows the **default fallback behavior**
when Rego policies are not configured. Rego policies can implement custom logic for any environment.
```

---

### ❌ Gap 4: implementation.md Line 1067 Comment
**Current**:
```go
// EnvironmentClassifier determines environment (prod/staging/dev) for alerts
```

**Issue**: Comment restricts valid values

**Fix Required**:
```go
// EnvironmentClassifier determines environment (any label value) for alerts
```

---

### ❌ Gap 5: testing-strategy.md Lines 499-503
**Current**:
```go
// BR-GATEWAY-052.5: prod-* namespace pattern matches production
Entry("prod-webapp namespace → inferred production with high confidence",
    "prod-webapp", "", "", "production", 0.85),

// BR-GATEWAY-052.6: staging-* namespace pattern matches staging
Entry("staging-api namespace → inferred staging with high confidence",
    "staging-api", "", "", "staging", 0.85),
```

**Issue**: These test cases show **pattern matching** (e.g., `prod-webapp` → `production`), but:
- overview.md:410 states "No Pattern Matching: Removed per user request"
- Current implementation does NOT do pattern matching
- Labels provide exact values, no inference

**Fix Required**: **DELETE** these test entries (BR-GATEWAY-052.5, 052.6) as they contradict the approved design:
```go
// ❌ REMOVED: Pattern matching not implemented per user request
// Entry("prod-webapp namespace → inferred production with high confidence",
//     "prod-webapp", "", "", "production", 0.85),
```

---

### ❌ Gap 6: implementation-plan.md Lines 497-498
**Current**:
```go
func isValidEnvironment(env string) bool {
    return env == "prod" || env == "staging" || env == "dev"
}
```

**Issue**: Shows hardcoded validation (now outdated)

**Fix Required**: Update to reflect current implementation:
```go
func isValidEnvironment(env string) bool {
    return env != ""  // Accept any non-empty string
}
```

---

### ❌ Gap 7: implementation-plan.md Lines 490-494 (Default Value)
**Current**:
```go
// 4. Default fallback
c.logger.WithFields(logrus.Fields{
    "namespace": namespace,
}).Debug("No environment label found, defaulting to 'dev'")

defaultEnv := "dev"
```

**Issue**: Default is `"dev"`, should be `"unknown"` per BR-GATEWAY-053

**Fix Required**:
```go
// 4. Default fallback
c.logger.WithFields(logrus.Fields{
    "namespace": namespace,
}).Warn("No environment label found, defaulting to 'unknown'")

defaultEnv := "unknown"
```

---

## Downstream Impact Analysis

### 1. Priority Assignment (priority.go)
**Current Behavior**:
- Uses Rego policy evaluation (primary)
- Falls back to hardcoded table if Rego fails

**Impact of Dynamic Environments**:
- ✅ **Rego policies** already accept arbitrary environments as input
- ⚠️ **Fallback table** currently only handles `"production"`, `"development"`, `"staging"`
- **Gap**: Fallback table should have a catch-all for unknown environments

**Recommendation**:
```go
fallbackTable := map[string]map[string]string{
    "critical": {
        "production":  "P0",
        "staging":     "P1",
        "development": "P2",
        "*":           "P1", // Catch-all: critical in any other environment → P1
    },
    "warning": {
        "production":  "P1",
        "staging":     "P2",
        "development": "P2",
        "*":           "P2", // Catch-all: warning in any other environment → P2
    },
    "info": {
        "*": "P3", // Catch-all: info in any environment → P3
    },
}
```

---

### 2. Remediation Path Decision (remediation_path.go)
**Current Behavior**:
- Uses Rego policy evaluation (primary)
- Falls back to hardcoded table if Rego fails

**Impact of Dynamic Environments**:
- ✅ **Rego policies** already accept arbitrary environments as input
- ⚠️ **Fallback table** needs catch-all for unknown environments

**Recommendation**: Similar catch-all pattern as priority.go

---

### 3. Rego Policies (config.app/gateway/policies/)
**Current Behavior**:
- `priority.rego`: Checks `input.environment == "production"`
- `remediation_path.rego`: Checks `input.environment == "production"`

**Impact of Dynamic Environments**:
- ✅ **Already supports arbitrary environments** - Organizations can add rules for `canary`, `qa`, etc.
- ✅ **Default rules** use `default` keyword for catch-all behavior
- **No changes required** - Policies are already flexible

**Example**: Organization can add custom rules:
```rego
# Custom rule for canary environment
priority := "P1" if {
    input.severity == "critical"
    input.environment == "canary"  # Custom environment
}

# Custom rule for qa-eu environment
priority := "P2" if {
    input.environment == "qa-eu"  # Custom environment
}
```

---

## Testing Gaps

### Existing Tests (environment_classification_test.go)
**Current Coverage**:
- ✅ Tests for `"prod"`, `"staging"`, `"dev"` (standard environments)
- ✅ Tests for `"production"` → normalized to `"prod"` (aliases)
- ✅ Tests for `"unknown"` (default fallback)

**Gap**: No tests for **custom/non-standard environments** like:
- `"canary"`
- `"qa-eu"`
- `"prod-west"`
- `"blue"`/`"green"` (deployment strategies)

**Recommendation**: Add test cases to verify dynamic configuration:
```go
It("accepts custom environment from namespace label for dynamic configuration", func() {
    // Business scenario: Organization uses canary deployments
    signal := &types.NormalizedSignal{
        Fingerprint: "abc123def456ghi789jkl012mno345",
        AlertName:   "HighLatency",
        Severity:    "warning",
        Namespace:   "canary-payment-api",  // Custom namespace
        // ...
    }

    // Mock: Namespace has custom environment label
    mockNamespace := &corev1.Namespace{
        ObjectMeta: metav1.ObjectMeta{
            Name: "canary-payment-api",
            Labels: map[string]string{
                "environment": "canary",  // Custom environment value
            },
        },
    }
    // ... mock setup ...

    env := classifier.Classify(ctx, "canary-payment-api")

    // BUSINESS OUTCOME: Gateway accepts ANY non-empty environment
    Expect(env).To(Equal("canary"),
        "Dynamic configuration: Organizations define their own environment taxonomy")
})

It("accepts regional environment from ConfigMap for multi-region deployments", func() {
    // Business scenario: Organization has region-specific environments
    // ... test for "prod-eu-west", "prod-us-east", etc.
})

It("accepts deployment strategy environment for blue/green deployments", func() {
    // Business scenario: Organization uses blue/green deployment strategy
    // ... test for "blue", "green", etc.
})
```

---

## Remediation Plan

### Phase 1: Documentation Updates (Effort: 30 minutes)

1. **README.md**:
   - Line 144: Change `(prod/staging/dev)` → `(dynamic: any label value)`

2. **overview.md**:
   - Line 250: Update result description to include examples of custom environments
   - Lines 273-279: Add clarifying note that fallback table is for default behavior

3. **implementation.md**:
   - Line 1067: Update comment to reflect dynamic configuration

4. **testing-strategy.md**:
   - Lines 499-503: **DELETE** pattern matching test entries (052.5, 052.6)

5. **implementation-plan.md**:
   - Lines 497-498: Update `isValidEnvironment()` implementation
   - Lines 490-494: Update default value from `"dev"` to `"unknown"`

---

### Phase 2: Fallback Table Enhancement (Effort: 1 hour)

1. **priority.go**:
   - Add catch-all `"*"` entries to fallback table
   - Update tests to cover unknown environments

2. **remediation_path.go**:
   - Add catch-all `"*"` entries to fallback table
   - Update tests to cover unknown environments

---

### Phase 3: Test Coverage (Effort: 1 hour)

1. **Add unit tests** for custom environments:
   - `canary` environment
   - Regional environments (`qa-eu`, `prod-west`)
   - Deployment strategies (`blue`, `green`)

2. **Add integration test** for custom environment end-to-end flow

---

## Confidence Assessment

| Component | Alignment Status | Confidence | Risk |
|-----------|-----------------|------------|------|
| **BR-GATEWAY-051** | ✅ Aligned | 95% | Low - BR correctly designed |
| **BR-GATEWAY-052** | ✅ Aligned | 95% | Low - BR correctly designed |
| **BR-GATEWAY-053** | ✅ Aligned | 100% | None - Exact match |
| **Implementation (classification.go)** | ✅ Aligned | 100% | None - Fixed |
| **Documentation** | ❌ Misaligned | 40% | Medium - Needs updates |
| **Downstream (Priority)** | ⚠️ Partial | 70% | Low - Needs catch-all |
| **Downstream (Rego)** | ✅ Aligned | 95% | Low - Already flexible |
| **Testing** | ⚠️ Partial | 60% | Low - Needs custom env tests |

---

## Conclusion

### Summary
The **Business Requirements (BR-GATEWAY-051 to 053) are correctly designed** and do NOT restrict valid environment values. The issue was in the **implementation** (now fixed) and **documentation** (needs updates).

### Key Insight
**Labels are for DYNAMIC configuration, not static validation.** Hardcoding valid values defeats the purpose of Kubernetes labels as a configuration mechanism.

### Next Steps
1. ✅ **Code**: Fixed (classification.go accepts any non-empty string)
2. ⏳ **Documentation**: Update 6 documents (30 minutes)
3. ⏳ **Downstream**: Add catch-all to fallback tables (1 hour)
4. ⏳ **Testing**: Add custom environment test cases (1 hour)

**Total Effort**: ~2.5 hours

---

**Document Status**: ✅ Complete
**Review**: Recommended for user approval before proceeding with Phase 1



# DD-SEVERITY-001: Strategy B (Policy-Defined Fallback) - Architectural Decision

**Date**: 2026-01-13
**Status**: ✅ **APPROVED** (User Decision)
**Phase**: RED Phase Test Updates
**Confidence**: 98%

---

## 🎯 **Decision Summary**

**Approved Strategy**: **Strategy B (Policy-Defined Fallback)**

Operators must define fallback behavior for unmapped severity values in their Rego policies. The system does **NOT** impose a default "unknown" fallback.

---

## 📋 **Context & Problem**

### **Original Test Design** (Strategy A)
Initial tests included system-defined fallback to "unknown" for unmapped severity values:

```go
// System decides: unmapped severity → "unknown"
if result == nil {
    return &SeverityResult{Severity: "unknown", Source: "fallback"}
}
```

### **User Concern**
> "I'm not convinced on the unknown value, it is being opinionated on something users need to define."

**Valid Concern**: System-defined "unknown" fallback removes operator control and imposes opinionated behavior.

---

## ⚖️ **Three Alternatives Considered**

### **Strategy A: System-Defined Fallback** ❌ REJECTED
- **Pros**: Simple, graceful degradation
- **Cons**: Opinionated, inflexible, removes operator control
- **User Decision**: Rejected - too opinionated

---

### **Strategy B: Policy-Defined Fallback** ✅ **APPROVED**
- **Approach**: Operator defines fallback in Rego policy, system errors if no severity determined
- **Pros**: Maximum flexibility, operator control, aligns with Kubernetes philosophy
- **Cons**: Requires operator thought (mitigated by clear error messages)
- **User Decision**: **APPROVED**

**Example Rego Policies**:

**Conservative (Safety-First)**:
```rego
determine_severity := "critical" {
    input.signal.severity == "Sev1"
} else := "warning" {
    input.signal.severity == "Sev2"
} else := "critical" {  # Unmapped → escalate for safety
    true
}
```

**Permissive (Ignore Unknown)**:
```rego
determine_severity := "critical" {
    input.signal.severity == "Sev1"
} else := "warning" {
    input.signal.severity == "Sev2"
} else := "info" {  # Unmapped → informational only
    true
}
```

---

### **Strategy C: Hybrid (Default + Override)** ❌ NOT SELECTED
- **Approach**: Ship default policy with "unknown" fallback, operator can override
- **Pros**: Works out-of-box, customizable
- **Cons**: Still opinionated about default, two code paths
- **User Decision**: Not selected (Strategy B preferred)

---

## 🔧 **Implementation Requirements**

### **Severity Classifier Behavior**

```go
// In pkg/signalprocessing/classifier/severity.go (GREEN phase)
func (c *SeverityClassifier) ClassifySeverity(ctx context.Context, sp *SignalProcessing) (*SeverityResult, error) {
    // No policy loaded → error
    if c.regoEngine == nil || !c.regoEngine.HasPolicy() {
        return nil, fmt.Errorf("no policy loaded - severity determination requires Rego policy")
    }

    result, err := c.regoEngine.Evaluate(ctx, input)
    if err != nil {
        return nil, fmt.Errorf("rego evaluation failed: %w", err)
    }

    // Policy returned no severity → error
    if result == "" {
        return nil, fmt.Errorf("no severity determined by policy for input %q - add else clause to policy for unmapped values", sp.Spec.Signal.Severity)
    }

    // Validate severity is valid enum
    if !isValidSeverity(result) {
        return nil, fmt.Errorf("policy returned invalid severity %q - must be critical/warning/info", result)
    }

    return &SeverityResult{
        Severity: result,
        Source:   "rego-policy",
    }, nil
}
```

### **Error Messages**

Clear, actionable error messages guide operators to fix policies:

```
Error: no severity determined by policy for input "UNMAPPED_VALUE"
Hint: Add else clause to policy for unmapped values
Example:
  } else := "critical" {  # Conservative fallback
      true
  }
```

### **Audit Trail**

Audit events always show `determination_source: "rego-policy"` (never "fallback" or "system"):

```json
{
  "event_type": "classification.decision",
  "event_data": {
    "external_severity": "CustomValue",
    "normalized_severity": "critical",
    "determination_source": "rego-policy",
    "policy_hash": "abc123..."
  }
}
```

---

## 📊 **Test Updates Completed**

### **Unit Tests** (3 tests updated)
1. **Test: Require policy-defined fallback** (formerly "should fall back to 'unknown'")
   - Now tests conservative policy with explicit catch-all clause
   - Validates operator-defined fallback (critical/warning/info)
   - Removes system "unknown" fallback

2. **Test: Error when policy returns no severity** (NEW)
   - Tests incomplete policy (missing catch-all)
   - Validates clear error message guides operator

3. **Test: Error when no policy loaded** (formerly "operate with no policy")
   - Now requires policy to be loaded (mandatory)
   - No silent default behavior

### **Integration Tests** (4 tests updated)
1. **Test: Policy-defined fallback audit event** (formerly "fallback to 'unknown'")
   - Validates audit shows rego-policy source (not "fallback")
   - Confirms normalized severity is critical/warning/info

2. **Test: Error handling transitions to Failed phase**
   - No fallback to "unknown" on error
   - Status.Severity remains empty (no system fallback)

3. **Test: Hot-reload removes Skip()** (Gap 1 fix)
   - Confirms hot-reload infrastructure exists
   - Validates pattern from environment/priority classifiers

4. **Test: All "unknown" references removed**
   - CRD status validation: critical/warning/info only
   - Parallel execution: critical/warning/info only

### **E2E Tests** (No changes required)
- E2E tests focus on workflow integration
- Not concerned with fallback strategy details

---

## ✅ **Business Benefits**

| Benefit | Conservative Policy | Permissive Policy |
|---|---|---|
| **Safety Posture** | Unmapped → critical (escalate) | Unmapped → info (ignore) |
| **Use Case** | Production systems (safety-first) | Development/testing (permissive) |
| **Operator Control** | ✅ Full control | ✅ Full control |
| **Compliance** | Audit shows policy enforcement | Audit shows policy enforcement |
| **Flexibility** | Customer defines fallback | Customer defines fallback |

---

## 🚫 **Removed System Behaviors**

### **Before (Strategy A)**
```go
// System imposed "unknown" fallback
return &SeverityResult{
    Severity: "unknown",  // System decision
    Source:   "fallback", // System fallback
}
```

### **After (Strategy B)**
```go
// Operator defines fallback in policy
determine_severity := "critical" {  # Operator decision
    # mappings...
} else := "critical" {  # Operator fallback
    true
}

// System errors if no result
if result == "" {
    return nil, fmt.Errorf("no severity determined - add else clause")
}
```

---

## 🎯 **Alignment with Existing Patterns**

Strategy B follows established SignalProcessing patterns:

| Component | Policy Path | Mandatory? | Hot-Reload? | Fallback Strategy |
|---|---|---|---|---|
| **Environment Classifier** | `/etc/signalprocessing/policies/environment.rego` | ✅ Yes | ✅ Yes (line 205) | Policy-defined |
| **Priority Assigner** | `/etc/signalprocessing/policies/priority.rego` | ✅ Yes | ✅ Yes (line 234) | Policy-defined |
| **CustomLabels Extractor** | `/etc/signalprocessing/policies/customlabels.rego` | ✅ Yes | ✅ Yes (line 272) | Policy-defined |
| **Severity Classifier** | `/etc/signalprocessing/policies/severity.rego` | ✅ Yes | ✅ Yes (GREEN) | **Policy-defined** ✅ |

**Consistency**: All Rego-based classifiers use policy-defined behavior (no system fallbacks).

---

## 📚 **Documentation References**

- **Design Decision**: `docs/architecture/decisions/DD-SEVERITY-001-severity-determination-refactoring.md`
- **Test Plan**: `docs/handoff/DD_SEVERITY_001_TEST_PLAN_JAN11_2026.md` (v3.0)
- **Existing Pattern**: `cmd/signalprocessing/main.go` lines 189-278 (environment/priority/customlabels hot-reload)
- **Business Requirements**: BR-SP-105 (Severity Determination via Rego Policy)

---

## ⏱️ **Timeline Impact**

**No timeline impact**: Strategy B complexity is similar to Strategy A.

| Phase | Duration | Status |
|---|---|---|
| **RED** | Days 1-2 | ✅ COMPLETE (tests updated) |
| **GREEN** | Days 3-4 | 🔜 READY (implementation) |
| **REFACTOR** | Day 5 | 🔜 PENDING (enhancements) |

---

## 🎯 **GREEN Phase Implementation Checklist**

### **Severity Classifier** (`pkg/signalprocessing/classifier/severity.go`)
- [ ] `NewSeverityClassifier()` constructor
- [ ] `ClassifySeverity()` method
- [ ] `LoadRegoPolicy()` method
- [ ] Error when no policy loaded
- [ ] Error when policy returns empty result
- [ ] Validate returned severity is valid enum

### **Rego Engine** (extend `pkg/signalprocessing/rego/engine.go`)
- [ ] Add severity determination query
- [ ] Reuse existing hot-reload infrastructure
- [ ] Policy hash tracking (SHA256)

### **Controller Integration** (`internal/controller/signalprocessing/signalprocessing_controller.go`)
- [ ] Wire SeverityClassifier into reconciler
- [ ] Call during Classifying phase
- [ ] Update Status.Severity field
- [ ] Emit classification.decision audit event

### **Main Application** (`cmd/signalprocessing/main.go`)
- [ ] Load severity policy at startup
- [ ] Start hot-reload (line ~240, after priority)
- [ ] Fail fast if policy missing/invalid

---

## ✅ **Confidence Assessment**

**Confidence**: **98%** (RED phase complete with Strategy B)

**Justification**:
- ✅ All tests updated to remove system "unknown" fallback
- ✅ Tests validate operator-defined fallback behavior
- ✅ Error messages guide operator to fix incomplete policies
- ✅ Aligns with existing environment/priority/customlabels patterns
- ✅ Strategy B approved by user

**Remaining 2% Risk**:
- Implementation complexity may reveal edge cases (mitigated by existing Rego patterns)

---

## 📖 **Related Decisions**

- **Gap 1 (Hot-Reload)**: Infrastructure exists, `Skip()` removed ✅
- **Gap 2 (Timeout)**: Existing 5s timeout sufficient ✅
- **Gap 3 (Fallback)**: Strategy B approved ✅
- **Gap 4 (Case Sensitivity)**: Deferred to REFACTOR phase ✅

---

## 🎉 **Ready for GREEN Phase**

**Status**: ✅ **READY FOR IMPLEMENTATION**

All test updates complete, architectural decision documented, ready to proceed with minimal implementation to pass 21 tests.

**Next Step**: Begin GREEN phase (Days 3-4) - Implement severity classifier with policy-defined fallback.

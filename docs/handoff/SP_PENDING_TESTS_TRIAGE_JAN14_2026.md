# SignalProcessing Pending Tests - Triage

**Date**: January 14, 2026
**Service**: SignalProcessing
**Issue**: 2 pending integration tests
**Status**: TRIAGED

---

## EXECUTIVE SUMMARY

**Current Test Status**: 90/92 passed, **2 pending**

**Pending Tests**:
1. **Policy Hash in Audit Event** - Line 372 (`PIt`)
2. **Failed Phase on Policy Error** - Line 432 (`PIt`)

**Can We Enable Them Now?**
- **Test #1**: ❌ **NO** - Requires schema change
- **Test #2**: ⚠️ **PARTIALLY** - Requires controller logic change

---

## TEST #1: Policy Hash in Audit Event

### Test Details

**File**: `test/integration/signalprocessing/severity_integration_test.go`
**Line**: 372
**Test Name**: `should include policy hash in audit event for policy version traceability`
**Status**: `PIt` (Pending)

### Business Context

**Business Need**: Compliance audit trail for severity determination decisions

**Business Value**:
- Operators can correlate severity decisions with specific Rego policy versions
- Change management audit requires policy version tracking
- Troubleshooting: "What policy version made this decision?"

**Compliance**: Change management requirements

### Current Implementation Status

#### ✅ **Policy Hash EXISTS in Business Logic**

**Location**: `pkg/signalprocessing/classifier/severity.go`

```go
type SeverityResult struct {
    Severity   string  // "critical" | "warning" | "info"
    Source     string  // "rego-policy"
    PolicyHash string  // SHA256 hash of Rego policy ✅
}
```

**Methods Available**:
- `GetPolicyHash()` - Implemented in `SeverityClassifier`
- `PolicyHash` is populated in `SeverityResult` (line 176)
- Logged during hot-reload (line 208)

#### ❌ **Policy Hash MISSING from Audit Schema**

**Location**: `pkg/datastorage/ogen-client/oas_schemas_gen.go`

**Current Schema** (`SignalProcessingAuditPayload`):
```go
type SignalProcessingAuditPayload struct {
    EventType           string
    Phase               string
    Signal              string
    Severity            OptString
    ExternalSeverity    OptString
    NormalizedSeverity  OptString
    DeterminationSource OptString
    Environment         OptString
    // ... other fields ...

    // ❌ MISSING: PolicyHash OptString
}
```

#### ❌ **Policy Hash NOT Emitted in Audit Client**

**Location**: `pkg/signalprocessing/audit/client.go`

**Current Code** (lines 221-295):
```go
func (c *AuditClient) RecordClassificationDecision(ctx context.Context, sp *SignalProcessing, durationMs int) {
    payload := api.SignalProcessingAuditPayload{
        EventType: EventTypeClassificationDecision,
        Signal:    sp.Spec.Signal.Name,
        Phase:     toSignalProcessingAuditPayloadPhase(string(sp.Status.Phase)),
    }

    // Sets: Severity, ExternalSeverity, NormalizedSeverity, etc.

    // ❌ MISSING: payload.PolicyHash.SetTo(severityResult.PolicyHash)

    // Build audit event...
}
```

**Problem**: The `PolicyHash` from `SeverityResult` is not being passed to the audit event.

### What's Needed to Enable Test #1

#### Step 1: Update OpenAPI Schema

**File**: `holmesgpt-api/openapi/audit-event.yaml` (or equivalent)

**Add field to `SignalProcessingAuditPayload`**:
```yaml
components:
  schemas:
    SignalProcessingAuditPayload:
      properties:
        # ... existing fields ...
        policy_hash:
          type: string
          description: "SHA256 hash of Rego policy used for severity determination"
          pattern: "^[a-f0-9]{64}$"
          example: "a3b5c8d9e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8"
```

#### Step 2: Regenerate Ogen Client

**Command**:
```bash
cd holmesgpt-api
make generate
```

**Result**: `PolicyHash OptString` field added to `SignalProcessingAuditPayload`

#### Step 3: Update Audit Client to Emit PolicyHash

**File**: `pkg/signalprocessing/audit/client.go`

**Modify `RecordClassificationDecision()`**:
```go
func (c *AuditClient) RecordClassificationDecision(ctx context.Context, sp *SignalProcessing, durationMs int, severityResult *SeverityResult) {
    payload := api.SignalProcessingAuditPayload{
        // ... existing fields ...
    }

    // Add policy hash if severity was determined
    if severityResult != nil && severityResult.PolicyHash != "" {
        payload.PolicyHash.SetTo(severityResult.PolicyHash)
    }

    // ... rest of function
}
```

**Note**: The function signature needs to accept `severityResult` parameter or access it from `sp.Status`.

#### Step 4: Update Controller to Pass SeverityResult

**Current Issue**: Controller doesn't store `severityResult` in `sp.Status`, it only stores `severityResult.Severity`.

**Options**:
1. **Option A**: Add `PolicyHash` field to `SignalProcessing` CRD Status
2. **Option B**: Pass `severityResult` to audit function (requires refactoring)

**Recommendation**: **Option A** - Add to CRD Status for audit trail

**Changes Needed**:
```go
// api/signalprocessing/v1alpha1/signalprocessing_types.go
type SignalProcessingStatus struct {
    // ... existing fields ...
    Severity   string `json:"severity,omitempty"`
    PolicyHash string `json:"policyHash,omitempty"` // NEW
}

// internal/controller/signalprocessing/signalprocessing_controller.go
if severityResult != nil {
    sp.Status.Severity = severityResult.Severity
    sp.Status.PolicyHash = severityResult.PolicyHash // NEW
}

// pkg/signalprocessing/audit/client.go
if sp.Status.PolicyHash != "" {
    payload.PolicyHash.SetTo(sp.Status.PolicyHash) // NEW
}
```

#### Step 5: Enable Test

**File**: `test/integration/signalprocessing/severity_integration_test.go`

**Change**: `PIt` → `It` (line 372)

**Update Test**:
```go
It("should include policy hash in audit event for policy version traceability", func() {
    // ... existing test code ...

    payload := event.EventData.SignalProcessingAuditPayload

    // Now we can validate policy hash
    Expect(payload.PolicyHash.IsSet()).To(BeTrue(),
        "PolicyHash should be set for audit trail")
    Expect(payload.PolicyHash.Value).To(MatchRegexp(`^[a-f0-9]{64}$`),
        "PolicyHash should be valid SHA256 hash")
})
```

### Confidence Assessment

**Can we enable now?** ❌ **NO**

**Effort Required**: Medium (3-4 hours)

**Complexity**:
- ⚠️ Requires OpenAPI schema change
- ⚠️ Requires CRD schema change
- ⚠️ Requires controller logic change
- ⚠️ Requires audit client change
- ✅ Business logic already implemented

**Risk**: Low - Additive change, no breaking changes

---

## TEST #2: Failed Phase on Policy Error

### Test Details

**File**: `test/integration/signalprocessing/severity_integration_test.go`
**Line**: 432
**Test Name**: `should transition to Failed phase if Rego policy evaluation fails persistently`
**Status**: `PIt` (Pending)

### Business Context

**Business Need**: Graceful degradation when Rego policy has bugs

**Business Value**:
- SignalProcessing CRD transitions to `Failed` phase for operator visibility
- Prevents silent failures that hide severity determination issues
- Operator is alerted to fix Rego policy bug

**Compliance**: Error visibility requirements

### Why Test is Pending

**Reason from Code** (lines 427-431):
```go
// DD-SEVERITY-001 REFACTOR NOTE: This test is pending because Strategy B requires
// operators to define fallback behavior in policy. With the fallback clause,
// unmapped severities no longer cause policy evaluation failures.
// To test true policy errors, we'd need to test with malformed Rego syntax,
// which is already covered in unit tests.
```

**Translation**:
- **DD-SEVERITY-001 Strategy B**: Rego policies now have fallback clauses (e.g., `else = "warning"`)
- **Result**: Unmapped severities (like `"trigger-error"`) don't cause policy failures anymore - they use the fallback
- **To test policy errors**: Would need malformed Rego syntax, which is covered in unit tests

### Current Implementation Status

#### ✅ **Error Handling EXISTS in Controller**

**Location**: `internal/controller/signalprocessing/signalprocessing_controller.go`

**Lines 538-548**:
```go
if r.SeverityClassifier != nil {
    severityResult, err = r.SeverityClassifier.ClassifySeverity(ctx, sp)
    if err != nil {
        // DD-005: Track phase processing failure
        r.Metrics.IncrementProcessingTotal("classifying", "failure")
        r.Metrics.ObserveProcessingDuration("classifying", time.Since(classifyingStart).Seconds())
        logger.Error(err, "Severity determination failed",
            "externalSeverity", signal.Severity,
            "hint", "Check Rego policy has else clause for unmapped values")
        return ctrl.Result{}, err // ❌ Just returns error, doesn't transition to Failed
    }
}
```

#### ❌ **Failed Phase Transition NOT Implemented**

**Problem**: When `ClassifySeverity` returns an error:
- Controller logs the error ✅
- Controller returns the error ✅
- Controller **DOES NOT** transition to `PhaseFailed` ❌
- CRD stays in `Classifying` phase with error logged

**Expected Behavior**:
```go
if err != nil {
    // Log error
    logger.Error(err, "Severity determination failed")

    // Transition to Failed phase
    updateErr := r.StatusManager.AtomicStatusUpdate(ctx, sp, func() error {
        sp.Status.Phase = signalprocessingv1alpha1.PhaseFailed
        sp.Status.Error = fmt.Sprintf("policy evaluation failed: %v", err)
        sp.Status.Severity = "" // Don't set severity when determination fails
        return nil
    })
    if updateErr != nil {
        return ctrl.Result{}, updateErr
    }

    // Emit error audit event
    if r.AuditClient != nil {
        r.AuditClient.RecordError(ctx, sp, err)
    }

    return ctrl.Result{}, nil // Don't requeue - terminal state
}
```

### What's Needed to Enable Test #2

#### Step 1: Add Failed Phase Transition Logic

**File**: `internal/controller/signalprocessing/signalprocessing_controller.go`

**Modify `reconcileClassifying()` lines 538-548**:
```go
if r.SeverityClassifier != nil {
    severityResult, err = r.SeverityClassifier.ClassifySeverity(ctx, sp)
    if err != nil {
        // DD-005: Track phase processing failure
        r.Metrics.IncrementProcessingTotal("classifying", "failure")
        r.Metrics.ObserveProcessingDuration("classifying", time.Since(classifyingStart).Seconds())
        logger.Error(err, "Severity determination failed - transitioning to Failed phase",
            "externalSeverity", signal.Severity,
            "hint", "Check Rego policy syntax and fallback clauses")

        // Transition to Failed phase (graceful degradation)
        updateErr := r.StatusManager.AtomicStatusUpdate(ctx, sp, func() error {
            sp.Status.Phase = signalprocessingv1alpha1.PhaseFailed
            sp.Status.Error = fmt.Sprintf("policy evaluation failed: %v", err)
            sp.Status.Message = "Severity determination failed due to policy error"
            // Don't set severity when determination fails (no system fallback)
            return nil
        })
        if updateErr != nil {
            return ctrl.Result{}, updateErr
        }

        // Emit error audit event
        if r.AuditClient != nil {
            r.AuditClient.RecordErrorOccurred(ctx, sp, err.Error())
        }

        // Don't requeue - Failed is a terminal state
        return ctrl.Result{}, nil
    }
}
```

#### Step 2: Test with Malformed Rego Policy

**Challenge**: How to trigger a policy error in integration tests?

**Options**:
1. **Option A**: Create a malformed Rego policy file and hot-reload it during test
2. **Option B**: Mock the `SeverityClassifier` to return an error
3. **Option C**: Create a special test ConfigMap with invalid Rego syntax

**Recommendation**: **Option C** - Use ConfigMap with invalid Rego

**Test Setup**:
```go
PIt("should transition to Failed phase if Rego policy evaluation fails persistently", func() {
    // GIVEN: Create ConfigMap with malformed Rego policy
    badPolicyConfig := &corev1.ConfigMap{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "bad-severity-policy",
            Namespace: "kubernaut-system",
        },
        Data: map[string]string{
            "severity.rego": `
                package severity
                # Malformed: missing closing bracket
                severity[signal] {
                    signal.severity == "critical"
                # ← SYNTAX ERROR
            `,
        },
    }
    Expect(k8sClient.Create(ctx, badPolicyConfig)).To(Succeed())

    // Allow policy hot-reload to detect error
    time.Sleep(2 * time.Second)

    // WHEN: Create SignalProcessing CRD
    sp := createTestSignalProcessingCRD(namespace, "test-policy-error")
    sp.Spec.Signal.Severity = "critical"
    Expect(k8sClient.Create(ctx, sp)).To(Succeed())

    // THEN: CRD transitions to Failed phase
    Eventually(func(g Gomega) {
        var updated signalprocessingv1alpha1.SignalProcessing
        g.Expect(k8sClient.Get(ctx, types.NamespacedName{
            Name:      sp.Name,
            Namespace: sp.Namespace,
        }, &updated)).To(Succeed())

        g.Expect(updated.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseFailed),
            "SignalProcessing should transition to Failed phase on policy error")
        g.Expect(updated.Status.Error).To(ContainSubstring("policy evaluation failed"),
            "Error message should explain policy evaluation failure")
        g.Expect(updated.Status.Severity).To(BeEmpty(),
            "Status.Severity should remain empty when determination fails")
    }, "60s", "2s").Should(Succeed())

    // Cleanup bad policy
    Expect(k8sClient.Delete(ctx, badPolicyConfig)).To(Succeed())
})
```

#### Step 3: Enable Test

**File**: `test/integration/signalprocessing/severity_integration_test.go`

**Change**: `PIt` → `It` (line 432)

### Confidence Assessment

**Can we enable now?** ⚠️ **PARTIALLY**

**Effort Required**: Medium (2-3 hours)

**Complexity**:
- ✅ Error handling exists in controller
- ❌ Failed phase transition NOT implemented
- ⚠️ Test setup requires malformed Rego policy injection
- ⚠️ Hot-reload timing makes test flaky

**Risk**: Medium
- Failed phase transition is additive (low risk)
- Test setup with malformed policy could affect other tests if not isolated
- Hot-reload timing introduces non-determinism

**Alternative Approach**:
- Keep test pending until we have a better mechanism for injecting policy errors
- Consider adding a "test mode" flag that allows simulating policy errors without hot-reload

---

## RECOMMENDATIONS

### Test #1: Policy Hash

**Recommendation**: **Implement when audit compliance requirements are prioritized**

**Priority**: P2 - Important for audit trail, but not blocking

**Effort**: 3-4 hours (schema changes + CRD changes + controller logic)

**Value**: High - Compliance audit trail for policy version tracking

### Test #2: Failed Phase

**Recommendation**: **Implement Failed phase transition, but keep test pending**

**Priority**: P1 - Graceful degradation is important

**Effort**:
- 2-3 hours for Failed phase transition logic
- Additional effort for reliable test setup (TBD)

**Value**: High - Operator visibility into policy errors

**Action Items**:
1. **Implement Failed phase transition** in controller (lines 538-548)
2. **Keep test pending** until we have a reliable mechanism for injecting policy errors
3. **Consider design decision** for test mode flag or better error injection

---

## SUMMARY

| Test | Can Enable Now? | Effort | Priority | Blocker |
|---|---|---|---|---|
| **Policy Hash in Audit Event** | ❌ NO | 3-4h | P2 | Schema changes + CRD changes |
| **Failed Phase on Policy Error** | ⚠️ Partial | 2-3h | P1 | Test setup complexity |

**Total Pending**: 2 tests
**Recommended Action**:
1. Implement Failed phase transition (Test #2 logic) - **High priority**
2. Keep both tests pending until fully implementable
3. Create follow-up tickets for completion

---

## REFERENCES

- **Test File**: `test/integration/signalprocessing/severity_integration_test.go`
- **Controller**: `internal/controller/signalprocessing/signalprocessing_controller.go`
- **Audit Client**: `pkg/signalprocessing/audit/client.go`
- **Severity Classifier**: `pkg/signalprocessing/classifier/severity.go`
- **DD-SEVERITY-001**: `docs/architecture/decisions/DD-SEVERITY-001-severity-determination-refactoring.md`

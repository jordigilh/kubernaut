# SignalProcessing PolicyHash Audit Field - Implementation Summary

**Date**: January 14, 2026
**Status**: ✅ **COMPLETE** - All Integration Tests Passing
**Test Results**: 91 Passed | 0 Failed | 1 Pending (expected)
**Business Requirement**: BR-SP-072 (Policy version traceability)
**Related Decisions**: DD-SEVERITY-001 (Severity Determination Refactoring)

---

## 📋 **Executive Summary**

Successfully implemented `policy_hash` field in SignalProcessing audit events to enable policy version traceability for compliance and audit requirements. The implementation required:
1. OpenAPI schema extension
2. CRD Status field addition
3. Controller Status update logic
4. Audit client payload inclusion
5. **Critical bug fix in FileWatcher hash generation**

---

## 🎯 **Business Context**

### **Problem Statement**
Operators need to correlate severity classification decisions with specific Rego policy versions for:
- **Audit Trail**: Which policy version made each decision?
- **Change Management**: Track policy evolution over time
- **Compliance**: Prove which policy was active at decision time

### **Solution**
Added `policy_hash` field (SHA256 hash) to `classification.decision` audit events, populated from the SeverityClassifier's Rego policy file hash.

---

## 🛠️ **Implementation Details**

### **1. OpenAPI Schema Extension**

**File**: `api/openapi/data-storage-v1.yaml`
**Change**: Added `policy_hash` to `SignalProcessingAuditPayload`

```yaml
SignalProcessingAuditPayload:
  properties:
    # ... existing fields ...
    policy_hash:
      type: string
      pattern: "^[a-f0-9]{64}$"
      description: SHA256 hash of Rego policy used for severity determination
      example: "a3b5c8d9e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8"
```

**Regeneration**: `make generate-datastorage-client` regenerated Ogen client with `PolicyHash OptString` field.

---

### **2. CRD Status Field**

**File**: `api/signalprocessing/v1alpha1/signalprocessing_types.go`
**Change**: Added `PolicyHash` to `SignalProcessingStatus`

```go
type SignalProcessingStatus struct {
    // ... existing fields ...
    // PolicyHash is the SHA256 hash of the Rego policy used for severity determination
    // Provides audit trail and policy version tracking for compliance requirements
    // +kubebuilder:validation:Pattern=^[a-f0-9]{64}$
    // +optional
    PolicyHash string `json:"policyHash,omitempty"`
}
```

**Regeneration**: `make manifests` regenerated CRD YAMLs with validation pattern.

---

### **3. Controller Status Update**

**File**: `internal/controller/signalprocessing/signalprocessing_controller.go`
**Change**: Set `sp.Status.PolicyHash` after severity classification

```go
func (r *SignalProcessingReconciler) reconcileClassifying(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing, logger logr.Logger) (ctrl.Result, error) {
    // ... existing classification logic ...

    if r.SeverityClassifier != nil {
        severityResult, err = r.SeverityClassifier.ClassifySeverity(ctx, sp)
        if err != nil { /* ... error handling ... */ }

        // Set PolicyHash in status for audit trail (BR-SP-072)
        sp.Status.PolicyHash = r.SeverityClassifier.GetPolicyHash()
    }

    // ... rest of reconciliation ...
}
```

---

### **4. Audit Client Payload**

**File**: `pkg/signalprocessing/audit/client.go`
**Change**: Include `PolicyHash` in `classification.decision` audit events

```go
func (c *AuditClient) RecordClassificationDecision(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing, durationMs int) {
    // ... existing payload fields ...

    // Add PolicyHash for traceability (BR-SP-072)
    if sp.Status.PolicyHash != "" {
        payload.PolicyHash.SetTo(sp.Status.PolicyHash)
    }

    // ... rest of audit emission ...
}
```

---

### **5. Critical Bug Fix: FileWatcher Hash Truncation**

#### **Root Cause**
**File**: `pkg/shared/hotreload/file_watcher.go`
**Issue**: `computeHash()` was returning only **first 8 bytes** (16 hex characters) instead of full SHA256 hash (64 hex characters)

**Original Code** (INCORRECT):
```go
func computeHash(content []byte) string {
    hash := sha256.Sum256(content)
    return hex.EncodeToString(hash[:8]) // First 8 bytes for brevity  ← PROBLEM
}
```

**Error Observed**:
```
⏳ Query error for signalprocessing.classification.decision:
decode response: validate: invalid: data (invalid: [0] (invalid: event_data
(invalid: policy_hash (string: no regex match: ^[a-f0-9]{64}$))))
```

#### **Fix**
**Changed to** (CORRECT):
```go
func computeHash(content []byte) string {
    hash := sha256.Sum256(content)
    return hex.EncodeToString(hash[:]) // Full SHA256 hash for audit compliance
}
```

**Impact**:
- ✅ Now returns **64-character** SHA256 hash matching OpenAPI schema regex
- ✅ Enables proper policy version traceability
- ✅ Fixes Ogen client validation errors

**Rationale**: The "brevity" optimization was inappropriate for audit trail requirements where full hash traceability is mandatory for compliance (SOC 2, ISO 27001).

---

## ✅ **Test Implementation**

**File**: `test/integration/signalprocessing/severity_integration_test.go`
**Test**: `"should include policy hash in audit event for policy version traceability"`

```go
It("should include policy hash in audit event for policy version traceability", func() {
    // GIVEN: SignalProcessing is created
    sp := createTestSignalProcessingCRD(namespace, "test-policy-hash")
    sp.Spec.Signal.Severity = "critical"
    Expect(k8sClient.Create(ctx, sp)).To(Succeed())

    correlationID := sp.Spec.RemediationRequestRef.Name

    // WHEN: Controller determines severity
    Eventually(func(g Gomega) {
        flushAuditStoreAndWait()

        count := countAuditEvents("signalprocessing.classification.decision", correlationID)
        g.Expect(count).To(BeNumerically(">", 0))

        event, err := getLatestAuditEvent("signalprocessing.classification.decision", correlationID)
        g.Expect(err).ToNot(HaveOccurred())

        payload := event.EventData.SignalProcessingAuditPayload

        // THEN: PolicyHash is set and valid
        g.Expect(payload.PolicyHash.IsSet()).To(BeTrue(),
            "PolicyHash should be set for audit trail")
        g.Expect(payload.PolicyHash.Value).To(MatchRegexp(`^[a-f0-9]{64}$`),
            "PolicyHash should be valid SHA256 hash")
    }, "30s", "2s").Should(Succeed())
})
```

**Status**: ✅ PASSING

---

## 🧪 **Test Results**

**Command**: `make test-integration-signalprocessing`

**Before Fix** (FileWatcher bug):
```
Ran 88 of 92 Specs in 128.590 seconds
FAIL! - Interrupted by Other Ginkgo Process -- 84 Passed | 4 Failed | 1 Pending
```

**After Fix** (Full SHA256 hash):
```
Ran 91 of 92 Specs in 119.331 seconds
SUCCESS! -- 91 Passed | 0 Failed | 1 Pending | 0 Skipped
```

**Pending Test**: `"should emit 'error.occurred' event when severity classification fails due to policy errors"` - This requires implementing the Failed Phase logic (separate work item).

---

## 📊 **Validation Checklist**

- [x] OpenAPI schema updated with `policy_hash` field
- [x] OpenAPI regex pattern: `^[a-f0-9]{64}$`
- [x] Ogen client regenerated successfully
- [x] CRD schema updated with `PolicyHash` status field
- [x] CRD validation pattern matches OpenAPI
- [x] CRD manifests regenerated
- [x] Controller sets `Status.PolicyHash` after classification
- [x] Audit client includes `PolicyHash` in events
- [x] FileWatcher returns full 64-character SHA256 hash
- [x] Integration test validates PolicyHash presence
- [x] Integration test validates SHA256 hash format
- [x] All SignalProcessing integration tests passing

---

## 🔗 **Documentation Alignment**

### **Authoritative Sources Consulted**

1. **DD-AUDIT-002**: Audit Shared Library Design
   - **Finding**: Services own their audit payload schemas
   - **Conclusion**: SignalProcessing team correctly owns `SignalProcessingAuditPayload`

2. **DD-AUDIT-004**: Structured Types for Audit Event Payloads
   - **Finding**: OpenAPI spec is the authoritative schema
   - **Conclusion**: Adding `policy_hash` to OpenAPI spec is the correct approach

3. **DD-SEVERITY-001**: Severity Determination Refactoring
   - **Finding**: Rego-based severity classification with hot-reload
   - **Conclusion**: PolicyHash field supports audit requirements for policy version tracking

### **Schema Governance**
- ✅ Each service owns their payload schema section in OpenAPI spec
- ✅ Services extend schemas as business requirements evolve
- ✅ OpenAPI spec generates Ogen client types used by all services
- ✅ Schema changes require client regeneration (`make generate-datastorage-client`)

---

## 🐛 **Bug Impact Analysis**

### **FileWatcher Hash Truncation Bug**

**Affected Components**:
1. `SeverityClassifier` - Severity policy hash
2. `EnvironmentClassifier` - Environment policy hash
3. `PriorityEngine` - Priority policy hash
4. `RegoEngine` - CustomLabels policy hash

**All of these components use `FileWatcher.GetLastHash()` which was returning truncated hashes.**

**Potential Issues Prevented**:
- ❌ **Audit trail integrity**: Truncated hashes could cause collisions
- ❌ **Compliance failures**: 16-char hashes insufficient for SOC 2/ISO 27001
- ❌ **Policy version tracking**: Unable to uniquely identify policy versions
- ❌ **Ogen client validation**: All services would fail to decode audit events with policy hashes

**Fix Scope**:
- ✅ **System-wide fix**: One change in `pkg/shared/hotreload/file_watcher.go` fixes all 4+ components
- ✅ **No breaking changes**: Existing functionality preserved, hash format corrected
- ✅ **Test coverage**: Integration tests validate full hash format

---

## 📝 **Related Work**

### **Completed**
- ✅ DataStorage connection pool fix (Jan 14, 2026)
- ✅ Service-wide config flag usage audit (Jan 14, 2026)
- ✅ SignalProcessing test correlation ID fixes (Jan 14, 2026)
- ✅ Structured type migration for audit events (Jan 14, 2026)
- ✅ Duplicate `classification.decision` audit event fix (Jan 14, 2026)
- ✅ Duplicate `phase.transition` audit event fix (Jan 14, 2026)
- ✅ **PolicyHash audit field implementation** (Jan 14, 2026)

### **Pending**
- ⏳ Failed Phase logic implementation (when policy errors occur)
- ⏳ Failed Phase integration test enablement

---

## 🎯 **Business Outcome**

**Operators can now**:
1. ✅ **Audit Trail**: See which policy version made each severity decision
2. ✅ **Change Management**: Track policy evolution over time
3. ✅ **Compliance**: Prove which policy was active at decision time
4. ✅ **Debugging**: Correlate decisions with specific policy file contents
5. ✅ **Rollback Support**: Identify when policy changes caused issues

**Example Audit Event**:
```json
{
  "event_type": "signalprocessing.classification.decision",
  "event_data": {
    "severity": "critical",
    "determination_source": "rego-policy",
    "policy_hash": "a3b5c8d9e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8"
  }
}
```

---

## 📚 **Key Learnings**

1. **Schema Ownership**: Each service owns their section of the OpenAPI spec
2. **Full Hash Required**: Audit trail requires full SHA256 hashes, not truncated versions
3. **Shared Code Impact**: Changes to shared components (FileWatcher) have system-wide effects
4. **Ogen Validation**: OpenAPI `pattern` constraints are enforced by Ogen client at decode time
5. **Test-Driven Discovery**: Integration tests caught the hash truncation bug immediately

---

## 📊 **Confidence Assessment**

**Overall Confidence**: 95%

**Justification**:
- ✅ All integration tests passing (91/91 non-pending)
- ✅ PolicyHash field present and valid in audit events
- ✅ Schema correctly defined in OpenAPI and CRD
- ✅ Ogen client regenerated successfully
- ✅ FileWatcher bug fixed at system level
- ⚠️ 5% uncertainty for edge cases in production (e.g., policy file corruption)

---

## 🔄 **Next Steps**

### **Immediate (Completed)**
- [x] Enable PolicyHash integration test
- [x] Fix FileWatcher hash truncation bug
- [x] Validate all SignalProcessing integration tests pass

### **Short-Term (Future Work)**
- [ ] Implement Failed Phase logic in `reconcileClassifying()`
- [ ] Develop reliable policy error injection mechanism for tests
- [ ] Enable "Failed Phase on Policy Error" integration test

### **Long-Term (Future Work)**
- [ ] Add PolicyHash to other classifiers' audit events (Environment, Priority)
- [ ] Implement policy version history API for compliance queries
- [ ] Add PolicyHash validation in must-gather diagnostic tools

---

**Maintained By**: Kubernaut SignalProcessing Team
**Last Updated**: January 14, 2026
**Review Cycle**: With next audit compliance review

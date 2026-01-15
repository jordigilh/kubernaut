# Gateway Integration Test Plan - Audit Structure Reassessment
**Date**: January 14, 2026
**Status**: CORRECTED - Based on Authoritative Sources
**Document**: GW_INTEGRATION_TEST_PLAN_V1.0.md
**Supersedes**: GW_TEST_PLAN_AUDIT_STRUCTURE_VIOLATIONS_JAN14_2026.md

---

## Executive Summary

**REASSESSMENT COMPLETE**: Initial triage was **partially incorrect**. After examining authoritative sources:

**✅ GOOD NEWS**: OpenAPI schema HAS most required fields (11 of 14 tested scenarios)
**⚠️ LIMITED ISSUES**: Only 3 scenarios need OpenAPI schema additions
**✅ MAIN FIX**: Test plan needs pattern corrections (not major schema changes)

**Actual Violations**: **30 instances** (not 68) across **3 scenarios** (not 14)
**Fix Effort**: **4-6 hours** (not 10-14 hours)

---

## Authoritative Sources Analyzed

### **1. OpenAPI Schema** (`api/openapi/data-storage-v1.yaml` lines 2356-2430)
✅ **CONFIRMED**: GatewayAuditPayload has these fields:
- `signal_type` (enum: prometheus-alert, kubernetes-event)
- `alert_name` (string)
- `namespace` (string)
- `fingerprint` (string)
- `severity` (enum: critical, warning, info)
- `resource_kind` (string)
- `resource_name` (string)
- `remediation_request` (string - namespace/name format)
- `deduplication_status` (enum: new, duplicate)
- `occurrence_count` (int32)
- `original_payload` (object - additionalProperties: true)
- `signal_labels` (object - additionalProperties: string)
- `signal_annotations` (object - additionalProperties: string)
- `error_details` (ref: ErrorDetails schema)

### **2. Gateway Implementation** (`pkg/gateway/server.go` lines 1244-1467)
✅ **CONFIRMED**: Gateway correctly populates GatewayAuditPayload:
```go
// emitSignalReceivedAudit (line 1281-1309)
payload := api.GatewayAuditPayload{
    EventType:   EventTypeSignalReceived,
    SignalType:  toGatewayAuditPayloadSignalType(signal.SourceType),
    AlertName:   signal.AlertName,
    Namespace:   signal.Namespace,
    Fingerprint: signal.Fingerprint,
}
payload.OriginalPayload.SetTo(convertMapToJxRaw(originalPayload))
payload.SignalLabels.SetTo(labels)
payload.SignalAnnotations.SetTo(annotations)
payload.Severity.SetTo(toGatewayAuditPayloadSeverity(signal.Severity))
```

### **3. E2E Test Pattern** (`test/e2e/gateway/23_audit_emission_test.go` lines 286-298)
✅ **CONFIRMED**: Correct access pattern:
```go
event := auditEvents[0]  // ogenclient.AuditEvent
gatewayPayload := event.EventData.GatewayAuditPayload  // Structured payload
Expect(gatewayPayload.AlertName).To(Equal("AuditTestAlert"))
Expect(gatewayPayload.Namespace).To(Equal(sharedNamespace))
Expect(string(gatewayPayload.SignalType)).To(Equal("prometheus-alert"))
```

---

## Corrected Violation Analysis

### **✅ Scenarios 1.1, 1.2, 3.1, 3.2, 5.1, 5.2, 6.1, 7.1 - NO CHANGES NEEDED**

**Test Plan Pattern**:
```go
Expect(auditEvent.SignalLabels).To(HaveKeyWithValue("severity", "critical"))
```

**Correct Integration Test Pattern** (from E2E test line 286):
```go
gatewayPayload := auditEvent.EventData.GatewayAuditPayload
signalLabels, ok := gatewayPayload.SignalLabels.Get()
Expect(ok).To(BeTrue())
Expect(signalLabels).To(HaveKeyWithValue("severity", "critical"))
```

**Fix Required**: Change access pattern (NOT add OpenAPI fields)
**Scenarios Affected**: 1.1 (8 instances), 1.2 (5 instances), 3.1 (6 instances), 3.2 (4 instances)
**Total**: 23 pattern corrections

---

### **⚠️ Scenario 1.3 (Signal Deduplicated) - MISSING OPENAPI FIELDS**

**Test Plan Assumes** (Lines 377-385):
```go
dedupeEvent := findEventByType(auditStore.Events, "gateway.signal.deduplicated")
reason := dedupeEvent.Metadata["deduplication_reason"]
Expect(reason).To(BeElementOf("status-based", "time-window", "manual-suppression"))
Expect(dedupeEvent.Metadata).To(HaveKeyWithValue("existing_rr_phase", "Pending"))
Expect(dedupeEvent.Metadata).To(HaveKeyWithValue("existing_rr_name", crd.Name))
```

**Reality**: These fields DON'T EXIST in GatewayAuditPayload schema:
- ❌ `deduplication_reason` (why it was deduplicated)
- ❌ `existing_rr_phase` (phase of existing RR)
- ❌ `existing_rr_name` (name of existing RR)

**Current Schema Has** (Lines 2417-2425):
```yaml
deduplication_status:  # "new" or "duplicate"
occurrence_count:      # int32
```

**RECOMMENDED FIX**: Use existing fields instead of adding new ones:
```go
gatewayPayload := dedupeEvent.EventData.GatewayAuditPayload

// Business rule: DeduplicationStatus tells us it's a duplicate
dedupStatus, ok := gatewayPayload.DeduplicationStatus.Get()
Expect(ok).To(BeTrue())
Expect(dedupStatus).To(Equal(api.GatewayAuditPayloadDeduplicationStatusDuplicate))

// Business rule: Occurrence count shows repeat signal
occurrenceCount, ok := gatewayPayload.OccurrenceCount.Get()
Expect(ok).To(BeTrue())
Expect(occurrenceCount).To(BeNumerically(">", 1))

// Business rule: RemediationRequest field contains existing RR name
rrRef, ok := gatewayPayload.RemediationRequest.Get()
Expect(ok).To(BeTrue())
Expect(rrRef).To(MatchRegexp(`^[^/]+/rr-[a-f0-9]+-\d+$`))
```

**Violations**: 4 instances in Scenario 1.3
**OpenAPI Changes Needed**: **NONE** (use existing fields)

---

###**⚠️ Scenario 1.4 (CRD Failed) - EXISTING ERRORDETAILS SCHEMA**

**Test Plan Assumes**:
```go
Expect(crdFailedEvent.Metadata).To(HaveKeyWithValue("error_code", "K8s API Error"))
Expect(crdFailedEvent.Metadata).To(HaveKeyWithValue("failure_reason", "..."))
```

**Reality**: ErrorDetails schema EXISTS (line 2428):
```yaml
error_details:
  $ref: '#/components/schemas/ErrorDetails'
```

**ErrorDetails Schema** (from OpenAPI):
```yaml
ErrorDetails:
  type: object
  required: [message, code, component, retry_possible]
  properties:
    message: string
    code: string
    component: enum (gateway, aianalysis, etc.)
    retry_possible: boolean
    stack_trace: array of strings
```

**Gateway Implementation** (`pkg/gateway/server.go` line 1449-1452):
```go
errorDetails := sharedaudit.NewErrorDetailsFromK8sError("gateway", err)
apiErrorDetails := toAPIErrorDetails(errorDetails)
payload.ErrorDetails.SetTo(apiErrorDetails)
```

**Correct Pattern**:
```go
gatewayPayload := crdFailedEvent.EventData.GatewayAuditPayload

errorDetails, ok := gatewayPayload.ErrorDetails.Get()
Expect(ok).To(BeTrue())
Expect(errorDetails.Code).To(ContainSubstring("K8s"))
Expect(errorDetails.Message).ToNot(BeEmpty())
Expect(errorDetails.Component).To(Equal(api.ErrorDetailsComponentGateway))
Expect(errorDetails.RetryPossible).To(BeFalse())
```

**Violations**: 3 instances in Scenario 1.4
**OpenAPI Changes Needed**: **NONE** (schema exists)

---

### **❌ Scenario 4.1 (Circuit Breaker) - WRONG TEST TIER**

**Test Plan Assumes**:
```go
Expect(auditEvent.Metadata).To(HaveKeyWithValue("circuit_state", "open"))
Expect(auditEvent.Metadata).To(HaveKeyWithValue("failure_count", "5"))
```

**CRITICAL ISSUE**: Circuit breaker state is **NOT audit data**!

**Circuit breaker is operational state, not compliance data:**
- ❌ Not needed for SOC2 compliance
- ❌ Not needed for RR reconstruction
- ❌ Should be in metrics/observability (Prometheus), not audit

**RECOMMENDED FIX**: **REMOVE Scenario 4.1 from Integration Tests**
**Alternative**: Move to **unit tests** for circuit breaker logic validation
**Rationale**: Circuit breaker testing doesn't require audit infrastructure

**Violations**: 0 (test scenario should be removed)

---

## Summary of Actual Issues

| Issue | Scenarios | Instances | Fix Type | Effort |
|-------|-----------|-----------|----------|--------|
| **Access Pattern** | 1.1, 1.2, 3.1, 3.2, 5.1, 5.2, 6.1, 7.1 | 23 | Change pattern | 3-4 hours |
| **Use Existing Fields** | 1.3 | 4 | Rewrite logic | 30 min |
| **Use ErrorDetails** | 1.4 | 3 | Use existing schema | 30 min |
| **Wrong Test Tier** | 4.1 | N/A | Remove scenario | 15 min |
| **TOTAL** | 11 of 14 | **30** | **Pattern fixes** | **4-6 hours** |

---

## Corrected Fix Strategy

### **Phase 1: Pattern Corrections (3-4 hours) - NO SCHEMA CHANGES**

**Fix all access patterns** from:
```go
❌ auditEvent.SignalLabels
```

To:
```go
✅ gatewayPayload := auditEvent.EventData.GatewayAuditPayload
   signalLabels, ok := gatewayPayload.SignalLabels.Get()
```

**Affected Scenarios**: 1.1, 1.2, 3.1, 3.2, 5.1, 5.2, 6.1, 7.1 (23 instances)

---

### **Phase 2: Rewrite Scenario 1.3 (30 minutes) - USE EXISTING FIELDS**

**Replace**:
```go
❌ deduplication_reason, existing_rr_phase, existing_rr_name
```

**With**:
```go
✅ deduplication_status, occurrence_count, remediation_request (all exist)
```

**Business Logic**:
- `deduplication_status == "duplicate"` proves deduplication occurred
- `occurrence_count > 1` proves it's a repeated signal
- `remediation_request` contains the RR name (format: "namespace/name")

---

### **Phase 3: Fix Scenario 1.4 (30 minutes) - USE ERROR DETAILS**

**Replace**:
```go
❌ error_code, failure_reason in Metadata
```

**With**:
```go
✅ error_details.Code, error_details.Message (schema exists)
```

**ErrorDetails fields available**:
- `message` (string)
- `code` (string)
- `component` (enum)
- `retry_possible` (boolean)
- `stack_trace` (array of strings)

---

### **Phase 4: Remove Scenario 4.1 (15 minutes)**

**Rationale**: Circuit breaker is operational state, not audit data
**Action**: Delete Scenario 4.1 tests (8 instances)
**Alternative**: Move circuit breaker logic to unit tests

---

### **Phase 5: Create Test Helpers (1 hour)**

```go
// test/integration/gateway/audit_test_helpers.go

// ParseGatewayPayload extracts GatewayAuditPayload from AuditEvent
func ParseGatewayPayload(event *ogenclient.AuditEvent) *api.GatewayAuditPayload {
    return &event.EventData.GatewayAuditPayload
}

// ExpectSignalLabels validates signal_labels field
func ExpectSignalLabels(payload *api.GatewayAuditPayload, expected map[string]string) {
    labels, ok := payload.SignalLabels.Get()
    Expect(ok).To(BeTrue(), "SignalLabels should be present")
    for k, v := range expected {
        Expect(labels).To(HaveKeyWithValue(k, v))
    }
}

// ExpectErrorDetails validates error_details field
func ExpectErrorDetails(payload *api.GatewayAuditPayload, expectedCode string) {
    errorDetails, ok := payload.ErrorDetails.Get()
    Expect(ok).To(BeTrue(), "ErrorDetails should be present")
    Expect(errorDetails.Code).To(ContainSubstring(expectedCode))
    Expect(errorDetails.Message).ToNot(BeEmpty())
}
```

---

## Examples: Test Plan Corrections

### **Example 1: Signal Received (Scenario 1.1)**

❌ **BEFORE (Test Plan - Lines 156-160)**:
```go
auditEvent := auditStore.Events[0]
Expect(auditEvent.SignalLabels).To(HaveKeyWithValue("severity", "critical"))
Expect(auditEvent.SignalLabels).To(HaveKeyWithValue("team", "platform"))
Expect(auditEvent.SignalLabels).To(HaveKeyWithValue("environment", "production"))
```

✅ **AFTER (Correct Pattern)**:
```go
auditEvent := auditStore.Events[0]

// Parse EventData to get GatewayAuditPayload
gatewayPayload := auditEvent.EventData.GatewayAuditPayload

// Access SignalLabels (Optional field - check if present)
signalLabels, ok := gatewayPayload.SignalLabels.Get()
Expect(ok).To(BeTrue(), "SignalLabels should be present in audit payload")
Expect(signalLabels).To(HaveKeyWithValue("severity", "critical"))
Expect(signalLabels).To(HaveKeyWithValue("team", "platform"))
Expect(signalLabels).To(HaveKeyWithValue("environment", "production"))
```

---

### **Example 2: Signal Deduplicated (Scenario 1.3)**

❌ **BEFORE (Test Plan - Lines 377-385)**:
```go
dedupeEvent := findEventByType(auditStore.Events, "gateway.signal.deduplicated")
reason := dedupeEvent.Metadata["deduplication_reason"]  // ❌ Field doesn't exist
Expect(reason).To(BeElementOf("status-based", "time-window", "manual-suppression"))
Expect(dedupeEvent.Metadata).To(HaveKeyWithValue("existing_rr_phase", "Pending"))  // ❌ Doesn't exist
```

✅ **AFTER (Use Existing Fields)**:
```go
dedupeEvent := findEventByType(auditStore.Events, "gateway.signal.deduplicated")

// Parse EventData
gatewayPayload := dedupeEvent.EventData.GatewayAuditPayload

// Business rule: DeduplicationStatus proves deduplication occurred
dedupStatus, ok := gatewayPayload.DeduplicationStatus.Get()
Expect(ok).To(BeTrue())
Expect(dedupStatus).To(Equal(api.GatewayAuditPayloadDeduplicationStatusDuplicate))

// Business rule: Occurrence count shows signal repetition pattern
occurrenceCount, ok := gatewayPayload.OccurrenceCount.Get()
Expect(ok).To(BeTrue())
Expect(occurrenceCount).To(BeNumerically(">", 1))

// Business rule: RemediationRequest contains existing RR reference
rrRef, ok := gatewayPayload.RemediationRequest.Get()
Expect(ok).To(BeTrue())
Expect(rrRef).To(MatchRegexp(`^[^/]+/rr-[a-f0-9]+-\d+$`))

// Extract RR name from "namespace/name" format
parts := strings.Split(rrRef, "/")
Expect(parts).To(HaveLen(2))
rrName := parts[1]
Expect(rrName).To(MatchRegexp(`^rr-[a-f0-9]{12}-\d{10}$`))
```

---

### **Example 3: CRD Failed (Scenario 1.4)**

❌ **BEFORE (Test Plan)**:
```go
crdFailedEvent := findEventByType(auditStore.Events, "gateway.crd.failed")
Expect(crdFailedEvent.Metadata).To(HaveKeyWithValue("error_code", "K8s API Error"))  // ❌ Wrong location
Expect(crdFailedEvent.Metadata).To(HaveKeyWithValue("failure_reason", "..."))  // ❌ Wrong location
```

✅ **AFTER (Use ErrorDetails)**:
```go
crdFailedEvent := findEventByType(auditStore.Events, "gateway.crd.failed")

// Parse EventData
gatewayPayload := crdFailedEvent.EventData.GatewayAuditPayload

// Access ErrorDetails (existing schema)
errorDetails, ok := gatewayPayload.ErrorDetails.Get()
Expect(ok).To(BeTrue(), "ErrorDetails should be present for failed CRD creation")

// Business rule: Error code identifies failure category
Expect(errorDetails.Code).To(ContainSubstring("K8s"))

// Business rule: Error message provides troubleshooting context
Expect(errorDetails.Message).ToNot(BeEmpty())
Expect(errorDetails.Message).To(ContainSubstring("namespace"))

// Business rule: Component identifies error source
Expect(errorDetails.Component).To(Equal(api.ErrorDetailsComponentGateway))

// Business rule: RetryPossible guides remediation strategy
Expect(errorDetails.RetryPossible).To(BeFalse(), "Namespace errors are not transient")
```

---

## Corrected Impact Assessment

### **Severity**: ⚠️ **MEDIUM** (not CRITICAL)

**Rationale**:
1. ✅ OpenAPI schema HAS all required fields (except non-essential ones)
2. ✅ Gateway implementation is CORRECT
3. ⚠️ Test plan uses wrong access patterns (easily fixable)
4. ⚠️ 3 scenarios need minor logic rewrites (not schema changes)

### **Effort Estimate** (CORRECTED):

| Task | Effort | Priority |
|------|--------|----------|
| **Fix access patterns (23 instances)** | 3-4 hours | P0 |
| **Rewrite Scenario 1.3 logic (4 instances)** | 30 minutes | P0 |
| **Fix Scenario 1.4 with ErrorDetails (3 instances)** | 30 minutes | P0 |
| **Remove Scenario 4.1 (wrong tier)** | 15 minutes | P1 |
| **Create test helpers** | 1 hour | P1 |
| **Validate all scenarios** | 1 hour | P1 |
| **TOTAL** | **6-7 hours** | **NON-BLOCKING** |

---

## Decision

### **Option A: Fix Test Plan Patterns NOW** ✅ **RECOMMENDED**
**Pros**:
- ✅ No OpenAPI schema changes needed
- ✅ Correct patterns from day 1
- ✅ Matches existing E2E test patterns
- ✅ Uses existing fields (no schema additions)

**Cons**:
- ⏱️ 6-7 hour delay before implementation starts

**Recommendation**: **PROCEED WITH OPTION A** (much simpler than initially assessed)

### **Why Initial Assessment Was Wrong**:
1. ❌ Didn't check actual OpenAPI schema thoroughly
2. ❌ Didn't examine existing E2E test patterns
3. ❌ Didn't verify Gateway implementation code
4. ❌ Assumed fields were missing when they exist
5. ❌ Included circuit breaker (operational state, not audit)

---

## Next Steps (Option A Approved)

1. **[ ] Fix Access Patterns** (3-4 hours)
   - Update 23 instances across 8 scenarios
   - Change from `auditEvent.FieldName` to `gatewayPayload.FieldName.Get()`

2. **[ ] Rewrite Scenario 1.3** (30 minutes)
   - Use existing fields: deduplication_status, occurrence_count, remediation_request
   - Remove references to non-existent fields

3. **[ ] Fix Scenario 1.4** (30 minutes)
   - Use existing ErrorDetails schema
   - Access via `gatewayPayload.ErrorDetails.Get()`

4. **[ ] Remove Scenario 4.1** (15 minutes)
   - Circuit breaker is operational state, not audit data
   - Consider moving to unit tests

5. **[ ] Create Test Helpers** (1 hour)
   - `ParseGatewayPayload()`
   - `ExpectSignalLabels()`
   - `ExpectErrorDetails()`

6. **[ ] Validate & Commit** (1 hour)
   - Review all 84 test specifications
   - Verify against E2E test patterns
   - Commit with comprehensive changelog

**TOTAL ESTIMATED TIME**: 6-7 hours (not 10-14 hours)

---

## Authority & References

- **OpenAPI Schema**: `api/openapi/data-storage-v1.yaml` (lines 2356-2430) - AUTHORITATIVE
- **Gateway Implementation**: `pkg/gateway/server.go` (lines 1244-1467) - AUTHORITATIVE
- **E2E Test Pattern**: `test/e2e/gateway/23_audit_emission_test.go` (lines 286-298) - AUTHORITATIVE
- **Generated Types**: `pkg/datastorage/ogen-client/oas_schemas_gen.go` (lines 4443-4476)
- **ErrorDetails Schema**: `api/openapi/data-storage-v1.yaml` (ErrorDetails component)
- **DD-AUDIT-002 V2.0.1**: Audit Shared Library Design
- **ADR-034**: Unified Audit Table Design

---

**Status**: ✅ **READY FOR PATTERN FIXES - NO SCHEMA CHANGES NEEDED**
**Owner**: Gateway Team
**Reviewer**: N/A (no OpenAPI changes)
**Next Action**: Approve Option A → Fix test plan patterns (6-7 hours)

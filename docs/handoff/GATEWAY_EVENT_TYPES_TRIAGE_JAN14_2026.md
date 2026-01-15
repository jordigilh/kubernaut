# Gateway Event Types Triage - RR Reconstruction Requirements

**Date**: January 14, 2026
**Purpose**: Triage 3 gateway event types against RR reconstruction BR-AUDIT-005
**Status**: ✅ **TRIAGE COMPLETE** - Confirmed test logic errors

---

## 🎯 **Business Requirement Analysis**

### **BR-AUDIT-005 v2.0: RR CRD Reconstruction**

> "**RR CRD Reconstruction**: MUST support RemediationRequest CRD reconstruction from audit traces via REST API (100% field coverage including optional TimeoutConfig)"
>
> **Source**: [docs/requirements/11_SECURITY_ACCESS_CONTROL.md:140](../requirements/11_SECURITY_ACCESS_CONTROL.md)

### **Authoritative Reconstruction Query**

**File**: `pkg/datastorage/reconstruction/query.go:49-54`

```go
AND event_type IN (
    'gateway.signal.received',              // ✅ ONLY gateway event needed
    'aianalysis.analysis.completed',        // ✅ Required
    'workflowexecution.selection.completed',// ✅ Required
    'workflowexecution.execution.started',  // ✅ Required
    'orchestrator.lifecycle.created'        // ✅ Required
)
```

**Conclusion**: RR reconstruction requires **EXACTLY 1 gateway event type**: `gateway.signal.received`

---

## 📊 **Event Type Triage Results**

### **Event 1: `gateway.storm.detected`**

| Attribute | Finding |
|---|---|
| **Defined in OpenAPI Schema?** | ❌ NO |
| **Required for RR Reconstruction?** | ❌ NO (not in reconstruction query) |
| **Business Requirement?** | ❌ NO (explicitly removed per DD-GATEWAY-015) |
| **Historical Context** | ✅ Intentionally removed - storm detection provided NO business value |
| **Verdict** | ⚠️ **TEST LOGIC ERROR** - Should be removed from test file |

**Evidence from DD-GATEWAY-015**:
```
Problem: Storm detection provides NO business value
- Redundant with deduplication (occurrenceCount already tracked)
- No downstream consumers (AI Analysis doesn't use storm flag)
- No workflow routing differences for storms
- Audit events: gateway.storm.detected REMOVED ✅
```

**Source**: [DD-GATEWAY-015: Storm Detection Logic Removal](../architecture/decisions/DD-GATEWAY-015-storm-detection-removal.md:36)

---

### **Event 2: `gateway.signal.rejected`**

| Attribute | Finding |
|---|---|
| **Defined in OpenAPI Schema?** | ❌ NO |
| **Required for RR Reconstruction?** | ❌ NO (not in reconstruction query) |
| **Business Requirement?** | ❌ NO (no BR reference found) |
| **Use Case** | 🤔 Could track invalid/rejected signals |
| **Verdict** | ⚠️ **TEST LOGIC ERROR** - No BR backing, not in schema |

**Analysis**:
- **Potential Use Case**: Track signals rejected due to validation errors
- **Reality**: No business requirement exists for this
- **RR Reconstruction**: Only **SUCCESSFUL** signal processing (`gateway.signal.received`) matters for reconstruction
- **Rejected signals**: Do NOT create RemediationRequest CRDs → NOT part of audit trail for RR reconstruction

---

### **Event 3: `gateway.error.occurred`**

| Attribute | Finding |
|---|---|
| **Defined in OpenAPI Schema?** | ❌ NO |
| **Required for RR Reconstruction?** | ❌ NO (not in reconstruction query) |
| **Business Requirement?** | ❌ NO (generic error event, not specific to RR lifecycle) |
| **Use Case** | 🤔 Could track general gateway errors |
| **Verdict** | ⚠️ **TEST LOGIC ERROR** - No BR backing, not in schema |

**Analysis**:
- **Potential Use Case**: Track general gateway operational errors
- **Reality**: No business requirement exists for this
- **RR Reconstruction**: General gateway errors (e.g., database connection failures) are NOT part of RR lifecycle
- **Gap #7 Coverage**: Service-specific `*.failure` events (e.g., `gateway.crd.failed`) already cover RR-related failures with standardized `error_details`

---

## 📝 **Valid Gateway Event Types (Per OpenAPI Schema)**

**File**: `api/openapi/data-storage-v1.yaml:2363`

```yaml
enum: [
  'gateway.signal.received',       # ✅ VALID - Required for RR reconstruction
  'gateway.signal.deduplicated',   # ✅ VALID - Tracks duplicate signals (not for reconstruction)
  'gateway.crd.created',           # ✅ VALID - Success audit (not for reconstruction)
  'gateway.crd.failed'             # ✅ VALID - Failure audit with error_details (Gap #7)
]
```

### **RR Reconstruction Usage**

| Event Type | Used in Reconstruction? | Purpose |
|---|---|---|
| `gateway.signal.received` | ✅ **YES** | Captures original payload for Gap #1-3 |
| `gateway.signal.deduplicated` | ❌ NO | Observability only (tracks duplicates) |
| `gateway.crd.created` | ❌ NO | Success audit only (RR already created) |
| `gateway.crd.failed` | ❌ NO | Failure audit with error_details (Gap #7) |

**Key Insight**: Only 1 out of 4 valid gateway events is used for RR reconstruction.

---

## 🚨 **Recommendation: Remove 3 Invalid Event Types**

### **Rationale**

1. **Not in OpenAPI Schema**: These events are undefined in the authoritative API contract
2. **Not Required for RR Reconstruction**: BR-AUDIT-005 satisfied with existing events
3. **No Business Requirement**: No BR-GATEWAY-XXX or BR-AUDIT-XXX backing
4. **Test Logic Errors**: Tests should only validate defined API behavior
5. **Historical Context**: `gateway.storm.detected` explicitly removed per DD-GATEWAY-015

### **Implementation**

Remove from `test/e2e/datastorage/09_event_type_jsonb_comprehensive_test.go`:
1. Lines 134-153: `gateway.storm.detected`
2. Lines 166-181: `gateway.signal.rejected`
3. Lines 181-196: `gateway.error.occurred`

**Expected Impact**:
- **Before**: 107/111 passing (96.4%) - 1 failure + 3 skipped
- **After**: 107/108 passing (99.1%) - 0 gateway failures

---

## 🎯 **Future Consideration: Should These Events Be Added?**

### **Option A: Keep Removed (RECOMMENDED)**

**Rationale**:
- ✅ No business requirement exists
- ✅ Not needed for RR reconstruction (BR-AUDIT-005 satisfied)
- ✅ Simpler schema (fewer event types to maintain)
- ✅ Existing events cover all RR lifecycle scenarios

**Verdict**: **Do NOT add** these event types unless a new business requirement emerges.

---

### **Option B: Add to Schema (NOT RECOMMENDED)**

**IF** a future business requirement emerges:

1. **`gateway.storm.detected`**:
   - **Use Case**: Track storm patterns for alerting/dashboards
   - **Alternative**: Query `gateway.signal.received` with `occurrence_count >= 5`
   - **Recommendation**: ❌ Keep removed per DD-GATEWAY-015 decision

2. **`gateway.signal.rejected`**:
   - **Use Case**: Track invalid signal formats for debugging
   - **Alternative**: Use application logs + metrics (`gateway_signals_rejected_total`)
   - **Recommendation**: ⚠️ Only add if BR created for signal validation observability

3. **`gateway.error.occurred`**:
   - **Use Case**: Track general gateway operational errors
   - **Alternative**: Use application logs + metrics + `gateway.crd.failed` for RR-related errors
   - **Recommendation**: ❌ Too generic - specific `*.failure` events better

---

## 📚 **Evidence Sources**

### **Business Requirements**
1. ✅ [BR-AUDIT-005 v2.0](../requirements/11_SECURITY_ACCESS_CONTROL.md:140) - RR reconstruction requirement
2. ✅ [SOC2 Test Plan](../development/SOC2/SOC2_AUDIT_RR_RECONSTRUCTION_TEST_PLAN.md:193) - Gap #1-3 uses `gateway.signal.received`

### **Implementation**
1. ✅ [query.go:49-54](../../pkg/datastorage/reconstruction/query.go) - Authoritative reconstruction query
2. ✅ [OpenAPI Schema](../../api/openapi/data-storage-v1.yaml:2363) - Defines 4 valid gateway events
3. ✅ [DD-GATEWAY-015](../architecture/decisions/DD-GATEWAY-015-storm-detection-removal.md) - Storm detection removal decision

### **Test Files**
1. ⚠️ [09_event_type_jsonb_comprehensive_test.go](../../test/e2e/datastorage/09_event_type_jsonb_comprehensive_test.go) - Contains 3 invalid event types

---

## ✅ **Triage Summary**

| Event Type | Valid? | RR Reconstruction? | Action |
|---|---|---|---|
| `gateway.signal.received` | ✅ YES | ✅ YES | Keep (required) |
| `gateway.signal.deduplicated` | ✅ YES | ❌ NO | Keep (observability) |
| `gateway.crd.created` | ✅ YES | ❌ NO | Keep (success audit) |
| `gateway.crd.failed` | ✅ YES | ❌ NO | Keep (Gap #7) |
| **`gateway.storm.detected`** | ❌ **NO** | ❌ NO | **Remove from tests** |
| **`gateway.signal.rejected`** | ❌ **NO** | ❌ NO | **Remove from tests** |
| **`gateway.error.occurred`** | ❌ **NO** | ❌ NO | **Remove from tests** |

**Verdict**: **Remove all 3 invalid event types** - they are test logic errors with no business requirement backing.

**Confidence**: 100% (authoritative sources confirm)

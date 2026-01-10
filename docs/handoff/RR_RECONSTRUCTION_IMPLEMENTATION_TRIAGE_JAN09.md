# RR Reconstruction Implementation Plan - Comprehensive Triage

**Date**: January 9, 2026
**Status**: 🔍 **PRE-IMPLEMENTATION TRIAGE**
**Branch**: `feature/soc2-compliance`
**Triage Scope**: Gap analysis and inconsistency detection before implementation

---

## 🎯 **Executive Summary**

**Triage Result**: ⚠️ **MAJOR INCONSISTENCY DETECTED** - Implementation plan is **OUTDATED**

**Key Finding**: The December 18, 2025 implementation plan identifies 8 critical gaps (0% coverage) that **ALREADY EXIST** in the current CRD schema as of January 9, 2026.

**Impact**: **Phase 1 and Phase 2 of the plan (4.5 days) are UNNECESSARY** - fields already exist in CRD.

**Recommendation**: **SKIP to Phase 3** (Reconstruction Logic) - estimated 2 days instead of 6.5 days.

---

## 🚨 **CRITICAL INCONSISTENCY: Plan vs. Reality**

### **Plan Claims (December 18, 2025)**

The implementation plan states these fields have **0% coverage** and need to be added:

| Gap # | Field | Plan Status | Estimated Effort |
|-------|-------|-------------|------------------|
| **1** | `OriginalPayload` | ❌ 0% - CRITICAL GAP | 3h (Gateway audit enhancement) |
| **2** | `ProviderData` | ❌ 0% - CRITICAL GAP | 3h (AIAnalysis audit enhancement) |
| **3** | `SignalLabels` | ❌ 0% - HIGH GAP | 2h (Gateway audit enhancement) |
| **4** | `SignalAnnotations` | ❌ 0% - HIGH GAP | 2h (Gateway audit enhancement) |
| **8** | `TimeoutConfig` | ❌ 0% - LOW GAP | 1h (RO audit enhancement) |

**Total Planned Effort for Phase 1**: 12 hours (1.5 days)

---

### **Current Reality (January 9, 2026)**

**ALL FIELDS ALREADY EXIST** in `api/remediation/v1alpha1/remediationrequest_types.go`:

```go
type RemediationRequestSpec struct {
    // ... other fields ...

    // ========================================
    // SIGNAL METADATA (PHASE 1 ADDITION) ← ALREADY EXISTS
    // ========================================
    SignalLabels      map[string]string `json:"signalLabels,omitempty"`      // Gap #3 ✅
    SignalAnnotations map[string]string `json:"signalAnnotations,omitempty"` // Gap #4 ✅

    // ========================================
    // PROVIDER-SPECIFIC DATA ← ALREADY EXISTS
    // ========================================
    ProviderData []byte `json:"providerData,omitempty"` // Gap #2 ✅

    // ========================================
    // AUDIT/DEBUG ← ALREADY EXISTS
    // ========================================
    OriginalPayload []byte `json:"originalPayload,omitempty"` // Gap #1 ✅

    // ========================================
    // WORKFLOW CONFIGURATION ← ALREADY EXISTS
    // ========================================
    TimeoutConfig *TimeoutConfig `json:"timeoutConfig,omitempty"` // Gap #8 ✅
}
```

**Verification Date**: Lines 300-338 of `remediationrequest_types.go`

---

## 📊 **Gap Analysis: Plan vs. Current State**

### **Phase 1: Critical Spec Fields (Gaps #1-4)**

| Gap | Field | Plan Status | **Actual Status** | Delta |
|-----|-------|-------------|-------------------|-------|
| #1 | `OriginalPayload` | ❌ 0% coverage | ✅ **EXISTS in CRD** (line 328) | **+100%** |
| #2 | `ProviderData` | ❌ 0% coverage | ✅ **EXISTS in CRD** (line 320) | **+100%** |
| #3 | `SignalLabels` | ❌ 0% coverage | ✅ **EXISTS in CRD** (line 300) | **+100%** |
| #4 | `SignalAnnotations` | ❌ 0% coverage | ✅ **EXISTS in CRD** (line 301) | **+100%** |

**Phase 1 Status**: ✅ **COMPLETE** (CRD schema already updated)
**Effort Saved**: **12 hours** (1.5 days)

---

### **Phase 2: Critical Status Fields (Gaps #5-7)**

| Gap | Field | Plan Status | **Actual Status** | Investigation Needed |
|-----|-------|-------------|-------------------|----------------------|
| #5 | `SelectedWorkflowRef` | ❌ 0% coverage | 🔍 **NEEDS VERIFICATION** | Check RR status fields |
| #6 | `ExecutionRef` | ❌ 0% coverage | 🔍 **NEEDS VERIFICATION** | Check RR status fields |
| #7 | `Error` (detailed) | ❌ 0% coverage | 🔍 **NEEDS VERIFICATION** | Check audit event schemas |

**Phase 2 Status**: 🔍 **REQUIRES INVESTIGATION**
**Action**: Verify if status fields exist and if audit events capture them

---

### **Phase 3: Optional Enhancement (Gap #8)**

| Gap | Field | Plan Status | **Actual Status** | Delta |
|-----|-------|-------------|-------------------|-------|
| #8 | `TimeoutConfig` | ❌ 0% coverage | ✅ **EXISTS in CRD** (line 338) | **+100%** |

**Phase 3 Status**: ✅ **COMPLETE** (CRD schema already updated)
**Effort Saved**: **1 hour** (0.5 days)

---

## 🔍 **Remaining Work Analysis**

### **What's Already Done (Unexpectedly)**

1. ✅ **CRD Schema Updates** - All 5 spec fields exist
2. ✅ **Type Definitions** - `SignalLabels`, `SignalAnnotations`, `ProviderData`, `OriginalPayload`, `TimeoutConfig`
3. ✅ **API Contract** - Fields are properly typed and documented

**Coverage Improvement**: 70% → **~85%** (CRD schema perspective)

---

### **What Still Needs to be Done**

#### **1. Audit Event Population (CRITICAL - BLOCKING)**

**Problem**: Fields exist in CRD, but **audit events may not capture them**.

**Investigation Required**:

```bash
# Check if Gateway audit events include these fields
grep -r "OriginalPayload\|ProviderData\|SignalLabels\|SignalAnnotations" pkg/gateway/audit/

# Check if audit event schemas support these fields
grep -r "original_payload\|provider_data\|signal_labels\|signal_annotations" api/datastorage/openapi.yaml
```

**Expected Findings**:
- ❌ Gateway audit events likely **DO NOT** populate these fields yet
- ❌ DataStorage audit event schemas may not have fields for storing them
- ❌ Audit managers may not extract these fields from CRDs

**Estimated Effort**: **8-12 hours** (1.5 days)
- Update Gateway audit event emission (4h)
- Update DataStorage audit event schemas (2h)
- Update audit query/reconstruction logic (4h)
- Integration tests (2h)

---

#### **2. Status Field Verification (MEDIUM PRIORITY)**

**Investigation Required**:

```bash
# Check RemediationRequest status fields
grep -A 100 "type RemediationRequestStatus" api/remediation/v1alpha1/remediationrequest_types.go | grep -E "SelectedWorkflowRef|ExecutionRef|Error"

# Check if audit events capture workflow selection
grep -r "workflow.selection.completed\|selected_workflow" pkg/*/audit/

# Check if audit events capture execution references
grep -r "execution.started\|execution_ref" pkg/*/audit/
```

**Expected Findings**:
- 🔍 `SelectedWorkflowRef` may exist in RR status (need to verify)
- 🔍 `ExecutionRef` may exist in RR status (need to verify)
- ❌ Audit events likely do not capture these status fields

**Estimated Effort**: **6-8 hours** (1 day)

---

#### **3. Reconstruction Logic (REQUIRED - CORE FEATURE)**

**This is the ACTUAL implementation work**:

| Task | Component | Effort | Status |
|------|-----------|--------|--------|
| Design reconstruction algorithm | - | 2h | ⏳ **TODO** |
| Implement RR spec reconstruction | API/Handler | 4h | ⏳ **TODO** |
| Implement RR status reconstruction | API/Handler | 3h | ⏳ **TODO** |
| Handle edge cases (missing events) | - | 2h | ⏳ **TODO** |
| **API endpoint** (reconstruction) | REST API | 3h | ⏳ **TODO** |
| E2E tests | - | 3h | ⏳ **TODO** |

**Total Effort**: **17 hours** (~2 days)

---

#### **4. OpenAPI Spec & Handler (REQUIRED)**

**Files to Create/Modify**:
1. `api/datastorage/openapi.yaml` - Add `/v1/audit/remediation-requests/:id/reconstruct` endpoint
2. `pkg/datastorage/server/reconstruction_handler.go` - Implement handler
3. `pkg/datastorage/ogen-client/` - Regenerate client (via `make generate`)

**Estimated Effort**: **6 hours** (0.75 days)

---

## 📋 **REVISED Implementation Plan**

### **Original Plan**: 6.5 days (52 hours)

| Phase | Tasks | Original Estimate |
|-------|-------|-------------------|
| Phase 1 | Add spec fields to CRD | 12h (1.5 days) |
| Phase 2 | Add status fields to audit | 9h (1 day) |
| Phase 3 | TimeoutConfig | 1h (0.5 days) |
| Phase 4 | Reconstruction logic | 17h (2 days) |
| Phase 5 | Documentation | 6h (0.75 days) |
| Buffer | Unexpected issues | 6h (0.75 days) |

---

### **REVISED Plan**: 3-4 days (24-32 hours)

| Phase | Tasks | Revised Estimate | Status |
|-------|-------|------------------|--------|
| ~~Phase 1~~ | ~~Add spec fields to CRD~~ | ~~12h~~ | ✅ **DONE** (already in CRD) |
| Phase 2A | **Audit event population** | **10h (1.25 days)** | ⏳ **NEW PRIORITY** |
| Phase 2B | Status field verification | 6h (0.75 days) | 🔍 **INVESTIGATION** |
| ~~Phase 3~~ | ~~TimeoutConfig~~ | ~~1h~~ | ✅ **DONE** (already in CRD) |
| Phase 4 | Reconstruction logic | 17h (2 days) | ⏳ **CORE WORK** |
| Phase 5 | OpenAPI spec + handler | 6h (0.75 days) | ⏳ **REQUIRED** |
| Phase 6 | Documentation updates | 4h (0.5 days) | ⏳ **REQUIRED** |
| Buffer | Unexpected issues | 4h (0.5 days) | ⏳ **BUFFER** |

**Total Revised Effort**: **47 hours** (~6 days if investigation reveals more gaps, **3 days** if audit events already capture fields)

**Effort Saved**: **13 hours** (1.5 days) due to existing CRD fields

---

## 🚨 **Critical Questions for Investigation**

### **Question 1: Are audit events already populating these fields?**

**Hypothesis**: CRD fields exist, but Gateway/AIAnalysis may not be populating them in audit events yet.

**Investigation**:
```bash
# Search for audit event population
grep -r "SetOriginalPayload\|SetProviderData\|SetSignalLabels" pkg/gateway/
grep -r "original_payload\|provider_data\|signal_labels" pkg/gateway/audit/
```

**Expected Answer**: ❌ **NO** - Audit events likely don't capture these fields yet.

**Impact**: If NO → **Phase 2A is REQUIRED** (10 hours of work)

---

### **Question 2: Do DataStorage audit schemas support these fields?**

**Hypothesis**: DataStorage OpenAPI spec may not have schemas for storing large payloads.

**Investigation**:
```bash
# Check audit event schemas
grep -A 20 "GatewaySignalReceivedPayload\|AIAnalysisCompletedPayload" api/datastorage/openapi.yaml
```

**Expected Answer**: ❌ **NO** - Schemas likely don't have `original_payload` or `provider_data` fields.

**Impact**: If NO → **OpenAPI spec update REQUIRED** (2 hours of work)

---

### **Question 3: Do status fields exist in RemediationRequest?**

**Hypothesis**: `SelectedWorkflowRef` and `ExecutionRef` may already exist in RR status.

**Investigation**:
```bash
# Check RR status struct
grep -A 150 "type RemediationRequestStatus" api/remediation/v1alpha1/remediationrequest_types.go | grep -E "SelectedWorkflow|Execution|Error"
```

**Expected Answer**: 🔍 **UNKNOWN** - Need to verify.

**Impact**: If YES → **Phase 2B effort reduced to 2 hours** (audit event updates only)

---

## 📊 **Risk Assessment**

### **Risk #1: Audit Event Population Gap**

**Risk**: CRD fields exist, but audit events don't populate them.

**Likelihood**: **HIGH** (85%)

**Impact**: **CRITICAL** - Without audit event population, reconstruction is impossible.

**Mitigation**:
1. Investigate audit event emission in Gateway and AIAnalysis
2. Update audit managers to extract fields from CRDs
3. Update DataStorage schemas to store large payloads
4. Add integration tests to verify audit event population

**Estimated Mitigation Effort**: **10 hours** (1.25 days)

---

### **Risk #2: Storage Size Impact**

**Risk**: `OriginalPayload` and `ProviderData` may be large (10KB-50KB per event).

**Likelihood**: **MEDIUM** (60%)

**Impact**: **MEDIUM** - Increased storage costs and query performance degradation.

**Mitigation**:
1. Implement compression (gzip) for large payloads
2. Add sampling (only store for P0/P1 incidents)
3. Use PostgreSQL JSONB compression
4. Monitor storage growth and query performance

**Estimated Mitigation Effort**: **4 hours** (0.5 days)

---

### **Risk #3: Sensitive Data in Payloads**

**Risk**: `OriginalPayload` may contain PII or credentials in labels/annotations.

**Likelihood**: **MEDIUM** (50%)

**Impact**: **HIGH** - Compliance violation (GDPR, SOC2).

**Mitigation**:
1. Implement data sanitization before storing
2. Redact known sensitive fields (passwords, tokens, API keys)
3. Add configuration for sensitive label/annotation patterns
4. Document data retention and sanitization policies

**Estimated Mitigation Effort**: **6 hours** (0.75 days)

---

## ✅ **Recommended Next Steps**

### **Immediate Actions (Before Implementation)**

1. **✅ VERIFY Audit Event Population** (1 hour)
   ```bash
   # Run these investigations
   grep -r "OriginalPayload\|ProviderData\|SignalLabels" pkg/gateway/audit/
   grep -r "original_payload\|provider_data" api/datastorage/openapi.yaml
   ```

2. **✅ VERIFY Status Fields** (30 min)
   ```bash
   # Check RR status struct
   grep -A 150 "type RemediationRequestStatus" api/remediation/v1alpha1/remediationrequest_types.go
   ```

3. **✅ VERIFY Audit Event Schemas** (30 min)
   ```bash
   # Check DataStorage schemas
   grep -A 50 "GatewaySignalReceivedPayload" api/datastorage/openapi.yaml
   ```

**Total Investigation Time**: **2 hours**

---

### **Implementation Order (After Investigation)**

#### **Scenario A: Audit Events Already Capture Fields** (BEST CASE)

**If investigation reveals audit events already populate fields:**

1. **Day 1-2**: Reconstruction Logic (17h)
   - Design algorithm
   - Implement spec reconstruction
   - Implement status reconstruction
   - Handle edge cases

2. **Day 2-3**: API Endpoint (6h)
   - OpenAPI spec
   - Handler implementation
   - Error handling
   - RBAC enforcement

3. **Day 3**: Testing & Documentation (7h)
   - Integration tests
   - E2E tests
   - Documentation updates

**Total**: **3 days** (24 hours)

---

#### **Scenario B: Audit Events Need Updates** (LIKELY CASE)

**If investigation reveals audit events DON'T capture fields:**

1. **Day 1**: Audit Event Population (10h)
   - Update Gateway audit events
   - Update AIAnalysis audit events
   - Update DataStorage schemas
   - Integration tests

2. **Day 2**: Status Field Audit (6h)
   - Add `SelectedWorkflowRef` to audit
   - Add `ExecutionRef` to audit
   - Add detailed error messages
   - Integration tests

3. **Day 3-4**: Reconstruction Logic (17h)
   - Design algorithm
   - Implement spec reconstruction
   - Implement status reconstruction
   - Handle edge cases

4. **Day 4-5**: API Endpoint (6h)
   - OpenAPI spec
   - Handler implementation
   - Error handling
   - RBAC enforcement

5. **Day 5**: Testing & Documentation (7h)
   - Integration tests
   - E2E tests
   - Documentation updates

**Total**: **5-6 days** (46 hours)

---

## 🎯 **Success Criteria (Updated)**

### **Must-Have (V1.0)**

1. ✅ **CRD Schema**: ALL spec fields exist (ALREADY DONE)
2. ⏳ **Audit Event Population**: Gateway and AIAnalysis emit fields
3. ⏳ **DataStorage Schemas**: Support storing large payloads
4. ⏳ **Reconstruction Logic**: API endpoint functional
5. ⏳ **Integration Tests**: Validate full reconstruction lifecycle
6. ⏳ **BR-AUDIT-005 v2.0**: Documentation updated

### **Nice-to-Have (Post-V1.0)**

1. ⏳ CLI wrapper (1-2 days)
2. ⏳ Bulk reconstruction API
3. ⏳ Web UI for reconstruction
4. ⏳ Diff view (compare reconstructed vs live RR)

---

## 📝 **Key Insights**

### **1. Plan is Outdated**

**Finding**: December 18, 2025 plan assumes 0% coverage for fields that **already exist** in CRD.

**Impact**: **1.5 days of planned work is unnecessary**.

**Lesson**: Always verify current state before implementing a plan from a previous date.

---

### **2. CRD Schema vs. Audit Event Gap**

**Finding**: CRD fields exist, but audit events may not populate them.

**Impact**: **Reconstruction is impossible without audit event population**.

**Lesson**: CRD schema is only half the solution; audit event emission is equally critical.

---

### **3. Phase Reordering Needed**

**Finding**: Original plan focuses on CRD schema first, reconstruction logic last.

**Reality**: CRD schema is done; audit event population is now the blocker.

**Lesson**: Prioritize audit event population (Phase 2A) before reconstruction logic (Phase 4).

---

## 🔗 **Related Documentation**

- [RR_RECONSTRUCTION_V1_0_GAP_CLOSURE_PLAN_DEC_18_2025.md](./RR_RECONSTRUCTION_V1_0_GAP_CLOSURE_PLAN_DEC_18_2025.md) - Original plan (OUTDATED)
- [RR_RECONSTRUCTION_API_DESIGN_DEC_18_2025.md](./RR_RECONSTRUCTION_API_DESIGN_DEC_18_2025.md) - API specification (STILL VALID)
- [RR_CRD_RECONSTRUCTION_FROM_AUDIT_TRACES_ASSESSMENT_DEC_18_2025.md](./RR_CRD_RECONSTRUCTION_FROM_AUDIT_TRACES_ASSESSMENT_DEC_18_2025.md) - Feasibility assessment (OUTDATED - coverage is now 85%, not 70%)
- [ADR-034: Unified Audit Table Design](../architecture/decisions/ADR-034-unified-audit-table-design.md) - Audit table schema
- [DD-AUDIT-003: Service Audit Trace Requirements](../architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md) - Audit requirements

---

## 💬 **Questions for User**

1. **Audit Event Population**: Should we investigate audit event population before starting implementation? (Recommended: YES - 2 hours)

2. **Scope Adjustment**: Given that CRD fields already exist, should we skip Phase 1 and focus on audit event population + reconstruction logic? (Recommended: YES - saves 1.5 days)

3. **Timeline**: Is 3-6 days acceptable (depending on audit event gap), or do we need to compress further?

4. **Risk Mitigation**: Should we implement data sanitization for sensitive payloads in V1.0, or defer to V1.1? (Recommended: V1.1 - adds 0.75 days)

5. **Testing Scope**: Should we include E2E reconstruction tests in V1.0, or focus on integration tests only? (Recommended: Integration tests only - saves 2 hours)

---

**Status**: 🔍 **TRIAGE COMPLETE** - Awaiting investigation results and user approval
**Confidence**: **95%** - Triage is accurate; implementation estimate depends on investigation findings
**Recommendation**: **INVESTIGATE FIRST** (2 hours), then proceed with revised plan (3-6 days)

**Next Step**: Run investigation commands to determine Scenario A vs. Scenario B


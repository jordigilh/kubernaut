# HAPI Test Failures - Timeline Analysis: Why Are They Failing Now?

**Date**: January 22, 2026
**Status**: ✅ **REGRESSION IDENTIFIED**
**Root Cause**: New feature (workflow catalog audit events) broke existing HAPI tests
**Timeline**: Tests were passing before November 27, 2025

---

## 🔍 **User Question**

> "How come these are showing now when we completed SOC2 and we ran the integration suite they were working?"

**Answer**: The tests **WERE** working during SOC2 compliance. They broke after **ADR-034 v1.1** was implemented on **November 27, 2025**, which added workflow catalog audit events.

---

## 📅 **Timeline of Changes**

### **Phase 1: SOC2 Compliance Period** (Before November 27, 2025)

**Test State**: ✅ **PASSING**

**Why Tests Passed**:
1. DataStorage **DID NOT** emit `workflow.catalog.search_completed` audit events
2. HAPI integration tests queried DataStorage for all events
3. All events returned had `event_category="analysis"` or other non-workflow categories
4. Test assertion `assert event.event_category == "analysis"` was true

**Architecture**:
```
HAPI → DataStorage workflow search
     → NO AUDIT EVENTS EMITTED
     → Tests see only HAPI-generated events
```

---

### **Phase 2: ADR-034 v1.1 Implementation** (November 27, 2025)

**Change**: Added Workflow Catalog Service audit events

**From ADR-034 Version History**:
```markdown
| **v1.1** | 2025-11-27 | Added Workflow Catalog Service (Phase 3, Item 4):
  `workflow.catalog.search_completed` event type with scoring breakdown
  for debugging workflow selection. Added DD-WORKFLOW-014 cross-reference.
```

**Implementation Document**: `docs/services/stateless/data-storage/implementation/DD-AUDIT-023-030-WORKFLOW-SEARCH-AUDIT-IMPLEMENTATION-PLAN-V1.2.md`

**Business Requirement**: BR-AUDIT-023
> "Audit event generation in Data Storage Service: Every workflow search generates `workflow.catalog.search_completed` event"

**New Architecture**:
```
HAPI → DataStorage workflow search
     → DataStorage emits workflow.catalog.search_completed
       event_category="workflow" ✅ (per ADR-034 v1.1)
     → Tests now see MIXED event categories
```

---

### **Phase 3: Test Regression** (After November 27, 2025)

**Test State**: ❌ **FAILING**

**What Changed**:
1. DataStorage **NOW EMITS** `workflow.catalog.search_completed` events
2. Events have `event_category="workflow"` (correct per ADR-034 v1.1)
3. HAPI integration tests still assert `event_category == "analysis"`
4. Assertion fails because workflow events are present

**Error**:
```python
AssertionError: Expected event_category='analysis' for HAPI, got 'workflow'
```

---

## 🔧 **Why Tests Weren't Updated**

### **Root Cause**: **Architectural Change Without Test Updates**

**What Happened**:
1. **DD-WORKFLOW-014** (Workflow Selection Audit Trail) was implemented
2. DataStorage service was updated to emit workflow audit events
3. **HAPI integration tests were NOT updated** to expect new event categories
4. Tests continued to assume all HAPI-triggered events would have category `"analysis"`

### **Testing Gap**

**Implementation Checklist** (from DD-AUDIT-023-030-WORKFLOW-SEARCH-AUDIT-IMPLEMENTATION-PLAN-V1.2.md):
```markdown
DAY 3 - CHECK PHASE (5 hours)
- [x] All unit tests passing
- [x] All integration tests passing  # ← DataStorage integration tests
- [ ] HAPI integration tests updated  # ← MISSED
```

**The Gap**: DataStorage team validated their own integration tests (workflow search audit emission), but HAPI integration tests (which consume those events) were not updated.

---

## 📊 **ADR-034 Evolution Summary**

| Version | Date | Key Change | Impact on HAPI Tests |
|---------|------|------------|----------------------|
| **v1.0** | 2025-11-08 | Initial unified audit table | ✅ Tests passing - no workflow events |
| **v1.1** | 2025-11-27 | **Added workflow.catalog.search_completed** | ❌ **Tests start failing** |
| **v1.2** | 2025-12-18 | Standardized event_category naming (service-level) | ❌ Tests still failing (same issue) |
| **v1.3** | 2025-12-18 | Added RR reconstruction field mapping | ❌ Tests still failing |
| **v1.4** | 2026-01-06 | Added Authentication Webhook category | ❌ Tests still failing |
| **v1.5** | 2026-01-08 | Fixed WorkflowExecution category | ❌ Tests still failing |

---

## 🎯 **Why This Matters**

### **This is NOT a Test Bug - It's a Regression**

**Original Assessment** (from previous RCA):
> "Tests are incorrectly written - NOT business logic bugs"

**Corrected Assessment**:
> "Tests were CORRECT for the architecture they were written against. New feature (workflow audit events) introduced breaking change that requires test updates."

### **Classification**

| Type | Status |
|------|--------|
| **Business Logic Bug** | ❌ No - DataStorage audit emission is working correctly |
| **Test Bug** | ❌ No - Tests were correct for original architecture |
| **Regression** | ✅ **YES** - New feature broke existing tests |
| **Test Maintenance Gap** | ✅ **YES** - Tests need updating for new architecture |

---

## ✅ **What Needs to Happen**

### **Fix 1: Update HAPI Integration Tests** (Required)

**File**: `holmesgpt-api/tests/integration/test_hapi_audit_flow_integration.py`

**Current (line 544) - Broken since November 27, 2025**:
```python
assert event.event_category == "analysis", \
    f"Expected event_category='analysis' for HAPI, got '{event.event_category}'"
```

**Fixed - Aligned with ADR-034 v1.1+ architecture**:
```python
# ADR-034 v1.1+: HAPI triggers workflow search in DataStorage,
# which emits workflow.catalog.search_completed events with category="workflow".
# HAPI integration tests must expect MIXED event categories.
valid_categories = ["analysis", "workflow", "holmesgptapi"]
assert event.event_category in valid_categories, \
    f"Expected ADR-034 category in {valid_categories}, got '{event.event_category}'"

# More specific validation per event type:
if event.event_type == "workflow.catalog.search_completed":
    assert event.event_category == "workflow", \
        "Workflow catalog events must have category='workflow' (ADR-034 v1.1)"
elif event.event_type.startswith("aianalysis."):
    assert event.event_category == "analysis", \
        "AI Analysis events must have category='analysis' (ADR-034 v1.0)"
```

### **Fix 2: Update Test Documentation** (Required)

Add comment explaining the architectural evolution:
```python
"""
ARCHITECTURAL NOTE (ADR-034 v1.1+ - November 27, 2025):

Prior to ADR-034 v1.1, HAPI integration tests only saw events with
category="analysis" because DataStorage did not emit workflow audit events.

After ADR-034 v1.1, DataStorage emits workflow.catalog.search_completed
events for every workflow search, with category="workflow" (correct per
ADR-034 service-level category naming convention).

HAPI integration tests MUST now handle MIXED event categories:
- "workflow" - DataStorage workflow catalog searches
- "analysis" - AI Analysis service operations
- "holmesgptapi" - HAPI HTTP API operations (future)

This is NOT a bug - it's architectural evolution per DD-WORKFLOW-014.
"""
```

### **Fix 3: Prevent Future Regressions** (Recommended)

**Add Cross-Service Test Validation Checklist**:

When implementing new audit events in one service:
1. ✅ Validate service's own integration tests pass
2. ✅ **Check downstream services** that consume audit events
3. ✅ Update downstream integration tests if event categories change
4. ✅ Document architectural changes in test comments

**Example**: DD-AUDIT-023-030 Implementation Checklist should have included:
```markdown
## Cross-Service Impact Analysis
- [ ] Check which services query DataStorage audit events
- [ ] HAPI integration tests - **IMPACTED** (expects only analysis category)
- [ ] Update HAPI tests to handle workflow category
- [ ] Document change in HAPI test comments
```

---

## 🔗 **Related Documentation**

### **ADR-034 History**
- `docs/architecture/decisions/ADR-034-unified-audit-table-design.md`
- Version 1.0 (2025-11-08): Initial design
- Version 1.1 (2025-11-27): **Added workflow.catalog.search_completed** ← Breaking change

### **Implementation Plans**
- `docs/services/stateless/data-storage/implementation/DD-AUDIT-023-030-WORKFLOW-SEARCH-AUDIT-IMPLEMENTATION-PLAN-V1.2.md`
- Status: ✅ IMPLEMENTED (November 27, 2025)
- Missing: HAPI integration test updates

### **Business Requirements**
- BR-AUDIT-023: Workflow search audit event generation
- Implemented: ✅ YES (DataStorage emits events correctly)
- Tests Updated: ❌ NO (HAPI integration tests not updated)

---

## 📋 **Lessons Learned**

### **What Went Right**
✅ DataStorage correctly implements BR-AUDIT-023
✅ Audit events follow ADR-034 v1.1 event_category semantics
✅ Infrastructure is working correctly (verified via must-gather)

### **What Went Wrong**
❌ HAPI integration tests not updated when workflow audit events added
❌ No cross-service test impact analysis in implementation plan
❌ Breaking change not flagged during DD-WORKFLOW-014 implementation

### **Process Improvement**
**Add to APDC CHECK Phase**:
```markdown
## Cross-Service Impact Checklist
When adding new audit events:
1. List all services that query DataStorage audit events
2. For each service, check if event_category expectations change
3. Update downstream service integration tests
4. Add architectural evolution comments to tests
5. Document cross-service changes in handoff
```

---

## 🎯 **Conclusion**

**Why Tests Were Working**: Before November 27, 2025, DataStorage didn't emit workflow audit events, so HAPI tests only saw category=`"analysis"` ✅

**Why Tests Are Failing Now**: After November 27, 2025, DataStorage emits workflow audit events with category=`"workflow"`, but HAPI tests still expect only category=`"analysis"` ❌

**Classification**: **REGRESSION** caused by new feature (workflow audit events) without corresponding test updates

**Fix Required**: Update HAPI integration tests to expect MIXED event categories per ADR-034 v1.1+ architecture

**Merge Status for `setup-envtest`**: ✅ **STILL READY** - This is a separate HAPI issue, not related to envtest refactoring

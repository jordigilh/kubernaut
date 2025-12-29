# DD-AUDIT-004 Rename Triage - Service-Specific Name Violation

**Date**: December 17, 2025
**Scope**: Design Decision naming conventions
**Status**: ❌ **VIOLATION - MUST FIX**
**Priority**: 🔴 **P1** (Design Decision Standards Violation)

---

## 🚨 **Problem Statement**

**Current**: `DD-AUDIT-004-audit-type-safety-specification.md`
**Violation**: Design Decision includes service name ("AIANALYSIS")

**Issue**: DDs should be **GENERIC** architectural patterns, not service-specific implementations.

---

## 📊 **Violation Analysis**

### **What DD-AUDIT-004 Actually Is**

**Content Analysis**:
- **Problem**: Using `map[string]interface{}` for audit event data payloads
- **Solution**: Create structured Go types for audit event payloads
- **Scope**: **ALL SERVICES** (not just AIAnalysis)

**Affected Services**:
1. ✅ **AIAnalysis**: Implemented (6 structured types)
2. ✅ **WorkflowExecution**: Implemented (1 structured type)
3. ✅ **Gateway**: Implemented (structured types)
4. ✅ **DataStorage**: Implemented (structured types)
5. ❌ **Notification**: NOT implemented (current violation)

**Finding**: This is a **PROJECT-WIDE PATTERN**, not an AIAnalysis-specific decision.

### **Why Service Names in DDs are Wrong**

**From ADR README (lines 113-120)**:
```markdown
### **When to Create an ADR**

Create an ADR for decisions that:
- ✅ Affect multiple services or the overall architecture
- ✅ Have long-term implications (>6 months)
- ✅ Involve trade-offs between alternatives
- ✅ Set precedents for future decisions
- ✅ Change existing architectural patterns
```

**This DD**:
- ✅ Affects **ALL** services with audit events (5+ services)
- ✅ Long-term implication: Coding standards mandate
- ✅ Involves trade-offs: Type safety vs. flexibility
- ✅ Sets precedent: All audit event data must be structured
- ✅ Changes pattern: From `map[string]interface{}` to structured types

**Conclusion**: ❌ **Service name should NOT be in the DD ID**

### **Existing DD Naming Patterns**

**Correct Generic DDs**:
- `DD-AUDIT-001`: Audit Responsibility Pattern (applies to ALL services)
- `DD-AUDIT-002`: Audit Shared Library Design (applies to ALL services)
- `DD-AUDIT-003`: Service Audit Trace Requirements (applies to 8 of 11 services)
- `DD-004`: RFC 7807 Error Response Standard (applies to ALL HTTP services)
- `DD-005`: Observability Standards (applies to ALL services)

**Correct Service-Specific DDs** (when pattern is truly service-specific):
- `DD-CONTEXT-001`: Cache Stampede Prevention (Context API only)
- `DD-GATEWAY-004`: Redis Memory Optimization (Gateway Service only)
- `DD-HOLMESGPT-008`: Safety-Aware Investigation (HolmesGPT API only)

**DD-AUDIT-004 Pattern**:
- ❌ **WRONG**: Named as service-specific
- ✅ **REALITY**: Generic pattern applied to ALL services

---

## 🎯 **Correct Name**

### **Proposed**: `DD-AUDIT-004-structured-types-for-audit-event-payloads.md`

**Rationale**:
1. ✅ **DD-AUDIT-XXX prefix**: Follows existing audit DD pattern
2. ✅ **Next sequential number**: DD-AUDIT-001, 002, 003 exist → next is 004
3. ✅ **Generic title**: No service name, describes the pattern
4. ✅ **Descriptive**: Clear what the DD is about
5. ✅ **Consistent**: Matches other generic DDs

### **Alternative Considered**: `DD-TYPE-001-audit-event-payload-type-safety.md`

**Rejected**:
- ⚠️ Creates new DD-TYPE-XXX namespace (unnecessary)
- ⚠️ Less discoverable (audit-related, should be DD-AUDIT-XXX)
- ⚠️ Inconsistent with existing DD-AUDIT-XXX pattern

---

## 📋 **Implementation Plan**

### **Phase 1: Rename DD File (5 minutes)**

**Action**: Rename file
```bash
mv docs/architecture/decisions/DD-AUDIT-004-audit-type-safety-specification.md \
   docs/architecture/decisions/DD-AUDIT-004-structured-types-for-audit-event-payloads.md
```

**Update Header**:
```markdown
# DD-AUDIT-004: Structured Types for Audit Event Payloads

**Status**: ✅ **APPROVED & IMPLEMENTED** (2025-12-16)
**Priority**: P0 (Type Safety Mandate)
**Last Reviewed**: 2025-12-16
**Confidence**: 95%
**Owner**: All Services (First Implemented by AIAnalysis Team)
**Implements**: [DD-AUDIT-003](./DD-AUDIT-003-service-audit-trace-requirements.md)
**Related**: Project Coding Standards ([02-go-coding-standards.mdc](../../.cursor/rules/02-go-coding-standards.mdc))
```

### **Phase 2: Update All References (20 files, ~30 minutes)**

**Files to Update**:
1. `docs/handoff/NT_UNSTRUCTURED_DATA_STRICT_TRIAGE.md` (2 references)
2. `docs/handoff/NT_SLACK_SDK_TRIAGE.md` (1 reference)
3. `internal/controller/workflowexecution/audit.go` (code comment)
4. `test/e2e/workflowexecution/02_observability_test.go` (test comment)
5. `pkg/workflowexecution/audit_types.go` (package comment)
6. `docs/handoff/WE_ADR032_E2E_VALIDATION_COMPLETE_DEC_17_2025.md`
7. `docs/handoff/WE_E2E_AUDIT_VALIDATION_EXTENDED.md`
8. `docs/handoff/TRIAGE_WE_AUDIT_VIOLATIONS_DEC_17_2025.md`
9. `docs/handoff/WE_REFACTORING_COMPLETE_DEC_17_2025.md`
10. `docs/handoff/WE_REFACTORING_SESSION_SUMMARY_DEC_17_2025.md`
11. `docs/handoff/AA_INTEGRATION_TEST_AUDIT_COVERAGE_TRIAGE_DEC_17_2025.md`
12. `test/e2e/aianalysis/05_audit_trail_test.go` (test comment)
13. `docs/handoff/AA_E2E_AUDIT_IMPLEMENTATION_DEC_17_2025.md`
14. `docs/handoff/AA_ADR_032_VIOLATION_TRIAGE_DEC_17_2025.md`
15. `docs/handoff/AA_V1_0_FINAL_STATUS_DEC_16_2025.md`
16. `docs/handoff/AA_INTEGRATION_TEST_EDGE_CASE_TRIAGE.md`
17. `docs/handoff/AA_DD_AIANALYSIS_005_TYPE_SAFETY_IMPLEMENTED.md` (rename file too)
18. `docs/handoff/AA_DD_RESTRUCTURING_COMPLETE.md`
19. `docs/handoff/AA_DD_DOCUMENTATION_STRUCTURE_TRIAGE.md`
20. `docs/handoff/AA_AUDIT_TYPE_SAFETY_VIOLATION_TRIAGE.md`

**Update Pattern**:
```bash
# Find and replace in all files
DD-AUDIT-004 → DD-AUDIT-004
DD-AUDIT-004-audit-type-safety-specification.md → DD-AUDIT-004-structured-types-for-audit-event-payloads.md
```

### **Phase 3: Update README Index (~5 minutes)**

**File**: `docs/architecture/decisions/README.md`

**Add Entry** (after DD-AUDIT-003, line 53):
```markdown
||| DD-AUDIT-004 | [Structured Types for Audit Event Payloads](./DD-AUDIT-004-structured-types-for-audit-event-payloads.md) | All Services | ✅ Approved | 2025-12-16 | Type-safe audit event data (eliminates `map[string]interface{}`) |
```

### **Phase 4: Rename Handoff Documentation (~5 minutes)**

**Action**: Rename AA-specific handoff doc to be generic

```bash
mv docs/handoff/AA_DD_AIANALYSIS_005_TYPE_SAFETY_IMPLEMENTED.md \
   docs/handoff/DD_AUDIT_004_TYPE_SAFETY_SPECIFICATION.md
```

**Update Header**:
```markdown
# DD-AUDIT-004: Structured Types for Audit Event Payloads - Implementation Status

**Date**: December 16, 2025
**DD**: [DD-AUDIT-004](../architecture/decisions/DD-AUDIT-004-structured-types-for-audit-event-payloads.md)
**Scope**: All Services with Audit Events
**Status**: ✅ **PARTIALLY IMPLEMENTED**

## 📊 **Implementation Status by Service**

| Service | Status | Structured Types | Reference |
|---------|--------|------------------|-----------|
| **AIAnalysis** | ✅ Complete | 6 types | `pkg/aianalysis/audit/event_types.go` |
| **WorkflowExecution** | ✅ Complete | 1 type | `pkg/workflowexecution/audit_types.go` |
| **Gateway** | ✅ Complete | Structured types | `pkg/datastorage/audit/gateway_event.go` |
| **DataStorage** | ✅ Complete | Structured types | `pkg/datastorage/audit/*.go` |
| **Notification** | ❌ Violation | None (uses `map[string]interface{}`) | `internal/controller/notification/audit.go` |

**First Implemented By**: AIAnalysis Team (December 16, 2025)
**Pattern Established**: All services must use structured types for audit event payloads
```

---

## ⏱️ **Effort Estimation**

| Phase | Effort | Risk |
|-------|--------|------|
| Phase 1: Rename DD file | 5 min | Low |
| Phase 2: Update 20 file references | 30 min | Low |
| Phase 3: Update README index | 5 min | Low |
| Phase 4: Rename handoff doc | 5 min | Low |
| **TOTAL** | **45 minutes** | **Low** |

**Confidence**: 100% (mechanical change, no functional impact)

---

## ✅ **Benefits of Rename**

### **Discoverability**

**BEFORE**:
- ❌ Looks like AIAnalysis-only decision
- ❌ Other services don't know this applies to them
- ❌ Inconsistent with DD-AUDIT-001, 002, 003 pattern

**AFTER**:
- ✅ Clear this applies to ALL services
- ✅ Easy to discover (DD-AUDIT-XXX pattern)
- ✅ Consistent with other audit DDs

### **Consistency**

**Current DD-AUDIT Pattern**:
- `DD-AUDIT-001`: Audit Responsibility Pattern (generic)
- `DD-AUDIT-002`: Audit Shared Library Design (generic)
- `DD-AUDIT-003`: Service Audit Trace Requirements (generic)
- ❌ `DD-AUDIT-004`: Audit Type Safety (WRONG - should be DD-AUDIT-004)

**After Rename**:
- `DD-AUDIT-001`: Audit Responsibility Pattern
- `DD-AUDIT-002`: Audit Shared Library Design
- `DD-AUDIT-003`: Service Audit Trace Requirements
- ✅ `DD-AUDIT-004`: Structured Types for Audit Event Payloads (CORRECT)

### **Enforcement**

**BEFORE**:
- ⚠️ Notification sees "DD-AUDIT-004" → assumes it's AIAnalysis-specific
- ❌ No clear indication this is a project-wide mandate

**AFTER**:
- ✅ "DD-AUDIT-004" → clearly project-wide audit standard
- ✅ Easier to reference in coding standards violations
- ✅ Clear this applies to ALL services

---

## 📚 **Authoritative References**

### **DD Naming Guidelines**

**From**: `docs/architecture/decisions/README.md` (lines 40-81)

**Project-Wide Standards** (lines 42-53):
- DD-001 to DD-005: Generic, cross-service patterns
- DD-AUDIT-001 to DD-AUDIT-003: Audit-specific, cross-service patterns
- DD-API-XXX, DD-TEST-XXX: Domain-specific, cross-service patterns

**Service-Specific Decisions** (lines 55-79):
- DD-CONTEXT-XXX: Context API only
- DD-GATEWAY-XXX: Gateway Service only
- DD-HOLMESGPT-XXX: HolmesGPT API only
- DD-EFFECTIVENESS-XXX: Effectiveness Monitor only

**DD-AUDIT-004 Violates This Pattern**:
- ❌ Named as service-specific (DD-AIANALYSIS-XXX)
- ✅ Actually project-wide (applies to ALL services)
- ❌ Should be DD-AUDIT-004 (next in DD-AUDIT-XXX sequence)

---

## 🎯 **Conclusion**

### **Verdict**: ❌ **MUST RENAME**

**Rationale**:
1. ❌ **Current name violates DD naming conventions**
2. ✅ **Pattern applies to ALL services** (not AIAnalysis-specific)
3. ✅ **Should follow DD-AUDIT-XXX pattern** (audit-related, cross-service)
4. ✅ **Improves discoverability** (easier for other services to find)
5. ✅ **Enhances enforcement** (clear this is a project-wide mandate)

### **Approved Rename**

**OLD**: `DD-AUDIT-004-audit-type-safety-specification.md`
**NEW**: `DD-AUDIT-004-structured-types-for-audit-event-payloads.md`

**Impact**: Low (documentation only, no functional changes)
**Effort**: 45 minutes
**Risk**: Low (mechanical rename)
**Priority**: P1 (standards violation)

---

**Triaged By**: Architecture Team
**Date**: December 17, 2025
**Status**: ❌ **VIOLATION CONFIRMED - RENAME REQUIRED**
**Confidence**: 100% (clear naming convention violation)





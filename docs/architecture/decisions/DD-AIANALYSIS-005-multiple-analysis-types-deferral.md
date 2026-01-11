# DD-AIANALYSIS-005: Multiple Analysis Types Feature Deferral

**Status**: Accepted
**Date**: January 11, 2026
**Version**: 1.0
**Authority**: AUTHORITATIVE - All BR-AI-002 references must defer to this document
**Related BR**: BR-AI-002 (Support Multiple Analysis Types)
**Deferred To**: v2.0

---

## Context

**Business Requirement BR-AI-002** (Priority P1) specifies:
> "AIAnalysis MUST support multiple analysis types (diagnostic, predictive, prescriptive) through HolmesGPT-API."

### Current Implementation Status (v1.x)

**Feature Status**: ❌ **NOT IMPLEMENTED**

Despite the CRD including an `AnalysisTypes []string` field, the feature has never been implemented:

1. **Controller**: Never reads the `AnalysisTypes` field
2. **HAPI**: No support for multiple analysis types in API contract
3. **OpenAPI**: No `analysisTypes` field in request/response schemas
4. **Tests**: Incorrectly assume multiple types trigger multiple HAPI calls

### Discovery Timeline

**January 11, 2026**: Comprehensive triage during multi-controller migration revealed:
- Field exists in CRD schema but is completely unused
- Tests fail expecting 2 HAPI calls with `["investigation", "workflow-selection"]` but controller makes 1
- No production code depends on this feature
- Feature gap was identified in December 2025 analysis as "medium priority, unused"

**Full Analysis**: See `docs/handoff/BR_AI_002_TRIAGE_JAN11_2026.md`

---

## Decision

**Defer BR-AI-002 implementation to v2.0** and maintain single analysis type support in v1.x.

### Rationale

1. **No Production Usage**: Feature never implemented, no user dependencies
2. **Design Ambiguity**: Original BR intent (HAPI-driven) conflicts with CRD schema (controller-driven)
3. **Migration Priority**: Multi-controller migration provides higher immediate value
4. **Unclear Requirements**: "diagnostic/predictive/prescriptive" categories don't match current HAPI endpoints
5. **Significant Scope**: Implementation requires 2-4 days across controller + HAPI + 400+ tests

---

## v1.x Behavior (Current & Maintained)

### Supported Pattern

**Single Analysis Type Per Request**:
```go
spec:
  analysisRequest:
    analysisTypes: ["investigation"]  // Single type only
```

**Controller Behavior**:
- Makes **exactly 1 HAPI call** per reconciliation
- Ignores additional values in `AnalysisTypes` array
- Returns single analysis result

**HAPI Endpoints**:
| Endpoint | Purpose | Analysis Type |
|---|---|---|
| `/api/v1/incident/analyze` | Initial RCA + workflow selection | Investigation |
| `/api/v1/recovery/analyze` | Post-failure recovery strategy | Recovery |
| `/api/v1/postexec/analyze` | Post-execution effectiveness | Post-execution |

### Field Status

**`spec.analysisRequest.analysisTypes`**:
- ✅ **Exists** in CRD schema
- ✅ **Required** (`minItems: 1`)
- ⚠️ **Functional** for single value only
- ❌ **Multiple values ignored** by controller

**Recommendation**: Use single-value arrays until v2.0

---

## v1.x Test Changes (Required)

### Integration Test Pattern

**Before (Incorrect)**:
```go
AnalysisTypes: []string{"investigation", "workflow-selection"},  // ❌ Expects 2 calls

Expect(eventTypeCounts[aiaudit.EventTypeHolmesGPTCall]).To(Equal(2))  // ❌ Fails
```

**After (Correct)**:
```go
AnalysisTypes: []string{"investigation"},  // ✅ Single type

Expect(eventTypeCounts[aiaudit.EventTypeHolmesGPTCall]).To(Equal(1))  // ✅ Passes
```

### Files Requiring Updates

**Critical** (Blocking test failures):
- `test/integration/aianalysis/audit_flow_integration_test.go`
- `test/integration/aianalysis/audit_provider_data_integration_test.go`

**Non-Critical** (Best practice):
- `test/integration/aianalysis/metrics_integration_test.go`
- All other integration/e2e tests using multiple types

**Total**: 90+ occurrences across test files

---

## v2.0 Implementation Scope (Deferred)

### Design Options for v2.0

#### Option 1: Controller-Driven Multiple Calls
**Model**: Loop through `AnalysisTypes`, make multiple HAPI calls
```go
for _, analysisType := range analysis.Spec.AnalysisRequest.AnalysisTypes {
    // Call HAPI with type-specific request
    // Emit separate audit event
    // Aggregate results
}
```

**Pros**: Aligns with current CRD schema
**Cons**: Requires HAPI refactoring, unclear business value

#### Option 2: HAPI-Driven Categorization (Original Intent)
**Model**: HAPI determines analysis type, returns in response
```yaml
status:
  analysisType: diagnostic  # or predictive, prescriptive
```

**Pros**: Matches original BR-AI-002 intent, simpler controller
**Cons**: Requires new HAPI logic, may not match real use cases

#### Option 3: Remove Feature Entirely
**Model**: Delete `analysisTypes` field, bump CRD version
```yaml
spec:
  analysisRequest:
    # AnalysisTypes removed - endpoints determine type
```

**Pros**: Eliminates confusion, aligns with reality
**Cons**: Breaking API change

### v2.0 Requirements (To Be Determined)

**Business Validation Needed**:
1. Are "diagnostic/predictive/prescriptive" still relevant categories?
2. What real-world scenario requires multiple analysis types?
3. Should analysis type be:
   - User-specified (controller-driven)?
   - AI-determined (HAPI-driven)?
   - Endpoint-implicit (remove field)?

**Technical Requirements**:
- OpenAPI contract update
- HAPI endpoint changes or new logic
- Controller loop implementation (if Option 1)
- Status field additions
- Migration guide for v1.x → v2.0

---

## Migration Guidance (v1.x Users)

### For Test Authors

**Pattern**:
```go
// ✅ CORRECT: Use single analysis type
spec:
  analysisRequest:
    analysisTypes: ["investigation"]

// ❌ INCORRECT: Multiple types (ignored by controller)
spec:
  analysisRequest:
    analysisTypes: ["investigation", "workflow-selection"]
```

### For Production Users

**Current Behavior**:
- Specify `["investigation"]` for incident analysis
- Specify `["recovery"]` for recovery analysis (if using recovery endpoints)
- Multiple values in array are ignored - controller uses endpoint logic

**No Breaking Changes**: v1.x will maintain current single-type behavior

---

## Documentation Updates Required

### Business Requirements

**File**: `docs/services/crd-controllers/02-aianalysis/BUSINESS_REQUIREMENTS.md`

**Update Section**: BR-AI-002 (lines 63-85)
```markdown
#### BR-AI-002: Support Multiple Analysis Types

**Status**: ⏸️ **DEFERRED TO v2.0**
**Authority**: See DD-AIANALYSIS-005

**v1.x Behavior**: Single analysis type supported per request.
Multiple values in `AnalysisTypes` array are ignored.

**Deferred**: Full multiple analysis types feature deferred to v2.0
pending business requirement validation.
```

### BR Mapping

**File**: `docs/services/crd-controllers/02-aianalysis/BR_MAPPING.md`

**Update**:
```markdown
| **BR-AI-002** | Support multiple analysis types | ⏸️ Deferred v2.0 | Single type only (DD-AIANALYSIS-005) |
```

### Implementation Plans

**Files**:
- `docs/services/crd-controllers/02-aianalysis/IMPLEMENTATION_PLAN_V1.0.md`
- `docs/requirements/02_AI_MACHINE_LEARNING.md`

**Add Section**:
```markdown
## Deferred Features

### BR-AI-002: Multiple Analysis Types
**Deferred To**: v2.0
**Reason**: Feature never implemented, unclear business value
**Authority**: DD-AIANALYSIS-005
```

### Gap Analysis Documents

**Files**:
- `docs/handoff/AA_V1_0_GAPS_RESOLUTION_DEC_20_2025.md`
- `docs/handoff/AA_V1_0_COMPLIANCE_TRIAGE_DEC_20_2025.md`
- `docs/handoff/AA_INTEGRATION_TEST_EDGE_CASE_TRIAGE.md`

**Update Status**: Mark BR-AI-002 as "Deferred v2.0" with reference to DD-AIANALYSIS-005

---

## Test Validation

### Acceptance Criteria (v1.x)

All AIAnalysis integration tests MUST:
- ✅ Use single value in `AnalysisTypes` array
- ✅ Expect exactly 1 HolmesGPT API call per reconciliation
- ✅ Pass with `--fail-fast` flag enabled
- ✅ Complete in <3 minutes (multi-controller parallel execution)

### v1.x Test Coverage

**Maintained**:
- ✅ Single incident analysis
- ✅ Single recovery analysis
- ✅ Workflow selection from single analysis
- ✅ Audit trail for single analysis

**Deferred to v2.0**:
- ❌ Multiple analysis types in single request
- ❌ Aggregated results from multiple analyses
- ❌ Diagnostic vs predictive vs prescriptive categorization

---

## Related Documents

**Primary References**:
- `docs/handoff/BR_AI_002_TRIAGE_JAN11_2026.md` - Full gap analysis
- `docs/services/crd-controllers/02-aianalysis/BUSINESS_REQUIREMENTS.md` - BR-AI-002 definition
- `docs/services/crd-controllers/02-aianalysis/crd-schema.md` - CRD field documentation

**Related Decisions**:
- `DD-AIANALYSIS-001` - Rego policy loading strategy
- `DD-AIANALYSIS-002` - Rego policy startup validation
- `DD-CONTRACT-001` - AIAnalysis-WorkflowExecution alignment
- `DD-HAPI-003` - Confidence scoring

**Test Standards**:
- `DD-TEST-002` - Parallel test execution standard
- `DD-TEST-010` - Controller-per-process architecture

---

## Change History

| Version | Date | Author | Changes |
|---|---|---|---|
| 1.0 | 2026-01-11 | System | Initial decision - defer BR-AI-002 to v2.0 |

---

## Questions for v2.0 Planning

1. **Business Need**: What production scenario requires multiple analysis types?
2. **User Experience**: Should users specify types or let AI determine them?
3. **API Design**: Multiple calls vs single call with multiple results?
4. **Categorization**: Are diagnostic/predictive/prescriptive still relevant?
5. **Priority**: Is this still P1 or should it be downgraded?

**Decision Authority**: Product team + AI/ML architects

---

## Summary

**v1.x Reality**:
- ✅ Single analysis type per request (working)
- ❌ Multiple analysis types (not implemented, deferred)

**v2.0 Roadmap**:
- Validate business requirements
- Choose design pattern (Option 1, 2, or 3)
- Implement if business value confirmed
- Update OpenAPI contracts
- Migrate tests and documentation

**Immediate Action**: Fix v1.x tests to match current single-type behavior (30 minutes)


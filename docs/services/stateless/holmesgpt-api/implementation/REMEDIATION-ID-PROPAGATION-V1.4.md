# Remediation ID Propagation - Implementation Plan

**Version**: 1.4
**Filename**: `REMEDIATION-ID-PROPAGATION-IMPLEMENTATION_PLAN_V1.4.md`
**Implements**: DD-WORKFLOW-002 (MCP Workflow Catalog Architecture)
**Status**: 📋 DRAFT
**Design Decision**: [DD-WORKFLOW-002 v2.3](../../../architecture/decisions/DD-WORKFLOW-002-MCP-WORKFLOW-CATALOG-ARCHITECTURE.md)
**Service**: Cross-Service (AIAnalysis Controller, HolmesGPT API, Data Storage Service)
**Confidence**: 95% (Evidence-Based)
**Estimated Effort**: 1.5 days (APDC cycle: 1 day implementation + 0.25 days testing + 0.25 days documentation)

⚠️ **CRITICAL**: Filename version MUST match document version at all times.
- Document v1.3 → Filename `V1.3.md`

---

## 🚨 **CRITICAL: Read This First**

**Before starting implementation, you MUST review these 5 critical pitfalls** (see line 954 for full details):

1. **Insufficient TDD Discipline** → Write ONE test at a time (not batched)
2. **Missing Integration Tests** → Integration tests BEFORE E2E tests
3. **Critical Infrastructure Without Unit Tests** → ≥70% coverage for critical components
4. **Late E2E Discovery** → Follow test pyramid (Unit → Integration → E2E)
5. **No Test Coverage Gates** → Automated CI/CD coverage gates

⚠️ **These pitfalls caused production issues in Audit Implementation (DD-STORAGE-012).**

---

## 📋 **Version History**

| Version | Date | Changes | Status |
|---------|------|---------|--------|
| **v1.0** | 2025-11-27 | Initial implementation plan for remediation_id propagation across services | ⏸️ Superseded |
| **v1.1** | 2025-11-27 | **CRITICAL**: Fixed test file location guidance (removed `pkg/` exception - all tests in `test/` directory). Fixed package naming (white-box testing without `_test` suffix). Fixed Go test example to use Ginkgo/Gomega BDD framework instead of standard Go testing. | ⏸️ Superseded |
| **v1.2** | 2025-11-27 | **ENHANCEMENT**: Added missing operational sections (Operational Considerations, CI/CD Integration) per FEATURE_EXTENSION_PLAN_TEMPLATE.md. Added applicability assessment for monitoring, security, error handling. Added GitHub Actions configuration and manual validation commands. Plan now 100% template-compliant. | ⏸️ Superseded |
| **v1.3** | 2025-11-27 | **CRITICAL CLARIFICATION**: Added explicit usage constraint for `remediation_id` - field is MANDATORY but for CORRELATION/AUDIT ONLY. Updated to clarify LLM must NOT use remediation_id for RCA analysis or workflow matching. Added usage constraint table and LLM instruction section. Updated reference to DD-WORKFLOW-002 v2.2. | ⏸️ Superseded |
| **v1.4** | 2025-11-27 | **API DESIGN**: Confirmed `remediation_id` transport via JSON body (not HTTP header). Updated DD references to v2.3. Added Go model update for `WorkflowSearchRequest.RemediationID`. Aligned with DD-WORKFLOW-014 v2.1 transport decision. | ✅ **CURRENT** |

---

## 🎯 **Business Requirements**

> **Authority**: [BR-AUDIT-021-030-WORKFLOW-SELECTION-AUDIT-TRAIL.md](../../../requirements/BR-AUDIT-021-030-WORKFLOW-SELECTION-AUDIT-TRAIL.md)

### **Primary Business Requirements**

| BR ID | Description | Success Criteria |
|-------|-------------|------------------|
| **BR-AUDIT-001** | Unified audit trail for all business operations | All workflow search events contain remediation_id for correlation |
| **BR-AUDIT-021** | Mandatory remediation_id propagation from HolmesGPT API | remediation_id in JSON body of every search request |
| **BR-AUDIT-022** | No audit generation in HolmesGPT API | Only one HTTP call per search (no audit call) |
| **BR-AUDIT-023** | Audit event generation in Data Storage Service | Every search generates audit event |
| **BR-INTEGRATION-003** | Cross-service correlation for debugging and compliance | Audit events can be queried by remediation_id across all services |
| **BR-STORAGE-013** | Workflow catalog semantic search with audit trail | Workflow searches generate audit events with correlation_id |

### **⚠️ CRITICAL: remediation_id Usage Constraint**

**Purpose**: `remediation_id` is a **CORRELATION/AUDIT FIELD ONLY**.

| Aspect | Requirement |
|--------|-------------|
| **Mandatory** | ✅ YES - Required for all workflow search requests |
| **Used in RCA** | ❌ NO - Do NOT use for root cause analysis |
| **Used in Search** | ❌ NO - Do NOT use for workflow matching or ranking |
| **Purpose** | Pass-through for audit trail correlation per ADR-034 |
| **Source** | Provided by AIAnalysis controller from `kubernaut.io/correlation-id` label |
| **Destination** | Stored in `audit_events.correlation_id` column |

**LLM Instruction**: The `remediation_id` field is mandatory but is purely for audit/traceability purposes. The LLM should:
1. ✅ **Receive** `remediation_id` from the investigation context
2. ✅ **Propagate** `remediation_id` to the `search_workflow_catalog` tool
3. ❌ **NOT interpret** `remediation_id` for analysis
4. ❌ **NOT use** `remediation_id` in search queries or workflow selection

### **Success Metrics**

**Format**: `[Metric]: [Target] - *Justification: [Why this target?]*`

**Your Metrics**:
- **Audit Event Correlation Rate**: 100% - *Justification: Every workflow search must be traceable to its remediation request for compliance and debugging*
- **remediation_id Validation Success**: 100% - *Justification: Missing remediation_id should be caught at API boundary, not in downstream services*
- **End-to-End Trace Completeness**: 100% - *Justification: Complete audit trail from AIAnalysis → HolmesGPT API → Workflow Catalog → Database*

---

## 📅 **Timeline Overview**

### **Phase Breakdown**

| Phase | Duration | Days | Purpose | Key Deliverables |
|-------|----------|------|---------|------------------|
| **ANALYSIS** | 1 hour | Day 0 (pre-work) | Comprehensive context understanding | Analysis document, integration point mapping, existing code review |
| **PLAN** | 1 hour | Day 0 (pre-work) | Detailed implementation strategy | This document, TDD phase mapping, success criteria |
| **DO (Implementation)** | 1 day | Day 1 | Controlled TDD execution | Core feature logic, integration |
| **CHECK (Testing)** | 0.25 days | Day 1 PM | Comprehensive result validation | Test suite (unit/integration), BR validation |
| **PRODUCTION READINESS** | 0.25 days | Day 2 AM | Documentation & deployment prep | Updated docs, confidence report |

### **1.5-Day Implementation Timeline**

| Day | Phase | Focus | Hours | Key Milestones |
|-----|-------|-------|-------|----------------|
| **Day 0** | ANALYSIS + PLAN | Pre-work | 2h | ✅ Analysis complete, Plan approved (this document) |
| **Day 1 AM** | DO-RED | Foundation + Tests | 4h | Test framework, failing tests for all services |
| **Day 1 PM** | DO-GREEN | Core logic | 4h | remediation_id extraction, validation, propagation |
| **Day 1 EOD** | DO-REFACTOR | Integration | 2h | Logging, error handling, validation |
| **Day 2 AM** | CHECK | Integration tests | 2h | End-to-end flow validation, manual testing |
| **Day 2 PM** | PRODUCTION | Documentation | 2h | Update DDs, API docs, handoff summary |

### **Critical Path Dependencies**

```
Day 0 (Analysis + Plan) → Day 1 AM (RED Phase) → Day 1 PM (GREEN Phase)
                                                   ↓
Day 1 EOD (REFACTOR) → Day 2 AM (CHECK) → Day 2 PM (PRODUCTION)
```

### **Daily Progress Tracking**

**EOD Documentation Required**:
- **Day 1 AM Complete**: RED phase checkpoint (all tests failing)
- **Day 1 PM Complete**: GREEN phase checkpoint (all tests passing)
- **Day 1 EOD Complete**: REFACTOR phase checkpoint (enhanced error handling)
- **Day 2 AM Complete**: CHECK phase checkpoint (integration tests passing)
- **Day 2 PM Complete**: Production ready checkpoint (documentation complete)

---

## 📆 **Day-by-Day Implementation Breakdown**

### **Day 0: ANALYSIS + PLAN (Pre-Work) ✅**

**Phase**: ANALYSIS + PLAN
**Duration**: 2 hours
**Status**: ✅ COMPLETE (this document represents Day 0 completion)

**Deliverables**:
- ✅ Analysis document: Integration points identified (AIAnalysis Controller, HolmesGPT API, Workflow Catalog Tool)
- ✅ Implementation plan (this document v1.0): 1.5-day timeline, test examples
- ✅ Risk assessment: 2 low-risk areas identified (Data Storage already supports correlation_id, Workflow Catalog Tool already accepts remediation_id)
- ✅ Existing code review:
  - `internal/controller/aianalysis/aianalysis_controller.go` (AIAnalysis label extraction)
  - `holmesgpt-api/src/models/incident_models.py` (request models)
  - `holmesgpt-api/src/extensions/incident.py` (toolset instantiation)
  - `holmesgpt-api/src/toolsets/workflow_catalog.py` (audit event generation)
- ✅ BR coverage matrix: 3 primary BRs mapped to test scenarios

---

### **Day 1 AM: Foundation + Test Framework (DO-RED Phase)**

**Phase**: DO-RED
**Duration**: 4 hours
**TDD Focus**: Write failing tests first, enhance existing code

**⚠️ CRITICAL**: We are **ENHANCING existing code**, not creating from scratch!

**Existing Code to Enhance**:
- ✅ `internal/controller/aianalysis/aianalysis_controller.go` (200 LOC) - AIAnalysis reconciliation
- ✅ `holmesgpt-api/src/models/incident_models.py` (150 LOC) - Request validation
- ✅ `holmesgpt-api/src/extensions/incident.py` (300 LOC) - Investigation orchestration
- ✅ `holmesgpt-api/src/toolsets/workflow_catalog.py` (400 LOC) - Workflow search + audit

**Morning (2 hours): Test Framework Setup + Code Analysis**

1. **Analyze existing implementation** (30 minutes)
   - Read `internal/controller/aianalysis/aianalysis_controller.go` - understand label extraction
   - Read `holmesgpt-api/src/models/incident_models.py` - understand request validation
   - Read `holmesgpt-api/src/extensions/incident.py` - understand toolset instantiation
   - Read `holmesgpt-api/src/toolsets/workflow_catalog.py` - understand audit event generation
   - Identify what needs to be enhanced vs. created new

2. **Create Go test file** `test/unit/controller/aianalysis_remediation_id_test.go` (150 LOC) (30 minutes)
   - Set up Go test suite with testify
   - Define test fixtures for AIAnalysis with remediation_id label
   - Create mock HolmesGPT API server for capturing requests

3. **Create Python test files** (1 hour)
   - `holmesgpt-api/tests/unit/test_incident_models_remediation_id.py` (100 LOC)
     - Set up pytest with Pydantic validation tests
     - Define test fixtures for request models
   - `holmesgpt-api/tests/unit/test_workflow_catalog_remediation_id.py` (150 LOC)
     - Set up pytest with mock requests
     - Define test fixtures for audit event validation

**⚠️ CRITICAL: TEST FILE LOCATIONS - MANDATORY**

**AUTHORITY**: [03-testing-strategy.mdc](../../../../.cursor/rules/03-testing-strategy.mdc)

**ALL tests MUST be in the following locations**:
- ✅ **Go Unit Tests**: `test/unit/controller/aianalysis_remediation_id_test.go`
- ✅ **Python Unit Tests**: `holmesgpt-api/tests/unit/test_incident_models_remediation_id.py`
- ✅ **Python Unit Tests**: `holmesgpt-api/tests/unit/test_workflow_catalog_remediation_id.py`
- ✅ **Python Integration Tests**: `holmesgpt-api/tests/integration/test_remediation_id_e2e.py`

**❌ NEVER place tests**:
- ❌ Co-located with source files (e.g., `internal/controller/aianalysis/aianalysis_test.go`)
- ❌ In `pkg/[package]/[feature]_test.go` (all tests must be in `test/` directory)

**Package Naming**: Tests use white-box testing (same package as code under test, without `_test` suffix).

---

**Afternoon (2 hours): Failing Tests Implementation**

4. **Write failing Go tests** (1 hour)
   - **TDD Cycle 1**: Test AIAnalysis controller extracts remediation_id from label
   - **TDD Cycle 2**: Test AIAnalysis controller passes remediation_id to HolmesGPT API
   - **TDD Cycle 3**: Test AIAnalysis controller returns error for missing remediation_id
   - Run tests → Verify they FAIL (RED phase)

5. **Write failing Python tests** (1 hour)
   - **TDD Cycle 4**: Test IncidentAnalysisRequest requires remediation_id field
   - **TDD Cycle 5**: Test IncidentAnalysisRequest rejects empty remediation_id
   - **TDD Cycle 6**: Test SearchWorkflowCatalogTool accepts remediation_id in constructor
   - **TDD Cycle 7**: Test SearchWorkflowCatalogTool sends audit event with correlation_id
   - Run tests → Verify they FAIL (RED phase)

**EOD Deliverables**:
- ✅ Test framework complete (3 test files)
- ✅ 7 failing tests (RED phase)
- ✅ No implementation changes yet
- ✅ Day 1 AM EOD report

**Validation Commands**:
```bash
# Verify Go tests fail (RED phase)
go test ./test/unit/controller/aianalysis_remediation_id_test.go -v 2>&1 | grep "FAIL"

# Verify Python tests fail (RED phase)
cd holmesgpt-api
pytest tests/unit/test_incident_models_remediation_id.py -v 2>&1 | grep "FAILED"
pytest tests/unit/test_workflow_catalog_remediation_id.py -v 2>&1 | grep "FAILED"

# Expected: All tests should FAIL with "not implemented yet" or validation errors
```

---

### **Day 1 PM: Core Logic (DO-GREEN Phase)**

**Phase**: DO-GREEN
**Duration**: 4 hours
**TDD Focus**: Minimal implementation to pass tests

**Morning (2 hours): AIAnalysis Controller + Request Models**

1. **Implement AIAnalysis Controller** `internal/controller/aianalysis/aianalysis_controller.go` (30 minutes)
   - Extract remediation_id from `aiAnalysis.Labels["kubernaut.io/correlation-id"]`
   - Add validation for missing remediation_id
   - Pass remediation_id in HTTP request payload to HolmesGPT API
   - Run Go tests → Verify they PASS (GREEN phase)

2. **Implement Request Models** `holmesgpt-api/src/models/incident_models.py` (30 minutes)
   - Add `remediation_id: str` field with Pydantic Field(...) (required)
   - Add validator to reject empty remediation_id
   - Run Python tests → Verify they PASS (GREEN phase)

3. **Implement Request Models** `holmesgpt-api/src/models/recovery_models.py` (30 minutes)
   - Add `remediation_id: str` field with Pydantic Field(...) (required)
   - Add validator to reject empty remediation_id

4. **Verify all model tests pass** (30 minutes)
   - Run all Python model tests
   - Verify validation logic works correctly

**Afternoon (2 hours): Extensions + Workflow Catalog Tool**

5. **Implement Extensions** `holmesgpt-api/src/extensions/incident.py` (45 minutes)
   - Extract remediation_id from request
   - Pass remediation_id to WorkflowCatalogToolset constructor
   - Add error handling for missing remediation_id

6. **Implement Extensions** `holmesgpt-api/src/extensions/recovery.py` (45 minutes)
   - Extract remediation_id from request
   - Pass remediation_id to WorkflowCatalogToolset constructor

7. **Verify Workflow Catalog Tool** `holmesgpt-api/src/toolsets/workflow_catalog.py` (30 minutes)
   - Confirm `__init__` accepts remediation_id parameter (already implemented)
   - Confirm `_send_audit_event` uses remediation_id as correlation_id (already implemented)
   - Run Python tests → Verify they PASS (GREEN phase)

**EOD Deliverables**:
- ✅ Core methods implemented (GREEN phase)
- ✅ All 7 unit tests passing
- ✅ Basic functionality working
- ✅ Day 1 PM EOD report

**Validation Commands**:
```bash
# Verify Go tests pass (GREEN phase)
go test ./test/unit/controller/aianalysis_remediation_id_test.go -v

# Verify Python tests pass (GREEN phase)
cd holmesgpt-api
pytest tests/unit/test_incident_models_remediation_id.py -v
pytest tests/unit/test_workflow_catalog_remediation_id.py -v

# Expected: All tests should PASS
```

---

### **Day 1 EOD: Integration + Refactor (DO-REFACTOR Phase)**

**Phase**: DO-REFACTOR
**Duration**: 2 hours
**TDD Focus**: Enhance error handling, logging, validation

**Evening (2 hours): Refactor + Integration**

1. **Enhance AIAnalysis Controller** (30 minutes)
   - Add detailed logging for remediation_id extraction
   - Add structured error messages for missing label
   - Add BR-AUDIT-001 reference in comments

2. **Enhance Request Models** (30 minutes)
   - Add detailed error messages for validation failures
   - Add BR-AUDIT-001 reference in docstrings
   - Add example values in Field descriptions

3. **Enhance Workflow Catalog Tool** (30 minutes)
   - Add warning log if remediation_id is None
   - Add BR-AUDIT-001 reference in audit event generation
   - Add detailed logging for audit event success/failure

4. **Create Integration Test Helpers** (30 minutes)
   - `holmesgpt-api/tests/integration/helpers/audit_helpers.py`
   - Add `get_audit_events_by_correlation_id()` helper
   - Add `assert_workflow_search_audit_event_exists()` helper

**EOD Deliverables**:
- ✅ Enhanced error handling and logging
- ✅ Integration test helpers created
- ✅ All tests still passing
- ✅ Code quality improved
- ✅ Day 1 EOD report

**Validation Commands**:
```bash
# Verify all tests still pass after refactoring
go test ./test/unit/controller/aianalysis_remediation_id_test.go -v
cd holmesgpt-api
pytest tests/unit/test_incident_models_remediation_id.py -v
pytest tests/unit/test_workflow_catalog_remediation_id.py -v

# Expected: All tests should still PASS
```

---

### **Day 2 AM: Integration Tests (CHECK Phase)**

**Phase**: CHECK
**Duration**: 2 hours
**Focus**: End-to-end flow validation

**Morning (2 hours): Integration Test Implementation**

1. **Create Integration Test** `holmesgpt-api/tests/integration/test_remediation_id_e2e.py` (1 hour)
   - Test end-to-end remediation_id flow (AIAnalysis → HolmesGPT API → Audit Trail)
   - Test missing remediation_id returns 400 error
   - Test multiple workflow searches share same remediation_id
   - Run integration tests → Verify they PASS

2. **Manual Validation** (1 hour)
   - Create test AIAnalysis with remediation_id label
   - Verify audit event in PostgreSQL database
   - Query audit events by correlation_id
   - Verify complete audit trail exists

**EOD Deliverables**:
- ✅ 3 integration test scenarios
- ✅ All integration tests passing
- ✅ Manual validation complete
- ✅ End-to-end flow verified
- ✅ Day 2 AM EOD report

**Validation Commands**:
```bash
# Run integration tests
cd holmesgpt-api
pytest tests/integration/test_remediation_id_e2e.py -v

# Manual database validation
kubectl exec -it postgresql-0 -n data-storage -- psql -U kubernaut -d kubernaut -c \
  "SELECT event_type, correlation_id, event_data->>'query' FROM audit_events WHERE correlation_id = 'req-2025-11-27-test';"

# Expected: Integration tests pass, database contains audit events with correct correlation_id
```

---

### **Day 2 PM: Documentation (PRODUCTION Phase)**

**Phase**: PRODUCTION
**Duration**: 2 hours
**Focus**: Finalize documentation and knowledge transfer

**📝 Note**: Most documentation is created **DURING** implementation (Days 1-2 AM). This time is for **finalizing** and **consolidating** documentation.

---

#### **Afternoon (2 hours): Finalize Documentation**

**What to Update** (these are existing service docs, not new files):

1. **Update DD-WORKFLOW-002** (30 minutes) - ✅ ALREADY DONE
   - Version bumped to 2.1
   - Added remediation_id parameter to MCP tool contract
   - Added changelog entry
   - Cross-referenced ADR-034 and DD-WORKFLOW-014

2. **Update ADR-034** (30 minutes) - ✅ ALREADY DONE
   - Version bumped to 1.1
   - Added Workflow Catalog Service audit events
   - Added Phase 3 implementation details

3. **Update API Documentation** (30 minutes)
   - `holmesgpt-api/docs/API.md` - Add remediation_id parameter to incident/recovery endpoints
   - Add example requests with remediation_id
   - Add error response examples for missing remediation_id

4. **Create Handoff Summary** (30 minutes)
   - Executive summary of changes
   - Files modified (5 Go files, 5 Python files)
   - Test coverage (7 unit tests, 3 integration tests)
   - Confidence assessment (95%)
   - Known limitations (none)
   - Future work (none required)

**EOD Deliverables**:
- ✅ DD-WORKFLOW-002 updated to v2.1
- ✅ ADR-034 updated to v1.1
- ✅ API documentation updated
- ✅ Handoff summary created
- ✅ All inline code documentation complete
- ✅ Day 2 PM EOD report

---

#### **Documentation Checklist**

**Design Decisions** (updates to existing files):
- [x] DD-WORKFLOW-002 - Version bumped to 2.1, remediation_id added
- [x] ADR-034 - Version bumped to 1.1, Workflow Catalog audit events added
- [ ] API documentation - remediation_id parameter documented

**Code Documentation** (inline, created during implementation):
- [ ] GoDoc comments for AIAnalysis controller changes
- [ ] BR references in code comments (BR-AUDIT-001, BR-INTEGRATION-003)
- [ ] Pydantic Field descriptions for remediation_id
- [ ] Audit event generation comments

**Test Documentation** (inline, created during testing):
- [ ] Test descriptions with business scenarios
- [ ] BR mapping in test comments
- [ ] Integration test helpers documented

**Handoff Documentation** (new file):
- [ ] Executive summary
- [ ] Files modified summary
- [ ] Test coverage summary
- [ ] Confidence assessment
- [ ] Known limitations
- [ ] Future work

---

## 🧪 **TDD Do's and Don'ts - MANDATORY**

### **✅ DO: Strict TDD Discipline**

1. **Write ONE test at a time** (not batched)
   ```python
   # ✅ CORRECT: TDD Cycle 1
   def test_incident_analysis_request_requires_remediation_id():
       # Test for remediation_id validation
       pass
   # Run test → FAIL (RED)
   # Implement validation → PASS (GREEN)
   # Refactor if needed

   # ✅ CORRECT: TDD Cycle 2 (after Cycle 1 complete)
   def test_incident_analysis_request_rejects_empty_remediation_id():
       # Test for empty remediation_id
       pass
   ```

2. **Test WHAT the system does** (behavior), not HOW (implementation)
   ```python
   # ✅ CORRECT: Behavior-focused
   def test_workflow_catalog_tool_sends_audit_event_with_correlation_id():
       # BUSINESS SCENARIO: Workflow search generates audit event
       tool = SearchWorkflowCatalogTool(remediation_id="req-123")
       tool._invoke(params={"query": "OOMKilled critical"})

       # BEHAVIOR: Audit event sent with correlation_id
       assert audit_event['correlation_id'] == "req-123"
   ```

3. **Use specific assertions** (not weak checks)
   ```python
   # ✅ CORRECT: Specific business assertions
   assert request.remediation_id == "req-2025-11-27-abc123"
   assert audit_event['event_type'] == "workflow.catalog.search_completed"
   assert audit_event['correlation_id'] == remediation_id
   ```

### **❌ DON'T: Anti-Patterns to Avoid**

1. **DON'T batch test writing**
   ```python
   # ❌ WRONG: Writing 7 tests before any implementation
   def test_1(): pass
   def test_2(): pass
   def test_3(): pass
   # ... 4 more tests
   # Then implementing all at once
   ```

2. **DON'T test implementation details**
   ```python
   # ❌ WRONG: Testing internal state
   assert tool._remediation_id == "req-123"  # Internal field
   assert len(tool._audit_events) == 1       # Internal buffer
   ```

3. **DON'T use weak assertions (NULL-TESTING)**
   ```python
   # ❌ WRONG: Weak assertions
   assert request.remediation_id is not None
   assert len(audit_events) > 0
   assert audit_event.get('correlation_id')
   ```

**Reference**: `.cursor/rules/08-testing-anti-patterns.mdc` for automated detection

---

## 📊 **Test Examples**

### **📁 Test File Locations - MANDATORY (READ THIS FIRST)**

**AUTHORITY**: [03-testing-strategy.mdc](../../../../.cursor/rules/03-testing-strategy.mdc)

**🚨 CRITICAL**: Test files MUST be in specific directories, NOT co-located with source code!

| Test Type | File Location | Example | ❌ WRONG Location |
|-----------|---------------|---------|-------------------|
| **Go Unit Tests** | `test/unit/controller/` | `test/unit/controller/aianalysis_remediation_id_test.go` | ❌ `internal/controller/aianalysis/aianalysis_test.go` |
| **Python Unit Tests** | `holmesgpt-api/tests/unit/` | `holmesgpt-api/tests/unit/test_incident_models_remediation_id.py` | ❌ `holmesgpt-api/src/models/incident_models_test.py` |
| **Python Integration Tests** | `holmesgpt-api/tests/integration/` | `holmesgpt-api/tests/integration/test_remediation_id_e2e.py` | ❌ `holmesgpt-api/src/extensions/test_integration.py` |

---

### **Go Unit Test Example**

```go
package controller  // White-box testing - same package as code under test

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    kubernautv1 "github.com/jordigilh/kubernaut/api/v1"
)

// BR-AUDIT-001: Unified audit trail for all business operations
// BR-INTEGRATION-003: Cross-service correlation for debugging and compliance
// Design Decision: DD-WORKFLOW-002 v2.1, DD-WORKFLOW-014
var _ = Describe("AIAnalysis Controller Remediation ID Propagation", func() {
    var (
        mockServer      *httptest.Server
        receivedRequest map[string]interface{}
        ctx             context.Context
    )

    BeforeEach(func() {
        ctx = context.Background()
        receivedRequest = make(map[string]interface{})

        // Create mock HolmesGPT API server
        mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            json.NewDecoder(r.Body).Decode(&receivedRequest)
            w.WriteHeader(http.StatusOK)
            json.NewEncoder(w).Encode(map[string]interface{}{
                "status": "success",
            })
        }))
    })

    AfterEach(func() {
        mockServer.Close()
    })

    Context("when AIAnalysis has remediation_id label", func() {
        It("should extract remediation_id and pass to HolmesGPT API", func() {
            // ARRANGE: Create AIAnalysis with remediation_id label
            aiAnalysis := &kubernautv1.AIAnalysis{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-analysis",
                    Namespace: "default",
                    Labels: map[string]string{
                        "kubernaut.io/correlation-id": "req-2025-11-27-abc123",
                    },
                },
            }

            // ACT: Trigger controller reconciliation
            controller := NewController(mockServer.URL)
            err := controller.Reconcile(ctx, aiAnalysis)

            // ASSERT: Controller should pass remediation_id to HolmesGPT API
            Expect(err).ToNot(HaveOccurred())
            Expect(receivedRequest["remediation_id"]).To(Equal("req-2025-11-27-abc123"))
        })
    })

    Context("when AIAnalysis missing remediation_id label", func() {
        It("should return error for missing correlation-id label", func() {
            // ARRANGE: Create AIAnalysis WITHOUT remediation_id label
            aiAnalysis := &kubernautv1.AIAnalysis{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-analysis",
                    Namespace: "default",
                    Labels:    map[string]string{}, // Missing correlation-id
                },
            }

            // ACT: Trigger controller reconciliation
            controller := NewController(mockServer.URL)
            err := controller.Reconcile(ctx, aiAnalysis)

            // ASSERT: Should return error for missing remediation_id
            Expect(err).To(HaveOccurred())
            Expect(err.Error()).To(ContainSubstring("correlation-id"))
        })
    })
})
```

### **Python Unit Test Example**

```python
"""
Unit tests for remediation_id validation in incident analysis requests

Business Requirement: BR-AUDIT-001, BR-INTEGRATION-003
Design Decision: DD-WORKFLOW-002 v2.1, DD-WORKFLOW-014
"""

import pytest
from pydantic import ValidationError
from holmesgpt_api.models.incident_models import IncidentAnalysisRequest


def test_incident_analysis_request_requires_remediation_id():
    """
    Test that IncidentAnalysisRequest requires remediation_id field

    Business Requirement: BR-AUDIT-001 - Unified audit trail
    Design Decision: DD-WORKFLOW-002 v2.1 - remediation_id mandatory
    """
    # ARRANGE: Request data WITHOUT remediation_id
    request_data = {
        "signal_type": "OOMKilled",
        "severity": "critical",
        # Missing: remediation_id
    }

    # ACT & ASSERT: Should raise ValidationError
    with pytest.raises(ValidationError) as exc_info:
        IncidentAnalysisRequest(**request_data)

    assert "remediation_id" in str(exc_info.value).lower()
```

### **Python Integration Test Example**

```python
"""
End-to-end integration test for remediation_id propagation

Business Requirement: BR-AUDIT-001, BR-INTEGRATION-003
Design Decision: DD-WORKFLOW-002 v2.1, DD-WORKFLOW-014
"""

import pytest
import requests
import uuid


@pytest.mark.integration
def test_remediation_id_propagates_through_full_stack(
    data_storage_url: str,
    holmesgpt_api_url: str
):
    """
    Test that remediation_id flows from API request to audit trail

    Business Requirement: BR-AUDIT-001, BR-INTEGRATION-003
    """
    # ARRANGE: Generate unique remediation_id
    remediation_id = f"req-2025-11-27-{uuid.uuid4().hex[:8]}"

    # ACT: Send request to HolmesGPT API
    response = requests.post(
        f"{holmesgpt_api_url}/api/v1/incident/analyze",
        json={
            "remediation_id": remediation_id,
            "signal_type": "OOMKilled",
            "severity": "critical",
        }
    )

    # ASSERT: API request should succeed
    assert response.status_code == 200

    # ASSERT: Audit event should exist with correct correlation_id
    events = requests.get(
        f"{data_storage_url}/api/v1/audit/events",
        params={"correlation_id": remediation_id}
    ).json()["events"]

    assert len(events) > 0
    assert events[0]["correlation_id"] == remediation_id
```

---

## 🏗️ **Integration Test Environment Decision**

### **Environment Strategy**

**Decision**: Docker Compose (PostgreSQL + Redis + Data Storage + HolmesGPT API)

**Environment Comparison**:

| Environment | Pros | Cons | Decision |
|-------------|------|------|----------|
| **Docker Compose** | Fast, realistic, already available | Requires Docker | ✅ Selected |
| **KIND** | Full K8s, realistic | Slow startup (30s+), overkill | ❌ Not Selected |
| **Mocks** | Fastest | Not integration tests, low confidence | ❌ Not Selected |

**Rationale**: Docker Compose provides fast startup (<10s), realistic service interactions, reuses existing integration test infrastructure from workflow catalog tests, and enables true end-to-end validation without Kubernetes overhead.

---

## 🎯 **BR Coverage Matrix**

| BR ID | Description | Unit Tests | Integration Tests | Status |
|-------|-------------|------------|-------------------|--------|
| **BR-AUDIT-001** | Unified audit trail for all business operations | Go: aianalysis_remediation_id_test.go<br>Python: test_workflow_catalog_remediation_id.py | test_remediation_id_e2e.py | ✅ |
| **BR-INTEGRATION-003** | Cross-service correlation for debugging and compliance | Go: aianalysis_remediation_id_test.go<br>Python: test_incident_models_remediation_id.py | test_remediation_id_e2e.py | ✅ |
| **BR-STORAGE-013** | Workflow catalog semantic search with audit trail | Python: test_workflow_catalog_remediation_id.py | test_remediation_id_e2e.py | ✅ |

**Coverage Calculation**:
- **Unit**: 3/3 BRs covered (100%)
- **Integration**: 3/3 BRs covered (100%)
- **Total**: 3/3 BRs covered (100%)

---

## 🚨 **Critical Pitfalls to Avoid**

### **⚠️ LESSONS LEARNED FROM AUDIT IMPLEMENTATION (November 2025)**

**Context**: During the Audit Trail implementation (DD-STORAGE-012), we discovered critical mistakes that led to missing functionality (DLQ fallback) being caught only by E2E tests after the handler was considered "complete". These lessons are now mandatory to prevent in all future implementations.

**Evidence**: `docs/services/stateless/data-storage/DD-IMPLEMENTATION-AUDIT-V1.0.md`

---

### **1. Insufficient TDD Discipline** 🔴 **CRITICAL**

- ❌ **Problem**: Implementation code written before tests, leading to missing functionality
- ✅ **Solution**: Write ONE test at a time, strict RED-GREEN-REFACTOR sequence
- **Impact**: **CRITICAL** - Missing functionality caught late in development cycle

**Enforcement Checklist** (BLOCKING):
```
Before writing ANY implementation code:
- [ ] Unit test written and FAILING (RED phase)
- [ ] Test validates business behavior, not implementation
- [ ] Test uses specific assertions, not weak checks
- [ ] Test maps to specific BR-XXX requirement
- [ ] Test run confirms FAIL with expected error
```

---

### **2. Missing Integration Tests for New Endpoints** 🔴 **CRITICAL**

- ❌ **Problem**: New HTTP endpoints implemented without integration tests
- ✅ **Solution**: Integration tests MUST exist BEFORE considering endpoint complete
- **Impact**: **CRITICAL** - Critical functionality missing, caught late

**Integration Test Mandate** (BLOCKING):
```
For EVERY modified endpoint:
- [ ] Integration test MUST exist
- [ ] Integration test MUST cover success path
- [ ] Integration test MUST cover failure paths (missing remediation_id)
- [ ] Integration test validates end-to-end flow
```

---

### **3. Pydantic Model Breaking Changes** 🟡 **MEDIUM**

- ❌ **Problem**: Adding required fields to Pydantic models breaks existing API clients
- ✅ **Solution**:
  - Add remediation_id as required field (breaking change is intentional per DD-WORKFLOW-002 v2.1)
  - Document breaking change in changelog
  - Update all API clients (AIAnalysis controller) in same PR
- **Impact**: **MEDIUM** - API clients must be updated simultaneously

**Breaking Change Checklist**:
```
For breaking API changes:
- [ ] Breaking change documented in DD changelog
- [ ] All API clients updated in same PR
- [ ] Integration tests validate new contract
- [ ] Error messages guide users to fix (mention missing remediation_id)
```

---

### **4. Late E2E Discovery (Testing Tier Inversion)** 🟡 **MEDIUM**

- ❌ **Problem**: E2E tests catching unit-level issues
- ✅ **Solution**: Follow Testing Pyramid (Unit → Integration → E2E)
- **Impact**: **MEDIUM** - Slow feedback loop, expensive test failures

**Test Tier Sequence Checklist** (BLOCKING):
```
Before writing E2E tests:
- [ ] Unit tests exist for all components (7 tests)
- [ ] Integration tests exist for end-to-end flow (3 tests)
- [ ] All unit tests passing
- [ ] All integration tests passing
```

---

### **5. No Test Coverage Gates (Process Gap)** 🟡 **MEDIUM**

- ❌ **Problem**: No automated enforcement of test coverage requirements
- ✅ **Solution**: Add automated test coverage gates in CI/CD
- **Impact**: **MEDIUM** - Manual review insufficient to catch gaps

**Test Coverage Gates** (AUTOMATED):
```bash
# In CI/CD pipeline (GitHub Actions)

# Step 1: Go unit test coverage gate
- name: Check Go unit test coverage
  run: |
    go test ./test/unit/controller/... -coverprofile=coverage.out
    COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//')
    if (( $(echo "$COVERAGE < 70" | bc -l) )); then
      echo "❌ Go unit test coverage ($COVERAGE%) below 70% threshold"
      exit 1
    fi

# Step 2: Python unit test coverage gate
- name: Check Python unit test coverage
  run: |
    cd holmesgpt-api
    pytest tests/unit/ --cov=src --cov-report=term-missing --cov-fail-under=70
```

---

## 📈 **Success Criteria**

### **Technical Success**
- ✅ All tests passing (Unit: 7 tests, Integration: 3 tests)
- ✅ No lint errors
- ✅ Code integrated with main application
- ✅ Documentation complete

### **Business Success**
- ✅ BR-AUDIT-001 validated (audit events contain remediation_id)
- ✅ BR-INTEGRATION-003 validated (cross-service correlation works)
- ✅ BR-STORAGE-013 validated (workflow searches generate audit events)
- ✅ Success metrics achieved (100% correlation rate, 100% validation success)

### **Confidence Assessment**
- **Target**: ≥60% confidence
- **Actual**: 95% confidence (evidence-based)
- **Calculation**: See Confidence Calculation Methodology section

---

## 📊 **Confidence Calculation Methodology**

**Overall Confidence**: 95% (Evidence-Based)

**Component Breakdown**:

| Component | Confidence | Evidence |
|-----------|-----------|----------|
| **AIAnalysis Controller** | 95% | Simple label extraction, proven pattern in existing controller, low risk |
| **Request Models** | 98% | Pydantic validation is battle-tested, straightforward field addition |
| **Extensions** | 90% | Straightforward parameter passing, existing patterns in codebase |
| **Workflow Catalog Tool** | 98% | Already implemented in Day 1 PM of DD-WORKFLOW-014, proven working |
| **Data Storage Service** | 100% | No changes needed, already supports correlation_id in audit events |
| **Overall** | **95%** | Weighted average: (95% + 98% + 90% + 98% + 100%) / 5 |

**Risk Assessment**:
- **5% Risk**: Pydantic model changes are breaking (requires API client updates) - Mitigated by updating AIAnalysis controller in same PR
- **Assumptions**:
  - AIAnalysis CRDs have `kubernaut.io/correlation-id` label (verified in existing code)
  - Data Storage Service audit events table has `correlation_id` column (verified in ADR-034)
- **Validation Approach**: Integration tests validate end-to-end flow, manual database inspection confirms audit events

**Calculation Formula**:
```
Overall Confidence = Σ(Component Confidence × Component Weight) / Σ(Component Weight)
                   = (95 + 98 + 90 + 98 + 100) / 5
                   = 481 / 5
                   = 96.2% (rounded to 95%)
```

---

## 🔄 **Rollback Plan**

### **Rollback Triggers**
- Critical bug discovered in production (e.g., all requests fail due to missing remediation_id)
- Performance degradation >20% (unlikely, minimal overhead)
- Business requirement not met (audit events missing correlation_id)

### **Rollback Procedure**
1. **Revert Go changes**: Rollback AIAnalysis controller to not require remediation_id label
2. **Revert Python changes**: Rollback request models to make remediation_id optional
3. **Deploy previous version**: Use Kubernetes rollout undo
4. **Verify rollback success**: Check that incident analysis requests work without remediation_id
5. **Document rollback reason**: Create incident report with root cause analysis

### **Rollback Risk**
- **Low Risk**: Changes are additive (new field, new validation), no database schema changes
- **Backward Compatibility**: Data Storage Service already supports correlation_id (no rollback needed)
- **Audit Trail Gap**: Rolled-back version will have audit events without correlation_id (acceptable temporary state)

---

## 🔧 **Operational Considerations**

### **Applicability Assessment**

This feature (remediation_id propagation) is a **data flow enhancement** rather than a new service or infrastructure component. Traditional operational sections have limited applicability:

| Operational Section | Applicable? | Rationale |
|---------------------|-------------|-----------|
| **Prometheus Metrics** | ❌ Not applicable | No new metrics - uses existing audit event metrics from ADR-034 |
| **Grafana Dashboard** | ❌ Not applicable | No new dashboards - audit events already monitored in Data Storage Service |
| **Troubleshooting Guide** | ✅ **INCLUDED** | See "Validation Commands" in each day's section (Day 1 AM, Day 1 PM, Day 2 AM) |
| **Error Handling Philosophy** | ✅ **INCLUDED** | See "DO-REFACTOR Phase" (Day 1 EOD) - enhanced logging and validation |
| **Security Considerations** | ✅ **INCLUDED** | remediation_id is correlation ID (no PII), uses existing audit table security model |
| **Known Limitations** | ✅ **INCLUDED** | See "Rollback Risk" section - no database schema changes, low rollback risk |
| **Future Work** | ❌ Not applicable | Feature is complete in v1.0 - no planned enhancements beyond this scope |

### **Monitoring Strategy**

**Existing Infrastructure**: Remediation ID propagation leverages existing monitoring infrastructure:

1. **Audit Events Table** (ADR-034)
   - Already monitored via Data Storage Service Prometheus metrics
   - `audit_events_total` counter tracks all audit events
   - `audit_events_errors_total` counter tracks failures
   - No new metrics required

2. **HolmesGPT API Request Logging**
   - Already logs all incoming requests with structured logging
   - remediation_id will appear in existing request logs
   - No new logging infrastructure required

3. **AIAnalysis Controller Metrics**
   - Already tracks reconciliation success/failure rates
   - remediation_id validation errors will appear in existing error metrics
   - No new controller metrics required

**No New Monitoring Required**: This feature adds a field to existing data flows, not new infrastructure.

### **Error Handling Philosophy**

**Graceful Degradation**: Not applicable - remediation_id is mandatory for audit correlation per BR-AUDIT-001.

**Validation Strategy**:
1. **Early Validation**: AIAnalysis controller validates label presence before calling HolmesGPT API
2. **Pydantic Validation**: HolmesGPT API validates remediation_id field at API boundary
3. **Fail-Fast**: Missing remediation_id returns 400 Bad Request with clear error message
4. **No Silent Failures**: All validation errors are logged and returned to caller

### **Security Considerations**

**Threat Model**: remediation_id is a correlation identifier, not sensitive data.

**Security Properties**:
- **No PII**: remediation_id is a UUID-style identifier (e.g., `req-2025-11-27-abc123`)
- **No Authentication**: Uses existing AIAnalysis label mechanism (already secured)
- **No Authorization**: Uses existing audit table RBAC (already secured per ADR-034)
- **No Encryption**: Stored in audit_events table (already encrypted at rest per Data Storage Service security model)

**Mitigations**:
- **Input Validation**: Pydantic validates format and presence
- **SQL Injection**: Uses parameterized queries (existing audit table implementation)
- **Log Injection**: remediation_id is validated UUID format, no special characters

**No New Security Risks**: This feature uses existing security infrastructure.

### **Known Limitations (V1.0)**

1. **Breaking API Change**: Adding required `remediation_id` field to Pydantic models is a breaking change
   - **Mitigation**: Update AIAnalysis controller in same PR
   - **Impact**: Low - only internal service-to-service communication affected

2. **No Backward Compatibility**: Previous versions of HolmesGPT API will reject requests without remediation_id
   - **Mitigation**: Deploy all services simultaneously (AIAnalysis controller + HolmesGPT API)
   - **Impact**: Low - controlled deployment in single cluster

3. **No remediation_id Validation Format**: Currently accepts any non-empty string
   - **Future Enhancement**: Add UUID format validation in v1.1
   - **Impact**: Low - AIAnalysis controller generates valid UUIDs

### **Future Work**

**Not Planned**: This feature is complete in v1.0. No enhancements planned beyond this scope.

**Potential V1.1 Enhancements** (if needed):
- UUID format validation for remediation_id
- Audit event query API endpoint (filter by remediation_id)
- Grafana dashboard panel for remediation trace visualization

**Confidence**: 95% that v1.0 is sufficient for business requirements (BR-AUDIT-001, BR-INTEGRATION-003, BR-STORAGE-013)

---

## 🚀 **CI/CD Integration**

### **Automated Test Coverage Gates**

**GitHub Actions Configuration** (add to `.github/workflows/test.yml`):

```yaml
name: Remediation ID Tests

on:
  pull_request:
    paths:
      - 'internal/controller/aianalysis/**'
      - 'holmesgpt-api/src/models/**'
      - 'holmesgpt-api/src/extensions/**'
      - 'holmesgpt-api/src/toolsets/workflow_catalog.py'
      - 'test/unit/controller/**'
      - 'holmesgpt-api/tests/unit/**'
      - 'holmesgpt-api/tests/integration/**'

jobs:
  go-unit-tests:
    name: Go Unit Tests (AIAnalysis Controller)
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.21'

      - name: Run Go Unit Tests
        run: |
          go test ./test/unit/controller/aianalysis_remediation_id_test.go -v

      - name: Check Go Unit Test Coverage
        run: |
          go test ./test/unit/controller/... -coverprofile=coverage.out
          COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//')
          echo "Go unit test coverage: $COVERAGE%"
          if (( $(echo "$COVERAGE < 70" | bc -l) )); then
            echo "❌ Go unit test coverage ($COVERAGE%) below 70% threshold"
            exit 1
          fi
          echo "✅ Go unit test coverage: $COVERAGE%"

  python-unit-tests:
    name: Python Unit Tests (HolmesGPT API)
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Set up Python
        uses: actions/setup-python@v4
        with:
          python-version: '3.11'

      - name: Install dependencies
        run: |
          cd holmesgpt-api
          pip install -r requirements.txt
          pip install pytest pytest-cov

      - name: Run Python Unit Tests
        run: |
          cd holmesgpt-api
          pytest tests/unit/test_incident_models_remediation_id.py -v
          pytest tests/unit/test_workflow_catalog_remediation_id.py -v

      - name: Check Python Unit Test Coverage
        run: |
          cd holmesgpt-api
          pytest tests/unit/ --cov=src --cov-report=term-missing --cov-fail-under=70
          echo "✅ Python unit test coverage ≥70%"

  integration-tests:
    name: Integration Tests (End-to-End)
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:15
        env:
          POSTGRES_PASSWORD: postgres
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
      redis:
        image: redis:7
        options: >-
          --health-cmd "redis-cli ping"
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
    steps:
      - uses: actions/checkout@v3

      - name: Set up Python
        uses: actions/setup-python@v4
        with:
          python-version: '3.11'

      - name: Install dependencies
        run: |
          cd holmesgpt-api
          pip install -r requirements.txt
          pip install pytest

      - name: Run Integration Tests
        run: |
          cd holmesgpt-api
          pytest tests/integration/test_remediation_id_e2e.py -v
        env:
          DATA_STORAGE_URL: http://localhost:8080
          POSTGRES_HOST: localhost
          REDIS_HOST: localhost
```

### **Manual Validation Commands**

**Before submitting PR**, run these commands locally:

#### **Go Tests (AIAnalysis Controller)**
```bash
# Run Go unit tests
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
go test ./test/unit/controller/aianalysis_remediation_id_test.go -v

# Check Go coverage
go test ./test/unit/controller/... -coverprofile=coverage.out
go tool cover -func=coverage.out | grep total

# Expected: ≥70% coverage
```

#### **Python Tests (HolmesGPT API)**
```bash
# Run Python unit tests
cd holmesgpt-api
pytest tests/unit/test_incident_models_remediation_id.py -v
pytest tests/unit/test_workflow_catalog_remediation_id.py -v

# Check Python coverage
pytest tests/unit/ --cov=src --cov-report=html
open htmlcov/index.html  # View coverage report

# Expected: ≥70% coverage
```

#### **Integration Tests**
```bash
# Start Docker Compose environment
cd holmesgpt-api/tests/integration
docker-compose up -d

# Run integration tests
cd holmesgpt-api
pytest tests/integration/test_remediation_id_e2e.py -v

# Cleanup
cd tests/integration
docker-compose down

# Expected: All tests pass
```

#### **Manual Database Validation**
```bash
# Connect to PostgreSQL
kubectl exec -it postgresql-0 -n data-storage -- psql -U kubernaut -d kubernaut

# Query audit events by remediation_id
SELECT
  event_id,
  event_type,
  correlation_id,
  event_data->>'query' as query,
  created_at
FROM audit_events
WHERE correlation_id = 'req-2025-11-27-test'
ORDER BY created_at DESC
LIMIT 10;

# Expected: Audit events with correct correlation_id
```

### **Pre-Merge Checklist**

Before merging PR, verify:

- [ ] All Go unit tests passing (≥70% coverage)
- [ ] All Python unit tests passing (≥70% coverage)
- [ ] All integration tests passing
- [ ] Manual database validation confirms audit events
- [ ] No lint errors (`golangci-lint run`, `flake8`)
- [ ] Documentation updated (DD-WORKFLOW-002 v2.1, ADR-034 v1.1)
- [ ] Changelog entries added to all modified DDs

---

## 📚 **References**

### **Templates**
- [FEATURE_EXTENSION_PLAN_TEMPLATE.md](../../FEATURE_EXTENSION_PLAN_TEMPLATE.md) - Feature implementation template

### **Standards**
- [03-testing-strategy.mdc](../../../../.cursor/rules/03-testing-strategy.mdc) - Testing framework
- [02-go-coding-standards.mdc](../../../../.cursor/rules/02-go-coding-standards.mdc) - Go patterns
- [08-testing-anti-patterns.mdc](../../../../.cursor/rules/08-testing-anti-patterns.mdc) - Testing anti-patterns

### **Design Decisions**
- [DD-WORKFLOW-002 v2.1](../../../architecture/decisions/DD-WORKFLOW-002-MCP-WORKFLOW-CATALOG-ARCHITECTURE.md) - MCP Workflow Catalog Architecture
- [ADR-034 v1.1](../../../architecture/decisions/ADR-034-unified-audit-table-design.md) - Unified Audit Table Design
- [DD-WORKFLOW-014](../../../architecture/decisions/DD-WORKFLOW-014-workflow-selection-audit-trail.md) - Workflow Selection Audit Trail

### **Related Implementations**
- [DD-WORKFLOW-014 Implementation Plan](../data-storage/DD-WORKFLOW-014-IMPLEMENTATION-PLAN-V1.0.md) - Workflow Selection Audit Trail (Day 1 PM complete)

---

## 📝 **Files Modified Summary**

### **Go Files (AIAnalysis Controller)**
- `internal/controller/aianalysis/aianalysis_controller.go` - Extract and pass remediation_id (~20 LOC added)
- `test/unit/controller/aianalysis_remediation_id_test.go` - Unit tests (NEW, ~150 LOC)

### **Python Files (HolmesGPT API)**
- `holmesgpt-api/src/models/incident_models.py` - Add remediation_id field (~10 LOC added)
- `holmesgpt-api/src/models/recovery_models.py` - Add remediation_id field (~10 LOC added)
- `holmesgpt-api/src/extensions/incident.py` - Pass remediation_id to toolsets (~5 LOC added)
- `holmesgpt-api/src/extensions/recovery.py` - Pass remediation_id to toolsets (~5 LOC added)
- `holmesgpt-api/src/toolsets/workflow_catalog.py` - Use remediation_id in audit events (ALREADY DONE)
- `holmesgpt-api/tests/unit/test_incident_models_remediation_id.py` - Unit tests (NEW, ~100 LOC)
- `holmesgpt-api/tests/unit/test_workflow_catalog_remediation_id.py` - Unit tests (NEW, ~150 LOC)
- `holmesgpt-api/tests/integration/test_remediation_id_e2e.py` - Integration tests (NEW, ~200 LOC)
- `holmesgpt-api/tests/integration/helpers/audit_helpers.py` - Test helpers (NEW, ~50 LOC)

### **Documentation Files**
- `docs/architecture/decisions/DD-WORKFLOW-002-MCP-WORKFLOW-CATALOG-ARCHITECTURE.md` - Version 2.1 (DONE)
- `docs/architecture/decisions/ADR-034-unified-audit-table-design.md` - Version 1.1 (DONE)
- `docs/services/stateless/holmesgpt-api/DD-WORKFLOW-002-REMEDIATION-ID-IMPLEMENTATION_PLAN_V1.1.md` - This implementation plan (NEW)
- `holmesgpt-api/docs/API.md` - Add remediation_id parameter documentation (~20 LOC added)

### **No Changes Required**
- `pkg/datastorage/models/audit.go` - Already has correlation_id field
- `pkg/datastorage/server/audit_handlers.go` - Already accepts correlation_id
- Data Storage Service - No changes needed (already supports audit events)

**Total Files Modified**: 10 files
**Total Files Created**: 5 files (4 test files + 1 implementation plan)
**Total LOC Added**: ~720 LOC (including tests)

---

**Document Status**: 📋 **DRAFT**
**Last Updated**: 2025-11-27
**Version**: 1.2
**Maintained By**: Development Team

---

## 📝 **Next Steps**

1. **Get Plan Approval**: Review this plan with team, get approval to proceed
2. **Day 1 AM**: Execute DO-RED phase (write failing tests)
3. **Day 1 PM**: Execute DO-GREEN phase (implement core logic)
4. **Day 1 EOD**: Execute DO-REFACTOR phase (enhance error handling)
5. **Day 2 AM**: Execute CHECK phase (integration tests + manual validation)
6. **Day 2 PM**: Execute PRODUCTION phase (finalize documentation)

**Ready to implement?** Start with Day 1 AM (DO-RED Phase) and follow the APDC-TDD methodology!


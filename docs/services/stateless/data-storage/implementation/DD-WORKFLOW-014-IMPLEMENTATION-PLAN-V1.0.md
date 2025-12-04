# DD-WORKFLOW-014: Workflow Selection Audit Trail - Implementation Plan

**Version**: 1.0
**Filename**: `DD-WORKFLOW-014-IMPLEMENTATION-PLAN-V1.0.md`
**Status**: 📋 DRAFT
**Design Decision**: [DD-WORKFLOW-014](../../../architecture/decisions/DD-WORKFLOW-014-workflow-selection-audit-trail.md)
**Service**: HolmesGPT API (Python) + Data Storage Service (Go)
**Confidence**: 95% (Evidence-Based)
**Estimated Effort**: 2 days (APDC cycle: 1 day implementation + 0.5 days testing + 0.5 days documentation)

⚠️ **CRITICAL**: Filename version MUST match document version at all times.
- Document v1.0 → Filename `V1.0.md`

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
| **v1.0** | 2025-11-27 | Initial implementation plan created for workflow selection audit trail | ✅ **CURRENT** |

---

## 🎯 **Business Requirements**

### **Primary Business Requirements**

| BR ID | Description | Success Criteria |
|-------|-------------|------------------|
| **BR-AUDIT-005** | Workflow Selection Audit Trail | Every workflow search captured with scoring breakdown |
| **BR-HAPI-251** | HolmesGPT API Audit Integration | HolmesGPT API generates audit events for workflow selections |
| **BR-STORAGE-015** | Workflow Audit Query API | Operators can query workflow selection history |

### **Success Metrics**

**Format**: `[Metric]: [Target] - *Justification: [Why this target?]*`

**Your Metrics**:
- **Audit Event Coverage**: 100% of workflow searches - *Justification: Compliance requires complete audit trail (SOC 2, ISO 27001)*
- **Query Performance**: <200ms P95 for workflow selection queries - *Justification: Operators need fast debugging experience*
- **Audit Write Latency**: <50ms P95 - *Justification: Audit writes must not slow down workflow search response*
- **Storage Efficiency**: <2KB per audit event - *Justification: 1000 searches/day = 2MB/day = 730MB/year (acceptable)*

---

## 📅 **Timeline Overview**

### **Phase Breakdown**

| Phase | Duration | Days | Purpose | Key Deliverables |
|-------|----------|------|---------|------------------|
| **ANALYSIS** | 2 hours | Day 0 (pre-work) | Comprehensive context understanding | Analysis document, risk assessment, existing code review |
| **PLAN** | 2 hours | Day 0 (pre-work) | Detailed implementation strategy | This document, TDD phase mapping, success criteria |
| **DO (Implementation)** | 1 day | Day 1 | Controlled TDD execution | Audit event generation, API integration |
| **CHECK (Testing)** | 0.5 days | Day 2 (morning) | Comprehensive result validation | Test suite (unit/integration), BR validation |
| **PRODUCTION READINESS** | 0.5 days | Day 2 (afternoon) | Documentation & deployment prep | Runbooks, handoff docs, confidence report |

### **2-Day Implementation Timeline**

| Day | Phase | Focus | Hours | Key Milestones |
|-----|-------|-------|-------|----------------|
| **Day 0** | ANALYSIS + PLAN | Pre-work | 4h | ✅ Analysis complete, Plan approved (this document) |
| **Day 1 AM** | DO-RED | Test framework + Failing tests | 4h | Test framework, interfaces, failing tests |
| **Day 1 PM** | DO-GREEN | Core audit logic | 4h | Audit event generation, API integration |
| **Day 2 AM** | CHECK | Unit + Integration tests | 4h | 70%+ unit coverage, integration tests passing |
| **Day 2 PM** | PRODUCTION | Documentation + Handoff | 4h | API docs, runbooks, confidence report |

### **Critical Path Dependencies**

```
Day 1 AM (Foundation) → Day 1 PM (Core Logic)
                                   ↓
Day 2 AM (Testing) → Day 2 PM (Production Readiness)
```

### **Daily Progress Tracking**

**EOD Documentation Required**:
- **Day 1 Complete**: Implementation checkpoint (audit event generation working)
- **Day 2 Complete**: Production ready checkpoint (tests passing, docs complete)

---

## 📆 **Day-by-Day Implementation Breakdown**

### **Day 0: ANALYSIS + PLAN (Pre-Work) ✅**

**Phase**: ANALYSIS + PLAN
**Duration**: 4 hours
**Status**: ✅ COMPLETE (this document represents Day 0 completion)

**Deliverables**:
- ✅ Analysis document: DD-WORKFLOW-014 created with complete audit event schema
- ✅ Implementation plan (this document v1.0): 2-day timeline, test examples
- ✅ Risk assessment: 2 critical pitfalls identified with mitigation strategies
- ✅ Existing code review: `holmesgpt-api/src/toolsets/workflow_catalog.py`, `pkg/datastorage/server/audit_handlers.go`
- ✅ BR coverage matrix: 3 primary BRs mapped to test scenarios

---

### **Day 1 AM: Foundation + Test Framework (DO-RED Phase)**

**Phase**: DO-RED
**Duration**: 4 hours
**TDD Focus**: Write failing tests first, enhance existing code

**⚠️ CRITICAL**: We are **ENHANCING existing code**, not creating from scratch!

**Existing Code to Enhance**:
- ✅ `holmesgpt-api/src/toolsets/workflow_catalog.py` (518 LOC) - SearchWorkflowCatalogTool
- ✅ `pkg/datastorage/server/audit_handlers.go` (existing audit API) - Write audit events
- ✅ `pkg/datastorage/models/audit.go` (existing audit models) - Audit event schema

**Morning Tasks (4 hours)**:

**Hour 1: Analyze Existing Implementation**
1. **Read** `holmesgpt-api/src/toolsets/workflow_catalog.py`
   - Understand `_invoke()` method (line 338-400)
   - Identify where audit event should be generated (after Data Storage response)
   - Understand `_transform_api_response()` method (line 402-450)

2. **Read** `pkg/datastorage/server/audit_handlers.go`
   - Understand existing `WriteAuditEvent()` handler
   - Identify validation requirements for workflow audit events

3. **Read** `pkg/datastorage/models/audit.go`
   - Understand `AuditEvent` struct
   - Verify `event_data` JSONB field supports workflow schema

**Hour 2: Create Python Test File**
4. **Create** `holmesgpt-api/tests/unit/test_workflow_catalog_audit.py` (200-300 LOC)
   - Set up pytest test suite
   - Define test fixtures for audit event validation
   - Create mock Data Storage audit API client

**Hour 3: Create Go Test File**
5. **Create** `test/unit/datastorage/workflow_audit_test.go` (200-300 LOC)
   - Set up Ginkgo/Gomega test suite
   - Define test fixtures for workflow audit event validation
   - Create helper functions for audit event testing

**Hour 4: Write Failing Tests**
6. **Write failing tests** (strict TDD: ONE test at a time)
   - **TDD Cycle 1**: Test audit event generation in Python (should call audit API)
   - **TDD Cycle 2**: Test audit event validation in Go (should validate workflow schema)
   - Run tests → Verify they FAIL (RED phase)

**EOD Deliverables**:
- ✅ Test framework complete (Python + Go)
- ✅ 2 failing tests (RED phase)
- ✅ Enhanced interfaces defined
- ✅ Day 1 AM checkpoint report

**Validation Commands**:
```bash
# Python: Verify tests fail (RED phase)
cd holmesgpt-api
python3 -m pytest tests/unit/test_workflow_catalog_audit.py -v 2>&1 | grep "FAILED"

# Go: Verify tests fail (RED phase)
go test ./test/unit/datastorage/workflow_audit_test.go -v 2>&1 | grep "FAIL"

# Expected: All tests should FAIL with "not implemented yet"
```

---

### **Day 1 PM: Core Audit Logic (DO-GREEN Phase)**

**Phase**: DO-GREEN
**Duration**: 4 hours
**TDD Focus**: Minimal implementation to pass tests

**Afternoon Tasks (4 hours)**:

**Hour 1-2: Python Implementation**
1. **Enhance** `holmesgpt-api/src/toolsets/workflow_catalog.py` (add ~100 LOC)
   - Add `_send_audit_event()` method
   - Add `_calculate_boost_breakdown()` helper
   - Add `_calculate_penalty_breakdown()` helper
   - Add `_get_correlation_id()` helper (from context)

2. **Implement audit event generation** in `_invoke()` method
   - After Data Storage response received
   - Build audit event payload per DD-WORKFLOW-014 schema
   - Send to Data Storage audit API

**Hour 3: Go Implementation**
3. **Enhance** `pkg/datastorage/server/audit_handlers.go` (add ~50 LOC)
   - Add workflow audit event validation in `WriteAuditEvent()`
   - Validate `event_type == "workflow.catalog.search_completed"`
   - Validate `event_data` contains required fields (query, results, workflows)

4. **Run tests** → Verify they PASS (GREEN phase)

**Hour 4: Integration Testing**
5. **Manual integration test**
   - Start Data Storage service
   - Run HolmesGPT API workflow search
   - Verify audit event written to database
   - Query audit event and validate schema

**EOD Deliverables**:
- ✅ Audit event generation implemented (Python)
- ✅ Audit event validation implemented (Go)
- ✅ All unit tests passing
- ✅ Manual integration test successful
- ✅ Day 1 PM checkpoint report

**Validation Commands**:
```bash
# Python: Verify tests pass (GREEN phase)
cd holmesgpt-api
python3 -m pytest tests/unit/test_workflow_catalog_audit.py -v

# Go: Verify tests pass (GREEN phase)
go test ./test/unit/datastorage/workflow_audit_test.go -v

# Expected: All tests should PASS
```

---

### **Day 2 AM: Comprehensive Testing (CHECK Phase)**

**Phase**: CHECK
**Duration**: 4 hours
**Focus**: Unit + Integration test coverage

**Morning Tasks (4 hours)**:

**Hour 1: Expand Python Unit Tests**
1. **Expand unit tests** to 70%+ coverage
   - Test edge cases (empty results, API timeout, malformed response)
   - Test error conditions (audit API failure, network error)
   - Test boundary values (large result sets, missing fields)

2. **Behavior & Correctness Validation**
   - Tests validate WHAT the system does (not HOW)
   - Clear business scenarios in test names
   - Specific assertions (not weak checks)

**Hour 2: Expand Go Unit Tests**
3. **Expand unit tests** to 70%+ coverage
   - Test workflow audit event validation (valid/invalid schemas)
   - Test JSONB query patterns (scoring breakdown queries)
   - Test edge cases (missing fields, invalid types)

**Hour 3: Integration Tests**
4. **Create** `test/integration/datastorage/workflow_audit_integration_test.go` (300-400 LOC)
   - Test complete audit event flow (Python → Go → PostgreSQL)
   - Test audit event query API
   - Test scoring breakdown queries

5. **Infrastructure setup**
   - Ensure PostgreSQL test database is available
   - Add cleanup logic (`AfterEach` with async waits)
   - Handle port collisions, resource conflicts

**Hour 4: Integration Scenarios**
6. **Test realistic scenarios**
   - Multiple workflow searches with different queries
   - Concurrent audit event writes
   - Query workflow selection history by correlation_id

**EOD Deliverables**:
- ✅ 70%+ unit test coverage (Python + Go)
- ✅ All unit tests passing
- ✅ Integration tests passing
- ✅ Tests follow behavior/correctness protocol
- ✅ Day 2 AM checkpoint report

**Validation Commands**:
```bash
# Python: Run unit tests with coverage
cd holmesgpt-api
python3 -m pytest tests/unit/test_workflow_catalog_audit.py --cov=src/toolsets/workflow_catalog --cov-report=term

# Go: Run unit tests with coverage
go test ./test/unit/datastorage/workflow_audit_test.go -coverprofile=coverage.out
go tool cover -func=coverage.out | grep total

# Expected: total coverage ≥70%

# Run integration tests
go test ./test/integration/datastorage/workflow_audit_integration_test.go -v

# Expected: All integration tests pass
```

---

### **Day 2 PM: Production Readiness (PRODUCTION Phase)**

**Phase**: PRODUCTION
**Duration**: 4 hours
**Focus**: Documentation and handoff

**Afternoon Tasks (4 hours)**:

**Hour 1: Update Service Documentation**
1. **Update** `holmesgpt-api/README.md`
   - Add workflow selection audit trail feature
   - Document audit event schema
   - Add configuration examples

2. **Update** `docs/services/stateless/data-storage/API.md`
   - Document workflow audit event type
   - Add example audit event payload
   - Document query patterns

**Hour 2: Create Operational Documentation**
3. **Create** `docs/services/stateless/data-storage/operations/workflow-audit-runbook.md`
   - Configuration guide (audit API endpoint, timeout)
   - Troubleshooting guide (common issues, debug commands)
   - Query examples (debugging workflow selection, tuning workflows)

4. **Update** `holmesgpt-api/config/holmesgpt-api.yaml` (inline comments)
   - Document `data_storage_audit_url` field
   - Document `audit_timeout_seconds` field
   - Provide examples and defaults

**Hour 3: Analytics Queries**
5. **Document analytics queries** in runbook
   - Workflow selection rate query
   - Low confidence workflow query
   - Search pattern analysis query
   - Most selected workflows query

**Hour 4: Handoff Summary**
6. **Create** `docs/services/stateless/data-storage/DD-WORKFLOW-014-HANDOFF.md`
   - Executive summary
   - Architecture overview (audit event flow)
   - Key decisions (event schema, query patterns)
   - Lessons learned
   - Known limitations
   - Confidence assessment (95%)

**EOD Deliverables**:
- ✅ Service documentation updated
- ✅ Operational runbook created
- ✅ Analytics queries documented
- ✅ Handoff summary complete
- ✅ Production ready checkpoint report

---

## 🧪 **TDD Do's and Don'ts - MANDATORY**

### **✅ DO: Strict TDD Discipline**

1. **Write ONE test at a time** (not batched)
   ```python
   # ✅ CORRECT: TDD Cycle 1
   def test_audit_event_generated_after_workflow_search():
       # Test for audit event generation
       pass
   # Run test → FAIL (RED)
   # Implement _send_audit_event() → PASS (GREEN)
   # Refactor if needed
   ```

2. **Test WHAT the system does** (behavior), not HOW (implementation)
   ```python
   # ✅ CORRECT: Behavior-focused
   def test_audit_event_contains_scoring_breakdown():
       # Verify audit event has base_similarity, label_boost, label_penalty
       assert "base_similarity" in audit_event["event_data"]["results"]["workflows"][0]["scoring"]
   ```

3. **Use specific assertions** (not weak checks)
   ```python
   # ✅ CORRECT: Specific business assertions
   assert audit_event["event_type"] == "workflow.catalog.search_completed"
   assert len(audit_event["event_data"]["results"]["workflows"]) == 3
   ```

### **❌ DON'T: Anti-Patterns to Avoid**

1. **DON'T batch test writing**
   ```python
   # ❌ WRONG: Writing 10 tests before any implementation
   def test_1(): pass
   def test_2(): pass
   def test_3(): pass
   # ... 7 more tests
   # Then implementing all at once
   ```

2. **DON'T test implementation details**
   ```python
   # ❌ WRONG: Testing internal state
   assert tool._audit_buffer is not None
   assert tool._last_audit_timestamp > 0
   ```

3. **DON'T use weak assertions (NULL-TESTING)**
   ```python
   # ❌ WRONG: Weak assertions
   assert audit_event is not None
   assert len(audit_event) > 0
   ```

**Reference**: `.cursor/rules/08-testing-anti-patterns.mdc` for automated detection

---

## 📊 **Test Examples**

### **📁 Test File Locations - MANDATORY (READ THIS FIRST)**

**AUTHORITY**: [03-testing-strategy.mdc](../../.cursor/rules/03-testing-strategy.mdc)

**🚨 CRITICAL**: Test files MUST be in specific directories, NOT co-located with source code!

| Test Type | File Location | Example |
|-----------|---------------|---------|
| **Python Unit Tests** | `holmesgpt-api/tests/unit/` | `holmesgpt-api/tests/unit/test_workflow_catalog_audit.py` |
| **Go Unit Tests** | `test/unit/datastorage/` | `test/unit/datastorage/workflow_audit_test.go` |
| **Go Integration Tests** | `test/integration/datastorage/` | `test/integration/datastorage/workflow_audit_integration_test.go` |

---

### **Python Unit Test Example**

```python
# holmesgpt-api/tests/unit/test_workflow_catalog_audit.py

import pytest
from unittest.mock import Mock, patch, AsyncMock
from src.toolsets.workflow_catalog import SearchWorkflowCatalogTool

class TestWorkflowCatalogAudit:
    """
    Unit tests for workflow catalog audit trail

    Business Requirement: BR-AUDIT-005 (Workflow Selection Audit Trail)
    Design Decision: DD-WORKFLOW-014
    """

    @pytest.fixture
    def mock_data_storage_response(self):
        """Mock Data Storage API response"""
        return {
            "workflows": [
                {
                    "workflow": {
                        "workflow_id": "pod-oom-gitops",
                        "version": "v1.0.0",
                        "title": "Pod OOM GitOps Recovery",
                        "labels": {
                            "signal-type": "OOMKilled",
                            "severity": "critical",
                            "resource-management": "gitops"
                        }
                    },
                    "base_similarity": 0.88,
                    "label_boost": 0.18,
                    "label_penalty": 0.0,
                    "final_score": 1.0,
                    "rank": 1
                }
            ],
            "total_results": 1
        }

    @pytest.fixture
    def tool(self):
        """Create SearchWorkflowCatalogTool instance"""
        return SearchWorkflowCatalogTool(
            data_storage_url="http://localhost:8080",
            audit_url="http://localhost:8080/api/v1/audit/events"
        )

    @pytest.mark.asyncio
    async def test_audit_event_generated_after_workflow_search(
        self, tool, mock_data_storage_response
    ):
        """
        BUSINESS SCENARIO: After workflow search, audit event is generated

        BR-AUDIT-005: Workflow Selection Audit Trail
        """
        with patch('requests.post') as mock_post:
            # Mock Data Storage search response
            mock_search_response = Mock()
            mock_search_response.json.return_value = mock_data_storage_response
            mock_search_response.elapsed.total_seconds.return_value = 0.045

            # Mock audit API response
            mock_audit_response = Mock()
            mock_audit_response.status_code = 201
            mock_audit_response.json.return_value = {"event_id": "uuid-123"}

            # Configure mock to return different responses for search vs audit
            mock_post.side_effect = [mock_search_response, mock_audit_response]

            # BEHAVIOR: Execute workflow search
            result = await tool._invoke(
                query="OOMKilled critical",
                filters={"resource-management": "gitops"},
                top_k=3
            )

            # CORRECTNESS: Verify audit event was sent
            assert mock_post.call_count == 2, "Should call search API + audit API"

            # Verify audit API call
            audit_call = mock_post.call_args_list[1]
            assert audit_call[0][0].endswith("/api/v1/audit/events")

            audit_payload = audit_call[1]["json"]

            # BUSINESS OUTCOME: Audit event contains complete workflow selection data
            assert audit_payload["event_type"] == "workflow.catalog.search_completed"
            assert audit_payload["event_category"] == "workflow"
            assert audit_payload["event_action"] == "search_completed"
            assert audit_payload["event_outcome"] == "success"
            assert audit_payload["service"] == "holmesgpt-api"

            # Verify scoring breakdown is included
            workflow_result = audit_payload["event_data"]["results"]["workflows"][0]
            assert workflow_result["scoring"]["confidence"] == 1.0
            assert workflow_result["scoring"]["base_similarity"] == 0.88
            assert workflow_result["scoring"]["label_boost"] == 0.18
            assert workflow_result["scoring"]["label_penalty"] == 0.0

            # Verify boost breakdown
            assert "boost_breakdown" in workflow_result["scoring"]
            assert workflow_result["scoring"]["boost_breakdown"]["resource-management"] == 0.10

    @pytest.mark.asyncio
    async def test_audit_event_includes_query_metadata(self, tool, mock_data_storage_response):
        """
        BUSINESS SCENARIO: Audit event includes complete query metadata

        BR-AUDIT-005: Workflow Selection Audit Trail
        """
        with patch('requests.post') as mock_post:
            mock_search_response = Mock()
            mock_search_response.json.return_value = mock_data_storage_response
            mock_search_response.elapsed.total_seconds.return_value = 0.045

            mock_audit_response = Mock()
            mock_audit_response.status_code = 201

            mock_post.side_effect = [mock_search_response, mock_audit_response]

            # BEHAVIOR: Execute workflow search with filters
            await tool._invoke(
                query="OOMKilled critical",
                filters={
                    "resource-management": "gitops",
                    "environment": "production"
                },
                top_k=5
            )

            # CORRECTNESS: Verify query metadata in audit event
            audit_call = mock_post.call_args_list[1]
            audit_payload = audit_call[1]["json"]

            query_data = audit_payload["event_data"]["query"]
            assert query_data["text"] == "OOMKilled critical"
            assert query_data["filters"]["resource-management"] == "gitops"
            assert query_data["filters"]["environment"] == "production"
            assert query_data["top_k"] == 5

            # BUSINESS OUTCOME: Complete query context captured for debugging
```

---

### **Go Unit Test Example**

```go
// test/unit/datastorage/workflow_audit_test.go

package datastorage

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

var _ = Describe("Workflow Audit Event Validation", func() {
    Context("when validating workflow.catalog.search_completed event", func() {
        It("should accept valid workflow audit event", func() {
            // BUSINESS SCENARIO: Valid workflow audit event is accepted
            // BR-AUDIT-005: Workflow Selection Audit Trail

            event := &models.AuditEvent{
                EventType:     "workflow.catalog.search_completed",
                EventCategory: "workflow",
                EventAction:   "search_completed",
                EventOutcome:  "success",
                Service:       "holmesgpt-api",
                CorrelationID: "rr-2025-001",
                EventData: map[string]interface{}{
                    "query": map[string]interface{}{
                        "text": "OOMKilled critical",
                        "filters": map[string]interface{}{
                            "signal-type": "OOMKilled",
                            "severity":    "critical",
                        },
                        "top_k": 3,
                    },
                    "results": map[string]interface{}{
                        "total_found": 5,
                        "returned":    3,
                        "workflows": []interface{}{
                            map[string]interface{}{
                                "workflow_id": "pod-oom-gitops",
                                "version":     "v1.0.0",
                                "rank":        1,
                                "scoring": map[string]interface{}{
                                    "confidence":       1.0,
                                    "base_similarity":  0.88,
                                    "label_boost":      0.18,
                                    "label_penalty":    0.0,
                                    "boost_breakdown":  map[string]interface{}{},
                                    "penalty_breakdown": map[string]interface{}{},
                                },
                            },
                        },
                    },
                },
            }

            // BEHAVIOR: Validate event schema
            err := validateWorkflowAuditEvent(event)

            // CORRECTNESS: Event should be valid
            Expect(err).ToNot(HaveOccurred(), "Valid workflow audit event should be accepted")

            // BUSINESS OUTCOME: Workflow selection audit trail is complete
        })

        It("should reject workflow audit event with missing query", func() {
            // BUSINESS SCENARIO: Incomplete audit event is rejected

            event := &models.AuditEvent{
                EventType:     "workflow.catalog.search_completed",
                EventCategory: "workflow",
                EventAction:   "search_completed",
                EventOutcome:  "success",
                Service:       "holmesgpt-api",
                CorrelationID: "rr-2025-001",
                EventData: map[string]interface{}{
                    "results": map[string]interface{}{
                        "total_found": 0,
                        "returned":    0,
                        "workflows":   []interface{}{},
                    },
                    // Missing "query" field
                },
            }

            // BEHAVIOR: Validate event schema
            err := validateWorkflowAuditEvent(event)

            // CORRECTNESS: Event should be rejected
            Expect(err).To(HaveOccurred(), "Event with missing query should be rejected")
            Expect(err.Error()).To(ContainSubstring("missing required field: query"))

            // BUSINESS OUTCOME: Incomplete audit events are prevented
        })

        It("should reject workflow audit event with missing scoring breakdown", func() {
            // BUSINESS SCENARIO: Audit event without scoring breakdown is rejected

            event := &models.AuditEvent{
                EventType:     "workflow.catalog.search_completed",
                EventCategory: "workflow",
                EventAction:   "search_completed",
                EventOutcome:  "success",
                Service:       "holmesgpt-api",
                CorrelationID: "rr-2025-001",
                EventData: map[string]interface{}{
                    "query": map[string]interface{}{
                        "text": "OOMKilled critical",
                    },
                    "results": map[string]interface{}{
                        "workflows": []interface{}{
                            map[string]interface{}{
                                "workflow_id": "pod-oom-gitops",
                                // Missing "scoring" field
                            },
                        },
                    },
                },
            }

            // BEHAVIOR: Validate event schema
            err := validateWorkflowAuditEvent(event)

            // CORRECTNESS: Event should be rejected
            Expect(err).To(HaveOccurred(), "Event with missing scoring should be rejected")
            Expect(err.Error()).To(ContainSubstring("missing required field: scoring"))

            // BUSINESS OUTCOME: Scoring breakdown is mandatory for debugging
        })
    })
})

// Helper function for validation (to be implemented)
func validateWorkflowAuditEvent(event *models.AuditEvent) error {
    // Implementation will be added during DO-GREEN phase
    return nil
}
```

---

### **Go Integration Test Example**

```go
// test/integration/datastorage/workflow_audit_integration_test.go

package datastorage

import (
    "context"
    "encoding/json"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "github.com/jordigilh/kubernaut/pkg/datastorage/models"
    "github.com/jordigilh/kubernaut/test/integration/infrastructure"
)

var _ = Describe("Workflow Audit Integration Tests", func() {
    var (
        ctx        context.Context
        testServer *infrastructure.TestDataStorageServer
        auditRepo  *infrastructure.TestAuditRepository
    )

    BeforeEach(func() {
        ctx = context.Background()
        testServer = infrastructure.NewTestDataStorageServer()
        auditRepo = infrastructure.NewTestAuditRepository(testServer.DB)
    })

    AfterEach(func() {
        testServer.Cleanup()
    })

    Context("when writing workflow audit event", func() {
        It("should persist complete audit event to database", func() {
            // BUSINESS SCENARIO: Workflow audit event is persisted with scoring breakdown
            // BR-AUDIT-005: Workflow Selection Audit Trail

            event := &models.AuditEvent{
                EventType:     "workflow.catalog.search_completed",
                EventCategory: "workflow",
                EventAction:   "search_completed",
                EventOutcome:  "success",
                Service:       "holmesgpt-api",
                CorrelationID: "rr-2025-001",
                EventData: map[string]interface{}{
                    "query": map[string]interface{}{
                        "text": "OOMKilled critical",
                        "filters": map[string]interface{}{
                            "signal-type": "OOMKilled",
                            "severity":    "critical",
                        },
                        "top_k": 3,
                    },
                    "results": map[string]interface{}{
                        "total_found": 1,
                        "returned":    1,
                        "workflows": []interface{}{
                            map[string]interface{}{
                                "workflow_id": "pod-oom-gitops",
                                "version":     "v1.0.0",
                                "rank":        1,
                                "scoring": map[string]interface{}{
                                    "confidence":      1.0,
                                    "base_similarity": 0.88,
                                    "label_boost":     0.18,
                                    "label_penalty":   0.0,
                                },
                            },
                        },
                    },
                },
            }

            // BEHAVIOR: Write audit event
            err := auditRepo.WriteEvent(ctx, event)
            Expect(err).ToNot(HaveOccurred())

            // CORRECTNESS: Query audit event from database
            events, err := auditRepo.QueryEvents(ctx, &models.AuditEventQuery{
                EventType:     "workflow.catalog.search_completed",
                CorrelationID: "rr-2025-001",
            })

            Expect(err).ToNot(HaveOccurred())
            Expect(events).To(HaveLen(1))

            retrievedEvent := events[0]
            Expect(retrievedEvent.EventType).To(Equal("workflow.catalog.search_completed"))
            Expect(retrievedEvent.EventCategory).To(Equal("workflow"))

            // Verify scoring breakdown is preserved
            eventData := retrievedEvent.EventData.(map[string]interface{})
            results := eventData["results"].(map[string]interface{})
            workflows := results["workflows"].([]interface{})
            workflow := workflows[0].(map[string]interface{})
            scoring := workflow["scoring"].(map[string]interface{})

            Expect(scoring["confidence"]).To(Equal(1.0))
            Expect(scoring["base_similarity"]).To(Equal(0.88))
            Expect(scoring["label_boost"]).To(Equal(0.18))

            // BUSINESS OUTCOME: Complete audit trail persisted for debugging
        })
    })

    Context("when querying workflow selection history", func() {
        It("should return workflows by correlation_id", func() {
            // BUSINESS SCENARIO: Operator queries workflow selection history for remediation
            // BR-STORAGE-015: Workflow Audit Query API

            // Setup: Write 3 workflow search audit events
            for i := 1; i <= 3; i++ {
                event := &models.AuditEvent{
                    EventType:     "workflow.catalog.search_completed",
                    EventCategory: "workflow",
                    CorrelationID: "rr-2025-001",
                    EventData: map[string]interface{}{
                        "query": map[string]interface{}{
                            "text": "OOMKilled critical",
                        },
                        "results": map[string]interface{}{
                            "returned": i,
                        },
                    },
                }
                err := auditRepo.WriteEvent(ctx, event)
                Expect(err).ToNot(HaveOccurred())
            }

            // BEHAVIOR: Query workflow selection history
            events, err := auditRepo.QueryEvents(ctx, &models.AuditEventQuery{
                EventType:     "workflow.catalog.search_completed",
                CorrelationID: "rr-2025-001",
            })

            // CORRECTNESS: All 3 events returned
            Expect(err).ToNot(HaveOccurred())
            Expect(events).To(HaveLen(3))

            // BUSINESS OUTCOME: Complete workflow selection history available
        })
    })
})
```

---

## 🏗️ **Integration Test Environment Decision**

### **Environment Strategy**

**Decision**: Podman (PostgreSQL container)

**Environment Comparison**:

| Environment | Pros | Cons | Decision |
|-------------|------|------|----------|
| **Podman** | Fast (<5s startup), isolated, reuses existing infrastructure | Requires Podman installed | ✅ **Selected** |
| **KIND** | Full K8s, realistic | Slow startup (30s+), overkill for audit API | ❌ Not Selected |
| **envtest** | Fast, lightweight | No real database, low confidence | ❌ Not Selected |
| **Mocks** | Fastest | Not integration tests, low confidence | ❌ Not Selected |

**Rationale**:
- Fast startup (<5s) enables rapid test iteration
- Isolated per test process prevents test interference
- Reuses existing Data Storage integration test infrastructure
- Realistic PostgreSQL interactions validate JSONB queries
- No K8s overhead for simple audit API testing

---

## 🎯 **BR Coverage Matrix**

| BR ID | Description | Unit Tests | Integration Tests | Status |
|-------|-------------|------------|-------------------|--------|
| **BR-AUDIT-005** | Workflow Selection Audit Trail | `test_workflow_catalog_audit.py`, `workflow_audit_test.go` | `workflow_audit_integration_test.go` | ✅ |
| **BR-HAPI-251** | HolmesGPT API Audit Integration | `test_workflow_catalog_audit.py` | `workflow_audit_integration_test.go` | ✅ |
| **BR-STORAGE-015** | Workflow Audit Query API | `workflow_audit_test.go` | `workflow_audit_integration_test.go` | ✅ |

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

- ❌ **Problem**: New handler (`handleCreateAuditEvent`) was implemented **WITHOUT writing tests first**. DLQ fallback was added to code **WITHOUT corresponding test coverage**. Tests were written **AFTER** implementation, not before.
- ✅ **Solution**:
  - Write **ONE test at a time** (not in batches)
  - Follow strict **RED-GREEN-REFACTOR** sequence for every feature
  - **NEVER** write implementation code before writing a failing test
  - Each test must map to a specific **BR-[SERVICE]-XXX** requirement
- **Impact**: **CRITICAL** - DLQ functionality was **MISSING** in production code until E2E test caught it. High risk of data loss in production. Required emergency fix and comprehensive test backfill.
- **Evidence**: Unit Tests: 0/1 DLQ tests (0% coverage), Integration Tests: 1/3 DLQ tests (33% coverage), E2E Tests: 1/1 DLQ tests (100% coverage) - but **TOO LATE**

**Mitigation for This Implementation**:
- ✅ Write audit event generation test FIRST (RED phase)
- ✅ Implement audit event generation AFTER test fails
- ✅ Write audit event validation test FIRST (RED phase)
- ✅ Implement validation AFTER test fails

---

### **2. Missing Integration Tests for New Endpoints** 🔴 **CRITICAL**

- ❌ **Problem**: New unified audit events endpoint (`/api/v1/audit/events`) was implemented with **NO integration tests**. DLQ fallback functionality was missing because integration tests would have caught it early in the development cycle.
- ✅ **Solution**:
  - **Integration tests MUST exist BEFORE E2E tests** (MANDATORY)
  - Every new HTTP endpoint **MUST have integration tests** covering:
    - Success path
    - Failure paths
    - Edge cases (empty results, invalid filters, timeouts)
  - Integration tests must validate component interactions (e.g., handler → repository → database)
- **Impact**: **CRITICAL** - Critical functionality (DLQ fallback) was missing. E2E test was the first to catch the issue (too late in development cycle). Required backfilling integration tests after implementation.

**Mitigation for This Implementation**:
- ✅ Integration tests created on Day 2 AM (before any E2E tests)
- ✅ Integration tests cover audit event write + query
- ✅ Integration tests validate PostgreSQL JSONB storage

---

### **3. Audit Event Schema Validation Gap** 🟡 **MEDIUM** (Feature-Specific)

- ❌ **Problem**: Audit event schema for workflow selection is complex (nested JSONB with scoring breakdown). Missing field validation could lead to incomplete audit trail.
- ✅ **Solution**:
  - Create comprehensive validation function for workflow audit events
  - Validate all required fields (query, results, workflows, scoring)
  - Validate scoring breakdown structure (base_similarity, label_boost, label_penalty)
  - Return clear error messages for missing/invalid fields
- **Impact**: **MEDIUM** - Incomplete audit events would prevent debugging workflow selection. Operators would lose visibility into scoring breakdown.

**Mitigation for This Implementation**:
- ✅ Unit tests validate complete audit event schema
- ✅ Unit tests validate missing field detection
- ✅ Integration tests validate JSONB storage and retrieval

---

## 📈 **Success Criteria**

### **Technical Success**
- ✅ All tests passing (Unit 70%+, Integration >50%)
- ✅ No lint errors (Python: flake8, Go: golangci-lint)
- ✅ Audit events written to PostgreSQL
- ✅ Audit events queryable by correlation_id
- ✅ Documentation complete

### **Business Success**
- ✅ BR-AUDIT-005 validated (workflow selection audit trail)
- ✅ BR-HAPI-251 validated (HolmesGPT API audit integration)
- ✅ BR-STORAGE-015 validated (workflow audit query API)
- ✅ Success metrics achieved (100% coverage, <200ms query, <50ms write)

### **Confidence Assessment**
- **Target**: ≥95% confidence
- **Calculation**: Evidence-based (test coverage + BR validation + integration status)

---

## 📊 **Confidence Calculation Methodology**

**Overall Confidence**: 95% (Evidence-Based)

**Component Breakdown**:

| Component | Confidence | Evidence |
|-----------|-----------|----------|
| **Audit Event Schema** | 98% | Follows ADR-034 unified audit table design (battle-tested) |
| **Python Implementation** | 95% | Enhances existing `workflow_catalog.py` (proven patterns) |
| **Go Implementation** | 95% | Enhances existing `audit_handlers.go` (proven patterns) |
| **JSONB Storage** | 98% | PostgreSQL JSONB proven in existing audit events |
| **Query Performance** | 90% | GIN index on `event_data` (ADR-034 design) |

**Risk Assessment**:
- **5% Risk**: Complex JSONB queries may require index tuning
  - **Mitigation**: Integration tests validate query performance
- **5% Risk**: Audit event payload size (>2KB) may impact storage
  - **Mitigation**: Monitor storage growth, implement archival if needed

**Assumptions**:
- PostgreSQL GIN index provides <200ms query performance
- Audit event write latency <50ms (async buffered pattern per ADR-038)
- 1000 workflow searches/day = 2MB/day = 730MB/year (acceptable)

**Validation Approach**:
- Unit tests validate audit event generation and schema
- Integration tests validate PostgreSQL storage and queries
- Manual testing validates end-to-end flow (HolmesGPT → Data Storage → PostgreSQL)

**Calculation Formula**:
```
Overall Confidence = (98% + 95% + 95% + 98% + 90%) / 5 = 95.2% ≈ 95%
```

---

## 🔄 **Rollback Plan**

### **Rollback Triggers**
- Audit event write failures >5% (high error rate)
- Query performance degradation >500ms P95
- Storage growth >10MB/day (unexpected)

### **Rollback Procedure**
1. **Disable audit event generation** in HolmesGPT API
   - Set `audit_enabled: false` in config
   - Deploy updated config
2. **Verify rollback success**
   - Confirm audit event writes stopped
   - Confirm workflow search still works
3. **Document rollback reason**
   - Create incident report
   - Identify root cause
   - Plan fix

**Rollback Impact**: Workflow selection audit trail temporarily unavailable (non-critical)

---

## 📚 **References**

### **Design Decisions**
- [DD-WORKFLOW-014](../../../architecture/decisions/DD-WORKFLOW-014-workflow-selection-audit-trail.md) - Workflow Selection Audit Trail
- [DD-WORKFLOW-013](../../../architecture/decisions/DD-WORKFLOW-013-scoring-field-population.md) - Scoring Field Population
- [DD-WORKFLOW-004](../../../architecture/decisions/DD-WORKFLOW-004-hybrid-weighted-label-scoring.md) - Hybrid Weighted Label Scoring
- [ADR-034](../../../architecture/decisions/ADR-034-unified-audit-table-design.md) - Unified Audit Table Design
- [ADR-038](../../../architecture/decisions/ADR-038-async-buffered-audit-ingestion.md) - Asynchronous Buffered Audit Ingestion
- [DD-AUDIT-001](../../../architecture/decisions/DD-AUDIT-001-audit-responsibility-pattern.md) - Audit Responsibility Pattern

### **Standards**
- [03-testing-strategy.mdc](../../../.cursor/rules/03-testing-strategy.mdc) - Testing framework
- [02-go-coding-standards.mdc](../../../.cursor/rules/02-go-coding-standards.mdc) - Go patterns
- [08-testing-anti-patterns.mdc](../../../.cursor/rules/08-testing-anti-patterns.mdc) - Testing anti-patterns

---

## 📝 **Operational Sections**

### **Prometheus Metrics**

**New Metrics**:
```go
// Audit event write metrics
workflow_audit_events_total{status="success|failure"}
workflow_audit_write_duration_seconds{quantile="0.5|0.95|0.99"}
workflow_audit_write_errors_total{error_type="validation|network|database"}

// Audit query metrics
workflow_audit_query_duration_seconds{quantile="0.5|0.95|0.99"}
workflow_audit_query_results_total
```

**Alert Rules**:
```yaml
# High audit write error rate
- alert: WorkflowAuditWriteErrorRate
  expr: rate(workflow_audit_write_errors_total[5m]) > 0.05
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "High workflow audit write error rate"
    description: "Audit write error rate is {{ $value }} errors/sec"

# Slow audit queries
- alert: WorkflowAuditQuerySlow
  expr: histogram_quantile(0.95, workflow_audit_query_duration_seconds) > 0.5
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "Slow workflow audit queries"
    description: "P95 query latency is {{ $value }}s"
```

---

### **Grafana Dashboard**

**Panels**:
1. **Audit Event Write Rate** (Graph)
   - Query: `rate(workflow_audit_events_total{status="success"}[5m])`
   - Unit: events/sec

2. **Audit Write Latency** (Graph)
   - Query: `histogram_quantile(0.95, workflow_audit_write_duration_seconds)`
   - Unit: seconds

3. **Audit Query Performance** (Graph)
   - Query: `histogram_quantile(0.95, workflow_audit_query_duration_seconds)`
   - Unit: seconds

4. **Audit Write Errors** (Graph)
   - Query: `rate(workflow_audit_write_errors_total[5m])`
   - Unit: errors/sec

---

### **Troubleshooting Guide**

**Common Issues**:

**Issue 1: Audit events not appearing in database**
- **Symptom**: Workflow searches succeed but no audit events in database
- **Debug Commands**:
  ```bash
  # Check HolmesGPT API logs
  kubectl logs -n kubernaut -l app=holmesgpt-api | grep "audit_event"

  # Check Data Storage audit API
  curl -X POST http://data-storage:8080/api/v1/audit/events \
    -H "Content-Type: application/json" \
    -d '{"event_type": "test", "service": "test"}'

  # Query database directly
  psql -h localhost -U postgres -d kubernaut \
    -c "SELECT COUNT(*) FROM audit_events WHERE event_type = 'workflow.catalog.search_completed';"
  ```
- **Solution**: Check audit API endpoint configuration in HolmesGPT API config

**Issue 2: Slow audit queries**
- **Symptom**: Workflow selection history queries take >500ms
- **Debug Commands**:
  ```bash
  # Check PostgreSQL query plan
  psql -h localhost -U postgres -d kubernaut \
    -c "EXPLAIN ANALYZE SELECT * FROM audit_events WHERE event_type = 'workflow.catalog.search_completed' AND correlation_id = 'rr-2025-001';"

  # Check GIN index usage
  psql -h localhost -U postgres -d kubernaut \
    -c "SELECT indexname, indexdef FROM pg_indexes WHERE tablename = 'audit_events';"
  ```
- **Solution**: Verify GIN index on `event_data` exists, consider adding index on `(event_type, correlation_id)`

**Issue 3: Large audit event payloads**
- **Symptom**: Audit events >5KB, storage growing rapidly
- **Debug Commands**:
  ```bash
  # Check average event size
  psql -h localhost -U postgres -d kubernaut \
    -c "SELECT AVG(LENGTH(event_data::text)) FROM audit_events WHERE event_type = 'workflow.catalog.search_completed';"
  ```
- **Solution**: Implement event data truncation for large result sets (>10 workflows)

---

### **Error Handling Philosophy**

**Graceful Degradation**:
- Audit event write failures **DO NOT** block workflow search
- Audit API timeout (5s) prevents slow audit writes from impacting user experience
- Audit event validation errors logged but do not fail workflow search

**Retry Strategy**:
- No retries for audit event writes (fire-and-forget pattern per ADR-038)
- Audit events are best-effort (not critical path)
- DLQ fallback for persistent failures (future enhancement)

---

### **Security Considerations**

**Threat Model**:
- **Threat**: Unauthorized access to workflow selection history
  - **Mitigation**: Audit query API requires authentication (RBAC)
- **Threat**: Audit event tampering
  - **Mitigation**: Audit events are immutable (append-only table)
- **Threat**: PII leakage in audit events
  - **Mitigation**: Workflow descriptions sanitized, no user PII in audit events

**RBAC Requirements**:
```yaml
# ClusterRole for audit query access
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: workflow-audit-reader
rules:
- apiGroups: [""]
  resources: ["audit-events"]
  verbs: ["get", "list"]
  resourceNames: ["workflow.catalog.search_completed"]
```

---

### **Known Limitations (V1.0)**

1. **No Audit Event Aggregation**: Individual audit events per search (not aggregated)
   - **Impact**: High storage for frequent searches
   - **Future**: V1.1 will add hourly aggregation

2. **No Audit Event Archival**: All audit events retained indefinitely
   - **Impact**: Storage growth over time
   - **Future**: V1.2 will add S3 archival after 90 days

3. **No Real-Time Audit Streaming**: Audit events only queryable from database
   - **Impact**: No real-time audit monitoring
   - **Future**: V2.0 will add Kafka streaming

---

### **Future Work (V1.1/V1.2/V2.0)**

**V1.1 (Q1 2026)**: Audit Event Aggregation
- Hourly aggregation of workflow searches
- Reduce storage by 80% for high-volume searches

**V1.2 (Q2 2026)**: Audit Event Archival
- S3 archival after 90 days
- Reduce active database storage by 70%

**V2.0 (Q3 2026)**: Real-Time Audit Streaming
- Kafka streaming for real-time audit monitoring
- Grafana dashboard for live workflow selection analytics

---

**Document Status**: 📋 **DRAFT**
**Last Updated**: 2025-11-27
**Version**: 1.0
**Maintained By**: Development Team

---

## ✅ **Ready to Implement**

This implementation plan follows the APDC-TDD methodology and includes:
- ✅ Comprehensive 2-day timeline
- ✅ Detailed test examples (Python + Go)
- ✅ Integration test strategy (Podman + PostgreSQL)
- ✅ BR coverage matrix (100% coverage)
- ✅ Critical pitfalls identified and mitigated
- ✅ 95% confidence assessment with evidence
- ✅ Operational documentation (metrics, alerts, troubleshooting)

**Start with Day 1 AM (DO-RED phase) and follow the TDD methodology!**


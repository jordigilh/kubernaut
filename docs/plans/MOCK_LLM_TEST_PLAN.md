# Mock LLM Service Test Plan

**Document ID**: PLAN-MOCK-LLM-TEST-001
**Version**: 1.3.0
**Created**: 2026-01-10
**Last Updated**: 2026-01-10
**Owner**: Test Team
**Status**: Draft

---

## Changelog

### Version 1.3.0 (2026-01-10)
- **Updated**: Swapped validation and cleanup phases (validate BEFORE cleanup)
- **Clarified**: AIAnalysis integration/E2E tests require Mock LLM (same as HAPI, DataStorage)
- **Updated**: Tier 5 (AIAnalysis) now explicit requirement, not optional
- **Rationale**: All test tiers must pass before removing business code

### Version 1.2.0 (2026-01-10)
- **Added**: Ginkgo synchronized suite lifecycle tests (Tier 2)
- **Added**: Test validation for parallel process coordination
- **Added**: Container teardown timing validation (after all processes finish)
- **Updated**: Integration test tier to validate `SynchronizedBeforeSuite`/`SynchronizedAfterSuite`
- **Clarified**: AIAnalysis integration uses Ginkgo coordination, HAPI uses pytest session fixtures

### Version 1.1.0 (2026-01-10)
- **Updated**: Service location from `test/e2e/services/mock-llm/` to `test/services/mock-llm/` (shared across test tiers)
- **Updated**: Tier 2 integration tests to use programmatic podman deployment (not compose)
- **Added**: HAPI integration test validation for mock LLM connection
- **Clarified**: Integration and E2E deployment patterns

### Version 1.0.0 (2026-01-10)
- Initial test plan created
- 6 test tiers defined (Unit, Integration, HAPI Integration, HAPI E2E, AIAnalysis, Performance)
- 40+ tests planned
- Success criteria established

---
**Related**: PLAN-MOCK-LLM-001 (Migration Plan)

---

## Executive Summary

**Objective**: Comprehensive testing strategy for standalone Mock LLM Service.

**Scope**:
- Unit tests for Mock LLM Service itself
- Integration tests with HAPI
- Integration tests with AIAnalysis
- E2E tests enabling 12 skipped tests
- Regression tests across all services

**Success Criteria**:
- ✅ 100% Mock LLM core functionality validated
- ✅ 12 HAPI E2E tests enabled and passing
- ✅ 0 test regressions in HAPI or AIAnalysis
- ✅ Performance SLAs met

---

## Test Tier 1: Mock LLM Service Unit Tests

### Objective
Validate Mock LLM Service core functionality in isolation.

### Test Environment
- **Location**: `test/services/mock-llm/tests/`
- **Framework**: pytest
- **Execution**: Local development, CI/CD
- **Duration Target**: <30 seconds

---

### Test Suite 1.1: Server Initialization

#### Test 1.1.1: Server Starts Successfully
- **ID**: MOCK-UT-001
- **Description**: Verify server starts without errors
- **Steps**:
  1. Import server module
  2. Initialize FastAPI app
  3. Check app configuration loaded
- **Expected**: Server initializes, no exceptions
- **Status**: [ ] Not Started

#### Test 1.1.2: Scenarios Load Correctly
- **ID**: MOCK-UT-002
- **Description**: Verify all scenario definitions load
- **Steps**:
  1. Import scenarios module
  2. Check `MOCK_SCENARIOS` dictionary populated
  3. Validate scenario structure
- **Expected**: 6+ scenarios loaded, all required fields present
- **Status**: [ ] Not Started

#### Test 1.1.3: Configuration from Environment
- **ID**: MOCK-UT-003
- **Description**: Verify config loads from env vars
- **Steps**:
  1. Set env vars (PORT, LOG_LEVEL)
  2. Load config
  3. Validate values
- **Expected**: Config matches environment settings
- **Status**: [ ] Not Started

---

### Test Suite 1.2: Health Endpoints

#### Test 1.2.1: Health Endpoint Returns 200
- **ID**: MOCK-UT-004
- **Description**: Verify liveness probe endpoint
- **Steps**:
  1. GET /health
  2. Check status code
  3. Check response body
- **Expected**: 200 OK, `{"status": "healthy"}`
- **Status**: [ ] Not Started

#### Test 1.2.2: Readiness Endpoint Returns 200
- **ID**: MOCK-UT-005
- **Description**: Verify readiness probe endpoint
- **Steps**:
  1. GET /ready
  2. Check status code
  3. Check response includes scenario count
- **Expected**: 200 OK, `{"status": "ready", "scenarios_loaded": 6}`
- **Status**: [ ] Not Started

#### Test 1.2.3: Models Endpoint OpenAI Compatible
- **ID**: MOCK-UT-006
- **Description**: Verify models list endpoint
- **Steps**:
  1. GET /v1/models
  2. Check response structure
- **Expected**: OpenAI-compatible models list
- **Status**: [ ] Not Started

---

### Test Suite 1.3: Chat Completions - Basic

#### Test 1.3.1: Simple Text Response
- **ID**: MOCK-UT-007
- **Description**: Verify basic chat completion
- **Steps**:
  1. POST /v1/chat/completions with simple message
  2. No tools parameter
  3. Check response structure
- **Expected**: Valid OpenAI response, text content present
- **Status**: [ ] Not Started

#### Test 1.3.2: Scenario Selection by Signal Type
- **ID**: MOCK-UT-008
- **Description**: Verify correct scenario selected
- **Steps**:
  1. POST with message containing "OOMKilled"
  2. Check response content
- **Expected**: Response matches OOMKilled scenario
- **Status**: [ ] Not Started

#### Test 1.3.3: Default Scenario for Unknown Type
- **ID**: MOCK-UT-009
- **Description**: Verify fallback to default scenario
- **Steps**:
  1. POST with unknown signal type
  2. Check response
- **Expected**: Default scenario response returned
- **Status**: [ ] Not Started

---

### Test Suite 1.4: Chat Completions - Tool Calls

#### Test 1.4.1: Tool Call Response Format
- **ID**: MOCK-UT-010 (CRITICAL)
- **Description**: Verify tool_calls structure
- **Steps**:
  1. POST with tools parameter
  2. Check response contains tool_calls array
  3. Validate structure
- **Expected**:
  ```json
  {
    "choices": [{
      "message": {
        "role": "assistant",
        "content": null,
        "tool_calls": [{
          "id": "call_...",
          "type": "function",
          "function": {
            "name": "search_workflow_catalog",
            "arguments": "{...}"
          }
        }]
      },
      "finish_reason": "tool_calls"
    }]
  }
  ```
- **Status**: [ ] Not Started

#### Test 1.4.2: Tool Call Arguments Valid JSON
- **ID**: MOCK-UT-011
- **Description**: Verify tool arguments are valid JSON
- **Steps**:
  1. POST with tools
  2. Extract tool_calls[0].function.arguments
  3. Parse as JSON
- **Expected**: Valid JSON, no parse errors
- **Status**: [ ] Not Started

#### Test 1.4.3: Tool Call ID Generation
- **ID**: MOCK-UT-012
- **Description**: Verify unique tool call IDs
- **Steps**:
  1. Make 3 requests with tools
  2. Extract tool call IDs
  3. Check uniqueness
- **Expected**: All IDs unique
- **Status**: [ ] Not Started

---

### Test Suite 1.5: Multi-Turn Conversations

#### Test 1.5.1: Tool Call Then Tool Result
- **ID**: MOCK-UT-013 (CRITICAL)
- **Description**: Verify multi-turn conversation handling
- **Steps**:
  1. Turn 1: POST with tools → get tool_calls
  2. Turn 2: POST with tool result → get final response
  3. Check final response has text content
- **Expected**: Turn 2 returns analysis text, not tool_calls
- **Status**: [ ] Not Started

#### Test 1.5.2: Multiple Tool Calls in Sequence
- **ID**: MOCK-UT-014
- **Description**: Verify handling of multiple tool results
- **Steps**:
  1. POST with multiple tool calls
  2. Provide multiple tool results
  3. Check response
- **Expected**: Final response incorporates all tool results
- **Status**: [ ] Not Started

#### Test 1.5.3: Conversation State Isolation
- **ID**: MOCK-UT-015
- **Description**: Verify parallel requests don't interfere
- **Steps**:
  1. Start conversation A (tool call)
  2. Start conversation B (tool call)
  3. Continue both independently
- **Expected**: No state leakage between conversations
- **Status**: [ ] Not Started

---

### Test Suite 1.6: Edge Cases

#### Test 1.6.1: No Workflow Found Scenario
- **ID**: MOCK-UT-016
- **Description**: Verify MOCK_NO_WORKFLOW_FOUND handling
- **Steps**:
  1. POST with signal_type="MOCK_NO_WORKFLOW_FOUND"
  2. Check response
- **Expected**: Response indicates needs_human_review=true
- **Status**: [ ] Not Started

#### Test 1.6.2: Low Confidence Scenario
- **ID**: MOCK-UT-017
- **Description**: Verify MOCK_LOW_CONFIDENCE handling
- **Steps**:
  1. POST with signal_type="MOCK_LOW_CONFIDENCE"
  2. Check confidence field
- **Expected**: Low confidence value, needs_human_review=true
- **Status**: [ ] Not Started

#### Test 1.6.3: Invalid Request Handling
- **ID**: MOCK-UT-018
- **Description**: Verify graceful error handling
- **Steps**:
  1. POST with malformed JSON
  2. POST with missing required fields
  3. Check error responses
- **Expected**: 400/422 errors, descriptive messages
- **Status**: [ ] Not Started

---

### Test Suite 1.7: Tool Call Tracking (Test Helpers)

#### Test 1.7.1: Tool Call Recording
- **ID**: MOCK-UT-019
- **Description**: Verify tool calls are tracked
- **Steps**:
  1. Make request with tools
  2. Query tool call tracker
  3. Check recorded calls
- **Expected**: Tool call recorded with correct name/args
- **Status**: [ ] Not Started

#### Test 1.7.2: Tool Call Assertion Helper
- **ID**: MOCK-UT-020
- **Description**: Verify assertion helper works
- **Steps**:
  1. Make request with specific tool args
  2. Use assert_tool_called_with()
  3. Check assertion passes/fails correctly
- **Expected**: Helper correctly validates tool calls
- **Status**: [ ] Not Started

---

## Test Tier 2: Mock LLM Integration Tests

### Objective
Validate Mock LLM Service integration with test infrastructure.

### Test Environment
- **Execution**: Podman container locally
- **Duration Target**: <2 minutes

---

### Test Suite 2.1: Container Deployment

#### Test 2.1.1: Docker Build Succeeds
- **ID**: MOCK-IT-001
- **Description**: Verify Dockerfile builds
- **Steps**:
  1. `podman build -t localhost/mock-llm:test .`
  2. Check exit code
  3. Check image exists
- **Expected**: Build succeeds, image tagged
- **Status**: [ ] Not Started

#### Test 2.1.2: Container Starts Successfully
- **ID**: MOCK-IT-002
- **Description**: Verify container runs
- **Steps**:
  1. `podman run -d -p 8080:8080 localhost/mock-llm:test`
  2. Check container status
  3. Check logs for errors
- **Expected**: Container running, no crash loops
- **Status**: [ ] Not Started

#### Test 2.1.3: Health Checks Pass in Container
- **ID**: MOCK-IT-003
- **Description**: Verify health endpoints work in container
- **Steps**:
  1. Wait 10 seconds for startup
  2. `curl http://localhost:8080/health`
  3. `curl http://localhost:8080/ready`
- **Expected**: Both return 200 OK
- **Status**: [ ] Not Started

---

### Test Suite 2.2: Lifecycle Management (Ginkgo Coordination)

#### Test 2.2.1: Synchronized Suite Startup
- **ID**: MOCK-IT-003a
- **Description**: Verify Ginkgo SynchronizedBeforeSuite pattern
- **Steps**:
  1. Run AIAnalysis integration tests with `-p` (4 parallel processes)
  2. Verify Process 1 starts Mock LLM container (logs show "Starting Mock LLM")
  3. Verify all processes wait for `/health` ready
  4. Verify all processes receive endpoint URL
- **Expected**: Container started once, all processes proceed
- **Pattern**: Same as DataStorage in `test/integration/aianalysis/suite_test.go`
- **Status**: [ ] Not Started

#### Test 2.2.2: Parallel Process Coordination
- **ID**: MOCK-IT-003b
- **Description**: Verify multiple processes share same Mock LLM container
- **Steps**:
  1. Run AIAnalysis integration tests with `-p 4`
  2. Check `podman ps` shows only ONE mock-llm container
  3. Verify all 4 processes can make requests to `http://localhost:8080`
  4. Verify no port conflicts
- **Expected**: Single container serves all parallel processes
- **Status**: [ ] Not Started

#### Test 2.2.3: Synchronized Suite Teardown
- **ID**: MOCK-IT-003c
- **Description**: Verify Mock LLM torn down AFTER all processes finish
- **Steps**:
  1. Run AIAnalysis integration tests with `-p 4`
  2. Monitor container lifecycle: `podman events --filter container=mock-llm`
  3. Verify container remains running until all 4 processes complete
  4. Verify SynchronizedAfterSuite stops container (Process 1 only)
  5. Verify `podman ps` shows no mock-llm container after suite
- **Expected**: Container survives until last process, then removed
- **Critical**: Prevents "connection refused" errors from premature teardown
- **Status**: [ ] Not Started

---

### Test Suite 2.3: OpenAI API Compatibility

#### Test 2.2.1: Chat Completions via Container
- **ID**: MOCK-IT-004
- **Description**: Verify API works through container
- **Steps**:
  1. POST to http://localhost:8080/v1/chat/completions
  2. Include OpenAI-format request
  3. Parse response
- **Expected**: Valid OpenAI response structure
- **Status**: [ ] Not Started

#### Test 2.2.2: Tool Calls via Container
- **ID**: MOCK-IT-005 (CRITICAL)
- **Description**: Verify tool calls work in container
- **Steps**:
  1. POST with tools parameter
  2. Check response has tool_calls
- **Expected**: Tool calls present in response
- **Status**: [ ] Not Started

---

### Test Suite 2.4: Kind Cluster Deployment

#### Test 2.3.1: Deploy to Kind Successfully
- **ID**: MOCK-IT-006
- **Description**: Verify K8s deployment
- **Steps**:
  1. Create test Kind cluster
  2. Load image to Kind
  3. `kubectl apply -k kubernetes/`
  4. Wait for pod ready
- **Expected**: Pod running, ready
- **Status**: [ ] Not Started

#### Test 2.3.2: Service Accessible via NodePort
- **ID**: MOCK-IT-007
- **Description**: Verify NodePort access
- **Steps**:
  1. Get NodePort URL
  2. `curl http://localhost:30180/health`
- **Expected**: 200 OK from Kind service
- **Status**: [ ] Not Started

#### Test 2.3.3: Pod Restart Recovery
- **ID**: MOCK-IT-008
- **Description**: Verify pod recovers from crash
- **Steps**:
  1. Delete pod
  2. Wait for new pod
  3. Check health
- **Expected**: New pod starts, service available
- **Status**: [ ] Not Started

---

## Test Tier 3: HAPI Integration Tests

### Objective
Validate HAPI integration with Mock LLM Service.

### Test Environment
- **Location**: HAPI integration test suite
- **Execution**: Make target or direct go test
- **Duration Target**: <3 minutes

---

### Test Suite 3.1: HAPI Connects to Mock LLM

#### Test 3.1.1: HAPI Calls Mock LLM Endpoint
- **ID**: HAPI-IT-001
- **Description**: Verify HAPI reaches mock LLM
- **Steps**:
  1. Start mock LLM in podman-compose
  2. Configure HAPI with LLM_ENDPOINT=http://mock-llm:8080
  3. Trigger HAPI incident analysis
  4. Check mock LLM logs for request
- **Expected**: Mock LLM receives request from HAPI
- **Status**: [ ] Not Started

#### Test 3.1.2: HAPI Processes Mock Response
- **ID**: HAPI-IT-002
- **Description**: Verify HAPI handles mock response
- **Steps**:
  1. HAPI calls mock LLM
  2. Mock returns deterministic response
  3. Check HAPI creates WorkflowExecution
- **Expected**: HAPI successfully processes mock response
- **Status**: [ ] Not Started

#### Test 3.1.3: HAPI Tool Call Integration
- **ID**: HAPI-IT-003 (CRITICAL)
- **Description**: Verify HAPI handles tool calls from mock
- **Steps**:
  1. HAPI requests with tools enabled
  2. Mock returns tool_calls
  3. HAPI provides tool result
  4. Mock returns final analysis
- **Expected**: Full tool call flow works
- **Status**: [ ] Not Started

---

### Test Suite 3.2: HAPI Regression Tests

#### Test 3.2.1: All HAPI Integration Tests Pass
- **ID**: HAPI-IT-004
- **Description**: Verify no regressions
- **Steps**:
  1. Run `make test-integration-holmesgpt-api`
  2. Count passing tests
- **Expected**: 65/65 tests pass (100%)
- **Status**: [ ] Not Started

#### Test 3.2.2: HAPI Audit Events Still Generated
- **ID**: HAPI-IT-005
- **Description**: Verify audit trail preserved
- **Steps**:
  1. Run HAPI integration test with audit validation
  2. Check audit events written to DataStorage
- **Expected**: LLM request/response events present
- **Status**: [ ] Not Started

---

## Test Tier 4: HAPI E2E Tests (12 Skipped Tests)

### Objective
Enable and validate 12 previously skipped HAPI E2E tests.

### Test Environment
- **Location**: `holmesgpt-api/tests/e2e/`
- **Execution**: pytest in Kind cluster
- **Duration Target**: <15 minutes

---

### Test Suite 4.1: Workflow Selection Tests

#### Test 4.1.1: Incident Analysis Calls Workflow Search Tool
- **ID**: HAPI-E2E-001 (CRITICAL)
- **File**: `test_workflow_selection_e2e.py`
- **Test**: `test_incident_analysis_calls_workflow_search_tool`
- **Description**: Verify LLM tool call for workflow search
- **Steps**:
  1. HAPI receives incident request
  2. Calls mock LLM
  3. Mock returns tool_calls for search_workflow_catalog
  4. Validate tool call structure
- **Expected**:
  - Tool called with correct name
  - Tool arguments include signal_type
  - Tool result provided back to LLM
- **Status**: [ ] Not Started

#### Test 4.1.2: Incident with Detected Labels Passes to Tool
- **ID**: HAPI-E2E-002 (CRITICAL)
- **File**: `test_workflow_selection_e2e.py`
- **Test**: `test_incident_with_detected_labels_passes_to_tool`
- **Description**: Verify detected labels in tool arguments
- **Steps**:
  1. HAPI receives incident with detected_labels
  2. Mock LLM returns tool_calls
  3. Validate tool arguments include detected_labels
- **Expected**: Tool args contain gitOpsManaged, pdbProtected, etc.
- **Status**: [ ] Not Started

#### Test 4.1.3: Recovery Analysis Calls Workflow Search Tool
- **ID**: HAPI-E2E-003 (CRITICAL)
- **File**: `test_workflow_selection_e2e.py`
- **Test**: `test_recovery_analysis_calls_workflow_search_tool`
- **Description**: Verify recovery flow uses tool calls
- **Steps**:
  1. HAPI receives recovery request
  2. Mock LLM returns tool_calls
  3. Validate workflow search occurs
- **Expected**: Recovery workflow uses tool call flow
- **Status**: [ ] Not Started

---

### Test Suite 4.2: Tool Call Format Validation

#### Test 4.2.1: Tool Call Query Format
- **ID**: HAPI-E2E-004
- **File**: `test_workflow_selection_e2e.py`
- **Test**: `test_tool_call_query_format`
- **Description**: Verify tool call structure matches spec
- **Steps**:
  1. Capture tool call from mock LLM response
  2. Validate JSON schema
- **Expected**: Tool call matches OpenAI spec
- **Status**: [ ] Not Started

#### Test 4.2.2: Tool Call Arguments Complete
- **ID**: HAPI-E2E-005
- **File**: `test_workflow_selection_e2e.py`
- **Test**: `test_tool_call_arguments_complete`
- **Description**: Verify all required args present
- **Steps**:
  1. Check tool call arguments
  2. Validate required fields (signal_type, detected_labels, etc.)
- **Expected**: All required arguments present
- **Status**: [ ] Not Started

---

### Test Suite 4.3: Additional Skipped Tests

#### Test 4.3.X: Remaining 7 Skipped Tests
- **ID**: HAPI-E2E-006 through HAPI-E2E-012
- **Description**: Enable and validate remaining skipped tests
- **Location**: Search for `@pytest.mark.skip` in E2E files
- **Expected**: All tests enabled and passing
- **Status**: [ ] Inventory needed

---

### Test Suite 4.4: HAPI E2E Regression

#### Test 4.4.1: All HAPI E2E Tests Pass
- **ID**: HAPI-E2E-013
- **Description**: Verify no regressions
- **Steps**:
  1. Run `make test-e2e-holmesgpt-api`
  2. Count passing/skipped tests
- **Expected**: 58 passed, 0 skipped (was 46 passed, 12 skipped)
- **Status**: [ ] Not Started

#### Test 4.4.2: E2E Test Duration Acceptable
- **ID**: HAPI-E2E-014
- **Description**: Verify performance not degraded
- **Steps**:
  1. Measure E2E test duration
  2. Compare to baseline (~12 minutes)
- **Expected**: Duration < 15 minutes (< 25% increase)
- **Status**: [ ] Not Started

---

## Test Tier 5: AIAnalysis Integration Tests

### Objective
Validate AIAnalysis integration with Mock LLM Service (no regressions).

### Test Environment
- **Location**: `test/integration/aianalysis/`
- **Execution**: Make target or Ginkgo
- **Duration Target**: <5 minutes

---

### Test Suite 5.1: AIAnalysis with Mock LLM

#### Test 5.1.1: AIAnalysis Calls HAPI (Mock LLM Mode)
- **ID**: AA-IT-001
- **Description**: Verify AIAnalysis → HAPI → Mock LLM flow
- **Steps**:
  1. Create RemediationProcessing CRD
  2. AIAnalysis reconciles
  3. Calls HAPI (which uses mock LLM)
  4. Check WorkflowExecution created
- **Expected**: Full flow works with mock LLM backend
- **Status**: [ ] Not Started

#### Test 5.1.2: Deterministic Mock Responses
- **ID**: AA-IT-002
- **Description**: Verify mock responses are deterministic
- **Steps**:
  1. Run same test 3 times
  2. Compare HAPI responses
- **Expected**: Identical responses each time
- **Status**: [ ] Not Started

---

### Test Suite 5.2: AIAnalysis Regression

#### Test 5.2.1: All AIAnalysis Integration Tests Pass
- **ID**: AA-IT-003
- **Description**: Verify no regressions
- **Steps**:
  1. Run `make test-integration-aianalysis`
  2. Count passing tests
- **Expected**: 100% pass rate (same as before migration)
- **Status**: [ ] Not Started

#### Test 5.2.2: AIAnalysis E2E Tests Pass
- **ID**: AA-IT-004
- **Description**: Verify E2E tests still work
- **Steps**:
  1. Run `make test-e2e-aianalysis`
  2. Check results
- **Expected**: All tests pass (no regressions)
- **Status**: [ ] Not Started

---

## Test Tier 6: Performance & Load Tests

### Objective
Validate Mock LLM Service performance meets SLAs.

### Test Environment
- **Execution**: Load testing tools (optional)
- **Duration**: 10-15 minutes

---

### Test Suite 6.1: Response Time

#### Test 6.1.1: Health Endpoint Latency
- **ID**: PERF-001
- **Description**: Measure health endpoint response time
- **Steps**:
  1. Make 100 requests to /health
  2. Calculate p50, p95, p99
- **Expected**: p95 < 50ms
- **Status**: [ ] Not Started

#### Test 6.1.2: Chat Completion Latency
- **ID**: PERF-002
- **Description**: Measure chat completion response time
- **Steps**:
  1. Make 50 chat completion requests
  2. Calculate latency percentiles
- **Expected**: p95 < 500ms
- **Status**: [ ] Not Started

---

### Test Suite 6.2: Concurrency

#### Test 6.2.1: Parallel Requests
- **ID**: PERF-003
- **Description**: Verify handling of concurrent requests
- **Steps**:
  1. Send 10 concurrent requests
  2. Check all succeed
  3. Verify no state leakage
- **Expected**: All requests succeed, correct responses
- **Status**: [ ] Not Started

---

## Test Execution Plan

### Phase 1: Unit Tests (Day 1)
1. **Morning**: Implement Mock LLM unit tests (Suite 1.1-1.3)
2. **Afternoon**: Implement tool call tests (Suite 1.4-1.5)
3. **Evening**: Edge case tests (Suite 1.6-1.7)
4. **Gate**: All unit tests must pass before Phase 2

### Phase 2: Integration Tests (Day 2 Morning)
1. Container deployment tests (Suite 2.1)
2. Kind cluster tests (Suite 2.3)
3. **Gate**: Mock LLM deploys successfully

### Phase 3: HAPI Integration (Day 2 Afternoon)
1. HAPI integration tests (Suite 3.1)
2. HAPI regression validation (Suite 3.2)
3. **Gate**: No HAPI regressions

### Phase 4: Enable Skipped Tests (Day 3 Morning)
1. Enable 12 HAPI E2E tests (Suite 4.1-4.3)
2. Run HAPI E2E suite
3. **Gate**: All 12 tests passing

### Phase 5: AIAnalysis Validation (Day 3 Afternoon)
1. AIAnalysis integration tests (Suite 5.1)
2. AIAnalysis regression tests (Suite 5.2)
3. **Gate**: No AIAnalysis regressions

### Phase 6: Performance Validation (Optional)
1. Response time tests (Suite 6.1)
2. Concurrency tests (Suite 6.2)
3. **Gate**: Performance SLAs met

---

## Test Tracking Matrix

| Test ID | Suite | Priority | Status | Assignee | Notes |
|---------|-------|----------|--------|----------|-------|
| MOCK-UT-001 | 1.1 | P1 | [ ] | - | - |
| MOCK-UT-010 | 1.4 | **P0** | [ ] | - | CRITICAL: Tool calls |
| MOCK-UT-013 | 1.5 | **P0** | [ ] | - | CRITICAL: Multi-turn |
| HAPI-E2E-001 | 4.1 | **P0** | [ ] | - | CRITICAL: Enable skipped test |
| HAPI-E2E-002 | 4.1 | **P0** | [ ] | - | CRITICAL: Enable skipped test |
| HAPI-E2E-003 | 4.1 | **P0** | [ ] | - | CRITICAL: Enable skipped test |
| ... | ... | ... | ... | ... | ... |

---

## Bug Tracking Template

### Bug Report Format
```
**Bug ID**: MOCK-BUG-XXX
**Test ID**: [Failed Test ID]
**Severity**: Critical / High / Medium / Low
**Description**: [What failed]
**Steps to Reproduce**:
1.
2.
3.
**Expected**: [Expected behavior]
**Actual**: [Actual behavior]
**Logs**: [Relevant logs]
**Workaround**: [If available]
**Fix Required**: Yes / No
```

---

## Success Criteria

### Coverage
- ✅ 100% Mock LLM unit test coverage (all 20 tests)
- ✅ 100% integration test coverage (all 8 tests)
- ✅ 100% HAPI E2E skipped tests enabled (12 tests)
- ✅ 100% regression test coverage (HAPI + AA)

### Quality
- ✅ 0 critical bugs
- ✅ 0 test failures
- ✅ 0 regressions in HAPI or AIAnalysis
- ✅ Performance SLAs met

### Documentation
- ✅ All tests documented
- ✅ Bug tracking maintained
- ✅ Test results published

---

## Sign-off

- [ ] **Test Lead**: Test plan reviewed and approved
- [ ] **Development Lead**: Technical approach validated
- [ ] **QA Team**: Test coverage adequate
- [ ] **Ready to Execute**: All approvals obtained

---

## Change Log

| Date | Version | Changes | Author |
|------|---------|---------|--------|
| 2026-01-10 | 1.0 | Initial draft | AI Assistant |

# HolmesGPT-API (HAPI) E2E Test Plan - Python to Go Migration

**Version**: 1.1.0  
**Created**: 2026-02-02  
**Updated**: 2026-02-05  
**Status**: Active  
**Purpose**: Document all E2E test scenarios for HAPI (48 Python migration + 3 predictive signal mode)  

**Authority**:
- DD-TEST-006: Test Plan Policy
- BR-HAPI-002, BR-HAPI-197, BR-HAPI-250, BR-AUDIT-005, BR-AI-075/080/081, BR-STORAGE-013
- BR-AI-084: Predictive Signal Mode Prompt Strategy, ADR-054: Predictive Signal Mode Classification

---

## Overview

This test plan documents all 48 E2E test scenarios from the Python test suite to ensure complete 1:1 migration to Go. Each scenario specifies **business outcomes** (behavior + correctness + business impact), not just API contract validation.

**Migration Context**: Python E2E tests experiencing HTTP client timeout configuration issues across multiple layers. Go migration provides type safety, reliability, and consistency with other Kubernaut E2E tests.

**Test Philosophy**: Validate **business outcomes**, not just API contracts.
- ✅ GOOD: "When no workflow found → needs_human_review=true AND reason='no_matching_workflows'"  
- ❌ BAD: "Response has needs_human_review field"

---

## Test Scenario Naming Convention

**Format**: `E2E-HAPI-{SEQUENCE}`

- **E2E**: End-to-End test tier
- **HAPI**: HolmesGPT-API service
- **SEQUENCE**: Zero-padded 3-digit (001-048)

**Examples**:
- `E2E-HAPI-001` - Incident analysis no workflow found scenario
- `E2E-HAPI-013` - Recovery endpoint happy path
- `E2E-HAPI-045` - Audit pipeline LLM request event

---

## Test Scenarios: Complete Catalog (57 Total)

### Category A: Incident Analysis (12 scenarios)

#### E2E-HAPI-001: No Workflow Found Returns Human Review

**Business Requirement**: BR-HAPI-197

**Business Outcome**: When no matching workflow exists, system escalates to human operator with clear reason

**Preconditions**:
- HAPI service with Mock LLM
- ServiceAccount authentication configured
- Mock LLM scenario: `MOCK_NO_WORKFLOW_FOUND`

**Test Steps**:
1. Create IncidentRequest with `signal_type="MOCK_NO_WORKFLOW_FOUND"`
2. Call POST `/api/v1/incident/analyze`
3. Parse IncidentResponse

**Expected Results (Business Outcomes)**:
- **BEHAVIOR**: Response indicates human review required
  - `needs_human_review = true`
  - `human_review_reason = "no_matching_workflows"`
  - `selected_workflow = null`
- **CORRECTNESS**: Values are accurate
  - `confidence = 0.0` (no automation confidence)
  - `warnings` contains "MOCK_MODE"
- **BUSINESS IMPACT**: AIAnalysis controller:
  - Sets phase = "RequiresHumanReview"
  - Creates notification for operator
  - Does NOT create WorkflowExecution CRD

**Python Source**: `test_mock_llm_edge_cases_e2e.py::TestIncidentEdgeCases::test_no_workflow_found_returns_needs_human_review`

**Go Target**: `test/e2e/holmesgpt-api/incident_analysis_test.go`

---

#### E2E-HAPI-002: Low Confidence Returns Human Review with Alternatives

**Business Requirement**: BR-HAPI-197

**Business Outcome**: When confidence is low, system provides tentative recommendation but requires human decision

**Preconditions**:
- HAPI service with Mock LLM
- Mock LLM scenario: `MOCK_LOW_CONFIDENCE`

**Test Steps**:
1. Create IncidentRequest with `signal_type="MOCK_LOW_CONFIDENCE"`
2. Call POST `/api/v1/incident/analyze`
3. Parse IncidentResponse

**Expected Results**:
- **BEHAVIOR**: Uncertain recommendation escalated
  - `needs_human_review = true`
  - `human_review_reason = "low_confidence"`
  - `selected_workflow != null` (tentative)
- **CORRECTNESS**: Low confidence reflected
  - `selected_workflow.confidence < 0.5`
  - `alternative_workflows.length > 0` (options for human)
- **BUSINESS IMPACT**: Operator reviews options and selects manually

**Python Source**: `test_mock_llm_edge_cases_e2e.py::TestIncidentEdgeCases::test_low_confidence_returns_needs_human_review`

**Go Target**: `test/e2e/holmesgpt-api/incident_analysis_test.go`

---

#### E2E-HAPI-003: Max Retries Exhausted Returns Validation History

**Business Requirement**: BR-HAPI-197

**Business Outcome**: When LLM self-correction fails after max retries, provide complete validation history for debugging

**Preconditions**:
- Mock LLM scenario: `MOCK_MAX_RETRIES_EXHAUSTED`

**Test Steps**:
1. Create IncidentRequest with `signal_type="MOCK_MAX_RETRIES_EXHAUSTED"`
2. Call POST `/api/v1/incident/analyze`
3. Parse IncidentResponse and validation history

**Expected Results**:
- **BEHAVIOR**: AI gave up after max retries
  - `needs_human_review = true`
  - `human_review_reason = "llm_parsing_error"`
  - `selected_workflow = null`
- **CORRECTNESS**: Complete audit trail
  - `validation_attempts_history.length >= 3`
  - Each attempt has: `attempt`, `workflow_id`, `is_valid=false`, `errors`, `timestamp`
- **BUSINESS IMPACT**: Operator sees why AI failed, can debug or manually intervene

**Python Source**: `test_mock_llm_edge_cases_e2e.py::TestIncidentEdgeCases::test_max_retries_exhausted_returns_validation_history`

**Go Target**: `test/e2e/holmesgpt-api/incident_analysis_test.go`

---

#### E2E-HAPI-004: Normal Incident Analysis Succeeds (Happy Path)

**Business Requirement**: BR-HAPI-002

**Business Outcome**: Standard signal types produce confident workflow recommendations

**Preconditions**:
- HAPI with Mock LLM
- Test workflows seeded in DataStorage

**Test Steps**:
1. Create IncidentRequest with `signal_type="OOMKilled"`
2. Call POST `/api/v1/incident/analyze`
3. Parse IncidentResponse

**Expected Results**:
- **BEHAVIOR**: Confident recommendation provided
  - `needs_human_review = false`
  - `selected_workflow != null`
  - `confidence > 0.8`
- **CORRECTNESS**: Workflow matches signal type
  - `selected_workflow.workflow_id` contains "oomkill"
- **BUSINESS IMPACT**: AIAnalysis creates WorkflowExecution automatically

**Python Source**: `test_mock_llm_edge_cases_e2e.py::TestHappyPathComparison::test_normal_incident_analysis_succeeds`

**Go Target**: `test/e2e/holmesgpt-api/incident_analysis_test.go`

---

#### E2E-HAPI-005: Incident Response Structure Validation

**Business Requirement**: BR-AI-075

**Business Outcome**: Response contains all fields required by AIAnalysis controller

**Preconditions**:
- HAPI service running
- Test workflows seeded

**Test Steps**:
1. Create valid IncidentRequest
2. Call POST `/api/v1/incident/analyze`
3. Validate response structure

**Expected Results**:
- **BEHAVIOR**: Complete response structure
  - `incident_id` matches request
  - `analysis != null`
  - `root_cause_analysis != null`
  - `confidence` present
- **CORRECTNESS**: Values within valid ranges
  - `0.0 <= confidence <= 1.0`
- **BUSINESS IMPACT**: AIAnalysis can parse response without errors

**Python Source**: `test_workflow_selection_e2e.py::TestIncidentAnalysisE2E::test_incident_analysis_returns_valid_response_structure`

**Go Target**: `test/e2e/holmesgpt-api/incident_analysis_test.go`

---

#### E2E-HAPI-006: Incident with Enrichment Results Processing

**Business Requirement**: DD-HAPI-001 (Custom Labels Auto-Append)

**Business Outcome**: EnrichmentResults (detectedLabels, customLabels) influence workflow selection

**Preconditions**:
- HAPI with enrichment processing enabled
- Test workflows with label filters

**Test Steps**:
1. Create IncidentRequest with enrichment_results containing detectedLabels and customLabels
2. Call POST `/api/v1/incident/analyze`
3. Validate workflow selection considers labels

**Expected Results**:
- **BEHAVIOR**: Workflow selection influenced by labels
  - `selected_workflow != null`
- **CORRECTNESS**: Appropriate workflow for label context
- **BUSINESS IMPACT**: Workflows respect cluster constraints (GitOps, PDB, stateful)

**Python Source**: `test_workflow_selection_e2e.py::TestIncidentAnalysisE2E::test_incident_with_enrichment_results`

**Go Target**: `test/e2e/holmesgpt-api/incident_analysis_test.go`

---

#### E2E-HAPI-007: Invalid Request Returns Error

**Business Requirement**: BR-HAPI-200

**Business Outcome**: Invalid requests rejected with clear error messages

**Preconditions**:
- HAPI service running

**Test Steps**:
1. Create IncidentRequest with missing required fields
2. Call POST `/api/v1/incident/analyze`
3. Expect validation error

**Expected Results**:
- **BEHAVIOR**: Request rejected
  - Client-side validation error OR HTTP 422
- **CORRECTNESS**: Error indicates missing fields
- **BUSINESS IMPACT**: Caller knows what to fix

**Python Source**: `test_workflow_selection_e2e.py::TestErrorHandlingE2E::test_invalid_request_returns_error`

**Go Target**: `test/e2e/holmesgpt-api/incident_analysis_test.go`

---

#### E2E-HAPI-008: Missing Remediation ID Returns Error

**Business Requirement**: DD-WORKFLOW-002

**Business Outcome**: remediation_id is mandatory for audit trail correlation

**Preconditions**:
- HAPI service running

**Test Steps**:
1. Create IncidentRequest WITHOUT remediation_id
2. Call POST `/api/v1/incident/analyze`
3. Expect validation error

**Expected Results**:
- **BEHAVIOR**: Request rejected
  - Pydantic ValidationError OR HTTP 422
- **CORRECTNESS**: Error message mentions "remediation_id"
- **BUSINESS IMPACT**: Audit trail can correlate events

**Python Source**: `test_workflow_selection_e2e.py::TestErrorHandlingE2E::test_missing_remediation_id_returns_error`

**Go Target**: `test/e2e/holmesgpt-api/incident_analysis_test.go`

---

### Category B: Recovery Analysis (17 scenarios)

#### E2E-HAPI-013: Recovery Endpoint Happy Path

**Business Requirement**: BR-AI-080, BR-AI-081

**Business Outcome**: Recovery endpoint provides complete response for workflow selection after failure

**Preconditions**:
- HAPI with Mock LLM
- Previous execution context available

**Test Steps**:
1. Create RecoveryRequest with previous_execution context
2. Call POST `/api/v1/recovery/analyze`
3. Validate complete response

**Expected Results**:
- **BEHAVIOR**: Complete recovery response
  - `incident_id` matches request
  - `selected_workflow != null` (BR-AI-080)
  - `recovery_analysis != null` (BR-AI-081)
  - `can_recover != null`
  - `analysis_confidence != null`
- **CORRECTNESS**: Values logical
  - Recovery workflow differs from failed workflow
- **BUSINESS IMPACT**: AIAnalysis can create second WorkflowExecution with alternative approach

**Python Source**: `test_recovery_endpoint_e2e.py::TestRecoveryEndpointE2EHappyPath::test_recovery_endpoint_returns_complete_response_e2e`

**Go Target**: `test/e2e/holmesgpt-api/recovery_analysis_test.go`

---

#### E2E-HAPI-014: Recovery Response Field Types Validation

**Business Requirement**: BR-AI-080

**Business Outcome**: Response field types match OpenAPI spec for AIAnalysis parsing

**Preconditions**:
- HAPI service running
- Previous execution context

**Test Steps**:
1. Create RecoveryRequest
2. Call POST `/api/v1/recovery/analyze`
3. Validate field types

**Expected Results**:
- **BEHAVIOR**: Type-safe response
  - `incident_id` is string
  - `can_recover` is bool
  - `analysis_confidence` is float (0.0-1.0)
  - `selected_workflow.workflow_id` is string
  - `selected_workflow.confidence` is float
- **CORRECTNESS**: Values within ranges
- **BUSINESS IMPACT**: Type mismatches don't break AIAnalysis controller

**Python Source**: `test_recovery_endpoint_e2e.py::TestRecoveryEndpointE2EFieldValidation::test_recovery_response_has_correct_field_types_e2e`

**Go Target**: `test/e2e/holmesgpt-api/recovery_analysis_test.go`

---

#### E2E-HAPI-015: Recovery Processes Previous Execution Context

**Business Requirement**: BR-AI-081

**Business Outcome**: Recovery analysis uses previous failure details to avoid repeating same approach

**Preconditions**:
- Previous execution with failure details

**Test Steps**:
1. Create RecoveryRequest with previous_execution containing:
   - `failed_workflow_id`
   - `failure.reason`
   - `failure.message`
2. Call POST `/api/v1/recovery/analyze`
3. Validate recovery considers previous failure

**Expected Results**:
- **BEHAVIOR**: Recovery differs from initial attempt
  - `recovery_attempt_number = 2`
  - `selected_workflow != null`
  - `recovery_analysis` reflects previous failure analysis
- **CORRECTNESS**: Recovery strategy addresses previous failure reason
- **BUSINESS IMPACT**: System doesn't retry same failed approach

**Python Source**: `test_recovery_endpoint_e2e.py::TestRecoveryEndpointE2EPreviousExecution::test_recovery_processes_previous_execution_context_e2e`

**Go Target**: `test/e2e/holmesgpt-api/recovery_analysis_test.go`

---

#### E2E-HAPI-016: Recovery Uses Detected Labels for Workflow Selection

**Business Requirement**: DD-HAPI-001

**Business Outcome**: Cluster context (detectedLabels) influences recovery workflow selection

**Preconditions**:
- Previous execution context
- Enrichment results with detectedLabels

**Test Steps**:
1. Create RecoveryRequest with enrichment_results containing:
   - `detectedLabels.gitOpsManaged = true`
   - `detectedLabels.pdbProtected = true`
   - `detectedLabels.stateful = true`
2. Call POST `/api/v1/recovery/analyze`
3. Validate workflow respects constraints

**Expected Results**:
- **BEHAVIOR**: Workflow selection considers labels
  - `selected_workflow != null`
- **CORRECTNESS**: Workflow appropriate for stateful + PDB context
- **BUSINESS IMPACT**: Recovery doesn't violate cluster policies (e.g., no pod deletion if PDB protected)

**Python Source**: `test_recovery_endpoint_e2e.py::TestRecoveryEndpointE2EDetectedLabels::test_recovery_uses_detected_labels_for_workflow_selection_e2e`

**Go Target**: `test/e2e/holmesgpt-api/recovery_analysis_test.go`

---

#### E2E-HAPI-017: Recovery Mock Mode Produces Valid Responses

**Business Requirement**: BR-HAPI-212

**Business Outcome**: Mock LLM mode provides OpenAPI-compliant responses for testing

**Preconditions**:
- Mock LLM enabled
- Previous execution context

**Test Steps**:
1. Create RecoveryRequest
2. Call POST `/api/v1/recovery/analyze`
3. Validate mock response structure

**Expected Results**:
- **BEHAVIOR**: Complete mock response
  - `selected_workflow != null`
  - `recovery_analysis != null`
  - `can_recover != null`
- **CORRECTNESS**: Mock mode indicated
  - `warnings` contains "MOCK"
- **BUSINESS IMPACT**: Tests can run without real LLM costs

**Python Source**: `test_recovery_endpoint_e2e.py::TestRecoveryEndpointE2EMockMode::test_recovery_mock_mode_produces_valid_responses_e2e`

**Go Target**: `test/e2e/holmesgpt-api/recovery_analysis_test.go`

---

#### E2E-HAPI-018: Recovery Rejects Invalid Attempt Number

**Business Requirement**: BR-HAPI-200

**Business Outcome**: Invalid recovery attempt numbers rejected (must be >= 1)

**Preconditions**:
- HAPI service running

**Test Steps**:
1. Create RecoveryRequest with `recovery_attempt_number = 0`
2. Expect client-side validation error

**Expected Results**:
- **BEHAVIOR**: Request rejected before API call
  - Pydantic ValidationError
  - Error message mentions "recovery_attempt_number"
- **CORRECTNESS**: Validation enforces >= 1
- **BUSINESS IMPACT**: Invalid state prevented at source

**Python Source**: `test_recovery_endpoint_e2e.py::TestRecoveryEndpointE2EErrorScenarios::test_recovery_rejects_invalid_recovery_attempt_number_e2e`

**Go Target**: `test/e2e/holmesgpt-api/recovery_analysis_test.go`

---

#### E2E-HAPI-019: Recovery Without Previous Execution Context

**Business Requirement**: BR-AI-081

**Business Outcome**: Recovery attempts should have previous execution context (test API behavior)

**Preconditions**:
- HAPI service running

**Test Steps**:
1. Create RecoveryRequest with `is_recovery_attempt=true` but NO `previous_execution`
2. Call POST `/api/v1/recovery/analyze`
3. Check if accepted or rejected

**Expected Results**:
- **BEHAVIOR**: Either succeeds with default behavior OR rejects
  - If succeeds: `can_recover != null`
  - If rejects: HTTP 400/422
- **CORRECTNESS**: Consistent behavior documented
- **BUSINESS IMPACT**: Contract clarity for AIAnalysis team

**Python Source**: `test_recovery_endpoint_e2e.py::TestRecoveryEndpointE2EErrorScenarios::test_recovery_requires_previous_execution_for_recovery_attempts_e2e`

**Go Target**: `test/e2e/holmesgpt-api/recovery_analysis_test.go`

---

#### E2E-HAPI-020: Recovery Searches DataStorage for Workflows

**Business Requirement**: BR-STORAGE-013

**Business Outcome**: Recovery endpoint integrates with DataStorage for workflow catalog search

**Preconditions**:
- DataStorage service running
- Test workflows seeded
- Previous execution context

**Test Steps**:
1. Create RecoveryRequest
2. Call POST `/api/v1/recovery/analyze`
3. Validate workflow comes from DataStorage

**Expected Results**:
- **BEHAVIOR**: DataStorage queried for workflows
  - `selected_workflow != null`
  - `selected_workflow.workflow_id` is UUID (from DataStorage)
- **CORRECTNESS**: Workflow exists in DataStorage catalog
- **BUSINESS IMPACT**: Recovery workflows centrally managed in DataStorage

**Python Source**: `test_recovery_endpoint_e2e.py::TestRecoveryEndpointE2EDataStorageIntegration::test_recovery_searches_data_storage_for_workflows_e2e`

**Go Target**: `test/e2e/holmesgpt-api/recovery_analysis_test.go`

---

#### E2E-HAPI-021: Recovery Returns Executable Workflow Specification

**Business Requirement**: BR-AI-080

**Business Outcome**: Selected workflow has all fields required by WorkflowExecution controller

**Preconditions**:
- Previous execution context

**Test Steps**:
1. Create RecoveryRequest
2. Call POST `/api/v1/recovery/analyze`
3. Validate workflow.executability

**Expected Results**:
- **BEHAVIOR**: Executable workflow returned
  - `selected_workflow.workflow_id != null`
  - `selected_workflow.parameters != null`
  - `selected_workflow.confidence != null`
  - `selected_workflow.rationale != null`
- **CORRECTNESS**: Parameters is dict (not null/empty)
- **BUSINESS IMPACT**: WorkflowExecution can execute without additional lookups

**Python Source**: `test_recovery_endpoint_e2e.py::TestRecoveryEndpointE2EWorkflowValidation::test_recovery_returns_executable_workflow_specification_e2e`

**Go Target**: `test/e2e/holmesgpt-api/recovery_analysis_test.go`

---

#### E2E-HAPI-022: Complete Incident to Recovery Flow

**Business Requirement**: BR-AI-080, BR-AI-081

**Business Outcome**: End-to-end flow from incident → recovery simulates real AIAnalysis workflow

**Preconditions**:
- HAPI service running
- Test workflows seeded

**Test Steps**:
1. Call incident analyze endpoint
2. Capture initial workflow_id
3. Simulate workflow failure
4. Call recovery analyze endpoint with previous_execution
5. Validate recovery workflow returned

**Expected Results**:
- **BEHAVIOR**: Complete remediation lifecycle
  - Incident analysis succeeds
  - Recovery analysis succeeds
  - Recovery workflow may differ from initial
- **CORRECTNESS**: IDs correlate across lifecycle
- **BUSINESS IMPACT**: Validates complete AIAnalysis → WorkflowExecution → RemediationOrchestrator flow

**Python Source**: `test_recovery_endpoint_e2e.py::TestRecoveryEndpointE2EEndToEndFlow::test_complete_incident_to_recovery_flow_e2e`

**Go Target**: `test/e2e/holmesgpt-api/recovery_analysis_test.go`

---

#### E2E-HAPI-023: Recovery Edge Case - Signal Not Reproducible

**Business Requirement**: BR-HAPI-212

**Business Outcome**: When issue self-resolved, system indicates no action needed

**Preconditions**:
- Mock LLM scenario: `MOCK_NOT_REPRODUCIBLE`

**Test Steps**:
1. Create RecoveryRequest with `signal_type="MOCK_NOT_REPRODUCIBLE"`
2. Call POST `/api/v1/recovery/analyze`
3. Parse response

**Expected Results**:
- **BEHAVIOR**: No recovery needed
  - `can_recover = false` (issue resolved)
  - `needs_human_review = false` (no decision needed)
  - `selected_workflow = null`
- **CORRECTNESS**: High confidence issue resolved
  - `analysis_confidence > 0.8`
  - `recovery_analysis.previous_attempt_assessment.state_changed = true`
- **BUSINESS IMPACT**: AIAnalysis marks remediation as "self-resolved", no further action

**Python Source**: `test_mock_llm_edge_cases_e2e.py::TestRecoveryEdgeCases::test_signal_not_reproducible_returns_no_recovery`

**Go Target**: `test/e2e/holmesgpt-api/recovery_analysis_test.go`

---

#### E2E-HAPI-024: Recovery Edge Case - No Recovery Workflow Found

**Business Requirement**: BR-HAPI-197

**Business Outcome**: When no recovery workflow available, escalate to human

**Preconditions**:
- Mock LLM scenario: `MOCK_NO_WORKFLOW_FOUND`

**Test Steps**:
1. Create RecoveryRequest with `signal_type="MOCK_NO_WORKFLOW_FOUND"`
2. Call POST `/api/v1/recovery/analyze`
3. Parse response

**Expected Results**:
- **BEHAVIOR**: Human intervention required
  - `can_recover = true` (recovery possible manually)
  - `needs_human_review = true`
  - `human_review_reason = "no_matching_workflows"`
  - `selected_workflow = null`
- **CORRECTNESS**: Flags set correctly
- **BUSINESS IMPACT**: Operator must find manual solution

**Python Source**: `test_mock_llm_edge_cases_e2e.py::TestRecoveryEdgeCases::test_no_recovery_workflow_returns_human_review`

**Go Target**: `test/e2e/holmesgpt-api/recovery_analysis_test.go`

---

#### E2E-HAPI-025: Recovery Edge Case - Low Confidence Recovery

**Business Requirement**: BR-HAPI-197

**Business Outcome**: Low confidence recovery workflows require human approval

**Preconditions**:
- Mock LLM scenario: `MOCK_LOW_CONFIDENCE`

**Test Steps**:
1. Create RecoveryRequest with `signal_type="MOCK_LOW_CONFIDENCE"`
2. Call POST `/api/v1/recovery/analyze`
3. Validate uncertain recovery handling

**Expected Results**:
- **BEHAVIOR**: Tentative recovery with human review
  - `can_recover = true`
  - `needs_human_review = true`
  - `human_review_reason = "low_confidence"`
  - `selected_workflow != null` (tentative)
- **CORRECTNESS**: Low confidence reflected
  - `analysis_confidence < 0.5`
  - `selected_workflow.confidence < 0.5`
- **BUSINESS IMPACT**: Operator approves/rejects tentative recovery plan

**Python Source**: `test_mock_llm_edge_cases_e2e.py::TestRecoveryEdgeCases::test_low_confidence_recovery_returns_human_review`

**Go Target**: `test/e2e/holmesgpt-api/recovery_analysis_test.go`

---

#### E2E-HAPI-026: Normal Recovery Analysis Succeeds

**Business Requirement**: BR-HAPI-002

**Business Outcome**: Standard recovery scenarios produce confident recommendations

**Preconditions**:
- Mock LLM
- Previous execution context

**Test Steps**:
1. Create RecoveryRequest with `signal_type="CrashLoopBackOff"`
2. Call POST `/api/v1/recovery/analyze`
3. Validate happy path recovery

**Expected Results**:
- **BEHAVIOR**: Confident recovery recommendation
  - `can_recover = true`
  - `needs_human_review = false`
  - `selected_workflow != null`
  - `analysis_confidence > 0.7`
- **CORRECTNESS**: Recovery workflow appropriate for CrashLoopBackOff
- **BUSINESS IMPACT**: Automatic recovery execution without human approval

**Python Source**: `test_mock_llm_edge_cases_e2e.py::TestHappyPathComparison::test_normal_recovery_analysis_succeeds`

**Go Target**: `test/e2e/holmesgpt-api/recovery_analysis_test.go`

---

#### E2E-HAPI-027: Recovery Response Structure Validation

**Business Requirement**: BR-AI-080

**Business Outcome**: Recovery response has all required fields for AIAnalysis

**Preconditions**:
- Previous execution context

**Test Steps**:
1. Create RecoveryRequest
2. Call POST `/api/v1/recovery/analyze`
3. Validate response structure completeness

**Expected Results**:
- **BEHAVIOR**: Complete response structure
  - `incident_id != null`
  - `can_recover != null`
  - `strategies != null`
  - `analysis_confidence != null`
- **CORRECTNESS**: confidence in range 0.0-1.0
- **BUSINESS IMPACT**: AIAnalysis can process response without null checks

**Python Source**: `test_workflow_selection_e2e.py::TestRecoveryAnalysisE2E::test_recovery_analysis_returns_valid_response`

**Go Target**: `test/e2e/holmesgpt-api/recovery_analysis_test.go`

---

#### E2E-HAPI-028: Recovery with Previous Execution Context (Workflow Selection)

**Business Requirement**: DD-RECOVERY-003

**Business Outcome**: Recovery requests with previous context yield different strategies

**Preconditions**:
- Previous execution with failure

**Test Steps**:
1. Create RecoveryRequest with previous_execution
2. Call POST `/api/v1/recovery/analyze`
3. Validate strategies differ

**Expected Results**:
- **BEHAVIOR**: Strategies provided
  - `strategies != null`
- **CORRECTNESS**: Recovery considers previous failure
- **BUSINESS IMPACT**: Alternate approaches explored after initial failure

**Python Source**: `test_workflow_selection_e2e.py::TestRecoveryAnalysisE2E::test_recovery_with_previous_execution_context`

**Go Target**: `test/e2e/holmesgpt-api/recovery_analysis_test.go`

---

#### E2E-HAPI-029: Real LLM Recovery Analysis (Opt-in)

**Business Requirement**: BR-HAPI-RECOVERY-001 to 006

**Business Outcome**: Recovery analysis works with real LLM providers

**Preconditions**:
- Real LLM provider configured (requires credentials)
- `RUN_REAL_LLM=true`

**Test Steps**:
1. Create RecoveryRequest with real scenario
2. Call POST `/api/v1/recovery/analyze` (real LLM call)
3. Validate response

**Expected Results**:
- **BEHAVIOR**: Real LLM produces valid response
  - Response structure matches mock mode
- **CORRECTNESS**: Real LLM reasoning present
- **BUSINESS IMPACT**: Production readiness validated

**Python Source**: `test_real_llm_integration.py::TestRealRecoveryAnalysis::test_recovery_analysis_with_real_llm`

**Go Target**: `test/e2e/holmesgpt-api/real_llm_integration_test.go`

**Note**: Skip by default unless `RUN_REAL_LLM=true`

---

### Category C: Audit Pipeline (4 scenarios)

#### E2E-HAPI-045: LLM Request Event Persisted to DataStorage

**Business Requirement**: BR-AUDIT-005

**Business Outcome**: All LLM API calls are audited for compliance and debugging

**Preconditions**:
- HAPI with audit buffering enabled
- DataStorage service running
- ServiceAccount auth for DataStorage queries

**Test Steps**:
1. Create IncidentRequest with unique remediation_id
2. Call POST `/api/v1/incident/analyze`
3. Query DataStorage for audit events with retry (async buffering)
4. Filter for `event_type="llm_request"`

**Expected Results**:
- **BEHAVIOR**: LLM request event persisted
  - Event found in DataStorage within 15s
  - `event_type = "llm_request"`
  - `correlation_id` matches remediation_id
- **CORRECTNESS**: Event data complete
  - `event_data.incident_id` matches request
  - `event_data.prompt_length` OR `event_data.prompt_preview` present
- **BUSINESS IMPACT**: Compliance team can audit all LLM interactions

**Python Source**: `test_audit_pipeline_e2e.py::TestAuditPipelineE2E::test_llm_request_event_persisted`

**Go Target**: `test/e2e/holmesgpt-api/audit_pipeline_test.go`

**Implementation Notes**:
```go
// IncidentRequest uses plain strings (not OptString as of ogen v1.0+)
req := &hapiclient.IncidentRequest{
    IncidentID:        "test-audit-045",
    RemediationID:     remediationID,
    SignalType:        "OOMKilled",
    Severity:          "high",
    SignalSource:      "kubernetes",
    ResourceNamespace: "default",
    ResourceKind:      "Pod",
    ResourceName:      "test-pod-045",
    ErrorMessage:      "Container memory limit exceeded",
}

// Query DataStorage using QueryAuditEvents (not ListAuditEvents)
resp, err := dataStorageClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
    CorrelationID: ogenclient.NewOptString(remediationID),
})
// Access events via resp.Data (AuditEventsQueryResponse)
events := resp.Data
```

---

#### E2E-HAPI-046: LLM Response Event Persisted to DataStorage

**Business Requirement**: BR-AUDIT-005

**Business Outcome**: All LLM responses audited for cost tracking and analysis

**Preconditions**:
- Audit pipeline enabled
- DataStorage running

**Test Steps**:
1. Create IncidentRequest
2. Call POST `/api/v1/incident/analyze`
3. Query DataStorage for `event_type="llm_response"`

**Expected Results**:
- **BEHAVIOR**: LLM response event persisted
  - At least 1 llm_response event found
  - `correlation_id` matches remediation_id
- **CORRECTNESS**: Response data captured
  - `event_data.incident_id` matches
  - `event_data.has_analysis = true`
  - `event_data.analysis_length` present
- **BUSINESS IMPACT**: Cost analysis, quality monitoring, debugging

**Python Source**: `test_audit_pipeline_e2e.py::TestAuditPipelineE2E::test_llm_response_event_persisted`

**Go Target**: `test/e2e/holmesgpt-api/audit_pipeline_test.go`

**Implementation Notes**:
```go
// Same DataStorage query pattern as E2E-HAPI-045
resp, err := dataStorageClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
    CorrelationID: ogenclient.NewOptString(remediationID),
})
events := resp.Data

// Filter for event_type="llm_response"
for _, event := range events {
    if event.EventType == "llm_response" {
        // Found LLM response audit event
    }
}
```

---

#### E2E-HAPI-047: Validation Attempt Event Persisted

**Business Requirement**: DD-HAPI-002 v1.2

**Business Outcome**: Workflow validation attempts audited for quality analysis

**Preconditions**:
- Audit pipeline enabled
- DataStorage running

**Test Steps**:
1. Create IncidentRequest
2. Call POST `/api/v1/incident/analyze` (triggers validation)
3. Query DataStorage for `event_type="workflow_validation_attempt"`

**Expected Results**:
- **BEHAVIOR**: Validation events persisted
  - At least 1 validation event found
  - `correlation_id` matches remediation_id
- **CORRECTNESS**: Validation data complete
  - `event_data.attempt` present
  - `event_data.max_attempts` present
  - `event_data.is_valid` present
  - If multi-attempt: last has `is_final_attempt=true`
- **BUSINESS IMPACT**: Self-correction quality analysis, debugging failed validations

**Python Source**: `test_audit_pipeline_e2e.py::TestAuditPipelineE2E::test_validation_attempt_event_persisted`

**Go Target**: `test/e2e/holmesgpt-api/audit_pipeline_test.go`

---

#### E2E-HAPI-048: Complete Audit Trail Persisted

**Business Requirement**: BR-AUDIT-005

**Business Outcome**: Complete audit trail (all event types) available for incident forensics

**Preconditions**:
- Full audit pipeline enabled
- DataStorage running
- Test workflows seeded

**Test Steps**:
1. Create IncidentRequest
2. Call POST `/api/v1/incident/analyze`
3. Query DataStorage for all event types
4. Validate complete trail

**Expected Results**:
- **BEHAVIOR**: All event types present
  - `aiagent.llm.request` event found
  - `aiagent.llm.response` event found
  - (Optional: `aiagent.workflow.validation_attempt` if validation occurred)
- **CORRECTNESS**: Consistent correlation across events
  - All events have same `correlation_id` (remediation_id)
  - All have same `incident_id` in event_data
- **BUSINESS IMPACT**: Complete incident forensics, compliance reporting

**Python Source**: `test_audit_pipeline_e2e.py::TestAuditPipelineE2E::test_complete_audit_trail_persisted`

**Go Target**: `test/e2e/holmesgpt-api/audit_pipeline_test.go`

**Implementation Notes**:
```go
// Same DataStorage query pattern
resp, err := dataStorageClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
    CorrelationID: ogenclient.NewOptString(remediationID),
})
events := resp.Data

// Filter for event_type="workflow_validation_attempt"
for _, event := range events {
    if event.EventType == "workflow_validation_attempt" {
        // Validate event_data structure
    }
}
```

---

**Implementation Notes (E2E-HAPI-048)**:
```go
// Query all events by correlation_id
resp, err := dataStorageClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
    CorrelationID: ogenclient.NewOptString(remediationID),
})
events := resp.Data

// Validate complete trail
hasLLMRequest := false
hasLLMResponse := false
for _, event := range events {
    if event.EventType == "llm_request" {
        hasLLMRequest = true
    }
    if event.EventType == "llm_response" {
        hasLLMResponse = true
    }
}

// Assert both required event types present
Expect(hasLLMRequest).To(BeTrue())
Expect(hasLLMResponse).To(BeTrue())
```

---

### Category D: Workflow Catalog (15 scenarios)

#### E2E-HAPI-030: Semantic Search with Exact Match

**Business Requirement**: BR-STORAGE-013

**Business Outcome**: Workflow catalog finds workflows by semantic similarity to incident description

**Preconditions**:
- DataStorage with pgvector
- Test workflows seeded
- Embeddings generated

**Test Steps**:
1. Create workflow catalog tool
2. Call `invoke(query="OOMKilled critical", top_k=5)`
3. Validate results

**Expected Results**:
- **BEHAVIOR**: Successful semantic search
  - `status = SUCCESS`
  - `workflows.length > 0`
- **CORRECTNESS**: Results ranked by confidence
  - `workflows[0].confidence >= workflows[1].confidence`
  - Top result `signal_type = "OOMKilled"`
  - workflow_id is UUID format
  - All workflows have required fields per DD-WORKFLOW-002 v3.0
- **BUSINESS IMPACT**: LLM finds relevant workflows without exact keyword matching

**Python Source**: `test_workflow_catalog_data_storage_integration.py::TestWorkflowCatalogEndToEnd::test_semantic_search_with_exact_match_br_storage_013`

**Go Target**: `test/e2e/holmesgpt-api/workflow_catalog_test.go`

---

#### E2E-HAPI-031: Confidence Scoring Validation

**Business Requirement**: DD-WORKFLOW-004 v2.0

**Business Outcome**: Workflows ranked by V1.0 base similarity scoring (no boost/penalty yet)

**Preconditions**:
- Test workflows seeded

**Test Steps**:
1. Search for "OOMKilled critical"
2. Get multiple results
3. Validate confidence sorting

**Expected Results**:
- **BEHAVIOR**: Results sorted by confidence descending
  - `workflows[i].confidence >= workflows[i+1].confidence` for all i
- **CORRECTNESS**: Confidence values valid
  - All `0.0 <= confidence <= 1.0`
- **BUSINESS IMPACT**: LLM sees most relevant workflows first

**Python Source**: `test_workflow_catalog_data_storage_integration.py::TestWorkflowCatalogEndToEnd::test_confidence_scoring_dd_workflow_004_v1`

**Go Target**: `test/e2e/holmesgpt-api/workflow_catalog_test.go`

---

#### E2E-HAPI-032: Empty Results Handling

**Business Requirement**: BR-HAPI-250

**Business Outcome**: No matching workflows returns empty array (not error)

**Preconditions**:
- DataStorage running

**Test Steps**:
1. Search for non-existent signal type
2. Expect SUCCESS with empty results

**Expected Results**:
- **BEHAVIOR**: Graceful empty results
  - `status = SUCCESS` (not ERROR)
  - `workflows = []` (empty array, not null)
- **CORRECTNESS**: Response format valid
- **BUSINESS IMPACT**: LLM can tell operator "No automated remediation available"

**Python Source**: `test_workflow_catalog_data_storage_integration.py::TestWorkflowCatalogEndToEnd::test_empty_results_handling_br_hapi_250`

**Go Target**: `test/e2e/holmesgpt-api/workflow_catalog_test.go`

---

#### E2E-HAPI-033: Filter Validation

**Business Requirement**: DD-LLM-001

**Business Outcome**: Mandatory label filters correctly narrow search results

**Preconditions**:
- Test workflows with different signal_types

**Test Steps**:
1. Search for "CrashLoopBackOff high"
2. Validate only matching workflows returned

**Expected Results**:
- **BEHAVIOR**: Filtered results
  - Only CrashLoopBackOff workflows (if any)
  - No OOMKilled workflows returned
- **CORRECTNESS**: signal_type is singular string (DD-WORKFLOW-002 v3.0)
- **BUSINESS IMPACT**: Workflows match incident type (no irrelevant suggestions)

**Python Source**: `test_workflow_catalog_data_storage_integration.py::TestWorkflowCatalogEndToEnd::test_filter_validation_dd_llm_001`

**Go Target**: `test/e2e/holmesgpt-api/workflow_catalog_test.go`

---

#### E2E-HAPI-034: Top-K Limiting

**Business Requirement**: BR-HAPI-250

**Business Outcome**: Tool respects result count limit (prevents LLM context overflow)

**Preconditions**:
- Test workflows seeded

**Test Steps**:
1. Search with `top_k=1`
2. Validate result count

**Expected Results**:
- **BEHAVIOR**: Result count limited
  - `workflows.length <= 1`
- **CORRECTNESS**: Most relevant workflow returned first
  - If 1 result: `confidence > 0`
- **BUSINESS IMPACT**: LLM doesn't get overwhelmed with too many options

**Python Source**: `test_workflow_catalog_data_storage_integration.py::TestWorkflowCatalogEndToEnd::test_top_k_limiting_br_hapi_250`

**Go Target**: `test/e2e/holmesgpt-api/workflow_catalog_test.go`

---

#### E2E-HAPI-035: Error Handling - Service Unavailable

**Business Requirement**: BR-STORAGE-013

**Business Outcome**: Tool handles DataStorage unavailability gracefully

**Preconditions**:
- Tool configured with invalid DataStorage URL

**Test Steps**:
1. Configure tool with `url="http://invalid:99999"`
2. Execute search
3. Validate error handling

**Expected Results**:
- **BEHAVIOR**: Error status returned (not exception)
  - `status = ERROR`
  - `error` message present
- **CORRECTNESS**: Error indicates service issue
  - Message contains "data storage" OR "connect" OR "failed"
- **BUSINESS IMPACT**: LLM can inform operator "workflow search unavailable"

**Python Source**: `test_workflow_catalog_data_storage_integration.py::TestWorkflowCatalogEndToEnd::test_error_handling_service_unavailable_br_storage_013`

**Go Target**: `test/e2e/holmesgpt-api/workflow_catalog_test.go`

---

#### E2E-HAPI-036: Critical User Journey - OOMKilled Finds Memory Workflow

**Business Requirement**: BR-STORAGE-013

**Business Outcome**: Complete user journey - AI finds OOMKilled remediation workflow

**Preconditions**:
- Test workflows bootstrapped

**Test Steps**:
1. Simulate LLM completing RCA for OOMKilled
2. Call workflow catalog search
3. Validate top result addresses memory

**Expected Results**:
- **BEHAVIOR**: Relevant workflow found
  - `workflows.length > 0`
  - Top workflow has title, description, confidence
- **CORRECTNESS**: Workflow addresses memory issues
  - `signal_type` in ["OOMKilled", "MemoryPressure", "ResourceQuota"]
  - `workflow_id` is UUID
- **BUSINESS IMPACT**: Operator presented with actionable remediation

**Python Source**: `test_workflow_catalog_e2e.py::TestCriticalUserJourneys::test_oomkilled_incident_finds_memory_workflow_e1_1`

**Go Target**: `test/e2e/holmesgpt-api/workflow_catalog_test.go`

---

#### E2E-HAPI-037: Critical User Journey - CrashLoop Finds Restart Workflow

**Business Requirement**: BR-STORAGE-013

**Business Outcome**: AI finds CrashLoopBackOff remediation workflow

**Preconditions**:
- Test workflows bootstrapped

**Test Steps**:
1. Simulate LLM RCA for CrashLoopBackOff
2. Search workflow catalog
3. Validate restart workflow found

**Expected Results**:
- **BEHAVIOR**: Relevant workflow found
  - `workflows.length > 0`
  - Workflow has title, description, positive confidence
- **CORRECTNESS**: Workflow addresses restart issues
- **BUSINESS IMPACT**: Operator has automated restart remediation option

**Python Source**: `test_workflow_catalog_e2e.py::TestCriticalUserJourneys::test_crashloop_incident_finds_restart_workflow_e1_2`

**Go Target**: `test/e2e/holmesgpt-api/workflow_catalog_test.go`

---

#### E2E-HAPI-038: AI Handles No Matching Workflows Gracefully

**Business Requirement**: BR-HAPI-250

**Business Outcome**: AI handles "no automated solution" scenario without errors

**Preconditions**:
- DataStorage running

**Test Steps**:
1. Search for non-existent incident type
2. Validate graceful empty results

**Expected Results**:
- **BEHAVIOR**: Graceful empty results
  - `status = SUCCESS` (not ERROR)
  - `workflows = []`
- **CORRECTNESS**: Response structure valid
- **BUSINESS IMPACT**: AI informs operator "No workflows found for this incident"

**Python Source**: `test_workflow_catalog_e2e.py::TestEdgeCaseUserJourneys::test_ai_handles_no_matching_workflows`

**Go Target**: `test/e2e/holmesgpt-api/workflow_catalog_test.go`

---

#### E2E-HAPI-039: AI Can Refine Search with Keywords

**Business Requirement**: BR-HAPI-250

**Business Outcome**: AI can perform broad search then refine with specific terms

**Preconditions**:
- Test workflows seeded

**Test Steps**:
1. Broad search: `query="memory"`
2. Refined search: `query="OOMKilled pod memory limit exceeded critical kubernetes"`
3. Compare results

**Expected Results**:
- **BEHAVIOR**: Both searches succeed
  - Broad: `status = SUCCESS`
  - Refined: `status = SUCCESS`
- **CORRECTNESS**: Results may differ in specificity
- **BUSINESS IMPACT**: AI can iteratively narrow search to find best workflow

**Python Source**: `test_workflow_catalog_e2e.py::TestEdgeCaseUserJourneys::test_ai_can_refine_search`

**Go Target**: `test/e2e/holmesgpt-api/workflow_catalog_test.go`

---

#### E2E-HAPI-040: DataStorage Returns container_image in Search

**Business Requirement**: BR-AI-075

**Business Outcome**: Workflow search results include container_image for WorkflowExecution

**Preconditions**:
- Test workflows with container_image registered
- DataStorage running

**Test Steps**:
1. Search for "OOMKilled critical"
2. Validate container_image field in results

**Expected Results**:
- **BEHAVIOR**: container_image included
  - All workflows have `container_image` field
- **CORRECTNESS**: OCI format if present
  - Contains "/" (registry/repo)
  - Has ":tag" OR "@sha256:" 
- **BUSINESS IMPACT**: WorkflowExecution can pull and execute container without additional lookups

**Python Source**: `test_workflow_catalog_container_image_integration.py::TestWorkflowCatalogContainerImageIntegration::test_data_storage_returns_container_image_in_search`

**Go Target**: `test/e2e/holmesgpt-api/workflow_catalog_test.go`

---

#### E2E-HAPI-041: DataStorage Returns container_digest in Search

**Business Requirement**: BR-AI-075

**Business Outcome**: Workflow results include immutable digest for security

**Preconditions**:
- Test workflows with container_digest
- DataStorage running

**Test Steps**:
1. Search for "OOMKilled critical"
2. Validate container_digest field

**Expected Results**:
- **BEHAVIOR**: container_digest included
  - All workflows have `container_digest` field
- **CORRECTNESS**: SHA256 format if present
  - Matches pattern: `sha256:[a-f0-9]{64}`
- **BUSINESS IMPACT**: WorkflowExecution uses immutable digest (security requirement)

**Python Source**: `test_workflow_catalog_container_image_integration.py::TestWorkflowCatalogContainerImageIntegration::test_data_storage_returns_container_digest_in_search`

**Go Target**: `test/e2e/holmesgpt-api/workflow_catalog_test.go`

---

#### E2E-HAPI-042: End-to-End Container Image Flow

**Business Requirement**: BR-AI-075

**Business Outcome**: Complete flow from search to container_image extraction validated

**Preconditions**:
- Test workflows seeded

**Test Steps**:
1. Search for "OOMKilled critical"
2. Validate all required fields in results
3. Confirm container_image and container_digest present

**Expected Results**:
- **BEHAVIOR**: Complete workflow data
  - Required fields per DD-WORKFLOW-002 v3.0: workflow_id, title, description, signal_type, confidence, container_image, container_digest
- **CORRECTNESS**: Field types correct
  - container_image: string or null
  - container_digest: string or null
  - confidence: numeric (0.0-1.0)
- **BUSINESS IMPACT**: AIAnalysis has all data to create WorkflowExecution CRD

**Python Source**: `test_workflow_catalog_container_image_integration.py::TestWorkflowCatalogContainerImageIntegration::test_end_to_end_container_image_flow`

**Go Target**: `test/e2e/holmesgpt-api/workflow_catalog_test.go`

---

#### E2E-HAPI-043: Container Image Matches Catalog Entry

**Business Requirement**: BR-AI-075

**Business Outcome**: Returned container_image has valid OCI format

**Preconditions**:
- Test workflows seeded

**Test Steps**:
1. Search for workflows
2. For each result with container_image, validate OCI format

**Expected Results**:
- **BEHAVIOR**: Valid OCI references
  - At least 1 workflow with container_image found
- **CORRECTNESS**: OCI format validated
  - Contains "/" (registry/repo)
  - Has ":tag" OR "@sha256:" suffix
- **BUSINESS IMPACT**: Container runtime can pull images without format errors

**Python Source**: `test_workflow_catalog_container_image_integration.py::TestWorkflowCatalogContainerImageIntegration::test_container_image_matches_catalog_entry`

**Go Target**: `test/e2e/holmesgpt-api/workflow_catalog_test.go`

---

#### E2E-HAPI-044: Direct API Search Returns Container Image

**Business Requirement**: BR-AI-075

**Business Outcome**: DataStorage API contract includes container_image (validates tool transformation)

**Preconditions**:
- DataStorage running
- Test workflows seeded

**Test Steps**:
1. Call DataStorage API directly (bypass tool)
2. POST `/api/v1/workflows/search` with filters
3. Validate container_image in raw response

**Expected Results**:
- **BEHAVIOR**: API response includes container_image
  - `workflows[i].container_image` attribute present
  - `workflows[i].container_digest` attribute present
- **CORRECTNESS**: DD-WORKFLOW-002 v3.0 flat structure
  - Fields directly on workflow object (not nested)
- **BUSINESS IMPACT**: Tool transformation not required for container_image (API provides it)

**Python Source**: `test_workflow_catalog_container_image_integration.py::TestWorkflowCatalogContainerImageDirectAPI::test_direct_api_search_returns_container_image`

**Go Target**: `test/e2e/holmesgpt-api/workflow_catalog_test.go`

---

## Migration Tracking Matrix

| Test ID | Category | Business Outcome | Python Source | Go Target | Status |
|---------|----------|------------------|---------------|-----------|--------|
| E2E-HAPI-001 | Incident | No workflow → human review | test_mock_llm_edge_cases_e2e.py:121 | incident_analysis_test.go | Not Started |
| E2E-HAPI-002 | Incident | Low confidence → human review | test_mock_llm_edge_cases_e2e.py:153 | incident_analysis_test.go | Not Started |
| E2E-HAPI-003 | Incident | Max retries → validation history | test_mock_llm_edge_cases_e2e.py:189 | incident_analysis_test.go | Not Started |
| E2E-HAPI-004 | Incident | Normal analysis succeeds | test_mock_llm_edge_cases_e2e.py:332 | incident_analysis_test.go | Not Started |
| E2E-HAPI-005 | Incident | Response structure validation | test_workflow_selection_e2e.py:217 | incident_analysis_test.go | Not Started |
| E2E-HAPI-006 | Incident | Enrichment results processing | test_workflow_selection_e2e.py:246 | incident_analysis_test.go | Not Started |
| E2E-HAPI-007 | Incident | Invalid request error | test_workflow_selection_e2e.py:342 | incident_analysis_test.go | Not Started |
| E2E-HAPI-008 | Incident | Missing remediation_id error | test_workflow_selection_e2e.py:364 | incident_analysis_test.go | Not Started |
| E2E-HAPI-013 | Recovery | Recovery endpoint happy path | test_recovery_endpoint_e2e.py:130 | recovery_analysis_test.go | Not Started |
| E2E-HAPI-014 | Recovery | Response field types | test_recovery_endpoint_e2e.py:186 | recovery_analysis_test.go | Not Started |
| E2E-HAPI-015 | Recovery | Previous execution processing | test_recovery_endpoint_e2e.py:238 | recovery_analysis_test.go | Not Started |
| E2E-HAPI-016 | Recovery | Detected labels influence | test_recovery_endpoint_e2e.py:279 | recovery_analysis_test.go | Not Started |
| E2E-HAPI-017 | Recovery | Mock mode validation | test_recovery_endpoint_e2e.py:322 | recovery_analysis_test.go | Not Started |
| E2E-HAPI-018 | Recovery | Invalid attempt number error | test_recovery_endpoint_e2e.py:363 | recovery_analysis_test.go | Not Started |
| E2E-HAPI-019 | Recovery | Missing previous execution | test_recovery_endpoint_e2e.py:384 | recovery_analysis_test.go | Not Started |
| E2E-HAPI-020 | Recovery | DataStorage integration | test_recovery_endpoint_e2e.py:419 | recovery_analysis_test.go | Not Started |
| E2E-HAPI-021 | Recovery | Executable workflow spec | test_recovery_endpoint_e2e.py:456 | recovery_analysis_test.go | Not Started |
| E2E-HAPI-022 | Recovery | Complete incident→recovery flow | test_recovery_endpoint_e2e.py:505 | recovery_analysis_test.go | Not Started |
| E2E-HAPI-023 | Recovery | Signal not reproducible | test_mock_llm_edge_cases_e2e.py:234 | recovery_analysis_test.go | Not Started |
| E2E-HAPI-024 | Recovery | No recovery workflow | test_mock_llm_edge_cases_e2e.py:271 | recovery_analysis_test.go | Not Started |
| E2E-HAPI-025 | Recovery | Low confidence recovery | test_mock_llm_edge_cases_e2e.py:298 | recovery_analysis_test.go | Not Started |
| E2E-HAPI-026 | Recovery | Normal recovery succeeds | test_mock_llm_edge_cases_e2e.py:354 | recovery_analysis_test.go | Not Started |
| E2E-HAPI-027 | Recovery | Response structure | test_workflow_selection_e2e.py:280 | recovery_analysis_test.go | Not Started |
| E2E-HAPI-028 | Recovery | Previous context strategies | test_workflow_selection_e2e.py:308 | recovery_analysis_test.go | Not Started |
| E2E-HAPI-029 | Recovery | Real LLM integration (opt-in) | test_real_llm_integration.py:128 | real_llm_integration_test.go | Not Started |
| E2E-HAPI-030 | Catalog | Semantic search exact match | test_workflow_catalog_data_storage_integration.py:181 | workflow_catalog_test.go | Not Started |
| E2E-HAPI-031 | Catalog | Confidence scoring | test_workflow_catalog_data_storage_integration.py:252 | workflow_catalog_test.go | Not Started |
| E2E-HAPI-032 | Catalog | Empty results handling | test_workflow_catalog_data_storage_integration.py:303 | workflow_catalog_test.go | Not Started |
| E2E-HAPI-033 | Catalog | Filter validation | test_workflow_catalog_data_storage_integration.py:335 | workflow_catalog_test.go | Not Started |
| E2E-HAPI-034 | Catalog | Top-K limiting | test_workflow_catalog_data_storage_integration.py:384 | workflow_catalog_test.go | Not Started |
| E2E-HAPI-035 | Catalog | Error handling service unavailable | test_workflow_catalog_data_storage_integration.py:428 | workflow_catalog_test.go | Not Started |
| E2E-HAPI-036 | Catalog | OOMKilled user journey | test_workflow_catalog_e2e.py:85 | workflow_catalog_test.go | Not Started |
| E2E-HAPI-037 | Catalog | CrashLoop user journey | test_workflow_catalog_e2e.py:151 | workflow_catalog_test.go | Not Started |
| E2E-HAPI-038 | Catalog | No matches graceful | test_workflow_catalog_e2e.py:218 | workflow_catalog_test.go | Not Started |
| E2E-HAPI-039 | Catalog | Search refinement | test_workflow_catalog_e2e.py:244 | workflow_catalog_test.go | Not Started |
| E2E-HAPI-040 | Catalog | container_image in search | test_workflow_catalog_container_image_integration.py:100 | workflow_catalog_test.go | Not Started |
| E2E-HAPI-041 | Catalog | container_digest in search | test_workflow_catalog_container_image_integration.py:152 | workflow_catalog_test.go | Not Started |
| E2E-HAPI-042 | Catalog | End-to-end container flow | test_workflow_catalog_container_image_integration.py:206 | workflow_catalog_test.go | Not Started |
| E2E-HAPI-043 | Catalog | Container image OCI format | test_workflow_catalog_container_image_integration.py:278 | workflow_catalog_test.go | Not Started |
| E2E-HAPI-044 | Catalog | Direct API container_image | test_workflow_catalog_container_image_integration.py:338 | workflow_catalog_test.go | Not Started |
| E2E-HAPI-045 | Audit | LLM request event persisted | test_audit_pipeline_e2e.py:350 | audit_pipeline_test.go | Not Started |
| E2E-HAPI-046 | Audit | LLM response event persisted | test_audit_pipeline_e2e.py:425 | audit_pipeline_test.go | Not Started |
| E2E-HAPI-047 | Audit | Validation attempt persisted | test_audit_pipeline_e2e.py:492 | audit_pipeline_test.go | Not Started |
| E2E-HAPI-048 | Audit | Complete audit trail | test_audit_pipeline_e2e.py:573 | audit_pipeline_test.go | Not Started |

**Note**: Real LLM integration tests (E2E-HAPI-029 + 6 more scenarios from `test_real_llm_integration.py`) are opt-in and will be documented in a separate section if needed.

---

## Test Execution Strategy

### Infrastructure Requirements

**All Tests Require**:
- Kind cluster with HAPI + DataStorage + Mock LLM
- PostgreSQL + Redis
- ServiceAccount authentication configured
- Test workflows seeded (10 workflows covering staging + production environments)

**Setup Command**: `make test-e2e-holmesgpt-api`

**Infrastructure Pattern**: AA E2E (Phase 4a-4e):
1. Deploy DataStorage infrastructure
2. Wait for DataStorage ready
3. Seed workflows → capture UUIDs
4. Deploy Mock LLM with ConfigMap (real UUIDs)
5. Deploy HAPI

### Go Test Organization

**Files** (7 total):
1. `incident_analysis_test.go` - E2E-HAPI-001 to 008 (8 scenarios)
2. `recovery_analysis_test.go` - E2E-HAPI-013 to 029 (17 scenarios)
3. `workflow_catalog_test.go` - E2E-HAPI-030 to 044 (15 scenarios)
4. `audit_pipeline_test.go` - E2E-HAPI-045 to 048 (4 scenarios)
5. `mock_llm_edge_cases_test.go` - (Subset of above, grouped by Mock LLM scenarios)
6. `workflow_selection_test.go` - (Subset of above, grouped by workflow selection logic)
7. `real_llm_integration_test.go` - (Opt-in real LLM tests)

**Note**: Files 5-7 may be merged into Files 1-4 for simpler organization.

### Test Pattern (Ginkgo/Gomega)

```go
var _ = Describe("E2E-HAPI-001: No Workflow Found Returns Human Review", Label("e2e"), func() {
    Context("BR-HAPI-197: Human review scenarios", func() {
        It("should return needs_human_review when no matching workflow exists", func() {
            // Arrange: Create request with MOCK_NO_WORKFLOW_FOUND
            req := &hapiogen.IncidentRequest{
                IncidentID:     hapiogen.NewOptString("test-edge-001"),
                RemediationID:  hapiogen.NewOptString("test-rem-001"),
                SignalType:     hapiogen.NewOptString("MOCK_NO_WORKFLOW_FOUND"),
                Severity:       hapiogen.NewOptString("high"),
                SignalSource:   hapiogen.NewOptString("prometheus"),
                // ... other required fields
            }
            
            // Act: Call HAPI
            resp, err := hapiClient.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePost(ctx, req)
            Expect(err).ToNot(HaveOccurred())
            
            // Assert: Business outcome validation
            incidentResp, ok := resp.(*hapiogen.IncidentResponse)
            Expect(ok).To(BeTrue(), "Expected IncidentResponse type")
            
            // BEHAVIOR: Human review required
            Expect(incidentResp.NeedsHumanReview.Value).To(BeTrue(), 
                "needs_human_review must be true when no workflow found")
            Expect(incidentResp.HumanReviewReason.Value).To(Equal("no_matching_workflows"),
                "human_review_reason must indicate no matching workflows")
            Expect(incidentResp.SelectedWorkflow.Set).To(BeFalse(),
                "selected_workflow must be null when no workflow found")
            
            // CORRECTNESS: Zero confidence
            Expect(incidentResp.Confidence.Value).To(BeNumerically("==", 0.0),
                "confidence must be 0.0 when no automation possible")
            
            // CORRECTNESS: Mock mode indicated
            Expect(incidentResp.Warnings.Value).To(ContainElement(ContainSubstring("MOCK_MODE")),
                "warnings must indicate mock mode")
                
            // BUSINESS IMPACT: (verified by integration tests - AIAnalysis sets RequiresHumanReview phase)
        })
    })
})
```

---

## Validation Strategy: Correctness Over Range Checks

**Version**: 2.0 (Updated 2026-02-02)  
**Improvement**: Enhanced validation to test **ACCURACY** instead of generic range checks

### Anti-Pattern Eliminated: NULL-TESTING

**Problem**: Original tests checked generic ranges without validating correctness:
```go
// ❌ WEAK: Only checks range, not accuracy
Expect(incidentResp.Confidence.Value).To(BeNumerically(">=", 0.0))
Expect(incidentResp.Confidence.Value).To(BeNumerically("<=", 1.0))
```

**Solution**: Validate EXACT Mock LLM behavior:
```go
// ✅ CORRECT: Tests actual business logic accuracy
Expect(incidentResp.Confidence.Value).To(BeNumerically("~", 0.95, 0.05),
    "Mock LLM 'oomkilled' scenario returns confidence = 0.95 ± 0.05 (server.py:88)")
```

### Mock LLM Reference Values

From `test/services/mock-llm/src/server.py` MOCK_SCENARIOS:

| Mock Scenario | Signal Type | Expected Confidence | Source Line |
|---------------|-------------|---------------------|-------------|
| oomkilled | OOMKilled | 0.95 ± 0.05 | server.py:88 |
| crashloop | CrashLoopBackOff | 0.88 ± 0.05 | server.py:102 |
| no_workflow_found | MOCK_NO_WORKFLOW_FOUND | 0.0 (exact) | server.py:157 |
| low_confidence | MOCK_LOW_CONFIDENCE | 0.35 ± 0.05 | server.py:171 |
| problem_resolved | MOCK_NOT_REPRODUCIBLE | 0.85 ± 0.05 | server.py:185 |
| recovery | (alternative workflow) | 0.85 ± 0.05 | server.py:130 |
| default | (fallback) | 0.75 ± 0.05 | server.py:217 |

### Validation Approach by Test Category

#### Incident/Recovery Tests (High Precision)
Tests that interact with Mock LLM **directly** validate EXACT confidence values:

```go
// E2E-HAPI-002: Low Confidence
Expect(selectedWorkflow.Confidence.Value).To(BeNumerically("~", 0.35, 0.05),
    "Mock LLM 'low_confidence' scenario returns confidence = 0.35 ± 0.05")

// E2E-HAPI-004: OOMKilled
Expect(incidentResp.Confidence.Value).To(BeNumerically("~", 0.95, 0.05),
    "Mock LLM 'oomkilled' scenario returns confidence = 0.95 ± 0.05")
```

**Rationale**: Mock LLM returns deterministic values we KNOW and can test for accuracy.

#### Workflow Catalog Tests (Semantic Search)
Tests that query DataStorage's **pgvector semantic search** use threshold-based validation:

```go
// E2E-HAPI-030: Semantic Search
Expect(selectedWorkflow.Confidence.Value).To(And(
    BeNumerically(">", 0.0),
    BeNumerically("<=", 1.0)),
    "semantic search confidence must be in valid range (0, 1]")

// E2E-HAPI-031: High Confidence Match
Expect(selectedWorkflow.Confidence.Value).To(And(
    BeNumerically(">", 0.5),
    BeNumerically("<=", 1.0)),
    "high-confidence semantic search result must be > 0.5 for exact signal match")
```

**Rationale**: Vector similarity scores vary based on embedding model and data. We validate:
1. Valid range (0, 1]
2. Reasonable thresholds based on query type
3. Structural correctness (workflow_id, title, container_image)

**Thresholds Explained**:
- Exact signal match: `> 0.5` (high semantic similarity expected)
- Detected labels: `> 0.3` (moderate similarity with label hints)
- Generic search: `> 0.0` (any positive match)

### Scenarios Updated with Exact Validations

**Incident Analysis** (5 scenarios):
- E2E-HAPI-002: Low confidence → 0.35 ± 0.05
- E2E-HAPI-003: Max retries → exactly 3 attempts
- E2E-HAPI-004: OOMKilled → 0.95 ± 0.05 + exact workflow title
- E2E-HAPI-005: CrashLoop → 0.88 ± 0.05

**Recovery Analysis** (5 scenarios):
- E2E-HAPI-014: Normal recovery → 0.85 ± 0.05
- E2E-HAPI-023: Problem resolved → 0.85 ± 0.05
- E2E-HAPI-025: Low confidence → 0.35 ± 0.05
- E2E-HAPI-026: Confident recovery → 0.85 ± 0.05
- E2E-HAPI-027: With previous execution → 0.85 ± 0.05

**Workflow Catalog** (6 scenarios):
- E2E-HAPI-030: Semantic search → valid range + title validation
- E2E-HAPI-031: Confidence scoring → > 0.5 threshold
- E2E-HAPI-034: Top-k limiting → exact Mock LLM confidence 0.95
- E2E-HAPI-037: Detected labels → > 0.3 threshold + title match
- E2E-HAPI-038: Mock mode → valid range + title validation
- E2E-HAPI-043: Container metadata → positive confidence + image format

**Audit Pipeline** (4 scenarios):
- E2E-HAPI-045 to E2E-HAPI-048: DataStorage integration
  - **Fix Applied**: IncidentRequest uses plain strings (not OptString)
  - **Fix Applied**: Use `QueryAuditEvents` instead of `ListAuditEvents`
  - **Fix Applied**: Access events via `resp.Data` (AuditEventsQueryResponse)
  - Validates async audit buffering and event persistence

### Benefits of New Validation Approach

1. **Accuracy Testing**: Validates business logic correctness, not just "is non-zero"
2. **Debugging**: When tests fail, clear indication of WHICH Mock LLM scenario is broken
3. **Documentation**: Test assertions self-document expected Mock LLM behavior
4. **Regression Detection**: Exact values catch unintended Mock LLM changes
5. **Anti-Pattern Compliance**: Eliminates NULL-TESTING and WEAK ASSERTIONS
6. **API Alignment**: Test code reflects actual OpenAPI client structure (plain strings, correct methods)

---

## OpenAPI Client API Alignment (2026-02-02)

**Context**: During migration, OpenAPI Go client structure changed. All test files updated to match current API.

### IncidentRequest Structure

**Fields**:
```go
IncidentRequest{
    IncidentID:        "test-001",        // plain string
    RemediationID:     "test-rem-001",    // plain string
    SignalType:        "OOMKilled",       // plain string
    Severity:          "high",            // plain string
    SignalSource:      "kubernetes",      // plain string
    ResourceNamespace: "default",         // plain string
    ResourceKind:      "Pod",             // plain string
    ResourceName:      "test-pod",        // plain string
    ErrorMessage:      "Container OOM",   // plain string
}
```

**Changes Applied**:
- ✅ All fields are plain `string` (not `OptString`)
- ✅ Removed deprecated: `SignalTimestamp`, `TargetResource`
- ✅ Added resource context: `ResourceNamespace`, `ResourceKind`, `ResourceName`, `ErrorMessage`

### RecoveryRequest Structure

**Fields**:
```go
RecoveryRequest{
    IncidentID:            "test-013",                                    // plain string
    RemediationID:         "test-rem-013",                                // plain string
    SignalType:            hapiclient.NewOptNilString("CrashLoopBackOff"), // OptNilString
    Severity:              hapiclient.NewOptNilString("high"),             // OptNilString
    IsRecoveryAttempt:     hapiclient.NewOptBool(true),                    // OptBool
    RecoveryAttemptNumber: hapiclient.NewOptNilInt(2),                     // OptNilInt
    PreviousExecution:     hapiclient.NewOptNilPreviousExecution(...),     // OptNilPreviousExecution
}
```

**Changes Applied**:
- ✅ `IncidentID`, `RemediationID` are plain `string`
- ✅ `SignalType`, `Severity` are `OptNilString` (use `NewOptNilString()`)
- ✅ Optional fields use `NewOptBool()`, `NewOptNilInt()`, `NewOptNilPreviousExecution()`

### IncidentResponse/RecoveryResponse

**Confidence Fields**:
```go
// ✅ Confidence is plain float64 (not OptFloat64)
incidentResp.Confidence                  // NOT .Confidence.Value
recoveryResp.AnalysisConfidence          // NOT .AnalysisConfidence.Value

// ✅ Other response fields
incidentResp.SelectedWorkflow.Set        // OptNil check
incidentResp.Warnings                    // []string (plain)
incidentResp.AlternativeWorkflows        // []AlternativeWorkflow (plain)
```

**Changes Applied**:
- ✅ Removed `.Value` access from plain `float64` fields
- ✅ `Warnings`, `AlternativeWorkflows`, `ValidationAttemptsHistory` are plain slices

### PreviousExecution Structure

**Correct Structure**:
```go
PreviousExecution{
    WorkflowExecutionRef: "we-123",               // plain string
    SelectedWorkflow: SelectedWorkflowSummary{     // plain struct
        WorkflowID:     "workflow-crashloop",
        Version:        "v1.0.0",
        ContainerImage: "ghcr.io/.../image:v1.0.0",
        Rationale:      "High confidence match",
    },
    Failure: ExecutionFailure{                     // plain struct
        ExitCode:    1,
        FailureStep: "apply-fix",
        ErrorOutput: "kubectl apply failed",
    },
    NaturalLanguageSummary: hapiclient.NewOptNilString("Workflow failed at step apply-fix"),
}
```

**Changes Applied**:
- ✅ Removed `WorkflowID` field (doesn't exist)
- ✅ Use `SelectedWorkflow.WorkflowID` instead
- ✅ `Failure` is `ExecutionFailure` struct (not `OptFailureDetails`)

### DataStorage Client API

**Query Method**:
```go
// ✅ Use QueryAuditEvents (not ListAuditEvents)
resp, err := dataStorageClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
    EventType:  ogenclient.NewOptString("llm_request"),
    Limit:      ogenclient.NewOptInt(10),
    Offset:     ogenclient.NewOptInt(0),
    StartTime:  ogenclient.NewOptDateTime(time.Now().Add(-1 * time.Hour)),
})

// ✅ Access events via .Data field
events := resp.Data  // []ogenclient.AuditEvent
```

**Changes Applied**:
- ✅ `ListAuditEvents` → `QueryAuditEvents`
- ✅ `listResp.Events` → `resp.Data` (AuditEventsQueryResponse)

### ValidationAttempt Structure

**Field Access**:
```go
for _, attempt := range incidentResp.ValidationAttemptsHistory {
    Expect(attempt.Attempt).To(Equal(1))           // plain int
    Expect(attempt.IsValid).To(BeFalse())          // plain bool
    Expect(attempt.Errors).ToNot(BeEmpty())        // []string
    Expect(attempt.Timestamp).ToNot(BeEmpty())     // plain string
}
```

**Changes Applied**:
- ✅ All fields are plain types (not Opt* wrappers)
- ✅ Removed `.Value` access

### Files Updated (391 errors → 0)

1. **incident_analysis_test.go** - 17 errors fixed
2. **recovery_analysis_test.go** - 229 errors fixed
3. **workflow_catalog_test.go** - 145 errors fixed
4. **audit_pipeline_test.go** - Initial fixes applied

---

## Success Criteria

Migration complete when:
- [ ] All 54 test scenarios migrated to Go (48 original + 6 new Mock LLM scenarios)
- [ ] All Go tests passing (100% pass rate)
- [ ] Business outcome assertions present in every test
- [ ] Test IDs referenced in test descriptions
- [ ] 6 new Mock LLM scenarios added to `test/services/mock-llm/src/server.py`
- [ ] No Python E2E test files remaining (except opt-in real LLM performance test)
- [ ] Test execution time ≤5 minutes
- [ ] BR coverage maintained (all BRs mapped)

---

### Category F: Advanced Recovery Scenarios (Mock LLM) (6 scenarios)

**Context**: These scenarios were originally in Python's `test_real_llm_integration.py` but can be implemented with Mock LLM for free. They test advanced recovery analysis patterns without requiring actual LLM API calls.

---

#### E2E-HAPI-049: Multi-Step Recovery with State Preservation

**Business Requirement**: BR-HAPI-RECOVERY-001 to 006, BR-WF-RECOVERY-001 to 011

**Business Outcome**: LLM analyzes recovery for multi-step workflow with partial completion, preserving successful step changes

**Preconditions**:
- HAPI service with Mock LLM
- Mock LLM scenario: `MOCK_MULTI_STEP_RECOVERY`
- ServiceAccount authentication configured

**Test Steps**:
1. Create RecoveryRequest with multi-step workflow context:
   - Step 1 (increase_memory_limit): ✅ Completed (Memory increased 1Gi → 2Gi)
   - Step 2 (scale_deployment): ❌ Failed (InsufficientResources)
   - Step 3 (validate_health): ⏳ Pending
2. Include `previous_execution` with Step 1 success details
3. Call POST `/api/v1/recovery/analyze`
4. Parse RecoveryResponse

**Expected Results (Business Outcomes)**:
- **BEHAVIOR**: Recovery strategy addresses Step 2 failure while preserving Step 1
  - `can_recover = true`
  - `strategies.length >= 1`
  - Primary strategy does NOT rollback Step 1 (memory increase preserved)
- **CORRECTNESS**: Mock LLM returns deterministic recovery confidence
  - `analysis_confidence = 0.85 ± 0.05` (from MOCK_MULTI_STEP_RECOVERY scenario)
  - Strategy addresses capacity issue (add_nodes, scale_down, enable_autoscaler)
  - Rationale mentions "Step 1 successful" or "preserve memory increase"
- **BUSINESS IMPACT**: WorkflowExecution controller:
  - Attempts recovery from Step 2 (not full rollback)
  - Maintains Step 1 changes (memory stays at 2Gi)
  - Continues to Step 3 after Step 2 recovery

**Mock LLM Scenario** (to be added to `test/services/mock-llm/src/server.py`):
```python
"multi_step_recovery": {
    "confidence": 0.85,
    "can_recover": True,
    "strategies": [{
        "action_type": "enable_autoscaler",
        "confidence": 0.85,
        "rationale": "Step 1 memory increase successful. Step 2 failed due to cluster capacity. Enable cluster autoscaler to add nodes for scaling.",
        "estimated_risk": "low"
    }]
}
```

**Python Source**: `test_real_llm_integration.py::TestRealRecoveryAnalysis::test_multi_step_recovery_analysis`

**Go Target**: `test/e2e/holmesgpt-api/recovery_analysis_test.go`

---

#### E2E-HAPI-050: Cascading Failure Root Cause Analysis

**Business Requirement**: BR-HAPI-RECOVERY-001 to 006, BR-WF-INVESTIGATION-001 to 005

**Business Outcome**: LLM identifies root cause in cascading failures (memory leak) vs. symptoms (OOM, crashes)

**Preconditions**:
- HAPI service with Mock LLM
- Mock LLM scenario: `MOCK_CASCADING_FAILURE`
- ServiceAccount authentication configured

**Test Steps**:
1. Create RecoveryRequest with cascading failure pattern:
   - Initial: HighMemoryUsage (25m ago)
   - Symptom 1: OOMKilled (20m ago, 3 times)
   - Attempted Fix: restart_pod (13m ago) → Failed (OOMKilled again after 12min)
   - Symptom 2: CrashLoopBackOff (10m ago)
2. Include investigation result showing memory leak pattern:
   - Memory grows from 512Mi → 2Gi in exactly 12 minutes (constant rate)
   - Pattern repeats after each restart (rules out load-based)
3. Call POST `/api/v1/recovery/analyze`
4. Parse RecoveryResponse

**Expected Results (Business Outcomes)**:
- **BEHAVIOR**: LLM identifies root cause and recommends appropriate strategy
  - `can_recover = true`
  - `strategies.length >= 1`
  - Strategy does NOT recommend simple restart (already failed)
  - Strategy addresses memory leak (increase_memory, rollback, enable_profiling)
- **CORRECTNESS**: Mock LLM returns deterministic confidence for cascading scenarios
  - `analysis_confidence = 0.75 ± 0.05` (cascading failures have more uncertainty)
  - Strategy action type: "increase_memory", "rollback_deployment", or "enable_memory_profiling"
  - Rationale mentions "leak", "pattern", or "recurring issue"
- **BUSINESS IMPACT**: WorkflowExecution controller:
  - Does NOT attempt simple restart (learned from previous failure)
  - Implements memory-based or rollback strategy
  - Provides debugging context for future remediation

**Mock LLM Scenario** (to be added):
```python
"cascading_failure": {
    "confidence": 0.75,
    "can_recover": True,
    "strategies": [{
        "action_type": "increase_memory_limit",
        "confidence": 0.75,
        "rationale": "Memory leak detected. Constant growth rate (50MB/min) repeats after restart. Increase memory limit as temporary mitigation while root cause investigation continues.",
        "estimated_risk": "medium"
    }, {
        "action_type": "rollback_deployment",
        "confidence": 0.70,
        "rationale": "Memory leak pattern indicates application bug. Rollback to last known good version to restore service.",
        "estimated_risk": "low"
    }]
}
```

**Python Source**: `test_real_llm_integration.py::TestRealRecoveryAnalysis::test_cascading_failure_recovery_analysis`

**Go Target**: `test/e2e/holmesgpt-api/recovery_analysis_test.go`

---

#### E2E-HAPI-051: Recovery Near Attempt Limit (Conservative Strategy)

**Business Requirement**: BR-WF-RECOVERY-001 (max 3 attempts), BR-HAPI-RECOVERY-001 to 006

**Business Outcome**: LLM recommends most conservative strategy (rollback) when near attempt limit

**Preconditions**:
- HAPI service with Mock LLM
- Mock LLM scenario: `MOCK_NEAR_ATTEMPT_LIMIT`
- ServiceAccount authentication configured

**Test Steps**:
1. Create RecoveryRequest with near-limit context:
   - Recovery attempts: 2 of 3 (final attempt)
   - Attempt 1: restart_deployment → Failed (database connection error)
   - Attempt 2: increase_connection_pool → Failed (file descriptor exhaustion)
   - Business impact: $50K/minute revenue loss
2. Include investigation showing database migration broke compatibility
3. Call POST `/api/v1/recovery/analyze`
4. Parse RecoveryResponse

**Expected Results (Business Outcomes)**:
- **BEHAVIOR**: LLM recommends conservative rollback strategy for final attempt
  - `can_recover = true`
  - `strategies.length >= 1`
  - Primary strategy is conservative: "rollback_deployment", "rollback_database", or "manual_intervention"
- **CORRECTNESS**: High confidence for reliable rollback strategy
  - `analysis_confidence = 0.90 ± 0.05` (rollback is reliable)
  - Strategy action type: "rollback_deployment" or "rollback_database"
  - Rationale mentions "final attempt", "conservative", or "reliable"
- **BUSINESS IMPACT**: WorkflowExecution controller:
  - Uses most reliable strategy (rollback) for final attempt
  - Restores service quickly to minimize revenue loss
  - Escalates to human if rollback also fails

**Mock LLM Scenario** (to be added):
```python
"near_attempt_limit": {
    "confidence": 0.90,
    "can_recover": True,
    "strategies": [{
        "action_type": "rollback_deployment",
        "confidence": 0.90,
        "rationale": "This is the final recovery attempt (2 of 3 exhausted). Both forward fixes failed with different errors. Conservative rollback to last known good version is most reliable strategy to restore service.",
        "estimated_risk": "low"
    }]
}
```

**Python Source**: `test_real_llm_integration.py::TestRealRecoveryAnalysis::test_recovery_near_attempt_limit`

**Go Target**: `test/e2e/holmesgpt-api/recovery_analysis_test.go`

---

#### E2E-HAPI-052: Noisy Neighbor Resource Contention Detection

**Business Requirement**: BR-HAPI-RECOVERY-001 to 006, BR-PERF-020 (resource management)

**Business Outcome**: LLM identifies noisy neighbor issue and recommends cluster-level resource management

**Preconditions**:
- HAPI service with Mock LLM
- Mock LLM scenario: `MOCK_NOISY_NEIGHBOR`
- ServiceAccount authentication configured

**Test Steps**:
1. Create RecoveryRequest with resource contention context:
   - Database service degraded (query latency: 50ms → 1200ms)
   - ML batch job in different namespace consuming 90%+ node CPU
   - Database pods experiencing CPU throttling
   - No resource quotas or priority classes enforced
2. Call POST `/api/v1/recovery/analyze`
3. Parse RecoveryResponse

**Expected Results (Business Outcomes)**:
- **BEHAVIOR**: LLM identifies noisy neighbor and recommends resource isolation
  - `can_recover = true`
  - `strategies.length >= 1`
  - Strategy addresses resource contention: "set_resource_quotas", "set_priority_classes", "apply_node_affinity"
- **CORRECTNESS**: Mock LLM returns deterministic confidence
  - `analysis_confidence = 0.80 ± 0.05`
  - Strategy action type includes "quota", "priority", or "affinity"
  - Rationale mentions "noisy neighbor", "resource contention", or "isolation"
- **BUSINESS IMPACT**: WorkflowExecution controller:
  - Implements cluster-level resource management (not just pod-level)
  - Prevents future noisy neighbor issues
  - Maintains multi-tenant fairness

**Mock LLM Scenario** (to be added):
```python
"noisy_neighbor": {
    "confidence": 0.80,
    "can_recover": True,
    "strategies": [{
        "action_type": "set_resource_quotas",
        "confidence": 0.80,
        "rationale": "Noisy neighbor detected. ML batch job consuming excessive resources on same nodes as database. Set resource quotas for ml-workloads namespace to enforce fairness.",
        "estimated_risk": "low"
    }, {
        "action_type": "set_priority_classes",
        "confidence": 0.75,
        "rationale": "Database is P0 service but lacks priority class. Set high priority for database pods to ensure scheduling preference during contention.",
        "estimated_risk": "low"
    }]
}
```

**Python Source**: `test_real_llm_integration.py::TestRealRecoveryAnalysis::test_multitenant_resource_contention_recovery`

**Go Target**: `test/e2e/holmesgpt-api/recovery_analysis_test.go`

---

#### E2E-HAPI-053: Network Partition Split-Brain Detection

**Business Requirement**: BR-HAPI-RECOVERY-001 to 006, BR-ORCH-018 (multi-cluster)

**Business Outcome**: LLM identifies network partition and recommends partition-aware recovery strategies

**Preconditions**:
- HAPI service with Mock LLM
- Mock LLM scenario: `MOCK_NETWORK_PARTITION`
- ServiceAccount authentication configured

**Test Steps**:
1. Create RecoveryRequest with network partition context:
   - Deployment pods not reaching Ready state (3 of 5 replicas stuck)
   - Network partition isolating 3 nodes (unreachable for 8+ minutes)
   - Healthy side: 2 nodes with 2 running pods
   - Split-brain risk if partition heals with conflicting state
2. Call POST `/api/v1/recovery/analyze`
3. Parse RecoveryResponse

**Expected Results (Business Outcomes)**:
- **BEHAVIOR**: LLM recommends partition-aware recovery
  - `can_recover = true`
  - `strategies.length >= 1`
  - Strategy is partition-aware: "wait_for_partition_heal", "drain_partition_nodes", "force_reschedule_to_healthy_nodes"
- **CORRECTNESS**: Mock LLM returns lower confidence for partition scenarios
  - `analysis_confidence = 0.70 ± 0.05` (network partitions have high uncertainty)
  - Strategy action type includes "wait", "drain", or "reschedule"
  - Rationale mentions "partition", "split-brain", or "network"
- **BUSINESS IMPACT**: WorkflowExecution controller:
  - Avoids exacerbating split-brain issues
  - Waits for partition heal or drains affected nodes safely
  - Maintains service consistency

**Mock LLM Scenario** (to be added):
```python
"network_partition": {
    "confidence": 0.70,
    "can_recover": True,
    "strategies": [{
        "action_type": "wait_for_partition_heal",
        "confidence": 0.70,
        "rationale": "Network partition detected (3 nodes unreachable for 8+ minutes). Wait for partition to heal before taking action to avoid split-brain scenario. Monitor partition status.",
        "estimated_risk": "medium"
    }, {
        "action_type": "drain_partition_nodes",
        "confidence": 0.65,
        "rationale": "If partition persists, drain affected nodes and reschedule pods to healthy side of cluster. Risk: service disruption during drain.",
        "estimated_risk": "medium"
    }]
}
```

**Python Source**: `test_real_llm_integration.py::TestRealRecoveryAnalysis::test_network_partition_recovery`

**Go Target**: `test/e2e/holmesgpt-api/recovery_analysis_test.go`

---

#### E2E-HAPI-054: Basic Recovery Analysis Pattern (Simple)

**Business Requirement**: BR-HAPI-RECOVERY-001 to 006

**Business Outcome**: Basic recovery analysis for common failure scenarios

**Preconditions**:
- HAPI service with Mock LLM
- Mock LLM scenario: `MOCK_RECOVERY_BASIC`
- ServiceAccount authentication configured

**Test Steps**:
1. Create RecoveryRequest with simple failure:
   - Failed action: restart_pod
   - Failure context: OOMKilled
   - No cascading failures or complex patterns
2. Call POST `/api/v1/recovery/analyze`
3. Parse RecoveryResponse

**Expected Results (Business Outcomes)**:
- **BEHAVIOR**: Straightforward recovery recommendation
  - `can_recover = true`
  - `strategies.length >= 1`
  - Strategy addresses root cause directly
- **CORRECTNESS**: High confidence for simple scenarios
  - `analysis_confidence = 0.85 ± 0.05`
  - Strategy action type: "increase_memory" or similar direct fix
  - Rationale explains root cause and solution
- **BUSINESS IMPACT**: WorkflowExecution controller implements recovery quickly

**Mock LLM Scenario** (to be added):
```python
"recovery_basic": {
    "confidence": 0.85,
    "can_recover": True,
    "strategies": [{
        "action_type": "increase_memory",
        "confidence": 0.85,
        "rationale": "Container killed due to out of memory. Increase memory limit from current 512Mi to 1Gi to prevent OOMKilled errors.",
        "estimated_risk": "low"
    }]
}
```

**Python Source**: `test_real_llm_integration.py::TestRealRecoveryAnalysis::test_recovery_analysis_with_real_llm`

**Go Target**: `test/e2e/holmesgpt-api/recovery_analysis_test.go`

---

### Category G: Predictive Signal Mode (3 scenarios)

**Business Requirements**: BR-AI-084 (Predictive Signal Mode Prompt Strategy)
**Architecture**: ADR-054 (Predictive Signal Mode Classification)
**Go Target**: `test/e2e/holmesgpt-api/predictive_signal_mode_test.go`

#### E2E-HAPI-055: Predictive OOMKill Returns Predictive-Aware Analysis

**Business Requirement**: BR-AI-084

**Business Outcome**: When signal_mode=predictive, HAPI adapts its 5-phase investigation prompt to perform preemptive analysis instead of reactive RCA. The Mock LLM detects predictive keywords in the prompt and returns the `oomkilled_predictive` scenario with prevention-focused root cause.

**Preconditions**:
- HAPI service with Mock LLM
- ServiceAccount authentication configured
- Mock LLM scenario: `oomkilled_predictive`

**Test Steps**:
1. Create IncidentRequest with `signal_type="OOMKilled"` (normalized by SP from PredictedOOMKill)
2. Set `signal_mode="predictive"`
3. Call POST `/api/v1/incident/analyze`
4. Parse IncidentResponse

**Expected Results (Business Outcomes)**:
- **BEHAVIOR**: Predictive-aware analysis returned
  - Analysis text contains predictive language ("Predict", "Preemptive", "trend")
  - Workflow selected (oomkill-increase-memory-v1)
- **CORRECTNESS**: Confidence matches predictive scenario
  - `confidence = 0.88 +/- 0.05`
  - `selected_workflow` is set and not null
- **BUSINESS IMPACT**: AIAnalysis controller can adapt phase handling for predictive signals

**Mock LLM Scenario**: `oomkilled_predictive` (server.py:226)

**Status**: ✅ Passed

---

#### E2E-HAPI-056: Reactive Signal Mode Returns Standard RCA

**Business Requirement**: BR-AI-084 (backwards compatibility)

**Business Outcome**: Existing reactive requests continue working unchanged. signal_mode=reactive produces standard RCA results.

**Preconditions**:
- HAPI service with Mock LLM
- ServiceAccount authentication configured
- Mock LLM scenario: `oomkilled` (standard)

**Test Steps**:
1. Create IncidentRequest with `signal_type="OOMKilled"`
2. Set `signal_mode="reactive"` (explicit)
3. Call POST `/api/v1/incident/analyze`
4. Parse IncidentResponse

**Expected Results (Business Outcomes)**:
- **BEHAVIOR**: Standard reactive RCA response
  - Non-empty analysis text
  - Workflow selected
- **CORRECTNESS**: Positive confidence
  - `confidence > 0.0`
  - `selected_workflow` is set
- **BUSINESS IMPACT**: Existing reactive flow unchanged by ADR-054

**Status**: ✅ Passed

---

#### E2E-HAPI-057: Missing Signal Mode Defaults to Reactive

**Business Requirement**: BR-AI-084 (default behavior)

**Business Outcome**: Requests without signal_mode should default to reactive behavior, ensuring backwards compatibility with pre-ADR-054 clients.

**Preconditions**:
- HAPI service with Mock LLM
- ServiceAccount authentication configured
- Mock LLM scenario: `oomkilled` (standard, no predictive keywords in prompt)

**Test Steps**:
1. Create IncidentRequest with `signal_type="OOMKilled"`
2. Do NOT set `signal_mode` (field omitted)
3. Call POST `/api/v1/incident/analyze`
4. Parse IncidentResponse

**Expected Results (Business Outcomes)**:
- **BEHAVIOR**: Same as explicit reactive mode
  - Non-empty analysis text
  - Workflow selected
- **CORRECTNESS**: Positive confidence
  - `confidence > 0.0`
  - `selected_workflow` is set
- **BUSINESS IMPACT**: Pre-ADR-054 clients continue working without modification

**Status**: ✅ Passed

---

## Notes

### Real LLM Integration Tests (Opt-in)

**Scenarios Migrated to Mock LLM** (6 tests - **now run for FREE** in Category F):
- ✅ E2E-HAPI-049: Multi-step recovery (previously `test_multi_step_recovery_analysis`)
- ✅ E2E-HAPI-050: Cascading failure (previously `test_cascading_failure_recovery_analysis`)
- ✅ E2E-HAPI-051: Near attempt limit (previously `test_recovery_near_attempt_limit`)
- ✅ E2E-HAPI-052: Noisy neighbor (previously `test_multitenant_resource_contention_recovery`)
- ✅ E2E-HAPI-053: Network partition (previously `test_network_partition_recovery`)
- ✅ E2E-HAPI-054: Basic recovery (previously `test_recovery_analysis_with_real_llm`)

**Remaining Real LLM Tests** (3 tests - **require `RUN_REAL_LLM=true` + costs**):
- ❌ LLM error handling (`test_llm_handles_invalid_input_gracefully`) - Not LLM-specific, just API validation
- ❌ LLM timeout handling (`test_llm_timeout_handling`) - Doesn't actually test timeouts, just checks health endpoint
- ✅ Performance testing (`test_recovery_analysis_performance`) - **TRUE requirement**: Validates 90s SLA with real LLM

**Analysis**:
- **6 scenarios migrated**: All business logic can now be tested for free with Mock LLM
- **2 scenarios not needed**: Error handling and timeout tests don't actually require real LLM
- **1 scenario still valuable**: Performance SLA validation requires real LLM API calls

**Execution**: Remaining test requires `RUN_REAL_LLM=true` and LLM provider credentials

**Authority**: BR-HAPI-026 to 030, BR-HAPI-RECOVERY-001 to 006, BR-PERF-020, BR-ORCH-018

**Recommendation**: Focus on the 57 Mock LLM scenarios (free, including 3 predictive signal mode). Performance test is optional for SLA validation.

---

## References

- **Python Test Suite**: `holmesgpt-api/tests/e2e/`
- **Go Test Suite Target**: `test/e2e/holmesgpt-api/`
- **Infrastructure**: `test/infrastructure/holmesgpt_api.go`
- **Migration Plan**: `.cursor/plans/hapi_e2e_go_migration_*.plan.md`
- **Template**: `V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md`

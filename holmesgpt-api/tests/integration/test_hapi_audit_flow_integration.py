"""
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
"""

"""
Flow-Based Audit Integration Tests for HAPI

Business Requirement: BR-AUDIT-005 - HAPI MUST generate audit traces
Authority: TESTING_GUIDELINES.md - Flow-based audit testing pattern

✅ CORRECT PATTERN (Integration Tests - Direct Business Logic):
1. Import and call business logic functions directly (no HTTP)
2. Wait for processing (ADR-038: buffered audit flush)
3. Verify audit events emitted via Data Storage API
4. Validate audit event content

This matches Go service integration testing:
- Go: Call controller.Reconcile() directly (no CRD, no HTTP)
- Python: Call analyze_incident() directly (no HTTP, no API client)

❌ ANTI-PATTERN (FORBIDDEN):
1. Use HTTP client in integration tests (that's E2E testing)
2. Manually create audit events
3. Directly call Data Storage API to store events
4. Test audit infrastructure, not business logic

Reference Implementations:
- SignalProcessing: test/integration/signalprocessing/audit_integration_test.go (direct Reconcile calls)
- Gateway: test/integration/gateway/audit_integration_test.go (direct handler calls)
- HAPI E2E: holmesgpt-api/tests/e2e/ (HTTP API testing - future)

Architecture Decision (Jan 4, 2026):
- Integration tests: Call business logic directly (like Go services)
- E2E tests: Use HTTP API + OpenAPI client (future implementation)
"""

import os
import time
import pytest
import asyncio
from typing import List, Dict, Any, Optional

# ========================================
# BUSINESS LOGIC IMPORTS (Direct Calls)
# ========================================
# Import business logic functions directly - NO HTTP client!
# This matches Go service integration testing pattern:
#   Go: controller.Reconcile(ctx, req)
#   Python: analyze_incident(request_data)
import sys
from pathlib import Path

# Add src/ to path for business logic imports
sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from src.extensions.incident.llm_integration import analyze_incident
from src.extensions.recovery.llm_integration import analyze_recovery

# Data Storage client for audit validation (external dependency)
sys.path.insert(0, str(Path(__file__).parent.parent / 'clients'))
from datastorage import ApiClient as DataStorageApiClient, Configuration as DataStorageConfiguration
from datastorage.api.audit_write_api_api import AuditWriteAPIApi
from datastorage.models.audit_event import AuditEvent


# ========================================
# FIXTURES
# ========================================

# Note: data_storage_url fixture is provided by conftest.py (session-scoped)
# This allows audit validation to work correctly
# Tests call business logic DIRECTLY (no hapi_url needed)


# ========================================
# HELPER FUNCTIONS
# ========================================

def query_audit_events_with_retry(
    audit_store,
    correlation_id: str,
    event_category: str,
    event_type: Optional[str] = None,
    min_expected_events: int = 1,
    timeout_seconds: int = 30,
    poll_interval: float = 1.0,  # Increased from 0.5s to 1.0s to match DataStorage batch flush interval
    limit: int = 100
) -> List[AuditEvent]:
    """
    Query Data Storage for audit events with explicit flush support.

    This function eliminates async race conditions by flushing the audit buffer
    before querying, similar to Go integration tests.

    Pattern: flush() → poll with Eventually() → assert
    
    Timeout Alignment (Feb 1, 2026):
    - Polling: 30s (matches Go AIAnalysis INT: Eventually(30*time.Second, 1*time.Second))
    - Poll interval: 1.0s (increased from 0.5s to match DataStorage 1-second batch flush interval)
    - Flush: 10s (matches Go Gateway/AuthWebhook suite_test.go)
    
    Pattern Difference from Go:
    - Go AIAnalysis: Flush on EACH retry (controller may buffer during polling)
    - HAPI Python: Flush ONCE (analyze_incident() completes before polling)
    Rationale: Direct function calls complete before polling, so single flush is sufficient.
    
    Architectural Fixes (Jan 31, 2026 - User Feedback):
    1. ✅ No direct DS access: Uses audit_store._audit_api (not separate DS client)
    2. ✅ Proper filtering: Requires event_category (mandatory) + event_type (optional)
    3. ✅ Pagination: Supports limit parameter (default 100, max 1000)
    4. ✅ Correct event_category: "aiagent" (AI Agent Provider, not "analysis")

    Args:
        audit_store: BufferedAuditStore instance (MANDATORY per ADR-032)
        correlation_id: Correlation ID for audit correlation
        event_category: Event category (ADR-034 mandatory field, "aiagent" for HAPI)
        event_type: Optional event type filter (e.g., "aiagent.response.complete")
        min_expected_events: Minimum number of events expected (default 1)
        timeout_seconds: Maximum time to wait for events (default 30s, aligned with Go)
        poll_interval: Time between polling attempts (default 1.0s, matches DataStorage batch flush)
        limit: Maximum events per query (1-1000, default 100 for pagination)

    Returns:
        List of AuditEvent Pydantic models

    Raises:
        AssertionError: If events don't appear within timeout or audit_store not provided
    """
    # ADR-032: Audit is MANDATORY - fail if audit_store not provided
    if audit_store is None:
        raise AssertionError(
            "audit_store is required (ADR-032: Audit is MANDATORY). "
            "Ensure audit_store fixture is provided to test."
        )

    print(f"🔄 Flushing audit buffer before querying (correlation_id={correlation_id})...")
    # Increased timeout from 5s to 10s for parallel test execution (4 workers)
    # With connection pool contention, flush may take longer
    success = audit_store.flush(timeout=10.0)
    if not success:
        raise AssertionError(
            "Audit flush timeout - events may not be persisted. "
            "This indicates a problem with the audit buffer or DataStorage connection pool."
        )
    print(f"✅ Audit buffer flushed successfully")

    start_time = time.time()
    attempts = 0

    while time.time() - start_time < timeout_seconds:
        attempts += 1
        
        # Query using audit_store's internal authenticated client (no separate DS access)
        if attempts == 1:
            print(f"🔍 AUDIT DEBUG: Querying DataStorage with:")
            print(f"   correlation_id={correlation_id}")
            print(f"   event_category={event_category}")
            print(f"   event_type={event_type}")
            print(f"   limit={limit}")
        
        response = audit_store._audit_api.query_audit_events(
            correlation_id=correlation_id,
            event_category=event_category,
            event_type=event_type,
            limit=limit,
            _request_timeout=5
        )
        events = response.data if response.data else []

        if len(events) >= min_expected_events:
            elapsed = time.time() - start_time
            print(f"✅ Found {len(events)} audit events after {elapsed:.2f}s ({attempts} attempts)")
            if events:
                print(f"🔍 AUDIT DEBUG: Event types found: {[e.event_type for e in events]}")
            return events

        if attempts % 5 == 0:  # Log every 5 attempts
            elapsed = time.time() - start_time
            print(f"⏳ Waiting for audit events... {len(events)}/{min_expected_events} found after {elapsed:.2f}s")
            if events:
                print(f"   Event types so far: {[e.event_type for e in events]}")

        time.sleep(poll_interval)

    # Timeout - fail with diagnostic info
    elapsed = time.time() - start_time
    final_response = audit_store._audit_api.query_audit_events(
        correlation_id=correlation_id,
        event_category=event_category,
        event_type=event_type,
        limit=limit,
        _request_timeout=5
    )
    final_events = final_response.data if final_response.data else []
    raise AssertionError(
        f"Timeout waiting for audit events: expected >={min_expected_events}, "
        f"got {len(final_events)} after {elapsed:.2f}s ({attempts} attempts). "
        f"ADR-038: Buffered audit flush may be delayed. "
        f"Filters: correlation_id={correlation_id}, event_category={event_category}, event_type={event_type}"
    )


# ========================================
# FLOW-BASED AUDIT INTEGRATION TESTS
# ========================================

class TestIncidentAnalysisAuditFlow:
    """
    Flow-based audit tests for incident analysis.

    Pattern: Call business logic directly → Verify audit events emitted

    BR-AUDIT-005: HAPI MUST generate audit traces for LLM interactions
    ADR-034: Audit events MUST include required fields
    ADR-038: Audit events are buffered (2s flush interval)

    Architecture: Integration tests call business logic DIRECTLY (no HTTP)
    - Go equivalent: controller.Reconcile(ctx, req)
    - Python equivalent: analyze_incident(request_data)
    """

    @pytest.mark.asyncio
    async def test_incident_analysis_emits_llm_request_and_response_events(
        self,
        audit_store,
        unique_test_id):
        """
        BR-AUDIT-005: Incident analysis MUST emit aiagent.llm.request and aiagent.llm.response audit events.

        This test validates that HAPI's business logic emits audit events as a side effect
        of processing an incident analysis request.

        ✅ CORRECT: Calls business logic directly (like Go controller.Reconcile())
        ❌ WRONG: Would use HTTP client (that's E2E testing)
        """
        # ARRANGE: Create valid incident request
        remediation_id = f"rem-int-audit-1-{unique_test_id}"
        incident_request = {
            "incident_id": f"inc-int-audit-1-{unique_test_id}",
            "remediation_id": remediation_id,
            "signal_type": "OOMKilled",
            "severity": "critical",
            "signal_source": "prometheus",
            "resource_kind": "Pod",
            "resource_name": "test-pod",
            "resource_namespace": "default",
            "cluster_name": "integration-test",
            "environment": "testing",
            "priority": "P1",
            "risk_tolerance": "low",
            "business_category": "test",
            "error_message": "Pod OOMKilled - integration test",
        }

        # ACT: Call business logic DIRECTLY (no HTTP, no API client)
        # This is the integration testing pattern - direct function call
        # Note: metrics=None is acceptable (audit tests don't assert on metrics)
        response = await analyze_incident(request_data=incident_request, mcp_config=None, app_config=None, metrics=None)

        # Verify business operation succeeded
        assert response is not None, "Business logic should return a response"
        assert "incident_id" in response, "Response should contain incident_id"

        # Give async audit operations time to complete before querying
        # DataStorage batches events with 1-second timer
        import asyncio
        await asyncio.sleep(1.5)

        # ASSERT: Verify audit events emitted as side effect
        # Uses explicit flush to eliminate async race conditions
        all_events = query_audit_events_with_retry(
            audit_store=audit_store,
            correlation_id=remediation_id,
            event_category="aiagent",  # HAPI is AI Agent Provider (ADR-034 v1.2 - autonomous tool-calling agent)
            event_type=None,  # Query all event types, filter in test
            min_expected_events=2,  # Minimum 2 events (may have more due to tool calls/validation)
            timeout_seconds=10
        )

        # DD-TESTING-001 §256-300: Deterministic count validation per event type
        event_counts = {}
        for e in all_events:
            event_counts[e.event_type] = event_counts.get(e.event_type, 0) + 1

        # DD-TESTING-001: Exactly 1 LLM request and 1 LLM response per incident analysis call
        assert event_counts.get("aiagent.llm.request", 0) == 1, \
            f"Expected exactly 1 aiagent.llm.request. Got: {event_counts}"
        assert event_counts.get("aiagent.llm.response", 0) == 1, \
            f"Expected exactly 1 aiagent.llm.response. Got: {event_counts}"

        # Verify all LLM events have same correlation_id
        for event in llm_events:
            assert event.correlation_id == remediation_id, \
                f"Event correlation_id mismatch: expected {remediation_id}, got {event.correlation_id}"

    @pytest.mark.asyncio
    async def test_incident_analysis_emits_llm_tool_call_events(
        self,
        audit_store,
        unique_test_id):
        """
        BR-AUDIT-005: Incident analysis MUST emit aiagent.llm.tool_call events for workflow searches.

        This test validates that HAPI emits audit events when LLM uses tools
        (e.g., workflow catalog search) during analysis.

        ✅ CORRECT: Calls business logic directly, verifies tool call audits
        """
        # ARRANGE
        remediation_id = f"rem-int-audit-2-{unique_test_id}"
        incident_request = {
            "incident_id": f"inc-int-audit-2-{unique_test_id}",
            "remediation_id": remediation_id,
            "signal_type": "CrashLoopBackOff",
            "severity": "high",
            "signal_source": "prometheus",
            "resource_kind": "Pod",
            "resource_name": "crashloop-pod",
            "resource_namespace": "default",
            "cluster_name": "integration-test",
            "environment": "testing",
            "priority": "P1",
            "risk_tolerance": "medium",
            "business_category": "test",
            "error_message": "Pod in CrashLoopBackOff",
        }

        # ACT: Call business logic directly
        # Note: metrics=None is acceptable (testing audit, not metrics)
        response = await analyze_incident(request_data=incident_request, mcp_config=None, app_config=None, metrics=None)
        assert response is not None

        # ASSERT: Verify tool call events emitted
        # DD-TESTING-001: Mock mode deterministic flow emits exactly 4 events:
        #   aiagent.llm.request (1) + aiagent.llm.tool_call (1) + aiagent.llm.response (1) + aiagent.workflow.validation_attempt (1)
        events = query_audit_events_with_retry(
            audit_store=audit_store,
            correlation_id=remediation_id,
            event_category="aiagent",  # HAPI is AI Agent Provider (ADR-034 v1.2)
            event_type=None,  # Query all event types, filter in test
            min_expected_events=4,  # Retry until all 4 deterministic events appear
            timeout_seconds=10
        )

        # DD-TESTING-001 §256-300: Deterministic count validation per event type
        event_counts = {}
        for e in events:
            event_counts[e.event_type] = event_counts.get(e.event_type, 0) + 1

        assert event_counts.get("aiagent.llm.request", 0) == 1, \
            f"Expected exactly 1 aiagent.llm.request. Got: {event_counts}"
        assert event_counts.get("aiagent.llm.tool_call", 0) == 1, \
            f"Expected exactly 1 aiagent.llm.tool_call. Got: {event_counts}"
        assert event_counts.get("aiagent.llm.response", 0) == 1, \
            f"Expected exactly 1 aiagent.llm.response. Got: {event_counts}"
        assert event_counts.get("aiagent.workflow.validation_attempt", 0) == 1, \
            f"Expected exactly 1 aiagent.workflow.validation_attempt. Got: {event_counts}"

    @pytest.mark.asyncio
    async def test_incident_analysis_workflow_validation_emits_validation_attempt_events(
        self,
        audit_store,
        unique_test_id):
        """
        BR-AUDIT-005: Workflow validation MUST emit aiagent.workflow.validation_attempt events.

        This test validates that HAPI emits audit events during workflow validation
        (self-correction loop) as part of incident analysis.

        ✅ CORRECT: Calls business logic directly, verifies validation audits
        """
        # ARRANGE
        remediation_id = f"rem-int-audit-3-{unique_test_id}"
        incident_request = {
            "incident_id": f"inc-int-audit-3-{unique_test_id}",
            "remediation_id": remediation_id,
            "signal_type": "OOMKilled",
            "severity": "critical",
            "signal_source": "prometheus",
            "resource_kind": "Pod",
            "resource_name": "validation-pod",
            "resource_namespace": "default",
            "cluster_name": "integration-test",
            "environment": "production",
            "priority": "P0",
            "risk_tolerance": "low",
            "business_category": "critical",
            "error_message": "Pod OOMKilled - validation test",
        }

        # ACT: Call business logic directly
        # Note: metrics=None is acceptable (testing audit, not metrics)
        response = await analyze_incident(request_data=incident_request, mcp_config=None, app_config=None, metrics=None)
        assert response is not None

        # ASSERT: Verify validation attempt events emitted (with retry polling)
        events = query_audit_events_with_retry(
            audit_store=audit_store,
            correlation_id=remediation_id,
            event_category="aiagent",  # HAPI is AI Agent Provider (ADR-034 v1.2)
            event_type="aiagent.workflow.validation_attempt",  # Specific event type for this test
            min_expected_events=1,  # Retry until event appears
            timeout_seconds=10
        )

        # DD-TESTING-001: Deterministic count - Mock LLM returns valid workflow on first try
        assert len(events) == 1, \
            f"Expected exactly 1 aiagent.workflow.validation_attempt event. Got: {len(events)}"
        assert events[0].event_type == "aiagent.workflow.validation_attempt", \
            f"Unexpected event type: {events[0].event_type}"


class TestRecoveryAnalysisAuditFlow:
    """
    Flow-based audit tests for recovery analysis.

    Pattern: Call business logic directly → Verify audit events emitted

    BR-AUDIT-005: HAPI MUST generate audit traces for recovery analysis

    Architecture: Integration tests call business logic DIRECTLY (no HTTP)
    """

    @pytest.mark.asyncio
    async def test_recovery_analysis_emits_llm_request_and_response_events(
        self,
        audit_store,
        unique_test_id):
        """
        BR-AUDIT-005: Recovery analysis MUST emit aiagent.llm.request and aiagent.llm.response audit events.

        This test validates that HAPI emits audit events during recovery analysis.

        ✅ CORRECT: Calls business logic directly (like Go controller.Reconcile())
        """
        # ARRANGE
        remediation_id = f"rem-int-audit-rec-1-{unique_test_id}"
        recovery_request = {
            "incident_id": f"inc-int-audit-rec-1-{unique_test_id}",
            "remediation_id": remediation_id,
            "signal_type": "OOMKilled",
            "previous_workflow_id": "oomkill-increase-memory-v1",
            "previous_workflow_result": "Failed",
            "resource_namespace": "default",
            "resource_name": "recovery-pod",
            "resource_kind": "Pod",
        }

        # ACT: Call business logic directly (no HTTP)
        # Note: metrics=None is acceptable (testing audit, not metrics)
        response = await analyze_recovery(request_data=recovery_request, app_config=None, metrics=None)
        assert response is not None

        # ASSERT: Verify audit events emitted
        all_events = query_audit_events_with_retry(
            audit_store=audit_store,
            correlation_id=remediation_id,
            event_category="aiagent",  # HAPI is AI Agent Provider (ADR-034 v1.2)
            event_type=None,  # Query all event types, filter in test
            min_expected_events=2,  # Minimum 2 events (may have more due to tool calls/validation)
            timeout_seconds=10
        )

        # DD-TESTING-001 §256-300: Deterministic count validation per event type
        event_counts = {}
        for e in all_events:
            event_counts[e.event_type] = event_counts.get(e.event_type, 0) + 1

        # DD-TESTING-001: Exactly 1 LLM request and 1 LLM response per recovery analysis call
        assert event_counts.get("aiagent.llm.request", 0) == 1, \
            f"Expected exactly 1 aiagent.llm.request. Got: {event_counts}"
        assert event_counts.get("aiagent.llm.response", 0) == 1, \
            f"Expected exactly 1 aiagent.llm.response. Got: {event_counts}"


class TestAuditEventSchemaValidation:
    """
    Flow-based tests for ADR-034 audit event schema compliance.

    Pattern: Call business logic directly → Verify audit events have required fields

    ADR-034: Audit events MUST include specific fields
    """

    @pytest.mark.asyncio
    async def test_audit_events_have_required_adr034_fields(
        self,
        audit_store,
        unique_test_id):
        """
        ADR-034: Audit events MUST include required fields per ADR-034 spec.

        This test validates that audit events emitted by HAPI business operations
        include all ADR-034 required fields.

        ✅ CORRECT: Calls business logic directly, validates emitted event schema
        """
        # ARRANGE
        remediation_id = f"rem-int-audit-schema-{unique_test_id}"
        incident_request = {
            "incident_id": f"inc-int-audit-schema-{unique_test_id}",
            "remediation_id": remediation_id,
            "signal_type": "OOMKilled",
            "severity": "critical",
            "signal_source": "prometheus",
            "resource_kind": "Pod",
            "resource_name": "schema-test-pod",
            "resource_namespace": "default",
            "cluster_name": "integration-test",
            "environment": "testing",
            "priority": "P1",
            "risk_tolerance": "low",
            "business_category": "test",
            "error_message": "ADR-034 schema validation test",
        }

        # ACT: Call business logic directly
        # Note: metrics=None is acceptable (testing audit, not metrics)
        response = await analyze_incident(request_data=incident_request, mcp_config=None, app_config=None, metrics=None)
        assert response is not None

        # ASSERT: Verify all audit events have ADR-034 required fields
        events = query_audit_events_with_retry(
            audit_store=audit_store,
            correlation_id=remediation_id,
            event_category="aiagent",  # HAPI is AI Agent Provider (ADR-034 v1.2)
            event_type=None,  # Query all event types for comprehensive validation
            min_expected_events=1,
            timeout_seconds=10
        )
        # DD-TESTING-001: Deterministic count - single incident analysis emits at least 2 HAPI events
        assert len(events) >= 2, f"Expected at least 2 audit events (aiagent.llm.request + aiagent.llm.response). Found: {len(events)}"

        # ADR-034 required fields (per ADR-034 v1.2 and Data Storage OpenAPI spec)
        required_fields = [
            "event_id",
            "event_type",
            "event_category",
            "correlation_id",
            "event_timestamp",  # Correct field name per ADR-034
            "event_action",      # Required per ADR-034
            "event_outcome",     # Required per ADR-034
            "event_data",
            "version",           # Required per ADR-034
        ]

        for event in events:
            for field in required_fields:
                assert hasattr(event, field), \
                    f"Event {event.event_type} missing ADR-034 required field: {field}"

                # Verify field is not None
                field_value = getattr(event, field)
                assert field_value is not None, \
                    f"Event {event.event_type} has null value for ADR-034 required field: {field}"

            # ADR-034 v1.6: HAPI uses 'aiagent', DataStorage uses 'workflow'
            # AIAnalysis controller uses 'analysis' (not tested here - separate service)
            # Tests must expect MIXED event categories (HAPI + DataStorage).
            valid_categories = ["aiagent", "workflow"]  # Updated for ADR-034 v1.6
            assert event.event_category in valid_categories, \
                f"Expected ADR-034 category in {valid_categories}, got '{event.event_category}'"

            # Validate event_category matches event_type per ADR-034 service-level naming
            if event.event_type.startswith("workflow."):
                assert event.event_category == "workflow", \
                    f"Workflow events must have category='workflow', got '{event.event_category}'"
            elif event.event_type.startswith("aianalysis."):
                # This should NOT appear in HAPI tests - AIAnalysis controller only
                assert event.event_category == "analysis", \
                    f"AI Analysis events must have category='analysis', got '{event.event_category}'"
            elif event.event_type in ["aiagent.llm.request", "aiagent.llm.response", "aiagent.llm.tool_call", 
                                       "aiagent.workflow.validation_attempt", "aiagent.response.complete"]:
                # ADR-034 v1.6: HAPI events use 'aiagent' category
                assert event.event_category == "aiagent", \
                    f"HAPI events must have category='aiagent' per ADR-034 v1.6, got '{event.event_category}'"

            # Verify event has valid version
            assert event.version is not None, \
                f"Event {event.event_type} has null version (required by ADR-034)"


class TestErrorScenarioAuditFlow:
    """
    Flow-based audit tests for error scenarios.

    Pattern: Call business logic directly with error inputs → Verify audit events include error context

    BR-AUDIT-005: HAPI MUST generate audit traces even for failed operations
    """

    @pytest.mark.asyncio
    async def test_workflow_not_found_emits_audit_with_error_context(
        self,
        audit_store,
        unique_test_id):
        """
        BR-AUDIT-005: HAPI MUST emit audit events even when business operations fail.

        This test validates that audit trail is maintained when business logic encounters
        errors (e.g., no suitable workflow found for the signal type).

        Pattern:
        - ✅ Valid request structure (passes validation)
        - ✅ Business logic executes (generates audit events)
        - ✅ Business operation fails gracefully (no workflow match)
        - ✅ Audit events include error/fallback context

        This is different from validation errors (which never reach business logic).
        """
        # ARRANGE: Valid request with non-existent workflow signal type
        remediation_id = f"rem-int-audit-bizfail-{unique_test_id}"
        incident_request = {
            "incident_id": f"inc-int-audit-bizfail-{unique_test_id}",
            "remediation_id": remediation_id,
            "signal_type": "NonExistentSignalType999999",  # Valid format, doesn't exist
            "severity": "critical",
            "signal_source": "prometheus",
            "resource_kind": "Pod",
            "resource_name": "test-pod",
            "resource_namespace": "default",
            "cluster_name": "integration-test",
            "environment": "testing",
            "priority": "P1",
            "risk_tolerance": "low",
            "business_category": "test",
            "error_message": "Testing business failure audit trail",
        }

        # ACT: Call business logic directly - will fail gracefully
        # Note: metrics=None is acceptable (testing error audit, not metrics)
        response = await analyze_incident(request_data=incident_request, mcp_config=None, app_config=None, metrics=None)

        # Verify business operation completed (even if no workflow found)
        assert response is not None, "Business logic should return a response"
        assert "analysis" in response, "Business logic should complete and return analysis"
        # Mock mode returns a deterministic response even for non-existent workflows

        # ASSERT: Verify audit events were generated despite business failure
        all_events = query_audit_events_with_retry(
            audit_store=audit_store,
            correlation_id=remediation_id,
            event_category="aiagent",  # HAPI is AI Agent Provider (ADR-034 v1.2)
            event_type=None,  # Query all event types, filter in test
            min_expected_events=2,  # Minimum 2 events (may have more due to tool calls/validation)
            timeout_seconds=10
        )

        # DD-TESTING-001 §256-300: Deterministic count validation per event type
        event_counts = {}
        for e in all_events:
            event_counts[e.event_type] = event_counts.get(e.event_type, 0) + 1

        # DD-TESTING-001: Even failed workflows must emit exactly 1 request + 1 response
        assert event_counts.get("aiagent.llm.request", 0) == 1, \
            f"Expected exactly 1 aiagent.llm.request even for failed workflow. Got: {event_counts}"
        assert event_counts.get("aiagent.llm.response", 0) == 1, \
            f"Expected exactly 1 aiagent.llm.response even for failed workflow. Got: {event_counts}"

        # Verify events include the remediation_id (correlation)
        for event in all_events:
            assert event.correlation_id == remediation_id, \
                f"Event correlation_id mismatch: {event.correlation_id} != {remediation_id}"


# ========================================
# TEST COLLECTION
# ========================================

# Total: 7 flow-based integration tests (direct business logic calls)
# - 3 incident analysis tests
# - 1 recovery analysis test
# - 1 schema validation test
# - 1 error scenario test
# - 1 tool call test

# Architecture Change (Jan 4, 2026):
# BEFORE: Tests used HTTP client (OpenAPI) - actually E2E tests
# AFTER: Tests call business logic directly - true integration tests
# Matches Go service testing pattern (controller.Reconcile() direct calls)

# HTTP API testing deferred to E2E test suite (future implementation)
# See: docs/handoff/HAPI_INTEGRATION_TEST_ARCHITECTURE_FIX_JAN_04_2026.md

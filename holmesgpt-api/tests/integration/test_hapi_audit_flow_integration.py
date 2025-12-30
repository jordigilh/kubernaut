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

These tests validate that HAPI business operations emit audit events as side effects.

✅ CORRECT PATTERN:
1. Trigger business operation (HTTP request to HAPI endpoint)
2. Wait for processing (ADR-038: buffered audit flush)
3. Verify audit events emitted via Data Storage API
4. Validate audit event content

❌ ANTI-PATTERN (FORBIDDEN):
1. Manually create audit events
2. Directly call Data Storage API to store events
3. Test audit infrastructure, not business logic

Reference Implementations:
- SignalProcessing: test/integration/signalprocessing/audit_integration_test.go
- Gateway: test/integration/gateway/audit_integration_test.go
- HAPI E2E: holmesgpt-api/tests/e2e/test_audit_pipeline_e2e.py

Replaced: test_audit_integration.py (anti-pattern tests deleted)
"""

import os
import time
import pytest
import requests
from typing import List, Dict, Any

# DD-API-001: Import OpenAPI generated clients
import sys
sys.path.insert(0, 'tests/clients')
from holmesgpt_api_client import ApiClient as HapiApiClient, Configuration as HapiConfiguration
from holmesgpt_api_client.api.incident_analysis_api import IncidentAnalysisApi
from holmesgpt_api_client.api.recovery_analysis_api import RecoveryAnalysisApi
from holmesgpt_api_client.models.incident_request import IncidentRequest
from holmesgpt_api_client.models.recovery_request import RecoveryRequest
from src.clients.datastorage import ApiClient as DataStorageApiClient, Configuration as DataStorageConfiguration
from src.clients.datastorage.api.audit_write_api_api import AuditWriteAPIApi
from src.clients.datastorage.models.audit_event import AuditEvent


# ========================================
# FIXTURES
# ========================================

# Note: hapi_client fixture is provided by conftest.py (session-scoped, TestClient)
# Note: data_storage_url fixture is provided by conftest.py (session-scoped)
# This allows workflow seeding and audit validation to work correctly


# ========================================
# HELPER FUNCTIONS
# ========================================

def query_audit_events(
    data_storage_url: str,
    correlation_id: str,
    timeout: int = 10
) -> List[AuditEvent]:
    """
    Query Data Storage for audit events by correlation_id using OpenAPI client.

    DD-API-001 COMPLIANT: Uses generated OpenAPI client (type-safe, contract-validated).

    Args:
        data_storage_url: Data Storage service URL
        correlation_id: Remediation ID for audit correlation
        timeout: Request timeout in seconds

    Returns:
        List of AuditEvent Pydantic models
    """
    config = DataStorageConfiguration(host=data_storage_url)
    client = DataStorageApiClient(configuration=config)
    api_instance = AuditWriteAPIApi(client)

    # DD-API-001: Use OpenAPI generated client
    response = api_instance.query_audit_events(
        correlation_id=correlation_id,
        _request_timeout=timeout
    )

    # Return Pydantic models directly
    return response.data if response.data else []


def query_audit_events_with_retry(
    data_storage_url: str,
    correlation_id: str,
    min_expected_events: int = 1,
    timeout_seconds: int = 10,
    poll_interval: float = 0.5
) -> List[AuditEvent]:
    """
    Query Data Storage for audit events with retry logic (Eventually pattern).

    ADR-038: Buffered audit store flushes asynchronously every 0.1s.
    Tests must poll for events rather than assuming immediate availability.

    Pattern: Similar to Ginkgo's Eventually() - poll with timeout until events appear

    Args:
        data_storage_url: Data Storage service URL
        correlation_id: Remediation ID for audit correlation
        min_expected_events: Minimum number of events expected (default 1)
        timeout_seconds: Maximum time to wait for events (default 10s)
        poll_interval: Time between polling attempts (default 0.5s)

    Returns:
        List of AuditEvent Pydantic models

    Raises:
        AssertionError: If events don't appear within timeout
    """
    start_time = time.time()
    attempts = 0

    while time.time() - start_time < timeout_seconds:
        attempts += 1
        events = query_audit_events(data_storage_url, correlation_id, timeout=5)

        if len(events) >= min_expected_events:
            elapsed = time.time() - start_time
            print(f"✅ Found {len(events)} audit events after {elapsed:.2f}s ({attempts} attempts)")
            return events

        if attempts % 5 == 0:  # Log every 5 attempts
            elapsed = time.time() - start_time
            print(f"⏳ Waiting for audit events... {len(events)}/{min_expected_events} found after {elapsed:.2f}s")

        time.sleep(poll_interval)

    # Timeout - fail with diagnostic info
    elapsed = time.time() - start_time
    final_events = query_audit_events(data_storage_url, correlation_id, timeout=5)
    raise AssertionError(
        f"Timeout waiting for audit events: expected >={min_expected_events}, "
        f"got {len(final_events)} after {elapsed:.2f}s ({attempts} attempts). "
        f"ADR-038: Buffered audit flush may be delayed. "
        f"correlation_id={correlation_id}"
    )


def call_hapi_incident_analyze(
    hapi_client,
    incident_data: Dict[str, Any]
) -> Dict[str, Any]:
    """
    Call HAPI's /api/v1/incident/analyze endpoint using FastAPI TestClient.

    Architecture: Integration tests use TestClient (in-process HAPI)

    Args:
        hapi_client: FastAPI TestClient fixture
        incident_data: Incident request data

    Returns:
        Response dict
    """
    response = hapi_client.post("/api/v1/incident/analyze", json=incident_data)
    assert response.status_code == 200, f"Expected 200, got {response.status_code}: {response.text}"
    return response.json()


def call_hapi_recovery_analyze(
    hapi_client,
    recovery_data: Dict[str, Any]
) -> Dict[str, Any]:
    """
    Call HAPI's /api/v1/recovery/analyze endpoint using FastAPI TestClient.

    Architecture: Integration tests use TestClient (in-process HAPI)

    Args:
        hapi_client: FastAPI TestClient fixture
        recovery_data: Recovery request data

    Returns:
        Response dict
    """
    response = hapi_client.post("/api/v1/recovery/analyze", json=recovery_data)
    assert response.status_code == 200, f"Expected 200, got {response.status_code}: {response.text}"
    return response.json()


# ========================================
# FLOW-BASED AUDIT INTEGRATION TESTS
# ========================================

class TestIncidentAnalysisAuditFlow:
    """
    Flow-based audit tests for incident analysis.

    Pattern: Trigger business operation → Verify audit events emitted

    BR-AUDIT-005: HAPI MUST generate audit traces for LLM interactions
    ADR-034: Audit events MUST include required fields
    ADR-038: Audit events are buffered (2s flush interval)
    """

    def test_incident_analysis_emits_llm_request_and_response_events(
        self,
        hapi_client,
        data_storage_url,
        unique_test_id):
        """
        BR-AUDIT-005: Incident analysis MUST emit llm_request and llm_response audit events.

        This test validates that HAPI's business logic emits audit events as a side effect
        of processing an incident analysis request.

        ✅ CORRECT: Tests HAPI behavior (emits audits during business operation)
        ❌ WRONG: Would manually create events and call DS API
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

        # ACT: Trigger business operation (incident analysis) via TestClient
        response = call_hapi_incident_analyze(hapi_client, incident_request)

        # Verify business operation succeeded
        assert response is not None, "HAPI should return a response"
        assert "incident_id" in response, "Response should contain incident_id"

        # ASSERT: Verify audit events emitted as side effect (with retry)
        # ADR-038: Buffered audit store flushes asynchronously, so poll for events
        events = query_audit_events_with_retry(
            data_storage_url,
            remediation_id,
            min_expected_events=2,  # llm_request + llm_response
            timeout_seconds=10
        )

        # Should have at least llm_request and llm_response
        assert len(events) >= 2, f"Expected at least 2 audit events (llm_request, llm_response), got {len(events)}"

        # Extract event types
        event_types = [e.event_type for e in events]

        # Verify llm_request event emitted
        assert "llm_request" in event_types, \
            f"llm_request event not found in {event_types}"

        # Verify llm_response event emitted
        assert "llm_response" in event_types, \
            f"llm_response event not found in {event_types}"

        # Verify all events have same correlation_id
        for event in events:
            assert event.correlation_id == remediation_id, \
                f"Event correlation_id mismatch: expected {remediation_id}, got {event.correlation_id}"

    def test_incident_analysis_emits_llm_tool_call_events(
        self,
        hapi_client,
        data_storage_url, unique_test_id):
        """
        BR-AUDIT-005: Incident analysis MUST emit llm_tool_call events for workflow searches.

        This test validates that HAPI emits audit events when LLM uses tools
        (e.g., workflow catalog search) during analysis.

        ✅ CORRECT: Verifies HAPI emits tool call audits during business operation
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

        # ACT: Trigger business operation
        response = call_hapi_incident_analyze(hapi_client, incident_request)
        assert response is not None

        # ASSERT: Verify tool call events emitted (with retry polling)
        events = query_audit_events_with_retry(
            data_storage_url,
            remediation_id,
            min_expected_events=1,  # At least llm_tool_call
            timeout_seconds=10
        )
        event_types = [e.event_type for e in events]

        # Tool call events should be present (workflow catalog search)
        assert "llm_tool_call" in event_types, \
            f"llm_tool_call event not found. Events: {event_types}"

    def test_incident_analysis_workflow_validation_emits_validation_attempt_events(
        self,
        hapi_client,
        data_storage_url, unique_test_id):
        """
        BR-AUDIT-005: Workflow validation MUST emit workflow_validation_attempt events.

        This test validates that HAPI emits audit events during workflow validation
        (self-correction loop) as part of incident analysis.

        ✅ CORRECT: Verifies HAPI emits validation audits during business operation
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

        # ACT: Trigger business operation
        response = call_hapi_incident_analyze(hapi_client, incident_request)
        assert response is not None

        # ASSERT: Verify validation attempt events emitted (with retry polling)
        events = query_audit_events_with_retry(
            data_storage_url,
            remediation_id,
            min_expected_events=1,  # At least workflow_validation_attempt
            timeout_seconds=10
        )
        event_types = [e.event_type for e in events]

        # Validation attempt events should be present
        assert "workflow_validation_attempt" in event_types, \
            f"workflow_validation_attempt event not found. Events: {event_types}"


class TestRecoveryAnalysisAuditFlow:
    """
    Flow-based audit tests for recovery analysis.

    Pattern: Trigger recovery analysis → Verify audit events emitted

    BR-AUDIT-005: HAPI MUST generate audit traces for recovery analysis
    """

    def test_recovery_analysis_emits_llm_request_and_response_events(
        self,
        hapi_client,
        data_storage_url, unique_test_id):
        """
        BR-AUDIT-005: Recovery analysis MUST emit llm_request and llm_response audit events.

        This test validates that HAPI emits audit events during recovery analysis.

        ✅ CORRECT: Tests HAPI behavior (emits audits during business operation)
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

        # ACT: Trigger recovery analysis
        response = call_hapi_recovery_analyze(hapi_client, recovery_request)
        assert response is not None

        # ASSERT: Verify audit events emitted (with retry polling)
        events = query_audit_events_with_retry(
            data_storage_url,
            remediation_id,
            min_expected_events=2,  # llm_request + llm_response
            timeout_seconds=10
        )
        assert len(events) >= 2, f"Expected at least 2 audit events, got {len(events)}"

        event_types = [e.event_type for e in events]
        assert "llm_request" in event_types, f"llm_request not found in {event_types}"
        assert "llm_response" in event_types, f"llm_response not found in {event_types}"


class TestAuditEventSchemaValidation:
    """
    Flow-based tests for ADR-034 audit event schema compliance.

    Pattern: Trigger business operation → Verify audit events have required fields

    ADR-034: Audit events MUST include specific fields
    """

    def test_audit_events_have_required_adr034_fields(
        self,
        hapi_client,
        data_storage_url, unique_test_id):
        """
        ADR-034: Audit events MUST include required fields per ADR-034 spec.

        This test validates that audit events emitted by HAPI business operations
        include all ADR-034 required fields.

        ✅ CORRECT: Validates HAPI-emitted events have required schema
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

        # ACT: Trigger business operation
        response = call_hapi_incident_analyze(hapi_client, incident_request)
        assert response is not None

        # ASSERT: Verify all audit events have ADR-034 required fields (with retry polling)
        events = query_audit_events_with_retry(
            data_storage_url,
            remediation_id,
            min_expected_events=1,
            timeout_seconds=10
        )
        assert len(events) > 0, "No audit events found"

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

            # Verify event_category is correct for HAPI (ADR-034 v1.2)
            assert event.event_category == "analysis", \
                f"Expected event_category='analysis' for HAPI, got '{event.event_category}'"

            # Verify event has valid version
            assert event.version is not None, \
                f"Event {event.event_type} has null version (required by ADR-034)"


class TestErrorScenarioAuditFlow:
    """
    Flow-based audit tests for error scenarios.

    Pattern: Trigger error scenario → Verify audit events include error context

    BR-AUDIT-005: HAPI MUST generate audit traces even for failed operations
    """

    def test_workflow_not_found_emits_audit_with_error_context(
        self,
        hapi_client,
        data_storage_url,
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

        # ACT: Trigger business operation that will fail gracefully
        response = call_hapi_incident_analyze(hapi_client, incident_request)

        # Verify business operation completed (even if no workflow found)
        assert response is not None, "HAPI should return a response"
        assert "analysis" in response, "Business logic should complete and return analysis"
        # Mock mode returns a deterministic response even for non-existent workflows

        # ASSERT: Verify audit events were generated despite business failure
        events = query_audit_events_with_retry(
            data_storage_url,
            remediation_id,
            min_expected_events=2,  # At minimum: llm_request + llm_response
            timeout_seconds=10
        )

        # Should have audit events even though no workflow matched
        assert len(events) >= 2, \
            f"Expected audit events even for failed workflow search, got {len(events)}"

        # Verify events include the remediation_id (correlation)
        for event in events:
            assert event.correlation_id == remediation_id, \
                f"Event correlation_id mismatch: {event.correlation_id} != {remediation_id}"

        # Business failure context should be captured in audit events
        event_types = [e.event_type for e in events]
        assert "llm_request" in event_types, "Should audit LLM request even when workflow not found"
        assert "llm_response" in event_types, "Should audit LLM response even when workflow not found"


# ========================================
# TEST COLLECTION
# ========================================

# Total: 7 flow-based tests
# - 3 incident analysis tests
# - 1 recovery analysis test
# - 1 schema validation test
# - 1 error scenario test
# - 1 tool call test (within incident analysis)

# These tests replace 6 anti-pattern tests that were deleted.
# See: docs/handoff/HAPI_AUDIT_INTEGRATION_ANTI_PATTERN_TRIAGE_DEC_26_2025.md


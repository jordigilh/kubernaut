"""
Copyright 2026 Jordi Gil.

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
Session-Based Async Integration Tests for HAPI (BR-AA-HAPI-064)

Tests the full session lifecycle (submit/poll/result) via FastAPI TestClient,
exercising the async endpoints with real Mock LLM and DataStorage dependencies.

Business Requirements:
- BR-AA-HAPI-064.1: POST /analyze returns 202 with session_id
- BR-AA-HAPI-064.2: GET /session/{id} returns session status
- BR-AA-HAPI-064.3: GET /session/{id}/result returns full result
- BR-AA-HAPI-064.9: Recovery session lifecycle mirrors incident

Architecture:
- TestClient runs in-process (no Docker container for HAPI)
- Mock LLM runs on port 18140 (started by Go infrastructure)
- DataStorage runs on port 18098 (started by Go infrastructure)
- TestClient runs BackgroundTasks synchronously

Reference: docs/testing/BR-AA-HAPI-064/session_based_pull_test_plan_v1.0.md
"""

import pytest
import sys
from pathlib import Path

# Add src/ to path for business logic imports
sys.path.insert(0, str(Path(__file__).parent.parent.parent))
# Add tests/integration/ to path for shared helpers (helpers.py)
sys.path.insert(0, str(Path(__file__).parent))

from src.session.session_manager import reset_session_manager, get_session_manager

# Shared audit query helper (extracted to helpers.py for cross-file reuse)
from helpers import query_audit_events_with_retry


# ========================================
# SESSION RESET FIXTURE
# ========================================

@pytest.fixture(autouse=True)
def _reset_sessions():
    """
    Reset the global SessionManager singleton between tests.
    BR-AA-HAPI-064: Prevents session leakage between test cases.
    """
    reset_session_manager()
    yield
    reset_session_manager()


# ========================================
# SHARED HELPERS
# ========================================

def _submit_and_get_result(client, endpoint_prefix: str, request_data: dict) -> dict:
    """
    Submit an investigation and retrieve the result via session endpoints.

    TestClient runs BackgroundTasks synchronously, so by the time POST returns 202,
    the background investigation has already completed. We can immediately fetch the result.

    Args:
        client: FastAPI TestClient
        endpoint_prefix: "/api/v1/incident" or "/api/v1/recovery"
        request_data: Request payload

    Returns:
        dict with keys: session_id, status_response, result_response
    """
    # Submit investigation
    submit_resp = client.post(f"{endpoint_prefix}/analyze", json=request_data)
    assert submit_resp.status_code == 202, (
        f"Expected 202 Accepted, got {submit_resp.status_code}: {submit_resp.text}"
    )
    session_id = submit_resp.json()["session_id"]
    assert session_id, "session_id must not be empty"

    # Poll session status (should be completed since TestClient runs tasks synchronously)
    status_resp = client.get(f"{endpoint_prefix}/session/{session_id}")
    assert status_resp.status_code == 200, (
        f"Expected 200 OK for session status, got {status_resp.status_code}: {status_resp.text}"
    )

    # Retrieve result
    result_resp = client.get(f"{endpoint_prefix}/session/{session_id}/result")
    assert result_resp.status_code == 200, (
        f"Expected 200 OK for session result, got {result_resp.status_code}: {result_resp.text}"
    )

    return {
        "session_id": session_id,
        "status_response": status_resp.json(),
        "result_response": result_resp.json(),
    }


# ========================================
# INCIDENT SESSION FLOW TESTS
# ========================================

class TestIncidentSessionIntegration:
    """
    IT-HAPI-064-001 to 004: Incident session lifecycle via HTTP endpoints.
    Uses TestClient with real Mock LLM and DataStorage.
    """

    def test_incident_submit_poll_result_lifecycle(
        self, hapi_client, unique_test_id
    ):
        """
        IT-HAPI-064-001: Submit + poll + result via TestClient.

        BR-AA-HAPI-064.1, .2, .3: Full incident session lifecycle.
        Verifies that HAPI processes an investigation asynchronously
        and produces a complete result accessible via session endpoints.
        """
        request_data = {
            "incident_id": f"inc-it-session-001-{unique_test_id}",
            "remediation_id": f"rem-it-session-001-{unique_test_id}",
            "signal_name": "OOMKilled",
            "severity": "critical",
            "signal_source": "prometheus",
            "resource_kind": "Pod",
            "resource_name": "session-test-pod",
            "resource_namespace": "default",
            "cluster_name": "integration-test",
            "environment": "testing",
            "priority": "P1",
            "risk_tolerance": "low",
            "business_category": "test",
            "error_message": "OOMKilled - session integration test",
        }

        result = _submit_and_get_result(hapi_client, "/api/v1/incident", request_data)

        # Verify session status is completed (TestClient runs tasks synchronously)
        assert result["status_response"]["status"] == "completed"

        # Verify result contains expected fields from IncidentResponse
        incident_result = result["result_response"]
        assert "incident_id" in incident_result, "Result should contain incident_id"
        assert "analysis" in incident_result, "Result should contain analysis"

    def test_session_status_transitions(
        self, hapi_client, unique_test_id
    ):
        """
        IT-HAPI-064-002: Session status transitions observable via polling.

        With TestClient's synchronous BackgroundTasks, the full transition
        (pending -> investigating -> completed) happens during the POST.
        We verify:
        - A manually created session starts as "pending"
        - After submit + background task, the session shows "completed"

        BR-AA-HAPI-064.2: Poll endpoint returns observable status.
        """
        # 1. Create a session manually to observe "pending" state
        sm = get_session_manager()
        manual_session_id = sm.create_session("incident", {"test": True})

        # Poll: should be "pending" (no background task triggered)
        status_resp = hapi_client.get(f"/api/v1/incident/session/{manual_session_id}")
        assert status_resp.status_code == 200
        assert status_resp.json()["status"] == "pending"

        # 2. Now submit a real investigation via HTTP
        request_data = {
            "incident_id": f"inc-it-session-002-{unique_test_id}",
            "remediation_id": f"rem-it-session-002-{unique_test_id}",
            "signal_name": "OOMKilled",
            "severity": "critical",
            "signal_source": "prometheus",
            "resource_kind": "Pod",
            "resource_name": "status-test-pod",
            "resource_namespace": "default",
            "cluster_name": "integration-test",
            "environment": "testing",
            "priority": "P1",
            "risk_tolerance": "low",
            "business_category": "test",
            "error_message": "Status transition test",
        }

        submit_resp = hapi_client.post("/api/v1/incident/analyze", json=request_data)
        assert submit_resp.status_code == 202
        session_id = submit_resp.json()["session_id"]

        # After POST returns, background task has completed (TestClient synchronous)
        status_resp = hapi_client.get(f"/api/v1/incident/session/{session_id}")
        assert status_resp.status_code == 200
        assert status_resp.json()["status"] == "completed"

    def test_session_audit_events_exact_counts(
        self, hapi_client, audit_store, unique_test_id
    ):
        """
        IT-HAPI-064-003: Session audit events emitted with exact counts.

        BR-AUDIT-005, ADR-034: Verifies that the async session flow produces
        the same audit trail as the direct business logic call pattern.

        Expected exact counts (with current Mock LLM behavior):
        The Mock LLM's OOMKilled response omits TARGET_RESOURCE, triggering
        DD-HAPI-002 v1.2 workflow validation retries (max 3 attempts).
        Each retry emits its own LLM request/response/tool_call events.

        - aiagent.llm.request: exactly 3 (initial + 2 retries with error feedback)
        - aiagent.llm.response: exactly 3
        - aiagent.llm.tool_call: exactly 3 (search_workflow_catalog per attempt)
        - aiagent.workflow.validation_attempt: exactly 4 (initial + 3 retry validations)
        - aiagent.response.complete: exactly 1
        - Total HAPI events: exactly 14
        """
        remediation_id = f"rem-it-session-003-{unique_test_id}"
        request_data = {
            "incident_id": f"inc-it-session-003-{unique_test_id}",
            "remediation_id": remediation_id,
            "signal_name": "OOMKilled",
            "severity": "critical",
            "signal_source": "prometheus",
            "resource_kind": "Pod",
            "resource_name": "audit-test-pod",
            "resource_namespace": "default",
            "cluster_name": "integration-test",
            "environment": "testing",
            "priority": "P1",
            "risk_tolerance": "low",
            "business_category": "test",
            "error_message": "Audit event count test",
        }

        # Submit via session endpoint (not direct business logic call)
        result = _submit_and_get_result(hapi_client, "/api/v1/incident", request_data)
        assert result["status_response"]["status"] == "completed"

        # Verify audit events
        # 14 total: 3*(request+response+tool_call) + 4*validation_attempt + 1*response.complete
        all_events = query_audit_events_with_retry(
            audit_store=audit_store,
            correlation_id=remediation_id,
            event_category="aiagent",
            event_type=None,
            min_expected_events=14,
            timeout_seconds=15,
        )

        event_counts = {}
        for e in all_events:
            event_counts[e.event_type] = event_counts.get(e.event_type, 0) + 1

        # Mock LLM OOMKilled response triggers 3 validation attempts (TARGET_RESOURCE missing)
        assert event_counts.get("aiagent.llm.request", 0) == 3, (
            f"Expected exactly 3 aiagent.llm.request (3 validation attempts). Got: {event_counts}"
        )
        assert event_counts.get("aiagent.llm.response", 0) == 3, (
            f"Expected exactly 3 aiagent.llm.response. Got: {event_counts}"
        )
        assert event_counts.get("aiagent.llm.tool_call", 0) == 3, (
            f"Expected exactly 3 aiagent.llm.tool_call. Got: {event_counts}"
        )
        assert event_counts.get("aiagent.workflow.validation_attempt", 0) == 4, (
            f"Expected exactly 4 aiagent.workflow.validation_attempt (initial + 3 retries). Got: {event_counts}"
        )
        assert event_counts.get("aiagent.response.complete", 0) == 1, (
            f"Expected exactly 1 aiagent.response.complete. Got: {event_counts}"
        )

        # Verify all events share the same correlation_id
        for event in all_events:
            assert event.correlation_id == remediation_id, (
                f"Event correlation_id mismatch: {event.correlation_id} != {remediation_id}"
            )

    def test_concurrent_sessions_independent(
        self, hapi_client, unique_test_id
    ):
        """
        IT-HAPI-064-004: Concurrent sessions complete independently.

        BR-AA-HAPI-064: Multiple investigations submitted in sequence do not
        interfere with each other. Each produces correct, independent results.
        """
        request_a = {
            "incident_id": f"inc-it-session-004a-{unique_test_id}",
            "remediation_id": f"rem-it-session-004a-{unique_test_id}",
            "signal_name": "OOMKilled",
            "severity": "critical",
            "signal_source": "prometheus",
            "resource_kind": "Pod",
            "resource_name": "concurrent-pod-a",
            "resource_namespace": "default",
            "cluster_name": "integration-test",
            "environment": "testing",
            "priority": "P1",
            "risk_tolerance": "low",
            "business_category": "test",
            "error_message": "Concurrent test A",
        }

        request_b = {
            "incident_id": f"inc-it-session-004b-{unique_test_id}",
            "remediation_id": f"rem-it-session-004b-{unique_test_id}",
            "signal_name": "CrashLoopBackOff",
            "severity": "high",
            "signal_source": "prometheus",
            "resource_kind": "Pod",
            "resource_name": "concurrent-pod-b",
            "resource_namespace": "default",
            "cluster_name": "integration-test",
            "environment": "testing",
            "priority": "P2",
            "risk_tolerance": "medium",
            "business_category": "test",
            "error_message": "Concurrent test B",
        }

        # Submit both investigations
        result_a = _submit_and_get_result(hapi_client, "/api/v1/incident", request_a)
        result_b = _submit_and_get_result(hapi_client, "/api/v1/incident", request_b)

        # Verify independent session IDs
        assert result_a["session_id"] != result_b["session_id"], (
            "Concurrent sessions should have different session IDs"
        )

        # Verify both completed independently
        assert result_a["status_response"]["status"] == "completed"
        assert result_b["status_response"]["status"] == "completed"

        # Verify results contain correct incident IDs
        assert result_a["result_response"]["incident_id"] == request_a["incident_id"]
        assert result_b["result_response"]["incident_id"] == request_b["incident_id"]


# ========================================
# RECOVERY SESSION FLOW TESTS
# ========================================

class TestRecoverySessionIntegration:
    """
    IT-HAPI-064-005 to 006: Recovery session lifecycle via HTTP endpoints.
    """

    def test_recovery_submit_poll_result_lifecycle(
        self, hapi_client, unique_test_id
    ):
        """
        IT-HAPI-064-005: Recovery submit + poll + result via TestClient.

        BR-AA-HAPI-064.9: Full recovery session lifecycle mirrors incident.
        """
        request_data = {
            "incident_id": f"inc-it-session-005-{unique_test_id}",
            "remediation_id": f"rem-it-session-005-{unique_test_id}",
            "signal_name": "OOMKilled",
            "previous_workflow_id": "oomkill-increase-memory-v1",
            "previous_workflow_result": "Failed",
            "resource_namespace": "default",
            "resource_name": "recovery-session-pod",
            "resource_kind": "Pod",
        }

        result = _submit_and_get_result(hapi_client, "/api/v1/recovery", request_data)

        # Verify session completed
        assert result["status_response"]["status"] == "completed"

        # Verify result contains expected recovery fields
        recovery_result = result["result_response"]
        assert "incident_id" in recovery_result, "Result should contain incident_id"

    def test_recovery_session_audit_events_exact_counts(
        self, hapi_client, audit_store, unique_test_id
    ):
        """
        IT-HAPI-064-006: Recovery session audit events emitted with exact counts.

        BR-AA-HAPI-064.9, BR-AUDIT-005: Recovery audit trail.

        Expected exact counts:
        - aiagent.llm.request: exactly 1
        - aiagent.llm.response: exactly 1
        - Total HAPI events: at least 2
        """
        remediation_id = f"rem-it-session-006-{unique_test_id}"
        request_data = {
            "incident_id": f"inc-it-session-006-{unique_test_id}",
            "remediation_id": remediation_id,
            "signal_name": "OOMKilled",
            "previous_workflow_id": "oomkill-increase-memory-v1",
            "previous_workflow_result": "Failed",
            "resource_namespace": "default",
            "resource_name": "recovery-audit-pod",
            "resource_kind": "Pod",
        }

        result = _submit_and_get_result(hapi_client, "/api/v1/recovery", request_data)
        assert result["status_response"]["status"] == "completed"

        # Verify audit events
        all_events = query_audit_events_with_retry(
            audit_store=audit_store,
            correlation_id=remediation_id,
            event_category="aiagent",
            event_type=None,
            min_expected_events=2,
            timeout_seconds=15,
        )

        event_counts = {}
        for e in all_events:
            event_counts[e.event_type] = event_counts.get(e.event_type, 0) + 1

        assert event_counts.get("aiagent.llm.request", 0) == 1, (
            f"Expected exactly 1 aiagent.llm.request. Got: {event_counts}"
        )
        assert event_counts.get("aiagent.llm.response", 0) == 1, (
            f"Expected exactly 1 aiagent.llm.response. Got: {event_counts}"
        )

        # Verify correlation ID
        for event in all_events:
            assert event.correlation_id == remediation_id, (
                f"Event correlation_id mismatch: {event.correlation_id} != {remediation_id}"
            )


# ========================================
# SESSION ERROR HANDLING TESTS
# ========================================

class TestSessionErrorHandling:
    """
    Additional session error handling integration tests.
    """

    def test_unknown_session_returns_404(self, hapi_client):
        """
        IT-HAPI-064-EXTRA: Unknown session ID returns 404 (session lost scenario).

        BR-AA-HAPI-064.5: 404 signals to AA controller that HAPI restarted.
        """
        resp = hapi_client.get("/api/v1/incident/session/nonexistent-session-id")
        assert resp.status_code == 404

    def test_pending_session_result_returns_409(self, hapi_client):
        """
        IT-HAPI-064-EXTRA: Requesting result of a pending session returns 409.

        BR-AA-HAPI-064.3: 409 signals session is not yet completed.
        """
        # Create a session manually (no background task)
        sm = get_session_manager()
        session_id = sm.create_session("incident", {"test": True})

        # Try to get result before completion
        resp = hapi_client.get(f"/api/v1/incident/session/{session_id}/result")
        assert resp.status_code == 409

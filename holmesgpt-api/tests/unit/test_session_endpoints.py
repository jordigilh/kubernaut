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
Session HTTP Endpoint Unit Tests (BR-AA-HAPI-064)

Test Plan: docs/testing/BR-AA-HAPI-064/session_based_pull_test_plan_v1.0.md
Section: 2.4, 2.5 -- HTTP Endpoints (Incident and Recovery)

Tests: UT-HAPI-064-011 through UT-HAPI-064-019

Note on BackgroundTasks behavior:
FastAPI's TestClient (Starlette) executes BackgroundTasks synchronously after the
response handler completes. This means by the time client.post() returns to the test,
the background investigation task has already run. For tests that need a "pending"
session (409 tests), we create the session directly via SessionManager without
triggering the background task.
"""

import pytest
import uuid
from unittest.mock import AsyncMock, patch


# ========================================
# Fixtures
# ========================================

@pytest.fixture
def sample_incident_request():
    """Valid IncidentRequest body for session endpoint tests (all required fields)."""
    return {
        "incident_id": "test-incident-endpoint-001",
        "remediation_id": "rem-endpoint-test-001",
        "signal_type": "OOMKilled",
        "severity": "high",
        "signal_source": "kube-state-metrics",
        "resource_namespace": "default",
        "resource_kind": "Pod",
        "resource_name": "test-pod",
        "error_message": "Container killed due to OOMKilled",
        "environment": "production",
        "priority": "P0",
        "risk_tolerance": "medium",
        "business_category": "standard",
        "cluster_name": "test-cluster",
    }


@pytest.fixture
def sample_recovery_request():
    """Valid RecoveryRequest body for session endpoint tests (all required fields)."""
    return {
        "incident_id": "test-recovery-endpoint-001",
        "remediation_id": "rem-recovery-endpoint-001",
        "is_recovery_attempt": True,
        "recovery_attempt_number": 1,
        "signal_type": "OOMKilled",
        "severity": "high",
        "resource_namespace": "default",
        "resource_kind": "Pod",
        "resource_name": "test-pod",
        "environment": "production",
        "priority": "P0",
        "risk_tolerance": "medium",
        "business_category": "standard",
        "cluster_name": "test-cluster",
        "error_message": "Container killed due to OOMKilled",
    }


@pytest.fixture
def mock_analyze_incident():
    """
    Mock analyze_incident for unit tests.
    Returns a minimal valid IncidentResponse dict so the background task completes successfully.
    """
    mock_response = {
        "incident_id": "test-incident-endpoint-001",
        "remediation_id": "rem-endpoint-test-001",
        "analysis": "Root cause: OOM due to memory leak in the application",
        "confidence": 0.9,
        "needs_human_review": False,
        "human_review_reason": None,
        "warnings": [],
        "alternative_workflows": [],
    }

    with patch(
        "src.extensions.incident.endpoint.analyze_incident",
        new_callable=AsyncMock,
    ) as mock:
        mock.return_value = mock_response
        yield mock


@pytest.fixture
def mock_analyze_recovery_for_session():
    """
    Mock analyze_recovery for session endpoint tests.
    Returns a minimal valid RecoveryResponse dict so the background task completes successfully.
    """
    mock_response = {
        "incident_id": "test-recovery-endpoint-001",
        "remediation_id": "rem-recovery-endpoint-001",
        "can_recover": True,
        "strategies": [
            {
                "action_type": "increase_memory",
                "confidence": 0.85,
                "rationale": "Increase memory limit to prevent OOM",
                "estimated_risk": "low",
            }
        ],
        "primary_recommendation": "increase_memory",
        "analysis_confidence": 0.85,
        "needs_human_review": False,
        "human_review_reason": None,
        "warnings": [],
        "alternative_workflows": [],
    }

    with patch(
        "src.extensions.recovery.endpoint.analyze_recovery",
        new_callable=AsyncMock,
    ) as mock:
        mock.return_value = mock_response
        yield mock


# ========================================
# 2.4 HTTP Endpoints (Incident)
# ========================================


class TestIncidentSessionEndpoints:
    """Tests for incident session HTTP endpoints (UT-HAPI-064-011 to 015)."""

    def test_incident_analyze_returns_202_with_session_id(
        self, client, sample_incident_request, mock_analyze_incident
    ):
        """
        UT-HAPI-064-011: POST /api/v1/incident/analyze returns 202 with session_id.

        Business Outcome: AA receives immediate acknowledgment and a handle to poll.
        BR: BR-AA-HAPI-064.1
        """
        response = client.post("/api/v1/incident/analyze", json=sample_incident_request)

        assert response.status_code == 202, (
            f"Expected 202 Accepted for async session creation, got {response.status_code}"
        )

        data = response.json()
        assert "session_id" in data, "Response must include session_id"

        # session_id must be a valid UUID
        parsed = uuid.UUID(data["session_id"])
        assert str(parsed) == data["session_id"]

    def test_incident_session_status_returns_200(
        self, client, sample_incident_request, mock_analyze_incident
    ):
        """
        UT-HAPI-064-012: GET /api/v1/incident/session/{id} returns status.

        Business Outcome: AA can observe investigation progress.
        BR: BR-AA-HAPI-064.2
        """
        # Create a session (background task runs synchronously in TestClient)
        create_resp = client.post("/api/v1/incident/analyze", json=sample_incident_request)
        assert create_resp.status_code == 202
        session_id = create_resp.json()["session_id"]

        # Poll session status
        response = client.get(f"/api/v1/incident/session/{session_id}")

        assert response.status_code == 200
        data = response.json()
        assert "status" in data
        assert data["status"] in ("pending", "investigating", "completed", "failed")

    def test_incident_session_status_returns_404_for_unknown(self, client):
        """
        UT-HAPI-064-013: GET /api/v1/incident/session/{id} returns 404 for unknown.

        Business Outcome: AA detects HAPI restart (lost sessions) via standard HTTP semantics.
        BR: BR-AA-HAPI-064.5
        """
        fake_session_id = str(uuid.uuid4())

        response = client.get(f"/api/v1/incident/session/{fake_session_id}")

        assert response.status_code == 404

    def test_incident_session_result_returns_response(
        self, client, sample_incident_request, mock_analyze_incident
    ):
        """
        UT-HAPI-064-014: GET /api/v1/incident/session/{id}/result returns IncidentResponse.

        Business Outcome: AA retrieves the full investigation result after completion.
        BR: BR-AA-HAPI-064.3

        Note: BackgroundTasks run synchronously in TestClient, so by the time we
        call GET result, the mock analyze_incident has already completed and the
        session is in "completed" status.
        """
        # Create session -- background task runs analyze_incident (mocked) synchronously
        create_resp = client.post("/api/v1/incident/analyze", json=sample_incident_request)
        assert create_resp.status_code == 202
        session_id = create_resp.json()["session_id"]

        # Session should now be "completed" (background task ran synchronously)
        response = client.get(f"/api/v1/incident/session/{session_id}/result")

        assert response.status_code == 200, (
            f"Expected 200 for completed session result, got {response.status_code}"
        )

        data = response.json()
        assert data["incident_id"] == "test-incident-endpoint-001"
        assert data["confidence"] == 0.9

    def test_incident_session_result_returns_409_when_not_completed(self, client):
        """
        UT-HAPI-064-015: GET /api/v1/incident/session/{id}/result returns 409 when not completed.

        Business Outcome: AA is told to keep polling if result is not ready yet.
        BR: BR-AA-HAPI-064.3

        Note: We create the session directly via SessionManager (without triggering
        the background task) to keep it in "pending" status.
        """
        from src.session.session_manager import get_session_manager

        sm = get_session_manager()
        session_id = sm.create_session("incident", {"test": "data"})
        # Session is "pending" -- no background task started

        response = client.get(f"/api/v1/incident/session/{session_id}/result")

        assert response.status_code == 409, (
            f"Expected 409 Conflict for pending session result, got {response.status_code}"
        )


# ========================================
# 2.5 HTTP Endpoints (Recovery -- Dedicated)
# ========================================


class TestRecoverySessionEndpoints:
    """Tests for recovery session HTTP endpoints (UT-HAPI-064-016 to 019)."""

    def test_recovery_analyze_returns_202_with_session_id(
        self, client, sample_recovery_request, mock_analyze_recovery_for_session
    ):
        """
        UT-HAPI-064-016: POST /api/v1/recovery/analyze returns 202 with session_id.

        Business Outcome: Recovery investigations use the same async pattern as incident investigations.
        BR: BR-AA-HAPI-064.9
        """
        response = client.post("/api/v1/recovery/analyze", json=sample_recovery_request)

        assert response.status_code == 202, (
            f"Expected 202 Accepted for async recovery session, got {response.status_code}"
        )

        data = response.json()
        assert "session_id" in data

        # session_id must be a valid UUID
        parsed = uuid.UUID(data["session_id"])
        assert str(parsed) == data["session_id"]

    def test_recovery_session_status_returns_200(
        self, client, sample_recovery_request, mock_analyze_recovery_for_session
    ):
        """
        UT-HAPI-064-017: GET /api/v1/recovery/session/{id} returns status.

        Business Outcome: AA can observe recovery investigation progress.
        BR: BR-AA-HAPI-064.9
        """
        # Create a recovery session
        create_resp = client.post("/api/v1/recovery/analyze", json=sample_recovery_request)
        assert create_resp.status_code == 202
        session_id = create_resp.json()["session_id"]

        # Poll session status
        response = client.get(f"/api/v1/recovery/session/{session_id}")

        assert response.status_code == 200
        data = response.json()
        assert "status" in data
        assert data["status"] in ("pending", "investigating", "completed", "failed")

    def test_recovery_session_result_returns_response(
        self, client, sample_recovery_request, mock_analyze_recovery_for_session
    ):
        """
        UT-HAPI-064-018: GET /api/v1/recovery/session/{id}/result returns RecoveryResponse.

        Business Outcome: AA retrieves the full recovery result after completion.
        BR: BR-AA-HAPI-064.9
        """
        # Create recovery session -- background task runs (mocked) synchronously
        create_resp = client.post("/api/v1/recovery/analyze", json=sample_recovery_request)
        assert create_resp.status_code == 202
        session_id = create_resp.json()["session_id"]

        # Session should now be "completed"
        response = client.get(f"/api/v1/recovery/session/{session_id}/result")

        assert response.status_code == 200, (
            f"Expected 200 for completed recovery result, got {response.status_code}"
        )

        data = response.json()
        assert data["incident_id"] == "test-recovery-endpoint-001"
        assert data["can_recover"] is True

    def test_recovery_session_result_returns_409_when_not_completed(self, client):
        """
        UT-HAPI-064-019: GET /api/v1/recovery/session/{id}/result returns 409 when not completed.

        Business Outcome: AA is told to keep polling if recovery result is not ready yet.
        BR: BR-AA-HAPI-064.9

        Note: Session created directly via SessionManager to keep in "pending" status.
        """
        from src.session.session_manager import get_session_manager

        sm = get_session_manager()
        session_id = sm.create_session("recovery", {"test": "data"})
        # Session is "pending" -- no background task started

        response = client.get(f"/api/v1/recovery/session/{session_id}/result")

        assert response.status_code == 409, (
            f"Expected 409 Conflict for pending recovery result, got {response.status_code}"
        )

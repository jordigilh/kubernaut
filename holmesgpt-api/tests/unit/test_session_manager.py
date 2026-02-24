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
Session Manager Unit Tests (BR-AA-HAPI-064)

Test Plan: docs/testing/BR-AA-HAPI-064/session_based_pull_test_plan_v1.0.md
Section: 2. Unit Tests -- HAPI (Python)

Tests: UT-HAPI-064-001 through UT-HAPI-064-010
TDD Phase: RED -- all tests expected to fail until GREEN phase implements SessionManager.
"""

import pytest
import uuid
import asyncio
from datetime import datetime, timezone, timedelta
from unittest.mock import AsyncMock, MagicMock

from src.session import SessionManager, SessionResultNotReady


# ========================================
# Fixtures
# ========================================

@pytest.fixture
def session_manager():
    """Create a fresh SessionManager instance for each test."""
    return SessionManager()


@pytest.fixture
def sample_incident_request_data():
    """Minimal incident request data for session creation."""
    return {
        "incident_id": "test-incident-001",
        "remediation_id": "rem-session-test-001",
        "signal_name": "OOMKilled",
        "severity": "high",
        "environment": "production",
        "business_priority": "P0",
        "target_resource": {
            "kind": "Pod",
            "name": "test-pod",
            "namespace": "default",
        },
    }


@pytest.fixture
def sample_recovery_request_data():
    """Minimal recovery request data for session creation."""
    return {
        "incident_id": "test-recovery-001",
        "remediation_id": "rem-recovery-session-001",
        "signal_name": "OOMKilled",
        "severity": "high",
        "is_recovery_attempt": True,
        "recovery_attempt_number": 1,
    }


# ========================================
# 2.1 SessionManager Core
# ========================================


class TestSessionManagerCore:
    """Tests for SessionManager core operations (UT-HAPI-064-001 to 005)."""

    def test_create_session_returns_uuid_and_stores_pending(
        self, session_manager, sample_incident_request_data
    ):
        """
        UT-HAPI-064-001: create_session() returns UUID and stores session with status "pending".

        Business Outcome: HAPI can accept an investigation request and return a session handle immediately.
        BR: BR-AA-HAPI-064.1
        """
        session_id = session_manager.create_session("incident", sample_incident_request_data)

        # Must return a valid UUID string
        assert session_id is not None
        parsed = uuid.UUID(session_id)  # Raises ValueError if not valid UUID
        assert str(parsed) == session_id

        # Session must be stored with status "pending" and created_at set
        session = session_manager.get_session(session_id)
        assert session is not None
        assert session["status"] == "pending"
        assert "created_at" in session
        assert isinstance(session["created_at"], datetime)

    def test_get_session_returns_status_for_existing(
        self, session_manager, sample_incident_request_data
    ):
        """
        UT-HAPI-064-002: get_session() returns session status for existing session.

        Business Outcome: AA can query the progress of an ongoing investigation.
        BR: BR-AA-HAPI-064.2
        """
        session_id = session_manager.create_session("incident", sample_incident_request_data)

        session = session_manager.get_session(session_id)

        assert session is not None
        assert "status" in session
        assert session["status"] in ("pending", "investigating", "completed", "failed")

    def test_get_session_returns_none_for_unknown(self, session_manager):
        """
        UT-HAPI-064-003: get_session() returns None for unknown session_id.

        Business Outcome: AA detects a lost session (HAPI restart) and can trigger regeneration.
        BR: BR-AA-HAPI-064.5
        """
        result = session_manager.get_session("nonexistent-session-id-12345")

        assert result is None

    def test_get_result_returns_response_when_completed(
        self, session_manager, sample_incident_request_data
    ):
        """
        UT-HAPI-064-004: get_result() returns IncidentResponse when status=completed.

        Business Outcome: AA retrieves a complete investigation result to advance the pipeline.
        BR: BR-AA-HAPI-064.3
        """
        session_id = session_manager.create_session("incident", sample_incident_request_data)

        # Manually set session to completed with a result (simulating background completion)
        session_manager._sessions[session_id]["status"] = "completed"
        session_manager._sessions[session_id]["result"] = {
            "incident_id": "test-incident-001",
            "analysis": "Root cause: OOM",
            "confidence": 0.9,
            "timestamp": "2026-02-09T10:00:00Z",
        }

        result = session_manager.get_result(session_id)

        assert result is not None
        assert result["incident_id"] == "test-incident-001"
        assert result["confidence"] == 0.9

    def test_get_result_raises_when_not_completed(
        self, session_manager, sample_incident_request_data
    ):
        """
        UT-HAPI-064-005: get_result() raises error when status != completed.

        Business Outcome: AA is prevented from reading partial results during an active investigation.
        BR: BR-AA-HAPI-064.3
        """
        session_id = session_manager.create_session("incident", sample_incident_request_data)

        # Session is still "pending" -- get_result should raise
        with pytest.raises(SessionResultNotReady) as exc_info:
            session_manager.get_result(session_id)

        assert exc_info.value.session_id == session_id
        assert exc_info.value.current_status == "pending"


# ========================================
# 2.2 TTL Cleanup
# ========================================


class TestSessionManagerTTLCleanup:
    """Tests for TTL cleanup (UT-HAPI-064-006 to 008)."""

    def test_expired_completed_sessions_removed(
        self, session_manager, sample_incident_request_data
    ):
        """
        UT-HAPI-064-006: Expired completed sessions are removed by cleanup.

        Business Outcome: HAPI memory does not grow unbounded from completed sessions.
        BR: BR-AA-HAPI-064.8
        """
        session_id = session_manager.create_session("incident", sample_incident_request_data)

        # Manually set session to completed 31 minutes ago
        session_manager._sessions[session_id]["status"] = "completed"
        session_manager._sessions[session_id]["completed_at"] = datetime.now(timezone.utc) - timedelta(minutes=31)

        removed = session_manager.cleanup_expired(ttl_minutes=30)

        assert removed >= 1
        assert session_manager.get_session(session_id) is None

    def test_active_investigating_sessions_preserved(
        self, session_manager, sample_incident_request_data
    ):
        """
        UT-HAPI-064-007: Active investigating sessions are preserved by cleanup.

        Business Outcome: Long-running LLM investigations are not prematurely garbage-collected.
        BR: BR-AA-HAPI-064.8
        """
        session_id = session_manager.create_session("incident", sample_incident_request_data)

        # Manually set session to investigating (5 minutes ago)
        session_manager._sessions[session_id]["status"] = "investigating"
        session_manager._sessions[session_id]["created_at"] = datetime.now(timezone.utc) - timedelta(minutes=5)

        removed = session_manager.cleanup_expired(ttl_minutes=30)

        assert removed == 0
        assert session_manager.get_session(session_id) is not None

    def test_failed_sessions_expire_like_completed(
        self, session_manager, sample_incident_request_data
    ):
        """
        UT-HAPI-064-008: Failed sessions expire like completed sessions.

        Business Outcome: Failed sessions don't leak memory either.
        BR: BR-AA-HAPI-064.8
        """
        session_id = session_manager.create_session("incident", sample_incident_request_data)

        # Manually set session to failed 31 minutes ago
        session_manager._sessions[session_id]["status"] = "failed"
        session_manager._sessions[session_id]["completed_at"] = datetime.now(timezone.utc) - timedelta(minutes=31)

        removed = session_manager.cleanup_expired(ttl_minutes=30)

        assert removed >= 1
        assert session_manager.get_session(session_id) is None


# ========================================
# 2.3 Background Execution
# ========================================


class TestSessionManagerBackgroundExecution:
    """Tests for background investigation execution (UT-HAPI-064-009 to 010)."""

    @pytest.mark.asyncio
    async def test_background_task_transitions_to_completed(
        self, session_manager, sample_incident_request_data
    ):
        """
        UT-HAPI-064-009: Background task transitions session pending -> investigating -> completed.

        Business Outcome: Investigation runs to completion without blocking the HTTP response.
        BR: BR-AA-HAPI-064.1
        """
        session_id = session_manager.create_session("incident", sample_incident_request_data)

        # Mock investigation function that returns a successful result
        mock_result = {
            "incident_id": "test-incident-001",
            "analysis": "Root cause: OOM due to memory leak",
            "confidence": 0.9,
            "timestamp": "2026-02-09T10:00:00Z",
        }
        mock_investigate = AsyncMock(return_value=mock_result)

        # Run the background task
        await session_manager.run_investigation(
            session_id, mock_investigate, sample_incident_request_data
        )

        # Verify final state
        session = session_manager.get_session(session_id)
        assert session is not None
        assert session["status"] == "completed"

        result = session_manager.get_result(session_id)
        assert result["incident_id"] == "test-incident-001"
        assert result["confidence"] == 0.9

    @pytest.mark.asyncio
    async def test_background_task_handles_exception(
        self, session_manager, sample_incident_request_data
    ):
        """
        UT-HAPI-064-010: Background task handles investigate_issues() exception.

        Business Outcome: LLM failures are captured in the session, not lost silently.
        BR: BR-AA-HAPI-064.1
        """
        session_id = session_manager.create_session("incident", sample_incident_request_data)

        # Mock investigation function that raises an exception
        mock_investigate = AsyncMock(side_effect=RuntimeError("LLM provider timeout"))

        # Run the background task -- should NOT raise
        await session_manager.run_investigation(
            session_id, mock_investigate, sample_incident_request_data
        )

        # Verify session transitioned to failed with error details
        session = session_manager.get_session(session_id)
        assert session is not None
        assert session["status"] == "failed"
        assert "error" in session
        assert "LLM provider timeout" in session["error"]

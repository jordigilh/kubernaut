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
E2E Tests for Audit Pipeline

Business Requirement: BR-AUDIT-005 (Workflow Selection Audit Trail)
Design Decisions:
  - ADR-038: Asynchronous Buffered Audit Trace Ingestion
  - DD-AUDIT-002: Audit Shared Library Design

These tests verify that:
1. Audit events are emitted during incident analysis
2. Audit events reach the REAL Data Storage service
3. Complete audit trail is preserved (LLM I/O + validation attempts)

Test Strategy (per TESTING_GUIDELINES.md):
- Uses REAL Data Storage service (REQUIRED - tests FAIL if unavailable)
- Mocks ONLY the LLM (due to cost)
- Queries Data Storage to verify events persisted in database
"""

import os
import time
import pytest
import requests
from typing import List, Dict, Any
from unittest.mock import patch, Mock


# ============================================================================
# FIXTURES
# ============================================================================

@pytest.fixture(scope="module")
def data_storage_url():
    """
    Get Data Storage URL from environment.

    E2E tests REQUIRE a running Data Storage service.
    Tests will FAIL (not skip) if Data Storage is unavailable.
    """
    url = os.environ.get("DATA_STORAGE_URL", "http://localhost:8080")

    # Verify Data Storage is accessible - FAIL if not available
    response = requests.get(f"{url}/health", timeout=5)
    assert response.status_code == 200, f"Data Storage not healthy at {url}"

    return url


@pytest.fixture
def mock_llm_response_valid():
    """
    Mock LLM response with valid workflow.

    LLM is the ONLY component we mock (due to cost).
    """
    mock = Mock()
    mock.analysis = '''
Based on my investigation, the pod was killed due to OOM.

```json
{
  "root_cause_analysis": {
    "summary": "Container exceeded memory limit",
    "severity": "critical",
    "contributing_factors": ["Memory leak", "Insufficient limits"]
  },
  "selected_workflow": {
    "workflow_id": "restart-pod-v1",
    "version": "1.0.0",
    "container_image": "ghcr.io/kubernaut/restart:v1.0.0",
    "confidence": 0.95,
    "rationale": "Standard OOM recovery",
    "parameters": {
      "namespace": "production",
      "pod_name": "app-xyz-123"
    }
  }
}
```
'''
    mock.tool_calls = []
    return mock


@pytest.fixture
def mock_llm_response_invalid_workflow():
    """Mock LLM response with invalid workflow (triggers validation retry)."""
    mock = Mock()
    mock.analysis = '''
```json
{
  "root_cause_analysis": {
    "summary": "Container OOM",
    "severity": "critical",
    "contributing_factors": []
  },
  "selected_workflow": {
    "workflow_id": "hallucinated-workflow-xyz",
    "confidence": 0.85,
    "parameters": {}
  }
}
```
'''
    mock.tool_calls = []
    return mock


@pytest.fixture
def unique_incident_id():
    """Generate unique incident ID for each test to avoid collisions."""
    import uuid
    return f"e2e-audit-{uuid.uuid4().hex[:8]}"


@pytest.fixture
def unique_remediation_id():
    """Generate unique remediation ID for each test."""
    import uuid
    return f"rem-audit-{uuid.uuid4().hex[:8]}"


def query_audit_events(
    data_storage_url: str,
    incident_id: str,
    timeout: int = 10
) -> List[Dict[str, Any]]:
    """
    Query Data Storage for audit events by incident_id.

    Args:
        data_storage_url: Data Storage service URL
        incident_id: Incident ID to filter events
        timeout: Request timeout in seconds

    Returns:
        List of audit events
    """
    response = requests.get(
        f"{data_storage_url}/api/v1/audit/events",
        params={"incident_id": incident_id},
        timeout=timeout
    )
    response.raise_for_status()
    return response.json().get("events", [])


def wait_for_audit_flush(seconds: float = 6.0):
    """
    Wait for audit buffer to flush to Data Storage.

    BufferedAuditStore has flush_interval_seconds=5.0 by default.
    """
    time.sleep(seconds)


# ============================================================================
# E2E TESTS - REAL DATA STORAGE, MOCK LLM ONLY
# ============================================================================

@pytest.mark.e2e
class TestAuditPipelineE2E:
    """
    E2E tests for audit event pipeline with REAL Data Storage.

    Business Requirement: BR-AUDIT-005
    Design Decision: DD-AUDIT-002

    These tests:
    - Connect to REAL Data Storage service (REQUIRED)
    - Mock ONLY the LLM (due to cost)
    - Verify audit events are persisted in database
    """

    def test_llm_request_event_persisted(
        self,
        data_storage_url,
        mock_llm_response_valid,
        unique_incident_id,
        unique_remediation_id
    ):
        """
        BR-AUDIT-005: LLM request audit events are persisted in Data Storage.

        Verifies:
        - llm_request event stored in database
        - Correlation IDs match
        - Prompt information captured
        """
        os.environ["DATA_STORAGE_URL"] = data_storage_url
        os.environ["LLM_PROVIDER"] = "openai"
        os.environ["LLM_MODEL"] = "gpt-4"
        os.environ["OPENAI_API_KEY"] = "mock-key-e2e"

        # Reset audit store singleton
        import src.extensions.incident as incident_module
        incident_module._audit_store = None

        with patch("src.extensions.incident.investigate_issues", return_value=mock_llm_response_valid):
            with patch("src.extensions.incident.Config"):
                import asyncio
                from src.extensions.incident import analyze_incident

                request_data = {
                    "incident_id": unique_incident_id,
                    "remediation_id": unique_remediation_id,
                    "signal_type": "OOMKilled",
                    "severity": "critical",
                    "resource_namespace": "production",
                    "resource_kind": "Pod",
                    "resource_name": "app-xyz-123",
                    "cluster_name": "e2e-test-cluster",
                    "environment": "production",
                    "priority": "P1",
                    "risk_tolerance": "medium",
                    "business_category": "standard",
                    "error_message": "Container killed due to OOM",
                }

                # Run analysis
                result = asyncio.run(analyze_incident(request_data))

        # Force flush audit store
        if incident_module._audit_store:
            incident_module._audit_store.close()

        # Wait for flush to complete
        wait_for_audit_flush()

        # Query Data Storage for persisted events
        events = query_audit_events(data_storage_url, unique_incident_id)

        # Verify llm_request event exists
        llm_requests = [e for e in events if e.get("event_type") == "llm_request"]
        assert len(llm_requests) >= 1, "llm_request event not found in Data Storage"

        # Verify correlation IDs
        event = llm_requests[0]
        assert event["incident_id"] == unique_incident_id
        assert event["remediation_id"] == unique_remediation_id
        assert "prompt_length" in event or "prompt_preview" in event

    def test_llm_response_event_persisted(
        self,
        data_storage_url,
        mock_llm_response_valid,
        unique_incident_id,
        unique_remediation_id
    ):
        """
        BR-AUDIT-005: LLM response audit events are persisted in Data Storage.
        """
        os.environ["DATA_STORAGE_URL"] = data_storage_url
        os.environ["LLM_PROVIDER"] = "openai"
        os.environ["LLM_MODEL"] = "gpt-4"
        os.environ["OPENAI_API_KEY"] = "mock-key-e2e"

        import src.extensions.incident as incident_module
        incident_module._audit_store = None

        with patch("src.extensions.incident.investigate_issues", return_value=mock_llm_response_valid):
            with patch("src.extensions.incident.Config"):
                import asyncio
                from src.extensions.incident import analyze_incident

                request_data = {
                    "incident_id": unique_incident_id,
                    "remediation_id": unique_remediation_id,
                    "signal_type": "CrashLoopBackOff",
                    "severity": "high",
                    "resource_namespace": "default",
                    "resource_kind": "Pod",
                    "resource_name": "crash-pod",
                    "cluster_name": "e2e-test",
                    "environment": "staging",
                    "priority": "P2",
                    "risk_tolerance": "high",
                    "business_category": "test",
                    "error_message": "Container crash",
                }

                result = asyncio.run(analyze_incident(request_data))

        if incident_module._audit_store:
            incident_module._audit_store.close()

        wait_for_audit_flush()

        events = query_audit_events(data_storage_url, unique_incident_id)

        # Verify llm_response event exists
        llm_responses = [e for e in events if e.get("event_type") == "llm_response"]
        assert len(llm_responses) >= 1, "llm_response event not found in Data Storage"

        event = llm_responses[0]
        assert event["incident_id"] == unique_incident_id
        assert "has_analysis" in event
        assert "analysis_length" in event

    def test_validation_attempt_event_persisted(
        self,
        data_storage_url,
        mock_llm_response_valid,
        unique_incident_id,
        unique_remediation_id
    ):
        """
        DD-HAPI-002 v1.2: Validation attempt audit events are persisted.
        """
        os.environ["DATA_STORAGE_URL"] = data_storage_url
        os.environ["LLM_PROVIDER"] = "openai"
        os.environ["LLM_MODEL"] = "gpt-4"
        os.environ["OPENAI_API_KEY"] = "mock-key-e2e"

        import src.extensions.incident as incident_module
        incident_module._audit_store = None

        with patch("src.extensions.incident.investigate_issues", return_value=mock_llm_response_valid):
            with patch("src.extensions.incident.Config"):
                import asyncio
                from src.extensions.incident import analyze_incident

                request_data = {
                    "incident_id": unique_incident_id,
                    "remediation_id": unique_remediation_id,
                    "signal_type": "OOMKilled",
                    "severity": "critical",
                    "resource_namespace": "prod",
                    "resource_kind": "Pod",
                    "resource_name": "validation-test-pod",
                    "cluster_name": "e2e-cluster",
                    "environment": "production",
                    "priority": "P1",
                    "risk_tolerance": "medium",
                    "business_category": "standard",
                    "error_message": "OOM",
                }

                result = asyncio.run(analyze_incident(request_data))

        if incident_module._audit_store:
            incident_module._audit_store.close()

        wait_for_audit_flush()

        events = query_audit_events(data_storage_url, unique_incident_id)

        # Verify validation attempt event exists
        validation_events = [e for e in events if e.get("event_type") == "workflow_validation_attempt"]
        assert len(validation_events) >= 1, "workflow_validation_attempt event not found in Data Storage"

        event = validation_events[0]
        assert event["incident_id"] == unique_incident_id
        assert "attempt" in event
        assert "max_attempts" in event
        assert "is_valid" in event

    def test_complete_audit_trail_persisted(
        self,
        data_storage_url,
        mock_llm_response_valid,
        unique_incident_id,
        unique_remediation_id
    ):
        """
        BR-AUDIT-005: Complete audit trail (all event types) persisted.

        Verifies all expected event types are in Data Storage:
        - llm_request
        - llm_response
        - workflow_validation_attempt
        """
        os.environ["DATA_STORAGE_URL"] = data_storage_url
        os.environ["LLM_PROVIDER"] = "openai"
        os.environ["LLM_MODEL"] = "gpt-4"
        os.environ["OPENAI_API_KEY"] = "mock-key-e2e"

        import src.extensions.incident as incident_module
        incident_module._audit_store = None

        with patch("src.extensions.incident.investigate_issues", return_value=mock_llm_response_valid):
            with patch("src.extensions.incident.Config"):
                import asyncio
                from src.extensions.incident import analyze_incident

                request_data = {
                    "incident_id": unique_incident_id,
                    "remediation_id": unique_remediation_id,
                    "signal_type": "OOMKilled",
                    "severity": "critical",
                    "resource_namespace": "production",
                    "resource_kind": "Pod",
                    "resource_name": "complete-audit-test",
                    "cluster_name": "e2e-cluster",
                    "environment": "production",
                    "priority": "P1",
                    "risk_tolerance": "medium",
                    "business_category": "standard",
                    "error_message": "OOM for complete audit test",
                }

                result = asyncio.run(analyze_incident(request_data))

        if incident_module._audit_store:
            incident_module._audit_store.close()

        wait_for_audit_flush()

        events = query_audit_events(data_storage_url, unique_incident_id)

        # Verify all event types present
        event_types = {e.get("event_type") for e in events}

        assert "llm_request" in event_types, "Missing llm_request in audit trail"
        assert "llm_response" in event_types, "Missing llm_response in audit trail"
        assert "workflow_validation_attempt" in event_types, "Missing workflow_validation_attempt in audit trail"

        # Verify all events have consistent correlation IDs
        for event in events:
            assert event["incident_id"] == unique_incident_id, f"Inconsistent incident_id in {event['event_type']}"
            assert event["remediation_id"] == unique_remediation_id, f"Inconsistent remediation_id in {event['event_type']}"

    def test_validation_retry_events_persisted(
        self,
        data_storage_url,
        mock_llm_response_invalid_workflow,
        unique_incident_id,
        unique_remediation_id
    ):
        """
        DD-HAPI-002 v1.2: Multiple validation attempts persisted during retry loop.

        When LLM returns invalid workflow, HAPI retries up to 3 times.
        Each attempt should be audited.
        """
        os.environ["DATA_STORAGE_URL"] = data_storage_url
        os.environ["LLM_PROVIDER"] = "openai"
        os.environ["LLM_MODEL"] = "gpt-4"
        os.environ["OPENAI_API_KEY"] = "mock-key-e2e"

        import src.extensions.incident as incident_module
        incident_module._audit_store = None

        # LLM always returns invalid workflow (triggers max retries)
        with patch("src.extensions.incident.investigate_issues", return_value=mock_llm_response_invalid_workflow):
            with patch("src.extensions.incident.Config"):
                import asyncio
                from src.extensions.incident import analyze_incident

                request_data = {
                    "incident_id": unique_incident_id,
                    "remediation_id": unique_remediation_id,
                    "signal_type": "OOMKilled",
                    "severity": "critical",
                    "resource_namespace": "production",
                    "resource_kind": "Pod",
                    "resource_name": "retry-test-pod",
                    "cluster_name": "e2e-cluster",
                    "environment": "production",
                    "priority": "P1",
                    "risk_tolerance": "medium",
                    "business_category": "standard",
                    "error_message": "OOM for retry test",
                }

                result = asyncio.run(analyze_incident(request_data))

        if incident_module._audit_store:
            incident_module._audit_store.close()

        wait_for_audit_flush()

        events = query_audit_events(data_storage_url, unique_incident_id)

        # Should have multiple LLM requests (retries)
        llm_requests = [e for e in events if e.get("event_type") == "llm_request"]
        assert len(llm_requests) >= 1, "Should have at least 1 LLM request"

        # Should have validation attempts
        validation_events = [e for e in events if e.get("event_type") == "workflow_validation_attempt"]
        assert len(validation_events) >= 1, "Should have validation attempts"

        # Verify result indicates human review needed (all retries failed)
        assert result.get("needs_human_review") is True

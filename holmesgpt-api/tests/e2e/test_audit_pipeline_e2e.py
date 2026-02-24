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
from typing import List, Dict, Any
from unittest.mock import patch, Mock

# DD-API-001: Use OpenAPI generated clients for ALL REST API communication
from datastorage import ApiClient as DSApiClient, Configuration as DSConfiguration
from datastorage.api import AuditWriteAPIApi

import sys
from pathlib import Path
# Add tests/clients to path (absolute path resolution for CI)
sys.path.insert(0, str(Path(__file__).parent.parent / 'clients'))
from holmesgpt_api_client import ApiClient as HAPIApiClient, Configuration as HAPIConfiguration
from holmesgpt_api_client.api import IncidentAnalysisApi
from holmesgpt_api_client.models import IncidentRequest


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
    import requests  # Import only for health check (non-business API)
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
    "execution_bundle": "ghcr.io/kubernaut/restart:v1.0.0",
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


@pytest.fixture
def mock_config():
    """
    Create a properly mocked Config object with serializable attributes.

    This fixes the _asdict() MagicMock error when llm_request events
    try to serialize config.model, config.toolsets, etc.
    """
    mock = Mock()
    mock.model = "gpt-4-mock"
    mock.toolsets = {"kubernetes/core": {}}
    mock.mcp_servers = {}
    return mock


def query_audit_events(
    data_storage_url: str,
    correlation_id: str,
    timeout: int = 10
) -> List[Dict[str, Any]]:
    """
    Query Data Storage for audit events by correlation_id.

    ADR-034: correlation_id is the primary filter (maps to remediation_id).
    The incident_id is inside event_data JSONB.

    DD-API-001 COMPLIANCE: Uses OpenAPI generated client instead of direct HTTP.
    DD-AUTH-014: Uses shared pool manager for ServiceAccount authentication (E2E).

    Args:
        data_storage_url: Data Storage service URL
        correlation_id: Correlation ID (remediation_id) to filter events
        timeout: Request timeout in seconds

    Returns:
        List of audit events
    """
    # DD-AUTH-014: Import shared pool manager for ServiceAccount token injection
    import sys
    from pathlib import Path
    sys.path.insert(0, str(Path(__file__).parent.parent.parent / "src"))
    from clients.datastorage_pool_manager import get_shared_datastorage_pool_manager
    
    # DD-API-001: Use OpenAPI generated client for Data Storage
    config = DSConfiguration(host=data_storage_url)
    config.timeout = 60  # CRITICAL: Prevent "read timeout=0" errors
    with DSApiClient(config) as api_client:
        # DD-AUTH-014: Inject ServiceAccount token via shared pool manager (same pattern as bootstrap_workflows)
        auth_pool = get_shared_datastorage_pool_manager()
        api_client.rest_client.pool_manager = auth_pool
        
        api_instance = AuditWriteAPIApi(api_client)
        response = api_instance.query_audit_events(
            correlation_id=correlation_id,
            _request_timeout=timeout
        )
        # OpenAPI client returns AuditEventsQueryResponse model with Pydantic AuditEvent objects
        # Return as-is for type-safe attribute access in tests
        return response.data if hasattr(response, 'data') and response.data else []


def query_audit_events_with_retry(
    data_storage_url: str,
    correlation_id: str,
    min_expected_events: int = 1,
    timeout_seconds: int = 15,
    poll_interval: float = 0.5
) -> List[Dict[str, Any]]:
    """
    Query Data Storage for audit events with retry logic (Eventually pattern).

    ADR-038: Buffered audit store flushes asynchronously (flush_interval_seconds).
    Tests must poll for events rather than assuming immediate availability.
    DD-AUTH-014: Uses shared pool manager for ServiceAccount authentication (E2E).

    Pattern: Similar to Ginkgo's Eventually() - poll with timeout until events appear

    Args:
        data_storage_url: Data Storage service URL
        correlation_id: Remediation ID for audit correlation
        min_expected_events: Minimum number of events expected (default 1)
        timeout_seconds: Maximum time to wait for events (default 15s for E2E)
        poll_interval: Time between polling attempts (default 0.5s)

    Returns:
        List of audit events

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


def wait_for_audit_flush(seconds: float = 6.0):
    """
    DEPRECATED: Use query_audit_events_with_retry() instead.

    Wait for audit buffer to flush to Data Storage.

    BufferedAuditStore has flush_interval_seconds=5.0 by default.
    """
    time.sleep(seconds)


def call_hapi_incident_analyze(
    hapi_url: str,
    request_data: Dict[str, Any],
    timeout: float = 60.0,  # Increased from 30s to 60s (LLM calls can be slow, even Mock LLM)
    auth_token: str = None
) -> Dict[str, Any]:
    """
    Call HAPI's incident analysis API using OpenAPI generated client.

    DD-API-001 COMPLIANCE: Uses OpenAPI generated client instead of direct HTTP.
    DD-AUTH-014: Supports Bearer token authentication for E2E tests.

    Args:
        hapi_url: HAPI service URL
        request_data: IncidentRequest data as dictionary
        timeout: Request timeout in seconds (default: 60s to handle LLM processing delays)
        auth_token: Optional Bearer token for authentication (DD-AUTH-014)

    Returns:
        IncidentResponse as dictionary
    """
    # DD-API-001: Use OpenAPI generated client for HAPI
    config = HAPIConfiguration(host=hapi_url)
    config.timeout = 60  # CRITICAL: Prevent "read timeout=0" errors
    
    with HAPIApiClient(config) as api_client:
        # Set default timeout at pool manager level to prevent "read timeout=0" errors
        # This ensures a timeout is always set even if _request_timeout is not provided
        if hasattr(api_client.rest_client, 'pool_manager'):
            import urllib3
            api_client.rest_client.pool_manager.connection_pool_kw['timeout'] = urllib3.Timeout(connect=10.0, read=timeout)
        
        # DD-AUTH-014: Inject Bearer token via set_default_header (E2E tests)
        if auth_token:
            api_client.set_default_header('Authorization', f'Bearer {auth_token}')
        
        api_instance = IncidentAnalysisApi(api_client)

        # Convert dict to IncidentRequest model
        incident_request = IncidentRequest(**request_data)

        # Call the API with explicit float timeout (Pydantic requires StrictFloat)
        response = api_instance.incident_analyze_endpoint_api_v1_incident_analyze_post(
            incident_request=incident_request,
            _request_timeout=float(timeout)
        )

        # Convert response model to dict for easier assertions
        return response.to_dict()


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
        hapi_auth_token,
        mock_llm_response_valid,
        mock_config,
        unique_incident_id,
        unique_remediation_id
    ):
        """
        BR-AUDIT-005: LLM request audit events are persisted in Data Storage.

        Verifies:
        - aiagent.llm.request event stored in database
        - Correlation IDs match
        - Prompt information captured

        NOTE: This test calls the REAL HAPI HTTP API (not direct function imports)
        HAPI service is configured with MOCK_LLM_MODE=true in E2E environment
        DD-AUTH-014: Uses ServiceAccount token for authentication
        """
        hapi_url = os.environ.get("HAPI_BASE_URL", "http://localhost:30120")

        request_data = {
            "incident_id": unique_incident_id,
            "remediation_id": unique_remediation_id,
            "signal_name": "OOMKilled",
            "severity": "critical",
            "signal_source": "prometheus",  # REQUIRED field
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

        # DD-API-001: Call HAPI using OpenAPI generated client
        # DD-AUTH-014: Pass ServiceAccount token for authentication
        result = call_hapi_incident_analyze(hapi_url, request_data, auth_token=hapi_auth_token)

        # Verify HAPI returned analysis
        assert "root_cause_analysis" in result or "selected_workflow" in result, \
            "HAPI response missing analysis fields"

        # Query Data Storage for persisted events with retry (ADR-038: buffered async flush)
        # DD-AUTH-014: Uses ServiceAccount token via datastorage_pool_manager (mounted file)
        events = query_audit_events_with_retry(
            data_storage_url,
            unique_remediation_id,
            min_expected_events=1,  # Retry until at least 1 event appears
            timeout_seconds=15  # E2E may be slower
        )

        # Verify aiagent.llm.request event exists (Pydantic model attribute access)
        llm_requests = [e for e in events if e.event_type == "aiagent.llm.request"]
        # DD-TESTING-001: Deterministic count - exactly 1 LLM call = 1 aiagent.llm.request event
        assert len(llm_requests) == 1, f"Expected exactly 1 aiagent.llm.request event. Found events: {[e.event_type for e in events]}"

        # ADR-034: incident_id and prompt info are in event_data
        event = llm_requests[0]
        assert event.correlation_id == unique_remediation_id
        # event_data is a oneOf discriminated union - access actual_instance
        event_data = event.event_data if hasattr(event, 'event_data') else None
        assert event_data is not None, "event_data should be present"
        assert hasattr(event_data, 'actual_instance'), "event_data should have actual_instance (oneOf wrapper)"
        payload = event_data.actual_instance
        assert hasattr(payload, 'incident_id'), "payload should have incident_id"
        assert payload.incident_id == unique_incident_id
        assert hasattr(payload, 'prompt_length') or hasattr(payload, 'prompt_preview'), \
            "payload should have prompt_length or prompt_preview"

    def test_llm_response_event_persisted(
        self,
        data_storage_url,
        hapi_auth_token,
        mock_llm_response_valid,
        mock_config,
        unique_incident_id,
        unique_remediation_id
    ):
        """
        BR-AUDIT-005: LLM response audit events are persisted in Data Storage.

        NOTE: Calls REAL HAPI HTTP API with MOCK_LLM_MODE=true
        """
        hapi_url = os.environ.get("HAPI_BASE_URL", "http://localhost:30120")

        request_data = {
            "incident_id": unique_incident_id,
            "remediation_id": unique_remediation_id,
            "signal_name": "CrashLoopBackOff",
            "severity": "high",
            "signal_source": "prometheus",  # REQUIRED field
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

        # DD-API-001: Call HAPI using OpenAPI generated client
        # DD-AUTH-014: Pass ServiceAccount token for authentication
        result = call_hapi_incident_analyze(hapi_url, request_data, auth_token=hapi_auth_token)

        # Query Data Storage for persisted events with retry (ADR-038: buffered async flush)
        events = query_audit_events_with_retry(
            data_storage_url,
            unique_remediation_id,
            min_expected_events=1,  # Retry until at least 1 event appears
            timeout_seconds=30  # Increased for E2E with real LLM mock delays
        )

        # Verify aiagent.llm.response event exists (Pydantic model attribute access)
        llm_responses = [e for e in events if e.event_type == "aiagent.llm.response"]
        # DD-TESTING-001: Deterministic count - Mock LLM makes exactly 1 call = 1 aiagent.llm.response
        assert len(llm_responses) == 1, f"Expected exactly 1 aiagent.llm.response event. Found events: {[e.event_type for e in events]}"

        # ADR-034: Fields are in event_data
        event = llm_responses[0]
        assert event.correlation_id == unique_remediation_id
        # event_data is a oneOf discriminated union - access actual_instance
        event_data = event.event_data if hasattr(event, 'event_data') else None
        assert event_data is not None, "event_data should be present"
        assert hasattr(event_data, 'actual_instance'), "event_data should have actual_instance (oneOf wrapper)"
        payload = event_data.actual_instance
        assert hasattr(payload, 'incident_id') and payload.incident_id == unique_incident_id
        assert hasattr(payload, 'has_analysis'), "payload should have has_analysis"
        assert hasattr(payload, 'analysis_length'), "payload should have analysis_length"

    def test_validation_attempt_event_persisted(
        self,
        data_storage_url,
        hapi_auth_token,
        mock_llm_response_valid,
        mock_config,
        unique_incident_id,
        unique_remediation_id
    ):
        """
        DD-HAPI-002 v1.2: Validation attempt audit events are persisted.

        NOTE: Calls REAL HAPI HTTP API with MOCK_LLM_MODE=true
        """
        hapi_url = os.environ.get("HAPI_BASE_URL", "http://localhost:30120")

        request_data = {
            "incident_id": unique_incident_id,
            "remediation_id": unique_remediation_id,
            "signal_name": "OOMKilled",
            "severity": "critical",
            "signal_source": "prometheus",  # REQUIRED field
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

        # DD-API-001: Call HAPI using OpenAPI generated client
        # DD-AUTH-014: Pass ServiceAccount token for authentication
        result = call_hapi_incident_analyze(hapi_url, request_data, auth_token=hapi_auth_token)

        # Query Data Storage for persisted events with retry (ADR-038: buffered async flush)
        # Complete flow: aiagent.llm.request + aiagent.llm.response + aiagent.workflow.validation_attempt
        events = query_audit_events_with_retry(
            data_storage_url,
            unique_remediation_id,
            min_expected_events=5,  # Retry until at least 5 events appear (request + response + validation attempts)
            timeout_seconds=30  # Increased for E2E with real LLM mock delays
        )

        # Verify validation attempt event exists (Pydantic model attribute access)
        validation_events = [e for e in events if e.event_type == "aiagent.workflow.validation_attempt"]
        # DD-TESTING-001: Deterministic count - Mock LLM returns valid workflow on first try = exactly 1 validation attempt
        assert len(validation_events) == 1, f"Expected exactly 1 aiagent.workflow.validation_attempt event. Found events: {[e.event_type for e in events]}"

        # Verify all validation events have correct correlation_id
        for event in validation_events:
            assert event.correlation_id == unique_remediation_id, f"Validation event should have correlation_id {unique_remediation_id}"

            # ADR-034: Fields are in event_data
            # event_data is a oneOf discriminated union - access actual_instance
            event_data = event.event_data if hasattr(event, 'event_data') else None
            assert event_data is not None, "event_data should be present"
            assert hasattr(event_data, 'actual_instance'), "event_data should have actual_instance (oneOf wrapper)"
            payload = event_data.actual_instance
            assert hasattr(payload, 'incident_id') and payload.incident_id == unique_incident_id, \
                f"Validation event should have incident_id {unique_incident_id}"

        # DD-HAPI-002 v1.2: Verify final attempt marker
        # - Single attempt success: is_final_attempt may not be present (validation succeeded on first try)
        # - Multi-attempt (self-correction): Last attempt should have is_final_attempt=True
        final_attempts = [e for e in validation_events
                         if hasattr(e.event_data.actual_instance, 'is_final_attempt')
                         and e.event_data.actual_instance.is_final_attempt]

        if len(validation_events) > 1:
            # Multi-attempt scenario: Require is_final_attempt=True on last attempt
            assert len(final_attempts) >= 1, \
                f"Multi-attempt validation (count={len(validation_events)}) should have is_final_attempt=True"
        # else: Single attempt success - is_final_attempt is optional

        assert hasattr(payload, 'attempt'), "payload should have attempt"
        assert hasattr(payload, 'max_attempts'), "payload should have max_attempts"
        assert hasattr(payload, 'is_valid'), "payload should have is_valid"

    def test_complete_audit_trail_persisted(
        self,
        data_storage_url,
        hapi_auth_token,
        mock_llm_response_valid,
        mock_config,
        unique_incident_id,
        unique_remediation_id,
        test_workflows_bootstrapped  # Ensure workflows are seeded for validation
    ):
        """
        BR-AUDIT-005: Complete audit trail (all event types) persisted.

        Verifies all expected event types are in Data Storage:
        - aiagent.llm.request
        - aiagent.llm.response
        - aiagent.workflow.validation_attempt

        NOTE: Calls REAL HAPI HTTP API with MOCK_LLM_MODE=true
        """
        hapi_url = os.environ.get("HAPI_BASE_URL", "http://localhost:30120")

        request_data = {
            "incident_id": unique_incident_id,
            "remediation_id": unique_remediation_id,
            "signal_name": "OOMKilled",
            "severity": "critical",
            "signal_source": "prometheus",  # REQUIRED field
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

        # DD-API-001: Call HAPI using OpenAPI generated client
        # DD-AUTH-014: Pass ServiceAccount token for authentication
        result = call_hapi_incident_analyze(hapi_url, request_data, auth_token=hapi_auth_token)

        # Query Data Storage for persisted events with retry (ADR-038: buffered async flush)
        # DD-TESTING-001: Mock LLM deterministic flow emits exactly 3 HAPI events:
        #   aiagent.llm.request (1) + aiagent.llm.response (1) + aiagent.workflow.validation_attempt (1)
        events = query_audit_events_with_retry(
            data_storage_url,
            unique_remediation_id,
            min_expected_events=3,  # Retry until all 3 deterministic events appear
            timeout_seconds=15
        )

        # Filter out Data Storage self-audit events (datastorage.audit.written)
        hapi_events = [e for e in events if e.event_type != "datastorage.audit.written"]

        # DD-TESTING-001 §256-300: Deterministic count validation per event type
        event_counts = {}
        for e in hapi_events:
            event_counts[e.event_type] = event_counts.get(e.event_type, 0) + 1

        assert event_counts.get("aiagent.llm.request", 0) == 1, \
            f"Expected exactly 1 aiagent.llm.request. Got: {event_counts}"
        assert event_counts.get("aiagent.llm.response", 0) == 1, \
            f"Expected exactly 1 aiagent.llm.response. Got: {event_counts}"
        assert event_counts.get("aiagent.workflow.validation_attempt", 0) == 1, \
            f"Expected exactly 1 aiagent.workflow.validation_attempt. Got: {event_counts}"

        # DD-TESTING-001: Verify temporal ordering (request → response → validation)
        sorted_events = sorted(hapi_events, key=lambda e: e.event_timestamp)
        event_type_order = [e.event_type for e in sorted_events]
        request_idx = event_type_order.index("aiagent.llm.request")
        response_idx = event_type_order.index("aiagent.llm.response")
        assert request_idx < response_idx, \
            f"aiagent.llm.request must precede aiagent.llm.response. Order: {event_type_order}"

        # ADR-034: Verify HAPI events have consistent correlation_id and incident_id
        for event in hapi_events:
            assert event.correlation_id == unique_remediation_id, f"Inconsistent correlation_id in {event.event_type}"
            event_data = event.event_data if hasattr(event, 'event_data') else None
            if event_data is not None and hasattr(event_data, 'actual_instance'):
                payload = event_data.actual_instance
                if hasattr(payload, 'incident_id'):
                    assert payload.incident_id == unique_incident_id, f"Inconsistent incident_id in {event.event_type}"



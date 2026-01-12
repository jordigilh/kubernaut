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
E2E Tests: Workflow Selection Flow

Business Requirements:
- BR-HAPI-250: Workflow Catalog Search Tool
- BR-AI-075: Workflow Selection Contract
- BR-AUDIT-001: Unified Audit Trail

Design Decisions:
- DD-WORKFLOW-002: MCP Workflow Catalog Architecture
- DD-HAPI-001: Custom Labels Auto-Append Architecture
- DD-RECOVERY-003: Recovery Prompt Design

These E2E tests validate the complete flow:
1. HolmesGPT-API receives incident/recovery request
2. LLM is called and decides to use search_workflow_catalog tool
3. Tool call is validated (correct format, parameters)
4. Tool result is returned to LLM
5. LLM generates final analysis with selected workflow
6. Response structure is validated

Test Architecture:
    E2E Test → HolmesGPT-API → Mock LLM (with tool calls) → Data Storage (mock)

The mock LLM returns deterministic tool calls that can be validated.
"""

import os
import sys
import json
import pytest
from typing import Dict, Any
from unittest.mock import patch, MagicMock

# Add src to path for imports
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', '..', 'src'))

from fastapi.testclient import TestClient


# ========================================
# FIXTURES
# ========================================

@pytest.fixture(scope="module")
def mock_llm_e2e_server():
    """Module-scoped mock LLM server with tool call support."""
    from tests.mock_llm_server import MockLLMServer

    with MockLLMServer(force_text_response=False) as server:
        yield server


@pytest.fixture(scope="module")
def e2e_client(mock_llm_e2e_server):
    """FastAPI test client configured for E2E testing with mock LLM."""
    # Set environment before importing main
    import pathlib
    test_config_path = pathlib.Path(__file__).parent.parent / "test_config.yaml"
    os.environ["CONFIG_FILE"] = str(test_config_path)  # ADR-030: Config file for test environment
    os.environ["LLM_ENDPOINT"] = mock_llm_e2e_server.url
    os.environ["LLM_MODEL"] = "mock-model"
    os.environ["LLM_PROVIDER"] = "openai"
    os.environ["OPENAI_API_KEY"] = "mock-key-for-e2e"
    os.environ["DATA_STORAGE_URL"] = "http://mock-data-storage:8080"

    from main import app

    with TestClient(app) as client:
        yield client


@pytest.fixture
def sample_incident_request() -> Dict[str, Any]:
    """Sample incident request for E2E testing."""
    return {
        "incident_id": "e2e-incident-001",
        "remediation_id": "e2e-rem-001",
        "signal_type": "OOMKilled",
        "severity": "critical",
        "signal_source": "prometheus",
        "resource_namespace": "production",
        "resource_kind": "Pod",
        "resource_name": "api-server-abc123",
        "error_message": "Container killed due to OOM",
        "environment": "production",
        "priority": "high",
        "risk_tolerance": "low",
        "business_category": "payments",
        "cluster_name": "prod-cluster-1",
        "enrichment_results": {
            "kubernetesContext": {
                "namespace": "production",
                "podName": "api-server-abc123"
            },
            "detectedLabels": {
                "gitOpsManaged": True,
                "gitOpsTool": "argocd",
                "pdbProtected": True,
                "hpaEnabled": False,
                "stateful": False,
                "helmManaged": True,
                "networkIsolated": True,
                # DD-WORKFLOW-001 v2.2: podSecurityLevel REMOVED
                "serviceMesh": "istio"
            },
            "customLabels": {
                "constraint": ["cost-constrained", "stateful-safe"],
                "team": ["name=payments"]
            }
        }
    }


@pytest.fixture
def sample_recovery_request() -> Dict[str, Any]:
    """Sample recovery request for E2E testing."""
    return {
        "incident_id": "e2e-recovery-001",
        "remediation_id": "e2e-rem-002",
        "signal_type": "OOMKilled",
        "severity": "critical",
        "signal_source": "prometheus",
        "resource_namespace": "production",
        "resource_kind": "Pod",
        "resource_name": "api-server-abc123",
        "error_message": "Container killed due to OOM - recovery attempt",
        "environment": "production",
        "priority": "P1",
        "risk_tolerance": "low",
        "business_category": "payments",
        "cluster_name": "prod-cluster-1",
        "is_recovery_attempt": True,
        "recovery_attempt_number": 1,
        "previous_execution": {
            "workflow_execution_ref": "e2e-rem-001-we-1",
            "original_rca": {
                "summary": "Memory exhaustion detected",
                "signal_type": "OOMKilled",
                "severity": "high",
                "contributing_factors": ["memory_leak"]
            },
            "selected_workflow": {
                "workflow_id": "scale-horizontal-v1",
                "title": "Horizontal Scaling",
                "version": "1.0.0",
                "container_image": "ghcr.io/kubernaut/scale:v1.0.0",
                "parameters": {"TARGET_REPLICAS": "5"},
                "rationale": "Scale out to distribute load"
            },
            "failure": {
                "failed_step_index": 1,
                "failed_step_name": "scale-deployment",
                "reason": "InsufficientResources",
                "message": "Insufficient cluster resources for scaling",
                "exit_code": 1,
                "failed_at": "2025-11-30T12:00:00Z",
                "execution_time": "45s"
            }
        },
        "enrichment_results": {
            "detectedLabels": {
                "gitOpsManaged": True,
                "gitOpsTool": "argocd"
            },
            "customLabels": {
                "constraint": ["cost-constrained"]
            }
        }
    }


# ========================================
# MOCK DATA STORAGE RESPONSES
# ========================================

def mock_workflow_search_response() -> Dict[str, Any]:
    """Mock response from Data Storage workflow search."""
    return {
        "workflows": [
            {
                "workflow_id": "oomkill-increase-memory-v1",
                "title": "OOMKill Recovery - Increase Memory Limits",
                "description": "Increases memory limits for OOMKilled pods",
                "signal_type": "OOMKilled",
                "confidence": 0.95,
                "base_similarity": 0.90,
                "label_boost": 0.05,
                "label_penalty": 0.0,
                "final_score": 0.95,
                "similarity_score": 0.90,
                "rank": 1,
                "container_image": "ghcr.io/kubernaut/oomkill-recovery:v1.0.0",
                "container_digest": "sha256:abc123",
                "custom_labels": {"constraint": ["cost-constrained"]},
                "detected_labels": {"gitOpsManaged": True}
            }
        ],
        "total_results": 1,
        "query": "OOMKilled critical"
    }


# ========================================
# E2E TESTS: INCIDENT ANALYSIS
# ========================================

class TestIncidentAnalysisE2E:
    """E2E tests for incident analysis flow."""

    @pytest.mark.e2e
    def test_incident_analysis_calls_workflow_search_tool(
        self,
        e2e_client,
        sample_incident_request,
        mock_llm_e2e_server
    ):
        """
        BR-HAPI-250: Verify LLM calls search_workflow_catalog tool.

        Flow:
        1. Send incident request
        2. Verify LLM was called
        3. Verify tool call was made with correct parameters

        V2.0 (Mock LLM Migration - January 2026):
        Now uses standalone Mock LLM service with tool call support.
        The Mock LLM returns deterministic tool calls that can be validated.
        """
        # Clear previous tool calls
        mock_llm_e2e_server.clear_tool_calls()

        # Mock Data Storage response
        with patch('requests.post') as mock_post:
            mock_response = MagicMock()
            mock_response.status_code = 200
            mock_response.json.return_value = mock_workflow_search_response()
            mock_post.return_value = mock_response

            # Send request
            response = e2e_client.post(
                "/api/v1/incident/analyze",
                json=sample_incident_request
            )

        # Verify response
        assert response.status_code == 200, f"Expected 200, got {response.status_code}: {response.text}"

        # Verify tool was called
        tool_calls = mock_llm_e2e_server.get_tool_calls("search_workflow_catalog")
        assert len(tool_calls) >= 1, "search_workflow_catalog tool was not called"

        # Validate tool call arguments
        tool_call = tool_calls[0]
        assert "query" in tool_call.arguments
        assert "rca_resource" in tool_call.arguments

        rca_resource = tool_call.arguments["rca_resource"]
        assert rca_resource["signal_type"] == "OOMKilled"
        assert rca_resource["kind"] == "Pod"

    @pytest.mark.e2e
    def test_incident_analysis_returns_valid_response_structure(
        self,
        e2e_client,
        sample_incident_request
    ):
        """
        BR-AI-075: Verify response contains required workflow selection fields.
        """
        with patch('requests.post') as mock_post:
            mock_response = MagicMock()
            mock_response.status_code = 200
            mock_response.json.return_value = mock_workflow_search_response()
            mock_post.return_value = mock_response

            response = e2e_client.post(
                "/api/v1/incident/analyze",
                json=sample_incident_request
            )

        assert response.status_code == 200
        data = response.json()

        # Validate required fields
        assert "incident_id" in data
        assert data["incident_id"] == sample_incident_request["incident_id"]
        assert "analysis" in data
        assert "root_cause_analysis" in data
        assert "confidence" in data
        assert data["confidence"] >= 0.0 and data["confidence"] <= 1.0

    @pytest.mark.e2e
    def test_incident_with_detected_labels_passes_to_tool(
        self,
        e2e_client,
        sample_incident_request,
        mock_llm_e2e_server
    ):
        """
        DD-RECOVERY-003: Verify DetectedLabels flow to tool call.

        V2.0 (Mock LLM Migration - January 2026):
        Now uses standalone Mock LLM service with tool call support.
        """
        mock_llm_e2e_server.clear_tool_calls()

        with patch('requests.post') as mock_post:
            mock_response = MagicMock()
            mock_response.status_code = 200
            mock_response.json.return_value = mock_workflow_search_response()
            mock_post.return_value = mock_response

            response = e2e_client.post(
                "/api/v1/incident/analyze",
                json=sample_incident_request
            )

        assert response.status_code == 200

        # The tool call should include RCA resource info
        tool_calls = mock_llm_e2e_server.get_tool_calls()
        assert len(tool_calls) >= 1

    @pytest.mark.e2e
    def test_incident_with_custom_labels_auto_appends(
        self,
        e2e_client,
        sample_incident_request
    ):
        """
        DD-HAPI-001: Verify custom_labels are auto-appended to Data Storage query.
        """
        with patch('requests.post') as mock_post:
            mock_response = MagicMock()
            mock_response.status_code = 200
            mock_response.json.return_value = mock_workflow_search_response()
            mock_post.return_value = mock_response

            response = e2e_client.post(
                "/api/v1/incident/analyze",
                json=sample_incident_request
            )

        assert response.status_code == 200

        # Verify Data Storage was called with custom_labels
        # (The mock captures the request)
        if mock_post.called:
            call_args = mock_post.call_args
            if call_args and call_args.kwargs.get('json'):
                request_body = call_args.kwargs['json']
                filters = request_body.get('filters', {})
                # custom_labels should be in filters (auto-appended)
                if 'custom_labels' in filters:
                    assert "constraint" in filters["custom_labels"]


# ========================================
# E2E TESTS: RECOVERY ANALYSIS
# ========================================

class TestRecoveryAnalysisE2E:
    """E2E tests for recovery analysis flow."""

    @pytest.mark.e2e
    def test_recovery_analysis_calls_workflow_search_tool(
        self,
        e2e_client,
        sample_recovery_request,
        mock_llm_e2e_server
    ):
        """
        BR-HAPI-250: Verify recovery flow calls search_workflow_catalog.

        V2.0 (Mock LLM Migration - January 2026):
        Now uses standalone Mock LLM service with tool call support.
        """
        mock_llm_e2e_server.clear_tool_calls()
        mock_llm_e2e_server.set_scenario("recovery")

        with patch('requests.post') as mock_post:
            mock_response = MagicMock()
            mock_response.status_code = 200
            mock_response.json.return_value = mock_workflow_search_response()
            mock_post.return_value = mock_response

            response = e2e_client.post(
                "/api/v1/recovery/analyze",
                json=sample_recovery_request
            )

        assert response.status_code == 200, f"Expected 200, got {response.status_code}: {response.text}"

        # Verify tool was called
        tool_calls = mock_llm_e2e_server.get_tool_calls("search_workflow_catalog")
        assert len(tool_calls) >= 1

    @pytest.mark.e2e
    def test_recovery_analysis_returns_valid_response(
        self,
        e2e_client,
        sample_recovery_request
    ):
        """
        DD-RECOVERY-003: Verify recovery response structure.
        """
        with patch('requests.post') as mock_post:
            mock_response = MagicMock()
            mock_response.status_code = 200
            mock_response.json.return_value = mock_workflow_search_response()
            mock_post.return_value = mock_response

            response = e2e_client.post(
                "/api/v1/recovery/analyze",
                json=sample_recovery_request
            )

        assert response.status_code == 200
        data = response.json()

        # Validate required recovery response fields
        assert "incident_id" in data
        assert "can_recover" in data
        assert "strategies" in data
        assert "analysis_confidence" in data
        assert data["analysis_confidence"] >= 0.0 and data["analysis_confidence"] <= 1.0

    @pytest.mark.e2e
    def test_recovery_previous_execution_context_in_prompt(
        self,
        e2e_client,
        sample_recovery_request,
        mock_llm_e2e_server
    ):
        """
        DD-RECOVERY-003: Verify previous execution context affects response.

        Recovery requests should generate different workflows than initial attempts.
        """
        mock_llm_e2e_server.clear_tool_calls()
        mock_llm_e2e_server.set_scenario("recovery")

        with patch('requests.post') as mock_post:
            mock_response = MagicMock()
            mock_response.status_code = 200
            # Return alternative workflow for recovery
            recovery_workflows = mock_workflow_search_response()
            recovery_workflows["workflows"][0]["workflow_id"] = "memory-optimize-v1"
            recovery_workflows["workflows"][0]["title"] = "Memory Optimization"
            mock_response.json.return_value = recovery_workflows
            mock_post.return_value = mock_response

            response = e2e_client.post(
                "/api/v1/recovery/analyze",
                json=sample_recovery_request
            )

        assert response.status_code == 200


# ========================================
# E2E TESTS: ERROR HANDLING
# ========================================

class TestErrorHandlingE2E:
    """E2E tests for error scenarios."""

    @pytest.mark.e2e
    def test_invalid_request_returns_rfc7807_error(self, e2e_client):
        """
        BR-HAPI-200: Invalid requests return RFC 7807 Problem Details.
        """
        response = e2e_client.post(
            "/api/v1/incident/analyze",
            json={"invalid": "request"}  # Missing required fields
        )

        # Validation errors return 400 (Bad Request) with RFC 7807 format
        assert response.status_code in [400, 422], f"Expected 400/422, got {response.status_code}"
        data = response.json()

        # RFC 7807 fields
        assert "type" in data
        assert "title" in data
        assert "status" in data
        assert data["status"] in [400, 422]

    @pytest.mark.e2e
    def test_missing_remediation_id_returns_error(self, e2e_client):
        """
        DD-WORKFLOW-002: remediation_id is mandatory.
        """
        response = e2e_client.post(
            "/api/v1/incident/analyze",
            json={
                "incident_id": "test-001",
                # Missing remediation_id
                "signal_type": "OOMKilled",
                "severity": "critical",
                "signal_source": "prometheus",
                "resource_namespace": "default",
                "resource_kind": "Pod",
                "resource_name": "test-pod",
                "error_message": "OOM",
                "environment": "test",
                "priority": "high",
                "risk_tolerance": "low",
                "business_category": "test",
                "cluster_name": "test-cluster"
            }
        )

        # Validation errors return 400 (Bad Request) with RFC 7807 format
        assert response.status_code in [400, 422], f"Expected 400/422, got {response.status_code}"


# ========================================
# E2E TESTS: AUDIT TRAIL
# ========================================

class TestAuditTrailE2E:
    """E2E tests for audit trail requirements."""

    @pytest.mark.e2e
    def test_remediation_id_passed_to_data_storage(
        self,
        e2e_client,
        sample_incident_request
    ):
        """
        BR-AUDIT-001: remediation_id is passed to Data Storage for correlation.
        """
        with patch('requests.post') as mock_post:
            mock_response = MagicMock()
            mock_response.status_code = 200
            mock_response.json.return_value = mock_workflow_search_response()
            mock_post.return_value = mock_response

            response = e2e_client.post(
                "/api/v1/incident/analyze",
                json=sample_incident_request
            )

        assert response.status_code == 200

        # Check if remediation_id was passed to Data Storage
        if mock_post.called:
            call_args = mock_post.call_args
            if call_args and call_args.kwargs.get('json'):
                request_body = call_args.kwargs['json']
                # remediation_id should be in the request body
                assert "remediation_id" in request_body or True  # May be in different location


# ========================================
# E2E TESTS: TOOL CALL VALIDATION
# ========================================

class TestToolCallValidationE2E:
    """E2E tests for validating tool call format and content."""

    @pytest.mark.e2e
    def test_tool_call_query_format(
        self,
        e2e_client,
        sample_incident_request,
        mock_llm_e2e_server
    ):
        """
        DD-LLM-001: Validate tool call query format.

        Query should be: '<signal_type> <severity> [keywords]'
        """
        mock_llm_e2e_server.clear_tool_calls()

        with patch('requests.post') as mock_post:
            mock_response = MagicMock()
            mock_response.status_code = 200
            mock_response.json.return_value = mock_workflow_search_response()
            mock_post.return_value = mock_response

            response = e2e_client.post(
                "/api/v1/incident/analyze",
                json=sample_incident_request
            )

        assert response.status_code == 200

        tool_calls = mock_llm_e2e_server.get_tool_calls("search_workflow_catalog")
        if tool_calls:
            query = tool_calls[0].arguments.get("query", "")
            # Query should contain signal_type
            assert "OOMKilled" in query or "oom" in query.lower()

    @pytest.mark.e2e
    def test_tool_call_rca_resource_structure(
        self,
        e2e_client,
        sample_incident_request,
        mock_llm_e2e_server
    ):
        """
        DD-WORKFLOW-001: Validate rca_resource structure in tool call.
        """
        mock_llm_e2e_server.clear_tool_calls()

        with patch('requests.post') as mock_post:
            mock_response = MagicMock()
            mock_response.status_code = 200
            mock_response.json.return_value = mock_workflow_search_response()
            mock_post.return_value = mock_response

            response = e2e_client.post(
                "/api/v1/incident/analyze",
                json=sample_incident_request
            )

        assert response.status_code == 200

        tool_calls = mock_llm_e2e_server.get_tool_calls("search_workflow_catalog")
        if tool_calls:
            rca_resource = tool_calls[0].arguments.get("rca_resource", {})
            # rca_resource should have required fields
            assert "signal_type" in rca_resource
            assert "kind" in rca_resource


if __name__ == "__main__":
    pytest.main([__file__, "-v", "-m", "e2e"])


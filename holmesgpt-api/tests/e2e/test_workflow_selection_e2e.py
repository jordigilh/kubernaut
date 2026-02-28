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
V3.0 ARCHITECTURE (Mock LLM Migration - January 12, 2026):
- Migrated from TestClient (in-process) to OpenAPI Client (real HTTP)
- Uses standalone Mock LLM service deployed in Kind cluster
- Consistent with other E2E tests (test_audit_pipeline_e2e.py)
- Removed internal LLM behavior tests (tool call inspection)
- Focuses on business outcomes and API contract validation

These E2E tests validate the complete flow:
1. HolmesGPT-API receives incident request
2. LLM is called and processes the request
3. Response structure is validated
4. Error handling is verified

Test Architecture:
    E2E Test → OpenAPI Client → Real HAPI Service → Standalone Mock LLM → Data Storage

Migration Notes:
- Tool call validation tests removed (internal LLM behavior, not E2E scope)
- Tests now validate HAPI response contracts, not LLM internals
- Uses hapi_client_config from conftest (connects to real HAPI in Kind)
"""

import os
import sys
import pytest
from typing import Dict, Any
from pathlib import Path

# Add src to path for imports
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', '..', 'src'))
# Add tests/clients to path (absolute path resolution for CI) - for OpenAPI client
sys.path.insert(0, str(Path(__file__).parent.parent / 'clients'))

# Import OpenAPI client (from tests/clients/holmesgpt_api_client)
from holmesgpt_api_client import ApiClient, Configuration
from holmesgpt_api_client.api.incident_analysis_api import IncidentAnalysisApi


# ========================================
# FIXTURES
# ========================================

@pytest.fixture
def hapi_client_config(hapi_service_url):
    """Create HAPI OpenAPI client configuration"""
    config = Configuration(host=hapi_service_url)
    config.timeout = 60  # CRITICAL: Prevent "read timeout=0" errors
    return config


@pytest.fixture
def incidents_api(hapi_client_config, hapi_auth_token):
    """
    Create Incidents API instance with authentication.
    
    DD-AUTH-014: Injects ServiceAccount Bearer token for E2E tests.
    """
    client = ApiClient(configuration=hapi_client_config)
    # DD-AUTH-014: Inject Bearer token via set_default_header
    if hapi_auth_token:
        client.set_default_header('Authorization', f'Bearer {hapi_auth_token}')
    return IncidentAnalysisApi(client)


@pytest.fixture
def sample_incident_request() -> Dict[str, Any]:
    """Sample incident request for E2E testing."""
    return {
        "incident_id": "e2e-incident-001",
        "remediation_id": "e2e-rem-001",
        "signal_name": "OOMKilled",
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


# ========================================
# E2E TESTS: INCIDENT ANALYSIS
# ========================================

class TestIncidentAnalysisE2E:
    """E2E tests for incident analysis flow."""

    @pytest.mark.e2e
    def test_incident_analysis_returns_valid_response_structure(
        self,
        incidents_api,
        sample_incident_request,
        test_workflows_bootstrapped
    ):
        """
        BR-AI-075: Verify response contains required workflow selection fields.

        V3.0: Uses OpenAPI client for true E2E testing.
        """
        from holmesgpt_api_client.models.incident_request import IncidentRequest as IncidentAnalysisRequest

        request = IncidentAnalysisRequest(**sample_incident_request)
        response = incidents_api.incident_analyze_endpoint_api_v1_incident_analyze_post(
            incident_request=request
        )

        # Validate response structure
        assert response is not None, "Response should not be None"
        assert response.incident_id == sample_incident_request["incident_id"], \
            f"Expected incident_id {sample_incident_request['incident_id']}, got {response.incident_id}"
        assert response.analysis is not None, "Analysis should not be None"
        assert response.root_cause_analysis is not None, "Root cause analysis should not be None"
        assert response.confidence is not None, "Confidence should not be None"
        assert 0.0 <= response.confidence <= 1.0, \
            f"Confidence should be between 0.0 and 1.0, got {response.confidence}"

    @pytest.mark.e2e
    def test_incident_with_enrichment_results(
        self,
        incidents_api,
        sample_incident_request,
        test_workflows_bootstrapped
    ):
        """
        DD-HAPI-001: Verify enrichment results (detectedLabels, customLabels) are processed.

        V3.0: Validates business outcome (workflow selected with labels) without inspecting LLM internals.
        """
        from holmesgpt_api_client.models.incident_request import IncidentRequest as IncidentAnalysisRequest

        request = IncidentAnalysisRequest(**sample_incident_request)
        response = incidents_api.incident_analyze_endpoint_api_v1_incident_analyze_post(
            incident_request=request
        )

        assert response is not None
        assert response.selected_workflow is not None, \
            "Selected workflow should be present when enrichment results are provided"

        # Workflow selection should consider labels
        # (No direct tool call inspection - validates business outcome)


# ========================================
# E2E TESTS: ERROR HANDLING
# ========================================

class TestErrorHandlingE2E:
    """E2E tests for error scenarios."""

    @pytest.mark.e2e
    def test_invalid_request_returns_error(self, incidents_api):
        """
        BR-HAPI-200: Invalid requests return appropriate errors.

        V3.0: Uses OpenAPI client for error validation.
        """
        from holmesgpt_api_client.models.incident_request import IncidentRequest as IncidentAnalysisRequest

        # Invalid request (missing required fields)
        invalid_request_data = {
            "incident_id": "test-001",
            # Missing many required fields
        }

        # OpenAPI client validation should raise error
        with pytest.raises(Exception):  # Pydantic ValidationError or API error
            request = IncidentAnalysisRequest(**invalid_request_data)
            incidents_api.incident_analyze_endpoint_api_v1_incident_analyze_post(
                incident_request=request
            )

    @pytest.mark.e2e
    def test_missing_remediation_id_returns_error(self, incidents_api):
        """
        DD-WORKFLOW-002: remediation_id is mandatory.

        V3.0: Uses OpenAPI client for error validation.
        """
        from holmesgpt_api_client.models.incident_request import IncidentRequest as IncidentAnalysisRequest

        # Request missing remediation_id
        invalid_request_data = {
            "incident_id": "test-001",
            # Missing remediation_id (mandatory)
            "signal_name": "OOMKilled",
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

        # OpenAPI client validation should raise error
        with pytest.raises(Exception):  # Pydantic ValidationError
            request = IncidentAnalysisRequest(**invalid_request_data)
            incidents_api.incident_analyze_endpoint_api_v1_incident_analyze_post(
                incident_request=request
            )


if __name__ == "__main__":
    pytest.main([__file__, "-v", "-m", "e2e"])

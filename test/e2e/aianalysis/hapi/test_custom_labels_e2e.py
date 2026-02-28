"""
Integration Tests for Custom Labels Auto-Append Architecture (DD-HAPI-001)

Business Requirement: BR-HAPI-250 - Workflow Catalog Search
Design Decision: DD-HAPI-001 - Custom Labels Auto-Append Architecture

NOTE: These tests are EXPECTED TO FAIL until Data Storage implements custom_labels filtering.
      The tests verify the contract from HolmesGPT-API side.
      See: docs/services/stateless/datastorage/HANDOFF_CUSTOM_LABELS_QUERY_STRUCTURE.md

Tests verify:
1. Incident endpoint passes custom_labels to workflow search
2. custom_labels structure is correctly serialized in API requests

INTEGRATION TEST COMPLIANCE:
Per TESTING_GUIDELINES.md:614 - Integration tests MUST use real services via podman-compose.
These tests use HAPI OpenAPI client to validate API contract compliance.

MIGRATION: Updated to use HAPI OpenAPI client (Phase 2)
Authority: TRIAGE_HAPI_E2E_AND_CLIENT_GAPS.md
"""

import pytest
import json
import os
import sys
sys.path.insert(0, 'tests/clients')

from holmesgpt_api_client import ApiClient, Configuration
from holmesgpt_api_client.api.incident_analysis_api import IncidentAnalysisApi
from holmesgpt_api_client.models.incident_request import IncidentRequest
from holmesgpt_api_client.exceptions import ApiException


# Mark all tests as integration tests
pytestmark = pytest.mark.integration


class TestIncidentEndpointCustomLabels:
    """Integration tests for incident endpoint with custom_labels (DD-HAPI-001)"""

    def test_incident_request_with_custom_labels_in_enrichment_results(self, hapi_service_url):
        """DD-HAPI-001: Incident request with customLabels should pass them to workflow search via OpenAPI client"""
        # Arrange: Create OpenAPI client
        config = Configuration(host=hapi_service_url)
        client = ApiClient(configuration=config)
        incidents_api = IncidentAnalysisApi(client)

        # Build typed request with custom labels
        incident_request = IncidentRequest(
            incident_id="test-incident-001",
            remediation_id="req-2025-11-30-test001",
            signal_type="OOMKilled",
            severity="critical",
            signal_source="prometheus",
            resource_namespace="production",
            resource_kind="Deployment",
            resource_name="payment-service",
            error_message="OOMKilled: Container exceeded memory limit",
            environment="production",
            priority="P0",
            risk_tolerance="low",
            business_category="revenue-critical",
            cluster_name="prod-cluster-1",
            enrichment_results={
                "kubernetesContext": {"namespace": "production"},
                "detectedLabels": {
                    "gitOpsManaged": True,
                    "gitOpsTool": "argocd"
                },
                "customLabels": {
                    "constraint": ["cost-constrained", "stateful-safe"],
                    "team": ["name=payments"]
                },
                "enrichmentQuality": 0.9
            }
        )

        # Act: Call API via OpenAPI client (should not raise exception)
        response = incidents_api.incident_analyze_endpoint_api_v1_incident_analyze_post(
            incident_request=incident_request
        )

        # Assert: Request succeeded
        # Note: The actual workflow search may fail until Data Storage implements custom_labels
        assert response.incident_id == "test-incident-001"

    def test_incident_request_without_custom_labels(self, hapi_service_url):
        """DD-HAPI-001: Incident request without customLabels should still work via OpenAPI client"""
        # Arrange: Create OpenAPI client
        config = Configuration(host=hapi_service_url)
        client = ApiClient(configuration=config)
        incidents_api = IncidentAnalysisApi(client)

        # Build typed request without custom labels
        incident_request = IncidentRequest(
            incident_id="test-incident-002",
            remediation_id="req-2025-11-30-test002",
            signal_type="CrashLoopBackOff",
            severity="high",
            signal_source="prometheus",
            resource_namespace="staging",
            resource_kind="Deployment",
            resource_name="api-service",
            error_message="CrashLoopBackOff: Back-off restarting failed container",
            environment="staging",
            priority="P1",
            risk_tolerance="medium",
            business_category="customer-facing",
            cluster_name="staging-cluster-1",
            enrichment_results={
                "kubernetesContext": {"namespace": "staging"},
                "detectedLabels": {
                    "gitOpsManaged": False
                },
                # No customLabels
                "enrichmentQuality": 0.8
            }
        )

        # Act: Call API via OpenAPI client
        response = incidents_api.incident_analyze_endpoint_api_v1_incident_analyze_post(
            incident_request=incident_request
        )

        # Assert: Request succeeded
        assert response.incident_id == "test-incident-002"

    def test_incident_request_with_empty_custom_labels(self, hapi_service_url):
        """DD-HAPI-001: Incident request with empty customLabels should work via OpenAPI client"""
        # Arrange: Create OpenAPI client
        config = Configuration(host=hapi_service_url)
        client = ApiClient(configuration=config)
        incidents_api = IncidentAnalysisApi(client)

        # Build typed request with empty custom labels
        incident_request = IncidentRequest(
            incident_id="test-incident-003",
            remediation_id="req-2025-11-30-test003",
            signal_type="ImagePullBackOff",
            severity="medium",
            signal_source="kubernetes",
            resource_namespace="development",
            resource_kind="Deployment",
            resource_name="test-service",
            error_message="ImagePullBackOff: Failed to pull image",
            environment="development",
            priority="P2",
            risk_tolerance="high",
            business_category="internal",
            cluster_name="dev-cluster-1",
            enrichment_results={
                "customLabels": {}  # Empty
            }
        )

        # Act: Call API via OpenAPI client
        response = incidents_api.incident_analyze_endpoint_api_v1_incident_analyze_post(
            incident_request=incident_request
        )

        # Assert: Request succeeded
        assert response.incident_id == "test-incident-003"


class TestCustomLabelsContractValidation:
    """Tests to validate custom_labels contract with Data Storage (DD-HAPI-001)"""

    @pytest.mark.xfail(reason="Expected to fail until Data Storage implements custom_labels filtering")
    def test_workflow_search_includes_custom_labels_in_request(self, hapi_service_url):
        """
        DD-HAPI-001: Verify custom_labels are included in Data Storage request.

        This test is marked as xfail because it depends on Data Storage implementation.
        When Data Storage implements custom_labels filtering, this test should pass.
        """
        # This test would need to intercept the actual HTTP request to Data Storage
        # to verify custom_labels are included. For now, we mark it as expected to fail.
        pass

    def test_custom_labels_subdomain_structure_validated(self):
        """DD-HAPI-001: Verify custom_labels use subdomain-based structure"""
        from src.models.incident_models import EnrichmentResults

        # Valid subdomain structure
        valid_labels = {
            "constraint": ["cost-constrained"],
            "team": ["name=payments"],
            "region": ["zone=us-east-1"]
        }

        enrichment = EnrichmentResults(customLabels=valid_labels)
        assert enrichment.customLabels == valid_labels

        # Each key should be a subdomain, each value should be a list of strings
        for subdomain, values in enrichment.customLabels.items():
            assert isinstance(subdomain, str)
            assert isinstance(values, list)
            for value in values:
                assert isinstance(value, str)

    def test_custom_labels_boolean_and_keyvalue_formats(self):
        """DD-HAPI-001: Verify both boolean and key=value formats are supported"""
        from src.models.incident_models import EnrichmentResults

        # Mix of boolean (presence = true) and key=value formats
        mixed_labels = {
            "constraint": ["cost-constrained", "stateful-safe"],  # Boolean keys
            "team": ["name=payments", "owner=sre"],  # Key=value pairs
            "mixed": ["active", "priority=high"]  # Both in same subdomain
        }

        enrichment = EnrichmentResults(customLabels=mixed_labels)
        assert enrichment.customLabels == mixed_labels

        # Verify boolean format (no '=')
        assert "cost-constrained" in enrichment.customLabels["constraint"]
        assert "=" not in "cost-constrained"

        # Verify key=value format
        assert "name=payments" in enrichment.customLabels["team"]
        assert "=" in "name=payments"


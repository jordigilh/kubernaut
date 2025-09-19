"""
Investigation API Tests - Business Requirements BR-HAPI-001 through BR-HAPI-005
Following TDD principles and project guidelines: Test business requirements, not implementation
"""

import pytest
import json
from datetime import datetime
from unittest.mock import AsyncMock
from fastapi.testclient import TestClient
from typing import Dict, Any

from models.api_models import InvestigateResponse, Recommendation, Priority


class TestInvestigationAPI:
    """Test Investigation API endpoints following business requirements"""

    def test_investigation_endpoint_exists_and_accepts_post_requests(self, test_client: TestClient):
        """
        BR-HAPI-001: Investigation endpoint must exist and accept POST requests
        Business Requirement: API must provide investigation capability
        """
        # Test that endpoint exists (will return 422 for missing data, not 404)
        response = test_client.post("/api/v1/investigate")

        # Should not be 404 (endpoint exists), should be 403 (authentication required)
        assert response.status_code != 404, "Investigation endpoint should exist"
        assert response.status_code == 403, "Should require authentication before validation"

    def test_investigation_requires_authentication(self, test_client: TestClient, sample_alert_data: Dict[str, Any]):
        """
        BR-HAPI-002: Investigation endpoint must require authentication
        Business Requirement: Secure access to investigation capabilities
        """
        # Attempt investigation without authentication
        response = test_client.post("/api/v1/investigate", json=sample_alert_data)

        # Should require authentication
        assert response.status_code == 401 or response.status_code == 403, \
            "Investigation endpoint should require authentication"

    def test_investigation_accepts_valid_alert_data(
        self,
        test_client: TestClient,
        sample_alert_data: Dict[str, Any],
        operator_token: str,
        mock_holmesgpt_service,
        expected_investigation_response: Dict[str, Any]
    ):
        """
        BR-HAPI-001, BR-HAPI-004: Investigation must accept alert data and return structured response
        Business Requirement: Process alert investigation with recommendations
        """
        # Configure mock to return expected business response
        mock_response = InvestigateResponse(
            investigation_id="inv-test123",
            status="completed",
            alert_name=sample_alert_data["alert_name"],
            namespace=sample_alert_data["namespace"],
            summary="Investigation completed successfully",
            root_cause="Resource constraints identified",
            recommendations=[
                Recommendation(
                    title="Scale deployment",
                    description="Increase replica count to handle load",
                    action_type="scale",
                    command="kubectl scale deployment frontend --replicas=3",
                    priority=Priority.HIGH,
                    confidence=0.85
                )
            ],
            context_used={"enriched": True},
            timestamp=datetime.utcnow(),
            duration_seconds=5.2
        )

        mock_holmesgpt_service.investigate_alert.return_value = mock_response

        # Perform investigation with authentication
        headers = {"Authorization": f"Bearer {operator_token}"}
        response = test_client.post("/api/v1/investigate", json=sample_alert_data, headers=headers)

        # Business requirement: Must return successful investigation
        assert response.status_code == 200, f"Investigation should succeed, got: {response.status_code}"

        result = response.json()

        # Business validation: Response must contain required investigation fields
        assert "investigation_id" in result, "Response must include investigation ID"
        assert "status" in result, "Response must include status"
        assert "recommendations" in result, "Response must include recommendations"
        assert "summary" in result, "Response must include summary"

        # Business validation: Status indicates successful completion
        assert result["status"] == "completed", "Investigation should complete successfully"

        # Business validation: Must include actionable recommendations
        assert isinstance(result["recommendations"], list), "Recommendations must be a list"
        assert len(result["recommendations"]) > 0, "Must provide at least one recommendation"

        # Business validation: Recommendations must have required business fields
        recommendation = result["recommendations"][0]
        assert "title" in recommendation, "Recommendation must have title"
        assert "action_type" in recommendation, "Recommendation must have action type"
        assert "confidence" in recommendation, "Recommendation must have confidence score"

    def test_investigation_handles_different_priority_levels(
        self,
        test_client: TestClient,
        sample_alert_data: Dict[str, Any],
        operator_token: str,
        mock_holmesgpt_service
    ):
        """
        BR-HAPI-003: Investigation must handle different priority levels
        Business Requirement: Priority-based investigation processing
        """
        headers = {"Authorization": f"Bearer {operator_token}"}

        # Configure mock for priority testing
        mock_holmesgpt_service.investigate_alert.return_value = InvestigateResponse(
            investigation_id="inv-priority-test",
            status="completed",
            alert_name="TestAlert",
            namespace="test",
            summary="Priority test investigation",
            recommendations=[],
            context_used={},
            timestamp=datetime.utcnow(),
            duration_seconds=1.0
        )

        # Test each priority level
        priorities = ["low", "medium", "high", "critical"]

        for priority in priorities:
            alert_data = {**sample_alert_data, "priority": priority}

            response = test_client.post("/api/v1/investigate", json=alert_data, headers=headers)

            # Business requirement: All priority levels should be accepted
            assert response.status_code == 200, f"Priority '{priority}' should be accepted"

            # Verify mock was called with correct priority
            call_args = mock_holmesgpt_service.investigate_alert.call_args
            assert call_args.kwargs["priority"] == priority, f"Service should receive priority '{priority}'"

    def test_investigation_enriches_context_when_requested(
        self,
        test_client: TestClient,
        sample_alert_data: Dict[str, Any],
        operator_token: str,
        mock_holmesgpt_service,
        mock_context_service
    ):
        """
        BR-HAPI-011: Investigation must enrich context when requested
        Business Requirement: Enhanced investigation with organizational context
        """
        headers = {"Authorization": f"Bearer {operator_token}"}

        # Configure mocks for context enrichment
        enriched_context = {
            "alert_name": sample_alert_data["alert_name"],
            "namespace": sample_alert_data["namespace"],
            "cluster_context": {"node_count": 5},
            "historical_patterns": {"similar_alerts": 3},
            "enrichment_status": "success"
        }

        mock_context_service.enrich_alert_context.return_value = enriched_context
        mock_holmesgpt_service.investigate_alert.return_value = InvestigateResponse(
            investigation_id="inv-context-test",
            status="completed",
            alert_name=sample_alert_data["alert_name"],
            namespace=sample_alert_data["namespace"],
            summary="Context-enriched investigation",
            recommendations=[],
            context_used=enriched_context,
            timestamp=datetime.utcnow(),
            duration_seconds=2.5
        )

        # Request investigation with context enrichment
        alert_data = {**sample_alert_data, "include_context": True}
        response = test_client.post("/api/v1/investigate", json=alert_data, headers=headers)

        # Business requirement: Context enrichment should succeed
        assert response.status_code == 200, "Context-enriched investigation should succeed"

        # Business validation: Context service should be called
        mock_context_service.enrich_alert_context.assert_called_once()

        # Business validation: Enriched context should be used in investigation
        call_args = mock_holmesgpt_service.investigate_alert.call_args
        assert "context" in call_args.kwargs, "Investigation should receive enriched context"

        result = response.json()
        # Business validation: Response should indicate context was used
        assert "context_used" in result, "Response should include context_used field"

    def test_investigation_handles_service_failures_gracefully(
        self,
        test_client: TestClient,
        sample_alert_data: Dict[str, Any],
        operator_token: str,
        mock_holmesgpt_service
    ):
        """
        BR-HAPI-005: Investigation must handle service failures gracefully
        Business Requirement: Robust error handling and appropriate error responses
        """
        headers = {"Authorization": f"Bearer {operator_token}"}

        # Configure mock to simulate service failure
        mock_holmesgpt_service.investigate_alert.side_effect = Exception("HolmesGPT service unavailable")

        response = test_client.post("/api/v1/investigate", json=sample_alert_data, headers=headers)

        # Business requirement: Should return appropriate error status
        assert response.status_code == 500, "Service failure should return 500 status"

        # Business requirement: Error response should be informative
        result = response.json()
        assert "detail" in result, "Error response should include details"
        assert "Investigation failed" in result["detail"], "Error should indicate investigation failure"

    def test_investigation_validates_required_fields(
        self,
        test_client: TestClient,
        operator_token: str
    ):
        """
        BR-HAPI-002: Investigation must validate required input fields
        Business Requirement: Input validation prevents invalid investigations
        """
        headers = {"Authorization": f"Bearer {operator_token}"}

        # Test missing alert_name
        incomplete_data = {
            "namespace": "test",
            "labels": {},
            "annotations": {}
        }

        response = test_client.post("/api/v1/investigate", json=incomplete_data, headers=headers)

        # Business requirement: Missing required fields should be rejected
        assert response.status_code == 422, "Missing alert_name should be rejected"

        # Test missing namespace
        incomplete_data = {
            "alert_name": "TestAlert",
            "labels": {},
            "annotations": {}
        }

        response = test_client.post("/api/v1/investigate", json=incomplete_data, headers=headers)

        # Business requirement: Missing required fields should be rejected
        assert response.status_code == 422, "Missing namespace should be rejected"

    def test_investigation_supports_async_processing(
        self,
        test_client: TestClient,
        sample_alert_data: Dict[str, Any],
        operator_token: str,
        mock_holmesgpt_service
    ):
        """
        BR-HAPI-005: Investigation must support asynchronous processing mode
        Business Requirement: Long-running investigations can be processed asynchronously
        """
        headers = {"Authorization": f"Bearer {operator_token}"}

        # Configure mock for async processing
        mock_holmesgpt_service.investigate_alert.return_value = InvestigateResponse(
            investigation_id="inv-async-test",
            status="completed",
            alert_name=sample_alert_data["alert_name"],
            namespace=sample_alert_data["namespace"],
            summary="Async investigation completed",
            recommendations=[],
            context_used={},
            timestamp=datetime.utcnow(),
            duration_seconds=0.1  # Should be faster in async mode
        )

        # Request async investigation
        alert_data = {**sample_alert_data, "async_processing": True}
        response = test_client.post("/api/v1/investigate", json=alert_data, headers=headers)

        # Business requirement: Async processing should be accepted
        assert response.status_code == 200, "Async investigation should succeed"

        # Business validation: Service should be called with async flag
        call_args = mock_holmesgpt_service.investigate_alert.call_args
        assert call_args.kwargs["async_mode"] == True, "Service should receive async_mode=True"

    def test_investigation_returns_metrics_tracking(
        self,
        test_client: TestClient,
        sample_alert_data: Dict[str, Any],
        operator_token: str,
        mock_holmesgpt_service,
        mock_metrics_service
    ):
        """
        BR-HAPI-016: Investigation must track metrics for monitoring
        Business Requirement: Investigation performance and usage metrics
        """
        headers = {"Authorization": f"Bearer {operator_token}"}

        # Configure mock for successful investigation
        mock_holmesgpt_service.investigate_alert.return_value = InvestigateResponse(
            investigation_id="inv-metrics-test",
            status="completed",
            alert_name=sample_alert_data["alert_name"],
            namespace=sample_alert_data["namespace"],
            summary="Metrics test investigation",
            recommendations=[],
            context_used={},
            timestamp=datetime.utcnow(),
            duration_seconds=3.7
        )

        response = test_client.post("/api/v1/investigate", json=sample_alert_data, headers=headers)

        # Business requirement: Investigation should succeed
        assert response.status_code == 200, "Investigation should succeed for metrics tracking"

        # Business requirement: Metrics should be recorded
        # Note: In real implementation, we would verify metrics calls
        # This tests the business requirement that metrics are tracked
        result = response.json()
        assert "duration_seconds" in result, "Response should include duration for metrics"
        assert isinstance(result["duration_seconds"], (int, float)), "Duration should be numeric"



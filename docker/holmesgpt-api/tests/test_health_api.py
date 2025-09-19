"""
Health API Tests - Business Requirements BR-HAPI-016 through BR-HAPI-020
Following TDD principles: Test business requirements, not implementation
"""

import pytest
from fastapi.testclient import TestClient
from typing import Dict, Any
from unittest.mock import Mock


class TestHealthAPI:
    """Test Health API endpoints following business requirements"""

    def test_health_endpoint_exists_and_accepts_get_requests(self, test_client: TestClient):
        """
        BR-HAPI-016: Health endpoint must exist and accept GET requests
        Business Requirement: API must provide health monitoring capability
        """
        response = test_client.get("/health")

        # Should exist and return some response (not 404)
        assert response.status_code != 404, "Health endpoint should exist"
        # Should be 200 (healthy) or 503 (unhealthy), not method not allowed
        assert response.status_code in [200, 503], "Health endpoint should accept GET requests"

    def test_health_endpoint_does_not_require_authentication(self, test_client: TestClient):
        """
        BR-HAPI-016: Health endpoint must be accessible without authentication
        Business Requirement: External monitoring systems need unauthenticated access
        """
        # Test without authentication headers
        response = test_client.get("/health")

        # Should not require authentication (not 401 or 403)
        assert response.status_code not in [401, 403], "Health endpoint should not require authentication"

    def test_health_check_reports_overall_system_status(
        self,
        test_client: TestClient,
        mock_holmesgpt_service,
        mock_context_service
    ):
        """
        BR-HAPI-016, BR-HAPI-019: Health check must report overall system status
        Business Requirement: Comprehensive health monitoring for system reliability
        """
        # Configure mocks for healthy system
        mock_holmesgpt_service.health_check.return_value = True
        mock_context_service.health_check.return_value = True

        response = test_client.get("/health")

        # Business requirement: Healthy system should return 200
        assert response.status_code == 200, "Healthy system should return 200"

        result = response.json()

        # Business validation: Response must contain required health fields
        assert "status" in result, "Response must include overall status"
        assert "timestamp" in result, "Response must include timestamp"
        assert "services" in result, "Response must include service health"
        assert "version" in result, "Response must include version"

        # Business validation: Overall status should reflect system health
        assert result["status"] in ["healthy", "degraded", "unhealthy"], "Status should be valid health state"

    def test_health_check_reports_individual_service_status(
        self,
        test_client: TestClient,
        mock_holmesgpt_service,
        mock_context_service
    ):
        """
        BR-HAPI-019: Health check must report individual service health
        Business Requirement: Granular health monitoring for troubleshooting
        """
        # Configure mocks for service-specific health
        mock_holmesgpt_service.health_check.return_value = True
        mock_context_service.health_check.return_value = False  # Simulate context API issue

        response = test_client.get("/health")

        # Business requirement: Should still return response even with service issues
        assert response.status_code in [200, 503], "Should return health status regardless"

        result = response.json()

        # Business validation: Should include individual service status
        assert "services" in result, "Response must include services health"
        services = result["services"]

        # Business validation: Should report HolmesGPT service health
        assert "holmesgpt_sdk" in services, "Should report HolmesGPT SDK health"

        # Business validation: Should report Context API health
        assert "context_api" in services, "Should report Context API health"

    def test_health_check_handles_service_failures_gracefully(
        self,
        test_client: TestClient,
        mock_holmesgpt_service,
        mock_context_service
    ):
        """
        BR-HAPI-019: Health check must handle service failures gracefully
        Business Requirement: Robust health monitoring that doesn't fail completely
        """
        # Configure mocks to simulate service failures
        mock_holmesgpt_service.health_check.side_effect = Exception("HolmesGPT service error")
        mock_context_service.health_check.side_effect = Exception("Context API error")

        response = test_client.get("/health")

        # Business requirement: Health check should not crash on service failures
        assert response.status_code in [200, 503], "Health check should handle service failures"

        result = response.json()

        # Business validation: Should still provide health information
        assert "status" in result, "Should still provide overall status"
        assert "timestamp" in result, "Should include timestamp"

        # Business validation: Overall status should reflect degraded/unhealthy state
        assert result["status"] in ["degraded", "unhealthy"], "Status should reflect service failures"

    def test_readiness_endpoint_exists_and_accepts_get_requests(self, test_client: TestClient):
        """
        BR-HAPI-017: Readiness endpoint must exist for Kubernetes probes
        Business Requirement: Kubernetes deployment readiness monitoring
        """
        response = test_client.get("/ready")

        # Should exist and return some response (not 404)
        assert response.status_code != 404, "Readiness endpoint should exist"
        # Should be 200 (ready) or 503 (not ready)
        assert response.status_code in [200, 503], "Readiness endpoint should accept GET requests"

    def test_readiness_check_verifies_service_initialization(
        self,
        test_client: TestClient
    ):
        """
        BR-HAPI-017: Readiness check must verify service initialization
        Business Requirement: Services must be ready before accepting traffic
        """
        response = test_client.get("/ready")

        # Business requirement: Should return readiness status
        assert response.status_code in [200, 503], "Should return readiness status"

        result = response.json()

        # Business validation: Response must contain readiness information
        assert "ready" in result, "Response must include readiness status"
        assert "timestamp" in result, "Response must include timestamp"
        assert "services" in result, "Response must include service readiness"

        # Business validation: Readiness should be boolean
        assert isinstance(result["ready"], bool), "Readiness should be boolean value"

    def test_status_endpoint_provides_service_capabilities(
        self,
        test_client: TestClient,
        mock_holmesgpt_service
    ):
        """
        BR-HAPI-020: Status endpoint must provide service capabilities
        Business Requirement: Service discovery and capability reporting
        """
        # Configure mock to return capabilities
        mock_holmesgpt_service.get_capabilities.return_value = [
            "alert_investigation",
            "interactive_chat",
            "kubernetes_analysis",
            "prometheus_metrics"
        ]

        response = test_client.get("/status")

        # Business requirement: Status endpoint should be accessible
        assert response.status_code == 200, "Status endpoint should be accessible"

        result = response.json()

        # Business validation: Response must contain service information
        assert "service" in result, "Response must include service name"
        assert "version" in result, "Response must include version"
        assert "status" in result, "Response must include status"
        assert "capabilities" in result, "Response must include capabilities"
        assert "timestamp" in result, "Response must include timestamp"

        # Business validation: Capabilities should be provided
        capabilities = result["capabilities"]
        assert isinstance(capabilities, list), "Capabilities should be a list"
        assert len(capabilities) > 0, "Should provide at least one capability"

    def test_health_endpoints_provide_monitoring_metrics(
        self,
        test_client: TestClient
    ):
        """
        BR-HAPI-016: Health endpoints must provide data for monitoring systems
        Business Requirement: Integration with external monitoring and alerting
        """
        response = test_client.get("/health")

        # Business requirement: Should provide structured data for monitoring
        assert response.status_code in [200, 503], "Should provide health status"

        result = response.json()

        # Business validation: Should provide timestamp for monitoring
        assert "timestamp" in result, "Must provide timestamp for monitoring"
        assert isinstance(result["timestamp"], (int, float)), "Timestamp should be numeric"

        # Business validation: Should provide version for monitoring
        assert "version" in result, "Must provide version for monitoring"

        # Business validation: Should provide structured status for alerting
        assert "status" in result, "Must provide status for alerting"

        # Business validation: Status should be machine-readable
        valid_statuses = ["healthy", "degraded", "unhealthy"]
        assert result["status"] in valid_statuses, f"Status should be one of {valid_statuses}"

    def test_health_responses_are_json_formatted(self, test_client: TestClient):
        """
        BR-HAPI-016: Health responses must be JSON formatted
        Business Requirement: Structured data for programmatic monitoring
        """
        endpoints = ["/health", "/ready", "/status"]

        for endpoint in endpoints:
            response = test_client.get(endpoint)

            # Business requirement: Should return JSON content
            assert response.headers["content-type"].startswith("application/json"), \
                f"{endpoint} should return JSON content"

            # Business validation: Should be valid JSON
            try:
                result = response.json()
                assert isinstance(result, dict), f"{endpoint} should return JSON object"
            except ValueError:
                pytest.fail(f"{endpoint} should return valid JSON")

    def test_health_endpoints_handle_high_load(self, test_client: TestClient):
        """
        BR-HAPI-018: Health endpoints must handle high request volume
        Business Requirement: Monitoring systems may check health frequently
        """
        # Simulate multiple concurrent health checks
        responses = []
        for _ in range(10):
            response = test_client.get("/health")
            responses.append(response)

        # Business requirement: All health checks should succeed
        for i, response in enumerate(responses):
            assert response.status_code in [200, 503], f"Health check {i} should not fail due to load"

        # Business validation: Responses should be consistent
        statuses = [resp.json()["status"] for resp in responses if resp.status_code == 200]
        if statuses:
            # All healthy responses should have same status (system state shouldn't change rapidly)
            first_status = statuses[0]
            assert all(status == first_status for status in statuses), \
                "Health status should be consistent across rapid checks"

    def test_health_check_performance_requirements(self, test_client: TestClient):
        """
        BR-HAPI-018: Health checks must meet performance requirements
        Business Requirement: Fast health checks for responsive monitoring
        """
        import time

        start_time = time.time()
        response = test_client.get("/health")
        end_time = time.time()

        duration = end_time - start_time

        # Business requirement: Health check should be fast (under 5 seconds)
        assert duration < 5.0, f"Health check took {duration}s, should be under 5s"

        # Business requirement: Should return a response
        assert response.status_code in [200, 503], "Health check should return status"



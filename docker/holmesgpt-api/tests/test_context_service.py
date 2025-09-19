"""
Context API Service Tests - Business Requirements BR-HAPI-011 through BR-HAPI-015
Following TDD principles: Test business requirements, not implementation
"""

import pytest
import asyncio
from unittest.mock import Mock, AsyncMock, patch
from typing import Dict, Any

from services.context_api_service import ContextAPIService
from config import Settings


class TestContextAPIService:
    """Test Context API Service following business requirements"""

    @pytest.fixture
    def context_service(self, test_settings: Settings) -> ContextAPIService:
        """Create context service instance for testing"""
        return ContextAPIService(test_settings)

    @pytest.mark.asyncio
    async def test_service_initialization_succeeds_with_valid_config(
        self,
        context_service: ContextAPIService
    ):
        """
        BR-HAPI-011: Service must initialize successfully with valid configuration
        Business Requirement: Reliable context service startup for enrichment capabilities
        """
        # Business requirement: Service should initialize successfully
        result = await context_service.initialize()

        # Business validation: Initialization should succeed
        assert result == True, "Service initialization should succeed with valid config"

        # Business validation: Service should be marked as initialized
        assert context_service._initialized == True, "Service should be marked as initialized"

        # Business validation: HTTP client should be configured
        assert context_service._client is not None, "HTTP client should be configured"

    @pytest.mark.asyncio
    async def test_service_health_check_reports_accurate_status(
        self,
        context_service: ContextAPIService
    ):
        """
        BR-HAPI-011: Service must provide accurate health status
        Business Requirement: Reliable health monitoring for context API integration
        """
        await context_service.initialize()

        # Mock successful health check response
        with patch('httpx.AsyncClient.get') as mock_get:
            mock_response = Mock()
            mock_response.status_code = 200
            mock_get.return_value = mock_response

            # Business requirement: Health check should succeed for available service
            health_status = await context_service.health_check()

            # Business validation: Available service should be healthy
            assert health_status == True, "Available context service should report healthy status"

    @pytest.mark.asyncio
    async def test_service_health_check_handles_unavailable_service(
        self,
        context_service: ContextAPIService
    ):
        """
        BR-HAPI-011: Service must handle unavailable context API gracefully
        Business Requirement: Graceful degradation when context API is unavailable
        """
        await context_service.initialize()

        # Mock service unavailable response
        with patch('httpx.AsyncClient.get') as mock_get:
            mock_response = Mock()
            mock_response.status_code = 503
            mock_get.return_value = mock_response

            # Business requirement: Unavailable service should report unhealthy
            health_status = await context_service.health_check()

            # Business validation: Unavailable service should be unhealthy
            assert health_status == False, "Unavailable context service should report unhealthy status"

    @pytest.mark.asyncio
    async def test_enrich_alert_context_provides_enhanced_information(
        self,
        context_service: ContextAPIService
    ):
        """
        BR-HAPI-013: Service must enrich alert context with organizational intelligence
        Business Requirement: Enhanced context for more accurate investigations
        """
        await context_service.initialize()

        # Test data
        alert_name = "PodCrashLooping"
        namespace = "production"
        labels = {"severity": "warning", "team": "platform"}
        annotations = {"description": "Pod is crash looping"}

        # Mock context API responses
        mock_responses = {
            "namespace_context": {"pod_count": 15, "service_count": 8},
            "alert_history": {"similar_alerts": 3, "last_occurrence": "2024-01-01"},
            "resource_context": {"cpu_usage": "85%", "memory_usage": "78%"},
            "pattern_analysis": {"correlation_score": 0.8, "recommendations": ["scale_up"]}
        }

        with patch('httpx.AsyncClient.get') as mock_get, \
             patch('httpx.AsyncClient.post') as mock_post:

            # Configure mocks for different endpoint calls
            def mock_response_factory(status_code, json_data):
                mock_resp = Mock()
                mock_resp.status_code = status_code
                mock_resp.json.return_value = json_data
                return mock_resp

            mock_get.side_effect = [
                mock_response_factory(200, mock_responses["namespace_context"]),
                mock_response_factory(200, mock_responses["alert_history"]),
                mock_response_factory(200, mock_responses["resource_context"])
            ]
            mock_post.return_value = mock_response_factory(200, mock_responses["pattern_analysis"])

            # Business requirement: Service should enrich context successfully
            enriched_context = await context_service.enrich_alert_context(
                alert_name, namespace, labels, annotations
            )

            # Business validation: Should return enriched context
            assert isinstance(enriched_context, dict), "Should return context dictionary"
            assert enriched_context["alert_name"] == alert_name, "Should include original alert name"
            assert enriched_context["namespace"] == namespace, "Should include original namespace"

            # Business validation: Should include enriched data
            assert "namespace_context" in enriched_context, "Should include namespace context"
            assert "alert_history" in enriched_context, "Should include alert history"
            assert "resource_context" in enriched_context, "Should include resource context"
            assert "pattern_analysis" in enriched_context, "Should include pattern analysis"

    @pytest.mark.asyncio
    async def test_enrich_alert_context_uses_caching(
        self,
        context_service: ContextAPIService
    ):
        """
        BR-HAPI-014: Service must cache context data for performance
        Business Requirement: Efficient context retrieval through caching
        """
        await context_service.initialize()

        alert_name = "TestAlert"
        namespace = "test"
        labels = {}
        annotations = {}

        # Mock basic context response
        with patch.object(context_service, '_create_basic_context') as mock_basic:
            mock_context = {"alert_name": alert_name, "cached": True}
            mock_basic.return_value = mock_context

            # First call should populate cache
            first_result = await context_service.enrich_alert_context(
                alert_name, namespace, labels, annotations
            )

            # Second call should use cache
            second_result = await context_service.enrich_alert_context(
                alert_name, namespace, labels, annotations
            )

            # Business validation: Results should be identical (from cache)
            assert first_result == second_result, "Cached results should be identical"

            # Business validation: Cache should contain the data
            cache_key = f"alert:{alert_name}:{namespace}"
            assert cache_key in context_service._cache, "Context should be cached"

    @pytest.mark.asyncio
    async def test_enrich_alert_context_handles_parallel_requests(
        self,
        context_service: ContextAPIService
    ):
        """
        BR-HAPI-015: Service must handle parallel context gathering efficiently
        Business Requirement: Concurrent context collection for performance
        """
        await context_service.initialize()

        # Mock multiple endpoints to verify parallel execution
        with patch('httpx.AsyncClient.get') as mock_get, \
             patch('httpx.AsyncClient.post') as mock_post:

            # Configure mocks to track call timing
            call_times = []

            def track_get_calls(*args, **kwargs):
                import time
                call_times.append(time.time())
                mock_resp = Mock()
                mock_resp.status_code = 200
                mock_resp.json.return_value = {"data": "test"}
                return mock_resp

            def track_post_calls(*args, **kwargs):
                import time
                call_times.append(time.time())
                mock_resp = Mock()
                mock_resp.status_code = 200
                mock_resp.json.return_value = {"data": "test"}
                return mock_resp

            mock_get.side_effect = track_get_calls
            mock_post.side_effect = track_post_calls

            # Business requirement: Context enrichment should handle parallel requests
            result = await context_service.enrich_alert_context(
                "ParallelTest", "test", {}, {}
            )

            # Business validation: Multiple API calls should have been made
            assert len(call_times) >= 3, "Should make multiple parallel API calls"

            # Business validation: Should return enriched context
            assert isinstance(result, dict), "Should return enriched context"

    @pytest.mark.asyncio
    async def test_get_current_context_provides_cluster_information(
        self,
        context_service: ContextAPIService
    ):
        """
        BR-HAPI-012: Service must provide current cluster context
        Business Requirement: Real-time cluster information for chat sessions
        """
        await context_service.initialize()

        # Mock current context response
        mock_context = {
            "cluster_status": "healthy",
            "total_nodes": 5,
            "total_pods": 150,
            "namespaces": ["default", "kube-system", "production"]
        }

        with patch('httpx.AsyncClient.get') as mock_get:
            mock_response = Mock()
            mock_response.status_code = 200
            mock_response.json.return_value = mock_context
            mock_get.return_value = mock_response

            # Business requirement: Should provide current context
            current_context = await context_service.get_current_context()

            # Business validation: Should return context data
            assert isinstance(current_context, dict), "Should return context dictionary"
            assert current_context["cluster_status"] == "healthy", "Should include cluster status"
            assert "total_pods" in current_context, "Should include pod count"

    @pytest.mark.asyncio
    async def test_get_current_context_supports_namespace_filtering(
        self,
        context_service: ContextAPIService
    ):
        """
        BR-HAPI-012: Service must support namespace-specific context filtering
        Business Requirement: Scoped context for targeted troubleshooting
        """
        await context_service.initialize()

        namespace = "production"

        with patch('httpx.AsyncClient.get') as mock_get:
            mock_response = Mock()
            mock_response.status_code = 200
            mock_response.json.return_value = {"namespace": namespace, "pod_count": 25}
            mock_get.return_value = mock_response

            # Business requirement: Should support namespace filtering
            context = await context_service.get_current_context(namespace=namespace)

            # Business validation: Should call API with namespace parameter
            mock_get.assert_called_once()
            call_args = mock_get.call_args
            assert "namespace" in call_args.kwargs["params"], "Should include namespace parameter"
            assert call_args.kwargs["params"]["namespace"] == namespace, "Should filter by namespace"

    @pytest.mark.asyncio
    async def test_get_current_context_includes_metrics_when_requested(
        self,
        context_service: ContextAPIService
    ):
        """
        BR-HAPI-012: Service must include metrics data when requested
        Business Requirement: Metrics-aware context for performance troubleshooting
        """
        await context_service.initialize()

        with patch('httpx.AsyncClient.get') as mock_get:
            mock_response = Mock()
            mock_response.status_code = 200
            mock_response.json.return_value = {
                "metrics": {
                    "cpu_usage": "85%",
                    "memory_usage": "78%",
                    "disk_usage": "45%"
                }
            }
            mock_get.return_value = mock_response

            # Business requirement: Should include metrics when requested
            context = await context_service.get_current_context(include_metrics=True)

            # Business validation: Should call API with metrics flag
            mock_get.assert_called_once()
            call_args = mock_get.call_args
            assert "include_metrics" in call_args.kwargs["params"], "Should include metrics parameter"
            assert call_args.kwargs["params"]["include_metrics"] == "true", "Should request metrics"

    @pytest.mark.asyncio
    async def test_service_provides_fallback_when_context_api_unavailable(
        self,
        context_service: ContextAPIService
    ):
        """
        BR-HAPI-013: Service must provide fallback context when Context API is unavailable
        Business Requirement: Graceful degradation ensures investigation continues
        """
        await context_service.initialize()

        # Simulate Context API unavailability
        context_service._client = None

        # Business requirement: Should provide basic context as fallback
        fallback_context = await context_service.enrich_alert_context(
            "FallbackTest", "test", {"severity": "high"}, {"description": "test alert"}
        )

        # Business validation: Should return basic context
        assert isinstance(fallback_context, dict), "Should return fallback context"
        assert fallback_context["alert_name"] == "FallbackTest", "Should include alert name"
        assert fallback_context["enrichment_status"] == "basic_only", "Should indicate fallback mode"
        assert fallback_context["context_api_available"] == False, "Should indicate API unavailability"

    @pytest.mark.asyncio
    async def test_service_cleanup_releases_resources(
        self,
        context_service: ContextAPIService
    ):
        """
        BR-HAPI-040: Service must properly cleanup resources on shutdown
        Business Requirement: Graceful shutdown prevents resource leaks
        """
        await context_service.initialize()

        # Add some cached data
        context_service._cache["test_key"] = {"test": "data"}

        # Verify resources exist
        assert context_service._client is not None, "HTTP client should exist before cleanup"
        assert len(context_service._cache) > 0, "Cache should have data before cleanup"

        # Business requirement: Cleanup should succeed
        await context_service.cleanup()

        # Business validation: Resources should be released
        assert len(context_service._cache) == 0, "Cache should be cleared"
        assert context_service._initialized == False, "Service should be marked as uninitialized"

    @pytest.mark.asyncio
    async def test_service_handles_network_errors_gracefully(
        self,
        context_service: ContextAPIService
    ):
        """
        BR-HAPI-011: Service must handle network errors gracefully
        Business Requirement: Robust error handling for reliable context enrichment
        """
        await context_service.initialize()

        # Mock network error
        with patch('httpx.AsyncClient.get') as mock_get:
            mock_get.side_effect = Exception("Network error")

            # Business requirement: Should handle network errors gracefully
            result = await context_service.enrich_alert_context(
                "NetworkErrorTest", "test", {}, {}
            )

            # Business validation: Should return fallback context on error
            assert isinstance(result, dict), "Should return context despite network error"
            assert "alert_name" in result, "Should include basic alert information"

    @pytest.mark.asyncio
    async def test_service_handles_invalid_api_responses(
        self,
        context_service: ContextAPIService
    ):
        """
        BR-HAPI-011: Service must handle invalid API responses gracefully
        Business Requirement: Robust response processing for reliable operation
        """
        await context_service.initialize()

        # Mock invalid response
        with patch('httpx.AsyncClient.get') as mock_get:
            mock_response = Mock()
            mock_response.status_code = 500
            mock_response.json.side_effect = Exception("Invalid JSON")
            mock_get.return_value = mock_response

            # Business requirement: Should handle invalid responses gracefully
            result = await context_service.enrich_alert_context(
                "InvalidResponseTest", "test", {}, {}
            )

            # Business validation: Should return fallback context on invalid response
            assert isinstance(result, dict), "Should return context despite invalid response"
            assert "alert_name" in result, "Should include basic alert information"



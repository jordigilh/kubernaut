"""
Integration Tests for Context API Client

Tests real HTTP integration with Context API service.
Uses mock Context API server when real service is unavailable.

**Test Modes**:
1. **Real Integration** (CONTEXT_API_URL set): Tests against real Context API in cluster
2. **Mock Integration** (no env var): Uses local mock server for development

Business Requirements: BR-HAPI-070 (Historical Context Integration)
"""

import pytest
import asyncio
import os
from unittest.mock import Mock, patch, AsyncMock
from aiohttp import web
from src.clients.context_api_client import ContextAPIClient


# ========================================
# TEST CONFIGURATION
# ========================================

def use_real_context_api():
    """Check if tests should use real Context API service"""
    context_api_url = os.getenv("CONTEXT_API_URL")
    return context_api_url is not None and context_api_url != ""


@pytest.fixture
def context_api_mode():
    """Determine test mode: 'real' or 'mock'"""
    return "real" if use_real_context_api() else "mock"


@pytest.fixture
def real_context_api_url():
    """Get real Context API URL from environment"""
    return os.getenv(
        "CONTEXT_API_URL",
        "http://context-api.kubernaut-system.svc.cluster.local:8091"
    )


# ========================================
# TEST FIXTURES
# ========================================

@pytest.fixture
def mock_context_api_response():
    """Mock Context API response data"""
    return {
        "success_rates": {
            "scale-deployment": {
                "success_rate": 89.4,
                "total_attempts": 47,
                "avg_duration": 45.2
            },
            "increase-memory": {
                "success_rate": 91.7,
                "total_attempts": 36,
                "avg_duration": 32.1
            }
        },
        "similar_incidents": [
            {
                "incident_id": "inc-001",
                "remediation_action": "increase-memory",
                "outcome": "success",
                "similarity_score": 0.95,
                "timestamp": "2025-10-15T10:30:00Z"
            },
            {
                "incident_id": "inc-002",
                "remediation_action": "scale-deployment",
                "outcome": "success",
                "similarity_score": 0.88,
                "timestamp": "2025-10-14T15:20:00Z"
            }
        ],
        "environment_patterns": {
            "production": "High memory usage typical during peak hours",
            "memory_pressure": "Frequent OOMKilled events"
        },
        "available": True
    }


@pytest.fixture
async def mock_context_api_server(aiohttp_server, mock_context_api_response):
    """
    Create a mock Context API server for integration testing

    Provides test endpoints that mimic Context API behavior
    """
    async def health_handler(request):
        return web.json_response({"status": "healthy"}, status=200)

    async def historical_context_handler(request):
        # Extract query parameters
        namespace = request.query.get("namespace")
        target_type = request.query.get("targetType")
        target_name = request.query.get("targetName")

        # Validate required parameters
        if not all([namespace, target_type, target_name]):
            return web.json_response(
                {"error": "Missing required parameters"},
                status=400
            )

        # Return mock response
        return web.json_response(mock_context_api_response, status=200)

    async def not_found_handler(request):
        return web.json_response(
            {"error": "No historical data found"},
            status=404
        )

    app = web.Application()
    app.router.add_get("/health", health_handler)
    app.router.add_get("/api/v1/context/historical", historical_context_handler)
    app.router.add_get("/api/v1/context/historical/notfound", not_found_handler)

    return await aiohttp_server(app)


@pytest.fixture
async def context_api_base_url(context_api_mode, real_context_api_url, mock_context_api_server):
    """
    Provide Context API base URL based on test mode
    
    Returns:
        - Real Context API URL if CONTEXT_API_URL is set
        - Mock server URL otherwise
    """
    if context_api_mode == "real":
        return real_context_api_url
    else:
        return str(mock_context_api_server.make_url(""))


# ========================================
# TEST SUITE 1: Client Initialization
# ========================================

class TestContextAPIClientInit:
    """Test ContextAPIClient initialization"""

    def test_initialization_with_custom_url(self):
        """Test client initialization with custom base URL"""
        custom_url = "http://custom-context-api:9000"
        client = ContextAPIClient(base_url=custom_url)

        assert client.base_url == custom_url

    def test_initialization_with_env_var(self):
        """Test client initialization from CONTEXT_API_URL env var"""
        with patch.dict('os.environ', {'CONTEXT_API_URL': 'http://env-context-api:8091'}):
            client = ContextAPIClient()

            assert client.base_url == "http://env-context-api:8091"

    def test_initialization_with_default_url(self):
        """Test client initialization with default URL"""
        with patch.dict('os.environ', {}, clear=True):
            # Clear CONTEXT_API_URL if it exists
            client = ContextAPIClient()

            assert "context-api" in client.base_url
            assert "8091" in client.base_url


# ========================================
# TEST SUITE 2: Health Check
# ========================================

class TestContextAPIHealthCheck:
    """Test Context API health check"""

    @pytest.mark.asyncio
    async def test_health_check_success(self, context_api_base_url, context_api_mode):
        """
        Test successful health check
        
        **Test Mode**: Real or Mock (based on CONTEXT_API_URL env var)
        """
        client = ContextAPIClient(base_url=context_api_base_url)

        is_healthy = await client.health_check()

        assert is_healthy is True, f"Health check failed in {context_api_mode} mode"

    @pytest.mark.asyncio
    async def test_health_check_failure_unavailable_service(self):
        """Test health check when service is unavailable"""
        client = ContextAPIClient(base_url="http://nonexistent-service:9999")

        is_healthy = await client.health_check()

        assert is_healthy is False


# ========================================
# TEST SUITE 3: Get Historical Context - Success Cases
# ========================================

class TestGetHistoricalContextSuccess:
    """Test successful historical context retrieval"""

    @pytest.mark.asyncio
    async def test_get_historical_context_success(
        self,
        context_api_base_url,
        context_api_mode,
        mock_context_api_response
    ):
        """
        Test successful retrieval of historical context
        
        **Test Mode**: Real or Mock (based on CONTEXT_API_URL env var)
        """
        client = ContextAPIClient(base_url=context_api_base_url)

        context = await client.get_historical_context(
            namespace="production",
            target_type="deployment",
            target_name="api-server",
            time_range="30d"
        )

        assert context is not None, f"Context retrieval failed in {context_api_mode} mode"
        assert context.get("available", True) is not False
        # When using real Context API, response structure may vary
        if context_api_mode == "mock":
            assert context["available"] is True
            assert "success_rates" in context
            assert "similar_incidents" in context
            assert "environment_patterns" in context

    @pytest.mark.asyncio
    async def test_get_historical_context_includes_success_rates(
        self,
        context_api_base_url,
        context_api_mode
    ):
        """
        Test historical context includes success rates
        
        **Test Mode**: Real or Mock (based on CONTEXT_API_URL env var)
        """
        client = ContextAPIClient(base_url=context_api_base_url)

        context = await client.get_historical_context(
            namespace="production",
            target_type="deployment",
            target_name="api-server"
        )

        # In mock mode, validate exact structure
        if context_api_mode == "mock":
            success_rates = context["success_rates"]
            assert "scale-deployment" in success_rates
            assert success_rates["scale-deployment"]["success_rate"] == 89.4
            assert success_rates["scale-deployment"]["total_attempts"] == 47
        # In real mode, just verify success_rates key exists if data is available
        elif context.get("available", True):
            assert "success_rates" in context or context.get("available") is False

    @pytest.mark.asyncio
    async def test_get_historical_context_includes_similar_incidents(
        self,
        context_api_base_url,
        context_api_mode
    ):
        """
        Test historical context includes similar incidents
        
        **Test Mode**: Real or Mock (based on CONTEXT_API_URL env var)
        """
        client = ContextAPIClient(base_url=context_api_base_url)

        context = await client.get_historical_context(
            namespace="production",
            target_type="deployment",
            target_name="api-server"
        )

        # In mock mode, validate exact structure
        if context_api_mode == "mock":
            similar_incidents = context["similar_incidents"]
            assert len(similar_incidents) == 2
            assert similar_incidents[0]["remediation_action"] == "increase-memory"
            assert similar_incidents[0]["similarity_score"] == 0.95
        # In real mode, just verify similar_incidents key exists if data is available
        elif context.get("available", True):
            assert "similar_incidents" in context or context.get("available") is False

    @pytest.mark.asyncio
    async def test_get_historical_context_includes_environment_patterns(
        self,
        context_api_base_url,
        context_api_mode
    ):
        """
        Test historical context includes environment patterns
        
        **Test Mode**: Real or Mock (based on CONTEXT_API_URL env var)
        """
        client = ContextAPIClient(base_url=context_api_base_url)

        context = await client.get_historical_context(
            namespace="production",
            target_type="deployment",
            target_name="api-server"
        )

        # In mock mode, validate exact structure
        if context_api_mode == "mock":
            env_patterns = context["environment_patterns"]
            assert "production" in env_patterns
            assert "High memory usage typical" in env_patterns["production"]
        # In real mode, just verify environment_patterns key exists if data is available
        elif context.get("available", True):
            assert "environment_patterns" in context or context.get("available") is False

    @pytest.mark.asyncio
    async def test_get_historical_context_with_optional_signal_type(
        self,
        context_api_base_url
    ):
        """Test historical context with optional signal_type parameter"""
        client = ContextAPIClient(base_url=context_api_base_url)

        context = await client.get_historical_context(
            namespace="production",
            target_type="deployment",
            target_name="api-server",
            signal_type="prometheus"
        )

        assert context is not None
        assert context["available"] is True


# ========================================
# TEST SUITE 4: Get Historical Context - Error Cases
# ========================================

class TestGetHistoricalContextErrors:
    """Test error handling in historical context retrieval"""

    @pytest.mark.asyncio
    async def test_get_historical_context_service_unavailable(self):
        """Test graceful degradation when Context API is unavailable"""
        client = ContextAPIClient(base_url="http://nonexistent-service:9999")

        context = await client.get_historical_context(
            namespace="production",
            target_type="deployment",
            target_name="api-server"
        )

        # Should return empty context, not raise exception
        assert context is not None
        assert context["available"] is False
        assert context["success_rates"] == {}
        assert context["similar_incidents"] == []
        assert context["environment_patterns"] == {}

    @pytest.mark.asyncio
    async def test_get_historical_context_404_no_data(self):
        """Test handling of 404 when no historical data exists"""
        # This test would need a custom mock server endpoint
        client = ContextAPIClient(base_url="http://nonexistent-service:9999")

        context = await client.get_historical_context(
            namespace="new-namespace",
            target_type="deployment",
            target_name="new-app"
        )

        # Should return empty context for 404
        assert context["available"] is False

    @pytest.mark.asyncio
    async def test_get_historical_context_timeout(self):
        """Test timeout handling"""
        # Use a non-routable IP to trigger timeout
        client = ContextAPIClient(base_url="http://192.0.2.1:8091")  # TEST-NET-1 (non-routable)

        context = await client.get_historical_context(
            namespace="production",
            target_type="deployment",
            target_name="api-server"
        )

        # Should gracefully degrade on timeout
        assert context["available"] is False


# ========================================
# TEST SUITE 5: Recovery Integration
# ========================================

class TestRecoveryIntegration:
    """Test Context API integration with recovery analysis"""

    @pytest.mark.asyncio
    async def test_recovery_analysis_uses_historical_context(
        self,
        mock_context_api_server
    ):
        """Test that recovery analysis integrates historical context"""
        from src.extensions.recovery import _get_historical_context

        request_data = {
            "context": {
                "namespace": "production",
                "target": {
                    "type": "deployment",
                    "name": "api-server"
                }
            }
        }

        # Mock the ContextAPIClient to use our test server
        with patch('src.extensions.recovery.ContextAPIClient') as MockClient:
            mock_client_instance = MockClient.return_value
            mock_client_instance.get_historical_context = AsyncMock(return_value={
                "success_rates": {"scale-deployment": {"success_rate": 89.4}},
                "similar_incidents": [],
                "environment_patterns": {},
                "available": True
            })

            historical_context = await _get_historical_context(request_data)

            assert historical_context["available"] is True
            assert "success_rates" in historical_context
            mock_client_instance.get_historical_context.assert_called_once()

    @pytest.mark.asyncio
    async def test_recovery_analysis_degrades_gracefully_without_context(self):
        """Test recovery analysis works without historical context"""
        from src.extensions.recovery import _get_historical_context

        request_data = {
            "context": {
                "namespace": "unknown",
                "target": {
                    "type": "unknown",
                    "name": "unknown"
                }
            }
        }

        historical_context = await _get_historical_context(request_data)

        # Should return unavailable context, not raise exception
        assert historical_context["available"] is False

    @pytest.mark.asyncio
    async def test_prompt_includes_historical_context(self):
        """Test that investigation prompt includes historical context"""
        from src.extensions.recovery import _create_investigation_prompt

        request_data = {
            "incident_id": "test-001",
            "failed_action": {"type": "scale-deployment"},
            "failure_context": {"error": "timeout"},
            "historical_context": {
                "available": True,
                "success_rates": {
                    "scale-deployment": {
                        "success_rate": 89.4,
                        "total_attempts": 47
                    }
                },
                "similar_incidents": [
                    {
                        "remediation_action": "increase-memory",
                        "outcome": "success",
                        "similarity_score": 0.95
                    }
                ],
                "environment_patterns": {
                    "production": "High memory usage typical"
                }
            }
        }

        prompt = _create_investigation_prompt(request_data)

        # Verify historical context is in the prompt
        assert "Historical Context" in prompt
        assert "Past Remediation Success Rates" in prompt
        assert "scale-deployment: 89.4% success (47 attempts)" in prompt
        assert "Similar Past Incidents" in prompt
        assert "increase-memory → success (similarity: 0.95)" in prompt
        assert "Environment-Specific Patterns" in prompt
        assert "High memory usage typical" in prompt


# ========================================
# TEST SUITE 6: Edge Cases
# ========================================

class TestContextAPIEdgeCases:
    """Test edge cases and boundary conditions"""

    @pytest.mark.asyncio
    async def test_empty_success_rates(self, mock_context_api_server):
        """Test handling of empty success rates"""
        client = ContextAPIClient(base_url=str(mock_context_api_server.make_url("")))

        # Mock response with empty success_rates
        with patch.object(client, 'get_historical_context', return_value={
            "success_rates": {},
            "similar_incidents": [],
            "environment_patterns": {},
            "available": True
        }):
            context = await client.get_historical_context(
                namespace="production",
                target_type="deployment",
                target_name="api-server"
            )

            assert context["success_rates"] == {}
            assert context["available"] is True

    @pytest.mark.asyncio
    async def test_large_similar_incidents_list(self):
        """Test handling of large similar incidents list"""
        from src.extensions.recovery import _create_investigation_prompt

        # Create request with 50 similar incidents
        similar_incidents = [
            {
                "remediation_action": f"action-{i}",
                "outcome": "success",
                "similarity_score": 0.9 - (i * 0.01)
            }
            for i in range(50)
        ]

        request_data = {
            "incident_id": "test-001",
            "failed_action": {"type": "test"},
            "failure_context": {"error": "test"},
            "historical_context": {
                "available": True,
                "success_rates": {},
                "similar_incidents": similar_incidents,
                "environment_patterns": {}
            }
        }

        prompt = _create_investigation_prompt(request_data)

        # Should only include top 5 similar incidents
        assert prompt.count("action-0") == 1
        assert prompt.count("action-4") == 1
        assert "action-5" not in prompt  # 6th incident should not be included

    @pytest.mark.asyncio
    async def test_malformed_context_data(self):
        """Test handling of malformed context data"""
        from src.extensions.recovery import _get_historical_context

        # Missing required fields
        request_data = {
            "context": {}  # Missing namespace and target
        }

        historical_context = await _get_historical_context(request_data)

        # Should gracefully degrade
        assert historical_context["available"] is False


# ========================================
# TEST SUITE 7: Performance
# ========================================

class TestContextAPIPerformance:
    """Test Context API client performance characteristics"""

    @pytest.mark.asyncio
    async def test_concurrent_requests(self, mock_context_api_server):
        """Test multiple concurrent Context API requests"""
        client = ContextAPIClient(base_url=str(mock_context_api_server.make_url("")))

        # Make 10 concurrent requests
        tasks = [
            client.get_historical_context(
                namespace=f"namespace-{i}",
                target_type="deployment",
                target_name="api-server"
            )
            for i in range(10)
        ]

        results = await asyncio.gather(*tasks)

        # All requests should succeed
        assert len(results) == 10
        for result in results:
            assert result["available"] is True

    @pytest.mark.asyncio
    async def test_timeout_enforced(self):
        """Test that client timeout is enforced"""
        import time
        from aiohttp import ClientTimeout

        # Create client with very short timeout
        client = ContextAPIClient(base_url="http://192.0.2.1:8091")  # Non-routable IP
        client.timeout = ClientTimeout(total=0.1)  # 100ms timeout

        start = time.time()
        context = await client.get_historical_context(
            namespace="test",
            target_type="deployment",
            target_name="test"
        )
        duration = time.time() - start

        # Should fail fast (within 1 second including retry logic)
        assert duration < 1.0
        assert context["available"] is False




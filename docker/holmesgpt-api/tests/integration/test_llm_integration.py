"""
LLM Integration Tests - Business Requirements BR-HAPI-046
Testing adaptive LLM support with real LLM and mock fallback
Following TDD principles and project guidelines
"""

import pytest
import time
from typing import Dict, Any

# Test markers
pytestmark = [pytest.mark.integration, pytest.mark.llm]


class TestLLMIntegration:
    """
    Integration tests for LLM support with real and mock fallback
    Business Requirement: BR-HAPI-046 - Integration tests with adaptive LLM support
    """

    def test_llm_provider_auto_detection(self, integration_config):
        """
        BR-HAPI-046.1: LLM provider should be auto-detected based on endpoint and availability
        Business Requirement: Flexible LLM integration
        """
        import os

        # Validate configuration
        assert "llm_provider" in integration_config, "Should have LLM provider configured"
        assert "llm_endpoint" in integration_config, "Should have LLM endpoint configured"
        assert "llm_available" in integration_config, "Should have LLM availability status"

        provider = integration_config["llm_provider"]
        endpoint = integration_config["llm_endpoint"]
        explicit_provider = os.getenv("LLM_PROVIDER", "auto-detect")

        # Business validation: Test behavior based on availability and configuration
        if explicit_provider != "auto-detect":
            # When provider is explicitly set, validate graceful degradation behavior
            if integration_config["llm_available"]:
                # LLM available: should use explicit provider
                assert provider == explicit_provider, f"Available LLM should use explicit provider {explicit_provider}, got {provider}"
            else:
                # LLM unavailable: should gracefully degrade to mock (BR-HAPI-046.4)
                assert provider == "mock", f"Unavailable LLM should gracefully degrade to mock for reliability, got {provider}"
                print(f"✅ Graceful degradation: {explicit_provider} → mock (endpoint unavailable)")
        else:
            # Only test auto-detection when auto-detection is enabled
            if ":11434" in endpoint and integration_config["llm_available"]:
                assert provider == "ollama", f"Ollama endpoint should use ollama provider, got {provider}"
            elif ":8080" in endpoint and integration_config["llm_available"]:
                assert provider == "localai", f"LocalAI endpoint should use localai provider, got {provider}"
            else:
                assert provider == "mock", f"Unavailable endpoint should fallback to mock, got {provider}"

    def test_investigation_with_real_llm(
        self,
        holmesgpt_api_client,
        serviceaccount_tokens,
        integration_config,
        sample_alert_data
    ):
        """
        BR-HAPI-046: Investigation should work with real LLM when available
        Business Requirement: Real LLM Integration (when available)
        """
        # Skip if using mock LLM
        if integration_config["use_mock_llm"]:
            pytest.skip("Test requires real LLM (USE_MOCK_LLM=false)")

        # Skip if LLM not available
        if not integration_config["llm_available"]:
            pytest.skip(f"Real LLM not available at {integration_config['llm_endpoint']}")

        admin_token = serviceaccount_tokens.get("test-admin-sa")
        if not admin_token:
            pytest.skip("Admin ServiceAccount token not available")

        holmesgpt_api_client.authenticate_with_bearer_token(admin_token)

        # Submit investigation with real LLM
        response = holmesgpt_api_client.post("/api/v1/investigate", json_data=sample_alert_data)
        assert response.status_code == 200, "Investigation should work with real LLM"

        investigation = response.json()

        # Business validation: Real LLM should provide quality responses
        assert "investigation_id" in investigation, "Should return investigation ID"
        assert "recommendations" in investigation, "Should return recommendations"

        recommendations = investigation["recommendations"]
        assert len(recommendations) > 0, "Real LLM should generate recommendations"

        # Real LLM should provide more contextual responses
        recommendation_text = " ".join([r.get("description", "") for r in recommendations]).lower()

        # Business validation: Real LLM responses should be contextual and detailed
        assert len(recommendation_text) > 50, "Real LLM should provide detailed recommendations"

        # Should include alert-specific context
        alert_name = sample_alert_data["alerts"][0]["labels"]["alertname"].lower()
        assert alert_name in recommendation_text, f"Should reference alert {alert_name} in recommendations"

    def test_investigation_with_mock_llm_fallback(
        self,
        holmesgpt_api_client,
        serviceaccount_tokens,
        integration_config,
        sample_alert_data
    ):
        """
        BR-HAPI-046: Investigation should work with mock LLM fallback
        Business Requirement: Mock LLM Fallback (for CI/CD reliability)
        """
        admin_token = serviceaccount_tokens.get("test-admin-sa")
        if not admin_token:
            pytest.skip("Admin ServiceAccount token not available")

        holmesgpt_api_client.authenticate_with_bearer_token(admin_token)

        # Force mock LLM usage by setting header or using mock-specific endpoint
        # In practice, this would be configured via environment variables

        response = holmesgpt_api_client.post("/api/v1/investigate", json_data=sample_alert_data)
        assert response.status_code == 200, "Investigation should work with mock LLM"

        investigation = response.json()

        # Business validation: Mock LLM should provide consistent responses
        assert "investigation_id" in investigation, "Should return investigation ID"
        assert "recommendations" in investigation, "Should return recommendations"

        recommendations = investigation["recommendations"]
        assert len(recommendations) > 0, "Mock LLM should generate recommendations"

        # Mock responses should be predictable and fast
        assert all("recommendation" in r.get("description", "").lower() for r in recommendations), \
            "Mock LLM should provide structured recommendations"

    def test_chat_with_real_llm(
        self,
        holmesgpt_api_client,
        serviceaccount_tokens,
        integration_config
    ):
        """
        BR-HAPI-046: Chat should work with real LLM when available
        Business Requirement: Real LLM Integration for interactive features
        """
        # Skip if using mock LLM
        if integration_config["use_mock_llm"]:
            pytest.skip("Test requires real LLM (USE_MOCK_LLM=false)")

        # Skip if LLM not available
        if not integration_config["llm_available"]:
            pytest.skip(f"Real LLM not available at {integration_config['llm_endpoint']}")

        viewer_token = serviceaccount_tokens.get("test-viewer-sa")
        if not viewer_token:
            pytest.skip("Viewer ServiceAccount token not available")

        holmesgpt_api_client.authenticate_with_bearer_token(viewer_token)

        # Test chat with real LLM
        chat_data = {
            "message": "Explain the difference between pods and deployments in Kubernetes",
            "session_id": "real-llm-test-session",
            "include_context": True
        }

        response = holmesgpt_api_client.post("/api/v1/chat", json_data=chat_data)
        assert response.status_code == 200, "Chat should work with real LLM"

        chat_result = response.json()

        # Business validation: Real LLM should provide informative responses
        assert "response" in chat_result, "Should return chat response"

        chat_response = chat_result["response"].lower()

        # Real LLM should understand Kubernetes concepts
        assert any(keyword in chat_response for keyword in ["pod", "deployment", "kubernetes"]), \
            "Real LLM should understand Kubernetes concepts"

        # Should provide educational content
        assert len(chat_response) > 100, "Real LLM should provide detailed explanations"

    def test_chat_with_mock_llm_fallback(
        self,
        holmesgpt_api_client,
        serviceaccount_tokens,
        integration_config
    ):
        """
        BR-HAPI-046: Chat should work with mock LLM fallback
        Business Requirement: Mock LLM Fallback for reliable testing
        """
        viewer_token = serviceaccount_tokens.get("test-viewer-sa")
        if not viewer_token:
            pytest.skip("Viewer ServiceAccount token not available")

        holmesgpt_api_client.authenticate_with_bearer_token(viewer_token)

        # Test chat with mock LLM
        chat_data = {
            "message": "Test message for mock LLM",
            "session_id": "mock-llm-test-session",
            "include_context": False
        }

        response = holmesgpt_api_client.post("/api/v1/chat", json_data=chat_data)
        assert response.status_code == 200, "Chat should work with mock LLM"

        chat_result = response.json()

        # Business validation: Mock LLM should provide predictable responses
        assert "response" in chat_result, "Should return chat response"
        assert "session_id" in chat_result, "Should maintain session"

        # Mock responses should be consistent
        chat_response = chat_result["response"]
        assert len(chat_response) > 0, "Mock LLM should provide responses"

    @pytest.mark.slow
    def test_llm_performance_comparison(
        self,
        holmesgpt_api_client,
        serviceaccount_tokens,
        integration_config
    ):
        """
        BR-HAPI-046: Performance comparison between real LLM and mock LLM
        Business Requirement: Performance benchmarks for different LLM types
        """
        admin_token = serviceaccount_tokens.get("test-admin-sa")
        if not admin_token:
            pytest.skip("Admin ServiceAccount token not available")

        holmesgpt_api_client.authenticate_with_bearer_token(admin_token)

        # Simple alert for performance testing
        simple_alert = {
            "alerts": [{
                "status": "firing",
                "labels": {
                    "alertname": "PerformanceTest",
                    "severity": "info"
                },
                "annotations": {
                    "description": "Performance test alert"
                },
                "startsAt": "2024-01-01T00:00:00Z"
            }]
        }

        # Measure response time
        start_time = time.time()
        response = holmesgpt_api_client.post("/api/v1/investigate", json_data=simple_alert)
        end_time = time.time()

        response_time = end_time - start_time

        # Business validation: Response should be within acceptable time
        assert response.status_code == 200, "Investigation should complete successfully"

        if integration_config["use_mock_llm"]:
            # Mock LLM should be very fast
            assert response_time < 5.0, f"Mock LLM should respond quickly, took {response_time:.3f}s"
        else:
            # Real LLM may take longer but should still be reasonable
            assert response_time < 30.0, f"Real LLM should respond within 30s, took {response_time:.3f}s"

    def test_llm_error_handling(
        self,
        holmesgpt_api_client,
        serviceaccount_tokens,
        integration_config
    ):
        """
        BR-HAPI-046: Error handling when LLM services are unavailable
        Business Requirement: Graceful degradation scenarios
        """
        admin_token = serviceaccount_tokens.get("test-admin-sa")
        if not admin_token:
            pytest.skip("Admin ServiceAccount token not available")

        holmesgpt_api_client.authenticate_with_bearer_token(admin_token)

        # Test with invalid chat data that might cause LLM errors
        problematic_chat = {
            "message": "",  # Empty message
            "session_id": "error-test-session"
        }

        response = holmesgpt_api_client.post("/api/v1/chat", json_data=problematic_chat)

        # Business validation: Should handle errors gracefully
        # Even with LLM errors, API should return structured error responses
        assert response.status_code in [400, 422], "Should return validation error for empty message"

        if response.status_code == 422:
            error_data = response.json()
            assert "detail" in error_data, "Should provide error details"

    def test_llm_provider_configuration_validation(self, integration_config):
        """
        BR-HAPI-046: Validate LLM provider configuration
        Business Requirement: Proper LLM provider detection and configuration
        """
        # Validate LLM configuration completeness
        required_llm_config = [
            "llm_endpoint", "llm_model", "llm_provider",
            "use_mock_llm", "llm_available"
        ]

        for config_key in required_llm_config:
            assert config_key in integration_config, f"Missing LLM config: {config_key}"

        # Validate configuration consistency - BR-HAPI-046
        import os
        explicit_provider = os.getenv("LLM_PROVIDER", "auto-detect")

        # Validate graceful degradation behavior (BR-HAPI-046.4)
        if explicit_provider != "auto-detect":
            # When provider is explicitly set, validate graceful degradation
            if integration_config["llm_available"]:
                # LLM available: should use explicit provider
                assert integration_config["llm_provider"] == explicit_provider, \
                    f"Available LLM should use explicit provider {explicit_provider}, got {integration_config['llm_provider']}"
            else:
                # LLM unavailable: should gracefully degrade to mock for reliability
                assert integration_config["llm_provider"] == "mock", \
                    f"Unavailable LLM should gracefully degrade to mock, got {integration_config['llm_provider']}"
                assert integration_config["use_mock_llm"] is True, \
                    "Should use mock LLM when endpoint unavailable"
        elif integration_config["use_mock_llm"] and not integration_config["llm_available"]:
            # Only require mock provider when auto-detecting AND endpoint unavailable
            assert integration_config["llm_provider"] == "mock", \
                "Auto-detected provider should be 'mock' when LLM endpoint unavailable"

        # Validate endpoint format
        endpoint = integration_config["llm_endpoint"]
        assert endpoint.startswith(("http://", "https://", "mock://")), \
            f"LLM endpoint should be valid URL format: {endpoint}"


class TestLLMProviderSpecificFeatures:
    """
    Test LLM provider-specific features and capabilities
    Business Requirement: BR-HAPI-046 - Provider-specific integration patterns
    """

    def test_ollama_specific_features(self, integration_config, holmesgpt_api_client):
        """Test Ollama-specific LLM features if available"""
        if integration_config["llm_provider"] != "ollama":
            pytest.skip("Test requires Ollama provider")

        # Ollama-specific tests would go here
        # For example: model listing, streaming responses, etc.
        pass

    def test_localai_specific_features(self, integration_config, holmesgpt_api_client):
        """Test LocalAI-specific LLM features if available"""
        if integration_config["llm_provider"] != "localai":
            pytest.skip("Test requires LocalAI provider")

        # LocalAI-specific tests would go here
        # For example: model management, embeddings, etc.
        pass

    def test_mock_llm_consistency(self, integration_config, holmesgpt_api_client):
        """Test mock LLM provides consistent responses"""
        if integration_config["llm_provider"] != "mock":
            pytest.skip("Test requires mock provider")

        # Mock LLM should provide deterministic responses for testing
        # This ensures test reliability in CI/CD environments
        pass


# Test fixtures for LLM integration
@pytest.fixture
def sample_llm_test_data():
    """Sample data specifically for LLM testing"""
    return {
        "simple_alert": {
            "alerts": [{
                "status": "firing",
                "labels": {
                    "alertname": "TestAlert",
                    "severity": "warning"
                },
                "annotations": {
                    "description": "Test alert for LLM integration"
                },
                "startsAt": "2024-01-01T00:00:00Z"
            }]
        },
        "complex_alert": {
            "alerts": [{
                "status": "firing",
                "labels": {
                    "alertname": "ComplexMemoryAlert",
                    "severity": "critical",
                    "namespace": "production",
                    "pod": "web-server-123",
                    "container": "nginx"
                },
                "annotations": {
                    "description": "Memory usage is at 95% in production nginx container",
                    "summary": "Critical memory usage detected",
                    "runbook_url": "https://wiki.example.com/memory-alerts"
                },
                "startsAt": "2024-01-01T10:30:00Z"
            }]
        }
    }

@pytest.fixture
def llm_test_scenarios():
    """Different test scenarios for LLM validation"""
    return [
        {
            "name": "kubernetes_question",
            "message": "What are the main differences between Deployments and StatefulSets?",
            "expected_keywords": ["deployment", "statefulset", "kubernetes", "pod"]
        },
        {
            "name": "troubleshooting_question",
            "message": "How do I troubleshoot a pod that keeps crashing?",
            "expected_keywords": ["pod", "crash", "logs", "troubleshoot"]
        },
        {
            "name": "performance_question",
            "message": "My application is running slowly, what should I check?",
            "expected_keywords": ["performance", "slow", "cpu", "memory", "resource"]
        }
    ]

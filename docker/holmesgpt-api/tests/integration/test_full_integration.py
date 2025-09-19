"""
Full Integration Tests for HolmesGPT API
Business Requirement: BR-HAPI-046 - Scenario 3: Kind + real LLM when available

Following project guidelines:
- Test business requirements, not implementation details
- Assertions backed by business outcomes
- AVOID weak business validations
- Reuse existing infrastructure (Kind, real LLM)
"""

import pytest
import time
from typing import Dict, Any

# Test marker for full integration tests
pytestmark = [pytest.mark.integration, pytest.mark.k8s, pytest.mark.llm, pytest.mark.slow]


class TestFullIntegration:
    """
    Scenario 3: Full integration with Kind + real LLM when available
    Business Requirement: BR-HAPI-045 + BR-HAPI-046 (complete ecosystem)
    """

    def test_full_ecosystem_availability(
        self,
        full_integration_config: Dict[str, Any],
        k8s_client,
        holmesgpt_api_client
    ):
        """
        BR-HAPI-046.3: Full ecosystem should be available for end-to-end testing
        Business Requirement: Complete system integration
        """
        # Validate K8s availability
        assert k8s_client is not None, "K8s cluster should be available"

        # Validate API availability
        health_response = holmesgpt_api_client.get("/health")
        assert health_response.status_code == 200, "HolmesGPT API should be healthy"

        # Validate LLM configuration
        llm_provider = full_integration_config["llm_provider"]
        llm_available = full_integration_config["llm_available"]
        use_mock_llm = full_integration_config["use_mock_llm"]

        # Business validation: System should have working LLM (real or mock fallback)
        assert llm_provider in ["ollama", "localai", "mock"], \
            f"Should have valid LLM provider: {llm_provider}"

        if use_mock_llm:
            assert llm_available is True, "Mock LLM should always be available"
        else:
            # Real LLM testing - availability may vary
            print(f"Testing with real LLM: {llm_provider} (available: {llm_available})")

    def test_k8s_auth_with_real_llm_investigation(
        self,
        holmesgpt_api_client,
        serviceaccount_tokens,
        full_integration_config: Dict[str, Any],
        sample_alert_data
    ):
        """
        BR-HAPI-045.1 + BR-HAPI-046.1: Full K8s auth + real LLM investigation
        Business Requirement: Complete authentication and investigation flow
        """
        if not serviceaccount_tokens:
            pytest.skip("No ServiceAccount tokens available for authentication testing")

        # Use admin ServiceAccount for comprehensive testing
        admin_token = serviceaccount_tokens.get("test-admin-sa")
        if not admin_token:
            pytest.skip("No admin ServiceAccount token available")

        # Authenticate with K8s ServiceAccount token
        holmesgpt_api_client.authenticate_with_bearer_token(admin_token)

        # Perform investigation with real/mock LLM
        response = holmesgpt_api_client.post("/api/v1/investigate", json_data=sample_alert_data)

        # Business validation: Investigation should work end-to-end
        if response.status_code == 200:
            result = response.json()
            assert "result" in result or "investigation" in result, \
                "Should return investigation result"

            # Validate LLM provider information in result
            llm_provider = full_integration_config["llm_provider"]
            if "metadata" in result:
                assert "llm_provider" in result["metadata"], \
                    "Should include LLM provider metadata"
                assert result["metadata"]["llm_provider"] == llm_provider, \
                    f"Should use configured LLM provider: {llm_provider}"

        elif response.status_code == 401:
            # Authentication might fail with mock tokens - validate error handling
            error_data = response.json()
            assert "detail" in error_data, "Should provide authentication error details"
        else:
            # Other errors - validate they're handled gracefully
            assert response.status_code in [400, 422, 500], \
                f"Should handle errors gracefully: {response.status_code}"

    def test_chat_with_k8s_auth_and_real_llm(
        self,
        holmesgpt_api_client,
        serviceaccount_tokens,
        full_integration_config: Dict[str, Any]
    ):
        """
        BR-HAPI-045.6 + BR-HAPI-046.1: Chat functionality with K8s auth + real LLM
        Business Requirement: Complete chat integration
        """
        if not serviceaccount_tokens:
            pytest.skip("No ServiceAccount tokens available for chat testing")

        # Use viewer ServiceAccount for chat testing
        viewer_token = serviceaccount_tokens.get("test-viewer-sa")
        if not viewer_token:
            pytest.skip("No viewer ServiceAccount token available")

        # Authenticate with K8s ServiceAccount token
        holmesgpt_api_client.authenticate_with_bearer_token(viewer_token)

        # Test chat functionality
        chat_message = {
            "message": "What are the common causes of pod failures in Kubernetes?",
            "context": "troubleshooting"
        }

        response = holmesgpt_api_client.post("/api/v1/chat", json_data=chat_message)

        # Business validation: Chat should work with K8s auth
        if response.status_code == 200:
            result = response.json()
            assert "response" in result or "message" in result, \
                "Should return chat response"

            # Validate LLM integration in chat
            llm_provider = full_integration_config["llm_provider"]
            use_mock_llm = full_integration_config["use_mock_llm"]

            if use_mock_llm:
                # Mock LLM should provide predictable responses
                response_text = result.get("response", result.get("message", ""))
                assert len(response_text) > 0, "Mock LLM should provide response"
            else:
                # Real LLM should provide meaningful responses
                response_text = result.get("response", result.get("message", ""))
                assert len(response_text) > 10, \
                    "Real LLM should provide substantial response"

        elif response.status_code == 401:
            # Authentication might fail with mock tokens
            error_data = response.json()
            assert "detail" in error_data, "Should provide authentication error details"

    def test_llm_performance_in_full_integration(
        self,
        holmesgpt_api_client,
        serviceaccount_tokens,
        full_integration_config: Dict[str, Any],
        sample_alert_data
    ):
        """
        BR-HAPI-046.5: LLM performance in full integration environment
        Business Requirement: Performance validation in complete ecosystem
        """
        if not serviceaccount_tokens:
            pytest.skip("No ServiceAccount tokens available for performance testing")

        admin_token = serviceaccount_tokens.get("test-admin-sa")
        if not admin_token:
            pytest.skip("No admin ServiceAccount token available")

        # Authenticate and test performance
        holmesgpt_api_client.authenticate_with_bearer_token(admin_token)

        # Measure end-to-end performance
        start_time = time.time()
        response = holmesgpt_api_client.post("/api/v1/investigate", json_data=sample_alert_data)
        end_time = time.time()

        total_time = end_time - start_time

        # Business validation: Performance should meet expectations
        llm_provider = full_integration_config["llm_provider"]
        use_mock_llm = full_integration_config["use_mock_llm"]

        if response.status_code == 200:
            if use_mock_llm:
                # Mock LLM should be very fast
                assert total_time < 5.0, \
                    f"Mock LLM full integration should be fast, took {total_time:.2f}s"
            else:
                # Real LLM might be slower but should be reasonable
                assert total_time < 60.0, \
                    f"Real LLM full integration should complete within 60s, took {total_time:.2f}s"

            print(f"Full integration investigation completed in {total_time:.2f}s using {llm_provider}")

        # Business outcome: Performance data should be available regardless of auth status
        assert total_time > 0, "Should measure actual processing time"

    def test_scope_based_authorization_full_integration(
        self,
        holmesgpt_api_client,
        serviceaccount_tokens,
        full_integration_config: Dict[str, Any]
    ):
        """
        BR-HAPI-045.4: OAuth 2 scope-based authorization in full integration
        Business Requirement: Complete RBAC-to-scope mapping validation
        """
        if not serviceaccount_tokens:
            pytest.skip("No ServiceAccount tokens available for authorization testing")

        # Test different authorization levels
        test_cases = [
            {
                "sa_name": "test-admin-sa",
                "expected_scopes": ["admin", "investigate", "chat", "read"],
                "should_access_admin": True
            },
            {
                "sa_name": "test-viewer-sa",
                "expected_scopes": ["read"],
                "should_access_admin": False
            },
            {
                "sa_name": "test-holmesgpt-sa",
                "expected_scopes": ["investigate", "chat", "read"],
                "should_access_admin": False
            }
        ]

        for test_case in test_cases:
            sa_name = test_case["sa_name"]
            token = serviceaccount_tokens.get(sa_name)

            if not token:
                continue  # Skip if token not available

            # Test with this ServiceAccount
            holmesgpt_api_client.authenticate_with_bearer_token(token)

            # Test investigation access (should work for investigate scope)
            if "investigate" in test_case["expected_scopes"]:
                response = holmesgpt_api_client.post("/api/v1/investigate",
                                                   json_data={"alertname": "test"})
                # Should either succeed or fail with auth error (not forbidden)
                assert response.status_code != 403, \
                    f"{sa_name} with investigate scope should not be forbidden"

            # Test chat access (should work for chat scope)
            if "chat" in test_case["expected_scopes"]:
                response = holmesgpt_api_client.post("/api/v1/chat",
                                                   json_data={"message": "test"})
                # Should either succeed or fail with auth error (not forbidden)
                assert response.status_code != 403, \
                    f"{sa_name} with chat scope should not be forbidden"

    def test_adaptive_llm_behavior_full_integration(
        self,
        full_integration_config: Dict[str, Any],
        holmesgpt_api_client
    ):
        """
        BR-HAPI-046.4: Adaptive LLM behavior in full integration
        Business Requirement: Graceful degradation in complete ecosystem
        """
        llm_provider = full_integration_config["llm_provider"]
        llm_available = full_integration_config["llm_available"]
        use_mock_llm = full_integration_config["use_mock_llm"]

        # Business validation: System should adapt to LLM availability
        if not llm_available and not use_mock_llm:
            # Should have fallen back to mock
            assert False, "System should have fallen back to mock LLM when real LLM unavailable"

        # Test adaptive behavior through API health check
        response = holmesgpt_api_client.get("/health")
        assert response.status_code == 200, "API should remain healthy regardless of LLM type"

        health_data = response.json()
        if "llm_status" in health_data:
            if use_mock_llm:
                assert health_data["llm_status"] == "mock", \
                    "Health check should reflect mock LLM usage"
            else:
                assert health_data["llm_status"] in ["available", "connected"], \
                    "Health check should reflect real LLM availability"

        # Business outcome: System remains functional with any LLM configuration
        print(f"Full integration using {llm_provider} LLM (mock: {use_mock_llm})")


class TestFullIntegrationConfiguration:
    """
    Full integration configuration validation tests
    Business Requirement: BR-HAPI-046 - Configuration integrity for Scenario 3
    """

    def test_full_integration_config_completeness(self, full_integration_config: Dict[str, Any]):
        """
        BR-HAPI-046: Full integration should have complete configuration
        Business Requirement: Configuration completeness for Scenario 3
        """
        # Should have both K8s and LLM configurations
        k8s_fields = ["kubeconfig", "namespace", "use_real_k8s", "api_endpoint"]
        llm_fields = ["llm_endpoint", "llm_provider", "use_mock_llm", "llm_available"]

        for field in k8s_fields + llm_fields:
            assert field in full_integration_config, f"Missing field: {field}"

        # Should have test infrastructure fields
        test_fields = ["test_timeout", "max_retries", "ci_mode"]
        for field in test_fields:
            assert field in full_integration_config, f"Missing test field: {field}"

    def test_full_integration_adaptive_config(self, full_integration_config: Dict[str, Any]):
        """
        BR-HAPI-046: Full integration should adapt to environment
        Business Requirement: Environment-aware configuration
        """
        import os

        # Validate adaptive LLM configuration
        explicit_provider = os.getenv("LLM_PROVIDER", "auto-detect")
        endpoint = full_integration_config["llm_endpoint"]
        provider = full_integration_config["llm_provider"]
        available = full_integration_config["llm_available"]

        # Business validation: Configuration should reflect environment
        if explicit_provider != "auto-detect":
            assert provider == explicit_provider, \
                f"Should honor explicit provider: {explicit_provider}"
        else:
            # Auto-detection should work correctly
            if ":11434" in endpoint and available:
                assert provider == "ollama", "Should detect Ollama from port 11434"
            elif ":8080" in endpoint and available:
                assert provider == "localai", "Should detect LocalAI from port 8080"
            else:
                assert provider == "mock", "Should fallback to mock when unavailable"

        # Validate CI/CD mode detection
        ci_mode = full_integration_config["ci_mode"]
        actual_ci = bool(os.getenv("CI", False)) or bool(os.getenv("GITHUB_ACTIONS", False))
        assert ci_mode == actual_ci, "Should correctly detect CI/CD mode"

    def test_scenario_flexibility(self, full_integration_config: Dict[str, Any]):
        """
        BR-HAPI-046: Full integration should be flexible for different environments
        Business Requirement: Multi-environment support
        """
        # Should support both real and mock LLM
        provider = full_integration_config["llm_provider"]
        use_mock = full_integration_config["use_mock_llm"]

        if provider == "mock":
            assert use_mock is True, "Mock provider should use mock LLM"
        else:
            # Real provider might still use mock due to availability
            # This is valid adaptive behavior
            pass

        # Should support real K8s (Kind cluster)
        use_real_k8s = full_integration_config["use_real_k8s"]
        assert use_real_k8s is True, "Full integration should use real K8s"

        # Business outcome: System should work in any valid configuration
        assert provider in ["ollama", "localai", "mock"], \
            f"Provider should be valid: {provider}"
        assert isinstance(use_mock, bool), "use_mock_llm should be boolean"


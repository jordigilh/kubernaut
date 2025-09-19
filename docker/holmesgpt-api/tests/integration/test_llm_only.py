"""
LLM-Only Integration Tests for HolmesGPT API
Business Requirement: BR-HAPI-046 - Scenario 1: Pure LLM testing without K8s dependencies

Following project guidelines:
- Test business requirements, not implementation details
- Assertions backed by business outcomes
- AVOID weak business validations
"""

import pytest
import os
from typing import Dict, Any

# Test marker for LLM-only tests
pytestmark = [pytest.mark.integration, pytest.mark.llm_only]


class TestLLMOnlyIntegration:
    """
    Scenario 1: LLM integration without Kubernetes dependencies
    Business Requirement: BR-HAPI-046.1, BR-HAPI-046.2, BR-HAPI-046.3
    """

    def test_llm_provider_detection_without_k8s(self, llm_only_config: Dict[str, Any]):
        """
        BR-HAPI-046.1: LLM provider auto-detection should work independently of K8s
        Business Requirement: Independent LLM functionality
        """
        # Validate LLM-only configuration structure
        assert llm_only_config["scenario"] == "llm_only", "Should be LLM-only scenario"
        assert "llm_provider" in llm_only_config, "Should have LLM provider"
        assert "llm_endpoint" in llm_only_config, "Should have LLM endpoint"
        assert "llm_available" in llm_only_config, "Should have availability status"

        # Business validation: Provider should be correctly determined with graceful degradation
        explicit_provider = os.getenv("LLM_PROVIDER", "auto-detect")
        if explicit_provider != "auto-detect":
            # When provider is explicitly set, validate graceful degradation behavior
            if llm_only_config["llm_available"]:
                # LLM available: should use explicit provider
                assert llm_only_config["llm_provider"] == explicit_provider, \
                    f"Available LLM should use explicit provider {explicit_provider}"
            else:
                # LLM unavailable: should gracefully degrade to mock (BR-HAPI-046.4)
                assert llm_only_config["llm_provider"] == "mock", \
                    f"Unavailable LLM should gracefully degrade to mock for reliability"
                print(f"✅ LLM-only graceful degradation: {explicit_provider} → mock (endpoint unavailable)")
        else:
            # Auto-detection should work without K8s, unless forced to mock
            endpoint = llm_only_config["llm_endpoint"]
            provider = llm_only_config["llm_provider"]
            available = llm_only_config["llm_available"]
            use_mock = llm_only_config["use_mock_llm"]

            if use_mock:
                # When forced to use mock LLM, provider should be mock regardless of endpoint
                assert provider == "mock", f"Forced mock LLM should use mock provider"
            elif ":11434" in endpoint and available:
                assert provider == "ollama", f"Ollama endpoint should use ollama provider"
            elif ":8080" in endpoint and available:
                assert provider == "localai", f"LocalAI endpoint should use localai provider"
            else:
                assert provider == "mock", f"Unavailable endpoint should fallback to mock"

    def test_llm_connectivity_without_k8s(self, llm_only_client):
        """
        BR-HAPI-046.2: LLM connectivity should be testable without K8s
        Business Requirement: Independent LLM connectivity validation
        """
        # Test LLM connectivity independently
        connectivity = llm_only_client.test_llm_connectivity()

        # Business validation: Connectivity should match configuration expectations
        config = llm_only_client.config
        if config["use_mock_llm"]:
            assert connectivity is True, "Mock LLM should always be available"
        else:
            # For real LLM, test actual connectivity
            # Business outcome: Either connects or gracefully handles failure
            assert isinstance(connectivity, bool), "Should return boolean connectivity status"

    def test_llm_investigation_simulation_without_k8s(self, llm_only_client):
        """
        BR-HAPI-046.3: LLM investigation should work without K8s authentication
        Business Requirement: Independent LLM investigation capability
        """
        # Sample alert data for investigation
        alert_data = {
            "alertname": "PodCrashLooping",
            "namespace": "test-ns",
            "pod": "test-pod",
            "message": "Container test-container has restarted 5 times"
        }

        # Perform LLM investigation simulation
        result = llm_only_client.simulate_investigation(alert_data)

        # Business validations: Investigation should provide meaningful results
        assert "status" in result, "Investigation should return status"
        assert result["status"] == "success", "Investigation should succeed"
        assert "result" in result, "Investigation should return analysis result"
        assert "provider" in result, "Investigation should identify LLM provider"
        assert "model" in result, "Investigation should identify LLM model"

        # Validate provider consistency
        config = llm_only_client.config
        assert result["provider"] == config["llm_provider"], \
            f"Result provider {result['provider']} should match config {config['llm_provider']}"
        assert result["model"] == config["llm_model"], \
            f"Result model {result['model']} should match config {config['llm_model']}"

        # Business outcome validation: Result should contain analysis
        if config["use_mock_llm"]:
            assert "Mock investigation result" in result["result"], \
                "Mock LLM should provide mock investigation"
            assert result.get("real_llm_used", False) is False, \
                "Mock LLM should not claim real LLM usage"
        else:
            # Real LLM attempt - check if it actually connected or fell back
            if result.get("real_llm_used", False):
                assert "response_time" in result, \
                    "Real LLM should report response time"
                assert result["response_time"] > 0, \
                    "Real LLM should have measurable response time"
                print(f"✅ Real LLM interaction successful: {result['response_time']:.2f}s")
            else:
                # Fell back to mock due to connection failure
                assert "fallback_reason" in result, \
                    "LLM fallback should provide reason"
                assert "unreachable" in result["result"].lower() or "fallback" in result["result"].lower(), \
                    "Fallback should indicate connection issue"
                print(f"⚠️ LLM fallback due to: {result.get('fallback_reason', 'unknown')}")
            
            # Either way, should have attempted real LLM first
            assert result.get("provider") in ["ollama", "localai", "mock"], \
                f"Provider should be valid LLM type: {result.get('provider')}"

    def test_llm_provider_fallback_without_k8s(self, llm_only_config: Dict[str, Any]):
        """
        BR-HAPI-046.4: LLM fallback logic should work without K8s
        Business Requirement: Graceful degradation for unavailable LLM
        """
        # Test fallback behavior when LLM is unavailable
        endpoint = llm_only_config["llm_endpoint"]
        provider = llm_only_config["llm_provider"]
        available = llm_only_config["llm_available"]
        use_mock = llm_only_config["use_mock_llm"]

        # Business validation: Fallback should ensure system remains functional
        explicit_provider = os.getenv("LLM_PROVIDER", "auto-detect")
        use_mock_env = os.getenv("USE_MOCK_LLM", "false").lower() in ('true', '1', 'yes', 'on')

        if not available and explicit_provider == "auto-detect" and not use_mock_env:
            # When endpoint unavailable and auto-detecting, should fallback to mock
            assert use_mock is True, "Should fallback to mock when LLM unavailable"
            assert provider == "mock", "Provider should be set to mock for fallback"

        # Business outcome: System should always have a working LLM (real or mock)
        assert provider in ["ollama", "localai", "mock"], \
            f"Provider should be valid LLM type: {provider}"

    @pytest.mark.slow
    def test_llm_performance_without_k8s(self, llm_only_client):
        """
        BR-HAPI-046.5: LLM performance should be measurable without K8s
        Business Requirement: Performance validation independent of K8s
        """
        import time

        alert_data = {
            "alertname": "HighCPUUsage",
            "namespace": "production",
            "pod": "web-server",
            "message": "CPU usage above 90% for 5 minutes"
        }

        # Measure LLM investigation performance
        start_time = time.time()
        result = llm_only_client.simulate_investigation(alert_data)
        end_time = time.time()

        investigation_time = end_time - start_time

        # Business validations: Performance should meet expectations
        assert result["status"] == "success", "Investigation should complete successfully"
        
        config = llm_only_client.config
        if config["use_mock_llm"]:
            # Mock LLM should be very fast
            assert investigation_time < 1.0, \
                f"Mock LLM should respond quickly, took {investigation_time:.2f}s"
            assert result.get("real_llm_used", False) is False, \
                "Mock LLM should not claim real LLM usage"
        else:
            # Check if real LLM was used or fell back
            if result.get("real_llm_used", False):
                # Real LLM interaction should be measurable and reasonable
                llm_response_time = result.get("response_time", 0)
                assert llm_response_time > 0.1, \
                    f"Real LLM should have measurable response time, got {llm_response_time:.2f}s"
                assert llm_response_time < 60.0, \
                    f"Real LLM should respond within 60s, took {llm_response_time:.2f}s"
                print(f"✅ Real LLM performance: {llm_response_time:.2f}s (total: {investigation_time:.2f}s)")
            else:
                # Fell back to mock - should be fast
                assert investigation_time < 35.0, \
                    f"LLM fallback should complete within 35s (including timeout), took {investigation_time:.2f}s"
                print(f"⚠️ LLM fallback performance: {investigation_time:.2f}s (reason: {result.get('fallback_reason', 'unknown')})")
        
        # Business outcome: Performance metrics should be available
        provider_used = result.get("provider", config["llm_provider"])
        real_llm_indicator = "✅ Real" if result.get("real_llm_used", False) else "🔄 Fallback"
        print(f"{real_llm_indicator} LLM investigation completed in {investigation_time:.2f}s using {provider_used}")


class TestLLMOnlyConfiguration:
    """
    LLM-only configuration validation tests
    Business Requirement: BR-HAPI-046 - Configuration integrity
    """

    def test_llm_only_config_structure(self, llm_only_config: Dict[str, Any]):
        """
        BR-HAPI-046: LLM-only configuration should have required fields
        Business Requirement: Configuration completeness
        """
        required_fields = [
            "llm_endpoint", "llm_model", "llm_provider",
            "use_mock_llm", "llm_available", "test_timeout",
            "log_level", "scenario"
        ]

        # Validate all required fields are present
        for field in required_fields:
            assert field in llm_only_config, f"Missing required field: {field}"

        # Validate scenario identifier
        assert llm_only_config["scenario"] == "llm_only", \
            "Should be identified as LLM-only scenario"

        # Business validation: Configuration should not include K8s fields
        k8s_fields = ["kubeconfig", "namespace", "use_real_k8s"]
        for field in k8s_fields:
            assert field not in llm_only_config, \
                f"LLM-only config should not include K8s field: {field}"

    def test_llm_only_config_values(self, llm_only_config: Dict[str, Any]):
        """
        BR-HAPI-046: LLM-only configuration values should be valid
        Business Requirement: Configuration validity
        """
        # Validate endpoint format
        endpoint = llm_only_config["llm_endpoint"]
        assert endpoint.startswith(("http://", "https://", "mock://")), \
            f"LLM endpoint should be valid URL: {endpoint}"

        # Validate provider is known type
        provider = llm_only_config["llm_provider"]
        assert provider in ["ollama", "localai", "mock"], \
            f"Provider should be valid LLM type: {provider}"

        # Validate boolean fields
        assert isinstance(llm_only_config["use_mock_llm"], bool), \
            "use_mock_llm should be boolean"
        assert isinstance(llm_only_config["llm_available"], bool), \
            "llm_available should be boolean"

        # Validate timeout is reasonable
        timeout = llm_only_config["test_timeout"]
        assert isinstance(timeout, int) and timeout > 0, \
            f"test_timeout should be positive integer: {timeout}"
        assert timeout <= 300, \
            f"test_timeout should be reasonable (<= 300s): {timeout}"

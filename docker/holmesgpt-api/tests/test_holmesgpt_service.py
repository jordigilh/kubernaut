"""
HolmesGPT Service Tests - Business Requirements BR-HAPI-026 through BR-HAPI-035
Following TDD principles: Test business requirements, not implementation
"""

import pytest
import asyncio
from unittest.mock import Mock, AsyncMock, patch
from datetime import datetime
from typing import Dict, Any

from services.holmesgpt_service import HolmesGPTService
from models.api_models import InvestigateResponse, ChatResponse, Recommendation, Priority, Toolset, Model
from config import Settings


class TestHolmesGPTService:
    """Test HolmesGPT Service following business requirements"""

    @pytest.fixture
    def holmesgpt_service(self, test_settings: Settings) -> HolmesGPTService:
        """Create HolmesGPT service instance for testing"""
        return HolmesGPTService(test_settings)

    @pytest.mark.asyncio
    async def test_service_initialization_succeeds_with_valid_config(
        self,
        holmesgpt_service: HolmesGPTService
    ):
        """
        BR-HAPI-026, BR-HAPI-030: Service must initialize successfully with valid configuration
        Business Requirement: Reliable service startup with proper configuration
        """
        # Business requirement: Service should initialize successfully
        result = await holmesgpt_service.initialize()

        # Business validation: Initialization should succeed
        assert result == True, "Service initialization should succeed with valid config"

        # Business validation: Service should be marked as initialized
        assert holmesgpt_service._initialized == True, "Service should be marked as initialized"

    @pytest.mark.asyncio
    async def test_service_health_check_reports_accurate_status(
        self,
        holmesgpt_service: HolmesGPTService
    ):
        """
        BR-HAPI-019: Service must provide accurate health status
        Business Requirement: Reliable health monitoring for system reliability
        """
        # Initialize service first
        await holmesgpt_service.initialize()

        # Business requirement: Health check should succeed for initialized service
        health_status = await holmesgpt_service.health_check()

        # Business validation: Initialized service should be healthy
        assert health_status == True, "Initialized service should report healthy status"

    @pytest.mark.asyncio
    async def test_service_health_check_fails_when_not_initialized(
        self,
        holmesgpt_service: HolmesGPTService
    ):
        """
        BR-HAPI-019: Service must report unhealthy when not properly initialized
        Business Requirement: Accurate health reporting prevents traffic to unready services
        """
        # Business requirement: Uninitialized service should report unhealthy
        health_status = await holmesgpt_service.health_check()

        # Business validation: Uninitialized service should be unhealthy
        assert health_status == False, "Uninitialized service should report unhealthy status"

    @pytest.mark.asyncio
    async def test_investigate_alert_processes_request_and_returns_structured_response(
        self,
        holmesgpt_service: HolmesGPTService
    ):
        """
        BR-HAPI-001, BR-HAPI-004: Service must process alert investigations and return structured responses
        Business Requirement: Core investigation functionality with actionable recommendations
        """
        await holmesgpt_service.initialize()

        # Business test data
        alert_name = "PodCrashLooping"
        namespace = "production"
        context = {"cluster": "prod-cluster", "team": "platform"}
        priority = Priority.HIGH

        # Business requirement: Investigation should process successfully
        result = await holmesgpt_service.investigate_alert(
            alert_name=alert_name,
            namespace=namespace,
            context=context,
            priority=priority,
            async_mode=False
        )

        # Business validation: Response should be properly structured
        assert isinstance(result, InvestigateResponse), "Should return InvestigateResponse object"
        assert result.alert_name == alert_name, "Response should include original alert name"
        assert result.namespace == namespace, "Response should include original namespace"
        assert result.status == "completed", "Investigation should complete successfully"

        # Business validation: Should provide actionable recommendations
        assert isinstance(result.recommendations, list), "Should provide list of recommendations"
        assert len(result.recommendations) > 0, "Should provide at least one recommendation"

        # Business validation: Each recommendation should be actionable
        for recommendation in result.recommendations:
            assert isinstance(recommendation, Recommendation), "Each recommendation should be properly structured"
            assert len(recommendation.title) > 0, "Recommendation should have meaningful title"
            assert len(recommendation.description) > 0, "Recommendation should have meaningful description"
            assert recommendation.confidence >= 0.0, "Confidence should be non-negative"
            assert recommendation.confidence <= 1.0, "Confidence should not exceed 1.0"

    @pytest.mark.asyncio
    async def test_investigate_alert_handles_different_priority_levels(
        self,
        holmesgpt_service: HolmesGPTService
    ):
        """
        BR-HAPI-003: Service must handle different investigation priority levels
        Business Requirement: Priority-based investigation processing for resource management
        """
        await holmesgpt_service.initialize()

        # Test all priority levels
        priorities = [Priority.LOW, Priority.MEDIUM, Priority.HIGH, Priority.CRITICAL]

        for priority in priorities:
            # Business requirement: Each priority level should be accepted
            result = await holmesgpt_service.investigate_alert(
                alert_name="TestAlert",
                namespace="test",
                context={},
                priority=priority,
                async_mode=False
            )

            # Business validation: Investigation should succeed for all priorities
            assert isinstance(result, InvestigateResponse), f"Priority {priority} should be processed"
            assert result.status == "completed", f"Priority {priority} investigation should complete"

    @pytest.mark.asyncio
    async def test_investigate_alert_supports_async_processing(
        self,
        holmesgpt_service: HolmesGPTService
    ):
        """
        BR-HAPI-005: Service must support asynchronous investigation processing
        Business Requirement: Non-blocking investigations for better system performance
        """
        await holmesgpt_service.initialize()

        # Measure processing time for async vs sync
        import time

        # Test synchronous processing
        start_time = time.time()
        sync_result = await holmesgpt_service.investigate_alert(
            alert_name="SyncTest",
            namespace="test",
            context={},
            priority=Priority.HIGH,
            async_mode=False
        )
        sync_duration = time.time() - start_time

        # Test asynchronous processing
        start_time = time.time()
        async_result = await holmesgpt_service.investigate_alert(
            alert_name="AsyncTest",
            namespace="test",
            context={},
            priority=Priority.HIGH,
            async_mode=True
        )
        async_duration = time.time() - start_time

        # Business requirement: Both modes should work
        assert isinstance(sync_result, InvestigateResponse), "Sync investigation should work"
        assert isinstance(async_result, InvestigateResponse), "Async investigation should work"

        # Business validation: Async should be faster (in this simulated case)
        assert async_duration < sync_duration, "Async processing should be faster than sync"

    @pytest.mark.asyncio
    async def test_process_chat_maintains_session_context(
        self,
        holmesgpt_service: HolmesGPTService
    ):
        """
        BR-HAPI-006, BR-HAPI-010: Service must process chat messages and maintain session context
        Business Requirement: Stateful conversation for effective troubleshooting assistance
        """
        await holmesgpt_service.initialize()

        session_id = "test-session-123"

        # First message in session
        first_response = await holmesgpt_service.process_chat(
            message="My pod is crashing, can you help?",
            session_id=session_id,
            context={"namespace": "production"},
            stream=False
        )

        # Business validation: First response should be structured
        assert isinstance(first_response, ChatResponse), "Should return ChatResponse object"
        assert first_response.session_id == session_id, "Should maintain session ID"
        assert len(first_response.response) > 0, "Should provide meaningful response"

        # Second message in same session
        second_response = await holmesgpt_service.process_chat(
            message="The pod logs show memory errors",
            session_id=session_id,
            context={"namespace": "production"},
            stream=False
        )

        # Business validation: Session should be maintained
        assert isinstance(second_response, ChatResponse), "Should return ChatResponse object"
        assert second_response.session_id == session_id, "Should maintain same session ID"

        # Business validation: Service should track session history
        assert session_id in holmesgpt_service._chat_sessions, "Session should be tracked"
        session_history = holmesgpt_service._chat_sessions[session_id]["history"]
        assert len(session_history) >= 4, "Should track both user and assistant messages"

    @pytest.mark.asyncio
    async def test_process_chat_includes_context_when_provided(
        self,
        holmesgpt_service: HolmesGPTService
    ):
        """
        BR-HAPI-011: Service must utilize provided context in chat responses
        Business Requirement: Context-aware chat for more accurate troubleshooting
        """
        await holmesgpt_service.initialize()

        # Rich context for chat
        context = {
            "namespace": "production",
            "cluster_info": {"node_count": 5, "version": "1.25"},
            "recent_alerts": ["PodCrashLooping", "HighMemoryUsage"],
            "metrics": {"cpu_usage": "85%", "memory_usage": "78%"}
        }

        # Business requirement: Chat should utilize provided context
        result = await holmesgpt_service.process_chat(
            message="What's wrong with my application?",
            session_id="context-test",
            context=context,
            stream=False
        )

        # Business validation: Context should be reflected in response
        assert isinstance(result, ChatResponse), "Should return ChatResponse object"
        assert result.context_used == context, "Should indicate context was used"

        # Business validation: Response should be contextual
        assert len(result.response) > 0, "Should provide contextual response"

    @pytest.mark.asyncio
    async def test_process_chat_provides_helpful_suggestions(
        self,
        holmesgpt_service: HolmesGPTService
    ):
        """
        BR-HAPI-008: Service must provide helpful follow-up suggestions
        Business Requirement: Guided troubleshooting with actionable next steps
        """
        await holmesgpt_service.initialize()

        # Business requirement: Chat should provide suggestions
        result = await holmesgpt_service.process_chat(
            message="My application is running slowly",
            session_id="suggestions-test",
            context=None,
            stream=False
        )

        # Business validation: Should include suggestions
        assert isinstance(result.suggestions, list), "Should provide list of suggestions"
        assert len(result.suggestions) > 0, "Should provide at least one suggestion"

        # Business validation: Suggestions should be actionable
        for suggestion in result.suggestions:
            assert isinstance(suggestion, str), "Each suggestion should be a string"
            assert len(suggestion) > 10, "Suggestions should be descriptive"

    @pytest.mark.asyncio
    async def test_get_capabilities_returns_available_features(
        self,
        holmesgpt_service: HolmesGPTService
    ):
        """
        BR-HAPI-020: Service must report its capabilities
        Business Requirement: Service discovery and capability advertisement
        """
        await holmesgpt_service.initialize()

        # Business requirement: Service should report capabilities
        capabilities = await holmesgpt_service.get_capabilities()

        # Business validation: Should return list of capabilities
        assert isinstance(capabilities, list), "Capabilities should be a list"
        assert len(capabilities) > 0, "Should report at least one capability"

        # Business validation: Should include core capabilities
        expected_capabilities = ["alert_investigation", "interactive_chat"]
        for capability in expected_capabilities:
            assert capability in capabilities, f"Should include {capability} capability"

    @pytest.mark.asyncio
    async def test_get_configuration_returns_current_settings(
        self,
        holmesgpt_service: HolmesGPTService
    ):
        """
        BR-HAPI-021: Service must provide current configuration information
        Business Requirement: Configuration transparency for monitoring and debugging
        """
        await holmesgpt_service.initialize()

        # Business requirement: Service should report configuration
        config = await holmesgpt_service.get_configuration()

        # Business validation: Should return configuration dictionary
        assert isinstance(config, dict), "Configuration should be a dictionary"

        # Business validation: Should include essential configuration
        expected_config_keys = ["llm_provider", "llm_model", "available_toolsets"]
        for key in expected_config_keys:
            assert key in config, f"Configuration should include {key}"

    @pytest.mark.asyncio
    async def test_get_available_toolsets_returns_enabled_toolsets(
        self,
        holmesgpt_service: HolmesGPTService
    ):
        """
        BR-HAPI-022, BR-HAPI-033: Service must provide available toolsets
        Business Requirement: Toolset discovery for investigation capabilities
        """
        await holmesgpt_service.initialize()

        # Business requirement: Service should report available toolsets
        toolsets = await holmesgpt_service.get_available_toolsets()

        # Business validation: Should return list of toolsets
        assert isinstance(toolsets, list), "Toolsets should be a list"

        # Business validation: Each toolset should be properly structured
        for toolset in toolsets:
            assert isinstance(toolset, Toolset), "Each toolset should be a Toolset object"
            assert len(toolset.name) > 0, "Toolset should have name"
            assert len(toolset.description) > 0, "Toolset should have description"
            assert isinstance(toolset.capabilities, list), "Toolset should have capabilities list"

    @pytest.mark.asyncio
    async def test_get_supported_models_returns_llm_models(
        self,
        holmesgpt_service: HolmesGPTService
    ):
        """
        BR-HAPI-023: Service must provide supported LLM models
        Business Requirement: Model discovery for configuration and monitoring
        """
        await holmesgpt_service.initialize()

        # Business requirement: Service should report supported models
        models = await holmesgpt_service.get_supported_models()

        # Business validation: Should return list of models
        assert isinstance(models, list), "Models should be a list"

        # Business validation: Each model should be properly structured
        for model in models:
            assert isinstance(model, Model), "Each model should be a Model object"
            assert len(model.name) > 0, "Model should have name"
            assert len(model.provider) > 0, "Model should have provider"
            assert isinstance(model.available, bool), "Model should have availability status"

    @pytest.mark.asyncio
    async def test_service_cleanup_releases_resources(
        self,
        holmesgpt_service: HolmesGPTService
    ):
        """
        BR-HAPI-040: Service must properly cleanup resources on shutdown
        Business Requirement: Graceful shutdown prevents resource leaks
        """
        await holmesgpt_service.initialize()

        # Add some session data
        await holmesgpt_service.process_chat(
            message="Test message",
            session_id="cleanup-test",
            context=None,
            stream=False
        )

        # Verify session exists
        assert "cleanup-test" in holmesgpt_service._chat_sessions, "Session should exist before cleanup"

        # Business requirement: Cleanup should succeed
        await holmesgpt_service.cleanup()

        # Business validation: Resources should be released
        assert len(holmesgpt_service._chat_sessions) == 0, "Chat sessions should be cleared"
        assert holmesgpt_service._initialized == False, "Service should be marked as uninitialized"

    @pytest.mark.asyncio
    async def test_service_handles_errors_gracefully(
        self,
        holmesgpt_service: HolmesGPTService
    ):
        """
        BR-HAPI-029: Service must handle errors gracefully with retry logic
        Business Requirement: Robust error handling for reliable service operation
        """
        await holmesgpt_service.initialize()

        # Simulate error conditions and verify graceful handling
        with patch.object(holmesgpt_service, '_generate_recommendations', side_effect=Exception("Test error")):
            # Business requirement: Service should handle internal errors gracefully
            with pytest.raises(Exception):
                await holmesgpt_service.investigate_alert(
                    alert_name="ErrorTest",
                    namespace="test",
                    context={},
                    priority=Priority.LOW,
                    async_mode=False
                )

        # Business validation: Service should still be operational after error
        health_status = await holmesgpt_service.health_check()
        assert health_status == True, "Service should remain healthy after handling errors"



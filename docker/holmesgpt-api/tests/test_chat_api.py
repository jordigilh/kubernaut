"""
Chat API Tests - Business Requirements BR-HAPI-006 through BR-HAPI-010
Following TDD principles: Test business requirements, not implementation
"""

import pytest
from datetime import datetime
from unittest.mock import AsyncMock
from fastapi.testclient import TestClient
from typing import Dict, Any

from models.api_models import ChatResponse


class TestChatAPI:
    """Test Chat API endpoints following business requirements"""

    def test_chat_endpoint_exists_and_accepts_post_requests(self, test_client: TestClient):
        """
        BR-HAPI-006: Chat endpoint must exist and accept POST requests
        Business Requirement: API must provide interactive chat capability
        """
        # Test that endpoint exists (will return 422 for missing data, not 404)
        response = test_client.post("/api/v1/chat")

        # Should not be 404 (endpoint exists), should be 403 (authentication required)
        assert response.status_code != 404, "Chat endpoint should exist"
        assert response.status_code == 403, "Should require authentication before validation"

    def test_chat_requires_authentication(self, test_client: TestClient, sample_chat_data: Dict[str, Any]):
        """
        BR-HAPI-007: Chat endpoint must require authentication
        Business Requirement: Secure access to chat capabilities
        """
        # Attempt chat without authentication
        response = test_client.post("/api/v1/chat", json=sample_chat_data)

        # Should require authentication
        assert response.status_code == 401 or response.status_code == 403, \
            "Chat endpoint should require authentication"

    def test_chat_processes_user_messages(
        self,
        test_client: TestClient,
        sample_chat_data: Dict[str, Any],
        operator_token: str,
        mock_holmesgpt_service
    ):
        """
        BR-HAPI-006, BR-HAPI-008: Chat must process user messages and return AI responses
        Business Requirement: Interactive conversation with AI troubleshooting assistant
        """
        headers = {"Authorization": f"Bearer {operator_token}"}

        # Configure mock to return expected chat response
        mock_response = ChatResponse(
            response="I can help you investigate the pod crashing issue. Let me analyze the frontend namespace for potential causes.",
            session_id=sample_chat_data["session_id"],
            context_used={"namespace": "production", "cluster_status": "healthy"},
            suggestions=["Check pod resource limits", "Review recent deployments"],
            timestamp=datetime.utcnow()
        )

        mock_holmesgpt_service.process_chat.return_value = mock_response

        # Send chat message
        response = test_client.post("/api/v1/chat", json=sample_chat_data, headers=headers)

        # Business requirement: Chat should process successfully
        assert response.status_code == 200, f"Chat should succeed, got: {response.status_code}"

        result = response.json()

        # Business validation: Response must contain required chat fields
        assert "response" in result, "Response must include AI response"
        assert "session_id" in result, "Response must include session ID"
        assert "timestamp" in result, "Response must include timestamp"

        # Business validation: Response should be helpful and contextual
        assert len(result["response"]) > 0, "AI response should not be empty"
        assert result["session_id"] == sample_chat_data["session_id"], "Session ID should match request"

    def test_chat_maintains_session_context(
        self,
        test_client: TestClient,
        sample_chat_data: Dict[str, Any],
        operator_token: str,
        mock_holmesgpt_service
    ):
        """
        BR-HAPI-010: Chat must maintain conversation context across sessions
        Business Requirement: Stateful conversation for effective troubleshooting
        """
        headers = {"Authorization": f"Bearer {operator_token}"}

        # Configure mock responses for conversation flow
        first_response = ChatResponse(
            response="I see you're asking about pod crashes. Can you provide more details?",
            session_id=sample_chat_data["session_id"],
            suggestions=["Check pod logs", "Review resource usage"],
            timestamp=datetime.utcnow()
        )

        second_response = ChatResponse(
            response="Based on our previous discussion about pod crashes, here's additional analysis.",
            session_id=sample_chat_data["session_id"],
            suggestions=["Scale deployment", "Update resource limits"],
            timestamp=datetime.utcnow()
        )

        mock_holmesgpt_service.process_chat.side_effect = [first_response, second_response]

        # First message in session
        response1 = test_client.post("/api/v1/chat", json=sample_chat_data, headers=headers)
        assert response1.status_code == 200, "First chat message should succeed"

        # Second message in same session
        followup_data = {
            **sample_chat_data,
            "message": "Here are the pod logs showing memory issues"
        }
        response2 = test_client.post("/api/v1/chat", json=followup_data, headers=headers)
        assert response2.status_code == 200, "Follow-up chat message should succeed"

        # Business validation: Service should be called with same session ID
        call_args_list = mock_holmesgpt_service.process_chat.call_args_list
        assert len(call_args_list) == 2, "Service should be called twice"

        session_id_1 = call_args_list[0].kwargs["session_id"]
        session_id_2 = call_args_list[1].kwargs["session_id"]
        assert session_id_1 == session_id_2, "Session ID should be maintained across messages"

    def test_chat_includes_context_when_requested(
        self,
        test_client: TestClient,
        sample_chat_data: Dict[str, Any],
        operator_token: str,
        mock_holmesgpt_service,
        mock_context_service
    ):
        """
        BR-HAPI-011: Chat must include cluster context when requested
        Business Requirement: Context-aware chat responses for better troubleshooting
        """
        headers = {"Authorization": f"Bearer {operator_token}"}

        # Configure context service mock
        cluster_context = {
            "namespace": sample_chat_data["namespace"],
            "pod_count": 15,
            "service_count": 8,
            "recent_events": ["PodCrashLooping", "ServiceUnavailable"]
        }
        mock_context_service.get_current_context.return_value = cluster_context

        # Configure chat service mock
        mock_response = ChatResponse(
            response="Based on the current cluster context, I can see 15 pods in production namespace.",
            session_id=sample_chat_data["session_id"],
            context_used=cluster_context,
            suggestions=["Investigate recent events"],
            timestamp=datetime.utcnow()
        )
        mock_holmesgpt_service.process_chat.return_value = mock_response

        # Send chat with context enabled
        chat_data = {**sample_chat_data, "include_context": True}
        response = test_client.post("/api/v1/chat", json=chat_data, headers=headers)

        # Business requirement: Context-aware chat should succeed
        assert response.status_code == 200, "Context-aware chat should succeed"

        # Business validation: Context service should be called
        mock_context_service.get_current_context.assert_called_once()

        # Business validation: Context should be provided to chat service
        call_args = mock_holmesgpt_service.process_chat.call_args
        assert "context" in call_args.kwargs, "Chat service should receive context"

        result = response.json()
        # Business validation: Response should include context information
        assert "context_used" in result, "Response should include context_used field"

    def test_chat_supports_streaming_responses(
        self,
        test_client: TestClient,
        sample_chat_data: Dict[str, Any],
        operator_token: str,
        mock_holmesgpt_service
    ):
        """
        BR-HAPI-009: Chat must support streaming responses for real-time interaction
        Business Requirement: Real-time chat experience for immediate feedback
        """
        headers = {"Authorization": f"Bearer {operator_token}"}

        # Configure mock for streaming response
        mock_response = ChatResponse(
            response="Streaming response: Analyzing your pod crash issue step by step...",
            session_id=sample_chat_data["session_id"],
            suggestions=["Continue monitoring", "Check logs"],
            timestamp=datetime.utcnow()
        )
        mock_holmesgpt_service.process_chat.return_value = mock_response

        # Request streaming chat
        chat_data = {**sample_chat_data, "stream": True}
        response = test_client.post("/api/v1/chat", json=chat_data, headers=headers)

        # Business requirement: Streaming should be accepted
        assert response.status_code == 200, "Streaming chat should succeed"

        # Business validation: Service should be called with stream flag
        call_args = mock_holmesgpt_service.process_chat.call_args
        assert call_args.kwargs["stream"] == True, "Service should receive stream=True"

    def test_chat_validates_required_fields(
        self,
        test_client: TestClient,
        operator_token: str
    ):
        """
        BR-HAPI-007: Chat must validate required input fields
        Business Requirement: Input validation prevents invalid chat requests
        """
        headers = {"Authorization": f"Bearer {operator_token}"}

        # Test missing message
        incomplete_data = {
            "session_id": "test-session",
            "namespace": "test"
        }

        response = test_client.post("/api/v1/chat", json=incomplete_data, headers=headers)

        # Business requirement: Missing message should be rejected
        assert response.status_code == 422, "Missing message should be rejected"

        # Test missing session_id
        incomplete_data = {
            "message": "Test message",
            "namespace": "test"
        }

        response = test_client.post("/api/v1/chat", json=incomplete_data, headers=headers)

        # Business requirement: Missing session_id should be rejected
        assert response.status_code == 422, "Missing session_id should be rejected"

    def test_chat_handles_service_failures_gracefully(
        self,
        test_client: TestClient,
        sample_chat_data: Dict[str, Any],
        operator_token: str,
        mock_holmesgpt_service
    ):
        """
        BR-HAPI-008: Chat must handle service failures gracefully
        Business Requirement: Robust error handling for chat service failures
        """
        headers = {"Authorization": f"Bearer {operator_token}"}

        # Configure mock to simulate service failure
        mock_holmesgpt_service.process_chat.side_effect = Exception("Chat service unavailable")

        response = test_client.post("/api/v1/chat", json=sample_chat_data, headers=headers)

        # Business requirement: Should return appropriate error status
        assert response.status_code == 500, "Service failure should return 500 status"

        # Business requirement: Error response should be informative
        result = response.json()
        assert "detail" in result, "Error response should include details"
        assert "Chat processing failed" in result["detail"], "Error should indicate chat failure"

    def test_chat_provides_helpful_suggestions(
        self,
        test_client: TestClient,
        sample_chat_data: Dict[str, Any],
        operator_token: str,
        mock_holmesgpt_service
    ):
        """
        BR-HAPI-008: Chat must provide helpful follow-up suggestions
        Business Requirement: Guided troubleshooting with suggested next steps
        """
        headers = {"Authorization": f"Bearer {operator_token}"}

        # Configure mock with helpful suggestions
        mock_response = ChatResponse(
            response="I can help you troubleshoot the pod crash issue.",
            session_id=sample_chat_data["session_id"],
            suggestions=[
                "Check pod resource limits and requests",
                "Review recent deployment changes",
                "Examine application logs for errors",
                "Verify service connectivity"
            ],
            timestamp=datetime.utcnow()
        )
        mock_holmesgpt_service.process_chat.return_value = mock_response

        response = test_client.post("/api/v1/chat", json=sample_chat_data, headers=headers)

        # Business requirement: Chat should provide suggestions
        assert response.status_code == 200, "Chat should succeed"

        result = response.json()

        # Business validation: Suggestions should be provided
        assert "suggestions" in result, "Response should include suggestions"
        assert isinstance(result["suggestions"], list), "Suggestions should be a list"
        assert len(result["suggestions"]) > 0, "Should provide at least one suggestion"

        # Business validation: Suggestions should be actionable
        for suggestion in result["suggestions"]:
            assert isinstance(suggestion, str), "Each suggestion should be a string"
            assert len(suggestion) > 10, "Suggestions should be descriptive"

    def test_chat_supports_metrics_inclusion(
        self,
        test_client: TestClient,
        sample_chat_data: Dict[str, Any],
        operator_token: str,
        mock_holmesgpt_service,
        mock_context_service
    ):
        """
        BR-HAPI-012: Chat must support including metrics data in context
        Business Requirement: Metrics-aware chat for performance troubleshooting
        """
        headers = {"Authorization": f"Bearer {operator_token}"}

        # Configure context service with metrics
        context_with_metrics = {
            "namespace": sample_chat_data["namespace"],
            "metrics": {
                "cpu_usage": "85%",
                "memory_usage": "92%",
                "pod_restarts": 15
            },
            "alerts": ["HighMemoryUsage", "PodCrashLooping"]
        }
        mock_context_service.get_current_context.return_value = context_with_metrics

        # Configure chat response
        mock_response = ChatResponse(
            response="I can see high memory usage (92%) which may be causing pod crashes.",
            session_id=sample_chat_data["session_id"],
            context_used=context_with_metrics,
            suggestions=["Increase memory limits", "Check for memory leaks"],
            timestamp=datetime.utcnow()
        )
        mock_holmesgpt_service.process_chat.return_value = mock_response

        # Request chat with metrics
        chat_data = {**sample_chat_data, "include_metrics": True}
        response = test_client.post("/api/v1/chat", json=chat_data, headers=headers)

        # Business requirement: Metrics-aware chat should succeed
        assert response.status_code == 200, "Metrics-aware chat should succeed"

        # Business validation: Context service should be called with metrics flag
        call_args = mock_context_service.get_current_context.call_args
        assert call_args.kwargs["include_metrics"] == True, "Should request metrics in context"

        result = response.json()
        # Business validation: Response should reference metrics context
        assert "context_used" in result, "Response should include metrics context"



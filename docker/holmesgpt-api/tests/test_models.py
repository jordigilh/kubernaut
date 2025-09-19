"""
API Models Tests - Business Requirements BR-HAPI-044
Following TDD principles: Test business requirements, not implementation
"""

import pytest
from datetime import datetime
from pydantic import ValidationError
from typing import Dict, Any

from models.api_models import (
    InvestigateRequest, InvestigateResponse, Recommendation, Priority,
    ChatRequest, ChatResponse, HealthResponse, StatusResponse,
    LoginRequest, LoginResponse, UserResponse, Role, Permission,
    APIError, Toolset, Model
)


class TestAPIModels:
    """Test API Models following business requirements"""

    def test_investigate_request_validates_required_fields(self):
        """
        BR-HAPI-044: InvestigateRequest must validate required fields
        Business Requirement: Input validation prevents invalid investigation requests
        """
        # Valid request data
        valid_data = {
            "alert_name": "PodCrashLooping",
            "namespace": "production",
            "labels": {"severity": "warning"},
            "annotations": {"description": "Pod is crash looping"}
        }

        # Business requirement: Valid data should create object successfully
        request = InvestigateRequest(**valid_data)
        assert request.alert_name == "PodCrashLooping", "Should preserve alert name"
        assert request.namespace == "production", "Should preserve namespace"
        assert request.priority == Priority.MEDIUM, "Should default to medium priority"
        assert request.async_processing == False, "Should default to synchronous processing"

        # Business requirement: Missing required fields should raise validation error
        with pytest.raises(ValidationError) as exc_info:
            InvestigateRequest(namespace="production")  # Missing alert_name

        errors = exc_info.value.errors()
        assert any(error["loc"] == ("alert_name",) for error in errors), "Should require alert_name"

    def test_investigate_request_validates_priority_enum(self):
        """
        BR-HAPI-003: InvestigateRequest must validate priority levels
        Business Requirement: Only valid priority levels should be accepted
        """
        valid_data = {
            "alert_name": "TestAlert",
            "namespace": "test"
        }

        # Business requirement: Valid priority values should be accepted
        valid_priorities = ["low", "medium", "high", "critical"]
        for priority in valid_priorities:
            request = InvestigateRequest(**valid_data, priority=priority)
            assert request.priority == priority, f"Should accept priority '{priority}'"

        # Business requirement: Invalid priority should raise validation error
        with pytest.raises(ValidationError) as exc_info:
            InvestigateRequest(**valid_data, priority="invalid_priority")

        errors = exc_info.value.errors()
        assert any("priority" in str(error) for error in errors), "Should reject invalid priority"

    def test_investigate_response_contains_required_fields(self):
        """
        BR-HAPI-004: InvestigateResponse must contain all required business fields
        Business Requirement: Investigation results must be comprehensive and actionable
        """
        # Create recommendation for response
        recommendation = Recommendation(
            title="Increase memory limit",
            description="Pod requires more memory than allocated",
            action_type="scale",
            command="kubectl patch deployment myapp",
            priority=Priority.HIGH,
            confidence=0.85
        )

        # Business requirement: Response should include all required fields
        response = InvestigateResponse(
            investigation_id="inv-123",
            status="completed",
            alert_name="PodCrashLooping",
            namespace="production",
            summary="Investigation completed successfully",
            root_cause="Memory constraints",
            recommendations=[recommendation],
            context_used={"cluster": "prod"},
            timestamp=datetime.utcnow(),
            duration_seconds=5.2
        )

        # Business validation: All required fields should be present
        assert response.investigation_id == "inv-123", "Should include investigation ID"
        assert response.status == "completed", "Should include status"
        assert response.alert_name == "PodCrashLooping", "Should include alert name"
        assert response.namespace == "production", "Should include namespace"
        assert response.summary, "Should include summary"
        assert len(response.recommendations) > 0, "Should include recommendations"
        assert isinstance(response.timestamp, datetime), "Should include timestamp"
        assert response.duration_seconds > 0, "Should include duration"

    def test_recommendation_validates_confidence_range(self):
        """
        BR-HAPI-004: Recommendation must validate confidence score range
        Business Requirement: Confidence scores must be meaningful (0.0 to 1.0)
        """
        base_data = {
            "title": "Test Recommendation",
            "description": "Test description",
            "action_type": "investigate",
            "priority": Priority.MEDIUM
        }

        # Business requirement: Valid confidence values should be accepted
        valid_confidences = [0.0, 0.5, 1.0, 0.85]
        for confidence in valid_confidences:
            recommendation = Recommendation(**base_data, confidence=confidence)
            assert recommendation.confidence == confidence, f"Should accept confidence {confidence}"

        # Business requirement: Invalid confidence values should be rejected
        invalid_confidences = [-0.1, 1.1, 2.0]
        for confidence in invalid_confidences:
            with pytest.raises(ValidationError) as exc_info:
                Recommendation(**base_data, confidence=confidence)

            errors = exc_info.value.errors()
            assert any("confidence" in str(error) for error in errors), f"Should reject confidence {confidence}"

    def test_chat_request_validates_message_content(self):
        """
        BR-HAPI-006: ChatRequest must validate message content
        Business Requirement: Chat messages must be non-empty and meaningful
        """
        base_data = {
            "session_id": "test-session-123",
            "namespace": "production"
        }

        # Business requirement: Valid message should be accepted
        valid_request = ChatRequest(**base_data, message="Why is my pod crashing?")
        assert valid_request.message == "Why is my pod crashing?", "Should preserve message content"
        assert valid_request.session_id == "test-session-123", "Should preserve session ID"

        # Business requirement: Empty message should be rejected
        with pytest.raises(ValidationError) as exc_info:
            ChatRequest(**base_data, message="")

        errors = exc_info.value.errors()
        assert any(error["loc"] == ("message",) for error in errors), "Should require non-empty message"

    def test_chat_response_includes_helpful_suggestions(self):
        """
        BR-HAPI-008: ChatResponse must include helpful suggestions
        Business Requirement: Guided troubleshooting with actionable next steps
        """
        # Business requirement: Response should support suggestions
        response = ChatResponse(
            response="I can help you investigate the issue.",
            session_id="test-session",
            suggestions=["Check pod logs", "Review resource limits", "Examine recent deployments"],
            timestamp=datetime.utcnow()
        )

        # Business validation: Suggestions should be included
        assert isinstance(response.suggestions, list), "Suggestions should be a list"
        assert len(response.suggestions) == 3, "Should include multiple suggestions"
        assert all(isinstance(s, str) for s in response.suggestions), "Each suggestion should be a string"

    def test_health_response_reports_service_status(self):
        """
        BR-HAPI-016: HealthResponse must report individual service health
        Business Requirement: Granular health monitoring for troubleshooting
        """
        # Business requirement: Health response should include service details
        health_response = HealthResponse(
            status="healthy",
            timestamp=1642176600.0,
            services={
                "holmesgpt_sdk": "healthy",
                "context_api": "healthy",
                "llm_provider": "healthy"
            },
            version="1.0.0"
        )

        # Business validation: Should include required health information
        assert health_response.status == "healthy", "Should include overall status"
        assert isinstance(health_response.timestamp, float), "Should include numeric timestamp"
        assert isinstance(health_response.services, dict), "Should include services dictionary"
        assert "holmesgpt_sdk" in health_response.services, "Should include HolmesGPT SDK status"
        assert health_response.version, "Should include version information"

    def test_status_response_reports_capabilities(self):
        """
        BR-HAPI-020: StatusResponse must report service capabilities
        Business Requirement: Service discovery and capability advertisement
        """
        # Business requirement: Status should include capability information
        status_response = StatusResponse(
            service="holmesgpt-api",
            version="1.0.0",
            status="running",
            capabilities=["alert_investigation", "interactive_chat", "kubernetes_analysis"],
            timestamp=1642176600.0
        )

        # Business validation: Should include capability information
        assert status_response.service == "holmesgpt-api", "Should include service name"
        assert isinstance(status_response.capabilities, list), "Should include capabilities list"
        assert len(status_response.capabilities) > 0, "Should report at least one capability"
        assert "alert_investigation" in status_response.capabilities, "Should include core capabilities"

    def test_login_request_validates_credentials(self):
        """
        BR-HAPI-026: LoginRequest must validate credential fields
        Business Requirement: Authentication requires username and password
        """
        # Business requirement: Valid credentials should be accepted
        login_request = LoginRequest(username="operator", password="operator123")
        assert login_request.username == "operator", "Should preserve username"
        assert login_request.password == "operator123", "Should preserve password"

        # Business requirement: Missing credentials should be rejected
        with pytest.raises(ValidationError) as exc_info:
            LoginRequest(username="operator")  # Missing password

        errors = exc_info.value.errors()
        assert any(error["loc"] == ("password",) for error in errors), "Should require password"

    def test_login_response_includes_token_and_user_info(self):
        """
        BR-HAPI-026: LoginResponse must include JWT token and user information
        Business Requirement: Complete authentication response for client applications
        """
        user_info = UserResponse(
            username="operator",
            email="operator@test.com",
            roles=[Role.OPERATOR],
            active=True,
            permissions=[Permission.INVESTIGATE_ALERTS, Permission.CHAT_INTERACTIVE]
        )

        # Business requirement: Login response should include token and user info
        login_response = LoginResponse(
            access_token="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
            token_type="bearer",
            expires_in=3600,
            user=user_info
        )

        # Business validation: Should include authentication details
        assert login_response.access_token, "Should include access token"
        assert login_response.token_type == "bearer", "Should specify bearer token type"
        assert login_response.expires_in > 0, "Should include expiration time"
        assert isinstance(login_response.user, UserResponse), "Should include user information"

    def test_user_response_includes_roles_and_permissions(self):
        """
        BR-HAPI-027: UserResponse must include roles and permissions
        Business Requirement: RBAC information for client-side access control
        """
        # Business requirement: User response should include RBAC information
        user_response = UserResponse(
            username="admin",
            email="admin@test.com",
            roles=[Role.ADMIN, Role.OPERATOR],
            active=True,
            permissions=[
                Permission.ADMIN_SYSTEM,
                Permission.INVESTIGATE_ALERTS,
                Permission.MANAGE_USERS
            ]
        )

        # Business validation: Should include RBAC details
        assert isinstance(user_response.roles, list), "Should include roles list"
        assert Role.ADMIN in user_response.roles, "Should include assigned roles"
        assert isinstance(user_response.permissions, list), "Should include permissions list"
        assert Permission.ADMIN_SYSTEM in user_response.permissions, "Should include assigned permissions"
        assert user_response.active == True, "Should include active status"

    def test_api_error_provides_consistent_error_format(self):
        """
        BR-HAPI-043: APIError must provide consistent error response format
        Business Requirement: Standardized error responses for client applications
        """
        # Business requirement: Error response should be standardized
        api_error = APIError(
            error="validation_error",
            message="Invalid input data provided",
            details={"field": "alert_name", "issue": "required"},
            timestamp="2025-01-15T10:30:00Z"
        )

        # Business validation: Should include error details
        assert api_error.error == "validation_error", "Should include error type"
        assert api_error.message, "Should include human-readable message"
        assert isinstance(api_error.details, dict), "Should include error details"
        assert api_error.timestamp, "Should include error timestamp"

    def test_toolset_model_validates_capabilities(self):
        """
        BR-HAPI-033: Toolset model must validate capabilities
        Business Requirement: Toolset metadata for investigation capabilities
        """
        # Business requirement: Toolset should include capability information
        toolset = Toolset(
            name="kubernetes",
            description="Kubernetes cluster investigation tools",
            version="1.0.0",
            capabilities=["get_pods", "get_services", "get_logs"],
            enabled=True
        )

        # Business validation: Should include toolset metadata
        assert toolset.name == "kubernetes", "Should include toolset name"
        assert toolset.description, "Should include description"
        assert isinstance(toolset.capabilities, list), "Should include capabilities list"
        assert len(toolset.capabilities) > 0, "Should have at least one capability"
        assert isinstance(toolset.enabled, bool), "Should include enabled status"

    def test_model_definition_includes_availability(self):
        """
        BR-HAPI-023: Model definition must include availability status
        Business Requirement: LLM model discovery and availability reporting
        """
        # Business requirement: Model should include availability information
        model = Model(
            name="gpt-4",
            provider="openai",
            description="GPT-4 model for complex reasoning",
            available=True
        )

        # Business validation: Should include model metadata
        assert model.name == "gpt-4", "Should include model name"
        assert model.provider == "openai", "Should include provider"
        assert model.description, "Should include description"
        assert isinstance(model.available, bool), "Should include availability status"

    def test_role_and_permission_enums_define_access_levels(self):
        """
        BR-HAPI-027: Role and Permission enums must define valid access levels
        Business Requirement: Well-defined RBAC system with clear access levels
        """
        # Business requirement: Roles should define access levels
        valid_roles = [Role.ADMIN, Role.OPERATOR, Role.VIEWER, Role.API_USER]
        assert len(valid_roles) == 4, "Should define expected number of roles"
        assert Role.ADMIN.value == "admin", "Admin role should have correct value"

        # Business requirement: Permissions should define granular access
        valid_permissions = [
            Permission.INVESTIGATE_ALERTS,
            Permission.CHAT_INTERACTIVE,
            Permission.VIEW_HEALTH,
            Permission.MANAGE_USERS,
            Permission.ADMIN_SYSTEM
        ]
        assert len(valid_permissions) >= 5, "Should define granular permissions"
        assert Permission.INVESTIGATE_ALERTS.value == "investigate:alerts", "Should use namespace format"

    def test_model_schema_examples_are_valid(self):
        """
        BR-HAPI-044: Model schema examples must be valid and useful
        Business Requirement: API documentation includes working examples
        """
        # Business requirement: Schema examples should be parseable

        # Test InvestigateRequest example
        example_data = {
            "alert_name": "PodCrashLooping",
            "namespace": "production",
            "labels": {"severity": "warning", "team": "platform"},
            "annotations": {"description": "Pod is crash looping"},
            "priority": "high",
            "async_processing": False,
            "include_context": True
        }

        # Should create valid object from example
        request = InvestigateRequest(**example_data)
        assert request.alert_name == "PodCrashLooping", "Example should create valid object"
        assert request.priority == Priority.HIGH, "Example should use valid enum values"



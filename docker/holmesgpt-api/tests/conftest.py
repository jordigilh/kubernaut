"""
Test configuration and fixtures for HolmesGPT API
Following TDD principles from project guidelines
"""

import asyncio
import pytest
import pytest_asyncio
from unittest.mock import Mock, AsyncMock
from fastapi.testclient import TestClient
from typing import Generator, Dict, Any

# Add src to path for imports
import sys
import os
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '../src'))

from main import app
from config import Settings, get_settings
from services.holmesgpt_service import HolmesGPTService
from services.auth_service import AuthService, User, Role
from services.context_api_service import ContextAPIService
from services.metrics_service import MetricsService


@pytest.fixture(scope="session")
def event_loop() -> Generator[asyncio.AbstractEventLoop, None, None]:
    """Create an instance of the default event loop for the test session."""
    loop = asyncio.get_event_loop_policy().new_event_loop()
    yield loop
    loop.close()


@pytest.fixture
def test_settings() -> Settings:
    """Test configuration settings"""
    return Settings(
        port=8090,
        llm_provider="openai",
        llm_model="gpt-4",
        llm_api_key="test-key",
        context_api_url="http://test-context-api:8091",
        jwt_secret_key="test-secret-key",
        access_token_expire_minutes=60
    )


@pytest.fixture
def mock_holmesgpt_service() -> Mock:
    """Mock HolmesGPT service following existing mock patterns"""
    service = Mock(spec=HolmesGPTService)
    service.initialize = AsyncMock(return_value=True)
    service.health_check = AsyncMock(return_value=True)
    service.cleanup = AsyncMock()
    # Configure investigate_alert mock with proper return structure
    from models.api_models import InvestigateResponse, ChatResponse, Recommendation, Priority
    from datetime import datetime

    mock_investigation_response = InvestigateResponse(
        investigation_id="mock-inv-123",
        status="completed",
        alert_name="MockAlert",
        namespace="mock-namespace",
        summary="Mock investigation completed",
        root_cause="Mock root cause",
        recommendations=[
            Recommendation(
                title="Mock Recommendation",
                description="Mock recommendation description",
                action_type="investigate",
                priority=Priority.MEDIUM,
                confidence=0.8
            )
        ],
        context_used={"mock": "context"},
        timestamp=datetime.now(),
        duration_seconds=1.5
    )

    mock_chat_response = ChatResponse(
        response="Mock chat response",
        session_id="mock-session",
        suggestions=["Mock suggestion 1", "Mock suggestion 2"],
        timestamp=datetime.now()
    )

    service.investigate_alert = AsyncMock(return_value=mock_investigation_response)
    service.process_chat = AsyncMock(return_value=mock_chat_response)
    service.get_capabilities = AsyncMock(return_value=["alert_investigation", "chat"])
    service.get_configuration = AsyncMock(return_value={
        "llm_provider": "openai",
        "llm_model": "gpt-4",
        "available_toolsets": ["kubernetes"],
        "max_concurrent_investigations": 10
    })
    service.get_available_toolsets = AsyncMock(return_value=[])
    service.get_supported_models = AsyncMock(return_value=[])
    return service


@pytest.fixture
def mock_context_service() -> Mock:
    """Mock Context API service"""
    service = Mock(spec=ContextAPIService)
    service.initialize = AsyncMock(return_value=True)
    service.health_check = AsyncMock(return_value=True)
    service.cleanup = AsyncMock()
    service.enrich_alert_context = AsyncMock(return_value={
        "alert_name": "test-alert",
        "namespace": "test-namespace",
        "enrichment_status": "success"
    })
    service.get_current_context = AsyncMock(return_value={
        "cluster_status": "healthy",
        "namespace_count": 5
    })
    return service


@pytest.fixture
def mock_auth_service(test_settings: Settings) -> AuthService:
    """Real auth service with test users"""
    auth_service = AuthService(test_settings)

    # Add test users following business requirements
    test_users = {
        "test_admin": User(
            username="test_admin",
            email="admin@test.local",
            roles=[Role.ADMIN],
            hashed_password=auth_service.hash_password("admin123"),
            active=True
        ),
        "test_operator": User(
            username="test_operator",
            email="operator@test.local",
            roles=[Role.OPERATOR],
            hashed_password=auth_service.hash_password("operator123"),
            active=True
        ),
        "test_viewer": User(
            username="test_viewer",
            email="viewer@test.local",
            roles=[Role.VIEWER],
            hashed_password=auth_service.hash_password("viewer123"),
            active=True
        )
    }

    auth_service.users_db.update(test_users)
    return auth_service


@pytest.fixture
def mock_metrics_service() -> Mock:
    """Mock metrics service"""
    service = Mock(spec=MetricsService)
    service.set_app_info = Mock()
    service.record_http_request = Mock()
    service.track_investigation = Mock()
    service.track_chat_message = Mock()
    service.record_error = Mock()
    service.set_service_status = Mock()
    service.get_metrics = Mock(return_value="# Mock metrics\n")
    service.get_content_type = Mock(return_value="text/plain")
    return service


@pytest.fixture
def test_client(
    test_settings: Settings,
    mock_holmesgpt_service: Mock,
    mock_context_service: Mock,
    mock_auth_service: AuthService,
    mock_metrics_service: Mock
) -> TestClient:
    """Test client with mocked services"""

    # Override settings - ensure test settings are used consistently
    def override_get_settings():
        return test_settings

    # Clear any existing overrides
    app.dependency_overrides.clear()
    app.dependency_overrides[get_settings] = override_get_settings

    # Also override the auth service dependency to use test settings
    from api.routes.auth import get_auth_service_dependency
    def override_auth_service_dependency():
        return mock_auth_service
    app.dependency_overrides[get_auth_service_dependency] = override_auth_service_dependency

    # Set up app state with mocked services
    app.state.holmesgpt_service = mock_holmesgpt_service
    app.state.context_api_service = mock_context_service
    app.state.auth_service = mock_auth_service
    app.state.metrics_service = mock_metrics_service

    client = TestClient(app)

    yield client

    # Clean up overrides
    app.dependency_overrides.clear()


@pytest.fixture
def admin_token(mock_auth_service: AuthService) -> str:
    """Valid admin JWT token for testing"""
    return mock_auth_service.create_access_token(
        data={
            "sub": "test_admin",
            "roles": ["admin"],
            "email": "admin@test.local"
        }
    )


@pytest.fixture
def operator_token(mock_auth_service: AuthService) -> str:
    """Valid operator JWT token for testing"""
    return mock_auth_service.create_access_token(
        data={
            "sub": "test_operator",
            "roles": ["operator"],
            "email": "operator@test.local"
        }
    )


@pytest.fixture
def viewer_token(mock_auth_service: AuthService) -> str:
    """Valid viewer JWT token for testing"""
    return mock_auth_service.create_access_token(
        data={
            "sub": "test_viewer",
            "roles": ["viewer"],
            "email": "viewer@test.local"
        }
    )


# Business requirement test data following BR-HAPI specifications

@pytest.fixture
def sample_alert_data() -> Dict[str, Any]:
    """Sample alert data for investigation tests - BR-HAPI-001"""
    return {
        "alert_name": "PodCrashLooping",
        "namespace": "production",
        "labels": {
            "severity": "warning",
            "team": "platform",
            "component": "frontend"
        },
        "annotations": {
            "description": "Pod is crash looping",
            "runbook_url": "https://runbooks.company.com/pod-crash"
        },
        "priority": "high",
        "async_processing": False,
        "include_context": True
    }


@pytest.fixture
def sample_chat_data() -> Dict[str, Any]:
    """Sample chat data for interactive tests - BR-HAPI-006"""
    return {
        "message": "Why is my pod crashing in the frontend namespace?",
        "session_id": "test-session-123",
        "namespace": "production",
        "include_context": True,
        "include_metrics": False,
        "stream": False
    }


@pytest.fixture
def expected_investigation_response() -> Dict[str, Any]:
    """Expected investigation response structure - BR-HAPI-004"""
    return {
        "investigation_id": "inv-test123",
        "status": "completed",
        "alert_name": "PodCrashLooping",
        "namespace": "production",
        "summary": "Investigation completed for PodCrashLooping in production",
        "root_cause": "Pod requires more memory than allocated",
        "recommendations": [
            {
                "title": "Increase memory limit",
                "description": "Pod requires more memory than allocated",
                "action_type": "scale",
                "command": "kubectl patch deployment myapp -p '{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"main\",\"resources\":{\"limits\":{\"memory\":\"512Mi\"}}}]}}}}'",
                "priority": "high",
                "confidence": 0.85
            }
        ],
        "context_used": {},
        "duration_seconds": 5.2
    }


# ========================================
# OAuth 2 Resource Server Test Fixtures - BR-HAPI-045
# ========================================
# Note: OAuth 2 authorization server fixtures removed - now using resource server pattern
# The holmesgpt-api now acts as an OAuth 2 resource server that validates K8s ServiceAccount tokens


@pytest.fixture
def mock_k8s_token_validator() -> Mock:
    """Mock K8sTokenValidator for unit testing"""
    from services.k8s_token_validator import K8sTokenValidator
    from services.oauth2_service import K8sServiceAccountInfo, OAuth2Scope
    from datetime import datetime, timedelta

    mock_validator = Mock(spec=K8sTokenValidator)

    # Mock valid K8s ServiceAccount token
    test_sa_info = K8sServiceAccountInfo(
        name="test-sa",
        namespace="default",
        uid="sa-uid-12345",
        audiences=["https://kubernetes.default.svc"],
        scopes=[OAuth2Scope.CLUSTER_INFO, OAuth2Scope.PODS_READ, OAuth2Scope.ALERTS_INVESTIGATE],
        expires_at=datetime.utcnow() + timedelta(hours=1)
    )

    mock_validator.validate_k8s_token.return_value = test_sa_info
    mock_validator.map_k8s_to_oauth2_scopes.return_value = test_sa_info.scopes
    mock_validator.validate_k8s_api_server_token.return_value = True

    return mock_validator


@pytest.fixture
def mock_oauth2_scope_validator() -> Mock:
    """Mock OAuth2ScopeValidator for unit testing"""
    from services.oauth2_service import OAuth2ScopeValidator, OAuth2Scope

    mock_validator = Mock(spec=OAuth2ScopeValidator)

    # Mock scope validation logic
    def mock_validate_scope_access(required_scope, granted_scopes):
        return required_scope in granted_scopes

    mock_validator.validate_scope_access.side_effect = mock_validate_scope_access

    # Mock endpoint to scope mapping
    endpoint_scope_mapping = {
        ("/investigate", "POST"): OAuth2Scope.ALERTS_INVESTIGATE,
        ("/chat", "POST"): OAuth2Scope.CHAT_INTERACTIVE,
        ("/health", "GET"): OAuth2Scope.CLUSTER_INFO,
        ("/auth/users", "GET"): OAuth2Scope.ADMIN_USERS,
        ("/auth/users", "POST"): OAuth2Scope.ADMIN_USERS
    }

    def mock_get_endpoint_required_scope(endpoint_path, http_method):
        return endpoint_scope_mapping.get((endpoint_path, http_method), OAuth2Scope.CLUSTER_INFO)

    mock_validator.get_endpoint_required_scope.side_effect = mock_get_endpoint_required_scope

    # Mock RBAC to scope migration
    rbac_scope_mapping = {
        "admin": [OAuth2Scope.ADMIN_SYSTEM, OAuth2Scope.ADMIN_USERS, OAuth2Scope.CLUSTER_INFO,
                 OAuth2Scope.PODS_READ, OAuth2Scope.PODS_WRITE, OAuth2Scope.ALERTS_INVESTIGATE,
                 OAuth2Scope.CHAT_INTERACTIVE],
        "operator": [OAuth2Scope.CLUSTER_INFO, OAuth2Scope.PODS_READ, OAuth2Scope.ALERTS_INVESTIGATE,
                    OAuth2Scope.CHAT_INTERACTIVE],
        "viewer": [OAuth2Scope.CLUSTER_INFO, OAuth2Scope.PODS_READ],
        "api_user": [OAuth2Scope.ALERTS_INVESTIGATE, OAuth2Scope.CHAT_INTERACTIVE]
    }

    def mock_migrate_rbac_to_scopes(roles):
        scopes = []
        for role in roles:
            scopes.extend(rbac_scope_mapping.get(role, []))
        return list(set(scopes))  # Remove duplicates

    mock_validator.migrate_rbac_to_scopes.side_effect = mock_migrate_rbac_to_scopes

    return mock_validator


@pytest.fixture
def oauth2_test_tokens() -> Dict[str, str]:
    """Pre-generated test tokens for OAuth 2 testing"""
    return {
        "valid_access_token": "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.valid.token",
        "expired_access_token": "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.expired.token",
        "invalid_access_token": "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.invalid.token",
        "k8s_service_account_token": "eyJhbGciOiJSUzI1NiIsImtpZCI6ImsxIn0.eyJpc3MiOiJrdWJlcm5ldGVzL3NlcnZpY2VhY2NvdW50Iiwic3ViIjoic3lzdGVtOnNlcnZpY2VhY2NvdW50OmRlZmF1bHQ6dGVzdC1zYSJ9.signature",
        "refresh_token": "refresh_token_12345"
    }


@pytest.fixture
def oauth2_test_clients() -> Dict[str, Dict[str, Any]]:
    """Pre-configured test OAuth 2 clients"""
    return {
        "confidential_client": {
            "client_id": "test-confidential-client",
            "client_secret": "confidential-secret",
            "client_type": "confidential",
            "redirect_uris": ["https://app.test.local/callback"],
            "grant_types": ["authorization_code", "client_credentials"]
        },
        "public_client": {
            "client_id": "test-public-client",
            "client_secret": None,
            "client_type": "public",
            "redirect_uris": ["https://spa.test.local/callback"],
            "grant_types": ["authorization_code"]
        },
        "k8s_dashboard": {
            "client_id": "k8s-dashboard",
            "client_secret": "dashboard-secret",
            "client_type": "confidential",
            "redirect_uris": ["https://dashboard.k8s.local/oauth/callback"],
            "grant_types": ["authorization_code"]
        }
    }

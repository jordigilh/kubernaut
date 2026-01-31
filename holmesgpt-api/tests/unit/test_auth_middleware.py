"""
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
"""

"""
Unit Tests for Authentication Middleware

Business Requirements: BR-HAPI-066, BR-HAPI-067, BR-HAPI-068
Test Coverage Target: 24% → 80%

Phase 1.2 of HolmesGPT API Implementation Plan
"""

import pytest
from unittest.mock import Mock, patch, AsyncMock
from fastapi import Request, HTTPException, status
from fastapi.responses import JSONResponse
from starlette.datastructures import Headers
from src.middleware.auth import AuthenticationMiddleware


# ========================================
# TEST SUITE 1: User Class
# ========================================
# NOTE: User class removed from production code - these tests are deprecated
# The authentication middleware now works with K8s ServiceAccount tokens directly
# without a separate User class abstraction.

# class TestUser:
#     """Test User class"""
#
#     def test_user_initialization(self):
#         """Test user initialization with username and role"""
#         user = User(username="test-user", role="admin")
#
#         assert user.username == "test-user"
#         assert user.role == "admin"
#
#     def test_user_default_role(self):
#         """Test user initialization with default readonly role"""
#         user = User(username="test-user")
#
#         assert user.username == "test-user"
#         assert user.role == "readonly"


# ========================================
# TEST SUITE 2: Middleware Initialization
# ========================================

class TestAuthenticationMiddlewareInit:
    """Test middleware initialization"""

    def test_initialization_with_dev_mode(self):
        """Test middleware initialization with dev_mode enabled"""
        app = Mock()
        config = {"dev_mode": True}

        middleware = AuthenticationMiddleware(app, config)

        assert middleware.config == config
        assert middleware.dev_mode is True

    def test_initialization_without_dev_mode(self):
        """Test middleware initialization with dev_mode disabled"""
        app = Mock()
        config = {"dev_mode": False}

        middleware = AuthenticationMiddleware(app, config)

        assert middleware.config == config
        assert middleware.dev_mode is False

    def test_initialization_dev_mode_default(self):
        """Test middleware initialization with default dev_mode (False)"""
        app = Mock()
        config = {}

        middleware = AuthenticationMiddleware(app, config)

        assert middleware.dev_mode is False

    def test_public_endpoints_defined(self):
        """Test that public endpoints are defined"""
        assert "/health" in AuthenticationMiddleware.PUBLIC_ENDPOINTS
        assert "/ready" in AuthenticationMiddleware.PUBLIC_ENDPOINTS
        assert "/docs" in AuthenticationMiddleware.PUBLIC_ENDPOINTS
        assert "/redoc" in AuthenticationMiddleware.PUBLIC_ENDPOINTS
        assert "/openapi.json" in AuthenticationMiddleware.PUBLIC_ENDPOINTS


# ========================================
# TEST SUITE 3: Public Endpoint Bypass
# ========================================

class TestPublicEndpointBypass:
    """Test public endpoint bypass logic"""

    @pytest.mark.asyncio
    async def test_health_endpoint_bypasses_auth(self):
        """Test /health endpoint bypasses authentication"""
        app = Mock()
        config = {"dev_mode": False}
        middleware = AuthenticationMiddleware(app, config)

        request = Mock(spec=Request)
        request.url = Mock()
        request.url.path = "/health"
        request.headers = Headers({})

        call_next = AsyncMock(return_value="response")

        response = await middleware.dispatch(request, call_next)

        assert response == "response"
        call_next.assert_called_once_with(request)

    @pytest.mark.asyncio
    async def test_ready_endpoint_bypasses_auth(self):
        """Test /ready endpoint bypasses authentication"""
        app = Mock()
        config = {"dev_mode": False}
        middleware = AuthenticationMiddleware(app, config)

        request = Mock(spec=Request)
        request.url = Mock()
        request.url.path = "/ready"
        request.headers = Headers({})

        call_next = AsyncMock(return_value="response")

        response = await middleware.dispatch(request, call_next)

        assert response == "response"
        call_next.assert_called_once_with(request)

    @pytest.mark.asyncio
    async def test_docs_endpoint_bypasses_auth(self):
        """Test /docs endpoint bypasses authentication"""
        app = Mock()
        config = {"dev_mode": False}
        middleware = AuthenticationMiddleware(app, config)

        request = Mock(spec=Request)
        request.url = Mock()
        request.url.path = "/docs"
        request.headers = Headers({})

        call_next = AsyncMock(return_value="response")

        response = await middleware.dispatch(request, call_next)

        assert response == "response"
        call_next.assert_called_once_with(request)


# ========================================
# TEST SUITE 4: Bearer Token Extraction
# ========================================

class TestBearerTokenExtraction:
    """Test Bearer token extraction from Authorization header"""

    @pytest.mark.asyncio
    async def test_extract_bearer_token_success(self):
        """Test successful Bearer token extraction"""
        app = Mock()
        config = {"dev_mode": True}
        middleware = AuthenticationMiddleware(app, config)

        request = Mock(spec=Request)
        request.url = Mock()
        request.url.path = "/api/v1/investigate"
        request.headers = Headers({"Authorization": "Bearer test-token-user-admin"})
        request.state = Mock()

        call_next = AsyncMock(return_value="response")

        response = await middleware.dispatch(request, call_next)

        assert response == "response"
        assert request.state.user.username == "user"
        assert request.state.user.role == "admin"

    @pytest.mark.asyncio
    async def test_no_authorization_header(self):
        """Test request without Authorization header"""
        app = Mock()
        config = {"dev_mode": False}
        middleware = AuthenticationMiddleware(app, config)

        request = Mock(spec=Request)
        request.url = Mock()
        request.url.path = "/api/v1/investigate"
        request.headers = Headers({})

        call_next = AsyncMock()

        response = await middleware.dispatch(request, call_next)

        assert isinstance(response, JSONResponse)
        assert response.status_code == status.HTTP_401_UNAUTHORIZED

    @pytest.mark.asyncio
    async def test_invalid_authorization_format(self):
        """Test Authorization header without Bearer prefix"""
        app = Mock()
        config = {"dev_mode": False}
        middleware = AuthenticationMiddleware(app, config)

        request = Mock(spec=Request)
        request.url = Mock()
        request.url.path = "/api/v1/investigate"
        request.headers = Headers({"Authorization": "Basic dXNlcjpwYXNz"})

        call_next = AsyncMock()

        response = await middleware.dispatch(request, call_next)

        assert isinstance(response, JSONResponse)
        assert response.status_code == status.HTTP_401_UNAUTHORIZED


# ========================================
# TEST SUITE 5: Dev Mode Token Validation
# ========================================

class TestDevModeTokenValidation:
    """Test dev mode token validation logic"""

    @pytest.mark.asyncio
    async def test_dev_token_with_username_and_role(self):
        """Test dev token in format test-token-username-role"""
        app = Mock()
        config = {"dev_mode": True}
        middleware = AuthenticationMiddleware(app, config)

        request = Mock(spec=Request)
        request.url = Mock()
        request.url.path = "/api/v1/investigate"
        request.headers = Headers({"Authorization": "Bearer test-token-alice-operator"})
        request.state = Mock()

        call_next = AsyncMock(return_value="response")

        response = await middleware.dispatch(request, call_next)

        assert response == "response"
        assert request.state.user.username == "alice"
        assert request.state.user.role == "operator"

    @pytest.mark.asyncio
    async def test_dev_token_with_username_only(self):
        """Test dev token with username, default role"""
        app = Mock()
        config = {"dev_mode": True}
        middleware = AuthenticationMiddleware(app, config)

        request = Mock(spec=Request)
        request.url = Mock()
        request.url.path = "/api/v1/investigate"
        request.headers = Headers({"Authorization": "Bearer test-token-bob-readonly"})
        request.state = Mock()

        call_next = AsyncMock(return_value="response")

        response = await middleware.dispatch(request, call_next)

        assert response == "response"
        assert request.state.user.username == "bob"
        assert request.state.user.role == "readonly"

    @pytest.mark.asyncio
    async def test_dev_mode_disabled_rejects_test_token(self):
        """Test dev token rejected when dev_mode is disabled"""
        app = Mock()
        config = {"dev_mode": False}
        middleware = AuthenticationMiddleware(app, config)

        request = Mock(spec=Request)
        request.url = Mock()
        request.url.path = "/api/v1/investigate"
        request.headers = Headers({"Authorization": "Bearer test-token-user-admin"})

        call_next = AsyncMock()

        with patch.object(middleware, '_call_token_reviewer_api', side_effect=Exception("Invalid token")):
            response = await middleware.dispatch(request, call_next)

        assert isinstance(response, JSONResponse)
        assert response.status_code == status.HTTP_401_UNAUTHORIZED


# ========================================
# TEST SUITE 6: TokenReviewer API Integration
# ========================================

class TestTokenReviewerAPI:
    """Test TokenReviewer API integration"""

    @pytest.mark.asyncio
    async def test_token_reviewer_success(self):
        """Test successful TokenReviewer API call"""
        app = Mock()
        config = {"dev_mode": False}
        middleware = AuthenticationMiddleware(app, config)

        # _call_token_reviewer_api returns dict with username and role
        mock_user_info = {
            "username": "system:serviceaccount:kubernaut:aianalysis",
            "role": "operator"
        }

        request = Mock(spec=Request)
        request.url = Mock()
        request.url.path = "/api/v1/investigate"
        request.headers = Headers({"Authorization": "Bearer real-k8s-token"})
        request.state = Mock()

        call_next = AsyncMock(return_value="response")

        with patch.object(middleware, '_call_token_reviewer_api', return_value=mock_user_info):
            response = await middleware.dispatch(request, call_next)

        assert response == "response"
        assert request.state.user.username == "system:serviceaccount:kubernaut:aianalysis"
        assert request.state.user.role == "operator"

    @pytest.mark.asyncio
    async def test_token_reviewer_unauthenticated(self):
        """Test TokenReviewer returns unauthenticated"""
        app = Mock()
        config = {"dev_mode": False}
        middleware = AuthenticationMiddleware(app, config)

        request = Mock(spec=Request)
        request.url = Mock()
        request.url.path = "/api/v1/investigate"
        request.headers = Headers({"Authorization": "Bearer invalid-token"})

        call_next = AsyncMock()

        with patch.object(middleware, '_call_token_reviewer_api', side_effect=HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Token not authenticated by Kubernetes"
        )):
            response = await middleware.dispatch(request, call_next)

        assert isinstance(response, JSONResponse)
        assert response.status_code == status.HTTP_401_UNAUTHORIZED

    @pytest.mark.asyncio
    async def test_token_reviewer_api_error(self):
        """Test TokenReviewer API network error"""
        app = Mock()
        config = {"dev_mode": False}
        middleware = AuthenticationMiddleware(app, config)

        request = Mock(spec=Request)
        request.url = Mock()
        request.url.path = "/api/v1/investigate"
        request.headers = Headers({"Authorization": "Bearer test-token"})

        call_next = AsyncMock()

        with patch.object(middleware, '_call_token_reviewer_api', side_effect=Exception("Connection timeout")):
            response = await middleware.dispatch(request, call_next)

        assert isinstance(response, JSONResponse)
        assert response.status_code == status.HTTP_401_UNAUTHORIZED


# ========================================
# TEST SUITE 7: Role Mapping
# ========================================

class TestRoleMapping:
    """Test Kubernetes group to role mapping"""

    def test_map_system_masters_to_admin(self):
        """Test system:masters group maps to admin role"""
        app = Mock()
        config = {}
        middleware = AuthenticationMiddleware(app, config)

        groups = ["system:masters", "system:authenticated"]
        role = middleware._map_k8s_groups_to_role(groups)

        assert role == "admin"

    def test_map_kubernaut_operators_to_operator(self):
        """Test kubernaut:operators group maps to operator role"""
        app = Mock()
        config = {}
        middleware = AuthenticationMiddleware(app, config)

        groups = ["kubernaut:operators", "system:authenticated"]
        role = middleware._map_k8s_groups_to_role(groups)

        assert role == "operator"

    def test_map_unknown_groups_to_readonly(self):
        """Test unknown groups map to readonly role"""
        app = Mock()
        config = {}
        middleware = AuthenticationMiddleware(app, config)

        groups = ["system:authenticated", "system:serviceaccounts"]
        role = middleware._map_k8s_groups_to_role(groups)

        assert role == "readonly"

    def test_map_empty_groups_to_readonly(self):
        """Test empty groups list maps to readonly role"""
        app = Mock()
        config = {}
        middleware = AuthenticationMiddleware(app, config)

        groups = []
        role = middleware._map_k8s_groups_to_role(groups)

        assert role == "readonly"


# ========================================
# TEST SUITE 8: Permission Checking
# ========================================

class TestPermissionChecking:
    """Test permission checking logic"""

    @pytest.mark.asyncio
    async def test_authenticated_user_has_permission(self):
        """Test authenticated users have permission (minimal RBAC)"""
        app = Mock()
        config = {"dev_mode": True}
        middleware = AuthenticationMiddleware(app, config)

        request = Mock(spec=Request)
        request.url = Mock()
        request.url.path = "/api/v1/investigate"
        request.headers = Headers({"Authorization": "Bearer test-token-user-readonly"})
        request.state = Mock()

        call_next = AsyncMock(return_value="response")

        response = await middleware.dispatch(request, call_next)

        # Since _check_permissions always returns True for internal service
        assert response == "response"

    def test_check_permissions_always_true(self):
        """Test _check_permissions returns True (K8s RBAC handles authorization)"""
        app = Mock()
        config = {}
        middleware = AuthenticationMiddleware(app, config)

        user = User(username="test", role="readonly")
        request = Mock(spec=Request)

        has_permission = middleware._check_permissions(user, request)

        assert has_permission is True

    @pytest.mark.asyncio
    async def test_permission_denied_returns_403(self):
        """Test permission denied returns 403 Forbidden (edge case)"""
        app = Mock()
        config = {"dev_mode": True}
        middleware = AuthenticationMiddleware(app, config)

        request = Mock(spec=Request)
        request.url = Mock()
        request.url.path = "/api/v1/investigate"
        request.headers = Headers({"Authorization": "Bearer test-token-user-admin"})
        request.state = Mock()

        call_next = AsyncMock()

        # Mock _check_permissions to return False (permission denied)
        with patch.object(middleware, '_check_permissions', return_value=False):
            response = await middleware.dispatch(request, call_next)

        assert isinstance(response, JSONResponse)
        assert response.status_code == status.HTTP_403_FORBIDDEN


# ========================================
# TEST SUITE 9: Error Handling
# ========================================

class TestErrorHandling:
    """Test error handling in middleware"""

    @pytest.mark.asyncio
    async def test_http_exception_returns_json_response(self):
        """Test HTTPException is caught and returned as JSONResponse"""
        app = Mock()
        config = {"dev_mode": False}
        middleware = AuthenticationMiddleware(app, config)

        request = Mock(spec=Request)
        request.url = Mock()
        request.url.path = "/api/v1/investigate"
        request.headers = Headers({})  # No auth header

        call_next = AsyncMock()

        response = await middleware.dispatch(request, call_next)

        assert isinstance(response, JSONResponse)
        assert response.status_code == status.HTTP_401_UNAUTHORIZED

    @pytest.mark.asyncio
    async def test_generic_exception_returns_500(self):
        """Test generic exceptions return 500 Internal Server Error"""
        app = Mock()
        config = {"dev_mode": True}
        middleware = AuthenticationMiddleware(app, config)

        request = Mock(spec=Request)
        request.url = Mock()
        request.url.path = "/api/v1/investigate"
        request.headers = Headers({"Authorization": "Bearer test-token-user-admin"})
        request.state = Mock()

        call_next = AsyncMock(side_effect=Exception("Unexpected error"))

        response = await middleware.dispatch(request, call_next)

        assert isinstance(response, JSONResponse)
        assert response.status_code == status.HTTP_500_INTERNAL_SERVER_ERROR


# ========================================
# TEST SUITE 10: Integration Scenarios
# ========================================

class TestIntegrationScenarios:
    """Test end-to-end authentication scenarios"""

    @pytest.mark.asyncio
    async def test_full_auth_flow_dev_mode(self):
        """Test complete authentication flow in dev mode"""
        app = Mock()
        config = {"dev_mode": True}
        middleware = AuthenticationMiddleware(app, config)

        request = Mock(spec=Request)
        request.url = Mock()
        request.url.path = "/api/v1/investigate"
        request.headers = Headers({"Authorization": "Bearer test-token-analyst-operator"})
        request.state = Mock()

        call_next = AsyncMock(return_value="investigation_response")

        response = await middleware.dispatch(request, call_next)

        assert response == "investigation_response"
        assert request.state.user.username == "analyst"
        assert request.state.user.role == "operator"
        call_next.assert_called_once_with(request)

    @pytest.mark.asyncio
    async def test_unauthorized_request_rejected(self):
        """Test request without valid authentication is rejected"""
        app = Mock()
        config = {"dev_mode": False}
        middleware = AuthenticationMiddleware(app, config)

        request = Mock(spec=Request)
        request.url = Mock()
        request.url.path = "/api/v1/investigate"
        request.headers = Headers({"Authorization": "Bearer invalid-token"})

        call_next = AsyncMock()

        with patch.object(middleware, '_call_token_reviewer_api', side_effect=Exception("Invalid")):
            response = await middleware.dispatch(request, call_next)

        assert isinstance(response, JSONResponse)
        assert response.status_code == status.HTTP_401_UNAUTHORIZED
        call_next.assert_not_called()

    @pytest.mark.asyncio
    async def test_malformed_token_rejected(self):
        """Test malformed dev token is rejected"""
        app = Mock()
        config = {"dev_mode": True}
        middleware = AuthenticationMiddleware(app, config)

        request = Mock(spec=Request)
        request.url = Mock()
        request.url.path = "/api/v1/investigate"
        # Malformed token (not test-token- prefix)
        request.headers = Headers({"Authorization": "Bearer malformed-token"})

        call_next = AsyncMock()

        with patch.object(middleware, '_call_token_reviewer_api', side_effect=Exception("Invalid")):
            response = await middleware.dispatch(request, call_next)

        assert isinstance(response, JSONResponse)
        assert response.status_code == status.HTTP_401_UNAUTHORIZED

    @pytest.mark.asyncio
    async def test_expired_token_rejected(self):
        """Test expired token is rejected by TokenReviewer"""
        app = Mock()
        config = {"dev_mode": False}
        middleware = AuthenticationMiddleware(app, config)

        request = Mock(spec=Request)
        request.url = Mock()
        request.url.path = "/api/v1/investigate"
        request.headers = Headers({"Authorization": "Bearer expired-token"})

        call_next = AsyncMock()

        with patch.object(middleware, '_call_token_reviewer_api', side_effect=HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Token expired"
        )):
            response = await middleware.dispatch(request, call_next)

        assert isinstance(response, JSONResponse)
        assert response.status_code == status.HTTP_401_UNAUTHORIZED


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
from src.auth import MockAuthenticator, MockAuthorizer


# ========================================
# TEST SUITE 1: User Class
# ========================================
# NOTE: User class removed from production code - these tests are deprecated
# The authentication middleware now works with K8s ServiceAccount tokens directly
# without a separate User class abstraction.

# class TestUser:
#     """Test User class"""
#
#     def test_user_initialization(self, mock_auth_components):
#         """Test user initialization with username and role"""
#         user = User(username="test-user", role="admin")
#
#         assert user.username == "test-user"
#         assert user.role == "admin"
#
#     def test_user_default_role(self, mock_auth_components):
#         """Test user initialization with default readonly role"""
#         user = User(username="test-user")
#
#         assert user.username == "test-user"
#         assert user.role == "readonly"


# ========================================
@pytest.fixture
def mock_auth_components():
    """Create mock authenticator and authorizer for tests"""
    valid_users = {
        "test-token": "system:serviceaccount:test:sa",
        "test-token-user-admin": "system:serviceaccount:test:admin-sa",
        "test-token-user-readonly": "system:serviceaccount:test:readonly-sa",
        "valid-token": "system:serviceaccount:test:valid-sa",
    }
    authenticator = MockAuthenticator(valid_users=valid_users)
    authorizer = MockAuthorizer(default_allow=True)
    return authenticator, authorizer


# TEST SUITE 2: Middleware Initialization
# ========================================

class TestAuthenticationMiddlewareInit:
    """Test middleware initialization"""

    def test_initialization_with_dev_mode(self, mock_auth_components):
        """Test middleware initialization with dev_mode enabled"""
        app = Mock()
        config = {"dev_mode": True}

        authenticator, authorizer = mock_auth_components
        middleware = AuthenticationMiddleware(app, authenticator, authorizer, config)

        assert middleware.config == config
        assert middleware.authenticator == authenticator
        assert middleware.authorizer == authorizer

    def test_initialization_without_dev_mode(self, mock_auth_components):
        """Test middleware initialization with authenticator and authorizer"""
        app = Mock()
        config = {"dev_mode": False}

        authenticator, authorizer = mock_auth_components
        middleware = AuthenticationMiddleware(app, authenticator, authorizer, config)

        assert middleware.config == config
        assert middleware.authenticator == authenticator
        assert middleware.authorizer == authorizer

    def test_initialization_dev_mode_default(self, mock_auth_components):
        """Test middleware initialization with dependency injection"""
        app = Mock()
        config = {}

        authenticator, authorizer = mock_auth_components
        middleware = AuthenticationMiddleware(app, authenticator, authorizer, config)

        assert middleware.authenticator == authenticator
        assert middleware.authorizer == authorizer

    def test_public_endpoints_defined(self, mock_auth_components):
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
    async def test_health_endpoint_bypasses_auth(self, mock_auth_components):
        """Test /health endpoint bypasses authentication"""
        app = Mock()
        config = {"dev_mode": False}
        authenticator, authorizer = mock_auth_components
        middleware = AuthenticationMiddleware(app, authenticator, authorizer, config)

        request = Mock(spec=Request)
        request.url = Mock()
        request.url.path = "/health"
        request.headers = Headers({})

        call_next = AsyncMock(return_value="response")

        response = await middleware.dispatch(request, call_next)

        assert response == "response"
        call_next.assert_called_once_with(request)

    @pytest.mark.asyncio
    async def test_ready_endpoint_bypasses_auth(self, mock_auth_components):
        """Test /ready endpoint bypasses authentication"""
        app = Mock()
        config = {"dev_mode": False}
        authenticator, authorizer = mock_auth_components
        middleware = AuthenticationMiddleware(app, authenticator, authorizer, config)

        request = Mock(spec=Request)
        request.url = Mock()
        request.url.path = "/ready"
        request.headers = Headers({})

        call_next = AsyncMock(return_value="response")

        response = await middleware.dispatch(request, call_next)

        assert response == "response"
        call_next.assert_called_once_with(request)

    @pytest.mark.asyncio
    async def test_docs_endpoint_bypasses_auth(self, mock_auth_components):
        """Test /docs endpoint bypasses authentication"""
        app = Mock()
        config = {"dev_mode": False}
        authenticator, authorizer = mock_auth_components
        middleware = AuthenticationMiddleware(app, authenticator, authorizer, config)

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
    async def test_extract_bearer_token_success(self, mock_auth_components):
        """Test successful Bearer token extraction"""
        app = Mock()
        config = {"dev_mode": True}
        authenticator, authorizer = mock_auth_components
        middleware = AuthenticationMiddleware(app, authenticator, authorizer, config)

        request = Mock(spec=Request)
        request.url = Mock()
        request.url.path = "/api/v1/investigate"
        request.headers = Headers({"Authorization": "Bearer test-token-user-admin"})
        request.state = Mock()

        call_next = AsyncMock(return_value="response")

        response = await middleware.dispatch(request, call_next)

        assert response == "response"
        # After DD-AUTH-014: request.state.user is user identity string (not object)
        assert request.state.user == "system:serviceaccount:test:admin-sa"
        call_next.assert_called_once_with(request)

    @pytest.mark.asyncio
    async def test_no_authorization_header(self, mock_auth_components):
        """Test request without Authorization header"""
        app = Mock()
        config = {"dev_mode": False}
        authenticator, authorizer = mock_auth_components
        middleware = AuthenticationMiddleware(app, authenticator, authorizer, config)

        request = Mock(spec=Request)
        request.url = Mock()
        request.url.path = "/api/v1/investigate"
        request.headers = Headers({})

        call_next = AsyncMock()

        response = await middleware.dispatch(request, call_next)

        assert isinstance(response, JSONResponse)
        assert response.status_code == status.HTTP_401_UNAUTHORIZED

    @pytest.mark.asyncio
    async def test_invalid_authorization_format(self, mock_auth_components):
        """Test Authorization header without Bearer prefix"""
        app = Mock()
        config = {"dev_mode": False}
        authenticator, authorizer = mock_auth_components
        middleware = AuthenticationMiddleware(app, authenticator, authorizer, config)

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

class TestPermissionChecking:
    """Test permission checking logic"""

    @pytest.mark.asyncio
    async def test_authenticated_user_has_permission(self, mock_auth_components):
        """Test authenticated users have permission (minimal RBAC)"""
        app = Mock()
        config = {"dev_mode": True}
        authenticator, authorizer = mock_auth_components
        middleware = AuthenticationMiddleware(app, authenticator, authorizer, config)

        request = Mock(spec=Request)
        request.url = Mock()
        request.url.path = "/api/v1/investigate"
        request.headers = Headers({"Authorization": "Bearer test-token-user-readonly"})
        request.state = Mock()

        call_next = AsyncMock(return_value="response")

        response = await middleware.dispatch(request, call_next)

        # Since _check_permissions always returns True for internal service
        assert response == "response"

    # NOTE: test_check_permissions_always_true DEPRECATED after DD-AUTH-014 refactor
    # Permission checking delegated to Authorizer (no longer a middleware method)

    @pytest.mark.asyncio
    async def test_permission_denied_returns_403(self, mock_auth_components):
        """Test permission denied returns 403 Forbidden (edge case)"""
        app = Mock()
        config = {"dev_mode": True}
        authenticator, authorizer = mock_auth_components
        middleware = AuthenticationMiddleware(app, authenticator, authorizer, config)

        request = Mock(spec=Request)
        request.url = Mock()
        request.url.path = "/api/v1/investigate"
        request.headers = Headers({"Authorization": "Bearer test-token-user-admin"})
        request.state = Mock()

        call_next = AsyncMock()

        # Mock authorizer.check_access to deny permission
        with patch.object(authorizer, 'check_access', new_callable=AsyncMock, return_value=False):
            response = await middleware.dispatch(request, call_next)

        assert isinstance(response, JSONResponse)
        assert response.status_code == status.HTTP_403_FORBIDDEN


# ========================================
# TEST SUITE 9: Error Handling
# ========================================

class TestErrorHandling:
    """Test error handling in middleware"""

    @pytest.mark.asyncio
    async def test_http_exception_returns_json_response(self, mock_auth_components):
        """Test HTTPException is caught and returned as JSONResponse"""
        app = Mock()
        config = {"dev_mode": False}
        authenticator, authorizer = mock_auth_components
        middleware = AuthenticationMiddleware(app, authenticator, authorizer, config)

        request = Mock(spec=Request)
        request.url = Mock()
        request.url.path = "/api/v1/investigate"
        request.headers = Headers({})  # No auth header

        call_next = AsyncMock()

        response = await middleware.dispatch(request, call_next)

        assert isinstance(response, JSONResponse)
        assert response.status_code == status.HTTP_401_UNAUTHORIZED

    @pytest.mark.asyncio
    async def test_generic_exception_returns_500(self, mock_auth_components):
        """Test generic exceptions return 500 Internal Server Error"""
        app = Mock()
        config = {"dev_mode": True}
        authenticator, authorizer = mock_auth_components
        middleware = AuthenticationMiddleware(app, authenticator, authorizer, config)

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


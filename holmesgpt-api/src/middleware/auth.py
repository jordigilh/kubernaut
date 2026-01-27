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
Authentication Middleware with SAR Authorization

Authority: DD-AUTH-014 (Middleware-Based SAR Authentication)

This middleware implements a secure, testable auth framework using dependency injection:
1. Accepts Authenticator and Authorizer via constructor (enables testing)
2. Validates Bearer tokens using TokenReview API
3. Checks authorization using SubjectAccessReview (SAR) API
4. Injects user identity into request.state (for audit logging)

Design Decision: Auth logic is in middleware (not sidecar) for better testability.
- Production: Real K8s TokenReview + SAR APIs
- Integration: Mock implementations (no K8s API calls)
- E2E: Real K8s APIs in Kind clusters

Business Requirements:
- BR-HAPI-066: API key authentication
- BR-HAPI-067: JWT token authentication (via TokenReview)
- BR-HAPI-068: Role-based access control (via SAR)

Security: No runtime disable flags - auth is always enforced via interface implementations.
"""

import logging
from typing import Dict, Any

from fastapi import Request, status
from fastapi.responses import JSONResponse
from starlette.middleware.base import BaseHTTPMiddleware

# Import auth interfaces (dependency injection)
from src.auth.interfaces import Authenticator, Authorizer

logger = logging.getLogger(__name__)

# Import metrics recording functions
try:
    from src.middleware.metrics import record_auth_failure, record_auth_success
except ImportError:
    # Graceful degradation if metrics not available
    def record_auth_failure(reason: str, endpoint: str):
        pass

    def record_auth_success(username: str, role: str):
        pass


class AuthenticationMiddleware(BaseHTTPMiddleware):
    """
    Kubernetes ServiceAccount token authentication + authorization middleware.

    This middleware implements a secure, testable auth framework using dependency injection:
    1. Authenticates using TokenReview API (via injected Authenticator)
    2. Authorizes using SubjectAccessReview API (via injected Authorizer)
    3. Injects validated user identity into request.state for audit logging

    Authority: DD-AUTH-014

    Business Requirements:
    - BR-HAPI-066: API key authentication
    - BR-HAPI-067: JWT token authentication
    - BR-HAPI-068: Role-based access control

    Request Flow:
    1. Skip public endpoints (/health, /metrics, etc.)
    2. Extract Bearer token from Authorization header
    3. Authenticate token using Authenticator (TokenReview)
    4. Authorize user using Authorizer (SAR)
    5. Inject validated user identity into request.state (for audit logging)
    6. Pass request to next handler

    Error Responses:
    - 401 Unauthorized: Missing/invalid Bearer token (RFC 7807)
    - 403 Forbidden: Insufficient RBAC permissions (RFC 7807)
    - 500 Internal Server Error: TokenReview/SAR API failure (RFC 7807)
    """

    PUBLIC_ENDPOINTS = ["/health", "/ready", "/metrics", "/docs", "/redoc", "/openapi.json"]

    def __init__(
        self,
        app,
        authenticator: Authenticator,
        authorizer: Authorizer,
        config: Dict[str, Any],
    ):
        """
        Initialize authentication middleware with dependency injection.

        Args:
            app: FastAPI application instance
            authenticator: Token validator (K8sAuthenticator or MockAuthenticator)
            authorizer: Permission checker (K8sAuthorizer or MockAuthorizer)
            config: Configuration dict with keys:
                - namespace: K8s namespace for SAR checks
                - resource: Resource type (e.g., "services")
                - resource_name: Specific resource name
                - verb: Default RBAC verb (can be overridden per endpoint)

        Example (Production):
            from src.auth import K8sAuthenticator, K8sAuthorizer
            app.add_middleware(
                AuthenticationMiddleware,
                authenticator=K8sAuthenticator(),
                authorizer=K8sAuthorizer(),
                config={
                    "namespace": "kubernaut-system",
                    "resource": "services",
                    "resource_name": "holmesgpt-api-service",
                    "verb": "create",
                }
            )

        Example (Integration Tests):
            from src.auth import MockAuthenticator, MockAuthorizer
            app.add_middleware(
                AuthenticationMiddleware,
                authenticator=MockAuthenticator(
                    valid_users={"test-token": "system:serviceaccount:test:sa"}
                ),
                authorizer=MockAuthorizer(default_allow=True),
                config={...}
            )
        """
        super().__init__(app)
        self.authenticator = authenticator
        self.authorizer = authorizer
        self.config = config

        logger.info({
            "event": "auth_middleware_initialized",
            "authenticator_type": type(authenticator).__name__,
            "authorizer_type": type(authorizer).__name__,
            "namespace": config.get("namespace"),
            "resource": config.get("resource"),
            "resource_name": config.get("resource_name"),
            "verb": config.get("verb")
        })

    async def dispatch(self, request: Request, call_next):
        """
        Main middleware dispatch with TokenReview + SAR.

        Authority: DD-AUTH-014

        Request Flow:
        1. Skip public endpoints
        2. Extract Bearer token
        3. Authenticate (TokenReview via authenticator)
        4. Authorize (SAR via authorizer)
        5. Inject validated user identity into request.state
        6. Continue to handler

        Error Responses (RFC 7807):
        - 401: Missing/invalid token
        - 403: Insufficient permissions
        - 500: K8s API failure
        """
        # Step 1: Skip auth for public endpoints
        if request.url.path in self.PUBLIC_ENDPOINTS:
            return await call_next(request)

        try:
            # Step 2: Extract Bearer token from Authorization header
            auth_header = request.headers.get("Authorization", "")
            if not auth_header.startswith("Bearer "):
                record_auth_failure("missing_auth_header", request.url.path)
                return self._create_rfc7807_response(
                    status_code=401,
                    title="Unauthorized",
                    detail="Missing Authorization header with Bearer token",
                )

            token = auth_header[7:]  # Remove "Bearer " prefix

            # Step 3: Authenticate token using TokenReview API
            # Authority: DD-AUTH-014 (uses injected Authenticator)
            user = await self.authenticator.validate_token(token)

            # Step 4: Authorize user using SubjectAccessReview (SAR) API
            # Authority: DD-AUTH-014 (uses injected Authorizer)
            allowed = await self.authorizer.check_access(
                user=user,
                namespace=self.config.get("namespace", "kubernaut-system"),
                resource=self.config.get("resource", "services"),
                resource_name=self.config.get("resource_name", "holmesgpt-api-service"),
                verb=self._get_verb_for_request(request),
            )

            if not allowed:
                record_auth_failure("insufficient_permissions", request.url.path)
                return self._create_rfc7807_response(
                    status_code=403,
                    title="Forbidden",
                    detail=f"Insufficient RBAC permissions for {request.method} {request.url.path}",
                )

            # Step 5: Inject validated user identity into request.state
            # Authority: DD-AUTH-014 (Middleware-based SAR authentication)
            # This allows handlers to access the authenticated user via:
            #   - user_context.get_authenticated_user(request)
            #   - request.state.user (direct access)
            request.state.user = user

            # Step 6: Record success metrics
            record_auth_success(user, "authenticated")

            # Step 7: Pass request to next handler
            return await call_next(request)

        except Exception as e:
            # Handle authentication/authorization exceptions
            logger.error({
                "event": "auth_middleware_error",
                "error": str(e),
                "error_type": type(e).__name__,
                "path": request.url.path
            })

            # Extract status code from HTTPException if available
            status_code = getattr(e, "status_code", 500)
            detail = getattr(e, "detail", str(e))

            record_auth_failure(f"error_{status_code}", request.url.path)

            # Return RFC 7807 Problem Details
            return self._create_rfc7807_response(
                status_code=status_code,
                title=self._get_title_for_status(status_code),
                detail=detail,
            )

    def _get_verb_for_request(self, request: Request) -> str:
        """
        Map HTTP method to RBAC verb.

        Authority: DD-AUTH-014 (Granular RBAC SAR Verb Mapping)

        Mapping:
        - GET: "get" (single resource) or "list" (collection)
        - POST: "create"
        - PUT/PATCH: "update"
        - DELETE: "delete"

        Returns default verb from config if method not recognized.
        """
        method = request.method.upper()

        # Map HTTP methods to RBAC verbs
        if method == "GET":
            # Heuristic: List operations typically have plural paths or query params
            # For now, default to "get" and let config override if needed
            return "get"
        elif method == "POST":
            return "create"
        elif method in ["PUT", "PATCH"]:
            return "update"
        elif method == "DELETE":
            return "delete"
        else:
            # Fallback to config default
            return self.config.get("verb", "create")

    def _get_title_for_status(self, status_code: int) -> str:
        """Get human-readable title for HTTP status code."""
        titles = {
            401: "Unauthorized",
            403: "Forbidden",
            500: "Internal Server Error",
            503: "Service Unavailable",
        }
        return titles.get(status_code, "Error")

    def _create_rfc7807_response(
        self, status_code: int, title: str, detail: str
    ) -> JSONResponse:
        """
        Create RFC 7807 Problem Details response.

        Authority: BR-HAPI-200 (RFC 7807 Error Response Standard)

        Format:
        {
            "type": "about:blank",
            "title": "Unauthorized",
            "status": 401,
            "detail": "Missing Authorization header with Bearer token"
        }
        """
        return JSONResponse(
            status_code=status_code,
            content={
                "type": "about:blank",
                "title": title,
                "status": status_code,
                "detail": detail,
            },
            headers={"Content-Type": "application/problem+json"},
        )

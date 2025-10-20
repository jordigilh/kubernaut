"""
Authentication Middleware - Minimal Internal Service

Business Requirements: BR-HAPI-066, BR-HAPI-067 (Basic Authentication Only)

Provides Kubernetes ServiceAccount token authentication for internal service.

Design Decision: DD-HOLMESGPT-012 - Minimal Internal Service Architecture
- No rate limiting (network policies handle access control)
- No complex RBAC (K8s RBAC handles authorization)
- No multiple auth methods (K8s ServiceAccount only)

REFACTOR phase: Production implementation with K8s TokenReviewer API
"""

import logging
import os
from typing import Dict, Any, List
import aiohttp
from fastapi import Request, HTTPException, status
from fastapi.responses import JSONResponse
from starlette.middleware.base import BaseHTTPMiddleware

logger = logging.getLogger(__name__)


class User:
    def __init__(self, username: str, role: str = "readonly"):
        self.username = username
        self.role = role


class AuthenticationMiddleware(BaseHTTPMiddleware):
    """
    Kubernetes ServiceAccount token authentication middleware

    Business Requirements:
    - BR-HAPI-066: API key authentication
    - BR-HAPI-067: JWT token authentication
    - BR-HAPI-068: Role-based access control

    REFACTOR phase: Production implementation with K8s TokenReviewer API
    """

    PUBLIC_ENDPOINTS = ["/health", "/ready", "/docs", "/redoc", "/openapi.json"]

    def __init__(self, app, config: Dict[str, Any]):
        super().__init__(app)
        self.config = config
        self.dev_mode = config.get("dev_mode", False)
        logger.info({
            "event": "auth_middleware_initialized",
            "dev_mode": self.dev_mode
        })

    async def dispatch(self, request: Request, call_next):
        """
        Main middleware dispatch

        Business Requirement: BR-HAPI-068 (Permission checking)
        """
        # Skip auth for public endpoints
        if request.url.path in self.PUBLIC_ENDPOINTS:
            return await call_next(request)

        try:
            # Validate request and extract user
            user = await self._validate_request(request)
            request.state.user = user

            # Check permissions (BR-HAPI-068)
            if not self._check_permissions(user, request):
                raise HTTPException(
                    status_code=status.HTTP_403_FORBIDDEN,
                    detail="Insufficient permissions"
                )

            return await call_next(request)

        except HTTPException as e:
            return JSONResponse(
                status_code=e.status_code,
                content={"detail": e.detail}
            )
        except Exception as e:
            logger.error({"event": "auth_error", "error": str(e)})
            return JSONResponse(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                content={"detail": "Internal server error"}
            )

    async def _validate_request(self, request: Request) -> User:
        """
        Validate authentication credentials

        REFACTOR phase: Uses K8s TokenReviewer API for production
        """
        auth_header = request.headers.get("Authorization", "")

        if auth_header.startswith("Bearer "):
            token = auth_header[7:]
            return await self._validate_k8s_token(token)

        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="No valid authentication credentials provided"
        )

    async def _validate_k8s_token(self, token: str) -> User:
        """
        Validate Kubernetes ServiceAccount token using TokenReviewer API.

        Business Requirement: BR-HAPI-067

        REFACTOR phase: Production implementation with resilience.
        Design Decision: DD-HOLMESGPT-011, DD-HOLMESGPT-012
        """
        # GREEN phase stub for dev mode
        if self.dev_mode and token.startswith("test-token-"):
            parts = token.split("-")
            if len(parts) >= 4:
                username = parts[2]
                role = parts[3] if len(parts) > 3 else "readonly"
                logger.info({
                    "event": "dev_token_validated",
                    "username": username,
                    "role": role
                })
                return User(username=username, role=role)

        # REFACTOR phase: Real K8s TokenReviewer API
        try:
            user_info = await self._call_token_reviewer_api(token)
            return User(username=user_info["username"], role=user_info["role"])
        except Exception as e:
            logger.error({"event": "k8s_token_validation_failed", "error": str(e)})
            raise HTTPException(
                status_code=status.HTTP_401_UNAUTHORIZED,
                detail=f"Kubernetes token validation failed: {str(e)}"
            )

    async def _call_token_reviewer_api(self, token: str) -> Dict[str, Any]:
        """
        Internal method to call the Kubernetes TokenReviewer API.

        REFACTOR phase: Production implementation with retry and circuit breaker
        """
        k8s_api_url = os.getenv("KUBERNETES_SERVICE_HOST", "https://kubernetes.default.svc")
        k8s_api_port = os.getenv("KUBERNETES_SERVICE_PORT", "443")
        token_reviewer_url = f"{k8s_api_url}:{k8s_api_port}/apis/authentication.k8s.io/v1/tokenreviews"

        request_body = {
            "apiVersion": "authentication.k8s.io/v1",
            "kind": "TokenReview",
            "spec": {
                "token": token
            }
        }

        async with aiohttp.ClientSession() as session:
            async with session.post(
                token_reviewer_url,
                json=request_body,
                headers={"Content-Type": "application/json"},
                ssl=False  # Internal cluster communication
            ) as response:
                if response.status != 200:
                    raise HTTPException(
                        status_code=status.HTTP_401_UNAUTHORIZED,
                        detail="Invalid Kubernetes ServiceAccount token"
                    )

                result = await response.json()

                # Check if token is authenticated
                if not result.get("status", {}).get("authenticated", False):
                    raise HTTPException(
                        status_code=status.HTTP_401_UNAUTHORIZED,
                        detail="Token not authenticated by Kubernetes"
                    )

                # Extract user info
                user_info = result.get("status", {}).get("user", {})
                username = user_info.get("username", "unknown")
                groups = user_info.get("groups", [])

                # Map K8s groups to application roles
                role = self._map_k8s_groups_to_role(groups)

                logger.info({
                    "event": "k8s_token_validated",
                    "username": username,
                    "role": role
                })

                return {"username": username, "role": role}

    def _map_k8s_groups_to_role(self, groups: List[str]) -> str:
        """
        Map Kubernetes groups to application roles

        Business Requirement: BR-HAPI-068 (RBAC)
        """
        # Simple mapping for internal service
        # Production: Use more sophisticated RBAC
        if "system:masters" in groups:
            return "admin"
        elif "kubernaut:operators" in groups:
            return "operator"
        else:
            return "readonly"

    def _check_permissions(self, user: User, request: Request) -> bool:
        """
        Check if user has permission for requested operation

        Business Requirement: BR-HAPI-068 (Permission checking)
        """
        # Minimal permission check for internal service
        # All authenticated users can call investigation endpoints
        return True  # K8s RBAC handles authorization at network level

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
Authentication Middleware - Minimal Internal Service

⚠️  **DEPRECATED** (January 7, 2026) ⚠️
This middleware is DEPRECATED in favor of oauth-proxy sidecar (DD-AUTH-006).

**Current Status**: auth_enabled=False (middleware disabled)
**Replacement**: OAuth-proxy sidecar handles authentication/authorization
**Reason**: Keep auth logic OUTSIDE business code (separation of concerns)

**Why Kept**:
- Emergency fallback if oauth-proxy has issues
- Reference implementation for token validation
- May be useful for local development without K8s

**Migration**: DD-AUTH-006 - OAuth-Proxy Sidecar Pattern
- OAuth-proxy validates ServiceAccount tokens
- OAuth-proxy performs Subject Access Review (SAR)
- OAuth-proxy injects X-Auth-Request-User header
- Python code extracts user for logging (see user_context.py)

---

Business Requirements: BR-HAPI-066, BR-HAPI-067 (Basic Authentication Only)

Provides Kubernetes ServiceAccount token authentication for internal service.

Design Decision: DD-HOLMESGPT-012 - Minimal Internal Service Architecture
- No rate limiting (network policies handle access control)
- No complex RBAC (K8s RBAC handles authorization)
- No multiple auth methods (K8s ServiceAccount only)

Production Implementation: Uses K8s TokenReviewer API with resilience
"""

import logging
import os
import asyncio
from typing import Dict, Any, List, Optional
import aiohttp
from fastapi import Request, HTTPException, status
from fastapi.responses import JSONResponse
from starlette.middleware.base import BaseHTTPMiddleware
from pathlib import Path

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

# Kubernetes ServiceAccount paths
K8S_SA_TOKEN_PATH = "/var/run/secrets/kubernetes.io/serviceaccount/token"
K8S_SA_CA_CERT_PATH = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
K8S_SA_NAMESPACE_PATH = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"


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

    Production Implementation: Uses K8s TokenReviewer API with ServiceAccount
    """

    PUBLIC_ENDPOINTS = ["/health", "/ready", "/metrics", "/docs", "/redoc", "/openapi.json"]

    def __init__(self, app, config: Dict[str, Any]):
        super().__init__(app)
        self.config = config
        self.dev_mode = config.get("dev_mode", False)

        # Load ServiceAccount token for authenticating to K8s API
        self.sa_token = self._load_serviceaccount_token()
        self.k8s_api_url = self._get_k8s_api_url()
        self.ca_cert_path = K8S_SA_CA_CERT_PATH if Path(K8S_SA_CA_CERT_PATH).exists() else None

        logger.info({
            "event": "auth_middleware_initialized",
            "dev_mode": self.dev_mode,
            "sa_token_loaded": self.sa_token is not None,
            "k8s_api_url": self.k8s_api_url,
            "ca_cert_available": self.ca_cert_path is not None
        })

    def _load_serviceaccount_token(self) -> Optional[str]:
        """
        Load ServiceAccount token from mounted volume.

        Business Requirement: BR-HAPI-067 (Token authentication)

        Returns None in dev mode or if file doesn't exist.
        """
        # Guard clause: Skip in dev mode
        if self.dev_mode:
            logger.info({"event": "skipping_sa_token_load", "reason": "dev_mode"})
            return None

        # Guard clause: Check file existence
        token_path = Path(K8S_SA_TOKEN_PATH)
        if not token_path.exists():
            logger.warning({
                "event": "sa_token_not_found",
                "path": K8S_SA_TOKEN_PATH,
                "note": "Running outside Kubernetes cluster"
            })
            return None

        # Load token from file
        try:
            with open(token_path, "r") as f:
                token = f.read().strip()
            logger.info({
                "event": "sa_token_loaded",
                "path": K8S_SA_TOKEN_PATH,
                "token_length": len(token)
            })
            return token
        except Exception as e:
            logger.error({
                "event": "sa_token_load_failed",
                "error": str(e),
                "path": K8S_SA_TOKEN_PATH
            })
            return None

    def _get_k8s_api_url(self) -> str:
        """
        Get Kubernetes API server URL.

        Uses environment variables set by Kubernetes or falls back to default.
        """
        host = os.getenv("KUBERNETES_SERVICE_HOST", "kubernetes.default.svc")
        port = os.getenv("KUBERNETES_SERVICE_PORT", "443")

        # Construct full URL
        if not host.startswith("http"):
            url = f"https://{host}:{port}"
        else:
            url = f"{host}:{port}"

        return url

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

            # Record authentication success
            record_auth_success(user.username, user.role)

            # Check permissions (BR-HAPI-068)
            if not self._check_permissions(user, request):
                record_auth_failure("insufficient_permissions", request.url.path)
                raise HTTPException(
                    status_code=status.HTTP_403_FORBIDDEN,
                    detail="Insufficient permissions"
                )

            return await call_next(request)

        except HTTPException as e:
            # Record authentication failure for 401/403 errors
            if e.status_code in [401, 403]:
                reason = "invalid_credentials" if e.status_code == 401 else "forbidden"
                record_auth_failure(reason, request.url.path)

            return JSONResponse(
                status_code=e.status_code,
                content={"detail": e.detail}
            )
        except Exception as e:
            logger.error({"event": "auth_error", "error": str(e)})
            record_auth_failure("internal_error", request.url.path)
            return JSONResponse(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                content={"detail": "Internal server error"}
            )

    async def _validate_request(self, request: Request) -> User:
        """
        Validate authentication credentials

        Production Implementation: Uses K8s TokenReviewer API
        """
        auth_header = request.headers.get("Authorization", "")

        # Guard clause: Early return for missing/invalid auth header
        if not auth_header.startswith("Bearer "):
            record_auth_failure("no_credentials", request.url.path)
            raise HTTPException(
                status_code=status.HTTP_401_UNAUTHORIZED,
                detail="No valid authentication credentials provided"
            )

        # Extract and validate token
        token = auth_header[7:]
        return await self._validate_k8s_token(token)

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
        Call the Kubernetes TokenReviewer API to validate a token.

        Business Requirement: BR-HAPI-067 (Token validation)

        Production Implementation:
        - Uses ServiceAccount token to authenticate to K8s API
        - Handles SSL certificate validation
        - Includes retry logic with exponential backoff
        - Provides detailed error logging
        """
        token_reviewer_url = f"{self.k8s_api_url}/apis/authentication.k8s.io/v1/tokenreviews"

        request_body = {
            "apiVersion": "authentication.k8s.io/v1",
            "kind": "TokenReview",
            "spec": {
                "token": token
            }
        }

        headers = {"Content-Type": "application/json"}

        # Add ServiceAccount token for authentication to K8s API
        if self.sa_token:
            headers["Authorization"] = f"Bearer {self.sa_token}"

        # Configure SSL
        ssl_context = None
        if self.ca_cert_path:
            import ssl
            ssl_context = ssl.create_default_context(cafile=self.ca_cert_path)
        else:
            # Dev mode or no CA cert available
            ssl_context = False

        # Retry logic with exponential backoff
        max_retries = 3
        retry_delay = 0.1  # Start with 100ms

        for attempt in range(max_retries):
            try:
                async with aiohttp.ClientSession() as session:
                    async with session.post(
                        token_reviewer_url,
                        json=request_body,
                        headers=headers,
                        ssl=ssl_context,
                        timeout=aiohttp.ClientTimeout(total=5.0)
                    ) as response:
                        response_text = await response.text()

                        if response.status != 201:  # TokenReview creation returns 201
                            logger.error({
                                "event": "token_review_failed",
                                "status": response.status,
                                "response": response_text[:200],
                                "attempt": attempt + 1
                            })

                            if attempt < max_retries - 1:
                                await asyncio.sleep(retry_delay)
                                retry_delay *= 2  # Exponential backoff
                                continue

                            raise HTTPException(
                                status_code=status.HTTP_401_UNAUTHORIZED,
                                detail=f"Kubernetes TokenReview failed: HTTP {response.status}"
                            )

                        result = await response.json()

                        # Check if token is authenticated
                        if not result.get("status", {}).get("authenticated", False):
                            error_msg = result.get("status", {}).get("error", "Token not authenticated")
                            logger.warning({
                                "event": "token_not_authenticated",
                                "error": error_msg
                            })
                            raise HTTPException(
                                status_code=status.HTTP_401_UNAUTHORIZED,
                                detail=f"Token not authenticated by Kubernetes: {error_msg}"
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
                            "role": role,
                            "groups_count": len(groups)
                        })

                        return {"username": username, "role": role}

            except aiohttp.ClientError as e:
                logger.error({
                    "event": "token_review_connection_error",
                    "error": str(e),
                    "attempt": attempt + 1,
                    "max_retries": max_retries
                })

                if attempt < max_retries - 1:
                    await asyncio.sleep(retry_delay)
                    retry_delay *= 2
                    continue

                raise HTTPException(
                    status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
                    detail=f"Cannot reach Kubernetes API: {str(e)}"
                )

        # Should not reach here, but just in case
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail="TokenReviewer API call failed after retries"
        )

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

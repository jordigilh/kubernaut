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
DataStorage Authentication Session

DD-AUTH-005: Client Authentication Pattern for DataStorage Service

This module provides a custom requests.Session that automatically injects
ServiceAccount tokens for authentication with the DataStorage service.

This is the Python equivalent of Go's pkg/shared/auth/transport.go.

Key Features:
- Reads ServiceAccount token from /var/run/secrets/kubernetes.io/serviceaccount/token
- Caches token for 5 minutes (reduces filesystem I/O)
- Injects Authorization: Bearer <token> header on every request
- Graceful degradation: If token file missing, requests proceed without auth
- Thread-safe: Multiple threads can share same session

Usage (Production/E2E with mounted ServiceAccount tokens):

    from datastorage import ApiClient, Configuration
    from datastorage_auth_session import ServiceAccountAuthSession

    # Create session with ServiceAccount token injection
    session = ServiceAccountAuthSession()

    # Configure DataStorage OpenAPI client with auth session
    config = Configuration(host="http://data-storage:8080")
    api_client = ApiClient(configuration=config)
    api_client.rest_client.pool_manager = session  # Inject auth session

    # All API calls now automatically include Authorization header
    audit_api = AuditWriteAPIApi(api_client)
    response = audit_api.create_audit_event(request)

ZERO TEST LOGIC: This production code contains no test-specific functionality.
For integration tests (mock user headers), use testutil.MockUserAuthSession().

Authority: DD-AUTH-005 (Authoritative client authentication pattern)
Related: pkg/shared/auth/transport.go (Go equivalent)
"""

import os
import time
import threading
from typing import Optional
import urllib3


class ServiceAccountAuthPoolManager(urllib3.PoolManager):
    """
    Custom urllib3.PoolManager that injects ServiceAccount tokens for DataStorage authentication.

    Behavior:
    - Reads token from /var/run/secrets/kubernetes.io/serviceaccount/token
    - Used by: holmesgpt-api service in production and E2E
    - Injects: Authorization: Bearer <token>
    - Caching: 5-minute cache to avoid filesystem reads on every request
    - Graceful degradation: If token file missing, request proceeds without auth

    Thread Safety:
    - request() is thread-safe (clones headers, no shared state mutation)
    - Token caching uses threading.Lock for concurrent access

    DD-AUTH-005: This pool manager enables holmesgpt-api to authenticate with
    DataStorage without modifying the OpenAPI-generated client code.

    ZERO TEST LOGIC: This production code contains no test-specific modes.
    For integration tests (mock user headers), use testutil.MockUserAuthSession().
    """

    # Class-level token cache (shared across instances)
    _token_cache: Optional[str] = None
    _token_cache_time: float = 0.0
    _token_cache_lock = threading.Lock()
    _cache_ttl_seconds = 300  # 5 minutes

    def __init__(
        self,
        token_path: str = "/var/run/secrets/kubernetes.io/serviceaccount/token",
        num_pools=10,
        headers=None,
        **connection_pool_kw
    ):
        """
        Initialize ServiceAccountAuthPoolManager.

        Args:
            token_path: Path to ServiceAccount token file
                       (default: /var/run/secrets/kubernetes.io/serviceaccount/token)
            num_pools: Number of connection pools (default: 10)
            headers: Optional default headers
            **connection_pool_kw: Additional arguments passed to urllib3.PoolManager
        """
        super().__init__(num_pools=num_pools, headers=headers, **connection_pool_kw)
        self._token_path = token_path

    def request(self, method, url, headers=None, **kwargs):
        """
        Override request() to inject Authorization header.

        This method is called for every HTTP request. It reads the ServiceAccount
        token (with caching) and injects the Authorization header before forwarding
        the request.

        Thread Safety: Safe for concurrent use (token cache uses threading.Lock).

        DD-AUTH-005: This method is called automatically for every DataStorage API call.
        Services using the OpenAPI-generated client don't need to know about authentication.
        """
        # Read token from filesystem (with 5-minute caching)
        token = self._get_service_account_token()
        if token:
            # Inject Authorization header (clone headers to avoid mutation)
            if headers is None:
                headers = {}
            else:
                # Clone headers to avoid mutating shared state
                headers = headers.copy()
            headers['Authorization'] = f'Bearer {token}'

        # Note: If token file doesn't exist, request proceeds without auth
        # This allows services to start before ServiceAccount token is mounted
        # Also allows local development without Kubernetes

        return super().request(method, url, headers=headers, **kwargs)

    def _get_service_account_token(self) -> Optional[str]:
        """
        Retrieve ServiceAccount token with 5-minute caching.
        Reduces filesystem reads from every request to once per 5 minutes.

        Thread Safety: Uses threading.Lock for concurrent access.

        Cache Strategy:
        1. Check cache with lock (fast path for cache hits)
        2. If cache miss or expired, read from filesystem
        3. Update cache
        4. Return token

        DD-AUTH-005: 5-minute cache balances performance (reduced filesystem I/O)
        with security (token rotation support).

        Returns:
            Token string if file exists and is readable, None otherwise
        """
        with self._token_cache_lock:
            # Check cache validity
            now = time.time()
            if (
                self._token_cache is not None
                and (now - self._token_cache_time) < self._cache_ttl_seconds
            ):
                return self._token_cache

            # Cache miss or expired - read from filesystem
            try:
                with open(self._token_path, 'r') as f:
                    token = f.read().strip()
                    # Update cache
                    self._token_cache = token
                    self._token_cache_time = now
                    return token
            except FileNotFoundError:
                # Graceful degradation: Token file doesn't exist yet
                # This is normal during pod startup or local development
                return None
            except PermissionError:
                # Security issue: Token file exists but can't be read
                # This should never happen in production
                return None
            except Exception as e:
                # Unexpected error: Log and continue without auth
                # Don't crash the service due to token read failure
                import logging
                logging.warning(
                    f"⚠️ DD-AUTH-005: Failed to read ServiceAccount token - "
                    f"path={self._token_path}, error={e}"
                )
                return None


# ========================================
# USAGE PATTERNS
# ========================================
#
# Production/E2E (ServiceAccount token from filesystem):
#
#   from datastorage import ApiClient, Configuration
#   from datastorage_auth_session import ServiceAccountAuthPoolManager
#
#   pool_manager = ServiceAccountAuthPoolManager()
#   config = Configuration(host="http://data-storage:8080")
#   api_client = ApiClient(configuration=config)
#   api_client.rest_client.pool_manager = pool_manager
#
#   audit_api = AuditWriteAPIApi(api_client)
#   response = audit_api.create_audit_event(request)
#
# Integration Tests (Mock user header, no oauth-proxy):
#
#   # Use testutil.MockUserAuthPoolManager() to avoid test logic in production code
#   from testutil import MockUserAuthPoolManager
#
#   pool_manager = MockUserAuthPoolManager(user_id="test-user@example.com")
#   config = Configuration(host="http://data-storage:8080")
#   api_client = ApiClient(configuration=config)
#   api_client.rest_client.pool_manager = pool_manager
#
# E2E Tests: Use SAME ServiceAccountAuthPoolManager as production
# (run tests in pods with mounted tokens)


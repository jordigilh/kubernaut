# Copyright 2025 Jordi Gil.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""
DataStorage Static Token Authentication Session (E2E Tests Only)

DD-AUTH-005: Client Authentication Pattern for DataStorage Service

This module provides a custom requests.Session that injects static tokens
(acquired externally) for E2E tests that run outside Kubernetes.

This is the Python equivalent of Go's pkg/testutil/auth_static_token.go.

Key Features:
- Injects static token (acquired via TokenRequest API or kubectl whoami -t)
- No token refresh (single-use token for E2E test duration)
- Thread-safe request interception
- E2E test-only (NOT for production use)

Usage (E2E Tests Only - External Test Runners):

    from datastorage import ApiClient, Configuration
    from testutil_static_token_session import StaticTokenAuthSession
    import subprocess

    # Acquire token externally (ServiceAccount or kubeadmin)
    # Option 1: ServiceAccount token via TokenRequest API
    token = get_service_account_token("datastorage-e2e-sa", "default", 3600)
    # Option 2: Kubeadmin token via kubectl
    token = subprocess.check_output(["kubectl", "whoami", "-t"]).decode().strip()

    # Create session with static token injection
    session = StaticTokenAuthSession(token)

    # Configure DataStorage OpenAPI client with auth session
    config = Configuration(host="http://localhost:30080")  # Kind NodePort
    api_client = ApiClient(configuration=config)
    api_client.rest_client.pool_manager = session  # Inject auth session

    # All API calls now automatically include Authorization header
    audit_api = AuditWriteAPIApi(api_client)
    response = audit_api.create_audit_event(request)

E2E Test Scenario:
- E2E tests: Run as pytest tests on host machine (NOT in pods)
- Target: Kind cluster NodePort → DataStorage pod (with oauth-proxy sidecar)
- No mounted ServiceAccount token available (tests run externally)
- Solution: Acquire token via TokenRequest API or kubectl, inject via this session

ZERO PRODUCTION LOGIC: This test-only code contains no production functionality.
For production use, see datastorage_auth_session.py (ServiceAccount tokens).

Authority: DD-AUTH-005 (Authoritative client authentication pattern)
Related: datastorage_auth_session.py (production session)
"""

from requests import Session
from requests.adapters import HTTPAdapter


class StaticTokenAuthSession(Session):
    """
    Custom requests.Session that injects static tokens for E2E tests running externally.

    Behavior:
    - Injects static token (acquired via TokenRequest API or kubectl)
    - Used by: holmesgpt-api E2E tests running externally (outside Kubernetes)
    - Injects: Authorization: Bearer <token>
    - No caching: Single static token for entire test duration
    - No refresh: Token must be valid for test duration

    Thread Safety:
    - request() is thread-safe (clones headers, no shared state mutation)
    - Token is immutable (set once at construction)

    DD-AUTH-005: This session enables holmesgpt-api E2E tests to authenticate
    with DataStorage without modifying the OpenAPI-generated client code.

    ZERO PRODUCTION LOGIC: This is test-only code.
    For production, use datastorage_auth_session.ServiceAccountAuthSession.
    """

    def __init__(
        self,
        token: str,
        **kwargs
    ):
        """
        Initialize StaticTokenAuthSession.

        Args:
            token: Static token (from TokenRequest API or kubectl whoami -t)
            **kwargs: Additional arguments passed to requests.Session
        """
        super().__init__(**kwargs)
        self._token = token

        # Configure connection pooling (same as production session)
        adapter = HTTPAdapter(
            pool_connections=100,
            pool_maxsize=100,
            pool_block=False
        )
        self.mount('http://', adapter)
        self.mount('https://', adapter)

    def request(self, method, url, **kwargs):
        """
        Override request() to inject Authorization header.

        This method is called for every HTTP request. It injects the static token
        provided at construction time.

        Thread Safety: Safe for concurrent use (token is immutable).

        DD-AUTH-005: This method is called automatically for every DataStorage API call.
        E2E tests using the OpenAPI-generated client don't need to know about authentication.
        """
        # Inject Authorization header (static token from E2E test setup)
        if 'headers' not in kwargs:
            kwargs['headers'] = {}
        if self._token:
            kwargs['headers']['Authorization'] = f'Bearer {self._token}'

        return super().request(method, url, **kwargs)


# ========================================
# USAGE PATTERNS
# ========================================
#
# E2E Tests (External - ServiceAccount token via TokenRequest API):
#
#   from datastorage import ApiClient, Configuration
#   from testutil_static_token_session import StaticTokenAuthSession
#
#   token = get_service_account_token("datastorage-e2e-sa", "default", 3600)
#   session = StaticTokenAuthSession(token)
#   config = Configuration(host="http://localhost:30080")
#   api_client = ApiClient(configuration=config)
#   api_client.rest_client.pool_manager = session
#
#   audit_api = AuditWriteAPIApi(api_client)
#   response = audit_api.create_audit_event(request)
#
# E2E Tests (External - Kubeadmin token via kubectl):
#
#   import subprocess
#   token = subprocess.check_output(["kubectl", "whoami", "-t"]).decode().strip()
#   session = StaticTokenAuthSession(token)
#   # ... same as above


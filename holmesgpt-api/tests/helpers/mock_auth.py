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
Mock authentication and authorization implementations for testing

Authority: DD-AUTH-014 (Middleware-Based SAR Authentication)

This module provides mock implementations of Authenticator and Authorizer
for integration tests where we want to validate authentication flow without
making real Kubernetes API calls.

Security Note: These mocks are ONLY for testing purposes.
Production code (main.py) always uses K8sAuthenticator/K8sAuthorizer with real Kubernetes APIs.

Design Pattern: Follows Go implementation from pkg/shared/auth/mock_auth.go

Business Requirements:
- BR-HAPI-067: JWT token authentication (mocked for tests)
- BR-HAPI-068: Role-based access control (mocked for tests)
"""

import logging
from typing import Dict, Optional

from fastapi import HTTPException, status

logger = logging.getLogger(__name__)


class MockAuthenticator:
    """
    Test double implementation of Authenticator.

    This mock is intended for integration tests where we want to validate authentication
    flow without making real Kubernetes API calls. The mock allows tests to control
    which tokens are valid and which users they map to.

    Authority: DD-AUTH-014

    Security Note: This mock is ONLY for testing purposes.
    Production code (main.py) always uses K8sAuthenticator with real Kubernetes APIs.

    Example usage in integration tests:

        authenticator = MockAuthenticator(
            valid_users={
                "test-token-authorized": "system:serviceaccount:test:authorized-sa",
                "test-token-readonly": "system:serviceaccount:test:readonly-sa",
            }
        )

        # Test with valid token
        user = await authenticator.validate_token("test-token-authorized")
        # Returns: "system:serviceaccount:test:authorized-sa"

        # Test with invalid token
        user = await authenticator.validate_token("invalid-token")
        # Raises: HTTPException(401)
    """

    def __init__(
        self,
        valid_users: Optional[Dict[str, str]] = None,
        error_to_return: Optional[Exception] = None,
    ):
        """
        Initialize the mock authenticator.

        Args:
            valid_users: Maps tokens to user identities.
                Key: token string
                Value: user identity (e.g., "system:serviceaccount:namespace:sa-name")
            error_to_return: If set, validate_token will raise this error
                instead of checking valid_users (useful for testing error handling)

        Example:
            # Mock with predefined users
            authenticator = MockAuthenticator(
                valid_users={
                    "token-1": "system:serviceaccount:test:sa-1",
                    "token-2": "system:serviceaccount:test:sa-2",
                }
            )

            # Mock that always fails (for error testing)
            authenticator = MockAuthenticator(
                error_to_return=HTTPException(503, "K8s API unavailable")
            )
        """
        self.valid_users = valid_users or {}
        self.error_to_return = error_to_return
        self.call_count = 0  # Track calls for testing caching behavior

    async def validate_token(self, token: str) -> str:
        """
        Validate token (mock implementation).

        Args:
            token: Bearer token string (without "Bearer " prefix)

        Returns:
            User identity if token is in valid_users map

        Raises:
            HTTPException: 401 if token is invalid or error_to_return is set
        """
        self.call_count += 1

        # Simulate API failure if configured
        if self.error_to_return:
            logger.warning({
                "event": "mock_auth_error_simulated",
                "error": str(self.error_to_return)
            })
            raise self.error_to_return

        # Check if token is in the valid users map
        if token in self.valid_users:
            username = self.valid_users[token]
            logger.info({
                "event": "mock_token_validated",
                "username": username,
                "call_count": self.call_count
            })
            return username

        # Token not found - return 401
        logger.warning({
            "event": "mock_token_invalid",
            "token_prefix": token[:10] if len(token) > 10 else token,
            "call_count": self.call_count
        })
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Invalid token (mock)",
        )


class MockAuthorizer:
    """
    Test double implementation of Authorizer.

    This mock is intended for integration tests where we want to validate authorization
    flow without making real Kubernetes SAR API calls. The mock allows tests to control
    which users have which permissions.

    Authority: DD-AUTH-014

    Security Note: This mock is ONLY for testing purposes.
    Production code (main.py) always uses K8sAuthorizer with real Kubernetes APIs.

    Example usage in integration tests:

        authorizer = MockAuthorizer(
            authorized_users={
                "system:serviceaccount:test:authorized-sa": True,
            },
            default_allow=False  # Deny by default
        )

        # Test with authorized user
        allowed = await authorizer.check_access(
            "system:serviceaccount:test:authorized-sa",
            "test-ns", "services", "test-service", "create"
        )
        # Returns: True

        # Test with unauthorized user
        allowed = await authorizer.check_access(
            "system:serviceaccount:test:unauthorized-sa",
            "test-ns", "services", "test-service", "create"
        )
        # Returns: False
    """

    def __init__(
        self,
        authorized_users: Optional[Dict[str, bool]] = None,
        default_allow: bool = True,
        error_to_return: Optional[Exception] = None,
    ):
        """
        Initialize the mock authorizer.

        Args:
            authorized_users: Maps user identities to authorization status.
                Key: user identity (e.g., "system:serviceaccount:namespace:sa-name")
                Value: True if authorized, False if denied
            default_allow: Default authorization for users not in authorized_users map.
                True = allow all (permissive, for development)
                False = deny all (restrictive, for testing denial scenarios)
            error_to_return: If set, check_access will raise this error
                instead of checking authorized_users (useful for testing error handling)

        Example:
            # Mock with specific permissions
            authorizer = MockAuthorizer(
                authorized_users={
                    "system:serviceaccount:test:admin": True,
                    "system:serviceaccount:test:readonly": False,
                },
                default_allow=False  # Deny unknown users
            )

            # Mock that always fails (for error testing)
            authorizer = MockAuthorizer(
                error_to_return=HTTPException(503, "K8s API unavailable")
            )
        """
        self.authorized_users = authorized_users or {}
        self.default_allow = default_allow
        self.error_to_return = error_to_return
        self.call_count = 0  # Track calls for testing caching behavior

    async def check_access(
        self,
        user: str,
        namespace: str,
        resource: str,
        resource_name: str,
        verb: str,
    ) -> bool:
        """
        Check authorization (mock implementation).

        Args:
            user: User identity from token validation
            namespace: Kubernetes namespace
            resource: Resource type (e.g., "services")
            resource_name: Specific resource name
            verb: RBAC verb (e.g., "create")

        Returns:
            True if user is in authorized_users map with value True,
            or if default_allow is True and user not in map.
            False otherwise.

        Raises:
            HTTPException: 500 if error_to_return is set
        """
        self.call_count += 1

        # Simulate API failure if configured
        if self.error_to_return:
            logger.warning({
                "event": "mock_authz_error_simulated",
                "error": str(self.error_to_return)
            })
            raise self.error_to_return

        # Check if user has explicit authorization status
        if user in self.authorized_users:
            allowed = self.authorized_users[user]
            logger.info({
                "event": "mock_authz_explicit",
                "user": user,
                "namespace": namespace,
                "resource": resource,
                "resource_name": resource_name,
                "verb": verb,
                "allowed": allowed,
                "call_count": self.call_count
            })
            return allowed

        # Use default authorization
        logger.info({
            "event": "mock_authz_default",
            "user": user,
            "namespace": namespace,
            "resource": resource,
            "resource_name": resource_name,
            "verb": verb,
            "allowed": self.default_allow,
            "call_count": self.call_count
        })
        return self.default_allow

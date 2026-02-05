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
Authentication and Authorization interfaces using Python Protocols

Authority: DD-AUTH-014 (Middleware-Based SAR Authentication)

This module defines the contracts for authentication and authorization components
using Python's Protocol (structural subtyping - similar to Go interfaces).

Design Decision: Use Protocol instead of ABC (Abstract Base Class) to enable
structural typing (duck typing) like Go interfaces. This allows for more flexible
testing and dependency injection.

Business Requirements:
- BR-HAPI-066: API key authentication
- BR-HAPI-067: JWT token authentication
- BR-HAPI-068: Role-based access control
"""

from typing import Protocol


class Authenticator(Protocol):
    """
    Validates tokens and returns user identity.

    Implementations:
      - K8sAuthenticator: Uses Kubernetes TokenReview API (production/E2E)
      - MockAuthenticator: Test double for integration tests

    Example usage:

        authenticator = K8sAuthenticator(k8s_client)
        user = await authenticator.validate_token("eyJhbGc...")
        if not user:
            # Return 401 Unauthorized
    """

    async def validate_token(self, token: str) -> str:
        """
        Check if the token is valid and return the user identity.

        This method validates ServiceAccount Bearer tokens by making TokenReview
        API calls to the Kubernetes API server.

        Args:
            token: Bearer token string (without "Bearer " prefix)

        Returns:
            User identity string (e.g., "system:serviceaccount:namespace:sa-name")

        Raises:
            HTTPException: Token validation failure (401 Unauthorized)
            - Token is invalid or expired
            - Token cannot be authenticated
            - Kubernetes API call fails

        Authority: https://kubernetes.io/docs/reference/kubernetes-api/authentication-resources/token-review-v1/
        """
        ...


class Authorizer(Protocol):
    """
    Checks if a user has permission to perform an action on a resource.

    Implementations:
      - K8sAuthorizer: Uses Kubernetes SubjectAccessReview (SAR) API (production/E2E)
      - MockAuthorizer: Test double for integration tests

    Example usage:

        authorizer = K8sAuthorizer(k8s_client)
        allowed = await authorizer.check_access(
            "system:serviceaccount:kubernaut-system:holmesgpt-api",
            "kubernaut-system",           # namespace
            "services",                   # resource
            "holmesgpt-api-service",      # resourceName
            "create",                     # verb
        )
        if not allowed:
            # Return 403 Forbidden
    """

    async def check_access(
        self,
        user: str,
        namespace: str,
        resource: str,
        resource_name: str,
        verb: str,
    ) -> bool:
        """
        Verify if the user has the required permissions.

        This method performs a Kubernetes SubjectAccessReview (SAR) check to determine
        if the specified user can perform the given verb on the specified resource.

        Args:
            user: User identity from token validation (e.g., "system:serviceaccount:ns:sa")
            namespace: Kubernetes namespace for the resource
            resource: Resource type (e.g., "services", "pods", "deployments")
            resource_name: Specific resource name (e.g., "holmesgpt-api-service")
            verb: RBAC verb (e.g., "create", "get", "list", "update", "delete")

        Returns:
            bool: True if access is allowed, False if denied

        Raises:
            HTTPException: SAR API call failure (not authorization denial)
            - Kubernetes API call fails
            - Invalid parameters
            - Network timeout

        Note:
            Authorization denial (user lacks permissions) returns False, not an exception.

        Authority: https://kubernetes.io/docs/reference/kubernetes-api/authorization-resources/subject-access-review-v1/
        """
        ...

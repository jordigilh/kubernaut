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
Kubernetes-based authentication and authorization implementations

Authority: DD-AUTH-014 (Middleware-Based SAR Authentication)

This module provides production implementations of Authenticator and Authorizer
using real Kubernetes TokenReview and SubjectAccessReview APIs.

These implementations are used in:
- Production environments (OpenShift/Kubernetes)
- E2E tests (Kind clusters with real K8s APIs)

Design Pattern: Follows Go implementation from pkg/shared/auth/k8s_auth.go

Business Requirements:
- BR-HAPI-067: JWT token authentication (via TokenReview)
- BR-HAPI-068: Role-based access control (via SAR)
"""

import logging
from typing import Optional

from kubernetes import client
from kubernetes.client.rest import ApiException
from fastapi import HTTPException, status

logger = logging.getLogger(__name__)


class K8sAuthenticator:
    """
    Validates ServiceAccount tokens using Kubernetes TokenReview API.

    This implementation is suitable for production and E2E environments where
    a real Kubernetes API server is available.

    Authority: DD-AUTH-014, BR-HAPI-067
    Reference: https://kubernetes.io/docs/reference/kubernetes-api/authentication-resources/token-review-v1/
    """

    def __init__(self, k8s_client: Optional[client.ApiClient] = None):
        """
        Initialize the K8s authenticator.

        Args:
            k8s_client: Kubernetes API client. If None, will load from in-cluster config.

        Example:
            # Production (in-cluster)
            authenticator = K8sAuthenticator()

            # Testing (with custom client)
            from kubernetes import client, config
            config.load_kube_config()
            k8s_api_client = client.ApiClient()
            authenticator = K8sAuthenticator(k8s_api_client)
        """
        if k8s_client is None:
            # Load in-cluster configuration (production)
            from kubernetes import config
            config.load_incluster_config()
            k8s_client = client.ApiClient()

        self.auth_api = client.AuthenticationV1Api(k8s_client)

    async def validate_token(self, token: str) -> str:
        """
        Validate a ServiceAccount token using Kubernetes TokenReview API.

        This method:
        1. Creates a TokenReview request with the provided token
        2. Sends the request to the Kubernetes API server
        3. Returns the authenticated user identity if valid

        Args:
            token: Bearer token string (without "Bearer " prefix)

        Returns:
            User identity (e.g., "system:serviceaccount:namespace:sa-name")

        Raises:
            HTTPException: 401 if token is invalid or authentication fails

        Authority: DD-AUTH-014
        """
        if not token:
            logger.warning({"event": "token_validation_failed", "reason": "empty_token"})
            raise HTTPException(
                status_code=status.HTTP_401_UNAUTHORIZED,
                detail="Token cannot be empty",
            )

        # Create TokenReview request
        # Authority: https://kubernetes.io/docs/reference/kubernetes-api/authentication-resources/token-review-v1/
        token_review = client.V1TokenReview(
            spec=client.V1TokenReviewSpec(token=token)
        )

        try:
            # Call Kubernetes TokenReview API
            result = self.auth_api.create_token_review(body=token_review)

            # Check if token is authenticated
            if not result.status.authenticated:
                error_msg = result.status.error or "Token not authenticated"
                logger.warning({
                    "event": "token_not_authenticated",
                    "error": error_msg
                })
                raise HTTPException(
                    status_code=status.HTTP_401_UNAUTHORIZED,
                    detail=f"Token not authenticated: {error_msg}",
                )

            # Check if user info is present
            if not result.status.user or not result.status.user.username:
                logger.error({
                    "event": "token_authenticated_but_no_user",
                    "authenticated": result.status.authenticated
                })
                raise HTTPException(
                    status_code=status.HTTP_401_UNAUTHORIZED,
                    detail="Token authenticated but user identity is empty",
                )

            username = result.status.user.username
            logger.info({
                "event": "token_validated",
                "username": username,
                "groups_count": len(result.status.user.groups or [])
            })

            return username

        except ApiException as e:
            logger.error({
                "event": "token_review_api_error",
                "error": str(e),
                "status": e.status
            })
            raise HTTPException(
                status_code=status.HTTP_401_UNAUTHORIZED,
                detail=f"Token validation failed: {e.reason}",
            )
        except HTTPException:
            # Re-raise HTTPException as-is
            raise
        except Exception as e:
            logger.error({
                "event": "token_validation_unexpected_error",
                "error": str(e),
                "error_type": type(e).__name__
            })
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail=f"Unexpected error during token validation: {str(e)}",
            )


class K8sAuthorizer:
    """
    Checks authorization using Kubernetes SubjectAccessReview (SAR) API.

    This implementation is suitable for production and E2E environments where
    a real Kubernetes API server is available.

    Authority: DD-AUTH-014, BR-HAPI-068
    Reference: https://kubernetes.io/docs/reference/kubernetes-api/authorization-resources/subject-access-review-v1/
    """

    def __init__(self, k8s_client: Optional[client.ApiClient] = None):
        """
        Initialize the K8s authorizer.

        Args:
            k8s_client: Kubernetes API client. If None, will load from in-cluster config.

        Example:
            # Production (in-cluster)
            authorizer = K8sAuthorizer()

            # Testing (with custom client)
            from kubernetes import client, config
            config.load_kube_config()
            k8s_api_client = client.ApiClient()
            authorizer = K8sAuthorizer(k8s_api_client)
        """
        if k8s_client is None:
            # Load in-cluster configuration (production)
            from kubernetes import config
            config.load_incluster_config()
            k8s_client = client.ApiClient()

        self.authz_api = client.AuthorizationV1Api(k8s_client)

    async def check_access(
        self,
        user: str,
        namespace: str,
        resource: str,
        resource_name: str,
        verb: str,
    ) -> bool:
        """
        Verify if the user has the required permissions using SAR.

        This method performs a Kubernetes SubjectAccessReview (SAR) check to determine
        if the specified user can perform the given verb on the specified resource.

        Args:
            user: User identity from token validation (e.g., "system:serviceaccount:ns:sa")
            namespace: Kubernetes namespace for the resource
            resource: Resource type (e.g., "services", "pods", "deployments")
            resource_name: Specific resource name (e.g., "holmesgpt-api-service")
            verb: RBAC verb (e.g., "create", "get", "list", "update", "delete")

        Returns:
            True if access is allowed, False if denied

        Raises:
            HTTPException: 500 if SAR API call fails (not authorization denial)

        Authority: DD-AUTH-014
        """
        # Validate parameters
        if not all([user, namespace, resource, resource_name, verb]):
            logger.error({
                "event": "sar_invalid_parameters",
                "user": user,
                "namespace": namespace,
                "resource": resource,
                "resource_name": resource_name,
                "verb": verb
            })
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail="Invalid SAR parameters: all fields required",
            )

        # Create SubjectAccessReview request
        # Authority: https://kubernetes.io/docs/reference/kubernetes-api/authorization-resources/subject-access-review-v1/
        sar = client.V1SubjectAccessReview(
            spec=client.V1SubjectAccessReviewSpec(
                user=user,
                resource_attributes=client.V1ResourceAttributes(
                    namespace=namespace,
                    resource=resource,
                    name=resource_name,
                    verb=verb,
                )
            )
        )

        try:
            # Call Kubernetes SubjectAccessReview API
            result = self.authz_api.create_subject_access_review(body=sar)

            # Check authorization result
            allowed = result.status.allowed

            logger.info({
                "event": "sar_check_completed",
                "user": user,
                "namespace": namespace,
                "resource": resource,
                "resource_name": resource_name,
                "verb": verb,
                "allowed": allowed,
                "reason": result.status.reason or "none"
            })

            return allowed

        except ApiException as e:
            logger.error({
                "event": "sar_api_error",
                "error": str(e),
                "status": e.status,
                "user": user,
                "namespace": namespace,
                "resource": resource,
                "resource_name": resource_name,
                "verb": verb
            })
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail=f"Authorization check failed: {e.reason}",
            )
        except Exception as e:
            logger.error({
                "event": "sar_unexpected_error",
                "error": str(e),
                "error_type": type(e).__name__,
                "user": user,
                "namespace": namespace,
                "resource": resource,
                "resource_name": resource_name,
                "verb": verb
            })
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail=f"Unexpected error during authorization check: {str(e)}",
            )

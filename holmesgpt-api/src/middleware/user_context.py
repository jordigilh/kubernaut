#
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
#

"""
User Context Extraction - Auth Middleware Integration

Authority: DD-AUTH-014 (Middleware-Based SAR Authentication)

This module provides user context extraction from the auth middleware's
validated user identity (stored in request.state.user).

Design Decision: DD-AUTH-014 - Auth via Middleware with Dependency Injection
- Auth middleware validates ServiceAccount tokens (TokenReview API)
- Auth middleware checks authorization (SubjectAccessReview API)
- Auth middleware stores validated user in request.state.user
- This function extracts user identity for logging/audit enrichment

Use Cases:
- LLM cost tracking: Which service is using LLM?
- Security auditing: Detect misuse patterns
- SOC2 readiness: User attribution for audit events
"""

import logging
from fastapi import Request

logger = logging.getLogger(__name__)


def get_authenticated_user(request: Request) -> str:
    """
    Extract authenticated user from auth middleware's validated identity.

    Authority: DD-AUTH-014 (Middleware-based SAR authentication)

    The auth middleware validates tokens via TokenReview and checks permissions
    via SAR, then stores the validated user identity in request.state.user.
    This function extracts that identity for logging/audit purposes.

    Args:
        request: FastAPI Request object with request.state.user set by auth middleware

    Returns:
        str: Authenticated user identity
        - Production: system:serviceaccount:kubernaut-system:gateway-sa
        - Integration tests: system:serviceaccount:test:test-sa (from mock auth)
        - Missing identity: "unknown" (should not happen if auth middleware is enabled)

    Example Usage:
        ```python
        from fastapi import Request
        from src.middleware.user_context import get_authenticated_user

        async def my_endpoint(request: Request, ...):
            user = get_authenticated_user(request)
            logger.info({"action": "incident_analysis", "user": user})
            # ... business logic ...
        ```

    Authority: DD-AUTH-014 (HAPI middleware-based authentication)
    Related: DD-AUTH-014 (DataStorage middleware pattern)
    """
    # Extract user from request.state (set by auth middleware after TokenReview)
    user = getattr(request.state, "user", None) or "unknown"

    # Log warning if user missing (should not happen in production with auth enabled)
    if user == "unknown":
        logger.warning({
            "event": "missing_user_identity",
            "path": request.url.path,
            "note": "Auth middleware should set request.state.user after TokenReview"
        })

    return user


def get_user_for_audit(request: Request) -> dict:
    """
    Extract user context for audit events.

    Authority: DD-AUTH-014 (Middleware-based SAR authentication)

    Enriches audit events with validated user attribution from the auth middleware.

    Args:
        request: FastAPI Request object with request.state.user set by auth middleware

    Returns:
        dict: User context for audit events
        {
            "user_id": "system:serviceaccount:kubernaut-system:gateway-sa",
            "user_type": "service_account"
        }

    Example Usage:
        ```python
        audit_event = {
            "event_type": "hapi.incident.analyzed",
            "event_category": "ai_analysis",
            **get_user_for_audit(request),  # Inject user context
            # ... other audit fields ...
        }
        ```

    Authority: DD-AUTH-014 (HAPI middleware-based authentication)
    """
    user = get_authenticated_user(request)

    # Determine user type from identity format
    user_type = "unknown"
    if user.startswith("system:serviceaccount:"):
        user_type = "service_account"
    elif user.startswith("system:"):
        user_type = "system_user"
    elif "@" in user:
        user_type = "integration_test"

    return {
        "user_id": user,
        "user_type": user_type
    }


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
User Context Extraction - OAuth-Proxy Integration

DD-AUTH-006: Extract authenticated user from oauth-proxy injected header

This module provides simple user context extraction WITHOUT implementing
authentication logic. Authentication is handled externally by oauth-proxy sidecar.

Design Decision: DD-AUTH-006 - Auth Outside Business Logic
- OAuth-proxy handles authentication/authorization (sidecar pattern)
- Python code ONLY extracts user identity for logging/audit
- NO token validation, NO RBAC checks (oauth-proxy handles these)
- auth_enabled remains False (no Python auth middleware)

Use Cases:
- LLM cost tracking: Which service is using LLM?
- Security auditing: Detect misuse patterns
- Future SOC2 readiness: User attribution for audit events
"""

import logging
from fastapi import Request
from typing import Optional

logger = logging.getLogger(__name__)

# DD-AUTH-006: OAuth-proxy header name
# This is injected by oauth-proxy after successful authentication/authorization
OAUTH_PROXY_USER_HEADER = "X-Auth-Request-User"


def get_authenticated_user(request: Request) -> str:
    """
    Extract authenticated user from oauth-proxy injected header.
    
    DD-AUTH-006: OAuth-proxy integration pattern
    - OAuth-proxy validates ServiceAccount token
    - OAuth-proxy performs Subject Access Review (SAR)
    - OAuth-proxy injects X-Auth-Request-User header
    - This function extracts header for logging/audit
    
    Args:
        request: FastAPI Request object
    
    Returns:
        str: Authenticated user identity
        - Production: system:serviceaccount:kubernaut-system:gateway-sa
        - Integration tests: test-service@integration.test (mock header)
        - Missing header: "unknown" (should not happen in production)
    
    Example Usage:
        ```python
        from fastapi import Request
        from src.middleware.user_context import get_authenticated_user
        
        async def my_endpoint(request: Request, ...):
            user = get_authenticated_user(request)
            logger.info({"action": "incident_analysis", "user": user})
            # ... business logic ...
        ```
    
    Authority: DD-AUTH-006 (HAPI oauth-proxy integration)
    Related: DD-AUTH-004 (DataStorage oauth-proxy pattern)
    """
    user = request.headers.get(OAUTH_PROXY_USER_HEADER, "unknown")
    
    # Log warning if header missing (should not happen in production)
    if user == "unknown":
        logger.warning({
            "event": "missing_user_header",
            "header": OAUTH_PROXY_USER_HEADER,
            "path": request.url.path,
            "note": "OAuth-proxy should inject this header after authentication"
        })
    
    return user


def get_user_for_audit(request: Request) -> dict:
    """
    Extract user context for audit events.
    
    DD-AUTH-006: Audit event enrichment with user attribution
    
    Args:
        request: FastAPI Request object
    
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
    
    Authority: DD-AUTH-006 (HAPI oauth-proxy integration)
    """
    user = get_authenticated_user(request)
    
    # Determine user type from identity format
    user_type = "unknown"
    if user.startswith("system:serviceaccount:"):
        user_type = "service_account"
    elif "@integration.test" in user:
        user_type = "integration_test"
    
    return {
        "user_id": user,
        "user_type": user_type
    }


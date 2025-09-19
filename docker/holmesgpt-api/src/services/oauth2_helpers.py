"""
OAuth 2 Resource Server Helper Functions - Refactoring Phase
Common utilities and helper functions for OAuth 2 and K8s integration
"""

import base64
import json
import time
from typing import Dict, Any, Optional, List, Callable
from datetime import datetime, timedelta
from functools import wraps

import structlog

from services.oauth2_constants import (
    JWT_HEADER_RS256, JWT_PADDING_CHAR, JWT_PADDING_MODULO,
    K8S_ISSUER, DEFAULT_K8S_AUDIENCE, ERROR_MESSAGES
)

logger = structlog.get_logger(__name__)

# =====================================================
# JWT Token Creation Helpers
# =====================================================

def create_k8s_jwt_token(
    service_account_name: str,
    namespace: str = "default",
    audience: Optional[List[str]] = None,
    expiration_hours: int = 24,
    expired: bool = False
) -> str:
    """
    Create a properly formatted Kubernetes ServiceAccount JWT token for testing.

    Args:
        service_account_name: Name of the ServiceAccount
        namespace: Kubernetes namespace (default: "default")
        audience: List of audiences (default: [DEFAULT_K8S_AUDIENCE])
        expiration_hours: Hours until expiration (default: 24)
        expired: If True, create an expired token (default: False)

    Returns:
        Formatted JWT token string
    """
    if audience is None:
        audience = [DEFAULT_K8S_AUDIENCE]

    # Calculate expiration timestamp
    if expired:
        exp_timestamp = int(time.time()) - 3600  # Expired 1 hour ago
    else:
        exp_timestamp = int(time.time()) + (expiration_hours * 3600)

    payload = {
        "iss": K8S_ISSUER,
        "sub": f"system:serviceaccount:{namespace}:{service_account_name}",
        "aud": audience,
        "exp": exp_timestamp
    }

    # Encode payload to base64
    payload_b64 = base64.urlsafe_b64encode(
        json.dumps(payload).encode()
    ).decode().rstrip('=')

    # Return formatted JWT token
    return f"{JWT_HEADER_RS256}.{payload_b64}.signature"

def create_bearer_header(token: str) -> str:
    """
    Create Authorization header with Bearer token.

    Args:
        token: The token to include in the header

    Returns:
        Formatted Authorization header string
    """
    return f"Bearer {token}"

# =====================================================
# JWT Token Parsing Helpers
# =====================================================

def decode_jwt_payload(token: str) -> Optional[Dict[str, Any]]:
    """
    Decode JWT payload without signature verification (for testing/local use).

    Args:
        token: JWT token string

    Returns:
        Decoded payload dictionary or None if invalid
    """
    try:
        # Split JWT token to check format
        parts = token.split('.')
        if len(parts) != 3:
            logger.debug(ERROR_MESSAGES["invalid_jwt_format"])
            return None

        # Decode payload with proper padding
        payload_part = parts[1]
        padding = JWT_PADDING_MODULO - len(payload_part) % JWT_PADDING_MODULO
        if padding != JWT_PADDING_MODULO:
            payload_part += JWT_PADDING_CHAR * padding

        payload_bytes = base64.urlsafe_b64decode(payload_part)
        payload = json.loads(payload_bytes)

        return payload

    except Exception as e:
        logger.debug(ERROR_MESSAGES["failed_jwt_decode"], error=str(e))
        return None

def extract_subject_parts(subject: str) -> Optional[tuple[str, str]]:
    """
    Extract namespace and service account name from K8s subject.

    Args:
        subject: ServiceAccount subject string (format: system:serviceaccount:namespace:name)

    Returns:
        Tuple of (namespace, service_account_name) or None if invalid
    """
    if not subject or not subject.startswith('system:serviceaccount:'):
        logger.debug(ERROR_MESSAGES["invalid_sa_subject"])
        return None

    # Extract namespace and service account name from subject
    # Format: system:serviceaccount:namespace:name
    subject_parts = subject.split(':')
    if len(subject_parts) != 4:
        logger.debug(ERROR_MESSAGES["invalid_sa_subject_format"])
        return None

    namespace = subject_parts[2]
    sa_name = subject_parts[3]

    return namespace, sa_name

# =====================================================
# Validation Helpers
# =====================================================

def is_token_expired(exp_timestamp: Optional[int]) -> bool:
    """
    Check if token has expired based on timestamp.

    Args:
        exp_timestamp: Expiration timestamp or None

    Returns:
        True if token is expired, False otherwise
    """
    if not exp_timestamp:
        return False  # No expiration set

    expires_at = datetime.fromtimestamp(exp_timestamp)
    current_time = datetime.utcnow()

    if current_time >= expires_at:
        logger.debug(ERROR_MESSAGES["token_expired"],
                    current_time=current_time.isoformat(),
                    expires_at=expires_at.isoformat())
        return True

    return False

def extract_bearer_token(auth_header: str) -> Optional[str]:
    """
    Extract token from Authorization header.

    Args:
        auth_header: Authorization header value

    Returns:
        Extracted token or None if invalid format
    """
    if not auth_header or not auth_header.startswith("Bearer "):
        logger.debug(ERROR_MESSAGES["invalid_auth_header"])
        return None

    return auth_header[7:]  # Remove "Bearer " prefix

# =====================================================
# Error Handling Decorators
# =====================================================

def handle_validation_errors(error_message: str = "Validation failed"):
    """
    Decorator to handle common validation errors and logging.

    Args:
        error_message: Custom error message for logging
    """
    def decorator(func: Callable) -> Callable:
        @wraps(func)
        def wrapper(*args, **kwargs):
            try:
                return func(*args, **kwargs)
            except Exception as e:
                logger.error(error_message, error=str(e))
                return None
        return wrapper
    return decorator

def log_validation_result(success_message: str, failure_message: str):
    """
    Decorator to log validation results consistently.

    Args:
        success_message: Message to log on successful validation
        failure_message: Message to log on failed validation
    """
    def decorator(func: Callable) -> Callable:
        @wraps(func)
        def wrapper(*args, **kwargs):
            result = func(*args, **kwargs)

            if result:
                logger.debug(success_message)
            else:
                logger.debug(failure_message)

            return result
        return wrapper
    return decorator

# =====================================================
# Scope Manipulation Helpers
# =====================================================

def format_subject_string(namespace: str, service_account_name: str) -> str:
    """
    Format Kubernetes ServiceAccount subject string.

    Args:
        namespace: Kubernetes namespace
        service_account_name: ServiceAccount name

    Returns:
        Formatted subject string
    """
    return f"system:serviceaccount:{namespace}:{service_account_name}"

def scope_values_to_list(scopes: List[Any]) -> List[str]:
    """
    Convert list of scope objects to list of scope value strings.

    Args:
        scopes: List of scope objects with .value attribute

    Returns:
        List of scope value strings
    """
    return [s.value if hasattr(s, 'value') else str(s) for s in scopes]

# =====================================================
# Settings Access Helpers
# =====================================================

def get_setting_with_default(settings: Any, key: str, default: Any) -> Any:
    """
    Safely get setting value with default fallback.

    Args:
        settings: Settings object
        key: Setting key to retrieve
        default: Default value if key not found

    Returns:
        Setting value or default
    """
    return getattr(settings, key, default)

# =====================================================
# ServiceAccount Pattern Matching
# =====================================================

def matches_pattern(text: str, patterns: List[str]) -> bool:
    """
    Check if text matches any of the provided patterns.

    Args:
        text: Text to check
        patterns: List of patterns to match against

    Returns:
        True if any pattern matches, False otherwise
    """
    text_lower = text.lower()
    return any(pattern.lower() in text_lower for pattern in patterns)

def get_full_serviceaccount_name(namespace: str, sa_name: str) -> str:
    """
    Get full ServiceAccount name for pattern matching.

    Args:
        namespace: Kubernetes namespace
        sa_name: ServiceAccount name

    Returns:
        Full ServiceAccount name (namespace:name)
    """
    return f"{namespace}:{sa_name}"

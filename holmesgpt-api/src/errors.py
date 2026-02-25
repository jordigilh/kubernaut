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
Comprehensive Error Handling for HolmesGPT API Service

Business Requirements:
- BR-HAPI-146 to 165 (Error Handling)
- BR-HAPI-200 (RFC 7807 Error Response Standard)

REFACTOR phase: Production-grade error handling with circuit breaker pattern.
Design Decision: DD-HOLMESGPT-011, DD-HOLMESGPT-012, DD-004 (RFC 7807)
"""

import logging
from typing import Optional, Dict, Any
from datetime import datetime, timedelta, timezone
from pydantic import BaseModel

logger = logging.getLogger(__name__)


# ========================================
# RFC 7807 PROBLEM DETAILS FOR HTTP APIs
# ========================================

class HTTPError(BaseModel):
    """
    RFC 7807 Problem Details for HTTP APIs

    Business Requirement: BR-HAPI-200 - RFC 7807 Error Response Standard
    Design Decision: DD-004 - RFC 7807 Problem Details

    Reference: https://tools.ietf.org/html/rfc7807
    Reference: Gateway Service (pkg/gateway/errors/rfc7807.go)
    Reference: Context API (pkg/contextapi/errors/rfc7807.go)
    Reference: Dynamic Toolset (pkg/toolset/errors/rfc7807.go)

    Named HTTPError in OpenAPI spec for compatibility with existing Go client.
    """
    type: str  # URI reference identifying the problem type
    title: str  # Short, human-readable summary
    detail: str  # Detailed explanation
    status: int  # HTTP status code
    instance: str  # URI reference to specific occurrence
    request_id: Optional[str] = None  # Request tracing identifier (RFC 7807 extension member)

    class Config:
        json_schema_extra = {
            "example": {
                "type": "https://kubernaut.ai/problems/validation-error",
                "title": "Bad Request",
                "detail": "Missing required field: 'namespace'",
                "status": 400,
                "instance": "/api/v1/incident/analyze",
                "request_id": "abc-123-def-456"
            }
        }


# Backward-compatible alias
RFC7807Error = HTTPError


# Shared error responses for OpenAPI spec (application/problem+json)
# Used by incident endpoint to ensure consistent error schema
# Authority: BR-HAPI-200 (RFC 7807 Error Response Standard), DD-AUTH-014 (Middleware-Based SAR Authentication)
PROBLEM_JSON_ERROR_RESPONSES = {
    401: {
        "description": "Authentication failed - Invalid or missing Bearer token.\n\n"
                       "**Source**: HAPI middleware (DD-AUTH-014)\n\n"
                       "**Cause**: No Authorization header, invalid token, expired token, or malformed token.\n\n"
                       "**Authority**: DD-AUTH-014 (Middleware-Based SAR Authentication)",
        "model": HTTPError,
        "content": {"application/problem+json": {"schema": HTTPError.model_json_schema()}},
    },
    403: {
        "description": "Authorization failed - Kubernetes SubjectAccessReview (SAR) denied access.\n\n"
                       "**Source**: HAPI middleware (DD-AUTH-014)\n\n"
                       "**Cause**: ServiceAccount lacks required SAR permissions.\n\n"
                       "**Resolution**: Grant ServiceAccount the holmesgpt-api-client ClusterRole.\n\n"
                       "**Authority**: DD-AUTH-014 (Middleware-Based SAR Authentication)",
        "model": HTTPError,
        "content": {"application/problem+json": {"schema": HTTPError.model_json_schema()}},
    },
    400: {
        "description": "Bad Request - Invalid request parameters or validation failure.\n\n"
                       "**Source**: HAPI middleware (RFC 7807)\n\n"
                       "**Cause**: Pydantic validation error, missing required fields, invalid field values.\n\n"
                       "**Authority**: BR-HAPI-200 (RFC 7807 Error Response Standard)",
        "model": HTTPError,
        "content": {"application/problem+json": {"schema": HTTPError.model_json_schema()}},
    },
    422: {
        "description": "Unprocessable Entity - Request validation failed.\n\n"
                       "**Source**: HAPI middleware (RFC 7807)\n\n"
                       "**Cause**: Pydantic validation error.\n\n"
                       "**Authority**: BR-HAPI-200 (RFC 7807 Error Response Standard)",
        "model": HTTPError,
        "content": {"application/problem+json": {"schema": HTTPError.model_json_schema()}},
    },
    500: {
        "description": "Internal server error - Unexpected failure in HolmesGPT API service.\n\n"
                       "**Source**: HAPI application\n\n"
                       "**Causes**: LLM provider API failure, database error, unhandled exception.\n\n"
                       "**Authority**: BR-HAPI-200 (RFC 7807 Error Response Standard)",
        "model": HTTPError,
        "content": {"application/problem+json": {"schema": HTTPError.model_json_schema()}},
    },
}


# Error type URI constants
# BR-HAPI-200: RFC 7807 error format
# Updated: December 18, 2025 - Changed domain from kubernaut.io to kubernaut.ai
#                              Changed path from /errors/ to /problems/ (RFC 7807 standard)
ERROR_TYPE_VALIDATION_ERROR = "https://kubernaut.ai/problems/validation-error"
ERROR_TYPE_UNAUTHORIZED = "https://kubernaut.ai/problems/unauthorized"
ERROR_TYPE_NOT_FOUND = "https://kubernaut.ai/problems/not-found"
ERROR_TYPE_INTERNAL_ERROR = "https://kubernaut.ai/problems/internal-error"
ERROR_TYPE_SERVICE_UNAVAILABLE = "https://kubernaut.ai/problems/service-unavailable"


def create_rfc7807_error(
    status_code: int,
    detail: str,
    instance: str,
    request_id: Optional[str] = None
) -> RFC7807Error:
    """
    Create RFC 7807 error response

    Business Requirement: BR-HAPI-200

    Maps HTTP status codes to RFC 7807 error types and titles.
    """
    error_type, title = _get_error_type_and_title(status_code)

    return RFC7807Error(
        type=error_type,
        title=title,
        detail=detail,
        status=status_code,
        instance=instance,
        request_id=request_id
    )


def _get_error_type_and_title(status_code: int) -> tuple[str, str]:
    """Map HTTP status codes to RFC 7807 error types and titles"""
    mapping = {
        400: (ERROR_TYPE_VALIDATION_ERROR, "Bad Request"),
        401: (ERROR_TYPE_UNAUTHORIZED, "Unauthorized"),
        404: (ERROR_TYPE_NOT_FOUND, "Not Found"),
        500: (ERROR_TYPE_INTERNAL_ERROR, "Internal Server Error"),
        503: (ERROR_TYPE_SERVICE_UNAVAILABLE, "Service Unavailable"),
    }
    return mapping.get(status_code, (ERROR_TYPE_INTERNAL_ERROR, "Error"))


# ========================================
# CUSTOM EXCEPTION CLASSES
# ========================================

class HolmesGPTAPIError(Exception):
    """Base exception for HolmesGPT API errors"""
    def __init__(self, message: str, details: Optional[Dict[str, Any]] = None):
        super().__init__(message)
        self.message = message
        self.details = details or {}
        self.timestamp = datetime.now(timezone.utc)


class AuthenticationError(HolmesGPTAPIError):
    """Authentication failed"""
    pass


class AuthorizationError(HolmesGPTAPIError):
    """Authorization failed (insufficient permissions)"""
    pass


class KubernetesAPIError(HolmesGPTAPIError):
    """Error communicating with Kubernetes API"""
    pass


class CircuitBreakerOpenError(HolmesGPTAPIError):
    """Circuit breaker is open, failing fast"""
    pass


class MaxRetriesExceededError(HolmesGPTAPIError):
    """Max retry attempts exceeded"""
    pass


class ValidationError(HolmesGPTAPIError):
    """Request validation failed"""
    pass


class SDKError(HolmesGPTAPIError):
    """HolmesGPT SDK error"""
    pass


# ========================================
# CIRCUIT BREAKER PATTERN (REFACTOR phase)
# ========================================

class CircuitBreaker:
    """
    Circuit breaker for external service calls

    Business Requirement: BR-HAPI-154 (Resilience patterns)

    REFACTOR phase: Production implementation with state management
    """

    def __init__(
        self,
        failure_threshold: int = 5,
        recovery_timeout: int = 60,
        expected_exception: type = Exception
    ):
        self.failure_threshold = failure_threshold
        self.recovery_timeout = recovery_timeout
        self.expected_exception = expected_exception
        self.failure_count = 0
        self.last_failure_time: Optional[datetime] = None
        self.state = "closed"  # closed, open, half_open

    def call(self, func, *args, **kwargs):
        """
        Execute function with circuit breaker protection
        """
        if self.state == "open":
            if self._should_attempt_reset():
                self.state = "half_open"
                logger.info({
                    "event": "circuit_breaker_half_open",
                    "function": func.__name__
                })
            else:
                raise CircuitBreakerOpenError(
                    f"Circuit breaker open for {func.__name__}",
                    details={
                        "failure_count": self.failure_count,
                        "last_failure": self.last_failure_time.isoformat() if self.last_failure_time else None
                    }
                )

        try:
            result = func(*args, **kwargs)
            self._on_success()
            return result
        except self.expected_exception:
            self._on_failure()
            raise

    def _should_attempt_reset(self) -> bool:
        """Check if enough time has passed to attempt reset"""
        if not self.last_failure_time:
            return True
        return datetime.now(timezone.utc) - self.last_failure_time > timedelta(seconds=self.recovery_timeout)

    def _on_success(self):
        """Handle successful call"""
        if self.state == "half_open":
            self.state = "closed"
            self.failure_count = 0
            logger.info({"event": "circuit_breaker_closed"})

    def _on_failure(self):
        """Handle failed call"""
        self.failure_count += 1
        self.last_failure_time = datetime.now(timezone.utc)

        if self.failure_count >= self.failure_threshold:
            self.state = "open"
            logger.error({
                "event": "circuit_breaker_opened",
                "failure_count": self.failure_count,
                "threshold": self.failure_threshold
            })


# ========================================
# RETRY DECORATOR (REFACTOR phase)
# ========================================

def retry_with_backoff(
    max_attempts: int = 3,
    initial_delay: float = 1.0,
    backoff_factor: float = 2.0,
    exceptions: tuple = (Exception,)
):
    """
    Decorator for retrying functions with exponential backoff

    Business Requirement: BR-HAPI-155 (Retry logic)

    REFACTOR phase: Production implementation
    """
    def decorator(func):
        async def wrapper(*args, **kwargs):
            delay = initial_delay
            last_exception = None

            for attempt in range(1, max_attempts + 1):
                try:
                    return await func(*args, **kwargs)
                except exceptions as e:
                    last_exception = e
                    if attempt == max_attempts:
                        logger.error({
                            "event": "max_retries_exceeded",
                            "function": func.__name__,
                            "attempts": attempt,
                            "error": str(e)
                        })
                        raise MaxRetriesExceededError(
                            f"Max retries ({max_attempts}) exceeded for {func.__name__}",
                            details={"last_error": str(e)}
                        )

                    logger.warning({
                        "event": "retry_attempt",
                        "function": func.__name__,
                        "attempt": attempt,
                        "max_attempts": max_attempts,
                        "delay": delay,
                        "error": str(e)
                    })

                    import asyncio
                    await asyncio.sleep(delay)
                    delay *= backoff_factor

            raise last_exception

        return wrapper
    return decorator

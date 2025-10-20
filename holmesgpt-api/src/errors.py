"""
Comprehensive Error Handling for HolmesGPT API Service

Business Requirements: BR-HAPI-146 to 165 (Error Handling)

REFACTOR phase: Production-grade error handling with circuit breaker pattern.
Design Decision: DD-HOLMESGPT-011, DD-HOLMESGPT-012
"""

import logging
from typing import Optional, Dict, Any
from datetime import datetime, timedelta

logger = logging.getLogger(__name__)


# ========================================
# CUSTOM EXCEPTION CLASSES
# ========================================

class HolmesGPTAPIError(Exception):
    """Base exception for HolmesGPT API errors"""
    def __init__(self, message: str, details: Optional[Dict[str, Any]] = None):
        super().__init__(message)
        self.message = message
        self.details = details or {}
        self.timestamp = datetime.utcnow()


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
        except self.expected_exception as e:
            self._on_failure()
            raise

    def _should_attempt_reset(self) -> bool:
        """Check if enough time has passed to attempt reset"""
        if not self.last_failure_time:
            return True
        return datetime.utcnow() - self.last_failure_time > timedelta(seconds=self.recovery_timeout)

    def _on_success(self):
        """Handle successful call"""
        if self.state == "half_open":
            self.state = "closed"
            self.failure_count = 0
            logger.info({"event": "circuit_breaker_closed"})

    def _on_failure(self):
        """Handle failed call"""
        self.failure_count += 1
        self.last_failure_time = datetime.utcnow()

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

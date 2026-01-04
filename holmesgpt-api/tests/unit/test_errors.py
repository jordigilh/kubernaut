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
Unit Tests for Error Handling Module

Business Requirements: BR-HAPI-146 to 165 (Error Handling)
Test Coverage Target: 0% → 80%

Phase 1.1 of HolmesGPT API Implementation Plan
"""

import pytest
from datetime import datetime, timezone
from src.errors import (
    HolmesGPTAPIError,
    AuthenticationError,
    AuthorizationError,
    KubernetesAPIError,
    CircuitBreakerOpenError,
    MaxRetriesExceededError,
    ValidationError,
    SDKError,
    CircuitBreaker,
    retry_with_backoff
)


# ========================================
# TEST SUITE 1: Custom Exception Classes
# ========================================

class TestHolmesGPTAPIError:
    """Test base exception class"""

    def test_init_with_message_only(self):
        """Test exception initialization with message"""
        error = HolmesGPTAPIError("Test error message")

        assert error.message == "Test error message"
        assert error.details == {}
        assert isinstance(error.timestamp, datetime)
        assert str(error) == "Test error message"

    def test_init_with_details(self):
        """Test exception initialization with details"""
        details = {"code": "ERR001", "resource": "pod/test"}
        error = HolmesGPTAPIError("Test error", details=details)

        assert error.message == "Test error"
        assert error.details == details
        assert error.details["code"] == "ERR001"

    def test_timestamp_is_recent(self):
        """Test that timestamp is set to current UTC time"""
        before = datetime.now(timezone.utc)
        error = HolmesGPTAPIError("Test")
        after = datetime.now(timezone.utc)

        assert before <= error.timestamp <= after

    def test_exception_inheritance(self):
        """Test that custom exception inherits from Exception"""
        error = HolmesGPTAPIError("Test")
        assert isinstance(error, Exception)


class TestAuthenticationError:
    """Test authentication error"""

    def test_inherits_from_base_error(self):
        """Test inheritance"""
        error = AuthenticationError("Invalid token")
        assert isinstance(error, HolmesGPTAPIError)
        assert isinstance(error, Exception)

    def test_can_be_raised_and_caught(self):
        """Test error can be raised and caught"""
        with pytest.raises(AuthenticationError) as exc_info:
            raise AuthenticationError("Token expired", {"token": "xxx"})

        assert exc_info.value.message == "Token expired"
        assert exc_info.value.details["token"] == "xxx"


class TestAuthorizationError:
    """Test authorization error"""

    def test_inherits_from_base_error(self):
        """Test inheritance"""
        error = AuthorizationError("Insufficient permissions")
        assert isinstance(error, HolmesGPTAPIError)

    def test_with_permission_details(self):
        """Test with permission details"""
        details = {
            "required": ["pods:read"],
            "actual": ["pods:list"]
        }
        error = AuthorizationError("Access denied", details=details)

        assert error.details["required"] == ["pods:read"]
        assert error.details["actual"] == ["pods:list"]


class TestKubernetesAPIError:
    """Test Kubernetes API error"""

    def test_inherits_from_base_error(self):
        """Test inheritance"""
        error = KubernetesAPIError("API call failed")
        assert isinstance(error, HolmesGPTAPIError)

    def test_with_k8s_error_details(self):
        """Test with K8s-specific error details"""
        details = {
            "status_code": 404,
            "resource": "pod/nginx",
            "namespace": "default"
        }
        error = KubernetesAPIError("Pod not found", details=details)

        assert error.details["status_code"] == 404
        assert error.details["resource"] == "pod/nginx"


class TestCircuitBreakerOpenError:
    """Test circuit breaker error"""

    def test_inherits_from_base_error(self):
        """Test inheritance"""
        error = CircuitBreakerOpenError("Circuit open")
        assert isinstance(error, HolmesGPTAPIError)

    def test_with_circuit_breaker_state(self):
        """Test with circuit breaker state details"""
        details = {
            "failure_count": 5,
            "last_failure": "2025-01-01T00:00:00"
        }
        error = CircuitBreakerOpenError("Circuit breaker tripped", details=details)

        assert error.details["failure_count"] == 5
        assert "last_failure" in error.details


class TestMaxRetriesExceededError:
    """Test max retries error"""

    def test_inherits_from_base_error(self):
        """Test inheritance"""
        error = MaxRetriesExceededError("Max retries reached")
        assert isinstance(error, HolmesGPTAPIError)

    def test_with_retry_details(self):
        """Test with retry attempt details"""
        details = {
            "attempts": 3,
            "last_error": "Connection timeout"
        }
        error = MaxRetriesExceededError("Retries exhausted", details=details)

        assert error.details["attempts"] == 3
        assert error.details["last_error"] == "Connection timeout"


class TestValidationError:
    """Test validation error"""

    def test_inherits_from_base_error(self):
        """Test inheritance"""
        error = ValidationError("Invalid input")
        assert isinstance(error, HolmesGPTAPIError)

    def test_with_validation_details(self):
        """Test with validation failure details"""
        details = {
            "field": "alert.namespace",
            "reason": "required field missing"
        }
        error = ValidationError("Validation failed", details=details)

        assert error.details["field"] == "alert.namespace"
        assert "required" in error.details["reason"]


class TestSDKError:
    """Test SDK error"""

    def test_inherits_from_base_error(self):
        """Test inheritance"""
        error = SDKError("SDK call failed")
        assert isinstance(error, HolmesGPTAPIError)

    def test_with_sdk_error_details(self):
        """Test with SDK-specific error details"""
        details = {
            "sdk_version": "1.0.0",
            "method": "investigate",
            "error_code": "SDK_TIMEOUT"
        }
        error = SDKError("Investigation timed out", details=details)

        assert error.details["sdk_version"] == "1.0.0"
        assert error.details["method"] == "investigate"


# ========================================
# TEST SUITE 2: Circuit Breaker Pattern
# ========================================

class TestCircuitBreaker:
    """Test circuit breaker implementation"""

    def test_initialization(self):
        """Test circuit breaker initialization"""
        cb = CircuitBreaker(failure_threshold=3, recovery_timeout=30)

        assert cb.failure_threshold == 3
        assert cb.recovery_timeout == 30
        assert cb.failure_count == 0
        assert cb.state == "closed"
        assert cb.last_failure_time is None

    def test_successful_call_when_closed(self):
        """Test successful call when circuit is closed"""
        cb = CircuitBreaker()

        def success_func():
            return "success"

        result = cb.call(success_func)
        assert result == "success"
        assert cb.state == "closed"
        assert cb.failure_count == 0

    def test_failure_increments_count(self):
        """Test that failures increment failure count"""
        cb = CircuitBreaker(failure_threshold=3)

        def failing_func():
            raise Exception("Test failure")

        # First failure
        with pytest.raises(Exception):
            cb.call(failing_func)

        assert cb.failure_count == 1
        assert cb.state == "closed"
        assert cb.last_failure_time is not None

    def test_circuit_opens_after_threshold(self):
        """Test circuit opens after reaching failure threshold"""
        cb = CircuitBreaker(failure_threshold=3)

        def failing_func():
            raise Exception("Test failure")

        # Cause 3 failures to trip the circuit
        for _ in range(3):
            with pytest.raises(Exception):
                cb.call(failing_func)

        assert cb.failure_count == 3
        assert cb.state == "open"

    def test_open_circuit_raises_circuit_breaker_error(self):
        """Test open circuit raises CircuitBreakerOpenError"""
        cb = CircuitBreaker(failure_threshold=2)

        def failing_func():
            raise Exception("Test failure")

        # Trip the circuit
        for _ in range(2):
            with pytest.raises(Exception):
                cb.call(failing_func)

        # Now circuit is open, should raise CircuitBreakerOpenError
        with pytest.raises(CircuitBreakerOpenError) as exc_info:
            cb.call(failing_func)

        assert "Circuit breaker open" in exc_info.value.message
        assert exc_info.value.details["failure_count"] == 2

    def test_half_open_after_recovery_timeout(self, wait_for):
        """Test circuit transitions to half-open after recovery timeout"""
        import time
        cb = CircuitBreaker(failure_threshold=2, recovery_timeout=1)

        def failing_func():
            raise Exception("Test failure")

        # Trip the circuit
        for _ in range(2):
            with pytest.raises(Exception):
                cb.call(failing_func)

        assert cb.state == "open"
        trip_time = time.time()

        # Wait for recovery timeout to elapse (typically <1.1s instead of blocking 1.1s)
        wait_for(lambda: time.time() - trip_time >= 1.0, timeout=1.5, error_msg="Recovery timeout should elapse")

        # Next call should transition to half-open and then fail
        with pytest.raises(Exception):
            cb.call(failing_func)

        # Circuit should have attempted half-open
        assert cb.failure_count >= 2

    def test_half_open_to_closed_on_success(self, wait_for):
        """Test circuit transitions from half-open to closed on success"""
        import time
        cb = CircuitBreaker(failure_threshold=2, recovery_timeout=1)

        call_count = [0]

        def flaky_func():
            call_count[0] += 1
            if call_count[0] <= 2:
                raise Exception("Initial failures")
            return "success"

        # Trip the circuit
        for _ in range(2):
            with pytest.raises(Exception):
                cb.call(flaky_func)

        assert cb.state == "open"
        trip_time = time.time()

        # Wait for recovery timeout to elapse (typically <1.1s instead of blocking 1.1s)
        wait_for(lambda: time.time() - trip_time >= 1.0, timeout=1.5, error_msg="Recovery timeout should elapse")

        # Next successful call should close the circuit
        result = cb.call(flaky_func)
        assert result == "success"
        assert cb.state == "closed"
        assert cb.failure_count == 0

    def test_custom_expected_exception(self):
        """Test circuit breaker with custom expected exception"""
        cb = CircuitBreaker(failure_threshold=2, expected_exception=ValueError)

        def value_error_func():
            raise ValueError("Value error")

        def type_error_func():
            raise TypeError("Type error")

        # ValueError should be caught by circuit breaker
        with pytest.raises(ValueError):
            cb.call(value_error_func)

        assert cb.failure_count == 1

        # TypeError should not be caught (should propagate)
        with pytest.raises(TypeError):
            cb.call(type_error_func)

        # Failure count should still be 1 (TypeError not counted)
        assert cb.failure_count == 1


# ========================================
# TEST SUITE 3: Retry Decorator
# ========================================

class TestRetryWithBackoff:
    """Test retry decorator with exponential backoff"""

    @pytest.mark.asyncio
    async def test_successful_call_no_retry(self):
        """Test successful call requires no retry"""
        call_count = [0]

        @retry_with_backoff(max_attempts=3)
        async def success_func():
            call_count[0] += 1
            return "success"

        result = await success_func()
        assert result == "success"
        assert call_count[0] == 1

    @pytest.mark.asyncio
    async def test_retries_on_failure(self):
        """Test function retries on failure"""
        call_count = [0]

        @retry_with_backoff(max_attempts=3, initial_delay=0.1)
        async def flaky_func():
            call_count[0] += 1
            if call_count[0] < 2:
                raise Exception("Temporary failure")
            return "success"

        result = await flaky_func()
        assert result == "success"
        assert call_count[0] == 2

    @pytest.mark.asyncio
    async def test_raises_max_retries_exceeded(self):
        """Test raises MaxRetriesExceededError after max attempts"""
        call_count = [0]

        @retry_with_backoff(max_attempts=3, initial_delay=0.1)
        async def always_fails():
            call_count[0] += 1
            raise Exception("Persistent failure")

        with pytest.raises(MaxRetriesExceededError) as exc_info:
            await always_fails()

        assert call_count[0] == 3
        assert "Max retries (3) exceeded" in exc_info.value.message
        assert "Persistent failure" in exc_info.value.details["last_error"]

    @pytest.mark.asyncio
    async def test_exponential_backoff_timing(self):
        """Test exponential backoff delay increases"""
        call_times = []

        @retry_with_backoff(max_attempts=3, initial_delay=0.1, backoff_factor=2.0)
        async def failing_func():
            call_times.append(datetime.now(timezone.utc))
            raise Exception("Failure")

        try:
            await failing_func()
        except MaxRetriesExceededError:
            pass

        # Should have 3 call times
        assert len(call_times) == 3

        # Check delays are approximately exponential
        delay1 = (call_times[1] - call_times[0]).total_seconds()
        delay2 = (call_times[2] - call_times[1]).total_seconds()

        # delay1 should be ~0.1s, delay2 should be ~0.2s
        assert 0.08 <= delay1 <= 0.15
        assert 0.18 <= delay2 <= 0.25

    @pytest.mark.asyncio
    async def test_custom_exception_types(self):
        """Test retry only on specific exception types"""
        call_count = [0]

        @retry_with_backoff(
            max_attempts=3,
            initial_delay=0.1,
            exceptions=(ValueError,)
        )
        async def specific_error_func():
            call_count[0] += 1
            if call_count[0] == 1:
                raise ValueError("Retryable error")
            elif call_count[0] == 2:
                raise TypeError("Non-retryable error")
            return "success"

        # ValueError should be retried, but TypeError should not
        with pytest.raises(TypeError):
            await specific_error_func()

        # Should have called twice (first ValueError, then TypeError)
        assert call_count[0] == 2

    @pytest.mark.asyncio
    async def test_backoff_factor_zero(self):
        """Test retry with no backoff (backoff_factor=1.0)"""
        call_times = []

        @retry_with_backoff(
            max_attempts=3,
            initial_delay=0.05,
            backoff_factor=1.0
        )
        async def failing_func():
            call_times.append(datetime.now(timezone.utc))
            raise Exception("Failure")

        try:
            await failing_func()
        except MaxRetriesExceededError:
            pass

        # All delays should be approximately the same
        delay1 = (call_times[1] - call_times[0]).total_seconds()
        delay2 = (call_times[2] - call_times[1]).total_seconds()

        assert 0.04 <= delay1 <= 0.08
        assert 0.04 <= delay2 <= 0.08
        assert abs(delay1 - delay2) < 0.02  # Similar delays


# ========================================
# TEST SUITE 4: Error Serialization
# ========================================

class TestErrorSerialization:
    """Test error serialization for API responses"""

    def test_error_dict_representation(self):
        """Test error can be converted to dict for JSON response"""
        error = HolmesGPTAPIError(
            "Test error",
            details={"code": "ERR001", "field": "alert.name"}
        )

        error_dict = {
            "message": error.message,
            "details": error.details,
            "timestamp": error.timestamp.isoformat()
        }

        assert error_dict["message"] == "Test error"
        assert error_dict["details"]["code"] == "ERR001"
        assert isinstance(error_dict["timestamp"], str)

    def test_nested_exception_details(self):
        """Test errors with nested details"""
        error = KubernetesAPIError(
            "API call failed",
            details={
                "endpoint": "/api/v1/pods",
                "error": {
                    "status": 500,
                    "reason": "Internal Server Error"
                }
            }
        )

        assert error.details["endpoint"] == "/api/v1/pods"
        assert error.details["error"]["status"] == 500
        assert error.details["error"]["reason"] == "Internal Server Error"


# ========================================
# TEST SUITE 5: Edge Cases
# ========================================

class TestEdgeCases:
    """Test edge cases and error handling boundaries"""

    def test_empty_details_dict(self):
        """Test error with explicitly empty details"""
        error = HolmesGPTAPIError("Test", details={})
        assert error.details == {}

    def test_none_details(self):
        """Test error with None details"""
        error = HolmesGPTAPIError("Test", details=None)
        assert error.details == {}

    def test_very_long_error_message(self):
        """Test error with very long message"""
        long_message = "Error: " + "x" * 10000
        error = HolmesGPTAPIError(long_message)
        assert len(error.message) == len(long_message)
        assert error.message.startswith("Error: xxx")

    def test_circuit_breaker_zero_threshold(self):
        """Test circuit breaker with zero threshold (should open immediately)"""
        cb = CircuitBreaker(failure_threshold=0)

        def failing_func():
            raise Exception("Failure")

        # Should open on first failure since threshold is 0
        with pytest.raises(Exception):
            cb.call(failing_func)

        # Circuit should be open now
        assert cb.state == "open"

    def test_circuit_breaker_negative_timeout(self):
        """Test circuit breaker with negative recovery timeout"""
        cb = CircuitBreaker(failure_threshold=2, recovery_timeout=-1)

        def failing_func():
            raise Exception("Failure")

        # Trip the circuit
        for _ in range(2):
            with pytest.raises(Exception):
                cb.call(failing_func)

        # With negative timeout, should always allow reset attempt
        # The circuit should transition to half-open immediately
        with pytest.raises(Exception):
            cb.call(failing_func)




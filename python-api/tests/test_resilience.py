"""
Test resilience utilities and decorators.

This module provides utilities to make tests more resilient to implementation changes
and external dependencies.
"""

import asyncio
import functools
import time
from typing import Any, Callable, Dict, List, Optional, Set, Union
import pytest
from unittest.mock import Mock, AsyncMock


class ResilientValidation:
    """Validation helpers that use ranges instead of exact values."""

    # Valid ranges for common test values
    VALID_HTTP_SUCCESS = {200, 201, 202, 204}
    VALID_HTTP_CLIENT_ERROR = {400, 422, 428, 429}
    VALID_HTTP_SERVER_ERROR = {500, 502, 503, 504}

    CONFIDENCE_RANGE = (0.0, 1.0)
    PROCESSING_TIME_MAX = 60.0  # seconds
    TOKENS_RANGE = (1, 50000)
    TEMPERATURE_RANGE = (0.0, 2.0)
    TIMEOUT_RANGE = (1, 600)  # seconds

    VALID_SEVERITIES = {"critical", "warning", "info"}
    VALID_STATUSES = {"firing", "resolved", "pending"}
    VALID_HEALTH_STATUSES = {"healthy", "degraded", "unhealthy"}
    VALID_ASSESSMENT_LEVELS = {"low", "medium", "high", "critical"}

    @staticmethod
    def is_success_status(status_code: int) -> bool:
        """Check if HTTP status indicates success."""
        return status_code in ResilientValidation.VALID_HTTP_SUCCESS

    @staticmethod
    def is_client_error(status_code: int) -> bool:
        """Check if HTTP status indicates client error."""
        return status_code in ResilientValidation.VALID_HTTP_CLIENT_ERROR

    @staticmethod
    def is_server_error(status_code: int) -> bool:
        """Check if HTTP status indicates server error."""
        return status_code in ResilientValidation.VALID_HTTP_SERVER_ERROR

    @staticmethod
    def is_valid_confidence(confidence: float) -> bool:
        """Check if confidence is in valid range."""
        return ResilientValidation.CONFIDENCE_RANGE[0] <= confidence <= ResilientValidation.CONFIDENCE_RANGE[1]

    @staticmethod
    def is_reasonable_processing_time(time_seconds: float) -> bool:
        """Check if processing time is reasonable."""
        return 0.0 <= time_seconds <= ResilientValidation.PROCESSING_TIME_MAX

    @staticmethod
    def is_valid_severity(severity: str) -> bool:
        """Check if severity is valid."""
        return severity.lower() in ResilientValidation.VALID_SEVERITIES

    @staticmethod
    def is_valid_assessment(assessment: str) -> bool:
        """Check if assessment level is valid."""
        return assessment.lower() in ResilientValidation.VALID_ASSESSMENT_LEVELS

    @staticmethod
    def normalize_severity(severity: str) -> str:
        """Normalize severity to expected format."""
        severity = severity.lower().strip()
        # Handle common variations
        mapping = {
            'warn': 'warning',
            'crit': 'critical',
            'error': 'critical',
            'high': 'critical',
            'medium': 'warning',
            'low': 'info'
        }
        return mapping.get(severity, severity)

    @staticmethod
    def assert_response_structure(data: Dict[str, Any], required_fields: List[str]):
        """Assert response has required structure without checking exact values."""
        for field in required_fields:
            assert field in data, f"Missing required field: {field}"

        # Validate common field types with ranges
        if 'confidence' in data:
            assert ResilientValidation.is_valid_confidence(data['confidence']), \
                f"Invalid confidence: {data['confidence']}"

        if 'severity' in data:
            assert ResilientValidation.is_valid_severity(data['severity']), \
                f"Invalid severity: {data['severity']}"

        if 'processing_time' in data:
            assert ResilientValidation.is_reasonable_processing_time(data['processing_time']), \
                f"Invalid processing time: {data['processing_time']}"


def retry_on_failure(max_attempts: int = 3, delay: float = 0.1, exponential_backoff: bool = True):
    """
    Decorator to retry flaky tests.

    Args:
        max_attempts: Maximum number of retry attempts
        delay: Initial delay between retries (seconds)
        exponential_backoff: Whether to use exponential backoff
    """
    def decorator(func):
        @functools.wraps(func)
        async def async_wrapper(*args, **kwargs):
            last_exception = None
            current_delay = delay

            for attempt in range(max_attempts):
                try:
                    return await func(*args, **kwargs)
                except (AssertionError, Exception) as e:
                    last_exception = e
                    if attempt < max_attempts - 1:  # Not the last attempt
                        await asyncio.sleep(current_delay)
                        if exponential_backoff:
                            current_delay *= 2
                    else:
                        # Last attempt, re-raise the exception
                        raise last_exception

            raise last_exception

        @functools.wraps(func)
        def sync_wrapper(*args, **kwargs):
            last_exception = None
            current_delay = delay

            for attempt in range(max_attempts):
                try:
                    return func(*args, **kwargs)
                except (AssertionError, Exception) as e:
                    last_exception = e
                    if attempt < max_attempts - 1:  # Not the last attempt
                        time.sleep(current_delay)
                        if exponential_backoff:
                            current_delay *= 2
                    else:
                        # Last attempt, re-raise the exception
                        raise last_exception

            raise last_exception

        # Return appropriate wrapper based on function type
        if asyncio.iscoroutinefunction(func):
            return async_wrapper
        else:
            return sync_wrapper

    return decorator


class MockStabilizer:
    """Utilities to make mocks more stable and predictable."""

    @staticmethod
    def create_stable_response(base_response: Dict[str, Any]) -> Dict[str, Any]:
        """Create a response that's stable across test runs."""
        stable_response = base_response.copy()

        # Ensure confidence is in valid range
        if 'confidence' in stable_response:
            confidence = stable_response['confidence']
            stable_response['confidence'] = max(0.0, min(1.0, float(confidence)))

        # Normalize severity
        if 'severity' in stable_response:
            stable_response['severity'] = ResilientValidation.normalize_severity(
                stable_response['severity']
            )

        # Ensure processing time is reasonable
        if 'processing_time' in stable_response:
            processing_time = stable_response['processing_time']
            stable_response['processing_time'] = max(0.0, min(60.0, float(processing_time)))

        return stable_response

    @staticmethod
    def create_resilient_mock(spec_class: type, **default_returns) -> Mock:
        """Create a mock that's resilient to interface changes."""
        mock = Mock(spec=spec_class)

        # Set up default return values
        for attr, value in default_returns.items():
            if hasattr(spec_class, attr):
                setattr(mock, attr, Mock(return_value=value))

        return mock


# Pytest fixtures for resilient testing
@pytest.fixture
def resilient_validator():
    """Provide access to resilient validation utilities."""
    return ResilientValidation()


@pytest.fixture
def mock_stabilizer():
    """Provide access to mock stabilization utilities."""
    return MockStabilizer()


@pytest.fixture
def http_status_sets():
    """Provide sets of valid HTTP status codes."""
    return {
        'success': ResilientValidation.VALID_HTTP_SUCCESS,
        'client_error': ResilientValidation.VALID_HTTP_CLIENT_ERROR,
        'server_error': ResilientValidation.VALID_HTTP_SERVER_ERROR
    }


# Helper functions for common test patterns
def assert_valid_ask_response(response_data: Dict[str, Any]):
    """Assert that ask response has valid structure and values."""
    required_fields = ['response', 'confidence', 'model_used']
    ResilientValidation.assert_response_structure(response_data, required_fields)

    # Additional ask-specific validations
    assert isinstance(response_data['response'], str)
    assert len(response_data['response']) > 0
    assert isinstance(response_data['model_used'], str)

    if 'recommendations' in response_data:
        assert isinstance(response_data['recommendations'], list)

    if 'sources' in response_data:
        assert isinstance(response_data['sources'], list)


def assert_valid_investigate_response(response_data: Dict[str, Any]):
    """Assert that investigate response has valid structure and values."""
    required_fields = ['investigation', 'confidence', 'severity_assessment']
    ResilientValidation.assert_response_structure(response_data, required_fields)

    # Additional investigate-specific validations
    assert 'investigation' in response_data
    assert isinstance(response_data['investigation'], dict)

    if 'alert_analysis' in response_data['investigation']:
        assert isinstance(response_data['investigation']['alert_analysis'], dict)

    # Severity assessment should be valid
    assert ResilientValidation.is_valid_assessment(response_data['severity_assessment'])


def assert_http_response_valid(response, expected_status_set: Optional[Set[int]] = None):
    """Assert HTTP response is valid with flexible status checking."""
    if expected_status_set is None:
        expected_status_set = ResilientValidation.VALID_HTTP_SUCCESS

    assert response.status_code in expected_status_set, \
        f"Status {response.status_code} not in expected set {expected_status_set}"

    if ResilientValidation.is_success_status(response.status_code):
        # Successful responses should have valid JSON
        try:
            data = response.json()
            assert isinstance(data, dict)
        except Exception as e:
            pytest.fail(f"Successful response should have valid JSON: {e}")

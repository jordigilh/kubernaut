"""
Test Isolation Framework

This module provides comprehensive test isolation to prevent state pollution
between tests. It ensures that global variables, mocks, and application state
are properly reset between test runs.

Key Features:
1. Global variable reset
2. Mock cleanup
3. Application state reset
4. Dependency injection reset
5. Automatic cleanup with pytest fixtures
"""

import asyncio
import logging
from typing import Any, Dict, Optional
from unittest.mock import AsyncMock, MagicMock
import pytest

# Import global variables that need to be reset
import app.main


class TestIsolationManager:
    """Manages test isolation by tracking and resetting global state."""

    def __init__(self):
        self._original_state: Dict[str, Any] = {}
        self._reset_callbacks = []

    def capture_original_state(self):
        """Capture the original state of global variables."""
        self._original_state = {
            'holmes_service': getattr(app.main, 'holmes_service', None),
            'metrics_manager': getattr(app.main, 'metrics_manager', None),
            # Add other global variables that need tracking
        }
        logging.debug(f"Captured original state: {self._original_state}")

    def reset_global_state(self):
        """Reset all global variables to their original state."""
        # Reset global variables
        app.main.holmes_service = self._original_state.get('holmes_service')
        app.main.metrics_manager = self._original_state.get('metrics_manager')

        # Execute any registered reset callbacks
        for callback in self._reset_callbacks:
            try:
                callback()
            except Exception as e:
                logging.warning(f"Reset callback failed: {e}")

        logging.debug("Reset global state to original values")

    def register_reset_callback(self, callback):
        """Register a callback to be executed during state reset."""
        self._reset_callbacks.append(callback)

    def create_proper_mock_service(self):
        """Create a properly configured mock service that returns valid data structures."""
        from app.models.responses import AskResponse, AnalysisResult

        # Create a mock that returns proper structured data
        mock_service = MagicMock()

        # Configure ask method to return proper AskResponse structure
        mock_ask_response = AskResponse(
            response="This is a test response",
            analysis=AnalysisResult(
                summary="Test analysis summary",
                root_cause="Test root cause",
                impact_assessment="Test impact assessment",
                urgency_level="medium",
                related_metrics={"test_metric": "value"}
            ),
            recommendations=[],
            confidence_score=0.85,
            model_used="test-model",
            processing_time=1.0,
            timestamp=asyncio.get_event_loop().time(),
            context_used={}
        )

        mock_service.ask = AsyncMock(return_value=mock_ask_response)
        mock_service.health_check = AsyncMock()

        return mock_service


# Global instance
_isolation_manager = TestIsolationManager()


@pytest.fixture(scope='function', autouse=True)
def test_isolation():
    """
    Automatic test isolation fixture.

    This fixture runs before and after each test to ensure proper isolation.
    It's marked as autouse=True so it applies to all tests automatically.
    """
    # Before test: capture original state
    _isolation_manager.capture_original_state()

    yield _isolation_manager

    # After test: reset state
    _isolation_manager.reset_global_state()


@pytest.fixture(scope='function')
def isolated_mock_service():
    """
    Provides a properly configured mock service for tests.

    This mock returns valid data structures instead of bare AsyncMock objects,
    preventing Pydantic validation errors.
    """
    return _isolation_manager.create_proper_mock_service()


@pytest.fixture(scope='function')
def clean_app_state():
    """
    Ensures the FastAPI app starts with clean state.

    This fixture can be used when tests need to ensure the app
    hasn't been modified by previous tests.
    """
    # Reset any app-level state
    from app.main import app

    # Clear any cached dependency overrides
    app.dependency_overrides.clear()

    yield app

    # Cleanup after test
    app.dependency_overrides.clear()


@pytest.fixture(scope='function')
def mock_holmes_service_with_exception():
    """
    Provides a mock service that raises exceptions as expected.

    Use this fixture when you need to test error handling paths.
    """
    mock_service = MagicMock()
    mock_service.ask = AsyncMock(side_effect=Exception("Test exception"))
    mock_service.health_check = AsyncMock(side_effect=Exception("Health check failed"))

    return mock_service


# Utility functions for manual state management
def reset_global_state():
    """Manually reset global state (for use in test setup)."""
    _isolation_manager.reset_global_state()


def get_proper_mock_service():
    """Get a properly configured mock service (for manual use)."""
    return _isolation_manager.create_proper_mock_service()


def ensure_test_isolation():
    """
    Decorator for test classes that need extra isolation guarantees.

    Usage:
        @ensure_test_isolation()
        class TestMyClass:
            def test_something(self):
                pass
    """
    def decorator(test_class):
        # Add setup and teardown methods if they don't exist
        original_setup = getattr(test_class, 'setup_method', None)
        original_teardown = getattr(test_class, 'teardown_method', None)

        def setup_method(self, method):
            _isolation_manager.capture_original_state()
            if original_setup:
                original_setup(method)

        def teardown_method(self, method):
            if original_teardown:
                original_teardown(method)
            _isolation_manager.reset_global_state()

        test_class.setup_method = setup_method
        test_class.teardown_method = teardown_method

        return test_class

    return decorator

"""
Environment isolation utilities for tests.

This module provides utilities to isolate test environments and prevent
cross-contamination between tests.
"""

import os
import tempfile
import logging
from contextlib import contextmanager
from typing import Dict, Optional, Any
from unittest.mock import patch
import pytest


@contextmanager
def isolated_environment(env_vars: Optional[Dict[str, str]] = None, clear_existing: bool = True):
    """
    Create an isolated environment for tests.

    Args:
        env_vars: Environment variables to set
        clear_existing: Whether to clear existing environment variables
    """
    env_vars = env_vars or {}

    # Store original environment
    original_env = dict(os.environ)

    try:
        if clear_existing:
            os.environ.clear()

        # Set test-specific environment variables
        test_env = {
            'ENVIRONMENT': 'test',
            'DEBUG_MODE': 'true',
            'LOG_LEVEL': 'DEBUG',
            'PYTEST_RUNNING': 'true',
            'METRICS_ENABLED': 'false',
            'CACHE_ENABLED': 'false'
        }
        test_env.update(env_vars)

        os.environ.update(test_env)

        yield

    finally:
        # Restore original environment
        os.environ.clear()
        os.environ.update(original_env)


@contextmanager
def isolated_logging(level: str = "DEBUG", format_type: str = "json",
                    log_file: Optional[str] = None, clean_handlers: bool = True):
    """
    Create isolated logging environment for tests.

    Args:
        level: Log level to set
        format_type: Format type (json or text)
        log_file: Optional log file path
        clean_handlers: Whether to clean existing handlers
    """
    # Store original logging configuration
    original_loggers = {}
    original_level = logging.getLogger().level
    original_handlers = logging.getLogger().handlers.copy()

    try:
        if clean_handlers:
            # Clear all existing handlers
            root_logger = logging.getLogger()
            for handler in root_logger.handlers[:]:
                root_logger.removeHandler(handler)

        # Set up isolated logging
        from app.utils.logging import setup_logging
        setup_logging(level=level, format_type=format_type, log_file=log_file)

        yield

    finally:
        # Restore original logging configuration
        root_logger = logging.getLogger()

        # Clear test handlers
        for handler in root_logger.handlers[:]:
            root_logger.removeHandler(handler)

        # Restore original handlers
        for handler in original_handlers:
            root_logger.addHandler(handler)

        root_logger.setLevel(original_level)


@pytest.fixture
def isolated_test_env():
    """Provide isolated test environment as fixture."""
    with isolated_environment() as env:
        yield env


@pytest.fixture
def temp_log_file():
    """Provide temporary log file for tests."""
    with tempfile.NamedTemporaryFile(mode='w', suffix='.log', delete=False) as f:
        log_file = f.name

    yield log_file

    # Cleanup
    try:
        os.unlink(log_file)
    except FileNotFoundError:
        pass


@contextmanager
def mock_system_resources(memory_percent: float = 50.0, cpu_percent: float = 25.0):
    """Mock system resource usage for consistent tests."""
    with patch('psutil.virtual_memory') as mock_memory, \
         patch('psutil.cpu_percent') as mock_cpu:

        # Mock memory usage
        mock_memory.return_value.percent = memory_percent

        # Mock CPU usage
        mock_cpu.return_value = cpu_percent

        yield


@contextmanager
def stable_timestamps():
    """Provide stable timestamp for consistent testing."""
    from datetime import datetime, timezone

    fixed_time = datetime(2024, 1, 15, 12, 0, 0, tzinfo=timezone.utc)

    with patch('datetime.datetime') as mock_datetime:
        mock_datetime.now.return_value = fixed_time
        mock_datetime.utcnow.return_value = fixed_time
        mock_datetime.side_effect = lambda *args, **kwargs: datetime(*args, **kwargs)

        yield fixed_time


class EnvironmentValidator:
    """Validate test environment setup."""

    @staticmethod
    def validate_test_environment():
        """Validate that we're in a proper test environment."""
        assert os.getenv('PYTEST_RUNNING') == 'true', "Not in pytest environment"
        assert os.getenv('ENVIRONMENT') == 'test', "Not in test environment"

    @staticmethod
    def validate_logging_isolation():
        """Validate that logging is properly isolated."""
        root_logger = logging.getLogger()

        # Should have at least one handler for testing
        assert len(root_logger.handlers) > 0, "No logging handlers configured"

        # Should not have production handlers
        handler_types = [type(h).__name__ for h in root_logger.handlers]
        assert 'SysLogHandler' not in handler_types, "Production handlers present"

    @staticmethod
    def validate_no_side_effects():
        """Validate that tests haven't left side effects."""
        # Check for common side effect indicators
        test_indicators = [
            'PYTEST_RUNNING',
            'test_',
            'mock_',
            'fake_'
        ]

        for key in os.environ:
            for indicator in test_indicators:
                if indicator in key.lower():
                    # This is expected in test environment
                    continue


# Helper decorators for test isolation

def with_isolated_environment(**env_vars):
    """Decorator for isolated environment tests."""
    def decorator(func):
        def wrapper(*args, **kwargs):
            with isolated_environment(env_vars):
                return func(*args, **kwargs)
        return wrapper
    return decorator


def with_isolated_logging(level="DEBUG", format_type="json"):
    """Decorator for isolated logging tests."""
    def decorator(func):
        def wrapper(*args, **kwargs):
            with isolated_logging(level=level, format_type=format_type):
                return func(*args, **kwargs)
        return wrapper
    return decorator

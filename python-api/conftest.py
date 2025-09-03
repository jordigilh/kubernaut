"""
Global test configuration and fixtures.

This file automatically loads the test isolation framework and makes
fixtures available to all tests in the project.
"""

import pytest
import sys
from pathlib import Path

# Add the project root to Python path for imports
project_root = Path(__file__).parent
sys.path.insert(0, str(project_root))

# Import test isolation fixtures to make them available globally
from tests.fixtures.test_isolation import (
    test_isolation,
    isolated_mock_service,
    clean_app_state,
    mock_holmes_service_with_exception,
)

# Re-export fixtures so they're available to all tests
pytest_plugins = ['tests.fixtures.test_isolation']

# Configure pytest to use the test isolation framework
def pytest_configure(config):
    """Configure pytest with custom settings."""
    # Add custom markers
    config.addinivalue_line(
        "markers", "isolation: mark test as requiring extra isolation"
    )
    config.addinivalue_line(
        "markers", "integration: mark test as integration test"
    )

def pytest_runtest_setup(item):
    """Run before each test."""
    # Additional setup can be added here if needed
    pass

def pytest_runtest_teardown(item, nextitem):
    """Run after each test."""
    # Additional cleanup can be added here if needed
    pass

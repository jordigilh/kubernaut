"""
Unit Test Configuration

Unit tests validate implementation correctness without external dependencies.
Per TESTING_GUIDELINES.md: Unit tests use mocks for all external services.
"""

import pytest
import os
import time
import tempfile
from unittest.mock import patch


def pytest_configure(config):
    """
    Pytest hook that runs before test collection.
    Create config file BEFORE any test modules are imported.
    """
    _config_content = """
llm:
  provider: "openai"
  model: "gpt-4"
  endpoint: "http://127.0.0.1:8080"

data_storage:
  url: "http://127.0.0.1:18098"

logging:
  level: "INFO"
  format: "json"
"""
    
    _temp_config = tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False)
    _temp_config.write(_config_content)
    _temp_config.close()
    
    # Set CONFIG_FILE and OPENAI_API_KEY env vars BEFORE any test modules import src.main
    os.environ["CONFIG_FILE"] = _temp_config.name
    os.environ["OPENAI_API_KEY"] = "test-api-key-for-unit-tests"
    
    # Store for cleanup
    config._temp_config_file = _temp_config.name


def pytest_unconfigure(config):
    """Cleanup temporary config file after all tests."""
    if hasattr(config, '_temp_config_file'):
        try:
            os.unlink(config._temp_config_file)
        except:
            pass


def wait_for_condition(check_fn, timeout=1.0, interval=0.01, error_msg="Condition not met"):
    """
    Poll a condition with short intervals instead of blocking sleep.
    
    This replaces time.sleep() anti-pattern for waiting on async operations.
    Per TESTING_GUIDELINES.md: Unit tests must not use time.sleep() for synchronization.
    
    Args:
        check_fn: Callable that returns True when condition is met
        timeout: Maximum time to wait in seconds (default: 1.0)
        interval: Polling interval in seconds (default: 0.01 = 10ms)
        error_msg: Error message if timeout is reached
        
    Returns:
        True if condition met, raises AssertionError if timeout
        
    Example:
        wait_for_condition(lambda: config.value == 42, timeout=1.0)
        
    Performance:
        - Replaces time.sleep(3) with max 1s wait (but usually <100ms)
        - 30x faster than blocking sleep for typical async operations
    """
    start = time.time()
    last_exception = None
    
    while time.time() - start < timeout:
        try:
            if check_fn():
                return True
        except Exception as e:
            # Store exception but keep polling (condition might not be ready yet)
            last_exception = e
        time.sleep(interval)
    
    # Timeout reached
    if last_exception:
        raise AssertionError(f"{error_msg} (timeout after {timeout}s, last error: {last_exception})")
    raise AssertionError(f"{error_msg} (timeout after {timeout}s)")


@pytest.fixture
def wait_for():
    """Fixture to provide wait_for_condition helper to tests."""
    return wait_for_condition


@pytest.fixture
def mock_llm_mode():
    """Enable mock LLM mode for unit tests"""
    with patch.dict(os.environ, {"MOCK_LLM_MODE": "true"}):
        yield


@pytest.fixture
def client():
    """Create FastAPI test client for unit tests"""
    from fastapi.testclient import TestClient
    
    # Config file already set at module level
    from src.main import app
    return TestClient(app)






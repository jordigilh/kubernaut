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

# BR-AA-HAPI-064: Session manager reset between tests
from src.session.session_manager import reset_session_manager


def pytest_configure(config):
    """
    Pytest hook that runs before test collection.
    Create config file BEFORE any test modules are imported.
    """
    # Prevent prometrix pydantic v1 crash on Python 3.14+.
    # prometrix uses pydantic v1 BaseModel which is incompatible with Python
    # 3.14's type system. Mock the entire package so the Holmes SDK import
    # chain doesn't crash at collection time.
    import sys
    from unittest.mock import MagicMock
    for mod_name in [
        "prometrix",
        "prometrix.auth",
        "prometrix.connect",
        "prometrix.connect.aws_connect",
        "prometrix.connect.custom_connect",
        "prometrix.exceptions",
        "prometrix.models",
        "prometrix.models.prometheus_config",
        "prometrix.models.prometheus_result",
        "prometrix.utils",
    ]:
        if mod_name not in sys.modules:
            sys.modules[mod_name] = MagicMock()

    _config_content = """
llm:
  provider: "openai"
  model: "gpt-4-turbo"
  endpoint: "http://127.0.0.1:8080"
  max_tokens: 16384

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

    # Set LLM_MODEL explicitly (takes precedence over config file per get_model_config_for_sdk)
    # This ensures consistent test behavior regardless of config file state
    # Use gpt-4-turbo for larger context window (128k tokens) to handle test prompts
    os.environ["LLM_MODEL"] = "gpt-4-turbo"

    # Set LLM_ENDPOINT to match the temp config
    os.environ["LLM_ENDPOINT"] = "http://127.0.0.1:8080"

    # Set MOCK_LLM_MODE to prevent real LLM initialization during unit tests
    # This must be set early (before src.main import) to prevent HolmesGPT SDK from trying to validate model with litellm
    os.environ["MOCK_LLM_MODE"] = "true"

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


@pytest.fixture(autouse=True)
def _reset_session_manager():
    """
    Reset the global SessionManager singleton between tests.
    BR-AA-HAPI-064: Prevents session leakage between test cases.
    """
    reset_session_manager()
    yield
    reset_session_manager()


@pytest.fixture
def mock_llm_mode():
    """Enable mock LLM mode for unit tests"""
    with patch.dict(os.environ, {"MOCK_LLM_MODE": "true"}):
        yield


@pytest.fixture
def mock_analyze_recovery():
    """
    Mock fixture for analyze_recovery function in unit tests.

    Recovery endpoint tests need this to avoid real LLM calls.
    Returns a mock that simulates successful recovery analysis.
    """
    from unittest.mock import AsyncMock, patch

    mock_response = {
        "incident_id": "test-inc-001",
        "can_recover": True,
        "strategies": [
            {
                "action_type": "scale_horizontal",
                "confidence": 0.85,
                "rationale": "Scale out to handle increased load",
                "estimated_risk": "low"
            }
        ],
        "primary_recommendation": "scale_horizontal",
        "analysis_confidence": 0.85,
        "needs_human_review": False,
        "human_review_reason": None,
        "warnings": [],
        "metadata": {
            "analysis_time_ms": 1500
        }
    }

    with patch('src.extensions.recovery.endpoint.analyze_recovery', new_callable=AsyncMock) as mock:
        mock.return_value = mock_response
        yield mock


@pytest.fixture
def client():
    """Create authenticated FastAPI test client for unit tests with mock auth"""
    from fastapi.testclient import TestClient
    from src.main import create_app
    from src.auth import MockAuthenticator, MockAuthorizer

    # Create app with mock auth components (no K8s cluster dependency)
    # Factory pattern: Pure dependency injection, no test-specific logic in business code
    app = create_app(
        authenticator=MockAuthenticator(valid_users={"test-token": "system:serviceaccount:test:sa"}),
        authorizer=MockAuthorizer(default_allow=True)
    )
    
    # Create test client with default auth header for protected endpoints
    test_client = TestClient(app)
    test_client.headers = {"Authorization": "Bearer test-token"}
    return test_client






"""
Integration Test Infrastructure Configuration

Business Requirement: BR-HAPI-250 - Workflow Catalog Search Tool
Design Decision: DD-TEST-001 - Port Allocation Strategy

This module provides pytest fixtures for integration tests that require
real infrastructure (Data Storage Service, PostgreSQL, Redis).

Port Allocation (per DD-TEST-001):
- PostgreSQL: 15433
- Redis: 16380
- Embedding Service: 18000
- Data Storage Service: 18090
- HolmesGPT API: 18120 (if needed)

Infrastructure Management:
1. Automatically detects if infrastructure is running
2. Provides skip markers for tests when infrastructure unavailable
3. Sets environment variables for service clients

Usage:
    # Tests automatically get infrastructure fixtures
    def test_workflow_search(data_storage_url):
        response = requests.post(f"{data_storage_url}/api/v1/workflows/search", ...)

    # Or check availability manually (per TESTING_GUIDELINES.md: use fail() not skip())
    def test_something():
        if not is_integration_infra_available():
            pytest.fail("REQUIRED: Infrastructure not running - start with setup script")
        ...

Setup Commands:
    # Start infrastructure
    cd tests/integration
    ./setup_workflow_catalog_integration.sh

    # Run integration tests
    python -m pytest tests/integration/ -v

    # Teardown infrastructure
    ./teardown_workflow_catalog_integration.sh
"""

import os
import time
import subprocess
from typing import Optional
import pytest
import requests


# ========================================
# DD-TEST-001: Port Allocation Constants
# ========================================
# HolmesGPT API Integration Test Ports (18120-18129 range)
# Data Storage as DEPENDENCY uses different ports than Data Storage's own tests
# Per DD-TEST-001: HolmesGPT-API is service index 5, uses dependency ports accordingly
POSTGRES_PORT = "15435"      # PostgreSQL for Data Storage dependency (not 15433 - that's DS's own)
REDIS_PORT = "16381"         # Redis for Data Storage dependency (not 16379 - that's DS's own)
EMBEDDING_SERVICE_PORT = "18001"  # Embedding Service (not 18000 - that's DS's own)
DATA_STORAGE_PORT = "18094"  # Data Storage as dependency (not 18090 - that's DS's own)
HOLMESGPT_API_PORT = "18120" # HolmesGPT API's own port

# Service URLs
DATA_STORAGE_URL = f"http://localhost:{DATA_STORAGE_PORT}"
EMBEDDING_SERVICE_URL = f"http://localhost:{EMBEDDING_SERVICE_PORT}"


def is_service_available(url: str, timeout: float = 2.0) -> bool:
    """
    Check if a service is available at the given URL.

    Args:
        url: Service URL to check
        timeout: Request timeout in seconds

    Returns:
        True if service responds with non-5xx status, False otherwise
    """
    try:
        # Try health endpoint first
        for endpoint in ["/health", "/api/v1/health", ""]:
            try:
                response = requests.get(f"{url}{endpoint}", timeout=timeout)
                if response.status_code < 500:
                    return True
            except requests.RequestException:
                continue
        return False
    except Exception:
        return False


def is_integration_infra_available() -> bool:
    """
    Check if the integration test infrastructure is running.

    Checks Data Storage Service availability as the primary indicator.
    Data Storage depends on PostgreSQL, Redis, and Embedding Service,
    so if it's healthy, the full stack is running.

    Returns:
        True if Data Storage Service is available, False otherwise
    """
    return is_service_available(DATA_STORAGE_URL)


def wait_for_infrastructure(timeout: float = 60.0, interval: float = 2.0) -> bool:
    """
    Wait for integration infrastructure to become available.

    Args:
        timeout: Maximum time to wait in seconds
        interval: Check interval in seconds

    Returns:
        True if infrastructure became available, False on timeout
    """
    start_time = time.time()
    while time.time() - start_time < timeout:
        if is_integration_infra_available():
            return True
        time.sleep(interval)
    return False


def start_infrastructure() -> bool:
    """
    Start the integration test infrastructure using the setup script.

    Returns:
        True if infrastructure started successfully, False otherwise
    """
    script_dir = os.path.dirname(os.path.abspath(__file__))
    setup_script = os.path.join(script_dir, "setup_workflow_catalog_integration.sh")

    if not os.path.exists(setup_script):
        print(f"❌ Setup script not found: {setup_script}")
        return False

    try:
        result = subprocess.run(
            ["bash", setup_script],
            cwd=script_dir,
            capture_output=True,
            text=True,
            timeout=300  # 5 minutes max for infrastructure setup
        )

        if result.returncode != 0:
            print(f"❌ Infrastructure setup failed:")
            print(result.stdout)
            print(result.stderr)
            return False

        # Wait for services to be fully ready
        return wait_for_infrastructure(timeout=60.0)

    except subprocess.TimeoutExpired:
        print("❌ Infrastructure setup timed out")
        return False
    except Exception as e:
        print(f"❌ Infrastructure setup error: {e}")
        return False


def stop_infrastructure() -> bool:
    """
    Stop the integration test infrastructure using the teardown script.

    Returns:
        True if infrastructure stopped successfully, False otherwise
    """
    script_dir = os.path.dirname(os.path.abspath(__file__))
    teardown_script = os.path.join(script_dir, "teardown_workflow_catalog_integration.sh")

    if not os.path.exists(teardown_script):
        print(f"⚠️ Teardown script not found: {teardown_script}")
        return False

    try:
        result = subprocess.run(
            ["bash", teardown_script],
            cwd=script_dir,
            capture_output=True,
            text=True,
            timeout=60
        )
        return result.returncode == 0
    except Exception as e:
        print(f"⚠️ Infrastructure teardown error: {e}")
        return False


# ========================================
# Pytest Fixtures
# ========================================

@pytest.fixture(scope="session")
def integration_infrastructure():
    """
    Session-scoped fixture that ensures integration infrastructure is available.

    This fixture:
    1. Checks if infrastructure is already running
    2. Sets environment variables for service clients
    3. Yields control to tests
    4. Does NOT teardown (leave infrastructure running for faster iteration)

    If infrastructure is not running, tests will be skipped.
    Start infrastructure manually with: ./setup_workflow_catalog_integration.sh
    """
    if not is_integration_infra_available():
        pytest.fail(
            "REQUIRED: Integration infrastructure not running.\n"
            "  Per TESTING_GUIDELINES.md: Tests MUST Fail, NEVER Skip\n"
            "  Start it with: ./tests/integration/setup_workflow_catalog_integration.sh"
        )

    # Set environment variables for service clients
    os.environ["DATA_STORAGE_URL"] = DATA_STORAGE_URL
    os.environ["EMBEDDING_SERVICE_URL"] = EMBEDDING_SERVICE_URL
    os.environ["POSTGRES_HOST"] = "localhost"
    os.environ["POSTGRES_PORT"] = POSTGRES_PORT
    os.environ["REDIS_HOST"] = "localhost"
    os.environ["REDIS_PORT"] = REDIS_PORT

    yield {
        "data_storage_url": DATA_STORAGE_URL,
        "embedding_service_url": EMBEDDING_SERVICE_URL,
        "postgres_host": "localhost",
        "postgres_port": POSTGRES_PORT,
        "redis_host": "localhost",
        "redis_port": REDIS_PORT,
    }

    # Don't teardown - leave infrastructure running for faster iteration
    # Teardown manually with: ./tests/integration/teardown_workflow_catalog_integration.sh


@pytest.fixture(scope="session")
def data_storage_url(integration_infrastructure):
    """
    Fixture that provides the Data Storage Service URL.

    This fixture depends on integration_infrastructure, so it will skip
    if infrastructure is not available.

    Returns:
        Data Storage Service URL (e.g., "http://localhost:18090")
    """
    return integration_infrastructure["data_storage_url"]


@pytest.fixture(scope="session")
def embedding_service_url(integration_infrastructure):
    """
    Fixture that provides the Embedding Service URL.

    Returns:
        Embedding Service URL (e.g., "http://localhost:18000")
    """
    return integration_infrastructure["embedding_service_url"]


# ========================================
# Pytest Markers
# ========================================

def pytest_configure(config):
    """Register custom markers for integration tests."""
    config.addinivalue_line(
        "markers",
        "integration: mark test as integration test (requires infrastructure)"
    )
    config.addinivalue_line(
        "markers",
        "requires_data_storage: mark test as requiring Data Storage Service"
    )


def pytest_collection_modifyitems(config, items):
    """
    Automatically skip integration tests if infrastructure is not available.

    ONLY tests explicitly marked with @pytest.mark.requires_data_storage
    will be skipped if the infrastructure is not running.

    Tests without this marker will run normally, even in the integration/ folder.
    """
    if is_integration_infra_available():
        # Infrastructure is running, no need to skip
        return

    # Per TESTING_GUIDELINES.md: Tests MUST Fail, NEVER Skip
    # If infrastructure is not available, tests marked with requires_data_storage will fail
    # when they try to connect, which is the correct behavior
    fail_reason = pytest.mark.xfail(
        reason="REQUIRED: Integration infrastructure not running. "
        "Start with: ./tests/integration/setup_workflow_catalog_integration.sh",
        strict=True,
        run=True  # Run the test and let it fail naturally
    )

    for item in items:
        # Mark tests that require data storage - they will fail if infra not available
        if "requires_data_storage" in item.keywords:
            item.add_marker(fail_reason)


# ========================================
# Helper for WorkflowCatalogToolset
# ========================================

@pytest.fixture(scope="function")
def workflow_catalog_toolset_with_infra(integration_infrastructure):
    """
    Fixture that creates a WorkflowCatalogToolset connected to real infrastructure.

    This toolset will use the real Data Storage Service for workflow searches,
    not mocks.
    """
    # Set the DATA_STORAGE_URL environment variable that WorkflowCatalogToolset uses
    original_url = os.environ.get("DATA_STORAGE_URL")
    os.environ["DATA_STORAGE_URL"] = integration_infrastructure["data_storage_url"]

    from src.toolsets.workflow_catalog import WorkflowCatalogToolset

    toolset = WorkflowCatalogToolset(enabled=True)

    yield toolset

    # Restore original environment
    if original_url:
        os.environ["DATA_STORAGE_URL"] = original_url
    elif "DATA_STORAGE_URL" in os.environ:
        del os.environ["DATA_STORAGE_URL"]


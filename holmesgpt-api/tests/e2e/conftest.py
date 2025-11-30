"""
E2E Test Infrastructure Configuration

Business Requirement: BR-HAPI-250 - Workflow Catalog Search Tool
Design Decision: DD-TEST-001 - Port Allocation Strategy

This module provides pytest fixtures for E2E tests that require
real infrastructure (Data Storage Service, PostgreSQL, Redis, Embedding Service).

E2E tests are different from integration tests:
- They require REAL infrastructure to be running
- They test full end-to-end flows
- They are expensive and should be run in CI or manually

Port Allocation (per DD-TEST-001):
- PostgreSQL: 15433
- Redis: 16380
- Embedding Service: 18000
- Data Storage Service: 18090

Setup:
    ./tests/integration/setup_workflow_catalog_integration.sh

Teardown:
    ./tests/integration/teardown_workflow_catalog_integration.sh
"""

import os
import pytest
import requests


# ========================================
# DD-TEST-001: Port Allocation Constants
# ========================================
POSTGRES_PORT = "15433"
REDIS_PORT = "16380"
EMBEDDING_SERVICE_PORT = "18000"
DATA_STORAGE_PORT = "18090"

# Service URLs
DATA_STORAGE_URL = f"http://localhost:{DATA_STORAGE_PORT}"
EMBEDDING_SERVICE_URL = f"http://localhost:{EMBEDDING_SERVICE_PORT}"


def is_service_available(url: str, timeout: float = 2.0) -> bool:
    """Check if a service is available at the given URL."""
    try:
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


def is_e2e_infra_available() -> bool:
    """
    Check if E2E infrastructure is running.

    Checks Data Storage Service as primary indicator.
    """
    return is_service_available(DATA_STORAGE_URL)


# ========================================
# Pytest Configuration
# ========================================

def pytest_configure(config):
    """Register custom markers for E2E tests."""
    config.addinivalue_line(
        "markers",
        "e2e: mark test as end-to-end test (requires full infrastructure)"
    )


def pytest_collection_modifyitems(config, items):
    """
    Skip E2E tests if infrastructure is not available.

    All tests in the e2e directory require real infrastructure.
    """
    if is_e2e_infra_available():
        return

    skip_reason = pytest.mark.skip(
        reason="E2E infrastructure not running. "
        "Start with: ./tests/integration/setup_workflow_catalog_integration.sh"
    )

    for item in items:
        item.add_marker(skip_reason)


# ========================================
# Pytest Fixtures
# ========================================

@pytest.fixture(scope="session")
def integration_infrastructure():
    """
    Session-scoped fixture that ensures E2E infrastructure is available.

    If infrastructure is not running, tests will be skipped.
    """
    if not is_e2e_infra_available():
        pytest.skip(
            "E2E infrastructure not running. "
            "Start with: ./tests/integration/setup_workflow_catalog_integration.sh"
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


@pytest.fixture(scope="session")
def data_storage_url(integration_infrastructure):
    """Fixture that provides the Data Storage Service URL."""
    return integration_infrastructure["data_storage_url"]




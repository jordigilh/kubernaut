"""
Pytest Configuration for HAPI E2E Tests

Provides fixtures for E2E testing of HolmesGPT API service.
"""

import pytest
import os


@pytest.fixture(scope="session")
def hapi_service_url():
    """
    Fixture providing HAPI service URL for E2E tests.

    For local E2E testing (this session):
    - Uses localhost:18120 (HAPI running via uvicorn)

    For Kind E2E testing (future):
    - Uses http://hapi-service:8080 (containerized HAPI in Kind)
    """
    # Check if running in Kind (E2E) or locally
    url = os.getenv("HAPI_SERVICE_URL", "http://localhost:18120")
    return url


@pytest.fixture(scope="session")
def data_storage_url():
    """
    Fixture providing Data Storage service URL for E2E tests.

    For local E2E testing:
    - Uses localhost:18098 (integration infrastructure)

    For Kind E2E testing (future):
    - Uses http://data-storage:8080 (containerized in Kind)
    """
    url = os.getenv("DATA_STORAGE_URL", "http://localhost:18098")
    return url




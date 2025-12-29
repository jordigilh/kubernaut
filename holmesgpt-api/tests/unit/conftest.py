"""
Unit Test Configuration

Unit tests validate implementation correctness without external dependencies.
Per TESTING_GUIDELINES.md: Unit tests use mocks for all external services.
"""

import pytest
import os
from unittest.mock import patch


@pytest.fixture
def mock_llm_mode():
    """Enable mock LLM mode for unit tests"""
    with patch.dict(os.environ, {"MOCK_LLM_MODE": "true"}):
        yield


@pytest.fixture
def client():
    """Create FastAPI test client for unit tests"""
    from fastapi.testclient import TestClient

    # Set mock mode before importing app
    with patch.dict(os.environ, {"MOCK_LLM_MODE": "true"}):
        from src.main import app
        return TestClient(app)






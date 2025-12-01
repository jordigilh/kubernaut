"""
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
"""

"""
E2E Test Configuration and Fixtures

Provides session-scoped fixtures for E2E testing with mock LLM server.
"""

import os
import sys
import pytest

# Add src to path
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', '..', 'src'))


def pytest_configure(config):
    """Register custom markers."""
    config.addinivalue_line(
        "markers", "e2e: mark test as E2E test"
    )


@pytest.fixture(scope="session", autouse=True)
def setup_e2e_environment():
    """Set up environment variables for E2E testing."""
    # These will be overridden by individual fixtures, but set defaults
    os.environ.setdefault("LLM_PROVIDER", "openai")
    os.environ.setdefault("LLM_MODEL", "mock-model")
    os.environ.setdefault("OPENAI_API_KEY", "mock-key-for-e2e")
    os.environ.setdefault("DATA_STORAGE_URL", "http://mock-data-storage:8080")
    os.environ.setdefault("DATA_STORAGE_TIMEOUT", "10")

    yield

    # Cleanup if needed


@pytest.fixture(scope="session")
def mock_llm_server_e2e():
    """
    Session-scoped mock LLM server with tool call support for E2E tests.

    This server:
    - Returns tool calls (not just text)
    - Tracks all tool calls for validation
    - Supports multi-turn conversations
    """
    from tests.mock_llm_server import MockLLMServer

    with MockLLMServer(force_text_response=False) as server:
        # Set environment to use this server
        os.environ["LLM_ENDPOINT"] = server.url
        yield server

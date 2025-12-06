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

Provides session-scoped fixtures for E2E testing:
- Kind cluster with Data Storage stack (programmatic, per DD-TEST-001)
- Mock LLM server (per TESTING_GUIDELINES.md - LLM mock only due to cost)

Per TESTING_GUIDELINES.md section 4:
- E2E tests must use all real services EXCEPT the LLM
- If Data Storage is unavailable, E2E tests should FAIL, not skip
"""

import os
import sys
import pytest

# Add src to path
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', '..', 'src'))

# Import infrastructure utilities
from tests.infrastructure import KindCluster, DataStorageDeployment, PORTS


def pytest_configure(config):
    """Register custom markers."""
    config.addinivalue_line(
        "markers", "e2e: mark test as E2E test"
    )
    config.addinivalue_line(
        "markers", "requires_data_storage: mark test as requiring real Data Storage service"
    )


# ============================================================================
# Session-Scoped Infrastructure Fixtures
# ============================================================================

@pytest.fixture(scope="session")
def kind_cluster():
    """
    Session-scoped Kind cluster for E2E tests.

    Per DD-TEST-001 v1.2: Uses dedicated ports for HAPI (8088/30088).
    Per ADR-016: Uses Podman as container runtime.

    The cluster is created once per test session and reused.
    """
    cluster = KindCluster("holmesgpt-e2e")

    # Create cluster (will reuse if already exists)
    cluster.create()
    cluster.wait_for_ready()

    yield cluster

    # Note: We don't delete the cluster by default to speed up development
    # Set HAPI_E2E_CLEANUP=true to delete after tests
    if os.environ.get("HAPI_E2E_CLEANUP", "").lower() == "true":
        cluster.delete()


@pytest.fixture(scope="session")
def data_storage_stack(kind_cluster):
    """
    Session-scoped Data Storage stack deployment.

    Deploys:
    - PostgreSQL + pgvector (port 5488/30488)
    - Redis (port 6388/30388)
    - Embedding Service (port 8188/30288) - optional
    - Data Storage Service (port 8089/30089)

    Per TESTING_GUIDELINES.md: Real infrastructure, mock LLM only.
    """
    deployment = DataStorageDeployment(kind_cluster)

    # Deploy full stack (skip embedding if HAPI_SKIP_EMBEDDING=true)
    skip_embedding = os.environ.get("HAPI_SKIP_EMBEDDING", "").lower() == "true"
    data_storage_url = deployment.deploy(skip_embedding=skip_embedding)

    yield data_storage_url

    # Teardown namespace
    if os.environ.get("HAPI_E2E_CLEANUP", "").lower() == "true":
        deployment.teardown()


@pytest.fixture(scope="session", autouse=True)
def setup_e2e_environment(data_storage_stack):
    """
    Set up environment variables for E2E testing.

    Uses real Data Storage URL from Kind cluster deployment.
    """
    # LLM is mocked (per TESTING_GUIDELINES.md - cost constraint)
    os.environ.setdefault("LLM_PROVIDER", "openai")
    os.environ.setdefault("LLM_MODEL", "mock-model")
    os.environ.setdefault("OPENAI_API_KEY", "mock-key-for-e2e")

    # Data Storage is REAL (from Kind cluster deployment)
    os.environ["DATA_STORAGE_URL"] = data_storage_stack
    os.environ.setdefault("DATA_STORAGE_TIMEOUT", "30")

    print(f"\n{'='*60}")
    print(f"E2E Environment Ready")
    print(f"{'='*60}")
    print(f"Data Storage URL: {data_storage_stack}")
    print(f"LLM: Mock (per TESTING_GUIDELINES.md)")
    print(f"{'='*60}\n")

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

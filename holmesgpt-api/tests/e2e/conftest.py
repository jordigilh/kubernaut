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

V1.0 ARCHITECTURE (December 2025):
- Uses SHARED Go infrastructure from test/infrastructure/*.go
- Data Storage stack deployed via `make test-e2e-datastorage`
- HAPI E2E tests connect to existing NodePort services
- No Python-based Kind/Podman management needed

DD-TEST-001 v1.1 COMPLIANCE (Infrastructure Image Cleanup):
- HAPI E2E tests do NOT build service images for Kind (runs separately)
- Infrastructure cleanup handled by Go test framework (test/infrastructure/)
- No service image cleanup needed for HAPI E2E tests
- Integration test cleanup in tests/integration/conftest.py handles HAPI-specific infrastructure

Per TESTING_GUIDELINES.md section 4:
- E2E tests must use all real services EXCEPT the LLM
- If Data Storage is unavailable, E2E tests should FAIL, not skip

USAGE:
  # Option 1: Run with Go infrastructure (recommended)
  make test-e2e-datastorage          # Set up infrastructure (once)
  make test-e2e-holmesgpt            # Run HAPI E2E tests

  # Option 2: Full suite (sets up infra + runs tests)
  make test-e2e-holmesgpt-full
"""

import os
import sys
import time
import pytest
import requests

# Add src to path
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', '..', 'src'))

# Import workflow fixtures (DD-API-001 compliant)
from tests.fixtures import bootstrap_workflows, get_test_workflows


def pytest_configure(config):
    """Register custom markers."""
    config.addinivalue_line(
        "markers", "e2e: mark test as E2E test"
    )
    config.addinivalue_line(
        "markers", "requires_data_storage: mark test as requiring real Data Storage service"
    )


# ============================================================================
# Go Infrastructure Detection
# ============================================================================

def _check_go_infrastructure():
    """
    Check if Go-managed Data Storage infrastructure is available.

    The Go infrastructure exposes services via NodePort:
    - Data Storage: http://localhost:8081 (NodePort 30081)
    - PostgreSQL: localhost:5432 (NodePort 30432)

    Returns:
        tuple: (is_available, data_storage_url)
    """
    # Check if using Go infrastructure (set by Makefile)
    use_go_infra = os.environ.get("HAPI_USE_GO_INFRA", "").lower() == "true"

    # NodePort URLs from Go infrastructure (kind-datastorage-config.yaml)
    data_storage_url = os.environ.get("DATA_STORAGE_URL", "http://localhost:8081")

    try:
        # Check if Data Storage is responding
        response = requests.get(f"{data_storage_url}/health/ready", timeout=5)
        if response.status_code == 200:
            print(f"\n✅ Go infrastructure detected: Data Storage at {data_storage_url}")
            return True, data_storage_url
    except requests.exceptions.RequestException:
        pass

    if use_go_infra:
        # User explicitly requested Go infra but it's not available
        pytest.fail(
            f"HAPI_USE_GO_INFRA=true but Data Storage not available at {data_storage_url}.\n"
            "Run 'make test-e2e-datastorage' first to set up infrastructure."
        )

    return False, data_storage_url


# ============================================================================
# Session-Scoped Infrastructure Fixtures
# ============================================================================

@pytest.fixture(scope="session")
def data_storage_stack():
    """
    Session-scoped Data Storage stack.

    V1.0: Uses Go-managed infrastructure via NodePort.
    The infrastructure should be set up via `make test-e2e-datastorage`.

    Per TESTING_GUIDELINES.md: Real infrastructure, mock LLM only.
    """
    is_available, data_storage_url = _check_go_infrastructure()

    if not is_available:
        pytest.fail(
            "REQUIRED: Data Storage infrastructure not available.\n"
            "  Per TESTING_GUIDELINES.md: E2E tests must use real services\n"
            "  Per TESTING_GUIDELINES.md: Tests MUST Fail, NEVER Skip\n"
            "  Run 'make test-e2e-datastorage' first, then 'make test-e2e-holmesgpt'.\n"
            "  Or run 'make test-e2e-holmesgpt-full' for complete setup."
        )

    # Wait for full readiness
    print(f"\n⏳ Verifying Data Storage readiness at {data_storage_url}...")
    max_retries = 30
    for i in range(max_retries):
        try:
            response = requests.get(f"{data_storage_url}/health/ready", timeout=5)
            if response.status_code == 200:
                print(f"✅ Data Storage is ready")
                break
        except requests.exceptions.RequestException:
            pass

        if i == max_retries - 1:
            pytest.fail(f"Data Storage not ready after {max_retries} attempts")
        time.sleep(2)

    yield data_storage_url


@pytest.fixture(scope="session")
def hapi_service_url():
    """
    HAPI service URL for E2E tests.

    Uses HAPI_BASE_URL environment variable (set by E2E infrastructure).
    For E2E: http://localhost:30120 (Kind NodePort)
    For Integration: http://127.0.0.1:18120 (local server)

    Fallback: HAPI_SERVICE_URL for backwards compatibility
    Default: http://127.0.0.1:18120 (integration test port)
    """
    url = os.environ.get("HAPI_BASE_URL") or os.environ.get("HAPI_SERVICE_URL", "http://127.0.0.1:18120")

    # Verify HAPI is available
    try:
        response = requests.get(f"{url}/health/ready", timeout=5)
        if response.status_code == 200:
            print(f"✅ HAPI service ready at {url}")
            return url
    except requests.exceptions.RequestException as e:
        pytest.fail(
            f"HAPI service not available at {url}.\n"
            f"Start HAPI with: make run-holmesgpt\n"
            f"Error: {e}"
        )

    return url


@pytest.fixture
def mock_llm_server_e2e(mock_llm_service_e2e):
    """
    Backward compatibility alias for tests using old fixture name.
    
    V2.0: Redirects to mock_llm_service_e2e (standalone Mock LLM service).
    """
    return mock_llm_service_e2e


@pytest.fixture(scope="session", autouse=True)
def setup_e2e_environment(data_storage_stack, mock_llm_service_e2e):
    """
    Set up environment variables for E2E testing.

    V2.0 (Mock LLM Migration - January 2026):
    - Uses real Data Storage URL from Go-managed Kind cluster
    - Uses standalone Mock LLM service deployed in Kind (ClusterIP)
    - LLM environment variables set by mock_llm_service_e2e fixture
    """
    # LLM configuration now handled by mock_llm_service_e2e fixture
    # This ensures tests use the standalone Mock LLM service in Kind
    os.environ.setdefault("OPENAI_API_KEY", "mock-key-for-e2e")

    # Data Storage is REAL (from Go-managed Kind cluster)
    os.environ["DATA_STORAGE_URL"] = data_storage_stack
    os.environ.setdefault("DATA_STORAGE_TIMEOUT", "30")

    print(f"\n{'='*60}")
    print(f"E2E Environment Ready (V2.0 - Mock LLM Migration)")
    print(f"{'='*60}")
    print(f"Data Storage URL: {data_storage_stack}")
    print(f"Mock LLM URL: {mock_llm_service_e2e.url}")
    print(f"LLM: Standalone Mock LLM (ClusterIP in kubernaut-system)")
    print(f"{'='*60}\n")

    yield


@pytest.fixture(scope="session")
def mock_llm_service_e2e():
    """
    Session-scoped Mock LLM service for E2E tests.

    V2.0 (Mock LLM Migration - January 2026):
    - Uses standalone Mock LLM service deployed in Kind cluster
    - Service accessible at http://mock-llm:8080 (ClusterIP in kubernaut-system)
    - Deployed via: kubectl apply -k deploy/mock-llm/
    - No embedded server - uses external service

    This service:
    - Returns tool calls (not just text responses)
    - Supports OpenAI-compatible API
    - Handles multi-turn conversations
    - Deployed in kubernaut-system namespace (same as HAPI/DataStorage)
    """
    # Mock LLM service URL from Kind cluster (ClusterIP internal URL)
    # Note: Tests run inside Kind cluster, so use short DNS name
    mock_llm_url = os.environ.get("LLM_ENDPOINT", "http://mock-llm:8080")
    
    print(f"\n⏳ Verifying Mock LLM service at {mock_llm_url}...")
    
    # Verify Mock LLM is available
    max_retries = 30
    for i in range(max_retries):
        try:
            response = requests.get(f"{mock_llm_url}/health", timeout=5)
            if response.status_code == 200:
                print(f"✅ Mock LLM service is ready")
                # Set environment for tests
                os.environ["LLM_ENDPOINT"] = mock_llm_url
                os.environ["LLM_MODEL"] = "mock-model"
                os.environ["LLM_PROVIDER"] = "openai"
                
                # Return simple object with URL for compatibility
                class MockLLMService:
                    def __init__(self, url):
                        self.url = url
                
                yield MockLLMService(mock_llm_url)
                return
        except requests.exceptions.RequestException:
            pass
        
        if i == max_retries - 1:
            pytest.fail(
                f"Mock LLM service not ready at {mock_llm_url} after {max_retries} attempts.\n"
                "Ensure Mock LLM is deployed: kubectl apply -k deploy/mock-llm/"
            )
        time.sleep(2)


@pytest.fixture
def mock_config():
    """
    Fixture to provide mock configuration for tests.
    Prevents MagicMock serialization issues in audit events.
    """
    return {
        "llm_provider": "mock",
        "llm_model": "mock-model",
        "data_storage_url": os.environ.get("DATA_STORAGE_URL", "http://localhost:8081"),
    }


@pytest.fixture(scope="session")
def test_workflows_bootstrapped(data_storage_stack):
    """
    Auto-bootstrap test workflows for E2E tests.

    DD-API-001 COMPLIANCE: Uses Python fixture with OpenAPI client instead of shell script.

    Benefits over shell script:
    - Type-safe with Pydantic models
    - Reusable across test files
    - DD-API-001 compliant (uses OpenAPI client)
    - Easy to import and customize
    - Better error handling

    Usage in tests:
        def test_workflow_search(test_workflows_bootstrapped, data_storage_stack):
            # Workflows automatically available
            ...
    """
    data_storage_url = data_storage_stack
    print(f"\n🔧 Bootstrapping test workflows to {data_storage_url}...")

    try:
        results = bootstrap_workflows(data_storage_url)
        print(f"  ✅ Created: {len(results['created'])}")
        print(f"  ⚠️  Existing: {len(results['existing'])}")
        print(f"  ❌ Failed: {len(results['failed'])}")

        if results['failed']:
            for failure in results['failed']:
                print(f"    - {failure['workflow']}: {failure['error']}")

        # Return results for test assertions if needed
        return results

    except Exception as e:
        pytest.fail(f"Failed to bootstrap test workflows: {e}")


@pytest.fixture
def oomkilled_workflows():
    """Get OOMKilled workflow fixtures for testing"""
    from tests.fixtures import get_oomkilled_workflows
    return get_oomkilled_workflows()


@pytest.fixture
def crashloop_workflows():
    """Get CrashLoopBackOff workflow fixtures for testing"""
    from tests.fixtures import get_crashloop_workflows
    return get_crashloop_workflows()


@pytest.fixture
def all_test_workflows():
    """Get all test workflow fixtures"""
    return get_test_workflows()


@pytest.fixture
def ensure_test_workflows(test_workflows_bootstrapped, data_storage_stack):
    """
    Verify test workflows are available in Data Storage.

    This is a compatibility fixture for tests that expect `ensure_test_workflows`.
    In E2E, workflows are bootstrapped by `test_workflows_bootstrapped` fixture.

    Returns:
        Bootstrap results from test_workflows_bootstrapped
    """
    # test_workflows_bootstrapped already created the workflows
    # This fixture just verifies they're available
    results = test_workflows_bootstrapped

    if results and results.get('failed'):
        pytest.fail(f"Some test workflows failed to bootstrap: {results['failed']}")

    total_available = len(results.get('created', [])) + len(results.get('existing', []))
    print(f"✅ Test workflows available: {total_available}")

    return results

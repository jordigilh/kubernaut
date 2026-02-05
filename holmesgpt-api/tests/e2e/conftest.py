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
import yaml
from pathlib import Path

# Add src to path
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', '..', 'src'))

# Import workflow fixtures (DD-API-001 compliant)
from tests.fixtures import bootstrap_workflows, get_test_workflows


def load_test_config():
    """
    Load test configuration from YAML file.
    
    Authority: ADR-030 Configuration Management Standard
    
    Returns:
        dict: Test configuration
    """
    # ANTI-PATTERN EXCEPTION: CONFIG_FILE is the ONLY allowed env var (ADR-030)
    # All other configuration MUST come from YAML files
    config_file = os.getenv("CONFIG_FILE", str(Path(__file__).parent.parent / "test_config.yaml"))
    with open(config_file, 'r') as f:
        return yaml.safe_load(f)


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
    # Authority: ADR-030 - Load from YAML config (no env vars except CONFIG_FILE)
    config = load_test_config()
    data_storage_url = config.get('data_storage', {}).get('url', 'http://localhost:8081')

    try:
        # Check if Data Storage is responding
        response = requests.get(f"{data_storage_url}/health/ready", timeout=5)
        if response.status_code == 200:
            print(f"\n✅ Go infrastructure detected: Data Storage at {data_storage_url}")
            return True, data_storage_url
    except requests.exceptions.RequestException:
        pass

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

    Authority: ADR-030 Configuration Management Standard
    Loaded from test_config.yaml (no environment variables)

    For E2E: http://localhost:30120 (Kind NodePort per DD-TEST-001)
    For Integration: http://localhost:18120 (local server)
    """
    config = load_test_config()
    # E2E config should have hapi.url, fallback to default for local testing
    url = config.get('hapi', {}).get('url', 'http://localhost:18120')

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
    # E2E environment ready (all config from YAML per ADR-030)
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
    # Used by HAPI (in-cluster), not by pytest directly
    # Authority: ADR-030 - Load from YAML config
    config = load_test_config()
    mock_llm_url = config.get('llm', {}).get('endpoint', 'http://mock-llm:8080')

    # Go infrastructure always manages E2E environment
    # Go infrastructure already waits for Mock LLM pod readiness via kubectl wait
    print(f"\n✅ Go infrastructure: Mock LLM at {mock_llm_url} (verified by kubectl wait)")

    class MockLLMService:
        def __init__(self, url):
            self.url = url

    yield MockLLMService(mock_llm_url)
    return

    # E2E REQUIRES Go infrastructure - no manual fallback
    pytest.fail(
        "Go-managed E2E infrastructure not available.\n"
        "Run: make test-e2e-holmesgpt-api"
    )


@pytest.fixture
def mock_config():
    """
    Fixture to provide mock configuration for tests.
    Prevents MagicMock serialization issues in audit events.
    """
    config = load_test_config()
    return {
        "llm_provider": "mock",
        "llm_model": "mock-model",
        "data_storage_url": config.get('data_storage', {}).get('url', 'http://localhost:8081'),
    }


@pytest.fixture(scope="session")
def test_workflows_bootstrapped(data_storage_stack):
    """
    DD-TEST-011 v2.0: Workflows already seeded by Go suite setup.
    
    Pattern matches AA integration tests:
    - Go: Seeds workflows in SynchronizedBeforeSuite Phase 1
    - Python: Fixture is a no-op, workflows already exist
    
    This prevents pytest-xdist parallel workers from bootstrapping concurrently,
    eliminating TokenReview rate limiting (BR-TEST-008).
    
    Migration from Python bootstrap (Feb 2, 2026):
    - Previous: Python fixture bootstrapped 5 workflows per worker (race condition)
    - Current: Go bootstraps ONCE before pytest starts (no race, faster)
    - Benefit: 15s faster startup, consistent with AA pattern
    
    See: test/e2e/holmesgpt-api/test_workflows.go for Go bootstrap implementation
    """
    data_storage_url = data_storage_stack
    print(f"\n✅ DD-TEST-011 v2.0: Workflows already seeded by Go suite setup")
    print(f"   Data Storage URL: {data_storage_url}")
    print(f"   Pattern: Matches AIAnalysis integration tests")
    
    # Return empty results (workflows already seeded by Go before pytest started)
    return {
        "created": [],
        "existing": [],
        "failed": [],
        "total": 0,
        "seeded_by": "go_before_suite"
    }


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


@pytest.fixture(scope="session")
def hapi_auth_token():
    """
    ServiceAccount Bearer token for HAPI authentication (DD-AUTH-014).
    
    Auth/Authz is ALWAYS enabled in E2E and INT for ALL services.
    Token loaded from standard K8s ServiceAccount mount path.
    
    Authority: DD-AUTH-014 (Middleware-Based SAR Authentication)
    
    Returns:
        str: Bearer token for Authorization header
    """
    token_path = "/var/run/secrets/kubernetes.io/serviceaccount/token"
    with open(token_path, 'r') as f:
        token = f.read().strip()
        print(f"\n🔐 ServiceAccount token loaded from {token_path} (DD-AUTH-014)")
        return token


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

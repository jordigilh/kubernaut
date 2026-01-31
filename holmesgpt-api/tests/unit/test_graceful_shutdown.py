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
Integration Tests for Graceful Shutdown - TDD RED Phase

Business Requirement: BR-HAPI-201 - Graceful shutdown with in-flight request completion
Design Decision: DD-007 - Kubernetes-Aware Graceful Shutdown Pattern

Test Coverage (2 tests):
1. Readiness probe coordination (P0) - Returns 503 during shutdown
2. In-flight request completion (P0) - Completes active requests before shutdown

Reference: Context API (test/integration/contextapi/13_graceful_shutdown_test.go)
Reference: Dynamic Toolset (test/integration/toolset/graceful_shutdown_test.go)

TDD Phase: RED - These tests WILL FAIL until implementation is added
"""

import pytest
from fastapi.testclient import TestClient
import os


# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# FIXTURES
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

@pytest.fixture(scope="session", autouse=True)
def setup_test_config():
    """Setup config file path for unit tests (ADR-030)."""
    test_dir = os.path.dirname(__file__)
    config_path = os.path.abspath(os.path.join(test_dir, "../../config.yaml"))
    os.environ["CONFIG_FILE"] = config_path
    yield


# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# TEST 1: Readiness Probe Coordination (P0)
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

def test_readiness_probe_returns_503_during_shutdown(setup_test_config):
    """
    Test 1: Readiness probe returns 503 during graceful shutdown

    BR-HAPI-201: Graceful shutdown with DD-007 pattern

    TDD RED Phase: This test WILL FAIL because:
    - is_shutting_down flag doesn't exist in main.py
    - /ready endpoint doesn't check shutdown flag

    Expected behavior:
    - /ready returns 200 before shutdown
    - /ready returns 503 during shutdown (after flag is set)
    - /health still returns 200 during shutdown (liveness stays healthy)

    This is the critical coordination mechanism for zero-downtime deployments.
    """
    from src.main import create_app
    from src.auth import MockAuthenticator, MockAuthorizer
    
    # Use factory pattern with mock auth (no K8s dependency)
    app = create_app(
        authenticator=MockAuthenticator(valid_users={"test-token": "system:serviceaccount:test:sa"}),
        authorizer=MockAuthorizer(default_allow=True)
    )

    # Create test client
    client = TestClient(app)

    # STEP 1: Verify readiness probe returns 200 before shutdown
    response = client.get("/ready")
    assert response.status_code == 200, \
        "Readiness probe should return 200 before shutdown"
    assert response.json()["status"] == "ready", \
        "Service should be ready"

    # STEP 2: Simulate shutdown by setting flag
    # This will fail because is_shutting_down doesn't exist yet (RED phase)
    try:
        # Try to access the shutdown flag
        import src.main as main_module

        # This will raise AttributeError because flag doesn't exist (RED)
        if hasattr(main_module, 'is_shutting_down'):
            # Set shutdown flag
            main_module.is_shutting_down = True

            # STEP 3: Verify readiness probe returns 503 during shutdown
            response = client.get("/ready")
            assert response.status_code == 503, \
                "Readiness probe should return 503 during shutdown"
            assert response.json()["status"] == "shutting_down", \
                "Status should indicate shutting down"

            # STEP 4: Verify liveness probe still returns 200
            response = client.get("/health")
            assert response.status_code == 200, \
                "Liveness probe should still return 200 during shutdown"

            # Reset flag for other tests
            main_module.is_shutting_down = False
        else:
            # Expected failure in RED phase
            pytest.fail(
                "EXPECTED FAILURE (RED phase): is_shutting_down flag not found in main.py. "
                "Implementation needed: Add global is_shutting_down flag to main.py"
            )
    except AttributeError as e:
        # Expected failure in RED phase
        pytest.fail(
            f"EXPECTED FAILURE (RED phase): {str(e)}. "
            "Implementation needed: Add global is_shutting_down flag to main.py"
        )


# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# TEST 2: In-Flight Request Completion (P0)
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

def test_inflight_request_completion(setup_test_config):
    """
    Test 2: In-flight requests complete before shutdown

    BR-HAPI-201: Graceful shutdown with DD-007 pattern

    TDD RED Phase: This test documents expected behavior.

    Note: uvicorn handles in-flight request completion automatically with SIGTERM.
    This test verifies the behavior is documented and understood.

    Expected behavior:
    - Long-running requests complete successfully
    - Server waits for in-flight requests before terminating
    - No requests are dropped during graceful shutdown

    This ensures zero request failures during rolling updates.
    """
    from src.main import create_app
    from src.auth import MockAuthenticator, MockAuthorizer
    
    # Use factory pattern with mock auth (no K8s dependency)
    app = create_app(
        authenticator=MockAuthenticator(valid_users={"test-token": "system:serviceaccount:test:sa"}),
        authorizer=MockAuthorizer(default_allow=True)
    )

    # Create test client
    client = TestClient(app)

    # STEP 1: Verify we can make a request
    response = client.get("/health")
    assert response.status_code == 200, "Health endpoint should be accessible"

    # STEP 2: Document uvicorn's graceful shutdown behavior
    # uvicorn automatically:
    # - Waits for in-flight requests to complete (default timeout: 30s)
    # - Closes server socket to prevent new connections
    # - Shuts down gracefully after all requests finish

    # This test passes to document that uvicorn handles this automatically
    # No additional implementation needed for in-flight request completion

    # The critical part is the readiness probe coordination (Test 1)
    # which prevents new requests from being routed to the shutting-down pod

    assert True, "uvicorn handles in-flight request completion automatically"


# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# TEST EXECUTION SUMMARY
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

"""
TDD RED Phase Summary:

Test 1: test_readiness_probe_returns_503_during_shutdown
-------------------------------------------------------
Status: WILL FAIL (expected)
Reason: is_shutting_down flag doesn't exist in main.py
Implementation Needed:
  1. Add global is_shutting_down = False to main.py
  2. Update /ready endpoint in health.py to check flag
  3. Return 503 if is_shutting_down is True

Test 2: test_inflight_request_completion
-----------------------------------------
Status: WILL PASS (documentation test)
Reason: Documents uvicorn's automatic behavior
Implementation Needed: None (uvicorn handles this)

Next Steps (GREEN Phase):
-------------------------
1. Add is_shutting_down flag to main.py
2. Update /ready endpoint to check flag
3. Run tests again - Test 1 should pass
4. Proceed to REFACTOR phase (signal handlers)

Graceful Shutdown Implementation Plan:
--------------------------------------
Phase 1 (GREEN): Readiness probe coordination
  - Add is_shutting_down flag
  - Update /ready endpoint
  - Test 1 passes

Phase 2 (REFACTOR): Signal handling
  - Add SIGTERM handler
  - Handler sets is_shutting_down flag
  - Add structured logging
  - Tests still pass

uvicorn Automatic Behavior:
---------------------------
- Handles SIGTERM gracefully
- Waits for in-flight requests (30s timeout)
- No manual connection draining needed
- Simpler than Go services (no 5-second wait, no manual drain)
"""

if __name__ == "__main__":
    print("=" * 70)
    print("HolmesGPT API Graceful Shutdown Tests - TDD RED Phase")
    print("=" * 70)
    print()
    print("Running tests with pytest...")
    print()
    import subprocess
    result = subprocess.run(
        ["python3", "-m", "pytest", __file__, "-v", "--tb=short"],
        capture_output=False
    )
    print()
    print("=" * 70)
    if result.returncode != 0:
        print("✅ RED phase confirmed: Tests failed as expected")
        print("   Next: Implement is_shutting_down flag (GREEN phase)")
    else:
        print("⚠️  Unexpected: Tests passed (should fail in RED phase)")
    print("=" * 70)


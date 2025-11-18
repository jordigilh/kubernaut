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
Pytest Configuration and Shared Fixtures

Provides reusable test fixtures for HolmesGPT API Service testing.
"""

import pytest
from fastapi.testclient import TestClient
from typing import Dict, Any


@pytest.fixture
def test_config() -> Dict[str, Any]:
    """
    Test configuration
    """
    return {
        "service_name": "holmesgpt-api",
        "version": "1.0.0",
        "environment": "test",
        "dev_mode": True,
        "auth_enabled": False,
        "llm": {
            "provider": "mock",
            "model": "test-model",
            "endpoint": "http://localhost:11434",
        },
    }


@pytest.fixture
def client(test_config):
    """
    FastAPI test client
    """
    import os
    os.environ["DEV_MODE"] = "true"
    os.environ["AUTH_ENABLED"] = "false"

    from src.main import app
    return TestClient(app)


@pytest.fixture
def auth_client(test_config):
    """
    FastAPI test client with authentication enabled
    """
    import os
    os.environ["DEV_MODE"] = "true"
    os.environ["AUTH_ENABLED"] = "true"

    from src.main import app
    return TestClient(app)


@pytest.fixture
def valid_jwt_token() -> str:
    """
    Valid JWT token for authentication tests
    BR-HAPI-067: JWT token authentication

    GREEN phase: Simple token format for testing
    REFACTOR phase: Will use real JWT or Kubernetes ServiceAccount tokens
    """
    # GREEN phase stub: Format "test-token-username-role"
    # REFACTOR: Replace with real JWT or K8s token generation
    return "test-token-testuser-operator"


@pytest.fixture
def expired_jwt_token() -> str:
    """
    Expired JWT token for negative authentication tests
    BR-HAPI-067: JWT token expiration validation

    GREEN phase: Returns invalid token format (gets rejected)
    REFACTOR phase: Will return properly expired JWT/K8s token
    """
    # GREEN phase stub: Return token that doesn't match expected format
    # This will be rejected as invalid
    # REFACTOR: Replace with real expired JWT or K8s token
    return "expired-old-token"


@pytest.fixture
def sample_recovery_request() -> Dict[str, Any]:
    """
    Sample recovery request for testing
    """
    return {
        "incident_id": "test-inc-001",
        "failed_action": {
            "type": "scale_deployment",
            "target": "nginx",
            "desired_replicas": 5
        },
        "failure_context": {
            "error": "insufficient_resources",
            "cluster_state": "normal"
        },
        "investigation_result": {
            "root_cause": "resource_exhaustion"
        },
        "context": {
            "namespace": "test",
            "cluster": "test-cluster"
        },
        "constraints": {
            "max_attempts": 3,
            "timeout": "5m"
        }
    }


@pytest.fixture
def sample_postexec_request() -> Dict[str, Any]:
    """
    Sample post-execution request for testing
    """
    return {
        "execution_id": "test-exec-001",
        "action_id": "test-action-001",
        "action_type": "scale_deployment",
        "action_details": {
            "deployment": "nginx",
            "replicas": 3
        },
        "execution_success": True,
        "execution_result": {
            "status": "scaled",
            "duration_ms": 2500
        },
        "pre_execution_state": {
            "replicas": 1,
            "cpu_usage": 0.95
        },
        "post_execution_state": {
            "replicas": 3,
            "cpu_usage": 0.35
        },
        "context": {
            "namespace": "test",
            "cluster": "test-cluster"
        }
    }

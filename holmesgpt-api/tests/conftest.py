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
Uses mock LLM server for integration tests - no DEV_MODE anti-pattern.
"""

import pytest
import os
import sys
from pathlib import Path
from fastapi.testclient import TestClient
from typing import Dict, Any

# V3.0 (Mock LLM Migration - January 12, 2026):
# Removed embedded MockLLMServer - now using standalone Mock LLM service
# from tests.mock_llm_server import MockLLMServer


# ========================================
# Pytest Hook: Configure PYTHONPATH Early
# ========================================

def pytest_configure(config):
    """
    Pytest hook that runs BEFORE test collection.

    Add datastorage client to PYTHONPATH so OpenAPI-generated types are available
    when test modules import src.models.audit_models.
    """
    project_root = Path(__file__).parent.parent
    datastorage_client_path = project_root / "src" / "clients" / "datastorage"
    if str(datastorage_client_path) not in sys.path:
        sys.path.insert(0, str(datastorage_client_path))


# ========================================
# Session-level Mock Mode Setup (BR-HAPI-212)
# ========================================

@pytest.fixture(scope="session", autouse=True)
def setup_mock_llm_mode():
    """
    Set MOCK_LLM_MODE=true for all tests before any module imports.

    BR-HAPI-212: Enable deterministic mock responses for fast unit testing.
    This must be session-scoped and autouse to ensure it's set before
    any test modules import the FastAPI app.
    """
    os.environ["MOCK_LLM_MODE"] = "true"
    yield
    # Cleanup
    os.environ.pop("MOCK_LLM_MODE", None)


@pytest.fixture
def test_config() -> Dict[str, Any]:
    """
    Test configuration for unit tests.

    V3.0 (Mock LLM Migration - January 12, 2026):
    - Removed dependency on embedded MockLLMServer
    - Uses environment variables or config file for LLM endpoint
    - Unit tests use TestClient with mocked config (see unit/conftest.py)
    - Integration tests use real Mock LLM container (see integration/conftest.py)
    """
    return {
        "service_name": "holmesgpt-api",
        "version": "1.0.0",
        "environment": "test",
        "auth_enabled": False,
        "llm": {
            "provider": "openai",
            "model": "mock-model",
            "endpoint": os.environ.get("LLM_ENDPOINT", "http://127.0.0.1:8080"),
        },
    }


@pytest.fixture
def client():
    """
    FastAPI test client for unit tests.

    BR-HAPI-212: MOCK_LLM_MODE is set by setup_mock_llm_mode session fixture.
    """
    os.environ["AUTH_ENABLED"] = "false"
    os.environ.pop("DEV_MODE", None)

    from src.main import app
    return TestClient(app)


@pytest.fixture
def auth_client():
    """
    FastAPI test client with authentication enabled.

    BR-HAPI-212: MOCK_LLM_MODE is set by setup_mock_llm_mode session fixture.
    """
    os.environ["AUTH_ENABLED"] = "true"
    os.environ.pop("DEV_MODE", None)

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
    Sample recovery request for testing (new DD-RECOVERY-003 format)

    Updated: DD-WORKFLOW-002 v2.2 - remediation_id is now mandatory
    Updated: DD-RECOVERY-003 - Uses PreviousExecution format (no legacy fields)
    """
    return {
        "incident_id": "test-inc-001",
        "remediation_id": "req-test-2025-11-27-001",  # DD-WORKFLOW-002 v2.2: mandatory
        "is_recovery_attempt": True,
        "recovery_attempt_number": 1,
        "previous_execution": {
            "workflow_execution_ref": "req-test-2025-11-27-001-we-1",
            "original_rca": {
                "summary": "Resource exhaustion causing scaling issue",
                "signal_type": "ResourcePressure",
                "severity": "medium",
                "contributing_factors": ["insufficient_resources"]
            },
            "selected_workflow": {
                "workflow_id": "scale-horizontal-v1",
                "version": "1.0.0",
                "container_image": "kubernaut/workflow-scale:v1.0.0",
                "parameters": {"TARGET_REPLICAS": "5"},
                "rationale": "Scaling out to handle resource pressure"
            },
            "failure": {
                "failed_step_index": 1,
                "failed_step_name": "scale_deployment",
                "reason": "InsufficientResources",
                "message": "Not enough resources to scale",
                "failed_at": "2025-11-27T10:30:00Z",
                "execution_time": "30s"
            }
        },
        "enrichment_results": {
            "detectedLabels": {
                "gitOpsManaged": False,
                "pdbProtected": False
            }
        },
        "signal_type": "ResourcePressure",
        "severity": "medium",
        "resource_namespace": "test",
        "resource_kind": "Deployment",
        "resource_name": "nginx"
    }


@pytest.fixture
def sample_recovery_request_with_previous_execution() -> Dict[str, Any]:
    """
    Sample recovery request with PreviousExecution context for testing

    Design Decision: DD-RECOVERY-002, DD-RECOVERY-003
    Business Outcome: Test recovery flow with complete failure context
    """
    return {
        "incident_id": "test-inc-002",
        "remediation_id": "req-test-2025-11-29-002",
        "is_recovery_attempt": True,
        "recovery_attempt_number": 2,
        "previous_execution": {
            "workflow_execution_ref": "req-test-2025-11-29-001-we-1",
            "original_rca": {
                "summary": "Memory exhaustion causing OOMKilled in test pod",
                "signal_type": "OOMKilled",
                "severity": "high",
                "contributing_factors": ["memory leak", "insufficient limits"]
            },
            "selected_workflow": {
                "workflow_id": "scale-horizontal-v1",
                "version": "1.0.0",
                "container_image": "kubernaut/workflow-scale:v1.0.0",
                "parameters": {"TARGET_REPLICAS": "5"},
                "rationale": "Scaling out to distribute memory load"
            },
            "failure": {
                "failed_step_index": 2,
                "failed_step_name": "scale_deployment",
                "reason": "OOMKilled",
                "message": "Container exceeded memory limit during scale operation",
                "exit_code": 137,
                "failed_at": "2025-11-29T10:30:00Z",
                "execution_time": "2m34s"
            }
        },
        "enrichment_results": {
            "detectedLabels": {
                "gitOpsManaged": True,
                "gitOpsTool": "argocd",
                "pdbProtected": True,
                "stateful": False
            }
        },
        "signal_type": "OOMKilled",
        "severity": "high",
        "resource_namespace": "test",
        "resource_kind": "Deployment",
        "resource_name": "test-api",
        "environment": "test",
        "priority": "P1",
        "risk_tolerance": "medium",
        "business_category": "critical",
        "cluster_name": "test-cluster"
    }


@pytest.fixture
def sample_incident_request_with_detected_labels() -> Dict[str, Any]:
    """
    Sample incident request with DetectedLabels for testing

    Design Decision: DD-RECOVERY-003
    Business Outcome: Test incident flow with cluster context
    """
    return {
        "incident_id": "test-inc-003",
        "remediation_id": "req-test-2025-11-29-003",
        "signal_type": "OOMKilled",
        "severity": "high",
        "signal_source": "prometheus",
        "resource_namespace": "production",
        "resource_kind": "Deployment",
        "resource_name": "api-server",
        "error_message": "Container exceeded memory limit",
        "environment": "production",
        "priority": "P1",
        "risk_tolerance": "medium",
        "business_category": "critical",
        "cluster_name": "prod-us-west-2",
        "enrichment_results": {
            "detectedLabels": {
                "gitOpsManaged": True,
                "gitOpsTool": "argocd",
                "pdbProtected": True,
                "hpaEnabled": False,
                "stateful": False,
                "helmManaged": True,
                "networkIsolated": True,
                # DD-WORKFLOW-001 v2.2: podSecurityLevel REMOVED
                "serviceMesh": "istio"
            },
            "enrichmentQuality": 0.95
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

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
Smoke Tests with Real LLM (gpt-oss-20b)

These tests validate that our prompts correctly guide the LLM to make
appropriate tool calls. Unlike E2E tests with mock LLM, these tests:

1. Use a REAL LLM (gpt-oss-20b running locally)
2. Validate prompt engineering effectiveness
3. Confirm tool calling works with actual model inference

Prerequisites:
- gpt-oss-20b model running on localhost:8443
- Sufficient VRAM (12-16GB with Q4 quantization)

Run manually:
    pytest tests/smoke/ -v -m smoke

Run with integration infrastructure:
    1. Start Data Storage: ./tests/integration/setup_workflow_catalog_integration.sh
    2. Start LLM: ollama serve (or your preferred method)
    3. Run tests: pytest tests/smoke/ -v -m smoke
"""

import os
import sys
import json
import time
import pytest
import requests
from typing import Dict, Any, Optional
from unittest.mock import patch, MagicMock

# Add src to path
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', '..', 'src'))

# LLM Configuration
REAL_LLM_ENDPOINT = os.getenv("REAL_LLM_ENDPOINT", "http://localhost:8443")
REAL_LLM_MODEL = os.getenv("REAL_LLM_MODEL", "bartowski/Qwen2.5-32B-Instruct-GGUF/Qwen2.5-32B-Instruct-Q4_K_M.gguf")


def is_llm_available() -> bool:
    """Check if the real LLM is available."""
    try:
        # Try Ollama-style health check
        response = requests.get(f"{REAL_LLM_ENDPOINT}/api/tags", timeout=5)
        if response.status_code == 200:
            return True
    except:
        pass

    try:
        # Try OpenAI-style health check
        response = requests.get(f"{REAL_LLM_ENDPOINT}/v1/models", timeout=5)
        if response.status_code == 200:
            return True
    except:
        pass

    try:
        # Try simple root health check
        response = requests.get(f"{REAL_LLM_ENDPOINT}/", timeout=5)
        if response.status_code in [200, 404]:  # 404 means server is up but no root handler
            return True
    except:
        pass

    return False


# Skip all tests if LLM not available
pytestmark = [
    pytest.mark.smoke,
    pytest.mark.skipif(
        not is_llm_available(),
        reason=f"Real LLM not available at {REAL_LLM_ENDPOINT}"
    )
]


# ========================================
# FIXTURES
# ========================================

@pytest.fixture(scope="module")
def real_llm_client():
    """Configure FastAPI test client with real LLM."""
    # Set environment for real LLM
    # The local model exposes an OpenAI-compatible API at localhost:8443
    os.environ["LLM_ENDPOINT"] = REAL_LLM_ENDPOINT
    os.environ["LLM_MODEL"] = REAL_LLM_MODEL
    os.environ["LLM_PROVIDER"] = "openai"  # OpenAI-compatible endpoint
    os.environ["OPENAI_API_KEY"] = "dummy-key-for-local-model"  # Required by HolmesGPT SDK
    os.environ["DATA_STORAGE_URL"] = "http://localhost:18094"  # DD-TEST-001 port

    # Import after setting environment
    from fastapi.testclient import TestClient
    from main import app

    with TestClient(app) as client:
        yield client


@pytest.fixture
def sample_oomkilled_incident() -> Dict[str, Any]:
    """Sample OOMKilled incident for smoke testing."""
    return {
        "incident_id": "smoke-incident-001",
        "remediation_id": "smoke-rem-001",
        "signal_type": "OOMKilled",
        "severity": "critical",
        "signal_source": "prometheus",
        "resource_namespace": "production",
        "resource_kind": "Pod",
        "resource_name": "api-server-abc123",
        "error_message": "Container killed due to OOM: memory limit exceeded",
        "environment": "production",
        "priority": "P1",
        "risk_tolerance": "low",
        "business_category": "payments",
        "cluster_name": "prod-cluster-1",
        "enrichment_results": {
            "kubernetesContext": {
                "namespace": "production",
                "podName": "api-server-abc123",
                "containerName": "api-server",
                "memoryLimit": "512Mi",
                "memoryUsage": "510Mi"
            },
            "detectedLabels": {
                "gitOpsManaged": True,
                "gitOpsTool": "argocd",
                "pdbProtected": True,
                "stateful": False
            },
            "customLabels": {
                "constraint": ["cost-constrained"],
                "team": ["name=payments"]
            }
        }
    }


@pytest.fixture
def sample_crashloop_incident() -> Dict[str, Any]:
    """Sample CrashLoopBackOff incident for smoke testing."""
    return {
        "incident_id": "smoke-incident-002",
        "remediation_id": "smoke-rem-002",
        "signal_type": "CrashLoopBackOff",
        "severity": "high",
        "signal_source": "prometheus",
        "resource_namespace": "staging",
        "resource_kind": "Pod",
        "resource_name": "worker-xyz789",
        "error_message": "Back-off restarting failed container",
        "environment": "staging",
        "priority": "P2",
        "risk_tolerance": "medium",
        "business_category": "backend",
        "cluster_name": "staging-cluster-1",
        "enrichment_results": {
            "kubernetesContext": {
                "namespace": "staging",
                "podName": "worker-xyz789"
            },
            "detectedLabels": {
                "gitOpsManaged": False,
                "helmManaged": True
            }
        }
    }


def mock_data_storage_response(signal_type: str = "OOMKilled") -> Dict[str, Any]:
    """Generate mock Data Storage response for smoke tests."""
    workflows = {
        "OOMKilled": {
            "workflow_id": "oomkill-increase-memory-v1",
            "title": "OOMKill Recovery - Increase Memory Limits",
            "description": "Increases memory limits for OOMKilled pods",
            "signal_type": "OOMKilled",
            "confidence": 0.95
        },
        "CrashLoopBackOff": {
            "workflow_id": "crashloop-config-fix-v1",
            "title": "CrashLoopBackOff - Configuration Fix",
            "description": "Fixes configuration issues causing crash loops",
            "signal_type": "CrashLoopBackOff",
            "confidence": 0.88
        }
    }

    workflow = workflows.get(signal_type, workflows["OOMKilled"])

    return {
        "workflows": [workflow],
        "totalResults": 1,
        "query": f"{signal_type} critical"
    }


# ========================================
# SMOKE TESTS
# ========================================

class TestRealLLMToolCalling:
    """Smoke tests for real LLM tool calling."""

    @pytest.mark.smoke
    @pytest.mark.timeout(120)  # 2 min timeout for LLM inference
    def test_llm_generates_tool_call_for_oomkilled(
        self,
        real_llm_client,
        sample_oomkilled_incident
    ):
        """
        SMOKE TEST: Verify real LLM generates search_workflow_catalog tool call.

        This validates that our prompt engineering correctly guides the LLM
        to use the workflow catalog tool for OOMKilled incidents.
        """
        print("\n🔥 SMOKE TEST: OOMKilled → Tool Call")
        print(f"   LLM: {REAL_LLM_ENDPOINT} / {REAL_LLM_MODEL}")

        start_time = time.time()

        with patch('requests.post') as mock_post:
            # Mock Data Storage response
            mock_response = MagicMock()
            mock_response.status_code = 200
            mock_response.json.return_value = mock_data_storage_response("OOMKilled")
            mock_post.return_value = mock_response

            response = real_llm_client.post(
                "/api/v1/incident/analyze",
                json=sample_oomkilled_incident
            )

        elapsed = time.time() - start_time
        print(f"   Response time: {elapsed:.2f}s")

        # Validate response
        assert response.status_code == 200, f"Expected 200, got {response.status_code}: {response.text}"

        data = response.json()
        print(f"   Response keys: {list(data.keys())}")

        # Check that we got a valid analysis
        assert "incident_id" in data
        assert data["incident_id"] == sample_oomkilled_incident["incident_id"]

        # Check that Data Storage was called (tool was used)
        assert mock_post.called, "Data Storage should have been called via tool"

        # Validate the request to Data Storage
        call_args = mock_post.call_args
        if call_args and call_args.kwargs.get('json'):
            ds_request = call_args.kwargs['json']
            print(f"   Data Storage request: {json.dumps(ds_request, indent=2)[:200]}...")

            # Validate query format
            if 'query' in ds_request:
                query = ds_request['query']
                assert "OOMKilled" in query or "oom" in query.lower(), \
                    f"Query should contain OOMKilled, got: {query}"

        print("   ✅ PASSED: LLM correctly generated tool call")

    @pytest.mark.smoke
    @pytest.mark.timeout(120)
    def test_llm_generates_tool_call_for_crashloop(
        self,
        real_llm_client,
        sample_crashloop_incident
    ):
        """
        SMOKE TEST: Verify real LLM generates tool call for CrashLoopBackOff.
        """
        print("\n🔥 SMOKE TEST: CrashLoopBackOff → Tool Call")

        start_time = time.time()

        with patch('requests.post') as mock_post:
            mock_response = MagicMock()
            mock_response.status_code = 200
            mock_response.json.return_value = mock_data_storage_response("CrashLoopBackOff")
            mock_post.return_value = mock_response

            response = real_llm_client.post(
                "/api/v1/incident/analyze",
                json=sample_crashloop_incident
            )

        elapsed = time.time() - start_time
        print(f"   Response time: {elapsed:.2f}s")

        assert response.status_code == 200
        assert mock_post.called, "Data Storage should have been called"

        print("   ✅ PASSED: LLM correctly generated tool call")

    @pytest.mark.smoke
    @pytest.mark.timeout(180)  # 3 min for recovery (more complex prompt)
    def test_llm_handles_recovery_scenario(
        self,
        real_llm_client
    ):
        """
        SMOKE TEST: Verify LLM handles recovery with previous execution context.
        """
        print("\n🔥 SMOKE TEST: Recovery Scenario")

        recovery_request = {
            "incident_id": "smoke-recovery-001",
            "remediation_id": "smoke-rem-003",
            "signal_type": "OOMKilled",
            "severity": "critical",
            "signal_source": "prometheus",
            "resource_namespace": "production",
            "resource_kind": "Pod",
            "resource_name": "api-server-abc123",
            "error_message": "OOM persists after first remediation",
            "environment": "production",
            "priority": "P1",
            "risk_tolerance": "low",
            "business_category": "payments",
            "cluster_name": "prod-cluster-1",
            "is_recovery_attempt": True,
            "recovery_attempt_number": 1,
            "previous_execution": {
                "workflow_execution_ref": "smoke-rem-002-we-1",
                "original_rca": {
                    "summary": "Memory exhaustion detected",
                    "signal_type": "OOMKilled",
                    "severity": "critical",
                    "contributing_factors": ["memory_leak", "traffic_spike"]
                },
                "selected_workflow": {
                    "workflow_id": "scale-horizontal-v1",
                    "title": "Horizontal Scaling",
                    "version": "1.0.0",
                    "container_image": "ghcr.io/kubernaut/scale:v1.0.0",
                    "parameters": {"TARGET_REPLICAS": "5"},
                    "rationale": "Scale out to distribute load"
                },
                "failure": {
                    "failed_step_index": 1,
                    "failed_step_name": "scale-deployment",
                    "reason": "InsufficientResources",
                    "message": "Cluster lacks resources for scaling",
                    "exit_code": 1,
                    "failed_at": "2025-11-30T12:00:00Z",
                    "execution_time": "45s"
                }
            },
            "enrichment_results": {
                "detectedLabels": {
                    "gitOpsManaged": True,
                    "gitOpsTool": "argocd"
                }
            }
        }

        start_time = time.time()

        with patch('requests.post') as mock_post:
            mock_response = MagicMock()
            mock_response.status_code = 200
            mock_response.json.return_value = {
                "workflows": [{
                    "workflowId": "memory-optimize-v1",
                    "title": "Memory Optimization",
                    "signalType": "OOMKilled",
                    "confidence": 0.85
                }],
                "totalResults": 1
            }
            mock_post.return_value = mock_response

            response = real_llm_client.post(
                "/api/v1/recovery/analyze",
                json=recovery_request
            )

        elapsed = time.time() - start_time
        print(f"   Response time: {elapsed:.2f}s")

        assert response.status_code == 200, f"Got {response.status_code}: {response.text}"

        data = response.json()
        assert "incident_id" in data

        print("   ✅ PASSED: LLM handled recovery scenario")


class TestRealLLMResponseQuality:
    """Smoke tests for LLM response quality."""

    @pytest.mark.smoke
    @pytest.mark.timeout(120)
    def test_llm_response_contains_reasoning(
        self,
        real_llm_client,
        sample_oomkilled_incident
    ):
        """
        SMOKE TEST: Verify LLM provides meaningful analysis, not just tool calls.
        """
        print("\n🔥 SMOKE TEST: Response Quality Check")

        with patch('requests.post') as mock_post:
            mock_response = MagicMock()
            mock_response.status_code = 200
            mock_response.json.return_value = mock_data_storage_response("OOMKilled")
            mock_post.return_value = mock_response

            response = real_llm_client.post(
                "/api/v1/incident/analyze",
                json=sample_oomkilled_incident
            )

        assert response.status_code == 200
        data = response.json()

        # Check for meaningful content
        if "analysis" in data:
            analysis = data["analysis"]
            print(f"   Analysis length: {len(analysis)} chars")

            # Should have some substance
            assert len(analysis) > 50, "Analysis should be substantive"

            # Should mention relevant concepts
            analysis_lower = analysis.lower()
            relevant_terms = ["memory", "oom", "pod", "container", "limit", "resource"]
            found_terms = [t for t in relevant_terms if t in analysis_lower]

            print(f"   Relevant terms found: {found_terms}")
            assert len(found_terms) >= 2, f"Analysis should mention relevant terms, found: {found_terms}"

        print("   ✅ PASSED: Response contains meaningful analysis")


# ========================================
# STANDALONE RUNNER
# ========================================

if __name__ == "__main__":
    print("=" * 60)
    print("SMOKE TESTS - Real LLM Validation")
    print("=" * 60)
    print(f"LLM Endpoint: {REAL_LLM_ENDPOINT}")
    print(f"LLM Model: {REAL_LLM_MODEL}")
    print()

    if is_llm_available():
        print("✅ LLM is available")
        pytest.main([__file__, "-v", "-m", "smoke", "--tb=short"])
    else:
        print(f"❌ LLM not available at {REAL_LLM_ENDPOINT}")
        print("   Start the LLM server and try again.")
        sys.exit(1)


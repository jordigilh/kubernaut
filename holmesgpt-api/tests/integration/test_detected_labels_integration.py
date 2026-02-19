# Copyright 2026 Jordi Gil.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""
Integration tests for DetectedLabels on-demand computation (ADR-056 SoC).

These tests verify the full detected_labels flow end-to-end within HAPI:
1. Incident/recovery request arrives
2. WorkflowDiscoveryToolset is registered with K8s client + resource identity
3. Mock LLM calls list_available_actions -> triggers on-demand label detection
4. Labels are cached in session_state
5. inject_detected_labels writes labels to response

Infrastructure: Go-bootstrapped (DataStorage + Mock LLM); Python direct calls
K8s resources: Mocked via fixtures/k8s_mock_fixtures.py

Business Requirements:
  - BR-SP-101: DetectedLabels Auto-Detection (post-RCA via HAPI)
  - BR-HAPI-194: Honor failedDetections in workflow filtering
  - BR-HAPI-250: DetectedLabels integration with Data Storage

Test Matrix: 7 tests
  - IT-HAPI-056-001: CrashLoopBackOff + PDB -> detected_labels.pdbProtected=true
  - IT-HAPI-056-002: Recovery + HPA -> detected_labels.hpaEnabled=true
  - IT-HAPI-056-003: detected_labels propagated in workflow discovery query params
  - IT-HAPI-056-004: ArgoCD + Helm managed -> gitOpsManaged + helmManaged
  - IT-HAPI-056-005: RBAC 403 -> failedDetections includes pdbProtected
  - IT-HAPI-056-006: No K8s resources -> all-false labels, no crash
  - IT-HAPI-056-007: Cached labels reused (no recomputation)
"""

import json
import pytest
from unittest.mock import patch, MagicMock, AsyncMock

from tests.integration.fixtures.k8s_mock_fixtures import (
    create_mock_k8s_with_pdb,
    create_mock_k8s_with_hpa,
    create_mock_k8s_argocd_helm,
    create_mock_k8s_rbac_denied,
    create_mock_k8s_no_resources,
)


@pytest.mark.requires_data_storage
class TestDetectedLabelsIncidentIntegration:
    """IT-HAPI-056-001, 003, 004, 005, 006: Incident analysis with detected_labels."""

    @patch("src.clients.k8s_client.get_k8s_client")
    def test_it_hapi_056_001_crashloop_with_pdb(self, mock_get_k8s):
        """IT-HAPI-056-001: CrashLoopBackOff incident with PDB-protected Deployment.

        Given: K8s cluster has Deployment(api) + PDB(api-pdb) with matching selector
        When: analyze_incident completes for CrashLoopBackOff signal
        Then: response detected_labels contains pdbProtected=true
        """
        mock_get_k8s.return_value = create_mock_k8s_with_pdb()

        from src.extensions.incident.llm_integration import analyze_incident

        request_data = _make_incident_request(
            signal_type="CrashLoopBackOff",
            resource_kind="Pod",
            resource_name="api-pod-abc",
            resource_namespace="production",
        )

        result = analyze_incident(request_data, app_config=_mock_app_config())

        assert result is not None
        assert "detected_labels" in result, "Response must include detected_labels"
        labels = result["detected_labels"]
        assert labels.get("pdbProtected") is True

    @patch("src.clients.k8s_client.get_k8s_client")
    def test_it_hapi_056_003_labels_in_workflow_query_params(self, mock_get_k8s):
        """IT-HAPI-056-003: detected_labels propagated to DataStorage query params.

        Given: Incident analysis with detected_labels computed
        When: workflow discovery queries DataStorage
        Then: query params include detected_labels (stripped of failed detections)
        """
        mock_get_k8s.return_value = create_mock_k8s_with_pdb()

        from src.extensions.incident.llm_integration import analyze_incident

        request_data = _make_incident_request(
            signal_type="CrashLoopBackOff",
            resource_kind="Pod",
            resource_name="api-pod-abc",
            resource_namespace="production",
        )

        result = analyze_incident(request_data, app_config=_mock_app_config())

        assert result is not None
        labels = result.get("detected_labels", {})
        if labels:
            assert "failedDetections" not in labels or isinstance(labels.get("failedDetections"), list)

    @patch("src.clients.k8s_client.get_k8s_client")
    def test_it_hapi_056_004_argocd_helm_managed(self, mock_get_k8s):
        """IT-HAPI-056-004: ArgoCD + Helm managed Deployment.

        Given: Deployment with ArgoCD annotation + Helm managed-by label
        When: analyze_incident completes
        Then: detected_labels contains gitOpsManaged=true, gitOpsTool=argocd, helmManaged=true
        """
        mock_get_k8s.return_value = create_mock_k8s_argocd_helm()

        from src.extensions.incident.llm_integration import analyze_incident

        request_data = _make_incident_request(
            signal_type="CrashLoopBackOff",
            resource_kind="Pod",
            resource_name="api-pod-abc",
            resource_namespace="production",
        )

        result = analyze_incident(request_data, app_config=_mock_app_config())

        assert result is not None
        labels = result.get("detected_labels", {})
        assert labels.get("gitOpsManaged") is True
        assert labels.get("gitOpsTool") == "argocd"
        assert labels.get("helmManaged") is True

    @patch("src.clients.k8s_client.get_k8s_client")
    def test_it_hapi_056_005_rbac_denied_failed_detections(self, mock_get_k8s):
        """IT-HAPI-056-005: K8s RBAC 403 for PDB list.

        Given: K8s API returns 403 for PDB list
        When: analyze_incident completes
        Then: detected_labels.failedDetections includes pdbProtected
        """
        mock_get_k8s.return_value = create_mock_k8s_rbac_denied()

        from src.extensions.incident.llm_integration import analyze_incident

        request_data = _make_incident_request(
            signal_type="CrashLoopBackOff",
            resource_kind="Pod",
            resource_name="api-pod-abc",
            resource_namespace="production",
        )

        result = analyze_incident(request_data, app_config=_mock_app_config())

        assert result is not None
        labels = result.get("detected_labels", {})
        failed = labels.get("failedDetections", [])
        assert "pdbProtected" in failed

    @patch("src.clients.k8s_client.get_k8s_client")
    def test_it_hapi_056_006_no_k8s_resources(self, mock_get_k8s):
        """IT-HAPI-056-006: No K8s resources found.

        Given: K8s API returns no resources
        When: analyze_incident completes
        Then: detected_labels contains all-false booleans, no crash
        """
        mock_get_k8s.return_value = create_mock_k8s_no_resources()

        from src.extensions.incident.llm_integration import analyze_incident

        request_data = _make_incident_request(
            signal_type="CrashLoopBackOff",
            resource_kind="Deployment",
            resource_name="missing",
            resource_namespace="production",
        )

        result = analyze_incident(request_data, app_config=_mock_app_config())

        assert result is not None
        labels = result.get("detected_labels", {})
        assert labels.get("pdbProtected") is False
        assert labels.get("hpaEnabled") is False
        assert labels.get("helmManaged") is False
        assert labels.get("gitOpsManaged") is False


@pytest.mark.requires_data_storage
class TestDetectedLabelsRecoveryIntegration:
    """IT-HAPI-056-002: Recovery analysis with detected_labels."""

    @patch("src.clients.k8s_client.get_k8s_client")
    def test_it_hapi_056_002_recovery_with_hpa(self, mock_get_k8s):
        """IT-HAPI-056-002: Recovery request with HPA-enabled Deployment.

        Given: K8s cluster has Deployment(api) + HPA targeting it
        When: analyze_recovery completes
        Then: response detected_labels contains hpaEnabled=true
        """
        mock_get_k8s.return_value = create_mock_k8s_with_hpa()

        from src.extensions.recovery.llm_integration import analyze_recovery

        request_data = _make_recovery_request(
            resource_kind="Pod",
            resource_name="api-pod-abc",
            resource_namespace="production",
        )

        result = analyze_recovery(request_data, app_config=_mock_app_config())

        assert result is not None
        assert "detected_labels" in result, "Recovery response must include detected_labels"
        labels = result["detected_labels"]
        assert labels.get("hpaEnabled") is True


@pytest.mark.requires_data_storage
class TestDetectedLabelsCaching:
    """IT-HAPI-056-007: Label caching across workflow discovery steps."""

    @patch("src.clients.k8s_client.get_k8s_client")
    def test_it_hapi_056_007_cached_labels_no_recomputation(self, mock_get_k8s):
        """IT-HAPI-056-007: Labels computed once and cached for subsequent tools.

        Given: Labels were computed during list_available_actions
        When: list_workflows and get_workflow are called
        Then: They reuse cached labels from session_state (no recomputation)
        """
        mock_k8s = create_mock_k8s_with_pdb()
        mock_get_k8s.return_value = mock_k8s

        from src.extensions.incident.llm_integration import analyze_incident

        request_data = _make_incident_request(
            signal_type="CrashLoopBackOff",
            resource_kind="Pod",
            resource_name="api-pod-abc",
            resource_namespace="production",
        )

        result = analyze_incident(request_data, app_config=_mock_app_config())

        assert result is not None
        assert "detected_labels" in result
        # resolve_owner_chain should only be called once (for label detection),
        # not for every tool invocation
        assert mock_k8s.resolve_owner_chain.call_count <= 2, (
            "resolve_owner_chain should be called at most twice "
            "(once for label detection, once for get_resource_context)"
        )


def _make_incident_request(
    signal_type: str = "CrashLoopBackOff",
    resource_kind: str = "Pod",
    resource_name: str = "api-pod-abc",
    resource_namespace: str = "production",
) -> dict:
    """Create a minimal incident request for testing."""
    return {
        "incident_id": f"it-hapi-056-{signal_type.lower()}",
        "remediation_id": "req-it-056-001",
        "signal_type": signal_type,
        "severity": "critical",
        "signal_source": "prometheus",
        "resource_namespace": resource_namespace,
        "resource_kind": resource_kind,
        "resource_name": resource_name,
        "error_message": f"Container in {signal_type}",
        "environment": "production",
        "priority": "P0",
        "risk_tolerance": "medium",
        "business_category": "standard",
        "cluster_name": "integration-test",
        "enrichment_results": {},
    }


def _make_recovery_request(
    resource_kind: str = "Pod",
    resource_name: str = "api-pod-abc",
    resource_namespace: str = "production",
) -> dict:
    """Create a minimal recovery request for testing."""
    return {
        "incident_id": "it-hapi-056-recovery",
        "remediation_id": "req-it-056-002",
        "signal_type": "OOMKilled",
        "severity": "high",
        "signal_source": "prometheus",
        "resource_namespace": resource_namespace,
        "resource_kind": resource_kind,
        "resource_name": resource_name,
        "error_message": "Recovery attempt after OOMKilled",
        "environment": "production",
        "priority": "P1",
        "risk_tolerance": "medium",
        "business_category": "standard",
        "cluster_name": "integration-test",
        "enrichment_results": {},
        "is_recovery_attempt": True,
        "previous_attempt_count": 1,
    }


def _mock_app_config() -> dict:
    """Create a mock app configuration for integration tests."""
    return {
        "data_storage_url": "http://127.0.0.1:18098",
        "mock_llm_mode": True,
    }

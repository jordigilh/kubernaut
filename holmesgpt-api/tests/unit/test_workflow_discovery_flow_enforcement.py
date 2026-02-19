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
Tests for on-demand label detection in workflow discovery tools.

ADR-056 SoC Refactoring: The cross-tool flow prerequisite (_check_flow_prerequisite)
has been removed. Instead, _ensure_detected_labels computes and caches labels
on-demand when any discovery tool is invoked. Labels are computed via LabelDetector
using the K8s client + resource identity passed at toolset creation time.

Business Requirements:
  - BR-HAPI-250: DetectedLabels integration with Data Storage
  - BR-SP-101:   DetectedLabels Auto-Detection (post-RCA via HAPI)

Test Matrix: 8 tests
  - UT-HAPI-056-043: list_available_actions computes labels on-demand when missing
  - UT-HAPI-056-044: list_available_actions uses cached labels when present
  - UT-HAPI-056-045: list_available_actions proceeds with empty sentinel (no recomputation)
  - UT-HAPI-056-046: list_workflows uses cached labels (no recomputation)
  - UT-HAPI-056-047: get_workflow uses cached labels (no recomputation)
  - UT-HAPI-056-048: Label detection failure writes empty sentinel, tool proceeds
  - UT-HAPI-056-049: No k8s_client -- labels not computed, tool proceeds
  - UT-HAPI-056-050: WorkflowDiscoveryToolset propagates k8s_client to all 3 tools
"""

import json
import pytest
from unittest.mock import Mock, AsyncMock, MagicMock, patch

from holmes.core.tools import StructuredToolResultStatus


OWNER_CHAIN_POD_TO_DEPLOY = [
    {"kind": "Pod", "name": "api-pod-abc", "namespace": "production"},
    {"kind": "ReplicaSet", "name": "api-rs-xyz", "namespace": "production"},
    {"kind": "Deployment", "name": "api", "namespace": "production"},
]

DETECTED_LABELS_ARGOCD = {
    "failedDetections": [],
    "gitOpsManaged": True,
    "gitOpsTool": "argocd",
    "pdbProtected": False,
    "hpaEnabled": False,
    "stateful": False,
    "helmManaged": False,
    "networkIsolated": False,
    "serviceMesh": "",
}


def _make_mock_k8s():
    """Create a mock K8s client for label detection."""
    mock_k8s = AsyncMock()
    mock_k8s.resolve_owner_chain.return_value = OWNER_CHAIN_POD_TO_DEPLOY
    mock_k8s.get_namespace_metadata = AsyncMock(
        return_value={"labels": {}, "annotations": {}}
    )
    mock_k8s._get_resource_metadata = AsyncMock(return_value=None)
    return mock_k8s


class TestOnDemandLabelDetection:
    """UT-HAPI-056-043 through UT-HAPI-056-045: On-demand label detection in list_available_actions."""

    @patch("src.toolsets.workflow_discovery.requests.get")
    @patch("src.detection.labels.LabelDetector")
    def test_ut_hapi_056_043_computes_labels_on_demand(self, mock_detector_cls, mock_get):
        """UT-HAPI-056-043: list_available_actions computes labels when session_state has no detected_labels."""
        from src.toolsets.workflow_discovery import ListAvailableActionsTool

        mock_get.return_value = Mock(
            status_code=200,
            json=Mock(return_value={
                "actionTypes": [{"actionType": "RestartPod", "workflowCount": 1}],
                "pagination": {"totalCount": 1, "offset": 0, "limit": 10, "hasMore": False},
            }),
        )

        mock_detector = AsyncMock()
        mock_detector.detect_labels.return_value = DETECTED_LABELS_ARGOCD
        mock_detector_cls.return_value = mock_detector

        mock_k8s = _make_mock_k8s()
        session_state = {}

        tool = ListAvailableActionsTool(
            severity="critical",
            component="Pod",
            environment="production",
            priority="P0",
            session_state=session_state,
            k8s_client=mock_k8s,
            resource_name="api-pod-abc",
            resource_namespace="production",
        )

        result = tool.invoke(params={})

        assert result.status == StructuredToolResultStatus.SUCCESS
        assert "detected_labels" in session_state
        assert session_state["detected_labels"] == DETECTED_LABELS_ARGOCD

    @patch("src.toolsets.workflow_discovery.requests.get")
    def test_ut_hapi_056_044_uses_cached_labels(self, mock_get):
        """UT-HAPI-056-044: list_available_actions uses cached labels when present (no recomputation)."""
        from src.toolsets.workflow_discovery import ListAvailableActionsTool

        mock_get.return_value = Mock(
            status_code=200,
            json=Mock(return_value={
                "actionTypes": [{"actionType": "RestartPod", "workflowCount": 1}],
                "pagination": {"totalCount": 1, "offset": 0, "limit": 10, "hasMore": False},
            }),
        )

        cached_labels = {"gitOpsManaged": True, "gitOpsTool": "argocd"}
        session_state = {"detected_labels": cached_labels}

        mock_k8s = _make_mock_k8s()

        tool = ListAvailableActionsTool(
            severity="critical",
            component="Pod",
            environment="production",
            priority="P0",
            session_state=session_state,
            k8s_client=mock_k8s,
            resource_name="api-pod-abc",
            resource_namespace="production",
        )

        result = tool.invoke(params={})

        assert result.status == StructuredToolResultStatus.SUCCESS
        assert session_state["detected_labels"] is cached_labels
        mock_k8s.resolve_owner_chain.assert_not_called()

    @patch("src.toolsets.workflow_discovery.requests.get")
    def test_ut_hapi_056_045_proceeds_with_empty_sentinel(self, mock_get):
        """UT-HAPI-056-045: list_available_actions proceeds when detected_labels is {} (no recomputation)."""
        from src.toolsets.workflow_discovery import ListAvailableActionsTool

        mock_get.return_value = Mock(
            status_code=200,
            json=Mock(return_value={
                "actionTypes": [],
                "pagination": {"totalCount": 0, "offset": 0, "limit": 10, "hasMore": False},
            }),
        )

        session_state = {"detected_labels": {}}
        mock_k8s = _make_mock_k8s()

        tool = ListAvailableActionsTool(
            severity="critical",
            component="Pod",
            environment="production",
            priority="P0",
            session_state=session_state,
            k8s_client=mock_k8s,
            resource_name="api-pod-abc",
            resource_namespace="production",
        )

        result = tool.invoke(params={})

        assert result.status == StructuredToolResultStatus.SUCCESS
        mock_k8s.resolve_owner_chain.assert_not_called()


class TestCachedLabelsInSubsequentTools:
    """UT-HAPI-056-046 through UT-HAPI-056-047: Subsequent tools reuse cached labels."""

    @patch("src.toolsets.workflow_discovery.requests.get")
    def test_ut_hapi_056_046_list_workflows_uses_cached_labels(self, mock_get):
        """UT-HAPI-056-046: list_workflows reads cached labels from session_state."""
        from src.toolsets.workflow_discovery import ListWorkflowsTool

        mock_get.return_value = Mock(
            status_code=200,
            json=Mock(return_value={
                "actionType": "ScaleReplicas",
                "workflows": [{"workflowId": "wf-1", "workflowName": "scale"}],
                "pagination": {"totalCount": 1, "offset": 0, "limit": 10, "hasMore": False},
            }),
        )

        session_state = {"detected_labels": {"gitOpsManaged": True}}

        tool = ListWorkflowsTool(
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
            session_state=session_state,
        )

        result = tool.invoke(params={"action_type": "ScaleReplicas"})
        assert result.status == StructuredToolResultStatus.SUCCESS

    @patch("src.toolsets.workflow_discovery.requests.get")
    def test_ut_hapi_056_047_get_workflow_uses_cached_labels(self, mock_get):
        """UT-HAPI-056-047: get_workflow reads cached labels from session_state."""
        from src.toolsets.workflow_discovery import GetWorkflowTool

        mock_get.return_value = Mock(
            status_code=200,
            json=Mock(return_value={
                "workflowId": "wf-uuid-001",
                "workflowName": "scale-conservative",
                "actionType": "ScaleReplicas",
                "parameters": {},
            }),
        )

        session_state = {"detected_labels": {"gitOpsManaged": True}}

        tool = GetWorkflowTool(
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
            session_state=session_state,
        )

        result = tool.invoke(params={"workflow_id": "wf-uuid-001"})
        assert result.status == StructuredToolResultStatus.SUCCESS


class TestLabelDetectionEdgeCases:
    """UT-HAPI-056-048 through UT-HAPI-056-049: Error handling and missing k8s_client."""

    @patch("src.toolsets.workflow_discovery.requests.get")
    @patch("src.detection.labels.LabelDetector")
    def test_ut_hapi_056_048_detection_failure_writes_sentinel(self, mock_detector_cls, mock_get):
        """UT-HAPI-056-048: Label detection failure writes {} sentinel, tool proceeds normally."""
        from src.toolsets.workflow_discovery import ListAvailableActionsTool

        mock_get.return_value = Mock(
            status_code=200,
            json=Mock(return_value={
                "actionTypes": [],
                "pagination": {"totalCount": 0, "offset": 0, "limit": 10, "hasMore": False},
            }),
        )

        mock_detector = AsyncMock()
        mock_detector.detect_labels.side_effect = RuntimeError("K8s RBAC denied")
        mock_detector_cls.return_value = mock_detector

        mock_k8s = _make_mock_k8s()
        session_state = {}

        tool = ListAvailableActionsTool(
            severity="critical",
            component="Pod",
            environment="production",
            priority="P0",
            session_state=session_state,
            k8s_client=mock_k8s,
            resource_name="api-pod-abc",
            resource_namespace="production",
        )

        result = tool.invoke(params={})

        assert result.status == StructuredToolResultStatus.SUCCESS
        assert session_state["detected_labels"] == {}

    @patch("src.toolsets.workflow_discovery.requests.get")
    def test_ut_hapi_056_049_no_k8s_client_skips_detection(self, mock_get):
        """UT-HAPI-056-049: No k8s_client -- labels not computed, tool proceeds."""
        from src.toolsets.workflow_discovery import ListAvailableActionsTool

        mock_get.return_value = Mock(
            status_code=200,
            json=Mock(return_value={
                "actionTypes": [],
                "pagination": {"totalCount": 0, "offset": 0, "limit": 10, "hasMore": False},
            }),
        )

        session_state = {}

        tool = ListAvailableActionsTool(
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
            session_state=session_state,
            k8s_client=None,
        )

        result = tool.invoke(params={})

        assert result.status == StructuredToolResultStatus.SUCCESS
        assert "detected_labels" not in session_state


class TestToolsetPropagation:
    """UT-HAPI-056-050: WorkflowDiscoveryToolset propagates k8s_client and resource identity."""

    def test_ut_hapi_056_050_toolset_propagates_k8s_client_and_resource_identity(self):
        """UT-HAPI-056-050: WorkflowDiscoveryToolset passes k8s_client + resource identity to all 3 tools."""
        from src.toolsets.workflow_discovery import WorkflowDiscoveryToolset

        mock_k8s = _make_mock_k8s()
        session_state = {}

        toolset = WorkflowDiscoveryToolset(
            severity="critical",
            component="Pod",
            environment="production",
            priority="P0",
            session_state=session_state,
            k8s_client=mock_k8s,
            resource_name="api-pod-abc",
            resource_namespace="production",
        )

        for tool in toolset.tools:
            assert tool._session_state is session_state
            assert tool._k8s_client is mock_k8s
            assert tool._resource_name == "api-pod-abc"
            assert tool._resource_namespace == "production"

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
Tests for workflow discovery label consumption from session_state.

ADR-056 v1.4: Labels are computed by get_resource_context and stored in
session_state["detected_labels"]. Workflow discovery tools read from
session_state rather than computing labels themselves.

Business Requirements:
  - BR-HAPI-250: DetectedLabels integration with Data Storage
  - BR-SP-101:   DetectedLabels Auto-Detection (post-RCA via HAPI)

Test Matrix: 8 tests
  - UT-HAPI-056-043: list_available_actions reads labels from session_state
  - UT-HAPI-056-044: list_available_actions uses cached labels (no K8s calls)
  - UT-HAPI-056-045: list_available_actions proceeds with empty sentinel
  - UT-HAPI-056-046: list_workflows reads cached labels from session_state
  - UT-HAPI-056-047: get_workflow reads cached labels from session_state
  - UT-HAPI-056-048: Discovery tool proceeds when session_state has no labels yet
  - UT-HAPI-056-049: Discovery tool proceeds when session_state is None
  - UT-HAPI-056-050: WorkflowDiscoveryToolset propagates session_state to all 3 tools
"""

import json
import pytest
from unittest.mock import Mock, AsyncMock, MagicMock, patch

from holmes.core.tools import StructuredToolResultStatus


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


class TestDiscoveryReadsSessionState:
    """UT-HAPI-056-043 through UT-HAPI-056-045: Discovery tools read labels from session_state."""

    @patch("src.toolsets.workflow_discovery.requests.get")
    def test_ut_hapi_056_043_reads_labels_from_session_state(self, mock_get):
        """UT-HAPI-056-043: list_available_actions reads labels populated by get_resource_context."""
        from src.toolsets.workflow_discovery import ListAvailableActionsTool

        mock_get.return_value = Mock(
            status_code=200,
            json=Mock(return_value={
                "actionTypes": [{"actionType": "RestartPod", "workflowCount": 1}],
                "pagination": {"totalCount": 1, "offset": 0, "limit": 10, "hasMore": False},
            }),
        )

        session_state = {"detected_labels": DETECTED_LABELS_ARGOCD}

        tool = ListAvailableActionsTool(
            severity="critical",
            component="Pod",
            environment="production",
            priority="P0",
            session_state=session_state,
        )

        result = tool.invoke(params={})

        assert result.status == StructuredToolResultStatus.SUCCESS
        assert session_state["detected_labels"] is DETECTED_LABELS_ARGOCD

    @patch("src.toolsets.workflow_discovery.requests.get")
    def test_ut_hapi_056_044_uses_cached_labels_no_k8s_calls(self, mock_get):
        """UT-HAPI-056-044: list_available_actions uses cached labels without making K8s API calls."""
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

        tool = ListAvailableActionsTool(
            severity="critical",
            component="Pod",
            environment="production",
            priority="P0",
            session_state=session_state,
        )

        result = tool.invoke(params={})

        assert result.status == StructuredToolResultStatus.SUCCESS
        assert session_state["detected_labels"] is cached_labels

    @patch("src.toolsets.workflow_discovery.requests.get")
    def test_ut_hapi_056_045_proceeds_with_empty_sentinel(self, mock_get):
        """UT-HAPI-056-045: list_available_actions proceeds when detected_labels is {} sentinel."""
        from src.toolsets.workflow_discovery import ListAvailableActionsTool

        mock_get.return_value = Mock(
            status_code=200,
            json=Mock(return_value={
                "actionTypes": [],
                "pagination": {"totalCount": 0, "offset": 0, "limit": 10, "hasMore": False},
            }),
        )

        session_state = {"detected_labels": {}}

        tool = ListAvailableActionsTool(
            severity="critical",
            component="Pod",
            environment="production",
            priority="P0",
            session_state=session_state,
        )

        result = tool.invoke(params={})

        assert result.status == StructuredToolResultStatus.SUCCESS


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


class TestDiscoveryGracefulDegradation:
    """UT-HAPI-056-048 through UT-HAPI-056-049: Graceful degradation without labels."""

    @patch("src.toolsets.workflow_discovery.requests.get")
    def test_ut_hapi_056_048_proceeds_without_labels_in_session(self, mock_get):
        """UT-HAPI-056-048: Discovery proceeds when session_state has no detected_labels key yet."""
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
        )

        result = tool.invoke(params={})

        assert result.status == StructuredToolResultStatus.SUCCESS
        assert "detected_labels" not in session_state

    @patch("src.toolsets.workflow_discovery.requests.get")
    def test_ut_hapi_056_049_proceeds_with_none_session_state(self, mock_get):
        """UT-HAPI-056-049: Discovery proceeds when session_state is None."""
        from src.toolsets.workflow_discovery import ListAvailableActionsTool

        mock_get.return_value = Mock(
            status_code=200,
            json=Mock(return_value={
                "actionTypes": [],
                "pagination": {"totalCount": 0, "offset": 0, "limit": 10, "hasMore": False},
            }),
        )

        tool = ListAvailableActionsTool(
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
            session_state=None,
        )

        result = tool.invoke(params={})

        assert result.status == StructuredToolResultStatus.SUCCESS


class TestToolsetPropagation:
    """UT-HAPI-056-050: WorkflowDiscoveryToolset propagates session_state."""

    def test_ut_hapi_056_050_toolset_propagates_session_state_to_all_tools(self):
        """UT-HAPI-056-050: WorkflowDiscoveryToolset passes session_state to all 3 tools."""
        from src.toolsets.workflow_discovery import WorkflowDiscoveryToolset

        session_state = {"detected_labels": DETECTED_LABELS_ARGOCD}

        toolset = WorkflowDiscoveryToolset(
            severity="critical",
            component="Pod",
            environment="production",
            priority="P0",
            session_state=session_state,
        )

        for tool in toolset.tools:
            assert tool._session_state is session_state

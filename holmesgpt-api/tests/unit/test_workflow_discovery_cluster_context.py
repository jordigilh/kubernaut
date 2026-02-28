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
Tests for cluster_context in list_available_actions tool response.

Authority: ADR-056 v1.3, DD-HAPI-017 v1.3, DD-HAPI-018 v1.1
Business Requirement: BR-HAPI-017-007

ADR-056 v1.3 surfaces detected labels to the LLM as a read-only
cluster_context section in the list_available_actions tool response.
This gives the LLM explicit infrastructure context for informed
action type selection without making labels a tool parameter.

Test Matrix: 7 tests
  - UT-HAPI-056-081: cluster_context present when detected_labels non-empty
  - UT-HAPI-056-082: failedDetections stripped from cluster_context
  - UT-HAPI-056-083: cluster_context omitted when no detected_labels key
  - UT-HAPI-056-084: cluster_context omitted when detected_labels is empty dict
  - UT-HAPI-056-085: cluster_context includes human-readable note field
  - UT-HAPI-056-086: list_workflows does NOT include cluster_context
  - UT-HAPI-056-087: get_workflow does NOT include cluster_context
"""

import json
from unittest.mock import Mock, patch
import pytest

from holmes.core.tools import StructuredToolResultStatus


DS_ACTION_TYPES_RESPONSE = {
    "actionTypes": [
        {
            "actionType": "GracefulRestart",
            "description": {
                "what": "Restart pods gracefully",
                "when_to_use": "Memory leaks",
                "when_not_to_use": "Persistent bugs",
                "preconditions": "Deployment managed",
            },
            "workflowCount": 1,
        }
    ],
    "pagination": {"totalCount": 1, "offset": 0, "limit": 10, "hasMore": False},
}

DS_WORKFLOWS_RESPONSE = {
    "workflows": [
        {
            "workflowId": "graceful-restart-v1",
            "workflowName": "Graceful Restart",
            "version": "1.0.0",
            "actionType": "GracefulRestart",
        }
    ],
    "pagination": {"totalCount": 1, "offset": 0, "limit": 10, "hasMore": False},
}

DS_SINGLE_WORKFLOW_RESPONSE = {
    "workflowId": "git-revert-v1",
    "workflow_name": "Git Revert",
    "version": "1.0.0",
    "actionType": "GitRevertCommit",
    "parameters": [],
}


def _mock_ds_response(data):
    """Create a mock HTTP response returning the given JSON data."""
    mock_resp = Mock()
    mock_resp.status_code = 200
    mock_resp.json.return_value = data
    return mock_resp


class TestClusterContextPresence:
    """UT-HAPI-056-081, 083, 084: cluster_context inclusion/omission based on detected_labels state."""

    @patch("src.toolsets.workflow_discovery.requests.get")
    def test_ut_hapi_056_081_cluster_context_present_with_labels(self, mock_get):
        """UT-HAPI-056-081: list_available_actions includes cluster_context when detected_labels present."""
        from src.toolsets.workflow_discovery import ListAvailableActionsTool

        mock_get.return_value = _mock_ds_response(DS_ACTION_TYPES_RESPONSE.copy())

        session_state = {
            "detected_labels": {
                "gitOpsManaged": True,
                "gitOpsTool": "argocd",
                "pdbProtected": False,
            }
        }

        tool = ListAvailableActionsTool(
            data_storage_url="http://mock:8080",
            severity="critical",
            component="Pod",
            environment="production",
            priority="P0",
            session_state=session_state,
        )

        result = tool.invoke(params={"offset": 0, "limit": 10})

        assert result.status == StructuredToolResultStatus.SUCCESS
        data = json.loads(result.data)

        assert "cluster_context" in data, "cluster_context must be present when detected_labels is non-empty"
        assert "detected_labels" in data["cluster_context"]
        assert data["cluster_context"]["detected_labels"]["gitOpsManaged"] is True
        assert data["cluster_context"]["detected_labels"]["gitOpsTool"] == "argocd"
        assert data["cluster_context"]["detected_labels"]["pdbProtected"] is False

        assert "actionTypes" in data, "Original DS response fields must be preserved"
        assert data["pagination"]["totalCount"] == 1

    @patch("src.toolsets.workflow_discovery.requests.get")
    def test_ut_hapi_056_083_no_cluster_context_without_key(self, mock_get):
        """UT-HAPI-056-083: cluster_context omitted when session_state has no detected_labels key."""
        from src.toolsets.workflow_discovery import ListAvailableActionsTool

        mock_get.return_value = _mock_ds_response(DS_ACTION_TYPES_RESPONSE.copy())

        session_state = {}

        tool = ListAvailableActionsTool(
            data_storage_url="http://mock:8080",
            severity="critical",
            component="Pod",
            environment="production",
            priority="P0",
            session_state=session_state,
        )

        result = tool.invoke(params={"offset": 0, "limit": 10})

        assert result.status == StructuredToolResultStatus.SUCCESS
        data = json.loads(result.data)

        assert "cluster_context" not in data, "cluster_context must NOT be present when no detected_labels key"
        assert "actionTypes" in data

    @patch("src.toolsets.workflow_discovery.requests.get")
    def test_ut_hapi_056_084_no_cluster_context_with_empty_dict(self, mock_get):
        """UT-HAPI-056-084: cluster_context omitted when detected_labels is empty dict."""
        from src.toolsets.workflow_discovery import ListAvailableActionsTool

        mock_get.return_value = _mock_ds_response(DS_ACTION_TYPES_RESPONSE.copy())

        session_state = {"detected_labels": {}}

        tool = ListAvailableActionsTool(
            data_storage_url="http://mock:8080",
            severity="critical",
            component="Pod",
            environment="production",
            priority="P0",
            session_state=session_state,
        )

        result = tool.invoke(params={"offset": 0, "limit": 10})

        assert result.status == StructuredToolResultStatus.SUCCESS
        data = json.loads(result.data)

        assert "cluster_context" not in data, "cluster_context must NOT be present when detected_labels is empty"


class TestClusterContextFailedDetections:
    """UT-HAPI-056-082: failedDetections stripped from cluster_context."""

    @patch("src.toolsets.workflow_discovery.requests.get")
    def test_ut_hapi_056_082_failed_detections_excluded(self, mock_get):
        """UT-HAPI-056-082: cluster_context.detected_labels excludes failedDetections fields."""
        from src.toolsets.workflow_discovery import ListAvailableActionsTool

        mock_get.return_value = _mock_ds_response(DS_ACTION_TYPES_RESPONSE.copy())

        session_state = {
            "detected_labels": {
                "failedDetections": ["pdbProtected"],
                "gitOpsManaged": True,
                "gitOpsTool": "argocd",
                "pdbProtected": False,
                "hpaEnabled": False,
            }
        }

        tool = ListAvailableActionsTool(
            data_storage_url="http://mock:8080",
            severity="critical",
            component="Pod",
            environment="production",
            priority="P0",
            session_state=session_state,
        )

        result = tool.invoke(params={"offset": 0, "limit": 10})

        assert result.status == StructuredToolResultStatus.SUCCESS
        data = json.loads(result.data)

        assert "cluster_context" in data
        ctx_labels = data["cluster_context"]["detected_labels"]
        assert "pdbProtected" not in ctx_labels, "Failed detection field must be excluded"
        assert "failedDetections" not in ctx_labels, "failedDetections array itself must be excluded"
        assert ctx_labels["gitOpsManaged"] is True, "Non-failed fields must be preserved"


class TestClusterContextNote:
    """UT-HAPI-056-085: cluster_context includes human-readable note."""

    @patch("src.toolsets.workflow_discovery.requests.get")
    def test_ut_hapi_056_085_note_field_present(self, mock_get):
        """UT-HAPI-056-085: cluster_context includes human-readable note field."""
        from src.toolsets.workflow_discovery import ListAvailableActionsTool

        mock_get.return_value = _mock_ds_response(DS_ACTION_TYPES_RESPONSE.copy())

        session_state = {
            "detected_labels": {"hpaEnabled": True}
        }

        tool = ListAvailableActionsTool(
            data_storage_url="http://mock:8080",
            severity="warning",
            component="Deployment",
            environment="production",
            priority="P1",
            session_state=session_state,
        )

        result = tool.invoke(params={"offset": 0, "limit": 10})

        assert result.status == StructuredToolResultStatus.SUCCESS
        data = json.loads(result.data)

        assert "cluster_context" in data
        assert "note" in data["cluster_context"]
        note = data["cluster_context"]["note"]
        assert isinstance(note, str)
        assert len(note) > 0, "Note must be non-empty"
        assert "action type" in note.lower(), "Note must guide LLM to use labels for action type selection"


class TestClusterContextGitOpsNote:
    """UT-HAPI-219-001: cluster_context note is prescriptive when gitOpsManaged=true.

    Issue #219: LLM selected FixCertificate instead of GitRevertCommit because the
    generic cluster_context note was too weak. When gitOpsManaged=true, the note must
    explicitly instruct the LLM to prefer git-based action types.
    """

    @patch("src.toolsets.workflow_discovery.requests.get")
    def test_ut_hapi_219_001_gitops_note_prescriptive(self, mock_get):
        """UT-HAPI-219-001: When gitOpsManaged=true, cluster_context note must prescribe git-based actions."""
        from src.toolsets.workflow_discovery import ListAvailableActionsTool

        mock_get.return_value = _mock_ds_response(DS_ACTION_TYPES_RESPONSE.copy())

        session_state = {
            "detected_labels": {
                "gitOpsManaged": True,
                "gitOpsTool": "argocd",
                "pdbProtected": False,
            }
        }

        tool = ListAvailableActionsTool(
            data_storage_url="http://mock:8080",
            severity="critical",
            component="Pod",
            environment="production",
            priority="P0",
            session_state=session_state,
        )

        result = tool.invoke(params={"offset": 0, "limit": 10})

        assert result.status == StructuredToolResultStatus.SUCCESS
        data = json.loads(result.data)

        assert "cluster_context" in data
        note = data["cluster_context"]["note"]
        assert "gitOpsManaged" in note, "Note must reference gitOpsManaged when it is true"
        assert "git-based" in note.lower() or "gitrevert" in note.lower(), \
            "Note must instruct LLM to prefer git-based action types"
        assert "source-of-truth" in note.lower() or "direct" in note.lower(), \
            "Note must warn against direct kubectl actions in GitOps environments"

    @patch("src.toolsets.workflow_discovery.requests.get")
    def test_ut_hapi_219_002_non_gitops_note_remains_generic(self, mock_get):
        """UT-HAPI-219-002: When gitOpsManaged=false/absent, cluster_context note remains generic."""
        from src.toolsets.workflow_discovery import ListAvailableActionsTool

        mock_get.return_value = _mock_ds_response(DS_ACTION_TYPES_RESPONSE.copy())

        session_state = {
            "detected_labels": {
                "gitOpsManaged": False,
                "hpaEnabled": True,
            }
        }

        tool = ListAvailableActionsTool(
            data_storage_url="http://mock:8080",
            severity="warning",
            component="Deployment",
            environment="production",
            priority="P1",
            session_state=session_state,
        )

        result = tool.invoke(params={"offset": 0, "limit": 10})

        assert result.status == StructuredToolResultStatus.SUCCESS
        data = json.loads(result.data)

        assert "cluster_context" in data
        note = data["cluster_context"]["note"]
        assert "gitOpsManaged" not in note, \
            "Generic note must NOT reference gitOpsManaged when it is false"


class TestClusterContextNotInSteps2And3:
    """UT-HAPI-056-086, 087: list_workflows and get_workflow do NOT include cluster_context."""

    @patch("src.toolsets.workflow_discovery.requests.get")
    def test_ut_hapi_056_086_list_workflows_no_cluster_context(self, mock_get):
        """UT-HAPI-056-086: list_workflows does NOT include cluster_context."""
        from src.toolsets.workflow_discovery import ListWorkflowsTool

        mock_get.return_value = _mock_ds_response(DS_WORKFLOWS_RESPONSE.copy())

        session_state = {
            "detected_labels": {
                "gitOpsManaged": True,
                "gitOpsTool": "argocd",
            }
        }

        tool = ListWorkflowsTool(
            data_storage_url="http://mock:8080",
            severity="critical",
            component="Pod",
            environment="production",
            priority="P0",
            session_state=session_state,
        )

        result = tool.invoke(params={"action_type": "GitRevertCommit"})

        assert result.status == StructuredToolResultStatus.SUCCESS
        data = json.loads(result.data)

        assert "cluster_context" not in data, "Step 2 must NOT include cluster_context"
        assert "workflows" in data

    @patch("src.toolsets.workflow_discovery.requests.get")
    def test_ut_hapi_056_087_get_workflow_no_cluster_context(self, mock_get):
        """UT-HAPI-056-087: get_workflow does NOT include cluster_context."""
        from src.toolsets.workflow_discovery import GetWorkflowTool

        mock_get.return_value = _mock_ds_response(DS_SINGLE_WORKFLOW_RESPONSE.copy())

        session_state = {
            "detected_labels": {
                "gitOpsManaged": True,
                "gitOpsTool": "argocd",
            }
        }

        tool = GetWorkflowTool(
            data_storage_url="http://mock:8080",
            severity="critical",
            component="Pod",
            environment="production",
            priority="P0",
            session_state=session_state,
        )

        result = tool.invoke(params={"workflow_id": "git-revert-v1"})

        assert result.status == StructuredToolResultStatus.SUCCESS
        data = json.loads(result.data)

        assert "cluster_context" not in data, "Step 3 must NOT include cluster_context"

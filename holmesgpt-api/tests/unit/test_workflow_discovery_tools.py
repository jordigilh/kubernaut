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
Unit Tests for Three-Step Workflow Discovery Tools

Authority: DD-WORKFLOW-016 (Action-Type Workflow Catalog Indexing)
Authority: DD-HAPI-017 (Three-Step Workflow Discovery Integration)
Business Requirement: BR-HAPI-017-001 (Three-Step Tool Implementation)

Test Plan: docs/testing/DD-HAPI-017/TEST_PLAN.md
Test IDs: UT-HAPI-017-001-001 through UT-HAPI-017-001-009

Strategy: TDD RED phase - tests written FIRST, implementation follows.
Infrastructure: None (mocked HTTP responses)

Three tools:
  Step 1: ListAvailableActionsTool
  Step 2: ListWorkflowsTool
  Step 3: GetWorkflowTool
"""

import json
from unittest.mock import Mock, patch, MagicMock
import pytest

from holmes.core.tools import StructuredToolResultStatus


# ========================================
# STEP 1: ListAvailableActionsTool Tests
# ========================================

class TestListAvailableActionsTool:
    """
    UT-HAPI-017-001-001 through 003: Unit tests for list_available_actions tool
    """

    def test_successful_action_listing_ut_001(self):
        """
        UT-HAPI-017-001-001: Successful listing of available action types

        Verifies tool returns action types with descriptions and workflow counts.
        """
        from src.toolsets.workflow_discovery import ListAvailableActionsTool

        tool = ListAvailableActionsTool(
            data_storage_url="http://mock:8080",
            remediation_id="rem-test-001",
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
        )

        # Mock the HTTP GET call
        mock_response = Mock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            "actionTypes": [
                {
                    "actionType": "ScaleReplicas",
                    "description": {
                        "what": "Horizontally scale a workload",
                        "when_to_use": "OOMKilled events",
                        "when_not_to_use": "Code bugs",
                        "preconditions": "HPA not managing"
                    },
                    "workflowCount": 3
                },
                {
                    "actionType": "RestartPod",
                    "description": {
                        "what": "Delete and recreate a pod",
                        "when_to_use": "CrashLoopBackOff",
                        "when_not_to_use": "Persistent bugs",
                        "preconditions": "Pod managed by controller"
                    },
                    "workflowCount": 2
                }
            ],
            "pagination": {
                "totalCount": 2,
                "offset": 0,
                "limit": 10,
                "hasMore": False
            }
        }

        with patch('requests.get', return_value=mock_response):
            result = tool.invoke(params={"offset": 0, "limit": 10})

        assert result is not None
        assert result.status == StructuredToolResultStatus.SUCCESS
        data = json.loads(result.data)
        assert "actionTypes" in data
        assert len(data["actionTypes"]) == 2
        assert data["actionTypes"][0]["actionType"] == "ScaleReplicas"
        assert data["pagination"]["totalCount"] == 2

    def test_pagination_support_ut_002(self):
        """
        UT-HAPI-017-001-002: Pagination parameters passed correctly

        Verifies offset and limit are forwarded to DS.
        """
        from src.toolsets.workflow_discovery import ListAvailableActionsTool

        tool = ListAvailableActionsTool(
            data_storage_url="http://mock:8080",
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
        )

        mock_response = Mock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            "actionTypes": [],
            "pagination": {"totalCount": 5, "offset": 3, "limit": 2, "hasMore": True}
        }

        with patch('requests.get', return_value=mock_response) as mock_get:
            result = tool.invoke(params={"offset": 3, "limit": 2})

        assert result.status == StructuredToolResultStatus.SUCCESS
        # Verify query params include offset and limit
        call_args = mock_get.call_args
        params = call_args[1].get("params", {})
        assert params.get("offset") == 3
        assert params.get("limit") == 2

    def test_context_filters_propagated_ut_003(self):
        """
        UT-HAPI-017-001-003: Signal context filters propagated as query params

        Verifies severity, component, environment, priority are sent to DS.
        """
        from src.toolsets.workflow_discovery import ListAvailableActionsTool

        tool = ListAvailableActionsTool(
            data_storage_url="http://mock:8080",
            remediation_id="rem-test-003",
            severity="high",
            component="deployment",
            environment="staging",
            priority="P1",
        )

        mock_response = Mock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            "actionTypes": [],
            "pagination": {"totalCount": 0, "offset": 0, "limit": 10, "hasMore": False}
        }

        with patch('requests.get', return_value=mock_response) as mock_get:
            result = tool.invoke(params={})

        assert result.status == StructuredToolResultStatus.SUCCESS
        # Verify query params contain context filters
        call_args = mock_get.call_args
        params = call_args[1].get("params", {})
        assert params.get("severity") == "high"
        assert params.get("component") == "deployment"
        assert params.get("environment") == "staging"
        assert params.get("priority") == "P1"
        assert params.get("remediation_id") == "rem-test-003"

    def test_connection_error_ut_004(self):
        """
        UT-HAPI-017-001-004: Connection error handled gracefully

        Verifies tool returns ERROR status on connection failure.
        """
        from src.toolsets.workflow_discovery import ListAvailableActionsTool
        import requests

        tool = ListAvailableActionsTool(
            data_storage_url="http://unreachable:8080",
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
        )

        with patch('requests.get', side_effect=requests.exceptions.ConnectionError("refused")):
            result = tool.invoke(params={})

        assert result.status == StructuredToolResultStatus.ERROR
        assert "connect" in result.error.lower() or "refused" in result.error.lower()


# ========================================
# STEP 2: ListWorkflowsTool Tests
# ========================================

class TestListWorkflowsTool:
    """
    UT-HAPI-017-001-004 through 006: Unit tests for list_workflows tool
    """

    def test_successful_workflow_listing_ut_004(self):
        """
        UT-HAPI-017-001-004: Successful listing of workflows for action type

        Verifies tool returns workflows with summary information.
        """
        from src.toolsets.workflow_discovery import ListWorkflowsTool

        tool = ListWorkflowsTool(
            data_storage_url="http://mock:8080",
            remediation_id="rem-test-004",
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
        )

        mock_response = Mock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            "actionType": "ScaleReplicas",
            "workflows": [
                {
                    "workflowId": "wf-uuid-001",
                    "workflowName": "scale-conservative",
                    "name": "Conservative Scale",
                    "description": "Scales replicas conservatively",
                    "version": "v1.0.0",
                    "containerImage": "quay.io/kubernaut-ai/scale:v1.0.0",
                    "executionEngine": "tekton",
                    "actualSuccessRate": 0.95,
                    "totalExecutions": 42
                }
            ],
            "pagination": {
                "totalCount": 1,
                "offset": 0,
                "limit": 10,
                "hasMore": False
            }
        }

        with patch('requests.get', return_value=mock_response):
            result = tool.invoke(params={"action_type": "ScaleReplicas"})

        assert result.status == StructuredToolResultStatus.SUCCESS
        data = json.loads(result.data)
        assert data["actionType"] == "ScaleReplicas"
        assert len(data["workflows"]) == 1
        assert data["workflows"][0]["workflowId"] == "wf-uuid-001"
        assert data["workflows"][0]["actualSuccessRate"] == 0.95

    def test_missing_action_type_ut_005(self):
        """
        UT-HAPI-017-001-005: Missing action_type parameter returns error

        Verifies tool validates required action_type parameter.
        """
        from src.toolsets.workflow_discovery import ListWorkflowsTool

        tool = ListWorkflowsTool(
            data_storage_url="http://mock:8080",
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
        )

        result = tool.invoke(params={})

        assert result.status == StructuredToolResultStatus.ERROR
        assert "action_type" in result.error.lower()

    def test_pagination_all_pages_ut_006(self):
        """
        UT-HAPI-017-001-006: Tool supports pagination parameters

        Verifies offset and limit can be provided for multi-page retrieval.
        DD-WORKFLOW-016: LLM MUST review ALL pages.
        """
        from src.toolsets.workflow_discovery import ListWorkflowsTool

        tool = ListWorkflowsTool(
            data_storage_url="http://mock:8080",
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
        )

        mock_response = Mock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            "actionType": "ScaleReplicas",
            "workflows": [{"workflowId": "wf-1", "workflowName": "wf-1"}],
            "pagination": {"totalCount": 15, "offset": 10, "limit": 10, "hasMore": True}
        }

        with patch('requests.get', return_value=mock_response) as mock_get:
            result = tool.invoke(params={"action_type": "ScaleReplicas", "offset": 10, "limit": 10})

        assert result.status == StructuredToolResultStatus.SUCCESS
        data = json.loads(result.data)
        assert data["pagination"]["hasMore"] is True
        assert data["pagination"]["totalCount"] == 15


# ========================================
# STEP 3: GetWorkflowTool Tests
# ========================================

class TestGetWorkflowTool:
    """
    UT-HAPI-017-001-007 through 009: Unit tests for get_workflow tool
    """

    def test_successful_workflow_retrieval_ut_007(self):
        """
        UT-HAPI-017-001-007: Successful workflow retrieval by ID

        Verifies tool returns full workflow with parameter schema.
        """
        from src.toolsets.workflow_discovery import GetWorkflowTool

        tool = GetWorkflowTool(
            data_storage_url="http://mock:8080",
            remediation_id="rem-test-007",
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
        )

        mock_response = Mock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            "workflowId": "wf-uuid-001",
            "workflowName": "scale-conservative",
            "name": "Conservative Scale",
            "description": "Scales replicas conservatively",
            "version": "v1.0.0",
            "containerImage": "quay.io/kubernaut-ai/scale:v1.0.0",
            "actionType": "ScaleReplicas",
            "parameters": {
                "schema": {
                    "parameters": [
                        {"name": "REPLICAS", "type": "integer", "required": True},
                        {"name": "NAMESPACE", "type": "string", "required": True}
                    ]
                }
            }
        }

        with patch('requests.get', return_value=mock_response):
            result = tool.invoke(params={"workflow_id": "wf-uuid-001"})

        assert result.status == StructuredToolResultStatus.SUCCESS
        data = json.loads(result.data)
        assert data["workflowId"] == "wf-uuid-001"
        assert "parameters" in data

    def test_missing_workflow_id_ut_008(self):
        """
        UT-HAPI-017-001-008: Missing workflow_id parameter returns error

        Verifies tool validates required workflow_id parameter.
        """
        from src.toolsets.workflow_discovery import GetWorkflowTool

        tool = GetWorkflowTool(
            data_storage_url="http://mock:8080",
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
        )

        result = tool.invoke(params={})

        assert result.status == StructuredToolResultStatus.ERROR
        assert "workflow_id" in result.error.lower()

    def test_workflow_not_found_security_gate_ut_009(self):
        """
        UT-HAPI-017-001-009: 404 response (security gate) handled correctly

        DD-WORKFLOW-016: When context filters don't match, DS returns 404.
        Tool should return ERROR with appropriate message.
        """
        from src.toolsets.workflow_discovery import GetWorkflowTool
        import requests

        tool = GetWorkflowTool(
            data_storage_url="http://mock:8080",
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
        )

        mock_response = Mock()
        mock_response.status_code = 404
        mock_response.raise_for_status.side_effect = requests.exceptions.HTTPError(
            response=mock_response
        )
        mock_response.json.return_value = {
            "type": "https://kubernaut.ai/problems/workflow-not-found",
            "title": "Workflow Not Found",
            "status": 404,
            "detail": "Workflow not found or does not match the provided signal context."
        }

        with patch('requests.get', return_value=mock_response):
            result = tool.invoke(params={"workflow_id": "wf-nonexistent"})

        assert result.status == StructuredToolResultStatus.ERROR
        assert "not found" in result.error.lower() or "404" in result.error


# ========================================
# TOOLSET REGISTRATION Tests
# ========================================

class TestWorkflowDiscoveryToolset:
    """
    UT-HAPI-017-003-001: Toolset registers all three discovery tools
    """

    def test_toolset_contains_three_tools(self):
        """
        Verifies WorkflowDiscoveryToolset has exactly 3 tools.
        """
        from src.toolsets.workflow_discovery import WorkflowDiscoveryToolset

        toolset = WorkflowDiscoveryToolset(
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
        )

        assert len(toolset.tools) == 3
        tool_names = [t.name for t in toolset.tools]
        assert "list_available_actions" in tool_names
        assert "list_workflows" in tool_names
        assert "get_workflow" in tool_names

    def test_toolset_enabled_by_default(self):
        """
        Verifies toolset is enabled by default.
        """
        from src.toolsets.workflow_discovery import WorkflowDiscoveryToolset

        toolset = WorkflowDiscoveryToolset(
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
        )

        assert toolset.enabled is True

    def test_toolset_propagates_context(self):
        """
        Verifies toolset passes context (remediation_id, filters) to all tools.
        """
        from src.toolsets.workflow_discovery import WorkflowDiscoveryToolset

        toolset = WorkflowDiscoveryToolset(
            remediation_id="rem-ctx-test",
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
        )

        for tool in toolset.tools:
            assert tool._remediation_id == "rem-ctx-test"
            assert tool._severity == "critical"
            assert tool._component == "pod"
            assert tool._environment == "production"
            assert tool._priority == "P0"

    def test_toolset_passes_detected_labels_to_all_tools(self):
        """
        DD-HAPI-017: WorkflowDiscoveryToolset passes detected_labels to all three tools.

        BR: BR-HAPI-017-001
        Verifies detected_labels are propagated to list_available_actions,
        list_workflows, and get_workflow.
        """
        from src.toolsets.workflow_discovery import WorkflowDiscoveryToolset
        from src.models.incident_models import DetectedLabels

        detected_labels = DetectedLabels(gitOpsManaged=True, gitOpsTool="argocd")

        toolset = WorkflowDiscoveryToolset(
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
            detected_labels=detected_labels,
        )

        for tool in toolset.tools:
            assert tool._detected_labels is detected_labels

    def test_toolset_accepts_custom_labels(self):
        """
        DD-HAPI-001: WorkflowDiscoveryToolset accepts custom_labels in constructor.

        Ported from legacy test_custom_labels_auto_append_dd_hapi_001.py
        """
        from src.toolsets.workflow_discovery import WorkflowDiscoveryToolset

        custom = {"constraint": ["cost-constrained", "stateful-safe"]}
        toolset = WorkflowDiscoveryToolset(
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
            custom_labels=custom,
        )

        for tool in toolset.tools:
            assert tool._custom_labels == custom

    def test_toolset_handles_none_custom_labels(self):
        """
        DD-HAPI-001: custom_labels=None defaults to empty dict.

        Ported from legacy test_custom_labels_auto_append_dd_hapi_001.py
        """
        from src.toolsets.workflow_discovery import WorkflowDiscoveryToolset

        toolset = WorkflowDiscoveryToolset(
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
            custom_labels=None,
        )

        for tool in toolset.tools:
            # None custom_labels should be stored as None or empty
            assert tool._custom_labels is None or tool._custom_labels == {}

    @patch("src.toolsets.workflow_discovery.requests.get")
    def test_custom_labels_appended_to_query_params(self, mock_get):
        """
        DD-HAPI-001: custom_labels are included in HTTP query params.

        Ported from legacy test_custom_labels_auto_append_dd_hapi_001.py
        """
        from src.toolsets.workflow_discovery import ListAvailableActionsTool

        mock_response = Mock()
        mock_response.status_code = 200
        mock_response.json.return_value = {"action_types": []}
        mock_get.return_value = mock_response

        custom = {"constraint": ["cost-constrained"]}
        tool = ListAvailableActionsTool(
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
            custom_labels=custom,
        )

        tool.invoke(params={})

        mock_get.assert_called_once()
        call_kwargs = mock_get.call_args
        params = call_kwargs.kwargs.get("params") or call_kwargs[1].get("params", {})
        # custom_labels should be serialized in params
        assert "custom_labels" in params

    @patch("src.toolsets.workflow_discovery.requests.get")
    def test_empty_custom_labels_not_in_params(self, mock_get):
        """
        DD-HAPI-001: Empty custom_labels are not added to query params.

        Ported from legacy test_custom_labels_auto_append_dd_hapi_001.py
        """
        from src.toolsets.workflow_discovery import ListAvailableActionsTool

        mock_response = Mock()
        mock_response.status_code = 200
        mock_response.json.return_value = {"action_types": []}
        mock_get.return_value = mock_response

        tool = ListAvailableActionsTool(
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
            custom_labels={},
        )

        tool.invoke(params={})

        mock_get.assert_called_once()
        call_kwargs = mock_get.call_args
        params = call_kwargs.kwargs.get("params") or call_kwargs[1].get("params", {})
        assert "custom_labels" not in params


# ========================================
# PHASE 4b: detected_labels Propagation Tests (DD-HAPI-017)
# ========================================

class TestDetectedLabelsPropagation:
    """
    DD-HAPI-017: Verify detected_labels are passed through the three-step protocol.
    BR-HAPI-017-001: All tools pass full signal context filters including detected_labels.
    DD-WORKFLOW-001 v2.1: strip_failed_detections applied before sending.
    """

    @patch("src.toolsets.workflow_discovery.requests.get")
    def test_build_context_params_includes_detected_labels_when_provided(self, mock_get):
        """
        _build_context_params() includes detected_labels when provided.

        Verifies detected_labels are JSON-encoded and added to query params.
        """
        from src.toolsets.workflow_discovery import ListAvailableActionsTool
        from src.models.incident_models import DetectedLabels

        detected_labels = DetectedLabels(gitOpsManaged=True, gitOpsTool="argocd")

        mock_get.return_value = Mock(
            status_code=200,
            json=Mock(return_value={"actionTypes": [], "pagination": {"totalCount": 0, "offset": 0, "limit": 10, "hasMore": False}}),
            raise_for_status=Mock(),
        )

        tool = ListAvailableActionsTool(
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
            detected_labels=detected_labels,
        )
        tool.invoke({})

        call_kwargs = mock_get.call_args
        params = call_kwargs.kwargs.get("params", {})
        assert "detected_labels" in params
        parsed = json.loads(params["detected_labels"])
        assert parsed.get("gitOpsManaged") is True
        assert parsed.get("gitOpsTool") == "argocd"

    @patch("src.toolsets.workflow_discovery.requests.get")
    def test_strip_failed_detections_applied_before_sending(self, mock_get):
        """
        strip_failed_detections() is applied before sending.

        DD-WORKFLOW-001 v2.1: Fields in failedDetections are excluded from params.
        """
        from src.toolsets.workflow_discovery import ListAvailableActionsTool
        from src.models.incident_models import DetectedLabels

        # pdbProtected detection failed - should be stripped
        detected_labels = DetectedLabels(
            gitOpsManaged=True,
            pdbProtected=False,
            failedDetections=["pdbProtected"],
        )

        mock_get.return_value = Mock(
            status_code=200,
            json=Mock(return_value={"actionTypes": [], "pagination": {"totalCount": 0, "offset": 0, "limit": 10, "hasMore": False}}),
            raise_for_status=Mock(),
        )

        tool = ListAvailableActionsTool(
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
            detected_labels=detected_labels,
        )
        tool.invoke({})

        call_kwargs = mock_get.call_args
        params = call_kwargs.kwargs.get("params", {})
        assert "detected_labels" in params
        parsed = json.loads(params["detected_labels"])
        # pdbProtected should NOT be in params (stripped due to failedDetections)
        assert "pdbProtected" not in parsed
        # gitOpsManaged should be present (not in failedDetections)
        assert parsed.get("gitOpsManaged") is True

    @patch("src.toolsets.workflow_discovery.requests.get")
    def test_detected_labels_none_omitted_from_params(self, mock_get):
        """
        detected_labels=None works - no detected_labels in params.

        When no detected_labels provided, params should not include detected_labels key.
        """
        from src.toolsets.workflow_discovery import ListAvailableActionsTool

        mock_get.return_value = Mock(
            status_code=200,
            json=Mock(return_value={"actionTypes": [], "pagination": {"totalCount": 0, "offset": 0, "limit": 10, "hasMore": False}}),
            raise_for_status=Mock(),
        )

        tool = ListAvailableActionsTool(
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
            detected_labels=None,
        )
        tool.invoke({})

        call_kwargs = mock_get.call_args
        params = call_kwargs.kwargs.get("params", {})
        assert "detected_labels" not in params


# ========================================
# PHASE 5: remediationId Propagation Tests
# (UT-HAPI-017-005-001 through 004)
# ========================================

class TestRemediationIdPropagation:
    """
    UT-HAPI-017-005: Verify remediationId is propagated as a query parameter
    on all three discovery steps for audit correlation (BR-HAPI-017-005).
    """

    @patch("src.toolsets.workflow_discovery.requests.get")
    def test_list_available_actions_passes_remediation_id_ut_005_001(self, mock_get):
        """
        UT-HAPI-017-005-001: ListAvailableActionsTool passes remediationId.

        BR: BR-HAPI-017-005
        Tool includes remediation_id as query parameter in DS call.
        """
        from src.toolsets.workflow_discovery import ListAvailableActionsTool

        mock_get.return_value = Mock(
            status_code=200,
            json=Mock(return_value={"action_types": [], "total": 0}),
            raise_for_status=Mock(),
        )

        tool = ListAvailableActionsTool(remediation_id="rem-uuid-123")
        tool.invoke({})

        # Verify remediation_id in query params
        call_kwargs = mock_get.call_args
        params = call_kwargs.kwargs.get("params", {})
        assert params.get("remediation_id") == "rem-uuid-123"

    @patch("src.toolsets.workflow_discovery.requests.get")
    def test_list_workflows_passes_remediation_id_ut_005_002(self, mock_get):
        """
        UT-HAPI-017-005-002: ListWorkflowsTool passes remediationId.

        BR: BR-HAPI-017-005
        Tool includes remediation_id as query parameter in DS call.
        """
        from src.toolsets.workflow_discovery import ListWorkflowsTool

        mock_get.return_value = Mock(
            status_code=200,
            json=Mock(return_value={"workflows": [], "total": 0, "hasMore": False}),
            raise_for_status=Mock(),
        )

        tool = ListWorkflowsTool(remediation_id="rem-uuid-123")
        tool.invoke({"action_type": "scale_up"})

        call_kwargs = mock_get.call_args
        params = call_kwargs.kwargs.get("params", {})
        assert params.get("remediation_id") == "rem-uuid-123"

    @patch("src.toolsets.workflow_discovery.requests.get")
    def test_get_workflow_passes_remediation_id_ut_005_003(self, mock_get):
        """
        UT-HAPI-017-005-003: GetWorkflowTool passes remediationId.

        BR: BR-HAPI-017-005
        Tool includes remediation_id as query parameter in DS call.
        """
        from src.toolsets.workflow_discovery import GetWorkflowTool

        mock_get.return_value = Mock(
            status_code=200,
            json=Mock(return_value={
                "workflowId": "wf-001",
                "name": "Test Workflow",
                "actionType": "scale_up",
                "version": "1.0.0",
                "containerImage": "registry.io/wf:1.0",
                "parameters": {},
            }),
            raise_for_status=Mock(),
        )

        tool = GetWorkflowTool(remediation_id="rem-uuid-123")
        tool.invoke({"workflow_id": "wf-001"})

        call_kwargs = mock_get.call_args
        params = call_kwargs.kwargs.get("params", {})
        assert params.get("remediation_id") == "rem-uuid-123"

    @patch("src.toolsets.workflow_discovery.requests.get")
    def test_empty_remediation_id_handled_gracefully_ut_005_004(self, mock_get):
        """
        UT-HAPI-017-005-004: Empty remediationId handled gracefully.

        BR: BR-HAPI-017-005
        Tool proceeds normally when remediationId is None or empty.
        No error raised; remediation_id omitted from query params.
        """
        from src.toolsets.workflow_discovery import ListAvailableActionsTool

        mock_get.return_value = Mock(
            status_code=200,
            json=Mock(return_value={"action_types": [], "total": 0}),
            raise_for_status=Mock(),
        )

        # Test with None
        tool = ListAvailableActionsTool(remediation_id=None)
        result = tool.invoke({})

        # Should succeed
        assert result is not None

        # remediation_id should NOT be in params (empty/None is excluded)
        call_kwargs = mock_get.call_args
        params = call_kwargs.kwargs.get("params", {})
        assert "remediation_id" not in params


# ========================================
# PHASE 6: Old Tool Removal Verification
# (UT-HAPI-017-006-001 through 002)
# ========================================

class TestOldToolRemoval:
    """
    UT-HAPI-017-006: Verify the old SearchWorkflowCatalogTool is no longer
    importable from production code paths and no references remain.
    """

    def test_search_workflow_catalog_tool_not_in_discovery_module_ut_006_001(self):
        """
        UT-HAPI-017-006-001: SearchWorkflowCatalogTool not exported from discovery module.

        BR: BR-HAPI-017-006
        The new workflow_discovery module should NOT export SearchWorkflowCatalogTool.
        """
        import src.toolsets.workflow_discovery as discovery_module

        # The discovery module should not have SearchWorkflowCatalogTool
        assert not hasattr(discovery_module, "SearchWorkflowCatalogTool"), (
            "SearchWorkflowCatalogTool should not be in the new discovery module"
        )

    def test_workflow_catalog_module_deleted_ut_006_003(self):
        """
        UT-HAPI-017-006-003: workflow_catalog.py module no longer exists.

        BR: BR-HAPI-017-006, DD-WORKFLOW-016 Gap 5
        The legacy workflow_catalog.py source module has been deleted.
        Shared utilities moved to workflow_discovery.py.
        """
        import importlib

        with pytest.raises(ModuleNotFoundError):
            importlib.import_module("src.toolsets.workflow_catalog")

    def test_no_active_search_workflow_catalog_usage_in_source_ut_006_002(self):
        """
        UT-HAPI-017-006-002: No active usage of search_workflow_catalog in source.

        BR: BR-HAPI-017-006
        No Python source file in holmesgpt-api/src/ should actively USE the old tool
        (import it, instantiate it, or register it). Docstrings, comments, and
        historical references are acceptable. The old workflow_catalog.py module
        and generated test files are excluded.
        """
        import os

        src_dir = os.path.join(os.path.dirname(__file__), "..", "..", "src")
        src_dir = os.path.abspath(src_dir)

        # Patterns that indicate ACTIVE USAGE (not just documentation)
        active_usage_patterns = [
            "import searchworkflowcatalogtool",      # import statement
            "searchworkflowcatalogtool(",             # instantiation
            'name="search_workflow_catalog"',         # tool name registration
            "name='search_workflow_catalog'",         # tool name registration (single quotes)
        ]

        # Files to exclude (legacy module and generated test files)
        excluded_paths = {"workflow_catalog.py", "test_", "__pycache__"}

        matches = []
        for root, _dirs, files in os.walk(src_dir):
            for fname in files:
                if not fname.endswith(".py"):
                    continue
                # Skip excluded files
                if any(excl in fname for excl in excluded_paths):
                    continue
                # Skip test directories under src/ (generated client tests)
                if "/test/" in root or "/tests/" in root:
                    continue

                fpath = os.path.join(root, fname)
                with open(fpath, "r") as f:
                    for line_no, line in enumerate(f, 1):
                        stripped = line.strip()
                        if stripped.startswith("#"):
                            continue
                        line_lower = line.lower().replace(" ", "")
                        for pattern in active_usage_patterns:
                            if pattern.replace(" ", "") in line_lower:
                                matches.append(f"{fpath}:{line_no}: {stripped}")
                                break

        assert len(matches) == 0, (
            f"Found {len(matches)} active usage(s) of search_workflow_catalog in src/:\n"
            + "\n".join(matches)
        )

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
Unit Tests for Execution Bundle Consumption (Issue #89 Phase 2)

TDD RED Phase: These tests are written FIRST, before implementation.
They will FAIL until the HAPI rename from container_image -> execution_bundle is complete.

Test Plan: docs/testing/DD-WORKFLOW-017/hapi_execution_bundle_test_plan_v1.0.md
Test IDs: UT-HAPI-017-001 through UT-HAPI-017-008

Business Requirements:
- BR-HAPI-196: Execution bundle consistency validation (renamed from container image)
- BR-HAPI-017-001: Three-step discovery tool integration
- DD-WORKFLOW-017: Workflow lifecycle field renames

Phase 1 prerequisite: DataStorage now returns execution_bundle (not container_image)
in API responses. HAPI must consume the renamed field end-to-end.
"""

import json
import pytest
from unittest.mock import Mock, patch


# =============================================================================
# FIXTURES
# =============================================================================

CATALOG_EXECUTION_BUNDLE = (
    "quay.io/kubernaut/bundles/restart@sha256:"
    "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
)

MISMATCHED_EXECUTION_BUNDLE = (
    "quay.io/kubernaut/bundles/restart@sha256:"
    "9999991234567890999999123456789099999912345678909999991234567890"
)


@pytest.fixture
def mock_ds_client():
    """Mock Data Storage client that returns a workflow with execution_bundle."""
    return Mock()


@pytest.fixture
def mock_workflow_with_execution_bundle():
    """
    Mock workflow object as returned by DS after Phase 1 rename.
    Has execution_bundle (not container_image).
    """
    workflow = Mock()
    workflow.workflow_id = "test-wf-001"
    workflow.execution_bundle = CATALOG_EXECUTION_BUNDLE
    workflow.schema_image = (
        "quay.io/kubernaut/schemas/restart@sha256:"
        "aaa111bbb222ccc333ddd444eee555fff666aaa111bbb222ccc333ddd444eee555"
    )
    workflow.action_type = "RestartPod"
    workflow.parameters = {
        "schema": {
            "parameters": [
                {
                    "name": "NAMESPACE",
                    "type": "string",
                    "required": True,
                    "description": "Target namespace",
                }
            ]
        }
    }
    return workflow


# =============================================================================
# PHASE 1: Validator - execution_bundle rename (UT-HAPI-017-001 to 004)
# =============================================================================

class TestValidatorExecutionBundleRename:
    """
    Tests that WorkflowResponseValidator reads execution_bundle from catalog
    and sets validated_execution_bundle on ValidationResult.

    Authority: DD-WORKFLOW-017 (Field Rename), BR-HAPI-196 (Consistency)
    Test Plan: docs/testing/DD-WORKFLOW-017/hapi_execution_bundle_test_plan_v1.0.md
    """

    def test_ut_hapi_017_001_validator_reads_execution_bundle_from_catalog(
        self, mock_ds_client, mock_workflow_with_execution_bundle
    ):
        """
        UT-HAPI-017-001: Validator reads execution_bundle from catalog and sets
        validated_execution_bundle when LLM provides None.

        Given: Catalog workflow has execution_bundle set
        When: validate() called with execution_bundle=None
        Then: result.validated_execution_bundle equals catalog value
        """
        mock_ds_client.get_workflow_by_id.return_value = mock_workflow_with_execution_bundle

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(mock_ds_client)

        result = validator.validate(
            workflow_id="test-wf-001",
            execution_bundle=None,
            parameters={"NAMESPACE": "default"},
        )

        assert result.is_valid is True
        assert result.validated_execution_bundle == CATALOG_EXECUTION_BUNDLE
        assert len(result.errors) == 0

    def test_ut_hapi_017_002_validator_passes_on_matching_execution_bundle(
        self, mock_ds_client, mock_workflow_with_execution_bundle
    ):
        """
        UT-HAPI-017-002: Validator passes when LLM execution_bundle matches catalog exactly.

        Given: Catalog and LLM both have the same execution_bundle
        When: validate() called with matching execution_bundle
        Then: result.is_valid is True, validated_execution_bundle equals catalog
        """
        mock_ds_client.get_workflow_by_id.return_value = mock_workflow_with_execution_bundle

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(mock_ds_client)

        result = validator.validate(
            workflow_id="test-wf-001",
            execution_bundle=CATALOG_EXECUTION_BUNDLE,
            parameters={"NAMESPACE": "default"},
        )

        assert result.is_valid is True
        assert result.validated_execution_bundle == CATALOG_EXECUTION_BUNDLE
        assert len(result.errors) == 0

    def test_ut_hapi_017_003_validator_rejects_mismatched_execution_bundle(
        self, mock_ds_client, mock_workflow_with_execution_bundle
    ):
        """
        UT-HAPI-017-003: Validator reports error when LLM execution_bundle differs from catalog.

        Given: LLM provides a different execution_bundle than catalog (hallucination)
        When: validate() called with mismatched execution_bundle
        Then: result.is_valid is False, error mentions mismatch,
              validated_execution_bundle still set to catalog value for self-correction
        """
        mock_ds_client.get_workflow_by_id.return_value = mock_workflow_with_execution_bundle

        from src.validation.workflow_response_validator import WorkflowResponseValidator

        validator = WorkflowResponseValidator(mock_ds_client)

        result = validator.validate(
            workflow_id="test-wf-001",
            execution_bundle=MISMATCHED_EXECUTION_BUNDLE,
            parameters={"NAMESPACE": "default"},
        )

        assert result.is_valid is False
        assert any("mismatch" in e.lower() for e in result.errors)
        assert result.validated_execution_bundle == CATALOG_EXECUTION_BUNDLE
        assert result.schema_hint is not None

    def test_ut_hapi_017_004_validation_result_uses_execution_bundle_field(self):
        """
        UT-HAPI-017-004: ValidationResult exposes validated_execution_bundle,
        not the deprecated validated_container_image.

        Given: ValidationResult dataclass
        When: Constructed with validated_execution_bundle
        Then: validated_execution_bundle accessible, validated_container_image does not exist
        """
        from src.validation.workflow_response_validator import ValidationResult

        result = ValidationResult(
            is_valid=True,
            errors=[],
            validated_execution_bundle=CATALOG_EXECUTION_BUNDLE,
        )

        assert result.validated_execution_bundle == CATALOG_EXECUTION_BUNDLE
        assert not hasattr(result, "validated_container_image"), (
            "validated_container_image should be removed -- use validated_execution_bundle"
        )


# =============================================================================
# PHASE 2: Result Parser - execution_bundle in selected_workflow (UT-HAPI-017-005, 006)
# =============================================================================

def _load_result_parser_isolated():
    """
    Load result_parser.py in isolation, bypassing the src.extensions.__init__.py
    import chain that eagerly loads holmes SDK dependencies.

    This allows unit-testing the parsing logic without the full holmes stack.
    """
    import importlib.util
    import sys
    import types

    saved = {}
    modules_to_mock = [
        "holmes", "holmes.core", "holmes.core.models",
        "holmes.core.investigation", "holmes.config",
        "holmes.core.tools", "holmes.core.tools_utils",
        "holmes.core.tools_utils.tool_executor",
        "holmes.core.tools_utils.toolset_utils",
        "holmes.core.toolset_manager",
        "holmes.plugins", "holmes.plugins.toolsets",
    ]
    for m in modules_to_mock:
        saved[m] = sys.modules.get(m)
        mod = types.ModuleType(m)
        mod.InvestigationResult = type("InvestigationResult", (), {"analysis": ""})
        mod.InvestigateRequest = Mock
        mod.Config = Mock
        sys.modules[m] = mod

    try:
        spec = importlib.util.spec_from_file_location(
            "result_parser_isolated",
            "src/extensions/incident/result_parser.py",
            submodule_search_locations=[],
        )
        module = importlib.util.module_from_spec(spec)

        src_validation = sys.modules.get("src.validation.workflow_response_validator")
        if src_validation is None:
            from src.validation import workflow_response_validator as wfv
            src_validation = wfv
        module.__dict__["src"] = types.ModuleType("src")
        module.__dict__["src"].validation = types.ModuleType("src.validation")
        module.__dict__["src"].validation.workflow_response_validator = src_validation

        spec.loader.exec_module(module)
        return module
    finally:
        for m in modules_to_mock:
            if saved[m] is None:
                sys.modules.pop(m, None)
            else:
                sys.modules[m] = saved[m]


class TestResultParserExecutionBundle:
    """
    Tests that parse_and_validate_investigation_result() writes
    "execution_bundle" (not "container_image") into selected_workflow dict.

    Authority: BR-HAPI-196 (Execution Bundle Consistency), DD-WORKFLOW-017
    Test Plan: docs/testing/DD-WORKFLOW-017/hapi_execution_bundle_test_plan_v1.0.md

    NOTE: The result_parser module is loaded in isolation (via importlib)
    to bypass the eager src.extensions.__init__.py -> holmes SDK import chain.
    This is acceptable for unit tests that only test parsing logic.
    """

    @pytest.fixture
    def result_parser(self):
        """Load result_parser module in isolation."""
        return _load_result_parser_isolated()

    @pytest.fixture
    def make_investigation(self):
        """Factory to create a mock InvestigationResult with a given analysis string."""
        def _make(analysis_text: str):
            inv = Mock()
            inv.analysis = analysis_text
            return inv
        return _make

    def test_ut_hapi_017_005_result_parser_writes_execution_bundle(
        self,
        mock_ds_client,
        mock_workflow_with_execution_bundle,
        make_investigation,
        result_parser,
    ):
        """
        UT-HAPI-017-005: Result parser writes execution_bundle into selected_workflow.

        Given: LLM investigation result with a selected_workflow
        When: parse_and_validate_investigation_result() is called
        Then: selected_workflow dict contains "execution_bundle" key (not "container_image")
              with the catalog-validated value
        """
        mock_ds_client.get_workflow_by_id.return_value = mock_workflow_with_execution_bundle

        investigation_json = json.dumps({
            "root_cause_analysis": {
                "summary": "OOM detected in pod app-1",
                "severity": "critical",
                "contributing_factors": ["memory leak"],
                "affectedResource": "pod/app-1",
            },
            "selected_workflow": {
                "workflow_id": "test-wf-001",
                "confidence": 0.95,
                "parameters": {"NAMESPACE": "default"},
            },
        })
        investigation = make_investigation(f"```json\n{investigation_json}\n```")

        request_data = {"incident_id": "inc-001"}
        result_data, validation_result = result_parser.parse_and_validate_investigation_result(
            investigation, request_data, mock_ds_client
        )

        selected = result_data.get("selected_workflow", {})
        assert "execution_bundle" in selected, (
            "selected_workflow must contain 'execution_bundle' key"
        )
        assert "container_image" not in selected, (
            "selected_workflow must NOT contain deprecated 'container_image' key"
        )
        assert selected["execution_bundle"] == CATALOG_EXECUTION_BUNDLE

    def test_ut_hapi_017_006_alternatives_use_execution_bundle_key(
        self,
        mock_ds_client,
        mock_workflow_with_execution_bundle,
        make_investigation,
        result_parser,
    ):
        """
        UT-HAPI-017-006: Alternative workflows use execution_bundle key.

        Given: LLM investigation result includes alternative_workflows
        When: parse_and_validate_investigation_result() is called
        Then: Each alternative has "execution_bundle" key, not "container_image"
        """
        mock_ds_client.get_workflow_by_id.return_value = mock_workflow_with_execution_bundle

        alt_bundle = (
            "quay.io/kubernaut/bundles/scale@sha256:"
            "bbbbbb1234567890bbbbbb1234567890bbbbbb1234567890bbbbbb1234567890"
        )
        investigation_json = json.dumps({
            "root_cause_analysis": {
                "summary": "OOM detected",
                "severity": "critical",
                "contributing_factors": ["memory leak"],
                "affectedResource": "pod/app-1",
            },
            "selected_workflow": {
                "workflow_id": "test-wf-001",
                "confidence": 0.95,
                "parameters": {"NAMESPACE": "default"},
            },
            "alternative_workflows": [
                {
                    "workflow_id": "test-wf-002",
                    "execution_bundle": alt_bundle,
                    "confidence": 0.7,
                    "rationale": "Alternative scale approach",
                },
            ],
        })
        investigation = make_investigation(f"```json\n{investigation_json}\n```")

        request_data = {"incident_id": "inc-001"}
        result_data, _ = result_parser.parse_and_validate_investigation_result(
            investigation, request_data, mock_ds_client
        )

        alternatives = result_data.get("alternative_workflows", [])
        assert len(alternatives) > 0, "Should have at least one alternative"
        for alt in alternatives:
            assert "execution_bundle" in alt, (
                f"Alternative must use 'execution_bundle' key, got keys: {list(alt.keys())}"
            )
            assert "container_image" not in alt, (
                "Alternative must NOT use deprecated 'container_image' key"
            )


# =============================================================================
# PHASE 3: Discovery Tools - execution_bundle in responses (UT-HAPI-017-007, 008)
# =============================================================================

class TestDiscoveryToolsExecutionBundle:
    """
    Tests that discovery tools expose execution_bundle (not container_image/containerImage)
    in their LLM-facing tool output.

    Authority: BR-HAPI-017-001 (Three-Step Discovery), DD-WORKFLOW-017
    Test Plan: docs/testing/DD-WORKFLOW-017/hapi_execution_bundle_test_plan_v1.0.md
    """

    def test_ut_hapi_017_007_list_workflows_includes_execution_bundle(self):
        """
        UT-HAPI-017-007: ListWorkflows (Step 2) response includes execution_bundle.

        Given: DS returns WorkflowDiscoveryResponse with execution_bundle field
        When: ListWorkflowsTool.invoke() is called
        Then: Tool output string contains "execution_bundle", not "containerImage"/"container_image"
        """
        from src.toolsets.workflow_discovery import ListWorkflowsTool

        tool = ListWorkflowsTool(
            data_storage_url="http://mock:8080",
            remediation_id="rem-test-001",
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
            session_state={"detected_labels": {}},
        )

        mock_response = Mock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            "actionType": "RestartPod",
            "workflows": [
                {
                    "workflowId": "wf-001",
                    "workflowName": "restart-pod-v1",
                    "name": "restart-pod-v1",
                    "description": {"what": "Restarts a pod", "whenToUse": "OOMKilled"},
                    "version": "1.0.0",
                    "schemaImage": (
                        "quay.io/kubernaut/schemas/restart@sha256:"
                        "aaa111bbb222ccc333ddd444eee555fff666aaa111bbb222ccc333ddd444eee555"
                    ),
                    "executionBundle": CATALOG_EXECUTION_BUNDLE,
                    "executionEngine": "tekton",
                },
            ],
            "pagination": {"totalCount": 1, "offset": 0, "limit": 10, "hasMore": False},
        }

        with patch('requests.get', return_value=mock_response):
            result = tool.invoke(params={"action_type": "RestartPod"})

        output = result.data if hasattr(result, 'data') else str(result)
        assert "execution_bundle" in output.lower() or "executionBundle" in output
        assert "containerImage" not in output
        assert "container_image" not in output

    def test_ut_hapi_017_008_get_workflow_includes_execution_bundle(self):
        """
        UT-HAPI-017-008: GetWorkflow (Step 3) response includes execution_bundle and schema_image.

        Given: DS returns full RemediationWorkflow with execution_bundle and schema_image
        When: GetWorkflowTool.invoke() is called
        Then: Tool output string contains "execution_bundle" and "schema_image",
              not "container_image" or "container_digest"
        """
        from src.toolsets.workflow_discovery import GetWorkflowTool

        tool = GetWorkflowTool(
            data_storage_url="http://mock:8080",
            remediation_id="rem-test-001",
            severity="critical",
            component="pod",
            environment="production",
            priority="P0",
            session_state={"detected_labels": {}},
        )

        mock_response = Mock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            "workflow_id": "uuid-001",
            "workflow_name": "restart-pod-v1",
            "version": "1.0.0",
            "schema_image": (
                "quay.io/kubernaut/schemas/restart@sha256:"
                "aaa111bbb222ccc333ddd444eee555fff666aaa111bbb222ccc333ddd444eee555"
            ),
            "schema_digest": "aaa111bbb222ccc333ddd444eee555fff666aaa111bbb222ccc333ddd444eee555",
            "execution_bundle": CATALOG_EXECUTION_BUNDLE,
            "execution_bundle_digest": (
                "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
            ),
            "execution_engine": "tekton",
            "parameters": {"schema": {"parameters": []}},
            "status": "active",
        }

        with patch('requests.get', return_value=mock_response):
            result = tool.invoke(params={"workflow_id": "uuid-001"})

        output = result.data if hasattr(result, 'data') else str(result)
        assert "execution_bundle" in output.lower() or "executionBundle" in output
        assert "schema_image" in output.lower() or "schemaImage" in output
        assert "container_image" not in output
        assert "container_digest" not in output

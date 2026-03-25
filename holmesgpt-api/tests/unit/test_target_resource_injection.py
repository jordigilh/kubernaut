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
Tests for BR-496 v2: HAPI-Owned Target Resource Identity.

HAPI owns the target resource identity by:
1. Stripping TARGET_RESOURCE_* from workflow schema before the LLM sees it
2. Injecting TARGET_RESOURCE_* from K8s-verified root_owner after LLM responds
3. Constructing affectedResource from root_owner for Go backward compat
4. Rejecting workflows missing canonical params in the validator
5. Setting rca_incomplete when root_owner is missing

Test Plan: docs/tests/496/TEST_PLAN.md

TDD Group 1: Injection Logic (8 tests)
  - UT-HAPI-496-001: TARGET_RESOURCE_* injected from root_owner
  - UT-HAPI-496-002: affectedResource constructed for Go compat
  - UT-HAPI-496-003: rca_incomplete when root_owner missing
  - UT-HAPI-496-004: No injection when no selected_workflow
  - UT-HAPI-496-005: affectedResource populated without workflow
  - UT-HAPI-496-006: Operational params preserved
  - UT-HAPI-496-018: Cluster-scoped resource (kind + name only)
  - UT-HAPI-496-019: LLM-provided affectedResource overwritten
"""

import copy
import json
import unittest
import pytest
from unittest.mock import Mock

from src.extensions.incident.llm_integration import _inject_target_resource
from src.extensions.incident.prompt_builder import create_incident_investigation_prompt
from src.extensions.incident.result_parser import parse_and_validate_investigation_result
from src.validation.workflow_response_validator import WorkflowResponseValidator
from src.toolsets.workflow_discovery import strip_hapi_managed_params


ROOT_OWNER_DEPLOYMENT = {
    "kind": "Deployment",
    "name": "postgres-emptydir",
    "namespace": "demo",
}

ROOT_OWNER_NODE = {
    "kind": "Node",
    "name": "worker-1",
}


def _make_result(selected_workflow=None, rca=None):
    """Build a minimal result dict matching HAPI's post-loop structure."""
    result = {
        "root_cause_analysis": rca if rca is not None else {
            "summary": "OOM detected",
            "severity": "high",
            "contributing_factors": ["memory limit too low"],
        },
    }
    if selected_workflow is not None:
        result["selected_workflow"] = selected_workflow
    else:
        result["selected_workflow"] = None
    return result


class TestInjectTargetResource:
    """TDD Group 1: _inject_target_resource injection logic."""

    def test_ut_hapi_496_001_target_params_injected_from_root_owner(self):
        """UT-HAPI-496-001: TARGET_RESOURCE_* injected into workflow parameters from root_owner."""
        result = _make_result(
            selected_workflow={
                "workflow_id": "oom-recovery-v1",
                "parameters": {"MEMORY_LIMIT_NEW": "512Mi"},
            }
        )
        session_state = {"root_owner": ROOT_OWNER_DEPLOYMENT}

        _inject_target_resource(result, session_state, "rem-001")

        params = result["selected_workflow"]["parameters"]
        assert params["TARGET_RESOURCE_NAME"] == "postgres-emptydir"
        assert params["TARGET_RESOURCE_KIND"] == "Deployment"
        assert params["TARGET_RESOURCE_NAMESPACE"] == "demo"

    def test_ut_hapi_496_002_affected_resource_constructed_for_go_compat(self):
        """UT-HAPI-496-002: affectedResource constructed in RCA from root_owner for Go backward compat."""
        result = _make_result(
            selected_workflow={
                "workflow_id": "oom-recovery-v1",
                "parameters": {},
            }
        )
        session_state = {"root_owner": ROOT_OWNER_DEPLOYMENT}

        _inject_target_resource(result, session_state, "rem-001")

        ar = result["root_cause_analysis"]["affectedResource"]
        assert ar["kind"] == "Deployment"
        assert ar["name"] == "postgres-emptydir"
        assert ar["namespace"] == "demo"

    def test_ut_hapi_496_003_rca_incomplete_when_root_owner_missing(self):
        """UT-HAPI-496-003: rca_incomplete when root_owner missing from session_state."""
        result = _make_result(
            selected_workflow={
                "workflow_id": "oom-recovery-v1",
                "parameters": {"MEMORY_LIMIT_NEW": "512Mi"},
            }
        )
        session_state = {}

        _inject_target_resource(result, session_state, "rem-001")

        assert result["needs_human_review"] is True
        assert result["human_review_reason"] == "rca_incomplete"
        assert "TARGET_RESOURCE_NAME" not in result["selected_workflow"]["parameters"]
        assert "affectedResource" not in result["root_cause_analysis"]

    def test_ut_hapi_496_004_no_injection_when_no_workflow(self):
        """UT-HAPI-496-004: No TARGET_RESOURCE_* injection when no selected_workflow."""
        result = _make_result(selected_workflow=None)
        session_state = {"root_owner": ROOT_OWNER_DEPLOYMENT}

        _inject_target_resource(result, session_state, "rem-001")

        assert result["selected_workflow"] is None
        assert "needs_human_review" not in result

    def test_ut_hapi_496_005_affected_resource_populated_without_workflow(self):
        """UT-HAPI-496-005: affectedResource populated in RCA even when no workflow selected."""
        result = _make_result(selected_workflow=None)
        session_state = {"root_owner": ROOT_OWNER_DEPLOYMENT}

        _inject_target_resource(result, session_state, "rem-001")

        ar = result["root_cause_analysis"]["affectedResource"]
        assert ar["kind"] == "Deployment"
        assert ar["name"] == "postgres-emptydir"
        assert ar["namespace"] == "demo"

    def test_ut_hapi_496_006_operational_params_preserved(self):
        """UT-HAPI-496-006: LLM-provided operational params preserved alongside canonical."""
        result = _make_result(
            selected_workflow={
                "workflow_id": "oom-recovery-v1",
                "parameters": {
                    "MEMORY_LIMIT_NEW": "512Mi",
                    "REPLICA_COUNT": "3",
                },
            }
        )
        session_state = {"root_owner": ROOT_OWNER_DEPLOYMENT}

        _inject_target_resource(result, session_state, "rem-001")

        params = result["selected_workflow"]["parameters"]
        assert params["MEMORY_LIMIT_NEW"] == "512Mi"
        assert params["REPLICA_COUNT"] == "3"
        assert params["TARGET_RESOURCE_NAME"] == "postgres-emptydir"
        assert params["TARGET_RESOURCE_KIND"] == "Deployment"
        assert params["TARGET_RESOURCE_NAMESPACE"] == "demo"
        assert len(params) == 5

    def test_ut_hapi_496_018_cluster_scoped_resource(self):
        """UT-HAPI-496-018: Cluster-scoped resource — kind + name only, no namespace."""
        result = _make_result(
            selected_workflow={
                "workflow_id": "node-drain-reboot",
                "parameters": {"DRAIN_TIMEOUT": "300"},
            }
        )
        session_state = {"root_owner": ROOT_OWNER_NODE}

        _inject_target_resource(result, session_state, "rem-002")

        params = result["selected_workflow"]["parameters"]
        assert params["TARGET_RESOURCE_NAME"] == "worker-1"
        assert params["TARGET_RESOURCE_KIND"] == "Node"
        assert "TARGET_RESOURCE_NAMESPACE" not in params

        ar = result["root_cause_analysis"]["affectedResource"]
        assert ar["kind"] == "Node"
        assert ar["name"] == "worker-1"
        assert "namespace" not in ar

    def test_ut_hapi_496_019_llm_provided_affected_resource_overwritten(self):
        """UT-HAPI-496-019: LLM-provided affectedResource unconditionally overwritten by root_owner."""
        result = _make_result(
            selected_workflow={
                "workflow_id": "oom-recovery-v1",
                "parameters": {},
            },
            rca={
                "summary": "OOM detected",
                "severity": "high",
                "contributing_factors": [],
                "affectedResource": {
                    "kind": "Pod",
                    "name": "api-xyz-123",
                    "namespace": "prod",
                },
            },
        )
        session_state = {
            "root_owner": {
                "kind": "Deployment",
                "name": "api",
                "namespace": "prod",
            }
        }

        _inject_target_resource(result, session_state, "rem-003")

        ar = result["root_cause_analysis"]["affectedResource"]
        assert ar["kind"] == "Deployment"
        assert ar["name"] == "api"
        assert ar["namespace"] == "prod"
        assert "needs_human_review" not in result


# ========================================
# TDD Group 2: Schema Validation
# ========================================

def _make_mock_workflow(param_names, all_required=True):
    """Build a mock workflow with given parameter names for validator tests."""
    workflow = Mock()
    workflow.workflow_id = "oom-recovery-v1"
    workflow.execution_bundle = "ghcr.io/kubernaut/oom-recovery:v1.0.0"
    workflow.action_type = None
    params = []
    for name in param_names:
        params.append({
            "name": name,
            "type": "string",
            "required": all_required,
            "description": f"Parameter {name}",
        })
    workflow.parameters = {"schema": {"parameters": params}}
    return workflow


def _make_validator(workflow):
    """Build a WorkflowResponseValidator with a mock DS client returning the given workflow."""
    ds_client = Mock()
    ds_client.get_workflow_by_id.return_value = workflow
    return WorkflowResponseValidator(ds_client)


class TestCanonicalParamValidation:
    """TDD Group 2: Validator Step 0 (canonical param check) and Step 3 (HAPI_MANAGED_PARAMS skip)."""

    def test_ut_hapi_496_007_passes_missing_target_resource_name(self):
        """UT-HAPI-496-007: Workflow missing TARGET_RESOURCE_NAME passes (#524 relaxed)."""
        workflow = _make_mock_workflow([
            "TARGET_RESOURCE_KIND",
            "TARGET_RESOURCE_NAMESPACE",
            "MEMORY_LIMIT_NEW",
        ])
        validator = _make_validator(workflow)

        result = validator.validate("oom-recovery-v1", None, {"MEMORY_LIMIT_NEW": "512Mi"})

        assert result.is_valid is True

    def test_ut_hapi_496_008_passes_missing_target_resource_kind(self):
        """UT-HAPI-496-008: Workflow missing TARGET_RESOURCE_KIND passes (#524 relaxed)."""
        workflow = _make_mock_workflow([
            "TARGET_RESOURCE_NAME",
            "TARGET_RESOURCE_NAMESPACE",
            "MEMORY_LIMIT_NEW",
        ])
        validator = _make_validator(workflow)

        result = validator.validate("oom-recovery-v1", None, {"MEMORY_LIMIT_NEW": "512Mi"})

        assert result.is_valid is True

    def test_ut_hapi_496_009_passes_missing_target_resource_namespace(self):
        """UT-HAPI-496-009: Workflow missing TARGET_RESOURCE_NAMESPACE passes (#524 relaxed)."""
        workflow = _make_mock_workflow([
            "TARGET_RESOURCE_NAME",
            "TARGET_RESOURCE_KIND",
            "MEMORY_LIMIT_NEW",
        ])
        validator = _make_validator(workflow)

        result = validator.validate("oom-recovery-v1", None, {"MEMORY_LIMIT_NEW": "512Mi"})

        assert result.is_valid is True

    def test_ut_hapi_496_010_passes_with_all_canonical_params(self):
        """UT-HAPI-496-010: Workflow declaring all 3 canonical params passes Step 0."""
        workflow = _make_mock_workflow([
            "TARGET_RESOURCE_NAME",
            "TARGET_RESOURCE_KIND",
            "TARGET_RESOURCE_NAMESPACE",
            "MEMORY_LIMIT_NEW",
        ])
        validator = _make_validator(workflow)

        result = validator.validate(
            "oom-recovery-v1", None, {"MEMORY_LIMIT_NEW": "512Mi"}
        )

        assert result.is_valid is True

    def test_ut_hapi_496_011_required_check_skipped_for_hapi_managed(self):
        """UT-HAPI-496-011: Required-check skipped for HAPI_MANAGED_PARAMS even when absent from LLM response."""
        workflow = _make_mock_workflow([
            "TARGET_RESOURCE_NAME",
            "TARGET_RESOURCE_KIND",
            "TARGET_RESOURCE_NAMESPACE",
            "MEMORY_LIMIT_NEW",
        ], all_required=True)
        validator = _make_validator(workflow)

        # LLM provides only MEMORY_LIMIT_NEW — canonical params intentionally absent
        result = validator.validate(
            "oom-recovery-v1", None, {"MEMORY_LIMIT_NEW": "512Mi"}
        )

        assert result.is_valid is True
        assert not any("TARGET_RESOURCE" in e for e in result.errors)


# ---------------------------------------------------------------------------
# TDD Group 3 — Schema Stripping (UT-HAPI-496-012 to 014, 020)
# ---------------------------------------------------------------------------

def _make_ds_workflow_response(param_names):
    """Build a DataStorage-style workflow dict with parameters.schema.parameters."""
    return {
        "workflow_id": "oom-recovery-v1",
        "workflow_name": "OOM Recovery",
        "parameters": {
            "schema": {
                "parameters": [
                    {"name": n, "type": "string", "required": True, "description": f"Param {n}"}
                    for n in param_names
                ]
            }
        },
    }


class TestSchemaStripping(unittest.TestCase):
    """TDD Group 3: get_workflow strips HAPI-managed params before LLM sees them."""

    def test_ut_hapi_496_012_strips_canonical_params(self):
        """UT-HAPI-496-012: Schema stripping removes canonical params."""
        data = _make_ds_workflow_response([
            "TARGET_RESOURCE_NAME",
            "TARGET_RESOURCE_KIND",
            "TARGET_RESOURCE_NAMESPACE",
            "MEMORY_LIMIT_NEW",
            "REPLICA_COUNT",
        ])

        strip_hapi_managed_params(data)

        remaining = [p["name"] for p in data["parameters"]["schema"]["parameters"]]
        assert remaining == ["MEMORY_LIMIT_NEW", "REPLICA_COUNT"]

    def test_ut_hapi_496_013_preserves_operational_params(self):
        """UT-HAPI-496-013: Schema stripping preserves operational params for legacy workflows."""
        data = _make_ds_workflow_response([
            "MEMORY_LIMIT_NEW",
            "REPLICA_COUNT",
        ])

        strip_hapi_managed_params(data)

        remaining = [p["name"] for p in data["parameters"]["schema"]["parameters"]]
        assert remaining == ["MEMORY_LIMIT_NEW", "REPLICA_COUNT"]

    def test_ut_hapi_496_014_handles_missing_parameters(self):
        """UT-HAPI-496-014: Schema stripping handles missing parameters key."""
        data = {"workflow_id": "no-params-wf", "workflow_name": "No Params"}

        strip_hapi_managed_params(data)

        assert "parameters" not in data

    def test_ut_hapi_496_020_handles_malformed_parameters(self):
        """UT-HAPI-496-020: Schema stripping handles malformed parameters (string instead of dict)."""
        data = {"workflow_id": "bad-wf", "parameters": "this-is-a-string"}

        strip_hapi_managed_params(data)

        assert data["parameters"] == "this-is-a-string"


# ---------------------------------------------------------------------------
# TDD Group 4 — Prompt & Parser (UT-HAPI-496-015 to 017)
# ---------------------------------------------------------------------------

STANDARD_REQUEST_DATA = {
    "signal_name": "OOMKilled",
    "severity": "critical",
    "resource_namespace": "production",
    "resource_kind": "Pod",
    "resource_name": "memory-eater-7f86bb8877-4hv68",
    "environment": "production",
    "priority": "P1",
    "risk_tolerance": "low",
    "business_category": "revenue-critical",
    "error_message": "Container exceeded memory limit",
    "description": "Pod OOMKilled after memory spike",
    "incident_id": "inc-test-496",
}


class TestPromptAndParser(unittest.TestCase):
    """TDD Group 4: Prompt omits affectedResource; parser does not flag missing affectedResource."""

    def test_ut_hapi_496_015_prompt_omits_affected_resource_instructions(self):
        """UT-HAPI-496-015: Prompt string must NOT contain affectedResource as LLM instruction."""
        prompt = create_incident_investigation_prompt(STANDARD_REQUEST_DATA)
        self.assertNotIn(
            "affectedResource",
            prompt,
            "Prompt still contains 'affectedResource' — HAPI owns this field, LLM must not see it",
        )

    def test_ut_hapi_496_016_prompt_instructs_resource_context_tools(self):
        """UT-HAPI-496-016: Prompt instructs LLM to call resource context tools (#524 renamed)."""
        prompt = create_incident_investigation_prompt(STANDARD_REQUEST_DATA)
        self.assertIn("get_namespaced_resource_context", prompt)
        self.assertIn("get_cluster_resource_context", prompt)
        self.assertIn("root_owner", prompt)

    def test_ut_hapi_496_017_parser_ignores_missing_affected_resource(self):
        """UT-HAPI-496-017: Parser must NOT set rca_incomplete for missing affectedResource."""
        from holmes.core.models import InvestigationResult

        analysis_json = json.dumps({
            "root_cause_analysis": {
                "summary": "OOM detected",
                "severity": "critical",
                "contributing_factors": ["memory spike"],
            },
            "selected_workflow": {
                "workflow_id": "oom-recovery-v1",
                "action_type": "IncreaseMemoryLimits",
                "version": "1.0.0",
                "confidence": 0.9,
                "rationale": "OOM recovery",
                "execution_engine": "tekton",
                "parameters": {"MEMORY_LIMIT_NEW": "512Mi"},
            },
        })
        analysis_text = f"Analysis text\n```json\n{analysis_json}\n```"
        investigation = InvestigationResult(analysis=analysis_text, tool_calls=[])

        result, validation_result = parse_and_validate_investigation_result(
            investigation, STANDARD_REQUEST_DATA, data_storage_client=None,
        )

        assert result.get("human_review_reason") != "rca_incomplete", (
            "Parser flagged rca_incomplete for missing affectedResource — "
            "BR-496 v2: HAPI injects affectedResource post-loop, parser must not check it"
        )
        assert result.get("needs_human_review") is False


# ---------------------------------------------------------------------------
# TDD Group 5 — Enum Cleanup (UT-HAPI-496-021)
# ---------------------------------------------------------------------------


class TestEnumCleanup(unittest.TestCase):
    """TDD Group 5: Defunct HumanReviewReason enum values removed."""

    def test_ut_hapi_496_021_defunct_enum_values_removed(self):
        """UT-HAPI-496-021: AFFECTED_RESOURCE_MISMATCH and UNVERIFIED_TARGET_RESOURCE removed."""
        from src.models.incident_models import HumanReviewReason

        assert not hasattr(HumanReviewReason, "AFFECTED_RESOURCE_MISMATCH"), (
            "AFFECTED_RESOURCE_MISMATCH still exists — should be removed per BR-496 v2"
        )
        assert not hasattr(HumanReviewReason, "UNVERIFIED_TARGET_RESOURCE"), (
            "UNVERIFIED_TARGET_RESOURCE still exists — should be removed per BR-496 v2"
        )
        assert hasattr(HumanReviewReason, "RCA_INCOMPLETE"), (
            "RCA_INCOMPLETE must still exist — used by _inject_target_resource"
        )


# ========================================
# Issue #524: Relaxed Canonical Param Validation (TDD Group 3)
# ========================================


class TestRelaxedCanonicalValidation524:
    """UT-HAPI-524-020 through 023: Validator no longer rejects missing canonical params."""

    def test_ut_hapi_524_020_passes_without_namespace(self):
        """UT-HAPI-524-020: Workflow missing TARGET_RESOURCE_NAMESPACE passes validation."""
        workflow = _make_mock_workflow([
            "TARGET_RESOURCE_NAME",
            "TARGET_RESOURCE_KIND",
            "TAINT_KEY",
        ])
        validator = _make_validator(workflow)

        result = validator.validate("remove-taint-v1", None, {"TAINT_KEY": "maintenance"})

        assert result.is_valid is True, (
            f"Expected is_valid=True for workflow without NAMESPACE, got errors: {result.errors}"
        )

    def test_ut_hapi_524_021_passes_without_any_canonical_params(self):
        """UT-HAPI-524-021: Workflow declaring zero canonical params passes validation."""
        workflow = _make_mock_workflow([
            "CUSTOM_PARAM_A",
            "CUSTOM_PARAM_B",
        ])
        validator = _make_validator(workflow)

        result = validator.validate("custom-wf-v1", None, {"CUSTOM_PARAM_A": "x", "CUSTOM_PARAM_B": "y"})

        assert result.is_valid is True, (
            f"Expected is_valid=True for workflow with no canonical params, got errors: {result.errors}"
        )

    def test_ut_hapi_524_022_passes_with_name_and_kind_only(self):
        """UT-HAPI-524-022: Workflow with NAME + KIND + operational params passes."""
        workflow = _make_mock_workflow([
            "TARGET_RESOURCE_NAME",
            "TARGET_RESOURCE_KIND",
            "TAINT_KEY",
            "NODE_SELECTOR",
        ])
        validator = _make_validator(workflow)

        result = validator.validate(
            "remove-taint-v1", None,
            {"TAINT_KEY": "maintenance", "NODE_SELECTOR": "worker"},
        )

        assert result.is_valid is True

    def test_ut_hapi_524_023_still_skips_required_check_for_managed(self):
        """UT-HAPI-524-023: Required-check still skipped for HAPI_MANAGED_PARAMS."""
        workflow = _make_mock_workflow([
            "TARGET_RESOURCE_NAME",
            "TARGET_RESOURCE_KIND",
            "TARGET_RESOURCE_NAMESPACE",
            "MEMORY_LIMIT_NEW",
        ], all_required=True)
        validator = _make_validator(workflow)

        result = validator.validate(
            "oom-recovery-v1", None, {"MEMORY_LIMIT_NEW": "512Mi"}
        )

        assert result.is_valid is True
        assert not any("TARGET_RESOURCE" in e for e in result.errors)


# ========================================
# Issue #524: Conditional Injection Logic (TDD Group 4)
# ========================================


class TestConditionalInjection524:
    """UT-HAPI-524-030 through 035: Injection respects resource_scope and workflow schema."""

    def test_ut_hapi_524_030_skips_namespace_for_cluster_scope(self):
        """UT-HAPI-524-030: Injection skips NAMESPACE when resource_scope is 'cluster'."""
        result = _make_result(
            selected_workflow={
                "workflow_id": "remove-taint-v1",
                "action_type": "RemoveTaint",
                "parameters": {"TAINT_KEY": "maintenance"},
            }
        )
        session_state = {
            "root_owner": {"kind": "Node", "name": "worker-3"},
            "resource_scope": "cluster",
        }

        _inject_target_resource(result, session_state, "rem-524-030")

        params = result["selected_workflow"]["parameters"]
        assert params["TARGET_RESOURCE_NAME"] == "worker-3"
        assert params["TARGET_RESOURCE_KIND"] == "Node"
        assert "TARGET_RESOURCE_NAMESPACE" not in params

    def test_ut_hapi_524_031_skips_namespace_when_schema_omits_it(self):
        """UT-HAPI-524-031: Injection skips NAMESPACE when workflow schema doesn't declare it."""
        result = _make_result(
            selected_workflow={
                "workflow_id": "remove-taint-v1",
                "action_type": "RemoveTaint",
                "parameters": {"TAINT_KEY": "maintenance"},
            }
        )
        session_state = {
            "root_owner": {"kind": "Deployment", "name": "api", "namespace": "prod"},
            "resource_scope": "namespaced",
            "workflow_schema": [
                {"name": "TARGET_RESOURCE_NAME", "type": "string", "required": True},
                {"name": "TARGET_RESOURCE_KIND", "type": "string", "required": True},
                {"name": "TAINT_KEY", "type": "string", "required": True},
            ],
        }

        _inject_target_resource(result, session_state, "rem-524-031")

        params = result["selected_workflow"]["parameters"]
        assert params["TARGET_RESOURCE_NAME"] == "api"
        assert params["TARGET_RESOURCE_KIND"] == "Deployment"
        assert "TARGET_RESOURCE_NAMESPACE" not in params

    def test_ut_hapi_524_032_all_params_for_namespaced_full_schema(self):
        """UT-HAPI-524-032: Injection populates all 3 for namespaced scope with full schema."""
        result = _make_result(
            selected_workflow={
                "workflow_id": "oom-recovery-v1",
                "parameters": {"MEMORY_LIMIT_NEW": "512Mi"},
            }
        )
        session_state = {
            "root_owner": {"kind": "Deployment", "name": "api", "namespace": "prod"},
            "resource_scope": "namespaced",
            "workflow_schema": [
                {"name": "TARGET_RESOURCE_NAME", "type": "string", "required": True},
                {"name": "TARGET_RESOURCE_KIND", "type": "string", "required": True},
                {"name": "TARGET_RESOURCE_NAMESPACE", "type": "string", "required": True},
                {"name": "MEMORY_LIMIT_NEW", "type": "string", "required": True},
            ],
        }

        _inject_target_resource(result, session_state, "rem-524-032")

        params = result["selected_workflow"]["parameters"]
        assert params["TARGET_RESOURCE_NAME"] == "api"
        assert params["TARGET_RESOURCE_KIND"] == "Deployment"
        assert params["TARGET_RESOURCE_NAMESPACE"] == "prod"

    def test_ut_hapi_524_033_only_declared_params_injected(self):
        """UT-HAPI-524-033: Injection populates only NAME + KIND when only those are declared."""
        result = _make_result(
            selected_workflow={
                "workflow_id": "node-drain-v1",
                "parameters": {"DRAIN_TIMEOUT": "300"},
            }
        )
        session_state = {
            "root_owner": {"kind": "Node", "name": "worker-3"},
            "resource_scope": "cluster",
            "workflow_schema": [
                {"name": "TARGET_RESOURCE_NAME", "type": "string", "required": True},
                {"name": "TARGET_RESOURCE_KIND", "type": "string", "required": True},
                {"name": "DRAIN_TIMEOUT", "type": "string", "required": True},
            ],
        }

        _inject_target_resource(result, session_state, "rem-524-033")

        params = result["selected_workflow"]["parameters"]
        assert params["TARGET_RESOURCE_NAME"] == "worker-3"
        assert params["TARGET_RESOURCE_KIND"] == "Node"
        assert "TARGET_RESOURCE_NAMESPACE" not in params
        assert params["DRAIN_TIMEOUT"] == "300"

    def test_ut_hapi_524_034_no_canonical_params_injected_when_none_declared(self):
        """UT-HAPI-524-034: No TARGET_RESOURCE_* injected when workflow declares none."""
        result = _make_result(
            selected_workflow={
                "workflow_id": "generic-cleanup-v1",
                "parameters": {"CLEANUP_AGE": "7d"},
            }
        )
        session_state = {
            "root_owner": {"kind": "Deployment", "name": "api", "namespace": "prod"},
            "resource_scope": "namespaced",
            "workflow_schema": [
                {"name": "CLEANUP_AGE", "type": "string", "required": True},
            ],
        }

        _inject_target_resource(result, session_state, "rem-524-034")

        params = result["selected_workflow"]["parameters"]
        assert "TARGET_RESOURCE_NAME" not in params
        assert "TARGET_RESOURCE_KIND" not in params
        assert "TARGET_RESOURCE_NAMESPACE" not in params
        assert params["CLEANUP_AGE"] == "7d"

    def test_ut_hapi_524_035_affected_resource_no_namespace_for_cluster(self):
        """UT-HAPI-524-035: affectedResource omits namespace for cluster-scoped root_owner."""
        result = _make_result(selected_workflow=None)
        session_state = {
            "root_owner": {"kind": "Node", "name": "worker-3"},
            "resource_scope": "cluster",
        }

        _inject_target_resource(result, session_state, "rem-524-035")

        ar = result["root_cause_analysis"]["affectedResource"]
        assert ar["kind"] == "Node"
        assert ar["name"] == "worker-3"
        assert "namespace" not in ar


# ========================================
# Issue #524: Prompt Update (TDD Group 5)
# ========================================


class TestPromptUpdate524(unittest.TestCase):
    """UT-HAPI-524-040 through 043: Prompt documents both tools."""

    def test_ut_hapi_524_040_prompt_contains_namespaced_tool(self):
        """UT-HAPI-524-040: Prompt contains get_namespaced_resource_context."""
        prompt = create_incident_investigation_prompt(STANDARD_REQUEST_DATA)
        self.assertIn("get_namespaced_resource_context", prompt)

    def test_ut_hapi_524_041_prompt_contains_cluster_tool(self):
        """UT-HAPI-524-041: Prompt contains get_cluster_resource_context."""
        prompt = create_incident_investigation_prompt(STANDARD_REQUEST_DATA)
        self.assertIn("get_cluster_resource_context", prompt)

    def test_ut_hapi_524_042_prompt_no_old_tool_name(self):
        """UT-HAPI-524-042: Prompt does not contain bare 'get_resource_context' (old name)."""
        prompt = create_incident_investigation_prompt(STANDARD_REQUEST_DATA)
        import re
        # Find all occurrences of get_resource_context that are NOT part of
        # get_namespaced_resource_context or get_cluster_resource_context
        old_refs = re.findall(
            r'(?<!namespaced_)(?<!cluster_)get_resource_context\b', prompt
        )
        self.assertEqual(len(old_refs), 0,
                         f"Found {len(old_refs)} old tool name 'get_resource_context' in prompt")

    def test_ut_hapi_524_043_prompt_explains_tool_selection(self):
        """UT-HAPI-524-043: Prompt explains when to use namespaced vs cluster tool."""
        prompt = create_incident_investigation_prompt(STANDARD_REQUEST_DATA)
        self.assertIn("Node", prompt)
        self.assertIn("cluster-scoped", prompt.lower() if hasattr(prompt, 'lower') else prompt)


# ========================================
# Issue #524: Post-Selection Validation Guard (TDD Group 6)
# ========================================


class TestScopeMismatchGuard524:
    """UT-HAPI-524-050 through 053: Scope mismatch detection."""

    def test_ut_hapi_524_050_node_workflow_namespaced_tool_triggers_nudge(self):
        """UT-HAPI-524-050: RemoveTaint + namespaced scope → nudge."""
        from src.extensions.incident.llm_integration import _check_scope_mismatch

        result = _make_result(
            selected_workflow={
                "workflow_id": "remove-taint-v1",
                "action_type": "RemoveTaint",
                "parameters": {"TAINT_KEY": "maintenance"},
            }
        )
        session_state = {"resource_scope": "namespaced"}

        nudge = _check_scope_mismatch(result, session_state)

        assert nudge is not None
        assert "get_cluster_resource_context" in nudge

    def test_ut_hapi_524_051_node_workflow_cluster_tool_no_nudge(self):
        """UT-HAPI-524-051: RemoveTaint + cluster scope → no nudge."""
        from src.extensions.incident.llm_integration import _check_scope_mismatch

        result = _make_result(
            selected_workflow={
                "workflow_id": "remove-taint-v1",
                "action_type": "RemoveTaint",
                "parameters": {"TAINT_KEY": "maintenance"},
            }
        )
        session_state = {"resource_scope": "cluster"}

        nudge = _check_scope_mismatch(result, session_state)

        assert nudge is None

    def test_ut_hapi_524_052_deployment_workflow_namespaced_tool_no_nudge(self):
        """UT-HAPI-524-052: RollbackDeployment + namespaced scope → no nudge."""
        from src.extensions.incident.llm_integration import _check_scope_mismatch

        result = _make_result(
            selected_workflow={
                "workflow_id": "rollback-v1",
                "action_type": "RollbackDeployment",
                "parameters": {},
            }
        )
        session_state = {"resource_scope": "namespaced"}

        nudge = _check_scope_mismatch(result, session_state)

        assert nudge is None

    def test_ut_hapi_524_053_missing_scope_no_nudge(self):
        """UT-HAPI-524-053: Missing resource_scope → graceful, no nudge."""
        from src.extensions.incident.llm_integration import _check_scope_mismatch

        result = _make_result(
            selected_workflow={
                "workflow_id": "remove-taint-v1",
                "action_type": "RemoveTaint",
                "parameters": {},
            }
        )
        session_state = {}

        nudge = _check_scope_mismatch(result, session_state)

        assert nudge is None


# ========================================
# Issue #524 GAP fixes: Wiring validation
# ========================================


class TestValidationResultParameterSchema524:
    """UT-HAPI-524-060 through 062: ValidationResult carries workflow parameter schema."""

    def test_ut_hapi_524_060_parameter_schema_populated_on_success(self):
        """UT-HAPI-524-060: Successful validation populates parameter_schema."""
        from src.validation.workflow_response_validator import ValidationResult

        schema = [
            {"name": "TARGET_RESOURCE_NAME", "type": "string", "required": True},
            {"name": "MEMORY_LIMIT", "type": "string", "required": True},
        ]
        vr = ValidationResult(
            is_valid=True,
            errors=[],
            validated_execution_bundle="tekton-bundle:v1",
            parameter_schema=schema,
        )
        assert vr.parameter_schema == schema
        assert vr.is_valid is True

    def test_ut_hapi_524_061_parameter_schema_populated_on_failure(self):
        """UT-HAPI-524-061: Failed validation still carries parameter_schema."""
        from src.validation.workflow_response_validator import ValidationResult

        schema = [
            {"name": "MEMORY_LIMIT", "type": "string", "required": True},
        ]
        vr = ValidationResult(
            is_valid=False,
            errors=["param X not in schema"],
            parameter_schema=schema,
        )
        assert vr.parameter_schema == schema
        assert vr.is_valid is False

    def test_ut_hapi_524_062_parameter_schema_defaults_to_none(self):
        """UT-HAPI-524-062: parameter_schema is None when not provided."""
        from src.validation.workflow_response_validator import ValidationResult

        vr = ValidationResult(is_valid=True)
        assert vr.parameter_schema is None

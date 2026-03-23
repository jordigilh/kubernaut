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
Tests for BR-496: affectedResource mismatch detection.

After the self-correction loop, HAPI compares the LLM's affectedResource
against the K8s-verified root_owner from session_state. Mismatches flag
needs_human_review; missing root_owner (get_resource_context never called)
flags unverified_target_resource.

Business Requirements:
  - BR-496: affectedResource must match root_owner

Test Matrix: 10 tests
  - UT-BR-496-010: Exact match returns True
  - UT-BR-496-011: Case-insensitive match returns True
  - UT-BR-496-012: Name mismatch returns False
  - UT-BR-496-013: Kind mismatch returns False
  - UT-BR-496-014: Namespace mismatch returns False
  - UT-BR-496-015: None llm_ar returns False
  - UT-BR-496-016: Empty dict llm_ar returns False
  - UT-BR-496-020: Mismatch sets needs_human_review
  - UT-BR-496-021: Match does not set needs_human_review
  - UT-BR-496-022: Missing root_owner sets unverified_target_resource
  - UT-BR-496-023: No selected_workflow skips check entirely
  - UT-BR-496-024: Match via affected_resource snake_case alias
"""

import pytest

from src.extensions.incident.llm_integration import (
    _affected_resource_matches,
    _check_affected_resource_mismatch,
)


ROOT_OWNER_DEPLOYMENT = {"kind": "Deployment", "name": "api", "namespace": "production"}
ROOT_OWNER_NODE = {"kind": "Node", "name": "worker-3", "namespace": ""}


class TestAffectedResourceMatches:
    """UT-BR-496-010 through 016: _affected_resource_matches comparison helper."""

    def test_ut_br_496_010_exact_match(self):
        """UT-BR-496-010: Exact kind/name/namespace match returns True."""
        llm_ar = {"kind": "Deployment", "name": "api", "namespace": "production"}
        assert _affected_resource_matches(llm_ar, ROOT_OWNER_DEPLOYMENT) is True

    def test_ut_br_496_011_case_insensitive_match(self):
        """UT-BR-496-011: Case differences still match (LLM may capitalize differently)."""
        llm_ar = {"kind": "deployment", "name": "Api", "namespace": "Production"}
        assert _affected_resource_matches(llm_ar, ROOT_OWNER_DEPLOYMENT) is True

    def test_ut_br_496_012_name_mismatch(self):
        """UT-BR-496-012: Different name returns False."""
        llm_ar = {"kind": "Deployment", "name": "web-frontend", "namespace": "production"}
        assert _affected_resource_matches(llm_ar, ROOT_OWNER_DEPLOYMENT) is False

    def test_ut_br_496_013_kind_mismatch(self):
        """UT-BR-496-013: Different kind returns False."""
        llm_ar = {"kind": "StatefulSet", "name": "api", "namespace": "production"}
        assert _affected_resource_matches(llm_ar, ROOT_OWNER_DEPLOYMENT) is False

    def test_ut_br_496_014_namespace_mismatch(self):
        """UT-BR-496-014: Different namespace returns False."""
        llm_ar = {"kind": "Deployment", "name": "api", "namespace": "staging"}
        assert _affected_resource_matches(llm_ar, ROOT_OWNER_DEPLOYMENT) is False

    def test_ut_br_496_015_none_llm_ar(self):
        """UT-BR-496-015: None affectedResource returns False."""
        assert _affected_resource_matches(None, ROOT_OWNER_DEPLOYMENT) is False

    def test_ut_br_496_016_empty_dict_llm_ar(self):
        """UT-BR-496-016: Empty dict affectedResource returns False."""
        assert _affected_resource_matches({}, ROOT_OWNER_DEPLOYMENT) is False

    def test_ut_br_496_017_cluster_scoped_match(self):
        """UT-BR-496-017: Cluster-scoped resource (empty namespace) matches."""
        llm_ar = {"kind": "Node", "name": "worker-3", "namespace": ""}
        assert _affected_resource_matches(llm_ar, ROOT_OWNER_NODE) is True

    def test_ut_br_496_018_missing_namespace_key(self):
        """UT-BR-496-018: Missing namespace key treated as empty string."""
        llm_ar = {"kind": "Node", "name": "worker-3"}
        assert _affected_resource_matches(llm_ar, ROOT_OWNER_NODE) is True


class TestCheckAffectedResourceMismatch:
    """UT-BR-496-020 through 024: _check_affected_resource_mismatch integration."""

    def test_ut_br_496_020_mismatch_sets_human_review(self):
        """UT-BR-496-020: Mismatch between LLM AR and root_owner flags human review."""
        result = {
            "selected_workflow": {"workflow_id": "oom-recovery-v1"},
            "root_cause_analysis": {
                "affectedResource": {"kind": "Deployment", "name": "WRONG", "namespace": "production"},
            },
        }
        session_state = {"root_owner": ROOT_OWNER_DEPLOYMENT}

        _check_affected_resource_mismatch(result, session_state, "rem-001")

        assert result["needs_human_review"] is True
        assert result["human_review_reason"] == "affectedResource_mismatch"

    def test_ut_br_496_021_match_does_not_set_human_review(self):
        """UT-BR-496-021: Matching AR and root_owner does not flag human review."""
        result = {
            "selected_workflow": {"workflow_id": "oom-recovery-v1"},
            "root_cause_analysis": {
                "affectedResource": {"kind": "Deployment", "name": "api", "namespace": "production"},
            },
        }
        session_state = {"root_owner": ROOT_OWNER_DEPLOYMENT}

        _check_affected_resource_mismatch(result, session_state, "rem-001")

        assert "needs_human_review" not in result
        assert "human_review_reason" not in result

    def test_ut_br_496_022_missing_root_owner_sets_unverified(self):
        """UT-BR-496-022: No root_owner in session_state flags unverified_target_resource."""
        result = {
            "selected_workflow": {"workflow_id": "oom-recovery-v1"},
            "root_cause_analysis": {
                "affectedResource": {"kind": "Deployment", "name": "api", "namespace": "production"},
            },
        }
        session_state = {}

        _check_affected_resource_mismatch(result, session_state, "rem-001")

        assert result["needs_human_review"] is True
        assert result["human_review_reason"] == "unverified_target_resource"

    def test_ut_br_496_023_no_workflow_skips_check(self):
        """UT-BR-496-023: No selected_workflow means no remediation target to check."""
        result = {
            "selected_workflow": None,
            "root_cause_analysis": {
                "affectedResource": {"kind": "Deployment", "name": "WRONG", "namespace": "production"},
            },
        }
        session_state = {"root_owner": ROOT_OWNER_DEPLOYMENT}

        _check_affected_resource_mismatch(result, session_state, "rem-001")

        assert "needs_human_review" not in result

    def test_ut_br_496_024_snake_case_alias_match(self):
        """UT-BR-496-024: affected_resource (snake_case) alias also matches."""
        result = {
            "selected_workflow": {"workflow_id": "oom-recovery-v1"},
            "root_cause_analysis": {
                "affected_resource": {"kind": "Deployment", "name": "api", "namespace": "production"},
            },
        }
        session_state = {"root_owner": ROOT_OWNER_DEPLOYMENT}

        _check_affected_resource_mismatch(result, session_state, "rem-001")

        assert "needs_human_review" not in result

    def test_ut_br_496_025_missing_ar_does_not_trigger_mismatch(self):
        """UT-BR-496-025: Missing affectedResource defers to parser's rca_incomplete, not mismatch."""
        result = {
            "selected_workflow": {"workflow_id": "oom-recovery-v1"},
            "root_cause_analysis": {
                "summary": "OOM detected",
            },
        }
        session_state = {"root_owner": ROOT_OWNER_DEPLOYMENT}

        _check_affected_resource_mismatch(result, session_state, "rem-001")

        assert "needs_human_review" not in result or result.get("human_review_reason") != "affectedResource_mismatch"

    def test_ut_br_496_026_preserves_existing_human_review_reason(self):
        """UT-BR-496-026: Pre-existing human_review_reason not overwritten by mismatch."""
        result = {
            "selected_workflow": {"workflow_id": "oom-recovery-v1"},
            "root_cause_analysis": {
                "affectedResource": {"kind": "Deployment", "name": "WRONG", "namespace": "production"},
            },
            "needs_human_review": True,
            "human_review_reason": "max_retries_exhausted",
        }
        session_state = {"root_owner": ROOT_OWNER_DEPLOYMENT}

        _check_affected_resource_mismatch(result, session_state, "rem-001")

        assert result["needs_human_review"] is True
        assert result["human_review_reason"] == "max_retries_exhausted"

    def test_ut_br_496_027_preserves_existing_reason_unverified(self):
        """UT-BR-496-027: Pre-existing reason preserved even for unverified_target_resource."""
        result = {
            "selected_workflow": {"workflow_id": "oom-recovery-v1"},
            "root_cause_analysis": {},
            "human_review_reason": "rca_incomplete",
        }
        session_state = {}

        _check_affected_resource_mismatch(result, session_state, "rem-001")

        assert result["needs_human_review"] is True
        assert result["human_review_reason"] == "rca_incomplete"

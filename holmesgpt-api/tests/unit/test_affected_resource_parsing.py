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
Unit tests for BR-HAPI-261: LLM-Provided remediationTarget Parsing.

TDD Group 2: remediationTarget parsing from Phase 1 LLM response.
Tests that HAPI correctly extracts and validates the remediationTarget
structure from the LLM's RCA response.

Issue #542: Renamed from affectedResource to remediationTarget.
"""

import pytest
from typing import Dict, Any, Optional


class TestRemediationTargetParsing:
    """G2: remediationTarget parsing (BR-HAPI-261, #542)."""

    def test_ut_hapi_261_001_parse_valid_namespaced_resource(self):
        """UT-HAPI-261-001: Parser extracts valid namespaced remediationTarget."""
        from src.extensions.incident.result_parser import _parse_remediation_target

        rca_data = {
            "summary": "OOM due to memory leak",
            "severity": "critical",
            "remediationTarget": {
                "kind": "Deployment",
                "name": "api-server",
                "namespace": "production",
            },
        }
        result = _parse_remediation_target(rca_data)
        assert result is not None
        assert result["kind"] == "Deployment"
        assert result["name"] == "api-server"
        assert result["namespace"] == "production"

    def test_ut_hapi_261_002_parse_valid_cluster_resource(self):
        """UT-HAPI-261-002: Parser extracts valid cluster remediationTarget."""
        from src.extensions.incident.result_parser import _parse_remediation_target

        rca_data = {
            "summary": "Node disk pressure",
            "severity": "critical",
            "remediationTarget": {
                "kind": "Node",
                "name": "worker-node-1",
            },
        }
        result = _parse_remediation_target(rca_data)
        assert result is not None
        assert result["kind"] == "Node"
        assert result["name"] == "worker-node-1"
        assert "namespace" not in result

    def test_ut_hapi_261_003_reject_missing_required_fields(self):
        """UT-HAPI-261-003: Parser rejects remediationTarget with missing required fields."""
        from src.extensions.incident.result_parser import _parse_remediation_target

        rca_missing_kind = {
            "summary": "Some issue",
            "remediationTarget": {"name": "pod-1", "namespace": "default"},
        }
        assert _parse_remediation_target(rca_missing_kind) is None

        rca_missing_name = {
            "summary": "Some issue",
            "remediationTarget": {"kind": "Pod", "namespace": "default"},
        }
        assert _parse_remediation_target(rca_missing_name) is None

    def test_ut_hapi_261_004_reject_wrong_type(self):
        """UT-HAPI-261-004: Parser rejects remediationTarget with wrong type."""
        from src.extensions.incident.result_parser import _parse_remediation_target

        rca_string = {
            "summary": "Some issue",
            "remediationTarget": "Deployment/api-server",
        }
        assert _parse_remediation_target(rca_string) is None

        rca_list = {
            "summary": "Some issue",
            "remediationTarget": ["Deployment", "api-server"],
        }
        assert _parse_remediation_target(rca_list) is None

    def test_ut_hapi_261_005_returns_none_when_absent(self):
        """UT-HAPI-261-005: Parser returns None when remediationTarget is absent."""
        from src.extensions.incident.result_parser import _parse_remediation_target

        rca_no_resource = {
            "summary": "Root cause identified",
            "severity": "high",
        }
        assert _parse_remediation_target(rca_no_resource) is None

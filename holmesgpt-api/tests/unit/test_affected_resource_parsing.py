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
Unit tests for BR-HAPI-261: LLM-Provided affectedResource Parsing.

TDD Group 2: affectedResource parsing from Phase 1 LLM response.
Tests that HAPI correctly extracts and validates the affectedResource
structure from the LLM's RCA response.
"""

import pytest
from typing import Dict, Any, Optional


class TestAffectedResourceParsing:
    """G2: affectedResource parsing (BR-HAPI-261)."""

    def test_ut_hapi_261_001_parse_valid_namespaced_resource(self):
        """UT-HAPI-261-001: Parser extracts valid namespaced affectedResource.

        BR-HAPI-261: The LLM provides affectedResource as {kind, name, namespace}
        for namespaced resources. HAPI must parse this correctly.
        """
        from src.extensions.incident.result_parser import _parse_affected_resource

        rca_data = {
            "summary": "OOM due to memory leak",
            "severity": "critical",
            "affectedResource": {
                "kind": "Deployment",
                "name": "api-server",
                "namespace": "production",
            },
        }
        result = _parse_affected_resource(rca_data)
        assert result is not None
        assert result["kind"] == "Deployment"
        assert result["name"] == "api-server"
        assert result["namespace"] == "production"

    def test_ut_hapi_261_002_parse_valid_cluster_resource(self):
        """UT-HAPI-261-002: Parser extracts valid cluster affectedResource.

        BR-HAPI-261: For cluster-scoped resources (Node, PV), the LLM provides
        {kind, name} without namespace. HAPI must parse this correctly.
        """
        from src.extensions.incident.result_parser import _parse_affected_resource

        rca_data = {
            "summary": "Node disk pressure",
            "severity": "critical",
            "affectedResource": {
                "kind": "Node",
                "name": "worker-node-1",
            },
        }
        result = _parse_affected_resource(rca_data)
        assert result is not None
        assert result["kind"] == "Node"
        assert result["name"] == "worker-node-1"
        assert "namespace" not in result

    def test_ut_hapi_261_003_reject_missing_required_fields(self):
        """UT-HAPI-261-003: Parser rejects affectedResource with missing required fields.

        BR-HAPI-261: If kind or name is missing, the resource is invalid.
        """
        from src.extensions.incident.result_parser import _parse_affected_resource

        # Missing kind
        rca_missing_kind = {
            "summary": "Some issue",
            "affectedResource": {"name": "pod-1", "namespace": "default"},
        }
        assert _parse_affected_resource(rca_missing_kind) is None

        # Missing name
        rca_missing_name = {
            "summary": "Some issue",
            "affectedResource": {"kind": "Pod", "namespace": "default"},
        }
        assert _parse_affected_resource(rca_missing_name) is None

    def test_ut_hapi_261_004_reject_wrong_type(self):
        """UT-HAPI-261-004: Parser rejects affectedResource with wrong type.

        BR-HAPI-261: If affectedResource is a string or other non-dict type,
        the parser must return None.
        """
        from src.extensions.incident.result_parser import _parse_affected_resource

        rca_string = {
            "summary": "Some issue",
            "affectedResource": "Deployment/api-server",
        }
        assert _parse_affected_resource(rca_string) is None

        rca_list = {
            "summary": "Some issue",
            "affectedResource": ["Deployment", "api-server"],
        }
        assert _parse_affected_resource(rca_list) is None

    def test_ut_hapi_261_005_returns_none_when_absent(self):
        """UT-HAPI-261-005: Parser returns None when affectedResource is absent.

        BR-HAPI-261: When the LLM doesn't provide affectedResource in the RCA,
        the parser returns None so HAPI can retry Phase 1.
        """
        from src.extensions.incident.result_parser import _parse_affected_resource

        rca_no_resource = {
            "summary": "Root cause identified",
            "severity": "high",
        }
        assert _parse_affected_resource(rca_no_resource) is None

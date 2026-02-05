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
Incident Analysis OwnerChain Validation Tests (DD-WORKFLOW-001 v1.7)

Business Requirements: BR-HAPI-002 (Incident analysis response schema)
Design Decision: DD-WORKFLOW-001 v1.7 (OwnerChain validation)
AIAnalysis Request: Dec 2, 2025 (target_in_owner_chain, warnings fields)

Tests validate BUSINESS OUTCOMES:
- target_in_owner_chain correctly identifies when RCA target is in OwnerChain
- warnings[] is populated for OwnerChain validation issues
- warnings[] is populated for low confidence and no workflow scenarios
"""

from unittest.mock import MagicMock


class TestOwnerChainValidation:
    """
    Tests for _is_target_in_owner_chain function

    Business Outcome: DetectedLabels validation ensures labels are
    applicable to the actual affected resource.
    """

    def test_target_matches_source_resource(self):
        """
        Business Outcome: When RCA target is the source resource, validation passes
        """
        from src.extensions.incident import _is_target_in_owner_chain

        rca_target = {"kind": "Deployment", "name": "nginx", "namespace": "default"}
        owner_chain = []  # Empty chain
        request_data = {
            "resource_kind": "Deployment",
            "resource_name": "nginx",
            "resource_namespace": "default"
        }

        result = _is_target_in_owner_chain(rca_target, owner_chain, request_data)
        assert result is True

    def test_target_in_owner_chain(self):
        """
        Business Outcome: When RCA target is in OwnerChain, validation passes
        """
        from src.extensions.incident import _is_target_in_owner_chain

        rca_target = {"kind": "Deployment", "name": "nginx", "namespace": "default"}
        owner_chain = [
            {"kind": "ReplicaSet", "name": "nginx-abc123", "namespace": "default"},
            {"kind": "Deployment", "name": "nginx", "namespace": "default"}
        ]
        request_data = {
            "resource_kind": "Pod",
            "resource_name": "nginx-abc123-xyz789",
            "resource_namespace": "default"
        }

        result = _is_target_in_owner_chain(rca_target, owner_chain, request_data)
        assert result is True

    def test_target_not_in_owner_chain(self):
        """
        Business Outcome: When RCA target is NOT in OwnerChain, validation fails
        DetectedLabels may be from a different scope than the affected resource.
        """
        from src.extensions.incident import _is_target_in_owner_chain

        rca_target = {"kind": "Node", "name": "worker-node-1"}  # Different resource
        owner_chain = [
            {"kind": "ReplicaSet", "name": "nginx-abc123", "namespace": "default"},
            {"kind": "Deployment", "name": "nginx", "namespace": "default"}
        ]
        request_data = {
            "resource_kind": "Pod",
            "resource_name": "nginx-abc123-xyz789",
            "resource_namespace": "default"
        }

        result = _is_target_in_owner_chain(rca_target, owner_chain, request_data)
        assert result is False

    def test_case_insensitive_matching(self):
        """
        Business Outcome: Matching is case-insensitive for robustness
        """
        from src.extensions.incident import _is_target_in_owner_chain

        rca_target = {"kind": "DEPLOYMENT", "name": "NGINX", "namespace": "DEFAULT"}
        owner_chain = [
            {"kind": "deployment", "name": "nginx", "namespace": "default"}
        ]
        request_data = {
            "resource_kind": "Pod",
            "resource_name": "nginx-pod",
            "resource_namespace": "default"
        }

        result = _is_target_in_owner_chain(rca_target, owner_chain, request_data)
        assert result is True

    def test_cluster_scoped_resource(self):
        """
        Business Outcome: Cluster-scoped resources (no namespace) are handled correctly
        """
        from src.extensions.incident import _is_target_in_owner_chain

        rca_target = {"kind": "Node", "name": "worker-node-1"}  # No namespace
        owner_chain = []
        request_data = {
            "resource_kind": "Node",
            "resource_name": "worker-node-1",
            "resource_namespace": ""
        }

        result = _is_target_in_owner_chain(rca_target, owner_chain, request_data)
        assert result is True


class TestParseInvestigationResultWarnings:
    """
    Tests for warnings[] field in _parse_investigation_result

    Business Outcome: AIAnalysis receives transparency on data quality
    for Rego policies and operator notifications.
    """

    def test_warning_when_target_not_in_owner_chain(self):
        """
        Business Outcome: AIAnalysis receives warning when DetectedLabels may not apply
        """
        from src.extensions.incident import _parse_investigation_result
        from holmes.core.models import InvestigationResult

        # Mock investigation with RCA target not in owner chain
        investigation = MagicMock(spec=InvestigationResult)
        investigation.analysis = '''```json
{
    "root_cause_analysis": {
        "summary": "Node resource exhaustion",
        "severity": "critical",
        "affectedResource": {"kind": "Node", "name": "worker-node-1"}
    },
    "selected_workflow": {
        "workflow_id": "wf-001",
        "confidence": 0.85
    }
}
```'''

        request_data = {
            "incident_id": "inc-001",
            "resource_kind": "Pod",
            "resource_name": "nginx-pod",
            "resource_namespace": "default"
        }
        owner_chain = [
            {"kind": "Deployment", "name": "nginx", "namespace": "default"}
        ]

        result = _parse_investigation_result(investigation, request_data, owner_chain)

        assert result["target_in_owner_chain"] is False
        assert len(result["warnings"]) >= 1
        assert any("OwnerChain" in w for w in result["warnings"])

    def test_warning_when_no_workflow_selected(self):
        """
        Business Outcome: AIAnalysis knows when no workflows matched
        """
        from src.extensions.incident import _parse_investigation_result
        from holmes.core.models import InvestigationResult

        investigation = MagicMock(spec=InvestigationResult)
        investigation.analysis = '''```json
{
    "root_cause_analysis": {
        "summary": "Memory leak in application",
        "severity": "high"
    }
}
```'''  # No selected_workflow

        request_data = {"incident_id": "inc-001"}

        result = _parse_investigation_result(investigation, request_data, owner_chain=None)

        # ADR-045 v1.2: selected_workflow field is only included when not None
        # Parser no longer includes null fields (lines 790-791 of result_parser.py)
        assert "selected_workflow" not in result or result.get("selected_workflow") is None
        assert "No workflows matched" in result["warnings"][0]

    def test_warning_when_low_confidence(self):
        """
        Business Outcome: Low confidence workflows are returned without HAPI warnings
        
        BR-HAPI-197: Confidence threshold enforcement is AIAnalysis's responsibility, not HAPI's.
        HAPI returns the workflow with confidence score; AIAnalysis decides approval requirements.
        
        BR-HAPI-212: If selected_workflow exists but affectedResource is missing, 
        HAPI sets human_review_reason = "rca_incomplete" (not "low_confidence").
        """
        from src.extensions.incident import _parse_investigation_result
        from holmes.core.models import InvestigationResult

        investigation = MagicMock(spec=InvestigationResult)
        investigation.analysis = '''```json
{
    "root_cause_analysis": {
        "summary": "Unknown issue",
        "severity": "medium",
        "affectedResource": {"kind": "Pod", "name": "test-pod", "namespace": "default"}
    },
    "selected_workflow": {
        "workflow_id": "wf-generic",
        "confidence": 0.55
    }
}
```'''

        request_data = {"incident_id": "inc-001"}

        result = _parse_investigation_result(investigation, request_data, owner_chain=None)

        # BR-HAPI-197: HAPI returns low confidence workflows without warnings
        # AIAnalysis will evaluate confidence and set approval requirements
        assert result["confidence"] == 0.55
        assert result.get("selected_workflow") is not None
        assert result["selected_workflow"]["workflow_id"] == "wf-generic"
        # No "Low confidence" warnings from HAPI - that's AIAnalysis's job
        assert not any("Low confidence" in w for w in result.get("warnings", []))

    def test_no_warnings_when_all_valid(self):
        """
        Business Outcome: No noise when everything is good
        """
        from src.extensions.incident import _parse_investigation_result
        from holmes.core.models import InvestigationResult

        investigation = MagicMock(spec=InvestigationResult)
        investigation.analysis = '''```json
{
    "root_cause_analysis": {
        "summary": "OOM in nginx deployment",
        "severity": "high",
        "affectedResource": {"kind": "Deployment", "name": "nginx", "namespace": "default"}
    },
    "selected_workflow": {
        "workflow_id": "wf-memory",
        "confidence": 0.92
    }
}
```'''

        request_data = {
            "incident_id": "inc-001",
            "resource_kind": "Pod",
            "resource_name": "nginx-abc123",
            "resource_namespace": "default"
        }
        owner_chain = [
            {"kind": "Deployment", "name": "nginx", "namespace": "default"}
        ]

        result = _parse_investigation_result(investigation, request_data, owner_chain)

        assert result["target_in_owner_chain"] is True
        assert result["confidence"] == 0.92
        assert len(result["warnings"]) == 0


class TestIncidentResponseFields:
    """
    Tests for IncidentResponse model fields

    Business Outcome: Response schema matches AIAnalysis expectations
    """

    def test_response_includes_new_fields(self):
        """
        Business Outcome: IncidentResponse has target_in_owner_chain and warnings
        """
        from src.models.incident_models import IncidentResponse

        response = IncidentResponse(
            incident_id="inc-001",
            analysis="Test analysis",
            root_cause_analysis={"summary": "Test"},
            confidence=0.9,
            timestamp="2025-12-02T00:00:00Z",
            target_in_owner_chain=False,
            warnings=["Test warning"]
        )

        assert response.target_in_owner_chain is False
        assert response.warnings == ["Test warning"]

    def test_response_defaults(self):
        """
        Business Outcome: Default values are correct when not explicitly set
        """
        from src.models.incident_models import IncidentResponse

        response = IncidentResponse(
            incident_id="inc-001",
            analysis="Test analysis",
            root_cause_analysis={"summary": "Test"},
            confidence=0.9,
            timestamp="2025-12-02T00:00:00Z"
        )

        assert response.target_in_owner_chain is True  # Default: True
        assert response.warnings == []  # Default: empty list



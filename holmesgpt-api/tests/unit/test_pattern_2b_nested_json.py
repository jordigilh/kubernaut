"""UT-HAPI-624-001..003: Pattern 2B nested JSON extraction tests.

Issue #624: Pattern 2B regex uses non-greedy {.*?} which truncates nested JSON.
These tests verify balanced brace extraction handles nested objects correctly.
"""


def _build_section_header_text(rca_json, workflow_json=None, confidence=0.85):
    """Build LLM output in section-header format (Pattern 2B) with nested JSON."""
    lines = [
        "Investigation of deployment scaling issue.",
        "",
        "# root_cause_analysis",
        rca_json,
        "",
        "# confidence",
        str(confidence),
    ]
    if workflow_json is not None:
        lines.extend(["", "# selected_workflow", workflow_json])
    else:
        lines.extend(["", "# selected_workflow", "None"])
    return "\n".join(lines)


def _parse(analysis_text, incident_id="test-624"):
    from unittest.mock import MagicMock
    from src.extensions.incident.result_parser import parse_and_validate_investigation_result

    investigation = MagicMock()
    investigation.analysis = analysis_text
    request_data = {
        "incident_id": incident_id,
        "signal_name": "KubeDeploymentReplicasMismatch",
        "severity": "critical",
    }
    result, _validation = parse_and_validate_investigation_result(
        investigation, request_data
    )
    return result


class TestPattern2BNestedJSON:
    """UT-HAPI-624-001..003: Pattern 2B correctly handles nested JSON objects."""

    def test_ut_hapi_624_001_nested_rca_extraction(self):
        """UT-HAPI-624-001: Pattern 2B extracts nested JSON in root_cause_analysis."""
        nested_rca = '{"summary": "Deployment has insufficient replicas", "severity": "critical", "contributing_factors": ["HPA misconfigured"], "remediationTarget": {"kind": "Deployment", "name": "web-app", "namespace": "production"}}'
        analysis = _build_section_header_text(nested_rca)
        result = _parse(analysis)

        rca = result.get("root_cause_analysis", {})
        assert isinstance(rca, dict), f"RCA should be a dict, got {type(rca)}"
        assert rca.get("summary") == "Deployment has insufficient replicas", \
            f"RCA summary mismatch: {rca.get('summary')}"
        target = rca.get("remediationTarget", {})
        assert target.get("kind") == "Deployment", \
            f"Nested remediationTarget.kind should be 'Deployment', got: {target.get('kind')}"
        assert target.get("name") == "web-app", \
            f"Nested remediationTarget.name should be 'web-app', got: {target.get('name')}"

    def test_ut_hapi_624_002_nested_workflow_extraction(self):
        """UT-HAPI-624-002: Pattern 2B extracts nested JSON in selected_workflow."""
        simple_rca = '{"summary": "Pod crash loop", "severity": "high", "contributing_factors": ["OOM"]}'
        nested_workflow = '{"workflow_id": "restart-pod-v1", "confidence": 0.9, "parameters": {"target": {"name": "web-app", "namespace": "prod"}}}'
        analysis = _build_section_header_text(simple_rca, workflow_json=nested_workflow)
        result = _parse(analysis)

        wf = result.get("selected_workflow")
        assert wf is not None, "selected_workflow should not be None"
        assert wf.get("workflow_id") == "restart-pod-v1", \
            f"workflow_id mismatch: {wf.get('workflow_id')}"
        params = wf.get("parameters", {})
        assert isinstance(params.get("target"), dict), \
            f"Nested parameters.target should be a dict, got: {type(params.get('target'))}"
        assert params["target"].get("name") == "web-app", \
            f"Nested target.name should be 'web-app', got: {params['target'].get('name')}"

    def test_ut_hapi_624_003_unbalanced_braces_graceful(self):
        """UT-HAPI-624-003: Unbalanced braces return empty/fallback RCA, not crash."""
        broken_rca = '{"summary": "incomplete object", "nested": {"broken": '
        analysis = _build_section_header_text(broken_rca)
        result = _parse(analysis)
        assert "root_cause_analysis" in result, "Result should always have root_cause_analysis key"

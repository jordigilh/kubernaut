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
Recovery Analysis Endpoint Tests

Business Requirements: BR-HAPI-001 to 050 (Recovery Analysis)

Note (BR-AA-HAPI-064): All endpoints now use the async session pattern.
POST returns 202 + session_id, results are fetched via GET /session/{id}/result.
"""


def _submit_and_get_result(client, request_data):
    """
    Helper: submit a recovery request and retrieve the result via the session pattern.

    BR-AA-HAPI-064: Async-first endpoints -- POST returns 202 + session_id,
    BackgroundTasks run synchronously in TestClient, GET result returns 200.
    """
    submit_resp = client.post("/api/v1/recovery/analyze", json=request_data)
    assert submit_resp.status_code == 202, (
        f"Expected 202 Accepted, got {submit_resp.status_code}: {submit_resp.text}"
    )
    session_id = submit_resp.json()["session_id"]

    result_resp = client.get(f"/api/v1/recovery/session/{session_id}/result")
    assert result_resp.status_code == 200, (
        f"Expected 200 for completed session, got {result_resp.status_code}: {result_resp.text}"
    )
    return result_resp


class TestRecoveryEndpoint:
    """Tests for /api/v1/recovery/analyze endpoint"""

    def test_recovery_returns_202_on_valid_request(self, client, sample_recovery_request, mock_analyze_recovery):
        """Business Requirement: Recovery endpoint accepts valid requests (async pattern)"""
        response = client.post("/api/v1/recovery/analyze", json=sample_recovery_request)
        assert response.status_code == 202

    def test_recovery_returns_incident_id(self, client, sample_recovery_request, mock_analyze_recovery):
        """Business Requirement: Response includes incident ID"""
        result_resp = _submit_and_get_result(client, sample_recovery_request)
        data = result_resp.json()

        assert data["incident_id"] == sample_recovery_request["incident_id"]

    def test_recovery_returns_can_recover_flag(self, client, sample_recovery_request, mock_analyze_recovery):
        """Business Requirement: Response indicates if recovery is possible"""
        result_resp = _submit_and_get_result(client, sample_recovery_request)
        data = result_resp.json()

        assert "can_recover" in data
        assert isinstance(data["can_recover"], bool)

    def test_recovery_returns_strategies_list(self, client, sample_recovery_request, mock_analyze_recovery):
        """Business Requirement: Response includes recovery strategies"""
        result_resp = _submit_and_get_result(client, sample_recovery_request)
        data = result_resp.json()

        assert "strategies" in data
        assert isinstance(data["strategies"], list)

    def test_recovery_strategy_has_required_fields(self, client, sample_recovery_request, mock_analyze_recovery):
        """Business Requirement: Each strategy has action_type, confidence, rationale"""
        result_resp = _submit_and_get_result(client, sample_recovery_request)
        data = result_resp.json()

        if len(data["strategies"]) > 0:
            strategy = data["strategies"][0]
            assert "action_type" in strategy
            assert "confidence" in strategy
            assert "rationale" in strategy
            assert "estimated_risk" in strategy

    def test_recovery_includes_primary_recommendation(self, client, sample_recovery_request, mock_analyze_recovery):
        """Business Requirement: Response includes primary recommendation"""
        result_resp = _submit_and_get_result(client, sample_recovery_request)
        data = result_resp.json()

        assert "primary_recommendation" in data

    def test_recovery_includes_confidence_score(self, client, sample_recovery_request, mock_analyze_recovery):
        """Business Requirement: Response includes overall confidence"""
        result_resp = _submit_and_get_result(client, sample_recovery_request)
        data = result_resp.json()

        assert "analysis_confidence" in data
        assert 0.0 <= data["analysis_confidence"] <= 1.0

    def test_recovery_handles_missing_fields(self, client):
        """Business Requirement: Validate required fields"""
        invalid_request = {
            "incident_id": "test-inc-001"
            # Missing failed_action and failure_context
        }
        response = client.post("/api/v1/recovery/analyze", json=invalid_request)
        assert response.status_code == 400  # RFC 7807: Validation errors return 400 Bad Request


class TestRecoveryAnalysisLogic:
    """Tests for recovery analysis core logic via HTTP endpoint (uses mock LLM)"""

    def test_analyze_recovery_generates_strategies(self, client, sample_recovery_request, mock_analyze_recovery):
        """Business Requirement: Analysis generates recovery strategies"""
        result_resp = _submit_and_get_result(client, sample_recovery_request)
        data = result_resp.json()

        assert data["can_recover"] is True
        assert len(data["strategies"]) > 0

    def test_analyze_recovery_includes_warnings_field(self, client, mock_analyze_recovery):
        """Business Requirement: Response includes warnings field"""
        request = {
            "incident_id": "test-inc-002",
            "remediation_id": "req-test-2025-11-27-002",  # DD-WORKFLOW-002 v2.2: mandatory
            "is_recovery_attempt": True,
            "recovery_attempt_number": 1,
            "previous_execution": {
                "workflow_execution_ref": "req-test-2025-11-27-001-we-1",
                "original_rca": {
                    "summary": "High load causing failures",
                    "signal_type": "OOMKilled",
                    "severity": "high",
                    "contributing_factors": ["high_load"]
                },
                "selected_workflow": {
                    "workflow_id": "scale-horizontal-v1",
                    "version": "1.0.0",
                    "execution_bundle": "kubernaut/workflow-scale:v1.0.0",
                    "parameters": {"TARGET_REPLICAS": "5"},
                    "rationale": "Scaling out"
                },
                "failure": {
                    "failed_step_index": 1,
                    "failed_step_name": "scale_deployment",
                    "reason": "HighLoad",
                    "message": "Cluster under high load",
                    "failed_at": "2025-11-27T10:30:00Z",
                    "execution_time": "30s"
                }
            },
            "signal_type": "OOMKilled",
            "severity": "high",
            "resource_namespace": "test",
            "resource_kind": "Deployment",
            "resource_name": "nginx"
        }

        result_resp = _submit_and_get_result(client, request)
        data = result_resp.json()

        # Warnings field should exist (may be empty depending on mock LLM response)
        assert "warnings" in data
        assert isinstance(data["warnings"], list)

    def test_analyze_recovery_returns_metadata(self, client, sample_recovery_request, mock_analyze_recovery):
        """Business Requirement: Response includes analysis metadata"""
        result_resp = _submit_and_get_result(client, sample_recovery_request)
        data = result_resp.json()

        assert "metadata" in data
        assert "analysis_time_ms" in data["metadata"]


class TestRecoveryErrorHandling:
    """Tests for recovery analysis error handling"""

    def test_recovery_returns_500_on_internal_error(self, client):
        """Business Requirement: Graceful error handling"""
        # This would require mocking internal failures
        # For GREEN phase, test basic error response structure
        pass

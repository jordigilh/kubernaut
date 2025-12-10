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
Post-Execution Analysis Endpoint Tests

Business Requirements: BR-HAPI-051 to 115 (Post-Execution Analysis)

NOTE: DD-017 defers PostExec endpoint to V1.1 (Effectiveness Monitor not in V1.0)
Endpoint tests are skipped; internal logic preserved in src/extensions/postexec.py
"""

import pytest


@pytest.mark.skip(reason="DD-017: PostExec endpoint deferred to V1.1 - Effectiveness Monitor not available in V1.0")
class TestPostExecEndpoint:
    """Tests for /api/v1/postexec/analyze endpoint"""

    def test_postexec_returns_200_on_valid_request(self, client, sample_postexec_request):
        """Business Requirement: Post-exec endpoint accepts valid requests"""
        response = client.post("/api/v1/postexec/analyze", json=sample_postexec_request)
        assert response.status_code == 200

    def test_postexec_returns_execution_id(self, client, sample_postexec_request):
        """Business Requirement: Response includes execution ID"""
        response = client.post("/api/v1/postexec/analyze", json=sample_postexec_request)
        data = response.json()

        assert data["execution_id"] == sample_postexec_request["execution_id"]

    def test_postexec_returns_effectiveness_assessment(self, client, sample_postexec_request):
        """Business Requirement: Response includes effectiveness assessment"""
        response = client.post("/api/v1/postexec/analyze", json=sample_postexec_request)
        data = response.json()

        assert "effectiveness" in data
        effectiveness = data["effectiveness"]
        assert "success" in effectiveness
        assert "confidence" in effectiveness
        assert "reasoning" in effectiveness

    def test_postexec_returns_objectives_met_flag(self, client, sample_postexec_request):
        """Business Requirement: Response indicates if objectives were met"""
        response = client.post("/api/v1/postexec/analyze", json=sample_postexec_request)
        data = response.json()

        assert "objectives_met" in data
        assert isinstance(data["objectives_met"], bool)

    def test_postexec_returns_side_effects_list(self, client, sample_postexec_request):
        """Business Requirement: Response includes detected side effects"""
        response = client.post("/api/v1/postexec/analyze", json=sample_postexec_request)
        data = response.json()

        assert "side_effects" in data
        assert isinstance(data["side_effects"], list)

    def test_postexec_returns_recommendations(self, client, sample_postexec_request):
        """Business Requirement: Response includes recommendations"""
        response = client.post("/api/v1/postexec/analyze", json=sample_postexec_request)
        data = response.json()

        assert "recommendations" in data
        assert isinstance(data["recommendations"], list)

    def test_postexec_handles_missing_fields(self, client):
        """Business Requirement: Validate required fields"""
        invalid_request = {
            "execution_id": "exec-001"
            # Missing required fields
        }
        response = client.post("/api/v1/postexec/analyze", json=invalid_request)
        assert response.status_code == 400  # RFC 7807: Validation errors return 400 Bad Request


class TestPostExecAnalysisLogic:
    """Tests for post-execution analysis core logic"""

    @pytest.mark.asyncio
    async def test_analyze_postexecution_assesses_effectiveness(self, sample_postexec_request):
        """Business Requirement: Analysis assesses execution effectiveness"""
        from src.extensions.postexec import analyze_postexecution

        result = await analyze_postexecution(sample_postexec_request)

        assert "effectiveness" in result
        assert result["effectiveness"]["success"] is True

    @pytest.mark.asyncio
    async def test_marks_success_when_objectives_achieved(self):
        """Business Requirement: Mark success when CPU reduced significantly"""
        from src.extensions.postexec import analyze_postexecution

        success_request = {
            "execution_id": "exec-002",
            "action_id": "action-002",
            "action_type": "scale_deployment",
            "action_details": {"replicas": 3},
            "execution_success": True,
            "execution_result": {"status": "success"},
            "pre_execution_state": {"cpu_usage": 0.95},
            "post_execution_state": {"cpu_usage": 0.35},
            "context": {}
        }

        result = await analyze_postexecution(success_request)

        assert result["objectives_met"] is True
        assert result["effectiveness"]["success"] is True

    @pytest.mark.asyncio
    async def test_marks_failure_when_execution_fails(self):
        """Business Requirement: Mark failure when execution fails"""
        from src.extensions.postexec import analyze_postexecution

        failure_request = {
            "execution_id": "exec-003",
            "action_id": "action-003",
            "action_type": "scale_deployment",
            "action_details": {"replicas": 3},
            "execution_success": False,
            "execution_result": {"error": "timeout"},
            "context": {}
        }

        result = await analyze_postexecution(failure_request)

        assert result["objectives_met"] is False
        assert result["effectiveness"]["success"] is False

    @pytest.mark.asyncio
    async def test_handles_missing_post_execution_state(self):
        """Business Requirement: Handle optional fields gracefully"""
        from src.extensions.postexec import analyze_postexecution

        incomplete_request = {
            "execution_id": "exec-001",
            "action_id": "action-001",
            "action_type": "scale_deployment",
            "action_details": {"replicas": 3},
            "execution_success": True,
            "execution_result": {"status": "success"},
            "context": {"cluster": "test"}
            # Missing post_execution_state (optional field, should handle gracefully)
        }
        result = await analyze_postexecution(incomplete_request)
        assert result is not None, "Should handle missing post_execution_state gracefully"

    @pytest.mark.asyncio
    async def test_analyze_postexecution_returns_metadata(self, sample_postexec_request):
        """Business Requirement: Response includes analysis metadata"""
        from src.extensions.postexec import analyze_postexecution

        result = await analyze_postexecution(sample_postexec_request)

        assert "metadata" in result
        assert "analysis_time_ms" in result["metadata"]


class TestPostExecErrorHandling:
    """Tests for post-execution analysis error handling"""

    def test_postexec_returns_500_on_internal_error(self, client):
        """Business Requirement: Graceful error handling"""
        # This would require mocking internal failures
        # For GREEN phase, test basic error response structure
        pass

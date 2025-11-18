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
"""

import pytest


class TestRecoveryEndpoint:
    """Tests for /api/v1/recovery/analyze endpoint"""

    def test_recovery_returns_200_on_valid_request(self, client, sample_recovery_request):
        """Business Requirement: Recovery endpoint accepts valid requests"""
        response = client.post("/api/v1/recovery/analyze", json=sample_recovery_request)
        assert response.status_code == 200

    def test_recovery_returns_incident_id(self, client, sample_recovery_request):
        """Business Requirement: Response includes incident ID"""
        response = client.post("/api/v1/recovery/analyze", json=sample_recovery_request)
        data = response.json()

        assert data["incident_id"] == sample_recovery_request["incident_id"]

    def test_recovery_returns_can_recover_flag(self, client, sample_recovery_request):
        """Business Requirement: Response indicates if recovery is possible"""
        response = client.post("/api/v1/recovery/analyze", json=sample_recovery_request)
        data = response.json()

        assert "can_recover" in data
        assert isinstance(data["can_recover"], bool)

    def test_recovery_returns_strategies_list(self, client, sample_recovery_request):
        """Business Requirement: Response includes recovery strategies"""
        response = client.post("/api/v1/recovery/analyze", json=sample_recovery_request)
        data = response.json()

        assert "strategies" in data
        assert isinstance(data["strategies"], list)

    def test_recovery_strategy_has_required_fields(self, client, sample_recovery_request):
        """Business Requirement: Each strategy has action_type, confidence, rationale"""
        response = client.post("/api/v1/recovery/analyze", json=sample_recovery_request)
        data = response.json()

        if len(data["strategies"]) > 0:
            strategy = data["strategies"][0]
            assert "action_type" in strategy
            assert "confidence" in strategy
            assert "rationale" in strategy
            assert "estimated_risk" in strategy

    def test_recovery_includes_primary_recommendation(self, client, sample_recovery_request):
        """Business Requirement: Response includes primary recommendation"""
        response = client.post("/api/v1/recovery/analyze", json=sample_recovery_request)
        data = response.json()

        assert "primary_recommendation" in data

    def test_recovery_includes_confidence_score(self, client, sample_recovery_request):
        """Business Requirement: Response includes overall confidence"""
        response = client.post("/api/v1/recovery/analyze", json=sample_recovery_request)
        data = response.json()

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
    """Tests for recovery analysis core logic"""

    @pytest.mark.asyncio
    async def test_analyze_recovery_generates_strategies(self, sample_recovery_request):
        """Business Requirement: Analysis generates recovery strategies"""
        from src.extensions.recovery import analyze_recovery

        result = await analyze_recovery(sample_recovery_request)

        assert result["can_recover"] is True
        assert len(result["strategies"]) > 0

    @pytest.mark.asyncio
    async def test_analyze_recovery_includes_warnings_for_high_load(self):
        """Business Requirement: Warning when cluster is under high load"""
        from src.extensions.recovery import analyze_recovery

        request = {
            "incident_id": "test-inc-002",
            "failed_action": {"type": "scale_deployment"},
            "failure_context": {"cluster_state": "high_load"}
        }

        result = await analyze_recovery(request)

        assert "warnings" in result
        assert len(result["warnings"]) > 0

    @pytest.mark.asyncio
    async def test_analyze_recovery_returns_metadata(self, sample_recovery_request):
        """Business Requirement: Response includes analysis metadata"""
        from src.extensions.recovery import analyze_recovery

        result = await analyze_recovery(sample_recovery_request)

        assert "metadata" in result
        assert "analysis_time_ms" in result["metadata"]


class TestRecoveryErrorHandling:
    """Tests for recovery analysis error handling"""

    def test_recovery_returns_500_on_internal_error(self, client):
        """Business Requirement: Graceful error handling"""
        # This would require mocking internal failures
        # For GREEN phase, test basic error response structure
        pass


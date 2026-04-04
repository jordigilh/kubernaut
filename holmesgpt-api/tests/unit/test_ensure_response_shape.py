"""UT-HAPI-624-004..007: Response shape normalization tests.

Issue #624: Early-exit result dicts from llm_integration.py lack required
IncidentResponseData fields. These tests verify the normalizer backfills
missing fields with safe defaults.
"""


class TestEnsureResponseShape:
    """UT-HAPI-624-004..007: ensure_incident_response_shape normalizer."""

    def test_ut_hapi_624_004_fills_missing_fields(self):
        """UT-HAPI-624-004: Missing required fields are filled with safe defaults."""
        from src.extensions.incident.result_parser import ensure_incident_response_shape

        partial = {
            "root_cause_analysis": {"summary": "test"},
            "needs_human_review": True,
            "human_review_reason": "rca_incomplete",
        }
        normalized = ensure_incident_response_shape(
            partial, incident_id="inc-001", analysis="Test analysis"
        )

        assert normalized["incident_id"] == "inc-001"
        assert normalized["analysis"] == "Test analysis"
        assert "confidence" in normalized
        assert "timestamp" in normalized
        assert normalized["root_cause_analysis"]["summary"] == "test"

    def test_ut_hapi_624_005_preserves_existing_fields(self):
        """UT-HAPI-624-005: Existing fields are preserved unchanged."""
        from src.extensions.incident.result_parser import ensure_incident_response_shape

        full_result = {
            "incident_id": "inc-existing",
            "analysis": "Existing analysis",
            "root_cause_analysis": {"summary": "existing", "severity": "high", "contributing_factors": []},
            "confidence": 0.95,
            "timestamp": "2026-03-04T12:00:00Z",
            "needs_human_review": False,
            "warnings": ["test warning"],
        }
        normalized = ensure_incident_response_shape(
            full_result, incident_id="inc-fallback", analysis="Fallback"
        )

        assert normalized["incident_id"] == "inc-existing", "Should preserve existing incident_id"
        assert normalized["analysis"] == "Existing analysis", "Should preserve existing analysis"
        assert normalized["confidence"] == 0.95, "Should preserve existing confidence"
        assert normalized["timestamp"] == "2026-03-04T12:00:00Z", "Should preserve existing timestamp"

    def test_ut_hapi_624_006_enrichment_failure_passes_validation(self):
        """UT-HAPI-624-006: Enrichment failure dict passes IncidentResponseData validation after normalization."""
        from src.extensions.incident.result_parser import ensure_incident_response_shape

        enrichment_failure = {
            "root_cause_analysis": {"summary": "OOM detected", "severity": "high", "contributing_factors": ["memory pressure"]},
            "needs_human_review": True,
            "human_review_reason": "rca_incomplete",
        }
        normalized = ensure_incident_response_shape(
            enrichment_failure, incident_id="inc-enrich-fail", analysis="OOM analysis text"
        )

        assert "incident_id" in normalized
        assert "analysis" in normalized
        assert "confidence" in normalized
        assert "timestamp" in normalized
        assert normalized["needs_human_review"] is True

    def test_ut_hapi_624_007_phase1_exhaustion_passes_validation(self):
        """UT-HAPI-624-007: Phase-1 exhaustion dict passes IncidentResponseData validation after normalization."""
        from src.extensions.incident.result_parser import ensure_incident_response_shape

        phase1_exhaustion = {
            "root_cause_analysis": {"summary": "Phase 1 failed to identify affected resource after all attempts"},
            "needs_human_review": True,
            "human_review_reason": "rca_incomplete",
            "selected_workflow": None,
        }
        normalized = ensure_incident_response_shape(
            phase1_exhaustion, incident_id="inc-phase1-exhaust", analysis="Phase 1 exhaustion analysis"
        )

        assert "incident_id" in normalized
        assert "analysis" in normalized
        assert "confidence" in normalized
        assert "timestamp" in normalized
        assert normalized["needs_human_review"] is True
        assert normalized["selected_workflow"] is None
